// Package whipecho implements the framework-independent diagnostic echo
// capability shared by TiRTC WHIP services and the echo sidecar.
package whipecho

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

const (
	// CmdTimeSyncRequest requests an Echo server timestamp.
	CmdTimeSyncRequest uint32 = 0x2010
	// CmdTimeSyncResponse returns an Echo server timestamp.
	CmdTimeSyncResponse uint32 = 0x2011
)

var (
	// ErrInvalidArgument indicates invalid caller input or configuration.
	ErrInvalidArgument = errors.New("whipecho: invalid argument")
	// ErrClosed indicates that the Echo session manager is shutting down.
	ErrClosed = errors.New("whipecho: session manager closed")
)

// AcceptRequest contains the SDP offer for a new Echo session.
type AcceptRequest struct {
	OfferSDP []byte
}

// AcceptResult contains the SDP answer and resource identity for an Echo session.
type AcceptResult struct {
	SessionID string
	AnswerSDP []byte
	Location  string
}

type echoSessionManager interface {
	Accept(context.Context, AcceptRequest) (*AcceptResult, error)
	Delete(context.Context, string) error
}

// Acceptor accepts an SDP offer without imposing an authentication policy.
type Acceptor interface {
	WhipAccept(context.Context, []byte, tirtcx.ConnEventOptions) (*tirtcx.WhipSession, error)
}

// Options configures Echo session behavior and observability.
type Options struct {
	ConnTimeout time.Duration
	Logger      *slog.Logger
	Observer    Observer
	stats       *statsRecorder
}

// SessionManager owns Echo sessions and their diagnostic lifecycle.
type SessionManager struct {
	acceptor       Acceptor
	locationPrefix string
	opts           Options
	stats          *statsRecorder

	mu       sync.Mutex
	sessions map[string]*session
	closed   bool
	wg       sync.WaitGroup
}

// NewSessionManager creates a stateful Echo session manager.
func NewSessionManager(
	acceptor Acceptor,
	locationPrefix string,
	opts Options,
) (*SessionManager, error) {
	if acceptor == nil {
		return nil, fmt.Errorf("%w: acceptor is nil", ErrInvalidArgument)
	}
	if err := validateLocationPrefix(locationPrefix); err != nil {
		return nil, err
	}
	if opts.ConnTimeout <= 0 {
		opts.ConnTimeout = 30 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Observer == nil {
		opts.Observer = noopObserver{}
	}
	opts.stats = &statsRecorder{}
	return &SessionManager{
		acceptor:       acceptor,
		locationPrefix: locationPrefix,
		opts:           opts,
		stats:          opts.stats,
		sessions:       make(map[string]*session),
	}, nil
}

// Stats returns a concurrency-safe point-in-time snapshot.
func (s *SessionManager) Stats() Stats {
	if s == nil || s.stats == nil {
		return Stats{ClosedTotal: map[CloseReason]uint64{}}
	}
	return s.stats.snapshot()
}

// Accept accepts an SDP offer and starts managing the resulting Echo session.
func (s *SessionManager) Accept(ctx context.Context, req AcceptRequest) (*AcceptResult, error) {
	if ctx == nil {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, fmt.Errorf("%w: context is nil", ErrInvalidArgument)
	}
	if len(req.OfferSDP) == 0 {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, fmt.Errorf("%w: offer is empty", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	sessionID, err := randomSessionID()
	if err != nil {
		s.recordAcceptFailure(AcceptStageSessionID)
		return nil, err
	}
	whipSession, err := s.acceptor.WhipAccept(ctx, req.OfferSDP, tirtcx.ConnEventOptions{
		AudioBuffer:        16,
		VideoBuffer:        16,
		CommandBuffer:      16,
		ErrorBuffer:        tirtcx.MinEventBufferSize,
		DisconnectedBuffer: tirtcx.MinEventBufferSize,
	})
	if err != nil {
		s.recordAcceptFailure(AcceptStageWHIP)
		return nil, err
	}
	pending := newSession(sessionID, "", whipSession, nil, s.opts, func(reason CloseReason) {
		s.remove(sessionID, reason)
	})
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = whipSession.Close()
		return nil, ErrClosed
	}
	s.sessions[sessionID] = pending
	s.stats.activeSessions.Store(uint64(len(s.sessions)))
	s.wg.Add(1)
	s.mu.Unlock()
	s.stats.acceptedTotal.Add(1)
	s.opts.Observer.SessionAccepted()
	go func() {
		defer s.wg.Done()
		s.waitAndRun(pending, whipSession)
	}()

	return &AcceptResult{
		SessionID: sessionID,
		AnswerSDP: whipSession.AnswerSDP(),
		Location:  s.locationPrefix + "/" + sessionID,
	}, nil
}

func (s *SessionManager) waitAndRun(
	session *session,
	whipSession *tirtcx.WhipSession,
) {
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), s.opts.ConnTimeout)
	defer cancelConnect()
	conn, err := whipSession.WaitConn(connectCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.stats.connectTimeoutTotal.Add(1)
			s.recordConnectFailure(ConnectFailureTimeout)
		} else if errors.Is(err, context.Canceled) || errors.Is(err, tirtcx.ErrWhipSessionClosed) {
			return
		} else {
			s.recordConnectFailure(ConnectFailureError)
		}
		s.opts.Logger.Error("echo connection failed",
			"session_id", session.id,
			"error", err)
		session.finish(CloseReasonConnectionFailed)
		return
	}
	if !session.bind(conn) {
		_ = conn.Close()
		return
	}
	s.opts.Logger.Info("echo session connected",
		"session_id", session.id,
		"device_id", session.deviceID)
	s.opts.Observer.SessionConnected(time.Since(session.createdAt))
	session.run(conn.Events())
}

// Shutdown rejects new sessions, closes all active sessions, and waits for
// their goroutines to exit until ctx is canceled.
func (s *SessionManager) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("%w: shutdown context is nil", ErrInvalidArgument)
	}
	s.mu.Lock()
	s.closed = true
	sessions := make([]*session, 0, len(s.sessions))
	for id, active := range s.sessions {
		sessions = append(sessions, active)
		delete(s.sessions, id)
	}
	s.stats.activeSessions.Store(0)
	s.mu.Unlock()

	for _, active := range sessions {
		if err := active.shutdown(CloseReasonShutdown); err != nil {
			s.opts.Logger.Error("close Echo session during shutdown",
				"session_id", active.id,
				"error", err)
		}
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Delete terminates a session. Deleting an unknown non-empty ID is idempotent.
func (s *SessionManager) Delete(_ context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("%w: session ID is empty", ErrInvalidArgument)
	}
	s.mu.Lock()
	session := s.sessions[sessionID]
	if session != nil {
		delete(s.sessions, sessionID)
	}
	count := len(s.sessions)
	s.stats.activeSessions.Store(uint64(count))
	s.mu.Unlock()
	if session == nil {
		return nil // WHIP DELETE is intentionally idempotent.
	}
	s.stats.deletedTotal.Add(1)
	return session.close()
}

func (s *SessionManager) remove(id string, reason CloseReason) {
	s.mu.Lock()
	session := s.sessions[id]
	if session != nil {
		delete(s.sessions, id)
	}
	count := len(s.sessions)
	s.stats.activeSessions.Store(uint64(count))
	s.mu.Unlock()
	if session != nil {
		s.opts.Logger.Info("echo session removed",
			"session_id", id,
			"reason", reason)
	}
}

func (s *SessionManager) recordAcceptFailure(stage AcceptStage) {
	s.stats.acceptFailedTotal.Add(1)
	s.opts.Observer.AcceptFailed(stage)
}

func (s *SessionManager) recordConnectFailure(reason ConnectFailureReason) {
	s.stats.connectFailedTotal.Add(1)
	s.opts.Observer.ConnectFailed(reason)
}

func validateLocationPrefix(locationPrefix string) error {
	switch {
	case locationPrefix == "":
		return fmt.Errorf("%w: invalid Location prefix %q: path is empty", ErrInvalidArgument, locationPrefix)
	case !strings.HasPrefix(locationPrefix, "/"):
		return fmt.Errorf("%w: invalid Location prefix %q: path must start with '/'", ErrInvalidArgument, locationPrefix)
	case strings.HasSuffix(locationPrefix, "/"):
		return fmt.Errorf("%w: invalid Location prefix %q: trailing slash is not allowed", ErrInvalidArgument, locationPrefix)
	case strings.ContainsAny(locationPrefix, "?#{} \t\r\n"):
		return fmt.Errorf("%w: invalid Location prefix %q: query, fragment, wildcard, and whitespace are not allowed", ErrInvalidArgument, locationPrefix)
	case path.Clean(locationPrefix) != locationPrefix:
		return fmt.Errorf("%w: invalid Location prefix %q: path is not canonical", ErrInvalidArgument, locationPrefix)
	default:
		return nil
	}
}

func randomSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

// TimeSyncRequest is the payload for CmdTimeSyncRequest.
type TimeSyncRequest struct {
	Version          int    `json:"version"`
	Seq              uint32 `json:"seq"`
	ClientSendUnixNS int64  `json:"client_send_unix_ns"`
}

// TimeSyncResponse is the payload for CmdTimeSyncResponse.
type TimeSyncResponse struct {
	Version          int    `json:"version"`
	Seq              uint32 `json:"seq"`
	ClientSendUnixNS int64  `json:"client_send_unix_ns"`
	ServerRecvUnixNS int64  `json:"server_recv_unix_ns"`
}

func marshalTimeSync(data []byte, receivedAt time.Time) ([]byte, error) {
	var request TimeSyncRequest
	if len(data) > 0 {
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
	}
	return json.Marshal(TimeSyncResponse{
		Version:          1,
		Seq:              request.Seq,
		ClientSendUnixNS: request.ClientSendUnixNS,
		ServerRecvUnixNS: receivedAt.UnixNano(),
	})
}

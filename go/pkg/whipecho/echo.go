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
	"net/url"
	"strings"
	"sync"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

const (
	ModeQueryKey = "_tg_mode"
	ModeEcho     = "echo"

	CmdTimeSyncRequest  uint32 = 0x2010
	CmdTimeSyncResponse uint32 = 0x2011
)

var (
	ErrBadRequest = errors.New("whipecho: bad request")
	ErrDisabled   = errors.New("whipecho: disabled")
)

type EchoRequest struct {
	OfferSDP []byte
	RawQuery string
}

type EchoResult struct {
	SessionID string
	AnswerSDP []byte
	Location  string
}

type EchoService interface {
	Accept(context.Context, EchoRequest) (*EchoResult, error)
	Delete(context.Context, string) error
}

// Acceptor accepts an SDP offer without imposing an authentication policy.
type Acceptor interface {
	WhipAccept([]byte, tirtcx.ConnEventOptions) (*tirtcx.WhipAcceptResult, error)
}

type Options struct {
	Acceptor       Acceptor
	Enabled        bool
	RequireMode    bool
	LocationPrefix string
	ConnTimeout    time.Duration
	Logger         *slog.Logger
	Observer       Observer
	stats          *statsRecorder
}

type Service struct {
	opts  Options
	stats *statsRecorder

	mu       sync.Mutex
	sessions map[string]*session
}

func New(opts Options) (*Service, error) {
	if opts.Acceptor == nil {
		return nil, fmt.Errorf("%w: acceptor is nil", ErrBadRequest)
	}
	if opts.LocationPrefix == "" {
		opts.LocationPrefix = "/whip/echo/resource"
	}
	opts.LocationPrefix = "/" + strings.Trim(strings.TrimSpace(opts.LocationPrefix), "/")
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
	return &Service{opts: opts, stats: opts.stats, sessions: make(map[string]*session)}, nil
}

// Stats returns a concurrency-safe point-in-time snapshot.
func (s *Service) Stats() Stats {
	if s == nil || s.stats == nil {
		return Stats{ClosedTotal: map[CloseReason]uint64{}}
	}
	return s.stats.snapshot()
}

// Mode returns the single, canonical _tg_mode value. Duplicate values and
// malformed query strings are rejected so routing and token verification see
// the same request semantics.
func Mode(rawQuery string) (string, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("%w: invalid query: %v", ErrBadRequest, err)
	}
	modes, exists := values[ModeQueryKey]
	if !exists {
		return "", nil
	}
	if len(modes) != 1 {
		return "", fmt.Errorf("%w: duplicate %s", ErrBadRequest, ModeQueryKey)
	}
	mode := strings.TrimSpace(modes[0])
	if mode == "" {
		return "", fmt.Errorf("%w: empty %s", ErrBadRequest, ModeQueryKey)
	}
	return mode, nil
}

func (s *Service) Accept(ctx context.Context, req EchoRequest) (*EchoResult, error) {
	if s == nil || !s.opts.Enabled {
		if s != nil {
			s.recordAcceptFailure(AcceptStageValidate)
		}
		return nil, ErrDisabled
	}
	if len(req.OfferSDP) == 0 {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, fmt.Errorf("%w: offer is empty", ErrBadRequest)
	}
	mode, err := Mode(req.RawQuery)
	if err != nil {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, err
	}
	if s.opts.RequireMode && mode != ModeEcho {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, fmt.Errorf("%w: %s must be %q", ErrBadRequest, ModeQueryKey, ModeEcho)
	}
	if mode != "" && mode != ModeEcho {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, fmt.Errorf("%w: unsupported %s %q", ErrBadRequest, ModeQueryKey, mode)
	}
	if err := ctx.Err(); err != nil {
		s.recordAcceptFailure(AcceptStageValidate)
		return nil, err
	}

	result, err := s.opts.Acceptor.WhipAccept(req.OfferSDP, tirtcx.ConnEventOptions{
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
	sessionID, err := randomSessionID()
	if err != nil {
		s.recordAcceptFailure(AcceptStageSessionID)
		return nil, err
	}
	pending := newSession(sessionID, "", nil, s.opts, func(reason CloseReason) {
		s.remove(sessionID, reason)
	})
	s.put(sessionID, pending)
	s.stats.acceptedTotal.Add(1)
	s.opts.Observer.SessionAccepted()
	go s.waitAndRun(pending, result)

	return &EchoResult{
		SessionID: sessionID,
		AnswerSDP: result.AnswerSDP,
		Location:  s.opts.LocationPrefix + "/" + sessionID,
	}, nil
}

func (s *Service) waitAndRun(session *session, result *tirtcx.WhipAcceptResult) {
	conn, err := result.WaitConn(s.opts.ConnTimeout)
	if err != nil {
		s.recordAcceptFailure(AcceptStageConnect)
		if errors.Is(err, tirtcx.ErrConnTimeout) {
			s.stats.connectTimeoutTotal.Add(1)
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

func (s *Service) Delete(_ context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
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

func (s *Service) put(id string, session *session) {
	s.mu.Lock()
	s.sessions[id] = session
	count := len(s.sessions)
	s.stats.activeSessions.Store(uint64(count))
	s.mu.Unlock()
}

func (s *Service) remove(id string, reason CloseReason) {
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

func (s *Service) recordAcceptFailure(stage AcceptStage) {
	s.stats.acceptFailedTotal.Add(1)
	s.opts.Observer.AcceptFailed(stage)
}

func randomSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

type TimeSyncRequest struct {
	Version          int    `json:"version"`
	Seq              uint32 `json:"seq"`
	ClientSendUnixNS int64  `json:"client_send_unix_ns"`
}

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

package tirtcx

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

var acceptWHIPFunc = acceptWHIP

// WhipAcceptor owns WHIP accept defaults without imposing an authentication
// policy. HTTP authentication should normally be implemented as middleware.
type WhipAcceptor struct {
	candidate string
}

// NewWhipAcceptor creates an authentication-independent WHIP acceptor.
func NewWhipAcceptor(candidate string) *WhipAcceptor {
	return &WhipAcceptor{candidate: strings.TrimSpace(candidate)}
}

// WhipSession owns a WHIP accept operation and its pending or connected Conn.
// Applications own resource IDs, Location paths, registries, and DELETE policy.
type WhipSession struct {
	answerSDP  []byte
	conn       *Conn
	connReady  chan struct{}
	resultOnce sync.Once
	resultConn *Conn
	resultErr  error
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
	waitErr    error
}

// WhipAccept calls TiRtcWhipAccept without performing authentication. ctx only
// controls the synchronous SDP-answer phase. The returned WhipSession owns
// connection establishment and its lifecycle until Close or disconnection.
//
// The caller owns authentication and HTTP behavior, including endpoint
// routing, Location resource design, and DELETE/PATCH semantics.
func (a *WhipAcceptor) WhipAccept(
	ctx context.Context,
	offer []byte,
	eventOptions ConnEventOptions,
) (*WhipSession, error) {
	if a == nil {
		return nil, ErrNilWhipAcceptor
	}
	if ctx == nil {
		return nil, fmt.Errorf("tirtcx: WHIP accept context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn := newConn(a.candidate, eventOptions)
	return acceptWHIPFunc(ctx, conn, offer, a.candidate)
}

func acceptWHIP(
	ctx context.Context,
	conn *Conn,
	offer []byte,
	candidate string,
) (*WhipSession, error) {
	sdpCh := make(chan []byte, 1)
	acceptErrCh := make(chan error, 1)
	session := &WhipSession{
		conn:      conn,
		connReady: make(chan struct{}),
		done:      make(chan struct{}),
	}

	err := tirtcclib.TiRtcWhipAccept(
		offer,
		candidate,
		func(sdp []byte) {
			select {
			case sdpCh <- append([]byte(nil), sdp...):
			default:
			}
		},
		func(errorCode int, hconn tirtcclib.ConnHandle) {
			if errorCode != 0 {
				err := tirtcclib.ErrorFromCode(errorCode)
				if sendToChan(conn.errorCh, err) {
					conn.eventStats.errorDropped.Add(1)
				}
				session.resolveConn(nil, err)
				select {
				case acceptErrCh <- err:
				default:
				}
				return
			}
			if _, ok := bindConnHandle(conn, hconn); !ok {
				err := fmt.Errorf("tirtcx: accepted handle bound to unexpected conn")
				session.resolveConn(nil, err)
				select {
				case acceptErrCh <- err:
				default:
				}
				return
			}
			session.resolveConn(conn, nil)
		},
	)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	select {
	case answer := <-sdpCh:
		session.answerSDP = answer
		return session, nil
	case err := <-acceptErrCh:
		_ = conn.Close()
		return nil, err
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	}
}

// AnswerSDP returns a copy of the SDP answer.
func (s *WhipSession) AnswerSDP() []byte {
	if s == nil {
		return nil
	}
	return append([]byte(nil), s.answerSDP...)
}

// WaitConn waits for the WHIP connection callback. Each caller may use an
// independent context; concurrent and repeated calls receive the same eventual
// connection result. Canceling a wait does not close the session.
func (s *WhipSession) WaitConn(ctx context.Context) (*Conn, error) {
	if s == nil {
		return nil, ErrNilWhipSession
	}
	if ctx == nil {
		return nil, fmt.Errorf("tirtcx: wait connection context is nil")
	}
	select {
	case <-s.done:
		return nil, s.waitErr
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.connReady:
		select {
		case <-s.done:
			return nil, s.waitErr
		default:
			return s.resultConn, s.resultErr
		}
	}
}

func (s *WhipSession) resolveConn(conn *Conn, err error) {
	s.resultOnce.Do(func() {
		s.resultConn = conn
		s.resultErr = err
		close(s.connReady)
	})
}

// Close terminates the pending or connected WHIP connection. It is idempotent.
func (s *WhipSession) Close() error {
	return s.close(ErrWhipSessionClosed)
}

func (s *WhipSession) close(waitErr error) error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.waitErr = waitErr
		if s.done != nil {
			close(s.done)
		}
		if s.conn != nil {
			s.closeErr = s.conn.Close()
		}
	})
	return s.closeErr
}

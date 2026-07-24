package tirtcx

import (
	"fmt"
	"strings"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

const defaultAnswerTimeout = 15 * time.Second

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

// WhipAcceptResult is the non-HTTP result of TiRtcWhipAccept.
type WhipAcceptResult struct {
	AnswerSDP []byte
	connCh    chan *Conn
	errCh     chan error
}

// WhipAccept calls TiRtcWhipAccept without performing authentication.
//
// The caller owns authentication and HTTP behavior, including endpoint
// routing, Location resource design, and DELETE/PATCH semantics.
func (a *WhipAcceptor) WhipAccept(offer []byte, eventOptions ConnEventOptions) (*WhipAcceptResult, error) {
	if a == nil {
		return nil, ErrNilWhipAcceptor
	}
	conn := newConn(a.candidate, eventOptions)
	return acceptWHIPFunc(conn, offer, a.candidate)
}

func acceptWHIP(conn *Conn, offer []byte, candidate string) (*WhipAcceptResult, error) {
	sdpCh := make(chan []byte, 1)
	connCh := make(chan *Conn, 1)
	errCh := make(chan error, 1)

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
				select {
				case errCh <- err:
				default:
				}
				return
			}
			if _, ok := bindConnHandle(conn, hconn); !ok {
				select {
				case errCh <- fmt.Errorf("tirtcx: accepted handle bound to unexpected conn"):
				default:
				}
				return
			}
			select {
			case connCh <- conn:
			default:
			}
		},
	)
	if err != nil {
		return nil, err
	}

	select {
	case answer := <-sdpCh:
		return &WhipAcceptResult{
			AnswerSDP: answer,
			connCh:    connCh,
			errCh:     errCh,
		}, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(defaultAnswerTimeout):
		return nil, ErrAnswerTimeout
	}
}

// WaitConn waits for TiRtcWhipAccept's connection callback.
func (r *WhipAcceptResult) WaitConn(timeout time.Duration) (*Conn, error) {
	if r == nil {
		return nil, ErrNilResult
	}
	select {
	case conn := <-r.connCh:
		return conn, nil
	case err := <-r.errCh:
		return nil, err
	case <-time.After(timeout):
		return nil, ErrConnTimeout
	}
}

// WhipConnect calls TiRtcWhipConnect and waits for the TiRTC connection.
func WhipConnect(serviceDesc, token string, timeout time.Duration) (*Conn, error) {
	connCh := make(chan *Conn, 1)
	errCh := make(chan error, 1)
	if err := tirtcclib.TiRtcWhipConnect(serviceDesc, token, func(errorCode int, hconn tirtcclib.ConnHandle) {
		if errorCode != 0 {
			select {
			case errCh <- tirtcclib.ErrorFromCode(errorCode):
			default:
			}
			return
		}
		select {
		case connCh <- registerConn(hconn):
		default:
		}
	}); err != nil {
		return nil, err
	}

	select {
	case conn := <-connCh:
		return conn, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(timeout):
		return nil, ErrConnTimeout
	}
}

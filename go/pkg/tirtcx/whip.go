package tirtcx

import (
	"fmt"
	"strings"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcclib"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
)

const defaultAnswerTimeout = 15 * time.Second

var acceptWHIPFunc = acceptWHIP

// Options configures a TiRTC-X WHIP server helper.
type Options struct {
	Service   string
	Verifier  tirtcxauth.TokenVerifier
	Candidate string
}

// WhipServer owns TiRTC-X token verification config and WHIP accept defaults.
type WhipServer struct {
	service   string
	candidate string
	verifier  tirtcxauth.TokenVerifier
}

// NewWhipServer creates a TiRTC-X WHIP server initialized with a caller-provided verifier.
func NewWhipServer(opts Options) (*WhipServer, error) {
	opts.Service = strings.TrimSpace(opts.Service)
	if opts.Service == "" {
		return nil, fmt.Errorf("%w: service is empty", ErrInvalidConfig)
	}
	if opts.Verifier == nil {
		return nil, fmt.Errorf("%w: verifier is nil", ErrInvalidConfig)
	}
	return &WhipServer{
		service:   opts.Service,
		candidate: strings.TrimSpace(opts.Candidate),
		verifier:  opts.Verifier,
	}, nil
}

// Auth verifies query/token with tirtcxauth and returns the token subject.
func (s *WhipServer) Auth(query string, token string) (deviceID string, err error) {
	if s == nil {
		return "", ErrNilWhipServer
	}
	claims, err := s.verifier.Verify(s.service, "", query, token)
	if err != nil {
		return "", fmt.Errorf("%w: verify token: %v", ErrUnauthorized, err)
	}
	return claims.Subject, nil
}

// WhipAcceptResult is the non-HTTP result of TiRtcWhipAccept.
type WhipAcceptResult struct {
	AnswerSDP []byte
	connCh    chan *Conn
	errCh     chan error
}

// WhipAccept calls TiRtcWhipAccept with callbacks installed on the accepted Conn.
//
// The caller still owns HTTP behavior, including endpoint routing, Location
// resource design, and DELETE/PATCH semantics.
func (s *WhipServer) WhipAccept(offer []byte, eventOptions ConnEventOptions) (*WhipAcceptResult, error) {
	if s == nil {
		return nil, ErrNilWhipServer
	}
	conn := newConn(s.candidate, eventOptions)
	return acceptWHIPFunc(conn, offer, s.candidate)
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

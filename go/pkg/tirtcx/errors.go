package tirtcx

import "errors"

var (
	ErrNilWhipAcceptor    = errors.New("tirtcx: nil WHIP acceptor")
	ErrNilContext         = errors.New("tirtcx: nil context")
	ErrNilConn            = errors.New("tirtcx: nil conn")
	ErrNilWhipSession     = errors.New("tirtcx: nil WHIP session")
	ErrWhipSessionClosed  = errors.New("tirtcx: WHIP session closed")
	ErrStoppedBeforeStart = errors.New("tirtcx: stopped before start completed")
)

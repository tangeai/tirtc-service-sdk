package tirtcx

import "errors"

var (
	ErrNilWhipAcceptor    = errors.New("tirtcx: nil WHIP acceptor")
	ErrNilConn            = errors.New("tirtcx: nil conn")
	ErrNilWhipSession     = errors.New("tirtcx: nil WHIP session")
	ErrWhipSessionClosed  = errors.New("tirtcx: WHIP session closed")
	ErrStartTimeout       = errors.New("tirtcx: start timeout")
	ErrStoppedBeforeStart = errors.New("tirtcx: stopped before start completed")
)

package tirtcx

import "errors"

var (
	ErrNilWhipAcceptor = errors.New("tirtcx: nil WHIP acceptor")
	ErrNilConn         = errors.New("tirtcx: nil conn")
	ErrNilResult       = errors.New("tirtcx: nil WHIP accept result")
	ErrAnswerTimeout   = errors.New("tirtcx: WHIP answer timeout")
	ErrConnTimeout     = errors.New("tirtcx: WHIP connection timeout")
)

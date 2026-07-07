package tirtcx

import "errors"

var (
	ErrInvalidConfig = errors.New("tirtcx: invalid config")
	ErrNilWhipServer = errors.New("tirtcx: nil WHIP server")
	ErrNilConn       = errors.New("tirtcx: nil conn")
	ErrNilResult     = errors.New("tirtcx: nil WHIP accept result")
	ErrUnauthorized  = errors.New("tirtcx: unauthorized")
	ErrAnswerTimeout = errors.New("tirtcx: WHIP answer timeout")
	ErrConnTimeout   = errors.New("tirtcx: WHIP connection timeout")
)

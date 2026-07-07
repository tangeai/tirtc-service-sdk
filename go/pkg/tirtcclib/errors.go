package tirtcclib

import (
	"errors"
	"fmt"
)

var (
	ErrUnavailable     = errors.New("tirtcclib: clib wrapper unavailable; build with -tags tirtc_clib and provide native libraries")
	ErrNotInitialized  = errors.New("tirtcclib: not initialized")
	ErrInvalidHandle   = errors.New("tirtcclib: invalid handle")
	ErrInvalidParam    = errors.New("tirtcclib: invalid parameter")
	ErrInvalidLicense  = errors.New("tirtcclib: invalid license")
	ErrTimeout         = errors.New("tirtcclib: timeout")
	ErrBusy            = errors.New("tirtcclib: busy")
	ErrConnTimeout     = errors.New("tirtcclib: connection timeout close")
	ErrConnRemoteClose = errors.New("tirtcclib: remote close")
	ErrConnOther       = errors.New("tirtcclib: connection error")
	ErrLackOfResource  = errors.New("tirtcclib: lack of resource")
	ErrCacheExpired    = errors.New("tirtcclib: cache expired")
	ErrServerError     = errors.New("tirtcclib: server error")
	ErrInternal        = errors.New("tirtcclib: internal error")
	ErrNoSecretKey     = errors.New("tirtcclib: no secret key")
	ErrUnexpectedResp  = errors.New("tirtcclib: unexpected response")
)

var errorCodeMap = map[int]error{
	-40001: ErrNotInitialized,
	-40002: ErrInvalidHandle,
	-40003: ErrInvalidParam,
	-40004: ErrInvalidLicense,
	-40005: ErrTimeout,
	-40006: ErrBusy,
	-40007: ErrConnTimeout,
	-40008: ErrConnRemoteClose,
	-40009: ErrConnOther,
	-40010: ErrLackOfResource,
	-40011: ErrCacheExpired,
	-40012: ErrServerError,
	-40013: ErrInternal,
	-40014: ErrNoSecretKey,
	-40015: ErrUnexpectedResp,
}

// ErrorFromCode maps a tiRTC.h TIRTC_E_* return code to a Go error.
func ErrorFromCode(code int) error {
	if code == 0 {
		return nil
	}
	if err, ok := errorCodeMap[code]; ok {
		return err
	}
	return fmt.Errorf("tirtcclib: error code %d", code)
}

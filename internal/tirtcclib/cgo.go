//go:build tirtc_clib

package tirtcclib

/*
#cgo CFLAGS: -I${SRCDIR}/../../clib/include/tirtc
#cgo LDFLAGS: ${SRCDIR}/../../clib/lib/libTiRTC.a -lm -ldl
#include <stdlib.h>
#include "tiRTC.h"

TIRTCCALLBACKS* bridge_get_rtc_callbacks();
int bridge_whip_accept(const char *offer, int offer_len, const char *candidate, void *context);
int bridge_whip_connect(const char *service_desc, const char *token, void *context);
*/
import "C"

import (
	"sync"
	"unsafe"
)

var (
	callbackMu      sync.RWMutex
	callbacks       *Callbacks
	whipRegistryMu  sync.Mutex
	whipRegistry    = map[unsafe.Pointer]whipCallbacks{}
	connectRegistry = map[unsafe.Pointer]ConnectCallback{}
)

type whipCallbacks struct {
	onSDP WhipServerOnSDPReadyCallback
	cb    ConnectCallback
}

// TiRtcGetVersion maps to tiRTC.h TiRtcGetVersion.
func TiRtcGetVersion() string {
	return C.GoString(C.TiRtcGetVersion())
}

// TiRtcLogSetLevel maps to tiRTC.h TiRtcLogSetLevel.
func TiRtcLogSetLevel(level int) {
	C.TiRtcLogSetLevel(C.int(level))
}

// TiRtcSetAudioRedundancyLossThreshold maps to tiRTC.h
// TiRtcSetAudioRedundancyLossThreshold.
func TiRtcSetAudioRedundancyLossThreshold(lossPercent int) int {
	return int(C.TiRtcSetAudioRedundancyLossThreshold(C.int(lossPercent)))
}

// TiRtcInit maps to tiRTC.h TiRtcInit.
func TiRtcInit() error {
	return ErrorFromCode(int(C.TiRtcInit()))
}

// TiRtcUninit maps to tiRTC.h TiRtcUninit.
func TiRtcUninit() {
	C.TiRtcUninit()
}

// TiRtcStart maps to tiRTC.h TiRtcStart.
func TiRtcStart(deviceID string, cbs *Callbacks) error {
	callbackMu.Lock()
	callbacks = cbs
	callbackMu.Unlock()

	var cDeviceID *C.char
	if deviceID != "" {
		cDeviceID = C.CString(deviceID)
		defer C.free(unsafe.Pointer(cDeviceID))
	}
	return ErrorFromCode(int(C.TiRtcStart(cDeviceID, C.bridge_get_rtc_callbacks())))
}

// TiRtcStop maps to tiRTC.h TiRtcStop.
func TiRtcStop() {
	C.TiRtcStop()
}

// TiRtcSetOption maps to tiRTC.h TiRtcSetOption.
func TiRtcSetOption(option Option, value []byte) error {
	var ptr unsafe.Pointer
	if len(value) > 0 {
		ptr = C.CBytes(value)
		defer C.free(ptr)
	}
	return ErrorFromCode(int(C.TiRtcSetOption(C.TIRTCOPTION(option), ptr, C.uint32_t(len(value)))))
}

// TiRtcWhipAccept maps to tiRTC.h TiRtcWhipAccept.
func TiRtcWhipAccept(
	offer []byte,
	candidate string,
	onSDP WhipServerOnSDPReadyCallback,
	cb ConnectCallback,
) (func(), error) {
	key := C.malloc(1)
	whipRegistryMu.Lock()
	whipRegistry[key] = whipCallbacks{onSDP: onSDP, cb: cb}
	whipRegistryMu.Unlock()

	var offerPtr *C.char
	if len(offer) > 0 {
		offerPtr = (*C.char)(C.CBytes(offer))
		defer C.free(unsafe.Pointer(offerPtr))
	}
	var cCandidate *C.char
	if candidate != "" {
		cCandidate = C.CString(candidate)
		defer C.free(unsafe.Pointer(cCandidate))
	}
	ret := C.bridge_whip_accept(offerPtr, C.int(len(offer)), cCandidate, key)
	if ret != 0 {
		whipRegistryMu.Lock()
		delete(whipRegistry, key)
		whipRegistryMu.Unlock()
		C.free(key)
		return nil, ErrorFromCode(int(ret))
	}
	var cancelOnce sync.Once
	cancel := func() {
		cancelOnce.Do(func() {
			whipRegistryMu.Lock()
			entry, ok := whipRegistry[key]
			if ok {
				entry.onSDP = nil
				entry.cb = func(_ int, hconn ConnHandle) {
					if hconn != nil {
						go func() {
							_ = TiRtcDisconnect(hconn)
						}()
					}
				}
				whipRegistry[key] = entry
			}
			whipRegistryMu.Unlock()
		})
	}
	return cancel, nil
}

// TiRtcWhipConnect maps to tiRTC.h TiRtcWhipConnect.
func TiRtcWhipConnect(serviceDesc, token string, cb ConnectCallback) error {
	key := C.malloc(1)
	whipRegistryMu.Lock()
	connectRegistry[key] = cb
	whipRegistryMu.Unlock()

	cServiceDesc := C.CString(serviceDesc)
	defer C.free(unsafe.Pointer(cServiceDesc))
	var cToken *C.char
	if token != "" {
		cToken = C.CString(token)
		defer C.free(unsafe.Pointer(cToken))
	}
	ret := C.bridge_whip_connect(cServiceDesc, cToken, key)
	if ret != 0 {
		whipRegistryMu.Lock()
		delete(connectRegistry, key)
		whipRegistryMu.Unlock()
		C.free(key)
	}
	return ErrorFromCode(int(ret))
}

// TiRtcDisconnect maps to tiRTC.h TiRtcDisconnect.
func TiRtcDisconnect(hconn ConnHandle) error {
	return ErrorFromCode(int(C.TiRtcDisconnect(C.tirtc_conn_t(hconn))))
}

// TiRtcSendMessageStream maps to tiRTC.h TiRtcSendMessageStream.
func TiRtcSendMessageStream(hconn ConnHandle, info FrameInfo, frame []byte) error {
	cInfo, ptr, cleanup := frameArgs(info, frame)
	defer cleanup()
	return errorFromSendResult(int(C.TiRtcSendMessageStream(C.tirtc_conn_t(hconn), &cInfo, ptr)))
}

// TiRtcSendVideoStream maps to tiRTC.h TiRtcSendVideoStream.
func TiRtcSendVideoStream(hconn ConnHandle, info FrameInfo, frame []byte) error {
	cInfo, ptr, cleanup := frameArgs(info, frame)
	defer cleanup()
	return errorFromSendResult(int(C.TiRtcSendVideoStream(C.tirtc_conn_t(hconn), &cInfo, ptr)))
}

// TiRtcSendAudioStream maps to tiRTC.h TiRtcSendAudioStream.
func TiRtcSendAudioStream(hconn ConnHandle, info FrameInfo, frame []byte) error {
	cInfo, ptr, cleanup := frameArgs(info, frame)
	defer cleanup()
	return errorFromSendResult(int(C.TiRtcSendAudioStream(C.tirtc_conn_t(hconn), &cInfo, ptr)))
}

func frameArgs(info FrameInfo, frame []byte) (C.TIRTCFRAMEINFO, unsafe.Pointer, func()) {
	cInfo := C.TIRTCFRAMEINFO{
		stream_id: C.uint8_t(info.StreamID),
		media:     C.uint8_t(info.Media),
		flags:     C.uint8_t(info.Flags),
		ts:        C.uint32_t(info.Ts),
		length:    C.uint32_t(len(frame)),
	}
	var ptr unsafe.Pointer
	if len(frame) > 0 {
		ptr = C.CBytes(frame)
		return cInfo, ptr, func() { C.free(ptr) }
	}
	return cInfo, nil, func() {}
}

// TiRtcSendCommand maps to tiRTC.h TiRtcSendCommand.
func TiRtcSendCommand(hconn ConnHandle, cmd uint32, data []byte) error {
	var ptr unsafe.Pointer
	if len(data) > 0 {
		ptr = C.CBytes(data)
		defer C.free(ptr)
	}
	return errorFromSendResult(int(C.TiRtcSendCommand(
		C.tirtc_conn_t(hconn),
		C.uint32_t(cmd),
		ptr,
		C.uint32_t(len(data)),
	)))
}

// TiRtcGetSendBufferUsed maps to tiRTC.h TiRtcGetSendBufferUsed.
func TiRtcGetSendBufferUsed(hconn ConnHandle) uint64 {
	return uint64(C.TiRtcGetSendBufferUsed(C.tirtc_conn_t(hconn)))
}

// TiRtcRequestKeyFrame maps to tiRTC.h TiRtcRequestKeyFrame.
func TiRtcRequestKeyFrame(hconn ConnHandle, streamID uint8) error {
	return ErrorFromCode(int(C.TiRtcRequestKeyFrame(C.tirtc_conn_t(hconn), C.uint8_t(streamID))))
}

// TiRtcSubscribeVideo maps to tiRTC.h TiRtcSubscribeVideo.
func TiRtcSubscribeVideo(hconn ConnHandle, streamID uint8) error {
	return ErrorFromCode(int(C.TiRtcSubscribeVideo(C.tirtc_conn_t(hconn), C.uint8_t(streamID))))
}

// TiRtcUnsubscribeVideo maps to tiRTC.h TiRtcUnsubscribeVideo.
func TiRtcUnsubscribeVideo(hconn ConnHandle, streamID uint8) error {
	return ErrorFromCode(int(C.TiRtcUnsubscribeVideo(C.tirtc_conn_t(hconn), C.uint8_t(streamID))))
}

// TiRtcSubscribeAudio maps to tiRTC.h TiRtcSubscribeAudio.
func TiRtcSubscribeAudio(hconn ConnHandle, streamID uint8) error {
	return ErrorFromCode(int(C.TiRtcSubscribeAudio(C.tirtc_conn_t(hconn), C.uint8_t(streamID))))
}

// TiRtcUnsubscribeAudio maps to tiRTC.h TiRtcUnsubscribeAudio.
func TiRtcUnsubscribeAudio(hconn ConnHandle, streamID uint8) error {
	return ErrorFromCode(int(C.TiRtcUnsubscribeAudio(C.tirtc_conn_t(hconn), C.uint8_t(streamID))))
}

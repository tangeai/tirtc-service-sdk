//go:build tirtc_clib

package tirtcclib

/*
#include <stdint.h>
#include <stdlib.h>
#include "tiRTC.h"
*/
import "C"

import "unsafe"

func currentCallbacks() *Callbacks {
	callbackMu.RLock()
	defer callbackMu.RUnlock()
	return callbacks
}

func cFrameInfoToGo(pFi unsafe.Pointer) FrameInfo {
	fi := (*C.TIRTCFRAMEINFO)(pFi)
	return FrameInfo{
		StreamID: uint8(fi.stream_id),
		Media:    MediaType(fi.media),
		Flags:    uint8(fi.flags),
		Ts:       uint32(fi.ts),
		Length:   uint32(fi.length),
	}
}

//export goRtcOnEvent
func goRtcOnEvent(event C.int, data unsafe.Pointer, length C.int) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnEvent != nil {
		cbs.OnEvent(int(event), C.GoBytes(data, length))
	}
}

//export goRtcOnConnAccepted
func goRtcOnConnAccepted(hconn unsafe.Pointer) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnConnAccepted != nil {
		cbs.OnConnAccepted(ConnHandle(hconn))
	}
}

//export goRtcOnConnError
func goRtcOnConnError(hconn unsafe.Pointer, errorCode C.int) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnConnError != nil {
		cbs.OnConnError(ConnHandle(hconn), int(errorCode))
	}
}

//export goRtcOnDisconnected
func goRtcOnDisconnected(hconn unsafe.Pointer) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnDisconnected != nil {
		cbs.OnDisconnected(ConnHandle(hconn))
	}
}

//export goRtcOnAudio
func goRtcOnAudio(hconn unsafe.Pointer, pFi unsafe.Pointer, data unsafe.Pointer) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnAudio != nil {
		info := cFrameInfoToGo(pFi)
		cbs.OnAudio(ConnHandle(hconn), info, C.GoBytes(data, C.int(info.Length)))
	}
}

//export goRtcOnVideo
func goRtcOnVideo(hconn unsafe.Pointer, pFi unsafe.Pointer, data unsafe.Pointer) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnVideo != nil {
		info := cFrameInfoToGo(pFi)
		cbs.OnVideo(ConnHandle(hconn), info, C.GoBytes(data, C.int(info.Length)))
	}
}

//export goRtcOnMessage
func goRtcOnMessage(hconn unsafe.Pointer, pFi unsafe.Pointer, data unsafe.Pointer) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnMessage != nil {
		info := cFrameInfoToGo(pFi)
		cbs.OnMessage(ConnHandle(hconn), info, C.GoBytes(data, C.int(info.Length)))
	}
}

//export goRtcOnCommand
func goRtcOnCommand(hconn unsafe.Pointer, cmd C.uint32_t, data unsafe.Pointer, length C.uint32_t) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnCommand != nil {
		cbs.OnCommand(ConnHandle(hconn), uint32(cmd), C.GoBytes(data, C.int(length)))
	}
}

//export goRtcOnRequestKeyFrame
func goRtcOnRequestKeyFrame(hconn unsafe.Pointer, streamID C.uint8_t) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnRequestKeyFrame != nil {
		cbs.OnRequestKeyFrame(ConnHandle(hconn), uint8(streamID))
	}
}

//export goRtcOnSubscribeVideo
func goRtcOnSubscribeVideo(hconn unsafe.Pointer, streamID C.uint8_t) C.int {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnSubscribeVideo != nil {
		return C.int(cbs.OnSubscribeVideo(ConnHandle(hconn), uint8(streamID)))
	}
	return 0
}

//export goRtcOnUnsubscribeVideo
func goRtcOnUnsubscribeVideo(hconn unsafe.Pointer, streamID C.uint8_t) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnUnsubscribeVideo != nil {
		cbs.OnUnsubscribeVideo(ConnHandle(hconn), uint8(streamID))
	}
}

//export goRtcOnSubscribeAudio
func goRtcOnSubscribeAudio(hconn unsafe.Pointer, streamID C.uint8_t) C.int {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnSubscribeAudio != nil {
		return C.int(cbs.OnSubscribeAudio(ConnHandle(hconn), uint8(streamID)))
	}
	return 0
}

//export goRtcOnUnsubscribeAudio
func goRtcOnUnsubscribeAudio(hconn unsafe.Pointer, streamID C.uint8_t) {
	if cbs := currentCallbacks(); cbs != nil && cbs.OnUnsubscribeAudio != nil {
		cbs.OnUnsubscribeAudio(ConnHandle(hconn), uint8(streamID))
	}
}

//export goWhipAcceptOnSDP
func goWhipAcceptOnSDP(sdp *C.char, length C.int, ctx unsafe.Pointer) {
	whipRegistryMu.Lock()
	entry, ok := whipRegistry[ctx]
	whipRegistryMu.Unlock()
	if ok && entry.onSDP != nil {
		entry.onSDP(C.GoBytes(unsafe.Pointer(sdp), length))
	}
}

//export goWhipAcceptOnConnect
func goWhipAcceptOnConnect(errorCode C.int, hconn unsafe.Pointer, ctx unsafe.Pointer) {
	whipRegistryMu.Lock()
	entry, ok := whipRegistry[ctx]
	delete(whipRegistry, ctx)
	whipRegistryMu.Unlock()
	if ok {
		C.free(ctx)
		if entry.cb != nil {
			entry.cb(int(errorCode), ConnHandle(hconn))
		}
	}
}

//export goWhipConnectOnConnect
func goWhipConnectOnConnect(errorCode C.int, hconn unsafe.Pointer, ctx unsafe.Pointer) {
	whipRegistryMu.Lock()
	cb, ok := connectRegistry[ctx]
	delete(connectRegistry, ctx)
	whipRegistryMu.Unlock()
	if ok {
		C.free(ctx)
		if cb != nil {
			cb(int(errorCode), ConnHandle(hconn))
		}
	}
}

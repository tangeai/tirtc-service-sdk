//go:build !tirtc_clib

package tirtcclib

// TiRtcGetVersion maps to tiRTC.h TiRtcGetVersion.
func TiRtcGetVersion() string {
	return ""
}

// TiRtcInit maps to tiRTC.h TiRtcInit.
func TiRtcInit() error {
	return ErrUnavailable
}

// TiRtcUninit maps to tiRTC.h TiRtcUninit.
func TiRtcUninit() {}

// TiRtcStart maps to tiRTC.h TiRtcStart.
func TiRtcStart(deviceID string, callbacks *Callbacks) error {
	return ErrUnavailable
}

// TiRtcStop maps to tiRTC.h TiRtcStop.
func TiRtcStop() {}

// TiRtcSetOption maps to tiRTC.h TiRtcSetOption.
func TiRtcSetOption(option Option, value []byte) error {
	return ErrUnavailable
}

// TiRtcWhipAccept maps to tiRTC.h TiRtcWhipAccept.
func TiRtcWhipAccept(offer []byte, candidate string, onSDP WhipServerOnSDPReadyCallback, cb ConnectCallback) error {
	return ErrUnavailable
}

// TiRtcWhipConnect maps to tiRTC.h TiRtcWhipConnect.
func TiRtcWhipConnect(serviceDesc, token string, cb ConnectCallback) error {
	return ErrUnavailable
}

// TiRtcDisconnect maps to tiRTC.h TiRtcDisconnect.
func TiRtcDisconnect(hconn ConnHandle) error {
	return ErrUnavailable
}

// TiRtcSendMessageStream maps to tiRTC.h TiRtcSendMessageStream.
func TiRtcSendMessageStream(hconn ConnHandle, info FrameInfo, frame []byte) error {
	return ErrUnavailable
}

// TiRtcSendVideoStream maps to tiRTC.h TiRtcSendVideoStream.
func TiRtcSendVideoStream(hconn ConnHandle, info FrameInfo, frame []byte) error {
	return ErrUnavailable
}

// TiRtcSendAudioStream maps to tiRTC.h TiRtcSendAudioStream.
func TiRtcSendAudioStream(hconn ConnHandle, info FrameInfo, frame []byte) error {
	return ErrUnavailable
}

// TiRtcSendCommand maps to tiRTC.h TiRtcSendCommand.
func TiRtcSendCommand(hconn ConnHandle, cmd uint32, data []byte) error {
	return ErrUnavailable
}

// TiRtcGetSendBufferUsed maps to tiRTC.h TiRtcGetSendBufferUsed.
func TiRtcGetSendBufferUsed(hconn ConnHandle) uint64 {
	return 0
}

// TiRtcRequestKeyFrame maps to tiRTC.h TiRtcRequestKeyFrame.
func TiRtcRequestKeyFrame(hconn ConnHandle, streamID uint8) error {
	return ErrUnavailable
}

// TiRtcSubscribeVideo maps to tiRTC.h TiRtcSubscribeVideo.
func TiRtcSubscribeVideo(hconn ConnHandle, streamID uint8) error {
	return ErrUnavailable
}

// TiRtcUnsubscribeVideo maps to tiRTC.h TiRtcUnsubscribeVideo.
func TiRtcUnsubscribeVideo(hconn ConnHandle, streamID uint8) error {
	return ErrUnavailable
}

// TiRtcSubscribeAudio maps to tiRTC.h TiRtcSubscribeAudio.
func TiRtcSubscribeAudio(hconn ConnHandle, streamID uint8) error {
	return ErrUnavailable
}

// TiRtcUnsubscribeAudio maps to tiRTC.h TiRtcUnsubscribeAudio.
func TiRtcUnsubscribeAudio(hconn ConnHandle, streamID uint8) error {
	return ErrUnavailable
}

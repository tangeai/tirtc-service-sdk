package tirtcclib

import "unsafe"

// ConnHandle is the Go representation of tiRTC.h tirtc_conn_t.
type ConnHandle unsafe.Pointer

// Option maps to tiRTC.h TIRTCOPTION.
type Option int

const (
	TiRtcOptServiceEndpoint  Option = 1
	TiRtcOptDeviceSecretKey  Option = 2
	TiRtcOptMaxConnections   Option = 3
	TiRtcOptNetworkType      Option = 4
	TiRtcOptICCID            Option = 5
	TiRtcOptWakeup           Option = 6
	TiRtcOptRestrictedNet    Option = 7
	TiRtcOptMaxSendBuffer    Option = 8
	TiRtcOptConnectCache     Option = 9
	TiRtcOptAppID            Option = 10
	TiRtcOptClientID         Option = 11
	TiRtcOptTGTRPPollTimeout Option = 12
	TiRtcOptSDKTaskPriority  Option = 13
)

// SystemEvent maps to tiRTC.h TIRTCSYSEVENT.
type SystemEvent int

const (
	TiRtcEventSystemStarted SystemEvent = iota
	TiRtcEventSystemStopped
	TiRtcEventAccessHijacking
)

// MediaType maps to tiRTC.h TIRTCMEDIA.
type MediaType uint8

const (
	TiRtcMediaMessage MediaType = 0
	TiRtcAudioPCM     MediaType = 1
	TiRtcAudioALaw    MediaType = 2
	TiRtcAudioAAC     MediaType = 3
	TiRtcAudioOpus    MediaType = 4
	TiRtcAudioAMR     MediaType = 5
	TiRtcVideoJPEG    MediaType = 65
	TiRtcVideoH264    MediaType = 66
	TiRtcVideoH265    MediaType = 67
)

// FrameInfo maps to tiRTC.h TIRTCFRAMEINFO.
type FrameInfo struct {
	StreamID uint8
	Media    MediaType
	Flags    uint8
	Ts       uint32
	Length   uint32
}

// Callbacks maps to tiRTC.h TIRTCCALLBACKS.
type Callbacks struct {
	OnEvent            func(event int, data []byte)
	OnConnAccepted     func(hconn ConnHandle)
	OnConnError        func(hconn ConnHandle, errorCode int)
	OnDisconnected     func(hconn ConnHandle)
	OnAudio            func(hconn ConnHandle, info FrameInfo, data []byte)
	OnVideo            func(hconn ConnHandle, info FrameInfo, data []byte)
	OnMessage          func(hconn ConnHandle, info FrameInfo, data []byte)
	OnCommand          func(hconn ConnHandle, cmd uint32, data []byte)
	OnRequestKeyFrame  func(hconn ConnHandle, streamID uint8)
	OnSubscribeVideo   func(hconn ConnHandle, streamID uint8) int
	OnUnsubscribeVideo func(hconn ConnHandle, streamID uint8)
	OnSubscribeAudio   func(hconn ConnHandle, streamID uint8) int
	OnUnsubscribeAudio func(hconn ConnHandle, streamID uint8)
}

// ConnectCallback maps to tiRTC.h TIRTCCONNECTCALLBACK.
type ConnectCallback func(errorCode int, hconn ConnHandle)

// WhipServerOnSDPReadyCallback maps to tiRTC.h TIRTCWHIPSERVERONSDPREADYCB.
type WhipServerOnSDPReadyCallback func(sdp []byte)

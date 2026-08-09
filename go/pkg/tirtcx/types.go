package tirtcx

import "gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"

type MediaType = tirtcclib.MediaType
type FrameInfo = tirtcclib.FrameInfo

const (
	MediaMessage = tirtcclib.TiRtcMediaMessage
	AudioPCM     = tirtcclib.TiRtcAudioPCM
	AudioALaw    = tirtcclib.TiRtcAudioALaw
	AudioAAC     = tirtcclib.TiRtcAudioAAC
	AudioOpus    = tirtcclib.TiRtcAudioOpus
	AudioAMR     = tirtcclib.TiRtcAudioAMR
	VideoJPEG    = tirtcclib.TiRtcVideoJPEG
	VideoH264    = tirtcclib.TiRtcVideoH264
	VideoH265    = tirtcclib.TiRtcVideoH265
)

type AudioFrame struct {
	Info FrameInfo
	Data []byte
}

type VideoFrame struct {
	Info FrameInfo
	Data []byte
}

type MessageFrame struct {
	Info FrameInfo
	Data []byte
}

type Command struct {
	Cmd  uint32
	Data []byte
}

type EventStats struct {
	AudioDropped            uint64
	VideoDropped            uint64
	MessageDropped          uint64
	CommandDropped          uint64
	ErrorDropped            uint64
	DisconnectedDropped     uint64
	RequestKeyFrameDropped  uint64
	UnsubscribeVideoDropped uint64
	UnsubscribeAudioDropped uint64
}

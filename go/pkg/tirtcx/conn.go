package tirtcx

import (
	"sync"
	"sync/atomic"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcclib"
)

// MinEventBufferSize is the minimum buffer capacity for enabled Conn event channels.
const MinEventBufferSize = 4

// ConnEventOptions configures optional per-connection TiRTC event channels.
// A buffer size <= 0 disables that event channel.
type ConnEventOptions struct {
	AudioBuffer            int
	VideoBuffer            int
	MessageBuffer          int
	CommandBuffer          int
	ErrorBuffer            int
	DisconnectedBuffer     int
	RequestKeyFrameBuffer  int
	SubscribeVideoBuffer   int
	UnsubscribeVideoBuffer int
	SubscribeAudioBuffer   int
	UnsubscribeAudioBuffer int
}

// ConnEvents exposes read-only event channels owned by Conn.
type ConnEvents struct {
	Audio            <-chan AudioFrame
	Video            <-chan VideoFrame
	Message          <-chan MessageFrame
	Command          <-chan Command
	Error            <-chan error
	Disconnected     <-chan struct{}
	RequestKeyFrame  <-chan uint8
	SubscribeVideo   <-chan SubscribeEvent
	UnsubscribeVideo <-chan uint8
	SubscribeAudio   <-chan SubscribeEvent
	UnsubscribeAudio <-chan uint8
}

// Conn wraps a TiRTC connection handle and exposes connection operations.
type Conn struct {
	handleMu sync.RWMutex
	handle   tirtcclib.ConnHandle

	candidate      string
	closeRequested atomic.Bool

	audioCh            chan AudioFrame
	videoCh            chan VideoFrame
	messageCh          chan MessageFrame
	commandCh          chan Command
	errorCh            chan error
	disconnectedCh     chan struct{}
	requestKeyFrameCh  chan uint8
	subscribeVideoCh   chan SubscribeEvent
	unsubscribeVideoCh chan uint8
	subscribeAudioCh   chan SubscribeEvent
	unsubscribeAudioCh chan uint8

	eventStats      eventCounters
	closeEventsOnce sync.Once
}

type eventCounters struct {
	audioDropped            atomic.Uint64
	videoDropped            atomic.Uint64
	messageDropped          atomic.Uint64
	commandDropped          atomic.Uint64
	errorDropped            atomic.Uint64
	disconnectedDropped     atomic.Uint64
	requestKeyFrameDropped  atomic.Uint64
	subscribeVideoDropped   atomic.Uint64
	unsubscribeVideoDropped atomic.Uint64
	subscribeAudioDropped   atomic.Uint64
	unsubscribeAudioDropped atomic.Uint64
}

func newConn(candidate string, options ConnEventOptions) *Conn {
	return &Conn{
		candidate:          candidate,
		audioCh:            makeEventChan[AudioFrame](options.AudioBuffer),
		videoCh:            makeEventChan[VideoFrame](options.VideoBuffer),
		messageCh:          makeEventChan[MessageFrame](options.MessageBuffer),
		commandCh:          makeEventChan[Command](options.CommandBuffer),
		errorCh:            makeEventChan[error](options.ErrorBuffer),
		disconnectedCh:     makeEventChan[struct{}](options.DisconnectedBuffer),
		requestKeyFrameCh:  makeEventChan[uint8](options.RequestKeyFrameBuffer),
		subscribeVideoCh:   makeEventChan[SubscribeEvent](options.SubscribeVideoBuffer),
		unsubscribeVideoCh: makeEventChan[uint8](options.UnsubscribeVideoBuffer),
		subscribeAudioCh:   makeEventChan[SubscribeEvent](options.SubscribeAudioBuffer),
		unsubscribeAudioCh: makeEventChan[uint8](options.UnsubscribeAudioBuffer),
	}
}

func makeEventChan[T any](buffer int) chan T {
	if buffer <= 0 {
		return nil
	}
	if buffer < MinEventBufferSize {
		buffer = MinEventBufferSize
	}
	return make(chan T, buffer)
}

// Handle returns the underlying clib connection handle.
func (c *Conn) Handle() tirtcclib.ConnHandle {
	c.handleMu.RLock()
	defer c.handleMu.RUnlock()
	return c.handle
}

func (c *Conn) bindHandle(handle tirtcclib.ConnHandle) {
	c.handleMu.Lock()
	c.handle = handle
	c.handleMu.Unlock()
	if c.closeRequested.Load() {
		go func() {
			_ = tirtcclib.TiRtcDisconnect(handle)
		}()
	}
}

func (c *Conn) EventStats() EventStats {
	if c == nil {
		return EventStats{}
	}
	return EventStats{
		AudioDropped:            c.eventStats.audioDropped.Load(),
		VideoDropped:            c.eventStats.videoDropped.Load(),
		MessageDropped:          c.eventStats.messageDropped.Load(),
		CommandDropped:          c.eventStats.commandDropped.Load(),
		ErrorDropped:            c.eventStats.errorDropped.Load(),
		DisconnectedDropped:     c.eventStats.disconnectedDropped.Load(),
		RequestKeyFrameDropped:  c.eventStats.requestKeyFrameDropped.Load(),
		SubscribeVideoDropped:   c.eventStats.subscribeVideoDropped.Load(),
		UnsubscribeVideoDropped: c.eventStats.unsubscribeVideoDropped.Load(),
		SubscribeAudioDropped:   c.eventStats.subscribeAudioDropped.Load(),
		UnsubscribeAudioDropped: c.eventStats.unsubscribeAudioDropped.Load(),
	}
}

func (c *Conn) Events() ConnEvents {
	if c == nil {
		return ConnEvents{}
	}
	return ConnEvents{
		Audio:            c.audioCh,
		Video:            c.videoCh,
		Message:          c.messageCh,
		Command:          c.commandCh,
		Error:            c.errorCh,
		Disconnected:     c.disconnectedCh,
		RequestKeyFrame:  c.requestKeyFrameCh,
		SubscribeVideo:   c.subscribeVideoCh,
		UnsubscribeVideo: c.unsubscribeVideoCh,
		SubscribeAudio:   c.subscribeAudioCh,
		UnsubscribeAudio: c.unsubscribeAudioCh,
	}
}

func (c *Conn) closeEventChannels() {
	if c == nil {
		return
	}
	c.closeEventsOnce.Do(func() {
		closeEventChan(c.audioCh)
		closeEventChan(c.videoCh)
		closeEventChan(c.messageCh)
		closeEventChan(c.commandCh)
		closeEventChan(c.errorCh)
		closeEventChan(c.disconnectedCh)
		closeEventChan(c.requestKeyFrameCh)
		closeEventChan(c.subscribeVideoCh)
		closeEventChan(c.unsubscribeVideoCh)
		closeEventChan(c.subscribeAudioCh)
		closeEventChan(c.unsubscribeAudioCh)
	})
}

func closeEventChan[T any](ch chan T) {
	if ch != nil {
		close(ch)
	}
}

func (c *Conn) SendMessage(info FrameInfo, data []byte) error {
	return tirtcclib.TiRtcSendMessageStream(c.Handle(), info, data)
}

func (c *Conn) SendAudio(info FrameInfo, data []byte) error {
	return tirtcclib.TiRtcSendAudioStream(c.Handle(), info, data)
}

func (c *Conn) SendVideo(info FrameInfo, data []byte) error {
	return tirtcclib.TiRtcSendVideoStream(c.Handle(), info, data)
}

func (c *Conn) SendCommand(cmd uint32, data []byte) error {
	return tirtcclib.TiRtcSendCommand(c.Handle(), cmd, data)
}

func (c *Conn) RequestKeyFrame(streamID uint8) error {
	return tirtcclib.TiRtcRequestKeyFrame(c.Handle(), streamID)
}

func (c *Conn) SubscribeVideo(streamID uint8) error {
	return tirtcclib.TiRtcSubscribeVideo(c.Handle(), streamID)
}

func (c *Conn) UnsubscribeVideo(streamID uint8) error {
	return tirtcclib.TiRtcUnsubscribeVideo(c.Handle(), streamID)
}

func (c *Conn) SubscribeAudio(streamID uint8) error {
	return tirtcclib.TiRtcSubscribeAudio(c.Handle(), streamID)
}

func (c *Conn) UnsubscribeAudio(streamID uint8) error {
	return tirtcclib.TiRtcUnsubscribeAudio(c.Handle(), streamID)
}

func (c *Conn) SendBufferUsed() uint64 {
	return tirtcclib.TiRtcGetSendBufferUsed(c.Handle())
}

func (c *Conn) Close() error {
	handle := c.Handle()
	if handle == nil {
		c.closeRequested.Store(true)
		return nil
	}
	return tirtcclib.TiRtcDisconnect(handle)
}

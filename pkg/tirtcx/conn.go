package tirtcx

import (
	"sync"
	"sync/atomic"

	"github.com/tangeai/tirtc-service-sdk/v2/internal/tirtcclib"
)

var disconnectConnFunc = tirtcclib.TiRtcDisconnect

// MinEventBufferSize is the minimum buffer capacity for enabled Conn event channels.
const MinEventBufferSize = 4

// ConnEventOptions configures optional per-connection TiRTC event channels and
// synchronous subscription decisions. A buffer size <= 0 disables that event
// channel. Subscription handlers run on a native callback and must return
// quickly; a nil handler or panic returns the default result 0.
type ConnEventOptions struct {
	AudioBuffer            int
	VideoBuffer            int
	MessageBuffer          int
	CommandBuffer          int
	ErrorBuffer            int
	DisconnectedBuffer     int
	RequestKeyFrameBuffer  int
	UnsubscribeVideoBuffer int
	UnsubscribeAudioBuffer int
	SubscribeVideoHandler  func(streamID uint8) int
	SubscribeAudioHandler  func(streamID uint8) int
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
	UnsubscribeVideo <-chan uint8
	UnsubscribeAudio <-chan uint8
}

// Conn wraps a TiRTC connection handle and exposes connection operations.
type Conn struct {
	handleMu sync.RWMutex
	handle   tirtcclib.ConnHandle

	candidate      string
	closeRequested atomic.Bool
	disconnectOnce sync.Once
	disconnectErr  error
	doneCh         chan struct{}
	terminalOnce   sync.Once
	terminalMu     sync.RWMutex
	terminalErr    error

	audioCh            chan AudioFrame
	videoCh            chan VideoFrame
	messageCh          chan MessageFrame
	commandCh          chan Command
	errorCh            chan error
	disconnectedCh     chan struct{}
	requestKeyFrameCh  chan uint8
	unsubscribeVideoCh chan uint8
	unsubscribeAudioCh chan uint8
	subscribeVideo     func(uint8) int
	subscribeAudio     func(uint8) int

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
	unsubscribeVideoDropped atomic.Uint64
	unsubscribeAudioDropped atomic.Uint64
}

func newConn(candidate string, options ConnEventOptions) *Conn {
	return &Conn{
		candidate:          candidate,
		doneCh:             make(chan struct{}),
		audioCh:            makeEventChan[AudioFrame](options.AudioBuffer),
		videoCh:            makeEventChan[VideoFrame](options.VideoBuffer),
		messageCh:          makeEventChan[MessageFrame](options.MessageBuffer),
		commandCh:          makeEventChan[Command](options.CommandBuffer),
		errorCh:            makeEventChan[error](options.ErrorBuffer),
		disconnectedCh:     makeEventChan[struct{}](options.DisconnectedBuffer),
		requestKeyFrameCh:  makeEventChan[uint8](options.RequestKeyFrameBuffer),
		unsubscribeVideoCh: makeEventChan[uint8](options.UnsubscribeVideoBuffer),
		unsubscribeAudioCh: makeEventChan[uint8](options.UnsubscribeAudioBuffer),
		subscribeVideo:     options.SubscribeVideoHandler,
		subscribeAudio:     options.SubscribeAudioHandler,
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
	closeRequested := c.closeRequested.Load()
	c.handleMu.Unlock()
	if closeRequested {
		go func() {
			_ = c.disconnect(handle)
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
		UnsubscribeVideoDropped: c.eventStats.unsubscribeVideoDropped.Load(),
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
		UnsubscribeVideo: c.unsubscribeVideoCh,
		UnsubscribeAudio: c.unsubscribeAudioCh,
	}
}

// Done is closed when the connection reaches a terminal error or disconnects.
func (c *Conn) Done() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.doneCh
}

// Err returns the terminal connection error, or nil after a normal disconnect.
// Callers should read Err after Done is closed.
func (c *Conn) Err() error {
	if c == nil {
		return ErrNilConn
	}
	c.terminalMu.RLock()
	defer c.terminalMu.RUnlock()
	return c.terminalErr
}

func (c *Conn) finish(err error) {
	if c == nil {
		return
	}
	c.terminalOnce.Do(func() {
		c.terminalMu.Lock()
		c.terminalErr = err
		c.terminalMu.Unlock()
		close(c.doneCh)
	})
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
		closeEventChan(c.unsubscribeVideoCh)
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
	if c == nil {
		return nil
	}
	c.handleMu.Lock()
	c.closeRequested.Store(true)
	handle := c.handle
	c.handleMu.Unlock()
	if handle == nil {
		return nil
	}
	return c.disconnect(handle)
}

func (c *Conn) disconnect(handle tirtcclib.ConnHandle) error {
	c.disconnectOnce.Do(func() {
		c.disconnectErr = disconnectConnFunc(handle)
	})
	return c.disconnectErr
}

package whipecho

import (
	"sync/atomic"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

// CloseReason describes why an Echo session ended.
type CloseReason string

const (
	CloseReasonDeleted          CloseReason = "deleted"
	CloseReasonDisconnected     CloseReason = "disconnected"
	CloseReasonConnectionError  CloseReason = "connection_error"
	CloseReasonConnectionFailed CloseReason = "connection_failed"
	CloseReasonShutdown         CloseReason = "shutdown"
)

// AcceptStage identifies a synchronous session acceptance stage.
type AcceptStage string

const (
	AcceptStageValidate  AcceptStage = "validate"
	AcceptStageWHIP      AcceptStage = "whip_accept"
	AcceptStageSessionID AcceptStage = "session_id"
)

// ConnectFailureReason describes an asynchronous connection failure.
type ConnectFailureReason string

const (
	ConnectFailureTimeout ConnectFailureReason = "timeout"
	ConnectFailureError   ConnectFailureReason = "error"
)

// MediaType identifies an Echo media category.
type MediaType string

const (
	MediaTypeAudio   MediaType = "audio"
	MediaTypeVideo   MediaType = "video"
	MediaTypeCommand MediaType = "command"
)

// EventType identifies a connection event category.
type EventType string

const (
	EventTypeAudio            EventType = "audio"
	EventTypeVideo            EventType = "video"
	EventTypeMessage          EventType = "message"
	EventTypeCommand          EventType = "command"
	EventTypeError            EventType = "error"
	EventTypeDisconnected     EventType = "disconnected"
	EventTypeRequestKeyFrame  EventType = "request_key_frame"
	EventTypeUnsubscribeVideo EventType = "unsubscribe_video"
	EventTypeUnsubscribeAudio EventType = "unsubscribe_audio"
)

// Observer receives bounded-cardinality Echo lifecycle events. Implementations
// must be concurrency-safe and should return quickly.
type Observer interface {
	SessionAccepted()
	SessionConnected(time.Duration)
	SessionClosed(CloseReason, time.Duration)
	AcceptFailed(AcceptStage)
	ConnectFailed(ConnectFailureReason)
	AudioEchoed(bytes int)
	VideoEchoed(bytes int)
	SendFailed(MediaType)
	EventsDropped(EventType, uint64)
}

// ObserverFuncs lets callers observe only the events they need.
type ObserverFuncs struct {
	OnSessionAccepted  func()
	OnSessionConnected func(time.Duration)
	OnSessionClosed    func(CloseReason, time.Duration)
	OnAcceptFailed     func(AcceptStage)
	OnConnectFailed    func(ConnectFailureReason)
	OnAudioEchoed      func(bytes int)
	OnVideoEchoed      func(bytes int)
	OnSendFailed       func(MediaType)
	OnEventsDropped    func(EventType, uint64)
}

func (o ObserverFuncs) SessionAccepted() {
	if o.OnSessionAccepted != nil {
		o.OnSessionAccepted()
	}
}
func (o ObserverFuncs) SessionConnected(duration time.Duration) {
	if o.OnSessionConnected != nil {
		o.OnSessionConnected(duration)
	}
}
func (o ObserverFuncs) SessionClosed(reason CloseReason, duration time.Duration) {
	if o.OnSessionClosed != nil {
		o.OnSessionClosed(reason, duration)
	}
}
func (o ObserverFuncs) AcceptFailed(stage AcceptStage) {
	if o.OnAcceptFailed != nil {
		o.OnAcceptFailed(stage)
	}
}
func (o ObserverFuncs) ConnectFailed(reason ConnectFailureReason) {
	if o.OnConnectFailed != nil {
		o.OnConnectFailed(reason)
	}
}
func (o ObserverFuncs) AudioEchoed(bytes int) {
	if o.OnAudioEchoed != nil {
		o.OnAudioEchoed(bytes)
	}
}
func (o ObserverFuncs) VideoEchoed(bytes int) {
	if o.OnVideoEchoed != nil {
		o.OnVideoEchoed(bytes)
	}
}
func (o ObserverFuncs) SendFailed(media MediaType) {
	if o.OnSendFailed != nil {
		o.OnSendFailed(media)
	}
}
func (o ObserverFuncs) EventsDropped(event EventType, count uint64) {
	if o.OnEventsDropped != nil {
		o.OnEventsDropped(event, count)
	}
}

type noopObserver struct{}

func (noopObserver) SessionAccepted()                         {}
func (noopObserver) SessionConnected(time.Duration)           {}
func (noopObserver) SessionClosed(CloseReason, time.Duration) {}
func (noopObserver) AcceptFailed(AcceptStage)                 {}
func (noopObserver) ConnectFailed(ConnectFailureReason)       {}
func (noopObserver) AudioEchoed(int)                          {}
func (noopObserver) VideoEchoed(int)                          {}
func (noopObserver) SendFailed(MediaType)                     {}
func (noopObserver) EventsDropped(EventType, uint64)          {}

// Stats is a point-in-time, concurrency-safe snapshot. Counters are monotonic
// for the lifetime of a SessionManager; ActiveSessions is a gauge.
type Stats struct {
	ActiveSessions      uint64
	AcceptedTotal       uint64
	AcceptFailedTotal   uint64
	ConnectFailedTotal  uint64
	ConnectTimeoutTotal uint64
	DeletedTotal        uint64

	AudioFramesTotal uint64
	AudioBytesTotal  uint64
	VideoFramesTotal uint64
	VideoBytesTotal  uint64

	SendFailedTotal    uint64
	DroppedEventsTotal uint64
	ClosedTotal        map[CloseReason]uint64
}

type statsRecorder struct {
	activeSessions      atomic.Uint64
	acceptedTotal       atomic.Uint64
	acceptFailedTotal   atomic.Uint64
	connectFailedTotal  atomic.Uint64
	connectTimeoutTotal atomic.Uint64
	deletedTotal        atomic.Uint64
	audioFramesTotal    atomic.Uint64
	audioBytesTotal     atomic.Uint64
	videoFramesTotal    atomic.Uint64
	videoBytesTotal     atomic.Uint64
	sendFailedTotal     atomic.Uint64
	droppedEventsTotal  atomic.Uint64

	closedDeleted          atomic.Uint64
	closedDisconnected     atomic.Uint64
	closedConnectionError  atomic.Uint64
	closedConnectionFailed atomic.Uint64
	closedShutdown         atomic.Uint64
}

func (r *statsRecorder) snapshot() Stats {
	return Stats{
		ActiveSessions:      r.activeSessions.Load(),
		AcceptedTotal:       r.acceptedTotal.Load(),
		AcceptFailedTotal:   r.acceptFailedTotal.Load(),
		ConnectFailedTotal:  r.connectFailedTotal.Load(),
		ConnectTimeoutTotal: r.connectTimeoutTotal.Load(),
		DeletedTotal:        r.deletedTotal.Load(),
		AudioFramesTotal:    r.audioFramesTotal.Load(),
		AudioBytesTotal:     r.audioBytesTotal.Load(),
		VideoFramesTotal:    r.videoFramesTotal.Load(),
		VideoBytesTotal:     r.videoBytesTotal.Load(),
		SendFailedTotal:     r.sendFailedTotal.Load(),
		DroppedEventsTotal:  r.droppedEventsTotal.Load(),
		ClosedTotal: map[CloseReason]uint64{
			CloseReasonDeleted:          r.closedDeleted.Load(),
			CloseReasonDisconnected:     r.closedDisconnected.Load(),
			CloseReasonConnectionError:  r.closedConnectionError.Load(),
			CloseReasonConnectionFailed: r.closedConnectionFailed.Load(),
			CloseReasonShutdown:         r.closedShutdown.Load(),
		},
	}
}

func (r *statsRecorder) recordClose(reason CloseReason) {
	switch reason {
	case CloseReasonDeleted:
		r.closedDeleted.Add(1)
	case CloseReasonDisconnected:
		r.closedDisconnected.Add(1)
	case CloseReasonConnectionError:
		r.closedConnectionError.Add(1)
	case CloseReasonConnectionFailed:
		r.closedConnectionFailed.Add(1)
	case CloseReasonShutdown:
		r.closedShutdown.Add(1)
	}
}

func (r *statsRecorder) recordDropped(observer Observer, stats tirtcx.EventStats) {
	dropped := []struct {
		event EventType
		count uint64
	}{
		{EventTypeAudio, stats.AudioDropped},
		{EventTypeVideo, stats.VideoDropped},
		{EventTypeMessage, stats.MessageDropped},
		{EventTypeCommand, stats.CommandDropped},
		{EventTypeError, stats.ErrorDropped},
		{EventTypeDisconnected, stats.DisconnectedDropped},
		{EventTypeRequestKeyFrame, stats.RequestKeyFrameDropped},
		{EventTypeUnsubscribeVideo, stats.UnsubscribeVideoDropped},
		{EventTypeUnsubscribeAudio, stats.UnsubscribeAudioDropped},
	}
	for _, item := range dropped {
		if item.count == 0 {
			continue
		}
		r.droppedEventsTotal.Add(item.count)
		observer.EventsDropped(item.event, item.count)
	}
}

package whipecho

import (
	"sync"
	"testing"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

type recordingObserver struct {
	mu       sync.Mutex
	accepted int
	closed   []CloseReason
	dropped  map[EventType]uint64
}

func (o *recordingObserver) SessionAccepted() {
	o.mu.Lock()
	o.accepted++
	o.mu.Unlock()
}
func (*recordingObserver) SessionConnected(time.Duration) {}
func (o *recordingObserver) SessionClosed(reason CloseReason, _ time.Duration) {
	o.mu.Lock()
	o.closed = append(o.closed, reason)
	o.mu.Unlock()
}
func (*recordingObserver) AcceptFailed(AcceptStage)           {}
func (*recordingObserver) ConnectFailed(ConnectFailureReason) {}
func (*recordingObserver) AudioEchoed(int)                    {}
func (*recordingObserver) VideoEchoed(int)                    {}
func (*recordingObserver) SendFailed(MediaType)               {}
func (o *recordingObserver) EventsDropped(event EventType, count uint64) {
	o.mu.Lock()
	if o.dropped == nil {
		o.dropped = make(map[EventType]uint64)
	}
	o.dropped[event] += count
	o.mu.Unlock()
}

func TestStatsSnapshotIsIndependent(t *testing.T) {
	recorder := &statsRecorder{}
	recorder.activeSessions.Store(2)
	recorder.acceptedTotal.Store(3)
	recorder.recordClose(CloseReasonDeleted)
	first := recorder.snapshot()
	first.ClosedTotal[CloseReasonDeleted] = 99
	second := recorder.snapshot()
	if second.ActiveSessions != 2 || second.AcceptedTotal != 3 {
		t.Fatalf("snapshot = %+v", second)
	}
	if second.ClosedTotal[CloseReasonDeleted] != 1 {
		t.Fatalf("closed deleted = %d", second.ClosedTotal[CloseReasonDeleted])
	}
}

func TestRecordDroppedCountsAllEventTypes(t *testing.T) {
	recorder := &statsRecorder{}
	observer := &recordingObserver{}
	recorder.recordDropped(observer, tirtcx.EventStats{
		AudioDropped:            1,
		MessageDropped:          2,
		RequestKeyFrameDropped:  3,
		UnsubscribeAudioDropped: 4,
	})
	stats := recorder.snapshot()
	if stats.DroppedEventsTotal != 10 {
		t.Fatalf("DroppedEventsTotal = %d", stats.DroppedEventsTotal)
	}
	if observer.dropped[EventTypeAudio] != 1 ||
		observer.dropped[EventTypeMessage] != 2 ||
		observer.dropped[EventTypeRequestKeyFrame] != 3 ||
		observer.dropped[EventTypeUnsubscribeAudio] != 4 {
		t.Fatalf("observer dropped = %+v", observer.dropped)
	}
}

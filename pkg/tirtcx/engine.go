package tirtcx

import (
	"context"

	"github.com/tangeai/tirtc-service-sdk/v2/internal/tirtcclib"
)

var nativeStart = tirtcclib.TiRtcStart

// Init maps to TiRtcInit and registers SDK callbacks through Start.
func Init() error {
	return tirtcclib.TiRtcInit()
}

// SetNativeLogLevel configures native TiRTC logging. Levels above 10 also
// enable TGTRP logging at level-10 and should be set before Init.
func SetNativeLogLevel(level int) {
	tirtcclib.TiRtcLogSetLevel(level)
}

// Start starts TiRTC in WHIP service mode and waits until the native SDK
// reports TIRTC_EVENT_SYS_STARTED.
func Start(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	events := make(chan tirtcclib.SystemEvent, 1)
	cbs := callbacks(func(event int, _ []byte) {
		switch systemEvent := tirtcclib.SystemEvent(event); systemEvent {
		case tirtcclib.TiRtcEventSystemStarted, tirtcclib.TiRtcEventSystemStopped:
			select {
			case events <- systemEvent:
			default:
			}
		}
	})
	if err := nativeStart("", cbs); err != nil {
		return err
	}

	select {
	case event := <-events:
		if event == tirtcclib.TiRtcEventSystemStarted {
			return nil
		}
		return ErrStoppedBeforeStart
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop maps to TiRtcStop.
func Stop() {
	tirtcclib.TiRtcStop()
}

// Uninit maps to TiRtcUninit.
func Uninit() {
	tirtcclib.TiRtcUninit()
}

// GetVersion maps to TiRtcGetVersion.
func GetVersion() string {
	return tirtcclib.TiRtcGetVersion()
}

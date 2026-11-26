package tirtcx

import (
	"context"

	"github.com/tangeai/tirtc-service-sdk/v2/internal/tirtcclib"
)

var (
	nativeStart                           = tirtcclib.TiRtcStart
	nativeSetAudioRedundancyLossThreshold = tirtcclib.TiRtcSetAudioRedundancyLossThreshold
)

// Init maps to TiRtcInit and registers SDK callbacks through Start.
func Init() error {
	return tirtcclib.TiRtcInit()
}

// SetNativeLogLevel configures native TiRTC logging. Levels above 10 also
// enable TGTRP logging at level-10 and should be set before Init.
func SetNativeLogLevel(level int) {
	tirtcclib.TiRtcLogSetLevel(level)
}

// SetAudioRedundancyLossThreshold configures the global TGTRP packet-loss
// percentage above which redundant audio packets are sent. Values are clamped
// by the native SDK to 0..100; 0 always enables redundancy and 100 disables
// it. Call this before Init and before creating connections. It returns the
// threshold accepted by the native SDK, or -1 when the native library is
// unavailable or does not support the capability.
func SetAudioRedundancyLossThreshold(lossPercent int) int {
	return nativeSetAudioRedundancyLossThreshold(lossPercent)
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

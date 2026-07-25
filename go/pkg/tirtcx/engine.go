package tirtcx

import (
	"fmt"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

const DefaultStartTimeout = 15 * time.Second

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
func Start() error {
	return StartWithTimeout(DefaultStartTimeout)
}

// StartWithTimeout starts TiRTC and waits at most timeout for the asynchronous
// native startup result. TiRtcStart returning zero only means that its
// arguments passed initial validation.
func StartWithTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("%w: timeout must be greater than zero", ErrInvalidArgument)
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

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case event := <-events:
		if event == tirtcclib.TiRtcEventSystemStarted {
			return nil
		}
		return ErrStoppedBeforeStart
	case <-timer.C:
		return fmt.Errorf("%w after %s", ErrStartTimeout, timeout)
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

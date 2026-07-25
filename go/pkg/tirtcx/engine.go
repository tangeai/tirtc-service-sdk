package tirtcx

import (
	"fmt"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

var (
	nativeStart  = tirtcclib.TiRtcStart
	startTimeout = 15 * time.Second
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

// Start starts TiRTC in WHIP service mode and waits until the native SDK
// reports TIRTC_EVENT_SYS_STARTED.
func Start() error {
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

	timer := time.NewTimer(startTimeout)
	defer timer.Stop()
	select {
	case event := <-events:
		if event == tirtcclib.TiRtcEventSystemStarted {
			return nil
		}
		return ErrStoppedBeforeStart
	case <-timer.C:
		return fmt.Errorf("%w after %s", ErrStartTimeout, startTimeout)
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

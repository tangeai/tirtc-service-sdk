package tirtcx

import (
	"errors"
	"testing"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

func replaceNativeStart(
	t *testing.T,
	replacement func(string, *tirtcclib.Callbacks) error,
) {
	t.Helper()
	previous := nativeStart
	nativeStart = replacement
	t.Cleanup(func() {
		nativeStart = previous
	})
}

func TestStartWithTimeoutWaitsForStartedEvent(t *testing.T) {
	replaceNativeStart(t, func(deviceID string, callbacks *tirtcclib.Callbacks) error {
		if deviceID != "" {
			t.Fatalf("deviceID = %q, want empty WHIP service mode", deviceID)
		}
		if callbacks.OnEvent == nil {
			t.Fatal("OnEvent callback is nil")
		}
		go callbacks.OnEvent(int(tirtcclib.TiRtcEventSystemStarted), nil)
		return nil
	})

	if err := StartWithTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithTimeoutReturnsNativeStartError(t *testing.T) {
	want := errors.New("native start failed")
	replaceNativeStart(t, func(string, *tirtcclib.Callbacks) error {
		return want
	})

	if err := StartWithTimeout(time.Second); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestStartWithTimeoutTimesOut(t *testing.T) {
	replaceNativeStart(t, func(string, *tirtcclib.Callbacks) error {
		return nil
	})

	if err := StartWithTimeout(time.Millisecond); !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("error = %v, want ErrStartTimeout", err)
	}
}

func TestStartWithTimeoutRejectsStoppedEvent(t *testing.T) {
	replaceNativeStart(t, func(_ string, callbacks *tirtcclib.Callbacks) error {
		go callbacks.OnEvent(int(tirtcclib.TiRtcEventSystemStopped), nil)
		return nil
	})

	if err := StartWithTimeout(time.Second); !errors.Is(err, ErrStoppedBeforeStart) {
		t.Fatalf("error = %v, want ErrStoppedBeforeStart", err)
	}
}

func TestStartWithTimeoutRejectsInvalidTimeout(t *testing.T) {
	if err := StartWithTimeout(0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
}

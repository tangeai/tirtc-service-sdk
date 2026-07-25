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

func replaceStartTimeout(t *testing.T, replacement time.Duration) {
	t.Helper()
	previous := startTimeout
	startTimeout = replacement
	t.Cleanup(func() {
		startTimeout = previous
	})
}

func TestStartWaitsForStartedEvent(t *testing.T) {
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

	if err := Start(); err != nil {
		t.Fatal(err)
	}
}

func TestStartReturnsNativeStartError(t *testing.T) {
	want := errors.New("native start failed")
	replaceNativeStart(t, func(string, *tirtcclib.Callbacks) error {
		return want
	})

	if err := Start(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestStartTimesOut(t *testing.T) {
	replaceNativeStart(t, func(string, *tirtcclib.Callbacks) error {
		return nil
	})
	replaceStartTimeout(t, time.Millisecond)

	if err := Start(); !errors.Is(err, ErrStartTimeout) {
		t.Fatalf("error = %v, want ErrStartTimeout", err)
	}
}

func TestStartRejectsStoppedEvent(t *testing.T) {
	replaceNativeStart(t, func(_ string, callbacks *tirtcclib.Callbacks) error {
		go callbacks.OnEvent(int(tirtcclib.TiRtcEventSystemStopped), nil)
		return nil
	})

	if err := Start(); !errors.Is(err, ErrStoppedBeforeStart) {
		t.Fatalf("error = %v, want ErrStoppedBeforeStart", err)
	}
}

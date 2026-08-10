package tirtcx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tangeai/tirtc-service-sdk/v2/internal/tirtcclib"
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

	if err := Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStartReturnsNativeStartError(t *testing.T) {
	want := errors.New("native start failed")
	replaceNativeStart(t, func(string, *tirtcclib.Callbacks) error {
		return want
	})

	if err := Start(context.Background()); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestStartReturnsContextError(t *testing.T) {
	replaceNativeStart(t, func(string, *tirtcclib.Callbacks) error {
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if err := Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestStartRejectsStoppedEvent(t *testing.T) {
	replaceNativeStart(t, func(_ string, callbacks *tirtcclib.Callbacks) error {
		go callbacks.OnEvent(int(tirtcclib.TiRtcEventSystemStopped), nil)
		return nil
	})

	if err := Start(context.Background()); !errors.Is(err, ErrStoppedBeforeStart) {
		t.Fatalf("error = %v, want ErrStoppedBeforeStart", err)
	}
}

func TestStartRejectsNilContext(t *testing.T) {
	if err := Start(nil); !errors.Is(err, ErrNilContext) {
		t.Fatalf("error = %v, want ErrNilContext", err)
	}
}

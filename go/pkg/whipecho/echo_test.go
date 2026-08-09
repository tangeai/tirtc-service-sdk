package whipecho

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

type fakeAcceptor struct{}

func (fakeAcceptor) WhipAccept(
	context.Context,
	[]byte,
	tirtcx.ConnEventOptions,
) (*tirtcx.WhipSession, error) {
	return nil, nil
}

func TestNewRejectsNilAcceptor(t *testing.T) {
	_, err := NewSessionManager(nil, "/whip/echo/resource", Options{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewSessionManager error = %v, want ErrInvalidArgument", err)
	}
}

func TestNewSessionManagerRejectsNonCanonicalLocationPrefix(t *testing.T) {
	_, err := NewSessionManager(fakeAcceptor{}, "whip/echo/resource/", Options{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewSessionManager error = %v, want ErrInvalidArgument", err)
	}
}

func TestSessionManagerRejectsEmptyDeleteSessionID(t *testing.T) {
	manager, err := NewSessionManager(fakeAcceptor{}, "/whip/echo/resource", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete(context.Background(), " "); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Delete error = %v, want ErrInvalidArgument", err)
	}
}

func TestSessionManagerShutdownRejectsNewSessions(t *testing.T) {
	manager, err := NewSessionManager(fakeAcceptor{}, "/whip/echo/resource", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("repeated Shutdown: %v", err)
	}
	_, err = manager.Accept(context.Background(), AcceptRequest{OfferSDP: []byte("offer")})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Accept error = %v, want ErrClosed", err)
	}
	if stats := manager.Stats(); stats.ActiveSessions != 0 {
		t.Fatalf("ActiveSessions = %d, want 0", stats.ActiveSessions)
	}
}

func TestTimeSyncCommandWords(t *testing.T) {
	if CmdTimeSyncRequest != 0x2010 || CmdTimeSyncResponse != 0x2011 {
		t.Fatalf("time sync commands = %#x/%#x", CmdTimeSyncRequest, CmdTimeSyncResponse)
	}
}

func TestPackAudioTimestampPreservesFrameIndex(t *testing.T) {
	const timestamp = uint32(0xabcdefab)
	got := packAudioTimestamp(time.Now().UnixMicro(), timestamp)
	if got&audioTimestampIndexMask != timestamp&audioTimestampIndexMask {
		t.Fatalf("frame index changed: got=%x want=%x", got, timestamp)
	}
}

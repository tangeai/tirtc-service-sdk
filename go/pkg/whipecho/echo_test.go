package whipecho

import (
	"errors"
	"testing"
	"time"
)

func TestMode(t *testing.T) {
	tests := []struct {
		query string
		want  string
		err   bool
	}{
		{"device_id=d1", "", false},
		{"device_id=d1&_tg_mode=echo", "echo", false},
		{"_tg_mode=", "", true},
		{"_tg_mode=echo&_tg_mode=echo", "", true},
		{"%zz", "", true},
	}
	for _, test := range tests {
		got, err := Mode(test.query)
		if got != test.want || (err != nil) != test.err {
			t.Errorf("Mode(%q) = %q, %v; want %q, err=%v", test.query, got, err, test.want, test.err)
		}
	}
}

func TestNewRejectsNilAcceptor(t *testing.T) {
	_, err := New(Options{Enabled: true})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("New error = %v, want ErrBadRequest", err)
	}
}

func TestProbePayloadRoundTrip(t *testing.T) {
	now := time.Now()
	payload := makeProbePayload(42, now, 160)
	info, ok := parseProbePayload(payload)
	if !ok || info.seq != 42 || info.sentAtNS != now.UnixNano() {
		t.Fatalf("parseProbePayload = %+v, %v", info, ok)
	}
}

func TestPackAudioTimestampPreservesFrameIndex(t *testing.T) {
	const timestamp = uint32(0xabcdefab)
	got := packAudioTimestamp(time.Now().UnixMicro(), timestamp)
	if got&audioTimestampIndexMask != timestamp&audioTimestampIndexMask {
		t.Fatalf("frame index changed: got=%x want=%x", got, timestamp)
	}
}

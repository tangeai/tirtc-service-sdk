package tirtcx

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"
	"unsafe"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcclib"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
)

func TestAuthVerifiesQueryAndTokenBeforeAccept(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := tirtcxauth.NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewWhipServer(Options{
		Service:  "whip-echo-svc",
		Verifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("whip-echo-svc", "ak1", "dev-1", "device_id=dev-1&a=2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Auth("device_id=dev-1&a=3", token); err == nil {
		t.Fatal("Auth accepted mismatched query")
	}
}

func TestWhipAcceptCallsClibAfterValidToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := tirtcxauth.NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewWhipServer(Options{
		Service:  "whip-echo-svc",
		Verifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("whip-echo-svc", "ak1", "dev-1", "device_id=dev-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := server.Auth("device_id=dev-1", token)
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	_, err = server.WhipAccept([]byte("v=0\r\n"), ConnEventOptions{})
	if !errors.Is(err, tirtcclib.ErrUnavailable) {
		t.Fatalf("WhipAccept err = %v, want ErrUnavailable from clib stub", err)
	}
	if deviceID != "dev-1" {
		t.Fatalf("deviceID = %q, want dev-1", deviceID)
	}
}

func TestAuthReturnsDeviceID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := tirtcxauth.NewEd25519TokenIssuer(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
		"ak1": base64.RawURLEncoding.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewWhipServer(Options{
		Service:  "whip-echo-svc",
		Verifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := issuer.Issue("whip-echo-svc", "ak1", "dev-1", "device_id=dev-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := server.Auth("device_id=dev-1", token)
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if deviceID != "dev-1" {
		t.Fatalf("deviceID = %q, want dev-1", deviceID)
	}
}

func TestWhipAcceptCreatesConnWithChannels(t *testing.T) {
	oldAcceptWHIP := acceptWHIPFunc
	var audioCh <-chan AudioFrame
	acceptWHIPFunc = func(conn *Conn, offer []byte, candidate string) (*WhipAcceptResult, error) {
		if conn == nil {
			t.Fatal("WhipAccept passed nil conn")
		}
		audioCh = conn.Events().Audio
		sendToChan(conn.audioCh, AudioFrame{})
		return &WhipAcceptResult{AnswerSDP: []byte("v=0\r\n")}, nil
	}
	defer func() { acceptWHIPFunc = oldAcceptWHIP }()

	server := &WhipServer{}
	if _, err := server.WhipAccept([]byte("offer"), ConnEventOptions{
		AudioBuffer: 1,
	}); err != nil {
		t.Fatalf("WhipAccept: %v", err)
	}
	select {
	case <-audioCh:
	default:
		t.Fatal("WhipAccept did not install audio channel on conn")
	}
}

func TestWhipAcceptReturnsAnswer(t *testing.T) {
	oldAcceptWHIP := acceptWHIPFunc
	acceptWHIPFunc = func(*Conn, []byte, string) (*WhipAcceptResult, error) {
		return &WhipAcceptResult{AnswerSDP: []byte("v=0\r\n")}, nil
	}
	defer func() { acceptWHIPFunc = oldAcceptWHIP }()

	server := &WhipServer{}
	result, err := server.WhipAccept([]byte("offer"), ConnEventOptions{})
	if err != nil {
		t.Fatalf("WhipAccept: %v", err)
	}
	if string(result.AnswerSDP) != "v=0\r\n" {
		t.Fatalf("AnswerSDP = %q", result.AnswerSDP)
	}
}

func TestBindConnHandleBindsConnSynchronously(t *testing.T) {
	var raw byte
	handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
	conn := newConn("", ConnEventOptions{AudioBuffer: 1})
	audioCh := conn.Events().Audio
	defer connRegistry.Delete(handle)

	bound, ok := bindConnHandle(conn, handle)
	if !ok {
		t.Fatal("bindConnHandle failed")
	}
	if bound != conn {
		t.Fatalf("bindConnHandle = %p, want %p", bound, conn)
	}
	if got := lookupConn(handle); got != conn {
		t.Fatalf("lookupConn = %p, want %p", got, conn)
	}
	if conn.Handle() != handle {
		t.Fatalf("conn handle = %v, want %v", conn.Handle(), handle)
	}
	callbacks().OnAudio(handle, tirtcclib.FrameInfo{}, nil)
	select {
	case <-audioCh:
	default:
		t.Fatal("audio callback was not available after bind")
	}
}

func TestEventChannelBufferMinimum(t *testing.T) {
	conn := newConn("", ConnEventOptions{
		AudioBuffer:            1,
		VideoBuffer:            1,
		MessageBuffer:          1,
		CommandBuffer:          1,
		ErrorBuffer:            1,
		DisconnectedBuffer:     1,
		RequestKeyFrameBuffer:  1,
		SubscribeVideoBuffer:   1,
		UnsubscribeVideoBuffer: 1,
		SubscribeAudioBuffer:   1,
		UnsubscribeAudioBuffer: 1,
	})
	tests := []struct {
		name string
		cap  int
	}{
		{name: "Audio", cap: cap(conn.audioCh)},
		{name: "Video", cap: cap(conn.videoCh)},
		{name: "Message", cap: cap(conn.messageCh)},
		{name: "Command", cap: cap(conn.commandCh)},
		{name: "Error", cap: cap(conn.errorCh)},
		{name: "Disconnected", cap: cap(conn.disconnectedCh)},
		{name: "RequestKeyFrame", cap: cap(conn.requestKeyFrameCh)},
		{name: "SubscribeVideo", cap: cap(conn.subscribeVideoCh)},
		{name: "UnsubscribeVideo", cap: cap(conn.unsubscribeVideoCh)},
		{name: "SubscribeAudio", cap: cap(conn.subscribeAudioCh)},
		{name: "UnsubscribeAudio", cap: cap(conn.unsubscribeAudioCh)},
	}
	for _, tt := range tests {
		if tt.cap != MinEventBufferSize {
			t.Fatalf("%s channel cap = %d, want %d", tt.name, tt.cap, MinEventBufferSize)
		}
	}
}

func TestEventStatsCountsDroppedEvents(t *testing.T) {
	var raw byte
	handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
	conn := newConn("", ConnEventOptions{AudioBuffer: 1})
	audioCh := conn.Events().Audio
	for range cap(conn.audioCh) {
		conn.audioCh <- AudioFrame{}
	}
	defer connRegistry.Delete(handle)
	if _, ok := bindConnHandle(conn, handle); !ok {
		t.Fatal("bindConnHandle failed")
	}

	callbacks().OnAudio(handle, tirtcclib.FrameInfo{}, nil)

	stats := conn.EventStats()
	if stats.AudioDropped != 1 {
		t.Fatalf("AudioDropped = %d, want 1", stats.AudioDropped)
	}
	if len(audioCh) != cap(conn.audioCh) {
		t.Fatalf("audio channel len = %d, want %d", len(audioCh), cap(conn.audioCh))
	}
}

func TestEventStatsCountsDroppedSubscribeEvents(t *testing.T) {
	var raw byte
	handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
	conn := newConn("", ConnEventOptions{SubscribeVideoBuffer: 1})
	for range cap(conn.subscribeVideoCh) {
		conn.subscribeVideoCh <- SubscribeEvent{StreamID: 1, Result: make(chan int, 1)}
	}
	defer connRegistry.Delete(handle)
	if _, ok := bindConnHandle(conn, handle); !ok {
		t.Fatal("bindConnHandle failed")
	}

	result := callbacks().OnSubscribeVideo(handle, 2)

	if result != 0 {
		t.Fatalf("subscribe result = %d, want 0", result)
	}
	stats := conn.EventStats()
	if stats.SubscribeVideoDropped != 1 {
		t.Fatalf("SubscribeVideoDropped = %d, want 1", stats.SubscribeVideoDropped)
	}
}

func TestOnDisconnectedClosesEventChannels(t *testing.T) {
	var raw byte
	handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
	conn := newConn("", ConnEventOptions{
		AudioBuffer:        1,
		DisconnectedBuffer: 1,
	})
	events := conn.Events()
	defer connRegistry.Delete(handle)
	if _, ok := bindConnHandle(conn, handle); !ok {
		t.Fatal("bindConnHandle failed")
	}

	callbacks().OnDisconnected(handle)

	if _, ok := <-events.Disconnected; !ok {
		t.Fatal("disconnected channel closed before event delivery")
	}
	if _, ok := <-events.Disconnected; ok {
		t.Fatal("disconnected channel remains open after event delivery")
	}
	if _, ok := <-events.Audio; ok {
		t.Fatal("audio channel remains open after disconnect")
	}
}

func TestNewWhipServerRejectsInvalidConfig(t *testing.T) {
	if _, err := NewWhipServer(Options{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewWhipServer err = %v, want ErrInvalidConfig", err)
	}
}

func TestAuthRejectsNilWhipServer(t *testing.T) {
	var server *WhipServer
	if _, err := server.Auth("", ""); !errors.Is(err, ErrNilWhipServer) {
		t.Fatalf("Auth err = %v, want ErrNilWhipServer", err)
	}
}

func TestWhipAcceptRejectsNilWhipServer(t *testing.T) {
	var server *WhipServer
	if _, err := server.WhipAccept(nil, ConnEventOptions{}); !errors.Is(err, ErrNilWhipServer) {
		t.Fatalf("WhipAccept err = %v, want ErrNilWhipServer", err)
	}
}

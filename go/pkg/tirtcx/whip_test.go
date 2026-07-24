package tirtcx

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

func TestWhipAcceptCallsClib(t *testing.T) {
	acceptor := NewWhipAcceptor("")
	_, err := acceptor.WhipAccept(context.Background(), []byte("v=0\r\n"), ConnEventOptions{})
	if !errors.Is(err, tirtcclib.ErrUnavailable) {
		t.Fatalf("WhipAccept err = %v, want ErrUnavailable from clib stub", err)
	}
}

func TestWhipAcceptCreatesConnWithChannels(t *testing.T) {
	oldAcceptWHIP := acceptWHIPFunc
	var audioCh <-chan AudioFrame
	acceptWHIPFunc = func(_ context.Context, conn *Conn, offer []byte, candidate string) (*WhipSession, error) {
		if conn == nil {
			t.Fatal("WhipAccept passed nil conn")
		}
		audioCh = conn.Events().Audio
		sendToChan(conn.audioCh, AudioFrame{})
		return &WhipSession{answerSDP: []byte("v=0\r\n")}, nil
	}
	defer func() { acceptWHIPFunc = oldAcceptWHIP }()

	acceptor := NewWhipAcceptor("")
	if _, err := acceptor.WhipAccept(context.Background(), []byte("offer"), ConnEventOptions{
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
	acceptWHIPFunc = func(context.Context, *Conn, []byte, string) (*WhipSession, error) {
		return &WhipSession{answerSDP: []byte("v=0\r\n")}, nil
	}
	defer func() { acceptWHIPFunc = oldAcceptWHIP }()

	acceptor := NewWhipAcceptor("")
	result, err := acceptor.WhipAccept(context.Background(), []byte("offer"), ConnEventOptions{})
	if err != nil {
		t.Fatalf("WhipAccept: %v", err)
	}
	if string(result.AnswerSDP()) != "v=0\r\n" {
		t.Fatalf("AnswerSDP = %q", result.AnswerSDP())
	}
	answer := result.AnswerSDP()
	answer[0] = 'x'
	if string(result.AnswerSDP()) != "v=0\r\n" {
		t.Fatal("AnswerSDP returned mutable session storage")
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

func TestConnCloseAndBindDisconnectExactlyOnce(t *testing.T) {
	oldDisconnect := disconnectConnFunc
	defer func() { disconnectConnFunc = oldDisconnect }()
	var calls atomic.Int64
	disconnectConnFunc = func(tirtcclib.ConnHandle) error {
		calls.Add(1)
		return nil
	}

	for range 100 {
		var raw byte
		handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
		conn := newConn("", ConnEventOptions{})
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			conn.bindHandle(handle)
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = conn.Close()
			_ = conn.Close()
		}()
		close(start)
		wg.Wait()
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() != 100 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if got := calls.Load(); got != 100 {
		t.Fatalf("disconnect calls = %d, want 100", got)
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
		UnsubscribeVideoBuffer: 1,
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
		{name: "UnsubscribeVideo", cap: cap(conn.unsubscribeVideoCh)},
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

func TestSubscribeHandlersReturnSynchronously(t *testing.T) {
	var raw byte
	handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
	conn := newConn("", ConnEventOptions{
		SubscribeVideoHandler: func(streamID uint8) int {
			return int(streamID) + 10
		},
		SubscribeAudioHandler: func(streamID uint8) int {
			return int(streamID) + 20
		},
	})
	defer connRegistry.Delete(handle)
	if _, ok := bindConnHandle(conn, handle); !ok {
		t.Fatal("bindConnHandle failed")
	}

	if result := callbacks().OnSubscribeVideo(handle, 2); result != 12 {
		t.Fatalf("video subscribe result = %d, want 12", result)
	}
	if result := callbacks().OnSubscribeAudio(handle, 3); result != 23 {
		t.Fatalf("audio subscribe result = %d, want 23", result)
	}
}

func TestSubscribeHandlerPanicReturnsDefault(t *testing.T) {
	if result := callSubscribeHandler(func(uint8) int {
		panic("failed")
	}, 1); result != 0 {
		t.Fatalf("subscribe result = %d, want 0", result)
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

	select {
	case <-conn.Done():
	default:
		t.Fatal("Done remains open after disconnect")
	}
	if err := conn.Err(); err != nil {
		t.Fatalf("Err = %v after normal disconnect", err)
	}
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

func TestOnConnErrorPublishesTerminalError(t *testing.T) {
	var raw byte
	handle := tirtcclib.ConnHandle(unsafe.Pointer(&raw))
	conn := newConn("", ConnEventOptions{ErrorBuffer: 1})
	defer connRegistry.Delete(handle)
	if _, ok := bindConnHandle(conn, handle); !ok {
		t.Fatal("bindConnHandle failed")
	}

	callbacks().OnConnError(handle, -40007)

	select {
	case <-conn.Done():
	default:
		t.Fatal("Done remains open after connection error")
	}
	if !errors.Is(conn.Err(), tirtcclib.ErrConnTimeout) {
		t.Fatalf("Err = %v, want ErrConnTimeout", conn.Err())
	}
}

func TestWhipAcceptRejectsNilAcceptor(t *testing.T) {
	var acceptor *WhipAcceptor
	if _, err := acceptor.WhipAccept(context.Background(), nil, ConnEventOptions{}); !errors.Is(err, ErrNilWhipAcceptor) {
		t.Fatalf("WhipAccept err = %v, want ErrNilWhipAcceptor", err)
	}
}

func TestWhipAcceptRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	acceptor := NewWhipAcceptor("")
	if _, err := acceptor.WhipAccept(ctx, nil, ConnEventOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("WhipAccept error = %v, want context.Canceled", err)
	}
}

func TestAcceptContextCancellationClosesPendingConn(t *testing.T) {
	conn := newConn("", ConnEventOptions{})
	var canceled atomic.Int64
	result := &WhipSession{
		conn:         conn,
		connReady:    make(chan struct{}),
		done:         make(chan struct{}),
		cancelAccept: func() { canceled.Add(1) },
	}
	result.close(context.Canceled)
	if _, err := result.WaitConn(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitConn error = %v, want context.Canceled", err)
	}
	if !conn.closeRequested.Load() {
		t.Fatal("pending conn was not marked for close")
	}
	result.close(context.Canceled)
	if got := canceled.Load(); got != 1 {
		t.Fatalf("pending accept cancel calls = %d, want 1", got)
	}
}

func TestWhipSessionCloseUnblocksWaitConn(t *testing.T) {
	conn := newConn("", ConnEventOptions{})
	result := &WhipSession{
		conn:      conn,
		connReady: make(chan struct{}),
		done:      make(chan struct{}),
	}
	waitDone := make(chan error, 1)
	go func() {
		_, err := result.WaitConn(context.Background())
		waitDone <- err
	}()

	if err := result.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-waitDone:
		if !errors.Is(err, ErrWhipSessionClosed) {
			t.Fatalf("WaitConn error = %v, want ErrWhipSessionClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitConn was not unblocked by Close")
	}
}

func TestWhipSessionWaitConnIsConcurrentAndRepeatable(t *testing.T) {
	conn := newConn("", ConnEventOptions{})
	session := &WhipSession{
		conn:      conn,
		connReady: make(chan struct{}),
		done:      make(chan struct{}),
	}
	session.resolveConn(conn, nil)

	const waiters = 8
	results := make(chan *Conn, waiters)
	errors := make(chan error, waiters)
	for range waiters {
		go func() {
			got, err := session.WaitConn(context.Background())
			results <- got
			errors <- err
		}()
	}
	for range waiters {
		if err := <-errors; err != nil {
			t.Fatalf("WaitConn error = %v", err)
		}
		if got := <-results; got != conn {
			t.Fatalf("WaitConn conn = %p, want %p", got, conn)
		}
	}

	got, err := session.WaitConn(context.Background())
	if err != nil {
		t.Fatalf("repeated WaitConn error = %v", err)
	}
	if got != conn {
		t.Fatalf("repeated WaitConn conn = %p, want %p", got, conn)
	}
}

func TestWhipSessionWaitContextDoesNotCloseSession(t *testing.T) {
	conn := newConn("", ConnEventOptions{})
	session := &WhipSession{
		conn:      conn,
		connReady: make(chan struct{}),
		done:      make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := session.WaitConn(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitConn error = %v, want context.Canceled", err)
	}
	if conn.closeRequested.Load() {
		t.Fatal("canceling one waiter closed the session")
	}
	session.resolveConn(conn, nil)
	if got, err := session.WaitConn(context.Background()); err != nil || got != conn {
		t.Fatalf("later WaitConn = (%p, %v), want (%p, nil)", got, err, conn)
	}
}

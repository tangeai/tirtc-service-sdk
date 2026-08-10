package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tangeai/tirtc-service-sdk/v2/pkg/tirtcx"
	"github.com/tangeai/tirtc-service-sdk/v2/pkg/whipecho"
)

type mediaService struct {
	acceptor       whipecho.Acceptor
	locationPrefix string
	logger         *slog.Logger
	mu             sync.Mutex
	sessions       map[string]*mediaSession
	closed         bool
	wg             sync.WaitGroup
}

type mediaSession struct {
	cancel      context.CancelFunc
	whipSession *tirtcx.WhipSession
}

const (
	businessAudioStreamID uint8 = 0
	businessVideoStreamID uint8 = 1
)

func newMediaService(acceptor whipecho.Acceptor, locationPrefix string, logger *slog.Logger) *mediaService {
	return &mediaService{
		acceptor: acceptor, locationPrefix: locationPrefix, logger: logger,
		sessions: make(map[string]*mediaSession),
	}
}

func (s *mediaService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/sdp") {
		http.Error(w, "content-type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := newSessionID()
	if err != nil {
		http.Error(w, "create session ID: "+err.Error(), http.StatusInternalServerError)
		return
	}
	acceptContext, cancelAccept := context.WithTimeout(r.Context(), 30*time.Second)
	whipSession, err := s.acceptor.WhipAccept(acceptContext, offer, tirtcx.ConnEventOptions{
		ErrorBuffer:        tirtcx.MinEventBufferSize,
		DisconnectedBuffer: tirtcx.MinEventBufferSize,
	})
	cancelAccept()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx, cancelSession := context.WithCancel(context.Background())
	session := &mediaSession{cancel: cancelSession, whipSession: whipSession}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancelSession()
		_ = whipSession.Close()
		http.Error(w, "service is shutting down", http.StatusServiceUnavailable)
		return
	}
	s.sessions[id] = session
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run(ctx, id, session)

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", s.locationPrefix+"/"+id)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(whipSession.AnswerSDP())
}

func (s *mediaService) deleteResource(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	session := s.sessions[r.PathValue("session_id")]
	delete(s.sessions, r.PathValue("session_id"))
	s.mu.Unlock()
	if session != nil {
		session.cancel()
		_ = session.whipSession.Close()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *mediaService) run(
	ctx context.Context,
	id string,
	session *mediaSession,
) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.sessions, id)
		s.mu.Unlock()
	}()
	connectContext, cancelConnect := context.WithTimeout(ctx, 30*time.Second)
	defer cancelConnect()
	defer session.whipSession.Close()
	conn, err := session.whipSession.WaitConn(connectContext)
	if err != nil {
		s.logger.Error("business session failed to connect", "session_id", id, "error", err)
		return
	}
	audioTicker := time.NewTicker(20 * time.Millisecond)
	videoTicker := time.NewTicker(time.Second)
	defer audioTicker.Stop()
	defer videoTicker.Stop()
	events := conn.Events()
	var audioFrame uint32
	var videoFrame uint32

	for {
		select {
		case now := <-audioTicker.C:
			audio := sinePCM(audioFrame)
			info := tirtcx.FrameInfo{StreamID: businessAudioStreamID, Media: tirtcx.AudioPCM, Ts: uint32(now.UnixMilli())}
			if err := conn.SendAudio(info, audio); err != nil {
				s.logger.Error("send business audio", "session_id", id, "error", err)
				return
			}
			audioFrame++
		case now := <-videoTicker.C:
			video := syntheticJPEG(videoFrame)
			info := tirtcx.FrameInfo{StreamID: businessVideoStreamID, Media: tirtcx.VideoJPEG, Ts: uint32(now.UnixMilli())}
			if err := conn.SendVideo(info, video); err != nil {
				s.logger.Error("send business video", "session_id", id, "error", err)
				return
			}
			videoFrame++
		case err, ok := <-events.Error:
			if ok {
				s.logger.Error("business connection error", "session_id", id, "error", err)
			}
			return
		case <-events.Disconnected:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *mediaService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	sessions := make([]*mediaSession, 0, len(s.sessions))
	for id, session := range s.sessions {
		sessions = append(sessions, session)
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	for _, session := range sessions {
		session.cancel()
		_ = session.whipSession.Close()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sinePCM(frame uint32) []byte {
	const sampleRate = 8000
	const samplesPerFrame = sampleRate / 50
	const frequency = 440.0
	payload := make([]byte, samplesPerFrame*2)
	start := int(frame) * samplesPerFrame
	for index := 0; index < samplesPerFrame; index++ {
		phase := 2 * math.Pi * frequency * float64(start+index) / sampleRate
		sample := int16(math.Sin(phase) * 6000)
		binary.LittleEndian.PutUint16(payload[index*2:], uint16(sample))
	}
	return payload
}

func syntheticJPEG(frame uint32) []byte {
	const width, height = 320, 240
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{R: byte(40 + frame*17), G: byte(90 + frame*11), B: 180, A: 255}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
	x := int(frame*23) % (width - 48)
	draw.Draw(canvas, image.Rect(x, 80, x+48, 160), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	var encoded bytes.Buffer
	_ = jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 75})
	return encoded.Bytes()
}

func newSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

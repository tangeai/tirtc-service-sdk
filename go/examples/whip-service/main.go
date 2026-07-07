package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
)

const maxSDPBytes = 1 << 20

type service struct {
	serviceName string
	server      *tirtcx.WhipServer

	mu       sync.Mutex
	sessions map[string]*tirtcx.Conn
}

func main() {
	var (
		listen    = flag.String("listen", ":8080", "HTTP listen address")
		name      = flag.String("service", "wxvoip", "WHIP service name")
		ak        = flag.String("ak", "", "TiRTC-X access key")
		sk        = flag.String("sk", "", "TiRTC-X Ed25519 public key for token verification")
		candidate = flag.String("candidate", "", "optional public candidate IP/address")
	)
	flag.Parse()

	if *ak == "" || *sk == "" {
		log.Fatal("-ak and -sk are required")
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{
		*ak: *sk,
	})
	if err != nil {
		log.Fatalf("create token verifier: %v", err)
	}
	server, err := tirtcx.NewWhipServer(tirtcx.Options{
		Service:   *name,
		Verifier:  verifier,
		Candidate: *candidate,
	})
	if err != nil {
		log.Fatalf("create tirtcx WHIP server: %v", err)
	}

	if err := tirtcx.Init(); err != nil {
		log.Fatalf("tirtc init: %v", err)
	}
	defer tirtcx.Uninit()
	if err := tirtcx.Start(); err != nil {
		log.Fatalf("tirtc start: %v", err)
	}
	defer tirtcx.Stop()

	svc := &service{
		serviceName: *name,
		server:      server,
		sessions:    map[string]*tirtcx.Conn{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/whip", svc.handleWHIP)
	mux.HandleFunc("/resource/", svc.handleResource)

	log.Printf("example WHIP service listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

func (s *service) handleWHIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isSDP(r.Header.Get("Content-Type")) {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSDPBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sessionID, err := randomSessionID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	location := "/resource/" + sessionID

	if _, err := s.server.Auth(r.URL.RawQuery, token); err != nil {
		if errors.Is(err, tirtcx.ErrUnauthorized) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	eventOptions := connLogEventOptions()
	result, err := s.server.WhipAccept(offer, eventOptions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go func() {
		conn, err := result.WaitConn(30 * time.Second)
		if err != nil {
			log.Printf("[%s] wait conn failed: %v", sessionID, err)
			return
		}
		s.mu.Lock()
		s.sessions[sessionID] = conn
		s.mu.Unlock()
		logConnEvents(sessionID, conn.Events())
	}()

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(result.AnswerSDP)
}

func (s *service) handleResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/resource/")
	if sessionID == "" || sessionID == r.URL.Path {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	conn := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if conn == nil {
		http.NotFound(w, r)
		return
	}
	if err := conn.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func connLogEventOptions() tirtcx.ConnEventOptions {
	return tirtcx.ConnEventOptions{
		AudioBuffer:        16,
		VideoBuffer:        16,
		CommandBuffer:      16,
		ErrorBuffer:        1,
		DisconnectedBuffer: 1,
	}
}

func logConnEvents(sessionID string, events tirtcx.ConnEvents) {
	for {
		select {
		case frame, ok := <-events.Audio:
			if !ok {
				return
			}
			log.Printf("[%s] audio: media=%d bytes=%d", sessionID, frame.Info.Media, len(frame.Data))
		case frame, ok := <-events.Video:
			if !ok {
				return
			}
			log.Printf("[%s] video: media=%d bytes=%d", sessionID, frame.Info.Media, len(frame.Data))
		case command, ok := <-events.Command:
			if !ok {
				return
			}
			log.Printf("[%s] command: cmd=0x%x bytes=%d", sessionID, command.Cmd, len(command.Data))
		case err, ok := <-events.Error:
			if !ok {
				return
			}
			log.Printf("[%s] conn error: %v", sessionID, err)
			return
		case _, ok := <-events.Disconnected:
			if !ok {
				return
			}
			log.Printf("[%s] conn disconnected", sessionID)
			return
		}
	}
}

func isSDP(contentType string) bool {
	return strings.EqualFold(strings.TrimSpace(strings.Split(contentType, ";")[0]), "application/sdp")
}

func bearerToken(header string) (string, bool) {
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

func randomSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

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

type echoService struct {
	server   *tirtcx.WhipServer
	mu       sync.Mutex
	sessions map[string]*echoSession
}

type echoSession struct {
	mu     sync.Mutex
	conn   *tirtcx.Conn
	closed bool
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	serviceName := flag.String("service", "echo", "registered WHIP service name")
	accessKey := flag.String("access-key", "", "TiRTC-X access key")
	publicKey := flag.String("public-key", "", "Ed25519 public key used to verify TiRTC-X tokens")
	candidate := flag.String("candidate", "", "optional public candidate address")
	flag.Parse()

	if *accessKey == "" || *publicKey == "" {
		log.Fatal("-access-key and -public-key are required")
	}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{*accessKey: *publicKey})
	if err != nil {
		log.Fatalf("create token verifier: %v", err)
	}
	server, err := tirtcx.NewWhipServer(tirtcx.Options{
		Service: *serviceName, Verifier: verifier, Candidate: *candidate,
	})
	if err != nil {
		log.Fatalf("create WHIP server: %v", err)
	}
	if err := tirtcx.Init(); err != nil {
		log.Fatalf("initialize TiRTC: %v", err)
	}
	defer tirtcx.Uninit()
	if err := tirtcx.Start(); err != nil {
		log.Fatalf("start TiRTC: %v", err)
	}
	defer tirtcx.Stop()

	svc := &echoService{server: server, sessions: make(map[string]*echoSession)}
	mux := http.NewServeMux()
	mux.HandleFunc("/whip", svc.handleWHIP)
	mux.HandleFunc("/resource/", svc.handleResource)
	log.Printf("WHIP echo service is listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

func (s *echoService) handleWHIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isSDP(r.Header.Get("Content-Type")) {
		http.Error(w, "content type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSDPBytes))
	if err != nil {
		http.Error(w, "read SDP offer: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.server.Auth(r.URL.RawQuery, token); err != nil {
		if errors.Is(err, tirtcx.ErrUnauthorized) {
			http.Error(w, "invalid bearer token", http.StatusForbidden)
			return
		}
		http.Error(w, "verify bearer token", http.StatusInternalServerError)
		return
	}
	result, err := s.server.WhipAccept(offer, tirtcx.ConnEventOptions{
		AudioBuffer: 32, VideoBuffer: 32,
		ErrorBuffer:        tirtcx.MinEventBufferSize,
		DisconnectedBuffer: tirtcx.MinEventBufferSize,
	})
	if err != nil {
		http.Error(w, "accept WHIP connection: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sessionID, err := randomSessionID()
	if err != nil {
		http.Error(w, "create session ID", http.StatusInternalServerError)
		return
	}
	session := &echoSession{}
	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()
	go s.runSession(sessionID, session, result)
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/resource/"+sessionID)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(result.AnswerSDP)
}

func (s *echoService) runSession(
	sessionID string,
	session *echoSession,
	result *tirtcx.WhipAcceptResult,
) {
	conn, err := result.WaitConn(30 * time.Second)
	if err != nil {
		log.Printf("[%s] connection failed: %v", sessionID, err)
		s.removeSession(sessionID, session)
		return
	}
	if !session.bind(conn) {
		_ = conn.Close()
		return
	}
	defer func() {
		s.removeSession(sessionID, session)
		_ = session.close()
	}()

	log.Printf("[%s] connected", sessionID)
	events := conn.Events()
	for {
		select {
		case frame, ok := <-events.Audio:
			if !ok {
				return
			}
			if err := conn.SendAudio(frame.Info, frame.Data); err != nil {
				log.Printf("[%s] echo audio: %v", sessionID, err)
				return
			}
		case frame, ok := <-events.Video:
			if !ok {
				return
			}
			if err := conn.SendVideo(frame.Info, frame.Data); err != nil {
				log.Printf("[%s] echo video: %v", sessionID, err)
				return
			}
		case err, ok := <-events.Error:
			if ok {
				log.Printf("[%s] connection error: %v", sessionID, err)
			}
			return
		case <-events.Disconnected:
			log.Printf("[%s] disconnected", sessionID)
			return
		}
	}
}

func (s *echoService) handleResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/resource/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	session := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if session == nil {
		http.NotFound(w, r)
		return
	}
	if err := session.close(); err != nil {
		http.Error(w, "close connection: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *echoService) removeSession(sessionID string, session *echoSession) {
	s.mu.Lock()
	if s.sessions[sessionID] == session {
		delete(s.sessions, sessionID)
	}
	s.mu.Unlock()
}

func (s *echoSession) bind(conn *tirtcx.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conn = conn
	return true
}

func (s *echoSession) close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
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
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

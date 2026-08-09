package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/whipecho"
)

type businessService struct {
	acceptor whipecho.Acceptor
	logger   *slog.Logger
	mu       sync.Mutex
	sessions map[string]*businessSession
	closed   bool
	wg       sync.WaitGroup
}

type businessSession struct {
	cancel      context.CancelFunc
	whipSession *tirtcx.WhipSession
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	serviceName := flag.String("service", "gin-service", "registered WHIP service name")
	accessKey := flag.String("access-key", "", "TiRTC-X access key")
	publicKey := flag.String("public-key", "", "Ed25519 public key used to verify TiRTC-X tokens")
	candidate := flag.String("candidate", "", "optional public candidate address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{*accessKey: *publicKey})
	if err != nil {
		logger.Error("create token verifier", "error", err)
		return
	}
	if err := tirtcx.Init(); err != nil {
		logger.Error("initialize TiRTC-X", "error", err)
		return
	}
	defer tirtcx.Uninit()
	if err := tirtcx.Start(context.Background()); err != nil {
		logger.Error("start TiRTC-X", "error", err)
		return
	}
	defer tirtcx.Stop()

	acceptor := tirtcx.NewWhipAcceptor(*candidate)
	business := &businessService{
		acceptor: acceptor,
		logger:   logger,
		sessions: make(map[string]*businessSession),
	}
	echo, err := whipecho.NewHTTPAdapter(acceptor, "/whip/echo/resource", whipecho.HTTPOptions{
		Logger: logger,
	})
	if err != nil {
		logger.Error("create Echo capability", "error", err)
		return
	}

	router := gin.New()
	router.Use(gin.Recovery())
	whip := router.Group("/whip")
	whip.POST(
		"",
		ginAuth(tirtcxauth.BearerInterceptor(*serviceName, verifier)),
		echoMiddleware(echo),
		business.postWHIP,
	)
	whip.DELETE("/resource/:session_id", business.deleteResource)
	whip.DELETE("/echo/resource/:session_id", echoDeleteHandler(echo))

	server := &http.Server{
		Addr:              *listen,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Gin WHIP service with Echo started", "listen", *listen)
		serverErrors <- server.ListenAndServe()
	}()

	stopContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server stopped", "error", err)
		}
	case <-stopContext.Done():
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("shut down HTTP server", "error", err)
	}
	if err := business.Shutdown(shutdownContext); err != nil {
		logger.Error("shut down business sessions", "error", err)
	}
	if err := echo.Shutdown(shutdownContext); err != nil {
		logger.Error("shut down Echo sessions", "error", err)
	}
}

func ginAuth(auth tirtcxauth.HTTPInterceptor) gin.HandlerFunc {
	return func(c *gin.Context) {
		authenticated := false
		auth(func(_ http.ResponseWriter, r *http.Request) {
			c.Request = r
			authenticated = true
		})(c.Writer, c.Request)
		if !authenticated {
			c.Abort()
		}
	}
}

func echoMiddleware(echo *whipecho.HTTPAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if echo.TryHandlePOST(c.Writer, c.Request) {
			c.Abort()
			return
		}
		c.Next()
	}
}

func echoDeleteHandler(echo *whipecho.HTTPAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		echo.ServeDelete(c.Writer, c.Request, c.Param("session_id"))
	}
}

func (s *businessService) postWHIP(c *gin.Context) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/sdp") {
		c.String(http.StatusUnsupportedMediaType, "content-type must be application/sdp")
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "%v", err)
		return
	}
	sessionID, err := newSessionID()
	if err != nil {
		c.String(http.StatusInternalServerError, "create session ID: %v", err)
		return
	}
	acceptContext, cancelAccept := context.WithTimeout(c.Request.Context(), 30*time.Second)
	whipSession, err := s.acceptor.WhipAccept(acceptContext, offer, tirtcx.ConnEventOptions{
		ErrorBuffer:        tirtcx.MinEventBufferSize,
		DisconnectedBuffer: tirtcx.MinEventBufferSize,
	})
	cancelAccept()
	if err != nil {
		c.String(http.StatusInternalServerError, "%v", err)
		return
	}
	ctx, cancelSession := context.WithCancel(context.Background())
	session := &businessSession{cancel: cancelSession, whipSession: whipSession}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancelSession()
		_ = whipSession.Close()
		c.String(http.StatusServiceUnavailable, "service is shutting down")
		return
	}
	s.sessions[sessionID] = session
	s.wg.Add(1)
	s.mu.Unlock()
	go s.waitSession(ctx, sessionID, session)

	c.Header("Content-Type", "application/sdp")
	c.Header("Location", "/whip/resource/"+sessionID)
	c.Data(http.StatusCreated, "application/sdp", whipSession.AnswerSDP())
}

func (s *businessService) deleteResource(c *gin.Context) {
	s.mu.Lock()
	session := s.sessions[c.Param("session_id")]
	delete(s.sessions, c.Param("session_id"))
	s.mu.Unlock()
	if session != nil {
		session.cancel()
		_ = session.whipSession.Close()
	}
	c.Status(http.StatusNoContent)
}

func (s *businessService) waitSession(
	ctx context.Context,
	sessionID string,
	session *businessSession,
) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
	}()
	connectContext, cancelConnect := context.WithTimeout(ctx, 30*time.Second)
	defer cancelConnect()
	conn, err := session.whipSession.WaitConn(connectContext)
	if err != nil {
		s.logger.Error("business session failed to connect", "session_id", sessionID, "error", err)
		return
	}
	defer session.whipSession.Close()
	events := conn.Events()
	select {
	case <-events.Error:
	case <-events.Disconnected:
	case <-ctx.Done():
	}
}

func (s *businessService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	sessions := make([]*businessSession, 0, len(s.sessions))
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

func newSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

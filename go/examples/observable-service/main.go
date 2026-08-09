package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/whipecho"
)

type observedMetrics struct {
	connectCount       atomic.Uint64
	connectDurationNS  atomic.Uint64
	authenticatedTotal atomic.Uint64
	authFailedTotal    atomic.Uint64
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	serviceName := flag.String("service", "echo", "registered WHIP service name")
	accessKey := flag.String("access-key", "", "TiRTC-X access key")
	publicKey := flag.String("public-key", "", "Ed25519 public key used to verify TiRTC-X tokens")
	candidate := flag.String("candidate", "", "optional public candidate address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	metrics := &observedMetrics{}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{*accessKey: *publicKey})
	if err != nil {
		logger.Error("create token verifier", "error", err)
		return
	}

	echoObserver := whipecho.ObserverFuncs{
		OnSessionConnected: func(duration time.Duration) {
			metrics.connectCount.Add(1)
			metrics.connectDurationNS.Add(uint64(duration))
			logger.Info("Echo session connected", "connect_duration", duration)
		},
		OnSessionClosed: func(reason whipecho.CloseReason, duration time.Duration) {
			logger.Info("Echo session closed", "reason", reason, "duration", duration)
		},
		OnAcceptFailed: func(stage whipecho.AcceptStage) {
			logger.Warn("Echo accept failed", "stage", stage)
		},
		OnConnectFailed: func(reason whipecho.ConnectFailureReason) {
			logger.Warn("Echo connection failed", "reason", reason)
		},
		OnSendFailed: func(media whipecho.MediaType) {
			logger.Warn("Echo send failed", "media", media)
		},
		OnEventsDropped: func(event whipecho.EventType, count uint64) {
			logger.Warn("Echo events dropped", "event", event, "count", count)
		},
	}
	authObserver := tirtcxauth.AuthObserverFuncs{
		OnAuthenticated: func() {
			metrics.authenticatedTotal.Add(1)
		},
		OnAuthenticationFailed: func(reason tirtcxauth.AuthFailureReason) {
			metrics.authFailedTotal.Add(1)
			logger.Warn("WHIP authentication failed", "reason", reason)
		},
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

	mux := http.NewServeMux()
	acceptor := tirtcx.NewWhipAcceptor(*candidate)
	echo, err := whipecho.NewHTTPAdapter(
		acceptor,
		"/whip/echo/resource",
		whipecho.HTTPOptions{
			Logger:   logger,
			Observer: echoObserver,
		},
	)
	if err != nil {
		logger.Error("create Echo capability", "error", err)
		return
	}
	auth := tirtcxauth.BearerInterceptor(*serviceName, verifier, authObserver)
	mux.HandleFunc("POST /whip", auth(echo.PostHandler()))
	mux.HandleFunc("DELETE /whip/echo/resource/{session_id}", echo.DeleteHandler())
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		writeMetrics(w, echo.Stats(), metrics)
	})

	server := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("observable WHIP Echo service started", "listen", *listen)
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
	if err := echo.Shutdown(shutdownContext); err != nil {
		logger.Error("shut down Echo sessions", "error", err)
	}
}

func writeMetrics(w http.ResponseWriter, stats whipecho.Stats, observed *observedMetrics) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "whip_echo_active_sessions %d\n", stats.ActiveSessions)
	_, _ = fmt.Fprintf(w, "whip_echo_accepted_total %d\n", stats.AcceptedTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_accept_failed_total %d\n", stats.AcceptFailedTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_connect_failed_total %d\n", stats.ConnectFailedTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_connect_timeout_total %d\n", stats.ConnectTimeoutTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_audio_frames_total %d\n", stats.AudioFramesTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_audio_bytes_total %d\n", stats.AudioBytesTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_video_frames_total %d\n", stats.VideoFramesTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_video_bytes_total %d\n", stats.VideoBytesTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_send_failed_total %d\n", stats.SendFailedTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_dropped_events_total %d\n", stats.DroppedEventsTotal)
	for reason, count := range stats.ClosedTotal {
		_, _ = fmt.Fprintf(w, "whip_echo_closed_total{reason=%q} %d\n", reason, count)
	}
	_, _ = fmt.Fprintf(w, "whip_echo_connect_duration_count %d\n", observed.connectCount.Load())
	_, _ = fmt.Fprintf(
		w,
		"whip_echo_connect_duration_seconds_sum %f\n",
		time.Duration(observed.connectDurationNS.Load()).Seconds(),
	)
	_, _ = fmt.Fprintf(w, "whip_auth_success_total %d\n", observed.authenticatedTotal.Load())
	_, _ = fmt.Fprintf(w, "whip_auth_failed_total %d\n", observed.authFailedTotal.Load())
}

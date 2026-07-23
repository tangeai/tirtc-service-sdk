package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
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
	metrics := &observedMetrics{}
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{*accessKey: *publicKey})
	must(err, logger)

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

	mux := http.NewServeMux()
	echo, err := whipecho.RegisterHTTPHandlers(
		mux,
		"/whip",
		whipecho.HTTPOptions{
			Candidate: *candidate,
			Enabled:   true,
			Logger:    logger,
			Observer:  echoObserver,
		},
		tirtcxauth.BearerInterceptor(*serviceName, verifier, authObserver),
	)
	must(err, logger)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		writeMetrics(w, echo.Stats(), metrics)
	})

	must(tirtcx.Init(), logger)
	defer tirtcx.Uninit()
	must(tirtcx.Start(), logger)
	defer tirtcx.Stop()

	logger.Info("observable WHIP Echo service started", "listen", *listen)
	must(http.ListenAndServe(*listen, mux), logger)
}

func writeMetrics(w http.ResponseWriter, stats whipecho.Stats, observed *observedMetrics) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "whip_echo_active_sessions %d\n", stats.ActiveSessions)
	_, _ = fmt.Fprintf(w, "whip_echo_accepted_total %d\n", stats.AcceptedTotal)
	_, _ = fmt.Fprintf(w, "whip_echo_accept_failed_total %d\n", stats.AcceptFailedTotal)
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

func must(err error, logger *slog.Logger) {
	if err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

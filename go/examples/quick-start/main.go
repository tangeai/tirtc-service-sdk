package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/whipecho"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	serviceName := flag.String("service", "quick-start", "registered WHIP service name")
	accessKey := flag.String("access-key", "", "TiRTC-X access key")
	publicKey := flag.String("public-key", "", "Ed25519 public key used to verify TiRTC-X tokens")
	candidate := flag.String("candidate", "", "optional public candidate address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	verifier, err := tirtcxauth.NewEd25519TokenVerifier(map[string]string{*accessKey: *publicKey})
	if err != nil {
		logger.Error("create token verifier", "error", err)
		return
	}
	mux := http.NewServeMux()
	acceptor := tirtcx.NewWhipAcceptor(*candidate)
	business := newMediaService(acceptor, "/whip/resource", logger)
	echo, err := whipecho.NewHTTPAdapter(acceptor, "/whip/echo/resource", whipecho.HTTPOptions{
		Logger: logger,
	})
	if err != nil {
		logger.Error("create Echo capability", "error", err)
		return
	}
	auth := tirtcxauth.BearerInterceptor(*serviceName, verifier)
	mux.HandleFunc("POST /whip", auth(echo.Wrap(business)))
	mux.HandleFunc("DELETE /whip/resource/{session_id}", business.deleteResource)
	mux.HandleFunc("DELETE /whip/echo/resource/{session_id}", echo.DeleteHandler())

	if err := tirtcx.Init(); err != nil {
		logger.Error("initialize TiRTC-X", "error", err)
		return
	}
	defer tirtcx.Uninit()
	if err := tirtcx.Start(); err != nil {
		logger.Error("start TiRTC-X", "error", err)
		return
	}
	defer tirtcx.Stop()

	server := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("WHIP quick start with synthetic media and Echo started", "listen", *listen)
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

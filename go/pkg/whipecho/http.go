package whipecho

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMaxSDPBytes = 1 << 20
const modeQueryKey = "_tg_mode"
const modeEcho = "echo"

// HTTPOptions configures the Echo HTTP handlers.
type HTTPOptions struct {
	MaxSDPBytes int64
	ConnTimeout time.Duration
	Logger      *slog.Logger
	Observer    Observer
}

// HTTPAdapter exposes Echo through composable net/http operations.
type HTTPAdapter struct {
	echo        echoSessionManager
	manager     *SessionManager
	maxSDPBytes int64
	logger      *slog.Logger
}

// NewHTTPAdapter creates an Echo HTTP adapter. locationPrefix is returned
// in successful POST responses with the session ID appended. It is used
// exactly as provided and must be a canonical absolute path without a trailing
// slash.
//
// For locationPrefix "/custom/whip/echo/resource", applications should
// register:
//
//	mux.HandleFunc("POST /custom/whip", auth(echo.Wrap(business)))
//	mux.HandleFunc("DELETE /custom/whip/echo/resource/{session_id}", echo.DeleteHandler())
//
// Other HTTP frameworks can call TryHandlePOST and ServeDelete directly.
//
// Echo mode is selected internally. Other POST requests are delegated to the
// handler passed to Wrap. Dedicated Echo endpoints use PostHandler.
func NewHTTPAdapter(
	acceptor Acceptor,
	locationPrefix string,
	opts HTTPOptions,
) (*HTTPAdapter, error) {
	manager, err := NewSessionManager(acceptor, locationPrefix, Options{
		ConnTimeout: opts.ConnTimeout,
		Logger:      opts.Logger,
		Observer:    opts.Observer,
	})
	if err != nil {
		return nil, err
	}

	adapter := &HTTPAdapter{
		echo:        manager,
		manager:     manager,
		maxSDPBytes: opts.MaxSDPBytes,
		logger:      manager.opts.Logger,
	}
	if adapter.maxSDPBytes <= 0 {
		adapter.maxSDPBytes = defaultMaxSDPBytes
	}
	return adapter, nil
}

// Wrap adds Echo mode to a WHIP business handler. It panics if business is nil.
func (h *HTTPAdapter) Wrap(business http.Handler) http.HandlerFunc {
	if business == nil {
		panic("whipecho: nil business handler")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if h.TryHandlePOST(w, r) {
			return
		}
		business.ServeHTTP(w, r)
	}
}

// PostHandler returns a handler for a dedicated Echo POST endpoint.
func (h *HTTPAdapter) PostHandler() http.HandlerFunc {
	return h.servePOST
}

// TryHandlePOST handles an Echo POST and reports whether it wrote the response.
// Ordinary business requests return false without consuming their body.
// Invalid internal mode values are rejected and return true.
func (h *HTTPAdapter) TryHandlePOST(w http.ResponseWriter, r *http.Request) bool {
	echo, err := echoMode(r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return true
	}
	if !echo {
		return false
	}
	h.servePOST(w, r)
	return true
}

// DeleteHandler returns the handler for the Echo resource route.
func (h *HTTPAdapter) DeleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeDelete(w, r, r.PathValue("session_id"))
	}
}

// ServeDelete deletes an Echo resource using a session ID extracted by the
// application's router.
func (h *HTTPAdapter) ServeDelete(w http.ResponseWriter, r *http.Request, sessionID string) {
	if err := h.echo.Delete(r.Context(), sessionID); err != nil {
		if errors.Is(err, ErrInvalidArgument) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.logger.Error("delete Echo session", "session_id", sessionID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Stats returns a snapshot of Echo session statistics.
func (h *HTTPAdapter) Stats() Stats {
	return h.manager.Stats()
}

// Shutdown closes all Echo sessions and waits for their goroutines to exit.
func (h *HTTPAdapter) Shutdown(ctx context.Context) error {
	if h == nil || h.manager == nil {
		return nil
	}
	return h.manager.Shutdown(ctx)
}

func echoMode(rawQuery string) (bool, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false, fmt.Errorf("%w: invalid query: %v", ErrInvalidArgument, err)
	}
	modes, exists := values[modeQueryKey]
	if !exists {
		return false, nil
	}
	if len(modes) != 1 || strings.TrimSpace(modes[0]) != modeEcho {
		return false, fmt.Errorf("%w: invalid internal mode", ErrInvalidArgument)
	}
	return true, nil
}

func (h *HTTPAdapter) servePOST(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/sdp") {
		http.Error(w, "content-type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxSDPBytes))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "SDP offer is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.echo.Accept(r.Context(), AcceptRequest{OfferSDP: offer})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidArgument):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrClosed):
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		default:
			h.logger.Error("accept Echo session", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", result.Location)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(result.AnswerSDP)
}

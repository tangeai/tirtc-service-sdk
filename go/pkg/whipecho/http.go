package whipecho

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

const defaultMaxSDPBytes = 1 << 20

// HTTPOptions configures the Echo HTTP handlers.
type HTTPOptions struct {
	Candidate          string
	Enabled            bool
	RequireMode        bool
	MaxSDPBytes        int64
	ConnTimeout        time.Duration
	LatencyRepeat      int
	LatencyInterval    time.Duration
	LatencyTimeout     time.Duration
	LatencyProbeBytes  int
	LatencyAudioStream uint8
	Logger             *slog.Logger
	Observer           Observer
}

// RegisterHTTPHandlers creates an Echo service and registers its related HTTP
// handlers on mux. basePath is used exactly as provided and must be a canonical
// absolute path without a trailing slash.
//
// For basePath "/custom/whip", the registered routes are:
//
//	POST   /custom/whip
//	DELETE /custom/whip/resource/{session_id}
//
// Successful POST responses use the DELETE route as their Location.
func RegisterHTTPHandlers(
	mux *http.ServeMux,
	basePath string,
	opts HTTPOptions,
	interceptors ...func(http.Handler) http.Handler,
) (*Service, error) {
	if mux == nil {
		return nil, fmt.Errorf("%w: HTTP mux is nil", ErrBadRequest)
	}
	if err := validateBasePath(basePath); err != nil {
		return nil, err
	}
	for index, interceptor := range interceptors {
		if interceptor == nil {
			return nil, fmt.Errorf("%w: HTTP interceptor %d is nil", ErrBadRequest, index)
		}
	}

	service, err := New(Options{
		Acceptor:           tirtcx.NewWhipAcceptor(opts.Candidate),
		Enabled:            opts.Enabled,
		RequireMode:        opts.RequireMode,
		LocationPrefix:     basePath + "/resource",
		ConnTimeout:        opts.ConnTimeout,
		LatencyRepeat:      opts.LatencyRepeat,
		LatencyInterval:    opts.LatencyInterval,
		LatencyTimeout:     opts.LatencyTimeout,
		LatencyProbeBytes:  opts.LatencyProbeBytes,
		LatencyAudioStream: opts.LatencyAudioStream,
		Logger:             opts.Logger,
		Observer:           opts.Observer,
	})
	if err != nil {
		return nil, err
	}

	handler := &httpHandler{echo: service, maxSDPBytes: opts.MaxSDPBytes}
	if handler.maxSDPBytes <= 0 {
		handler.maxSDPBytes = defaultMaxSDPBytes
	}
	accept := applyInterceptors(http.HandlerFunc(handler.postWHIP), interceptors)
	deleteResource := applyInterceptors(http.HandlerFunc(handler.deleteResource), interceptors)
	mux.Handle(http.MethodPost+" "+basePath, accept)
	mux.Handle(http.MethodDelete+" "+basePath+"/resource/{session_id}", deleteResource)
	return service, nil
}

func validateBasePath(basePath string) error {
	switch {
	case basePath == "":
		return fmt.Errorf("%w: invalid HTTP base path %q: path is empty", ErrBadRequest, basePath)
	case !strings.HasPrefix(basePath, "/"):
		return fmt.Errorf("%w: invalid HTTP base path %q: path must start with '/'", ErrBadRequest, basePath)
	case strings.HasSuffix(basePath, "/"):
		return fmt.Errorf("%w: invalid HTTP base path %q: trailing slash is not allowed", ErrBadRequest, basePath)
	case strings.ContainsAny(basePath, "?#{} \t\r\n"):
		return fmt.Errorf("%w: invalid HTTP base path %q: query, fragment, wildcard, and whitespace are not allowed", ErrBadRequest, basePath)
	case path.Clean(basePath) != basePath:
		return fmt.Errorf("%w: invalid HTTP base path %q: path is not canonical", ErrBadRequest, basePath)
	default:
		return nil
	}
}

func applyInterceptors(handler http.Handler, interceptors []func(http.Handler) http.Handler) http.Handler {
	for index := len(interceptors) - 1; index >= 0; index-- {
		handler = interceptors[index](handler)
	}
	return handler
}

type httpHandler struct {
	echo        EchoService
	maxSDPBytes int64
}

func (h *httpHandler) postWHIP(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/sdp") {
		http.Error(w, "content-type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}
	offer, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxSDPBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.echo.Accept(r.Context(), EchoRequest{OfferSDP: offer, RawQuery: r.URL.RawQuery})
	if err != nil {
		switch {
		case errors.Is(err, ErrBadRequest):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrDisabled):
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", result.Location)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(result.AnswerSDP)
}

func (h *httpHandler) deleteResource(w http.ResponseWriter, r *http.Request) {
	if err := h.echo.Delete(r.Context(), r.PathValue("session_id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

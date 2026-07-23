package whipecho

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeEcho struct {
	request EchoRequest
	result  *EchoResult
	err     error
	deleted string
}

func (f *fakeEcho) Accept(_ context.Context, request EchoRequest) (*EchoResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeEcho) Delete(_ context.Context, sessionID string) error {
	f.deleted = sessionID
	return f.err
}

func TestHTTPHandlerForwardsRawQuery(t *testing.T) {
	echo := &fakeEcho{result: &EchoResult{
		SessionID: "session-1",
		AnswerSDP: []byte("v=0\r\n"),
		Location:  "/whip/echo/resource/session-1",
	}}
	handler := &httpHandler{echo: echo, maxSDPBytes: 1024}
	request := httptest.NewRequest(http.MethodPost, "/whip?b=1&_tg_mode=echo&a=2", strings.NewReader("offer"))
	request.Header.Set("Content-Type", "application/sdp")
	response := httptest.NewRecorder()
	handler.postWHIP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if echo.request.RawQuery != "b=1&_tg_mode=echo&a=2" || string(echo.request.OfferSDP) != "offer" {
		t.Fatalf("request not forwarded: %+v", echo.request)
	}
	if got := response.Header().Get("Location"); got != echo.result.Location {
		t.Fatalf("Location = %q", got)
	}
}

func TestHTTPHandlerMapsErrors(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{ErrBadRequest, http.StatusBadRequest},
		{ErrDisabled, http.StatusServiceUnavailable},
		{errors.New("failed"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		handler := &httpHandler{echo: &fakeEcho{err: test.err}, maxSDPBytes: 1024}
		request := httptest.NewRequest(http.MethodPost, "/whip?_tg_mode=echo", strings.NewReader("offer"))
		request.Header.Set("Content-Type", "application/sdp")
		response := httptest.NewRecorder()
		handler.postWHIP(response, request)
		if response.Code != test.want {
			t.Errorf("error %v status = %d, want %d", test.err, response.Code, test.want)
		}
	}
}

func TestHTTPHandlerDeleteIsForwarded(t *testing.T) {
	echo := &fakeEcho{}
	handler := &httpHandler{echo: echo}
	request := httptest.NewRequest(http.MethodDelete, "/whip/echo/resource/session-1", nil)
	request.SetPathValue("session_id", "session-1")
	response := httptest.NewRecorder()
	handler.deleteResource(response, request)
	if response.Code != http.StatusNoContent || echo.deleted != "session-1" {
		t.Fatalf("status=%d deleted=%q", response.Code, echo.deleted)
	}
}

func TestValidateBasePathIsStrict(t *testing.T) {
	valid := []string{"/whip", "/custom/whip", "/v1/echo-whip"}
	for _, basePath := range valid {
		if err := validateBasePath(basePath); err != nil {
			t.Errorf("validateBasePath(%q): %v", basePath, err)
		}
	}
	invalid := []string{
		"",
		"custom/whip",
		"/custom/whip/",
		"/custom//whip",
		"/custom/./whip",
		"/custom/../whip",
		"/custom/whip?mode=echo",
		"/custom/{service}",
	}
	for _, basePath := range invalid {
		if err := validateBasePath(basePath); err == nil {
			t.Errorf("validateBasePath(%q) unexpectedly succeeded", basePath)
		}
	}
}

func TestApplyInterceptorsUsesDeclarationOrder(t *testing.T) {
	var calls []string
	first := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "first-before")
			next.ServeHTTP(w, r)
			calls = append(calls, "first-after")
		})
	}
	second := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "second-before")
			next.ServeHTTP(w, r)
			calls = append(calls, "second-after")
		})
	}
	handler := applyInterceptors(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls = append(calls, "handler")
	}), []func(http.Handler) http.Handler{first, second})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	got := strings.Join(calls, ",")
	want := "first-before,second-before,handler,second-after,first-after"
	if got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

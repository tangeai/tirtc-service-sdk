package whipecho

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeEcho struct {
	request AcceptRequest
	result  *AcceptResult
	err     error
	deleted string
}

func (f *fakeEcho) Accept(_ context.Context, request AcceptRequest) (*AcceptResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeEcho) Delete(_ context.Context, sessionID string) error {
	f.deleted = sessionID
	return f.err
}

func TestHTTPHandlerAcceptsEcho(t *testing.T) {
	echo := &fakeEcho{result: &AcceptResult{
		SessionID: "session-1",
		AnswerSDP: []byte("v=0\r\n"),
		Location:  "/whip/echo/resource/session-1",
	}}
	handler := &HTTPAdapter{echo: echo, maxSDPBytes: 1024, logger: slog.Default()}
	request := httptest.NewRequest(http.MethodPost, "/whip?b=1&_tg_mode=echo&a=2", strings.NewReader("offer"))
	request.Header.Set("Content-Type", "application/sdp")
	response := httptest.NewRecorder()
	if !handler.TryHandlePOST(response, request) {
		t.Fatal("Echo request was not handled")
	}

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if string(echo.request.OfferSDP) != "offer" {
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
		{ErrInvalidArgument, http.StatusBadRequest},
		{ErrClosed, http.StatusServiceUnavailable},
		{errors.New("failed"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		handler := &HTTPAdapter{echo: &fakeEcho{err: test.err}, maxSDPBytes: 1024, logger: slog.Default()}
		request := httptest.NewRequest(http.MethodPost, "/whip?_tg_mode=echo", strings.NewReader("offer"))
		request.Header.Set("Content-Type", "application/sdp")
		response := httptest.NewRecorder()
		handler.servePOST(response, request)
		if response.Code != test.want {
			t.Errorf("error %v status = %d, want %d", test.err, response.Code, test.want)
		}
		if test.want == http.StatusInternalServerError && strings.Contains(response.Body.String(), test.err.Error()) {
			t.Errorf("internal error leaked in response body: %q", response.Body.String())
		}
	}
}

func TestHTTPAdapterRejectsOversizedOffer(t *testing.T) {
	handler := &HTTPAdapter{echo: &fakeEcho{}, maxSDPBytes: 4, logger: slog.Default()}
	request := httptest.NewRequest(http.MethodPost, "/whip", strings.NewReader("offer"))
	request.Header.Set("Content-Type", "application/sdp")
	response := httptest.NewRecorder()
	handler.PostHandler()(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHTTPAdapterWrapRejectsNilBusiness(t *testing.T) {
	handler := &HTTPAdapter{}
	defer func() {
		if recover() == nil {
			t.Fatal("Wrap(nil) did not panic")
		}
	}()
	handler.Wrap(nil)
}

func TestHTTPHandlerDelegatesBusinessRequest(t *testing.T) {
	called := false
	handler := &HTTPAdapter{
		echo: &fakeEcho{},
	}
	business := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	})
	response := httptest.NewRecorder()
	handler.Wrap(business).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/whip?room=1", nil))
	if !called || response.Code != http.StatusAccepted {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}

func TestHTTPHandlerKeepsModeInternal(t *testing.T) {
	handler := &HTTPAdapter{echo: &fakeEcho{}}
	for _, query := range []string{"_tg_mode=", "_tg_mode=other", "_tg_mode=echo&_tg_mode=echo"} {
		response := httptest.NewRecorder()
		handler.Wrap(http.NotFoundHandler()).ServeHTTP(
			response,
			httptest.NewRequest(http.MethodPost, "/whip?"+query, nil),
		)
		if response.Code != http.StatusBadRequest {
			t.Errorf("query=%q status=%d", query, response.Code)
		}
	}
}

func TestHTTPHandlerDeleteIsForwarded(t *testing.T) {
	echo := &fakeEcho{}
	handler := &HTTPAdapter{echo: echo, logger: slog.Default()}
	request := httptest.NewRequest(http.MethodDelete, "/whip/echo/resource/session-1", nil)
	request.SetPathValue("session_id", "session-1")
	response := httptest.NewRecorder()
	handler.ServeDelete(response, request, "session-1")
	if response.Code != http.StatusNoContent || echo.deleted != "session-1" {
		t.Fatalf("status=%d deleted=%q", response.Code, echo.deleted)
	}
}

func TestHTTPAdapterRejectsEmptyDeleteSessionID(t *testing.T) {
	handler := &HTTPAdapter{
		echo:    &fakeEcho{err: fmt.Errorf("%w: session ID is empty", ErrInvalidArgument)},
		logger:  slog.Default(),
		manager: nil,
	}
	response := httptest.NewRecorder()
	handler.ServeDelete(
		response,
		httptest.NewRequest(http.MethodDelete, "/whip/echo/resource/", nil),
		"",
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestHTTPHandlerIgnoresBusinessRequestWithoutConsumingBody(t *testing.T) {
	handler := &HTTPAdapter{echo: &fakeEcho{}}
	request := httptest.NewRequest(http.MethodPost, "/whip?room=1", strings.NewReader("offer"))
	response := httptest.NewRecorder()
	if handler.TryHandlePOST(response, request) {
		t.Fatal("business request was handled as Echo")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "offer" {
		t.Fatalf("body = %q", body)
	}
}

func TestValidateLocationPrefixIsStrict(t *testing.T) {
	valid := []string{"/whip/resource", "/custom/whip/echo/resource", "/v1/echo-whip/resource"}
	for _, locationPrefix := range valid {
		if err := validateLocationPrefix(locationPrefix); err != nil {
			t.Errorf("validateLocationPrefix(%q): %v", locationPrefix, err)
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
	for _, locationPrefix := range invalid {
		if err := validateLocationPrefix(locationPrefix); err == nil {
			t.Errorf("validateLocationPrefix(%q) unexpectedly succeeded", locationPrefix)
		}
	}
}

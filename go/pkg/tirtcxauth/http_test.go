package tirtcxauth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type verifierFunc func(service, deviceID, query, token string) (*TokenClaims, error)

func (f verifierFunc) Verify(service, deviceID, query, token string) (*TokenClaims, error) {
	return f(service, deviceID, query, token)
}

func TestBearerInterceptorPassesRawQuery(t *testing.T) {
	var gotQuery, gotToken string
	verifier := verifierFunc(func(_, _, query, token string) (*TokenClaims, error) {
		gotQuery, gotToken = query, token
		return &TokenClaims{Subject: "device-1"}, nil
	})
	handler := BearerInterceptor("echo", verifier)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/whip?b=1&_tg_mode=echo&a=2", nil)
	request.Header.Set("Authorization", "Bearer signed-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || gotQuery != "b=1&_tg_mode=echo&a=2" || gotToken != "signed-token" {
		t.Fatalf("status=%d query=%q token=%q", response.Code, gotQuery, gotToken)
	}
}

func TestBearerInterceptorRejectsMissingToken(t *testing.T) {
	handler := BearerInterceptor("echo", verifierFunc(func(_, _, _, _ string) (*TokenClaims, error) {
		t.Fatal("verifier should not be called")
		return nil, nil
	}))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next should not be called")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/whip", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}

type authRecordingObserver struct {
	mu       sync.Mutex
	success  int
	failures []AuthFailureReason
}

func (o *authRecordingObserver) Authenticated() {
	o.mu.Lock()
	o.success++
	o.mu.Unlock()
}

func (o *authRecordingObserver) AuthenticationFailed(reason AuthFailureReason) {
	o.mu.Lock()
	o.failures = append(o.failures, reason)
	o.mu.Unlock()
}

func TestBearerInterceptorObserver(t *testing.T) {
	observer := &authRecordingObserver{}
	verifier := verifierFunc(func(_, _, _, _ string) (*TokenClaims, error) {
		return &TokenClaims{Subject: "device-1"}, nil
	})
	handler := BearerInterceptor("echo", verifier, observer)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/whip", nil))
	validRequest := httptest.NewRequest(http.MethodPost, "/whip", nil)
	validRequest.Header.Set("Authorization", "Bearer token")
	valid := httptest.NewRecorder()
	handler.ServeHTTP(valid, validRequest)

	if observer.success != 1 || len(observer.failures) != 1 ||
		observer.failures[0] != AuthFailureMissingToken {
		t.Fatalf("observer success=%d failures=%v", observer.success, observer.failures)
	}
}

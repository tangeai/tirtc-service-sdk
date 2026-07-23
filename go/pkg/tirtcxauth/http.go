package tirtcxauth

import (
	"net/http"
	"strings"
)

// HTTPInterceptor is standard net/http middleware.
type HTTPInterceptor func(http.Handler) http.Handler

type AuthFailureReason string

const (
	AuthFailureMissingToken  AuthFailureReason = "missing_token"
	AuthFailureInvalidToken  AuthFailureReason = "invalid_token"
	AuthFailureMisconfigured AuthFailureReason = "misconfigured"
)

// AuthObserver receives bounded-cardinality authentication results.
// Implementations must be concurrency-safe and should return quickly.
type AuthObserver interface {
	Authenticated()
	AuthenticationFailed(AuthFailureReason)
}

type AuthObserverFuncs struct {
	OnAuthenticated        func()
	OnAuthenticationFailed func(AuthFailureReason)
}

func (o AuthObserverFuncs) Authenticated() {
	if o.OnAuthenticated != nil {
		o.OnAuthenticated()
	}
}

func (o AuthObserverFuncs) AuthenticationFailed(reason AuthFailureReason) {
	if o.OnAuthenticationFailed != nil {
		o.OnAuthenticationFailed(reason)
	}
}

// BearerInterceptor verifies the Authorization bearer token against the
// request's complete raw query before invoking the next handler.
func BearerInterceptor(service string, verifier TokenVerifier, observers ...AuthObserver) HTTPInterceptor {
	service = strings.TrimSpace(service)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				notifyAuthFailed(observers, AuthFailureMissingToken)
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			if verifier == nil || service == "" {
				notifyAuthFailed(observers, AuthFailureMisconfigured)
				http.Error(w, "token verifier is not configured", http.StatusInternalServerError)
				return
			}
			if _, err := verifier.Verify(service, "", r.URL.RawQuery, token); err != nil {
				notifyAuthFailed(observers, AuthFailureInvalidToken)
				http.Error(w, "invalid bearer token", http.StatusForbidden)
				return
			}
			for _, observer := range observers {
				if observer != nil {
					observer.Authenticated()
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func notifyAuthFailed(observers []AuthObserver, reason AuthFailureReason) {
	for _, observer := range observers {
		if observer != nil {
			observer.AuthenticationFailed(reason)
		}
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], parts[1] != ""
}

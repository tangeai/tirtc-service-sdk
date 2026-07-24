package tirtcxauth

import (
	"context"
	"net/http"
	"strings"
)

type claimsContextKey struct{}

// HTTPInterceptor authenticates an HTTP handler function.
type HTTPInterceptor func(http.HandlerFunc) http.HandlerFunc

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

// ClaimsFromContext returns a copy of the verified token claims attached to an
// HTTP request by BearerInterceptor.
func ClaimsFromContext(ctx context.Context) (TokenClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(TokenClaims)
	return claims, ok
}

// BearerInterceptor verifies the Authorization bearer token against the
// request's complete raw query. On success, it attaches a copy of the verified
// claims to the request context before invoking the next handler.
func BearerInterceptor(service string, verifier TokenVerifier, observers ...AuthObserver) HTTPInterceptor {
	service = strings.TrimSpace(service)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
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
			claims, err := verifier.Verify(service, "", r.URL.RawQuery, token)
			if err != nil || claims == nil {
				notifyAuthFailed(observers, AuthFailureInvalidToken)
				http.Error(w, "invalid bearer token", http.StatusForbidden)
				return
			}
			for _, observer := range observers {
				if observer != nil {
					observer.Authenticated()
				}
			}
			ctx := context.WithValue(r.Context(), claimsContextKey{}, *claims)
			next(w, r.WithContext(ctx))
		}
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

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcxauth"
)

func TestBusinessDeleteIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &businessService{}
	router := gin.New()
	router.DELETE("/whip/resource/:session_id", service.deleteResource)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/whip/resource/missing", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestGinAuthStopsOrContinuesHandlerChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := tirtcxauth.HTTPInterceptor(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer valid" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	})
	called := 0
	router := gin.New()
	router.GET("/protected", ginAuth(auth), func(c *gin.Context) {
		called++
		c.Status(http.StatusNoContent)
	})

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if unauthorized.Code != http.StatusUnauthorized || called != 0 {
		t.Fatalf("unauthorized status=%d called=%d", unauthorized.Code, called)
	}

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer valid")
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("authorized status=%d called=%d", authorized.Code, called)
	}
}

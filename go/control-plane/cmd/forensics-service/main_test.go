package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticateExceptHealth(t *testing.T) {
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer valid" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	handler := authenticateExceptHealth(authenticate)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/ready"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d, want %d", path, recorder.Code, http.StatusOK)
		}
	}

	for _, path := range []string{"/health/extra", "/readyz", "/api/v1/forensics/tasks"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

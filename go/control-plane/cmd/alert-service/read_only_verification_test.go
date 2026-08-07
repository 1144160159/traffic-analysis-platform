package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadOnlyVerificationHandlerAllowsSafeMethods(t *testing.T) {
	called := 0
	handler := readOnlyVerificationHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/alerts", nil))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", method, recorder.Code, http.StatusNoContent)
		}
	}
	if called != 3 {
		t.Fatalf("safe method calls = %d, want 3", called)
	}
}

func TestReadOnlyVerificationHandlerRejectsMutationsBeforeRouting(t *testing.T) {
	called := false
	handler := readOnlyVerificationHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/playbooks/block-scanner/execute", strings.NewReader(`{}`)))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want %d", method, recorder.Code, http.StatusMethodNotAllowed)
		}
		if !strings.Contains(recorder.Body.String(), `"code":"READ_ONLY_VERIFICATION"`) {
			t.Fatalf("%s response did not explain read-only gate: %s", method, recorder.Body.String())
		}
	}
	if called {
		t.Fatal("mutation reached the application router")
	}
}

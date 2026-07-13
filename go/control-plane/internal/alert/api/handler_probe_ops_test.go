package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func TestProbeOperationRoutesMatchAPIPrefix(t *testing.T) {
	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	NewSystemHandler(nil, nil, zap.NewNop()).RegisterRoutes(apiRouter)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/probes/probe-001/config", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Fatalf("probe config route returned 404; route was not matched")
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

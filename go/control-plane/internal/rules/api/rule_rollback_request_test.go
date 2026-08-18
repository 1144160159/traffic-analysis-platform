package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func TestRuleRollbackRouteIsRegisteredAsPost(t *testing.T) {
	handler := NewHandler(nil, nil, nil, nil, nil, zap.NewNop(), DefaultHandlerConfig())
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	post := httptest.NewRecorder()
	router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/rules/rule-1/rollback", strings.NewReader(`{}`)))
	if post.Code == http.StatusNotFound || post.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST rollback route status = %d", post.Code)
	}
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/rules/rule-1/rollback", nil))
	if get.Code != http.StatusNotFound && get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET rollback route status = %d, want route rejection", get.Code)
	}
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/rules/rule-1/operations/11111111-1111-4111-8111-111111111111",
		nil,
	))
	if status.Code == http.StatusNotFound || status.Code == http.StatusMethodNotAllowed {
		t.Fatalf("GET rule application status route = %d", status.Code)
	}
}

func TestRollbackRuleRequestValidate(t *testing.T) {
	req := &RollbackRuleRequest{TargetVersion: 2, ExpectedVersion: 5, Reason: "  restore known-good DNS rule  "}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Reason != "restore known-good DNS rule" {
		t.Fatalf("trimmed reason = %q", req.Reason)
	}

	tests := []struct {
		name string
		req  RollbackRuleRequest
		code commonerrors.ErrorCode
	}{
		{name: "target", req: RollbackRuleRequest{ExpectedVersion: 5, Reason: "reason"}, code: commonerrors.ErrCodeInvalidParameter},
		{name: "expected", req: RollbackRuleRequest{TargetVersion: 2, Reason: "reason"}, code: commonerrors.ErrCodeInvalidParameter},
		{name: "reason", req: RollbackRuleRequest{TargetVersion: 2, ExpectedVersion: 5}, code: commonerrors.ErrCodeMissingParameter},
		{name: "reason length", req: RollbackRuleRequest{TargetVersion: 2, ExpectedVersion: 5, Reason: strings.Repeat("x", 1001)}, code: commonerrors.ErrCodeInvalidParameter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.req.Validate(); commonerrors.GetCode(err) != tt.code {
				t.Fatalf("Validate() error = %v, code = %s, want %s", err, commonerrors.GetCode(err), tt.code)
			}
		})
	}
}

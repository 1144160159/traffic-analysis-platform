package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func TestUpdateStatusRequiresReasonBeforeServiceCall(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/status", bytes.NewBufferString(`{"status":"assigned"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertWrite)
	req = mux.SetURLVars(req, map[string]string{"id": "AL-1"})
	rr := httptest.NewRecorder()

	handler.UpdateStatus(rr, req)

	if rr.Code < 400 {
		t.Fatalf("status=%d want 4xx body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "reason is required") {
		t.Fatalf("response should explain missing reason: %s", rr.Body.String())
	}
}

func TestBatchUpdateStatusRequiresReasonBeforeServiceCall(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/batch/status", bytes.NewBufferString(`{"alert_ids":["AL-1"],"status":"closed"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertWrite)
	rr := httptest.NewRecorder()

	handler.BatchUpdateStatus(rr, req)

	if rr.Code < 400 {
		t.Fatalf("status=%d want 4xx body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "reason is required") {
		t.Fatalf("response should explain missing reason: %s", rr.Body.String())
	}
}

func TestCloseAlertRequiresReasonBeforeServiceCall(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/close", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertWrite)
	req = mux.SetURLVars(req, map[string]string{"id": "AL-1"})
	rr := httptest.NewRecorder()

	handler.CloseAlert(rr, req)

	if rr.Code < 400 {
		t.Fatalf("status=%d want 4xx body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "reason is required") {
		t.Fatalf("response should explain missing reason: %s", rr.Body.String())
	}
}

func TestUpdateStatusRejectsViewerWithoutAlertWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/status", bytes.NewBufferString(`{"status":"assigned","reason":"triage owner set"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertRead)
	req = mux.SetURLVars(req, map[string]string{"id": "AL-1"})
	rr := httptest.NewRecorder()

	handler.UpdateStatus(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alert:write required") {
		t.Fatalf("response should explain missing permission: %s", rr.Body.String())
	}
}

func TestAssignAlertRejectsViewerWithoutAlertWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/assign", bytes.NewBufferString(`{"assignee":"sec_analyst"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertRead)
	req = mux.SetURLVars(req, map[string]string{"id": "AL-1"})
	rr := httptest.NewRecorder()

	handler.AssignAlert(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alert:write required") {
		t.Fatalf("response should explain missing permission: %s", rr.Body.String())
	}
}

func TestCloseAlertRejectsViewerWithoutAlertWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/close", bytes.NewBufferString(`{"reason":"analysis complete"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertRead)
	req = mux.SetURLVars(req, map[string]string{"id": "AL-1"})
	rr := httptest.NewRecorder()

	handler.CloseAlert(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alert:write required") {
		t.Fatalf("response should explain missing permission: %s", rr.Body.String())
	}
}

func TestReopenAlertRejectsViewerWithoutAlertWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reopen", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertRead)
	req = mux.SetURLVars(req, map[string]string{"id": "AL-1"})
	rr := httptest.NewRecorder()

	handler.ReopenAlert(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alert:write required") {
		t.Fatalf("response should explain missing permission: %s", rr.Body.String())
	}
}

func TestBatchUpdateStatusRejectsViewerWithoutAlertWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/batch/status", bytes.NewBufferString(`{"alert_ids":["AL-1"],"status":"closed","reason":"bulk closure approved"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertRead)
	rr := httptest.NewRecorder()

	handler.BatchUpdateStatus(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alert:write required") {
		t.Fatalf("response should explain missing permission: %s", rr.Body.String())
	}
}

func TestParseExpectedStateVersionAcceptsBodyAndIfMatch(t *testing.T) {
	bodyVersion := uint64(1782712345678)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/status", nil)
	req.Header.Set("If-Match", `W/"1782712345678"`)

	version, err := parseExpectedStateVersion(UpdateStatusRequest{StateVersion: &bodyVersion}, req)
	if err != nil {
		t.Fatalf("parseExpectedStateVersion() error = %v", err)
	}
	if version == nil || *version != bodyVersion {
		t.Fatalf("version=%v want %d", version, bodyVersion)
	}
}

func TestParseExpectedStateVersionRejectsMismatchedSources(t *testing.T) {
	bodyVersion := uint64(1782712345678)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/status", nil)
	req.Header.Set("If-Match", `"1782712345677"`)

	_, err := parseExpectedStateVersion(UpdateStatusRequest{StateVersion: &bodyVersion}, req)
	if err == nil {
		t.Fatal("parseExpectedStateVersion() expected mismatch error")
	}
	if !strings.Contains(err.Error(), "If-Match") {
		t.Fatalf("error should mention If-Match mismatch: %v", err)
	}
}

func TestParseExpectedStateVersionRejectsInvalidIfMatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/status", nil)
	req.Header.Set("If-Match", `"not-a-version"`)

	_, err := parseExpectedStateVersion(UpdateStatusRequest{}, req)
	if err == nil {
		t.Fatal("parseExpectedStateVersion() expected invalid If-Match error")
	}
	if !strings.Contains(err.Error(), "invalid If-Match state_version") {
		t.Fatalf("error should explain invalid If-Match: %v", err)
	}
}

func TestBuildBatchStatusItemsAcceptsVersionedItems(t *testing.T) {
	version := uint64(1782712345678)
	items, err := buildBatchStatusItems(BatchUpdateStatusRequest{
		Items: []BatchStatusItemRequest{{AlertID: " AL-1 ", StateVersion: &version}},
	})
	if err != nil {
		t.Fatalf("buildBatchStatusItems() error = %v", err)
	}
	if len(items) != 1 || items[0].AlertID != "AL-1" {
		t.Fatalf("items=%+v want AL-1", items)
	}
	if items[0].ExpectedVersion == nil || *items[0].ExpectedVersion != version {
		t.Fatalf("expected version=%v want %d", items[0].ExpectedVersion, version)
	}
}

func TestBatchUpdateStatusRejectsZeroItemVersionBeforeServiceCall(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/batch/status", bytes.NewBufferString(`{"items":[{"alert_id":"AL-1","state_version":0}],"status":"closed","reason":"bulk closure approved"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req = withPermissions(req, model.ScopeAlertWrite)
	rr := httptest.NewRecorder()

	handler.BatchUpdateStatus(rr, req)

	if rr.Code < 400 {
		t.Fatalf("status=%d want 4xx body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "items[].state_version") {
		t.Fatalf("response should explain invalid item state_version: %s", rr.Body.String())
	}
}

func TestAlertWritePermissionAcceptsWildcards(t *testing.T) {
	for _, permission := range []string{model.ScopeAll, "alert:*", model.ScopeAdminAll, model.ScopeAlertWrite} {
		t.Run(permission, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/AL-1/status", nil)
			req = withPermissions(req, permission)
			if !hasAlertWritePermission(req.Context()) {
				t.Fatalf("permission %q should allow alert status updates", permission)
			}
		})
	}
}

func withPermissions(req *http.Request, permissions ...string) *http.Request {
	ctx := context.WithValue(req.Context(), httpx.ContextKeyPermissions, permissions)
	return req.WithContext(ctx)
}

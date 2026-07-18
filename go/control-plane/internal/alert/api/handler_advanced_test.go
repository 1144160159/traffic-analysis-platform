package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/playbook"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

func newAdvancedTestRouter(h *AdvancedHandler) http.Handler {
	r := mux.NewRouter()
	api := r.PathPrefix("/api/v1").Subrouter()
	h.RegisterAPIRoutes(api)
	return r
}

func newAdvancedTestPlaybookEngine(t *testing.T) *playbook.PlaybookEngine {
	t.Helper()

	engine := playbook.NewPlaybookEngine(playbook.NewActionExecutor(zap.NewNop()), zap.NewNop())
	engine.RegisterPlaybook(&playbook.Playbook{
		Name:        "isolate-host",
		Description: "test isolation playbook",
		Enabled:     true,
		Trigger:     playbook.Trigger{AlertType: "scan", SeverityMin: "medium", ScoreMin: 0.5},
		Actions: []playbook.Action{
			{Type: "tag", Parameters: map[string]interface{}{"tags": []string{"isolated"}}, Timeout: time.Second},
			{Type: "notify", Parameters: map[string]interface{}{"channel": "webhook"}, Timeout: time.Second},
		},
		MaxRuns: 5,
	})
	return engine
}

func doAdvancedRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-test")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func doAdvancedRequestWithPermissions(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body string,
	roles []string,
	permissions []string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "tenant-test")
	ctx := req.Context()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-test")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "11111111-1111-1111-1111-111111111111")
	ctx = context.WithValue(ctx, httpx.ContextKeyRoles, roles)
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req.WithContext(ctx))
	return rr
}

func decodeAdvancedBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v\nbody=%s", err, rr.Body.String())
	}
	return body
}

func TestDataQualityTablePaginationValidation(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, nil, &AdvancedRepository{}))
	tests := []struct {
		name string
		path string
		want int
	}{
		{name: "supported dataset defaults pagination", path: "/api/v1/data-quality/tables/fieldQualityRows", want: http.StatusNotFound},
		{name: "unsupported dataset", path: "/api/v1/data-quality/tables/privateRows", want: http.StatusBadRequest},
		{name: "page must be positive", path: "/api/v1/data-quality/tables/fieldQualityRows?page=0", want: http.StatusBadRequest},
		{name: "page must be numeric", path: "/api/v1/data-quality/tables/fieldQualityRows?page=next", want: http.StatusBadRequest},
		{name: "page size must be positive", path: "/api/v1/data-quality/tables/fieldQualityRows?page_size=0", want: http.StatusBadRequest},
		{name: "page size is bounded", path: "/api/v1/data-quality/tables/fieldQualityRows?page_size=101", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := doAdvancedRequestWithPermissions(
				t,
				router,
				http.MethodGet,
				tc.path,
				"",
				[]string{"viewer"},
				[]string{authmodel.ScopeDataQualityRead},
			)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestAdvancedHandlerUnavailableDependenciesReturnJSON(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, nil, nil))

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "risk asset scorer unavailable", method: http.MethodGet, path: "/api/v1/risk/assets/10.0.0.1", want: http.StatusServiceUnavailable},
		{name: "data quality unavailable", method: http.MethodGet, path: "/api/v1/data-quality", want: http.StatusServiceUnavailable},
		{name: "latency chain unavailable", method: http.MethodGet, path: "/api/v1/data-quality/latency-chain", want: http.StatusServiceUnavailable},
		{name: "data quality baseline unavailable", method: http.MethodPost, path: "/api/v1/data-quality/baseline", want: http.StatusServiceUnavailable},
		{name: "data quality action unavailable", method: http.MethodPost, path: "/api/v1/data-quality/actions", want: http.StatusServiceUnavailable},
		{name: "playbook catalog unavailable", method: http.MethodGet, path: "/api/v1/playbooks/catalog", want: http.StatusServiceUnavailable},
		{name: "playbook executions empty without repo", method: http.MethodGet, path: "/api/v1/playbooks/executions", want: http.StatusOK},
		{name: "notification test unavailable", method: http.MethodPost, path: "/api/v1/notifications/test", want: http.StatusServiceUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rr *httptest.ResponseRecorder
			if tc.path == "/api/v1/notifications/test" {
				rr = doAdvancedRequestWithPermissions(t, router, tc.method, tc.path, "", []string{"admin"}, []string{"admin:*"})
			} else if strings.HasPrefix(tc.path, "/api/v1/data-quality") {
				permission := authmodel.ScopeDataQualityRead
				body := ""
				if tc.method == http.MethodPost {
					permission = authmodel.ScopeDataQualityWrite
				}
				if tc.path == "/api/v1/data-quality/actions" {
					body = `{"view":"overview","action":"inspect","target":"current-selection","dry_run":true}`
				}
				rr = doAdvancedRequestWithPermissions(t, router, tc.method, tc.path, body, []string{"operator"}, []string{permission})
			} else {
				rr = doAdvancedRequest(t, router, tc.method, tc.path, "")
			}
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
			body := decodeAdvancedBody(t, rr)
			if _, ok := body["success"].(bool); !ok {
				t.Fatalf("response should include boolean success: %#v", body)
			}
		})
	}
}

func TestAdvancedHandlerPatchPlaybookValidationAndSuccess(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, newAdvancedTestPlaybookEngine(t), nil, nil))

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "unknown field", body: `{"enabled":false,"extra":true}`, want: http.StatusBadRequest},
		{name: "negative max runs", body: `{"max_runs":-1}`, want: http.StatusBadRequest},
		{name: "negative cooldown", body: `{"cooldown_seconds":-1}`, want: http.StatusBadRequest},
		{name: "missing playbook", body: `{"enabled":false}`, want: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/playbooks/isolate-host"
			if tc.name == "missing playbook" {
				path = "/api/v1/playbooks/not-found"
			}
			rr := doAdvancedRequest(t, router, http.MethodPatch, path, tc.body)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}

	rr := doAdvancedRequest(t, router, http.MethodPatch, "/api/v1/playbooks/isolate-host", `{"enabled":false,"max_runs":3,"cooldown_seconds":30}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := decodeAdvancedBody(t, rr)
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data object: %#v", body)
	}
	if data["enabled"] != false {
		t.Fatalf("enabled=%v want false", data["enabled"])
	}
	if data["max_runs"] != float64(3) {
		t.Fatalf("max_runs=%v want 3", data["max_runs"])
	}
}

func TestAdvancedHandlerLatencyChainRejectsInvalidLookback(t *testing.T) {
	monitor := dataquality.NewMonitor(nil, dataquality.MonitorConfig{}, zap.NewNop())
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, monitor, nil))

	rr := doAdvancedRequestWithPermissions(t, router, http.MethodGet, "/api/v1/data-quality/latency-chain?lookback_minutes=0", "", []string{"viewer"}, []string{authmodel.ScopeDataQualityRead})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	body := decodeAdvancedBody(t, rr)
	if body["success"] != false {
		t.Fatalf("response should fail for invalid lookback: %#v", body)
	}
}

func TestDataQualityPermissionAndActionValidation(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, nil, nil))

	readDenied := doAdvancedRequestWithPermissions(t, router, http.MethodGet, "/api/v1/data-quality", "", []string{"viewer"}, []string{authmodel.ScopeAlertRead})
	if readDenied.Code != http.StatusForbidden {
		t.Fatalf("read without scope status=%d body=%s", readDenied.Code, readDenied.Body.String())
	}
	tableDenied := doAdvancedRequestWithPermissions(t, router, http.MethodGet, "/api/v1/data-quality/tables/fieldQualityRows?page=2&page_size=5", "", []string{"viewer"}, []string{authmodel.ScopeAlertRead})
	if tableDenied.Code != http.StatusForbidden {
		t.Fatalf("table read without scope status=%d body=%s", tableDenied.Code, tableDenied.Body.String())
	}
	actionDenied := doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/data-quality/actions", `{"view":"overview","action":"inspect","target":"current-selection","dry_run":true}`, []string{"viewer"}, []string{authmodel.ScopeDataQualityRead})
	if actionDenied.Code != http.StatusForbidden {
		t.Fatalf("write with read scope status=%d body=%s", actionDenied.Code, actionDenied.Body.String())
	}
	invalidView := doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/data-quality/actions", `{"view":"unknown","action":"inspect","target":"current-selection","dry_run":true}`, []string{"operator"}, []string{authmodel.ScopeDataQualityWrite})
	if invalidView.Code != http.StatusBadRequest {
		t.Fatalf("invalid view status=%d body=%s", invalidView.Code, invalidView.Body.String())
	}
	unconfirmed := doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/data-quality/actions", `{"view":"overview","action":"repair","target":"dlq.v1","dry_run":false,"reason":"short"}`, []string{"operator"}, []string{authmodel.ScopeDataQualityWrite})
	if unconfirmed.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed write status=%d body=%s", unconfirmed.Code, unconfirmed.Body.String())
	}
}

func TestAdvancedHandlerExecutePlaybookUsesRequestPayload(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, newAdvancedTestPlaybookEngine(t), nil, nil))

	rr := doAdvancedRequest(t, router, http.MethodPost, "/api/v1/playbooks/isolate-host/execute", `{
		"alert_id":"alert-42",
		"alert_type":"scan",
		"severity":"high",
		"score":0.9,
		"source_ip":"192.0.2.10",
		"dest_ip":"198.51.100.20",
		"related_alert_count":7,
		"asset_risk":"high"
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	body := decodeAdvancedBody(t, rr)
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data object: %#v", body)
	}
	if data["playbook"] != "isolate-host" {
		t.Fatalf("playbook=%v want isolate-host", data["playbook"])
	}
	if data["alert_id"] != "alert-42" {
		t.Fatalf("alert_id=%v want alert-42", data["alert_id"])
	}
	if data["success_actions"] != float64(2) || data["failed_actions"] != float64(0) {
		t.Fatalf("unexpected action counts: %#v", data)
	}
}

func TestAdvancedHandlerPlaybookExecutionsLimitValidation(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, newAdvancedTestPlaybookEngine(t), nil, nil))

	for _, path := range []string{"/api/v1/playbooks/executions?limit=0", "/api/v1/playbooks/executions?limit=abc"} {
		rr := doAdvancedRequest(t, router, http.MethodGet, path, "")
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d want=%d body=%s", path, rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	}

	rr := doAdvancedRequest(t, router, http.MethodGet, "/api/v1/playbooks/executions?limit=2", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := decodeAdvancedBody(t, rr)
	data := body["data"].(map[string]interface{})
	if data["total"] != float64(0) {
		t.Fatalf("total=%v want 0", data["total"])
	}
}

func TestAdvancedHandlerNotificationSettingsRejectInlineSecrets(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, nil, nil))

	rr := doAdvancedRequestWithPermissions(t, router, http.MethodPut, "/api/v1/notifications/settings", `{
		"enabled":false,
		"channels":{"email":true},
		"secret_ref":"traffic-analysis/notification-secret"
	}`, []string{"admin"}, []string{"admin:*"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := decodeAdvancedBody(t, rr)
	data := body["data"].(map[string]interface{})
	if data["enabled"] != false {
		t.Fatalf("enabled=%v want false", data["enabled"])
	}
	channels := data["channels"].(map[string]interface{})
	if channels["email"] != true || channels["webhook"] != false {
		t.Fatalf("channels were not merged as expected: %#v", channels)
	}
	if data["secret_ref"] != "traffic-analysis/notification-secret" {
		t.Fatalf("secret_ref=%v", data["secret_ref"])
	}

	rr = doAdvancedRequestWithPermissions(t, router, http.MethodPut, "/api/v1/notifications/settings", `{"webhook_token":"plain-text-token"}`, []string{"admin"}, []string{"admin:*"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	rr = doAdvancedRequestWithPermissions(t, router, http.MethodPut, "/api/v1/notifications/settings", `{"enabled":false}`, []string{"viewer"}, []string{"user:read"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestAdvancedHandlerTestNotification(t *testing.T) {
	notifier := notification.NewNotificationService(notification.NotifyConfig{
		MinSeverity:     "high",
		RateLimitPerMin: 10,
	}, zap.NewNop())
	router := newAdvancedTestRouter(NewAdvancedHandler(notifier, nil, nil, nil, nil))

	rr := doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/notifications/test", "", []string{"admin"}, []string{"admin:*"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := decodeAdvancedBody(t, rr)
	if body["message"] != "test notification sent" {
		t.Fatalf("message=%v", body["message"])
	}

	rr = doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/notifications/test", "", []string{"viewer"}, []string{"user:read"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestAdvancedHandlerNotificationSilenceRuleValidation(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, nil, nil))

	rr := doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/notifications/silence-rules", `{
		"name":"夜间核心交换机维护",
		"scope":"主园区",
		"starts_at":"2026-06-30T23:00:00Z",
		"ends_at":"2026-06-30T22:00:00Z",
		"affected_targets":["core-switch"],
		"policy":"night-escalation",
		"reason":"维护窗口"
	}`, []string{"admin"}, []string{"admin:*"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	rr = doAdvancedRequestWithPermissions(t, router, http.MethodPost, "/api/v1/notifications/silence-rules", `{
		"name":"夜间核心交换机维护",
		"scope":"主园区",
		"starts_at":"2026-06-30T22:00:00Z",
		"ends_at":"2026-07-01T02:00:00Z",
		"affected_targets":["core-switch"],
		"policy":"night-escalation",
		"reason":"维护窗口"
	}`, []string{"viewer"}, []string{"user:read"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
)

type testClaims struct {
	userID      string
	tenantID    string
	username    string
	roles       []string
	permissions []string
}

func (c testClaims) GetUserID() string        { return c.userID }
func (c testClaims) GetTenantID() string      { return c.tenantID }
func (c testClaims) GetUsername() string      { return c.username }
func (c testClaims) GetRoles() []string       { return c.roles }
func (c testClaims) GetPermissions() []string { return c.permissions }
func (c testClaims) GetEmail() string         { return c.username + "@local" }
func (c testClaims) GetSessionID() string     { return "test-session" }
func (c testClaims) HasRole(role string) bool { return containsString(c.roles, role) }
func (c testClaims) HasPermission(permission string) bool {
	for _, granted := range c.permissions {
		if permissionMatches(granted, permission) {
			return true
		}
	}
	return false
}

func TestGenerateComplianceReportRequiresAdminPermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	body := strings.NewReader(`{"report_type":"weekly"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/reports/generate", body)
	req = requestWithClaims(req, viewerClaims())

	recorder := httptest.NewRecorder()
	handler.GenerateComplianceReport(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected viewer to be rejected before report generation, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "admin:* required") {
		t.Fatalf("expected admin permission error, got body %s", recorder.Body.String())
	}
}

func TestTopicGovernanceRequiresTopicWritePermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	cases := []struct {
		name   string
		method string
		path   string
		body   string
		call   func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "save view",
			method: http.MethodPost,
			path:   "/api/v1/topics/views",
			body:   `{"topic":"tunnel","name":"viewer should fail"}`,
			call:   handler.SaveTopicView,
		},
		{
			name:   "update view",
			method: http.MethodPatch,
			path:   "/api/v1/topics/views/view-001",
			body:   `{"favorite":true}`,
			call:   handler.UpdateTopicView,
		},
		{
			name:   "update scope",
			method: http.MethodPut,
			path:   "/api/v1/topics/scopes/tunnel",
			body:   `{"scope_name":"viewer should fail"}`,
			call:   handler.UpdateTopicScope,
		},
		{
			name:   "create subscription",
			method: http.MethodPost,
			path:   "/api/v1/topics/subscriptions",
			body:   `{"topic":"tunnel","recipients":["ops"]}`,
			call:   handler.CreateTopicSubscription,
		},
		{
			name:   "update subscription",
			method: http.MethodPatch,
			path:   "/api/v1/topics/subscriptions/sub-001",
			body:   `{"enabled":false}`,
			call:   handler.UpdateTopicSubscription,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req = requestWithClaims(req, viewerClaims())
			recorder := httptest.NewRecorder()

			tc.call(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("expected viewer to be rejected, got status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "topic:write required") {
				t.Fatalf("expected topic write permission error, got body %s", recorder.Body.String())
			}
		})
	}
}

func TestTopicExportsRequireTopicExportPermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	cases := []struct {
		name string
		path string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "report", path: "/api/v1/topics/reports/export", call: handler.ExportTopicReport},
		{name: "evidence package", path: "/api/v1/topics/evidence-packages/export", call: handler.ExportTopicEvidencePackage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{"topic":"tunnel"}`))
			req = requestWithClaims(req, viewerClaims())
			recorder := httptest.NewRecorder()

			tc.call(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("expected viewer to be rejected, got status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "topic:export required") {
				t.Fatalf("expected topic export permission error, got body %s", recorder.Body.String())
			}
		})
	}
}

func TestTopicGovernanceAdminReachesPostgresGate(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/views", strings.NewReader(`{"topic":"tunnel","name":"admin reaches pg gate"}`))
	req = requestWithClaims(req, adminClaims())

	recorder := httptest.NewRecorder()
	handler.SaveTopicView(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected admin to pass permission and hit postgres gate, got status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestBehaviorBaselineResetRequiresAlertWritePermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/ip:10.12.4.12/reset", nil)
	req = requestWithClaims(req, viewerClaims())

	recorder := httptest.NewRecorder()
	handler.ResetBehaviorBaseline(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected viewer to be rejected, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "alert:write required") {
		t.Fatalf("expected alert write permission error, got body %s", recorder.Body.String())
	}
}

func TestBehaviorBaselineResetAdminReachesPostgresGate(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/ip:10.12.4.12/reset", nil)
	req = requestWithClaims(req, adminClaims())

	recorder := httptest.NewRecorder()
	handler.ResetBehaviorBaseline(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected admin to pass permission and hit postgres gate, got status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestEncryptedTrafficEgressActionRequiresAlertWritePermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/encrypted-traffic/egress-actions", strings.NewReader(`{"action":"create_alert","target":"203.0.113.45","data_mode":"simulated"}`))
	req = requestWithClaims(req, viewerClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitEncryptedTrafficEgressAction(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected viewer to be rejected, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "alert:write required") {
		t.Fatalf("expected alert write permission error, got body %s", recorder.Body.String())
	}
}

func TestEncryptedTrafficEgressActionAdminReachesPostgresGate(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/encrypted-traffic/egress-actions", strings.NewReader(`{"action":"create_alert","target":"203.0.113.45","data_mode":"simulated"}`))
	req = requestWithClaims(req, adminClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitEncryptedTrafficEgressAction(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected admin to pass permission and hit postgres gate, got status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestEncryptedTrafficEvidenceActionRequiresAlertWritePermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/encrypted-traffic/evidence-actions", strings.NewReader(`{"action":"create_task","target":"session-001","data_mode":"simulated"}`))
	req = requestWithClaims(req, viewerClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitEncryptedTrafficEvidenceAction(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected viewer to be rejected, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "alert:write required") {
		t.Fatalf("expected alert write permission error, got body %s", recorder.Body.String())
	}
}

func TestEncryptedTrafficEvidenceActionAdminReachesPostgresGate(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/encrypted-traffic/evidence-actions", strings.NewReader(`{"action":"create_task","target":"session-001","data_mode":"simulated"}`))
	req = requestWithClaims(req, adminClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitEncryptedTrafficEvidenceAction(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected admin to pass permission and hit postgres gate, got status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestEncryptedTrafficEvidenceActionSupportsEvidenceClosureActions(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	for _, action := range []string{"associate_analysis", "preserve_evidence", "link_alert", "expert_review", "mark_gap", "submit_recommendation", "export_report"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/encrypted-traffic/evidence-actions", strings.NewReader(`{"action":"`+action+`","target":"session-001","data_mode":"simulated"}`))
		req = requestWithClaims(req, adminClaims())
		recorder := httptest.NewRecorder()

		handler.SubmitEncryptedTrafficEvidenceAction(recorder, req)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected %s to pass validation and hit postgres gate, got status %d body %s", action, recorder.Code, recorder.Body.String())
		}
	}
}

func TestEncryptedTrafficEvidenceActionRejectsUnsupportedDataMode(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/encrypted-traffic/evidence-actions", strings.NewReader(`{"action":"create_task","target":"session-001","data_mode":"invented"}`))
	req = requestWithClaims(req, adminClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitEncryptedTrafficEvidenceAction(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid data mode to be rejected before postgres, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "unsupported evidence data_mode") {
		t.Fatalf("expected data mode validation error, got body %s", recorder.Body.String())
	}
}

func TestEncryptedEvidenceAnomalyTrendPreservesSessionBuckets(t *testing.T) {
	trend := encryptedEvidenceAnomalyTrend([]encryptedTrafficSessionDTO{
		{StartTime: 1735689600000, AnomalyScore: 0.41},
		{StartTime: 1735689900000, AnomalyScore: 0.76},
	})

	if len(trend) != 2 {
		t.Fatalf("expected two anomaly buckets, got %d", len(trend))
	}
	if trend[1].BucketStart != 1735689900000 || trend[1].AnomalyScore != 0.76 {
		t.Fatalf("unexpected entropy bucket: %#v", trend[1])
	}
}

func TestFusionValueReportNoDependenciesReturnsGatedReport(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fusion/value-report?window_hours=24", nil)
	req = requestWithClaims(req, adminClaims())
	recorder := httptest.NewRecorder()

	handler.GetFusionValueReport(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected value report to be available without live dependencies, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{"fusion-value-ablation-v1", "single_source_baseline", "multi_source", "quality_gates", "source_coverage"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected response body to include %q, got %s", expected, body)
		}
	}
}

func TestTopicGovernanceRoutesAreRegisteredUnderAPIV1(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	handler.RegisterRoutes(apiRouter)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "views list", method: http.MethodGet, path: "/api/v1/topics/views"},
		{name: "views create", method: http.MethodPost, path: "/api/v1/topics/views", body: `{"topic":"tunnel","name":"route"}`},
		{name: "view update", method: http.MethodPatch, path: "/api/v1/topics/views/00000000-0000-0000-0000-000000000001", body: `{"favorite":true}`},
		{name: "scope update", method: http.MethodPut, path: "/api/v1/topics/scopes/tunnel", body: `{"scope_name":"route"}`},
		{name: "subscriptions list", method: http.MethodGet, path: "/api/v1/topics/subscriptions"},
		{name: "subscription create", method: http.MethodPost, path: "/api/v1/topics/subscriptions", body: `{"topic":"tunnel","recipients":["ops"]}`},
		{name: "subscription update", method: http.MethodPatch, path: "/api/v1/topics/subscriptions/00000000-0000-0000-0000-000000000001", body: `{"enabled":false}`},
		{name: "report export", method: http.MethodPost, path: "/api/v1/topics/reports/export", body: `{"topic":"tunnel"}`},
		{name: "evidence package export", method: http.MethodPost, path: "/api/v1/topics/evidence-packages/export", body: `{"topic":"tunnel"}`},
		{name: "fusion value report", method: http.MethodGet, path: "/api/v1/fusion/value-report?window_hours=24"},
		{name: "baseline reset", method: http.MethodPost, path: "/api/v1/baselines/ip:10.12.4.12/reset"},
		{name: "encrypted egress action", method: http.MethodPost, path: "/api/v1/encrypted-traffic/egress-actions", body: `{"action":"create_alert","target":"203.0.113.45","data_mode":"simulated"}`},
		{name: "encrypted evidence action", method: http.MethodPost, path: "/api/v1/encrypted-traffic/evidence-actions", body: `{"action":"create_task","target":"session-001","data_mode":"simulated"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req = requestWithClaims(req, adminClaims())
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			if recorder.Code == http.StatusNotFound {
				t.Fatalf("expected route %s %s to be registered, got 404 body %s", tc.method, tc.path, recorder.Body.String())
			}
		})
	}
}

func TestEncryptedEvidenceCompletenessDoesNotTreatUnlinkedPcapAsSessionEvidence(t *testing.T) {
	sessions := []encryptedTrafficSessionDTO{
		{SessionID: "session-1", SrcIP: "10.0.0.1", DstIP: "203.0.113.10", HasHandshakeMetadata: true},
		{SessionID: "session-2", SrcIP: "10.0.0.2", DstIP: "203.0.113.11", PcapIndex: "pcap/session-2.pcap"},
	}
	pcaps := []encryptedEvidencePcapDTO{
		{FileKey: "pcap/unlinked-1.pcap", SHA256: "hash-1"},
		{FileKey: "pcap/unlinked-2.pcap", SHA256: "hash-2"},
	}

	items := encryptedEvidenceCompleteness(sessions, pcaps)
	byLabel := make(map[string]encryptedEvidenceCompletenessDTO, len(items))
	for _, item := range items {
		byLabel[item.Label] = item
	}

	if got := byLabel["PCAP关联"]; got.Complete != 1 || got.Total != 2 {
		t.Fatalf("expected only the explicitly linked session to count, got %+v", got)
	}
	if got := byLabel["索引Hash"]; got.Complete != 2 || got.Total != 2 {
		t.Fatalf("expected independent index hashes to remain observable, got %+v", got)
	}
}

func requestWithClaims(req *http.Request, claims testClaims) *http.Request {
	ctx := context.WithValue(req.Context(), httpx.ContextKeyClaims, claims)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, claims.userID)
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, claims.tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyRoles, claims.roles)
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, claims.permissions)
	return req.WithContext(ctx)
}

func viewerClaims() testClaims {
	return testClaims{
		userID:      "00000000-0000-0000-0000-000000000001",
		tenantID:    "default",
		username:    "codex-viewer",
		roles:       []string{"viewer"},
		permissions: []string{"user:read", "audit:read"},
	}
}

func adminClaims() testClaims {
	return testClaims{
		userID:      "00000000-0000-0000-0000-000000000002",
		tenantID:    "default",
		username:    "codex-admin",
		roles:       []string{"admin"},
		permissions: []string{"*", "admin:*", "topic:write", "topic:export"},
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

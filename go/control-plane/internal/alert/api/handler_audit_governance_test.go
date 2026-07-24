package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
)

func TestAuditGovernanceReadPermissionsFailClosed(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	for _, test := range []struct {
		name string
		path string
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "list", path: "/api/v1/audit/logs", call: handler.ListAuditLogs},
		{name: "detail", path: "/api/v1/audit/logs/audit-1", call: handler.GetAuditLog},
	} {
		t.Run(test.name+" rejects missing scope", func(t *testing.T) {
			req := auditGovernanceRequest(http.MethodGet, test.path, "", []string{"viewer"}, []string{"alert:read"})
			if test.name == "detail" {
				req = mux.SetURLVars(req, map[string]string{"id": "audit-1"})
			}
			rr := httptest.NewRecorder()
			test.call(rr, req)
			if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "audit:read required") {
				t.Fatalf("expected audit read rejection, got %d %s", rr.Code, rr.Body.String())
			}
		})
		t.Run(test.name+" requires postgres after scope", func(t *testing.T) {
			req := auditGovernanceRequest(http.MethodGet, test.path, "", []string{"auditor"}, []string{authmodel.ScopeAuditRead})
			if test.name == "detail" {
				req = mux.SetURLVars(req, map[string]string{"id": "audit-1"})
			}
			rr := httptest.NewRecorder()
			test.call(rr, req)
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected fail-closed postgres status, got %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAuditGovernanceWriteScopesAreSeparated(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	tests := []struct {
		name       string
		path       string
		body       string
		call       func(http.ResponseWriter, *http.Request)
		permission string
	}{
		{name: "saved query", path: "/api/v1/audit/saved-queries", body: `{"name":"high risk","filters":{"risk":"high"}}`, call: handler.CreateAuditSavedQuery, permission: authmodel.ScopeAuditWrite},
		{name: "review", path: "/api/v1/audit/reviews", body: `{"audit_log_id":"audit-1","decision":"approved"}`, call: handler.CreateAuditReview, permission: authmodel.ScopeAuditWrite},
		{name: "integrity", path: "/api/v1/audit/integrity-checks", body: `{}`, call: handler.CreateAuditIntegrityCheck, permission: authmodel.ScopeAuditWrite},
		{name: "export", path: "/api/v1/audit/exports", body: `{"format":"json","filters":{}}`, call: handler.CreateAuditExport, permission: authmodel.ScopeAuditExport},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			readOnly := auditGovernanceRequest(http.MethodPost, test.path, test.body, []string{"auditor"}, []string{authmodel.ScopeAuditRead})
			rr := httptest.NewRecorder()
			test.call(rr, readOnly)
			if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), test.permission+" required") {
				t.Fatalf("expected %s rejection, got %d %s", test.permission, rr.Code, rr.Body.String())
			}

			allowed := auditGovernanceRequest(http.MethodPost, test.path, test.body, []string{"auditor"}, []string{test.permission})
			rr = httptest.NewRecorder()
			test.call(rr, allowed)
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected database fail-closed response after permission, got %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAuditLogFiltersIncludeGovernanceFieldsAndTenant(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs?log_id=audit-1&action=WHITELIST_*&result=failure&risk=high&request_id=req-1&trace_id=trace-1&start=2026-07-01T00:00:00Z&end=1782950400000", nil)
	filters, err := auditFiltersFromRequest(request)
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	where, args := buildAuditLogWhere("tenant-a", filters)
	for _, fragment := range []string{"tenant_id=$1", "event_id=$2 OR id::text=$2", "action LIKE", "NULLIF(result", "NULLIF(risk_level", "NULLIF(request_id", "NULLIF(trace_id", "created_at >=", "created_at <="} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("where clause missing %q: %s", fragment, where)
		}
	}
	if args[0] != "tenant-a" || args[1] != "audit-1" || args[2] != "WHITELIST_%" {
		t.Fatalf("tenant or wildcard argument mismatch: %#v", args)
	}
}

func TestAuditLogFilterRejectsInvalidRangeAndRisk(t *testing.T) {
	for _, target := range []string{
		"/api/v1/audit/logs?risk=severe",
		"/api/v1/audit/logs?start=2026-07-02T00:00:00Z&end=2026-07-01T00:00:00Z",
		"/api/v1/audit/logs?start=yesterday",
	} {
		if _, err := auditFiltersFromRequest(httptest.NewRequest(http.MethodGet, target, nil)); err == nil {
			t.Fatalf("expected invalid filter for %s", target)
		}
	}
}

func TestAuditGovernanceMutationPayloadAliases(t *testing.T) {
	reviewRequest := httptest.NewRequest(http.MethodPost, "/api/v1/audit/reviews", strings.NewReader(`{"log_id":"audit-1","reason":"高风险操作复核"}`))
	var review auditReviewRequest
	if err := decodeAuditGovernanceJSON(reviewRequest, &review); err != nil {
		t.Fatalf("decode review aliases: %v", err)
	}
	if review.LogID != "audit-1" || review.Reason != "高风险操作复核" {
		t.Fatalf("review aliases not preserved: %#v", review)
	}

	integrityRequest := httptest.NewRequest(http.MethodPost, "/api/v1/audit/integrity-checks", strings.NewReader(`{"filters":{"object_type":"rule","start":1782864000000,"end":1782950400000}}`))
	var integrity auditIntegrityRequest
	if err := decodeAuditGovernanceJSON(integrityRequest, &integrity); err != nil {
		t.Fatalf("decode nested integrity filters: %v", err)
	}
	if integrity.Filters.ObjectType != "rule" || integrity.Filters.Start != "1782864000000" || integrity.Filters.End != "1782950400000" {
		t.Fatalf("nested filters not preserved: %#v", integrity.Filters)
	}
}

func TestBuildAuditExportArtifacts(t *testing.T) {
	logs := []auditGovernanceLog{{
		LogID: "audit-1", TenantID: "tenant-a", UserID: "user-1", Action: "RULE_PUBLISHED",
		ResourceType: "rule", ResourceID: "rule-1", Result: "success", Risk: "high",
		RequestID: "req-real", TraceID: "trace-real", UserAgent: "Mozilla/5.0", Timestamp: time.Now().UnixMilli(),
		Details: map[string]interface{}{"before": "draft", "after": "published"},
	}}
	jsonArtifact, mimeType, extension, err := buildAuditExportArtifact("json", "tenant-a", auditLogFilters{Risk: "high"}, logs)
	if err != nil || mimeType != "application/json" || extension != "json" {
		t.Fatalf("build JSON export: mime=%s extension=%s err=%v", mimeType, extension, err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(jsonArtifact, &payload); err != nil || payload["tenant_id"] != "tenant-a" {
		t.Fatalf("invalid JSON export: %v payload=%v", err, payload)
	}

	csvArtifact, _, extension, err := buildAuditExportArtifact("csv", "tenant-a", auditLogFilters{}, logs)
	if err != nil || extension != "csv" {
		t.Fatalf("build CSV export: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(csvArtifact)).ReadAll()
	if err != nil || len(records) != 2 || records[1][8] != "req-real" || records[1][11] != "Mozilla/5.0" || !strings.Contains(records[1][12], `"before":"draft"`) {
		t.Fatalf("CSV did not preserve request/user-agent fields: records=%v err=%v", records, err)
	}

	pdfArtifact, mimeType, extension, err := buildAuditExportArtifact("pdf", "tenant-a", auditLogFilters{}, logs)
	if err != nil || mimeType != "application/pdf" || extension != "pdf" || !bytes.HasPrefix(pdfArtifact, []byte("%PDF-1.4")) || !bytes.HasSuffix(pdfArtifact, []byte("%%EOF\n")) || !bytes.Contains(pdfArtifact, []byte(`details={"after":"published","before":"draft"}`)) {
		t.Fatalf("invalid PDF export: mime=%s extension=%s err=%v", mimeType, extension, err)
	}
}

func TestAuditExportMasksSensitiveFieldsWithoutMutatingSource(t *testing.T) {
	logs := []auditGovernanceLog{{LogID: "audit-1", IPAddress: "10.0.0.8", UserAgent: "Mozilla/5.0", Details: map[string]interface{}{"source_ip": "10.0.0.9", "remote_ip": "10.0.0.10", "peer-ip": "10.0.0.11", "x_forwarded_for": "10.0.0.12", "nested": map[string]interface{}{"user_agent": "curl/8", "http_user_agent": "probe/1"}, "safe": "kept"}}}
	masked := maskAuditExportLogs(logs, true)
	nested, _ := masked[0].Details["nested"].(map[string]interface{})
	if masked[0].IPAddress != "***masked***" || masked[0].UserAgent != "***masked***" || masked[0].Details["source_ip"] != "***masked***" || masked[0].Details["remote_ip"] != "***masked***" || masked[0].Details["peer-ip"] != "***masked***" || masked[0].Details["x_forwarded_for"] != "***masked***" || nested["user_agent"] != "***masked***" || nested["http_user_agent"] != "***masked***" || masked[0].Details["safe"] != "kept" {
		t.Fatalf("sensitive export fields were not masked: %#v", masked[0])
	}
	sourceNested, _ := logs[0].Details["nested"].(map[string]interface{})
	if logs[0].IPAddress != "10.0.0.8" || logs[0].UserAgent != "Mozilla/5.0" || logs[0].Details["source_ip"] != "10.0.0.9" || sourceNested["user_agent"] != "curl/8" {
		t.Fatalf("masking mutated source logs: %#v", logs[0])
	}
}

func TestAuditGovernancePDFPaginatesLargeExports(t *testing.T) {
	logs := make([]auditGovernanceLog, 150)
	for index := range logs {
		logs[index] = auditGovernanceLog{LogID: fmt.Sprintf("audit-%03d", index), Timestamp: time.Now().UnixMilli()}
	}
	pdf := buildAuditGovernancePDF("tenant-a", logs)
	if pages := bytes.Count(pdf, []byte("/Type /Page ")); pages < 3 {
		t.Fatalf("expected multi-page PDF, got %d pages", pages)
	}
}

func TestAuditGovernanceRoutesAreRegistered(t *testing.T) {
	router := mux.NewRouter()
	handler := NewSystemHandler(nil, nil, nil)
	handler.RegisterRoutes(router)
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/audit/logs/audit-1"},
		{http.MethodPost, "/audit/saved-queries"},
		{http.MethodPost, "/audit/exports"},
		{http.MethodPost, "/audit/reviews"},
		{http.MethodPost, "/audit/integrity-checks"},
	} {
		match := &mux.RouteMatch{}
		if !router.Match(httptest.NewRequest(route.method, route.path, nil), match) || match.MatchErr != nil {
			t.Fatalf("route not registered: %s %s err=%v", route.method, route.path, match.MatchErr)
		}
	}
}

func auditGovernanceRequest(method, path, body string, roles, permissions []string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := req.Context()
	ctx = contextWithAuditClaims(ctx, roles, permissions)
	return req.WithContext(ctx)
}

func contextWithAuditClaims(ctx context.Context, roles, permissions []string) context.Context {
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-test")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "11111111-1111-1111-1111-111111111111")
	ctx = context.WithValue(ctx, httpx.ContextKeyRoles, roles)
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyRequestID, "req-test")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-test")
	return ctx
}

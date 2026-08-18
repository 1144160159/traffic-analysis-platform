package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
)

type testClaims struct {
	userID      string
	tenantID    string
	username    string
	roles       []string
	permissions []string
}

type encryptedTrafficStatsServiceFunc func(context.Context, encryptedTrafficStatsQuery) (encryptedTrafficStatsDTO, error)

func (fn encryptedTrafficStatsServiceFunc) Load(ctx context.Context, query encryptedTrafficStatsQuery) (encryptedTrafficStatsDTO, error) {
	return fn(ctx, query)
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
	if !strings.Contains(recorder.Body.String(), "compliance:write required") {
		t.Fatalf("expected compliance write permission error, got body %s", recorder.Body.String())
	}
}

func TestFusionSourceValueMatchesCanonicalFacts(t *testing.T) {
	sourceValues := []map[string]interface{}{
		{"source": "Flow 流量", "value": "WEB-SRV-01"},
		{"source": "CMDB 资产库", "value": "web-srv-01"},
	}
	for _, tc := range []struct {
		name   string
		source string
		value  string
		want   bool
	}{
		{name: "exact canonical pair", source: "CMDB 资产库", value: "web-srv-01", want: true},
		{name: "client altered value", source: "CMDB 资产库", value: "web-srv-01::repair-required", want: false},
		{name: "value from another source", source: "Flow 流量", value: "web-srv-01", want: false},
		{name: "unknown source", source: "EDR 终端", value: "WEB-SRV-01", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fusionSourceValueMatches(sourceValues, tc.source, tc.value); got != tc.want {
				t.Fatalf("fusionSourceValueMatches() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComplianceReadsRequireDedicatedPermissions(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	for _, tc := range []struct {
		name    string
		path    string
		call    func(http.ResponseWriter, *http.Request)
		message string
	}{
		{name: "reports", path: "/api/v1/compliance/reports", call: handler.ListComplianceReports, message: "compliance:read required"},
		{name: "audit", path: "/api/v1/compliance/audit-trail", call: handler.ListAuditTrail, message: "audit:read required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := requestWithClaims(httptest.NewRequest(http.MethodGet, tc.path, nil), testClaims{userID: "viewer", tenantID: "default", roles: []string{"viewer"}})
			recorder := httptest.NewRecorder()
			tc.call(recorder, req)
			if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), tc.message) {
				t.Fatalf("expected permission rejection %q, got %d %s", tc.message, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestComplianceEvidenceExportRequiresExportPermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := requestWithClaims(httptest.NewRequest(http.MethodPost, "/api/v1/compliance/reports/report-1/evidence-package", nil), viewerClaims())
	recorder := httptest.NewRecorder()
	handler.ExportComplianceEvidencePackage(recorder, req)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "compliance:export required") {
		t.Fatalf("expected compliance export rejection, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestComplianceWorkflowPermissionsFailClosed(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	for _, tc := range []struct {
		name, path, body, message string
		call                      func(http.ResponseWriter, *http.Request)
	}{
		{name: "report export", path: "/api/v1/compliance/reports/report-1/export", body: `{"format":"pdf"}`, message: "compliance:export required", call: handler.ExportComplianceReport},
		{name: "remediation", path: "/api/v1/compliance/reports/report-1/remediations", body: `{}`, message: "compliance:remediate required", call: handler.CreateComplianceRemediations},
		{name: "finalize", path: "/api/v1/compliance/reports/report-1/finalize", body: `{}`, message: "compliance:finalize required", call: handler.FinalizeComplianceReport},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := requestWithClaims(httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)), viewerClaims())
			recorder := httptest.NewRecorder()
			tc.call(recorder, req)
			if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), tc.message) {
				t.Fatalf("expected permission rejection %q, got %d %s", tc.message, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestComplianceZeroEvidenceIsNotPass(t *testing.T) {
	sections := complianceSections(complianceSummaryDTO{})
	if len(sections) == 0 {
		t.Fatal("expected explicit insufficient evidence sections")
	}
	for _, section := range sections {
		if section.Status != "insufficient_evidence" {
			t.Fatalf("zero evidence must not pass: %+v", section)
		}
	}
}

func TestComplianceRangeValidation(t *testing.T) {
	now := time.Now()
	if err := validateComplianceRange(now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), now); err != nil {
		t.Fatalf("valid range rejected: %v", err)
	}
	for _, bounds := range [][2]int64{{now.UnixMilli(), now.Add(-time.Hour).UnixMilli()}, {now.Add(-400 * 24 * time.Hour).UnixMilli(), now.UnixMilli()}, {now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli()}} {
		if err := validateComplianceRange(bounds[0], bounds[1], now); err == nil {
			t.Fatalf("invalid range accepted: %v", bounds)
		}
	}
}

func TestBuildComplianceEvidencePackage(t *testing.T) {
	report := complianceReportDTO{ReportID: "report-1", TenantID: "default", ReportType: "weekly", Status: "completed"}
	canonical, err := canonicalComplianceReportJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	content, checksum, err := buildComplianceEvidencePackage(report)
	if err != nil {
		t.Fatalf("build package: %v", err)
	}
	if !strings.HasPrefix(checksum, "sha256:") || len(content) == 0 {
		t.Fatalf("invalid package metadata: %s %d", checksum, len(content))
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	files := map[string]bool{}
	for _, file := range reader.File {
		files[file.Name] = true
		if file.Name == "manifest.json" {
			rc, openErr := file.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			var manifest map[string]interface{}
			if decodeErr := json.NewDecoder(rc).Decode(&manifest); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			_ = rc.Close()
			wantHash := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
			if manifest["report_sha256"] != wantHash {
				t.Fatalf("manifest report hash=%v want=%s", manifest["report_sha256"], wantHash)
			}
		}
	}
	if !files["manifest.json"] || !files["report.json"] {
		t.Fatalf("package files missing: %v", files)
	}
}

func TestBuildComplianceReportArtifacts(t *testing.T) {
	report := complianceReportDTO{ReportID: "report-1", TenantID: "default", ReportType: "weekly", Status: "non_compliant", Summary: complianceSummaryDTO{TotalAlerts: 10, ResolvedAlerts: 8}, Sections: []complianceSectionDTO{{SectionName: "alert_response", Title: "告警响应闭环", Status: "warning", Content: map[string]interface{}{"total_alerts": 10}}}}
	audits := []complianceAuditLine{{Action: "COMPLIANCE_REPORT_GENERATED", Success: true, CreatedAt: time.Unix(1, 0), Reference: "audit-1"}}
	pdf := buildCompliancePDF(report, audits)
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.Contains(pdf, []byte("Report ID: report-1")) || !bytes.Contains(pdf, []byte("alert_response")) || !bytes.Contains(pdf, []byte("COMPLIANCE_REPORT_GENERATED")) {
		t.Fatalf("invalid PDF artifact: %q", pdf[:min(len(pdf), 32)])
	}
	docx, err := buildComplianceDOCX(report, audits)
	if err != nil {
		t.Fatalf("build DOCX: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("open DOCX: %v", err)
	}
	files := map[string]bool{}
	var documentXML []byte
	for _, file := range reader.File {
		files[file.Name] = true
		if file.Name == "word/document.xml" {
			rc, openErr := file.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			documentXML, _ = io.ReadAll(rc)
			_ = rc.Close()
		}
	}
	if !files["[Content_Types].xml"] || !files["word/document.xml"] {
		t.Fatalf("DOCX files missing: %v", files)
	}
	if !bytes.Contains(documentXML, []byte("alert_response")) || !bytes.Contains(documentXML, []byte("COMPLIANCE_REPORT_GENERATED")) {
		t.Fatalf("DOCX report sections or audit trail missing: %s", documentXML)
	}
}

func TestComplianceSectionsFailClosedAcrossCanonicalGates(t *testing.T) {
	sections := complianceSections(complianceSummaryDTO{})
	if len(sections) < 7 {
		t.Fatalf("canonical sections=%d want at least 7", len(sections))
	}
	for _, section := range sections {
		if section.Status == "pass" {
			t.Fatalf("zero-evidence section %s passed", section.SectionName)
		}
	}
	if status := complianceReportStatus(sections); status != "insufficient_evidence" {
		t.Fatalf("status=%s want insufficient_evidence", status)
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

func TestBehaviorBaselineActionRequiresAlertWritePermission(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/ip:10.12.4.12/actions", strings.NewReader(`{"action":"freeze","reason":"incident containment"}`))
	req = mux.SetURLVars(req, map[string]string{"id": "ip:10.12.4.12"})
	req = requestWithClaims(req, viewerClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitBehaviorBaselineAction(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected viewer to be rejected, got status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "alert:write required") {
		t.Fatalf("expected alert write permission error, got body %s", recorder.Body.String())
	}
}

func TestBehaviorBaselineActionAdminReachesPostgresGate(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/baselines/ip:10.12.4.12/actions", strings.NewReader(`{"action":"adjust_threshold","warning_multiplier":2.0,"alert_multiplier":3.0}`))
	req = mux.SetURLVars(req, map[string]string{"id": "ip:10.12.4.12"})
	req = requestWithClaims(req, adminClaims())
	recorder := httptest.NewRecorder()

	handler.SubmitBehaviorBaselineAction(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected admin to pass permission and hit postgres gate, got status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestBehaviorBaselineListAllowsOneBoundedFiveHundredRowPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/baselines?limit=500&offset=0", nil)
	if behaviorBaselineListMax != 500 {
		t.Fatalf("behavior baseline max=%d want 500", behaviorBaselineListMax)
	}
	limit, offset := parsePageLimitOffset(req, 20, behaviorBaselineListMax)
	if limit != 500 || offset != 0 {
		t.Fatalf("limit=%d offset=%d want one bounded 500-row page", limit, offset)
	}
}

func TestBehaviorBaselineSettingsAreLoadedInTwoBatchQueries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	items := []behaviorBaselineDTO{
		{BaselineID: "asset:10.0.0.1", BaselineType: "asset", EntityID: "10.0.0.1", Status: "active", Version: 1, Metrics: []behaviorMetricDTO{metricDTO("bytes_per_session", "bytes", 1, 0, 1)}},
		{BaselineID: "asset:10.0.0.2", BaselineType: "asset", EntityID: "10.0.0.2", Status: "active", Version: 1, Metrics: []behaviorMetricDTO{metricDTO("bytes_per_session", "bytes", 1, 0, 1)}},
	}
	mock.ExpectQuery("FROM behavior_baseline_resets").
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"baseline_id", "reset_at"}))
	mock.ExpectQuery("FROM behavior_baseline_settings").
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"baseline_id", "warning_multiplier", "alert_multiplier", "frozen", "drift_watch", "version"}).
			AddRow("asset:10.0.0.1", 2.5, 4.0, true, false, 7).
			AddRow("asset:10.0.0.2", 2.0, 3.5, false, true, 5))

	if err := handler.applyBehaviorBaselineSettingsBatch(context.Background(), "tenant-a", items); err != nil {
		t.Fatalf("apply batch settings: %v", err)
	}
	if !items[0].Frozen || items[0].Status != "frozen" || items[0].Version != 7 || items[0].Metrics[0].ThresholdConfig.AlertMultiplier != 4.0 {
		t.Fatalf("unexpected first governed baseline: %+v", items[0])
	}
	if !items[1].DriftWatch || items[1].Status != "drift" || items[1].Version != 5 || items[1].Metrics[0].ThresholdConfig.WarningMultiplier != 2.0 {
		t.Fatalf("unexpected second governed baseline: %+v", items[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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

func TestGetEncryptedTrafficStatsCharacterizesHandlerServiceBoundary(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	var captured encryptedTrafficStatsQuery
	handler.encryptedTrafficStats = encryptedTrafficStatsServiceFunc(func(_ context.Context, query encryptedTrafficStatsQuery) (encryptedTrafficStatsDTO, error) {
		captured = query
		return encryptedTrafficStatsDTO{
			TotalSessions:       8,
			ObservedSessions:    10,
			EncryptedRatio:      0.8,
			TLSSessions:         5,
			QUICSessions:        2,
			JA3Fingerprints:     4,
			MaliciousJA3Matches: 1,
		}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/encrypted-traffic/stats?tenant_id=untrusted&start_time=1700000000000&end_time=1700003600000", nil)
	req = requestWithClaims(req, testClaims{tenantID: "tenant-a"})
	recorder := httptest.NewRecorder()

	handler.GetEncryptedTrafficStats(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if captured != (encryptedTrafficStatsQuery{TenantID: "tenant-a", StartMilli: 1700000000000, EndMilli: 1700003600000}) {
		t.Fatalf("unexpected service query: %+v", captured)
	}
	var response struct {
		Success bool                     `json:"success"`
		Data    encryptedTrafficStatsDTO `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.TotalSessions != 8 || response.Data.ObservedSessions != 10 || response.Data.EncryptedRatio != 0.8 || response.Data.JA3Fingerprints != 4 || response.Data.MaliciousJA3Matches != 1 {
		t.Fatalf("unexpected response: %+v body %s", response, recorder.Body.String())
	}
}

func TestGetEncryptedTrafficStatsPreservesInternalErrorEnvelope(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	handler.encryptedTrafficStats = encryptedTrafficStatsServiceFunc(func(context.Context, encryptedTrafficStatsQuery) (encryptedTrafficStatsDTO, error) {
		return encryptedTrafficStatsDTO{}, fmt.Errorf("stats read failed")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/encrypted-traffic/stats?start=1700000000000&end=1700003600000", nil)
	req = requestWithClaims(req, testClaims{tenantID: "tenant-a"})
	recorder := httptest.NewRecorder()

	handler.GetEncryptedTrafficStats(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"INTERNAL"`) || !strings.Contains(recorder.Body.String(), "stats read failed") {
		t.Fatalf("unexpected error envelope: %s", recorder.Body.String())
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

func TestFusionWriteRequestsCarryOptimisticVersions(t *testing.T) {
	conflictVersion := int64(7)
	conflict := fusionConflictResolveRequest{ExpectedStateVersion: &conflictVersion, FieldName: " 主机名 ", SelectedSource: " CMDB ", SelectedValue: " srv-12 "}
	conflict.normalize()
	if conflict.ExpectedStateVersion == nil || *conflict.ExpectedStateVersion != 7 || conflict.FieldName != "主机名" || conflict.SelectedSource != "CMDB" {
		t.Fatalf("unexpected normalized conflict request: %+v", conflict)
	}
	ruleVersion := int64(3)
	threshold := 1.4
	rule := fusionRuleUpdateRequest{ExpectedVersion: &ruleVersion, ConfidenceThreshold: &threshold}
	rule.normalize("IP_MAC_BIND_V3")
	if rule.ExpectedVersion == nil || *rule.ExpectedVersion != 3 || *rule.ConfidenceThreshold != 1.4 || rule.RuleName != "IP_MAC_BIND_V3" {
		t.Fatalf("unexpected normalized rule request: %+v", rule)
	}
}

func TestFusionRuleCanonicalEnums(t *testing.T) {
	for _, status := range []string{"active", "draft", "disabled"} {
		if !validFusionRuleStatus(status) {
			t.Fatalf("expected status %q to be accepted", status)
		}
	}
	if validFusionRuleStatus("client-forged") {
		t.Fatal("unexpected acceptance of forged fusion rule status")
	}
	for _, strategy := range []string{"authoritative-source", "weighted-confidence", "latest-observation", "manual-review"} {
		if !validFusionRuleStrategy(strategy) {
			t.Fatalf("expected strategy %q to be accepted", strategy)
		}
	}
	if validFusionRuleStrategy("client-forged") {
		t.Fatal("unexpected acceptance of forged fusion rule strategy")
	}
}

func TestVersionedFusionResolutionRequiresReasonBeforeDatabaseMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	handler.SetFusionV1FeatureFlag(true)
	body := `{"selected_source":"CMDB","selected_value":"srv-12","strategy":"authoritative-source","expected_state_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fusion/conflicts/conflict-1/resolve", strings.NewReader(body))
	req = mux.SetURLVars(requestWithClaims(req, adminClaims()), map[string]string{"id": "conflict-1"})
	recorder := httptest.NewRecorder()
	handler.ResolveFusionConflict(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "note is required") {
		t.Fatalf("expected missing versioned resolution reason to fail before mutation, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendFusionResolutionV1AppendsHistoryAndPendingOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	previousID := "00000000-0000-0000-0000-000000000101"
	resolutionID := "00000000-0000-0000-0000-000000000102"
	eventID := "00000000-0000-0000-0000-000000000103"
	mock.ExpectQuery(`SELECT resolution_id::text,resolution_version`).
		WithArgs("tenant-a", "conflict-a").
		WillReturnRows(sqlmock.NewRows([]string{"resolution_id", "resolution_version"}).AddRow(previousID, int64(4)))
	mock.ExpectQuery(`INSERT INTO fusion_resolution_history`).
		WithArgs(
			"tenant-a", "conflict-a", int64(5), int64(8), "applied", "CMDB", "srv-12",
			"authoritative-source", "approved source priority", previousID, sqlmock.AnyArg(),
			"00000000-0000-0000-0000-000000000002", "trace-a",
		).
		WillReturnRows(sqlmock.NewRows([]string{"resolution_id"}).AddRow(resolutionID))
	mock.ExpectQuery(`INSERT INTO fusion_projection_outbox`).
		WithArgs("tenant-a", resolutionID, int64(5), "tenant-a:conflict-a", sqlmock.AnyArg(), sqlmock.AnyArg(), "trace-a").
		WillReturnRows(sqlmock.NewRows([]string{"event_id"}).AddRow(eventID))
	mock.ExpectRollback()
	dto := fusionConflictResolutionDTO{
		TenantID: "tenant-a", ConflictID: "conflict-a", ObjectID: "asset-a", ObjectType: "asset",
		FieldName: "hostname", SelectedSource: "CMDB", SelectedValue: "srv-12", Strategy: "authoritative-source",
		Note: "approved source priority", RuleID: "rule-a", StateVersion: 8,
		ResolvedBy: "00000000-0000-0000-0000-000000000002", Detail: map[string]interface{}{"source": "test"},
	}
	gotResolutionID, gotVersion, gotEventID, err := appendFusionResolutionV1(
		context.Background(), tx, dto, []map[string]interface{}{{"source": "CMDB", "value": "srv-12"}}, "trace-a",
	)
	if err != nil {
		t.Fatalf("appendFusionResolutionV1: %v", err)
	}
	if gotResolutionID != resolutionID || gotVersion != 5 || gotEventID != eventID {
		t.Fatalf("unexpected versioned receipt: resolution=%s version=%d event=%s", gotResolutionID, gotVersion, gotEventID)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFusionSourceRequiresRuleWritePermissionBeforeDatabase(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fusion/sources/traffic/sync", nil)
	req = mux.SetURLVars(requestWithClaims(req, testClaims{
		userID:      "00000000-0000-0000-0000-000000000003",
		tenantID:    "default",
		username:    "fusion-reader",
		roles:       []string{"viewer"},
		permissions: []string{"graph:read", "rule:read"},
	}), map[string]string{"id": "traffic"})
	recorder := httptest.NewRecorder()

	handler.SyncFusionSource(recorder, req)

	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "rule:write required") {
		t.Fatalf("expected rule:write rejection before database access, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestFusionPaginationQueryBounds(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fusion/workbench?conflict_limit=999&conflict_offset=-5&audit_limit=0&audit_offset=12", nil)
	if got := boundedPositiveIntQuery(req, "conflict_limit", 100, 200); got != 200 {
		t.Fatalf("expected conflict limit cap 200, got %d", got)
	}
	if got := boundedIntQuery(req, "conflict_offset", 0, 1000000); got != 0 {
		t.Fatalf("expected negative conflict offset to normalize to 0, got %d", got)
	}
	if got := boundedPositiveIntQuery(req, "audit_limit", 50, 200); got != 50 {
		t.Fatalf("expected zero audit limit to use default 50, got %d", got)
	}
	if got := boundedIntQuery(req, "audit_offset", 0, 1000000); got != 12 {
		t.Fatalf("expected audit offset 12, got %d", got)
	}
}

func TestFusionEvidenceFilenameSlugIsSafe(t *testing.T) {
	if got := slugIdentifier(" CF/2026 018 "); got != "cf-2026-018" {
		t.Fatalf("unexpected slug %q", got)
	}
}

func TestPostgresAssetVulnerabilitySourceCountsOnlyVulnerabilityItems(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets WHERE tenant_id=$1")).
		WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"total", "recent", "latest"}).AddRow(3, 2, now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets WHERE tenant_id=$1 AND updated_at >= $2")).
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "count"}).AddRow(now.UnixMilli()/fusionTrendBucketMillis*fusionTrendBucketMillis, 2))

	handler := NewSystemHandler(nil, db, nil)
	source := handler.postgresAssetVulnerabilitySource(context.Background(), "tenant-a", now.UnixMilli())
	if source.SourceID != "vulnerability" || source.Config["total_records"] != int64(3) {
		t.Fatalf("unexpected vulnerability source: %+v", source)
	}
	if source.Config["storage"] != "postgres.assets.metadata.vulnerabilities" {
		t.Fatalf("unexpected vulnerability storage: %v", source.Config["storage"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestLoadTopicPanelSimulationUsesDatabasePayloadAndRuntimeContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT to_regclass").
		WillReturnRows(sqlmock.NewRows([]string{"to_regclass"}).AddRow("topic_panel_simulations"))
	mock.ExpectQuery("SELECT simulation_id, version, payload").
		WithArgs("tunnel", "tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"simulation_id", "version", "payload", "updated_at"}).AddRow(
			"topic-tunnel-ui-v1",
			"ui-suite-gpt-v1",
			[]byte(`{"summary":{"protocol_count":7},"updated_at":1}`),
			time.UnixMilli(1718888888000),
		))

	handler := NewSystemHandler(nil, db, nil)
	scope := &topicScopeDTO{TenantID: "tenant-a", Topic: "tunnel", TimeWindow: "7d"}
	payload, ok, err := handler.loadTopicPanelSimulation(context.Background(), "tenant-a", "tunnel", scope, 100, 200)
	if err != nil || !ok {
		t.Fatalf("expected simulation payload, ok=%v err=%v", ok, err)
	}
	if payload["data_mode"] != "simulated" || payload["simulation_id"] != "topic-tunnel-ui-v1" || payload["simulation_version"] != "ui-suite-gpt-v1" {
		t.Fatalf("unexpected simulation metadata: %+v", payload)
	}
	if payload["updated_at"] != int64(1718888888000) {
		t.Fatalf("expected stable database updated_at, got %#v", payload["updated_at"])
	}
	timeRange, ok := payload["time_range"].(map[string]int64)
	if !ok || timeRange["start"] != 100 || timeRange["end"] != 200 {
		t.Fatalf("unexpected runtime time range: %#v", payload["time_range"])
	}
	if payload["scope"] != scope {
		t.Fatalf("expected runtime scope to override fixture scope")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestLoadTopicPanelSimulationFallsBackWhenTableIsAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT to_regclass").
		WillReturnRows(sqlmock.NewRows([]string{"to_regclass"}).AddRow(nil))

	handler := NewSystemHandler(nil, db, nil)
	payload, ok, err := handler.loadTopicPanelSimulation(context.Background(), "tenant-a", "exfil", nil, 100, 200)
	if err != nil || ok || payload != nil {
		t.Fatalf("expected live-data fallback, payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
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
		{name: "topic action", method: http.MethodPost, path: "/api/v1/topics/tunnel/actions", body: `{"action":"extract_pcap","target":"10.12.8.45","data_mode":"live"}`},
		{name: "apt evidence action", method: http.MethodPost, path: "/api/v1/topics/apt/evidence-actions", body: `{"action":"trace","target":"ioc-1","data_mode":"live"}`},
		{name: "fusion value report", method: http.MethodGet, path: "/api/v1/fusion/value-report?window_hours=24"},
		{name: "fusion workbench", method: http.MethodGet, path: "/api/v1/fusion/workbench"},
		{name: "fusion evidence export", method: http.MethodPost, path: "/api/v1/fusion/evidence-packages", body: `{"conflict_id":"CF-20260625-018"}`},
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

func TestSummarizeAPTTopicCampaignsUsesListedCampaignEvidence(t *testing.T) {
	campaigns := []campaignDTO{
		{
			CampaignID:     "apt-1",
			Entities:       []string{"asset-1", "asset-2", "asset-2"},
			Alerts:         []string{"alert-1", "alert-2"},
			Score:          0.9,
			AttackPhases:   []string{"initial_access", "persistence", "lateral_movement", "exfiltration"},
			Status:         "closed",
			ActivityStatus: "active",
		},
		{
			CampaignID:     "apt-2",
			Entities:       []string{"asset-1", "asset-3"},
			Alerts:         []string{"alert-3"},
			Score:          0.7,
			AttackPhases:   []string{"execution", "lateral_movement"},
			Status:         "investigating",
			ActivityStatus: "investigating",
		},
	}

	phases, summary := summarizeAPTTopicCampaigns(campaigns, 9)

	if phases["lateral_movement"] != 2 || phases["persistence"] != 1 {
		t.Fatalf("unexpected phase distribution: %#v", phases)
	}
	if summary["campaign_count"] != int64(9) || summary["listed_campaigns"] != 2 {
		t.Fatalf("unexpected campaign scope: %#v", summary)
	}
	if summary["entity_count"] != 3 || summary["alert_count"] != 3 {
		t.Fatalf("unexpected entity/alert counts: %#v", summary)
	}
	if summary["lateral_move_links"] != 2 || summary["persistence_signals"] != 1 || summary["exfil_evidence_count"] != 2 {
		t.Fatalf("unexpected APT evidence metrics: %#v", summary)
	}
	if summary["cluster_density"] != 1.0 || summary["closure_rate"] != 50.0 || math.Abs(summary["report_confidence"].(float64)-80.0) > 0.0001 {
		t.Fatalf("unexpected APT derived rates: %#v", summary)
	}
	if summary["metric_scope"] != "listed_campaigns" || summary["metric_scope_campaigns"] != 2 {
		t.Fatalf("expected listed-campaign scope disclosure: %#v", summary)
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

func TestFillFusionTrendKeepsEightOrderedBuckets(t *testing.T) {
	const nowMillis int64 = 12_345_678_901
	endBucket := nowMillis / fusionTrendBucketMillis * fusionTrendBucketMillis
	points := map[int64]int64{
		endBucket - 7*fusionTrendBucketMillis: 3,
		endBucket - 3*fusionTrendBucketMillis: 9,
		endBucket:                             4,
	}
	trend := fillFusionTrend(points, nowMillis)
	if len(trend) != fusionTrendBucketCount {
		t.Fatalf("expected %d buckets, got %d", fusionTrendBucketCount, len(trend))
	}
	if trend[0] != 3 || trend[4] != 9 || trend[7] != 4 {
		t.Fatalf("unexpected ordered trend: %#v", trend)
	}
}

func TestFusionSourcesExposeUnavailableDependencies(t *testing.T) {
	handler := &SystemHandler{}
	createdAt := time.Now().UnixMilli()
	tests := []struct {
		name      string
		source    dataSourceDTO
		storage   string
		errorCode string
	}{
		{
			name:      "clickhouse not configured",
			source:    handler.clickHouseSource(context.Background(), "default", "traffic", "traffic", "流量元数据", "traffic.sessions", "timestamp", createdAt),
			storage:   "clickhouse.traffic.sessions",
			errorCode: "SOURCE_NOT_CONFIGURED",
		},
		{
			name:      "postgres not configured",
			source:    handler.postgresSource(context.Background(), "default", "threat_intel", "threat_intel", "威胁情报", "threat_intel", "updated_at", createdAt),
			storage:   "postgres.threat_intel",
			errorCode: "SOURCE_NOT_CONFIGURED",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.source.Status != "unavailable" {
				t.Fatalf("expected unavailable status, got %q", tc.source.Status)
			}
			if tc.source.Config["storage"] != tc.storage {
				t.Fatalf("expected storage %q, got %#v", tc.storage, tc.source.Config["storage"])
			}
			if tc.source.Config["error_code"] != tc.errorCode {
				t.Fatalf("expected error code %q, got %#v", tc.errorCode, tc.source.Config["error_code"])
			}
		})
	}
}

func TestTopicActionBusinessEffectReturnsTraceableResult(t *testing.T) {
	tests := []struct {
		name         string
		topic        string
		action       string
		target       string
		dataMode     string
		wantStatus   string
		wantState    string
		wantField    string
		wantContains string
	}{
		{
			name: "simulated containment completes with target state", topic: "tunnel", action: "contain",
			target: "TUN-20260620-001", dataMode: "simulated", wantStatus: "completed", wantState: "completed",
			wantField: "target_state", wantContains: "contained",
		},
		{
			name: "live containment is queued for the execution worker", topic: "tunnel", action: "contain",
			target: "TUN-20260620-001", dataMode: "live", wantStatus: "queued", wantState: "queued",
			wantField: "message", wantContains: "等待执行器",
		},
		{
			name: "trace returns graph route", topic: "apt", action: "trace",
			target: "APT CN/2026", dataMode: "simulated", wantStatus: "completed", wantState: "completed",
			wantField: "next_route", wantContains: "/graph?topic=apt&target=APT+CN%2F2026",
		},
		{
			name: "pcap extraction returns evidence reference", topic: "exfil", action: "extract_pcap",
			target: "EXFIL-001", dataMode: "simulated", wantStatus: "completed", wantState: "completed",
			wantField: "evidence_ref", wantContains: "topic/exfil/pcap/EXFIL-001",
		},
		{
			name: "session inspection returns session evidence", topic: "tunnel", action: "inspect_session",
			target: "TN-20260620-0001", dataMode: "simulated", wantStatus: "completed", wantState: "completed",
			wantField: "evidence_ref", wantContains: "topic/tunnel/session/TN-20260620-0001",
		},
		{
			name: "certificate inspection returns fingerprint evidence", topic: "tunnel", action: "inspect_certificate",
			target: "TN-20260620-0001", dataMode: "simulated", wantStatus: "completed", wantState: "completed",
			wantField: "evidence_ref", wantContains: "topic/tunnel/certificate/TN-20260620-0001",
		},
		{
			name: "trace path returns graph route", topic: "tunnel", action: "trace_path",
			target: "TN-20260620-0001", dataMode: "simulated", wantStatus: "completed", wantState: "completed",
			wantField: "next_route", wantContains: "/graph?topic=tunnel&trace=TN-20260620-0001",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, effect := topicActionBusinessEffect(tc.topic, tc.action, tc.target, tc.dataMode)
			if status != tc.wantStatus {
				t.Fatalf("expected status %q, got %q", tc.wantStatus, status)
			}
			if effect["state"] != tc.wantState {
				t.Fatalf("expected effect state %q, got %#v", tc.wantState, effect["state"])
			}
			if effect["operation"] != tc.action || effect["data_mode"] != tc.dataMode {
				t.Fatalf("effect lost action or data mode: %#v", effect)
			}
			value, ok := effect[tc.wantField].(string)
			if !ok || !strings.Contains(value, tc.wantContains) {
				t.Fatalf("expected %s to contain %q, got %#v", tc.wantField, tc.wantContains, effect[tc.wantField])
			}
		})
	}
}

func TestBuildTopicArtifactContainsSnapshotMetricsAndDistinctZipManifest(t *testing.T) {
	snapshot := map[string]interface{}{
		"topic": "tunnel", "data_mode": "simulated", "simulation_id": "topic-tunnel-ui-v1", "simulation_version": "ui-suite-gpt-v1",
		"summary":         map[string]interface{}{"protocol_count": 7, "session_count": 64, "evidence_completeness": 62},
		"events":          []interface{}{map[string]interface{}{"event_id": "TUN-001"}, map[string]interface{}{"event_id": "TUN-002"}},
		"evidence_bundle": []interface{}{map[string]interface{}{"label": "PCAP", "complete": 42, "total": 64}},
		"presentation": map[string]interface{}{
			"report_title":      "加密隧道专题分析报告",
			"report_scope":      "主校区办公终端",
			"report_conclusion": "发现 64 条异常隧道会话。",
		},
	}
	parameters := map[string]interface{}{"format": "pdf", "data_mode": "simulated"}

	pdf, pdfName, pdfType, err := buildTopicArtifact("tunnel", "report", "pdf", "default", "tester", parameters, snapshot)
	if err != nil {
		t.Fatalf("build pdf: %v", err)
	}
	if !strings.HasSuffix(pdfName, ".pdf") || pdfType != "application/pdf" || len(pdf) < 900 {
		t.Fatalf("expected a populated PDF artifact, name=%q type=%q bytes=%d", pdfName, pdfType, len(pdf))
	}
	for _, marker := range []string{"summary.protocol_count=7", "events.count=2", "evidence_bundle.count=1", "snapshot_sha256=sha256:"} {
		if !bytes.Contains(pdf, []byte(topicPDFUTF16Hex(marker))) {
			t.Fatalf("PDF is missing report marker %q", marker)
		}
	}
	for _, marker := range []string{"加密隧道专题分析报告", "scope=主校区办公终端", "conclusion=发现 64 条异常隧道会话。"} {
		if !bytes.Contains(pdf, []byte(topicPDFUTF16Hex(marker))) {
			t.Fatalf("PDF is missing UTF-16 report content %q", marker)
		}
	}

	docx, _, _, err := buildTopicArtifact("tunnel", "report", "docx", "default", "tester", parameters, snapshot)
	if err != nil {
		t.Fatalf("build docx: %v", err)
	}
	docReader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("open docx: %v", err)
	}
	var documentXML []byte
	for _, file := range docReader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open document.xml: %v", openErr)
		}
		documentXML, err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatalf("read document.xml: %v", err)
		}
	}
	if !bytes.Contains(documentXML, []byte("summary.protocol_count=7")) || !bytes.Contains(documentXML, []byte("TUN-001")) {
		t.Fatalf("DOCX does not contain snapshot metrics and event content")
	}

	artifactZip, _, _, err := buildTopicArtifact("tunnel", "evidence_package", "zip", "default", "tester", parameters, snapshot)
	if err != nil {
		t.Fatalf("build zip: %v", err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(artifactZip), int64(len(artifactZip)))
	if err != nil {
		t.Fatalf("open evidence zip: %v", err)
	}
	entries := map[string][]byte{}
	for _, file := range zipReader.File {
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open %s: %v", file.Name, openErr)
		}
		entries[file.Name], err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
	}
	if bytes.Equal(entries["manifest.json"], entries["snapshot.json"]) {
		t.Fatalf("manifest.json must not duplicate snapshot.json")
	}
	if !bytes.Contains(entries["manifest.json"], []byte("snapshot_sha256")) || !bytes.Contains(entries["snapshot.json"], []byte("TUN-001")) {
		t.Fatalf("ZIP manifest/snapshot content is incomplete")
	}
}

func TestLoadTopicSourceReportSnapshotReusesTenantScopedSnapshotAndHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	snapshot := map[string]interface{}{
		"topic": "exfil",
		"summary": map[string]interface{}{
			"path_count": 112,
		},
		"events": []interface{}{map[string]interface{}{"event_id": "EXF-001"}},
	}
	checksum, err := topicSnapshotChecksum(snapshot)
	if err != nil {
		t.Fatalf("checksum snapshot: %v", err)
	}
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"snapshot":        snapshot,
		"snapshot_sha256": checksum,
		"report_model":    buildTopicReportModel("exfil", snapshot),
	})
	parametersJSON, _ := json.Marshal(map[string]interface{}{
		"data_mode":          "simulated",
		"simulation_id":      "topic-exfil-ui-v2",
		"simulation_version": "ui-suite-gpt-v2",
	})

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT result::text, parameters::text
		FROM topic_exports
		WHERE tenant_id=$1 AND topic=$2 AND export_type='report' AND status='completed' AND export_id=$3`)).
		WithArgs("tenant-a", "exfil", "11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"result", "parameters"}).AddRow(string(resultJSON), string(parametersJSON)))

	reused, parameters, reusedChecksum, err := loadTopicSourceReportSnapshot(
		context.Background(),
		db,
		"tenant-a",
		"exfil",
		"11111111-1111-1111-1111-111111111111",
	)
	if err != nil {
		t.Fatalf("load source report snapshot: %v", err)
	}
	if reusedChecksum != checksum {
		t.Fatalf("expected checksum %q, got %q", checksum, reusedChecksum)
	}
	if fmt.Sprint(reused["topic"]) != "exfil" || fmt.Sprint(parameters["simulation_id"]) != "topic-exfil-ui-v2" {
		t.Fatalf("source snapshot or parameters changed: snapshot=%#v parameters=%#v", reused, parameters)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestLoadTopicSourceReportSnapshotRejectsChecksumDrift(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"snapshot":        map[string]interface{}{"topic": "apt", "summary": map[string]interface{}{"campaign_count": 7}},
		"snapshot_sha256": "sha256:stale",
	})
	mock.ExpectQuery("SELECT result::text, parameters::text").
		WithArgs("tenant-a", "apt", "22222222-2222-2222-2222-222222222222").
		WillReturnRows(sqlmock.NewRows([]string{"result", "parameters"}).AddRow(string(resultJSON), `{}`))

	_, _, _, err = loadTopicSourceReportSnapshot(
		context.Background(),
		db,
		"tenant-a",
		"apt",
		"22222222-2222-2222-2222-222222222222",
	)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestTopicSnapshotChecksumIsStableAcrossJSONBStructRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"topic": "tunnel",
		"scope": &topicScopeDTO{
			TenantID:       "tenant-a",
			Topic:          "tunnel",
			ScopeName:      "核心范围",
			IncludedAssets: []string{"core-switch"},
			RiskLevels:     []string{"high"},
			TimeWindow:     "24h",
			UpdatedAt:      1718888888000,
		},
		"time_range": map[string]int64{"start": 1718800000000, "end": 1718888888000},
	}
	originalChecksum, err := topicSnapshotChecksum(original)
	if err != nil {
		t.Fatalf("checksum original snapshot: %v", err)
	}
	encoded, _ := json.Marshal(original)
	reloaded := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatalf("round trip snapshot: %v", err)
	}
	reloadedChecksum, err := topicSnapshotChecksum(reloaded)
	if err != nil {
		t.Fatalf("checksum reloaded snapshot: %v", err)
	}
	if originalChecksum != reloadedChecksum {
		t.Fatalf("checksum drifted across JSONB-style round trip: original=%s reloaded=%s", originalChecksum, reloadedChecksum)
	}
}

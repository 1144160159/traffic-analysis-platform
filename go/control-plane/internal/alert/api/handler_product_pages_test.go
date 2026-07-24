package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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

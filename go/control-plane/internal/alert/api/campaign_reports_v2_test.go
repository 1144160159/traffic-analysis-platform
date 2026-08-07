package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
)

func deterministicCampaignReportModel() CampaignReportModel {
	return CampaignReportModel{
		SchemaVersion: 2, ContractVersion: 2, SnapshotID: "00000000-0000-4000-8000-000000000001",
		TenantID: "tenant-a", CampaignID: "campaign-a", CampaignRevision: 7,
		Status: "investigating", Assignee: "analyst-a", Summary: "deterministic campaign",
		Score: 91.25, CampaignType: "apt", Entities: []string{"asset-a"},
		AttackPhases: []string{"initial-access"}, RuleIDs: []string{"rule-a"}, ModelIDs: []string{"model-a"},
		MemberAlertIDs: []string{"alert-a", "alert-b"}, MemberCount: 2,
		TimeWindow: CampaignReportTimeWindow{Start: 100, End: 200}, Sections: []string{"summary", "evidence"},
		EvidenceCount: 3, MembershipSource: "postgresql.campaign_alert_links",
		SourceWatermarks: map[string]string{"postgresql.campaign_workbench_state.revision": "7"},
	}
}

func TestCampaignReportArtifactsAreDeterministicAndValid(t *testing.T) {
	model := deterministicCampaignReportModel()
	canonical, _, err := canonicalCampaignSnapshot(&model)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "pdf", "word"} {
		first, mimeType, extension, err := buildCampaignReportArtifact(format, model, canonical)
		if err != nil {
			t.Fatalf("format=%s: %v", format, err)
		}
		second, _, _, err := buildCampaignReportArtifact(format, model, canonical)
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("format=%s is not deterministic: err=%v", format, err)
		}
		if len(first) == 0 || mimeType == "" || extension == "" {
			t.Fatalf("format=%s produced an incomplete artifact", format)
		}
		switch format {
		case "json":
			var decoded CampaignReportModel
			if err := json.Unmarshal(first, &decoded); err != nil || decoded.SnapshotID != model.SnapshotID {
				t.Fatalf("invalid JSON report: decoded=%+v err=%v", decoded, err)
			}
		case "pdf":
			if !bytes.HasPrefix(first, []byte("%PDF-1.4")) {
				t.Fatal("invalid PDF signature")
			}
		case "word":
			reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
			if err != nil || len(reader.File) != 3 {
				t.Fatalf("invalid DOCX: entries=%d err=%v", len(reader.File), err)
			}
		}
	}
}

func campaignReportRequestContext(tenantID string, permissions ...string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "campaign-report-user")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-campaign-report-test")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	return ctx
}

func TestCampaignReportDownloadRejectsMissingPermission(t *testing.T) {
	handler := &SystemHandler{}
	request := httptest.NewRequest(http.MethodGet, "/campaigns/campaign-a/reports/report-a/download", nil)
	request = request.WithContext(campaignReportRequestContext("tenant-a", authmodel.ScopeCampaignRead))
	request = mux.SetURLVars(request, map[string]string{"id": "campaign-a", "report_id": "report-a"})
	recorder := httptest.NewRecorder()
	handler.DownloadCampaignReport(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

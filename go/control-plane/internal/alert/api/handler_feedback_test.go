package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func TestFeedbackWhitelistDraftRequiresAlertWriteBeforeAlertLookup(t *testing.T) {
	handler := NewFeedbackHandler(nil, nil, nil, nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-secret/feedback", strings.NewReader(`{"label":"FP","reason_code":"FALSE_ALARM","add_to_whitelist":true}`))
	req = requestWithClaims(req, viewerClaims())
	req = mux.SetURLVars(req, map[string]string{"id": "AL-secret"})
	recorder := httptest.NewRecorder()

	handler.SubmitFeedback(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "alert:write required") {
		t.Fatalf("expected alert:write denial, got %s", recorder.Body.String())
	}
}

func TestBuildWhitelistDraftEntryFromFeedback(t *testing.T) {
	entry, err := buildWhitelistDraftEntry("tenant-a", "AL-20260629-0001", "FB-1", "sec_analyst", "FALSE_ALARM", &service.AlertDetailDTO{
		AlertDTO: service.AlertDTO{
			SrcIP: "10.12.4.23",
			DstIP: "192.0.2.10",
		},
	})
	if err != nil {
		t.Fatalf("buildWhitelistDraftEntry() error = %v", err)
	}

	if entry.Status != "draft" {
		t.Fatalf("status=%q want draft", entry.Status)
	}
	if entry.SourceAlertID != "AL-20260629-0001" || entry.FeedbackID != "FB-1" {
		t.Fatalf("source/feedback mismatch: %+v", entry)
	}
	if entry.Type != "ip" || entry.Value != "10.12.4.23" {
		t.Fatalf("whitelist object=%s/%s want ip/10.12.4.23", entry.Type, entry.Value)
	}
	if !strings.Contains(entry.Description, "alert_id=AL-20260629-0001") {
		t.Fatalf("description should include source alert: %s", entry.Description)
	}
	if entry.CreatedBy != "sec_analyst" {
		t.Fatalf("created_by=%q want sec_analyst", entry.CreatedBy)
	}
}

func TestBuildWhitelistDraftEntryFallsBackToDestinationIP(t *testing.T) {
	entry, err := buildWhitelistDraftEntry("tenant-a", "AL-20260629-0002", "FB-2", "", "BUSINESS_NORMAL", &service.AlertDetailDTO{
		AlertDTO: service.AlertDTO{
			DstIP: "192.0.2.10",
		},
	})
	if err != nil {
		t.Fatalf("buildWhitelistDraftEntry() error = %v", err)
	}
	if entry.Value != "192.0.2.10" {
		t.Fatalf("value=%q want dst ip", entry.Value)
	}
	if entry.CreatedBy != "feedback-system" {
		t.Fatalf("created_by=%q want feedback-system", entry.CreatedBy)
	}
}

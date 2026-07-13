package api

import (
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
)

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

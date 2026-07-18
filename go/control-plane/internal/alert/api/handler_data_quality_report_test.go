package api

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
)

func TestBuildDataQualityDailyReportUsesLiveChecksAndFixture(t *testing.T) {
	end := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	live := &dataquality.DataQualityReport{
		Timestamp: end,
		TenantID:  "tenant-a",
		Overall:   "degraded",
		Metrics: map[string]float64{
			"data_completeness":   98.7,
			"p95_latency_ms":      1200,
			"flow_rate":           2400,
			"insert_rate_per_min": 2100,
		},
		Checks: []dataquality.QualityCheck{
			{Name: "data_completeness", Status: "pass", Value: 98.7, Threshold: 95},
			{Name: "end_to_end_latency", Status: "warn", Message: "p95 increased", Value: 1200, Threshold: 1000},
		},
	}
	fixture := &DataQualityUIFixture{FixtureVersion: "fixture-v1", Payload: map[string]interface{}{
		"storageComponentRows": []interface{}{[]interface{}{"ClickHouse", "正常", "2.1K EPS", "99.9%"}},
	}}
	report := buildDataQualityDailyReport(live, fixture, end.Add(-24*time.Hour), end)
	if report.ReportID != "dq-tenant-a-20260716" {
		t.Fatalf("unexpected report id: %s", report.ReportID)
	}
	if report.Score != 95 {
		t.Fatalf("expected score 95, got %.1f", report.Score)
	}
	if len(report.Anomalies) != 1 || report.Anomalies[0].RootCause != "p95 increased" {
		t.Fatalf("unexpected anomalies: %#v", report.Anomalies)
	}
	if len(report.StorageRows) != 1 || report.StorageRows[0][0] != "ClickHouse" {
		t.Fatalf("fixture storage rows were not used: %#v", report.StorageRows)
	}
	if report.Source["fixture_version"] != "fixture-v1" {
		t.Fatalf("unexpected report source: %#v", report.Source)
	}
}

func TestDataQualityReportWindowValidation(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/data-quality/reports/daily?start_time=2000&end_time=1000", nil)
	if _, _, err := dataQualityReportWindow(req); err == nil {
		t.Fatal("expected invalid report window error")
	}
	req = httptest.NewRequest("GET", "/api/v1/data-quality/reports/daily?start_time=bad", nil)
	if _, _, err := dataQualityReportWindow(req); err == nil || !strings.Contains(err.Error(), "unix milliseconds") {
		t.Fatalf("expected timestamp validation error, got %v", err)
	}
}

func TestBuildDataQualityReportPDF(t *testing.T) {
	report := &dataQualityDailyReport{
		ReportID: "dq-default-20260716", TenantID: "default", Version: "v2026.07.16",
		PeriodStart: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		Score: 97.8, Overall: "healthy", KeyMetrics: [][]string{{"flow_rate", "2000", "100", "pass"}},
	}
	payload := buildDataQualityReportPDF(report)
	if !bytes.HasPrefix(payload, []byte("%PDF-1.4")) || !bytes.HasSuffix(payload, []byte("%%EOF\n")) {
		t.Fatalf("invalid PDF envelope: %q", payload[:min(16, len(payload))])
	}
	if !bytes.Contains(payload, []byte("dq-default-20260716")) {
		t.Fatal("PDF does not contain report identity")
	}
}

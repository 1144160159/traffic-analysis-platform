package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type dashboardSnapshotStubReader struct {
	result dashboardSnapshotReadResult
	tenant string
	calls  int
}

func (r *dashboardSnapshotStubReader) ReadDashboardSnapshot(_ context.Context, tenant string, _, _ time.Time) dashboardSnapshotReadResult {
	r.calls++
	r.tenant = tenant
	return r.result
}

type dashboardSnapshotTestResponse struct {
	Success bool                  `json:"success"`
	Data    DashboardSnapshotData `json:"data"`
	Meta    httpx.ContractMeta    `json:"meta"`
	Error   *httpx.ErrorInfo      `json:"error"`
}

func TestDashboardSnapshotUsesAuthenticatedTenantAndStableMeta(t *testing.T) {
	metricValue := 3.0
	reader := &dashboardSnapshotStubReader{result: dashboardSnapshotReadResult{
		Data:             DashboardSnapshotData{Metrics: []DashboardSnapshotMetric{{Key: "high_risk_open", Label: "高危未处理", Value: &metricValue, Unit: "条", State: "risk", Spark: []float64{1, 3}}}},
		SourceWatermarks: map[string]string{"clickhouse.dashboard.watermark": "2026-08-03T10:00:00Z"},
	}}
	handler := newDashboardSnapshotHandlerForTest(reader, true)
	handler.now = func() time.Time { return time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC) }

	first := performDashboardSnapshotRequest(t, handler, "/api/v1/dashboard/snapshot?start_time=2026-08-03T09:00:00Z&end_time=2026-08-03T10:00:00Z", "tenant-auth", []string{"alert:read"})
	second := performDashboardSnapshotRequest(t, handler, "/api/v1/dashboard/snapshot?start_time=2026-08-03T09:00:00Z&end_time=2026-08-03T10:00:00Z", "tenant-auth", []string{"alert:read"})
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d/%d", first.Code, second.Code)
	}
	if reader.tenant != "tenant-auth" || reader.calls != 2 {
		t.Fatalf("reader tenant/calls = %q/%d", reader.tenant, reader.calls)
	}
	var one, two dashboardSnapshotTestResponse
	if err := json.Unmarshal(first.Body.Bytes(), &one); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &two); err != nil {
		t.Fatal(err)
	}
	if !one.Success || one.Meta.ContractVersion != 2 || one.Meta.Partial || one.Meta.SnapshotID == "" {
		t.Fatalf("unexpected response: %+v", one)
	}
	if one.Meta.SnapshotID != two.Meta.SnapshotID {
		t.Fatalf("snapshot id changed: %s != %s", one.Meta.SnapshotID, two.Meta.SnapshotID)
	}
	if got := one.Meta.SourceWatermarks["clickhouse.dashboard.watermark"]; got != "2026-08-03T10:00:00Z" {
		t.Fatalf("watermark=%q", got)
	}
}

func TestDashboardSnapshotRejectsTenantOverrideAndMissingScope(t *testing.T) {
	reader := &dashboardSnapshotStubReader{}
	handler := newDashboardSnapshotHandlerForTest(reader, true)
	override := performDashboardSnapshotRequest(t, handler, "/api/v1/dashboard/snapshot?tenant_id=other", "tenant-auth", []string{"alert:read"})
	if override.Code != http.StatusBadRequest || reader.calls != 0 {
		t.Fatalf("override status/calls=%d/%d", override.Code, reader.calls)
	}
	forbidden := performDashboardSnapshotRequest(t, handler, "/api/v1/dashboard/snapshot", "tenant-auth", []string{"alert:write"})
	if forbidden.Code != http.StatusForbidden || reader.calls != 0 {
		t.Fatalf("scope status/calls=%d/%d", forbidden.Code, reader.calls)
	}
	unauthenticated := httptest.NewRecorder()
	handler.GetSnapshot(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/snapshot", nil))
	if unauthenticated.Code != http.StatusUnauthorized || reader.calls != 0 {
		t.Fatalf("auth status/calls=%d/%d", unauthenticated.Code, reader.calls)
	}
}

func TestDashboardSnapshotReportsPartialWithoutInventingZero(t *testing.T) {
	reader := &dashboardSnapshotStubReader{result: dashboardSnapshotReadResult{
		Data:             DashboardSnapshotData{Metrics: []DashboardSnapshotMetric{{Key: "sla_overdue", Label: "超时 SLA", Value: nil, Unit: "项", State: "unknown", Spark: []float64{}}}},
		SourceWatermarks: map[string]string{"postgresql.dashboard_tasks.updated_at": "empty"},
		MissingSections:  []string{"kpis.sla", "opensearch.alerts_projection", "kpis.sla"},
	}}
	handler := newDashboardSnapshotHandlerForTest(reader, true)
	response := performDashboardSnapshotRequest(t, handler, "/api/v1/dashboard/snapshot", "tenant-auth", []string{"alert:*"})
	var payload dashboardSnapshotTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !payload.Meta.Partial {
		t.Fatalf("status/partial=%d/%v", response.Code, payload.Meta.Partial)
	}
	if len(payload.Meta.MissingSections) != 2 || payload.Data.Metrics[0].Value != nil || payload.Data.Metrics[0].State != "unknown" {
		t.Fatalf("partial payload=%+v", payload)
	}
}

func TestDashboardSnapshotFeatureFlagFailsClosed(t *testing.T) {
	reader := &dashboardSnapshotStubReader{}
	handler := newDashboardSnapshotHandlerForTest(reader, false)
	response := performDashboardSnapshotRequest(t, handler, "/api/v1/dashboard/snapshot", "tenant-auth", []string{"alert:read"})
	if response.Code != http.StatusServiceUnavailable || reader.calls != 0 {
		t.Fatalf("status/calls=%d/%d", response.Code, reader.calls)
	}
}

func TestFormatDashboardUnixMillisUsesClickHouseInt64Contract(t *testing.T) {
	const millis int64 = 1785866538999
	if got, want := formatDashboardUnixMillis(millis), "2026-08-04T18:02:18.999Z"; got != want {
		t.Fatalf("formatDashboardUnixMillis(%d)=%q want %q", millis, got, want)
	}
	if got := formatDashboardUnixMillis(0); got != "" {
		t.Fatalf("zero dashboard timestamp=%q", got)
	}
}

func TestDashboardSnapshotViewDoesNotTurnSourceFailureIntoZero(t *testing.T) {
	data := buildDashboardSnapshotView(
		dashboardAlertSummary{}, dashboardTaskSummary{}, nil, nil, nil, nil, 0,
		[]string{"kpis.alerts", "postgresql.dashboard_tasks", "opensearch.alerts_projection"},
	)
	for _, label := range []string{"超时 SLA", "临近超时数", "高危未处理", "待取证", "待反馈", "待复核", "审计留痕缺口", "合规门禁缺口", "队列积压量"} {
		var found *DashboardSnapshotMetric
		for index := range data.Metrics {
			if data.Metrics[index].Label == label {
				found = &data.Metrics[index]
				break
			}
		}
		if found == nil {
			t.Fatalf("metric %s missing", label)
		}
		if found.Value != nil || found.State != "unknown" {
			t.Fatalf("metric %s invented zero: %+v", label, *found)
		}
	}
}

func TestDashboardSnapshotViewUsesAuthoritativeAlertWorkflowMetrics(t *testing.T) {
	data := buildDashboardSnapshotView(
		dashboardAlertSummary{
			Total: 120, New: 80, Critical: 5, High: 7, EvidenceMissing: 9,
			SLAOverdue: 2, SLANearTimeout: 3, FeedbackPending: 11, ReviewPending: 4,
		},
		dashboardTaskSummary{AuditMissing: 6, ComplianceOpen: 8}, nil, nil, nil, nil, 0, nil,
	)
	want := map[string]struct {
		value float64
		state string
	}{
		"sla_overdue":         {2, "risk"},
		"sla_near_timeout":    {3, "warn"},
		"high_risk_open":      {12, "risk"},
		"evidence_pending":    {9, "risk"},
		"feedback_pending":    {11, "warn"},
		"review_pending":      {4, "warn"},
		"audit_trace_gap":     {6, "risk"},
		"compliance_gate_gap": {8, "warn"},
	}
	for _, metric := range data.Metrics {
		expected, ok := want[metric.Key]
		if !ok {
			continue
		}
		if metric.Value == nil || *metric.Value != expected.value || metric.State != expected.state {
			t.Fatalf("metric %s=%+v want value=%v state=%s", metric.Key, metric, expected.value, expected.state)
		}
		delete(want, metric.Key)
	}
	if len(want) != 0 {
		t.Fatalf("metrics not rendered: %+v", want)
	}
}

func TestDashboardSnapshotViewDoesNotClaimProjectionMatchForBoundedOrDivergentCount(t *testing.T) {
	data := buildDashboardSnapshotView(
		dashboardAlertSummary{Total: 20}, dashboardTaskSummary{}, nil, nil, nil, nil, 10,
		[]string{"reconciliation.alerts_projection"},
	)
	for _, quality := range data.QualityRings {
		if quality.Label != "CH/OS 投影一致率" {
			continue
		}
		if quality.Value != nil || quality.State != "unknown" {
			t.Fatalf("projection reconciliation invented a percentage: %+v", quality)
		}
		return
	}
	t.Fatal("projection reconciliation quality metric missing")
}

func performDashboardSnapshotRequest(t *testing.T, handler *DashboardSnapshotHandler, target, tenant string, permissions []string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenant)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "dashboard-reader")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-dashboard-snapshot")
	response := httptest.NewRecorder()
	handler.GetSnapshot(response, request.WithContext(ctx))
	return response
}

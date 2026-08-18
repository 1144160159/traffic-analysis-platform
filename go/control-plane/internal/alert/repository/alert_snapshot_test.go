package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

func TestAlertSearchProjectionRequestsCanonicalUpdatedAt(t *testing.T) {
	fields := "," + strings.Join(alertSearchSourceFields, ",") + ","
	if !strings.Contains(fields, ",updated_at,") {
		t.Fatalf("OpenSearch source fields omit canonical updated_at: %s", fields)
	}
	if strings.Contains(fields, ",updated_ts,") {
		t.Fatalf("OpenSearch source fields retain non-existent updated_ts: %s", fields)
	}
}

type snapshotFactSourceStub struct {
	alerts []*persistence.Alert
	err    error
}

func (s snapshotFactSourceStub) GetByIDs(context.Context, string, []string) ([]*persistence.Alert, error) {
	return s.alerts, s.err
}

type snapshotSearchSourceStub struct {
	result *SearchResult
	err    error
}

func (s snapshotSearchSourceStub) Search(context.Context, *SearchQuery) (*SearchResult, error) {
	return s.result, s.err
}

type snapshotMetadataSourceStub struct {
	states   map[string]AlertManualState
	receipts map[string]AlertProjectionReceipt
	err      error
}

func (s snapshotMetadataSourceStub) Load(context.Context, string, []string, string) (map[string]AlertManualState, map[string]AlertProjectionReceipt, error) {
	return s.states, s.receipts, s.err
}

func TestAlertSnapshotSearchHydratesSearchCandidatesFromAuthorities(t *testing.T) {
	updatedAt := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	authority := snapshotAlert("alert-a", updatedAt)
	searchDocument := authority.Clone()
	searchDocument.SrcIP = "203.0.113.250"
	extraDocument := snapshotAlert("alert-extra", updatedAt)

	repo := &AlertSnapshotRepository{
		facts: snapshotFactSourceStub{alerts: []*persistence.Alert{authority}},
		search: snapshotSearchSourceStub{result: &SearchResult{
			Alerts: []*persistence.Alert{searchDocument, extraDocument}, Total: 2,
			AsOf: "2026-08-15T06:00:01Z", CursorMode: SearchCursorModePIT,
		}},
		metadata: snapshotMetadataSourceStub{
			states: map[string]AlertManualState{"alert-a": {
				AlertID: "alert-a", StateVersion: updatedAt.UnixMilli() + 1,
				Assignee: "analyst-a", Status: "assigned", ProjectionStatus: "pending",
			}},
			receipts: map[string]AlertProjectionReceipt{"alert-a": {
				AlertID: "alert-a", SourceVersion: updatedAt.UnixMilli() - 1,
				SourceSHA256: "stale-digest",
			}},
		},
		targetIndexVersion: "alerts-v2-write",
		now:                func() time.Time { return updatedAt.Add(time.Second) },
		logger:             zap.NewNop(),
	}

	result, err := repo.Search(context.Background(), &SearchQuery{TenantID: "tenant-a", Size: 50})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Alerts) != 1 {
		t.Fatalf("authoritative alerts = %d, want 1", len(result.Alerts))
	}
	alert := result.Alerts[0]
	if alert.SrcIP != authority.SrcIP {
		t.Fatalf("SrcIP = %q, want ClickHouse fact %q", alert.SrcIP, authority.SrcIP)
	}
	if alert.Status != "assigned" || alert.Assignee != "analyst-a" {
		t.Fatalf("manual state = %s/%s, want assigned/analyst-a", alert.Status, alert.Assignee)
	}
	if result.StateSources["alert-a"] != "postgresql" || result.ProjectionStatuses["alert-a"] != "pending" {
		t.Fatalf("state metadata = %q/%q", result.StateSources["alert-a"], result.ProjectionStatuses["alert-a"])
	}
	if len(result.Reconciliation.Extra) != 1 || result.Reconciliation.Extra[0].AlertID != "alert-extra" {
		t.Fatalf("extra reconciliation = %#v", result.Reconciliation.Extra)
	}
	assertSnapshotIssue(t, result.Reconciliation.Stale, "opensearch.alerts.search")
	assertSnapshotIssue(t, result.Reconciliation.Stale, "postgresql.alert_opensearch_projection_watermarks")
	assertSnapshotIssue(t, result.Reconciliation.Stale, "clickhouse.alerts.manual_state_projection")
	if !result.Partial {
		t.Fatal("cross-store differences must make the snapshot partial")
	}
	if result.SourceWatermarks["clickhouse.alerts.source_version"] == "" ||
		result.SourceWatermarks["postgresql.alert_assignment_states.state_version"] == "" ||
		result.SourceWatermarks["postgresql.alert_opensearch_projection_watermarks.source_version"] == "" {
		t.Fatalf("source watermarks = %#v", result.SourceWatermarks)
	}
}

func TestAlertSnapshotSearchReportsMissingReceipt(t *testing.T) {
	alert := snapshotAlert("alert-a", time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC))
	repo := &AlertSnapshotRepository{
		facts: snapshotFactSourceStub{alerts: []*persistence.Alert{alert}},
		search: snapshotSearchSourceStub{result: &SearchResult{
			Alerts: []*persistence.Alert{alert.Clone()}, Total: 1,
		}},
		metadata: snapshotMetadataSourceStub{states: map[string]AlertManualState{}, receipts: map[string]AlertProjectionReceipt{}},
		now:      time.Now,
		logger:   zap.NewNop(),
	}
	result, err := repo.Search(context.Background(), &SearchQuery{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	assertSnapshotIssue(t, result.Reconciliation.Missing, "postgresql.alert_opensearch_projection_watermarks")
	if len(result.MissingSections) != 1 || result.MissingSections[0] != "postgresql.alerts.projection_receipts" {
		t.Fatalf("missing sections = %#v", result.MissingSections)
	}
}

func TestAlertSnapshotSearchDegradesExplicitlyWhenPostgresMetadataFails(t *testing.T) {
	alert := snapshotAlert("alert-a", time.Now().UTC())
	repo := &AlertSnapshotRepository{
		facts:    snapshotFactSourceStub{alerts: []*persistence.Alert{alert}},
		search:   snapshotSearchSourceStub{result: &SearchResult{Alerts: []*persistence.Alert{alert.Clone()}, Total: 1}},
		metadata: snapshotMetadataSourceStub{err: errors.New("postgres unavailable")},
		now:      time.Now,
		logger:   zap.NewNop(),
	}
	result, err := repo.Search(context.Background(), &SearchQuery{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !result.Partial || len(result.MissingSections) != 1 || result.MissingSections[0] != "postgresql.alert_snapshot_metadata" {
		t.Fatalf("partial=%v missing=%#v", result.Partial, result.MissingSections)
	}
	if result.StateSources["alert-a"] != "clickhouse" {
		t.Fatalf("state source = %q, want clickhouse", result.StateSources["alert-a"])
	}
}

func TestAlertSnapshotSearchFailsClosedWhenClickHouseAuthorityFails(t *testing.T) {
	alert := snapshotAlert("alert-a", time.Now().UTC())
	repo := &AlertSnapshotRepository{
		facts:  snapshotFactSourceStub{err: errors.New("clickhouse unavailable")},
		search: snapshotSearchSourceStub{result: &SearchResult{Alerts: []*persistence.Alert{alert}}},
		now:    time.Now,
		logger: zap.NewNop(),
	}
	if _, err := repo.Search(context.Background(), &SearchQuery{TenantID: "tenant-a"}); err == nil {
		t.Fatal("Search() succeeded without ClickHouse authority")
	}
}

func TestPostgresAlertSnapshotMetadataSourceIsTenantAndTargetBound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	updatedAt := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("WITH requested AS").
		WithArgs("tenant-a", `{"alert-a","alert-b"}`, "alerts-v2-write").
		WillReturnRows(sqlmock.NewRows([]string{
			"alert_id", "state_version", "assignee", "status", "projection_status", "updated_at",
			"source_version", "source_sha256", "applied_at",
		}).AddRow("alert-a", updatedAt.UnixMilli(), "analyst-a", "assigned", "applied", updatedAt,
			updatedAt.UnixMilli(), "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", updatedAt).
			AddRow("alert-b", nil, nil, nil, nil, nil, nil, nil, nil))

	states, receipts, err := (&postgresAlertSnapshotMetadataSource{db: db}).Load(
		context.Background(), "tenant-a", []string{"alert-a", "alert-b"}, "alerts-v2-write",
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(states) != 1 || states["alert-a"].Assignee != "analyst-a" || len(receipts) != 1 {
		t.Fatalf("states=%#v receipts=%#v", states, receipts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func snapshotAlert(alertID string, updatedAt time.Time) *persistence.Alert {
	return &persistence.Alert{
		TenantID: "tenant-a", AlertID: alertID, Fingerprint: "fp-" + alertID,
		SrcIP: "192.0.2.10", DstIP: "198.51.100.20", SrcPort: 12345, DstPort: 443,
		Protocol: 6, AlertType: "tls_anomaly", Labels: []string{"encrypted"}, Score: 0.91,
		Severity: "high", FirstSeen: updatedAt.Add(-time.Minute), LastSeen: updatedAt,
		UpdatedTs: updatedAt, Status: "new", ModelVersion: "model-v1", RuleVersion: "rule-v1",
	}
}

func assertSnapshotIssue(t *testing.T, issues []AlertSnapshotIssue, source string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Source == source {
			return
		}
	}
	t.Fatalf("source %q not found in issues %#v", source, issues)
}

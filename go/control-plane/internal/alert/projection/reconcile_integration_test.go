package projection

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type ephemeralProjectionSource struct {
	alerts []*persistence.Alert
}

func (s ephemeralProjectionSource) ListProjectionAlerts(context.Context, persistence.ProjectionScope) ([]*persistence.Alert, bool, error) {
	return s.alerts, false, nil
}

type countingProjectionTarget struct {
	ProjectionRepairTarget
	writes int
}

func (t *countingProjectionTarget) WriteAlert(ctx context.Context, alert *persistence.Alert) error {
	t.writes++
	return t.ProjectionRepairTarget.WriteAlert(ctx, alert)
}

type ephemeralReconcileStore struct {
	applied    []string
	result     persistence.ProjectionReconcileResult
	watermarks map[string]bool
}

func (*ephemeralReconcileStore) StartProjectionReconcileRun(context.Context, persistence.ProjectionReconcileRun, int) error {
	return nil
}

func (s *ephemeralReconcileStore) CompleteProjectionReconcileRun(_ context.Context, _ string, result persistence.ProjectionReconcileResult) error {
	s.result = result
	return nil
}

func (s *ephemeralReconcileStore) RecordProjectionApplied(_ context.Context, alert *persistence.Alert, _ string) error {
	s.applied = append(s.applied, alert.AlertID)
	if s.watermarks == nil {
		s.watermarks = make(map[string]bool)
	}
	s.watermarks[alert.AlertID] = true
	return nil
}

func (s *ephemeralReconcileStore) ListProjectionWatermarkMismatches(_ context.Context, alerts []*persistence.Alert, _ string) ([]string, error) {
	missing := make([]string, 0)
	for _, alert := range alerts {
		if !s.watermarks[alert.AlertID] {
			missing = append(missing, alert.AlertID)
		}
	}
	return missing, nil
}

// TestAlertProjectionRepairTerminalReceiptRealOpenSearch proves that the
// production repair target refreshes and re-reads the exact V2 aliases before
// returning convergence. The surrounding runner owns and removes the cluster.
func TestAlertProjectionRepairTerminalReceiptRealOpenSearch(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL"), "/")
	if baseURL == "" {
		t.Skip("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL is not set")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil || parsed.Scheme != "http" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("refusing non-loopback OpenSearch endpoint %q: %v", baseURL, err)
	}
	if os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing OpenSearch endpoint without explicit ephemeral sentinel")
	}
	response, err := http.Get(baseURL + "/codex-ephemeral-asset-projection-sentinel/_doc/ephemeral-only")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var sentinel struct {
		Found  bool `json:"found"`
		Source struct {
			Marker string `json:"marker"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sentinel); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !sentinel.Found || sentinel.Source.Marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel OpenSearch: status=%s sentinel=%+v", response.Status, sentinel)
	}

	target, err := persistence.NewOpenSearchReconcileTarget(
		[]string{baseURL}, "", "", "alerts-v2-read", "alerts-v2-write", true, false, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	base := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	alert := func(id, status string, updated time.Time) *persistence.Alert {
		return &persistence.Alert{
			TenantID: "tenant-alert-repair-g1", AlertID: id, Fingerprint: "fingerprint-" + id,
			SrcIP: "192.0.2.92", DstIP: "203.0.113.92", SrcPort: 49002, DstPort: 443,
			Protocol: 6, AlertType: "projection-repair", Severity: "high", Score: 0.92,
			FirstSeen: base.Add(-time.Minute), LastSeen: updated, UpdatedTs: updated,
			Count: 1, Status: status, EventID: "event-" + id,
		}
	}
	sourceAlerts := []*persistence.Alert{
		alert("repair-missing", "new", base.Add(2*time.Second)),
		alert("repair-stale", "closed", base.Add(3*time.Second)),
	}
	for _, seed := range []*persistence.Alert{
		alert("repair-stale", "new", base),
		alert("repair-extra", "new", base.Add(time.Second)),
	} {
		if err := target.WriteAlert(context.Background(), seed); err != nil {
			t.Fatal(err)
		}
	}
	if err := target.RefreshProjectionTarget(context.Background()); err != nil {
		t.Fatal(err)
	}

	store := &ephemeralReconcileStore{}
	reconciler, err := NewReconciler(ReconcileConfig{MaxDocuments: 100, StopErrorCount: 2, RepairPerSecond: 100000}, ephemeralProjectionSource{alerts: sourceAlerts}, target, store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.Run(context.Background(), ReconcileRequest{
		Mode: "repair", RequestedBy: "ephemeral-runner", TraceID: "trace-alert-repair-g1",
		Scope: persistence.ProjectionScope{TenantID: "tenant-alert-repair-g1", TargetIndexVersion: "alerts-v2-write", MaxDocuments: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(store.applied)
	if result.Status != "completed" || result.MissingCount != 1 || result.StaleCount != 1 || result.ExtraCount != 1 ||
		result.RepairedCount != 2 || result.ErrorCount != 0 || !result.VerificationPerformed || !result.WatermarksConverged || !result.RepairConverged ||
		result.RemainingMissingCount != 0 || result.RemainingStaleCount != 0 || result.RemainingExtraCount != 1 ||
		len(result.RemainingExtraIDs) != 1 || result.RemainingExtraIDs[0] != "repair-extra" ||
		strings.Join(store.applied, ",") != "repair-missing,repair-stale" || !store.result.RepairConverged {
		t.Fatalf("unexpected terminal repair receipt: result=%+v applied=%v stored=%+v", result, store.applied, store.result)
	}
}

// TestAlertProjectionRepairRealPostgresAndOpenSearch proves the terminal
// receipt across both real projection stores in one owned run. It also deletes
// one PostgreSQL receipt after the target is already converged and proves that
// the next run restores the receipt without rewriting an OpenSearch document.
func TestAlertProjectionRepairRealPostgresAndOpenSearch(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL"), "/")
	dsn := os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_PG_DSN")
	if baseURL == "" || dsn == "" {
		t.Skip("combined ephemeral OpenSearch and PostgreSQL endpoints are not set")
	}
	parsedOS, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	osHost, _, osHostErr := net.SplitHostPort(parsedOS.Host)
	parsedPG, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pgHost, _, pgHostErr := net.SplitHostPort(parsedPG.Host)
	if osHostErr != nil || parsedOS.Scheme != "http" || net.ParseIP(osHost) == nil || !net.ParseIP(osHost).IsLoopback() ||
		pgHostErr != nil || net.ParseIP(pgHost) == nil || !net.ParseIP(pgHost).IsLoopback() || parsedPG.Query().Get("sslmode") != "disable" {
		t.Fatalf("refusing non-loopback combined endpoints: os=%q os_err=%v pg_host_err=%v", baseURL, osHostErr, pgHostErr)
	}
	if os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing OpenSearch endpoint without explicit ephemeral sentinel")
	}
	response, err := http.Get(baseURL + "/codex-ephemeral-alert-reconcile-sentinel/_doc/ephemeral-only")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var osSentinel struct {
		Found  bool `json:"found"`
		Source struct {
			Marker string `json:"marker"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&osSentinel); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !osSentinel.Found || osSentinel.Source.Marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel OpenSearch: status=%s sentinel=%+v", response.Status, osSentinel)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var pgMarker string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_alert_projection_sentinel LIMIT 1`).Scan(&pgMarker); err != nil || pgMarker != "ephemeral-only" {
		t.Fatalf("refusing PostgreSQL without ephemeral sentinel: marker=%q err=%v", pgMarker, err)
	}
	store := persistence.NewProjectionDebtStore(db)
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	tenantID := "tenant-alert-combined-g1"
	defer db.ExecContext(ctx, `DELETE FROM alert_opensearch_reconcile_runs WHERE tenant_id=$1`, tenantID)
	defer db.ExecContext(ctx, `DELETE FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1`, tenantID)

	target, err := persistence.NewOpenSearchReconcileTarget(
		[]string{baseURL}, "", "", "alerts-v2-read", "alerts-v2-write", true, false, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	base := time.Date(2026, 8, 7, 14, 0, 0, 0, time.UTC)
	alert := func(id, status string, updated time.Time) *persistence.Alert {
		return &persistence.Alert{
			TenantID: tenantID, AlertID: id, Fingerprint: "fingerprint-" + id,
			SrcIP: "192.0.2.93", DstIP: "203.0.113.93", SrcPort: 49003, DstPort: 443,
			Protocol: 6, AlertType: "projection-combined", Severity: "high", Score: 0.93,
			FirstSeen: base.Add(-time.Minute), LastSeen: updated, UpdatedTs: updated,
			Count: 1, Status: status, EventID: "event-" + id,
		}
	}
	sourceAlerts := []*persistence.Alert{
		alert("combined-missing", "new", base.Add(2*time.Second)),
		alert("combined-stale", "closed", base.Add(3*time.Second)),
	}
	for _, seed := range []*persistence.Alert{
		alert("combined-stale", "new", base),
		alert("combined-extra", "new", base.Add(time.Second)),
	} {
		if err := target.WriteAlert(ctx, seed); err != nil {
			t.Fatal(err)
		}
	}
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	countedTarget := &countingProjectionTarget{ProjectionRepairTarget: target}
	newReconciler := func() *Reconciler {
		reconciler, err := NewReconciler(
			ReconcileConfig{MaxDocuments: 100, StopErrorCount: 2, RepairPerSecond: 100000},
			ephemeralProjectionSource{alerts: sourceAlerts}, countedTarget, store,
		)
		if err != nil {
			t.Fatal(err)
		}
		return reconciler
	}
	request := ReconcileRequest{
		Mode: "repair", RequestedBy: "ephemeral-combined-runner", TraceID: "trace-alert-combined-g1",
		Scope: persistence.ProjectionScope{TenantID: tenantID, TargetIndexVersion: "alerts-v2-write", MaxDocuments: 100},
	}
	first, err := newReconciler().Run(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "completed" || first.MissingCount != 1 || first.StaleCount != 1 || first.ExtraCount != 1 ||
		first.RepairedCount != 2 || !first.VerificationPerformed || !first.WatermarksConverged || !first.RepairConverged ||
		first.WatermarkMismatchCount != 0 || first.RemainingMissingCount != 0 || first.RemainingStaleCount != 0 || first.RemainingExtraCount != 1 || countedTarget.writes != 2 {
		t.Fatalf("unexpected combined first receipt: %+v", first)
	}
	var receiptCount, convergedRunCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1 AND target_index_version='alerts-v2-write'`, tenantID).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_opensearch_reconcile_runs WHERE tenant_id=$1 AND status='completed' AND (result_manifest->'post_repair_verification'->>'repair_converged')::boolean`, tenantID).Scan(&convergedRunCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 2 || convergedRunCount != 1 {
		t.Fatalf("combined durable receipt mismatch: watermarks=%d converged_runs=%d", receiptCount, convergedRunCount)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1 AND alert_id='combined-missing' AND target_index_version='alerts-v2-write'`, tenantID); err != nil {
		t.Fatal(err)
	}
	countedTarget.writes = 0

	second, err := newReconciler().Run(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "completed" || second.MissingCount != 0 || second.StaleCount != 0 || second.ExtraCount != 1 ||
		second.RepairedCount != 0 || !second.VerificationPerformed || !second.WatermarksConverged || !second.RepairConverged || second.WatermarkMismatchCount != 0 || countedTarget.writes != 0 {
		t.Fatalf("converged OpenSearch target did not recover deleted PostgreSQL receipt: %+v", second)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1 AND target_index_version='alerts-v2-write'`, tenantID).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 2 {
		t.Fatalf("deleted receipt was not recovered: watermarks=%d", receiptCount)
	}
}

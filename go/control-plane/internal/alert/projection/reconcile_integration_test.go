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
	alertrepo "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
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

// TestAlertProjectionRepairRealClickHousePostgresAndOpenSearch replaces the
// bounded in-memory authority with the production ClickHouse repository. One
// owned run must finish with equal authoritative and target hashes plus the
// matching durable PostgreSQL receipt for every source alert.
func TestAlertProjectionRepairRealClickHousePostgresAndOpenSearch(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL"), "/")
	pgDSN := os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_PG_DSN")
	clickHouseHost := strings.TrimSpace(os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_HOST"))
	if baseURL == "" || pgDSN == "" || clickHouseHost == "" {
		t.Skip("three-store ephemeral ClickHouse, PostgreSQL and OpenSearch endpoints are not set")
	}
	parsedOS, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	osHost, _, osHostErr := net.SplitHostPort(parsedOS.Host)
	parsedPG, err := url.Parse(pgDSN)
	if err != nil {
		t.Fatal(err)
	}
	pgHost, _, pgHostErr := net.SplitHostPort(parsedPG.Host)
	chHost, _, chHostErr := net.SplitHostPort(clickHouseHost)
	if osHostErr != nil || parsedOS.Scheme != "http" || net.ParseIP(osHost) == nil || !net.ParseIP(osHost).IsLoopback() ||
		pgHostErr != nil || net.ParseIP(pgHost) == nil || !net.ParseIP(pgHost).IsLoopback() || parsedPG.Query().Get("sslmode") != "disable" ||
		chHostErr != nil || net.ParseIP(chHost) == nil || !net.ParseIP(chHost).IsLoopback() {
		t.Fatalf("refusing non-loopback three-store endpoints: os_err=%v pg_err=%v ch_err=%v", osHostErr, pgHostErr, chHostErr)
	}
	if os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL") != "ephemeral-only" ||
		os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing three-store endpoints without explicit ephemeral sentinels")
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

	pgDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pgDB.Close()
	ctx := context.Background()
	var pgMarker string
	if err := pgDB.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_alert_projection_sentinel LIMIT 1`).Scan(&pgMarker); err != nil || pgMarker != "ephemeral-only" {
		t.Fatalf("refusing PostgreSQL without ephemeral sentinel: marker=%q err=%v", pgMarker, err)
	}
	store := persistence.NewProjectionDebtStore(pgDB)
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}

	clickHouseClient, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{clickHouseHost}, Database: "traffic",
		Username:     os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_USER"),
		Password:     os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_PASSWORD"),
		MaxOpenConns: 2, MaxIdleConns: 1, DialTimeout: 5 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer clickHouseClient.Close()
	row, err := clickHouseClient.QueryRow(ctx, `SELECT marker FROM traffic.codex_ephemeral_alert_reconcile_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var chMarker string
	if err := row.Scan(&chMarker); err != nil || chMarker != "ephemeral-only" {
		t.Fatalf("refusing ClickHouse without ephemeral sentinel: marker=%q err=%v", chMarker, err)
	}

	tenantID := "tenant-alert-three-store-g1"
	defer pgDB.ExecContext(ctx, `DELETE FROM alert_opensearch_reconcile_runs WHERE tenant_id=$1`, tenantID)
	defer pgDB.ExecContext(ctx, `DELETE FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1`, tenantID)
	target, err := persistence.NewOpenSearchReconcileTarget(
		[]string{baseURL}, "", "", "alerts-v2-read", "alerts-v2-write", true, false, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	base := time.Date(2026, 8, 7, 15, 30, 0, 0, time.UTC)
	alert := func(id, status string, updated time.Time) *persistence.Alert {
		return &persistence.Alert{
			TenantID: tenantID, AlertID: id, Fingerprint: "fingerprint-" + id,
			CommunityID: "community-" + id, SessionID: "session-" + id,
			SrcIP: "192.0.2.94", DstIP: "203.0.113.94", SrcPort: 49004, DstPort: 443,
			Protocol: 6, AlertType: "projection-three-store", Labels: []string{"integration"},
			Severity: "high", Score: 0.94, FirstSeen: base.Add(-time.Minute), LastSeen: updated,
			UpdatedTs: updated, Count: 1, Status: status, ModelVersion: "model-g1", RuleVersion: "rule-g1",
			FeatureSetID: "feature-g1", EvidenceIDs: []string{"evidence-" + id}, EventID: "event-" + id,
			TraceID: "1234567890abcdef1234567890abcdef", StateVersion: uint64(updated.UnixMilli()),
		}
	}
	sourceAlerts := []*persistence.Alert{
		alert("three-store-missing", "new", base.Add(2*time.Second)),
		alert("three-store-stale", "closed", base.Add(3*time.Second)),
	}
	clickHouseWriter, err := persistence.NewClickHouseWriter(clickHouseClient, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := clickHouseWriter.WriteBatch(ctx, sourceAlerts); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []*persistence.Alert{
		alert("three-store-stale", "new", base),
		alert("three-store-extra", "new", base.Add(time.Second)),
	} {
		if err := target.WriteAlert(ctx, seed); err != nil {
			t.Fatal(err)
		}
	}
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	authority := alertrepo.NewAlertRepository(clickHouseClient, zap.NewNop())
	countedTarget := &countingProjectionTarget{ProjectionRepairTarget: target}
	reconciler, err := NewReconciler(
		ReconcileConfig{MaxDocuments: 100, StopErrorCount: 2, RepairPerSecond: 100000}, authority, countedTarget, store,
	)
	if err != nil {
		t.Fatal(err)
	}
	scope := persistence.ProjectionScope{TenantID: tenantID, TargetIndexVersion: "alerts-v2-write", MaxDocuments: 100}
	result, err := reconciler.Run(ctx, ReconcileRequest{
		Mode: "repair", RequestedBy: "ephemeral-three-store-runner", TraceID: "trace-alert-three-store-g1", Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.MissingCount != 1 || result.StaleCount != 1 || result.ExtraCount != 1 ||
		result.RepairedCount != 2 || countedTarget.writes != 2 || !result.VerificationPerformed || !result.WatermarksConverged ||
		!result.RepairConverged || result.RemainingMissingCount != 0 || result.RemainingStaleCount != 0 || result.RemainingExtraCount != 1 {
		t.Fatalf("unexpected three-store terminal receipt: %+v writes=%d", result, countedTarget.writes)
	}
	authoritative, sourceTruncated, err := authority.ListProjectionAlerts(ctx, scope)
	if err != nil || sourceTruncated || len(authoritative) != 2 {
		t.Fatalf("unexpected ClickHouse authority: alerts=%d truncated=%v err=%v", len(authoritative), sourceTruncated, err)
	}
	projected, targetTruncated, err := target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(projected) != 3 {
		t.Fatalf("unexpected OpenSearch target: alerts=%d truncated=%v err=%v", len(projected), targetTruncated, err)
	}
	projectedByID := make(map[string]*persistence.Alert, len(projected))
	for _, item := range projected {
		projectedByID[item.AlertID] = item
	}
	for _, sourceAlert := range authoritative {
		sourceHash, err := persistence.AlertProjectionSHA256(sourceAlert)
		if err != nil {
			t.Fatal(err)
		}
		targetHash, err := persistence.AlertProjectionSHA256(projectedByID[sourceAlert.AlertID])
		if err != nil {
			t.Fatal(err)
		}
		var receiptVersion int64
		var receiptHash string
		if err := pgDB.QueryRowContext(ctx, `SELECT source_version,source_sha256 FROM alert_opensearch_projection_watermarks WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version='alerts-v2-write'`, tenantID, sourceAlert.AlertID).Scan(&receiptVersion, &receiptHash); err != nil {
			t.Fatal(err)
		}
		if sourceHash != targetHash || receiptHash != sourceHash || receiptVersion != persistence.AlertSourceVersion(sourceAlert) {
			t.Fatalf("three-store hash/version mismatch alert=%s source=%s target=%s receipt=%s version=%d", sourceAlert.AlertID, sourceHash, targetHash, receiptHash, receiptVersion)
		}
	}
	var convergedRunCount int
	if err := pgDB.QueryRowContext(ctx, `SELECT count(*) FROM alert_opensearch_reconcile_runs WHERE tenant_id=$1 AND status='completed' AND (result_manifest->'post_repair_verification'->>'repair_converged')::boolean`, tenantID).Scan(&convergedRunCount); err != nil {
		t.Fatal(err)
	}
	if convergedRunCount != 1 {
		t.Fatalf("durable three-store terminal receipt count=%d want=1", convergedRunCount)
	}
}

// TestAlertProjectionShadowRealClickHouseAndOpenSearch proves the production
// readers can classify one bounded V2 scope without a PostgreSQL dependency or
// any target mutation. The surrounding runner owns and removes both services.
func TestAlertProjectionShadowRealClickHouseAndOpenSearch(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL"), "/")
	clickHouseHost := os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_HOST")
	if baseURL == "" || clickHouseHost == "" {
		t.Skip("combined ephemeral ClickHouse and OpenSearch endpoints are not set")
	}
	parsedOS, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	osHost, _, osHostErr := net.SplitHostPort(parsedOS.Host)
	chHost, _, chHostErr := net.SplitHostPort(clickHouseHost)
	if osHostErr != nil || parsedOS.Scheme != "http" || net.ParseIP(osHost) == nil || !net.ParseIP(osHost).IsLoopback() ||
		chHostErr != nil || net.ParseIP(chHost) == nil || !net.ParseIP(chHost).IsLoopback() {
		t.Fatalf("refusing non-loopback shadow endpoints: os_err=%v ch_err=%v", osHostErr, chHostErr)
	}
	if os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL") != "ephemeral-only" ||
		os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing shadow endpoints without explicit ephemeral sentinels")
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

	ctx := context.Background()
	clickHouseClient, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{clickHouseHost}, Database: "traffic",
		Username:     os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_USER"),
		Password:     os.Getenv("ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_PASSWORD"),
		MaxOpenConns: 2, MaxIdleConns: 1, DialTimeout: 5 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer clickHouseClient.Close()
	row, err := clickHouseClient.QueryRow(ctx, `SELECT marker FROM traffic.codex_ephemeral_alert_reconcile_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var chMarker string
	if err := row.Scan(&chMarker); err != nil || chMarker != "ephemeral-only" {
		t.Fatalf("refusing ClickHouse without ephemeral sentinel: marker=%q err=%v", chMarker, err)
	}

	target, err := persistence.NewOpenSearchReconcileTarget(
		[]string{baseURL}, "", "", "alerts-v2-read", "alerts-v2-write", true, false, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	metadata, err := target.ProjectionMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.WriteIndices) != 1 || !metadata.WriteIndices[0].IsWriteIndex {
		t.Fatalf("ephemeral V2 write alias is not unique: %+v", metadata)
	}
	tenantID := "tenant-alert-shadow-g1"
	base := time.Date(2026, 8, 7, 18, 0, 0, 0, time.UTC)
	alert := func(id, status string, updated time.Time) *persistence.Alert {
		return &persistence.Alert{
			TenantID: tenantID, AlertID: id, Fingerprint: "fingerprint-" + id,
			CommunityID: "community-" + id, SessionID: "session-" + id,
			SrcIP: "192.0.2.95", DstIP: "203.0.113.95", SrcPort: 49005, DstPort: 443,
			Protocol: 6, AlertType: "projection-shadow", Labels: []string{"integration"},
			Severity: "high", Score: 0.95, FirstSeen: base.Add(-time.Minute), LastSeen: updated,
			UpdatedTs: updated, Count: 1, Status: status, ModelVersion: "model-g1", RuleVersion: "rule-g1",
			FeatureSetID: "feature-g1", EvidenceIDs: []string{"evidence-" + id}, EventID: "event-" + id,
			TraceID: "1234567890abcdef1234567890abcdef", StateVersion: uint64(updated.UnixMilli()),
		}
	}
	sourceAlerts := []*persistence.Alert{
		alert("shadow-missing", "new", base.Add(2*time.Second)),
		alert("shadow-stale", "closed", base.Add(3*time.Second)),
	}
	clickHouseWriter, err := persistence.NewClickHouseWriter(clickHouseClient, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := clickHouseWriter.WriteBatch(ctx, sourceAlerts); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []*persistence.Alert{
		alert("shadow-stale", "new", base),
		alert("shadow-extra", "new", base.Add(time.Second)),
	} {
		if err := target.WriteAlert(ctx, seed); err != nil {
			t.Fatal(err)
		}
	}
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	authority := alertrepo.NewAlertRepository(clickHouseClient, zap.NewNop())
	scope := persistence.ProjectionScope{
		TenantID: tenantID, StartTime: base.Add(-5 * time.Minute), EndTime: base.Add(5 * time.Minute),
		TargetIndexVersion: "alerts-v2-write", MaxDocuments: 100,
	}
	sourceBefore, sourceTruncated, err := authority.ListProjectionAlerts(ctx, scope)
	if err != nil || sourceTruncated || len(sourceBefore) != 2 {
		t.Fatalf("unexpected pre-shadow authority: count=%d truncated=%t err=%v", len(sourceBefore), sourceTruncated, err)
	}
	targetBefore, targetTruncated, err := target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(targetBefore) != 2 {
		t.Fatalf("unexpected pre-shadow target: count=%d truncated=%t err=%v", len(targetBefore), targetTruncated, err)
	}
	manifest, err := BuildShadowManifest(ctx, ShadowConfig{
		MaxDocuments: 10_000, MaxWindow: time.Hour, MinimumWindowAge: 15 * time.Minute,
		Now: func() time.Time { return base.Add(time.Hour) },
	}, authority, target, ShadowRequest{
		RequestedBy: "ephemeral-shadow-runner", TraceID: "trace-alert-shadow-g1", EnvironmentID: "owned-loopback-g1", Scope: scope,
		Target: ShadowTargetMetadata{
			ClusterUUID: metadata.ClusterUUID, ReadTarget: metadata.ReadTarget, WriteAlias: metadata.WriteAlias,
			WriteIndices: []ShadowWriteIndex{{Index: metadata.WriteIndices[0].Index, IsWriteIndex: metadata.WriteIndices[0].IsWriteIndex}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != ShadowStatusDiff || manifest.ApprovalReadiness != ShadowApprovalReady ||
		manifest.MissingCount != 1 || manifest.StaleCount != 1 || manifest.ExtraCount != 1 ||
		manifest.ProductionApplied || len(manifest.ProductionMutations) != 0 || manifest.BindingSHA256 == "" {
		t.Fatalf("unexpected real-service shadow manifest: %+v", manifest)
	}
	sourceAfter, sourceTruncated, err := authority.ListProjectionAlerts(ctx, scope)
	if err != nil || sourceTruncated || len(sourceAfter) != len(sourceBefore) {
		t.Fatalf("authority changed during shadow: before=%d after=%d truncated=%t err=%v", len(sourceBefore), len(sourceAfter), sourceTruncated, err)
	}
	targetAfter, targetTruncated, err := target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(targetAfter) != len(targetBefore) {
		t.Fatalf("target changed during shadow: before=%d after=%d truncated=%t err=%v", len(targetBefore), len(targetAfter), targetTruncated, err)
	}
	for index := range sourceBefore {
		before, hashErr := persistence.AlertProjectionSHA256(sourceBefore[index])
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		after, hashErr := persistence.AlertProjectionSHA256(sourceAfter[index])
		if hashErr != nil || before != after {
			t.Fatalf("authority hash changed during shadow: before=%s after=%s err=%v", before, after, hashErr)
		}
	}
	for index := range targetBefore {
		before, hashErr := persistence.AlertProjectionSHA256(targetBefore[index])
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		after, hashErr := persistence.AlertProjectionSHA256(targetAfter[index])
		if hashErr != nil || before != after {
			t.Fatalf("target hash changed during shadow: before=%s after=%s err=%v", before, after, hashErr)
		}
	}
	t.Logf("shadow_binding_sha256=%s cluster_uuid=%s write_index=%s production_mutations=0", manifest.BindingSHA256, metadata.ClusterUUID, metadata.WriteIndices[0].Index)
}

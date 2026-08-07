package projection

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type ephemeralProjectionSource struct {
	alerts []*persistence.Alert
}

func (s ephemeralProjectionSource) ListProjectionAlerts(context.Context, persistence.ProjectionScope) ([]*persistence.Alert, bool, error) {
	return s.alerts, false, nil
}

type ephemeralReconcileStore struct {
	applied []string
	result  persistence.ProjectionReconcileResult
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
	return nil
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
		result.RepairedCount != 2 || result.ErrorCount != 0 || !result.VerificationPerformed || !result.RepairConverged ||
		result.RemainingMissingCount != 0 || result.RemainingStaleCount != 0 || result.RemainingExtraCount != 1 ||
		len(result.RemainingExtraIDs) != 1 || result.RemainingExtraIDs[0] != "repair-extra" ||
		strings.Join(store.applied, ",") != "repair-missing,repair-stale" || !store.result.RepairConverged {
		t.Fatalf("unexpected terminal repair receipt: result=%+v applied=%v stored=%+v", result, store.applied, store.result)
	}
}

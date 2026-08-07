package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type reconcileReader struct {
	alerts    []*persistence.Alert
	truncated bool
	err       error
}

func (r *reconcileReader) ListProjectionAlerts(context.Context, persistence.ProjectionScope) ([]*persistence.Alert, bool, error) {
	return r.alerts, r.truncated, r.err
}

type reconcileTarget struct {
	reconcileReader
	version     string
	writes      []string
	writeErr    error
	refreshErr  error
	ignoreWrite bool
}

func (t *reconcileTarget) WriteAlert(_ context.Context, alert *persistence.Alert) error {
	t.writes = append(t.writes, alert.AlertID)
	if t.writeErr != nil {
		return t.writeErr
	}
	if t.ignoreWrite {
		return nil
	}
	for index, current := range t.alerts {
		if current.AlertID == alert.AlertID {
			t.alerts[index] = alert
			return nil
		}
	}
	t.alerts = append(t.alerts, alert)
	return nil
}
func (t *reconcileTarget) RefreshProjectionTarget(context.Context) error { return t.refreshErr }
func (t *reconcileTarget) TargetVersion() string                         { return t.version }

type reconcileStore struct {
	started, completed int
	applied            []string
	result             persistence.ProjectionReconcileResult
	appliedErr         error
	watermarks         map[string]bool
	watermarkReadErr   error
}

func (s *reconcileStore) StartProjectionReconcileRun(context.Context, persistence.ProjectionReconcileRun, int) error {
	s.started++
	return nil
}
func (s *reconcileStore) CompleteProjectionReconcileRun(_ context.Context, _ string, result persistence.ProjectionReconcileResult) error {
	s.completed++
	s.result = result
	return nil
}
func (s *reconcileStore) RecordProjectionApplied(_ context.Context, alert *persistence.Alert, _ string) error {
	s.applied = append(s.applied, alert.AlertID)
	if s.appliedErr != nil {
		return s.appliedErr
	}
	if s.watermarks == nil {
		s.watermarks = make(map[string]bool)
	}
	s.watermarks[alert.AlertID] = true
	return nil
}
func (s *reconcileStore) ListProjectionWatermarkMismatches(_ context.Context, alerts []*persistence.Alert, _ string) ([]string, error) {
	if s.watermarkReadErr != nil {
		return nil, s.watermarkReadErr
	}
	missing := make([]string, 0)
	for _, alert := range alerts {
		if !s.watermarks[alert.AlertID] {
			missing = append(missing, alert.AlertID)
		}
	}
	return missing, nil
}

func reconcileAlert(id, status string) *persistence.Alert {
	now := time.Unix(1_800_000_000, 0).UTC()
	return &persistence.Alert{TenantID: "tenant-a", AlertID: id, Status: status, FirstSeen: now, LastSeen: now, UpdatedTs: now}
}

func reconcileRequest(mode string) ReconcileRequest {
	return ReconcileRequest{Mode: mode, RequestedBy: "operator-a", TraceID: "trace-a", Scope: persistence.ProjectionScope{
		TenantID: "tenant-a", TargetIndexVersion: "alerts-v2-write", MaxDocuments: 100,
	}}
}

func newReconcilerTest(t *testing.T, source ProjectionReader, target ProjectionRepairTarget, store ReconcileStore, stop int) *Reconciler {
	t.Helper()
	r, err := NewReconciler(ReconcileConfig{MaxDocuments: 100, StopErrorCount: stop, RepairPerSecond: 100000}, source, target, store)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReconcileClassifiesMissingExtraAndStale(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new"), reconcileAlert("b", "closed")}}
	target := &reconcileTarget{reconcileReader: reconcileReader{alerts: []*persistence.Alert{reconcileAlert("b", "new"), reconcileAlert("c", "new")}}, version: "alerts-v2-write"}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 2).Run(context.Background(), reconcileRequest("plan"))
	if err != nil {
		t.Fatal(err)
	}
	if result.MissingCount != 1 || result.StaleCount != 1 || result.ExtraCount != 1 || len(target.writes) != 0 {
		t.Fatalf("unexpected classification: %+v writes=%v", result, target.writes)
	}
	if result.MissingIDs[0] != "a" || result.StaleIDs[0] != "b" || result.ExtraIDs[0] != "c" {
		t.Fatalf("unexpected IDs: %+v", result)
	}
}

func TestRepairWritesOnlyMissingAndStaleNeverExtra(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new"), reconcileAlert("b", "closed")}}
	target := &reconcileTarget{reconcileReader: reconcileReader{alerts: []*persistence.Alert{reconcileAlert("b", "new"), reconcileAlert("c", "new")}}, version: "alerts-v2-write"}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 2).Run(context.Background(), reconcileRequest("repair"))
	if err != nil {
		t.Fatal(err)
	}
	if result.RepairedCount != 2 || len(target.writes) != 2 || len(store.applied) != 2 {
		t.Fatalf("unexpected repair outcome: %+v writes=%v applied=%v", result, target.writes, store.applied)
	}
	if !result.VerificationPerformed || !result.WatermarksConverged || !result.RepairConverged || result.WatermarkMismatchCount != 0 || result.RemainingMissingCount != 0 || result.RemainingStaleCount != 0 || result.RemainingExtraCount != 1 {
		t.Fatalf("repair did not produce a verified terminal receipt: %+v", result)
	}
	for _, id := range target.writes {
		if id == "c" {
			t.Fatal("extra projection must never be automatically deleted or rewritten")
		}
	}
}

func TestReconcileFailsReceiptWhenRefreshFails(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new")}}
	target := &reconcileTarget{version: "alerts-v2-write", refreshErr: errors.New("refresh rejected")}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 2).Run(context.Background(), reconcileRequest("repair"))
	if err == nil || result.Status != "partial" || result.StopReason != "post_repair_refresh_failed" || result.VerificationPerformed || store.completed != 1 {
		t.Fatalf("refresh failure did not fail the terminal receipt: result=%+v err=%v completed=%d", result, err, store.completed)
	}
}

func TestReconcileDoesNotConvergeWhenAcknowledgedWriteIsNotVisible(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new")}}
	target := &reconcileTarget{version: "alerts-v2-write", ignoreWrite: true}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 2).Run(context.Background(), reconcileRequest("repair"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.StopReason != "post_repair_differences_remain" || result.RepairConverged || result.RemainingMissingCount != 1 {
		t.Fatalf("unobservable write was incorrectly reported as converged: %+v", result)
	}
}

func TestReconcileDoesNotConvergeWhenWatermarkWriteFails(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new")}}
	target := &reconcileTarget{version: "alerts-v2-write"}
	store := &reconcileStore{appliedErr: errors.New("PostgreSQL watermark unavailable")}
	result, err := newReconcilerTest(t, source, target, store, 2).Run(context.Background(), reconcileRequest("repair"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.StopReason != "post_repair_watermark_mismatches_remain" || result.RepairConverged || result.WatermarksConverged || result.WatermarkMismatchCount != 1 || result.RemainingMissingCount != 0 || result.ErrorCount != 1 {
		t.Fatalf("missing watermark was incorrectly reported as converged: %+v", result)
	}
}

func TestReconcileRetriesMissingWatermarkAfterTargetAlreadyConverged(t *testing.T) {
	alert := reconcileAlert("a", "new")
	source := &reconcileReader{alerts: []*persistence.Alert{alert}}
	target := &reconcileTarget{reconcileReader: reconcileReader{alerts: []*persistence.Alert{alert}}, version: "alerts-v2-write"}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 2).Run(context.Background(), reconcileRequest("repair"))
	if err != nil {
		t.Fatal(err)
	}
	if result.RepairedCount != 0 || len(target.writes) != 0 || len(store.applied) != 1 || store.applied[0] != "a" || !result.WatermarksConverged || !result.RepairConverged {
		t.Fatalf("converged target did not recover its missing watermark: result=%+v writes=%v applied=%v", result, target.writes, store.applied)
	}
}

func TestReconcileStopsOnTruncationBeforeRepair(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new")}, truncated: true}
	target := &reconcileTarget{version: "alerts-v2-write"}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 1).Run(context.Background(), reconcileRequest("repair"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Partial || result.StopReason != "bounded_scope_truncated" || len(target.writes) != 0 {
		t.Fatalf("truncated scope did not fail closed: %+v writes=%v", result, target.writes)
	}
}

func TestReconcileStopsAtRepairErrorThreshold(t *testing.T) {
	source := &reconcileReader{alerts: []*persistence.Alert{reconcileAlert("a", "new"), reconcileAlert("b", "new")}}
	target := &reconcileTarget{version: "alerts-v2-write", writeErr: errors.New("OpenSearch rejected write")}
	store := &reconcileStore{}
	result, err := newReconcilerTest(t, source, target, store, 1).Run(context.Background(), reconcileRequest("repair"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "stopped" || result.ErrorCount != 1 || len(target.writes) != 1 {
		t.Fatalf("repair did not stop at threshold: %+v writes=%v", result, target.writes)
	}
}

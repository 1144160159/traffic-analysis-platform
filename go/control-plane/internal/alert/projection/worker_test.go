package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type workerTestStore struct {
	debts    []persistence.ProjectionDebt
	resolved []persistence.ProjectionDebt
	retried  []persistence.ProjectionDebt
}

func (s *workerTestStore) ClaimProjectionDebts(context.Context, string, int, time.Duration) ([]persistence.ProjectionDebt, error) {
	return s.debts, nil
}
func (s *workerTestStore) ResolveProjectionDebt(_ context.Context, _ string, debt persistence.ProjectionDebt, _ *persistence.Alert) error {
	s.resolved = append(s.resolved, debt)
	return nil
}
func (s *workerTestStore) RetryProjectionDebt(_ context.Context, _ string, debt persistence.ProjectionDebt, _ error, _ int) error {
	s.retried = append(s.retried, debt)
	return nil
}

type workerTestSource struct {
	alert *persistence.Alert
	err   error
}

func (s *workerTestSource) GetByID(context.Context, string, string) (*persistence.Alert, error) {
	return s.alert, s.err
}

type workerTestTarget struct {
	version string
	err     error
	writes  int
}

func (t *workerTestTarget) WriteAlert(context.Context, *persistence.Alert) error {
	t.writes++
	return t.err
}
func (t *workerTestTarget) TargetVersion() string { return t.version }

func newWorkerForTest(t *testing.T, store DebtStore, source AlertSource, target AlertTarget) *Worker {
	t.Helper()
	worker, err := NewWorker(WorkerConfig{Interval: time.Second, Lease: time.Minute, BatchSize: 10, MaxAttempts: 3}, store, source, target, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func TestWorkerRepairsAuthoritativeProjectionAndResolvesDebt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	alert := &persistence.Alert{TenantID: "tenant-a", AlertID: "alert-a", FirstSeen: now, LastSeen: now, UpdatedTs: now}
	debt := persistence.ProjectionDebt{TenantID: "tenant-a", AlertID: "alert-a", SourceVersion: persistence.AlertSourceVersion(alert), TargetIndexVersion: "alerts-v2-write", AttemptCount: 1}
	store := &workerTestStore{debts: []persistence.ProjectionDebt{debt}}
	target := &workerTestTarget{version: "alerts-v2-write"}
	worker := newWorkerForTest(t, store, &workerTestSource{alert: alert}, target)
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if target.writes != 1 || len(store.resolved) != 1 || len(store.retried) != 0 {
		t.Fatalf("unexpected repair outcome: writes=%d resolved=%d retried=%d", target.writes, len(store.resolved), len(store.retried))
	}
}

func TestWorkerDoesNotWriteWrongIndexGeneration(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	alert := &persistence.Alert{TenantID: "tenant-a", AlertID: "alert-a", UpdatedTs: now}
	store := &workerTestStore{debts: []persistence.ProjectionDebt{{TenantID: "tenant-a", AlertID: "alert-a", SourceVersion: 1, TargetIndexVersion: "alerts-v1", AttemptCount: 1}}}
	target := &workerTestTarget{version: "alerts-v2-write"}
	worker := newWorkerForTest(t, store, &workerTestSource{alert: alert}, target)
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("target mismatch must fail")
	}
	if target.writes != 0 || len(store.retried) != 1 || len(store.resolved) != 0 {
		t.Fatalf("wrong generation was not stopped: writes=%d retried=%d", target.writes, len(store.retried))
	}
}

func TestWorkerRetriesWhenSourceOrTargetFails(t *testing.T) {
	debt := persistence.ProjectionDebt{TenantID: "tenant-a", AlertID: "alert-a", SourceVersion: 1, TargetIndexVersion: "alerts-v2-write", AttemptCount: 1}
	store := &workerTestStore{debts: []persistence.ProjectionDebt{debt}}
	worker := newWorkerForTest(t, store, &workerTestSource{err: errors.New("ClickHouse unavailable")}, &workerTestTarget{version: "alerts-v2-write"})
	if err := worker.RunOnce(context.Background()); err == nil || len(store.retried) != 1 {
		t.Fatalf("source failure must be retried: err=%v retried=%d", err, len(store.retried))
	}
}

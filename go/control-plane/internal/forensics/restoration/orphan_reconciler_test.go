package restoration

import (
	"context"
	"testing"
	"time"
)

type fakeOrphanAuthority struct {
	observedBefore time.Time
	limit          int
	report         OrphanReconciliationReport
	err            error
}

func (authority *fakeOrphanAuthority) ReconcileOrphans(_ context.Context, observedBefore time.Time, limit int) (OrphanReconciliationReport, error) {
	authority.observedBefore = observedBefore
	authority.limit = limit
	return authority.report, authority.err
}

func TestOrphanReconcilerAppliesGraceWindowAndBatchLimit(t *testing.T) {
	authority := &fakeOrphanAuthority{report: OrphanReconciliationReport{Scanned: 3, Reconciled: 1, Conflicts: 1, Pending: 1}}
	reconciler, err := NewOrphanReconciler(authority, OrphanReconcilerConfig{
		WorkerID: "worker-a", Interval: time.Minute, GracePeriod: 15 * time.Minute, BatchSize: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)
	report, err := reconciler.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if authority.observedBefore != now.Add(-15*time.Minute) || authority.limit != 100 || report.Scanned != 3 {
		t.Fatalf("reconcile window/limit/report = %v/%d/%+v", authority.observedBefore, authority.limit, report)
	}
}

func TestOrphanReconcilerRejectsUnboundedScheduling(t *testing.T) {
	if _, err := NewOrphanReconciler(&fakeOrphanAuthority{}, OrphanReconcilerConfig{WorkerID: "worker-a"}); err == nil {
		t.Fatal("NewOrphanReconciler() accepted unbounded scheduling")
	}
}

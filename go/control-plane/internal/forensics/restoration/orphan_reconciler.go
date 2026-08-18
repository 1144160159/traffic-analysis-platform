package restoration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

type OrphanReconciliationAuthority interface {
	ReconcileOrphans(context.Context, time.Time, int) (OrphanReconciliationReport, error)
}

type OrphanReconcilerConfig struct {
	WorkerID    string
	Interval    time.Duration
	GracePeriod time.Duration
	BatchSize   int
	Logger      *zap.Logger
}

type OrphanReconciler struct {
	authority OrphanReconciliationAuthority
	config    OrphanReconcilerConfig
}

func NewOrphanReconciler(authority OrphanReconciliationAuthority, config OrphanReconcilerConfig) (*OrphanReconciler, error) {
	if authority == nil || strings.TrimSpace(config.WorkerID) == "" {
		return nil, errors.New("restoration orphan reconciler authority and worker identity are required")
	}
	if config.Interval <= 0 || config.GracePeriod <= 0 || config.BatchSize <= 0 || config.BatchSize > 1000 {
		return nil, errors.New("restoration orphan reconciler requires positive bounded scheduling")
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}
	return &OrphanReconciler{authority: authority, config: config}, nil
}

func (reconciler *OrphanReconciler) RunOnce(ctx context.Context, now time.Time) (OrphanReconciliationReport, error) {
	if now.IsZero() {
		return OrphanReconciliationReport{}, errors.New("restoration orphan reconciliation time is required")
	}
	report, err := reconciler.authority.ReconcileOrphans(ctx, now.UTC().Add(-reconciler.config.GracePeriod), reconciler.config.BatchSize)
	if err != nil {
		return OrphanReconciliationReport{}, fmt.Errorf("reconcile restoration orphans: %w", err)
	}
	return report, nil
}

func (reconciler *OrphanReconciler) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			report, err := reconciler.RunOnce(ctx, now)
			if err != nil {
				reconciler.config.Logger.Warn("Restoration orphan reconciliation failed",
					zap.String("worker_id", reconciler.config.WorkerID), zap.Error(err))
			} else if report.Scanned > 0 {
				reconciler.config.Logger.Info("Restoration orphan reconciliation completed",
					zap.String("worker_id", reconciler.config.WorkerID), zap.Int("scanned", report.Scanned),
					zap.Int("reconciled", report.Reconciled), zap.Int("conflicts", report.Conflicts),
					zap.Int("pending", report.Pending))
			}
			timer.Reset(reconciler.config.Interval)
		}
	}
}

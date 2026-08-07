package projection

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type AlertSource interface {
	GetByID(context.Context, string, string) (*persistence.Alert, error)
}

type DebtStore interface {
	ClaimProjectionDebts(context.Context, string, int, time.Duration) ([]persistence.ProjectionDebt, error)
	ResolveProjectionDebt(context.Context, string, persistence.ProjectionDebt, *persistence.Alert) error
	RetryProjectionDebt(context.Context, string, persistence.ProjectionDebt, error, int) error
}

type AlertTarget interface {
	WriteAlert(context.Context, *persistence.Alert) error
	TargetVersion() string
}

type WorkerConfig struct {
	Interval    time.Duration
	Lease       time.Duration
	BatchSize   int
	MaxAttempts int
}

type Worker struct {
	config   WorkerConfig
	store    DebtStore
	source   AlertSource
	target   AlertTarget
	workerID string
	logger   *zap.Logger
}

func NewWorker(config WorkerConfig, store DebtStore, source AlertSource, target AlertTarget, logger *zap.Logger) (*Worker, error) {
	if store == nil || source == nil || target == nil || logger == nil {
		return nil, fmt.Errorf("alert projection worker dependencies are required")
	}
	if config.Interval <= 0 || config.Lease <= 0 || config.BatchSize < 1 || config.BatchSize > 1000 || config.MaxAttempts < 1 {
		return nil, fmt.Errorf("invalid alert projection worker budget")
	}
	return &Worker{config: config, store: store, source: source, target: target, workerID: uuid.NewString(), logger: logger}, nil
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	for {
		if err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			w.logger.Error("Alert OpenSearch projection repair iteration failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	debts, err := w.store.ClaimProjectionDebts(ctx, w.workerID, w.config.BatchSize, w.config.Lease)
	if err != nil {
		return err
	}
	var firstErr error
	for _, debt := range debts {
		if err := w.repair(ctx, debt); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if retryErr := w.store.RetryProjectionDebt(ctx, w.workerID, debt, err, w.config.MaxAttempts); retryErr != nil {
				return fmt.Errorf("repair alert %s failed (%v) and retry state failed: %w", debt.AlertID, err, retryErr)
			}
		}
	}
	return firstErr
}

func (w *Worker) repair(ctx context.Context, debt persistence.ProjectionDebt) error {
	if debt.TargetIndexVersion != w.target.TargetVersion() {
		return fmt.Errorf("projection target mismatch: debt=%s worker=%s", debt.TargetIndexVersion, w.target.TargetVersion())
	}
	alert, err := w.source.GetByID(ctx, debt.TenantID, debt.AlertID)
	if err != nil {
		return fmt.Errorf("read authoritative ClickHouse alert: %w", err)
	}
	if alert.TenantID != debt.TenantID || alert.AlertID != debt.AlertID {
		return fmt.Errorf("authoritative alert identity mismatch")
	}
	if persistence.AlertSourceVersion(alert) < debt.SourceVersion {
		return fmt.Errorf("authoritative source version %d is behind debt %d", persistence.AlertSourceVersion(alert), debt.SourceVersion)
	}
	if err := w.target.WriteAlert(ctx, alert); err != nil {
		return fmt.Errorf("repair OpenSearch projection: %w", err)
	}
	if err := w.store.ResolveProjectionDebt(ctx, w.workerID, debt, alert); err != nil {
		return err
	}
	w.logger.Info("Alert OpenSearch projection debt resolved",
		zap.String("tenant_id", debt.TenantID), zap.String("alert_id", debt.AlertID),
		zap.Int64("source_version", persistence.AlertSourceVersion(alert)),
		zap.String("target_index_version", debt.TargetIndexVersion))
	return nil
}

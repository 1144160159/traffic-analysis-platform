package projection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
)

type ProjectionReader interface {
	ListProjectionAlerts(context.Context, persistence.ProjectionScope) ([]*persistence.Alert, bool, error)
}

type ProjectionRepairTarget interface {
	ProjectionReader
	WriteAlert(context.Context, *persistence.Alert) error
	TargetVersion() string
}

type ReconcileStore interface {
	StartProjectionReconcileRun(context.Context, persistence.ProjectionReconcileRun, int) error
	CompleteProjectionReconcileRun(context.Context, string, persistence.ProjectionReconcileResult) error
	RecordProjectionApplied(context.Context, *persistence.Alert, string) error
}

type ReconcileConfig struct {
	MaxDocuments    int
	StopErrorCount  int
	RepairPerSecond int
}

type ReconcileRequest struct {
	Mode, RequestedBy, TraceID string
	Scope                      persistence.ProjectionScope
}

type Reconciler struct {
	config ReconcileConfig
	source ProjectionReader
	target ProjectionRepairTarget
	store  ReconcileStore
}

func NewReconciler(config ReconcileConfig, source ProjectionReader, target ProjectionRepairTarget, store ReconcileStore) (*Reconciler, error) {
	if source == nil || target == nil || store == nil {
		return nil, fmt.Errorf("alert projection reconciler dependencies are required")
	}
	if config.MaxDocuments < 1 || config.MaxDocuments > 100000 || config.StopErrorCount < 1 || config.RepairPerSecond < 1 {
		return nil, fmt.Errorf("invalid alert projection reconcile budget")
	}
	return &Reconciler{config: config, source: source, target: target, store: store}, nil
}

func (r *Reconciler) Run(ctx context.Context, request ReconcileRequest) (persistence.ProjectionReconcileResult, error) {
	result := persistence.ProjectionReconcileResult{Status: "failed"}
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode != "plan" && request.Mode != "repair" {
		return result, fmt.Errorf("reconcile mode must be plan or repair")
	}
	if strings.TrimSpace(request.Scope.TenantID) == "" || strings.TrimSpace(request.RequestedBy) == "" || strings.TrimSpace(request.TraceID) == "" {
		return result, fmt.Errorf("tenant_id, requested_by, and trace_id are required")
	}
	if request.Scope.MaxDocuments == 0 {
		request.Scope.MaxDocuments = r.config.MaxDocuments
	}
	if request.Scope.MaxDocuments < 1 || request.Scope.MaxDocuments > r.config.MaxDocuments {
		return result, fmt.Errorf("max_documents exceeds approved budget %d", r.config.MaxDocuments)
	}
	if request.Scope.TargetIndexVersion != r.target.TargetVersion() {
		return result, fmt.Errorf("target index version mismatch: request=%s current=%s", request.Scope.TargetIndexVersion, r.target.TargetVersion())
	}
	if !request.Scope.StartTime.IsZero() && !request.Scope.EndTime.IsZero() && request.Scope.EndTime.Before(request.Scope.StartTime) {
		return result, fmt.Errorf("end_time precedes start_time")
	}
	runID := uuid.NewString()
	run := persistence.ProjectionReconcileRun{RunID: runID, TenantID: request.Scope.TenantID, RequestedBy: request.RequestedBy, TraceID: request.TraceID, Mode: request.Mode, Scope: request.Scope}
	if err := r.store.StartProjectionReconcileRun(ctx, run, r.config.StopErrorCount); err != nil {
		return result, err
	}
	complete := func() error { return r.store.CompleteProjectionReconcileRun(ctx, runID, result) }

	sourceAlerts, sourceTruncated, err := r.source.ListProjectionAlerts(ctx, request.Scope)
	if err != nil {
		result.StopReason = "authoritative_source_read_failed"
		_ = complete()
		return result, err
	}
	targetAlerts, targetTruncated, err := r.target.ListProjectionAlerts(ctx, request.Scope)
	if err != nil {
		result.SourceCount = len(sourceAlerts)
		result.StopReason = "projection_target_read_failed"
		_ = complete()
		return result, err
	}
	result.SourceCount, result.TargetCount = len(sourceAlerts), len(targetAlerts)
	if sourceTruncated || targetTruncated {
		result.Status, result.Partial, result.StopReason = "partial", true, "bounded_scope_truncated"
		if err := complete(); err != nil {
			return result, err
		}
		return result, nil
	}

	sourceByID, err := projectionMap(sourceAlerts)
	if err != nil {
		result.StopReason = "invalid_authoritative_identity"
		_ = complete()
		return result, err
	}
	targetByID, err := projectionMap(targetAlerts)
	if err != nil {
		result.StopReason = "invalid_projection_identity"
		_ = complete()
		return result, err
	}
	for id, sourceAlert := range sourceByID {
		targetAlert, exists := targetByID[id]
		if !exists {
			result.MissingIDs = append(result.MissingIDs, id)
			continue
		}
		sourceHash, _ := persistence.AlertProjectionSHA256(sourceAlert)
		targetHash, _ := persistence.AlertProjectionSHA256(targetAlert)
		if sourceHash != targetHash {
			result.StaleIDs = append(result.StaleIDs, id)
		}
	}
	for id := range targetByID {
		if _, exists := sourceByID[id]; !exists {
			result.ExtraIDs = append(result.ExtraIDs, id)
		}
	}
	sort.Strings(result.MissingIDs)
	sort.Strings(result.StaleIDs)
	sort.Strings(result.ExtraIDs)
	result.MissingCount, result.StaleCount, result.ExtraCount = len(result.MissingIDs), len(result.StaleIDs), len(result.ExtraIDs)
	result.Status = "completed"

	if request.Mode == "repair" {
		ids := append(append([]string{}, result.MissingIDs...), result.StaleIDs...)
		interval := time.Second / time.Duration(r.config.RepairPerSecond)
		for index, id := range ids {
			if index > 0 && interval > 0 {
				timer := time.NewTimer(interval)
				select {
				case <-ctx.Done():
					timer.Stop()
					result.Status, result.Partial, result.StopReason = "stopped", true, "context_cancelled"
					break
				case <-timer.C:
				}
			}
			if result.Status == "stopped" {
				break
			}
			alert := sourceByID[id]
			if err := r.target.WriteAlert(ctx, alert); err != nil {
				result.ErrorCount++
				if result.ErrorCount >= r.config.StopErrorCount {
					result.Status, result.Partial, result.StopReason = "stopped", true, "repair_error_threshold_reached"
					break
				}
				continue
			}
			if err := r.store.RecordProjectionApplied(ctx, alert, request.Scope.TargetIndexVersion); err != nil {
				result.ErrorCount++
				if result.ErrorCount >= r.config.StopErrorCount {
					result.Status, result.Partial, result.StopReason = "stopped", true, "watermark_error_threshold_reached"
					break
				}
				continue
			}
			result.RepairedCount++
		}
		if result.ErrorCount > 0 && result.Status == "completed" {
			result.Status, result.Partial, result.StopReason = "partial", true, "repair_errors_below_stop_threshold"
		}
	}
	if err := complete(); err != nil {
		return result, err
	}
	return result, nil
}

func projectionMap(alerts []*persistence.Alert) (map[string]*persistence.Alert, error) {
	result := make(map[string]*persistence.Alert, len(alerts))
	for _, alert := range alerts {
		if alert == nil || strings.TrimSpace(alert.TenantID) == "" || strings.TrimSpace(alert.AlertID) == "" {
			return nil, fmt.Errorf("projection record has empty identity")
		}
		if _, exists := result[alert.AlertID]; exists {
			return nil, fmt.Errorf("duplicate projection alert_id %s", alert.AlertID)
		}
		result[alert.AlertID] = alert
	}
	return result, nil
}

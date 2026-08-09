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
	RefreshProjectionTarget(context.Context) error
	TargetVersion() string
}

type ReconcileStore interface {
	StartProjectionReconcileRun(context.Context, persistence.ProjectionReconcileRun, int) error
	CompleteProjectionReconcileRun(context.Context, string, persistence.ProjectionReconcileResult) error
	RecordProjectionApplied(context.Context, *persistence.Alert, string) error
	ListProjectionWatermarkMismatches(context.Context, []*persistence.Alert, string) ([]string, error)
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
	result.MissingIDs, result.ExtraIDs, result.StaleIDs = projectionDiff(sourceByID, targetByID)
	result.MissingCount, result.StaleCount, result.ExtraCount = len(result.MissingIDs), len(result.StaleIDs), len(result.ExtraIDs)
	result.Status = "completed"

	if request.Mode == "repair" {
		ids := append(append([]string{}, result.MissingIDs...), result.StaleIDs...)
		writtenIDs := make(map[string]struct{}, len(ids))
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
			writtenIDs[id] = struct{}{}
		}
		if result.ErrorCount > 0 && result.Status == "completed" {
			result.Status, result.Partial, result.StopReason = "partial", true, "repair_errors_below_stop_threshold"
		}
		if result.Status != "stopped" {
			if err := r.target.RefreshProjectionTarget(ctx); err != nil {
				result.ErrorCount++
				result.Status, result.Partial, result.StopReason = "partial", true, "post_repair_refresh_failed"
				_ = complete()
				return result, err
			}
			verifiedAlerts, verifiedTruncated, err := r.target.ListProjectionAlerts(ctx, request.Scope)
			if err != nil {
				result.ErrorCount++
				result.Status, result.Partial, result.StopReason = "partial", true, "post_repair_target_read_failed"
				_ = complete()
				return result, err
			}
			result.VerificationPerformed = true
			result.VerificationTargetCount = len(verifiedAlerts)
			if verifiedTruncated {
				result.Status, result.Partial, result.StopReason = "partial", true, "post_repair_scope_truncated"
			} else {
				verifiedByID, mapErr := projectionMap(verifiedAlerts)
				if mapErr != nil {
					result.ErrorCount++
					result.Status, result.Partial, result.StopReason = "partial", true, "invalid_post_repair_projection_identity"
					_ = complete()
					return result, mapErr
				}
				result.RemainingMissingIDs, result.RemainingExtraIDs, result.RemainingStaleIDs = projectionDiff(sourceByID, verifiedByID)
				result.RemainingMissingCount = len(result.RemainingMissingIDs)
				result.RemainingExtraCount = len(result.RemainingExtraIDs)
				result.RemainingStaleCount = len(result.RemainingStaleIDs)
				remainingRepairable := make(map[string]struct{}, result.RemainingMissingCount+result.RemainingStaleCount)
				for _, id := range result.RemainingMissingIDs {
					remainingRepairable[id] = struct{}{}
				}
				for _, id := range result.RemainingStaleIDs {
					remainingRepairable[id] = struct{}{}
				}
				watermarkAlerts := make([]*persistence.Alert, 0, len(sourceByID)-len(remainingRepairable))
				for id, alert := range sourceByID {
					if _, unresolved := remainingRepairable[id]; !unresolved {
						watermarkAlerts = append(watermarkAlerts, alert)
					}
				}
				sort.Slice(watermarkAlerts, func(i, j int) bool { return watermarkAlerts[i].AlertID < watermarkAlerts[j].AlertID })
				watermarkMismatches, watermarkErr := r.store.ListProjectionWatermarkMismatches(ctx, watermarkAlerts, request.Scope.TargetIndexVersion)
				if watermarkErr != nil {
					result.ErrorCount++
					result.Status, result.Partial, result.StopReason = "partial", true, "post_repair_watermark_read_failed"
					_ = complete()
					return result, watermarkErr
				}
				for _, id := range watermarkMismatches {
					if err := r.store.RecordProjectionApplied(ctx, sourceByID[id], request.Scope.TargetIndexVersion); err != nil {
						result.ErrorCount++
						if result.ErrorCount >= r.config.StopErrorCount {
							result.Status, result.Partial, result.StopReason = "stopped", true, "watermark_error_threshold_reached"
							break
						}
					}
				}
				result.WatermarkMismatchIDs, watermarkErr = r.store.ListProjectionWatermarkMismatches(ctx, watermarkAlerts, request.Scope.TargetIndexVersion)
				if watermarkErr != nil {
					result.ErrorCount++
					result.Status, result.Partial, result.StopReason = "partial", true, "post_repair_watermark_verify_failed"
					_ = complete()
					return result, watermarkErr
				}
				sort.Strings(result.WatermarkMismatchIDs)
				result.WatermarkMismatchCount = len(result.WatermarkMismatchIDs)
				result.WatermarksConverged = result.WatermarkMismatchCount == 0
				for id := range writtenIDs {
					if _, unresolved := remainingRepairable[id]; unresolved {
						continue
					}
					if !containsSortedString(result.WatermarkMismatchIDs, id) {
						result.RepairedCount++
					}
				}
				result.RepairConverged = result.RemainingMissingCount == 0 && result.RemainingStaleCount == 0 && result.WatermarksConverged && result.ErrorCount == 0
				if !result.RepairConverged && result.Status == "completed" {
					stopReason := "post_repair_differences_remain"
					if result.WatermarkMismatchCount > 0 {
						stopReason = "post_repair_watermark_mismatches_remain"
					}
					result.Status, result.Partial, result.StopReason = "partial", true, stopReason
				}
			}
		}
	}
	if err := complete(); err != nil {
		return result, err
	}
	return result, nil
}

func containsSortedString(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func projectionDiff(sourceByID, targetByID map[string]*persistence.Alert) (missing, extra, stale []string) {
	for id, sourceAlert := range sourceByID {
		targetAlert, exists := targetByID[id]
		if !exists {
			missing = append(missing, id)
			continue
		}
		sourceHash, _ := persistence.AlertProjectionSHA256(sourceAlert)
		targetHash, _ := persistence.AlertProjectionSHA256(targetAlert)
		if sourceHash != targetHash {
			stale = append(stale, id)
		}
	}
	for id := range targetByID {
		if _, exists := sourceByID[id]; !exists {
			extra = append(extra, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(stale)
	return missing, extra, stale
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

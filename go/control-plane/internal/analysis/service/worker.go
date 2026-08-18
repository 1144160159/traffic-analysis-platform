package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// MaterializeWorker 物化 worker:领取 PENDING_MATERIALIZATION 触发→解析计划→物化事务。
// 多副本安全(FOR UPDATE SKIP LOCKED);同 identity 幂等由事务层保证。
type MaterializeWorker struct {
	repo   *repository.Repo
	logger *zap.Logger
	now    func() time.Time
}

func NewMaterializeWorker(repo *repository.Repo, logger *zap.Logger) *MaterializeWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MaterializeWorker{repo: repo, logger: logger, now: time.Now}
}

// ProcessOnce 处理一批(最多 limit 个)待物化触发。
func (w *MaterializeWorker) ProcessOnce(ctx context.Context, limit int) (processed int, err error) {
	for i := 0; i < limit; i++ {
		trigger, err := w.repo.NextPendingTrigger(ctx)
		if err != nil {
			return processed, fmt.Errorf("next pending trigger: %w", err)
		}
		if trigger == nil {
			return processed, nil // 无待处理
		}

		// 调度触发绑定冻结修订(触发器实例携带 plan_revision);仅无修订的
		// 遗留触发回落到当前激活计划(不随激活头漂移)。
		var plan *repository.ActivePlanRow
		if trigger.PlanRevision > 0 {
			plan, err = w.repo.GetPlanByDefinitionAndRevision(ctx, trigger.TenantID, trigger.TaskDefinitionID, trigger.PlanRevision)
		} else {
			plan, err = w.repo.GetActivePlanForDefinition(ctx, trigger.TenantID, trigger.TaskDefinitionID)
		}
		if err != nil {
			w.logger.Warn("pending trigger has no active plan; quarantined as suppressed",
				zap.String("trigger_id", trigger.TriggerID), zap.Error(err))
			// 无激活计划:置 SUPPRESSED(不伪造 CANCELLED Task)
			_ = w.suppressTrigger(ctx, trigger.TriggerID)
			continue
		}

		windowStart, windowEnd := windowFromTrigger(trigger, plan, w.now())
		// 有效调度策略(§76.45.2):class = schedule.class ?? definition.default_class;
		// requested = plan.resource_budget;cap = schedule resource_restrictions 逐维最小。
		policy, policyErr := ResolveEffectiveSchedulingPolicy(PolicyInputs{
			ScheduleClass:        trigger.EffectiveClass,
			PlanBudget:           plan.ResourceBudget,
			ScheduleRestrictions: json.RawMessage(trigger.ResourceRestrictions),
		})
		if policyErr != nil {
			w.logger.Warn("effective policy resolution failed; trigger suppressed",
				zap.String("trigger_id", trigger.TriggerID), zap.Error(policyErr))
			_ = w.suppressTrigger(ctx, trigger.TriggerID)
			continue
		}
		queueCost, costErr := DRRVectorCost(policy.ResourceVector)
		if costErr != nil {
			w.logger.Warn("drr vector cost failed; trigger suppressed",
				zap.String("trigger_id", trigger.TriggerID), zap.Error(costErr))
			_ = w.suppressTrigger(ctx, trigger.TriggerID)
			continue
		}
		cmd := repository.MaterializeCommand{
			TenantID:              trigger.TenantID,
			IdentityKind:          trigger.IdentityKind,
			CanonicalIdentityHash: trigger.CanonicalIdentityHash,
			RequestSHA256:         trigger.RequestSHA256,
			TriggerInstanceID:     trigger.TriggerID,
			TriggerKind:           trigger.TriggerKind,
			WindowID:              trigger.WindowID,
			WindowStartMs:         windowStart,
			WindowEndMs:           windowEnd,
			TaskDefinitionID:      trigger.TaskDefinitionID,
			PlanRevision:          plan.PlanRevision,
			ExecutionSpecSHA256:   plan.ExecutionSpecSHA256,
			ScheduleRevision:      trigger.ScheduleRevision,
			EffectiveClass:        policy.Class,
			EffectivePolicySHA256: policy.PolicySHA256,
			ResourcePool:          policy.ResourcePool,
			ResourceVectorJSON:    policy.ResourceVector,
			QueueCostMilli:        queueCost,
			ExpiresAt:             w.now().Add(5 * time.Minute),
			NodesJSON:             defaultNodeExactSet(),
			PlanSpecJSON:          plan.StageDAG,
		}
		receipt, replayed, err := w.repo.MaterializeAnalysisTaskAtomic(ctx, cmd)
		if err != nil {
			w.logger.Error("materialize failed", zap.String("trigger_id", trigger.TriggerID), zap.Error(err))
			return processed, fmt.Errorf("materialize: %w", err)
		}
		if replayed {
			w.logger.Debug("trigger replay (already materialized)", zap.String("trigger_id", trigger.TriggerID))
			continue
		}
		w.logger.Info("materialized run",
			zap.String("task_id", receipt.TaskID), zap.String("run_id", receipt.RunID),
			zap.String("trigger_id", trigger.TriggerID))
		processed++
	}
	return processed, nil
}

// suppressTrigger 无激活计划的触发置 SUPPRESSED(事实保留,不创建 Task)。
func (w *MaterializeWorker) suppressTrigger(ctx context.Context, triggerID string) error {
	_, err := w.repo.SuppressTrigger(ctx, triggerID, "NO_ACTIVE_PLAN")
	return err
}

// windowFromTrigger 从触发事实推导窗口(schedule: windowID 编码;on-demand: 当前起 10 分钟)。
func windowFromTrigger(t *repository.PendingTrigger, plan *repository.ActivePlanRow, now time.Time) (int64, int64) {
	if strings.HasPrefix(t.WindowID, "w-") {
		parts := strings.Split(t.WindowID, "-")
		if len(parts) >= 3 {
			if start, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				if dur, err := strconv.ParseInt(parts[2], 10, 64); err == nil && dur > 0 {
					return start, start + dur
				}
			}
		}
	}
	return now.UnixMilli(), now.Add(10 * time.Minute).UnixMilli()
}

// nodeExactSetFromPlan 从 plan stage_dag 提取 required ExecutionNode exact-set。
func nodeExactSetFromPlan(stageDAG json.RawMessage) []byte {
	if len(stageDAG) == 0 {
		return []byte(`[]`)
	}
	var dag struct {
		Nodes []map[string]interface{} `json:"nodes"`
	}
	if err := json.Unmarshal(stageDAG, &dag); err == nil && len(dag.Nodes) > 0 {
		out, err := json.Marshal(dag.Nodes)
		if err == nil {
			return out
		}
	}
	return []byte(`[]`)
}

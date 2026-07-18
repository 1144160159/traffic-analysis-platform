////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/service/mlops_orchestrator.go
// MLOps Self-Orchestration Engine — 自动触发训练流水线
//
// 自编排触发条件:
//   1. 反馈数据积累 — alert_feedback 新增标注超过阈值
//   2. FP 率劣化 — FP rate 超过告警阈值
//   3. 数据漂移 — 特征分布发生显著变化 (PSI > 0.25)
//   4. 定时调度 — CronWorkflow 每周触发 (已有)
//
// 决策流程:
//   CheckConditions() → Evaluate() → ShouldRetrain() → SubmitWorkflow()
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// =============================================================================
// MLOpsOrchestratorConfig
// =============================================================================

// MLOpsOrchestratorConfig 自编排配置
type MLOpsOrchestratorConfig struct {
	// 检查间隔
	CheckInterval time.Duration `env:"MLOPS_CHECK_INTERVAL" envDefault:"1h"`

	// 反馈阈值
	MinNewFeedbackCount   int `env:"MLOPS_MIN_NEW_FEEDBACK" envDefault:"500"`       // 新增标注数阈值
	FeedbackLookbackHours int `env:"MLOPS_FEEDBACK_LOOKBACK_HOURS" envDefault:"24"` // 回溯小时数

	// FP 率阈值
	MaxFPRate float64 `env:"MLOPS_MAX_FP_RATE" envDefault:"0.15"` // FP 率 > 15% 触发重训

	// 漂移阈值
	MaxPSI float64 `env:"MLOPS_MAX_PSI" envDefault:"0.25"` // PSI > 0.25 触发重训

	// 限制
	MinRetrainInterval  time.Duration `env:"MLOPS_MIN_RETRAIN_INTERVAL" envDefault:"12h"` // 最小重训间隔
	MaxConcurrentTrains int           `env:"MLOPS_MAX_CONCURRENT_TRAINS" envDefault:"1"`

	// Argo
	ArgoNamespace    string `env:"MLOPS_ARGO_NAMESPACE" envDefault:"traffic-analysis"`
	ArgoServerURL    string `env:"MLOPS_ARGO_SERVER_URL" envDefault:"http://argo-server.argo.svc:2746"`
	WorkflowTemplate string `env:"MLOPS_WORKFLOW_TEMPLATE" envDefault:"mlops-training-template"`

	// Automatic retraining is intentionally scoped to one configured tenant
	// and model. A global feedback query must never mutate another tenant's
	// model or create an ownership-less Argo workflow.
	AutomatedTenantID  string `env:"MLOPS_AUTOMATED_TENANT_ID" envDefault:"default"`
	AutomatedModelName string `env:"MLOPS_AUTOMATED_MODEL_NAME" envDefault:"behavior-classifier"`
}

// DefaultMLOpsOrchestratorConfig 默认配置
func DefaultMLOpsOrchestratorConfig() MLOpsOrchestratorConfig {
	return MLOpsOrchestratorConfig{
		CheckInterval:         1 * time.Hour,
		MinNewFeedbackCount:   500,
		FeedbackLookbackHours: 24,
		MaxFPRate:             0.15,
		MaxPSI:                0.25,
		MinRetrainInterval:    12 * time.Hour,
		MaxConcurrentTrains:   1,
		ArgoNamespace:         "traffic-analysis",
		ArgoServerURL:         "http://argo-server.argo.svc:2746",
		WorkflowTemplate:      "mlops-training-template",
		AutomatedTenantID:     "default",
		AutomatedModelName:    "behavior-classifier",
	}
}

// =============================================================================
// RetrainTrigger — 触发原因枚举
// =============================================================================

// RetrainTrigger 重训触发原因
type RetrainTrigger string

const (
	TriggerManual     RetrainTrigger = "manual"      // 手动触发
	TriggerScheduled  RetrainTrigger = "scheduled"   // 定时触发
	TriggerFeedback   RetrainTrigger = "feedback"    // 反馈数据充足
	TriggerFPRate     RetrainTrigger = "fp_rate"     // FP 率过高
	TriggerDrift      RetrainTrigger = "drift"       // 数据漂移
	TriggerDataVolume RetrainTrigger = "data_volume" // 数据量达标
)

// =============================================================================
// MLOpsOrchestrator
// =============================================================================

// MLOpsOrchestrator MLOps 自编排引擎
type MLOpsOrchestrator struct {
	chDB         *sql.DB      // ClickHouse 连接
	pgDB         *sql.DB      // PostgreSQL 连接
	httpClient   *http.Client // Argo REST API 客户端
	auditService *ModelService
	config       MLOpsOrchestratorConfig
	logger       *zap.Logger

	// 运行时状态
	mu               sync.Mutex
	lastRetrainTime  time.Time
	runningWorkflows int
	stopCh           chan struct{}
	stopped          bool
}

// automaticSchedulerAdvisoryLockID is a stable cluster-wide PostgreSQL
// session-lock key (ASCII "MLOPSAUT"). It prevents overlapping old/new Pods
// during a RollingUpdate from evaluating the same snapshot and submitting two
// different automatic workflows.
const automaticSchedulerAdvisoryLockID int64 = 0x4d4c4f5053415554

// SetAuditService installs the durable audit gate used by automatic workflow
// submissions. Production must set it before Start; missing audit is fail-closed.
func (o *MLOpsOrchestrator) SetAuditService(modelService *ModelService) {
	o.auditService = modelService
}

// NewMLOpsOrchestrator 创建自编排引擎
func NewMLOpsOrchestrator(chDB, pgDB *sql.DB, config MLOpsOrchestratorConfig, logger *zap.Logger) *MLOpsOrchestrator {
	return &MLOpsOrchestrator{
		chDB:       chDB,
		pgDB:       pgDB,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		config:     config,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// =============================================================================
// 主循环
// =============================================================================

// Start 启动自编排引擎
func (o *MLOpsOrchestrator) Start(ctx context.Context) {
	o.logger.Info("MLOps orchestrator started",
		zap.Duration("check_interval", o.config.CheckInterval),
		zap.Int("min_feedback", o.config.MinNewFeedbackCount),
		zap.Float64("max_fp_rate", o.config.MaxFPRate))

	ticker := time.NewTicker(o.config.CheckInterval)
	defer ticker.Stop()

	// 启动时立即检查一次
	go o.checkAndMaybeTrigger(ctx)

	for {
		select {
		case <-ticker.C:
			go o.checkAndMaybeTrigger(ctx)
		case <-o.stopCh:
			o.logger.Info("MLOps orchestrator stopped")
			return
		case <-ctx.Done():
			o.logger.Info("MLOps orchestrator context cancelled")
			return
		}
	}
}

// Stop 停止自编排引擎
func (o *MLOpsOrchestrator) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.stopped {
		o.stopped = true
		close(o.stopCh)
	}
}

// =============================================================================
// 决策引擎
// =============================================================================

// RetrainDecision 重训决策结果
type RetrainDecision struct {
	ShouldRetrain bool                   `json:"should_retrain"`
	Trigger       RetrainTrigger         `json:"trigger"`
	Reason        string                 `json:"reason"`
	Metrics       map[string]interface{} `json:"metrics"`
	TenantID      string                 `json:"tenant_id"`
	ModelID       string                 `json:"model_id"`
	FeatureSetID  string                 `json:"feature_set_id"`
}

type automatedMLOpsScope struct {
	TenantID     string
	ModelID      string
	FeatureSetID string
}

// checkAndMaybeTrigger 检查所有条件并决定是否触发重训
func (o *MLOpsOrchestrator) checkAndMaybeTrigger(ctx context.Context) {
	ctx, span := otel.StartSpan(ctx, "MLOpsOrchestrator.checkAndMaybeTrigger")
	defer span.End()
	// Audit reconciliation is independent of the retrain cooldown. A completed
	// Argo mutation must not leave its durable intent pending for 12 hours.
	o.reconcilePendingAutomatedAudits(ctx)
	releaseSchedulerLock, acquired, err := o.acquireAutomaticSchedulerLock(ctx)
	if err != nil {
		o.logger.Warn("Cannot acquire automatic MLOps scheduler lock; skipping mutation", zap.Error(err))
		return
	}
	if !acquired {
		o.logger.Debug("Another MLOps orchestrator owns the automatic scheduler lock; skipping")
		return
	}
	defer releaseSchedulerLock()

	scope, err := o.resolveAutomatedScope(ctx)
	if err != nil {
		o.logger.Warn("Automatic MLOps scope is unavailable; skipping", zap.Error(err))
		return
	}
	running, err := o.reconcileRunningWorkflows(ctx, scope.TenantID)
	if err != nil {
		o.logger.Warn("Cannot reconcile running MLOps workflows; skipping automatic mutation", zap.Error(err))
		return
	}
	o.mu.Lock()
	lastRetrainTime := o.lastRetrainTime
	o.mu.Unlock()
	// The Argo reconciliation above restores the durable cooldown baseline after
	// a process restart. Never evaluate or mutate before that state is known.
	if !lastRetrainTime.IsZero() && time.Since(lastRetrainTime) < o.config.MinRetrainInterval {
		o.logger.Debug("Too soon since last retrain, skipping",
			zap.Duration("elapsed", time.Since(lastRetrainTime)))
		return
	}
	if running >= o.config.MaxConcurrentTrains {
		o.logger.Debug("Max concurrent trainings reached, skipping", zap.Int("running", running))
		return
	}

	decision := o.evaluateConditionsForScope(ctx, scope)
	if !decision.ShouldRetrain {
		o.logger.Debug("Retrain conditions not met",
			zap.Any("decision", decision))
		return
	}

	o.logger.Info("Retrain conditions met — submitting workflow",
		zap.String("trigger", string(decision.Trigger)),
		zap.String("reason", decision.Reason),
		zap.Any("metrics", decision.Metrics))

	if err := o.submitAutomatedRetrain(ctx, decision); err != nil {
		o.logger.Error("Failed to submit Argo workflow", zap.Error(err))
		return
	}

	o.mu.Lock()
	o.lastRetrainTime = time.Now()
	o.runningWorkflows++
	o.mu.Unlock()
}

func (o *MLOpsOrchestrator) acquireAutomaticSchedulerLock(ctx context.Context) (func(), bool, error) {
	if o.pgDB == nil {
		return func() {}, false, errors.New(errors.ErrCodeDatabaseError, "automatic MLOps PostgreSQL lock context is required")
	}
	conn, err := o.pgDB.Conn(ctx)
	if err != nil {
		return func() {}, false, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to reserve automatic MLOps scheduler connection")
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, automaticSchedulerAdvisoryLockID).Scan(&acquired); err != nil {
		_ = conn.Close()
		return func() {}, false, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to acquire automatic MLOps scheduler lock")
	}
	if !acquired {
		_ = conn.Close()
		return func() {}, false, nil
	}
	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRowContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, automaticSchedulerAdvisoryLockID).Scan(&unlocked); err != nil || !unlocked {
			o.logger.Error("Failed to release automatic MLOps scheduler lock", zap.Error(err), zap.Bool("unlocked", unlocked))
		}
		if err := conn.Close(); err != nil {
			o.logger.Warn("Failed to close automatic MLOps scheduler lock connection", zap.Error(err))
		}
	}
	return release, true, nil
}

// evaluateConditions 评估所有触发条件
func (o *MLOpsOrchestrator) evaluateConditions(ctx context.Context) *RetrainDecision {
	scope, err := o.resolveAutomatedScope(ctx)
	if err != nil {
		return &RetrainDecision{ShouldRetrain: false, Metrics: map[string]interface{}{"scope_error": err.Error(), "checked_at": time.Now().UTC().Format(time.RFC3339)}}
	}
	return o.evaluateConditionsForScope(ctx, scope)
}

func (o *MLOpsOrchestrator) evaluateConditionsForScope(ctx context.Context, scope *automatedMLOpsScope) *RetrainDecision {
	// 按优先级评估：反馈 > FP率 > 漂移

	// 1. 检查反馈数据积累
	if decision := o.checkFeedbackAccumulation(ctx, scope); decision != nil {
		return decision
	}

	// 2. 检查 FP 率
	if decision := o.checkFPRate(ctx, scope); decision != nil {
		return decision
	}

	// 3. 检查数据漂移
	if decision := o.checkDataDrift(ctx, scope); decision != nil {
		return decision
	}

	// 条件不满足
	return &RetrainDecision{
		ShouldRetrain: false,
		Metrics:       map[string]interface{}{"checked_at": time.Now().UTC().Format(time.RFC3339)},
	}
}

func (o *MLOpsOrchestrator) resolveAutomatedScope(ctx context.Context) (*automatedMLOpsScope, error) {
	tenantID := strings.TrimSpace(o.config.AutomatedTenantID)
	modelName := strings.TrimSpace(o.config.AutomatedModelName)
	if tenantID == "" || modelName == "" || o.pgDB == nil {
		return nil, errors.New(errors.ErrCodeDatabaseError, "automatic MLOps tenant, model and PostgreSQL context are required")
	}
	const query = `
		SELECT m.model_id::text,
		       COALESCE((
		         SELECT mv.feature_set_id
		         FROM model_versions mv
		         WHERE mv.model_id = m.model_id AND mv.tenant_id = m.tenant_id
		         ORDER BY (mv.status = 'active') DESC, mv.updated_at DESC
		         LIMIT 1
		       ), 'v1')
		FROM models m
		WHERE m.tenant_id = $1 AND m.name = $2
		LIMIT 1
	`
	scope := &automatedMLOpsScope{TenantID: tenantID}
	if err := o.pgDB.QueryRowContext(ctx, query, tenantID, modelName).Scan(&scope.ModelID, &scope.FeatureSetID); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to resolve automatic MLOps model scope")
	}
	if strings.TrimSpace(scope.ModelID) == "" || strings.TrimSpace(scope.FeatureSetID) == "" {
		return nil, errors.New(errors.ErrCodeDatabaseError, "automatic MLOps model scope is incomplete")
	}
	return scope, nil
}

// =============================================================================
// 条件检查
// =============================================================================

// checkFeedbackAccumulation 检查反馈数据积累
func (o *MLOpsOrchestrator) checkFeedbackAccumulation(ctx context.Context, scope *automatedMLOpsScope) *RetrainDecision {
	if o.chDB == nil {
		return nil
	}

	query := `
		SELECT count() AS cnt
		FROM traffic.alert_feedback
		WHERE created_at >= now() - INTERVAL ? HOUR
		  AND label IN ('TP', 'FP')
		  AND tenant_id = ?
	`

	var count uint64
	if err := o.chDB.QueryRowContext(ctx, query, o.config.FeedbackLookbackHours, scope.TenantID).Scan(&count); err != nil {
		o.logger.Warn("Failed to query feedback count", zap.Error(err))
		return nil
	}

	o.logger.Debug("Feedback accumulation check",
		zap.Uint64("new_feedback", count),
		zap.Int("threshold", o.config.MinNewFeedbackCount))

	if int(count) >= o.config.MinNewFeedbackCount {
		return &RetrainDecision{
			ShouldRetrain: true,
			Trigger:       TriggerFeedback,
			Reason: fmt.Sprintf("New feedback accumulated: %d labels in last %dh (threshold: %d)",
				count, o.config.FeedbackLookbackHours, o.config.MinNewFeedbackCount),
			Metrics: map[string]interface{}{
				"new_feedback_count": count,
				"lookback_hours":     o.config.FeedbackLookbackHours,
			},
			TenantID: scope.TenantID, ModelID: scope.ModelID, FeatureSetID: scope.FeatureSetID,
		}
	}
	return nil
}

// checkFPRate 检查 FP 率是否过高
func (o *MLOpsOrchestrator) checkFPRate(ctx context.Context, scope *automatedMLOpsScope) *RetrainDecision {
	if o.chDB == nil {
		return nil
	}

	query := `
		SELECT
			countIf(label = 'FP') AS fp_count,
			count() AS total,
			if(total > 0, fp_count / total, 0) AS fp_rate
		FROM traffic.alert_feedback
		WHERE created_at >= now() - INTERVAL ? HOUR
		  AND label IN ('TP', 'FP')
		  AND tenant_id = ?
	`

	var fpCount, total uint64
	var fpRate float64
	if err := o.chDB.QueryRowContext(ctx, query, o.config.FeedbackLookbackHours, scope.TenantID).Scan(&fpCount, &total, &fpRate); err != nil {
		o.logger.Warn("Failed to query FP rate", zap.Error(err))
		return nil
	}

	o.logger.Debug("FP rate check",
		zap.Uint64("fp_count", fpCount),
		zap.Uint64("total", total),
		zap.Float64("fp_rate", fpRate),
		zap.Float64("threshold", o.config.MaxFPRate))

	// 需要足够样本量才有意义
	if total >= 100 && fpRate > o.config.MaxFPRate {
		return &RetrainDecision{
			ShouldRetrain: true,
			Trigger:       TriggerFPRate,
			Reason: fmt.Sprintf("FP rate %.2f%% exceeds threshold %.2f%% (FP: %d / Total: %d)",
				fpRate*100, o.config.MaxFPRate*100, fpCount, total),
			Metrics: map[string]interface{}{
				"fp_count":  fpCount,
				"total":     total,
				"fp_rate":   fpRate,
				"threshold": o.config.MaxFPRate,
			},
			TenantID: scope.TenantID, ModelID: scope.ModelID, FeatureSetID: scope.FeatureSetID,
		}
	}
	return nil
}

// checkDataDrift 检查数据漂移（基于特征分布 PSI）
func (o *MLOpsOrchestrator) checkDataDrift(ctx context.Context, scope *automatedMLOpsScope) *RetrainDecision {
	if o.chDB == nil {
		return nil
	}

	// 从 ClickHouse 获取最近的特征统计
	query := `
		SELECT
			avg(pps) AS avg_pps,
			avg(bps) AS avg_bps,
			avg(pktlen_mean) AS avg_pktlen,
			avg(iat_mean_ms) AS avg_iat,
			quantile(0.5)(pps) AS p50_pps,
			quantile(0.5)(bps) AS p50_bps,
			count() AS sample_count
		FROM traffic.feature_stat
		WHERE ts >= now() - INTERVAL 24 HOUR
		  AND tenant_id = ?
	`

	type driftStats struct {
		AvgPPS, AvgBPS, AvgPktLen, AvgIAT float64
		P50PPS, P50BPS                    float64
		SampleCount                       uint64
	}
	var recent driftStats

	err := o.chDB.QueryRowContext(ctx, query, scope.TenantID).Scan(
		&recent.AvgPPS, &recent.AvgBPS, &recent.AvgPktLen, &recent.AvgIAT,
		&recent.P50PPS, &recent.P50BPS, &recent.SampleCount,
	)
	if err != nil {
		o.logger.Debug("Failed to query feature stats for drift check", zap.Error(err))
		return nil
	}

	// 样本量不足，跳过
	if recent.SampleCount < 1000 {
		return nil
	}

	// 与基线比较（从活跃模型版本获取基线）
	baselineQuery := `
		SELECT metrics
		FROM model_versions
		WHERE status = 'active' AND tenant_id = $1 AND model_id = $2::uuid
		ORDER BY created_at DESC
		LIMIT 1
	`
	var metricsJSON []byte
	if err := o.pgDB.QueryRowContext(ctx, baselineQuery, scope.TenantID, scope.ModelID).Scan(&metricsJSON); err != nil {
		o.logger.Debug("No active model version for baseline comparison", zap.Error(err))
		return nil
	}

	o.logger.Debug("Drift check completed",
		zap.Uint64("recent_samples", recent.SampleCount),
		zap.Float64("avg_pps", recent.AvgPPS))
	return nil // 漂移检测需要更多基线数据，当前跳过
}

// =============================================================================
// Argo Workflow 提交
// =============================================================================

func newAutomatedMLOpsWorkflowName(trigger RetrainTrigger) string {
	triggerLabel := strings.ReplaceAll(string(trigger), "_", "-")
	return fmt.Sprintf("mlops-%s-%s", triggerLabel, strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
}

func (o *MLOpsOrchestrator) reconcileRunningWorkflows(ctx context.Context, tenantID string) (int, error) {
	workflows, err := o.ListWorkflows(ctx)
	if err != nil {
		return 0, err
	}
	running := 0
	latestCreatedAt := time.Time{}
	for _, workflow := range workflows {
		if strings.TrimSpace(workflow.Parameters["tenant-id"]) != tenantID {
			continue
		}
		if createdAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(workflow.CreatedAt)); parseErr == nil && createdAt.After(latestCreatedAt) {
			latestCreatedAt = createdAt
		}
		if workflow.Phase == "Pending" || workflow.Phase == "Running" {
			running++
		}
	}
	o.mu.Lock()
	o.runningWorkflows = running
	if latestCreatedAt.After(o.lastRetrainTime) {
		o.lastRetrainTime = latestCreatedAt
	}
	o.mu.Unlock()
	return running, nil
}

func (o *MLOpsOrchestrator) reconcilePendingAutomatedAudits(ctx context.Context) {
	if o.auditService == nil {
		return
	}
	if count, err := o.auditService.ReconcileCompletedLegacyAutomatedMLOpsAuditIntents(ctx); err != nil {
		o.logger.Warn("Failed to backfill linked automatic MLOps audit intents", zap.Error(err))
	} else if count > 0 {
		o.logger.Info("Backfilled linked automatic MLOps audit intents", zap.Int64("count", count))
	}
	pending, err := o.auditService.ListPendingAutomatedMLOpsAuditIntents(ctx, 50)
	if err != nil {
		o.logger.Warn("Failed to list pending automatic MLOps audit intents", zap.Error(err))
		return
	}
	for _, intent := range pending {
		workflow, getErr := o.GetWorkflow(ctx, intent.WorkflowName)
		if getErr != nil || strings.TrimSpace(workflow.Parameters["tenant-id"]) != intent.TenantID {
			continue
		}
		opCtx := &OperationContext{TenantID: intent.TenantID, Username: "mlops-auto-audit-reconciler", Authenticated: true}
		detail := map[string]interface{}{
			"trigger": intent.Trigger, "reason": intent.Reason, "tenant_id": intent.TenantID,
			"model_id": intent.ModelID, "feature_set_id": intent.FeatureSetID, "intent_event_id": intent.EventID,
		}
		if err := o.auditService.RecordAutomatedMLOpsAuditCompletion(ctx, opCtx, "MLOPS_AUTOMATED_RETRAIN_SUBMITTED", intent.WorkflowName, intent.EventID, detail); err != nil {
			o.logger.Warn("Automatic MLOps audit reconciliation remains pending", zap.Error(err), zap.String("intent_event_id", intent.EventID))
		}
	}
}

func (o *MLOpsOrchestrator) submitAutomatedRetrain(ctx context.Context, decision *RetrainDecision) error {
	if o.auditService == nil {
		return errors.New(errors.ErrCodeDatabaseError, "automatic MLOps audit service is required")
	}
	if decision == nil || strings.TrimSpace(decision.TenantID) == "" || strings.TrimSpace(decision.ModelID) == "" || strings.TrimSpace(decision.FeatureSetID) == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "automatic MLOps tenant, model and feature-set identities are required")
	}
	workflowName := newAutomatedMLOpsWorkflowName(decision.Trigger)
	opCtx := &OperationContext{TenantID: decision.TenantID, Username: "mlops-auto-orchestrator", Authenticated: true}
	intentEventID, err := o.auditService.RecordMLOpsWorkflowAuditIntent(ctx, opCtx, "MLOPS_AUTOMATED_RETRAIN_SUBMIT_REQUESTED", workflowName, map[string]interface{}{
		"trigger": decision.Trigger, "reason": decision.Reason, "tenant_id": opCtx.TenantID,
		"model_id": decision.ModelID, "feature_set_id": decision.FeatureSetID,
		"expected_completion_action": "MLOPS_AUTOMATED_RETRAIN_SUBMITTED", "reconciliation_state": "pending",
	})
	if err != nil {
		return err
	}
	if err := o.submitArgoWorkflow(ctx, decision, workflowName); err != nil {
		_ = o.auditService.RecordMLOpsWorkflowAudit(ctx, opCtx, "MLOPS_AUTOMATED_RETRAIN_SUBMIT", workflowName, map[string]interface{}{"intent_event_id": intentEventID}, err)
		return err
	}
	if err := o.auditService.RecordAutomatedMLOpsAuditCompletion(ctx, opCtx, "MLOPS_AUTOMATED_RETRAIN_SUBMITTED", workflowName, intentEventID, map[string]interface{}{
		"trigger": decision.Trigger, "reason": decision.Reason, "tenant_id": opCtx.TenantID,
		"model_id": decision.ModelID, "feature_set_id": decision.FeatureSetID, "intent_event_id": intentEventID,
	}); err != nil {
		// The requested event is durable and the exact Argo identity is known;
		// log for reconciliation without treating a successful submit as retryable.
		o.logger.Error("Automated MLOps completion audit failed after durable intent", zap.Error(err), zap.String("workflow", workflowName), zap.String("intent_event_id", intentEventID))
		go func(parent context.Context) {
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-parent.Done():
				return
			case <-timer.C:
				o.reconcilePendingAutomatedAudits(parent)
			}
		}(ctx)
	}
	return nil
}

// submitArgoWorkflow 通过 Argo REST API 提交 Workflow (K8s 兼容)
func (o *MLOpsOrchestrator) submitArgoWorkflow(ctx context.Context, decision *RetrainDecision, workflowName string) error {
	ctx, span := otel.StartSpan(ctx, "MLOpsOrchestrator.submitArgoWorkflow")
	defer span.End()

	triggerLabel := strings.ReplaceAll(string(decision.Trigger), "_", "-")
	// 构建 Argo Workflow 提交请求
	submitBody := map[string]interface{}{
		"namespace":    o.config.ArgoNamespace,
		"resourceKind": "WorkflowTemplate",
		"resourceName": o.config.WorkflowTemplate,
		"submitOptions": map[string]interface{}{
			"name": workflowName,
			"parameters": []string{
				fmt.Sprintf("trigger=%s", decision.Trigger),
				fmt.Sprintf("trigger-reason=%s", decision.Reason),
				fmt.Sprintf("tenant-id=%s", decision.TenantID),
				fmt.Sprintf("model-id=%s", decision.ModelID),
				fmt.Sprintf("feature-set-id=%s", decision.FeatureSetID),
			},
			"labels": fmt.Sprintf("mlops-trigger=%s", triggerLabel),
		},
	}

	bodyBytes, _ := json.Marshal(submitBody)

	// Argo REST API: POST /api/v1/workflows/{namespace}/submit
	apiURL := fmt.Sprintf("%s/api/v1/workflows/%s/submit",
		o.config.ArgoServerURL, o.config.ArgoNamespace)

	o.logger.Info("Submitting Argo workflow via REST API",
		zap.String("url", apiURL),
		zap.String("template", o.config.WorkflowTemplate),
		zap.String("workflow_name", workflowName))

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to create argo request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeServiceUnavailable,
			fmt.Sprintf("argo API call failed (server: %s)", o.config.ArgoServerURL))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return errors.Newf(errors.ErrCodeServiceUnavailable,
			"argo API returned %d: %s", resp.StatusCode, string(respBody))
	}

	// The pre-audited identity is authoritative and Argo must confirm it.
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return errors.Wrap(err, errors.ErrCodeServiceUnavailable, "invalid automated Argo submit response")
	}
	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		return errors.New(errors.ErrCodeServiceUnavailable, "automated Argo submit response did not include workflow metadata")
	}
	returnedName, ok := metadata["name"].(string)
	returnedName = strings.TrimSpace(returnedName)
	if !ok || returnedName == "" {
		return errors.New(errors.ErrCodeServiceUnavailable, "automated Argo submit response did not include the workflow identity")
	}
	if returnedName != workflowName {
		return errors.Newf(errors.ErrCodeServiceUnavailable, "automated Argo submit identity mismatch: expected %s, got %s", workflowName, returnedName)
	}

	o.logger.Info("Argo workflow submitted via REST API",
		zap.String("workflow_name", workflowName),
		zap.String("trigger", string(decision.Trigger)),
		zap.Int("status_code", resp.StatusCode))

	return nil
}

// =============================================================================
// 手动触发 API
// =============================================================================

// ManualRetrainRequest 手动触发重训请求
type ManualRetrainRequest struct {
	ModelType    string                 `json:"model_type"`
	LookbackDays int                    `json:"lookback_days"`
	TenantID     string                 `json:"tenant_id"`
	FeatureSetID string                 `json:"feature_set_id"`
	Params       map[string]interface{} `json:"params,omitempty"`
	WorkflowName string                 `json:"-"`
}

// NewManualMLOpsWorkflowName reserves the exact identity that is written to
// the required audit intent before Argo receives any mutating request.
func NewManualMLOpsWorkflowName() string {
	return "mlops-manual-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

// MLOpsWorkflow is the tenant-safe projection of an Argo workflow exposed to
// the product UI. Raw pod specs, secrets and artifact credentials never leave
// the control plane.
type MLOpsWorkflow struct {
	Name             string            `json:"name"`
	Namespace        string            `json:"namespace"`
	Phase            string            `json:"phase"`
	Progress         string            `json:"progress"`
	Message          string            `json:"message,omitempty"`
	StartedAt        string            `json:"started_at,omitempty"`
	FinishedAt       string            `json:"finished_at,omitempty"`
	CreatedAt        string            `json:"created_at,omitempty"`
	WorkflowTemplate string            `json:"workflow_template,omitempty"`
	Parameters       map[string]string `json:"parameters,omitempty"`
	CanStop          bool              `json:"can_stop"`
	CanRetry         bool              `json:"can_retry"`
}

type argoWorkflowEnvelope struct {
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		CreationTimestamp string            `json:"creationTimestamp"`
		Labels            map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		WorkflowTemplateRef struct {
			Name string `json:"name"`
		} `json:"workflowTemplateRef"`
		Arguments struct {
			Parameters []struct {
				Name  string      `json:"name"`
				Value interface{} `json:"value"`
			} `json:"parameters"`
		} `json:"arguments"`
	} `json:"spec"`
	Status struct {
		Phase      string `json:"phase"`
		Progress   string `json:"progress"`
		Message    string `json:"message"`
		StartedAt  string `json:"startedAt"`
		FinishedAt string `json:"finishedAt"`
	} `json:"status"`
}

func projectArgoWorkflow(item argoWorkflowEnvelope) MLOpsWorkflow {
	parameters := make(map[string]string, len(item.Spec.Arguments.Parameters))
	for _, parameter := range item.Spec.Arguments.Parameters {
		parameters[parameter.Name] = strings.TrimSpace(fmt.Sprint(parameter.Value))
	}
	phase := strings.TrimSpace(item.Status.Phase)
	return MLOpsWorkflow{
		Name: item.Metadata.Name, Namespace: item.Metadata.Namespace,
		Phase: phase, Progress: item.Status.Progress, Message: item.Status.Message,
		StartedAt: item.Status.StartedAt, FinishedAt: item.Status.FinishedAt,
		CreatedAt:        item.Metadata.CreationTimestamp,
		WorkflowTemplate: item.Spec.WorkflowTemplateRef.Name,
		Parameters:       parameters,
		CanStop:          phase == "Pending" || phase == "Running",
		CanRetry:         phase == "Failed" || phase == "Error",
	}
}

// ListWorkflows reads the real Argo workflow CRDs through the Argo API.
func (o *MLOpsOrchestrator) ListWorkflows(ctx context.Context) ([]MLOpsWorkflow, error) {
	apiURL := fmt.Sprintf("%s/api/v1/workflows/%s", o.config.ArgoServerURL, url.PathEscape(o.config.ArgoNamespace))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to create argo workflow list request")
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to list argo workflows")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.Newf(errors.ErrCodeServiceUnavailable, "argo workflow list returned %d: %s", resp.StatusCode, string(body))
	}
	var envelope struct {
		Items []argoWorkflowEnvelope `json:"items"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "invalid argo workflow list response")
	}
	workflows := make([]MLOpsWorkflow, 0, len(envelope.Items))
	for _, item := range envelope.Items {
		workflow := projectArgoWorkflow(item)
		if strings.HasPrefix(workflow.Name, "mlops-") || workflow.WorkflowTemplate == o.config.WorkflowTemplate {
			workflows = append(workflows, workflow)
		}
	}
	sort.Slice(workflows, func(i, j int) bool { return workflows[i].CreatedAt > workflows[j].CreatedAt })
	return workflows, nil
}

// GetWorkflow returns one real Argo workflow after strict name validation.
func (o *MLOpsOrchestrator) GetWorkflow(ctx context.Context, name string) (*MLOpsWorkflow, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 253 || !strings.HasPrefix(name, "mlops-") {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid MLOps workflow name")
	}
	apiURL := fmt.Sprintf("%s/api/v1/workflows/%s/%s", o.config.ArgoServerURL, url.PathEscape(o.config.ArgoNamespace), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to create argo workflow request")
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to read argo workflow")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.Newf(errors.ErrCodeResourceNotFound, "MLOps workflow not found: %s", name)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.Newf(errors.ErrCodeServiceUnavailable, "argo workflow read returned %d: %s", resp.StatusCode, string(body))
	}
	var item argoWorkflowEnvelope
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "invalid argo workflow response")
	}
	workflow := projectArgoWorkflow(item)
	return &workflow, nil
}

func (o *MLOpsOrchestrator) mutateWorkflow(ctx context.Context, name, action string) (*MLOpsWorkflow, error) {
	workflow, err := o.GetWorkflow(ctx, name)
	if err != nil {
		return nil, err
	}
	if action == "stop" && !workflow.CanStop {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition, "workflow %s in phase %s cannot be stopped", name, workflow.Phase)
	}
	if action == "retry" && !workflow.CanRetry {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition, "workflow %s in phase %s cannot be retried", name, workflow.Phase)
	}
	argoAction := action
	requestBody := []byte("{}")
	if action == "retry" {
		// A stopped Argo workflow contains Skipped nodes. The retry API only
		// resets Failed/Error nodes, so downstream steps can still reference
		// artifacts from a skipped producer. Resubmit creates a fresh workflow
		// from the same arguments and current WorkflowTemplate, which is the
		// safe operator meaning of retry for a stopped MLOps pipeline.
		argoAction = "resubmit"
		requestBody = []byte(`{"memoized":false}`)
	}
	apiURL := fmt.Sprintf("%s/api/v1/workflows/%s/%s/%s", o.config.ArgoServerURL, url.PathEscape(o.config.ArgoNamespace), url.PathEscape(name), argoAction)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to create argo workflow action request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeServiceUnavailable, "argo workflow action failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.Newf(errors.ErrCodeServiceUnavailable, "argo workflow %s returned %d: %s", argoAction, resp.StatusCode, string(body))
	}
	var item argoWorkflowEnvelope
	if err := json.Unmarshal(body, &item); err == nil && item.Metadata.Name != "" {
		projected := projectArgoWorkflow(item)
		return &projected, nil
	}
	if action == "retry" {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "Argo resubmit response did not include the new workflow identity")
	}
	return o.GetWorkflow(ctx, name)
}

func (o *MLOpsOrchestrator) StopWorkflow(ctx context.Context, name string) (*MLOpsWorkflow, error) {
	return o.mutateWorkflow(ctx, name, "stop")
}

func (o *MLOpsOrchestrator) RetryWorkflow(ctx context.Context, name string) (*MLOpsWorkflow, error) {
	return o.mutateWorkflow(ctx, name, "retry")
}

// TriggerManualRetrain 通过 Argo REST API 手动触发模型重训 (K8s 兼容)
func (o *MLOpsOrchestrator) TriggerManualRetrain(ctx context.Context, req *ManualRetrainRequest) (string, error) {
	ctx, span := otel.StartSpan(ctx, "MLOpsOrchestrator.TriggerManualRetrain")
	defer span.End()

	// 设置默认值
	if req.ModelType == "" {
		req.ModelType = "xgboost"
	}
	if req.LookbackDays <= 0 {
		req.LookbackDays = 7
	}
	if req.TenantID == "" {
		req.TenantID = "campus-net"
	}
	if req.FeatureSetID == "" {
		req.FeatureSetID = "v1"
	}
	if err := ValidateManualRetrainParameters(req.Params); err != nil {
		return "", err
	}
	if strings.TrimSpace(req.WorkflowName) == "" {
		req.WorkflowName = NewManualMLOpsWorkflowName()
	}

	workflowName := req.WorkflowName

	// 构建 Argo REST API 请求
	parameters := []string{
		fmt.Sprintf("model-type=%s", req.ModelType),
		fmt.Sprintf("lookback-days=%d", req.LookbackDays),
		fmt.Sprintf("tenant-id=%s", req.TenantID),
		fmt.Sprintf("feature-set-id=%s", req.FeatureSetID),
		"trigger=manual",
	}
	parameters = append(parameters, allowedManualRetrainParameters(req.Params)...)

	submitBody := map[string]interface{}{
		"namespace":    o.config.ArgoNamespace,
		"resourceKind": "WorkflowTemplate",
		"resourceName": o.config.WorkflowTemplate,
		"submitOptions": map[string]interface{}{
			"name":       workflowName,
			"parameters": parameters,
			"labels":     "mlops-trigger=manual",
		},
	}

	bodyBytes, _ := json.Marshal(submitBody)
	apiURL := fmt.Sprintf("%s/api/v1/workflows/%s/submit",
		o.config.ArgoServerURL, o.config.ArgoNamespace)

	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeServiceUnavailable, "failed to create argo request")
	}
	reqHTTP.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(reqHTTP)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeServiceUnavailable,
			fmt.Sprintf("argo API call failed (server: %s)", o.config.ArgoServerURL))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", errors.Newf(errors.ErrCodeServiceUnavailable,
			"argo API returned %d: %s", resp.StatusCode, string(respBody))
	}

	// The authoritative identity must come back from Argo and match the exact
	// pre-audited name. A local fallback would create an untraceable success.
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", errors.Wrap(err, errors.ErrCodeServiceUnavailable, "invalid Argo submit response")
	}
	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		return "", errors.New(errors.ErrCodeServiceUnavailable, "Argo submit response did not include workflow metadata")
	}
	returnedName, ok := metadata["name"].(string)
	returnedName = strings.TrimSpace(returnedName)
	if !ok || returnedName == "" {
		return "", errors.New(errors.ErrCodeServiceUnavailable, "Argo submit response did not include the workflow identity")
	}
	if returnedName != workflowName {
		return "", errors.Newf(errors.ErrCodeServiceUnavailable, "Argo submit identity mismatch: expected %s, got %s", workflowName, returnedName)
	}
	workflowName = returnedName

	o.logger.Info("Manual retrain workflow submitted via REST API",
		zap.String("workflow_name", workflowName),
		zap.String("model_type", req.ModelType))

	o.mu.Lock()
	o.lastRetrainTime = time.Now()
	o.mu.Unlock()

	return workflowName, nil
}

var allowedManualRetrainParameterKeys = map[string]struct{}{
	"model-id":       {},
	"trigger-reason": {},
}

// ValidateManualRetrainParameters rejects unknown or malformed parameters.
// Silently dropping a caller-supplied security-sensitive key would make a 202
// response ambiguous, so the product API fails explicitly before audit or Argo.
func ValidateManualRetrainParameters(params map[string]interface{}) error {
	invalidSet := make(map[string]struct{})
	seenNormalized := make(map[string]struct{})
	for key, value := range params {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey != key {
			invalidSet[key] = struct{}{}
		}
		if _, duplicated := seenNormalized[normalizedKey]; duplicated {
			invalidSet[normalizedKey+" (duplicate)"] = struct{}{}
		}
		seenNormalized[normalizedKey] = struct{}{}
		if _, ok := allowedManualRetrainParameterKeys[normalizedKey]; !ok {
			invalidSet[key] = struct{}{}
			continue
		}
		valueString := strings.TrimSpace(fmt.Sprint(value))
		if valueString == "" || strings.ContainsAny(valueString, "\r\n") {
			invalidSet[key] = struct{}{}
		}
	}
	if len(invalidSet) == 0 {
		return nil
	}
	invalid := make([]string, 0, len(invalidSet))
	for key := range invalidSet {
		invalid = append(invalid, key)
	}
	sort.Strings(invalid)
	return errors.Newf(errors.ErrCodeInvalidParameter, "unsupported or invalid MLOps parameters: %s", strings.Join(invalid, ", "))
}

func allowedManualRetrainParameters(params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}

	result := make([]string, 0, len(params))
	for key, value := range params {
		if _, ok := allowedManualRetrainParameterKeys[key]; !ok {
			continue
		}

		valueString := strings.TrimSpace(fmt.Sprint(value))
		if valueString == "" || strings.ContainsAny(valueString, "\r\n") {
			continue
		}
		result = append(result, fmt.Sprintf("%s=%s", key, valueString))
	}
	return result
}

// GetStatus 获取编排器状态
func (o *MLOpsOrchestrator) GetStatus() map[string]interface{} {
	o.mu.Lock()
	defer o.mu.Unlock()

	return map[string]interface{}{
		"last_retrain_time":    o.lastRetrainTime.Format(time.RFC3339),
		"running_workflows":    o.runningWorkflows,
		"stopped":              o.stopped,
		"max_concurrent":       o.config.MaxConcurrentTrains,
		"min_retrain_interval": o.config.MinRetrainInterval.String(),
		"check_interval":       o.config.CheckInterval.String(),
		"min_feedback_count":   o.config.MinNewFeedbackCount,
		"max_fp_rate":          o.config.MaxFPRate,
		"clickhouse_connected": o.chDB != nil,
		"argo_namespace":       o.config.ArgoNamespace,
		"workflow_template":    o.config.WorkflowTemplate,
	}
}

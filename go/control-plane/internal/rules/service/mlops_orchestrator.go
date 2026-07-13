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
	"strings"
	"sync"
	"time"

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
	chDB       *sql.DB      // ClickHouse 连接
	pgDB       *sql.DB      // PostgreSQL 连接
	httpClient *http.Client // Argo REST API 客户端
	config     MLOpsOrchestratorConfig
	logger     *zap.Logger

	// 运行时状态
	mu               sync.Mutex
	lastRetrainTime  time.Time
	runningWorkflows int
	stopCh           chan struct{}
	stopped          bool
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
}

// checkAndMaybeTrigger 检查所有条件并决定是否触发重训
func (o *MLOpsOrchestrator) checkAndMaybeTrigger(ctx context.Context) {
	ctx, span := otel.StartSpan(ctx, "MLOpsOrchestrator.checkAndMaybeTrigger")
	defer span.End()

	o.mu.Lock()
	// 检查重训间隔
	if time.Since(o.lastRetrainTime) < o.config.MinRetrainInterval {
		o.mu.Unlock()
		o.logger.Debug("Too soon since last retrain, skipping",
			zap.Duration("elapsed", time.Since(o.lastRetrainTime)))
		return
	}
	// 检查并发限制
	if o.runningWorkflows >= o.config.MaxConcurrentTrains {
		o.mu.Unlock()
		o.logger.Debug("Max concurrent trainings reached, skipping",
			zap.Int("running", o.runningWorkflows))
		return
	}
	o.mu.Unlock()

	decision := o.evaluateConditions(ctx)
	if !decision.ShouldRetrain {
		o.logger.Debug("Retrain conditions not met",
			zap.Any("decision", decision))
		return
	}

	o.logger.Info("Retrain conditions met — submitting workflow",
		zap.String("trigger", string(decision.Trigger)),
		zap.String("reason", decision.Reason),
		zap.Any("metrics", decision.Metrics))

	if err := o.submitArgoWorkflow(ctx, decision); err != nil {
		o.logger.Error("Failed to submit Argo workflow", zap.Error(err))
		return
	}

	o.mu.Lock()
	o.lastRetrainTime = time.Now()
	o.runningWorkflows++
	o.mu.Unlock()
}

// evaluateConditions 评估所有触发条件
func (o *MLOpsOrchestrator) evaluateConditions(ctx context.Context) *RetrainDecision {
	// 按优先级评估：反馈 > FP率 > 漂移

	// 1. 检查反馈数据积累
	if decision := o.checkFeedbackAccumulation(ctx); decision != nil {
		return decision
	}

	// 2. 检查 FP 率
	if decision := o.checkFPRate(ctx); decision != nil {
		return decision
	}

	// 3. 检查数据漂移
	if decision := o.checkDataDrift(ctx); decision != nil {
		return decision
	}

	// 条件不满足
	return &RetrainDecision{
		ShouldRetrain: false,
		Metrics:       map[string]interface{}{"checked_at": time.Now().UTC().Format(time.RFC3339)},
	}
}

// =============================================================================
// 条件检查
// =============================================================================

// checkFeedbackAccumulation 检查反馈数据积累
func (o *MLOpsOrchestrator) checkFeedbackAccumulation(ctx context.Context) *RetrainDecision {
	if o.chDB == nil {
		return nil
	}

	query := `
		SELECT count() AS cnt
		FROM traffic.alert_feedback
		WHERE created_at >= now() - INTERVAL ? HOUR
		  AND label IN ('TP', 'FP')
	`

	var count uint64
	if err := o.chDB.QueryRowContext(ctx, query, o.config.FeedbackLookbackHours).Scan(&count); err != nil {
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
		}
	}
	return nil
}

// checkFPRate 检查 FP 率是否过高
func (o *MLOpsOrchestrator) checkFPRate(ctx context.Context) *RetrainDecision {
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
	`

	var fpCount, total uint64
	var fpRate float64
	if err := o.chDB.QueryRowContext(ctx, query, o.config.FeedbackLookbackHours).Scan(&fpCount, &total, &fpRate); err != nil {
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
		}
	}
	return nil
}

// checkDataDrift 检查数据漂移（基于特征分布 PSI）
func (o *MLOpsOrchestrator) checkDataDrift(ctx context.Context) *RetrainDecision {
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

	// 使用默认租户
	err := o.chDB.QueryRowContext(ctx, query, "campus-net").Scan(
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
		WHERE status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`
	var metricsJSON []byte
	if err := o.pgDB.QueryRowContext(ctx, baselineQuery).Scan(&metricsJSON); err != nil {
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

// submitArgoWorkflow 通过 Argo REST API 提交 Workflow (K8s 兼容)
func (o *MLOpsOrchestrator) submitArgoWorkflow(ctx context.Context, decision *RetrainDecision) error {
	ctx, span := otel.StartSpan(ctx, "MLOpsOrchestrator.submitArgoWorkflow")
	defer span.End()

	triggerLabel := strings.ReplaceAll(string(decision.Trigger), "_", "-")
	workflowName := fmt.Sprintf("mlops-%s-%s", triggerLabel, time.Now().Format("20060102-150405"))

	// 构建 Argo Workflow 提交请求
	submitBody := map[string]interface{}{
		"namespace":    o.config.ArgoNamespace,
		"resourceKind": "WorkflowTemplate",
		"resourceName": o.config.WorkflowTemplate,
		"submitOptions": map[string]interface{}{
			"generateName": fmt.Sprintf("mlops-%s-", triggerLabel),
			"parameters": []string{
				fmt.Sprintf("trigger=%s", decision.Trigger),
				fmt.Sprintf("trigger-reason=%s", decision.Reason),
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

	// 解析响应获取 workflow metadata
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		if metadata, ok := result["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				workflowName = name
			}
		}
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

	workflowName := fmt.Sprintf("mlops-manual-%s", time.Now().Format("20060102-150405"))

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
			"generateName": "mlops-manual-",
			"parameters":   parameters,
			"labels":       "mlops-trigger=manual",
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

	// 解析响应获取 workflow name
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		if metadata, ok := result["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				workflowName = name
			}
		}
	}

	o.logger.Info("Manual retrain workflow submitted via REST API",
		zap.String("workflow_name", workflowName),
		zap.String("model_type", req.ModelType))

	o.mu.Lock()
	o.lastRetrainTime = time.Now()
	o.mu.Unlock()

	return workflowName, nil
}

func allowedManualRetrainParameters(params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}

	allowed := map[string]struct{}{
		"model-id":           {},
		"min-feedback-count": {},
		"min-feature-count":  {},
		"test-size":          {},
		"min-f1-score":       {},
		"auto-activate":      {},
		"trainer-image":      {},
		"trigger-reason":     {},
	}

	result := make([]string, 0, len(params))
	for key, value := range params {
		normalizedKey := strings.TrimSpace(key)
		if _, ok := allowed[normalizedKey]; !ok {
			continue
		}

		valueString := strings.TrimSpace(fmt.Sprint(value))
		if valueString == "" || strings.ContainsAny(valueString, "\r\n") {
			continue
		}
		result = append(result, fmt.Sprintf("%s=%s", normalizedKey, valueString))
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
		"max_concurrent":       o.config.MaxConcurrentTrains,
		"min_retrain_interval": o.config.MinRetrainInterval.String(),
		"check_interval":       o.config.CheckInterval.String(),
		"min_feedback_count":   o.config.MinNewFeedbackCount,
		"max_fp_rate":          o.config.MaxFPRate,
		"clickhouse_connected": o.chDB != nil,
	}
}

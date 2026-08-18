// Package service §20 页面到 API 唯一映射的服务层补齐:
// 任务定义权威、整 Run 重试(同 task 新 run)、预检(不物化)、报告重试/下载票、
// 报告策略、资源视图、调度触发历史。
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// randHexID 16 字节随机 hex id(下载票秘密、临时句柄等)。
func randHexID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("rand read: %v", err))
	}
	return hex.EncodeToString(b)
}

// TaskDefinitionService 任务定义权威(§76.45.1 同型 CAS;保存不联动计划)。
type TaskDefinitionService struct {
	repo *repository.Repo
}

func NewTaskDefinitionService(repo *repository.Repo) *TaskDefinitionService {
	return &TaskDefinitionService{repo: repo}
}

// CreateTaskDefinition 保存任务定义(DRAFT;幂等经台账;保存绝不创建 Plan/激活)。
func (s *TaskDefinitionService) Create(ctx context.Context, tenantID, name, owner, defaultClass, clientIdempotencyKey string) (defID string, replayed bool, err error) {
	if tenantID == "" || name == "" {
		return "", false, fmt.Errorf("%s: tenant_id and name are required", contract.ErrCodeInvalidTransition)
	}
	if defaultClass != "" && !SchedulingClasses[defaultClass] {
		return "", false, fmt.Errorf("%s: unknown scheduling class %q", contract.ErrCodeInvalidTransition, defaultClass)
	}
	if clientIdempotencyKey == "" {
		clientIdempotencyKey = identityHash("task-definition", tenantID, name)
	}
	requestSHA := identityHash("task-definition", tenantID, name, owner, defaultClass)
	defID, replayed, err = s.repo.CreateTaskDefinitionAtomic(ctx, tenantID, name, owner, defaultClass, clientIdempotencyKey, requestSHA)
	if err == repository.ErrPayloadMismatch {
		return "", false, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
	}
	return defID, replayed, err
}

// Activate DRAFT→ACTIVE(expected revision If-Match)。
func (s *TaskDefinitionService) Activate(ctx context.Context, tenantID, defID string, expectedRevision int64, actor string) (int64, error) {
	if expectedRevision < 0 {
		return 0, fmt.Errorf("%s: expected revision is required", contract.ErrCodeInvalidTransition)
	}
	return s.repo.ActivateTaskDefinitionAtomic(ctx, tenantID, defID, expectedRevision, actor)
}

// Suspend ACTIVE→SUSPENDED(expected revision If-Match)。
func (s *TaskDefinitionService) Suspend(ctx context.Context, tenantID, defID string, expectedRevision int64, actor string) (int64, error) {
	if expectedRevision < 0 {
		return 0, fmt.Errorf("%s: expected revision is required", contract.ErrCodeInvalidTransition)
	}
	return s.repo.SuspendTaskDefinitionAtomic(ctx, tenantID, defID, expectedRevision, actor)
}

// Detail / Plans 只读投影。
func (s *TaskDefinitionService) Detail(ctx context.Context, tenantID, defID string) (*repository.TaskDefinitionDetail, error) {
	return s.repo.GetTaskDefinitionDetail(ctx, tenantID, defID)
}

func (s *TaskDefinitionService) Plans(ctx context.Context, tenantID, defID string) ([]repository.PlanRevisionView, error) {
	return s.repo.ListPlansForDefinition(ctx, tenantID, defID)
}

// SaveReportPolicy 保存报告策略修订(独立冻结,不进执行 plan hash;§8)。
func (s *TaskDefinitionService) SaveReportPolicy(ctx context.Context, tenantID, defID, mode, templateRevision, locale string, retentionDays int64, clientIdempotencyKey string) (policyID string, revision int64, replayed bool, err error) {
	switch mode {
	case "DISABLED", "ON_DEMAND", "AUTO_ASYNC":
	default:
		return "", 0, false, fmt.Errorf("%s: unknown report policy mode %q", contract.ErrCodeInvalidTransition, mode)
	}
	if templateRevision == "" {
		templateRevision = "default-v1"
	}
	if locale == "" {
		locale = "zh-CN"
	}
	revision, err = s.repo.NextHumanReportPolicyRevision(ctx, tenantID, defID)
	if err != nil {
		return "", 0, false, err
	}
	policySHA := sha256Hex(mustJSON(map[string]interface{}{
		"tenant_id": tenantID, "task_definition_id": defID, "mode": mode,
		"template_revision": templateRevision, "locale": locale, "retention_days": retentionDays,
	}))
	if clientIdempotencyKey == "" {
		clientIdempotencyKey = identityHash("report-policy", tenantID, defID, fmt.Sprint(revision))
	}
	requestSHA := identityHash("report-policy", policySHA)
	policyID, frozenRevision, replayed, err := s.repo.SaveHumanReportPolicyAtomic(ctx, tenantID, defID, mode, templateRevision, locale,
		retentionDays, revision, policySHA, clientIdempotencyKey, requestSHA)
	if err == repository.ErrPayloadMismatch {
		return "", 0, false, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
	}
	if err != nil {
		return "", 0, false, err
	}
	return policyID, frozenRevision, replayed, nil
}

// ListReportPolicies 报告策略修订列表。
func (s *TaskDefinitionService) ListReportPolicies(ctx context.Context, tenantID, defID string) ([]repository.HumanReportPolicyView, error) {
	return s.repo.ListHumanReportPolicies(ctx, tenantID, defID)
}

// PreflightService 即时分析预检(只 resolve+compile,不物化;§20 Preflight)。
type PreflightService struct {
	triggers *TriggerService
}

func NewPreflightService(triggers *TriggerService) *PreflightService {
	return &PreflightService{triggers: triggers}
}

// PreflightResult 预检结果(兼容性 + 冻结 spec;不创建 Trigger/Task/Run)。
type PreflightResult struct {
	ExecutionSpecSHA256 string          `json:"execution_spec_sha256"`
	CanonicalSpecJSON   json.RawMessage `json:"canonical_spec"`
	SourceKind          string          `json:"source_kind"`
	Compatible          bool            `json:"compatible"`
}

// Preflight 解析+编译(与 Submit 同一 Resolver/Compiler;不物化)。
func (s *PreflightService) Preflight(ctx context.Context, req SubmitRequest) (*PreflightResult, error) {
	resolver, err := ResolveForPlanSource(s.triggers.resolvers, req.PlanSource)
	if err != nil {
		return nil, err
	}
	// 装配侧模板/目录注入:未显式携带模板时,由加载器从激活计划装配。
	if req.Template == nil && s.triggers.templateLoader != nil {
		tpl, catalog, err := s.triggers.templateLoader(ctx, req.TenantID, req.TaskDefinitionID)
		if err != nil {
			return nil, err
		}
		req.Template = tpl
		req.Catalog = catalog
	}
	intent, err := resolver.Resolve(ctx, ResolveRequest{
		TenantID:         req.TenantID,
		TaskDefinitionID: req.TaskDefinitionID,
		PlanSource:       req.PlanSource,
		CustomOverrides:  req.CustomOverrides,
		Actor:            req.Actor,
		ActorScopes:      req.ActorScopes,
		Catalog:          req.Catalog,
		Template:         req.Template,
		Approved:         req.Approved,
		CustomReleased:   req.CustomReleased,
	})
	if err != nil {
		return nil, err
	}
	// 按需触发允许覆盖源类型(默认取 intent 的源)
	if req.SourceKind != "" {
		intent.SourceKind = req.SourceKind
	}
	if len(req.SourceSpec) > 0 {
		intent.SourceSpec = req.SourceSpec
	}
	compiled, err := s.triggers.compiler.Compile(ctx, *intent)
	if err != nil {
		return nil, err
	}
	return &PreflightResult{
		ExecutionSpecSHA256: compiled.ExecutionSpecSHA256,
		CanonicalSpecJSON:   compiled.CanonicalSpecJSON,
		SourceKind:          intent.SourceKind,
		Compatible:          true,
	}, nil
}

// RetryTaskService 整 Run 重试:同 task 新 run(§76.47.3 RetryTask)。
// 冻结原 task 的 plan revision/spec;旧 run 终态不回退。
type RetryTaskService struct {
	repo *repository.Repo
	now  func() time.Time
}

func NewRetryTaskService(repo *repository.Repo) *RetryTaskService {
	return &RetryTaskService{repo: repo, now: time.Now}
}

// RetryTask 同 task 创建新 run(判别联合 tenant+actor+client_key;窗口 now+10min)。
func (s *RetryTaskService) RetryTask(ctx context.Context, tenantID, runID, actor, clientIdempotencyKey string) (*repository.MaterializedReceipt, error) {
	binding, err := s.repo.GetTaskByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if clientIdempotencyKey == "" {
		return nil, newAnalysisError(contract.ErrCodeMissingIdempotencyKey, "client_idempotency_key is required for retry-task")
	}
	canonicalIdentity := identityHash(tenantID, "actor", actor, clientIdempotencyKey)
	requestSHA := identityHash(canonicalIdentity, binding.ExecutionSpecSHA256, "retry-task", runID)
	// 重试新 run 为 ON_DEMAND 语义,不携带源 schedule_revision(不混入调度触发历史投影)。
	triggerID, created, err := s.repo.InsertTriggerInstance(ctx, tenantID, "actor", canonicalIdentity, requestSHA,
		"ON_DEMAND", "", binding.TaskDefinitionID, binding.PlanRevision, actor, binding.EffectiveClass, "{}", 0)
	if err != nil {
		return nil, err
	}
	if !created {
		trig, err := s.repo.FindTriggerInstanceByIdentity(ctx, tenantID, "actor", canonicalIdentity)
		if err != nil || trig == nil {
			return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "retry-task idempotency resolution failed")
		}
		if trig.MaterializedTaskID != "" {
			runID, err := s.repo.GetTaskRunBinding(ctx, tenantID, trig.MaterializedTaskID)
			if err != nil {
				return nil, err
			}
			return &repository.MaterializedReceipt{TaskID: trig.MaterializedTaskID, RunID: runID}, nil
		}
		triggerID = trig.TriggerID // PENDING_MATERIALIZATION 残留:继续物化
	}
	receipt, replayed, err := s.repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID:              tenantID,
		IdentityKind:          "actor",
		CanonicalIdentityHash: canonicalIdentity,
		RequestSHA256:         requestSHA,
		TriggerInstanceID:     triggerID,
		TriggerKind:           "ON_DEMAND",
		WindowStartMs:         s.now().UnixMilli(),
		WindowEndMs:           s.now().Add(10 * time.Minute).UnixMilli(),
		TaskDefinitionID:      binding.TaskDefinitionID,
		PlanRevision:          binding.PlanRevision,
		ExecutionSpecSHA256:   binding.ExecutionSpecSHA256,
		ScheduleRevision:      0,
		EffectiveClass:        binding.EffectiveClass,
		EffectivePolicySHA256: binding.EffectivePolicySHA,
		ResourcePool:          "analysis-cpu",
		ResourceVectorJSON:    []byte(`{"cpu":2}`),
		QueueCostMilli:        2000,
		ExpiresAt:             s.now().Add(5 * time.Minute),
		NodesJSON:             defaultNodeExactSet(),
		PlanSpecJSON:          []byte(`{}`),
	})
	if err != nil {
		if err == repository.ErrPayloadMismatch {
			return nil, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
		}
		return nil, err
	}
	if replayed {
		trig, err := s.repo.FindTriggerInstanceByIdentity(ctx, tenantID, "actor", canonicalIdentity)
		if err != nil || trig == nil || trig.MaterializedTaskID == "" {
			return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "idempotent replay without materialized task")
		}
		runID, err := s.repo.GetTaskRunBinding(ctx, tenantID, trig.MaterializedTaskID)
		if err != nil {
			return nil, err
		}
		return &repository.MaterializedReceipt{TaskID: trig.MaterializedTaskID, RunID: runID}, nil
	}
	return receipt, nil
}

// RetryReport 报告重试(FAILED/CANCELLED→原地重新排队;§10.3 报告状态独立,失败不回退 Run)。
// uq(run_id,summary,template,locale) 决定同一 run+参数组合唯一一行报告,重试复用同一报告行。
func (s *HumanReportService) RetryReport(ctx context.Context, tenantID, reportID, clientIdempotencyKey string) (newReportID string, replayed bool, err error) {
	if clientIdempotencyKey == "" {
		return "", false, newAnalysisError(contract.ErrCodeMissingIdempotencyKey, "client_idempotency_key is required for report retry")
	}
	requestSHA := identityHash("report-retry", tenantID, reportID, clientIdempotencyKey)
	replayed, err = s.repo.RetryReportAtomic(ctx, tenantID, reportID, clientIdempotencyKey, requestSHA)
	if err == repository.ErrPayloadMismatch {
		return "", false, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
	}
	if err != nil {
		return "", false, err
	}
	return reportID, replayed, nil
}

// DownloadReport 下载票消费(一次性):校验票归属/未过期/未使用 → 返回对象键。
func (s *HumanReportService) DownloadReport(ctx context.Context, tenantID, reportID, ticketID string) (objectKey string, err error) {
	if ticketID == "" {
		return "", newAnalysisError(contract.ErrCodeInvalidTransition, "ticket_id is required")
	}
	return s.repo.ConsumeDownloadTicketAtomic(ctx, tenantID, reportID, ticketID)
}

// IssueDownloadTicket 签发下载票(仅 AVAILABLE;短期有效 + 审计;§20 DownloadTicket)。
func (s *HumanReportService) IssueDownloadTicket(ctx context.Context, tenantID, reportID string, ttl time.Duration, actor string) (ticketID string, expiresAt time.Time, err error) {
	if ttl <= 0 || ttl > 24*time.Hour {
		ttl = 5 * time.Minute
	}
	ticketSecret := randHexID()
	ticketSHA := sha256Hex([]byte(ticketSecret))
	return s.repo.IssueDownloadTicketAtomic(ctx, tenantID, reportID, ticketSHA, ttl, actor)
}

// ResourceService 调度资源视图(admin;§20 Resources)。
type ResourceService struct {
	repo *repository.Repo
}

func NewResourceService(repo *repository.Repo) *ResourceService { return &ResourceService{repo: repo} }

func (s *ResourceService) Views(ctx context.Context, tenantID string) (*repository.ResourceViews, error) {
	return s.repo.GetResourceViews(ctx, tenantID)
}

// StageReceipts 阶段回执投影(运行详情"技术详情"页签)。
func (s *ResourceService) StageReceipts(ctx context.Context, tenantID, runID string) ([]repository.StageReceiptProjection, error) {
	return s.repo.ListStageReceiptsForRun(ctx, tenantID, runID)
}

// TriggerHistory 调度触发历史投影(调度详情页)。
func (s *ScheduleService) TriggerHistory(ctx context.Context, tenantID, scheduleID string) ([]repository.TriggerHistoryView, error) {
	return s.repo.ListTriggersForSchedule(ctx, tenantID, scheduleID)
}

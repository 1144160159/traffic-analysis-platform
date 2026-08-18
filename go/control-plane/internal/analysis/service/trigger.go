package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// TriggerService 触发与物化(四种触发归一为同一物化事务)。
type TriggerService struct {
	repo           *repository.Repo
	resolvers      map[string]PlanResolver
	compiler       *PlanCompiler
	now            func() time.Time
	templateLoader TemplateLoader
}

// TemplateLoader 装配侧钩子:按定义装载已批准模板与目录快照(真实环境接目录缓存)。
// 返回 contract.ErrCodePlanNotApproved 语义错误时按无可用计划处理。
type TemplateLoader func(ctx context.Context, tenantID, taskDefinitionID string) (*DefaultTemplate, CatalogSnapshot, error)

func NewTriggerService(repo *repository.Repo, defaultR, customR PlanResolver, c *PlanCompiler) *TriggerService {
	return &TriggerService{
		repo:      repo,
		resolvers: map[string]PlanResolver{"AUTO_DEFAULT": defaultR, "MANUAL_CUSTOM": customR},
		compiler:  c,
		now:       time.Now,
	}
}

// SetTemplateLoader 注入装配侧模板/目录加载器(可选;未注入时维持核心卷行为)。
func (s *TriggerService) SetTemplateLoader(l TemplateLoader) { s.templateLoader = l }

// SubmitRequest 按需触发(即时分析三步向导最终提交)。
type SubmitRequest struct {
	TenantID             string
	TaskDefinitionID     string
	PlanSource           string
	CustomOverrides      json.RawMessage
	SourceKind           string
	SourceSpec           json.RawMessage
	ClientIdempotencyKey string
	Actor                string
	ActorScopes          []string
	Catalog              CatalogSnapshot
	Template             *DefaultTemplate
	Approved             bool
	CustomReleased       bool
}

// SubmitResponse 202 回执(仅物化事务提交后返回)。
type SubmitResponse struct {
	TaskID              string
	RunID               string
	StatusURL           string
	ExecutionSpecSHA256 string
}

// Submit 解析计划→冻结→物化;同 identity 同 hash 精确重放,异 hash 409。
func (s *TriggerService) Submit(ctx context.Context, req SubmitRequest) (*SubmitResponse, error) {
	if strings.TrimSpace(req.ClientIdempotencyKey) == "" {
		return nil, newAnalysisError(contract.ErrCodeMissingIdempotencyKey, "idempotency key is required")
	}
	resolver, err := ResolveForPlanSource(s.resolvers, req.PlanSource)
	if err != nil {
		return nil, err
	}
	// 装配侧模板/目录注入:未显式携带模板时,由加载器从激活计划装配。
	// AUTO_DEFAULT 以模板为全部输入;MANUAL_CUSTOM 以模板为覆盖基座。
	if req.Template == nil && s.templateLoader != nil {
		tpl, catalog, err := s.templateLoader(ctx, req.TenantID, req.TaskDefinitionID)
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
	compiled, err := s.compiler.Compile(ctx, *intent)
	if err != nil {
		return nil, err
	}

	// 物化身份判别联合:operator 按需触发绑定 tenant+actor+client_idempotency_key
	canonicalIdentity := identityHash(req.TenantID, "actor", req.Actor, req.ClientIdempotencyKey)
	requestSHA := identityHash(canonicalIdentity, compiled.ExecutionSpecSHA256, req.SourceKind, string(req.SourceSpec))

	// 计划修订绑定:AUTO_DEFAULT 绑定激活默认计划修订;
	// MANUAL_CUSTOM 绑定已审批激活(ACTIVE)的人工定制计划修订,且编译哈希必须
	// 与冻结哈希一致(异哈希 409 拒绝,不静默挂到其他修订上)。
	planRevision := int64(0)
	switch {
	case req.Template != nil && req.PlanSource == "AUTO_DEFAULT":
		planRevision = req.Template.PlanRevision
	case req.PlanSource == "MANUAL_CUSTOM":
		row, err := s.repo.GetActivePlanForDefinitionBySource(ctx, req.TenantID, req.TaskDefinitionID, "MANUAL_CUSTOM")
		if err != nil {
			return nil, err
		}
		if row.ExecutionSpecSHA256 != compiled.ExecutionSpecSHA256 {
			return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "custom plan spec mismatch: re-save and approve the custom plan before triggering")
		}
		planRevision = row.PlanRevision
	}

	// 有效调度策略(§76.45.2):按需触发无 schedule、无授权 override → class =
	// definition.default_scheduling_class;requested = plan.resource_budget;
	// 硬上限逐维最小(当前无 schedule/trigger cap 层)。policy 与触发事实同事务冻结。
	defClass, err := s.repo.GetDefinitionDefaultClass(ctx, req.TenantID, req.TaskDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("load definition scheduling defaults: %w", err)
	}
	planBudget := json.RawMessage(`{"cpu":2}`)
	if req.Template != nil && len(req.Template.ResourceBudget) > 0 && string(req.Template.ResourceBudget) != "null" {
		planBudget = req.Template.ResourceBudget
	}
	policy, err := ResolveEffectiveSchedulingPolicy(PolicyInputs{
		DefaultClass: defClass,
		PlanBudget:   planBudget,
	})
	if err != nil {
		return nil, err
	}
	// DRR 向量折算(§76.45.3):cost 与冻结策略同源。
	queueCost, err := DRRVectorCost(policy.ResourceVector)
	if err != nil {
		return nil, fmt.Errorf("drr vector cost: %w", err)
	}

	// 触发实例先行(PENDING_MATERIALIZATION):物化事务内 FOR UPDATE 锁行;
	// 判别联合已存在时复用既有实例 id(幂等重放由台账裁决)。
	triggerID, created, err := s.repo.InsertTriggerInstance(ctx, req.TenantID, "actor",
		canonicalIdentity, requestSHA, "ON_DEMAND", "", req.TaskDefinitionID, planRevision, req.Actor, policy.Class, "{}", 0)
	if err != nil {
		return nil, err
	}
	if !created {
		trig, err := s.repo.FindTriggerInstanceByIdentity(ctx, req.TenantID, "actor", canonicalIdentity)
		if err != nil {
			return nil, err
		}
		if trig == nil {
			return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "trigger instance conflict resolution failed")
		}
		triggerID = trig.TriggerID
	}

	expires := s.now().Add(5 * time.Minute)

	receipt, replayed, err := s.repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID:              req.TenantID,
		IdentityKind:          "actor",
		CanonicalIdentityHash: canonicalIdentity,
		RequestSHA256:         requestSHA,
		TriggerInstanceID:     triggerID,
		TriggerKind:           "ON_DEMAND",
		WindowStartMs:         s.now().UnixMilli(),
		WindowEndMs:           s.now().Add(10 * time.Minute).UnixMilli(),
		TaskDefinitionID:      req.TaskDefinitionID,
		PlanRevision:          planRevision,
		ExecutionSpecSHA256:   compiled.ExecutionSpecSHA256,
		EffectiveClass:        policy.Class,
		EffectivePolicySHA256: policy.PolicySHA256,
		ResourcePool:          policy.ResourcePool,
		ResourceVectorJSON:    policy.ResourceVector,
		QueueCostMilli:        queueCost,
		ExpiresAt:             expires,
		NodesJSON:             defaultNodeExactSet(),
		PlanSpecJSON:          compiled.CanonicalSpecJSON,
	})
	if err != nil {
		if err == repository.ErrPayloadMismatch {
			return nil, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
		}
		return nil, err
	}
	if replayed {
		// 幂等重放:回源已物化任务与当前运行,返回同一次提交的句柄。
		trig, err := s.repo.FindTriggerInstanceByIdentity(ctx, req.TenantID, "actor", canonicalIdentity)
		if err != nil {
			return nil, err
		}
		if trig == nil || trig.MaterializedTaskID == "" {
			return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "idempotent replay without materialized task")
		}
		runID, err := s.repo.GetTaskRunBinding(ctx, req.TenantID, trig.MaterializedTaskID)
		if err != nil {
			return nil, err
		}
		return &SubmitResponse{
			TaskID:              trig.MaterializedTaskID,
			RunID:               runID,
			StatusURL:           fmt.Sprintf("/api/v1/analysis/runs/%s", runID),
			ExecutionSpecSHA256: compiled.ExecutionSpecSHA256,
		}, nil
	}
	return &SubmitResponse{
		TaskID:              receipt.TaskID,
		RunID:               receipt.RunID,
		StatusURL:           fmt.Sprintf("/api/v1/analysis/runs/%s", receipt.RunID),
		ExecutionSpecSHA256: compiled.ExecutionSpecSHA256,
	}, nil
}

// identityHash 稳定身份哈希(判别联合)。
func identityHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// DefaultNodeExactSet 核心主链 required ExecutionNode exact-set(对齐方案 §11;
// 导出供集成测试与装配侧使用)。
func DefaultNodeExactSet() []byte { return defaultNodeExactSet() }

// defaultNodeExactSet 核心主链 required ExecutionNode exact-set(对齐方案 §11)。
func defaultNodeExactSet() []byte {
	nodes := []map[string]string{
		{"business_phase_id": "S1", "execution_node_id": "SOURCE_ACTIVATE", "provider_mode": "DEDICATED_OPERATION", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S2", "execution_node_id": "SESSIONIZATION", "provider_mode": "SHARED_STREAM", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S2", "execution_node_id": "FEATURE_EXTRACTION", "provider_mode": "SHARED_STREAM", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S3", "execution_node_id": "ENCRYPTED_RECOGNIZER", "provider_mode": "SHARED_STREAM", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S4", "execution_node_id": "RULE_DETECTION", "provider_mode": "SHARED_STREAM", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S4", "execution_node_id": "BEHAVIOR_DETECTION", "provider_mode": "SHARED_STREAM", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S4", "execution_node_id": "DETECTION_AGGREGATE", "provider_mode": "SHARED_STREAM", "activation_mode": "PIPELINED_STREAM"},
		{"business_phase_id": "S5", "execution_node_id": "RECONCILE", "provider_mode": "AUTHORITY_LOCAL", "activation_mode": "AUTHORITY_LOCAL"},
		{"business_phase_id": "S5", "execution_node_id": "MACHINE_FINALIZATION", "provider_mode": "AUTHORITY_LOCAL", "activation_mode": "AUTHORITY_LOCAL"},
	}
	b, _ := json.Marshal(nodes)
	return b
}

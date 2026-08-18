package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// PlanAuthorService 人工选择列车(P2)服务端:
// 定制计划草稿保存(preflight→resolve→compile→SavePlanDraftAtomic)与 maker/checker 审批激活。
// 不触发、不物化;触发由 TriggerService 以 ACTIVE 定制修订绑定。
type PlanAuthorService struct {
	repo           *repository.Repo
	compiler       *PlanCompiler
	customResolver *CustomPlanResolver
	loader         TemplateLoader
}

// NewPlanAuthorService 构造人工选择列车服务。
func NewPlanAuthorService(repo *repository.Repo, compiler *PlanCompiler, customResolver *CustomPlanResolver, loader TemplateLoader) *PlanAuthorService {
	return &PlanAuthorService{repo: repo, compiler: compiler, customResolver: customResolver, loader: loader}
}

// SaveCustomPlanRequest 定制计划草稿保存请求。
type SaveCustomPlanRequest struct {
	TenantID             string
	TaskDefinitionID     string
	CustomOverrides      json.RawMessage
	Actor                string
	ClientIdempotencyKey string
}

// SaveCustomPlanResponse 草稿保存结果(plan_revision 由仓储事务内分配)。
type SaveCustomPlanResponse struct {
	PlanID              string `json:"plan_id"`
	PlanRevision        int64  `json:"plan_revision"`
	ExecutionSpecSHA256 string `json:"execution_spec_sha256"`
	PlanRevisionSHA256  string `json:"plan_revision_sha256"`
}

// SaveCustom 保存 MANUAL_CUSTOM 草稿(DRAFT 治理头;同 identity 同 hash 幂等回源)。
func (s *PlanAuthorService) SaveCustom(ctx context.Context, req SaveCustomPlanRequest) (*SaveCustomPlanResponse, error) {
	if strings.TrimSpace(req.ClientIdempotencyKey) == "" {
		return nil, newAnalysisError(contract.ErrCodeMissingIdempotencyKey, "idempotency key is required")
	}
	if s.loader == nil {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "template loader is not configured")
	}
	tpl, catalog, err := s.loader(ctx, req.TenantID, req.TaskDefinitionID)
	if err != nil {
		return nil, err
	}
	intent, err := s.customResolver.Resolve(ctx, ResolveRequest{
		TenantID:         req.TenantID,
		TaskDefinitionID: req.TaskDefinitionID,
		PlanSource:       "MANUAL_CUSTOM",
		CustomOverrides:  req.CustomOverrides,
		Actor:            req.Actor,
		Catalog:          catalog,
		Template:         tpl,
		Approved:         true,
		CustomReleased:   true,
	})
	if err != nil {
		return nil, err
	}
	compiled, err := s.compiler.Compile(ctx, *intent)
	if err != nil {
		return nil, err
	}

	identity := identityHash("plan", req.TenantID, req.TaskDefinitionID, req.Actor, req.ClientIdempotencyKey)
	requestSHA := identityHash(identity, compiled.ExecutionSpecSHA256)

	planID, planRevision, replayed, err := s.repo.SavePlanDraftAtomic(ctx, repository.SavePlanDraftCommand{
		TenantID:                req.TenantID,
		TaskDefinitionID:        req.TaskDefinitionID,
		PlanRevision:            0, // 仓储事务内分配
		PlanSource:              "MANUAL_CUSTOM",
		SourceKind:              intent.SourceKind,
		SourceSpec:              intent.SourceSpec,
		SelectedFeatureIDs:      mustJSONList(intent.SelectedFeatureIDs),
		FeatureSetID:            intent.FeatureSetID,
		RecognitionModel:        intent.EncryptedRecognitionModelRef,
		DetectorRefs:            mustJSONList(intent.ThreatDetectorRefs),
		RuleRefs:                mustJSONList(intent.RuleRefs),
		MachineSummarySchemaRef: intent.MachineSummarySchemaRef,
		StageDAG:                intent.StageDAG,
		CompletionPolicy:        intent.CompletionPolicy,
		ResourceBudget:          intent.ResourceBudget,
		CatalogRevision:         intent.CatalogRevision,
		SelectionOrigins:        mustJSONList(intent.SelectionOrigins),
		ExecutionSpecSHA256:     compiled.ExecutionSpecSHA256,
		PlanRevisionSHA256:      compiled.PlanRevisionSHA256,
		CreatedBy:               req.Actor,
		RequestSHA256:           requestSHA,
		IdempotencyKey:          identity,
	})
	if err != nil {
		if err == repository.ErrPayloadMismatch {
			return nil, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
		}
		return nil, err
	}
	if replayed {
		// 幂等回源:按执行哈希定位同一草稿修订
		id, rev, err := s.repo.FindPlanByExecutionSpec(ctx, req.TenantID, req.TaskDefinitionID, "MANUAL_CUSTOM", compiled.ExecutionSpecSHA256)
		if err != nil {
			return nil, err
		}
		planID, planRevision = id, rev
	}
	return &SaveCustomPlanResponse{
		PlanID:              planID,
		PlanRevision:        planRevision,
		ExecutionSpecSHA256: compiled.ExecutionSpecSHA256,
		PlanRevisionSHA256:  compiled.PlanRevisionSHA256,
	}, nil
}

// ApprovePlanRequest maker/checker 审批请求。
type ApprovePlanRequest struct {
	TenantID string
	PlanID   string
	Maker    string
	Checker  string
}

// Approve maker/checker 审批并激活(ACTIVE)。同人审批拒绝;已激活幂等返回。
func (s *PlanAuthorService) Approve(ctx context.Context, req ApprovePlanRequest) error {
	if strings.TrimSpace(req.Maker) == "" || strings.TrimSpace(req.Checker) == "" {
		return newAnalysisError(contract.ErrCodeInvalidTransition, "maker and checker are required")
	}
	if req.Maker == req.Checker {
		return newAnalysisError(contract.ErrCodeInvalidTransition, "maker and checker must differ")
	}
	state, rev, err := s.repo.GetPlanGovernanceHead(ctx, req.TenantID, req.PlanID)
	if err != nil {
		return err
	}
	if state == "ACTIVE" {
		return nil // 幂等:已激活
	}
	if state != "DRAFT" {
		return newAnalysisError(contract.ErrCodeInvalidTransition, "only DRAFT plans can be approved")
	}
	if err := s.repo.ApproveOrActivatePlanAtomic(ctx, repository.ApprovePlanCommand{
		TenantID:         req.TenantID,
		PlanID:           req.PlanID,
		Maker:            req.Maker,
		Checker:          req.Checker,
		ExpectedRevision: rev,
		NewState:         "ACTIVE",
	}); err != nil {
		return err
	}
	return nil
}

// SaveDefault 保存 AUTO_DEFAULT 修订(模板=全部输入;默认值即全部输入,无覆盖项)。
// 供任务编排页保存默认计划 + 计划冻结哈希漂移后的重冻结(新修订,旧修订不动)。
func (s *PlanAuthorService) SaveDefault(ctx context.Context, req SaveCustomPlanRequest) (*SaveCustomPlanResponse, error) {
	if strings.TrimSpace(req.ClientIdempotencyKey) == "" {
		return nil, newAnalysisError(contract.ErrCodeMissingIdempotencyKey, "idempotency key is required")
	}
	if s.loader == nil {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "template loader is not configured")
	}
	tpl, catalog, err := s.loader(ctx, req.TenantID, req.TaskDefinitionID)
	if err != nil {
		return nil, err
	}
	intent, err := NewDefaultPlanResolver(s.compiler).Resolve(ctx, ResolveRequest{
		TenantID:         req.TenantID,
		TaskDefinitionID: req.TaskDefinitionID,
		PlanSource:       "AUTO_DEFAULT",
		Catalog:          catalog,
		Template:         tpl,
	})
	if err != nil {
		return nil, err
	}
	compiled, err := s.compiler.Compile(ctx, *intent)
	if err != nil {
		return nil, err
	}

	identity := identityHash("plan-default", req.TenantID, req.TaskDefinitionID, req.Actor, req.ClientIdempotencyKey)
	requestSHA := identityHash(identity, compiled.ExecutionSpecSHA256)

	planID, planRevision, replayed, err := s.repo.SavePlanDraftAtomic(ctx, repository.SavePlanDraftCommand{
		TenantID:                req.TenantID,
		TaskDefinitionID:        req.TaskDefinitionID,
		PlanRevision:            0, // 仓储事务内分配
		PlanSource:              "AUTO_DEFAULT",
		SourceKind:              intent.SourceKind,
		SourceSpec:              intent.SourceSpec,
		SelectedFeatureIDs:      mustJSONList(intent.SelectedFeatureIDs),
		FeatureSetID:            intent.FeatureSetID,
		RecognitionModel:        intent.EncryptedRecognitionModelRef,
		DetectorRefs:            mustJSONList(intent.ThreatDetectorRefs),
		RuleRefs:                mustJSONList(intent.RuleRefs),
		MachineSummarySchemaRef: intent.MachineSummarySchemaRef,
		StageDAG:                intent.StageDAG,
		CompletionPolicy:        intent.CompletionPolicy,
		ResourceBudget:          intent.ResourceBudget,
		CatalogRevision:         intent.CatalogRevision,
		SelectionOrigins:        mustJSONList(intent.SelectionOrigins),
		ExecutionSpecSHA256:     compiled.ExecutionSpecSHA256,
		PlanRevisionSHA256:      compiled.PlanRevisionSHA256,
		CreatedBy:               req.Actor,
		RequestSHA256:           requestSHA,
		IdempotencyKey:          identity,
	})
	if err != nil {
		if err == repository.ErrPayloadMismatch {
			return nil, newAnalysisError(contract.ErrCodeIdempotencyPayloadMismatch, "same idempotency key with different payload")
		}
		return nil, err
	}
	if replayed {
		id, rev, err := s.repo.FindPlanByExecutionSpec(ctx, req.TenantID, req.TaskDefinitionID, "AUTO_DEFAULT", compiled.ExecutionSpecSHA256)
		if err != nil {
			return nil, err
		}
		planID, planRevision = id, rev
	}
	return &SaveCustomPlanResponse{
		PlanID:              planID,
		PlanRevision:        planRevision,
		ExecutionSpecSHA256: compiled.ExecutionSpecSHA256,
		PlanRevisionSHA256:  compiled.PlanRevisionSHA256,
	}, nil
}

// mustJSONList 序列化字符串数组(解析器已保证合法,失败仅防御)。
func mustJSONList(v []string) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// CatalogSnapshot 版本化目录(特征集/识别模型/检测器/规则/探针)。
type CatalogSnapshot struct {
	Revision          int64               `json:"revision"`
	FeatureSets       map[string][]string `json:"feature_sets"`       // feature_set_id -> feature ids
	RecognitionModels []string            `json:"recognition_models"` // refs
	ThreatDetectors   []string            `json:"threat_detectors"`   // refs
	Rules             []string            `json:"rules"`              // refs
	Probes            []string            `json:"probes"`             // probe ids
}

// DefaultTemplate 已批准默认模板(自动计划输入)。
type DefaultTemplate struct {
	TaskDefinitionID string          `json:"task_definition_id"`
	PlanSource       string          `json:"plan_source"`
	PlanRevision     int64           `json:"plan_revision,omitempty"`
	SourceKind       string          `json:"source_kind"`
	SourceSpec       json.RawMessage `json:"source_spec"`
	FeatureSetID     string          `json:"feature_set_id"`
	RecognitionModel string          `json:"recognition_model_ref"`
	DetectorRefs     []string        `json:"threat_detector_refs"`
	RuleRefs         []string        `json:"rule_refs"`
	// MachineSummarySchemaRef 机器摘要 schema 引用(执行哈希冻结字段;装配层必须原样携带,
	// 丢失会导致同内容不同哈希——见 live 计划冻结哈希漂移修复)。
	MachineSummarySchemaRef string          `json:"machine_summary_schema_ref"`
	StageDAG                json.RawMessage `json:"stage_dag"`
	CompletionPolicy        json.RawMessage `json:"completion_policy"`
	ResourceBudget          json.RawMessage `json:"resource_budget"`
}

// PlanResolver 计划来源解析器:只产 NormalizedAnalysisIntent,不触发、不写库。
type PlanResolver interface {
	Resolve(ctx context.Context, req ResolveRequest) (*NormalizedAnalysisIntent, error)
}

// ResolveRequest 解析请求(Default/Custom 共用;custom_overrides 仅 custom 携带)。
type ResolveRequest struct {
	TenantID         string           `json:"tenant_id"`
	TaskDefinitionID string           `json:"task_definition_id"`
	PlanSource       string           `json:"plan_source"`
	CustomOverrides  json.RawMessage  `json:"custom_overrides"` // 探针/特征/识别模型/检测模型/规则/阈值
	Actor            string           `json:"actor"`
	ActorScopes      []string         `json:"actor_scopes"`
	Catalog          CatalogSnapshot  `json:"catalog"`
	Template         *DefaultTemplate `json:"template"`
	Approved         bool             `json:"approved"` // 审批字段 maker/checker 是否闭合
	CustomReleased   bool             `json:"custom_released"`
}

// DefaultPlanResolver 自动默认:只读已批准模板,默认值即全部输入。
type DefaultPlanResolver struct {
	compiler *PlanCompiler
}

func NewDefaultPlanResolver(c *PlanCompiler) *DefaultPlanResolver {
	return &DefaultPlanResolver{compiler: c}
}

func (r *DefaultPlanResolver) Resolve(ctx context.Context, req ResolveRequest) (*NormalizedAnalysisIntent, error) {
	if req.Template == nil {
		return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "no approved default template for this task definition")
	}
	features := req.Catalog.FeatureSets[req.Template.FeatureSetID]
	if features == nil {
		return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "template feature set not in catalog revision")
	}
	selected := append([]string(nil), features...)
	sort.Strings(selected)
	intent := &NormalizedAnalysisIntent{
		TenantID:                     req.TenantID,
		TaskDefinitionID:             req.TaskDefinitionID,
		PlanSource:                   "AUTO_DEFAULT",
		SourceKind:                   req.Template.SourceKind,
		SourceSpec:                   req.Template.SourceSpec,
		SelectedFeatureIDs:           selected,
		FeatureSetID:                 req.Template.FeatureSetID,
		EncryptedRecognitionModelRef: req.Template.RecognitionModel,
		ThreatDetectorRefs:           append([]string(nil), req.Template.DetectorRefs...),
		RuleRefs:                     append([]string(nil), req.Template.RuleRefs...),
		MachineSummarySchemaRef:      req.Template.MachineSummarySchemaRef,
		StageDAG:                     req.Template.StageDAG,
		CompletionPolicy:             req.Template.CompletionPolicy,
		ResourceBudget:               req.Template.ResourceBudget,
		CatalogRevision:              req.Catalog.Revision,
		SelectionOrigins:             []string{"default-template:" + req.Template.FeatureSetID},
	}
	if _, err := r.compiler.Compile(ctx, *intent); err != nil {
		return nil, err
	}
	return intent, nil
}

// CustomOverrides 人工覆盖项(主业务链执行环节):探针/采集源/特征/识别模型/检测模型/规则/阈值。
type CustomOverrides struct {
	SourceKind                   *string         `json:"source_kind,omitempty"`
	SourceSpec                   json.RawMessage `json:"source_spec,omitempty"`
	ProbeID                      *string         `json:"probe_id,omitempty"`
	SelectedFeatureIDs           []string        `json:"selected_feature_ids,omitempty"`
	EncryptedRecognitionModelRef *string         `json:"encrypted_recognition_model_ref,omitempty"`
	ThreatDetectorRefs           []string        `json:"threat_detector_refs,omitempty"`
	RuleRefs                     []string        `json:"rule_refs,omitempty"`
	Thresholds                   json.RawMessage `json:"thresholds,omitempty"`
	CompletionPolicy             json.RawMessage `json:"completion_policy,omitempty"`
	ResourceBudget               json.RawMessage `json:"resource_budget,omitempty"`
}

// CustomPlanResolver 人工定制:覆盖项规范化,未覆盖字段按模板填充并记 selection_origins;
// 与默认计划经同一 PlanCompiler 冻结,人工选择只进入本次 plan revision,不改全局 active 指针。
type CustomPlanResolver struct {
	compiler *PlanCompiler
}

func NewCustomPlanResolver(c *PlanCompiler) *CustomPlanResolver {
	return &CustomPlanResolver{compiler: c}
}

func (r *CustomPlanResolver) Resolve(ctx context.Context, req ResolveRequest) (*NormalizedAnalysisIntent, error) {
	if !req.CustomReleased {
		return nil, newAnalysisError(contract.ErrCodeFeatureNotReleased, "manual custom plans are not released")
	}
	if req.Template == nil {
		return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "custom plans require an approved base template")
	}
	// 覆盖项必须存在,空覆盖视为请求错误(区别于 default)
	if len(req.CustomOverrides) == 0 || string(req.CustomOverrides) == "{}" || string(req.CustomOverrides) == "null" {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "custom_overrides is required for MANUAL_CUSTOM")
	}
	var ov CustomOverrides
	if err := json.Unmarshal(req.CustomOverrides, &ov); err != nil {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "invalid custom_overrides")
	}

	// 从模板基线开始(与 AUTO 同基线,保证未覆盖字段一致)
	base := NormalizedAnalysisIntent{
		TenantID:                     req.TenantID,
		TaskDefinitionID:             req.TaskDefinitionID,
		PlanSource:                   "MANUAL_CUSTOM",
		SourceKind:                   req.Template.SourceKind,
		SourceSpec:                   req.Template.SourceSpec,
		FeatureSetID:                 req.Template.FeatureSetID,
		EncryptedRecognitionModelRef: req.Template.RecognitionModel,
		ThreatDetectorRefs:           append([]string(nil), req.Template.DetectorRefs...),
		RuleRefs:                     append([]string(nil), req.Template.RuleRefs...),
		MachineSummarySchemaRef:      req.Template.MachineSummarySchemaRef,
		StageDAG:                     req.Template.StageDAG,
		CompletionPolicy:             req.Template.CompletionPolicy,
		ResourceBudget:               req.Template.ResourceBudget,
		CatalogRevision:              req.Catalog.Revision,
	}
	base.SelectedFeatureIDs = append([]string(nil), req.Catalog.FeatureSets[req.Template.FeatureSetID]...)

	origins := []string{"template:" + req.Template.FeatureSetID}
	applyOverride := func(field string, changed bool) {
		if changed {
			origins = append(origins, "actor:"+req.Actor+":field:"+field)
		}
	}

	if ov.SourceKind != nil {
		base.SourceKind = *ov.SourceKind
		applyOverride("source_kind", true)
	}
	if len(ov.SourceSpec) > 0 {
		base.SourceSpec = ov.SourceSpec
		applyOverride("source_spec", true)
	}
	if ov.ProbeID != nil {
		// 探针选择校验:必须在目录中存在
		found := false
		for _, p := range req.Catalog.Probes {
			if p == *ov.ProbeID {
				found = true
				break
			}
		}
		if !found {
			return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "probe not in catalog")
		}
		applyOverride("probe_id", true)
	}
	if len(ov.SelectedFeatureIDs) > 0 {
		// 特征 exact-set:必须都在 feature_set 内(闭包校验由 Compiler 负责)
		base.SelectedFeatureIDs = append([]string(nil), ov.SelectedFeatureIDs...)
		applyOverride("selected_feature_ids", true)
	}
	if ov.EncryptedRecognitionModelRef != nil {
		if !contains(req.Catalog.RecognitionModels, *ov.EncryptedRecognitionModelRef) {
			return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "recognition model not in catalog")
		}
		base.EncryptedRecognitionModelRef = *ov.EncryptedRecognitionModelRef
		applyOverride("encrypted_recognition_model_ref", true)
	}
	if len(ov.ThreatDetectorRefs) > 0 {
		for _, d := range ov.ThreatDetectorRefs {
			if !contains(req.Catalog.ThreatDetectors, d) {
				return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "detector not in catalog: "+d)
			}
		}
		base.ThreatDetectorRefs = append([]string(nil), ov.ThreatDetectorRefs...)
		applyOverride("threat_detector_refs", true)
	}
	if len(ov.RuleRefs) > 0 {
		for _, ru := range ov.RuleRefs {
			if !contains(req.Catalog.Rules, ru) {
				return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "rule not in catalog: "+ru)
			}
		}
		base.RuleRefs = append([]string(nil), ov.RuleRefs...)
		applyOverride("rule_refs", true)
	}
	if len(ov.CompletionPolicy) > 0 {
		base.CompletionPolicy = ov.CompletionPolicy
		applyOverride("completion_policy", true)
	}
	if len(ov.ResourceBudget) > 0 {
		base.ResourceBudget = ov.ResourceBudget
		applyOverride("resource_budget", true)
	}
	// 阈值并进 completion_policy(检测阈值属于计划冻结字段)
	if len(ov.Thresholds) > 0 {
		base.CompletionPolicy = mergeJSON(base.CompletionPolicy, map[string]interface{}{"thresholds": json.RawMessage(ov.Thresholds)})
		applyOverride("thresholds", true)
	}

	sort.Strings(base.SelectedFeatureIDs)
	base.SelectionOrigins = origins
	if !req.Approved {
		return nil, newAnalysisError(contract.ErrCodePlanNotApproved, "approval (maker/checker) is required for custom plan")
	}

	if _, err := r.compiler.Compile(ctx, base); err != nil {
		return nil, err
	}
	return &base, nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if strings.TrimSpace(s) == strings.TrimSpace(v) {
			return true
		}
	}
	return false
}

// mergeJSON 浅合并两个 JSON 对象(阈值并入 completion_policy)。
func mergeJSON(base json.RawMessage, extra map[string]interface{}) json.RawMessage {
	m := map[string]interface{}{}
	if len(base) > 0 {
		_ = json.Unmarshal(base, &m)
	}
	for k, v := range extra {
		m[k] = v
	}
	out, err := json.Marshal(m)
	if err != nil {
		return base
	}
	return out
}

// ResolveForPlanSource 按 plan_source 选择解析器(主业务链两个执行环节同一出口)。
func ResolveForPlanSource(resolvers map[string]PlanResolver, planSource string) (PlanResolver, error) {
	r, ok := resolvers[planSource]
	if !ok {
		return nil, fmt.Errorf("%s: unknown plan_source %q", contract.ErrCodeInvalidTransition, planSource)
	}
	return r, nil
}

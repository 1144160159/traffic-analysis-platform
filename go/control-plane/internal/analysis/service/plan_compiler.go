// Package service 调度域应用服务(薄编排,事务在 repository)。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// CanonicalizationVersion 规范化版本(变更必须递增并保留旧版读取)。
const CanonicalizationVersion = "v1"

// NormalizedAnalysisIntent 规范化计划意图(Default/Custom Resolver 的共同输出)。
// plan_source 与 selection_origins 不进入 execution_spec_sha256。
type NormalizedAnalysisIntent struct {
	TenantID                     string   `json:"tenant_id"`
	TaskDefinitionID             string   `json:"task_definition_id"`
	PlanSource                   string   `json:"plan_source"` // AUTO_DEFAULT|MANUAL_CUSTOM
	SourceKind                   string   `json:"source_kind"`
	SourceSpec                   json.RawMessage `json:"source_spec"`
	SelectedFeatureIDs           []string `json:"selected_feature_ids"`
	FeatureSetID                 string   `json:"feature_set_id"`
	EncryptedRecognitionModelRef string   `json:"encrypted_recognition_model_ref"`
	ThreatDetectorRefs           []string `json:"threat_detector_refs"`
	RuleRefs                     []string `json:"rule_refs"`
	MachineSummarySchemaRef      string   `json:"machine_summary_schema_ref"`
	StageDAG                     json.RawMessage `json:"stage_dag"`
	CompletionPolicy             json.RawMessage `json:"completion_policy"`
	ResourceBudget               json.RawMessage `json:"resource_budget"`
	CatalogRevision              int64    `json:"catalog_revision"`
	SelectionOrigins             []string `json:"selection_origins"`
}

// CompiledPlanRevision 冻结结果。
type CompiledPlanRevision struct {
	ExecutionSpecSHA256  string
	PlanRevisionSHA256   string
	CanonicalSpecJSON    []byte
}

// PlanCompiler 把 NormalizedIntent 冻结为不可变计划(与 AUTO/MANUAL 同一编译器)。
type PlanCompiler struct{}

func NewPlanCompiler() *PlanCompiler { return &PlanCompiler{} }

// executionCanonicalFields 执行哈希只覆盖规范化后的运行字段
// (排除 plan_source/selection_origins/tenant 以外治理元数据)。
func executionCanonicalFields(i NormalizedAnalysisIntent) map[string]interface{} {
	sort.Strings(i.SelectedFeatureIDs)
	sort.Strings(i.ThreatDetectorRefs)
	sort.Strings(i.RuleRefs)
	return map[string]interface{}{
		"tenant_id":                        i.TenantID,
		"task_definition_id":               i.TaskDefinitionID,
		"source_kind":                      i.SourceKind,
		"source_spec":                      rawJSON(i.SourceSpec),
		"selected_feature_ids":             i.SelectedFeatureIDs,
		"feature_set_id":                   i.FeatureSetID,
		"encrypted_recognition_model_ref":  i.EncryptedRecognitionModelRef,
		"threat_detector_refs":             i.ThreatDetectorRefs,
		"rule_refs":                        i.RuleRefs,
		"machine_summary_schema_ref":       i.MachineSummarySchemaRef,
		"stage_dag":                        rawJSON(i.StageDAG),
		"completion_policy":                rawJSON(i.CompletionPolicy),
		"resource_budget":                  rawJSON(i.ResourceBudget),
		"catalog_revision":                 i.CatalogRevision,
		"canonicalization_version":         CanonicalizationVersion,
	}
}

// rawJSON 保留结构化 JSON(键序规范化)。
func rawJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b) // 非法 JSON 保留原文并让校验阶段拒绝
	}
	return normalizeJSON(v)
}

// normalizeJSON 递归键排序,保证 canonical bytes 稳定。
func normalizeJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(t))
		for _, k := range keys {
			out[k] = normalizeJSON(t[k])
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = normalizeJSON(e)
		}
		return out
	default:
		return v
	}
}

// sha256Hex 小工具。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Compile 冻结计划:校验→canonical JSON→双哈希(纯函数,不写库)。
func (c *PlanCompiler) Compile(_ context.Context, intent NormalizedAnalysisIntent) (*CompiledPlanRevision, error) {
	if strings.TrimSpace(intent.TenantID) == "" || strings.TrimSpace(intent.TaskDefinitionID) == "" {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "tenant_id and task_definition_id are required")
	}
	if len(intent.SelectedFeatureIDs) == 0 && intent.FeatureSetID == "" {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "feature exact-set is empty")
	}
	if intent.SourceKind != "LIVE_STREAM_WINDOW" && intent.SourceKind != "PROBE_CAPTURE_WINDOW" && intent.SourceKind != "PCAP_REPLAY" {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "unknown source_kind")
	}
	if len(intent.ThreatDetectorRefs) == 0 {
		return nil, newAnalysisError(contract.ErrCodeInvalidTransition, "threat_detector_refs is required")
	}

	execFields := executionCanonicalFields(intent)
	execBytes, err := json.Marshal(execFields)
	if err != nil {
		return nil, fmt.Errorf("marshal execution canonical fields: %w", err)
	}

	// plan_revision_sha256 覆盖执行哈希+来源+选择依据+治理元数据(审批/审计用)。
	govFields := map[string]interface{}{
		"execution_spec_sha256": sha256Hex(execBytes),
		"plan_source":           intent.PlanSource,
		"selection_origins":     append([]string(nil), intent.SelectionOrigins...),
	}
	govBytes, err := json.Marshal(govFields)
	if err != nil {
		return nil, fmt.Errorf("marshal governance canonical fields: %w", err)
	}

	return &CompiledPlanRevision{
		ExecutionSpecSHA256: sha256Hex(execBytes),
		PlanRevisionSHA256:  sha256Hex(govBytes),
		CanonicalSpecJSON:   execBytes,
	}, nil
}

// newAnalysisError 统一错误框架入口(common/errors AppError;HTTP/重试分类由码表统一裁决)。
func newAnalysisError(code commonerrors.ErrorCode, message string) *commonerrors.AppError {
	return commonerrors.New(code, message)
}

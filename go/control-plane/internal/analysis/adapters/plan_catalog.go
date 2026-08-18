// Package adapters 装配侧适配器:模板/目录加载(PROJECT_ADAPTATION)。
// 核心卷将"目录/模板从 service 装配侧注入"留为适配点;本实现从 PG 激活计划行装配
// DefaultTemplate,并以计划冻结的 selected_feature_ids 合成目录快照特征集
// (真实环境可将 LoadTemplate 替换为目录缓存服务,不改服务层契约)。
package adapters

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

// PGPlanTemplateProvider 从 PG 读取激活计划并装配模板与目录快照。
type PGPlanTemplateProvider struct {
	repo *repository.Repo
}

// NewPGPlanTemplateProvider 构造装配侧模板加载器。
func NewPGPlanTemplateProvider(repo *repository.Repo) *PGPlanTemplateProvider {
	return &PGPlanTemplateProvider{repo: repo}
}

// LoadTemplate 读取激活的 AUTO_DEFAULT 计划并装配模板+目录快照
// (人工选择以默认计划为覆盖基座,同样经此加载)。
// 无激活默认计划时返回 contract.ErrCodePlanNotApproved 语义错误(fail-closed)。
func (p *PGPlanTemplateProvider) LoadTemplate(ctx context.Context, tenantID, taskDefinitionID string) (*service.DefaultTemplate, service.CatalogSnapshot, error) {
	row, err := p.repo.GetActivePlanForDefinitionBySource(ctx, tenantID, taskDefinitionID, "AUTO_DEFAULT")
	if err != nil {
		return nil, service.CatalogSnapshot{}, err
	}
	tpl, catalog, err := BuildTemplateFromPlanRow(taskDefinitionID, row)
	if err != nil {
		return nil, service.CatalogSnapshot{}, err
	}
	return tpl, catalog, nil
}

// BuildTemplateFromPlanRow 纯映射:激活计划行 → DefaultTemplate + CatalogSnapshot。
// 冻结字段损坏(feature/detector/rule JSON 非法)一律失败,不猜测。
func BuildTemplateFromPlanRow(taskDefinitionID string, row *repository.ActivePlanRow) (*service.DefaultTemplate, service.CatalogSnapshot, error) {
	var features []string
	if err := json.Unmarshal(row.SelectedFeatureIDs, &features); err != nil {
		return nil, service.CatalogSnapshot{}, fmt.Errorf("%s: selected_feature_ids corrupt: %w", contract.ErrCodePlanNotApproved, err)
	}
	var detectors []string
	if len(row.DetectorRefs) > 0 && string(row.DetectorRefs) != "null" {
		if err := json.Unmarshal(row.DetectorRefs, &detectors); err != nil {
			return nil, service.CatalogSnapshot{}, fmt.Errorf("%s: threat_detector_refs corrupt: %w", contract.ErrCodePlanNotApproved, err)
		}
	}
	var rules []string
	if len(row.RuleRefs) > 0 && string(row.RuleRefs) != "null" {
		if err := json.Unmarshal(row.RuleRefs, &rules); err != nil {
			return nil, service.CatalogSnapshot{}, fmt.Errorf("%s: rule_refs corrupt: %w", contract.ErrCodePlanNotApproved, err)
		}
	}
	recognitionModels := []string{}
	if row.RecognitionModel != "" {
		recognitionModels = append(recognitionModels, row.RecognitionModel)
	}
	tpl := &service.DefaultTemplate{
		TaskDefinitionID:        taskDefinitionID,
		PlanSource:              row.PlanSource,
		PlanRevision:            row.PlanRevision,
		SourceKind:              row.SourceKind,
		SourceSpec:              row.SourceSpec,
		FeatureSetID:            row.FeatureSetID,
		RecognitionModel:        row.RecognitionModel,
		DetectorRefs:            detectors,
		RuleRefs:                rules,
		MachineSummarySchemaRef: row.MachineSummarySchemaRef,
		StageDAG:                row.StageDAG,
		CompletionPolicy:        row.CompletionPolicy,
		ResourceBudget:          row.ResourceBudget,
	}
	catalog := service.CatalogSnapshot{
		Revision:          row.CatalogRevision,
		FeatureSets:       map[string][]string{row.FeatureSetID: features},
		RecognitionModels: recognitionModels,
		ThreatDetectors:   detectors,
		Rules:             rules,
	}
	return tpl, catalog, nil
}

// Package service §20/§21 allowedActions 服务端驱动:动作授权由服务端按
// Run 六轴 + attempt 事实 + 报告状态计算(前端只渲染,不自行判定;被拒动作不隐藏,403/409 保留审计)。
package service

import (
	"context"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// allowedActionsReader 只读端口(依赖倒置,便于测试)。
type allowedActionsReader interface {
	GetRun(ctx context.Context, tenantID, runID string) (*repository.RunView, error)
	ListRunStages(ctx context.Context, tenantID, runID string) ([]repository.RunStageView, error)
	GetRunSummaryHash(ctx context.Context, tenantID, runID string) (*repository.ReportSummaryRef, error)
	GetTaskDefinitionDetail(ctx context.Context, tenantID, defID string) (*repository.TaskDefinitionDetail, error)
}

// AllowedActionsService 动作授权判定(服务端唯一权威;UI 不得自行推断)。
type AllowedActionsService struct {
	repo allowedActionsReader
}

func NewAllowedActionsService(r allowedActionsReader) *AllowedActionsService {
	return &AllowedActionsService{repo: r}
}

// RunAllowedActions 运行详情动作集(§76.47.3 重试族 + 取消 + 报告请求)。
type RunAllowedActions struct {
	RunID         string `json:"run_id"`
	State         string `json:"state"`
	Cancel        bool   `json:"cancel"`
	RetryStage    bool   `json:"retry_stage"`
	RetryTask     bool   `json:"retry_task"`
	RequestReport bool   `json:"request_report"`
}

// ForRun 按六轴 + attempt 事实 + 报告状态判定(全部只读)。
func (s *AllowedActionsService) ForRun(ctx context.Context, tenantID, runID string) (*RunAllowedActions, error) {
	run, err := s.repo.GetRun(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	actions := &RunAllowedActions{RunID: run.RunID, State: run.State}
	if run.State == "CANCEL_REQUESTED" {
		return actions, nil
	}
	if !repository.IsTerminalRunState(run.State) {
		actions.Cancel = true
		// retry_stage:仅当存在 FAILED attempt(节点级重试输入)
		stages, err := s.repo.ListRunStages(ctx, tenantID, runID)
		if err != nil {
			return nil, err
		}
		for _, st := range stages {
			if st.State == "FAILED" {
				actions.RetryStage = true
				break
			}
		}
		return actions, nil
	}
	// 终态:整 Run 重试(同 task 新 run)始终允许(SUCCEEDED/PARTIALLY_SUCCEEDED/FAILED/CANCELLED);
	// 报告请求:冻结摘要存在且无在途/已完成报告行。
	actions.RetryTask = true
	ref, err := s.repo.GetRunSummaryHash(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	if ref.SummaryExists && ref.SummarySHA256 != "" {
		switch run.ReportState {
		case "QUEUED", "GENERATING", "VERIFYING", "AVAILABLE":
			// 已有在途或已完成报告:重复请求由幂等回放承载,UI 不重复暴露
		default: // NOT_REQUESTED / FAILED / CANCELLED
			actions.RequestReport = true
		}
	}
	return actions, nil
}

// DefinitionAllowedActions 任务定义动作集(activate/suspend,If-Match revision)。
type DefinitionAllowedActions struct {
	TaskDefinitionID string `json:"task_definition_id"`
	State            string `json:"state"`
	Revision         int64  `json:"revision"`
	Activate         bool   `json:"activate"`
	Suspend          bool   `json:"suspend"`
}

// ForDefinition 按定义状态判定(activate:DRAFT;suspend:ACTIVE)。
func (s *AllowedActionsService) ForDefinition(ctx context.Context, tenantID, defID string) (*DefinitionAllowedActions, error) {
	d, err := s.repo.GetTaskDefinitionDetail(ctx, tenantID, defID)
	if err != nil {
		return nil, err
	}
	return &DefinitionAllowedActions{
		TaskDefinitionID: d.ID,
		State:            d.State,
		Revision:         d.Revision,
		Activate:         d.State == "DRAFT",
		Suspend:          d.State == "ACTIVE",
	}, nil
}

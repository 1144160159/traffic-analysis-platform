// Package service Stage retry 服务(§76.47.3):Run 非终态 + FAILED attempt +
// DEDICATED_OPERATION(可重放输入)→ 新 attempt;SHARED_STREAM 无 replay 输入
// 返回 STAGE_RETRY_UNSUPPORTED 并引导整 Run retry。
package service

import (
	"context"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// MaxStageRetryAttempts 单节点最大 attempt 数(预算上限;超出要求整 Run retry)。
const MaxStageRetryAttempts = 3

// RetryService 节点级重试权威。
type RetryService struct {
	repo *repository.Repo
}

func NewRetryService(repo *repository.Repo) *RetryService { return &RetryService{repo: repo} }

// RetryStage 节点级重试:事务内校验 + 新 attempt + 审计。
func (s *RetryService) RetryStage(ctx context.Context, tenantID, runID, executionNodeID, actor string) (*repository.RetryStageResult, error) {
	if tenantID == "" || runID == "" || executionNodeID == "" {
		return nil, fmt.Errorf("tenant_id, run_id and execution_node_id are required")
	}
	return s.repo.RetryStageAtomic(ctx, tenantID, runID, executionNodeID, MaxStageRetryAttempts, actor)
}

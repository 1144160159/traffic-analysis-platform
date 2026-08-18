package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// StageGateLoop 阶段闸门:S1 SUCCEEDED 后按 DAG 逐级把 S2/S3/S4 尝试置 RUNNING
// (共享流阶段共用 run 级 fencing token),并把更新后的 RunSubscription(带 fence)
// 重新入 outbox,由中继发布 → 路由器注入 envelope → 执行器回执回显同一 token。
type StageGateLoop struct {
	repo   *repository.Repo
	logger *zap.Logger
}

// NewStageGateLoop 构造。
func NewStageGateLoop(repo *repository.Repo, logger *zap.Logger) *StageGateLoop {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &StageGateLoop{repo: repo, logger: logger}
}

// GateOnce 开闸一个阶段批次(同一 run 同阶段共享一个 run 级 fencing token;
// 订阅→envelope→回执回显链路保证 token 一致)。
func (g *StageGateLoop) GateOnce(ctx context.Context) (gated bool, err error) {
	batch, err := g.repo.NextGatingBatch(ctx)
	if err != nil {
		return false, fmt.Errorf("next gating batch: %w", err)
	}
	if batch == nil || len(batch.Attempts) == 0 {
		return false, nil
	}
	// 批内共享一个 token:新 run 用物化时预分配的流水线 token;旧 run(物化时
	// 未预分配)兜底批内统一生成一个,保证同批各节点 token 一致(回执回显同值)。
	token := ""
	for _, cand := range batch.Attempts {
		if cand.FencingToken != "" {
			token = cand.FencingToken
			break
		}
	}
	if token == "" {
		token = repository.NewFencingToken()
	}
	for _, cand := range batch.Attempts {
		ok, err := g.repo.MarkAttemptRunningAtomic(ctx, cand.TenantID, cand.AttemptID, token)
		if err != nil {
			return false, fmt.Errorf("mark running: %w", err)
		}
		if !ok {
			return false, fmt.Errorf("gating CAS lost")
		}
		g.logger.Info("stage gated",
			zap.String("run_id", cand.RunID), zap.String("node", cand.ExecutionNodeID),
			zap.String("phase", cand.BusinessPhaseID), zap.String("token", token[:8]))
	}

	return true, nil
}

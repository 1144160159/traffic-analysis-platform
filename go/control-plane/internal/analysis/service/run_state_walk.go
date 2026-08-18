// Package service Run 状态机行走接入(对齐方案 §7.2):
// ACCEPTED --PLAN_READY--> PREPARING --PLAN_READY--> QUEUED --LEASE_ACQUIRED--> RUNNING
// --ALL_NODES_TERMINAL--> FINALIZING --CLOSURE_COMMITTED--> 终态(终态由 EvaluateRunClosure 覆写)。
//
// 事实驱动 Sync:每次调用查询当前事实(订阅已广播/准入已消费/业务节点已领取/
// 全业务节点终态)并按 CanAdvance+ValidateRunTransition 循环推进到该事实允许的
// 最大状态。调用点(订阅发布后、S1 领取后、终局前)任意先后均收敛——单步钩子
// 在"派发先于订阅广播"的合法交错下会卡在 PREPARING,故统一为幂等 Sync。
package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// RunStateWalker 状态机行走器(事实驱动、幂等;非终态才可推进)。
type RunStateWalker struct {
	repo   *repository.Repo
	logger *zap.Logger
	now    func() time.Time
}

func NewRunStateWalker(repo *repository.Repo, logger *zap.Logger) *RunStateWalker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RunStateWalker{repo: repo, logger: logger, now: time.Now}
}

// RunStateFacts 行走事实(由 Sync 从权威仓储查询)。
type RunStateFacts struct {
	PlanReady             bool // 订阅已广播(PlanReady 事实代理)
	AdmissionConsumed     bool // 准入预留已 CONSUMED
	HasStartedBusinessNode bool // 已有业务节点 DISPATCHED/RUNNING/终态(lease 事实代理)
	AllBusinessTerminal   bool // S1-S4 全终态
}

// GatherFacts 查询行走事实(失败时保守返回 false,由后续调用点重试)。
func (w *RunStateWalker) GatherFacts(ctx context.Context, runID string) RunStateFacts {
	f := RunStateFacts{}
	if ok, err := w.repo.HasPublishedRunEvent(ctx, runID); err == nil {
		f.PlanReady = ok
	}
	if s, err := w.repo.ReservationState(ctx, runID); err == nil {
		f.AdmissionConsumed = s == "CONSUMED"
	}
	if ok, err := w.repo.HasStartedBusinessNode(ctx, runID); err == nil {
		f.HasStartedBusinessNode = ok
	}
	if ok, err := w.repo.AllBusinessNodesTerminal(ctx, runID); err == nil {
		f.AllBusinessTerminal = ok
	}
	return f
}

// Sync 按当前事实推进 run 状态到合法最大值(幂等;CAS 失败即停)。
// tenantID 可空(run_id 为唯一键)。
func (w *RunStateWalker) Sync(ctx context.Context, tenantID, runID string) {
	facts := w.GatherFacts(ctx, runID)
	for i := 0; i < 6; i++ {
		current, err := w.repo.GetRunState(ctx, tenantID, runID)
		if err != nil {
			w.logger.Warn("read run state failed", zap.String("run_id", runID), zap.Error(err))
			return
		}
		events := state.CanAdvance(state.RunState(current), state.RunPreconditions{
			PlanReady:        facts.PlanReady,
			AdmissionValid:   facts.AdmissionConsumed,
			HasNodeLease:     facts.HasStartedBusinessNode,
			AllNodesTerminal: facts.AllBusinessTerminal,
		})
		if len(events) == 0 {
			return
		}
		event := events[0] // 进度事件优先于 EvCancelRequested(CanAdvance 恒置尾)
		if event == state.EvCancelRequested {
			return
		}
		toState := transitionTarget(state.RunState(current), event)
		if toState == "" {
			return
		}
		to := string(toState)
		if ok, want := state.ValidateRunTransition(state.RunState(current), event, toState); !ok || want != toState {
			w.logger.Warn("run state transition rejected (fail-closed)",
				zap.String("run_id", runID), zap.String("from", current), zap.String("to", to), zap.String("event", string(event)))
			return
		}
		moved, err := w.repo.AdvanceRunStateAtomic(ctx, tenantID, runID, []string{current}, to)
		if err != nil {
			w.logger.Warn("advance run state failed", zap.String("run_id", runID), zap.Error(err))
			return
		}
		if !moved {
			return
		}
		w.logger.Info("run state advanced", zap.String("run_id", runID),
			zap.String("from", current), zap.String("to", to), zap.String("event", string(event)))
	}
}

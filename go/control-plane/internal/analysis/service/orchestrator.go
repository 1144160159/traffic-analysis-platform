package service

import (
	"context"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// OrchestratorInputs 编排事实输入(由 repository 事务加载;本服务做确定性选择,不裁决终态)。
type OrchestratorInputs struct {
	RunState             state.RunState
	PlanReady            bool // required consumer exact-set 齐
	AdmissionValid       bool
	HasNodeLease         bool
	SubscriptionActive   bool // PREPARE→ACTIVE 已完成
	ReservationConsumed  bool
	Nodes                []NodeFact
	RunPreconditions     state.RunPreconditions
}

// NodeFact 执行节点事实。
type NodeFact struct {
	ExecutionNodeID  string
	BusinessPhaseID  string
	ProviderMode     string // SHARED_STREAM|DEDICATED_OPERATION|AUTHORITY_LOCAL
	ActivationMode   string // PIPELINED_STREAM|AFTER_UPSTREAM_CLOSE|AUTHORITY_LOCAL
	Required         bool
	State            state.StageAttemptState
	UpstreamManifest bool // AFTER_UPSTREAM_CLOSE 节点:上游冻结 manifest 已存在
}

// WaitReason 0 候选时的明确等待原因(对齐方案 §7.3/§1.5)。
type WaitReason string

const (
	WaitPlanReady           WaitReason = "WAIT_PLAN_READY"
	WaitWindowStart         WaitReason = "WAIT_WINDOW_START"
	WaitWatermark           WaitReason = "WAIT_WATERMARK"
	WaitProviderAck         WaitReason = "WAIT_PROVIDER_ACK"
	WaitCapacity            WaitReason = "WAIT_CAPACITY"
	ReadyToReconcile        WaitReason = "READY_TO_RECONCILE"
	ReadyToFinalize         WaitReason = "READY_TO_FINALIZE"
	UnrecoverableFailure    WaitReason = "UNRECOVERABLE_FAILURE"
)

// AdvanceDecision 编排决策。
type AdvanceDecision struct {
	Dispatchables []string   // 可派发的 execution_node_id(0..N)
	Wait          WaitReason // 0 候选时的原因
	Transition    *RunTransition
}

// RunTransition 状态推进建议(权威事务执行)。
type RunTransition struct {
	From  state.RunState
	To    state.RunState
	Event state.RunEvent
}

// Orchestrator 编排器:确定性选择可派发节点;不创建 Flink Job(共享流节点只做逻辑准入)。
type Orchestrator struct{}

func NewOrchestrator() *Orchestrator { return &Orchestrator{} }

// Advance 确定性 0..N 派发(卷A §1.5)。纯函数(输入即事实快照)。
func (o *Orchestrator) Advance(_ context.Context, in OrchestratorInputs) (*AdvanceDecision, error) {
	decision := &AdvanceDecision{}

	// A. 状态推进建议(权威事务负责 CAS;此处只给建议)
	if evs := state.CanAdvance(in.RunState, in.RunPreconditions); len(evs) > 0 {
		for _, ev := range evs {
			if ev == state.EvCancelRequested {
				continue
			}
			ok, to := state.ValidateRunTransition(in.RunState, ev, transitionTarget(in.RunState, ev))
			if ok {
				decision.Transition = &RunTransition{From: in.RunState, To: to, Event: ev}
				break
			}
		}
	}

	// B. PlanReady 屏障(阶段 1 前,不可删除)
	if !in.PlanReady {
		decision.Wait = WaitPlanReady
		return decision, nil
	}

	// C. 选择可派发节点
	for _, n := range in.Nodes {
		if !n.Required || state.IsTerminalStage(n.State) {
			continue
		}
		if n.State == state.StageDispatched || n.State == state.StageRunning {
			continue // 已有活跃 attempt
		}
		switch n.ActivationMode {
		case "PIPELINED_STREAM":
			if in.SubscriptionActive && in.ReservationConsumed {
				decision.Dispatchables = append(decision.Dispatchables, n.ExecutionNodeID)
			}
		case "AFTER_UPSTREAM_CLOSE":
			if n.UpstreamManifest {
				decision.Dispatchables = append(decision.Dispatchables, n.ExecutionNodeID)
			}
		case "AUTHORITY_LOCAL":
			// Reconcile/Finalizer 由谓词驱动
			if n.ExecutionNodeID == "RECONCILE" && allBusinessTerminal(in.Nodes) {
				decision.Dispatchables = append(decision.Dispatchables, n.ExecutionNodeID)
			}
			if n.ExecutionNodeID == "MACHINE_FINALIZATION" && in.RunPreconditions.AllNodesTerminal {
				decision.Dispatchables = append(decision.Dispatchables, n.ExecutionNodeID)
			}
		}
	}

	if len(decision.Dispatchables) == 0 {
		decision.Wait = firstWaitReason(in)
	}
	return decision, nil
}

// transitionTarget 事件→目标态(与 state 包一致;最终 CAS 由权威事务校验)。
func transitionTarget(from state.RunState, ev state.RunEvent) state.RunState {
	switch ev {
	case state.EvPlanReady:
		if from == state.RunAccepted {
			return state.RunPreparing
		}
		return state.RunQueued
	case state.EvLeaseAcquired:
		return state.RunRunning
	case state.EvAllNodesTerminal:
		return state.RunFinalizing
	case state.EvClosureCommitted:
		return state.RunSucceeded
	case state.EvCancelClosure:
		return state.RunCancelled
	}
	return ""
}

// allBusinessTerminal 全部业务执行节点(S1-S4)terminal/skip/cancel。
func allBusinessTerminal(nodes []NodeFact) bool {
	has := false
	for _, n := range nodes {
		if n.BusinessPhaseID == "S5" {
			continue // authority-local 不算业务节点
		}
		if !n.Required {
			continue
		}
		has = true
		if !state.IsTerminalStage(n.State) {
			return false
		}
	}
	return has
}

// firstWaitReason 0 候选时的原因优先级。
func firstWaitReason(in OrchestratorInputs) WaitReason {
	if !in.AdmissionValid {
		return WaitCapacity
	}
	if !in.SubscriptionActive {
		return WaitWindowStart
	}
	if in.RunPreconditions.AllNodesTerminal && !in.RunPreconditions.ClosureCommitted {
		return ReadyToFinalize
	}
	return WaitProviderAck
}

// ReconcileReport 对账结论(纯判定;持久化由权威事务)。
type ReconcileReport struct {
	OK            bool
	Differences   []string
}

// ReconcileFacts 对账输入。
type ReconcileFacts struct {
	AttemptGap        bool
	CountMismatches   []string // 已解释差异说明
	WatermarkMismatch bool
	MissingTerminal   []string // 未终态 required 节点
}

// Reconcile 对账纯判定(ATC-REC-001 的判定核)。
func Reconcile(f ReconcileFacts) (*ReconcileReport, error) {
	report := &ReconcileReport{OK: true}
	if f.AttemptGap {
		report.Differences = append(report.Differences, "attempt_gap")
	}
	if f.WatermarkMismatch {
		report.Differences = append(report.Differences, "watermark_mismatch")
	}
	if len(f.MissingTerminal) > 0 {
		report.Differences = append(report.Differences, fmt.Sprintf("missing_terminal:%v", f.MissingTerminal))
	}
	// count 差异必须"无未解释差异"——这里只允许白名单式已解释差异,任何条目都算未解释
	if len(f.CountMismatches) > 0 {
		report.Differences = append(report.Differences, fmt.Sprintf("count_mismatch:%v", f.CountMismatches))
	}
	if len(report.Differences) > 0 {
		report.OK = false
		return report, newAnalysisError(contract.ErrCodeInvalidTransition, "reconcile differences: "+fmt.Sprint(report.Differences))
	}
	return report, nil
}

// Package state 调度域纯函数状态机:合法迁移穷举、终态真值表。
// 纯函数约束(ENG-FUNC-001):不读 clock/random/env/global,依赖全参数注入。
package state

// RunState 运行状态(与 proto RunState 一致,仓库内用 string 表达以便 PG 存 JSONB 友好)。
type RunState string

const (
	RunAccepted         RunState = "ACCEPTED"
	RunPreparing        RunState = "PREPARING"
	RunQueued           RunState = "QUEUED"
	RunRunning          RunState = "RUNNING"
	RunFinalizing       RunState = "FINALIZING"
	RunSucceeded        RunState = "SUCCEEDED"
	RunPartiallySucceeded RunState = "PARTIALLY_SUCCEEDED"
	RunFailed           RunState = "FAILED"
	RunCancelRequested  RunState = "CANCEL_REQUESTED"
	RunCancelled        RunState = "CANCELLED"
)

// RunEvent 触发迁移的事件(权威事实)。
type RunEvent string

const (
	EvPlanReady       RunEvent = "PLAN_READY"
	EvLeaseAcquired   RunEvent = "LEASE_ACQUIRED"
	EvNodesDispatched RunEvent = "NODES_DISPATCHED"
	EvAllNodesTerminal RunEvent = "ALL_NODES_TERMINAL"
	EvClosureCommitted RunEvent = "CLOSURE_COMMITTED"
	EvCancelRequested RunEvent = "CANCEL_REQUESTED"
	EvCancelClosure   RunEvent = "CANCEL_CLOSURE"
)

// TerminalRunStates 终态集合(不回退)。
var TerminalRunStates = map[RunState]bool{
	RunSucceeded: true, RunPartiallySucceeded: true, RunFailed: true, RunCancelled: true,
}

// IsTerminal 是否终态。
func IsTerminal(s RunState) bool { return TerminalRunStates[s] }

// ValidateRunTransition 穷尽编码合法迁移(对齐方案 §7.2)。返回错误码语义。
// allowed 表:from → (event → to)。未列出的迁移一律拒绝(fail closed)。
var runTransitions = map[RunState]map[RunEvent]RunState{
	RunAccepted: {
		EvPlanReady: RunPreparing, // PLAN_READY 后才能 PREPARING→QUEUED(见下)
	},
	RunPreparing: {
		EvPlanReady:     RunQueued,
		EvCancelRequested: RunCancelRequested,
	},
	RunQueued: {
		EvLeaseAcquired:   RunRunning,
		EvCancelRequested: RunCancelRequested,
	},
	RunRunning: {
		EvAllNodesTerminal: RunFinalizing,
		EvCancelRequested:  RunCancelRequested,
	},
	RunFinalizing: {
		EvClosureCommitted: RunSucceeded, // 具体终态由 EvaluateRunClosure 决定;此处事件化后由调用方覆写
		EvCancelRequested:  RunCancelRequested,
	},
	RunCancelRequested: {
		EvCancelClosure: RunCancelled,
	},
}

// ValidateRunTransition 校验 from --event--> to 是否合法。
// 返回 (ok, expected)。ok=false 时 expected 为空,调用方回 ANALYSIS_INVALID_TRANSITION。
func ValidateRunTransition(from RunState, event RunEvent, to RunState) (bool, RunState) {
	if !IsTerminal(from) {
		if m, ok := runTransitions[from]; ok {
			if want, ok2 := m[event]; ok2 && want == to {
				return true, want
			}
		}
	}
	return false, ""
}

// CancelRequestAllowed 哪些非终态允许进入 CANCEL_REQUESTED(对齐方案 §7.2)。
func CancelRequestAllowed(from RunState) bool {
	switch from {
	case RunAccepted, RunPreparing, RunQueued, RunRunning, RunFinalizing:
		return true
	}
	return false
}

// Preconditions 关键闸门(对齐方案 §7.2):
// - PLAN_READY + 有效 AdmissionReservation 才能 PREPARING→QUEUED
// - 有效节点 lease 才能 QUEUED→RUNNING
// - 全部业务执行节点 terminal 才能 RUNNING→FINALIZING
type RunPreconditions struct {
	PlanReady           bool // PLAN_READY 已成立(required consumer exact-set 齐)
	AdmissionValid      bool // 有效 AdmissionReservation(CONSUMED 且未过期)
	HasNodeLease        bool // 至少一个业务执行节点持有有效 lease
	AllNodesTerminal    bool // 全部业务执行节点 terminal/skip/cancel
	ClosureCommitted    bool // 三件套同事务已提交
	CancelExactSetDrained bool // 取消 exact-set 已 terminal/drained/fenced
}

// CanAdvance 按 preconditions 返回可执行的事件(不产生副作用,由权威事务消费)。
func CanAdvance(from RunState, p RunPreconditions) []RunEvent {
	if IsTerminal(from) {
		return nil
	}
	events := []RunEvent{}
	switch from {
	case RunAccepted:
		if p.PlanReady {
			events = append(events, EvPlanReady)
		}
	case RunPreparing:
		if p.PlanReady && p.AdmissionValid {
			events = append(events, EvPlanReady) // PREPARING→QUEUED
		}
	case RunQueued:
		if p.HasNodeLease {
			events = append(events, EvLeaseAcquired)
		}
	case RunRunning:
		if p.AllNodesTerminal {
			events = append(events, EvAllNodesTerminal)
		}
	case RunFinalizing:
		if p.ClosureCommitted {
			events = append(events, EvClosureCommitted)
		}
	case RunCancelRequested:
		if p.CancelExactSetDrained {
			events = append(events, EvCancelClosure)
		}
	}
	if CancelRequestAllowed(from) {
		events = append(events, EvCancelRequested)
	}
	return events
}

package state

// StageAttemptState 执行节点 attempt 状态。
type StageAttemptState string

const (
	StagePending         StageAttemptState = "PENDING"
	StageDispatched      StageAttemptState = "DISPATCHED"
	StageRunning         StageAttemptState = "RUNNING"
	StageSucceeded       StageAttemptState = "SUCCEEDED"
	StagePartial         StageAttemptState = "PARTIAL"
	StageFailed          StageAttemptState = "FAILED"
	StageCancelRequested StageAttemptState = "CANCEL_REQUESTED"
	StageCancelled       StageAttemptState = "CANCELLED"
	StageSkipped         StageAttemptState = "SKIPPED"
)

// TerminalStageStates attempt 终态。
var TerminalStageStates = map[StageAttemptState]bool{
	StageSucceeded: true, StagePartial: true, StageFailed: true, StageCancelled: true, StageSkipped: true,
}

// IsTerminalStage 是否终态。
func IsTerminalStage(s StageAttemptState) bool { return TerminalStageStates[s] }

// SkipReason 未派发 SKIPPED 的合法原因(对齐方案 §7.3)。
const (
	SkipOptionalPredicateFalse  = "OPTIONAL_PREDICATE_FALSE"
	SkipBlockedByUpstreamFailure = "BLOCKED_BY_UPSTREAM_FAILURE"
	SkipCancelledBeforeDispatch = "CANCELLED_BEFORE_DISPATCH"
	SkipNotApplicableByPlan     = "NOT_APPLICABLE_BY_PLAN"
)

// ValidSkipReasons 合法 skip 原因集合。
var ValidSkipReasons = map[string]bool{
	SkipOptionalPredicateFalse: true, SkipBlockedByUpstreamFailure: true,
	SkipCancelledBeforeDispatch: true, SkipNotApplicableByPlan: true,
}

// StageEvent attempt 事件。
type StageEvent string

const (
	StEvDispatch StageEvent = "DISPATCH"
	StEvStart    StageEvent = "START"
	StEvComplete StageEvent = "COMPLETE" // 携带 to ∈ {SUCCEEDED,PARTIAL,FAILED}
	StEvCancelReq StageEvent = "CANCEL_REQUESTED"
	StEvCancel   StageEvent = "CANCEL"
	StEvSkip     StageEvent = "SKIP" // 未派发路径
)

var stageTransitions = map[StageAttemptState]map[StageEvent][]StageAttemptState{
	StagePending: {
		StEvDispatch: {StageDispatched},
		StEvSkip:     {StageSkipped},
	},
	StageDispatched: {
		StEvStart:     {StageRunning},
		StEvCancelReq: {StageCancelRequested},
	},
	StageRunning: {
		StEvComplete:  {StageSucceeded, StagePartial, StageFailed},
		StEvCancelReq: {StageCancelRequested},
	},
	StageCancelRequested: {
		StEvCancel: {StageCancelled},
	},
}

// ValidateStageAttemptTransition 校验 attempt 迁移;拒绝 terminal 回退。
// attempt gap 与旧 fencing token 由权威事务校验(不在纯函数内)。
func ValidateStageAttemptTransition(from StageAttemptState, event StageEvent, to StageAttemptState) bool {
	if IsTerminalStage(from) {
		return false
	}
	if tos, ok := stageTransitions[from]; ok {
		if allowed, ok2 := tos[event]; ok2 {
			for _, t := range allowed {
				if t == to {
					return true
				}
			}
		}
	}
	return false
}

// EffectiveBusinessPhaseState 由节点 exact-set 确定性投影业务阶段状态(对齐方案 §7.3)。
// required 节点失败/非预期 SKIP 必须使 Run 进入 FINALIZING 并最终 FAILED;
// PARTIAL 只有冻结 completion_policy 明确允许时才参与 PARTIALLY_SUCCEEDED。
func EffectiveBusinessPhaseState(nodes []struct {
	Required bool
	State    StageAttemptState
	SkipReason string
}, allowPartial bool) StageAttemptState {
	hasFailed := false
	hasPartial := false
	hasSucceeded := false
	for _, n := range nodes {
		switch {
		case n.State == StageFailed:
			hasFailed = true
		case n.State == StageSkipped && n.SkipReason != SkipOptionalPredicateFalse && n.SkipReason != SkipNotApplicableByPlan:
			hasFailed = true // required 非预期 SKIP 视为失败
		case n.State == StagePartial:
			hasPartial = true
		case n.State == StageSucceeded:
			hasSucceeded = true
		}
	}
	switch {
	case hasFailed:
		return StageFailed
	case hasPartial && !allowPartial:
		return StageFailed
	case hasPartial:
		return StagePartial
	case hasSucceeded:
		return StageSucceeded
	default:
		return StageRunning
	}
}

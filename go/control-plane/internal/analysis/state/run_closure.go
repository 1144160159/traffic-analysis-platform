package state

// EvaluateRunClosure 纯函数真值表(对齐方案 §7.6)。
// 优先级从高到低;低优先级不得覆盖高优先级。无 IO/clock/random。

// FindingConclusion / RiskSeverity / Completeness / IntegrityState 用 string 表达(与 proto 枚举同名)。
const (
	ConclusionThreatFound       = "THREAT_FOUND"
	ConclusionNoThreatObserved  = "NO_THREAT_OBSERVED"
	ConclusionInconclusive      = "INCONCLUSIVE"
	ConclusionNoData            = "NO_DATA"
	ConclusionNotEvaluated      = "NOT_EVALUATED"

	SeverityCritical = "CRITICAL"
	SeverityHigh     = "HIGH"
	SeverityMedium   = "MEDIUM"
	SeverityLow      = "LOW"
	SeverityNone     = "NONE"
	SeverityUnknown  = "UNKNOWN"

	CompleteComplete   = "COMPLETE"
	CompletePartial    = "PARTIAL"
	CompleteIncomplete = "INCOMPLETE"
	CompleteUnknown    = "UNKNOWN"

	IntegrityVerified   = "VERIFIED"
	IntegrityUnverified = "UNVERIFIED"
	IntegrityFailed     = "FAILED"
)

// ClosureFacts 终态判定输入(冻结事实)。
type ClosureFacts struct {
	IdentityIntegrityOK     bool // 身份/hash 一致性
	FenceCountIntegrityOK   bool // fence/count 无未解释冲突
	CancelCASWon            bool // cancel CAS 先胜且 exact-set 已 drain/fence
	RequiredNodeFailed      bool // required 节点失败/阻断/重试耗尽
	DeadlineReached         bool
	PartialAllowed          bool // completion policy 允许部分
	PartialThresholdMet     bool
	ZeroInputProven         bool // source fence 证明 accepted input=0
	ZeroInputPolicy         string // ALLOW_NO_DATA|FAIL_EMPTY
	AllRequiredSucceeded    bool
	HasTrustedPositive      bool
	AllInputsExplicitNegative bool // 每个 accepted input×required detector 明确阴性
	EvidenceSufficient      bool // 工作完整但证据不足→INCONCLUSIVE 分支
}

// ClosureDecision 五轴终态。
type ClosureDecision struct {
	RunState          RunState
	Completeness      string
	IntegrityState    string
	FindingConclusion string
	RiskSeverity      string
}

// EvaluateRunClosure 逐行实现真值表。
func EvaluateRunClosure(f ClosureFacts) ClosureDecision {
	d := ClosureDecision{}

	// 1. 身份/hash/fence/计数完整性失败(最高优先级)
	if !f.IdentityIntegrityOK || !f.FenceCountIntegrityOK {
		d.RunState = RunFailed
		d.Completeness = CompleteIncomplete
		d.IntegrityState = IntegrityFailed
		d.FindingConclusion = ConclusionNotEvaluated
		return d
	}
	// 2. cancel CAS 先胜
	if f.CancelCASWon {
		d.RunState = RunCancelled
		d.Completeness = CompletePartial
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionNotEvaluated
		if f.HasTrustedPositive {
			d.FindingConclusion = ConclusionThreatFound
		}
		return d
	}
	// 3. required node 失败/阻断
	if f.RequiredNodeFailed {
		d.RunState = RunFailed
		d.Completeness = CompleteIncomplete
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionNotEvaluated
		if f.HasTrustedPositive {
			d.FindingConclusion = ConclusionThreatFound
		}
		return d
	}
	// 4/5. deadline
	if f.DeadlineReached {
		if f.PartialAllowed && f.PartialThresholdMet {
			d.RunState = RunPartiallySucceeded
			d.Completeness = CompletePartial
			d.IntegrityState = IntegrityVerified
			d.FindingConclusion = ConclusionInconclusive
			if f.HasTrustedPositive {
				d.FindingConclusion = ConclusionThreatFound
			}
			return d
		}
		d.RunState = RunFailed
		d.Completeness = CompleteIncomplete
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionNotEvaluated
		if f.HasTrustedPositive {
			d.FindingConclusion = ConclusionThreatFound
		}
		return d
	}
	// 6. zero input
	if f.ZeroInputProven {
		d.RunState = RunSucceeded
		d.Completeness = CompleteComplete
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionNoData
		if f.ZeroInputPolicy == "FAIL_EMPTY" {
			d.RunState = RunFailed
			d.Completeness = CompleteIncomplete
		}
		return d
	}
	// 7. 全部成功 + 任一可信阳性
	if f.AllRequiredSucceeded && f.HasTrustedPositive {
		d.RunState = RunSucceeded
		d.Completeness = CompleteComplete
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionThreatFound
		d.RiskSeverity = SeverityUnknown // 由发现聚合填入,本函数不判定
		return d
	}
	// 8. 全部成功 + 每输入×required detector 明确阴性
	if f.AllRequiredSucceeded && f.AllInputsExplicitNegative {
		d.RunState = RunSucceeded
		d.Completeness = CompleteComplete
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionNoThreatObserved
		return d
	}
	// 9. 全部工作完整但证据不足
	if f.AllRequiredSucceeded && !f.EvidenceSufficient {
		d.RunState = RunSucceeded
		d.Completeness = CompleteComplete
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionInconclusive
		return d
	}
	// 兜底:允许且已解释的丢失/ERROR 达到部分阈值
	if f.PartialAllowed && f.PartialThresholdMet {
		d.RunState = RunPartiallySucceeded
		d.Completeness = CompletePartial
		d.IntegrityState = IntegrityVerified
		d.FindingConclusion = ConclusionInconclusive
		if f.HasTrustedPositive {
			d.FindingConclusion = ConclusionThreatFound
		}
		return d
	}
	// 其余情况不产生终态(仍可运行/需更多事实)
	d.RunState = RunRunning
	d.Completeness = CompleteUnknown
	d.IntegrityState = IntegrityUnverified
	d.FindingConclusion = ConclusionNotEvaluated
	return d
}

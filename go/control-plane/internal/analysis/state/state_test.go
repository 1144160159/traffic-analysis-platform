package state

import "testing"

func TestRunStateTransitionExhaustive(t *testing.T) {
	cases := []struct {
		from  RunState
		event RunEvent
		to    RunState
		ok    bool
	}{
		{RunAccepted, EvPlanReady, RunPreparing, true},
		{RunPreparing, EvPlanReady, RunQueued, true},
		{RunQueued, EvLeaseAcquired, RunRunning, true},
		{RunRunning, EvAllNodesTerminal, RunFinalizing, true},
		{RunCancelRequested, EvCancelClosure, RunCancelled, true},
		// 非法迁移
		{RunAccepted, EvLeaseAcquired, RunRunning, false}, // 未 PLAN_READY 不得直接 RUNNING
		{RunQueued, EvAllNodesTerminal, RunFinalizing, false},
		{RunSucceeded, EvPlanReady, RunPreparing, false}, // 终态不回退
		{RunRunning, EvClosureCommitted, RunSucceeded, false}, // 需先 FINALIZING
	}
	for _, c := range cases {
		ok, _ := ValidateRunTransition(c.from, c.event, c.to)
		if ok != c.ok {
			t.Errorf("transition %v --%v--> %v: got ok=%v want %v", c.from, c.event, c.to, ok, c.ok)
		}
	}
}

func TestCancelRequestAllowed(t *testing.T) {
	for _, s := range []RunState{RunAccepted, RunPreparing, RunQueued, RunRunning, RunFinalizing} {
		if !CancelRequestAllowed(s) {
			t.Errorf("%v should allow cancel request", s)
		}
	}
	for _, s := range []RunState{RunSucceeded, RunFailed, RunCancelled, RunPartiallySucceeded} {
		if CancelRequestAllowed(s) {
			t.Errorf("%v must not allow cancel request", s)
		}
	}
}

func TestCanAdvanceGates(t *testing.T) {
	// PLAN_READY+Admission 才能 PREPARING→QUEUED
	if evs := CanAdvance(RunPreparing, RunPreconditions{PlanReady: true, AdmissionValid: false}); containsEvent(evs, EvPlanReady) {
		t.Errorf("PREPARING without valid admission must not advance")
	}
	if evs := CanAdvance(RunPreparing, RunPreconditions{PlanReady: true, AdmissionValid: true}); !containsEvent(evs, EvPlanReady) {
		t.Errorf("PREPARING with plan-ready+admission should advance")
	}
	// lease 才能 QUEUED→RUNNING
	if evs := CanAdvance(RunQueued, RunPreconditions{HasNodeLease: false}); containsEvent(evs, EvLeaseAcquired) {
		t.Errorf("QUEUED without lease must not advance")
	}
}

func containsEvent(evs []RunEvent, e RunEvent) bool {
	for _, v := range evs {
		if v == e {
			return true
		}
	}
	return false
}

func TestStageAttemptTransition(t *testing.T) {
	if !ValidateStageAttemptTransition(StagePending, StEvDispatch, StageDispatched) {
		t.Errorf("PENDING --DISPATCH--> DISPATCHED must be valid")
	}
	if !ValidateStageAttemptTransition(StagePending, StEvSkip, StageSkipped) {
		t.Errorf("PENDING --SKIP--> SKIPPED must be valid")
	}
	if !ValidateStageAttemptTransition(StageRunning, StEvComplete, StageFailed) {
		t.Errorf("RUNNING --COMPLETE--> FAILED must be valid")
	}
	if ValidateStageAttemptTransition(StageSucceeded, StEvComplete, StageFailed) {
		t.Errorf("terminal must not regress")
	}
	if ValidateStageAttemptTransition(StagePending, StEvComplete, StageSucceeded) {
		t.Errorf("PENDING cannot complete directly")
	}
}

func TestEvaluateRunClosureTruthTable(t *testing.T) {
	cases := []struct {
		name string
		f    ClosureFacts
		want ClosureDecision
	}{
		{
			"integrity conflict -> FAILED/NOT_EVALUATED/FAILED",
			ClosureFacts{IdentityIntegrityOK: false, FenceCountIntegrityOK: true},
			ClosureDecision{RunState: RunFailed, Completeness: CompleteIncomplete, IntegrityState: IntegrityFailed, FindingConclusion: ConclusionNotEvaluated},
		},
		{
			"cancel CAS won -> CANCELLED",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, CancelCASWon: true},
			ClosureDecision{RunState: RunCancelled, Completeness: CompletePartial, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionNotEvaluated},
		},
		{
			"required node failed -> FAILED",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, RequiredNodeFailed: true},
			ClosureDecision{RunState: RunFailed, Completeness: CompleteIncomplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionNotEvaluated},
		},
		{
			"deadline partial allowed -> PARTIALLY_SUCCEEDED",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, DeadlineReached: true, PartialAllowed: true, PartialThresholdMet: true},
			ClosureDecision{RunState: RunPartiallySucceeded, Completeness: CompletePartial, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionInconclusive},
		},
		{
			"deadline no partial -> FAILED (not negative)",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, DeadlineReached: true, PartialAllowed: false},
			ClosureDecision{RunState: RunFailed, Completeness: CompleteIncomplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionNotEvaluated},
		},
		{
			"zero input ALLOW_NO_DATA -> SUCCEEDED/NO_DATA",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, ZeroInputProven: true, ZeroInputPolicy: "ALLOW_NO_DATA"},
			ClosureDecision{RunState: RunSucceeded, Completeness: CompleteComplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionNoData},
		},
		{
			"zero input FAIL_EMPTY -> FAILED/NO_DATA",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, ZeroInputProven: true, ZeroInputPolicy: "FAIL_EMPTY"},
			ClosureDecision{RunState: RunFailed, Completeness: CompleteIncomplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionNoData},
		},
		{
			"trusted positive -> THREAT_FOUND",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, AllRequiredSucceeded: true, HasTrustedPositive: true},
			ClosureDecision{RunState: RunSucceeded, Completeness: CompleteComplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionThreatFound, RiskSeverity: SeverityUnknown},
		},
		{
			"all explicit negative -> NO_THREAT_OBSERVED",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, AllRequiredSucceeded: true, AllInputsExplicitNegative: true, EvidenceSufficient: true},
			ClosureDecision{RunState: RunSucceeded, Completeness: CompleteComplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionNoThreatObserved},
		},
		{
			"work complete but evidence insufficient -> INCONCLUSIVE",
			ClosureFacts{IdentityIntegrityOK: true, FenceCountIntegrityOK: true, AllRequiredSucceeded: true, EvidenceSufficient: false},
			ClosureDecision{RunState: RunSucceeded, Completeness: CompleteComplete, IntegrityState: IntegrityVerified, FindingConclusion: ConclusionInconclusive},
		},
	}
	for _, c := range cases {
		got := EvaluateRunClosure(c.f)
		if got != c.want {
			t.Errorf("%s:\n got  %+v\n want %+v", c.name, got, c.want)
		}
	}
}

func TestEvaluateRunClosurePriority(t *testing.T) {
	// 高优先级覆盖低优先级:integrity 冲突时即便有可信阳性也 FAILED
	got := EvaluateRunClosure(ClosureFacts{
		IdentityIntegrityOK: false, AllRequiredSucceeded: true, HasTrustedPositive: true,
	})
	if got.RunState != RunFailed || got.FindingConclusion != ConclusionNotEvaluated {
		t.Errorf("priority violation: %+v", got)
	}
}

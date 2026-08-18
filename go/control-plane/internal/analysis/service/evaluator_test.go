package service

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

func testContract() EvaluationContract {
	return EvaluationContract{
		SchemaVersion: "1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		Accuracy: 0.962, DetectionRate: 0.97, FalsePositiveRate: 0.031,
		KnownAttackRecall: 0.97, UnknownRecall: 0.9,
		CILowerAccuracy: 0.951, CIUpperFPR: 0.041,
		StrataComplete: true, SampleCount: 4000,
		GatePassed: true,
	}
}

func TestEvaluationGatePassesAtThresholds(t *testing.T) {
	c := testContract()
	passed, reasons := EvaluationGate(c)
	if !passed {
		t.Fatalf("expected pass, reasons=%v", reasons)
	}
}

func TestEvaluationGateFailsLowAccuracy(t *testing.T) {
	c := testContract()
	c.Accuracy = 0.93
	passed, reasons := EvaluationGate(c)
	if passed || len(reasons) == 0 {
		t.Fatalf("expected fail with accuracy reason, got %v", reasons)
	}
}

func TestEvaluationGateFailsHighFPR(t *testing.T) {
	c := testContract()
	c.FalsePositiveRate = 0.07
	if passed, _ := EvaluationGate(c); passed {
		t.Fatalf("expected fpr failure")
	}
}

func TestEvaluationGateFailsIncompleteStrata(t *testing.T) {
	c := testContract()
	c.StrataComplete = false
	if passed, _ := EvaluationGate(c); passed {
		t.Fatalf("expected strata failure")
	}
}

func TestEvaluationGateFailsInvalidLabels(t *testing.T) {
	c := testContract()
	c.InvalidLabels = 3
	if passed, _ := EvaluationGate(c); passed {
		t.Fatalf("invalid labels must block")
	}
}

func TestEvaluationGateFailsMissingCI(t *testing.T) {
	c := testContract()
	c.CILowerAccuracy = 0
	c.CIUpperFPR = 0
	if passed, reasons := EvaluationGate(c); passed || len(reasons) == 0 {
		t.Fatalf("missing CI must block: %v", reasons)
	}
}

func TestClosureFactsGateFailureMapsToRequiredNodeFailed(t *testing.T) {
	f := ClosureFactsForEvaluation(testContract(), false)
	if !f.RequiredNodeFailed {
		t.Fatalf("gate failure must map to required-node-failed")
	}
	decision := state.EvaluateRunClosure(f)
	if decision.RunState != state.RunFailed {
		t.Fatalf("state=%s want FAILED", decision.RunState)
	}
}

func TestClosureFactsGatePassWithPositive(t *testing.T) {
	f := ClosureFactsForEvaluation(testContract(), true)
	decision := state.EvaluateRunClosure(f)
	if decision.RunState != state.RunSucceeded || decision.FindingConclusion != state.ConclusionThreatFound {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestClosureFactsGatePassAllNegative(t *testing.T) {
	c := testContract()
	c.DetectionRate = 0
	f := ClosureFactsForEvaluation(c, true)
	decision := state.EvaluateRunClosure(f)
	if decision.RunState != state.RunSucceeded || decision.FindingConclusion != state.ConclusionNoThreatObserved {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

type fakeEvalExecutor struct {
	contract *EvaluationContract
	err      error
}

func (f *fakeEvalExecutor) RunEvaluation(context.Context, string, string, string) (*EvaluationContract, error) {
	return f.contract, f.err
}

func TestEvaluationServiceIdentityMismatchRejectedByEvaluator(t *testing.T) {
	// PythonEvaluator 的身份校验由单元场景覆盖;此处验证 service 层透传
	executor := &fakeEvalExecutor{err: nil, contract: nil}
	svc := NewEvaluationService(executor, NewFinalizerService(nil))
	_, err := svc.EvaluateAndFinalize(context.Background(), "tenant-a", "run-1", "spec-1", "/pkg")
	if err == nil {
		t.Fatalf("nil contract must be rejected by finalizer path")
	}
}

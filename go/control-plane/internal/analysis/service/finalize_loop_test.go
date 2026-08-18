package service

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

func s5Fact(id, node, state, token string) repository.ClosureAttemptFact {
	return repository.ClosureAttemptFact{
		ID: id, ExecutionNodeID: node, BusinessPhaseID: "S5", Attempt: 1, State: state, FencingToken: token,
	}
}

func stageFact(node, phase, state, token string) repository.ClosureAttemptFact {
	return repository.ClosureAttemptFact{
		ID: "id-" + node, ExecutionNodeID: node, BusinessPhaseID: phase, Attempt: 1, State: state, FencingToken: token,
	}
}

func receiptFact(node, provider, token, payload string, in, out, err int64, fence string) repository.ClosureReceiptFact {
	return repository.ClosureReceiptFact{
		ExecutionNodeID: node, Provider: provider, FencingToken: token, PayloadHash: payload,
		InputCount: in, OutputCount: out, ErrorCount: err, Fence: []byte(fence),
	}
}

func benignFacts() *repository.RunClosureFactsRow {
	return &repository.RunClosureFactsRow{
		TenantID:            "default",
		RunID:               "run-1",
		RunState:            "FINALIZING",
		ExecutionSpecSHA256: "spec",
		WindowStartMs:       1000,
		WindowEndMs:         2000,
		Attempts: []repository.ClosureAttemptFact{
			stageFact("SOURCE_ACTIVATE", "S1", "SUCCEEDED", "t1"),
			stageFact("SESSIONIZATION", "S2", "SUCCEEDED", "t2"),
			stageFact("FEATURE_EXTRACTION", "S2", "SUCCEEDED", "t2"),
			stageFact("ENCRYPTED_RECOGNIZER", "S3", "SUCCEEDED", "t3"),
			stageFact("RULE_DETECTION", "S4", "SUCCEEDED", "t4"),
			stageFact("BEHAVIOR_DETECTION", "S4", "SUCCEEDED", "t4"),
			stageFact("DETECTION_AGGREGATE", "S4", "SUCCEEDED", "t4"),
		},
		S5Attempts: []repository.ClosureAttemptFact{
			s5Fact("id-r", "RECONCILE", "RUNNING", "t5"),
			s5Fact("id-m", "MACHINE_FINALIZATION", "RUNNING", "t5"),
		},
		Receipts: []repository.ClosureReceiptFact{
			receiptFact("SOURCE_ACTIVATE", "probe-agent", "t1", "", 15000, 0, 0, `{"kind":"source_fence"}`),
			receiptFact("SESSIONIZATION", "flink-run-receipt", "t2", "spec", 6149, 6149, 0, `{"kind":"session_fence"}`),
			receiptFact("FEATURE_EXTRACTION", "flink-feature-receipt", "t2", "spec", 6149, 6149, 0, `{"kind":"feature_fence"}`),
			receiptFact("ENCRYPTED_RECOGNIZER", "flink-behavior-receipt", "t3", "spec", 6149, 6149, 0, `{"kind":"recognition_fence"}`),
			receiptFact("RULE_DETECTION", "flink-behavior-receipt", "t4", "spec", 6149, 0, 0, `{"kind":"detector_fence"}`),
			receiptFact("BEHAVIOR_DETECTION", "flink-behavior-receipt", "t4", "spec", 6149, 0, 0, `{"kind":"detector_fence"}`),
			receiptFact("DETECTION_AGGREGATE", "flink-behavior-receipt", "t4", "spec", 6149, 6149, 0,
				`{"kind":"detection_fence","total":6149,"positive":0,"negative":6149,"inconclusive":0,"error":0}`),
		},
	}
}

func TestFinalizeLoopReconcileClean(t *testing.T) {
	l := NewFinalizeLoop(nil, nil)
	report := l.reconcile(benignFacts())
	if !report.OK {
		t.Fatalf("expected clean reconcile, got differences: %v", report.Items)
	}
	if report.SourceInput != 15000 || report.Sessions != 6149 || report.Negative != 6149 {
		t.Fatalf("unexpected counters: %+v", report)
	}
}

func TestFinalizeLoopReconcileFenceMismatch(t *testing.T) {
	l := NewFinalizeLoop(nil, nil)
	f := benignFacts()
	f.Receipts[1].FencingToken = "wrong-token"
	report := l.reconcile(f)
	if report.OK {
		t.Fatalf("expected fence mismatch to be reported")
	}
	if report.Differences == 0 {
		t.Fatalf("expected difference entries")
	}
}

func TestFinalizeLoopAssembleNoThreat(t *testing.T) {
	l := NewFinalizeLoop(nil, nil)
	f := benignFacts()
	report := l.reconcile(f)
	facts := l.assembleClosureFacts(f, report)
	d := state.EvaluateRunClosure(facts)
	if d.RunState != state.RunSucceeded {
		t.Fatalf("expected SUCCEEDED, got %s", d.RunState)
	}
	if d.FindingConclusion != state.ConclusionNoThreatObserved {
		t.Fatalf("expected NO_THREAT_OBSERVED, got %s", d.FindingConclusion)
	}
	if d.Completeness != state.CompleteComplete || d.IntegrityState != state.IntegrityVerified {
		t.Fatalf("unexpected axes: %s/%s", d.Completeness, d.IntegrityState)
	}
}

func TestFinalizeLoopAssembleThreatFound(t *testing.T) {
	l := NewFinalizeLoop(nil, nil)
	f := benignFacts()
	f.Receipts[6].Fence = []byte(`{"kind":"detection_fence","total":6149,"positive":3,"negative":6146,"inconclusive":0,"error":0}`)
	report := l.reconcile(f)
	facts := l.assembleClosureFacts(f, report)
	d := state.EvaluateRunClosure(facts)
	if d.FindingConclusion != state.ConclusionThreatFound {
		t.Fatalf("expected THREAT_FOUND, got %s", d.FindingConclusion)
	}
}

func TestFinalizeLoopAssembleFailedNode(t *testing.T) {
	l := NewFinalizeLoop(nil, nil)
	f := benignFacts()
	f.Attempts[2].State = "FAILED"
	f.Receipts[2].ErrorCount = 1
	report := l.reconcile(f)
	facts := l.assembleClosureFacts(f, report)
	d := state.EvaluateRunClosure(facts)
	if d.RunState != state.RunFailed {
		t.Fatalf("expected FAILED, got %s", d.RunState)
	}
}

func TestFinalizeLoopAssembleZeroInput(t *testing.T) {
	l := NewFinalizeLoop(nil, nil)
	f := benignFacts()
	f.Receipts[0].InputCount = 0
	f.Receipts[1].InputCount = 0
	f.Receipts[1].OutputCount = 0
	f.Receipts[6].Fence = []byte(`{"kind":"detection_fence","total":0,"positive":0,"negative":0,"inconclusive":0,"error":0}`)
	report := l.reconcile(f)
	facts := l.assembleClosureFacts(f, report)
	d := state.EvaluateRunClosure(facts)
	if d.RunState != state.RunSucceeded || d.FindingConclusion != state.ConclusionNoData {
		t.Fatalf("expected SUCCEEDED/NO_DATA, got %s/%s", d.RunState, d.FindingConclusion)
	}
}

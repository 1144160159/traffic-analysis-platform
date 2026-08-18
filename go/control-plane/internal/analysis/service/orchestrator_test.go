package service

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

func testNodes() []NodeFact {
	return []NodeFact{
		{ExecutionNodeID: "SOURCE_ACTIVATE", BusinessPhaseID: "S1", ProviderMode: "DEDICATED_OPERATION", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "SESSIONIZATION", BusinessPhaseID: "S2", ProviderMode: "SHARED_STREAM", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "FEATURE_EXTRACTION", BusinessPhaseID: "S2", ProviderMode: "SHARED_STREAM", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "ENCRYPTED_RECOGNIZER", BusinessPhaseID: "S3", ProviderMode: "SHARED_STREAM", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "RULE_DETECTION", BusinessPhaseID: "S4", ProviderMode: "SHARED_STREAM", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "BEHAVIOR_DETECTION", BusinessPhaseID: "S4", ProviderMode: "SHARED_STREAM", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "DETECTION_AGGREGATE", BusinessPhaseID: "S4", ProviderMode: "SHARED_STREAM", ActivationMode: "PIPELINED_STREAM", Required: true, State: state.StagePending},
		{ExecutionNodeID: "RECONCILE", BusinessPhaseID: "S5", ProviderMode: "AUTHORITY_LOCAL", ActivationMode: "AUTHORITY_LOCAL", Required: true, State: state.StagePending},
		{ExecutionNodeID: "MACHINE_FINALIZATION", BusinessPhaseID: "S5", ProviderMode: "AUTHORITY_LOCAL", ActivationMode: "AUTHORITY_LOCAL", Required: true, State: state.StagePending},
	}
}

func TestOrchestratorPlanReadyBarrier(t *testing.T) {
	o := NewOrchestrator()
	decision, err := o.Advance(context.Background(), OrchestratorInputs{
		RunState: state.RunAccepted, PlanReady: false, Nodes: testNodes(),
	})
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if decision.Wait != WaitPlanReady || len(decision.Dispatchables) != 0 {
		t.Fatalf("plan-ready barrier violated: %+v", decision)
	}
}

func TestOrchestratorPipelinedDispatch(t *testing.T) {
	o := NewOrchestrator()
	decision, err := o.Advance(context.Background(), OrchestratorInputs{
		RunState: state.RunRunning, PlanReady: true, AdmissionValid: true,
		SubscriptionActive: true, ReservationConsumed: true,
		Nodes: testNodes(),
	})
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if len(decision.Dispatchables) < 7 {
		t.Fatalf("expected all pipelined business nodes dispatchable, got %v", decision.Dispatchables)
	}
	// Reconcile/Finalizer 只在业务节点全部终态时派发
	for _, d := range decision.Dispatchables {
		if d == "RECONCILE" || d == "MACHINE_FINALIZATION" {
			t.Fatalf("authority-local nodes dispatched too early: %v", decision.Dispatchables)
		}
	}
}

func TestOrchestratorWaitCapacity(t *testing.T) {
	o := NewOrchestrator()
	decision, _ := o.Advance(context.Background(), OrchestratorInputs{
		RunState: state.RunRunning, PlanReady: true, AdmissionValid: false,
		SubscriptionActive: true, ReservationConsumed: false, Nodes: testNodes(),
	})
	if decision.Wait != WaitCapacity {
		t.Fatalf("expected WAIT_CAPACITY, got %v", decision.Wait)
	}
}

func TestOrchestratorReconcileThenFinalize(t *testing.T) {
	nodes := testNodes()
	for i := range nodes {
		if nodes[i].BusinessPhaseID != "S5" {
			nodes[i].State = state.StageSucceeded
		}
	}
	o := NewOrchestrator()
	decision, _ := o.Advance(context.Background(), OrchestratorInputs{
		RunState: state.RunFinalizing, PlanReady: true, AdmissionValid: true,
		SubscriptionActive: true, ReservationConsumed: true,
		RunPreconditions: state.RunPreconditions{AllNodesTerminal: true},
		Nodes:            nodes,
	})
	if !containsStr(decision.Dispatchables, "RECONCILE") {
		t.Fatalf("RECONCILE should dispatch after business nodes terminal: %v", decision.Dispatchables)
	}
}

func TestReconcileRejectsDifferences(t *testing.T) {
	if report, err := Reconcile(ReconcileFacts{AttemptGap: true}); err == nil || report.OK {
		t.Fatalf("attempt gap must fail reconcile")
	}
	if report, err := Reconcile(ReconcileFacts{}); err != nil || !report.OK {
		t.Fatalf("clean reconcile must pass: %v", err)
	}
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

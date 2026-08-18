package modeldrift

import (
	"reflect"
	"testing"
	"time"
)

func completeSnapshot(now time.Time) Snapshot {
	distributions := map[string]Distribution{}
	for _, feature := range RequiredFeatures {
		distributions[feature] = Distribution{
			Baseline: []uint64{250, 250, 250, 250},
			Current:  []uint64{250, 250, 250, 250},
		}
	}
	return Snapshot{
		TenantID: "tenant-a", ModelID: "model-a", ModelVersion: "model-v1", FeatureSetID: "feature-v1",
		EvaluatedAt: now, BaselineWindowStart: now.Add(-8 * 24 * time.Hour), BaselineWindowEnd: now.Add(-24 * time.Hour),
		CurrentWindowStart: now.Add(-24 * time.Hour), CurrentWindowEnd: now,
		FeatureWatermark: now.Add(-time.Minute), FeedbackWatermark: now.Add(-time.Hour),
		CurrentFeatureCount: 1000, FeedbackCount: 100, FalsePositiveCount: 10,
		FeatureDistributions: distributions,
	}
}

func TestEvaluateProducesCandidateWithoutActivationAuthority(t *testing.T) {
	now := time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC)
	snapshot := completeSnapshot(now)
	snapshot.FeatureDistributions["pps"] = Distribution{
		Baseline: []uint64{700, 200, 90, 10},
		Current:  []uint64{100, 200, 300, 400},
	}

	decision, err := Evaluate(DefaultPolicy(), snapshot)
	if err != nil {
		t.Fatalf("evaluate drift: %v", err)
	}
	if decision.State != DecisionCandidate || decision.ActivationAuthorized {
		t.Fatalf("drift must create candidate only: %+v", decision)
	}
	if decision.MaxObservedPSI <= DefaultPolicy().MaxPSI || !reflect.DeepEqual(decision.Reasons, []string{"psi_threshold_exceeded"}) {
		t.Fatalf("PSI trigger was not preserved: %+v", decision)
	}
	if len(decision.PolicySHA256) != 64 || len(decision.SignalSHA256) != 64 {
		t.Fatalf("decision hashes missing: %+v", decision)
	}

	replay, err := Evaluate(DefaultPolicy(), snapshot)
	if err != nil || !reflect.DeepEqual(replay, decision) {
		t.Fatalf("identical signal replay drifted: replay=%+v err=%v", replay, err)
	}
}

func TestEvaluateBlocksMissingStalePartialAndFutureSignals(t *testing.T) {
	now := time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC)
	snapshot := completeSnapshot(now)
	delete(snapshot.FeatureDistributions, "bps")
	snapshot.FeatureWatermark = now.Add(-3 * time.Hour)
	snapshot.FeedbackWatermark = now.Add(10 * time.Minute)
	snapshot.PartialFeatureCount = 20
	snapshot.FeedbackCount = 99

	decision, err := Evaluate(DefaultPolicy(), snapshot)
	if err != nil {
		t.Fatalf("blocked evidence must remain a decision: %v", err)
	}
	if decision.State != DecisionBlocked || decision.ActivationAuthorized {
		t.Fatalf("incomplete signals did not fail closed: %+v", decision)
	}
	want := []string{
		"feature_distribution_missing:bps",
		"feature_quality_partial",
		"feature_signals_stale",
		"feedback_samples_insufficient",
		"feedback_watermark_in_future",
	}
	if !reflect.DeepEqual(decision.Reasons, want) {
		t.Fatalf("blocked reasons = %v, want %v", decision.Reasons, want)
	}
}

func TestEvaluateNoActionAtExactThresholds(t *testing.T) {
	now := time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC)
	snapshot := completeSnapshot(now)
	snapshot.FalsePositiveCount = 15
	decision, err := Evaluate(DefaultPolicy(), snapshot)
	if err != nil || decision.State != DecisionNoAction {
		t.Fatalf("exact threshold must not mis-trigger: decision=%+v err=%v", decision, err)
	}
}

func TestEvaluateRejectsMalformedSnapshotInsteadOfHashingIt(t *testing.T) {
	now := time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*Snapshot){
		"cross-tenant-shape": func(snapshot *Snapshot) { snapshot.TenantID = " tenant-a" },
		"window-gap":         func(snapshot *Snapshot) { snapshot.CurrentWindowStart = snapshot.CurrentWindowStart.Add(time.Minute) },
		"unknown-feature": func(snapshot *Snapshot) {
			snapshot.FeatureDistributions["unknown"] = Distribution{Baseline: []uint64{1, 1}, Current: []uint64{1, 1}}
		},
		"count-overflow": func(snapshot *Snapshot) { snapshot.PartialFeatureCount = snapshot.CurrentFeatureCount + 1 },
		"fp-overflow":    func(snapshot *Snapshot) { snapshot.FalsePositiveCount = snapshot.FeedbackCount + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := completeSnapshot(now)
			mutate(&snapshot)
			if _, err := Evaluate(DefaultPolicy(), snapshot); err == nil {
				t.Fatal("malformed signal snapshot was accepted")
			}
		})
	}
}

package modelcanary

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestDecodeObservationRejectsUnknownAndTrailingJSON(t *testing.T) {
	observation := comparedObservation(1, time.UnixMilli(1_800_000_000_000), false)
	payload, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeObservation(payload)
	if err != nil || decoded.ObservationID != observation.ObservationID {
		t.Fatalf("strict valid observation decode failed: decoded=%+v err=%v", decoded, err)
	}
	withUnknown := append(payload[:len(payload)-1], []byte(`,"unknown":true}`)...)
	if _, err := DecodeObservation(withUnknown); err == nil {
		t.Fatal("unknown observation field was accepted")
	}
	if _, err := DecodeObservation(append(payload, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing observation JSON was accepted")
	}
}

func TestPolicyValidateFailsClosed(t *testing.T) {
	policy := validPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	tooBroad := policy
	tooBroad.RolloutPercentage = 11
	if err := tooBroad.Validate(); err == nil {
		t.Fatal("rollout above the tenant canary ceiling was accepted")
	}

	insufficientShadow := policy
	insufficientShadow.ShadowEvidence.MinimumSamples = policy.MinimumSamples - 1
	if err := insufficientShadow.Validate(); err == nil {
		t.Fatal("insufficient shadow evidence was accepted")
	}

	sameRollback := policy
	sameRollback.RollbackDeploymentID = sameRollback.DeploymentID
	if err := sameRollback.Validate(); err == nil {
		t.Fatal("canary without an independent rollback target was accepted")
	}
}

func TestWindowCompletesOnlyAfterSamplesAndObservationWindow(t *testing.T) {
	policy := validPolicy()
	window, err := NewWindow(policy)
	if err != nil {
		t.Fatal(err)
	}
	base := time.UnixMilli(1_800_000_000_000)
	var decision Decision
	for index := 0; index < policy.MinimumSamples; index++ {
		observation := comparedObservation(index, base.Add(
			time.Duration(index)*time.Duration(policy.ObservationWindowSeconds)*time.Second/
				time.Duration(policy.MinimumSamples-1)), false)
		decision, err = window.Observe(observation, base.Add(10*time.Minute))
		if err != nil {
			t.Fatalf("observation %d rejected: %v", index, err)
		}
	}
	if decision.State != StateWindowComplete || !decision.ExpandAllowed || decision.RollbackRequired {
		t.Fatalf("healthy complete window decision=%+v", decision)
	}
	if decision.Metrics.Samples != policy.MinimumSamples || decision.Metrics.Compared != policy.MinimumSamples {
		t.Fatalf("unexpected complete metrics: %+v", decision.Metrics)
	}
	if decision.Metrics.LatencyRatioP95 != 1.2 {
		t.Fatalf("latency p95=%f want=1.2", decision.Metrics.LatencyRatioP95)
	}
}

func TestWindowStopsOnRateThresholdAndNeverAutoExpands(t *testing.T) {
	policy := validPolicy()
	policy.Thresholds.MaximumDecisionChangeRate = 0.05
	window, _ := NewWindow(policy)
	base := time.UnixMilli(1_800_000_000_000)
	var decision Decision
	for index := 0; index < policy.MinimumSamples; index++ {
		decision, _ = window.Observe(
			comparedObservation(index, base.Add(time.Duration(index)*time.Second), index < 10),
			base.Add(10*time.Minute))
	}
	if decision.State != StateStopped || !decision.RollbackRequired || decision.ExpandAllowed {
		t.Fatalf("threshold stop decision=%+v", decision)
	}
	if !contains(decision.StopReasons, "decision_change_rate_exceeded") {
		t.Fatalf("missing decision threshold reason: %v", decision.StopReasons)
	}

	afterTerminal, _ := window.Observe(
		comparedObservation(999, base.Add(20*time.Minute), false), base.Add(20*time.Minute))
	if afterTerminal.State != StateStopped || afterTerminal.ExpandAllowed {
		t.Fatalf("terminal stop was not sticky: %+v", afterTerminal)
	}
}

func TestWindowStopsBeforeMinimumOnConsecutiveFailures(t *testing.T) {
	policy := validPolicy()
	policy.Thresholds.MaximumConsecutiveNonCompared = 3
	window, _ := NewWindow(policy)
	base := time.UnixMilli(1_800_000_000_000)
	var decision Decision
	for index := 0; index < 3; index++ {
		observation := comparedObservation(index, base.Add(time.Duration(index)*time.Second), false)
		observation.Status = "timeout"
		observation.ErrorCode = "challenger_timeout"
		observation.ErrorMessage = "timeout"
		observation.ChallengerScore = nil
		observation.ChallengerDetected = nil
		observation.AbsoluteScoreDelta = nil
		observation.DecisionChanged = nil
		observation.LabelChanged = nil
		decision, _ = window.Observe(observation, base.Add(time.Minute))
	}
	if decision.State != StateStopped || decision.Metrics.Samples != 3 ||
		!contains(decision.StopReasons, "consecutive_non_compared_exceeded") {
		t.Fatalf("consecutive failures did not stop canary: %+v", decision)
	}
}

func TestWindowScopesAndDeduplicatesStrictly(t *testing.T) {
	policy := validPolicy()
	window, _ := NewWindow(policy)
	now := time.UnixMilli(1_800_000_600_000)
	observation := comparedObservation(1, now.Add(-time.Minute), false)

	otherTenant := observation
	otherTenant.TenantID = "tenant-b"
	decision, err := window.Observe(otherTenant, now)
	if err != nil || decision.State != StateIgnored || decision.Metrics.Samples != 0 {
		t.Fatalf("other tenant was not ignored: decision=%+v err=%v", decision, err)
	}

	decision, err = window.Observe(observation, now)
	if err != nil || decision.State != StateObserving || decision.Metrics.Samples != 1 {
		t.Fatalf("first observation was not accepted: decision=%+v err=%v", decision, err)
	}
	decision, err = window.Observe(observation, now)
	if err != nil || decision.State != StateIgnored || decision.Metrics.Samples != 1 {
		t.Fatalf("exact duplicate was not idempotent: decision=%+v err=%v", decision, err)
	}

	conflict := observation
	changed := 0.7
	conflict.ChallengerScore = &changed
	decision, err = window.Observe(conflict, now)
	if err != nil || decision.State != StateStopped ||
		!contains(decision.StopReasons, "observation_id_payload_conflict") {
		t.Fatalf("same-ID payload conflict did not stop: decision=%+v err=%v", decision, err)
	}
}

func TestWindowStopsOnCandidateIdentityDrift(t *testing.T) {
	window, _ := NewWindow(validPolicy())
	now := time.UnixMilli(1_800_000_600_000)
	observation := comparedObservation(1, now.Add(-time.Minute), false)
	observation.ChallengerPackageSHA256 = digest("different-package")
	decision, err := window.Observe(observation, now)
	if err != nil || decision.State != StateStopped ||
		!contains(decision.StopReasons, "candidate_or_champion_identity_drift") {
		t.Fatalf("candidate drift did not stop: decision=%+v err=%v", decision, err)
	}
}

func validPolicy() Policy {
	return Policy{
		SchemaVersion:              SchemaVersion,
		CanaryID:                   "m08-n013-canary-1",
		Enabled:                    true,
		TenantID:                   "tenant-a",
		DeploymentID:               "deployment-candidate",
		RollbackDeploymentID:       "deployment-champion",
		CandidateModelID:           "model-a",
		CandidateVersion:           "v2",
		CandidatePackageSHA256:     digest("candidate-package"),
		CandidateAggregateRevision: 2,
		ChampionVersion:            "v1",
		RolloutPercentage:          5,
		MinimumSamples:             100,
		MaximumSamples:             1000,
		ObservationWindowSeconds:   300,
		MaximumClockSkewSeconds:    60,
		Thresholds: Thresholds{
			MaximumErrorRate:              0.01,
			MaximumTimeoutRate:            0.005,
			MaximumDecisionChangeRate:     0.2,
			MaximumLabelChangeRate:        0.2,
			MaximumAbsoluteScoreDeltaP95:  0.3,
			MaximumLatencyRatioP95:        1.5,
			MaximumChallengerHeapDeltaP95: 64 << 20,
			MaximumConsecutiveNonCompared: 3,
		},
		ShadowEvidence: Evidence{
			Path:                 "evidence/n012.json",
			SHA256:               digest("shadow-evidence"),
			RequiredStatus:       "PASS",
			MinimumSamples:       100,
			MinimumWindowSeconds: 300,
		},
	}
}

func comparedObservation(index int, observedAt time.Time, decisionChanged bool) Observation {
	championScore := 0.5
	challengerScore := 0.55
	delta := 0.05
	championDetected := false
	challengerDetected := decisionChanged
	labelChanged := decisionChanged
	return Observation{
		SchemaVersion:               SchemaVersion,
		ObservationID:               digest(fmt.Sprintf("observation-%d", index)),
		TenantID:                    "tenant-a",
		SourceEventID:               fmt.Sprintf("event-%d", index),
		ObjectID:                    fmt.Sprintf("object-%d", index),
		CommunityID:                 "1:test",
		EventTimeMS:                 observedAt.Add(-time.Second).UnixMilli(),
		ObservedAtMS:                observedAt.UnixMilli(),
		SampleBucket:                index % 10_000,
		ServingResultSource:         "champion",
		ChampionModelID:             "champion-model",
		ChampionVersion:             "v1",
		ChampionLabel:               "benign",
		ChampionScore:               &championScore,
		ChampionDetected:            &championDetected,
		ChampionLatencyNanos:        100,
		ChallengerModelID:           "model-a",
		ChallengerVersion:           "v2",
		ChallengerPackageID:         "package-a",
		ChallengerPackageSHA256:     digest("candidate-package"),
		ChallengerAggregateRevision: 2,
		ChallengerLabel:             "benign",
		ChallengerScore:             &challengerScore,
		ChallengerDetected:          &challengerDetected,
		ChallengerLatencyNanos:      120,
		ChallengerCPUNanos:          100,
		ChallengerHeapDeltaBytes:    1024,
		AbsoluteScoreDelta:          &delta,
		DecisionChanged:             &decisionChanged,
		LabelChanged:                &labelChanged,
		Status:                      "compared",
	}
}

func digest(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// Package modeldrift evaluates evidence-bound model drift snapshots.
//
// It is intentionally side-effect free.  Callers may persist a CANDIDATE
// decision or dispatch candidate training, but this package never authorizes
// serving activation.
package modeldrift

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type DecisionState string

const (
	DecisionBlocked   DecisionState = "BLOCKED"
	DecisionNoAction  DecisionState = "NO_ACTION"
	DecisionCandidate DecisionState = "CANDIDATE"
)

var RequiredFeatures = []string{"bps", "iat_mean_ms", "pktlen_mean", "pps"}

type Policy struct {
	Version              string        `json:"version"`
	MaxPSI               float64       `json:"max_psi"`
	MaxFPRate            float64       `json:"max_fp_rate"`
	MaxPartialRate       float64       `json:"max_partial_rate"`
	MinFeatureSamples    uint64        `json:"min_feature_samples"`
	MinFeedbackSamples   uint64        `json:"min_feedback_samples"`
	MaxFeatureSignalAge  time.Duration `json:"max_feature_signal_age_ns"`
	MaxFeedbackSignalAge time.Duration `json:"max_feedback_signal_age_ns"`
	MaximumFutureSkew    time.Duration `json:"maximum_future_skew_ns"`
}

func DefaultPolicy() Policy {
	return Policy{
		Version:              "model-drift-candidate.v1",
		MaxPSI:               0.25,
		MaxFPRate:            0.15,
		MaxPartialRate:       0.01,
		MinFeatureSamples:    1000,
		MinFeedbackSamples:   100,
		MaxFeatureSignalAge:  2 * time.Hour,
		MaxFeedbackSignalAge: 48 * time.Hour,
		MaximumFutureSkew:    5 * time.Minute,
	}
}

type Distribution struct {
	Baseline []uint64 `json:"baseline"`
	Current  []uint64 `json:"current"`
}

type Snapshot struct {
	TenantID             string                  `json:"tenant_id"`
	ModelID              string                  `json:"model_id"`
	ModelVersion         string                  `json:"model_version"`
	FeatureSetID         string                  `json:"feature_set_id"`
	EvaluatedAt          time.Time               `json:"evaluated_at"`
	BaselineWindowStart  time.Time               `json:"baseline_window_start"`
	BaselineWindowEnd    time.Time               `json:"baseline_window_end"`
	CurrentWindowStart   time.Time               `json:"current_window_start"`
	CurrentWindowEnd     time.Time               `json:"current_window_end"`
	FeatureWatermark     time.Time               `json:"feature_watermark"`
	FeedbackWatermark    time.Time               `json:"feedback_watermark"`
	CurrentFeatureCount  uint64                  `json:"current_feature_count"`
	PartialFeatureCount  uint64                  `json:"partial_feature_count"`
	FeedbackCount        uint64                  `json:"feedback_count"`
	FalsePositiveCount   uint64                  `json:"false_positive_count"`
	FeatureDistributions map[string]Distribution `json:"feature_distributions"`
}

type Decision struct {
	State                DecisionState      `json:"state"`
	Reasons              []string           `json:"reasons"`
	PSI                  map[string]float64 `json:"psi"`
	MaxObservedPSI       float64            `json:"max_observed_psi"`
	FalsePositiveRate    float64            `json:"false_positive_rate"`
	PolicySHA256         string             `json:"policy_sha256"`
	SignalSHA256         string             `json:"signal_sha256"`
	ActivationAuthorized bool               `json:"activation_authorized"`
}

func Evaluate(policy Policy, snapshot Snapshot) (Decision, error) {
	if err := validatePolicy(policy); err != nil {
		return Decision{}, err
	}
	if err := validateSnapshotIdentity(snapshot); err != nil {
		return Decision{}, err
	}

	policySHA, err := hashCanonical(policy)
	if err != nil {
		return Decision{}, fmt.Errorf("hash drift policy: %w", err)
	}
	signalSHA, err := hashCanonical(snapshot)
	if err != nil {
		return Decision{}, fmt.Errorf("hash drift signals: %w", err)
	}
	decision := Decision{
		State:                DecisionNoAction,
		PSI:                  make(map[string]float64, len(RequiredFeatures)),
		PolicySHA256:         policySHA,
		SignalSHA256:         signalSHA,
		ActivationAuthorized: false,
	}

	blocked := make([]string, 0)
	if snapshot.FeatureWatermark.IsZero() {
		blocked = append(blocked, "feature_watermark_missing")
	} else {
		blocked = append(blocked, watermarkReasons("feature", snapshot.FeatureWatermark, snapshot.EvaluatedAt, policy.MaxFeatureSignalAge, policy.MaximumFutureSkew)...)
	}
	if snapshot.FeedbackWatermark.IsZero() {
		blocked = append(blocked, "feedback_watermark_missing")
	} else {
		blocked = append(blocked, watermarkReasons("feedback", snapshot.FeedbackWatermark, snapshot.EvaluatedAt, policy.MaxFeedbackSignalAge, policy.MaximumFutureSkew)...)
	}
	if snapshot.CurrentFeatureCount == 0 {
		blocked = append(blocked, "feature_signals_missing")
	} else if float64(snapshot.PartialFeatureCount)/float64(snapshot.CurrentFeatureCount) > policy.MaxPartialRate {
		blocked = append(blocked, "feature_quality_partial")
	}
	if snapshot.FeedbackCount < policy.MinFeedbackSamples {
		blocked = append(blocked, "feedback_samples_insufficient")
	}
	if snapshot.FalsePositiveCount > snapshot.FeedbackCount {
		return Decision{}, errors.New("false-positive count exceeds feedback count")
	}

	for _, feature := range RequiredFeatures {
		distribution, ok := snapshot.FeatureDistributions[feature]
		if !ok {
			blocked = append(blocked, "feature_distribution_missing:"+feature)
			continue
		}
		baselineTotal, currentTotal, distributionErr := validateDistribution(distribution)
		if distributionErr != nil {
			return Decision{}, fmt.Errorf("feature %s: %w", feature, distributionErr)
		}
		if baselineTotal < policy.MinFeatureSamples {
			blocked = append(blocked, "baseline_samples_insufficient:"+feature)
			continue
		}
		if currentTotal < policy.MinFeatureSamples {
			blocked = append(blocked, "current_samples_insufficient:"+feature)
			continue
		}
		value := populationStabilityIndex(distribution.Baseline, distribution.Current)
		decision.PSI[feature] = value
		if value > decision.MaxObservedPSI {
			decision.MaxObservedPSI = value
		}
	}

	if snapshot.FeedbackCount > 0 {
		decision.FalsePositiveRate = float64(snapshot.FalsePositiveCount) / float64(snapshot.FeedbackCount)
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		decision.State = DecisionBlocked
		decision.Reasons = blocked
		return decision, nil
	}

	triggers := make([]string, 0, 2)
	if decision.MaxObservedPSI > policy.MaxPSI {
		triggers = append(triggers, "psi_threshold_exceeded")
	}
	if decision.FalsePositiveRate > policy.MaxFPRate {
		triggers = append(triggers, "false_positive_rate_threshold_exceeded")
	}
	if len(triggers) > 0 {
		decision.State = DecisionCandidate
		decision.Reasons = triggers
	}
	return decision, nil
}

func validatePolicy(policy Policy) error {
	if strings.TrimSpace(policy.Version) == "" {
		return errors.New("drift policy version is required")
	}
	for name, value := range map[string]float64{
		"max_psi": policy.MaxPSI, "max_fp_rate": policy.MaxFPRate, "max_partial_rate": policy.MaxPartialRate,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || (name != "max_psi" && value > 1) {
			return fmt.Errorf("invalid %s", name)
		}
	}
	if policy.MinFeatureSamples == 0 || policy.MinFeedbackSamples == 0 || policy.MaxFeatureSignalAge <= 0 || policy.MaxFeedbackSignalAge <= 0 || policy.MaximumFutureSkew < 0 {
		return errors.New("drift policy sample and time limits must be positive")
	}
	return nil
}

func validateSnapshotIdentity(snapshot Snapshot) error {
	for name, value := range map[string]string{
		"tenant_id": snapshot.TenantID, "model_id": snapshot.ModelID,
		"model_version": snapshot.ModelVersion, "feature_set_id": snapshot.FeatureSetID,
	} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%s is required and must be canonical", name)
		}
	}
	if snapshot.EvaluatedAt.IsZero() || !snapshot.BaselineWindowStart.Before(snapshot.BaselineWindowEnd) || !snapshot.CurrentWindowStart.Before(snapshot.CurrentWindowEnd) || !snapshot.BaselineWindowEnd.Equal(snapshot.CurrentWindowStart) || snapshot.CurrentWindowEnd.After(snapshot.EvaluatedAt) {
		return errors.New("drift signal windows are invalid")
	}
	if snapshot.PartialFeatureCount > snapshot.CurrentFeatureCount {
		return errors.New("partial feature count exceeds current feature count")
	}
	for feature := range snapshot.FeatureDistributions {
		found := false
		for _, required := range RequiredFeatures {
			if feature == required {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unsupported feature distribution %s", feature)
		}
	}
	return nil
}

func validateDistribution(distribution Distribution) (uint64, uint64, error) {
	if len(distribution.Baseline) < 2 || len(distribution.Baseline) != len(distribution.Current) {
		return 0, 0, errors.New("baseline and current distributions require the same bins")
	}
	var baselineTotal, currentTotal uint64
	for index := range distribution.Baseline {
		baselineTotal += distribution.Baseline[index]
		currentTotal += distribution.Current[index]
	}
	return baselineTotal, currentTotal, nil
}

func watermarkReasons(source string, watermark, evaluatedAt time.Time, maxAge, futureSkew time.Duration) []string {
	if watermark.After(evaluatedAt.Add(futureSkew)) {
		return []string{source + "_watermark_in_future"}
	}
	if watermark.Before(evaluatedAt.Add(-maxAge)) {
		return []string{source + "_signals_stale"}
	}
	return nil
}

func populationStabilityIndex(baseline, current []uint64) float64 {
	var baselineTotal, currentTotal uint64
	for index := range baseline {
		baselineTotal += baseline[index]
		currentTotal += current[index]
	}
	const epsilon = 1e-6
	denominatorBaseline := float64(baselineTotal) + epsilon*float64(len(baseline))
	denominatorCurrent := float64(currentTotal) + epsilon*float64(len(current))
	var psi float64
	for index := range baseline {
		baselineRate := (float64(baseline[index]) + epsilon) / denominatorBaseline
		currentRate := (float64(current[index]) + epsilon) / denominatorCurrent
		psi += (currentRate - baselineRate) * math.Log(currentRate/baselineRate)
	}
	return psi
}

func hashCanonical(value interface{}) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

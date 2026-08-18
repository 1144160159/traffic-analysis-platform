package modelcanary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

// Policy is the immutable N013 tenant-scoped rollout contract. It deliberately
// has no automatic promotion switch: a healthy window may become eligible for
// an independently approved expansion, while every stop decision is terminal.
type Policy struct {
	SchemaVersion              int        `json:"schema_version"`
	CanaryID                   string     `json:"canary_id"`
	Enabled                    bool       `json:"enabled"`
	TenantID                   string     `json:"tenant_id"`
	DeploymentID               string     `json:"deployment_id"`
	RollbackDeploymentID       string     `json:"rollback_deployment_id"`
	CandidateModelID           string     `json:"candidate_model_id"`
	CandidateVersion           string     `json:"candidate_version"`
	CandidatePackageSHA256     string     `json:"candidate_package_sha256"`
	CandidateAggregateRevision int64      `json:"candidate_aggregate_revision"`
	ChampionVersion            string     `json:"champion_version"`
	RolloutPercentage          int        `json:"rollout_percentage"`
	MinimumSamples             int        `json:"minimum_samples"`
	MaximumSamples             int        `json:"maximum_samples"`
	ObservationWindowSeconds   int        `json:"observation_window_seconds"`
	MaximumClockSkewSeconds    int        `json:"maximum_clock_skew_seconds"`
	Thresholds                 Thresholds `json:"stop_thresholds"`
	ShadowEvidence             Evidence   `json:"shadow_evidence"`
}

type Evidence struct {
	Path                 string `json:"path"`
	SHA256               string `json:"sha256"`
	RequiredStatus       string `json:"required_status"`
	MinimumSamples       int    `json:"minimum_samples"`
	MinimumWindowSeconds int    `json:"minimum_window_seconds"`
}

type Thresholds struct {
	MaximumErrorRate              float64 `json:"maximum_error_rate"`
	MaximumTimeoutRate            float64 `json:"maximum_timeout_rate"`
	MaximumDecisionChangeRate     float64 `json:"maximum_decision_change_rate"`
	MaximumLabelChangeRate        float64 `json:"maximum_label_change_rate"`
	MaximumAbsoluteScoreDeltaP95  float64 `json:"maximum_absolute_score_delta_p95"`
	MaximumLatencyRatioP95        float64 `json:"maximum_latency_ratio_p95"`
	MaximumChallengerHeapDeltaP95 int64   `json:"maximum_challenger_heap_delta_bytes_p95"`
	MaximumConsecutiveNonCompared int     `json:"maximum_consecutive_non_compared"`
}

func (p Policy) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("model canary schema_version must be %d", SchemaVersion)
	}
	for name, value := range map[string]string{
		"canary_id": p.CanaryID, "tenant_id": p.TenantID,
		"deployment_id":          p.DeploymentID,
		"rollback_deployment_id": p.RollbackDeploymentID,
		"candidate_model_id":     p.CandidateModelID,
		"candidate_version":      p.CandidateVersion,
		"champion_version":       p.ChampionVersion,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("model canary %s is required", name)
		}
	}
	if p.DeploymentID == p.RollbackDeploymentID {
		return errors.New("model canary deployment and rollback target must differ")
	}
	if !isSHA256(p.CandidatePackageSHA256) {
		return errors.New("model canary candidate_package_sha256 is invalid")
	}
	if p.CandidateAggregateRevision <= 0 {
		return errors.New("model canary candidate_aggregate_revision must be positive")
	}
	if p.RolloutPercentage <= 0 || p.RolloutPercentage > 10 {
		return errors.New("model canary rollout_percentage must be within [1,10]")
	}
	if p.MinimumSamples < 100 || p.MaximumSamples < p.MinimumSamples {
		return errors.New("model canary sample bounds are invalid or below 100")
	}
	if p.MaximumSamples > 1_000_000 {
		return errors.New("model canary maximum_samples exceeds the bounded window limit")
	}
	if p.ObservationWindowSeconds < 300 || p.ObservationWindowSeconds > 7*24*60*60 {
		return errors.New("model canary observation window must be within [300,604800] seconds")
	}
	if p.MaximumClockSkewSeconds < 0 || p.MaximumClockSkewSeconds > 300 {
		return errors.New("model canary maximum clock skew must be within [0,300] seconds")
	}
	if err := p.Thresholds.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.ShadowEvidence.Path) == "" || !isSHA256(p.ShadowEvidence.SHA256) {
		return errors.New("model canary shadow evidence path and sha256 are required")
	}
	if p.ShadowEvidence.RequiredStatus != "PASS" {
		return errors.New("model canary shadow evidence must require PASS")
	}
	if p.ShadowEvidence.MinimumSamples < p.MinimumSamples ||
		p.ShadowEvidence.MinimumWindowSeconds < p.ObservationWindowSeconds {
		return errors.New("model canary shadow evidence must cover the canary sample and window floors")
	}
	return nil
}

func (t Thresholds) validate() error {
	for name, value := range map[string]float64{
		"maximum_error_rate":               t.MaximumErrorRate,
		"maximum_timeout_rate":             t.MaximumTimeoutRate,
		"maximum_decision_change_rate":     t.MaximumDecisionChangeRate,
		"maximum_label_change_rate":        t.MaximumLabelChangeRate,
		"maximum_absolute_score_delta_p95": t.MaximumAbsoluteScoreDeltaP95,
	} {
		if !finiteWithin(value, 0, 1) {
			return fmt.Errorf("model canary %s must be finite and within [0,1]", name)
		}
	}
	if !finiteWithin(t.MaximumLatencyRatioP95, 1, 100) {
		return errors.New("model canary maximum_latency_ratio_p95 must be within [1,100]")
	}
	if t.MaximumChallengerHeapDeltaP95 < 0 || t.MaximumChallengerHeapDeltaP95 > 1<<40 {
		return errors.New("model canary heap delta threshold is invalid")
	}
	if t.MaximumConsecutiveNonCompared < 1 || t.MaximumConsecutiveNonCompared > 1000 {
		return errors.New("model canary consecutive failure threshold must be within [1,1000]")
	}
	return nil
}

func finiteWithin(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

// Observation mirrors the strict N012 Kafka JSON contract used as the N013
// safety signal. Nullable comparison fields stay pointers so malformed error
// records cannot be mistaken for zero-valued successful comparisons.
type Observation struct {
	SchemaVersion               int      `json:"schema_version"`
	ObservationID               string   `json:"observation_id"`
	TenantID                    string   `json:"tenant_id"`
	SourceEventID               string   `json:"source_event_id"`
	ObjectID                    string   `json:"object_id"`
	CommunityID                 string   `json:"community_id"`
	EventTimeMS                 int64    `json:"event_time_ms"`
	ObservedAtMS                int64    `json:"observed_at_ms"`
	SampleBucket                int      `json:"sample_bucket"`
	ServingResultSource         string   `json:"serving_result_source"`
	ChampionModelID             string   `json:"champion_model_id"`
	ChampionVersion             string   `json:"champion_version"`
	ChampionLabel               string   `json:"champion_label"`
	ChampionScore               *float64 `json:"champion_score"`
	ChampionDetected            *bool    `json:"champion_detected"`
	ChampionLatencyNanos        int64    `json:"champion_latency_nanos"`
	ChallengerModelID           string   `json:"challenger_model_id"`
	ChallengerVersion           string   `json:"challenger_version"`
	ChallengerPackageID         string   `json:"challenger_package_id"`
	ChallengerPackageSHA256     string   `json:"challenger_package_sha256"`
	ChallengerAggregateRevision int64    `json:"challenger_aggregate_revision"`
	ChallengerLabel             string   `json:"challenger_label"`
	ChallengerScore             *float64 `json:"challenger_score"`
	ChallengerDetected          *bool    `json:"challenger_detected"`
	ChallengerLatencyNanos      int64    `json:"challenger_latency_nanos"`
	ChallengerCPUNanos          int64    `json:"challenger_cpu_nanos"`
	ChallengerHeapDeltaBytes    int64    `json:"challenger_heap_delta_bytes"`
	AbsoluteScoreDelta          *float64 `json:"absolute_score_delta"`
	DecisionChanged             *bool    `json:"decision_changed"`
	LabelChanged                *bool    `json:"label_changed"`
	Status                      string   `json:"status"`
	ErrorCode                   string   `json:"error_code"`
	ErrorMessage                string   `json:"error_message"`
}

func DecodeObservation(payload []byte) (Observation, error) {
	var observation Observation
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&observation); err != nil {
		return Observation{}, fmt.Errorf("decode model canary observation: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Observation{}, errors.New("decode model canary observation: trailing JSON value")
		}
		return Observation{}, fmt.Errorf("decode model canary observation trailing data: %w", err)
	}
	if err := observation.Validate(); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

func (o Observation) Validate() error {
	if o.SchemaVersion != SchemaVersion || !isSHA256(o.ObservationID) {
		return errors.New("model canary observation identity is invalid")
	}
	if strings.TrimSpace(o.TenantID) == "" || strings.TrimSpace(o.SourceEventID) == "" ||
		o.ObservedAtMS <= 0 || o.EventTimeMS < 0 || o.SampleBucket < 0 || o.SampleBucket >= 10_000 {
		return errors.New("model canary observation source contract is invalid")
	}
	if o.ServingResultSource != "champion" {
		return errors.New("model canary safety input must preserve champion serving authority")
	}
	if strings.TrimSpace(o.ChallengerModelID) == "" || strings.TrimSpace(o.ChallengerVersion) == "" ||
		strings.TrimSpace(o.ChallengerPackageID) == "" || !isSHA256(o.ChallengerPackageSHA256) ||
		o.ChallengerAggregateRevision <= 0 {
		return errors.New("model canary challenger identity is invalid")
	}
	switch o.Status {
	case "compared":
		if o.ChampionScore == nil || o.ChampionDetected == nil || o.ChallengerScore == nil ||
			o.ChallengerDetected == nil || o.AbsoluteScoreDelta == nil ||
			o.DecisionChanged == nil || o.LabelChanged == nil {
			return errors.New("model canary compared observation is incomplete")
		}
		for _, value := range []*float64{o.ChampionScore, o.ChallengerScore, o.AbsoluteScoreDelta} {
			if !finiteWithin(*value, 0, 1) {
				return errors.New("model canary compared score is invalid")
			}
		}
		if o.ErrorCode != "" || o.ErrorMessage != "" || o.ChampionLatencyNanos <= 0 ||
			o.ChallengerLatencyNanos <= 0 {
			return errors.New("model canary compared latency or error contract is invalid")
		}
	case "champion_unavailable", "timeout", "error", "overloaded":
		if strings.TrimSpace(o.ErrorCode) == "" {
			return errors.New("model canary failed observation requires error_code")
		}
	default:
		return fmt.Errorf("model canary observation status %q is unsupported", o.Status)
	}
	return nil
}

type State string

const (
	StateIgnored        State = "IGNORED"
	StateObserving      State = "OBSERVING"
	StateStopped        State = "STOPPED"
	StateWindowComplete State = "WINDOW_COMPLETE"
)

type Metrics struct {
	Samples                     int     `json:"samples"`
	Compared                    int     `json:"compared"`
	Errors                      int     `json:"errors"`
	Timeouts                    int     `json:"timeouts"`
	DecisionChanges             int     `json:"decision_changes"`
	LabelChanges                int     `json:"label_changes"`
	ErrorRate                   float64 `json:"error_rate"`
	TimeoutRate                 float64 `json:"timeout_rate"`
	DecisionChangeRate          float64 `json:"decision_change_rate"`
	LabelChangeRate             float64 `json:"label_change_rate"`
	AbsoluteScoreDeltaP95       float64 `json:"absolute_score_delta_p95"`
	LatencyRatioP95             float64 `json:"latency_ratio_p95"`
	ChallengerHeapDeltaBytesP95 int64   `json:"challenger_heap_delta_bytes_p95"`
	ConsecutiveNonCompared      int     `json:"consecutive_non_compared"`
	FirstObservedAtMS           int64   `json:"first_observed_at_ms"`
	LastObservedAtMS            int64   `json:"last_observed_at_ms"`
}

type Decision struct {
	SchemaVersion     int      `json:"schema_version"`
	CanaryID          string   `json:"canary_id"`
	TenantID          string   `json:"tenant_id"`
	DeploymentID      string   `json:"deployment_id"`
	State             State    `json:"state"`
	ExpandAllowed     bool     `json:"expand_allowed"`
	RollbackRequired  bool     `json:"rollback_required"`
	StopReasons       []string `json:"stop_reasons"`
	Metrics           Metrics  `json:"metrics"`
	LastObservationID string   `json:"last_observation_id"`
	DecidedAtMS       int64    `json:"decided_at_ms"`
}

type Window struct {
	policy              Policy
	seen                map[string][sha256.Size]byte
	metrics             Metrics
	absoluteScoreDeltas []float64
	latencyRatios       []float64
	heapDeltas          []int64
	terminal            *Decision
}

func NewWindow(policy Policy) (*Window, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &Window{policy: policy, seen: make(map[string][sha256.Size]byte)}, nil
}

// ForceStop converts an infrastructure, contract or controller failure into
// the same terminal rollback decision used by metric thresholds.
func (w *Window) ForceStop(now time.Time, reason string) Decision {
	if w.terminal != nil {
		return *w.terminal
	}
	if strings.TrimSpace(reason) == "" {
		reason = "controller_failure"
	}
	return w.stop(now, reason)
}

// Observe accepts one strict N012 observation. Shared-topic records for another
// tenant or model are ignored. A same-candidate identity drift is a hard stop.
func (w *Window) Observe(observation Observation, now time.Time) (Decision, error) {
	if w.terminal != nil {
		return *w.terminal, nil
	}
	if err := observation.Validate(); err != nil {
		return w.stop(now, "invalid_observation_contract"), err
	}
	if observation.TenantID != w.policy.TenantID ||
		observation.ChallengerModelID != w.policy.CandidateModelID {
		return w.decision(StateIgnored, now, observation.ObservationID, nil), nil
	}
	if observation.ChallengerVersion != w.policy.CandidateVersion ||
		observation.ChallengerPackageSHA256 != w.policy.CandidatePackageSHA256 ||
		observation.ChallengerAggregateRevision != w.policy.CandidateAggregateRevision ||
		(observation.ChampionVersion != "" && observation.ChampionVersion != w.policy.ChampionVersion) {
		return w.stop(now, "candidate_or_champion_identity_drift"), nil
	}
	clockSkew := now.UnixMilli() - observation.ObservedAtMS
	if clockSkew < -int64(w.policy.MaximumClockSkewSeconds)*1000 {
		return w.stop(now, "observation_from_future"), nil
	}
	payload, err := json.Marshal(observation)
	if err != nil {
		return w.stop(now, "observation_hash_failed"), err
	}
	digest := sha256.Sum256(payload)
	if previous, exists := w.seen[observation.ObservationID]; exists {
		if previous != digest {
			return w.stop(now, "observation_id_payload_conflict"), nil
		}
		return w.decision(StateIgnored, now, observation.ObservationID, nil), nil
	}
	w.seen[observation.ObservationID] = digest
	w.metrics.Samples++
	if w.metrics.FirstObservedAtMS == 0 || observation.ObservedAtMS < w.metrics.FirstObservedAtMS {
		w.metrics.FirstObservedAtMS = observation.ObservedAtMS
	}
	if observation.ObservedAtMS > w.metrics.LastObservedAtMS {
		w.metrics.LastObservedAtMS = observation.ObservedAtMS
	}
	if observation.Status == "compared" {
		w.metrics.Compared++
		w.metrics.ConsecutiveNonCompared = 0
		if *observation.DecisionChanged {
			w.metrics.DecisionChanges++
		}
		if *observation.LabelChanged {
			w.metrics.LabelChanges++
		}
		w.absoluteScoreDeltas = append(w.absoluteScoreDeltas, *observation.AbsoluteScoreDelta)
		w.latencyRatios = append(w.latencyRatios,
			float64(observation.ChallengerLatencyNanos)/float64(observation.ChampionLatencyNanos))
		w.heapDeltas = append(w.heapDeltas, max64(0, observation.ChallengerHeapDeltaBytes))
	} else {
		w.metrics.Errors++
		w.metrics.ConsecutiveNonCompared++
		if observation.Status == "timeout" {
			w.metrics.Timeouts++
		}
		if observation.Status == "champion_unavailable" {
			return w.stop(now, "champion_unavailable"), nil
		}
	}
	w.refreshMetrics()
	if w.metrics.ConsecutiveNonCompared >= w.policy.Thresholds.MaximumConsecutiveNonCompared {
		return w.stop(now, "consecutive_non_compared_exceeded"), nil
	}
	if w.metrics.Samples >= w.policy.MinimumSamples {
		if reasons := w.thresholdReasons(); len(reasons) > 0 {
			return w.stop(now, reasons...), nil
		}
	}
	windowMS := int64(w.policy.ObservationWindowSeconds) * 1000
	if w.metrics.Samples >= w.policy.MinimumSamples &&
		w.metrics.LastObservedAtMS-w.metrics.FirstObservedAtMS >= windowMS {
		decision := w.decision(StateWindowComplete, now, observation.ObservationID, nil)
		decision.ExpandAllowed = true
		w.terminal = &decision
		return decision, nil
	}
	if w.metrics.Samples >= w.policy.MaximumSamples {
		return w.stop(now, "maximum_samples_before_observation_window"), nil
	}
	return w.decision(StateObserving, now, observation.ObservationID, nil), nil
}

func (w *Window) thresholdReasons() []string {
	t := w.policy.Thresholds
	reasons := make([]string, 0, 7)
	if w.metrics.ErrorRate > t.MaximumErrorRate {
		reasons = append(reasons, "error_rate_exceeded")
	}
	if w.metrics.TimeoutRate > t.MaximumTimeoutRate {
		reasons = append(reasons, "timeout_rate_exceeded")
	}
	if w.metrics.DecisionChangeRate > t.MaximumDecisionChangeRate {
		reasons = append(reasons, "decision_change_rate_exceeded")
	}
	if w.metrics.LabelChangeRate > t.MaximumLabelChangeRate {
		reasons = append(reasons, "label_change_rate_exceeded")
	}
	if w.metrics.AbsoluteScoreDeltaP95 > t.MaximumAbsoluteScoreDeltaP95 {
		reasons = append(reasons, "absolute_score_delta_p95_exceeded")
	}
	if w.metrics.LatencyRatioP95 > t.MaximumLatencyRatioP95 {
		reasons = append(reasons, "latency_ratio_p95_exceeded")
	}
	if w.metrics.ChallengerHeapDeltaBytesP95 > t.MaximumChallengerHeapDeltaP95 {
		reasons = append(reasons, "challenger_heap_delta_p95_exceeded")
	}
	return reasons
}

func (w *Window) refreshMetrics() {
	w.metrics.ErrorRate = rate(w.metrics.Errors, w.metrics.Samples)
	w.metrics.TimeoutRate = rate(w.metrics.Timeouts, w.metrics.Samples)
	w.metrics.DecisionChangeRate = rate(w.metrics.DecisionChanges, w.metrics.Compared)
	w.metrics.LabelChangeRate = rate(w.metrics.LabelChanges, w.metrics.Compared)
	w.metrics.AbsoluteScoreDeltaP95 = percentile95(w.absoluteScoreDeltas)
	w.metrics.LatencyRatioP95 = percentile95(w.latencyRatios)
	w.metrics.ChallengerHeapDeltaBytesP95 = percentile95Int(w.heapDeltas)
}

func (w *Window) stop(now time.Time, reasons ...string) Decision {
	decision := w.decision(StateStopped, now, "", reasons)
	decision.RollbackRequired = true
	w.terminal = &decision
	return decision
}

func (w *Window) decision(state State, now time.Time, observationID string, reasons []string) Decision {
	return Decision{
		SchemaVersion:     SchemaVersion,
		CanaryID:          w.policy.CanaryID,
		TenantID:          w.policy.TenantID,
		DeploymentID:      w.policy.DeploymentID,
		State:             state,
		ExpandAllowed:     false,
		RollbackRequired:  false,
		StopReasons:       append([]string(nil), reasons...),
		Metrics:           w.metrics,
		LastObservationID: observationID,
		DecidedAtMS:       now.UnixMilli(),
	}
}

func rate(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyOf := append([]float64(nil), values...)
	sort.Float64s(copyOf)
	index := int(math.Ceil(float64(len(copyOf))*0.95)) - 1
	return copyOf[max(0, index)]
}

func percentile95Int(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyOf := append([]int64(nil), values...)
	sort.Slice(copyOf, func(i, j int) bool { return copyOf[i] < copyOf[j] })
	index := int(math.Ceil(float64(len(copyOf))*0.95)) - 1
	return copyOf[max(0, index)]
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

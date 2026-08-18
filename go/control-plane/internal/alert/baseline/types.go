package baseline

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidRequest   = errors.New("invalid behavior baseline request")
	ErrIdentityConflict = errors.New("behavior baseline identity conflict")
	ErrRevisionConflict = errors.New("behavior baseline revision conflict")
	ErrStateConflict    = errors.New("behavior baseline state conflict")
)

const (
	LifecycleTopic         = "baseline.lifecycle.v1"
	ActivationAckTopic     = "baseline.activation-acks.v1"
	ActivationAckGroup     = "alert-service-baseline-activation-acks-v1"
	ActivationAckEventType = "baseline.activation.acknowledged.v1"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type BuildRequest struct {
	TenantID            string
	BaselineID          string
	BaselineKind        string
	EntityType          string
	EntityID            string
	ExpectedRevision    int64
	WindowStart         *time.Time
	WindowEnd           *time.Time
	MinimumEligibleRows int64
	AlgorithmVersion    string
	SamplePolicy        map[string]interface{}
	ThresholdSpec       map[string]interface{}
	ExpectedConsumers   []string
	CandidateSHA256     string
	IdempotencyKey      string
	RequestedBy         string
	Reason              string
	TraceID             string
}

func (request *BuildRequest) Validate() error {
	if request == nil {
		return fmt.Errorf("%w: request is nil", ErrInvalidRequest)
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.BaselineID = strings.TrimSpace(request.BaselineID)
	request.EntityType = strings.TrimSpace(request.EntityType)
	request.EntityID = strings.TrimSpace(request.EntityID)
	request.AlgorithmVersion = strings.TrimSpace(request.AlgorithmVersion)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.RequestedBy = strings.TrimSpace(request.RequestedBy)
	request.Reason = strings.TrimSpace(request.Reason)
	request.TraceID = strings.TrimSpace(request.TraceID)
	if request.TenantID == "" || request.BaselineID == "" || request.EntityType == "" || request.EntityID == "" ||
		request.BaselineID != request.EntityType+":"+request.EntityID || request.ExpectedRevision <= 0 ||
		request.AlgorithmVersion == "" || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 200 ||
		request.RequestedBy == "" || request.Reason == "" || request.TraceID == "" ||
		request.SamplePolicy == nil || request.ThresholdSpec == nil || !sha256Pattern.MatchString(request.CandidateSHA256) {
		return fmt.Errorf("%w: incomplete build identity", ErrInvalidRequest)
	}
	switch request.BaselineKind {
	case "dynamic":
		if request.WindowStart == nil || request.WindowEnd == nil || !request.WindowStart.Before(*request.WindowEnd) ||
			request.WindowEnd.Sub(*request.WindowStart) > 31*24*time.Hour || request.MinimumEligibleRows <= 0 {
			return fmt.Errorf("%w: dynamic build requires a closed window of at most 31 days and a positive minimum sample", ErrInvalidRequest)
		}
		request.SamplePolicy["minimum_eligible_rows"] = request.MinimumEligibleRows
		maxAge, ok := numericValue(request.SamplePolicy["max_active_age_seconds"])
		if !ok || maxAge <= 0 {
			return fmt.Errorf("%w: dynamic build requires a positive max_active_age_seconds", ErrInvalidRequest)
		}
	case "static":
		if request.WindowStart != nil || request.WindowEnd != nil || request.MinimumEligibleRows != 0 {
			return fmt.Errorf("%w: static build cannot carry a learned sample window", ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: baseline_kind must be static or dynamic", ErrInvalidRequest)
	}
	request.ExpectedConsumers = uniqueSorted(request.ExpectedConsumers)
	if len(request.ExpectedConsumers) == 0 {
		return fmt.Errorf("%w: at least one activation consumer is required", ErrInvalidRequest)
	}
	return nil
}

type BuildReceipt struct {
	JobID              string `json:"job_id"`
	BaselineID         string `json:"baseline_id"`
	BaselineKind       string `json:"baseline_kind"`
	DefinitionRevision int64  `json:"definition_revision"`
	TargetVersion      int64  `json:"target_version"`
	Status             string `json:"status"`
	EventID            string `json:"event_id"`
	OutboxStatus       string `json:"outbox_status"`
	Replayed           bool   `json:"replayed"`
}

type DynamicSampleResult struct {
	TenantID           string
	JobID              string
	CandidateSHA256    string
	MaxEventTime       *time.Time
	RowCount           int64
	EligibleRowCount   int64
	QualityStatus      string
	PartialReasons     []string
	SourceWatermark    map[string]interface{}
	SourceQuerySHA256  string
	SampleObjectURI    string
	SampleObjectSHA256 string
	Statistics         map[string]interface{}
	Provenance         map[string]interface{}
	CompletedBy        string
}

func (result *DynamicSampleResult) Validate() error {
	if result == nil || strings.TrimSpace(result.TenantID) == "" || strings.TrimSpace(result.JobID) == "" ||
		!sha256Pattern.MatchString(result.CandidateSHA256) || result.RowCount < 0 || result.EligibleRowCount < 0 ||
		result.EligibleRowCount > result.RowCount || result.SourceWatermark == nil || result.Statistics == nil ||
		result.Provenance == nil || strings.TrimSpace(result.CompletedBy) == "" || !sha256Pattern.MatchString(result.SourceQuerySHA256) {
		return fmt.Errorf("%w: incomplete dynamic sample result", ErrInvalidRequest)
	}
	if result.QualityStatus != "complete" && result.QualityStatus != "partial" && result.QualityStatus != "failed" {
		return fmt.Errorf("%w: unsupported sample quality", ErrInvalidRequest)
	}
	result.PartialReasons = uniqueSorted(result.PartialReasons)
	if result.QualityStatus == "complete" && len(result.PartialReasons) != 0 {
		return fmt.Errorf("%w: complete sample cannot carry partial reasons", ErrInvalidRequest)
	}
	if (result.SampleObjectURI == "") != (result.SampleObjectSHA256 == "") ||
		(result.SampleObjectSHA256 != "" && !sha256Pattern.MatchString(result.SampleObjectSHA256)) {
		return fmt.Errorf("%w: sample object URI and SHA must be supplied together", ErrInvalidRequest)
	}
	return nil
}

type VersionReceipt struct {
	JobID            string `json:"job_id"`
	BaselineID       string `json:"baseline_id"`
	BaselineVersion  int64  `json:"baseline_version"`
	VersionID        string `json:"version_id"`
	SampleSnapshotID string `json:"sample_snapshot_id,omitempty"`
	LifecycleState   string `json:"lifecycle_state"`
	QualityStatus    string `json:"quality_status"`
	SnapshotSHA256   string `json:"snapshot_sha256"`
	EventID          string `json:"event_id"`
}

type StaticVersionResult struct {
	TenantID        string
	JobID           string
	CandidateSHA256 string
	Statistics      map[string]interface{}
	Provenance      map[string]interface{}
	CompletedBy     string
}

func (result *StaticVersionResult) Validate() error {
	if result == nil || strings.TrimSpace(result.TenantID) == "" || strings.TrimSpace(result.JobID) == "" ||
		!sha256Pattern.MatchString(result.CandidateSHA256) || result.Statistics == nil || result.Provenance == nil ||
		strings.TrimSpace(result.CompletedBy) == "" {
		return fmt.Errorf("%w: incomplete static version result", ErrInvalidRequest)
	}
	return nil
}

type ApprovalRequest struct {
	TenantID         string
	BaselineID       string
	BaselineVersion  int64
	ExpectedRevision int64
	CandidateSHA256  string
	IdempotencyKey   string
	RequestedBy      string
	Reason           string
	TraceID          string
	ExpiresAt        time.Time
}

func (request *ApprovalRequest) Validate(now time.Time) error {
	if request == nil || strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.BaselineID) == "" ||
		request.BaselineVersion <= 0 || request.ExpectedRevision <= 0 || !sha256Pattern.MatchString(request.CandidateSHA256) ||
		strings.TrimSpace(request.IdempotencyKey) == "" || len(strings.TrimSpace(request.IdempotencyKey)) > 200 ||
		strings.TrimSpace(request.RequestedBy) == "" || strings.TrimSpace(request.Reason) == "" ||
		strings.TrimSpace(request.TraceID) == "" || !request.ExpiresAt.After(now) || request.ExpiresAt.After(now.Add(24*time.Hour)) {
		return fmt.Errorf("%w: approval request must identify a frozen version and expire within 24 hours", ErrInvalidRequest)
	}
	return nil
}

type ApprovalReceipt struct {
	ApprovalID        string   `json:"approval_id"`
	BaselineID        string   `json:"baseline_id"`
	BaselineVersion   int64    `json:"baseline_version"`
	Status            string   `json:"status"`
	ExpectedConsumers []string `json:"expected_consumers,omitempty"`
	EventID           string   `json:"event_id,omitempty"`
	Replayed          bool     `json:"replayed"`
}

type ActivationAck struct {
	EventID         string
	TenantID        string
	BaselineID      string
	BaselineVersion int64
	ConsumerID      string
	CandidateSHA256 string
	SnapshotSHA256  string
	AckSHA256       string
	AppliedAt       time.Time
	TraceID         string
}

func (ack *ActivationAck) Validate() error {
	if ack == nil || strings.TrimSpace(ack.EventID) == "" || strings.TrimSpace(ack.TenantID) == "" ||
		strings.TrimSpace(ack.BaselineID) == "" || ack.BaselineVersion <= 0 || strings.TrimSpace(ack.ConsumerID) == "" ||
		!sha256Pattern.MatchString(ack.CandidateSHA256) || !sha256Pattern.MatchString(ack.SnapshotSHA256) ||
		!sha256Pattern.MatchString(ack.AckSHA256) || ack.AppliedAt.IsZero() || strings.TrimSpace(ack.TraceID) == "" {
		return fmt.Errorf("%w: incomplete activation acknowledgement", ErrInvalidRequest)
	}
	return nil
}

type ActivationReceipt struct {
	BaselineID        string   `json:"baseline_id"`
	BaselineVersion   int64    `json:"baseline_version"`
	ConsumerID        string   `json:"consumer_id"`
	AckStatus         string   `json:"ack_status"`
	AckedConsumers    []string `json:"acked_consumers"`
	PendingConsumers  []string `json:"pending_consumers"`
	FailedConsumers   []string `json:"failed_consumers"`
	LifecycleState    string   `json:"lifecycle_state"`
	ActivationEventID string   `json:"activation_event_id,omitempty"`
	Replayed          bool     `json:"replayed"`
}

type RollbackRequest struct {
	TenantID            string
	BaselineID          string
	TargetStableVersion int64
	ExpectedRevision    int64
	CandidateSHA256     string
	IdempotencyKey      string
	RequestedBy         string
	Reason              string
	TraceID             string
}

func (request *RollbackRequest) Validate() error {
	if request == nil || strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.BaselineID) == "" ||
		request.TargetStableVersion <= 0 || request.ExpectedRevision <= 0 || !sha256Pattern.MatchString(request.CandidateSHA256) ||
		strings.TrimSpace(request.IdempotencyKey) == "" || len(strings.TrimSpace(request.IdempotencyKey)) > 200 ||
		strings.TrimSpace(request.RequestedBy) == "" || strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.TraceID) == "" {
		return fmt.Errorf("%w: incomplete rollback request", ErrInvalidRequest)
	}
	return nil
}

type RollbackReceipt struct {
	BaselineID          string `json:"baseline_id"`
	TargetStableVersion int64  `json:"target_stable_version"`
	RollbackVersion     int64  `json:"rollback_version"`
	VersionID           string `json:"version_id"`
	SnapshotSHA256      string `json:"snapshot_sha256"`
	LifecycleState      string `json:"lifecycle_state"`
	EventID             string `json:"event_id"`
	Replayed            bool   `json:"replayed"`
}

type EvaluationRequest struct {
	TenantID      string
	BaselineID    string
	MetricName    string
	ObservedValue float64
	ObservedAt    time.Time
	WindowStart   *time.Time
	WindowEnd     *time.Time
	EvidenceRefs  []string
	TraceID       string
}

func (request *EvaluationRequest) Validate() error {
	if request == nil || strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.BaselineID) == "" ||
		strings.TrimSpace(request.MetricName) == "" || request.ObservedAt.IsZero() ||
		strings.TrimSpace(request.TraceID) == "" {
		return fmt.Errorf("%w: evaluation identity, time and evidence are required", ErrInvalidRequest)
	}
	request.EvidenceRefs = uniqueSorted(request.EvidenceRefs)
	if len(request.EvidenceRefs) == 0 {
		return fmt.Errorf("%w: evaluation identity, time and evidence are required", ErrInvalidRequest)
	}
	if request.WindowStart != nil || request.WindowEnd != nil {
		if request.WindowStart == nil || request.WindowEnd == nil || !request.WindowStart.Before(*request.WindowEnd) ||
			request.ObservedAt.Before(*request.WindowStart) || request.ObservedAt.After(*request.WindowEnd) {
			return fmt.Errorf("%w: evaluation window is invalid", ErrInvalidRequest)
		}
	}
	return nil
}

type EvaluationReceipt struct {
	EvaluationID     string   `json:"evaluation_id"`
	BaselineID       string   `json:"baseline_id"`
	BaselineVersion  int64    `json:"baseline_version,omitempty"`
	SnapshotSHA256   string   `json:"snapshot_sha256,omitempty"`
	MetricName       string   `json:"metric_name"`
	ObservedValue    float64  `json:"observed_value"`
	MeanValue        *float64 `json:"mean_value,omitempty"`
	StdDevValue      *float64 `json:"stddev_value,omitempty"`
	DeviationScore   *float64 `json:"deviation_score,omitempty"`
	WarningThreshold *float64 `json:"warning_threshold,omitempty"`
	AlertThreshold   *float64 `json:"alert_threshold,omitempty"`
	Disposition      string   `json:"disposition"`
	QualityStatus    string   `json:"quality_status"`
	FailureCode      string   `json:"failure_code,omitempty"`
	EvidenceRefs     []string `json:"evidence_refs"`
}

func numericValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		result, err := typed.Float64()
		return result, err == nil
	default:
		return 0, false
	}
}

type ApprovalDecision struct {
	TenantID         string
	ApprovalID       string
	ExpectedRevision int64
	CandidateSHA256  string
	DecidedBy        string
	Approve          bool
	Reason           string
	TraceID          string
}

func (decision *ApprovalDecision) Validate() error {
	if decision == nil || strings.TrimSpace(decision.TenantID) == "" || strings.TrimSpace(decision.ApprovalID) == "" ||
		decision.ExpectedRevision <= 0 || !sha256Pattern.MatchString(decision.CandidateSHA256) ||
		strings.TrimSpace(decision.DecidedBy) == "" || strings.TrimSpace(decision.Reason) == "" || strings.TrimSpace(decision.TraceID) == "" {
		return fmt.Errorf("%w: incomplete approval decision", ErrInvalidRequest)
	}
	return nil
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

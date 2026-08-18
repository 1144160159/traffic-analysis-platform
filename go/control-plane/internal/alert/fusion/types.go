package fusion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SourceSyncTopic     = "fusion.commands.v1"
	SourceSyncEventType = "fusion.source-sync.requested.v1"
	SourceSyncGroup     = "alert-service-fusion-projection-v1"
	MaxSourceFacts      = 100000
)

var (
	ErrInvalidCommand   = errors.New("invalid fusion source-sync command")
	ErrIdentityConflict = errors.New("fusion source snapshot identity conflict")
	ErrVersionConflict  = errors.New("fusion source snapshot version conflict")
)

var sourceKinds = map[string]string{
	"traffic":  "flow",
	"asset":    "asset",
	"log":      "device_log",
	"behavior": "user_event",
}

var requiredSourceIDs = []string{"asset", "behavior", "log", "traffic"}

type SourceSyncCommand struct {
	EventID               string    `json:"event_id"`
	EventType             string    `json:"event_type"`
	SchemaVersion         int64     `json:"schema_version"`
	AggregateType         string    `json:"aggregate_type"`
	AggregateID           string    `json:"aggregate_id"`
	AggregateVersion      int64     `json:"aggregate_version"`
	PartitionKey          string    `json:"partition_key"`
	TenantID              string    `json:"tenant_id"`
	JobID                 string    `json:"job_id"`
	SourceID              string    `json:"source_id"`
	SourceKind            string    `json:"source_kind"`
	WindowStart           time.Time `json:"window_start"`
	WindowEnd             time.Time `json:"window_end"`
	ExpectedSourceVersion *int64    `json:"expected_source_version,omitempty"`
	RequestedBy           string    `json:"requested_by"`
	Reason                string    `json:"reason"`
	TraceID               string    `json:"trace_id"`
	OccurredAt            time.Time `json:"occurred_at"`
}

func (command SourceSyncCommand) Validate() error {
	command.TenantID = strings.TrimSpace(command.TenantID)
	command.JobID = strings.TrimSpace(command.JobID)
	command.SourceID = strings.TrimSpace(command.SourceID)
	command.SourceKind = strings.TrimSpace(command.SourceKind)
	if command.EventType != SourceSyncEventType || command.SchemaVersion != 1 ||
		command.AggregateType != "source_sync_job" || command.AggregateVersion < 1 ||
		command.EventID == "" || command.JobID == "" || command.AggregateID != command.JobID ||
		command.TenantID == "" || command.PartitionKey != command.TenantID+":"+command.JobID ||
		command.RequestedBy == "" || strings.TrimSpace(command.Reason) == "" ||
		strings.TrimSpace(command.TraceID) == "" || command.WindowStart.IsZero() ||
		command.WindowEnd.IsZero() || !command.WindowStart.Before(command.WindowEnd) ||
		command.OccurredAt.IsZero() {
		return fmt.Errorf("%w: incomplete event envelope", ErrInvalidCommand)
	}
	if expectedKind, ok := sourceKinds[command.SourceID]; !ok || expectedKind != command.SourceKind {
		return fmt.Errorf("%w: unsupported source identity", ErrInvalidCommand)
	}
	if command.ExpectedSourceVersion != nil && *command.ExpectedSourceVersion < 0 {
		return fmt.Errorf("%w: expected_source_version cannot be negative", ErrInvalidCommand)
	}
	if command.WindowEnd.Sub(command.WindowStart) > 31*24*time.Hour {
		return fmt.Errorf("%w: source window exceeds 31 days", ErrInvalidCommand)
	}
	return nil
}

type KafkaPosition struct {
	Topic     string
	Partition int
	Offset    int64
}

type SourceFact struct {
	AggregateID         string
	EventID             string
	EventTime           time.Time
	SourceTopic         string
	SourcePartition     int
	SourceOffset        int64
	SourcePayloadSHA256 string
	SourceVersion       uint64
	ProjectionIdentity  string
	PayloadBase64       string
	ProjectionHash      string
}

type SourceFactBatch struct {
	Facts     []SourceFact
	Truncated bool
	Total     int64
}

type SourceFactReader interface {
	ReadSourceFacts(context.Context, string, string, time.Time, time.Time, int) (SourceFactBatch, error)
}

type SourceEntityFact struct {
	SourceEntityID   string
	EntityKind       string
	Identifiers      map[string]string
	EvidenceEventIDs []string
	Provenance       map[string]interface{}
}

type SourceRelationFact struct {
	SourceRelationID string
	SourceEntityID   string
	TargetEntityID   string
	RelationKind     string
	EventTime        time.Time
	EvidenceEventIDs []string
	Provenance       map[string]interface{}
}

type BoundSourceEntityFact struct {
	SourceID         string
	SourceSnapshotID string
	Fact             SourceEntityFact
}

type BoundSourceRelationFact struct {
	SourceID         string
	SourceSnapshotID string
	Fact             SourceRelationFact
}

type CanonicalEntity struct {
	EntityID         string
	EntityKind       string
	Identifiers      map[string]string
	SourceEntityRefs []string
	SourceCount      int
	Confidence       float64
	Provenance       map[string]interface{}
}

type CanonicalRelation struct {
	RelationID     string
	SourceEntityID string
	TargetEntityID string
	RelationKind   string
	EdgeOrigin     string
	EventTime      time.Time
	Confidence     float64
	EvidenceRefs   []string
	Provenance     map[string]interface{}
}

type FeatureMetric struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Semantics string  `json:"semantics"`
}

type AblationResult struct {
	OmittedSourceID     string `json:"omitted_source_id"`
	Status              string `json:"status"`
	IncludedSourceCount int    `json:"included_source_count"`
	EntityCount         int    `json:"entity_count"`
	RelationCount       int    `json:"relation_count"`
	EntityDelta         int    `json:"entity_delta"`
	RelationDelta       int    `json:"relation_delta"`
	CanonicalSHA256     string `json:"canonical_sha256"`
}

type ProjectionReceipt struct {
	EventID           string `json:"event_id"`
	JobID             string `json:"job_id"`
	SourceSnapshotID  string `json:"source_snapshot_id"`
	DataSnapshotID    string `json:"data_snapshot_id"`
	FeatureSnapshotID string `json:"feature_snapshot_id"`
	SourceVersion     int64  `json:"source_version"`
	DataVersion       int64  `json:"data_version"`
	FeatureVersion    int64  `json:"feature_version"`
	Disposition       string `json:"disposition"`
	FailureCode       string `json:"failure_code,omitempty"`
	QualityStatus     string `json:"quality_status"`
	Replayed          bool   `json:"replayed"`
}

func SourceKind(sourceID string) (string, bool) {
	kind, ok := sourceKinds[strings.TrimSpace(sourceID)]
	return kind, ok
}

func RequiredSourceIDs() []string {
	result := append([]string(nil), requiredSourceIDs...)
	sort.Strings(result)
	return result
}

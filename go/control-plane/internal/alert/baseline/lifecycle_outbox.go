package baseline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

type LifecyclePublisher interface {
	Send(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error)
}

type lifecycleOutboxItem struct {
	Sequence         int64
	EventID          string
	TenantID         string
	BaselineID       string
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	Payload          []byte
	PayloadSHA256    string
	TraceID          string
	ClaimToken       string
}

type lifecycleEnvelope struct {
	EventID         string                 `json:"event_id"`
	EventType       string                 `json:"event_type"`
	SchemaVersion   int64                  `json:"schema_version"`
	PartitionKey    string                 `json:"partition_key"`
	TenantID        string                 `json:"tenant_id"`
	BaselineID      string                 `json:"baseline_id"`
	BaselineVersion int64                  `json:"baseline_version,omitempty"`
	TargetVersion   int64                  `json:"target_version,omitempty"`
	RetiredBy       int64                  `json:"retired_by_version,omitempty"`
	JobID           string                 `json:"job_id,omitempty"`
	VersionID       string                 `json:"version_id,omitempty"`
	BaselineKind    string                 `json:"baseline_kind,omitempty"`
	Algorithm       string                 `json:"algorithm_version,omitempty"`
	CandidateSHA256 string                 `json:"candidate_sha256"`
	SnapshotSHA256  string                 `json:"snapshot_sha256,omitempty"`
	QualityStatus   string                 `json:"quality_status,omitempty"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	Expected        []string               `json:"expected_consumers,omitempty"`
	Acked           []string               `json:"acked_consumers,omitempty"`
	Thresholds      map[string]interface{} `json:"threshold_spec,omitempty"`
	Statistics      map[string]interface{} `json:"statistics,omitempty"`
	TraceID         string                 `json:"trace_id"`
}

type LifecycleOutboxDispatcher struct {
	db              *sql.DB
	publisher       LifecyclePublisher
	readiness       *AckReadinessStore
	candidateSHA256 string
	now             func() time.Time
}

func NewLifecycleOutboxDispatcher(
	db *sql.DB,
	publisher LifecyclePublisher,
	readiness *AckReadinessStore,
	candidateSHA256 string,
) (*LifecycleOutboxDispatcher, error) {
	candidateSHA256 = strings.TrimSpace(candidateSHA256)
	if db == nil || publisher == nil || readiness == nil || !sha256Pattern.MatchString(candidateSHA256) {
		return nil, fmt.Errorf("behavior baseline lifecycle dispatcher dependencies are required")
	}
	return &LifecycleOutboxDispatcher{db: db, publisher: publisher, readiness: readiness,
		candidateSHA256: candidateSHA256, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (dispatcher *LifecycleOutboxDispatcher) Drain(ctx context.Context, limit int) (int, error) {
	if dispatcher == nil || dispatcher.db == nil || dispatcher.publisher == nil || dispatcher.readiness == nil {
		return 0, fmt.Errorf("behavior baseline lifecycle dispatcher is unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tx, err := dispatcher.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin behavior baseline lifecycle claim: %w", err)
	}
	defer tx.Rollback()
	if err := dispatcher.readiness.AssertReadyTx(ctx, tx, dispatcher.candidateSHA256); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `WITH candidates AS (
		SELECT o.event_id FROM behavior_baseline_lifecycle_outbox_v1 o
		WHERE o.next_attempt_at<=now()
		  AND (o.publish_state='PENDING' OR (o.publish_state='OUTCOME_UNKNOWN' AND o.claimed_at<now()-interval '30 seconds'))
		  AND NOT EXISTS (
			SELECT 1 FROM behavior_baseline_lifecycle_outbox_v1 prior
			WHERE prior.partition_key=o.partition_key AND prior.outbox_sequence<o.outbox_sequence
			  AND prior.publish_state<>'KAFKA_ACKED'
		  )
		ORDER BY o.next_attempt_at,o.outbox_sequence LIMIT $1 FOR UPDATE OF o SKIP LOCKED
	), claimed AS (
		UPDATE behavior_baseline_lifecycle_outbox_v1 o SET publish_state='OUTCOME_UNKNOWN',
			claim_token=uuid_generate_v4(),claimed_at=now(),attempts=attempts+1,
			next_attempt_at=now()+interval '30 seconds',last_error=''
		FROM candidates c WHERE o.event_id=c.event_id
		RETURNING o.outbox_sequence,o.event_id::text,o.tenant_id,o.baseline_id,o.aggregate_type,o.aggregate_id,
			o.aggregate_version,o.event_type,o.partition_key,o.payload::text,o.payload_sha256,o.trace_id,
			o.claim_token::text
	) SELECT outbox_sequence,event_id,tenant_id,baseline_id,aggregate_type,aggregate_id,aggregate_version,event_type,
		partition_key,payload,payload_sha256,trace_id,claim_token FROM claimed ORDER BY outbox_sequence`, limit)
	if err != nil {
		return 0, fmt.Errorf("claim behavior baseline lifecycle outbox: %w", err)
	}
	items := make([]lifecycleOutboxItem, 0, limit)
	for rows.Next() {
		var item lifecycleOutboxItem
		var payload string
		if err := rows.Scan(&item.Sequence, &item.EventID, &item.TenantID, &item.BaselineID, &item.AggregateType,
			&item.AggregateID, &item.AggregateVersion, &item.EventType, &item.PartitionKey, &payload,
			&item.PayloadSHA256, &item.TraceID, &item.ClaimToken); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan behavior baseline lifecycle outbox: %w", err)
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close behavior baseline lifecycle outbox rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit behavior baseline lifecycle claims: %w", err)
	}
	published := 0
	for _, item := range items {
		if err := dispatcher.publish(ctx, item); err == nil {
			published++
		}
	}
	return published, nil
}

func (dispatcher *LifecycleOutboxDispatcher) publish(ctx context.Context, item lifecycleOutboxItem) error {
	payload, envelope, err := validateLifecycleOutboxItem(item, dispatcher.candidateSHA256)
	if err != nil {
		dispatcher.release(ctx, item, "INVALID_OUTBOX_PAYLOAD")
		return err
	}
	receipt, err := dispatcher.publisher.Send(ctx, item.PartitionKey, payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: "1"},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "baseline_id", Value: item.BaselineID},
		commonkafka.MessageHeader{Key: "baseline_version", Value: strconv.FormatInt(envelope.BaselineVersion, 10)},
		commonkafka.MessageHeader{Key: "candidate_sha256", Value: envelope.CandidateSHA256},
		commonkafka.MessageHeader{Key: "snapshot_sha256", Value: envelope.SnapshotSHA256},
		commonkafka.MessageHeader{Key: "aggregate_type", Value: item.AggregateType},
		commonkafka.MessageHeader{Key: "aggregate_id", Value: item.AggregateID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: strconv.FormatInt(item.AggregateVersion, 10)},
		commonkafka.MessageHeader{Key: "trace_id", Value: item.TraceID},
		commonkafka.MessageHeader{Key: "target_topic", Value: LifecycleTopic},
		commonkafka.MessageHeader{Key: commonkafka.PublishAttemptHeader, Value: item.ClaimToken},
	)
	if err != nil {
		var unknown *commonkafka.PublishOutcomeUnknownError
		if !errors.As(err, &unknown) {
			dispatcher.release(ctx, item, err.Error())
		}
		return err
	}
	if receipt.AttemptID != item.ClaimToken || receipt.Topic != LifecycleTopic || receipt.Partition < 0 ||
		receipt.Offset < 0 || receipt.Key != item.PartitionKey || receipt.AcknowledgedAt.IsZero() {
		return fmt.Errorf("behavior baseline lifecycle broker receipt identity is invalid")
	}
	result, err := dispatcher.db.ExecContext(ctx, `UPDATE behavior_baseline_lifecycle_outbox_v1
		SET publish_state='KAFKA_ACKED',broker_topic=$1,broker_partition=$2,broker_offset=$3,acked_at=$4,
			claim_token=NULL,claimed_at=NULL,next_attempt_at=now(),last_error=''
		WHERE event_id=$5 AND publish_state='OUTCOME_UNKNOWN' AND claim_token=$6`, receipt.Topic,
		receipt.Partition, receipt.Offset, receipt.AcknowledgedAt, item.EventID, item.ClaimToken)
	if err != nil {
		return fmt.Errorf("record behavior baseline lifecycle broker acknowledgement: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("behavior baseline lifecycle claim was lost before broker acknowledgement")
	}
	return nil
}

func validateLifecycleOutboxItem(item lifecycleOutboxItem, candidateSHA256 string) ([]byte, lifecycleEnvelope, error) {
	var envelope lifecycleEnvelope
	var canonicalValue interface{}
	if item.EventID == "" || item.TenantID == "" || item.BaselineID == "" || item.AggregateID == "" ||
		item.AggregateVersion <= 0 || item.ClaimToken == "" || item.PartitionKey != item.TenantID+":"+item.BaselineID ||
		item.TraceID == "" || !allowedLifecycleEventType(item.EventType) ||
		json.Unmarshal(item.Payload, &canonicalValue) != nil {
		return nil, envelope, fmt.Errorf("behavior baseline lifecycle outbox identity is invalid")
	}
	canonicalPayload, err := json.Marshal(canonicalValue)
	if err != nil {
		return nil, envelope, fmt.Errorf("canonicalize behavior baseline lifecycle payload: %w", err)
	}
	hash := sha256.Sum256(canonicalPayload)
	if hex.EncodeToString(hash[:]) != item.PayloadSHA256 || json.Unmarshal(canonicalPayload, &envelope) != nil ||
		envelope.EventID != item.EventID || envelope.EventType != item.EventType || envelope.SchemaVersion != 1 ||
		envelope.PartitionKey != item.PartitionKey || envelope.TenantID != item.TenantID ||
		envelope.BaselineID != item.BaselineID || envelope.TraceID != item.TraceID ||
		envelope.CandidateSHA256 != candidateSHA256 {
		return nil, envelope, fmt.Errorf("behavior baseline lifecycle payload does not match its outbox row")
	}
	switch item.EventType {
	case "baseline.build.requested.v1":
		if item.AggregateType != "baseline_build_job" || envelope.JobID != item.AggregateID ||
			envelope.TargetVersion != item.AggregateVersion || envelope.BaselineKind == "" {
			return nil, envelope, fmt.Errorf("behavior baseline build payload is incomplete")
		}
	case "baseline.version.failed.v1":
		failedBuild := item.AggregateType == "baseline_build_job" && envelope.JobID == item.AggregateID &&
			envelope.TargetVersion == item.AggregateVersion
		failedVersion := item.AggregateType == "baseline_version" && envelope.VersionID == item.AggregateID &&
			envelope.BaselineVersion == item.AggregateVersion && sha256Pattern.MatchString(envelope.SnapshotSHA256)
		if (!failedBuild && !failedVersion) || envelope.ErrorCode == "" {
			return nil, envelope, fmt.Errorf("behavior baseline failure payload is incomplete")
		}
	case "baseline.version.frozen.v1":
		if item.AggregateType != "baseline_version" || envelope.VersionID != item.AggregateID ||
			envelope.BaselineVersion != item.AggregateVersion || envelope.QualityStatus != "complete" ||
			envelope.BaselineKind == "" || !sha256Pattern.MatchString(envelope.SnapshotSHA256) {
			return nil, envelope, fmt.Errorf("behavior baseline frozen payload is incomplete")
		}
	case "baseline.activation.requested.v1":
		if item.AggregateType != "baseline_version" || envelope.BaselineVersion != item.AggregateVersion ||
			!sha256Pattern.MatchString(envelope.SnapshotSHA256) || len(uniqueSorted(envelope.Expected)) == 0 ||
			envelope.BaselineKind == "" || envelope.Algorithm == "" || envelope.Thresholds == nil ||
			envelope.Statistics == nil {
			return nil, envelope, fmt.Errorf("behavior baseline activation payload is incomplete")
		}
	case "baseline.version.activated.v1":
		if item.AggregateType != "baseline_version" || envelope.BaselineVersion != item.AggregateVersion ||
			!sha256Pattern.MatchString(envelope.SnapshotSHA256) || len(uniqueSorted(envelope.Acked)) == 0 {
			return nil, envelope, fmt.Errorf("behavior baseline activated payload is incomplete")
		}
	case "baseline.version.retired.v1":
		if item.AggregateType != "baseline_version" || envelope.BaselineVersion != item.AggregateVersion ||
			!sha256Pattern.MatchString(envelope.SnapshotSHA256) || envelope.RetiredBy <= 0 ||
			envelope.RetiredBy == envelope.BaselineVersion {
			return nil, envelope, fmt.Errorf("behavior baseline retired payload is incomplete")
		}
	}
	return canonicalPayload, envelope, nil
}

func allowedLifecycleEventType(eventType string) bool {
	switch eventType {
	case "baseline.build.requested.v1", "baseline.version.frozen.v1", "baseline.version.failed.v1",
		"baseline.activation.requested.v1", "baseline.version.activated.v1", "baseline.version.retired.v1":
		return true
	default:
		return false
	}
}

func (dispatcher *LifecycleOutboxDispatcher) release(ctx context.Context, item lifecycleOutboxItem, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = dispatcher.db.ExecContext(ctx, `UPDATE behavior_baseline_lifecycle_outbox_v1
		SET publish_state='PENDING',claim_token=NULL,claimed_at=NULL,next_attempt_at=now()+interval '10 seconds',last_error=$1
		WHERE event_id=$2 AND publish_state='OUTCOME_UNKNOWN' AND claim_token=$3`, message, item.EventID, item.ClaimToken)
}

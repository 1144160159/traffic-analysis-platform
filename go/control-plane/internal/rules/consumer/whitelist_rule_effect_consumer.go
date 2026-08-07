package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const DefaultWhitelistEventTopicV2 = "whitelist.events.v2"

type WhitelistRuleProjectionInput struct {
	EventID        string
	TenantID       string
	EntryID        string
	EntryVersion   int64
	DesiredState   string
	EntryType      string
	MatchValue     string
	Scope          string
	ExpiresAt      *time.Time
	RuleRevision   string
	AckEventID     string
	PayloadSHA256  string
	KafkaPartition int
	KafkaOffset    int64
}

type WhitelistRuleProjectionApplier interface {
	ApplyWhitelistRuleProjection(context.Context, WhitelistRuleProjectionInput) error
}

type WhitelistRuleEffectConsumer struct {
	consumer *commonkafka.Consumer
	applier  WhitelistRuleProjectionApplier
	topic    string
	logger   *zap.Logger
}

func NewWhitelistRuleEffectConsumer(consumer *commonkafka.Consumer, applier WhitelistRuleProjectionApplier,
	topic string, logger *zap.Logger) (*WhitelistRuleEffectConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("whitelist event consumer and rule projection applier are required")
	}
	if strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("whitelist event topic is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WhitelistRuleEffectConsumer{consumer: consumer, applier: applier, topic: topic, logger: logger}, nil
}

func (c *WhitelistRuleEffectConsumer) Start(ctx context.Context) error {
	return c.consumer.Consume(ctx, c.handle)
}

func (c *WhitelistRuleEffectConsumer) Close() error { return c.consumer.Close() }

type whitelistLifecycleEventV2 struct {
	EventID          string                `json:"event_id"`
	EventType        string                `json:"event_type"`
	SchemaVersion    int                   `json:"schema_version"`
	TenantID         string                `json:"tenant_id"`
	EntryID          string                `json:"entry_id"`
	AggregateVersion int64                 `json:"aggregate_version"`
	ActionID         string                `json:"action_id"`
	Reason           string                `json:"reason"`
	TraceID          string                `json:"trace_id"`
	DesiredRuleState string                `json:"desired_rule_state"`
	OccurredAt       time.Time             `json:"occurred_at"`
	Entry            whitelistEventEntryV2 `json:"entry"`
}

type whitelistEventEntryV2 struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Type           string     `json:"type"`
	Value          string     `json:"value"`
	Status         string     `json:"status"`
	ApprovalStatus string     `json:"approval_status"`
	Scope          string     `json:"scope"`
	Version        int64      `json:"version"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ArchivedAt     *time.Time `json:"archived_at"`
}

func (c *WhitelistRuleEffectConsumer) handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return fmt.Errorf("whitelist Kafka message is nil")
	}
	if message.Topic != c.topic {
		return fmt.Errorf("whitelist Kafka topic mismatch")
	}
	event, err := decodeWhitelistLifecycleEvent(message.Value)
	if err != nil {
		return err
	}
	if err := validateWhitelistLifecycleMessage(event, message); err != nil {
		return err
	}
	if event.DesiredRuleState == "" {
		return nil
	}
	payloadDigest := sha256.Sum256(message.Value)
	expiresIdentity := ""
	if event.Entry.ExpiresAt != nil {
		expiresIdentity = event.Entry.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	revisionDigest := sha256.Sum256([]byte(strings.Join([]string{
		event.EventID, fmt.Sprint(event.AggregateVersion), event.DesiredRuleState,
		event.Entry.Type, event.Entry.Value, event.Entry.Scope, expiresIdentity,
	}, "\x00")))
	input := WhitelistRuleProjectionInput{
		EventID: event.EventID, TenantID: event.TenantID, EntryID: event.EntryID,
		EntryVersion: event.AggregateVersion, DesiredState: event.DesiredRuleState,
		EntryType: event.Entry.Type, MatchValue: event.Entry.Value, Scope: event.Entry.Scope,
		ExpiresAt: event.Entry.ExpiresAt, RuleRevision: hex.EncodeToString(revisionDigest[:]),
		AckEventID:    uuid.NewSHA1(uuid.NameSpaceURL, []byte("whitelist-rule-ack\x00"+event.EventID)).String(),
		PayloadSHA256: hex.EncodeToString(payloadDigest[:]), KafkaPartition: message.Partition,
		KafkaOffset: message.Offset,
	}
	if err := c.applier.ApplyWhitelistRuleProjection(ctx, input); err != nil {
		return fmt.Errorf("apply whitelist rule projection %s: %w", event.EventID, err)
	}
	if c.logger != nil {
		c.logger.Info("Whitelist rule projection applied", zap.String("event_id", event.EventID),
			zap.String("entry_id", event.EntryID), zap.String("tenant_id", event.TenantID),
			zap.String("desired_state", event.DesiredRuleState), zap.Int64("kafka_offset", message.Offset))
	}
	return nil
}

func decodeWhitelistLifecycleEvent(payload []byte) (whitelistLifecycleEventV2, error) {
	var event whitelistLifecycleEventV2
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	if err := decoder.Decode(&event); err != nil {
		return event, fmt.Errorf("decode whitelist event: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return event, fmt.Errorf("decode whitelist event: multiple JSON values")
		}
		return event, fmt.Errorf("decode whitelist event trailing data: %w", err)
	}
	return event, nil
}

func validateWhitelistLifecycleMessage(event whitelistLifecycleEventV2, message *commonkafka.ReceivedMessage) error {
	if event.SchemaVersion != 2 || event.AggregateVersion < 1 || event.OccurredAt.IsZero() {
		return fmt.Errorf("unsupported whitelist event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid whitelist event_id")
	}
	if _, err := uuid.Parse(event.EntryID); err != nil {
		return fmt.Errorf("invalid whitelist entry_id")
	}
	if event.Entry.ID != event.EntryID || event.Entry.TenantID != event.TenantID ||
		event.Entry.Version != event.AggregateVersion || strings.TrimSpace(event.TenantID) == "" ||
		strings.TrimSpace(event.ActionID) == "" || strings.TrimSpace(event.Reason) == "" ||
		strings.TrimSpace(event.TraceID) == "" || strings.TrimSpace(event.Entry.Value) == "" ||
		!validWhitelistEntryType(event.Entry.Type) {
		return fmt.Errorf("incomplete whitelist event contract")
	}
	if string(message.Key) != event.TenantID+":"+event.EntryID {
		return fmt.Errorf("whitelist partition key/body mismatch")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType, "schema_version": "2",
		"aggregate_version": fmt.Sprint(event.AggregateVersion), "tenant_id": event.TenantID,
		"entry_id": event.EntryID, "action_id": event.ActionID,
		"desired_rule_state": event.DesiredRuleState, "trace_id": event.TraceID,
	}
	for key, expected := range expectedHeaders {
		if message.GetHeader(key) != expected {
			return fmt.Errorf("whitelist event %s header/body mismatch", key)
		}
	}
	expectedDesired, err := expectedWhitelistDesiredState(event)
	if err != nil {
		return err
	}
	if event.DesiredRuleState != expectedDesired {
		return fmt.Errorf("whitelist desired rule state does not match lifecycle state")
	}
	return nil
}

func expectedWhitelistDesiredState(event whitelistLifecycleEventV2) (string, error) {
	effective := event.Entry.Status == "active" && event.Entry.ApprovalStatus == "approved" &&
		event.Entry.ArchivedAt == nil && (event.Entry.ExpiresAt == nil || event.Entry.ExpiresAt.After(event.OccurredAt))
	switch event.EventType {
	case "traffic.whitelist.v2.EntryApproved":
		if !effective {
			return "", fmt.Errorf("approved whitelist event is not effective")
		}
		return "effective", nil
	case "traffic.whitelist.v2.EntryRevoked", "traffic.whitelist.v2.EntryRejected",
		"traffic.whitelist.v2.EntryArchived", "traffic.whitelist.v2.EntryExpired":
		if effective {
			return "", fmt.Errorf("revoked whitelist event remains effective")
		}
		return "revoked", nil
	case "traffic.whitelist.v2.EntryUpdated", "traffic.whitelist.v2.EntryExtended",
		"traffic.whitelist.v2.EntryAssigned":
		if effective {
			return "effective", nil
		}
		return "", nil
	case "traffic.whitelist.v2.EntryDrafted", "traffic.whitelist.v2.ApprovalSubmitted":
		return "", nil
	default:
		return "", fmt.Errorf("unsupported whitelist event type %q", event.EventType)
	}
}

func validWhitelistEntryType(value string) bool {
	switch value {
	case "ip", "domain", "fingerprint", "subnet", "asset", "account", "rule", "model":
		return true
	default:
		return false
	}
}

type PostgresWhitelistRuleProjection struct{ db *sql.DB }

func NewPostgresWhitelistRuleProjection(db *sql.DB) (*PostgresWhitelistRuleProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("whitelist rule projection database is required")
	}
	return &PostgresWhitelistRuleProjection{db: db}, nil
}

func (p *PostgresWhitelistRuleProjection) VerifySchema(ctx context.Context) error {
	var columns int
	if err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
		  (table_name='whitelist_rule_projection' AND column_name IN
		    ('tenant_id','entry_id','entry_version','source_event_id','desired_state','entry_type',
		     'match_value','scope','expires_at','rule_revision','payload_sha256','kafka_partition',
		     'kafka_offset','applied_at'))
		  OR (table_name='whitelist_rule_effects' AND column_name IN
		    ('tenant_id','entry_id','entry_version','event_id','desired_state','status',
		     'rule_revision','ack_event_id','last_error','acknowledged_at'))
		)`).Scan(&columns); err != nil {
		return fmt.Errorf("verify whitelist rule projection schema: %w", err)
	}
	if columns != 24 {
		return fmt.Errorf("whitelist rule projection schema incomplete: columns=%d want=24", columns)
	}
	return nil
}

func (p *PostgresWhitelistRuleProjection) ApplyWhitelistRuleProjection(ctx context.Context, input WhitelistRuleProjectionInput) error {
	if input.DesiredState != "effective" && input.DesiredState != "revoked" {
		return fmt.Errorf("invalid whitelist desired state %q", input.DesiredState)
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin whitelist rule projection: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO whitelist_rule_projection
		(tenant_id,entry_id,entry_version,source_event_id,desired_state,entry_type,match_value,
		 scope,expires_at,rule_revision,payload_sha256,kafka_partition,kafka_offset)
		VALUES ($1,$2::uuid,$3,$4::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (tenant_id,entry_id) DO UPDATE SET
		 entry_version=EXCLUDED.entry_version,source_event_id=EXCLUDED.source_event_id,
		 desired_state=EXCLUDED.desired_state,entry_type=EXCLUDED.entry_type,
		 match_value=EXCLUDED.match_value,scope=EXCLUDED.scope,expires_at=EXCLUDED.expires_at,
		 rule_revision=EXCLUDED.rule_revision,payload_sha256=EXCLUDED.payload_sha256,
		 kafka_partition=EXCLUDED.kafka_partition,kafka_offset=EXCLUDED.kafka_offset,applied_at=now()
		WHERE EXCLUDED.entry_version > whitelist_rule_projection.entry_version
		   OR (EXCLUDED.entry_version = whitelist_rule_projection.entry_version
		       AND EXCLUDED.source_event_id = whitelist_rule_projection.source_event_id
		       AND EXCLUDED.payload_sha256 = whitelist_rule_projection.payload_sha256)`,
		input.TenantID, input.EntryID, input.EntryVersion, input.EventID, input.DesiredState,
		input.EntryType, input.MatchValue, input.Scope, input.ExpiresAt, input.RuleRevision,
		input.PayloadSHA256, input.KafkaPartition, input.KafkaOffset)
	if err != nil {
		return fmt.Errorf("upsert whitelist rule projection: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect whitelist rule projection: %w", err)
	}
	if affected == 0 {
		var currentVersion int64
		var currentEvent, currentDigest string
		if err := tx.QueryRowContext(ctx, `SELECT entry_version,source_event_id::text,payload_sha256
			FROM whitelist_rule_projection WHERE tenant_id=$1 AND entry_id=$2::uuid`,
			input.TenantID, input.EntryID).Scan(&currentVersion, &currentEvent, &currentDigest); err != nil {
			return fmt.Errorf("inspect whitelist projection collision: %w", err)
		}
		if currentVersion < input.EntryVersion ||
			(currentVersion == input.EntryVersion && (currentEvent != input.EventID || currentDigest != input.PayloadSHA256)) {
			return fmt.Errorf("whitelist rule projection identity collision")
		}
	}
	result, err = tx.ExecContext(ctx, `UPDATE whitelist_rule_effects
		SET status='applied',ack_event_id=$6,rule_revision=$7,last_error='',acknowledged_at=now()
		WHERE tenant_id=$1 AND entry_id=$2::uuid AND entry_version=$3
		  AND event_id=$4::uuid AND desired_state=$5 AND status='pending'`,
		input.TenantID, input.EntryID, input.EntryVersion, input.EventID, input.DesiredState,
		input.AckEventID, input.RuleRevision)
	if err != nil {
		return fmt.Errorf("acknowledge whitelist rule effect: %w", err)
	}
	acknowledged, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect whitelist rule effect acknowledgement: %w", err)
	}
	if acknowledged == 0 {
		var status, ackEventID, revision string
		err = tx.QueryRowContext(ctx, `SELECT status,ack_event_id,rule_revision
			FROM whitelist_rule_effects WHERE tenant_id=$1 AND entry_id=$2::uuid
			  AND entry_version=$3 AND event_id=$4::uuid AND desired_state=$5`,
			input.TenantID, input.EntryID, input.EntryVersion, input.EventID, input.DesiredState).
			Scan(&status, &ackEventID, &revision)
		if err != nil {
			return fmt.Errorf("verify whitelist rule effect acknowledgement: %w", err)
		}
		if status != "applied" || ackEventID != input.AckEventID || revision != input.RuleRevision {
			return fmt.Errorf("whitelist rule effect acknowledgement conflict")
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit whitelist rule projection: %w", err)
	}
	return nil
}

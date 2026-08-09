package whitelist

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const WhitelistEventTopicV2 = "whitelist.events.v2"

type EventPublisher interface {
	Send(context.Context, string, []byte, ...commonkafka.MessageHeader) error
}

type OutboxDispatcherConfig struct {
	WorkerID    string
	Lease       time.Duration
	MaxAttempts int
	BatchSize   int
	Interval    time.Duration
	Logger      *zap.Logger
}

type OutboxDispatcher struct {
	db        *sql.DB
	publisher EventPublisher
	cfg       OutboxDispatcherConfig
}

type lifecycleEventV2 struct {
	EventID          string          `json:"event_id"`
	EventType        string          `json:"event_type"`
	SchemaVersion    int             `json:"schema_version"`
	TenantID         string          `json:"tenant_id"`
	EntryID          string          `json:"entry_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	ActionID         string          `json:"action_id"`
	Reason           string          `json:"reason"`
	TraceID          string          `json:"trace_id"`
	DesiredRuleState string          `json:"desired_rule_state"`
	OccurredAt       time.Time       `json:"occurred_at"`
	Entry            json.RawMessage `json:"entry"`
}

func NewOutboxDispatcher(db *sql.DB, publisher EventPublisher, cfg OutboxDispatcherConfig) (*OutboxDispatcher, error) {
	if db == nil || publisher == nil {
		return nil, fmt.Errorf("whitelist outbox database and publisher are required")
	}
	if strings.TrimSpace(cfg.WorkerID) == "" {
		return nil, fmt.Errorf("whitelist outbox worker_id is required")
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 30 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	return &OutboxDispatcher{db: db, publisher: publisher, cfg: cfg}, nil
}

func (d *OutboxDispatcher) VerifySchema(ctx context.Context) error {
	var columns int
	if err := d.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='whitelist_event_outbox'
		  AND column_name IN ('outbox_id','event_id','tenant_id','entry_id','aggregate_version',
		    'event_type','schema_version','partition_key','payload','trace_id','status',
		    'attempt_count','available_at','locked_until','locked_by','last_error',
		    'occurred_at','published_at')`).Scan(&columns); err != nil {
		return fmt.Errorf("verify whitelist outbox schema: %w", err)
	}
	if columns != 18 {
		return fmt.Errorf("whitelist outbox schema incomplete: columns=%d want=18", columns)
	}
	return nil
}

func (d *OutboxDispatcher) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			for processed := 0; processed < d.cfg.BatchSize; processed++ {
				found, err := d.DispatchNext(ctx)
				if err != nil {
					d.cfg.Logger.Warn("Whitelist outbox dispatch failed", zap.Error(err))
					break
				}
				if !found {
					break
				}
			}
			timer.Reset(d.cfg.Interval)
		}
	}
}

func (d *OutboxDispatcher) DispatchNext(ctx context.Context) (bool, error) {
	var (
		outboxID, aggregateVersion       int64
		eventID, tenantID, entryID       string
		eventType, partitionKey, traceID string
		desiredState                     string
		schemaVersion, attemptCount      int
		payload                          []byte
	)
	leaseSeconds := int64(d.cfg.Lease.Seconds())
	if leaseSeconds < 1 {
		leaseSeconds = 1
	}
	err := d.db.QueryRowContext(ctx, `
		WITH candidate AS (
		  SELECT outbox_id FROM whitelist_event_outbox
		  WHERE (status='pending' AND available_at<=now())
		     OR (status='processing' AND locked_until<now())
		  ORDER BY available_at,occurred_at,outbox_id
		  FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE whitelist_event_outbox outbox
		SET status='processing',attempt_count=outbox.attempt_count+1,
		    locked_by=$1,locked_until=now()+($2*interval '1 second')
		FROM candidate WHERE outbox.outbox_id=candidate.outbox_id
		RETURNING outbox.outbox_id,outbox.event_id::text,outbox.tenant_id,
		  outbox.entry_id::text,outbox.aggregate_version,outbox.event_type,
		  outbox.schema_version,outbox.partition_key,outbox.trace_id,outbox.payload::text,
		  outbox.attempt_count,COALESCE((SELECT effect.desired_state
		    FROM whitelist_rule_effects effect WHERE effect.event_id=outbox.event_id),'')`,
		d.cfg.WorkerID, leaseSeconds,
	).Scan(&outboxID, &eventID, &tenantID, &entryID, &aggregateVersion, &eventType,
		&schemaVersion, &partitionKey, &traceID, &payload, &attemptCount, &desiredState)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lease whitelist outbox: %w", err)
	}

	event, err := decodeLifecycleEvent(payload)
	if err != nil {
		return true, d.recordFailure(ctx, outboxID, attemptCount, err)
	}
	if err := validateLifecycleEvent(event, eventID, tenantID, entryID, eventType, partitionKey,
		traceID, desiredState, aggregateVersion, schemaVersion); err != nil {
		return true, d.recordFailure(ctx, outboxID, attemptCount, err)
	}

	if err := d.publisher.Send(ctx, partitionKey, payload,
		commonkafka.MessageHeader{Key: "event_id", Value: eventID},
		commonkafka.MessageHeader{Key: "event_type", Value: eventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: strconv.Itoa(schemaVersion)},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: strconv.FormatInt(aggregateVersion, 10)},
		commonkafka.MessageHeader{Key: "tenant_id", Value: tenantID},
		commonkafka.MessageHeader{Key: "entry_id", Value: entryID},
		commonkafka.MessageHeader{Key: "action_id", Value: event.ActionID},
		commonkafka.MessageHeader{Key: "desired_rule_state", Value: desiredState},
		commonkafka.MessageHeader{Key: "trace_id", Value: traceID},
	); err != nil {
		return true, d.recordFailure(ctx, outboxID, attemptCount, err)
	}

	result, err := d.db.ExecContext(ctx, `UPDATE whitelist_event_outbox
		SET status='published',published_at=now(),locked_by='',locked_until=NULL,last_error=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, outboxID, d.cfg.WorkerID)
	if err != nil {
		return true, fmt.Errorf("mark whitelist outbox published: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return true, fmt.Errorf("whitelist outbox lease lost after Kafka acknowledgement")
	}
	return true, nil
}

func decodeLifecycleEvent(payload []byte) (lifecycleEventV2, error) {
	var event lifecycleEventV2
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
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

func validateLifecycleEvent(event lifecycleEventV2, eventID, tenantID, entryID, eventType,
	partitionKey, traceID, desiredState string, aggregateVersion int64, schemaVersion int) error {
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid whitelist event_id")
	}
	if _, err := uuid.Parse(event.EntryID); err != nil {
		return fmt.Errorf("invalid whitelist entry_id")
	}
	if event.EventID != eventID || event.TenantID != tenantID || event.EntryID != entryID ||
		event.EventType != eventType || event.AggregateVersion != aggregateVersion ||
		event.SchemaVersion != schemaVersion || event.TraceID != traceID ||
		event.DesiredRuleState != desiredState || partitionKey != tenantID+":"+entryID {
		return fmt.Errorf("whitelist outbox envelope does not match authoritative columns")
	}
	if schemaVersion != 2 || aggregateVersion < 1 || strings.TrimSpace(tenantID) == "" ||
		strings.TrimSpace(event.ActionID) == "" || strings.TrimSpace(event.Reason) == "" ||
		strings.TrimSpace(traceID) == "" || event.OccurredAt.IsZero() || !json.Valid(event.Entry) {
		return fmt.Errorf("incomplete whitelist event contract")
	}
	if !knownWhitelistEventType(eventType) {
		return fmt.Errorf("unsupported whitelist event type %q", eventType)
	}
	if desiredState != "" && desiredState != "effective" && desiredState != "revoked" {
		return fmt.Errorf("invalid whitelist desired rule state %q", desiredState)
	}
	return nil
}

func knownWhitelistEventType(eventType string) bool {
	switch eventType {
	case "traffic.whitelist.v2.EntryDrafted", "traffic.whitelist.v2.ApprovalSubmitted",
		"traffic.whitelist.v2.EntryApproved", "traffic.whitelist.v2.EntryRejected",
		"traffic.whitelist.v2.EntryRevoked", "traffic.whitelist.v2.EntryArchived",
		"traffic.whitelist.v2.EntryExpired", "traffic.whitelist.v2.EntryExtended",
		"traffic.whitelist.v2.EntryAssigned", "traffic.whitelist.v2.EntryUpdated":
		return true
	default:
		return false
	}
}

func (d *OutboxDispatcher) recordFailure(ctx context.Context, outboxID int64, attemptCount int, dispatchErr error) error {
	status := "pending"
	if attemptCount >= d.cfg.MaxAttempts {
		status = "dead"
	}
	exponent := attemptCount
	if exponent > 8 {
		exponent = 8
	}
	retrySeconds := 1 << exponent
	message := dispatchErr.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	result, err := d.db.ExecContext(ctx, `UPDATE whitelist_event_outbox
		SET status=$3,available_at=now()+($4*interval '1 second'),
		    locked_by='',locked_until=NULL,last_error=$5
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`,
		outboxID, d.cfg.WorkerID, status, retrySeconds, message)
	if err != nil {
		return fmt.Errorf("record whitelist outbox failure: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("whitelist outbox failure state collision after: %w", dispatchErr)
	}
	return dispatchErr
}

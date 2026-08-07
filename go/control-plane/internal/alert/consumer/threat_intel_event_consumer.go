package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var threatIntelEventTypes = map[string]struct{}{
	"threat_intel.entry_upserted":          {},
	"threat_intel.feed_imported":           {},
	"threat_intel.feed_source_run":         {},
	"threat_intel.feed_scheduled_imported": {},
	"threat_intel.feed_configured":         {},
	"threat_intel.feed_run_failed":         {},
}

type ThreatIntelEventProjectionInput struct {
	EventID          string
	EventType        string
	SchemaVersion    int
	AggregateVersion int64
	TenantID         string
	Source           string
	TraceID          string
	OccurredAt       time.Time
	Entries          []threatintel.IntelEntry
	Payload          json.RawMessage
	KafkaPartition   int
	KafkaOffset      int64
}

type ThreatIntelEventProjectionApplier interface {
	ApplyThreatIntelEventProjection(context.Context, ThreatIntelEventProjectionInput) error
}

type ThreatIntelEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  ThreatIntelEventProjectionApplier
	logger   *zap.Logger
}

func NewThreatIntelEventConsumer(
	consumer *commonkafka.Consumer,
	applier ThreatIntelEventProjectionApplier,
	logger *zap.Logger,
) (*ThreatIntelEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("threat intel Kafka consumer and projection applier are required")
	}
	return &ThreatIntelEventConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func (consumer *ThreatIntelEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *ThreatIntelEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type threatIntelEventV1 struct {
	EventID           string                   `json:"event_id"`
	EventType         string                   `json:"event_type"`
	Version           int                      `json:"version"`
	SchemaVersion     int                      `json:"schema_version,omitempty"`
	AggregateVersion  int64                    `json:"aggregate_version,omitempty"`
	TenantID          string                   `json:"tenant_id"`
	UserID            string                   `json:"user_id,omitempty"`
	Username          string                   `json:"username,omitempty"`
	ActionID          string                   `json:"action_id,omitempty"`
	Reason            string                   `json:"reason,omitempty"`
	CompatibilityMode bool                     `json:"compatibility_mode,omitempty"`
	Source            string                   `json:"source"`
	Entry             *threatintel.IntelEntry  `json:"entry,omitempty"`
	Entries           []threatintel.IntelEntry `json:"entries,omitempty"`
	Feed              *threatintel.FeedSource  `json:"feed,omitempty"`
	Count             int                      `json:"count"`
	RequestID         string                   `json:"request_id,omitempty"`
	TraceID           string                   `json:"trace_id,omitempty"`
	OccurredAt        time.Time                `json:"occurred_at"`
}

func (consumer *ThreatIntelEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("threat intel Kafka message is nil")
	}
	var event threatIntelEventV1
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode threat intel event: %w", err)
	}
	if err := rejectTrailingThreatIntelJSON(decoder); err != nil {
		return err
	}
	if event.Version != 1 {
		return fmt.Errorf("unsupported threat intel version")
	}
	if _, ok := threatIntelEventTypes[event.EventType]; !ok {
		return fmt.Errorf("unsupported threat intel event_type")
	}
	if !strings.HasPrefix(event.EventID, "ti-") {
		return fmt.Errorf("invalid threat intel event_id prefix")
	}
	if _, err := uuid.Parse(strings.TrimPrefix(event.EventID, "ti-")); err != nil {
		return fmt.Errorf("invalid threat intel event_id")
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.Source) == "" ||
		event.OccurredAt.IsZero() {
		return fmt.Errorf("incomplete threat intel event contract")
	}
	legacyContract := event.SchemaVersion == 0 && event.AggregateVersion == 0
	if !legacyContract && (event.SchemaVersion != 1 || event.AggregateVersion < 1) {
		return fmt.Errorf("unsupported threat intel schema or aggregate version")
	}
	if !legacyContract && strings.TrimSpace(event.TraceID) == "" {
		return fmt.Errorf("canonical threat intel event requires trace_id")
	}
	entries, err := validateThreatIntelEventEntries(event)
	if err != nil {
		return err
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"tenant_id": event.TenantID, "source": event.Source,
		"trace_id": event.TraceID,
	}
	if !legacyContract {
		expectedHeaders["schema_version"] = "1"
		expectedHeaders["aggregate_version"] = strconv.FormatInt(event.AggregateVersion, 10)
		expectedHeaders["content_type"] = "application/json"
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("threat intel %s header/body mismatch", key)
		}
	}
	canonicalKey := string(message.Key) == event.TenantID
	legacyKey := string(message.Key) ==
		event.TenantID+":"+event.EventType+":"+event.Source+":"+event.EventID
	if !canonicalKey && !(legacyContract && legacyKey) {
		return fmt.Errorf("threat intel partition key/body mismatch")
	}
	traceID := event.TraceID
	if traceID == "" {
		traceID = event.EventID
	}
	schemaVersion := event.SchemaVersion
	aggregateVersion := event.AggregateVersion
	if legacyContract {
		schemaVersion = 1
		aggregateVersion = 1
	}
	input := ThreatIntelEventProjectionInput{
		EventID: event.EventID, EventType: event.EventType,
		SchemaVersion: schemaVersion, AggregateVersion: aggregateVersion,
		TenantID: event.TenantID, Source: event.Source, TraceID: traceID,
		OccurredAt: event.OccurredAt.UTC(), Entries: entries,
		Payload:        append(json.RawMessage(nil), message.Value...),
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyThreatIntelEventProjection(ctx, input); err != nil {
		return fmt.Errorf("apply threat intel projection %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Threat intel event projection committed",
			zap.String("event_id", event.EventID),
			zap.String("tenant_id", event.TenantID),
			zap.Int("indicator_count", len(entries)),
			zap.Int64("kafka_offset", message.Offset),
		)
	}
	return nil
}

func validateThreatIntelEventEntries(event threatIntelEventV1) ([]threatintel.IntelEntry, error) {
	if event.Count < 0 {
		return nil, fmt.Errorf("threat intel count must be non-negative")
	}
	if event.Entry != nil && len(event.Entries) != 0 {
		return nil, fmt.Errorf("threat intel event cannot contain both entry and entries")
	}
	entries := event.Entries
	if event.Entry != nil {
		entries = []threatintel.IntelEntry{*event.Entry}
	}
	if event.Count != len(entries) {
		return nil, fmt.Errorf("threat intel count/entries mismatch")
	}
	identities := make(map[string]struct{}, len(entries))
	for index := range entries {
		entry := &entries[index]
		entry.Type = strings.ToLower(strings.TrimSpace(entry.Type))
		entry.Value = strings.TrimSpace(entry.Value)
		entry.Source = strings.TrimSpace(entry.Source)
		if entry.Type == "" || entry.Value == "" || entry.Source == "" ||
			strings.TrimSpace(string(entry.Reputation)) == "" {
			return nil, fmt.Errorf("threat intel entry %d is incomplete", index)
		}
		identity := entry.Type + "\x00" + entry.Value
		if _, exists := identities[identity]; exists {
			return nil, fmt.Errorf("threat intel event contains duplicate indicator")
		}
		identities[identity] = struct{}{}
	}
	return entries, nil
}

func rejectTrailingThreatIntelJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode threat intel event: multiple JSON values")
		}
		return fmt.Errorf("decode threat intel event trailing data: %w", err)
	}
	return nil
}

type PostgresThreatIntelEventProjection struct {
	db *sql.DB
}

func NewPostgresThreatIntelEventProjection(db *sql.DB) (*PostgresThreatIntelEventProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("threat intel projection database is required")
	}
	return &PostgresThreatIntelEventProjection{db: db}, nil
}

func (projection *PostgresThreatIntelEventProjection) VerifySchema(ctx context.Context) error {
	var columnCount int
	err := projection.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='threat_intel_event_projection'`,
	).Scan(&columnCount)
	if err != nil {
		return fmt.Errorf("verify threat intel projection schema: %w", err)
	}
	if columnCount < 13 {
		return fmt.Errorf("threat intel projection schema is incomplete: columns=%d want>=13", columnCount)
	}
	return nil
}

func (projection *PostgresThreatIntelEventProjection) ApplyThreatIntelEventProjection(
	ctx context.Context,
	input ThreatIntelEventProjectionInput,
) error {
	if !json.Valid(input.Payload) {
		return fmt.Errorf("threat intel projection payload is invalid JSON")
	}
	entriesJSON, err := json.Marshal(input.Entries)
	if err != nil {
		return fmt.Errorf("marshal threat intel projection entries: %w", err)
	}
	tx, err := projection.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin threat intel projection: %w", err)
	}
	defer tx.Rollback()
	if len(input.Entries) > 0 {
		var authoritativeMatches int
		err = tx.QueryRowContext(ctx, `
			SELECT count(*)
			FROM jsonb_to_recordset($2::jsonb) AS event_entry(
				type text,value text,reputation text,category text,source text,description text
			)
			JOIN threat_intel authoritative
			  ON authoritative.tenant_id=$1
			 AND authoritative.type=event_entry.type
			 AND authoritative.value=event_entry.value
			 AND authoritative.reputation=event_entry.reputation
			 AND authoritative.category=event_entry.category
			 AND authoritative.source=event_entry.source
			 AND authoritative.description=event_entry.description`,
			input.TenantID, string(entriesJSON),
		).Scan(&authoritativeMatches)
		if err != nil {
			return fmt.Errorf("verify authoritative threat intel entries: %w", err)
		}
		if authoritativeMatches != len(input.Entries) {
			return fmt.Errorf(
				"threat intel authoritative payload mismatch: matched=%d want=%d",
				authoritativeMatches, len(input.Entries),
			)
		}
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO threat_intel_event_projection (
			event_id,event_type,schema_version,aggregate_version,tenant_id,source,
			indicator_count,payload,trace_id,occurred_at,kafka_partition,kafka_offset
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.EventType, input.SchemaVersion, input.AggregateVersion,
		input.TenantID, input.Source, len(input.Entries), string(input.Payload),
		input.TraceID, input.OccurredAt, input.KafkaPartition, input.KafkaOffset,
	)
	if err != nil {
		return fmt.Errorf("insert threat intel event projection: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect threat intel event projection insert: %w", err)
	}
	if inserted == 0 {
		var exactDuplicate bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM threat_intel_event_projection
				WHERE event_id=$1 AND event_type=$2
				  AND schema_version=$3 AND aggregate_version=$4
				  AND tenant_id=$5 AND source=$6 AND indicator_count=$7
				  AND payload=$8::jsonb AND trace_id=$9 AND occurred_at=$10
			)`,
			input.EventID, input.EventType, input.SchemaVersion, input.AggregateVersion,
			input.TenantID, input.Source, len(input.Entries), string(input.Payload),
			input.TraceID, input.OccurredAt,
		).Scan(&exactDuplicate)
		if err != nil {
			return fmt.Errorf("verify duplicate threat intel event: %w", err)
		}
		if !exactDuplicate {
			return fmt.Errorf("threat intel event identity or Kafka offset collision")
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit threat intel event projection: %w", err)
	}
	return nil
}

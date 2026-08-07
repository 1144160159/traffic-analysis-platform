package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fakeThreatIntelProjectionApplier struct {
	inputs []ThreatIntelEventProjectionInput
	err    error
}

func (applier *fakeThreatIntelProjectionApplier) ApplyThreatIntelEventProjection(
	_ context.Context,
	input ThreatIntelEventProjectionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return applier.err
}

func threatIntelKafkaMessage(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	event := threatIntelEventV1{
		EventID:   "ti-11111111-1111-4111-8111-111111111111",
		EventType: "threat_intel.entry_upserted",
		Version:   1, SchemaVersion: 1, AggregateVersion: 1,
		TenantID: "tenant-a", Source: "manual", Count: 1,
		TraceID: "trace-a", OccurredAt: time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC),
		Entry: &threatintel.IntelEntry{
			Type: "ip", Value: "203.0.113.9", Reputation: threatintel.RepMalicious,
			Category: "c2", Source: "manual", Description: "test indicator",
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := make([]segmentkafka.Header, 0, 8)
	for key, value := range map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": event.TenantID, "source": event.Source,
		"trace_id": event.TraceID, "content_type": "application/json",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "threat.intel.v1", Partition: 1, Offset: 27,
		Key: []byte(event.TenantID), Value: payload, Headers: headers,
	}}
}

func TestThreatIntelEventConsumerAppliesCanonicalEvent(t *testing.T) {
	applier := &fakeThreatIntelProjectionApplier{}
	eventConsumer := &ThreatIntelEventConsumer{applier: applier}
	if err := eventConsumer.handle(context.Background(), threatIntelKafkaMessage(t)); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want=1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.TenantID != "tenant-a" || input.SchemaVersion != 1 ||
		input.AggregateVersion != 1 || len(input.Entries) != 1 ||
		input.KafkaPartition != 1 || input.KafkaOffset != 27 {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestThreatIntelEventConsumerRejectsKeyMismatch(t *testing.T) {
	applier := &fakeThreatIntelProjectionApplier{}
	eventConsumer := &ThreatIntelEventConsumer{applier: applier}
	message := threatIntelKafkaMessage(t)
	message.Key = []byte("tenant-b")
	if err := eventConsumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected partition key mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestThreatIntelEventConsumerRejectsCountMismatch(t *testing.T) {
	applier := &fakeThreatIntelProjectionApplier{}
	eventConsumer := &ThreatIntelEventConsumer{applier: applier}
	message := threatIntelKafkaMessage(t)
	var payload map[string]interface{}
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		t.Fatal(err)
	}
	payload["count"] = 2
	message.Value, _ = json.Marshal(payload)
	if err := eventConsumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected count mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestThreatIntelEventConsumerPropagatesProjectionFailure(t *testing.T) {
	applier := &fakeThreatIntelProjectionApplier{err: errors.New("database unavailable")}
	eventConsumer := &ThreatIntelEventConsumer{applier: applier}
	if err := eventConsumer.handle(context.Background(), threatIntelKafkaMessage(t)); err == nil {
		t.Fatal("expected projection failure")
	}
}

func TestPostgresThreatIntelEventProjectionCommitsAuthoritativeEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, err := NewPostgresThreatIntelEventProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	input := threatIntelProjectionInput(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*)")).
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO threat_intel_event_projection").
		WithArgs(
			input.EventID, input.EventType, 1, int64(1), input.TenantID,
			input.Source, 1, sqlmock.AnyArg(), input.TraceID, input.OccurredAt,
			input.KafkaPartition, input.KafkaOffset,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := projection.ApplyThreatIntelEventProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresThreatIntelEventProjectionRejectsAuthoritativeMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, err := NewPostgresThreatIntelEventProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	input := threatIntelProjectionInput(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*)")).
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	if err := projection.ApplyThreatIntelEventProjection(context.Background(), input); err == nil {
		t.Fatal("expected authoritative payload mismatch")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresThreatIntelEventProjectionAcceptsDuplicateAtNewOffset(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, err := NewPostgresThreatIntelEventProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	input := threatIntelProjectionInput(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*)")).
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO threat_intel_event_projection").
		WithArgs(
			input.EventID, input.EventType, 1, int64(1), input.TenantID,
			input.Source, 1, sqlmock.AnyArg(), input.TraceID, input.OccurredAt,
			input.KafkaPartition, input.KafkaOffset,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(
			input.EventID, input.EventType, 1, int64(1), input.TenantID,
			input.Source, 1, sqlmock.AnyArg(), input.TraceID, input.OccurredAt,
		).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()
	if err := projection.ApplyThreatIntelEventProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func threatIntelProjectionInput(t *testing.T) ThreatIntelEventProjectionInput {
	t.Helper()
	message := threatIntelKafkaMessage(t)
	var event threatIntelEventV1
	if err := json.Unmarshal(message.Value, &event); err != nil {
		t.Fatal(err)
	}
	return ThreatIntelEventProjectionInput{
		EventID: event.EventID, EventType: event.EventType,
		SchemaVersion: 1, AggregateVersion: 1,
		TenantID: event.TenantID, Source: event.Source, TraceID: event.TraceID,
		OccurredAt: event.OccurredAt, Entries: []threatintel.IntelEntry{*event.Entry},
		Payload: message.Value, KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
}

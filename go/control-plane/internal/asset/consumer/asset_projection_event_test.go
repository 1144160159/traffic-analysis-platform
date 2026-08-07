package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/segmentio/kafka-go"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

func TestAssetProjectionEventConsumerAcceptsAuthoritativeEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	consumer, err := NewAssetProjectionEventConsumer(db)
	if err != nil {
		t.Fatal(err)
	}
	event := validAssetProjectionEvent()
	raw := marshalValidAssetProjectionEvent(t)
	assetJSON, _ := json.Marshal(event.Asset)
	canonicalPayload, _ := canonicalJSON(raw)
	hash := sha256.Sum256(canonicalPayload)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`)).
		WithArgs("8:tenant-a:" + event.AssetID + ":2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT tenant_id,asset_id::text,revision,trace_id,new_value::text`).
		WithArgs(event.EventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "asset_id", "revision", "trace_id", "new_value",
		}).AddRow(event.TenantID, event.AssetID, event.AggregateVersion, event.TraceID, assetJSON))
	mock.ExpectQuery(`SELECT event_id::text,payload_sha256`).
		WithArgs(event.EventID, event.TenantID, event.AssetID, event.AggregateVersion).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "payload_sha256"}))
	mock.ExpectExec(`INSERT INTO asset_projection_inbox`).
		WithArgs(
			event.EventID, event.TenantID, event.AssetID, event.AggregateVersion,
			event.PartitionKey, event.TraceID, raw, hex.EncodeToString(hash[:]), 3, int64(42),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := consumer.Accept(context.Background(), event, 3, 42, raw); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetProjectionEventConsumerExactReplayDoesNotDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	consumer, _ := NewAssetProjectionEventConsumer(db)
	event := validAssetProjectionEvent()
	raw := marshalValidAssetProjectionEvent(t)
	assetJSON, _ := json.Marshal(event.Asset)
	canonicalPayload, _ := canonicalJSON(raw)
	hash := sha256.Sum256(canonicalPayload)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT tenant_id,asset_id::text,revision,trace_id,new_value::text`).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "asset_id", "revision", "trace_id", "new_value",
		}).AddRow(event.TenantID, event.AssetID, event.AggregateVersion, event.TraceID, assetJSON))
	mock.ExpectQuery(`SELECT event_id::text,payload_sha256`).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "payload_sha256"}).
			AddRow(event.EventID, hex.EncodeToString(hash[:])))
	mock.ExpectCommit()

	if err := consumer.Accept(context.Background(), event, 9, 999, raw); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetProjectionEventConsumerRejectsHistoryMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	consumer, _ := NewAssetProjectionEventConsumer(db)
	event := validAssetProjectionEvent()
	raw := marshalValidAssetProjectionEvent(t)
	mismatched := event.Asset
	mismatched.Hostname = "forged-host"
	assetJSON, _ := json.Marshal(mismatched)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT tenant_id,asset_id::text,revision,trace_id,new_value::text`).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "asset_id", "revision", "trace_id", "new_value",
		}).AddRow(event.TenantID, event.AssetID, event.AggregateVersion, event.TraceID, assetJSON))
	mock.ExpectRollback()

	if err := consumer.Accept(context.Background(), event, 3, 42, raw); err == nil {
		t.Fatal("expected authoritative history mismatch")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetProjectionEventConsumerValidatesKafkaIdentity(t *testing.T) {
	event := validAssetProjectionEvent()
	raw := marshalValidAssetProjectionEvent(t)
	message := &kafkaCommon.ReceivedMessage{Message: kafka.Message{
		Key:   []byte(event.PartitionKey),
		Value: raw,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(event.EventID)},
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "schema_version", Value: []byte("2")},
			{Key: "aggregate_version", Value: []byte("2")},
			{Key: "tenant_id", Value: []byte(event.TenantID)},
			{Key: "asset_id", Value: []byte(event.AssetID)},
			{Key: "trace_id", Value: []byte("wrong-trace")},
		},
	}}
	if err := validateAssetProjectionHeaders(message, event); err == nil {
		t.Fatal("expected header/payload mismatch")
	}
}

package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/segmentio/kafka-go"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/sourcequality"
)

type recordingSourceQuality struct {
	receipts []sourcequality.Receipt
}

func (r *recordingSourceQuality) Record(_ context.Context, receipt sourcequality.Receipt) error {
	r.receipts = append(r.receipts, receipt)
	return nil
}

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
		WithArgs("8:tenant-a:" + event.AssetID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT tenant_id,asset_id::text,revision,trace_id,new_value::text`).
		WithArgs(event.EventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "asset_id", "revision", "trace_id", "new_value",
		}).AddRow(event.TenantID, event.AssetID, event.AggregateVersion, event.TraceID, assetJSON))
	mock.ExpectQuery(`SELECT event_id::text,payload_sha256,kafka_partition,kafka_offset`).
		WithArgs(event.EventID, event.TenantID, event.AssetID, event.AggregateVersion).
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "payload_sha256", "kafka_partition", "kafka_offset",
		}))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(aggregate_version\),0\)`).
		WithArgs(event.TenantID, event.AssetID).
		WillReturnRows(sqlmock.NewRows([]string{"maximum_version"}).AddRow(int64(0)))
	mock.ExpectExec(`INSERT INTO asset_projection_inbox`).
		WithArgs(
			event.EventID, event.TenantID, event.AssetID, event.AggregateVersion,
			event.PartitionKey, event.TraceID, raw, hex.EncodeToString(hash[:]), 3, int64(42),
			"asset.events.v2", event.Asset.LastSeen.UnixMilli(), raw,
			sourcequality.HashSource(raw), "disabled",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	disposition, err := consumer.AcceptClassified(context.Background(), event, 3, 42, raw)
	if err != nil {
		t.Fatal(err)
	}
	if disposition != AssetProjectionAccepted {
		t.Fatalf("disposition=%q want accepted", disposition)
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
	mock.ExpectQuery(`SELECT event_id::text,payload_sha256,kafka_partition,kafka_offset`).
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "payload_sha256", "kafka_partition", "kafka_offset",
		}).AddRow(event.EventID, hex.EncodeToString(hash[:]), 3, int64(42)))
	mock.ExpectCommit()

	disposition, err := consumer.AcceptClassified(context.Background(), event, 9, 999, raw)
	if err != nil {
		t.Fatal(err)
	}
	if disposition != AssetProjectionDuplicate {
		t.Fatalf("disposition=%q want duplicate", disposition)
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

func TestAssetProjectionEventConsumerClassifiesMissingOlderVersionAsLate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	consumer, _ := NewAssetProjectionEventConsumer(db)
	event := validAssetProjectionEvent()
	raw := marshalValidAssetProjectionEvent(t)
	assetJSON, _ := json.Marshal(event.Asset)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT tenant_id,asset_id::text,revision,trace_id,new_value::text`).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "asset_id", "revision", "trace_id", "new_value",
		}).AddRow(event.TenantID, event.AssetID, event.AggregateVersion, event.TraceID, assetJSON))
	mock.ExpectQuery(`SELECT event_id::text,payload_sha256,kafka_partition,kafka_offset`).
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "payload_sha256", "kafka_partition", "kafka_offset",
		}))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(aggregate_version\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"maximum_version"}).AddRow(int64(3)))
	mock.ExpectRollback()

	_, err = consumer.AcceptClassified(context.Background(), event, 3, 42, raw)
	if !kafkaCommon.IsPermanent(err) || !errors.Is(err, ErrAssetProjectionLate) {
		t.Fatalf("error=%v must be permanent late classification", err)
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

func TestAssetProjectionDLQReceiptIsDurableBarrier(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	recorder := &recordingSourceQuality{}
	consumer, err := NewAssetProjectionEventConsumerWithQuality(
		db, "asset-projection-v2", recorder)
	if err != nil {
		t.Fatal(err)
	}
	message := &kafkaCommon.ReceivedMessage{Message: kafka.Message{
		Topic: "asset.events.v2", Partition: 4, Offset: 19,
		Time: time.UnixMilli(2_000), Value: []byte("bad-json"),
		Headers: []kafka.Header{{Key: "tenant_id", Value: []byte("tenant-a")}},
	}}
	processingErr := kafkaCommon.Permanent(fmt.Errorf(
		"%w: duplicate header", ErrAssetProjectionEnvelope))

	if err := consumer.RecordDLQAcknowledgement(
		context.Background(), message, processingErr); err != nil {
		t.Fatal(err)
	}
	if len(recorder.receipts) != 1 {
		t.Fatalf("receipts=%d want 1", len(recorder.receipts))
	}
	receipt := recorder.receipts[0]
	if receipt.Category != sourcequality.Rejected || receipt.Source.Offset != 19 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
}

func TestAssetProjectionEnvelopeFailureIsPermanent(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	consumer, _ := NewAssetProjectionEventConsumer(db)
	message := &kafkaCommon.ReceivedMessage{Message: kafka.Message{
		Topic: "wrong.topic", Partition: 0, Offset: 1, Time: time.UnixMilli(1),
	}}
	if err := consumer.Handle(context.Background(), message); !kafkaCommon.IsPermanent(err) {
		t.Fatalf("error=%v must be permanent", err)
	}
}

func TestAssetProjectionMalformedPayloadIsPermanent(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	consumer, _ := NewAssetProjectionEventConsumer(db)
	message := &kafkaCommon.ReceivedMessage{Message: kafka.Message{
		Topic: "asset.events.v2", Partition: 2, Offset: 7, Time: time.UnixMilli(2_000),
		Key: []byte("tenant-a:asset-a"), Value: []byte("{not-json"),
	}}
	if err := consumer.Handle(context.Background(), message); !kafkaCommon.IsPermanent(err) {
		t.Fatalf("error=%v must be permanent", err)
	}
}

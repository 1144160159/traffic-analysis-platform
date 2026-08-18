package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestPublishFusionCommandRecordsExactBrokerAcknowledgement(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := fusionCommandTestItem(t)
	acknowledgedAt := time.Unix(5000, 0).UTC()
	handler := NewSystemHandler(nil, db, nil)
	handler.fusionCommandPublish = fusionCommandPublisherStub{receipt: commonkafka.BrokerReceipt{
		AttemptID: item.ClaimToken, Topic: fusion.SourceSyncTopic, Partition: 2, Offset: 17,
		Key: item.PartitionKey, AcknowledgedAt: acknowledgedAt,
	}}
	mock.ExpectExec(`UPDATE fusion_projection_outbox SET publish_state='KAFKA_ACKED'`).
		WithArgs(fusion.SourceSyncTopic, 2, int64(17), acknowledgedAt, item.EventID, item.ClaimToken).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := handler.publishFusionCommandOutboxItem(context.Background(), item); err != nil {
		t.Fatalf("publish fusion command: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishFusionCommandKeepsOutcomeUnknownForAmbiguousBrokerResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := fusionCommandTestItem(t)
	handler := NewSystemHandler(nil, db, nil)
	handler.fusionCommandPublish = fusionCommandPublisherStub{err: &commonkafka.PublishOutcomeUnknownError{
		Receipt: commonkafka.BrokerReceipt{AttemptID: item.ClaimToken, Topic: fusion.SourceSyncTopic},
		Cause:   errors.New("broker acknowledgement timed out"),
	}}
	if err := handler.publishFusionCommandOutboxItem(context.Background(), item); err == nil {
		t.Fatal("expected ambiguous publish error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("outcome-unknown path must not falsely write ACK or reset PENDING: %v", err)
	}
}

func TestPublishFusionCommandRejectsPayloadHashBeforeKafka(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := fusionCommandTestItem(t)
	item.PayloadSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	handler := NewSystemHandler(nil, db, nil)
	handler.fusionCommandPublish = fusionCommandPublisherStub{}
	mock.ExpectExec(`UPDATE fusion_projection_outbox SET publish_state='PENDING'`).
		WithArgs("INVALID_OUTBOX_PAYLOAD", item.EventID, item.ClaimToken).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := handler.publishFusionCommandOutboxItem(context.Background(), item); err == nil {
		t.Fatal("expected invalid outbox payload rejection")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func fusionCommandTestItem(t *testing.T) fusionCommandOutboxItem {
	t.Helper()
	command := fusion.SourceSyncCommand{
		EventID: "00000000-0000-0000-0000-000000000401", EventType: fusion.SourceSyncEventType,
		SchemaVersion: 1, AggregateType: "source_sync_job", AggregateID: "00000000-0000-0000-0000-000000000402",
		AggregateVersion: 1, PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000402",
		TenantID: "tenant-a", JobID: "00000000-0000-0000-0000-000000000402", SourceID: "traffic", SourceKind: "flow",
		WindowStart: time.Unix(100, 0).UTC(), WindowEnd: time.Unix(200, 0).UTC(),
		RequestedBy: "00000000-0000-0000-0000-000000000403", Reason: "refresh", TraceID: "trace-a", OccurredAt: time.Unix(201, 0).UTC(),
	}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	return fusionCommandOutboxItem{
		EventID: command.EventID, TenantID: command.TenantID, AggregateID: command.JobID,
		AggregateVersion: command.AggregateVersion, EventType: command.EventType, PartitionKey: command.PartitionKey,
		Payload: payload, PayloadSHA256: stringSHA256(payload), TraceID: command.TraceID,
		ClaimToken: "00000000-0000-0000-0000-000000000404",
	}
}

func stringSHA256(payload []byte) string {
	return fmtHash(sha256.Sum256(payload))
}

func fmtHash(value [32]byte) string {
	const digits = "0123456789abcdef"
	result := make([]byte, 64)
	for index, part := range value {
		result[index*2] = digits[part>>4]
		result[index*2+1] = digits[part&15]
	}
	return string(result)
}

package projection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

func TestInboxAcceptCommitsDurableEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inbox, _ := NewInbox(db)
	event := validRelationEvent(t, 1, "event", "")
	message := projectionMessage(t, event)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT payload_sha256,projection_sha256").
		WithArgs(event.GetHeader().GetEventId()).WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "projection_sha256"}))
	mock.ExpectQuery("SELECT event_id,projection_sha256").
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "projection_sha256"}))
	mock.ExpectQuery("SELECT max\\(aggregate_version\\)").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))
	mock.ExpectExec("INSERT INTO graph_projection_inbox_v1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	disposition, err := inbox.Accept(context.Background(), message)
	if err != nil || disposition != InboxAccepted {
		t.Fatalf("accept graph projection: disposition=%s err=%v", disposition, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInboxDuplicateEventRequiresExactBytes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inbox, _ := NewInbox(db)
	event := validRelationEvent(t, 1, "event", "")
	message := projectionMessage(t, event)
	sum := sha256.Sum256(message.Value)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT payload_sha256,projection_sha256").
		WithArgs(event.GetHeader().GetEventId()).
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "projection_sha256"}).
			AddRow(hex.EncodeToString(sum[:]), event.GetRelation().GetProjectionSha256()))
	mock.ExpectCommit()

	disposition, err := inbox.Accept(context.Background(), message)
	if err != nil || disposition != InboxDuplicate {
		t.Fatalf("duplicate graph projection: disposition=%s err=%v", disposition, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInboxRejectsHeaderBodyMismatchBeforeDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inbox, _ := NewInbox(db)
	event := validRelationEvent(t, 1, "event", "")
	message := projectionMessage(t, event)
	for index := range message.Headers {
		if message.Headers[index].Key == "tenant_id" {
			message.Headers[index].Value = []byte("tenant-b")
		}
	}
	if _, err := inbox.Accept(context.Background(), message); err == nil || !IsPermanentAdmissionError(err) {
		t.Fatalf("header/body mismatch was not permanent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func projectionMessage(t *testing.T, event *trafficv1.GraphProjectionEvent) *commonkafka.ReceivedMessage {
	t.Helper()
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := metadataOf(event)
	if err != nil {
		t.Fatal(err)
	}
	header := event.GetHeader()
	headers := map[string]string{
		"content_type": "application/x-protobuf", "proto_message_type": ProjectionProtoType,
		"event_id": header.GetEventId(), "event_type": header.GetEventType(),
		"schema_version": header.GetSchemaVersion(), "aggregate_type": header.GetAggregateType(),
		"aggregate_id": header.GetAggregateId(), "aggregate_version": strconv.FormatUint(header.GetAggregateVersion(), 10),
		"tenant_id": metadata.tenantID, "projection_kind": metadata.kind,
		"projection_id": metadata.projectionID, "projection_sha256": metadata.projectionSHA256,
		"source_event_id": metadata.sourceEventID, "trace_id": header.GetTraceId(),
	}
	kafkaHeaders := make([]segmentkafka.Header, 0, len(headers))
	for key, value := range headers {
		kafkaHeaders = append(kafkaHeaders, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: Topic, Partition: 2, Offset: 10, Key: []byte(event.GetPartitionKey()),
		Value: payload, Headers: kafkaHeaders, Time: time.UnixMilli(1700000000000),
	}}
}

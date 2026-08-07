package dataquality

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

func TestDecodeFlowReplayProjectionMessageRequiresMatchingBodyHeadersAndKey(t *testing.T) {
	repairID := uuid.NewString()
	event := &pb.FlowEvent{
		Header: &pb.EventHeader{
			EventId: "event-1", TenantId: "tenant-a", EventType: "flow.replay.v1",
			EventTs: 1785844801000, IngestTs: 1785844802000, TraceId: "trace-a",
			CausationId: repairID, CorrelationId: repairID, IdempotencyKey: repairID + ":event-1",
			Producer: "data-quality-repair-executor",
		},
		CommunityId: "community-a", Tuple: &pb.FiveTuple{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
	}
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	message := &commonkafka.ReceivedMessage{Message: kafkago.Message{
		Topic: FlowProjectionReplayTopic, Partition: 2, Offset: 9,
		Key: []byte("tenant-a:community-a"), Value: payload,
		Headers: []kafkago.Header{
			{Key: "tenant_id", Value: []byte("tenant-a")}, {Key: "event_id", Value: []byte("event-1")},
			{Key: "repair_id", Value: []byte(repairID)}, {Key: "idempotency_key", Value: []byte(repairID + ":event-1")},
			{Key: "content_type", Value: []byte("application/x-protobuf")},
			{Key: "proto_message_type", Value: []byte("traffic.v1.FlowEvent")},
			{Key: "proto_schema_version", Value: []byte("v1")}, {Key: "replay", Value: []byte("true")},
		},
	}}
	input, err := decodeFlowReplayProjectionMessage(message, FlowProjectionReplayTopic)
	if err != nil {
		t.Fatal(err)
	}
	if input.TenantID != "tenant-a" || input.RepairID != repairID || input.EventID != "event-1" || input.KafkaPartition != 2 || input.KafkaOffset != 9 {
		t.Fatalf("unexpected projection input: %+v", input)
	}
	invalidMessage := &commonkafka.ReceivedMessage{Message: message.Message}
	invalidMessage.Headers = append([]kafkago.Header(nil), message.Headers...)
	invalidMessage.Headers[0].Value = []byte("tenant-b")
	if _, err := decodeFlowReplayProjectionMessage(invalidMessage, FlowProjectionReplayTopic); err == nil {
		t.Fatal("header/body tenant mismatch must be rejected")
	}
}

func TestPostgresFlowReplayProjectionCommitsTargetAndReceiptAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repairID := uuid.NewString()
	payload := []byte("canonical-flow-protobuf")
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payload))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT operation_id,status FROM data_quality_repairs`).
		WithArgs("tenant-a", repairID).
		WillReturnRows(sqlmock.NewRows([]string{"operation_id", "status"}).AddRow("flow_replay_window_v1", "executing"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO data_quality_flow_replay_projection(")).
		WithArgs("tenant-a", repairID, "event-1", repairID+":event-1", payloadHash, payload, int64(1), int64(2), FlowReplayProjectionVersion, "trace-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO data_quality_replay_projection_receipts(")).
		WithArgs("tenant-a", repairID, "event-1", FlowReplayProjectionVersion, repairID+":event-1", payloadHash, FlowProjectionReplayTopic, 1, int64(7), "trace-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	projection, err := NewPostgresFlowReplayProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	err = projection.Commit(context.Background(), FlowReplayProjectionInput{
		TenantID: "tenant-a", RepairID: repairID, EventID: "event-1", IdempotencyKey: repairID + ":event-1",
		TraceID: "trace-a", Payload: payload, SourceEventTS: 1, SourceIngestTS: 2,
		KafkaTopic: FlowProjectionReplayTopic, KafkaPartition: 1, KafkaOffset: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

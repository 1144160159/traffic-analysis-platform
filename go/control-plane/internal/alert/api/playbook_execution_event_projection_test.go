package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func playbookProjectionInput() PlaybookExecutionEventProjectionInput {
	return PlaybookExecutionEventProjectionInput{
		EventID: "44444444-4444-4444-8444-444444444444", TenantID: "tenant-a",
		ExecutionID: "execution-a", PlaybookName: "isolate-host", PlaybookVersion: 3,
		AlertID: "alert-a", EventType: "traffic.playbook.v2.ExecutionCompleted", Status: "completed",
		ApprovalStatus: "approved", ExecutorStatus: "succeeded", SchemaVersion: 2,
		AggregateVersion: 3, PartitionKey: "tenant-a:execution-a", TraceID: "trace-projection",
		Payload: map[string]interface{}{
			"event_id": "44444444-4444-4444-8444-444444444444", "status": "completed",
			"aggregate_version": 3,
		},
		KafkaTopic: PlaybookExecutionEventTopic, KafkaPartition: 2, KafkaOffset: 17,
	}
}

func TestApplyPlaybookExecutionEventProjectionCommitsInboxAndLatestState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAdvancedHandler(nil, nil, nil, nil, NewAdvancedRepository(db, zap.NewNop()))
	input := playbookProjectionInput()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT workflow_revision FROM alert_playbook_executions").
		WithArgs(input.TenantID, input.ExecutionID).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_revision"}).AddRow(int64(3)))
	mock.ExpectExec("INSERT INTO alert_playbook_execution_event_projection").
		WithArgs(input.EventID, input.TenantID, input.ExecutionID, input.PlaybookName, input.EventType,
			input.SchemaVersion, input.AggregateVersion, input.PartitionKey, input.TraceID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), input.KafkaTopic, input.KafkaPartition, input.KafkaOffset).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO alert_playbook_execution_state_projection").
		WithArgs(input.TenantID, input.ExecutionID, input.PlaybookName, input.PlaybookVersion, input.AlertID,
			input.Status, input.ApprovalStatus, input.ExecutorStatus, input.AggregateVersion, input.EventType,
			input.TraceID, input.EventID, sqlmock.AnyArg(), sqlmock.AnyArg(), input.KafkaTopic,
			input.KafkaPartition, input.KafkaOffset).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := handler.ApplyPlaybookExecutionEventProjection(context.Background(), input); err != nil {
		t.Fatalf("ApplyPlaybookExecutionEventProjection() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPlaybookExecutionEventProjectionAcceptsExactReplayWithoutStateRewrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAdvancedHandler(nil, nil, nil, nil, NewAdvancedRepository(db, zap.NewNop()))
	input := playbookProjectionInput()
	canonical, err := json.Marshal(input.Payload)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	payloadSHA := hex.EncodeToString(digest[:])
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT workflow_revision FROM alert_playbook_executions").
		WithArgs(input.TenantID, input.ExecutionID).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_revision"}).AddRow(int64(5)))
	mock.ExpectExec("INSERT INTO alert_playbook_execution_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT payload_sha256,kafka_topic,kafka_partition,kafka_offset").
		WithArgs(input.EventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"payload_sha256", "kafka_topic", "kafka_partition", "kafka_offset",
			"tenant_id", "execution_id", "aggregate_version",
		}).AddRow(payloadSHA, input.KafkaTopic, input.KafkaPartition, input.KafkaOffset,
			input.TenantID, input.ExecutionID, input.AggregateVersion))
	mock.ExpectCommit()
	if err := handler.ApplyPlaybookExecutionEventProjection(context.Background(), input); err != nil {
		t.Fatalf("exact replay error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPlaybookExecutionEventProjectionRejectsEventAheadOfAuthority(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAdvancedHandler(nil, nil, nil, nil, NewAdvancedRepository(db, zap.NewNop()))
	input := playbookProjectionInput()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT workflow_revision FROM alert_playbook_executions").
		WithArgs(input.TenantID, input.ExecutionID).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_revision"}).AddRow(int64(2)))
	mock.ExpectRollback()
	if err := handler.ApplyPlaybookExecutionEventProjection(context.Background(), input); err == nil {
		t.Fatal("event ahead of PostgreSQL authority must fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

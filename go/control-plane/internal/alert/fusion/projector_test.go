package fusion

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type sourceFactReaderStub struct {
	batch SourceFactBatch
	err   error
	calls int
}

func (stub *sourceFactReaderStub) ReadSourceFacts(context.Context, string, string, time.Time, time.Time, int) (SourceFactBatch, error) {
	stub.calls++
	return stub.batch, stub.err
}

func TestProjectorDurablyFailsInvalidSourceFactBeforeOffsetCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	command := SourceSyncCommand{
		EventID: "00000000-0000-0000-0000-000000000201", EventType: SourceSyncEventType,
		SchemaVersion: 1, AggregateType: "source_sync_job", AggregateID: "00000000-0000-0000-0000-000000000202",
		AggregateVersion: 1, PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000202",
		TenantID: "tenant-a", JobID: "00000000-0000-0000-0000-000000000202", SourceID: "traffic", SourceKind: "flow",
		WindowStart: start, WindowEnd: end, RequestedBy: "00000000-0000-0000-0000-000000000203",
		Reason: "test", TraceID: "trace-a", OccurredAt: end.Add(time.Second),
	}
	eventSHA := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	mock.ExpectQuery(`SELECT tenant_id,job_id::text,event_sha256,disposition`).
		WithArgs(command.EventID).WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT source_id,source_kind,request_sha256,requested_window_start`).
		WithArgs(command.TenantID, command.JobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source_id", "source_kind", "request_sha256", "requested_window_start", "requested_window_end",
			"expected_source_version", "status", "revision", "trace_id",
		}).AddRow("traffic", "flow", eventSHA, start, end, nil, "queued", int64(1), "trace-a"))
	mock.ExpectExec(`UPDATE fusion_source_sync_jobs`).
		WithArgs(command.TenantID, command.JobID, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT tenant_id,job_id::text,event_sha256,disposition`).
		WithArgs(command.EventID).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT source_id,source_kind,request_sha256,requested_window_start`).
		WithArgs(command.TenantID, command.JobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source_id", "source_kind", "request_sha256", "requested_window_start", "requested_window_end",
			"expected_source_version", "status", "trace_id",
		}).AddRow("traffic", "flow", eventSHA, start, end, nil, "running", "trace-a"))
	mock.ExpectExec(`UPDATE fusion_source_sync_jobs SET status='failed'`).
		WithArgs("INVALID_SOURCE_FACT", sqlmock.AnyArg(), command.TenantID, command.JobID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO fusion_projection_inbox`).
		WithArgs(command.EventID, command.TenantID, command.JobID, command.SourceID, eventSHA,
			SourceSyncTopic, 1, int64(7), "INVALID_SOURCE_FACT", command.TraceID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	reader := &sourceFactReaderStub{batch: SourceFactBatch{Total: 1, Facts: []SourceFact{{
		EventID: "bad-flow", PayloadBase64: "not-base64",
	}}}}
	projector, err := NewProjector(db, reader, 100)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := projector.ApplySourceSync(context.Background(), command, eventSHA, KafkaPosition{Topic: SourceSyncTopic, Partition: 1, Offset: 7})
	if err != nil {
		t.Fatalf("invalid source fact should produce a durable failed receipt, got %v", err)
	}
	if receipt.Disposition != "failed" || receipt.FailureCode != "INVALID_SOURCE_FACT" || reader.calls != 1 {
		t.Fatalf("unexpected failure receipt: %#v calls=%d", receipt, reader.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

package api

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func probeOperationProjectionFixture() ProbeOperationProjectionInput {
	return ProbeOperationProjectionInput{
		EventID:   "11111111-1111-4111-8111-111111111111",
		EventType: "traffic.probe.v2.OperationAcknowledged",
		TenantID:  "tenant-a", ProbeID: "probe-a",
		OperationID: "22222222-2222-4222-8222-222222222222",
		Revision:    3, Status: "completed", TraceID: "trace-a",
		Payload:        map[string]interface{}{"revision": float64(3)},
		KafkaPartition: 1, KafkaOffset: 9,
	}
}

func TestApplyProbeOperationProjectionCommitsEventAndLatestState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO probe_operation_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO probe_operation_state_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := handler.ApplyProbeOperationProjection(
		context.Background(), probeOperationProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyProbeOperationProjectionCrossOffsetReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO probe_operation_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT operation_id::text").
		WillReturnRows(sqlmock.NewRows([]string{
			"operation_id", "tenant_id", "probe_id", "event_type", "revision", "status", "trace_id", "payload",
		}).AddRow(
			"22222222-2222-4222-8222-222222222222", "tenant-a", "probe-a",
			"traffic.probe.v2.OperationAcknowledged", int64(3), "completed", "trace-a",
			[]byte(`{"revision":3}`),
		))
	mock.ExpectCommit()
	replay := probeOperationProjectionFixture()
	replay.KafkaPartition = 4
	replay.KafkaOffset = 101
	if err := handler.ApplyProbeOperationProjection(context.Background(), replay); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyProbeOperationProjectionRejectsCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO probe_operation_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT operation_id::text").
		WillReturnRows(sqlmock.NewRows([]string{
			"operation_id", "tenant_id", "probe_id", "event_type", "revision", "status", "trace_id", "payload",
		}).AddRow(
			"22222222-2222-4222-8222-222222222222", "tenant-a", "probe-a",
			"traffic.probe.v2.OperationAcknowledged", int64(3), "failed", "trace-a",
			[]byte(`{"revision":3,"conflict":true}`),
		))
	mock.ExpectRollback()
	if err := handler.ApplyProbeOperationProjection(
		context.Background(), probeOperationProjectionFixture(),
	); !errors.Is(err, ErrProbeOperationProjectionConflict) {
		t.Fatalf("error = %v, want ErrProbeOperationProjectionConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

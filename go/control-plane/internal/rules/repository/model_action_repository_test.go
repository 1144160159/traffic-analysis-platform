package repository

import (
	"context"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestCreateAsynchronousModelActionCommitsJobOutboxAndAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := NewModelRepository(db, zap.NewNop())
	job := &model.ModelActionJob{
		JobID:    "22222222-2222-4222-8222-222222222222",
		EventID:  "11111111-1111-4111-8111-111111111111",
		ActionID: "33333333-3333-4333-8333-333333333333",
		Revision: 1, TenantID: "tenant-a",
		ModelID: "44444444-4444-4444-8444-444444444444",
		Version: "version-1", Action: "request-evaluation",
		Target: "mlops", Payload: map[string]interface{}{"dataset_id": "dataset-1"},
		Status: "queued", RequestedBy: "operator-a",
		CreatedAt: time.Date(2026, 7, 31, 8, 30, 0, 0, time.UTC),
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_action_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.CreateModelAction(
		context.Background(), job, "MODEL_EVALUATION_REQUESTED", "127.0.0.1", "test",
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAsynchronousModelActionRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := NewModelRepository(db, zap.NewNop())
	job := &model.ModelActionJob{
		JobID:    "22222222-2222-4222-8222-222222222222",
		EventID:  "11111111-1111-4111-8111-111111111111",
		ActionID: "33333333-3333-4333-8333-333333333333",
		Revision: 1, TenantID: "tenant-a",
		ModelID: "44444444-4444-4444-8444-444444444444",
		Action:  "request-retraining", Target: "mlops",
		Payload: map[string]interface{}{
			"dataset_id": "dataset-1", "strategy": "full", "reason": "drift",
		},
		Status: "queued", RequestedBy: "operator-a", CreatedAt: time.Now().UTC(),
	}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("INSERT INTO model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_action_outbox").
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	if err := repository.CreateModelAction(
		context.Background(), job, "MODEL_RETRAIN_REQUESTED", "", "",
	); err == nil {
		t.Fatal("expected outbox failure to roll back model action transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

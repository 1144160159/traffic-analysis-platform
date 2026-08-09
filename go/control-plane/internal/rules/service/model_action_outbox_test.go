package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func modelActionOutboxFixture() *modelActionOutboxRecord {
	return &modelActionOutboxRecord{
		OutboxID:         7,
		EventID:          "11111111-1111-4111-8111-111111111111",
		JobID:            "22222222-2222-4222-8222-222222222222",
		TenantID:         "tenant-a",
		ModelID:          "44444444-4444-4444-8444-444444444444",
		PartitionKey:     "44444444-4444-4444-8444-444444444444",
		AggregateVersion: 1, AttemptCount: 1,
		ActionID:    "33333333-3333-4333-8333-333333333333",
		RequestedBy: "operator-a",
	}
}

func TestCompleteModelActionDispatchIsNonTerminalAndAudited(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &ModelService{db: db, outboxWorkerID: "worker-a"}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE model_action_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.completeModelActionDispatch(
		context.Background(), modelActionOutboxFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFailModelActionOutboxRetriesBeforeDeadLetter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &ModelService{db: db, outboxWorkerID: "worker-a"}
	record := modelActionOutboxFixture()
	record.AttemptCount = 2
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE model_action_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.failModelActionOutbox(
		context.Background(), record, errors.New("Kafka unavailable"),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFailModelActionOutboxDeadLettersAndFailsJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &ModelService{db: db, outboxWorkerID: "worker-a"}
	record := modelActionOutboxFixture()
	record.AttemptCount = 8
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE model_action_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.failModelActionOutbox(
		context.Background(), record, errors.New("Kafka unavailable"),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFailModelActionOutboxReconcilesConsumerReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &ModelService{db: db, outboxWorkerID: "worker-a"}
	record := modelActionOutboxFixture()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE model_action_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.failModelActionOutbox(
		context.Background(), record, errors.New("ambiguous Kafka timeout"),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

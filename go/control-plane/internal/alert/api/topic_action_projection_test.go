package api

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func topicActionProjectionFixture() TopicActionProjectionInput {
	return TopicActionProjectionInput{
		EventID:   "11111111-1111-4111-8111-111111111111",
		EventType: "traffic.topic.v2.ActionResult",
		TenantID:  "tenant-a", Topic: "apt",
		JobID:    "22222222-2222-4222-8222-222222222222",
		ActionID: "export_snapshot", Revision: 3, Status: "completed",
		TraceID: "trace-a", Payload: map[string]interface{}{"revision": float64(3)},
		KafkaPartition: 2, KafkaOffset: 19,
	}
}

func TestApplyTopicActionProjectionCommitsEventAndLatestJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO topic_action_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO topic_action_job_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := handler.ApplyTopicActionProjection(
		context.Background(),
		topicActionProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTopicActionProjectionAcceptsExactDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO topic_action_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()

	if err := handler.ApplyTopicActionProjection(
		context.Background(),
		topicActionProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTopicActionProjectionRejectsIdentityCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO topic_action_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()

	if err := handler.ApplyTopicActionProjection(
		context.Background(),
		topicActionProjectionFixture(),
	); err == nil {
		t.Fatal("expected event identity collision")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

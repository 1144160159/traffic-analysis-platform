package api

import (
	"context"
	"errors"
	"regexp"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func topicActionOutboxRows(eventID, jobID, eventType, payload string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"event_id", "job_id", "tenant_id", "event_type", "partition_key",
		"aggregate_version", "schema_version", "payload",
	}).AddRow(
		eventID, jobID, "tenant-a", eventType, "tenant-a:tunnel",
		int64(3), 2, payload,
	)
}

func TestDrainTopicActionOutboxPublishesWithStableHeaders(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "11111111-1111-4111-8111-111111111111"
	jobID := "22222222-2222-4222-8222-222222222222"
	var published bool
	handler.topicActionPublish = func(
		_ context.Context,
		key string,
		payload []byte,
		headers ...commonkafka.MessageHeader,
	) error {
		published = true
		if key != "tenant-a:tunnel" {
			t.Fatalf("unexpected key %q", key)
		}
		if string(payload) != `{"event_id":"`+eventID+`","job_id":"`+jobID+`"}` {
			t.Fatalf("unexpected payload %s", payload)
		}
		assertProbeHeader(t, headers, "event_id", eventID)
		assertProbeHeader(t, headers, "event_type", "traffic.topic.v2.ActionRequested")
		assertProbeHeader(t, headers, "tenant_id", "tenant-a")
		assertProbeHeader(t, headers, "job_id", jobID)
		assertProbeHeader(t, headers, "aggregate_version", "3")
		assertProbeHeader(t, headers, "schema_version", "2")
		assertProbeHeader(t, headers, "target_topic", topicActionKafkaTopic)
		return nil
	}
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-1").
		WillReturnRows(topicActionOutboxRows(
			eventID, jobID, "traffic.topic.v2.ActionRequested",
			`{"event_id":"`+eventID+`","job_id":"`+jobID+`"}`,
		))
	mock.ExpectExec("UPDATE topic_action_outbox").
		WithArgs(eventID, "worker-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainTopicActionOutbox(context.Background(), "worker-1", 50)
	if err != nil {
		t.Fatalf("drainTopicActionOutbox() error = %v", err)
	}
	if processed != 1 || !published {
		t.Fatalf("processed=%d published=%v, want 1/true", processed, published)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDrainTopicActionOutboxFailureRemainsPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "33333333-3333-4333-8333-333333333333"
	jobID := "44444444-4444-4444-8444-444444444444"
	handler.topicActionPublish = func(
		context.Context,
		string,
		[]byte,
		...commonkafka.MessageHeader,
	) error {
		return errors.New("kafka unavailable")
	}
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-2").
		WillReturnRows(topicActionOutboxRows(
			eventID, jobID, "traffic.topic.v2.ActionResult",
			`{"event_id":"`+eventID+`","job_id":"`+jobID+`"}`,
		))
	mock.ExpectExec("UPDATE topic_action_outbox").
		WithArgs(eventID, "kafka unavailable", "worker-2").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainTopicActionOutbox(context.Background(), "worker-2", 50)
	if err != nil {
		t.Fatalf("drainTopicActionOutbox() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed=%d, want 0", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDrainTopicActionOutboxRejectsInvalidJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "55555555-5555-4555-8555-555555555555"
	jobID := "66666666-6666-4666-8666-666666666666"
	handler.topicActionPublish = func(
		context.Context,
		string,
		[]byte,
		...commonkafka.MessageHeader,
	) error {
		t.Fatal("invalid JSON must not be published")
		return nil
	}
	mock.ExpectQuery(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(50, "worker-3").
		WillReturnRows(topicActionOutboxRows(
			eventID, jobID, "traffic.topic.v2.ActionRequested", `{`,
		))
	mock.ExpectExec("UPDATE topic_action_outbox").
		WithArgs(eventID, "invalid outbox JSON payload", "worker-3").
		WillReturnResult(sqlmock.NewResult(0, 1))

	processed, err := handler.drainTopicActionOutbox(context.Background(), "worker-3", 50)
	if err != nil {
		t.Fatalf("drainTopicActionOutbox() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed=%d, want 0", processed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

package kafka

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

func TestPostgresDLQAcknowledgementBarrierMatrix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	barrier, err := NewPostgresDLQAcknowledgementBarrier(db, "probe-group")
	if err != nil {
		t.Fatal(err)
	}
	message := &ReceivedMessage{Message: segmentkafka.Message{
		Topic: "probe.topic", Partition: 2, Offset: 9,
		Key: []byte("key"), Value: []byte("value"),
		Headers: []segmentkafka.Header{{Key: "event_id", Value: []byte("event")}},
	}}
	query := regexp.QuoteMeta("INSERT INTO kafka_dlq_acknowledgement_receipts")
	mock.ExpectQuery(query).
		WithArgs("probe-group", "probe.topic", 2, int64(9),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exact"}).AddRow(true))
	if err := barrier(context.Background(), message, errors.New("invalid envelope")); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(query).
		WithArgs("probe-group", "probe.topic", 2, int64(9),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sqlmock.ErrCancelled)
	if err := barrier(context.Background(), message, errors.New("invalid envelope")); err == nil ||
		errors.Is(err, ErrDLQAcknowledgementConflict) {
		t.Fatalf("storage error classification=%v", err)
	}
	mock.ExpectQuery(query).
		WithArgs("probe-group", "probe.topic", 2, int64(9),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sqlmock.ErrCancelled)
	if err := barrier(context.Background(), message, errors.New("changed error")); err == nil {
		t.Fatal("changed error identity was acknowledged")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDLQAcknowledgementBarrierRejectsConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	barrier, err := NewPostgresDLQAcknowledgementBarrier(db, "probe-group")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kafka_dlq_acknowledgement_receipts")).
		WillReturnRows(sqlmock.NewRows([]string{"exact"}))
	err = barrier(context.Background(), &ReceivedMessage{Message: segmentkafka.Message{
		Topic: "probe.topic", Partition: 0, Offset: 1, Key: []byte("k"), Value: []byte("v"),
	}}, errors.New("poison"))
	if !errors.Is(err, ErrDLQAcknowledgementConflict) {
		t.Fatalf("err=%v want conflict", err)
	}
}

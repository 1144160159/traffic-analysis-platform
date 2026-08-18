package kafka

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
)

func TestPostgresDLQAcknowledgementBarrierRealPostgresIdentity(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("KAFKA_DLQ_BARRIER_EPHEMERAL_PG_DSN"))
	if dsn == "" {
		t.Skip("KAFKA_DLQ_BARRIER_EPHEMERAL_PG_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var marker string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_m02_probe_pipeline_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel PostgreSQL: marker=%q err=%v", marker, err)
	}
	groupID := "dlq-integration-" + uuid.NewString()
	barrier, err := NewPostgresDLQAcknowledgementBarrier(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	message := &ReceivedMessage{Message: segmentkafka.Message{
		Topic: "probe.control.v2", Partition: 1, Offset: 27,
		Key: []byte("tenant-a:probe-a"), Value: []byte("poison-v1"),
		Headers: []segmentkafka.Header{
			{Key: "event_id", Value: []byte("event-a")},
			{Key: "schema_version", Value: []byte("2")},
		},
	}}
	processingErr := errors.New("invalid command envelope")
	if err := barrier(ctx, message, processingErr); err != nil {
		t.Fatal(err)
	}
	if err := barrier(ctx, message, processingErr); err != nil {
		t.Fatalf("exact source replay was not idempotent: %v", err)
	}
	conflict := *message
	conflict.Value = []byte("poison-v2")
	if err := barrier(ctx, &conflict, processingErr); !errors.Is(err, ErrDLQAcknowledgementConflict) {
		t.Fatalf("conflicting source tuple err=%v want ErrDLQAcknowledgementConflict", err)
	}
	var receiptCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kafka_dlq_acknowledgement_receipts
		WHERE consumer_group=$1 AND source_topic=$2 AND source_partition=$3 AND source_offset=$4`,
		groupID, message.Topic, message.Partition, message.Offset).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 1 {
		t.Fatalf("durable receipt count=%d want=1", receiptCount)
	}
}

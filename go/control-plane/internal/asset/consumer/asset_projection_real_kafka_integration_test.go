package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

const assetProjectionRealKafkaTenant = "asset-projection-real-kafka"

// TestAssetProjectionRealKafkaDurableInbox crosses an actual Kafka protocol
// boundary. It only accepts an owned sentinel PostgreSQL database and a
// loopback broker provisioned by the alignment runner.
func TestAssetProjectionRealKafkaDurableInbox(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ASSET_PROJECTION_EPHEMERAL_PG_DSN"))
	broker := strings.TrimSpace(os.Getenv("ASSET_PROJECTION_EPHEMERAL_KAFKA_BROKER"))
	if dsn == "" || broker == "" {
		t.Skip("explicit ephemeral PostgreSQL and Kafka settings are required")
	}
	if os.Getenv("ASSET_PROJECTION_EPHEMERAL_KAFKA_SENTINEL") != "ephemeral-only" {
		t.Fatal("explicit ephemeral Kafka sentinel is required")
	}
	host, _, err := net.SplitHostPort(broker)
	if err != nil || (host != "127.0.0.1" && host != "localhost") {
		t.Fatalf("ephemeral Kafka must use a loopback broker: %q", broker)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_projection_kafka_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}
	cleanupAssetProjectionKafkaTenant(t, db)
	defer cleanupAssetProjectionKafkaTenant(t, db)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Projection Real Kafka')`, assetProjectionRealKafkaTenant); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	record := &config.AssetRecord{
		TenantID: assetProjectionRealKafkaTenant, MACAddress: "02:00:00:00:02:01",
		IPAddress: "192.0.2.201", Hostname: "asset-projection-kafka",
		AssetType: "server", Status: "active", Source: "integration", Criticality: 4,
		Metadata: map[string]any{"zone": "ephemeral"},
	}
	upsert, err := repo.UpsertAtomic(ctx, record, config.AssetUpsertCommand{
		ExpectedRevision: 0, IdempotencyKey: "asset-projection-real-kafka-create",
		Actor: "integration-runner", TraceID: "trace-asset-projection-real-kafka",
		RequestID: "request-asset-projection-real-kafka",
	})
	if err != nil {
		t.Fatal(err)
	}

	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: "asset.events.v2", BatchSize: 1,
		BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3,
		RequiredAcks: "all", Compression: "none", Async: false,
		IdempotentKey: "tenant_id+asset_id",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	dispatcher, err := assetRepository.NewAssetOutboxDispatcher(db, producer, assetRepository.OutboxDispatcherConfig{
		WorkerID: "asset-projection-real-kafka-dispatcher", Lease: 10 * time.Second,
		MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	eventConsumer, err := NewAssetProjectionEventConsumer(db)
	if err != nil {
		t.Fatal(err)
	}
	kafkaConsumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: []string{broker}, Topic: "asset.events.v2",
		GroupID:  "asset-projection-real-kafka-" + uuid.NewString(),
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 100 * time.Millisecond,
		StartOffset: segmentkafka.FirstOffset, MaxRetries: 1, RetryBackoff: 25 * time.Millisecond,
		CommitOnHandlerError: false, DLQPermanentOnly: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	committed := make(chan segmentkafka.Message, 4)
	kafkaConsumer.SetCommitObserver(func(messages []segmentkafka.Message) {
		for _, message := range messages {
			committed <- message
		}
	})
	consumeCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)
	go func() { consumerDone <- kafkaConsumer.Consume(consumeCtx, eventConsumer.Handle) }()
	defer func() {
		stopConsumer()
		_ = kafkaConsumer.Close()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
		}
	}()

	if found, dispatchErr := dispatcher.DispatchNext(ctx); dispatchErr != nil || !found {
		t.Fatalf("dispatch found=%v err=%v", found, dispatchErr)
	}
	first := waitForAssetProjectionCommit(t, committed, upsert.EventID, 15*time.Second)
	assertAssetProjectionInbox(t, db, upsert.EventID, first.Partition, first.Offset, 1)

	var outboxStatus string
	var publishedAt sql.NullTime
	if err := db.QueryRow(`SELECT status,published_at FROM asset_event_outbox WHERE event_id=$1`, upsert.EventID).Scan(&outboxStatus, &publishedAt); err != nil {
		t.Fatal(err)
	}
	if outboxStatus != "published" || !publishedAt.Valid {
		t.Fatalf("outbox status=%q published_at=%v", outboxStatus, publishedAt)
	}

	replayWriter := segmentkafka.NewWriter(segmentkafka.WriterConfig{
		Brokers: []string{broker}, Balancer: &segmentkafka.Hash{},
		RequiredAcks: int(segmentkafka.RequireAll), MaxAttempts: 3,
	})
	defer replayWriter.Close()
	replay := first
	replay.Topic = "asset.events.v2"
	replay.Partition = 0
	replay.Offset = 0
	if err := replayWriter.WriteMessages(ctx, replay); err != nil {
		t.Fatal(err)
	}
	second := waitForAssetProjectionCommit(t, committed, upsert.EventID, 15*time.Second)
	if second.Offset <= first.Offset {
		t.Fatalf("replay offset=%d must be after first offset=%d", second.Offset, first.Offset)
	}
	assertAssetProjectionInbox(t, db, upsert.EventID, first.Partition, first.Offset, 1)
	if kafkaConsumer.GetMetrics().CommitsSucceeded < 2 {
		t.Fatalf("commits_succeeded=%d want>=2", kafkaConsumer.GetMetrics().CommitsSucceeded)
	}
}

func waitForAssetProjectionCommit(t *testing.T, messages <-chan segmentkafka.Message, eventID string, timeout time.Duration) segmentkafka.Message {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case message := <-messages:
			var event AssetUpsertedV2
			if err := json.Unmarshal(message.Value, &event); err == nil && event.EventID == eventID {
				return message
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for committed asset event %s", eventID)
		}
	}
}

func assertAssetProjectionInbox(t *testing.T, db *sql.DB, eventID string, partition int, offset int64, wantCount int) {
	t.Helper()
	var count, storedPartition int
	var storedOffset int64
	var status string
	if err := db.QueryRow(`SELECT count(*),min(kafka_partition),min(kafka_offset),min(status) FROM asset_projection_inbox WHERE event_id=$1`, eventID).Scan(&count, &storedPartition, &storedOffset, &status); err != nil {
		t.Fatal(err)
	}
	if count != wantCount || storedPartition != partition || storedOffset != offset || status != "pending" {
		t.Fatalf("inbox count=%d partition=%d offset=%d status=%q want=%d/%d/%d/pending", count, storedPartition, storedOffset, status, wantCount, partition, offset)
	}
}

func cleanupAssetProjectionKafkaTenant(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`DELETE FROM asset_projection_watermarks WHERE tenant_id=$1`,
		`DELETE FROM asset_projection_inbox WHERE tenant_id=$1`,
		`DELETE FROM asset_upsert_requests WHERE tenant_id=$1`,
		`DELETE FROM asset_event_outbox WHERE tenant_id=$1`,
		`DELETE FROM asset_events WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM assets WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, assetProjectionRealKafkaTenant); err != nil {
			t.Fatalf("cleanup asset projection tenant: %v", err)
		}
	}
}

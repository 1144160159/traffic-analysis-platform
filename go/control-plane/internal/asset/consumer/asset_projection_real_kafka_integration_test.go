package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
const assetProjectionFailureTenant = "asset-projection-publish-failure"

type ackObservingAssetPublisher struct {
	delegate           assetRepository.AssetEventPublisher
	db                 *sql.DB
	eventID            string
	ackBeforePublished bool
}

func (p *ackObservingAssetPublisher) Send(
	ctx context.Context,
	key string,
	payload []byte,
	headers ...commonkafka.MessageHeader,
) error {
	if err := p.delegate.Send(ctx, key, payload, headers...); err != nil {
		return err
	}
	var status string
	var publishedAt sql.NullTime
	if err := p.db.QueryRowContext(ctx,
		`SELECT status,published_at FROM asset_event_outbox WHERE event_id=$1`,
		p.eventID,
	).Scan(&status, &publishedAt); err != nil {
		return err
	}
	p.ackBeforePublished = status == "processing" && !publishedAt.Valid
	if !p.ackBeforePublished {
		return errors.New("broker ACK was not observed before published transition")
	}
	return nil
}

type failingAssetPublisher struct{ err error }

func (p failingAssetPublisher) Send(
	context.Context,
	string,
	[]byte,
	...commonkafka.MessageHeader,
) error {
	return p.err
}

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
	observedPublisher := &ackObservingAssetPublisher{
		delegate: producer, db: db, eventID: upsert.EventID,
	}
	dispatcher, err := assetRepository.NewAssetOutboxDispatcher(db, observedPublisher, assetRepository.OutboxDispatcherConfig{
		WorkerID: "asset-projection-real-kafka-dispatcher", Lease: 10 * time.Second,
		MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond, TenantID: assetProjectionRealKafkaTenant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	assertAssetOutboxState(t, db, upsert.EventID, "pending", false, 0)
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
	handlerErrors := make(chan error, 4)
	go func() {
		consumerDone <- kafkaConsumer.Consume(consumeCtx, func(handlerCtx context.Context, message *commonkafka.ReceivedMessage) error {
			handleErr := eventConsumer.Handle(handlerCtx, message)
			if handleErr != nil {
				select {
				case handlerErrors <- handleErr:
				default:
				}
			}
			return handleErr
		})
	}()
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
	if !observedPublisher.ackBeforePublished {
		t.Fatal("broker ACK must return while the durable outbox is still processing")
	}
	t.Log("TOPIC1_ORACLE PASS ACK_BEFORE_PUBLISHED")
	first := waitForAssetProjectionCommitOrConsumerStop(t, committed, consumerDone, handlerErrors, upsert.EventID, 15*time.Second)
	assertAssetEventEnvelope(t, first, upsert.EventID, record, upsert.Revision)
	t.Log("TOPIC1_ORACLE PASS HEADERS_PAYLOAD")
	assertAssetProjectionInbox(t, db, upsert.EventID, first.Partition, first.Offset, 1)
	t.Log("TOPIC1_ORACLE PASS DURABLE_INBOX_OFFSET")

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
	second := waitForAssetProjectionCommitOrConsumerStop(t, committed, consumerDone, handlerErrors, upsert.EventID, 15*time.Second)
	if second.Offset <= first.Offset {
		t.Fatalf("replay offset=%d must be after first offset=%d", second.Offset, first.Offset)
	}
	assertAssetProjectionInbox(t, db, upsert.EventID, first.Partition, first.Offset, 1)
	if kafkaConsumer.GetMetrics().CommitsSucceeded < 2 {
		t.Fatalf("commits_succeeded=%d want>=2", kafkaConsumer.GetMetrics().CommitsSucceeded)
	}
	t.Log("TOPIC1_ORACLE PASS EXACT_REPLAY_IDEMPOTENT")
}

func TestAssetProjectionKafkaPublishFailureKeepsOutboxPending(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ASSET_PROJECTION_EPHEMERAL_PG_DSN"))
	if dsn == "" {
		t.Skip("explicit ephemeral PostgreSQL setting is required")
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
	cleanupAssetProjectionKafkaTenantID(t, db, assetProjectionFailureTenant)
	defer cleanupAssetProjectionKafkaTenantID(t, db, assetProjectionFailureTenant)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Projection Publish Failure')`, assetProjectionFailureTenant); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	record := &config.AssetRecord{
		TenantID: assetProjectionFailureTenant, MACAddress: "02:00:00:00:02:02",
		IPAddress: "192.0.2.202", Hostname: "asset-publish-failure",
		AssetType: "server", Status: "active", Source: "integration", Criticality: 3,
	}
	upsert, err := repo.UpsertAtomic(ctx, record, config.AssetUpsertCommand{
		ExpectedRevision: 0, IdempotencyKey: "asset-projection-publish-failure",
		Actor: "integration-runner", TraceID: "trace-asset-publish-failure",
		RequestID: "request-asset-publish-failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertAssetOutboxState(t, db, upsert.EventID, "pending", false, 0)
	dispatcher, err := assetRepository.NewAssetOutboxDispatcher(
		db,
		failingAssetPublisher{err: errors.New("deterministic broker failure")},
		assetRepository.OutboxDispatcherConfig{
			WorkerID: "asset-projection-failure-dispatcher", Lease: 10 * time.Second,
			MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond, TenantID: assetProjectionFailureTenant,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if found, dispatchErr := dispatcher.DispatchNext(ctx); !found || dispatchErr == nil {
		t.Fatalf("dispatch found=%v err=%v", found, dispatchErr)
	}

	observer, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	assertAssetOutboxState(t, observer, upsert.EventID, "pending", false, 1)
	t.Log("TOPIC1_ORACLE PASS PUBLISH_FAILURE_PENDING")
}

func waitForAssetProjectionCommitOrConsumerStop(
	t *testing.T,
	messages <-chan segmentkafka.Message,
	consumerDone <-chan error,
	handlerErrors <-chan error,
	eventID string,
	timeout time.Duration,
) segmentkafka.Message {
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
		case err := <-consumerDone:
			t.Fatalf("asset projection consumer stopped before commit: %v", err)
		case err := <-handlerErrors:
			t.Fatalf("asset projection consumer rejected event %s before commit: %v", eventID, err)
		case <-timer.C:
			t.Fatalf("timed out waiting for committed asset event %s", eventID)
		}
	}
}

func waitForAssetProjectionCommit(
	t *testing.T,
	messages <-chan segmentkafka.Message,
	eventID string,
	timeout time.Duration,
) segmentkafka.Message {
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

func assertAssetOutboxState(t *testing.T, db *sql.DB, eventID, wantStatus string, wantPublished bool, wantAttempts int) {
	t.Helper()
	var status string
	var publishedAt sql.NullTime
	var attempts int
	if err := db.QueryRow(
		`SELECT status,published_at,attempt_count FROM asset_event_outbox WHERE event_id=$1`,
		eventID,
	).Scan(&status, &publishedAt, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || publishedAt.Valid != wantPublished || attempts != wantAttempts {
		t.Fatalf("outbox status=%q published=%v attempts=%d want=%q/%v/%d", status, publishedAt.Valid, attempts, wantStatus, wantPublished, wantAttempts)
	}
}

func assertAssetEventEnvelope(t *testing.T, message segmentkafka.Message, eventID string, record *config.AssetRecord, revision int64) {
	t.Helper()
	var event AssetUpsertedV2
	if err := json.Unmarshal(message.Value, &event); err != nil {
		t.Fatal(err)
	}
	if event.EventID != eventID || event.EventType != "traffic.asset.v2.AssetUpserted" ||
		event.SchemaVersion != 2 || event.AggregateVersion != revision ||
		event.Revision != revision || event.TenantID != record.TenantID ||
		event.PartitionKey != record.TenantID+":"+event.AssetID || event.TraceID != "trace-asset-projection-real-kafka" {
		t.Fatalf("unexpected asset event envelope: %+v", event)
	}
	expected := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "2", "aggregate_version": "1",
		"tenant_id": event.TenantID, "asset_id": event.AssetID, "trace_id": event.TraceID,
	}
	observed := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		if _, duplicate := observed[header.Key]; duplicate {
			t.Fatalf("duplicate Kafka header %q", header.Key)
		}
		observed[header.Key] = string(header.Value)
	}
	if len(observed) != len(expected) {
		t.Fatalf("Kafka header count=%d want=%d: %v", len(observed), len(expected), observed)
	}
	for key, want := range expected {
		if observed[key] != want {
			t.Fatalf("Kafka header %s=%q want=%q", key, observed[key], want)
		}
	}
}

func cleanupAssetProjectionKafkaTenant(t *testing.T, db *sql.DB) {
	cleanupAssetProjectionKafkaTenantID(t, db, assetProjectionRealKafkaTenant)
}

func cleanupAssetProjectionKafkaTenantID(t *testing.T, db *sql.DB, tenantID string) {
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
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Fatalf("cleanup asset projection tenant: %v", err)
		}
	}
}

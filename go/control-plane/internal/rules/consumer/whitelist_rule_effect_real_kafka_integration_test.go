package consumer

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	alertwhitelist "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// TestWhitelistEventPipelineRealKafka proves the production dispatcher and
// consumer handler across an actual Kafka protocol boundary. It only accepts
// an explicitly marked disposable PostgreSQL database and loopback broker.
func TestWhitelistEventPipelineRealKafka(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("WHITELIST_EVENT_PIPELINE_EPHEMERAL_PG_DSN"))
	broker := strings.TrimSpace(os.Getenv("WHITELIST_EVENT_PIPELINE_EPHEMERAL_KAFKA_BROKER"))
	if dsn == "" || broker == "" {
		t.Skip("explicit ephemeral PostgreSQL and Kafka settings are required")
	}
	if os.Getenv("WHITELIST_EVENT_PIPELINE_EPHEMERAL_KAFKA_SENTINEL") != "ephemeral-only" {
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
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_whitelist_event_pipeline_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tenantID := "whitelist-real-kafka-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "Whitelist Real Kafka Integration"); err != nil {
		t.Fatal(err)
	}
	defer cleanupWhitelistPipelineIntegration(t, db, tenantID)

	repo := alertwhitelist.NewRepository(db, zap.NewNop())
	creator := "00000000-0000-0000-0000-000000000301"
	reviewer := "00000000-0000-0000-0000-000000000302"
	expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	entry := &alertwhitelist.Entry{
		TenantID: tenantID, Type: "ip", Value: "192.0.2.40", Reason: "verified false positive",
		Description: "real Kafka pipeline integration", OwnerRole: "security-operations",
		Scope: "tenant", RiskLevel: "medium", CreatedBy: creator, ExpiresAt: &expires,
	}
	if _, err := repo.CreateAtomic(ctx, entry,
		whitelistPipelineMeta(tenantID, creator, alertwhitelist.ActionCreate, "whitelist-real-kafka-create-0001", 0),
		whitelistPipelineAudit(creator)); err != nil {
		t.Fatal(err)
	}
	pending := "pending"
	if _, err := repo.UpdateAtomic(ctx, entry.ID,
		alertwhitelist.UpdateRequest{Status: &pending, ApprovalStatus: &pending},
		whitelistPipelineMeta(tenantID, creator, alertwhitelist.ActionSubmitApproval, "whitelist-real-kafka-submit-0001", 1),
		whitelistPipelineAudit(creator)); err != nil {
		t.Fatal(err)
	}
	active, approved := "active", "approved"
	approveMeta := whitelistPipelineMeta(tenantID, reviewer, alertwhitelist.ActionApprove, "whitelist-real-kafka-approve-0001", 2)
	approveMeta.ApprovalAuthorized = true
	if _, err := repo.UpdateAtomic(ctx, entry.ID,
		alertwhitelist.UpdateRequest{Status: &active, ApprovalStatus: &approved},
		approveMeta, whitelistPipelineAudit(reviewer)); err != nil {
		t.Fatal(err)
	}

	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: DefaultWhitelistEventTopicV2,
		BatchSize: 1, BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3,
		RequiredAcks: "all", Compression: "none", Async: false,
		IdempotentKey: "tenant_id+entry_id",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	dispatcher, err := alertwhitelist.NewOutboxDispatcher(db, producer, alertwhitelist.OutboxDispatcherConfig{
		WorkerID: "whitelist-real-kafka-dispatcher", Lease: 10 * time.Second,
		MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := NewPostgresWhitelistRuleProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := projection.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}

	kafkaConsumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: []string{broker}, Topic: DefaultWhitelistEventTopicV2,
		GroupID:  "whitelist-rule-effects-real-" + uuid.NewString(),
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 100 * time.Millisecond,
		StartOffset: segmentkafka.FirstOffset, MaxRetries: 1, RetryBackoff: 25 * time.Millisecond,
		CommitOnHandlerError: false, DLQPermanentOnly: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	eventConsumer, err := NewWhitelistRuleEffectConsumer(kafkaConsumer, projection, DefaultWhitelistEventTopicV2, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	consumeCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)
	committed := make(chan segmentkafka.Message, 16)
	kafkaConsumer.SetCommitObserver(func(messages []segmentkafka.Message) {
		for _, message := range messages {
			committed <- message
		}
	})
	go func() { consumerDone <- eventConsumer.Start(consumeCtx) }()
	defer func() {
		stopConsumer()
		_ = eventConsumer.Close()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
		}
	}()

	for index := 0; index < 3; index++ {
		found, dispatchErr := dispatcher.DispatchNext(ctx)
		if dispatchErr != nil || !found {
			t.Fatalf("dispatch %d found=%v err=%v", index, found, dispatchErr)
		}
	}
	approvedMessage := waitForCommittedWhitelistEvent(t, committed, "traffic.whitelist.v2.EntryApproved", 15*time.Second)
	waitForWhitelistProjectionState(t, ctx, db, tenantID, entry.ID, "effective", 3)
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.20", "fingerprint-real"); err != nil || !matched {
		t.Fatalf("entry did not match after real Kafka effective projection: matched=%v err=%v", matched, err)
	}

	disabled := "disabled"
	if _, err := repo.UpdateAtomic(ctx, entry.ID, alertwhitelist.UpdateRequest{Status: &disabled},
		whitelistPipelineMeta(tenantID, reviewer, alertwhitelist.ActionDisable, "whitelist-real-kafka-disable-0001", 3),
		whitelistPipelineAudit(reviewer)); err != nil {
		t.Fatal(err)
	}
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.20", "fingerprint-real"); err != nil || matched {
		t.Fatalf("disabled current version matched before real Kafka revocation ACK: matched=%v err=%v", matched, err)
	}
	if found, dispatchErr := dispatcher.DispatchNext(ctx); dispatchErr != nil || !found {
		t.Fatalf("revocation dispatch found=%v err=%v", found, dispatchErr)
	}
	revokedMessage := waitForCommittedWhitelistEvent(t, committed, "traffic.whitelist.v2.EntryRevoked", 15*time.Second)
	waitForWhitelistProjectionState(t, ctx, db, tenantID, entry.ID, "revoked", 4)

	replayWriter := segmentkafka.NewWriter(segmentkafka.WriterConfig{
		Brokers: []string{broker}, Balancer: &segmentkafka.Hash{},
		RequiredAcks: int(segmentkafka.RequireAll), MaxAttempts: 3,
	})
	defer replayWriter.Close()
	approvedMessage.Topic = DefaultWhitelistEventTopicV2
	approvedMessage.Partition = 0
	approvedMessage.Offset = 0
	if err := replayWriter.WriteMessages(ctx, approvedMessage); err != nil {
		t.Fatal(err)
	}
	replayedMessage := waitForCommittedWhitelistEvent(t, committed, "traffic.whitelist.v2.EntryApproved", 15*time.Second)
	if replayedMessage.Offset <= revokedMessage.Offset {
		t.Fatalf("replayed approval offset=%d is not after revocation offset=%d", replayedMessage.Offset, revokedMessage.Offset)
	}
	waitForWhitelistProjectionState(t, ctx, db, tenantID, entry.ID, "revoked", 4)

	var outboxPublished, effectsApplied int
	var projectedPartition int
	var projectedOffset int64
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM whitelist_event_outbox WHERE tenant_id=$1 AND status='published'),
		(SELECT count(*) FROM whitelist_rule_effects WHERE tenant_id=$1 AND status='applied'),
		(SELECT kafka_partition FROM whitelist_rule_projection WHERE tenant_id=$1 AND entry_id=$2),
		(SELECT kafka_offset FROM whitelist_rule_projection WHERE tenant_id=$1 AND entry_id=$2)`,
		tenantID, entry.ID).Scan(&outboxPublished, &effectsApplied, &projectedPartition, &projectedOffset); err != nil {
		t.Fatal(err)
	}
	metrics := kafkaConsumer.GetMetrics()
	if outboxPublished != 4 || effectsApplied != 2 || projectedPartition != revokedMessage.Partition ||
		projectedOffset != revokedMessage.Offset || metrics.CommitsSucceeded < 5 || metrics.LastOffset != replayedMessage.Offset {
		t.Fatalf("reconcile outbox=%d effects=%d projection=%d/%d revoke=%d/%d commits=%d last=%d replay=%d",
			outboxPublished, effectsApplied, projectedPartition, projectedOffset,
			revokedMessage.Partition, revokedMessage.Offset, metrics.CommitsSucceeded,
			metrics.LastOffset, replayedMessage.Offset)
	}
}

func waitForCommittedWhitelistEvent(t *testing.T, messages <-chan segmentkafka.Message, eventType string, timeout time.Duration) segmentkafka.Message {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case message := <-messages:
			for _, header := range message.Headers {
				if header.Key == "event_type" && string(header.Value) == eventType {
					return message
				}
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for committed whitelist event %s", eventType)
		}
	}
}

func waitForWhitelistProjectionState(t *testing.T, ctx context.Context, db *sql.DB, tenantID, entryID, desired string, version int64) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		var currentState string
		var currentVersion int64
		err := db.QueryRowContext(ctx, `SELECT desired_state,entry_version FROM whitelist_rule_projection
			WHERE tenant_id=$1 AND entry_id=$2`, tenantID, entryID).Scan(&currentState, &currentVersion)
		if err == nil && currentState == desired && currentVersion == version {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("projection did not reach %s/%d: state=%q version=%d err=%v", desired, version, currentState, currentVersion, err)
		case <-ticker.C:
		}
	}
}

package consumer

import (
	"context"
	"database/sql"
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

const whitelistLifecycleContractSHA256 = "d87787272d140c8529686ce45eef30f2a6345fb7f2e918a450399c8f698aad49"

func TestWhitelistGovernanceEphemeralKubernetes(t *testing.T) {
	if os.Getenv("WHITELIST_GOVERNANCE_K8S_INTEGRATION") != "run-scoped-only" {
		t.Skip("run-scoped Kubernetes sentinel is required")
	}
	dsn := strings.TrimSpace(os.Getenv("WHITELIST_GOVERNANCE_K8S_PG_DSN"))
	broker := strings.TrimSpace(os.Getenv("WHITELIST_GOVERNANCE_K8S_KAFKA_BROKER"))
	runID := strings.TrimSpace(os.Getenv("WHITELIST_GOVERNANCE_K8S_RUN_ID"))
	if dsn == "" || broker == "" || runID == "" {
		t.Fatal("Kubernetes PostgreSQL, Kafka and run identity are required")
	}
	parsedRunID, err := uuid.Parse(runID)
	if err != nil || parsedRunID.String() != runID {
		t.Fatalf("canonical Kubernetes run UUID is required: %q", runID)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_whitelist_governance_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-ephemeral PostgreSQL: marker=%q err=%v", marker, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tenantID := "m09-n018-" + strings.ReplaceAll(runID, "-", "")[:12]
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "M09 N018 whitelist governance"); err != nil {
		t.Fatal(err)
	}
	defer cleanupWhitelistPipelineIntegration(t, db, tenantID)

	consumerGroup := "m09-n018-whitelist-" + uuid.NewString()
	candidateSHA256 := strings.Repeat("a", 64)
	readiness := alertwhitelist.ProducerReadiness{
		Topic: DefaultWhitelistEventTopicV2, ConsumerGroup: consumerGroup,
		CandidateSHA256: candidateSHA256, ContractSHA256: whitelistLifecycleContractSHA256,
	}
	if err := alertwhitelist.VerifyProducerReadiness(ctx, db, readiness); err == nil {
		t.Fatal("producer readiness must fail before a broker projection receipt")
	}

	repo := alertwhitelist.NewRepository(db, zap.NewNop())
	creator := "00000000-0000-0000-0000-000000000401"
	reviewer := "00000000-0000-0000-0000-000000000402"
	expires := time.Now().UTC().Add(10 * time.Second).Truncate(time.Microsecond)
	entry := &alertwhitelist.Entry{
		TenantID: tenantID, Type: "ip", Value: "192.0.2.81", Reason: "model feedback confirmed false positive",
		Description: "M09 N018 run-scoped Kubernetes lifecycle", SourceAlertID: "alert-" + runID,
		FeedbackID: "feedback-" + runID, OwnerRole: "security-operations", Scope: "tenant",
		RiskLevel: "high", CreatedBy: creator, ExpiresAt: &expires,
	}
	if _, err := repo.CreateAtomic(ctx, entry,
		whitelistPipelineMeta(tenantID, creator, alertwhitelist.ActionCreate, "m09-n018-whitelist-create-0001", 0),
		whitelistPipelineAudit(creator)); err != nil {
		t.Fatal(err)
	}
	pending := "pending"
	if _, err := repo.UpdateAtomic(ctx, entry.ID,
		alertwhitelist.UpdateRequest{Status: &pending, ApprovalStatus: &pending},
		whitelistPipelineMeta(tenantID, creator, alertwhitelist.ActionSubmitApproval, "m09-n018-whitelist-submit-0001", 1),
		whitelistPipelineAudit(creator)); err != nil {
		t.Fatal(err)
	}
	active, approved := "active", "approved"
	approveMeta := whitelistPipelineMeta(tenantID, reviewer, alertwhitelist.ActionApprove, "m09-n018-whitelist-approve-0001", 2)
	approveMeta.ApprovalAuthorized = true
	if _, err := repo.UpdateAtomic(ctx, entry.ID,
		alertwhitelist.UpdateRequest{Status: &active, ApprovalStatus: &approved}, approveMeta,
		whitelistPipelineAudit(reviewer)); err != nil {
		t.Fatal(err)
	}

	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: DefaultWhitelistEventTopicV2,
		BatchSize: 1, BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3,
		RequiredAcks: "all", Compression: "none", Async: false, IdempotentKey: "tenant_id+entry_id",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	dispatcher, err := alertwhitelist.NewOutboxDispatcher(db, producer, alertwhitelist.OutboxDispatcherConfig{
		WorkerID: "m09-n018-whitelist-dispatcher", Lease: 10 * time.Second,
		MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := NewPostgresWhitelistRuleProjectionWithReadiness(db, WhitelistConsumerReadinessOptions{
		ConsumerGroup: consumerGroup, CandidateSHA256: candidateSHA256,
		ContractSHA256: whitelistLifecycleContractSHA256,
	})
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
		Brokers: []string{broker}, Topic: DefaultWhitelistEventTopicV2, GroupID: consumerGroup,
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
	if err := alertwhitelist.VerifyProducerReadiness(ctx, db, readiness); err != nil {
		t.Fatalf("consumer broker projection receipt did not authorize producer: %v", err)
	}
	current, err := repo.Get(ctx, tenantID, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	approvedRevision := current.RuleRevision
	if current.Reason != entry.Reason || current.RuleEffectStatus != alertwhitelist.RuleEffectApplied ||
		len(current.RuleRevision) != 64 || current.RuleAckEventID == "" || current.RuleAcknowledgedAt == nil ||
		current.RuleKafkaPartition == nil || current.RuleKafkaOffset == nil ||
		*current.RuleKafkaPartition != approvedMessage.Partition || *current.RuleKafkaOffset != approvedMessage.Offset {
		t.Fatalf("approved whitelist ACK snapshot is incomplete: %+v", current)
	}
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.81", "fingerprint-k8s"); err != nil || !matched {
		t.Fatalf("approved and ACKed entry did not affect detection governance: matched=%v err=%v", matched, err)
	}

	if delay := time.Until(expires.Add(150 * time.Millisecond)); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			t.Fatal(ctx.Err())
		}
	}
	changed, err := repo.ExpireDue(ctx, 10)
	if err != nil || changed != 1 {
		t.Fatalf("expiry sweep changed=%d err=%v", changed, err)
	}
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.81", "fingerprint-k8s"); err != nil || matched {
		t.Fatalf("expired entry still affected detection before revocation ACK: matched=%v err=%v", matched, err)
	}
	if found, dispatchErr := dispatcher.DispatchNext(ctx); dispatchErr != nil || !found {
		t.Fatalf("expiry dispatch found=%v err=%v", found, dispatchErr)
	}
	expiredMessage := waitForCommittedWhitelistEvent(t, committed, "traffic.whitelist.v2.EntryExpired", 15*time.Second)
	waitForWhitelistProjectionState(t, ctx, db, tenantID, entry.ID, "revoked", 4)
	current, err = repo.Get(ctx, tenantID, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "disabled" || current.RuleDesiredState != "revoked" ||
		current.RuleEffectStatus != alertwhitelist.RuleEffectApplied || current.RuleRevision == approvedRevision ||
		current.RuleKafkaPartition == nil || current.RuleKafkaOffset == nil ||
		*current.RuleKafkaPartition != expiredMessage.Partition || *current.RuleKafkaOffset != expiredMessage.Offset {
		t.Fatalf("expired whitelist revocation ACK snapshot is incomplete: %+v", current)
	}

	var history, audit, published, applied, readinessRows int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM whitelist_entry_versions WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='whitelist'),
		(SELECT count(*) FROM whitelist_event_outbox WHERE tenant_id=$1 AND status='published'),
		(SELECT count(*) FROM whitelist_rule_effects WHERE tenant_id=$1 AND status='applied'),
		(SELECT count(*) FROM whitelist_consumer_readiness_receipt WHERE consumer_group=$2
		  AND candidate_sha256=$3 AND contract_sha256=$4 AND state='READY')`,
		tenantID, consumerGroup, candidateSHA256, whitelistLifecycleContractSHA256,
	).Scan(&history, &audit, &published, &applied, &readinessRows); err != nil {
		t.Fatal(err)
	}
	if history != 4 || audit != 4 || published != 4 || applied != 2 || readinessRows != 1 {
		t.Fatalf("reconcile history=%d audit=%d published=%d applied=%d readiness=%d",
			history, audit, published, applied, readinessRows)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM whitelist_consumer_readiness_receipt WHERE consumer_group=$1`, consumerGroup); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS consumer_ready=true producer_admitted=true draft_from_feedback=true two_person=true expiry=true projection_ack=true rule_revision=true network_blocking=false")
}

package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	alertwhitelist "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type fakeWhitelistProjectionApplier struct {
	inputs []WhitelistRuleProjectionInput
	err    error
}

func (a *fakeWhitelistProjectionApplier) ApplyWhitelistRuleProjection(_ context.Context, input WhitelistRuleProjectionInput) error {
	a.inputs = append(a.inputs, input)
	return a.err
}

func whitelistLifecycleMessage(t *testing.T, eventType, desired, status, approval string) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id": "11111111-1111-4111-8111-111111111111", "event_type": eventType,
		"schema_version": 2, "tenant_id": "tenant-a",
		"entry_id": "22222222-2222-4222-8222-222222222222", "aggregate_version": int64(3),
		"action_id": "whitelist-approve", "reason": "reviewed false positive", "trace_id": "trace-a",
		"desired_rule_state": desired, "occurred_at": "2026-08-07T10:00:00Z",
		"entry": map[string]interface{}{
			"id": "22222222-2222-4222-8222-222222222222", "tenant_id": "tenant-a",
			"type": "ip", "value": "192.0.2.10", "status": status,
			"approval_status": approval, "scope": "tenant", "version": int64(3),
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := make([]segmentkafka.Header, 0, 9)
	for key, value := range map[string]string{
		"event_id": "11111111-1111-4111-8111-111111111111", "event_type": eventType,
		"schema_version": "2", "aggregate_version": "3", "tenant_id": "tenant-a",
		"entry_id": "22222222-2222-4222-8222-222222222222", "action_id": "whitelist-approve",
		"desired_rule_state": desired, "trace_id": "trace-a",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic:     DefaultWhitelistEventTopicV2,
		Key:       []byte("tenant-a:22222222-2222-4222-8222-222222222222"),
		Partition: 2, Offset: 17, Value: payload, Headers: headers,
	}}
}

func TestWhitelistRuleConsumerAppliesValidatedEffectiveEvent(t *testing.T) {
	applier := &fakeWhitelistProjectionApplier{}
	consumer := &WhitelistRuleEffectConsumer{applier: applier, topic: DefaultWhitelistEventTopicV2}
	message := whitelistLifecycleMessage(t, "traffic.whitelist.v2.EntryApproved", "effective", "active", "approved")
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want=1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.DesiredState != "effective" || input.EntryVersion != 3 || input.KafkaOffset != 17 ||
		len(input.RuleRevision) != 64 || input.AckEventID == "" || len(input.PayloadSHA256) != 64 {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestWhitelistRuleConsumerValidatesNonEffectLifecycleWithoutApplying(t *testing.T) {
	applier := &fakeWhitelistProjectionApplier{}
	consumer := &WhitelistRuleEffectConsumer{applier: applier, topic: DefaultWhitelistEventTopicV2}
	message := whitelistLifecycleMessage(t, "traffic.whitelist.v2.EntryDrafted", "", "draft", "draft")
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 0 {
		t.Fatal("non-effect lifecycle event reached projection applier")
	}
}

func TestWhitelistRuleConsumerRejectsHeaderBodyMismatch(t *testing.T) {
	applier := &fakeWhitelistProjectionApplier{}
	consumer := &WhitelistRuleEffectConsumer{applier: applier, topic: DefaultWhitelistEventTopicV2}
	message := whitelistLifecycleMessage(t, "traffic.whitelist.v2.EntryApproved", "effective", "active", "approved")
	message.Headers = append(message.Headers, segmentkafka.Header{Key: "tenant_id", Value: []byte("tenant-b")})
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected header/body mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestWhitelistRuleConsumerPropagatesProjectionFailure(t *testing.T) {
	applier := &fakeWhitelistProjectionApplier{err: errors.New("database unavailable")}
	consumer := &WhitelistRuleEffectConsumer{applier: applier, topic: DefaultWhitelistEventTopicV2}
	message := whitelistLifecycleMessage(t, "traffic.whitelist.v2.EntryApproved", "effective", "active", "approved")
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected projection failure")
	}
}

func TestPostgresWhitelistProjectionCommitsProjectionAndAckTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresWhitelistRuleProjection(db)
	input := WhitelistRuleProjectionInput{
		EventID: "11111111-1111-4111-8111-111111111111", TenantID: "tenant-a",
		EntryID: "22222222-2222-4222-8222-222222222222", EntryVersion: 3,
		DesiredState: "effective", EntryType: "ip", MatchValue: "192.0.2.10", Scope: "tenant",
		RuleRevision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AckEventID:     "33333333-3333-4333-8333-333333333333",
		PayloadSHA256:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		KafkaPartition: 2, KafkaOffset: 17,
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO whitelist_rule_projection").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE whitelist_rule_effects").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := projection.ApplyWhitelistRuleProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresWhitelistProjectionRollsBackWhenAckDoesNotExist(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresWhitelistRuleProjection(db)
	input := WhitelistRuleProjectionInput{
		EventID: "11111111-1111-4111-8111-111111111111", TenantID: "tenant-a",
		EntryID: "22222222-2222-4222-8222-222222222222", EntryVersion: 3,
		DesiredState: "effective", EntryType: "ip", MatchValue: "192.0.2.10",
		RuleRevision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AckEventID:     "33333333-3333-4333-8333-333333333333",
		PayloadSHA256:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		KafkaPartition: 2, KafkaOffset: 17,
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO whitelist_rule_projection").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE whitelist_rule_effects").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status,ack_event_id,rule_revision").WillReturnError(errors.New("missing effect"))
	mock.ExpectRollback()
	if err := projection.ApplyWhitelistRuleProjection(context.Background(), input); err == nil {
		t.Fatal("expected missing effect failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type capturedWhitelistMessage struct {
	key     string
	payload []byte
	headers []commonkafka.MessageHeader
}

type capturingWhitelistPublisher struct{ messages []capturedWhitelistMessage }

func (p *capturingWhitelistPublisher) Send(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error {
	p.messages = append(p.messages, capturedWhitelistMessage{
		key: key, payload: append([]byte(nil), payload...), headers: append([]commonkafka.MessageHeader(nil), headers...),
	})
	return nil
}

func TestWhitelistEventPipelineEphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("WHITELIST_EVENT_PIPELINE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("WHITELIST_EVENT_PIPELINE_EPHEMERAL_PG_DSN is not set")
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantID := "whitelist-pipeline-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "Whitelist Pipeline Integration"); err != nil {
		t.Fatal(err)
	}
	defer cleanupWhitelistPipelineIntegration(t, db, tenantID)

	repo := alertwhitelist.NewRepository(db, zap.NewNop())
	creator := "00000000-0000-0000-0000-000000000201"
	reviewer := "00000000-0000-0000-0000-000000000202"
	expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	entry := &alertwhitelist.Entry{
		TenantID: tenantID, Type: "ip", Value: "192.0.2.10", Reason: "verified false positive",
		Description: "pipeline integration", OwnerRole: "security-operations", Scope: "tenant",
		RiskLevel: "medium", CreatedBy: creator, ExpiresAt: &expires,
	}
	createMeta := whitelistPipelineMeta(tenantID, creator, alertwhitelist.ActionCreate, "whitelist-pipeline-create-0001", 0)
	if _, err := repo.CreateAtomic(ctx, entry, createMeta, whitelistPipelineAudit(creator)); err != nil {
		t.Fatal(err)
	}
	pending := "pending"
	if _, err := repo.UpdateAtomic(ctx, entry.ID,
		alertwhitelist.UpdateRequest{Status: &pending, ApprovalStatus: &pending},
		whitelistPipelineMeta(tenantID, creator, alertwhitelist.ActionSubmitApproval, "whitelist-pipeline-submit-0001", 1),
		whitelistPipelineAudit(creator)); err != nil {
		t.Fatal(err)
	}
	active, approved := "active", "approved"
	approveMeta := whitelistPipelineMeta(tenantID, reviewer, alertwhitelist.ActionApprove, "whitelist-pipeline-approve-0001", 2)
	approveMeta.ApprovalAuthorized = true
	if _, err := repo.UpdateAtomic(ctx, entry.ID,
		alertwhitelist.UpdateRequest{Status: &active, ApprovalStatus: &approved}, approveMeta,
		whitelistPipelineAudit(reviewer)); err != nil {
		t.Fatal(err)
	}
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.20", "fingerprint-a"); err != nil || matched {
		t.Fatalf("entry matched before rule projection: matched=%v err=%v", matched, err)
	}

	publisher := &capturingWhitelistPublisher{}
	dispatcher, err := alertwhitelist.NewOutboxDispatcher(db, publisher, alertwhitelist.OutboxDispatcherConfig{
		WorkerID: "whitelist-pipeline-integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := NewPostgresWhitelistRuleProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	eventConsumer := &WhitelistRuleEffectConsumer{applier: projection, topic: DefaultWhitelistEventTopicV2, logger: zap.NewNop()}
	consumeCaptured := func(index int, offset int64) {
		t.Helper()
		message := publisher.messages[index]
		headers := make([]segmentkafka.Header, 0, len(message.headers))
		for _, header := range message.headers {
			headers = append(headers, segmentkafka.Header{Key: header.Key, Value: []byte(header.Value)})
		}
		received := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
			Topic: DefaultWhitelistEventTopicV2, Key: []byte(message.key), Value: message.payload,
			Partition: 0, Offset: offset, Headers: headers,
		}}
		if err := eventConsumer.handle(ctx, received); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 3; index++ {
		found, dispatchErr := dispatcher.DispatchNext(ctx)
		if dispatchErr != nil || !found {
			t.Fatalf("dispatch %d found=%v err=%v", index, found, dispatchErr)
		}
		consumeCaptured(index, int64(index+1))
	}
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.20", "fingerprint-a"); err != nil || !matched {
		t.Fatalf("entry did not match after effective projection: matched=%v err=%v", matched, err)
	}

	disabled := "disabled"
	disableMeta := whitelistPipelineMeta(tenantID, reviewer, alertwhitelist.ActionDisable, "whitelist-pipeline-disable-0001", 3)
	if _, err := repo.UpdateAtomic(ctx, entry.ID, alertwhitelist.UpdateRequest{Status: &disabled}, disableMeta,
		whitelistPipelineAudit(reviewer)); err != nil {
		t.Fatal(err)
	}
	if matched, err := repo.MatchDetection(ctx, tenantID, entry.Value, "198.51.100.20", "fingerprint-a"); err != nil || matched {
		t.Fatalf("disabled current version matched before revocation ACK: matched=%v err=%v", matched, err)
	}
	found, err := dispatcher.DispatchNext(ctx)
	if err != nil || !found {
		t.Fatalf("revocation dispatch found=%v err=%v", found, err)
	}
	consumeCaptured(3, 4)
	consumeCaptured(3, 4) // exact duplicate must be idempotent
	consumeCaptured(2, 3) // late older approval must not overwrite revocation

	var outboxPublished, effectApplied int
	var desiredState string
	var projectedVersion int64
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM whitelist_event_outbox WHERE tenant_id=$1 AND status='published'),
		(SELECT count(*) FROM whitelist_rule_effects WHERE tenant_id=$1 AND status='applied'),
		(SELECT desired_state FROM whitelist_rule_projection WHERE tenant_id=$1 AND entry_id=$2),
		(SELECT entry_version FROM whitelist_rule_projection WHERE tenant_id=$1 AND entry_id=$2)`,
		tenantID, entry.ID).Scan(&outboxPublished, &effectApplied, &desiredState, &projectedVersion); err != nil {
		t.Fatal(err)
	}
	if outboxPublished != 4 || effectApplied != 2 || desiredState != "revoked" || projectedVersion != 4 {
		t.Fatalf("reconcile published=%d applied=%d state=%q version=%d",
			outboxPublished, effectApplied, desiredState, projectedVersion)
	}
}

func whitelistPipelineMeta(tenantID, actor, action, key string, version int) alertwhitelist.CommandMeta {
	return alertwhitelist.CommandMeta{
		TenantID: tenantID, ActorID: actor, ActionID: action, IdempotencyKey: key,
		ExpectedVersion: version, Reason: "pipeline integration", TraceID: "trace-" + key,
		SourceIP: "127.0.0.1", UserAgent: "whitelist-pipeline-integration",
	}
}

func whitelistPipelineAudit(actor string) alertwhitelist.AuditRecord {
	return alertwhitelist.AuditRecord{
		UserID: actor, IPAddress: "127.0.0.1", UserAgent: "whitelist-pipeline-integration",
		Detail: map[string]interface{}{"source": "ephemeral-whitelist-pipeline"},
	}
}

func cleanupWhitelistPipelineIntegration(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	for _, statement := range []string{
		`DELETE FROM whitelist_rule_projection WHERE tenant_id=$1`,
		`DELETE FROM whitelist_rule_effects WHERE tenant_id=$1`,
		`DELETE FROM whitelist_command_requests WHERE tenant_id=$1`,
		`DELETE FROM whitelist_entry_versions WHERE tenant_id=$1`,
		`DELETE FROM whitelist_event_outbox WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type='whitelist'`,
		`DELETE FROM whitelist WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Errorf("cleanup whitelist pipeline fixture: %v", err)
		}
	}
}

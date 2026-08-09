package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const (
	alertResponseRealKafkaTopic       = "alert.response.requested.v1"
	alertResponseRealKafkaDLQTopic    = "dlq.v1"
	alertResponseRealKafkaInvalidDLQ  = "invalid alert response dlq topic"
	alertResponseRealKafkaSentinelSQL = "codex_ephemeral_alert_response_real_kafka_sentinel"
)

// TestAlertResponseRealKafkaDLQBarrier exercises the production producer,
// consumer, canonical DLQ and PostgreSQL acknowledgement barrier against an
// owned Redpanda broker. The Python runner supplies fresh sentinel-protected
// endpoints; without all three sentinel values the test is skipped.
func TestAlertResponseRealKafkaDLQBarrier(t *testing.T) {
	dsn := os.Getenv("ALERT_RESPONSE_REAL_KAFKA_EPHEMERAL_PG_DSN")
	broker := os.Getenv("ALERT_RESPONSE_REAL_KAFKA_EPHEMERAL_BROKER")
	sentinel := os.Getenv("ALERT_RESPONSE_REAL_KAFKA_EPHEMERAL_SENTINEL")
	if dsn == "" || broker == "" || sentinel == "" {
		t.Skip("alert response real Kafka sentinel environment is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM ` + alertResponseRealKafkaSentinelSQL + ` LIMIT 1`).Scan(&marker); err != nil || marker != sentinel {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int64
	var providerCommand AlertResponseExecutionCommand
	providerExecutedAt := time.Now().UTC().Truncate(time.Microsecond)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		providerCalls.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("Idempotency-Key") == "" {
			t.Errorf("invalid provider request method=%s idempotency=%q", request.Method, request.Header.Get("Idempotency-Key"))
			http.Error(w, "invalid provider request", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&providerCommand); err != nil {
			t.Errorf("decode provider command: %v", err)
			http.Error(w, "invalid provider command", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(AlertResponseExecutionReceipt{
			Status: "completed", Provider: "owned-real-kafka-firewall",
			ProviderReceiptID: "provider-" + providerCommand.EventID,
			EffectState:       "confirmed", EffectIDs: []string{"rule-" + providerCommand.EventID},
			Result: map[string]interface{}{"rule_state": "active"}, ExecutedAt: providerExecutedAt,
		})
	}))
	defer providerServer.Close()
	executor, err := NewHTTPAlertResponseExecutor(providerServer.URL, "owned-real-kafka-token", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ConfigureExecutor(executor); err != nil {
		t.Fatal(err)
	}
	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: alertResponseRealKafkaTopic,
		BatchSize: 1, BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3,
		RequiredAcks: "all", Compression: "none", Async: false,
		IdempotentKey: "alert-response-real-kafka-partition-key",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()

	tenantID := "alert-response-real-kafka-" + uuid.NewString()
	actionID := "alert-response-block-ip"
	validEventID := uuid.NewString()
	validJobID := "alert-response-valid-job-" + uuid.NewString()
	validAlertID := "AL-REAL-KAFKA-VALID-" + uuid.NewString()
	validTraceID := "trace-alert-response-valid-" + uuid.NewString()
	validTarget := "198.51.100.44"
	validReason := "owned real Kafka provider integration"
	if _, err := db.ExecContext(ctx, `INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,
		 status,approval_status,revision,trace_id,idempotency_key,expected_revision,
		 detail,requested_by,approved_by,approved_at)
		VALUES ($1,$2::uuid,$3,$4,$5,'block_ip',$6,$7,false,
		 'approved_awaiting_executor','approved',2,$8,$9,0,
		 '{}'::jsonb,'operator-a','approver-b',now())`,
		validJobID, validEventID, tenantID, validAlertID, actionID, validTarget,
		validReason, validTraceID, "owned-real-kafka-"+validEventID,
	); err != nil {
		t.Fatal(err)
	}
	validEvent := alertResponseRequestedV1{
		EventID: validEventID, EventType: alertResponseRealKafkaTopic, SchemaVersion: 1,
		AggregateVersion: 2, JobID: validJobID, TenantID: tenantID, AlertID: validAlertID,
		ActionID: actionID, Action: "block_ip", Target: validTarget, Reason: validReason,
		RequestedBy: "operator-a", ApprovedBy: "approver-b",
		ApprovalReason: "independent owned real Kafka approval", TraceID: validTraceID,
	}
	validValue, err := json.Marshal(validEvent)
	if err != nil {
		t.Fatal(err)
	}
	validHeaders := []commonkafka.MessageHeader{
		{Key: "event_id", Value: validEventID},
		{Key: "event_type", Value: alertResponseRealKafkaTopic},
		{Key: "schema_version", Value: "1"},
		{Key: "aggregate_version", Value: "2"},
		{Key: "tenant_id", Value: tenantID},
		{Key: "alert_id", Value: validAlertID},
		{Key: "job_id", Value: validJobID},
		{Key: "action_id", Value: actionID},
		{Key: "trace_id", Value: validTraceID},
		{Key: "content_type", Value: "application/json"},
		{Key: "target_topic", Value: alertResponseRealKafkaTopic},
	}
	if err := producer.Send(ctx, tenantID+":"+validJobID, validValue, validHeaders...); err != nil {
		t.Fatal(err)
	}

	eventID := uuid.NewString()
	jobID := "alert-response-job-" + uuid.NewString()
	alertID := "AL-REAL-KAFKA-" + uuid.NewString()
	traceID := "trace-alert-response-poison-" + uuid.NewString()
	poisonKey := tenantID + ":" + jobID
	poisonValue := []byte(`{"event_id":`)
	headers := []commonkafka.MessageHeader{
		{Key: "event_id", Value: eventID},
		{Key: "event_type", Value: alertResponseRealKafkaTopic},
		{Key: "schema_version", Value: "1"},
		{Key: "aggregate_version", Value: "2"},
		{Key: "tenant_id", Value: tenantID},
		{Key: "alert_id", Value: alertID},
		{Key: "job_id", Value: jobID},
		{Key: "action_id", Value: actionID},
		{Key: "trace_id", Value: traceID},
		{Key: "content_type", Value: "application/json"},
		{Key: "target_topic", Value: alertResponseRealKafkaTopic},
	}
	if err := producer.Send(ctx, poisonKey, poisonValue, headers...); err != nil {
		t.Fatal(err)
	}

	groupID := "alert-response-real-kafka-" + uuid.NewString()
	var committedMessages atomic.Int64
	var maximumCommittedOffset atomic.Int64
	maximumCommittedOffset.Store(-1)
	failedConsumer, stopFailed := startAlertResponseRealKafkaConsumer(
		t, broker, groupID, projection, alertResponseRealKafkaInvalidDLQ,
		&committedMessages, &maximumCommittedOffset,
	)
	waitAlertResponseRealKafka(t, "DLQ acknowledgement failure retains source offset", func() (bool, string) {
		metrics := failedConsumer.GetMetrics()
		return committedMessages.Load() == 1 && maximumCommittedOffset.Load() == 0 &&
				metrics.MessagesFailed > 0 && metrics.ConsecutiveProcessingFailures > 0,
			fmt.Sprintf("committed=%d max=%d failed=%d processing_failures=%d",
				committedMessages.Load(), maximumCommittedOffset.Load(), metrics.MessagesFailed,
				metrics.ConsecutiveProcessingFailures)
	})
	if committedMessages.Load() != 1 || maximumCommittedOffset.Load() != 0 {
		t.Fatalf("poison source offset advanced without DLQ acknowledgement committed=%d max=%d",
			committedMessages.Load(), maximumCommittedOffset.Load())
	}
	var validStatus, validProvider, validProviderReceipt, validEffectState string
	var validReceiptTrace, validAuditAction, validAuditTrace string
	var validExternalEffect bool
	var validKafkaPartition int
	var validKafkaOffset int64
	if err := db.QueryRowContext(ctx, `SELECT a.status,r.provider,r.provider_receipt_id,r.effect_state,
		r.external_effect,r.kafka_partition,r.kafka_offset,r.trace_id,l.action,l.trace_id
		FROM alert_response_actions a
		JOIN alert_response_execution_receipts r ON r.job_id=a.job_id
		JOIN audit_logs l ON l.event_id='audit-alert-response-execution-'||r.event_id::text
		WHERE a.tenant_id=$1 AND a.job_id=$2`, tenantID, validJobID).Scan(
		&validStatus, &validProvider, &validProviderReceipt, &validEffectState,
		&validExternalEffect, &validKafkaPartition, &validKafkaOffset, &validReceiptTrace,
		&validAuditAction, &validAuditTrace,
	); err != nil {
		t.Fatal(err)
	}
	if providerCalls.Load() != 1 || validStatus != "completed" ||
		validProvider != "owned-real-kafka-firewall" || validProviderReceipt == "" ||
		validEffectState != "confirmed" || !validExternalEffect || validKafkaPartition != 0 ||
		validKafkaOffset != 0 || validReceiptTrace != validTraceID ||
		validAuditAction != "ALERT_RESPONSE_EXECUTION_COMPLETED" || validAuditTrace != validTraceID ||
		providerCommand.IdempotencyKey != "alert-response:"+validEventID ||
		providerCommand.TraceID != validTraceID || providerCommand.ApprovedBy != "approver-b" {
		t.Fatalf("real Kafka provider path did not reconcile calls=%d status=%s provider=%s receipt=%s effect=%s/%t partition=%d offset=%d receipt_trace=%s audit=%s/%s command=%+v",
			providerCalls.Load(), validStatus, validProvider, validProviderReceipt, validEffectState,
			validExternalEffect, validKafkaPartition, validKafkaOffset, validReceiptTrace,
			validAuditAction, validAuditTrace, providerCommand)
	}
	var prematureReceipts int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_response_dlq_receipts
		WHERE source_topic=$1 AND source_partition=0 AND source_offset=1`,
		alertResponseRealKafkaTopic).Scan(&prematureReceipts); err != nil || prematureReceipts != 0 {
		t.Fatalf("DLQ receipt persisted before broker acknowledgement count=%d err=%v", prematureReceipts, err)
	}
	stopFailed()

	recoveryConsumer, stopRecovery := startAlertResponseRealKafkaConsumer(
		t, broker, groupID, projection, alertResponseRealKafkaDLQTopic,
		&committedMessages, &maximumCommittedOffset,
	)
	waitAlertResponseRealKafka(t, "poison event redelivered after durable DLQ acknowledgement", func() (bool, string) {
		metrics := recoveryConsumer.GetMetrics()
		return committedMessages.Load() == 2 && maximumCommittedOffset.Load() == 1 && metrics.MessagesDLQ > 0,
			fmt.Sprintf("committed=%d max=%d dlq=%d", committedMessages.Load(), maximumCommittedOffset.Load(), metrics.MessagesDLQ)
	})
	stopRecovery()

	var payloadSHA, headerSHA, storedEventID, storedTenantID, storedJobID string
	var storedAlertID, storedActionID, storedTraceID string
	var storedAggregateVersion sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT payload_sha256,headers_sha256,event_id,tenant_id,
		job_id,alert_id,action_id,trace_id,aggregate_version
		FROM alert_response_dlq_receipts
		WHERE source_topic=$1 AND source_partition=0 AND source_offset=1`,
		alertResponseRealKafkaTopic).Scan(&payloadSHA, &headerSHA, &storedEventID, &storedTenantID,
		&storedJobID, &storedAlertID, &storedActionID, &storedTraceID, &storedAggregateVersion); err != nil {
		t.Fatal(err)
	}
	expectedPayloadDigest := sha256.Sum256(poisonValue)
	if payloadSHA != hex.EncodeToString(expectedPayloadDigest[:]) || len(headerSHA) != 64 ||
		storedEventID != eventID || storedTenantID != tenantID || storedJobID != jobID ||
		storedAlertID != alertID || storedActionID != actionID || storedTraceID != traceID ||
		!storedAggregateVersion.Valid || storedAggregateVersion.Int64 != 2 {
		t.Fatalf("invalid PostgreSQL DLQ receipt payload=%s headers=%s event=%s tenant=%s job=%s alert=%s action=%s trace=%s aggregate=%v",
			payloadSHA, headerSHA, storedEventID, storedTenantID, storedJobID, storedAlertID,
			storedActionID, storedTraceID, storedAggregateVersion)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE tenant_id=$1
		AND action='ALERT_RESPONSE_EVENT_QUARANTINED' AND detail->>'source_topic'=$2
		AND (detail->>'source_offset')::bigint=1 AND detail->>'dlq_acknowledged'='true'
		AND detail->>'source_offset_commit_pending'='true'`,
		tenantID, alertResponseRealKafkaTopic).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("alert response poison audit count=%d err=%v", auditCount, err)
	}

	reader := segmentkafka.NewReader(segmentkafka.ReaderConfig{
		Brokers: []string{broker}, Topic: alertResponseRealKafkaDLQTopic, Partition: 0,
		StartOffset: segmentkafka.FirstOffset, MinBytes: 1, MaxBytes: 1 << 20,
		MaxWait: 100 * time.Millisecond,
	})
	defer reader.Close()
	readContext, readCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readCancel()
	dlqKafkaMessage, err := reader.ReadMessage(readContext)
	if err != nil {
		t.Fatal(err)
	}
	dlqReceipt, err := commonkafka.DecodeDLQMessage(dlqKafkaMessage.Value)
	if err != nil {
		t.Fatal(err)
	}
	originalValue, err := dlqReceipt.GetOriginalValue()
	if err != nil {
		t.Fatal(err)
	}
	if dlqReceipt.OriginalTopic != alertResponseRealKafkaTopic ||
		dlqReceipt.OriginalPartition != 0 || dlqReceipt.OriginalOffset != 1 ||
		dlqReceipt.OriginalKey != poisonKey || string(originalValue) != string(poisonValue) ||
		dlqReceipt.TenantID != tenantID || dlqReceipt.EventID != eventID ||
		dlqReceipt.TraceID != traceID || dlqReceipt.ErrorCode != "PROCESSING_FAILED" ||
		dlqReceipt.ServiceName != "consumer-"+alertResponseRealKafkaTopic ||
		!strings.Contains(dlqReceipt.ErrorMessage, "decode alert response event") {
		t.Fatalf("canonical alert response DLQ payload mismatch: %+v", dlqReceipt)
	}
	t.Logf("alert_response_real_kafka_dlq=pass group=%s source_offset=1 dlq_offset=%d receipt=1 audit=1 required_acks=all redelivered=true external_execution=1 provider_calls=1 valid_trace=%s",
		groupID, dlqKafkaMessage.Offset, validTraceID)
}

func startAlertResponseRealKafkaConsumer(
	t *testing.T,
	broker string,
	groupID string,
	projection *PostgresAlertResponseProjection,
	dlqTopic string,
	committedMessages *atomic.Int64,
	maximumCommittedOffset *atomic.Int64,
) (*commonkafka.Consumer, func()) {
	t.Helper()
	consumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: []string{broker}, Topic: alertResponseRealKafkaTopic, GroupID: groupID,
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 100 * time.Millisecond,
		StartOffset: segmentkafka.FirstOffset, MaxRetries: 1, RetryBackoff: 50 * time.Millisecond,
		CommitOnHandlerError: false, EnableDLQ: true, DLQTopic: dlqTopic,
		CommitOnDLQSuccess: true, DLQPermanentOnly: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	consumer.SetCommitObserver(func(messages []segmentkafka.Message) {
		committedMessages.Add(int64(len(messages)))
		for _, message := range messages {
			for {
				current := maximumCommittedOffset.Load()
				if message.Offset <= current || maximumCommittedOffset.CompareAndSwap(current, message.Offset) {
					break
				}
			}
		}
	})
	consumer.SetDLQAcknowledgementBarrier(projection.RecordDLQAcknowledgement)
	eventConsumer, err := NewAlertResponseEventConsumer(consumer, projection, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	consumerContext, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eventConsumer.Start(consumerContext) }()
	var once sync.Once
	return consumer, func() {
		once.Do(func() {
			cancel()
			_ = eventConsumer.Close()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("alert response real Kafka consumer did not stop")
			}
		})
	}
}

func waitAlertResponseRealKafka(t *testing.T, description string, condition func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	last := "not evaluated"
	for time.Now().Before(deadline) {
		ok, detail := condition()
		last = detail
		if ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s: %s", description, last)
}

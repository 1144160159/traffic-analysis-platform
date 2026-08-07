package api

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

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const dashboardRealProviderToken = "dashboard-real-components-ephemeral-token"

// TestDashboardTaskRealComponents exercises the production PostgreSQL, Kafka
// and HTTP-provider boundaries. The Python runner supplies owned, sentinel
// PostgreSQL and Redpanda endpoints; without them this test cannot run.
func TestDashboardTaskRealComponents(t *testing.T) {
	dsn := os.Getenv("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_PG_DSN")
	broker := os.Getenv("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_KAFKA_BROKER")
	sentinel := os.Getenv("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_SENTINEL")
	if dsn == "" || broker == "" || sentinel == "" {
		t.Skip("dashboard real-component sentinel environment is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_dashboard_task_real_components_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != sentinel {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tenantID := "dashboard-real-" + uuid.NewString()
	otherTenantID := "dashboard-real-other-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,'Dashboard Real Components'),($2,'Dashboard Real Other')`, tenantID, otherTenantID); err != nil {
		t.Fatal(err)
	}

	provider := newDashboardRealProviderHarness()
	providerServer := httptest.NewServer(provider)
	defer providerServer.Close()
	executor, err := NewHTTPDashboardTaskExecutor(providerServer.URL+"/execute", dashboardRealProviderToken, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	compensator, err := NewHTTPDashboardTaskCompensator(providerServer.URL+"/compensate", dashboardRealProviderToken, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: dashboardTaskKafkaTopic, BatchSize: 1,
		BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3, RequiredAcks: "all",
		Compression: "none", Async: false, IdempotentKey: "dashboard-task-partition-key",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	pipeline, err := NewDashboardTaskPipeline(db, executor, producer.Send, dashboardTaskKafkaTopic, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.EnableCompensation(compensator); err != nil {
		t.Fatal(err)
	}
	handler := NewDashboardTaskHandler(db, zap.NewNop(), true)
	handler.EnableCompensation(true)

	groupID := "dashboard-task-real-components-" + uuid.NewString()
	var committedMessages atomic.Int64
	var maximumCommittedOffset atomic.Int64
	maximumCommittedOffset.Store(-1)
	stopConsumer := startDashboardRealConsumer(t, broker, groupID, pipeline, &committedMessages, &maximumCommittedOffset)
	defer func() { stopConsumer() }()

	completedReceipt, completedTask := runDashboardRealExecution(t, ctx, handler, pipeline, db, tenantID,
		"dashboard-real-completed", "provider-confirmed-target", false, "completed")
	if completedTask.Result["effect_state"] != "confirmed" {
		t.Fatalf("completed task lost provider effect authority: %+v", completedTask.Result)
	}
	if provider.executionCallCount() != 1 || provider.executionUniqueCount() != 1 {
		t.Fatalf("unexpected provider execution calls total=%d unique=%d", provider.executionCallCount(), provider.executionUniqueCount())
	}

	// Restart the real consumer group, republish the exact request event at a
	// new broker offset, and prove the durable inbox suppresses a second effect.
	stopConsumer()
	stopConsumer = startDashboardRealConsumer(t, broker, groupID, pipeline, &committedMessages, &maximumCommittedOffset)
	commitsBeforeReplay := committedMessages.Load()
	republishDashboardRealOutboxEvent(t, ctx, db, pipeline, completedReceipt.EventID)
	waitDashboardReal(t, "exact request replay committed", func() (bool, string) {
		return committedMessages.Load() > commitsBeforeReplay,
			fmt.Sprintf("committed=%d before=%d", committedMessages.Load(), commitsBeforeReplay)
	})
	if provider.executionCallCount() != 1 {
		t.Fatalf("exact Kafka replay duplicated provider execution: calls=%d", provider.executionCallCount())
	}

	crossTenant := performDashboardTaskCompensation(t, handler, otherTenantID, completedTask.TaskID,
		"dashboard-real-cross-tenant", DashboardTaskCompensationCreateRequest{
			ActionID: dashboardTaskCompensationAction, ExpectedRevision: completedTask.Revision,
			Reason: "cross tenant compensation must remain invisible",
		})
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant compensation status=%d body=%s", crossTenant.Code, crossTenant.Body.String())
	}
	compensation := runDashboardRealCompensation(t, ctx, handler, pipeline, db, tenantID, completedTask,
		"dashboard-real-compensated", "remove the confirmed provider effect", "compensated")
	if provider.compensationCallCount() != 1 || provider.compensationUniqueCount() != 1 {
		t.Fatalf("unexpected provider compensation calls total=%d unique=%d", provider.compensationCallCount(), provider.compensationUniqueCount())
	}
	commitsBeforeCompensationReplay := committedMessages.Load()
	republishDashboardRealOutboxEvent(t, ctx, db, pipeline, compensation.EventID)
	waitDashboardReal(t, "exact compensation replay committed", func() (bool, string) {
		return committedMessages.Load() > commitsBeforeCompensationReplay,
			fmt.Sprintf("committed=%d before=%d", committedMessages.Load(), commitsBeforeCompensationReplay)
	})
	if provider.compensationCallCount() != 1 {
		t.Fatalf("exact compensation Kafka replay duplicated provider effect: calls=%d", provider.compensationCallCount())
	}

	_, ambiguousExecution := runDashboardRealExecution(t, ctx, handler, pipeline, db, tenantID,
		"dashboard-real-execution-ambiguous", "transport-ambiguous", false, "partial")
	if ambiguousExecution.ErrorCode != "EXECUTOR_TRANSPORT_UNKNOWN" || ambiguousExecution.Result["effect_state"] != "unknown" {
		t.Fatalf("execution transport ambiguity was not preserved: %+v", ambiguousExecution)
	}

	_, compensationSource := runDashboardRealExecution(t, ctx, handler, pipeline, db, tenantID,
		"dashboard-real-compensation-source", "provider-compensation-source", true, "completed")
	ambiguousCompensation := runDashboardRealCompensation(t, ctx, handler, pipeline, db, tenantID, compensationSource,
		"dashboard-real-compensation-ambiguous", "transport-ambiguous compensation response", "compensation_partial")
	if ambiguousCompensation.Status != "compensating" {
		t.Fatalf("accepted compensation receipt changed unexpectedly: %+v", ambiguousCompensation)
	}
	compensationPartial, err := handler.getTask(ctx, tenantID, compensationSource.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if compensationPartial.ErrorCode != "COMPENSATOR_TRANSPORT_UNKNOWN" || compensationPartial.Result["compensation"] == nil {
		t.Fatalf("compensation transport ambiguity was not preserved: %+v", compensationPartial)
	}

	stopConsumer()
	waitDashboardReal(t, "all broker commits acknowledged", func() (bool, string) {
		return committedMessages.Load() >= 12 && maximumCommittedOffset.Load() >= 11,
			fmt.Sprintf("committed=%d max_offset=%d", committedMessages.Load(), maximumCommittedOffset.Load())
	})

	var tasks, history, outbox, publishedOutbox, inbox, executionAttempts, executionReceipts int
	var compensationRequests, compensationAttempts, compensationReceipts, audits int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM dashboard_tasks WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_history WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_outbox WHERE tenant_id=$1 AND status='published'),
		(SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_execution_attempts WHERE tenant_id=$1 AND status='completed'),
		(SELECT count(*) FROM dashboard_task_execution_receipts WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_compensation_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_compensation_attempts WHERE tenant_id=$1 AND status='completed'),
		(SELECT count(*) FROM dashboard_task_compensation_receipts WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='dashboard_task')`, tenantID).
		Scan(&tasks, &history, &outbox, &publishedOutbox, &inbox, &executionAttempts, &executionReceipts,
			&compensationRequests, &compensationAttempts, &compensationReceipts, &audits); err != nil {
		t.Fatal(err)
	}
	if tasks != 3 || history != 13 || outbox != 10 || publishedOutbox != 10 || inbox != 10 ||
		executionAttempts != 3 || executionReceipts != 3 || compensationRequests != 2 ||
		compensationAttempts != 2 || compensationReceipts != 2 || audits != 13 {
		t.Fatalf("real-component reconciliation tasks=%d history=%d outbox=%d published=%d inbox=%d execution_attempts=%d execution_receipts=%d compensation_requests=%d compensation_attempts=%d compensation_receipts=%d audits=%d",
			tasks, history, outbox, publishedOutbox, inbox, executionAttempts, executionReceipts,
			compensationRequests, compensationAttempts, compensationReceipts, audits)
	}
	var executionTraceCount int
	var reconciledExecutionTrace string
	if err := db.QueryRowContext(ctx, `WITH traces AS (
		SELECT trace_id FROM dashboard_tasks WHERE tenant_id=$1 AND task_id=$2
		UNION ALL SELECT trace_id FROM dashboard_task_outbox WHERE tenant_id=$1 AND task_id=$2 AND event_type IN ($3,$4)
		UNION ALL SELECT trace_id FROM dashboard_task_event_inbox WHERE tenant_id=$1 AND task_id=$2 AND event_type IN ($3,$4)
		UNION ALL SELECT trace_id FROM dashboard_task_execution_receipts WHERE tenant_id=$1 AND task_id=$2
		UNION ALL SELECT trace_id FROM audit_logs WHERE tenant_id=$1 AND object_type='dashboard_task'
			AND object_id=$2::text AND action NOT LIKE 'DASHBOARD_TASK_COMPENSATION_%'
	) SELECT count(DISTINCT trace_id),min(trace_id) FROM traces`, tenantID, completedTask.TaskID,
		dashboardTaskRequestedEvent, dashboardTaskResultEvent).
		Scan(&executionTraceCount, &reconciledExecutionTrace); err != nil {
		t.Fatal(err)
	}
	if executionTraceCount != 1 || reconciledExecutionTrace != completedReceipt.TraceID {
		t.Fatalf("execution same-trace reconciliation failed count=%d trace=%q expected=%q",
			executionTraceCount, reconciledExecutionTrace, completedReceipt.TraceID)
	}
	var compensationTraceCount int
	var reconciledCompensationTrace string
	if err := db.QueryRowContext(ctx, `WITH traces AS (
		SELECT trace_id FROM dashboard_task_outbox WHERE tenant_id=$1 AND task_id=$2 AND event_type IN ($3,$4)
		UNION ALL SELECT trace_id FROM dashboard_task_event_inbox WHERE tenant_id=$1 AND task_id=$2 AND event_type IN ($3,$4)
		UNION ALL SELECT trace_id FROM dashboard_task_compensation_requests WHERE tenant_id=$1 AND task_id=$2
		UNION ALL SELECT trace_id FROM dashboard_task_compensation_receipts WHERE tenant_id=$1 AND task_id=$2
		UNION ALL SELECT trace_id FROM audit_logs WHERE tenant_id=$1 AND object_type='dashboard_task'
			AND object_id=$2::text AND action LIKE 'DASHBOARD_TASK_COMPENSATION_%'
	) SELECT count(DISTINCT trace_id),min(trace_id) FROM traces`, tenantID, completedTask.TaskID,
		dashboardTaskCompensationRequestedEvent, dashboardTaskCompensationResultEvent).
		Scan(&compensationTraceCount, &reconciledCompensationTrace); err != nil {
		t.Fatal(err)
	}
	if compensationTraceCount != 1 || reconciledCompensationTrace != compensation.TraceID {
		t.Fatalf("compensation same-trace reconciliation failed count=%d trace=%q expected=%q",
			compensationTraceCount, reconciledCompensationTrace, compensation.TraceID)
	}
	if provider.executionCallCount() < 3 || provider.executionUniqueCount() != 3 ||
		provider.compensationCallCount() < 2 || provider.compensationUniqueCount() != 2 {
		t.Fatalf("provider reconciliation execution=%d/%d compensation=%d/%d",
			provider.executionCallCount(), provider.executionUniqueCount(),
			provider.compensationCallCount(), provider.compensationUniqueCount())
	}
	t.Logf("dashboard_real_components=pass group=%s committed_messages=%d max_offset=%d tasks=%d inbox=%d execution_trace=%s compensation_trace=%s execution_calls=%d execution_effects=%d compensation_calls=%d compensation_effects=%d",
		groupID, committedMessages.Load(), maximumCommittedOffset.Load(), tasks, inbox,
		reconciledExecutionTrace, reconciledCompensationTrace,
		provider.executionCallCount(), provider.executionUniqueCount(),
		provider.compensationCallCount(), provider.compensationUniqueCount())
}

func runDashboardRealExecution(t *testing.T, ctx context.Context, handler *DashboardTaskHandler,
	pipeline *DashboardTaskPipeline, db *sql.DB, tenantID, idempotencyKey, target string,
	proveOutOfOrder bool, expectedStatus string,
) (DashboardTaskReceipt, *DashboardTask) {
	t.Helper()
	created := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite}, idempotencyKey,
		DashboardTaskCreateRequest{Target: target, Priority: "high", SnapshotID: "snapshot-" + idempotencyKey,
			Reason: "real component dashboard task", Context: map[string]interface{}{"source": "real-components"}})
	if created.Code != http.StatusAccepted {
		t.Fatalf("create task status=%d body=%s", created.Code, created.Body.String())
	}
	receipt := decodeDashboardTaskReceipt(t, created)
	if proveOutOfOrder {
		outOfOrder := dashboardTaskLifecycleEnvelope{
			EventID: uuid.NewString(), EventType: dashboardTaskResultEvent, SchemaVersion: 1,
			AggregateType: "dashboard_task", AggregateID: receipt.TaskID, AggregateVersion: 3,
			PartitionKey: tenantID + ":" + receipt.TaskID, TenantID: tenantID, TaskID: receipt.TaskID,
			ActionID: "dashboard-task-create", Status: "completed", SnapshotID: "snapshot-" + idempotencyKey,
			Provider: "out-of-order-provider", ProviderReceiptID: "out-of-order-receipt",
			EffectState: "confirmed", EffectIDs: []string{"out-of-order-effect"},
			Result: map[string]interface{}{"invalid_order": true}, ReceiptSHA256: strings.Repeat("a", 64),
			TraceID: "trace-" + idempotencyKey, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		payload, err := json.Marshal(outOfOrder)
		if err != nil {
			t.Fatal(err)
		}
		message := dashboardTaskReceivedMessage(dashboardTaskPublishedMessage{
			Key: outOfOrder.PartitionKey, Value: payload, Headers: dashboardRealEventHeaders(outOfOrder),
		}, 0, -1)
		if err := pipeline.HandleKafkaMessage(ctx, message); err == nil || !strings.Contains(err.Error(), "not backed by terminal PostgreSQL authority") {
			t.Fatalf("out-of-order result did not fail closed: %v", err)
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_event_inbox WHERE event_id=$1`, outOfOrder.EventID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("out-of-order event leaked inbox fact count=%d err=%v", count, err)
		}
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-real-outbox-"+idempotencyKey, 10); err != nil || drained != 1 {
		t.Fatalf("drain request outbox count=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "execution attempt created", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_execution_attempts WHERE request_event_id=$1 AND status='pending'`, receipt.EventID).Scan(&count)
		return err == nil && count == 1, fmt.Sprintf("count=%d err=%v", count, err)
	})
	if drained, err := pipeline.DrainExecutions(ctx, "dashboard-real-executor-"+idempotencyKey, 10); err != nil || drained != 1 {
		t.Fatalf("drain execution count=%d err=%v", drained, err)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-real-result-"+idempotencyKey, 10); err != nil || drained != 1 {
		t.Fatalf("drain result outbox count=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "execution result consumed", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1 AND task_id=$2 AND event_type=$3`, tenantID, receipt.TaskID, dashboardTaskResultEvent).Scan(&count)
		return err == nil && count == 1, fmt.Sprintf("count=%d err=%v", count, err)
	})
	task, err := handler.getTask(ctx, tenantID, receipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != expectedStatus || task.Revision != 3 {
		t.Fatalf("unexpected real execution task: %+v", task)
	}
	return receipt, task
}

func runDashboardRealCompensation(t *testing.T, ctx context.Context, handler *DashboardTaskHandler,
	pipeline *DashboardTaskPipeline, db *sql.DB, tenantID string, task *DashboardTask,
	idempotencyKey, reason, expectedStatus string,
) DashboardTaskCompensationAccepted {
	t.Helper()
	created := performDashboardTaskCompensation(t, handler, tenantID, task.TaskID, idempotencyKey,
		DashboardTaskCompensationCreateRequest{ActionID: dashboardTaskCompensationAction,
			ExpectedRevision: task.Revision, Reason: reason})
	if created.Code != http.StatusAccepted {
		t.Fatalf("create compensation status=%d body=%s", created.Code, created.Body.String())
	}
	receipt := decodeDashboardTaskCompensationAccepted(t, created)
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-real-compensation-outbox-"+idempotencyKey, 10); err != nil || drained != 1 {
		t.Fatalf("drain compensation request count=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "compensation attempt created", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_compensation_attempts WHERE request_event_id=$1 AND status='pending'`, receipt.EventID).Scan(&count)
		return err == nil && count == 1, fmt.Sprintf("count=%d err=%v", count, err)
	})
	if drained, err := pipeline.DrainCompensations(ctx, "dashboard-real-compensator-"+idempotencyKey, 10); err != nil || drained != 1 {
		t.Fatalf("drain compensation count=%d err=%v", drained, err)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-real-compensation-result-"+idempotencyKey, 10); err != nil || drained != 1 {
		t.Fatalf("drain compensation result count=%d err=%v", drained, err)
	}
	waitDashboardReal(t, "compensation result consumed", func() (bool, string) {
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1 AND task_id=$2 AND event_type=$3`, tenantID, task.TaskID, dashboardTaskCompensationResultEvent).Scan(&count)
		return err == nil && count == 1, fmt.Sprintf("count=%d err=%v", count, err)
	})
	terminal, err := handler.getTask(ctx, tenantID, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Status != expectedStatus || terminal.Revision != 5 {
		t.Fatalf("unexpected real compensation task: %+v", terminal)
	}
	return receipt
}

func startDashboardRealConsumer(t *testing.T, broker, groupID string, pipeline *DashboardTaskPipeline,
	committedMessages, maximumCommittedOffset *atomic.Int64,
) func() {
	t.Helper()
	consumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: []string{broker}, Topic: dashboardTaskKafkaTopic, GroupID: groupID,
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 100 * time.Millisecond,
		StartOffset: segmentkafka.FirstOffset, MaxRetries: 1, RetryBackoff: 50 * time.Millisecond,
		CommitOnHandlerError: false, EnableDLQ: false,
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
	eventConsumer, err := NewDashboardTaskEventConsumer(consumer, pipeline)
	if err != nil {
		t.Fatal(err)
	}
	consumerContext, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eventConsumer.Start(consumerContext) }()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			_ = eventConsumer.Close()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("dashboard real-component consumer did not stop")
			}
		})
	}
}

func republishDashboardRealOutboxEvent(t *testing.T, ctx context.Context, db *sql.DB,
	pipeline *DashboardTaskPipeline, eventID string,
) {
	t.Helper()
	var event dashboardTaskLifecycleEnvelope
	var payload []byte
	if err := db.QueryRowContext(ctx, `SELECT payload::text FROM dashboard_task_outbox WHERE event_id=$1`, eventID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.publish(ctx, event.PartitionKey, payload, dashboardRealEventHeaders(event)...); err != nil {
		t.Fatal(err)
	}
}

func dashboardRealEventHeaders(event dashboardTaskLifecycleEnvelope) []commonkafka.MessageHeader {
	return []commonkafka.MessageHeader{
		{Key: "event_id", Value: event.EventID}, {Key: "event_type", Value: event.EventType},
		{Key: "tenant_id", Value: event.TenantID}, {Key: "task_id", Value: event.TaskID},
		{Key: "aggregate_version", Value: fmt.Sprintf("%d", event.AggregateVersion)},
		{Key: "schema_version", Value: "1"}, {Key: "trace_id", Value: event.TraceID},
		{Key: "content_type", Value: "application/json"}, {Key: "target_topic", Value: dashboardTaskKafkaTopic},
	}
}

func waitDashboardReal(t *testing.T, description string, condition func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	last := ""
	for time.Now().Before(deadline) {
		ok, detail := condition()
		last = detail
		if ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s: %s", description, last)
}

type dashboardRealProviderHarness struct {
	mu                   sync.Mutex
	executionCalls       int
	compensationCalls    int
	executionCommands    map[string]string
	compensationCommands map[string]string
}

func newDashboardRealProviderHarness() *dashboardRealProviderHarness {
	return &dashboardRealProviderHarness{
		executionCommands: make(map[string]string), compensationCommands: make(map[string]string),
	}
}

func (provider *dashboardRealProviderHarness) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer "+dashboardRealProviderToken {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch request.URL.Path {
	case "/execute":
		provider.serveExecution(writer, request)
	case "/compensate":
		provider.serveCompensation(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (provider *dashboardRealProviderHarness) serveExecution(writer http.ResponseWriter, request *http.Request) {
	var envelope struct {
		SchemaVersion int                           `json:"schema_version"`
		Command       DashboardTaskExecutionRequest `json:"command"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.SchemaVersion != 1 {
		http.Error(writer, "invalid execution envelope", http.StatusBadRequest)
		return
	}
	command := envelope.Command
	if request.Header.Get("Idempotency-Key") != command.IdempotencyKey ||
		request.Header.Get("X-Tenant-ID") != command.TenantID || request.Header.Get("X-Trace-ID") != command.TraceID {
		http.Error(writer, "execution metadata mismatch", http.StatusBadRequest)
		return
	}
	if !provider.recordExecution(command.IdempotencyKey, command) {
		http.Error(writer, "execution idempotency collision", http.StatusConflict)
		return
	}
	if command.Target == "transport-ambiguous" {
		dashboardRealCloseWithoutResponse(writer)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(DashboardTaskExecutionReceipt{
		Status: "completed", Provider: "real-loopback-provider",
		ProviderReceiptID: "execution-receipt-" + command.RequestEventID,
		EffectState:       "confirmed", EffectIDs: []string{"effect-" + command.TaskID},
		Result:     map[string]interface{}{"task_id": command.TaskID, "durable": true},
		ExecutedAt: time.Now().UTC(),
	})
}

func (provider *dashboardRealProviderHarness) serveCompensation(writer http.ResponseWriter, request *http.Request) {
	var envelope struct {
		SchemaVersion int                              `json:"schema_version"`
		Command       DashboardTaskCompensationRequest `json:"command"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.SchemaVersion != 1 {
		http.Error(writer, "invalid compensation envelope", http.StatusBadRequest)
		return
	}
	command := envelope.Command
	if request.Header.Get("Idempotency-Key") != command.CompensationIdempotency ||
		request.Header.Get("X-Tenant-ID") != command.TenantID || request.Header.Get("X-Trace-ID") != command.TraceID ||
		len(command.OriginalEffectIDs) == 0 {
		http.Error(writer, "compensation metadata mismatch", http.StatusBadRequest)
		return
	}
	if !provider.recordCompensation(command.CompensationIdempotency, command) {
		http.Error(writer, "compensation idempotency collision", http.StatusConflict)
		return
	}
	if strings.Contains(command.Reason, "transport-ambiguous") {
		dashboardRealCloseWithoutResponse(writer)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(DashboardTaskCompensationReceipt{
		Status: "compensated", Provider: "real-loopback-provider",
		ProviderReceiptID: "compensation-receipt-" + command.RequestEventID,
		EffectState:       "confirmed", CompensatedEffectIDs: append([]string(nil), command.OriginalEffectIDs...),
		Result:        map[string]interface{}{"task_id": command.TaskID, "removed": true},
		CompensatedAt: time.Now().UTC(),
	})
}

func (provider *dashboardRealProviderHarness) recordExecution(key string, command DashboardTaskExecutionRequest) bool {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.executionCalls++
	digest := dashboardRealDigest(command)
	if previous, exists := provider.executionCommands[key]; exists {
		return previous == digest
	}
	provider.executionCommands[key] = digest
	return true
}

func (provider *dashboardRealProviderHarness) recordCompensation(key string, command DashboardTaskCompensationRequest) bool {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.compensationCalls++
	digest := dashboardRealDigest(command)
	if previous, exists := provider.compensationCommands[key]; exists {
		return previous == digest
	}
	provider.compensationCommands[key] = digest
	return true
}

func dashboardRealDigest(value interface{}) string {
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func dashboardRealCloseWithoutResponse(writer http.ResponseWriter) {
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		panic("real HTTP provider does not support connection hijacking")
	}
	connection, _, err := hijacker.Hijack()
	if err != nil {
		panic(err)
	}
	_ = connection.Close()
}

func (provider *dashboardRealProviderHarness) executionCallCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.executionCalls
}

func (provider *dashboardRealProviderHarness) executionUniqueCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return len(provider.executionCommands)
}

func (provider *dashboardRealProviderHarness) compensationCallCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.compensationCalls
}

func (provider *dashboardRealProviderHarness) compensationUniqueCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return len(provider.compensationCommands)
}

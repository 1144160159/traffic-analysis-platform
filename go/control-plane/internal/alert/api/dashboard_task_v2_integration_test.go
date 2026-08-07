package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestDashboardTaskV2EphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("DASHBOARD_TASK_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("DASHBOARD_TASK_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_dashboard_task_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantID := "dashboard-" + uuid.NewString()
	otherTenantID := "dashboard-other-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,'Dashboard Integration'),($2,'Dashboard Other')`, tenantID, otherTenantID); err != nil {
		t.Fatal(err)
	}
	defer cleanupDashboardTaskIntegration(t, db, tenantID, otherTenantID)
	handler := NewDashboardTaskHandler(db, zap.NewNop(), true)

	createBody := DashboardTaskCreateRequest{
		Target: "critical-alert-queue", Priority: "critical", SnapshotID: "dashboard-snapshot-integration-1",
		Reason: "create a durable closure work item", Context: map[string]interface{}{"metric": "high-risk-open", "value": float64(7)},
	}
	created := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite},
		"dashboard-task-integration-key-0001", createBody)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	receipt := decodeDashboardTaskReceipt(t, created)
	if receipt.TaskID == "" || receipt.JobID != receipt.TaskID || receipt.Status != "accepted" || receipt.ActionID != "dashboard-task-create" || receipt.Replayed {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}

	replayed := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite},
		"dashboard-task-integration-key-0001", createBody)
	replayReceipt := decodeDashboardTaskReceipt(t, replayed)
	if replayed.Code != http.StatusAccepted || replayReceipt.TaskID != receipt.TaskID || !replayReceipt.Replayed {
		t.Fatalf("unexpected replay status=%d receipt=%+v", replayed.Code, replayReceipt)
	}

	collisionBody := createBody
	collisionBody.Target = "different-target"
	collision := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite},
		"dashboard-task-integration-key-0001", collisionBody)
	if collision.Code != http.StatusConflict {
		t.Fatalf("idempotency collision status=%d body=%s", collision.Code, collision.Body.String())
	}

	viewer := performDashboardTaskCreate(t, handler, tenantID, []string{},
		"dashboard-task-integration-key-0002", createBody)
	if viewer.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d body=%s", viewer.Code, viewer.Body.String())
	}

	getOwn := performDashboardTaskGet(handler, tenantID, receipt.TaskID)
	if getOwn.Code != http.StatusOK {
		t.Fatalf("own task status=%d body=%s", getOwn.Code, getOwn.Body.String())
	}
	getOther := performDashboardTaskGet(handler, otherTenantID, receipt.TaskID)
	if getOther.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant task status=%d body=%s", getOther.Code, getOther.Body.String())
	}

	var tasks, history, outbox, requests, audits int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM dashboard_tasks WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_history WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='dashboard_task')`, tenantID).
		Scan(&tasks, &history, &outbox, &requests, &audits); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || history != 1 || outbox != 1 || requests != 1 || audits != 1 {
		t.Fatalf("atomic facts tasks=%d history=%d outbox=%d requests=%d audits=%d", tasks, history, outbox, requests, audits)
	}

	snapshotReader := &dashboardSnapshotProductionReader{postgres: db, logger: zap.NewNop()}
	snapshotStart := time.Now().UTC().Add(-time.Hour)
	snapshotEnd := time.Now().UTC().Add(time.Minute)
	ownSnapshot := snapshotReader.ReadDashboardSnapshot(ctx, tenantID, snapshotStart, snapshotEnd)
	ownBacklog := dashboardIntegrationMetric(t, ownSnapshot.Data, "queue_backlog")
	if ownBacklog.Value == nil || *ownBacklog.Value != 1 || ownBacklog.State != "ok" {
		t.Fatalf("own tenant snapshot backlog=%+v", ownBacklog)
	}
	if ownSnapshot.SourceWatermarks["postgresql.dashboard_tasks.updated_at"] == "" {
		t.Fatalf("missing PostgreSQL dashboard watermark: %+v", ownSnapshot.SourceWatermarks)
	}
	otherSnapshot := snapshotReader.ReadDashboardSnapshot(ctx, otherTenantID, snapshotStart, snapshotEnd)
	otherBacklog := dashboardIntegrationMetric(t, otherSnapshot.Data, "queue_backlog")
	if otherBacklog.Value == nil || *otherBacklog.Value != 0 {
		t.Fatalf("cross-tenant snapshot leaked backlog=%+v", otherBacklog)
	}

	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE FUNCTION codex_fail_dashboard_audit() RETURNS trigger AS $$
		BEGIN IF NEW.tenant_id = '`+tenantID+`' AND NEW.object_type = 'dashboard_task' THEN RAISE EXCEPTION 'forced dashboard audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql;
		CREATE TRIGGER codex_fail_dashboard_audit_once BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION codex_fail_dashboard_audit()`); err != nil {
		t.Fatal(err)
	}
	failed := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite},
		"dashboard-task-integration-key-0003", DashboardTaskCreateRequest{
			Target: "rollback-target", Priority: "high", SnapshotID: "dashboard-snapshot-integration-2", Reason: "prove audit boundary rollback",
		})
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("forced rollback status=%d body=%s", failed.Code, failed.Body.String())
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER codex_fail_dashboard_audit_once ON audit_logs; DROP FUNCTION codex_fail_dashboard_audit()`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM dashboard_tasks WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_history WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_requests WHERE tenant_id=$1)`, tenantID).
		Scan(&tasks, &history, &outbox, &requests); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || history != 1 || outbox != 1 || requests != 1 {
		t.Fatalf("failed command leaked rows tasks=%d history=%d outbox=%d requests=%d", tasks, history, outbox, requests)
	}

	executor := &dashboardTaskIntegrationExecutor{receipt: DashboardTaskExecutionReceipt{
		Status: "completed", Provider: "integration-ticketing", ProviderReceiptID: "ticket-receipt-1",
		EffectState: "confirmed", EffectIDs: []string{"ticket-42"},
		Result: map[string]interface{}{"ticket_id": "ticket-42"}, ExecutedAt: time.Now().UTC(),
	}}
	published := make([]dashboardTaskPublishedMessage, 0, 2)
	publish := func(_ context.Context, key string, value []byte, headers ...commonkafka.MessageHeader) error {
		published = append(published, dashboardTaskPublishedMessage{Key: key, Value: append([]byte(nil), value...), Headers: append([]commonkafka.MessageHeader(nil), headers...)})
		return nil
	}
	pipeline, err := NewDashboardTaskPipeline(db, executor, publish, dashboardTaskKafkaTopic, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-integration-outbox", 10); err != nil || drained != 1 {
		t.Fatalf("drain request outbox count=%d err=%v", drained, err)
	}
	if len(published) != 1 {
		t.Fatalf("expected one requested event, got %d", len(published))
	}
	requestedMessage := dashboardTaskReceivedMessage(published[0], 0, 1)
	if err := pipeline.HandleKafkaMessage(ctx, requestedMessage); err != nil {
		t.Fatalf("consume requested event: %v", err)
	}
	if err := pipeline.HandleKafkaMessage(ctx, requestedMessage); err != nil {
		t.Fatalf("idempotent requested replay: %v", err)
	}
	if drained, err := pipeline.DrainExecutions(ctx, "dashboard-integration-executor", 10); err != nil || drained != 1 {
		t.Fatalf("drain execution count=%d err=%v", drained, err)
	}
	if executor.calls != 1 || executor.last.IdempotencyKey != "dashboard-task:"+receipt.EventID {
		t.Fatalf("executor identity calls=%d command=%+v", executor.calls, executor.last)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-integration-outbox-result", 10); err != nil || drained != 1 {
		t.Fatalf("drain result outbox count=%d err=%v", drained, err)
	}
	if len(published) != 2 {
		t.Fatalf("expected requested and result events, got %d", len(published))
	}
	resultMessage := dashboardTaskReceivedMessage(published[1], 0, 2)
	if err := pipeline.HandleKafkaMessage(ctx, resultMessage); err != nil {
		t.Fatalf("consume result event: %v", err)
	}
	terminal, err := handler.getTask(ctx, tenantID, receipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Status != "completed" || terminal.Revision != 3 || terminal.Result["provider"] != "integration-ticketing" {
		t.Fatalf("unexpected terminal task: %+v", terminal)
	}
	var attempts, executionReceipts, inbox, terminalHistory, resultOutbox int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM dashboard_task_execution_attempts WHERE tenant_id=$1 AND status='completed'),
		(SELECT count(*) FROM dashboard_task_execution_receipts WHERE tenant_id=$1 AND status='completed'),
		(SELECT count(*) FROM dashboard_task_event_inbox WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_history WHERE tenant_id=$1),
		(SELECT count(*) FROM dashboard_task_outbox WHERE tenant_id=$1 AND status='published')`, tenantID).
		Scan(&attempts, &executionReceipts, &inbox, &terminalHistory, &resultOutbox); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || executionReceipts != 1 || inbox != 2 || terminalHistory != 3 || resultOutbox != 2 {
		t.Fatalf("pipeline facts attempts=%d receipts=%d inbox=%d history=%d published=%d", attempts, executionReceipts, inbox, terminalHistory, resultOutbox)
	}

	unknownCreated := performDashboardTaskCreate(t, handler, tenantID, []string{authmodel.ScopeDashboardWrite},
		"dashboard-task-integration-key-0004", DashboardTaskCreateRequest{
			Target: "transport-unknown-target", Priority: "high", SnapshotID: "dashboard-snapshot-integration-3",
			Reason: "prove transport ambiguity never claims success or clean failure",
		})
	if unknownCreated.Code != http.StatusAccepted {
		t.Fatalf("transport-unknown task status=%d body=%s", unknownCreated.Code, unknownCreated.Body.String())
	}
	unknownReceipt := decodeDashboardTaskReceipt(t, unknownCreated)
	executor.err = errors.New("provider response lost after request write")
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-integration-outbox-unknown", 10); err != nil || drained != 1 {
		t.Fatalf("drain ambiguous request count=%d err=%v", drained, err)
	}
	if err := pipeline.HandleKafkaMessage(ctx, dashboardTaskReceivedMessage(published[2], 0, 3)); err != nil {
		t.Fatalf("consume ambiguous request: %v", err)
	}
	if drained, err := pipeline.DrainExecutions(ctx, "dashboard-integration-executor-unknown", 10); err != nil || drained != 1 {
		t.Fatalf("drain ambiguous execution count=%d err=%v", drained, err)
	}
	if drained, err := pipeline.DrainOutbox(ctx, "dashboard-integration-outbox-unknown-result", 10); err != nil || drained != 1 {
		t.Fatalf("drain ambiguous result count=%d err=%v", drained, err)
	}
	if err := pipeline.HandleKafkaMessage(ctx, dashboardTaskReceivedMessage(published[3], 0, 4)); err != nil {
		t.Fatalf("consume ambiguous result: %v", err)
	}
	unknownTerminal, err := handler.getTask(ctx, tenantID, unknownReceipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if unknownTerminal.Status != "partial" || unknownTerminal.ErrorCode != "EXECUTOR_TRANSPORT_UNKNOWN" ||
		unknownTerminal.Result["effect_state"] != "unknown" {
		t.Fatalf("transport ambiguity was not represented truthfully: %+v", unknownTerminal)
	}
}

type dashboardTaskIntegrationExecutor struct {
	receipt DashboardTaskExecutionReceipt
	err     error
	calls   int
	last    DashboardTaskExecutionRequest
}

func (executor *dashboardTaskIntegrationExecutor) ExecuteDashboardTask(_ context.Context, command DashboardTaskExecutionRequest) (DashboardTaskExecutionReceipt, error) {
	executor.calls++
	executor.last = command
	if executor.err != nil {
		return DashboardTaskExecutionReceipt{}, executor.err
	}
	return executor.receipt, nil
}

type dashboardTaskPublishedMessage struct {
	Key     string
	Value   []byte
	Headers []commonkafka.MessageHeader
}

func dashboardTaskReceivedMessage(published dashboardTaskPublishedMessage, partition int, offset int64) *commonkafka.ReceivedMessage {
	headers := make([]segmentkafka.Header, 0, len(published.Headers))
	for _, header := range published.Headers {
		headers = append(headers, segmentkafka.Header{Key: header.Key, Value: []byte(header.Value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: dashboardTaskKafkaTopic, Key: []byte(published.Key), Value: published.Value,
		Headers: headers, Partition: partition, Offset: offset,
	}}
}

func dashboardIntegrationMetric(t *testing.T, data DashboardSnapshotData, key string) DashboardSnapshotMetric {
	t.Helper()
	for _, metric := range data.Metrics {
		if metric.Key == key {
			return metric
		}
	}
	t.Fatalf("dashboard metric %s missing", key)
	return DashboardSnapshotMetric{}
}

func performDashboardTaskCreate(t *testing.T, handler *DashboardTaskHandler, tenantID string, permissions []string, idempotencyKey string, body DashboardTaskCreateRequest) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/tasks", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "dashboard-integration-operator")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-"+idempotencyKey)
	recorder := httptest.NewRecorder()
	handler.Create(recorder, request.WithContext(ctx))
	return recorder
}

func performDashboardTaskGet(handler *DashboardTaskHandler, tenantID, taskID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/tasks/"+taskID, nil)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "dashboard-integration-operator")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, []string{authmodel.ScopeDashboardWrite})
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-dashboard-get")
	request = mux.SetURLVars(request.WithContext(ctx), map[string]string{"task_id": taskID})
	recorder := httptest.NewRecorder()
	handler.Get(recorder, request)
	return recorder
}

func decodeDashboardTaskReceipt(t *testing.T, recorder *httptest.ResponseRecorder) DashboardTaskReceipt {
	t.Helper()
	var envelope struct {
		Data DashboardTaskReceipt `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode receipt: %v body=%s", err, recorder.Body.String())
	}
	return envelope.Data
}

func cleanupDashboardTaskIntegration(t *testing.T, db *sql.DB, tenantIDs ...string) {
	t.Helper()
	_, _ = db.Exec(`DROP TRIGGER IF EXISTS codex_fail_dashboard_audit_once ON audit_logs`)
	_, _ = db.Exec(`DROP FUNCTION IF EXISTS codex_fail_dashboard_audit()`)
	for _, tenantID := range tenantIDs {
		for _, statement := range []string{
			`DELETE FROM dashboard_task_requests WHERE tenant_id=$1`,
			`DELETE FROM dashboard_task_event_inbox WHERE tenant_id=$1`,
			`DELETE FROM dashboard_task_execution_receipts WHERE tenant_id=$1`,
			`DELETE FROM dashboard_task_execution_attempts WHERE tenant_id=$1`,
			`DELETE FROM dashboard_task_history WHERE tenant_id=$1`,
			`DELETE FROM dashboard_task_outbox WHERE tenant_id=$1`,
			`DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type='dashboard_task'`,
			`DELETE FROM dashboard_tasks WHERE tenant_id=$1`,
			`DELETE FROM tenants WHERE tenant_id=$1`,
		} {
			if _, err := db.Exec(statement, tenantID); err != nil {
				t.Errorf("cleanup dashboard task fixture %s: %v", tenantID, err)
			}
		}
	}
}

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type sentinelPlaybookExecutionProvider struct {
	executeCalls    int
	compensateCalls int
}

type capturedPlaybookExecutionEvent struct {
	key       string
	payload   []byte
	headers   []commonkafka.MessageHeader
	partition int
	offset    int64
}

func (provider *sentinelPlaybookExecutionProvider) Execute(_ context.Context, request PlaybookExecutionProviderRequest) (PlaybookExecutionProviderReceipt, error) {
	provider.executeCalls++
	return PlaybookExecutionProviderReceipt{Status: "succeeded", Steps: []PlaybookStepReceipt{{
		StepIndex: 0, ActionType: "quarantine", Provider: "sentinel-playbook",
		ProviderReceiptID: request.ExecutionID + ":execute:0", Status: "succeeded", ExternalEffect: true,
		Detail: map[string]interface{}{"effect_id": "quarantine-live-1"},
	}}}, nil
}

func (provider *sentinelPlaybookExecutionProvider) Compensate(_ context.Context, request PlaybookExecutionProviderRequest, prior PlaybookExecutionProviderReceipt) (PlaybookExecutionProviderReceipt, error) {
	provider.compensateCalls++
	if len(prior.Steps) != 1 || prior.Steps[0].ProviderReceiptID != request.ExecutionID+":execute:0" {
		return PlaybookExecutionProviderReceipt{}, sql.ErrNoRows
	}
	return PlaybookExecutionProviderReceipt{Status: "succeeded", Steps: []PlaybookStepReceipt{{
		StepIndex: 0, ActionType: "quarantine", Provider: "sentinel-playbook",
		ProviderReceiptID: request.ExecutionID + ":compensate:0", Status: "succeeded", ExternalEffect: true,
		Detail: map[string]interface{}{"reversed_effect_id": "quarantine-live-1"},
	}}}, nil
}

func TestPlaybookExecutionV2PostgresApprovalAndCancelIntegration(t *testing.T) {
	dsn := os.Getenv("PLAYBOOK_EXECUTION_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("PLAYBOOK_EXECUTION_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_playbook_v2_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	tenantID := "playbook-v2-integration-" + time.Now().UTC().Format("150405000000")
	cleanupPlaybookExecutionV2Integration(t, db, tenantID)
	defer cleanupPlaybookExecutionV2Integration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Playbook V2 Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	definition := `{
		"name":"isolate-critical-host","description":"integration definition","enabled":true,
		"trigger":{"alert_type":"c2","severity_min":"high","score_min":0.8},
		"actions":[{"type":"quarantine","parameters":{"target":"host"},"timeout":30000000000}],
		"cooldown":0,"max_runs":1,"run_count":0,
		"approval_policy":{"required":true,"minimum_role":"security-l2","two_person_rule":true},
		"rollback_policy":{"supported":true,"automatic":false}
	}`
	if _, err := db.Exec(`INSERT INTO alert_playbook_definitions
		(tenant_id,name,display_name,description,version,stage,enabled,risk_level,definition_payload,
		 created_by,submitted_by,approved_by)
		VALUES ($1,'isolate-critical-host','隔离高危主机','integration definition',3,'approved',true,'critical',$2::jsonb,
		 'author-a','author-a','definition-approver')`, tenantID, definition); err != nil {
		t.Fatal(err)
	}

	repo := NewAdvancedRepository(db, zap.NewNop())
	handler := NewAdvancedHandler(nil, nil, nil, nil, repo)
	handler.SetPlaybookExecutionV2FeatureFlag(true)
	missingAlert := playbookExecutionV2Request(http.MethodPost, "/playbooks/isolate-critical-host/execute",
		`{"expected_version":3,"reason":"拒绝缺少真实告警编号的执行请求","alert_context":{"source_ip":"198.51.100.9"}}`,
		tenantID, "execution-requester", "playbook-missing-alert-0001", authmodel.ScopePlaybookExecute)
	missingAlert = mux.SetURLVars(missingAlert, map[string]string{"name": "isolate-critical-host"})
	missingAlertRecorder := httptest.NewRecorder()
	handler.ExecutePlaybook(missingAlertRecorder, missingAlert)
	if missingAlertRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing alert status=%d body=%s", missingAlertRecorder.Code, missingAlertRecorder.Body.String())
	}
	execute := playbookExecutionV2Request(http.MethodPost, "/playbooks/isolate-critical-host/execute",
		`{"expected_version":3,"reason":"请求执行高危主机隔离剧本","alert_context":{"alert_id":"alert-live-1","source_ip":"198.51.100.10"}}`,
		tenantID, "execution-requester", "playbook-execution-request-0001", authmodel.ScopePlaybookExecute)
	execute = mux.SetURLVars(execute, map[string]string{"name": "isolate-critical-host"})
	executeRecorder := httptest.NewRecorder()
	handler.ExecutePlaybook(executeRecorder, execute)
	if executeRecorder.Code != http.StatusAccepted {
		t.Fatalf("execute status=%d body=%s", executeRecorder.Code, executeRecorder.Body.String())
	}
	var executionID string
	if err := db.QueryRow(`SELECT execution_id FROM alert_playbook_executions WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, "playbook-execution-request-0001").Scan(&executionID); err != nil {
		t.Fatal(err)
	}

	replay := playbookExecutionV2Request(http.MethodPost, "/playbooks/isolate-critical-host/execute",
		`{"expected_version":3,"reason":"请求执行高危主机隔离剧本","alert_context":{"alert_id":"alert-live-1","source_ip":"198.51.100.10"}}`,
		tenantID, "execution-requester", "playbook-execution-request-0001", authmodel.ScopePlaybookExecute)
	replay = mux.SetURLVars(replay, map[string]string{"name": "isolate-critical-host"})
	replayRecorder := httptest.NewRecorder()
	handler.ExecutePlaybook(replayRecorder, replay)
	if replayRecorder.Code != http.StatusAccepted {
		t.Fatalf("execute replay status=%d body=%s", replayRecorder.Code, replayRecorder.Body.String())
	}

	selfApproval := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+executionID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"申请人不得批准自己的执行请求"}`,
		tenantID, "execution-requester", "playbook-self-approval-0001", authmodel.ScopePlaybookApprove)
	selfApproval = mux.SetURLVars(selfApproval, map[string]string{"execution_id": executionID})
	selfRecorder := httptest.NewRecorder()
	handler.DecidePlaybookExecutionV2(selfRecorder, selfApproval)
	if selfRecorder.Code != http.StatusForbidden {
		t.Fatalf("self approval status=%d body=%s", selfRecorder.Code, selfRecorder.Body.String())
	}

	approval := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+executionID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"独立审批人确认执行高危隔离剧本"}`,
		tenantID, "execution-approver", "playbook-approval-request-0001", authmodel.ScopePlaybookApprove)
	approval = mux.SetURLVars(approval, map[string]string{"execution_id": executionID})
	approvalRecorder := httptest.NewRecorder()
	handler.DecidePlaybookExecutionV2(approvalRecorder, approval)
	if approvalRecorder.Code != http.StatusAccepted {
		t.Fatalf("approval status=%d body=%s", approvalRecorder.Code, approvalRecorder.Body.String())
	}
	approvalReplay := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+executionID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"独立审批人确认执行高危隔离剧本"}`,
		tenantID, "execution-approver", "playbook-approval-request-0001", authmodel.ScopePlaybookApprove)
	approvalReplay = mux.SetURLVars(approvalReplay, map[string]string{"execution_id": executionID})
	approvalReplayRecorder := httptest.NewRecorder()
	handler.DecidePlaybookExecutionV2(approvalReplayRecorder, approvalReplay)
	if approvalReplayRecorder.Code != http.StatusAccepted {
		t.Fatalf("approval replay status=%d body=%s", approvalReplayRecorder.Code, approvalReplayRecorder.Body.String())
	}

	crossTenant := playbookExecutionV2Request(http.MethodGet, "/playbooks/executions/"+executionID, "",
		"another-tenant", "reader-a", "unused-idempotency-key", authmodel.ScopePlaybookRead)
	crossTenant = mux.SetURLVars(crossTenant, map[string]string{"execution_id": executionID})
	crossTenantRecorder := httptest.NewRecorder()
	handler.GetPlaybookExecutionV2(crossTenantRecorder, crossTenant)
	if crossTenantRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross tenant read status=%d body=%s", crossTenantRecorder.Code, crossTenantRecorder.Body.String())
	}

	cancel := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+executionID+"/cancel",
		`{"expected_revision":2,"reason":"执行器尚未配置，申请人取消本次请求"}`,
		tenantID, "execution-requester", "playbook-cancel-request-0001", authmodel.ScopePlaybookExecute)
	cancel = mux.SetURLVars(cancel, map[string]string{"execution_id": executionID})
	cancelRecorder := httptest.NewRecorder()
	handler.CancelPlaybookExecutionV2(cancelRecorder, cancel)
	if cancelRecorder.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", cancelRecorder.Code, cancelRecorder.Body.String())
	}

	var status, approvalStatus, executorStatus string
	var revision, executions, approvals, controls, receipts, outbox, audits int
	if err := db.QueryRow(`SELECT
		(SELECT status FROM alert_playbook_executions WHERE tenant_id=$1 AND execution_id=$2),
		(SELECT approval_status FROM alert_playbook_executions WHERE tenant_id=$1 AND execution_id=$2),
		(SELECT executor_status FROM alert_playbook_executions WHERE tenant_id=$1 AND execution_id=$2),
		(SELECT workflow_revision FROM alert_playbook_executions WHERE tenant_id=$1 AND execution_id=$2),
		(SELECT count(*) FROM alert_playbook_executions WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_playbook_execution_approvals WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_playbook_execution_controls WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_playbook_step_receipts WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_playbook_execution_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_id=$2)`, tenantID, executionID).Scan(
		&status, &approvalStatus, &executorStatus, &revision, &executions, &approvals,
		&controls, &receipts, &outbox, &audits); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" || approvalStatus != "cancelled" || executorStatus != "cancelled" ||
		revision != 3 || executions != 1 || approvals != 1 || controls != 1 || receipts != 0 || outbox != 3 || audits != 3 {
		t.Fatalf("state=%s/%s/%s revision=%d executions=%d approvals=%d controls=%d receipts=%d outbox=%d audits=%d",
			status, approvalStatus, executorStatus, revision, executions, approvals, controls, receipts, outbox, audits)
	}

	provider := &sentinelPlaybookExecutionProvider{}
	handler.SetPlaybookExecutionProvider(provider)
	providerExecute := playbookExecutionV2Request(http.MethodPost, "/playbooks/isolate-critical-host/execute",
		`{"expected_version":3,"reason":"请求执行并验证provider回执闭环","alert_context":{"alert_id":"alert-live-2","source_ip":"203.0.113.20"}}`,
		tenantID, "provider-requester", "playbook-provider-request-0001", authmodel.ScopePlaybookExecute)
	providerExecute = mux.SetURLVars(providerExecute, map[string]string{"name": "isolate-critical-host"})
	providerExecuteRecorder := httptest.NewRecorder()
	handler.ExecutePlaybook(providerExecuteRecorder, providerExecute)
	if providerExecuteRecorder.Code != http.StatusAccepted {
		t.Fatalf("provider execute status=%d body=%s", providerExecuteRecorder.Code, providerExecuteRecorder.Body.String())
	}
	var providerExecutionID string
	if err := db.QueryRow(`SELECT execution_id FROM alert_playbook_executions WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, "playbook-provider-request-0001").Scan(&providerExecutionID); err != nil {
		t.Fatal(err)
	}
	providerApproval := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+providerExecutionID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"独立审批provider真实执行测试"}`,
		tenantID, "provider-approver", "playbook-provider-approval-001", authmodel.ScopePlaybookApprove)
	providerApproval = mux.SetURLVars(providerApproval, map[string]string{"execution_id": providerExecutionID})
	providerApprovalRecorder := httptest.NewRecorder()
	handler.DecidePlaybookExecutionV2(providerApprovalRecorder, providerApproval)
	if providerApprovalRecorder.Code != http.StatusAccepted {
		t.Fatalf("provider approval status=%d body=%s", providerApprovalRecorder.Code, providerApprovalRecorder.Body.String())
	}
	if err := handler.processNextPlaybookExecution(context.Background()); err != nil {
		t.Fatal(err)
	}
	completed, err := loadPlaybookExecution(context.Background(), db, tenantID, providerExecutionID)
	if err != nil || completed.Status != "completed" || completed.ExecutorStatus != "succeeded" || completed.WorkflowRevision != 3 {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}
	if provider.executeCalls != 1 {
		t.Fatalf("provider execute calls=%d", provider.executeCalls)
	}
	selfCompensation := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+providerExecutionID+"/compensate",
		`{"expected_revision":3,"reason":"原申请人不得批准补偿真实外部效果"}`,
		tenantID, "provider-requester", "playbook-provider-self-comp-01", authmodel.ScopePlaybookApprove)
	selfCompensation = mux.SetURLVars(selfCompensation, map[string]string{"execution_id": providerExecutionID})
	selfCompensationRecorder := httptest.NewRecorder()
	handler.CompensatePlaybookExecutionV2(selfCompensationRecorder, selfCompensation)
	if selfCompensationRecorder.Code != http.StatusForbidden {
		t.Fatalf("self compensation status=%d body=%s", selfCompensationRecorder.Code, selfCompensationRecorder.Body.String())
	}
	compensation := playbookExecutionV2Request(http.MethodPost, "/playbooks/executions/"+providerExecutionID+"/compensate",
		`{"expected_revision":3,"reason":"独立审批人确认撤销provider外部效果"}`,
		tenantID, "provider-compensator", "playbook-provider-compensate-01", authmodel.ScopePlaybookApprove)
	compensation = mux.SetURLVars(compensation, map[string]string{"execution_id": providerExecutionID})
	compensationRecorder := httptest.NewRecorder()
	handler.CompensatePlaybookExecutionV2(compensationRecorder, compensation)
	if compensationRecorder.Code != http.StatusAccepted {
		t.Fatalf("compensation status=%d body=%s", compensationRecorder.Code, compensationRecorder.Body.String())
	}
	if err := handler.processNextPlaybookExecution(context.Background()); err != nil {
		t.Fatal(err)
	}
	compensated, err := loadPlaybookExecution(context.Background(), db, tenantID, providerExecutionID)
	if err != nil || compensated.Status != "compensated" || compensated.ExecutorStatus != "compensated" || compensated.WorkflowRevision != 5 {
		t.Fatalf("compensated=%+v err=%v", compensated, err)
	}
	if provider.compensateCalls != 1 {
		t.Fatalf("provider compensate calls=%d", provider.compensateCalls)
	}
	var providerReceipts, providerOutbox int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM alert_playbook_step_receipts WHERE tenant_id=$1 AND execution_id=$2),
		(SELECT count(*) FROM alert_playbook_execution_outbox WHERE tenant_id=$1 AND execution_id=$2)`,
		tenantID, providerExecutionID).Scan(&providerReceipts, &providerOutbox); err != nil {
		t.Fatal(err)
	}
	if providerReceipts != 2 || providerOutbox != 5 {
		t.Fatalf("provider receipts=%d outbox=%d", providerReceipts, providerOutbox)
	}

	// Exercise the durable delivery and projection boundary against real
	// PostgreSQL. The producer callback is deliberately only an acknowledged
	// broker boundary sentinel, so this remains G2 PostgreSQL evidence and does
	// not claim a real Kafka deployment.
	publishedEvents := make([]capturedPlaybookExecutionEvent, 0, 8)
	broker := strings.TrimSpace(os.Getenv("PLAYBOOK_EXECUTION_EPHEMERAL_KAFKA_BROKER"))
	if broker == "" {
		handler.playbookExecutionTopic = PlaybookExecutionEventTopic
		handler.playbookExecutionPublish = func(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error {
			publishedEvents = append(publishedEvents, capturedPlaybookExecutionEvent{
				key: key, payload: append([]byte(nil), payload...), headers: append([]commonkafka.MessageHeader(nil), headers...),
			})
			return nil
		}
	} else {
		requireEphemeralKafkaBroker(t, broker)
		producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
			Brokers: []string{broker}, Topic: PlaybookExecutionEventTopic, BatchSize: 1,
			RequiredAcks: "all", Compression: "none", Async: false, IdempotentKey: "tenant+execution",
		}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		defer producer.Close()
		handler.SetPlaybookExecutionEventProducer(producer, PlaybookExecutionEventTopic)
	}
	published, err := handler.drainPlaybookExecutionOutbox(context.Background(), "playbook-integration-publisher", 20)
	if err != nil || published != 8 {
		t.Fatalf("outbox published=%d captured=%d err=%v", published, len(publishedEvents), err)
	}
	if broker != "" {
		reader := segmentkafka.NewReader(segmentkafka.ReaderConfig{
			Brokers: []string{broker}, GroupID: "playbook-event-integration-" + tenantID,
			Topic: PlaybookExecutionEventTopic, StartOffset: segmentkafka.FirstOffset,
			MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 500 * time.Millisecond,
		})
		defer reader.Close()
		readContext, cancelRead := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelRead()
		for len(publishedEvents) < 8 {
			message, readErr := reader.ReadMessage(readContext)
			if readErr != nil {
				t.Fatalf("read playbook event %d: %v", len(publishedEvents), readErr)
			}
			var identity struct {
				TenantID string `json:"tenant_id"`
			}
			if json.Unmarshal(message.Value, &identity) != nil || identity.TenantID != tenantID {
				continue
			}
			headers := make([]commonkafka.MessageHeader, 0, len(message.Headers))
			for _, header := range message.Headers {
				headers = append(headers, commonkafka.MessageHeader{Key: header.Key, Value: string(header.Value)})
			}
			publishedEvents = append(publishedEvents, capturedPlaybookExecutionEvent{
				key: string(message.Key), payload: append([]byte(nil), message.Value...), headers: headers,
				partition: message.Partition, offset: message.Offset,
			})
		}
	}
	if len(publishedEvents) != 8 {
		t.Fatalf("captured events=%d", len(publishedEvents))
	}
	for offset, event := range publishedEvents {
		var envelope struct {
			EventID          string `json:"event_id"`
			EventType        string `json:"event_type"`
			TenantID         string `json:"tenant_id"`
			ExecutionID      string `json:"execution_id"`
			PlaybookName     string `json:"playbook_name"`
			PlaybookVersion  int    `json:"playbook_version"`
			AlertID          string `json:"alert_id"`
			Status           string `json:"status"`
			ApprovalStatus   string `json:"approval_status"`
			ExecutorStatus   string `json:"executor_status"`
			SchemaVersion    int    `json:"schema_version"`
			AggregateVersion int64  `json:"aggregate_version"`
			PartitionKey     string `json:"partition_key"`
			TraceID          string `json:"trace_id"`
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(event.payload, &envelope); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(event.payload, &payload); err != nil {
			t.Fatal(err)
		}
		if event.key != envelope.PartitionKey {
			t.Fatalf("event key=%q partition_key=%q", event.key, envelope.PartitionKey)
		}
		if len(event.headers) == 0 {
			t.Fatal("published event is missing contract headers")
		}
		headerValues := make(map[string]string, len(event.headers))
		for _, header := range event.headers {
			headerValues[header.Key] = header.Value
		}
		for key, expected := range map[string]string{
			"event_id": envelope.EventID, "event_type": envelope.EventType,
			"tenant_id": envelope.TenantID, "aggregate_type": "playbook_execution",
			"aggregate_id":      envelope.ExecutionID,
			"aggregate_version": strconv.FormatInt(envelope.AggregateVersion, 10),
			"schema_version":    "2", "trace_id": envelope.TraceID,
			"target_topic": PlaybookExecutionEventTopic,
		} {
			if headerValues[key] != expected {
				t.Fatalf("event %s header %s=%q want %q", envelope.EventID, key, headerValues[key], expected)
			}
		}
		input := PlaybookExecutionEventProjectionInput{
			EventID: envelope.EventID, TenantID: envelope.TenantID, ExecutionID: envelope.ExecutionID,
			PlaybookName: envelope.PlaybookName, PlaybookVersion: envelope.PlaybookVersion,
			AlertID: envelope.AlertID, EventType: envelope.EventType, Status: envelope.Status,
			ApprovalStatus: envelope.ApprovalStatus, ExecutorStatus: envelope.ExecutorStatus,
			SchemaVersion: envelope.SchemaVersion, AggregateVersion: envelope.AggregateVersion,
			PartitionKey: envelope.PartitionKey, TraceID: envelope.TraceID, Payload: payload,
			KafkaTopic: PlaybookExecutionEventTopic, KafkaPartition: event.partition, KafkaOffset: event.offset,
		}
		if broker == "" {
			input.KafkaOffset = int64(offset)
		}
		if err := handler.ApplyPlaybookExecutionEventProjection(context.Background(), input); err != nil {
			t.Fatalf("project offset=%d event=%s: %v", offset, envelope.EventID, err)
		}
		if offset == len(publishedEvents)-1 {
			if err := handler.ApplyPlaybookExecutionEventProjection(context.Background(), input); err != nil {
				t.Fatalf("exact replay event=%s: %v", envelope.EventID, err)
			}
		}
	}
	var publishedRows, eventRows, stateRows int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM alert_playbook_execution_outbox WHERE tenant_id=$1 AND published=true AND status='published'),
		(SELECT count(*) FROM alert_playbook_execution_event_projection WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_playbook_execution_state_projection WHERE tenant_id=$1)`, tenantID).Scan(
		&publishedRows, &eventRows, &stateRows); err != nil {
		t.Fatal(err)
	}
	var cancelledProjectionStatus, compensatedProjectionStatus string
	var cancelledProjectionRevision, compensatedProjectionRevision int64
	if err := db.QueryRow(`SELECT status,aggregate_version FROM alert_playbook_execution_state_projection
		WHERE tenant_id=$1 AND execution_id=$2`, tenantID, executionID).Scan(
		&cancelledProjectionStatus, &cancelledProjectionRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status,aggregate_version FROM alert_playbook_execution_state_projection
		WHERE tenant_id=$1 AND execution_id=$2`, tenantID, providerExecutionID).Scan(
		&compensatedProjectionStatus, &compensatedProjectionRevision); err != nil {
		t.Fatal(err)
	}
	if publishedRows != 8 || eventRows != 8 || stateRows != 2 ||
		cancelledProjectionStatus != "cancelled" || cancelledProjectionRevision != 3 ||
		compensatedProjectionStatus != "compensated" || compensatedProjectionRevision != 5 {
		t.Fatalf("published=%d events=%d states=%d cancelled=%s/%d compensated=%s/%d",
			publishedRows, eventRows, stateRows, cancelledProjectionStatus, cancelledProjectionRevision,
			compensatedProjectionStatus, compensatedProjectionRevision)
	}
	runLimit := playbookExecutionV2Request(http.MethodPost, "/playbooks/isolate-critical-host/execute",
		`{"expected_version":3,"reason":"验证同一剧本版本执行次数上限","alert_context":{"alert_id":"alert-live-3"}}`,
		tenantID, "limit-requester", "playbook-run-limit-request-01", authmodel.ScopePlaybookExecute)
	runLimit = mux.SetURLVars(runLimit, map[string]string{"name": "isolate-critical-host"})
	runLimitRecorder := httptest.NewRecorder()
	handler.ExecutePlaybook(runLimitRecorder, runLimit)
	if runLimitRecorder.Code != http.StatusConflict {
		t.Fatalf("run limit status=%d body=%s", runLimitRecorder.Code, runLimitRecorder.Body.String())
	}
	var runLimitEnvelope httpx.Response
	if err := json.Unmarshal(runLimitRecorder.Body.Bytes(), &runLimitEnvelope); err != nil {
		t.Fatalf("decode run limit response: %v body=%s", err, runLimitRecorder.Body.String())
	}
	if runLimitEnvelope.Error == nil || runLimitEnvelope.Error.Code != "PLAYBOOK_RUN_LIMIT_REACHED" {
		t.Fatalf("unexpected run limit error: %+v body=%s", runLimitEnvelope.Error, runLimitRecorder.Body.String())
	}
	var executionCount, rejectedAuditCount int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM alert_playbook_executions WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_id='playbook-run-limit-request-01')`, tenantID).Scan(
		&executionCount, &rejectedAuditCount); err != nil {
		t.Fatal(err)
	}
	if executionCount != 2 || rejectedAuditCount != 0 {
		t.Fatalf("rejected run limit created side effects: executions=%d rejected_audits=%d", executionCount, rejectedAuditCount)
	}
}

func playbookExecutionV2Request(method, path, body, tenantID, actor, idempotencyKey string, permissions ...string) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, actor)
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-playbook-v2-integration")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	return request.WithContext(ctx)
}

func cleanupPlaybookExecutionV2Integration(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	for _, statement := range []string{
		`DELETE FROM alert_playbook_execution_state_projection WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_execution_event_projection WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_step_receipts WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_execution_approvals WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_execution_controls WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_execution_outbox WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_executions WHERE tenant_id=$1`,
		`DELETE FROM alert_playbook_definitions WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Fatalf("cleanup %q: %v", statement, err)
		}
	}
}

package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// TestAlertResponseWorkflowPostgresIntegration is intentionally guarded by a
// sentinel table in an ephemeral PostgreSQL instance. It must never run merely
// because a DSN happens to be present in a developer or production shell.
func TestAlertResponseWorkflowPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_RESPONSE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ALERT_RESPONSE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var guard string
	if err := db.QueryRow(`SELECT guard_value FROM remediation_ephemeral_guard WHERE guard_value='alert-response-integration-v1'`).Scan(&guard); err != nil {
		t.Fatalf("refusing to run without ephemeral database guard: %v", err)
	}

	tenantID := "integration-alert-response-" + time.Now().UTC().Format("150405000000")
	alertID := "AL-INTEGRATION-1"
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))

	createBody := `{"action_id":"alert-response-block-ip","action":"block_ip","target":"198.51.100.10","reason":"confirmed malicious source","dry_run":false,"expected_revision":0}`
	create := runAlertResponseIntegrationRequest(t, handler.CreateAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions", createBody,
		tenantID, alertID, "", "operator-a", "integration-create-key-0001",
		[]string{authmodel.ScopeAlertWrite})
	if create.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	jobID := responseJobIDFromEnvelope(t, create)

	replay := runAlertResponseIntegrationRequest(t, handler.CreateAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions", createBody,
		tenantID, alertID, "", "operator-a", "integration-create-key-0001",
		[]string{authmodel.ScopeAlertWrite})
	if replay.Code != http.StatusAccepted || responseJobIDFromEnvelope(t, replay) != jobID ||
		!responseBooleanFromEnvelope(t, replay, "idempotent_reuse") {
		t.Fatalf("create replay was not idempotent: status=%d body=%s", replay.Code, replay.Body.String())
	}

	var status, approvalStatus string
	var revision int64
	if err := db.QueryRow(`SELECT status,approval_status,revision FROM alert_response_actions
		WHERE tenant_id=$1 AND alert_id=$2 AND job_id=$3`,
		tenantID, alertID, jobID,
	).Scan(&status, &approvalStatus, &revision); err != nil {
		t.Fatal(err)
	}
	if status != "pending_approval" || approvalStatus != "pending" || revision != 1 {
		t.Fatalf("unexpected initial state: status=%s approval=%s revision=%d", status, approvalStatus, revision)
	}
	var outboxCount int
	if err := db.QueryRow(`SELECT count(*) FROM alert_response_outbox WHERE tenant_id=$1 AND job_id=$2`, tenantID, jobID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 {
		t.Fatalf("unapproved real action created %d outbox rows", outboxCount)
	}

	selfApproval := runAlertResponseIntegrationRequest(t, handler.DecideAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"independent approval"}`,
		tenantID, alertID, jobID, "operator-a", "integration-approval-self",
		[]string{authmodel.ScopePlaybookApprove})
	if selfApproval.Code != http.StatusForbidden {
		t.Fatalf("self approval status=%d body=%s", selfApproval.Code, selfApproval.Body.String())
	}

	approvalBody := `{"decision":"approve","expected_revision":1,"reason":"independent approval"}`
	approval := runAlertResponseIntegrationRequest(t, handler.DecideAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/approval",
		approvalBody, tenantID, alertID, jobID, "approver-b", "integration-approval-key-1",
		[]string{authmodel.ScopePlaybookApprove})
	if approval.Code != http.StatusAccepted {
		t.Fatalf("approval status=%d body=%s", approval.Code, approval.Body.String())
	}
	approvalReplay := runAlertResponseIntegrationRequest(t, handler.DecideAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/approval",
		approvalBody, tenantID, alertID, jobID, "approver-b", "integration-approval-key-1",
		[]string{authmodel.ScopePlaybookApprove})
	if approvalReplay.Code != http.StatusAccepted ||
		!responseBooleanFromEnvelope(t, approvalReplay, "idempotent_reuse") {
		t.Fatalf("approval replay was not idempotent: status=%d body=%s", approvalReplay.Code, approvalReplay.Body.String())
	}

	var aggregateVersion int64
	var cancelledAt sql.NullTime
	if err := db.QueryRow(`SELECT a.status,a.approval_status,a.revision,o.aggregate_version,o.cancelled_at
		FROM alert_response_actions a JOIN alert_response_outbox o ON o.job_id=a.job_id
		WHERE a.tenant_id=$1 AND a.alert_id=$2 AND a.job_id=$3`,
		tenantID, alertID, jobID,
	).Scan(&status, &approvalStatus, &revision, &aggregateVersion, &cancelledAt); err != nil {
		t.Fatal(err)
	}
	if status != "approved_awaiting_executor" || approvalStatus != "approved" ||
		revision != 2 || aggregateVersion != 2 || cancelledAt.Valid {
		t.Fatalf("unexpected approved state: status=%s approval=%s revision=%d aggregate=%d cancelled=%t",
			status, approvalStatus, revision, aggregateVersion, cancelledAt.Valid)
	}

	cancelBody := `{"expected_revision":2,"reason":"cancel before delivery"}`
	cancel := runAlertResponseIntegrationRequest(t, handler.CancelAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/cancel",
		cancelBody, tenantID, alertID, jobID, "operator-a", "integration-cancel-key-001",
		[]string{authmodel.ScopeAlertWrite})
	if cancel.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
	cancelReplay := runAlertResponseIntegrationRequest(t, handler.CancelAlertResponseAction,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/cancel",
		cancelBody, tenantID, alertID, jobID, "operator-a", "integration-cancel-key-001",
		[]string{authmodel.ScopeAlertWrite})
	if cancelReplay.Code != http.StatusAccepted ||
		!responseBooleanFromEnvelope(t, cancelReplay, "idempotent_reuse") {
		t.Fatalf("cancel replay was not idempotent: status=%d body=%s", cancelReplay.Code, cancelReplay.Body.String())
	}
	if err := db.QueryRow(`SELECT a.status,a.revision,o.cancelled_at
		FROM alert_response_actions a JOIN alert_response_outbox o ON o.job_id=a.job_id
		WHERE a.tenant_id=$1 AND a.job_id=$2`,
		tenantID, jobID,
	).Scan(&status, &revision, &cancelledAt); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" || revision != 3 || !cancelledAt.Valid {
		t.Fatalf("unexpected cancelled state: status=%s revision=%d outbox_cancelled=%t", status, revision, cancelledAt.Valid)
	}

	compensation := runAlertResponseIntegrationRequest(t, handler.RequestAlertResponseCompensation,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/compensations",
		`{"expected_revision":3,"reason":"restore prior network access"}`,
		tenantID, alertID, jobID, "approver-b", "integration-compensation-1",
		[]string{authmodel.ScopePlaybookApprove})
	if compensation.Code != http.StatusConflict {
		t.Fatalf("compensation without receipt status=%d body=%s", compensation.Code, compensation.Body.String())
	}

	var approvals, controls, audits int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM alert_response_approvals WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM alert_response_control_requests WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_id IN ($2,$3))`,
		tenantID, jobID, alertID,
	).Scan(&approvals, &controls, &audits); err != nil {
		t.Fatal(err)
	}
	if approvals != 1 || controls != 1 || audits != 3 {
		t.Fatalf("unexpected transactional evidence counts: approvals=%d controls=%d audits=%d", approvals, controls, audits)
	}
}

func TestAlertResponseCompensationQueuePostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_RESPONSE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ALERT_RESPONSE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var guard string
	if err := db.QueryRow(`SELECT guard_value FROM remediation_ephemeral_guard WHERE guard_value='alert-response-integration-v1'`).Scan(&guard); err != nil {
		t.Fatalf("refusing to run without ephemeral database guard: %v", err)
	}

	suffix := time.Now().UTC().Format("150405000000")
	tenantID := "integration-alert-compensation-" + suffix
	alertID := "AL-COMPENSATION-1"
	jobID := "alert-action-compensation-" + suffix
	eventID := "55555555-5555-4555-8555-" + suffix
	traceID := "trace-alert-compensation-" + suffix
	if _, err := db.Exec(`INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,
		 status,approval_status,revision,trace_id,idempotency_key,expected_revision,
		 detail,requested_by,approved_by,approved_at,result,error)
		VALUES ($1,$2::uuid,$3,$4,'alert-response-block-ip','block_ip','198.51.100.40',
		 'confirmed malicious source',false,'completed','approved',3,$5,$6,0,
		 '{}'::jsonb,'operator-a','approver-b',now(),'{}'::jsonb,'')`,
		jobID, eventID, tenantID, alertID, traceID, "compensation-source-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_execution_receipts
		(event_id,job_id,tenant_id,alert_id,action_id,state,simulated,external_effect,
		 aggregate_version,result,error,kafka_partition,kafka_offset,provider,
		 provider_receipt_id,effect_state,effect_ids,trace_id,receipt_sha256,
		 authority_lookup,executed_at)
		VALUES ($1::uuid,$2,$3,$4,'alert-response-block-ip','completed',false,true,2,
		 '{}'::jsonb,'',7,$5,'ephemeral-firewall',$6,'confirmed',$7::jsonb,$8,
		 repeat('a',64),'{}'::jsonb,now())`,
		eventID, jobID, tenantID, alertID, time.Now().UnixNano(),
		"execution-receipt-"+suffix, `["rule-`+suffix+`"]`, traceID,
	); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetResponseCompensationEnabled(true)
	handler.SetResponseCompensationMaxAttempts(5)
	body := `{"expected_revision":3,"reason":"restore approved network access"}`
	request := runAlertResponseIntegrationRequest(t, handler.RequestAlertResponseCompensation,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/compensations",
		body, tenantID, alertID, jobID, "security-approver-c", "integration-compensation-key-0001",
		[]string{authmodel.ScopePlaybookApprove})
	if request.Code != http.StatusAccepted {
		t.Fatalf("compensation queue status=%d body=%s", request.Code, request.Body.String())
	}
	replay := runAlertResponseIntegrationRequest(t, handler.RequestAlertResponseCompensation,
		http.MethodPost, "/api/v1/alerts/"+alertID+"/response-actions/"+jobID+"/compensations",
		body, tenantID, alertID, jobID, "security-approver-c", "integration-compensation-key-0001",
		[]string{authmodel.ScopePlaybookApprove})
	if replay.Code != http.StatusAccepted || !responseBooleanFromEnvelope(t, replay, "idempotent_reuse") {
		t.Fatalf("compensation queue replay failed: status=%d body=%s", replay.Code, replay.Body.String())
	}
	var status, attemptStatus, providerKey string
	var revision int64
	var controls, attempts, audits, maxAttempts int
	if err := db.QueryRow(`SELECT a.status,a.revision,c.status,c.provider_idempotency_key,
		(SELECT count(*) FROM alert_response_control_requests WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM alert_response_compensation_attempts WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_id=$2 AND action='ALERT_RESPONSE_COMPENSATION_QUEUED'),
		c.max_attempts
		FROM alert_response_actions a
		JOIN alert_response_compensation_attempts c ON c.job_id=a.job_id
		WHERE a.tenant_id=$1 AND a.job_id=$2`, tenantID, jobID).Scan(
		&status, &revision, &attemptStatus, &providerKey, &controls, &attempts, &audits, &maxAttempts,
	); err != nil {
		t.Fatal(err)
	}
	if status != "compensation_queued" || revision != 4 || attemptStatus != "pending" ||
		!strings.HasPrefix(providerKey, "alert-response-compensation:") || controls != 1 ||
		attempts != 1 || audits != 1 || maxAttempts != 5 {
		t.Fatalf("queued compensation facts diverged: status=%s revision=%d attempt=%s key=%s controls=%d attempts=%d audits=%d max=%d",
			status, revision, attemptStatus, providerKey, controls, attempts, audits, maxAttempts)
	}
}

func runAlertResponseIntegrationRequest(
	t *testing.T,
	handler func(http.ResponseWriter, *http.Request),
	method, path, body, tenantID, alertID, jobID, actor, idempotencyKey string,
	permissions []string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := responseWorkflowRequest(method, path, body, permissions, actor, idempotencyKey)
	request.Header.Set("X-Tenant-ID", tenantID)
	request = muxWithAlertResponseVars(request, alertID, jobID)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func muxWithAlertResponseVars(request *http.Request, alertID, jobID string) *http.Request {
	return mux.SetURLVars(request, map[string]string{"id": alertID, "job_id": jobID})
}

func responseJobIDFromEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	jobID, _ := envelope.Data["job_id"].(string)
	if jobID == "" {
		t.Fatalf("response has no job_id: %s", recorder.Body.String())
	}
	return jobID
}

func responseBooleanFromEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, key string) bool {
	t.Helper()
	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	value, _ := envelope.Data[key].(bool)
	return value
}

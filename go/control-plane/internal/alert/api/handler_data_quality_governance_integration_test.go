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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

func TestDataQualityGovernanceHTTPEphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("DATA_QUALITY_GOVERNANCE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("DATA_QUALITY_GOVERNANCE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_data_quality_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}
	tenantID := "dq-http-" + uuid.NewString()
	otherTenantID := "dq-http-other-" + uuid.NewString()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'DQ HTTP'),($2,'DQ HTTP Other')`, tenantID, otherTenantID); err != nil {
		t.Fatal(err)
	}
	defer cleanupDataQualityGovernanceHTTPIntegration(t, db, tenantID, otherTenantID)
	monitor := dataquality.NewMonitor(nil, dataquality.MonitorConfig{}, zap.NewNop())
	monitor.SetControlDB(db)
	handler := NewAdvancedHandler(nil, nil, nil, monitor, nil)
	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()
	handler.RegisterAPIRoutes(api)
	var served http.Handler = httpx.RequestID()(router)

	datasetBody := `{"display_name":"Raw flows","owner":"data-platform","schema_version":1,"signal_contract_version":"data-quality-dataset-signals-v1","business_keys":["event_id"],"allowed_lateness_seconds":60,"retention_seconds":86400,"upstreams":["flow.events.v1"],"downstreams":["traffic.flows_raw"],"slo_target":0.999,"status":"active","expected_revision":0,"action_id":"dataset-create","reason":"register governed flow dataset"}`
	denied := performDataQualityGovernanceRequest(served, http.MethodPut, "/api/v1/data-quality/datasets/flows_raw", datasetBody, tenantID, "reader-a", "dq-http-dataset-denied-1", authmodel.ScopeDataQualityRead)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("write scope status=%d body=%s", denied.Code, denied.Body.String())
	}
	created := performDataQualityGovernanceRequest(served, http.MethodPut, "/api/v1/data-quality/datasets/flows_raw", datasetBody, tenantID, "requester-a", "dq-http-dataset-create-1", authmodel.ScopeDataQualityWrite)
	if created.Code != http.StatusOK {
		t.Fatalf("dataset create status=%d body=%s", created.Code, created.Body.String())
	}
	if governanceResponseField(t, created, "revision") != float64(1) {
		t.Fatalf("dataset revision response=%s", created.Body.String())
	}
	replay := performDataQualityGovernanceRequest(served, http.MethodPut, "/api/v1/data-quality/datasets/flows_raw", datasetBody, tenantID, "requester-a", "dq-http-dataset-create-1", authmodel.ScopeDataQualityWrite)
	if replay.Code != http.StatusOK || governanceResponseField(t, replay, "replayed") != true {
		t.Fatalf("dataset replay status=%d body=%s", replay.Code, replay.Body.String())
	}

	ruleBody := `{"dataset_id":"flows_raw","rule_key":"flow-event-id-present","dimension":"completeness","field_path":"event_id","predicate":{"op":"not_empty"},"threshold":{"minimum":1},"window_seconds":300,"sampling":{"rate":1},"severity":"high","owner":"data-platform","exemption_policy":{},"repair_action":"","gate_policy":"observe","expected_revision":0,"action_id":"rule-create","reason":"create event identity completeness rule"}`
	ruleCreate := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/rules", ruleBody, tenantID, "requester-a", "dq-http-rule-create-0001", authmodel.ScopeDataQualityWrite)
	if ruleCreate.Code != http.StatusOK {
		t.Fatalf("rule create status=%d body=%s", ruleCreate.Code, ruleCreate.Body.String())
	}
	ruleID, _ := governanceResponseField(t, ruleCreate, "rule_id").(string)
	revision := int64(1)
	for index, action := range []string{"start_shadow", "submit_approval"} {
		body, _ := json.Marshal(map[string]interface{}{"action": action, "expected_revision": revision, "action_id": "rule-" + action, "reason": "advance rule through governed lifecycle"})
		response := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/rules/"+ruleID+"/transitions", string(body), tenantID, "requester-a", "dq-http-rule-step-000"+strconv.Itoa(index+1), authmodel.ScopeDataQualityWrite)
		if response.Code != http.StatusOK {
			t.Fatalf("transition %s status=%d body=%s", action, response.Code, response.Body.String())
		}
		revision++
	}
	approveBody := `{"action":"approve","expected_revision":3,"action_id":"rule-approve","reason":"independent approval after shadow evidence"}`
	self := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/rules/"+ruleID+"/transitions", approveBody, tenantID, "requester-a", "dq-http-rule-self-approve", authmodel.ScopeDataQualityWrite)
	if self.Code != http.StatusForbidden {
		t.Fatalf("self approval status=%d body=%s", self.Code, self.Body.String())
	}
	approved := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/rules/"+ruleID+"/transitions", approveBody, tenantID, "reviewer-b", "dq-http-rule-approve-001", authmodel.ScopeDataQualityWrite)
	if approved.Code != http.StatusOK || governanceResponseField(t, approved, "status") != "active" {
		t.Fatalf("independent approval status=%d body=%s", approved.Code, approved.Body.String())
	}

	qualityEventID := uuid.NewString()
	windowStart := time.Date(2026, 8, 4, 12, 5, 0, 0, time.UTC)
	windowEnd := windowStart.Add(5 * time.Minute)
	if _, err := db.Exec(`
		INSERT INTO data_quality_events (
			quality_event_id,tenant_id,dataset_id,rule_id,rule_version,status,severity,
			window_start,window_end,affected_count,measured_value,source_watermarks,owner,revision,trace_id
		) SELECT $1,$2,'flows_raw',rule_id,rule_version,'detected','high',$4,$5,2,
			'{"ratio":0.8,"minimum":1}'::jsonb,'{}'::jsonb,'data-platform',1,'trace-http-repair-event'
		FROM data_quality_rules WHERE tenant_id=$2 AND rule_id=$3
	`, qualityEventID, tenantID, ruleID, windowStart, windowEnd); err != nil {
		t.Fatal(err)
	}
	repairCreateBody, _ := json.Marshal(map[string]interface{}{
		"operation_id": "flow_replay_window_v1",
		"input_scope": map[string]interface{}{
			"dataset_id": "flows_raw", "tenant_id": tenantID,
			"window_start": windowStart.Format(time.RFC3339), "window_end": windowEnd.Format(time.RFC3339),
		},
		"resource_budget": map[string]interface{}{"max_rows": 1000, "max_duration_seconds": 60},
		"action_id":       "repair-create", "reason": "plan bounded flow replay for HTTP quality event",
	})
	repairCreate := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/events/"+qualityEventID+"/repairs", string(repairCreateBody), tenantID, "requester-a", "dq-http-repair-create-0001", authmodel.ScopeDataQualityWrite)
	if repairCreate.Code != http.StatusOK || governanceResponseField(t, repairCreate, "status") != "planned" {
		t.Fatalf("repair create status=%d body=%s", repairCreate.Code, repairCreate.Body.String())
	}
	repairID, _ := governanceResponseField(t, repairCreate, "repair_id").(string)
	repairRevision := int64(1)
	transitionRepair := func(action, actor, key string, summary map[string]interface{}, wantStatus int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{
			"action": action, "expected_revision": repairRevision, "summary": summary,
			"action_id": "repair-" + action, "reason": "advance bounded repair through HTTP lifecycle",
		})
		response := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/repairs/"+repairID+"/transitions", string(body), tenantID, actor, key, authmodel.ScopeDataQualityWrite)
		if response.Code != wantStatus {
			t.Fatalf("repair transition %s status=%d body=%s", action, response.Code, response.Body.String())
		}
		if wantStatus == http.StatusOK || wantStatus == http.StatusAccepted {
			repairRevision++
		}
		return response
	}
	transitionRepair("complete_dry_run", "requester-a", "dq-http-repair-no-evidence", map[string]interface{}{}, http.StatusServiceUnavailable)
	handler.SetDataQualityRepairEvidenceProvider(dataQualityRepairEvidenceStub{})
	dryRun := transitionRepair("complete_dry_run", "requester-a", "dq-http-repair-dryrun-001", map[string]interface{}{"within_budget": false, "destructive": true, "estimated_rows": 999999}, http.StatusOK)
	dryRunSummary := governanceResponseField(t, dryRun, "repair_summary").(map[string]interface{})
	if dryRunSummary["destructive"] != false || dryRunSummary["evidence_source"] != "server-test-provider" {
		t.Fatalf("client dry-run summary was trusted: %s", dryRun.Body.String())
	}
	transitionRepair("submit_approval", "requester-a", "dq-http-repair-submit-001", map[string]interface{}{}, http.StatusOK)
	transitionRepair("approve", "requester-a", "dq-http-repair-self-approve", map[string]interface{}{}, http.StatusForbidden)
	transitionRepair("approve", "reviewer-b", "dq-http-repair-approve-001", map[string]interface{}{}, http.StatusOK)
	transitionRepair("start_execution", "executor-a", "dq-http-repair-disabled-01", map[string]interface{}{}, http.StatusServiceUnavailable)
	handler.SetDataQualityRepairExecutionFeatureFlag(true)
	transitionRepair("start_execution", "executor-a", "dq-http-repair-no-executor", map[string]interface{}{}, http.StatusServiceUnavailable)
	handler.SetDataQualityRepairExecutor(dataQualityRepairExecutorReadyStub{})
	started := transitionRepair("start_execution", "executor-a", "dq-http-repair-execute-001", map[string]interface{}{}, http.StatusAccepted)
	if governanceResponseField(t, started, "status") != "executing" {
		t.Fatalf("accepted repair did not enter executing: %s", started.Body.String())
	}
	transitionRepair("reconcile", "reconciler-a", "dq-http-repair-too-early-01", map[string]interface{}{"all_match": true, "missing_count": 0, "extra_count": 0}, http.StatusConflict)
	executed, err := monitor.TransitionRepair(context.Background(), dataquality.RepairTransitionCommand{
		TenantID: tenantID, RepairID: repairID, Action: "record_executed", ExpectedRevision: repairRevision,
		Summary:  map[string]interface{}{"published": true, "published_rows": float64(2), "server_derived": true},
		ActionID: "repair-record-executed", IdempotencyKey: "dq-http-repair-record-executed-01",
		Reason: "record server derived bounded replay completion", Actor: "system:data-quality-repair-executor", TraceID: "trace-http-repair-executed",
	}, true)
	if err != nil || executed.Status != "executed" {
		t.Fatalf("record executed replay: record=%+v err=%v", executed, err)
	}
	repairRevision = executed.Revision
	reconciled := transitionRepair("reconcile", "reconciler-a", "dq-http-repair-reconcile-01", map[string]interface{}{"all_match": true, "missing_count": 0, "extra_count": 0}, http.StatusOK)
	if governanceResponseField(t, reconciled, "status") != "reconciled" {
		t.Fatalf("repair did not reconcile: %s", reconciled.Body.String())
	}

	crossTenantBody, _ := json.Marshal(map[string]interface{}{
		"operation_id": "flow_replay_window_v1",
		"input_scope": map[string]interface{}{
			"dataset_id": "flows_raw", "tenant_id": otherTenantID,
			"window_start": windowStart.Format(time.RFC3339), "window_end": windowEnd.Format(time.RFC3339),
		},
		"resource_budget": map[string]interface{}{"max_rows": 1000, "max_duration_seconds": 60},
		"action_id":       "repair-cross-tenant", "reason": "verify quality event tenant isolation at repair boundary",
	})
	crossTenant := performDataQualityGovernanceRequest(served, http.MethodPost, "/api/v1/data-quality/events/"+qualityEventID+"/repairs", string(crossTenantBody), otherTenantID, "requester-b", "dq-http-repair-cross-tenant", authmodel.ScopeDataQualityWrite)
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant repair status=%d body=%s", crossTenant.Code, crossTenant.Body.String())
	}

	other := performDataQualityGovernanceRequest(served, http.MethodGet, "/api/v1/data-quality/datasets", "", otherTenantID, "reader-b", "", authmodel.ScopeDataQualityRead)
	if other.Code != http.StatusOK {
		t.Fatalf("other tenant list status=%d body=%s", other.Code, other.Body.String())
	}
	response := decodeDataQualityGovernanceResponse(t, other)
	if response["data"].(map[string]interface{})["total"] != float64(0) {
		t.Fatalf("cross-tenant dataset leak: %s", other.Body.String())
	}
}

type dataQualityRepairExecutorReadyStub struct{}

func (dataQualityRepairExecutorReadyStub) Ready(context.Context) error { return nil }

type dataQualityRepairEvidenceStub struct{}

func (dataQualityRepairEvidenceStub) DryRun(context.Context, string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"within_budget": true, "destructive": false, "estimated_rows": float64(2), "evidence_source": "server-test-provider"}, nil
}

func (dataQualityRepairEvidenceStub) Reconcile(context.Context, string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"all_match": true, "missing_count": float64(0), "extra_count": float64(0), "evidence_source": "server-test-provider"}, nil
}

func performDataQualityGovernanceRequest(handler http.Handler, method, path, body, tenantID, actor, idempotencyKey string, permissions ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, actor)
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request.WithContext(ctx))
	return recorder
}

func decodeDataQualityGovernanceResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode governance response: %v body=%s", err, recorder.Body.String())
	}
	return response
}

func governanceResponseField(t *testing.T, recorder *httptest.ResponseRecorder, field string) interface{} {
	t.Helper()
	response := decodeDataQualityGovernanceResponse(t, recorder)
	return response["data"].(map[string]interface{})[field]
}

func cleanupDataQualityGovernanceHTTPIntegration(t *testing.T, db *sql.DB, tenantIDs ...string) {
	t.Helper()
	for _, tenantID := range tenantIDs {
		statements := []string{
			`DELETE FROM data_quality_repair_requests WHERE tenant_id=$1`, `DELETE FROM data_quality_repair_history WHERE tenant_id=$1`,
			`DELETE FROM data_quality_command_requests WHERE tenant_id=$1`, `DELETE FROM data_quality_rule_history WHERE tenant_id=$1`,
			`DELETE FROM data_quality_dataset_history WHERE tenant_id=$1`, `DELETE FROM data_quality_outbox WHERE tenant_id=$1`,
			`DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type IN ('data_quality_dataset','data_quality_rule','data_quality_rule_evaluation','data_quality_repair')`,
			`DELETE FROM data_quality_repairs WHERE tenant_id=$1`, `DELETE FROM data_quality_rule_evaluations WHERE tenant_id=$1`,
			`DELETE FROM data_quality_events WHERE tenant_id=$1`,
			`DELETE FROM data_quality_rules WHERE tenant_id=$1`, `DELETE FROM data_quality_datasets WHERE tenant_id=$1`, `DELETE FROM tenants WHERE tenant_id=$1`,
		}
		for _, statement := range statements {
			if _, err := db.Exec(statement, tenantID); err != nil {
				t.Errorf("cleanup governance HTTP integration: %v", err)
				return
			}
		}
	}
}

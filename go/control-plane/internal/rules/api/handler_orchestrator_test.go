package api

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/service"
	"go.uber.org/zap"
)

func TestRequireMLOpsWorkflowTenant(t *testing.T) {
	opCtx := &service.OperationContext{TenantID: "tenant-a"}
	if err := requireMLOpsWorkflowTenant(opCtx, &service.MLOpsWorkflow{Parameters: map[string]string{"tenant-id": "tenant-a"}}); err != nil {
		t.Fatalf("same-tenant workflow should be allowed: %v", err)
	}
	for _, workflow := range []*service.MLOpsWorkflow{
		{Parameters: map[string]string{"tenant-id": "tenant-b"}},
		{Parameters: map[string]string{}},
		nil,
	} {
		if err := requireMLOpsWorkflowTenant(opCtx, workflow); err == nil {
			t.Fatalf("workflow without the exact authenticated tenant must be denied: %+v", workflow)
		}
	}
}

type failingMLOpsAuditConnector struct{}

func (failingMLOpsAuditConnector) Connect(context.Context) (driver.Conn, error) {
	return failingMLOpsAuditConn{}, nil
}
func (failingMLOpsAuditConnector) Driver() driver.Driver { return failingMLOpsAuditDriver{} }

type failingMLOpsAuditDriver struct{}

func (failingMLOpsAuditDriver) Open(string) (driver.Conn, error) {
	return failingMLOpsAuditConn{}, nil
}

type failingMLOpsAuditConn struct{}

func (failingMLOpsAuditConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (failingMLOpsAuditConn) Close() error                        { return nil }
func (failingMLOpsAuditConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (failingMLOpsAuditConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, stderrors.New("audit unavailable")
}

type stagedMLOpsAuditConnector struct {
	execCount int
}

func (connector *stagedMLOpsAuditConnector) Connect(context.Context) (driver.Conn, error) {
	return &stagedMLOpsAuditConn{connector: connector}, nil
}
func (connector *stagedMLOpsAuditConnector) Driver() driver.Driver {
	return stagedMLOpsAuditDriver{connector: connector}
}

type stagedMLOpsAuditDriver struct {
	connector *stagedMLOpsAuditConnector
}

func (driver stagedMLOpsAuditDriver) Open(string) (driver.Conn, error) {
	return &stagedMLOpsAuditConn{connector: driver.connector}, nil
}

type stagedMLOpsAuditConn struct {
	connector *stagedMLOpsAuditConnector
}

func (*stagedMLOpsAuditConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*stagedMLOpsAuditConn) Close() error                        { return nil }
func (*stagedMLOpsAuditConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (connection *stagedMLOpsAuditConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	connection.connector.execCount++
	if connection.connector.execCount == 1 {
		return driver.RowsAffected(1), nil
	}
	return nil, stderrors.New("completion audit unavailable")
}

func TestRequiredMLOpsAuditIntentPreventsMutationWhenDatabaseFails(t *testing.T) {
	db := sql.OpenDB(failingMLOpsAuditConnector{})
	defer db.Close()
	modelService := service.NewModelService(db, nil, nil, nil, zap.NewNop(), service.DefaultModelServiceConfig())
	opCtx := &service.OperationContext{TenantID: "tenant-a", UserID: "00000000-0000-0000-0000-000000000001", Username: "tester"}
	mutationCalled := false

	_, intentEventID, err := runAfterRequiredMLOpsAuditIntent(context.Background(), modelService, opCtx, "MLOPS_WORKFLOW_STOP_INTENT", "mlops-test", nil, func() (string, error) {
		mutationCalled = true
		return "mutated", nil
	})
	if err == nil {
		t.Fatal("audit intent failure must stop the handler path")
	}
	if intentEventID != "" {
		t.Fatalf("failed intent must not return an event id: %s", intentEventID)
	}
	if mutationCalled {
		t.Fatal("Argo or local mutation must not run before required audit intent")
	}
}

func TestCompletionAuditFailureReturnsDurableIntentWithoutRetryableError(t *testing.T) {
	connector := &stagedMLOpsAuditConnector{}
	db := sql.OpenDB(connector)
	defer db.Close()
	modelService := service.NewModelService(db, nil, nil, nil, zap.NewNop(), service.DefaultModelServiceConfig())
	opCtx := &service.OperationContext{TenantID: "tenant-a", UserID: "00000000-0000-0000-0000-000000000001", Username: "tester"}

	workflowName, intentEventID, err := runAfterRequiredMLOpsAuditIntent(context.Background(), modelService, opCtx, "MLOPS_RETRAIN_SUBMIT_REQUESTED", "mlops-test", nil, func() (string, error) {
		return "mlops-test", nil
	})
	if err != nil || intentEventID == "" {
		t.Fatalf("durable intent and mutation should succeed before completion failure: event=%q err=%v", intentEventID, err)
	}
	auditEvent, pending, completionErr := recordMLOpsCompletionAfterIntent(context.Background(), modelService, opCtx, "MLOPS_RETRAIN_SUBMITTED", "MLOPS_RETRAIN_SUBMIT_REQUESTED", workflowName, map[string]interface{}{"intent_event_id": intentEventID})
	if completionErr == nil {
		t.Fatal("staged completion insert must fail")
	}
	if auditEvent != "MLOPS_RETRAIN_SUBMIT_REQUESTED" || !pending {
		t.Fatalf("response must expose the durable intent and pending reconciliation, event=%q pending=%v", auditEvent, pending)
	}
	if connector.execCount != 2 {
		t.Fatalf("expected one intent insert and one completion insert, got %d", connector.execCount)
	}
}

func TestTriggerRetrainHTTPReturnsAcceptedDurableIntentWhenCompletionAuditFails(t *testing.T) {
	connector := &stagedMLOpsAuditConnector{}
	db := sql.OpenDB(connector)
	defer db.Close()
	modelService := service.NewModelService(db, nil, nil, nil, zap.NewNop(), service.DefaultModelServiceConfig())
	argoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody struct {
			SubmitOptions struct {
				Name string `json:"name"`
			} `json:"submitOptions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode Argo request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"metadata": map[string]string{"name": requestBody.SubmitOptions.Name}})
	}))
	defer argoServer.Close()
	mlopsConfig := service.DefaultMLOpsOrchestratorConfig()
	mlopsConfig.ArgoServerURL = argoServer.URL
	mlopsConfig.ArgoNamespace = "argo"
	handler := NewHandler(nil, nil, modelService, nil, nil, zap.NewNop(), DefaultHandlerConfig())
	handler.SetOrchestrator(service.NewMLOpsOrchestrator(nil, nil, mlopsConfig, zap.NewNop()))
	router := http.NewServeMux()
	router.HandleFunc("/api/v1/mlops/retrain", handler.TriggerRetrain)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/mlops/retrain", bytes.NewBufferString(`{"model_type":"xgboost","feature_set_id":"v1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("X-User-ID", "00000000-0000-0000-0000-000000000001")
	request.Header.Set("X-Username", "tester")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("successful Argo mutation must not become retryable after completion audit failure: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			WorkflowName           string `json:"workflow_name"`
			AuditEvent             string `json:"audit_event"`
			AuditIntentEventID     string `json:"audit_intent_event_id"`
			AuditCompletionPending bool   `json:"audit_completion_pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode handler response: %v", err)
	}
	if response.Data.WorkflowName == "" || response.Data.AuditIntentEventID == "" {
		t.Fatalf("response must expose exact workflow and intent identities: %+v", response.Data)
	}
	if response.Data.AuditEvent != "MLOPS_RETRAIN_SUBMIT_REQUESTED" || !response.Data.AuditCompletionPending {
		t.Fatalf("response must expose durable intent with pending reconciliation: %+v", response.Data)
	}
	if connector.execCount != 2 {
		t.Fatalf("expected one intent insert and one completion insert, got %d", connector.execCount)
	}
}

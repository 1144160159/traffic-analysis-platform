package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPDashboardTaskExecutorRequiresConfirmedEffectForCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "dashboard-task:event-00000001" || r.Header.Get("X-Tenant-ID") != "tenant-a" {
			t.Fatalf("missing stable provider identity headers: %+v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(DashboardTaskExecutionReceipt{
			Status: "completed", Provider: "ticketing", ProviderReceiptID: "receipt-1",
			EffectState: "unknown", EffectIDs: []string{}, Result: map[string]interface{}{}, ExecutedAt: time.Now().UTC(),
		})
	}))
	defer server.Close()
	executor, err := NewHTTPDashboardTaskExecutor(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.ExecuteDashboardTask(context.Background(), dashboardProviderTestCommand())
	if err == nil || !strings.Contains(err.Error(), "completion requires confirmed external effects") {
		t.Fatalf("HTTP 2xx without confirmed effect was accepted: %v", err)
	}
}

func TestHTTPDashboardTaskExecutorAcceptsDurableCompletedReceipt(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(DashboardTaskExecutionReceipt{
			Status: "completed", Provider: " ticketing ", ProviderReceiptID: " receipt-2 ",
			EffectState: "confirmed", EffectIDs: []string{" ticket-42 "},
			Result: map[string]interface{}{"ticket_id": "ticket-42"}, ExecutedAt: now,
		})
	}))
	defer server.Close()
	executor, err := NewHTTPDashboardTaskExecutor(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := executor.ExecuteDashboardTask(context.Background(), dashboardProviderTestCommand())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Provider != "ticketing" || receipt.ProviderReceiptID != "receipt-2" || len(receipt.EffectIDs) != 1 || receipt.EffectIDs[0] != "ticket-42" {
		t.Fatalf("receipt was not normalized: %+v", receipt)
	}
}

func TestHTTPDashboardTaskExecutorRejectsRedirectAndOversizedResponse(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	executor, err := NewHTTPDashboardTaskExecutor(redirect.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.ExecuteDashboardTask(context.Background(), dashboardProviderTestCommand()); err == nil || !strings.Contains(err.Error(), "redirects are disabled") {
		t.Fatalf("redirect was not rejected: %v", err)
	}

	oversized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", dashboardTaskProviderResponseLimit+1)))
	}))
	defer oversized.Close()
	executor, err = NewHTTPDashboardTaskExecutor(oversized.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.ExecuteDashboardTask(context.Background(), dashboardProviderTestCommand()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response was not rejected: %v", err)
	}
}

func TestHTTPDashboardTaskExecutorAuthorityLookupRecoversOnlyMatchingDurableReceipt(t *testing.T) {
	command := dashboardProviderTestCommand()
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	executedAt := checkedAt.Add(-time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer authority-token" ||
			r.Header.Get("Idempotency-Key") != command.IdempotencyKey ||
			r.Header.Get("X-Tenant-ID") != command.TenantID || r.Header.Get("X-Trace-ID") != command.TraceID {
			t.Fatalf("authority lookup metadata mismatch: %+v", r.Header)
		}
		var envelope struct {
			SchemaVersion int `json:"schema_version"`
			Lookup        struct {
				RequestEventID string `json:"request_event_id"`
				TenantID       string `json:"tenant_id"`
				TaskID         string `json:"task_id"`
				IdempotencyKey string `json:"idempotency_key"`
				TraceID        string `json:"trace_id"`
			} `json:"lookup"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil || envelope.SchemaVersion != 1 ||
			envelope.Lookup.RequestEventID != command.RequestEventID || envelope.Lookup.IdempotencyKey != command.IdempotencyKey {
			t.Fatalf("authority lookup body mismatch: %+v err=%v", envelope, err)
		}
		_ = json.NewEncoder(w).Encode(DashboardTaskExecutionAuthorityLookup{
			RequestEventID: command.RequestEventID, TenantID: command.TenantID, TaskID: command.TaskID,
			IdempotencyKey: command.IdempotencyKey, TraceID: command.TraceID,
			State: "receipt_found", Provider: "ticketing", CheckedAt: checkedAt,
			Receipt: &DashboardTaskExecutionReceipt{
				Status: "completed", Provider: "ticketing", ProviderReceiptID: "receipt-authority-1",
				EffectState: "confirmed", EffectIDs: []string{"ticket-42"},
				Result: map[string]interface{}{"ticket_id": "ticket-42"}, ExecutedAt: executedAt,
			},
		})
	}))
	defer server.Close()
	executor, err := NewHTTPDashboardTaskExecutor(server.URL+"/execute", "authority-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ConfigureAuthorityLookup(server.URL + "/execute/status"); err != nil {
		t.Fatal(err)
	}
	lookup, err := executor.LookupDashboardTaskExecution(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.State != "receipt_found" || lookup.Receipt == nil || lookup.Receipt.ProviderReceiptID != "receipt-authority-1" {
		t.Fatalf("durable authority receipt was not recovered: %+v", lookup)
	}
}

func TestHTTPDashboardTaskExecutorAuthorityLookupRejectsMismatchedIdentityAndUnprovenAbsence(t *testing.T) {
	command := dashboardProviderTestCommand()
	response := DashboardTaskExecutionAuthorityLookup{
		RequestEventID: command.RequestEventID, TenantID: command.TenantID, TaskID: command.TaskID,
		IdempotencyKey: "different-key", TraceID: command.TraceID, State: "absent",
		Provider: "ticketing", CheckedAt: time.Now().UTC(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	executor, err := NewHTTPDashboardTaskExecutor(server.URL+"/execute", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ConfigureAuthorityLookup(server.URL + "/execute/status"); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.LookupDashboardTaskExecution(context.Background(), command); err == nil || !strings.Contains(err.Error(), "identity does not match") {
		t.Fatalf("mismatched authority identity was accepted: %v", err)
	}
	response.IdempotencyKey = command.IdempotencyKey
	lookup, err := executor.LookupDashboardTaskExecution(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.State != "absent" || lookup.Receipt != nil {
		t.Fatalf("absence must not fabricate a provider receipt: %+v", lookup)
	}
}

func TestValidateDashboardTaskExecutionReceiptRejectsUnknownFailure(t *testing.T) {
	err := validateDashboardTaskExecutionReceipt(DashboardTaskExecutionReceipt{
		Status: "failed", Provider: "ticketing", ProviderReceiptID: "receipt-3",
		EffectState: "unknown", Result: map[string]interface{}{}, ExecutedAt: time.Now().UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "represented as partial") {
		t.Fatalf("unknown effect was allowed as failed: %v", err)
	}
}

func TestHTTPDashboardTaskCompensatorRequiresConfirmedEffect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "dashboard-task-compensation:event-00000001" || r.Header.Get("X-Tenant-ID") != "tenant-a" {
			t.Fatalf("missing stable compensation identity headers: %+v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(DashboardTaskCompensationReceipt{
			Status: "compensated", Provider: "ticketing", ProviderReceiptID: "compensation-receipt-1",
			EffectState: "unknown", CompensatedEffectIDs: []string{}, Result: map[string]interface{}{},
			CompensatedAt: time.Now().UTC(),
		})
	}))
	defer server.Close()
	compensator, err := NewHTTPDashboardTaskCompensator(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = compensator.CompensateDashboardTask(context.Background(), dashboardProviderTestCompensationCommand())
	if err == nil || !strings.Contains(err.Error(), "compensation requires confirmed external effects") {
		t.Fatalf("HTTP 2xx without confirmed compensation was accepted: %v", err)
	}
}

func TestHTTPDashboardTaskCompensatorAcceptsDurableReceipt(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(DashboardTaskCompensationReceipt{
			Status: "compensated", Provider: " ticketing ", ProviderReceiptID: " compensation-receipt-2 ",
			EffectState: "confirmed", CompensatedEffectIDs: []string{" ticket-42 "},
			Result: map[string]interface{}{"deleted": true}, CompensatedAt: now,
		})
	}))
	defer server.Close()
	compensator, err := NewHTTPDashboardTaskCompensator(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := compensator.CompensateDashboardTask(context.Background(), dashboardProviderTestCompensationCommand())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Provider != "ticketing" || receipt.ProviderReceiptID != "compensation-receipt-2" ||
		len(receipt.CompensatedEffectIDs) != 1 || receipt.CompensatedEffectIDs[0] != "ticket-42" {
		t.Fatalf("compensation receipt was not normalized: %+v", receipt)
	}
}

func TestHTTPDashboardTaskCompensatorAuthorityLookupRecoversMatchingReceipt(t *testing.T) {
	command := dashboardProviderTestCompensationCommand()
	checkedAt := time.Now().UTC().Truncate(time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(DashboardTaskCompensationAuthorityLookup{
			RequestEventID: command.RequestEventID, TenantID: command.TenantID, TaskID: command.TaskID,
			IdempotencyKey: command.CompensationIdempotency, TraceID: command.TraceID,
			State: "receipt_found", Provider: "ticketing", CheckedAt: checkedAt,
			Receipt: &DashboardTaskCompensationReceipt{
				Status: "compensated", Provider: "ticketing", ProviderReceiptID: "compensation-authority-1",
				EffectState: "confirmed", CompensatedEffectIDs: []string{"ticket-42"},
				Result: map[string]interface{}{"removed": true}, CompensatedAt: checkedAt.Add(-time.Second),
			},
		})
	}))
	defer server.Close()
	compensator, err := NewHTTPDashboardTaskCompensator(server.URL+"/compensate", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := compensator.ConfigureAuthorityLookup(server.URL + "/compensate/status"); err != nil {
		t.Fatal(err)
	}
	lookup, err := compensator.LookupDashboardTaskCompensation(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.State != "receipt_found" || lookup.Receipt == nil || lookup.Receipt.ProviderReceiptID != "compensation-authority-1" {
		t.Fatalf("durable compensation authority receipt was not recovered: %+v", lookup)
	}
}

func TestHTTPDashboardTaskAuthorityLookupIsExplicitlyConfigured(t *testing.T) {
	executor, err := NewHTTPDashboardTaskExecutor("https://provider.example/execute", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.LookupDashboardTaskExecution(context.Background(), dashboardProviderTestCommand()); !errors.Is(err, errDashboardTaskAuthorityLookupNotConfigured) {
		t.Fatalf("unconfigured authority lookup did not fail closed: %v", err)
	}
	if err := executor.ConfigureAuthorityLookup("https://user:password@provider.example/status"); err == nil {
		t.Fatal("authority lookup URL accepted embedded credentials")
	}
	if err := executor.ConfigureAuthorityLookup("https://different-provider.example/status"); err == nil || !strings.Contains(err.Error(), "executor origin") {
		t.Fatalf("authority lookup accepted a different provider origin: %v", err)
	}
}

func TestDashboardTaskPipelineAuthorityAbsenceNeverRecoversSuccess(t *testing.T) {
	executionCommand := dashboardProviderTestCommand()
	compensationCommand := dashboardProviderTestCompensationCommand()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/execute/status":
			_ = json.NewEncoder(w).Encode(DashboardTaskExecutionAuthorityLookup{
				RequestEventID: executionCommand.RequestEventID, TenantID: executionCommand.TenantID, TaskID: executionCommand.TaskID,
				IdempotencyKey: executionCommand.IdempotencyKey, TraceID: executionCommand.TraceID,
				State: "absent", Provider: "ticketing", CheckedAt: time.Now().UTC(),
			})
		case "/compensate/status":
			_ = json.NewEncoder(w).Encode(DashboardTaskCompensationAuthorityLookup{
				RequestEventID: compensationCommand.RequestEventID, TenantID: compensationCommand.TenantID, TaskID: compensationCommand.TaskID,
				IdempotencyKey: compensationCommand.CompensationIdempotency, TraceID: compensationCommand.TraceID,
				State: "absent", Provider: "ticketing", CheckedAt: time.Now().UTC(),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	executor, err := NewHTTPDashboardTaskExecutor(server.URL+"/execute", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ConfigureAuthorityLookup(server.URL + "/execute/status"); err != nil {
		t.Fatal(err)
	}
	compensator, err := NewHTTPDashboardTaskCompensator(server.URL+"/compensate", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := compensator.ConfigureAuthorityLookup(server.URL + "/compensate/status"); err != nil {
		t.Fatal(err)
	}
	pipeline := &DashboardTaskPipeline{executor: executor, compensator: compensator}
	if receipt, resolution, recovered := pipeline.reconcileExecutionAuthority(context.Background(), executionCommand); recovered || resolution == nil || resolution.State != "absent" || resolution.RecoveredReceipt || receipt.ProviderReceiptID != "" {
		t.Fatalf("execution absence fabricated recovery receipt=%+v resolution=%+v recovered=%v", receipt, resolution, recovered)
	}
	if receipt, resolution, recovered := pipeline.reconcileCompensationAuthority(context.Background(), compensationCommand); recovered || resolution == nil || resolution.State != "absent" || resolution.RecoveredReceipt || receipt.ProviderReceiptID != "" {
		t.Fatalf("compensation absence fabricated recovery receipt=%+v resolution=%+v recovered=%v", receipt, resolution, recovered)
	}
}

func dashboardProviderTestCommand() DashboardTaskExecutionRequest {
	return DashboardTaskExecutionRequest{
		RequestEventID: "00000000-0000-0000-0000-000000000001", TenantID: "tenant-a",
		TaskID: "00000000-0000-0000-0000-000000000002", ActionID: "dashboard-task-create",
		TaskType: "closure", Target: "dashboard", Priority: "high", SnapshotID: "snapshot-1",
		Reason: "prove durable provider receipt", RequestedBy: "operator", TraceID: "trace-dashboard-provider",
		Context: map[string]interface{}{}, IdempotencyKey: "dashboard-task:event-00000001",
	}
}

func dashboardProviderTestCompensationCommand() DashboardTaskCompensationRequest {
	return DashboardTaskCompensationRequest{
		RequestEventID: "00000000-0000-0000-0000-000000000003", TenantID: "tenant-a",
		TaskID: "00000000-0000-0000-0000-000000000002", ActionID: dashboardTaskCompensationAction,
		SnapshotID: "snapshot-1", Reason: "remove confirmed provider effect", RequestedBy: "operator",
		TraceID: "trace-dashboard-compensator", OriginalProvider: "ticketing", OriginalReceiptID: "receipt-2",
		OriginalEffectIDs: []string{"ticket-42"}, OriginalResult: map[string]interface{}{"ticket_id": "ticket-42"},
		CompensationIdempotency: "dashboard-task-compensation:event-00000001",
	}
}

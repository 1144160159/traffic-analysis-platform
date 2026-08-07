package api

import (
	"context"
	"encoding/json"
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

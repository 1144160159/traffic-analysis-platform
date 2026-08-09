package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeAlertResponseExecutor struct {
	receipt      AlertResponseExecutionReceipt
	err          error
	lookup       AlertResponseExecutionAuthorityLookup
	lookErr      error
	command      AlertResponseExecutionCommand
	executeCalls int
	lookupCalls  int
}

func (executor *fakeAlertResponseExecutor) ExecuteAlertResponse(
	_ context.Context,
	command AlertResponseExecutionCommand,
) (AlertResponseExecutionReceipt, error) {
	executor.executeCalls++
	executor.command = command
	return executor.receipt, executor.err
}

func (executor *fakeAlertResponseExecutor) LookupAlertResponseExecution(
	_ context.Context,
	command AlertResponseExecutionCommand,
) (AlertResponseExecutionAuthorityLookup, error) {
	executor.lookupCalls++
	executor.command = command
	return executor.lookup, executor.lookErr
}

func approvedAlertResponseInput() AlertResponseProjectionInput {
	return AlertResponseProjectionInput{
		EventID:  "11111111-1111-4111-8111-111111111111",
		JobID:    "alert-action-22222222-2222-4222-8222-222222222222",
		TenantID: "tenant-a", AlertID: "alert-1", ActionID: "alert-response-block-ip",
		Action: "block_ip", Target: "198.51.100.10", Reason: "confirmed malicious source",
		RequestedBy: "operator-a", ApprovedBy: "approver-b",
		ApprovalReason: "independent approval recorded", TraceID: "trace-alert-response",
		AggregateVersion: 2, KafkaPartition: 1, KafkaOffset: 9,
	}
}

func confirmedAlertResponseReceipt() AlertResponseExecutionReceipt {
	return AlertResponseExecutionReceipt{
		Status: "completed", Provider: "firewall-provider", ProviderReceiptID: "receipt-001",
		EffectState: "confirmed", EffectIDs: []string{"rule-9001"},
		Result: map[string]interface{}{"rule_state": "active"}, ExecutedAt: time.Now().UTC(),
	}
}

func TestHTTPAlertResponseExecutorRequiresDurableBusinessReceipt(t *testing.T) {
	var command AlertResponseExecutionCommand
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") != "alert-response:event-1" ||
			r.Header.Get("X-Trace-ID") != "trace-1" || r.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("unexpected executor request: method=%s headers=%v", r.Method, r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(AlertResponseExecutionReceipt{
			Status: "completed", Provider: "firewall-provider", ProviderReceiptID: "receipt-1",
			EffectState: "confirmed", EffectIDs: []string{"rule-1"},
			Result: map[string]interface{}{"active": true}, ExecutedAt: time.Now().UTC(),
		})
	}))
	defer server.Close()
	executor, err := NewHTTPAlertResponseExecutor(server.URL, "token-1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := executor.ExecuteAlertResponse(context.Background(), AlertResponseExecutionCommand{
		EventID: "event-1", JobID: "job-1", TenantID: "tenant-a", AlertID: "alert-1",
		ActionID: "alert-response-block-ip", Action: "block_ip", Target: "198.51.100.10",
		Reason: "confirmed malicious source", RequestedBy: "operator-a", ApprovedBy: "approver-b",
		ApprovalReason: "independent approval", TraceID: "trace-1", AggregateVersion: 2,
		IdempotencyKey: "alert-response:event-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ProviderReceiptID != "receipt-1" || command.IdempotencyKey != "alert-response:event-1" {
		t.Fatalf("unexpected command/receipt: command=%#v receipt=%#v", command, receipt)
	}
}

func TestAlertResponseProjectionCommitsProviderReceiptStateAndAuditAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	executor := &fakeAlertResponseExecutor{receipt: confirmedAlertResponseReceipt()}
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ConfigureExecutor(executor); err != nil {
		t.Fatal(err)
	}
	expectNoCommittedAlertResponseReceipt(mock)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_execution_receipts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	input := approvedAlertResponseInput()
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if executor.command.IdempotencyKey != "alert-response:"+input.EventID ||
		executor.command.ApprovedBy != input.ApprovedBy || executor.command.TraceID != input.TraceID {
		t.Fatalf("executor did not receive immutable authority: %#v", executor.command)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertResponseProjectionRecoversLostProviderResponseFromAuthority(t *testing.T) {
	input := approvedAlertResponseInput()
	receipt := confirmedAlertResponseReceipt()
	executor := &fakeAlertResponseExecutor{err: errors.New("response timeout")}
	executor.lookup = AlertResponseExecutionAuthorityLookup{
		EventID: input.EventID, JobID: input.JobID, TenantID: input.TenantID,
		IdempotencyKey: "alert-response:" + input.EventID, TraceID: input.TraceID,
		State: "receipt_found", Provider: receipt.Provider, CheckedAt: time.Now().UTC(), Receipt: &receipt,
	}
	projection := &PostgresAlertResponseProjection{executor: executor}
	outcome, err := projection.resolveExecutionOutcome(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != "completed" || !outcome.ExternalEffect || outcome.ProviderReceiptID != receipt.ProviderReceiptID ||
		outcome.AuthorityLookup["recovered_receipt"] != true {
		t.Fatalf("authority recovery was not preserved: %#v", outcome)
	}
}

func TestAlertResponseProjectionRecordsUnknownEffectWithoutBlindRetry(t *testing.T) {
	input := approvedAlertResponseInput()
	executor := &fakeAlertResponseExecutor{
		err: errors.New("response timeout"),
		lookup: AlertResponseExecutionAuthorityLookup{
			EventID: input.EventID, JobID: input.JobID, TenantID: input.TenantID,
			IdempotencyKey: "alert-response:" + input.EventID, TraceID: input.TraceID,
			State: "unknown", Provider: "firewall-provider", CheckedAt: time.Now().UTC(),
		},
	}
	projection := &PostgresAlertResponseProjection{executor: executor}
	outcome, err := projection.resolveExecutionOutcome(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != "partial" || outcome.EffectState != "unknown" || outcome.ExternalEffect ||
		outcome.ProviderReceiptID != "transport-unknown:"+input.EventID {
		t.Fatalf("transport ambiguity was converted into unsafe authority: %#v", outcome)
	}
}

func TestAlertResponseProjectionAtomicallyQueuesBoundedUnknownEffectRecheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input := approvedAlertResponseInput()
	executor := &fakeAlertResponseExecutor{
		err: errors.New("response timeout"),
		lookup: AlertResponseExecutionAuthorityLookup{
			EventID: input.EventID, JobID: input.JobID, TenantID: input.TenantID,
			IdempotencyKey: "alert-response:" + input.EventID, TraceID: input.TraceID,
			State: "unknown", Provider: "firewall-provider", CheckedAt: time.Now().UTC(),
		},
	}
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ConfigureExecutor(executor); err != nil {
		t.Fatal(err)
	}
	if err := projection.ConfigureUnknownEffectReconciliation(7, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	expectNoCommittedAlertResponseReceipt(mock)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_execution_receipts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO alert_response_execution_authority_rechecks").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if executor.executeCalls != 1 || executor.lookupCalls != 1 {
		t.Fatalf("unexpected initial execution/lookup calls: %d/%d", executor.executeCalls, executor.lookupCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func alertResponseCompensationCommand() AlertResponseCompensationCommand {
	return AlertResponseCompensationCommand{
		RequestID: "11111111-1111-4111-8111-111111111111",
		EventID:   "22222222-2222-4222-8222-222222222222", JobID: "job-1",
		TenantID: "tenant-a", AlertID: "alert-1",
		OriginalActionID: "alert-response-block-ip", CompensationActionID: "alert-response-unblock-ip",
		OriginalProvider: "firewall-provider", OriginalProviderReceiptID: "execution-receipt-1",
		OriginalEffectIDs: []string{"rule-1", "rule-2"}, RequestedBy: "approver-b",
		Reason: "restore approved network access", TraceID: "trace-compensation-1",
		AggregateVersion: 4, IdempotencyKey: "alert-response-compensation:11111111-1111-4111-8111-111111111111",
	}
}

func TestHTTPAlertResponseExecutorCompensationAndAuthorityUseStableIdentity(t *testing.T) {
	command := alertResponseCompensationCommand()
	compensatedAt := time.Now().UTC().Truncate(time.Microsecond)
	var compensationCalls, lookupCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Idempotency-Key") != command.IdempotencyKey ||
			request.Header.Get("X-Request-ID") != command.RequestID ||
			request.Header.Get("X-Trace-ID") != command.TraceID {
			t.Fatalf("compensation identity headers missing: %v", request.Header)
		}
		switch request.URL.Path {
		case "/compensate":
			compensationCalls++
			_ = json.NewEncoder(w).Encode(AlertResponseCompensationReceipt{
				Status: "compensated", Provider: "firewall-provider", ProviderReceiptID: "compensation-receipt-1",
				EffectState: "compensated", CompensatedEffectIDs: []string{"rule-2", "rule-1"},
				Result: map[string]interface{}{"removed": 2}, CompensatedAt: compensatedAt,
			})
		case "/compensation-authority":
			lookupCalls++
			if request.URL.Query().Get("request_id") != command.RequestID ||
				request.URL.Query().Get("idempotency_key") != command.IdempotencyKey {
				t.Fatalf("compensation authority query lost identity: %s", request.URL.RawQuery)
			}
			receipt := AlertResponseCompensationReceipt{
				Status: "compensated", Provider: "firewall-provider", ProviderReceiptID: "compensation-receipt-1",
				EffectState: "compensated", CompensatedEffectIDs: []string{"rule-1", "rule-2"},
				Result: map[string]interface{}{"removed": 2}, CompensatedAt: compensatedAt,
			}
			_ = json.NewEncoder(w).Encode(AlertResponseCompensationAuthorityLookup{
				RequestID: command.RequestID, EventID: command.EventID, JobID: command.JobID,
				TenantID: command.TenantID, IdempotencyKey: command.IdempotencyKey, TraceID: command.TraceID,
				State: "receipt_found", Provider: receipt.Provider, CheckedAt: compensatedAt, Receipt: &receipt,
			})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	executor, err := NewHTTPAlertResponseExecutor(server.URL+"/execute", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ConfigureCompensation(server.URL+"/compensate", server.URL+"/compensation-authority"); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.CompensateAlertResponse(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	lookup, err := executor.LookupAlertResponseCompensation(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if compensationCalls != 1 || lookupCalls != 1 || lookup.Receipt == nil ||
		!sameAlertResponseEffectSet(command.OriginalEffectIDs, lookup.Receipt.CompensatedEffectIDs) {
		t.Fatalf("unexpected compensation/authority result: calls=%d/%d lookup=%#v", compensationCalls, lookupCalls, lookup)
	}
}

func TestHTTPAlertResponseExecutorRejectsCompensationEffectIdentityDrift(t *testing.T) {
	command := alertResponseCompensationCommand()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(AlertResponseCompensationReceipt{
			Status: "compensated", Provider: "firewall-provider", ProviderReceiptID: "compensation-receipt-drift",
			EffectState: "compensated", CompensatedEffectIDs: []string{"different-rule"},
			CompensatedAt: time.Now().UTC(),
		})
	}))
	defer server.Close()
	executor, err := NewHTTPAlertResponseExecutor(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ConfigureCompensation(server.URL, server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.CompensateAlertResponse(context.Background(), command); err == nil ||
		!strings.Contains(err.Error(), "does not reconcile") {
		t.Fatalf("effect identity drift was accepted: %v", err)
	}
}

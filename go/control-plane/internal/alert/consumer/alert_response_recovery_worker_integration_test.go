package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

type fakeAlertResponseRecoveryProvider struct {
	executionLookup      AlertResponseExecutionAuthorityLookup
	executionLookupCalls int
	compensationReceipt  AlertResponseCompensationReceipt
	compensationErr      error
	compensationLookup   []AlertResponseCompensationAuthorityLookup
	compensationCalls    int
	compensationLookups  int
}

func (provider *fakeAlertResponseRecoveryProvider) LookupAlertResponseExecution(
	_ context.Context,
	_ AlertResponseExecutionCommand,
) (AlertResponseExecutionAuthorityLookup, error) {
	provider.executionLookupCalls++
	return provider.executionLookup, nil
}

func (provider *fakeAlertResponseRecoveryProvider) CompensateAlertResponse(
	_ context.Context,
	_ AlertResponseCompensationCommand,
) (AlertResponseCompensationReceipt, error) {
	provider.compensationCalls++
	return provider.compensationReceipt, provider.compensationErr
}

func (provider *fakeAlertResponseRecoveryProvider) LookupAlertResponseCompensation(
	_ context.Context,
	_ AlertResponseCompensationCommand,
) (AlertResponseCompensationAuthorityLookup, error) {
	index := provider.compensationLookups
	provider.compensationLookups++
	if index >= len(provider.compensationLookup) {
		return AlertResponseCompensationAuthorityLookup{}, errors.New("no compensation authority response configured")
	}
	return provider.compensationLookup[index], nil
}

func TestAlertResponseRecoveryWorkerResolvesUnknownExecutionByAuthorityOnly(t *testing.T) {
	db := openAlertResponseIntegrationDB(t)
	suffix := time.Now().UTC().Format("150405000000")
	tenantID := "integration-authority-recheck-" + suffix
	jobID := "alert-action-authority-" + suffix
	eventID := "66666666-6666-4666-8666-" + suffix
	recheckID := "77777777-7777-4777-8777-" + suffix
	traceID := "trace-authority-recheck-" + suffix
	if _, err := db.Exec(`INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,status,
		 approval_status,revision,trace_id,idempotency_key,expected_revision,detail,
		 requested_by,approved_by,approved_at,result,error)
		VALUES ($1,$2::uuid,$3,'AL-AUTHORITY-1','alert-response-block-ip','block_ip',
		 '198.51.100.50','confirmed malicious source',false,'partial','approved',3,$4,$5,0,
		 '{}'::jsonb,'operator-a','approver-b',now(),'{}'::jsonb,'provider transport timeout')`,
		jobID, eventID, tenantID, traceID, "authority-source-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_approvals
		(approval_id,job_id,tenant_id,alert_id,decision,expected_revision,idempotency_key,
		 reason,decided_by,resulting_revision,resulting_status,approval_status)
		VALUES ($1::uuid,$2,$3,'AL-AUTHORITY-1','approve',1,$4,
		 'independent provider approval','approver-b',2,'approved_awaiting_executor','approved')`,
		"88888888-8888-4888-8888-"+suffix, jobID, tenantID, "authority-approval-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_execution_receipts
		(event_id,job_id,tenant_id,alert_id,action_id,state,simulated,external_effect,
		 aggregate_version,result,error,kafka_partition,kafka_offset,provider,
		 provider_receipt_id,effect_state,effect_ids,trace_id,receipt_sha256,
		 authority_lookup,executed_at)
		VALUES ($1::uuid,$2,$3,'AL-AUTHORITY-1','alert-response-block-ip','partial',false,false,2,
		 '{}'::jsonb,'provider transport timeout',8,$4,'alert-response-executor',$5,
		 'unknown','[]'::jsonb,$6,repeat('b',64),'{}'::jsonb,now())`,
		eventID, jobID, tenantID, time.Now().UnixNano(), "transport-unknown:"+eventID, traceID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_execution_authority_rechecks
		(recheck_id,event_id,job_id,tenant_id,trace_id,status,attempts,max_attempts,next_attempt_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,'pending',0,3,now())`,
		recheckID, eventID, jobID, tenantID, traceID,
	); err != nil {
		t.Fatal(err)
	}
	executedAt := time.Now().UTC().Truncate(time.Microsecond)
	receipt := AlertResponseExecutionReceipt{
		Status: "completed", Provider: "ephemeral-firewall", ProviderReceiptID: "authority-receipt-" + suffix,
		EffectState: "confirmed", EffectIDs: []string{"rule-authority-" + suffix},
		Result: map[string]interface{}{"rule_state": "active"}, ExecutedAt: executedAt,
	}
	provider := &fakeAlertResponseRecoveryProvider{executionLookup: AlertResponseExecutionAuthorityLookup{
		EventID: eventID, JobID: jobID, TenantID: tenantID,
		IdempotencyKey: "alert-response:" + eventID, TraceID: traceID,
		State: "receipt_found", Provider: receipt.Provider, CheckedAt: executedAt, Receipt: &receipt,
	}}
	worker, err := NewAlertResponseRecoveryWorker(db, provider, nil, nil, AlertResponseRecoveryConfig{
		ExecutionEnabled: true, BatchSize: 10, Lease: 2 * time.Second,
		RequestTimeout: time.Second, RetryBase: time.Millisecond, WorkerID: "integration-execution-authority",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var actionStatus, receiptState, effectState, recheckStatus string
	var revision, histories, audits int
	if err := db.QueryRow(`SELECT a.status,a.revision,r.state,r.effect_state,q.status,
		(SELECT count(*) FROM alert_response_authority_check_history WHERE subject_type='execution' AND subject_id=$1),
		(SELECT count(*) FROM audit_logs WHERE event_id='audit-alert-response-authority-'||$2)
		FROM alert_response_actions a
		JOIN alert_response_execution_receipts r ON r.job_id=a.job_id
		JOIN alert_response_execution_authority_rechecks q ON q.event_id=r.event_id
		WHERE a.tenant_id=$3 AND a.job_id=$4`, recheckID, eventID, tenantID, jobID).Scan(
		&actionStatus, &revision, &receiptState, &effectState, &recheckStatus, &histories, &audits,
	); err != nil {
		t.Fatal(err)
	}
	if processed != 1 || provider.executionLookupCalls != 1 || actionStatus != "completed" ||
		revision != 4 || receiptState != "completed" || effectState != "confirmed" ||
		recheckStatus != "resolved" || histories != 1 || audits != 1 {
		t.Fatalf("execution authority recovery diverged: processed=%d lookups=%d action=%s/%d receipt=%s/%s recheck=%s history=%d audit=%d",
			processed, provider.executionLookupCalls, actionStatus, revision, receiptState,
			effectState, recheckStatus, histories, audits)
	}
}

func TestAlertResponseRecoveryWorkerNeverBlindRetriesAmbiguousCompensation(t *testing.T) {
	db := openAlertResponseIntegrationDB(t)
	suffix := time.Now().UTC().Format("150405000000")
	tenantID := "integration-compensation-recovery-" + suffix
	jobID := "alert-action-compensation-recovery-" + suffix
	eventID := "99999999-9999-4999-8999-" + suffix
	requestID := "aaaaaaaa-aaaa-4aaa-8aaa-" + suffix
	traceID := "trace-compensation-recovery-" + suffix
	providerKey := "alert-response-compensation:" + requestID
	if _, err := db.Exec(`INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,status,
		 approval_status,revision,trace_id,idempotency_key,expected_revision,detail,
		 requested_by,approved_by,approved_at,result,error)
		VALUES ($1,$2::uuid,$3,'AL-COMP-RECOVERY-1','alert-response-block-ip','block_ip',
		 '198.51.100.60','confirmed malicious source',false,'compensation_queued','approved',4,$4,$5,0,
		 '{}'::jsonb,'operator-a','approver-b',now(),'{}'::jsonb,'')`,
		jobID, eventID, tenantID, traceID, "comp-recovery-source-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_execution_receipts
		(event_id,job_id,tenant_id,alert_id,action_id,state,simulated,external_effect,
		 aggregate_version,result,error,kafka_partition,kafka_offset,provider,
		 provider_receipt_id,effect_state,effect_ids,trace_id,receipt_sha256,
		 authority_lookup,executed_at)
		VALUES ($1::uuid,$2,$3,'AL-COMP-RECOVERY-1','alert-response-block-ip','completed',false,true,2,
		 '{}'::jsonb,'',9,$4,'ephemeral-firewall',$5,'confirmed',$6::jsonb,$7,
		 repeat('c',64),'{}'::jsonb,now())`,
		eventID, jobID, tenantID, time.Now().UnixNano(), "execution-comp-recovery-"+suffix,
		`["rule-comp-`+suffix+`"]`, traceID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_control_requests
		(request_id,job_id,tenant_id,alert_id,operation,expected_revision,idempotency_key,
		 reason,requested_by,state,resulting_revision,resulting_status)
		VALUES ($1::uuid,$2,$3,'AL-COMP-RECOVERY-1','compensate',3,$4,
		 'restore approved network access','security-approver-c','queued',4,'compensation_queued')`,
		requestID, jobID, tenantID, "comp-recovery-control-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_response_compensation_attempts
		(request_id,event_id,job_id,tenant_id,alert_id,original_action_id,
		 compensation_action_id,original_provider,original_provider_receipt_id,
		 original_effect_ids,requested_by,reason,trace_id,aggregate_version,
		 provider_idempotency_key,status,attempts,max_attempts,next_attempt_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,'AL-COMP-RECOVERY-1','alert-response-block-ip',
		 'alert-response-unblock-ip','ephemeral-firewall',$5,$6::jsonb,'security-approver-c',
		 'restore approved network access',$7,4,$8,'pending',0,3,now())`,
		requestID, eventID, jobID, tenantID, "execution-comp-recovery-"+suffix,
		`["rule-comp-`+suffix+`"]`, traceID, providerKey,
	); err != nil {
		t.Fatal(err)
	}
	compensatedAt := time.Now().UTC().Truncate(time.Microsecond)
	terminalReceipt := AlertResponseCompensationReceipt{
		Status: "compensated", Provider: "ephemeral-firewall", ProviderReceiptID: "compensated-" + suffix,
		EffectState: "compensated", CompensatedEffectIDs: []string{"rule-comp-" + suffix},
		Result: map[string]interface{}{"removed": true}, CompensatedAt: compensatedAt,
	}
	provider := &fakeAlertResponseRecoveryProvider{
		compensationErr: errors.New("provider response timeout after submit"),
		compensationLookup: []AlertResponseCompensationAuthorityLookup{
			{
				RequestID: requestID, EventID: eventID, JobID: jobID, TenantID: tenantID,
				IdempotencyKey: providerKey, TraceID: traceID, State: "unknown",
				Provider: "ephemeral-firewall", CheckedAt: compensatedAt,
			},
			{
				RequestID: requestID, EventID: eventID, JobID: jobID, TenantID: tenantID,
				IdempotencyKey: providerKey, TraceID: traceID, State: "receipt_found",
				Provider: "ephemeral-firewall", CheckedAt: compensatedAt.Add(time.Second), Receipt: &terminalReceipt,
			},
		},
	}
	worker, err := NewAlertResponseRecoveryWorker(db, nil, provider, provider, AlertResponseRecoveryConfig{
		CompensationEnabled: true, BatchSize: 10, Lease: 2 * time.Second,
		RequestTimeout: time.Second, RetryBase: time.Hour, WorkerID: "integration-compensation-authority",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.RunOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("first compensation recovery pass processed=%d err=%v", processed, err)
	}
	var attemptStatus string
	if err := db.QueryRow(`SELECT status FROM alert_response_compensation_attempts WHERE request_id=$1::uuid`, requestID).Scan(&attemptStatus); err != nil {
		t.Fatal(err)
	}
	if provider.compensationCalls != 1 || provider.compensationLookups != 1 || attemptStatus != "authority_pending" {
		t.Fatalf("ambiguous first pass did not enter lookup-only state: calls=%d lookups=%d status=%s",
			provider.compensationCalls, provider.compensationLookups, attemptStatus)
	}
	if _, err := db.Exec(`UPDATE alert_response_compensation_attempts SET next_attempt_at=now() WHERE request_id=$1::uuid`, requestID); err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.RunOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("second compensation recovery pass processed=%d err=%v", processed, err)
	}
	var actionStatus, receiptState, effectState string
	var revision, histories, receipts, audits int
	if err := db.QueryRow(`SELECT a.status,a.revision,c.status,r.state,r.effect_state,
		(SELECT count(*) FROM alert_response_authority_check_history WHERE subject_type='compensation' AND subject_id=$1),
		(SELECT count(*) FROM alert_response_compensation_receipts WHERE request_id=$2::uuid),
		(SELECT count(*) FROM audit_logs WHERE event_id='audit-alert-response-compensation-'||$1)
		FROM alert_response_actions a
		JOIN alert_response_compensation_attempts c ON c.job_id=a.job_id
		JOIN alert_response_compensation_receipts r ON r.request_id=c.request_id
		WHERE a.tenant_id=$3 AND a.job_id=$4`, requestID, requestID, tenantID, jobID).Scan(
		&actionStatus, &revision, &attemptStatus, &receiptState, &effectState,
		&histories, &receipts, &audits,
	); err != nil {
		t.Fatal(err)
	}
	if provider.compensationCalls != 1 || provider.compensationLookups != 2 ||
		actionStatus != "compensated" || revision != 5 || attemptStatus != "compensated" ||
		receiptState != "compensated" || effectState != "compensated" ||
		histories != 2 || receipts != 1 || audits != 1 {
		t.Fatalf("compensation authority recovery diverged: calls=%d lookups=%d action=%s/%d attempt=%s receipt=%s/%s history=%d receipts=%d audits=%d",
			provider.compensationCalls, provider.compensationLookups, actionStatus, revision,
			attemptStatus, receiptState, effectState, histories, receipts, audits)
	}
}

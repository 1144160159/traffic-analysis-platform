package api

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestAlertBatchAssignmentPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_BATCH_ASSIGNMENT_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("ALERT_BATCH_ASSIGNMENT_INTEGRATION_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	fixedNow := time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)
	handler := NewAlertBatchAssignmentHandler(db, nil, true, alertBatchTestSigningSecret)
	handler.SetCompensationEnabled(true)
	handler.now = func() time.Time { return fixedNow }
	command := alertBatchCommandContext{TenantID: "tenant-batch-a", ActorID: "actor-a", IdempotencyKey: "selection-idempotency-0001", TraceID: "trace-batch-0001", SourceIP: "127.0.0.1", UserAgent: "integration"}
	selectionRequest := AlertBatchSelectionRequest{SnapshotID: "alerts:snapshot:revision:42", Items: []AlertBatchSelectionItem{{AlertID: "alert-b", StateVersion: 12}, {AlertID: "alert-a", StateVersion: 11}}}
	selection, err := handler.createSelection(ctx, command, selectionRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayedSelection, err := handler.createSelection(ctx, command, selectionRequest)
	if err != nil || !replayedSelection.Replayed || replayedSelection.SelectionToken != selection.SelectionToken {
		t.Fatalf("selection retry not idempotent: receipt=%+v err=%v", replayedSelection, err)
	}
	var rawTokenPersisted bool
	if err := db.QueryRowContext(ctx, `SELECT response_payload::text LIKE '%' || $3 || '%'
		FROM alert_assignment_selection_requests WHERE tenant_id=$1 AND idempotency_key=$2`, command.TenantID, command.IdempotencyKey, selection.SelectionToken).Scan(&rawTokenPersisted); err != nil {
		t.Fatal(err)
	}
	if rawTokenPersisted {
		t.Fatal("selection idempotency receipt must not persist the raw bearer token")
	}
	changedSelection := selectionRequest
	changedSelection.Items = []AlertBatchSelectionItem{{AlertID: "alert-c", StateVersion: 13}}
	if _, err := handler.createSelection(ctx, command, changedSelection); !errors.Is(err, errAlertBatchIdempotencyConflict) {
		t.Fatalf("changed selection retry must conflict: %v", err)
	}

	assignmentCommand := command
	assignmentCommand.IdempotencyKey = "assignment-idempotency-0001"
	crossTenant := assignmentCommand
	crossTenant.TenantID = "tenant-batch-b"
	if _, err := handler.createAssignment(ctx, crossTenant, AlertBatchAssignmentRequest{SelectionToken: selection.SelectionToken, Assignee: "analyst-a", Reason: "approved batch assignment"}); !errors.Is(err, errAlertBatchSelectionInvalid) {
		t.Fatalf("cross-tenant selection token must fail without disclosure: %v", err)
	}
	assignmentRequest := AlertBatchAssignmentRequest{SelectionToken: selection.SelectionToken, Assignee: "analyst-a", Reason: "approved batch assignment"}
	receipt, err := handler.createAssignment(ctx, assignmentCommand, assignmentRequest)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "accepted" || receipt.AcceptedCount != 2 || receipt.AppliedCount != 0 || receipt.OutboxStatus != "pending" {
		t.Fatalf("202 receipt must remain honestly non-final: %+v", receipt)
	}
	replayedAssignment, err := handler.createAssignment(ctx, assignmentCommand, assignmentRequest)
	if err != nil || !replayedAssignment.Replayed || replayedAssignment.BatchID != receipt.BatchID || replayedAssignment.EventID != receipt.EventID {
		t.Fatalf("assignment retry not idempotent: receipt=%+v err=%v", replayedAssignment, err)
	}
	changedAssignment := assignmentRequest
	changedAssignment.Assignee = "analyst-b"
	if _, err := handler.createAssignment(ctx, assignmentCommand, changedAssignment); !errors.Is(err, errAlertBatchIdempotencyConflict) {
		t.Fatalf("changed assignment retry must conflict: %v", err)
	}
	secondCommand := assignmentCommand
	secondCommand.IdempotencyKey = "assignment-idempotency-0002"
	if _, err := handler.createAssignment(ctx, secondCommand, assignmentRequest); !errors.Is(err, errAlertBatchSelectionInvalid) {
		t.Fatalf("consumed selection token must not dispatch twice: %v", err)
	}

	job, err := handler.getAssignment(ctx, assignmentCommand.TenantID, receipt.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Items) != 2 || job.Items[0].AlertID != "alert-b" || job.Items[1].AlertID != "alert-a" || job.Status != "accepted" {
		t.Fatalf("frozen ordered job results mismatch: %+v", job)
	}
	if _, err := handler.getAssignment(ctx, "tenant-batch-b", receipt.BatchID); !errors.Is(err, errAlertBatchNotFound) {
		t.Fatalf("cross-tenant job lookup must be indistinguishable from missing: %v", err)
	}
	var itemCount, itemHistoryCount, batchHistoryCount, outboxCount, requestCount, auditCount int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM alert_assignment_batch_items WHERE tenant_id=$1 AND batch_id::text=$2),
		(SELECT count(*) FROM alert_assignment_batch_item_history WHERE tenant_id=$1 AND batch_id::text=$2 AND trace_id=$3),
		(SELECT count(*) FROM alert_assignment_batch_history WHERE tenant_id=$1 AND batch_id::text=$2 AND trace_id=$3),
		(SELECT count(*) FROM alert_assignment_batch_outbox WHERE tenant_id=$1 AND batch_id::text=$2 AND trace_id=$3 AND status='pending'),
		(SELECT count(*) FROM alert_assignment_batch_requests WHERE tenant_id=$1 AND batch_id::text=$2 AND trace_id=$3),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_id=$2 AND trace_id=$3 AND action='ALERT_BATCH_ASSIGNMENT_ACCEPTED')`,
		assignmentCommand.TenantID, receipt.BatchID, assignmentCommand.TraceID).Scan(&itemCount, &itemHistoryCount, &batchHistoryCount, &outboxCount, &requestCount, &auditCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 2 || itemHistoryCount != 2 || batchHistoryCount != 1 || outboxCount != 1 || requestCount != 1 || auditCount != 1 {
		t.Fatalf("batch facts did not commit atomically: items=%d item_history=%d batch_history=%d outbox=%d request=%d audit=%d", itemCount, itemHistoryCount, batchHistoryCount, outboxCount, requestCount, auditCount)
	}

	expiringCommand := command
	expiringCommand.IdempotencyKey = "selection-idempotency-expired"
	expiringSelection, err := handler.createSelection(ctx, expiringCommand, AlertBatchSelectionRequest{SnapshotID: "alerts:snapshot:revision:43", Items: []AlertBatchSelectionItem{{AlertID: "alert-expired", StateVersion: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	handler.now = func() time.Time { return fixedNow.Add(alertBatchSelectionTTL + time.Second) }
	expiredAssignment := assignmentCommand
	expiredAssignment.IdempotencyKey = "assignment-idempotency-expired"
	if _, err := handler.createAssignment(ctx, expiredAssignment, AlertBatchAssignmentRequest{SelectionToken: expiringSelection.SelectionToken, Assignee: "analyst-a", Reason: "expired selection check"}); !errors.Is(err, errAlertBatchSelectionInvalid) {
		t.Fatalf("expired selection token must fail: %v", err)
	}

	handler.now = func() time.Time { return fixedNow }
	auditSelectionCommand := command
	auditSelectionCommand.IdempotencyKey = "selection-idempotency-audit-fail"
	auditSelection, err := handler.createSelection(ctx, auditSelectionCommand, AlertBatchSelectionRequest{SnapshotID: "alerts:snapshot:revision:44", Items: []AlertBatchSelectionItem{{AlertID: "alert-audit-fail", StateVersion: 21}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE FUNCTION reject_alert_batch_assignment_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.action='ALERT_BATCH_ASSIGNMENT_ACCEPTED' AND NEW.trace_id='trace-audit-failure' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END $$;
		DROP TRIGGER IF EXISTS reject_alert_batch_assignment_audit_trigger ON audit_logs;
		CREATE TRIGGER reject_alert_batch_assignment_audit_trigger BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_alert_batch_assignment_audit()`); err != nil {
		t.Fatal(err)
	}
	auditFailureCommand := assignmentCommand
	auditFailureCommand.IdempotencyKey = "assignment-idempotency-audit-fail"
	auditFailureCommand.TraceID = "trace-audit-failure"
	if _, err := handler.createAssignment(ctx, auditFailureCommand, AlertBatchAssignmentRequest{SelectionToken: auditSelection.SelectionToken, Assignee: "analyst-a", Reason: "audit rollback check"}); err == nil {
		t.Fatal("injected audit failure must roll back the entire assignment transaction")
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER reject_alert_batch_assignment_audit_trigger ON audit_logs; DROP FUNCTION reject_alert_batch_assignment_audit()`); err != nil {
		t.Fatal(err)
	}
	var failedBatches, failedOutbox, failedRequests, consumedSelections int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM alert_assignment_batches WHERE tenant_id=$1 AND trace_id='trace-audit-failure'),
		(SELECT count(*) FROM alert_assignment_batch_outbox WHERE tenant_id=$1 AND trace_id='trace-audit-failure'),
		(SELECT count(*) FROM alert_assignment_batch_requests WHERE tenant_id=$1 AND trace_id='trace-audit-failure'),
		(SELECT count(*) FROM alert_assignment_selections WHERE tenant_id=$1 AND selection_id=$2 AND consumed_by_batch_id IS NOT NULL)`,
		auditFailureCommand.TenantID, auditSelection.SelectionID).Scan(&failedBatches, &failedOutbox, &failedRequests, &consumedSelections); err != nil {
		t.Fatal(err)
	}
	if failedBatches != 0 || failedOutbox != 0 || failedRequests != 0 || consumedSelections != 0 {
		t.Fatalf("audit failure leaked transaction facts: batches=%d outbox=%d requests=%d consumed=%d", failedBatches, failedOutbox, failedRequests, consumedSelections)
	}

	t.Logf("alert_batch_assignment_postgres=pass batch_id=%s event_id=%s selection_sha256=%s", receipt.BatchID, receipt.EventID, receipt.SelectionSHA256)
}

func TestAlertBatchAssignmentTerminalQueryPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var batchID string
	if err := db.QueryRowContext(ctx, `SELECT batch_id::text FROM alert_assignment_batches
		WHERE tenant_id='tenant-batch-a' AND status='completed' ORDER BY completed_at DESC LIMIT 1`).Scan(&batchID); err != nil {
		t.Fatalf("completed pipeline batch prerequisite missing: %v", err)
	}
	handler := NewAlertBatchAssignmentHandler(db, nil, true, alertBatchTestSigningSecret)
	job, err := handler.getAssignment(ctx, "tenant-batch-a", batchID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" || job.Revision != 3 || job.TotalCount != 2 || job.AcceptedCount != 0 || job.AppliedCount != 2 ||
		job.ConflictedCount != 0 || job.ForbiddenCount != 0 || job.FailedCount != 0 || job.OutboxStatus != "published" || len(job.Items) != 2 {
		t.Fatalf("terminal query receipt mismatch: %+v", job)
	}
	for _, item := range job.Items {
		if item.Status != "applied" || item.ItemRevision != 3 || item.ResultingStateVersion <= item.ExpectedStateVersion {
			t.Fatalf("terminal item receipt mismatch: %+v", item)
		}
	}
	compensationCommand := alertBatchCommandContext{TenantID: "tenant-batch-a", ActorID: "actor-compensation-a",
		IdempotencyKey: "assignment-compensation-0001", TraceID: "trace-assignment-compensation-0001",
		SourceIP: "127.0.0.1", UserAgent: "integration"}
	compensationRequest := AlertBatchAssignmentCompensationRequest{ActionID: alertBatchCompensationActionID,
		ExpectedRevision: 3, Reason: "operator approved revision-safe rollback"}
	compensation, err := handler.createCompensation(ctx, compensationCommand, batchID, compensationRequest)
	if err != nil {
		t.Fatal(err)
	}
	if compensation.Status != "accepted" || compensation.Revision != 1 || compensation.TotalCount != 2 ||
		compensation.AcceptedCount != 2 || compensation.CompensatedCount != 0 || compensation.OutboxStatus != "pending" {
		t.Fatalf("compensation 202 receipt must remain honestly non-final: %+v", compensation)
	}
	replayed, err := handler.createCompensation(ctx, compensationCommand, batchID, compensationRequest)
	if err != nil || !replayed.Replayed || replayed.RequestID != compensation.RequestID || replayed.EventID != compensation.EventID {
		t.Fatalf("compensation retry not idempotent: receipt=%+v err=%v", replayed, err)
	}
	changedRequest := compensationRequest
	changedRequest.Reason = "different rollback reason must conflict"
	if _, err := handler.createCompensation(ctx, compensationCommand, batchID, changedRequest); !errors.Is(err, errAlertBatchIdempotencyConflict) {
		t.Fatalf("changed compensation retry must conflict: %v", err)
	}
	secondCommand := compensationCommand
	secondCommand.IdempotencyKey = "assignment-compensation-0002"
	if _, err := handler.createCompensation(ctx, secondCommand, batchID, compensationRequest); !errors.Is(err, errAlertBatchCompensationConflict) {
		t.Fatalf("a second compensation request must not own the same batch: %v", err)
	}
	compensationJob, err := handler.getCompensation(ctx, "tenant-batch-a", batchID, compensation.RequestID)
	if err != nil || compensationJob.Status != "accepted" || len(compensationJob.Items) != 2 {
		t.Fatalf("accepted compensation query mismatch: job=%+v err=%v", compensationJob, err)
	}
	for _, item := range compensationJob.Items {
		if item.Status != "accepted" || item.ItemRevision != 1 || item.ExpectedStateVersion <= 0 ||
			item.RestoreStatus != "new" || item.CurrentStatus != "assigned" {
			t.Fatalf("compensation source authority mismatch: %+v", item)
		}
	}
	t.Logf("alert_batch_assignment_terminal_query_postgres=pass batch_id=%s event_id=%s compensation_request_id=%s",
		job.BatchID, job.EventID, compensation.RequestID)
}

func TestAlertBatchAssignmentCompensationTerminalQueryPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var batchID, requestID string
	if err := db.QueryRowContext(ctx, `SELECT batch_id::text,request_id::text FROM alert_assignment_compensation_requests
		WHERE tenant_id='tenant-batch-a' AND status='partial' ORDER BY completed_at DESC LIMIT 1`).Scan(&batchID, &requestID); err != nil {
		t.Fatalf("partial compensation prerequisite missing: %v", err)
	}
	handler := NewAlertBatchAssignmentHandler(db, nil, true, alertBatchTestSigningSecret)
	handler.SetCompensationEnabled(true)
	job, err := handler.getCompensation(ctx, "tenant-batch-a", batchID, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "partial" || job.Revision != 3 || job.TotalCount != 2 || job.AcceptedCount != 0 ||
		job.CompensatedCount != 1 || job.ConflictedCount != 1 || job.FailedCount != 0 ||
		job.OutboxStatus != "published" || len(job.Items) != 2 {
		t.Fatalf("terminal compensation receipt mismatch: %+v", job)
	}
	statuses := map[string]int{}
	for _, item := range job.Items {
		statuses[item.Status]++
		if item.ItemRevision != 3 || item.CompensationStateVersion <= item.ExpectedStateVersion {
			t.Fatalf("terminal compensation item mismatch: %+v", item)
		}
	}
	if statuses["compensated"] != 1 || statuses["conflicted"] != 1 {
		t.Fatalf("terminal compensation outcomes mismatch: %+v", statuses)
	}
	if _, err := handler.getCompensation(ctx, "tenant-batch-b", batchID, requestID); !errors.Is(err, errAlertBatchNotFound) {
		t.Fatalf("cross-tenant compensation lookup must be indistinguishable from missing: %v", err)
	}
	t.Logf("alert_batch_assignment_compensation_terminal_query_postgres=pass batch_id=%s request_id=%s", batchID, requestID)
}

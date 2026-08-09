package consumer

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestPostgresAlertResponseProjectionIntegration(t *testing.T) {
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
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().UTC().Format("150405000000")
	tenantID := "integration-response-projection-" + suffix
	jobID := "alert-action-" + suffix
	eventID := "11111111-1111-4111-8111-" + suffix
	if _, err := db.Exec(`INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,
		 status,approval_status,revision,trace_id,idempotency_key,expected_revision,
		 detail,requested_by,approved_by,approved_at)
		VALUES ($1,$2::uuid,$3,'AL-PROJECTION-1','alert-response-block-ip','block_ip',
		 '198.51.100.10','confirmed malicious source',false,
		 'approved_awaiting_executor','approved',2,'trace-integration',$4,0,
		 '{}'::jsonb,'operator-a','approver-b',now())`,
		jobID, eventID, tenantID, "projection-idempotency-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	input := AlertResponseProjectionInput{
		EventID: eventID, JobID: jobID, TenantID: tenantID,
		AlertID: "AL-PROJECTION-1", ActionID: "alert-response-block-ip",
		Action: "block_ip", Target: "198.51.100.10",
		Reason: "confirmed malicious source", RequestedBy: "operator-a",
		DryRun: false, AggregateVersion: 2, KafkaPartition: 1, KafkaOffset: 10,
	}
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	var state, approvalStatus string
	var externalEffect bool
	var revision, aggregateVersion int64
	if err := db.QueryRow(`SELECT a.status,a.approval_status,a.revision,
		r.external_effect,r.aggregate_version
		FROM alert_response_actions a JOIN alert_response_execution_receipts r ON r.job_id=a.job_id
		WHERE a.tenant_id=$1 AND a.job_id=$2`,
		tenantID, jobID,
	).Scan(&state, &approvalStatus, &revision, &externalEffect, &aggregateVersion); err != nil {
		t.Fatal(err)
	}
	if state != "blocked_external_executor" || approvalStatus != "approved" ||
		revision != 3 || externalEffect || aggregateVersion != 2 {
		t.Fatalf("unexpected receipt projection: state=%s approval=%s revision=%d external=%t aggregate=%d",
			state, approvalStatus, revision, externalEffect, aggregateVersion)
	}

	// Kafka may replay the stable event at a different offset. Its immutable
	// business identity remains idempotent and must not bump the action again.
	input.KafkaPartition = 2
	input.KafkaOffset = 99
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatalf("stable replay at a new offset failed: %v", err)
	}
	if err := db.QueryRow(`SELECT revision FROM alert_response_actions WHERE tenant_id=$1 AND job_id=$2`,
		tenantID, jobID,
	).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 3 {
		t.Fatalf("idempotent replay changed revision to %d", revision)
	}

	// Reusing the event identity with another aggregate version is a collision.
	input.AggregateVersion = 3
	input.KafkaOffset = 100
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err == nil {
		t.Fatal("aggregate-version collision was accepted")
	}

	cancelledJobID := "alert-action-cancelled-" + suffix
	cancelledEventID := "22222222-2222-4222-8222-" + suffix
	if _, err := db.Exec(`INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,
		 status,approval_status,revision,trace_id,idempotency_key,expected_revision,
		 detail,requested_by,approved_by,approved_at)
		VALUES ($1,$2::uuid,$3,'AL-PROJECTION-2','alert-response-block-ip','block_ip',
		 '198.51.100.11','confirmed malicious source',false,
		 'cancelled','approved',3,'trace-integration',$4,0,
		 '{}'::jsonb,'operator-a','approver-b',now())`,
		cancelledJobID, cancelledEventID, tenantID, "projection-cancelled-"+suffix,
	); err != nil {
		t.Fatal(err)
	}
	cancelledInput := input
	cancelledInput.EventID = cancelledEventID
	cancelledInput.JobID = cancelledJobID
	cancelledInput.AlertID = "AL-PROJECTION-2"
	cancelledInput.Target = "198.51.100.11"
	cancelledInput.AggregateVersion = 2
	cancelledInput.KafkaOffset = 101
	if err := projection.ApplyAlertResponseProjection(context.Background(), cancelledInput); err == nil {
		t.Fatal("cancelled terminal action accepted a late execution receipt")
	}
	var cancelledReceipts int
	if err := db.QueryRow(`SELECT count(*) FROM alert_response_execution_receipts WHERE job_id=$1`, cancelledJobID).Scan(&cancelledReceipts); err != nil {
		t.Fatal(err)
	}
	if cancelledReceipts != 0 {
		t.Fatalf("late receipt transaction was not rolled back: receipts=%d", cancelledReceipts)
	}
}

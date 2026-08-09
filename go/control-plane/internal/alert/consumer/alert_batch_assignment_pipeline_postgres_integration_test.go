package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
)

type alertBatchIntegrationAuthority struct {
	versions       map[string]uint64
	assignees      map[string]string
	statuses       map[string]string
	projectionCall int
}

func (authority *alertBatchIntegrationAuthority) GetAlert(_ context.Context, tenantID, alertID string) (*service.AlertDetailDTO, error) {
	version, ok := authority.versions[alertID]
	if !ok || tenantID != "tenant-batch-a" {
		return nil, fmt.Errorf("unexpected alert authority lookup %s/%s", tenantID, alertID)
	}
	return &service.AlertDetailDTO{AlertDTO: service.AlertDTO{
		AlertID: alertID, TenantID: tenantID, Status: authority.statuses[alertID], Assignee: authority.assignees[alertID], StateVersion: version,
	}}, nil
}

func (authority *alertBatchIntegrationAuthority) ProjectAlertAssignment(_ context.Context, tenantID, alertID, assignee, _ string, expectedVersion, resultingVersion uint64) (*service.AlertAssignmentProjectionResult, error) {
	if tenantID != "tenant-batch-a" || authority.versions[alertID] != expectedVersion || resultingVersion <= expectedVersion {
		return nil, fmt.Errorf("unexpected deterministic projection %s/%s %d->%d", tenantID, alertID, expectedVersion, resultingVersion)
	}
	authority.versions[alertID] = resultingVersion
	authority.assignees[alertID] = assignee
	authority.statuses[alertID] = "assigned"
	authority.projectionCall++
	return &service.AlertAssignmentProjectionResult{AlertID: alertID, Assignee: assignee, ResultingStateVersion: resultingVersion}, nil
}

func (authority *alertBatchIntegrationAuthority) ProjectAlertAssignmentCompensation(_ context.Context, tenantID, alertID, assignee, status, _ string, expectedVersion, resultingVersion uint64) (*service.AlertAssignmentProjectionResult, error) {
	if tenantID != "tenant-batch-a" || authority.versions[alertID] != expectedVersion || resultingVersion <= expectedVersion {
		return nil, commonerrors.Newf(commonerrors.ErrCodeVersionConflict,
			"unexpected deterministic compensation %s/%s %d->%d", tenantID, alertID, expectedVersion, resultingVersion)
	}
	authority.versions[alertID] = resultingVersion
	authority.assignees[alertID] = assignee
	authority.statuses[alertID] = status
	authority.projectionCall++
	return &service.AlertAssignmentProjectionResult{AlertID: alertID, Assignee: assignee, Status: status, ResultingStateVersion: resultingVersion}, nil
}

type alertBatchPublishedMessage struct {
	key     string
	payload []byte
	headers []commonkafka.MessageHeader
}

func (published alertBatchPublishedMessage) received(offset int64) *commonkafka.ReceivedMessage {
	headers := make([]segmentkafka.Header, 0, len(published.headers))
	for _, header := range published.headers {
		headers = append(headers, segmentkafka.Header{Key: header.Key, Value: []byte(header.Value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: AlertAssignmentEventTopic, Partition: 0, Offset: offset,
		Key: []byte(published.key), Value: append([]byte(nil), published.payload...), Headers: headers,
	}}
}

func TestAlertBatchAssignmentPipelinePostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	var batchID string
	if err := db.QueryRowContext(ctx, `SELECT batch_id::text FROM alert_assignment_batches
		WHERE tenant_id='tenant-batch-a' AND status='accepted' ORDER BY created_at DESC LIMIT 1`).Scan(&batchID); err != nil {
		t.Fatalf("accepted API batch prerequisite missing: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name,status) VALUES('tenant-batch-a','Batch integration','active') ON CONFLICT(tenant_id) DO NOTHING;
		INSERT INTO users(tenant_id,username,status) VALUES('tenant-batch-a','analyst-a','active') ON CONFLICT(tenant_id,username) DO UPDATE SET status='active';
		INSERT INTO roles(tenant_id,name,permissions) VALUES('tenant-batch-a','alert-batch-writer','{"alert":"*"}'::jsonb) ON CONFLICT(tenant_id,name) DO UPDATE SET permissions=EXCLUDED.permissions;
		INSERT INTO user_roles(user_id,role_id)
		SELECT u.user_id,r.role_id FROM users u JOIN roles r ON r.tenant_id=u.tenant_id
		WHERE u.tenant_id='tenant-batch-a' AND u.username='analyst-a' AND r.name='alert-batch-writer'
		ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	authority := &alertBatchIntegrationAuthority{versions: map[string]uint64{}, assignees: map[string]string{}, statuses: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT alert_id,expected_state_version FROM alert_assignment_batch_items
		WHERE tenant_id='tenant-batch-a' AND batch_id=$1 ORDER BY position`, batchID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var alertID string
		var version uint64
		if err := rows.Scan(&alertID, &version); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		authority.versions[alertID] = version
		authority.statuses[alertID] = "new"
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	published := make([]alertBatchPublishedMessage, 0, 2)
	publisher := func(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error {
		published = append(published, alertBatchPublishedMessage{
			key: key, payload: append([]byte(nil), payload...), headers: append([]commonkafka.MessageHeader(nil), headers...),
		})
		return nil
	}
	pipeline, err := NewAlertBatchAssignmentPipeline(db, authority, publisher, AlertAssignmentEventTopic, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	if count, err := pipeline.DrainOutbox(ctx, "integration-requested", 10); err != nil || count != 1 || len(published) != 1 {
		t.Fatalf("requested outbox drain count=%d published=%d err=%v", count, len(published), err)
	}
	requestedMessage := published[0].received(10)
	if err := pipeline.HandleKafkaMessage(ctx, requestedMessage); err != nil {
		t.Fatalf("requested event failed: %v", err)
	}
	if count, err := pipeline.DrainOutbox(ctx, "integration-changed", 10); err != nil || count != 1 || len(published) != 2 {
		t.Fatalf("changed outbox drain count=%d published=%d err=%v", count, len(published), err)
	}
	changedMessage := published[1].received(11)
	if err := pipeline.HandleKafkaMessage(ctx, changedMessage); err != nil {
		t.Fatalf("changed event failed: %v", err)
	}
	if authority.projectionCall != 2 {
		t.Fatalf("projection calls=%d want=2", authority.projectionCall)
	}
	if err := pipeline.HandleKafkaMessage(ctx, changedMessage); err != nil {
		t.Fatalf("exact changed-event replay failed: %v", err)
	}
	if authority.projectionCall != 2 {
		t.Fatalf("exact inbox replay repeated external projection: calls=%d", authority.projectionCall)
	}

	var status string
	var revision, totalCount, appliedCount, inboxCount, receiptCount, stateCount, terminalAuditCount int
	if err := db.QueryRowContext(ctx, `SELECT b.status,b.revision,b.total_count,b.applied_count,
		(SELECT count(*) FROM alert_assignment_batch_inbox i WHERE i.tenant_id=b.tenant_id AND i.batch_id=b.batch_id),
		(SELECT count(*) FROM alert_assignment_projection_receipts r WHERE r.tenant_id=b.tenant_id AND r.batch_id=b.batch_id),
		(SELECT count(*) FROM alert_assignment_states s WHERE s.tenant_id=b.tenant_id AND s.source_batch_id=b.batch_id AND s.projection_status='applied' AND s.last_error=''),
		(SELECT count(*) FROM audit_logs a WHERE a.tenant_id=b.tenant_id AND a.object_id=b.batch_id::text AND a.action='ALERT_BATCH_ASSIGNMENT_TERMINAL')
		FROM alert_assignment_batches b WHERE b.tenant_id='tenant-batch-a' AND b.batch_id=$1`, batchID).Scan(
		&status, &revision, &totalCount, &appliedCount, &inboxCount, &receiptCount, &stateCount, &terminalAuditCount); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || revision != 3 || totalCount != 2 || appliedCount != 2 || inboxCount != 2 || receiptCount != 2 || stateCount != 2 || terminalAuditCount != 1 {
		t.Fatalf("terminal facts mismatch status=%s revision=%d total=%d applied=%d inbox=%d receipts=%d states=%d audits=%d",
			status, revision, totalCount, appliedCount, inboxCount, receiptCount, stateCount, terminalAuditCount)
	}

	poison := alertBatchMessage(t, alertBatchRequestedFixture(), 0, 99)
	poison.Headers[0].Value = []byte("identity-drift")
	processingErr := commonkafka.Permanent(errors.New("strict event identity rejected"))
	if err := pipeline.RecordDLQAcknowledgement(ctx, poison, processingErr); err != nil {
		t.Fatalf("DLQ acknowledgement barrier failed: %v", err)
	}
	if err := pipeline.RecordDLQAcknowledgement(ctx, poison, processingErr); err != nil {
		t.Fatalf("DLQ acknowledgement replay failed: %v", err)
	}
	var dlqReceipts int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_assignment_batch_dlq_receipts
		WHERE source_topic=$1 AND source_partition=$2 AND source_offset=$3`, poison.Topic, poison.Partition, poison.Offset).Scan(&dlqReceipts); err != nil {
		t.Fatal(err)
	}
	if dlqReceipts != 1 {
		t.Fatalf("DLQ source tuple receipts=%d want=1", dlqReceipts)
	}

	t.Logf("alert_batch_assignment_execution_postgres=pass batch_id=%s projected=%d inbox=%d receipts=%d dlq_receipts=%d",
		batchID, authority.projectionCall, inboxCount, receiptCount, dlqReceipts)
}

func TestAlertBatchAssignmentCompensationPipelinePostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("ALERT_BATCH_ASSIGNMENT_EXECUTION_INTEGRATION_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var batchID, requestID string
	if err := db.QueryRowContext(ctx, `SELECT batch_id::text,request_id::text
		FROM alert_assignment_compensation_requests WHERE tenant_id='tenant-batch-a' AND status='accepted'
		ORDER BY created_at DESC LIMIT 1`).Scan(&batchID, &requestID); err != nil {
		t.Fatalf("accepted compensation prerequisite missing: %v", err)
	}
	authority := &alertBatchIntegrationAuthority{versions: map[string]uint64{}, assignees: map[string]string{}, statuses: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT alert_id,expected_state_version,current_assignee,current_status
		FROM alert_assignment_compensation_items WHERE tenant_id='tenant-batch-a' AND request_id=$1 ORDER BY position`, requestID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var alertID, assignee, status string
		var version uint64
		if err := rows.Scan(&alertID, &version, &assignee, &status); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		authority.versions[alertID] = version
		authority.assignees[alertID] = assignee
		authority.statuses[alertID] = status
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	published := make([]alertBatchPublishedMessage, 0, 2)
	publisher := func(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error {
		published = append(published, alertBatchPublishedMessage{
			key: key, payload: append([]byte(nil), payload...), headers: append([]commonkafka.MessageHeader(nil), headers...),
		})
		return nil
	}
	pipeline, err := NewAlertBatchAssignmentPipeline(db, authority, publisher, AlertAssignmentEventTopic, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	if count, err := pipeline.DrainOutbox(ctx, "integration-compensation-requested", 10); err != nil || count != 1 || len(published) != 1 {
		t.Fatalf("compensation requested outbox drain count=%d published=%d err=%v", count, len(published), err)
	}
	requestedMessage := published[0].received(20)
	if err := pipeline.HandleKafkaMessage(ctx, requestedMessage); err != nil {
		t.Fatalf("compensation requested event failed: %v", err)
	}
	if count, err := pipeline.DrainOutbox(ctx, "integration-compensated", 10); err != nil || count != 1 || len(published) != 2 {
		t.Fatalf("compensated outbox drain count=%d published=%d err=%v", count, len(published), err)
	}
	var changed alertBatchAssignmentLifecycleEvent
	if err := json.Unmarshal(published[1].payload, &changed); err != nil {
		t.Fatal(err)
	}
	if len(changed.Items) != 2 {
		t.Fatalf("compensation projecting items=%d want=2", len(changed.Items))
	}
	// Simulate an intervening write after preflight. One item must compensate;
	// the other must become an explicit revision conflict rather than being
	// blindly overwritten.
	conflictItem := changed.Items[1]
	authority.versions[conflictItem.AlertID] = uint64(conflictItem.ExpectedStateVersion + 1)
	compensatedMessage := published[1].received(21)
	if err := pipeline.HandleKafkaMessage(ctx, compensatedMessage); err != nil {
		t.Fatalf("compensated event failed: %v", err)
	}
	if authority.projectionCall != 1 {
		t.Fatalf("successful compensation projections=%d want=1", authority.projectionCall)
	}
	if err := pipeline.HandleKafkaMessage(ctx, compensatedMessage); err != nil {
		t.Fatalf("exact compensated-event replay failed: %v", err)
	}
	if authority.projectionCall != 1 {
		t.Fatalf("exact compensation inbox replay repeated external effect: calls=%d", authority.projectionCall)
	}
	var status string
	var revision, totalCount, compensatedCount, conflictedCount, inboxCount, receiptCount, auditCount int
	if err := db.QueryRowContext(ctx, `SELECT r.status,r.revision,r.total_count,r.compensated_count,r.conflicted_count,
		(SELECT count(*) FROM alert_assignment_batch_inbox i WHERE i.tenant_id=r.tenant_id AND i.aggregate_type='alert_assignment_compensation' AND i.aggregate_id=r.request_id::text),
		(SELECT count(*) FROM alert_assignment_compensation_projection_receipts p WHERE p.tenant_id=r.tenant_id AND p.request_id=r.request_id),
		(SELECT count(*) FROM audit_logs a WHERE a.tenant_id=r.tenant_id AND a.object_id=r.request_id::text AND a.action='ALERT_BATCH_ASSIGNMENT_COMPENSATION_TERMINAL')
		FROM alert_assignment_compensation_requests r WHERE r.tenant_id='tenant-batch-a' AND r.request_id=$1`, requestID).Scan(
		&status, &revision, &totalCount, &compensatedCount, &conflictedCount, &inboxCount, &receiptCount, &auditCount); err != nil {
		t.Fatal(err)
	}
	if status != "partial" || revision != 3 || totalCount != 2 || compensatedCount != 1 || conflictedCount != 1 ||
		inboxCount != 2 || receiptCount != 2 || auditCount != 1 {
		t.Fatalf("compensation terminal facts mismatch status=%s revision=%d total=%d compensated=%d conflicted=%d inbox=%d receipts=%d audits=%d",
			status, revision, totalCount, compensatedCount, conflictedCount, inboxCount, receiptCount, auditCount)
	}
	t.Logf("alert_batch_assignment_compensation_execution_postgres=pass batch_id=%s request_id=%s compensated=%d conflicted=%d inbox=%d receipts=%d",
		batchID, requestID, compensatedCount, conflictedCount, inboxCount, receiptCount)
}

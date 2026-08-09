package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

// RecordDLQAcknowledgement persists the alert-domain poison-event receipt only
// after Kafka has acknowledged the canonical DLQ record and before the common
// consumer may commit the source offset. The immutable source tuple is the
// idempotency identity across redelivery.
func (projection *PostgresAlertResponseProjection) RecordDLQAcknowledgement(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
	processingErr error,
) error {
	if projection == nil || projection.db == nil {
		return fmt.Errorf("alert response DLQ receipt database is required")
	}
	if message == nil || message.Topic != "alert.response.requested.v1" ||
		message.Partition < 0 || message.Offset < 0 {
		return fmt.Errorf("invalid alert response DLQ source identity")
	}
	if processingErr == nil || !commonkafka.IsPermanent(processingErr) {
		return fmt.Errorf("alert response DLQ receipt requires a permanent processing error")
	}
	headers := message.GetAllHeaders()
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	payloadDigest := sha256.Sum256(message.Value)
	headerDigest := sha256.Sum256(headersJSON)
	payloadSHA := hex.EncodeToString(payloadDigest[:])
	headerSHA := hex.EncodeToString(headerDigest[:])
	eventID := strings.TrimSpace(headers["event_id"])
	tenantID := strings.TrimSpace(headers["tenant_id"])
	jobID := strings.TrimSpace(headers["job_id"])
	alertID := strings.TrimSpace(headers["alert_id"])
	actionID := strings.TrimSpace(headers["action_id"])
	traceID := strings.TrimSpace(headers["trace_id"])
	var aggregateVersion sql.NullInt64
	var aggregateVersionDetail interface{}
	if raw := strings.TrimSpace(headers["aggregate_version"]); raw != "" {
		if value, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && value > 0 {
			aggregateVersion = sql.NullInt64{Int64: value, Valid: true}
			aggregateVersionDetail = value
		}
	}
	now := time.Now().UTC()
	errorMessage := truncateAlertResponseError(processingErr.Error())
	tx, err := projection.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var storedPayloadSHA, storedHeaderSHA, storedEventID, storedTenantID string
	var storedJobID, storedAlertID, storedActionID, storedTraceID string
	var storedAggregateVersion sql.NullInt64
	if err := tx.QueryRowContext(ctx, `INSERT INTO alert_response_dlq_receipts
		(source_topic,source_partition,source_offset,dlq_topic,event_id,tenant_id,job_id,
		 alert_id,action_id,aggregate_version,trace_id,error_code,error_message,
		 payload_sha256,headers_sha256,headers,acknowledged_at)
		VALUES($1,$2,$3,'dlq.v1',$4,$5,$6,$7,$8,$9,$10,'PROCESSING_FAILED',$11,$12,$13,$14::jsonb,$15)
		ON CONFLICT(source_topic,source_partition,source_offset) DO UPDATE
		SET acknowledged_at=alert_response_dlq_receipts.acknowledged_at
		RETURNING payload_sha256,headers_sha256,event_id,tenant_id,job_id,alert_id,action_id,trace_id,aggregate_version`,
		message.Topic, message.Partition, message.Offset, eventID, tenantID, jobID, alertID,
		actionID, aggregateVersion, traceID, errorMessage, payloadSHA, headerSHA,
		string(headersJSON), now,
	).Scan(&storedPayloadSHA, &storedHeaderSHA, &storedEventID, &storedTenantID,
		&storedJobID, &storedAlertID, &storedActionID, &storedTraceID,
		&storedAggregateVersion); err != nil {
		return err
	}
	if storedPayloadSHA != payloadSHA || storedHeaderSHA != headerSHA ||
		storedEventID != eventID || storedTenantID != tenantID || storedJobID != jobID ||
		storedAlertID != alertID || storedActionID != actionID || storedTraceID != traceID ||
		storedAggregateVersion != aggregateVersion {
		return fmt.Errorf("alert response DLQ source tuple collision")
	}

	auditTenantID := tenantID
	if auditTenantID == "" {
		auditTenantID = "__unknown__"
	}
	objectID := jobID
	if objectID == "" {
		objectID = fmt.Sprintf("%s:%d:%d", message.Topic, message.Partition, message.Offset)
	}
	auditIdentity := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%d", message.Topic, message.Partition, message.Offset,
	)))
	auditEventID := "audit-alert-response-dlq-" + hex.EncodeToString(auditIdentity[:])
	detailJSON, err := json.Marshal(map[string]interface{}{
		"dlq_topic": "dlq.v1", "source_topic": message.Topic,
		"source_partition": message.Partition, "source_offset": message.Offset,
		"event_id": eventID, "job_id": jobID, "alert_id": alertID,
		"action_id": actionID, "aggregate_version": aggregateVersionDetail,
		"trace_id": traceID, "payload_sha256": payloadSHA, "headers_sha256": headerSHA,
		"error_code": "PROCESSING_FAILED", "error_message": errorMessage,
		"dlq_acknowledged": true, "source_offset_commit_pending": true,
	})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,error_message,created_at)
		VALUES($1,$2,NULL,'ALERT_RESPONSE_EVENT_QUARANTINED','alert_response_event',$3,$4::jsonb,$5,
		 'quarantined',true,$6,$7) ON CONFLICT(event_id) DO NOTHING`,
		auditEventID, auditTenantID, objectID, string(detailJSON), traceID, errorMessage, now,
	); err != nil {
		return err
	}
	var storedAuditTenantID, storedObjectID, storedAuditPayloadSHA, storedAuditTopic string
	var storedAuditPartition int
	var storedAuditOffset int64
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id,object_id,detail->>'payload_sha256',
		detail->>'source_topic',(detail->>'source_partition')::integer,(detail->>'source_offset')::bigint
		FROM audit_logs WHERE event_id=$1 FOR UPDATE`, auditEventID).Scan(
		&storedAuditTenantID, &storedObjectID, &storedAuditPayloadSHA, &storedAuditTopic,
		&storedAuditPartition, &storedAuditOffset,
	); err != nil {
		return err
	}
	if storedAuditTenantID != auditTenantID || storedObjectID != objectID ||
		storedAuditPayloadSHA != payloadSHA || storedAuditTopic != message.Topic ||
		storedAuditPartition != message.Partition || storedAuditOffset != message.Offset {
		return fmt.Errorf("alert response DLQ audit identity collision")
	}
	return tx.Commit()
}

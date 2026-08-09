package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	dashboardTaskKafkaTopic                 = "dashboard.task.events.v1"
	dashboardTaskRequestedEvent             = "traffic.dashboard.v1.TaskRequested"
	dashboardTaskResultEvent                = "traffic.dashboard.v1.TaskResult"
	dashboardTaskCompensationRequestedEvent = "traffic.dashboard.v1.TaskCompensationRequested"
	dashboardTaskCompensationResultEvent    = "traffic.dashboard.v1.TaskCompensationResult"
)

type DashboardTaskPublishFunc func(context.Context, string, []byte, ...commonkafka.MessageHeader) error

type DashboardTaskPipeline struct {
	db          *sql.DB
	executor    DashboardTaskExecutor
	compensator DashboardTaskCompensator
	publish     DashboardTaskPublishFunc
	topic       string
	logger      *zap.Logger
}

func (pipeline *DashboardTaskPipeline) EnableCompensation(compensator DashboardTaskCompensator) error {
	if compensator == nil {
		return fmt.Errorf("dashboard task compensator is required")
	}
	pipeline.compensator = compensator
	return nil
}

func NewDashboardTaskPipeline(db *sql.DB, executor DashboardTaskExecutor, publish DashboardTaskPublishFunc, topic string, logger *zap.Logger) (*DashboardTaskPipeline, error) {
	if db == nil || executor == nil || publish == nil {
		return nil, fmt.Errorf("dashboard task database, executor and Kafka publisher are required")
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = dashboardTaskKafkaTopic
	}
	if topic != dashboardTaskKafkaTopic {
		return nil, fmt.Errorf("dashboard task topic must be %s", dashboardTaskKafkaTopic)
	}
	return &DashboardTaskPipeline{db: db, executor: executor, publish: publish, topic: topic, logger: logger}, nil
}

type dashboardTaskOutboxItem struct {
	OutboxID         int64
	EventID          string
	TenantID         string
	TaskID           string
	EventType        string
	SchemaVersion    int
	AggregateVersion int64
	PartitionKey     string
	Payload          []byte
	TraceID          string
}

func (pipeline *DashboardTaskPipeline) StartOutboxWorker(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := dashboardPipelineWorkerID("outbox")
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := pipeline.DrainOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && pipeline.logger != nil {
				pipeline.logger.Warn("Failed to drain dashboard task outbox", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (pipeline *DashboardTaskPipeline) DrainOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := pipeline.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM dashboard_task_outbox
		WHERE event_type IN ($1,$2,$3,$4) AND available_at<=now()
		  AND (status='pending' OR (status='processing' AND locked_until<now()))
		ORDER BY available_at,occurred_at,outbox_id LIMIT $5 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE dashboard_task_outbox o SET status='processing',attempt_count=attempt_count+1,
		  locked_until=now()+interval '60 seconds',locked_by=$6
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.event_id::text,o.tenant_id,o.task_id::text,o.event_type,
		  o.schema_version,o.aggregate_version,o.partition_key,o.payload::text,o.trace_id
	) SELECT outbox_id,event_id,tenant_id,task_id,event_type,schema_version,
		aggregate_version,partition_key,payload,trace_id FROM claimed ORDER BY outbox_id`,
		dashboardTaskRequestedEvent, dashboardTaskResultEvent, dashboardTaskCompensationRequestedEvent,
		dashboardTaskCompensationResultEvent, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]dashboardTaskOutboxItem, 0, limit)
	for rows.Next() {
		var item dashboardTaskOutboxItem
		var payload string
		if err := rows.Scan(&item.OutboxID, &item.EventID, &item.TenantID, &item.TaskID,
			&item.EventType, &item.SchemaVersion, &item.AggregateVersion, &item.PartitionKey,
			&payload, &item.TraceID); err != nil {
			return len(items), err
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if !json.Valid(item.Payload) {
			pipeline.releaseOutbox(ctx, workerID, item.OutboxID, "invalid dashboard outbox JSON")
			continue
		}
		err := pipeline.publish(ctx, item.PartitionKey, item.Payload,
			commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
			commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
			commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
			commonkafka.MessageHeader{Key: "task_id", Value: item.TaskID},
			commonkafka.MessageHeader{Key: "aggregate_version", Value: strconv.FormatInt(item.AggregateVersion, 10)},
			commonkafka.MessageHeader{Key: "schema_version", Value: strconv.Itoa(item.SchemaVersion)},
			commonkafka.MessageHeader{Key: "trace_id", Value: item.TraceID},
			commonkafka.MessageHeader{Key: "content_type", Value: "application/json"},
			commonkafka.MessageHeader{Key: "target_topic", Value: pipeline.topic},
		)
		if err != nil {
			pipeline.releaseOutbox(ctx, workerID, item.OutboxID, err.Error())
			continue
		}
		result, err := pipeline.db.ExecContext(ctx, `UPDATE dashboard_task_outbox
			SET status='published',published_at=now(),locked_until=NULL,locked_by='',last_error=''
			WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, item.OutboxID, workerID)
		if err != nil {
			return processed, err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return processed, fmt.Errorf("dashboard task outbox lease lost after Kafka acknowledgement")
		}
		processed++
	}
	return processed, nil
}

func (pipeline *DashboardTaskPipeline) releaseOutbox(ctx context.Context, workerID string, outboxID int64, message string) {
	message = truncateDashboardPipelineError(message)
	_, _ = pipeline.db.ExecContext(ctx, `UPDATE dashboard_task_outbox SET
		status=CASE WHEN attempt_count>=10 THEN 'dead' ELSE 'pending' END,
		available_at=now()+(LEAST(300,POWER(2,LEAST(attempt_count,8)))::text||' seconds')::interval,
		locked_until=NULL,locked_by='',last_error=$3
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, outboxID, workerID, message)
}

type dashboardTaskLifecycleEnvelope struct {
	EventID           string                 `json:"event_id"`
	EventType         string                 `json:"event_type"`
	SchemaVersion     int                    `json:"schema_version"`
	AggregateType     string                 `json:"aggregate_type"`
	AggregateID       string                 `json:"aggregate_id"`
	AggregateVersion  int64                  `json:"aggregate_version"`
	PartitionKey      string                 `json:"partition_key"`
	TenantID          string                 `json:"tenant_id"`
	TaskID            string                 `json:"task_id"`
	ActionID          string                 `json:"action_id"`
	Status            string                 `json:"status"`
	SnapshotID        string                 `json:"snapshot_id"`
	Provider          string                 `json:"provider,omitempty"`
	ProviderReceiptID string                 `json:"provider_receipt_id,omitempty"`
	EffectState       string                 `json:"effect_state,omitempty"`
	EffectIDs         []string               `json:"effect_ids"`
	Result            map[string]interface{} `json:"result"`
	ErrorCode         string                 `json:"error_code,omitempty"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	ReceiptSHA256     string                 `json:"receipt_sha256,omitempty"`
	TraceID           string                 `json:"trace_id"`
	OccurredAt        string                 `json:"occurred_at"`
}

func (pipeline *DashboardTaskPipeline) HandleKafkaMessage(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("dashboard task Kafka message is nil"))
	}
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	var event dashboardTaskLifecycleEnvelope
	if err := decoder.Decode(&event); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode dashboard task event: %w", err))
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return commonkafka.Permanent(fmt.Errorf("dashboard task event contains trailing JSON"))
	}
	if err := validateDashboardTaskLifecycleEnvelope(event); err != nil {
		return commonkafka.Permanent(err)
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType, "tenant_id": event.TenantID,
		"task_id": event.TaskID, "aggregate_version": strconv.FormatInt(event.AggregateVersion, 10),
		"schema_version": "1", "trace_id": event.TraceID, "content_type": "application/json",
		"target_topic": pipeline.topic,
	}
	for key, expected := range expectedHeaders {
		if message.GetHeader(key) != expected {
			return commonkafka.Permanent(fmt.Errorf("dashboard task %s header/body mismatch", key))
		}
	}
	if string(message.Key) != event.PartitionKey || message.Topic != pipeline.topic {
		return commonkafka.Permanent(fmt.Errorf("dashboard task Kafka key or topic mismatch"))
	}
	return pipeline.applyLifecycleEvent(ctx, event, message.Value, message.Partition, message.Offset)
}

// RecordDLQAcknowledgement is called only after Kafka has acknowledged the
// canonical DLQ record and before the source offset is committed. The source
// tuple is the idempotency identity: if PostgreSQL or its audit write fails,
// the common consumer retains the source offset and safely retries this
// transaction after another durable DLQ write.
func (pipeline *DashboardTaskPipeline) RecordDLQAcknowledgement(ctx context.Context, message *commonkafka.ReceivedMessage, processingErr error) error {
	if pipeline == nil || pipeline.db == nil {
		return fmt.Errorf("dashboard task DLQ receipt database is required")
	}
	if message == nil || message.Topic != pipeline.topic || message.Partition < 0 || message.Offset < 0 {
		return fmt.Errorf("invalid dashboard task DLQ source identity")
	}
	if processingErr == nil || !commonkafka.IsPermanent(processingErr) {
		return fmt.Errorf("dashboard task DLQ receipt requires a permanent processing error")
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
	tenantID := strings.TrimSpace(headers["tenant_id"])
	eventID := strings.TrimSpace(headers["event_id"])
	taskID := strings.TrimSpace(headers["task_id"])
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
	errorMessage := truncateDashboardPipelineError(processingErr.Error())
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var storedPayloadSHA, storedHeaderSHA, storedTenantID, storedEventID, storedTaskID, storedTraceID string
	var storedAggregateVersion sql.NullInt64
	if err := tx.QueryRowContext(ctx, `INSERT INTO dashboard_task_dlq_receipts
		(source_topic,source_partition,source_offset,dlq_topic,event_id,tenant_id,task_id,
		 aggregate_version,trace_id,error_code,error_message,payload_sha256,headers_sha256,headers,acknowledged_at)
		VALUES($1,$2,$3,'dlq.v1',$4,$5,$6,$7,$8,'PROCESSING_FAILED',$9,$10,$11,$12::jsonb,$13)
		ON CONFLICT(source_topic,source_partition,source_offset) DO UPDATE
		SET acknowledged_at=dashboard_task_dlq_receipts.acknowledged_at
		RETURNING payload_sha256,headers_sha256,tenant_id,event_id,task_id,trace_id,aggregate_version`,
		message.Topic, message.Partition, message.Offset, eventID, tenantID, taskID, aggregateVersion,
		traceID, errorMessage, payloadSHA, headerSHA, string(headersJSON), now).
		Scan(&storedPayloadSHA, &storedHeaderSHA, &storedTenantID, &storedEventID, &storedTaskID,
			&storedTraceID, &storedAggregateVersion); err != nil {
		return err
	}
	if storedPayloadSHA != payloadSHA || storedHeaderSHA != headerSHA || storedTenantID != tenantID ||
		storedEventID != eventID || storedTaskID != taskID || storedTraceID != traceID ||
		storedAggregateVersion != aggregateVersion {
		return fmt.Errorf("dashboard task DLQ source tuple collision")
	}
	auditTenantID := tenantID
	if auditTenantID == "" {
		auditTenantID = "__unknown__"
	}
	objectID := taskID
	if objectID == "" {
		objectID = fmt.Sprintf("%s:%d:%d", message.Topic, message.Partition, message.Offset)
	}
	auditIdentity := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", message.Topic, message.Partition, message.Offset)))
	auditEventID := "audit-dashboard-task-dlq-" + hex.EncodeToString(auditIdentity[:])
	detail := map[string]interface{}{
		"dlq_topic": "dlq.v1", "source_topic": message.Topic,
		"source_partition": message.Partition, "source_offset": message.Offset,
		"event_id": eventID, "task_id": taskID, "aggregate_version": aggregateVersionDetail,
		"trace_id": traceID, "payload_sha256": payloadSHA, "headers_sha256": headerSHA,
		"error_code": "PROCESSING_FAILED", "error_message": errorMessage,
		"dlq_acknowledged": true, "source_offset_commit_pending": true,
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,error_message,created_at)
		VALUES($1,$2,NULL,'DASHBOARD_TASK_EVENT_QUARANTINED','dashboard_task_event',$3,$4::jsonb,$5,
		 'quarantined',true,$6,$7) ON CONFLICT(event_id) DO NOTHING`, auditEventID, auditTenantID,
		objectID, string(detailJSON), traceID, errorMessage, now); err != nil {
		return err
	}
	var storedAuditTenantID, storedObjectID, storedAuditPayloadSHA, storedAuditSourceTopic string
	var storedAuditPartition int
	var storedAuditOffset int64
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id,object_id,detail->>'payload_sha256',
		detail->>'source_topic',(detail->>'source_partition')::integer,(detail->>'source_offset')::bigint
		FROM audit_logs WHERE event_id=$1 FOR UPDATE`, auditEventID).Scan(&storedAuditTenantID, &storedObjectID,
		&storedAuditPayloadSHA, &storedAuditSourceTopic, &storedAuditPartition, &storedAuditOffset); err != nil {
		return err
	}
	if storedAuditTenantID != auditTenantID || storedObjectID != objectID || storedAuditPayloadSHA != payloadSHA ||
		storedAuditSourceTopic != message.Topic || storedAuditPartition != message.Partition || storedAuditOffset != message.Offset {
		return fmt.Errorf("dashboard task DLQ audit identity collision")
	}
	return tx.Commit()
}

func validateDashboardTaskLifecycleEnvelope(event dashboardTaskLifecycleEnvelope) error {
	if event.EventType != dashboardTaskRequestedEvent && event.EventType != dashboardTaskResultEvent &&
		event.EventType != dashboardTaskCompensationRequestedEvent && event.EventType != dashboardTaskCompensationResultEvent {
		return fmt.Errorf("unsupported dashboard task event_type")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid dashboard task event_id")
	}
	if _, err := uuid.Parse(event.TaskID); err != nil || event.AggregateID != event.TaskID {
		return fmt.Errorf("invalid dashboard task aggregate identity")
	}
	if event.SchemaVersion != 1 || event.AggregateType != "dashboard_task" || event.AggregateVersion <= 0 ||
		strings.TrimSpace(event.TenantID) == "" || event.PartitionKey != event.TenantID+":"+event.TaskID ||
		strings.TrimSpace(event.ActionID) == "" || strings.TrimSpace(event.SnapshotID) == "" ||
		strings.TrimSpace(event.TraceID) == "" {
		return fmt.Errorf("incomplete dashboard task event contract")
	}
	if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
		return fmt.Errorf("invalid dashboard task occurred_at")
	}
	if event.EventType == dashboardTaskRequestedEvent && event.Status != "accepted" {
		return fmt.Errorf("dashboard task request event must be accepted")
	}
	if event.EventType == dashboardTaskCompensationRequestedEvent && event.Status != "compensating" {
		return fmt.Errorf("dashboard task compensation request event must be compensating")
	}
	if event.EventType == dashboardTaskResultEvent {
		receipt := DashboardTaskExecutionReceipt{Status: event.Status, Provider: event.Provider,
			ProviderReceiptID: event.ProviderReceiptID, EffectState: event.EffectState,
			EffectIDs: event.EffectIDs, Result: event.Result, ErrorCode: event.ErrorCode,
			ErrorMessage: event.ErrorMessage, ExecutedAt: time.Now().UTC()}
		if err := validateDashboardTaskExecutionReceipt(receipt); err != nil {
			return fmt.Errorf("invalid dashboard task result event: %w", err)
		}
		if len(event.ReceiptSHA256) != 64 {
			return fmt.Errorf("invalid dashboard task receipt hash")
		}
	}
	if event.EventType == dashboardTaskCompensationResultEvent {
		receipt := DashboardTaskCompensationReceipt{Status: event.Status, Provider: event.Provider,
			ProviderReceiptID: event.ProviderReceiptID, EffectState: event.EffectState,
			CompensatedEffectIDs: event.EffectIDs, Result: event.Result, ErrorCode: event.ErrorCode,
			ErrorMessage: event.ErrorMessage, CompensatedAt: time.Now().UTC()}
		if err := validateDashboardTaskCompensationReceipt(receipt); err != nil {
			return fmt.Errorf("invalid dashboard task compensation result event: %w", err)
		}
		if len(event.ReceiptSHA256) != 64 {
			return fmt.Errorf("invalid dashboard task compensation receipt hash")
		}
	}
	return nil
}

func (pipeline *DashboardTaskPipeline) applyLifecycleEvent(ctx context.Context, event dashboardTaskLifecycleEnvelope, raw []byte, partition int, offset int64) error {
	digest := sha256.Sum256(raw)
	payloadSHA := hex.EncodeToString(digest[:])
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldTenant, oldTask, oldType, oldSHA, oldTopic string
	var oldVersion int64
	err = tx.QueryRowContext(ctx, `SELECT tenant_id,task_id::text,event_type,aggregate_version,payload_sha256,kafka_topic
		FROM dashboard_task_event_inbox WHERE event_id=$1`, event.EventID).
		Scan(&oldTenant, &oldTask, &oldType, &oldVersion, &oldSHA, &oldTopic)
	if err == nil {
		if oldTenant != event.TenantID || oldTask != event.TaskID || oldType != event.EventType ||
			oldVersion != event.AggregateVersion || oldSHA != payloadSHA || oldTopic != pipeline.topic {
			return fmt.Errorf("dashboard task event replay identity collision")
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var occupiedEvent string
	err = tx.QueryRowContext(ctx, `SELECT event_id::text FROM dashboard_task_event_inbox
		WHERE kafka_topic=$1 AND kafka_partition=$2 AND kafka_offset=$3`, pipeline.topic, partition, offset).Scan(&occupiedEvent)
	if err == nil {
		return fmt.Errorf("dashboard task Kafka position collision with %s", occupiedEvent)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var status, actionID, snapshotID, requestedBy, reason string
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT status,revision,action_id,snapshot_id,requested_by,reason
		FROM dashboard_tasks WHERE tenant_id=$1 AND task_id=$2 FOR UPDATE`, event.TenantID, event.TaskID).
		Scan(&status, &revision, &actionID, &snapshotID, &requestedBy, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("dashboard task authority is missing")
	}
	if err != nil {
		return err
	}
	expectedActionID := actionID
	if event.EventType == dashboardTaskCompensationRequestedEvent || event.EventType == dashboardTaskCompensationResultEvent {
		expectedActionID = dashboardTaskCompensationAction
	}
	if expectedActionID != event.ActionID || snapshotID != event.SnapshotID {
		return fmt.Errorf("dashboard task event differs from PostgreSQL authority")
	}
	switch event.EventType {
	case dashboardTaskRequestedEvent:
		if event.AggregateVersion != 1 {
			return fmt.Errorf("dashboard task request version must be 1")
		}
		if status == "accepted" {
			runningRevision := revision + 1
			runningEventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(event.EventID+":running")).String()
			now := time.Now().UTC()
			result, err := tx.ExecContext(ctx, `UPDATE dashboard_tasks SET status='running',revision=$3,
				started_at=COALESCE(started_at,$4),updated_at=$4 WHERE tenant_id=$1 AND task_id=$2 AND status='accepted' AND revision=$5`,
				event.TenantID, event.TaskID, runningRevision, now, revision)
			if err != nil {
				return err
			}
			affected, _ := result.RowsAffected()
			if affected != 1 {
				return fmt.Errorf("dashboard task running transition lost optimistic lock")
			}
			snapshotJSON, _ := json.Marshal(map[string]interface{}{"task_id": event.TaskID, "status": "running", "revision": runningRevision, "trace_id": event.TraceID})
			if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_history
				(event_id,tenant_id,task_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
				VALUES($1,$2,$3,$4,$5,'accepted','running',$6,$7,$8,$9::jsonb,$10)`, runningEventID,
				event.TenantID, event.TaskID, runningRevision, event.ActionID, requestedBy, reason, event.TraceID, string(snapshotJSON), now); err != nil {
				return err
			}
			if err := insertDashboardTaskPipelineAudit(ctx, tx, runningEventID, event.TenantID, requestedBy,
				"DASHBOARD_TASK_EXECUTION_STARTED", event.TaskID, event.TraceID, "running", snapshotJSON, now); err != nil {
				return err
			}
		}
		if status == "accepted" || status == "running" {
			_, err = tx.ExecContext(ctx, `INSERT INTO dashboard_task_execution_attempts
				(request_event_id,task_id,tenant_id,idempotency_key,status)
				VALUES($1,$2,$3,$4,'pending') ON CONFLICT (request_event_id) DO NOTHING`,
				event.EventID, event.TaskID, event.TenantID, "dashboard-task:"+event.EventID)
			if err != nil {
				return err
			}
		}
	case dashboardTaskResultEvent:
		if status != event.Status || revision != event.AggregateVersion ||
			(status != "completed" && status != "partial" && status != "failed") {
			return fmt.Errorf("dashboard task result event is not backed by terminal PostgreSQL authority")
		}
	case dashboardTaskCompensationRequestedEvent:
		if status != "compensating" || revision != event.AggregateVersion {
			return fmt.Errorf("dashboard task compensation request is not backed by PostgreSQL authority")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO dashboard_task_compensation_attempts
			(request_event_id,task_id,tenant_id,idempotency_key,status)
			VALUES($1,$2,$3,$4,'pending') ON CONFLICT (request_event_id) DO NOTHING`, event.EventID,
			event.TaskID, event.TenantID, "dashboard-task-compensation:"+event.EventID)
		if err != nil {
			return err
		}
	case dashboardTaskCompensationResultEvent:
		if status != event.Status || revision != event.AggregateVersion ||
			(status != "compensated" && status != "compensation_partial" && status != "compensation_failed") {
			return fmt.Errorf("dashboard task compensation result is not backed by terminal PostgreSQL authority")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_event_inbox
		(event_id,tenant_id,task_id,event_type,aggregate_version,payload_sha256,trace_id,kafka_topic,kafka_partition,kafka_offset)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, event.EventID, event.TenantID, event.TaskID,
		event.EventType, event.AggregateVersion, payloadSHA, event.TraceID, pipeline.topic, partition, offset); err != nil {
		return err
	}
	return tx.Commit()
}

func (pipeline *DashboardTaskPipeline) StartExecutionWorker(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := dashboardPipelineWorkerID("executor")
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := pipeline.DrainExecutions(ctx, workerID, 20); err != nil && ctx.Err() == nil && pipeline.logger != nil {
				pipeline.logger.Warn("Failed to drain dashboard task executions", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

type dashboardExecutionAttempt struct {
	RequestEventID string
	TaskID         string
	TenantID       string
	IdempotencyKey string
}

type dashboardTaskProviderAuthorityResolution struct {
	Attempted        bool   `json:"attempted"`
	State            string `json:"state"`
	Provider         string `json:"provider,omitempty"`
	CheckedAt        string `json:"checked_at,omitempty"`
	RecoveredReceipt bool   `json:"recovered_receipt"`
	ErrorCode        string `json:"error_code,omitempty"`
}

func (pipeline *DashboardTaskPipeline) DrainExecutions(ctx context.Context, workerID string, limit int) (int, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := pipeline.db.QueryContext(ctx, `WITH candidates AS (
		SELECT request_event_id FROM dashboard_task_execution_attempts WHERE available_at<=now()
		  AND (status='pending' OR (status='processing' AND locked_until<now()))
		ORDER BY available_at,created_at,request_event_id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE dashboard_task_execution_attempts a SET status='processing',attempt_count=attempt_count+1,
		  locked_until=now()+interval '5 minutes',locked_by=$2 FROM candidates c
		WHERE a.request_event_id=c.request_event_id
		RETURNING a.request_event_id::text,a.task_id::text,a.tenant_id,a.idempotency_key
	) SELECT request_event_id,task_id,tenant_id,idempotency_key FROM claimed ORDER BY request_event_id`, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	attempts := make([]dashboardExecutionAttempt, 0, limit)
	for rows.Next() {
		var attempt dashboardExecutionAttempt
		if err := rows.Scan(&attempt.RequestEventID, &attempt.TaskID, &attempt.TenantID, &attempt.IdempotencyKey); err != nil {
			return len(attempts), err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return len(attempts), err
	}
	completed := 0
	for _, attempt := range attempts {
		command, err := pipeline.loadExecutionCommand(ctx, attempt)
		if err != nil {
			pipeline.releaseExecution(ctx, workerID, attempt.RequestEventID, err.Error())
			continue
		}
		receipt, executeErr := pipeline.executor.ExecuteDashboardTask(ctx, command)
		var authorityResolution *dashboardTaskProviderAuthorityResolution
		if executeErr != nil {
			var recovered bool
			receipt, authorityResolution, recovered = pipeline.reconcileExecutionAuthority(ctx, command)
			if !recovered {
				receipt = DashboardTaskExecutionReceipt{
					Status: "partial", Provider: "dashboard-task-http-executor",
					ProviderReceiptID: "transport-unknown-" + attempt.RequestEventID,
					EffectState:       "unknown", EffectIDs: []string{}, Result: map[string]interface{}{},
					ErrorCode: "EXECUTOR_TRANSPORT_UNKNOWN", ErrorMessage: truncateDashboardPipelineError(executeErr.Error()),
					ExecutedAt: time.Now().UTC(),
				}
			}
		}
		if err := pipeline.commitExecutionReceipt(ctx, workerID, attempt, receipt, authorityResolution); err != nil {
			pipeline.releaseExecution(ctx, workerID, attempt.RequestEventID, err.Error())
			continue
		}
		completed++
	}
	return completed, nil
}

func (pipeline *DashboardTaskPipeline) reconcileExecutionAuthority(ctx context.Context, command DashboardTaskExecutionRequest) (DashboardTaskExecutionReceipt, *dashboardTaskProviderAuthorityResolution, bool) {
	authority, ok := pipeline.executor.(DashboardTaskExecutionAuthority)
	if !ok {
		return DashboardTaskExecutionReceipt{}, nil, false
	}
	lookup, err := authority.LookupDashboardTaskExecution(ctx, command)
	if errors.Is(err, errDashboardTaskAuthorityLookupNotConfigured) {
		return DashboardTaskExecutionReceipt{}, nil, false
	}
	resolution := &dashboardTaskProviderAuthorityResolution{Attempted: true, State: "unknown", ErrorCode: "EXECUTOR_AUTHORITY_LOOKUP_FAILED"}
	if err != nil {
		return DashboardTaskExecutionReceipt{}, resolution, false
	}
	lookup = normalizeDashboardTaskExecutionAuthorityLookup(lookup)
	if err := validateDashboardTaskExecutionAuthorityLookup(command, lookup); err != nil {
		return DashboardTaskExecutionReceipt{}, resolution, false
	}
	resolution.State = lookup.State
	resolution.Provider = lookup.Provider
	resolution.CheckedAt = lookup.CheckedAt.UTC().Format(time.RFC3339Nano)
	resolution.ErrorCode = ""
	if lookup.State != "receipt_found" || lookup.Receipt == nil {
		return DashboardTaskExecutionReceipt{}, resolution, false
	}
	resolution.RecoveredReceipt = true
	return *lookup.Receipt, resolution, true
}

func (pipeline *DashboardTaskPipeline) loadExecutionCommand(ctx context.Context, attempt dashboardExecutionAttempt) (DashboardTaskExecutionRequest, error) {
	var command DashboardTaskExecutionRequest
	var input []byte
	var status string
	err := pipeline.db.QueryRowContext(ctx, `SELECT action_id,task_type,target,priority,snapshot_id,reason,requested_by,trace_id,input,status
		FROM dashboard_tasks WHERE tenant_id=$1 AND task_id=$2`, attempt.TenantID, attempt.TaskID).
		Scan(&command.ActionID, &command.TaskType, &command.Target, &command.Priority, &command.SnapshotID,
			&command.Reason, &command.RequestedBy, &command.TraceID, &input, &status)
	if err != nil {
		return command, err
	}
	if status != "running" {
		return command, fmt.Errorf("dashboard task is not in running state")
	}
	if err := json.Unmarshal(input, &command.Context); err != nil {
		return command, err
	}
	command.RequestEventID = attempt.RequestEventID
	command.TenantID = attempt.TenantID
	command.TaskID = attempt.TaskID
	command.IdempotencyKey = attempt.IdempotencyKey
	return command, nil
}

func (pipeline *DashboardTaskPipeline) commitExecutionReceipt(ctx context.Context, workerID string, attempt dashboardExecutionAttempt, receipt DashboardTaskExecutionReceipt, authorityResolution *dashboardTaskProviderAuthorityResolution) error {
	receipt = normalizeDashboardTaskExecutionReceipt(receipt)
	if err := validateDashboardTaskExecutionReceipt(receipt); err != nil {
		return err
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(receiptJSON)
	receiptSHA := hex.EncodeToString(digest[:])
	effectJSON, _ := json.Marshal(receipt.EffectIDs)
	resultJSON, _ := json.Marshal(receipt.Result)
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attemptStatus, lockedBy string
	err = tx.QueryRowContext(ctx, `SELECT status,locked_by FROM dashboard_task_execution_attempts
		WHERE request_event_id=$1 FOR UPDATE`, attempt.RequestEventID).Scan(&attemptStatus, &lockedBy)
	if err != nil {
		return err
	}
	if attemptStatus == "completed" {
		return tx.Commit()
	}
	if attemptStatus != "processing" || lockedBy != workerID {
		return fmt.Errorf("dashboard execution lease lost")
	}
	var status, actionID, snapshotID, requestedBy, reason, traceID string
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT status,revision,action_id,snapshot_id,requested_by,reason,trace_id
		FROM dashboard_tasks WHERE tenant_id=$1 AND task_id=$2 FOR UPDATE`, attempt.TenantID, attempt.TaskID).
		Scan(&status, &revision, &actionID, &snapshotID, &requestedBy, &reason, &traceID)
	if err != nil {
		return err
	}
	if status == "completed" || status == "partial" || status == "failed" {
		_, err = tx.ExecContext(ctx, `UPDATE dashboard_task_execution_attempts SET status='completed',
			completed_at=COALESCE(completed_at,now()),locked_until=NULL,locked_by='',last_error=''
			WHERE request_event_id=$1`, attempt.RequestEventID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if status != "running" {
		return fmt.Errorf("dashboard task cannot transition from %s to %s", status, receipt.Status)
	}
	resultEventID := uuid.NewString()
	terminalRevision := revision + 1
	now := time.Now().UTC()
	terminalResult := map[string]interface{}{
		"provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
		"effect_state": receipt.EffectState, "effect_ids": receipt.EffectIDs, "result": receipt.Result,
		"receipt_sha256": receiptSHA,
	}
	if authorityResolution != nil {
		terminalResult["authority_lookup"] = authorityResolution
	}
	terminalResultJSON, _ := json.Marshal(terminalResult)
	result, err := tx.ExecContext(ctx, `UPDATE dashboard_tasks SET status=$3,revision=$4,result=$5::jsonb,
		error_code=$6,error_message=$7,completed_at=$8,updated_at=$8
		WHERE tenant_id=$1 AND task_id=$2 AND status='running' AND revision=$9`, attempt.TenantID, attempt.TaskID,
		receipt.Status, terminalRevision, string(terminalResultJSON), receipt.ErrorCode, receipt.ErrorMessage, now, revision)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return fmt.Errorf("dashboard task terminal transition lost optimistic lock")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_execution_receipts
		(task_id,tenant_id,request_event_id,provider,provider_receipt_id,status,effect_state,effect_ids,
		 result,error_code,error_message,receipt_sha256,trace_id,executed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14)`, attempt.TaskID,
		attempt.TenantID, attempt.RequestEventID, receipt.Provider, receipt.ProviderReceiptID, receipt.Status,
		receipt.EffectState, string(effectJSON), string(resultJSON), receipt.ErrorCode, receipt.ErrorMessage,
		receiptSHA, traceID, receipt.ExecutedAt); err != nil {
		return err
	}
	historySnapshotValue := map[string]interface{}{"task_id": attempt.TaskID, "status": receipt.Status,
		"revision": terminalRevision, "provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
		"effect_state": receipt.EffectState, "effect_ids": receipt.EffectIDs, "receipt_sha256": receiptSHA,
		"trace_id": traceID}
	if authorityResolution != nil {
		historySnapshotValue["authority_lookup"] = authorityResolution
	}
	historySnapshot, _ := json.Marshal(historySnapshotValue)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_history
		(event_id,tenant_id,task_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,$4,$5,'running',$6,$7,$8,$9,$10::jsonb,$11)`, resultEventID, attempt.TenantID,
		attempt.TaskID, terminalRevision, actionID, receipt.Status, requestedBy, reason, traceID, string(historySnapshot), now); err != nil {
		return err
	}
	if err := insertDashboardTaskPipelineAudit(ctx, tx, resultEventID, attempt.TenantID, requestedBy,
		"DASHBOARD_TASK_EXECUTION_"+strings.ToUpper(receipt.Status), attempt.TaskID, traceID, receipt.Status, historySnapshot, now); err != nil {
		return err
	}
	partitionKey := attempt.TenantID + ":" + attempt.TaskID
	eventPayload := dashboardTaskLifecycleEnvelope{
		EventID: resultEventID, EventType: dashboardTaskResultEvent, SchemaVersion: 1,
		AggregateType: "dashboard_task", AggregateID: attempt.TaskID, AggregateVersion: terminalRevision,
		PartitionKey: partitionKey, TenantID: attempt.TenantID, TaskID: attempt.TaskID, ActionID: actionID,
		Status: receipt.Status, SnapshotID: snapshotID, Provider: receipt.Provider,
		ProviderReceiptID: receipt.ProviderReceiptID, EffectState: receipt.EffectState, EffectIDs: receipt.EffectIDs,
		Result: receipt.Result, ErrorCode: receipt.ErrorCode, ErrorMessage: receipt.ErrorMessage,
		ReceiptSHA256: receiptSHA, TraceID: traceID, OccurredAt: now.Format(time.RFC3339Nano),
	}
	eventJSON, _ := json.Marshal(eventPayload)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_outbox
		(event_id,tenant_id,task_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,status,occurred_at)
		VALUES($1,$2,$3,$4,$5,1,$6,$7::jsonb,$8,'pending',$9)`, resultEventID, attempt.TenantID,
		attempt.TaskID, terminalRevision, dashboardTaskResultEvent, partitionKey, string(eventJSON), traceID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dashboard_task_execution_attempts SET status='completed',completed_at=$2,
		locked_until=NULL,locked_by='',last_error='' WHERE request_event_id=$1 AND locked_by=$3`,
		attempt.RequestEventID, now, workerID); err != nil {
		return err
	}
	return tx.Commit()
}

func (pipeline *DashboardTaskPipeline) releaseExecution(ctx context.Context, workerID, requestEventID, message string) {
	_, _ = pipeline.db.ExecContext(ctx, `UPDATE dashboard_task_execution_attempts SET status='pending',
		available_at=now()+(LEAST(300,POWER(2,LEAST(attempt_count,8)))::text||' seconds')::interval,
		locked_until=NULL,locked_by='',last_error=$3 WHERE request_event_id=$1 AND status='processing' AND locked_by=$2`,
		requestEventID, workerID, truncateDashboardPipelineError(message))
}

func (pipeline *DashboardTaskPipeline) StartCompensationWorker(ctx context.Context, interval time.Duration) error {
	if pipeline.compensator == nil {
		return fmt.Errorf("dashboard task compensator is not configured")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := dashboardPipelineWorkerID("compensator")
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := pipeline.DrainCompensations(ctx, workerID, 20); err != nil && ctx.Err() == nil && pipeline.logger != nil {
				pipeline.logger.Warn("Failed to drain dashboard task compensations", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

type dashboardCompensationAttempt struct {
	RequestEventID string
	TaskID         string
	TenantID       string
	IdempotencyKey string
}

func (pipeline *DashboardTaskPipeline) DrainCompensations(ctx context.Context, workerID string, limit int) (int, error) {
	if pipeline.compensator == nil {
		return 0, fmt.Errorf("dashboard task compensator is not configured")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := pipeline.db.QueryContext(ctx, `WITH candidates AS (
		SELECT request_event_id FROM dashboard_task_compensation_attempts WHERE available_at<=now()
		  AND (status='pending' OR (status='processing' AND locked_until<now()))
		ORDER BY available_at,created_at,request_event_id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE dashboard_task_compensation_attempts a SET status='processing',attempt_count=attempt_count+1,
		  locked_until=now()+interval '5 minutes',locked_by=$2 FROM candidates c
		WHERE a.request_event_id=c.request_event_id
		RETURNING a.request_event_id::text,a.task_id::text,a.tenant_id,a.idempotency_key
	) SELECT request_event_id,task_id,tenant_id,idempotency_key FROM claimed ORDER BY request_event_id`, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	attempts := make([]dashboardCompensationAttempt, 0, limit)
	for rows.Next() {
		var attempt dashboardCompensationAttempt
		if err := rows.Scan(&attempt.RequestEventID, &attempt.TaskID, &attempt.TenantID, &attempt.IdempotencyKey); err != nil {
			return len(attempts), err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return len(attempts), err
	}
	completed := 0
	for _, attempt := range attempts {
		command, err := pipeline.loadCompensationCommand(ctx, attempt)
		if err != nil {
			pipeline.releaseCompensation(ctx, workerID, attempt.RequestEventID, err.Error())
			continue
		}
		receipt, compensateErr := pipeline.compensator.CompensateDashboardTask(ctx, command)
		var authorityResolution *dashboardTaskProviderAuthorityResolution
		if compensateErr != nil {
			var recovered bool
			receipt, authorityResolution, recovered = pipeline.reconcileCompensationAuthority(ctx, command)
			if !recovered {
				receipt = DashboardTaskCompensationReceipt{
					Status: "compensation_partial", Provider: "dashboard-task-http-compensator",
					ProviderReceiptID: "transport-unknown-" + attempt.RequestEventID, EffectState: "unknown",
					CompensatedEffectIDs: []string{}, Result: map[string]interface{}{},
					ErrorCode: "COMPENSATOR_TRANSPORT_UNKNOWN", ErrorMessage: truncateDashboardPipelineError(compensateErr.Error()),
					CompensatedAt: time.Now().UTC(),
				}
			}
		}
		if err := pipeline.commitCompensationReceipt(ctx, workerID, attempt, receipt, authorityResolution); err != nil {
			pipeline.releaseCompensation(ctx, workerID, attempt.RequestEventID, err.Error())
			continue
		}
		completed++
	}
	return completed, nil
}

func (pipeline *DashboardTaskPipeline) reconcileCompensationAuthority(ctx context.Context, command DashboardTaskCompensationRequest) (DashboardTaskCompensationReceipt, *dashboardTaskProviderAuthorityResolution, bool) {
	authority, ok := pipeline.compensator.(DashboardTaskCompensationAuthority)
	if !ok {
		return DashboardTaskCompensationReceipt{}, nil, false
	}
	lookup, err := authority.LookupDashboardTaskCompensation(ctx, command)
	if errors.Is(err, errDashboardTaskAuthorityLookupNotConfigured) {
		return DashboardTaskCompensationReceipt{}, nil, false
	}
	resolution := &dashboardTaskProviderAuthorityResolution{Attempted: true, State: "unknown", ErrorCode: "COMPENSATOR_AUTHORITY_LOOKUP_FAILED"}
	if err != nil {
		return DashboardTaskCompensationReceipt{}, resolution, false
	}
	lookup = normalizeDashboardTaskCompensationAuthorityLookup(lookup)
	if err := validateDashboardTaskCompensationAuthorityLookup(command, lookup); err != nil {
		return DashboardTaskCompensationReceipt{}, resolution, false
	}
	resolution.State = lookup.State
	resolution.Provider = lookup.Provider
	resolution.CheckedAt = lookup.CheckedAt.UTC().Format(time.RFC3339Nano)
	resolution.ErrorCode = ""
	if lookup.State != "receipt_found" || lookup.Receipt == nil {
		return DashboardTaskCompensationReceipt{}, resolution, false
	}
	resolution.RecoveredReceipt = true
	return *lookup.Receipt, resolution, true
}

func (pipeline *DashboardTaskPipeline) loadCompensationCommand(ctx context.Context, attempt dashboardCompensationAttempt) (DashboardTaskCompensationRequest, error) {
	var command DashboardTaskCompensationRequest
	var effectIDsJSON, originalResultJSON []byte
	var status, effectState string
	err := pipeline.db.QueryRowContext(ctx, `SELECT t.snapshot_id,c.reason,c.requested_by,c.trace_id,
		r.provider,r.provider_receipt_id,r.effect_state,r.effect_ids,r.result,t.status
		FROM dashboard_task_compensation_requests c
		JOIN dashboard_tasks t ON t.task_id=c.task_id AND t.tenant_id=c.tenant_id
		JOIN dashboard_task_execution_receipts r ON r.task_id=c.task_id AND r.tenant_id=c.tenant_id
		WHERE c.tenant_id=$1 AND c.task_id=$2 AND c.request_event_id=$3`, attempt.TenantID, attempt.TaskID, attempt.RequestEventID).
		Scan(&command.SnapshotID, &command.Reason, &command.RequestedBy, &command.TraceID,
			&command.OriginalProvider, &command.OriginalReceiptID, &effectState, &effectIDsJSON, &originalResultJSON, &status)
	if err != nil {
		return command, err
	}
	if status != "compensating" || effectState != "confirmed" {
		return command, fmt.Errorf("dashboard task is not in a confirmed compensating state")
	}
	if err := json.Unmarshal(effectIDsJSON, &command.OriginalEffectIDs); err != nil || len(command.OriginalEffectIDs) == 0 {
		return command, fmt.Errorf("dashboard task original effect identities are invalid")
	}
	if err := json.Unmarshal(originalResultJSON, &command.OriginalResult); err != nil {
		return command, err
	}
	command.RequestEventID = attempt.RequestEventID
	command.TenantID = attempt.TenantID
	command.TaskID = attempt.TaskID
	command.ActionID = dashboardTaskCompensationAction
	command.CompensationIdempotency = attempt.IdempotencyKey
	return command, nil
}

func (pipeline *DashboardTaskPipeline) commitCompensationReceipt(ctx context.Context, workerID string, attempt dashboardCompensationAttempt, receipt DashboardTaskCompensationReceipt, authorityResolution *dashboardTaskProviderAuthorityResolution) error {
	receipt = normalizeDashboardTaskCompensationReceipt(receipt)
	if err := validateDashboardTaskCompensationReceipt(receipt); err != nil {
		return err
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(receiptJSON)
	receiptSHA := hex.EncodeToString(digest[:])
	effectJSON, _ := json.Marshal(receipt.CompensatedEffectIDs)
	resultJSON, _ := json.Marshal(receipt.Result)
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attemptStatus, lockedBy string
	if err := tx.QueryRowContext(ctx, `SELECT status,locked_by FROM dashboard_task_compensation_attempts
		WHERE request_event_id=$1 FOR UPDATE`, attempt.RequestEventID).Scan(&attemptStatus, &lockedBy); err != nil {
		return err
	}
	if attemptStatus == "completed" {
		return tx.Commit()
	}
	if attemptStatus != "processing" || lockedBy != workerID {
		return fmt.Errorf("dashboard compensation lease lost")
	}
	var status, snapshotID, requestedBy, reason, traceID string
	var revision int64
	var originalResultJSON []byte
	if err := tx.QueryRowContext(ctx, `SELECT t.status,t.revision,t.snapshot_id,c.requested_by,c.reason,c.trace_id,t.result
		FROM dashboard_tasks t JOIN dashboard_task_compensation_requests c
		ON c.task_id=t.task_id AND c.tenant_id=t.tenant_id
		WHERE t.tenant_id=$1 AND t.task_id=$2 FOR UPDATE`, attempt.TenantID, attempt.TaskID).
		Scan(&status, &revision, &snapshotID, &requestedBy, &reason, &traceID, &originalResultJSON); err != nil {
		return err
	}
	if status == "compensated" || status == "compensation_partial" || status == "compensation_failed" {
		_, err = tx.ExecContext(ctx, `UPDATE dashboard_task_compensation_attempts SET status='completed',
			completed_at=COALESCE(completed_at,now()),locked_until=NULL,locked_by='',last_error=''
			WHERE request_event_id=$1`, attempt.RequestEventID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if status != "compensating" {
		return fmt.Errorf("dashboard task cannot transition from %s to %s", status, receipt.Status)
	}
	now := time.Now().UTC()
	resultEventID := uuid.NewString()
	nextRevision := revision + 1
	var originalResult map[string]interface{}
	if err := json.Unmarshal(originalResultJSON, &originalResult); err != nil {
		return err
	}
	compensationResult := map[string]interface{}{
		"provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
		"effect_state": receipt.EffectState, "compensated_effect_ids": receipt.CompensatedEffectIDs,
		"result": receipt.Result, "receipt_sha256": receiptSHA,
	}
	if authorityResolution != nil {
		compensationResult["authority_lookup"] = authorityResolution
	}
	terminalResult := map[string]interface{}{"execution": originalResult, "compensation": compensationResult}
	terminalResultJSON, _ := json.Marshal(terminalResult)
	result, err := tx.ExecContext(ctx, `UPDATE dashboard_tasks SET status=$3,revision=$4,result=$5::jsonb,
		error_code=$6,error_message=$7,updated_at=$8,completed_at=$8,
		cancelled_at=CASE WHEN $3='compensated' THEN $8 ELSE cancelled_at END
		WHERE tenant_id=$1 AND task_id=$2 AND status='compensating' AND revision=$9`, attempt.TenantID,
		attempt.TaskID, receipt.Status, nextRevision, string(terminalResultJSON), receipt.ErrorCode, receipt.ErrorMessage, now, revision)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("dashboard compensation transition lost optimistic lock")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_compensation_receipts
		(task_id,tenant_id,request_event_id,provider,provider_receipt_id,status,effect_state,compensated_effect_ids,
		result,error_code,error_message,receipt_sha256,trace_id,compensated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14)`, attempt.TaskID, attempt.TenantID,
		attempt.RequestEventID, receipt.Provider, receipt.ProviderReceiptID, receipt.Status, receipt.EffectState,
		string(effectJSON), string(resultJSON), receipt.ErrorCode, receipt.ErrorMessage, receiptSHA, traceID, receipt.CompensatedAt); err != nil {
		return err
	}
	historySnapshotValue := map[string]interface{}{"task_id": attempt.TaskID, "status": receipt.Status,
		"revision": nextRevision, "provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
		"effect_state": receipt.EffectState, "compensated_effect_ids": receipt.CompensatedEffectIDs,
		"receipt_sha256": receiptSHA, "trace_id": traceID}
	if authorityResolution != nil {
		historySnapshotValue["authority_lookup"] = authorityResolution
	}
	historySnapshot, _ := json.Marshal(historySnapshotValue)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_history
		(event_id,tenant_id,task_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,$4,$5,'compensating',$6,$7,$8,$9,$10::jsonb,$11)`, resultEventID, attempt.TenantID,
		attempt.TaskID, nextRevision, dashboardTaskCompensationAction, receipt.Status, requestedBy, reason, traceID,
		string(historySnapshot), now); err != nil {
		return err
	}
	if err := insertDashboardTaskPipelineAudit(ctx, tx, resultEventID, attempt.TenantID, requestedBy,
		"DASHBOARD_TASK_COMPENSATION_"+strings.ToUpper(receipt.Status), attempt.TaskID, traceID, receipt.Status, historySnapshot, now); err != nil {
		return err
	}
	eventPayload := dashboardTaskLifecycleEnvelope{EventID: resultEventID, EventType: dashboardTaskCompensationResultEvent,
		SchemaVersion: 1, AggregateType: "dashboard_task", AggregateID: attempt.TaskID, AggregateVersion: nextRevision,
		PartitionKey: attempt.TenantID + ":" + attempt.TaskID, TenantID: attempt.TenantID, TaskID: attempt.TaskID,
		ActionID: dashboardTaskCompensationAction, Status: receipt.Status, SnapshotID: snapshotID, Provider: receipt.Provider,
		ProviderReceiptID: receipt.ProviderReceiptID, EffectState: receipt.EffectState, EffectIDs: receipt.CompensatedEffectIDs,
		Result: receipt.Result, ErrorCode: receipt.ErrorCode, ErrorMessage: receipt.ErrorMessage, ReceiptSHA256: receiptSHA,
		TraceID: traceID, OccurredAt: now.Format(time.RFC3339Nano)}
	eventJSON, _ := json.Marshal(eventPayload)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_outbox
		(event_id,tenant_id,task_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,status,occurred_at)
		VALUES($1,$2,$3,$4,$5,1,$6,$7::jsonb,$8,'pending',$9)`, resultEventID, attempt.TenantID, attempt.TaskID,
		nextRevision, dashboardTaskCompensationResultEvent, attempt.TenantID+":"+attempt.TaskID, string(eventJSON), traceID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dashboard_task_compensation_attempts SET status='completed',completed_at=$2,
		locked_until=NULL,locked_by='',last_error='' WHERE request_event_id=$1 AND locked_by=$3`, attempt.RequestEventID, now, workerID); err != nil {
		return err
	}
	return tx.Commit()
}

func (pipeline *DashboardTaskPipeline) releaseCompensation(ctx context.Context, workerID, requestEventID, message string) {
	_, _ = pipeline.db.ExecContext(ctx, `UPDATE dashboard_task_compensation_attempts SET status='pending',
		available_at=now()+(LEAST(300,POWER(2,LEAST(attempt_count,8)))::text||' seconds')::interval,
		locked_until=NULL,locked_by='',last_error=$3 WHERE request_event_id=$1 AND status='processing' AND locked_by=$2`,
		requestEventID, workerID, truncateDashboardPipelineError(message))
}

func insertDashboardTaskPipelineAudit(ctx context.Context, tx *sql.Tx, eventID, tenantID, actorID, action, taskID, traceID, result string, detail []byte, occurredAt time.Time) error {
	var dataType string
	err := tx.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='audit_logs' AND column_name='user_id'`).Scan(&dataType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	userExpression := "$3"
	if dataType == "uuid" {
		userExpression = "NULLIF($3,'')::uuid"
		if _, err := uuid.Parse(actorID); err != nil {
			actorID = ""
		}
	}
	query := `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
		VALUES($1,$2,` + userExpression + `,$4,'dashboard_task',$5,$6::jsonb,$7,$8,$9,$10)`
	_, err = tx.ExecContext(ctx, query, "audit-"+eventID, tenantID, actorID, action, taskID,
		string(detail), traceID, result, result != "failed", occurredAt)
	return err
}

type DashboardTaskEventConsumer struct {
	consumer *commonkafka.Consumer
	pipeline *DashboardTaskPipeline
}

func NewDashboardTaskEventConsumer(consumer *commonkafka.Consumer, pipeline *DashboardTaskPipeline) (*DashboardTaskEventConsumer, error) {
	if consumer == nil || pipeline == nil {
		return nil, fmt.Errorf("dashboard task Kafka consumer and pipeline are required")
	}
	return &DashboardTaskEventConsumer{consumer: consumer, pipeline: pipeline}, nil
}

func (consumer *DashboardTaskEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.pipeline.HandleKafkaMessage)
}

func (consumer *DashboardTaskEventConsumer) Close() error { return consumer.consumer.Close() }

func dashboardPipelineWorkerID(kind string) string {
	return "dashboard-" + kind + ":" + uuid.NewString()
}

func truncateDashboardPipelineError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	return message
}

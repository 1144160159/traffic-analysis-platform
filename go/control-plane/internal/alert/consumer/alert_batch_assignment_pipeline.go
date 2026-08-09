package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	alertstate "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/state"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	AlertAssignmentEventTopic            = "alert.assignment.events.v1"
	alertBatchRequestedEvent             = "alert.batch-assignment.requested.v1"
	alertAssignmentChangedEvent          = "alert.assignment.changed.v1"
	alertBatchCompensationRequestedEvent = "alert.batch-assignment.compensation-requested.v1"
	alertAssignmentCompensatedEvent      = "alert.assignment.compensated.v1"
	alertBatchOutboxMaxAttempts          = 10
)

var errAlertBatchPermanent = errors.New("permanent alert batch assignment event failure")

type AlertBatchAssignmentPublishFunc func(context.Context, string, []byte, ...commonkafka.MessageHeader) error

type AlertBatchAssignmentAuthority interface {
	GetAlert(context.Context, string, string) (*service.AlertDetailDTO, error)
	ProjectAlertAssignment(context.Context, string, string, string, string, uint64, uint64) (*service.AlertAssignmentProjectionResult, error)
	ProjectAlertAssignmentCompensation(context.Context, string, string, string, string, string, uint64, uint64) (*service.AlertAssignmentProjectionResult, error)
}

type AlertBatchAssignmentPipeline struct {
	db        *sql.DB
	authority AlertBatchAssignmentAuthority
	publish   AlertBatchAssignmentPublishFunc
	topic     string
	logger    *zap.Logger
	now       func() time.Time
}

func NewAlertBatchAssignmentPipeline(
	db *sql.DB,
	authority AlertBatchAssignmentAuthority,
	publish AlertBatchAssignmentPublishFunc,
	topic string,
	logger *zap.Logger,
) (*AlertBatchAssignmentPipeline, error) {
	if db == nil || authority == nil || publish == nil {
		return nil, fmt.Errorf("alert batch assignment database, authority and publisher are required")
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = AlertAssignmentEventTopic
	}
	if topic != AlertAssignmentEventTopic {
		return nil, fmt.Errorf("alert batch assignment topic must be %s", AlertAssignmentEventTopic)
	}
	return &AlertBatchAssignmentPipeline{db: db, authority: authority, publish: publish, topic: topic, logger: logger, now: time.Now}, nil
}

func (pipeline *AlertBatchAssignmentPipeline) VerifySchema(ctx context.Context) error {
	var tables int
	if err := pipeline.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema=current_schema() AND table_name IN
		('alert_assignment_states','alert_assignment_state_history','alert_assignment_batch_inbox',
		 'alert_assignment_projection_receipts','alert_assignment_batch_dlq_receipts',
		 'alert_assignment_compensation_requests','alert_assignment_compensation_items',
		 'alert_assignment_compensation_history','alert_assignment_compensation_item_history',
		 'alert_assignment_compensation_projection_receipts')`).Scan(&tables); err != nil {
		return err
	}
	if tables != 10 {
		return fmt.Errorf("alert batch assignment execution schema is incomplete: tables=%d want=10", tables)
	}
	var migrationApplied bool
	if err := pipeline.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM alignment_schema_migrations WHERE version='202608092130'
	) AND EXISTS(
		SELECT 1 FROM alignment_schema_migrations WHERE version='202608092300'
	)`).Scan(&migrationApplied); err != nil {
		return err
	}
	if !migrationApplied {
		return fmt.Errorf("alert batch assignment execution migrations 202608092130 and 202608092300 are not registered")
	}
	var requiredColumns int
	if err := pipeline.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
			(table_name='alert_assignment_states' AND column_name IN
			 ('previous_state_version','previous_assignee','previous_status','status','projection_status','last_error','source_event_id')) OR
			(table_name='alert_assignment_batch_inbox' AND column_name IN
			 ('source_topic','source_partition','source_offset','payload_sha256','headers_sha256','aggregate_type','aggregate_id')) OR
			(table_name='alert_assignment_projection_receipts' AND column_name IN
			 ('source_topic','source_partition','source_offset','outcome','previous_status','resulting_status')) OR
			(table_name='alert_assignment_batch_dlq_receipts' AND column_name IN
			 ('dlq_topic','payload_sha256','headers_sha256')) OR
			(table_name='alert_assignment_batch_items' AND column_name IN
			 ('previous_status','resulting_status')) OR
			(table_name='alert_assignment_batch_outbox' AND column_name IN
			 ('aggregate_type','aggregate_id'))
		)`).Scan(&requiredColumns); err != nil {
		return err
	}
	if requiredColumns != 27 {
		return fmt.Errorf("alert batch assignment execution schema is incomplete: required_columns=%d want=27", requiredColumns)
	}
	return nil
}

type alertBatchOutboxItem struct {
	OutboxID         int64
	EventID          string
	TenantID         string
	BatchID          string
	AggregateVersion int64
	AggregateType    string
	AggregateID      string
	EventType        string
	SchemaVersion    int
	PartitionKey     string
	Payload          []byte
	TraceID          string
}

func (pipeline *AlertBatchAssignmentPipeline) StartOutboxWorker(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := "alert-batch-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := pipeline.DrainOutbox(ctx, workerID, 50); err != nil && ctx.Err() == nil && pipeline.logger != nil {
				pipeline.logger.Warn("Failed to drain alert batch assignment outbox", zap.Error(err))
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

func (pipeline *AlertBatchAssignmentPipeline) DrainOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := pipeline.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM alert_assignment_batch_outbox
		WHERE status IN ('pending','processing') AND publish_attempts < $3
		  AND next_retry_at<=now() AND (locked_until IS NULL OR locked_until<now())
		ORDER BY next_retry_at,occurred_at,outbox_id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE alert_assignment_batch_outbox o SET status='processing',locked_until=now()+interval '60 seconds',locked_by=$2
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.event_id::text,o.tenant_id,o.batch_id::text,o.aggregate_version,
		 o.aggregate_type,o.aggregate_id,o.event_type,o.schema_version,o.partition_key,o.payload::text,o.trace_id
		) SELECT outbox_id,event_id,tenant_id,batch_id,aggregate_version,aggregate_type,
			aggregate_id,event_type,schema_version,partition_key,payload,trace_id FROM claimed ORDER BY outbox_id`, limit, workerID, alertBatchOutboxMaxAttempts)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]alertBatchOutboxItem, 0, limit)
	for rows.Next() {
		var item alertBatchOutboxItem
		var payload string
		if err := rows.Scan(&item.OutboxID, &item.EventID, &item.TenantID, &item.BatchID,
			&item.AggregateVersion, &item.AggregateType, &item.AggregateID, &item.EventType, &item.SchemaVersion, &item.PartitionKey,
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
		if err := pipeline.publishOutboxItem(ctx, workerID, item); err != nil {
			if pipeline.logger != nil {
				pipeline.logger.Warn("Alert batch assignment outbox delivery failed", zap.String("event_id", item.EventID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (pipeline *AlertBatchAssignmentPipeline) publishOutboxItem(ctx context.Context, workerID string, item alertBatchOutboxItem) error {
	if !json.Valid(item.Payload) || item.SchemaVersion != 1 || item.AggregateVersion <= 0 ||
		strings.TrimSpace(item.AggregateType) == "" || strings.TrimSpace(item.AggregateID) == "" ||
		(item.EventType != alertBatchRequestedEvent && item.EventType != alertAssignmentChangedEvent &&
			item.EventType != alertBatchCompensationRequestedEvent && item.EventType != alertAssignmentCompensatedEvent) {
		err := fmt.Errorf("invalid alert batch assignment outbox envelope")
		pipeline.releaseOutbox(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	if err := pipeline.publish(ctx, item.PartitionKey, item.Payload,
		commonkafka.MessageHeader{Key: "event_id", Value: item.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: item.EventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: "1"},
		commonkafka.MessageHeader{Key: "aggregate_type", Value: item.AggregateType},
		commonkafka.MessageHeader{Key: "aggregate_id", Value: item.AggregateID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: strconv.FormatInt(item.AggregateVersion, 10)},
		commonkafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		commonkafka.MessageHeader{Key: "batch_id", Value: item.BatchID},
		commonkafka.MessageHeader{Key: "trace_id", Value: item.TraceID},
		commonkafka.MessageHeader{Key: "content_type", Value: "application/json"},
		commonkafka.MessageHeader{Key: "target_topic", Value: pipeline.topic},
	); err != nil {
		pipeline.releaseOutbox(ctx, workerID, item.OutboxID, err.Error())
		return err
	}
	result, err := pipeline.db.ExecContext(ctx, `UPDATE alert_assignment_batch_outbox
		SET status='published',publish_attempts=publish_attempts+1,published_at=now(),
		 locked_until=NULL,locked_by='',last_error=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, item.OutboxID, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("alert batch assignment outbox lease lost after Kafka acknowledgement")
	}
	return nil
}

func (pipeline *AlertBatchAssignmentPipeline) releaseOutbox(ctx context.Context, workerID string, outboxID int64, message string) {
	message = truncateAlertBatchError(message)
	_, _ = pipeline.db.ExecContext(ctx, `UPDATE alert_assignment_batch_outbox SET
		publish_attempts=publish_attempts+1,last_error=$3,
		status=CASE WHEN publish_attempts+1 >= $4 THEN 'dead' ELSE 'pending' END,
		next_retry_at=now()+(LEAST(300,POWER(2,LEAST(publish_attempts+1,8)))::text||' seconds')::interval,
		locked_until=NULL,locked_by=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`, outboxID, workerID, message, alertBatchOutboxMaxAttempts)
}

type alertAssignmentEventItem struct {
	AlertID               string `json:"alert_id"`
	Position              int    `json:"position"`
	ExpectedStateVersion  int64  `json:"expected_state_version"`
	ResultingStateVersion int64  `json:"resulting_state_version"`
	PreviousAssignee      string `json:"previous_assignee"`
	ResultingAssignee     string `json:"resulting_assignee"`
	PreviousStatus        string `json:"previous_status"`
	ResultingStatus       string `json:"resulting_status"`
}

type alertBatchAssignmentLifecycleEvent struct {
	EventID               string                     `json:"event_id"`
	EventType             string                     `json:"event_type"`
	SchemaVersion         int                        `json:"schema_version"`
	AggregateType         string                     `json:"aggregate_type"`
	AggregateID           string                     `json:"aggregate_id"`
	AggregateVersion      int64                      `json:"aggregate_version"`
	PartitionKey          string                     `json:"partition_key"`
	TenantID              string                     `json:"tenant_id"`
	BatchID               string                     `json:"batch_id"`
	RequestID             string                     `json:"request_id,omitempty"`
	ActionID              string                     `json:"action_id,omitempty"`
	ExpectedBatchRevision int64                      `json:"expected_batch_revision,omitempty"`
	SelectionID           string                     `json:"selection_id,omitempty"`
	SelectionSnapshotID   string                     `json:"selection_snapshot_id,omitempty"`
	SelectionSHA256       string                     `json:"selection_sha256,omitempty"`
	Assignee              string                     `json:"assignee"`
	RequestedBy           string                     `json:"requested_by"`
	Reason                string                     `json:"reason"`
	Status                string                     `json:"status"`
	TotalCount            int                        `json:"total_count"`
	Items                 []alertAssignmentEventItem `json:"items,omitempty"`
	TraceID               string                     `json:"trace_id"`
	OccurredAt            string                     `json:"occurred_at"`
}

func (pipeline *AlertBatchAssignmentPipeline) HandleKafkaMessage(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	event, err := pipeline.decodeEvent(message)
	if err != nil {
		return commonkafka.Permanent(err)
	}
	replayed, err := pipeline.inboxReplay(ctx, message, event)
	if err != nil {
		if errors.Is(err, errAlertBatchPermanent) {
			return commonkafka.Permanent(err)
		}
		return err
	}
	if replayed {
		return nil
	}
	switch event.EventType {
	case alertBatchRequestedEvent:
		err = pipeline.processRequested(ctx, message, event)
	case alertAssignmentChangedEvent:
		err = pipeline.processChanged(ctx, message, event)
	case alertBatchCompensationRequestedEvent:
		err = pipeline.processCompensationRequested(ctx, message, event)
	case alertAssignmentCompensatedEvent:
		err = pipeline.processCompensated(ctx, message, event)
	default:
		err = fmt.Errorf("%w: unsupported event type", errAlertBatchPermanent)
	}
	if errors.Is(err, errAlertBatchPermanent) {
		return commonkafka.Permanent(err)
	}
	return err
}

func (pipeline *AlertBatchAssignmentPipeline) inboxReplay(ctx context.Context, message *commonkafka.ReceivedMessage, event alertBatchAssignmentLifecycleEvent) (bool, error) {
	payloadSHA, headersSHA := alertBatchMessageDigests(message)
	var storedType, tenantID, batchID, aggregateType, aggregateID, topic, storedPayload, storedHeaders, traceID string
	var version int64
	var partition int
	var offset int64
	err := pipeline.db.QueryRowContext(ctx, `SELECT event_type,tenant_id,batch_id::text,aggregate_type,aggregate_id,aggregate_version,source_topic,
		source_partition,source_offset,payload_sha256,headers_sha256,trace_id
		FROM alert_assignment_batch_inbox WHERE event_id=$1`, event.EventID).Scan(&storedType, &tenantID,
		&batchID, &aggregateType, &aggregateID, &version, &topic, &partition, &offset, &storedPayload, &storedHeaders, &traceID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedType != event.EventType || tenantID != event.TenantID || batchID != event.BatchID ||
		aggregateType != event.AggregateType || aggregateID != event.AggregateID || version != event.AggregateVersion ||
		topic != message.Topic || partition != message.Partition ||
		offset != message.Offset || storedPayload != payloadSHA || storedHeaders != headersSHA || traceID != event.TraceID {
		return false, fmt.Errorf("%w: alert batch inbox event identity collision", errAlertBatchPermanent)
	}
	return true, nil
}

func (pipeline *AlertBatchAssignmentPipeline) decodeEvent(message *commonkafka.ReceivedMessage) (alertBatchAssignmentLifecycleEvent, error) {
	var event alertBatchAssignmentLifecycleEvent
	if message == nil || message.Topic != pipeline.topic || message.Partition < 0 || message.Offset < 0 {
		return event, fmt.Errorf("invalid alert batch assignment Kafka source")
	}
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, fmt.Errorf("decode alert batch assignment event: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return event, fmt.Errorf("alert batch assignment event contains trailing JSON")
	}
	isCompensation := event.EventType == alertBatchCompensationRequestedEvent || event.EventType == alertAssignmentCompensatedEvent
	validAggregate := (!isCompensation && event.AggregateType == "alert_assignment_batch" && event.AggregateID == event.BatchID) ||
		(isCompensation && event.AggregateType == "alert_assignment_compensation" && event.AggregateID == event.RequestID)
	if event.SchemaVersion != 1 || !validAggregate || event.AggregateVersion <= 0 ||
		strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.Assignee) == "" || len(event.Assignee) > 128 ||
		strings.TrimSpace(event.RequestedBy) == "" || len(strings.TrimSpace(event.Reason)) < 4 || len(event.Reason) > 1000 ||
		strings.TrimSpace(event.TraceID) == "" || event.TotalCount < 1 || event.TotalCount > 100 {
		return event, fmt.Errorf("incomplete alert batch assignment event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return event, fmt.Errorf("invalid alert batch assignment event_id")
	}
	if _, err := uuid.Parse(event.BatchID); err != nil {
		return event, fmt.Errorf("invalid alert batch assignment batch_id")
	}
	if isCompensation {
		if _, err := uuid.Parse(event.RequestID); err != nil {
			return event, fmt.Errorf("invalid alert batch assignment compensation request_id")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
		return event, fmt.Errorf("invalid alert batch assignment occurred_at")
	}
	if event.PartitionKey != event.TenantID+":"+event.BatchID || string(message.Key) != event.PartitionKey {
		return event, fmt.Errorf("alert batch assignment partition key mismatch")
	}
	if event.EventType == alertBatchRequestedEvent {
		if event.AggregateVersion != 1 || event.Status != "accepted" || len(event.Items) != 0 ||
			event.SelectionID == "" || event.SelectionSnapshotID == "" || len(event.SelectionSHA256) != 64 {
			return event, fmt.Errorf("invalid requested alert batch assignment contract")
		}
		if _, err := uuid.Parse(event.SelectionID); err != nil {
			return event, fmt.Errorf("invalid alert batch assignment selection_id")
		}
		if digest, err := hex.DecodeString(event.SelectionSHA256); err != nil || len(digest) != sha256.Size {
			return event, fmt.Errorf("invalid alert batch assignment selection_sha256")
		}
	} else if event.EventType == alertAssignmentChangedEvent {
		if event.AggregateVersion != 2 || event.Status != "running" || len(event.Items) == 0 || len(event.Items) > 100 {
			return event, fmt.Errorf("invalid changed alert assignment contract")
		}
		seen := map[string]bool{}
		positions := map[int]bool{}
		for index, item := range event.Items {
			if strings.TrimSpace(item.AlertID) == "" || item.Position < 0 || item.Position >= event.TotalCount ||
				item.ExpectedStateVersion <= 0 || item.ResultingStateVersion <= item.ExpectedStateVersion ||
				item.ResultingAssignee != event.Assignee || item.PreviousStatus == "" || item.ResultingStatus != "assigned" ||
				seen[item.AlertID] || positions[item.Position] {
				return event, fmt.Errorf("invalid changed alert assignment item at index %d", index)
			}
			seen[item.AlertID] = true
			positions[item.Position] = true
		}
	} else if event.EventType == alertBatchCompensationRequestedEvent || event.EventType == alertAssignmentCompensatedEvent {
		if err := validateAlertBatchCompensationEvent(event); err != nil {
			return event, err
		}
	} else {
		return event, fmt.Errorf("unsupported alert batch assignment event type")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType, "schema_version": "1",
		"aggregate_type": event.AggregateType, "aggregate_id": event.AggregateID,
		"aggregate_version": strconv.FormatInt(event.AggregateVersion, 10),
		"tenant_id":         event.TenantID, "batch_id": event.BatchID, "trace_id": event.TraceID,
		"content_type": "application/json", "target_topic": pipeline.topic,
	}
	headerCounts := make(map[string]int, len(expectedHeaders))
	for _, header := range message.Headers {
		if _, required := expectedHeaders[header.Key]; required {
			headerCounts[header.Key]++
		}
	}
	for key, expected := range expectedHeaders {
		if headerCounts[key] != 1 || message.GetHeader(key) != expected {
			return event, fmt.Errorf("alert batch assignment %s header/body mismatch", key)
		}
	}
	return event, nil
}

type alertBatchExecutionItem struct {
	AlertID              string
	Position             int
	ExpectedStateVersion int64
	Status               string
	PreviousAssignee     string
	PreviousStatus       string
	ResultingVersion     int64
	ErrorCode            string
	ErrorMessage         string
}

func (pipeline *AlertBatchAssignmentPipeline) processRequested(ctx context.Context, message *commonkafka.ReceivedMessage, event alertBatchAssignmentLifecycleEvent) error {
	items, err := pipeline.loadAcceptedItems(ctx, event)
	if err != nil {
		return err
	}
	authorized, err := pipeline.assigneeAuthorized(ctx, event.TenantID, event.Assignee)
	if err != nil {
		return err
	}
	baseTime, _ := time.Parse(time.RFC3339Nano, event.OccurredAt)
	for index := range items {
		item := &items[index]
		if !authorized {
			item.Status, item.ErrorCode, item.ErrorMessage = "forbidden", "ASSIGNEE_FORBIDDEN", "assignee is inactive, outside the tenant or lacks alert write authority"
			continue
		}
		alert, lookupErr := pipeline.authority.GetAlert(ctx, event.TenantID, item.AlertID)
		if lookupErr != nil {
			if commonerrors.IsCode(lookupErr, commonerrors.ErrCodeAlertNotFound) {
				item.Status, item.ErrorCode, item.ErrorMessage = "failed", "ALERT_NOT_FOUND", "alert does not exist in the tenant authority"
				continue
			}
			return fmt.Errorf("load alert assignment authority %s: %w", item.AlertID, lookupErr)
		}
		if int64(alert.StateVersion) != item.ExpectedStateVersion {
			item.Status, item.ErrorCode, item.ErrorMessage = "conflicted", "REVISION_CONFLICT", fmt.Sprintf("expected state_version %d but authority is %d", item.ExpectedStateVersion, alert.StateVersion)
			continue
		}
		currentStatus, parseErr := alertstate.ParseStatus(alert.Status)
		if parseErr != nil || (currentStatus != alertstate.StatusAssigned && alertstate.Transition(currentStatus, alertstate.StatusAssigned) != nil) {
			item.Status, item.ErrorCode, item.ErrorMessage = "failed", "INVALID_STATE_TRANSITION", "alert cannot transition to assigned"
			continue
		}
		item.Status = "projecting"
		item.PreviousAssignee = alert.Assignee
		item.PreviousStatus = currentStatus.String()
		candidate := baseTime.UnixMilli() + int64(item.Position) + 1
		if candidate <= item.ExpectedStateVersion {
			candidate = item.ExpectedStateVersion + 1
		}
		item.ResultingVersion = candidate
	}
	return pipeline.commitRequested(ctx, message, event, items)
}

func (pipeline *AlertBatchAssignmentPipeline) loadAcceptedItems(ctx context.Context, event alertBatchAssignmentLifecycleEvent) ([]alertBatchExecutionItem, error) {
	var selectionID, snapshotID, selectionSHA, assignee, reason, status, requestedBy, traceID, outboxEventID string
	var revision int64
	var totalCount int
	var payloadMatches bool
	canonicalPayload, _ := json.Marshal(event)
	err := pipeline.db.QueryRowContext(ctx, `SELECT b.selection_id::text,b.selection_snapshot_id,b.selection_sha256,b.assignee,b.reason,
		b.status,b.revision,b.total_count,b.requested_by,b.trace_id,o.event_id::text,o.payload=$4::jsonb
		FROM alert_assignment_batches b JOIN alert_assignment_batch_outbox o
		 ON o.tenant_id=b.tenant_id AND o.batch_id=b.batch_id AND o.aggregate_version=1 AND o.event_type=$3
		WHERE b.tenant_id=$1 AND b.batch_id=$2`, event.TenantID, event.BatchID, alertBatchRequestedEvent, string(canonicalPayload)).Scan(
		&selectionID, &snapshotID, &selectionSHA, &assignee, &reason, &status, &revision,
		&totalCount, &requestedBy, &traceID, &outboxEventID, &payloadMatches)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: requested batch is not authoritative", errAlertBatchPermanent)
	}
	if err != nil {
		return nil, err
	}
	if selectionID != event.SelectionID || snapshotID != event.SelectionSnapshotID || selectionSHA != event.SelectionSHA256 ||
		assignee != event.Assignee || reason != event.Reason || requestedBy != event.RequestedBy || traceID != event.TraceID ||
		totalCount != event.TotalCount || outboxEventID != event.EventID || !payloadMatches || status != "accepted" || revision != 1 {
		return nil, fmt.Errorf("%w: requested event differs from the PostgreSQL authority", errAlertBatchPermanent)
	}
	rows, err := pipeline.db.QueryContext(ctx, `SELECT alert_id,position,expected_state_version,status
		FROM alert_assignment_batch_items WHERE tenant_id=$1 AND batch_id=$2 ORDER BY position`, event.TenantID, event.BatchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]alertBatchExecutionItem, 0, totalCount)
	for rows.Next() {
		var item alertBatchExecutionItem
		if err := rows.Scan(&item.AlertID, &item.Position, &item.ExpectedStateVersion, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) != totalCount {
		return nil, fmt.Errorf("%w: incomplete authoritative batch items", errAlertBatchPermanent)
	}
	return items, nil
}

func (pipeline *AlertBatchAssignmentPipeline) assigneeAuthorized(ctx context.Context, tenantID, assignee string) (bool, error) {
	rows, err := pipeline.db.QueryContext(ctx, `SELECT r.name,r.permissions::text FROM users u
		LEFT JOIN user_roles ur ON ur.user_id=u.user_id LEFT JOIN roles r ON r.role_id=ur.role_id AND r.tenant_id=u.tenant_id
		WHERE u.tenant_id=$1 AND u.status='active' AND (u.username=$2 OR u.user_id::text=$2)`, tenantID, assignee)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var role sql.NullString
		var raw sql.NullString
		if err := rows.Scan(&role, &raw); err != nil {
			return false, err
		}
		if role.Valid && strings.EqualFold(role.String, "admin") {
			return true, nil
		}
		if raw.Valid && alertBatchPermissionsAllowWrite([]byte(raw.String)) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func alertBatchPermissionsAllowWrite(raw []byte) bool {
	var value interface{}
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	allowed := map[string]bool{"*": true, "alert:*": true, "alerts:*": true, "alert:write": true, "alerts:write": true}
	var permissions []string
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				permissions = append(permissions, text)
			}
		}
	case map[string]interface{}:
		for key, item := range typed {
			switch permission := item.(type) {
			case bool:
				if permission {
					permissions = append(permissions, key)
				}
			case string:
				if permission == "*" && key != "*" && !strings.Contains(key, ":") {
					permissions = append(permissions, key+":*")
				} else if permission != "" && permission != "*" && !strings.Contains(key, ":") {
					permissions = append(permissions, key+":"+permission)
				} else {
					permissions = append(permissions, key)
				}
			}
		}
	}
	for _, permission := range permissions {
		if allowed[strings.TrimSpace(permission)] {
			return true
		}
	}
	return false
}

func (pipeline *AlertBatchAssignmentPipeline) commitRequested(ctx context.Context, message *commonkafka.ReceivedMessage, event alertBatchAssignmentLifecycleEvent, items []alertBatchExecutionItem) error {
	payloadSHA, headersSHA := alertBatchMessageDigests(message)
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	replayed, err := insertAlertBatchInbox(ctx, tx, message, event, payloadSHA, headersSHA, pipeline.now().UTC())
	if err != nil {
		return err
	}
	if replayed {
		return tx.Commit()
	}
	var status, assignee, requestedBy, reason, traceID string
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT status,revision,assignee,requested_by,reason,trace_id FROM alert_assignment_batches
		WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, event.TenantID, event.BatchID).Scan(&status, &revision, &assignee, &requestedBy, &reason, &traceID); err != nil {
		return err
	}
	if status != "accepted" || revision != 1 || assignee != event.Assignee || requestedBy != event.RequestedBy || reason != event.Reason || traceID != event.TraceID {
		return fmt.Errorf("%w: batch changed before requested-event execution", errAlertBatchPermanent)
	}
	projecting := make([]alertAssignmentEventItem, 0, len(items))
	counts := map[string]int{"projecting": 0, "conflicted": 0, "forbidden": 0, "failed": 0}
	now := pipeline.now().UTC()
	changedEventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-assignment-changed-v1:"+event.EventID)).String()
	for _, item := range items {
		switch item.Status {
		case "projecting", "conflicted", "forbidden", "failed":
		default:
			return fmt.Errorf("%w: batch item has invalid execution classification", errAlertBatchPermanent)
		}
		if item.Status == "projecting" {
			var existingVersion, existingPreviousVersion sql.NullInt64
			var existingBatch sql.NullString
			var existingProjectionStatus string
			queryErr := tx.QueryRowContext(ctx, `SELECT state_version,previous_state_version,source_batch_id::text,projection_status FROM alert_assignment_states
				WHERE tenant_id=$1 AND alert_id=$2 FOR UPDATE`, event.TenantID, item.AlertID).Scan(
				&existingVersion, &existingPreviousVersion, &existingBatch, &existingProjectionStatus)
			if queryErr != nil && queryErr != sql.ErrNoRows {
				return queryErr
			}
			existingAuthorityVersion := existingVersion.Int64
			if existingProjectionStatus == "conflicted" || existingProjectionStatus == "failed" {
				existingAuthorityVersion = existingPreviousVersion.Int64
			}
			if queryErr == nil && (existingAuthorityVersion != item.ExpectedStateVersion || existingBatch.String == event.BatchID) {
				item.Status = "conflicted"
				item.ErrorCode = "REVISION_CONFLICT"
				item.ErrorMessage = fmt.Sprintf("PostgreSQL assignment authority is version %d with projection status %s from batch %s", existingAuthorityVersion, existingProjectionStatus, existingBatch.String)
				item.ResultingVersion = 0
			}
		}
		counts[item.Status]++
		itemEventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-item-command-v1:"+event.EventID+":"+item.AlertID)).String()
		result, updateErr := tx.ExecContext(ctx, `UPDATE alert_assignment_batch_items SET
			status=$4,item_revision=2,resulting_state_version=$5,previous_assignee=$6,resulting_assignee=$7,
			previous_status=$8,resulting_status='assigned',error_code=$9,error_message=$10,updated_at=$11
			WHERE tenant_id=$1 AND batch_id=$2 AND alert_id=$3 AND status='accepted' AND item_revision=1`,
			event.TenantID, event.BatchID, item.AlertID, item.Status, item.ResultingVersion,
			item.PreviousAssignee, event.Assignee, item.PreviousStatus, item.ErrorCode, item.ErrorMessage, now)
		if updateErr != nil {
			return updateErr
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return fmt.Errorf("%w: batch item command lease lost", errAlertBatchPermanent)
		}
		detail, _ := json.Marshal(map[string]interface{}{"event_id": event.EventID, "assignee": event.Assignee, "error_code": item.ErrorCode, "error_message": item.ErrorMessage})
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_item_history
			(event_id,tenant_id,batch_id,alert_id,item_revision,previous_status,resulting_status,
			expected_state_version,resulting_state_version,actor_id,reason,trace_id,detail,occurred_at)
			VALUES($1,$2,$3,$4,2,'accepted',$5,$6,$7,$8,$9,$10,$11::jsonb,$12)`,
			itemEventID, event.TenantID, event.BatchID, item.AlertID, item.Status,
			item.ExpectedStateVersion, item.ResultingVersion, event.RequestedBy, event.Reason,
			event.TraceID, string(detail), now); err != nil {
			return err
		}
		if item.Status != "projecting" {
			continue
		}
		stateEventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-assignment-state-v1:"+changedEventID+":"+item.AlertID)).String()
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_states
				(tenant_id,alert_id,state_version,assignee,status,source_batch_id,source_event_id,
					previous_state_version,previous_assignee,previous_status,projection_status,last_error,trace_id,updated_at)
				VALUES($1,$2,$3,$4,'assigned',$5,$6,$7,$8,$9,'pending','',$10,$11)
				ON CONFLICT(tenant_id,alert_id) DO UPDATE SET state_version=EXCLUDED.state_version,
					assignee=EXCLUDED.assignee,status=EXCLUDED.status,source_batch_id=EXCLUDED.source_batch_id,source_event_id=EXCLUDED.source_event_id,
					previous_state_version=EXCLUDED.previous_state_version,previous_assignee=EXCLUDED.previous_assignee,previous_status=EXCLUDED.previous_status,
				projection_status='pending',last_error='',trace_id=EXCLUDED.trace_id,updated_at=EXCLUDED.updated_at
			WHERE alert_assignment_states.state_version=EXCLUDED.previous_state_version
			   OR (alert_assignment_states.projection_status IN ('conflicted','failed')
			       AND alert_assignment_states.previous_state_version=EXCLUDED.previous_state_version)`,
			event.TenantID, item.AlertID, item.ResultingVersion, event.Assignee, event.BatchID,
			changedEventID, item.ExpectedStateVersion, item.PreviousAssignee, item.PreviousStatus, event.TraceID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_state_history
			(event_id,tenant_id,alert_id,batch_id,previous_state_version,resulting_state_version,
				 previous_assignee,resulting_assignee,previous_status,resulting_status,requested_by,reason,trace_id,occurred_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'assigned',$10,$11,$12,$13)`, stateEventID, event.TenantID,
			item.AlertID, event.BatchID, item.ExpectedStateVersion, item.ResultingVersion,
			item.PreviousAssignee, event.Assignee, item.PreviousStatus, event.RequestedBy, event.Reason, event.TraceID, now); err != nil {
			return err
		}
		projecting = append(projecting, alertAssignmentEventItem{AlertID: item.AlertID, Position: item.Position,
			ExpectedStateVersion: item.ExpectedStateVersion, ResultingStateVersion: item.ResultingVersion,
			PreviousAssignee: item.PreviousAssignee, ResultingAssignee: event.Assignee,
			PreviousStatus: item.PreviousStatus, ResultingStatus: "assigned"})
	}
	resultingStatus := "failed"
	if len(projecting) > 0 {
		resultingStatus = "running"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_batches SET status=$3,revision=2,accepted_count=0,
		applied_count=0,conflicted_count=$4,forbidden_count=$5,failed_count=$6,
		started_at=COALESCE(started_at,$7),completed_at=CASE WHEN $3='failed' THEN $7 ELSE NULL END,updated_at=$7
		WHERE tenant_id=$1 AND batch_id=$2`, event.TenantID, event.BatchID, resultingStatus,
		counts["conflicted"], counts["forbidden"], counts["failed"], now); err != nil {
		return err
	}
	snapshot, _ := json.Marshal(map[string]interface{}{"batch_id": event.BatchID, "status": resultingStatus, "revision": 2,
		"total_count": event.TotalCount, "projecting_count": len(projecting), "conflicted_count": counts["conflicted"],
		"forbidden_count": counts["forbidden"], "failed_count": counts["failed"], "trace_id": event.TraceID})
	historyEventID := changedEventID
	if len(projecting) == 0 {
		historyEventID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-terminal-v1:"+event.EventID)).String()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_history
		(event_id,tenant_id,batch_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,2,'alert-batch-assignment-execute','accepted',$4,$5,$6,$7,$8::jsonb,$9)`,
		historyEventID, event.TenantID, event.BatchID, resultingStatus, event.RequestedBy,
		event.Reason, event.TraceID, string(snapshot), now); err != nil {
		return err
	}
	if len(projecting) > 0 {
		sort.Slice(projecting, func(i, j int) bool { return projecting[i].Position < projecting[j].Position })
		changed := alertBatchAssignmentLifecycleEvent{EventID: changedEventID, EventType: alertAssignmentChangedEvent,
			SchemaVersion: 1, AggregateType: "alert_assignment_batch", AggregateID: event.BatchID,
			AggregateVersion: 2, PartitionKey: event.PartitionKey, TenantID: event.TenantID, BatchID: event.BatchID,
			Assignee: event.Assignee, RequestedBy: event.RequestedBy, Reason: event.Reason, Status: "running",
			TotalCount: event.TotalCount, Items: projecting, TraceID: event.TraceID, OccurredAt: now.Format(time.RFC3339Nano)}
		payload, _ := json.Marshal(changed)
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_outbox
				(event_id,tenant_id,batch_id,aggregate_version,aggregate_type,aggregate_id,event_type,schema_version,partition_key,payload,trace_id,status,occurred_at)
				VALUES($1,$2,$3::uuid,2,'alert_assignment_batch',$3::text,$4,1,$5,$6::jsonb,$7,'pending',$8)`, changedEventID, event.TenantID,
			event.BatchID, alertAssignmentChangedEvent, event.PartitionKey, string(payload), event.TraceID, now); err != nil {
			return err
		}
	}
	if err := insertAlertBatchPipelineAudit(ctx, tx, event, "ALERT_BATCH_ASSIGNMENT_EXECUTION_STARTED", resultingStatus, snapshot, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (pipeline *AlertBatchAssignmentPipeline) processChanged(ctx context.Context, message *commonkafka.ReceivedMessage, event alertBatchAssignmentLifecycleEvent) error {
	if err := pipeline.validateChangedAuthority(ctx, event); err != nil {
		return err
	}
	outcomes := make([]alertBatchProjectionOutcome, 0, len(event.Items))
	for _, item := range event.Items {
		_, err := pipeline.authority.ProjectAlertAssignment(ctx, event.TenantID, item.AlertID,
			event.Assignee, event.RequestedBy, uint64(item.ExpectedStateVersion), uint64(item.ResultingStateVersion))
		if err == nil {
			outcomes = append(outcomes, alertBatchProjectionOutcome{Item: item, Status: "applied"})
			continue
		}
		switch commonerrors.GetCode(err) {
		case commonerrors.ErrCodeVersionConflict:
			outcomes = append(outcomes, alertBatchProjectionOutcome{Item: item, Status: "conflicted", ErrorCode: "REVISION_CONFLICT", ErrorMessage: truncateAlertBatchError(err.Error())})
		case commonerrors.ErrCodeAlertNotFound, commonerrors.ErrCodeInvalidStateTransition, commonerrors.ErrCodeInvalidParameter:
			outcomes = append(outcomes, alertBatchProjectionOutcome{Item: item, Status: "failed", ErrorCode: commonerrors.GetCode(err).String(), ErrorMessage: truncateAlertBatchError(err.Error())})
		default:
			return fmt.Errorf("project alert assignment %s: %w", item.AlertID, err)
		}
	}
	return pipeline.commitProjectionOutcomes(ctx, message, event, outcomes)
}

func (pipeline *AlertBatchAssignmentPipeline) validateChangedAuthority(ctx context.Context, event alertBatchAssignmentLifecycleEvent) error {
	var status, assignee, requestedBy, reason, traceID, outboxEventID string
	var revision, totalCount int64
	var payloadMatches bool
	canonicalPayload, _ := json.Marshal(event)
	err := pipeline.db.QueryRowContext(ctx, `SELECT b.status,b.revision,b.total_count,b.assignee,b.requested_by,b.reason,b.trace_id,o.event_id::text,o.payload=$4::jsonb
		FROM alert_assignment_batches b JOIN alert_assignment_batch_outbox o
		 ON o.tenant_id=b.tenant_id AND o.batch_id=b.batch_id AND o.aggregate_version=2 AND o.event_type=$3
		WHERE b.tenant_id=$1 AND b.batch_id=$2`, event.TenantID, event.BatchID, alertAssignmentChangedEvent, string(canonicalPayload)).Scan(
		&status, &revision, &totalCount, &assignee, &requestedBy, &reason, &traceID, &outboxEventID, &payloadMatches)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: changed batch event is not authoritative", errAlertBatchPermanent)
	}
	if err != nil {
		return err
	}
	if status != "running" || revision != 2 || int(totalCount) != event.TotalCount || assignee != event.Assignee ||
		requestedBy != event.RequestedBy || reason != event.Reason || traceID != event.TraceID || outboxEventID != event.EventID || !payloadMatches {
		return fmt.Errorf("%w: changed event differs from running PostgreSQL authority", errAlertBatchPermanent)
	}
	rows, err := pipeline.db.QueryContext(ctx, `SELECT alert_id,position,expected_state_version,resulting_state_version,
		previous_assignee,resulting_assignee,previous_status,resulting_status FROM alert_assignment_batch_items
		WHERE tenant_id=$1 AND batch_id=$2 AND status='projecting' ORDER BY position`, event.TenantID, event.BatchID)
	if err != nil {
		return err
	}
	defer rows.Close()
	authoritative := make([]alertAssignmentEventItem, 0, len(event.Items))
	for rows.Next() {
		var item alertAssignmentEventItem
		if err := rows.Scan(&item.AlertID, &item.Position, &item.ExpectedStateVersion,
			&item.ResultingStateVersion, &item.PreviousAssignee, &item.ResultingAssignee,
			&item.PreviousStatus, &item.ResultingStatus); err != nil {
			return err
		}
		authoritative = append(authoritative, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(authoritative) != len(event.Items) {
		return fmt.Errorf("%w: changed event has incomplete projecting items", errAlertBatchPermanent)
	}
	for index := range authoritative {
		if authoritative[index] != event.Items[index] {
			return fmt.Errorf("%w: changed event item %d differs from PostgreSQL authority", errAlertBatchPermanent, index)
		}
	}
	return nil
}

type alertBatchProjectionOutcome struct {
	Item         alertAssignmentEventItem
	Status       string
	ErrorCode    string
	ErrorMessage string
}

func (pipeline *AlertBatchAssignmentPipeline) commitProjectionOutcomes(ctx context.Context, message *commonkafka.ReceivedMessage, event alertBatchAssignmentLifecycleEvent, outcomes []alertBatchProjectionOutcome) error {
	payloadSHA, headersSHA := alertBatchMessageDigests(message)
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := pipeline.now().UTC()
	replayed, err := insertAlertBatchInbox(ctx, tx, message, event, payloadSHA, headersSHA, now)
	if err != nil {
		return err
	}
	if replayed {
		return tx.Commit()
	}
	var status, assignee, requestedBy, reason, traceID string
	var revision, totalCount int64
	if err := tx.QueryRowContext(ctx, `SELECT status,revision,total_count,assignee,requested_by,reason,trace_id
		FROM alert_assignment_batches WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, event.TenantID, event.BatchID).Scan(
		&status, &revision, &totalCount, &assignee, &requestedBy, &reason, &traceID); err != nil {
		return err
	}
	if status != "running" || revision != 2 || int(totalCount) != event.TotalCount || assignee != event.Assignee ||
		requestedBy != event.RequestedBy || reason != event.Reason || traceID != event.TraceID {
		return fmt.Errorf("%w: changed event differs from running batch authority", errAlertBatchPermanent)
	}
	for _, outcome := range outcomes {
		var storedStatus, previousAssignee, resultingAssignee, previousStatus, resultingStatus string
		var expectedVersion, resultingVersion, itemRevision int64
		if err := tx.QueryRowContext(ctx, `SELECT status,expected_state_version,resulting_state_version,
			previous_assignee,resulting_assignee,previous_status,resulting_status,item_revision FROM alert_assignment_batch_items
			WHERE tenant_id=$1 AND batch_id=$2 AND alert_id=$3 FOR UPDATE`, event.TenantID, event.BatchID,
			outcome.Item.AlertID).Scan(&storedStatus, &expectedVersion, &resultingVersion, &previousAssignee,
			&resultingAssignee, &previousStatus, &resultingStatus, &itemRevision); err != nil {
			return err
		}
		if storedStatus != "projecting" || itemRevision != 2 || expectedVersion != outcome.Item.ExpectedStateVersion ||
			resultingVersion != outcome.Item.ResultingStateVersion || previousAssignee != outcome.Item.PreviousAssignee ||
			resultingAssignee != outcome.Item.ResultingAssignee || previousStatus != outcome.Item.PreviousStatus ||
			resultingStatus != outcome.Item.ResultingStatus {
			return fmt.Errorf("%w: changed item differs from PostgreSQL authority", errAlertBatchPermanent)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_batch_items SET status=$4,item_revision=3,
			error_code=$5,error_message=$6,updated_at=$7 WHERE tenant_id=$1 AND batch_id=$2 AND alert_id=$3`,
			event.TenantID, event.BatchID, outcome.Item.AlertID, outcome.Status, outcome.ErrorCode, outcome.ErrorMessage, now); err != nil {
			return err
		}
		itemEventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-item-result-v1:"+event.EventID+":"+outcome.Item.AlertID)).String()
		detail, _ := json.Marshal(map[string]interface{}{"projection_event_id": event.EventID, "error_code": outcome.ErrorCode, "error_message": outcome.ErrorMessage})
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_item_history
			(event_id,tenant_id,batch_id,alert_id,item_revision,previous_status,resulting_status,
			expected_state_version,resulting_state_version,actor_id,reason,trace_id,detail,occurred_at)
			VALUES($1,$2,$3,$4,3,'projecting',$5,$6,$7,$8,$9,$10,$11::jsonb,$12)`, itemEventID,
			event.TenantID, event.BatchID, outcome.Item.AlertID, outcome.Status,
			outcome.Item.ExpectedStateVersion, outcome.Item.ResultingStateVersion, event.RequestedBy,
			event.Reason, event.TraceID, string(detail), now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_projection_receipts
			(event_id,tenant_id,batch_id,alert_id,expected_state_version,resulting_state_version,
			 previous_assignee,resulting_assignee,previous_status,resulting_status,outcome,error_code,error_message,source_topic,
			 source_partition,source_offset,trace_id,applied_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, event.EventID,
			event.TenantID, event.BatchID, outcome.Item.AlertID, outcome.Item.ExpectedStateVersion,
			outcome.Item.ResultingStateVersion, outcome.Item.PreviousAssignee, outcome.Item.ResultingAssignee,
			outcome.Item.PreviousStatus, outcome.Item.ResultingStatus, outcome.Status, outcome.ErrorCode, outcome.ErrorMessage, message.Topic, message.Partition,
			message.Offset, event.TraceID, now); err != nil {
			return err
		}
		stateResult, err := tx.ExecContext(ctx, `UPDATE alert_assignment_states SET projection_status=$4,
			last_error=$5,updated_at=$6 WHERE tenant_id=$1 AND alert_id=$2 AND source_event_id=$3`,
			event.TenantID, outcome.Item.AlertID, event.EventID, outcome.Status,
			truncateAlertBatchError(outcome.ErrorMessage), now)
		if err != nil {
			return err
		}
		if affected, _ := stateResult.RowsAffected(); affected != 1 {
			return fmt.Errorf("%w: assignment projection state receipt is missing", errAlertBatchPermanent)
		}
	}
	var accepted, applied, conflicted, forbidden, failed int
	if err := tx.QueryRowContext(ctx, `SELECT
		count(*) FILTER(WHERE status='accepted'),count(*) FILTER(WHERE status='applied'),
		count(*) FILTER(WHERE status='conflicted'),count(*) FILTER(WHERE status='forbidden'),
		count(*) FILTER(WHERE status='failed') FROM alert_assignment_batch_items
		WHERE tenant_id=$1 AND batch_id=$2`, event.TenantID, event.BatchID).Scan(&accepted, &applied, &conflicted, &forbidden, &failed); err != nil {
		return err
	}
	terminal := "failed"
	if applied == int(totalCount) {
		terminal = "completed"
	} else if applied > 0 {
		terminal = "partial"
	}
	if accepted != 0 || applied+conflicted+forbidden+failed != int(totalCount) {
		return fmt.Errorf("%w: terminal item accounting is incomplete", errAlertBatchPermanent)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_batches SET status=$3,revision=3,
		accepted_count=0,applied_count=$4,conflicted_count=$5,forbidden_count=$6,failed_count=$7,
		completed_at=$8,updated_at=$8 WHERE tenant_id=$1 AND batch_id=$2`, event.TenantID, event.BatchID,
		terminal, applied, conflicted, forbidden, failed, now); err != nil {
		return err
	}
	snapshot, _ := json.Marshal(map[string]interface{}{"batch_id": event.BatchID, "status": terminal, "revision": 3,
		"total_count": totalCount, "applied_count": applied, "conflicted_count": conflicted,
		"forbidden_count": forbidden, "failed_count": failed, "trace_id": event.TraceID,
		"source_topic": message.Topic, "source_partition": message.Partition, "source_offset": message.Offset})
	historyID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-result-v1:"+event.EventID)).String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_history
		(event_id,tenant_id,batch_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,3,'alert-batch-assignment-result','running',$4,$5,$6,$7,$8::jsonb,$9)`, historyID,
		event.TenantID, event.BatchID, terminal, event.RequestedBy, event.Reason, event.TraceID, string(snapshot), now); err != nil {
		return err
	}
	if err := insertAlertBatchPipelineAudit(ctx, tx, event, "ALERT_BATCH_ASSIGNMENT_TERMINAL", terminal, snapshot, now); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAlertBatchInbox(ctx context.Context, tx *sql.Tx, message *commonkafka.ReceivedMessage, event alertBatchAssignmentLifecycleEvent, payloadSHA, headersSHA string, now time.Time) (bool, error) {
	var storedType, tenantID, batchID, aggregateType, aggregateID, topic, storedPayload, storedHeaders, traceID string
	var version int64
	var partition int
	var offset int64
	err := tx.QueryRowContext(ctx, `SELECT event_type,tenant_id,batch_id::text,aggregate_type,aggregate_id,aggregate_version,source_topic,
		source_partition,source_offset,payload_sha256,headers_sha256,trace_id
		FROM alert_assignment_batch_inbox WHERE event_id=$1`, event.EventID).Scan(&storedType, &tenantID,
		&batchID, &aggregateType, &aggregateID, &version, &topic, &partition, &offset, &storedPayload, &storedHeaders, &traceID)
	if err == nil {
		if storedType != event.EventType || tenantID != event.TenantID || batchID != event.BatchID ||
			aggregateType != event.AggregateType || aggregateID != event.AggregateID || version != event.AggregateVersion ||
			topic != message.Topic || partition != message.Partition ||
			offset != message.Offset || storedPayload != payloadSHA || storedHeaders != headersSHA || traceID != event.TraceID {
			return false, fmt.Errorf("%w: alert batch inbox event identity collision", errAlertBatchPermanent)
		}
		return true, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	var existingEvent string
	err = tx.QueryRowContext(ctx, `SELECT event_id::text FROM alert_assignment_batch_inbox
		WHERE source_topic=$1 AND source_partition=$2 AND source_offset=$3`, message.Topic, message.Partition, message.Offset).Scan(&existingEvent)
	if err == nil {
		return false, fmt.Errorf("%w: alert batch inbox source tuple collision with %s", errAlertBatchPermanent, existingEvent)
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_inbox
		(event_id,event_type,tenant_id,batch_id,aggregate_type,aggregate_id,aggregate_version,source_topic,source_partition,
		 source_offset,payload_sha256,headers_sha256,trace_id,processed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, event.EventID, event.EventType,
		event.TenantID, event.BatchID, event.AggregateType, event.AggregateID, event.AggregateVersion,
		message.Topic, message.Partition, message.Offset, payloadSHA, headersSHA, event.TraceID, now)
	return false, err
}

func insertAlertBatchPipelineAudit(ctx context.Context, tx *sql.Tx, event alertBatchAssignmentLifecycleEvent, action, result string, detail []byte, now time.Time) error {
	eventID := "audit-alert-batch-" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(action+":"+event.EventID)).String()
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
		VALUES($1,$2,NULL,$3,'alert_assignment_batch',$4,$5::jsonb,$6,$7,$8,$9)`, eventID,
		event.TenantID, action, event.BatchID, string(detail), event.TraceID, result,
		result != "failed", now)
	return err
}

func alertBatchMessageDigests(message *commonkafka.ReceivedMessage) (string, string) {
	payload := sha256.Sum256(message.Value)
	headers, _ := json.Marshal(message.GetAllHeaders())
	headerDigest := sha256.Sum256(headers)
	return hex.EncodeToString(payload[:]), hex.EncodeToString(headerDigest[:])
}

func truncateAlertBatchError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		return message[:2000]
	}
	return message
}

func (pipeline *AlertBatchAssignmentPipeline) RecordDLQAcknowledgement(ctx context.Context, message *commonkafka.ReceivedMessage, processingErr error) error {
	if message == nil || message.Topic != pipeline.topic || message.Partition < 0 || message.Offset < 0 ||
		processingErr == nil || !commonkafka.IsPermanent(processingErr) {
		return fmt.Errorf("invalid alert batch assignment DLQ acknowledgement")
	}
	headers := message.GetAllHeaders()
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	payloadSHA, headersSHA := alertBatchMessageDigests(message)
	var aggregateVersion sql.NullInt64
	if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(headers["aggregate_version"]), 10, 64); parseErr == nil && parsed > 0 {
		aggregateVersion = sql.NullInt64{Int64: parsed, Valid: true}
	}
	now := pipeline.now().UTC()
	tx, err := pipeline.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var storedPayload, storedHeaders, storedEvent string
	if err := tx.QueryRowContext(ctx, `INSERT INTO alert_assignment_batch_dlq_receipts
		(source_topic,source_partition,source_offset,dlq_topic,event_id,event_type,tenant_id,batch_id,
		 aggregate_version,trace_id,error_code,error_message,payload_sha256,headers_sha256,headers,acknowledged_at)
		VALUES($1,$2,$3,'dlq.v1',$4,$5,$6,$7,$8,$9,'PROCESSING_FAILED',$10,$11,$12,$13::jsonb,$14)
		ON CONFLICT(source_topic,source_partition,source_offset) DO UPDATE
		SET acknowledged_at=alert_assignment_batch_dlq_receipts.acknowledged_at
		RETURNING payload_sha256,headers_sha256,event_id`, message.Topic, message.Partition, message.Offset,
		headers["event_id"], headers["event_type"], headers["tenant_id"], headers["batch_id"],
		aggregateVersion, headers["trace_id"], truncateAlertBatchError(processingErr.Error()), payloadSHA,
		headersSHA, string(headersJSON), now).Scan(&storedPayload, &storedHeaders, &storedEvent); err != nil {
		return err
	}
	if storedPayload != payloadSHA || storedHeaders != headersSHA || storedEvent != headers["event_id"] {
		return fmt.Errorf("alert batch assignment DLQ source tuple collision")
	}
	auditID := "audit-alert-batch-dlq-" + uuid.NewSHA1(uuid.NameSpaceOID,
		[]byte(fmt.Sprintf("%s:%d:%d", message.Topic, message.Partition, message.Offset))).String()
	detail, _ := json.Marshal(map[string]interface{}{"source_topic": message.Topic, "source_partition": message.Partition,
		"source_offset": message.Offset, "event_id": headers["event_id"], "event_type": headers["event_type"],
		"batch_id": headers["batch_id"], "payload_sha256": payloadSHA, "headers_sha256": headersSHA,
		"source_offset_commit_pending": true})
	tenantID := strings.TrimSpace(headers["tenant_id"])
	if tenantID == "" {
		tenantID = "__unknown__"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,error_message,created_at)
		VALUES($1,$2,NULL,'ALERT_BATCH_ASSIGNMENT_EVENT_QUARANTINED','alert_assignment_event',$3,
		$4::jsonb,$5,'quarantined',true,$6,$7) ON CONFLICT(event_id) DO NOTHING`, auditID, tenantID,
		headers["batch_id"], string(detail), headers["trace_id"], truncateAlertBatchError(processingErr.Error()), now); err != nil {
		return err
	}
	return tx.Commit()
}

type AlertBatchAssignmentEventConsumer struct {
	consumer *commonkafka.Consumer
	pipeline *AlertBatchAssignmentPipeline
}

func NewAlertBatchAssignmentEventConsumer(consumer *commonkafka.Consumer, pipeline *AlertBatchAssignmentPipeline) (*AlertBatchAssignmentEventConsumer, error) {
	if consumer == nil || pipeline == nil {
		return nil, fmt.Errorf("alert batch assignment Kafka consumer and pipeline are required")
	}
	return &AlertBatchAssignmentEventConsumer{consumer: consumer, pipeline: pipeline}, nil
}

func (consumer *AlertBatchAssignmentEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.pipeline.HandleKafkaMessage)
}

func (consumer *AlertBatchAssignmentEventConsumer) Close() error {
	return consumer.consumer.Close()
}

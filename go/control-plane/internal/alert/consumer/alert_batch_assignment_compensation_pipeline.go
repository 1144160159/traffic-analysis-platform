package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
)

func validateAlertBatchCompensationEvent(event alertBatchAssignmentLifecycleEvent) error {
	if event.AggregateType != "alert_assignment_compensation" || event.AggregateID != event.RequestID ||
		event.ActionID != "alert-batch-assignment-compensate" || event.ExpectedBatchRevision != 3 ||
		strings.TrimSpace(event.RequestedBy) == "" || len(strings.TrimSpace(event.Reason)) < 8 ||
		event.SelectionID != "" || event.SelectionSnapshotID != "" || event.SelectionSHA256 != "" {
		return fmt.Errorf("invalid alert batch assignment compensation contract")
	}
	if event.EventType == alertBatchCompensationRequestedEvent {
		if event.AggregateVersion != 1 || event.Status != "accepted" || len(event.Items) != 0 {
			return fmt.Errorf("invalid requested alert batch assignment compensation contract")
		}
		return nil
	}
	if event.AggregateVersion != 2 || event.Status != "running" || len(event.Items) == 0 || len(event.Items) > event.TotalCount {
		return fmt.Errorf("invalid compensated alert assignment contract")
	}
	seen := map[string]bool{}
	positions := map[int]bool{}
	for index, item := range event.Items {
		if strings.TrimSpace(item.AlertID) == "" || item.Position < 0 || item.Position >= 100 ||
			item.ExpectedStateVersion <= 0 || item.ResultingStateVersion <= item.ExpectedStateVersion ||
			item.PreviousAssignee != event.Assignee || item.PreviousStatus != "assigned" ||
			strings.TrimSpace(item.ResultingStatus) == "" || seen[item.AlertID] || positions[item.Position] {
			return fmt.Errorf("invalid compensated alert assignment item at index %d", index)
		}
		seen[item.AlertID] = true
		positions[item.Position] = true
	}
	return nil
}

type alertBatchCompensationExecutionItem struct {
	AlertID                  string
	Position                 int
	Status                   string
	ExpectedStateVersion     int64
	CompensationStateVersion int64
	RestoreAssignee          string
	RestoreStatus            string
	CurrentAssignee          string
	CurrentStatus            string
	ErrorCode                string
	ErrorMessage             string
}

func (pipeline *AlertBatchAssignmentPipeline) processCompensationRequested(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
	event alertBatchAssignmentLifecycleEvent,
) error {
	items, err := pipeline.loadAcceptedCompensationItems(ctx, event)
	if err != nil {
		return err
	}
	baseTime, _ := time.Parse(time.RFC3339Nano, event.OccurredAt)
	for index := range items {
		item := &items[index]
		alert, lookupErr := pipeline.authority.GetAlert(ctx, event.TenantID, item.AlertID)
		if lookupErr != nil {
			if commonerrors.IsCode(lookupErr, commonerrors.ErrCodeAlertNotFound) {
				item.Status, item.ErrorCode, item.ErrorMessage = "failed", "ALERT_NOT_FOUND", "alert does not exist in the tenant authority"
				continue
			}
			return fmt.Errorf("load alert compensation authority %s: %w", item.AlertID, lookupErr)
		}
		if int64(alert.StateVersion) != item.ExpectedStateVersion || alert.Assignee != item.CurrentAssignee || alert.Status != item.CurrentStatus {
			item.Status, item.ErrorCode, item.ErrorMessage = "conflicted", "REVISION_CONFLICT",
				fmt.Sprintf("expected %d/%s/%s but authority is %d/%s/%s", item.ExpectedStateVersion,
					item.CurrentAssignee, item.CurrentStatus, alert.StateVersion, alert.Assignee, alert.Status)
			continue
		}
		item.Status = "projecting"
		candidate := baseTime.UnixMilli() + int64(item.Position) + 1
		if candidate <= item.ExpectedStateVersion {
			candidate = item.ExpectedStateVersion + 1
		}
		item.CompensationStateVersion = candidate
	}
	return pipeline.commitCompensationRequested(ctx, message, event, items)
}

func (pipeline *AlertBatchAssignmentPipeline) loadAcceptedCompensationItems(
	ctx context.Context,
	event alertBatchAssignmentLifecycleEvent,
) ([]alertBatchCompensationExecutionItem, error) {
	canonicalPayload, _ := json.Marshal(event)
	var status, actionID, requestedBy, reason, traceID, assignee, outboxEventID string
	var revision, expectedBatchRevision int64
	var totalCount int
	var payloadMatches bool
	err := pipeline.db.QueryRowContext(ctx, `SELECT r.status,r.revision,r.expected_batch_revision,r.total_count,
		r.action_id,r.requested_by,r.reason,r.trace_id,b.assignee,o.event_id::text,o.payload=$5::jsonb
		FROM alert_assignment_compensation_requests r
		JOIN alert_assignment_batches b ON b.tenant_id=r.tenant_id AND b.batch_id=r.batch_id
		JOIN alert_assignment_batch_outbox o ON o.tenant_id=r.tenant_id AND o.batch_id=r.batch_id
		 AND o.aggregate_type='alert_assignment_compensation' AND o.aggregate_id=r.request_id::text
		 AND o.aggregate_version=1 AND o.event_type=$4
		WHERE r.tenant_id=$1 AND r.batch_id=$2 AND r.request_id=$3`, event.TenantID, event.BatchID,
		event.RequestID, alertBatchCompensationRequestedEvent, string(canonicalPayload)).Scan(&status, &revision,
		&expectedBatchRevision, &totalCount, &actionID, &requestedBy, &reason, &traceID, &assignee,
		&outboxEventID, &payloadMatches)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: requested compensation is not authoritative", errAlertBatchPermanent)
	}
	if err != nil {
		return nil, err
	}
	if status != "accepted" || revision != 1 || expectedBatchRevision != event.ExpectedBatchRevision ||
		totalCount != event.TotalCount || actionID != event.ActionID || requestedBy != event.RequestedBy ||
		reason != event.Reason || traceID != event.TraceID || assignee != event.Assignee ||
		outboxEventID != event.EventID || !payloadMatches {
		return nil, fmt.Errorf("%w: requested compensation differs from PostgreSQL authority", errAlertBatchPermanent)
	}
	rows, err := pipeline.db.QueryContext(ctx, `SELECT alert_id,position,status,expected_state_version,
		restore_assignee,restore_status,current_assignee,current_status
		FROM alert_assignment_compensation_items WHERE tenant_id=$1 AND request_id=$2 ORDER BY position`,
		event.TenantID, event.RequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]alertBatchCompensationExecutionItem, 0, totalCount)
	for rows.Next() {
		var item alertBatchCompensationExecutionItem
		if err := rows.Scan(&item.AlertID, &item.Position, &item.Status, &item.ExpectedStateVersion,
			&item.RestoreAssignee, &item.RestoreStatus, &item.CurrentAssignee, &item.CurrentStatus); err != nil {
			return nil, err
		}
		if item.Status != "accepted" {
			return nil, fmt.Errorf("%w: compensation item is not accepted", errAlertBatchPermanent)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) != totalCount {
		return nil, fmt.Errorf("%w: incomplete compensation item authority", errAlertBatchPermanent)
	}
	return items, nil
}

func (pipeline *AlertBatchAssignmentPipeline) commitCompensationRequested(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
	event alertBatchAssignmentLifecycleEvent,
	items []alertBatchCompensationExecutionItem,
) error {
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
	var status, actionID, requestedBy, reason, traceID string
	var revision, expectedBatchRevision int64
	var totalCount int
	if err := tx.QueryRowContext(ctx, `SELECT status,revision,expected_batch_revision,total_count,action_id,requested_by,reason,trace_id
		FROM alert_assignment_compensation_requests WHERE tenant_id=$1 AND batch_id=$2 AND request_id=$3 FOR UPDATE`,
		event.TenantID, event.BatchID, event.RequestID).Scan(&status, &revision, &expectedBatchRevision,
		&totalCount, &actionID, &requestedBy, &reason, &traceID); err != nil {
		return err
	}
	if status != "accepted" || revision != 1 || expectedBatchRevision != event.ExpectedBatchRevision ||
		totalCount != event.TotalCount || actionID != event.ActionID || requestedBy != event.RequestedBy ||
		reason != event.Reason || traceID != event.TraceID {
		return fmt.Errorf("%w: compensation changed before requested-event execution", errAlertBatchPermanent)
	}
	changedEventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-assignment-compensated-v1:"+event.EventID)).String()
	projecting := make([]alertAssignmentEventItem, 0, len(items))
	counts := map[string]int{"projecting": 0, "conflicted": 0, "failed": 0}
	for _, item := range items {
		if item.Status != "projecting" && item.Status != "conflicted" && item.Status != "failed" {
			return fmt.Errorf("%w: invalid compensation execution classification", errAlertBatchPermanent)
		}
		if item.Status == "projecting" {
			var stateVersion int64
			var stateAssignee, stateStatus, stateBatch, projectionStatus string
			queryErr := tx.QueryRowContext(ctx, `SELECT state_version,assignee,status,source_batch_id::text,projection_status
				FROM alert_assignment_states WHERE tenant_id=$1 AND alert_id=$2 FOR UPDATE`,
				event.TenantID, item.AlertID).Scan(&stateVersion, &stateAssignee, &stateStatus, &stateBatch, &projectionStatus)
			if queryErr != nil && queryErr != sql.ErrNoRows {
				return queryErr
			}
			if queryErr == sql.ErrNoRows || stateVersion != item.ExpectedStateVersion || stateAssignee != item.CurrentAssignee ||
				stateStatus != item.CurrentStatus || stateBatch != event.BatchID || projectionStatus != "applied" {
				item.Status, item.ErrorCode, item.ErrorMessage = "conflicted", "REVISION_CONFLICT",
					"PostgreSQL assignment projection authority no longer matches the applied batch item"
				item.CompensationStateVersion = 0
			}
		}
		counts[item.Status]++
		itemHistoryID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-compensation-item-command-v1:"+event.EventID+":"+item.AlertID)).String()
		result, err := tx.ExecContext(ctx, `UPDATE alert_assignment_compensation_items SET status=$4,item_revision=2,
			compensation_state_version=$5,error_code=$6,error_message=$7,updated_at=$8
			WHERE tenant_id=$1 AND request_id=$2 AND alert_id=$3 AND status='accepted' AND item_revision=1`,
			event.TenantID, event.RequestID, item.AlertID, item.Status, item.CompensationStateVersion,
			item.ErrorCode, truncateAlertBatchError(item.ErrorMessage), now)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return fmt.Errorf("%w: compensation item command lease lost", errAlertBatchPermanent)
		}
		detail, _ := json.Marshal(map[string]interface{}{
			"event_id": event.EventID, "restore_assignee": item.RestoreAssignee, "restore_status": item.RestoreStatus,
			"error_code": item.ErrorCode, "error_message": item.ErrorMessage,
		})
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_item_history
			(event_id,tenant_id,request_id,batch_id,alert_id,item_revision,previous_status,resulting_status,
			 expected_state_version,compensation_state_version,actor_id,reason,trace_id,detail,occurred_at)
			VALUES($1,$2,$3,$4,$5,2,'accepted',$6,$7,$8,$9,$10,$11,$12::jsonb,$13)`, itemHistoryID,
			event.TenantID, event.RequestID, event.BatchID, item.AlertID, item.Status, item.ExpectedStateVersion,
			item.CompensationStateVersion, event.RequestedBy, event.Reason, event.TraceID, string(detail), now); err != nil {
			return err
		}
		if item.Status != "projecting" {
			continue
		}
		stateHistoryID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-assignment-compensation-state-v1:"+changedEventID+":"+item.AlertID)).String()
		stateResult, err := tx.ExecContext(ctx, `UPDATE alert_assignment_states SET state_version=$3,assignee=$4,status=$5,
			source_event_id=$6,previous_state_version=$7,previous_assignee=$8,previous_status=$9,
			projection_status='pending',last_error='',trace_id=$10,updated_at=$11
			WHERE tenant_id=$1 AND alert_id=$2 AND state_version=$7 AND assignee=$12 AND status='assigned'
			 AND source_batch_id=$13 AND projection_status='applied'`, event.TenantID, item.AlertID,
			item.CompensationStateVersion, item.RestoreAssignee, item.RestoreStatus, changedEventID,
			item.ExpectedStateVersion, item.CurrentAssignee, item.CurrentStatus, event.TraceID, now,
			item.CurrentAssignee, event.BatchID)
		if err != nil {
			return err
		}
		if affected, _ := stateResult.RowsAffected(); affected != 1 {
			return fmt.Errorf("%w: compensation PostgreSQL state lease lost", errAlertBatchPermanent)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_state_history
			(event_id,tenant_id,alert_id,batch_id,previous_state_version,resulting_state_version,
			 previous_assignee,resulting_assignee,previous_status,resulting_status,requested_by,reason,trace_id,occurred_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, stateHistoryID, event.TenantID,
			item.AlertID, event.BatchID, item.ExpectedStateVersion, item.CompensationStateVersion,
			item.CurrentAssignee, item.RestoreAssignee, item.CurrentStatus, item.RestoreStatus,
			event.RequestedBy, event.Reason, event.TraceID, now); err != nil {
			return err
		}
		projecting = append(projecting, alertAssignmentEventItem{
			AlertID: item.AlertID, Position: item.Position, ExpectedStateVersion: item.ExpectedStateVersion,
			ResultingStateVersion: item.CompensationStateVersion, PreviousAssignee: item.CurrentAssignee,
			ResultingAssignee: item.RestoreAssignee, PreviousStatus: item.CurrentStatus, ResultingStatus: item.RestoreStatus,
		})
	}
	resultingStatus := "failed"
	if len(projecting) > 0 {
		resultingStatus = "running"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_compensation_requests SET status=$4,revision=2,
		accepted_count=0,compensated_count=0,conflicted_count=$5,failed_count=$6,
		started_at=COALESCE(started_at,$7),completed_at=CASE WHEN $4='failed' THEN $7 ELSE NULL END,updated_at=$7
		WHERE tenant_id=$1 AND batch_id=$2 AND request_id=$3`, event.TenantID, event.BatchID, event.RequestID,
		resultingStatus, counts["conflicted"], counts["failed"], now); err != nil {
		return err
	}
	snapshot, _ := json.Marshal(map[string]interface{}{
		"request_id": event.RequestID, "batch_id": event.BatchID, "status": resultingStatus, "revision": 2,
		"total_count": event.TotalCount, "projecting_count": len(projecting),
		"conflicted_count": counts["conflicted"], "failed_count": counts["failed"], "trace_id": event.TraceID,
	})
	historyID := changedEventID
	if len(projecting) == 0 {
		historyID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-compensation-terminal-v1:"+event.EventID)).String()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_history
		(event_id,tenant_id,request_id,batch_id,revision,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,$4,2,'accepted',$5,$6,$7,$8,$9::jsonb,$10)`, historyID, event.TenantID,
		event.RequestID, event.BatchID, resultingStatus, event.RequestedBy, event.Reason, event.TraceID, string(snapshot), now); err != nil {
		return err
	}
	if len(projecting) > 0 {
		sort.Slice(projecting, func(i, j int) bool { return projecting[i].Position < projecting[j].Position })
		changed := alertBatchAssignmentLifecycleEvent{
			EventID: changedEventID, EventType: alertAssignmentCompensatedEvent, SchemaVersion: 1,
			AggregateType: "alert_assignment_compensation", AggregateID: event.RequestID, AggregateVersion: 2,
			PartitionKey: event.PartitionKey, TenantID: event.TenantID, BatchID: event.BatchID, RequestID: event.RequestID,
			ActionID: event.ActionID, ExpectedBatchRevision: event.ExpectedBatchRevision, Assignee: event.Assignee,
			RequestedBy: event.RequestedBy, Reason: event.Reason, Status: "running", TotalCount: event.TotalCount,
			Items: projecting, TraceID: event.TraceID, OccurredAt: now.Format(time.RFC3339Nano),
		}
		payload, _ := json.Marshal(changed)
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_batch_outbox
			(event_id,tenant_id,batch_id,aggregate_version,aggregate_type,aggregate_id,event_type,schema_version,
			 partition_key,payload,trace_id,status,occurred_at)
			VALUES($1,$2,$3,2,'alert_assignment_compensation',$4,$5,1,$6,$7::jsonb,$8,'pending',$9)`,
			changedEventID, event.TenantID, event.BatchID, event.RequestID, alertAssignmentCompensatedEvent,
			event.PartitionKey, string(payload), event.TraceID, now); err != nil {
			return err
		}
	}
	if err := insertAlertBatchCompensationPipelineAudit(ctx, tx, event,
		"ALERT_BATCH_ASSIGNMENT_COMPENSATION_STARTED", resultingStatus, snapshot, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (pipeline *AlertBatchAssignmentPipeline) processCompensated(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
	event alertBatchAssignmentLifecycleEvent,
) error {
	if err := pipeline.validateCompensatedAuthority(ctx, event); err != nil {
		return err
	}
	outcomes := make([]alertBatchProjectionOutcome, 0, len(event.Items))
	for _, item := range event.Items {
		_, err := pipeline.authority.ProjectAlertAssignmentCompensation(ctx, event.TenantID, item.AlertID,
			item.ResultingAssignee, item.ResultingStatus, event.RequestedBy,
			uint64(item.ExpectedStateVersion), uint64(item.ResultingStateVersion))
		if err == nil {
			outcomes = append(outcomes, alertBatchProjectionOutcome{Item: item, Status: "compensated"})
			continue
		}
		switch commonerrors.GetCode(err) {
		case commonerrors.ErrCodeVersionConflict:
			outcomes = append(outcomes, alertBatchProjectionOutcome{Item: item, Status: "conflicted",
				ErrorCode: "REVISION_CONFLICT", ErrorMessage: truncateAlertBatchError(err.Error())})
		case commonerrors.ErrCodeAlertNotFound, commonerrors.ErrCodeInvalidStateTransition, commonerrors.ErrCodeInvalidParameter:
			outcomes = append(outcomes, alertBatchProjectionOutcome{Item: item, Status: "failed",
				ErrorCode: commonerrors.GetCode(err).String(), ErrorMessage: truncateAlertBatchError(err.Error())})
		default:
			return fmt.Errorf("project alert assignment compensation %s: %w", item.AlertID, err)
		}
	}
	return pipeline.commitCompensationProjectionOutcomes(ctx, message, event, outcomes)
}

func (pipeline *AlertBatchAssignmentPipeline) validateCompensatedAuthority(
	ctx context.Context,
	event alertBatchAssignmentLifecycleEvent,
) error {
	canonicalPayload, _ := json.Marshal(event)
	var status, actionID, requestedBy, reason, traceID, assignee, outboxEventID string
	var revision, expectedBatchRevision int64
	var totalCount int
	var payloadMatches bool
	err := pipeline.db.QueryRowContext(ctx, `SELECT r.status,r.revision,r.expected_batch_revision,r.total_count,
		r.action_id,r.requested_by,r.reason,r.trace_id,b.assignee,o.event_id::text,o.payload=$5::jsonb
		FROM alert_assignment_compensation_requests r
		JOIN alert_assignment_batches b ON b.tenant_id=r.tenant_id AND b.batch_id=r.batch_id
		JOIN alert_assignment_batch_outbox o ON o.tenant_id=r.tenant_id AND o.batch_id=r.batch_id
		 AND o.aggregate_type='alert_assignment_compensation' AND o.aggregate_id=r.request_id::text
		 AND o.aggregate_version=2 AND o.event_type=$4
		WHERE r.tenant_id=$1 AND r.batch_id=$2 AND r.request_id=$3`, event.TenantID, event.BatchID,
		event.RequestID, alertAssignmentCompensatedEvent, string(canonicalPayload)).Scan(&status, &revision,
		&expectedBatchRevision, &totalCount, &actionID, &requestedBy, &reason, &traceID, &assignee,
		&outboxEventID, &payloadMatches)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: compensated event is not authoritative", errAlertBatchPermanent)
	}
	if err != nil {
		return err
	}
	if status != "running" || revision != 2 || expectedBatchRevision != event.ExpectedBatchRevision ||
		totalCount != event.TotalCount || actionID != event.ActionID || requestedBy != event.RequestedBy ||
		reason != event.Reason || traceID != event.TraceID || assignee != event.Assignee ||
		outboxEventID != event.EventID || !payloadMatches {
		return fmt.Errorf("%w: compensated event differs from PostgreSQL authority", errAlertBatchPermanent)
	}
	rows, err := pipeline.db.QueryContext(ctx, `SELECT alert_id,position,expected_state_version,
		compensation_state_version,current_assignee,restore_assignee,current_status,restore_status
		FROM alert_assignment_compensation_items
		WHERE tenant_id=$1 AND request_id=$2 AND status='projecting' ORDER BY position`, event.TenantID, event.RequestID)
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
		return fmt.Errorf("%w: compensated event has incomplete projecting items", errAlertBatchPermanent)
	}
	for index := range authoritative {
		if authoritative[index] != event.Items[index] {
			return fmt.Errorf("%w: compensated item %d differs from PostgreSQL authority", errAlertBatchPermanent, index)
		}
	}
	return nil
}

func (pipeline *AlertBatchAssignmentPipeline) commitCompensationProjectionOutcomes(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
	event alertBatchAssignmentLifecycleEvent,
	outcomes []alertBatchProjectionOutcome,
) error {
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
	var status, actionID, requestedBy, reason, traceID string
	var revision, expectedBatchRevision int64
	var totalCount int
	if err := tx.QueryRowContext(ctx, `SELECT status,revision,expected_batch_revision,total_count,action_id,requested_by,reason,trace_id
		FROM alert_assignment_compensation_requests WHERE tenant_id=$1 AND batch_id=$2 AND request_id=$3 FOR UPDATE`,
		event.TenantID, event.BatchID, event.RequestID).Scan(&status, &revision, &expectedBatchRevision,
		&totalCount, &actionID, &requestedBy, &reason, &traceID); err != nil {
		return err
	}
	if status != "running" || revision != 2 || expectedBatchRevision != event.ExpectedBatchRevision ||
		totalCount != event.TotalCount || actionID != event.ActionID || requestedBy != event.RequestedBy ||
		reason != event.Reason || traceID != event.TraceID {
		return fmt.Errorf("%w: compensated event differs from running compensation authority", errAlertBatchPermanent)
	}
	for _, outcome := range outcomes {
		var storedStatus, restoreAssignee, restoreStatus, currentAssignee, currentStatus string
		var expectedVersion, compensationVersion, itemRevision int64
		if err := tx.QueryRowContext(ctx, `SELECT status,item_revision,expected_state_version,compensation_state_version,
			restore_assignee,restore_status,current_assignee,current_status FROM alert_assignment_compensation_items
			WHERE tenant_id=$1 AND request_id=$2 AND alert_id=$3 FOR UPDATE`, event.TenantID, event.RequestID,
			outcome.Item.AlertID).Scan(&storedStatus, &itemRevision, &expectedVersion, &compensationVersion,
			&restoreAssignee, &restoreStatus, &currentAssignee, &currentStatus); err != nil {
			return err
		}
		if storedStatus != "projecting" || itemRevision != 2 || expectedVersion != outcome.Item.ExpectedStateVersion ||
			compensationVersion != outcome.Item.ResultingStateVersion || currentAssignee != outcome.Item.PreviousAssignee ||
			restoreAssignee != outcome.Item.ResultingAssignee || currentStatus != outcome.Item.PreviousStatus ||
			restoreStatus != outcome.Item.ResultingStatus {
			return fmt.Errorf("%w: compensated item differs from PostgreSQL authority", errAlertBatchPermanent)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_compensation_items SET status=$4,item_revision=3,
			error_code=$5,error_message=$6,updated_at=$7 WHERE tenant_id=$1 AND request_id=$2 AND alert_id=$3`,
			event.TenantID, event.RequestID, outcome.Item.AlertID, outcome.Status, outcome.ErrorCode,
			outcome.ErrorMessage, now); err != nil {
			return err
		}
		itemHistoryID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-compensation-item-result-v1:"+event.EventID+":"+outcome.Item.AlertID)).String()
		detail, _ := json.Marshal(map[string]interface{}{
			"projection_event_id": event.EventID, "error_code": outcome.ErrorCode, "error_message": outcome.ErrorMessage,
		})
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_item_history
			(event_id,tenant_id,request_id,batch_id,alert_id,item_revision,previous_status,resulting_status,
			 expected_state_version,compensation_state_version,actor_id,reason,trace_id,detail,occurred_at)
			VALUES($1,$2,$3,$4,$5,3,'projecting',$6,$7,$8,$9,$10,$11,$12::jsonb,$13)`, itemHistoryID,
			event.TenantID, event.RequestID, event.BatchID, outcome.Item.AlertID, outcome.Status,
			outcome.Item.ExpectedStateVersion, outcome.Item.ResultingStateVersion, event.RequestedBy,
			event.Reason, event.TraceID, string(detail), now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_projection_receipts
			(event_id,tenant_id,request_id,batch_id,alert_id,expected_state_version,compensation_state_version,
			 restore_assignee,restore_status,outcome,error_code,error_message,source_topic,source_partition,
			 source_offset,trace_id,applied_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, event.EventID,
			event.TenantID, event.RequestID, event.BatchID, outcome.Item.AlertID, outcome.Item.ExpectedStateVersion,
			outcome.Item.ResultingStateVersion, outcome.Item.ResultingAssignee, outcome.Item.ResultingStatus,
			outcome.Status, outcome.ErrorCode, outcome.ErrorMessage, message.Topic, message.Partition,
			message.Offset, event.TraceID, now); err != nil {
			return err
		}
		projectionStatus := outcome.Status
		if projectionStatus == "compensated" {
			projectionStatus = "applied"
		}
		stateResult, err := tx.ExecContext(ctx, `UPDATE alert_assignment_states SET projection_status=$4,last_error=$5,
			updated_at=$6 WHERE tenant_id=$1 AND alert_id=$2 AND source_event_id=$3`, event.TenantID,
			outcome.Item.AlertID, event.EventID, projectionStatus, truncateAlertBatchError(outcome.ErrorMessage), now)
		if err != nil {
			return err
		}
		if affected, _ := stateResult.RowsAffected(); affected != 1 {
			return fmt.Errorf("%w: compensation projection state receipt is missing", errAlertBatchPermanent)
		}
	}
	var accepted, compensated, conflicted, failed int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE status='accepted'),
		count(*) FILTER(WHERE status='compensated'),count(*) FILTER(WHERE status='conflicted'),
		count(*) FILTER(WHERE status='failed') FROM alert_assignment_compensation_items
		WHERE tenant_id=$1 AND request_id=$2`, event.TenantID, event.RequestID).Scan(
		&accepted, &compensated, &conflicted, &failed); err != nil {
		return err
	}
	terminal := "failed"
	if compensated == totalCount {
		terminal = "completed"
	} else if compensated > 0 {
		terminal = "partial"
	}
	if accepted != 0 || compensated+conflicted+failed != totalCount {
		return fmt.Errorf("%w: compensation terminal item accounting is incomplete", errAlertBatchPermanent)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_assignment_compensation_requests SET status=$4,revision=3,
		accepted_count=0,compensated_count=$5,conflicted_count=$6,failed_count=$7,
		completed_at=$8,updated_at=$8 WHERE tenant_id=$1 AND batch_id=$2 AND request_id=$3`,
		event.TenantID, event.BatchID, event.RequestID, terminal, compensated, conflicted, failed, now); err != nil {
		return err
	}
	snapshot, _ := json.Marshal(map[string]interface{}{
		"request_id": event.RequestID, "batch_id": event.BatchID, "status": terminal, "revision": 3,
		"total_count": totalCount, "compensated_count": compensated, "conflicted_count": conflicted,
		"failed_count": failed, "trace_id": event.TraceID, "source_topic": message.Topic,
		"source_partition": message.Partition, "source_offset": message.Offset,
	})
	historyID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-batch-compensation-result-v1:"+event.EventID)).String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_assignment_compensation_history
		(event_id,tenant_id,request_id,batch_id,revision,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,$4,3,'running',$5,$6,$7,$8,$9::jsonb,$10)`, historyID, event.TenantID,
		event.RequestID, event.BatchID, terminal, event.RequestedBy, event.Reason, event.TraceID, string(snapshot), now); err != nil {
		return err
	}
	if err := insertAlertBatchCompensationPipelineAudit(ctx, tx, event,
		"ALERT_BATCH_ASSIGNMENT_COMPENSATION_TERMINAL", terminal, snapshot, now); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAlertBatchCompensationPipelineAudit(
	ctx context.Context,
	tx *sql.Tx,
	event alertBatchAssignmentLifecycleEvent,
	action, result string,
	detail []byte,
	now time.Time,
) error {
	eventID := "audit-alert-batch-compensation-" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(action+":"+event.EventID)).String()
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
		VALUES($1,$2,NULL,$3,'alert_assignment_compensation',$4,$5::jsonb,$6,$7,$8,$9)`, eventID,
		event.TenantID, action, event.RequestID, string(detail), event.TraceID, result, result != "failed", now)
	return err
}

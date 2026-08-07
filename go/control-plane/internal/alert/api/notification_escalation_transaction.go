package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type notificationEscalationMutation func(*sql.Tx) (*NotificationEscalationPolicyRecord, bool, error)

func (r *AdvancedRepository) createNotificationEscalationCommand(ctx context.Context, request *http.Request, tenantID, actor string, req notificationEscalationRequest) (*NotificationEscalationPolicyRecord, error) {
	stages, err := json.Marshal(req.Stages)
	if err != nil {
		return nil, fmt.Errorf("marshal notification escalation stages: %w", err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	mutation := func(tx *sql.Tx) (*NotificationEscalationPolicyRecord, bool, error) {
		record, scanErr := scanNotificationEscalationPolicy(tx.QueryRowContext(ctx, `INSERT INTO notification_escalation_policies
			(tenant_id,name,stages,enabled,created_by,revision) VALUES ($1,$2,$3::jsonb,$4,$5,1)
			RETURNING policy_id::text,tenant_id,name,stages,enabled,created_by,revision,created_at,updated_at`,
			tenantID, strings.TrimSpace(req.Name), string(stages), enabled, actor))
		if scanErr != nil {
			return nil, false, fmt.Errorf("create notification escalation policy: %w", scanErr)
		}
		return &record, true, nil
	}
	record, _, err := r.executeNotificationEscalationCommand(ctx, request, tenantID, actor, "created", "traffic.notification.escalation.v1.PolicyCreated", "", req, mutation)
	return record, err
}

func (r *AdvancedRepository) patchNotificationEscalationCommand(ctx context.Context, request *http.Request, tenantID, policyID, actor string, req notificationEscalationRequest) (*NotificationEscalationPolicyRecord, bool, error) {
	var stages interface{}
	if req.Stages != nil {
		encoded, err := json.Marshal(req.Stages)
		if err != nil {
			return nil, false, fmt.Errorf("marshal notification escalation stages: %w", err)
		}
		stages = string(encoded)
	}
	mutation := func(tx *sql.Tx) (*NotificationEscalationPolicyRecord, bool, error) {
		expectedRevision := int64(0)
		if req.ExpectedRevision != nil {
			expectedRevision = *req.ExpectedRevision
		}
		if expectedRevision < 1 {
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM notification_escalation_policies
				WHERE tenant_id=$1 AND policy_id::text=$2 FOR UPDATE`, tenantID, policyID).Scan(&expectedRevision); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, fmt.Errorf("load notification escalation revision: %w", err)
			}
		}
		record, scanErr := scanNotificationEscalationPolicy(tx.QueryRowContext(ctx, `UPDATE notification_escalation_policies
			SET name=COALESCE(NULLIF($3,''),name),stages=COALESCE($4::jsonb,stages),enabled=COALESCE($5,enabled),
			    revision=revision+1,updated_at=now()
			WHERE tenant_id=$1 AND policy_id::text=$2 AND revision=$6
			RETURNING policy_id::text,tenant_id,name,stages,enabled,created_by,revision,created_at,updated_at`,
			tenantID, policyID, strings.TrimSpace(req.Name), stages, req.Enabled, expectedRevision))
		if scanErr == sql.ErrNoRows {
			var current int64
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM notification_escalation_policies
				WHERE tenant_id=$1 AND policy_id::text=$2`, tenantID, policyID).Scan(&current); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, err
			}
			return nil, false, fmt.Errorf("%w: expected=%d current=%d", errNotificationRuleRevisionConflict, expectedRevision, current)
		}
		if scanErr != nil {
			return nil, false, fmt.Errorf("patch notification escalation policy: %w", scanErr)
		}
		return &record, true, nil
	}
	return r.executeNotificationEscalationCommand(ctx, request, tenantID, actor, "updated", "traffic.notification.escalation.v1.PolicyUpdated", policyID, req, mutation)
}

func (r *AdvancedRepository) executeNotificationEscalationCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor, action, eventType, aggregateHint string,
	req notificationEscalationRequest,
	mutation notificationEscalationMutation,
) (*NotificationEscalationPolicyRecord, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, fmt.Errorf("notification repository is unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	actor = strings.TrimSpace(actor)
	actionID := strings.TrimSpace(req.ActionID)
	if actionID == "" {
		actionID = uuid.NewString()
	}
	idempotencyKey := ""
	if request != nil {
		idempotencyKey = strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	}
	if idempotencyKey == "" {
		idempotencyKey = actionID
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "notification governance compatibility command"
	}
	traceID := httpx.GetTraceID(ctx)
	if strings.TrimSpace(traceID) == "" {
		traceID = "trace-" + uuid.NewString()
	}
	payload, err := json.Marshal(map[string]interface{}{"action": action, "aggregate_id": aggregateHint, "request": req})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification escalation command: %w", err)
	}
	digest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(digest[:])
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-escalation.v1:"+tenantID+":"+idempotencyKey)).String()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, fmt.Errorf("begin notification escalation command: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return nil, false, fmt.Errorf("lock notification escalation command: %w", err)
	}
	var existingHash, existingResponse, existingEventID, existingOutboxStatus string
	err = tx.QueryRowContext(ctx, `SELECT r.payload_sha256,r.response_payload::text,r.event_id::text,
		COALESCE(o.status,'pending') FROM notification_governance_requests r
		LEFT JOIN notification_governance_outbox o ON o.event_id=r.event_id
		WHERE r.tenant_id=$1 AND r.idempotency_key=$2 FOR UPDATE OF r`, tenantID, idempotencyKey).
		Scan(&existingHash, &existingResponse, &existingEventID, &existingOutboxStatus)
	if err == nil {
		if existingHash != payloadHash {
			return nil, false, errNotificationCommandConflict
		}
		var record NotificationEscalationPolicyRecord
		if err := json.Unmarshal([]byte(existingResponse), &record); err != nil {
			return nil, false, fmt.Errorf("decode notification escalation replay: %w", err)
		}
		record.EventID, record.OutboxStatus, record.IdempotentReuse = existingEventID, existingOutboxStatus, true
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit notification escalation replay: %w", err)
		}
		return &record, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("resolve notification escalation idempotency: %w", err)
	}

	record, found, err := mutation(tx)
	if err != nil || !found {
		return record, found, err
	}
	record.EventID, record.OutboxStatus, record.IdempotentReuse = eventID, "pending", false
	responsePayload, err := json.Marshal(record)
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification escalation response: %w", err)
	}
	envelope, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 1,
		"aggregate_type": "notification_escalation_policy", "aggregate_id": record.PolicyID,
		"aggregate_version": record.Revision, "tenant_id": tenantID, "escalation_policy": record,
		"action_id": actionID, "reason": reason, "changed_by": actor, "trace_id": traceID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification escalation event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_history
		(event_id,tenant_id,aggregate_type,aggregate_id,revision,action,snapshot,changed_by,reason,trace_id)
		VALUES ($1::uuid,$2,'notification_escalation_policy',$3,$4,$5,$6::jsonb,$7,$8,$9)`,
		eventID, tenantID, record.PolicyID, record.Revision, action, string(responsePayload), actor, reason, traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification escalation history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_outbox
		(event_id,aggregate_type,aggregate_id,aggregate_version,tenant_id,event_type,schema_version,partition_key,payload,trace_id)
		VALUES ($1::uuid,'notification_escalation_policy',$2,$3,$4,$5,1,$6,$7::jsonb,$8)`,
		eventID, record.PolicyID, record.Revision, tenantID, eventType, tenantID+":"+record.PolicyID, string(envelope), traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification escalation outbox: %w", err)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"actor": actor, "reason": reason, "action_id": actionID, "event_id": eventID, "revision": record.Revision, "atomic": true,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent)
		VALUES ($1,$2,NULL,$3,'notification_escalation_policy',$4,$5::jsonb,$6,$7)`,
		"audit-"+uuid.NewString(), tenantID, "NOTIFICATION_ESCALATION_"+strings.ToUpper(action), record.PolicyID,
		string(detail), requestClientIP(request), requestUserAgent(request)); err != nil {
		return nil, false, fmt.Errorf("insert notification escalation audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_requests
		(tenant_id,idempotency_key,payload_sha256,action_id,aggregate_type,aggregate_id,resulting_revision,event_id,response_payload)
		VALUES ($1,$2,$3,$4,'notification_escalation_policy',$5,$6,$7::uuid,$8::jsonb)`,
		tenantID, idempotencyKey, payloadHash, actionID, record.PolicyID, record.Revision, eventID, string(responsePayload)); err != nil {
		return nil, false, fmt.Errorf("insert notification escalation request registry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit notification escalation command: %w", err)
	}
	return record, true, nil
}

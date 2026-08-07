package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

var (
	errNotificationRuleRevisionConflict = errors.New("notification rule revision conflict")
	errNotificationCommandConflict      = errors.New("notification command idempotency conflict")
)

type notificationRuleMutation func(*sql.Tx, string, string) (*NotificationRuleRecord, bool, error)

func writeNotificationRuleCommandError(w http.ResponseWriter, ctx context.Context, err error) {
	status := http.StatusServiceUnavailable
	code := "NOTIFICATION_RULE_PERSISTENCE_FAILED"
	if errors.Is(err, errNotificationRuleRevisionConflict) || errors.Is(err, errNotificationCommandConflict) {
		status = http.StatusConflict
		code = "NOTIFICATION_RULE_CONFLICT"
	}
	httpx.JSONError(w, ctx, status, code, err.Error())
}

func (r *AdvancedRepository) createNotificationRuleCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor string,
	req notificationRuleRequest,
) (*NotificationRuleRecord, error) {
	conditions, err := json.Marshal(req.Conditions)
	if err != nil {
		return nil, fmt.Errorf("marshal notification rule conditions: %w", err)
	}
	channels, err := json.Marshal(req.Channels)
	if err != nil {
		return nil, fmt.Errorf("marshal notification rule channels: %w", err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	mutation := func(tx *sql.Tx, eventID, traceID string) (*NotificationRuleRecord, bool, error) {
		row := tx.QueryRowContext(ctx, `INSERT INTO notification_rules
			(tenant_id,name,conditions,channels,enabled,created_by,revision,trace_id)
			VALUES ($1,$2,$3::jsonb,$4::jsonb,$5,NULLIF($6,'')::uuid,1,$7)
			RETURNING rule_id::text,tenant_id,name,conditions,channels,enabled,
			          COALESCE(created_by::text,''),revision,created_at,updated_at`,
			tenantID, strings.TrimSpace(req.Name), string(conditions), string(channels), enabled,
			validNotificationUUID(actor), traceID)
		record, scanErr := scanNotificationRule(row)
		if scanErr != nil {
			return nil, false, fmt.Errorf("create notification rule: %w", scanErr)
		}
		return &record, true, nil
	}
	record, _, err := r.executeNotificationRuleCommand(
		ctx, request, tenantID, actor, "created", "traffic.notification.rule.v1.RuleCreated", "", req, mutation,
	)
	return record, err
}

func (r *AdvancedRepository) patchNotificationRuleCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, ruleID, actor string,
	req notificationRuleRequest,
) (*NotificationRuleRecord, bool, error) {
	var conditions, channels interface{}
	if req.Conditions != nil {
		encoded, err := json.Marshal(req.Conditions)
		if err != nil {
			return nil, false, fmt.Errorf("marshal notification rule conditions: %w", err)
		}
		conditions = string(encoded)
	}
	if req.Channels != nil {
		encoded, err := json.Marshal(req.Channels)
		if err != nil {
			return nil, false, fmt.Errorf("marshal notification rule channels: %w", err)
		}
		channels = string(encoded)
	}
	mutation := func(tx *sql.Tx, eventID, traceID string) (*NotificationRuleRecord, bool, error) {
		expectedRevision := int64(0)
		if req.ExpectedRevision != nil {
			expectedRevision = *req.ExpectedRevision
		}
		if expectedRevision < 1 {
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM notification_rules
				WHERE tenant_id=$1 AND rule_id::text=$2 FOR UPDATE`, tenantID, ruleID).Scan(&expectedRevision); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, fmt.Errorf("load notification rule revision: %w", err)
			}
		}
		row := tx.QueryRowContext(ctx, `UPDATE notification_rules
			SET name=COALESCE(NULLIF($3,''),name),conditions=COALESCE($4::jsonb,conditions),
			    channels=COALESCE($5::jsonb,channels),enabled=COALESCE($6,enabled),
			    revision=revision+1,trace_id=$7,updated_at=now()
			WHERE tenant_id=$1 AND rule_id::text=$2 AND revision=$8
			RETURNING rule_id::text,tenant_id,name,conditions,channels,enabled,
			          COALESCE(created_by::text,''),revision,created_at,updated_at`,
			tenantID, ruleID, strings.TrimSpace(req.Name), conditions, channels, req.Enabled,
			traceID, expectedRevision)
		record, scanErr := scanNotificationRule(row)
		if scanErr == sql.ErrNoRows {
			var current int64
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM notification_rules
				WHERE tenant_id=$1 AND rule_id::text=$2`, tenantID, ruleID).Scan(&current); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, err
			}
			return nil, false, fmt.Errorf("%w: expected=%d current=%d", errNotificationRuleRevisionConflict, expectedRevision, current)
		}
		if scanErr != nil {
			return nil, false, fmt.Errorf("patch notification rule: %w", scanErr)
		}
		return &record, true, nil
	}
	record, found, err := r.executeNotificationRuleCommand(
		ctx, request, tenantID, actor, "updated", "traffic.notification.rule.v1.RuleUpdated", ruleID, req, mutation,
	)
	return record, found, err
}

func (r *AdvancedRepository) executeNotificationRuleCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor, action, eventType, aggregateHint string,
	req notificationRuleRequest,
	mutation notificationRuleMutation,
) (*NotificationRuleRecord, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("notification repository is unavailable")
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
	payload, err := json.Marshal(map[string]interface{}{
		"action": action, "aggregate_id": aggregateHint, "request": req,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification command: %w", err)
	}
	digest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(digest[:])
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-rule.v1:"+tenantID+":"+idempotencyKey)).String()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, fmt.Errorf("begin notification rule command: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return nil, false, fmt.Errorf("lock notification rule command: %w", err)
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
		var record NotificationRuleRecord
		if err := json.Unmarshal([]byte(existingResponse), &record); err != nil {
			return nil, false, fmt.Errorf("decode notification command replay: %w", err)
		}
		record.EventID = existingEventID
		record.OutboxStatus = existingOutboxStatus
		record.IdempotentReuse = true
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit notification command replay: %w", err)
		}
		return &record, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("resolve notification command idempotency: %w", err)
	}

	record, found, err := mutation(tx, eventID, traceID)
	if err != nil || !found {
		return record, found, err
	}
	record.EventID = eventID
	record.OutboxStatus = "pending"
	record.IdempotentReuse = false
	responsePayload, err := json.Marshal(record)
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification command response: %w", err)
	}
	envelope, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 1,
		"aggregate_type": "notification_rule", "aggregate_id": record.RuleID,
		"aggregate_version": record.Revision, "tenant_id": tenantID,
		"rule": record, "action_id": actionID, "reason": reason,
		"changed_by": actor, "trace_id": traceID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification rule event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_history
		(event_id,tenant_id,aggregate_type,aggregate_id,revision,action,snapshot,changed_by,reason,trace_id)
		VALUES ($1::uuid,$2,'notification_rule',$3,$4,$5,$6::jsonb,$7,$8,$9)`,
		eventID, tenantID, record.RuleID, record.Revision, action, string(responsePayload), actor, reason, traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification rule history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_outbox
		(event_id,aggregate_type,aggregate_id,aggregate_version,tenant_id,event_type,
		 schema_version,partition_key,payload,trace_id)
		VALUES ($1::uuid,'notification_rule',$2,$3,$4,$5,1,$6,$7::jsonb,$8)`,
		eventID, record.RuleID, record.Revision, tenantID, eventType,
		tenantID+":"+record.RuleID, string(envelope), traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification rule outbox: %w", err)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"actor": actor, "reason": reason, "action_id": actionID,
		"event_id": eventID, "revision": record.Revision, "atomic": true,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent)
		VALUES ($1,$2,NULL,$3,'notification_rule',$4,$5::jsonb,$6,$7)`,
		"audit-"+uuid.NewString(), tenantID, "NOTIFICATION_RULE_"+strings.ToUpper(action),
		record.RuleID, string(detail), requestClientIP(request), requestUserAgent(request)); err != nil {
		return nil, false, fmt.Errorf("insert notification rule audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_requests
		(tenant_id,idempotency_key,payload_sha256,action_id,aggregate_type,aggregate_id,
		 resulting_revision,event_id,response_payload)
		VALUES ($1,$2,$3,$4,'notification_rule',$5,$6,$7::uuid,$8::jsonb)`,
		tenantID, idempotencyKey, payloadHash, actionID, record.RuleID, record.Revision,
		eventID, string(responsePayload)); err != nil {
		return nil, false, fmt.Errorf("insert notification rule request registry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit notification rule command: %w", err)
	}
	return record, true, nil
}

func requestClientIP(request *http.Request) string {
	if request == nil {
		return ""
	}
	return clientIP(request)
}

func requestUserAgent(request *http.Request) string {
	if request == nil {
		return ""
	}
	return request.UserAgent()
}

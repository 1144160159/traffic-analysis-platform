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

type notificationSilenceMutation func(*sql.Tx) (*NotificationSilenceRule, bool, error)

func (r *AdvancedRepository) createNotificationSilenceCommand(ctx context.Context, request *http.Request, rule NotificationSilenceRule, req notificationSilenceRuleRequest) (*NotificationSilenceRule, error) {
	if rule.RuleID == "" {
		rule.RuleID = uuid.NewString()
	}
	targets, err := json.Marshal(rule.AffectedTargets)
	if err != nil {
		return nil, fmt.Errorf("marshal silence rule targets: %w", err)
	}
	mutation := func(tx *sql.Tx) (*NotificationSilenceRule, bool, error) {
		record, scanErr := scanNotificationSilenceRule(tx.QueryRowContext(ctx, `INSERT INTO notification_silence_rules
			(rule_id,tenant_id,name,scope,starts_at,ends_at,affected_targets,policy,reason,enabled,created_by,revision)
			VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,1)
			RETURNING rule_id,tenant_id,name,scope,starts_at,ends_at,affected_targets,policy,reason,enabled,created_by,revision,created_at,updated_at`,
			rule.RuleID, rule.TenantID, rule.Name, rule.Scope, rule.StartsAt, rule.EndsAt, string(targets), rule.Policy, rule.Reason, rule.Enabled, rule.CreatedBy))
		if scanErr != nil {
			return nil, false, fmt.Errorf("create notification silence rule: %w", scanErr)
		}
		return &record, true, nil
	}
	record, _, err := r.executeNotificationSilenceCommand(ctx, request, rule.TenantID, rule.CreatedBy, "created", "traffic.notification.silence.v1.SilenceCreated", rule.RuleID, req.ActionID, req.ActionReason, req, mutation)
	return record, err
}

func (r *AdvancedRepository) patchNotificationSilenceCommand(ctx context.Context, request *http.Request, tenantID, ruleID, actor string, req notificationSilencePatchRequest) (*NotificationSilenceRule, bool, error) {
	var targets interface{}
	if req.AffectedTargets != nil {
		encoded, err := json.Marshal(*req.AffectedTargets)
		if err != nil {
			return nil, false, fmt.Errorf("marshal silence rule targets: %w", err)
		}
		targets = string(encoded)
	}
	mutation := func(tx *sql.Tx) (*NotificationSilenceRule, bool, error) {
		expectedRevision := int64(0)
		if req.ExpectedRevision != nil {
			expectedRevision = *req.ExpectedRevision
		}
		if expectedRevision < 1 {
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM notification_silence_rules
				WHERE tenant_id=$1 AND rule_id=$2 FOR UPDATE`, tenantID, ruleID).Scan(&expectedRevision); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, fmt.Errorf("load notification silence revision: %w", err)
			}
		}
		record, scanErr := scanNotificationSilenceRule(tx.QueryRowContext(ctx, `UPDATE notification_silence_rules
			SET name=COALESCE(NULLIF(BTRIM($3),''),name),scope=COALESCE($4,scope),starts_at=COALESCE($5,starts_at),
			    ends_at=COALESCE($6,ends_at),affected_targets=COALESCE($7::jsonb,affected_targets),
			    policy=COALESCE($8,policy),reason=COALESCE($9,reason),enabled=COALESCE($10,enabled),
			    revision=revision+1,updated_at=now()
			WHERE tenant_id=$1 AND rule_id=$2 AND revision=$11
			RETURNING rule_id,tenant_id,name,scope,starts_at,ends_at,affected_targets,policy,reason,enabled,created_by,revision,created_at,updated_at`,
			tenantID, ruleID, req.Name, req.Scope, req.StartsAt, req.EndsAt, targets, req.Policy, req.Reason, req.Enabled, expectedRevision))
		if scanErr == sql.ErrNoRows {
			var current int64
			if err := tx.QueryRowContext(ctx, `SELECT revision FROM notification_silence_rules WHERE tenant_id=$1 AND rule_id=$2`, tenantID, ruleID).Scan(&current); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, err
			}
			return nil, false, fmt.Errorf("%w: expected=%d current=%d", errNotificationRuleRevisionConflict, expectedRevision, current)
		}
		if scanErr != nil {
			return nil, false, fmt.Errorf("patch notification silence rule: %w", scanErr)
		}
		return &record, true, nil
	}
	return r.executeNotificationSilenceCommand(ctx, request, tenantID, actor, "updated", "traffic.notification.silence.v1.SilenceUpdated", ruleID, req.ActionID, req.ActionReason, req, mutation)
}

func (r *AdvancedRepository) executeNotificationSilenceCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor, action, eventType, aggregateHint, actionID, actionReason string,
	command interface{},
	mutation notificationSilenceMutation,
) (*NotificationSilenceRule, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, fmt.Errorf("notification repository is unavailable")
	}
	tenantID, actor = strings.TrimSpace(tenantID), strings.TrimSpace(actor)
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		actionID = uuid.NewString()
	}
	idempotencyKey := actionID
	if request != nil && strings.TrimSpace(request.Header.Get("Idempotency-Key")) != "" {
		idempotencyKey = strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	}
	actionReason = strings.TrimSpace(actionReason)
	if actionReason == "" {
		actionReason = "notification silence governance command"
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = "trace-" + uuid.NewString()
	}
	payload, err := json.Marshal(map[string]interface{}{"action": action, "aggregate_id": aggregateHint, "request": command})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification silence command: %w", err)
	}
	digest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(digest[:])
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-silence.v1:"+tenantID+":"+idempotencyKey)).String()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, fmt.Errorf("begin notification silence command: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return nil, false, fmt.Errorf("lock notification silence command: %w", err)
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
		var record NotificationSilenceRule
		if err := json.Unmarshal([]byte(existingResponse), &record); err != nil {
			return nil, false, fmt.Errorf("decode notification silence replay: %w", err)
		}
		record.EventID, record.OutboxStatus, record.IdempotentReuse = existingEventID, existingOutboxStatus, true
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit notification silence replay: %w", err)
		}
		return &record, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("resolve notification silence idempotency: %w", err)
	}
	record, found, err := mutation(tx)
	if err != nil || !found {
		return record, found, err
	}
	record.EventID, record.OutboxStatus, record.IdempotentReuse = eventID, "pending", false
	responsePayload, err := json.Marshal(record)
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification silence response: %w", err)
	}
	envelope, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 1,
		"aggregate_type": "notification_silence_rule", "aggregate_id": record.RuleID,
		"aggregate_version": record.Revision, "tenant_id": tenantID, "silence_rule": record,
		"action_id": actionID, "reason": actionReason, "changed_by": actor, "trace_id": traceID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification silence event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_history
		(event_id,tenant_id,aggregate_type,aggregate_id,revision,action,snapshot,changed_by,reason,trace_id)
		VALUES ($1::uuid,$2,'notification_silence_rule',$3,$4,$5,$6::jsonb,$7,$8,$9)`,
		eventID, tenantID, record.RuleID, record.Revision, action, string(responsePayload), actor, actionReason, traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification silence history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_outbox
		(event_id,aggregate_type,aggregate_id,aggregate_version,tenant_id,event_type,schema_version,partition_key,payload,trace_id)
		VALUES ($1::uuid,'notification_silence_rule',$2,$3,$4,$5,1,$6,$7::jsonb,$8)`,
		eventID, record.RuleID, record.Revision, tenantID, eventType, tenantID+":"+record.RuleID, string(envelope), traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification silence outbox: %w", err)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"actor": actor, "reason": actionReason, "action_id": actionID, "event_id": eventID,
		"revision": record.Revision, "atomic": true,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent)
		VALUES ($1,$2,NULL,$3,'notification_silence_rule',$4,$5::jsonb,$6,$7)`,
		"audit-"+uuid.NewString(), tenantID, "NOTIFICATION_SILENCE_RULE_"+strings.ToUpper(action), record.RuleID,
		string(detail), requestClientIP(request), requestUserAgent(request)); err != nil {
		return nil, false, fmt.Errorf("insert notification silence audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_requests
		(tenant_id,idempotency_key,payload_sha256,action_id,aggregate_type,aggregate_id,resulting_revision,event_id,response_payload)
		VALUES ($1,$2,$3,$4,'notification_silence_rule',$5,$6,$7::uuid,$8::jsonb)`,
		tenantID, idempotencyKey, payloadHash, actionID, record.RuleID, record.Revision, eventID, string(responsePayload)); err != nil {
		return nil, false, fmt.Errorf("insert notification silence request registry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit notification silence command: %w", err)
	}
	return record, true, nil
}

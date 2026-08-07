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

type notificationTemplateMutation func(*sql.Tx, string, string) (*NotificationTemplateRecord, bool, error)

func (r *AdvancedRepository) createNotificationTemplateCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor string,
	req notificationTemplateRequest,
) (*NotificationTemplateRecord, error) {
	variables, err := json.Marshal(req.VariableSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal notification template variables: %w", err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	mutation := func(tx *sql.Tx, _, _ string) (*NotificationTemplateRecord, bool, error) {
		record, scanErr := scanNotificationTemplate(tx.QueryRowContext(ctx, `INSERT INTO notification_templates
			(tenant_id,template_type,name,subject,body,variable_schema,enabled,created_by)
			VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8)
			RETURNING template_id::text,tenant_id,template_type,name,version,subject,body,
			          variable_schema,validation_status,enabled,created_by,created_at,updated_at`,
			tenantID, strings.TrimSpace(req.TemplateType), strings.TrimSpace(req.Name), req.Subject,
			req.Body, string(variables), enabled, actor))
		if scanErr != nil {
			return nil, false, fmt.Errorf("create notification template: %w", scanErr)
		}
		return &record, true, nil
	}
	record, _, err := r.executeNotificationTemplateCommand(
		ctx, request, tenantID, actor, "created", "traffic.notification.template.v1.TemplateCreated", "", req, mutation,
	)
	return record, err
}

func (r *AdvancedRepository) patchNotificationTemplateCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, templateID, actor string,
	req notificationTemplateRequest,
) (*NotificationTemplateRecord, bool, error) {
	var variables interface{}
	if req.VariableSchema != nil {
		encoded, err := json.Marshal(req.VariableSchema)
		if err != nil {
			return nil, false, fmt.Errorf("marshal notification template variables: %w", err)
		}
		variables = string(encoded)
	}
	mutation := func(tx *sql.Tx, _, _ string) (*NotificationTemplateRecord, bool, error) {
		expectedVersion := 0
		if req.ExpectedVersion != nil {
			expectedVersion = *req.ExpectedVersion
		}
		if expectedVersion < 1 {
			if err := tx.QueryRowContext(ctx, `SELECT version FROM notification_templates
				WHERE tenant_id=$1 AND template_id::text=$2 FOR UPDATE`, tenantID, templateID).Scan(&expectedVersion); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, fmt.Errorf("load notification template version: %w", err)
			}
		}
		record, scanErr := scanNotificationTemplate(tx.QueryRowContext(ctx, `UPDATE notification_templates
			SET template_type=COALESCE(NULLIF($3,''),template_type),name=COALESCE(NULLIF($4,''),name),
			    subject=CASE WHEN $5='' THEN subject ELSE $5 END,body=CASE WHEN $6='' THEN body ELSE $6 END,
			    variable_schema=COALESCE($7::jsonb,variable_schema),enabled=COALESCE($8,enabled),
			    version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND template_id::text=$2 AND version=$9
			RETURNING template_id::text,tenant_id,template_type,name,version,subject,body,
			          variable_schema,validation_status,enabled,created_by,created_at,updated_at`,
			tenantID, templateID, strings.TrimSpace(req.TemplateType), strings.TrimSpace(req.Name),
			req.Subject, req.Body, variables, req.Enabled, expectedVersion))
		if scanErr == sql.ErrNoRows {
			var current int
			if err := tx.QueryRowContext(ctx, `SELECT version FROM notification_templates
				WHERE tenant_id=$1 AND template_id::text=$2`, tenantID, templateID).Scan(&current); err != nil {
				if err == sql.ErrNoRows {
					return nil, false, nil
				}
				return nil, false, err
			}
			return nil, false, fmt.Errorf("%w: expected=%d current=%d", errNotificationRuleRevisionConflict, expectedVersion, current)
		}
		if scanErr != nil {
			return nil, false, fmt.Errorf("patch notification template: %w", scanErr)
		}
		return &record, true, nil
	}
	return r.executeNotificationTemplateCommand(
		ctx, request, tenantID, actor, "updated", "traffic.notification.template.v1.TemplateUpdated", templateID, req, mutation,
	)
}

func (r *AdvancedRepository) executeNotificationTemplateCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor, action, eventType, aggregateHint string,
	req notificationTemplateRequest,
	mutation notificationTemplateMutation,
) (*NotificationTemplateRecord, bool, error) {
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
	payload, err := json.Marshal(map[string]interface{}{
		"action": action, "aggregate_id": aggregateHint, "request": req,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification template command: %w", err)
	}
	digest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(digest[:])
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-template.v1:"+tenantID+":"+idempotencyKey)).String()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, fmt.Errorf("begin notification template command: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return nil, false, fmt.Errorf("lock notification template command: %w", err)
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
		var record NotificationTemplateRecord
		if err := json.Unmarshal([]byte(existingResponse), &record); err != nil {
			return nil, false, fmt.Errorf("decode notification template replay: %w", err)
		}
		record.EventID = existingEventID
		record.OutboxStatus = existingOutboxStatus
		record.IdempotentReuse = true
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit notification template replay: %w", err)
		}
		return &record, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("resolve notification template idempotency: %w", err)
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
		return nil, false, fmt.Errorf("marshal notification template response: %w", err)
	}
	envelope, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": eventType, "schema_version": 1,
		"aggregate_type": "notification_template", "aggregate_id": record.TemplateID,
		"aggregate_version": record.Version, "tenant_id": tenantID,
		"template": record, "action_id": actionID, "reason": reason,
		"changed_by": actor, "trace_id": traceID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshal notification template event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_history
		(event_id,tenant_id,aggregate_type,aggregate_id,revision,action,snapshot,changed_by,reason,trace_id)
		VALUES ($1::uuid,$2,'notification_template',$3,$4,$5,$6::jsonb,$7,$8,$9)`,
		eventID, tenantID, record.TemplateID, record.Version, action, string(responsePayload), actor, reason, traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification template history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_outbox
		(event_id,aggregate_type,aggregate_id,aggregate_version,tenant_id,event_type,
		 schema_version,partition_key,payload,trace_id)
		VALUES ($1::uuid,'notification_template',$2,$3,$4,$5,1,$6,$7::jsonb,$8)`,
		eventID, record.TemplateID, record.Version, tenantID, eventType,
		tenantID+":"+record.TemplateID, string(envelope), traceID); err != nil {
		return nil, false, fmt.Errorf("insert notification template outbox: %w", err)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"actor": actor, "reason": reason, "action_id": actionID,
		"event_id": eventID, "version": record.Version, "atomic": true,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent)
		VALUES ($1,$2,NULL,$3,'notification_template',$4,$5::jsonb,$6,$7)`,
		"audit-"+uuid.NewString(), tenantID, "NOTIFICATION_TEMPLATE_"+strings.ToUpper(action),
		record.TemplateID, string(detail), requestClientIP(request), requestUserAgent(request)); err != nil {
		return nil, false, fmt.Errorf("insert notification template audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_requests
		(tenant_id,idempotency_key,payload_sha256,action_id,aggregate_type,aggregate_id,
		 resulting_revision,event_id,response_payload)
		VALUES ($1,$2,$3,$4,'notification_template',$5,$6,$7::uuid,$8::jsonb)`,
		tenantID, idempotencyKey, payloadHash, actionID, record.TemplateID, record.Version,
		eventID, string(responsePayload)); err != nil {
		return nil, false, fmt.Errorf("insert notification template request registry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit notification template command: %w", err)
	}
	return record, true, nil
}

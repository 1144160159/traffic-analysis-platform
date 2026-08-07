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
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

func (r *AdvancedRepository) SaveNotificationSettingsCommand(
	ctx context.Context,
	request *http.Request,
	tenantID, actor string,
	settings map[string]interface{},
	actionID, reason string,
	expectedRevision *int64,
) (map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("notification repository is unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	actor = strings.TrimSpace(actor)
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		actionID = uuid.NewString()
	}
	idempotencyKey := actionID
	if request != nil && strings.TrimSpace(request.Header.Get("Idempotency-Key")) != "" {
		idempotencyKey = strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "update notification settings"
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = "trace-" + uuid.NewString()
	}

	storedSettings := mergeSettings(map[string]interface{}{}, settings)
	delete(storedSettings, "revision")
	delete(storedSettings, "event_id")
	delete(storedSettings, "outbox_status")
	delete(storedSettings, "idempotent_reuse")
	delete(storedSettings, "updated_at")
	commandPayload, err := json.Marshal(map[string]interface{}{
		"action": "updated", "settings": storedSettings, "action_id": actionID,
		"reason": reason, "expected_revision": expectedRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal notification settings command: %w", err)
	}
	digest := sha256.Sum256(commandPayload)
	payloadHash := hex.EncodeToString(digest[:])
	aggregateID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-settings.v1:"+tenantID)).String()
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification-settings.v1:"+tenantID+":"+idempotencyKey)).String()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin notification settings command: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return nil, fmt.Errorf("lock notification settings command: %w", err)
	}

	var existingHash, existingResponse, existingEventID, existingOutboxStatus string
	err = tx.QueryRowContext(ctx, `SELECT r.payload_sha256,r.response_payload::text,r.event_id::text,
		COALESCE(o.status,'pending') FROM notification_governance_requests r
		LEFT JOIN notification_governance_outbox o ON o.event_id=r.event_id
		WHERE r.tenant_id=$1 AND r.idempotency_key=$2 FOR UPDATE OF r`, tenantID, idempotencyKey).
		Scan(&existingHash, &existingResponse, &existingEventID, &existingOutboxStatus)
	if err == nil {
		if existingHash != payloadHash {
			return nil, errNotificationCommandConflict
		}
		var response map[string]interface{}
		if err := json.Unmarshal([]byte(existingResponse), &response); err != nil {
			return nil, fmt.Errorf("decode notification settings replay: %w", err)
		}
		response["event_id"] = existingEventID
		response["outbox_status"] = existingOutboxStatus
		response["idempotent_reuse"] = true
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit notification settings replay: %w", err)
		}
		return response, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("resolve notification settings idempotency: %w", err)
	}

	var currentRevision int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM alert_notification_settings WHERE tenant_id=$1 FOR UPDATE`, tenantID).Scan(&currentRevision)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("load notification settings revision: %w", err)
	}
	creating := err == sql.ErrNoRows
	requestedRevision := currentRevision
	if expectedRevision != nil {
		requestedRevision = *expectedRevision
	}
	if creating {
		if requestedRevision > 0 {
			return nil, fmt.Errorf("%w: expected=%d current=0", errNotificationRuleRevisionConflict, requestedRevision)
		}
	} else if expectedRevision != nil && requestedRevision != currentRevision {
		return nil, fmt.Errorf("%w: expected=%d current=%d", errNotificationRuleRevisionConflict, requestedRevision, currentRevision)
	}
	encodedSettings, err := json.Marshal(storedSettings)
	if err != nil {
		return nil, fmt.Errorf("marshal notification settings: %w", err)
	}
	var revision int64
	var updatedAt time.Time
	if creating {
		err = tx.QueryRowContext(ctx, `INSERT INTO alert_notification_settings (tenant_id,settings,revision,updated_at)
			VALUES ($1,$2::jsonb,1,now()) RETURNING revision,updated_at`, tenantID, string(encodedSettings)).Scan(&revision, &updatedAt)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE alert_notification_settings SET settings=$2::jsonb,revision=revision+1,updated_at=now()
			WHERE tenant_id=$1 AND revision=$3 RETURNING revision,updated_at`, tenantID, string(encodedSettings), currentRevision).Scan(&revision, &updatedAt)
	}
	if err != nil {
		return nil, fmt.Errorf("save notification settings: %w", err)
	}
	response := mergeSettings(map[string]interface{}{}, storedSettings)
	response["revision"] = revision
	response["event_id"] = eventID
	response["outbox_status"] = "pending"
	response["idempotent_reuse"] = false
	response["updated_at"] = updatedAt
	responsePayload, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal notification settings response: %w", err)
	}
	envelope, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "traffic.notification.settings.v1.SettingsUpdated", "schema_version": 1,
		"aggregate_type": "notification_settings", "aggregate_id": aggregateID, "aggregate_version": revision,
		"tenant_id": tenantID, "settings": response, "action_id": actionID, "reason": reason,
		"changed_by": actor, "trace_id": traceID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal notification settings event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_history
		(event_id,tenant_id,aggregate_type,aggregate_id,revision,action,snapshot,changed_by,reason,trace_id)
		VALUES ($1::uuid,$2,'notification_settings',$3,$4,'updated',$5::jsonb,$6,$7,$8)`,
		eventID, tenantID, aggregateID, revision, string(responsePayload), actor, reason, traceID); err != nil {
		return nil, fmt.Errorf("insert notification settings history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_outbox
		(event_id,aggregate_type,aggregate_id,aggregate_version,tenant_id,event_type,schema_version,partition_key,payload,trace_id)
		VALUES ($1::uuid,'notification_settings',$2,$3,$4,'traffic.notification.settings.v1.SettingsUpdated',1,$5,$6::jsonb,$7)`,
		eventID, aggregateID, revision, tenantID, tenantID+":"+aggregateID, string(envelope), traceID); err != nil {
		return nil, fmt.Errorf("insert notification settings outbox: %w", err)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"actor": actor, "reason": reason, "action_id": actionID, "event_id": eventID, "revision": revision,
		"settings": notificationSettingsAuditDetail(storedSettings), "atomic": true,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent)
		VALUES ($1,$2,NULL,'NOTIFICATION_SETTINGS_UPDATED','notification_settings',$3,$4::jsonb,$5,$6)`,
		"audit-"+uuid.NewString(), tenantID, aggregateID, string(detail), requestClientIP(request), requestUserAgent(request)); err != nil {
		return nil, fmt.Errorf("insert notification settings audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_governance_requests
		(tenant_id,idempotency_key,payload_sha256,action_id,aggregate_type,aggregate_id,resulting_revision,event_id,response_payload)
		VALUES ($1,$2,$3,$4,'notification_settings',$5,$6,$7::uuid,$8::jsonb)`,
		tenantID, idempotencyKey, payloadHash, actionID, aggregateID, revision, eventID, string(responsePayload)); err != nil {
		return nil, fmt.Errorf("insert notification settings request registry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit notification settings command: %w", err)
	}
	return response, nil
}

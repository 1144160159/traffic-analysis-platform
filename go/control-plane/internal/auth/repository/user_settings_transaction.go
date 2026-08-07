package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// UserSettingsCommand carries the stable identity and optimistic revision for one preference mutation.
type UserSettingsCommand struct {
	TenantID         string
	UserID           uuid.UUID
	Username         string
	Category         string
	Settings         map[string]interface{}
	ActionID         string
	Reason           string
	IdempotencyKey   string
	ExpectedRevision *int64
	TraceID          string
	SourceIP         string
	UserAgent        string
}

func (r *UserSettingsRepository) SaveCommand(ctx context.Context, command UserSettingsCommand) (*UserSettings, error) {
	if r == nil || r.db == nil {
		return nil, commonerrors.New(commonerrors.ErrCodeServiceUnavailable, "user settings repository is unavailable")
	}
	command.TenantID = strings.TrimSpace(command.TenantID)
	command.Category = strings.TrimSpace(command.Category)
	command.Username = strings.TrimSpace(command.Username)
	command.ActionID = strings.TrimSpace(command.ActionID)
	if command.ActionID == "" {
		command.ActionID = uuid.NewString()
	}
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.IdempotencyKey == "" {
		command.IdempotencyKey = command.ActionID
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.Reason == "" {
		command.Reason = "update user settings"
	}
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = "trace-" + uuid.NewString()
	}
	if command.TenantID == "" || command.UserID == uuid.Nil || command.Category == "" {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidParameter, "tenant_id, user_id and category are required")
	}
	if command.Settings == nil {
		command.Settings = map[string]interface{}{}
	}

	rawSettings, err := json.Marshal(command.Settings)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeInvalidParameter, "invalid user settings payload")
	}
	commandPayload, err := json.Marshal(map[string]interface{}{
		"action_id": command.ActionID, "reason": command.Reason,
		"expected_revision": command.ExpectedRevision, "settings": command.Settings,
	})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to encode user settings command")
	}
	digest := sha256.Sum256(commandPayload)
	payloadHash := hex.EncodeToString(digest[:])
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("user-settings.v1:"+command.TenantID+":"+command.UserID.String()+":"+command.Category+":"+command.IdempotencyKey)).String()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin user settings command")
	}
	defer tx.Rollback()
	lockKey := command.TenantID + ":" + command.UserID.String() + ":" + command.Category + ":" + command.IdempotencyKey
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to lock user settings command")
	}

	var existingHash, responseText, existingEventID, outboxStatus string
	err = tx.QueryRowContext(ctx, `SELECT r.payload_sha256,r.response_payload::text,r.event_id::text,
		COALESCE(o.status,'pending') FROM user_settings_requests r
		LEFT JOIN user_settings_outbox o ON o.event_id=r.event_id
		WHERE r.tenant_id=$1 AND r.user_id=$2 AND r.category=$3 AND r.idempotency_key=$4 FOR UPDATE OF r`,
		command.TenantID, command.UserID, command.Category, command.IdempotencyKey).
		Scan(&existingHash, &responseText, &existingEventID, &outboxStatus)
	if err == nil {
		if existingHash != payloadHash {
			return nil, commonerrors.New(commonerrors.ErrCodeDedupConflict, "idempotency key was already used with a different user settings payload")
		}
		var replay UserSettings
		if err := json.Unmarshal([]byte(responseText), &replay); err != nil {
			return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to decode user settings replay")
		}
		replay.EventID = existingEventID
		replay.OutboxStatus = outboxStatus
		replay.IdempotentReuse = true
		if err := tx.Commit(); err != nil {
			return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit user settings replay")
		}
		return &replay, nil
	}
	if err != sql.ErrNoRows {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to resolve user settings idempotency")
	}

	var currentRevision int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM user_settings
		WHERE tenant_id=$1 AND user_id=$2 AND category=$3 FOR UPDATE`, command.TenantID, command.UserID, command.Category).
		Scan(&currentRevision)
	if err != nil && err != sql.ErrNoRows {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to load user settings revision")
	}
	creating := err == sql.ErrNoRows
	if creating {
		if command.ExpectedRevision != nil && *command.ExpectedRevision != 0 {
			return nil, commonerrors.Newf(commonerrors.ErrCodeVersionConflict, "user settings revision conflict: expected=%d current=0", *command.ExpectedRevision)
		}
	} else if command.ExpectedRevision != nil && *command.ExpectedRevision != currentRevision {
		return nil, commonerrors.Newf(commonerrors.ErrCodeVersionConflict, "user settings revision conflict: expected=%d current=%d", *command.ExpectedRevision, currentRevision)
	}

	var saved UserSettings
	var stored json.RawMessage
	if creating {
		err = tx.QueryRowContext(ctx, `INSERT INTO user_settings
			(tenant_id,user_id,category,settings,revision,created_at,updated_at)
			VALUES ($1,$2,$3,$4::jsonb,1,now(),now())
			RETURNING tenant_id,user_id,category,settings,revision,created_at,updated_at`,
			command.TenantID, command.UserID, command.Category, string(rawSettings)).Scan(
			&saved.TenantID, &saved.UserID, &saved.Category, &stored, &saved.Revision, &saved.CreatedAt, &saved.UpdatedAt)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE user_settings SET settings=$4::jsonb,revision=revision+1,updated_at=now()
			WHERE tenant_id=$1 AND user_id=$2 AND category=$3 AND revision=$5
			RETURNING tenant_id,user_id,category,settings,revision,created_at,updated_at`,
			command.TenantID, command.UserID, command.Category, string(rawSettings), currentRevision).Scan(
			&saved.TenantID, &saved.UserID, &saved.Category, &stored, &saved.Revision, &saved.CreatedAt, &saved.UpdatedAt)
	}
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to save user settings")
	}
	if err := json.Unmarshal(stored, &saved.Settings); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to decode saved user settings")
	}
	saved.EventID, saved.OutboxStatus = eventID, "pending"
	responsePayload, err := json.Marshal(saved)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to encode user settings response")
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings_history
		(event_id,tenant_id,user_id,category,revision,action_id,reason,snapshot)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8::jsonb)`, eventID, saved.TenantID, saved.UserID,
		saved.Category, saved.Revision, command.ActionID, command.Reason, string(rawSettings)); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to append user settings history")
	}
	userEvent := map[string]interface{}{
		"event_id": eventID, "tenant_id": saved.TenantID, "user_id": saved.UserID.String(),
		"username": command.Username, "event_type": "settings_update", "source_ip": command.SourceIP,
		"user_agent": command.UserAgent, "resource": "user_settings/" + saved.Category,
		"action": "update", "result": "success", "timestamp": saved.UpdatedAt.UnixMilli(),
	}
	eventPayload, err := json.Marshal(userEvent)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to encode user settings event")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings_outbox
		(event_id,tenant_id,user_id,category,aggregate_version,event_type,schema_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,'traffic.user.settings.v1.SettingsUpdated',1,$6,$7::jsonb)`,
		eventID, saved.TenantID, saved.UserID, saved.Category, saved.Revision,
		saved.TenantID+":"+saved.UserID.String(), string(eventPayload)); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to append user settings outbox")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{
		"action_id": command.ActionID, "reason": command.Reason, "category": saved.Category,
		"revision": saved.Revision, "event_id": eventID, "sub_action": "settings_update", "atomic": true,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,request_id,trace_id,success,risk_level,result)
		VALUES ($1,$2,$3,'USER_UPDATE','user_settings',$4,$5::jsonb,$6,$7,$8,$9,true,'low','success')`,
		"audit-"+uuid.NewString(), saved.TenantID, saved.UserID.String(), saved.Category, string(auditDetail),
		command.SourceIP, command.UserAgent, command.ActionID, command.TraceID); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to append user settings audit")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings_requests
		(tenant_id,user_id,category,idempotency_key,payload_sha256,action_id,resulting_revision,event_id,response_payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::uuid,$9::jsonb)`, saved.TenantID, saved.UserID, saved.Category,
		command.IdempotencyKey, payloadHash, command.ActionID, saved.Revision, eventID, string(responsePayload)); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to register user settings request")
	}
	if err := tx.Commit(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit user settings command")
	}
	return &saved, nil
}

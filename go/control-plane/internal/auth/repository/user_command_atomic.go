package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	UserLocalCreateAction    = "auth-user-local-create"
	UserProfileUpdateAction  = "auth-user-profile-update"
	UserPasswordChangeAction = "auth-user-password-change"
	UserLoginObservedAction  = "auth-user-login-observed"
	UserOIDCSyncAction       = "auth-user-oidc-sync"
)

func (r *UserRepository) CreateLocalUserAtomic(ctx context.Context, user *model.User, password string) error {
	if user == nil || strings.TrimSpace(user.TenantID) == "" || strings.TrimSpace(user.Username) == "" || password == "" {
		return commonerrors.New(commonerrors.ErrCodeInvalidParameter, "tenant, username and password are required")
	}
	if user.UserID == uuid.Nil {
		user.UserID = uuid.New()
	}
	meta := normalizeUserCommand(UserCommandMetadata{TenantID: user.TenantID, CompatibilityMode: true},
		user.TenantID, UserLocalCreateAction, "compatibility local user creation")
	requestHash, err := userCommandHash(meta.ActionID, user.UserID, map[string]string{
		"username": user.Username, "email": user.Email, "password_sha256": digestOpaque(password)})
	if err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to hash local user command")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeInternal, "failed to hash password")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin local user command")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, user.TenantID+":username:"+user.Username); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to lock local user identity")
	}
	var priorHash, priorResponse string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_payload::text FROM user_command_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, user.TenantID, meta.IdempotencyKey).Scan(&priorHash, &priorResponse)
	if err == nil {
		if priorHash != requestHash {
			return commonerrors.New(commonerrors.ErrCodeDedupConflict, "Idempotency-Key was used for a different local user")
		}
		var result UserCommandResult
		if decodeErr := json.Unmarshal([]byte(priorResponse), &result); decodeErr != nil {
			return commonerrors.Wrap(decodeErr, commonerrors.ErrCodeSerializationError, "failed to decode local user replay")
		}
		if parsed, parseErr := uuid.Parse(result.UserID); parseErr == nil {
			user.UserID = parsed
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to inspect local user replay")
	}
	now := time.Now()
	user.PasswordHash, user.Status, user.Revision = string(passwordHash), model.UserStatusActive, 1
	user.CreatedAt, user.UpdatedAt = now, now
	var externalID interface{}
	if strings.TrimSpace(user.ExternalID) != "" {
		externalID = strings.TrimSpace(user.ExternalID)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO users
		(user_id,tenant_id,username,email,password_hash,status,external_id,last_login_at,revision,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,$9,$9)`, user.UserID, user.TenantID, user.Username, user.Email,
		user.PasswordHash, user.Status, externalID, user.LastLoginAt, now); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to create local user")
	}
	eventID := uuid.New()
	event := pb.UserEvent{EventId: eventID.String(), TenantId: user.TenantID, UserId: user.UserID.String(), Username: user.Username,
		EventType: "user_create", Resource: "user", Action: meta.ActionID, Result: "success", Timestamp: now.UnixMilli()}
	eventJSON, _ := json.Marshal(&event)
	newJSON, _ := json.Marshal(userSnapshot(user))
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_history
		(tenant_id,user_id,revision,action_id,actor_id,reason,trace_id,old_value,new_value)
		VALUES ($1,$2,1,$3,NULL,$4,$5,'{}'::jsonb,$6::jsonb)`, user.TenantID, user.UserID, meta.ActionID,
		meta.Reason, meta.TraceID, string(newJSON)); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist local user history")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_outbox
		(event_id,tenant_id,user_id,aggregate_version,event_type,schema_version,partition_key,payload)
		VALUES ($1,$2,$3,1,'traffic.user.command.v1.user_create',1,$4,$5::jsonb)`, eventID, user.TenantID,
		user.UserID, user.TenantID+":"+user.UserID.String(), string(eventJSON)); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist local user outbox")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{"action_id": meta.ActionID, "revision": 1,
		"event_id": eventID.String(), "compatibility_mode": true})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,request_id,trace_id,success,risk_level,result)
		VALUES ($1,$2,NULL,$3,'user',$4,$5::jsonb,$6,$6,true,'medium','success')`, "audit-"+eventID.String(),
		user.TenantID, meta.ActionID, user.UserID.String(), string(auditDetail), meta.TraceID); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist local user audit")
	}
	result := &UserCommandResult{UserID: user.UserID.String(), Revision: 1, EventID: eventID.String(), OutboxStatus: "pending"}
	responseJSON, _ := json.Marshal(result)
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_requests
		(request_id,tenant_id,user_id,idempotency_key,request_hash,action_id,expected_revision,resulting_revision,
		 response_payload,event_id,actor_id,reason,trace_id,compatibility_mode)
		VALUES ($1,$2,$3,$4,$5,$6,0,1,$7::jsonb,$8,NULL,$9,$10,true)`, uuid.New(), user.TenantID, user.UserID,
		meta.IdempotencyKey, requestHash, meta.ActionID, string(responseJSON), eventID, meta.Reason, meta.TraceID); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist local user request")
	}
	if err = tx.Commit(); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit local user command")
	}
	return nil
}

// UserCommandMetadata carries the immutable identity and concurrency controls
// for a user mutation. CompatibilityMode is recorded when an old client does
// not yet provide an idempotency key or expected revision.
type UserCommandMetadata struct {
	TenantID          string
	ActorID           uuid.UUID
	ActionID          string
	Reason            string
	IdempotencyKey    string
	ExpectedRevision  *int64
	TraceID           string
	SourceIP          string
	UserAgent         string
	CompatibilityMode bool
}

type UserCommandResult struct {
	UserID          string `json:"user_id,omitempty"`
	Revision        int64  `json:"revision"`
	EventID         string `json:"event_id"`
	OutboxStatus    string `json:"outbox_status"`
	IdempotentReuse bool   `json:"idempotent_reuse"`
}

func (r *UserRepository) SyncOIDCUserAtomic(ctx context.Context, claims *model.OIDCClaims, roles []string, meta UserCommandMetadata) (*model.User, *UserCommandResult, error) {
	if claims == nil || strings.TrimSpace(claims.Subject) == "" || strings.TrimSpace(meta.TenantID) == "" {
		return nil, nil, commonerrors.New(commonerrors.ErrCodeInvalidParameter, "OIDC subject and tenant are required")
	}
	meta = normalizeUserCommand(meta, meta.TenantID, UserOIDCSyncAction, "verified OIDC identity and role synchronization")
	syncRoles := roles != nil
	roles = normalizedRoleNames(roles)
	request := map[string]interface{}{"subject": claims.Subject, "email": strings.TrimSpace(claims.Email),
		"username": strings.TrimSpace(claims.PreferredUsername), "roles": roles}
	deterministicUserID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(meta.TenantID+":"+claims.Subject))
	requestHash, err := userCommandHash(meta.ActionID, deterministicUserID, request)
	if err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to hash OIDC command")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin OIDC command")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "oidc:"+claims.Subject); err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to lock OIDC identity")
	}
	var priorHash, priorResponse string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_payload::text FROM user_command_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, meta.TenantID, meta.IdempotencyKey).Scan(&priorHash, &priorResponse)
	if err == nil {
		if priorHash != requestHash {
			return nil, nil, commonerrors.New(commonerrors.ErrCodeDedupConflict, "Idempotency-Key was used for a different OIDC command")
		}
		var result UserCommandResult
		if decodeErr := json.Unmarshal([]byte(priorResponse), &result); decodeErr != nil {
			return nil, nil, commonerrors.Wrap(decodeErr, commonerrors.ErrCodeSerializationError, "failed to decode OIDC replay")
		}
		result.IdempotentReuse = true
		userID, parseErr := uuid.Parse(result.UserID)
		if parseErr != nil {
			return nil, nil, commonerrors.Wrap(parseErr, commonerrors.ErrCodeSerializationError, "invalid OIDC replay user")
		}
		user, queryErr := scanLockedUser(tx.QueryRowContext(ctx, `SELECT user_id,tenant_id,username,email,password_hash,status,
			external_id,last_login_at,revision,created_at,updated_at FROM users WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, userID))
		if queryErr != nil {
			return nil, nil, commonerrors.Wrap(queryErr, commonerrors.ErrCodeDatabaseError, "failed to load OIDC replay user")
		}
		return user, &result, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to inspect OIDC replay")
	}

	var user *model.User
	user, err = scanLockedUser(tx.QueryRowContext(ctx, `SELECT user_id,tenant_id,username,email,password_hash,status,
		external_id,last_login_at,revision,created_at,updated_at FROM users WHERE external_id=$1 FOR UPDATE`, claims.Subject))
	creating := err == sql.ErrNoRows
	if err != nil && !creating {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to inspect OIDC identity")
	}
	if !creating && user.TenantID != meta.TenantID {
		return nil, nil, commonerrors.New(commonerrors.ErrCodeUnauthorized, "OIDC identity belongs to a different tenant")
	}
	now := time.Now()
	oldValue := map[string]interface{}{}
	if creating {
		username := strings.TrimSpace(claims.PreferredUsername)
		if username == "" {
			username = strings.TrimSpace(claims.Email)
		}
		user = &model.User{UserID: deterministicUserID, TenantID: meta.TenantID, Username: username,
			Email: strings.TrimSpace(claims.Email), Status: model.UserStatusActive, ExternalID: claims.Subject,
			LastLoginAt: &now, Revision: 1, CreatedAt: now, UpdatedAt: now}
		_, err = tx.ExecContext(ctx, `INSERT INTO users
			(user_id,tenant_id,username,email,status,external_id,last_login_at,revision,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,1,$7,$7)`, user.UserID, user.TenantID, user.Username, user.Email,
			user.Status, user.ExternalID, now)
	} else {
		oldValue = userSnapshot(user)
		user.Username = strings.TrimSpace(claims.PreferredUsername)
		if user.Username == "" {
			user.Username = strings.TrimSpace(claims.Email)
		}
		user.Email, user.LastLoginAt, user.Revision, user.UpdatedAt = strings.TrimSpace(claims.Email), &now, user.Revision+1, now
		_, err = tx.ExecContext(ctx, `UPDATE users SET username=$3,email=$4,last_login_at=$5,revision=$6,updated_at=$5
			WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, user.UserID, user.Username, user.Email, now, user.Revision)
	}
	if err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist OIDC user")
	}
	if syncRoles {
		if err = syncOIDCRolesTx(ctx, tx, meta.TenantID, user.UserID, roles); err != nil {
			return nil, nil, err
		}
	}

	eventID := uuid.New()
	event := pb.UserEvent{EventId: eventID.String(), TenantId: meta.TenantID, UserId: user.UserID.String(), Username: user.Username,
		EventType: "oidc_sync", SourceIp: meta.SourceIP, UserAgent: meta.UserAgent, Resource: "user",
		Action: meta.ActionID, Result: "success", Timestamp: now.UnixMilli()}
	eventJSON, _ := json.Marshal(&event)
	newValue := userSnapshot(user)
	newValue["roles"] = roles
	oldJSON, _ := json.Marshal(oldValue)
	newJSON, _ := json.Marshal(newValue)
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_history
		(tenant_id,user_id,revision,action_id,actor_id,reason,trace_id,old_value,new_value)
		VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8::jsonb,$9::jsonb)`, meta.TenantID, user.UserID,
		user.Revision, meta.ActionID, nullableUUID(meta.ActorID), meta.Reason, meta.TraceID, string(oldJSON), string(newJSON)); err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist OIDC history")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_outbox
		(event_id,tenant_id,user_id,aggregate_version,event_type,schema_version,partition_key,payload)
		VALUES ($1,$2,$3,$4,'traffic.user.command.v1.oidc_sync',1,$5,$6::jsonb)`, eventID, meta.TenantID,
		user.UserID, user.Revision, meta.TenantID+":"+user.UserID.String(), string(eventJSON)); err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist OIDC outbox")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{"action_id": meta.ActionID, "reason": meta.Reason,
		"revision": user.Revision, "event_id": eventID.String(), "roles": roles, "created": creating})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,request_id,trace_id,success,risk_level,result)
		VALUES ($1,$2,NULLIF($3,''),$4,'user',$5,$6::jsonb,$7,$8,$9,$9,true,'medium','success')`,
		"audit-"+eventID.String(), meta.TenantID, nullableUUID(meta.ActorID), meta.ActionID, user.UserID.String(),
		string(auditDetail), meta.SourceIP, meta.UserAgent, meta.TraceID); err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist OIDC audit")
	}
	result := &UserCommandResult{UserID: user.UserID.String(), Revision: user.Revision, EventID: eventID.String(), OutboxStatus: "pending"}
	responseJSON, _ := json.Marshal(result)
	expected := int64(0)
	if !creating {
		expected = user.Revision - 1
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_requests
		(request_id,tenant_id,user_id,idempotency_key,request_hash,action_id,expected_revision,resulting_revision,
		 response_payload,event_id,actor_id,reason,trace_id,compatibility_mode)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,NULLIF($11,'')::uuid,$12,$13,$14)`, uuid.New(),
		meta.TenantID, user.UserID, meta.IdempotencyKey, requestHash, meta.ActionID, expected, user.Revision, string(responseJSON),
		eventID, nullableUUID(meta.ActorID), meta.Reason, meta.TraceID, meta.CompatibilityMode); err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist OIDC request")
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit OIDC command")
	}
	return user, result, nil
}

func normalizedRoleNames(roles []string) []string {
	seen := make(map[string]struct{}, len(roles))
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		result = append(result, role)
	}
	sort.Strings(result)
	return result
}

func syncOIDCRolesTx(ctx context.Context, tx *sql.Tx, tenantID string, userID uuid.UUID, roles []string) error {
	roleIDs := make([]uuid.UUID, 0, len(roles))
	for _, role := range roles {
		var roleID uuid.UUID
		if err := tx.QueryRowContext(ctx, `SELECT role_id FROM roles WHERE tenant_id=$1 AND name=$2`, tenantID, role).Scan(&roleID); err != nil {
			if err == sql.ErrNoRows {
				return commonerrors.Newf(commonerrors.ErrCodeEntityNotFound, "OIDC role %s is not provisioned", role)
			}
			return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to resolve OIDC role")
		}
		roleIDs = append(roleIDs, roleID)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles ur USING roles r
		WHERE ur.role_id=r.role_id AND ur.user_id=$1 AND r.tenant_id=$2
		  AND NOT (ur.role_id = ANY($3::uuid[]))`, userID, tenantID, uuidArrayLiteral(roleIDs)); err != nil {
		return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to remove stale OIDC roles")
	}
	for _, roleID := range roleIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
			return commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to add OIDC role")
		}
	}
	return nil
}

func uuidArrayLiteral(ids []uuid.UUID) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	return "{" + strings.Join(values, ",") + "}"
}

func nullableUUID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

type lockedUserMutation func(*sql.Tx, *model.User, int64) (map[string]interface{}, error)

func normalizeUserCommand(meta UserCommandMetadata, tenantID, actionID, reason string) UserCommandMetadata {
	meta.TenantID = strings.TrimSpace(meta.TenantID)
	if meta.TenantID == "" {
		meta.TenantID = strings.TrimSpace(tenantID)
	}
	meta.ActionID = strings.TrimSpace(meta.ActionID)
	if meta.ActionID == "" {
		meta.ActionID = actionID
		meta.CompatibilityMode = true
	}
	meta.Reason = strings.TrimSpace(meta.Reason)
	if meta.Reason == "" {
		meta.Reason = reason
		meta.CompatibilityMode = true
	}
	meta.TraceID = strings.TrimSpace(meta.TraceID)
	if meta.TraceID == "" {
		meta.TraceID = "legacy-" + uuid.NewString()
		meta.CompatibilityMode = true
	}
	meta.IdempotencyKey = strings.TrimSpace(meta.IdempotencyKey)
	if len(meta.IdempotencyKey) < 16 || len(meta.IdempotencyKey) > 200 {
		meta.IdempotencyKey = "legacy-" + uuid.NewString()
		meta.CompatibilityMode = true
	}
	return meta
}

func userCommandHash(actionID string, userID uuid.UUID, request interface{}) (string, error) {
	payload, err := json.Marshal(struct {
		ActionID string      `json:"action_id"`
		UserID   string      `json:"user_id"`
		Request  interface{} `json:"request"`
	}{actionID, userID.String(), request})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func scanLockedUser(row *sql.Row) (*model.User, error) {
	var user model.User
	var email, passwordHash, externalID sql.NullString
	var lastLogin sql.NullTime
	err := row.Scan(&user.UserID, &user.TenantID, &user.Username, &email, &passwordHash,
		&user.Status, &externalID, &lastLogin, &user.Revision, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if email.Valid {
		user.Email = email.String
	}
	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}
	if externalID.Valid {
		user.ExternalID = externalID.String
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func userSnapshot(user *model.User) map[string]interface{} {
	return map[string]interface{}{
		"tenant_id": user.TenantID, "user_id": user.UserID.String(), "username": user.Username,
		"email": user.Email, "status": user.Status, "external_id": user.ExternalID,
		"last_login_at": user.LastLoginAt, "revision": user.Revision,
	}
}

func (r *UserRepository) executeExistingUserCommand(
	ctx context.Context,
	userID uuid.UUID,
	meta UserCommandMetadata,
	eventType string,
	request interface{},
	mutation lockedUserMutation,
) (*UserCommandResult, error) {
	if r == nil || r.db == nil || userID == uuid.Nil || strings.TrimSpace(meta.TenantID) == "" {
		return nil, commonerrors.New(commonerrors.ErrCodeInvalidParameter, "tenant_id and user_id are required")
	}
	requestHash, err := userCommandHash(meta.ActionID, userID, request)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to hash user command")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to begin user command")
	}
	defer tx.Rollback()
	lockKey := meta.TenantID + ":user:" + userID.String()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to lock user command")
	}

	var priorHash, priorResponse string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_payload::text FROM user_command_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, meta.TenantID, meta.IdempotencyKey).
		Scan(&priorHash, &priorResponse)
	if err == nil {
		if priorHash != requestHash {
			return nil, commonerrors.New(commonerrors.ErrCodeDedupConflict, "Idempotency-Key was used for a different user command")
		}
		var result UserCommandResult
		if decodeErr := json.Unmarshal([]byte(priorResponse), &result); decodeErr != nil {
			return nil, commonerrors.Wrap(decodeErr, commonerrors.ErrCodeSerializationError, "failed to decode user command replay")
		}
		result.IdempotentReuse = true
		return &result, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to inspect user command replay")
	}

	user, err := scanLockedUser(tx.QueryRowContext(ctx, `SELECT user_id,tenant_id,username,email,password_hash,status,
		external_id,last_login_at,revision,created_at,updated_at FROM users
		WHERE tenant_id=$1 AND user_id=$2 FOR UPDATE`, meta.TenantID, userID))
	if err == sql.ErrNoRows {
		return nil, commonerrors.New(commonerrors.ErrCodeUserNotFound, "User not found in authenticated tenant")
	}
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to lock user")
	}
	if meta.ExpectedRevision != nil && *meta.ExpectedRevision != user.Revision {
		return nil, commonerrors.New(commonerrors.ErrCodeVersionConflict, "expected_revision does not match user revision")
	}
	oldValue := userSnapshot(user)
	newRevision := user.Revision + 1
	newValue, err := mutation(tx, user, newRevision)
	if err != nil {
		return nil, err
	}
	eventID := uuid.New()
	event := pb.UserEvent{EventId: eventID.String(), TenantId: meta.TenantID, UserId: userID.String(),
		Username: user.Username, EventType: eventType, SourceIp: meta.SourceIP, UserAgent: meta.UserAgent,
		Resource: "user", Action: meta.ActionID, Result: "success", Timestamp: time.Now().UnixMilli()}
	eventJSON, err := json.Marshal(&event)
	if err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeSerializationError, "failed to encode user event")
	}
	oldJSON, _ := json.Marshal(oldValue)
	newJSON, _ := json.Marshal(newValue)
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_history
		(tenant_id,user_id,revision,action_id,actor_id,reason,trace_id,old_value,new_value)
		VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8::jsonb,$9::jsonb)`,
		meta.TenantID, userID, newRevision, meta.ActionID, nullableUUID(meta.ActorID), meta.Reason, meta.TraceID, string(oldJSON), string(newJSON)); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist user history")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_outbox
		(event_id,tenant_id,user_id,aggregate_version,event_type,schema_version,partition_key,payload)
		VALUES ($1,$2,$3,$4,$5,1,$6,$7::jsonb)`, eventID, meta.TenantID, userID, newRevision,
		"traffic.user.command.v1."+eventType, meta.TenantID+":"+userID.String(), string(eventJSON)); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist user outbox")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{"action_id": meta.ActionID, "reason": meta.Reason,
		"revision": newRevision, "event_id": eventID.String(), "compatibility_mode": meta.CompatibilityMode})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent,request_id,trace_id,success,risk_level,result)
		VALUES ($1,$2,NULLIF($3,''),$4,'user',$5,$6::jsonb,$7,$8,$9,$9,true,$10,'success')`,
		"audit-"+eventID.String(), meta.TenantID, nullableUUID(meta.ActorID), meta.ActionID, userID.String(),
		string(auditDetail), meta.SourceIP, meta.UserAgent, meta.TraceID,
		map[bool]string{true: "medium", false: "low"}[eventType == "password_change"]); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist user audit")
	}
	result := &UserCommandResult{Revision: newRevision, EventID: eventID.String(), OutboxStatus: "pending"}
	responseJSON, _ := json.Marshal(result)
	expected := user.Revision
	if meta.ExpectedRevision != nil {
		expected = *meta.ExpectedRevision
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_command_requests
		(request_id,tenant_id,user_id,idempotency_key,request_hash,action_id,expected_revision,resulting_revision,
		 response_payload,event_id,actor_id,reason,trace_id,compatibility_mode)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,NULLIF($11,'')::uuid,$12,$13,$14)`,
		uuid.New(), meta.TenantID, userID, meta.IdempotencyKey, requestHash, meta.ActionID, expected, newRevision,
		string(responseJSON), eventID, nullableUUID(meta.ActorID), meta.Reason, meta.TraceID, meta.CompatibilityMode); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to persist user request")
	}
	if err = tx.Commit(); err != nil {
		return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to commit user command")
	}
	return result, nil
}

func (r *UserRepository) UpdateProfileAtomic(ctx context.Context, userID uuid.UUID, email string, meta UserCommandMetadata) (*UserCommandResult, error) {
	meta = normalizeUserCommand(meta, meta.TenantID, UserProfileUpdateAction, "self service profile update")
	email = strings.TrimSpace(email)
	return r.executeExistingUserCommand(ctx, userID, meta, "profile_update", map[string]string{"email": email},
		func(tx *sql.Tx, user *model.User, revision int64) (map[string]interface{}, error) {
			if _, err := tx.ExecContext(ctx, `UPDATE users SET email=$3,revision=$4,updated_at=now()
				WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, userID, email, revision); err != nil {
				return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to update user profile")
			}
			user.Email, user.Revision = email, revision
			return userSnapshot(user), nil
		})
}

func (r *UserRepository) ChangePasswordAtomic(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string, meta UserCommandMetadata) (*UserCommandResult, error) {
	meta = normalizeUserCommand(meta, meta.TenantID, UserPasswordChangeAction, "self service password change")
	request := map[string]interface{}{"current_password_sha256": digestOpaque(currentPassword), "new_password_sha256": digestOpaque(newPassword)}
	return r.executeExistingUserCommand(ctx, userID, meta, "password_change", request,
		func(tx *sql.Tx, user *model.User, revision int64) (map[string]interface{}, error) {
			if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
				return nil, commonerrors.New(commonerrors.ErrCodeInvalidCredentials, "Invalid current password")
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
			if err != nil {
				return nil, commonerrors.Wrap(err, commonerrors.ErrCodeInternal, "failed to hash password")
			}
			if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash=$3,revision=$4,updated_at=now()
				WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, userID, string(hash), revision); err != nil {
				return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to update password")
			}
			return map[string]interface{}{"tenant_id": meta.TenantID, "user_id": userID.String(),
				"password_changed": true, "revision": revision}, nil
		})
}

func (r *UserRepository) RecordLoginAtomic(ctx context.Context, userID uuid.UUID, tenantID string) error {
	meta := normalizeUserCommand(UserCommandMetadata{TenantID: tenantID, ActorID: userID, CompatibilityMode: true},
		tenantID, UserLoginObservedAction, "successful password login")
	_, err := r.executeExistingUserCommand(ctx, userID, meta, "login", map[string]string{"result": "success"},
		func(tx *sql.Tx, user *model.User, revision int64) (map[string]interface{}, error) {
			now := time.Now()
			if _, err := tx.ExecContext(ctx, `UPDATE users SET last_login_at=$3,revision=$4,updated_at=$3
				WHERE tenant_id=$1 AND user_id=$2`, tenantID, userID, now, revision); err != nil {
				return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to update last login")
			}
			user.LastLoginAt, user.Revision, user.UpdatedAt = &now, revision, now
			return userSnapshot(user), nil
		})
	return err
}

func (r *UserRepository) legacyUserMetadata(ctx context.Context, userID uuid.UUID, actionID, reason string) (UserCommandMetadata, error) {
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return UserCommandMetadata{}, err
	}
	if user == nil {
		return UserCommandMetadata{}, commonerrors.New(commonerrors.ErrCodeUserNotFound, "User not found")
	}
	return normalizeUserCommand(UserCommandMetadata{TenantID: user.TenantID, ActorID: userID, CompatibilityMode: true},
		user.TenantID, actionID, reason), nil
}

func (r *UserRepository) UpdateUserLegacyAtomic(ctx context.Context, user *model.User) error {
	if user == nil {
		return commonerrors.New(commonerrors.ErrCodeInvalidParameter, "user cannot be nil")
	}
	meta, err := r.legacyUserMetadata(ctx, user.UserID, "auth-user-legacy-update", "compatibility user update")
	if err != nil {
		return err
	}
	_, err = r.executeExistingUserCommand(ctx, user.UserID, meta, "profile_update",
		map[string]interface{}{"username": user.Username, "email": user.Email, "status": user.Status},
		func(tx *sql.Tx, current *model.User, revision int64) (map[string]interface{}, error) {
			if _, err := tx.ExecContext(ctx, `UPDATE users SET username=$3,email=$4,status=$5,revision=$6,updated_at=now()
				WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, user.UserID, user.Username, user.Email, user.Status, revision); err != nil {
				return nil, commonerrors.Wrap(err, commonerrors.ErrCodeDatabaseError, "failed to update user")
			}
			current.Username, current.Email, current.Status, current.Revision = user.Username, user.Email, user.Status, revision
			return userSnapshot(current), nil
		})
	return err
}

func (r *UserRepository) SetPasswordLegacyAtomic(ctx context.Context, userID uuid.UUID, newPassword string) error {
	meta, err := r.legacyUserMetadata(ctx, userID, "auth-user-legacy-password-set", "compatibility password update")
	if err != nil {
		return err
	}
	_, err = r.executeExistingUserCommand(ctx, userID, meta, "password_change",
		map[string]string{"new_password_sha256": digestOpaque(newPassword)},
		func(tx *sql.Tx, _ *model.User, revision int64) (map[string]interface{}, error) {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
			if hashErr != nil {
				return nil, commonerrors.Wrap(hashErr, commonerrors.ErrCodeInternal, "failed to hash password")
			}
			if _, execErr := tx.ExecContext(ctx, `UPDATE users SET password_hash=$3,revision=$4,updated_at=now()
				WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, userID, string(hash), revision); execErr != nil {
				return nil, commonerrors.Wrap(execErr, commonerrors.ErrCodeDatabaseError, "failed to set password")
			}
			return map[string]interface{}{"user_id": userID.String(), "password_changed": true, "revision": revision}, nil
		})
	return err
}

func (r *UserRepository) UpdateUserStatusAtomic(ctx context.Context, userID uuid.UUID, status string) error {
	meta, err := r.legacyUserMetadata(ctx, userID, "auth-user-status-update", "compatibility user status update")
	if err != nil {
		return err
	}
	_, err = r.executeExistingUserCommand(ctx, userID, meta, "status_update", map[string]string{"status": status},
		func(tx *sql.Tx, current *model.User, revision int64) (map[string]interface{}, error) {
			if _, execErr := tx.ExecContext(ctx, `UPDATE users SET status=$3,revision=$4,updated_at=now()
				WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, userID, status, revision); execErr != nil {
				return nil, commonerrors.Wrap(execErr, commonerrors.ErrCodeDatabaseError, "failed to update user status")
			}
			current.Status, current.Revision = status, revision
			return userSnapshot(current), nil
		})
	return err
}

func (r *UserRepository) AssignRoleAtomic(ctx context.Context, userID, roleID uuid.UUID, remove bool) error {
	action, eventType, reason := "auth-user-role-assign", "role_update", "compatibility role assignment"
	if remove {
		action, reason = "auth-user-role-remove", "compatibility role removal"
	}
	meta, err := r.legacyUserMetadata(ctx, userID, action, reason)
	if err != nil {
		return err
	}
	_, err = r.executeExistingUserCommand(ctx, userID, meta, eventType,
		map[string]interface{}{"role_id": roleID.String(), "remove": remove},
		func(tx *sql.Tx, current *model.User, revision int64) (map[string]interface{}, error) {
			var roleTenant string
			if queryErr := tx.QueryRowContext(ctx, `SELECT tenant_id FROM roles WHERE role_id=$1`, roleID).Scan(&roleTenant); queryErr != nil {
				if queryErr == sql.ErrNoRows {
					return nil, commonerrors.New(commonerrors.ErrCodeEntityNotFound, "Role not found")
				}
				return nil, commonerrors.Wrap(queryErr, commonerrors.ErrCodeDatabaseError, "failed to load role")
			}
			if roleTenant != meta.TenantID {
				return nil, commonerrors.New(commonerrors.ErrCodeUnauthorized, "Role belongs to a different tenant")
			}
			if remove {
				result, execErr := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=$1 AND role_id=$2`, userID, roleID)
				if execErr != nil {
					return nil, commonerrors.Wrap(execErr, commonerrors.ErrCodeDatabaseError, "failed to remove role")
				}
				if affected, _ := result.RowsAffected(); affected == 0 {
					return nil, commonerrors.New(commonerrors.ErrCodeEntityNotFound, "Role assignment not found")
				}
			} else if _, execErr := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, userID, roleID); execErr != nil {
				return nil, commonerrors.Wrap(execErr, commonerrors.ErrCodeDatabaseError, "failed to assign role")
			}
			if _, execErr := tx.ExecContext(ctx, `UPDATE users SET revision=$3,updated_at=now() WHERE tenant_id=$1 AND user_id=$2`,
				meta.TenantID, userID, revision); execErr != nil {
				return nil, commonerrors.Wrap(execErr, commonerrors.ErrCodeDatabaseError, "failed to advance role revision")
			}
			current.Revision = revision
			value := userSnapshot(current)
			value["role_id"], value["removed"] = roleID.String(), remove
			return value, nil
		})
	return err
}

func (r *UserRepository) DeleteUserAtomic(ctx context.Context, userID uuid.UUID) error {
	meta, err := r.legacyUserMetadata(ctx, userID, "auth-user-delete", "compatibility hard delete")
	if err != nil {
		return err
	}
	_, err = r.executeExistingUserCommand(ctx, userID, meta, "user_delete", map[string]string{"user_id": userID.String()},
		func(tx *sql.Tx, current *model.User, revision int64) (map[string]interface{}, error) {
			if _, execErr := tx.ExecContext(ctx, `DELETE FROM users WHERE tenant_id=$1 AND user_id=$2`, meta.TenantID, userID); execErr != nil {
				return nil, commonerrors.Wrap(execErr, commonerrors.ErrCodeDatabaseError, "failed to delete user")
			}
			return map[string]interface{}{"tenant_id": current.TenantID, "user_id": userID.String(), "deleted": true, "revision": revision}, nil
		})
	return err
}

func digestOpaque(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (m UserCommandMetadata) Validate() error {
	if strings.TrimSpace(m.TenantID) == "" || strings.TrimSpace(m.ActionID) == "" || strings.TrimSpace(m.Reason) == "" || strings.TrimSpace(m.TraceID) == "" {
		return fmt.Errorf("tenant, action, reason and trace are required")
	}
	return nil
}

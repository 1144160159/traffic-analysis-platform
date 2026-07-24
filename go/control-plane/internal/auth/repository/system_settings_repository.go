package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// SystemSettingsStore is the persistence contract used by the system settings service.
type SystemSettingsStore interface {
	GetTenant(ctx context.Context, tenantID string) (string, string, error)
	GetSettings(ctx context.Context, tenantID string) (*model.SystemSettings, int64, time.Time, error)
	SaveSettingsWithAudit(ctx context.Context, tenantID string, updatedBy uuid.UUID, expectedRevision int64, settings model.SystemSettings, auditAction string, auditDetail map[string]interface{}) (int64, time.Time, error)
	ListRoles(ctx context.Context, tenantID string) ([]model.SystemRole, error)
	GetTokenSummary(ctx context.Context, tenantID string) (model.SystemTokenSummary, error)
	InsertAudit(ctx context.Context, tenantID, userID, action string, detail map[string]interface{}) error
	RecordNavigationMiss(ctx context.Context, tenantID, userID, eventID, traceID, source string) (time.Time, string, error)
	RecordNavigationSupportRequest(ctx context.Context, tenantID, userID, eventID, traceID string) (time.Time, string, error)
}

// SystemSettingsRepository stores tenant-level governance settings in PostgreSQL.
type SystemSettingsRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewSystemSettingsRepository(db *sql.DB, logger *zap.Logger) *SystemSettingsRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SystemSettingsRepository{db: db, logger: logger}
}

func (r *SystemSettingsRepository) GetTenant(ctx context.Context, tenantID string) (string, string, error) {
	var name, status string
	err := r.db.QueryRowContext(ctx, `SELECT name, status FROM tenants WHERE tenant_id = $1`, tenantID).Scan(&name, &status)
	if err == sql.ErrNoRows {
		return "", "", errors.New(errors.ErrCodeTenantNotFound, "tenant not found")
	}
	if err != nil {
		return "", "", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to load tenant")
	}
	return name, status, nil
}

func (r *SystemSettingsRepository) GetSettings(ctx context.Context, tenantID string) (*model.SystemSettings, int64, time.Time, error) {
	var raw json.RawMessage
	var revision int64
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT settings, revision, updated_at
		FROM tenant_system_settings
		WHERE tenant_id = $1
	`, tenantID).Scan(&raw, &revision, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, 0, time.Time{}, nil
	}
	if err != nil {
		return nil, 0, time.Time{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to load system settings")
	}
	var settings model.SystemSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, 0, time.Time{}, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode system settings")
	}
	return &settings, revision, updatedAt, nil
}

func (r *SystemSettingsRepository) SaveSettingsWithAudit(
	ctx context.Context,
	tenantID string,
	updatedBy uuid.UUID,
	expectedRevision int64,
	settings model.SystemSettings,
	auditAction string,
	auditDetail map[string]interface{},
) (int64, time.Time, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return 0, time.Time{}, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode system settings")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, time.Time{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start system settings transaction")
	}
	defer func() { _ = tx.Rollback() }()

	var revision int64
	var updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		WITH updated AS (
			UPDATE tenant_system_settings
			SET settings = $2::jsonb, revision = revision + 1, updated_by = $3, updated_at = now()
			WHERE tenant_id = $1 AND revision = $4
			RETURNING revision, updated_at
		), inserted AS (
			INSERT INTO tenant_system_settings (tenant_id, settings, revision, updated_by, created_at, updated_at)
			SELECT $1, $2::jsonb, 1, $3, now(), now()
			WHERE $4 = 0 AND NOT EXISTS (SELECT 1 FROM tenant_system_settings WHERE tenant_id = $1)
			RETURNING revision, updated_at
		)
		SELECT revision, updated_at FROM updated
		UNION ALL
		SELECT revision, updated_at FROM inserted
		LIMIT 1
	`, tenantID, string(raw), updatedBy, expectedRevision).Scan(&revision, &updatedAt)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, errors.New(errors.ErrCodeVersionConflict, "system settings revision conflict")
	}
	if err != nil {
		return 0, time.Time{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to save system settings")
	}

	detail := make(map[string]interface{}, len(auditDetail)+1)
	for key, value := range auditDetail {
		detail[key] = value
	}
	detail["revision"] = revision
	auditRaw, err := json.Marshal(detail)
	if err != nil {
		return 0, time.Time{}, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode settings audit")
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, created_at)
		VALUES ($1, $2, $3, 'system_settings', $1, $4::jsonb, now())
	`, tenantID, updatedBy.String(), auditAction, string(auditRaw)); err != nil {
		return 0, time.Time{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to write settings audit")
	}
	if err = tx.Commit(); err != nil {
		return 0, time.Time{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit system settings transaction")
	}
	return revision, updatedAt, nil
}

func (r *SystemSettingsRepository) ListRoles(ctx context.Context, tenantID string) ([]model.SystemRole, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT role_id::text, name, COALESCE(description, ''), permissions, is_system
		FROM roles
		WHERE tenant_id = $1
		ORDER BY is_system DESC, name ASC
	`, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to load roles")
	}
	defer rows.Close()

	roles := make([]model.SystemRole, 0)
	for rows.Next() {
		var role model.SystemRole
		var permissions json.RawMessage
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &permissions, &role.System); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan role")
		}
		role.Permissions = decodePermissions(permissions)
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate roles")
	}
	return roles, nil
}

func (r *SystemSettingsRepository) GetTokenSummary(ctx context.Context, tenantID string) (model.SystemTokenSummary, error) {
	var summary model.SystemTokenSummary
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = 'active' AND (expires_at IS NULL OR expires_at > now()))::int,
			COUNT(*) FILTER (WHERE status = 'active' AND expires_at > now() AND expires_at <= now() + interval '7 days')::int,
			COUNT(*) FILTER (WHERE status = 'revoked')::int
		FROM api_tokens
		WHERE tenant_id = $1
	`, tenantID).Scan(&summary.Total, &summary.Active, &summary.ExpiringSoon, &summary.Revoked)
	if err != nil {
		return summary, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to load token summary")
	}
	return summary, nil
}

func (r *SystemSettingsRepository) InsertAudit(ctx context.Context, tenantID, userID, action string, detail map[string]interface{}) error {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode settings audit")
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, created_at)
		VALUES ($1, $2, $3, 'system_settings', $1, $4::jsonb, now())
	`, tenantID, userID, action, string(raw))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to write settings audit")
	}
	return nil
}

// RecordNavigationMiss persists one idempotent, path-free frontend routing miss.
// The browser supplies only an opaque event identifier and a fixed source label;
// raw URLs and internal routing details are deliberately excluded from storage.
func (r *SystemSettingsRepository) RecordNavigationMiss(
	ctx context.Context,
	tenantID, userID, eventID, traceID, source string,
) (time.Time, string, error) {
	detail, err := json.Marshal(map[string]interface{}{
		"event_id":   eventID,
		"request_id": traceID,
		"trace_id":   traceID,
		"route_kind": "unknown",
		"source":     source,
		"result":     "success",
		"risk_level": "low",
	})
	if err != nil {
		return time.Time{}, "", errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode navigation miss audit")
	}
	var createdAt time.Time
	var persistedTraceID string
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO audit_logs (
			event_id, tenant_id, user_id, action, object_type, object_id,
			detail, request_id, trace_id, success, risk_level, result, created_at
		)
		VALUES ($1, $2, $3, 'navigation_not_found', 'frontend_route', $1,
			$4::jsonb, $5, $5, true, 'low', 'success', now())
		ON CONFLICT (event_id) DO UPDATE SET event_id = EXCLUDED.event_id
		WHERE audit_logs.tenant_id = EXCLUDED.tenant_id
		  AND audit_logs.user_id::text = EXCLUDED.user_id::text
		  AND audit_logs.action = EXCLUDED.action
		RETURNING created_at, trace_id
	`, eventID, tenantID, userID, string(detail), traceID).Scan(&createdAt, &persistedTraceID)
	if err == sql.ErrNoRows {
		return time.Time{}, "", errors.New(errors.ErrCodeDedupConflict, "navigation event belongs to another authenticated principal")
	}
	if err != nil {
		return time.Time{}, "", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist navigation miss audit")
	}
	return createdAt, persistedTraceID, nil
}

// RecordNavigationSupportRequest creates one observable, idempotent support
// request for a navigation miss owned by the current tenant and user.
func (r *SystemSettingsRepository) RecordNavigationSupportRequest(
	ctx context.Context,
	tenantID, userID, eventID, traceID string,
) (time.Time, string, error) {
	supportID := "support-" + strings.TrimPrefix(eventID, "nav-")
	detail, err := json.Marshal(map[string]interface{}{
		"navigation_event_id": eventID,
		"request_id":          traceID,
		"trace_id":            traceID,
		"queue":               "platform-duty-admin",
		"status":              "queued",
		"result":              "success",
		"risk_level":          "low",
	})
	if err != nil {
		return time.Time{}, "", errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode navigation support audit")
	}
	var createdAt time.Time
	var persistedTraceID string
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO audit_logs (
			event_id, tenant_id, user_id, action, object_type, object_id,
			detail, request_id, trace_id, success, risk_level, result, created_at
		)
		SELECT $1, $2, $3::uuid, 'navigation_support_requested', 'frontend_route', $4,
			$5::jsonb, $6, $6, true, 'low', 'success', now()
		WHERE EXISTS (
			SELECT 1 FROM audit_logs
			WHERE event_id = $4 AND tenant_id = $2 AND user_id = $3::uuid
			  AND action = 'navigation_not_found'
		)
		ON CONFLICT (event_id) DO UPDATE SET event_id = EXCLUDED.event_id
		WHERE audit_logs.tenant_id = EXCLUDED.tenant_id
		  AND audit_logs.user_id::text = EXCLUDED.user_id::text
		  AND audit_logs.action = EXCLUDED.action
		RETURNING created_at, trace_id
	`, supportID, tenantID, userID, eventID, string(detail), traceID).Scan(&createdAt, &persistedTraceID)
	if err == sql.ErrNoRows {
		return time.Time{}, "", errors.New(errors.ErrCodeDedupConflict, "navigation event is unavailable for this authenticated principal")
	}
	if err != nil {
		return time.Time{}, "", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist navigation support request")
	}
	return createdAt, persistedTraceID, nil
}

func decodePermissions(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return uniqueSorted(list)
	}
	var object map[string]interface{}
	if json.Unmarshal(raw, &object) != nil {
		return []string{}
	}
	for key, value := range object {
		if allowed, ok := value.(bool); ok && !allowed {
			continue
		}
		if text, ok := value.(string); ok && text != "" && text != "*" && !strings.Contains(key, ":") {
			list = append(list, key+":"+text)
		} else {
			list = append(list, key)
		}
	}
	return uniqueSorted(list)
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

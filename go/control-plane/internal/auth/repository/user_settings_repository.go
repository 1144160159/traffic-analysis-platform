package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// UserSettings 保存单个用户某一类偏好设置。
type UserSettings struct {
	TenantID        string                 `json:"tenant_id"`
	UserID          uuid.UUID              `json:"user_id"`
	Category        string                 `json:"category"`
	Settings        map[string]interface{} `json:"settings"`
	Revision        int64                  `json:"revision"`
	EventID         string                 `json:"event_id,omitempty"`
	OutboxStatus    string                 `json:"outbox_status,omitempty"`
	IdempotentReuse bool                   `json:"idempotent_reuse,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// UserSettingsRepository 管理用户偏好设置。
type UserSettingsRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewUserSettingsRepository(db *sql.DB, logger *zap.Logger) *UserSettingsRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UserSettingsRepository{db: db, logger: logger}
}

func (r *UserSettingsRepository) Get(ctx context.Context, tenantID string, userID uuid.UUID, category string) (*UserSettings, error) {
	query := `
		SELECT tenant_id, user_id, category, settings, revision, created_at, updated_at
		FROM user_settings
		WHERE tenant_id = $1 AND user_id = $2 AND category = $3
	`

	var settings UserSettings
	var raw json.RawMessage
	if err := r.db.QueryRowContext(ctx, query, tenantID, userID, category).Scan(
		&settings.TenantID,
		&settings.UserID,
		&settings.Category,
		&raw,
		&settings.Revision,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get user settings",
			zap.String("tenant_id", tenantID),
			zap.String("user_id", userID.String()),
			zap.String("category", category),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get user settings")
	}

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings.Settings); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to decode user settings")
		}
	}
	if settings.Settings == nil {
		settings.Settings = map[string]interface{}{}
	}

	return &settings, nil
}

func (r *UserSettingsRepository) Upsert(ctx context.Context, tenantID string, userID uuid.UUID, category string, values map[string]interface{}) (*UserSettings, error) {
	actionID := uuid.NewString()
	return r.SaveCommand(ctx, UserSettingsCommand{
		TenantID: tenantID, UserID: userID, Category: category, Settings: values,
		ActionID: actionID, IdempotencyKey: actionID, Reason: "update user settings",
	})
}

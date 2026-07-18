package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/playbook"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type AdvancedRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

type PlaybookExecutionRecord struct {
	ExecutionID    string                 `json:"execution_id"`
	TenantID       string                 `json:"tenant_id"`
	PlaybookName   string                 `json:"playbook"`
	AlertID        string                 `json:"alert_id"`
	SuccessActions int                    `json:"success_actions"`
	FailedActions  int                    `json:"failed_actions"`
	DurationMS     int64                  `json:"duration_ms"`
	RequestPayload map[string]interface{} `json:"request_payload"`
	Result         map[string]interface{} `json:"result"`
	CreatedAt      time.Time              `json:"created_at"`
}

type PlaybookOverride struct {
	TenantID        string        `json:"tenant_id"`
	Name            string        `json:"name"`
	Enabled         bool          `json:"enabled"`
	MaxRuns         int           `json:"max_runs"`
	Cooldown        time.Duration `json:"cooldown"`
	CooldownSeconds int64         `json:"cooldown_seconds"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type NotificationSilenceRule struct {
	RuleID          string    `json:"rule_id"`
	TenantID        string    `json:"tenant_id"`
	Name            string    `json:"name"`
	Scope           string    `json:"scope"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	AffectedTargets []string  `json:"affected_targets"`
	Policy          string    `json:"policy"`
	Reason          string    `json:"reason"`
	Enabled         bool      `json:"enabled"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DataQualityActionRecord struct {
	ActionID    string                 `json:"action_id"`
	TenantID    string                 `json:"tenant_id"`
	View        string                 `json:"view"`
	Action      string                 `json:"action"`
	Target      string                 `json:"target"`
	DryRun      bool                   `json:"dry_run"`
	Status      string                 `json:"status"`
	RequestedBy string                 `json:"requested_by"`
	Request     map[string]interface{} `json:"request"`
	CreatedAt   time.Time              `json:"created_at"`
}

type DataQualityUIFixture struct {
	TenantID       string                 `json:"tenant_id"`
	FixtureVersion string                 `json:"fixture_version"`
	Payload        map[string]interface{} `json:"payload"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type DataQualityTablePage struct {
	TenantID       string        `json:"tenant_id"`
	FixtureVersion string        `json:"fixture_version"`
	Dataset        string        `json:"dataset"`
	Items          []interface{} `json:"items"`
	Total          int           `json:"total"`
	Page           int           `json:"page"`
	PageSize       int           `json:"page_size"`
}

func NewAdvancedRepository(db *sql.DB, logger *zap.Logger) *AdvancedRepository {
	return &AdvancedRepository{db: db, logger: logger}
}

func (r *AdvancedRepository) InitSchema(ctx context.Context) error {
	if r == nil || r.db == nil {
		return nil
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS alert_playbook_executions (
			execution_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			playbook_name TEXT NOT NULL,
			alert_id TEXT NOT NULL,
			success_actions INTEGER NOT NULL DEFAULT 0,
			failed_actions INTEGER NOT NULL DEFAULT 0,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_playbook_executions_tenant_created
			ON alert_playbook_executions (tenant_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS alert_playbook_overrides (
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			max_runs INTEGER NOT NULL DEFAULT 0,
			cooldown_seconds BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (tenant_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS alert_notification_settings (
			tenant_id TEXT PRIMARY KEY,
			settings JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS notification_silence_rules (
			rule_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT '',
			starts_at TIMESTAMPTZ NOT NULL,
			ends_at TIMESTAMPTZ NOT NULL,
			affected_targets JSONB NOT NULL DEFAULT '[]'::jsonb,
			policy TEXT NOT NULL DEFAULT 'all',
			reason TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS data_quality_actions (
			action_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			view_name TEXT NOT NULL,
			action_name TEXT NOT NULL,
			target TEXT NOT NULL,
			dry_run BOOLEAN NOT NULL DEFAULT TRUE,
			status TEXT NOT NULL DEFAULT 'dry_run',
			requested_by TEXT NOT NULL DEFAULT '',
			request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_data_quality_actions_tenant_created
			ON data_quality_actions (tenant_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS data_quality_ui_fixtures (
			tenant_id TEXT PRIMARY KEY,
			fixture_version TEXT NOT NULL,
			payload JSONB NOT NULL,
			active BOOLEAN NOT NULL DEFAULT false,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_data_quality_ui_fixtures_active
			ON data_quality_ui_fixtures (tenant_id, active)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_time
			ON notification_silence_rules (tenant_id, starts_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_enabled
			ON notification_silence_rules (tenant_id, enabled, starts_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("init advanced API schema: %w", err)
		}
	}
	return nil
}

func (r *AdvancedRepository) SavePlaybookExecution(
	ctx context.Context,
	tenantID string,
	alert *playbook.AlertContext,
	result *playbook.ExecutionResult,
) (*PlaybookExecutionRecord, error) {
	if r == nil || r.db == nil || alert == nil || result == nil {
		return nil, nil
	}

	requestPayload, err := json.Marshal(alert)
	if err != nil {
		return nil, fmt.Errorf("marshal playbook request payload: %w", err)
	}
	resultPayload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal playbook result payload: %w", err)
	}

	record := &PlaybookExecutionRecord{
		ExecutionID:    uuid.NewString(),
		TenantID:       tenantID,
		PlaybookName:   result.PlaybookName,
		AlertID:        result.AlertID,
		SuccessActions: result.SuccessActions,
		FailedActions:  result.FailedActions,
		DurationMS:     result.Duration.Milliseconds(),
		CreatedAt:      time.Now(),
		RequestPayload: decodeJSONMap(requestPayload),
		Result:         decodeJSONMap(resultPayload),
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO alert_playbook_executions (
			execution_id, tenant_id, playbook_name, alert_id,
			success_actions, failed_actions, duration_ms,
			request_payload, result_payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10)
	`, record.ExecutionID, record.TenantID, record.PlaybookName, record.AlertID,
		record.SuccessActions, record.FailedActions, record.DurationMS,
		string(requestPayload), string(resultPayload), record.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("save playbook execution: %w", err)
	}

	return record, nil
}

func (r *AdvancedRepository) ListPlaybookExecutions(ctx context.Context, tenantID string, limit int) ([]PlaybookExecutionRecord, error) {
	if r == nil || r.db == nil {
		return []PlaybookExecutionRecord{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT execution_id, tenant_id, playbook_name, alert_id,
			success_actions, failed_actions, duration_ms,
			request_payload, result_payload, created_at
		FROM alert_playbook_executions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list playbook executions: %w", err)
	}
	defer rows.Close()

	records := make([]PlaybookExecutionRecord, 0)
	for rows.Next() {
		var record PlaybookExecutionRecord
		var requestPayload, resultPayload []byte
		if err := rows.Scan(
			&record.ExecutionID,
			&record.TenantID,
			&record.PlaybookName,
			&record.AlertID,
			&record.SuccessActions,
			&record.FailedActions,
			&record.DurationMS,
			&requestPayload,
			&resultPayload,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan playbook execution: %w", err)
		}
		record.RequestPayload = decodeJSONMap(requestPayload)
		record.Result = decodeJSONMap(resultPayload)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playbook executions: %w", err)
	}
	return records, nil
}

func (r *AdvancedRepository) SavePlaybookOverride(ctx context.Context, tenantID string, pb *playbook.Playbook) error {
	if r == nil || r.db == nil || pb == nil {
		return nil
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO alert_playbook_overrides (
			tenant_id, name, enabled, max_runs, cooldown_seconds, updated_at
		) VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			max_runs = EXCLUDED.max_runs,
			cooldown_seconds = EXCLUDED.cooldown_seconds,
			updated_at = now()
	`, tenantID, pb.Name, pb.Enabled, pb.MaxRuns, int64(pb.Cooldown.Seconds()))
	if err != nil {
		return fmt.Errorf("save playbook override: %w", err)
	}
	return nil
}

func (r *AdvancedRepository) ListPlaybookOverrides(ctx context.Context, tenantID string) ([]PlaybookOverride, error) {
	if r == nil || r.db == nil {
		return []PlaybookOverride{}, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT tenant_id, name, enabled, max_runs, cooldown_seconds, updated_at
		FROM alert_playbook_overrides
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list playbook overrides: %w", err)
	}
	defer rows.Close()

	overrides := make([]PlaybookOverride, 0)
	for rows.Next() {
		var override PlaybookOverride
		if err := rows.Scan(
			&override.TenantID,
			&override.Name,
			&override.Enabled,
			&override.MaxRuns,
			&override.CooldownSeconds,
			&override.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan playbook override: %w", err)
		}
		override.Cooldown = time.Duration(override.CooldownSeconds) * time.Second
		overrides = append(overrides, override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playbook overrides: %w", err)
	}
	return overrides, nil
}

func (r *AdvancedRepository) GetNotificationSettings(ctx context.Context, tenantID string) (map[string]interface{}, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}

	var settingsBytes []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT settings
		FROM alert_notification_settings
		WHERE tenant_id = $1
	`, tenantID).Scan(&settingsBytes)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get notification settings: %w", err)
	}
	return decodeJSONMap(settingsBytes), true, nil
}

func (r *AdvancedRepository) SaveNotificationSettings(ctx context.Context, tenantID string, settings map[string]interface{}) error {
	if r == nil || r.db == nil {
		return nil
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal notification settings: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO alert_notification_settings (tenant_id, settings, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (tenant_id) DO UPDATE SET
			settings = EXCLUDED.settings,
			updated_at = now()
	`, tenantID, string(payload))
	if err != nil {
		return fmt.Errorf("save notification settings: %w", err)
	}
	return nil
}

func (r *AdvancedRepository) ListNotificationSilenceRules(ctx context.Context, tenantID string, limit int) ([]NotificationSilenceRule, error) {
	if r == nil || r.db == nil {
		return []NotificationSilenceRule{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT rule_id, tenant_id, name, scope, starts_at, ends_at,
			affected_targets, policy, reason, enabled, created_by, created_at, updated_at
		FROM notification_silence_rules
		WHERE tenant_id = $1
		ORDER BY starts_at DESC, updated_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification silence rules: %w", err)
	}
	defer rows.Close()

	rules := make([]NotificationSilenceRule, 0)
	for rows.Next() {
		rule, err := scanNotificationSilenceRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification silence rules: %w", err)
	}
	return rules, nil
}

func (r *AdvancedRepository) CreateNotificationSilenceRule(ctx context.Context, rule NotificationSilenceRule) (*NotificationSilenceRule, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if rule.RuleID == "" {
		rule.RuleID = uuid.NewString()
	}
	targets, err := json.Marshal(rule.AffectedTargets)
	if err != nil {
		return nil, fmt.Errorf("marshal silence rule targets: %w", err)
	}

	row := r.db.QueryRowContext(ctx, `
		INSERT INTO notification_silence_rules (
			rule_id, tenant_id, name, scope, starts_at, ends_at,
			affected_targets, policy, reason, enabled, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, now(), now())
		RETURNING rule_id, tenant_id, name, scope, starts_at, ends_at,
			affected_targets, policy, reason, enabled, created_by, created_at, updated_at
	`, rule.RuleID, rule.TenantID, rule.Name, rule.Scope, rule.StartsAt, rule.EndsAt,
		string(targets), rule.Policy, rule.Reason, rule.Enabled, rule.CreatedBy)
	created, err := scanNotificationSilenceRule(row)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *AdvancedRepository) SetNotificationSilenceRuleEnabled(
	ctx context.Context,
	tenantID string,
	ruleID string,
	enabled bool,
) (*NotificationSilenceRule, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}

	row := r.db.QueryRowContext(ctx, `
		UPDATE notification_silence_rules
		SET enabled = $3, updated_at = now()
		WHERE tenant_id = $1 AND rule_id = $2
		RETURNING rule_id, tenant_id, name, scope, starts_at, ends_at,
			affected_targets, policy, reason, enabled, created_by, created_at, updated_at
	`, tenantID, ruleID, enabled)
	rule, err := scanNotificationSilenceRule(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &rule, true, nil
}

func (r *AdvancedRepository) RecordAuditLog(
	ctx context.Context,
	req *http.Request,
	tenantID string,
	userID string,
	action string,
	objectType string,
	objectID string,
	detail map[string]interface{},
) error {
	if r == nil || r.db == nil {
		return nil
	}
	if tenantID == "" {
		tenantID = httpx.GetTenantID(ctx)
	}
	if userID == "" {
		userID = httpx.GetUserID(ctx)
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}

	userIDExpr := "NULLIF($3, '')"
	if r.pgColumnType(ctx, "audit_logs", "user_id") == "uuid" {
		userIDExpr = "NULLIF($3, '')::uuid"
		if userID != "" {
			if _, err := uuid.Parse(userID); err != nil {
				userID = ""
			}
		}
	}

	ip := ""
	userAgent := ""
	if req != nil {
		ip = clientIP(req)
		userAgent = req.UserAgent()
	}
	if r.pgColumnExists(ctx, "audit_logs", "event_id") {
		query := `INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, ` + userIDExpr + `, $4, $5, $6, $7::jsonb, $8, $9)`
		_, err = r.db.ExecContext(ctx, query,
			"audit-"+uuid.NewString(),
			tenantID,
			userID,
			action,
			objectType,
			objectID,
			string(detailJSON),
			ip,
			userAgent)
		return err
	}

	query := `INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, ` + strings.Replace(userIDExpr, "$3", "$2", 1) + `, $3, $4, $5, $6::jsonb, $7, $8)`
	_, err = r.db.ExecContext(ctx, query,
		tenantID,
		userID,
		action,
		objectType,
		objectID,
		string(detailJSON),
		ip,
		userAgent)
	return err
}

func (r *AdvancedRepository) CreateDataQualityAction(
	ctx context.Context,
	req *http.Request,
	record DataQualityActionRecord,
) (*DataQualityActionRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("data quality action repository is not available")
	}
	if record.ActionID == "" {
		record.ActionID = uuid.NewString()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(record.Request)
	if err != nil {
		return nil, fmt.Errorf("marshal data quality action: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin data quality action: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data_quality_actions (
			action_id, tenant_id, view_name, action_name, target,
			dry_run, status, requested_by, request_payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
	`, record.ActionID, record.TenantID, record.View, record.Action, record.Target,
		record.DryRun, record.Status, record.RequestedBy, string(payload), record.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert data quality action: %w", err)
	}

	var userID interface{}
	if parsed, parseErr := uuid.Parse(record.RequestedBy); parseErr == nil {
		userID = parsed
	}
	detail := map[string]interface{}{
		"view": record.View, "action": record.Action, "target": record.Target,
		"dry_run": record.DryRun, "status": record.Status,
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return nil, fmt.Errorf("marshal data quality audit: %w", err)
	}
	ip, userAgent := "", ""
	if req != nil {
		ip, userAgent = clientIP(req), req.UserAgent()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			event_id, tenant_id, user_id, action, object_type, object_id,
			detail, ip_addr, user_agent, created_at
		) VALUES ($1, $2, $3, 'DATA_QUALITY_ACTION_REQUESTED', 'data_quality_action', $4, $5::jsonb, $6, $7, $8)
	`, "audit-"+uuid.NewString(), record.TenantID, userID, record.ActionID, string(detailJSON), ip, userAgent, record.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert data quality action audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit data quality action: %w", err)
	}
	return &record, nil
}

func (r *AdvancedRepository) GetDataQualityUIFixture(ctx context.Context, tenantID string) (*DataQualityUIFixture, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}
	var fixture DataQualityUIFixture
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, fixture_version, payload, updated_at
		FROM data_quality_ui_fixtures
		WHERE tenant_id = $1 AND active = TRUE
	`, tenantID).Scan(&fixture.TenantID, &fixture.FixtureVersion, &payload, &fixture.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load data quality UI fixture: %w", err)
	}
	if err := json.Unmarshal(payload, &fixture.Payload); err != nil {
		return nil, false, fmt.Errorf("decode data quality UI fixture: %w", err)
	}
	return &fixture, true, nil
}

func (r *AdvancedRepository) GetDataQualityTablePage(
	ctx context.Context,
	tenantID string,
	dataset string,
	page int,
	pageSize int,
) (*DataQualityTablePage, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}
	offset := (page - 1) * pageSize
	var result DataQualityTablePage
	var items []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT
			tenant_id,
			fixture_version,
			COALESCE(jsonb_array_length(payload -> $2), 0) AS total,
			COALESCE((
				SELECT jsonb_agg(entry.value ORDER BY entry.ordinality)
				FROM jsonb_array_elements(payload -> $2) WITH ORDINALITY AS entry(value, ordinality)
				WHERE entry.ordinality > $3 AND entry.ordinality <= $3 + $4
			), '[]'::jsonb) AS items
		FROM data_quality_ui_fixtures
		WHERE tenant_id = $1
		  AND active = TRUE
		  AND jsonb_typeof(payload -> $2) = 'array'
	`, tenantID, dataset, offset, pageSize).Scan(
		&result.TenantID,
		&result.FixtureVersion,
		&result.Total,
		&items,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load data quality table page: %w", err)
	}
	if err := json.Unmarshal(items, &result.Items); err != nil {
		return nil, false, fmt.Errorf("decode data quality table page: %w", err)
	}
	result.Dataset = dataset
	result.Page = page
	result.PageSize = pageSize
	return &result, true, nil
}

func decodeJSONMap(payload []byte) map[string]interface{} {
	if len(payload) == 0 {
		return map[string]interface{}{}
	}
	var value map[string]interface{}
	if err := json.Unmarshal(payload, &value); err != nil {
		return map[string]interface{}{}
	}
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

type notificationSilenceScanner interface {
	Scan(dest ...interface{}) error
}

func scanNotificationSilenceRule(scanner notificationSilenceScanner) (NotificationSilenceRule, error) {
	var rule NotificationSilenceRule
	var targets []byte
	if err := scanner.Scan(
		&rule.RuleID,
		&rule.TenantID,
		&rule.Name,
		&rule.Scope,
		&rule.StartsAt,
		&rule.EndsAt,
		&targets,
		&rule.Policy,
		&rule.Reason,
		&rule.Enabled,
		&rule.CreatedBy,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return rule, err
	}
	if len(targets) > 0 {
		_ = json.Unmarshal(targets, &rule.AffectedTargets)
	}
	if rule.AffectedTargets == nil {
		rule.AffectedTargets = []string{}
	}
	return rule, nil
}

func (r *AdvancedRepository) pgColumnExists(ctx context.Context, tableName, columnName string) bool {
	if r == nil || r.db == nil {
		return false
	}
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, tableName, columnName).Scan(&exists)
	if err != nil && r.logger != nil {
		r.logger.Debug("Failed to inspect advanced column existence", zap.Error(err))
	}
	return err == nil && exists
}

func (r *AdvancedRepository) pgColumnType(ctx context.Context, tableName, columnName string) string {
	if r == nil || r.db == nil {
		return ""
	}
	var dataType string
	err := r.db.QueryRowContext(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2
		ORDER BY CASE WHEN table_schema = 'public' THEN 0 ELSE 1 END
		LIMIT 1`, tableName, columnName).Scan(&dataType)
	if err != nil && r.logger != nil {
		r.logger.Debug("Failed to inspect advanced column type", zap.Error(err))
	}
	return dataType
}

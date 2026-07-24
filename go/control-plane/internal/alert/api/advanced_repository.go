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

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/playbook"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type AdvancedRepository struct {
	db                        *sql.DB
	logger                    *zap.Logger
	notificationStateResolver func(context.Context, string, string) (*notification.AlertInfo, string, error)
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
	Mode           string                 `json:"mode"`
	Status         string                 `json:"status"`
	RollbackOf     string                 `json:"rollback_of,omitempty"`
	Effect         map[string]interface{} `json:"effect"`
	RequestedBy    string                 `json:"requested_by"`
	RolledBackAt   *time.Time             `json:"rolled_back_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

type PlaybookDefinitionRecord struct {
	TenantID        string                 `json:"tenant_id"`
	Name            string                 `json:"name"`
	DisplayName     string                 `json:"display_name"`
	Description     string                 `json:"description"`
	Version         int                    `json:"version"`
	Stage           string                 `json:"stage"`
	Enabled         bool                   `json:"enabled"`
	RiskLevel       string                 `json:"risk_level"`
	Definition      map[string]interface{} `json:"definition"`
	CreatedBy       string                 `json:"created_by"`
	SubmittedBy     string                 `json:"submitted_by,omitempty"`
	ApprovedBy      string                 `json:"approved_by,omitempty"`
	RejectionReason string                 `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type PlaybookAuditRecord struct {
	EventID   string                 `json:"event_id"`
	Action    string                 `json:"action"`
	ObjectID  string                 `json:"object_id"`
	Detail    map[string]interface{} `json:"detail"`
	CreatedAt time.Time              `json:"created_at"`
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

// SetNotificationAlertStateResolver supplies the live alert lifecycle state
// used by the escalation worker immediately before delivery.
func (r *AdvancedRepository) SetNotificationAlertStateResolver(resolver func(context.Context, string, string) (*notification.AlertInfo, string, error)) {
	if r != nil {
		r.notificationStateResolver = resolver
	}
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
			mode TEXT NOT NULL DEFAULT 'legacy',
			status TEXT NOT NULL DEFAULT 'succeeded',
			rollback_of TEXT,
			effect_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			requested_by TEXT NOT NULL DEFAULT '',
			rolled_back_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'legacy'`,
		`ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'succeeded'`,
		`ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS rollback_of TEXT`,
		`ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS effect_payload JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS requested_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS rolled_back_at TIMESTAMPTZ`,
		`DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_mode_check CHECK (mode IN ('legacy', 'drill')); EXCEPTION WHEN duplicate_object THEN NULL; END $do$`,
		`DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_status_check CHECK (status IN ('succeeded', 'failed', 'rolled_back', 'rollback_recorded')); EXCEPTION WHEN duplicate_object THEN NULL; END $do$`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_playbook_executions_tenant_id ON alert_playbook_executions (tenant_id, execution_id)`,
		`DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_rollback_fk FOREIGN KEY (tenant_id, rollback_of) REFERENCES alert_playbook_executions (tenant_id, execution_id); EXCEPTION WHEN duplicate_object THEN NULL; END $do$`,
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
		`CREATE TABLE IF NOT EXISTS alert_playbook_definitions (
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			stage TEXT NOT NULL DEFAULT 'draft',
			enabled BOOLEAN NOT NULL DEFAULT FALSE,
			risk_level TEXT NOT NULL DEFAULT 'medium',
			definition_payload JSONB NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			submitted_by TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			rejection_reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (tenant_id, name),
			CONSTRAINT alert_playbook_definition_stage_check CHECK (stage IN ('draft', 'approval_pending', 'approved', 'rejected')),
			CONSTRAINT alert_playbook_definition_risk_check CHECK (risk_level IN ('low', 'medium', 'high', 'critical'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_playbook_definitions_tenant_stage
			ON alert_playbook_definitions (tenant_id, stage, updated_at DESC)`,
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
		`CREATE TABLE IF NOT EXISTS notification_rules (
			rule_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
			channels JSONB NOT NULL DEFAULT '[]'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_by UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS notification_history (
			notification_id BIGSERIAL PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			rule_id UUID REFERENCES notification_rules(rule_id) ON DELETE SET NULL,
			alert_id TEXT NOT NULL DEFAULT '',
			target_name TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL,
			alert_type TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			error_message TEXT,
			retry_count INTEGER NOT NULL DEFAULT 0,
			trace_id TEXT NOT NULL DEFAULT '',
			sent_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS target_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS alert_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS notification_escalation_policies (
			policy_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			stages JSONB NOT NULL DEFAULT '[]'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS notification_escalation_jobs (
			job_id BIGSERIAL PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			alert_key TEXT NOT NULL,
			alert_id TEXT NOT NULL DEFAULT '',
			rule_id UUID NOT NULL REFERENCES notification_rules(rule_id) ON DELETE CASCADE,
			stage_index INTEGER NOT NULL,
			policy_id UUID,
			policy_updated_at TIMESTAMPTZ,
			stage_after_minutes DOUBLE PRECISION,
			stage_fingerprint TEXT NOT NULL DEFAULT '',
			target_role TEXT NOT NULL,
			channel TEXT NOT NULL,
			due_at TIMESTAMPTZ NOT NULL,
			alert_payload JSONB NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT,
			locked_at TIMESTAMPTZ,
			lock_token TEXT NOT NULL DEFAULT '',
			trace_id TEXT NOT NULL DEFAULT '',
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, alert_key, rule_id, stage_index, channel)
		)`,
		`ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ`,
		`ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS lock_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS policy_id UUID`,
		`ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS policy_updated_at TIMESTAMPTZ`,
		`ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS stage_after_minutes DOUBLE PRECISION`,
		`ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS stage_fingerprint TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS notification_templates (
			template_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			template_type TEXT NOT NULL,
			name TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			variable_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
			validation_status TEXT NOT NULL DEFAULT 'passed',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (tenant_id, name)
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
		`CREATE INDEX IF NOT EXISTS idx_notification_rules_tenant_enabled
			ON notification_rules (tenant_id, enabled, updated_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_rules_tenant_name
			ON notification_rules (tenant_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_history_tenant_created
			ON notification_history (tenant_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_history_tenant_status
			ON notification_history (tenant_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_escalation_tenant_enabled
			ON notification_escalation_policies (tenant_id, enabled, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_escalation_jobs_due
			ON notification_escalation_jobs (status, due_at, job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_notification_templates_tenant_enabled
			ON notification_templates (tenant_id, enabled, updated_at DESC)`,
		`CREATE OR REPLACE FUNCTION notification_governance_atomic_audit()
		RETURNS TRIGGER AS $$
		DECLARE row_data JSONB; tenant_value TEXT; object_value TEXT; action_prefix TEXT;
		BEGIN
			row_data := CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
			tenant_value := COALESCE(row_data->>'tenant_id','default');
			object_value := CASE WHEN TG_TABLE_NAME='notification_escalation_jobs' THEN COALESCE(row_data->>'job_id',tenant_value) ELSE COALESCE(row_data->>'rule_id',row_data->>'template_id',row_data->>'policy_id',row_data->>'notification_id',tenant_value) END;
			action_prefix := CASE TG_TABLE_NAME
				WHEN 'alert_notification_settings' THEN 'NOTIFICATION_SETTINGS'
				WHEN 'notification_rules' THEN 'NOTIFICATION_RULE'
				WHEN 'notification_templates' THEN 'NOTIFICATION_TEMPLATE'
				WHEN 'notification_escalation_policies' THEN 'NOTIFICATION_ESCALATION'
				WHEN 'notification_escalation_jobs' THEN 'NOTIFICATION_ESCALATION_JOB'
				WHEN 'notification_silence_rules' THEN 'NOTIFICATION_SILENCE_RULE'
				WHEN 'notification_history' THEN 'NOTIFICATION_DELIVERY'
				ELSE 'NOTIFICATION_GOVERNANCE' END;
			INSERT INTO audit_logs(event_id,tenant_id,user_id,action,object_type,object_id,detail)
			VALUES ('audit-'||uuid_generate_v4()::TEXT,tenant_value,NULL,action_prefix||'_DB_'||TG_OP,TG_TABLE_NAME,object_value,jsonb_build_object('atomic',true,'operation',TG_OP));
			RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
		END;
		$$ LANGUAGE plpgsql`,
		`DO $$
		DECLARE table_name TEXT; trigger_name TEXT;
		BEGIN
			FOREACH table_name IN ARRAY ARRAY['alert_notification_settings','notification_rules','notification_templates','notification_escalation_policies','notification_escalation_jobs','notification_silence_rules','notification_history'] LOOP
				trigger_name := 'trg_'||table_name||'_atomic_audit';
				EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I',trigger_name,table_name);
				EXECUTE format('CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION notification_governance_atomic_audit()',trigger_name,table_name);
			END LOOP;
		END $$`,
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
	return r.SavePlaybookExecutionWithMetadata(ctx, tenantID, "", alert, result)
}

func (r *AdvancedRepository) SavePlaybookExecutionWithMetadata(
	ctx context.Context,
	tenantID string,
	requestedBy string,
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

	effectPayload := playbookExecutionEffect(alert, result)
	effectJSON, err := json.Marshal(effectPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal playbook effect payload: %w", err)
	}
	mode := strings.TrimSpace(result.Mode)
	if mode == "" {
		mode = "legacy"
	}
	status := "succeeded"
	if result.FailedActions > 0 {
		status = "failed"
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
		Mode:           mode,
		Status:         status,
		Effect:         effectPayload,
		RequestedBy:    requestedBy,
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin playbook execution: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO alert_playbook_executions (
			execution_id, tenant_id, playbook_name, alert_id,
			success_actions, failed_actions, duration_ms,
			request_payload, result_payload, mode, status,
			effect_payload, requested_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10, $11, $12::jsonb, $13, $14)
	`, record.ExecutionID, record.TenantID, record.PlaybookName, record.AlertID,
		record.SuccessActions, record.FailedActions, record.DurationMS,
		string(requestPayload), string(resultPayload), record.Mode, record.Status,
		string(effectJSON), record.RequestedBy, record.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("save playbook execution: %w", err)
	}
	auditAction := "PLAYBOOK_EXECUTED"
	if record.Mode == "drill" {
		auditAction = "PLAYBOOK_DRILL_COMPLETED"
	}
	if err := insertPlaybookAuditTx(ctx, tx, nil, record.TenantID, record.RequestedBy, auditAction, record.ExecutionID, map[string]interface{}{
		"playbook": record.PlaybookName, "status": record.Status, "mode": record.Mode,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit playbook execution: %w", err)
	}

	return record, nil
}

func (r *AdvancedRepository) ListPlaybookExecutions(ctx context.Context, tenantID string, limit int) ([]PlaybookExecutionRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return r.queryPlaybookExecutions(ctx, tenantID, "", limit)
}

func (r *AdvancedRepository) ListPlaybookExecutionsByName(ctx context.Context, tenantID, playbookName string, limit int) ([]PlaybookExecutionRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return r.queryPlaybookExecutions(ctx, tenantID, playbookName, limit)
}

func (r *AdvancedRepository) ListAllPlaybookExecutions(ctx context.Context, tenantID string) ([]PlaybookExecutionRecord, error) {
	return r.queryPlaybookExecutions(ctx, tenantID, "", 0)
}

func (r *AdvancedRepository) queryPlaybookExecutions(ctx context.Context, tenantID, playbookName string, limit int) ([]PlaybookExecutionRecord, error) {
	if r == nil || r.db == nil {
		return []PlaybookExecutionRecord{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT execution_id, tenant_id, playbook_name, alert_id,
			success_actions, failed_actions, duration_ms,
			request_payload, result_payload, mode, status,
			COALESCE(rollback_of, ''), effect_payload, requested_by,
			rolled_back_at, created_at
		FROM alert_playbook_executions
		WHERE tenant_id = $1 AND ($2 = '' OR playbook_name = $2)
		ORDER BY created_at DESC
		LIMIT NULLIF($3, 0)
	`, tenantID, playbookName, limit)
	if err != nil {
		return nil, fmt.Errorf("list playbook executions: %w", err)
	}
	defer rows.Close()

	records := make([]PlaybookExecutionRecord, 0)
	for rows.Next() {
		var record PlaybookExecutionRecord
		var requestPayload, resultPayload, effectPayload []byte
		var rolledBackAt sql.NullTime
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
			&record.Mode,
			&record.Status,
			&record.RollbackOf,
			&effectPayload,
			&record.RequestedBy,
			&rolledBackAt,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan playbook execution: %w", err)
		}
		record.RequestPayload = decodeJSONMap(requestPayload)
		record.Result = decodeJSONMap(resultPayload)
		record.Effect = decodeJSONMap(effectPayload)
		if rolledBackAt.Valid {
			value := rolledBackAt.Time
			record.RolledBackAt = &value
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playbook executions: %w", err)
	}
	return records, nil
}

func (r *AdvancedRepository) EnsurePlaybookDefinitions(ctx context.Context, tenantID string, defaults []*playbook.Playbook) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("playbook repository is not available")
	}
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("tenant id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin playbook bootstrap: %w", err)
	}
	defer tx.Rollback()
	for index, definition := range defaults {
		if definition == nil || strings.TrimSpace(definition.Name) == "" {
			continue
		}
		payload, err := json.Marshal(definition)
		if err != nil {
			return fmt.Errorf("marshal playbook %s: %w", definition.Name, err)
		}
		displayName := playbookDisplayName(definition, index)
		riskLevel := playbookRiskLevel(definition)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO alert_playbook_definitions (
				tenant_id, name, display_name, description, version, stage,
				enabled, risk_level, definition_payload, created_by,
				submitted_by, approved_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 1, 'approved', $5, $6, $7::jsonb, 'system', 'system', 'system', now(), now())
			ON CONFLICT (tenant_id, name) DO NOTHING
		`, tenantID, definition.Name, displayName, definition.Description, definition.Enabled, riskLevel, string(payload)); err != nil {
			return fmt.Errorf("bootstrap playbook %s: %w", definition.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit playbook bootstrap: %w", err)
	}
	return nil
}

func (r *AdvancedRepository) ListPlaybookDefinitions(ctx context.Context, tenantID string) ([]PlaybookDefinitionRecord, error) {
	if r == nil || r.db == nil {
		return []PlaybookDefinitionRecord{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT tenant_id, name, display_name, description, version, stage,
			enabled, risk_level, definition_payload, created_by,
			submitted_by, approved_by, rejection_reason, created_at, updated_at
		FROM alert_playbook_definitions
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list playbook definitions: %w", err)
	}
	defer rows.Close()
	records := make([]PlaybookDefinitionRecord, 0)
	for rows.Next() {
		record, err := scanPlaybookDefinition(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playbook definitions: %w", err)
	}
	return records, nil
}

func (r *AdvancedRepository) GetPlaybookDefinition(ctx context.Context, tenantID, name string) (*PlaybookDefinitionRecord, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT tenant_id, name, display_name, description, version, stage,
			enabled, risk_level, definition_payload, created_by,
			submitted_by, approved_by, rejection_reason, created_at, updated_at
		FROM alert_playbook_definitions
		WHERE tenant_id = $1 AND name = $2
	`, tenantID, name)
	record, err := scanPlaybookDefinition(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

func (r *AdvancedRepository) SavePlaybookDraft(
	ctx context.Context,
	req *http.Request,
	record PlaybookDefinitionRecord,
	expectedVersion int,
) (*PlaybookDefinitionRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("playbook repository is not available")
	}
	definitionJSON, err := json.Marshal(record.Definition)
	if err != nil {
		return nil, fmt.Errorf("marshal playbook definition: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin playbook draft: %w", err)
	}
	defer tx.Rollback()
	var currentVersion int
	var currentStage string
	err = tx.QueryRowContext(ctx, `
		SELECT version, stage FROM alert_playbook_definitions
		WHERE tenant_id = $1 AND name = $2 FOR UPDATE
	`, record.TenantID, record.Name).Scan(&currentVersion, &currentStage)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("lock playbook draft: %w", err)
	}
	if err == sql.ErrNoRows {
		if expectedVersion != 0 {
			return nil, fmt.Errorf("playbook version conflict: expected %d but definition does not exist", expectedVersion)
		}
		currentVersion = 0
	} else {
		if currentVersion != expectedVersion {
			return nil, fmt.Errorf("playbook version conflict: expected %d, current %d", expectedVersion, currentVersion)
		}
		if currentStage == "approval_pending" {
			return nil, fmt.Errorf("playbook cannot be edited while approval is pending")
		}
	}
	nextVersion := currentVersion + 1
	row := tx.QueryRowContext(ctx, `
		INSERT INTO alert_playbook_definitions (
			tenant_id, name, display_name, description, version, stage,
			enabled, risk_level, definition_payload, created_by,
			submitted_by, approved_by, rejection_reason, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'draft', FALSE, $6, $7::jsonb, $8, '', '', '', now(), now())
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			description = EXCLUDED.description,
			version = EXCLUDED.version,
			stage = 'draft', enabled = FALSE,
			risk_level = EXCLUDED.risk_level,
			definition_payload = EXCLUDED.definition_payload,
			submitted_by = '', approved_by = '', rejection_reason = '', updated_at = now()
		RETURNING tenant_id, name, display_name, description, version, stage,
			enabled, risk_level, definition_payload, created_by,
			submitted_by, approved_by, rejection_reason, created_at, updated_at
	`, record.TenantID, record.Name, record.DisplayName, record.Description, nextVersion,
		record.RiskLevel, string(definitionJSON), record.CreatedBy)
	created, err := scanPlaybookDefinition(row)
	if err != nil {
		return nil, fmt.Errorf("save playbook draft: %w", err)
	}
	if err := insertPlaybookAuditTx(ctx, tx, req, record.TenantID, record.CreatedBy, "PLAYBOOK_DRAFT_SAVED", record.Name, map[string]interface{}{
		"version": created.Version, "stage": created.Stage, "risk_level": created.RiskLevel,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit playbook draft: %w", err)
	}
	return &created, nil
}

func (r *AdvancedRepository) TransitionPlaybook(
	ctx context.Context,
	req *http.Request,
	tenantID, name, action, actor, reason string,
	expectedVersion int,
) (*PlaybookDefinitionRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("playbook repository is not available")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin playbook transition: %w", err)
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `
		SELECT tenant_id, name, display_name, description, version, stage,
			enabled, risk_level, definition_payload, created_by,
			submitted_by, approved_by, rejection_reason, created_at, updated_at
		FROM alert_playbook_definitions
		WHERE tenant_id = $1 AND name = $2 FOR UPDATE
	`, tenantID, name)
	current, err := scanPlaybookDefinition(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("playbook not found: %s", name)
	}
	if err != nil {
		return nil, err
	}
	if current.Version != expectedVersion {
		return nil, fmt.Errorf("playbook version conflict: expected %d, current %d", expectedVersion, current.Version)
	}
	nextStage, enabled, submittedBy, approvedBy, rejectionReason, auditAction, err := playbookTransition(current, action, actor, reason)
	if err != nil {
		return nil, err
	}
	row = tx.QueryRowContext(ctx, `
		UPDATE alert_playbook_definitions
		SET version = version + 1, stage = $3, enabled = $4,
			submitted_by = $5, approved_by = $6, rejection_reason = $7, updated_at = now()
		WHERE tenant_id = $1 AND name = $2
		RETURNING tenant_id, name, display_name, description, version, stage,
			enabled, risk_level, definition_payload, created_by,
			submitted_by, approved_by, rejection_reason, created_at, updated_at
	`, tenantID, name, nextStage, enabled, submittedBy, approvedBy, rejectionReason)
	updated, err := scanPlaybookDefinition(row)
	if err != nil {
		return nil, fmt.Errorf("transition playbook: %w", err)
	}
	if err := insertPlaybookAuditTx(ctx, tx, req, tenantID, actor, auditAction, name, map[string]interface{}{
		"version": updated.Version, "stage": updated.Stage, "reason": reason,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit playbook transition: %w", err)
	}
	return &updated, nil
}

func (r *AdvancedRepository) ListPlaybookAudits(ctx context.Context, tenantID, objectID string, limit int) ([]PlaybookAuditRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return r.queryPlaybookAudits(ctx, tenantID, objectID, false, limit)
}

func (r *AdvancedRepository) ListPlaybookAuditsForPlaybook(ctx context.Context, tenantID, playbookName string, limit int) ([]PlaybookAuditRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return r.queryPlaybookAudits(ctx, tenantID, playbookName, true, limit)
}

func (r *AdvancedRepository) ListAllPlaybookAudits(ctx context.Context, tenantID string) ([]PlaybookAuditRecord, error) {
	return r.queryPlaybookAudits(ctx, tenantID, "", false, 0)
}

func (r *AdvancedRepository) queryPlaybookAudits(ctx context.Context, tenantID, objectID string, includePlaybookDetail bool, limit int) ([]PlaybookAuditRecord, error) {
	if r == nil || r.db == nil {
		return []PlaybookAuditRecord{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT event_id, action, object_id, detail, created_at
		FROM audit_logs
		WHERE tenant_id = $1 AND object_type IN ('playbook', 'playbook_execution')
			AND ($2 = '' OR object_id = $2 OR ($3 AND detail->>'playbook' = $2))
		ORDER BY created_at DESC
		LIMIT NULLIF($4, 0)
	`, tenantID, objectID, includePlaybookDetail, limit)
	if err != nil {
		return nil, fmt.Errorf("list playbook audits: %w", err)
	}
	defer rows.Close()
	records := make([]PlaybookAuditRecord, 0)
	for rows.Next() {
		var record PlaybookAuditRecord
		var detail []byte
		if err := rows.Scan(&record.EventID, &record.Action, &record.ObjectID, &detail, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan playbook audit: %w", err)
		}
		record.Detail = decodeJSONMap(detail)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playbook audits: %w", err)
	}
	return records, nil
}

func (r *AdvancedRepository) RollbackPlaybookDrill(
	ctx context.Context,
	req *http.Request,
	tenantID, executionID, actor, reason string,
) (*PlaybookExecutionRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("playbook repository is not available")
	}
	if len([]rune(strings.TrimSpace(reason))) < 8 {
		return nil, fmt.Errorf("rollback reason must contain at least 8 characters")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin playbook drill rollback: %w", err)
	}
	defer tx.Rollback()
	var playbookName, alertID, mode, status string
	if err := tx.QueryRowContext(ctx, `
		SELECT playbook_name, alert_id, mode, status
		FROM alert_playbook_executions
		WHERE tenant_id = $1 AND execution_id = $2 FOR UPDATE
	`, tenantID, executionID).Scan(&playbookName, &alertID, &mode, &status); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("playbook execution not found: %s", executionID)
		}
		return nil, fmt.Errorf("lock playbook execution: %w", err)
	}
	if mode != "drill" {
		return nil, fmt.Errorf("only drill executions can use the built-in no-effect rollback")
	}
	if status != "succeeded" {
		return nil, fmt.Errorf("playbook execution cannot be rolled back from status %s", status)
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE alert_playbook_executions
		SET status = 'rolled_back', rolled_back_at = $3
		WHERE tenant_id = $1 AND execution_id = $2 AND status = 'succeeded'
	`, tenantID, executionID, now)
	if err != nil {
		return nil, fmt.Errorf("mark playbook drill rolled back: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, fmt.Errorf("playbook drill changed concurrently")
	}
	rollbackID := uuid.NewString()
	requestPayload := map[string]interface{}{"reason": strings.TrimSpace(reason), "rollback_of": executionID}
	resultPayload := map[string]interface{}{"mode": "drill", "external_effect_applied": false, "message": "drill state rolled back"}
	effectPayload := map[string]interface{}{"source": "drill-rollback", "external_effect_applied": false}
	requestJSON, _ := json.Marshal(requestPayload)
	resultJSON, _ := json.Marshal(resultPayload)
	effectJSON, _ := json.Marshal(effectPayload)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO alert_playbook_executions (
			execution_id, tenant_id, playbook_name, alert_id,
			success_actions, failed_actions, duration_ms,
			request_payload, result_payload, mode, status, rollback_of,
			effect_payload, requested_by, created_at
		) VALUES ($1, $2, $3, $4, 0, 0, 0, $5::jsonb, $6::jsonb, 'drill', 'rollback_recorded', $7, $8::jsonb, $9, $10)
	`, rollbackID, tenantID, playbookName, alertID, string(requestJSON), string(resultJSON), executionID, string(effectJSON), actor, now); err != nil {
		return nil, fmt.Errorf("insert playbook drill rollback: %w", err)
	}
	if err := insertPlaybookAuditTx(ctx, tx, req, tenantID, actor, "PLAYBOOK_DRILL_ROLLED_BACK", rollbackID, map[string]interface{}{
		"playbook": playbookName, "rollback_of": executionID, "reason": strings.TrimSpace(reason), "external_effect_applied": false,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit playbook drill rollback: %w", err)
	}
	return &PlaybookExecutionRecord{
		ExecutionID: rollbackID, TenantID: tenantID, PlaybookName: playbookName, AlertID: alertID,
		Mode: "drill", Status: "rollback_recorded", RollbackOf: executionID,
		RequestPayload: requestPayload, Result: resultPayload, Effect: effectPayload,
		RequestedBy: actor, CreatedAt: now,
	}, nil
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
		ORDER BY starts_at DESC, updated_at DESC, rule_id ASC
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

func (r *AdvancedRepository) PatchNotificationSilenceRule(
	ctx context.Context,
	tenantID string,
	ruleID string,
	patch notificationSilencePatchRequest,
) (*NotificationSilenceRule, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}

	var targets interface{}
	if patch.AffectedTargets != nil {
		encoded, err := json.Marshal(*patch.AffectedTargets)
		if err != nil {
			return nil, false, fmt.Errorf("marshal silence rule targets: %w", err)
		}
		targets = string(encoded)
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE notification_silence_rules
		SET name = COALESCE(NULLIF(BTRIM($3), ''), name),
			scope = COALESCE($4, scope),
			starts_at = COALESCE($5, starts_at),
			ends_at = COALESCE($6, ends_at),
			affected_targets = COALESCE($7::jsonb, affected_targets),
			policy = COALESCE($8, policy),
			reason = COALESCE($9, reason),
			enabled = COALESCE($10, enabled),
			updated_at = now()
		WHERE tenant_id = $1 AND rule_id = $2
		RETURNING rule_id, tenant_id, name, scope, starts_at, ends_at,
			affected_targets, policy, reason, enabled, created_by, created_at, updated_at
	`, tenantID, ruleID, patch.Name, patch.Scope, patch.StartsAt, patch.EndsAt, targets, patch.Policy, patch.Reason, patch.Enabled)
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

type playbookDefinitionScanner interface {
	Scan(dest ...interface{}) error
}

func scanPlaybookDefinition(scanner playbookDefinitionScanner) (PlaybookDefinitionRecord, error) {
	var record PlaybookDefinitionRecord
	var payload []byte
	if err := scanner.Scan(
		&record.TenantID,
		&record.Name,
		&record.DisplayName,
		&record.Description,
		&record.Version,
		&record.Stage,
		&record.Enabled,
		&record.RiskLevel,
		&payload,
		&record.CreatedBy,
		&record.SubmittedBy,
		&record.ApprovedBy,
		&record.RejectionReason,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	if err := json.Unmarshal(payload, &record.Definition); err != nil {
		return record, fmt.Errorf("decode playbook definition %s: %w", record.Name, err)
	}
	return record, nil
}

func DecodePlaybookDefinition(record PlaybookDefinitionRecord) (*playbook.Playbook, error) {
	payload, err := json.Marshal(record.Definition)
	if err != nil {
		return nil, fmt.Errorf("marshal playbook definition %s: %w", record.Name, err)
	}
	var definition playbook.Playbook
	if err := json.Unmarshal(payload, &definition); err != nil {
		return nil, fmt.Errorf("decode playbook definition %s: %w", record.Name, err)
	}
	definition.Name = record.Name
	definition.Description = record.Description
	definition.Enabled = record.Enabled
	return &definition, nil
}

func playbookTransition(
	current PlaybookDefinitionRecord,
	action, actor, reason string,
) (stage string, enabled bool, submittedBy, approvedBy, rejectionReason, auditAction string, err error) {
	action = strings.TrimSpace(action)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "", false, "", "", "", "", fmt.Errorf("playbook transition actor is required")
	}
	switch action {
	case "submit":
		if current.Stage != "draft" && current.Stage != "rejected" {
			return "", false, "", "", "", "", fmt.Errorf("cannot submit playbook from stage %s", current.Stage)
		}
		return "approval_pending", false, actor, "", "", "PLAYBOOK_APPROVAL_SUBMITTED", nil
	case "approve":
		if current.Stage != "approval_pending" {
			return "", false, "", "", "", "", fmt.Errorf("cannot approve playbook from stage %s", current.Stage)
		}
		if current.SubmittedBy == actor {
			return "", false, "", "", "", "", fmt.Errorf("playbook two-person approval requires a different approver")
		}
		return "approved", true, current.SubmittedBy, actor, "", "PLAYBOOK_APPROVED", nil
	case "reject":
		if current.Stage != "approval_pending" {
			return "", false, "", "", "", "", fmt.Errorf("cannot reject playbook from stage %s", current.Stage)
		}
		if len([]rune(strings.TrimSpace(reason))) < 8 {
			return "", false, "", "", "", "", fmt.Errorf("rejection reason must contain at least 8 characters")
		}
		if current.SubmittedBy == actor {
			return "", false, "", "", "", "", fmt.Errorf("playbook two-person rejection requires a different reviewer")
		}
		return "rejected", false, current.SubmittedBy, actor, strings.TrimSpace(reason), "PLAYBOOK_REJECTED", nil
	case "disable":
		if current.Stage != "approved" || !current.Enabled {
			return "", false, "", "", "", "", fmt.Errorf("only an enabled approved playbook can be disabled")
		}
		return "approved", false, current.SubmittedBy, current.ApprovedBy, "", "PLAYBOOK_DISABLED", nil
	case "enable":
		if current.Stage != "approved" || current.Enabled {
			return "", false, "", "", "", "", fmt.Errorf("only a disabled approved playbook can be enabled")
		}
		return "approved", true, current.SubmittedBy, current.ApprovedBy, "", "PLAYBOOK_ENABLED", nil
	default:
		return "", false, "", "", "", "", fmt.Errorf("unsupported playbook transition: %s", action)
	}
}

func playbookDisplayName(definition *playbook.Playbook, index int) string {
	names := map[string]string{
		"block-scanner":        "高危扫描源封禁",
		"quarantine-c2":        "C2 连接阻断剧本",
		"throttle-brute-force": "暴力破解限速",
		"investigate-exfil":    "数据外泄取证升级",
		"log-lateral-movement": "横向移动记录标记",
		"dns-tunnel-block":     "DNS 隧道阻断剧本",
	}
	if value := names[definition.Name]; value != "" {
		return value
	}
	if strings.TrimSpace(definition.Description) != "" {
		return strings.TrimSpace(definition.Description)
	}
	return fmt.Sprintf("SOAR 剧本-%d", index+1)
}

func playbookRiskLevel(definition *playbook.Playbook) string {
	if definition == nil {
		return "medium"
	}
	for _, action := range definition.Actions {
		switch action.Type {
		case "block_ip", "block_domain", "quarantine":
			return "critical"
		case "rate_limit", "escalate":
			return "high"
		}
	}
	if definition.Trigger.SeverityMin == "critical" {
		return "critical"
	}
	if definition.Trigger.SeverityMin == "high" {
		return "high"
	}
	return "medium"
}

func playbookExecutionEffect(alert *playbook.AlertContext, result *playbook.ExecutionResult) map[string]interface{} {
	beforeAlerts := alert.RelatedAlertCount
	if beforeAlerts < 0 {
		beforeAlerts = 0
	}
	afterAlerts := beforeAlerts - result.SuccessActions
	if afterAlerts < 0 {
		afterAlerts = 0
	}
	isolatedHosts := 0
	blockedConnections := 0
	for _, action := range result.Actions {
		if action.Error != "" {
			continue
		}
		switch action.ActionType {
		case "quarantine":
			isolatedHosts++
		case "block_ip", "block_domain", "rate_limit":
			blockedConnections++
		}
	}
	return map[string]interface{}{
		"source":                  "derived-from-drill-input-and-action-results",
		"alerts_before":           beforeAlerts,
		"alerts_after":            afterAlerts,
		"blocked_connections":     blockedConnections,
		"isolated_hosts":          isolatedHosts,
		"false_operation_rate":    0,
		"external_effect_applied": result.Mode == "live",
	}
}

func insertPlaybookAuditTx(
	ctx context.Context,
	tx *sql.Tx,
	req *http.Request,
	tenantID, actor, action, objectID string,
	detail map[string]interface{},
) error {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["actor"] = actor
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal playbook audit: %w", err)
	}
	ip, userAgent := "", ""
	if req != nil {
		ip, userAgent = clientIP(req), req.UserAgent()
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			event_id, tenant_id, user_id, action, object_type, object_id,
			detail, ip_addr, user_agent, created_at
		) VALUES ($1, $2, NULL, $3, 'playbook', $4, $5::jsonb, $6, $7, now())
	`, "audit-"+uuid.NewString(), tenantID, action, objectID, string(detailJSON), ip, userAgent); err != nil {
		return fmt.Errorf("insert playbook audit: %w", err)
	}
	return nil
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

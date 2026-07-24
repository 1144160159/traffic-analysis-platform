// Whitelist Management — 告警白名单 CRUD
// 业务场景: 安全分析师标记已知误报来源，后续自动过滤
package whitelist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Entry 白名单条目
type Entry struct {
	ID             string     `json:"id" db:"id"`
	TenantID       string     `json:"tenant_id" db:"tenant_id"`
	Type           string     `json:"type" db:"type"`     // ip | domain | fingerprint | subnet
	Value          string     `json:"value" db:"value"`   // IP/域名/指纹值
	Reason         string     `json:"reason" db:"reason"` // 加入原因 (FP reason code)
	Description    string     `json:"description" db:"description"`
	Status         string     `json:"status,omitempty" db:"status"`                   // draft | pending | active | disabled
	ApprovalStatus string     `json:"approval_status,omitempty" db:"approval_status"` // draft | pending | approved | rejected
	SourceAlertID  string     `json:"source_alert_id,omitempty" db:"source_alert_id"`
	FeedbackID     string     `json:"feedback_id,omitempty" db:"feedback_id"`
	OwnerRole      string     `json:"owner_role,omitempty" db:"owner_role"`
	Scope          string     `json:"scope,omitempty" db:"scope"`
	RiskLevel      string     `json:"risk_level,omitempty" db:"risk_level"`
	CoveredAlerts  int        `json:"covered_alerts,omitempty" db:"covered_alerts"`
	CoveredAssets  int        `json:"covered_assets,omitempty" db:"covered_assets"`
	Version        int        `json:"version" db:"version"`
	CreatedBy      string     `json:"created_by" db:"created_by"`
	ApprovedBy     string     `json:"approved_by,omitempty" db:"approved_by"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty" db:"approved_at"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"` // nil=永久
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type UpdateRequest struct {
	Status          *string    `json:"status,omitempty"`
	ApprovalStatus  *string    `json:"approval_status,omitempty"`
	Reason          *string    `json:"reason,omitempty"`
	Description     *string    `json:"description,omitempty"`
	OwnerRole       *string    `json:"owner_role,omitempty"`
	Scope           *string    `json:"scope,omitempty"`
	RiskLevel       *string    `json:"risk_level,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	ExpectedVersion *int       `json:"expected_version,omitempty"`
}

var (
	ErrVersionConflict = errors.New("whitelist version conflict")
	ErrAlreadyExists   = errors.New("whitelist entry already exists")
)

type sqlRunner interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

// AuditRecord is the security audit row committed atomically with a whitelist
// mutation. Callers provide request metadata; the repository owns the database
// transaction so a missing audit row can never leave a whitelist business row.
type AuditRecord struct {
	UserID    string
	Action    string
	ObjectID  string
	Detail    map[string]interface{}
	IPAddress string
	UserAgent string
}

// Repository 白名单持久化 (PostgreSQL)
type Repository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewRepository(db *sql.DB, logger *zap.Logger) *Repository {
	return &Repository{db: db, logger: logger}
}

// CreateWithAudit inserts a whitelist draft and its security audit row in the
// same PostgreSQL transaction. It is used by non-whitelist HTTP workflows such
// as FP feedback so they cannot bypass the governance audit invariant.
func (r *Repository) CreateWithAudit(ctx context.Context, entry *Entry, audit AuditRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.CreateTx(ctx, tx, entry); err != nil {
		return err
	}
	if audit.ObjectID == "" {
		audit.ObjectID = entry.ID
	}
	if err := r.insertAuditWithRunner(ctx, tx, entry.TenantID, audit); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) insertAuditWithRunner(ctx context.Context, runner sqlRunner, tenantID string, audit AuditRecord) error {
	detail := make(map[string]interface{}, len(audit.Detail)+1)
	for key, value := range audit.Detail {
		detail[key] = value
	}
	detail["result"] = "success"
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	userIDExpr := "NULLIF($3, '')"
	userID := audit.UserID
	if r.pgColumnType(ctx, "audit_logs", "user_id") == "uuid" {
		userIDExpr = "NULLIF($3, '')::uuid"
		if userID != "" {
			if _, err := uuid.Parse(userID); err != nil {
				userID = ""
			}
		}
	}
	args := []interface{}{tenantID, userID, audit.Action, "whitelist", audit.ObjectID, string(detailJSON), audit.IPAddress, audit.UserAgent}
	query := `INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, ` + strings.Replace(userIDExpr, "$3", "$2", 1) + `, $3, $4, $5, $6::jsonb, $7, $8)`
	if r.pgColumnExists(ctx, "audit_logs", "event_id") {
		query = `INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, ` + userIDExpr + `, $4, $5, $6, $7::jsonb, $8, $9)`
		args = append([]interface{}{"audit-" + uuid.NewString()}, args...)
	}
	_, err = runner.ExecContext(ctx, query, args...)
	return err
}

func (r *Repository) pgColumnExists(ctx context.Context, tableName, columnName string) bool {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2
	)`, tableName, columnName).Scan(&exists)
	return err == nil && exists
}

func (r *Repository) pgColumnType(ctx context.Context, tableName, columnName string) string {
	var dataType string
	err := r.db.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2
		ORDER BY CASE WHEN table_schema = 'public' THEN 0 ELSE 1 END LIMIT 1`, tableName, columnName).Scan(&dataType)
	if err != nil {
		return ""
	}
	return dataType
}

func (r *Repository) InitSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS whitelist (
			id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id   TEXT NOT NULL,
			type        TEXT NOT NULL CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model')),
			value       TEXT NOT NULL,
			reason      TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'draft',
			approval_status TEXT NOT NULL DEFAULT 'draft',
			source_alert_id TEXT NOT NULL DEFAULT '',
			feedback_id TEXT NOT NULL DEFAULT '',
			owner_role  TEXT NOT NULL DEFAULT '',
			scope       TEXT NOT NULL DEFAULT '',
			risk_level  TEXT NOT NULL DEFAULT 'medium',
			covered_alerts INTEGER NOT NULL DEFAULT 0,
			covered_assets INTEGER NOT NULL DEFAULT 0,
			version     INTEGER NOT NULL DEFAULT 1,
			created_by  TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			approved_at TIMESTAMPTZ,
			disabled_at TIMESTAMPTZ,
			expires_at  TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(tenant_id, type, value)
		);
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'draft';
		ALTER TABLE whitelist ALTER COLUMN status SET DEFAULT 'draft';
		ALTER TABLE whitelist ALTER COLUMN approval_status SET DEFAULT 'draft';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS source_alert_id TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS feedback_id TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS owner_role TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'medium';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS covered_alerts INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS covered_assets INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
		ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_type_check;
		ALTER TABLE whitelist ADD CONSTRAINT whitelist_type_check CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model'));
		ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_governance_state_check;
		ALTER TABLE whitelist ADD CONSTRAINT whitelist_governance_state_check CHECK (
			(status='draft' AND approval_status='draft') OR
			(status='pending' AND approval_status='pending') OR
			(status='active' AND approval_status='approved') OR
			(status='disabled' AND approval_status IN ('approved','rejected'))
		);
		CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_whitelist_entries_tenant_status ON whitelist(tenant_id, status, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_whitelist_entries_approval ON whitelist(tenant_id, approval_status, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_whitelist_source_alert ON whitelist(tenant_id, source_alert_id) WHERE source_alert_id <> '';
		CREATE INDEX IF NOT EXISTS idx_whitelist_expires ON whitelist(expires_at) WHERE expires_at IS NOT NULL;`)
	return err
}

func (r *Repository) Create(ctx context.Context, entry *Entry) error {
	return r.createWithRunner(ctx, r.db, entry)
}

func (r *Repository) CreateTx(ctx context.Context, tx *sql.Tx, entry *Entry) error {
	return r.createWithRunner(ctx, tx, entry)
}

func (r *Repository) createWithRunner(ctx context.Context, runner sqlRunner, entry *Entry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.Status == "" {
		entry.Status = "draft"
	}
	entry.Status = normalizeStatus(entry.Status, "draft")
	if entry.ApprovalStatus == "" {
		entry.ApprovalStatus = "draft"
	}
	entry.ApprovalStatus = normalizeApprovalStatus(entry.ApprovalStatus, "draft")
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = entry.CreatedAt
	}
	if entry.Version <= 0 {
		entry.Version = 1
	}
	entry.Type = normalizeType(entry.Type)
	entry.RiskLevel = normalizeRiskLevel(entry.RiskLevel)
	if entry.Status != "draft" || entry.ApprovalStatus != "draft" {
		return errors.New("new whitelist entries must start as draft/draft")
	}
	err := runner.QueryRowContext(ctx,
		`INSERT INTO whitelist (id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id, owner_role, scope, risk_level, covered_alerts, covered_assets, version, created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		 ON CONFLICT (tenant_id, type, value) DO NOTHING
		 RETURNING id, version, created_at, updated_at`,
		entry.ID, entry.TenantID, entry.Type, entry.Value, entry.Reason, entry.Description,
		entry.Status, entry.ApprovalStatus, entry.SourceAlertID, entry.FeedbackID, entry.OwnerRole,
		entry.Scope, entry.RiskLevel, entry.CoveredAlerts, entry.CoveredAssets, entry.Version,
		entry.CreatedBy, entry.ApprovedBy, entry.ApprovedAt, entry.DisabledAt, entry.ExpiresAt,
		entry.CreatedAt, entry.UpdatedAt).Scan(&entry.ID, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
	if err == sql.ErrNoRows {
		return ErrAlreadyExists
	}
	return err
}

func (r *Repository) DeleteTx(ctx context.Context, tx *sql.Tx, tenantID, id string, expectedVersion int) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM whitelist WHERE tenant_id=$1 AND id=$2 AND version=$3`, tenantID, id, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (r *Repository) List(ctx context.Context, tenantID string, limit, offset int) ([]*Entry, int, error) {
	var total int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1`, tenantID).Scan(&total)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		        owner_role, scope, risk_level, covered_alerts, covered_assets, version,
		        created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at
		 FROM whitelist WHERE tenant_id=$1
		 ORDER BY updated_at DESC, created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries := make([]*Entry, 0)
	for rows.Next() {
		var e Entry
		rows.Scan(&e.ID, &e.TenantID, &e.Type, &e.Value, &e.Reason, &e.Description, &e.Status, &e.ApprovalStatus,
			&e.SourceAlertID, &e.FeedbackID, &e.OwnerRole, &e.Scope, &e.RiskLevel, &e.CoveredAlerts, &e.CoveredAssets, &e.Version,
			&e.CreatedBy, &e.ApprovedBy, &e.ApprovedAt,
			&e.DisabledAt, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
		entries = append(entries, &e)
	}
	return entries, total, nil
}

func (r *Repository) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	return r.getWithRunner(ctx, r.db, tenantID, id)
}

func (r *Repository) getWithRunner(ctx context.Context, runner sqlRunner, tenantID, id string) (*Entry, error) {
	var e Entry
	err := runner.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		        owner_role, scope, risk_level, covered_alerts, covered_assets, version,
		        created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at
		 FROM whitelist WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(
		&e.ID, &e.TenantID, &e.Type, &e.Value, &e.Reason, &e.Description, &e.Status, &e.ApprovalStatus,
		&e.SourceAlertID, &e.FeedbackID, &e.OwnerRole, &e.Scope, &e.RiskLevel, &e.CoveredAlerts, &e.CoveredAssets, &e.Version,
		&e.CreatedBy, &e.ApprovedBy, &e.ApprovedAt,
		&e.DisabledAt, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) Update(ctx context.Context, tenantID, id string, req UpdateRequest, actor string) (*Entry, error) {
	return r.updateWithRunner(ctx, r.db, tenantID, id, req, actor)
}

func (r *Repository) UpdateTx(ctx context.Context, tx *sql.Tx, tenantID, id string, req UpdateRequest, actor string) (*Entry, error) {
	return r.updateWithRunner(ctx, tx, tenantID, id, req, actor)
}

func (r *Repository) updateWithRunner(ctx context.Context, runner sqlRunner, tenantID, id string, req UpdateRequest, actor string) (*Entry, error) {
	entry, err := r.getWithRunner(ctx, runner, tenantID, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if req.Reason != nil {
		entry.Reason = *req.Reason
	}
	if req.Description != nil {
		entry.Description = *req.Description
	}
	if req.OwnerRole != nil {
		entry.OwnerRole = *req.OwnerRole
	}
	if req.Scope != nil {
		entry.Scope = *req.Scope
	}
	if req.RiskLevel != nil {
		entry.RiskLevel = normalizeRiskLevel(*req.RiskLevel)
	}
	if req.ExpiresAt != nil {
		entry.ExpiresAt = req.ExpiresAt
	}
	if req.Status != nil {
		entry.Status = normalizeStatus(*req.Status, entry.Status)
		if entry.Status == "disabled" {
			entry.DisabledAt = &now
		}
	}
	if req.ApprovalStatus != nil {
		entry.ApprovalStatus = normalizeApprovalStatus(*req.ApprovalStatus, entry.ApprovalStatus)
	}
	if entry.Status == "active" && entry.ApprovalStatus == "" {
		entry.ApprovalStatus = "approved"
	}
	if entry.ApprovalStatus == "approved" && entry.ApprovedAt == nil {
		entry.ApprovedAt = &now
		entry.ApprovedBy = actor
	}
	if entry.Status != "disabled" {
		entry.DisabledAt = nil
	}

	expectedVersion := entry.Version
	if req.ExpectedVersion != nil {
		expectedVersion = *req.ExpectedVersion
	}
	err = runner.QueryRowContext(ctx,
		`UPDATE whitelist
		    SET reason=$3, description=$4, status=$5, approval_status=$6, owner_role=$7,
		        scope=$8, risk_level=$9, approved_by=$10, approved_at=$11, disabled_at=$12,
		        expires_at=$13, version=version+1, updated_at=now()
		  WHERE tenant_id=$1 AND id=$2 AND version=$14
		  RETURNING id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		            owner_role, scope, risk_level, covered_alerts, covered_assets, version,
		            created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at`,
		tenantID, id, entry.Reason, entry.Description, entry.Status, entry.ApprovalStatus, entry.OwnerRole,
		entry.Scope, entry.RiskLevel, entry.ApprovedBy, entry.ApprovedAt, entry.DisabledAt, entry.ExpiresAt, expectedVersion).Scan(
		&entry.ID, &entry.TenantID, &entry.Type, &entry.Value, &entry.Reason, &entry.Description, &entry.Status,
		&entry.ApprovalStatus, &entry.SourceAlertID, &entry.FeedbackID, &entry.OwnerRole, &entry.Scope, &entry.RiskLevel,
		&entry.CoveredAlerts, &entry.CoveredAssets, &entry.Version, &entry.CreatedBy,
		&entry.ApprovedBy, &entry.ApprovedAt, &entry.DisabledAt, &entry.ExpiresAt, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			if _, getErr := r.getWithRunner(ctx, runner, tenantID, id); getErr == nil {
				return nil, ErrVersionConflict
			}
		}
		return nil, err
	}
	return entry, nil
}

func (r *Repository) IsWhitelisted(ctx context.Context, tenantID, value string) bool {
	var count int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1 AND value=$2 AND status='active' AND approval_status='approved' AND (expires_at IS NULL OR expires_at > now())`, tenantID, value).Scan(&count)
	return count > 0
}

// MatchesAlert 检查告警是否匹配白名单 (IP/指纹)
func (r *Repository) MatchesAlert(ctx context.Context, tenantID, srcIP, dstIP, fingerprint string) bool {
	if r.IsWhitelisted(ctx, tenantID, srcIP) || r.IsWhitelisted(ctx, tenantID, dstIP) {
		return true
	}
	if fingerprint != "" && r.IsWhitelisted(ctx, tenantID, fingerprint) {
		return true
	}
	return false
}

// MatchesSubnet 检查 IP 是否在白名单子网内 (优化版: 使用 PostgreSQL inet 类型进行服务端过滤)
func (r *Repository) MatchesSubnet(ctx context.Context, tenantID, ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	// 使用 PostgreSQL 的 inet << 操作符进行服务端子网匹配，避免全表扫描后客户端过滤
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM whitelist WHERE tenant_id=$1 AND type='subnet'
		 AND status='active' AND approval_status='approved'
		 AND (expires_at IS NULL OR expires_at > now())
		 AND $2::inet <<= value::inet)`, tenantID, ip).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func (e *Entry) ToJSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

func approvalStatusForEntryStatus(status string) string {
	switch normalizeStatus(status, "active") {
	case "draft":
		return "draft"
	case "pending":
		return "pending"
	case "disabled":
		return "approved"
	default:
		return "approved"
	}
}

func normalizeStatus(status, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "draft", "pending", "active", "disabled":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return fallback
	}
}

func normalizeApprovalStatus(status, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "draft", "pending", "approved", "rejected":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return fallback
	}
}

func normalizeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ip", "domain", "fingerprint", "subnet", "asset", "account", "rule", "model":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeRiskLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

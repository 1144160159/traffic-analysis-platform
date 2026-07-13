// Whitelist Management — 告警白名单 CRUD
// 业务场景: 安全分析师标记已知误报来源，后续自动过滤
package whitelist

import (
	"context"
	"database/sql"
	"encoding/json"
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
	CreatedBy      string     `json:"created_by" db:"created_by"`
	ApprovedBy     string     `json:"approved_by,omitempty" db:"approved_by"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty" db:"approved_at"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"` // nil=永久
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type UpdateRequest struct {
	Status         *string    `json:"status,omitempty"`
	ApprovalStatus *string    `json:"approval_status,omitempty"`
	Reason         *string    `json:"reason,omitempty"`
	Description    *string    `json:"description,omitempty"`
	OwnerRole      *string    `json:"owner_role,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// Repository 白名单持久化 (PostgreSQL)
type Repository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewRepository(db *sql.DB, logger *zap.Logger) *Repository {
	return &Repository{db: db, logger: logger}
}

func (r *Repository) InitSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS whitelist (
			id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id   TEXT NOT NULL,
			type        TEXT NOT NULL CHECK (type IN ('ip','domain','fingerprint','subnet')),
			value       TEXT NOT NULL,
			reason      TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'active',
			approval_status TEXT NOT NULL DEFAULT 'approved',
			source_alert_id TEXT NOT NULL DEFAULT '',
			feedback_id TEXT NOT NULL DEFAULT '',
			owner_role  TEXT NOT NULL DEFAULT '',
			created_by  TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			approved_at TIMESTAMPTZ,
			disabled_at TIMESTAMPTZ,
			expires_at  TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(tenant_id, type, value)
		);
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'approved';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS source_alert_id TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS feedback_id TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS owner_role TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;
		ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
		CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_whitelist_entries_tenant_status ON whitelist(tenant_id, status, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_whitelist_entries_approval ON whitelist(tenant_id, approval_status, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_whitelist_source_alert ON whitelist(tenant_id, source_alert_id) WHERE source_alert_id <> '';
		CREATE INDEX IF NOT EXISTS idx_whitelist_expires ON whitelist(expires_at) WHERE expires_at IS NOT NULL;`)
	return err
}

func (r *Repository) Create(ctx context.Context, entry *Entry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.Status == "" {
		entry.Status = "active"
	}
	entry.Status = normalizeStatus(entry.Status, "active")
	if entry.ApprovalStatus == "" {
		entry.ApprovalStatus = approvalStatusForEntryStatus(entry.Status)
	}
	entry.ApprovalStatus = normalizeApprovalStatus(entry.ApprovalStatus, approvalStatusForEntryStatus(entry.Status))
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = entry.CreatedAt
	}
	if entry.Status == "active" && entry.ApprovalStatus == "approved" && entry.ApprovedAt == nil {
		approvedAt := entry.CreatedAt
		entry.ApprovedAt = &approvedAt
	}
	return r.db.QueryRowContext(ctx,
		`INSERT INTO whitelist (id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id, owner_role, created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 ON CONFLICT (tenant_id, type, value) DO UPDATE SET
		   reason=$5, description=$6, status=$7, approval_status=$8, source_alert_id=$9, feedback_id=$10,
		   owner_role=$11, created_by=$12, approved_by=$13, approved_at=$14, disabled_at=$15, expires_at=$16, updated_at=now()
		 RETURNING id, created_at, updated_at`,
		entry.ID, entry.TenantID, entry.Type, entry.Value, entry.Reason, entry.Description,
		entry.Status, entry.ApprovalStatus, entry.SourceAlertID, entry.FeedbackID, entry.OwnerRole,
		entry.CreatedBy, entry.ApprovedBy, entry.ApprovedAt, entry.DisabledAt, entry.ExpiresAt,
		entry.CreatedAt, entry.UpdatedAt).Scan(&entry.ID, &entry.CreatedAt, &entry.UpdatedAt)
}

func (r *Repository) Delete(ctx context.Context, tenantID, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM whitelist WHERE tenant_id=$1 AND id=$2`, tenantID, id)
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
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1 AND (expires_at IS NULL OR expires_at > now())`, tenantID).Scan(&total)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		        owner_role, created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at
		 FROM whitelist WHERE tenant_id=$1 AND (expires_at IS NULL OR expires_at > now())
		 ORDER BY updated_at DESC, created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries := make([]*Entry, 0)
	for rows.Next() {
		var e Entry
		rows.Scan(&e.ID, &e.TenantID, &e.Type, &e.Value, &e.Reason, &e.Description, &e.Status, &e.ApprovalStatus,
			&e.SourceAlertID, &e.FeedbackID, &e.OwnerRole, &e.CreatedBy, &e.ApprovedBy, &e.ApprovedAt,
			&e.DisabledAt, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
		entries = append(entries, &e)
	}
	return entries, total, nil
}

func (r *Repository) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	var e Entry
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		        owner_role, created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at
		 FROM whitelist WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(
		&e.ID, &e.TenantID, &e.Type, &e.Value, &e.Reason, &e.Description, &e.Status, &e.ApprovalStatus,
		&e.SourceAlertID, &e.FeedbackID, &e.OwnerRole, &e.CreatedBy, &e.ApprovedBy, &e.ApprovedAt,
		&e.DisabledAt, &e.ExpiresAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) Update(ctx context.Context, tenantID, id string, req UpdateRequest, actor string) (*Entry, error) {
	entry, err := r.Get(ctx, tenantID, id)
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

	err = r.db.QueryRowContext(ctx,
		`UPDATE whitelist
		    SET reason=$3, description=$4, status=$5, approval_status=$6, owner_role=$7,
		        approved_by=$8, approved_at=$9, disabled_at=$10, expires_at=$11, updated_at=now()
		  WHERE tenant_id=$1 AND id=$2
		  RETURNING id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		            owner_role, created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at`,
		tenantID, id, entry.Reason, entry.Description, entry.Status, entry.ApprovalStatus, entry.OwnerRole,
		entry.ApprovedBy, entry.ApprovedAt, entry.DisabledAt, entry.ExpiresAt).Scan(
		&entry.ID, &entry.TenantID, &entry.Type, &entry.Value, &entry.Reason, &entry.Description, &entry.Status,
		&entry.ApprovalStatus, &entry.SourceAlertID, &entry.FeedbackID, &entry.OwnerRole, &entry.CreatedBy,
		&entry.ApprovedBy, &entry.ApprovedAt, &entry.DisabledAt, &entry.ExpiresAt, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (r *Repository) IsWhitelisted(ctx context.Context, tenantID, value string) bool {
	var count int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1 AND value=$2 AND status='active' AND (expires_at IS NULL OR expires_at > now())`, tenantID, value).Scan(&count)
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
		 AND status='active'
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

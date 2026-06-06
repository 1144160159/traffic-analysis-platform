// Whitelist Management — 告警白名单 CRUD
// 业务场景: 安全分析师标记已知误报来源，后续自动过滤
package whitelist

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Entry 白名单条目
type Entry struct {
	ID          string    `json:"id" db:"id"`
	TenantID    string    `json:"tenant_id" db:"tenant_id"`
	Type        string    `json:"type" db:"type"`         // ip | domain | fingerprint | subnet
	Value       string    `json:"value" db:"value"`       // IP/域名/指纹值
	Reason      string    `json:"reason" db:"reason"`     // 加入原因 (FP reason code)
	Description string    `json:"description" db:"description"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"` // nil=永久
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
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
			created_by  TEXT NOT NULL DEFAULT '',
			expires_at  TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(tenant_id, type, value)
		);
		CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_whitelist_expires ON whitelist(expires_at) WHERE expires_at IS NOT NULL;`)
	return err
}

func (r *Repository) Create(ctx context.Context, entry *Entry) error {
	if entry.ID == "" { entry.ID = uuid.New().String() }
	if entry.CreatedAt.IsZero() { entry.CreatedAt = time.Now() }
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO whitelist (id, tenant_id, type, value, reason, description, created_by, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (tenant_id, type, value) DO UPDATE SET reason=$5, description=$6, expires_at=$8`,
		entry.ID, entry.TenantID, entry.Type, entry.Value, entry.Reason, entry.Description,
		entry.CreatedBy, entry.ExpiresAt, entry.CreatedAt)
	return err
}

func (r *Repository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM whitelist WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	return err
}

func (r *Repository) List(ctx context.Context, tenantID string, limit, offset int) ([]*Entry, int, error) {
	var total int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1 AND (expires_at IS NULL OR expires_at > now())`, tenantID).Scan(&total)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, type, value, reason, description, created_by, expires_at, created_at
		 FROM whitelist WHERE tenant_id=$1 AND (expires_at IS NULL OR expires_at > now())
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil { return nil, 0, err }
	defer rows.Close()
	var entries []*Entry
	for rows.Next() {
		var e Entry
		rows.Scan(&e.ID, &e.TenantID, &e.Type, &e.Value, &e.Reason, &e.Description, &e.CreatedBy, &e.ExpiresAt, &e.CreatedAt)
		entries = append(entries, &e)
	}
	return entries, total, nil
}

func (r *Repository) IsWhitelisted(ctx context.Context, tenantID, value string) bool {
	var count int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1 AND value=$2 AND (expires_at IS NULL OR expires_at > now())`, tenantID, value).Scan(&count)
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

// MatchesSubnet 检查 IP 是否在白名单子网内
func (r *Repository) MatchesSubnet(ctx context.Context, tenantID, ip string) bool {
	rows, err := r.db.QueryContext(ctx,
		`SELECT value FROM whitelist WHERE tenant_id=$1 AND type='subnet' AND (expires_at IS NULL OR expires_at > now())`, tenantID)
	if err != nil { return false }
	defer rows.Close()
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil { return false }
	for rows.Next() {
		var subnet string
		rows.Scan(&subnet)
		_, cidr, err := net.ParseCIDR(subnet)
		if err == nil && cidr.Contains(parsedIP) { return true }
	}
	return false
}

func (e *Entry) ToJSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

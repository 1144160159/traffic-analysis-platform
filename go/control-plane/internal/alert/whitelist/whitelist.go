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

	"go.uber.org/zap"
)

// Entry 白名单条目
type Entry struct {
	ID               string     `json:"id" db:"id"`
	TenantID         string     `json:"tenant_id" db:"tenant_id"`
	Type             string     `json:"type" db:"type"`     // ip | domain | fingerprint | subnet
	Value            string     `json:"value" db:"value"`   // IP/域名/指纹值
	Reason           string     `json:"reason" db:"reason"` // 加入原因 (FP reason code)
	Description      string     `json:"description" db:"description"`
	Status           string     `json:"status,omitempty" db:"status"`                   // draft | pending | active | disabled
	ApprovalStatus   string     `json:"approval_status,omitempty" db:"approval_status"` // draft | pending | approved | rejected
	SourceAlertID    string     `json:"source_alert_id,omitempty" db:"source_alert_id"`
	FeedbackID       string     `json:"feedback_id,omitempty" db:"feedback_id"`
	OwnerRole        string     `json:"owner_role,omitempty" db:"owner_role"`
	Scope            string     `json:"scope,omitempty" db:"scope"`
	RiskLevel        string     `json:"risk_level,omitempty" db:"risk_level"`
	CoveredAlerts    int        `json:"covered_alerts,omitempty" db:"covered_alerts"`
	CoveredAssets    int        `json:"covered_assets,omitempty" db:"covered_assets"`
	Version          int        `json:"version" db:"version"`
	CreatedBy        string     `json:"created_by" db:"created_by"`
	ApprovedBy       string     `json:"approved_by,omitempty" db:"approved_by"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty" db:"approved_at"`
	DisabledAt       *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty" db:"expires_at"` // nil=永久
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty" db:"archived_at"`
	LastActionID     string     `json:"last_action_id,omitempty" db:"last_action_id"`
	LastTraceID      string     `json:"last_trace_id,omitempty" db:"last_trace_id"`
	RuleEffectStatus string     `json:"rule_effect_status,omitempty" db:"rule_effect_status"`
	RuleDesiredState string     `json:"rule_desired_state,omitempty" db:"rule_desired_state"`
	RuleRevision     string     `json:"rule_revision,omitempty" db:"rule_revision"`
}

type UpdateRequest struct {
	Status           *string    `json:"status,omitempty"`
	ApprovalStatus   *string    `json:"approval_status,omitempty"`
	Reason           *string    `json:"reason,omitempty"`
	Description      *string    `json:"description,omitempty"`
	OwnerRole        *string    `json:"owner_role,omitempty"`
	Scope            *string    `json:"scope,omitempty"`
	RiskLevel        *string    `json:"risk_level,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ExpectedVersion  *int       `json:"expected_version,omitempty"`
	ExpectedRevision *int       `json:"expected_revision,omitempty"`
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
	EventID   string
	UserID    string
	Action    string
	ObjectID  string
	Detail    map[string]interface{}
	IPAddress string
	UserAgent string
	RequestID string
	TraceID   string
}

// Repository 白名单持久化 (PostgreSQL)
type Repository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewRepository(db *sql.DB, logger *zap.Logger) *Repository {
	return &Repository{db: db, logger: logger}
}

func (r *Repository) VerifyRuleProjectionSchema(ctx context.Context) error {
	var columns int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='whitelist_rule_projection'
		  AND column_name IN ('tenant_id','entry_id','entry_version','source_event_id',
		    'desired_state','entry_type','match_value','scope','expires_at','rule_revision',
		    'payload_sha256','kafka_partition','kafka_offset','applied_at')`).Scan(&columns); err != nil {
		return err
	}
	if columns != 14 {
		return errors.New("whitelist rule projection schema is incomplete")
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID string, limit, offset int) ([]*Entry, int, error) {
	var total int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist WHERE tenant_id=$1 AND archived_at IS NULL`, tenantID).Scan(&total)
	rows, err := r.db.QueryContext(ctx,
		whitelistEntrySelect+` WHERE w.tenant_id=$1 AND w.archived_at IS NULL
		 ORDER BY w.updated_at DESC,w.created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries := make([]*Entry, 0)
	for rows.Next() {
		entry, scanErr := scanWhitelistEntry(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		entries = append(entries, entry)
	}
	return entries, total, rows.Err()
}

func (r *Repository) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	return r.getWithRunner(ctx, r.db, tenantID, id)
}

func (r *Repository) getWithRunner(ctx context.Context, runner sqlRunner, tenantID, id string) (*Entry, error) {
	entry, err := scanWhitelistEntry(runner.QueryRowContext(ctx, whitelistEntrySelect+`
		WHERE w.tenant_id=$1 AND w.id=$2 AND w.archived_at IS NULL`, tenantID, id))
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (r *Repository) IsWhitelisted(ctx context.Context, tenantID, value string) bool {
	var count int
	r.db.QueryRowContext(ctx, `SELECT count(*) FROM whitelist_rule_projection p
		JOIN whitelist w ON w.tenant_id=p.tenant_id AND w.id=p.entry_id AND w.version=p.entry_version
		WHERE p.tenant_id=$1 AND p.match_value=$2 AND p.desired_state='effective'
		AND w.status='active' AND w.approval_status='approved' AND w.archived_at IS NULL
		AND (w.expires_at IS NULL OR w.expires_at > now())
		AND (p.expires_at IS NULL OR p.expires_at > now())`, tenantID, value).Scan(&count)
	return count > 0
}

// MatchDetection checks the durable rule-manager projection used by alert
// ingestion. Query failures are returned so callers can fail open and retain
// the alert while surfacing the broken enforcement dependency.
func (r *Repository) MatchDetection(ctx context.Context, tenantID, srcIP, dstIP, fingerprint string) (bool, error) {
	srcSubnet := ""
	if net.ParseIP(srcIP) != nil {
		srcSubnet = srcIP
	}
	dstSubnet := ""
	if net.ParseIP(dstIP) != nil {
		dstSubnet = dstIP
	}
	var matched bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM whitelist_rule_projection p
		JOIN whitelist w ON w.tenant_id=p.tenant_id AND w.id=p.entry_id AND w.version=p.entry_version
		WHERE p.tenant_id=$1 AND p.desired_state='effective'
		  AND w.status='active' AND w.approval_status='approved' AND w.archived_at IS NULL
		  AND (w.expires_at IS NULL OR w.expires_at > now())
		  AND (p.expires_at IS NULL OR p.expires_at > now())
		  AND (
		    (p.entry_type='ip' AND p.match_value IN ($2,$3))
		    OR (p.entry_type='fingerprint' AND p.match_value=$4)
		    OR (p.entry_type='subnet' AND (
		      NULLIF($5,'')::inet <<= p.match_value::inet
		      OR NULLIF($6,'')::inet <<= p.match_value::inet
		    ))
		  )
	)`, tenantID, srcIP, dstIP, fingerprint, srcSubnet, dstSubnet).Scan(&matched)
	if err != nil {
		return false, err
	}
	return matched, nil
}

// MatchesAlert 检查告警是否匹配白名单 (IP/指纹)
func (r *Repository) MatchesAlert(ctx context.Context, tenantID, srcIP, dstIP, fingerprint string) bool {
	matched, err := r.MatchDetection(ctx, tenantID, srcIP, dstIP, fingerprint)
	return err == nil && matched
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
		`SELECT EXISTS(SELECT 1 FROM whitelist_rule_projection p
		 JOIN whitelist w ON w.tenant_id=p.tenant_id AND w.id=p.entry_id AND w.version=p.entry_version
		 WHERE p.tenant_id=$1 AND p.entry_type='subnet' AND p.desired_state='effective'
		 AND w.status='active' AND w.approval_status='approved' AND w.archived_at IS NULL
		 AND (w.expires_at IS NULL OR w.expires_at > now())
		 AND (p.expires_at IS NULL OR p.expires_at > now()) AND $2::inet <<= p.match_value::inet)`, tenantID, ip).Scan(&exists)
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

const whitelistEntrySelect = `SELECT
	w.id::text,w.tenant_id,w.type,w.value,w.reason,w.description,w.status,w.approval_status,
	w.source_alert_id,w.feedback_id,w.owner_role,w.scope,w.risk_level,w.covered_alerts,w.covered_assets,
	w.version,w.created_by,w.approved_by,w.approved_at,w.disabled_at,w.expires_at,w.created_at,w.updated_at,
	w.archived_at,w.last_action_id,w.last_trace_id,COALESCE(e.status,''),COALESCE(e.desired_state,''),
	COALESCE(e.rule_revision,'')
	FROM whitelist w LEFT JOIN whitelist_rule_effects e ON e.tenant_id=w.tenant_id
	 AND e.entry_id=w.id AND e.entry_version=w.version`

type whitelistScanner interface{ Scan(...interface{}) error }

func scanWhitelistEntry(scanner whitelistScanner) (*Entry, error) {
	var entry Entry
	err := scanner.Scan(&entry.ID, &entry.TenantID, &entry.Type, &entry.Value, &entry.Reason, &entry.Description,
		&entry.Status, &entry.ApprovalStatus, &entry.SourceAlertID, &entry.FeedbackID, &entry.OwnerRole, &entry.Scope,
		&entry.RiskLevel, &entry.CoveredAlerts, &entry.CoveredAssets, &entry.Version, &entry.CreatedBy, &entry.ApprovedBy,
		&entry.ApprovedAt, &entry.DisabledAt, &entry.ExpiresAt, &entry.CreatedAt, &entry.UpdatedAt, &entry.ArchivedAt,
		&entry.LastActionID, &entry.LastTraceID, &entry.RuleEffectStatus, &entry.RuleDesiredState, &entry.RuleRevision)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

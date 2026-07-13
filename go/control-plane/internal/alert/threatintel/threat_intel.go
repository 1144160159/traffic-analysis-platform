// Threat Intelligence Service — IP/域名信誉查询 + 告警自动富化
//
// 业务价值: SOC 分析师看到告警时自动获得威胁情报上下文
//
//	已知恶意 IP → 标记为 "threat_intel:malicious_ip"
//	已知 C2 域名 → 标记为 "threat_intel:c2_domain"
//	已知扫描器 IP → 标记为 "threat_intel:scanner"
package threatintel

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Reputation 威胁情报信誉等级
type Reputation string

const (
	RepMalicious  Reputation = "malicious"
	RepSuspicious Reputation = "suspicious"
	RepScanner    Reputation = "scanner"
	RepC2         Reputation = "c2"
	RepClean      Reputation = "clean"
	RepUnknown    Reputation = "unknown"
)

// IntelEntry 威胁情报条目
type IntelEntry struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id,omitempty"`
	Type        string     `json:"type"` // ip | domain | hash
	Value       string     `json:"value"`
	Reputation  Reputation `json:"reputation"`
	Category    string     `json:"category"` // malware | phishing | c2 | scanner | botnet | proxy | tor
	Source      string     `json:"source"`   // feed name
	Description string     `json:"description"`
	LastSeen    time.Time  `json:"last_seen"`
}

type ListFilter struct {
	Type       string
	Reputation string
	Source     string
	Limit      int
	Offset     int
}

type FeedSource struct {
	ID              string       `json:"id,omitempty"`
	TenantID        string       `json:"tenant_id,omitempty"`
	Name            string       `json:"name"`
	Enabled         bool         `json:"enabled"`
	IntervalSeconds int          `json:"interval_seconds"`
	Entries         []IntelEntry `json:"entries"`
	LastRunAt       *time.Time   `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time   `json:"next_run_at,omitempty"`
	LastStatus      string       `json:"last_status"`
	LastError       string       `json:"last_error,omitempty"`
	RunCount        int          `json:"run_count"`
	CreatedAt       time.Time    `json:"created_at,omitempty"`
	UpdatedAt       time.Time    `json:"updated_at,omitempty"`
}

// Enrichment 告警富化结果
type Enrichment struct {
	IPs       map[string]Reputation `json:"ips"`
	Domains   map[string]Reputation `json:"domains,omitempty"`
	Tags      []string              `json:"tags"`
	RiskScore float32               `json:"risk_score"` // 0.0-1.0 威胁风险分
}

// Service 威胁情报服务
type Service struct {
	db     *sql.DB
	logger *zap.Logger
	mu     sync.RWMutex
	// 本地缓存 (避免重复查库)
	cache    map[string]*IntelEntry
	cacheTTL time.Duration
	// 内置威胁源 (快速启动，无需 DB)
	builtinThreats map[string]*IntelEntry
}

// NewService 创建威胁情报服务
func NewService(db *sql.DB, logger *zap.Logger) *Service {
	svc := &Service{
		db:       db,
		logger:   logger,
		cache:    make(map[string]*IntelEntry),
		cacheTTL: 10 * time.Minute,
	}
	svc.loadBuiltinThreats()
	return svc
}

// InitSchema 初始化威胁情报表
func (s *Service) InitSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		CREATE TABLE IF NOT EXISTS threat_intel (
			id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id   TEXT NOT NULL DEFAULT 'default',
			type        TEXT NOT NULL CHECK (type IN ('ip','domain','hash')),
			value       TEXT NOT NULL,
			reputation  TEXT NOT NULL DEFAULT 'unknown',
			category    TEXT NOT NULL DEFAULT '',
			source      TEXT NOT NULL DEFAULT 'manual',
			description TEXT NOT NULL DEFAULT '',
			last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(tenant_id, type, value)
		);
		ALTER TABLE threat_intel ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
		ALTER TABLE threat_intel ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
		ALTER TABLE threat_intel DROP CONSTRAINT IF EXISTS threat_intel_type_value_key;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_threat_intel_tenant_type_value ON threat_intel(tenant_id, type, value);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_value ON threat_intel(type, value);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_tenant_value ON threat_intel(tenant_id, type, value);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_rep ON threat_intel(reputation);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_source ON threat_intel(source);
		CREATE TABLE IF NOT EXISTS threat_intel_feeds (
			id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id        TEXT NOT NULL DEFAULT 'default',
			name             TEXT NOT NULL,
			enabled          BOOLEAN NOT NULL DEFAULT true,
			interval_seconds INTEGER NOT NULL DEFAULT 3600 CHECK (interval_seconds >= 1),
			entries          JSONB NOT NULL DEFAULT '[]'::jsonb,
			last_run_at      TIMESTAMPTZ,
			next_run_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_status      TEXT NOT NULL DEFAULT 'never',
			last_error       TEXT NOT NULL DEFAULT '',
			run_count        INTEGER NOT NULL DEFAULT 0,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(tenant_id, name)
		);
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT true;
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS interval_seconds INTEGER NOT NULL DEFAULT 3600;
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS entries JSONB NOT NULL DEFAULT '[]'::jsonb;
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS last_run_at TIMESTAMPTZ;
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS next_run_at TIMESTAMPTZ NOT NULL DEFAULT now();
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS last_status TEXT NOT NULL DEFAULT 'never';
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS run_count INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
		ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
		CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_due ON threat_intel_feeds(enabled, next_run_at);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_tenant ON threat_intel_feeds(tenant_id, name);`)
	return err
}

// ---- 内置威胁源 ----

func (s *Service) loadBuiltinThreats() {
	s.builtinThreats = map[string]*IntelEntry{
		// 已知 C2 服务器 (示例)
		"ip:185.130.5.253": {Type: "ip", Value: "185.130.5.253", Reputation: RepC2, Category: "c2", Source: "builtin", Description: "Known Cobalt Strike C2"},
		"ip:103.253.41.45": {Type: "ip", Value: "103.253.41.45", Reputation: RepC2, Category: "c2", Source: "builtin", Description: "Known Metasploit C2"},
		// 已知扫描器
		"ip:71.6.135.131":   {Type: "ip", Value: "71.6.135.131", Reputation: RepScanner, Category: "scanner", Source: "builtin", Description: "Shodan scanner"},
		"ip:66.240.236.119": {Type: "ip", Value: "66.240.236.119", Reputation: RepScanner, Category: "scanner", Source: "builtin", Description: "Censys scanner"},
		// 已知恶意 IP
		"ip:5.188.87.0/24": {Type: "ip", Value: "5.188.87.0/24", Reputation: RepMalicious, Category: "malware", Source: "builtin", Description: "Known malware distribution subnet"},
		// 已知钓鱼域名
		"domain:evil-phish.com":  {Type: "domain", Value: "evil-phish.com", Reputation: RepMalicious, Category: "phishing", Source: "builtin", Description: "Known phishing domain"},
		"domain:malware-cdn.net": {Type: "domain", Value: "malware-cdn.net", Reputation: RepMalicious, Category: "malware", Source: "builtin", Description: "Malware CDN"},
	}
}

// ---- 查询 ----

// CheckIP 查询 IP 信誉
func (s *Service) CheckIP(ctx context.Context, ip string) Reputation {
	return s.CheckIPForTenant(ctx, "default", ip)
}

func (s *Service) CheckIPForTenant(ctx context.Context, tenantID, ip string) Reputation {
	entry, found, err := s.LookupForTenant(ctx, tenantID, "ip", ip)
	if err != nil {
		s.logger.Warn("Threat intel IP lookup failed", zap.String("ip", ip), zap.Error(err))
		return RepUnknown
	}
	if !found {
		return RepUnknown
	}
	return entry.Reputation
}

// CheckDomain 查询域名信誉
func (s *Service) CheckDomain(ctx context.Context, domain string) Reputation {
	return s.CheckDomainForTenant(ctx, "default", domain)
}

func (s *Service) CheckDomainForTenant(ctx context.Context, tenantID, domain string) Reputation {
	entry, found, err := s.LookupForTenant(ctx, tenantID, "domain", domain)
	if err != nil {
		s.logger.Warn("Threat intel domain lookup failed", zap.String("domain", domain), zap.Error(err))
		return RepUnknown
	}
	if !found {
		return RepUnknown
	}
	return entry.Reputation
}

func (s *Service) builtinLookup(typ, value string) (*IntelEntry, bool) {
	if value == "" {
		return nil, false
	}
	// 内置威胁源
	key := typ + ":" + value
	s.mu.RLock()
	if entry, ok := s.builtinThreats[key]; ok {
		s.mu.RUnlock()
		return cloneEntry(entry), true
	}
	// 子网匹配 (仅 IP)
	if typ == "ip" {
		for bk, entry := range s.builtinThreats {
			if strings.HasSuffix(bk, "/24") {
				subnet := bk[3:] // "ip:5.188.87.0/24" → "5.188.87.0/24"
				if s.matchesSubnet(value, subnet) {
					s.mu.RUnlock()
					return cloneEntry(entry), true
				}
			}
		}
	}
	s.mu.RUnlock()
	return nil, false
}

// Lookup 查询威胁情报条目。优先内置源，再查 PostgreSQL。
func (s *Service) Lookup(ctx context.Context, typ, value string) (*IntelEntry, bool, error) {
	return s.LookupForTenant(ctx, "default", typ, value)
}

func (s *Service) LookupForTenant(ctx context.Context, tenantID, typ, value string) (*IntelEntry, bool, error) {
	tenantID = normalizeTenantID(tenantID)
	typ = normalizeType(typ)
	value = normalizeValue(typ, value)
	if typ == "" || value == "" {
		return nil, false, nil
	}

	if entry, ok := s.builtinLookup(typ, value); ok {
		return entry, true, nil
	}

	s.mu.RLock()
	if entry, ok := s.cache[tenantID+":"+typ+":"+value]; ok {
		s.mu.RUnlock()
		return cloneEntry(entry), true, nil
	}
	s.mu.RUnlock()

	if s.db == nil {
		return nil, false, nil
	}

	var entry IntelEntry
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, tenant_id, type, value, reputation, category, source, description, last_seen
		FROM threat_intel
		WHERE tenant_id = $1 AND type = $2 AND value = $3
	`, tenantID, typ, value).Scan(
		&entry.ID,
		&entry.TenantID,
		&entry.Type,
		&entry.Value,
		&entry.Reputation,
		&entry.Category,
		&entry.Source,
		&entry.Description,
		&entry.LastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	return &entry, true, nil
}

func (s *Service) matchesSubnet(ip, cidr string) bool {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return subnet.Contains(parsed)
}

// EnrichAlert 使用威胁情报富化告警
func (s *Service) EnrichAlert(ctx context.Context, srcIP, dstIP string) *Enrichment {
	return s.EnrichAlertForTenant(ctx, "default", srcIP, dstIP)
}

func (s *Service) EnrichAlertForTenant(ctx context.Context, tenantID, srcIP, dstIP string) *Enrichment {
	enrich := &Enrichment{
		IPs:  make(map[string]Reputation),
		Tags: make([]string, 0),
	}

	// 检查源 IP
	srcRep := s.CheckIPForTenant(ctx, tenantID, srcIP)
	if srcRep != RepUnknown && srcRep != RepClean {
		enrich.IPs[srcIP] = srcRep
		enrich.Tags = append(enrich.Tags, "threat_intel:src_"+string(srcRep))
		enrich.RiskScore += 0.3
	}

	// 检查目的 IP
	dstRep := s.CheckIPForTenant(ctx, tenantID, dstIP)
	if dstRep != RepUnknown && dstRep != RepClean {
		enrich.IPs[dstIP] = dstRep
		enrich.Tags = append(enrich.Tags, "threat_intel:dst_"+string(dstRep))
		enrich.RiskScore += 0.3
	}

	// 风险分上限
	if enrich.RiskScore > 1.0 {
		enrich.RiskScore = 1.0
	}
	if enrich.RiskScore > 0 {
		enrich.Tags = append(enrich.Tags, fmt.Sprintf("threat_risk:%.2f", enrich.RiskScore))
	}

	return enrich
}

// ImportFromFile 从 JSON 文件导入威胁情报
func (s *Service) ImportFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read threat intel file: %w", err)
	}
	var entries []IntelEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse threat intel: %w", err)
	}
	for _, e := range entries {
		s.builtinThreats[e.Type+":"+e.Value] = &e
	}
	s.logger.Info("Imported threat intel", zap.Int("count", len(entries)), zap.String("source", path))
	return nil
}

// Insert 插入威胁情报条目到数据库
func (s *Service) Insert(ctx context.Context, entry *IntelEntry) error {
	return s.InsertForTenant(ctx, "default", entry)
}

func (s *Service) InsertForTenant(ctx context.Context, tenantID string, entry *IntelEntry) error {
	if entry == nil {
		return fmt.Errorf("nil threat intel entry")
	}
	entry.TenantID = normalizeTenantID(tenantID)
	if err := normalizeEntry(entry); err != nil {
		return err
	}
	if s.db == nil {
		s.mu.Lock()
		s.builtinThreats[entry.TenantID+":"+entry.Type+":"+entry.Value] = cloneEntry(entry)
		s.mu.Unlock()
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO threat_intel (tenant_id, type, value, reputation, category, source, description, last_seen)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (tenant_id, type, value) DO UPDATE SET
		   reputation=$4,
		   category=$5,
		   source=$6,
		   description=$7,
		   last_seen=$8,
		   updated_at=now()`,
		entry.TenantID, entry.Type, entry.Value, entry.Reputation, entry.Category, entry.Source, entry.Description, entry.LastSeen)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.cache[entry.TenantID+":"+entry.Type+":"+entry.Value] = cloneEntry(entry)
	s.mu.Unlock()
	return nil
}

func (s *Service) ImportEntries(ctx context.Context, entries []IntelEntry, defaultSource string) (int, error) {
	return s.ImportEntriesForTenant(ctx, "default", entries, defaultSource)
}

func (s *Service) ImportEntriesForTenant(ctx context.Context, tenantID string, entries []IntelEntry, defaultSource string) (int, error) {
	imported := 0
	for i := range entries {
		if entries[i].Source == "" {
			entries[i].Source = defaultSource
		}
		if err := s.InsertForTenant(ctx, tenantID, &entries[i]); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]IntelEntry, int64, error) {
	return s.ListForTenant(ctx, "default", filter)
}

func (s *Service) ListForTenant(ctx context.Context, tenantID string, filter ListFilter) ([]IntelEntry, int64, error) {
	tenantID = normalizeTenantID(tenantID)
	if s.db == nil {
		entries := make([]IntelEntry, 0, len(s.builtinThreats))
		s.mu.RLock()
		for _, entry := range s.builtinThreats {
			if entry.TenantID != "" && entry.TenantID != tenantID {
				continue
			}
			if filter.Type != "" && entry.Type != normalizeType(filter.Type) {
				continue
			}
			if filter.Reputation != "" && string(entry.Reputation) != filter.Reputation {
				continue
			}
			if filter.Source != "" && entry.Source != filter.Source {
				continue
			}
			entries = append(entries, *cloneEntry(entry))
		}
		s.mu.RUnlock()
		return entries, int64(len(entries)), nil
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	clauses := make([]string, 0, 3)
	args := make([]interface{}, 0, 5)
	args = append(args, tenantID)
	clauses = append(clauses, "tenant_id = $1")
	if typ := normalizeType(filter.Type); typ != "" {
		args = append(args, typ)
		clauses = append(clauses, fmt.Sprintf("type = $%d", len(args)))
	}
	if filter.Reputation != "" {
		args = append(args, strings.TrimSpace(filter.Reputation))
		clauses = append(clauses, fmt.Sprintf("reputation = $%d", len(args)))
	}
	if filter.Source != "" {
		args = append(args, strings.TrimSpace(filter.Source))
		clauses = append(clauses, fmt.Sprintf("source = $%d", len(args)))
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	var total int64
	countSQL := "SELECT count(*) FROM threat_intel " + where
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id::text, tenant_id, type, value, reputation, category, source, description, last_seen
		FROM threat_intel
		%s
		ORDER BY last_seen DESC, value ASC
		LIMIT $%d OFFSET $%d
	`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]IntelEntry, 0, limit)
	for rows.Next() {
		var entry IntelEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.TenantID,
			&entry.Type,
			&entry.Value,
			&entry.Reputation,
			&entry.Category,
			&entry.Source,
			&entry.Description,
			&entry.LastSeen,
		); err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	return entries, total, rows.Err()
}

func (s *Service) UpsertFeed(ctx context.Context, feed *FeedSource) (*FeedSource, error) {
	if feed == nil {
		return nil, fmt.Errorf("nil threat intel feed")
	}
	normalizeFeed(feed)
	if feed.Name == "" {
		return nil, fmt.Errorf("feed name is required")
	}
	if feed.IntervalSeconds <= 0 {
		feed.IntervalSeconds = 3600
	}
	if feed.TenantID == "" {
		feed.TenantID = "default"
	}
	if feed.NextRunAt == nil {
		now := time.Now().UTC()
		feed.NextRunAt = &now
	}
	if feed.LastStatus == "" {
		feed.LastStatus = "configured"
	}
	entriesJSON, err := json.Marshal(feed.Entries)
	if err != nil {
		return nil, err
	}
	if s.db == nil {
		return feed, nil
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO threat_intel_feeds (tenant_id, name, enabled, interval_seconds, entries, next_run_at, last_status, last_error)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8)
		ON CONFLICT (tenant_id, name) DO UPDATE SET
		  enabled=$3,
		  interval_seconds=$4,
		  entries=$5::jsonb,
		  next_run_at=$6,
		  last_status=$7,
		  last_error=$8,
		  updated_at=now()
		RETURNING id::text, tenant_id, name, enabled, interval_seconds, entries, last_run_at, next_run_at, last_status, last_error, run_count, created_at, updated_at`,
		feed.TenantID,
		feed.Name,
		feed.Enabled,
		feed.IntervalSeconds,
		string(entriesJSON),
		feed.NextRunAt,
		feed.LastStatus,
		feed.LastError,
	)
	return scanFeed(row)
}

func (s *Service) ListFeeds(ctx context.Context, tenantID, name string, enabledOnly bool) ([]FeedSource, error) {
	if s.db == nil {
		return nil, nil
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	clauses := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	if name = normalizeFeedName(name); name != "" {
		args = append(args, name)
		clauses = append(clauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if enabledOnly {
		clauses = append(clauses, "enabled = true")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id::text, tenant_id, name, enabled, interval_seconds, entries, last_run_at, next_run_at, last_status, last_error, run_count, created_at, updated_at
		FROM threat_intel_feeds
		WHERE %s
		ORDER BY enabled DESC, next_run_at ASC, name ASC
		LIMIT 200`, strings.Join(clauses, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	feeds := make([]FeedSource, 0)
	for rows.Next() {
		feed, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, *feed)
	}
	return feeds, rows.Err()
}

func (s *Service) GetFeed(ctx context.Context, tenantID, name string) (*FeedSource, error) {
	feeds, err := s.ListFeeds(ctx, tenantID, name, false)
	if err != nil {
		return nil, err
	}
	if len(feeds) == 0 {
		return nil, sql.ErrNoRows
	}
	return &feeds[0], nil
}

func (s *Service) DueFeeds(ctx context.Context, now time.Time, limit int) ([]FeedSource, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, tenant_id, name, enabled, interval_seconds, entries, last_run_at, next_run_at, last_status, last_error, run_count, created_at, updated_at
		FROM threat_intel_feeds
		WHERE enabled = true AND next_run_at <= $1
		ORDER BY next_run_at ASC, name ASC
		LIMIT $2`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	feeds := make([]FeedSource, 0, limit)
	for rows.Next() {
		feed, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, *feed)
	}
	return feeds, rows.Err()
}

func (s *Service) RecordFeedRun(ctx context.Context, feed FeedSource, status, lastError string, ranAt time.Time) error {
	if s.db == nil {
		return nil
	}
	normalizeFeed(&feed)
	if feed.TenantID == "" || feed.Name == "" {
		return fmt.Errorf("feed tenant_id and name are required")
	}
	if ranAt.IsZero() {
		ranAt = time.Now().UTC()
	}
	nextRunAt := ranAt.Add(time.Duration(feed.IntervalSeconds) * time.Second)
	_, err := s.db.ExecContext(ctx, `
		UPDATE threat_intel_feeds
		SET last_run_at=$1,
		    next_run_at=$2,
		    last_status=$3,
		    last_error=$4,
		    run_count=run_count+1,
		    updated_at=now()
		WHERE tenant_id=$5 AND name=$6`,
		ranAt.UTC(),
		nextRunAt.UTC(),
		status,
		lastError,
		feed.TenantID,
		feed.Name)
	return err
}

func normalizeEntry(entry *IntelEntry) error {
	entry.TenantID = normalizeTenantID(entry.TenantID)
	entry.Type = normalizeType(entry.Type)
	entry.Value = normalizeValue(entry.Type, entry.Value)
	entry.Source = strings.TrimSpace(entry.Source)
	entry.Category = strings.TrimSpace(entry.Category)
	entry.Description = strings.TrimSpace(entry.Description)
	if entry.Type == "" {
		return fmt.Errorf("type must be one of ip, domain, hash")
	}
	if entry.Value == "" {
		return fmt.Errorf("value is required")
	}
	if entry.Reputation == "" {
		entry.Reputation = RepUnknown
	}
	if entry.Source == "" {
		entry.Source = "manual"
	}
	if entry.LastSeen.IsZero() {
		entry.LastSeen = time.Now().UTC()
	}
	return nil
}

func normalizeTenantID(tenantID string) string {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "default"
	}
	return tenantID
}

type feedScanner interface {
	Scan(dest ...interface{}) error
}

func scanFeed(scanner feedScanner) (*FeedSource, error) {
	var feed FeedSource
	var entriesJSON []byte
	if err := scanner.Scan(
		&feed.ID,
		&feed.TenantID,
		&feed.Name,
		&feed.Enabled,
		&feed.IntervalSeconds,
		&entriesJSON,
		&feed.LastRunAt,
		&feed.NextRunAt,
		&feed.LastStatus,
		&feed.LastError,
		&feed.RunCount,
		&feed.CreatedAt,
		&feed.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(entriesJSON) > 0 {
		if err := json.Unmarshal(entriesJSON, &feed.Entries); err != nil {
			return nil, err
		}
	}
	return &feed, nil
}

func normalizeFeed(feed *FeedSource) {
	feed.TenantID = strings.TrimSpace(feed.TenantID)
	feed.Name = normalizeFeedName(feed.Name)
	if feed.IntervalSeconds <= 0 {
		feed.IntervalSeconds = 3600
	}
	if feed.LastStatus == "" {
		feed.LastStatus = "configured"
	}
}

func normalizeFeedName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "ip", "domain", "hash":
		return strings.ToLower(strings.TrimSpace(typ))
	default:
		return ""
	}
}

func normalizeValue(typ, value string) string {
	value = strings.TrimSpace(value)
	switch typ {
	case "domain", "hash":
		return strings.ToLower(value)
	default:
		return value
	}
}

func cloneEntry(entry *IntelEntry) *IntelEntry {
	if entry == nil {
		return nil
	}
	clone := *entry
	return &clone
}

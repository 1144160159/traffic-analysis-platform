// Threat Intelligence Service — IP/域名信誉查询 + 告警自动富化
//
// 业务价值: SOC 分析师看到告警时自动获得威胁情报上下文
//   已知恶意 IP → 标记为 "threat_intel:malicious_ip"
//   已知 C2 域名 → 标记为 "threat_intel:c2_domain"
//   已知扫描器 IP → 标记为 "threat_intel:scanner"
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
	Type        string     `json:"type"` // ip | domain | hash
	Value       string     `json:"value"`
	Reputation  Reputation `json:"reputation"`
	Category    string     `json:"category"` // malware | phishing | c2 | scanner | botnet | proxy | tor
	Source      string     `json:"source"`   // feed name
	Description string     `json:"description"`
	LastSeen    time.Time  `json:"last_seen"`
}

// Enrichment 告警富化结果
type Enrichment struct {
	IPs        map[string]Reputation `json:"ips"`
	Domains    map[string]Reputation `json:"domains,omitempty"`
	Tags       []string              `json:"tags"`
	RiskScore  float32               `json:"risk_score"` // 0.0-1.0 威胁风险分
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
		CREATE TABLE IF NOT EXISTS threat_intel (
			id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			type        TEXT NOT NULL CHECK (type IN ('ip','domain','hash')),
			value       TEXT NOT NULL,
			reputation  TEXT NOT NULL DEFAULT 'unknown',
			category    TEXT NOT NULL DEFAULT '',
			source      TEXT NOT NULL DEFAULT 'manual',
			description TEXT NOT NULL DEFAULT '',
			last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE(type, value)
		);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_value ON threat_intel(type, value);
		CREATE INDEX IF NOT EXISTS idx_threat_intel_rep ON threat_intel(reputation);`)
	return err
}

// ---- 内置威胁源 ----

func (s *Service) loadBuiltinThreats() {
	s.builtinThreats = map[string]*IntelEntry{
		// 已知 C2 服务器 (示例)
		"ip:185.130.5.253":     {Type: "ip", Value: "185.130.5.253", Reputation: RepC2, Category: "c2", Source: "builtin", Description: "Known Cobalt Strike C2"},
		"ip:103.253.41.45":     {Type: "ip", Value: "103.253.41.45", Reputation: RepC2, Category: "c2", Source: "builtin", Description: "Known Metasploit C2"},
		// 已知扫描器
		"ip:71.6.135.131":      {Type: "ip", Value: "71.6.135.131", Reputation: RepScanner, Category: "scanner", Source: "builtin", Description: "Shodan scanner"},
		"ip:66.240.236.119":    {Type: "ip", Value: "66.240.236.119", Reputation: RepScanner, Category: "scanner", Source: "builtin", Description: "Censys scanner"},
		// 已知恶意 IP
		"ip:5.188.87.0/24":     {Type: "ip", Value: "5.188.87.0/24", Reputation: RepMalicious, Category: "malware", Source: "builtin", Description: "Known malware distribution subnet"},
		// 已知钓鱼域名
		"domain:evil-phish.com":   {Type: "domain", Value: "evil-phish.com", Reputation: RepMalicious, Category: "phishing", Source: "builtin", Description: "Known phishing domain"},
		"domain:malware-cdn.net":  {Type: "domain", Value: "malware-cdn.net", Reputation: RepMalicious, Category: "malware", Source: "builtin", Description: "Malware CDN"},
	}
}

// ---- 查询 ----

// CheckIP 查询 IP 信誉
func (s *Service) CheckIP(ctx context.Context, ip string) Reputation {
	return s.check("ip", ip)
}

// CheckDomain 查询域名信誉
func (s *Service) CheckDomain(ctx context.Context, domain string) Reputation {
	return s.check("domain", domain)
}

func (s *Service) check(typ, value string) Reputation {
	if value == "" { return RepUnknown }
	// 内置威胁源
	key := typ + ":" + value
	s.mu.RLock()
	if entry, ok := s.builtinThreats[key]; ok {
		s.mu.RUnlock()
		return entry.Reputation
	}
	// 子网匹配 (仅 IP)
	if typ == "ip" {
		for bk, entry := range s.builtinThreats {
			if strings.HasSuffix(bk, "/24") {
				subnet := bk[3:] // "ip:5.188.87.0/24" → "5.188.87.0/24"
				if s.matchesSubnet(value, subnet) {
					s.mu.RUnlock()
					return entry.Reputation
				}
			}
		}
	}
	s.mu.RUnlock()
	return RepUnknown
}

func (s *Service) matchesSubnet(ip, cidr string) bool {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil { return false }
	parsed := net.ParseIP(ip)
	if parsed == nil { return false }
	return subnet.Contains(parsed)
}

// EnrichAlert 使用威胁情报富化告警
func (s *Service) EnrichAlert(ctx context.Context, srcIP, dstIP string) *Enrichment {
	enrich := &Enrichment{
		IPs:  make(map[string]Reputation),
		Tags: make([]string, 0),
	}

	// 检查源 IP
	srcRep := s.CheckIP(ctx, srcIP)
	if srcRep != RepUnknown && srcRep != RepClean {
		enrich.IPs[srcIP] = srcRep
		enrich.Tags = append(enrich.Tags, "threat_intel:src_"+string(srcRep))
		enrich.RiskScore += 0.3
	}

	// 检查目的 IP
	dstRep := s.CheckIP(ctx, dstIP)
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
	if err != nil { return fmt.Errorf("read threat intel file: %w", err) }
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
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO threat_intel (type, value, reputation, category, source, description, last_seen)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (type, value) DO UPDATE SET reputation=$3, category=$4, last_seen=$7`,
		entry.Type, entry.Value, entry.Reputation, entry.Category, entry.Source, entry.Description, time.Now())
	return err
}

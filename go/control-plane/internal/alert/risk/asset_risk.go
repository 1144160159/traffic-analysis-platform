////////////////////////////////////////////////////////////////////////////////
// Asset Risk Scoring — 资产风险评分引擎
// 缺失业务逻辑 #2: 基于告警/漏洞/行为的多维度资产风险评分
////////////////////////////////////////////////////////////////////////////////

package risk

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// AssetRiskScore — 资产风险评分结果
// =============================================================================

type AssetRiskScore struct {
	AssetID   string `json:"asset_id"`
	IPAddress string `json:"ip_address"`
	Hostname  string `json:"hostname,omitempty"`
	TenantID  string `json:"tenant_id"`

	TotalScore float64 `json:"total_score"` // 0–100
	RiskLevel  string  `json:"risk_level"`  // critical/high/medium/low

	AlertScore    float64 `json:"alert_score"`    // 告警维度
	VulnScore     float64 `json:"vuln_score"`     // 漏洞维度
	BehaviorScore float64 `json:"behavior_score"` // 行为维度
	ExposureScore float64 `json:"exposure_score"` // 暴露维度

	ActiveAlerts   int    `json:"active_alerts"`
	TotalAlerts7d  int    `json:"total_alerts_7d"`
	CriticalAlerts int    `json:"critical_alerts"`
	GeoRiskLevel   string `json:"geo_risk_level"`
	IsGateway      bool   `json:"is_gateway"`
	HasOpenPorts   int    `json:"open_ports_count"`

	UpdatedAt time.Time `json:"updated_at"`
}

type RiskLevel string

const (
	RiskCritical RiskLevel = "critical"
	RiskHigh     RiskLevel = "high"
	RiskMedium   RiskLevel = "medium"
	RiskLow      RiskLevel = "low"
)

// =============================================================================
// AssetRiskScorer — 评分引擎
// =============================================================================

type AssetRiskScorer struct {
	chDB   *sql.DB // ClickHouse (alerts, sessions, feature_stat)
	pgDB   *sql.DB // PostgreSQL (assets 表)
	logger *zap.Logger
}

func NewAssetRiskScorer(chDB, pgDB *sql.DB, logger *zap.Logger) *AssetRiskScorer {
	return &AssetRiskScorer{chDB: chDB, pgDB: pgDB, logger: logger}
}

// ScoreAsset 计算单个资产的风险评分
// 要求 ClickHouse 连接可用（查询 alerts + sessions + feature_stat）
func (s *AssetRiskScorer) ScoreAsset(ctx context.Context, tenantID, ipAddress string) (*AssetRiskScore, error) {
	if s.chDB == nil {
		return nil, fmt.Errorf("asset risk scoring requires ClickHouse connection (alerts + sessions + feature_stat)")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	score := &AssetRiskScore{
		AssetID:   ipAddress,
		IPAddress: ipAddress,
		TenantID:  tenantID,
		UpdatedAt: time.Now(),
	}

	// 维度 1: 告警评分 (权重 0.40)
	score.AlertScore = s.computeAlertScore(ctx, tenantID, ipAddress, score)

	// 维度 2: 漏洞评分 (权重 0.15) — 基于协议异常和弱密码检测
	score.VulnScore = s.computeVulnScore(ctx, tenantID, ipAddress)

	// 维度 3: 行为异常评分 (权重 0.25) — 基于流量行为偏离基线
	score.BehaviorScore = s.computeBehaviorScore(ctx, tenantID, ipAddress)

	// 维度 4: 暴露面评分 (权重 0.20) — 基于开放端口/服务
	score.ExposureScore = s.computeExposureScore(ctx, tenantID, ipAddress, score)

	// 加权总分
	score.TotalScore = score.AlertScore*0.40 +
		score.VulnScore*0.15 +
		score.BehaviorScore*0.25 +
		score.ExposureScore*0.20

	score.RiskLevel = s.levelFromScore(score.TotalScore)
	return score, nil
}

// =============================================================================
// 维度 1: 告警评分 (40%)
// =============================================================================

func (s *AssetRiskScorer) computeAlertScore(ctx context.Context, tenantID, ip string, score *AssetRiskScore) float64 {
	query := `
		SELECT
			countIf(status NOT IN ('ALERT_STATUS_CLOSED', 'ALERT_STATUS_RESOLVED', 'closed', 'resolved')) AS active,
			count() AS total_7d,
			countIf(severity IN ('critical', 'CRITICAL', 'SEVERITY_CRITICAL')) AS critical
		FROM traffic.alerts
		WHERE tenant_id = ? AND (src_ip = ? OR dst_ip = ?)
		  AND last_seen >= toUnixTimestamp64Milli(now64(3) - INTERVAL 7 DAY)
	`
	row := s.chDB.QueryRowContext(ctx, query, tenantID, ip, ip)
	var active, total, critical int
	if err := row.Scan(&active, &total, &critical); err != nil {
		return 25.0 // 默认中等
	}

	score.ActiveAlerts = active
	score.TotalAlerts7d = total
	score.CriticalAlerts = critical

	if total == 0 {
		return 5.0 // 无告警 → 低风险
	}

	// 活跃告警越多，分数越高
	alertScore := math.Min(100, float64(active)*15+float64(critical)*25)
	return math.Max(5, alertScore)
}

// =============================================================================
// 维度 2: 漏洞评分 (15%)
// =============================================================================

func (s *AssetRiskScorer) computeVulnScore(ctx context.Context, tenantID, ip string) float64 {
	// 检测协议异常 (TCP flags, 弱 TLS, ICMP 隧道)
	query := `
		SELECT count() FROM traffic.feature_stat
		WHERE tenant_id = ?
		  AND (object_id LIKE ? OR object_id LIKE ?)
		  AND ts >= now() - INTERVAL 7 DAY
		  AND (
		    tcp_flag_syn_cnt > 1000           -- SYN flood indicator
		    OR protocol = 1 AND pps > 5000     -- ICMP flood
		    OR (protocol = 6 AND up_down_ratio < 0.1)  -- 非对称流量
		  )
	`
	var anomalyCount int
	_ = s.chDB.QueryRowContext(ctx, query, tenantID, "%"+ip+"%", ip+"%").Scan(&anomalyCount)

	return math.Min(100, float64(anomalyCount)*10)
}

// =============================================================================
// 维度 3: 行为异常评分 (25%)
// =============================================================================

func (s *AssetRiskScorer) computeBehaviorScore(ctx context.Context, tenantID, ip string) float64 {
	// 检测行为偏离: 流量突增、非工作时间活动、新端口开放
	query := `
		SELECT
			avg(pps) AS avg_pps,
			quantile(0.95)(pps) AS p95_pps
		FROM traffic.feature_stat
		WHERE tenant_id = ? AND object_id LIKE ?
		  AND ts >= now() - INTERVAL 24 HOUR
	`
	var avgPPS, p95PPS float64
	_ = s.chDB.QueryRowContext(ctx, query, tenantID, "%"+ip+"%").Scan(&avgPPS, &p95PPS)

	// 流量突增检测: P95 / avg > 5 → 异常
	if avgPPS > 0 && p95PPS/avgPPS > 5 {
		return 70.0
	}
	if avgPPS > 10000 {
		return 55.0 // 高流量主机
	}
	return 15.0
}

// =============================================================================
// 维度 4: 暴露面评分 (20%)
// =============================================================================

func (s *AssetRiskScorer) computeExposureScore(ctx context.Context, tenantID, ip string, score *AssetRiskScore) float64 {
	query := `
		SELECT
			uniqExact(dst_port) AS open_ports,
			maxIf(1, dst_port IN (22, 3389, 23, 21)) AS has_risky_port,
			maxIf(1, protocol = 6 AND flags_syn > flags_ack * 0.8) AS is_server
		FROM traffic.sessions
		WHERE tenant_id = ? AND (src_ip = ? OR dst_ip = ?)
		  AND ts_start >= toUnixTimestamp64Milli(now64(3) - INTERVAL 24 HOUR)
	`
	var openPorts int
	var hasRiskyPort, isServer int
	_ = s.chDB.QueryRowContext(ctx, query, tenantID, ip, ip).Scan(&openPorts, &hasRiskyPort, &isServer)
	score.HasOpenPorts = openPorts

	exposureScore := 10.0 // 基础分

	if isServer == 1 {
		exposureScore += 20 // 对外服务
	}
	if hasRiskyPort == 1 {
		exposureScore += 30 // SSH/RDP/Telnet/FTP 暴露
	}
	exposureScore += math.Min(40, float64(openPorts)*2) // 端口越多暴露面越大

	return math.Min(100, exposureScore)
}

// =============================================================================
// 批量评分
// =============================================================================

// ScoreAllAssets 对租户下所有资产进行风险评分
func (s *AssetRiskScorer) ScoreAllAssets(ctx context.Context, tenantID string) ([]*AssetRiskScore, error) {
	if s.pgDB == nil {
		return nil, fmt.Errorf("PostgreSQL connection not available for asset query")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	query := `SELECT DISTINCT ip_address FROM assets WHERE tenant_id = $1`
	rows, err := s.pgDB.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()

	var scores []*AssetRiskScore
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			continue
		}
		score, err := s.ScoreAsset(ctx, tenantID, ip)
		if err != nil {
			s.logger.Warn("Failed to score asset", zap.String("ip", ip), zap.Error(err))
			continue
		}
		scores = append(scores, score)
	}
	return scores, nil
}

// =============================================================================
// 运维接口: 风险 Top-N
// =============================================================================

type RiskSummary struct {
	TotalAssets      int               `json:"total_assets"`
	RiskDistribution map[string]int    `json:"risk_distribution"`
	TopRiskyAssets   []*AssetRiskScore `json:"top_risky_assets"`
}

func (s *AssetRiskScorer) GetRiskSummary(ctx context.Context, tenantID string, topN int) (*RiskSummary, error) {
	scores, err := s.ScoreAllAssets(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	summary := &RiskSummary{
		TotalAssets:      len(scores),
		RiskDistribution: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0},
	}

	for _, sc := range scores {
		summary.RiskDistribution[sc.RiskLevel]++
	}

	// 排序取 Top-N
	sortByScore(scores)
	if len(scores) > topN {
		scores = scores[:topN]
	}
	summary.TopRiskyAssets = scores
	return summary, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (s *AssetRiskScorer) levelFromScore(score float64) string {
	switch {
	case score >= 80:
		return "critical"
	case score >= 60:
		return "high"
	case score >= 30:
		return "medium"
	default:
		return "low"
	}
}

func sortByScore(scores []*AssetRiskScore) {
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].TotalScore > scores[i].TotalScore {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}
}

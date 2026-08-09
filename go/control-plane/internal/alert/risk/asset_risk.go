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
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	maxRiskSummaryCandidates = 500
	alertDimensionWeight     = 0.40
	exposureDimensionWeight  = 0.20
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

	AvailableDimensions []string  `json:"available_dimensions"`
	MissingDimensions   []string  `json:"missing_dimensions,omitempty"`
	Partial             bool      `json:"partial"`
	UpdatedAt           time.Time `json:"updated_at"`
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

// ScoreAsset 计算单个资产的风险评分。
//
// feature_stat.object_id 当前保存的是会话 ID，而非资产 IP；在建立可对账的
// session/asset 血缘投影前，漏洞和行为维度必须显式标记为缺失，不能用 IP 模糊
// 匹配制造貌似完整的分数。
func (s *AssetRiskScorer) ScoreAsset(ctx context.Context, tenantID, ipAddress string) (*AssetRiskScore, error) {
	if s.chDB == nil {
		return nil, fmt.Errorf("asset risk scoring requires ClickHouse connection (alerts + sessions)")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if strings.TrimSpace(ipAddress) == "" {
		return nil, fmt.Errorf("asset IP address is required")
	}

	scores, _, _, err := s.scoreIPBatch(ctx, tenantID, []string{ipAddress})
	if err != nil {
		return nil, err
	}
	return scores[0], nil
}

// =============================================================================
// 批量维度查询
// =============================================================================

type alertRiskAggregate struct {
	active   int
	total    int
	critical int
}

type exposureRiskAggregate struct {
	observedDestinationPorts int
	hasRiskyPort             int
	isServer                 int
}

// clickHouseArrayPlaceholders returns a ClickHouse array expression whose
// values remain bound parameters. Candidate IPs come from PostgreSQL, but are
// still never interpolated into SQL text.
func clickHouseArrayPlaceholders(size int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("?,", size), ",") + "]"
}

func stringArgs(values []string) []interface{} {
	args := make([]interface{}, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func (s *AssetRiskScorer) queryAlertRiskBatch(
	ctx context.Context,
	tenantID string,
	ips []string,
) (map[string]alertRiskAggregate, error) {
	query := fmt.Sprintf(`
		WITH %s AS candidate_ips
		SELECT
			ip,
			countIf(status NOT IN ('ALERT_STATUS_CLOSED', 'ALERT_STATUS_RESOLVED', 'closed', 'resolved')) AS active,
			count() AS total_7d,
			countIf(severity IN ('critical', 'CRITICAL', 'SEVERITY_CRITICAL')) AS critical
		FROM (
			SELECT
				arrayJoin(arrayDistinct([src_ip, dst_ip])) AS ip,
				status,
				severity
			FROM traffic.alerts_latest FINAL
			WHERE tenant_id = ?
			  AND last_seen >= toUnixTimestamp64Milli(now64(3) - INTERVAL 7 DAY)
			  AND (has(candidate_ips, src_ip) OR has(candidate_ips, dst_ip))
		)
		WHERE has(candidate_ips, ip)
		GROUP BY ip
	`, clickHouseArrayPlaceholders(len(ips)))
	args := append(stringArgs(ips), tenantID)
	rows, err := s.chDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aggregates := make(map[string]alertRiskAggregate, len(ips))
	for rows.Next() {
		var ip string
		var aggregate alertRiskAggregate
		if err := rows.Scan(&ip, &aggregate.active, &aggregate.total, &aggregate.critical); err != nil {
			return nil, err
		}
		aggregates[ip] = aggregate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aggregates, nil
}

func (s *AssetRiskScorer) queryExposureRiskBatch(
	ctx context.Context,
	tenantID string,
	ips []string,
) (map[string]exposureRiskAggregate, error) {
	query := fmt.Sprintf(`
		WITH %s AS candidate_ips
		SELECT
			dst_ip,
			uniqExact(dst_port) AS observed_destination_ports,
			maxIf(1, dst_port IN (22, 3389, 23, 21)) AS has_risky_port,
			maxIf(1, protocol = 6 AND flags_syn > flags_ack * 0.8) AS is_server
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND has(candidate_ips, dst_ip)
		  AND ts_start >= toUnixTimestamp64Milli(now64(3) - INTERVAL 24 HOUR)
		GROUP BY dst_ip
	`, clickHouseArrayPlaceholders(len(ips)))
	args := append(stringArgs(ips), tenantID)
	rows, err := s.chDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aggregates := make(map[string]exposureRiskAggregate, len(ips))
	for rows.Next() {
		var ip string
		var aggregate exposureRiskAggregate
		if err := rows.Scan(
			&ip,
			&aggregate.observedDestinationPorts,
			&aggregate.hasRiskyPort,
			&aggregate.isServer,
		); err != nil {
			return nil, err
		}
		aggregates[ip] = aggregate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aggregates, nil
}

func alertScore(aggregate alertRiskAggregate) float64 {
	if aggregate.total == 0 {
		return 5
	}
	return math.Max(5, math.Min(100, float64(aggregate.active)*15+float64(aggregate.critical)*25))
}

func exposureScore(aggregate exposureRiskAggregate) float64 {
	score := 10.0
	if aggregate.isServer == 1 {
		score += 20
	}
	if aggregate.hasRiskyPort == 1 {
		score += 30
	}
	score += math.Min(40, float64(aggregate.observedDestinationPorts)*2)
	return math.Min(100, score)
}

func (s *AssetRiskScorer) scoreIPBatch(
	ctx context.Context,
	tenantID string,
	ips []string,
) ([]*AssetRiskScore, []string, []string, error) {
	if s.chDB == nil {
		return nil, nil, nil, fmt.Errorf("ClickHouse connection not available for asset risk scoring")
	}
	if len(ips) == 0 {
		return []*AssetRiskScore{}, []string{}, []string{"vulnerability", "behavior"}, nil
	}

	var alertAggregates map[string]alertRiskAggregate
	var exposureAggregates map[string]exposureRiskAggregate
	var alertErr, exposureErr error
	var queries sync.WaitGroup
	queries.Add(2)
	go func() {
		defer queries.Done()
		alertAggregates, alertErr = s.queryAlertRiskBatch(ctx, tenantID, ips)
	}()
	go func() {
		defer queries.Done()
		exposureAggregates, exposureErr = s.queryExposureRiskBatch(ctx, tenantID, ips)
	}()
	queries.Wait()

	available := make([]string, 0, 2)
	missing := []string{"vulnerability", "behavior"}
	if alertErr == nil {
		available = append(available, "alert")
	} else {
		missing = append(missing, "alert")
		s.logger.Warn("Asset risk alert dimension unavailable", zap.Error(alertErr))
	}
	if exposureErr == nil {
		available = append(available, "exposure")
	} else {
		missing = append(missing, "exposure")
		s.logger.Warn("Asset risk exposure dimension unavailable", zap.Error(exposureErr))
	}
	if len(available) == 0 {
		return nil, nil, missing, fmt.Errorf(
			"all asset risk dimensions unavailable: alert=%v; exposure=%v",
			alertErr,
			exposureErr,
		)
	}

	now := time.Now()
	scores := make([]*AssetRiskScore, 0, len(ips))
	for _, ip := range ips {
		score := &AssetRiskScore{
			AssetID:             ip,
			IPAddress:           ip,
			TenantID:            tenantID,
			AvailableDimensions: append([]string(nil), available...),
			MissingDimensions:   append([]string(nil), missing...),
			Partial:             len(missing) > 0,
			UpdatedAt:           now,
		}
		weightedScore := 0.0
		availableWeight := 0.0
		if alertErr == nil {
			aggregate := alertAggregates[ip]
			score.ActiveAlerts = aggregate.active
			score.TotalAlerts7d = aggregate.total
			score.CriticalAlerts = aggregate.critical
			score.AlertScore = alertScore(aggregate)
			weightedScore += score.AlertScore * alertDimensionWeight
			availableWeight += alertDimensionWeight
		}
		if exposureErr == nil {
			aggregate := exposureAggregates[ip]
			score.HasOpenPorts = aggregate.observedDestinationPorts
			score.ExposureScore = exposureScore(aggregate)
			weightedScore += score.ExposureScore * exposureDimensionWeight
			availableWeight += exposureDimensionWeight
		}
		score.TotalScore = weightedScore / availableWeight
		score.RiskLevel = s.levelFromScore(score.TotalScore)
		scores = append(scores, score)
	}
	return scores, available, missing, nil
}

// =============================================================================
// 批量评分
// =============================================================================

// ScoreAllAssets 对租户下所有资产进行风险评分
func (s *AssetRiskScorer) ScoreAllAssets(ctx context.Context, tenantID string) ([]*AssetRiskScore, error) {
	scores, _, _, _, _, err := s.scoreCandidateAssets(ctx, tenantID, maxRiskSummaryCandidates)
	return scores, err
}

func (s *AssetRiskScorer) scoreCandidateAssets(
	ctx context.Context,
	tenantID string,
	limit int,
) ([]*AssetRiskScore, int, int, []string, []string, error) {
	if s.pgDB == nil {
		return nil, 0, 0, nil, nil, fmt.Errorf("PostgreSQL connection not available for asset query")
	}
	if s.chDB == nil {
		return nil, 0, 0, nil, nil, fmt.Errorf("ClickHouse connection not available for asset risk scoring")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if limit < 1 || limit > maxRiskSummaryCandidates {
		limit = maxRiskSummaryCandidates
	}
	query := `
		WITH candidates AS (
			SELECT ip_address, max(last_seen) AS last_seen
			FROM assets
			WHERE tenant_id = $1 AND ip_address IS NOT NULL AND ip_address <> ''
			GROUP BY ip_address
		)
		SELECT ip_address, count(*) OVER() AS total_assets
		FROM candidates
		ORDER BY last_seen DESC, ip_address
		LIMIT $2
	`
	rows, err := s.pgDB.QueryContext(ctx, query, tenantID, limit)
	if err != nil {
		return nil, 0, 0, nil, nil, fmt.Errorf("query risk candidate assets: %w", err)
	}
	defer rows.Close()

	ips := make([]string, 0, limit)
	totalAssets := 0
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip, &totalAssets); err != nil {
			return nil, 0, 0, nil, nil, fmt.Errorf("scan risk candidate asset: %w", err)
		}
		ips = append(ips, ip)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, nil, nil, fmt.Errorf("iterate risk candidate assets: %w", err)
	}
	if len(ips) == 0 {
		return []*AssetRiskScore{}, totalAssets, 0, []string{}, []string{"vulnerability", "behavior"}, nil
	}

	scores, available, missing, err := s.scoreIPBatch(ctx, tenantID, ips)
	if err != nil {
		return nil, totalAssets, len(ips), available, missing, err
	}
	return scores, totalAssets, 0, available, missing, nil
}

// =============================================================================
// 运维接口: 风险 Top-N
// =============================================================================

type RiskSummary struct {
	TotalAssets         int               `json:"total_assets"`
	EvaluatedAssets     int               `json:"evaluated_assets"`
	FailedAssets        int               `json:"failed_assets"`
	Partial             bool              `json:"partial"`
	CandidateLimit      int               `json:"candidate_limit"`
	AvailableDimensions []string          `json:"available_dimensions"`
	MissingDimensions   []string          `json:"missing_dimensions,omitempty"`
	RiskDistribution    map[string]int    `json:"risk_distribution"`
	TopRiskyAssets      []*AssetRiskScore `json:"top_risky_assets"`
}

func (s *AssetRiskScorer) GetRiskSummary(ctx context.Context, tenantID string, topN int) (*RiskSummary, error) {
	scores, totalAssets, failedAssets, available, missing, err := s.scoreCandidateAssets(
		ctx,
		tenantID,
		maxRiskSummaryCandidates,
	)
	if err != nil {
		return nil, err
	}
	if totalAssets > 0 && len(scores) == 0 {
		return nil, fmt.Errorf("asset risk summary has no successful scores for %d assets", totalAssets)
	}

	summary := &RiskSummary{
		TotalAssets:         totalAssets,
		EvaluatedAssets:     len(scores),
		FailedAssets:        failedAssets,
		Partial:             totalAssets > len(scores)+failedAssets || failedAssets > 0 || len(missing) > 0,
		CandidateLimit:      maxRiskSummaryCandidates,
		AvailableDimensions: available,
		MissingDimensions:   missing,
		RiskDistribution:    map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0},
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

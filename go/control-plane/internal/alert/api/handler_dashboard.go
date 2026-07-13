package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// DashboardHandler Dashboard API — 实时统计指标
type DashboardHandler struct {
	chClient *storage.ClickHouseClient
	logger   *zap.Logger
}

func NewDashboardHandler(chClient *storage.ClickHouseClient, logger *zap.Logger) *DashboardHandler {
	return &DashboardHandler{chClient: chClient, logger: logger}
}

// GetStats 获取 Dashboard 总览统计
func (h *DashboardHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	stats := map[string]interface{}{
		"alerts":        h.queryAlertStats(ctx, tenantID),
		"sessions":      h.querySessionStats(ctx, tenantID),
		"traffic":       h.queryTrafficStats(ctx, tenantID),
		"probes":        h.queryProbeStats(ctx, tenantID),
		"attack_chains": h.queryAttackChainStats(ctx, tenantID),
		"fusion": map[string]interface{}{
			"total_events":     int64(0),
			"entities_aligned": int64(0),
			"alignment_rate":   float64(0),
			"completeness":     float64(0),
		},
		"compliance": map[string]interface{}{
			"pass_rate":             float64(0),
			"sla_violations":        int64(0),
			"avg_response_time_min": float64(0),
		},
		"baseline": map[string]interface{}{
			"total":           int64(0),
			"alerted_metrics": int64(0),
			"learning_count":  int64(0),
		},
		"performance": h.queryPerformanceStats(ctx, tenantID),
	}
	httpx.JSONSuccess(w, ctx, stats)
}

// GetAlertTrend 告警趋势 (最近 24h 按小时)
func (h *DashboardHandler) GetAlertTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	start, end, err := dashboardRange(r, 24*time.Hour)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}
	bucket := dashboardBucketExpr("last_seen", r.URL.Query().Get("granularity"))

	sql := `SELECT ` + bucket + ` as hour, severity, count() as cnt
		FROM traffic.alerts WHERE tenant_id=? AND last_seen >= ? AND last_seen <= ?
		GROUP BY hour, severity ORDER BY hour`
	rows, err := h.chClient.Query(ctx, sql, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type trendPoint struct {
		Hour     string `json:"hour"`
		Severity string `json:"severity"`
		Count    int64  `json:"count"`
	}
	trend := make([]trendPoint, 0)
	for rows.Next() {
		var tp trendPoint
		var hour time.Time
		var count uint64
		if err := rows.Scan(&hour, &tp.Severity, &count); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		tp.Count = int64(count)
		tp.Hour = hour.Format(time.RFC3339)
		trend = append(trend, tp)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"trend": trend})
}

// GetAttackPhases 攻击阶段分布 (基于 Campaign 数据)
func (h *DashboardHandler) GetAttackPhases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	sql := `SELECT campaign_type, count() as cnt, avg(score) as avg_score
		FROM traffic.campaigns WHERE tenant_id=? AND ts_start >= ?
		GROUP BY campaign_type ORDER BY cnt DESC LIMIT 10`
	rows, err := h.chClient.Query(ctx, sql, tenantID, time.Now().Add(-7*24*time.Hour).UnixMilli())
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type phaseStat struct {
		Phase    string  `json:"phase"`
		Count    int64   `json:"count"`
		AvgScore float64 `json:"avg_score"`
	}
	phases := make([]phaseStat, 0)
	for rows.Next() {
		var p phaseStat
		var count uint64
		if err := rows.Scan(&p.Phase, &count, &p.AvgScore); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		p.Count = int64(count)
		phases = append(phases, p)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"phases": phases})
}

// GetTopIPs 获取 Top-N 活跃 IP
func (h *DashboardHandler) GetTopIPs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	start, end, err := dashboardRange(r, time.Hour)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}
	limit := parseDashboardLimit(r.URL.Query().Get("limit"), 10, 100)
	ipColumn := "src_ip"
	switch mux.Vars(r)["type"] {
	case "dst":
		ipColumn = "dst_ip"
	case "", "src":
		ipColumn = "src_ip"
	default:
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "type must be src or dst")
		return
	}

	sql := `SELECT ` + ipColumn + `, count() as cnt, sum(bytes_fwd+bytes_bwd) as total_bytes
		FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= ? AND ts_start <= ?
		GROUP BY ` + ipColumn + ` ORDER BY total_bytes DESC LIMIT ?`
	rows, err := h.chClient.Query(ctx, sql, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type topIP struct {
		IP         string `json:"ip"`
		Flows      int64  `json:"flows"`
		Count      int64  `json:"count"`
		TotalBytes uint64 `json:"total_bytes"`
	}
	ips := make([]topIP, 0)
	for rows.Next() {
		var ip topIP
		var flows uint64
		if err := rows.Scan(&ip.IP, &flows, &ip.TotalBytes); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		ip.Flows = int64(flows)
		ip.Count = ip.Flows
		ips = append(ips, ip)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"ips": ips})
}

// GetEncryptedTrend 加密流量趋势
func (h *DashboardHandler) GetEncryptedTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}

	start, end, err := dashboardRange(r, 24*time.Hour)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}
	bucket := dashboardBucketExpr("ts_start", r.URL.Query().Get("granularity"))

	bucketSeconds := dashboardBucketSeconds(r.URL.Query().Get("granularity"))
	sql := `SELECT ` + bucket + ` as hour,
		countIf(dst_port=443 OR dst_port=8443) as encrypted,
		count() as total,
		sumIf(bytes_fwd+bytes_bwd, dst_port=443 OR dst_port=8443) as encrypted_bytes,
		sum(bytes_fwd+bytes_bwd) as total_bytes
		FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= ? AND ts_start <= ?
		GROUP BY hour ORDER BY hour`
	rows, err := h.chClient.Query(ctx, sql, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type encPoint struct {
		Hour             string  `json:"hour"`
		Timestamp        int64   `json:"timestamp"`
		Encrypted        int64   `json:"encrypted"`
		Total            int64   `json:"total"`
		EncryptedBytes   uint64  `json:"encrypted_bytes"`
		TotalBytes       uint64  `json:"total_bytes"`
		Ratio            float64 `json:"ratio"`
		EncryptedRatio   float64 `json:"encrypted_ratio"`
		EncryptedGbps    float64 `json:"encrypted_gbps"`
		NonEncryptedGbps float64 `json:"non_encrypted_gbps"`
	}
	trend := make([]encPoint, 0)
	for rows.Next() {
		var ep encPoint
		var hour time.Time
		var encrypted, total uint64
		if err := rows.Scan(&hour, &encrypted, &total, &ep.EncryptedBytes, &ep.TotalBytes); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		ep.Encrypted = int64(encrypted)
		ep.Total = int64(total)
		ep.Hour = hour.Format(time.RFC3339)
		ep.Timestamp = hour.UnixMilli()
		if ep.Total > 0 {
			ep.Ratio = float64(ep.Encrypted) / float64(ep.Total)
		}
		ep.EncryptedRatio = ep.Ratio
		ep.EncryptedGbps = float64(ep.EncryptedBytes) * 8 / bucketSeconds / 1e9
		if ep.TotalBytes > ep.EncryptedBytes {
			ep.NonEncryptedGbps = float64(ep.TotalBytes-ep.EncryptedBytes) * 8 / bucketSeconds / 1e9
		}
		trend = append(trend, ep)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"trend": trend})
}

// ---- 内部查询方法 ----

func (h *DashboardHandler) queryAlertStats(ctx context.Context, tenantID string) map[string]interface{} {
	result := map[string]interface{}{
		"total": int64(0), "new": int64(0), "critical": int64(0),
		"high": int64(0), "medium": int64(0), "low": int64(0),
	}

	sql := `SELECT severity, status, count() FROM traffic.alerts WHERE tenant_id=? AND last_seen >= ? GROUP BY severity, status`
	rows, err := h.chClient.Query(ctx, sql, tenantID, time.Now().Add(-24*time.Hour).UnixMilli())
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var severity, status string
		var cnt uint64
		if err := rows.Scan(&severity, &status, &cnt); err != nil {
			return result
		}
		count := int64(cnt)
		result["total"] = result["total"].(int64) + count
		if status == "new" || status == "ALERT_STATUS_NEW" {
			result["new"] = result["new"].(int64) + count
		}
		switch severity {
		case "critical", "SEVERITY_CRITICAL":
			result["critical"] = result["critical"].(int64) + count
		case "high", "SEVERITY_HIGH":
			result["high"] = result["high"].(int64) + count
		case "medium", "SEVERITY_MEDIUM":
			result["medium"] = result["medium"].(int64) + count
		case "low", "SEVERITY_LOW":
			result["low"] = result["low"].(int64) + count
		}
	}
	return result
}

func (h *DashboardHandler) querySessionStats(ctx context.Context, tenantID string) map[string]interface{} {
	now := time.Now()
	sql := `SELECT count(), countIf(ts_end >= ?) FROM traffic.sessions WHERE tenant_id=? AND ts_start >= ?`
	var total, active uint64
	row, err := h.chClient.QueryRow(ctx, sql, now.Add(-5*time.Minute).UnixMilli(), tenantID, now.Add(-24*time.Hour).UnixMilli())
	if err != nil {
		return map[string]interface{}{"total": int64(0), "active": int64(0)}
	}
	if err := row.Scan(&total, &active); err != nil {
		return map[string]interface{}{"total": int64(0), "active": int64(0)}
	}
	return map[string]interface{}{"total": int64(total), "active": int64(active)}
}

func (h *DashboardHandler) queryTrafficStats(ctx context.Context, tenantID string) map[string]interface{} {
	sql := `SELECT
		toFloat64(sum(packets_fwd+packets_bwd))/60.0,
		toFloat64(sum(bytes_fwd+bytes_bwd))/60.0,
		toFloat64(countIf(dst_port=443 OR dst_port=8443))/toFloat64(greatest(count(), 1)),
		toFloat64(sumIf(packets_fwd+packets_bwd, dst_port NOT IN (443,8443)))/60.0
		FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= ?`
	var pps, bps, encRatio, nonEncPPS float64
	row, err := h.chClient.QueryRow(ctx, sql, tenantID, time.Now().Add(-time.Minute).UnixMilli())
	if err != nil {
		return map[string]interface{}{
			"pps": float64(0), "bps": float64(0), "encrypted_ratio": float64(0), "non_encrypted_pps": float64(0),
		}
	}
	if err := row.Scan(&pps, &bps, &encRatio, &nonEncPPS); err != nil {
		return map[string]interface{}{
			"pps": float64(0), "bps": float64(0), "encrypted_ratio": float64(0), "non_encrypted_pps": float64(0),
		}
	}
	return map[string]interface{}{
		"pps":               dashboardFinite(pps),
		"bps":               dashboardFinite(bps),
		"encrypted_ratio":   dashboardFinite(encRatio),
		"non_encrypted_pps": dashboardFinite(nonEncPPS),
	}
}

func (h *DashboardHandler) queryProbeStats(ctx context.Context, tenantID string) map[string]interface{} {
	sql := `SELECT count(DISTINCT probe_id) FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= ?`
	var active uint64
	row, err := h.chClient.QueryRow(ctx, sql, tenantID, time.Now().Add(-5*time.Minute).UnixMilli())
	if err != nil {
		return map[string]interface{}{"total": int64(0), "online": int64(0), "degraded": int64(0)}
	}
	if err := row.Scan(&active); err != nil {
		return map[string]interface{}{"total": int64(0), "online": int64(0), "degraded": int64(0)}
	}
	return map[string]interface{}{"total": int64(active), "online": int64(active), "degraded": int64(0)}
}

func (h *DashboardHandler) queryAttackChainStats(ctx context.Context, tenantID string) map[string]interface{} {
	result := map[string]interface{}{"active": int64(0), "total": int64(0), "high_risk": int64(0)}
	sql := `SELECT count(), countIf(ts_end >= ?), countIf(score >= 0.8)
		FROM traffic.campaigns WHERE tenant_id=? AND ts_start >= ?`
	var total, active, highRisk uint64
	row, err := h.chClient.QueryRow(ctx, sql, time.Now().Add(-24*time.Hour).UnixMilli(), tenantID, time.Now().Add(-7*24*time.Hour).UnixMilli())
	if err != nil {
		return result
	}
	if err := row.Scan(&total, &active, &highRisk); err != nil {
		return result
	}
	result["active"] = int64(active)
	result["total"] = int64(total)
	result["high_risk"] = int64(highRisk)
	return result
}

func (h *DashboardHandler) queryPerformanceStats(ctx context.Context, tenantID string) map[string]interface{} {
	result := map[string]interface{}{
		"end_to_end_p95_ms":      float64(0),
		"kafka_lag":              int64(0),
		"flink_backpressure_pct": float64(0),
	}
	sql := `SELECT quantile(0.95)(toFloat64(greatest(created_at - first_seen, 0)))
		FROM traffic.alerts WHERE tenant_id=? AND last_seen >= ?`
	var p95 float64
	row, err := h.chClient.QueryRow(ctx, sql, tenantID, time.Now().Add(-24*time.Hour).UnixMilli())
	if err != nil {
		return result
	}
	if err := row.Scan(&p95); err != nil {
		return result
	}
	result["end_to_end_p95_ms"] = dashboardFinite(p95)
	return result
}

func dashboardFinite(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func dashboardRange(r *http.Request, defaultLookback time.Duration) (int64, int64, error) {
	now := time.Now()
	start := now.Add(-defaultLookback).UnixMilli()
	end := now.UnixMilli()

	if value := firstQueryValue(r, "start_time", "start"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start_time: %s", value)
		}
		start = parsed
	}
	if value := firstQueryValue(r, "end_time", "end"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end_time: %s", value)
		}
		end = parsed
	}
	if start > end {
		return 0, 0, fmt.Errorf("start_time must be less than or equal to end_time")
	}
	return start, end, nil
}

func firstQueryValue(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := r.URL.Query().Get(name); value != "" {
			return value
		}
	}
	return ""
}

func dashboardBucketExpr(column, granularity string) string {
	switch granularity {
	case "minute":
		return "toStartOfMinute(fromUnixTimestamp64Milli(" + column + "))"
	case "day":
		return "toStartOfDay(fromUnixTimestamp64Milli(" + column + "))"
	default:
		return "toStartOfHour(fromUnixTimestamp64Milli(" + column + "))"
	}
}

func dashboardBucketSeconds(granularity string) float64 {
	switch granularity {
	case "minute":
		return 60
	case "day":
		return 86400
	default:
		return 3600
	}
}

func parseDashboardLimit(value string, fallback, max int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > max {
		return max
	}
	return parsed
}

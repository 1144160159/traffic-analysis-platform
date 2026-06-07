package api

import (
	"context"
	"net/http"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
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
	if tenantID == "" { tenantID = "default" }

	stats := map[string]interface{}{
		"alerts":   h.queryAlertStats(ctx, tenantID),
		"sessions": h.querySessionStats(ctx, tenantID),
		"traffic":  h.queryTrafficStats(ctx, tenantID),
		"probes":   h.queryProbeStats(ctx, tenantID),
	}
	httpx.JSONSuccess(w, ctx, stats)
}

// GetAlertTrend 告警趋势 (最近 24h 按小时)
func (h *DashboardHandler) GetAlertTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" { tenantID = "default" }

	sql := `SELECT toStartOfHour(last_seen) as hour, severity, count() as cnt
		FROM traffic.alerts WHERE tenant_id=? AND last_seen >= now()-INTERVAL 24 HOUR
		GROUP BY hour, severity ORDER BY hour`
	rows, err := h.chClient.Query(ctx, sql, tenantID)
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
	var trend []trendPoint
	for rows.Next() {
		var tp trendPoint
		var hour time.Time
		rows.Scan(&hour, &tp.Severity, &tp.Count)
		tp.Hour = hour.Format(time.RFC3339)
		trend = append(trend, tp)
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"trend": trend})
}

// GetAttackPhases 攻击阶段分布 (基于 Campaign 数据)
func (h *DashboardHandler) GetAttackPhases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" { tenantID = "default" }

	sql := `SELECT campaign_type, count() as cnt, avg(score) as avg_score
		FROM traffic.campaigns WHERE tenant_id=? AND ts_start >= now()-INTERVAL 7 DAY
		GROUP BY campaign_type ORDER BY cnt DESC LIMIT 10`
	rows, err := h.chClient.Query(ctx, sql, tenantID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type phaseStat struct {
		Phase     string  `json:"phase"`
		Count     int64   `json:"count"`
		AvgScore  float64 `json:"avg_score"`
	}
	var phases []phaseStat
	for rows.Next() {
		var p phaseStat
		rows.Scan(&p.Phase, &p.Count, &p.AvgScore)
		phases = append(phases, p)
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"phases": phases})
}

// GetTopIPs 获取 Top-N 活跃 IP
func (h *DashboardHandler) GetTopIPs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" { tenantID = "default" }

	sql := `SELECT src_ip, count() as cnt, sum(bytes_fwd+bytes_bwd) as total_bytes
		FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= now()-INTERVAL 1 HOUR
		GROUP BY src_ip ORDER BY total_bytes DESC LIMIT 10`
	rows, err := h.chClient.Query(ctx, sql, tenantID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type topIP struct {
		IP         string `json:"ip"`
		Flows      int64  `json:"flows"`
		TotalBytes uint64 `json:"total_bytes"`
	}
	var ips []topIP
	for rows.Next() {
		var ip topIP
		rows.Scan(&ip.IP, &ip.Flows, &ip.TotalBytes)
		ips = append(ips, ip)
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"ips": ips})
}

// GetEncryptedTrend 加密流量趋势
func (h *DashboardHandler) GetEncryptedTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" { tenantID = "default" }

	sql := `SELECT toStartOfHour(ts_start) as hour,
		countIf(dst_port=443 OR dst_port=8443) as encrypted,
		count() as total
		FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= now()-INTERVAL 24 HOUR
		GROUP BY hour ORDER BY hour`
	rows, err := h.chClient.Query(ctx, sql, tenantID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	type encPoint struct {
		Hour      string  `json:"hour"`
		Encrypted int64   `json:"encrypted"`
		Total     int64   `json:"total"`
		Ratio     float64 `json:"ratio"`
	}
	var trend []encPoint
	for rows.Next() {
		var ep encPoint
		var hour time.Time
		rows.Scan(&hour, &ep.Encrypted, &ep.Total)
		ep.Hour = hour.Format(time.RFC3339)
		if ep.Total > 0 { ep.Ratio = float64(ep.Encrypted) / float64(ep.Total) }
		trend = append(trend, ep)
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"trend": trend})
}

// ---- 内部查询方法 ----

func (h *DashboardHandler) queryAlertStats(ctx context.Context, tenantID string) map[string]interface{} {
	sql := `SELECT severity, status, count() FROM traffic.alerts WHERE tenant_id=? AND last_seen >= now()-INTERVAL 24 HOUR GROUP BY severity, status`
	rows, err := h.chClient.Query(ctx, sql, tenantID)
	if err != nil { return nil }
	defer rows.Close()

	result := map[string]interface{}{
		"total": int64(0), "new": int64(0), "critical": int64(0),
		"high": int64(0), "medium": int64(0), "low": int64(0),
	}
	for rows.Next() {
		var severity, status string
		var cnt int64
		rows.Scan(&severity, &status, &cnt)
		result["total"] = result["total"].(int64) + cnt
		if status == "new" || status == "ALERT_STATUS_NEW" { result["new"] = result["new"].(int64) + cnt }
		switch severity {
		case "critical", "SEVERITY_CRITICAL": result["critical"] = result["critical"].(int64) + cnt
		case "high", "SEVERITY_HIGH": result["high"] = result["high"].(int64) + cnt
		case "medium", "SEVERITY_MEDIUM": result["medium"] = result["medium"].(int64) + cnt
		case "low", "SEVERITY_LOW": result["low"] = result["low"].(int64) + cnt
		}
	}
	return result
}

func (h *DashboardHandler) querySessionStats(ctx context.Context, tenantID string) map[string]interface{} {
	sql := `SELECT count(), countIf(ts_end >= now()-INTERVAL 5 MINUTE) FROM traffic.sessions WHERE tenant_id=? AND ts_start >= now()-INTERVAL 24 HOUR`
	var total, active int64
	row, _ := h.chClient.QueryRow(ctx, sql, tenantID)
	row.Scan(&total, &active)
	return map[string]interface{}{"total": total, "active": active}
}

func (h *DashboardHandler) queryTrafficStats(ctx context.Context, tenantID string) map[string]interface{} {
	sql := `SELECT sum(packets_fwd+packets_bwd)/60.0, sum(bytes_fwd+bytes_bwd)/60.0,
		countIf(dst_port=443 OR dst_port=8443)*100.0/greatest(count(),1),
		sumIf(packets_fwd+packets_bwd, dst_port NOT IN (443,8443))/60.0
		FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= now()-INTERVAL 1 MINUTE`
	var pps, bps, encRatio, nonEncPPS float64
	row, _ := h.chClient.QueryRow(ctx, sql, tenantID)
	row.Scan(&pps, &bps, &encRatio, &nonEncPPS)
	return map[string]interface{}{
		"pps": pps, "bps": bps, "encrypted_ratio": encRatio, "non_encrypted_pps": nonEncPPS,
	}
}

func (h *DashboardHandler) queryProbeStats(ctx context.Context, tenantID string) map[string]interface{} {
	sql := `SELECT count(DISTINCT probe_id) FROM traffic.flows_raw WHERE tenant_id=? AND ts_start >= now()-INTERVAL 5 MINUTE`
	var active int64
	row, _ := h.chClient.QueryRow(ctx, sql, tenantID)
	row.Scan(&active)
	return map[string]interface{}{"total": active, "active": active}
}

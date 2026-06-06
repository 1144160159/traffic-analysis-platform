package evidence

import (
	"context"
	"crypto/md5"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/arkime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// AutoEvidenceGenerator 自动证据生成器 — 告警到达时自动生成调查证据
// 实现 AlertConsumer.EvidenceGenerator 接口
type AutoEvidenceGenerator struct {
	chClient       *storage.ClickHouseClient
	arkimeLinkGen  *arkime.LinkGenerator
	visualBaseURL  string
	logger         *zap.Logger
}

// NewAutoEvidenceGenerator 创建自动证据生成器
func NewAutoEvidenceGenerator(
	chClient *storage.ClickHouseClient,
	arkimeLinkGen *arkime.LinkGenerator,
	visualBaseURL string,
	logger *zap.Logger,
) *AutoEvidenceGenerator {
	return &AutoEvidenceGenerator{
		chClient:      chClient,
		arkimeLinkGen: arkimeLinkGen,
		visualBaseURL: visualBaseURL,
		logger:        logger,
	}
}

// GenerateForAlert 为告警自动生成证据条目 (实现 EvidenceGenerator 接口)
// 返回证据 ID 列表，失败时返回 nil + error
func (g *AutoEvidenceGenerator) GenerateForAlert(ctx context.Context, alert *persistence.Alert) ([]string, error) {
	if alert == nil || g.chClient == nil {
		return nil, nil
	}

	evidenceIDs := make([]string, 0, 4)

	// 1. 统计指纹证据 (流量特征摘要)
	if id, err := g.generateStatFingerprint(ctx, alert); err == nil && id != "" {
		evidenceIDs = append(evidenceIDs, id)
	}

	// 2. 会话上下文证据 (关联的 session 信息)
	if id, err := g.generateSessionContext(ctx, alert); err == nil && id != "" {
		evidenceIDs = append(evidenceIDs, id)
	}

	// 3. Arkime 链接证据 (PCAP 可视化跳转)
	if id := g.generateArkimeLink(alert); id != "" {
		evidenceIDs = append(evidenceIDs, id)
	}

	return evidenceIDs, nil
}

// ---- 统计指纹 ----

func (g *AutoEvidenceGenerator) generateStatFingerprint(ctx context.Context, alert *persistence.Alert) (string, error) {
	sql := `
		SELECT count(), sum(bytes_total), avg(duration_ms), sum(packets_total)
		FROM traffic.sessions
		WHERE tenant_id = ? AND community_id = ?
		  AND ts_start >= ? AND ts_start <= ?
		LIMIT 1`
	start := alert.FirstSeen.Add(-5 * time.Minute)
	end := alert.LastSeen.Add(5 * time.Minute)

	rows, err := g.chClient.Query(ctx, sql, alert.TenantID, alert.CommunityID, start, end)
	if err != nil {
		return "", fmt.Errorf("stat fingerprint query: %w", err)
	}
	defer rows.Close()

	var sessionCount, totalBytes, totalPackets uint64
	var avgDuration float64
	if rows.Next() {
		rows.Scan(&sessionCount, &totalBytes, &avgDuration, &totalPackets)
	}

	evidenceID := uuid.New().String()
	summary := fmt.Sprintf("Traffic fingerprint: %d sessions, %d bytes, %.0fms avg duration",
		sessionCount, totalBytes, avgDuration)

	ev := &Evidence{
		EvidenceID: evidenceID, TenantID: alert.TenantID, AlertID: alert.AlertID,
		Timestamp: time.Now(), Type: EvidenceTypeStat, Summary: summary,
		Confidence: 0.8, EventID: alert.EventID,
		Metrics: map[string]interface{}{
			"session_count": sessionCount, "total_bytes": totalBytes,
			"total_packets": totalPackets, "avg_duration_ms": avgDuration,
		},
	}
	if err := g.insertEvidence(ctx, ev); err != nil {
		return "", err
	}
	return evidenceID, nil
}

// ---- 会话上下文 ----

func (g *AutoEvidenceGenerator) generateSessionContext(ctx context.Context, alert *persistence.Alert) (string, error) {
	if alert.SessionID == "" {
		return "", nil
	}

	sql := `SELECT ts_start, ts_end, client_ip, server_ip, client_port, server_port, protocol, packets_total, bytes_total
		FROM traffic.sessions WHERE tenant_id = ? AND session_id = ? LIMIT 1`
	rows, err := g.chClient.Query(ctx, sql, alert.TenantID, alert.SessionID)
	if err != nil {
		return "", fmt.Errorf("session context query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return "", nil
	}
	var tsStart, tsEnd time.Time
	var clientIP, serverIP string
	var clientPort, serverPort uint16
	var protocol uint8
	var packets, bytes uint64
	rows.Scan(&tsStart, &tsEnd, &clientIP, &serverIP, &clientPort, &serverPort, &protocol, &packets, &bytes)

	evidenceID := uuid.New().String()
	summary := fmt.Sprintf("Session context: %s:%d → %s:%d proto=%d, %d pkts/%d bytes",
		clientIP, clientPort, serverIP, serverPort, protocol, packets, bytes)

	ev := &Evidence{
		EvidenceID: evidenceID, TenantID: alert.TenantID, AlertID: alert.AlertID,
		Timestamp: time.Now(), Type: EvidenceTypeSequence, Summary: summary,
		Confidence: 0.9, EventID: alert.EventID,
		Metrics: map[string]interface{}{
			"ts_start": tsStart.Format(time.RFC3339), "ts_end": tsEnd.Format(time.RFC3339),
			"client_ip": clientIP, "server_ip": serverIP,
			"packets": packets, "bytes": bytes,
		},
	}
	if err := g.insertEvidence(ctx, ev); err != nil {
		return "", err
	}
	return evidenceID, nil
}

// ---- Arkime 链接 ----

func (g *AutoEvidenceGenerator) generateArkimeLink(alert *persistence.Alert) string {
	if g.arkimeLinkGen == nil {
		return ""
	}
	arkimeURL := g.arkimeLinkGen.GenerateTupleLink(alert.SrcIP, alert.DstIP, alert.SrcPort, alert.DstPort, alert.Protocol, alert.FirstSeen, alert.LastSeen)
	if arkimeURL == "" {
		return ""
	}

	evidenceID := uuid.New().String()
	summary := fmt.Sprintf("Arkime PCAP session: %s ↔ %s", alert.SrcIP, alert.DstIP)

	ev := &Evidence{
		EvidenceID: evidenceID, TenantID: alert.TenantID, AlertID: alert.AlertID,
		Timestamp: time.Now(), Type: EvidenceTypePcap, Summary: summary,
		Confidence: 1.0, EventID: alert.EventID,
		ArkimeLink: arkimeURL,
	}
	if err := g.insertEvidence(context.Background(), ev); err != nil {
		g.logger.Warn("Failed to insert Arkime evidence", zap.Error(err))
		return ""
	}
	return evidenceID
}

// ---- 持久化 ----

func (g *AutoEvidenceGenerator) insertEvidence(ctx context.Context, ev *Evidence) error {
	sql := `INSERT INTO traffic.evidence (tenant_id, evidence_id, alert_id, ts, type, summary, metrics_json, snippet_ref_json, arkime_link, confidence, event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	return g.chClient.Exec(ctx, sql,
		ev.TenantID, ev.EvidenceID, ev.AlertID, ev.Timestamp,
		string(ev.Type), ev.Summary,
		toJSON(ev.Metrics), toJSON(ev.SnippetRef),
		ev.ArkimeLink, ev.Confidence, ev.EventID,
	)
}

func toJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	// Use fmt.Sprintf for simple maps
	if m, ok := v.(map[string]interface{}); ok {
		parts := make([]string, 0, len(m))
		for k, val := range m {
			parts = append(parts, fmt.Sprintf("\"%s\":\"%v\"", k, val))
		}
		return "{" + fmt.Sprintf("%s", join(parts, ",")) + "}"
	}
	return "{}"
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// GenerateFingerprint 生成去重指纹 (供外部使用)
func GenerateFingerprint(tenantID, alertType, srcIP, dstIP string, dstPort uint32, severity string, ts int64) string {
	data := fmt.Sprintf("%s:%s:%s:%s:%d:%s:%d", tenantID, alertType, srcIP, dstIP, dstPort, severity, ts)
	return fmt.Sprintf("%x", md5.Sum([]byte(data)))
}

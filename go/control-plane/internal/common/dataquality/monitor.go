////////////////////////////////////////////////////////////////////////////////
// Data Quality Monitor — 数据质量监控
// 缺失业务逻辑 #4: 管道健康检查、数据缺失检测、Schema 漂移
////////////////////////////////////////////////////////////////////////////////

package dataquality

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// DataQualityReport
// =============================================================================

type DataQualityReport struct {
	Timestamp time.Time          `json:"timestamp"`
	TenantID  string             `json:"tenant_id"`
	Overall   string             `json:"overall"` // healthy | degraded | unhealthy
	Checks    []QualityCheck     `json:"checks"`
	Metrics   map[string]float64 `json:"metrics"`
}

type QualityCheck struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // pass | warn | fail
	Message   string  `json:"message"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
}

type LatencyChainReport struct {
	Timestamp       time.Time             `json:"timestamp"`
	TenantID        string                `json:"tenant_id"`
	LookbackMinutes int64                 `json:"lookback_minutes"`
	ThresholdMs     float64               `json:"threshold_ms"`
	FullChainClosed bool                  `json:"full_chain_closed"`
	Result          string                `json:"result"` // pass | fail | gap
	Stages          []LatencyChainStage   `json:"stages"`
	Segments        []LatencySegmentStats `json:"segments"`
	Gaps            []string              `json:"gaps"`
}

type LatencyChainStage struct {
	Name   string `json:"name"`
	Status string `json:"status"` // present | measured | missing
	Source string `json:"source"`
	Detail string `json:"detail,omitempty"`
}

type LatencySegmentStats struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	SampleCount uint64  `json:"sample_count"`
	P50Ms       float64 `json:"p50_ms"`
	P90Ms       float64 `json:"p90_ms"`
	P95Ms       float64 `json:"p95_ms"`
	P99Ms       float64 `json:"p99_ms"`
	Status      string  `json:"status"` // pass | fail | gap
	Detail      string  `json:"detail,omitempty"`
}

// =============================================================================
// Monitor
// =============================================================================

type Monitor struct {
	db     *sql.DB
	logger *zap.Logger
	config MonitorConfig

	baseline *Baseline
	mu       sync.RWMutex
}

type MonitorConfig struct {
	CheckInterval       time.Duration `env:"DQ_CHECK_INTERVAL" envDefault:"15m"`
	MinFlowRate         float64       `env:"DQ_MIN_FLOW_RATE" envDefault:"100"`     // 最低流速率 (flows/min)
	MaxMissingPercent   float64       `env:"DQ_MAX_MISSING" envDefault:"5.0"`       // 最大缺失率 %
	MaxLatencyP95       float64       `env:"DQ_MAX_LATENCY_P95" envDefault:"60000"` // 最大延迟 P95 ms
	MaxSchemaDriftCount int           `env:"DQ_MAX_SCHEMA_DRIFT" envDefault:"3"`
}

type Baseline struct {
	AvgFlowRate  float64   `json:"avg_flow_rate"`
	AvgPPS       float64   `json:"avg_pps"`
	AvgBPS       float64   `json:"avg_bps"`
	AvgPktLen    float64   `json:"avg_pktlen"`
	FeatureCount int       `json:"feature_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func NewMonitor(db *sql.DB, cfg MonitorConfig, logger *zap.Logger) *Monitor {
	return &Monitor{db: db, config: cfg, logger: logger}
}

func (m *Monitor) CheckLatencyChain(ctx context.Context, tenantID string, lookback time.Duration) (*LatencyChainReport, error) {
	if m.db == nil {
		return nil, fmt.Errorf("data quality monitor requires ClickHouse connection")
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}

	report := &LatencyChainReport{
		Timestamp:       time.Now(),
		TenantID:        tenantID,
		LookbackMinutes: int64(lookback / time.Minute),
		ThresholdMs:     m.config.MaxLatencyP95,
		Result:          "gap",
	}

	columns := m.latencyColumns(ctx)
	report.Stages = []LatencyChainStage{
		latencyStage("event_ts", "present", "traffic.flows_raw/traffic.sessions", "source event timestamp", hasColumn(columns, "flows_raw", "event_ts") || hasColumn(columns, "sessions", "event_ts")),
		latencyStage("ingest_ts", "present", "traffic.flows_raw/traffic.sessions", "ingest write timestamp", hasColumn(columns, "flows_raw", "ingest_ts") || hasColumn(columns, "sessions", "ingest_ts")),
		latencyStage("kafka_ts", "present", "ClickHouse/API latency chain schema", "Kafka append timestamp", hasAnyColumn(columns, "kafka_ts")),
		latencyStage("flink_out_ts", "present", "ClickHouse/API latency chain schema", "Flink output timestamp", hasAnyColumn(columns, "flink_out_ts")),
		{Name: "api_seen_ts", Status: "measured", Source: "/api/v1/data-quality/latency-chain", Detail: fmt.Sprintf("%d", report.Timestamp.UnixMilli())},
		{Name: "ui_seen_ts", Status: "missing", Source: "browser test", Detail: "browser-side script must attach ui_seen_ts evidence"},
	}

	windowStart := report.Timestamp.Add(-lookback).UnixMilli()
	report.Segments = append(report.Segments,
		m.latencySegment(ctx, "flow_event_to_ingest", "traffic.flows_raw.ingest_ts - event_ts", tenantID, "traffic.flows_raw", "ingest_ts", "event_ts", windowStart),
		m.latencySegment(ctx, "session_event_to_ingest", "traffic.sessions.ingest_ts - event_ts", tenantID, "traffic.sessions", "ingest_ts", "event_ts", windowStart),
		m.latencySegmentIfColumns(ctx, columns, "sessions", "session_ingest_to_kafka", "traffic.sessions.kafka_ts - ingest_ts", tenantID, "traffic.sessions", "kafka_ts", "ingest_ts", windowStart),
		m.latencySegmentIfColumns(ctx, columns, "sessions", "session_kafka_to_flink", "traffic.sessions.flink_out_ts - kafka_ts", tenantID, "traffic.sessions", "flink_out_ts", "kafka_ts", windowStart),
		m.latencySegmentIfColumns(ctx, columns, "sessions", "session_event_to_flink", "traffic.sessions.flink_out_ts - event_ts", tenantID, "traffic.sessions", "flink_out_ts", "event_ts", windowStart),
		m.latencySegment(ctx, "alert_last_seen_to_created", "traffic.alerts.created_at - last_seen", tenantID, "traffic.alerts", "created_at", "last_seen", windowStart),
	)

	for _, stage := range report.Stages {
		if stage.Status == "missing" {
			report.Gaps = append(report.Gaps, fmt.Sprintf("%s is missing (%s)", stage.Name, stage.Source))
		}
	}
	for _, segment := range report.Segments {
		if segment.Status == "gap" {
			report.Gaps = append(report.Gaps, fmt.Sprintf("%s has no samples in the selected lookback", segment.Name))
		}
	}

	if len(report.Gaps) == 0 {
		report.FullChainClosed = true
		report.Result = "pass"
		for _, segment := range report.Segments {
			if segment.Status == "fail" {
				report.Result = "fail"
				break
			}
		}
	}
	return report, nil
}

// CheckAll 执行全量数据质量检查
// 要求 ClickHouse 连接可用
func (m *Monitor) CheckAll(ctx context.Context, tenantID string) (*DataQualityReport, error) {
	if m.db == nil {
		return nil, fmt.Errorf("data quality monitor requires ClickHouse connection")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	report := &DataQualityReport{
		Timestamp: time.Now(),
		TenantID:  tenantID,
		Metrics:   make(map[string]float64),
	}

	// Check 1: 数据流入率 (flows_raw 最近 15 分钟)
	m.checkFlowRate(ctx, tenantID, report)

	// Check 2: 数据缺失 (feature_stat 与 sessions 对比)
	m.checkMissingData(ctx, tenantID, report)

	// Check 3: 端到端延迟 (ingest_ts → event_ts)
	m.checkEndToEndLatency(ctx, tenantID, report)

	// Check 4: Schema 漂移 (特征列数量)
	m.checkSchemaDrift(ctx, report)

	// Check 5: Kafka 积压
	m.checkKafkaLag(ctx, tenantID, report)

	// 评估总体状态
	report.Overall = m.evaluateOverall(report)
	return report, nil
}

// =============================================================================
// Check 1: 数据流入率
// =============================================================================

func (m *Monitor) checkFlowRate(ctx context.Context, tenantID string, report *DataQualityReport) {
	query := `
		SELECT count() / 15.0 AS flows_per_min
		FROM traffic.flows_raw
		WHERE tenant_id = ?
		  AND ingest_ts >= toUnixTimestamp64Milli(now64(3) - INTERVAL 15 MINUTE)
	`
	var flowRate float64
	err := m.db.QueryRowContext(ctx, query, tenantID).Scan(&flowRate)
	status := "pass"
	msg := fmt.Sprintf("Flow rate: %.1f flows/min", flowRate)
	if err != nil {
		status = "fail"
		msg = fmt.Sprintf("Cannot query flows_raw: %v", err)
	} else if flowRate < m.config.MinFlowRate {
		flowRate = finiteOrZero(flowRate)
		status = "fail"
		if flowRate == 0 {
			msg = fmt.Sprintf("No new flow traffic in the last 15 minutes; threshold is %.0f flows/min", m.config.MinFlowRate)
		} else {
			msg = fmt.Sprintf("Flow rate %.1f below threshold %.0f", flowRate, m.config.MinFlowRate)
		}
		report.Metrics["flow_rate"] = flowRate
	} else if flowRate < m.config.MinFlowRate*2 {
		status = "warn"
	}
	flowRate = finiteOrZero(flowRate)
	report.Metrics["flow_rate"] = flowRate
	report.Checks = append(report.Checks, QualityCheck{
		Name: "flow_rate", Status: status, Message: msg,
		Value: flowRate, Threshold: m.config.MinFlowRate,
	})
}

// =============================================================================
// Check 2: 数据缺失检测
// =============================================================================

func (m *Monitor) checkMissingData(ctx context.Context, tenantID string, report *DataQualityReport) {
	query := `
		SELECT
			(SELECT count() FROM traffic.sessions WHERE tenant_id = ? AND ts_start >= toUnixTimestamp64Milli(now64(3) - INTERVAL 1 HOUR)) AS sessions,
			(SELECT count() FROM traffic.feature_stat WHERE tenant_id = ? AND ts >= now() - INTERVAL 1 HOUR) AS features
	`
	var rawSessions, rawFeatures interface{}
	err := m.db.QueryRowContext(ctx, query, tenantID, tenantID).Scan(&rawSessions, &rawFeatures)
	sessions := finiteOrZero(dbNumeric(rawSessions))
	features := finiteOrZero(dbNumeric(rawFeatures))
	status := "pass"
	msg := fmt.Sprintf("Sessions: %.0f, Features: %.0f", sessions, features)

	if err != nil {
		status = "fail"
		msg = fmt.Sprintf("Data missing check failed: %v", err)
		sessions = 0
		features = 0
	} else if sessions == 0 && features == 0 {
		msg = "No sessions or feature rows in the last hour; completeness is not failing on an empty window"
	} else if sessions > 0 {
		ratio := features / sessions
		if ratio < 0.9 {
			status = "warn"
			msg = fmt.Sprintf("Feature/Session ratio %.2f < 0.9 (possible missing features)", ratio)
		}
	}
	report.Metrics["session_count_1h"] = sessions
	report.Metrics["feature_count_1h"] = features
	ratio := finiteOrZero(features / math.Max(sessions, 1))
	report.Checks = append(report.Checks, QualityCheck{
		Name: "data_completeness", Status: status, Message: msg,
		Value: ratio, Threshold: 0.9,
	})
}

// =============================================================================
// Check 3: 端到端延迟 P95
// =============================================================================

func (m *Monitor) checkEndToEndLatency(ctx context.Context, tenantID string, report *DataQualityReport) {
	query := `
		SELECT quantile(0.95)(ingest_ts - event_ts) / 1000 AS p95_latency_ms
		FROM traffic.flows_raw
		WHERE tenant_id = ?
		  AND ingest_ts >= toUnixTimestamp64Milli(now64(3) - INTERVAL 15 MINUTE)
	`
	var latencyMs float64
	err := m.db.QueryRowContext(ctx, query, tenantID).Scan(&latencyMs)
	status := "pass"
	latencyMs = finiteOrZero(latencyMs)
	msg := fmt.Sprintf("P95 latency: %.0f ms", latencyMs)

	if err != nil {
		status = "fail"
		msg = fmt.Sprintf("Latency check failed: %v", err)
	} else if latencyMs > m.config.MaxLatencyP95 {
		status = "fail"
		msg = fmt.Sprintf("P95 latency %.0f ms exceeds threshold %.0f ms", latencyMs, m.config.MaxLatencyP95)
	}
	report.Metrics["p95_latency_ms"] = latencyMs
	report.Checks = append(report.Checks, QualityCheck{
		Name: "end_to_end_latency", Status: status, Message: msg,
		Value: latencyMs, Threshold: m.config.MaxLatencyP95,
	})
}

func (m *Monitor) latencyColumns(ctx context.Context) map[string]map[string]bool {
	query := `
		SELECT table, name
		FROM system.columns
		WHERE database = 'traffic'
		  AND table IN ('flows_raw', 'sessions', 'alerts', 'evidence')
		  AND name IN ('event_ts', 'ingest_ts', 'kafka_ts', 'flink_out_ts', 'api_seen_ts', 'ui_seen_ts', 'first_seen', 'created_at', 'last_seen')
	`
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return map[string]map[string]bool{}
	}
	defer rows.Close()

	result := make(map[string]map[string]bool)
	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			continue
		}
		if result[tableName] == nil {
			result[tableName] = make(map[string]bool)
		}
		result[tableName][columnName] = true
	}
	return result
}

func (m *Monitor) latencySegment(ctx context.Context, name, source, tenantID, tableName, endColumn, startColumn string, windowStart int64) LatencySegmentStats {
	stats := LatencySegmentStats{Name: name, Source: source, Status: "gap"}
	query := fmt.Sprintf(`
		SELECT
			count() AS sample_count,
			quantile(0.50)(toFloat64(greatest(%s - %s, 0))) AS p50_ms,
			quantile(0.90)(toFloat64(greatest(%s - %s, 0))) AS p90_ms,
			quantile(0.95)(toFloat64(greatest(%s - %s, 0))) AS p95_ms,
			quantile(0.99)(toFloat64(greatest(%s - %s, 0))) AS p99_ms
		FROM %s
		WHERE tenant_id = ?
		  AND %s > 0
		  AND %s > 0
		  AND %s >= ?
	`, endColumn, startColumn, endColumn, startColumn, endColumn, startColumn, endColumn, startColumn, tableName, endColumn, startColumn, endColumn)

	if err := m.db.QueryRowContext(ctx, query, tenantID, windowStart).Scan(&stats.SampleCount, &stats.P50Ms, &stats.P90Ms, &stats.P95Ms, &stats.P99Ms); err != nil {
		stats.Detail = err.Error()
		return stats
	}
	stats.P50Ms = finiteOrZero(stats.P50Ms)
	stats.P90Ms = finiteOrZero(stats.P90Ms)
	stats.P95Ms = finiteOrZero(stats.P95Ms)
	stats.P99Ms = finiteOrZero(stats.P99Ms)
	if stats.SampleCount == 0 {
		stats.Detail = "no samples"
		return stats
	}
	stats.Status = "pass"
	if stats.P95Ms > m.config.MaxLatencyP95 {
		stats.Status = "fail"
		stats.Detail = fmt.Sprintf("p95 %.0f ms exceeds threshold %.0f ms", stats.P95Ms, m.config.MaxLatencyP95)
	}
	return stats
}

func (m *Monitor) latencySegmentIfColumns(ctx context.Context, columns map[string]map[string]bool, tableKey, name, source, tenantID, tableName, endColumn, startColumn string, windowStart int64) LatencySegmentStats {
	stats := LatencySegmentStats{Name: name, Source: source, Status: "gap"}
	if !hasColumn(columns, tableKey, endColumn) || !hasColumn(columns, tableKey, startColumn) {
		stats.Detail = fmt.Sprintf("missing required columns %s/%s", endColumn, startColumn)
		return stats
	}
	return m.latencySegment(ctx, name, source, tenantID, tableName, endColumn, startColumn, windowStart)
}

// =============================================================================
// Check 4: Schema 漂移
// =============================================================================

func (m *Monitor) checkSchemaDrift(ctx context.Context, report *DataQualityReport) {
	// 获取当前 flows_raw 列数
	query := `
		SELECT count() FROM system.columns
		WHERE database = 'traffic' AND table = 'flows_raw'
	`
	var rawColCount interface{}
	err := m.db.QueryRowContext(ctx, query).Scan(&rawColCount)
	colCount := finiteOrZero(dbNumeric(rawColCount))

	status := "pass"
	msg := fmt.Sprintf("flows_raw columns: %.0f", colCount)

	if err != nil {
		status = "warn"
		msg = fmt.Sprintf("Schema check unavailable: %v", err)
		colCount = 0
	} else {
		m.mu.RLock()
		if m.baseline != nil && math.Abs(colCount-float64(m.baseline.FeatureCount)) > float64(m.config.MaxSchemaDriftCount) {
			status = "fail"
			msg = fmt.Sprintf("Schema drift: %.0f columns (baseline: %d)", colCount, m.baseline.FeatureCount)
		}
		m.mu.RUnlock()
	}
	report.Metrics["flows_raw_columns"] = colCount
	report.Checks = append(report.Checks, QualityCheck{
		Name: "schema_drift", Status: status, Message: msg,
		Value: colCount, Threshold: float64(m.config.MaxSchemaDriftCount),
	})
}

// =============================================================================
// Check 5: Kafka 消费积压
// =============================================================================

func (m *Monitor) checkKafkaLag(ctx context.Context, tenantID string, report *DataQualityReport) {
	// ClickHouse 可近似监测: 最近 5 分钟写入率
	query := `
		SELECT count() / 5.0 AS inserts_per_min
		FROM traffic.flows_raw
		WHERE tenant_id = ?
		  AND ingest_ts >= toUnixTimestamp64Milli(now64(3) - INTERVAL 5 MINUTE)
	`
	var rate float64
	err := m.db.QueryRowContext(ctx, query, tenantID).Scan(&rate)
	status := "pass"
	msg := fmt.Sprintf("Insert rate: %.0f/min", rate)

	if err != nil {
		status = "warn"
		msg = fmt.Sprintf("Kafka lag check degraded: %v", err)
	} else if rate < m.config.MinFlowRate*0.5 {
		rate = finiteOrZero(rate)
		status = "warn"
		msg = fmt.Sprintf("Insert rate %.0f/min is low (possible Kafka lag)", rate)
	}
	rate = finiteOrZero(rate)
	report.Metrics["insert_rate_per_min"] = rate
	report.Checks = append(report.Checks, QualityCheck{
		Name: "kafka_lag_proxy", Status: status, Message: msg,
		Value: rate, Threshold: m.config.MinFlowRate * 0.5,
	})
}

func latencyStage(name, status, source, detail string, present bool) LatencyChainStage {
	if !present {
		status = "missing"
	}
	return LatencyChainStage{Name: name, Status: status, Source: source, Detail: detail}
}

func hasColumn(columns map[string]map[string]bool, tableName, columnName string) bool {
	return columns[tableName] != nil && columns[tableName][columnName]
}

func hasAnyColumn(columns map[string]map[string]bool, columnName string) bool {
	for _, tableColumns := range columns {
		if tableColumns[columnName] {
			return true
		}
	}
	return false
}

// =============================================================================
// Baseline Management
// =============================================================================

func (m *Monitor) UpdateBaseline(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("database connection not available")
	}
	query := `
		SELECT
			avg(pps) AS avg_pps,
			avg(bps) AS avg_bps,
			avg(pktlen_mean) AS avg_pktlen
		FROM traffic.feature_stat
		WHERE ts >= now() - INTERVAL 24 HOUR
	`
	baseline := &Baseline{UpdatedAt: time.Now()}
	var avgPPS, avgBPS, avgPktLen sql.NullFloat64
	err := m.db.QueryRowContext(ctx, query).Scan(&avgPPS, &avgBPS, &avgPktLen)
	if err != nil {
		return fmt.Errorf("update baseline: %w", err)
	}
	baseline.AvgPPS = finiteOrZero(nullFloat64(avgPPS))
	baseline.AvgBPS = finiteOrZero(nullFloat64(avgBPS))
	baseline.AvgPktLen = finiteOrZero(nullFloat64(avgPktLen))

	colQuery := `SELECT count() FROM system.columns WHERE database = 'traffic' AND table = 'flows_raw'`
	var featureCount interface{}
	if err := m.db.QueryRowContext(ctx, colQuery).Scan(&featureCount); err != nil {
		return fmt.Errorf("update baseline schema: %w", err)
	}
	baseline.FeatureCount = int(dbNumeric(featureCount))

	m.mu.Lock()
	m.baseline = baseline
	m.mu.Unlock()
	return nil
}

func (m *Monitor) GetBaseline() *Baseline {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baseline
}

// =============================================================================
// Helpers
// =============================================================================

func (m *Monitor) evaluateOverall(report *DataQualityReport) string {
	failCount, warnCount := 0, 0
	for _, c := range report.Checks {
		switch c.Status {
		case "fail":
			failCount++
		case "warn":
			warnCount++
		}
	}
	if failCount > 0 {
		return "unhealthy"
	}
	if warnCount > 1 {
		return "degraded"
	}
	return "healthy"
}

func finiteOrZero(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func nullFloat64(value sql.NullFloat64) float64 {
	if !value.Valid {
		return 0
	}
	return value.Float64
}

func dbNumeric(value interface{}) float64 {
	switch v := value.(type) {
	case nil:
		return 0
	case uint64:
		return float64(v)
	case *uint64:
		if v == nil {
			return 0
		}
		return float64(*v)
	case int64:
		return float64(v)
	case *int64:
		if v == nil {
			return 0
		}
		return float64(*v)
	case int:
		return float64(v)
	case *int:
		if v == nil {
			return 0
		}
		return float64(*v)
	case float64:
		return v
	case *float64:
		if v == nil {
			return 0
		}
		return *v
	case float32:
		return float64(v)
	case *float32:
		if v == nil {
			return 0
		}
		return float64(*v)
	case []byte:
		parsed, _ := strconv.ParseFloat(string(v), 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(v, 64)
		return parsed
	default:
		parsed, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		return parsed
	}
}

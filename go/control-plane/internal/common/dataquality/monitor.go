////////////////////////////////////////////////////////////////////////////////
// Data Quality Monitor — 数据质量监控
// 缺失业务逻辑 #4: 管道健康检查、数据缺失检测、Schema 漂移
////////////////////////////////////////////////////////////////////////////////

package dataquality

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// DataQualityReport
// =============================================================================

type DataQualityReport struct {
	Timestamp        time.Time              `json:"timestamp"`
	TenantID         string                 `json:"tenant_id"`
	Overall          string                 `json:"overall"` // healthy | degraded | unhealthy | unknown
	Checks           []QualityCheck         `json:"checks"`
	Metrics          map[string]float64     `json:"metrics"`
	SourceWatermarks map[string]interface{} `json:"source_watermarks"`
}

type QualityCheck struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // pass | warn | fail | unknown
	Message   string  `json:"message"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Measured  bool    `json:"measured"`
	Source    string  `json:"source"`
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
	db        *sql.DB
	controlDB *sql.DB
	logger    *zap.Logger
	config    MonitorConfig
}

type MonitorConfig struct {
	CheckInterval       time.Duration `env:"DQ_CHECK_INTERVAL" envDefault:"15m"`
	MinFlowRate         float64       `env:"DQ_MIN_FLOW_RATE" envDefault:"100"`     // 最低流速率 (flows/min)
	MaxMissingPercent   float64       `env:"DQ_MAX_MISSING" envDefault:"5.0"`       // 最大缺失率 %
	MaxLatencyP95       float64       `env:"DQ_MAX_LATENCY_P95" envDefault:"60000"` // 最大延迟 P95 ms
	MaxSchemaDriftCount int           `env:"DQ_MAX_SCHEMA_DRIFT" envDefault:"3"`
	MaxKafkaLag         int64         `env:"DQ_MAX_KAFKA_LAG" envDefault:"10000"`
	MaxSignalAge        time.Duration `env:"DQ_MAX_SIGNAL_AGE" envDefault:"5m"`
}

type Baseline struct {
	BaselineID       string                 `json:"baseline_id"`
	TenantID         string                 `json:"tenant_id"`
	DatasetID        string                 `json:"dataset_id"`
	BaselineVersion  int64                  `json:"baseline_version"`
	AvgFlowRate      float64                `json:"avg_flow_rate"`
	AvgPPS           float64                `json:"avg_pps"`
	AvgBPS           float64                `json:"avg_bps"`
	AvgPktLen        float64                `json:"avg_pktlen"`
	FeatureCount     int                    `json:"feature_count"`
	SampleCount      uint64                 `json:"sample_count"`
	SchemaSHA256     string                 `json:"schema_sha256"`
	SchemaColumns    []SchemaColumn         `json:"schema_columns"`
	SourceWatermarks map[string]interface{} `json:"source_watermarks"`
	WindowStart      time.Time              `json:"window_start"`
	WindowEnd        time.Time              `json:"window_end"`
	TraceID          string                 `json:"trace_id"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type SchemaColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DefaultKind string `json:"default_kind"`
}

func NewMonitor(db *sql.DB, cfg MonitorConfig, logger *zap.Logger) *Monitor {
	return &Monitor{db: db, config: cfg, logger: logger}
}

// SetControlDB binds the PostgreSQL quality control plane. Production baseline
// updates fail closed when it is absent instead of falling back to process memory.
func (m *Monitor) SetControlDB(db *sql.DB) {
	if m != nil {
		m.controlDB = db
	}
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
		Timestamp:        time.Now(),
		TenantID:         tenantID,
		Metrics:          make(map[string]float64),
		SourceWatermarks: make(map[string]interface{}),
	}

	// Check 1: 数据流入率 (flows_raw 最近 15 分钟)
	m.checkFlowRate(ctx, tenantID, report)

	// Check 2: 数据缺失 (feature_stat 与 sessions 对比)
	m.checkMissingData(ctx, tenantID, report)

	// Check 3: 端到端延迟 (ingest_ts → event_ts)
	m.checkEndToEndLatency(ctx, tenantID, report)

	// Check 4: Schema 漂移 (特征列数量)
	m.checkSchemaDrift(ctx, tenantID, report)

	// Checks 5-7: Kafka/Flink/Sink hand-off signals. These are loaded from
	// the last persisted real collection; the GET path never mutates state.
	m.applyHandoffSignals(ctx, tenantID, report)

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
		Measured: err == nil, Source: "clickhouse.traffic.flows_raw.ingest_ts",
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
		Measured: err == nil, Source: "clickhouse.traffic.sessions_to_feature_stat",
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
		Measured: err == nil, Source: "clickhouse.traffic.flows_raw.event_ts_to_ingest_ts",
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

func (m *Monitor) checkSchemaDrift(ctx context.Context, tenantID string, report *DataQualityReport) {
	columns, schemaHash, err := m.currentFlowSchema(ctx)
	colCount := float64(len(columns))
	status := "unknown"
	measured := false
	msg := "Schema baseline is not measured"

	if err != nil {
		msg = fmt.Sprintf("Schema check unavailable: %v", err)
		colCount = 0
	} else {
		baseline, baselineErr := m.GetBaseline(ctx, tenantID)
		switch {
		case baselineErr != nil:
			msg = fmt.Sprintf("Persistent schema baseline unavailable: %v", baselineErr)
		case baseline == nil:
			msg = "Persistent schema baseline is not configured; current schema is not treated as passing"
		default:
			measured = true
			status = "pass"
			msg = fmt.Sprintf("flows_raw schema hash matches persistent baseline v%d", baseline.BaselineVersion)
			if schemaHash != baseline.SchemaSHA256 {
				status = "fail"
				msg = fmt.Sprintf("Schema drift: current %s differs from baseline v%d %s", schemaHash, baseline.BaselineVersion, baseline.SchemaSHA256)
			}
		}
	}
	report.Metrics["flows_raw_columns"] = colCount
	report.Checks = append(report.Checks, QualityCheck{
		Name: "schema_drift", Status: status, Message: msg,
		Value: colCount, Threshold: float64(m.config.MaxSchemaDriftCount),
		Measured: measured, Source: "clickhouse.system.columns+postgresql.data_quality_baselines",
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

func (m *Monitor) UpdateBaseline(ctx context.Context, tenantID, requestedBy, traceID string) (*Baseline, error) {
	if m == nil || m.db == nil {
		return nil, fmt.Errorf("ClickHouse connection not available")
	}
	if m.controlDB == nil {
		return nil, fmt.Errorf("PostgreSQL data quality control plane not available")
	}
	if tenantID == "" || requestedBy == "" || traceID == "" {
		return nil, fmt.Errorf("tenant_id, requested_by and trace_id are required")
	}

	windowEnd := time.Now().UTC()
	windowStart := windowEnd.Add(-24 * time.Hour)
	baseline := &Baseline{
		BaselineID: uuid.NewString(), TenantID: tenantID, DatasetID: "flows_raw",
		WindowStart: windowStart, WindowEnd: windowEnd, TraceID: traceID, UpdatedAt: windowEnd,
	}
	var avgPPS, avgBPS, avgPktLen sql.NullFloat64
	var sampleCount interface{}
	if err := m.db.QueryRowContext(ctx, `
		SELECT avg(pps), avg(bps), avg(pktlen_mean), count()
		FROM traffic.feature_stat
		WHERE tenant_id = ? AND ts >= now() - INTERVAL 24 HOUR
	`, tenantID).Scan(&avgPPS, &avgBPS, &avgPktLen, &sampleCount); err != nil {
		return nil, fmt.Errorf("read ClickHouse baseline metrics: %w", err)
	}
	baseline.AvgPPS = finiteOrZero(nullFloat64(avgPPS))
	baseline.AvgBPS = finiteOrZero(nullFloat64(avgBPS))
	baseline.AvgPktLen = finiteOrZero(nullFloat64(avgPktLen))
	baseline.SampleCount = uint64(dbNumeric(sampleCount))

	columns, schemaHash, err := m.currentFlowSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("read ClickHouse baseline schema: %w", err)
	}
	baseline.SchemaColumns = columns
	baseline.SchemaSHA256 = schemaHash
	baseline.FeatureCount = len(columns)
	var featureWatermark string
	if err := m.db.QueryRowContext(ctx, `
		SELECT if(count() = 0, '', toString(max(ts)))
		FROM traffic.feature_stat
		WHERE tenant_id = ? AND ts >= now() - INTERVAL 24 HOUR
	`, tenantID).Scan(&featureWatermark); err != nil {
		return nil, fmt.Errorf("read ClickHouse feature watermark: %w", err)
	}
	baseline.SourceWatermarks = map[string]interface{}{
		"clickhouse.feature_stat.max_ts":     featureWatermark,
		"clickhouse.flows_raw.schema_sha256": schemaHash,
	}

	metricsJSON, err := json.Marshal(map[string]interface{}{
		"avg_flow_rate": baseline.AvgFlowRate,
		"avg_pps":       baseline.AvgPPS,
		"avg_bps":       baseline.AvgBPS,
		"avg_pktlen":    baseline.AvgPktLen,
		"feature_count": baseline.FeatureCount,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal baseline metrics: %w", err)
	}
	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, fmt.Errorf("marshal baseline schema: %w", err)
	}
	watermarksJSON, err := json.Marshal(baseline.SourceWatermarks)
	if err != nil {
		return nil, fmt.Errorf("marshal baseline watermarks: %w", err)
	}

	tx, err := m.controlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin persistent baseline transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenantID+":flows_raw"); err != nil {
		return nil, fmt.Errorf("lock persistent baseline stream: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data_quality_datasets (
			tenant_id,dataset_id,display_name,owner,schema_version,signal_contract_version,business_keys,
			allowed_lateness_seconds,retention_seconds,upstreams,downstreams,slo_target,
			status,revision,trace_id,created_at,updated_at
		) VALUES ($1,'flows_raw','Raw flow facts',$2,1,'data-quality-dataset-signals-v1','["event_id"]'::jsonb,
			60,2592000,'["flow.events.v1"]'::jsonb,'["flink-session-job","traffic.sessions"]'::jsonb,
			0.999,'active',1,$3,$4,$4)
		ON CONFLICT (tenant_id,dataset_id) DO NOTHING
	`, tenantID, requestedBy, traceID, windowEnd); err != nil {
		return nil, fmt.Errorf("ensure quality dataset: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(baseline_version),0)+1
		FROM data_quality_baselines WHERE tenant_id=$1 AND dataset_id='flows_raw'
	`, tenantID).Scan(&baseline.BaselineVersion); err != nil {
		return nil, fmt.Errorf("allocate baseline version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE data_quality_baselines SET status='superseded', updated_at=$3
		WHERE tenant_id=$1 AND dataset_id=$2 AND status='active'
	`, tenantID, baseline.DatasetID, windowEnd); err != nil {
		return nil, fmt.Errorf("supersede previous baseline: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data_quality_baselines (
			baseline_id,tenant_id,dataset_id,baseline_version,status,window_start,window_end,
			sample_count,metrics,schema_columns,schema_sha256,source_watermarks,created_by,trace_id,created_at,updated_at
		) VALUES ($1,$2,$3,$4,'active',$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11::jsonb,$12,$13,$6,$6)
	`, baseline.BaselineID, tenantID, baseline.DatasetID, baseline.BaselineVersion,
		windowStart, windowEnd, baseline.SampleCount, string(metricsJSON), string(columnsJSON), schemaHash,
		string(watermarksJSON), requestedBy, traceID); err != nil {
		return nil, fmt.Errorf("insert persistent baseline: %w", err)
	}
	eventID := uuid.NewString()
	payloadJSON, err := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "data_quality.baseline.activated.v1",
		"tenant_id": tenantID, "dataset_id": baseline.DatasetID,
		"baseline_id": baseline.BaselineID, "baseline_version": baseline.BaselineVersion,
		"schema_sha256": schemaHash, "trace_id": traceID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal baseline event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data_quality_outbox (
			event_id,tenant_id,aggregate_type,aggregate_id,aggregate_version,event_type,
			schema_version,partition_key,payload,trace_id,status,occurred_at
		) VALUES ($1,$2,'baseline',$3,$4,'data_quality.baseline.activated.v1',1,$2,$5::jsonb,$6,'pending',$7)
	`, eventID, tenantID, baseline.BaselineID, baseline.BaselineVersion, string(payloadJSON), traceID, windowEnd); err != nil {
		return nil, fmt.Errorf("insert baseline outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,success,result,risk_level,created_at
		) VALUES ($1,$2,NULLIF($3,''),'DATA_QUALITY_BASELINE_ACTIVATED','data_quality_baseline',$4,$5::jsonb,$6,true,'success','medium',$7)
	`, "audit-"+uuid.NewString(), tenantID, requestedBy, baseline.BaselineID, string(payloadJSON), traceID, windowEnd); err != nil {
		return nil, fmt.Errorf("insert baseline audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit persistent baseline: %w", err)
	}
	return baseline, nil
}

func (m *Monitor) GetBaseline(ctx context.Context, tenantID string) (*Baseline, error) {
	if m == nil || m.controlDB == nil {
		return nil, nil
	}
	var baseline Baseline
	var metricsJSON, columnsJSON, watermarksJSON []byte
	err := m.controlDB.QueryRowContext(ctx, `
		SELECT baseline_id::text,tenant_id,dataset_id,baseline_version,sample_count,
			metrics,schema_columns,schema_sha256,source_watermarks,window_start,window_end,trace_id,updated_at
		FROM data_quality_baselines
		WHERE tenant_id=$1 AND dataset_id='flows_raw' AND status='active'
		ORDER BY baseline_version DESC LIMIT 1
	`, tenantID).Scan(
		&baseline.BaselineID, &baseline.TenantID, &baseline.DatasetID, &baseline.BaselineVersion,
		&baseline.SampleCount, &metricsJSON, &columnsJSON, &baseline.SchemaSHA256, &watermarksJSON,
		&baseline.WindowStart, &baseline.WindowEnd, &baseline.TraceID, &baseline.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load persistent baseline: %w", err)
	}
	var metrics map[string]float64
	if err := json.Unmarshal(metricsJSON, &metrics); err != nil {
		return nil, fmt.Errorf("decode persistent baseline metrics: %w", err)
	}
	if err := json.Unmarshal(columnsJSON, &baseline.SchemaColumns); err != nil {
		return nil, fmt.Errorf("decode persistent baseline schema: %w", err)
	}
	if err := json.Unmarshal(watermarksJSON, &baseline.SourceWatermarks); err != nil {
		return nil, fmt.Errorf("decode persistent baseline watermarks: %w", err)
	}
	baseline.AvgFlowRate = metrics["avg_flow_rate"]
	baseline.AvgPPS = metrics["avg_pps"]
	baseline.AvgBPS = metrics["avg_bps"]
	baseline.AvgPktLen = metrics["avg_pktlen"]
	baseline.FeatureCount = int(metrics["feature_count"])
	return &baseline, nil
}

func (m *Monitor) currentFlowSchema(ctx context.Context) ([]SchemaColumn, string, error) {
	if m == nil || m.db == nil {
		return nil, "", fmt.Errorf("ClickHouse connection not available")
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT name,type,default_kind FROM system.columns
		WHERE database='traffic' AND table='flows_raw' ORDER BY position
	`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	columns := make([]SchemaColumn, 0, 64)
	for rows.Next() {
		var column SchemaColumn
		if err := rows.Scan(&column.Name, &column.Type, &column.DefaultKind); err != nil {
			return nil, "", err
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(columns) == 0 {
		return nil, "", fmt.Errorf("traffic.flows_raw schema is empty")
	}
	payload, err := json.Marshal(columns)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(payload)
	return columns, hex.EncodeToString(digest[:]), nil
}

// =============================================================================
// Helpers
// =============================================================================

func (m *Monitor) evaluateOverall(report *DataQualityReport) string {
	failCount, warnCount, unknownCount := 0, 0, 0
	for _, c := range report.Checks {
		switch c.Status {
		case "fail":
			failCount++
		case "warn":
			warnCount++
		case "unknown":
			unknownCount++
		}
	}
	if failCount > 0 {
		return "unhealthy"
	}
	if warnCount > 1 {
		return "degraded"
	}
	if unknownCount > 0 {
		return "unknown"
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

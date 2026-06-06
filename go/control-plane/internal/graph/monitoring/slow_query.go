////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/monitoring/slow_query.go
// Graph Service 慢查询检测器（完整实现）
// 功能：
// 1. 检测超过阈值的查询
// 2. 记录到 ClickHouse graph_slow_queries 表
// 3. 输出警告日志
////////////////////////////////////////////////////////////////////////////////

package monitoring

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// SlowQueryDetector 慢查询检测器
type SlowQueryDetector struct {
	threshold time.Duration
	client    *storage.ClickHouseClient
	logger    *zap.Logger
	enabled   bool
}

// NewSlowQueryDetector 创建慢查询检测器
func NewSlowQueryDetector(threshold time.Duration, client *storage.ClickHouseClient, logger *zap.Logger) *SlowQueryDetector {
	if threshold <= 0 {
		threshold = 5 * time.Second
	}

	return &SlowQueryDetector{
		threshold: threshold,
		client:    client,
		logger:    logger,
		enabled:   true,
	}
}

// SetEnabled 设置是否启用
func (d *SlowQueryDetector) SetEnabled(enabled bool) {
	d.enabled = enabled
}

// CheckAndLog 检查并记录慢查询
func (d *SlowQueryDetector) CheckAndLog(
	ctx context.Context,
	tenantID, queryID, queryType, centerIP string,
	depth int,
	runID string,
	duration time.Duration,
	nodeCount, edgeCount int,
	chRowsRead, chBytesRead uint64,
	errorMessage string,
) {
	if !d.enabled {
		return
	}

	// 检查是否超过阈值
	if duration < d.threshold {
		return
	}

	// 记录警告日志
	d.logger.Warn("Slow query detected",
		zap.String("tenant_id", tenantID),
		zap.String("query_id", queryID),
		zap.String("query_type", queryType),
		zap.String("center_ip", centerIP),
		zap.Int("depth", depth),
		zap.String("run_id", runID),
		zap.Duration("duration", duration),
		zap.Duration("threshold", d.threshold),
		zap.Int("node_count", nodeCount),
		zap.Int("edge_count", edgeCount),
		zap.Uint64("ch_rows_read", chRowsRead),
		zap.Uint64("ch_bytes_read", chBytesRead))

	// 异步写入 ClickHouse
	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := d.writeSlowQuery(writeCtx, tenantID, queryID, queryType, centerIP, depth, runID, duration, nodeCount, edgeCount, chRowsRead, chBytesRead, errorMessage); err != nil {
			d.logger.Error("Failed to write slow query to ClickHouse",
				zap.String("query_id", queryID),
				zap.Error(err))
		}
	}()
}

// writeSlowQuery 写入慢查询到 ClickHouse
func (d *SlowQueryDetector) writeSlowQuery(
	ctx context.Context,
	tenantID, queryID, queryType, centerIP string,
	depth int,
	runID string,
	duration time.Duration,
	nodeCount, edgeCount int,
	chRowsRead, chBytesRead uint64,
	errorMessage string,
) error {
	sql := `
		INSERT INTO traffic.graph_slow_queries (
			tenant_id, query_id, query_type,
			center_ip, depth, run_id,
			duration_ms,
			node_count, edge_count,
			ch_rows_read, ch_bytes_read,
			error_message,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return d.client.Exec(ctx, sql,
		tenantID,
		queryID,
		queryType,
		centerIP,
		uint8(depth),
		runID,
		uint32(duration.Milliseconds()),
		uint32(nodeCount),
		uint32(edgeCount),
		chRowsRead,
		chBytesRead,
		errorMessage,
		time.Now(),
	)
}

// GetThreshold 获取慢查询阈值
func (d *SlowQueryDetector) GetThreshold() time.Duration {
	return d.threshold
}

// SetThreshold 设置慢查询阈值
func (d *SlowQueryDetector) SetThreshold(threshold time.Duration) {
	if threshold > 0 {
		d.threshold = threshold
		d.logger.Info("Slow query threshold updated",
			zap.Duration("threshold", threshold))
	}
}

// GetSlowQueries 获取最近的慢查询
func (d *SlowQueryDetector) GetSlowQueries(ctx context.Context, tenantID string, hours int, limit int) ([]*SlowQuery, error) {
	if limit <= 0 {
		limit = 100
	}

	sql := `
		SELECT
			tenant_id, query_id, query_type,
			center_ip, depth, run_id,
			duration_ms,
			node_count, edge_count,
			ch_rows_read, ch_bytes_read,
			error_message,
			created_at
		FROM traffic.graph_slow_queries
		WHERE tenant_id = ?
		  AND created_at >= now() - INTERVAL ? HOUR
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := d.client.Query(ctx, sql, tenantID, hours, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query slow queries: %w", err)
	}
	defer rows.Close()

	var queries []*SlowQuery
	for rows.Next() {
		var q SlowQuery
		err := rows.Scan(
			&q.TenantID,
			&q.QueryID,
			&q.QueryType,
			&q.CenterIP,
			&q.Depth,
			&q.RunID,
			&q.DurationMs,
			&q.NodeCount,
			&q.EdgeCount,
			&q.CHRowsRead,
			&q.CHBytesRead,
			&q.ErrorMessage,
			&q.CreatedAt,
		)
		if err != nil {
			d.logger.Error("Failed to scan slow query", zap.Error(err))
			continue
		}
		queries = append(queries, &q)
	}

	return queries, rows.Err()
}

// SlowQuery 慢查询记录
type SlowQuery struct {
	TenantID     string
	QueryID      string
	QueryType    string
	CenterIP     string
	Depth        uint8
	RunID        string
	DurationMs   uint32
	NodeCount    uint32
	EdgeCount    uint32
	CHRowsRead   uint64
	CHBytesRead  uint64
	ErrorMessage string
	CreatedAt    time.Time
}

// GetStats 获取慢查询统计
func (d *SlowQueryDetector) GetStats(ctx context.Context, tenantID string, hours int) (*SlowQueryStats, error) {
	sql := `
		SELECT
			count() as total_count,
			uniq(query_type) as query_types,
			avg(duration_ms) as avg_duration_ms,
			max(duration_ms) as max_duration_ms,
			avg(node_count) as avg_node_count,
			avg(edge_count) as avg_edge_count
		FROM traffic.graph_slow_queries
		WHERE tenant_id = ?
		  AND created_at >= now() - INTERVAL ? HOUR
	`

	var stats SlowQueryStats
	row, err := d.client.QueryRow(ctx, sql, tenantID, hours)
	err = row.Scan(
		&stats.TotalCount,
		&stats.QueryTypes,
		&stats.AvgDurationMs,
		&stats.MaxDurationMs,
		&stats.AvgNodeCount,
		&stats.AvgEdgeCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get slow query stats: %w", err)
	}

	return &stats, nil
}

// SlowQueryStats 慢查询统计
type SlowQueryStats struct {
	TotalCount    uint64
	QueryTypes    uint64
	AvgDurationMs float64
	MaxDurationMs uint32
	AvgNodeCount  float64
	AvgEdgeCount  float64
}

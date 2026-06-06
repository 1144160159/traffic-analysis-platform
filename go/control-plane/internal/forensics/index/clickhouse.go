////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/index/clickhouse.go
// 修复：添加缺失的 ProbeID 字段
////////////////////////////////////////////////////////////////////////////////

package index

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// IndexQuery 索引查询参数
type IndexQuery struct {
	TenantID    string
	ProbeID     string // 修复：添加缺失的 ProbeID 字段
	SrcIP       string
	DstIP       string
	SrcPort     uint16
	DstPort     uint16
	Protocol    uint8
	CommunityID string
	StartTime   int64
	EndTime     int64
	Limit       int
}

// FileMetadata 文件元数据
type FileMetadata struct {
	FileKey     string
	TsStart     time.Time
	TsEnd       time.Time
	ProbeID     string
	ByteSize    uint64
	CommunityID string
	OffsetStart uint64
	OffsetEnd   uint64
}

// IndexClient 索引客户端
type IndexClient struct {
	client *storage.ClickHouseClient
	logger *zap.Logger
}

// NewIndexClient 创建索引客户端
func NewIndexClient(client *storage.ClickHouseClient, logger *zap.Logger) *IndexClient {
	return &IndexClient{
		client: client,
		logger: logger,
	}
}

// LookupFiles 查询匹配的 PCAP 文件
func (c *IndexClient) LookupFiles(ctx context.Context, query *IndexQuery) ([]*FileMetadata, error) {
	ctx, span := otel.StartSpan(ctx, "IndexClient.LookupFiles")
	defer span.End()

	// 构建 SQL
	sql := `
		SELECT DISTINCT
			file_key,
			ts_start,
			ts_end,
			probe_id,
			byte_size,
			community_id,
			offset_start,
			offset_end
		FROM traffic.pcap_index
		WHERE tenant_id = ?
		  AND ts_start <= toDateTime64(?, 3)
		  AND ts_end >= toDateTime64(?, 3)
	`

	args := []interface{}{
		query.TenantID,
		time.UnixMilli(query.EndTime),
		time.UnixMilli(query.StartTime),
	}

	// 可选条件
	if query.CommunityID != "" {
		sql += " AND community_id = ?"
		args = append(args, query.CommunityID)
	}

	// 修复：正确使用 ProbeID 字段
	if query.ProbeID != "" {
		sql += " AND probe_id = ?"
		args = append(args, query.ProbeID)
	}

	// 排序和限制
	sql += " ORDER BY ts_start ASC"

	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	sql += fmt.Sprintf(" LIMIT %d", limit)

	// 执行查询
	rows, err := c.client.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query pcap_index")
	}
	defer rows.Close()

	var files []*FileMetadata
	for rows.Next() {
		var file FileMetadata
		var offsetStart, offsetEnd *uint64

		if err := rows.Scan(
			&file.FileKey,
			&file.TsStart,
			&file.TsEnd,
			&file.ProbeID,
			&file.ByteSize,
			&file.CommunityID,
			&offsetStart,
			&offsetEnd,
		); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to scan row")
		}

		// 处理可空字段
		if offsetStart != nil {
			file.OffsetStart = *offsetStart
		}
		if offsetEnd != nil {
			file.OffsetEnd = *offsetEnd
		}

		files = append(files, &file)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "error iterating rows")
	}

	c.logger.Debug("PCAP files found",
		zap.String("tenant_id", query.TenantID),
		zap.String("probe_id", query.ProbeID),
		zap.String("community_id", query.CommunityID),
		zap.Int64("start_time", query.StartTime),
		zap.Int64("end_time", query.EndTime),
		zap.Int("count", len(files)))

	return files, nil
}

// LookupByCommunityID 根据 Community ID 查询文件
func (c *IndexClient) LookupByCommunityID(ctx context.Context, tenantID, communityID string) ([]*FileMetadata, error) {
	ctx, span := otel.StartSpan(ctx, "IndexClient.LookupByCommunityID")
	defer span.End()

	sql := `
		SELECT
			file_key,
			ts_start,
			ts_end,
			probe_id,
			byte_size,
			community_id,
			offset_start,
			offset_end
		FROM traffic.pcap_index
		WHERE tenant_id = ?
		  AND community_id = ?
		ORDER BY ts_start ASC
		LIMIT 100
	`

	rows, err := c.client.Query(ctx, sql, tenantID, communityID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query by community_id")
	}
	defer rows.Close()

	var files []*FileMetadata
	for rows.Next() {
		var file FileMetadata
		var offsetStart, offsetEnd *uint64

		if err := rows.Scan(
			&file.FileKey,
			&file.TsStart,
			&file.TsEnd,
			&file.ProbeID,
			&file.ByteSize,
			&file.CommunityID,
			&offsetStart,
			&offsetEnd,
		); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to scan row")
		}

		if offsetStart != nil {
			file.OffsetStart = *offsetStart
		}
		if offsetEnd != nil {
			file.OffsetEnd = *offsetEnd
		}

		files = append(files, &file)
	}

	return files, rows.Err()
}

// LookupByProbeID 根据 Probe ID 查询文件
func (c *IndexClient) LookupByProbeID(ctx context.Context, tenantID, probeID string, startTime, endTime int64) ([]*FileMetadata, error) {
	ctx, span := otel.StartSpan(ctx, "IndexClient.LookupByProbeID")
	defer span.End()

	sql := `
		SELECT
			file_key,
			ts_start,
			ts_end,
			probe_id,
			byte_size,
			community_id,
			offset_start,
			offset_end
		FROM traffic.pcap_index
		WHERE tenant_id = ?
		  AND probe_id = ?
		  AND ts_start <= toDateTime64(?, 3)
		  AND ts_end >= toDateTime64(?, 3)
		ORDER BY ts_start ASC
		LIMIT 1000
	`

	rows, err := c.client.Query(ctx, sql, tenantID, probeID, time.UnixMilli(endTime), time.UnixMilli(startTime))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to query by probe_id")
	}
	defer rows.Close()

	var files []*FileMetadata
	for rows.Next() {
		var file FileMetadata
		var offsetStart, offsetEnd *uint64

		if err := rows.Scan(
			&file.FileKey,
			&file.TsStart,
			&file.TsEnd,
			&file.ProbeID,
			&file.ByteSize,
			&file.CommunityID,
			&offsetStart,
			&offsetEnd,
		); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to scan row")
		}

		if offsetStart != nil {
			file.OffsetStart = *offsetStart
		}
		if offsetEnd != nil {
			file.OffsetEnd = *offsetEnd
		}

		files = append(files, &file)
	}

	return files, rows.Err()
}

// GetFilesByTimeRange 根据时间范围查询文件
func (c *IndexClient) GetFilesByTimeRange(ctx context.Context, tenantID string, startTime, endTime time.Time) ([]*FileMetadata, error) {
	query := &IndexQuery{
		TenantID:  tenantID,
		StartTime: startTime.UnixMilli(),
		EndTime:   endTime.UnixMilli(),
		Limit:     1000,
	}
	return c.LookupFiles(ctx, query)
}

// CountFiles 统计文件数量
func (c *IndexClient) CountFiles(ctx context.Context, tenantID string, startTime, endTime int64) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "IndexClient.CountFiles")
	defer span.End()

	sql := `
		SELECT count(DISTINCT file_key)
		FROM traffic.pcap_index
		WHERE tenant_id = ?
		  AND ts_start <= toDateTime64(?, 3)
		  AND ts_end >= toDateTime64(?, 3)
	`

	var count int64
	row, _ := c.client.QueryRow(ctx, sql, tenantID, time.UnixMilli(endTime), time.UnixMilli(startTime))
	if err := row.Scan(&count); err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to count files")
	}

	return count, nil
}

// Ping 检查连接
func (c *IndexClient) Ping(ctx context.Context) error {
	return c.client.Ping(ctx)
}

// Close 关闭客户端
func (c *IndexClient) Close() error {
	return c.client.Close()
}

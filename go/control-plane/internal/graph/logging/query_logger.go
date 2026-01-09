////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/logging/query_logger.go
// Graph Service 查询日志记录器（完整修复版）
// 修复内容：
// 1. 添加 Buffer 溢出保护（丢弃策略）
// 2. 添加优雅关闭等待
// 3. 修复批量写入错误处理
// 4. 添加指标统计
////////////////////////////////////////////////////////////////////////////////

package logging

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// QueryLog 查询日志结构
type QueryLog struct {
	TenantID  string
	QueryID   string
	UserID    string
	QueryType string
	CenterIP  string
	CenterIPs []string
	Depth     uint8
	RunID     string

	QueryStartTime time.Time
	QueryEndTime   time.Time

	NodeCount       uint32
	EdgeCount       uint32
	PathCount       uint32
	ResultSizeBytes uint64

	DurationMs uint32
	CacheHit   bool

	CHQueryCount      uint16
	CHTotalDurationMs uint32
	CHRowsRead        uint64
	CHBytesRead       uint64

	Status       string
	ErrorCode    string
	ErrorMessage string

	TraceID   string
	ClientIP  string
	UserAgent string

	CreatedAt time.Time
}

// QueryLogger 查询日志记录器
type QueryLogger struct {
	client *storage.ClickHouseClient
	logger *zap.Logger

	bufferSize    int
	batchSize     int
	flushInterval time.Duration

	buffer    []*QueryLog
	bufferMu  sync.Mutex
	flushChan chan struct{}
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// 修复：添加统计指标
	stats struct {
		totalLogged   int64
		totalFlushed  int64
		totalDropped  int64
		flushErrors   int64
		lastFlushTime time.Time
	}
}

// NewQueryLogger 创建查询日志记录器
func NewQueryLogger(client *storage.ClickHouseClient, logger *zap.Logger) *QueryLogger {
	ql := &QueryLogger{
		client:        client,
		logger:        logger,
		bufferSize:    1000,
		batchSize:     100,
		flushInterval: 5 * time.Second,
		buffer:        make([]*QueryLog, 0, 1000),
		flushChan:     make(chan struct{}, 1),
		stopChan:      make(chan struct{}),
	}

	ql.stats.lastFlushTime = time.Now()

	// 启动后台刷新器
	ql.wg.Add(1)
	go ql.backgroundFlusher()

	return ql
}

// Log 记录查询日志（异步，带溢出保护）
func (ql *QueryLogger) Log(ctx context.Context, log *QueryLog) {
	ql.bufferMu.Lock()
	defer ql.bufferMu.Unlock()

	log.CreatedAt = time.Now()

	// 修复：检查缓冲区是否已满
	if len(ql.buffer) >= ql.bufferSize {
		// 丢弃策略：丢弃最旧的日志
		atomic.AddInt64(&ql.stats.totalDropped, 1)
		ql.logger.Warn("Query log buffer full, dropping oldest log",
			zap.Int("buffer_size", len(ql.buffer)),
			zap.Int64("total_dropped", atomic.LoadInt64(&ql.stats.totalDropped)))

		// 移除最旧的 10% 日志
		dropCount := ql.bufferSize / 10
		if dropCount < 1 {
			dropCount = 1
		}
		ql.buffer = ql.buffer[dropCount:]
	}

	ql.buffer = append(ql.buffer, log)
	atomic.AddInt64(&ql.stats.totalLogged, 1)

	// 缓冲区接近满时触发刷新
	if len(ql.buffer) >= ql.bufferSize*9/10 {
		select {
		case ql.flushChan <- struct{}{}:
		default:
			// Channel 已满，跳过触发
		}
	}
}

// backgroundFlusher 后台刷新器
func (ql *QueryLogger) backgroundFlusher() {
	defer ql.wg.Done()

	ticker := time.NewTicker(ql.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ql.stopChan:
			// 最后一次刷新
			ql.logger.Info("Query logger stopping, flushing remaining logs")
			ql.flush()
			return

		case <-ticker.C:
			ql.flush()

		case <-ql.flushChan:
			ql.flush()
		}
	}
}

// flush 刷新缓冲区到数据库
func (ql *QueryLogger) flush() {
	ql.bufferMu.Lock()
	if len(ql.buffer) == 0 {
		ql.bufferMu.Unlock()
		return
	}

	// 取出当前缓冲区
	toFlush := ql.buffer
	ql.buffer = make([]*QueryLog, 0, ql.bufferSize)
	ql.bufferMu.Unlock()

	ql.logger.Debug("Flushing query logs",
		zap.Int("count", len(toFlush)))

	// 分批写入
	for i := 0; i < len(toFlush); i += ql.batchSize {
		end := i + ql.batchSize
		if end > len(toFlush) {
			end = len(toFlush)
		}

		batch := toFlush[i:end]
		if err := ql.writeBatch(batch); err != nil {
			ql.logger.Error("Failed to write query log batch",
				zap.Error(err),
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(batch)))
			atomic.AddInt64(&ql.stats.flushErrors, 1)
		} else {
			atomic.AddInt64(&ql.stats.totalFlushed, int64(len(batch)))
		}
	}

	ql.stats.lastFlushTime = time.Now()
}

// writeBatch 批量写入
func (ql *QueryLogger) writeBatch(logs []*QueryLog) error {
	// 修复：增加超时控制
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sql := `
		INSERT INTO traffic.graph_query_log (
			tenant_id, query_id, user_id,
			query_type, center_ip, center_ips, depth, run_id,
			query_start_time, query_end_time,
			node_count, edge_count, path_count, result_size_bytes,
			duration_ms, cache_hit,
			ch_query_count, ch_total_duration_ms, ch_rows_read, ch_bytes_read,
			status, error_code, error_message,
			trace_id, client_ip, user_agent,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	batch, err := ql.client.PrepareBatch(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, log := range logs {
		// 修复：添加字段验证
		if log.TenantID == "" {
			ql.logger.Warn("Skipping log with empty tenant_id", zap.String("query_id", log.QueryID))
			continue
		}

		err := batch.Append(
			log.TenantID,
			log.QueryID,
			log.UserID,
			log.QueryType,
			log.CenterIP,
			log.CenterIPs,
			log.Depth,
			log.RunID,
			log.QueryStartTime,
			log.QueryEndTime,
			log.NodeCount,
			log.EdgeCount,
			log.PathCount,
			log.ResultSizeBytes,
			log.DurationMs,
			boolToUint8(log.CacheHit),
			log.CHQueryCount,
			log.CHTotalDurationMs,
			log.CHRowsRead,
			log.CHBytesRead,
			log.Status,
			log.ErrorCode,
			log.ErrorMessage,
			log.TraceID,
			log.ClientIP,
			log.UserAgent,
			log.CreatedAt,
		)
		if err != nil {
			ql.logger.Error("Failed to append log to batch",
				zap.Error(err),
				zap.String("query_id", log.QueryID))
			continue
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	ql.logger.Debug("Query logs written successfully",
		zap.Int("count", len(logs)))

	return nil
}

// Close 关闭日志记录器（修复：等待刷新完成）
func (ql *QueryLogger) Close() error {
	ql.logger.Info("Closing query logger")

	// 发送停止信号
	close(ql.stopChan)

	// 等待后台任务完成（最多 10 秒）
	done := make(chan struct{})
	go func() {
		ql.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		ql.logger.Info("Query logger closed gracefully")
	case <-time.After(10 * time.Second):
		ql.logger.Warn("Query logger close timeout, forcing exit")
	}

	// 打印最终统计
	ql.logger.Info("Query logger final statistics",
		zap.Int64("total_logged", atomic.LoadInt64(&ql.stats.totalLogged)),
		zap.Int64("total_flushed", atomic.LoadInt64(&ql.stats.totalFlushed)),
		zap.Int64("total_dropped", atomic.LoadInt64(&ql.stats.totalDropped)),
		zap.Int64("flush_errors", atomic.LoadInt64(&ql.stats.flushErrors)))

	return nil
}

// GetStats 获取统计信息
func (ql *QueryLogger) GetStats() map[string]interface{} {
	ql.bufferMu.Lock()
	bufferLen := len(ql.buffer)
	ql.bufferMu.Unlock()

	return map[string]interface{}{
		"total_logged":    atomic.LoadInt64(&ql.stats.totalLogged),
		"total_flushed":   atomic.LoadInt64(&ql.stats.totalFlushed),
		"total_dropped":   atomic.LoadInt64(&ql.stats.totalDropped),
		"flush_errors":    atomic.LoadInt64(&ql.stats.flushErrors),
		"buffer_size":     bufferLen,
		"buffer_capacity": ql.bufferSize,
		"last_flush_time": ql.stats.lastFlushTime,
	}
}

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

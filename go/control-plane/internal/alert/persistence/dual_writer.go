////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/persistence/dual_writer.go
// 修复版：消除数据竞争、完善错误处理、增加重试逻辑
// 主要修复：使用 channel 消除 chErr/osErr 数据竞争
////////////////////////////////////////////////////////////////////////////////

package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fallback"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// DualWriter metrics
var (
	dualWriteTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_dual_write_total",
			Help: "Total number of dual write operations",
		},
		[]string{"storage", "status"},
	)

	dualWriteLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alert_dual_write_latency_seconds",
			Help:    "Dual write latency in seconds",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"storage"},
	)

	dualWriteBatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alert_dual_write_batch_size",
			Help:    "Dual write batch size",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"storage"},
	)

	dualWritePartialFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "alert_dual_write_partial_failures_total",
			Help: "Total number of partial write failures (one backend failed)",
		},
	)

	dualWriteTotalFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "alert_dual_write_total_failures_total",
			Help: "Total number of total write failures (all backends failed)",
		},
	)
)

// DualWriter 双写器（ClickHouse + OpenSearch）
type DualWriter struct {
	chWriter      *ClickHouseWriter
	osWriter      *OpenSearchWriter
	fallback      *fallback.FallbackStrategy
	maxRetries    int
	retryInterval time.Duration
	logger        *zap.Logger
}

// NewDualWriter 创建双写器
func NewDualWriter(
	chWriter *ClickHouseWriter,
	osWriter *OpenSearchWriter,
	failThreshold int,
	logger *zap.Logger,
) *DualWriter {
	return &DualWriter{
		chWriter:      chWriter,
		osWriter:      osWriter,
		fallback:      fallback.NewFallbackStrategy(failThreshold, 5*time.Minute, logger),
		maxRetries:    3,
		retryInterval: 100 * time.Millisecond,
		logger:        logger,
	}
}

// writeResult 写入结果
type writeResult struct {
	storageType fallback.StorageType
	err         error
	duration    time.Duration
}

// WriteAlert 写入告警到所有可用存储（修复版：消除数据竞争）
func (d *DualWriter) WriteAlert(ctx context.Context, alert *Alert) error {
	ctx, span := otel.StartSpan(ctx, "dual_writer.write_alert")
	defer span.End()

	targets := d.fallback.GetWriteTargets()
	if len(targets) == 0 {
		dualWriteTotalFailures.Inc()
		err := errors.New(errors.ErrCodeServiceUnavailable, "all storage backends unavailable")
		otel.RecordError(ctx, err)
		return err
	}

	// 使用 channel 收集结果（修复数据竞争）
	resultChan := make(chan writeResult, 2)

	for _, target := range targets {
		go func(t fallback.StorageType) {
			start := time.Now()
			var err error

			switch t {
			case fallback.StorageClickHouse:
				err = d.writeToClickHouse(ctx, alert)
			case fallback.StorageOpenSearch:
				err = d.writeToOpenSearch(ctx, alert)
			}

			resultChan <- writeResult{
				storageType: t,
				err:         err,
				duration:    time.Since(start),
			}
		}(target)
	}

	// 收集结果
	var chErr, osErr error
	for i := 0; i < len(targets); i++ {
		result := <-resultChan

		switch result.storageType {
		case fallback.StorageClickHouse:
			chErr = result.err
			dualWriteLatency.WithLabelValues("clickhouse").Observe(result.duration.Seconds())
			if result.err == nil {
				dualWriteTotal.WithLabelValues("clickhouse", "success").Inc()
			} else {
				dualWriteTotal.WithLabelValues("clickhouse", "failure").Inc()
			}

		case fallback.StorageOpenSearch:
			osErr = result.err
			dualWriteLatency.WithLabelValues("opensearch").Observe(result.duration.Seconds())
			if result.err == nil {
				dualWriteTotal.WithLabelValues("opensearch", "success").Inc()
			} else {
				dualWriteTotal.WithLabelValues("opensearch", "failure").Inc()
			}
		}
	}

	// 记录结果
	logger := logging.L(ctx)
	if chErr != nil {
		logger.Warn("ClickHouse write failed",
			zap.String("alert_id", alert.AlertID),
			zap.Error(chErr))
	}
	if osErr != nil {
		logger.Warn("OpenSearch write failed",
			zap.String("alert_id", alert.AlertID),
			zap.Error(osErr))
	}

	// 分析写入结果
	if chErr != nil && osErr != nil {
		dualWriteTotalFailures.Inc()
		return errors.Wrap(chErr, errors.ErrCodeDatabaseError, "all writes failed")
	}

	if chErr != nil || osErr != nil {
		dualWritePartialFailures.Inc()
	}

	return nil
}

// writeToClickHouse 写入 ClickHouse（带重试）
func (d *DualWriter) writeToClickHouse(ctx context.Context, alert *Alert) error {
	var lastErr error

	for attempt := 0; attempt < d.maxRetries; attempt++ {
		if !d.fallback.ShouldRetry(fallback.StorageClickHouse, attempt) {
			break
		}

		err := d.chWriter.WriteAlert(ctx, alert)
		if err == nil {
			d.fallback.RecordSuccess(fallback.StorageClickHouse)
			return nil
		}

		lastErr = err
		d.fallback.RecordFailure(ctx, fallback.StorageClickHouse, err)

		if attempt < d.maxRetries-1 {
			time.Sleep(d.retryInterval * time.Duration(attempt+1))
		}
	}

	return lastErr
}

// writeToOpenSearch 写入 OpenSearch（带重试）
func (d *DualWriter) writeToOpenSearch(ctx context.Context, alert *Alert) error {
	var lastErr error

	for attempt := 0; attempt < d.maxRetries; attempt++ {
		if !d.fallback.ShouldRetry(fallback.StorageOpenSearch, attempt) {
			break
		}

		err := d.osWriter.WriteAlert(ctx, alert)
		if err == nil {
			d.fallback.RecordSuccess(fallback.StorageOpenSearch)
			return nil
		}

		lastErr = err
		d.fallback.RecordFailure(ctx, fallback.StorageOpenSearch, err)

		if attempt < d.maxRetries-1 {
			time.Sleep(d.retryInterval * time.Duration(attempt+1))
		}
	}

	return lastErr
}

// WriteBatch 批量写入（修复版：消除数据竞争）
func (d *DualWriter) WriteBatch(ctx context.Context, alerts []*Alert) error {
	ctx, span := otel.StartSpan(ctx, "dual_writer.write_batch")
	defer span.End()

	if len(alerts) == 0 {
		return nil
	}

	targets := d.fallback.GetWriteTargets()
	if len(targets) == 0 {
		dualWriteTotalFailures.Inc()
		return errors.New(errors.ErrCodeServiceUnavailable, "all storage backends unavailable")
	}

	// 使用 channel 收集结果（修复数据竞争）
	resultChan := make(chan writeResult, 2)

	for _, target := range targets {
		go func(t fallback.StorageType) {
			start := time.Now()
			var err error

			switch t {
			case fallback.StorageClickHouse:
				err = d.chWriter.WriteBatch(ctx, alerts)
				if err == nil {
					d.fallback.RecordSuccess(fallback.StorageClickHouse)
					dualWriteTotal.WithLabelValues("clickhouse", "success").Inc()
				} else {
					d.fallback.RecordFailure(ctx, fallback.StorageClickHouse, err)
					dualWriteTotal.WithLabelValues("clickhouse", "failure").Inc()
				}
				dualWriteLatency.WithLabelValues("clickhouse").Observe(time.Since(start).Seconds())
				dualWriteBatchSize.WithLabelValues("clickhouse").Observe(float64(len(alerts)))

			case fallback.StorageOpenSearch:
				err = d.osWriter.WriteBatch(ctx, alerts)
				if err == nil {
					d.fallback.RecordSuccess(fallback.StorageOpenSearch)
					dualWriteTotal.WithLabelValues("opensearch", "success").Inc()
				} else {
					d.fallback.RecordFailure(ctx, fallback.StorageOpenSearch, err)
					dualWriteTotal.WithLabelValues("opensearch", "failure").Inc()
				}
				dualWriteLatency.WithLabelValues("opensearch").Observe(time.Since(start).Seconds())
				dualWriteBatchSize.WithLabelValues("opensearch").Observe(float64(len(alerts)))
			}

			resultChan <- writeResult{
				storageType: t,
				err:         err,
				duration:    time.Since(start),
			}
		}(target)
	}

	// 收集结果
	var chErr, osErr error
	for i := 0; i < len(targets); i++ {
		result := <-resultChan

		switch result.storageType {
		case fallback.StorageClickHouse:
			chErr = result.err
		case fallback.StorageOpenSearch:
			osErr = result.err
		}
	}

	if chErr != nil && osErr != nil {
		dualWriteTotalFailures.Inc()
		logging.L(ctx).Error("Batch write failed to all backends",
			zap.Int("count", len(alerts)),
			zap.NamedError("clickhouse_error", chErr),
			zap.NamedError("opensearch_error", osErr))
		return fmt.Errorf("all backends failed: ch=%v, os=%v", chErr, osErr)
	}

	if chErr != nil {
		dualWritePartialFailures.Inc()
		logging.L(ctx).Error("Primary storage (ClickHouse) failed",
			zap.Int("count", len(alerts)),
			zap.Error(chErr))
		// ClickHouse失败视为严重错误，返回错误
		return fmt.Errorf("primary storage failed: %w", chErr)
	}

	if osErr != nil {
		dualWritePartialFailures.Inc()
		logging.L(ctx).Warn("Secondary storage (OpenSearch) failed, continuing",
			zap.Int("count", len(alerts)),
			zap.Error(osErr))
		// OpenSearch失败仅记录警告，不影响主流程
	}

	return nil
}

// GetStatus 获取存储健康状态
func (d *DualWriter) GetStatus() map[fallback.StorageType]map[string]interface{} {
	return d.fallback.GetStatus()
}

// StartHealthCheck 启动后台健康检查
func (d *DualWriter) StartHealthCheck(ctx context.Context) {
	d.fallback.StartHealthCheck(ctx,
		func(ctx context.Context) error { return d.chWriter.Ping(ctx) },
		func(ctx context.Context) error { return d.osWriter.Ping(ctx) },
	)
}

// ForceRecovery 强制恢复存储
func (d *DualWriter) ForceRecovery(storageType fallback.StorageType) {
	d.fallback.ForceRecovery(storageType)
}

// Close 关闭双写器
func (d *DualWriter) Close() error {
	var errs []error

	if err := d.chWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("clickhouse: %w", err))
	}

	if err := d.osWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("opensearch: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing writers: %v", errs)
	}

	d.logger.Info("Dual writer closed")
	return nil
}

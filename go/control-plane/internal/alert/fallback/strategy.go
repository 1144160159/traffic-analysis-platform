package fallback

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

// Fallback metrics
var (
	storageAvailability = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alert_storage_availability",
			Help: "Storage backend availability (1=available, 0=unavailable)",
		},
		[]string{"storage"},
	)

	storageFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_storage_failures_total",
			Help: "Total number of storage failures",
		},
		[]string{"storage"},
	)

	storageRecoveries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_storage_recoveries_total",
			Help: "Total number of storage recoveries",
		},
		[]string{"storage"},
	)

	fallbackActivations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_fallback_activations_total",
			Help: "Total number of fallback activations",
		},
		[]string{"storage"},
	)
)

// StorageType 存储类型
type StorageType string

const (
	StorageClickHouse StorageType = "clickhouse"
	StorageOpenSearch StorageType = "opensearch"
)

// StorageHealth 存储健康状态
type StorageHealth struct {
	available        int32        // atomic: 1=可用, 0=不可用
	consecutiveFails int32        // atomic: 连续失败次数
	lastCheckTime    int64        // atomic: 上次检查时间 (unix ms)
	lastRecoveryTime int64        // atomic: 上次恢复时间
	lastErrorMsg     atomic.Value // string
}

// FallbackStrategy 降级策略
type FallbackStrategy struct {
	clickhouseHealth *StorageHealth
	opensearchHealth *StorageHealth
	failThreshold    int32
	recoveryWindow   time.Duration
	logger           *zap.Logger
}

// NewFallbackStrategy 创建降级策略
func NewFallbackStrategy(failThreshold int, recoveryWindow time.Duration, logger *zap.Logger) *FallbackStrategy {
	chHealth := &StorageHealth{available: 1}
	chHealth.lastErrorMsg.Store("")

	osHealth := &StorageHealth{available: 1}
	osHealth.lastErrorMsg.Store("")

	// 初始化指标
	storageAvailability.WithLabelValues(string(StorageClickHouse)).Set(1)
	storageAvailability.WithLabelValues(string(StorageOpenSearch)).Set(1)

	return &FallbackStrategy{
		clickhouseHealth: chHealth,
		opensearchHealth: osHealth,
		failThreshold:    int32(failThreshold),
		recoveryWindow:   recoveryWindow,
		logger:           logger,
	}
}

// RecordSuccess 记录成功操作
func (f *FallbackStrategy) RecordSuccess(storageType StorageType) {
	health := f.getHealth(storageType)
	if health == nil {
		return
	}

	wasUnavailable := atomic.LoadInt32(&health.available) == 0

	atomic.StoreInt32(&health.consecutiveFails, 0)
	atomic.StoreInt32(&health.available, 1)
	atomic.StoreInt64(&health.lastCheckTime, time.Now().UnixMilli())

	// 更新指标
	storageAvailability.WithLabelValues(string(storageType)).Set(1)

	// 如果从不可用恢复，记录恢复事件
	if wasUnavailable {
		atomic.StoreInt64(&health.lastRecoveryTime, time.Now().UnixMilli())
		storageRecoveries.WithLabelValues(string(storageType)).Inc()
		f.logger.Info("Storage recovered",
			zap.String("storage", string(storageType)))
	}
}

// RecordFailure 记录失败操作
func (f *FallbackStrategy) RecordFailure(ctx context.Context, storageType StorageType, err error) {
	health := f.getHealth(storageType)
	if health == nil {
		return
	}

	fails := atomic.AddInt32(&health.consecutiveFails, 1)
	health.lastErrorMsg.Store(err.Error())
	atomic.StoreInt64(&health.lastCheckTime, time.Now().UnixMilli())

	// 记录失败指标
	storageFailures.WithLabelValues(string(storageType)).Inc()

	if fails >= f.failThreshold {
		wasAvailable := atomic.CompareAndSwapInt32(&health.available, 1, 0)
		if wasAvailable {
			// 更新指标
			storageAvailability.WithLabelValues(string(storageType)).Set(0)
			fallbackActivations.WithLabelValues(string(storageType)).Inc()

			logging.L(ctx).Warn("Storage marked unavailable",
				zap.String("storage", string(storageType)),
				zap.Int32("consecutive_fails", fails),
				zap.String("last_error", err.Error()))

			// 使用OpenTelemetry记录事件
			span := trace.SpanFromContext(ctx)
			if span.SpanContext().IsValid() {
				span.AddEvent("storage_unavailable",
					trace.WithAttributes(
						attribute.String("storage", string(storageType)),
						attribute.Int64("consecutive_fails", int64(fails)),
						attribute.String("error", err.Error()),
					),
				)
			}
		}
	}
}

// IsAvailable 检查存储是否可用
func (f *FallbackStrategy) IsAvailable(storageType StorageType) bool {
	health := f.getHealth(storageType)
	if health == nil {
		return false
	}
	return atomic.LoadInt32(&health.available) == 1
}

// GetWriteTargets 获取当前可用的写入目标
func (f *FallbackStrategy) GetWriteTargets() []StorageType {
	targets := make([]StorageType, 0, 2)

	// ClickHouse 优先（OLAP 主查询）
	if f.IsAvailable(StorageClickHouse) {
		targets = append(targets, StorageClickHouse)
	}

	// OpenSearch 次之（全文检索）
	if f.IsAvailable(StorageOpenSearch) {
		targets = append(targets, StorageOpenSearch)
	}

	return targets
}

// ShouldRetry 判断是否应该重试
func (f *FallbackStrategy) ShouldRetry(storageType StorageType, attempt int) bool {
	if attempt >= 3 {
		return false
	}
	return f.IsAvailable(storageType)
}

// GetStatus 获取当前健康状态
func (f *FallbackStrategy) GetStatus() map[StorageType]map[string]interface{} {
	chLastError := ""
	if v := f.clickhouseHealth.lastErrorMsg.Load(); v != nil {
		chLastError = v.(string)
	}

	osLastError := ""
	if v := f.opensearchHealth.lastErrorMsg.Load(); v != nil {
		osLastError = v.(string)
	}

	return map[StorageType]map[string]interface{}{
		StorageClickHouse: {
			"available":          f.IsAvailable(StorageClickHouse),
			"consecutive_fails":  atomic.LoadInt32(&f.clickhouseHealth.consecutiveFails),
			"last_check_time":    time.UnixMilli(atomic.LoadInt64(&f.clickhouseHealth.lastCheckTime)),
			"last_recovery_time": time.UnixMilli(atomic.LoadInt64(&f.clickhouseHealth.lastRecoveryTime)),
			"last_error":         chLastError,
		},
		StorageOpenSearch: {
			"available":          f.IsAvailable(StorageOpenSearch),
			"consecutive_fails":  atomic.LoadInt32(&f.opensearchHealth.consecutiveFails),
			"last_check_time":    time.UnixMilli(atomic.LoadInt64(&f.opensearchHealth.lastCheckTime)),
			"last_recovery_time": time.UnixMilli(atomic.LoadInt64(&f.opensearchHealth.lastRecoveryTime)),
			"last_error":         osLastError,
		},
	}
}

// StartHealthCheck 启动后台健康检查
func (f *FallbackStrategy) StartHealthCheck(ctx context.Context, chPing, osPing func(context.Context) error) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	f.logger.Info("Starting storage health check",
		zap.Duration("interval", 30*time.Second))

	for {
		select {
		case <-ctx.Done():
			f.logger.Info("Health check stopped")
			return
		case <-ticker.C:
			f.performHealthCheck(ctx, chPing, osPing)
		}
	}
}

// performHealthCheck 执行一次健康检查
func (f *FallbackStrategy) performHealthCheck(ctx context.Context, chPing, osPing func(context.Context) error) {
	// 检查 ClickHouse
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := chPing(checkCtx); err == nil {
		f.RecordSuccess(StorageClickHouse)
	} else {
		f.RecordFailure(checkCtx, StorageClickHouse, err)
	}
	cancel()

	// 检查 OpenSearch
	checkCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	if err := osPing(checkCtx); err == nil {
		f.RecordSuccess(StorageOpenSearch)
	} else {
		f.RecordFailure(checkCtx, StorageOpenSearch, err)
	}
	cancel()
}

// ForceRecovery 强制恢复存储（运维用）
func (f *FallbackStrategy) ForceRecovery(storageType StorageType) {
	health := f.getHealth(storageType)
	if health == nil {
		return
	}

	atomic.StoreInt32(&health.consecutiveFails, 0)
	atomic.StoreInt32(&health.available, 1)
	atomic.StoreInt64(&health.lastRecoveryTime, time.Now().UnixMilli())
	health.lastErrorMsg.Store("")

	storageAvailability.WithLabelValues(string(storageType)).Set(1)
	storageRecoveries.WithLabelValues(string(storageType)).Inc()

	f.logger.Info("Storage force recovered",
		zap.String("storage", string(storageType)))
}

func (f *FallbackStrategy) getHealth(storageType StorageType) *StorageHealth {
	switch storageType {
	case StorageClickHouse:
		return f.clickhouseHealth
	case StorageOpenSearch:
		return f.opensearchHealth
	default:
		return nil
	}
}

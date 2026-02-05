////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/metrics/prometheus.go
// 修复版 v2：
// 1. 修复问题 6：添加 Session 事件指标
// 2. 补充详细设计要求的所有指标
// 3. 移除硬编码，使用 config 常量
////////////////////////////////////////////////////////////////////////////////

package metrics

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// Metrics 指标收集器
type Metrics struct {
	// Flow 事件指标
	flowEventsTotal *prometheus.CounterVec
	flowBytesTotal  *prometheus.CounterVec

	// 修复问题 6：Session 事件指标（新增）
	sessionEventsTotal *prometheus.CounterVec
	sessionBytesTotal  *prometheus.CounterVec

	// PCAP 索引指标
	pcapIndexTotal *prometheus.CounterVec

	// 错误指标
	errorsTotal *prometheus.CounterVec

	// 延迟指标
	latencyHistogram *prometheus.HistogramVec

	// 探针状态指标
	probeStatusGauge *prometheus.GaugeVec

	// === 详细设计要求的指标 ===

	// QPS 指标（按租户）
	ingestQPS *prometheus.CounterVec

	// 限流拒绝计数
	rejectTotal *prometheus.CounterVec

	// 认证失败计数
	authFailTotal prometheus.Counter

	// Kafka 写入延迟
	kafkaProduceLatency *prometheus.HistogramVec

	// Kafka 错误率
	kafkaErrorTotal prometheus.Counter

	// 去重指标
	dedupHitTotal  prometheus.Counter
	dedupMissTotal prometheus.Counter

	// 活跃连接数
	activeConnections prometheus.Gauge

	// 请求大小分布
	requestSizeHistogram *prometheus.HistogramVec

	// 批次大小分布
	batchSizeHistogram *prometheus.HistogramVec

	logger *zap.Logger

	// 内部统计（用于快速查询）
	totalEventsReceived int64
	totalEventsAccepted int64
	totalEventsRejected int64
}

// NewMetrics 创建指标收集器
func NewMetrics(logger *zap.Logger) *Metrics {
	m := &Metrics{
		flowEventsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_flow_events_total",
				Help: "Total number of flow events ingested",
			},
			[]string{"tenant_id", "status"},
		),

		flowBytesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_flow_bytes_total",
				Help: "Total bytes of flow events ingested",
			},
			[]string{"tenant_id"},
		),

		// 修复问题 6：添加 Session 事件指标
		sessionEventsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_session_events_total",
				Help: "Total number of session events ingested",
			},
			[]string{"tenant_id", "status"},
		),

		sessionBytesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_session_bytes_total",
				Help: "Total bytes of session events ingested",
			},
			[]string{"tenant_id"},
		),

		pcapIndexTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_pcap_index_total",
				Help: "Total number of PCAP index entries",
			},
			[]string{"tenant_id"},
		),

		errorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_errors_total",
				Help: "Total number of errors by type",
			},
			[]string{"error_type"},
		),

		latencyHistogram: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ingest_request_duration_seconds",
				Help:    "Request latency histogram",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
			},
			[]string{"operation"},
		),

		probeStatusGauge: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "probe_status",
				Help: "Probe status metrics",
			},
			[]string{"probe_id", "metric"},
		),

		// === 新增指标 ===

		ingestQPS: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_qps_total",
				Help: "Total number of ingest requests (QPS) by tenant",
			},
			[]string{"tenant_id", "method"},
		),

		rejectTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "ingest_reject_total",
				Help: "Total number of rejected requests by reason",
			},
			[]string{"reason"}, // 429, 400, 401, 503
		),

		authFailTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "ingest_auth_fail_total",
				Help: "Total number of authentication failures",
			},
		),

		kafkaProduceLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ingest_kafka_produce_duration_seconds",
				Help:    "Kafka produce latency histogram",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~2s
			},
			[]string{"topic"},
		),

		kafkaErrorTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "ingest_kafka_error_total",
				Help: "Total number of Kafka write errors",
			},
		),

		dedupHitTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "ingest_dedup_hit_total",
				Help: "Total number of duplicate events detected",
			},
		),

		dedupMissTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "ingest_dedup_miss_total",
				Help: "Total number of unique events",
			},
		),

		activeConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "ingest_active_connections",
				Help: "Number of active gRPC/HTTP connections",
			},
		),

		requestSizeHistogram: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ingest_request_size_bytes",
				Help:    "Request size distribution",
				Buckets: prometheus.ExponentialBuckets(1024, 2, 16), // 1KB to 32MB
			},
			[]string{"method"},
		),

		batchSizeHistogram: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ingest_batch_size",
				Help:    "Batch size distribution (number of events)",
				Buckets: prometheus.ExponentialBuckets(1, 2, 14), // 1 to 8192
			},
			[]string{"tenant_id"},
		),

		logger: logger,
	}

	return m
}

// RecordFlowEvents 记录 Flow 事件（带状态）
func (m *Metrics) RecordFlowEvents(tenantID string, count int64) {
	m.flowEventsTotal.WithLabelValues(tenantID, "accepted").Add(float64(count))
	m.ingestQPS.WithLabelValues(tenantID, "flow").Add(float64(count))
	atomic.AddInt64(&m.totalEventsAccepted, count)
}

// RecordFlowEventsRejected 记录被拒绝的 Flow 事件
func (m *Metrics) RecordFlowEventsRejected(tenantID string, count int64) {
	m.flowEventsTotal.WithLabelValues(tenantID, "rejected").Add(float64(count))
	atomic.AddInt64(&m.totalEventsRejected, count)
}

// RecordFlowBytes 记录 Flow 字节数
func (m *Metrics) RecordFlowBytes(tenantID string, bytes int64) {
	m.flowBytesTotal.WithLabelValues(tenantID).Add(float64(bytes))
}

// 修复问题 6：添加 Session 事件指标方法

// RecordSessionEvents 记录 Session 事件（新增）
func (m *Metrics) RecordSessionEvents(tenantID string, count int64) {
	m.sessionEventsTotal.WithLabelValues(tenantID, "accepted").Add(float64(count))
	m.ingestQPS.WithLabelValues(tenantID, "session").Add(float64(count))
	atomic.AddInt64(&m.totalEventsAccepted, count)
}

// RecordSessionEventsRejected 记录被拒绝的 Session 事件（新增）
func (m *Metrics) RecordSessionEventsRejected(tenantID string, count int64) {
	m.sessionEventsTotal.WithLabelValues(tenantID, "rejected").Add(float64(count))
	atomic.AddInt64(&m.totalEventsRejected, count)
}

// RecordSessionBytes 记录 Session 字节数（新增）
func (m *Metrics) RecordSessionBytes(tenantID string, bytes int64) {
	m.sessionBytesTotal.WithLabelValues(tenantID).Add(float64(bytes))
}

// RecordPcapIndex 记录 PCAP 索引
func (m *Metrics) RecordPcapIndex(tenantID string) {
	m.pcapIndexTotal.WithLabelValues(tenantID).Inc()
	m.ingestQPS.WithLabelValues(tenantID, "pcap").Inc()
}

// RecordError 记录错误
func (m *Metrics) RecordError(errorType string) {
	m.errorsTotal.WithLabelValues(errorType).Inc()
}

// RecordLatency 记录延迟
func (m *Metrics) RecordLatency(operation string, duration time.Duration) {
	m.latencyHistogram.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordProbeStatus 记录探针状态
func (m *Metrics) RecordProbeStatus(probeID string, status *pb.ProbeStatus) {
	if status == nil {
		return
	}
	m.probeStatusGauge.WithLabelValues(probeID, "cpu_usage").Set(float64(status.CpuUsage))
	m.probeStatusGauge.WithLabelValues(probeID, "memory_usage").Set(float64(status.MemoryUsage))
	m.probeStatusGauge.WithLabelValues(probeID, "capture_pps").Set(float64(status.CapturePps))
	m.probeStatusGauge.WithLabelValues(probeID, "upload_bps").Set(float64(status.UploadBps))
	m.probeStatusGauge.WithLabelValues(probeID, "packets_captured").Set(float64(status.PacketsCaptured))
	m.probeStatusGauge.WithLabelValues(probeID, "packets_dropped").Set(float64(status.PacketsDropped))
}

// === 新增方法 ===

// RecordReject429 记录限流拒绝
func (m *Metrics) RecordReject429() {
	m.rejectTotal.WithLabelValues("429").Inc()
}

// RecordReject400 记录请求无效拒绝
func (m *Metrics) RecordReject400() {
	m.rejectTotal.WithLabelValues("400").Inc()
}

// RecordReject401 记录认证失败拒绝
func (m *Metrics) RecordReject401() {
	m.rejectTotal.WithLabelValues("401").Inc()
}

// RecordReject503 记录服务不可用拒绝
func (m *Metrics) RecordReject503() {
	m.rejectTotal.WithLabelValues("503").Inc()
}

// RecordAuthFailure 记录认证失败
func (m *Metrics) RecordAuthFailure() {
	m.authFailTotal.Inc()
	m.rejectTotal.WithLabelValues("401").Inc()
}

// RecordKafkaLatency 记录 Kafka 写入延迟
func (m *Metrics) RecordKafkaLatency(topic string, duration time.Duration) {
	m.kafkaProduceLatency.WithLabelValues(topic).Observe(duration.Seconds())
}

// RecordKafkaError 记录 Kafka 错误
func (m *Metrics) RecordKafkaError() {
	m.kafkaErrorTotal.Inc()
	m.errorsTotal.WithLabelValues("kafka_write_failed").Inc()
}

// RecordDedupHit 记录去重命中
func (m *Metrics) RecordDedupHit() {
	m.dedupHitTotal.Inc()
}

// RecordDedupMiss 记录去重未命中（新事件）
func (m *Metrics) RecordDedupMiss() {
	m.dedupMissTotal.Inc()
}

// IncrActiveConnections 增加活跃连接数
func (m *Metrics) IncrActiveConnections() {
	m.activeConnections.Inc()
}

// DecrActiveConnections 减少活跃连接数
func (m *Metrics) DecrActiveConnections() {
	m.activeConnections.Dec()
}

// RecordRequestSize 记录请求大小
func (m *Metrics) RecordRequestSize(method string, bytes int64) {
	m.requestSizeHistogram.WithLabelValues(method).Observe(float64(bytes))
}

// RecordBatchSize 记录批次大小
func (m *Metrics) RecordBatchSize(tenantID string, size int) {
	m.batchSizeHistogram.WithLabelValues(tenantID).Observe(float64(size))
}

// GetStats 获取统计信息
func (m *Metrics) GetStats() MetricsStats {
	return MetricsStats{
		TotalEventsReceived: atomic.LoadInt64(&m.totalEventsReceived),
		TotalEventsAccepted: atomic.LoadInt64(&m.totalEventsAccepted),
		TotalEventsRejected: atomic.LoadInt64(&m.totalEventsRejected),
	}
}

// MetricsStats 统计信息
type MetricsStats struct {
	TotalEventsReceived int64
	TotalEventsAccepted int64
	TotalEventsRejected int64
}

// Handler 返回 Prometheus HTTP Handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

// StartServer 启动指标服务器
func (m *Metrics) StartServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	m.logger.Info("Metrics server starting", zap.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		m.logger.Error("Metrics server failed", zap.Error(err))
	}
}

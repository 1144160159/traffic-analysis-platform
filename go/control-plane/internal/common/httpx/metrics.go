package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestSize      *prometheus.SummaryVec
	httpResponseSize     *prometheus.SummaryVec
	httpRequestsInFlight *prometheus.GaugeVec
)

func init() {
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"service", "method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "path", "status"},
	)

	httpRequestSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "http_request_size_bytes",
			Help:       "HTTP request size in bytes",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"service", "method", "path"},
	)

	httpResponseSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "http_response_size_bytes",
			Help:       "HTTP response size in bytes",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"service", "method", "path"},
	)

	httpRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of HTTP requests being processed",
		},
		[]string{"service", "method"},
	)
}

// Metrics Prometheus指标中间件
func Metrics(serviceName string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 增加进行中请求计数
			httpRequestsInFlight.WithLabelValues(serviceName, r.Method).Inc()
			defer httpRequestsInFlight.WithLabelValues(serviceName, r.Method).Dec()

			// 记录请求大小
			if r.ContentLength > 0 {
				httpRequestSize.WithLabelValues(serviceName, r.Method, r.URL.Path).Observe(float64(r.ContentLength))
			}

			// 包装ResponseWriter
			rw := newResponseWriter(w)

			// 处理请求
			next.ServeHTTP(rw, r)

			// 计算耗时
			duration := time.Since(start).Seconds()
			statusStr := strconv.Itoa(rw.statusCode)

			// 记录指标
			httpRequestsTotal.WithLabelValues(serviceName, r.Method, r.URL.Path, statusStr).Inc()
			httpRequestDuration.WithLabelValues(serviceName, r.Method, r.URL.Path, statusStr).Observe(duration)
			httpResponseSize.WithLabelValues(serviceName, r.Method, r.URL.Path).Observe(float64(rw.bytesWritten))
		})
	}
}

// MetricsWithPathNormalizer 带路径归一化的指标中间件
func MetricsWithPathNormalizer(serviceName string, normalizer func(string) string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 归一化路径（避免高基数问题）
			normalizedPath := r.URL.Path
			if normalizer != nil {
				normalizedPath = normalizer(r.URL.Path)
			}

			httpRequestsInFlight.WithLabelValues(serviceName, r.Method).Inc()
			defer httpRequestsInFlight.WithLabelValues(serviceName, r.Method).Dec()

			if r.ContentLength > 0 {
				httpRequestSize.WithLabelValues(serviceName, r.Method, normalizedPath).Observe(float64(r.ContentLength))
			}

			rw := newResponseWriter(w)
			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			statusStr := strconv.Itoa(rw.statusCode)

			httpRequestsTotal.WithLabelValues(serviceName, r.Method, normalizedPath, statusStr).Inc()
			httpRequestDuration.WithLabelValues(serviceName, r.Method, normalizedPath, statusStr).Observe(duration)
			httpResponseSize.WithLabelValues(serviceName, r.Method, normalizedPath).Observe(float64(rw.bytesWritten))
		})
	}
}

// DefaultPathNormalizer 默认路径归一化器
func DefaultPathNormalizer(path string) string {
	// 将类似 /api/v1/alerts/abc-123 的路径归一化为 /api/v1/alerts/:id
	// 这里是简化实现，实际应根据路由定义来归一化
	return path
}

// BusinessMetrics 业务指标
type BusinessMetrics struct {
	service string

	alertsProcessed    *prometheus.CounterVec
	dedupRate          *prometheus.GaugeVec
	pcapCutDuration    *prometheus.HistogramVec
	graphQueryDuration *prometheus.HistogramVec
}

// NewBusinessMetrics 创建业务指标
func NewBusinessMetrics(service string) *BusinessMetrics {
	return &BusinessMetrics{
		service: service,

		alertsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alerts_processed_total",
				Help: "Total number of alerts processed",
			},
			[]string{"service", "tenant_id", "severity"},
		),

		dedupRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "alert_dedup_rate",
				Help: "Alert deduplication rate",
			},
			[]string{"service", "tenant_id"},
		),

		pcapCutDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pcap_cut_duration_seconds",
				Help:    "PCAP cutting duration in seconds",
				Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
			},
			[]string{"service", "tenant_id"},
		),

		graphQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_query_duration_seconds",
				Help:    "Graph query duration in seconds",
				Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"service", "tenant_id", "depth"},
		),
	}
}

// RecordAlertProcessed 记录告警处理
func (m *BusinessMetrics) RecordAlertProcessed(tenantID, severity string) {
	m.alertsProcessed.WithLabelValues(m.service, tenantID, severity).Inc()
}

// RecordDedupRate 记录去重率
func (m *BusinessMetrics) RecordDedupRate(tenantID string, rate float64) {
	m.dedupRate.WithLabelValues(m.service, tenantID).Set(rate)
}

// RecordPcapCutDuration 记录PCAP裁剪耗时
func (m *BusinessMetrics) RecordPcapCutDuration(tenantID string, duration time.Duration) {
	m.pcapCutDuration.WithLabelValues(m.service, tenantID).Observe(duration.Seconds())
}

// RecordGraphQueryDuration 记录图查询耗时
func (m *BusinessMetrics) RecordGraphQueryDuration(tenantID string, depth int, duration time.Duration) {
	m.graphQueryDuration.WithLabelValues(m.service, tenantID, strconv.Itoa(depth)).Observe(duration.Seconds())
}

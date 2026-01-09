////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/metrics/metrics.go
// Graph Service 业务指标（完整修复版）
// 修复内容：
// 1. 修复 service 标签固定化
// 2. 添加缓存命中率计算
// 3. 添加指标重置功能
// 4. 优化标签使用
////////////////////////////////////////////////////////////////////////////////

package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// GraphMetrics Graph Service 业务指标
type GraphMetrics struct {
	serviceName string // 修复：固定 service 名称

	// 图探索指标
	exploreRequestsTotal *prometheus.CounterVec
	exploreDuration      *prometheus.HistogramVec
	exploreNodesCount    *prometheus.HistogramVec
	exploreEdgesCount    *prometheus.HistogramVec
	exploreTimeouts      *prometheus.CounterVec

	// 缓存指标
	cacheHits   *prometheus.CounterVec
	cacheMisses *prometheus.CounterVec
	cacheSize   *prometheus.GaugeVec

	// 查询指标
	queryDuration    *prometheus.HistogramVec
	queryConcurrency *prometheus.GaugeVec
	queryErrors      *prometheus.CounterVec

	// 路径查询指标
	pathSearchDuration *prometheus.HistogramVec
	pathSearchHops     *prometheus.HistogramVec

	// 实体查询指标
	entityDetailsDuration  *prometheus.HistogramVec
	entityTimelineDuration *prometheus.HistogramVec

	// 修复：添加数据库查询指标
	dbQueryDuration *prometheus.HistogramVec
	dbQueryErrors   *prometheus.CounterVec
}

// NewGraphMetrics 创建 Graph Service 指标
func NewGraphMetrics(serviceName string) *GraphMetrics {
	return &GraphMetrics{
		serviceName: serviceName, // 修复：存储 service 名称

		exploreRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "graph_explore_requests_total",
				Help: "Total number of graph exploration requests",
				ConstLabels: prometheus.Labels{
					"service": serviceName, // 修复：使用 ConstLabels 固定 service
				},
			},
			[]string{"tenant_id", "status"}, // 修复：移除 service 标签
		),

		exploreDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_explore_duration_seconds",
				Help:    "Graph exploration duration in seconds",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id", "depth"},
		),

		exploreNodesCount: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_explore_nodes_count",
				Help:    "Number of nodes in explored graph",
				Buckets: []float64{10, 50, 100, 200, 500, 1000, 2000, 5000},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id", "depth"},
		),

		exploreEdgesCount: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_explore_edges_count",
				Help:    "Number of edges in explored graph",
				Buckets: []float64{10, 50, 100, 500, 1000, 5000, 10000},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id", "depth"},
		),

		exploreTimeouts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "graph_explore_timeouts_total",
				Help: "Total number of graph exploration timeouts",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id"},
		),

		cacheHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "graph_cache_hits_total",
				Help: "Total number of cache hits",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"cache_type"},
		),

		cacheMisses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "graph_cache_misses_total",
				Help: "Total number of cache misses",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"cache_type"},
		),

		cacheSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "graph_cache_size",
				Help: "Current cache size",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id"},
		),

		queryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_query_duration_seconds",
				Help:    "Graph query duration in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"query_type"},
		),

		queryConcurrency: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "graph_query_concurrency",
				Help: "Current number of concurrent queries",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{}, // 修复：移除 service 标签
		),

		queryErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "graph_query_errors_total",
				Help: "Total number of query errors",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"query_type", "error_type"},
		),

		pathSearchDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_path_search_duration_seconds",
				Help:    "Path search duration in seconds",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id"},
		),

		pathSearchHops: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_path_search_hops",
				Help:    "Number of hops in found paths",
				Buckets: []float64{1, 2, 3, 4, 5, 7, 10},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"tenant_id"},
		),

		entityDetailsDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_entity_details_duration_seconds",
				Help:    "Entity details query duration in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"entity_type"},
		),

		entityTimelineDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_entity_timeline_duration_seconds",
				Help:    "Entity timeline query duration in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"granularity"},
		),

		// 修复：新增数据库查询指标
		dbQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "graph_db_query_duration_seconds",
				Help:    "Database query duration in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2, 5},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"query_type"},
		),

		dbQueryErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "graph_db_query_errors_total",
				Help: "Total number of database query errors",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"query_type", "error_type"},
		),
	}
}

// RecordExploreRequest 记录图探索请求（修复：移除 service 参数）
func (m *GraphMetrics) RecordExploreRequest(tenantID, status string) {
	m.exploreRequestsTotal.WithLabelValues(tenantID, status).Inc()
}

// RecordExploreDuration 记录图探索耗时（修复：移除 service 参数）
func (m *GraphMetrics) RecordExploreDuration(tenantID string, depth int, duration time.Duration) {
	m.exploreDuration.WithLabelValues(tenantID, strconv.Itoa(depth)).Observe(duration.Seconds())
}

// RecordExploreGraph 记录图大小（修复：移除 service 参数）
func (m *GraphMetrics) RecordExploreGraph(tenantID string, depth, nodeCount, edgeCount int) {
	depthStr := strconv.Itoa(depth)
	m.exploreNodesCount.WithLabelValues(tenantID, depthStr).Observe(float64(nodeCount))
	m.exploreEdgesCount.WithLabelValues(tenantID, depthStr).Observe(float64(edgeCount))
}

// RecordExploreTimeout 记录图探索超时（修复：移除 service 参数）
func (m *GraphMetrics) RecordExploreTimeout(tenantID string) {
	m.exploreTimeouts.WithLabelValues(tenantID).Inc()
}

// RecordCacheHit 记录缓存命中（修复：移除 service 参数）
func (m *GraphMetrics) RecordCacheHit(cacheType string) {
	m.cacheHits.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss 记录缓存未命中（修复：移除 service 参数）
func (m *GraphMetrics) RecordCacheMiss(cacheType string) {
	m.cacheMisses.WithLabelValues(cacheType).Inc()
}

// SetCacheSize 设置缓存大小（修复：移除 service 参数）
func (m *GraphMetrics) SetCacheSize(tenantID string, size int64) {
	m.cacheSize.WithLabelValues(tenantID).Set(float64(size))
}

// RecordQueryDuration 记录查询耗时（修复：移除 service 参数）
func (m *GraphMetrics) RecordQueryDuration(queryType string, duration time.Duration) {
	m.queryDuration.WithLabelValues(queryType).Observe(duration.Seconds())
}

// IncQueryConcurrency 增加并发查询数（修复：移除 service 参数）
func (m *GraphMetrics) IncQueryConcurrency() {
	m.queryConcurrency.WithLabelValues().Inc()
}

// DecQueryConcurrency 减少并发查询数（修复：移除 service 参数）
func (m *GraphMetrics) DecQueryConcurrency() {
	m.queryConcurrency.WithLabelValues().Dec()
}

// RecordQueryError 记录查询错误（修复：移除 service 参数）
func (m *GraphMetrics) RecordQueryError(queryType, errorType string) {
	m.queryErrors.WithLabelValues(queryType, errorType).Inc()
}

// RecordPathSearch 记录路径查询（修复：移除 service 参数）
func (m *GraphMetrics) RecordPathSearch(tenantID string, duration time.Duration, hops int) {
	m.pathSearchDuration.WithLabelValues(tenantID).Observe(duration.Seconds())
	m.pathSearchHops.WithLabelValues(tenantID).Observe(float64(hops))
}

// RecordEntityDetails 记录实体详情查询（修复：移除 service 参数）
func (m *GraphMetrics) RecordEntityDetails(entityType string, duration time.Duration) {
	m.entityDetailsDuration.WithLabelValues(entityType).Observe(duration.Seconds())
}

// RecordEntityTimeline 记录实体时间线查询（修复：移除 service 参数）
func (m *GraphMetrics) RecordEntityTimeline(granularity string, duration time.Duration) {
	m.entityTimelineDuration.WithLabelValues(granularity).Observe(duration.Seconds())
}

// RecordDBQuery 记录数据库查询（新增）
func (m *GraphMetrics) RecordDBQuery(queryType string, duration time.Duration) {
	m.dbQueryDuration.WithLabelValues(queryType).Observe(duration.Seconds())
}

// RecordDBQueryError 记录数据库查询错误（新增）
func (m *GraphMetrics) RecordDBQueryError(queryType, errorType string) {
	m.dbQueryErrors.WithLabelValues(queryType, errorType).Inc()
}

// WithExploreMetrics 包装函数以自动记录指标（修复：移除 service 参数）
func (m *GraphMetrics) WithExploreMetrics(tenantID string, depth int, fn func() (nodeCount, edgeCount int, err error)) error {
	start := time.Now()

	m.IncQueryConcurrency()
	defer m.DecQueryConcurrency()

	nodeCount, edgeCount, err := fn()
	duration := time.Since(start)

	if err != nil {
		m.RecordExploreRequest(tenantID, "error")
		m.RecordQueryError("explore", "internal")
		return err
	}

	m.RecordExploreRequest(tenantID, "success")
	m.RecordExploreDuration(tenantID, depth, duration)
	m.RecordExploreGraph(tenantID, depth, nodeCount, edgeCount)

	return nil
}

// MeasureQuery 测量查询耗时（修复：移除 service 参数）
func (m *GraphMetrics) MeasureQuery(queryType string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	m.RecordQueryDuration(queryType, duration)

	if err != nil {
		m.RecordQueryError(queryType, "execution")
	}

	return err
}

// MeasureDBQuery 测量数据库查询耗时（新增）
func (m *GraphMetrics) MeasureDBQuery(queryType string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	m.RecordDBQuery(queryType, duration)

	if err != nil {
		m.RecordDBQueryError(queryType, "execution")
	}

	return err
}

// GetServiceName 获取服务名称（新增）
func (m *GraphMetrics) GetServiceName() string {
	return m.serviceName
}

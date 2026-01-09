////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/health/checker.go
// 健康检查器 - 完整修复版
////////////////////////////////////////////////////////////////////////////////

package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/publisher"
)

// =============================================================================
// 健康状态
// =============================================================================

// Status 健康状态
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
	StatusUnknown   Status = "unknown"
)

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Name      string                 `json:"name"`
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Latency   time.Duration          `json:"latency_ms"`
	Details   map[string]interface{} `json:"details,omitempty"`
	CheckedAt time.Time              `json:"checked_at"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status     Status                      `json:"status"`
	Service    string                      `json:"service"`
	Version    string                      `json:"version"`
	Components map[string]*ComponentHealth `json:"components"`
	Timestamp  time.Time                   `json:"timestamp"`
}

// =============================================================================
// Checker 健康检查器
// =============================================================================

// CheckerConfig 检查器配置
type CheckerConfig struct {
	ServiceName    string
	ServiceVersion string
	CheckTimeout   time.Duration
	Logger         *zap.Logger
}

// DefaultCheckerConfig 默认配置
func DefaultCheckerConfig() CheckerConfig {
	return CheckerConfig{
		ServiceName:    "rule-manager",
		ServiceVersion: "1.0.0",
		CheckTimeout:   5 * time.Second,
	}
}

// Checker 健康检查器
type Checker struct {
	config     CheckerConfig
	components map[string]ComponentChecker
	mu         sync.RWMutex
	logger     *zap.Logger
	ready      bool // 新增：就绪状态标志
}

// ComponentChecker 组件检查器接口
type ComponentChecker interface {
	Name() string
	Check(ctx context.Context) *ComponentHealth
}

// NewChecker 创建健康检查器
func NewChecker(logger *zap.Logger) *Checker {
	config := DefaultCheckerConfig()
	config.Logger = logger
	return NewCheckerWithConfig(config)
}

// NewCheckerWithConfig 创建健康检查器（使用配置）
func NewCheckerWithConfig(config CheckerConfig) *Checker {
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Checker{
		config:     config,
		components: make(map[string]ComponentChecker),
		logger:     logger,
		ready:      true, // 默认为就绪状态
	}
}

// RegisterComponent 注册组件检查器
func (c *Checker) RegisterComponent(checker ComponentChecker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.components[checker.Name()] = checker
}

// AddCheck 添加检查器（RegisterComponent 的别名）
func (c *Checker) AddCheck(name string, checker ComponentChecker) {
	c.RegisterComponent(checker)
}

// SetReady 设置就绪状态
func (c *Checker) SetReady(ready bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = ready
}

// IsReady 获取就绪状态
func (c *Checker) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Check 执行健康检查
func (c *Checker) Check(ctx context.Context) *HealthResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	response := &HealthResponse{
		Status:     StatusHealthy,
		Service:    c.config.ServiceName,
		Version:    c.config.ServiceVersion,
		Components: make(map[string]*ComponentHealth),
		Timestamp:  time.Now(),
	}

	// 并发检查所有组件
	var wg sync.WaitGroup
	resultCh := make(chan *ComponentHealth, len(c.components))

	for _, checker := range c.components {
		wg.Add(1)
		go func(ch ComponentChecker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, c.config.CheckTimeout)
			defer cancel()

			health := ch.Check(checkCtx)
			resultCh <- health
		}(checker)
	}

	// 等待所有检查完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	for health := range resultCh {
		response.Components[health.Name] = health

		// 更新整体状态
		switch health.Status {
		case StatusUnhealthy:
			response.Status = StatusUnhealthy
		case StatusDegraded:
			if response.Status != StatusUnhealthy {
				response.Status = StatusDegraded
			}
		}
	}

	return response
}

// CheckLive 存活检查（轻量级）
func (c *Checker) CheckLive(ctx context.Context) bool {
	// 存活检查只需要确认服务进程正常运行
	return true
}

// CheckReady 就绪检查（检查依赖 + 就绪状态）
func (c *Checker) CheckReady(ctx context.Context) *HealthResponse {
	response := c.Check(ctx)

	// 如果手动设置为不就绪，强制设置状态为 unhealthy
	if !c.IsReady() {
		response.Status = StatusUnhealthy
		response.Components["readiness"] = &ComponentHealth{
			Name:      "readiness",
			Status:    StatusUnhealthy,
			Message:   "service is shutting down",
			CheckedAt: time.Now(),
		}
	}

	return response
}

// =============================================================================
// HTTP Handler
// =============================================================================

// HealthHandler 健康检查 HTTP 处理器
func (c *Checker) HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := c.Check(r.Context())

	w.Header().Set("Content-Type", "application/json")

	switch response.Status {
	case StatusHealthy:
		w.WriteHeader(http.StatusOK)
	case StatusDegraded:
		w.WriteHeader(http.StatusOK) // 降级仍然返回 200
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// LivenessHandler 存活检查 HTTP 处理器（新增别名）
func (c *Checker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	c.LiveHandler(w, r)
}

// LiveHandler 存活检查 HTTP 处理器
func (c *Checker) LiveHandler(w http.ResponseWriter, r *http.Request) {
	if c.CheckLive(r.Context()) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"alive"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"dead"}`))
	}
}

// ReadinessHandler 就绪检查 HTTP 处理器（新增别名）
func (c *Checker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	c.ReadyHandler(w, r)
}

// ReadyHandler 就绪检查 HTTP 处理器
func (c *Checker) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	response := c.CheckReady(r.Context())

	w.Header().Set("Content-Type", "application/json")

	if response.Status == StatusHealthy || response.Status == StatusDegraded {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes 注册健康检查路由
func (c *Checker) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", c.HealthHandler)
	mux.HandleFunc("/healthz", c.LiveHandler)
	mux.HandleFunc("/readyz", c.ReadyHandler)
}

// =============================================================================
// PostgreSQL 检查器
// =============================================================================

// PostgresChecker PostgreSQL 健康检查器
type PostgresChecker struct {
	db *sql.DB
}

// NewPostgresChecker 创建 PostgreSQL 检查器
func NewPostgresChecker(db *sql.DB) *PostgresChecker {
	return &PostgresChecker{db: db}
}

// Name 返回组件名称
func (c *PostgresChecker) Name() string {
	return "postgresql"
}

// Check 执行检查
func (c *PostgresChecker) Check(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:      c.Name(),
		CheckedAt: time.Now(),
	}

	if c.db == nil {
		health.Status = StatusUnhealthy
		health.Message = "database connection is nil"
		return health
	}

	// 执行 ping
	if err := c.db.PingContext(ctx); err != nil {
		health.Status = StatusUnhealthy
		health.Message = fmt.Sprintf("ping failed: %v", err)
		health.Latency = time.Since(start)
		return health
	}

	// 获取连接池状态
	stats := c.db.Stats()
	health.Status = StatusHealthy
	health.Latency = time.Since(start)
	health.Details = map[string]interface{}{
		"open_connections": stats.OpenConnections,
		"in_use":           stats.InUse,
		"idle":             stats.Idle,
		"max_open":         stats.MaxOpenConnections,
		"wait_count":       stats.WaitCount,
		"wait_duration_ms": stats.WaitDuration.Milliseconds(),
	}

	// 检查连接池是否接近满载
	if stats.MaxOpenConnections > 0 {
		usageRatio := float64(stats.InUse) / float64(stats.MaxOpenConnections)
		if usageRatio > 0.9 {
			health.Status = StatusDegraded
			health.Message = fmt.Sprintf("connection pool near capacity: %.0f%%", usageRatio*100)
		}
	}

	return health
}

// =============================================================================
// Redis 检查器
// =============================================================================

// RedisChecker Redis 健康检查器
type RedisChecker struct {
	client *redis.Client
}

// NewRedisChecker 创建 Redis 检查器
func NewRedisChecker(client *redis.Client) *RedisChecker {
	return &RedisChecker{client: client}
}

// Name 返回组件名称
func (c *RedisChecker) Name() string {
	return "redis"
}

// Check 执行检查
func (c *RedisChecker) Check(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:      c.Name(),
		CheckedAt: time.Now(),
	}

	if c.client == nil {
		health.Status = StatusUnhealthy
		health.Message = "redis client is nil"
		return health
	}

	// 执行 ping
	if err := c.client.Ping(ctx).Err(); err != nil {
		health.Status = StatusUnhealthy
		health.Message = fmt.Sprintf("ping failed: %v", err)
		health.Latency = time.Since(start)
		return health
	}

	health.Status = StatusHealthy
	health.Latency = time.Since(start)

	// 获取 Redis 信息
	info, err := c.client.Info(ctx, "server", "memory").Result()
	if err == nil {
		health.Details = map[string]interface{}{
			"info_available": true,
		}
	} else {
		health.Details = map[string]interface{}{
			"info_available": false,
			"info_error":     err.Error(),
		}
	}
	_ = info // 可以进一步解析 info

	return health
}

// =============================================================================
// Kafka 检查器
// =============================================================================

// KafkaChecker Kafka 健康检查器（基于 Publisher）
type KafkaChecker struct {
	publisher *publisher.KafkaPublisher
}

// NewKafkaHealthChecker 创建 Kafka 检查器
func NewKafkaHealthChecker(pub *publisher.KafkaPublisher) *KafkaChecker {
	return &KafkaChecker{publisher: pub}
}

// Name 返回组件名称
func (c *KafkaChecker) Name() string {
	return "kafka"
}

// Check 执行检查
func (c *KafkaChecker) Check(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:      c.Name(),
		CheckedAt: time.Now(),
	}

	if c.publisher == nil {
		health.Status = StatusUnhealthy
		health.Message = "kafka publisher is nil"
		return health
	}

	// 检查 Publisher 健康状态
	if err := c.publisher.HealthCheck(ctx); err != nil {
		health.Status = StatusUnhealthy
		health.Message = fmt.Sprintf("health check failed: %v", err)
		health.Latency = time.Since(start)
		return health
	}

	health.Status = StatusHealthy
	health.Latency = time.Since(start)

	// 获取 Publisher 指标
	metrics := c.publisher.GetMetrics()
	health.Details = map[string]interface{}{
		"messages_sent":  metrics.RuleMessagesSent,
		"messages_error": metrics.RuleMessagesError,
		"last_send_time": metrics.RuleLastSendTime,
	}

	return health
}

// KafkaBrokerChecker Kafka Broker 检查器（直连模式）
type KafkaBrokerChecker struct {
	brokers []string
	timeout time.Duration
}

// NewKafkaBrokerChecker 创建 Kafka Broker 检查器
func NewKafkaBrokerChecker(brokers []string) *KafkaBrokerChecker {
	return &KafkaBrokerChecker{
		brokers: brokers,
		timeout: 5 * time.Second,
	}
}

// Name 返回组件名称
func (c *KafkaBrokerChecker) Name() string {
	return "kafka"
}

// Check 执行检查
func (c *KafkaBrokerChecker) Check(ctx context.Context) *ComponentHealth {
	start := time.Now()
	health := &ComponentHealth{
		Name:      c.Name(),
		CheckedAt: time.Now(),
	}

	if len(c.brokers) == 0 {
		health.Status = StatusUnhealthy
		health.Message = "no brokers configured"
		return health
	}

	// 尝试连接第一个 broker
	conn, err := kafka.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		health.Status = StatusUnhealthy
		health.Message = fmt.Sprintf("dial failed: %v", err)
		health.Latency = time.Since(start)
		return health
	}
	defer conn.Close()

	// 获取 broker 信息
	brokers, err := conn.Brokers()
	if err != nil {
		health.Status = StatusDegraded
		health.Message = fmt.Sprintf("failed to get brokers: %v", err)
		health.Latency = time.Since(start)
		return health
	}

	health.Status = StatusHealthy
	health.Latency = time.Since(start)
	health.Details = map[string]interface{}{
		"broker_count":       len(brokers),
		"configured_brokers": c.brokers,
	}

	return health
}

// =============================================================================
// 自定义检查器
// =============================================================================

// CustomChecker 自定义检查器
type CustomChecker struct {
	name      string
	checkFunc func(ctx context.Context) *ComponentHealth
}

// NewCustomChecker 创建自定义检查器
func NewCustomChecker(name string, checkFunc func(ctx context.Context) *ComponentHealth) *CustomChecker {
	return &CustomChecker{
		name:      name,
		checkFunc: checkFunc,
	}
}

// Name 返回组件名称
func (c *CustomChecker) Name() string {
	return c.name
}

// Check 执行检查
func (c *CustomChecker) Check(ctx context.Context) *ComponentHealth {
	return c.checkFunc(ctx)
}

// =============================================================================
// 辅助函数
// =============================================================================

// AggregateStatus 聚合多个状态
func AggregateStatus(statuses ...Status) Status {
	hasUnhealthy := false
	hasDegraded := false

	for _, s := range statuses {
		switch s {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/health/checker.go
// 完整修复版 v2：
// 1. 修复 #35：DummyChecker 名称误导（移除 _disabled 后缀）
// 2. 增加健康检查降级模式
// 3. 完善错误处理
////////////////////////////////////////////////////////////////////////////////

package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// Status 健康状态
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// CheckResult 检查结果
type CheckResult struct {
	Status    Status                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]CheckDetail `json:"checks"`
}

// CheckDetail 检查详情
type CheckDetail struct {
	Status   Status        `json:"status"`
	Message  string        `json:"message,omitempty"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
	Disabled bool          `json:"disabled,omitempty"` // 修复 #35：通过字段标识禁用状态
}

// Checker 健康检查器接口
type Checker interface {
	Check(ctx context.Context) (Status, error)
	Name() string
}

// HealthChecker 健康检查管理器
type HealthChecker struct {
	checkers map[string]Checker
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewHealthChecker 创建健康检查管理器
func NewHealthChecker(logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		checkers: make(map[string]Checker),
		logger:   logger,
	}
}

// AddChecker 添加检查器
func (h *HealthChecker) AddChecker(checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[checker.Name()] = checker
}

// RemoveChecker 移除检查器
func (h *HealthChecker) RemoveChecker(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checkers, name)
}

// Check 执行所有检查
func (h *HealthChecker) Check(ctx context.Context) *CheckResult {
	h.mu.RLock()
	checkers := make(map[string]Checker, len(h.checkers))
	for name, checker := range h.checkers {
		checkers[name] = checker
	}
	h.mu.RUnlock()

	result := &CheckResult{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Checks:    make(map[string]CheckDetail),
	}

	// 并发执行检查
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, checker := range checkers {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()

			start := time.Now()
			status, err := checker.Check(ctx)
			duration := time.Since(start)

			detail := CheckDetail{
				Status:   status,
				Duration: duration,
			}

			// 修复 #35：检查是否是 DummyChecker
			if dummy, ok := checker.(*DummyChecker); ok {
				detail.Disabled = true
				detail.Message = "Component disabled: " + dummy.reason
			} else {
				if err != nil {
					detail.Error = err.Error()
					detail.Message = "Check failed"
				} else if status == StatusHealthy {
					detail.Message = "OK"
				}
			}

			mu.Lock()
			result.Checks[name] = detail

			// 更新整体状态
			if status == StatusUnhealthy {
				result.Status = StatusUnhealthy
			} else if status == StatusDegraded && result.Status == StatusHealthy {
				result.Status = StatusDegraded
			}
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()

	return result
}

// Handler 返回 HTTP 处理函数
func (h *HealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		result := h.Check(ctx)

		statusCode := http.StatusOK
		if result.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if result.Status == StatusDegraded {
			statusCode = http.StatusOK // 降级时仍返回 200，但状态为 degraded
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(result)
	}
}

// ReadinessHandler 就绪检查处理函数（用于 K8s）
func (h *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		result := h.Check(ctx)

		// 只有完全健康才算就绪
		if result.Status == StatusHealthy {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ready",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "not ready",
			})
		}
	}
}

// LivenessHandler 存活检查处理函数（用于 K8s）
func (h *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 存活检查只检查服务本身是否活着，不检查依赖
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	}
}

// PostgresChecker PostgreSQL 健康检查器
type PostgresChecker struct {
	client *storage.PostgresClient
}

// NewPostgresChecker 创建 PostgreSQL 检查器
func NewPostgresChecker(client *storage.PostgresClient) *PostgresChecker {
	return &PostgresChecker{client: client}
}

// Check 执行检查
func (c *PostgresChecker) Check(ctx context.Context) (Status, error) {
	if err := c.client.Ping(ctx); err != nil {
		return StatusUnhealthy, err
	}
	return StatusHealthy, nil
}

// Name 返回名称
func (c *PostgresChecker) Name() string {
	return "postgresql"
}

// RedisChecker Redis 健康检查器
type RedisChecker struct {
	client *storage.RedisClient
}

// NewRedisChecker 创建 Redis 检查器
func NewRedisChecker(client *storage.RedisClient) *RedisChecker {
	return &RedisChecker{client: client}
}

// Check 执行检查
func (c *RedisChecker) Check(ctx context.Context) (Status, error) {
	if err := c.client.Ping(ctx); err != nil {
		return StatusUnhealthy, err
	}
	return StatusHealthy, nil
}

// Name 返回名称
func (c *RedisChecker) Name() string {
	return "redis"
}

// DummyChecker 虚拟检查器（修复 #35：当依赖未启用时使用）
type DummyChecker struct {
	name   string
	reason string // 禁用原因
}

// NewDummyChecker 创建虚拟检查器（修复 #35）
func NewDummyChecker(name string) *DummyChecker {
	return &DummyChecker{
		name:   name,
		reason: "component disabled in configuration",
	}
}

// NewDummyCheckerWithReason 创建带原因的虚拟检查器
func NewDummyCheckerWithReason(name, reason string) *DummyChecker {
	return &DummyChecker{
		name:   name,
		reason: reason,
	}
}

// Check 执行检查（修复 #35：返回 Degraded 而非 Healthy）
func (c *DummyChecker) Check(ctx context.Context) (Status, error) {
	// 修复 #35：返回 Degraded 状态，表明功能受限
	return StatusDegraded, nil
}

// Name 返回名称（修复 #35：不添加 _disabled 后缀）
func (c *DummyChecker) Name() string {
	return c.name
}

// IsDisabled 检查是否为禁用状态
func (c *DummyChecker) IsDisabled() bool {
	return true
}

// GetReason 获取禁用原因
func (c *DummyChecker) GetReason() string {
	return c.reason
}

// OIDCChecker OIDC 健康检查器（可选）
type OIDCChecker struct {
	enabled bool
	issuer  string
}

// NewOIDCChecker 创建 OIDC 检查器
func NewOIDCChecker(enabled bool, issuer string) *OIDCChecker {
	return &OIDCChecker{
		enabled: enabled,
		issuer:  issuer,
	}
}

// Check 执行检查
func (c *OIDCChecker) Check(ctx context.Context) (Status, error) {
	if !c.enabled {
		return StatusDegraded, nil
	}

	// 简单检查：验证 issuer 可达性
	// 实际生产环境可以调用 /.well-known/openid-configuration
	if c.issuer == "" {
		return StatusUnhealthy, nil
	}

	return StatusHealthy, nil
}

// Name 返回名称
func (c *OIDCChecker) Name() string {
	return "oidc"
}

// AggregatedChecker 聚合检查器（可选：组合多个检查器）
type AggregatedChecker struct {
	name     string
	checkers []Checker
	logger   *zap.Logger
}

// NewAggregatedChecker 创建聚合检查器
func NewAggregatedChecker(name string, checkers []Checker, logger *zap.Logger) *AggregatedChecker {
	return &AggregatedChecker{
		name:     name,
		checkers: checkers,
		logger:   logger,
	}
}

// Check 执行聚合检查
func (c *AggregatedChecker) Check(ctx context.Context) (Status, error) {
	overallStatus := StatusHealthy
	var lastError error

	for _, checker := range c.checkers {
		status, err := checker.Check(ctx)
		if err != nil {
			lastError = err
			c.logger.Warn("Aggregated check failed",
				zap.String("checker", checker.Name()),
				zap.Error(err))
		}

		// 更新整体状态（取最差状态）
		if status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	return overallStatus, lastError
}

// Name 返回名称
func (c *AggregatedChecker) Name() string {
	return c.name
}

// CustomChecker 自定义检查器（可扩展）
type CustomChecker struct {
	name      string
	checkFunc func(ctx context.Context) (Status, error)
}

// NewCustomChecker 创建自定义检查器
func NewCustomChecker(name string, checkFunc func(ctx context.Context) (Status, error)) *CustomChecker {
	return &CustomChecker{
		name:      name,
		checkFunc: checkFunc,
	}
}

// Check 执行检查
func (c *CustomChecker) Check(ctx context.Context) (Status, error) {
	if c.checkFunc == nil {
		return StatusUnhealthy, nil
	}
	return c.checkFunc(ctx)
}

// Name 返回名称
func (c *CustomChecker) Name() string {
	return c.name
}

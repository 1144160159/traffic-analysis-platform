////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/utils/circuit_breaker.go
// 新增文件：断路器（额外补充）
// 功能：
// 1. 防止级联故障（雪崩）
// 2. 支持三种状态：Closed（正常）、Open（熔断）、Half-Open（半开）
// 3. 支持失败率和慢调用比例两种熔断策略
// 4. 提供统计和健康检查
////////////////////////////////////////////////////////////////////////////////

package utils

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState 断路器状态
type CircuitState int32

const (
	// StateClosed 关闭状态（正常工作）
	StateClosed CircuitState = iota
	// StateOpen 开启状态（熔断中）
	StateOpen
	// StateHalfOpen 半开状态（尝试恢复）
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen 断路器开启错误
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests 请求过多错误（半开状态）
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig struct {
	// 名称
	Name string

	// 失败率阈值（0.0-1.0，例如 0.5 表示 50%）
	FailureThreshold float64
	// 慢调用阈值（超过此时间认为是慢调用）
	SlowCallThreshold time.Duration
	// 慢调用比例阈值（0.0-1.0）
	SlowCallRateThreshold float64

	// 滑动窗口大小（秒）
	WindowSize time.Duration
	// 最小请求数（窗口内请求数低于此值不触发熔断）
	MinimumRequests int

	// 熔断持续时间
	OpenTimeout time.Duration
	// 半开状态允许的请求数
	HalfOpenMaxRequests int

	// 状态变更回调
	OnStateChange func(from, to CircuitState)
}

// DefaultCircuitBreakerConfig 默认断路器配置
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.5, // 50% 失败率
		SlowCallThreshold:     time.Second,
		SlowCallRateThreshold: 0.5, // 50% 慢调用率
		WindowSize:            10 * time.Second,
		MinimumRequests:       5,
		OpenTimeout:           30 * time.Second,
		HalfOpenMaxRequests:   3,
		OnStateChange:         nil,
	}
}

// CircuitBreaker 断路器
type CircuitBreaker struct {
	config CircuitBreakerConfig
	state  int32 // atomic: CircuitState

	mu              sync.RWMutex
	metrics         *CircuitMetrics
	lastStateChange time.Time
	openedAt        time.Time
	halfOpenCount   int32 // atomic
}

// CircuitMetrics 断路器指标
type CircuitMetrics struct {
	mu sync.RWMutex

	totalRequests    int64
	successRequests  int64
	failedRequests   int64
	slowCalls        int64
	rejectedRequests int64

	// 滑动窗口
	window      []callRecord
	windowStart time.Time
	windowSize  time.Duration
}

type callRecord struct {
	success  bool
	duration time.Duration
	time     time.Time
}

// NewCircuitBreaker 创建断路器
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	// 验证配置
	if cfg.FailureThreshold <= 0 || cfg.FailureThreshold > 1 {
		cfg.FailureThreshold = 0.5
	}
	if cfg.SlowCallRateThreshold <= 0 || cfg.SlowCallRateThreshold > 1 {
		cfg.SlowCallRateThreshold = 0.5
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 10 * time.Second
	}
	if cfg.MinimumRequests <= 0 {
		cfg.MinimumRequests = 5
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMaxRequests <= 0 {
		cfg.HalfOpenMaxRequests = 3
	}

	return &CircuitBreaker{
		config: cfg,
		state:  int32(StateClosed),
		metrics: &CircuitMetrics{
			window:      make([]callRecord, 0, 1000),
			windowStart: time.Now(),
			windowSize:  cfg.WindowSize,
		},
		lastStateChange: time.Now(),
	}
}

// Execute 执行函数（带断路器保护）
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// 检查是否允许执行
	if err := cb.beforeCall(); err != nil {
		return err
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start)

	// 记录调用结果
	cb.afterCall(err == nil, duration)

	return err
}

// beforeCall 调用前检查
func (cb *CircuitBreaker) beforeCall() error {
	state := cb.GetState()

	switch state {
	case StateClosed:
		return nil

	case StateOpen:
		// 检查是否可以进入半开状态
		cb.mu.RLock()
		openedAt := cb.openedAt
		cb.mu.RUnlock()

		if time.Since(openedAt) > cb.config.OpenTimeout {
			cb.setState(StateHalfOpen)
			return nil
		}

		atomic.AddInt64(&cb.metrics.rejectedRequests, 1)
		return ErrCircuitOpen

	case StateHalfOpen:
		// 检查是否超过半开状态允许的请求数
		count := atomic.AddInt32(&cb.halfOpenCount, 1)
		if count > int32(cb.config.HalfOpenMaxRequests) {
			atomic.AddInt32(&cb.halfOpenCount, -1)
			atomic.AddInt64(&cb.metrics.rejectedRequests, 1)
			return ErrTooManyRequests
		}
		return nil

	default:
		return nil
	}
}

// afterCall 调用后记录
func (cb *CircuitBreaker) afterCall(success bool, duration time.Duration) {
	state := cb.GetState()

	// 记录指标
	cb.recordCall(success, duration)

	switch state {
	case StateClosed:
		// 检查是否需要熔断
		if cb.shouldOpen() {
			cb.setState(StateOpen)
			cb.mu.Lock()
			cb.openedAt = time.Now()
			cb.mu.Unlock()
		}

	case StateHalfOpen:
		atomic.AddInt32(&cb.halfOpenCount, -1)

		if success {
			// 半开状态下成功，检查是否可以完全恢复
			if cb.shouldClose() {
				cb.setState(StateClosed)
				atomic.StoreInt32(&cb.halfOpenCount, 0)
			}
		} else {
			// 半开状态下失败，重新进入开启状态
			cb.setState(StateOpen)
			cb.mu.Lock()
			cb.openedAt = time.Now()
			cb.mu.Unlock()
			atomic.StoreInt32(&cb.halfOpenCount, 0)
		}
	}
}

// recordCall 记录调用
func (cb *CircuitBreaker) recordCall(success bool, duration time.Duration) {
	cb.metrics.mu.Lock()
	defer cb.metrics.mu.Unlock()

	now := time.Now()

	// 清理过期的窗口数据
	cb.metrics.cleanExpiredRecords(now)

	// 记录新调用
	cb.metrics.window = append(cb.metrics.window, callRecord{
		success:  success,
		duration: duration,
		time:     now,
	})

	atomic.AddInt64(&cb.metrics.totalRequests, 1)
	if success {
		atomic.AddInt64(&cb.metrics.successRequests, 1)
	} else {
		atomic.AddInt64(&cb.metrics.failedRequests, 1)
	}

	if duration > cb.config.SlowCallThreshold {
		atomic.AddInt64(&cb.metrics.slowCalls, 1)
	}
}

// cleanExpiredRecords 清理过期记录
func (m *CircuitMetrics) cleanExpiredRecords(now time.Time) {
	cutoff := now.Add(-m.windowSize)
	i := 0
	for i < len(m.window) && m.window[i].time.Before(cutoff) {
		i++
	}
	if i > 0 {
		m.window = m.window[i:]
	}
}

// shouldOpen 检查是否应该开启断路器
func (cb *CircuitBreaker) shouldOpen() bool {
	cb.metrics.mu.RLock()
	defer cb.metrics.mu.RUnlock()

	windowRecords := cb.metrics.window
	if len(windowRecords) < cb.config.MinimumRequests {
		return false
	}

	// 计算失败率
	var failures, total int
	var slowCalls int

	for _, record := range windowRecords {
		total++
		if !record.success {
			failures++
		}
		if record.duration > cb.config.SlowCallThreshold {
			slowCalls++
		}
	}

	failureRate := float64(failures) / float64(total)
	slowCallRate := float64(slowCalls) / float64(total)

	// 检查是否超过阈值
	return failureRate >= cb.config.FailureThreshold ||
		slowCallRate >= cb.config.SlowCallRateThreshold
}

// shouldClose 检查是否应该关闭断路器
func (cb *CircuitBreaker) shouldClose() bool {
	cb.metrics.mu.RLock()
	defer cb.metrics.mu.RUnlock()

	windowRecords := cb.metrics.window
	if len(windowRecords) < cb.config.HalfOpenMaxRequests {
		return false
	}

	// 检查最近的请求是否都成功
	recentRecords := windowRecords
	if len(windowRecords) > cb.config.HalfOpenMaxRequests {
		recentRecords = windowRecords[len(windowRecords)-cb.config.HalfOpenMaxRequests:]
	}

	for _, record := range recentRecords {
		if !record.success {
			return false
		}
	}

	return true
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

// setState 设置状态
func (cb *CircuitBreaker) setState(newState CircuitState) {
	oldState := CircuitState(atomic.SwapInt32(&cb.state, int32(newState)))

	if oldState != newState {
		cb.mu.Lock()
		cb.lastStateChange = time.Now()
		cb.mu.Unlock()

		if cb.config.OnStateChange != nil {
			cb.config.OnStateChange(oldState, newState)
		}
	}
}

// Reset 重置断路器
func (cb *CircuitBreaker) Reset() {
	cb.setState(StateClosed)
	cb.metrics.mu.Lock()
	cb.metrics.window = cb.metrics.window[:0]
	cb.metrics.mu.Unlock()
	atomic.StoreInt64(&cb.metrics.totalRequests, 0)
	atomic.StoreInt64(&cb.metrics.successRequests, 0)
	atomic.StoreInt64(&cb.metrics.failedRequests, 0)
	atomic.StoreInt64(&cb.metrics.slowCalls, 0)
	atomic.StoreInt64(&cb.metrics.rejectedRequests, 0)
	atomic.StoreInt32(&cb.halfOpenCount, 0)
}

// GetMetrics 获取指标
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.metrics.mu.RLock()
	windowSize := len(cb.metrics.window)
	cb.metrics.mu.RUnlock()

	return CircuitBreakerMetrics{
		State:            cb.GetState(),
		TotalRequests:    atomic.LoadInt64(&cb.metrics.totalRequests),
		SuccessRequests:  atomic.LoadInt64(&cb.metrics.successRequests),
		FailedRequests:   atomic.LoadInt64(&cb.metrics.failedRequests),
		SlowCalls:        atomic.LoadInt64(&cb.metrics.slowCalls),
		RejectedRequests: atomic.LoadInt64(&cb.metrics.rejectedRequests),
		WindowSize:       windowSize,
		LastStateChange:  cb.lastStateChange,
	}
}

// CircuitBreakerMetrics 断路器指标
type CircuitBreakerMetrics struct {
	State            CircuitState
	TotalRequests    int64
	SuccessRequests  int64
	FailedRequests   int64
	SlowCalls        int64
	RejectedRequests int64
	WindowSize       int
	LastStateChange  time.Time
}

// FailureRate 失败率
func (m CircuitBreakerMetrics) FailureRate() float64 {
	if m.TotalRequests == 0 {
		return 0
	}
	return float64(m.FailedRequests) / float64(m.TotalRequests)
}

// SlowCallRate 慢调用率
func (m CircuitBreakerMetrics) SlowCallRate() float64 {
	if m.TotalRequests == 0 {
		return 0
	}
	return float64(m.SlowCalls) / float64(m.TotalRequests)
}

// ==================== CircuitBreakerGroup ====================

// CircuitBreakerGroup 断路器组
type CircuitBreakerGroup struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewCircuitBreakerGroup 创建断路器组
func NewCircuitBreakerGroup(defaultConfig CircuitBreakerConfig) *CircuitBreakerGroup {
	return &CircuitBreakerGroup{
		breakers: make(map[string]*CircuitBreaker),
		config:   defaultConfig,
	}
}

// Get 获取或创建断路器
func (g *CircuitBreakerGroup) Get(name string) *CircuitBreaker {
	g.mu.RLock()
	cb, exists := g.breakers[name]
	g.mu.RUnlock()

	if exists {
		return cb
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// 双重检查
	if cb, exists := g.breakers[name]; exists {
		return cb
	}

	config := g.config
	config.Name = name
	cb = NewCircuitBreaker(config)
	g.breakers[name] = cb

	return cb
}

// Execute 执行函数
func (g *CircuitBreakerGroup) Execute(ctx context.Context, name string, fn func() error) error {
	cb := g.Get(name)
	return cb.Execute(ctx, fn)
}

// GetAll 获取所有断路器
func (g *CircuitBreakerGroup) GetAll() map[string]*CircuitBreaker {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make(map[string]*CircuitBreaker, len(g.breakers))
	for k, v := range g.breakers {
		result[k] = v
	}
	return result
}

// ResetAll 重置所有断路器
func (g *CircuitBreakerGroup) ResetAll() {
	g.mu.RLock()
	breakers := make([]*CircuitBreaker, 0, len(g.breakers))
	for _, cb := range g.breakers {
		breakers = append(breakers, cb)
	}
	g.mu.RUnlock()

	for _, cb := range breakers {
		cb.Reset()
	}
}

// ==================== 便捷函数 ====================

// WrapWithCircuitBreaker 用断路器包装函数
func WrapWithCircuitBreaker(cb *CircuitBreaker, fn func() error) func() error {
	return func() error {
		return cb.Execute(context.Background(), fn)
	}
}

// ExecuteWithFallback 执行函数，失败时使用降级函数
func ExecuteWithFallback(ctx context.Context, cb *CircuitBreaker, fn func() error, fallback func() error) error {
	err := cb.Execute(ctx, fn)
	if err == ErrCircuitOpen || err == ErrTooManyRequests {
		if fallback != nil {
			return fallback()
		}
	}
	return err
}

// ==================== 场景特定配置 ====================

// DatabaseCircuitBreakerConfig 数据库断路器配置
func DatabaseCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.6, // 60% 失败率
		SlowCallThreshold:     5 * time.Second,
		SlowCallRateThreshold: 0.5,
		WindowSize:            30 * time.Second,
		MinimumRequests:       10,
		OpenTimeout:           60 * time.Second,
		HalfOpenMaxRequests:   5,
	}
}

// ExternalAPICircuitBreakerConfig 外部 API 断路器配置
func ExternalAPICircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.5,
		SlowCallThreshold:     3 * time.Second,
		SlowCallRateThreshold: 0.4,
		WindowSize:            20 * time.Second,
		MinimumRequests:       5,
		OpenTimeout:           30 * time.Second,
		HalfOpenMaxRequests:   3,
	}
}

// KafkaCircuitBreakerConfig Kafka 断路器配置
func KafkaCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.7,
		SlowCallThreshold:     2 * time.Second,
		SlowCallRateThreshold: 0.6,
		WindowSize:            15 * time.Second,
		MinimumRequests:       5,
		OpenTimeout:           20 * time.Second,
		HalfOpenMaxRequests:   3,
	}
}

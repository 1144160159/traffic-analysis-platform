////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/utils/retry.go
// 新增文件：通用重试器（额外补充）
// 功能：
// 1. 支持指数退避、固定间隔、抖动等多种重试策略
// 2. 支持可重试错误判断
// 3. 支持上下文取消
// 4. 提供重试统计
////////////////////////////////////////////////////////////////////////////////

package utils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryPolicy 重试策略
type RetryPolicy int

const (
	// RetryPolicyExponential 指数退避
	RetryPolicyExponential RetryPolicy = iota
	// RetryPolicyConstant 固定间隔
	RetryPolicyConstant
	// RetryPolicyLinear 线性增长
	RetryPolicyLinear
)

// RetryConfig 重试配置
type RetryConfig struct {
	// 最大重试次数（0 表示不重试）
	MaxAttempts int
	// 初始延迟
	InitialDelay time.Duration
	// 最大延迟
	MaxDelay time.Duration
	// 重试策略
	Policy RetryPolicy
	// 指数退避的倍数（默认 2.0）
	Multiplier float64
	// 抖动比例（0.0-1.0，默认 0.1）
	Jitter float64
	// 可重试错误判断函数（nil 表示所有错误都重试）
	IsRetryable func(error) bool
	// 重试前的回调
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Policy:       RetryPolicyExponential,
		Multiplier:   2.0,
		Jitter:       0.1,
		IsRetryable:  nil, // 所有错误都重试
		OnRetry:      nil,
	}
}

// Retry 执行重试
func Retry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	if cfg.MaxAttempts == 0 {
		return fn()
	}

	if cfg.Multiplier == 0 {
		cfg.Multiplier = 2.0
	}
	if cfg.Jitter < 0 {
		cfg.Jitter = 0
	}
	if cfg.Jitter > 1 {
		cfg.Jitter = 1
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxAttempts; attempt++ {
		// 第一次尝试不延迟
		if attempt > 0 {
			delay := calculateDelay(cfg, attempt-1)

			// 调用重试回调
			if cfg.OnRetry != nil {
				cfg.OnRetry(attempt, lastErr, delay)
			}

			// 等待延迟或上下文取消
			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled after %d attempts: %w, last error: %v", attempt, ctx.Err(), lastErr)
			case <-time.After(delay):
			}
		}

		// 执行函数
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否可重试
		if cfg.IsRetryable != nil && !cfg.IsRetryable(err) {
			return fmt.Errorf("non-retryable error after %d attempts: %w", attempt+1, err)
		}

		// 最后一次尝试后不再重试
		if attempt == cfg.MaxAttempts {
			break
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxAttempts, lastErr)
}

// calculateDelay 计算延迟时间
func calculateDelay(cfg RetryConfig, attempt int) time.Duration {
	var delay time.Duration

	switch cfg.Policy {
	case RetryPolicyExponential:
		// 指数退避: delay = initialDelay * (multiplier ^ attempt)
		delay = time.Duration(float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt)))

	case RetryPolicyConstant:
		// 固定间隔
		delay = cfg.InitialDelay

	case RetryPolicyLinear:
		// 线性增长: delay = initialDelay * (attempt + 1)
		delay = cfg.InitialDelay * time.Duration(attempt+1)
	}

	// 限制最大延迟
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// 添加抖动
	if cfg.Jitter > 0 {
		jitterAmount := float64(delay) * cfg.Jitter
		jitter := (rand.Float64()*2 - 1) * jitterAmount // -jitterAmount ~ +jitterAmount
		delay = time.Duration(float64(delay) + jitter)
		if delay < 0 {
			delay = 0
		}
	}

	return delay
}

// ==================== 便捷函数 ====================

// RetryWithExponentialBackoff 使用指数退避重试
func RetryWithExponentialBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = maxAttempts
	return Retry(ctx, cfg, fn)
}

// RetryWithConstantDelay 使用固定间隔重试
func RetryWithConstantDelay(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
	cfg := RetryConfig{
		MaxAttempts:  maxAttempts,
		InitialDelay: delay,
		MaxDelay:     delay,
		Policy:       RetryPolicyConstant,
	}
	return Retry(ctx, cfg, fn)
}

// RetryOnCondition 基于条件重试
func RetryOnCondition(ctx context.Context, maxAttempts int, isRetryable func(error) bool, fn func() error) error {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = maxAttempts
	cfg.IsRetryable = isRetryable
	return Retry(ctx, cfg, fn)
}

// ==================== 常见的可重试错误判断 ====================

// IsNetworkError 判断是否为网络错误
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsAny(errStr, []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
		"timeout",
	})
}

// IsTemporaryError 判断是否为临时错误
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否实现了 Temporary 接口
	type temporary interface {
		Temporary() bool
	}
	if te, ok := err.(temporary); ok {
		return te.Temporary()
	}

	return IsNetworkError(err)
}

// IsTimeoutError 判断是否为超时错误
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否实现了 Timeout 接口
	type timeout interface {
		Timeout() bool
	}
	if te, ok := err.(timeout); ok {
		return te.Timeout()
	}

	errStr := err.Error()
	return containsAny(errStr, []string{
		"timeout",
		"deadline exceeded",
		"context deadline exceeded",
	})
}

// ==================== Retryer 对象（面向对象风格） ====================

// Retryer 重试器
type Retryer struct {
	config RetryConfig
	stats  RetryStats
}

// RetryStats 重试统计
type RetryStats struct {
	TotalAttempts   int
	SuccessAttempts int
	FailedAttempts  int
	TotalRetries    int
	LastError       error
	LastAttemptTime time.Time
}

// NewRetryer 创建重试器
func NewRetryer(cfg RetryConfig) *Retryer {
	return &Retryer{
		config: cfg,
	}
}

// Do 执行重试
func (r *Retryer) Do(ctx context.Context, fn func() error) error {
	r.stats.TotalAttempts++
	r.stats.LastAttemptTime = time.Now()

	err := Retry(ctx, r.config, fn)
	if err != nil {
		r.stats.FailedAttempts++
		r.stats.LastError = err
	} else {
		r.stats.SuccessAttempts++
	}

	return err
}

// GetStats 获取统计信息
func (r *Retryer) GetStats() RetryStats {
	return r.stats
}

// ResetStats 重置统计
func (r *Retryer) ResetStats() {
	r.stats = RetryStats{}
}

// ==================== 辅助函数 ====================

func containsAny(s string, substrs []string) bool {
	sLower := toLower(s)
	for _, substr := range substrs {
		if contains(sLower, toLower(substr)) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsIgnoreCase(s, substr)
}

func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// ==================== 场景特定重试配置 ====================

// DatabaseRetryConfig 数据库操作重试配置
func DatabaseRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Policy:       RetryPolicyExponential,
		Multiplier:   2.0,
		Jitter:       0.1,
		IsRetryable:  IsTemporaryError,
	}
}

// KafkaRetryConfig Kafka 操作重试配置
func KafkaRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Policy:       RetryPolicyExponential,
		Multiplier:   2.0,
		Jitter:       0.1,
		IsRetryable:  IsNetworkError,
	}
}

// HTTPRetryConfig HTTP 请求重试配置
func HTTPRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Policy:       RetryPolicyExponential,
		Multiplier:   2.0,
		Jitter:       0.2,
		IsRetryable: func(err error) bool {
			return IsNetworkError(err) || IsTimeoutError(err)
		},
	}
}

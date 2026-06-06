package utils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

type RetryPolicy int

const (
	RetryPolicyExponential RetryPolicy = iota

	RetryPolicyConstant

	RetryPolicyLinear
)

type RetryConfig struct {
	MaxAttempts int

	InitialDelay time.Duration

	MaxDelay time.Duration

	Policy RetryPolicy

	Multiplier float64

	Jitter float64

	IsRetryable func(error) bool

	OnRetry func(attempt int, err error, delay time.Duration)
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Policy:       RetryPolicyExponential,
		Multiplier:   2.0,
		Jitter:       0.1,
		IsRetryable:  nil,
		OnRetry:      nil,
	}
}

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

		if attempt > 0 {
			delay := calculateDelay(cfg, attempt-1)

			if cfg.OnRetry != nil {
				cfg.OnRetry(attempt, lastErr, delay)
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled after %d attempts: %w, last error: %v", attempt, ctx.Err(), lastErr)
			case <-time.After(delay):
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if cfg.IsRetryable != nil && !cfg.IsRetryable(err) {
			return fmt.Errorf("non-retryable error after %d attempts: %w", attempt+1, err)
		}

		if attempt == cfg.MaxAttempts {
			break
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxAttempts, lastErr)
}

func calculateDelay(cfg RetryConfig, attempt int) time.Duration {
	var delay time.Duration

	switch cfg.Policy {
	case RetryPolicyExponential:

		delay = time.Duration(float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt)))

	case RetryPolicyConstant:

		delay = cfg.InitialDelay

	case RetryPolicyLinear:

		delay = cfg.InitialDelay * time.Duration(attempt+1)
	}

	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	if cfg.Jitter > 0 {
		jitterAmount := float64(delay) * cfg.Jitter
		jitter := (rand.Float64()*2 - 1) * jitterAmount
		delay = time.Duration(float64(delay) + jitter)
		if delay < 0 {
			delay = 0
		}
	}

	return delay
}

func RetryWithExponentialBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = maxAttempts
	return Retry(ctx, cfg, fn)
}

func RetryWithConstantDelay(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
	cfg := RetryConfig{
		MaxAttempts:  maxAttempts,
		InitialDelay: delay,
		MaxDelay:     delay,
		Policy:       RetryPolicyConstant,
	}
	return Retry(ctx, cfg, fn)
}

func RetryOnCondition(ctx context.Context, maxAttempts int, isRetryable func(error) bool, fn func() error) error {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = maxAttempts
	cfg.IsRetryable = isRetryable
	return Retry(ctx, cfg, fn)
}

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

func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	type temporary interface {
		Temporary() bool
	}
	if te, ok := err.(temporary); ok {
		return te.Temporary()
	}

	return IsNetworkError(err)
}

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

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

type Retryer struct {
	config RetryConfig
	stats  RetryStats
}

type RetryStats struct {
	TotalAttempts   int
	SuccessAttempts int
	FailedAttempts  int
	TotalRetries    int
	LastError       error
	LastAttemptTime time.Time
}

func NewRetryer(cfg RetryConfig) *Retryer {
	return &Retryer{
		config: cfg,
	}
}

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

func (r *Retryer) GetStats() RetryStats {
	return r.stats
}

func (r *Retryer) ResetStats() {
	r.stats = RetryStats{}
}

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

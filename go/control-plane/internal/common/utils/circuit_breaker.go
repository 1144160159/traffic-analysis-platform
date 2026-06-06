package utils

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type CircuitState int32

const (
	StateClosed CircuitState = iota

	StateOpen

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
	ErrCircuitOpen = errors.New("circuit breaker is open")

	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

type CircuitBreakerConfig struct {
	Name string

	FailureThreshold float64

	SlowCallThreshold time.Duration

	SlowCallRateThreshold float64

	WindowSize time.Duration

	MinimumRequests int

	OpenTimeout time.Duration

	HalfOpenMaxRequests int

	OnStateChange func(from, to CircuitState)
}

func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.5,
		SlowCallThreshold:     time.Second,
		SlowCallRateThreshold: 0.5,
		WindowSize:            10 * time.Second,
		MinimumRequests:       5,
		OpenTimeout:           30 * time.Second,
		HalfOpenMaxRequests:   3,
		OnStateChange:         nil,
	}
}

type CircuitBreaker struct {
	config CircuitBreakerConfig
	state  int32

	mu              sync.RWMutex
	metrics         *CircuitMetrics
	lastStateChange time.Time
	openedAt        time.Time
	halfOpenCount   int32
}

type CircuitMetrics struct {
	mu sync.RWMutex

	totalRequests    int64
	successRequests  int64
	failedRequests   int64
	slowCalls        int64
	rejectedRequests int64

	window      []callRecord
	windowStart time.Time
	windowSize  time.Duration
}

type callRecord struct {
	success  bool
	duration time.Duration
	time     time.Time
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {

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

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {

	if err := cb.beforeCall(); err != nil {
		return err
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start)

	cb.afterCall(err == nil, duration)

	return err
}

func (cb *CircuitBreaker) beforeCall() error {
	state := cb.GetState()

	switch state {
	case StateClosed:
		return nil

	case StateOpen:

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

func (cb *CircuitBreaker) afterCall(success bool, duration time.Duration) {
	state := cb.GetState()

	cb.recordCall(success, duration)

	switch state {
	case StateClosed:

		if cb.shouldOpen() {
			cb.setState(StateOpen)
			cb.mu.Lock()
			cb.openedAt = time.Now()
			cb.mu.Unlock()
		}

	case StateHalfOpen:
		atomic.AddInt32(&cb.halfOpenCount, -1)

		if success {

			if cb.shouldClose() {
				cb.setState(StateClosed)
				atomic.StoreInt32(&cb.halfOpenCount, 0)
			}
		} else {

			cb.setState(StateOpen)
			cb.mu.Lock()
			cb.openedAt = time.Now()
			cb.mu.Unlock()
			atomic.StoreInt32(&cb.halfOpenCount, 0)
		}
	}
}

func (cb *CircuitBreaker) recordCall(success bool, duration time.Duration) {
	cb.metrics.mu.Lock()
	defer cb.metrics.mu.Unlock()

	now := time.Now()

	cb.metrics.cleanExpiredRecords(now)

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

func (cb *CircuitBreaker) shouldOpen() bool {
	cb.metrics.mu.RLock()
	defer cb.metrics.mu.RUnlock()

	windowRecords := cb.metrics.window
	if len(windowRecords) < cb.config.MinimumRequests {
		return false
	}

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

	return failureRate >= cb.config.FailureThreshold ||
		slowCallRate >= cb.config.SlowCallRateThreshold
}

func (cb *CircuitBreaker) shouldClose() bool {
	cb.metrics.mu.RLock()
	defer cb.metrics.mu.RUnlock()

	windowRecords := cb.metrics.window
	if len(windowRecords) < cb.config.HalfOpenMaxRequests {
		return false
	}

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

func (cb *CircuitBreaker) GetState() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

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

func (m CircuitBreakerMetrics) FailureRate() float64 {
	if m.TotalRequests == 0 {
		return 0
	}
	return float64(m.FailedRequests) / float64(m.TotalRequests)
}

func (m CircuitBreakerMetrics) SlowCallRate() float64 {
	if m.TotalRequests == 0 {
		return 0
	}
	return float64(m.SlowCalls) / float64(m.TotalRequests)
}

type CircuitBreakerGroup struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

func NewCircuitBreakerGroup(defaultConfig CircuitBreakerConfig) *CircuitBreakerGroup {
	return &CircuitBreakerGroup{
		breakers: make(map[string]*CircuitBreaker),
		config:   defaultConfig,
	}
}

func (g *CircuitBreakerGroup) Get(name string) *CircuitBreaker {
	g.mu.RLock()
	cb, exists := g.breakers[name]
	g.mu.RUnlock()

	if exists {
		return cb
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if cb, exists := g.breakers[name]; exists {
		return cb
	}

	config := g.config
	config.Name = name
	cb = NewCircuitBreaker(config)
	g.breakers[name] = cb

	return cb
}

func (g *CircuitBreakerGroup) Execute(ctx context.Context, name string, fn func() error) error {
	cb := g.Get(name)
	return cb.Execute(ctx, fn)
}

func (g *CircuitBreakerGroup) GetAll() map[string]*CircuitBreaker {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make(map[string]*CircuitBreaker, len(g.breakers))
	for k, v := range g.breakers {
		result[k] = v
	}
	return result
}

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

func WrapWithCircuitBreaker(cb *CircuitBreaker, fn func() error) func() error {
	return func() error {
		return cb.Execute(context.Background(), fn)
	}
}

func ExecuteWithFallback(ctx context.Context, cb *CircuitBreaker, fn func() error, fallback func() error) error {
	err := cb.Execute(ctx, fn)
	if err == ErrCircuitOpen || err == ErrTooManyRequests {
		if fallback != nil {
			return fallback()
		}
	}
	return err
}

func DatabaseCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.6,
		SlowCallThreshold:     5 * time.Second,
		SlowCallRateThreshold: 0.5,
		WindowSize:            30 * time.Second,
		MinimumRequests:       10,
		OpenTimeout:           60 * time.Second,
		HalfOpenMaxRequests:   5,
	}
}

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

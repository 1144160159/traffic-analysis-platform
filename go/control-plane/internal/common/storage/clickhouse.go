////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/storage/clickhouse.go
// 修复版：
// 1. 增加自动重连机制（带指数退避）
// 2. 增加连接健康检查
// 3. 增加 Prometheus 指标
// 4. 增加连接状态通知回调
////////////////////////////////////////////////////////////////////////////////

package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// ClickHouseConfig ClickHouse配置
type ClickHouseConfig struct {
	Hosts           []string      `env:"CLICKHOUSE_HOSTS" envSeparator:","`
	Database        string        `env:"CLICKHOUSE_DATABASE" envDefault:"traffic"`
	Username        string        `env:"CLICKHOUSE_USERNAME" envDefault:"default"`
	Password        string        `env:"CLICKHOUSE_PASSWORD"`
	MaxOpenConns    int           `env:"CLICKHOUSE_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns    int           `env:"CLICKHOUSE_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"CLICKHOUSE_CONN_MAX_LIFETIME" envDefault:"1h"`
	DialTimeout     time.Duration `env:"CLICKHOUSE_DIAL_TIMEOUT" envDefault:"10s"`
	ReadTimeout     time.Duration `env:"CLICKHOUSE_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout    time.Duration `env:"CLICKHOUSE_WRITE_TIMEOUT" envDefault:"30s"`
	CompressionLZ4  bool          `env:"CLICKHOUSE_COMPRESSION_LZ4" envDefault:"true"`
	Debug           bool          `env:"CLICKHOUSE_DEBUG" envDefault:"false"`

	// 重连配置
	EnableAutoReconnect bool          `env:"CLICKHOUSE_AUTO_RECONNECT" envDefault:"true"`
	ReconnectInterval   time.Duration `env:"CLICKHOUSE_RECONNECT_INTERVAL" envDefault:"5s"`
	MaxReconnectDelay   time.Duration `env:"CLICKHOUSE_MAX_RECONNECT_DELAY" envDefault:"60s"`
	HealthCheckInterval time.Duration `env:"CLICKHOUSE_HEALTH_CHECK_INTERVAL" envDefault:"30s"`
}

// ConnectionState 连接状态
type ConnectionState int32

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateReconnecting
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// ConnectionCallback 连接状态变化回调
type ConnectionCallback func(oldState, newState ConnectionState, err error)

// ClickHouseClient ClickHouse客户端（带自动重连）
type ClickHouseClient struct {
	conn   driver.Conn
	config ClickHouseConfig
	logger *zap.Logger

	// 连接状态
	state          int32 // atomic: ConnectionState
	mu             sync.RWMutex
	reconnectTimer *time.Timer
	stopReconnect  chan struct{}
	healthTicker   *time.Ticker
	stopHealth     chan struct{}

	// 回调
	callbacks []ConnectionCallback

	// 指标
	metrics *clickhouseMetrics

	// 生命周期
	closed int32 // atomic
}

// clickhouseMetrics ClickHouse 指标
type clickhouseMetrics struct {
	connectionState   prometheus.Gauge
	reconnectAttempts prometheus.Counter
	reconnectFailures prometheus.Counter
	queryDuration     prometheus.Histogram
	queryErrors       prometheus.Counter
	batchInserts      prometheus.Counter
}

func newClickHouseMetrics(serviceName string) *clickhouseMetrics {
	return &clickhouseMetrics{
		connectionState: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "clickhouse_connection_state",
			Help:        "ClickHouse connection state (0=disconnected, 1=connecting, 2=connected, 3=reconnecting)",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		reconnectAttempts: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "clickhouse_reconnect_attempts_total",
			Help:        "Total number of ClickHouse reconnection attempts",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		reconnectFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "clickhouse_reconnect_failures_total",
			Help:        "Total number of ClickHouse reconnection failures",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		queryDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "clickhouse_query_duration_seconds",
			Help:        "ClickHouse query duration in seconds",
			ConstLabels: prometheus.Labels{"service": serviceName},
			Buckets:     []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}),
		queryErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "clickhouse_query_errors_total",
			Help:        "Total number of ClickHouse query errors",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		batchInserts: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "clickhouse_batch_inserts_total",
			Help:        "Total number of ClickHouse batch inserts",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
	}
}

// NewClickHouseClient 创建ClickHouse客户端
func NewClickHouseClient(cfg ClickHouseConfig, logger *zap.Logger) (*ClickHouseClient, error) {
	if len(cfg.Hosts) == 0 {
		return nil, fmt.Errorf("clickhouse hosts not configured")
	}

	// 设置默认值
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 5 * time.Second
	}
	if cfg.MaxReconnectDelay <= 0 {
		cfg.MaxReconnectDelay = 60 * time.Second
	}
	if cfg.HealthCheckInterval <= 0 {
		cfg.HealthCheckInterval = 30 * time.Second
	}

	client := &ClickHouseClient{
		config:        cfg,
		logger:        logger,
		stopReconnect: make(chan struct{}),
		stopHealth:    make(chan struct{}),
		metrics:       newClickHouseMetrics("clickhouse"),
	}

	// 初始连接
	if err := client.connect(); err != nil {
		if !cfg.EnableAutoReconnect {
			return nil, err
		}
		logger.Warn("Initial connection failed, will retry in background",
			zap.Error(err))
		go client.reconnectLoop()
	}

	// 启动健康检查
	if cfg.EnableAutoReconnect {
		go client.healthCheckLoop()
	}

	return client, nil
}

// connect 建立连接
func (c *ClickHouseClient) connect() error {
	c.setState(StateConnecting)

	options := &clickhouse.Options{
		Addr: c.config.Hosts,
		Auth: clickhouse.Auth{
			Database: c.config.Database,
			Username: c.config.Username,
			Password: c.config.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     c.config.DialTimeout,
		MaxOpenConns:    c.config.MaxOpenConns,
		MaxIdleConns:    c.config.MaxIdleConns,
		ConnMaxLifetime: c.config.ConnMaxLifetime,
		Debug:           c.config.Debug,
		Debugf: func(format string, v ...interface{}) {
			c.logger.Debug(fmt.Sprintf(format, v...))
		},
	}

	if c.config.CompressionLZ4 {
		options.Compression = &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		}
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		c.setState(StateDisconnected)
		return fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), c.config.DialTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		c.setState(StateDisconnected)
		return fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	c.mu.Lock()
	// 关闭旧连接
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = conn
	c.mu.Unlock()

	c.setState(StateConnected)

	c.logger.Info("Connected to ClickHouse",
		zap.Strings("hosts", c.config.Hosts),
		zap.String("database", c.config.Database))

	return nil
}

// reconnectLoop 重连循环
func (c *ClickHouseClient) reconnectLoop() {
	attempt := 0
	baseDelay := c.config.ReconnectInterval

	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			return
		}

		state := c.GetState()
		if state == StateConnected {
			// 已连接，重置计数
			attempt = 0
			time.Sleep(baseDelay)
			continue
		}

		// 计算退避延迟（指数退避）
		delay := baseDelay * time.Duration(1<<uint(attempt))
		if delay > c.config.MaxReconnectDelay {
			delay = c.config.MaxReconnectDelay
		}

		c.logger.Info("Attempting to reconnect to ClickHouse",
			zap.Int("attempt", attempt+1),
			zap.Duration("delay", delay))

		c.metrics.reconnectAttempts.Inc()

		select {
		case <-c.stopReconnect:
			return
		case <-time.After(delay):
		}

		c.setState(StateReconnecting)
		if err := c.connect(); err != nil {
			c.metrics.reconnectFailures.Inc()
			c.logger.Error("Reconnection failed",
				zap.Error(err),
				zap.Int("attempt", attempt+1))
			attempt++
		} else {
			c.logger.Info("Reconnected to ClickHouse successfully")
			attempt = 0
		}
	}
}

// healthCheckLoop 健康检查循环
func (c *ClickHouseClient) healthCheckLoop() {
	c.healthTicker = time.NewTicker(c.config.HealthCheckInterval)
	defer c.healthTicker.Stop()

	for {
		select {
		case <-c.stopHealth:
			return
		case <-c.healthTicker.C:
			if atomic.LoadInt32(&c.closed) == 1 {
				return
			}

			if c.GetState() != StateConnected {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.Ping(ctx)
			cancel()

			if err != nil {
				c.logger.Warn("Health check failed, marking as disconnected",
					zap.Error(err))
				c.setState(StateDisconnected)
			}
		}
	}
}

// setState 设置连接状态并触发回调
func (c *ClickHouseClient) setState(newState ConnectionState) {
	oldState := ConnectionState(atomic.SwapInt32(&c.state, int32(newState)))

	if oldState == newState {
		return
	}

	c.metrics.connectionState.Set(float64(newState))

	// 触发回调
	c.mu.RLock()
	callbacks := c.callbacks
	c.mu.RUnlock()

	for _, cb := range callbacks {
		go cb(oldState, newState, nil)
	}

	c.logger.Info("ClickHouse connection state changed",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()))
}

// GetState 获取当前连接状态
func (c *ClickHouseClient) GetState() ConnectionState {
	return ConnectionState(atomic.LoadInt32(&c.state))
}

// OnStateChange 注册状态变化回调
func (c *ClickHouseClient) OnStateChange(cb ConnectionCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = append(c.callbacks, cb)
}

// Conn 获取原生连接（会等待连接可用）
func (c *ClickHouseClient) Conn() (driver.Conn, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, fmt.Errorf("client is closed")
	}

	// 等待连接可用（最多等待 30 秒）
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if c.GetState() == StateConnected {
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()
			if conn != nil {
				return conn, nil
			}
		}

		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for connection")
		case <-ticker.C:
			continue
		}
	}
}

// Query 执行查询
func (c *ClickHouseClient) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	ctx, span := otel.StartSpan(ctx, "clickhouse.query")
	defer span.End()

	conn, err := c.Conn()
	if err != nil {
		return nil, err
	}

	start := time.Now()
	rows, err := conn.Query(ctx, query, args...)
	duration := time.Since(start)

	c.metrics.queryDuration.Observe(duration.Seconds())

	if err != nil {
		c.metrics.queryErrors.Inc()
		c.logger.Error("ClickHouse query failed",
			zap.Error(err),
			zap.String("query", truncateQuery(query)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return nil, fmt.Errorf("query failed: %w", err)
	}

	c.logger.Debug("ClickHouse query executed",
		zap.String("query", truncateQuery(query)),
		zap.Duration("duration", duration))

	return rows, nil
}

// QueryRow 执行单行查询
func (c *ClickHouseClient) QueryRow(ctx context.Context, query string, args ...interface{}) (driver.Row, error) {
	ctx, span := otel.StartSpan(ctx, "clickhouse.query_row")
	defer span.End()

	conn, err := c.Conn()
	if err != nil {
		return nil, err
	}

	return conn.QueryRow(ctx, query, args...), nil
}

// Exec 执行命令
func (c *ClickHouseClient) Exec(ctx context.Context, query string, args ...interface{}) error {
	ctx, span := otel.StartSpan(ctx, "clickhouse.exec")
	defer span.End()

	conn, err := c.Conn()
	if err != nil {
		return err
	}

	start := time.Now()
	err = conn.Exec(ctx, query, args...)
	duration := time.Since(start)

	if err != nil {
		c.metrics.queryErrors.Inc()
		c.logger.Error("ClickHouse exec failed",
			zap.Error(err),
			zap.String("query", truncateQuery(query)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return fmt.Errorf("exec failed: %w", err)
	}

	c.logger.Debug("ClickHouse exec completed",
		zap.String("query", truncateQuery(query)),
		zap.Duration("duration", duration))

	return nil
}

// PrepareBatch 准备批量插入
func (c *ClickHouseClient) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	ctx, span := otel.StartSpan(ctx, "clickhouse.prepare_batch")
	defer span.End()

	conn, err := c.Conn()
	if err != nil {
		return nil, err
	}

	batch, err := conn.PrepareBatch(ctx, query)
	if err != nil {
		c.logger.Error("Failed to prepare batch",
			zap.Error(err),
			zap.String("query", truncateQuery(query)))
		return nil, fmt.Errorf("prepare batch failed: %w", err)
	}

	return batch, nil
}

// BatchInsert 批量插入辅助函数
func (c *ClickHouseClient) BatchInsert(ctx context.Context, query string, appendFunc func(batch driver.Batch) error) error {
	ctx, span := otel.StartSpan(ctx, "clickhouse.batch_insert")
	defer span.End()

	batch, err := c.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}

	if err := appendFunc(batch); err != nil {
		return fmt.Errorf("append to batch failed: %w", err)
	}

	start := time.Now()
	if err := batch.Send(); err != nil {
		c.metrics.queryErrors.Inc()
		c.logger.Error("Batch send failed",
			zap.Error(err),
			zap.Duration("duration", time.Since(start)))
		return fmt.Errorf("batch send failed: %w", err)
	}

	c.metrics.batchInserts.Inc()
	c.logger.Debug("Batch insert completed",
		zap.Duration("duration", time.Since(start)))

	return nil
}

// Ping 测试连接
func (c *ClickHouseClient) Ping(ctx context.Context) error {
	conn, err := c.Conn()
	if err != nil {
		return err
	}
	return conn.Ping(ctx)
}

// Stats 获取连接统计
func (c *ClickHouseClient) Stats() driver.Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn != nil {
		return c.conn.Stats()
	}
	return driver.Stats{}
}

// Close 关闭连接
func (c *ClickHouseClient) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	c.logger.Info("Closing ClickHouse connection")

	// 停止重连和健康检查
	close(c.stopReconnect)
	close(c.stopHealth)

	if c.reconnectTimer != nil {
		c.reconnectTimer.Stop()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

func truncateQuery(query string) string {
	if len(query) > 200 {
		return query[:200] + "..."
	}
	return query
}

// ClickHouseHealthChecker 健康检查
type ClickHouseHealthChecker struct {
	client *ClickHouseClient
}

// NewClickHouseHealthChecker 创建健康检查器
func NewClickHouseHealthChecker(client *ClickHouseClient) *ClickHouseHealthChecker {
	return &ClickHouseHealthChecker{client: client}
}

// Check 执行健康检查
func (h *ClickHouseHealthChecker) Check(ctx context.Context) error {
	if h.client.GetState() != StateConnected {
		return fmt.Errorf("clickhouse not connected: %s", h.client.GetState().String())
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return h.client.Ping(ctx)
}

// Name 返回检查器名称
func (h *ClickHouseHealthChecker) Name() string {
	return "clickhouse"
}

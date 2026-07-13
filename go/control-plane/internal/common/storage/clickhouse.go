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

type ClickHouseConfig struct {
	// 集群模式 — 多 shard 端点列表 (每个 shard 至少一个端点)
	// 示例: "clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000"
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

	// 集群模式: 是否使用 Distributed 表读写
	// 为 true 时, 读写 Distributed 表 (如 flows_raw 而非 flows_raw_local)
	// 写入 Distributed 表会自动按 sharding key 分布到各 shard
	ClusterMode bool `env:"CLICKHOUSE_CLUSTER_MODE" envDefault:"true"`

	// 集群模式: Distributed 表后缀 (空=直接用表名, 如 flows_raw)
	// 如需使用 _local 表直连 shard, 设置为 Distributed 表名
	TableSuffix string `env:"CLICKHOUSE_TABLE_SUFFIX" envDefault:""`

	EnableAutoReconnect bool          `env:"CLICKHOUSE_AUTO_RECONNECT" envDefault:"true"`
	ReconnectInterval   time.Duration `env:"CLICKHOUSE_RECONNECT_INTERVAL" envDefault:"5s"`
	MaxReconnectDelay   time.Duration `env:"CLICKHOUSE_MAX_RECONNECT_DELAY" envDefault:"60s"`
	HealthCheckInterval time.Duration `env:"CLICKHOUSE_HEALTH_CHECK_INTERVAL" envDefault:"30s"`
}

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

type ConnectionCallback func(oldState, newState ConnectionState, err error)

type ClickHouseClient struct {
	conn   driver.Conn
	config ClickHouseConfig
	logger *zap.Logger

	state          int32
	mu             sync.RWMutex
	reconnectTimer *time.Timer
	stopReconnect  chan struct{}
	healthTicker   *time.Ticker
	stopHealth     chan struct{}

	callbacks []ConnectionCallback

	metrics *clickhouseMetrics

	closed int32
}

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

func NewClickHouseClient(cfg ClickHouseConfig, logger *zap.Logger) (*ClickHouseClient, error) {
	if len(cfg.Hosts) == 0 {
		return nil, fmt.Errorf("clickhouse hosts not configured")
	}

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

	if err := client.connect(); err != nil {
		if !cfg.EnableAutoReconnect {
			return nil, err
		}
		logger.Warn("Initial connection failed, will retry in background",
			zap.Error(err))
	}

	if cfg.EnableAutoReconnect {
		go client.reconnectLoop()
		go client.healthCheckLoop()
	}

	return client, nil
}

// NewClickHouseClientFromConn 从已有的 driver.Conn 创建 ClickHouseClient 包装器
func NewClickHouseClientFromConn(conn driver.Conn, logger *zap.Logger) *ClickHouseClient {
	client := &ClickHouseClient{
		conn:          conn,
		logger:        logger,
		stopReconnect: make(chan struct{}),
		stopHealth:    make(chan struct{}),
		metrics:       newClickHouseMetrics("clickhouse"),
	}
	atomic.StoreInt32(&client.state, int32(StateConnected))
	client.metrics.connectionState.Set(float64(StateConnected))
	return client
}

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
			"max_execution_time":                                 60,
			"distributed_product_mode":                           "allow",
			"prefer_localhost_replica":                           1,
			"load_balancing":                                     "random",
			"fallback_to_stale_replicas_for_distributed_queries": 1,
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

	ctx, cancel := context.WithTimeout(context.Background(), c.config.DialTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		c.setState(StateDisconnected)
		return fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	c.mu.Lock()

	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = conn
	c.mu.Unlock()

	c.setState(StateConnected)

	c.logger.Info("Connected to ClickHouse",
		zap.Strings("hosts", c.config.Hosts),
		zap.String("database", c.config.Database),
		zap.Bool("cluster_mode", c.config.ClusterMode))

	return nil
}

func (c *ClickHouseClient) reconnectLoop() {
	attempt := 0
	baseDelay := c.config.ReconnectInterval

	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			return
		}

		state := c.GetState()
		if state == StateConnected {

			attempt = 0
			time.Sleep(baseDelay)
			continue
		}

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

func (c *ClickHouseClient) setState(newState ConnectionState) {
	oldState := ConnectionState(atomic.SwapInt32(&c.state, int32(newState)))

	if oldState == newState {
		return
	}

	c.metrics.connectionState.Set(float64(newState))

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

func (c *ClickHouseClient) GetState() ConnectionState {
	return ConnectionState(atomic.LoadInt32(&c.state))
}

func (c *ClickHouseClient) OnStateChange(cb ConnectionCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = append(c.callbacks, cb)
}

func (c *ClickHouseClient) Conn(ctx context.Context) (driver.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, fmt.Errorf("client is closed")
	}

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
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
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, fmt.Errorf("timeout waiting for connection")
		case <-ticker.C:
			continue
		}
	}
}

func (c *ClickHouseClient) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	ctx, span := otel.StartSpan(ctx, "clickhouse.query")
	defer span.End()

	conn, err := c.Conn(ctx)
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
			zap.String("query", truncateClickhouseQuery(query)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return nil, fmt.Errorf("query failed: %w", err)
	}

	c.logger.Debug("ClickHouse query executed",
		zap.String("query", truncateClickhouseQuery(query)),
		zap.Duration("duration", duration))

	return rows, nil
}

func (c *ClickHouseClient) QueryRow(ctx context.Context, query string, args ...interface{}) (driver.Row, error) {
	ctx, span := otel.StartSpan(ctx, "clickhouse.query_row")
	defer span.End()

	conn, err := c.Conn(ctx)
	if err != nil {
		return nil, err
	}

	row := conn.QueryRow(ctx, query, args...)
	// 检查 row 本身的错误 (与 Query 不同, QueryRow 延迟返回错误)
	if err := row.Err(); err != nil {
		c.logger.Error("ClickHouse QueryRow failed",
			zap.String("query", truncateClickhouseQuery(query)),
			zap.Error(err))
		otel.RecordError(ctx, err)
		return nil, fmt.Errorf("query row failed: %w", err)
	}
	return row, nil
}

func (c *ClickHouseClient) Exec(ctx context.Context, query string, args ...interface{}) error {
	ctx, span := otel.StartSpan(ctx, "clickhouse.exec")
	defer span.End()

	conn, err := c.Conn(ctx)
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
			zap.String("query", truncateClickhouseQuery(query)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return fmt.Errorf("exec failed: %w", err)
	}

	c.logger.Debug("ClickHouse exec completed",
		zap.String("query", truncateClickhouseQuery(query)),
		zap.Duration("duration", duration))

	return nil
}

func (c *ClickHouseClient) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	ctx, span := otel.StartSpan(ctx, "clickhouse.prepare_batch")
	defer span.End()

	conn, err := c.Conn(ctx)
	if err != nil {
		return nil, err
	}

	batch, err := conn.PrepareBatch(ctx, query)
	if err != nil {
		c.logger.Error("Failed to prepare batch",
			zap.Error(err),
			zap.String("query", truncateClickhouseQuery(query)))
		return nil, fmt.Errorf("prepare batch failed: %w", err)
	}

	return batch, nil
}

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

func (c *ClickHouseClient) Ping(ctx context.Context) error {
	conn, err := c.Conn(ctx)
	if err != nil {
		return err
	}
	return conn.Ping(ctx)
}

func (c *ClickHouseClient) Stats() driver.Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn != nil {
		return c.conn.Stats()
	}
	return driver.Stats{}
}

func (c *ClickHouseClient) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	c.logger.Info("Closing ClickHouse connection")

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

func truncateClickhouseQuery(query string) string {
	if len(query) > 200 {
		return query[:200] + "..."
	}
	return query
}

type ClickHouseHealthChecker struct {
	client *ClickHouseClient
}

func NewClickHouseHealthChecker(client *ClickHouseClient) *ClickHouseHealthChecker {
	return &ClickHouseHealthChecker{client: client}
}

func (h *ClickHouseHealthChecker) Check(ctx context.Context) error {
	if h.client.GetState() != StateConnected {
		return fmt.Errorf("clickhouse not connected: %s", h.client.GetState().String())
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return h.client.Ping(ctx)
}

func (h *ClickHouseHealthChecker) Name() string {
	return "clickhouse"
}

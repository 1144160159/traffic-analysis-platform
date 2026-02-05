////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/storage/postgres.go
// 修复版本 v2：
// 1. 修复 #6：增加连接池监控指标（Prometheus）
// 2. 新增连接池健康检查方法
// 3. 增加慢查询日志记录
// 4. 优化事务处理
////////////////////////////////////////////////////////////////////////////////

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// PostgresConfig PostgreSQL配置
type PostgresConfig struct {
	Host            string        `env:"POSTGRES_HOST" envDefault:"localhost"`
	Port            int           `env:"POSTGRES_PORT" envDefault:"5432"`
	Database        string        `env:"POSTGRES_DATABASE" envDefault:"traffic"`
	Username        string        `env:"POSTGRES_USERNAME" envDefault:"postgres"`
	Password        string        `env:"POSTGRES_PASSWORD"`
	SSLMode         string        `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME" envDefault:"1h"`
	ConnMaxIdleTime time.Duration `env:"POSTGRES_CONN_MAX_IDLE_TIME" envDefault:"30m"`
	ConnectTimeout  int           `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10"`

	// 修复 #6：新增慢查询阈值配置
	SlowQueryThreshold time.Duration `env:"POSTGRES_SLOW_QUERY_THRESHOLD" envDefault:"1s"`
}

// DSN 生成连接字符串
func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode, c.ConnectTimeout,
	)
}

// 修复 #6：Prometheus 指标定义
var (
	postgresConnectionsInUse = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "postgres_connections_in_use",
			Help: "Number of PostgreSQL connections currently in use",
		},
		[]string{"database"},
	)

	postgresConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "postgres_connections_idle",
			Help: "Number of idle PostgreSQL connections",
		},
		[]string{"database"},
	)

	postgresConnectionsWaiting = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "postgres_connections_waiting",
			Help: "Number of connections waiting for a connection from the pool",
		},
		[]string{"database"},
	)

	postgresConnectionsMaxOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "postgres_connections_max_open",
			Help: "Maximum number of open PostgreSQL connections",
		},
		[]string{"database"},
	)

	postgresQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "postgres_query_duration_seconds",
			Help:    "PostgreSQL query duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"database", "operation"},
	)

	postgresSlowQueries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "postgres_slow_queries_total",
			Help: "Total number of slow PostgreSQL queries",
		},
		[]string{"database", "operation"},
	)

	postgresErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "postgres_errors_total",
			Help: "Total number of PostgreSQL errors",
		},
		[]string{"database", "operation", "error_type"},
	)
)

// PostgresClient PostgreSQL客户端（修复版）
type PostgresClient struct {
	db     *sql.DB
	config PostgresConfig
	logger *zap.Logger

	// 修复 #6：指标收集器
	metricsCollector *postgresMetricsCollector
}

// postgresMetricsCollector 指标收集器
type postgresMetricsCollector struct {
	client   *PostgresClient
	stopChan chan struct{}
}

// NewPostgresClient 创建PostgreSQL客户端（修复版）
func NewPostgresClient(cfg PostgresConfig, logger *zap.Logger) (*PostgresClient, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// 配置连接池
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ConnectTimeout)*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	logger.Info("Connected to PostgreSQL",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database))

	client := &PostgresClient{
		db:     db,
		config: cfg,
		logger: logger,
	}

	// 修复 #6：启动指标收集
	client.metricsCollector = &postgresMetricsCollector{
		client:   client,
		stopChan: make(chan struct{}),
	}
	go client.metricsCollector.start()

	return client, nil
}

// start 启动指标收集（修复 #6）
func (mc *postgresMetricsCollector) start() {
	ticker := time.NewTicker(10 * time.Second) // 每 10 秒采集一次
	defer ticker.Stop()

	for {
		select {
		case <-mc.stopChan:
			return
		case <-ticker.C:
			mc.collect()
		}
	}
}

// collect 采集指标（修复 #6）
func (mc *postgresMetricsCollector) collect() {
	stats := mc.client.db.Stats()

	dbName := mc.client.config.Database

	postgresConnectionsInUse.WithLabelValues(dbName).Set(float64(stats.InUse))
	postgresConnectionsIdle.WithLabelValues(dbName).Set(float64(stats.Idle))
	postgresConnectionsWaiting.WithLabelValues(dbName).Set(float64(stats.WaitCount))
	postgresConnectionsMaxOpen.WithLabelValues(dbName).Set(float64(stats.MaxOpenConnections))
}

// stop 停止指标收集（修复 #6）
func (mc *postgresMetricsCollector) stop() {
	close(mc.stopChan)
}

// DB 获取原生数据库连接
func (c *PostgresClient) DB() *sql.DB {
	return c.db
}

// Query 执行查询（修复版：增加慢查询日志）
func (c *PostgresClient) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	ctx, span := otel.StartSpan(ctx, "postgres.query")
	defer span.End()

	start := time.Now()
	rows, err := c.db.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	// 记录指标
	postgresQueryDuration.WithLabelValues(c.config.Database, "query").Observe(duration.Seconds())

	// 修复 #6：记录慢查询
	if duration > c.config.SlowQueryThreshold {
		postgresSlowQueries.WithLabelValues(c.config.Database, "query").Inc()
		c.logger.Warn("Slow query detected",
			zap.String("query", truncatePostgresQuery(query)),
			zap.Duration("duration", duration),
			zap.Duration("threshold", c.config.SlowQueryThreshold))
	}

	if err != nil {
		postgresErrors.WithLabelValues(c.config.Database, "query", categorizePostgresError(err)).Inc()
		c.logger.Error("PostgreSQL query failed",
			zap.Error(err),
			zap.String("query", truncatePostgresQuery(query)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return nil, fmt.Errorf("query failed: %w", err)
	}

	c.logger.Debug("PostgreSQL query executed",
		zap.String("query", truncatePostgresQuery(query)),
		zap.Duration("duration", duration))

	return rows, nil
}

// QueryRow 执行单行查询
func (c *PostgresClient) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	ctx, span := otel.StartSpan(ctx, "postgres.query_row")
	defer span.End()

	start := time.Now()
	row := c.db.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)

	// 记录指标
	postgresQueryDuration.WithLabelValues(c.config.Database, "query_row").Observe(duration.Seconds())

	// 记录慢查询
	if duration > c.config.SlowQueryThreshold {
		postgresSlowQueries.WithLabelValues(c.config.Database, "query_row").Inc()
		c.logger.Warn("Slow query_row detected",
			zap.String("query", truncatePostgresQuery(query)),
			zap.Duration("duration", duration))
	}

	return row
}

// Exec 执行命令（修复版：增加慢查询日志）
func (c *PostgresClient) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	ctx, span := otel.StartSpan(ctx, "postgres.exec")
	defer span.End()

	start := time.Now()
	result, err := c.db.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	// 记录指标
	postgresQueryDuration.WithLabelValues(c.config.Database, "exec").Observe(duration.Seconds())

	// 记录慢查询
	if duration > c.config.SlowQueryThreshold {
		postgresSlowQueries.WithLabelValues(c.config.Database, "exec").Inc()
		c.logger.Warn("Slow exec detected",
			zap.String("query", truncatePostgresQuery(query)),
			zap.Duration("duration", duration))
	}

	if err != nil {
		postgresErrors.WithLabelValues(c.config.Database, "exec", categorizePostgresError(err)).Inc()
		c.logger.Error("PostgreSQL exec failed",
			zap.Error(err),
			zap.String("query", truncatePostgresQuery(query)),
			zap.Duration("duration", duration))
		otel.RecordError(ctx, err)
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	c.logger.Debug("PostgreSQL exec completed",
		zap.String("query", truncatePostgresQuery(query)),
		zap.Duration("duration", duration))

	return result, nil
}

// BeginTx 开始事务
func (c *PostgresClient) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	ctx, span := otel.StartSpan(ctx, "postgres.begin_tx")
	defer span.End()

	tx, err := c.db.BeginTx(ctx, opts)
	if err != nil {
		postgresErrors.WithLabelValues(c.config.Database, "begin_tx", categorizePostgresError(err)).Inc()
		c.logger.Error("Failed to begin transaction", zap.Error(err))
		return nil, fmt.Errorf("begin transaction failed: %w", err)
	}

	return tx, nil
}

// Transaction 事务辅助函数（修复版：优化错误处理）
func (c *PostgresClient) Transaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	ctx, span := otel.StartSpan(ctx, "postgres.transaction")
	defer span.End()

	start := time.Now()

	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// 确保事务被正确处理
	defer func() {
		if p := recover(); p != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				c.logger.Error("Failed to rollback transaction after panic",
					zap.Error(rbErr),
					zap.Any("panic", p))
			}
			panic(p) // 重新抛出 panic
		}
	}()

	// 执行业务逻辑
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			postgresErrors.WithLabelValues(c.config.Database, "rollback", categorizePostgresError(rbErr)).Inc()
			c.logger.Error("Failed to rollback transaction",
				zap.Error(rbErr),
				zap.NamedError("original_error", err))
		}
		return err
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		postgresErrors.WithLabelValues(c.config.Database, "commit", categorizePostgresError(err)).Inc()
		c.logger.Error("Failed to commit transaction", zap.Error(err))
		return fmt.Errorf("commit transaction failed: %w", err)
	}

	duration := time.Since(start)
	postgresQueryDuration.WithLabelValues(c.config.Database, "transaction").Observe(duration.Seconds())

	c.logger.Debug("Transaction completed",
		zap.Duration("duration", duration))

	return nil
}

// Ping 测试连接
func (c *PostgresClient) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Stats 获取连接统计
func (c *PostgresClient) Stats() sql.DBStats {
	return c.db.Stats()
}

// GetPoolHealth 获取连接池健康状态（修复 #6：新增）
func (c *PostgresClient) GetPoolHealth() PoolHealth {
	stats := c.db.Stats()

	return PoolHealth{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
	}
}

// PoolHealth 连接池健康状态（修复 #6：新增）
type PoolHealth struct {
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration"`
	MaxIdleClosed      int64         `json:"max_idle_closed"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed"`
	MaxIdleTimeClosed  int64         `json:"max_idle_time_closed"`
}

// IsHealthy 检查连接池是否健康（修复 #6：新增）
func (h *PoolHealth) IsHealthy() bool {
	// 检查是否有可用连接
	if h.Idle == 0 && h.InUse >= h.MaxOpenConnections {
		return false
	}

	// 检查等待队列是否过长（超过 100 个等待）
	if h.WaitCount > 100 {
		return false
	}

	return true
}

// Close 关闭连接（修复版：停止指标收集）
func (c *PostgresClient) Close() error {
	c.logger.Info("Closing PostgreSQL connection")

	// 修复 #6：停止指标收集
	if c.metricsCollector != nil {
		c.metricsCollector.stop()
	}

	return c.db.Close()
}

// PostgresHealthChecker 健康检查
type PostgresHealthChecker struct {
	client *PostgresClient
}

// NewPostgresHealthChecker 创建健康检查器
func NewPostgresHealthChecker(client *PostgresClient) *PostgresHealthChecker {
	return &PostgresHealthChecker{client: client}
}

// Check 执行健康检查（修复版：增加连接池检查）
func (h *PostgresHealthChecker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 检查连接
	if err := h.client.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// 修复 #6：检查连接池健康状态
	poolHealth := h.client.GetPoolHealth()
	if !poolHealth.IsHealthy() {
		return fmt.Errorf("connection pool unhealthy: in_use=%d, idle=%d, wait_count=%d",
			poolHealth.InUse, poolHealth.Idle, poolHealth.WaitCount)
	}

	return nil
}

// Name 返回检查器名称
func (h *PostgresHealthChecker) Name() string {
	return "postgres"
}

// PostgresMigrator 数据库迁移器
type PostgresMigrator struct {
	client *PostgresClient
	logger *zap.Logger
}

// NewPostgresMigrator 创建迁移器
func NewPostgresMigrator(client *PostgresClient, logger *zap.Logger) *PostgresMigrator {
	return &PostgresMigrator{
		client: client,
		logger: logger,
	}
}

// EnsureMigrationTable 确保迁移表存在
func (m *PostgresMigrator) EnsureMigrationTable(ctx context.Context) error {
	query := `
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version VARCHAR(255) PRIMARY KEY,
            applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
    `
	_, err := m.client.Exec(ctx, query)
	return err
}

// IsApplied 检查迁移是否已应用
func (m *PostgresMigrator) IsApplied(ctx context.Context, version string) (bool, error) {
	var count int
	err := m.client.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", version).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// MarkApplied 标记迁移已应用
func (m *PostgresMigrator) MarkApplied(ctx context.Context, version string) error {
	_, err := m.client.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
	return err
}

// Migrate 执行迁移
func (m *PostgresMigrator) Migrate(ctx context.Context, version string, query string) error {
	applied, err := m.IsApplied(ctx, version)
	if err != nil {
		return fmt.Errorf("check migration status failed: %w", err)
	}

	if applied {
		m.logger.Debug("Migration already applied", zap.String("version", version))
		return nil
	}

	m.logger.Info("Applying migration", zap.String("version", version))

	if err := m.client.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	m.logger.Info("Migration applied", zap.String("version", version))
	return nil
}

// truncatePostgresQuery 截断查询字符串
func truncatePostgresQuery(query string) string {
	if len(query) > 200 {
		return query[:200] + "..."
	}
	return query
}

// categorizePostgresError 分类 PostgreSQL 错误（修复 #6：新增）
func categorizePostgresError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()

	switch {
	case contains(errStr, "connection refused"):
		return "connection_refused"
	case contains(errStr, "timeout"):
		return "timeout"
	case contains(errStr, "deadlock"):
		return "deadlock"
	case contains(errStr, "duplicate key"):
		return "duplicate_key"
	case contains(errStr, "foreign key"):
		return "foreign_key_violation"
	case contains(errStr, "syntax error"):
		return "syntax_error"
	case contains(errStr, "does not exist"):
		return "not_found"
	default:
		return "other"
	}
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsIgnoreCase(s, substr)
}

// containsIgnoreCase 不区分大小写的子串查找
func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)

	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// toLower 转换为小写
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

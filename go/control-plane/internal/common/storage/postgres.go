package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

type PostgresConfig struct {
	// 主库 (读写) — 必填
	Host     string `env:"POSTGRES_HOST" envDefault:"postgres-primary.databases.svc"`
	Port     int    `env:"POSTGRES_PORT" envDefault:"5432"`
	Database string `env:"POSTGRES_DATABASE" envDefault:"traffic_platform"`

	// 只读副本 (可选, 多个副本逗号分隔)
	// 示例: "postgres-replica.databases.svc,postgres-replica-1.databases.svc"
	ReplicaHosts string `env:"POSTGRES_REPLICA_HOSTS"`

	Username        string        `env:"POSTGRES_USERNAME" envDefault:"postgres"`
	Password        string        `env:"POSTGRES_PASSWORD"`
	SSLMode         string        `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME" envDefault:"1h"`
	ConnMaxIdleTime time.Duration `env:"POSTGRES_CONN_MAX_IDLE_TIME" envDefault:"30m"`
	ConnectTimeout  int           `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10"`

	SlowQueryThreshold time.Duration `env:"POSTGRES_SLOW_QUERY_THRESHOLD" envDefault:"1s"`
}

// PrimaryDSN 返回主库 (读写) DSN
func (c PostgresConfig) PrimaryDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode, c.ConnectTimeout,
	)
}

// ReplicaDSN 返回只读副本 DSN (如果配置了副本)
// 未配置副本时回退到主库
func (c PostgresConfig) ReplicaDSN() string {
	if c.ReplicaHosts != "" {
		// 取第一个副本
		hosts := splitHosts(c.ReplicaHosts)
		if len(hosts) > 0 {
			return fmt.Sprintf(
				"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
				hosts[0], c.Port, c.Username, c.Password, c.Database, c.SSLMode, c.ConnectTimeout,
			)
		}
	}
	// 回退到主库
	return c.PrimaryDSN()
}

func splitHosts(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// DSN 保留向后兼容 (等同于 PrimaryDSN)
func (c PostgresConfig) DSN() string {
	return c.PrimaryDSN()
}

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

type PostgresClient struct {
	db     *sql.DB
	config PostgresConfig
	logger *zap.Logger

	metricsCollector *postgresMetricsCollector
}

type postgresMetricsCollector struct {
	client   *PostgresClient
	stopChan chan struct{}
}

func NewPostgresClient(cfg PostgresConfig, logger *zap.Logger) (*PostgresClient, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

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

	client.metricsCollector = &postgresMetricsCollector{
		client:   client,
		stopChan: make(chan struct{}),
	}
	go client.metricsCollector.start()

	return client, nil
}

// NewPostgresClientFromDB 从已有的 *sql.DB 创建 PostgresClient 包装器
func NewPostgresClientFromDB(db *sql.DB, logger *zap.Logger) *PostgresClient {
	return &PostgresClient{
		db:     db,
		logger: logger,
	}
}

func (mc *postgresMetricsCollector) start() {
	ticker := time.NewTicker(10 * time.Second)
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

func (mc *postgresMetricsCollector) collect() {
	stats := mc.client.db.Stats()

	dbName := mc.client.config.Database

	postgresConnectionsInUse.WithLabelValues(dbName).Set(float64(stats.InUse))
	postgresConnectionsIdle.WithLabelValues(dbName).Set(float64(stats.Idle))
	postgresConnectionsWaiting.WithLabelValues(dbName).Set(float64(stats.WaitCount))
	postgresConnectionsMaxOpen.WithLabelValues(dbName).Set(float64(stats.MaxOpenConnections))
}

func (mc *postgresMetricsCollector) stop() {
	close(mc.stopChan)
}

func (c *PostgresClient) DB() *sql.DB {
	return c.db
}

func (c *PostgresClient) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	ctx, span := otel.StartSpan(ctx, "postgres.query")
	defer span.End()

	start := time.Now()
	rows, err := c.db.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	postgresQueryDuration.WithLabelValues(c.config.Database, "query").Observe(duration.Seconds())

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

func (c *PostgresClient) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	ctx, span := otel.StartSpan(ctx, "postgres.query_row")
	defer span.End()

	start := time.Now()
	row := c.db.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)

	postgresQueryDuration.WithLabelValues(c.config.Database, "query_row").Observe(duration.Seconds())

	if duration > c.config.SlowQueryThreshold {
		postgresSlowQueries.WithLabelValues(c.config.Database, "query_row").Inc()
		c.logger.Warn("Slow query_row detected",
			zap.String("query", truncatePostgresQuery(query)),
			zap.Duration("duration", duration))
	}

	return row
}

func (c *PostgresClient) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	ctx, span := otel.StartSpan(ctx, "postgres.exec")
	defer span.End()

	start := time.Now()
	result, err := c.db.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	postgresQueryDuration.WithLabelValues(c.config.Database, "exec").Observe(duration.Seconds())

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

func (c *PostgresClient) Transaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	ctx, span := otel.StartSpan(ctx, "postgres.transaction")
	defer span.End()

	start := time.Now()

	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				c.logger.Error("Failed to rollback transaction after panic",
					zap.Error(rbErr),
					zap.Any("panic", p))
			}
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			postgresErrors.WithLabelValues(c.config.Database, "rollback", categorizePostgresError(rbErr)).Inc()
			c.logger.Error("Failed to rollback transaction",
				zap.Error(rbErr),
				zap.NamedError("original_error", err))
		}
		return err
	}

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

func (c *PostgresClient) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *PostgresClient) Stats() sql.DBStats {
	return c.db.Stats()
}

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

func (h *PoolHealth) IsHealthy() bool {

	if h.Idle == 0 && h.InUse >= h.MaxOpenConnections {
		return false
	}

	if h.WaitCount > 100 {
		return false
	}

	return true
}

func (c *PostgresClient) Close() error {
	c.logger.Info("Closing PostgreSQL connection")

	if c.metricsCollector != nil {
		c.metricsCollector.stop()
	}

	return c.db.Close()
}

type PostgresHealthChecker struct {
	client *PostgresClient
}

func NewPostgresHealthChecker(client *PostgresClient) *PostgresHealthChecker {
	return &PostgresHealthChecker{client: client}
}

func (h *PostgresHealthChecker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := h.client.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	poolHealth := h.client.GetPoolHealth()
	if !poolHealth.IsHealthy() {
		return fmt.Errorf("connection pool unhealthy: in_use=%d, idle=%d, wait_count=%d",
			poolHealth.InUse, poolHealth.Idle, poolHealth.WaitCount)
	}

	return nil
}

func (h *PostgresHealthChecker) Name() string {
	return "postgres"
}

type PostgresMigrator struct {
	client *PostgresClient
	logger *zap.Logger
}

func NewPostgresMigrator(client *PostgresClient, logger *zap.Logger) *PostgresMigrator {
	return &PostgresMigrator{
		client: client,
		logger: logger,
	}
}

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

func (m *PostgresMigrator) IsApplied(ctx context.Context, version string) (bool, error) {
	var count int
	err := m.client.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", version).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *PostgresMigrator) MarkApplied(ctx context.Context, version string) error {
	_, err := m.client.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
	return err
}

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

func truncatePostgresQuery(query string) string {
	if len(query) > 200 {
		return query[:200] + "..."
	}
	return query
}

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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsIgnoreCase(s, substr)
}

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

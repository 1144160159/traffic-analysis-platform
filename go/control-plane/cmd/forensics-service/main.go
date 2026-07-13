////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/cmd/forensics-service/main.go
// 完整版：修复配置加载、Redis 客户端创建、Auth 中间件集成
////////////////////////////////////////////////////////////////////////////////

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	authJwt "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	authMiddleware "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	authRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	authService "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/cutter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/task"
)

func main() {
	// 初始化日志
	logCfg := logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Format:      getEnv("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     "forensics-service",
		Version:     getEnv("SERVICE_VERSION", "1.0.0"),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logging.Sync(logger)

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid config", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("listen_addr", cfg.API.ListenAddr),
		zap.Bool("auth_enabled", cfg.Auth.Enabled),
		zap.Strings("clickhouse_hosts", cfg.ClickHouse.Hosts),
		zap.String("s3_endpoint", cfg.S3.Endpoint))

	// 初始化 PostgreSQL（用于任务状态存储）
	pgDB, err := initPostgreSQL(cfg.PostgreSQL, logger)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer pgDB.Close()
	logger.Info("Connected to PostgreSQL")

	// 创建 PostgresClient 包装
	pgClient := storage.NewPostgresClientFromDB(pgDB, logger)

	// 初始化 ClickHouse（用于索引查询）
	rawCHConn, err := initClickHouse(cfg.ClickHouse, logger)
	if err != nil {
		logger.Fatal("Failed to connect to ClickHouse", zap.Error(err))
	}
	defer rawCHConn.Close()
	logger.Info("Connected to ClickHouse")

	// 创建 ClickHouseClient 包装
	chClient := storage.NewClickHouseClientFromConn(rawCHConn, logger)

	// 初始化 Redis（可选，用于缓存和 Auth）
	var rdb *redis.Client
	if cfg.Redis.Addr != "" || len(cfg.Redis.ClusterAddrs) > 0 || cfg.Redis.IsSentinelMode() {
		rdb, err = initRedis(cfg.Redis, logger)
		if err != nil {
			logger.Warn("Failed to connect to Redis, caching disabled", zap.Error(err))
		} else {
			defer rdb.Close()
			logger.Info("Connected to Redis")
		}
	}

	// 初始化 Index Client
	indexClient := index.NewIndexClient(chClient, logger)

	// 初始化 S3 Client
	s3Client, err := s3client.NewS3Client(
		cfg.S3.Endpoint,
		cfg.S3.AccessKey,
		cfg.S3.SecretKey,
		cfg.S3.Bucket,
		cfg.S3.UseSSL,
		cfg.S3.ResultBucket,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to create S3 client", zap.Error(err))
	}

	// 确保结果 bucket 存在
	ctx := context.Background()
	if err := s3Client.EnsureBucket(ctx, cfg.S3.ResultBucket); err != nil {
		logger.Warn("Failed to ensure result bucket", zap.Error(err))
	}

	// 初始化 PCAP Cutter
	pcapCutter := cutter.NewCutter(
		s3Client,
		indexClient,
		cfg.Cutter.MaxConcurrent,
		cfg.Cutter.MaxPackets,
		cfg.Cutter.PerFileTimeout,
		logger,
	)

	// 初始化 Task Repository
	taskRepo := repository.NewTaskRepository(pgClient, logger)

	// 初始化审计日志
	var auditLogger *audit.Logger
	if len(cfg.Kafka.Brokers) > 0 {
		auditCfg := audit.Config{
			KafkaBrokers:  cfg.Kafka.Brokers,
			Topic:         cfg.Kafka.AuditTopic,
			ServiceName:   "forensics-service",
			BufferSize:    1000,
			BatchSize:     100,
			FlushInterval: time.Second,
			Security:      cfg.KafkaSecurity,
		}
		auditLogger, err = audit.NewLogger(auditCfg, logger)
		if err != nil {
			logger.Warn("Failed to create audit logger", zap.Error(err))
		} else {
			defer auditLogger.Close()
			logger.Info("Audit logger initialized")
		}
	}

	// 初始化异步任务处理器
	asyncCutter := task.NewAsyncCutter(
		pcapCutter,
		s3Client,
		taskRepo,
		cfg.Task.WorkerCount,
		cfg.Task.QueueSize,
		cfg.Task.ResultExpiry,
		logger,
	)

	// 启动后台任务处理器
	taskCtx, taskCancel := context.WithCancel(context.Background())
	defer taskCancel()
	asyncCutter.Start(taskCtx)
	logger.Info("Async task processor started",
		zap.Int("workers", cfg.Task.WorkerCount))

	// 初始化 API Handler
	handler := api.NewHandler(
		pcapCutter,
		asyncCutter,
		s3Client,
		taskRepo,
		auditLogger,
		logger,
	)
	handler.SetAuditDB(pgDB)

	// 创建 Router
	r := mux.NewRouter()

	// 注册路由。Handler 内部已经声明 /api/v1 前缀，入口层只传根路由。
	handler.RegisterRoutes(r)

	// 构建中间件链
	middlewareChain := buildMiddlewareChain(cfg, logger, pgDB, rdb)

	// 应用中间件
	finalHandler := middlewareChain.Then(r)

	// HTTP Server
	srv := &http.Server{
		Addr:         cfg.API.ListenAddr,
		Handler:      finalHandler,
		ReadTimeout:  cfg.API.ReadTimeout,
		WriteTimeout: cfg.API.WriteTimeout,
		IdleTimeout:  cfg.API.IdleTimeout,
	}

	// 启动服务器
	go func() {
		logger.Info("Starting Forensics API server",
			zap.String("addr", cfg.API.ListenAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// 等待关闭信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	logger.Info("Shutting down gracefully...")

	// 停止接收新任务
	asyncCutter.Stop()

	// 优雅关闭 HTTP 服务器
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	// 关闭审计日志
	if auditLogger != nil {
		if err := auditLogger.Close(); err != nil {
			logger.Error("Failed to close audit logger", zap.Error(err))
		}
	}

	logger.Info("Shutdown complete")
}

// initPostgreSQL 初始化 PostgreSQL 连接
func initPostgreSQL(cfg config.PostgreSQLConfig, logger *zap.Logger) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ConnectTimeout)*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// initClickHouse 初始化 ClickHouse 连接
func initClickHouse(cfg config.ClickHouseConfig, logger *zap.Logger) (clickhouse.Conn, error) {
	options := &clickhouse.Options{
		Addr: cfg.Hosts,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     cfg.DialTimeout,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		Debug:           cfg.Debug,
	}

	if cfg.CompressionLZ4 {
		options.Compression = &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		}
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	return conn, nil
}

// initRedis 初始化 Redis 连接
func initRedis(cfg config.RedisConfig, logger *zap.Logger) (*redis.Client, error) {
	var client *redis.Client

	if cfg.IsClusterMode() {
		// 集群模式 - 这里简化处理，实际应使用 ClusterClient
		logger.Warn("Cluster mode detected, using first address as standalone")
		client = redis.NewClient(&redis.Options{
			Addr:            cfg.ClusterAddrs[0],
			Password:        cfg.Password,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})
	} else if cfg.IsSentinelMode() {
		// 哨兵模式
		client = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:      cfg.SentinelMaster,
			SentinelAddrs:   cfg.SentinelAddrs,
			Password:        cfg.Password,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})
	} else {
		// 单机模式
		addr := cfg.Addr
		if addr == "" {
			addr = "localhost:6379"
		}
		client = redis.NewClient(&redis.Options{
			Addr:            addr,
			Password:        cfg.Password,
			DB:              cfg.DB,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return client, nil
}

// buildMiddlewareChain 构建中间件链
func buildMiddlewareChain(cfg *config.Config, logger *zap.Logger, db *sql.DB, rdb *redis.Client) *httpx.Chain {
	corsConfig := &httpx.CORSConfig{
		AllowedOrigins:   cfg.API.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-User-ID", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID", "Content-Disposition", "X-Content-SHA256"},
		AllowCredentials: true,
		MaxAge:           86400,
	}

	chain := httpx.NewChain(
		httpx.Recovery(logger),
		httpx.RequestID(),
		httpx.Logging(logger),
		httpx.CORS(corsConfig),
		httpx.Metrics("forensics-service"),
		httpx.TimeoutWithConfig(300, nil), // 5分钟超时
		httpx.TenantExtractor(),
	)

	// 如果启用认证，添加 Auth 中间件
	if cfg.Auth.Enabled && db != nil && rdb != nil {
		authMw := buildAuthMiddleware(cfg.Auth, logger, db, rdb)
		if authMw != nil {
			chain = chain.Append(authMw.Authenticate)
			logger.Info("Auth middleware enabled")
		}
	} else {
		logger.Warn("Auth middleware disabled - API endpoints are not protected")
	}

	return chain
}

// buildAuthMiddleware 构建认证中间件
func buildAuthMiddleware(cfg config.AuthConfig, logger *zap.Logger, db *sql.DB, rdb *redis.Client) *authMiddleware.AuthMiddleware {
	// JWT 配置
	jwtCfg := authJwt.Config{
		SigningKey:      cfg.JWTSigningKey,
		SigningMethod:   cfg.JWTSigningMethod,
		AccessTokenTTL:  cfg.AccessTokenTTL,
		RefreshTokenTTL: cfg.RefreshTokenTTL,
		Issuer:          cfg.JWTIssuer,
	}

	// 创建依赖
	userRepo := authRepository.NewUserRepository(db, logger)
	tokenRepo := authRepository.NewTokenRepository(db, logger)
	jwtSvc, jwtErr := authJwt.NewService(jwtCfg, storage.NewRedisClientFromExisting(rdb, logger), tokenRepo, logger)
	if jwtErr != nil {
		logger.Fatal("Failed to init JWT", zap.Error(jwtErr))
	}
	authSvc := authService.NewAuthService(userRepo, jwtSvc, nil, nil, logger, nil)

	return authMiddleware.NewAuthMiddleware(authSvc, logger)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ============================================================================

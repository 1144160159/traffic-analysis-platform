////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/cmd/graph-service/main.go
// Graph Service 主入口（完整修复版）
// 修复内容：
// 1. 修复 G1/G2/G3：集成 QueryLogger/SlowQueryDetector/TenantConfigLoader
// 2. 修复 G4：添加监控 Goroutine 的空指针检查
// 3. 修复 W1：初始化 WarmupService 时注入 GraphQuery
// 4. 完善优雅关闭逻辑
////////////////////////////////////////////////////////////////////////////////

package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	authConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/config"
	authjwt "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/oidc"
	authrepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	authservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/utils"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/cache"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphlogging "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/metrics"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/monitoring"
	graphnebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
	graphprojection "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/projection"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

type tokenValidatorAdapter struct {
	authService *authservice.AuthService
}

func (a tokenValidatorAdapter) ValidateToken(tokenString string) (httpx.Claims, error) {
	return a.authService.ValidateToken(tokenString)
}

func main() {
	// 初始化日志
	logCfg := logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Format:      getEnv("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     "graph-service",
		Version:     getEnv("SERVICE_VERSION", "1.0.0"),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logging.Sync(logger)

	logger.Info("Starting Graph Service",
		zap.String("version", logCfg.Version),
		zap.String("environment", logCfg.Environment))

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Configuration loaded", zap.Any("summary", cfg.GetConfigSummary()))

	// 创建 Context（用于优雅关闭）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// WaitGroup 用于跟踪所有 goroutine
	var wg sync.WaitGroup

	// ==================== 初始化 OpenTelemetry ====================

	var tracerProvider *otel.TracerProvider
	var meterProvider *otel.MeterProvider

	if cfg.OTEL.Enabled {
		tracerCfg := otel.TracerConfig{
			ServiceName:    cfg.OTEL.ServiceName,
			ServiceVersion: cfg.OTEL.ServiceVersion,
			Environment:    cfg.OTEL.Environment,
			Endpoint:       cfg.OTEL.Endpoint,
			Insecure:       cfg.OTEL.Insecure,
			SampleRate:     cfg.OTEL.SampleRate,
			Enabled:        cfg.OTEL.Enabled,
		}
		tracerProvider, err = otel.NewTracerProvider(tracerCfg)
		if err != nil {
			logger.Fatal("Failed to initialize tracer", zap.Error(err))
		}
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
				logger.Error("Failed to shutdown tracer", zap.Error(err))
			}
		}()

		meterCfg := otel.MeterConfig{
			ServiceName:    cfg.OTEL.ServiceName,
			ServiceVersion: cfg.OTEL.ServiceVersion,
			Environment:    cfg.OTEL.Environment,
			Endpoint:       cfg.OTEL.Endpoint,
			Insecure:       cfg.OTEL.Insecure,
			Enabled:        cfg.OTEL.Enabled,
		}
		meterProvider, err = otel.NewMeterProvider(meterCfg)
		if err != nil {
			logger.Fatal("Failed to initialize meter", zap.Error(err))
		}
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := meterProvider.Shutdown(shutdownCtx); err != nil {
				logger.Error("Failed to shutdown meter", zap.Error(err))
			}
		}()

		logger.Info("OpenTelemetry initialized",
			zap.String("endpoint", cfg.OTEL.Endpoint),
			zap.Float64("sample_rate", cfg.OTEL.SampleRate))
	}

	// ==================== 初始化数据库连接 ====================

	// PostgreSQL（修复 G2）
	pgDSN := postgresDSNFromEnv()
	pgDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer pgDB.Close()

	pgDB.SetMaxOpenConns(20)
	pgDB.SetMaxIdleConns(5)
	pgDB.SetConnMaxLifetime(time.Hour)

	if err := pgDB.Ping(); err != nil {
		logger.Fatal("Failed to ping PostgreSQL", zap.Error(err))
	}
	logger.Info("Connected to PostgreSQL")

	// ClickHouse
	chClient, err := storage.NewClickHouseClient(cfg.ClickHouse, logger)
	if err != nil {
		logger.Fatal("Failed to connect to ClickHouse", zap.Error(err))
	}
	defer chClient.Close()
	logger.Info("Connected to ClickHouse",
		zap.Strings("hosts", cfg.ClickHouse.Hosts),
		zap.String("database", cfg.ClickHouse.Database))

	// ClickHouse Circuit Breaker
	chCircuitBreaker := utils.NewCircuitBreaker(utils.DatabaseCircuitBreakerConfig("clickhouse"))
	logger.Info("ClickHouse Circuit Breaker initialized")

	// ==================== 初始化 Redis（可选）====================

	var redisClient *storage.RedisClient
	var graphCache *cache.GraphCache

	if cfg.Redis.IsConfigured() {
		redisClient, err = storage.NewRedisClient(cfg.Redis.ToStorageConfig(), logger)
		if err != nil {
			logger.Warn("Failed to connect to Redis, caching disabled", zap.Error(err))
		} else {
			defer redisClient.Close()
			logger.Info("Connected to Redis")

			if cfg.Cache.Enabled {
				graphCache = cache.NewGraphCache(
					redisClient,
					cfg.Cache.NeighborTTL,
					cfg.Cache.EntityTTL,
					cfg.Cache.GraphTTL,
					cfg.Cache.MaxNodesPerItem,
					cfg.Cache.MaxEdgesPerItem,
					logger,
				)
				logger.Info("Graph cache enabled",
					zap.Duration("neighbor_ttl", cfg.Cache.NeighborTTL),
					zap.Duration("entity_ttl", cfg.Cache.EntityTTL),
					zap.Duration("graph_ttl", cfg.Cache.GraphTTL))
			}
		}
	} else {
		logger.Info("Redis not configured, caching disabled")
	}

	// ==================== 初始化审计日志 ====================

	var auditLogger *audit.Logger
	if cfg.Audit.Enabled && len(cfg.Kafka.Brokers) > 0 {
		auditCfg := audit.Config{
			KafkaBrokers:  cfg.Kafka.Brokers,
			Topic:         cfg.Audit.Topic,
			ServiceName:   cfg.OTEL.ServiceName,
			BufferSize:    cfg.Audit.BufferSize,
			BatchSize:     cfg.Audit.BatchSize,
			FlushInterval: cfg.Audit.FlushInterval,
			BackupEnabled: cfg.Audit.BackupEnabled,
			BackupDir:     cfg.Audit.BackupDir,
			Security:      cfg.KafkaSecurity,
		}
		auditLogger, err = audit.NewLogger(auditCfg, logger)
		if err != nil {
			logger.Warn("Failed to initialize audit logger", zap.Error(err))
		} else {
			defer func() {
				if err := auditLogger.Close(); err != nil {
					logger.Error("Failed to close audit logger", zap.Error(err))
				}
			}()
			logger.Info("Audit logger initialized",
				zap.String("topic", cfg.Audit.Topic),
				zap.Bool("backup_enabled", cfg.Audit.BackupEnabled))
		}
	}

	// ==================== 初始化速率限流器 ====================

	var rateLimiter *storage.SlidingWindowRateLimiter
	if cfg.Security.RateLimitEnabled && redisClient != nil {
		rateLimiter = storage.NewSlidingWindowRateLimiter(redisClient)
		logger.Info("Rate limiter enabled",
			zap.Int("rps", cfg.Security.RateLimitRPS),
			zap.Int("window_sec", cfg.Security.RateLimitWindowSec))
	}

	// ==================== 初始化业务指标 ====================

	graphMetrics := metrics.NewGraphMetrics(cfg.OTEL.ServiceName)
	_ = graphMetrics // 使用变量避免警告

	// ==================== 修复 G2：初始化租户配置加载器 ====================

	tenantConfigLoader := config.NewTenantConfigLoader(pgDB, cfg, logger)
	defer tenantConfigLoader.Close()
	logger.Info("Tenant config loader initialized")

	// ==================== 修复 G1：初始化查询日志记录器 ====================

	queryLogger := graphlogging.NewQueryLogger(chClient, logger)
	defer queryLogger.Close()
	logger.Info("Query logger initialized")

	// ==================== 修复 G1：初始化慢查询检测器 ====================

	slowQueryThreshold := 5 * time.Second
	if thresholdEnv := getEnv("SLOW_QUERY_THRESHOLD", ""); thresholdEnv != "" {
		if duration, err := time.ParseDuration(thresholdEnv); err == nil {
			slowQueryThreshold = duration
		}
	}
	slowQueryDetector := monitoring.NewSlowQueryDetector(slowQueryThreshold, chClient, logger)
	logger.Info("Slow query detector initialized",
		zap.Duration("threshold", slowQueryThreshold))

	// ==================== 初始化图查询引擎 ====================

	graphQuery := query.NewGraphQueryWithCircuitBreaker(
		chClient,
		graphCache,
		cfg.Query,
		chCircuitBreaker,
		logger,
	)
	var workbenchStore *graphnebula.WorkbenchStore
	if cfg.Nebula.Enabled {
		var nebulaErr error
		workbenchStore, nebulaErr = graphnebula.NewWorkbenchStore(cfg.Nebula, logger)
		if nebulaErr != nil {
			logger.Fatal("Failed to initialize NebulaGraph workbench store", zap.Error(nebulaErr))
		}
		defer workbenchStore.Close()
		graphQuery.SetWorkbenchStore(workbenchStore)
		logger.Info("NebulaGraph workbench store initialized",
			zap.Strings("addresses", cfg.Nebula.Addresses),
			zap.String("space", cfg.Nebula.Space))
	} else {
		logger.Warn("NebulaGraph workbench store disabled; using ClickHouse compatibility path")
	}
	logger.Info("Graph query engine initialized")

	// ==================== M07 durable graph projection ====================

	var graphProjectionConsumer *commonkafka.Consumer
	if cfg.Projection.ConsumerEnabled || cfg.Projection.WorkerEnabled {
		projectionInbox, projectionErr := graphprojection.NewInbox(pgDB)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize graph projection inbox", zap.Error(projectionErr))
		}
		readinessCtx, readinessCancel := context.WithTimeout(ctx, 10*time.Second)
		projectionErr = projectionInbox.VerifySchema(readinessCtx)
		readinessCancel()
		if projectionErr != nil {
			logger.Fatal("Graph projection inbox schema is not ready", zap.Error(projectionErr))
		}

		if cfg.Projection.ConsumerEnabled {
			graphProjectionConsumer, projectionErr = commonkafka.NewConsumer(commonkafka.ConsumerConfig{
				Brokers: cfg.Kafka.Brokers, Topic: cfg.Projection.Topic, GroupID: cfg.Projection.GroupID,
				StartOffset: segmentkafka.FirstOffset, MaxRetries: 3, RetryBackoff: time.Second,
				EnableDLQ: true, DLQTopic: cfg.Projection.DLQTopic,
				CommitOnDLQSuccess: true, DLQPermanentOnly: true,
				Security: cfg.KafkaSecurity,
			}, logger)
			if projectionErr != nil {
				logger.Fatal("Failed to initialize graph projection Kafka consumer", zap.Error(projectionErr))
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if consumeErr := graphProjectionConsumer.Consume(ctx, projectionInbox.Handle); consumeErr != nil && !errors.Is(consumeErr, context.Canceled) {
					logger.Error("Graph projection Kafka consumer stopped", zap.Error(consumeErr))
				}
			}()
			logger.Info("Graph projection consumer enabled",
				zap.String("topic", cfg.Projection.Topic), zap.String("group_id", cfg.Projection.GroupID))
		}

		if cfg.Projection.WorkerEnabled {
			projectionTarget, targetErr := graphnebula.NewProjectionTarget(workbenchStore)
			if targetErr != nil {
				logger.Fatal("Failed to initialize NebulaGraph projection target", zap.Error(targetErr))
			}
			hostname, _ := os.Hostname()
			projectionWorker, workerErr := graphprojection.NewWorker(pgDB, projectionTarget, graphprojection.WorkerConfig{
				WorkerID: "graph-projection/" + hostname, Lease: cfg.Projection.Lease,
				Interval: cfg.Projection.Interval, MaxAttempts: cfg.Projection.MaxAttempts, Logger: logger,
			})
			if workerErr != nil {
				logger.Fatal("Failed to initialize graph projection worker", zap.Error(workerErr))
			}
			readinessCtx, readinessCancel = context.WithTimeout(ctx, 10*time.Second)
			workerErr = projectionWorker.VerifySchema(readinessCtx)
			readinessCancel()
			if workerErr != nil {
				logger.Fatal("Graph projection target is not ready", zap.Error(workerErr))
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				projectionWorker.Run(ctx)
			}()
			logger.Info("NebulaGraph projection worker enabled",
				zap.Duration("lease", cfg.Projection.Lease), zap.Int("max_attempts", cfg.Projection.MaxAttempts))
		}
	} else {
		logger.Info("M07 graph projection remains default-off")
	}

	// ==================== 修复 W1：初始化缓存预热服务 ====================

	var warmupService *cache.WarmupService
	if graphCache != nil && pgDB != nil {
		// 适配器：GraphQueryWithCircuitBreaker → GraphQueryInterface
		warmupService = cache.NewWarmupService(
			pgDB,
			graphCache,
			&graphQueryExploreAdapter{graphQuery},
			logger,
			1*time.Hour,
		)
		warmupService.Start()
		logger.Info("Cache warmup service started")
	}

	// ==================== 修复 G3：初始化 API Handler ====================

	handler := api.NewHandlerWithMonitoring(
		graphQuery,
		graphCache,
		auditLogger,
		rateLimiter,
		cfg.Security,
		cfg.Query,
		cfg.Workbench,
		logger,
		queryLogger,        // 修复 G1
		slowQueryDetector,  // 修复 G1
		tenantConfigLoader, // 修复 G2
	)
	projectionStatusRepository, err := graphprojection.NewStatusRepository(
		pgDB,
		cfg.Projection.ConsumerEnabled,
		cfg.Projection.WorkerEnabled && cfg.ContinuousProjectionEnabled,
		5*time.Minute,
	)
	if err != nil {
		logger.Fatal("Failed to initialize graph projection status repository", zap.Error(err))
	}
	handler.SetProjectionStatusRepository(projectionStatusRepository)
	logger.Info("API handler initialized with monitoring")

	// ==================== 创建 Router ====================

	r := mux.NewRouter()

	// 注册健康检查和指标端点
	r.HandleFunc("/health", handler.HealthCheck).Methods("GET")
	r.HandleFunc("/ready", handler.ReadinessCheck).Methods("GET")

	// 注册业务路由。Handler 内部已经声明 /api/v1 前缀，入口层只传根路由。
	handler.RegisterRoutes(r)

	var graphAuthMiddleware httpx.Middleware
	if cfg.Auth.Enabled && cfg.Auth.RequireAuth {
		jwtSecret := getEnv("JWT_SECRET_KEY", getEnv("JWT_SIGNING_KEY", ""))
		if jwtSecret == "" {
			logger.Fatal("AUTH_REQUIRE_AUTH is enabled but JWT_SECRET_KEY/JWT_SIGNING_KEY is empty")
		}
		userRepo := authrepository.NewUserRepository(pgDB, logger)
		tokenRepo := authrepository.NewTokenRepository(pgDB, logger)
		jwtService, jwtErr := authjwt.NewService(authjwt.Config{
			SigningKey:      jwtSecret,
			SigningMethod:   "HS256",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "traffic-auth-service",
		}, redisClient, tokenRepo, logger)
		if jwtErr != nil {
			logger.Fatal("Failed to initialize graph JWT service", zap.Error(jwtErr))
		}
		// 统一令牌模型 P1:附带 OIDC Provider,使 ValidateToken 具备
		// Keycloak 访问令牌 JWKS 回退(修复 nil provider 缺陷)。
		authCfg, authCfgErr := authConfig.Load()
		if authCfgErr != nil {
			authCfg = &authConfig.Config{}
		}
		var oidcProvider *oidc.Provider
		if authCfg.OIDC.Enabled {
			if p, err := oidc.NewProvider(authCfg.OIDC, logger); err != nil {
				logger.Warn("OIDC provider init failed; Keycloak token fallback disabled", zap.Error(err))
			} else {
				oidcProvider = p
			}
		}
		authService := authservice.NewAuthService(userRepo, jwtService, oidcProvider, authCfg, logger, nil)
		graphAuthMiddleware = httpx.Auth(tokenValidatorAdapter{authService: authService}, logger)
		logger.Info("Graph API auth middleware initialized")
	}

	// /metrics 仅在启用认证时要求 admin:* 权限,防止指标端点被匿名访问
	// (Prometheus 抓取需配置对应凭据或由 NetworkPolicy 收敛访问面)。
	metricsHandler := promhttp.Handler()
	if graphAuthMiddleware != nil {
		metricsHandler = httpx.RequireAnyPermission("admin:*", "*")(graphAuthMiddleware(metricsHandler))
	}
	r.Handle("/metrics", metricsHandler).Methods("GET")

	// ==================== 构建中间件链 ====================

	middlewareChain := buildMiddlewareChain(cfg, logger)
	finalHandler := middlewareChain.Then(protectGraphBusinessAPI(r, graphAuthMiddleware, logger, auditLogger))

	// ==================== HTTP Server ====================

	srv := &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           finalHandler,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
	}

	// ==================== 修复 G4：启动后台监控 Goroutines ====================

	// 连接池监控
	if cfg.IsDevelopment() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitorConnectionPools(ctx, chClient, redisClient, logger)
		}()
	}

	// 缓存统计打印
	if graphCache != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			printCacheStats(ctx, graphCache, logger)
		}()
	}

	// Circuit Breaker 监控
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitorCircuitBreaker(ctx, chCircuitBreaker, logger)
	}()

	// ==================== 启动服务器 ====================

	go func() {
		logger.Info("Starting Graph API server",
			zap.String("addr", cfg.Server.ListenAddr),
			zap.Duration("read_timeout", cfg.Server.ReadTimeout),
			zap.Duration("write_timeout", cfg.Server.WriteTimeout))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// ==================== 等待关闭信号 ====================

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	logger.Info("Shutting down gracefully...")

	// 取消 context，通知所有 goroutine 退出
	cancel()
	if graphProjectionConsumer != nil {
		if err := graphProjectionConsumer.Close(); err != nil {
			logger.Error("Failed to close graph projection Kafka consumer", zap.Error(err))
		}
	}

	// 停止缓存预热服务
	if warmupService != nil {
		warmupService.Stop()
	}

	// 优雅关闭 HTTP Server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	// 关闭图查询引擎
	if err := graphQuery.Close(); err != nil {
		logger.Error("Graph query engine close error", zap.Error(err))
	}

	// 等待所有 goroutine 退出（修复泄漏）
	logger.Info("Waiting for background tasks to complete...")
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("All background tasks completed")
	case <-time.After(10 * time.Second):
		logger.Warn("Timeout waiting for background tasks, forcing exit")
	}

	// ==================== 记录最终统计 ====================

	if graphCache != nil {
		stats := graphCache.GetStats()
		logger.Info("Final cache statistics", zap.Any("stats", stats))
	}

	queryMetrics := graphQuery.GetMetrics()
	logger.Info("Final query statistics",
		zap.Int64("total_queries", queryMetrics.TotalQueries),
		zap.Int64("failed_queries", queryMetrics.FailedQueries),
		zap.Int64("timeout_queries", queryMetrics.TimeoutQueries),
		zap.Int64("cache_hits", queryMetrics.CacheHits),
		zap.Int64("cache_misses", queryMetrics.CacheMisses))

	cbMetrics := chCircuitBreaker.GetMetrics()
	logger.Info("Final circuit breaker statistics",
		zap.String("state", cbMetrics.State.String()),
		zap.Int64("total_requests", cbMetrics.TotalRequests),
		zap.Int64("failed_requests", cbMetrics.FailedRequests),
		zap.Int64("rejected_requests", cbMetrics.RejectedRequests))

	logger.Info("Shutdown complete")
}

func protectGraphBusinessAPI(next http.Handler, authMiddleware httpx.Middleware, logger *zap.Logger, auditLogger *audit.Logger) http.Handler {
	if authMiddleware == nil {
		return next
	}
	// P2 契约解释器:认证之后、业务路由之前逐操作判定(认证层先产出主体,
	// 契约层按 required_scope 判定;/api/v1/graph/* 4 操作均要求 graph:read)。
	contractNext := next
	if mode := strings.TrimSpace(getEnv("AUTHZ_CONTRACT_MODE", "")); mode == "enforce" || mode == "shadow" {
		contractNext = authz.EnforceContract(graphContractPrincipal, mode, nil, logger, graphContractDenyAudit(auditLogger))(next)
		logger.Info("Contract interpreter enabled (graph business API)", zap.String("mode", mode))
	}
	protected := authMiddleware(httpx.RequireAnyPermission("graph:read", "admin:*", "*")(contractNext))
	// 缓存管理端点为管理员操作:除处理器内部角色校验外,路由层也收紧为 admin 权限。
	adminProtected := authMiddleware(httpx.RequireAnyPermission("admin:*", "*")(contractNext))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || !strings.HasPrefix(r.URL.Path, "/api/v1/graph") {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/graph/cache/") {
			adminProtected.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

// ==================== 辅助函数 ====================

// buildMiddlewareChain 构建中间件链
func buildMiddlewareChain(cfg *config.Config, logger *zap.Logger) *httpx.Chain {
	corsConfig := &httpx.CORSConfig{
		AllowedOrigins: cfg.API.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept", "Authorization", "Content-Type",
			"X-Tenant-ID", "X-User-ID", "X-Request-ID", "X-Trace-ID",
			"X-Run-ID", "X-Probe-ID",
		},
		ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID"},
		AllowCredentials: true,
		MaxAge:           86400,
	}

	otelConfig := otel.DefaultMiddlewareConfig(cfg.OTEL.ServiceName)

	chain := httpx.NewChain(
		httpx.Recovery(logger),
		httpx.RequestID(),
		httpx.Logging(logger),
		httpx.CORS(corsConfig),
		httpx.BodyLimit(cfg.API.MaxRequestBodySize),
		httpx.Metrics(cfg.OTEL.ServiceName),
		httpx.Middleware(otel.HTTPMiddlewareWithConfig(otelConfig)),
		httpx.TimeoutWithConfig(cfg.API.RequestTimeout, nil),
		httpx.TenantExtractor(),
	)

	return chain
}

// monitorConnectionPools 监控连接池状态（修复 G4：添加空指针检查）
func monitorConnectionPools(ctx context.Context, chClient *storage.ClickHouseClient, redisClient *storage.RedisClient, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Connection pool monitor stopped")
			return
		case <-ticker.C:
			// 修复 G4：添加空指针检查
			if chClient != nil {
				stats := chClient.Stats()
				if stats.Open > 0 {
					logger.Debug("ClickHouse connection pool stats",
						zap.Int("open", stats.Open),
						zap.Int("open_connections", stats.Open),
						zap.Int("idle", stats.Idle))
				}
			}

			if redisClient != nil {
				client := redisClient.Client()
				if client != nil {
					stats := client.PoolStats()
					logger.Debug("Redis connection pool stats",
						zap.Uint32("total_conns", stats.TotalConns),
						zap.Uint32("idle_conns", stats.IdleConns))
				}
			}
		}
	}
}

// printCacheStats 定期打印缓存统计
func printCacheStats(ctx context.Context, graphCache *cache.GraphCache, logger *zap.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Cache stats printer stopped")
			return
		case <-ticker.C:
			stats := graphCache.GetStats()
			logger.Info("Cache statistics",
				zap.Any("hits", stats["hits"]),
				zap.Any("misses", stats["misses"]),
				zap.Any("hit_rate", stats["hit_rate"]))
		}
	}
}

// monitorCircuitBreaker 监控 Circuit Breaker 状态
func monitorCircuitBreaker(ctx context.Context, cb *utils.CircuitBreaker, logger *zap.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Circuit breaker monitor stopped")
			return
		case <-ticker.C:
			metrics := cb.GetMetrics()
			if metrics.State != utils.StateClosed {
				logger.Warn("Circuit breaker not in closed state",
					zap.String("state", metrics.State.String()),
					zap.Int64("failed_requests", metrics.FailedRequests),
					zap.Int64("rejected_requests", metrics.RejectedRequests),
					zap.Float64("failure_rate", metrics.FailureRate()))
			} else {
				logger.Debug("Circuit breaker status",
					zap.String("state", metrics.State.String()),
					zap.Int64("total_requests", metrics.TotalRequests),
					zap.Float64("failure_rate", metrics.FailureRate()))
			}
		}
	}
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func postgresDSNFromEnv() string {
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn
	}

	pairs := []string{
		pqKV("host", getEnv("POSTGRES_HOST", "localhost")),
		pqKV("port", getEnv("POSTGRES_PORT", "5432")),
		pqKV("user", getEnv("POSTGRES_USERNAME", "postgres")),
		pqKV("password", os.Getenv("POSTGRES_PASSWORD")),
		pqKV("dbname", getEnv("POSTGRES_DATABASE", "traffic_platform")),
		pqKV("sslmode", getEnv("POSTGRES_SSL_MODE", "disable")),
		pqKV("connect_timeout", getEnv("POSTGRES_CONNECT_TIMEOUT", "10")),
	}
	return strings.Join(pairs, " ")
}

func pqKV(key, value string) string {
	return key + "=" + pqQuote(value)
}

func pqQuote(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return "'" + escaped + "'"
}

// graphQueryExploreAdapter 将 GraphQueryWithCircuitBreaker 适配为 GraphQueryInterface
type graphQueryExploreAdapter struct {
	gq *query.GraphQueryWithCircuitBreaker
}

func (a *graphQueryExploreAdapter) Explore(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) (interface{}, error) {
	return a.gq.Explore(ctx, tenantID, centerIP, depth, startTime, endTime, runID)
}

// graphContractPrincipal 将 graph-service 认证层(httpx.Auth 统一入口)产出的
// httpx 上下文键适配为契约解释器判定主体。
func graphContractPrincipal(r *http.Request) *authz.Principal {
	userID := strings.TrimSpace(httpx.GetUserID(r.Context()))
	if userID == "" {
		return nil
	}
	return &authz.Principal{
		Kind:        authz.PrincipalKindHuman,
		Subject:     userID,
		Username:    httpx.GetUsername(r.Context()),
		TenantID:    httpx.GetTenantID(r.Context()),
		Roles:       httpx.GetRoles(r.Context()),
		Permissions: httpx.GetPermissions(r.Context()),
	}
}

// graphContractDenyAudit 契约拒绝留痕(Kafka 审计事件)。
func graphContractDenyAudit(auditLogger *audit.Logger) authz.ContractDenyAuditor {
	return func(r *http.Request, op *authz.Operation, principal *authz.Principal, status int) {
		if auditLogger == nil {
			return
		}
		userID := ""
		if principal != nil {
			userID = principal.Subject
		}
		auditLogger.Log(r.Context(), &audit.AuditEvent{
			EventType:    audit.EventTypePermissionDenied,
			TenantID:     httpx.GetTenantID(r.Context()),
			UserID:       userID,
			ServiceName:  "graph-service",
			SourceIP:     httpx.GetClientIP(r),
			Action:       "CONTRACT_ACCESS_DENIED",
			ResourceType: "contract_operation",
			ResourceID:   op.OperationID,
			Detail:       map[string]interface{}{"operation_id": op.OperationID, "required_scope": op.RequiredScope, "path": r.URL.Path, "result": "denied", "status": status},
			Result:       audit.ResultFailure,
		})
	}
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/cmd/alert-service/main.go
// 修复版：修复配置类型转换、初始化 Arkime 和 Evidence Generator
////////////////////////////////////////////////////////////////////////////////

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/arkime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/consumer"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/evidence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/playbook"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/realtime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/risk"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	authRepo "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	authService "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	commonAudit "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

func main() {
	// 初始化日志
	logCfg := logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Format:      getEnv("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     "alert-service",
		Version:     getEnv("SERVICE_VERSION", "1.0.0"),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logging.Sync(logger)

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Starting Alert Service",
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.String("kafka_topic", cfg.Kafka.Topic),
		zap.String("api_addr", cfg.API.ListenAddr))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ==================== 初始化 Redis ====================
	var rdb *redis.Client
	if len(cfg.Redis.SentinelAddrs) > 0 && cfg.Redis.SentinelMaster != "" {
		rdb = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    cfg.Redis.SentinelMaster,
			SentinelAddrs: cfg.Redis.SentinelAddrs,
			Password:      cfg.Redis.Password,
			DB:            cfg.Redis.DB,
			PoolSize:      cfg.Redis.PoolSize,
			MinIdleConns:  5,
		})

		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			logger.Fatal("Failed to connect to Redis Sentinel master", zap.Error(err))
		}
		pingCancel()
		logger.Info("Connected to Redis Sentinel",
			zap.Strings("sentinels", cfg.Redis.SentinelAddrs),
			zap.String("master", cfg.Redis.SentinelMaster))
		defer rdb.Close()
	} else if len(cfg.Redis.Addrs) > 0 && cfg.Redis.Addrs[0] != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:         cfg.Redis.Addrs[0],
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: 5,
		})

		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			logger.Fatal("Failed to connect to Redis", zap.Error(err))
		}
		pingCancel()
		logger.Info("Connected to Redis", zap.String("addr", cfg.Redis.Addrs[0]))
		defer rdb.Close()
	} else {
		logger.Fatal("Redis configuration is required")
	}

	// ==================== 初始化 ClickHouse ====================
	chClient, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts:           cfg.ClickHouse.GetHosts(),
		Database:        cfg.ClickHouse.GetDatabase(),
		Username:        cfg.ClickHouse.GetUsername(),
		Password:        cfg.ClickHouse.GetPassword(),
		MaxOpenConns:    cfg.ClickHouse.MaxOpenConns,
		MaxIdleConns:    cfg.ClickHouse.MaxIdleConns,
		ConnMaxLifetime: time.Hour,
		DialTimeout:     10 * time.Second,
		CompressionLZ4:  true,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to connect to ClickHouse", zap.Error(err))
	}
	defer chClient.Close()
	logger.Info("Connected to ClickHouse",
		zap.Strings("hosts", cfg.ClickHouse.GetHosts()),
		zap.String("database", cfg.ClickHouse.GetDatabase()))

	var chSQLDB *sql.DB
	if chSQLDB, err = initClickHouseSQLDB(cfg.ClickHouse, logger); err != nil {
		logger.Warn("Failed to initialize SQL ClickHouse client for advanced APIs", zap.Error(err))
	} else {
		defer chSQLDB.Close()
	}

	// ==================== 初始化 PostgreSQL (用于 Auth) ====================
	var db *sql.DB
	if cfg.Auth.Enabled {
		authPostgresDSN := cfg.Auth.ConnectionString()
		if authPostgresDSN == "" {
			logger.Fatal("PostgreSQL DSN is required when authentication is enabled")
		}
		db, err = sql.Open("postgres", authPostgresDSN)
		if err != nil {
			logger.Fatal("Failed to open PostgreSQL while authentication is enabled", zap.Error(err))
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)

		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pingCtx)
		pingCancel()
		if err != nil {
			_ = db.Close()
			logger.Fatal("Failed to ping PostgreSQL while authentication is enabled", zap.Error(err))
		}
		defer db.Close()
		logger.Info("Connected to PostgreSQL for auth")
	}

	// ==================== 初始化 ClickHouse Writer ====================
	chWriter, err := persistence.NewClickHouseWriter(chClient, logger)
	if err != nil {
		logger.Fatal("Failed to create ClickHouse writer", zap.Error(err))
	}

	// ==================== 初始化 OpenSearch Writer ====================
	osWriter, err := persistence.NewOpenSearchWriter(
		cfg.OpenSearch.Addresses,
		cfg.OpenSearch.Username,
		cfg.OpenSearch.Password,
		cfg.OpenSearch.Index,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to create OpenSearch writer", zap.Error(err))
	}
	defer osWriter.Close()
	logger.Info("Connected to OpenSearch",
		zap.Strings("addresses", cfg.OpenSearch.Addresses),
		zap.String("index", cfg.OpenSearch.Index))

	// ==================== 初始化 Dual Writer ====================
	dualWriter := persistence.NewDualWriter(chWriter, osWriter, 5, logger)
	go dualWriter.StartHealthCheck(ctx)

	// ==================== 初始化 Redis Dedup ====================
	redisDedup := dedup.NewRedisDedup(rdb, cfg.Dedup.TTL, logger)

	// ==================== 初始化 Arkime Link Generator ====================
	arkimeBaseURL := getEnv("ARKIME_BASE_URL", "http://arkime:8005")
	arkimeConfig := arkime.Config{
		BaseURL:        arkimeBaseURL,
		SessionsPath:   getEnv("ARKIME_SESSIONS_PATH", "/sessions"),
		TimeBufferSecs: getIntEnv("ARKIME_TIME_BUFFER_SECS", 60),
	}
	arkimeLinkGen := arkime.NewLinkGenerator(arkimeConfig)
	logger.Info("Initialized Arkime Link Generator",
		zap.String("base_url", arkimeConfig.BaseURL))

	// ==================== 初始化 Evidence Generator ====================
	visualBaseURL := getEnv("VISUALIZATION_BASE_URL", "http://localhost:3000")
	evidenceGen := evidence.NewGenerator(chClient, arkimeLinkGen, visualBaseURL, logger)
	logger.Info("Initialized Evidence Generator",
		zap.String("visual_base_url", visualBaseURL))

	// ==================== 初始化 Repositories ====================
	alertRepo := repository.NewAlertRepository(chClient, logger)

	osRepo, err := repository.NewOpenSearchRepository(repository.OpenSearchConfig{
		Addresses: cfg.OpenSearch.Addresses,
		Username:  cfg.OpenSearch.Username,
		Password:  cfg.OpenSearch.Password,
		IndexName: cfg.OpenSearch.Index,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to create OpenSearch repository", zap.Error(err))
	}

	// ==================== 初始化 Audit Logger ====================
	auditCfg := commonAudit.Config{
		KafkaBrokers:  cfg.Kafka.Brokers,
		Topic:         "audit.logs",
		ServiceName:   "alert-service",
		BufferSize:    1000,
		BatchSize:     100,
		FlushInterval: time.Second,
		Security:      cfg.Kafka.Security,
	}
	auditLogger, err := commonAudit.NewLogger(auditCfg, logger)
	if err != nil {
		logger.Warn("Failed to create audit logger, continuing without audit", zap.Error(err))
	} else {
		defer auditLogger.Close()
	}
	alertAuditLogger := audit.NewAlertAuditLogger(auditLogger)

	// ==================== 初始化 Alert Service ====================
	// 使用带 Evidence 的构造函数
	alertService := service.NewAlertServiceWithEvidence(
		alertRepo,
		osRepo,
		dualWriter,
		redisDedup,
		evidenceGen,
		arkimeLinkGen,
		alertAuditLogger,
		logger,
	)

	// ==================== 初始化 Kafka Producer (for Feedback) ====================
	var feedbackProducer *kafka.Producer
	feedbackProducerCfg := kafka.ProducerConfig{
		Brokers:      cfg.Kafka.Brokers,
		Topic:        "alert.feedback.v1",
		BatchSize:    100,
		RequiredAcks: "all",
		Compression:  "lz4",
		Security:     cfg.Kafka.Security,
	}
	feedbackProducer, err = kafka.NewProducer(feedbackProducerCfg, logger)
	if err != nil {
		logger.Warn("Failed to create feedback Kafka producer", zap.Error(err))
		feedbackProducer = nil
	} else {
		defer feedbackProducer.Close()
	}

	// ==================== 初始化 API Handler ====================
	var apiHandler *api.Handler
	if feedbackProducer != nil {
		apiHandler = api.NewHandlerWithFeedback(alertService, feedbackProducer, alertAuditLogger, logger)
	} else {
		apiHandler = api.NewHandler(alertService, alertAuditLogger, logger)
	}
	apiHandler.SetActionAuditWriter(api.NewAlertActionAuditWriter(db, logger))

	// 初始化反馈持久化 (ClickHouse) — TP/FP 闭环
	if chClient != nil {
		feedbackRepo := api.NewFeedbackRepository(chClient, logger)
		apiHandler.SetFeedbackRepo(feedbackRepo)
		go func() {
			schemaCtx, schemaCancel := context.WithTimeout(ctx, 30*time.Second)
			defer schemaCancel()
			if err := feedbackRepo.InitSchema(schemaCtx); err != nil {
				logger.Warn("Failed to init feedback schema", zap.Error(err))
				return
			}
			logger.Info("Feedback repository initialized (TP/FP persistence + stats)")
		}()
		logger.Info("Feedback repository configured; schema initialization runs asynchronously")
	}

	// ==================== 初始化 Kafka Consumer ====================
	// 使用带 Evidence 的构造函数
	kafkaConsumer := consumer.NewConsumerWithEvidence(
		cfg.Kafka,
		cfg.Dedup,
		redisDedup,
		dualWriter,
		evidenceGen,
		arkimeLinkGen,
		logger,
	)
	if kafkaConsumer != nil {
		defer kafkaConsumer.Close()
		apiHandler.SetConsumerHealthCheck(kafkaConsumer.HealthCheck)

		// 启动 Consumer
		go func() {
			logger.Info("Starting Kafka consumer")
			if err := kafkaConsumer.Start(ctx); err != nil {
				if err != context.Canceled {
					logger.Error("Kafka consumer error", zap.Error(err))
				}
			}
		}()
	} else {
		logger.Warn("Kafka consumer not initialized")
	}

	// ==================== 设置路由 ====================
	r := mux.NewRouter()

	// 健康检查（不需要认证）
	r.HandleFunc("/health", apiHandler.HealthCheck).Methods("GET")
	r.HandleFunc("/ready", apiHandler.ReadinessCheck).Methods("GET")

	// Metrics 端点
	r.Handle("/metrics", promhttp.Handler())

	// ==================== 初始化 Auth 中间件 ====================
	var authMiddleware *middleware.AuthMiddleware
	var realtimeAuthService *authService.AuthService
	if cfg.Auth.Enabled && db != nil {
		// 初始化 User Repository
		userRepo := authRepo.NewUserRepository(db, logger)

		// 初始化 JWT Service（使用正确的配置类型）
		jwtConfig := jwt.Config{
			SigningKey:      cfg.Auth.JWTSecretKey,
			SigningMethod:   "HS256",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "traffic-auth-service",
		}
		tokenRepo := authRepo.NewTokenRepository(db, logger)
		jwtService, jwtErr := jwt.NewService(jwtConfig, storage.NewRedisClientFromExisting(rdb, logger), tokenRepo, logger)
		if jwtErr != nil {
			logger.Fatal("Failed to init JWT service", zap.Error(jwtErr))
		}

		// 初始化 Auth Service
		authSvc := authService.NewAuthService(userRepo, jwtService, nil, nil, logger, nil)
		realtimeAuthService = authSvc

		// 初始化 Auth Middleware
		authMiddleware = middleware.NewAuthMiddleware(authSvc, logger)
		logger.Info("Auth middleware initialized")
	} else {
		logger.Warn("Authentication is explicitly disabled by configuration")
	}

	applyAPIMiddlewares := func(router *mux.Router) {
		router.Use(
			mux.MiddlewareFunc(httpx.Recovery(logger)),
			mux.MiddlewareFunc(httpx.RequestID()),
			mux.MiddlewareFunc(httpx.Logging(logger)),
			mux.MiddlewareFunc(httpx.CORS(httpx.DefaultCORSConfig())),
			mux.MiddlewareFunc(httpx.Metrics("alert-service")),
			mux.MiddlewareFunc(httpx.TenantExtractor()),
		)
		if authMiddleware != nil {
			router.Use(authMiddleware.Authenticate)
		}
	}
	systemHandler := api.NewSystemHandler(chClient, db, logger)

	realtimeHandler := realtime.NewHandler(realtimeAuthService, logger)
	r.HandleFunc("/ws", realtimeHandler.HandleEvents).Methods("GET")
	r.HandleFunc("/ws/events", realtimeHandler.HandleEvents).Methods("GET")
	logger.Info("Realtime WebSocket endpoint registered", zap.Strings("paths", []string{"/ws", "/ws/events"}))

	probeRouter := r.PathPrefix("/api/v1/probes").Subrouter()
	applyAPIMiddlewares(probeRouter)
	probeRouter.HandleFunc("", systemHandler.ListProbes).Methods("GET")
	probeRouter.HandleFunc("/batch-upgrade", systemHandler.BatchUpgradeProbes).Methods("POST")
	probeRouter.HandleFunc("/{id}/config", systemHandler.PushProbeConfig).Methods("POST")
	probeRouter.HandleFunc("/{id}/connectivity-test", systemHandler.RunProbeConnectivityTest).Methods("POST")
	probeRouter.HandleFunc("/{id}/certificates/rotate", systemHandler.RotateProbeCertificate).Methods("POST")
	logger.Info("Probe operations API registered", zap.Strings("paths", []string{
		"/api/v1/probes",
		"/api/v1/probes/batch-upgrade",
		"/api/v1/probes/{id}/config",
		"/api/v1/probes/{id}/connectivity-test",
		"/api/v1/probes/{id}/certificates/rotate",
	}))

	// ==================== API 路由 ====================
	apiRouter := r.PathPrefix("/api/v1").Subrouter()

	// 应用中间件链
	applyAPIMiddlewares(apiRouter)

	// 注册 API 路由
	apiHandler.RegisterRoutes(apiRouter)

	// Dashboard API — 实时统计 (Web UI 大屏)
	dashboardHandler := api.NewDashboardHandler(chClient, logger)
	apiRouter.HandleFunc("/dashboard/stats", dashboardHandler.GetStats).Methods("GET")
	apiRouter.HandleFunc("/dashboard/alerts/trend", dashboardHandler.GetAlertTrend).Methods("GET")
	apiRouter.HandleFunc("/dashboard/attack-phases", dashboardHandler.GetAttackPhases).Methods("GET")
	apiRouter.HandleFunc("/dashboard/top-ips/{type}", dashboardHandler.GetTopIPs).Methods("GET")
	apiRouter.HandleFunc("/dashboard/encrypted/trend", dashboardHandler.GetEncryptedTrend).Methods("GET")
	logger.Info("Dashboard API registered (stats/trend/phases/top-ips/encrypted)")

	systemHandler.RegisterRoutes(apiRouter)
	logger.Info("Campaign, attack-chain and probe APIs registered")
	apiRouter.HandleFunc("/topics/views", systemHandler.ListTopicViews).Methods("GET")
	apiRouter.HandleFunc("/topics/views", systemHandler.SaveTopicView).Methods("POST")
	apiRouter.HandleFunc("/topics/views/{id}", systemHandler.UpdateTopicView).Methods("PATCH")
	apiRouter.HandleFunc("/topics/scopes/{topic}", systemHandler.UpdateTopicScope).Methods("PUT", "PATCH")
	apiRouter.HandleFunc("/topics/subscriptions", systemHandler.ListTopicSubscriptions).Methods("GET")
	apiRouter.HandleFunc("/topics/subscriptions", systemHandler.CreateTopicSubscription).Methods("POST")
	apiRouter.HandleFunc("/topics/subscriptions/{id}", systemHandler.UpdateTopicSubscription).Methods("PATCH")
	apiRouter.HandleFunc("/topics/reports/export", systemHandler.ExportTopicReport).Methods("POST")
	apiRouter.HandleFunc("/topics/evidence-packages/export", systemHandler.ExportTopicEvidencePackage).Methods("POST")
	logger.Info("Topic governance APIs registered")

	// 白名单管理 (PostgreSQL) — Web UI /api/v1/whitelist
	if db != nil {
		whitelistRepo := whitelist.NewRepository(db, logger)
		if err := whitelistRepo.InitSchema(context.Background()); err != nil {
			logger.Warn("Failed to init whitelist schema", zap.Error(err))
		} else {
			apiHandler.SetFeedbackWhitelistRepo(whitelistRepo)
			whitelistHandler := whitelist.NewHandler(whitelistRepo, logger)
			whitelistHandler.RegisterRoutes(apiRouter)
			logger.Info("Whitelist management initialized (IP/domain/fingerprint/subnet)")
		}
	}

	// Advanced Alert Features (notification + risk + playbook + data quality)
	notifyCfg := notification.NotifyConfig{MinSeverity: "high", RateLimitPerMin: 10}
	notifier := notification.NewNotificationService(notifyCfg, logger)
	executor := playbook.NewActionExecutor(logger)
	playbookEngine := playbook.NewPlaybookEngine(executor, logger)
	for _, pb := range playbook.DefaultPlaybooks() {
		playbookEngine.RegisterPlaybook(pb)
	}

	var advancedRepo *api.AdvancedRepository
	if db != nil {
		advancedRepo = api.NewAdvancedRepository(db, logger)
		advancedCtx, advancedCancel := context.WithTimeout(ctx, 15*time.Second)
		if err := advancedRepo.InitSchema(advancedCtx); err != nil {
			logger.Warn("Failed to init advanced API schema", zap.Error(err))
			advancedRepo = nil
		} else {
			overrides, err := advancedRepo.ListPlaybookOverrides(advancedCtx, "default")
			if err != nil {
				logger.Warn("Failed to load playbook overrides", zap.Error(err))
			} else {
				for _, override := range overrides {
					enabled := override.Enabled
					maxRuns := override.MaxRuns
					cooldown := override.Cooldown
					if _, err := playbookEngine.UpdatePlaybook(override.Name, &enabled, &maxRuns, &cooldown); err != nil {
						logger.Warn("Failed to apply playbook override", zap.String("name", override.Name), zap.Error(err))
					}
				}
				logger.Info("Advanced API repository initialized", zap.Int("playbook_overrides", len(overrides)))
			}
		}
		advancedCancel()
	}

	var riskScorer *risk.AssetRiskScorer
	var dqMonitor *dataquality.Monitor
	if chSQLDB != nil {
		riskScorer = risk.NewAssetRiskScorer(chSQLDB, db, logger)
		dqMonitor = dataquality.NewMonitor(chSQLDB, dataquality.MonitorConfig{
			CheckInterval:       15 * time.Minute,
			MinFlowRate:         100,
			MaxMissingPercent:   5.0,
			MaxLatencyP95:       60000,
			MaxSchemaDriftCount: 3,
		}, logger)
	}
	advHandler := api.NewAdvancedHandler(notifier, riskScorer, playbookEngine, dqMonitor, advancedRepo)
	advHandler.RegisterAPIRoutes(apiRouter)
	logger.Info("Advanced alert features enabled (notification + risk + playbook + data quality)")
	// ==================== HTTP Server ====================
	srv := &http.Server{
		Addr:         cfg.API.ListenAddr,
		Handler:      r,
		ReadTimeout:  cfg.API.ReadTimeout,
		WriteTimeout: cfg.API.WriteTimeout,
		IdleTimeout:  cfg.API.IdleTimeout,
	}

	// 启动 HTTP 服务器
	go func() {
		logger.Info("Starting HTTP server", zap.String("addr", cfg.API.ListenAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// ==================== 等待关闭信号 ====================
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// 优雅关闭
	cancel() // 停止 Consumer

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}

	logger.Info("Alert Service stopped")
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getIntEnv 获取整数环境变量，带默认值
func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func initClickHouseSQLDB(cfg config.ClickHouseConfig, logger *zap.Logger) (*sql.DB, error) {
	hosts := cfg.GetHosts()
	if len(hosts) == 0 {
		return nil, fmt.Errorf("clickhouse hosts not configured")
	}

	host := strings.TrimSpace(hosts[0])
	if host == "" {
		return nil, fmt.Errorf("clickhouse host is empty")
	}

	database := cfg.GetDatabase()
	if database == "" {
		database = "traffic"
	}
	username := cfg.GetUsername()
	if username == "" {
		username = "default"
	}
	password := cfg.GetPassword()

	dsn := url.URL{
		Scheme: "clickhouse",
		Host:   host,
		Path:   "/" + database,
	}
	if password != "" {
		dsn.User = url.UserPassword(username, password)
	} else {
		dsn.User = url.User(username)
	}
	q := dsn.Query()
	q.Set("dial_timeout", "10s")
	q.Set("read_timeout", "30s")
	dsn.RawQuery = q.Encode()

	db, err := sql.Open("clickhouse", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open clickhouse SQL client: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(10)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(5)
	}
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping clickhouse SQL client: %w", err)
	}

	logger.Info("Connected to ClickHouse SQL client for advanced APIs",
		zap.String("host", host),
		zap.String("database", database))
	return db, nil
}

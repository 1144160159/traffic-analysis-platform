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
	"os"
	"os/signal"
	"syscall"
	"time"

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
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	authRepo "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	authService "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	commonAudit "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
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
	if len(cfg.Redis.Addrs) > 0 && cfg.Redis.Addrs[0] != "" {
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

	// ==================== 初始化 PostgreSQL (用于 Auth) ====================
	var db *sql.DB
	if cfg.Auth.Enabled && cfg.Auth.PostgresDSN != "" {
		db, err = sql.Open("postgres", cfg.Auth.PostgresDSN)
		if err != nil {
			logger.Warn("Failed to open PostgreSQL for auth, auth will be disabled", zap.Error(err))
		} else {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(time.Hour)

			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			if err := db.PingContext(pingCtx); err != nil {
				logger.Warn("Failed to ping PostgreSQL, auth will be disabled", zap.Error(err))
				db.Close()
				db = nil
			}
			pingCancel()

			if db != nil {
				defer db.Close()
				logger.Info("Connected to PostgreSQL for auth")
			}
		}
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
	if cfg.Auth.Enabled && db != nil {
		// 初始化 User Repository
		userRepo := authRepo.NewUserRepository(db)

		// 初始化 JWT Service（使用正确的配置类型）
		jwtConfig := jwt.Config{
			SigningKey:      cfg.Auth.JWTSecretKey,
			SigningMethod:   "HS256",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "traffic-auth-service",
		}
		jwtService := jwt.NewService(jwtConfig, rdb, logger)

		// 初始化 Auth Service
		authSvc := authService.NewAuthService(userRepo, jwtService, nil, nil, logger)

		// 初始化 Auth Middleware
		authMiddleware = middleware.NewAuthMiddleware(authSvc, logger)
		logger.Info("Auth middleware initialized")
	} else {
		logger.Warn("Auth middleware not initialized, API endpoints will not be protected")
	}

	// ==================== API 路由 ====================
	apiRouter := r.PathPrefix("/api/v1").Subrouter()

	// 应用中间件链
	apiRouter.Use(
		httpx.Recovery(logger),
		httpx.RequestID(),
		httpx.Logging(logger),
		httpx.CORS(httpx.DefaultCORSConfig()),
		httpx.Metrics("alert-service"),
		httpx.TenantExtractor(),
	)

	// 添加 Auth 中间件（如果可用）
	if authMiddleware != nil {
		apiRouter.Use(authMiddleware.Authenticate)
	}

	// 注册 API 路由
	apiHandler.RegisterRoutes(apiRouter)

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

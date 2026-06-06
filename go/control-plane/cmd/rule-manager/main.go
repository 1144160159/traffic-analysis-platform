////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/cmd/rule-manager/main.go
// Rule Manager 服务主入口 - 完整修复版
// 修复内容：
// 1. ✅ 集成审计日志
// 2. ✅ 集成 Redis 缓存
// 3. ✅ 添加健康检查端点 (/healthz, /readyz)
// 4. ✅ 添加 Metrics 端点
// 5. ✅ 增强优雅关闭
// 6. ✅ 添加启动依赖检查
// 7. ✅ 集成 RBAC 权限检查
// 8. ✅ 传递 *sql.DB 给 RuleService 以支持 Outbox
// 9. ✅ 注册 RuleService 优雅关闭
////////////////////////////////////////////////////////////////////////////////

package main

import (
	"context"
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

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/health"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/publisher"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/service"
)

const (
	serviceName    = "rule-manager"
	serviceVersion = "1.0.0"
)

func main() {
	// =========================================================================
	// 1. 初始化日志
	// =========================================================================
	logCfg := logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Format:      getEnv("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     serviceName,
		Version:     getEnv("SERVICE_VERSION", serviceVersion),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logging.Sync(logger)

	logger.Info("Starting Rule Manager service",
		zap.String("version", serviceVersion),
		zap.String("environment", logCfg.Environment))

	// =========================================================================
	// 2. 加载配置
	// =========================================================================
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("api_addr", cfg.API.ListenAddr),
		zap.String("metrics_addr", cfg.Metrics.ListenAddr),
		zap.Bool("audit_enabled", cfg.Audit.Enabled),
		zap.Bool("rbac_enabled", cfg.RBAC.Enabled))

	// =========================================================================
	// 3. 初始化 PostgreSQL
	// =========================================================================
	pgCfg := storage.PostgresConfig{
		Host:            cfg.PostgreSQL.Host,
		Port:            cfg.PostgreSQL.Port,
		Database:        cfg.PostgreSQL.Database,
		Username:        cfg.PostgreSQL.Username,
		Password:        cfg.PostgreSQL.Password,
		SSLMode:         cfg.PostgreSQL.SSLMode,
		MaxOpenConns:    cfg.PostgreSQL.MaxOpenConns,
		MaxIdleConns:    cfg.PostgreSQL.MaxIdleConns,
		ConnMaxLifetime: cfg.PostgreSQL.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.PostgreSQL.ConnMaxIdleTime,
		ConnectTimeout:  int(cfg.PostgreSQL.ConnectTimeout.Seconds()),
	}

	pgClient, err := storage.NewPostgresClient(pgCfg, logger)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer func() {
		logger.Info("Closing PostgreSQL connection...")
		if err := pgClient.Close(); err != nil {
			logger.Error("Failed to close PostgreSQL connection", zap.Error(err))
		}
	}()

	logger.Info("Connected to PostgreSQL",
		zap.String("host", pgCfg.Host),
		zap.String("database", pgCfg.Database))

	// =========================================================================
	// 4. 初始化 Redis（可选，用于缓存和限流）
	// =========================================================================
	var redisClient *redis.Client
	if cfg.Redis.Enabled {
		// 优先使用 Addrs[0]，回退到 Addr
		redisAddr := cfg.Redis.Addr
		if len(cfg.Redis.Addrs) > 0 {
			redisAddr = cfg.Redis.Addrs[0]
		}

		if redisAddr != "" {
			redisClient = redis.NewClient(&redis.Options{
				Addr:            redisAddr,
				Password:        cfg.Redis.Password,
				DB:              cfg.Redis.DB,
				DialTimeout:     cfg.Redis.DialTimeout,
				ReadTimeout:     cfg.Redis.ReadTimeout,
				WriteTimeout:    cfg.Redis.WriteTimeout,
				PoolSize:        cfg.Redis.PoolSize,
				MinIdleConns:    cfg.Redis.MinIdleConns,
				PoolTimeout:     cfg.Redis.PoolTimeout,
				ConnMaxIdleTime: cfg.Redis.ConnMaxIdleTime,
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := redisClient.Ping(ctx).Err(); err != nil {
				logger.Warn("Failed to connect to Redis, caching disabled", zap.Error(err))
				redisClient = nil
			} else {
				logger.Info("Connected to Redis", zap.String("addr", redisAddr))
			}
			cancel()

			if redisClient != nil {
				defer func() {
					logger.Info("Closing Redis connection...")
					if err := redisClient.Close(); err != nil {
						logger.Error("Failed to close Redis connection", zap.Error(err))
					}
				}()
			}
		}
	}

	// =========================================================================
	// 5. 初始化审计日志
	// =========================================================================
	var auditLogger *audit.Logger
	if cfg.Audit.Enabled && len(cfg.Kafka.Brokers) > 0 {
		auditCfg := audit.Config{
			KafkaBrokers:    cfg.Kafka.Brokers,
			Topic:           cfg.Audit.Topic,
			ServiceName:     serviceName,
			BufferSize:      cfg.Audit.BufferSize,
			BatchSize:       cfg.Audit.BatchSize,
			FlushInterval:   cfg.Audit.FlushInterval,
			ShutdownTimeout: cfg.Audit.ShutdownTimeout,
			BackupEnabled:   cfg.Audit.BackupEnabled,
			BackupDir:       cfg.Audit.BackupDir,
		}

		auditLogger, err = audit.NewLogger(auditCfg, logger)
		if err != nil {
			logger.Warn("Failed to initialize audit logger", zap.Error(err))
		} else {
			logger.Info("Audit logger initialized",
				zap.String("topic", cfg.Audit.Topic),
				zap.Bool("backup_enabled", cfg.Audit.BackupEnabled))
			defer func() {
				logger.Info("Closing audit logger...")
				if err := auditLogger.Close(); err != nil {
					logger.Error("Failed to close audit logger", zap.Error(err))
				}
			}()
		}
	}

	// =========================================================================
	// 6. 初始化 Kafka Publisher
	// =========================================================================
	publisherCfg := publisher.PublisherConfig{
		Brokers:        cfg.Kafka.Brokers,
		RuleTopic:      cfg.Kafka.RuleTopic,
		AuditTopic:     cfg.Kafka.AuditTopic,
		SendTimeout:    cfg.Kafka.SendTimeout,
		PublishTimeout: cfg.Kafka.PublishTimeout,
		MaxRetries:     cfg.Kafka.MaxRetries,
		RetryBackoff:   cfg.Kafka.RetryBackoff,
		Compression:    cfg.Kafka.Compression,
		RequiredAcks:   cfg.Kafka.RequiredAcks,
	}

	kafkaPublisher, err := publisher.NewKafkaPublisherWithConfig(publisherCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Kafka publisher", zap.Error(err))
	}
	defer func() {
		logger.Info("Closing Kafka publisher...")
		if err := kafkaPublisher.Close(); err != nil {
			logger.Error("Failed to close Kafka publisher", zap.Error(err))
		}
	}()

	logger.Info("Kafka publisher initialized",
		zap.String("rule_topic", cfg.Kafka.RuleTopic),
		zap.Strings("brokers", cfg.Kafka.Brokers))

	// =========================================================================
	// 7. 初始化 Repository
	// =========================================================================
	ruleRepo := repository.NewRuleRepository(pgClient, logger)

	// =========================================================================
	// 8. 初始化 RBAC Checker
	// =========================================================================
	var rbacChecker *rbac.Checker
	if cfg.RBAC.Enabled {
		rbacChecker = rbac.NewChecker(logger)
		logger.Info("RBAC checker initialized")
	}

	// =========================================================================
	// 9. 初始化 Services
	// =========================================================================
	ruleServiceCfg := service.RuleServiceConfig{
		MaxRulesPerTenant:     10000,
		EnableCache:           cfg.Service.CacheEnabled,
		CacheTTL:              cfg.Service.CacheTTL,
		EnableAudit:           cfg.Audit.Enabled,
		KafkaPublishRetries:   cfg.Kafka.MaxRetries,
		KafkaPublishTimeout:   cfg.Kafka.PublishTimeout,
		EnableOutbox:          true, // ✅ 启用 Outbox 模式
		OutboxProcessInterval: 5 * time.Second,
	}

	// ✅ 修复：传递 pgClient.DB() 给 RuleService 以支持 Outbox
	ruleService := service.NewRuleServiceWithDeps(
		ruleRepo,
		kafkaPublisher,
		auditLogger,
		rbacChecker,
		redisClient,
		pgClient.DB(), // ✅ 传递 *sql.DB 用于 Outbox 操作
		logger,
		ruleServiceCfg,
	)

	// ✅ 注册 RuleService 优雅关闭
	defer func() {
		logger.Info("Stopping rule service...")
		ruleService.Stop()
	}()

	deploymentServiceCfg := service.DeploymentServiceConfig{
		MaxActiveDeploymentsPerTenant: 10,
		GrayTimeout:                   cfg.Deployment.MaxGrayDuration,
		RequireRollbackReason:         cfg.Deployment.RequireRollbackReason,
		EnableAutoRollback:            cfg.Deployment.EnableAutoRollback,
		AutoRollbackThreshold:         cfg.Deployment.AutoRollbackThreshold,
		EnableGrayValidation:          cfg.Deployment.EnableGrayValidation,
		MaxGrayDuration:               cfg.Deployment.MaxGrayDuration,
	}
	deploymentService := service.NewDeploymentServiceWithDeps(
		pgClient.DB(),
		kafkaPublisher,
		auditLogger,
		rbacChecker,
		logger,
		deploymentServiceCfg,
	)

	// =========================================================================
	// 10. 初始化健康检查器
	// =========================================================================
	healthChecker := health.NewChecker(logger)
	healthChecker.RegisterComponent(health.NewPostgresChecker(pgClient.DB()))
	healthChecker.RegisterComponent(health.NewKafkaHealthChecker(kafkaPublisher))
	if redisClient != nil {
		healthChecker.RegisterComponent(health.NewRedisChecker(redisClient))
	}

	// =========================================================================
	// 11. 初始化 API Handler
	// =========================================================================
	handlerCfg := api.HandlerConfig{
		EnableRBAC:       cfg.RBAC.Enabled,
		EnableAudit:      cfg.Audit.Enabled,
		MaxPageSize:      cfg.API.MaxPageSize,
		DefaultPageSize:  cfg.API.DefaultPageSize,
		RequestTimeout:   cfg.API.RequestTimeout,
		EnableRequestLog: cfg.API.EnableRequestLog,
		MaxRequestSize:   cfg.API.MaxRequestSize,
	}
	handler := api.NewHandler(ruleService, deploymentService, auditLogger, rbacChecker, logger, handlerCfg)

	// =========================================================================
	// 12. 创建 Router
	// =========================================================================
	r := mux.NewRouter()

	// 配置 CORS
	corsConfig := &httpx.CORSConfig{
		AllowedOrigins:   cfg.API.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-User-ID", "X-Request-ID", "X-Trace-ID", "X-Username", "X-Roles", "X-Permissions"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID"},
		AllowCredentials: true,
		MaxAge:           86400,
	}

	// 构建中间件链
	middlewareChain := httpx.NewChain(
		// 1. Recovery - 最外层，捕获 panic
		httpx.Recovery(logger),

		// 2. Request ID - 生成/传播请求ID
		httpx.RequestID(),

		// 3. Logging - 请求日志
		httpx.Logging(logger),

		// 4. CORS - 跨域处理
		httpx.CORS(corsConfig),

		// 5. Metrics - Prometheus 指标
		httpx.Metrics(serviceName),

		// 6. Timeout - 请求超时控制
		httpx.TimeoutWithConfig(int(cfg.API.RequestTimeout.Seconds()), nil),

		// 7. Tenant Extractor - 租户信息提取
		httpx.TenantExtractor(),
	)

	// 注册业务路由
	apiRouter := r.PathPrefix("/api/v1").Subrouter()
	handler.RegisterRoutes(apiRouter)

	// 注册健康检查路由
	r.HandleFunc("/healthz", healthChecker.LivenessHandler).Methods("GET")
	r.HandleFunc("/readyz", healthChecker.ReadinessHandler).Methods("GET")
	r.HandleFunc("/health", healthChecker.HealthHandler).Methods("GET")

	// 应用中间件
	finalHandler := middlewareChain.Then(r)

	// =========================================================================
	// 13. 启动 Metrics 服务器
	// =========================================================================
	var metricsServer *http.Server
	if cfg.Metrics.Enabled {
		metricsRouter := mux.NewRouter()
		metricsRouter.Handle("/metrics", promhttp.Handler())
		metricsRouter.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		metricsServer = &http.Server{
			Addr:         cfg.Metrics.ListenAddr,
			Handler:      metricsRouter,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		}

		go func() {
			logger.Info("Starting Metrics server", zap.String("addr", cfg.Metrics.ListenAddr))
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("Metrics server failed", zap.Error(err))
			}
		}()
	}

	// =========================================================================
	// 14. 启动 API 服务器
	// =========================================================================
	srv := &http.Server{
		Addr:         cfg.API.ListenAddr,
		Handler:      finalHandler,
		ReadTimeout:  cfg.API.ReadTimeout,
		WriteTimeout: cfg.API.WriteTimeout,
		IdleTimeout:  cfg.API.IdleTimeout,
	}

	go func() {
		logger.Info("Starting Rule Manager API server",
			zap.String("addr", cfg.API.ListenAddr),
			zap.Bool("rbac_enabled", cfg.RBAC.Enabled),
			zap.Bool("audit_enabled", cfg.Audit.Enabled),
			zap.Bool("outbox_enabled", true))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// =========================================================================
	// 15. 等待关闭信号
	// =========================================================================
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	logger.Info("Shutting down gracefully...")

	// =========================================================================
	// 16. 优雅关闭
	// =========================================================================
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// 标记为不健康，停止接收新请求
	healthChecker.SetReady(false)

	// 等待一段时间让负载均衡器移除此实例
	logger.Info("Waiting for load balancer to drain connections...")
	time.Sleep(5 * time.Second)

	// 关闭 API 服务器
	logger.Info("Shutting down API server...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("API server shutdown error", zap.Error(err))
	} else {
		logger.Info("API server shutdown complete")
	}

	// 关闭 Metrics 服务器
	if metricsServer != nil {
		logger.Info("Shutting down Metrics server...")
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Metrics server shutdown error", zap.Error(err))
		} else {
			logger.Info("Metrics server shutdown complete")
		}
	}

	// 停止 RuleService（停止 Outbox 处理器）
	logger.Info("Stopping RuleService (Outbox processor)...")
	ruleService.Stop()
	logger.Info("RuleService stopped")

	// 关闭审计日志（确保所有日志都已写入）
	if auditLogger != nil {
		logger.Info("Flushing audit logs...")
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := auditLogger.Flush(flushCtx); err != nil {
			logger.Error("Audit logger flush error", zap.Error(err))
		}
		flushCancel()

		logger.Info("Closing audit logger...")
		if err := auditLogger.Close(); err != nil {
			logger.Error("Audit logger close error", zap.Error(err))
		} else {
			logger.Info("Audit logger closed")
		}
	}

	// 关闭 Kafka Publisher（确保所有消息都已发送）
	logger.Info("Closing Kafka publisher...")
	if err := kafkaPublisher.Close(); err != nil {
		logger.Error("Kafka publisher close error", zap.Error(err))
	} else {
		logger.Info("Kafka publisher closed")
	}

	// 关闭 Redis
	if redisClient != nil {
		logger.Info("Closing Redis connection...")
		if err := redisClient.Close(); err != nil {
			logger.Error("Redis close error", zap.Error(err))
		} else {
			logger.Info("Redis connection closed")
		}
	}

	// 关闭 PostgreSQL
	logger.Info("Closing PostgreSQL connection...")
	if err := pgClient.Close(); err != nil {
		logger.Error("PostgreSQL close error", zap.Error(err))
	} else {
		logger.Info("PostgreSQL connection closed")
	}

	logger.Info("Shutdown complete")
}

// =============================================================================
// 辅助函数
// =============================================================================

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 获取整数环境变量
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

// getEnvBool 获取布尔环境变量
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

// getEnvDuration 获取时间环境变量
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/cmd/ingest-gateway/main.go
// 修复版 v3：
// 1. 修复问题 7：优化优雅关闭顺序，避免 Kafka 缓冲区数据丢失
// 2. 调整超时：gRPC 25s + Kafka 5s
// 3. 添加详细日志记录关键步骤
////////////////////////////////////////////////////////////////////////////////

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/dlq"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/metrics"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/queue"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/quota"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/server"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func main() {
	// 1. 初始化日志
	logger := initLogger()
	defer logger.Sync()

	logger.Info("Ingest Gateway starting...")

	// 2. 加载配置
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("grpc_addr", cfg.Server.GRPCAddr),
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.Bool("require_mtls", cfg.Auth.RequireMTLS),
		zap.Bool("allow_no_token", cfg.Auth.AllowNoToken),
		zap.String("default_tenant_id", cfg.Auth.DefaultTenantID),
		zap.Bool("metrics_enabled", cfg.Metrics.Enabled),
		zap.Bool("enable_dedup", cfg.Dedup.Enabled),
		zap.Bool("dedup_redis_enabled", cfg.Dedup.RedisEnabled))

	// 3. 初始化 Redis
	var rdb redis.UniversalClient
	if len(cfg.Redis.Addrs) > 0 && cfg.Redis.Addrs[0] != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:         cfg.Redis.Addrs[0],
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rdb.Ping(ctx).Err(); err != nil {
			logger.Warn("Failed to connect to Redis, will use fallback", zap.Error(err))
		} else {
			logger.Info("Connected to Redis")
		}
		cancel()
	}
	if rdb != nil {
		defer rdb.Close()
	}

	// 4. 初始化 PostgreSQL（用于 Token 验证降级）
	var pgDB *sql.DB
	var pgValidator *auth.PGTokenValidator

	pgDSN := os.Getenv("POSTGRES_DSN")
	if pgDSN != "" {
		pgDB, err = sql.Open("postgres", pgDSN)
		if err != nil {
			logger.Warn("Failed to connect to PostgreSQL", zap.Error(err))
		} else {
			pgDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
			pgDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
			pgDB.SetConnMaxLifetime(cfg.Postgres.ConnLifetime)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := pgDB.PingContext(ctx); err != nil {
				logger.Warn("Failed to ping PostgreSQL", zap.Error(err))
			} else {
				logger.Info("Connected to PostgreSQL (fallback)")
				pgValidator = auth.NewPGTokenValidator(pgDB, auth.PGTokenValidatorConfig{
					CacheTTL: time.Minute,
				}, logger)
			}
			cancel()
		}
	}
	if pgDB != nil {
		defer pgDB.Close()
	}

	// 5. 初始化 Token 缓存
	tokenCache := auth.NewTokenCache(rdb, logger, auth.TokenCacheConfig{
		TTL:            cfg.Auth.TokenTTL,
		Prefix:         "token:",
		LocalTTL:       cfg.Auth.LocalCacheTTL,
		LocalCacheSize: cfg.Auth.LocalCacheSize,
	})

	if pgValidator != nil {
		tokenCache.SetPGValidator(pgValidator)
	}

	// 6. 初始化限流器
	limiter := quota.NewLimiter(rdb, quota.LimiterConfig{
		RedisEnabled:         cfg.Quota.RedisEnabled,
		RedisPrefix:          cfg.Quota.RedisPrefix,
		GlobalRPS:            cfg.Quota.GlobalRPS,
		GlobalBurst:          cfg.Quota.GlobalBurst,
		TenantRPS:            cfg.Quota.TenantRPS,
		TenantBurst:          cfg.Quota.TenantBurst,
		ProbeRPS:             cfg.Quota.ProbeRPS,
		ProbeBurst:           cfg.Quota.ProbeBurst,
		LocalFallbackEnabled: cfg.Quota.LocalFallbackEnabled,
	}, logger)
	defer limiter.Close()

	// 7. 初始化 Metrics
	m := metrics.NewMetrics(logger)
	if cfg.Metrics.Enabled {
		go func() {
			logger.Info("Starting metrics server", zap.String("addr", cfg.Metrics.ListenAddr))
			m.StartServer(cfg.Metrics.ListenAddr)
		}()
	}

	// 8. 初始化 Kafka Producer
	producerCfg := queue.ProducerConfig{
		Brokers:           cfg.Kafka.Brokers,
		FlowTopic:         cfg.Kafka.FlowTopic,
		PcapIndexTopic:    cfg.Kafka.PcapTopic,
		SessionTopic:      "session.events.v1",
		BatchSize:         cfg.Kafka.BatchSize,
		BatchTimeout:      cfg.Kafka.BatchTimeout,
		Compression:       cfg.Kafka.Compression,
		RequiredAcks:      cfg.Kafka.RequiredAcks,
		MaxRetries:        cfg.Kafka.MaxRetries,
		EnableIdempotence: cfg.Kafka.EnableIdempotence,
	}
	producer, err := queue.NewProducer(producerCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Kafka producer", zap.Error(err))
	}
	// ✅ 注意：不在这里 defer Close()，因为需要在优雅关闭中显式控制顺序
	logger.Info("Kafka producer initialized")

	// 9. 初始化 DLQ Producer
	dlqConfig := dlq.DefaultConfig(cfg.Kafka.Brokers)
	dlqConfig.EnableFallback = true
	dlqConfig.FallbackDir = getEnvOrDefault("DLQ_FALLBACK_DIR", "/var/log/ingest-gateway/dlq-fallback")
	dlqConfig.MaxFallbackSize = 100 * 1024 * 1024 // 100MB

	dlqProducer := dlq.NewProducer(dlqConfig, logger)
	// ✅ 注意：不在这里 defer Close()，因为需要在优雅关闭中显式控制顺序

	// 启动降级文件重放任务
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	go dlqProducer.StartFallbackReplay(mainCtx, 5*time.Minute)

	// 10. 初始化去重器
	var deduper *dedup.Deduplicator
	if cfg.Dedup.Enabled {
		dedupCfg := dedup.DefaultDedupConfig()
		dedupCfg.LocalCacheSize = cfg.Dedup.LocalCacheSize
		dedupCfg.LocalTTL = cfg.Dedup.LocalTTL
		dedupCfg.RedisEnabled = cfg.Dedup.RedisEnabled
		dedupCfg.RedisPrefix = cfg.Dedup.RedisPrefix
		dedupCfg.RedisTTL = cfg.Dedup.RedisTTL

		deduper, err = dedup.NewDeduplicator(dedupCfg, rdb, logger)
		if err != nil {
			logger.Warn("Failed to create deduplicator", zap.Error(err))
		} else {
			logger.Info("Deduplicator initialized",
				zap.Bool("redis_enabled", cfg.Dedup.RedisEnabled),
				zap.Int("local_cache_size", cfg.Dedup.LocalCacheSize))
		}
	}

	// 11. 初始化探针配置管理器
	var configManager *config.ProbeConfigManager
	if rdb != nil {
		configManager = config.NewProbeConfigManager(rdb, cfg.Probe, logger)
		defer configManager.Close()
		logger.Info("Probe config manager initialized")
	}

	// 12. 初始化 gRPC Handler
	handlerConfig := server.HandlerConfig{
		MaxBatchSize:       cfg.Handler.MaxBatchSize,
		MaxEventSize:       cfg.Handler.MaxEventSize,
		StreamBufferSize:   cfg.Handler.StreamBufferSize,
		HeartbeatInterval:  cfg.Handler.HeartbeatInterval,
		EnableDLQ:          cfg.Handler.EnableDLQ,
		EnableDedup:        cfg.Dedup.Enabled,
		ProbeStatusTimeout: cfg.Handler.ProbeStatusTimeout,
	}

	handler := server.NewIngestHandlerWithConfig(logger, producer, dlqProducer, m, handlerConfig)

	if deduper != nil {
		handler.SetDeduplicator(deduper)
	}

	if configManager != nil {
		handler.SetConfigManager(configManager)
	}

	handler.SetDLQProducer(dlqProducer)

	handler.StartProbeStatusCleaner(mainCtx)

	// 13. 配置 gRPC Server
	var opts []grpc.ServerOption

	// TLS 配置
	if cfg.Auth.RequireMTLS && cfg.Server.TLSCertFile != "" {
		tlsConfig, err := loadTLSConfig(cfg.Server)
		if err != nil {
			logger.Fatal("Failed to load TLS config", zap.Error(err))
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		logger.Info("mTLS enabled")
	}

	// Panic Recovery 中间件
	recoveryOpts := []grpc_recovery.Option{
		grpc_recovery.WithRecoveryHandler(func(p interface{}) error {
			logger.Error("Panic recovered in gRPC handler",
				zap.Any("panic", p),
				zap.Stack("stack"))
			return status.Errorf(codes.Internal, "internal server error")
		}),
	}

	// 认证拦截器
	interceptorConfig := auth.InterceptorConfig{
		RequireMTLS:     cfg.Auth.RequireMTLS,
		AllowNoToken:    cfg.Auth.AllowNoToken,
		EnableRateLimit: true,
		GlobalRPS:       cfg.Quota.GlobalRPS,
		GlobalBurst:     cfg.Quota.GlobalBurst,
		TenantRPS:       cfg.Quota.TenantRPS,
		DefaultTenantID: cfg.Auth.DefaultTenantID,
		RequireScopes:   cfg.Auth.RequireScopes,
		RequiredScopes:  cfg.Auth.RequiredScopes,
		EnableAuditLog:  cfg.Auth.EnableAudit,
		EnableProbeRBAC: cfg.Auth.EnableProbeRBAC,
	}
	interceptor := auth.NewInterceptor(logger, tokenCache, interceptorConfig)
	interceptor.SetLimiter(limiter)

	if cfg.Audit.Enabled {
		interceptor.SetAuditCallback(func(ctx context.Context, event *auth.AuditEvent) {
			logger.Info("Audit event",
				zap.String("type", event.EventType),
				zap.String("tenant_id", event.TenantID),
				zap.String("probe_id", event.ProbeID),
				zap.String("method", event.Method),
				zap.String("client_ip", event.ClientIP),
				zap.String("error", event.Error),
				zap.Strings("scopes", event.Scopes))
		})
	}

	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			grpc_recovery.UnaryServerInterceptor(recoveryOpts...),
			interceptor.UnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			grpc_recovery.StreamServerInterceptor(recoveryOpts...),
			interceptor.StreamInterceptor,
		),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     cfg.Server.MaxConnectionIdle,
			MaxConnectionAge:      cfg.Server.MaxConnectionAge,
			MaxConnectionAgeGrace: cfg.Server.MaxConnectionAgeGrace,
			Time:                  cfg.Server.KeepaliveTime,
			Timeout:               cfg.Server.KeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(cfg.Server.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.Server.MaxSendMsgSize),
	)

	// 14. 创建 gRPC Server
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterIngestServiceServer(grpcServer, handler)

	// 注册健康检查
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("ingest.IngestService", grpc_health_v1.HealthCheckResponse_SERVING)

	// 注册反射
	reflection.Register(grpcServer)

	// 15. 启动 gRPC Server
	listener, err := net.Listen("tcp", cfg.Server.GRPCAddr)
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	logger.Info("Starting gRPC server", zap.String("addr", cfg.Server.GRPCAddr))

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal("gRPC server failed", zap.Error(err))
		}
	}()

	// 16. 启动 HTTP 健康检查端点
	go startHealthEndpoint(cfg.Metrics.ListenAddr, producer, tokenCache, logger)

	// ==================== 优雅关闭流程（修复版） ====================

	// 17. 等待关闭信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal, starting graceful shutdown...",
		zap.String("signal", sig.String()))

	// ✅ 步骤 1：设置健康检查为 NOT_SERVING（K8s 停止转发新流量）
	logger.Info("Step 1/7: Marking service as NOT_SERVING")
	healthServer.SetServingStatus("ingest.IngestService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// ✅ 步骤 2：等待 1 秒，确保 K8s/负载均衡器停止转发新请求
	logger.Info("Step 2/7: Waiting for load balancer to stop routing traffic (1s)")
	time.Sleep(1 * time.Second)

	// ✅ 步骤 3：取消后台任务（DLQ 回放、探针状态清理等）
	logger.Info("Step 3/7: Stopping background tasks")
	mainCancel()

	// ✅ 步骤 4：优雅关闭 gRPC Server（等待进行中的 RPC 完成，最多 25 秒）
	logger.Info("Step 4/7: Gracefully stopping gRPC server (timeout: 25s)")
	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("gRPC server stopped gracefully")
	case <-time.After(25 * time.Second):
		logger.Warn("gRPC graceful shutdown timed out (25s), forcing stop")
		grpcServer.Stop()
	}

	// ✅ 步骤 5：刷新 Kafka Producer 缓冲区（确保所有消息已发送）
	logger.Info("Step 5/7: Flushing Kafka producer buffers...")
	kafkaCloseStart := time.Now()
	if err := producer.Close(); err != nil {
		logger.Error("Failed to close Kafka producer", zap.Error(err))
	} else {
		logger.Info("Kafka producer closed successfully, all buffered messages flushed",
			zap.Duration("duration", time.Since(kafkaCloseStart)))
	}

	// ✅ 步骤 6：关闭 DLQ Producer
	logger.Info("Step 6/7: Closing DLQ producer...")
	if err := dlqProducer.Close(); err != nil {
		logger.Error("Failed to close DLQ producer", zap.Error(err))
	} else {
		logger.Info("DLQ producer closed")
	}

	// 打印 DLQ 降级文件统计
	fileCount, fileSize, err := dlqProducer.GetFallbackStats()
	if err == nil && fileCount > 0 {
		logger.Warn("DLQ fallback files pending (require manual replay)",
			zap.Int("file_count", fileCount),
			zap.Int64("total_size_bytes", fileSize),
			zap.String("directory", dlqConfig.FallbackDir))
	}

	// ✅ 步骤 7：关闭去重器
	if deduper != nil {
		logger.Info("Step 7/7: Closing deduplicator...")
		deduper.Close()
		logger.Info("Deduplicator closed")
	}

	// 打印最终统计
	stats := handler.GetStats()
	logger.Info("Final statistics",
		zap.Int64("total_events_received", stats.TotalEventsReceived),
		zap.Int64("total_events_accepted", stats.TotalEventsAccepted),
		zap.Int64("total_events_rejected", stats.TotalEventsRejected),
		zap.Int64("total_events_dedupe", stats.TotalEventsDedupe),
		zap.Int("active_probes", stats.ActiveProbes),
		zap.Bool("dedup_enabled", stats.DedupEnabled))

	logger.Info("Ingest Gateway stopped gracefully")
}

// initLogger 初始化日志
func initLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if level := os.Getenv("LOG_LEVEL"); level != "" {
		var zapLevel zapcore.Level
		if err := zapLevel.UnmarshalText([]byte(level)); err == nil {
			config.Level.SetLevel(zapLevel)
		}
	}

	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	return logger
}

// loadTLSConfig 加载 TLS 配置
func loadTLSConfig(cfg config.ServerConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	caCert, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to add CA cert to pool")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// getEnvOrDefault 获取环境变量或默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// startHealthEndpoint 启动 HTTP 健康检查端点
func startHealthEndpoint(metricsAddr string, producer *queue.Producer, tokenCache *auth.TokenCache, logger *zap.Logger) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 检查关键组件
		healthy := true
		status := make(map[string]string)

		// Kafka Producer 健康
		if producer.Healthy() {
			status["kafka"] = "healthy"
		} else {
			status["kafka"] = "unhealthy"
			healthy = false
		}

		// Token Cache 健康
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if tokenCache.Healthy(ctx) {
			status["auth"] = "healthy"
		} else {
			status["auth"] = "degraded"
		}

		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		statusStr := "healthy"
		if !healthy {
			statusStr = "unhealthy"
		}
		fmt.Fprintf(w, `{"status":"%s","components":{"kafka":"%s","auth":"%s"}}`,
			statusStr, status["kafka"], status["auth"])
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if producer.Healthy() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ready"}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"not_ready"}`))
		}
	})

	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"alive"}`))
	})

	addr := ":9092"
	if metricsAddr != "" && metricsAddr != ":9091" {
		return
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logger.Info("Starting health endpoint", zap.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Health endpoint failed", zap.Error(err))
	}
}

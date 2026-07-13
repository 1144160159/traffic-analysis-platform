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

	logger := initLogger()
	defer logger.Sync()

	logger.Info("Ingest Gateway starting...")

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Config loaded from environment and config.env")

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logConfigSummary(logger, cfg)

	rdb := initRedis(cfg.Redis, logger)
	if rdb != nil {
		defer rdb.Close()
	}

	pgDB, pgValidator := initPostgreSQL(cfg.Postgres, logger)
	if pgDB != nil {
		defer pgDB.Close()
	}

	tokenCache := initTokenCache(rdb, pgValidator, cfg.Auth, logger)

	limiter := initLimiter(rdb, cfg.Quota, logger)
	defer limiter.Close()

	m := initMetrics(cfg.Metrics, logger)

	producer := initKafkaProducer(cfg.Kafka, logger)

	dlqProducer := initDLQProducer(cfg, logger)

	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()
	go dlqProducer.StartFallbackReplay(mainCtx, config.DefaultDLQReplayInterval)

	deduper := initDeduplicator(cfg.Dedup, rdb, logger)

	configManager := initProbeConfigManager(rdb, cfg.Probe, logger)
	if configManager != nil {
		defer configManager.Close()
	}

	handler := initHandler(producer, dlqProducer, deduper, configManager, m, cfg.Handler, logger)
	handler.StartProbeStatusCleaner(mainCtx)

	grpcServer := createGRPCServer(cfg, tokenCache, limiter, handler, logger)
	healthServer := registerHealthCheck(grpcServer)

	listener := startGRPCServer(grpcServer, cfg.Server.GRPCAddr, logger)
	defer listener.Close()

	replayManager := initDLQReplayManager(dlqProducer, rdb, logger)
	go startHealthEndpoint(producer, tokenCache, replayManager, cfg.JWT, logger)

	gracefulShutdown(
		logger,
		healthServer,
		grpcServer,
		mainCancel,
		producer,
		dlqProducer,
		deduper,
		handler,
		cfg.Kafka,
	)
}

func initLogger() *zap.Logger {
	zapConfig := zap.NewProductionConfig()
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if level := os.Getenv(config.EnvLogLevel); level != "" {
		var zapLevel zapcore.Level
		if err := zapLevel.UnmarshalText([]byte(level)); err == nil {
			zapConfig.Level.SetLevel(zapLevel)
		}
	}

	logger, err := zapConfig.Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	return logger
}

func logConfigSummary(logger *zap.Logger, cfg *config.Config) {
	logger.Info("Configuration loaded",
		zap.String("grpc_addr", cfg.Server.GRPCAddr),
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.String("flow_topic", cfg.Kafka.FlowTopic),
		zap.String("session_topic", cfg.Kafka.SessionTopic),
		zap.String("pcap_topic", cfg.Kafka.PcapTopic),
		zap.Bool("require_mtls", cfg.Auth.RequireMTLS),
		zap.Bool("allow_no_token", cfg.Auth.AllowNoToken),
		zap.String("default_tenant_id", cfg.Auth.DefaultTenantID),
		zap.Bool("metrics_enabled", cfg.Metrics.Enabled),
		zap.Bool("enable_dedup", cfg.Dedup.Enabled),
		zap.Bool("dedup_redis_enabled", cfg.Dedup.RedisEnabled))
}

func initRedis(cfg config.RedisConfig, logger *zap.Logger) redis.UniversalClient {
	if len(cfg.SentinelAddrs) > 0 && cfg.SentinelMaster != "" {
		rdb := redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    cfg.SentinelMaster,
			SentinelAddrs: cfg.SentinelAddrs,
			Password:      cfg.Password,
			DB:            cfg.DB,
			DialTimeout:   cfg.DialTimeout,
			ReadTimeout:   cfg.ReadTimeout,
			WriteTimeout:  cfg.WriteTimeout,
			PoolSize:      cfg.PoolSize,
			MinIdleConns:  cfg.MinIdleConns,
		})

		ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
		defer cancel()

		if err := rdb.Ping(ctx).Err(); err != nil {
			logger.Warn("Failed to connect to Redis Sentinel master, will use fallback", zap.Error(err))
			return nil
		}

		logger.Info("Connected to Redis Sentinel",
			zap.Strings("sentinels", cfg.SentinelAddrs),
			zap.String("master", cfg.SentinelMaster))
		return rdb
	}

	if len(cfg.Addrs) == 0 || cfg.Addrs[0] == "" {
		logger.Warn("Redis not configured, some features will be disabled")
		return nil
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addrs[0],
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("Failed to connect to Redis, will use fallback", zap.Error(err))
		return nil
	}

	logger.Info("Connected to Redis", zap.String("addr", cfg.Addrs[0]))
	return rdb
}

func initPostgreSQL(cfg config.PostgresConfig, logger *zap.Logger) (*sql.DB, *auth.PGTokenValidator) {
	pgDSN := cfg.ConnectionString()
	if pgDSN == "" {
		logger.Info("PostgreSQL not configured, token validation will rely on Redis only")
		return nil, nil
	}

	pgDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		logger.Warn("Failed to connect to PostgreSQL", zap.Error(err))
		return nil, nil
	}

	pgDB.SetMaxOpenConns(cfg.MaxOpenConns)
	pgDB.SetMaxIdleConns(cfg.MaxIdleConns)
	pgDB.SetConnMaxLifetime(cfg.ConnLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
	defer cancel()

	if err := pgDB.PingContext(ctx); err != nil {
		logger.Warn("Failed to ping PostgreSQL", zap.Error(err))
		pgDB.Close()
		return nil, nil
	}

	logger.Info("Connected to PostgreSQL (fallback)")

	pgValidator := auth.NewPGTokenValidator(pgDB, auth.PGTokenValidatorConfig{
		CacheTTL: time.Minute,
	}, logger)

	return pgDB, pgValidator
}

func initTokenCache(
	rdb redis.UniversalClient,
	pgValidator *auth.PGTokenValidator,
	cfg config.AuthConfig,
	logger *zap.Logger,
) *auth.TokenCache {
	tokenCache := auth.NewTokenCache(rdb, logger, auth.TokenCacheConfig{
		TTL:            cfg.TokenTTL,
		Prefix:         config.RedisTokenPrefix,
		LocalTTL:       cfg.LocalCacheTTL,
		LocalCacheSize: cfg.LocalCacheSize,
	})

	if pgValidator != nil {
		tokenCache.SetPGValidator(pgValidator)
	}

	logger.Info("Token cache initialized",
		zap.Duration("ttl", cfg.TokenTTL),
		zap.Bool("pg_fallback", pgValidator != nil))

	return tokenCache
}

func initLimiter(rdb redis.UniversalClient, cfg config.QuotaConfig, logger *zap.Logger) *quota.Limiter {
	return quota.NewLimiter(rdb, quota.LimiterConfig{
		RedisEnabled:         cfg.RedisEnabled,
		RedisPrefix:          cfg.RedisPrefix,
		GlobalRPS:            cfg.GlobalRPS,
		GlobalBurst:          cfg.GlobalBurst,
		TenantRPS:            cfg.TenantRPS,
		TenantBurst:          cfg.TenantBurst,
		ProbeRPS:             cfg.ProbeRPS,
		ProbeBurst:           cfg.ProbeBurst,
		LocalFallbackEnabled: cfg.LocalFallbackEnabled,
	}, logger)
}

func initMetrics(cfg config.MetricsConfig, logger *zap.Logger) *metrics.Metrics {
	m := metrics.NewMetrics(logger)
	if cfg.Enabled {
		go m.StartServer(cfg.ListenAddr)
		logger.Info("Metrics server starting", zap.String("addr", cfg.ListenAddr))
	}
	return m
}

func initKafkaProducer(cfg config.KafkaConfig, logger *zap.Logger) *queue.Producer {
	producerCfg := queue.ProducerConfig{
		Brokers:           cfg.Brokers,
		FlowTopic:         cfg.FlowTopic,
		PcapIndexTopic:    cfg.PcapTopic,
		SessionTopic:      cfg.SessionTopic,
		BatchSize:         cfg.BatchSize,
		BatchTimeout:      cfg.BatchTimeout,
		Compression:       cfg.Compression,
		RequiredAcks:      cfg.RequiredAcks,
		MaxRetries:        cfg.MaxRetries,
		EnableIdempotence: cfg.EnableIdempotence,
		Security:          cfg.Security,
	}

	producer, err := queue.NewProducer(producerCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Kafka producer", zap.Error(err))
	}

	logger.Info("Kafka producer initialized",
		zap.Strings("topics", []string{cfg.FlowTopic, cfg.SessionTopic, cfg.PcapTopic}))

	return producer
}

func initDLQProducer(cfg *config.Config, logger *zap.Logger) *dlq.Producer {
	dlqConfig := dlq.DefaultConfig(cfg.Kafka.Brokers)
	dlqConfig.DLQTopic = cfg.Kafka.DLQTopic
	dlqConfig.FlowTopic = cfg.Kafka.FlowTopic
	dlqConfig.SessionTopic = cfg.Kafka.SessionTopic
	dlqConfig.PcapTopic = cfg.Kafka.PcapTopic
	dlqConfig.EnableFallback = true
	dlqConfig.Security = cfg.Kafka.Security

	fallbackDir := os.Getenv(config.EnvDLQFallbackDir)
	if fallbackDir == "" {
		fallbackDir = config.DefaultDLQFallbackDir
	}
	dlqConfig.FallbackDir = fallbackDir

	dlqProducer, err := dlq.NewProducer(dlqConfig, logger)
	if err != nil {
		logger.Fatal("Failed to create DLQ producer", zap.Error(err))
	}

	logger.Info("DLQ producer initialized",
		zap.String("dlq_topic", dlqConfig.DLQTopic),
		zap.String("fallback_dir", fallbackDir))

	return dlqProducer
}

func initDLQReplayManager(dlqProducer *dlq.Producer, rdb redis.UniversalClient, logger *zap.Logger) *dlq.ReplayManager {
	var store dlq.ReplayIdempotencyStore
	if rdb != nil {
		store = dlq.NewRedisReplayIdempotencyStore(rdb, "", config.DefaultDLQReplayIdempotencyTTL, logger)
		logger.Info("DLQ replay idempotency store initialized",
			zap.String("backend", "redis"),
			zap.Duration("ttl", config.DefaultDLQReplayIdempotencyTTL))
	} else {
		store = dlq.NewMemoryReplayIdempotencyStore()
		logger.Warn("DLQ replay idempotency store falling back to memory; duplicate suppression is process-local")
	}
	return dlq.NewReplayManager(dlqProducer, store, logger)
}

func initDeduplicator(cfg config.DedupConfig, rdb redis.UniversalClient, logger *zap.Logger) *dedup.Deduplicator {
	if !cfg.Enabled {
		logger.Info("Deduplication disabled")
		return nil
	}

	deduper, err := dedup.NewDeduplicator(dedup.DedupConfig{
		LocalCacheSize: cfg.LocalCacheSize,
		LocalTTL:       cfg.LocalTTL,
		RedisEnabled:   cfg.RedisEnabled,
		RedisPrefix:    cfg.RedisPrefix,
		RedisTTL:       cfg.RedisTTL,
	}, rdb, logger)

	if err != nil {
		logger.Warn("Failed to create deduplicator", zap.Error(err))
		return nil
	}

	logger.Info("Deduplicator initialized",
		zap.Bool("redis_enabled", cfg.RedisEnabled),
		zap.Int("local_cache_size", cfg.LocalCacheSize))

	return deduper
}

func initProbeConfigManager(rdb redis.UniversalClient, cfg config.ProbeConfig, logger *zap.Logger) *config.ProbeConfigManager {
	if rdb == nil {
		logger.Info("Probe config manager disabled (no Redis)")
		return nil
	}

	configManager := config.NewProbeConfigManager(rdb, cfg, logger)
	logger.Info("Probe config manager initialized")

	return configManager
}

func initHandler(
	producer *queue.Producer,
	dlqProducer *dlq.Producer,
	deduper *dedup.Deduplicator,
	configManager *config.ProbeConfigManager,
	m *metrics.Metrics,
	cfg config.HandlerConfig,
	logger *zap.Logger,
) *server.IngestHandler {
	handler := server.NewIngestHandlerWithConfig(logger, producer, dlqProducer, m, server.HandlerConfig{
		MaxBatchSize:       cfg.MaxBatchSize,
		MaxEventSize:       cfg.MaxEventSize,
		StreamBufferSize:   cfg.StreamBufferSize,
		HeartbeatInterval:  cfg.HeartbeatInterval,
		EnableDLQ:          cfg.EnableDLQ,
		EnableDedup:        cfg.EnableDedup,
		ProbeStatusTimeout: cfg.ProbeStatusTimeout,
	})

	if deduper != nil {
		handler.SetDeduplicator(deduper)
	}

	if configManager != nil {
		handler.SetConfigManager(configManager)
	}

	handler.SetDLQProducer(dlqProducer)

	logger.Info("Handler initialized",
		zap.Int("max_batch_size", cfg.MaxBatchSize),
		zap.Bool("enable_dedup", cfg.EnableDedup))

	return handler
}

func createGRPCServer(
	cfg *config.Config,
	tokenCache *auth.TokenCache,
	limiter *quota.Limiter,
	handler *server.IngestHandler,
	logger *zap.Logger,
) *grpc.Server {
	var opts []grpc.ServerOption

	if cfg.Auth.RequireMTLS && cfg.Server.TLSCertFile != "" {
		tlsConfig, err := loadTLSConfig(cfg.Server)
		if err != nil {
			logger.Fatal("Failed to load TLS config", zap.Error(err))
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		logger.Info("mTLS enabled")
	}

	recoveryOpts := []grpc_recovery.Option{
		grpc_recovery.WithRecoveryHandler(func(p interface{}) error {
			logger.Error("Panic recovered in gRPC handler",
				zap.Any("panic", p),
				zap.Stack("stack"))
			return status.Errorf(codes.Internal, "internal server error")
		}),
	}

	interceptor := createAuthInterceptor(cfg, tokenCache, limiter, logger)

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
			MinTime:             config.MinKeepaliveTime,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(cfg.Server.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.Server.MaxSendMsgSize),
	)

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterIngestServiceServer(grpcServer, handler)

	logger.Info("gRPC server configured")
	return grpcServer
}

func createAuthInterceptor(
	cfg *config.Config,
	tokenCache *auth.TokenCache,
	limiter *quota.Limiter,
	logger *zap.Logger,
) *auth.Interceptor {
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

	return interceptor
}

func registerHealthCheck(grpcServer *grpc.Server) *health.Server {
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("ingest.IngestService", grpc_health_v1.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	return healthServer
}

func startGRPCServer(grpcServer *grpc.Server, addr string, logger *zap.Logger) net.Listener {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	logger.Info("Starting gRPC server", zap.String("addr", addr))

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal("gRPC server failed", zap.Error(err))
		}
	}()

	return listener
}

func gracefulShutdown(
	logger *zap.Logger,
	healthServer *health.Server,
	grpcServer *grpc.Server,
	mainCancel context.CancelFunc,
	producer *queue.Producer,
	dlqProducer *dlq.Producer,
	deduper *dedup.Deduplicator,
	handler *server.IngestHandler,
	kafkaCfg config.KafkaConfig,
) {

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal, starting graceful shutdown...",
		zap.String("signal", sig.String()))

	logger.Info("Step 1/7: Marking service as NOT_SERVING")
	healthServer.SetServingStatus("ingest.IngestService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	logger.Info("Step 2/7: Waiting for load balancer to stop routing traffic")
	time.Sleep(1 * time.Second)

	logger.Info("Step 3/7: Stopping background tasks")
	mainCancel()

	logger.Info("Step 4/7: Gracefully stopping gRPC server",
		zap.Duration("timeout", config.GracefulShutdownTimeout))
	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("gRPC server stopped gracefully")
	case <-time.After(config.GracefulShutdownTimeout):
		logger.Warn("gRPC graceful shutdown timed out, forcing stop")
		grpcServer.Stop()
	}

	logger.Info("Step 5/7: Flushing Kafka producer buffers...",
		zap.Duration("timeout", config.KafkaFlushTimeout))
	kafkaCloseStart := time.Now()
	if err := producer.Close(); err != nil {
		logger.Error("Failed to close Kafka producer", zap.Error(err))
	} else {
		logger.Info("Kafka producer closed successfully",
			zap.Duration("duration", time.Since(kafkaCloseStart)))
	}

	logger.Info("Step 6/7: Closing DLQ producer...")
	if err := dlqProducer.Close(); err != nil {
		logger.Error("Failed to close DLQ producer", zap.Error(err))
	} else {
		logger.Info("DLQ producer closed")
	}

	fileCount, fileSize, err := dlqProducer.GetFallbackStats()
	if err == nil && fileCount > 0 {
		logger.Warn("DLQ fallback files pending (require manual replay)",
			zap.Int("file_count", fileCount),
			zap.Int64("total_size_bytes", fileSize))
	}

	if deduper != nil {
		logger.Info("Step 7/7: Closing deduplicator...")
		deduper.Close()
		logger.Info("Deduplicator closed")
	}

	stats := handler.GetStats()
	logger.Info("Final statistics",
		zap.Int64("total_events_received", stats.TotalEventsReceived),
		zap.Int64("total_events_accepted", stats.TotalEventsAccepted),
		zap.Int64("total_events_rejected", stats.TotalEventsRejected),
		zap.Int64("total_events_dedupe", stats.TotalEventsDedupe),
		zap.Int("active_probes", stats.ActiveProbes))

	logger.Info("Ingest Gateway stopped gracefully")
}

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

func startHealthEndpoint(producer *queue.Producer, tokenCache *auth.TokenCache, replayManager *dlq.ReplayManager, jwtConfig config.JWTConfig, logger *zap.Logger) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		healthy := true
		status := make(map[string]string)

		if producer.Healthy() {
			status["kafka"] = "healthy"
		} else {
			status["kafka"] = "unhealthy"
			healthy = false
		}

		ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
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

	dlq.NewReplayHTTPHandler(replayManager, auth.NewReplayTokenValidator(tokenCache, jwtConfig, logger), logger).Register(mux)

	healthAddr := os.Getenv(config.EnvHealthAddr)
	if healthAddr == "" {
		healthAddr = config.DefaultHealthAddr
	}

	server := &http.Server{
		Addr:         healthAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logger.Info("Starting health endpoint", zap.String("addr", healthAddr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Health endpoint failed", zap.Error(err))
	}
}

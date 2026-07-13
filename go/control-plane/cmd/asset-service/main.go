package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/consumer"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func main() {
	// =========================================================================
	// 阶段1：初始化日志
	// =========================================================================
	logCfg := logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Format:      getEnv("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     "asset-service",
		Version:     getEnv("SERVICE_VERSION", "1.0.0"),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logging.Sync(logger)

	logger.Info("Starting Asset Service",
		zap.String("version", logCfg.Version),
		zap.String("environment", logCfg.Environment))

	// =========================================================================
	// 阶段2：加载配置
	// =========================================================================
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.Int("grpc_port", cfg.Server.GRPCPort),
		zap.Int("http_port", cfg.Server.HTTPPort))

	// =========================================================================
	// 阶段3：连接 PostgreSQL
	// =========================================================================
	pgDB, err := sql.Open("postgres", cfg.Postgres.DSN())
	if err != nil {
		logger.Fatal("Failed to open PostgreSQL", zap.Error(err))
	}
	defer pgDB.Close()

	pgDB.SetMaxOpenConns(20)
	pgDB.SetMaxIdleConns(5)
	pgDB.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pgDB.PingContext(ctx); err != nil {
		logger.Fatal("Failed to ping PostgreSQL", zap.Error(err))
	}
	logger.Info("Connected to PostgreSQL")

	// =========================================================================
	// 阶段4：初始化 Repository
	// =========================================================================
	assetRepo, err := repository.NewAssetRepository(pgDB, logger)
	if err != nil {
		logger.Fatal("Failed to initialize asset repository", zap.Error(err))
	}

	// 初始化数据库 Schema
	schemaCtx, schemaCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer schemaCancel()
	if err := assetRepo.InitSchema(schemaCtx); err != nil {
		logger.Warn("Failed to init asset schema (may already exist)", zap.Error(err))
	} else {
		logger.Info("Asset schema initialized")
	}

	// =========================================================================
	// 阶段5：初始化 Service + Handler
	// =========================================================================
	assetSvc := service.New(cfg, assetRepo, logger)
	assetHandler := api.NewAssetHandler(assetSvc, assetRepo, logger)

	logger.Info("Asset service initialized")

	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	assetSvc.StartDiscoveryScheduler(consumerCtx)
	var bindingConsumer *consumer.BindingConsumer
	if cfg.Kafka.Enabled {
		bc, err := consumer.NewBindingConsumer(cfg.Kafka, assetSvc, logger)
		if err != nil {
			logger.Fatal("Failed to initialize asset binding consumer", zap.Error(err))
		}
		bindingConsumer = bc
		go bindingConsumer.Run(consumerCtx)
		logger.Info("Asset binding Kafka consumer enabled",
			zap.String("topic", cfg.Kafka.Topic),
			zap.String("group_id", cfg.Kafka.GroupID))
	} else {
		logger.Info("Asset binding Kafka consumer disabled")
	}

	// =========================================================================
	// 阶段6：启动 gRPC Server
	// =========================================================================
	grpcAddr := fmt.Sprintf(":%d", cfg.Server.GRPCPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Fatal("Failed to listen on gRPC port", zap.String("addr", grpcAddr), zap.Error(err))
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingUnaryInterceptor(logger),
			recoveryUnaryInterceptor(logger),
		),
	)

	// 注册 AssetService
	pb.RegisterAssetServiceServer(grpcServer, assetHandler)

	// 注册 gRPC Health Check
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("asset-service", grpc_health_v1.HealthCheckResponse_SERVING)

	// 注册 gRPC Reflection（开发调试用）
	reflection.Register(grpcServer)

	go func() {
		logger.Info("gRPC server listening", zap.String("addr", grpcAddr))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal("gRPC server failed", zap.Error(err))
		}
	}()

	// =========================================================================
	// 阶段7：启动 HTTP Health Check + Metrics
	// =========================================================================
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"asset-service"}`))
	})
	httpMux.HandleFunc("/health/readiness", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pgDB.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"not_ready","reason":"postgres_unreachable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})
	assetHTTPHandler := api.NewHTTPHandler(assetSvc, logger)
	httpMux.Handle("/api/v1/assets", assetHTTPHandler)
	httpMux.Handle("/api/v1/assets/", assetHTTPHandler)

	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	httpServer := &http.Server{
		Addr:         httpAddr,
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("HTTP health server listening", zap.String("addr", httpAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// =========================================================================
	// 阶段8：优雅关闭
	// =========================================================================
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
	consumerCancel()
	if bindingConsumer != nil {
		if err := bindingConsumer.Close(); err != nil {
			logger.Warn("Asset binding consumer close failed", zap.Error(err))
		}
	}

	// 标记 gRPC 服务为 NOT_SERVING
	healthServer.SetServingStatus("asset-service", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// 优雅关闭 HTTP
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}

	// 优雅关闭 gRPC
	grpcServer.GracefulStop()

	logger.Info("Asset Service stopped")
}

// =============================================================================
// gRPC 拦截器
// =============================================================================

func loggingUnaryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		if err != nil {
			logger.Warn("gRPC request failed",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
				zap.Error(err))
		} else {
			logger.Debug("gRPC request",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration))
		}

		return resp, err
	}
}

func recoveryUnaryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("gRPC handler panic recovered",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r))
				err = fmt.Errorf("internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// =============================================================================
// 环境变量辅助
// =============================================================================

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

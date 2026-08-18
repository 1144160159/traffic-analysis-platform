package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	segmentKafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/apitoken"
	authrepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/consumer"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/miniohttp"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/sourcequality"
	graphConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
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
	var assetProjectionNebulaStore *graphNebula.WorkbenchStore
	if cfg.Kafka.ProjectionEnabled || cfg.Detail.NebulaEnabled {
		assetProjectionNebulaStore, err = graphNebula.NewWorkbenchStore(assetNebulaConfig(cfg), logger)
		if err != nil {
			logger.Fatal("Failed to initialize asset NebulaGraph store", zap.Error(err))
		}
	}

	// =========================================================================
	// 阶段5：初始化 Service + Handler
	// =========================================================================
	assetSvc := service.New(cfg, assetRepo, logger)
	var assetDetailClickHouseDB *sql.DB
	if cfg.Detail.ClickHouseEnabled {
		assetDetailClickHouseDB = openAssetDetailClickHouse(cfg.Detail)
		defer assetDetailClickHouseDB.Close()
		reader, readerErr := repository.NewAssetDetailClickHouseReader(assetDetailClickHouseDB, cfg.Detail)
		if readerErr != nil {
			logger.Fatal("Failed to initialize asset detail ClickHouse reader", zap.Error(readerErr))
		}
		assetSvc.WithAssetDetailReaders(reader, reader)
		readinessCtx, readinessCancel := context.WithTimeout(context.Background(), cfg.Detail.ClickHouseDial)
		readinessErr := assetDetailClickHouseDB.PingContext(readinessCtx)
		readinessCancel()
		if readinessErr != nil {
			logger.Warn("Asset detail ClickHouse is initially unavailable; snapshot requests will remain explicitly partial", zap.Error(readinessErr))
		} else {
			logger.Info("Asset detail ClickHouse readers enabled",
				zap.Strings("hosts", cfg.Detail.ClickHouseHosts),
				zap.Duration("lookback", cfg.Detail.ClickHouseLookback),
				zap.Int("alert_limit", cfg.Detail.ClickHouseAlertLimit))
		}
	} else {
		logger.Info("Asset detail ClickHouse readers disabled")
	}
	if cfg.Detail.EvidenceEnabled {
		endpoint := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cfg.Export.S3Endpoint), "http://"), "https://")
		transport, transportErr := miniohttp.NewTransport(cfg.Export.S3UseSSL, cfg.Export.S3CAFile)
		if transportErr != nil {
			logger.Fatal("Failed to configure asset evidence MinIO TLS", zap.Error(transportErr))
		}
		minioClient, minioErr := minio.New(endpoint, &minio.Options{
			Creds:     credentials.NewStaticV4(cfg.Export.S3AccessKey, cfg.Export.S3SecretKey, ""),
			Secure:    cfg.Export.S3UseSSL,
			Transport: transport,
		})
		if minioErr != nil {
			logger.Fatal("Failed to initialize asset evidence MinIO client", zap.Error(minioErr))
		}
		objectStore, objectStoreErr := repository.NewMinIOAssetEvidenceObjectStore(minioClient)
		if objectStoreErr != nil {
			logger.Fatal("Failed to initialize asset evidence object store", zap.Error(objectStoreErr))
		}
		evidenceReader, evidenceErr := repository.NewAssetDetailEvidenceReader(
			assetDetailClickHouseDB, objectStore, cfg.Detail.ClickHouseQuery, cfg.Detail.EvidenceLimit,
		)
		if evidenceErr != nil {
			logger.Fatal("Failed to initialize asset evidence reader", zap.Error(evidenceErr))
		}
		assetSvc.WithAssetEvidenceObjectReader(evidenceReader)
		logger.Info("Asset detail ClickHouse/MinIO evidence reconciliation enabled", zap.Int("evidence_limit", cfg.Detail.EvidenceLimit))
	} else {
		logger.Info("Asset detail ClickHouse/MinIO evidence reconciliation disabled")
	}
	if cfg.Detail.NebulaEnabled {
		reader, readerErr := repository.NewAssetDetailNebulaReader(assetProjectionNebulaStore, cfg.Detail.NebulaRelationLimit)
		if readerErr != nil {
			logger.Fatal("Failed to initialize asset detail NebulaGraph reader", zap.Error(readerErr))
		}
		assetSvc.WithAssetGraphProjectionReader(reader)
		logger.Info("Asset detail bounded NebulaGraph reader enabled", zap.Int("relation_limit", cfg.Detail.NebulaRelationLimit))
	} else {
		logger.Info("Asset detail bounded NebulaGraph reader disabled")
	}
	assetHandler := api.NewAssetHandler(assetSvc, assetRepo, logger)

	logger.Info("Asset service initialized")

	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	assetSvc.StartDiscoveryScheduler(consumerCtx)
	assetSvc.StartDiscoveryWorker(consumerCtx)
	assetSvc.StartAssetExportWorker(consumerCtx)
	var bindingConsumer *consumer.BindingConsumer
	if cfg.Kafka.Enabled {
		barrier, barrierErr := kafkaCommon.NewPostgresDLQAcknowledgementBarrier(
			pgDB, cfg.Kafka.GroupID,
		)
		if barrierErr != nil {
			logger.Fatal("Failed to initialize asset binding DLQ barrier", zap.Error(barrierErr))
		}
		bc, err := consumer.NewBindingConsumer(cfg.Kafka, assetSvc, barrier, logger)
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

	var assetEventProducer *kafkaCommon.Producer
	var assetDispatcherDone chan struct{}
	if cfg.Kafka.EventOutboxEnabled {
		assetEventProducer, err = kafkaCommon.NewProducer(kafkaCommon.ProducerConfig{
			Brokers:       cfg.Kafka.BrokerList(),
			Topic:         cfg.Kafka.EventTopic,
			BatchSize:     cfg.Kafka.OutboxBatchSize,
			BatchTimeout:  100 * time.Millisecond,
			MaxAttempts:   3,
			RequiredAcks:  "all",
			Compression:   "lz4",
			Async:         false,
			IdempotentKey: "event_id",
			Security:      cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to initialize asset event producer", zap.Error(err))
		}
		hostname, _ := os.Hostname()
		dispatcher, dispatcherErr := repository.NewAssetOutboxDispatcher(
			pgDB,
			assetEventProducer,
			repository.OutboxDispatcherConfig{
				WorkerID:    "asset-event-outbox/" + hostname,
				Lease:       cfg.Kafka.OutboxLease,
				MaxAttempts: cfg.Kafka.OutboxMaxAttempts,
				BatchSize:   cfg.Kafka.OutboxBatchSize,
				Interval:    cfg.Kafka.OutboxInterval,
				TenantID:    cfg.Kafka.EventOutboxTenantID,
				Logger:      logger,
			},
		)
		if dispatcherErr != nil {
			logger.Fatal("Failed to initialize asset outbox dispatcher", zap.Error(dispatcherErr))
		}
		if dispatcherErr = dispatcher.VerifySchema(consumerCtx); dispatcherErr != nil {
			logger.Fatal("Asset outbox schema is not ready", zap.Error(dispatcherErr))
		}
		assetDispatcherDone = make(chan struct{})
		go func() {
			defer close(assetDispatcherDone)
			dispatcher.Run(consumerCtx)
		}()
		logger.Info("Asset transactional outbox dispatcher started",
			zap.String("topic", cfg.Kafka.EventTopic))
	} else {
		logger.Warn("Asset transactional outbox dispatcher disabled")
	}

	var discoveryEventProducer *kafkaCommon.Producer
	var discoveryDispatcherDone chan struct{}
	if cfg.Kafka.DiscoveryOutboxEnabled {
		discoveryEventProducer, err = kafkaCommon.NewProducer(kafkaCommon.ProducerConfig{
			Brokers:       cfg.Kafka.BrokerList(),
			Topic:         cfg.Kafka.DiscoveryEventTopic,
			BatchSize:     cfg.Kafka.OutboxBatchSize,
			BatchTimeout:  100 * time.Millisecond,
			MaxAttempts:   3,
			RequiredAcks:  "all",
			Compression:   "lz4",
			Async:         false,
			IdempotentKey: "event_id",
			Security:      cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to initialize asset discovery event producer", zap.Error(err))
		}
		hostname, _ := os.Hostname()
		dispatcher, dispatcherErr := repository.NewDiscoveryOutboxDispatcher(
			pgDB,
			discoveryEventProducer,
			repository.OutboxDispatcherConfig{
				WorkerID:    "asset-discovery-outbox/" + hostname,
				Lease:       cfg.Kafka.OutboxLease,
				MaxAttempts: cfg.Kafka.OutboxMaxAttempts,
				BatchSize:   cfg.Kafka.OutboxBatchSize,
				Interval:    cfg.Kafka.OutboxInterval,
				Logger:      logger,
			},
		)
		if dispatcherErr != nil {
			logger.Fatal("Failed to initialize asset discovery outbox dispatcher", zap.Error(dispatcherErr))
		}
		if dispatcherErr = dispatcher.VerifySchema(consumerCtx); dispatcherErr != nil {
			logger.Fatal("Asset discovery outbox schema is not ready", zap.Error(dispatcherErr))
		}
		discoveryDispatcherDone = make(chan struct{})
		go func() {
			defer close(discoveryDispatcherDone)
			dispatcher.Run(consumerCtx)
		}()
		logger.Info("Asset discovery transactional outbox dispatcher started",
			zap.String("topic", cfg.Kafka.DiscoveryEventTopic))
	} else {
		logger.Warn("Asset discovery transactional outbox dispatcher disabled")
	}

	var assetExportEventProducer *kafkaCommon.Producer
	var assetExportDispatcherDone chan struct{}
	if cfg.Export.OutboxEnabled {
		assetExportEventProducer, err = kafkaCommon.NewProducer(kafkaCommon.ProducerConfig{
			Brokers:       cfg.Kafka.BrokerList(),
			Topic:         cfg.Export.EventTopic,
			BatchSize:     cfg.Kafka.OutboxBatchSize,
			BatchTimeout:  100 * time.Millisecond,
			MaxAttempts:   3,
			RequiredAcks:  "all",
			Compression:   "lz4",
			Async:         false,
			IdempotentKey: "event_id",
			Security:      cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to initialize asset export event producer", zap.Error(err))
		}
		hostname, _ := os.Hostname()
		exportDispatcher, dispatcherErr := repository.NewAssetExportOutboxDispatcher(
			pgDB,
			assetExportEventProducer,
			repository.OutboxDispatcherConfig{
				WorkerID:    "asset-export-outbox/" + hostname,
				Lease:       cfg.Kafka.OutboxLease,
				MaxAttempts: cfg.Kafka.OutboxMaxAttempts,
				BatchSize:   cfg.Kafka.OutboxBatchSize,
				Interval:    cfg.Kafka.OutboxInterval,
				Logger:      logger,
			},
		)
		if dispatcherErr != nil {
			logger.Fatal("Failed to initialize asset export outbox dispatcher", zap.Error(dispatcherErr))
		}
		if dispatcherErr = exportDispatcher.VerifySchema(consumerCtx); dispatcherErr != nil {
			logger.Fatal("Asset export outbox schema is not ready", zap.Error(dispatcherErr))
		}
		assetExportDispatcherDone = make(chan struct{})
		go func() {
			defer close(assetExportDispatcherDone)
			exportDispatcher.Run(consumerCtx)
		}()
		logger.Info("Asset export transactional outbox dispatcher started", zap.String("topic", cfg.Export.EventTopic))
	} else {
		logger.Warn("Asset export transactional outbox dispatcher disabled")
	}

	var assetProjectionConsumer *kafkaCommon.Consumer
	var assetProjectionDone chan struct{}
	if cfg.Kafka.ProjectionEnabled {
		projectionEventConsumer, projectionErr := consumer.NewAssetProjectionEventConsumerWithQuality(
			pgDB,
			cfg.Kafka.ProjectionGroupID,
			sourcequality.NewRepository(pgDB),
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize asset projection event consumer", zap.Error(projectionErr))
		}
		projectionEventConsumer.SetClickHouseProjectionEnabled(
			cfg.Projection.ClickHouse.Enabled)
		osProjection, projectionErr := consumer.NewOpenSearchAssetProjection(
			cfg.Projection.OpenSearch.Addresses,
			cfg.Projection.OpenSearch.Username,
			cfg.Projection.OpenSearch.Password,
			cfg.Projection.OpenSearch.WriteAlias,
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize asset OpenSearch projection", zap.Error(projectionErr))
		}
		readinessCtx, readinessCancel := context.WithTimeout(consumerCtx, 5*time.Second)
		projectionErr = osProjection.Ready(readinessCtx)
		readinessCancel()
		if projectionErr != nil {
			logger.Fatal("Asset OpenSearch projection is not ready", zap.Error(projectionErr))
		}
		nebulaProjection, projectionErr := consumer.NewNebulaAssetProjection(assetProjectionNebulaStore)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize asset graph projection", zap.Error(projectionErr))
		}
		readinessCtx, readinessCancel = context.WithTimeout(consumerCtx, 5*time.Second)
		projectionErr = assetProjectionNebulaStore.Ready(readinessCtx)
		readinessCancel()
		if projectionErr != nil {
			logger.Fatal("Asset NebulaGraph projection is not ready", zap.Error(projectionErr))
		}
		projectionTargets := []consumer.AssetProjectionTarget{osProjection, nebulaProjection}
		if cfg.Projection.ClickHouse.Enabled {
			projectionDB := openAssetProjectionClickHouse(cfg.Projection.ClickHouse)
			defer projectionDB.Close()
			readinessCtx, readinessCancel = context.WithTimeout(
				consumerCtx, cfg.Projection.ClickHouse.Dial)
			projectionErr = projectionDB.PingContext(readinessCtx)
			readinessCancel()
			if projectionErr != nil {
				logger.Fatal("Asset ClickHouse projection is not ready", zap.Error(projectionErr))
			}
			clickHouseProjection, targetErr := consumer.NewClickHouseAssetProjection(
				projectionDB,
				cfg.Projection.ClickHouse.Table,
				cfg.Kafka.ProjectionGroupID,
			)
			if targetErr != nil {
				logger.Fatal("Failed to initialize asset ClickHouse projection", zap.Error(targetErr))
			}
			readinessCtx, readinessCancel = context.WithTimeout(
				consumerCtx, cfg.Projection.ClickHouse.Read)
			projectionErr = clickHouseProjection.Ready(readinessCtx)
			readinessCancel()
			if projectionErr != nil {
				logger.Fatal("Asset ClickHouse source-fact table is not ready", zap.Error(projectionErr))
			}
			projectionTargets = append(projectionTargets, clickHouseProjection)
		}
		hostname, _ := os.Hostname()
		projectionWorker, projectionErr := consumer.NewAssetProjectionWorker(
			pgDB,
			projectionTargets,
			consumer.AssetProjectionWorkerConfig{
				WorkerID:    "asset-projection/" + hostname,
				Lease:       cfg.Projection.Lease,
				Interval:    cfg.Projection.Interval,
				MaxAttempts: cfg.Kafka.ProjectionMaxAttempts,
				Logger:      logger,
			},
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize asset projection worker", zap.Error(projectionErr))
		}
		if projectionErr = projectionWorker.VerifySchema(consumerCtx); projectionErr != nil {
			logger.Fatal("Asset projection schema is not ready", zap.Error(projectionErr))
		}
		assetProjectionConsumer, projectionErr = kafkaCommon.NewConsumer(
			assetProjectionConsumerConfig(cfg.Kafka),
			logger,
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize asset projection Kafka consumer", zap.Error(projectionErr))
		}
		assetProjectionConsumer.SetDLQAcknowledgementBarrier(
			projectionEventConsumer.RecordDLQAcknowledgement)
		assetProjectionDone = make(chan struct{}, 2)
		go func() {
			defer func() { assetProjectionDone <- struct{}{} }()
			if consumeErr := assetProjectionConsumer.Consume(consumerCtx, projectionEventConsumer.Handle); consumeErr != nil &&
				consumeErr != context.Canceled {
				logger.Error("Asset projection Kafka consumer stopped", zap.Error(consumeErr))
			}
		}()
		go func() {
			defer func() { assetProjectionDone <- struct{}{} }()
			projectionWorker.Run(consumerCtx)
		}()
		logger.Info("Asset durable projection worker enabled",
			zap.String("topic", cfg.Kafka.EventTopic),
			zap.String("group_id", cfg.Kafka.ProjectionGroupID),
			zap.String("opensearch_write_alias", cfg.Projection.OpenSearch.WriteAlias),
			zap.Bool("clickhouse_source_facts", cfg.Projection.ClickHouse.Enabled))
	} else {
		logger.Warn("Asset durable OpenSearch and NebulaGraph projection disabled")
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

	// 注册 gRPC Reflection（仅显式配置开启时注册，生产默认关闭）
	if cfg.Server.GRPCEnableReflection {
		reflection.Register(grpcServer)
		logger.Warn("gRPC reflection is ENABLED via ASSET_GRPC_ENABLE_REFLECTION; do not enable in production")
	}

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

	// 权限体系三层防御:asset HTTP 面此前不校验任何令牌(只信租户头),
	// 现统一接入共享鉴权中间件(JWKS 验签 + 租户绑定 + API token 回退),
	// 再叠加契约解释器(/v1/assets/* 25 操作逐操作判定)。
	var assetRoot http.Handler = httpMux
	authzCfg := authz.Config{
		JWKSURL:            getEnv("AUTHZ_JWKS_URL", "http://10.0.5.8:30180/auth/realms/master/protocol/openid-connect/certs"),
		Issuer:             getEnv("AUTHZ_ISSUER", ""),
		Mode:               getEnv("AUTHZ_MODE", "shadow"),
		RequireTenantClaim: getBoolEnv("AUTHZ_REQUIRE_TENANT_CLAIM", false),
		AllowedAZP:         splitCSV(getEnv("AUTHZ_ALLOWED_AZP", "")),
		DenyAuditor:        authzDenyAudit(pgDB, logger),
		ExemptPaths:        []string{"/health", "/health/readiness"},
	}
	if getBoolEnv("AUTHZ_API_TOKEN_ENABLED", false) {
		authzCfg.Fallback = apitoken.NewValidator(authrepository.NewTokenRepository(pgDB, logger), logger).Validate
		logger.Info("API token fallback enabled (asset HTTP API)")
	}
	authzMW := authz.New(authzCfg, logger)
	if mode := strings.TrimSpace(getEnv("AUTHZ_CONTRACT_MODE", "")); mode == "enforce" || mode == "shadow" {
		// 顺序:认证(Handler,外层,先产出主体)→ 契约判定(内层) → 业务。
		assetRoot = authz.EnforceContract(authz.PrincipalFromRequest, mode, nil, logger, assetContractDenyAudit(pgDB, logger))(assetRoot)
		logger.Info("Contract interpreter enabled on asset HTTP API", zap.String("mode", mode))
	}
	assetRoot = authzMW.Handler(assetRoot)

	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	httpServer := &http.Server{
		Addr:         httpAddr,
		Handler:      assetRoot,
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
	if assetDispatcherDone != nil {
		select {
		case <-assetDispatcherDone:
		case <-time.After(5 * time.Second):
			logger.Warn("Asset outbox dispatcher did not stop before producer close")
		}
	}
	if assetEventProducer != nil {
		if err := assetEventProducer.Close(); err != nil {
			logger.Warn("Asset event producer close failed", zap.Error(err))
		}
	}
	if discoveryDispatcherDone != nil {
		select {
		case <-discoveryDispatcherDone:
		case <-time.After(5 * time.Second):
			logger.Warn("Asset discovery outbox dispatcher did not stop before producer close")
		}
	}
	if discoveryEventProducer != nil {
		if err := discoveryEventProducer.Close(); err != nil {
			logger.Warn("Asset discovery event producer close failed", zap.Error(err))
		}
	}
	if assetExportDispatcherDone != nil {
		select {
		case <-assetExportDispatcherDone:
		case <-time.After(5 * time.Second):
			logger.Warn("Asset export outbox dispatcher did not stop before producer close")
		}
	}
	if assetExportEventProducer != nil {
		if err := assetExportEventProducer.Close(); err != nil {
			logger.Warn("Asset export event producer close failed", zap.Error(err))
		}
	}
	if assetProjectionConsumer != nil {
		if err := assetProjectionConsumer.Close(); err != nil {
			logger.Warn("Asset projection consumer close failed", zap.Error(err))
		}
	}
	if assetProjectionDone != nil {
		for stopped := 0; stopped < 2; stopped++ {
			select {
			case <-assetProjectionDone:
			case <-time.After(5 * time.Second):
				logger.Warn("Asset projection component did not stop cleanly")
				stopped = 2
			}
		}
	}
	if assetProjectionNebulaStore != nil {
		assetProjectionNebulaStore.Close()
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

func assetProjectionConsumerConfig(cfg config.KafkaConfig) kafkaCommon.ConsumerConfig {
	return kafkaCommon.ConsumerConfig{
		Brokers: cfg.BrokerList(),
		Topic:   cfg.EventTopic,
		GroupID: cfg.ProjectionGroupID,
		// A new projection group must rebuild from the retained event log.
		// Existing groups still resume from their committed offsets.
		StartOffset:          segmentKafka.FirstOffset,
		MinBytes:             cfg.MinBytes,
		MaxBytes:             cfg.MaxBytes,
		MaxRetries:           cfg.ProjectionMaxAttempts,
		RetryBackoff:         time.Second,
		EnableDLQ:            true,
		DLQTopic:             cfg.ProjectionDLQTopic,
		CommitOnDLQSuccess:   true,
		DLQPermanentOnly:     true,
		CommitOnHandlerError: false,
		Security:             cfg.Security,
	}
}

func openAssetDetailClickHouse(cfg config.AssetDetailConfig) *sql.DB {
	querySeconds := int((cfg.ClickHouseQuery + time.Second - 1) / time.Second)
	return clickhouse.OpenDB(&clickhouse.Options{
		Addr: cfg.ClickHouseHosts,
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouseDatabase,
			Username: cfg.ClickHouseUsername,
			Password: cfg.ClickHousePassword,
		},
		DialTimeout:     cfg.ClickHouseDial,
		ReadTimeout:     cfg.ClickHouseRead,
		MaxOpenConns:    8,
		MaxIdleConns:    4,
		ConnMaxLifetime: time.Hour,
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
		Settings: clickhouse.Settings{
			"max_execution_time": querySeconds,
			"max_rows_to_read":   cfg.ClickHouseMaxRows,
			"max_bytes_to_read":  cfg.ClickHouseMaxBytes,
			"read_overflow_mode": "throw",
		},
	})
}

func openAssetProjectionClickHouse(cfg config.ProjectionClickHouseConfig) *sql.DB {
	return clickhouse.OpenDB(&clickhouse.Options{
		Addr: cfg.Hosts,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout:     cfg.Dial,
		ReadTimeout:     cfg.Read,
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
}

func assetNebulaConfig(cfg *config.Config) graphConfig.NebulaConfig {
	return graphConfig.NebulaConfig{
		Enabled:     true,
		Addresses:   cfg.Projection.Nebula.Addresses,
		Username:    cfg.Projection.Nebula.Username,
		Password:    cfg.Projection.Nebula.Password,
		Space:       cfg.Projection.Nebula.Space,
		Timeout:     cfg.Projection.Nebula.Timeout,
		IdleTime:    cfg.Projection.Nebula.IdleTime,
		MaxPoolSize: cfg.Projection.Nebula.MaxPoolSize,
		MinPoolSize: cfg.Projection.Nebula.MinPoolSize,
	}
}

func getBoolEnv(key string, def bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return def
	}
	return parsed
}

// authzDenyAudit 认证拒绝留痕(审计三联):401/403 拒绝 best-effort 落 audit_logs。
func authzDenyAudit(db *sql.DB, logger *zap.Logger) authz.DenyAuditFunc {
	return func(r *http.Request, status int, reason string, principal *authz.Principal) {
		if db == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		detail, _ := json.Marshal(map[string]interface{}{
			"reason": reason, "path": r.URL.Path, "status": status, "result": "denied",
		})
		tenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
		if tenant == "" {
			tenant = "default"
		}
		userID := ""
		if principal != nil {
			userID = principal.Subject
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, NULLIF($2,'')::uuid, 'AUTHZ_ACCESS_DENIED', 'authz', '', $3::jsonb, $4, $5)`,
			tenant, userID, string(detail), httpx.GetClientIP(r), r.UserAgent()); err != nil {
			logger.Warn("authz deny audit persist failed", zap.String("path", r.URL.Path), zap.Error(err))
		}
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// assetContractDenyAudit 契约拒绝留痕(复用 PG 直落审计通道)。
func assetContractDenyAudit(db *sql.DB, logger *zap.Logger) authz.ContractDenyAuditor {
	return func(r *http.Request, op *authz.Operation, principal *authz.Principal, status int) {
		reason := fmt.Sprintf("contract scope required: %s (operation=%s)", op.RequiredScope, op.OperationID)
		authzDenyAudit(db, logger)(r, status, reason, principal)
	}
}

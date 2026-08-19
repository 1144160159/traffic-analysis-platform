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
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/arkime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/attackchain"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/baseline"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/campaignrail"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/consumer"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/evidence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/playbook"
	alertProjection "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/projection"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/realtime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/risk"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	authConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/oidc"
	authRepo "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	authService "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	commonAudit "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	graphConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
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
	readOnlyVerificationMode := getBoolEnv("READ_ONLY_VERIFICATION_MODE", false)

	logger.Info("Starting Alert Service",
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.String("kafka_topic", cfg.Kafka.Topic),
		zap.String("api_addr", cfg.API.ListenAddr),
		zap.Bool("read_only_verification_mode", readOnlyVerificationMode))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if stage := strings.TrimSpace(os.Getenv("CAMPAIGN_RAIL_CANARY_STAGE")); stage != "" {
		if err := runCampaignRailCanary(ctx, stage, cfg, logger); err != nil {
			logger.Fatal("Campaign rail K8s canary stopped", zap.String("stage", stage), zap.Error(err))
		}
		return
	}

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
	var whitelistRepo *whitelist.Repository
	if db != nil {
		whitelistRepo = whitelist.NewRepository(db, logger)
	}
	if cfg.Kafka.WhitelistEventProducerEnabled || cfg.Kafka.WhitelistDetectionMatcherEnabled {
		if whitelistRepo == nil || readOnlyVerificationMode {
			logger.Fatal("Whitelist producer or detection matcher requires writable PostgreSQL mode")
		}
		verifyCtx, verifyCancel := context.WithTimeout(ctx, 10*time.Second)
		verifyErr := whitelistRepo.VerifyRuleProjectionSchema(verifyCtx)
		verifyCancel()
		if verifyErr != nil {
			logger.Fatal("Whitelist rule projection schema is unavailable", zap.Error(verifyErr))
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
		cfg.OpenSearch.WriteTarget(),
		cfg.OpenSearch.V2Enabled,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to create OpenSearch writer", zap.Error(err))
	}
	defer osWriter.Close()
	logger.Info("Connected to OpenSearch",
		zap.Strings("addresses", cfg.OpenSearch.Addresses),
		zap.String("read_target", cfg.OpenSearch.ReadTarget()),
		zap.String("write_target", cfg.OpenSearch.WriteTarget()),
		zap.Bool("alerts_v2_enabled", cfg.OpenSearch.V2Enabled))

	// ==================== 初始化 Dual Writer ====================
	dualWriter := persistence.NewDualWriter(chWriter, osWriter, 5, logger)
	var projectionDebtStore *persistence.ProjectionDebtStore
	if db != nil {
		projectionDebtStore = persistence.NewProjectionDebtStore(db)
		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		projectionSchemaErr := projectionDebtStore.CheckSchema(checkCtx)
		checkCancel()
		if projectionSchemaErr == nil {
			dualWriter.SetProjectionDebtRecorder(projectionDebtStore)
			logger.Info("Durable alert OpenSearch projection debt barrier is ready",
				zap.String("target_index_version", osWriter.TargetVersion()))
		} else if cfg.AlertProjection.ReconcileEnabled && !readOnlyVerificationMode {
			logger.Fatal("Alert projection reconcile was enabled without its PostgreSQL schema", zap.Error(projectionSchemaErr))
		} else {
			// Safe degradation: OpenSearch failures block Kafka offset commits until
			// the expand-only migration becomes available.
			logger.Warn("Alert projection debt schema is unavailable; OpenSearch failures will block offset commits",
				zap.Error(projectionSchemaErr))
		}
	} else if cfg.AlertProjection.ReconcileEnabled && !readOnlyVerificationMode {
		logger.Fatal("Alert projection reconcile requires PostgreSQL authentication storage")
	}
	if !readOnlyVerificationMode {
		go dualWriter.StartHealthCheck(ctx)
	}

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
	if cfg.AlertProjection.ReconcileEnabled && !readOnlyVerificationMode {
		projectionWorker, workerErr := alertProjection.NewWorker(alertProjection.WorkerConfig{
			Interval: cfg.AlertProjection.Interval, Lease: cfg.AlertProjection.Lease,
			BatchSize: cfg.AlertProjection.BatchSize, MaxAttempts: cfg.AlertProjection.MaxAttempts,
		}, projectionDebtStore, alertRepo, osWriter, logger)
		if workerErr != nil {
			logger.Fatal("Failed to initialize alert OpenSearch projection repair worker", zap.Error(workerErr))
		}
		go projectionWorker.Run(ctx)
		logger.Info("Alert OpenSearch projection repair worker enabled",
			zap.Int("batch_size", cfg.AlertProjection.BatchSize),
			zap.Int("max_attempts", cfg.AlertProjection.MaxAttempts),
			zap.String("target_index_version", osWriter.TargetVersion()))
	}

	osRepo, err := repository.NewOpenSearchRepository(repository.OpenSearchConfig{
		Addresses:          cfg.OpenSearch.Addresses,
		Username:           cfg.OpenSearch.Username,
		Password:           cfg.OpenSearch.Password,
		ReadTarget:         cfg.OpenSearch.ReadTarget(),
		WriteTarget:        cfg.OpenSearch.WriteTarget(),
		ExactTarget:        cfg.OpenSearch.V2Enabled,
		CursorEnabled:      cfg.OpenSearch.SearchCursorEnabled,
		CursorSigningKey:   cfg.Auth.JWTSecretKey,
		ShallowResultLimit: cfg.OpenSearch.SearchShallowLimit,
		MaxPageSize:        cfg.OpenSearch.SearchMaxPageSize,
		QueryTimeout:       cfg.OpenSearch.SearchQueryTimeout,
		CursorTTL:          cfg.OpenSearch.SearchCursorTTL,
		TrackTotalHitsUpTo: cfg.OpenSearch.SearchTrackTotal,
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
	var auditLogger *commonAudit.Logger
	if !readOnlyVerificationMode {
		auditLogger, err = commonAudit.NewLogger(auditCfg, logger)
		if err != nil {
			logger.Warn("Failed to create audit logger, continuing without audit", zap.Error(err))
		} else {
			defer auditLogger.Close()
		}
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
	alertService.SetAlertSnapshotRepository(repository.NewAlertSnapshotRepository(
		alertRepo,
		osRepo,
		db,
		cfg.OpenSearch.WriteTarget(),
		logger,
	))

	// ==================== 初始化 Kafka Producer (for Feedback) ====================
	var feedbackProducer *kafka.Producer
	var modelFeedbackRevisionProducer *kafka.Producer
	var responseActionProducer *kafka.Producer
	var savedViewEventProducer *kafka.Producer
	savedViewTransactionEnabled := cfg.Kafka.SavedViewTransactionEnabled && !readOnlyVerificationMode
	if !readOnlyVerificationMode {
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
		responseActionProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.ResponseActionTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Warn("Failed to create response-action Kafka producer; durable outbox will remain pending", zap.Error(err))
			responseActionProducer = nil
		} else {
			defer responseActionProducer.Close()
		}
		if savedViewTransactionEnabled {
			savedViewEventProducer, err = kafka.NewProducer(kafka.ProducerConfig{
				Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.SavedViewEventTopic, BatchSize: 100,
				RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
			}, logger)
			if err != nil {
				logger.Warn("Failed to create saved-view Kafka producer; committed outbox events will remain pending", zap.Error(err))
				savedViewEventProducer = nil
			} else {
				defer savedViewEventProducer.Close()
			}
		}
	}

	// ==================== 初始化 API Handler ====================
	// Feedback HTTP always uses the PostgreSQL transaction/outbox path. A
	// temporarily unavailable producer leaves committed outbox rows pending.
	apiHandler := api.NewHandlerWithFeedback(alertService, feedbackProducer, alertAuditLogger, logger)
	alertActionAuditWriter := api.NewAlertActionAuditWriter(db, logger)
	apiHandler.SetActionAuditWriter(alertActionAuditWriter)
	if alertActionAuditWriter != nil {
		alertActionAuditWriter.StartRetryWorker(ctx)
		logger.Info("Alert action audit compensation retry worker started")
	}
	alertEvidenceChainEnabled := getBoolEnv("ALERT_EVIDENCE_CHAIN_V1_ENABLED", false)
	if alertEvidenceChainEnabled && strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_DOWNLOAD_SECRET")) == "" {
		logger.Fatal("ALERT_EVIDENCE_DOWNLOAD_SECRET is required when strict evidence access is enabled")
	}
	alertEvidenceManifests := api.NewPostgresAlertEvidenceManifestStore(db)
	if alertEvidenceChainEnabled {
		verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
		err = alertEvidenceManifests.VerifySchema(verifyCtx)
		cancelVerify()
		if err != nil {
			logger.Fatal("Alert evidence manifest schema is unavailable", zap.Error(err))
		}
	}
	apiHandler.SetAlertEvidenceManifestStore(alertEvidenceManifests)
	apiHandler.SetAlertEvidenceChainEnabled(alertEvidenceChainEnabled)
	alertEvidenceLinkConsumerEnabled := cfg.Kafka.AlertEvidenceLinkConsumerEnabled && !readOnlyVerificationMode
	alertEvidenceLinkDispatcherEnabled := cfg.Kafka.AlertEvidenceLinkDispatcherEnabled && !readOnlyVerificationMode
	alertEvidenceLinkWriterEnabled := getBoolEnv("ALERT_EVIDENCE_LINK_WRITER_V1_ENABLED", false) && !readOnlyVerificationMode
	var alertEvidenceLinkConsumer *consumer.AlertEvidenceLinkConsumer
	if alertEvidenceLinkConsumerEnabled {
		if cfg.Kafka.AlertEvidenceLinkTopic != api.AlertEvidenceLinkEventTopic {
			logger.Fatal("Alert evidence link topic must match the versioned contract",
				zap.String("expected", api.AlertEvidenceLinkEventTopic), zap.String("actual", cfg.Kafka.AlertEvidenceLinkTopic))
		}
		candidateSHA256 := strings.ToLower(strings.TrimSpace(getEnv("ALERT_EVIDENCE_LINK_CANDIDATE_SHA256", "")))
		projection := api.NewAlertEvidenceLinkProjectionApplier(db, chClient)
		kafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.AlertEvidenceLinkTopic,
			GroupID: cfg.Kafka.AlertEvidenceLinkGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create alert evidence link projection consumer", zap.Error(consumerErr))
		}
		alertEvidenceLinkConsumer, consumerErr = consumer.NewAlertEvidenceLinkConsumer(
			kafkaConsumer, projection, cfg.Kafka.AlertEvidenceLinkTopic,
			cfg.Kafka.AlertEvidenceLinkGroup, candidateSHA256, logger)
		if consumerErr != nil {
			_ = kafkaConsumer.Close()
			logger.Fatal("Failed to initialize alert evidence link projection consumer", zap.Error(consumerErr))
		}
		defer alertEvidenceLinkConsumer.Close()
		apiHandler.AddReadinessCheck("alert_evidence_link_projection_v1", alertEvidenceLinkConsumer.Ready)
		go func() {
			logger.Info("Starting alert evidence link projection consumer",
				zap.String("topic", cfg.Kafka.AlertEvidenceLinkTopic),
				zap.String("group_id", cfg.Kafka.AlertEvidenceLinkGroup),
				zap.String("candidate_sha256", candidateSHA256))
			if startErr := alertEvidenceLinkConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Alert evidence link projection consumer stopped", zap.Error(startErr))
				cancel()
			}
		}()
	} else {
		logger.Info("Alert evidence link projection consumer is disabled")
	}
	if alertEvidenceLinkWriterEnabled {
		if !alertEvidenceLinkConsumerEnabled || !alertEvidenceLinkDispatcherEnabled || alertEvidenceLinkConsumer == nil {
			logger.Fatal("Alert evidence link writer requires the consumer-first dispatcher stage")
		}
		readinessDeadline := time.Now().Add(10 * time.Second)
		for alertEvidenceLinkConsumer.Ready(ctx) != nil && time.Now().Before(readinessDeadline) && ctx.Err() == nil {
			time.Sleep(50 * time.Millisecond)
		}
		if readyErr := alertEvidenceLinkConsumer.Ready(ctx); readyErr != nil {
			logger.Fatal("Alert evidence link writer cannot start before the projection consumer is ready", zap.Error(readyErr))
		}
		producer, producerErr := kafka.NewKeyedProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.AlertEvidenceLinkTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if producerErr != nil {
			logger.Fatal("Failed to initialize alert evidence link event producer", zap.Error(producerErr))
		}
		defer producer.Close()
		apiHandler.SetAlertEvidenceLinkRuntime(true, alertEvidenceLinkConsumer.Ready, producer)
		if workerErr := apiHandler.StartAlertEvidenceLinkOutboxWorker(ctx, 2*time.Second); workerErr != nil {
			logger.Fatal("Failed to start alert evidence link outbox worker", zap.Error(workerErr))
		}
		logger.Info("Alert evidence link writer enabled after consumer readiness")
	} else {
		apiHandler.SetAlertEvidenceLinkRuntime(false, nil, nil)
		if alertEvidenceLinkDispatcherEnabled {
			logger.Warn("Alert evidence link dispatcher requested without the writer; it remains stopped")
		}
		logger.Info("Alert evidence link writer is disabled")
	}
	feedbackTransactionalOutboxEnabled := getBoolEnv("ALERT_FEEDBACK_TRANSACTIONAL_OUTBOX_V1_ENABLED", true) && !readOnlyVerificationMode
	apiHandler.SetFeedbackTransactionalOutboxEnabled(feedbackTransactionalOutboxEnabled)
	modelFeedbackRevisionAuthorityEnabled := getBoolEnv("MODEL_FEEDBACK_REVISION_AUTHORITY_V1_ENABLED", false) && !readOnlyVerificationMode
	modelFeedbackRevisionProducerEnabled := getBoolEnv("MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED", false) && !readOnlyVerificationMode
	if modelFeedbackRevisionProducerEnabled {
		if !modelFeedbackRevisionAuthorityEnabled || !feedbackTransactionalOutboxEnabled {
			logger.Fatal("Model feedback revision producer requires the transactional revision authority")
		}
		readiness := api.ModelFeedbackProducerReadiness{
			Topic:           getEnv("KAFKA_MODEL_FEEDBACK_TOPIC", "model.feedback.v1"),
			ConsumerGroup:   getEnv("KAFKA_MODEL_FEEDBACK_CONSUMER_GROUP", "rule-manager-model-feedback-revision-v1"),
			CandidateSHA256: getEnv("MODEL_FEEDBACK_CONSUMER_CANDIDATE_SHA256", ""),
			ContractSHA256:  getEnv("MODEL_FEEDBACK_CONTRACT_SHA256", ""),
		}
		verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
		readinessErr := api.VerifyModelFeedbackProducerReadiness(verifyCtx, db, readiness)
		cancelVerify()
		if readinessErr != nil {
			logger.Fatal("Model feedback revision producer is not authorized by a consumer broker receipt", zap.Error(readinessErr))
		}
		modelFeedbackRevisionProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: readiness.Topic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to create model feedback revision producer", zap.Error(err))
		}
		defer modelFeedbackRevisionProducer.Close()
	}
	apiHandler.SetModelFeedbackRevisionRuntime(modelFeedbackRevisionAuthorityEnabled, modelFeedbackRevisionProducer)
	apiHandler.SetResponseActionProducer(responseActionProducer)
	responseUnknownReconciliationEnabled := getBoolEnv("ALERT_RESPONSE_UNKNOWN_EFFECT_RECONCILIATION_V1_ENABLED", false) && !readOnlyVerificationMode
	responseCompensationEnabled := getBoolEnv("ALERT_RESPONSE_COMPENSATION_EXECUTOR_V1_ENABLED", false) && !readOnlyVerificationMode
	responseCompensationMaxAttempts := getIntEnv("ALERT_RESPONSE_COMPENSATION_MAX_ATTEMPTS", 8)
	if (responseUnknownReconciliationEnabled || responseCompensationEnabled) && !cfg.Kafka.ResponseActionEnabled {
		logger.Fatal("Alert response recovery requires ALERT_RESPONSE_EXECUTION_V1_ENABLED")
	}
	if responseCompensationEnabled && (responseCompensationMaxAttempts < 1 || responseCompensationMaxAttempts > 100) {
		logger.Fatal("ALERT_RESPONSE_COMPENSATION_MAX_ATTEMPTS must be between 1 and 100")
	}
	apiHandler.SetResponseCompensationEnabled(responseCompensationEnabled)
	apiHandler.SetResponseCompensationMaxAttempts(responseCompensationMaxAttempts)
	apiHandler.SetSavedViewEventProducer(savedViewEventProducer)
	alertReportFeatureEnabled := getBoolEnv("ALERT_REPORT_JOBS_V1_ENABLED", false) && !readOnlyVerificationMode
	alertReportArtifactTTL, alertReportTTLParseErr := time.ParseDuration(getEnv("ALERT_REPORT_ARTIFACT_TTL", "24h"))
	if alertReportTTLParseErr != nil {
		logger.Fatal("ALERT_REPORT_ARTIFACT_TTL must be a duration between 5m and 720h", zap.Error(alertReportTTLParseErr))
	}
	if alertReportArtifactTTL < 5*time.Minute || alertReportArtifactTTL > 30*24*time.Hour {
		logger.Fatal("ALERT_REPORT_ARTIFACT_TTL must be a duration between 5m and 720h", zap.Duration("configured_ttl", alertReportArtifactTTL))
	}
	apiHandler.SetAlertReportArtifactTTL(alertReportArtifactTTL)
	campaignLinkFeatureEnabled := getBoolEnv("CAMPAIGN_ALERT_LINKS_V1_ENABLED", true)
	campaignAggregateV2Enabled := getBoolEnv("CAMPAIGN_AGGREGATE_V2_ENABLED", false)
	apiHandler.SetAlignmentFeatureFlags(alertReportFeatureEnabled, campaignLinkFeatureEnabled)
	apiHandler.SetCampaignAggregateV2FeatureFlag(campaignAggregateV2Enabled)
	if chSQLDB != nil {
		apiHandler.SetCampaignLookup(api.NewClickHouseAlertCampaignLookup(chSQLDB))
	}
	if responseActionProducer != nil && cfg.Kafka.ResponseActionEnabled {
		if err := apiHandler.StartResponseActionOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start response-action outbox worker", zap.Error(err))
		} else {
			logger.Info("Response-action outbox worker started")
		}
	} else if !cfg.Kafka.ResponseActionEnabled {
		logger.Warn("Response-action outbox worker is disabled; durable requests remain pending")
	}
	if savedViewTransactionEnabled && savedViewEventProducer != nil {
		if err := apiHandler.StartSavedViewOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start saved-view outbox worker", zap.Error(err))
		} else {
			logger.Info("Saved-view outbox worker started", zap.String("topic", cfg.Kafka.SavedViewEventTopic))
		}
	} else if !savedViewTransactionEnabled {
		logger.Warn("Saved-view transaction dispatcher is disabled; durable events remain pending")
	}
	if cfg.Kafka.ResponseActionEnabled && !readOnlyVerificationMode {
		responseExternalExecutorEnabled := getBoolEnv("ALERT_RESPONSE_EXTERNAL_EXECUTOR_V1_ENABLED", false)
		if (responseUnknownReconciliationEnabled || responseCompensationEnabled) && !responseExternalExecutorEnabled {
			logger.Fatal("Alert response recovery requires the external executor and authority gate")
		}
		responseProjection, projectionErr := consumer.NewPostgresAlertResponseProjection(db)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize alert response execution projection", zap.Error(projectionErr))
		}
		verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
		projectionErr = responseProjection.VerifySchema(verifyCtx)
		cancelVerify()
		if projectionErr != nil {
			logger.Fatal("Alert response execution projection schema is unavailable", zap.Error(projectionErr))
		}
		var responseExecutor *consumer.HTTPAlertResponseExecutor
		if responseExternalExecutorEnabled {
			executorURL := strings.TrimSpace(getEnv("ALERT_RESPONSE_EXECUTOR_URL", ""))
			lookupURL := strings.TrimSpace(getEnv("ALERT_RESPONSE_EXECUTOR_LOOKUP_URL", ""))
			if executorURL == "" || lookupURL == "" {
				logger.Fatal("Alert response external execution requires executor and authority lookup URLs")
			}
			responseExecutor, projectionErr = consumer.NewHTTPAlertResponseExecutor(
				executorURL, getEnv("ALERT_RESPONSE_EXECUTOR_TOKEN", ""),
				time.Duration(getIntEnv("ALERT_RESPONSE_EXECUTOR_TIMEOUT_SECONDS", 30))*time.Second,
			)
			if projectionErr != nil {
				logger.Fatal("Invalid alert response executor configuration", zap.Error(projectionErr))
			}
			if projectionErr = responseExecutor.ConfigureAuthorityLookup(lookupURL); projectionErr != nil {
				logger.Fatal("Invalid alert response authority lookup configuration", zap.Error(projectionErr))
			}
			if responseCompensationEnabled {
				compensationURL := strings.TrimSpace(getEnv("ALERT_RESPONSE_COMPENSATION_URL", ""))
				compensationLookupURL := strings.TrimSpace(getEnv("ALERT_RESPONSE_COMPENSATION_LOOKUP_URL", ""))
				if compensationURL == "" || compensationLookupURL == "" {
					logger.Fatal("Alert response compensation requires executor and authority lookup URLs")
				}
				if projectionErr = responseExecutor.ConfigureCompensation(compensationURL, compensationLookupURL); projectionErr != nil {
					logger.Fatal("Invalid alert response compensation configuration", zap.Error(projectionErr))
				}
			}
			if projectionErr = responseProjection.ConfigureExecutor(responseExecutor); projectionErr != nil {
				logger.Fatal("Failed to enable alert response external executor", zap.Error(projectionErr))
			}
			logger.Info("Alert response external executor enabled with mandatory authority lookup")
		}
		if responseUnknownReconciliationEnabled {
			projectionErr = responseProjection.ConfigureUnknownEffectReconciliation(
				getIntEnv("ALERT_RESPONSE_UNKNOWN_EFFECT_MAX_ATTEMPTS", 8),
				time.Duration(getIntEnv("ALERT_RESPONSE_UNKNOWN_EFFECT_INITIAL_DELAY_SECONDS", 15))*time.Second,
			)
			if projectionErr != nil {
				logger.Fatal("Invalid alert response unknown-effect reconciliation configuration", zap.Error(projectionErr))
			}
		}
		if responseUnknownReconciliationEnabled || responseCompensationEnabled {
			recoveryWorker, recoveryErr := consumer.NewAlertResponseRecoveryWorker(
				db, responseExecutor, responseExecutor, responseExecutor,
				consumer.AlertResponseRecoveryConfig{
					ExecutionEnabled: responseUnknownReconciliationEnabled, CompensationEnabled: responseCompensationEnabled,
					Interval:       time.Duration(getIntEnv("ALERT_RESPONSE_RECOVERY_INTERVAL_SECONDS", 5)) * time.Second,
					Lease:          time.Duration(getIntEnv("ALERT_RESPONSE_RECOVERY_LEASE_SECONDS", 45)) * time.Second,
					RequestTimeout: time.Duration(getIntEnv("ALERT_RESPONSE_RECOVERY_REQUEST_TIMEOUT_SECONDS", 30)) * time.Second,
					RetryBase:      time.Duration(getIntEnv("ALERT_RESPONSE_RECOVERY_RETRY_BASE_SECONDS", 15)) * time.Second,
					BatchSize:      getIntEnv("ALERT_RESPONSE_RECOVERY_BATCH_SIZE", 25),
				}, logger,
			)
			if recoveryErr != nil {
				logger.Fatal("Failed to initialize alert response recovery worker", zap.Error(recoveryErr))
			}
			verifyCtx, cancelVerify = context.WithTimeout(context.Background(), 10*time.Second)
			recoveryErr = recoveryWorker.VerifySchema(verifyCtx)
			cancelVerify()
			if recoveryErr != nil {
				logger.Fatal("Alert response recovery schema is unavailable", zap.Error(recoveryErr))
			}
			recoveryCtx, cancelRecovery := context.WithCancel(context.Background())
			defer cancelRecovery()
			go func() {
				if err := recoveryWorker.Start(recoveryCtx); err != nil && err != context.Canceled {
					logger.Error("Alert response recovery worker stopped", zap.Error(err))
				}
			}()
			logger.Info("Alert response recovery worker started",
				zap.Bool("execution_authority_recheck", responseUnknownReconciliationEnabled),
				zap.Bool("external_compensation", responseCompensationEnabled))
		}
		responseKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.ResponseActionTopic,
			GroupID: cfg.Kafka.ResponseActionGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create alert response execution consumer", zap.Error(consumerErr))
		}
		responseKafkaConsumer.SetDLQAcknowledgementBarrier(responseProjection.RecordDLQAcknowledgement)
		responseEventConsumer, consumerErr := consumer.NewAlertResponseEventConsumer(
			responseKafkaConsumer, responseProjection, logger,
		)
		if consumerErr != nil {
			_ = responseKafkaConsumer.Close()
			logger.Fatal("Failed to initialize alert response execution consumer", zap.Error(consumerErr))
		}
		responseEventCtx, cancelResponseEvent := context.WithCancel(context.Background())
		defer cancelResponseEvent()
		defer responseEventConsumer.Close()
		go func() {
			logger.Info(
				"Starting alert response execution receipt consumer",
				zap.String("topic", cfg.Kafka.ResponseActionTopic),
				zap.String("group_id", cfg.Kafka.ResponseActionGroup),
			)
			if err := responseEventConsumer.Start(responseEventCtx); err != nil && err != context.Canceled {
				logger.Error("Alert response execution consumer stopped", zap.Error(err))
			}
		}()
	} else {
		logger.Warn("Alert response execution consumer is disabled")
	}
	if getBoolEnv("THREAT_INTEL_EVENT_PROJECTION_V1_ENABLED", true) && !readOnlyVerificationMode {
		threatIntelProjection, projectionErr := consumer.NewPostgresThreatIntelEventProjection(db)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize threat intel event projection", zap.Error(projectionErr))
		}
		verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
		projectionErr = threatIntelProjection.VerifySchema(verifyCtx)
		cancelVerify()
		if projectionErr != nil {
			logger.Fatal("Threat intel event projection schema is unavailable", zap.Error(projectionErr))
		}
		threatIntelTopic := getEnv("KAFKA_THREAT_INTEL_EVENT_TOPIC", "threat.intel.v1")
		threatIntelGroup := getEnv(
			"KAFKA_THREAT_INTEL_EVENT_GROUP",
			"alert-service-threat-intel-projection-v1",
		)
		threatIntelKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: threatIntelTopic, GroupID: threatIntelGroup,
			MinBytes: 1, MaxWait: 500 * time.Millisecond, MaxRetries: 3,
			RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create threat intel event projection consumer", zap.Error(consumerErr))
		}
		threatIntelEventConsumer, consumerErr := consumer.NewThreatIntelEventConsumer(
			threatIntelKafkaConsumer, threatIntelProjection, logger,
		)
		if consumerErr != nil {
			_ = threatIntelKafkaConsumer.Close()
			logger.Fatal("Failed to initialize threat intel event consumer", zap.Error(consumerErr))
		}
		threatIntelEventCtx, cancelThreatIntelEvent := context.WithCancel(context.Background())
		defer cancelThreatIntelEvent()
		defer threatIntelEventConsumer.Close()
		go func() {
			logger.Info(
				"Starting threat intel authoritative event projection consumer",
				zap.String("topic", threatIntelTopic),
				zap.String("group_id", threatIntelGroup),
			)
			if err := threatIntelEventConsumer.Start(threatIntelEventCtx); err != nil &&
				err != context.Canceled {
				logger.Error("Threat intel event projection consumer stopped", zap.Error(err))
			}
		}()
	} else {
		logger.Warn("Threat intel event projection consumer is disabled")
	}
	if feedbackProducer != nil && feedbackTransactionalOutboxEnabled {
		if err := apiHandler.StartFeedbackOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start feedback outbox worker", zap.Error(err))
		} else {
			logger.Info("Feedback transactional outbox worker started")
		}
	}
	if modelFeedbackRevisionProducer != nil && modelFeedbackRevisionAuthorityEnabled {
		if err := apiHandler.StartModelFeedbackRevisionOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Fatal("Failed to start model feedback revision outbox worker", zap.Error(err))
		}
		logger.Info("Model feedback revision outbox worker started after consumer receipt verification")
	} else {
		logger.Info("Model feedback revision producer is disabled; revision outbox rows remain pending")
	}
	if db != nil && alertReportFeatureEnabled {
		if err := apiHandler.StartAlertReportWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start alert-report worker", zap.Error(err))
		} else {
			logger.Info("Alert-report worker started")
		}
	}

	// 初始化反馈持久化 (ClickHouse) — TP/FP 闭环
	if chClient != nil {
		feedbackRepo := api.NewFeedbackRepository(chClient, logger)
		apiHandler.SetFeedbackRepo(feedbackRepo)
		logger.Info("Feedback repository configured; schema is managed by versioned ClickHouse migrations")
	}

	// ==================== 初始化 Kafka Consumer ====================
	// 使用带 Evidence 的构造函数
	var kafkaConsumer *consumer.Consumer
	if !readOnlyVerificationMode {
		kafkaConsumer = consumer.NewConsumerWithEvidence(
			cfg.Kafka,
			cfg.Dedup,
			redisDedup,
			dualWriter,
			evidenceGen,
			arkimeLinkGen,
			logger,
		)
		if cfg.Kafka.WhitelistDetectionMatcherEnabled {
			kafkaConsumer.SetWhitelistMatcher(whitelistRepo)
		}
	}
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

		// 初始化 Auth Service(统一令牌模型 P1:附带 OIDC Provider,使 ValidateToken
		// 具备 Keycloak 访问令牌 JWKS 回退)
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
		authSvc := authService.NewAuthService(userRepo, jwtService, oidcProvider, authCfg, logger, nil)
		realtimeAuthService = authSvc

		// 初始化 Auth Middleware
		authMiddleware = middleware.NewAuthMiddleware(authSvc, logger)
		logger.Info("Auth middleware initialized")
	} else {
		logger.Warn("Authentication is explicitly disabled by configuration")
	}

	// 统一中间件责任链:顺序在单一构造点显式冻结(Recovery→RequestID→Logging→
	// CORS→Metrics→Tenant→Authenticate),各业务路由共享同一链,避免鉴权前置
	// 在个别路由上漂移失效。
	apiMiddlewares := []mux.MiddlewareFunc{
		mux.MiddlewareFunc(httpx.Recovery(logger)),
		mux.MiddlewareFunc(httpx.RequestID()),
		mux.MiddlewareFunc(httpx.Logging(logger)),
		mux.MiddlewareFunc(httpx.CORS(httpx.DefaultCORSConfig())),
		mux.MiddlewareFunc(httpx.Metrics("alert-service")),
		mux.MiddlewareFunc(httpx.TenantExtractor()),
	}
	if authMiddleware != nil {
		apiMiddlewares = append(apiMiddlewares, authMiddleware.Authenticate)
	}
	applyAPIMiddlewares := func(router *mux.Router) {
		router.Use(apiMiddlewares...)
	}
	systemHandler := newAlertSystemHandler(
		chClient,
		db,
		logger,
		api.NewClickHouseEncryptedTrafficStatsService(chClient),
	)
	var campaignProtoReadiness *campaignrail.ProtoProjectionStore
	if cfg.Kafka.CampaignEventEnabled {
		logger.Warn("CAMPAIGN_EVENT_PIPELINE_V2_ENABLED is deprecated and does not enable either campaign JSON rail; set the consumer and dispatcher switches explicitly")
	}
	if cfg.Kafka.CampaignProtoEnabled && !readOnlyVerificationMode {
		candidateSHA256 := strings.TrimSpace(getEnv("CAMPAIGNS_PROTO_CANDIDATE_SHA256", ""))
		protoStore, protoErr := campaignrail.NewProtoProjectionStore(db)
		if protoErr != nil {
			logger.Fatal("Failed to initialize campaigns.v1 projection store", zap.Error(protoErr))
		}
		readinessCtx, readinessCancel := context.WithTimeout(ctx, 10*time.Second)
		protoErr = protoStore.VerifySchema(readinessCtx)
		readinessCancel()
		if protoErr != nil {
			logger.Fatal("campaigns.v1 projection schema is not ready", zap.Error(protoErr))
		}
		campaignProtoReadiness = protoStore
		protoKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignProtoTopic,
			GroupID: cfg.Kafka.CampaignProtoGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create campaigns.v1 Protobuf consumer", zap.Error(consumerErr))
		}
		protoConsumer, consumerErr := consumer.NewCampaignDetectionConsumer(
			protoKafkaConsumer, protoStore, candidateSHA256,
			cfg.Kafka.CampaignProtoTopic, cfg.Kafka.CampaignProtoGroup, logger)
		if consumerErr != nil {
			_ = protoKafkaConsumer.Close()
			logger.Fatal("Failed to initialize campaigns.v1 Protobuf consumer", zap.Error(consumerErr))
		}
		defer protoConsumer.Close()
		apiHandler.AddReadinessCheck("campaigns_proto_consumer_v1", protoConsumer.Ready)
		go func() {
			logger.Info("Starting campaigns.v1 Protobuf consumer; readiness waits for a durable canary receipt",
				zap.String("topic", cfg.Kafka.CampaignProtoTopic), zap.String("group_id", cfg.Kafka.CampaignProtoGroup),
				zap.String("candidate_sha256", candidateSHA256))
			if startErr := protoConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("campaigns.v1 Protobuf consumer stopped", zap.Error(startErr))
				cancel()
			}
		}()
	} else {
		logger.Info("campaigns.v1 Protobuf consumer is disabled")
	}
	attackChainV1Enabled := getBoolEnv("ATTACK_CHAIN_V1_ENABLED", false)
	if attackChainV1Enabled {
		attackChainRepository, attackChainErr := attackchain.NewRepository(db)
		if attackChainErr != nil {
			logger.Fatal("Failed to initialize attack-chain snapshot repository", zap.Error(attackChainErr))
		}
		readinessCtx, readinessCancel := context.WithTimeout(ctx, 10*time.Second)
		attackChainErr = attackChainRepository.VerifySchema(readinessCtx)
		readinessCancel()
		if attackChainErr != nil {
			logger.Fatal("Attack-chain snapshot schema is not ready", zap.Error(attackChainErr))
		}
		systemHandler.SetAttackChainV1Runtime(true, attackChainRepository)
		logger.Info("Versioned attack-chain snapshot API enabled")
	} else {
		systemHandler.SetAttackChainV1Runtime(false, nil)
		logger.Info("Versioned attack-chain snapshot API disabled; compatibility reads remain active")
	}
	topicSnapshotFeatureEnabled := getBoolEnv("TOPIC_SNAPSHOT_V1_ENABLED", true)
	topicExecutorFeatureEnabled := getBoolEnv("TOPIC_EXECUTOR_V2_ENABLED", true) && !readOnlyVerificationMode
	probeOperationPipelineConfig := cfg.ProbeOperation
	if readOnlyVerificationMode {
		probeOperationPipelineConfig = config.ProbeOperationPipelineConfig{}
	}
	auditBatchFeatureEnabled := getBoolEnv("AUDIT_BATCH_FAIL_CLOSED_V1_ENABLED", false) && !readOnlyVerificationMode
	systemHandler.SetCampaignAggregateV2FeatureFlag(campaignAggregateV2Enabled)
	fusionWriterEnabled := getBoolEnv("FUSION_V1_ENABLED", false) && !readOnlyVerificationMode
	systemHandler.SetFusionV1FeatureFlag(false)
	baselineWriterEnabled := getBoolEnv("BEHAVIOR_BASELINE_V1_ENABLED", false) && !readOnlyVerificationMode
	baselineCandidateSHA256 := strings.TrimSpace(getEnv("BEHAVIOR_BASELINE_CANDIDATE_SHA256", ""))
	if baselineWriterEnabled && len(baselineCandidateSHA256) != 64 {
		logger.Fatal("Behavior baseline V1 writer requires a 64-character candidate SHA")
	}
	systemHandler.SetBehaviorBaselineV1Runtime(false, baselineCandidateSHA256)
	systemHandler.SetTopicAlignmentFeatureFlags(topicSnapshotFeatureEnabled, topicExecutorFeatureEnabled)
	systemHandler.SetProbeOperationAckFeatureFlag(probeOperationPipelineConfig.DesiredWriterEnabled)
	fusionKafkaConfig := cfg.Kafka
	if readOnlyVerificationMode {
		fusionKafkaConfig.FusionProjectionEnabled = false
	}
	fusionRuntime, fusionPipelineErr := initFusionProjectionPipeline(ctx, FusionProjectionPipelineDeps{
		Kafka: fusionKafkaConfig, Postgres: db, ClickHouse: chSQLDB,
		CandidateSHA256: strings.TrimSpace(getEnv("FUSION_CANDIDATE_SHA256", "")), Logger: logger,
	})
	if fusionPipelineErr != nil {
		logger.Fatal("Failed to initialize fusion projection pipeline", zap.Error(fusionPipelineErr))
	}
	if fusionRuntime != nil {
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := fusionRuntime.Close(shutdownCtx); err != nil {
				logger.Error("Failed to stop fusion projection pipeline", zap.Error(err))
			}
		}()
	}
	if fusionKafkaConfig.FusionProjectionEnabled {
		logger.Info("Fusion projection consumer is assigned and ready",
			zap.String("topic", fusionKafkaConfig.FusionCommandTopic),
			zap.String("group_id", fusionKafkaConfig.FusionProjectionGroup))
	} else {
		logger.Info("Fusion projection consumer is disabled")
	}
	var fusionCommandProducer *kafka.KeyedProducer
	if fusionWriterEnabled {
		if !fusionKafkaConfig.FusionProjectionEnabled || fusionRuntime == nil || fusionRuntime.readiness == nil {
			logger.Fatal("Fusion V1 writer cannot start before its candidate-bound projection consumer is ready")
		}
		fusionCommandProducer, err = kafka.NewKeyedProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.FusionCommandTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to initialize fusion command producer", zap.Error(err))
		}
		defer fusionCommandProducer.Close()
		systemHandler.SetFusionV1Runtime(strings.TrimSpace(getEnv("FUSION_CANDIDATE_SHA256", "")), fusionRuntime.readiness, fusionCommandProducer)
		if err := systemHandler.StartFusionCommandOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Fatal("Failed to start fusion command outbox worker", zap.Error(err))
		}
		systemHandler.SetFusionV1FeatureFlag(true)
		logger.Info("Fusion V1 authority writer enabled behind current consumer readiness fence")
	} else {
		logger.Info("Fusion V1 authority writer is disabled")
	}
	baselineAckKafkaConfig := cfg.Kafka
	if readOnlyVerificationMode {
		baselineAckKafkaConfig.BaselineActivationAckEnabled = false
	}
	baselineAckRuntime, baselineAckPipelineErr := initBaselineActivationAckPipeline(ctx, BaselineAckPipelineDeps{
		Kafka:           baselineAckKafkaConfig,
		Postgres:        db,
		CandidateSHA256: baselineCandidateSHA256,
		Logger:          logger,
	})
	if baselineAckPipelineErr != nil {
		logger.Fatal("Failed to initialize behavior baseline activation ACK pipeline", zap.Error(baselineAckPipelineErr))
	}
	if baselineAckRuntime != nil {
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := baselineAckRuntime.Close(shutdownCtx); err != nil {
				logger.Error("Failed to stop behavior baseline activation ACK pipeline", zap.Error(err))
			}
		}()
	}
	if baselineAckKafkaConfig.BaselineActivationAckEnabled {
		logger.Info("Behavior baseline activation ACK consumer is assigned and ready",
			zap.String("topic", baselineAckKafkaConfig.BaselineActivationAckTopic),
			zap.String("group_id", baselineAckKafkaConfig.BaselineActivationAckGroup),
			zap.String("candidate_sha256", baselineCandidateSHA256))
	} else {
		logger.Info("Behavior baseline activation ACK consumer is disabled")
	}
	baselineLifecycleDispatcherEnabled := getBoolEnv("BEHAVIOR_BASELINE_LIFECYCLE_DISPATCHER_V1_ENABLED", false) && !readOnlyVerificationMode
	var baselineLifecycleDispatcher *baseline.LifecycleOutboxDispatcher
	var baselineLifecycleProducer *kafka.KeyedProducer
	if baselineLifecycleDispatcherEnabled {
		if baselineAckRuntime == nil || baselineAckRuntime.readiness == nil ||
			!baselineAckKafkaConfig.BaselineActivationAckEnabled || cfg.Kafka.BaselineLifecycleTopic != baseline.LifecycleTopic {
			logger.Fatal("Behavior baseline lifecycle dispatcher requires the ready ACK consumer and exact lifecycle topic")
		}
		baselineLifecycleProducer, err = kafka.NewKeyedProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.BaselineLifecycleTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to initialize behavior baseline lifecycle producer", zap.Error(err))
		}
		defer baselineLifecycleProducer.Close()
		baselineLifecycleDispatcher, err = baseline.NewLifecycleOutboxDispatcher(db, baselineLifecycleProducer,
			baselineAckRuntime.readiness, baselineCandidateSHA256)
		if err != nil {
			logger.Fatal("Failed to initialize behavior baseline lifecycle dispatcher", zap.Error(err))
		}
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				if _, drainErr := baselineLifecycleDispatcher.Drain(ctx, 50); drainErr != nil && ctx.Err() == nil {
					logger.Warn("Failed to drain behavior baseline lifecycle outbox", zap.Error(drainErr))
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		logger.Info("Behavior baseline lifecycle dispatcher enabled",
			zap.String("topic", cfg.Kafka.BaselineLifecycleTopic), zap.String("candidate_sha256", baselineCandidateSHA256))
	} else {
		logger.Info("Behavior baseline lifecycle dispatcher is disabled")
	}
	baselineWorkerEnabled := getBoolEnv("BEHAVIOR_BASELINE_BUILD_WORKER_V1_ENABLED", false) && !readOnlyVerificationMode
	if baselineWorkerEnabled {
		if len(baselineCandidateSHA256) != 64 || chSQLDB == nil || db == nil {
			logger.Fatal("Behavior baseline build worker requires PostgreSQL, ClickHouse and the exact candidate SHA")
		}
		sampleReader, sampleReaderErr := baseline.NewClickHouseSampleReader(chSQLDB)
		if sampleReaderErr != nil {
			logger.Fatal("Failed to initialize behavior baseline ClickHouse reader", zap.Error(sampleReaderErr))
		}
		baselineWorker, baselineWorkerErr := baseline.NewWorker(db, sampleReader, baselineCandidateSHA256,
			"alert-service:"+getEnv("POD_NAME", "local"))
		if baselineWorkerErr != nil {
			logger.Fatal("Failed to initialize behavior baseline worker", zap.Error(baselineWorkerErr))
		}
		go func() {
			if err := baseline.RunWorkerLoop(ctx, baselineWorker, 2*time.Second); err != nil && err != context.Canceled {
				logger.Error("Behavior baseline build worker stopped", zap.Error(err))
				cancel()
			}
		}()
		logger.Info("Behavior baseline build worker enabled", zap.String("candidate_sha256", baselineCandidateSHA256))
	} else {
		logger.Info("Behavior baseline build worker is disabled")
	}
	if baselineWriterEnabled {
		if db == nil || !baselineAckKafkaConfig.BaselineActivationAckEnabled || baselineAckRuntime == nil || baselineAckRuntime.readiness == nil {
			logger.Fatal("Behavior baseline V1 writer cannot start before its candidate-bound ACK consumer is ready")
		}
		if !baselineLifecycleDispatcherEnabled || baselineLifecycleDispatcher == nil || baselineLifecycleProducer == nil {
			logger.Fatal("Behavior baseline V1 writer cannot start before its lifecycle dispatcher is ready")
		}
		systemHandler.SetBehaviorBaselineV1Runtime(true, baselineCandidateSHA256)
		logger.Info("Behavior baseline V1 authority writer enabled; projection dispatch remains separately gated",
			zap.String("candidate_sha256", baselineCandidateSHA256))
	} else {
		systemHandler.SetBehaviorBaselineV1Runtime(false, baselineCandidateSHA256)
		logger.Info("Behavior baseline V1 authority writer is disabled")
	}
	var auditBatchProducer *kafka.Producer
	if auditBatchFeatureEnabled {
		auditBatchProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: "audit.logs", BatchSize: 200,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Audit batch ingress is enabled but Kafka producer initialization failed", zap.Error(err))
		}
		defer auditBatchProducer.Close()
		systemHandler.SetAuditBatchProducer(auditBatchProducer)
		logger.Info("Fail-closed audit batch ingress enabled", zap.String("topic", auditBatchProducer.Topic()))
	} else {
		logger.Info("Fail-closed audit batch ingress is disabled")
	}
	if campaignSOARExecutorURL := strings.TrimSpace(getEnv("CAMPAIGN_SOAR_EXECUTOR_URL", "")); campaignSOARExecutorURL != "" {
		campaignSOARExecutor, executorErr := api.NewHTTPCampaignSOARExecutor(
			campaignSOARExecutorURL,
			getEnv("CAMPAIGN_SOAR_EXECUTOR_TOKEN", ""),
			time.Duration(getIntEnv("CAMPAIGN_SOAR_EXECUTOR_TIMEOUT_SECONDS", 30))*time.Second,
		)
		if executorErr != nil {
			logger.Fatal("Invalid campaign SOAR executor configuration", zap.Error(executorErr))
		}
		systemHandler.SetCampaignSOARExecutor(campaignSOARExecutor)
		logger.Info("Campaign SOAR provider adapter configured")
	} else {
		logger.Warn("Campaign SOAR provider adapter is not configured; approved jobs will remain awaiting executor")
	}
	var campaignAggregateEventProducer *kafka.Producer
	var campaignMembershipEventProducer *kafka.Producer
	var campaignJSONReadiness *campaignrail.ProtoProjectionStore
	if (cfg.Kafka.CampaignEventConsumerEnabled || cfg.Kafka.CampaignEventDispatcherEnabled) && !readOnlyVerificationMode {
		if db == nil {
			logger.Fatal("Campaign JSON event rails require PostgreSQL")
		}
		campaignJSONReadiness, err = campaignrail.NewProtoProjectionStore(db)
		if err != nil {
			logger.Fatal("Failed to initialize campaign JSON readiness authority", zap.Error(err))
		}
		readinessCtx, readinessCancel := context.WithTimeout(ctx, 10*time.Second)
		err = campaignJSONReadiness.VerifySchema(readinessCtx)
		readinessCancel()
		if err != nil {
			logger.Fatal("Campaign JSON readiness schema is unavailable", zap.Error(err))
		}
	}
	if cfg.Kafka.CampaignEventConsumerEnabled && !readOnlyVerificationMode {
		candidateSHA256 := strings.TrimSpace(getEnv("CAMPAIGN_JSON_V2_CANDIDATE_SHA256", ""))
		campaignAggregateKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignEventTopic,
			GroupID: cfg.Kafka.CampaignEventGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create campaign aggregate projection consumer", zap.Error(consumerErr))
		}
		campaignAggregateConsumer, consumerErr := consumer.NewCampaignEventConsumer(
			campaignAggregateKafkaConsumer, systemHandler, "aggregate", cfg.Kafka.CampaignEventTopic, logger)
		if consumerErr == nil {
			consumerErr = campaignAggregateConsumer.SetReadinessAuthority(
				campaignJSONReadiness, candidateSHA256, cfg.Kafka.CampaignEventGroup)
		}
		if consumerErr != nil {
			_ = campaignAggregateKafkaConsumer.Close()
			logger.Fatal("Failed to initialize campaign aggregate projection consumer", zap.Error(consumerErr))
		}
		defer campaignAggregateConsumer.Close()
		apiHandler.AddReadinessCheck("campaign_aggregate_json_v2", campaignAggregateConsumer.Ready)
		go func() {
			logger.Info("Starting campaign aggregate projection consumer", zap.String("topic", cfg.Kafka.CampaignEventTopic), zap.String("group_id", cfg.Kafka.CampaignEventGroup))
			if startErr := campaignAggregateConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Campaign aggregate projection consumer stopped", zap.Error(startErr))
				cancel()
			}
		}()

		campaignMembershipKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignMemberTopic,
			GroupID: cfg.Kafka.CampaignMemberGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create campaign membership projection consumer", zap.Error(consumerErr))
		}
		campaignMembershipConsumer, consumerErr := consumer.NewCampaignEventConsumer(
			campaignMembershipKafkaConsumer, systemHandler, "membership", cfg.Kafka.CampaignMemberTopic, logger)
		if consumerErr == nil {
			consumerErr = campaignMembershipConsumer.SetReadinessAuthority(
				campaignJSONReadiness, candidateSHA256, cfg.Kafka.CampaignMemberGroup)
		}
		if consumerErr != nil {
			_ = campaignMembershipKafkaConsumer.Close()
			logger.Fatal("Failed to initialize campaign membership projection consumer", zap.Error(consumerErr))
		}
		defer campaignMembershipConsumer.Close()
		apiHandler.AddReadinessCheck("campaign_membership_json_v2", campaignMembershipConsumer.Ready)
		go func() {
			logger.Info("Starting campaign membership projection consumer", zap.String("topic", cfg.Kafka.CampaignMemberTopic), zap.String("group_id", cfg.Kafka.CampaignMemberGroup))
			if startErr := campaignMembershipConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Campaign membership projection consumer stopped", zap.Error(startErr))
				cancel()
			}
		}()
		logger.Info("Campaign JSON V2 consumers started; dispatcher remains independently gated",
			zap.String("candidate_sha256", candidateSHA256))
	} else {
		logger.Info("Campaign JSON V2 consumers are disabled")
	}
	if cfg.Kafka.CampaignEventDispatcherEnabled && !readOnlyVerificationMode {
		if !cfg.Kafka.CampaignEventConsumerEnabled || campaignJSONReadiness == nil {
			logger.Fatal("Campaign JSON V2 dispatcher cannot start before both consumers are enabled")
		}
		candidateSHA256 := strings.TrimSpace(getEnv("CAMPAIGN_JSON_V2_CANDIDATE_SHA256", ""))
		dispatchAdmission := func(admissionCtx context.Context) error {
			if err := campaignJSONReadiness.AssertConsumerReady(admissionCtx,
				campaignrail.AggregateJSONRailID, candidateSHA256,
				cfg.Kafka.CampaignEventTopic, cfg.Kafka.CampaignEventGroup); err != nil {
				return fmt.Errorf("aggregate JSON consumer: %w", err)
			}
			if err := campaignJSONReadiness.AssertConsumerReady(admissionCtx,
				campaignrail.MembershipJSONRailID, candidateSHA256,
				cfg.Kafka.CampaignMemberTopic, cfg.Kafka.CampaignMemberGroup); err != nil {
				return fmt.Errorf("membership JSON consumer: %w", err)
			}
			return nil
		}
		readinessCtx, readinessCancel := context.WithTimeout(ctx, 10*time.Second)
		err = dispatchAdmission(readinessCtx)
		readinessCancel()
		if err != nil {
			logger.Fatal("Campaign JSON V2 dispatcher lacks both durable consumer canary receipts", zap.Error(err))
		}
		campaignAggregateEventProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignEventTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to create campaign aggregate event producer", zap.Error(err))
		}
		defer campaignAggregateEventProducer.Close()
		campaignMembershipEventProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignMemberTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Fatal("Failed to create campaign membership event producer", zap.Error(err))
		}
		defer campaignMembershipEventProducer.Close()
		systemHandler.SetCampaignEventProducers(campaignAggregateEventProducer, campaignMembershipEventProducer)
		systemHandler.SetCampaignDispatcherAdmission(dispatchAdmission)
		if err := systemHandler.StartCampaignEventOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Fatal("Failed to start campaign event outbox worker", zap.Error(err))
		}
		logger.Info("Campaign JSON V2 dispatcher started after both consumer receipts",
			zap.String("aggregate_topic", cfg.Kafka.CampaignEventTopic), zap.String("membership_topic", cfg.Kafka.CampaignMemberTopic))
	} else {
		logger.Info("Campaign JSON V2 dispatcher is disabled")
	}
	if cfg.Kafka.CampaignRailCorrelationEnabled && !readOnlyVerificationMode {
		if !cfg.Kafka.CampaignProtoEnabled || !cfg.Kafka.CampaignEventConsumerEnabled ||
			!cfg.Kafka.CampaignEventDispatcherEnabled || campaignProtoReadiness == nil {
			logger.Fatal("Campaign rail correlation requires the Protobuf consumer and both JSON consumer/dispatcher stages")
		}
		protoCandidateSHA := strings.TrimSpace(getEnv("CAMPAIGNS_PROTO_CANDIDATE_SHA256", ""))
		jsonCandidateSHA := strings.TrimSpace(getEnv("CAMPAIGN_JSON_V2_CANDIDATE_SHA256", ""))
		correlationContractSHA := strings.TrimSpace(getEnv("CAMPAIGN_RAIL_CORRELATION_CONTRACT_SHA256", ""))
		if correlationContractSHA != "f4f564d6d22084c1202634af99fabe29067bd51f5748dfd30b9fd48f0541980b" {
			logger.Fatal("Campaign rail correlation contract candidate SHA is missing or stale")
		}
		correlationAdmission := func(admissionCtx context.Context) error {
			checks := []struct {
				rail, candidate, topic, group string
			}{
				{campaignrail.ProtoRailID, protoCandidateSHA, cfg.Kafka.CampaignProtoTopic, cfg.Kafka.CampaignProtoGroup},
				{campaignrail.AggregateJSONRailID, jsonCandidateSHA, cfg.Kafka.CampaignEventTopic, cfg.Kafka.CampaignEventGroup},
				{campaignrail.MembershipJSONRailID, jsonCandidateSHA, cfg.Kafka.CampaignMemberTopic, cfg.Kafka.CampaignMemberGroup},
			}
			for _, check := range checks {
				if err := campaignProtoReadiness.AssertConsumerReady(admissionCtx,
					check.rail, check.candidate, check.topic, check.group); err != nil {
					return fmt.Errorf("%s readiness: %w", check.rail, err)
				}
			}
			return nil
		}
		admissionCtx, admissionCancel := context.WithTimeout(ctx, 10*time.Second)
		err = correlationAdmission(admissionCtx)
		admissionCancel()
		if err != nil {
			logger.Fatal("Campaign rail correlation lacks three durable candidate-bound consumer receipts", zap.Error(err))
		}
		systemHandler.SetCampaignRailCorrelationAdmission(correlationAdmission)
		if err := systemHandler.StartCampaignRailCorrelationWorker(
			ctx,
			time.Duration(getIntEnv("CAMPAIGN_RAIL_CORRELATION_INTERVAL_SECONDS", 60))*time.Second,
			time.Duration(getIntEnv("CAMPAIGN_RAIL_CORRELATION_WINDOW_SECONDS", 300))*time.Second,
			time.Duration(getIntEnv("CAMPAIGN_RAIL_CORRELATION_CLOSE_LAG_SECONDS", 60))*time.Second,
			getIntEnv("CAMPAIGN_RAIL_CORRELATION_MAX_CAMPAIGNS", 1000),
		); err != nil {
			logger.Fatal("Failed to start campaign rail correlation worker", zap.Error(err))
		}
		logger.Info("Campaign rail correlation worker started after three durable receipts",
			zap.String("contract_sha256", correlationContractSHA))
	} else {
		logger.Info("Campaign rail correlation worker is disabled")
	}
	if db != nil && campaignAggregateV2Enabled && !readOnlyVerificationMode {
		if err := systemHandler.StartCampaignReportWorker(ctx, 2*time.Second); err != nil {
			logger.Fatal("Failed to start campaign report executor", zap.Error(err))
		}
		logger.Info("Campaign report executor started")
		if err := systemHandler.StartCampaignSOARWorker(ctx, 2*time.Second); err != nil {
			logger.Fatal("Failed to start campaign SOAR executor", zap.Error(err))
		}
	}
	if cfg.CampaignProjection.Enabled && !readOnlyVerificationMode {
		if !cfg.Kafka.CampaignEventConsumerEnabled || !cfg.Kafka.CampaignEventDispatcherEnabled {
			logger.Fatal("Campaign target projection requires both JSON consumers and the independently gated dispatcher")
		}
		if db == nil {
			logger.Fatal("Campaign target projection requires PostgreSQL")
		}
		clickHouseProjection, projectionErr := api.NewClickHouseCampaignProjection(
			chClient,
			cfg.CampaignProjection.ClickHouseTable,
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize campaign ClickHouse projection", zap.Error(projectionErr))
		}
		openSearchProjection, projectionErr := api.NewOpenSearchCampaignProjection(
			cfg.OpenSearch.Addresses,
			cfg.OpenSearch.Username,
			cfg.OpenSearch.Password,
			cfg.CampaignProjection.OpenSearchWriteAlias,
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize campaign OpenSearch projection", zap.Error(projectionErr))
		}
		campaignNebulaStore, projectionErr := graphNebula.NewWorkbenchStore(
			graphConfig.NebulaConfig{
				Enabled:     true,
				Addresses:   cfg.CampaignProjection.Nebula.Addresses,
				Username:    cfg.CampaignProjection.Nebula.Username,
				Password:    cfg.CampaignProjection.Nebula.Password,
				Space:       cfg.CampaignProjection.Nebula.Space,
				Timeout:     cfg.CampaignProjection.Nebula.Timeout,
				IdleTime:    cfg.CampaignProjection.Nebula.IdleTime,
				MaxPoolSize: cfg.CampaignProjection.Nebula.MaxPoolSize,
				MinPoolSize: cfg.CampaignProjection.Nebula.MinPoolSize,
			},
			logger,
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize campaign NebulaGraph projection store", zap.Error(projectionErr))
		}
		defer campaignNebulaStore.Close()
		nebulaProjection, projectionErr := api.NewNebulaCampaignProjection(campaignNebulaStore)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize campaign NebulaGraph projection", zap.Error(projectionErr))
		}
		hostname, _ := os.Hostname()
		projectionWorker, projectionErr := api.NewCampaignTargetProjectionWorker(
			db,
			[]api.CampaignProjectionTarget{clickHouseProjection, openSearchProjection, nebulaProjection},
			api.CampaignTargetProjectionWorkerConfig{
				WorkerID:    "campaign-target-projection/" + hostname + "/" + uuid.NewString(),
				Lease:       cfg.CampaignProjection.Lease,
				Interval:    cfg.CampaignProjection.Interval,
				MaxAttempts: cfg.CampaignProjection.MaxAttempts,
				Logger:      logger,
			},
		)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize campaign target projection worker", zap.Error(projectionErr))
		}
		campaignProjectionReadiness := func(readinessCtx context.Context) error {
			if readinessErr := projectionWorker.VerifySchema(readinessCtx); readinessErr != nil {
				return readinessErr
			}
			if readinessErr := clickHouseProjection.Ready(readinessCtx); readinessErr != nil {
				return readinessErr
			}
			if readinessErr := openSearchProjection.Ready(readinessCtx); readinessErr != nil {
				return readinessErr
			}
			return nebulaProjection.Ready(readinessCtx)
		}
		readinessCtx, readinessCancel := context.WithTimeout(ctx, 15*time.Second)
		projectionErr = campaignProjectionReadiness(readinessCtx)
		readinessCancel()
		if projectionErr != nil {
			logger.Fatal("Campaign target projection dependencies are not ready", zap.Error(projectionErr))
		}
		apiHandler.AddReadinessCheck("campaign_target_projection", campaignProjectionReadiness)
		go projectionWorker.Run(ctx)
		logger.Info(
			"Campaign ClickHouse, OpenSearch and NebulaGraph projection worker started",
			zap.String("clickhouse_table", cfg.CampaignProjection.ClickHouseTable),
			zap.String("opensearch_write_alias", cfg.CampaignProjection.OpenSearchWriteAlias),
			zap.String("nebula_space", cfg.CampaignProjection.Nebula.Space),
		)
	} else {
		logger.Warn("Campaign target projection V2 is disabled")
	}
	var topicActionProducer *kafka.Producer
	if topicExecutorFeatureEnabled {
		topicActionProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: "traffic.topic.action.v2", BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Warn("Failed to create topic-action Kafka producer; durable outbox will remain pending", zap.Error(err))
			topicActionProducer = nil
		} else {
			defer topicActionProducer.Close()
			systemHandler.SetTopicActionProducer(topicActionProducer)
		}
	}
	if db != nil && topicExecutorFeatureEnabled {
		if err := systemHandler.StartTopicActionWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start topic-action worker", zap.Error(err))
		} else {
			logger.Info("Topic-action worker started")
		}
	}
	if db != nil && topicExecutorFeatureEnabled && topicActionProducer != nil {
		if err := systemHandler.StartTopicActionOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start topic-action outbox worker", zap.Error(err))
		} else {
			logger.Info("Topic-action outbox worker started", zap.String("topic", topicActionProducer.Topic()))
		}
	}
	if db != nil && topicExecutorFeatureEnabled {
		topicActionKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.TopicActionEventTopic,
			GroupID: cfg.Kafka.TopicActionEventGroup, MaxRetries: 3, RetryBackoff: time.Second,
			EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Warn("Failed to create topic-action projection consumer", zap.Error(consumerErr))
		} else {
			topicActionEventConsumer, initErr := consumer.NewTopicActionEventConsumer(
				topicActionKafkaConsumer, systemHandler, logger,
			)
			if initErr != nil {
				_ = topicActionKafkaConsumer.Close()
				logger.Warn("Failed to initialize topic-action projection consumer", zap.Error(initErr))
			} else {
				defer topicActionEventConsumer.Close()
				go func() {
					logger.Info(
						"Starting topic-action projection consumer",
						zap.String("topic", cfg.Kafka.TopicActionEventTopic),
						zap.String("group_id", cfg.Kafka.TopicActionEventGroup),
						zap.String("dlq_topic", "dlq.v1"),
					)
					if startErr := topicActionEventConsumer.Start(ctx); startErr != nil &&
						startErr != context.Canceled {
						logger.Error("Topic-action projection consumer stopped", zap.Error(startErr))
					}
				}()
			}
		}
	}
	probeOperationRuntime, probeOperationErr := initProbeOperationPipelines(
		ctx,
		ProbeOperationPipelineDeps{Kafka: cfg.Kafka, DB: db, Handler: systemHandler, Logger: logger},
		probeOperationPipelineConfig,
	)
	if probeOperationErr != nil {
		logger.Fatal("Failed to initialize probe operation generation pipeline", zap.Error(probeOperationErr))
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if closeErr := probeOperationRuntime.Close(shutdownCtx); closeErr != nil {
			logger.Error("Probe operation generation pipeline did not close cleanly", zap.Error(closeErr))
		}
	}()

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
	// P2 契约解释器:对命中 m10 契约的 /v1/* 操作逐操作判定 required_scope
	// (alert 29 操作/whitelist 5 操作/dashboard 等),未命中契约路径原样放行;
	// /internal/v1 内部路由不受影响。
	if mode := strings.TrimSpace(getEnv("AUTHZ_CONTRACT_MODE", "")); mode == "enforce" || mode == "shadow" {
		apiRouter.Use(authz.EnforceContract(alertContractPrincipal, mode, nil, logger, alertContractDenyAudit(auditLogger)))
		logger.Info("Contract interpreter enabled on API router", zap.String("mode", mode))
	}
	internalRouter := r.PathPrefix("/internal/v1").Subrouter()
	applyAPIMiddlewares(internalRouter)
	systemHandler.RegisterInternalRoutes(internalRouter)

	// 注册 API 路由
	apiHandler.RegisterRoutes(apiRouter)
	alertBatchAssignmentEnabled := getBoolEnv("ALERT_BATCH_ASSIGNMENT_V1_ENABLED", false) && !readOnlyVerificationMode
	alertBatchAssignmentPipelineEnabled := getBoolEnv("ALERT_BATCH_ASSIGNMENT_PIPELINE_V1_ENABLED", false) && !readOnlyVerificationMode
	alertBatchAssignmentCompensationEnabled := getBoolEnv("ALERT_BATCH_ASSIGNMENT_COMPENSATION_V1_ENABLED", false) && !readOnlyVerificationMode
	if alertBatchAssignmentPipelineEnabled && !alertBatchAssignmentEnabled {
		logger.Fatal("Alert batch assignment execution pipeline requires ALERT_BATCH_ASSIGNMENT_V1_ENABLED")
	}
	if alertBatchAssignmentCompensationEnabled && (!alertBatchAssignmentEnabled || !alertBatchAssignmentPipelineEnabled) {
		logger.Fatal("Alert batch assignment compensation requires the batch API and execution pipeline")
	}
	if alertBatchAssignmentEnabled && db == nil {
		logger.Fatal("Alert batch assignment requires PostgreSQL")
	}
	alertBatchSelectionSigningSecret := strings.TrimSpace(os.Getenv("ALERT_BATCH_SELECTION_SIGNING_SECRET"))
	if alertBatchAssignmentEnabled && len(alertBatchSelectionSigningSecret) < 32 {
		logger.Fatal("ALERT_BATCH_SELECTION_SIGNING_SECRET must contain at least 32 bytes when alert batch assignment is enabled")
	}
	alertBatchAssignmentHandler := api.NewAlertBatchAssignmentHandler(db, logger, alertBatchAssignmentEnabled, alertBatchSelectionSigningSecret)
	alertBatchAssignmentHandler.SetCompensationEnabled(alertBatchAssignmentCompensationEnabled)
	if alertBatchAssignmentEnabled {
		verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
		err = alertBatchAssignmentHandler.VerifySchema(verifyCtx)
		cancelVerify()
		if err != nil {
			logger.Fatal("Alert batch assignment schema is unavailable", zap.Error(err))
		}
	}
	alertBatchAssignmentHandler.RegisterRoutes(apiRouter)
	if alertBatchAssignmentPipelineEnabled {
		topic := strings.TrimSpace(getEnv("ALERT_BATCH_ASSIGNMENT_EVENT_TOPIC", consumer.AlertAssignmentEventTopic))
		groupID := strings.TrimSpace(getEnv("ALERT_BATCH_ASSIGNMENT_EVENT_GROUP", "alert-service-batch-assignment-execution-v1"))
		if groupID == "" {
			logger.Fatal("ALERT_BATCH_ASSIGNMENT_EVENT_GROUP must not be empty")
		}
		producer, producerErr := kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: topic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if producerErr != nil {
			logger.Fatal("Failed to create alert batch assignment event producer", zap.Error(producerErr))
		}
		defer producer.Close()
		pipeline, pipelineErr := consumer.NewAlertBatchAssignmentPipeline(db, alertService, producer.Send, topic, logger)
		if pipelineErr != nil {
			logger.Fatal("Failed to initialize alert batch assignment pipeline", zap.Error(pipelineErr))
		}
		verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 10*time.Second)
		pipelineErr = pipeline.VerifySchema(verifyCtx)
		cancelVerify()
		if pipelineErr != nil {
			logger.Fatal("Alert batch assignment execution schema is unavailable", zap.Error(pipelineErr))
		}
		if workerErr := pipeline.StartOutboxWorker(ctx, 2*time.Second); workerErr != nil {
			logger.Fatal("Failed to start alert batch assignment outbox worker", zap.Error(workerErr))
		}
		kafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: topic, GroupID: groupID,
			MinBytes: 1, MaxWait: 500 * time.Millisecond, MaxRetries: 3, RetryBackoff: time.Second,
			EnableDLQ: true, DLQTopic: "dlq.v1", CommitOnDLQSuccess: true,
			CommitOnHandlerError: false, DLQPermanentOnly: true, Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create alert batch assignment event consumer", zap.Error(consumerErr))
		}
		kafkaConsumer.SetDLQAcknowledgementBarrier(pipeline.RecordDLQAcknowledgement)
		eventConsumer, consumerErr := consumer.NewAlertBatchAssignmentEventConsumer(kafkaConsumer, pipeline)
		if consumerErr != nil {
			_ = kafkaConsumer.Close()
			logger.Fatal("Failed to initialize alert batch assignment event consumer", zap.Error(consumerErr))
		}
		defer eventConsumer.Close()
		go func() {
			logger.Info("Starting alert batch assignment event consumer", zap.String("topic", topic), zap.String("group_id", groupID))
			if startErr := eventConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Alert batch assignment event consumer stopped", zap.Error(startErr))
			}
		}()
		logger.Info("Alert batch assignment execution pipeline started", zap.String("topic", topic), zap.String("group_id", groupID))
	} else {
		logger.Info("Alert batch assignment execution pipeline is disabled")
	}

	// Dashboard API — 实时统计 (Web UI 大屏)
	dashboardHandler := api.NewDashboardHandler(chClient, logger)
	dashboardSnapshotHandler := api.NewDashboardSnapshotHandler(chClient, db, osRepo, rdb, logger, getBoolEnv("DASHBOARD_SNAPSHOT_V1_ENABLED", false))
	dashboardSnapshotHandler.RegisterRoutes(apiRouter)
	encryptedTrafficSnapshotEnabled := getBoolEnv("ENCRYPTED_TRAFFIC_SNAPSHOT_V1_ENABLED", false)
	if encryptedTrafficSnapshotEnabled && len(strings.TrimSpace(cfg.Auth.JWTSecretKey)) < 32 {
		logger.Fatal("Encrypted traffic snapshot V1 requires a continuation signing key of at least 32 characters")
	}
	encryptedTrafficSnapshotHandler := api.NewEncryptedTrafficSnapshotHandler(
		chClient,
		logger,
		encryptedTrafficSnapshotEnabled,
		cfg.Auth.JWTSecretKey,
	)
	encryptedTrafficSnapshotHandler.RegisterRoutes(apiRouter)
	apiRouter.HandleFunc("/dashboard/stats", dashboardHandler.GetStats).Methods("GET")
	apiRouter.HandleFunc("/dashboard/alerts/trend", dashboardHandler.GetAlertTrend).Methods("GET")
	apiRouter.HandleFunc("/dashboard/attack-phases", dashboardHandler.GetAttackPhases).Methods("GET")
	apiRouter.HandleFunc("/dashboard/top-ips/{type}", dashboardHandler.GetTopIPs).Methods("GET")
	apiRouter.HandleFunc("/dashboard/encrypted/trend", dashboardHandler.GetEncryptedTrend).Methods("GET")
	dashboardTaskHandler := api.NewDashboardTaskHandler(db, logger, getBoolEnv("DASHBOARD_TASK_V2_ENABLED", true))
	dashboardTaskPipelineEnabled := getBoolEnv("DASHBOARD_TASK_PIPELINE_V1_ENABLED", false) && !readOnlyVerificationMode
	dashboardTaskCompensationEnabled := getBoolEnv("DASHBOARD_TASK_COMPENSATION_V1_ENABLED", false) && dashboardTaskPipelineEnabled
	dashboardTaskProviderAuthorityLookupEnabled := getBoolEnv("DASHBOARD_TASK_PROVIDER_AUTHORITY_LOOKUP_V1_ENABLED", false) && dashboardTaskPipelineEnabled
	dashboardTaskHandler.EnableCompensation(dashboardTaskCompensationEnabled)
	dashboardTaskHandler.RegisterRoutes(apiRouter)
	if dashboardTaskPipelineEnabled {
		if db == nil {
			logger.Fatal("Dashboard task execution pipeline requires PostgreSQL")
		}
		executorURL := strings.TrimSpace(getEnv("DASHBOARD_TASK_EXECUTOR_URL", ""))
		if executorURL == "" {
			logger.Fatal("Dashboard task execution pipeline requires DASHBOARD_TASK_EXECUTOR_URL")
		}
		executor, executorErr := api.NewHTTPDashboardTaskExecutor(
			executorURL,
			getEnv("DASHBOARD_TASK_EXECUTOR_TOKEN", ""),
			time.Duration(getIntEnv("DASHBOARD_TASK_EXECUTOR_TIMEOUT_SECONDS", 30))*time.Second,
		)
		if executorErr != nil {
			logger.Fatal("Invalid dashboard task executor configuration", zap.Error(executorErr))
		}
		if dashboardTaskProviderAuthorityLookupEnabled {
			executorLookupURL := strings.TrimSpace(getEnv("DASHBOARD_TASK_EXECUTOR_LOOKUP_URL", ""))
			if executorLookupURL == "" {
				logger.Fatal("Dashboard task provider authority lookup requires DASHBOARD_TASK_EXECUTOR_LOOKUP_URL")
			}
			if lookupErr := executor.ConfigureAuthorityLookup(executorLookupURL); lookupErr != nil {
				logger.Fatal("Invalid dashboard task executor authority lookup configuration", zap.Error(lookupErr))
			}
		}
		topic := strings.TrimSpace(getEnv("DASHBOARD_TASK_EVENT_TOPIC", "dashboard.task.events.v1"))
		producer, producerErr := kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: topic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if producerErr != nil {
			logger.Fatal("Failed to create dashboard task event producer", zap.Error(producerErr))
		}
		defer producer.Close()
		pipeline, pipelineErr := api.NewDashboardTaskPipeline(db, executor, producer.Send, topic, logger)
		if pipelineErr != nil {
			logger.Fatal("Failed to initialize dashboard task pipeline", zap.Error(pipelineErr))
		}
		if workerErr := pipeline.StartOutboxWorker(ctx, 2*time.Second); workerErr != nil {
			logger.Fatal("Failed to start dashboard task outbox worker", zap.Error(workerErr))
		}
		if workerErr := pipeline.StartExecutionWorker(ctx, 2*time.Second); workerErr != nil {
			logger.Fatal("Failed to start dashboard task execution worker", zap.Error(workerErr))
		}
		if dashboardTaskCompensationEnabled {
			compensatorURL := strings.TrimSpace(getEnv("DASHBOARD_TASK_COMPENSATOR_URL", ""))
			if compensatorURL == "" {
				logger.Fatal("Dashboard task compensation requires DASHBOARD_TASK_COMPENSATOR_URL")
			}
			compensator, compensatorErr := api.NewHTTPDashboardTaskCompensator(
				compensatorURL, getEnv("DASHBOARD_TASK_COMPENSATOR_TOKEN", ""),
				time.Duration(getIntEnv("DASHBOARD_TASK_COMPENSATOR_TIMEOUT_SECONDS", 30))*time.Second,
			)
			if compensatorErr != nil {
				logger.Fatal("Invalid dashboard task compensator configuration", zap.Error(compensatorErr))
			}
			if dashboardTaskProviderAuthorityLookupEnabled {
				compensatorLookupURL := strings.TrimSpace(getEnv("DASHBOARD_TASK_COMPENSATOR_LOOKUP_URL", ""))
				if compensatorLookupURL == "" {
					logger.Fatal("Dashboard task provider authority lookup requires DASHBOARD_TASK_COMPENSATOR_LOOKUP_URL when compensation is enabled")
				}
				if lookupErr := compensator.ConfigureAuthorityLookup(compensatorLookupURL); lookupErr != nil {
					logger.Fatal("Invalid dashboard task compensator authority lookup configuration", zap.Error(lookupErr))
				}
			}
			if enableErr := pipeline.EnableCompensation(compensator); enableErr != nil {
				logger.Fatal("Failed to enable dashboard task compensation", zap.Error(enableErr))
			}
			if workerErr := pipeline.StartCompensationWorker(ctx, 2*time.Second); workerErr != nil {
				logger.Fatal("Failed to start dashboard task compensation worker", zap.Error(workerErr))
			}
		}
		kafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: topic,
			GroupID: "alert-service-dashboard-task-execution-v1", MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create dashboard task event consumer", zap.Error(consumerErr))
		}
		kafkaConsumer.SetDLQAcknowledgementBarrier(pipeline.RecordDLQAcknowledgement)
		eventConsumer, consumerErr := api.NewDashboardTaskEventConsumer(kafkaConsumer, pipeline)
		if consumerErr != nil {
			_ = kafkaConsumer.Close()
			logger.Fatal("Failed to initialize dashboard task event consumer", zap.Error(consumerErr))
		}
		defer eventConsumer.Close()
		go func() {
			logger.Info("Starting dashboard task event consumer", zap.String("topic", topic),
				zap.String("group_id", "alert-service-dashboard-task-execution-v1"))
			if startErr := eventConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Dashboard task event consumer stopped", zap.Error(startErr))
			}
		}()
		logger.Info("Dashboard task execution pipeline started", zap.String("topic", topic))
	} else {
		logger.Info("Dashboard task execution pipeline is disabled")
	}
	logger.Info("Dashboard API registered (unified-snapshot/stats/trend/phases/top-ips/encrypted/durable-tasks)")

	systemHandler.RegisterRoutes(apiRouter)
	logger.Info("Campaign, attack-chain and probe APIs registered")
	logger.Info("Topic governance APIs registered")

	// 白名单管理 (PostgreSQL) — Web UI /api/v1/whitelist
	if whitelistRepo != nil {
		apiHandler.SetFeedbackWhitelistRepo(whitelistRepo)
		whitelistHandler := whitelist.NewHandler(whitelistRepo, logger)
		whitelistHandler.RegisterRoutes(apiRouter)
		if !readOnlyVerificationMode {
			go whitelistRepo.RunExpirySweeper(ctx, time.Duration(getIntEnv("WHITELIST_EXPIRY_SWEEP_SECONDS", 60))*time.Second, getIntEnv("WHITELIST_EXPIRY_SWEEP_BATCH", 100))
		}
		if cfg.Kafka.WhitelistEventProducerEnabled {
			readiness := whitelist.ProducerReadiness{
				Topic:           cfg.Kafka.WhitelistEventTopic,
				ConsumerGroup:   cfg.Kafka.WhitelistEventConsumerGroup,
				CandidateSHA256: cfg.Kafka.WhitelistConsumerCandidateSHA256,
				ContractSHA256:  cfg.Kafka.WhitelistEventContractSHA256,
			}
			verifyCtx, verifyCancel := context.WithTimeout(ctx, 10*time.Second)
			readinessErr := whitelist.VerifyProducerReadiness(verifyCtx, db, readiness)
			verifyCancel()
			if readinessErr != nil {
				logger.Fatal("Whitelist producer is not authorized by a consumer broker projection receipt", zap.Error(readinessErr))
			}
			whitelistProducer, producerErr := kafka.NewProducer(kafka.ProducerConfig{
				Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.WhitelistEventTopic,
				BatchSize: 100, RequiredAcks: "all", Compression: "lz4", Async: false,
				Security: cfg.Kafka.Security,
			}, logger)
			if producerErr != nil {
				logger.Fatal("Failed to create whitelist event producer", zap.Error(producerErr))
			}
			defer whitelistProducer.Close()
			dispatcher, dispatcherErr := whitelist.NewOutboxDispatcher(db, whitelistProducer, whitelist.OutboxDispatcherConfig{
				WorkerID: "alert-service-whitelist-" + uuid.NewString(), Logger: logger,
			})
			if dispatcherErr != nil {
				logger.Fatal("Failed to initialize whitelist outbox dispatcher", zap.Error(dispatcherErr))
			}
			verifyCtx, verifyCancel = context.WithTimeout(ctx, 10*time.Second)
			dispatcherErr = dispatcher.VerifySchema(verifyCtx)
			verifyCancel()
			if dispatcherErr != nil {
				logger.Fatal("Whitelist outbox schema is unavailable", zap.Error(dispatcherErr))
			}
			go dispatcher.Run(ctx)
			logger.Info("Whitelist event producer started after consumer readiness",
				zap.String("topic", cfg.Kafka.WhitelistEventTopic),
				zap.String("consumer_group", cfg.Kafka.WhitelistEventConsumerGroup),
				zap.String("consumer_candidate_sha256", cfg.Kafka.WhitelistConsumerCandidateSHA256))
		} else {
			logger.Warn("Whitelist event producer is disabled")
		}
		logger.Info("Whitelist governance initialized (IP/domain/asset/account/rule/model, approval and expiry lifecycle)")
	}

	// Advanced Alert Features (notification + risk + playbook + data quality)
	notifyCfg := notification.NotifyConfig{
		SMTPHost: getEnv("NOTIFY_SMTP_HOST", ""), SMTPPort: getIntEnv("NOTIFY_SMTP_PORT", 587),
		SMTPUser: getEnv("NOTIFY_SMTP_USER", ""), SMTPPassword: getEnv("NOTIFY_SMTP_PASSWORD", ""),
		FromEmail:    getEnv("NOTIFY_FROM_EMAIL", "alerts@traffic-analysis.local"),
		SlackWebhook: getEnv("NOTIFY_SLACK_WEBHOOK", ""), WebhookURL: getEnv("NOTIFY_WEBHOOK_URL", ""),
		WechatWebhook: getEnv("NOTIFY_WECHAT_WEBHOOK", ""), DingtalkWebhook: getEnv("NOTIFY_DINGTALK_WEBHOOK", ""),
		FeishuWebhook: getEnv("NOTIFY_FEISHU_WEBHOOK", ""),
		MinSeverity:   getEnv("NOTIFY_MIN_SEVERITY", "high"), RateLimitPerMin: getIntEnv("NOTIFY_RATE_LIMIT", 10),
		TemplateDir: getEnv("NOTIFY_TEMPLATE_DIR", "/etc/traffic/templates"),
	}
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
		advancedRepo.SetNotificationAlertStateResolver(func(resolveCtx context.Context, tenantID, alertID string) (*notification.AlertInfo, string, error) {
			detail, resolveErr := alertService.GetAlert(resolveCtx, tenantID, alertID)
			if resolveErr != nil {
				return nil, "", resolveErr
			}
			return &notification.AlertInfo{
				AlertID: detail.AlertID, TenantID: detail.TenantID, Fingerprint: detail.Fingerprint,
				Severity: detail.Severity, Score: float64(detail.Score), SourceIP: detail.SrcIP,
				DestIP: detail.DstIP, AlertType: detail.AlertType, CampaignID: detail.CampaignID,
				Timestamp: detail.LastSeen, Labels: append([]string(nil), detail.Labels...),
				Description: strings.Join(detail.Labels, " "), Count: detail.Count,
			}, detail.Status, nil
		})
		notifier.SetChannelResolver(advancedRepo.ResolveNotificationChannels)
		notifier.SetDeliveryRecorder(advancedRepo.RecordAutomaticNotificationDelivery)
		if !readOnlyVerificationMode {
			go advancedRepo.RunNotificationEscalationWorker(ctx, notifier, 2*time.Second)
		}
		if kafkaConsumer != nil {
			kafkaConsumer.SetNotificationDispatcher(notifier)
		}
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
			MaxKafkaLag:         cfg.DataQuality.MaxKafkaLag,
			MaxSignalAge:        cfg.DataQuality.MaxSignalAge,
		}, logger)
		dqMonitor.SetControlDB(db)
		if cfg.DataQuality.EvaluationEnabled && !readOnlyVerificationMode {
			if db == nil {
				logger.Fatal("Data quality rule evaluation requires PostgreSQL control plane")
			}
			ruleReader := dataquality.NewClickHouseRuleMeasurementReader(chSQLDB)
			go dataquality.RunRuleEvaluationLoop(ctx, dqMonitor, ruleReader,
				cfg.DataQuality.EvaluationInterval, cfg.DataQuality.EvaluationTimeout, logger)
			logger.Info("Data quality active-rule evaluation enabled",
				zap.Duration("interval", cfg.DataQuality.EvaluationInterval),
				zap.Duration("timeout", cfg.DataQuality.EvaluationTimeout))
		} else {
			logger.Info("Data quality active-rule evaluation disabled")
		}
		if cfg.DataQuality.CollectionEnabled && !readOnlyVerificationMode {
			contract := dataquality.DefaultFlowDatasetContract(
				cfg.DataQuality.KafkaTopic, cfg.DataQuality.KafkaGroupID,
				cfg.DataQuality.FlinkJobName, cfg.DataQuality.FlinkVertex,
			)
			kafkaDefinition, definitionErr := dataquality.SignalDefinitionFor(contract, dataquality.SignalKindKafkaOffset)
			if definitionErr != nil {
				logger.Fatal("Invalid Kafka data quality signal contract", zap.Error(definitionErr))
			}
			flinkDefinition, definitionErr := dataquality.SignalDefinitionFor(contract, dataquality.SignalKindFlinkWatermark)
			if definitionErr != nil {
				logger.Fatal("Invalid Flink data quality signal contract", zap.Error(definitionErr))
			}
			sinkDefinition, definitionErr := dataquality.SignalDefinitionFor(contract, dataquality.SignalKindSinkCommit)
			if definitionErr != nil {
				logger.Fatal("Invalid sink data quality signal contract", zap.Error(definitionErr))
			}
			businessDefinition, definitionErr := dataquality.SignalDefinitionFor(contract, dataquality.SignalKindBusinessVersion)
			if definitionErr != nil {
				logger.Fatal("Invalid business-version data quality signal contract", zap.Error(definitionErr))
			}
			objectDefinition, definitionErr := dataquality.SignalDefinitionFor(contract, dataquality.SignalKindObjectManifest)
			if definitionErr != nil {
				logger.Fatal("Invalid object-manifest data quality signal contract", zap.Error(definitionErr))
			}
			kafkaOffsetReader, readerErr := dataquality.NewKafkaBrokerOffsetReader(cfg.Kafka.Brokers, cfg.Kafka.Security, cfg.DataQuality.CollectionTimeout)
			if readerErr != nil {
				logger.Fatal("Failed to initialize Kafka data quality signal reader", zap.Error(readerErr))
			}
			defer kafkaOffsetReader.Close()
			flinkWatermarkReader, readerErr := dataquality.NewFlinkRESTWatermarkReader(cfg.DataQuality.FlinkRESTURL, cfg.DataQuality.CollectionTimeout)
			if readerErr != nil {
				logger.Fatal("Failed to initialize Flink data quality signal reader", zap.Error(readerErr))
			}
			collector, collectorErr := dataquality.NewCompositeSignalCollector(contract,
				dataquality.NewKafkaOffsetCollector(kafkaDefinition, cfg.DataQuality.KafkaTopic, cfg.DataQuality.KafkaGroupID, kafkaOffsetReader),
				dataquality.NewFlinkWatermarkCollector(flinkDefinition, cfg.DataQuality.FlinkJobName, cfg.DataQuality.FlinkVertex, cfg.DataQuality.FlinkMetric, flinkWatermarkReader),
				dataquality.NewSinkCommitCollector(sinkDefinition, dataquality.NewClickHouseSinkCommitReader(chSQLDB)),
				dataquality.NewNotApplicableSignalCollector(businessDefinition),
				dataquality.NewNotApplicableSignalCollector(objectDefinition),
			)
			if collectorErr != nil {
				logger.Fatal("Failed to initialize data quality signal contract", zap.Error(collectorErr))
			}
			go dataquality.RunHandoffSignalCollectionLoop(ctx, dqMonitor, collector, cfg.DataQuality.CollectionInterval, logger)
			logger.Info("Data quality hand-off signal collection enabled",
				zap.String("contract_version", contract.ContractVersion),
				zap.String("dataset_id", contract.DatasetID),
				zap.Duration("interval", cfg.DataQuality.CollectionInterval))
		} else {
			logger.Info("Data quality hand-off signal collection disabled")
		}
	}
	advHandler := api.NewAdvancedHandler(notifier, riskScorer, playbookEngine, dqMonitor, advancedRepo)
	advHandler.SetDataQualityRepairExecutionFeatureFlag(cfg.DataQuality.RepairExecutionEnabled && !readOnlyVerificationMode)
	repairEvidenceRegistered := db != nil && chSQLDB != nil
	if repairEvidenceRegistered {
		advHandler.SetDataQualityRepairEvidenceProvider(dataquality.NewClickHouseRepairEvidenceProvider(db, chSQLDB, cfg.DataQuality.RepairEvidenceTimeout))
	}
	var repairProjectionConsumer *dataquality.FlowReplayProjectionConsumer
	var repairTopicReader *dataquality.KafkaBrokerOffsetReader
	if cfg.DataQuality.RepairProjectionEnabled && !readOnlyVerificationMode {
		if db == nil {
			logger.Fatal("Data quality repair projection requires PostgreSQL")
		}
		projection, projectionErr := dataquality.NewPostgresFlowReplayProjection(db)
		if projectionErr != nil {
			logger.Fatal("Failed to initialize data quality repair projection", zap.Error(projectionErr))
		}
		verifyCtx, verifyCancel := context.WithTimeout(ctx, 10*time.Second)
		projectionErr = projection.Ready(verifyCtx)
		verifyCancel()
		if projectionErr != nil {
			logger.Fatal("Data quality repair projection schema is unavailable", zap.Error(projectionErr))
		}
		repairKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.DataQuality.RepairProjectionTopic,
			GroupID: cfg.DataQuality.RepairProjectionGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create data quality repair projection consumer", zap.Error(consumerErr))
		}
		repairProjectionConsumer, consumerErr = dataquality.NewFlowReplayProjectionConsumer(
			repairKafkaConsumer, projection, cfg.DataQuality.RepairProjectionTopic, logger,
		)
		if consumerErr != nil {
			_ = repairKafkaConsumer.Close()
			logger.Fatal("Failed to initialize data quality repair projection consumer", zap.Error(consumerErr))
		}
		defer repairProjectionConsumer.Close()
		go func() {
			logger.Info("Starting data quality repair projection consumer",
				zap.String("topic", cfg.DataQuality.RepairProjectionTopic), zap.String("group_id", cfg.DataQuality.RepairProjectionGroup))
			if startErr := repairProjectionConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Data quality repair projection consumer stopped", zap.Error(startErr))
			}
		}()
		var readerErr error
		repairTopicReader, readerErr = dataquality.NewKafkaBrokerOffsetReader(cfg.Kafka.Brokers, cfg.Kafka.Security, cfg.DataQuality.RepairEvidenceTimeout)
		if readerErr != nil {
			logger.Fatal("Failed to initialize data quality repair topic readiness", zap.Error(readerErr))
		}
		defer repairTopicReader.Close()
	} else {
		logger.Info("Data quality repair projection consumer disabled")
	}

	executorRegistered := false
	if cfg.DataQuality.RepairExecutionEnabled && !readOnlyVerificationMode {
		if !cfg.DataQuality.RepairProjectionEnabled || repairProjectionConsumer == nil || repairTopicReader == nil || db == nil || chSQLDB == nil || dqMonitor == nil {
			logger.Fatal("Data quality repair execution requires enabled PostgreSQL projection, Kafka readiness, ClickHouse and monitor")
		}
		repairProducer, producerErr := kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.DataQuality.RepairProjectionTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, MaxAttempts: 5, Security: cfg.Kafka.Security,
		}, logger)
		if producerErr != nil {
			logger.Fatal("Failed to create data quality repair replay producer", zap.Error(producerErr))
		}
		defer repairProducer.Close()
		publisher := dataquality.NewKafkaFlowReplayPublisher(repairProducer, cfg.DataQuality.RepairProjectionTopic,
			func(readyCtx context.Context, topic string) error {
				if err := repairProjectionConsumer.Ready(readyCtx); err != nil {
					return err
				}
				_, err := repairTopicReader.ReadLag(readyCtx, topic, cfg.DataQuality.RepairProjectionGroup)
				return err
			})
		driver := dataquality.NewClickHouseFlowReplayDriver(chSQLDB, publisher, 500)
		worker := dataquality.NewRepairExecutionWorker(db, dqMonitor, driver, cfg.DataQuality.RepairWorkerInterval, logger)
		readyCtx, readyCancel := context.WithTimeout(ctx, cfg.DataQuality.RepairEvidenceTimeout)
		readyErr := worker.Ready(readyCtx)
		readyCancel()
		if readyErr != nil {
			logger.Fatal("Data quality repair executor is not ready", zap.Error(readyErr))
		}
		advHandler.SetDataQualityRepairExecutor(worker)
		go worker.Run(ctx)
		executorRegistered = true
		logger.Info("Data quality repair executor started", zap.String("topic", cfg.DataQuality.RepairProjectionTopic), zap.Duration("interval", cfg.DataQuality.RepairWorkerInterval))
	}
	logger.Info("Data quality repair execution gate configured",
		zap.Bool("flag_enabled", cfg.DataQuality.RepairExecutionEnabled),
		zap.Bool("projection_enabled", cfg.DataQuality.RepairProjectionEnabled),
		zap.Bool("evidence_provider_registered", repairEvidenceRegistered), zap.Bool("executor_registered", executorRegistered))
	var notificationGovernanceProducer *kafka.Producer
	if advancedRepo != nil && !readOnlyVerificationMode {
		notificationGovernanceProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.NotificationGovernanceEventTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Warn("Failed to create notification governance producer; committed outbox events will remain pending", zap.Error(err))
			notificationGovernanceProducer = nil
		} else {
			defer notificationGovernanceProducer.Close()
			advHandler.SetNotificationGovernanceEventProducer(notificationGovernanceProducer)
			if workerErr := advHandler.StartNotificationGovernanceOutboxWorker(ctx, 2*time.Second); workerErr != nil {
				logger.Warn("Failed to start notification governance outbox worker", zap.Error(workerErr))
			} else {
				logger.Info("Notification governance outbox worker started", zap.String("topic", cfg.Kafka.NotificationGovernanceEventTopic))
			}
		}
	}
	playbookExecutionV2Enabled := getBoolEnv("PLAYBOOK_EXECUTION_V2_ENABLED", false) && !readOnlyVerificationMode
	advHandler.SetPlaybookExecutionV2FeatureFlag(playbookExecutionV2Enabled)
	if playbookExecutionV2Enabled {
		if providerURL := strings.TrimSpace(getEnv("PLAYBOOK_EXECUTION_PROVIDER_URL", "")); providerURL != "" {
			provider, providerErr := api.NewHTTPPlaybookExecutionProvider(
				providerURL,
				getEnv("PLAYBOOK_EXECUTION_PROVIDER_TOKEN", ""),
				time.Duration(getIntEnv("PLAYBOOK_EXECUTION_PROVIDER_TIMEOUT_SECONDS", 30))*time.Second,
			)
			if providerErr != nil {
				logger.Fatal("Invalid playbook execution provider configuration", zap.Error(providerErr))
			}
			advHandler.SetPlaybookExecutionProvider(provider)
			if err := advHandler.StartPlaybookExecutionWorker(ctx, 2*time.Second); err != nil {
				logger.Fatal("Failed to start playbook execution worker", zap.Error(err))
			}
			logger.Info("Playbook execution v2 provider adapter configured")
		} else {
			logger.Warn("Playbook execution v2 is enabled without a provider; approved executions will remain awaiting executor")
		}
	}
	if cfg.Kafka.PlaybookEventEnabled && !readOnlyVerificationMode {
		if advancedRepo == nil {
			logger.Fatal("Playbook execution event pipeline requires PostgreSQL")
		}
		playbookEventProducer, producerErr := kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.PlaybookEventTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: cfg.Kafka.Security,
		}, logger)
		if producerErr != nil {
			logger.Fatal("Failed to create playbook execution event producer", zap.Error(producerErr))
		}
		defer playbookEventProducer.Close()
		advHandler.SetPlaybookExecutionEventProducer(playbookEventProducer, cfg.Kafka.PlaybookEventTopic)
		if workerErr := advHandler.StartPlaybookExecutionEventOutboxWorker(ctx, 2*time.Second); workerErr != nil {
			logger.Fatal("Failed to start playbook execution event outbox worker", zap.Error(workerErr))
		}
		playbookKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.PlaybookEventTopic,
			GroupID: cfg.Kafka.PlaybookEventGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, DLQPermanentOnly: true,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create playbook execution projection consumer", zap.Error(consumerErr))
		}
		playbookEventConsumer, consumerErr := consumer.NewPlaybookExecutionEventConsumer(
			playbookKafkaConsumer, advHandler, cfg.Kafka.PlaybookEventTopic, logger)
		if consumerErr != nil {
			_ = playbookKafkaConsumer.Close()
			logger.Fatal("Failed to initialize playbook execution projection consumer", zap.Error(consumerErr))
		}
		defer playbookEventConsumer.Close()
		go func() {
			logger.Info("Starting playbook execution projection consumer", zap.String("topic", cfg.Kafka.PlaybookEventTopic), zap.String("group_id", cfg.Kafka.PlaybookEventGroup))
			if startErr := playbookEventConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Playbook execution projection consumer stopped", zap.Error(startErr))
			}
		}()
		logger.Info("Playbook execution event V2 pipeline started", zap.String("topic", cfg.Kafka.PlaybookEventTopic))
	} else {
		logger.Warn("Playbook execution event V2 pipeline is disabled")
	}
	advHandler.RegisterAPIRoutes(apiRouter)
	logger.Info("Advanced alert features enabled (notification + risk + playbook + data quality)")
	// ==================== HTTP Server ====================
	var rootHandler http.Handler = r
	if readOnlyVerificationMode {
		rootHandler = readOnlyVerificationHandler(rootHandler)
	}
	srv := &http.Server{
		Addr:         cfg.API.ListenAddr,
		Handler:      rootHandler,
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

func newAlertSystemHandler(
	chClient *storage.ClickHouseClient,
	pgDB *sql.DB,
	logger *zap.Logger,
	encryptedTrafficStats api.EncryptedTrafficStatsService,
) *api.SystemHandler {
	handler := api.NewSystemHandler(chClient, pgDB, logger)
	handler.SetEncryptedTrafficStatsService(encryptedTrafficStats)
	return handler
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

func getBoolEnv(key string, defaultValue bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
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

// alertContractPrincipal 将 alert-service 自有认证层(判定合一 P1:HMAC→OIDC
// 统一 ValidateToken 入口,httpx 上下文键)适配为契约解释器的判定主体。
func alertContractPrincipal(r *http.Request) *authz.Principal {
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

// alertContractDenyAudit 契约拒绝留痕(审计三联:拒绝→审计行,Kafka 审计事件)。
func alertContractDenyAudit(auditLogger *commonAudit.Logger) authz.ContractDenyAuditor {
	return func(r *http.Request, op *authz.Operation, principal *authz.Principal, status int) {
		if auditLogger == nil {
			return
		}
		detail := map[string]interface{}{
			"operation_id":   op.OperationID,
			"required_scope": op.RequiredScope,
			"path":           r.URL.Path,
			"result":         "denied",
			"status":         status,
		}
		userID := ""
		if principal != nil {
			userID = principal.Subject
		}
		auditLogger.Log(r.Context(), &commonAudit.AuditEvent{
			EventType:    commonAudit.EventTypePermissionDenied,
			TenantID:     httpx.GetTenantID(r.Context()),
			UserID:       userID,
			ServiceName:  "alert-service",
			SourceIP:     httpx.GetClientIP(r),
			Action:       "CONTRACT_ACCESS_DENIED",
			ResourceType: "contract_operation",
			ResourceID:   op.OperationID,
			Detail:       detail,
			Result:       commonAudit.ResultFailure,
		})
	}
}

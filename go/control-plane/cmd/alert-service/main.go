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
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/audit"
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
	if cfg.Kafka.WhitelistEventPipelineEnabled {
		if whitelistRepo == nil || readOnlyVerificationMode {
			logger.Fatal("Whitelist event pipeline requires writable PostgreSQL mode")
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

	// ==================== 初始化 Kafka Producer (for Feedback) ====================
	var feedbackProducer *kafka.Producer
	var responseActionProducer *kafka.Producer
	var savedViewEventProducer *kafka.Producer
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

	// ==================== 初始化 API Handler ====================
	// Feedback HTTP always uses the PostgreSQL transaction/outbox path. A
	// temporarily unavailable producer leaves committed outbox rows pending.
	apiHandler := api.NewHandlerWithFeedback(alertService, feedbackProducer, alertAuditLogger, logger)
	apiHandler.SetActionAuditWriter(api.NewAlertActionAuditWriter(db, logger))
	feedbackTransactionalOutboxEnabled := getBoolEnv("ALERT_FEEDBACK_TRANSACTIONAL_OUTBOX_V1_ENABLED", true) && !readOnlyVerificationMode
	apiHandler.SetFeedbackTransactionalOutboxEnabled(feedbackTransactionalOutboxEnabled)
	apiHandler.SetResponseActionProducer(responseActionProducer)
	apiHandler.SetSavedViewEventProducer(savedViewEventProducer)
	alertReportFeatureEnabled := getBoolEnv("ALERT_REPORT_JOBS_V1_ENABLED", true) && !readOnlyVerificationMode
	campaignLinkFeatureEnabled := getBoolEnv("CAMPAIGN_ALERT_LINKS_V1_ENABLED", true)
	campaignAggregateV2Enabled := getBoolEnv("CAMPAIGN_AGGREGATE_V2_ENABLED", false)
	apiHandler.SetAlignmentFeatureFlags(alertReportFeatureEnabled, campaignLinkFeatureEnabled)
	apiHandler.SetCampaignAggregateV2FeatureFlag(campaignAggregateV2Enabled)
	if chSQLDB != nil {
		apiHandler.SetCampaignLookup(api.NewClickHouseAlertCampaignLookup(chSQLDB))
	}
	if responseActionProducer != nil {
		if err := apiHandler.StartResponseActionOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start response-action outbox worker", zap.Error(err))
		} else {
			logger.Info("Response-action outbox worker started")
		}
	}
	if savedViewEventProducer != nil {
		if err := apiHandler.StartSavedViewOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start saved-view outbox worker", zap.Error(err))
		} else {
			logger.Info("Saved-view outbox worker started", zap.String("topic", cfg.Kafka.SavedViewEventTopic))
		}
	}
	if cfg.Kafka.ResponseActionEnabled && !readOnlyVerificationMode {
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
		responseKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.ResponseActionTopic,
			GroupID: cfg.Kafka.ResponseActionGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create alert response execution consumer", zap.Error(consumerErr))
		}
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
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, Security: cfg.Kafka.Security,
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
		if cfg.Kafka.WhitelistEventPipelineEnabled {
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
	topicSnapshotFeatureEnabled := getBoolEnv("TOPIC_SNAPSHOT_V1_ENABLED", true)
	topicExecutorFeatureEnabled := getBoolEnv("TOPIC_EXECUTOR_V2_ENABLED", true) && !readOnlyVerificationMode
	probeOperationFeatureEnabled := getBoolEnv("PROBE_OPERATION_ACK_V2_ENABLED", true) && !readOnlyVerificationMode
	auditBatchFeatureEnabled := getBoolEnv("AUDIT_BATCH_FAIL_CLOSED_V1_ENABLED", false) && !readOnlyVerificationMode
	systemHandler.SetCampaignAggregateV2FeatureFlag(campaignAggregateV2Enabled)
	systemHandler.SetTopicAlignmentFeatureFlags(topicSnapshotFeatureEnabled, topicExecutorFeatureEnabled)
	systemHandler.SetProbeOperationAckFeatureFlag(probeOperationFeatureEnabled)
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
	if cfg.Kafka.CampaignEventEnabled && !readOnlyVerificationMode {
		if db == nil {
			logger.Fatal("Campaign event pipeline requires PostgreSQL")
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
		if err := systemHandler.StartCampaignEventOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Fatal("Failed to start campaign event outbox worker", zap.Error(err))
		}
		campaignAggregateKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignEventTopic,
			GroupID: cfg.Kafka.CampaignEventGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create campaign aggregate projection consumer", zap.Error(consumerErr))
		}
		campaignAggregateConsumer, consumerErr := consumer.NewCampaignEventConsumer(
			campaignAggregateKafkaConsumer, systemHandler, "aggregate", cfg.Kafka.CampaignEventTopic, logger)
		if consumerErr != nil {
			_ = campaignAggregateKafkaConsumer.Close()
			logger.Fatal("Failed to initialize campaign aggregate projection consumer", zap.Error(consumerErr))
		}
		defer campaignAggregateConsumer.Close()
		go func() {
			logger.Info("Starting campaign aggregate projection consumer", zap.String("topic", cfg.Kafka.CampaignEventTopic), zap.String("group_id", cfg.Kafka.CampaignEventGroup))
			if startErr := campaignAggregateConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Campaign aggregate projection consumer stopped", zap.Error(startErr))
			}
		}()
		campaignMembershipKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.CampaignMemberTopic,
			GroupID: cfg.Kafka.CampaignMemberGroup, MinBytes: 1, MaxWait: 500 * time.Millisecond,
			MaxRetries: 3, RetryBackoff: time.Second, EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Fatal("Failed to create campaign membership projection consumer", zap.Error(consumerErr))
		}
		campaignMembershipConsumer, consumerErr := consumer.NewCampaignEventConsumer(
			campaignMembershipKafkaConsumer, systemHandler, "membership", cfg.Kafka.CampaignMemberTopic, logger)
		if consumerErr != nil {
			_ = campaignMembershipKafkaConsumer.Close()
			logger.Fatal("Failed to initialize campaign membership projection consumer", zap.Error(consumerErr))
		}
		defer campaignMembershipConsumer.Close()
		go func() {
			logger.Info("Starting campaign membership projection consumer", zap.String("topic", cfg.Kafka.CampaignMemberTopic), zap.String("group_id", cfg.Kafka.CampaignMemberGroup))
			if startErr := campaignMembershipConsumer.Start(ctx); startErr != nil && startErr != context.Canceled {
				logger.Error("Campaign membership projection consumer stopped", zap.Error(startErr))
			}
		}()
		logger.Info("Campaign event V2 pipeline started", zap.String("aggregate_topic", cfg.Kafka.CampaignEventTopic), zap.String("membership_topic", cfg.Kafka.CampaignMemberTopic))
	} else {
		logger.Warn("Campaign event V2 pipeline is disabled")
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
		if !cfg.Kafka.CampaignEventEnabled {
			logger.Fatal("Campaign target projection requires the campaign event V2 pipeline")
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
			CommitOnDLQSuccess: true, CommitOnHandlerError: false,
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
	var probeOperationProducer *kafka.Producer
	var probeOperationEventProducer *kafka.Producer
	if probeOperationFeatureEnabled {
		probeOperationProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: "probe.control.v2", BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Warn("Failed to create probe-operation Kafka producer; durable outbox will remain pending", zap.Error(err))
			probeOperationProducer = nil
		} else {
			defer probeOperationProducer.Close()
			systemHandler.SetProbeOperationProducer(probeOperationProducer)
		}
		probeOperationEventProducer, err = kafka.NewProducer(kafka.ProducerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: "probe.events.v2", BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
		}, logger)
		if err != nil {
			logger.Warn("Failed to create probe-operation event producer; durable ACK outbox will remain pending", zap.Error(err))
			probeOperationEventProducer = nil
		} else {
			defer probeOperationEventProducer.Close()
			systemHandler.SetProbeOperationEventProducer(probeOperationEventProducer)
		}
	}
	if db != nil && probeOperationFeatureEnabled && probeOperationProducer != nil && probeOperationEventProducer != nil {
		if err := systemHandler.StartProbeOperationOutboxWorker(ctx, 2*time.Second); err != nil {
			logger.Warn("Failed to start probe-operation outbox worker", zap.Error(err))
		} else {
			logger.Info("Probe-operation outbox worker started", zap.String("topic", probeOperationProducer.Topic()))
		}
	}
	if db != nil && probeOperationFeatureEnabled {
		probeEventKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.ProbeEventTopic,
			GroupID: cfg.Kafka.ProbeEventGroup, MaxRetries: 3, RetryBackoff: time.Second,
			EnableDLQ: true, DLQTopic: "dlq.v1",
			CommitOnDLQSuccess: true, CommitOnHandlerError: false,
			Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Warn("Failed to create probe event projection consumer", zap.Error(consumerErr))
		} else {
			probeEventConsumer, initErr := consumer.NewProbeOperationEventConsumer(
				probeEventKafkaConsumer, systemHandler, logger,
			)
			if initErr != nil {
				_ = probeEventKafkaConsumer.Close()
				logger.Warn("Failed to initialize probe event projection consumer", zap.Error(initErr))
			} else {
				defer probeEventConsumer.Close()
				go func() {
					logger.Info(
						"Starting probe event projection consumer",
						zap.String("topic", cfg.Kafka.ProbeEventTopic),
						zap.String("group_id", cfg.Kafka.ProbeEventGroup),
						zap.String("dlq_topic", "dlq.v1"),
					)
					if startErr := probeEventConsumer.Start(ctx); startErr != nil &&
						startErr != context.Canceled {
						logger.Error("Probe event projection consumer stopped", zap.Error(startErr))
					}
				}()
			}
		}
	}
	if db != nil && probeOperationFeatureEnabled {
		probeAckKafkaConsumer, consumerErr := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.ProbeAckTopic,
			GroupID: cfg.Kafka.ProbeAckGroup, MaxRetries: 3, RetryBackoff: time.Second,
			EnableDLQ: true, DLQTopicPrefix: "dlq.", CommitOnDLQSuccess: true,
			CommitOnHandlerError: false, Security: cfg.Kafka.Security,
		}, logger)
		if consumerErr != nil {
			logger.Warn("Failed to create probe ACK Kafka consumer", zap.Error(consumerErr))
		} else {
			probeAckConsumer, consumerErr := consumer.NewProbeAckConsumer(
				probeAckKafkaConsumer, systemHandler, logger,
			)
			if consumerErr != nil {
				_ = probeAckKafkaConsumer.Close()
				logger.Warn("Failed to initialize probe ACK consumer", zap.Error(consumerErr))
			} else {
				defer probeAckConsumer.Close()
				go func() {
					logger.Info(
						"Starting probe ACK consumer",
						zap.String("topic", cfg.Kafka.ProbeAckTopic),
						zap.String("group_id", cfg.Kafka.ProbeAckGroup),
					)
					if startErr := probeAckConsumer.Start(ctx); startErr != nil &&
						startErr != context.Canceled {
						logger.Error("Probe ACK consumer stopped", zap.Error(startErr))
					}
				}()
			}
		}
	}

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
	internalRouter := r.PathPrefix("/internal/v1").Subrouter()
	applyAPIMiddlewares(internalRouter)
	systemHandler.RegisterInternalRoutes(internalRouter)

	// 注册 API 路由
	apiHandler.RegisterRoutes(apiRouter)

	// Dashboard API — 实时统计 (Web UI 大屏)
	dashboardHandler := api.NewDashboardHandler(chClient, logger)
	dashboardSnapshotHandler := api.NewDashboardSnapshotHandler(chClient, db, osRepo, rdb, logger, getBoolEnv("DASHBOARD_SNAPSHOT_V1_ENABLED", false))
	dashboardSnapshotHandler.RegisterRoutes(apiRouter)
	apiRouter.HandleFunc("/dashboard/stats", dashboardHandler.GetStats).Methods("GET")
	apiRouter.HandleFunc("/dashboard/alerts/trend", dashboardHandler.GetAlertTrend).Methods("GET")
	apiRouter.HandleFunc("/dashboard/attack-phases", dashboardHandler.GetAttackPhases).Methods("GET")
	apiRouter.HandleFunc("/dashboard/top-ips/{type}", dashboardHandler.GetTopIPs).Methods("GET")
	apiRouter.HandleFunc("/dashboard/encrypted/trend", dashboardHandler.GetEncryptedTrend).Methods("GET")
	dashboardTaskHandler := api.NewDashboardTaskHandler(db, logger, getBoolEnv("DASHBOARD_TASK_V2_ENABLED", true))
	dashboardTaskHandler.RegisterRoutes(apiRouter)
	if getBoolEnv("DASHBOARD_TASK_PIPELINE_V1_ENABLED", false) && !readOnlyVerificationMode {
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
		if cfg.Kafka.WhitelistEventPipelineEnabled {
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
			verifyCtx, verifyCancel := context.WithTimeout(ctx, 10*time.Second)
			dispatcherErr = dispatcher.VerifySchema(verifyCtx)
			verifyCancel()
			if dispatcherErr != nil {
				logger.Fatal("Whitelist outbox schema is unavailable", zap.Error(dispatcherErr))
			}
			go dispatcher.Run(ctx)
			logger.Info("Whitelist event pipeline started", zap.String("topic", cfg.Kafka.WhitelistEventTopic))
		} else {
			logger.Warn("Whitelist event pipeline is disabled")
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
			CommitOnDLQSuccess: true, CommitOnHandlerError: false, Security: cfg.Kafka.Security,
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

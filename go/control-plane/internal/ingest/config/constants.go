package config

import "time"

const (
	// Kafka Topics — 对齐 common/kafka/create-topics.sh
	TopicFlowEvents    = "flow.events.v1"    // Probe → Ingest → Flink
	TopicSessionEvents = "session.events.v1" // Flink Session Job 产出
	TopicPcapIndex     = "pcap.index.v1"     // Probe PCAP 元数据
	TopicFeatureStat   = "feature.stat.v1"   // Flink Feature Job 产出
	TopicDetections    = "detections.v1"     // Flink Detection Job 产出
	TopicAlerts        = "alerts.v1"         // Flink Alert Job 产出
	TopicRuleUpdates   = "rule.updates"      // Rule Manager → Flink
	TopicAuditLogs     = "audit.logs"        // 审计日志
	TopicAssetBindings = "asset.bindings.v1" // MAC→IP 绑定
	TopicDeviceLogs    = "device.logs.v1"    // 设备 Syslog
	TopicUserEvents    = "user.events.v1"    // 用户行为
	TopicDLQ           = "dlq.v1"            // 死信队列
)

const (
	// Redis Key Prefixes — 对齐 common/redis/README.md
	PrefixDedup   = "dedup:"   // 事件去重
	PrefixQuota   = "quota:"   // 配额限流
	PrefixSession = "session:" // 用户会话
	PrefixProbe   = "probe:"   // 探针状态/配置
	PrefixAlert   = "alert:"   // 告警去重/状态
	PrefixAsset   = "asset:"   // 资产缓存
	PrefixStats   = "stats:"   // Dashboard 统计
	PrefixLock    = "lock:"    // 分布式锁
	PrefixConfig  = "config:"  // 配置缓存

	// Backward compatibility aliases
	RedisTokenPrefix        = PrefixSession
	RedisRateLimitPrefix    = PrefixQuota
	RedisDedupPrefix        = PrefixDedup
	RedisProbeConfigPrefix  = PrefixProbe
	RedisProbeHistoryPrefix = PrefixProbe
	RedisProbeStatusPrefix  = PrefixProbe
)

const (
	ScopeIngestWrite = "ingest:write"
	ScopeIngestRead  = "ingest:read"
	ScopePcapWrite   = "pcap:write"
	ScopePcapRead    = "pcap:read"
	ScopeDLQReplay   = "dlq:replay"
	ScopeAdminWrite  = "admin:write"
	ScopeAdminAll    = "admin:*"
	ScopeAdminRead   = "admin:read"
	ScopeWildcard    = "*"
)

const (
	ContentTypeProtobuf      = "application/x-protobuf"
	ContentTypeJSON          = "application/json"
	ProtoPackage             = "traffic.v1"
	ProtoSchemaVersion       = "v1"
	ProtoMessageFlowEvent    = "traffic.v1.FlowEvent"
	ProtoMessageSessionEvent = "traffic.v1.SessionEvent"
	ProtoMessagePcapIndex    = "traffic.v1.PcapIndexMeta"
)

var PublicMethodPrefixes = []string{
	"grpc.health.v1.Health/",
	"/grpc.health.v1.Health/",
	"/grpc.reflection.v1alpha.ServerReflection/",
	"/grpc.reflection.v1.ServerReflection/",
}

var PublicHTTPPaths = []string{
	"/health",
	"/healthz",
	"/ready",
	"/readyz",
	"/live",
	"/livez",
	"/metrics",
	"/version",
	"/api/v1/ping",
}

const (
	DefaultFeatureSetID        = "v1"
	DefaultKafkaBatchSize      = 1000
	DefaultKafkaCompression    = "lz4"
	DefaultMaxBatchSize        = 10000
	DefaultMaxEventSize        = 65536
	DefaultStreamBufferSize    = 1000
	DefaultHeartbeatInterval   = 30 * time.Second
	DefaultProbeStatusTimeout  = 5 * time.Minute
	DefaultTokenTTL            = 5 * time.Minute
	DefaultLocalCacheTTL       = 30 * time.Second
	DefaultLocalCacheSize      = 10000
	DefaultDedupLocalCacheSize = 100000
	DefaultDedupLocalTTL       = 5 * time.Minute
	DefaultDedupRedisTTL       = 10 * time.Minute
	DefaultGlobalRPS           = 100000.0
	DefaultGlobalBurst         = 200000
	DefaultTenantRPS           = 10000.0
	DefaultTenantBurst         = 20000
	DefaultProbeRPS            = 5000.0
	DefaultProbeBurst          = 10000
)

const (
	RedisDialTimeout        = 5 * time.Second
	RedisReadTimeout        = 3 * time.Second
	RedisWriteTimeout       = 3 * time.Second
	RedisPoolTimeout        = 4 * time.Second
	RedisConnMaxIdleTime    = 30 * time.Minute
	PostgresConnLifetime    = 1 * time.Hour
	KafkaBatchTimeout       = 100 * time.Millisecond
	HealthCheckTimeout      = 2 * time.Second
	GracefulShutdownTimeout = 25 * time.Second
	KafkaFlushTimeout       = 5 * time.Second
	HTTPRequestTimeout      = 30 * time.Second
	GRPCRequestTimeout      = 30 * time.Second
)

const (
	MaxRedisTTL         = 86400
	MaxFallbackFileSize = 100 * 1024 * 1024
	MinKeepaliveTime    = 5 * time.Second
	TokenScaleFactor    = 1000000
	MaxUInt8            = 255
	MaxUInt16           = 65535
	MaxUInt32           = 4294967295
	MaxRecvMsgSize      = 64 * 1024 * 1024
	MaxSendMsgSize      = 64 * 1024 * 1024
)

const (
	StatusCodeOK                 = 200
	StatusCodeBadRequest         = 400
	StatusCodeUnauthorized       = 401
	StatusCodeForbidden          = 403
	StatusCodeNotFound           = 404
	StatusCodeTooManyRequests    = 429
	StatusCodeInternalError      = 500
	StatusCodeServiceUnavailable = 503
)

const (
	EnvLogLevel            = "LOG_LEVEL"
	EnvEnvironment         = "ENVIRONMENT"
	EnvDefaultFeatureSetID = "DEFAULT_FEATURE_SET_ID"
	EnvHealthAddr          = "HEALTH_ADDR"
	EnvDLQFallbackDir      = "DLQ_FALLBACK_DIR"
	EnvPostgresDSN         = "POSTGRES_DSN"
	EnvJWTSigningKey       = "JWT_SIGNING_KEY"
	EnvServiceName         = "SERVICE_NAME"
	EnvServiceVersion      = "SERVICE_VERSION"
)

const (
	DefaultDLQFallbackDir = "/var/log/ingest-gateway/dlq-fallback"
	DefaultHealthAddr     = ":8081"
	DefaultMetricsAddr    = ":9090"
	DefaultGRPCAddr       = ":50051"
	DefaultHTTPAddr       = ":8080"
	DefaultConfigPath     = "./config.env"
	DefaultTLSCertPath    = "./certs/server/server-cert.pem"
	DefaultTLSKeyPath     = "./certs/server/server-key.pem"
	DefaultTLSCAPath      = "./certs/ca/ca-cert.pem"
)

const (
	DLQFallbackFileFormat   = "dlq-fallback-%d-%d.log"
	AuditBackupFileFormat   = "audit-%s-%s.jsonl"
	ProbeConfigFileFormat   = "probe-config-%s.json"
	TokenExportFileFormat   = "tokens-%s.json"
	MetricsExportFileFormat = "metrics-%s.json"
	LogRotateFileFormat     = "app-%s.log"
	BackupArchiveFileFormat = "backup-%s.tar.gz"
	PCAPIndexFileFormat     = "pcap-index-%s.json"
	SessionIndexFileFormat  = "session-index-%s.json"
)

const (
	DefaultRedisPoolSize     = 100
	DefaultRedisMinIdleConns = 10
	RedisMaxRetries          = 3
	RedisRetryInterval       = 100 * time.Millisecond
)

const (
	DefaultKafkaMaxRetries  = 3
	KafkaRetryBackoff       = 100 * time.Millisecond
	KafkaRequiredAcksAll    = "all"
	KafkaRequiredAcksOne    = "one"
	KafkaRequiredAcksNone   = "none"
	KafkaCompressionLZ4     = "lz4"
	KafkaCompressionGzip    = "gzip"
	KafkaCompressionSnappy  = "snappy"
	KafkaCompressionZstd    = "zstd"
	KafkaCompressionNone    = "none"
	KafkaBalancerHash       = "hash"
	KafkaBalancerRoundRobin = "roundrobin"
	KafkaBalancerLeastBytes = "leastbytes"
)

const (
	DefaultDLQReplayInterval       = 5 * time.Minute
	DefaultDLQReplayBatchSize      = 1000
	DefaultDLQBatchSize            = 100
	DefaultDLQRetryBackoff         = 100 * time.Millisecond
	DefaultDLQReplaySuccessRate    = 0.5
	DefaultDLQMaxRetries           = 3
	DefaultDLQReplayIdempotencyTTL = 24 * time.Hour
	DLQMessageFormatVersion        = "v1"
)

const (
	DefaultAuditBufferSize    = 1000
	DefaultAuditBatchSize     = 100
	DefaultAuditFlushInterval = 1 * time.Second
	DefaultAuditRetention     = 90 * 24 * time.Hour
	AuditEventFormatVersion   = "v1"
)

const (
	LogLevelDebug   = "debug"
	LogLevelInfo    = "info"
	LogLevelWarn    = "warn"
	LogLevelError   = "error"
	LogLevelFatal   = "fatal"
	LogFormatJSON   = "json"
	LogFormatText   = "text"
	LogOutputStdout = "stdout"
	LogOutputStderr = "stderr"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentStaging     = "staging"
	EnvironmentProduction  = "production"
	EnvironmentTest        = "test"
)

const (
	ErrCodeUnknown            = "UNKNOWN"
	ErrCodeInvalidArgument    = "INVALID_ARGUMENT"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeAlreadyExists      = "ALREADY_EXISTS"
	ErrCodePermissionDenied   = "PERMISSION_DENIED"
	ErrCodeUnauthenticated    = "UNAUTHENTICATED"
	ErrCodeResourceExhausted  = "RESOURCE_EXHAUSTED"
	ErrCodeFailedPrecondition = "FAILED_PRECONDITION"
	ErrCodeAborted            = "ABORTED"
	ErrCodeOutOfRange         = "OUT_OF_RANGE"
	ErrCodeUnimplemented      = "UNIMPLEMENTED"
	ErrCodeInternal           = "INTERNAL"
	ErrCodeUnavailable        = "UNAVAILABLE"
	ErrCodeDataLoss           = "DATA_LOSS"
	ErrCodeDeadlineExceeded   = "DEADLINE_EXCEEDED"
)

const (
	AuditEventTypeLogin            = "login"
	AuditEventTypeLogout           = "logout"
	AuditEventTypeTokenCreate      = "token_create"
	AuditEventTypeTokenRevoke      = "token_revoke"
	AuditEventTypeConfigChange     = "config_change"
	AuditEventTypeProbeRegister    = "probe_register"
	AuditEventTypeProbeUnregister  = "probe_unregister"
	AuditEventTypeDataExport       = "data_export"
	AuditEventTypeDataDelete       = "data_delete"
	AuditEventTypePermissionGrant  = "permission_grant"
	AuditEventTypePermissionRevoke = "permission_revoke"
	AuditEventTypeRateLimitChange  = "rate_limit_change"
	AuditEventTypeAuthFailure      = "auth_failure"
	AuditEventTypeAccessDenied     = "access_denied"
)

const (
	ProbeStatusHealthy      = "healthy"
	ProbeStatusWarning      = "warning"
	ProbeStatusCritical     = "critical"
	ProbeStatusOffline      = "offline"
	ProbeStatusUnknown      = "unknown"
	ProbeStatusRegistering  = "registering"
	ProbeStatusShuttingDown = "shutting_down"
)

const (
	MetricLabelTenantID  = "tenant_id"
	MetricLabelProbeID   = "probe_id"
	MetricLabelEventType = "event_type"
	MetricLabelStatus    = "status"
	MetricLabelMethod    = "method"
	MetricLabelTopic     = "topic"
	MetricLabelErrorType = "error_type"
	MetricLabelReason    = "reason"
	MetricLabelOperation = "operation"
)

const (
	TokenTypeAPI       = "api"
	TokenTypeSession   = "session"
	TokenTypeRefresh   = "refresh"
	TokenTypeService   = "service"
	TokenTypeTemporary = "temporary"
)

const (
	DefaultBatchProcessInterval = 100 * time.Millisecond
	DefaultBatchMaxSize         = 1000
	DefaultBatchMaxWaitTime     = 5 * time.Second
	DefaultWorkerPoolSize       = 10
	DefaultQueueSize            = 10000
)

const (
	DefaultMaxRetryAttempts    = 3
	DefaultInitialRetryBackoff = 100 * time.Millisecond
	DefaultMaxRetryBackoff     = 10 * time.Second
	DefaultRetryMultiplier     = 2.0
	DefaultRetryJitter         = 0.1
)

const (
	DefaultCPULimit       = 0.8
	DefaultMemoryLimit    = 0.9
	DefaultGoroutineLimit = 10000
	DefaultChannelBuffer  = 1000
	DefaultMapInitSize    = 100
	DefaultSliceCapacity  = 100
)

const (
	DefaultMetricsRetention      = 7 * 24 * time.Hour
	DefaultLogsRetention         = 30 * 24 * time.Hour
	DefaultAuditLogsRetention    = 90 * 24 * time.Hour
	DefaultProbeStatusRetention  = 24 * time.Hour
	DefaultTokenHistoryRetention = 365 * 24 * time.Hour
)

const (
	ServiceNameIngestGateway = "ingest-gateway"
	ServiceNameAuthService   = "auth-service"
	ServiceNameQueryService  = "query-service"
	ServiceNameAlertService  = "alert-service"
	ServiceRegistryConsul    = "consul"
	ServiceRegistryEtcd      = "etcd"
	ServiceRegistryK8s       = "kubernetes"
)

const (
	DefaultHashCost        = 12
	DefaultTokenLength     = 32
	DefaultSecretKeyLength = 32
	DefaultSaltLength      = 16
	DefaultNonceLength     = 12
)

const (
	APIVersionV1     = "v1"
	ConfigVersion    = "1.0.0"
	ProtoVersion     = "v1"
	SchemaVersion    = "1.0.0"
	MinClientVersion = "1.0.0"
	MaxClientVersion = "2.0.0"
)

func IsPublicMethod(fullMethod string) bool {
	for _, prefix := range PublicMethodPrefixes {
		if len(fullMethod) >= len(prefix) && fullMethod[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func IsPublicHTTPPath(path string) bool {
	for _, publicPath := range PublicHTTPPaths {
		if path == publicPath {
			return true
		}
	}
	return false
}

func GetDefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"kafka_batch_size":          DefaultKafkaBatchSize,
		"kafka_compression":         DefaultKafkaCompression,
		"max_batch_size":            DefaultMaxBatchSize,
		"max_event_size":            DefaultMaxEventSize,
		"stream_buffer_size":        DefaultStreamBufferSize,
		"heartbeat_interval":        DefaultHeartbeatInterval,
		"probe_status_timeout":      DefaultProbeStatusTimeout,
		"token_ttl":                 DefaultTokenTTL,
		"local_cache_ttl":           DefaultLocalCacheTTL,
		"local_cache_size":          DefaultLocalCacheSize,
		"dedup_local_cache_size":    DefaultDedupLocalCacheSize,
		"dedup_local_ttl":           DefaultDedupLocalTTL,
		"dedup_redis_ttl":           DefaultDedupRedisTTL,
		"global_rps":                DefaultGlobalRPS,
		"global_burst":              DefaultGlobalBurst,
		"tenant_rps":                DefaultTenantRPS,
		"tenant_burst":              DefaultTenantBurst,
		"probe_rps":                 DefaultProbeRPS,
		"probe_burst":               DefaultProbeBurst,
		"dlq_replay_interval":       DefaultDLQReplayInterval,
		"dlq_replay_batch_size":     DefaultDLQReplayBatchSize,
		"dlq_replay_success_rate":   DefaultDLQReplaySuccessRate,
		"audit_buffer_size":         DefaultAuditBufferSize,
		"audit_batch_size":          DefaultAuditBatchSize,
		"audit_flush_interval":      DefaultAuditFlushInterval,
		"feature_set_id":            DefaultFeatureSetID,
		"health_check_timeout":      HealthCheckTimeout,
		"graceful_shutdown_timeout": GracefulShutdownTimeout,
		"kafka_flush_timeout":       KafkaFlushTimeout,
	}
}

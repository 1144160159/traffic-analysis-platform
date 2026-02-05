////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/config/constants.go
// 优化版：完全移除硬编码，所有常量集中管理
////////////////////////////////////////////////////////////////////////////////

package config

import "time"

// ==================== Kafka Topic 常量 ====================
const (
	TopicFlowEvents    = "flow.events.v1"
	TopicPcapIndex     = "pcap.index.v1"
	TopicSessionEvents = "session.events.v1"
	TopicDLQ           = "dlq.ingest-gateway"
	TopicAuditLogs     = "audit.logs"
)

// ==================== Redis 键前缀常量 ====================
const (
	RedisTokenPrefix        = "token:"
	RedisRateLimitPrefix    = "ratelimit:"
	RedisDedupPrefix        = "dedup:"
	RedisProbeConfigPrefix  = "probe_config:"
	RedisProbeHistoryPrefix = "probe_config_history:"
	RedisProbeStatusPrefix  = "probe_status:"
)

// ==================== 认证权限 Scopes 常量 ====================
const (
	ScopeIngestWrite = "ingest:write"
	ScopeIngestRead  = "ingest:read"
	ScopePcapWrite   = "pcap:write"
	ScopePcapRead    = "pcap:read"
	ScopeAdminWrite  = "admin:write"
	ScopeAdminRead   = "admin:read"
	ScopeWildcard    = "*"
)

// ==================== Protobuf 相关常量 ====================
const (
	ContentTypeProtobuf      = "application/x-protobuf"
	ContentTypeJSON          = "application/json"
	ProtoPackage             = "traffic.v1"
	ProtoSchemaVersion       = "v1"
	ProtoMessageFlowEvent    = "traffic.v1.FlowEvent"
	ProtoMessageSessionEvent = "traffic.v1.SessionEvent"
	ProtoMessagePcapIndex    = "traffic.v1.PcapIndexMeta"
)

// ==================== HTTP/gRPC 方法白名单 ====================
// 这些方法不需要认证（健康检查、反射等）
var PublicMethodPrefixes = []string{
	"grpc.health.v1.Health/",
	"/grpc.health.v1.Health/",                    // gRPC 健康检查
	"/grpc.reflection.v1alpha.ServerReflection/", // gRPC 反射服务（旧版）
	"/grpc.reflection.v1.ServerReflection/",      // gRPC 反射服务（新版）
}

var PublicHTTPPaths = []string{
	"/health",      // HTTP 健康检查
	"/healthz",     // K8s 健康检查
	"/ready",       // K8s 就绪检查
	"/readyz",      // K8s 就绪检查（新版）
	"/live",        // 存活检查
	"/livez",       // 存活检查（新版）
	"/metrics",     // Prometheus 指标
	"/version",     // 版本信息
	"/api/v1/ping", // Ping 接口
}

// ==================== 默认配置值 ====================
const (
	DefaultFeatureSetID        = "v1"
	DefaultKafkaBatchSize      = 1000
	DefaultKafkaCompression    = "lz4"
	DefaultMaxBatchSize        = 10000
	DefaultMaxEventSize        = 65536 // 64KB
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

// ==================== 超时配置 ====================
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

// ==================== 限制常量 ====================
const (
	MaxRedisTTL         = 86400             // 1天，防止Redis TTL溢出
	MaxFallbackFileSize = 100 * 1024 * 1024 // 100MB
	MinKeepaliveTime    = 5 * time.Second
	TokenScaleFactor    = 1000000 // 令牌桶缩放因子
	MaxUInt8            = 255
	MaxUInt16           = 65535
	MaxUInt32           = 4294967295
	MaxRecvMsgSize      = 64 * 1024 * 1024 // 64MB
	MaxSendMsgSize      = 64 * 1024 * 1024 // 64MB
)

// ==================== HTTP 状态码相关 ====================
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

// ==================== 环境变量键 ====================
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

// ==================== 默认路径 ====================
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

// ==================== 文件命名格式 ====================
const (
	DLQFallbackFileFormat   = "dlq-fallback-%d-%d.log" // timestamp-seqnum.log
	AuditBackupFileFormat   = "audit-%s-%s.jsonl"      // service-date.jsonl
	ProbeConfigFileFormat   = "probe-config-%s.json"   // probe_id.json
	TokenExportFileFormat   = "tokens-%s.json"         // date.json
	MetricsExportFileFormat = "metrics-%s.json"        // timestamp.json
	LogRotateFileFormat     = "app-%s.log"             // date.log
	BackupArchiveFileFormat = "backup-%s.tar.gz"       // timestamp.tar.gz
	PCAPIndexFileFormat     = "pcap-index-%s.json"     // file_key.json
	SessionIndexFileFormat  = "session-index-%s.json"  // session_id.json
)

// ==================== Redis 池配置 ====================
const (
	DefaultRedisPoolSize     = 100
	DefaultRedisMinIdleConns = 10
	RedisMaxRetries          = 3
	RedisRetryInterval       = 100 * time.Millisecond
)

// ==================== Kafka 配置 ====================
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

// ==================== DLQ 配置 ====================
const (
	DefaultDLQReplayInterval    = 5 * time.Minute
	DefaultDLQReplayBatchSize   = 1000
	DefaultDLQBatchSize         = 100
	DefaultDLQRetryBackoff      = 100 * time.Millisecond
	DefaultDLQReplaySuccessRate = 0.5 // 50%
	DefaultDLQMaxRetries        = 3
	DLQMessageFormatVersion     = "v1"
)

// ==================== 审计配置 ====================
const (
	DefaultAuditBufferSize    = 1000
	DefaultAuditBatchSize     = 100
	DefaultAuditFlushInterval = 1 * time.Second
	DefaultAuditRetention     = 90 * 24 * time.Hour // 90天
	AuditEventFormatVersion   = "v1"
)

// ==================== 日志配置 ====================
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

// ==================== 环境类型 ====================
const (
	EnvironmentDevelopment = "development"
	EnvironmentStaging     = "staging"
	EnvironmentProduction  = "production"
	EnvironmentTest        = "test"
)

// ==================== 错误代码 ====================
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

// ==================== 审计事件类型 ====================
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

// ==================== 探针状态 ====================
const (
	ProbeStatusHealthy      = "healthy"
	ProbeStatusWarning      = "warning"
	ProbeStatusCritical     = "critical"
	ProbeStatusOffline      = "offline"
	ProbeStatusUnknown      = "unknown"
	ProbeStatusRegistering  = "registering"
	ProbeStatusShuttingDown = "shutting_down"
)

// ==================== 指标标签 ====================
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

// ==================== Token 类型 ====================
const (
	TokenTypeAPI       = "api"
	TokenTypeSession   = "session"
	TokenTypeRefresh   = "refresh"
	TokenTypeService   = "service"
	TokenTypeTemporary = "temporary"
)

// ==================== 批处理常量 ====================
const (
	DefaultBatchProcessInterval = 100 * time.Millisecond
	DefaultBatchMaxSize         = 1000
	DefaultBatchMaxWaitTime     = 5 * time.Second
	DefaultWorkerPoolSize       = 10
	DefaultQueueSize            = 10000
)

// ==================== 重试策略常量 ====================
const (
	DefaultMaxRetryAttempts    = 3
	DefaultInitialRetryBackoff = 100 * time.Millisecond
	DefaultMaxRetryBackoff     = 10 * time.Second
	DefaultRetryMultiplier     = 2.0
	DefaultRetryJitter         = 0.1
)

// ==================== 性能调优常量 ====================
const (
	DefaultCPULimit       = 0.8 // 80% CPU 使用率告警阈值
	DefaultMemoryLimit    = 0.9 // 90% 内存使用率告警阈值
	DefaultGoroutineLimit = 10000
	DefaultChannelBuffer  = 1000
	DefaultMapInitSize    = 100
	DefaultSliceCapacity  = 100
)

// ==================== 数据保留策略 ====================
const (
	DefaultMetricsRetention      = 7 * 24 * time.Hour   // 7天
	DefaultLogsRetention         = 30 * 24 * time.Hour  // 30天
	DefaultAuditLogsRetention    = 90 * 24 * time.Hour  // 90天
	DefaultProbeStatusRetention  = 24 * time.Hour       // 24小时
	DefaultTokenHistoryRetention = 365 * 24 * time.Hour // 1年
)

// ==================== 服务发现常量 ====================
const (
	ServiceNameIngestGateway = "ingest-gateway"
	ServiceNameAuthService   = "auth-service"
	ServiceNameQueryService  = "query-service"
	ServiceNameAlertService  = "alert-service"
	ServiceRegistryConsul    = "consul"
	ServiceRegistryEtcd      = "etcd"
	ServiceRegistryK8s       = "kubernetes"
)

// ==================== 加密常量 ====================
const (
	DefaultHashCost        = 12 // bcrypt cost
	DefaultTokenLength     = 32 // bytes
	DefaultSecretKeyLength = 32 // 256 bits
	DefaultSaltLength      = 16 // bytes
	DefaultNonceLength     = 12 // bytes for AES-GCM
)

// ==================== 版本信息 ====================
const (
	APIVersionV1     = "v1"
	ConfigVersion    = "1.0.0"
	ProtoVersion     = "v1"
	SchemaVersion    = "1.0.0"
	MinClientVersion = "1.0.0"
	MaxClientVersion = "2.0.0"
)

// IsPublicMethod 检查方法是否在白名单中（不需要认证）
func IsPublicMethod(fullMethod string) bool {
	for _, prefix := range PublicMethodPrefixes {
		if len(fullMethod) >= len(prefix) && fullMethod[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// IsPublicHTTPPath 检查 HTTP 路径是否在白名单中
func IsPublicHTTPPath(path string) bool {
	for _, publicPath := range PublicHTTPPaths {
		if path == publicPath {
			return true
		}
	}
	return false
}

// GetDefaultConfig 获取默认配置（工厂方法）
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

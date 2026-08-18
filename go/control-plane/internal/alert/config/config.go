////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/config/config.go
// 修复版：添加默认值，增加 ClickHouse 主机解析
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v10"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

// Config Alert Service 总配置
type Config struct {
	Kafka              KafkaConfig
	Redis              RedisConfig
	ClickHouse         ClickHouseConfig
	OpenSearch         OpenSearchConfig
	AlertProjection    AlertProjectionConfig
	CampaignProjection CampaignProjectionConfig
	DataQuality        DataQualitySignalConfig
	Dedup              DedupConfig
	API                APIConfig
	Auth               AuthConfig
	ProbeOperation     ProbeOperationPipelineConfig
}

// DataQualitySignalConfig controls the read-only cross-system collectors. The
// deployment switch is explicit so the application cannot write watermark
// rows before the versioned PostgreSQL migration has been applied.
type DataQualitySignalConfig struct {
	CollectionEnabled       bool          `env:"DATA_QUALITY_SIGNAL_COLLECTION_ENABLED" envDefault:"false"`
	CollectionInterval      time.Duration `env:"DATA_QUALITY_SIGNAL_COLLECTION_INTERVAL" envDefault:"1m"`
	CollectionTimeout       time.Duration `env:"DATA_QUALITY_SIGNAL_COLLECTION_TIMEOUT" envDefault:"10s"`
	EvaluationEnabled       bool          `env:"DATA_QUALITY_RULE_EVALUATION_ENABLED" envDefault:"false"`
	EvaluationInterval      time.Duration `env:"DATA_QUALITY_RULE_EVALUATION_INTERVAL" envDefault:"5m"`
	EvaluationTimeout       time.Duration `env:"DATA_QUALITY_RULE_EVALUATION_TIMEOUT" envDefault:"30s"`
	RepairExecutionEnabled  bool          `env:"DATA_QUALITY_REPAIR_EXECUTION_ENABLED" envDefault:"false"`
	RepairProjectionEnabled bool          `env:"DATA_QUALITY_REPAIR_PROJECTION_ENABLED" envDefault:"false"`
	RepairEvidenceTimeout   time.Duration `env:"DATA_QUALITY_REPAIR_EVIDENCE_TIMEOUT" envDefault:"15s"`
	RepairProjectionTopic   string        `env:"DATA_QUALITY_REPAIR_PROJECTION_TOPIC" envDefault:"flow.projection-replay.v1"`
	RepairProjectionGroup   string        `env:"DATA_QUALITY_REPAIR_PROJECTION_GROUP" envDefault:"alert-service-flow-replay-projection-v1"`
	RepairWorkerInterval    time.Duration `env:"DATA_QUALITY_REPAIR_WORKER_INTERVAL" envDefault:"5s"`
	MaxSignalAge            time.Duration `env:"DQ_MAX_SIGNAL_AGE" envDefault:"5m"`
	MaxKafkaLag             int64         `env:"DQ_MAX_KAFKA_LAG" envDefault:"10000"`
	KafkaTopic              string        `env:"DATA_QUALITY_KAFKA_TOPIC" envDefault:"flow.events.v1"`
	KafkaGroupID            string        `env:"DATA_QUALITY_KAFKA_GROUP_ID" envDefault:"flink-session-job"`
	FlinkRESTURL            string        `env:"DATA_QUALITY_FLINK_REST_URL" envDefault:"http://flink-jobmanager.flink.svc:8081"`
	FlinkJobName            string        `env:"DATA_QUALITY_FLINK_JOB_NAME" envDefault:"Session Aggregation Job V2"`
	FlinkVertex             string        `env:"DATA_QUALITY_FLINK_VERTEX_CONTAINS" envDefault:"Assign FlowEvent Watermarks"`
	FlinkMetric             string        `env:"DATA_QUALITY_FLINK_METRIC_CONTAINS" envDefault:"currentOutputWatermark"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers                            []string `env:"KAFKA_BROKERS" envSeparator:"," envDefault:"kafka-bootstrap.middleware.svc:9092"`
	Topic                              string   `env:"KAFKA_TOPIC" envDefault:"detections.v1"`
	GroupID                            string   `env:"KAFKA_GROUP_ID" envDefault:"alert-service"`
	BatchSize                          int      `env:"KAFKA_BATCH_SIZE" envDefault:"100"`
	ProbeAckTopic                      string   `env:"KAFKA_PROBE_ACK_TOPIC" envDefault:"probe.acks.v2"`
	ProbeAckGroup                      string   `env:"KAFKA_PROBE_ACK_GROUP" envDefault:"alert-service-probe-acks-v2"`
	TopicActionEventTopic              string   `env:"KAFKA_TOPIC_ACTION_EVENT_TOPIC" envDefault:"traffic.topic.action.v2"`
	TopicActionEventGroup              string   `env:"KAFKA_TOPIC_ACTION_EVENT_GROUP" envDefault:"alert-service-topic-action-projection-v2"`
	ProbeEventTopic                    string   `env:"KAFKA_PROBE_EVENT_TOPIC" envDefault:"probe.events.v2"`
	ProbeEventGroup                    string   `env:"KAFKA_PROBE_EVENT_GROUP" envDefault:"alert-service-probe-event-projection-v2"`
	ProbeGroupReadinessTopic           string   `env:"KAFKA_PROBE_GROUP_READINESS_TOPIC" envDefault:"probe.group-readiness.v1"`
	ProbeGroupReadinessGroup           string   `env:"KAFKA_PROBE_GROUP_READINESS_GROUP" envDefault:"alert-service-probe-group-readiness-v1"`
	ResponseActionTopic                string   `env:"KAFKA_RESPONSE_ACTION_TOPIC" envDefault:"alert.response.requested.v1"`
	ResponseActionGroup                string   `env:"KAFKA_RESPONSE_ACTION_GROUP" envDefault:"alert-service-response-execution-v1"`
	ResponseActionEnabled              bool     `env:"ALERT_RESPONSE_EXECUTION_V1_ENABLED" envDefault:"false"`
	SavedViewEventTopic                string   `env:"KAFKA_SAVED_VIEW_EVENT_TOPIC" envDefault:"alert.saved-view.events.v1"`
	SavedViewTransactionEnabled        bool     `env:"ALERT_SAVED_VIEW_TRANSACTION_V2_ENABLED" envDefault:"false"`
	AlertEvidenceLinkTopic             string   `env:"KAFKA_ALERT_EVIDENCE_LINK_TOPIC" envDefault:"alert.evidence-links.v1"`
	AlertEvidenceLinkGroup             string   `env:"KAFKA_ALERT_EVIDENCE_LINK_GROUP" envDefault:"alert-service-evidence-link-projection-v1"`
	AlertEvidenceLinkConsumerEnabled   bool     `env:"ALERT_EVIDENCE_LINK_CONSUMER_V1_ENABLED" envDefault:"false"`
	AlertEvidenceLinkDispatcherEnabled bool     `env:"ALERT_EVIDENCE_LINK_DISPATCHER_V1_ENABLED" envDefault:"false"`
	WhitelistEventTopic                string   `env:"KAFKA_WHITELIST_EVENT_TOPIC" envDefault:"whitelist.events.v2"`
	WhitelistEventConsumerGroup        string   `env:"KAFKA_WHITELIST_EVENT_GROUP" envDefault:"rule-manager-whitelist-rule-effect-v2"`
	WhitelistEventProducerEnabled      bool     `env:"WHITELIST_EVENT_PRODUCER_V2_ENABLED" envDefault:"false"`
	WhitelistDetectionMatcherEnabled   bool     `env:"WHITELIST_DETECTION_MATCHER_V2_ENABLED" envDefault:"false"`
	WhitelistConsumerCandidateSHA256   string   `env:"WHITELIST_CONSUMER_CANDIDATE_SHA256" envDefault:"0000000000000000000000000000000000000000000000000000000000000000"`
	WhitelistEventContractSHA256       string   `env:"WHITELIST_EVENT_CONTRACT_SHA256" envDefault:"d87787272d140c8529686ce45eef30f2a6345fb7f2e918a450399c8f698aad49"`
	// Deprecated combined switch retained so old manifests continue to parse.
	// Runtime admission deliberately ignores it: producer, consumer and matcher
	// are separate rollout rails.
	WhitelistEventPipelineEnabled    bool   `env:"WHITELIST_EVENT_PIPELINE_V2_ENABLED" envDefault:"false"`
	NotificationGovernanceEventTopic string `env:"KAFKA_NOTIFICATION_GOVERNANCE_EVENT_TOPIC" envDefault:"notification.governance.events.v1"`
	CampaignEventTopic               string `env:"KAFKA_CAMPAIGN_EVENT_TOPIC" envDefault:"campaign.events.v2"`
	CampaignEventGroup               string `env:"KAFKA_CAMPAIGN_EVENT_GROUP" envDefault:"alert-service-campaign-event-projection-v2"`
	CampaignMemberTopic              string `env:"KAFKA_CAMPAIGN_MEMBERSHIP_TOPIC" envDefault:"campaign.membership.events.v2"`
	CampaignMemberGroup              string `env:"KAFKA_CAMPAIGN_MEMBERSHIP_GROUP" envDefault:"alert-service-campaign-membership-projection-v2"`
	CampaignProtoTopic               string `env:"KAFKA_CAMPAIGNS_PROTO_TOPIC" envDefault:"campaigns.v1"`
	CampaignProtoGroup               string `env:"KAFKA_CAMPAIGNS_PROTO_GROUP" envDefault:"alert-service-campaigns-proto-projection-v1"`
	CampaignProtoEnabled             bool   `env:"CAMPAIGNS_PROTO_CONSUMER_V1_ENABLED" envDefault:"false"`
	CampaignEventConsumerEnabled     bool   `env:"CAMPAIGN_EVENT_CONSUMER_V2_ENABLED" envDefault:"false"`
	CampaignEventDispatcherEnabled   bool   `env:"CAMPAIGN_EVENT_DISPATCHER_V2_ENABLED" envDefault:"false"`
	CampaignRailCorrelationEnabled   bool   `env:"CAMPAIGN_RAIL_CORRELATION_V1_ENABLED" envDefault:"false"`
	// Deprecated combined switch retained only so old manifests still parse.
	// Runtime admission uses the two explicit switches above.
	CampaignEventEnabled         bool   `env:"CAMPAIGN_EVENT_PIPELINE_V2_ENABLED" envDefault:"false"`
	PlaybookEventTopic           string `env:"KAFKA_PLAYBOOK_EXECUTION_EVENT_TOPIC" envDefault:"playbook.execution.events.v2"`
	PlaybookEventGroup           string `env:"KAFKA_PLAYBOOK_EXECUTION_EVENT_GROUP" envDefault:"alert-service-playbook-execution-projection-v2"`
	PlaybookEventEnabled         bool   `env:"PLAYBOOK_EXECUTION_EVENT_PIPELINE_V2_ENABLED" envDefault:"false"`
	FusionCommandTopic           string `env:"KAFKA_FUSION_COMMAND_TOPIC" envDefault:"fusion.commands.v1"`
	FusionProjectionGroup        string `env:"KAFKA_FUSION_PROJECTION_GROUP" envDefault:"alert-service-fusion-projection-v1"`
	FusionProjectionEnabled      bool   `env:"FUSION_PROJECTION_CONSUMER_V1_ENABLED" envDefault:"false"`
	BaselineLifecycleTopic       string `env:"KAFKA_BASELINE_LIFECYCLE_TOPIC" envDefault:"baseline.lifecycle.v1"`
	BaselineActivationAckTopic   string `env:"KAFKA_BASELINE_ACTIVATION_ACK_TOPIC" envDefault:"baseline.activation-acks.v1"`
	BaselineActivationAckGroup   string `env:"KAFKA_BASELINE_ACTIVATION_ACK_GROUP" envDefault:"alert-service-baseline-activation-acks-v1"`
	BaselineActivationAckEnabled bool   `env:"BEHAVIOR_BASELINE_ACK_CONSUMER_V1_ENABLED" envDefault:"false"`
	Security                     kafkaCommon.SecurityConfig
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addrs          []string      `env:"REDIS_ADDRS" envSeparator:"," envDefault:"redis-master.databases.svc:6379"`
	Password       string        `env:"REDIS_PASSWORD"`
	DB             int           `env:"REDIS_DB" envDefault:"0"`
	SentinelAddrs  []string      `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster string        `env:"REDIS_SENTINEL_MASTER"`
	PoolSize       int           `env:"REDIS_POOL_SIZE" envDefault:"20"`
	TTL            time.Duration `env:"REDIS_TTL" envDefault:"24h"`
}

// ClickHouseConfig ClickHouse 配置
type ClickHouseConfig struct {
	// DSN 格式: clickhouse://user:password@host:port/database
	DSN          string   `env:"CLICKHOUSE_DSN" envDefault:"clickhouse://default:@clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000/traffic"`
	Hosts        []string `env:"CLICKHOUSE_HOSTS" envSeparator:","`
	Database     string   `env:"CLICKHOUSE_DATABASE" envDefault:"traffic"`
	Username     string   `env:"CLICKHOUSE_USERNAME" envDefault:"default"`
	Password     string   `env:"CLICKHOUSE_PASSWORD"`
	MaxOpenConns int      `env:"CLICKHOUSE_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns int      `env:"CLICKHOUSE_MAX_IDLE_CONNS" envDefault:"5"`
}

// GetHosts 从 DSN 解析出主机列表
func (c *ClickHouseConfig) GetHosts() []string {
	if hosts := normalizeClickHouseHosts(c.Hosts); len(hosts) > 0 {
		return hosts
	}

	if c.DSN == "" {
		return []string{"clickhouse-1.middleware.svc:9000"}
	}

	// 解析 DSN: clickhouse://user:password@host:port/database
	dsn := c.DSN

	// 移除 clickhouse:// 前缀
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	// 尝试解析为 URL
	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		// 如果解析失败，假设就是 host:port 格式
		if strings.Contains(dsn, "@") {
			parts := strings.SplitN(dsn, "@", 2)
			if len(parts) == 2 {
				hostPart := parts[1]
				if idx := strings.Index(hostPart, "/"); idx > 0 {
					return normalizeClickHouseHosts([]string{hostPart[:idx]})
				}
				return normalizeClickHouseHosts([]string{hostPart})
			}
		}
		return normalizeClickHouseHosts([]string{dsn})
	}

	host := u.Host
	if host == "" {
		host = "clickhouse-1.middleware.svc:9000"
	}

	return normalizeClickHouseHosts([]string{host})
}

func normalizeClickHouseHosts(values []string) []string {
	hosts := make([]string, 0, len(values))
	for _, value := range values {
		for _, rawHost := range strings.Split(value, ",") {
			host := strings.TrimSpace(rawHost)
			if host == "" {
				continue
			}
			if !strings.Contains(host, ":") {
				host += ":9000"
			}
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// GetDatabase 从 DSN 解析出数据库名
func (c *ClickHouseConfig) GetDatabase() string {
	if c.Database != "" {
		return c.Database
	}

	if c.DSN == "" {
		return "traffic"
	}

	dsn := c.DSN
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		return "traffic"
	}

	path := strings.TrimPrefix(u.Path, "/")
	if path == "" {
		return "traffic"
	}

	return path
}

// GetUsername 从 DSN 解析出用户名
func (c *ClickHouseConfig) GetUsername() string {
	if c.Username != "" {
		return c.Username
	}

	if c.DSN == "" {
		return "default"
	}

	dsn := c.DSN
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		return "default"
	}

	if u.User != nil {
		return u.User.Username()
	}

	return "default"
}

// GetPassword 从 DSN 解析出密码
func (c *ClickHouseConfig) GetPassword() string {
	if c.Password != "" {
		return c.Password
	}

	if c.DSN == "" {
		return ""
	}

	dsn := c.DSN
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		return ""
	}

	if u.User != nil {
		password, _ := u.User.Password()
		return password
	}

	return ""
}

// OpenSearchConfig OpenSearch 配置
type OpenSearchConfig struct {
	Addresses           []string      `env:"OPENSEARCH_ADDRS" envSeparator:"," envDefault:"http://opensearch.middleware.svc:9200"`
	Username            string        `env:"OPENSEARCH_USERNAME" envDefault:"admin"`
	Password            string        `env:"OPENSEARCH_PASSWORD" envDefault:""` // 生产环境必须通过环境变量注入
	Index               string        `env:"OPENSEARCH_INDEX" envDefault:"traffic-alerts"`
	LegacyReadTarget    string        `env:"OPENSEARCH_LEGACY_READ_TARGET" envDefault:""`
	V2Enabled           bool          `env:"OPENSEARCH_ALERTS_V2_ENABLED" envDefault:"false"`
	ReadAlias           string        `env:"OPENSEARCH_ALERTS_READ_ALIAS" envDefault:"alerts-v2-read"`
	WriteAlias          string        `env:"OPENSEARCH_ALERTS_WRITE_ALIAS" envDefault:"alerts-v2-write"`
	SearchCursorEnabled bool          `env:"OPENSEARCH_SEARCH_CURSOR_V1_ENABLED" envDefault:"false"`
	SearchShallowLimit  int           `env:"OPENSEARCH_SEARCH_SHALLOW_RESULT_LIMIT" envDefault:"1000"`
	SearchMaxPageSize   int           `env:"OPENSEARCH_SEARCH_MAX_PAGE_SIZE" envDefault:"200"`
	SearchQueryTimeout  time.Duration `env:"OPENSEARCH_SEARCH_QUERY_TIMEOUT" envDefault:"2s"`
	SearchCursorTTL     time.Duration `env:"OPENSEARCH_SEARCH_CURSOR_TTL" envDefault:"2m"`
	SearchTrackTotal    int           `env:"OPENSEARCH_SEARCH_TRACK_TOTAL_HITS_UP_TO" envDefault:"10000"`
}

// AlertProjectionConfig controls repair of durable ClickHouse-to-OpenSearch
// projection debt. Recording debt is independent of this switch; automatic
// repair remains fail-closed and disabled until a canary is approved.
type AlertProjectionConfig struct {
	ReconcileEnabled bool          `env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED" envDefault:"false"`
	Interval         time.Duration `env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_INTERVAL" envDefault:"1s"`
	Lease            time.Duration `env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_LEASE" envDefault:"45s"`
	BatchSize        int           `env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_BATCH_SIZE" envDefault:"100"`
	MaxAttempts      int           `env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_MAX_ATTEMPTS" envDefault:"8"`
	MaxDocuments     int           `env:"OPENSEARCH_ALERT_PROJECTION_REBUILD_MAX_DOCUMENTS" envDefault:"10000"`
	StopErrorCount   int           `env:"OPENSEARCH_ALERT_PROJECTION_STOP_ERROR_COUNT" envDefault:"25"`
	RepairPerSecond  int           `env:"OPENSEARCH_ALERT_PROJECTION_REPAIR_PER_SECOND" envDefault:"100"`
}

// ReadTarget returns the exact v2 alias only after the migration flag is
// approved. The legacy wildcard contract remains available until cutover.
func (c OpenSearchConfig) ReadTarget() string {
	if c.V2Enabled {
		return c.ReadAlias
	}
	if target := strings.TrimSpace(c.LegacyReadTarget); target != "" {
		return target
	}
	return c.Index + "-*"
}

// WriteTarget returns the v2 write alias only after the migration flag is
// approved. Legacy writes retain their date-partitioned physical index names.
func (c OpenSearchConfig) WriteTarget() string {
	if c.V2Enabled {
		return c.WriteAlias
	}
	return c.Index
}

// CampaignProjectionConfig controls the independently acknowledged
// ClickHouse/OpenSearch/NebulaGraph projection worker. It is deliberately
// disabled by default until every target schema and credential passes startup
// readiness.
type CampaignProjectionConfig struct {
	Enabled              bool          `env:"CAMPAIGN_TARGET_PROJECTION_V2_ENABLED" envDefault:"false"`
	Interval             time.Duration `env:"CAMPAIGN_TARGET_PROJECTION_INTERVAL" envDefault:"500ms"`
	Lease                time.Duration `env:"CAMPAIGN_TARGET_PROJECTION_LEASE" envDefault:"45s"`
	MaxAttempts          int           `env:"CAMPAIGN_TARGET_PROJECTION_MAX_ATTEMPTS" envDefault:"8"`
	ClickHouseTable      string        `env:"CAMPAIGN_TARGET_PROJECTION_CLICKHOUSE_TABLE" envDefault:"traffic.campaign_projection_events_v2"`
	OpenSearchWriteAlias string        `env:"CAMPAIGN_TARGET_PROJECTION_OS_WRITE_ALIAS" envDefault:"campaign-projections-v2-write"`
	Nebula               CampaignProjectionNebulaConfig
}

type CampaignProjectionNebulaConfig struct {
	Addresses   []string      `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_ADDRESSES" envSeparator:"," envDefault:"nebula-graph.middleware.svc:9669"`
	Username    string        `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_USERNAME" envDefault:"root"`
	Password    string        `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_PASSWORD"`
	Space       string        `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_SPACE" envDefault:"traffic_graph"`
	Timeout     time.Duration `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_TIMEOUT" envDefault:"10s"`
	IdleTime    time.Duration `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_IDLE_TIME" envDefault:"30m"`
	MaxPoolSize int           `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_MAX_POOL_SIZE" envDefault:"20"`
	MinPoolSize int           `env:"CAMPAIGN_TARGET_PROJECTION_NEBULA_MIN_POOL_SIZE" envDefault:"2"`
}

// DedupConfig 去重配置
type DedupConfig struct {
	TimeBucketMinutes int           `env:"DEDUP_TIME_BUCKET" envDefault:"10"`
	TTL               time.Duration `env:"DEDUP_TTL" envDefault:"10m"`
}

// APIConfig API 配置
type APIConfig struct {
	ListenAddr     string        `env:"API_LISTEN_ADDR" envDefault:":8081"`
	ReadTimeout    time.Duration `env:"API_READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout   time.Duration `env:"API_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout    time.Duration `env:"API_IDLE_TIMEOUT" envDefault:"60s"`
	AllowedOrigins []string      `env:"API_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
}

// AuthConfig Auth 配置
type AuthConfig struct {
	Enabled                bool   `env:"AUTH_ENABLED" envDefault:"true"`
	PostgresDSN            string `env:"AUTH_POSTGRES_DSN"`
	PostgresHost           string `env:"AUTH_POSTGRES_HOST" envDefault:"postgres-primary.databases.svc"`
	PostgresPort           int    `env:"AUTH_POSTGRES_PORT" envDefault:"5432"`
	PostgresDatabase       string `env:"AUTH_POSTGRES_DATABASE" envDefault:"traffic_platform"`
	PostgresUsername       string `env:"AUTH_POSTGRES_USERNAME" envDefault:"postgres"`
	PostgresPassword       string `env:"AUTH_POSTGRES_PASSWORD"`
	PostgresSSLMode        string `env:"AUTH_POSTGRES_SSL_MODE" envDefault:"disable"`
	PostgresConnectTimeout int    `env:"AUTH_POSTGRES_CONNECT_TIMEOUT" envDefault:"10"`
	JWTSecretKey           string `env:"JWT_SECRET_KEY"`
}

func (c AuthConfig) ConnectionString() string {
	if c.PostgresDSN != "" {
		return c.PostgresDSN
	}
	if c.PostgresHost == "" || c.PostgresDatabase == "" || c.PostgresUsername == "" {
		return ""
	}
	port := c.PostgresPort
	if port == 0 {
		port = 5432
	}
	sslMode := c.PostgresSSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connectTimeout := c.PostgresConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = 10
	}
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.PostgresUsername, c.PostgresPassword),
		Host:   fmt.Sprintf("%s:%d", c.PostgresHost, port),
		Path:   "/" + c.PostgresDatabase,
	}
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	query.Set("connect_timeout", strconv.Itoa(connectTimeout))
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// 确保默认值
	if len(cfg.Redis.Addrs) == 0 || cfg.Redis.Addrs[0] == "" {
		cfg.Redis.Addrs = []string{"redis-master.databases.svc:6379"}
	}

	if len(cfg.Kafka.Brokers) == 0 || cfg.Kafka.Brokers[0] == "" {
		cfg.Kafka.Brokers = []string{"kafka-bootstrap.middleware.svc:9092"}
	}

	if len(cfg.OpenSearch.Addresses) == 0 || cfg.OpenSearch.Addresses[0] == "" {
		cfg.OpenSearch.Addresses = []string{"http://opensearch.middleware.svc:9200"}
	}

	// 安全验证：启用认证时禁止缺失或弱 JWT 密钥。
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate 安全配置检查
func (c *Config) validate() error {
	if len(c.API.AllowedOrigins) == 0 {
		return fmt.Errorf("API_ALLOWED_ORIGINS must contain at least one explicit origin")
	}
	for _, origin := range c.API.AllowedOrigins {
		if origin == "*" {
			// 生产环境禁止通配 CORS：fail-closed，必须显式配置来源域
			return fmt.Errorf("API_ALLOWED_ORIGINS must not contain '*'; configure explicit origins")
		}
	}
	if c.Auth.Enabled && (len(c.Auth.JWTSecretKey) < 32 || c.Auth.JWTSecretKey == "your-256-bit-secret-key-here") {
		return fmt.Errorf("JWT_SECRET_KEY must be supplied by a secret reference and contain at least 32 characters when auth is enabled")
	}
	if c.OpenSearch.Password == "" {
		println("⚠ SECURITY WARNING: OpenSearch password is empty. Set OPENSEARCH_PASSWORD environment variable.")
	}
	if c.OpenSearch.SearchCursorEnabled {
		if !c.OpenSearch.V2Enabled || strings.TrimSpace(c.OpenSearch.ReadAlias) == "" {
			return fmt.Errorf("OpenSearch cursor search requires OPENSEARCH_ALERTS_V2_ENABLED and an exact read alias")
		}
		if len(c.Auth.JWTSecretKey) < 32 || c.Auth.JWTSecretKey == "your-256-bit-secret-key-here" {
			return fmt.Errorf("OpenSearch cursor search requires a secret-backed JWT_SECRET_KEY of at least 32 characters")
		}
		if c.OpenSearch.SearchMaxPageSize < 1 || c.OpenSearch.SearchMaxPageSize > 200 ||
			c.OpenSearch.SearchShallowLimit < c.OpenSearch.SearchMaxPageSize || c.OpenSearch.SearchShallowLimit > 10000 ||
			c.OpenSearch.SearchQueryTimeout < 100*time.Millisecond || c.OpenSearch.SearchQueryTimeout > 10*time.Second ||
			c.OpenSearch.SearchCursorTTL < 10*time.Second || c.OpenSearch.SearchCursorTTL > 15*time.Minute ||
			c.OpenSearch.SearchTrackTotal < 100 || c.OpenSearch.SearchTrackTotal > 100000 {
			return fmt.Errorf("OpenSearch cursor search budgets are outside the approved bounds")
		}
	}
	if err := c.ProbeOperation.Validate(c.Kafka); err != nil {
		return err
	}
	return nil
}

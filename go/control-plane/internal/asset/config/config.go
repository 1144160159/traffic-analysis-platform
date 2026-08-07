package config

import (
	"strings"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

type Config struct {
	Server     ServerConfig
	Postgres   PostgresConfig
	Metrics    MetricsConfig
	Kafka      KafkaConfig
	Projection ProjectionConfig
	Discovery  DiscoveryConfig
	Export     AssetExportConfig
	Detail     AssetDetailConfig
	Governance AssetGovernanceConfig
	Auth       AuthConfig
	Cursor     CursorConfig
}

type ServerConfig struct {
	GRPCPort int `env:"ASSET_GRPC_PORT" envDefault:"50053"`
	HTTPPort int `env:"ASSET_HTTP_PORT" envDefault:"8083"`
}

type PostgresConfig struct {
	Host     string `env:"ASSET_PG_HOST" envDefault:"postgres-primary.databases.svc"`
	Port     int    `env:"ASSET_PG_PORT" envDefault:"5432"`
	User     string `env:"ASSET_PG_USER" envDefault:"postgres"`
	Password string `env:"ASSET_PG_PASSWORD"`
	Database string `env:"ASSET_PG_DB" envDefault:"traffic_platform"`
	SSLMode  string `env:"ASSET_PG_SSLMODE" envDefault:"disable"`
}

type MetricsConfig struct {
	Enabled bool `env:"ASSET_METRICS_ENABLED" envDefault:"true"`
	Port    int  `env:"ASSET_METRICS_PORT" envDefault:"9094"`
}

type AuthConfig struct {
	JWTSigningKey string `env:"JWT_SIGNING_KEY"`
}

type CursorConfig struct {
	Enabled bool `env:"ASSET_CURSOR_V2_ENABLED" envDefault:"false"`
}

type AssetExportConfig struct {
	Enabled        bool          `env:"ASSET_EXPORT_JOBS_V1_ENABLED" envDefault:"false"`
	WorkerEnabled  bool          `env:"ASSET_EXPORT_WORKER_ENABLED" envDefault:"false"`
	WorkerInterval time.Duration `env:"ASSET_EXPORT_WORKER_INTERVAL" envDefault:"2s"`
	WorkerLease    time.Duration `env:"ASSET_EXPORT_WORKER_LEASE" envDefault:"5m"`
	MaxRows        int           `env:"ASSET_EXPORT_MAX_ROWS" envDefault:"100000"`
	MaxBytes       int64         `env:"ASSET_EXPORT_MAX_BYTES" envDefault:"104857600"`
	Retention      time.Duration `env:"ASSET_EXPORT_RETENTION" envDefault:"168h"`
	Bucket         string        `env:"ASSET_EXPORT_BUCKET" envDefault:"report-artifacts"`
	S3Endpoint     string        `env:"S3_ENDPOINT" envDefault:"minio.minio.svc:9000"`
	S3AccessKey    string        `env:"S3_ACCESS_KEY"`
	S3SecretKey    string        `env:"S3_SECRET_KEY"`
	S3UseSSL       bool          `env:"S3_USE_SSL" envDefault:"false"`
	S3CAFile       string        `env:"S3_CA_CERT"`
	OutboxEnabled  bool          `env:"ASSET_EXPORT_OUTBOX_ENABLED" envDefault:"false"`
	EventTopic     string        `env:"ASSET_EXPORT_EVENT_TOPIC" envDefault:"asset.exports.v1"`
}

type AssetDetailConfig struct {
	SnapshotV1Enabled    bool          `env:"ASSET_DETAIL_SNAPSHOT_V1_ENABLED" envDefault:"false"`
	ClickHouseEnabled    bool          `env:"ASSET_DETAIL_CLICKHOUSE_ENABLED" envDefault:"false"`
	ClickHouseHosts      []string      `env:"CLICKHOUSE_HOSTS" envSeparator:"," envDefault:"clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000"`
	ClickHouseDatabase   string        `env:"CLICKHOUSE_DATABASE" envDefault:"traffic"`
	ClickHouseUsername   string        `env:"CLICKHOUSE_USERNAME" envDefault:"default"`
	ClickHousePassword   string        `env:"CLICKHOUSE_PASSWORD"`
	ClickHouseDial       time.Duration `env:"ASSET_DETAIL_CLICKHOUSE_DIAL_TIMEOUT" envDefault:"5s"`
	ClickHouseRead       time.Duration `env:"ASSET_DETAIL_CLICKHOUSE_READ_TIMEOUT" envDefault:"8s"`
	ClickHouseQuery      time.Duration `env:"ASSET_DETAIL_CLICKHOUSE_QUERY_TIMEOUT" envDefault:"3s"`
	ClickHouseLookback   time.Duration `env:"ASSET_DETAIL_CLICKHOUSE_LOOKBACK" envDefault:"168h"`
	ClickHouseAlertLimit int           `env:"ASSET_DETAIL_CLICKHOUSE_ALERT_LIMIT" envDefault:"50"`
	ClickHouseMaxRows    uint64        `env:"ASSET_DETAIL_CLICKHOUSE_MAX_ROWS_TO_READ" envDefault:"5000000"`
	ClickHouseMaxBytes   uint64        `env:"ASSET_DETAIL_CLICKHOUSE_MAX_BYTES_TO_READ" envDefault:"536870912"`
	NebulaEnabled        bool          `env:"ASSET_DETAIL_NEBULA_ENABLED" envDefault:"false"`
	NebulaRelationLimit  int           `env:"ASSET_DETAIL_NEBULA_RELATION_LIMIT" envDefault:"100"`
	EvidenceEnabled      bool          `env:"ASSET_DETAIL_EVIDENCE_ENABLED" envDefault:"false"`
	EvidenceLimit        int           `env:"ASSET_DETAIL_EVIDENCE_LIMIT" envDefault:"100"`
}

type AssetGovernanceConfig struct {
	Enabled bool `env:"ASSET_GOVERNANCE_V1_ENABLED" envDefault:"false"`
}

type KafkaConfig struct {
	Enabled                bool          `env:"ASSET_KAFKA_ENABLED" envDefault:"true"`
	Brokers                string        `env:"ASSET_KAFKA_BROKERS" envDefault:"kafka-bootstrap.middleware.svc:9092"`
	Topic                  string        `env:"ASSET_KAFKA_TOPIC" envDefault:"asset.bindings.v1"`
	GroupID                string        `env:"ASSET_KAFKA_GROUP_ID" envDefault:"asset-service-bindings"`
	MinBytes               int           `env:"ASSET_KAFKA_MIN_BYTES" envDefault:"1"`
	MaxBytes               int           `env:"ASSET_KAFKA_MAX_BYTES" envDefault:"1048576"`
	EventOutboxEnabled     bool          `env:"ASSET_EVENT_OUTBOX_ENABLED" envDefault:"true"`
	EventTopic             string        `env:"ASSET_EVENT_TOPIC" envDefault:"asset.events.v2"`
	DiscoveryOutboxEnabled bool          `env:"ASSET_DISCOVERY_OUTBOX_ENABLED" envDefault:"true"`
	DiscoveryEventTopic    string        `env:"ASSET_DISCOVERY_EVENT_TOPIC" envDefault:"asset.discovery.events.v1"`
	OutboxInterval         time.Duration `env:"ASSET_OUTBOX_INTERVAL" envDefault:"500ms"`
	OutboxLease            time.Duration `env:"ASSET_OUTBOX_LEASE" envDefault:"30s"`
	OutboxMaxAttempts      int           `env:"ASSET_OUTBOX_MAX_ATTEMPTS" envDefault:"8"`
	OutboxBatchSize        int           `env:"ASSET_OUTBOX_BATCH_SIZE" envDefault:"50"`
	ProjectionEnabled      bool          `env:"ASSET_PROJECTION_ENABLED" envDefault:"true"`
	ProjectionGroupID      string        `env:"ASSET_PROJECTION_GROUP_ID" envDefault:"asset-projection-v2"`
	ProjectionDLQTopic     string        `env:"ASSET_PROJECTION_DLQ_TOPIC" envDefault:"dlq.v1"`
	ProjectionMaxAttempts  int           `env:"ASSET_PROJECTION_MAX_ATTEMPTS" envDefault:"8"`
	Security               kafkaCommon.SecurityConfig
}

type ProjectionConfig struct {
	Interval   time.Duration `env:"ASSET_PROJECTION_INTERVAL" envDefault:"500ms"`
	Lease      time.Duration `env:"ASSET_PROJECTION_LEASE" envDefault:"45s"`
	OpenSearch ProjectionOpenSearchConfig
	Nebula     ProjectionNebulaConfig
}

type ProjectionOpenSearchConfig struct {
	Addresses  []string `env:"ASSET_PROJECTION_OS_ADDRESSES" envSeparator:"," envDefault:"http://opensearch.middleware.svc:9200"`
	Username   string   `env:"ASSET_PROJECTION_OS_USERNAME"`
	Password   string   `env:"ASSET_PROJECTION_OS_PASSWORD"`
	WriteAlias string   `env:"ASSET_PROJECTION_OS_WRITE_ALIAS" envDefault:"assets-v2-write"`
}

type ProjectionNebulaConfig struct {
	Addresses   []string      `env:"ASSET_PROJECTION_NEBULA_ADDRESSES" envSeparator:"," envDefault:"nebula-graph.middleware.svc:9669"`
	Username    string        `env:"ASSET_PROJECTION_NEBULA_USERNAME" envDefault:"root"`
	Password    string        `env:"ASSET_PROJECTION_NEBULA_PASSWORD"`
	Space       string        `env:"ASSET_PROJECTION_NEBULA_SPACE" envDefault:"traffic_graph"`
	Timeout     time.Duration `env:"ASSET_PROJECTION_NEBULA_TIMEOUT" envDefault:"10s"`
	IdleTime    time.Duration `env:"ASSET_PROJECTION_NEBULA_IDLE_TIME" envDefault:"30m"`
	MaxPoolSize int           `env:"ASSET_PROJECTION_NEBULA_MAX_POOL_SIZE" envDefault:"20"`
	MinPoolSize int           `env:"ASSET_PROJECTION_NEBULA_MIN_POOL_SIZE" envDefault:"2"`
}

type DiscoveryConfig struct {
	JobsV2Enabled     bool          `env:"ASSET_DISCOVERY_JOBS_V2_ENABLED" envDefault:"false"`
	WorkerEnabled     bool          `env:"ASSET_DISCOVERY_WORKER_ENABLED" envDefault:"false"`
	WorkerInterval    time.Duration `env:"ASSET_DISCOVERY_WORKER_INTERVAL" envDefault:"1s"`
	WorkerLease       time.Duration `env:"ASSET_DISCOVERY_WORKER_LEASE" envDefault:"2m"`
	SchedulerEnabled  bool          `env:"ASSET_DISCOVERY_SCHEDULER_ENABLED" envDefault:"false"`
	Interval          time.Duration `env:"ASSET_DISCOVERY_INTERVAL" envDefault:"30m"`
	InitialDelay      time.Duration `env:"ASSET_DISCOVERY_INITIAL_DELAY" envDefault:"30s"`
	TenantID          string        `env:"ASSET_DISCOVERY_TENANT_ID" envDefault:"default"`
	Mode              string        `env:"ASSET_DISCOVERY_MODE" envDefault:"snmp_lldp"`
	TargetCIDR        string        `env:"ASSET_DISCOVERY_TARGET_CIDR"`
	CredentialID      string        `env:"ASSET_DISCOVERY_CREDENTIAL_ID"`
	RequestedBy       string        `env:"ASSET_DISCOVERY_REQUESTED_BY" envDefault:"asset-discovery-scheduler"`
	SchedulerReason   string        `env:"ASSET_DISCOVERY_SCHEDULER_REASON"`
	SchedulerApprover string        `env:"ASSET_DISCOVERY_SCHEDULER_APPROVER"`
	SchedulerRate     int           `env:"ASSET_DISCOVERY_SCHEDULER_RATE" envDefault:"10"`
	SNMPCommunity     string        `env:"ASSET_DISCOVERY_SNMP_COMMUNITY"`
	SNMPPort          uint16        `env:"ASSET_DISCOVERY_SNMP_PORT" envDefault:"161"`
	SNMPTimeout       time.Duration `env:"ASSET_DISCOVERY_SNMP_TIMEOUT" envDefault:"3s"`
	SNMPRetries       int           `env:"ASSET_DISCOVERY_SNMP_RETRIES" envDefault:"1"`
	MaxHosts          int           `env:"ASSET_DISCOVERY_MAX_HOSTS" envDefault:"128"`
}

func (c KafkaConfig) BrokerList() []string {
	parts := strings.Split(c.Brokers, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		broker := strings.TrimSpace(part)
		if broker != "" {
			brokers = append(brokers, broker)
		}
	}
	return brokers
}

type AssetRecord struct {
	AssetID     string         `json:"asset_id"`
	Revision    int64          `json:"revision"`
	DisplayCode string         `json:"display_code"`
	TenantID    string         `json:"tenant_id"`
	AssetType   string         `json:"asset_type"`
	Status      string         `json:"status"`
	IPAddress   string         `json:"ip_address"`
	MACAddress  string         `json:"mac_address"`
	Hostname    string         `json:"hostname,omitempty"`
	Vendor      string         `json:"vendor,omitempty"`
	OSType      string         `json:"os_type,omitempty"`
	Source      string         `json:"source"`
	VlanID      string         `json:"vlan_id,omitempty"`
	SwitchPort  string         `json:"switch_port,omitempty"`
	Department  string         `json:"department,omitempty"`
	Campus      string         `json:"campus,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	Criticality int            `json:"criticality"`
	Tags        map[string]any `json:"tags,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	FirstSeen   time.Time      `json:"first_seen"`
	LastSeen    time.Time      `json:"last_seen"`
}

type AssetUpsertCommand struct {
	ActionID               string
	ExpectedRevision       int64
	ResolveCurrentRevision bool
	IdempotencyKey         string
	Actor                  string
	Reason                 string
	HistoryEventType       string
	ObservedAt             time.Time
	TraceID                string
	RequestID              string
	ClientIP               string
	UserAgent              string
}

type AssetUpsertResult struct {
	AssetID          string `json:"asset_id"`
	Created          bool   `json:"created"`
	Revision         int64  `json:"revision"`
	EventID          string `json:"event_id"`
	OutboxID         int64  `json:"outbox_id"`
	TraceID          string `json:"trace_id"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

const (
	AssetUpsertAction            = "asset-upsert"
	AssetObservationUpsertAction = "asset-observation-upsert"
	AssetInactiveSweepAction     = "asset-inactive-sweep"
)

type AssetInactiveCommand struct {
	ActionID       string
	IdempotencyKey string
	Actor          string
	Reason         string
	TraceID        string
	RequestID      string
	Cutoff         time.Time
}

type AssetInactiveResult struct {
	Count            int      `json:"count"`
	EventIDs         []string `json:"event_ids"`
	TraceID          string   `json:"trace_id"`
	IdempotentReplay bool     `json:"idempotent_replay"`
}

type AssetListFilter struct {
	AssetType  string `json:"asset_type,omitempty"`
	Status     string `json:"status,omitempty"`
	Search     string `json:"search,omitempty"`
	Department string `json:"department,omitempty"`
	Campus     string `json:"campus,omitempty"`
	IPPrefix   string `json:"ip_prefix,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
}

type AssetCursorPosition struct {
	SnapshotAt   time.Time
	SnapshotXIDs string
	LastSeen     time.Time
	LastAssetID  string
	Total        int
}

type AssetCursorPage struct {
	Assets       []*AssetRecord
	Total        int
	SnapshotAt   time.Time
	SnapshotXIDs string
	LastSeen     time.Time
	LastAssetID  string
	HasMore      bool
}

type AssetStats struct {
	Total                int `json:"total"`
	Active               int `json:"active"`
	Inactive             int `json:"inactive"`
	Unknown              int `json:"unknown"`
	HighCriticality      int `json:"high_criticality"`
	CriticalAssets       int `json:"critical_assets"`
	Unowned              int `json:"unowned"`
	OpenServices         int `json:"open_services"`
	HighRiskServices     int `json:"high_risk_services"`
	WeakPasswords        int `json:"weak_passwords"`
	NetworkInterfaces    int `json:"network_interfaces"`
	ConfigurationChanges int `json:"configuration_changes"`
	DependencyAssets     int `json:"dependency_assets"`
	KeyServices          int `json:"key_services"`
	SLAAtRisk            int `json:"sla_at_risk"`
	OwnershipCandidates  int `json:"ownership_candidates"`
	PendingTickets       int `json:"pending_tickets"`
	ContextRecords       int `json:"context_records"`
}

type AssetEvent struct {
	EventID   int       `json:"event_id"`
	AssetID   string    `json:"asset_id"`
	TenantID  string    `json:"tenant_id"`
	EventType string    `json:"event_type"`
	OldValue  string    `json:"old_value,omitempty"`
	NewValue  string    `json:"new_value,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AssetNetworkInterface is a persisted network observation attached to an asset.
// Values originate from discovery/probe metadata and are never synthesized by the UI.
type AssetNetworkInterface struct {
	Name          string  `json:"name"`
	Adapter       string  `json:"adapter"`
	IPAddress     string  `json:"ip_address"`
	MACAddress    string  `json:"mac_address"`
	VlanID        string  `json:"vlan_id"`
	MirrorMode    string  `json:"mirror_mode"`
	Status        string  `json:"status"`
	Speed         string  `json:"speed"`
	Duplex        string  `json:"duplex"`
	IngressBytes  uint64  `json:"ingress_bytes"`
	EgressBytes   uint64  `json:"egress_bytes"`
	PacketLossPct float64 `json:"packet_loss_pct"`
	ErrorCount    int     `json:"error_count"`
	ProbeID       string  `json:"probe_id"`
}

type AssetOpenService struct {
	Port              int    `json:"port"`
	Protocol          string `json:"protocol"`
	Service           string `json:"service"`
	Version           string `json:"version"`
	ExposureScope     string `json:"exposure_scope"`
	AccessSourceCount int    `json:"access_source_count"`
	RiskLevel         string `json:"risk_level"`
	AlertCount        int    `json:"alert_count"`
}

type AssetOwnershipLink struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Owner  string `json:"owner"`
	Status string `json:"status"`
}

type AssetResponsibility struct {
	Role   string `json:"role"`
	Owner  string `json:"owner"`
	Status string `json:"status"`
}

type AssetOwnership struct {
	Campus           string                `json:"campus"`
	Department       string                `json:"department"`
	Owner            string                `json:"owner"`
	BusinessSystems  []AssetOwnershipLink  `json:"business_systems"`
	AssetGroups      []AssetOwnershipLink  `json:"asset_groups"`
	DataDomains      []AssetOwnershipLink  `json:"data_domains"`
	Responsibilities []AssetResponsibility `json:"responsibilities"`
	PendingFields    []string              `json:"pending_fields"`
}

type AssetDetails struct {
	AssetID           string                  `json:"asset_id"`
	DataContract      string                  `json:"data_contract"`
	NetworkInterfaces []AssetNetworkInterface `json:"network_interfaces"`
	OpenServices      []AssetOpenService      `json:"open_services"`
	Ownership         AssetOwnership          `json:"ownership"`
	ObservedAt        time.Time               `json:"observed_at"`
}

// AssetTopologyNode is a render-neutral node returned by the asset topology API.
// The UI computes positions, while identity and business state remain API data.
type AssetTopologyNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
	Risk   string `json:"risk,omitempty"`
}

// AssetTopologyEdge preserves source/target and observed relationship semantics.
// It must not be replaced by a UI-generated star relationship.
type AssetTopologyEdge struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	Target       string    `json:"target"`
	Relationship string    `json:"relationship"`
	Direction    string    `json:"direction,omitempty"`
	Protocol     string    `json:"protocol,omitempty"`
	Health       string    `json:"health,omitempty"`
	Confidence   int       `json:"confidence,omitempty"`
	ObservedAt   time.Time `json:"observed_at,omitempty"`
}

type AssetTopologyGraph struct {
	AssetID     string              `json:"asset_id"`
	Source      string              `json:"source"`
	FixtureMode bool                `json:"fixture_mode"`
	Nodes       []AssetTopologyNode `json:"nodes"`
	Edges       []AssetTopologyEdge `json:"edges"`
	ObservedAt  time.Time           `json:"observed_at"`
}

// AssetDetailSnapshot binds every PostgreSQL-backed detail section to one
// repeatable-read snapshot. Cross-store sections stay explicitly missing until
// their authoritative readers provide a compatible watermark.
type AssetDetailSnapshot struct {
	ContractVersion   int                      `json:"contract_version"`
	SnapshotID        string                   `json:"snapshot_id"`
	Asset             *AssetRecord             `json:"asset"`
	Details           AssetDetails             `json:"details"`
	History           []*AssetEvent            `json:"history"`
	Topology          AssetTopologyGraph       `json:"topology"`
	Observations      *AssetObservationSummary `json:"observations,omitempty"`
	AlertContext      *AssetAlertContext       `json:"alert_context,omitempty"`
	GraphProjection   *AssetGraphProjection    `json:"graph_projection,omitempty"`
	EvidenceObjects   *AssetEvidenceObjectSet  `json:"evidence_objects,omitempty"`
	AvailableSections []string                 `json:"available_sections"`
	MissingSections   []string                 `json:"missing_sections"`
	Partial           bool                     `json:"partial"`
	SourceWatermarks  map[string]string        `json:"source_watermarks"`
	AsOf              time.Time                `json:"as_of"`
}

// AssetResolvedIdentity records which current, authoritative PostgreSQL
// identity was used to bind a cross-store read back to a stable asset ID.
type AssetResolvedIdentity struct {
	Kind          string `json:"kind"`
	Value         string `json:"value"`
	AssetRevision int64  `json:"asset_revision"`
}

// AssetObservationSummary is a bounded aggregate from ClickHouse sessions.
// It intentionally returns no fabricated observations when the identity has
// no matching session data.
type AssetObservationSummary struct {
	AssetID          string                `json:"asset_id"`
	ResolvedIdentity AssetResolvedIdentity `json:"resolved_identity"`
	Source           string                `json:"source"`
	WindowStart      time.Time             `json:"window_start"`
	WindowEnd        time.Time             `json:"window_end"`
	FirstObservedAt  *time.Time            `json:"first_observed_at,omitempty"`
	LastObservedAt   *time.Time            `json:"last_observed_at,omitempty"`
	SessionCount     uint64                `json:"session_count"`
	BytesTotal       uint64                `json:"bytes_total"`
	PacketsTotal     uint64                `json:"packets_total"`
	DistinctPeers    uint64                `json:"distinct_peers"`
	Protocols        []uint32              `json:"protocols"`
}

type AssetAlertSummary struct {
	AlertID         string    `json:"alert_id"`
	Severity        string    `json:"severity"`
	Status          string    `json:"status"`
	AlertType       string    `json:"alert_type"`
	SourceIP        string    `json:"src_ip"`
	DestinationIP   string    `json:"dst_ip"`
	SourcePort      uint32    `json:"src_port"`
	DestinationPort uint32    `json:"dst_port"`
	Protocol        uint32    `json:"protocol"`
	Score           float32   `json:"score"`
	EvidenceIDs     []string  `json:"evidence_ids"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	StateVersion    uint64    `json:"state_version"`
	EventID         string    `json:"event_id"`
}

// AssetAlertContext is the latest deterministic state for a bounded number of
// alerts associated with the current authoritative asset identity.
type AssetAlertContext struct {
	AssetID          string                `json:"asset_id"`
	ResolvedIdentity AssetResolvedIdentity `json:"resolved_identity"`
	Source           string                `json:"source"`
	WindowStart      time.Time             `json:"window_start"`
	WindowEnd        time.Time             `json:"window_end"`
	Alerts           []AssetAlertSummary   `json:"alerts"`
	Truncated        bool                  `json:"truncated"`
}

type AssetGraphProjectionRelation struct {
	RelationID   string         `json:"relation_id"`
	SourceID     string         `json:"source_id"`
	TargetID     string         `json:"target_id"`
	RelationType string         `json:"relation_type"`
	RiskLevel    string         `json:"risk_level,omitempty"`
	EvidenceID   string         `json:"evidence_id,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Weight       float32        `json:"weight"`
	ObservedAt   time.Time      `json:"observed_at,omitempty"`
}

// AssetGraphProjection is the tenant-scoped, bounded NebulaGraph projection
// for exactly one stable asset ID. Stale projections are returned as evidence
// but remain missing for completion purposes until their revision catches up.
type AssetGraphProjection struct {
	AssetID           string                         `json:"asset_id"`
	Source            string                         `json:"source"`
	Label             string                         `json:"label"`
	Detail            string                         `json:"detail"`
	RiskScore         uint8                          `json:"risk_score"`
	RiskLevel         string                         `json:"risk_level"`
	Icon              string                         `json:"icon"`
	Metadata          map[string]any                 `json:"metadata"`
	ProjectedRevision int64                          `json:"projected_revision"`
	PostgresRevision  int64                          `json:"postgres_revision"`
	UpdatedAt         time.Time                      `json:"updated_at"`
	Relations         []AssetGraphProjectionRelation `json:"relations"`
	Truncated         bool                           `json:"truncated"`
	Stale             bool                           `json:"stale"`
}

type AssetEvidenceObjectManifest struct {
	EvidenceID      string    `json:"evidence_id"`
	AlertID         string    `json:"alert_id"`
	EvidenceType    string    `json:"evidence_type"`
	Summary         string    `json:"summary"`
	Bucket          string    `json:"bucket"`
	ObjectKey       string    `json:"object_key"`
	ObjectVersion   string    `json:"object_version,omitempty"`
	ContentType     string    `json:"content_type"`
	SizeBytes       int64     `json:"size_bytes"`
	ETag            string    `json:"etag,omitempty"`
	SHA256          string    `json:"sha256,omitempty"`
	IntegrityStatus string    `json:"integrity_status"`
	EvidenceAt      time.Time `json:"evidence_at"`
	LastModified    time.Time `json:"last_modified"`
}

// AssetEvidenceObjectSet reconciles bounded alert evidence references with
// ClickHouse evidence rows and MinIO object metadata. Missing SHA256 metadata
// remains unverified and prevents the section from being considered complete.
type AssetEvidenceObjectSet struct {
	AssetID            string                        `json:"asset_id"`
	Source             string                        `json:"source"`
	Objects            []AssetEvidenceObjectManifest `json:"objects"`
	MissingEvidenceIDs []string                      `json:"missing_evidence_ids"`
	Truncated          bool                          `json:"truncated"`
	Partial            bool                          `json:"partial"`
}

// MacIpBinding MAC→IP 绑定（来自 ARP/DHCP 被动发现）
type MacIpBinding struct {
	MACAddress string `json:"mac_address"`
	IPAddress  string `json:"ip_address"`
	TenantID   string `json:"tenant_id"`
	ObservedAt int64  `json:"observed_at"`
	Source     string `json:"source"` // arp / dhcp
}

package config

import (
	"strings"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

type Config struct {
	Server    ServerConfig
	Postgres  PostgresConfig
	Metrics   MetricsConfig
	Kafka     KafkaConfig
	Discovery DiscoveryConfig
	Auth      AuthConfig
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

type KafkaConfig struct {
	Enabled  bool   `env:"ASSET_KAFKA_ENABLED" envDefault:"true"`
	Brokers  string `env:"ASSET_KAFKA_BROKERS" envDefault:"kafka-bootstrap.middleware.svc:9092"`
	Topic    string `env:"ASSET_KAFKA_TOPIC" envDefault:"asset.bindings.v1"`
	GroupID  string `env:"ASSET_KAFKA_GROUP_ID" envDefault:"asset-service-bindings"`
	MinBytes int    `env:"ASSET_KAFKA_MIN_BYTES" envDefault:"1"`
	MaxBytes int    `env:"ASSET_KAFKA_MAX_BYTES" envDefault:"1048576"`
	Security kafkaCommon.SecurityConfig
}

type DiscoveryConfig struct {
	SchedulerEnabled bool          `env:"ASSET_DISCOVERY_SCHEDULER_ENABLED" envDefault:"false"`
	Interval         time.Duration `env:"ASSET_DISCOVERY_INTERVAL" envDefault:"30m"`
	InitialDelay     time.Duration `env:"ASSET_DISCOVERY_INITIAL_DELAY" envDefault:"30s"`
	TenantID         string        `env:"ASSET_DISCOVERY_TENANT_ID" envDefault:"default"`
	Mode             string        `env:"ASSET_DISCOVERY_MODE" envDefault:"snmp_lldp"`
	TargetCIDR       string        `env:"ASSET_DISCOVERY_TARGET_CIDR"`
	CredentialID     string        `env:"ASSET_DISCOVERY_CREDENTIAL_ID"`
	RequestedBy      string        `env:"ASSET_DISCOVERY_REQUESTED_BY" envDefault:"asset-discovery-scheduler"`
	SNMPCommunity    string        `env:"ASSET_DISCOVERY_SNMP_COMMUNITY"`
	SNMPPort         uint16        `env:"ASSET_DISCOVERY_SNMP_PORT" envDefault:"161"`
	SNMPTimeout      time.Duration `env:"ASSET_DISCOVERY_SNMP_TIMEOUT" envDefault:"3s"`
	SNMPRetries      int           `env:"ASSET_DISCOVERY_SNMP_RETRIES" envDefault:"1"`
	MaxHosts         int           `env:"ASSET_DISCOVERY_MAX_HOSTS" envDefault:"128"`
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

type AssetListFilter struct {
	AssetType  string
	Status     string
	Search     string
	Department string
	Campus     string
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

// MacIpBinding MAC→IP 绑定（来自 ARP/DHCP 被动发现）
type MacIpBinding struct {
	MACAddress string `json:"mac_address"`
	IPAddress  string `json:"ip_address"`
	TenantID   string `json:"tenant_id"`
	ObservedAt int64  `json:"observed_at"`
	Source     string `json:"source"` // arp / dhcp
}

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
	AssetID    string    `json:"asset_id"`
	TenantID   string    `json:"tenant_id"`
	IPAddress  string    `json:"ip_address"`
	MACAddress string    `json:"mac_address"`
	Hostname   string    `json:"hostname,omitempty"`
	Vendor     string    `json:"vendor,omitempty"`
	OSType     string    `json:"os_type,omitempty"`
	Source     string    `json:"source"`
	VlanID     string    `json:"vlan_id,omitempty"`
	SwitchPort string    `json:"switch_port,omitempty"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
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

// MacIpBinding MAC→IP 绑定（来自 ARP/DHCP 被动发现）
type MacIpBinding struct {
	MACAddress string `json:"mac_address"`
	IPAddress  string `json:"ip_address"`
	TenantID   string `json:"tenant_id"`
	ObservedAt int64  `json:"observed_at"`
	Source     string `json:"source"` // arp / dhcp
}

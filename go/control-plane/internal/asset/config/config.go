package config

import "time"

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Metrics  MetricsConfig
}

type ServerConfig struct {
	GRPCPort int `env:"ASSET_GRPC_PORT" envDefault:"50053"`
	HTTPPort int `env:"ASSET_HTTP_PORT" envDefault:"8083"`
}

type PostgresConfig struct {
	Host     string `env:"ASSET_PG_HOST" envDefault:"postgres-primary.databases.svc"`
	Port     int    `env:"ASSET_PG_PORT" envDefault:"5432"`
	User     string `env:"ASSET_PG_USER" envDefault:"postgres"`
	Password string `env:"ASSET_PG_PASSWORD" envDefault:"pgadmin123"`
	Database string `env:"ASSET_PG_DB" envDefault:"traffic_platform"`
	SSLMode  string `env:"ASSET_PG_SSLMODE" envDefault:"disable"`
}

type MetricsConfig struct {
	Enabled bool   `env:"ASSET_METRICS_ENABLED" envDefault:"true"`
	Port    int    `env:"ASSET_METRICS_PORT" envDefault:"9094"`
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

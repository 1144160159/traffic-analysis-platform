package config

import "testing"

func TestClickHouseHostsNormalizeCommaSeparatedEnvironmentValue(t *testing.T) {
	cfg := ClickHouseConfig{Hosts: []string{"clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000"}}

	got := cfg.GetHosts()
	want := []string{"clickhouse-1.middleware.svc:9000", "clickhouse-2.middleware.svc:9000"}
	if len(got) != len(want) {
		t.Fatalf("GetHosts() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("GetHosts()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestClickHouseHostsNormalizeDefaultDSN(t *testing.T) {
	cfg := ClickHouseConfig{DSN: "clickhouse://default:@clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000/traffic"}

	got := cfg.GetHosts()
	want := []string{"clickhouse-1.middleware.svc:9000", "clickhouse-2.middleware.svc:9000"}
	if len(got) != len(want) {
		t.Fatalf("GetHosts() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("GetHosts()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestAuthConnectionStringFromParts(t *testing.T) {
	cfg := AuthConfig{
		PostgresHost:           "postgres-primary.databases.svc",
		PostgresPort:           5432,
		PostgresDatabase:       "traffic_platform",
		PostgresUsername:       "postgres",
		PostgresPassword:       "pass word/@:",
		PostgresSSLMode:        "disable",
		PostgresConnectTimeout: 10,
	}

	got := cfg.ConnectionString()
	want := "postgres://postgres:pass%20word%2F%40%3A@postgres-primary.databases.svc:5432/traffic_platform?connect_timeout=10&sslmode=disable"
	if got != want {
		t.Fatalf("ConnectionString() = %q, want %q", got, want)
	}
}

func TestAuthConnectionStringPrefersExplicitDSN(t *testing.T) {
	cfg := AuthConfig{
		PostgresDSN:      "postgres://explicit",
		PostgresHost:     "postgres-primary.databases.svc",
		PostgresDatabase: "traffic_platform",
		PostgresUsername: "postgres",
	}

	if got := cfg.ConnectionString(); got != cfg.PostgresDSN {
		t.Fatalf("ConnectionString() = %q, want explicit DSN", got)
	}
}

package main

import "testing"

func TestPostgresDSNFromEnvPrefersExplicitDSN(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://user:pass@example/traffic?sslmode=require")
	t.Setenv("POSTGRES_PASSWORD", "ignored")

	got := postgresDSNFromEnv()
	want := "postgres://user:pass@example/traffic?sslmode=require"
	if got != want {
		t.Fatalf("postgresDSNFromEnv() = %q, want %q", got, want)
	}
}

func TestPostgresDSNFromEnvBuildsSecretBackedKeywordDSN(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("POSTGRES_HOST", "postgres-primary.databases.svc")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_USERNAME", "postgres")
	t.Setenv("POSTGRES_PASSWORD", `pa'ss\word`)
	t.Setenv("POSTGRES_DATABASE", "traffic_platform")
	t.Setenv("POSTGRES_SSL_MODE", "disable")
	t.Setenv("POSTGRES_CONNECT_TIMEOUT", "5")

	got := postgresDSNFromEnv()
	want := `host='postgres-primary.databases.svc' port='5432' user='postgres' password='pa\'ss\\word' dbname='traffic_platform' sslmode='disable' connect_timeout='5'`
	if got != want {
		t.Fatalf("postgresDSNFromEnv() = %q, want %q", got, want)
	}
}

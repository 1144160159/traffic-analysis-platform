// Package config analysis-service 配置(fail-closed:必需项缺失拒启)。
package config

import (
	"fmt"
	"os"
)

// Config 运行配置(全部来自环境变量/Secret 注入,零默认凭证)。
type Config struct {
	ListenAddr   string
	PostgresDSN  string
	JWTSecretSet bool
}

// Load 读取环境变量;PG DSN 由部署注入(POSTGRES_DSN 或分段变量)。
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr: getenv("ANALYSIS_LISTEN_ADDR", ":8090"),
	}
	dsn := os.Getenv("ANALYSIS_POSTGRES_DSN")
	if dsn == "" {
		host := getenv("POSTGRES_HOST", "")
		if host == "" {
			return nil, fmt.Errorf("ANALYSIS_POSTGRES_DSN (or POSTGRES_HOST) is required")
		}
		user := getenv("POSTGRES_USERNAME", "postgres")
		pass := os.Getenv("POSTGRES_PASSWORD")
		if pass == "" {
			return nil, fmt.Errorf("POSTGRES_PASSWORD is required (inject from Secret)")
		}
		db := getenv("POSTGRES_DATABASE", "traffic_platform")
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", host, user, pass, db)
	}
	cfg.PostgresDSN = dsn
	cfg.JWTSecretSet = os.Getenv("JWT_SECRET") != ""
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

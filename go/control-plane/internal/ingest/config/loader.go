package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/caarlos0/env/v10"
)

func LoadFromEnvFile(filePath string) error {

	if _, err := os.Stat(filePath); os.IsNotExist(err) {

		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open env file %s: %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line %d in %s: %s", lineNum, filePath, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		value = strings.Trim(value, `"'`)

		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("failed to set env var %s: %w", key, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading env file %s: %w", filePath, err)
	}

	return nil
}

func Load() (*Config, error) {

	envPaths := []string{
		DefaultConfigPath,
		"control-plane/config.env",
		"/etc/ingest-gateway/config.env",
		"/config/config.env",
	}

	if customPath := os.Getenv("CONFIG_FILE"); customPath != "" {
		envPaths = append([]string{customPath}, envPaths...)
	}

	environment := os.Getenv(EnvEnvironment)
	if environment == "" {
		environment = EnvironmentDevelopment
	}

	envSpecificPaths := []string{
		fmt.Sprintf("config.%s.env", environment),
		fmt.Sprintf("control-plane/config.%s.env", environment),
		fmt.Sprintf("/etc/ingest-gateway/config.%s.env", environment),
	}
	envPaths = append(envSpecificPaths, envPaths...)

	loadedFrom := ""
	for _, path := range envPaths {
		if err := LoadFromEnvFile(path); err != nil {

			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to load env file %s: %w", path, err)
			}
		} else if _, statErr := os.Stat(path); statErr == nil {
			loadedFrom = path
			break
		}
	}

	cfg := &Config{}

	opts := env.Options{
		RequiredIfNoDef: false,
	}

	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, fmt.Errorf("failed to parse config from environment: %w", err)
	}

	cfg.SetDefaults()

	if err := validateProductionConfig(cfg, environment); err != nil {
		return nil, err
	}

	if loadedFrom != "" {

		fmt.Printf("Configuration loaded from: %s (environment: %s)\n", loadedFrom, environment)
	} else {
		fmt.Printf("Configuration loaded from environment variables only (environment: %s)\n", environment)
	}

	return cfg, nil
}

func validateProductionConfig(cfg *Config, environment string) error {
	if environment != EnvironmentProduction {
		return nil
	}

	if cfg.JWT.SigningKey == "your-256-bit-secret-key-here" || cfg.JWT.SigningKey == "" {
		return fmt.Errorf("JWT_SIGNING_KEY must be set in production environment")
	}

	if !cfg.Auth.RequireMTLS {
		fmt.Println("WARNING: mTLS is disabled in production, this is not recommended")
	}

	if cfg.Auth.AllowNoToken {
		fmt.Println("WARNING: AllowNoToken is enabled in production, this is not recommended")
	}

	if !cfg.Audit.Enabled {
		return fmt.Errorf("audit logging must be enabled in production environment")
	}

	if len(cfg.Redis.Addrs) == 0 {
		fmt.Println("WARNING: Redis not configured in production, distributed features will be disabled")
	}

	return nil
}

func LoadWithPath(envFilePath string) (*Config, error) {

	if err := LoadFromEnvFile(envFilePath); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg.SetDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("Config validation failed: %v", err))
	}

	return cfg
}

func MustLoadWithPath(envFilePath string) *Config {
	cfg, err := LoadWithPath(envFilePath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config from %s: %v", envFilePath, err))
	}
	return cfg
}

func ReloadFromEnv(cfg *Config) error {
	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}
	cfg.SetDefaults()
	return cfg.Validate()
}

func SaveToFile(cfg *Config, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	if _, err := writer.WriteString("# Ingest Gateway Configuration\n"); err != nil {
		return err
	}
	if _, err := writer.WriteString("# Auto-generated at " +
		fmt.Sprint(os.Getenv("GENERATED_AT")) + "\n\n"); err != nil {
		return err
	}

	writeSection := func(section string, items map[string]string) error {
		if _, err := writer.WriteString(fmt.Sprintf("# %s\n", section)); err != nil {
			return err
		}
		for key, value := range items {
			if _, err := writer.WriteString(fmt.Sprintf("%s=%s\n", key, value)); err != nil {
				return err
			}
		}
		if _, err := writer.WriteString("\n"); err != nil {
			return err
		}
		return nil
	}

	kafkaConfig := map[string]string{
		"KAFKA_BROKERS":            strings.Join(cfg.Kafka.Brokers, ","),
		"KAFKA_FLOW_TOPIC":         cfg.Kafka.FlowTopic,
		"KAFKA_SESSION_TOPIC":      cfg.Kafka.SessionTopic,
		"KAFKA_PCAP_TOPIC":         cfg.Kafka.PcapTopic,
		"KAFKA_DLQ_TOPIC":          cfg.Kafka.DLQTopic,
		"KAFKA_BATCH_SIZE":         fmt.Sprint(cfg.Kafka.BatchSize),
		"KAFKA_COMPRESSION":        cfg.Kafka.Compression,
		"KAFKA_REQUIRED_ACKS":      cfg.Kafka.RequiredAcks,
		"KAFKA_ENABLE_IDEMPOTENCE": fmt.Sprint(cfg.Kafka.EnableIdempotence),
	}
	if err := writeSection("Kafka Configuration", kafkaConfig); err != nil {
		return err
	}

	return nil
}

func GetEnvironment() string {
	env := os.Getenv(EnvEnvironment)
	if env == "" {
		return EnvironmentDevelopment
	}
	return env
}

func IsProduction() bool {
	return GetEnvironment() == EnvironmentProduction
}

func IsDevelopment() bool {
	return GetEnvironment() == EnvironmentDevelopment
}

func IsStaging() bool {
	return GetEnvironment() == EnvironmentStaging
}

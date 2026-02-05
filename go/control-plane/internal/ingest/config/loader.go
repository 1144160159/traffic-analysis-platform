////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/config/loader.go
// 修复版 v7：
// 1. 修复 env.Options 报错问题 (移除 Environment: os.Environ())
// 2. 保持多环境配置加载逻辑
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/caarlos0/env/v10"
)

// LoadFromEnvFile 从 .env 文件加载配置到环境变量
func LoadFromEnvFile(filePath string) error {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// 文件不存在不是错误，允许只使用环境变量
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

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 移除 export 前缀（兼容 shell 格式）
		line = strings.TrimPrefix(line, "export ")

		// 解析 KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line %d in %s: %s", lineNum, filePath, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除引号（支持单引号和双引号）
		value = strings.Trim(value, `"'`)

		// 只设置未设置的环境变量（环境变量优先级高于文件）
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

// Load 加载配置（优先级：环境变量 > .env 文件 > 默认值）
func Load() (*Config, error) {
	// 1. 尝试从多个可能的路径加载配置文件
	envPaths := []string{
		DefaultConfigPath,                // ./config.env
		"control-plane/config.env",       // 项目根目录
		"/etc/ingest-gateway/config.env", // 系统配置目录
		"/config/config.env",             // Docker/K8s 挂载路径
	}

	// 支持环境变量指定配置文件路径
	if customPath := os.Getenv("CONFIG_FILE"); customPath != "" {
		envPaths = append([]string{customPath}, envPaths...)
	}

	// 支持多环境配置文件（development, staging, production）
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
			// 如果是文件读取错误（非文件不存在），返回错误
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to load env file %s: %w", path, err)
			}
		} else if _, statErr := os.Stat(path); statErr == nil {
			loadedFrom = path
			break
		}
	}

	// 2. 解析环境变量到结构体
	cfg := &Config{}
	// 修复：移除 Environment: os.Environ()，库默认会读取系统环境变量
	opts := env.Options{
		RequiredIfNoDef: false,
	}

	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, fmt.Errorf("failed to parse config from environment: %w", err)
	}

	// 3. 应用默认值
	cfg.SetDefaults()

	// 4. 生产环境安全检查
	if err := validateProductionConfig(cfg, environment); err != nil {
		return nil, err
	}

	// 5. 记录配置来源
	if loadedFrom != "" {
		// 使用标准输出避免循环依赖
		fmt.Printf("Configuration loaded from: %s (environment: %s)\n", loadedFrom, environment)
	} else {
		fmt.Printf("Configuration loaded from environment variables only (environment: %s)\n", environment)
	}

	return cfg, nil
}

// validateProductionConfig 生产环境配置验证
func validateProductionConfig(cfg *Config, environment string) error {
	if environment != EnvironmentProduction {
		return nil
	}

	// 生产环境必须设置的配置
	if cfg.JWT.SigningKey == "your-256-bit-secret-key-here" || cfg.JWT.SigningKey == "" {
		return fmt.Errorf("JWT_SIGNING_KEY must be set in production environment")
	}

	// 生产环境建议启用 mTLS
	if !cfg.Auth.RequireMTLS {
		fmt.Println("WARNING: mTLS is disabled in production, this is not recommended")
	}

	// 生产环境建议禁用无 Token 访问
	if cfg.Auth.AllowNoToken {
		fmt.Println("WARNING: AllowNoToken is enabled in production, this is not recommended")
	}

	// 生产环境必须配置审计
	if !cfg.Audit.Enabled {
		return fmt.Errorf("audit logging must be enabled in production environment")
	}

	// 生产环境必须配置 Redis（用于分布式限流和去重）
	if len(cfg.Redis.Addrs) == 0 {
		fmt.Println("WARNING: Redis not configured in production, distributed features will be disabled")
	}

	return nil
}

// LoadWithPath 从指定路径加载配置
func LoadWithPath(envFilePath string) (*Config, error) {
	// 1. 加载 .env 文件
	if err := LoadFromEnvFile(envFilePath); err != nil {
		return nil, err
	}

	// 2. 解析环境变量
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 3. 应用默认值
	cfg.SetDefaults()

	// 4. 验证
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// MustLoad 加载配置，失败则 panic
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("Config validation failed: %v", err))
	}

	return cfg
}

// MustLoadWithPath 从指定路径加载配置，失败则 panic
func MustLoadWithPath(envFilePath string) *Config {
	cfg, err := LoadWithPath(envFilePath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config from %s: %v", envFilePath, err))
	}
	return cfg
}

// ReloadFromEnv 从环境变量重新加载配置（用于热更新）
func ReloadFromEnv(cfg *Config) error {
	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}
	cfg.SetDefaults()
	return cfg.Validate()
}

// SaveToFile 保存配置到文件（用于配置导出）
func SaveToFile(cfg *Config, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// 写入文件头
	if _, err := writer.WriteString("# Ingest Gateway Configuration\n"); err != nil {
		return err
	}
	if _, err := writer.WriteString("# Auto-generated at " +
		fmt.Sprint(os.Getenv("GENERATED_AT")) + "\n\n"); err != nil {
		return err
	}

	// 写入配置（使用反射或手动写入）
	// 这里简化实现，实际可以使用 reflect 包自动遍历
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

	// Kafka 配置
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

// GetEnvironment 获取当前环境
func GetEnvironment() string {
	env := os.Getenv(EnvEnvironment)
	if env == "" {
		return EnvironmentDevelopment
	}
	return env
}

// IsProduction 是否生产环境
func IsProduction() bool {
	return GetEnvironment() == EnvironmentProduction
}

// IsDevelopment 是否开发环境
func IsDevelopment() bool {
	return GetEnvironment() == EnvironmentDevelopment
}

// IsStaging 是否测试环境
func IsStaging() bool {
	return GetEnvironment() == EnvironmentStaging
}

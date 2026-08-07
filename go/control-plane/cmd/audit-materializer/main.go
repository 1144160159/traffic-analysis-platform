package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

const (
	serviceName    = "audit-materializer"
	serviceVersion = "1.0.0"
)

type config struct {
	listenAddr string
	brokers    []string
	topic      string
	dlqTopic   string
	groupID    string
	postgres   storage.PostgresConfig
	security   commonkafka.SecurityConfig
}

type readiness struct {
	postgres     *storage.PostgresClient
	kafka        *commonkafka.Consumer
	materializer *audit.Consumer
}

func main() {
	logger, err := logging.NewLogger(logging.Config{
		Level:       envOr("LOG_LEVEL", "info"),
		Format:      envOr("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     serviceName,
		Version:     envOr("SERVICE_VERSION", serviceVersion),
		Environment: envOr("ENVIRONMENT", "production"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize logger:", err)
		os.Exit(1)
	}
	defer logging.Sync(logger)

	if err := run(logger, os.Getenv); err != nil {
		logger.Error("Audit materializer stopped", zap.Error(err))
		logging.Sync(logger)
		os.Exit(1)
	}
}

func run(logger *zap.Logger, getenv func(string) string) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	postgres, err := storage.NewPostgresClient(cfg.postgres, logger)
	if err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}
	defer postgres.Close()

	kafkaConsumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers:              cfg.brokers,
		Topic:                cfg.topic,
		GroupID:              cfg.groupID,
		EnableDLQ:            true,
		DLQTopic:             cfg.dlqTopic,
		CommitOnDLQSuccess:   true,
		CommitOnHandlerError: false,
		DLQPermanentOnly:     true,
		Security:             cfg.security,
	}, logger)
	if err != nil {
		return fmt.Errorf("initialize Kafka consumer: %w", err)
	}
	defer kafkaConsumer.Close()

	materializer := audit.NewConsumer(
		kafkaConsumer,
		postgres.DB(),
		logger,
		cfg.topic,
		cfg.groupID,
	)
	state := readiness{postgres: postgres, kafka: kafkaConsumer, materializer: materializer}
	server := &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           healthMux(state),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("Audit materializer health server starting", zap.String("addr", cfg.listenAddr))
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serverErr <- serveErr
		}
	}()

	consumerErr := make(chan error, 1)
	go func() { consumerErr <- materializer.Start(ctx) }()

	var runErr error
	select {
	case <-ctx.Done():
	case err = <-consumerErr:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			runErr = fmt.Errorf("consume audit log: %w", err)
		}
		stop()
	case err = <-serverErr:
		runErr = fmt.Errorf("serve health endpoints: %w", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil && runErr == nil {
		runErr = fmt.Errorf("shutdown health server: %w", shutdownErr)
	}
	return runErr
}

func healthMux(state readiness) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := state.postgres.DB().PingContext(ctx); err != nil {
			writeStatus(w, http.StatusServiceUnavailable, "postgres_unavailable")
			return
		}
		if err := state.kafka.HealthCheck(); err != nil {
			writeStatus(w, http.StatusServiceUnavailable, "kafka_unavailable")
			return
		}
		writeStatus(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !state.materializer.Ready() {
			writeStatus(w, http.StatusServiceUnavailable, "consumer_not_ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := state.postgres.DB().PingContext(ctx); err != nil {
			writeStatus(w, http.StatusServiceUnavailable, "postgres_unavailable")
			return
		}
		if err := state.kafka.HealthCheck(); err != nil {
			writeStatus(w, http.StatusServiceUnavailable, "kafka_unavailable")
			return
		}
		writeStatus(w, http.StatusOK, "ready")
	})
	return mux
}

func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func loadConfig(getenv func(string) string) (config, error) {
	brokers := splitNonEmpty(valueOr(getenv, "KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092"))
	if len(brokers) == 0 {
		return config{}, fmt.Errorf("KAFKA_BROKERS must contain at least one broker")
	}
	protocol := strings.ToUpper(strings.TrimSpace(valueOr(getenv, "KAFKA_SECURITY_PROTOCOL", "SASL_SSL")))
	username := strings.TrimSpace(getenv("KAFKA_SASL_USERNAME"))
	password := getenv("KAFKA_SASL_PASSWORD")
	if strings.HasPrefix(protocol, "SASL_") && (username == "" || password == "") {
		return config{}, fmt.Errorf("KAFKA_SASL_USERNAME and KAFKA_SASL_PASSWORD are required for %s", protocol)
	}
	pgPassword := getenv("POSTGRES_PASSWORD")
	if pgPassword == "" {
		return config{}, fmt.Errorf("POSTGRES_PASSWORD is required")
	}

	return config{
		listenAddr: valueOr(getenv, "HTTP_LISTEN_ADDR", ":8090"),
		brokers:    brokers,
		topic:      valueOr(getenv, "AUDIT_TOPIC", "audit.logs"),
		dlqTopic:   valueOr(getenv, "AUDIT_DLQ_TOPIC", "dlq.v1"),
		groupID:    valueOr(getenv, "AUDIT_CONSUMER_GROUP", "audit-consumer"),
		postgres: storage.PostgresConfig{
			Host:            valueOr(getenv, "POSTGRES_HOST", "postgres-primary.databases.svc"),
			Port:            intValueOr(getenv, "POSTGRES_PORT", 5432),
			Database:        valueOr(getenv, "POSTGRES_DATABASE", "traffic_platform"),
			Username:        valueOr(getenv, "POSTGRES_USERNAME", "postgres"),
			Password:        pgPassword,
			SSLMode:         valueOr(getenv, "POSTGRES_SSL_MODE", "disable"),
			MaxOpenConns:    intValueOr(getenv, "POSTGRES_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    intValueOr(getenv, "POSTGRES_MAX_IDLE_CONNS", 3),
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: 30 * time.Minute,
			ConnectTimeout:  intValueOr(getenv, "POSTGRES_CONNECT_TIMEOUT", 10),
		},
		security: commonkafka.SecurityConfig{
			SecurityProtocol: protocol,
			SASLMechanism:    valueOr(getenv, "KAFKA_SASL_MECHANISM", "SCRAM-SHA-512"),
			SASLUsername:     username,
			SASLPassword:     password,
			TLSCAFile:        valueOr(getenv, "KAFKA_TLS_CA_FILE", "/etc/kafka/tls/ca.crt"),
			TLSServerName:    valueOr(getenv, "KAFKA_TLS_SERVER_NAME", "kafka-bootstrap.middleware.svc"),
			TLSSkipVerify:    boolValueOr(getenv, "KAFKA_TLS_SKIP_VERIFY", false),
			ClientID:         serviceName,
		},
	}, nil
}

func envOr(key, fallback string) string {
	return valueOr(os.Getenv, key, fallback)
}

func valueOr(getenv func(string) string, key, fallback string) string {
	if value := strings.TrimSpace(getenv(key)); value != "" {
		return value
	}
	return fallback
}

func intValueOr(getenv func(string) string, key string, fallback int) int {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func boolValueOr(getenv func(string) string, key string, fallback bool) bool {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

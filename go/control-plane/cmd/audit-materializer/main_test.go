package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigUsesDedicatedKafkaIdentity(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{
		"POSTGRES_PASSWORD":   "pg-secret",
		"KAFKA_SASL_USERNAME": "traffic-audit-materializer",
		"KAFKA_SASL_PASSWORD": "kafka-secret",
		"KAFKA_BROKERS":       "kafka-a:9092, kafka-b:9092",
	}))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.topic != "audit.logs" || cfg.dlqTopic != "dlq.v1" || cfg.groupID != "audit-consumer" {
		t.Fatalf("unexpected audit contract: topic=%q dlq=%q group=%q", cfg.topic, cfg.dlqTopic, cfg.groupID)
	}
	if got := strings.Join(cfg.brokers, ","); got != "kafka-a:9092,kafka-b:9092" {
		t.Fatalf("unexpected brokers: %s", got)
	}
	if cfg.security.SASLUsername != "traffic-audit-materializer" {
		t.Fatalf("unexpected Kafka identity: %s", cfg.security.SASLUsername)
	}
	if cfg.security.SecurityProtocol != "SASL_SSL" || cfg.security.ClientID != serviceName {
		t.Fatalf("unexpected Kafka security config: %+v", cfg.security)
	}
}

func TestLoadConfigFailsClosedWithoutRequiredSecrets(t *testing.T) {
	_, err := loadConfig(env(map[string]string{"POSTGRES_PASSWORD": "pg-secret"}))
	if err == nil || !strings.Contains(err.Error(), "KAFKA_SASL_USERNAME") {
		t.Fatalf("expected missing Kafka credentials error, got %v", err)
	}

	_, err = loadConfig(env(map[string]string{
		"KAFKA_SASL_USERNAME": "traffic-audit-materializer",
		"KAFKA_SASL_PASSWORD": "kafka-secret",
	}))
	if err == nil || !strings.Contains(err.Error(), "POSTGRES_PASSWORD") {
		t.Fatalf("expected missing PostgreSQL password error, got %v", err)
	}
}

func TestWriteStatusProducesStructuredJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeStatus(recorder, http.StatusServiceUnavailable, "consumer_not_ready")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "consumer_not_ready" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

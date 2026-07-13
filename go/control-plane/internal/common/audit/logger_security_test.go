package audit

import (
	"strings"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"go.uber.org/zap"
)

func TestNewLoggerRejectsInvalidKafkaSecurity(t *testing.T) {
	logger, err := NewLogger(Config{
		KafkaBrokers: []string{"127.0.0.1:9092"},
		Topic:        "audit.logs",
		ServiceName:  "audit-test",
		Security: commonkafka.SecurityConfig{
			SecurityProtocol: "SASL_SSL",
			SASLUsername:     "audit-user",
		},
	}, zap.NewNop())
	if logger != nil {
		_ = logger.Close()
		t.Fatal("expected no logger for invalid Kafka security")
	}
	if err == nil || !strings.Contains(err.Error(), "username and password") {
		t.Fatalf("expected incomplete SASL credentials error, got %v", err)
	}
}

package consumer

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

func TestKafkaSecurityPropagatesToConsumerAndDLQ(t *testing.T) {
	security := kafkaCommon.SecurityConfig{
		SecurityProtocol: "SASL_SSL",
		SASLMechanism:    "SCRAM-SHA-512",
		SASLUsername:     "traffic-app",
		SASLPassword:     "secret",
		TLSCAFile:        "/etc/kafka/tls/ca.crt",
		TLSServerName:    "kafka-bootstrap.middleware.svc",
	}
	cfg := config.KafkaConfig{Brokers: []string{"kafka:9092"}, Topic: "detections.v1", GroupID: "alert-service", Security: security}

	if got := buildKafkaConsumerConfig(cfg).Security; got != security {
		t.Fatalf("consumer security not propagated: got=%+v", got)
	}
	if got := buildKafkaDLQConfig(cfg).Security; got != security {
		t.Fatalf("DLQ security not propagated: got=%+v", got)
	}
}

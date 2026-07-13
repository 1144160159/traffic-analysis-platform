package dlq

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"go.uber.org/zap"
)

func TestReplayTopicWriterAllowsMessageTopic(t *testing.T) {
	producer, err := NewProducer(Config{
		Brokers:        []string{"127.0.0.1:9092"},
		EnableFallback: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewProducer() error = %v", err)
	}
	t.Cleanup(func() {
		_ = producer.Close()
	})

	writer, err := producer.getTopicWriter("dlq.v1")
	if err != nil {
		t.Fatalf("getTopicWriter() error = %v", err)
	}
	if writer.Topic != "" {
		t.Fatalf("replay topic writer must leave Topic empty so kafka.Message.Topic can route replay messages, got %q", writer.Topic)
	}
}

func TestReplayFallbackFilesReportsPreservedInvalidFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "dlq-fallback-invalid.log")
	line := `dlq.v1|tenant-a:event-1|{"original_topic":"flow.events.v1","event_type":"flow","tenant_id":"tenant-a","probe_id":"probe-a","event_id":"event-1","failed_at":"2026-06-29T00:00:00Z","error_message":"invalid payload regression","retry_count":0,"headers":{"tenant_id":"tenant-a"},"payload_base64":"@@not-base64@@"}` + "\n"
	if err := os.WriteFile(filePath, []byte(line), 0644); err != nil {
		t.Fatalf("write invalid fallback file: %v", err)
	}

	producer, err := NewProducer(Config{
		Brokers:         []string{"127.0.0.1:9092"},
		EnableFallback:  true,
		FallbackDir:     dir,
		MaxRetries:      1,
		RetryBackoff:    1,
		ReplayBatchSize: 1,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewProducer() error = %v", err)
	}
	t.Cleanup(func() {
		_ = producer.Close()
	})

	report := producer.ReplayFallbackFiles(context.Background())
	if report.FailedFiles != 1 {
		t.Fatalf("FailedFiles=%d want 1", report.FailedFiles)
	}
	if report.ReplayedFiles != 0 {
		t.Fatalf("ReplayedFiles=%d want 0", report.ReplayedFiles)
	}
	if report.RemainingFallbackFiles != 1 {
		t.Fatalf("RemainingFallbackFiles=%d want 1", report.RemainingFallbackFiles)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("invalid fallback file should be preserved: %v", err)
	}
}

func TestNewProducerRejectsInvalidKafkaSecurity(t *testing.T) {
	producer, err := NewProducer(Config{
		Brokers: []string{"127.0.0.1:9092"},
		Security: kafkaCommon.SecurityConfig{
			SecurityProtocol: "SASL_SSL",
			SASLMechanism:    "unsupported",
			SASLUsername:     "ingest",
			SASLPassword:     "test-only",
		},
	}, zap.NewNop())
	if err == nil {
		if producer != nil {
			_ = producer.Close()
		}
		t.Fatal("NewProducer() expected invalid Kafka security error")
	}
	if producer != nil {
		t.Fatal("NewProducer() returned a producer for invalid Kafka security")
	}
}

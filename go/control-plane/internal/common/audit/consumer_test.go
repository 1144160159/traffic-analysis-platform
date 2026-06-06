package audit

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	auditkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func makeMsg(data []byte) *auditkafka.ReceivedMessage {
	return &auditkafka.ReceivedMessage{
		Message: kafkago.Message{Value: data, Offset: 0, Partition: 0},
	}
}

func TestParseAuditLogMessage(t *testing.T) {
	c := &Consumer{}

	batch := &pb.AuditLogBatch{
		Events: []*pb.AuditLog{{
			EventId: "audit-001", TenantId: "tenant1", UserId: "user1",
			Action: "ALERT_TRIAGE", ObjectType: "alert", ObjectId: "alert-001",
			Detail: `{"note":"confirmed"}`, IpAddr: "192.168.1.1",
			UserAgent: "curl/7.68", CreatedAt: 1717670400000,
		}},
	}
	data, _ := proto.Marshal(batch)

	result, err := c.parseMessage(makeMsg(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.eventID != "audit-001" {
		t.Errorf("expected audit-001, got %s", result.eventID)
	}
}

func TestParseAuditLogSingle(t *testing.T) {
	c := &Consumer{}
	single := &pb.AuditLog{
		EventId: "audit-single", TenantId: "t2", UserId: "u2",
		Action: "LOGIN", CreatedAt: 1717670400000,
	}
	data, _ := proto.Marshal(single)
	result, err := c.parseMessage(makeMsg(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.eventID != "audit-single" {
		t.Error("failed to parse single AuditLog")
	}
}

func TestParseAuditLogJSON(t *testing.T) {
	c := &Consumer{}
	jsonData := []byte(`{"event_id":"json-audit","tenant_id":"t3","action":"EXPORT"}`)
	result, err := c.parseMessage(makeMsg(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.eventID != "json-audit" {
		t.Error("failed to parse JSON audit")
	}
}

func TestParseAuditLogUnknown(t *testing.T) {
	c := &Consumer{}
	_, err := c.parseMessage(makeMsg([]byte("garbage")))
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestDefaultConsumerConfig(t *testing.T) {
	cfg := DefaultConsumerConfig()
	if cfg.Topic != "audit.logs" || cfg.GroupID != "audit-consumer" || cfg.BatchSize != 200 {
		t.Error("default config mismatch")
	}
}

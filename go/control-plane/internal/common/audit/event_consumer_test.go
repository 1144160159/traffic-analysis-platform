package audit

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestUnmarshalUserEvent(t *testing.T) {
	// 构造 UserEventBatch
	batch := &pb.UserEventBatch{
		Events: []*pb.UserEvent{
			{
				EventId:   "evt-001",
				TenantId:  "tenant1",
				UserId:    "user1",
				Username:  "admin",
				EventType: "login",
				SourceIp:  "192.168.1.1",
				UserAgent: "Mozilla/5.0",
				Resource:  "/api/v1/alerts",
				Action:    "GET",
				Result:    "success",
				Timestamp: 1717670400000,
			},
		},
	}
	data, err := proto.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}

	// 验证反序列化
	var decoded pb.UserEventBatch
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(decoded.Events))
	}
	if decoded.Events[0].EventId != "evt-001" {
		t.Errorf("expected evt-001, got %s", decoded.Events[0].EventId)
	}
}

func TestUnmarshalDeviceLog(t *testing.T) {
	batch := &pb.DeviceLogBatch{
		Events: []*pb.DeviceLog{
			{
				LogId:      "log-001",
				TenantId:   "tenant1",
				DeviceIp:   "10.0.0.1",
				DeviceType: "switch",
				Facility:   16,
				Severity:   3,
				Timestamp:  1717670400000,
				Message:    "Interface Gi1/0/1 down",
				Source:     "syslog",
			},
		},
	}
	data, _ := proto.Marshal(batch)

	var decoded pb.DeviceLogBatch
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(decoded.Events))
	}
}

func TestUnmarshalDeadLetter(t *testing.T) {
	dl := &pb.DeadLetter{
		EventId:     "dl-001",
		TenantId:    "tenant1",
		SourceTopic: "flow.events.v1",
		SourceKey:   "key1",
		ErrorMsg:    "parse error",
		RawPayload:  "base64data",
		RetryCount:  1,
		CreatedAt:   1717670400000,
	}
	data, _ := proto.Marshal(dl)

	var decoded pb.DeadLetter
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.EventId != "dl-001" {
		t.Errorf("expected dl-001, got %s", decoded.EventId)
	}
}

func TestJSONToDeviceLog(t *testing.T) {
	raw := map[string]interface{}{
		"log_id":      "json-log-1",
		"tenant_id":   "t1",
		"device_ip":   "10.0.0.1",
		"device_type": "firewall",
		"facility":    float64(1),
		"severity":    float64(5),
		"timestamp":   float64(1717670400000),
		"message":     "test message",
		"source":      "syslog",
	}
	dl := jsonToDeviceLog(raw)
	if dl == nil {
		t.Fatal("expected non-nil DeviceLog")
	}
	if dl.LogId != "json-log-1" {
		t.Errorf("expected json-log-1, got %s", dl.LogId)
	}
}

func TestJSONToDeadLetter(t *testing.T) {
	raw := map[string]interface{}{
		"event_id":     "json-dl-1",
		"tenant_id":    "t1",
		"source_topic": "test.topic",
		"error_msg":    "test error",
		"retry_count":  float64(3),
	}
	dl := jsonToDeadLetter(raw)
	if dl == nil {
		t.Fatal("expected non-nil DeadLetter")
	}
	if dl.EventId != "json-dl-1" {
		t.Errorf("expected json-dl-1, got %s", dl.EventId)
	}
}

func TestJSONToDeviceLogNil(t *testing.T) {
	raw := map[string]interface{}{
		"device_ip": "10.0.0.1",
	}
	dl := jsonToDeviceLog(raw)
	if dl != nil {
		t.Error("expected nil without log_id")
	}
}

func TestJSONToDeadLetterNil(t *testing.T) {
	raw := map[string]interface{}{
		"source_topic": "test",
	}
	dl := jsonToDeadLetter(raw)
	if dl != nil {
		t.Error("expected nil without event_id")
	}
}

func TestKafkaEventTypeConstants(t *testing.T) {
	if KafkaEventUser != "user_event" {
		t.Error("KafkaEventUser mismatch")
	}
	if KafkaEventDevice != "device_log" {
		t.Error("KafkaEventDevice mismatch")
	}
	if KafkaEventDeadLetter != "dead_letter" {
		t.Error("KafkaEventDeadLetter mismatch")
	}
}

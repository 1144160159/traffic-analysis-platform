package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestWriteAssetBindingsUsesCanonicalEnvelopeAndRequiredAcks(t *testing.T) {
	var capturedTopic string
	var captured []kafkaCommon.Message
	producer := &Producer{
		logger: zap.NewNop(),
		config: ProducerConfig{BindingTopic: config.TopicAssetBindings},
		writeBindingBatch: func(_ context.Context, topic string, messages []kafkaCommon.Message) error {
			capturedTopic = topic
			captured = append(captured, messages...)
			return nil
		},
	}
	result, err := producer.WriteAssetBindings(context.Background(), []*pb.MacIpBinding{validAssetBinding("obs-1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateExactSet(1); err != nil {
		t.Fatal(err)
	}
	if result.Items[0].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED ||
		result.Items[0].AckScope != "KAFKA_RECORD" {
		t.Fatalf("result=%+v", result.Items[0])
	}
	if capturedTopic != config.TopicAssetBindings || len(captured) != 1 || captured[0].Key != "tenant-a:00:11:22:33:44:55" {
		t.Fatalf("topic=%q messages=%+v", capturedTopic, captured)
	}
	headers := map[string]string{}
	for _, header := range captured[0].Headers {
		if _, exists := headers[header.Key]; exists {
			t.Fatalf("duplicate header %q", header.Key)
		}
		headers[header.Key] = header.Value
	}
	want := map[string]string{
		"tenant_id": "tenant-a", "probe_id": "probe-a", "observation_id": "obs-1",
		"event_id": "obs-1", "source": "arp", "schema_version": "1",
		"content_type": config.ContentTypeProtobuf, "message_type": config.ProtoMessageAssetBinding,
	}
	if len(headers) != len(want) {
		t.Fatalf("headers=%v", headers)
	}
	for name, value := range want {
		if headers[name] != value {
			t.Fatalf("header %s=%q want %q", name, headers[name], value)
		}
	}
}

func TestWriteAssetBindingsPreservesPartialKafkaOutcome(t *testing.T) {
	producer := &Producer{
		logger: zap.NewNop(),
		config: ProducerConfig{BindingTopic: config.TopicAssetBindings},
		writeBindingBatch: func(context.Context, string, []kafkaCommon.Message) error {
			return kafka.WriteErrors{nil, errors.New("broker unavailable")}
		},
	}
	result, err := producer.WriteAssetBindings(context.Background(), []*pb.MacIpBinding{
		validAssetBinding("obs-1"), validAssetBinding("obs-2"),
	})
	if err == nil {
		t.Fatal("expected partial Kafka error")
	}
	if result.Items[0].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED ||
		result.Items[1].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE {
		t.Fatalf("items=%+v", result.Items)
	}
}

func TestWriteAssetBindingsRejectsInvalidBeforeKafka(t *testing.T) {
	called := false
	producer := &Producer{
		logger: zap.NewNop(), config: ProducerConfig{BindingTopic: config.TopicAssetBindings},
		writeBindingBatch: func(context.Context, string, []kafkaCommon.Message) error { called = true; return nil },
	}
	result, err := producer.WriteAssetBindings(context.Background(), []*pb.MacIpBinding{{ObservationId: "obs-invalid"}})
	if err != nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
	if result.Items[0].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID {
		t.Fatalf("item=%+v", result.Items[0])
	}
}

func validAssetBinding(observationID string) *pb.MacIpBinding {
	return &pb.MacIpBinding{
		MacAddress: "00:11:22:33:44:55", IpAddress: "10.0.0.8",
		TenantId: "tenant-a", ProbeId: "probe-a", ObservationId: observationID,
		ObservedAt: time.Now().Add(-time.Second).UnixMilli(), Source: "arp", SchemaVersion: 1,
	}
}

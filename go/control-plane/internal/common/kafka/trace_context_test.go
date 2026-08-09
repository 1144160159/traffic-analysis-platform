package kafka

import (
	"context"
	"testing"

	segmentkafka "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/trace"

	commonotel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

func remoteContext(t *testing.T, traceID string) context.Context {
	t.Helper()
	spanContext, err := commonotel.NewSpanContext(traceID, "2222222222222222", true)
	if err != nil {
		t.Fatal(err)
	}
	return commonotel.ContextWithRemoteSpanContext(context.Background(), spanContext)
}

func headerValue(headers []segmentkafka.Header, key string) string {
	for _, header := range headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}

func TestBuildKafkaHeadersInjectsW3CAndStableTraceID(t *testing.T) {
	headers, err := buildKafkaHeaders(remoteContext(t, "11111111111111111111111111111111"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := headerValue(headers, "trace_id"); got != "11111111111111111111111111111111" {
		t.Fatalf("unexpected trace_id: %q", got)
	}
	if got := headerValue(headers, "traceparent"); got != "00-11111111111111111111111111111111-2222222222222222-01" {
		t.Fatalf("unexpected traceparent: %q", got)
	}
}

func TestBuildKafkaHeadersRejectsSplitBrainTrace(t *testing.T) {
	_, err := buildKafkaHeaders(remoteContext(t, "11111111111111111111111111111111"), []MessageHeader{{Key: "trace_id", Value: "33333333333333333333333333333333"}})
	if err == nil {
		t.Fatal("conflicting trace identities must be rejected")
	}
}

func TestReceivedMessageContextExtractsTraceparent(t *testing.T) {
	message := &ReceivedMessage{Message: segmentkafka.Message{
		Topic: "events", Partition: 1, Offset: 9,
		Headers: []segmentkafka.Header{{Key: "traceparent", Value: []byte("00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01")}},
	}}
	spanContext := trace.SpanContextFromContext(message.Context(context.Background()))
	if !spanContext.IsValid() || spanContext.TraceID().String() != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("Kafka W3C context was not restored: %v", spanContext)
	}
}

func TestReceivedMessageContextSupportsValidLegacyTraceOnly(t *testing.T) {
	message := &ReceivedMessage{Message: segmentkafka.Message{
		Topic: "events", Partition: 1, Offset: 9,
		Headers: []segmentkafka.Header{{Key: "trace_id", Value: []byte("cccccccccccccccccccccccccccccccc")}},
	}}
	spanContext := trace.SpanContextFromContext(message.Context(context.Background()))
	if !spanContext.IsValid() || spanContext.TraceID().String() != "cccccccccccccccccccccccccccccccc" {
		t.Fatalf("legacy Kafka trace was not restored: %v", spanContext)
	}
}

func TestReceivedMessageContextIgnoresMalformedLegacyTrace(t *testing.T) {
	message := &ReceivedMessage{Message: segmentkafka.Message{Headers: []segmentkafka.Header{{Key: "trace_id", Value: []byte("not-a-w3c-trace")}}}}
	if spanContext := trace.SpanContextFromContext(message.Context(context.Background())); spanContext.IsValid() {
		t.Fatalf("malformed Kafka trace must not be trusted: %v", spanContext)
	}
}

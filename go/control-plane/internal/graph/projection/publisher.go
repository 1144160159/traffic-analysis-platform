package projection

import (
	"context"
	"fmt"
	"strconv"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

type Publisher struct {
	producer *commonkafka.KeyedProducer
}

func NewPublisher(producer *commonkafka.KeyedProducer) (*Publisher, error) {
	if producer == nil || producer.Topic() != Topic {
		return nil, fmt.Errorf("graph projection publisher is pinned to %s", Topic)
	}
	return &Publisher{producer: producer}, nil
}

// Publish emits deterministic protobuf bytes and a complete duplicated
// envelope. It is intentionally not wired into any source service until the
// consumer readiness receipt and rollout gate have been accepted.
func (publisher *Publisher) Publish(ctx context.Context, event *trafficv1.GraphProjectionEvent) error {
	if publisher == nil || publisher.producer == nil {
		return fmt.Errorf("graph projection publisher is unavailable")
	}
	if err := ValidateEvent(event); err != nil {
		return err
	}
	metadata, err := metadataOf(event)
	if err != nil {
		return err
	}
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal graph projection event: %w", err)
	}
	header := event.GetHeader()
	headers := []commonkafka.MessageHeader{
		{Key: "content_type", Value: "application/x-protobuf"},
		{Key: "proto_message_type", Value: ProjectionProtoType},
		{Key: "event_id", Value: header.GetEventId()},
		{Key: "event_type", Value: header.GetEventType()},
		{Key: "schema_version", Value: header.GetSchemaVersion()},
		{Key: "aggregate_type", Value: header.GetAggregateType()},
		{Key: "aggregate_id", Value: header.GetAggregateId()},
		{Key: "aggregate_version", Value: strconv.FormatUint(header.GetAggregateVersion(), 10)},
		{Key: "tenant_id", Value: metadata.tenantID},
		{Key: "projection_kind", Value: metadata.kind},
		{Key: "projection_id", Value: metadata.projectionID},
		{Key: "projection_sha256", Value: metadata.projectionSHA256},
		{Key: "source_event_id", Value: metadata.sourceEventID},
		{Key: "trace_id", Value: header.GetTraceId()},
	}
	if _, err := publisher.producer.Send(ctx, event.GetPartitionKey(), payload, headers...); err != nil {
		return fmt.Errorf("publish graph projection event %s: %w", header.GetEventId(), err)
	}
	return nil
}

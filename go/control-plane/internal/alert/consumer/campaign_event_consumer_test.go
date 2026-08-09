package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type campaignProjectionCapture struct {
	input *api.CampaignEventProjectionInput
}

func (capture *campaignProjectionCapture) ApplyCampaignEventProjection(_ context.Context, input api.CampaignEventProjectionInput) error {
	capture.input = &input
	return nil
}

func TestCampaignMembershipConsumerValidatesHeadersAndApplies(t *testing.T) {
	capture := &campaignProjectionCapture{}
	consumer := &CampaignEventConsumer{applier: capture, expectedStream: "membership", expectedTopic: api.CampaignMembershipEventTopic}
	eventID := "11111111-1111-4111-8111-111111111111"
	relationID := "22222222-2222-4222-8222-222222222222"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.AlertLinked","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":7,"partition_key":"tenant-a:campaign-a","schema_version":2,"campaign_id":"campaign-a","alert_id":"alert-a","relation_id":"` + relationID + `","relation_revision":2,"campaign_revision":7,"trace_id":"trace-1"}`)
	headers := map[string]string{"event_id": eventID, "event_type": "traffic.campaign.v2.AlertLinked", "tenant_id": "tenant-a",
		"stream": "membership", "aggregate_id": relationID, "campaign_id": "campaign-a", "aggregate_version": "7",
		"relation_revision": "2", "schema_version": "2", "trace_id": "trace-1", "target_topic": api.CampaignMembershipEventTopic}
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Topic: api.CampaignMembershipEventTopic,
		Key: []byte("tenant-a:campaign-a"), Value: payload, Partition: 1, Offset: 12, Time: time.Unix(100, 0)}}
	for key, value := range headers {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatalf("handle() error = %v", err)
	}
	if capture.input == nil || capture.input.Stream != "membership" || capture.input.AggregateID != relationID ||
		capture.input.AggregateRevision != 7 || capture.input.RelationRevision != 2 || capture.input.KafkaOffset != 12 {
		t.Fatalf("unexpected projection input: %#v", capture.input)
	}
}

func TestCampaignConsumerRejectsHeaderBodyMismatch(t *testing.T) {
	capture := &campaignProjectionCapture{}
	consumer := &CampaignEventConsumer{applier: capture, expectedStream: "aggregate", expectedTopic: api.CampaignAggregateEventTopic}
	eventID := "33333333-3333-4333-8333-333333333333"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.StatusChanged","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":3,"partition_key":"tenant-a:campaign-a","schema_version":2,"trace_id":"trace-2"}`)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Topic: api.CampaignAggregateEventTopic,
		Key: []byte("tenant-a:campaign-a"), Value: payload, Partition: 0, Offset: 2}}
	for key, value := range map[string]string{"event_id": eventID, "event_type": "traffic.campaign.v2.StatusChanged", "tenant_id": "wrong-tenant",
		"stream": "aggregate", "aggregate_id": "campaign-a", "campaign_id": "campaign-a", "aggregate_version": "3",
		"relation_revision": "0", "schema_version": "2", "trace_id": "trace-2", "target_topic": api.CampaignAggregateEventTopic} {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("header/body mismatch must fail closed")
	}
	if capture.input != nil {
		t.Fatal("mismatched event must not be applied")
	}
}

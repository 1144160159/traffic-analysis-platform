package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/campaignrail"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type campaignProjectionCapture struct {
	input *api.CampaignEventProjectionInput
	err   error
}

func (capture *campaignProjectionCapture) ApplyCampaignEventProjection(_ context.Context, input api.CampaignEventProjectionInput) error {
	capture.input = &input
	return capture.err
}

type campaignReadinessCapture struct {
	receipts []campaignrail.ConsumerReceipt
	asserted bool
}

func (*campaignReadinessCapture) VerifySchema(context.Context) error { return nil }

func (capture *campaignReadinessCapture) RecordConsumerReceipt(_ context.Context, receipt campaignrail.ConsumerReceipt) error {
	capture.receipts = append(capture.receipts, receipt)
	return nil
}

func (capture *campaignReadinessCapture) AssertConsumerReady(context.Context, string, string, string, string) error {
	capture.asserted = true
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

func TestCampaignJSONConsumerRejectsProtobufRailPermanently(t *testing.T) {
	consumer := &CampaignEventConsumer{applier: &campaignProjectionCapture{}, expectedStream: "aggregate", expectedTopic: api.CampaignAggregateEventTopic}
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: api.CampaignAggregateEventTopic,
		Headers: []segmentkafka.Header{
			{Key: "content_type", Value: []byte("application/x-protobuf")},
			{Key: "proto_message_type", Value: []byte("traffic.v1.Campaign")},
		},
	}}
	err := consumer.handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) {
		t.Fatalf("Protobuf cross-rail delivery must be permanent, got %v", err)
	}
}

func TestCampaignJSONConsumerRejectsUnknownTenantPermanently(t *testing.T) {
	capture := &campaignProjectionCapture{}
	consumer := &CampaignEventConsumer{applier: capture, expectedStream: "aggregate", expectedTopic: api.CampaignAggregateEventTopic}
	eventID := "44444444-4444-4444-8444-444444444444"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.StatusChanged","tenant_id":"unknown","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":3,"partition_key":"unknown:campaign-a","schema_version":2,"trace_id":"trace-unknown"}`)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Topic: api.CampaignAggregateEventTopic,
		Key: []byte("unknown:campaign-a"), Value: payload}}
	for key, value := range map[string]string{"event_id": eventID, "event_type": "traffic.campaign.v2.StatusChanged", "tenant_id": "unknown",
		"stream": "aggregate", "aggregate_id": "campaign-a", "campaign_id": "campaign-a", "aggregate_version": "3",
		"relation_revision": "0", "schema_version": "2", "trace_id": "trace-unknown", "target_topic": api.CampaignAggregateEventTopic} {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	err := consumer.handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) || capture.input != nil {
		t.Fatalf("unknown tenant must fail before projection, err=%v input=%#v", err, capture.input)
	}
}

func TestCampaignJSONConsumerRecordsReadyOnlyAfterProjectionCommit(t *testing.T) {
	projectionErr := errors.New("postgres commit unavailable")
	capture := &campaignProjectionCapture{err: projectionErr}
	readiness := &campaignReadinessCapture{}
	consumer := &CampaignEventConsumer{
		applier: capture, expectedStream: "aggregate", expectedTopic: api.CampaignAggregateEventTopic,
		readiness: readiness, candidateSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		consumerGroup: "alert-service-campaign-events-v2", railID: campaignrail.AggregateJSONRailID,
	}
	eventID := "55555555-5555-4555-8555-555555555555"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.StatusChanged","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":3,"partition_key":"tenant-a:campaign-a","schema_version":2,"trace_id":"trace-db"}`)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Topic: api.CampaignAggregateEventTopic,
		Key: []byte("tenant-a:campaign-a"), Value: payload, Partition: 2, Offset: 9, Time: time.Unix(200, 0)}}
	for key, value := range map[string]string{"event_id": eventID, "event_type": "traffic.campaign.v2.StatusChanged", "tenant_id": "tenant-a",
		"stream": "aggregate", "aggregate_id": "campaign-a", "campaign_id": "campaign-a", "aggregate_version": "3",
		"relation_revision": "0", "schema_version": "2", "trace_id": "trace-db", "target_topic": api.CampaignAggregateEventTopic} {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	if err := consumer.handle(context.Background(), message); !errors.Is(err, projectionErr) {
		t.Fatalf("projection error = %v", err)
	}
	if consumer.ready.Load() || len(readiness.receipts) != 0 {
		t.Fatalf("failed projection must not publish readiness: ready=%v receipts=%+v", consumer.ready.Load(), readiness.receipts)
	}

	capture.err = nil
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatalf("handle committed projection: %v", err)
	}
	if !consumer.ready.Load() || len(readiness.receipts) != 1 || readiness.receipts[0].EventID != eventID ||
		readiness.receipts[0].SourcePartition != 2 || readiness.receipts[0].SourceOffset != 9 {
		t.Fatalf("committed projection readiness mismatch: ready=%v receipts=%+v", consumer.ready.Load(), readiness.receipts)
	}
}

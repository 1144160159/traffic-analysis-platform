package consumer

import (
	"strings"
	"testing"
	"time"

	segmentkafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/campaignrail"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestDecodeCampaignProtoMessageAcceptsExactProtobufEnvelope(t *testing.T) {
	message := validCampaignProtoMessage(t)
	input, err := decodeCampaignProtoMessage(message, campaignrail.ProtoTopic)
	if err != nil {
		t.Fatal(err)
	}
	if input.Campaign.GetTenantId() != "tenant-a" || input.Campaign.GetCampaignId() != "campaign-1" ||
		len(input.PayloadSHA256) != 64 || input.KafkaOffset != 9 {
		t.Fatalf("unexpected projection input: %+v", input)
	}
}

func TestDecodeCampaignProtoMessageRejectsJSONCrossRailAndIdentityDrift(t *testing.T) {
	tests := map[string]func(*commonkafka.ReceivedMessage){
		"json": func(message *commonkafka.ReceivedMessage) {
			message.Value = []byte(`{"tenant_id":"tenant-a"}`)
		},
		"content_type": func(message *commonkafka.ReceivedMessage) {
			setCampaignProtoHeader(message, "content_type", "application/json")
		},
		"wrong_topic": func(message *commonkafka.ReceivedMessage) {
			message.Topic = "campaign.events.v2"
		},
		"wrong_key": func(message *commonkafka.ReceivedMessage) {
			message.Key = []byte("tenant-b:campaign-1")
		},
		"duplicate_header": func(message *commonkafka.ReceivedMessage) {
			message.Headers = append(message.Headers, segmentkafka.Header{Key: "tenant_id", Value: []byte("tenant-a")})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			message := validCampaignProtoMessage(t)
			mutate(message)
			if _, err := decodeCampaignProtoMessage(message, campaignrail.ProtoTopic); err == nil {
				t.Fatal("invalid campaign protobuf envelope was accepted")
			}
		})
	}
}

func TestDecodeCampaignProtoMessageRejectsUnknownTenant(t *testing.T) {
	campaign := validCampaignProto()
	campaign.TenantId = "unknown"
	campaign.Header.TenantId = "unknown"
	payload, err := proto.Marshal(campaign)
	if err != nil {
		t.Fatal(err)
	}
	message := validCampaignProtoMessage(t)
	message.Value = payload
	message.Key = []byte("unknown:campaign-1")
	setCampaignProtoHeader(message, "tenant_id", "unknown")
	if _, err := decodeCampaignProtoMessage(message, campaignrail.ProtoTopic); err == nil {
		t.Fatal("unknown tenant reached the protobuf projection")
	}
}

func validCampaignProtoMessage(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	campaign := validCampaignProto()
	payload, err := proto.Marshal(campaign)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{
		"content_type": "application/x-protobuf", "proto_message_type": campaignrail.ProtoMessageType,
		"schema_version": "1", "source_service": campaignrail.ProtoSourceService,
		"target_topic": campaignrail.ProtoTopic, "tenant_id": campaign.GetTenantId(),
		"campaign_id": campaign.GetCampaignId(), "event_id": campaign.GetEventId(),
	}
	kafkaHeaders := make([]segmentkafka.Header, 0, len(headers))
	for key, value := range headers {
		kafkaHeaders = append(kafkaHeaders, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: campaignrail.ProtoTopic, Partition: 2, Offset: 9,
		Key: []byte("tenant-a:campaign-1"), Value: payload, Headers: kafkaHeaders,
		Time: time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC),
	}}
}

func validCampaignProto() *trafficv1.Campaign {
	return &trafficv1.Campaign{
		TenantId: "tenant-a", CampaignId: "campaign-1",
		TsStart: 1700000000000, TsEnd: 1700000001000,
		Entities: []string{"ip:192.0.2.1", "ip:198.51.100.1"}, Alerts: []string{"alert-1"},
		Score: 0.8, Summary: "scan then exploit",
		EventId: "11111111-1111-4111-8111-111111111111", IngestTs: 1700000002000,
		Header: &trafficv1.EventHeader{
			EventId: "11111111-1111-4111-8111-111111111111", TenantId: "tenant-a",
			RunId: "realtime", EventTs: 1700000001000, IngestTs: 1700000002000,
			ProbeId: "cep-engine", FeatureSetId: "campaign",
		},
		CampaignType: "scan_exploit", AttackPhases: []string{"reconnaissance", "initial_access"},
		RuleIds: []string{"rule-1"}, ModelIds: []string{"model-1"},
	}
}

func setCampaignProtoHeader(message *commonkafka.ReceivedMessage, name, value string) {
	for index := range message.Headers {
		if message.Headers[index].Key == name {
			message.Headers[index].Value = []byte(value)
			return
		}
	}
	panic("test header not found: " + strings.TrimSpace(name))
}

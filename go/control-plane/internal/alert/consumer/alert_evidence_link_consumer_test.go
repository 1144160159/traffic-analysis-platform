package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type alertEvidenceProjectionCapture struct {
	input *api.AlertEvidenceLinkProjectionInput
	err   error
}

func (*alertEvidenceProjectionCapture) VerifySchema(context.Context) error { return nil }

func (c *alertEvidenceProjectionCapture) Apply(_ context.Context, input api.AlertEvidenceLinkProjectionInput) error {
	c.input = &input
	return c.err
}

func validAlertEvidenceLinkMessage() *commonkafka.ReceivedMessage {
	eventID := "11111111-1111-4111-8111-111111111111"
	relationID := "22222222-2222-4222-8222-222222222222"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.alert-evidence.v1.Linked","schema_version":1,"tenant_id":"tenant-a","aggregate_type":"alert_evidence_link","aggregate_id":"` + relationID + `","aggregate_version":1,"partition_key":"tenant-a:alert-a","alert_id":"alert-a","evidence_id":"evidence-a","evidence_type":"pcap","status":"linked","source_store":"minio","object_bucket":"evidence","object_key":"tenants/tenant-a/evidence/a.pcap","object_version":"version-1","object_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_bytes":42,"content_type":"application/vnd.tcpdump.pcap","manifest_revision":3,"reason":"incident investigation","trace_id":"trace-1","occurred_at":"2026-08-16T00:00:00Z"}`)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: api.AlertEvidenceLinkEventTopic, Key: []byte("tenant-a:alert-a"), Value: payload,
		Partition: 2, Offset: 17, Time: time.Unix(100, 0).UTC(),
	}}
	for key, value := range map[string]string{
		"event_id": eventID, "event_type": "traffic.alert-evidence.v1.Linked", "tenant_id": "tenant-a",
		"stream": "alert_evidence_link", "aggregate_id": relationID, "aggregate_version": "1",
		"schema_version": "1", "trace_id": "trace-1", "target_topic": api.AlertEvidenceLinkEventTopic,
		"content_type": "application/json",
	} {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return message
}

func TestAlertEvidenceLinkConsumerAppliesExactEnvelope(t *testing.T) {
	capture := &alertEvidenceProjectionCapture{}
	c := &AlertEvidenceLinkConsumer{projection: capture, expectedTopic: api.AlertEvidenceLinkEventTopic}
	message := validAlertEvidenceLinkMessage()
	if err := c.Handle(context.Background(), message); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if capture.input == nil || capture.input.AggregateVersion != 1 ||
		capture.input.ObjectVersion != "version-1" || capture.input.KafkaOffset != 17 {
		t.Fatalf("unexpected projection input: %#v", capture.input)
	}
}

func TestAlertEvidenceLinkConsumerRejectsHeaderBodyMismatchPermanently(t *testing.T) {
	capture := &alertEvidenceProjectionCapture{}
	c := &AlertEvidenceLinkConsumer{projection: capture, expectedTopic: api.AlertEvidenceLinkEventTopic}
	message := validAlertEvidenceLinkMessage()
	for index := range message.Headers {
		if message.Headers[index].Key == "tenant_id" {
			message.Headers[index].Value = []byte("tenant-b")
		}
	}
	err := c.Handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) || capture.input != nil {
		t.Fatalf("header/body mismatch must fail before projection, err=%v input=%#v", err, capture.input)
	}
}

func TestAlertEvidenceLinkConsumerRejectsUnknownFieldsPermanently(t *testing.T) {
	c := &AlertEvidenceLinkConsumer{projection: &alertEvidenceProjectionCapture{}, expectedTopic: api.AlertEvidenceLinkEventTopic}
	message := validAlertEvidenceLinkMessage()
	message.Value = append(message.Value[:len(message.Value)-1], []byte(`,"tenant_override":"tenant-b"}`)...)
	err := c.Handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) {
		t.Fatalf("unknown field must be permanent, got %v", err)
	}
}

func TestAlertEvidenceLinkConsumerWithdrawsReadyOnProjectionFailure(t *testing.T) {
	capture := &alertEvidenceProjectionCapture{err: errors.New("clickhouse unavailable")}
	c := &AlertEvidenceLinkConsumer{projection: capture, expectedTopic: api.AlertEvidenceLinkEventTopic}
	c.ready.Store(true)
	err := c.Handle(context.Background(), validAlertEvidenceLinkMessage())
	if err == nil || c.Ready(context.Background()) == nil {
		t.Fatalf("projection failure must withdraw readiness, err=%v", err)
	}
}

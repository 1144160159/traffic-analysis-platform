package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	segmentkafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

type fakeProbeReadinessAuthority struct {
	receipts []alertconfig.ProbePipelineReadinessReceipt
	err      error
}

func (authority *fakeProbeReadinessAuthority) IssueRenewRevoke(
	_ context.Context,
	receipt alertconfig.ProbePipelineReadinessReceipt,
) error {
	authority.receipts = append(authority.receipts, receipt)
	return authority.err
}

func readinessMessageFixture(
	t *testing.T,
	now time.Time,
	state pb.ProbeGroupReadinessStateV1,
) *commonkafka.ReceivedMessage {
	t.Helper()
	receipt := &pb.ProbeGroupReadinessReceiptV1{
		ReceiptId:     "11111111-1111-4111-8111-111111111111",
		ConsumerGroup: "ingest-gateway-probe-control-v2",
		ObservedTopic: "probe.control.v2", MemberId: "member-a",
		GenerationId: 7, OwnerEpoch: 42, State: state,
		ObservedAtMs:        now.UnixMilli(),
		PublisherInstanceId: "22222222-2222-4222-8222-222222222222",
	}
	if state == pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY {
		receipt.ExpiresAtMs = now.Add(time.Minute).UnixMilli()
	}
	payload, err := proto.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{}
	for key, value := range map[string]string{
		"event_id":           receipt.ReceiptId,
		"event_type":         probeGroupReadinessEventType,
		"schema_version":     "1",
		"source_service":     probeReadinessSourceService,
		"consumer_group":     receipt.ConsumerGroup,
		"observed_topic":     receipt.ObservedTopic,
		"proto_message_type": "traffic.v1.ProbeGroupReadinessReceiptV1",
		"target_topic":       probeGroupReadinessTopic,
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: probeGroupReadinessTopic, Partition: 0, Offset: 11,
		Key: []byte(receipt.ConsumerGroup), Value: payload, Headers: headers,
	}}
}

func TestProbeReadinessConsumerEnvelopeMatrix(t *testing.T) {
	now := time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		state         pb.ProbeGroupReadinessStateV1
		mutate        func(*commonkafka.ReceivedMessage)
		authorityErr  error
		wantCalls     int
		wantState     alertconfig.ProbePipelineReadinessState
		wantErr       bool
		wantPermanent bool
	}{
		{name: "ready applies live authority", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY, wantCalls: 1, wantState: alertconfig.ProbePipelineReady},
		{name: "assigned is informational", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_ASSIGNED},
		{name: "revoke closes authority", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_REVOKED, wantCalls: 1, wantState: alertconfig.ProbePipelineRevoked},
		{name: "stopped closes authority", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_STOPPED, wantCalls: 1, wantState: alertconfig.ProbePipelineRevoked},
		{name: "wrong Kafka key is permanent", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY, mutate: func(message *commonkafka.ReceivedMessage) { message.Key = []byte("other-group") }, wantErr: true, wantPermanent: true},
		{name: "wrong source service is permanent", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY, mutate: func(message *commonkafka.ReceivedMessage) { setReadinessHeader(message, "source_service", "other") }, wantErr: true, wantPermanent: true},
		{name: "authority storage failure is retryable", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY, authorityErr: errors.New("database unavailable"), wantCalls: 1, wantState: alertconfig.ProbePipelineReady, wantErr: true},
		{name: "stale authority receipt is safely acknowledged", state: pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY, authorityErr: api.ErrProbeReadinessStaleOwner, wantCalls: 1, wantState: alertconfig.ProbePipelineReady},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority := &fakeProbeReadinessAuthority{err: test.authorityErr}
			consumer, err := NewProbeReadinessConsumer(
				authority, "ingest-gateway-probe-control-v2", "probe.control.v2",
			)
			if err != nil {
				t.Fatal(err)
			}
			consumer.now = func() time.Time { return now }
			message := readinessMessageFixture(t, now, test.state)
			if test.mutate != nil {
				test.mutate(message)
			}
			err = consumer.handle(context.Background(), message)
			if (err != nil) != test.wantErr || commonkafka.IsPermanent(err) != test.wantPermanent {
				t.Fatalf("err=%v permanent=%v", err, commonkafka.IsPermanent(err))
			}
			if len(authority.receipts) != test.wantCalls {
				t.Fatalf("authority calls=%d want=%d", len(authority.receipts), test.wantCalls)
			}
			if test.wantCalls == 1 {
				receipt := authority.receipts[0]
				if receipt.State != test.wantState || receipt.OwnerEpoch != 42 ||
					receipt.ConsumerRole != alertconfig.ProbeCommandDeliveryConsumer ||
					receipt.OwnerID != "22222222-2222-4222-8222-222222222222:member-a" {
					t.Fatalf("unexpected authority receipt: %#v", receipt)
				}
			}
		})
	}
}

func TestProbeReadinessConsumerAcknowledgesExpiredRetainedLeaseWithoutAuthority(t *testing.T) {
	observed := time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)
	authority := &fakeProbeReadinessAuthority{}
	consumer, err := NewProbeReadinessConsumer(
		authority, "ingest-gateway-probe-control-v2", "probe.control.v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	consumer.now = func() time.Time { return observed.Add(2 * time.Minute) }
	if err := consumer.handle(context.Background(), readinessMessageFixture(
		t, observed, pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY,
	)); err != nil {
		t.Fatal(err)
	}
	if len(authority.receipts) != 0 {
		t.Fatal("expired retained readiness lease opened authority")
	}
}

func setReadinessHeader(message *commonkafka.ReceivedMessage, key, value string) {
	for index := range message.Headers {
		if message.Headers[index].Key == key {
			message.Headers[index].Value = []byte(value)
			return
		}
	}
}

package consumer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	segmentkafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

const (
	probeGroupReadinessTopic     = "probe.group-readiness.v1"
	probeGroupReadinessEventType = "traffic.probe.v1.GroupReadinessObserved"
	probeReadinessSourceService  = "ingest-gateway"
)

type probeReadinessAuthority interface {
	IssueRenewRevoke(context.Context, alertconfig.ProbePipelineReadinessReceipt) error
}

// ProbeReadinessConsumer is the only adapter allowed to translate the
// ingest-gateway command generation receipt into command-delivery authority.
type ProbeReadinessConsumer struct {
	authority     probeReadinessAuthority
	consumerGroup string
	observedTopic string
	now           func() time.Time
}

func NewProbeReadinessConsumer(
	authority probeReadinessAuthority,
	consumerGroup string,
	observedTopic string,
) (*ProbeReadinessConsumer, error) {
	if authority == nil || strings.TrimSpace(consumerGroup) == "" ||
		strings.TrimSpace(observedTopic) == "" {
		return nil, fmt.Errorf("probe readiness authority command group and topic are required")
	}
	return &ProbeReadinessConsumer{
		authority: authority, consumerGroup: consumerGroup,
		observedTopic: observedTopic, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (consumer *ProbeReadinessConsumer) StartGeneration(
	ctx context.Context,
	runner *commonkafka.GenerationConsumer,
	processor *commonkafka.GenerationMessageProcessor,
) error {
	if consumer == nil || consumer.authority == nil || runner == nil || processor == nil {
		return fmt.Errorf("probe readiness generation runner processor and authority are required")
	}
	return runner.Run(ctx, func(
		generationContext context.Context,
		generation *segmentkafka.Generation,
		topic string,
		assignment segmentkafka.PartitionAssignment,
	) error {
		return processor.ProcessPartition(
			generationContext, generation, topic, assignment, consumer.handle,
		)
	})
}

func (consumer *ProbeReadinessConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if consumer == nil || consumer.authority == nil || consumer.now == nil {
		return fmt.Errorf("probe readiness consumer authority is unavailable")
	}
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("probe readiness Kafka message is nil"))
	}
	if message.Topic != probeGroupReadinessTopic || message.Partition < 0 || message.Offset < 0 {
		return commonkafka.Permanent(fmt.Errorf("probe readiness Kafka source is invalid"))
	}
	var receipt pb.ProbeGroupReadinessReceiptV1
	if err := proto.Unmarshal(message.Value, &receipt); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode probe readiness receipt: %w", err))
	}
	if err := consumer.validateEnvelope(message, &receipt); err != nil {
		return commonkafka.Permanent(err)
	}

	now := consumer.now()
	observedAt := time.UnixMilli(receipt.ObservedAtMs).UTC()
	expiresAt := time.Time{}
	if receipt.ExpiresAtMs > 0 {
		expiresAt = time.UnixMilli(receipt.ExpiresAtMs).UTC()
	}
	if receipt.State == pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_ASSIGNED {
		return nil
	}
	if receipt.State == pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY &&
		!expiresAt.After(now) {
		// A retained lease may be consumed after it expires. It cannot open the
		// fence and is safe to acknowledge without creating quarantine noise.
		return nil
	}
	state := alertconfig.ProbePipelineRevoked
	if receipt.State == pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY {
		state = alertconfig.ProbePipelineReady
	}
	authorityReceipt := alertconfig.ProbePipelineReadinessReceipt{
		PipelineID:    alertconfig.ProbeOperationPipelineID,
		ConsumerRole:  alertconfig.ProbeCommandDeliveryConsumer,
		ConsumerGroup: receipt.ConsumerGroup,
		OwnerID:       receipt.PublisherInstanceId + ":" + receipt.MemberId,
		OwnerEpoch:    receipt.OwnerEpoch, State: state,
		ObservedAt: observedAt, LeaseExpiresAt: expiresAt,
	}
	if err := consumer.authority.IssueRenewRevoke(ctx, authorityReceipt); err != nil {
		if errors.Is(err, api.ErrProbeReadinessStaleOwner) {
			// A stale retained record cannot change authority and must not block
			// progress behind the current owner forever.
			return nil
		}
		return fmt.Errorf("apply remote probe readiness authority: %w", err)
	}
	return nil
}

func (consumer *ProbeReadinessConsumer) validateEnvelope(
	message *commonkafka.ReceivedMessage,
	receipt *pb.ProbeGroupReadinessReceiptV1,
) error {
	if receipt == nil {
		return fmt.Errorf("probe readiness receipt is missing")
	}
	if _, err := uuid.Parse(receipt.ReceiptId); err != nil {
		return fmt.Errorf("probe readiness receipt_id is invalid")
	}
	if _, err := uuid.Parse(receipt.PublisherInstanceId); err != nil {
		return fmt.Errorf("probe readiness publisher_instance_id is invalid")
	}
	if receipt.ConsumerGroup != consumer.consumerGroup ||
		receipt.ObservedTopic != consumer.observedTopic ||
		strings.TrimSpace(receipt.MemberId) == "" || receipt.GenerationId < 0 ||
		receipt.OwnerEpoch <= 0 || receipt.ObservedAtMs <= 0 {
		return fmt.Errorf("probe readiness receipt identity is incomplete or unexpected")
	}
	observedAt := time.UnixMilli(receipt.ObservedAtMs).UTC()
	if observedAt.After(consumer.now().Add(time.Minute)) {
		return fmt.Errorf("probe readiness receipt observed_at is in the future")
	}
	switch receipt.State {
	case pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_ASSIGNED,
		pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_REVOKED,
		pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_STOPPED:
		if receipt.ExpiresAtMs != 0 {
			return fmt.Errorf("non-ready probe readiness receipt cannot extend a lease")
		}
	case pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY:
		expiresAt := time.UnixMilli(receipt.ExpiresAtMs).UTC()
		lease := expiresAt.Sub(observedAt)
		if lease < time.Second || lease > 5*time.Minute {
			return fmt.Errorf("probe readiness receipt lease is invalid")
		}
	default:
		return fmt.Errorf("probe readiness receipt state is unsupported")
	}
	expectedHeaders := map[string]string{
		"event_id":           receipt.ReceiptId,
		"event_type":         probeGroupReadinessEventType,
		"schema_version":     "1",
		"source_service":     probeReadinessSourceService,
		"consumer_group":     receipt.ConsumerGroup,
		"observed_topic":     receipt.ObservedTopic,
		"proto_message_type": "traffic.v1.ProbeGroupReadinessReceiptV1",
		"target_topic":       probeGroupReadinessTopic,
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("probe readiness %s header/body mismatch", key)
		}
	}
	if string(message.Key) != receipt.ConsumerGroup {
		return fmt.Errorf("probe readiness Kafka key/body mismatch")
	}
	if value := message.GetHeader("owner_epoch"); value != "" && value != strconv.FormatInt(receipt.OwnerEpoch, 10) {
		return fmt.Errorf("probe readiness owner_epoch header/body mismatch")
	}
	return nil
}

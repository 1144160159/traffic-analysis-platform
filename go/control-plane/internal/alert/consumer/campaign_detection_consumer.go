package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/campaignrail"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type campaignProtoProjectionAuthority interface {
	VerifySchema(context.Context) error
	ApplyProtoProjection(context.Context, campaignrail.ProtoProjectionInput) error
	RecordConsumerReceipt(context.Context, campaignrail.ConsumerReceipt) error
	AssertConsumerReady(context.Context, string, string, string, string) error
}

// CampaignDetectionConsumer is the campaigns.v1 Protobuf-only detection rail.
// It cannot be pointed at either JSON v2 topic, and it does not publish its
// ready receipt until one real broker delivery has crossed the PostgreSQL
// durability boundary.
type CampaignDetectionConsumer struct {
	consumer        *commonkafka.Consumer
	authority       campaignProtoProjectionAuthority
	candidateSHA256 string
	topic           string
	group           string
	logger          *zap.Logger
	ready           atomic.Bool

	mu          sync.Mutex
	lastReceipt campaignrail.ConsumerReceipt
}

func NewCampaignDetectionConsumer(
	consumer *commonkafka.Consumer,
	authority campaignProtoProjectionAuthority,
	candidateSHA256, topic, group string,
	logger *zap.Logger,
) (*CampaignDetectionConsumer, error) {
	if consumer == nil || authority == nil || !isCampaignSHA(candidateSHA256) ||
		topic != campaignrail.ProtoTopic || strings.TrimSpace(group) == "" {
		return nil, fmt.Errorf("campaign protobuf consumer configuration is invalid")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CampaignDetectionConsumer{
		consumer: consumer, authority: authority, candidateSHA256: candidateSHA256,
		topic: topic, group: group, logger: logger,
	}, nil
}

func (consumer *CampaignDetectionConsumer) Start(ctx context.Context) (runErr error) {
	if err := consumer.authority.VerifySchema(ctx); err != nil {
		return fmt.Errorf("verify campaign protobuf projection schema: %w", err)
	}
	defer func() {
		consumer.ready.Store(false)
		receipt := campaignrail.ConsumerReceipt{
			RailID: campaignrail.ProtoRailID, CandidateSHA256: consumer.candidateSHA256,
			SourceTopic: consumer.topic, ConsumerGroup: consumer.group,
			State: "stopped", ObservedAt: time.Now().UTC(),
		}
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := consumer.authority.RecordConsumerReceipt(stopCtx, receipt); err != nil && runErr == nil {
			runErr = fmt.Errorf("record stopped campaign protobuf consumer: %w", err)
		}
	}()
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *CampaignDetectionConsumer) Close() error { return consumer.consumer.Close() }

func (consumer *CampaignDetectionConsumer) Ready(ctx context.Context) error {
	if !consumer.ready.Load() {
		return campaignrail.ErrConsumerNotReady
	}
	return consumer.authority.AssertConsumerReady(
		ctx, campaignrail.ProtoRailID, consumer.candidateSHA256, consumer.topic, consumer.group)
}

func (consumer *CampaignDetectionConsumer) handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	input, err := decodeCampaignProtoMessage(message, consumer.topic)
	if err != nil {
		consumer.ready.Store(false)
		return commonkafka.Permanent(err)
	}
	if err := consumer.authority.ApplyProtoProjection(ctx, input); err != nil {
		consumer.ready.Store(false)
		if errors.Is(err, campaignrail.ErrProtoIdentityCollision) {
			return commonkafka.Permanent(err)
		}
		return err
	}
	receipt := campaignrail.ConsumerReceipt{
		RailID: campaignrail.ProtoRailID, CandidateSHA256: consumer.candidateSHA256,
		SourceTopic: consumer.topic, ConsumerGroup: consumer.group, State: "ready",
		EventID: input.Campaign.GetEventId(), SourcePartition: input.KafkaPartition,
		SourceOffset: input.KafkaOffset, ObservedAt: input.ReceivedAt,
	}
	if err := consumer.authority.RecordConsumerReceipt(ctx, receipt); err != nil {
		consumer.ready.Store(false)
		return fmt.Errorf("record campaign protobuf consumer ready receipt: %w", err)
	}
	consumer.mu.Lock()
	consumer.lastReceipt = receipt
	consumer.mu.Unlock()
	consumer.ready.Store(true)
	consumer.logger.Info("Campaign protobuf consumer ready receipt committed",
		zap.String("event_id", receipt.EventID), zap.Int("partition", receipt.SourcePartition),
		zap.Int64("offset", receipt.SourceOffset))
	return nil
}

func decodeCampaignProtoMessage(
	message *commonkafka.ReceivedMessage,
	expectedTopic string,
) (campaignrail.ProtoProjectionInput, error) {
	if message == nil || message.Topic != expectedTopic || expectedTopic != campaignrail.ProtoTopic ||
		message.Partition < 0 || message.Offset < 0 || len(message.Value) == 0 || len(message.DuplicateHeaderNames()) > 0 {
		return campaignrail.ProtoProjectionInput{}, fmt.Errorf("invalid campaigns.v1 Kafka source envelope")
	}
	required := map[string]string{
		"content_type": "application/x-protobuf", "proto_message_type": campaignrail.ProtoMessageType,
		"schema_version": campaignrail.ProtoSchema, "source_service": campaignrail.ProtoSourceService,
		"target_topic": campaignrail.ProtoTopic,
	}
	for name, expected := range required {
		if message.GetHeader(name) != expected {
			return campaignrail.ProtoProjectionInput{}, fmt.Errorf("campaign protobuf %s header mismatch", name)
		}
	}
	var campaign trafficv1.Campaign
	if err := proto.Unmarshal(message.Value, &campaign); err != nil {
		return campaignrail.ProtoProjectionInput{}, fmt.Errorf("decode traffic.v1.Campaign: %w", err)
	}
	if _, err := uuid.Parse(campaign.GetEventId()); err != nil ||
		message.GetHeader("tenant_id") != campaign.GetTenantId() ||
		message.GetHeader("campaign_id") != campaign.GetCampaignId() ||
		message.GetHeader("event_id") != campaign.GetEventId() ||
		string(message.Key) != campaign.GetTenantId()+":"+campaign.GetCampaignId() {
		return campaignrail.ProtoProjectionInput{}, fmt.Errorf("campaign protobuf key/header/body identity mismatch")
	}
	receivedAt := message.Time.UTC()
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	digest := sha256.Sum256(message.Value)
	input := campaignrail.ProtoProjectionInput{
		Campaign: &campaign, Payload: append([]byte(nil), message.Value...),
		PayloadSHA256: hex.EncodeToString(digest[:]), KafkaTopic: message.Topic,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset, ReceivedAt: receivedAt,
	}
	if err := campaignrail.ValidateProtoProjectionInput(input); err != nil {
		return campaignrail.ProtoProjectionInput{}, err
	}
	return input, nil
}

func isCampaignSHA(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

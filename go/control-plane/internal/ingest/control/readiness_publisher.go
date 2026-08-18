package control

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const (
	ProbeGroupReadinessTopic     = "probe.group-readiness.v1"
	ProbeGroupReadinessEventType = "traffic.probe.v1.GroupReadinessObserved"
	ProbeGroupReadinessSchema    = "1"
	ProbeReadinessSourceService  = "ingest-gateway"
)

type readinessKeyedProducer interface {
	Send(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error)
	Topic() string
}

// ProbeControlReadinessPublisher turns the real Kafka generation lifecycle
// into broker-acknowledged facts. Its output alone is not admission authority;
// the alert service must consume and fence the receipt in PostgreSQL.
type ProbeControlReadinessPublisher struct {
	producer            readinessKeyedProducer
	publisherInstanceID string
	now                 func() time.Time
	renewalTicker       func(time.Duration) (<-chan time.Time, func())
}

func NewProbeControlReadinessPublisher(
	producer readinessKeyedProducer,
	publisherInstanceID string,
) (*ProbeControlReadinessPublisher, error) {
	if producer == nil || producer.Topic() != ProbeGroupReadinessTopic {
		return nil, fmt.Errorf("probe readiness keyed producer must target %s", ProbeGroupReadinessTopic)
	}
	publisherInstanceID = strings.TrimSpace(publisherInstanceID)
	if publisherInstanceID == "" {
		publisherInstanceID = uuid.NewString()
	}
	if _, err := uuid.Parse(publisherInstanceID); err != nil {
		return nil, fmt.Errorf("probe readiness publisher_instance_id is invalid")
	}
	return &ProbeControlReadinessPublisher{
		producer: producer, publisherInstanceID: publisherInstanceID,
		now: func() time.Time { return time.Now().UTC() },
		renewalTicker: func(interval time.Duration) (<-chan time.Time, func()) {
			ticker := time.NewTicker(interval)
			return ticker.C, ticker.Stop
		},
	}, nil
}

func (publisher *ProbeControlReadinessPublisher) Publish(
	ctx context.Context,
	generation commonkafka.GroupLifecycleReceipt,
	leaseTTL time.Duration,
) (commonkafka.BrokerReceipt, error) {
	if publisher == nil || publisher.producer == nil || publisher.now == nil {
		return commonkafka.BrokerReceipt{}, fmt.Errorf("probe readiness publisher is unavailable")
	}
	if err := validateReadinessGeneration(generation); err != nil {
		return commonkafka.BrokerReceipt{}, err
	}
	if generation.State == commonkafka.GroupLifecycleReady {
		if leaseTTL < time.Second || leaseTTL > 5*time.Minute {
			return commonkafka.BrokerReceipt{}, fmt.Errorf("probe readiness lease TTL must be between 1s and 5m")
		}
	}

	observedAt := publisher.now()
	state, err := readinessProtoState(generation.State)
	if err != nil {
		return commonkafka.BrokerReceipt{}, err
	}
	expiresAt := time.Time{}
	if generation.State == commonkafka.GroupLifecycleReady {
		expiresAt = observedAt.Add(leaseTTL)
	}
	receiptID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join([]string{
		"traffic.probe.group-readiness.v1", publisher.publisherInstanceID,
		generation.GroupID, generation.Topic, generation.MemberID,
		strconv.FormatInt(int64(generation.GenerationID), 10),
		strconv.FormatInt(generation.OwnerEpoch, 10), string(generation.State),
		strconv.FormatInt(observedAt.UnixMilli(), 10),
	}, "\x00"))).String()
	receipt := &pb.ProbeGroupReadinessReceiptV1{
		ReceiptId: receiptID, ConsumerGroup: generation.GroupID,
		ObservedTopic: generation.Topic, MemberId: generation.MemberID,
		GenerationId: generation.GenerationID, OwnerEpoch: generation.OwnerEpoch,
		State: state, ObservedAtMs: observedAt.UnixMilli(),
		PublisherInstanceId: publisher.publisherInstanceID,
	}
	if !expiresAt.IsZero() {
		receipt.ExpiresAtMs = expiresAt.UnixMilli()
	}
	payload, err := proto.Marshal(receipt)
	if err != nil {
		return commonkafka.BrokerReceipt{}, fmt.Errorf("marshal probe group readiness receipt: %w", err)
	}
	brokerReceipt, err := publisher.producer.Send(
		ctx, generation.GroupID, payload,
		commonkafka.MessageHeader{Key: "event_id", Value: receiptID},
		commonkafka.MessageHeader{Key: "event_type", Value: ProbeGroupReadinessEventType},
		commonkafka.MessageHeader{Key: "schema_version", Value: ProbeGroupReadinessSchema},
		commonkafka.MessageHeader{Key: "source_service", Value: ProbeReadinessSourceService},
		commonkafka.MessageHeader{Key: "consumer_group", Value: generation.GroupID},
		commonkafka.MessageHeader{Key: "observed_topic", Value: generation.Topic},
		commonkafka.MessageHeader{Key: "proto_message_type", Value: "traffic.v1.ProbeGroupReadinessReceiptV1"},
		commonkafka.MessageHeader{Key: "target_topic", Value: ProbeGroupReadinessTopic},
	)
	if err != nil {
		return brokerReceipt, fmt.Errorf("publish probe group readiness receipt: %w", err)
	}
	return brokerReceipt, nil
}

// RunRenewal keeps exactly one READY generation lease alive. The caller owns
// ctx and must cancel it before publishing revoke or releasing the generation.
func (publisher *ProbeControlReadinessPublisher) RunRenewal(
	ctx context.Context,
	generation commonkafka.GroupLifecycleReceipt,
	leaseTTL time.Duration,
) error {
	if generation.State != commonkafka.GroupLifecycleReady {
		return fmt.Errorf("probe readiness renewal requires a READY generation")
	}
	if leaseTTL < 2*time.Second || leaseTTL > 5*time.Minute {
		return fmt.Errorf("probe readiness renewal TTL must be between 2s and 5m")
	}
	if err := validateReadinessGeneration(generation); err != nil {
		return err
	}
	if publisher == nil || publisher.renewalTicker == nil {
		return fmt.Errorf("probe readiness renewal timer is unavailable")
	}
	ticks, stop := publisher.renewalTicker(leaseTTL / 3)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticks:
			if _, err := publisher.Publish(ctx, generation, leaseTTL); err != nil {
				return err
			}
		}
	}
}

func validateReadinessGeneration(generation commonkafka.GroupLifecycleReceipt) error {
	if strings.TrimSpace(generation.Topic) == "" || strings.TrimSpace(generation.GroupID) == "" ||
		strings.TrimSpace(generation.MemberID) == "" || strings.TrimSpace(generation.OwnerID) == "" ||
		generation.GenerationID < 0 || generation.OwnerEpoch <= 0 {
		return fmt.Errorf("probe readiness generation identity is incomplete")
	}
	switch generation.State {
	case commonkafka.GroupLifecycleAssigned, commonkafka.GroupLifecycleReady,
		commonkafka.GroupLifecycleRevoked, commonkafka.GroupLifecycleStopped:
		return nil
	default:
		return fmt.Errorf("probe readiness generation state is unsupported")
	}
}

func readinessProtoState(state commonkafka.GroupLifecycleState) (pb.ProbeGroupReadinessStateV1, error) {
	switch state {
	case commonkafka.GroupLifecycleAssigned:
		return pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_ASSIGNED, nil
	case commonkafka.GroupLifecycleReady:
		return pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY, nil
	case commonkafka.GroupLifecycleRevoked:
		return pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_REVOKED, nil
	case commonkafka.GroupLifecycleStopped:
		return pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_STOPPED, nil
	default:
		return pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_UNSPECIFIED,
			fmt.Errorf("probe readiness generation state is unsupported")
	}
}

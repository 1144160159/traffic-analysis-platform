package control

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

const (
	RequestedEventType = "traffic.probe.v2.OperationRequested"
	AgentAckEventType  = "traffic.probe.v2.OperationAgentAcknowledged"
)

type CommandStore interface {
	Put(context.Context, *pb.ProbeOperationCommand) error
	List(context.Context, string, string, int) ([]*pb.ProbeOperationCommand, error)
	Delete(context.Context, string, string, []string) error
}

type AckPublisher interface {
	Publish(
		context.Context,
		string,
		[]byte,
		...commonkafka.MessageHeader,
	) error
}

type KafkaAckPublisher struct {
	Producer *commonkafka.Producer
}

func (publisher *KafkaAckPublisher) Publish(
	ctx context.Context,
	key string,
	payload []byte,
	headers ...commonkafka.MessageHeader,
) error {
	if publisher == nil || publisher.Producer == nil {
		return fmt.Errorf("probe ACK Kafka producer is unavailable")
	}
	return publisher.Producer.Send(ctx, key, payload, headers...)
}

type Bridge struct {
	store     CommandStore
	publisher AckPublisher
}

func NewBridge(store CommandStore, publisher AckPublisher) (*Bridge, error) {
	if store == nil || publisher == nil {
		return nil, fmt.Errorf("probe control store and ACK publisher are required")
	}
	return &Bridge{store: store, publisher: publisher}, nil
}

func (bridge *Bridge) Exchange(
	ctx context.Context,
	tenantID string,
	probeID string,
	acks []*pb.ProbeOperationAck,
) ([]*pb.ProbeOperationCommand, []string, error) {
	tenantID = strings.TrimSpace(tenantID)
	probeID = strings.TrimSpace(probeID)
	if tenantID == "" || probeID == "" {
		return nil, nil, fmt.Errorf("authenticated tenant and probe identities are required")
	}

	accepted := make([]string, 0, len(acks))
	for _, ack := range acks {
		if err := validateAck(ack); err != nil {
			return nil, nil, err
		}
		eventID := deterministicAckEventID(tenantID, probeID, ack)
		payload, err := json.Marshal(map[string]interface{}{
			"event_id":           eventID,
			"event_type":         AgentAckEventType,
			"schema_version":     2,
			"tenant_id":          tenantID,
			"probe_id":           probeID,
			"operation_id":       ack.OperationId,
			"command_revision":   ack.CommandRevision,
			"reported_version":   ack.ReportedVersion,
			"reported_hash":      ack.ReportedHash,
			"agent_version":      ack.AgentVersion,
			"applied":            ack.Applied,
			"error":              ack.Error,
			"acknowledged_at_ms": ack.AcknowledgedAtMs,
			"detail":             json.RawMessage(defaultJSON(ack.DetailJson)),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal probe ACK: %w", err)
		}
		if err := bridge.publisher.Publish(
			ctx,
			tenantID+":"+probeID,
			payload,
			commonkafka.MessageHeader{Key: "event_id", Value: eventID},
			commonkafka.MessageHeader{Key: "event_type", Value: AgentAckEventType},
			commonkafka.MessageHeader{Key: "tenant_id", Value: tenantID},
			commonkafka.MessageHeader{Key: "probe_id", Value: probeID},
			commonkafka.MessageHeader{Key: "operation_id", Value: ack.OperationId},
		); err != nil {
			return nil, nil, fmt.Errorf("publish probe Agent ACK: %w", err)
		}
		accepted = append(accepted, ack.OperationId)
	}

	if len(accepted) > 0 {
		if err := bridge.store.Delete(ctx, tenantID, probeID, accepted); err != nil {
			return nil, nil, fmt.Errorf("remove acknowledged probe commands: %w", err)
		}
	}
	commands, err := bridge.store.List(ctx, tenantID, probeID, 20)
	if err != nil {
		return nil, nil, fmt.Errorf("list probe commands: %w", err)
	}
	return commands, accepted, nil
}

type requestedEvent struct {
	EventID         string          `json:"event_id"`
	EventType       string          `json:"event_type"`
	SchemaVersion   int             `json:"schema_version"`
	TenantID        string          `json:"tenant_id"`
	ProbeID         string          `json:"probe_id"`
	OperationID     string          `json:"operation_id"`
	OperationType   string          `json:"operation_type"`
	CommandRevision int64           `json:"command_revision"`
	DesiredVersion  string          `json:"desired_version"`
	CommandHash     string          `json:"command_hash"`
	ExpiresAt       string          `json:"expires_at"`
	TraceID         string          `json:"trace_id"`
	Command         json.RawMessage `json:"command"`
}

type Router struct {
	store CommandStore
}

func NewRouter(store CommandStore) (*Router, error) {
	if store == nil {
		return nil, fmt.Errorf("probe control command store is required")
	}
	return &Router{store: store}, nil
}

// Route validates a durable Kafka command before placing it in the
// tenant/probe-specific delivery store. Kafka offsets may be committed only
// after this method succeeds.
func (router *Router) Route(ctx context.Context, payload []byte) error {
	var event requestedEvent
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode probe control event: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, event.ExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid probe command expires_at: %w", err)
	}
	command := &pb.ProbeOperationCommand{
		EventId:         event.EventID,
		TenantId:        event.TenantID,
		ProbeId:         event.ProbeID,
		OperationId:     event.OperationID,
		OperationType:   event.OperationType,
		CommandRevision: event.CommandRevision,
		DesiredVersion:  event.DesiredVersion,
		CommandHash:     strings.ToLower(event.CommandHash),
		ExpiresAtMs:     expiresAt.UnixMilli(),
		TraceId:         event.TraceID,
		CommandJson:     event.Command,
	}
	if event.EventType != RequestedEventType || event.SchemaVersion != 2 {
		return fmt.Errorf("unsupported probe control event contract")
	}
	if err := validateCommand(command); err != nil {
		return err
	}
	return router.store.Put(ctx, command)
}

type RedisCommandStore struct {
	client redis.UniversalClient
	ttl    time.Duration
}

func NewRedisCommandStore(client redis.UniversalClient, ttl time.Duration) (*RedisCommandStore, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis is required for probe command routing")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RedisCommandStore{client: client, ttl: ttl}, nil
}

func (store *RedisCommandStore) Put(ctx context.Context, command *pb.ProbeOperationCommand) error {
	if err := validateCommand(command); err != nil {
		return err
	}
	raw, err := proto.Marshal(command)
	if err != nil {
		return err
	}
	key := commandKey(command.TenantId, command.ProbeId)
	pipe := store.client.TxPipeline()
	pipe.HSet(ctx, key, command.OperationId, raw)
	pipe.Expire(ctx, key, store.ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (store *RedisCommandStore) List(
	ctx context.Context,
	tenantID string,
	probeID string,
	limit int,
) ([]*pb.ProbeOperationCommand, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	values, err := store.client.HVals(ctx, commandKey(tenantID, probeID)).Result()
	if err != nil {
		return nil, err
	}
	commands := make([]*pb.ProbeOperationCommand, 0, len(values))
	for _, value := range values {
		command := &pb.ProbeOperationCommand{}
		if err := proto.Unmarshal([]byte(value), command); err != nil {
			return nil, fmt.Errorf("decode routed probe command: %w", err)
		}
		if command.TenantId != tenantID || command.ProbeId != probeID {
			return nil, fmt.Errorf("routed probe command identity mismatch")
		}
		commands = append(commands, command)
	}
	sort.Slice(commands, func(left, right int) bool {
		return commands[left].CommandRevision < commands[right].CommandRevision
	})
	if len(commands) > limit {
		commands = commands[:limit]
	}
	return commands, nil
}

func (store *RedisCommandStore) Delete(
	ctx context.Context,
	tenantID string,
	probeID string,
	operationIDs []string,
) error {
	if len(operationIDs) == 0 {
		return nil
	}
	fields := make([]string, 0, len(operationIDs))
	for _, operationID := range operationIDs {
		if _, err := uuid.Parse(operationID); err != nil {
			return fmt.Errorf("invalid acknowledged operation_id")
		}
		fields = append(fields, operationID)
	}
	return store.client.HDel(ctx, commandKey(tenantID, probeID), fields...).Err()
}

func validateCommand(command *pb.ProbeOperationCommand) error {
	if command == nil {
		return fmt.Errorf("probe command is nil")
	}
	if _, err := uuid.Parse(command.EventId); err != nil {
		return fmt.Errorf("invalid probe command event_id")
	}
	if _, err := uuid.Parse(command.OperationId); err != nil {
		return fmt.Errorf("invalid probe command operation_id")
	}
	if strings.TrimSpace(command.TenantId) == "" || strings.TrimSpace(command.ProbeId) == "" {
		return fmt.Errorf("probe command identity is required")
	}
	if command.CommandRevision <= 0 || command.ExpiresAtMs <= 0 {
		return fmt.Errorf("probe command revision and expiry are required")
	}
	if len(command.CommandHash) != 64 || len(command.CommandJson) == 0 || !json.Valid(command.CommandJson) {
		return fmt.Errorf("probe command hash and JSON payload are invalid")
	}
	return nil
}

func validateAck(ack *pb.ProbeOperationAck) error {
	if ack == nil {
		return fmt.Errorf("probe operation ACK is nil")
	}
	if _, err := uuid.Parse(ack.OperationId); err != nil {
		return fmt.Errorf("invalid ACK operation_id")
	}
	if ack.CommandRevision <= 0 || len(ack.ReportedHash) != 64 ||
		strings.TrimSpace(ack.AgentVersion) == "" || ack.AcknowledgedAtMs <= 0 {
		return fmt.Errorf("probe operation ACK contract is incomplete")
	}
	if len(ack.DetailJson) > 0 && !json.Valid(ack.DetailJson) {
		return fmt.Errorf("probe operation ACK detail is invalid JSON")
	}
	return nil
}

func deterministicAckEventID(tenantID, probeID string, ack *pb.ProbeOperationAck) string {
	name := fmt.Sprintf(
		"traffic.probe.v2.agent-ack:%s:%s:%s:%d",
		tenantID, probeID, ack.OperationId, ack.CommandRevision,
	)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func defaultJSON(value []byte) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	return value
}

func commandKey(tenantID, probeID string) string {
	return "probe_control:v2:" + tenantID + ":" + probeID
}

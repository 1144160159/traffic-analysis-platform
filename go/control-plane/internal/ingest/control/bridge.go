package control

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	RequestedEventType = "traffic.probe.v2.OperationRequested"
	AgentAckEventType  = "traffic.probe.v2.OperationAgentAcknowledged"
)

type CommandStore interface {
	PutIfNewer(context.Context, RoutedCommand) (DeliveryCacheReceipt, error)
	Get(context.Context, string, string, string) (*pb.ProbeOperationCommand, error)
	List(context.Context, string, string, int) ([]*pb.ProbeOperationCommand, error)
	DeleteIfRevision(context.Context, string, string, string, int64) (bool, error)
}

type KafkaSource struct {
	Topic         string            `json:"topic"`
	Partition     int               `json:"partition"`
	Offset        int64             `json:"offset"`
	Key           string            `json:"key"`
	Headers       map[string]string `json:"headers"`
	HeadersSHA256 string            `json:"headers_sha256"`
}

type RoutedCommand struct {
	Command       *pb.ProbeOperationCommand
	Source        KafkaSource
	PayloadSHA256 string
}

type PutCommandStatus string

const (
	PutCommandInserted              PutCommandStatus = "INSERTED"
	PutCommandReplay                PutCommandStatus = "REPLAY"
	PutCommandSameRevisionCollision PutCommandStatus = "SAME_REVISION_COLLISION"
	PutCommandStaleRevision         PutCommandStatus = "STALE_REVISION"
)

type DeliveryCacheReceipt struct {
	EventID         string           `json:"event_id"`
	OperationID     string           `json:"operation_id"`
	CommandRevision int64            `json:"command_revision"`
	Source          KafkaSource      `json:"source"`
	CacheGeneration int64            `json:"cache_generation"`
	Status          PutCommandStatus `json:"status"`
}

var (
	ErrProbeCommandNotFound         = fmt.Errorf("probe command not found in delivery cache")
	ErrProbeCommandRevisionMismatch = fmt.Errorf("probe command revision mismatch")
	ErrProbeCommandCollision        = fmt.Errorf("probe command same-revision collision")
	ErrProbeCommandStaleRevision    = fmt.Errorf("probe command stale revision")
	ErrProbeCommandContract         = fmt.Errorf("probe command contract invalid")
)

type AckPublisher interface {
	Publish(
		context.Context,
		string,
		[]byte,
		...commonkafka.MessageHeader,
	) error
}

type KafkaAckPublisher struct {
	Producer *commonkafka.KeyedProducer
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
	_, err := publisher.Producer.Send(ctx, key, payload, headers...)
	return err
}

type Bridge struct {
	store     CommandStore
	publisher AckPublisher
	// receipts 可选:采集类操作(pcap_replay/capture_window)ACK 后向
	// analysis.receipts.v1 发布 StageReceipt(调度中心回执权威的输入)。
	// 未配置时采集类 ACK 发布失败(fail-closed,不静默丢回执)。
	receipts StageReceiptPublisher
	logger   *zap.Logger
}

func NewBridge(store CommandStore, publisher AckPublisher) (*Bridge, error) {
	if store == nil || publisher == nil {
		return nil, fmt.Errorf("probe control store and ACK publisher are required")
	}
	return &Bridge{store: store, publisher: publisher, logger: zap.NewNop()}, nil
}

// SetLogger 注入日志器(装配层可选;缺省为 nop)。
func (bridge *Bridge) SetLogger(logger *zap.Logger) {
	if logger != nil {
		bridge.logger = logger
	}
}

// SetStageReceiptPublisher 注入采集回执发布器(装配层可选能力)。
func (bridge *Bridge) SetStageReceiptPublisher(p StageReceiptPublisher) {
	bridge.receipts = p
}

// StageReceiptPublisher 采集回执发布端口。
type StageReceiptPublisher interface {
	Publish(ctx context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error
}

// KafkaStageReceiptPublisher analysis.receipts.v1 发布实现。
type KafkaStageReceiptPublisher struct {
	Producer *commonkafka.KeyedProducer
}

func (p *KafkaStageReceiptPublisher) Publish(
	ctx context.Context,
	key string,
	payload []byte,
	headers ...commonkafka.MessageHeader,
) error {
	if p == nil || p.Producer == nil {
		return fmt.Errorf("analysis receipt Kafka producer is unavailable")
	}
	_, err := p.Producer.Send(ctx, key, payload, headers...)
	return err
}

// publishStageReceipt 把采集类操作 ACK 转译为 StageReceipt(回执权威输入)。
// 与 analysis/service/receipt_applier.go 的 ReceiptMessage 契约一致。
func (bridge *Bridge) publishStageReceipt(
	ctx context.Context,
	tenantID string,
	cmd *pb.ProbeOperationCommand,
	ack *pb.ProbeOperationAck,
) error {
	if bridge.receipts == nil {
		return fmt.Errorf("analysis receipt publisher is not configured for %s", cmd.OperationType)
	}
	var detail struct {
		RunID               string `json:"run_id"`
		FencingToken        string `json:"fencing_token"`
		Packets             int64  `json:"packets"`
		BytesConsumed       int64  `json:"bytes_consumed"`
		WatermarkMs         int64  `json:"watermark_ms"`
		Detail              string `json:"detail"`
		ExecutionSpecSHA256 string `json:"execution_spec_sha256"`
	}
	if err := json.Unmarshal(ack.DetailJson, &detail); err != nil {
		return fmt.Errorf("decode replay ACK detail: %w", err)
	}
	// 未应用(validation 失败/stale revision 等)的 ACK detail 不含 run 身份;
	// 从冻结命令负载回填 run_id/fencing_token,否则该 ACK 会让整个 Exchange
	// 永久失败(探针持久化重投),阻塞后续命令投递。
	if detail.RunID == "" || detail.FencingToken == "" {
		var cmdPayload struct {
			RunID        string `json:"run_id"`
			FencingToken string `json:"fencing_token"`
		}
		if err := json.Unmarshal(cmd.CommandJson, &cmdPayload); err != nil {
			return fmt.Errorf("decode cached replay command payload: %w", err)
		}
		detail.RunID = cmdPayload.RunID
		detail.FencingToken = cmdPayload.FencingToken
	}
	if detail.RunID == "" || detail.FencingToken == "" {
		return fmt.Errorf("replay ACK missing run identity (run_id/fencing_token)")
	}
	node := "SOURCE_ACTIVATE"
	attempt := int32(1)
	eventID := deterministicAckEventID(tenantID, node, ack) + "-receipt"
	fenceKind := "source_fence"
	if cmd.OperationType == "capture_window" {
		// 实时采集窗口:source 回执只确认覆盖、不计包数;终局对账据此
		// 放行跨阶段计数守恒并关闭 zero-input 判定。
		fenceKind = "capture_window_fence"
	}
	receipt := map[string]interface{}{
		"event_id":           eventID,
		"schema_version":     "1",
		"tenant_id":          tenantID,
		"run_id":             detail.RunID,
		"execution_node_id":  node,
		"attempt":            attempt,
		"fencing_token":      detail.FencingToken,
		"provider":           "probe-agent",
		"input_count":        detail.Packets,
		"output_count":       0,
		"error_count":        boolInt(!ack.Applied),
		"reject_count":       0,
		"watermark_ms":       detail.WatermarkMs,
		"fence":              json.RawMessage(fmt.Sprintf(`{"kind":%q,"packets":%d,"bytes_consumed":%d,"operation_id":%q,"detail":%q,"object_sha256":%q}`,
			fenceKind, detail.Packets, detail.BytesConsumed, ack.OperationId, detail.Detail, cmd.CommandHash)),
		"payload_hash":      cmd.DesiredVersion,
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("marshal stage receipt: %w", err)
	}
	return bridge.receipts.Publish(ctx, detail.RunID, payload,
		commonkafka.MessageHeader{Key: "event_id", Value: eventID},
		commonkafka.MessageHeader{Key: "tenant_id", Value: tenantID},
		commonkafka.MessageHeader{Key: "run_id", Value: detail.RunID},
	)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
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
		cached, err := bridge.store.Get(ctx, tenantID, probeID, ack.OperationId)
		if errors.Is(err, ErrProbeCommandNotFound) {
			// Idempotent Receiver:命令可能已被上一轮 Exchange 释放(ACK
			// 已处理、命令已从投递缓存删除),或已过期被投递缓存惰性清理。
			// 探针 ACK 出箱是持久化的 at-least-once 重投:把这类 ACK 视为
			// 已接受并丢弃,否则整个 Exchange 永远失败,阻塞后续命令投递
			// (实测 livelock:每次心跳 exchange unavailable)。
			bridge.logger.Warn("probe ACK for released/expired command accepted idempotently",
				zap.String("tenant_id", tenantID),
				zap.String("probe_id", probeID),
				zap.String("operation_id", ack.OperationId))
			accepted = append(accepted, ack.OperationId)
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("load cached probe command for ACK: %w", err)
		}
		if cached.CommandRevision != ack.CommandRevision {
			return nil, nil, fmt.Errorf(
				"%w: operation_id=%s cached=%d acknowledged=%d",
				ErrProbeCommandRevisionMismatch,
				ack.OperationId,
				cached.CommandRevision,
				ack.CommandRevision,
			)
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
			commonkafka.MessageHeader{Key: "command_revision", Value: strconv.FormatInt(ack.CommandRevision, 10)},
			commonkafka.MessageHeader{Key: "schema_version", Value: "2"},
			commonkafka.MessageHeader{Key: "target_topic", Value: "probe.acks.v2"},
		); err != nil {
			return nil, nil, fmt.Errorf("publish probe Agent ACK: %w", err)
		}
		// 采集类操作 ACK → analysis.receipts.v1(回执权威输入;失败则整体失败,重试幂等)
		switch cached.OperationType {
		case "pcap_replay", "capture_window":
			if err := bridge.publishStageReceipt(ctx, tenantID, cached, ack); err != nil {
				return nil, nil, fmt.Errorf("publish stage receipt for %s: %w", cached.OperationType, err)
			}
		}
		deleted, err := bridge.store.DeleteIfRevision(
			ctx, tenantID, probeID, ack.OperationId, ack.CommandRevision,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("release acknowledged probe command: %w", err)
		}
		if !deleted {
			return nil, nil, fmt.Errorf(
				"%w after Kafka ACK: operation_id=%s revision=%d",
				ErrProbeCommandRevisionMismatch,
				ack.OperationId,
				ack.CommandRevision,
			)
		}
		accepted = append(accepted, ack.OperationId)
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
// after the returned receipt binds the exact broker source and Redis write.
func (router *Router) Route(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) (DeliveryCacheReceipt, error) {
	return router.RouteMessage(ctx, message)
}

func (router *Router) StartGeneration(
	ctx context.Context,
	runner *commonkafka.GenerationConsumer,
	processor *commonkafka.GenerationMessageProcessor,
) error {
	if router == nil || router.store == nil || runner == nil || processor == nil {
		return fmt.Errorf("probe command generation runner processor and store are required")
	}
	return runner.Run(ctx, func(
		generationContext context.Context,
		generation *segmentkafka.Generation,
		topic string,
		assignment segmentkafka.PartitionAssignment,
	) error {
		return processor.ProcessPartition(
			generationContext, generation, topic, assignment,
			func(messageContext context.Context, message *commonkafka.ReceivedMessage) error {
				_, routeErr := router.Route(messageContext, message)
				return classifyProbeRouteError(routeErr)
			},
		)
	})
}

func classifyProbeRouteError(err error) error {
	if err == nil || commonkafka.IsPermanent(err) {
		return err
	}
	if errors.Is(err, ErrProbeCommandContract) || errors.Is(err, ErrProbeCommandCollision) ||
		errors.Is(err, ErrProbeCommandStaleRevision) {
		return commonkafka.Permanent(err)
	}
	return err
}

func (router *Router) RouteMessage(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) (DeliveryCacheReceipt, error) {
	if message == nil {
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf("probe control Kafka message is nil"))
	}
	if message.Topic != "probe.control.v2" || message.Partition < 0 || message.Offset < 0 {
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf(
			"invalid probe control Kafka source: topic=%q partition=%d offset=%d",
			message.Topic,
			message.Partition,
			message.Offset,
		))
	}

	var event requestedEvent
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf("decode probe control event: %w", err))
	}
	if err := rejectTrailingProbeControlJSON(decoder); err != nil {
		return DeliveryCacheReceipt{}, probeCommandContractError(err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, event.ExpiresAt)
	if err != nil {
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf("invalid probe command expires_at: %w", err))
	}
	if !expiresAt.After(time.Now()) {
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf("probe command is expired"))
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
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf("unsupported probe control event contract"))
	}
	if err := validateCommand(command); err != nil {
		return DeliveryCacheReceipt{}, probeCommandContractError(err)
	}

	headers, headersDigest, err := canonicalProbeControlHeaders(message.Headers)
	if err != nil {
		return DeliveryCacheReceipt{}, probeCommandContractError(err)
	}
	expectedHeaders := map[string]string{
		"event_id":          event.EventID,
		"event_type":        event.EventType,
		"tenant_id":         event.TenantID,
		"probe_id":          event.ProbeID,
		"operation_id":      event.OperationID,
		"command_revision":  strconv.FormatInt(event.CommandRevision, 10),
		"aggregate_version": strconv.FormatInt(event.CommandRevision, 10),
		"schema_version":    strconv.Itoa(event.SchemaVersion),
		"target_topic":      message.Topic,
	}
	for key, want := range expectedHeaders {
		if got := headers[key]; got != want {
			return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf(
				"probe control header %s mismatch: got=%q want=%q",
				key,
				got,
				want,
			))
		}
	}
	expectedKey := event.TenantID + ":" + event.ProbeID
	if string(message.Key) != expectedKey {
		return DeliveryCacheReceipt{}, probeCommandContractError(fmt.Errorf(
			"probe control Kafka key mismatch: got=%q want=%q",
			string(message.Key),
			expectedKey,
		))
	}
	payloadDigest := sha256.Sum256(message.Value)
	return router.store.PutIfNewer(ctx, RoutedCommand{
		Command: command,
		Source: KafkaSource{
			Topic:         message.Topic,
			Partition:     message.Partition,
			Offset:        message.Offset,
			Key:           string(message.Key),
			Headers:       headers,
			HeadersSHA256: headersDigest,
		},
		PayloadSHA256: fmt.Sprintf("%x", payloadDigest[:]),
	})
}

func probeCommandContractError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrProbeCommandContract, err)
}

func rejectTrailingProbeControlJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("probe control event contains a trailing JSON value")
		}
		return fmt.Errorf("decode trailing probe control JSON: %w", err)
	}
	return nil
}

func canonicalProbeControlHeaders(headers []segmentkafka.Header) (map[string]string, string, error) {
	canonical := make(map[string]string, len(headers))
	for _, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header.Key))
		if key == "" {
			return nil, "", fmt.Errorf("probe control Kafka header key is empty")
		}
		if _, exists := canonical[key]; exists {
			return nil, "", fmt.Errorf("duplicate probe control Kafka header %q", key)
		}
		canonical[key] = string(header.Value)
	}
	keys := make([]string, 0, len(canonical))
	for key := range canonical {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hasher := sha256.New()
	for _, key := range keys {
		_, _ = hasher.Write([]byte(key))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(canonical[key]))
		_, _ = hasher.Write([]byte{0})
	}
	return canonical, fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

type RedisCommandStore struct {
	client redis.UniversalClient
	ttl    time.Duration
}

type cachedCommandRecord struct {
	Command         *pb.ProbeOperationCommand `json:"command"`
	Source          KafkaSource               `json:"source"`
	PayloadSHA256   string                    `json:"payload_sha256"`
	CacheGeneration int64                     `json:"cache_generation"`
}

var putCommandIfNewerScript = redis.NewScript(`
local key = KEYS[1]
local operation_id = ARGV[1]
local incoming_raw = ARGV[2]
local incoming_revision = tonumber(ARGV[3])
local incoming_event_id = ARGV[4]
local incoming_payload_sha = ARGV[5]

local existing_raw = redis.call('HGET', key, operation_id)
if existing_raw then
  local ok, existing = pcall(cjson.decode, existing_raw)
  if not ok or not existing.command then
    return {'LEGACY_ENTRY', '0', existing_raw}
  end
  local existing_revision = tonumber(existing.command.command_revision)
  if existing_revision == incoming_revision and
     existing.command.event_id == incoming_event_id and
     existing.payload_sha256 == incoming_payload_sha then
    return {'REPLAY', tostring(existing.cache_generation or 0), existing_raw}
  end
  if existing_revision == incoming_revision then
    return {'SAME_REVISION_COLLISION', tostring(existing.cache_generation or 0), existing_raw}
  end
  if existing_revision > incoming_revision then
    return {'STALE_REVISION', tostring(existing.cache_generation or 0), existing_raw}
  end
  return {'SAME_REVISION_COLLISION', tostring(existing.cache_generation or 0), existing_raw}
end

local values = redis.call('HGETALL', key)
local max_revision = 0
for index = 1, #values, 2 do
  local field = values[index]
  local raw = values[index + 1]
  if field ~= '__cache_generation' then
    local ok, record = pcall(cjson.decode, raw)
    if not ok or not record.command then
      return {'LEGACY_ENTRY', '0', raw}
    end
    local revision = tonumber(record.command.command_revision) or 0
    if revision > max_revision then
      max_revision = revision
    end
  end
end
if incoming_revision < max_revision then
  return {'STALE_REVISION', '0', ''}
end
if incoming_revision == max_revision and max_revision > 0 then
  return {'SAME_REVISION_COLLISION', '0', ''}
end

local generation = redis.call('HINCRBY', key, '__cache_generation', 1)
local incoming = cjson.decode(incoming_raw)
incoming.cache_generation = generation
local stored_raw = cjson.encode(incoming)
redis.call('HSET', key, operation_id, stored_raw)
redis.call('PERSIST', key)
return {'INSERTED', tostring(generation), stored_raw}
`)

var deleteCommandIfRevisionScript = redis.NewScript(`
local raw = redis.call('HGET', KEYS[1], ARGV[1])
if not raw then
  return 0
end
local ok, record = pcall(cjson.decode, raw)
if not ok or not record.command then
  return -1
end
if tonumber(record.command.command_revision) ~= tonumber(ARGV[2]) then
  return 0
end
return redis.call('HDEL', KEYS[1], ARGV[1])
`)

func NewRedisCommandStore(client redis.UniversalClient, ttl time.Duration) (*RedisCommandStore, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis is required for probe command routing")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RedisCommandStore{client: client, ttl: ttl}, nil
}

func (store *RedisCommandStore) PutIfNewer(
	ctx context.Context,
	routed RoutedCommand,
) (DeliveryCacheReceipt, error) {
	if err := validateRoutedCommand(routed); err != nil {
		return DeliveryCacheReceipt{}, err
	}
	record := cachedCommandRecord{
		Command:       proto.Clone(routed.Command).(*pb.ProbeOperationCommand),
		Source:        routed.Source,
		PayloadSHA256: routed.PayloadSHA256,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return DeliveryCacheReceipt{}, fmt.Errorf("encode routed probe command: %w", err)
	}
	command := routed.Command
	if time.UnixMilli(command.ExpiresAtMs).After(time.Now().Add(store.ttl)) {
		return DeliveryCacheReceipt{}, fmt.Errorf("probe command expiry exceeds delivery cache TTL")
	}
	result, err := putCommandIfNewerScript.Run(
		ctx,
		store.client,
		[]string{commandKey(command.TenantId, command.ProbeId)},
		command.OperationId,
		string(raw),
		command.CommandRevision,
		command.EventId,
		routed.PayloadSHA256,
	).Slice()
	if err != nil {
		return DeliveryCacheReceipt{}, fmt.Errorf("route probe command atomically: %w", err)
	}
	if len(result) != 3 {
		return DeliveryCacheReceipt{}, fmt.Errorf("unexpected Redis route result: %#v", result)
	}
	status := PutCommandStatus(fmt.Sprint(result[0]))
	storedRaw := fmt.Sprint(result[2])
	if status == PutCommandSameRevisionCollision {
		return DeliveryCacheReceipt{}, ErrProbeCommandCollision
	}
	if status == PutCommandStaleRevision {
		return DeliveryCacheReceipt{}, ErrProbeCommandStaleRevision
	}
	if status != PutCommandInserted && status != PutCommandReplay {
		return DeliveryCacheReceipt{}, fmt.Errorf("unsupported Redis route status %q", status)
	}
	var stored cachedCommandRecord
	if err := json.Unmarshal([]byte(storedRaw), &stored); err != nil {
		return DeliveryCacheReceipt{}, fmt.Errorf("decode Redis delivery receipt: %w", err)
	}
	if err := validateCachedCommandRecord(stored); err != nil {
		return DeliveryCacheReceipt{}, err
	}
	return DeliveryCacheReceipt{
		EventID:         stored.Command.EventId,
		OperationID:     stored.Command.OperationId,
		CommandRevision: stored.Command.CommandRevision,
		Source:          stored.Source,
		CacheGeneration: stored.CacheGeneration,
		Status:          status,
	}, nil
}

func (store *RedisCommandStore) Get(
	ctx context.Context,
	tenantID string,
	probeID string,
	operationID string,
) (*pb.ProbeOperationCommand, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(probeID) == "" {
		return nil, fmt.Errorf("probe command identity is required")
	}
	if _, err := uuid.Parse(operationID); err != nil {
		return nil, fmt.Errorf("invalid probe operation_id")
	}
	key := commandKey(tenantID, probeID)
	raw, err := store.client.HGet(ctx, key, operationID).Bytes()
	if err == redis.Nil {
		return nil, ErrProbeCommandNotFound
	}
	if err != nil {
		return nil, err
	}
	record, err := decodeCachedCommandRecord(raw)
	if err != nil {
		return nil, err
	}
	if record.Command.TenantId != tenantID || record.Command.ProbeId != probeID ||
		record.Command.OperationId != operationID {
		return nil, fmt.Errorf("routed probe command identity mismatch")
	}
	if record.Command.ExpiresAtMs <= time.Now().UnixMilli() {
		_ = store.client.HDel(ctx, key, operationID).Err()
		return nil, ErrProbeCommandNotFound
	}
	return proto.Clone(record.Command).(*pb.ProbeOperationCommand), nil
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
	key := commandKey(tenantID, probeID)
	values, err := store.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	commands := make([]*pb.ProbeOperationCommand, 0, len(values))
	expired := make([]string, 0)
	nowMS := time.Now().UnixMilli()
	for operationID, value := range values {
		if operationID == "__cache_generation" {
			continue
		}
		record, err := decodeCachedCommandRecord([]byte(value))
		if err != nil {
			return nil, err
		}
		command := record.Command
		if command.TenantId != tenantID || command.ProbeId != probeID {
			return nil, fmt.Errorf("routed probe command identity mismatch")
		}
		if command.ExpiresAtMs <= nowMS {
			expired = append(expired, operationID)
			continue
		}
		commands = append(commands, proto.Clone(command).(*pb.ProbeOperationCommand))
	}
	if len(expired) > 0 {
		fields := make([]string, len(expired))
		copy(fields, expired)
		if err := store.client.HDel(ctx, key, fields...).Err(); err != nil {
			return nil, fmt.Errorf("remove expired routed probe commands: %w", err)
		}
	}
	sort.Slice(commands, func(left, right int) bool {
		return commands[left].CommandRevision < commands[right].CommandRevision
	})
	if len(commands) > limit {
		commands = commands[:limit]
	}
	return commands, nil
}

func (store *RedisCommandStore) DeleteIfRevision(
	ctx context.Context,
	tenantID string,
	probeID string,
	operationID string,
	commandRevision int64,
) (bool, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(probeID) == "" {
		return false, fmt.Errorf("probe command identity is required")
	}
	if _, err := uuid.Parse(operationID); err != nil || commandRevision <= 0 {
		return false, fmt.Errorf("valid operation_id and command revision are required")
	}
	result, err := deleteCommandIfRevisionScript.Run(
		ctx,
		store.client,
		[]string{commandKey(tenantID, probeID)},
		operationID,
		commandRevision,
	).Int64()
	if err != nil {
		return false, err
	}
	if result < 0 {
		return false, fmt.Errorf("legacy probe command cannot be revision-CAS deleted")
	}
	return result == 1, nil
}

func validateRoutedCommand(routed RoutedCommand) error {
	if err := validateCommand(routed.Command); err != nil {
		return err
	}
	if routed.Source.Topic != "probe.control.v2" || routed.Source.Partition < 0 ||
		routed.Source.Offset < 0 || strings.TrimSpace(routed.Source.Key) == "" {
		return fmt.Errorf("routed probe command source receipt is incomplete")
	}
	if len(routed.Source.HeadersSHA256) != 64 || len(routed.PayloadSHA256) != 64 {
		return fmt.Errorf("routed probe command source digests are invalid")
	}
	return nil
}

func validateCachedCommandRecord(record cachedCommandRecord) error {
	if record.CacheGeneration <= 0 {
		return fmt.Errorf("routed probe command cache generation is invalid")
	}
	return validateRoutedCommand(RoutedCommand{
		Command:       record.Command,
		Source:        record.Source,
		PayloadSHA256: record.PayloadSHA256,
	})
}

func decodeCachedCommandRecord(raw []byte) (cachedCommandRecord, error) {
	var record cachedCommandRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return cachedCommandRecord{}, fmt.Errorf("decode routed probe command record: %w", err)
	}
	if err := validateCachedCommandRecord(record); err != nil {
		return cachedCommandRecord{}, err
	}
	return record, nil
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

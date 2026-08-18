package control

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/redis/go-redis/v9"
	segmentkafka "github.com/segmentio/kafka-go"
)

type memoryCommandStore struct {
	commands []*pb.ProbeOperationCommand
	put      *pb.ProbeOperationCommand
	routed   *RoutedCommand
	deleted  []revisionDelete
	err      error
}

type revisionDelete struct {
	operationID string
	revision    int64
}

func (store *memoryCommandStore) PutIfNewer(
	_ context.Context,
	routed RoutedCommand,
) (DeliveryCacheReceipt, error) {
	if store.err != nil {
		return DeliveryCacheReceipt{}, store.err
	}
	store.put = routed.Command
	store.routed = &routed
	return DeliveryCacheReceipt{
		EventID:         routed.Command.EventId,
		OperationID:     routed.Command.OperationId,
		CommandRevision: routed.Command.CommandRevision,
		Source:          routed.Source,
		CacheGeneration: 1,
		Status:          PutCommandInserted,
	}, nil
}

func (store *memoryCommandStore) Get(
	_ context.Context,
	_ string,
	_ string,
	operationID string,
) (*pb.ProbeOperationCommand, error) {
	if store.err != nil {
		return nil, store.err
	}
	for _, command := range store.commands {
		if command.OperationId == operationID {
			return command, nil
		}
	}
	if store.put != nil && store.put.OperationId == operationID {
		return store.put, nil
	}
	return nil, ErrProbeCommandNotFound
}

func (store *memoryCommandStore) List(
	context.Context,
	string,
	string,
	int,
) ([]*pb.ProbeOperationCommand, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.commands, nil
}

func (store *memoryCommandStore) DeleteIfRevision(
	_ context.Context,
	_ string,
	_ string,
	operationID string,
	revision int64,
) (bool, error) {
	if store.err != nil {
		return false, store.err
	}
	command, err := store.Get(context.Background(), "", "", operationID)
	if err != nil {
		return false, nil
	}
	if command.CommandRevision != revision {
		return false, nil
	}
	store.deleted = append(store.deleted, revisionDelete{operationID: operationID, revision: revision})
	remaining := store.commands[:0]
	for _, item := range store.commands {
		if item.OperationId != operationID {
			remaining = append(remaining, item)
		}
	}
	store.commands = remaining
	return true, nil
}

type publishedAck struct {
	key     string
	payload []byte
	headers []commonkafka.MessageHeader
}

type memoryAckPublisher struct {
	events []publishedAck
	err    error
}

func (publisher *memoryAckPublisher) Publish(
	_ context.Context,
	key string,
	payload []byte,
	headers ...commonkafka.MessageHeader,
) error {
	if publisher.err != nil {
		return publisher.err
	}
	publisher.events = append(publisher.events, publishedAck{
		key: key, payload: payload, headers: headers,
	})
	return nil
}

func probeControlMessageFixture(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{
		"event_id":         "11111111-1111-4111-8111-111111111111",
		"event_type":       RequestedEventType,
		"schema_version":   2,
		"tenant_id":        "tenant-a",
		"probe_id":         "probe-a",
		"operation_id":     "22222222-2222-4222-8222-222222222222",
		"operation_type":   "connectivity_test",
		"command_revision": 3,
		"desired_version":  "",
		"command_hash":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"expires_at":       time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano),
		"trace_id":         "trace-a",
		"command":          map[string]interface{}{"targets": []string{"ingest-gateway"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic:     "probe.control.v2",
		Partition: 3,
		Offset:    42,
		Key:       []byte("tenant-a:probe-a"),
		Value:     payload,
		Headers: []segmentkafka.Header{
			{Key: "event_id", Value: []byte("11111111-1111-4111-8111-111111111111")},
			{Key: "event_type", Value: []byte(RequestedEventType)},
			{Key: "tenant_id", Value: []byte("tenant-a")},
			{Key: "probe_id", Value: []byte("probe-a")},
			{Key: "operation_id", Value: []byte("22222222-2222-4222-8222-222222222222")},
			{Key: "command_revision", Value: []byte("3")},
			{Key: "aggregate_version", Value: []byte("3")},
			{Key: "schema_version", Value: []byte("2")},
			{Key: "target_topic", Value: []byte("probe.control.v2")},
		},
	}}
}

func TestRouterPersistsOnlyValidatedV2Command(t *testing.T) {
	store := &memoryCommandStore{}
	router, err := NewRouter(store)
	if err != nil {
		t.Fatal(err)
	}
	message := probeControlMessageFixture(t)

	receipt, err := router.Route(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	if store.put == nil || store.put.ProbeId != "probe-a" ||
		store.put.CommandRevision != 3 || len(store.put.CommandJson) == 0 {
		t.Fatalf("unexpected routed command: %#v", store.put)
	}
	if store.routed == nil || store.routed.Source.Partition != 3 ||
		store.routed.Source.Offset != 42 || store.routed.Source.Key != "tenant-a:probe-a" ||
		len(store.routed.Source.HeadersSHA256) != sha256.Size*2 ||
		receipt.Source.Offset != 42 || receipt.CacheGeneration != 1 {
		t.Fatalf("source receipt was not retained: routed=%#v receipt=%#v", store.routed, receipt)
	}
}

func TestRouteMessageEnvelopeMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*commonkafka.ReceivedMessage)
	}{
		{name: "nil message", mutate: func(message *commonkafka.ReceivedMessage) { *message = commonkafka.ReceivedMessage{} }},
		{name: "wrong topic", mutate: func(message *commonkafka.ReceivedMessage) { message.Topic = "other.topic" }},
		{name: "negative partition", mutate: func(message *commonkafka.ReceivedMessage) { message.Partition = -1 }},
		{name: "negative offset", mutate: func(message *commonkafka.ReceivedMessage) { message.Offset = -1 }},
		{name: "wrong key", mutate: func(message *commonkafka.ReceivedMessage) { message.Key = []byte("tenant-a:probe-b") }},
		{name: "wrong tenant header", mutate: func(message *commonkafka.ReceivedMessage) { setProbeHeader(message, "tenant_id", "tenant-b") }},
		{name: "wrong revision header", mutate: func(message *commonkafka.ReceivedMessage) { setProbeHeader(message, "command_revision", "4") }},
		{name: "duplicate header", mutate: func(message *commonkafka.ReceivedMessage) {
			message.Headers = append(message.Headers, segmentkafka.Header{Key: "event_id", Value: []byte("duplicate")})
		}},
		{name: "trailing JSON", mutate: func(message *commonkafka.ReceivedMessage) {
			message.Value = append(message.Value, []byte(`{"extra":true}`)...)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryCommandStore{}
			router, err := NewRouter(store)
			if err != nil {
				t.Fatal(err)
			}
			message := probeControlMessageFixture(t)
			test.mutate(message)
			if _, err := router.RouteMessage(context.Background(), message); err == nil {
				t.Fatal("RouteMessage() expected envelope error")
			}
			if store.put != nil || store.routed != nil {
				t.Fatalf("invalid envelope mutated cache: put=%#v routed=%#v", store.put, store.routed)
			}
		})
	}
}

func setProbeHeader(message *commonkafka.ReceivedMessage, key, value string) {
	for index := range message.Headers {
		if message.Headers[index].Key == key {
			message.Headers[index].Value = []byte(value)
			return
		}
	}
	panic(fmt.Sprintf("fixture header %s not found", key))
}

func TestBridgePublishesStableAckBeforeDeletingAndConfirming(t *testing.T) {
	operationID := "22222222-2222-4222-8222-222222222222"
	store := &memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		OperationId: operationID, CommandRevision: 3,
	}, {
		OperationId: "33333333-3333-4333-8333-333333333333", CommandRevision: 4,
	}}}
	publisher := &memoryAckPublisher{}
	bridge, err := NewBridge(store, publisher)
	if err != nil {
		t.Fatal(err)
	}
	ack := &pb.ProbeOperationAck{
		OperationId:      operationID,
		CommandRevision:  3,
		ReportedHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AgentVersion:     "0.1.0",
		Applied:          false,
		AcknowledgedAtMs: time.Now().UnixMilli(),
		DetailJson:       []byte(`{"reason":"unsupported"}`),
	}

	commands, accepted, err := bridge.Exchange(
		context.Background(), "tenant-a", "probe-a", []*pb.ProbeOperationAck{ack},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(publisher.events) != 1 || len(store.deleted) != 1 ||
		store.deleted[0].operationID != operationID || store.deleted[0].revision != 3 ||
		len(accepted) != 1 || len(commands) != 1 {
		t.Fatalf("unexpected exchange: events=%d deleted=%v accepted=%v commands=%d",
			len(publisher.events), store.deleted, accepted, len(commands))
	}
	firstEventID := eventHeader(publisher.events[0].headers, "event_id")
	secondPublisher := &memoryAckPublisher{}
	secondBridge, _ := NewBridge(&memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		OperationId: operationID, CommandRevision: 3,
	}}}, secondPublisher)
	if _, _, err := secondBridge.Exchange(
		context.Background(), "tenant-a", "probe-a", []*pb.ProbeOperationAck{ack},
	); err != nil {
		t.Fatal(err)
	}
	if firstEventID == "" || firstEventID != eventHeader(secondPublisher.events[0].headers, "event_id") {
		t.Fatal("ACK event id is not stable across Agent retries")
	}
}

func TestBridgeDoesNotDeleteOrConfirmWhenPublishFails(t *testing.T) {
	operationID := "22222222-2222-4222-8222-222222222222"
	store := &memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		OperationId: operationID, CommandRevision: 3,
	}}}
	publisher := &memoryAckPublisher{err: errors.New("Kafka unavailable")}
	bridge, _ := NewBridge(store, publisher)
	ack := &pb.ProbeOperationAck{
		OperationId:      operationID,
		CommandRevision:  3,
		ReportedHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AgentVersion:     "0.1.0",
		AcknowledgedAtMs: time.Now().UnixMilli(),
	}

	_, accepted, err := bridge.Exchange(
		context.Background(), "tenant-a", "probe-a", []*pb.ProbeOperationAck{ack},
	)
	if err == nil || len(accepted) != 0 || len(store.deleted) != 0 {
		t.Fatalf("failure was acknowledged: err=%v accepted=%v deleted=%v", err, accepted, store.deleted)
	}
}

func TestBridgeRejectsWrongRevisionBeforePublishing(t *testing.T) {
	operationID := "22222222-2222-4222-8222-222222222222"
	store := &memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		OperationId: operationID, CommandRevision: 4,
	}}}
	publisher := &memoryAckPublisher{}
	bridge, err := NewBridge(store, publisher)
	if err != nil {
		t.Fatal(err)
	}
	_, accepted, err := bridge.Exchange(context.Background(), "tenant-a", "probe-a", []*pb.ProbeOperationAck{{
		OperationId: operationID, CommandRevision: 3,
		ReportedHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AgentVersion: "0.1.0", AcknowledgedAtMs: time.Now().UnixMilli(),
	}})
	if !errors.Is(err, ErrProbeCommandRevisionMismatch) || len(accepted) != 0 ||
		len(publisher.events) != 0 || len(store.deleted) != 0 {
		t.Fatalf("wrong revision crossed authority boundary: err=%v accepted=%v events=%d deleted=%v", err, accepted, len(publisher.events), store.deleted)
	}
}

func TestRedisCommandStoreRevisionMatrix(t *testing.T) {
	address := os.Getenv("PROBE_CONTROL_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("PROBE_CONTROL_TEST_REDIS_ADDR is required for the real Redis revision matrix")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping test Redis: %v", err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush isolated test Redis: %v", err)
	}

	store, err := NewRedisCommandStore(client, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	base := routedCommandFixture(t, 3, "22222222-2222-4222-8222-222222222222", "11111111-1111-4111-8111-111111111111", time.Now().Add(10*time.Minute))
	inserted, err := store.PutIfNewer(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if inserted.Status != PutCommandInserted || inserted.CacheGeneration != 1 || inserted.Source.Offset != 42 {
		t.Fatalf("unexpected inserted receipt: %#v", inserted)
	}
	replay, err := store.PutIfNewer(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Status != PutCommandReplay || replay.CacheGeneration != inserted.CacheGeneration ||
		replay.Source.Topic != inserted.Source.Topic || replay.Source.Partition != inserted.Source.Partition ||
		replay.Source.Offset != inserted.Source.Offset || replay.Source.Key != inserted.Source.Key ||
		replay.Source.HeadersSHA256 != inserted.Source.HeadersSHA256 {
		t.Fatalf("unexpected replay receipt: %#v", replay)
	}

	collision := base
	collision.PayloadSHA256 = stringsOf("b", sha256.Size*2)
	if _, err := store.PutIfNewer(ctx, collision); !errors.Is(err, ErrProbeCommandCollision) {
		t.Fatalf("same revision collision error = %v", err)
	}
	stale := routedCommandFixture(t, 2, "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", time.Now().Add(10*time.Minute))
	if _, err := store.PutIfNewer(ctx, stale); !errors.Is(err, ErrProbeCommandStaleRevision) {
		t.Fatalf("stale revision error = %v", err)
	}
	newer := routedCommandFixture(t, 4, "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", time.Now().Add(10*time.Minute))
	newerReceipt, err := store.PutIfNewer(ctx, newer)
	if err != nil {
		t.Fatal(err)
	}
	if newerReceipt.Status != PutCommandInserted || newerReceipt.CacheGeneration != 2 {
		t.Fatalf("unexpected newer receipt: %#v", newerReceipt)
	}

	if deleted, err := store.DeleteIfRevision(ctx, "tenant-a", "probe-a", newer.Command.OperationId, 3); err != nil || deleted {
		t.Fatalf("wrong revision delete = %v, err=%v", deleted, err)
	}
	if deleted, err := store.DeleteIfRevision(ctx, "tenant-a", "probe-a", newer.Command.OperationId, 4); err != nil || !deleted {
		t.Fatalf("exact revision delete = %v, err=%v", deleted, err)
	}

	expired := routedCommandFixture(t, 5, "77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888", time.Now().Add(time.Second))
	if _, err := store.PutIfNewer(ctx, expired); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	commands, err := store.List(ctx, "tenant-a", "probe-a", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].CommandRevision != 3 {
		t.Fatalf("List() returned expired or wrong commands: %#v", commands)
	}
	if _, err := store.Get(ctx, "tenant-a", "probe-a", expired.Command.OperationId); !errors.Is(err, ErrProbeCommandNotFound) {
		t.Fatalf("Get(expired) error = %v", err)
	}
}

func routedCommandFixture(
	t *testing.T,
	revision int64,
	operationID string,
	eventID string,
	expiresAt time.Time,
) RoutedCommand {
	t.Helper()
	command := &pb.ProbeOperationCommand{
		EventId: eventID, TenantId: "tenant-a", ProbeId: "probe-a",
		OperationId: operationID, OperationType: "connectivity_test",
		CommandRevision: revision, CommandHash: stringsOf("a", sha256.Size*2),
		ExpiresAtMs: expiresAt.UnixMilli(), CommandJson: []byte(`{"targets":["gateway"]}`),
	}
	headers := map[string]string{
		"event_id": eventID, "command_revision": strconv.FormatInt(revision, 10),
	}
	return RoutedCommand{
		Command: command,
		Source: KafkaSource{
			Topic: "probe.control.v2", Partition: 1, Offset: 42,
			Key: "tenant-a:probe-a", Headers: headers, HeadersSHA256: stringsOf("c", sha256.Size*2),
		},
		PayloadSHA256: stringsOf("d", sha256.Size*2),
	}
}

func stringsOf(value string, count int) string {
	result := ""
	for len(result) < count {
		result += value
	}
	return result[:count]
}

func eventHeader(headers []commonkafka.MessageHeader, key string) string {
	for _, header := range headers {
		if header.Key == key {
			return header.Value
		}
	}
	return ""
}

func TestBridgePublishesStageReceiptForReplayAck(t *testing.T) {
	store := &memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		EventId:         "11111111-1111-4111-8111-111111111111",
		TenantId:        "tenant-a",
		ProbeId:         "probe-a",
		OperationId:     "22222222-2222-4222-8222-222222222222",
		OperationType:   "pcap_replay",
		CommandRevision: 3,
	}}}
	bridge, err := NewBridge(store, &memoryAckPublisher{})
	if err != nil {
		t.Fatal(err)
	}
	receipts := &memoryAckPublisher{}
	bridge.SetStageReceiptPublisher(receipts)

	ack := &pb.ProbeOperationAck{
		OperationId:      "22222222-2222-4222-8222-222222222222",
		CommandRevision:  3,
		ReportedVersion:  "spec-1",
		ReportedHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AgentVersion:     "1.0.0",
		Applied:          true,
		AcknowledgedAtMs: 1700000000000,
		DetailJson:       []byte(`{"packets":15000,"bytes_consumed":7446364,"watermark_ms":1700005000000,"run_id":"run-1","fencing_token":"fence-1","execution_spec_sha256":"spec-1"}`),
	}
	_, _, err = bridge.Exchange(context.Background(), "tenant-a", "probe-a", []*pb.ProbeOperationAck{ack})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if len(receipts.events) != 1 {
		t.Fatalf("expected 1 stage receipt, got %d", len(receipts.events))
	}
	var receipt map[string]interface{}
	if err := json.Unmarshal(receipts.events[0].payload, &receipt); err != nil {
		t.Fatalf("receipt json: %v", err)
	}
	if receipt["run_id"] != "run-1" || receipt["execution_node_id"] != "SOURCE_ACTIVATE" ||
		receipt["fencing_token"] != "fence-1" || receipt["provider"] != "probe-agent" ||
		receipt["input_count"] != float64(15000) {
		t.Fatalf("receipt fields mismatch: %v", receipt)
	}
}

func TestBridgeReplayAckWithoutReceiptPublisherFailsClosed(t *testing.T) {
	store := &memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		EventId:         "11111111-1111-4111-8111-111111111111",
		TenantId:        "tenant-a",
		ProbeId:         "probe-a",
		OperationId:     "22222222-2222-4222-8222-222222222222",
		OperationType:   "pcap_replay",
		CommandRevision: 3,
	}}}
	bridge, err := NewBridge(store, &memoryAckPublisher{})
	if err != nil {
		t.Fatal(err)
	}
	// 未注入回执发布器 → fail-closed,ACK 不确认
	ack := &pb.ProbeOperationAck{
		OperationId:     "22222222-2222-4222-8222-222222222222",
		CommandRevision: 3,
		ReportedHash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AgentVersion:    "1.0.0",
		Applied:         true,
		DetailJson:      []byte(`{"run_id":"run-1","fencing_token":"fence-1"}`),
	}
	if _, _, err := bridge.Exchange(context.Background(), "tenant-a", "probe-a", []*pb.ProbeOperationAck{ack}); err == nil {
		t.Fatalf("expected fail-closed error without receipt publisher")
	}
}

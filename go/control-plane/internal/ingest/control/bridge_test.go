package control

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type memoryCommandStore struct {
	commands []*pb.ProbeOperationCommand
	put      *pb.ProbeOperationCommand
	deleted  []string
	err      error
}

func (store *memoryCommandStore) Put(_ context.Context, command *pb.ProbeOperationCommand) error {
	if store.err != nil {
		return store.err
	}
	store.put = command
	return nil
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

func (store *memoryCommandStore) Delete(
	_ context.Context,
	_ string,
	_ string,
	operationIDs []string,
) error {
	if store.err != nil {
		return store.err
	}
	store.deleted = append([]string(nil), operationIDs...)
	return nil
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

func TestRouterPersistsOnlyValidatedV2Command(t *testing.T) {
	store := &memoryCommandStore{}
	router, err := NewRouter(store)
	if err != nil {
		t.Fatal(err)
	}
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

	if err := router.Route(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if store.put == nil || store.put.ProbeId != "probe-a" ||
		store.put.CommandRevision != 3 || len(store.put.CommandJson) == 0 {
		t.Fatalf("unexpected routed command: %#v", store.put)
	}
}

func TestBridgePublishesStableAckBeforeDeletingAndConfirming(t *testing.T) {
	operationID := "22222222-2222-4222-8222-222222222222"
	store := &memoryCommandStore{commands: []*pb.ProbeOperationCommand{{
		OperationId: "33333333-3333-4333-8333-333333333333",
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
		store.deleted[0] != operationID || len(accepted) != 1 ||
		len(commands) != 1 {
		t.Fatalf("unexpected exchange: events=%d deleted=%v accepted=%v commands=%d",
			len(publisher.events), store.deleted, accepted, len(commands))
	}
	firstEventID := eventHeader(publisher.events[0].headers, "event_id")
	secondPublisher := &memoryAckPublisher{}
	secondBridge, _ := NewBridge(&memoryCommandStore{}, secondPublisher)
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
	store := &memoryCommandStore{}
	publisher := &memoryAckPublisher{err: errors.New("Kafka unavailable")}
	bridge, _ := NewBridge(store, publisher)
	ack := &pb.ProbeOperationAck{
		OperationId:      "22222222-2222-4222-8222-222222222222",
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

func eventHeader(headers []commonkafka.MessageHeader, key string) string {
	for _, header := range headers {
		if header.Key == key {
			return header.Value
		}
	}
	return ""
}

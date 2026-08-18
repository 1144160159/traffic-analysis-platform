package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type bindingRecorderStub struct {
	accepted int32
	rejected int32
	err      error
	bindings []*config.MacIpBinding
	proof    service.BindingProvenance
}

func (r *bindingRecorderStub) RecordMacIpBinding(
	_ context.Context,
	bindings []*config.MacIpBinding,
	proof service.BindingProvenance,
) (int32, int32, error) {
	r.bindings = bindings
	r.proof = proof
	return r.accepted, r.rejected, r.err
}

func TestBindingConsumerAcceptsStrictAuthenticatedEnvelope(t *testing.T) {
	recorder := &bindingRecorderStub{accepted: 1}
	consumer := &BindingConsumer{
		recorder: recorder, topic: "asset.bindings.v1",
		consumerGroup: "asset-service-bindings-shadow-test", maxBytes: 1 << 20,
	}
	message := validBindingMessage(t)

	if err := consumer.handleMessage(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(recorder.bindings) != 1 || recorder.bindings[0].ObservationID != "obs-1" ||
		recorder.bindings[0].ProbeID != "probe-a" {
		t.Fatalf("unexpected binding: %+v", recorder.bindings)
	}
	if recorder.proof.Topic != "asset.bindings.v1" || recorder.proof.Offset != 41 ||
		recorder.proof.Actor != "probe:probe-a" {
		t.Fatalf("unexpected provenance: %+v", recorder.proof)
	}
}

func TestBindingConsumerRejectsTenantAndDuplicateHeaderForgery(t *testing.T) {
	message := validBindingMessage(t)
	message.Headers[0].Value = []byte("tenant-b")
	if _, err := decodeAndValidateBinding(message, "asset.bindings.v1", 1<<20); err == nil {
		t.Fatal("expected tenant header mismatch")
	}

	message = validBindingMessage(t)
	message.Headers = append(message.Headers, kafka.Header{Key: "tenant_id", Value: []byte("tenant-a")})
	if _, err := decodeAndValidateBinding(message, "asset.bindings.v1", 1<<20); err == nil {
		t.Fatal("expected duplicate header rejection")
	}
}

func TestBindingConsumerClassifiesMalformedAndAuthorityRejectionPermanent(t *testing.T) {
	consumer := &BindingConsumer{
		recorder: &bindingRecorderStub{accepted: 1}, topic: "asset.bindings.v1", maxBytes: 1 << 20,
	}
	malformed := validBindingMessage(t)
	malformed.Value = []byte{0xff, 0x01}
	if err := consumer.handleMessage(context.Background(), malformed); !kafkaCommon.IsPermanent(err) {
		t.Fatalf("malformed error must be permanent: %v", err)
	}

	consumer.recorder = &bindingRecorderStub{rejected: 1}
	if err := consumer.handleMessage(context.Background(), validBindingMessage(t)); !kafkaCommon.IsPermanent(err) {
		t.Fatalf("authority rejection must be permanent: %v", err)
	}
}

func TestBindingConsumerLeavesTransientAuthorityFailureReplayable(t *testing.T) {
	consumer := &BindingConsumer{
		recorder: &bindingRecorderStub{err: errors.New("postgres unavailable")},
		topic:    "asset.bindings.v1", maxBytes: 1 << 20,
	}
	if err := consumer.handleMessage(context.Background(), validBindingMessage(t)); err == nil || kafkaCommon.IsPermanent(err) {
		t.Fatalf("authority dependency failure must remain retryable: %v", err)
	}
}

// TestBindingConsumerRealKafkaDurableAuthority is guarded by two explicit
// endpoints and a database sentinel. The alignment runner creates both
// dependencies, verifies their identities, and destroys them after this test.
func TestBindingConsumerRealKafkaDurableAuthority(t *testing.T) {
	dsn := os.Getenv("ASSET_BINDING_EPHEMERAL_PG_DSN")
	broker := os.Getenv("ASSET_BINDING_EPHEMERAL_KAFKA_BROKER")
	if dsn == "" || broker == "" {
		t.Skip("explicit ephemeral PostgreSQL and Kafka endpoints are required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_binding_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	if os.Getenv("ASSET_BINDING_EPHEMERAL_KAFKA_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing Kafka without the runner-owned sentinel")
	}

	const tenantID = "asset-binding-real-kafka"
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES ($1,$2) ON CONFLICT (tenant_id) DO NOTHING`, tenantID, "Asset Binding Real Kafka"); err != nil {
		t.Fatal(err)
	}
	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	assetService := service.New(&config.Config{}, repo, zap.NewNop())
	groupID := "asset-service-bindings-real-" + uuid.NewString()
	barrier, err := kafkaCommon.NewPostgresDLQAcknowledgementBarrier(db, groupID)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := NewBindingConsumer(config.KafkaConfig{
		Brokers: broker, Topic: "asset.bindings.v1", GroupID: groupID,
		MinBytes: 1, MaxBytes: 1 << 20, BindingDLQTopic: "dlq.v1", BindingMaxAttempts: 1,
	}, assetService, barrier, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	committed := make(chan kafka.Message, 8)
	owned.consumer.SetCommitObserver(func(messages []kafka.Message) {
		for _, message := range messages {
			committed <- message
		}
	})
	consumeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go owned.Run(consumeCtx)
	defer owned.Close()

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{broker}, Balancer: &kafka.Hash{},
		RequiredAcks: int(kafka.RequireAll), MaxAttempts: 3,
	})
	defer writer.Close()
	observedAt := time.Now().UTC().Add(-time.Second).UnixMilli()
	valid := bindingKafkaMessage(t, tenantID, "probe-real", "obs-real-1", observedAt)
	if err := writer.WriteMessages(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	t.Log("M06_BINDING_ORACLE PASS KAFKA_REQUIRED_ACKS_ALL")
	first := waitForBindingCommit(t, committed, "obs-real-1", 15*time.Second)
	assertBindingAuthorityCounts(t, db, tenantID, 1, 1, 1, 1)
	t.Log("M06_BINDING_ORACLE PASS DURABLE_AUTHORITY_COMMIT_BEFORE_OFFSET")

	if err := writer.WriteMessages(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	second := waitForBindingCommit(t, committed, "obs-real-1", 15*time.Second)
	if second.Offset <= first.Offset {
		t.Fatalf("replay offset=%d must follow first=%d", second.Offset, first.Offset)
	}
	assertBindingAuthorityCounts(t, db, tenantID, 1, 1, 1, 1)
	t.Log("M06_BINDING_ORACLE PASS EXACT_REPLAY_IDEMPOTENT")

	dlqReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker}, Topic: "dlq.v1", Partition: 0,
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 100 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer dlqReader.Close()
	poison := bindingKafkaMessage(t, tenantID, "probe-real", "obs-forged-tenant", observedAt+1)
	for index := range poison.Headers {
		if poison.Headers[index].Key == "tenant_id" {
			poison.Headers[index].Value = []byte("tenant-forged")
		}
	}
	if err := writer.WriteMessages(context.Background(), poison); err != nil {
		t.Fatal(err)
	}
	poisonCommit := waitForBindingCommit(t, committed, "obs-forged-tenant", 15*time.Second)
	var receiptCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM kafka_dlq_acknowledgement_receipts
		WHERE consumer_group=$1 AND source_topic='asset.bindings.v1'
		  AND source_partition=$2 AND source_offset=$3`,
		groupID, poisonCommit.Partition, poisonCommit.Offset,
	).Scan(&receiptCount); err != nil || receiptCount != 1 {
		t.Fatalf("DLQ acknowledgement receipt count=%d err=%v", receiptCount, err)
	}
	dlqCtx, stopDLQ := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopDLQ()
	dlqRecord, err := dlqReader.ReadMessage(dlqCtx)
	if err != nil {
		t.Fatal(err)
	}
	var dlq kafkaCommon.DLQMessage
	if err := json.Unmarshal(dlqRecord.Value, &dlq); err != nil {
		t.Fatal(err)
	}
	if dlq.OriginalTopic != "asset.bindings.v1" || dlq.OriginalOffset != poisonCommit.Offset || dlq.EventID != "obs-forged-tenant" {
		t.Fatalf("unexpected DLQ record: %+v", dlq)
	}
	assertBindingAuthorityCounts(t, db, tenantID, 1, 1, 1, 1)
	t.Log("M06_BINDING_ORACLE PASS DLQ_ACK_BEFORE_SOURCE_COMMIT")
	t.Log("M06_BINDING_ORACLE PASS TENANT_FORGERY_QUARANTINED")
}

func bindingKafkaMessage(
	t *testing.T,
	tenantID string,
	probeID string,
	observationID string,
	observedAt int64,
) kafka.Message {
	t.Helper()
	binding := &pb.MacIpBinding{
		MacAddress: "02:11:22:33:44:55", IpAddress: "192.0.2.18",
		TenantId: tenantID, ObservedAt: observedAt, Source: "arp",
		ObservationId: observationID, ProbeId: probeID, SchemaVersion: 1,
		SourceEventId: "packet-" + observationID,
	}
	payload, err := proto.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	return kafka.Message{
		Topic: "asset.bindings.v1", Time: time.UnixMilli(observedAt + 100),
		Key: []byte(tenantID + ":" + binding.MacAddress), Value: payload,
		Headers: []kafka.Header{
			{Key: "tenant_id", Value: []byte(tenantID)},
			{Key: "probe_id", Value: []byte(probeID)},
			{Key: "observation_id", Value: []byte(observationID)},
			{Key: "event_id", Value: []byte(observationID)},
			{Key: "source", Value: []byte("arp")},
			{Key: "schema_version", Value: []byte("1")},
			{Key: "content_type", Value: []byte("application/x-protobuf")},
			{Key: "message_type", Value: []byte("traffic.v1.MacIpBinding")},
		},
	}
}

func waitForBindingCommit(t *testing.T, commits <-chan kafka.Message, observationID string, timeout time.Duration) kafka.Message {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case message := <-commits:
			for _, header := range message.Headers {
				if header.Key == "event_id" && string(header.Value) == observationID {
					return message
				}
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for binding commit %s", observationID)
		}
	}
}

func assertBindingAuthorityCounts(
	t *testing.T,
	db *sql.DB,
	tenantID string,
	assets int,
	history int,
	outbox int,
	requests int,
) {
	t.Helper()
	queries := []struct {
		name string
		sql  string
		want int
	}{
		{"assets", `SELECT count(*) FROM assets WHERE tenant_id=$1`, assets},
		{"history", `SELECT count(*) FROM asset_events WHERE tenant_id=$1`, history},
		{"outbox", `SELECT count(*) FROM asset_event_outbox WHERE tenant_id=$1`, outbox},
		{"requests", `SELECT count(*) FROM asset_upsert_requests WHERE tenant_id=$1`, requests},
	}
	for _, query := range queries {
		var got int
		if err := db.QueryRow(query.sql, tenantID).Scan(&got); err != nil || got != query.want {
			t.Fatalf("%s count=%d want=%d err=%v", query.name, got, query.want, err)
		}
	}
}

func validBindingMessage(t *testing.T) *kafkaCommon.ReceivedMessage {
	t.Helper()
	observedAt := int64(1_700_000_000_000)
	binding := &pb.MacIpBinding{
		MacAddress: "00:11:22:33:44:55", IpAddress: "192.0.2.8",
		TenantId: "tenant-a", ObservedAt: observedAt, Source: "arp",
		ObservationId: "obs-1", ProbeId: "probe-a", SchemaVersion: 1,
	}
	payload, err := proto.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	return &kafkaCommon.ReceivedMessage{Message: kafka.Message{
		Topic: "asset.bindings.v1", Partition: 2, Offset: 41,
		Time: time.UnixMilli(observedAt + 100),
		Key:  []byte("tenant-a:00:11:22:33:44:55"), Value: payload,
		Headers: []kafka.Header{
			{Key: "tenant_id", Value: []byte("tenant-a")},
			{Key: "probe_id", Value: []byte("probe-a")},
			{Key: "observation_id", Value: []byte("obs-1")},
			{Key: "event_id", Value: []byte("obs-1")},
			{Key: "source", Value: []byte("arp")},
			{Key: "schema_version", Value: []byte("1")},
			{Key: "content_type", Value: []byte("application/x-protobuf")},
			{Key: "message_type", Value: []byte("traffic.v1.MacIpBinding")},
		},
	}}
}

package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	redisv9 "github.com/redis/go-redis/v9"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	alertrepo "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/dataquality"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	alertProjectionKafkaTenant = "tenant-alert-projection-kafka-g1"
	alertProjectionKafkaEvent  = "event-alert-projection-kafka-g1"
	alertProjectionKafkaRetry  = "event-alert-projection-kafka-retry-g1"
)

// TestAlertProjectionReceiptRealKafka proves that the production alert
// consumer advances its real broker offset only after ClickHouse, OpenSearch,
// and the atomic PostgreSQL applied receipt have all succeeded.
func TestAlertProjectionReceiptRealKafka(t *testing.T) {
	settings := readAlertProjectionKafkaSettings(t)
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	logger := zap.NewNop()

	pgDB, err := sql.Open("postgres", settings.pgDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pgDB.Close()
	var marker string
	if err := pgDB.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_alert_projection_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing PostgreSQL without ephemeral sentinel: marker=%q err=%v", marker, err)
	}

	chClient, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{settings.clickHouseHost}, Database: "traffic",
		Username: settings.clickHouseUser, Password: settings.clickHousePassword,
		MaxOpenConns: 2, MaxIdleConns: 1, DialTimeout: 5 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer chClient.Close()
	row, err := chClient.QueryRow(ctx, `SELECT marker FROM traffic.codex_ephemeral_alert_reconcile_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := row.Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing ClickHouse without ephemeral sentinel: marker=%q err=%v", marker, err)
	}

	redisClient := redisv9.NewClient(&redisv9.Options{Addr: settings.redisAddr})
	defer redisClient.Close()
	if value, err := redisClient.Get(ctx, "codex:ephemeral:alert-consumer-sentinel").Result(); err != nil || value != "ephemeral-only" {
		t.Fatalf("refusing Redis without ephemeral sentinel: value=%q err=%v", value, err)
	}
	if err := verifyAlertProjectionOpenSearchSentinel(settings.openSearchURL); err != nil {
		t.Fatal(err)
	}

	chWriter, err := persistence.NewClickHouseWriter(chClient, logger)
	if err != nil {
		t.Fatal(err)
	}
	osWriter, err := persistence.NewOpenSearchWriter(
		[]string{settings.openSearchURL}, "", "", "alerts-v2-write", true, logger,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer osWriter.Close()
	dualWriter := persistence.NewDualWriter(chWriter, osWriter, 100, logger)
	receipts := persistence.NewProjectionDebtStore(pgDB)
	if err := receipts.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	dualWriter.SetProjectionDebtRecorder(receipts)

	groupID := "alert-projection-receipt-g1"
	topic := "detections.alert-projection-receipt.v1"
	alertConsumer := NewConsumer(
		alertconfig.KafkaConfig{Brokers: []string{settings.kafkaBroker}, Topic: topic, GroupID: groupID, BatchSize: 1},
		alertconfig.DedupConfig{TimeBucketMinutes: 10, TTL: 10 * time.Minute},
		dedup.NewRedisDedup(redisClient, 10*time.Minute, logger), dualWriter, logger,
	)
	if alertConsumer == nil {
		t.Fatal("production alert consumer could not be created")
	}
	consumeCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)
	firstConsumerClosed := false
	go func() { consumerDone <- alertConsumer.Start(consumeCtx) }()
	t.Cleanup(func() {
		if firstConsumerClosed {
			return
		}
		stopConsumer()
		_ = alertConsumer.Close()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
		}
	})

	writer := segmentkafka.NewWriter(segmentkafka.WriterConfig{
		Brokers: []string{settings.kafkaBroker}, Balancer: &segmentkafka.Hash{},
		RequiredAcks: int(segmentkafka.RequireAll), MaxAttempts: 3,
	})
	defer writer.Close()
	if err := writeAlertProjectionDetection(ctx, writer, topic, alertProjectionKafkaDetection()); err != nil {
		t.Fatal(err)
	}

	offsetReader, err := dataquality.NewKafkaBrokerOffsetReader(
		[]string{settings.kafkaBroker}, commonkafka.SecurityConfig{}, 5*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer offsetReader.Close()
	var lag dataquality.KafkaLagSnapshot
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		lag, err = offsetReader.ReadLag(ctx, topic, groupID)
		alertConsumer.commitMetricMu.Lock()
		lastEvent := alertConsumer.lastCommitEvent[topic+":0"]
		alertConsumer.commitMetricMu.Unlock()
		metrics := alertConsumer.kafkaConsumer.GetMetrics()
		if err == nil && lag.TotalLag == 0 && lag.TotalCommittedOffset == 1 &&
			metrics.CommitsSucceeded == 1 && metrics.LastOffset == 0 && lastEvent == alertProjectionKafkaEvent {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || lag.TotalLag != 0 || lag.TotalCommittedOffset != 1 {
		t.Fatalf("broker offset did not converge after projection receipts: lag=%+v err=%v", lag, err)
	}
	metrics := alertConsumer.kafkaConsumer.GetMetrics()
	alertConsumer.commitMetricMu.Lock()
	lastEvent := alertConsumer.lastCommitEvent[topic+":0"]
	alertConsumer.commitMetricMu.Unlock()
	if metrics.CommitsSucceeded != 1 || metrics.LastOffset != 0 || lastEvent != alertProjectionKafkaEvent {
		t.Fatalf("post-ACK observer mismatch: metrics=%+v last_event_id=%q", metrics, lastEvent)
	}

	authority := alertrepo.NewAlertRepository(chClient, logger)
	scope := persistence.ProjectionScope{TenantID: alertProjectionKafkaTenant, MaxDocuments: 10, TargetIndexVersion: "alerts-v2-write"}
	authoritative, truncated, err := authority.ListProjectionAlerts(ctx, scope)
	if err != nil || truncated || len(authoritative) != 1 {
		t.Fatalf("unexpected ClickHouse authority: count=%d truncated=%v err=%v", len(authoritative), truncated, err)
	}
	target, err := persistence.NewOpenSearchReconcileTarget(
		[]string{settings.openSearchURL}, "", "", "alerts-v2-read", "alerts-v2-write", true, false, logger,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	projected, targetTruncated, err := target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(projected) != 1 {
		t.Fatalf("unexpected OpenSearch projection: count=%d truncated=%v err=%v", len(projected), targetTruncated, err)
	}
	sourceHash, err := persistence.AlertProjectionSHA256(authoritative[0])
	if err != nil {
		t.Fatal(err)
	}
	targetHash, err := persistence.AlertProjectionSHA256(projected[0])
	if err != nil {
		t.Fatal(err)
	}
	var receiptEvent, receiptHash string
	var receiptVersion int64
	if err := pgDB.QueryRowContext(ctx, `SELECT source_event_id,source_version,source_sha256
		FROM alert_opensearch_projection_watermarks
		WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version='alerts-v2-write'`,
		alertProjectionKafkaTenant, authoritative[0].AlertID,
	).Scan(&receiptEvent, &receiptVersion, &receiptHash); err != nil {
		t.Fatal(err)
	}
	if authoritative[0].EventID != alertProjectionKafkaEvent || projected[0].EventID != alertProjectionKafkaEvent ||
		receiptEvent != alertProjectionKafkaEvent || sourceHash != targetHash || sourceHash != receiptHash ||
		receiptVersion != persistence.AlertSourceVersion(authoritative[0]) {
		t.Fatalf("Kafka/CH/OS/PG identity mismatch: source_event=%q target_event=%q receipt_event=%q hashes=%s/%s/%s versions=%d/%d",
			authoritative[0].EventID, projected[0].EventID, receiptEvent, sourceHash, targetHash, receiptHash,
			persistence.AlertSourceVersion(authoritative[0]), receiptVersion)
	}

	// Make the applied-receipt table unavailable after CH and OS are healthy.
	// The next event must remain uncommitted even though both projection writes
	// have already succeeded. Restoring the table and restarting the same group
	// must then redeliver and converge that exact event.
	faultTable := "alert_opensearch_projection_watermarks_fault_g1"
	restored := false
	restoreReceiptTable := func() {
		if restored {
			return
		}
		if _, restoreErr := pgDB.ExecContext(ctx, `ALTER TABLE `+faultTable+` RENAME TO alert_opensearch_projection_watermarks`); restoreErr != nil {
			t.Fatalf("restore PostgreSQL receipt table: %v", restoreErr)
		}
		restored = true
	}
	if _, err := pgDB.ExecContext(ctx, `ALTER TABLE alert_opensearch_projection_watermarks RENAME TO `+faultTable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restoreReceiptTable)
	if err := writeAlertProjectionDetection(ctx, writer, topic, alertProjectionKafkaRetryDetection()); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		lag, err = offsetReader.ReadLag(ctx, topic, groupID)
		metrics = alertConsumer.kafkaConsumer.GetMetrics()
		if err == nil && !alertConsumer.IsRunning() && metrics.MessagesFailed >= 1 &&
			metrics.CommitsSucceeded == 1 && lag.TotalCommittedOffset == 1 && lag.TotalLag == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	alertConsumer.commitMetricMu.Lock()
	failedLastEvent := alertConsumer.lastCommitEvent[topic+":0"]
	alertConsumer.commitMetricMu.Unlock()
	if err != nil || alertConsumer.IsRunning() || metrics.MessagesFailed < 1 || metrics.CommitsSucceeded != 1 ||
		lag.TotalCommittedOffset != 1 || lag.TotalLag != 1 || failedLastEvent != alertProjectionKafkaEvent {
		t.Fatalf("receipt outage advanced Kafka or observer state: lag=%+v metrics=%+v running=%v last_event=%q err=%v",
			lag, metrics, alertConsumer.IsRunning(), failedLastEvent, err)
	}
	restoreReceiptTable()
	var retryReceiptCount int
	if err := pgDB.QueryRowContext(ctx, `SELECT count(*) FROM alert_opensearch_projection_watermarks WHERE source_event_id=$1`, alertProjectionKafkaRetry).Scan(&retryReceiptCount); err != nil {
		t.Fatal(err)
	}
	if retryReceiptCount != 0 {
		t.Fatalf("failed receipt unexpectedly persisted before retry: count=%d", retryReceiptCount)
	}

	stopConsumer()
	if err := alertConsumer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-consumerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("failed alert consumer did not stop")
	}
	firstConsumerClosed = true

	retryConsumer := NewConsumer(
		alertconfig.KafkaConfig{Brokers: []string{settings.kafkaBroker}, Topic: topic, GroupID: groupID, BatchSize: 1},
		alertconfig.DedupConfig{TimeBucketMinutes: 10, TTL: 10 * time.Minute},
		dedup.NewRedisDedup(redisClient, 10*time.Minute, logger), dualWriter, logger,
	)
	if retryConsumer == nil {
		t.Fatal("restarted production alert consumer could not be created")
	}
	retryCtx, stopRetry := context.WithCancel(ctx)
	retryDone := make(chan error, 1)
	go func() { retryDone <- retryConsumer.Start(retryCtx) }()
	t.Cleanup(func() {
		stopRetry()
		_ = retryConsumer.Close()
		select {
		case <-retryDone:
		case <-time.After(2 * time.Second):
		}
	})
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		lag, err = offsetReader.ReadLag(ctx, topic, groupID)
		retryConsumer.commitMetricMu.Lock()
		retryLastEvent := retryConsumer.lastCommitEvent[topic+":0"]
		retryConsumer.commitMetricMu.Unlock()
		retryMetrics := retryConsumer.kafkaConsumer.GetMetrics()
		if err == nil && lag.TotalLag == 0 && lag.TotalCommittedOffset == 2 &&
			retryMetrics.CommitsSucceeded == 1 && retryMetrics.LastOffset == 1 && retryLastEvent == alertProjectionKafkaRetry {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	retryConsumer.commitMetricMu.Lock()
	retryLastEvent := retryConsumer.lastCommitEvent[topic+":0"]
	retryConsumer.commitMetricMu.Unlock()
	if err != nil || lag.TotalLag != 0 || lag.TotalCommittedOffset != 2 || retryLastEvent != alertProjectionKafkaRetry {
		t.Fatalf("same-group retry did not converge: lag=%+v last_event=%q err=%v", lag, retryLastEvent, err)
	}
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	authoritative, truncated, err = authority.ListProjectionAlerts(ctx, scope)
	if err != nil || truncated || len(authoritative) != 2 {
		t.Fatalf("unexpected ClickHouse authority after retry: count=%d truncated=%v err=%v", len(authoritative), truncated, err)
	}
	projected, targetTruncated, err = target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(projected) != 2 {
		t.Fatalf("unexpected OpenSearch projection after retry: count=%d truncated=%v err=%v", len(projected), targetTruncated, err)
	}
	var retrySource, retryTarget *persistence.Alert
	for _, alert := range authoritative {
		if alert.EventID == alertProjectionKafkaRetry {
			retrySource = alert
		}
	}
	for _, alert := range projected {
		if alert.EventID == alertProjectionKafkaRetry {
			retryTarget = alert
		}
	}
	if retrySource == nil || retryTarget == nil {
		t.Fatalf("retried event missing from stores: source=%v target=%v", retrySource, retryTarget)
	}
	retrySourceHash, err := persistence.AlertProjectionSHA256(retrySource)
	if err != nil {
		t.Fatal(err)
	}
	retryTargetHash, err := persistence.AlertProjectionSHA256(retryTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := pgDB.QueryRowContext(ctx, `SELECT source_event_id,source_version,source_sha256
		FROM alert_opensearch_projection_watermarks
		WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version='alerts-v2-write'`,
		alertProjectionKafkaTenant, retrySource.AlertID,
	).Scan(&receiptEvent, &receiptVersion, &receiptHash); err != nil {
		t.Fatal(err)
	}
	if receiptEvent != alertProjectionKafkaRetry || retrySourceHash != retryTargetHash || retrySourceHash != receiptHash ||
		receiptVersion != persistence.AlertSourceVersion(retrySource) {
		t.Fatalf("retried Kafka/CH/OS/PG identity mismatch: event=%q hashes=%s/%s/%s versions=%d/%d",
			receiptEvent, retrySourceHash, retryTargetHash, receiptHash, persistence.AlertSourceVersion(retrySource), receiptVersion)
	}
	t.Logf("PASS_ALERT_PROJECTION_REAL_KAFKA_RECEIPT event_id=%s committed_offset=1 lag_before_fault=0 source_sha256=%s", lastEvent, sourceHash)
	t.Logf("PASS_ALERT_PROJECTION_RECEIPT_FAILURE_RESTART retry_event_id=%s retained_offset=1 retained_lag=1 recovered_offset=%d recovered_lag=%d source_sha256=%s",
		retryLastEvent, lag.TotalCommittedOffset, lag.TotalLag, retrySourceHash)
}

func writeAlertProjectionDetection(ctx context.Context, writer *segmentkafka.Writer, topic string, detection *pb.DetectionBatch) error {
	payload, err := proto.Marshal(detection)
	if err != nil {
		return err
	}
	eventID := detection.GetBehaviors()[0].GetHeader().GetEventId()
	return writer.WriteMessages(ctx, segmentkafka.Message{
		Topic: topic, Key: []byte(alertProjectionKafkaTenant + ":" + eventID), Value: payload,
		Headers: []segmentkafka.Header{
			{Key: "tenant_id", Value: []byte(alertProjectionKafkaTenant)},
			{Key: "event_id", Value: []byte(eventID)},
			{Key: "trace_id", Value: []byte("0123456789abcdef0123456789abcdef")},
			{Key: "content_type", Value: []byte("application/x-protobuf")},
			{Key: "proto_message_type", Value: []byte("traffic.v1.DetectionBatch")},
		},
	})
}

type alertProjectionKafkaSettings struct {
	clickHouseHost, clickHouseUser, clickHousePassword string
	pgDSN, openSearchURL, redisAddr, kafkaBroker       string
}

func readAlertProjectionKafkaSettings(t *testing.T) alertProjectionKafkaSettings {
	t.Helper()
	settings := alertProjectionKafkaSettings{
		clickHouseHost:     strings.TrimSpace(os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_CH_HOST")),
		clickHouseUser:     os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_CH_USER"),
		clickHousePassword: os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_CH_PASSWORD"),
		pgDSN:              strings.TrimSpace(os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_PG_DSN")),
		openSearchURL:      strings.TrimRight(os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_OS_URL"), "/"),
		redisAddr:          strings.TrimSpace(os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_REDIS_ADDR")),
		kafkaBroker:        strings.TrimSpace(os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_BROKER")),
	}
	if settings.clickHouseHost == "" || settings.pgDSN == "" || settings.openSearchURL == "" || settings.redisAddr == "" || settings.kafkaBroker == "" {
		t.Skip("explicit five-store ephemeral endpoints are required")
	}
	if os.Getenv("ALERT_PROJECTION_KAFKA_EPHEMERAL_SENTINEL") != "ephemeral-only" {
		t.Fatal("explicit five-store ephemeral sentinel is required")
	}
	for name, address := range map[string]string{
		"ClickHouse": settings.clickHouseHost, "Redis": settings.redisAddr, "Kafka": settings.kafkaBroker,
	} {
		host, _, err := net.SplitHostPort(address)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			t.Fatalf("%s endpoint must use a numeric loopback address: %q", name, address)
		}
	}
	pgURL, err := url.Parse(settings.pgDSN)
	if err != nil {
		t.Fatal(err)
	}
	pgHost, _, pgErr := net.SplitHostPort(pgURL.Host)
	if pgErr != nil || net.ParseIP(pgHost) == nil || !net.ParseIP(pgHost).IsLoopback() || pgURL.Query().Get("sslmode") != "disable" {
		t.Fatalf("PostgreSQL endpoint must be loopback ephemeral: %q", settings.pgDSN)
	}
	osURL, err := url.Parse(settings.openSearchURL)
	if err != nil {
		t.Fatal(err)
	}
	osHost, _, osErr := net.SplitHostPort(osURL.Host)
	if osErr != nil || osURL.Scheme != "http" || net.ParseIP(osHost) == nil || !net.ParseIP(osHost).IsLoopback() {
		t.Fatalf("OpenSearch endpoint must be loopback HTTP: %q", settings.openSearchURL)
	}
	return settings
}

func verifyAlertProjectionOpenSearchSentinel(baseURL string) error {
	response, err := http.Get(baseURL + "/codex-ephemeral-alert-reconcile-sentinel/_doc/ephemeral-only")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var payload struct {
		Found  bool `json:"found"`
		Source struct {
			Marker string `json:"marker"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK || !payload.Found || payload.Source.Marker != "ephemeral-only" {
		return fmt.Errorf("refusing OpenSearch without ephemeral sentinel: status=%s payload=%+v", response.Status, payload)
	}
	return nil
}

func alertProjectionKafkaDetection() *pb.DetectionBatch {
	observed := time.Date(2026, 8, 7, 16, 30, 0, 0, time.UTC).UnixMilli()
	return &pb.DetectionBatch{
		BatchId: "batch-alert-projection-kafka-g1", TenantId: alertProjectionKafkaTenant,
		CreatedAt: observed,
		Behaviors: []*pb.DetectionBehavior{{
			Header: &pb.EventHeader{
				EventId: alertProjectionKafkaEvent, TenantId: alertProjectionKafkaTenant,
				EventTs: observed, IngestTs: observed + 1, KafkaTs: observed + 2, FlinkOutTs: observed + 3,
				EventType: "traffic.detection.behavior.v1", SchemaVersion: "1",
				AggregateType: "detection", AggregateId: "session-alert-projection-kafka-g1", AggregateVersion: 1,
				OccurredAt: observed, ProducedAt: observed + 3,
				TraceId: "0123456789abcdef0123456789abcdef", CausationId: "feature-alert-projection-kafka-g1",
				CorrelationId: "community-alert-projection-kafka-g1", IdempotencyKey: alertProjectionKafkaEvent,
				Producer: "flink-behavior-job", FeatureSetId: "feature-set-alert-projection-kafka-g1",
			},
			ModelVersion: "model-alert-projection-kafka-g1", CommunityId: "community-alert-projection-kafka-g1",
			ObjectType: "session", ObjectId: "session-alert-projection-kafka-g1", Ts: observed,
			Labels: []string{"scan", "asset_scope:ephemeral"}, TopLabel: "scan", TopScore: 0.97,
			Tuple:       &pb.FiveTuple{SrcIp: "192.0.2.107", DstIp: "198.51.100.107", SrcPort: 49107, DstPort: 443, Protocol: 6},
			EvidenceIds: []string{"evidence-alert-projection-kafka-g1"},
		}},
	}
}

func alertProjectionKafkaRetryDetection() *pb.DetectionBatch {
	detection := alertProjectionKafkaDetection()
	behavior := detection.Behaviors[0]
	behavior.Header.EventId = alertProjectionKafkaRetry
	behavior.Header.IdempotencyKey = alertProjectionKafkaRetry
	behavior.Header.AggregateId = "session-alert-projection-kafka-retry-g1"
	behavior.Header.EventTs += 1000
	behavior.Header.OccurredAt += 1000
	behavior.Header.ProducedAt += 1000
	behavior.Ts += 1000
	behavior.ObjectId = "session-alert-projection-kafka-retry-g1"
	behavior.Tuple.SrcPort++
	detection.BatchId = "batch-alert-projection-kafka-retry-g1"
	detection.CreatedAt += 1000
	return detection
}

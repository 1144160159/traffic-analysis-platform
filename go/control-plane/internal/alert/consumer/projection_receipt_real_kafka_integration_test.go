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
	"strconv"
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
	alertProjectionKafkaNewer  = "event-alert-projection-kafka-newer-g1"
	alertProjectionKafkaOlder  = "event-alert-projection-kafka-older-g1"
	alertProjectionKafkaP0     = "event-alert-projection-kafka-partition-0-g1"
	alertProjectionKafkaP1     = "event-alert-projection-kafka-partition-1-g1"
	alertProjectionKafkaMove   = "event-alert-projection-kafka-rebalance-takeover-g1"
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
	redisDedup := dedup.NewRedisDedup(redisClient, 10*time.Minute, logger)
	alertConsumer := NewConsumer(
		alertconfig.KafkaConfig{Brokers: []string{settings.kafkaBroker}, Topic: topic, GroupID: groupID, BatchSize: 1},
		alertconfig.DedupConfig{TimeBucketMinutes: 10, TTL: 10 * time.Minute},
		redisDedup, dualWriter, logger,
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
		_, observed := committedEventPartition(alertConsumer, topic, alertProjectionKafkaEvent)
		metrics := alertConsumer.kafkaConsumer.GetMetrics()
		if err == nil && lag.TotalLag == 0 && lag.TotalCommittedOffset == 1 &&
			metrics.CommitsSucceeded == 1 && metrics.LastOffset == 0 && observed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || lag.TotalLag != 0 || lag.TotalCommittedOffset != 1 {
		t.Fatalf("broker offset did not converge after projection receipts: lag=%+v err=%v", lag, err)
	}
	metrics := alertConsumer.kafkaConsumer.GetMetrics()
	_, observed := committedEventPartition(alertConsumer, topic, alertProjectionKafkaEvent)
	if metrics.CommitsSucceeded != 1 || metrics.LastOffset != 0 || !observed {
		t.Fatalf("post-ACK observer mismatch: metrics=%+v event_observed=%v", metrics, observed)
	}
	lastEvent := alertProjectionKafkaEvent

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
	_, originalObserved := committedEventPartition(alertConsumer, topic, alertProjectionKafkaEvent)
	_, failedEventObserved := committedEventPartition(alertConsumer, topic, alertProjectionKafkaRetry)
	if err != nil || alertConsumer.IsRunning() || metrics.MessagesFailed < 1 || metrics.CommitsSucceeded != 1 ||
		lag.TotalCommittedOffset != 1 || lag.TotalLag != 1 || !originalObserved || failedEventObserved {
		t.Fatalf("receipt outage advanced Kafka or observer state: lag=%+v metrics=%+v running=%v original_observed=%v failed_observed=%v err=%v",
			lag, metrics, alertConsumer.IsRunning(), originalObserved, failedEventObserved, err)
	}
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	failureAuthoritative, truncated, err := authority.ListProjectionAlerts(ctx, scope)
	if err != nil || truncated || len(failureAuthoritative) != 2 {
		t.Fatalf("unexpected ClickHouse authority during receipt failure: count=%d truncated=%v err=%v", len(failureAuthoritative), truncated, err)
	}
	failureProjected, targetTruncated, err := target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(failureProjected) != 2 {
		t.Fatalf("unexpected OpenSearch projection during receipt failure: count=%d truncated=%v err=%v", len(failureProjected), targetTruncated, err)
	}
	failureSourceHash := projectionHashForEvent(t, failureAuthoritative, alertProjectionKafkaRetry)
	failureTargetHash := projectionHashForEvent(t, failureProjected, alertProjectionKafkaRetry)
	if failureSourceHash != failureTargetHash {
		t.Fatalf("CH and OS diverged before receipt recovery: source=%s target=%s", failureSourceHash, failureTargetHash)
	}
	retryFingerprint := dedup.CalculateFingerprint(alertProjectionKafkaRetryDetection(), 10)
	redisCountKey := fmt.Sprintf("alert:dedup:%s:%s:count", alertProjectionKafkaTenant, retryFingerprint)
	countDuringFailure, err := redisClient.Get(ctx, redisCountKey).Int64()
	if err != nil || countDuringFailure != 2 {
		t.Fatalf("unexpected Redis aggregate count during receipt failure: count=%d err=%v", countDuringFailure, err)
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
	retryConsumerClosed := false
	go func() { retryDone <- retryConsumer.Start(retryCtx) }()
	t.Cleanup(func() {
		if retryConsumerClosed {
			return
		}
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
		_, retryObserved := committedEventPartition(retryConsumer, topic, alertProjectionKafkaRetry)
		retryMetrics := retryConsumer.kafkaConsumer.GetMetrics()
		if err == nil && lag.TotalLag == 0 && lag.TotalCommittedOffset == 2 &&
			retryMetrics.CommitsSucceeded == 1 && retryObserved {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, retryObserved := committedEventPartition(retryConsumer, topic, alertProjectionKafkaRetry)
	retryLastEvent := alertProjectionKafkaRetry
	if err != nil || lag.TotalLag != 0 || lag.TotalCommittedOffset != 2 || !retryObserved {
		t.Fatalf("same-group retry did not converge: lag=%+v event_observed=%v err=%v", lag, retryObserved, err)
	}
	recoveredOffset := lag.TotalCommittedOffset
	recoveredLag := lag.TotalLag
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
	countAfterRecovery, err := redisClient.Get(ctx, redisCountKey).Int64()
	if err != nil || countAfterRecovery != countDuringFailure {
		t.Fatalf("exact event replay inflated Redis aggregate count: before=%d after=%d err=%v", countDuringFailure, countAfterRecovery, err)
	}
	if retrySourceHash != failureSourceHash || retryTargetHash != failureTargetHash {
		t.Fatalf("exact event replay changed projection hash: before=%s/%s after=%s/%s",
			failureSourceHash, failureTargetHash, retrySourceHash, retryTargetHash)
	}
	if _, collisionErr := redisDedup.CheckAndIncrementEventAtomic(
		ctx, retryFingerprint+"-different-payload", alertProjectionKafkaRetry,
		alertProjectionKafkaRetryDetection().GetBehaviors()[0].GetHeader().GetEventTs(), alertProjectionKafkaTenant,
	); collisionErr == nil || !strings.Contains(collisionErr.Error(), "event identity collision") {
		t.Fatalf("event identity collision did not fail closed: %v", collisionErr)
	}
	countAfterCollision, err := redisClient.Get(ctx, redisCountKey).Int64()
	if err != nil || countAfterCollision != countAfterRecovery {
		t.Fatalf("event identity collision mutated Redis aggregate: before=%d after=%d err=%v", countAfterRecovery, countAfterCollision, err)
	}

	// Distinct events are independent alert identities. Publish a newer source
	// event before an older one and prove both project, while the Redis aggregate
	// keeps min(first_seen), max(last_seen), and a monotonic count. Replaying the
	// newer event after the older one must recover its original per-event tuple.
	newerDetection := alertProjectionKafkaDistinctDetection(alertProjectionKafkaNewer, 2*time.Minute, 2)
	olderDetection := alertProjectionKafkaDistinctDetection(alertProjectionKafkaOlder, time.Minute, 3)
	if err := writeAlertProjectionDetection(ctx, writer, topic, newerDetection); err != nil {
		t.Fatal(err)
	}
	waitForAlertProjectionOffset(t, ctx, offsetReader, retryConsumer, topic, groupID, 3, alertProjectionKafkaNewer)
	if err := writeAlertProjectionDetection(ctx, writer, topic, olderDetection); err != nil {
		t.Fatal(err)
	}
	waitForAlertProjectionOffset(t, ctx, offsetReader, retryConsumer, topic, groupID, 4, alertProjectionKafkaOlder)

	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	authoritative, truncated, err = authority.ListProjectionAlerts(ctx, scope)
	if err != nil || truncated || len(authoritative) != 4 {
		t.Fatalf("unexpected ClickHouse authority after out-of-order events: count=%d truncated=%v err=%v", len(authoritative), truncated, err)
	}
	projected, targetTruncated, err = target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(projected) != 4 {
		t.Fatalf("unexpected OpenSearch projection after out-of-order events: count=%d truncated=%v err=%v", len(projected), targetTruncated, err)
	}
	newerHash := verifyProjectionEventAcrossStores(t, ctx, pgDB, authoritative, projected, alertProjectionKafkaNewer)
	verifyProjectionEventAcrossStores(t, ctx, pgDB, authoritative, projected, alertProjectionKafkaOlder)
	countAfterOutOfOrder, err := redisClient.Get(ctx, redisCountKey).Int64()
	if err != nil || countAfterOutOfOrder != 4 {
		t.Fatalf("out-of-order events produced wrong Redis count: count=%d err=%v", countAfterOutOfOrder, err)
	}
	firstSeenAfterOutOfOrder, err := redisClient.Get(ctx, strings.TrimSuffix(redisCountKey, ":count")+":first_seen").Int64()
	if err != nil || firstSeenAfterOutOfOrder != alertProjectionKafkaDetection().GetBehaviors()[0].GetHeader().GetEventTs() {
		t.Fatalf("out-of-order events changed aggregate first_seen: value=%d err=%v", firstSeenAfterOutOfOrder, err)
	}
	lastSeenAfterOutOfOrder, err := redisClient.Get(ctx, strings.TrimSuffix(redisCountKey, ":count")+":last_seen").Int64()
	if err != nil || lastSeenAfterOutOfOrder != newerDetection.GetBehaviors()[0].GetHeader().GetEventTs() {
		t.Fatalf("out-of-order event regressed aggregate last_seen: value=%d err=%v", lastSeenAfterOutOfOrder, err)
	}

	if err := writeAlertProjectionDetection(ctx, writer, topic, newerDetection); err != nil {
		t.Fatal(err)
	}
	waitForAlertProjectionOffset(t, ctx, offsetReader, retryConsumer, topic, groupID, 5, alertProjectionKafkaNewer)
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	authoritative, truncated, err = authority.ListProjectionAlerts(ctx, scope)
	if err != nil || truncated || len(authoritative) != 4 {
		t.Fatalf("unexpected ClickHouse authority after delayed exact replay: count=%d truncated=%v err=%v", len(authoritative), truncated, err)
	}
	projected, targetTruncated, err = target.ListProjectionAlerts(ctx, scope)
	if err != nil || targetTruncated || len(projected) != 4 {
		t.Fatalf("unexpected OpenSearch projection after delayed exact replay: count=%d truncated=%v err=%v", len(projected), targetTruncated, err)
	}
	newerReplayHash := verifyProjectionEventAcrossStores(t, ctx, pgDB, authoritative, projected, alertProjectionKafkaNewer)
	if newerReplayHash != newerHash {
		t.Fatalf("delayed exact replay changed newer event projection hash: before=%s after=%s", newerHash, newerReplayHash)
	}
	countAfterDelayedReplay, err := redisClient.Get(ctx, redisCountKey).Int64()
	if err != nil || countAfterDelayedReplay != countAfterOutOfOrder {
		t.Fatalf("delayed exact replay inflated Redis count: before=%d after=%d err=%v", countAfterOutOfOrder, countAfterDelayedReplay, err)
	}

	// Add a second member to the two-partition group, require a stable one-
	// partition assignment per member, and publish one event directly to each
	// partition. Both consumers must commit before the first member leaves. The
	// remaining member must then take over the partition previously owned by the
	// departed member without losing its next projection receipt.
	rebalanceConsumer := NewConsumer(
		alertconfig.KafkaConfig{Brokers: []string{settings.kafkaBroker}, Topic: topic, GroupID: groupID, BatchSize: 1},
		alertconfig.DedupConfig{TimeBucketMinutes: 10, TTL: 10 * time.Minute},
		dedup.NewRedisDedup(redisClient, 10*time.Minute, logger), dualWriter, logger,
	)
	if rebalanceConsumer == nil {
		t.Fatal("second production alert consumer could not be created")
	}
	rebalanceCtx, stopRebalance := context.WithCancel(ctx)
	rebalanceDone := make(chan error, 1)
	go func() { rebalanceDone <- rebalanceConsumer.Start(rebalanceCtx) }()
	t.Cleanup(func() {
		stopRebalance()
		_ = rebalanceConsumer.Close()
		select {
		case <-rebalanceDone:
		case <-time.After(2 * time.Second):
		}
	})
	waitForAlertProjectionGroupAssignments(t, ctx, settings.kafkaBroker, topic, groupID, 2, 2)
	primaryCommitsBeforeRebalance := retryConsumer.kafkaConsumer.GetMetrics().CommitsSucceeded
	p0Detection := alertProjectionKafkaDistinctDetection(alertProjectionKafkaP0, 3*time.Minute, 4)
	p1Detection := alertProjectionKafkaDistinctDetection(alertProjectionKafkaP1, 4*time.Minute, 5)
	if err := writeAlertProjectionDetectionToPartition(ctx, settings.kafkaBroker, topic, 0, p0Detection); err != nil {
		t.Fatal(err)
	}
	if err := writeAlertProjectionDetectionToPartition(ctx, settings.kafkaBroker, topic, 1, p1Detection); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		lag, err = offsetReader.ReadLag(ctx, topic, groupID)
		primaryMetrics := retryConsumer.kafkaConsumer.GetMetrics()
		secondaryMetrics := rebalanceConsumer.kafkaConsumer.GetMetrics()
		_, p0Primary := committedEventPartition(retryConsumer, topic, alertProjectionKafkaP0)
		_, p1Primary := committedEventPartition(retryConsumer, topic, alertProjectionKafkaP1)
		_, p0Secondary := committedEventPartition(rebalanceConsumer, topic, alertProjectionKafkaP0)
		_, p1Secondary := committedEventPartition(rebalanceConsumer, topic, alertProjectionKafkaP1)
		if err == nil && lag.TotalCommittedOffset == 7 && lag.TotalLag == 0 &&
			primaryMetrics.CommitsSucceeded > primaryCommitsBeforeRebalance && secondaryMetrics.CommitsSucceeded > 0 &&
			(p0Primary || p0Secondary) && (p1Primary || p1Secondary) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	primaryPartition, primaryP0 := committedEventPartition(retryConsumer, topic, alertProjectionKafkaP0)
	primaryPartitionP1, primaryP1 := committedEventPartition(retryConsumer, topic, alertProjectionKafkaP1)
	if primaryP0 == primaryP1 {
		t.Fatalf("two-member assignment did not produce exactly one new partition commit on primary: p0=%v p1=%v lag=%+v err=%v", primaryP0, primaryP1, lag, err)
	}
	if primaryP1 {
		primaryPartition = primaryPartitionP1
	}
	secondaryPartition, secondaryP0 := committedEventPartition(rebalanceConsumer, topic, alertProjectionKafkaP0)
	secondaryPartitionP1, secondaryP1 := committedEventPartition(rebalanceConsumer, topic, alertProjectionKafkaP1)
	if secondaryP0 == secondaryP1 {
		t.Fatalf("two-member assignment did not produce exactly one new partition commit on secondary: p0=%v p1=%v lag=%+v err=%v", secondaryP0, secondaryP1, lag, err)
	}
	if secondaryP1 {
		secondaryPartition = secondaryPartitionP1
	}
	if err != nil || lag.TotalCommittedOffset != 7 || lag.TotalLag != 0 || primaryPartition == secondaryPartition {
		t.Fatalf("two-partition projection did not converge across both consumers: primary_partition=%d secondary_partition=%d lag=%+v err=%v",
			primaryPartition, secondaryPartition, lag, err)
	}

	stopRetry()
	if err := retryConsumer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-retryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("primary rebalance consumer did not stop")
	}
	retryConsumerClosed = true
	waitForAlertProjectionGroupAssignments(t, ctx, settings.kafkaBroker, topic, groupID, 1, 2)
	takeoverDetection := alertProjectionKafkaDistinctDetection(alertProjectionKafkaMove, 5*time.Minute, 6)
	if err := writeAlertProjectionDetectionToPartition(ctx, settings.kafkaBroker, topic, primaryPartition, takeoverDetection); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		lag, err = offsetReader.ReadLag(ctx, topic, groupID)
		takeoverPartition, takeoverObserved := committedEventPartition(rebalanceConsumer, topic, alertProjectionKafkaMove)
		if err == nil && lag.TotalCommittedOffset == 8 && lag.TotalLag == 0 && takeoverObserved && takeoverPartition == primaryPartition {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	takeoverPartition, takeoverObserved := committedEventPartition(rebalanceConsumer, topic, alertProjectionKafkaMove)
	if err != nil || lag.TotalCommittedOffset != 8 || lag.TotalLag != 0 || !takeoverObserved || takeoverPartition != primaryPartition {
		t.Fatalf("remaining consumer did not take over departed partition: departed_partition=%d observed_partition=%d observed=%v lag=%+v err=%v",
			primaryPartition, takeoverPartition, takeoverObserved, lag, err)
	}
	if err := target.RefreshProjectionTarget(ctx); err != nil {
		t.Fatal(err)
	}
	authoritative, truncated, err = authority.ListProjectionAlerts(ctx, persistence.ProjectionScope{TenantID: alertProjectionKafkaTenant, MaxDocuments: 20, TargetIndexVersion: "alerts-v2-write"})
	if err != nil || truncated || len(authoritative) != 7 {
		t.Fatalf("unexpected ClickHouse authority after rebalance: count=%d truncated=%v err=%v", len(authoritative), truncated, err)
	}
	projected, targetTruncated, err = target.ListProjectionAlerts(ctx, persistence.ProjectionScope{TenantID: alertProjectionKafkaTenant, MaxDocuments: 20, TargetIndexVersion: "alerts-v2-write"})
	if err != nil || targetTruncated || len(projected) != 7 {
		t.Fatalf("unexpected OpenSearch projection after rebalance: count=%d truncated=%v err=%v", len(projected), targetTruncated, err)
	}
	verifyProjectionEventAcrossStores(t, ctx, pgDB, authoritative, projected, alertProjectionKafkaP0)
	verifyProjectionEventAcrossStores(t, ctx, pgDB, authoritative, projected, alertProjectionKafkaP1)
	verifyProjectionEventAcrossStores(t, ctx, pgDB, authoritative, projected, alertProjectionKafkaMove)
	t.Logf("PASS_ALERT_PROJECTION_REAL_KAFKA_RECEIPT event_id=%s committed_offset=1 lag_before_fault=0 source_sha256=%s", lastEvent, sourceHash)
	t.Logf("PASS_ALERT_PROJECTION_RECEIPT_FAILURE_RESTART retry_event_id=%s retained_offset=1 retained_lag=1 recovered_offset=%d recovered_lag=%d source_sha256=%s",
		retryLastEvent, recoveredOffset, recoveredLag, retrySourceHash)
	t.Logf("PASS_ALERT_PROJECTION_OUT_OF_ORDER_DISTINCT_EVENTS newer_event_id=%s older_event_id=%s final_offset=5 redis_count=4 aggregate_first_seen=%d aggregate_last_seen=%d newer_sha256=%s",
		alertProjectionKafkaNewer, alertProjectionKafkaOlder, firstSeenAfterOutOfOrder, lastSeenAfterOutOfOrder, newerReplayHash)
	t.Logf("PASS_ALERT_PROJECTION_MULTI_PARTITION_REBALANCE partitions=2 members_before_leave=2 primary_partition=%d secondary_partition=%d takeover_partition=%d final_offset=8 final_lag=0 projected_events=7",
		primaryPartition, secondaryPartition, takeoverPartition)
}

func waitForAlertProjectionOffset(
	t *testing.T,
	ctx context.Context,
	offsetReader *dataquality.KafkaBrokerOffsetReader,
	consumer *Consumer,
	topic string,
	groupID string,
	wantOffset int64,
	wantEventID string,
) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lag dataquality.KafkaLagSnapshot
	var err error
	var lastEvent string
	for time.Now().Before(deadline) {
		lag, err = offsetReader.ReadLag(ctx, topic, groupID)
		_, observed := committedEventPartition(consumer, topic, wantEventID)
		if observed {
			lastEvent = wantEventID
		}
		if err == nil && lag.TotalCommittedOffset == wantOffset && lag.TotalLag == 0 && observed {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Kafka projection offset did not converge: want_offset=%d want_event=%s lag=%+v last_event=%s err=%v",
		wantOffset, wantEventID, lag, lastEvent, err)
}

func committedEventPartition(consumer *Consumer, topic, eventID string) (int, bool) {
	consumer.commitMetricMu.Lock()
	defer consumer.commitMetricMu.Unlock()
	prefix := topic + ":"
	for key, committedEventID := range consumer.lastCommitEvent {
		if committedEventID != eventID || !strings.HasPrefix(key, prefix) {
			continue
		}
		partition, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
		if err == nil {
			return partition, true
		}
	}
	return 0, false
}

func waitForAlertProjectionGroupAssignments(
	t *testing.T,
	ctx context.Context,
	broker string,
	topic string,
	groupID string,
	wantMembers int,
	wantPartitions int,
) {
	t.Helper()
	client := &segmentkafka.Client{Addr: segmentkafka.TCP(broker)}
	deadline := time.Now().Add(30 * time.Second)
	var lastState string
	var lastMembers, lastPartitions int
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := client.DescribeGroups(ctx, &segmentkafka.DescribeGroupsRequest{GroupIDs: []string{groupID}})
		lastErr = err
		if err == nil && len(response.Groups) == 1 {
			group := response.Groups[0]
			lastState = group.GroupState
			lastMembers = len(group.Members)
			assigned := map[int]struct{}{}
			for _, member := range group.Members {
				for _, assignment := range member.MemberAssignments.Topics {
					if assignment.Topic != topic {
						continue
					}
					for _, partition := range assignment.Partitions {
						assigned[partition] = struct{}{}
					}
				}
			}
			lastPartitions = len(assigned)
			if group.Error == nil && group.GroupState == "Stable" && lastMembers == wantMembers && lastPartitions == wantPartitions {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("consumer group assignment did not stabilize: state=%s members=%d partitions=%d want_members=%d want_partitions=%d err=%v",
		lastState, lastMembers, lastPartitions, wantMembers, wantPartitions, lastErr)
}

func verifyProjectionEventAcrossStores(
	t *testing.T,
	ctx context.Context,
	pgDB *sql.DB,
	authoritative []*persistence.Alert,
	projected []*persistence.Alert,
	eventID string,
) string {
	t.Helper()
	var source, target *persistence.Alert
	for _, alert := range authoritative {
		if alert.EventID == eventID {
			source = alert
			break
		}
	}
	for _, alert := range projected {
		if alert.EventID == eventID {
			target = alert
			break
		}
	}
	if source == nil || target == nil {
		t.Fatalf("event %s missing from source or target: source=%v target=%v", eventID, source, target)
	}
	sourceHash, err := persistence.AlertProjectionSHA256(source)
	if err != nil {
		t.Fatal(err)
	}
	targetHash, err := persistence.AlertProjectionSHA256(target)
	if err != nil {
		t.Fatal(err)
	}
	var receiptEvent, receiptHash string
	var receiptVersion int64
	if err := pgDB.QueryRowContext(ctx, `SELECT source_event_id,source_version,source_sha256
		FROM alert_opensearch_projection_watermarks
		WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version='alerts-v2-write'`,
		alertProjectionKafkaTenant, source.AlertID,
	).Scan(&receiptEvent, &receiptVersion, &receiptHash); err != nil {
		t.Fatal(err)
	}
	if receiptEvent != eventID || sourceHash != targetHash || sourceHash != receiptHash ||
		receiptVersion != persistence.AlertSourceVersion(source) {
		t.Fatalf("event %s cross-store mismatch: receipt_event=%s hashes=%s/%s/%s versions=%d/%d",
			eventID, receiptEvent, sourceHash, targetHash, receiptHash, persistence.AlertSourceVersion(source), receiptVersion)
	}
	return sourceHash
}

func projectionHashForEvent(t *testing.T, alerts []*persistence.Alert, eventID string) string {
	t.Helper()
	for _, alert := range alerts {
		if alert.EventID != eventID {
			continue
		}
		hash, err := persistence.AlertProjectionSHA256(alert)
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	t.Fatalf("projection event %s not found", eventID)
	return ""
}

func writeAlertProjectionDetection(ctx context.Context, writer *segmentkafka.Writer, topic string, detection *pb.DetectionBatch) error {
	message, err := alertProjectionKafkaMessage(topic, detection)
	if err != nil {
		return err
	}
	return writer.WriteMessages(ctx, message)
}

func writeAlertProjectionDetectionToPartition(ctx context.Context, broker, topic string, partition int, detection *pb.DetectionBatch) error {
	connection, err := segmentkafka.DialLeader(ctx, "tcp", broker, topic, partition)
	if err != nil {
		return err
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetWriteDeadline(deadline)
	}
	message, err := alertProjectionKafkaMessage("", detection)
	if err != nil {
		return err
	}
	_, err = connection.WriteMessages(message)
	return err
}

func alertProjectionKafkaMessage(topic string, detection *pb.DetectionBatch) (segmentkafka.Message, error) {
	payload, err := proto.Marshal(detection)
	if err != nil {
		return segmentkafka.Message{}, err
	}
	eventID := detection.GetBehaviors()[0].GetHeader().GetEventId()
	return segmentkafka.Message{
		Topic: topic, Key: []byte(alertProjectionKafkaTenant + ":" + eventID), Value: payload,
		Headers: []segmentkafka.Header{
			{Key: "tenant_id", Value: []byte(alertProjectionKafkaTenant)},
			{Key: "event_id", Value: []byte(eventID)},
			{Key: "trace_id", Value: []byte("0123456789abcdef0123456789abcdef")},
			{Key: "content_type", Value: []byte("application/x-protobuf")},
			{Key: "proto_message_type", Value: []byte("traffic.v1.DetectionBatch")},
		},
	}, nil
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

func alertProjectionKafkaDistinctDetection(eventID string, delta time.Duration, portDelta uint32) *pb.DetectionBatch {
	detection := alertProjectionKafkaDetection()
	behavior := detection.Behaviors[0]
	deltaMillis := int64(delta / time.Millisecond)
	behavior.Header.EventId = eventID
	behavior.Header.IdempotencyKey = eventID
	behavior.Header.AggregateId = "session-" + eventID
	behavior.Header.EventTs += deltaMillis
	behavior.Header.IngestTs += deltaMillis
	behavior.Header.KafkaTs += deltaMillis
	behavior.Header.FlinkOutTs += deltaMillis
	behavior.Header.OccurredAt += deltaMillis
	behavior.Header.ProducedAt += deltaMillis
	behavior.Ts += deltaMillis
	behavior.ObjectId = "session-" + eventID
	behavior.Tuple.SrcPort += portDelta
	detection.BatchId = "batch-" + eventID
	detection.CreatedAt += deltaMillis
	return detection
}

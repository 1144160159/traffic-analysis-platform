package consumer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestAlertEvidenceLinkConsumerEphemeralKubernetes(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_EPHEMERAL_PG_DSN"))
	broker := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_EPHEMERAL_KAFKA_BROKER"))
	clickhouseHost := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_EPHEMERAL_CLICKHOUSE_HOST"))
	runID := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_CANARY_RUN_ID"))
	if dsn == "" || broker == "" || clickhouseHost == "" || runID == "" {
		t.Skip("run-scoped Kubernetes PostgreSQL, Kafka, ClickHouse and canary identity are required")
	}
	if _, err := uuid.Parse(runID); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_alert_evidence_link_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel PostgreSQL: marker=%q err=%v", sentinel, err)
	}
	chClient, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{clickhouseHost}, Database: "traffic", Username: "default",
		DialTimeout: 10 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer chClient.Close()
	row, err := chClient.QueryRow(context.Background(), `SELECT any(marker) FROM traffic.codex_ephemeral_alert_evidence_link_sentinel`)
	if err != nil {
		t.Fatal(err)
	}
	if err := row.Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel ClickHouse: marker=%q err=%v", sentinel, err)
	}

	projection := api.NewAlertEvidenceLinkProjectionApplier(db, chClient)
	kafkaConsumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: []string{broker}, Topic: api.AlertEvidenceLinkEventTopic,
		GroupID:  "n012-consumer-" + strings.ReplaceAll(runID, "-", ""),
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 250 * time.Millisecond,
		StartOffset: segmentkafka.FirstOffset, MaxRetries: 3, RetryBackoff: 100 * time.Millisecond,
		CommitOnHandlerError: false, EnableDLQ: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := NewAlertEvidenceLinkConsumer(
		kafkaConsumer, projection, api.AlertEvidenceLinkEventTopic,
		"n012-consumer-"+strings.ReplaceAll(runID, "-", ""), strings.Repeat("a", 64), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = consumer.Close()
	})
	tenantID := "n012-" + strings.ReplaceAll(runID, "-", "")[:16]
	deadline := time.Now().Add(30 * time.Second)
	for {
		var projected int
		if err := db.QueryRow(`SELECT count(*) FROM alert_evidence_link_projection_inbox
			WHERE tenant_id=$1 AND projection_status='projected'`, tenantID).Scan(&projected); err != nil {
			t.Fatal(err)
		}
		if projected == 3 {
			break
		}
		select {
		case startErr := <-done:
			t.Fatalf("consumer stopped before projection completed: %v", startErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for three projected events; observed %d", projected)
		}
		time.Sleep(100 * time.Millisecond)
	}

	row, err = chClient.QueryRow(context.Background(), `SELECT count(),uniqExact(event_id),
		argMax(status,relation_revision),max(relation_revision),uniqExact(object_sha256),uniqExact(object_version)
		FROM traffic.alert_evidence_links_v1_local WHERE tenant_id=?`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	var rowCount, eventCount, maxRevision, digestCount, versionCount uint64
	var status string
	if err := row.Scan(&rowCount, &eventCount, &status, &maxRevision, &digestCount, &versionCount); err != nil {
		t.Fatal(err)
	}
	var inboxCount, deliveryCount, watermarkCount int
	_ = db.QueryRow(`SELECT count(*) FROM alert_evidence_link_projection_inbox WHERE tenant_id=$1`, tenantID).Scan(&inboxCount)
	_ = db.QueryRow(`SELECT count(*) FROM alert_evidence_link_projection_deliveries d
		JOIN alert_evidence_link_projection_inbox i ON i.event_id=d.event_id WHERE i.tenant_id=$1`, tenantID).Scan(&deliveryCount)
	_ = db.QueryRow(`SELECT count(*) FROM alert_evidence_link_projection_watermarks`).Scan(&watermarkCount)
	if rowCount < 3 || eventCount != 3 || status != "linked" || maxRevision != 3 ||
		digestCount != 1 || versionCount != 1 || inboxCount != 3 || deliveryCount != 3 || watermarkCount < 1 {
		t.Fatalf("rows=%d events=%d status=%s revision=%d digests=%d versions=%d inbox=%d deliveries=%d watermarks=%d",
			rowCount, eventCount, status, maxRevision, digestCount, versionCount, inboxCount, deliveryCount, watermarkCount)
	}

	cancel()
	_ = consumer.Close()
	select {
	case startErr := <-done:
		if startErr != nil && startErr != context.Canceled && !strings.Contains(startErr.Error(), "closed") {
			t.Fatalf("consumer shutdown: %v", startErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop within deadline")
	}
	cleanupAlertEvidenceLinkCanary(t, db, chClient, tenantID)
}

func cleanupAlertEvidenceLinkCanary(
	t *testing.T, db *sql.DB, chClient *storage.ClickHouseClient, tenantID string,
) {
	t.Helper()
	statements := []string{
		`DELETE FROM alert_evidence_link_projection_deliveries WHERE event_id IN (SELECT event_id FROM alert_evidence_link_projection_inbox WHERE tenant_id=$1)`,
		`DELETE FROM alert_evidence_link_projection_watermarks`,
		`DELETE FROM alert_evidence_link_projection_inbox WHERE tenant_id=$1`,
		`DELETE FROM alert_evidence_link_commands WHERE tenant_id=$1`,
		`DELETE FROM alert_evidence_link_outbox WHERE tenant_id=$1`,
		`DELETE FROM alert_evidence_link_history WHERE tenant_id=$1`,
		`DELETE FROM alert_evidence_links WHERE tenant_id=$1`,
		`DELETE FROM alert_evidence_manifest_history WHERE tenant_id=$1`,
		`DELETE FROM alert_evidence_manifests WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	}
	for _, statement := range statements {
		var err error
		if strings.Contains(statement, "$1") {
			_, err = db.Exec(statement, tenantID)
		} else {
			_, err = db.Exec(statement)
		}
		if err != nil {
			t.Fatal(fmt.Errorf("cleanup %q: %w", statement, err))
		}
	}
	if err := chClient.Exec(context.Background(), `TRUNCATE TABLE traffic.alert_evidence_links_v1_local`); err != nil {
		t.Fatal(err)
	}
}

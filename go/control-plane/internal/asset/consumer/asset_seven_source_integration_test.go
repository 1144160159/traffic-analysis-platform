package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	segmentkafka "github.com/segmentio/kafka-go"
	nebula_go "github.com/vesoft-inc/nebula-go/v3"
	"go.uber.org/zap"

	alertPersistence "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	graphConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

const (
	sevenSourceTenant = "asset-seven-source-integration"
	sevenSourceTrace  = "0123456789abcdef0123456789abcdef"
)

type sevenSourceRecord struct {
	RecordID string `json:"record_id"`
	Version  int64  `json:"version"`
	SHA256   string `json:"sha256"`
	TraceID  string `json:"trace_id"`
}

type sevenSourceEntry struct {
	Source    string              `json:"source"`
	Watermark map[string]string   `json:"watermark"`
	Records   []sevenSourceRecord `json:"records"`
}

type sevenSourceManifest struct {
	SchemaVersion       int                `json:"schema_version"`
	TenantID            string             `json:"tenant_id"`
	DataDomain          string             `json:"data_domain"`
	AuthoritativeSource string             `json:"authoritative_source"`
	Sources             []sevenSourceEntry `json:"sources"`
}

// TestAssetSevenSourceTraceReconciliation exercises one owned, bounded asset
// revision through the production PostgreSQL/outbox/Kafka/inbox/OpenSearch/
// NebulaGraph path. ClickHouse uses the production alert writer and MinIO uses
// an explicitly seeded evidence object because neither is currently downstream
// of asset.events.v2. The emitted normalized manifest makes that G1 boundary
// machine-checkable by the repository reconciliation oracle.
func TestAssetSevenSourceTraceReconciliation(t *testing.T) {
	manifestPath := strings.TrimSpace(os.Getenv("ASSET_SEVEN_SOURCE_MANIFEST"))
	if manifestPath == "" {
		t.Skip("ASSET_SEVEN_SOURCE_MANIFEST is not set")
	}
	if os.Getenv("ASSET_SEVEN_SOURCE_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing seven-source endpoints without explicit ephemeral sentinel")
	}
	for name, endpoint := range map[string]string{
		"kafka":       os.Getenv("ASSET_SEVEN_SOURCE_KAFKA_BROKER"),
		"opensearch":  strings.TrimPrefix(os.Getenv("ASSET_SEVEN_SOURCE_OS_URL"), "http://"),
		"clickhouse":  os.Getenv("ASSET_SEVEN_SOURCE_CLICKHOUSE_HOST"),
		"minio":       os.Getenv("ASSET_SEVEN_SOURCE_MINIO_ENDPOINT"),
		"nebulagraph": os.Getenv("ASSET_SEVEN_SOURCE_NEBULA_ADDRESS"),
	} {
		requireSevenSourceLoopback(t, name, endpoint)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", os.Getenv("ASSET_SEVEN_SOURCE_PG_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM codex_ephemeral_seven_source_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel PostgreSQL: marker=%q err=%v", marker, err)
	}
	cleanupSevenSourceTenant(t, db)
	defer cleanupSevenSourceTenant(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,'Seven Source Integration')`, sevenSourceTenant); err != nil {
		t.Fatal(err)
	}

	graphStore := openSevenSourceNebula(t, ctx)
	defer graphStore.Close()
	osTarget, err := NewOpenSearchAssetProjection([]string{os.Getenv("ASSET_SEVEN_SOURCE_OS_URL")}, "", "", "assets-v2-write")
	if err != nil {
		t.Fatal(err)
	}
	if err := osTarget.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	nebulaTarget, err := NewNebulaAssetProjection(graphStore)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Minute)
	record := &config.AssetRecord{
		TenantID: sevenSourceTenant, MACAddress: "02:00:00:00:07:01",
		IPAddress: "192.0.2.71", Hostname: "seven-source-asset", AssetType: "server",
		Status: "active", Source: "integration", Criticality: 4,
		Metadata: map[string]any{"scope": "owned-ephemeral"},
	}
	upsert, err := repo.UpsertAtomic(ctx, record, config.AssetUpsertCommand{
		ExpectedRevision: 0, IdempotencyKey: "asset-seven-source-create",
		Actor: "alignment-runner", Reason: "bounded seven-source reconciliation",
		TraceID: sevenSourceTrace, RequestID: "request-seven-source", ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	broker := os.Getenv("ASSET_SEVEN_SOURCE_KAFKA_BROKER")
	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: "asset.events.v2", BatchSize: 1,
		BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3, RequiredAcks: "all",
		Compression: "none", Async: false, IdempotentKey: "tenant_id+asset_id",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	dispatcher, err := assetRepository.NewAssetOutboxDispatcher(db, producer, assetRepository.OutboxDispatcherConfig{
		WorkerID: "seven-source-outbox", Lease: 10 * time.Second, MaxAttempts: 3, BatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	eventConsumer, err := NewAssetProjectionEventConsumer(db)
	if err != nil {
		t.Fatal(err)
	}
	kafkaConsumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: []string{broker}, Topic: "asset.events.v2", GroupID: "seven-source-" + uuid.NewString(),
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 100 * time.Millisecond,
		StartOffset: segmentkafka.FirstOffset, MaxRetries: 1, RetryBackoff: 25 * time.Millisecond,
		CommitOnHandlerError: false, DLQPermanentOnly: true,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	committed := make(chan segmentkafka.Message, 1)
	kafkaConsumer.SetCommitObserver(func(messages []segmentkafka.Message) {
		for _, message := range messages {
			committed <- message
		}
	})
	consumeCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)
	go func() { consumerDone <- kafkaConsumer.Consume(consumeCtx, eventConsumer.Handle) }()
	defer func() {
		stopConsumer()
		_ = kafkaConsumer.Close()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
		}
	}()
	if found, dispatchErr := dispatcher.DispatchNext(ctx); dispatchErr != nil || !found {
		t.Fatalf("dispatch found=%v err=%v", found, dispatchErr)
	}
	kafkaMessage := waitForAssetProjectionCommit(t, committed, upsert.EventID, 20*time.Second)
	var kafkaEvent AssetUpsertedV2
	if err := json.Unmarshal(kafkaMessage.Value, &kafkaEvent); err != nil {
		t.Fatal(err)
	}

	worker, err := NewAssetProjectionWorker(db, []AssetProjectionTarget{osTarget, nebulaTarget}, AssetProjectionWorkerConfig{
		WorkerID: "seven-source-projection", Lease: 10 * time.Second, MaxAttempts: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	projectDeadline := time.Now().Add(45 * time.Second)
	var lastProjectErr error
	for {
		found, projectErr := worker.ProjectNext(ctx)
		if projectErr != nil {
			lastProjectErr = projectErr
		}
		var projectionStatus string
		if err := db.QueryRowContext(ctx, `SELECT status FROM asset_projection_inbox WHERE event_id=$1`, upsert.EventID).Scan(&projectionStatus); err != nil {
			t.Fatal(err)
		}
		if projectionStatus == "applied" {
			break
		}
		if projectionStatus == "dead" || time.Now().After(projectDeadline) {
			t.Fatalf("projection did not converge: found=%v status=%q last_err=%v", found, projectionStatus, lastProjectErr)
		}
		time.Sleep(500 * time.Millisecond)
	}

	canonical := struct {
		TenantID string `json:"tenant_id"`
		AssetID  string `json:"asset_id"`
		EventID  string `json:"event_id"`
		TraceID  string `json:"trace_id"`
		Revision int64  `json:"revision"`
	}{sevenSourceTenant, upsert.AssetID, upsert.EventID, sevenSourceTrace, upsert.Revision}
	canonicalBytes, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonicalBytes)
	canonicalSHA := hex.EncodeToString(digest[:])

	verifySevenSourcePostgres(t, db, upsert, canonicalSHA)
	if kafkaEvent.TenantID != sevenSourceTenant || kafkaEvent.AssetID != upsert.AssetID || kafkaEvent.EventID != upsert.EventID || kafkaEvent.TraceID != sevenSourceTrace || kafkaEvent.AggregateVersion != upsert.Revision {
		t.Fatalf("Kafka identity drifted: %+v", kafkaEvent)
	}
	verifySevenSourceOpenSearch(t, os.Getenv("ASSET_SEVEN_SOURCE_OS_URL"), upsert)
	verifySevenSourceNebula(t, graphStore, upsert)
	writeAndVerifySevenSourceClickHouse(t, ctx, upsert, observedAt)
	writeAndVerifySevenSourceMinIO(t, ctx, upsert, canonicalSHA, canonicalBytes)

	position := strconv.FormatInt(upsert.Revision, 10)
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	sources := []string{"postgresql", "kafka", "clickhouse", "opensearch", "nebulagraph", "minio", "audit"}
	manifest := sevenSourceManifest{
		SchemaVersion: 1, TenantID: sevenSourceTenant, DataDomain: "assets",
		AuthoritativeSource: "postgresql", Sources: make([]sevenSourceEntry, 0, len(sources)),
	}
	for _, source := range sources {
		manifest.Sources = append(manifest.Sources, sevenSourceEntry{
			Source:    source,
			Watermark: map[string]string{"position_kind": "aggregate_version", "position": position, "observed_at": observed, "trace_id": sevenSourceTrace, "state": "complete"},
			Records:   []sevenSourceRecord{{RecordID: upsert.AssetID, Version: upsert.Revision, SHA256: canonicalSHA, TraceID: sevenSourceTrace}},
		})
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func requireSevenSourceLoopback(t *testing.T, name, endpoint string) {
	t.Helper()
	host, _, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("refusing non-loopback %s endpoint %q: %v", name, endpoint, err)
	}
}

func cleanupSevenSourceTenant(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`DELETE FROM asset_projection_watermarks WHERE tenant_id=$1`,
		`DELETE FROM asset_projection_inbox WHERE tenant_id=$1`,
		`DELETE FROM asset_upsert_requests WHERE tenant_id=$1`,
		`DELETE FROM asset_event_outbox WHERE tenant_id=$1`,
		`DELETE FROM asset_events WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM assets WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, sevenSourceTenant); err != nil {
			t.Fatalf("cleanup seven-source tenant: %v", err)
		}
	}
}

func openSevenSourceNebula(t *testing.T, ctx context.Context) *graphNebula.WorkbenchStore {
	t.Helper()
	address := os.Getenv("ASSET_SEVEN_SOURCE_NEBULA_ADDRESS")
	storageHost := os.Getenv("ASSET_SEVEN_SOURCE_NEBULA_STORAGE_HOST")
	if !strings.HasPrefix(storageHost, "codex-seven-source-nebula-storage-") {
		t.Fatalf("refusing unscoped Nebula storage host %q", storageHost)
	}
	host, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	poolConfig := nebula_go.GetDefaultConf()
	poolConfig.TimeOut = 5 * time.Second
	poolConfig.MaxConnPoolSize = 2
	poolConfig.MinConnPoolSize = 1
	var pool *nebula_go.ConnectionPool
	var session *nebula_go.Session
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		candidate, poolErr := nebula_go.NewConnectionPool(
			[]nebula_go.HostAddress{{Host: host, Port: port}}, poolConfig, nebula_go.DefaultLogger{},
		)
		if poolErr != nil {
			return poolErr
		}
		candidateSession, sessionErr := candidate.GetSession("root", "nebula")
		if sessionErr != nil {
			candidate.Close()
			return sessionErr
		}
		pool = candidate
		session = candidateSession
		return nil
	})
	defer pool.Close()
	defer session.Release()
	hosts := requireAssetNebulaStatement(t, session, "SHOW HOSTS;")
	if !strings.Contains(fmt.Sprint(hosts.AsStringTable()), storageHost) {
		requireAssetNebulaStatement(t, session, fmt.Sprintf("ADD HOSTS %q:9779;", storageHost))
	}
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		result, executeErr := session.Execute("SHOW HOSTS;")
		if executeErr != nil || !result.IsSucceed() || !strings.Contains(fmt.Sprint(result.AsStringTable()), "ONLINE") {
			return fmt.Errorf("storage is not online: %v %s", executeErr, assetNebulaResultError(result))
		}
		return nil
	})
	const space = "asset_seven_source_ephemeral"
	requireAssetNebulaStatement(t, session, "CREATE SPACE IF NOT EXISTS "+space+"(partition_num=1, replica_factor=1, vid_type=FIXED_STRING(32));")
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		result, executeErr := session.Execute("USE " + space + ";")
		if executeErr != nil || !result.IsSucceed() {
			return fmt.Errorf("space unavailable: %v %s", executeErr, assetNebulaResultError(result))
		}
		return nil
	})
	requireAssetNebulaStatement(t, session, `CREATE TAG IF NOT EXISTS entity(
		tenant_id STRING NOT NULL, entity_id STRING NOT NULL, entity_type STRING NOT NULL,
		label STRING NOT NULL, detail STRING, risk_score INT64 DEFAULT 0, risk_level STRING,
		x DOUBLE DEFAULT 0.0, y DOUBLE DEFAULT 0.0, icon STRING,
		metadata_json STRING DEFAULT '{}', updated_at INT64);`)
	requireAssetNebulaStatement(t, session, `CREATE EDGE IF NOT EXISTS relation(
		tenant_id STRING NOT NULL, relation_id STRING NOT NULL, source_id STRING NOT NULL,
		target_id STRING NOT NULL, relation_type STRING NOT NULL, risk_level STRING,
		evidence_id STRING, attributes_json STRING DEFAULT '{}', weight DOUBLE DEFAULT 1.0,
		observed_at INT64);`)
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		result, executeErr := session.Execute("DESC TAG entity;")
		if executeErr != nil || !result.IsSucceed() {
			return fmt.Errorf("entity schema unavailable: %v %s", executeErr, assetNebulaResultError(result))
		}
		return nil
	})
	var store *graphNebula.WorkbenchStore
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		candidate, storeErr := graphNebula.NewWorkbenchStore(graphConfig.NebulaConfig{
			Enabled: true, Addresses: []string{address}, Username: "root", Password: "nebula",
			Space: space, Timeout: 5 * time.Second, IdleTime: time.Minute, MaxPoolSize: 2, MinPoolSize: 1,
		}, zap.NewNop())
		if storeErr != nil {
			return storeErr
		}
		if readyErr := candidate.Ready(ctx); readyErr != nil {
			candidate.Close()
			return readyErr
		}
		store = candidate
		return nil
	})
	return store
}

func verifySevenSourcePostgres(t *testing.T, db *sql.DB, upsert *config.AssetUpsertResult, canonicalSHA string) {
	t.Helper()
	var eventID, traceID, outboxStatus, auditTrace string
	var revision int64
	var auditDetail []byte
	if err := db.QueryRow(`
		SELECT history.event_uuid::text,history.revision,history.trace_id,outbox.status,audit.trace_id,audit.detail::text
		FROM asset_events history
		JOIN asset_event_outbox outbox ON outbox.event_id=history.event_uuid
		JOIN audit_logs audit ON audit.object_id=history.asset_id::text AND audit.tenant_id=history.tenant_id
		WHERE history.event_uuid=$1 AND audit.action='ASSET_UPSERT'`, upsert.EventID).
		Scan(&eventID, &revision, &traceID, &outboxStatus, &auditTrace, &auditDetail); err != nil {
		t.Fatal(err)
	}
	var detail map[string]any
	if err := json.Unmarshal(auditDetail, &detail); err != nil {
		t.Fatal(err)
	}
	if eventID != upsert.EventID || revision != upsert.Revision || traceID != sevenSourceTrace || auditTrace != sevenSourceTrace || outboxStatus != "published" || detail["event_id"] != upsert.EventID || detail["trace_id"] != sevenSourceTrace || detail["revision"] != float64(upsert.Revision) || len(canonicalSHA) != 64 {
		t.Fatalf("PostgreSQL/audit identity drift event=%q revision=%d trace=%q audit=%q status=%q detail=%v", eventID, revision, traceID, auditTrace, outboxStatus, detail)
	}
	var inboxStatus string
	var inboxRevision int64
	if err := db.QueryRow(`SELECT status,aggregate_version FROM asset_projection_inbox WHERE event_id=$1`, upsert.EventID).Scan(&inboxStatus, &inboxRevision); err != nil {
		t.Fatal(err)
	}
	if inboxStatus != "applied" || inboxRevision != upsert.Revision {
		t.Fatalf("projection inbox status=%q revision=%d", inboxStatus, inboxRevision)
	}
}

func verifySevenSourceOpenSearch(t *testing.T, baseURL string, upsert *config.AssetUpsertResult) {
	t.Helper()
	response, err := http.Get(strings.TrimRight(baseURL, "/") + "/assets-v2-read/_doc/" + upsert.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var stored struct {
		Found  bool                `json:"found"`
		Source assetSearchDocument `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !stored.Found || stored.Source.EventID != upsert.EventID || stored.Source.TraceID != sevenSourceTrace || stored.Source.Revision != upsert.Revision || stored.Source.AssetID != upsert.AssetID {
		t.Fatalf("OpenSearch identity drift status=%s document=%+v", response.Status, stored)
	}
}

func verifySevenSourceNebula(t *testing.T, store *graphNebula.WorkbenchStore, upsert *config.AssetUpsertResult) {
	t.Helper()
	node, _, _, err := store.LoadAssetProjection(context.Background(), sevenSourceTenant, upsert.AssetID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(node.Metadata["event_id"]) != upsert.EventID || fmt.Sprint(node.Metadata["trace_id"]) != sevenSourceTrace || fmt.Sprint(node.Metadata["revision"]) != strconv.FormatInt(upsert.Revision, 10) {
		t.Fatalf("NebulaGraph identity drift: %+v", node.Metadata)
	}
}

func writeAndVerifySevenSourceClickHouse(t *testing.T, ctx context.Context, upsert *config.AssetUpsertResult, observedAt time.Time) {
	t.Helper()
	client, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{os.Getenv("ASSET_SEVEN_SOURCE_CLICKHOUSE_HOST")}, Database: "traffic",
		Username: os.Getenv("ASSET_SEVEN_SOURCE_CLICKHOUSE_USER"), Password: os.Getenv("ASSET_SEVEN_SOURCE_CLICKHOUSE_PASSWORD"),
		MaxOpenConns: 2, MaxIdleConns: 1, DialTimeout: 5 * time.Second, CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var marker string
	row, err := client.QueryRow(ctx, `SELECT marker FROM traffic.codex_ephemeral_asset_detail_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := row.Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel ClickHouse: marker=%q err=%v", marker, err)
	}
	writer, err := alertPersistence.NewClickHouseWriter(client, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	alert := &alertPersistence.Alert{
		TenantID: sevenSourceTenant, AlertID: upsert.AssetID, Fingerprint: "asset-" + upsert.AssetID,
		SrcIP: "192.0.2.71", DstIP: "203.0.113.71", SrcPort: 47001, DstPort: 443,
		Protocol: 6, AlertType: "asset-reconciliation", Labels: []string{"integration"},
		Score: 0.8, Severity: "medium", FirstSeen: observedAt, LastSeen: observedAt,
		Count: 1, Status: "new", UpdatedTs: observedAt, StateVersion: uint64(upsert.Revision),
		ModelVersion: "asset-g1", RuleVersion: "asset-g1", FeatureSetID: "asset-g1",
		EventID: upsert.EventID, TraceID: sevenSourceTrace,
	}
	if err := writer.WriteAlert(ctx, alert); err != nil {
		t.Fatal(err)
	}
	var eventID, traceID string
	var version uint64
	row, err = client.QueryRow(ctx, `SELECT event_id,trace_id,state_version FROM traffic.alerts WHERE tenant_id=? AND alert_id=?`, sevenSourceTenant, upsert.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if err := row.Scan(&eventID, &traceID, &version); err != nil || eventID != upsert.EventID || traceID != sevenSourceTrace || version != uint64(upsert.Revision) {
		t.Fatalf("ClickHouse identity drift event=%q trace=%q version=%d err=%v", eventID, traceID, version, err)
	}
}

func writeAndVerifySevenSourceMinIO(t *testing.T, ctx context.Context, upsert *config.AssetUpsertResult, canonicalSHA string, content []byte) {
	t.Helper()
	client, err := minio.New(os.Getenv("ASSET_SEVEN_SOURCE_MINIO_ENDPOINT"), &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("ASSET_SEVEN_SOURCE_MINIO_ACCESS_KEY"), os.Getenv("ASSET_SEVEN_SOURCE_MINIO_SECRET_KEY"), ""),
		Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	bucket := os.Getenv("ASSET_SEVEN_SOURCE_MINIO_BUCKET")
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	objectKey := sevenSourceTenant + "/" + upsert.AssetID + ".json"
	t.Cleanup(func() {
		_ = client.RemoveObject(context.Background(), bucket, objectKey, minio.RemoveObjectOptions{})
		_ = client.RemoveBucket(context.Background(), bucket)
	})
	metadata := map[string]string{
		"sha256": canonicalSHA, "event-id": upsert.EventID, "trace-id": sevenSourceTrace,
		"revision": strconv.FormatInt(upsert.Revision, 10),
	}
	if _, err := client.PutObject(ctx, bucket, objectKey, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{ContentType: "application/json", UserMetadata: metadata}); err != nil {
		t.Fatal(err)
	}
	stat, err := client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if stat.Metadata.Get("X-Amz-Meta-Sha256") != canonicalSHA || stat.Metadata.Get("X-Amz-Meta-Event-Id") != upsert.EventID || stat.Metadata.Get("X-Amz-Meta-Trace-Id") != sevenSourceTrace || stat.Metadata.Get("X-Amz-Meta-Revision") != strconv.FormatInt(upsert.Revision, 10) {
		t.Fatalf("MinIO identity metadata drift: %v", stat.Metadata)
	}
}

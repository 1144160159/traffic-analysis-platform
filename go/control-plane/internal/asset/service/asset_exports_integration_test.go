package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	segmentKafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

const assetExportIntegrationTenant = "asset-export-integration"
const assetExportMinIOIntegrationTenant = "asset-export-minio-integration"
const assetExportKafkaIntegrationTenant = "asset-export-kafka-integration"

type memoryAssetExportStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

type recordingAssetExportPublisher struct {
	mu            sync.Mutex
	payloads      [][]byte
	failRemaining int
}

func (p *recordingAssetExportPublisher) Send(_ context.Context, _ string, payload []byte, _ ...kafkaCommon.MessageHeader) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failRemaining > 0 {
		p.failRemaining--
		return errors.New("injected asset export publisher failure")
	}
	p.payloads = append(p.payloads, append([]byte(nil), payload...))
	return nil
}

func (s *memoryAssetExportStore) Put(_ context.Context, bucket, key string, reader io.Reader, size int64, _ string) error {
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(content)) != size {
		return fmt.Errorf("object size=%d want=%d", len(content), size)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[bucket+"/"+key] = append([]byte(nil), content...)
	return nil
}

func (s *memoryAssetExportStore) Open(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.objects[bucket+"/"+key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), content...))), nil
}

// The explicit DSN plus sentinel prevents this test from touching a shared or
// production PostgreSQL database.
func TestAssetExportPostgresObjectManifestAndPreferenceLifecycle(t *testing.T) {
	dsn := os.Getenv("ASSET_ATOMIC_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ASSET_ATOMIC_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	if err := cleanupAssetExportIntegration(db); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupAssetExportIntegration(db); err != nil {
			t.Errorf("cleanup asset export integration: %v", err)
		}
	}()
	if _, err := db.Exec(`
		INSERT INTO tenants(tenant_id,name) VALUES
		  ($1,'Asset Export Integration'),
		  ('asset-export-other','Asset Export Other')`, assetExportIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO assets(
		  tenant_id,display_code,asset_type,status,ip_address,mac_address,
		  hostname,department,campus,owner,criticality,source,last_seen
		) VALUES
		  ($1,'ASSET-001','server','active','192.0.2.10','02:00:00:00:00:10','server-a','security','north','alice',90,'integration',now()-interval '1 minute'),
		  ($1,'ASSET-002','server','active','192.0.2.11','02:00:00:00:00:11','server-b','security','north','bob',70,'integration',now()-interval '2 minutes'),
		  ($1,'ASSET-003','endpoint','active','192.0.2.12','02:00:00:00:00:12','endpoint-c','security','north','carol',20,'integration',now()-interval '3 minutes'),
		  ('asset-export-other','OTHER-001','server','active','198.51.100.1','02:00:00:00:10:01','other-tenant','security','north','mallory',99,'integration',now())`, assetExportIntegrationTenant); err != nil {
		t.Fatal(err)
	}

	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryAssetExportStore{objects: map[string][]byte{}}
	svc := service.New(&config.Config{Export: config.AssetExportConfig{
		Enabled: true, MaxRows: 100, MaxBytes: 1 << 20,
		WorkerLease: 30 * time.Second, Retention: time.Hour,
		Bucket: "report-artifacts",
	}}, repo, zap.NewNop()).WithAssetExportObjectStore(store)
	request := config.AssetExportRequest{
		ActionID: config.AssetExportActionID,
		Format:   "csv",
		Columns:  []string{"display_code", "hostname", "ip_address"},
		Filter: config.AssetListFilter{
			AssetType: "server", Department: "security", Campus: "north",
		},
		Reason: "authorized integration asset export",
	}
	command := config.AssetExportCommand{
		IdempotencyKey: "asset-export-integration-create",
		Actor:          "integration-analyst", TraceID: "trace-asset-export",
		RequestID: "request-asset-export", ClientIP: "127.0.0.1", UserAgent: "integration-test",
	}
	accepted, err := svc.CreateAssetExportJob(context.Background(), assetExportIntegrationTenant, request, command)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != config.AssetExportStatusAccepted || accepted.Revision != 1 {
		t.Fatalf("accepted=%+v", accepted)
	}
	replay, err := svc.CreateAssetExportJob(context.Background(), assetExportIntegrationTenant, request, command)
	if err != nil || replay.JobID != accepted.JobID || !replay.IdempotentReplay {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	conflict := request
	conflict.Format = "jsonl"
	if _, err := svc.CreateAssetExportJob(context.Background(), assetExportIntegrationTenant, conflict, command); !errors.Is(err, repository.ErrAssetExportIdempotencyConflict) {
		t.Fatalf("idempotency conflict err=%v", err)
	}

	found, err := svc.ProcessNextAssetExport(context.Background(), "asset-export-integration-worker")
	if err != nil || !found {
		t.Fatalf("worker found=%v err=%v", found, err)
	}
	completed, err := svc.GetAssetExportJob(context.Background(), assetExportIntegrationTenant, accepted.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != config.AssetExportStatusCompleted || completed.Revision != 3 || completed.RowCount != 2 || completed.ArtifactSHA256 == "" || completed.SizeBytes <= 0 || completed.SnapshotID == "" {
		t.Fatalf("completed=%+v", completed)
	}
	content, err := svc.ReadAssetExportArtifact(context.Background(), completed)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "ASSET-001") || !strings.Contains(text, "ASSET-002") || strings.Contains(text, "ASSET-003") || strings.Contains(text, "OTHER-001") {
		t.Fatalf("artifact tenant/filter isolation failed: %s", text)
	}
	if _, err := svc.GetAssetExportJob(context.Background(), "asset-export-other", accepted.JobID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant job read err=%v", err)
	}

	preference, err := svc.GetAssetColumnPreference(context.Background(), assetExportIntegrationTenant, "integration-user", "asset-inventory")
	if err != nil || preference.Revision != 0 || len(preference.Columns) == 0 {
		t.Fatalf("default preference=%+v err=%v", preference, err)
	}
	preferenceCommand := config.AssetColumnPreferenceCommand{
		ViewID: "asset-inventory", Columns: []string{"display_code", "hostname"},
		ExpectedRevision: 0, Reason: "set integration visible columns",
		Actor: "integration-user", TraceID: "trace-preference-1", RequestID: "request-preference-1",
	}
	preference, err = svc.UpsertAssetColumnPreference(context.Background(), assetExportIntegrationTenant, "integration-user", preferenceCommand)
	if err != nil || preference.Revision != 1 {
		t.Fatalf("created preference=%+v err=%v", preference, err)
	}
	preferenceCommand.Columns = []string{"display_code", "hostname", "ip_address"}
	preferenceCommand.ExpectedRevision = 1
	preferenceCommand.TraceID = "trace-preference-2"
	preference, err = svc.UpsertAssetColumnPreference(context.Background(), assetExportIntegrationTenant, "integration-user", preferenceCommand)
	if err != nil || preference.Revision != 2 {
		t.Fatalf("updated preference=%+v err=%v", preference, err)
	}
	preferenceCommand.ExpectedRevision = 1
	preferenceCommand.TraceID = "trace-preference-stale"
	if _, err := svc.UpsertAssetColumnPreference(context.Background(), assetExportIntegrationTenant, "integration-user", preferenceCommand); !errors.Is(err, repository.ErrAssetColumnPreferenceRevisionConflict) {
		t.Fatalf("stale preference err=%v", err)
	}
	otherUser, err := svc.GetAssetColumnPreference(context.Background(), assetExportIntegrationTenant, "another-user", "asset-inventory")
	if err != nil || otherUser.Revision != 0 {
		t.Fatalf("user preference isolation=%+v err=%v", otherUser, err)
	}
	if err := svc.RecordAssetExportDownload(context.Background(), completed, "integration-analyst", "trace-download", "request-download", "127.0.0.1", "integration-test"); err != nil {
		t.Fatal(err)
	}
	publisher := &recordingAssetExportPublisher{}
	dispatcher, err := repository.NewAssetExportOutboxDispatcher(db, publisher, repository.OutboxDispatcherConfig{
		WorkerID: "asset-export-integration-dispatcher", Lease: 30 * time.Second,
		MaxAttempts: 3, BatchSize: 10, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	for expected := 0; expected < 2; expected++ {
		found, err := dispatcher.DispatchNext(context.Background())
		if err != nil || !found {
			t.Fatalf("dispatch %d found=%v err=%v", expected, found, err)
		}
	}
	if found, err := dispatcher.DispatchNext(context.Background()); err != nil || found {
		t.Fatalf("empty export dispatch found=%v err=%v", found, err)
	}
	if len(publisher.payloads) != 2 || !bytes.Contains(publisher.payloads[0], []byte("traffic.asset.export.v1.Requested")) || !bytes.Contains(publisher.payloads[1], []byte("traffic.asset.export.v1.Completed")) {
		t.Fatalf("published export events=%q", publisher.payloads)
	}

	var jobs, outbox, publishedOutbox, requestedAudit, completedAudit, downloadedAudit, preferenceAudit, preferences int
	if err := db.QueryRow(`
		SELECT
		  (SELECT count(*) FROM asset_export_jobs WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_export_outbox WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_export_outbox WHERE tenant_id=$1 AND status='published'),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='ASSET_EXPORT_REQUESTED'),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='ASSET_EXPORT_COMPLETED'),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='ASSET_EXPORT_DOWNLOADED'),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='ASSET_COLUMN_PREFERENCE_UPDATED'),
		  (SELECT count(*) FROM asset_column_preferences WHERE tenant_id=$1)`,
		assetExportIntegrationTenant,
	).Scan(&jobs, &outbox, &publishedOutbox, &requestedAudit, &completedAudit, &downloadedAudit, &preferenceAudit, &preferences); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || outbox != 2 || publishedOutbox != 2 || requestedAudit != 1 || completedAudit != 1 || downloadedAudit != 1 || preferenceAudit != 2 || preferences != 1 {
		t.Fatalf("reconcile jobs=%d outbox=%d published=%d audits=%d/%d/%d/%d preferences=%d", jobs, outbox, publishedOutbox, requestedAudit, completedAudit, downloadedAudit, preferenceAudit, preferences)
	}

	assertAssetExportOutboxRetryAndDeadLifecycle(t, db, accepted.JobID, publisher)

	store.mu.Lock()
	store.objects[completed.ObjectBucket+"/"+completed.ObjectKey] = []byte("tampered")
	store.mu.Unlock()
	if _, err := svc.ReadAssetExportArtifact(context.Background(), completed); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("tampered object should fail manifest validation, err=%v", err)
	}
}

func assertAssetExportOutboxRetryAndDeadLifecycle(
	t *testing.T,
	db *sql.DB,
	jobID string,
	publisher *recordingAssetExportPublisher,
) {
	t.Helper()
	insertSyntheticAssetExportEvent := func(eventID string, aggregateVersion int) {
		t.Helper()
		payload := fmt.Sprintf(`{"event_id":%q,"event_type":"traffic.asset.export.v1.Completed","schema_version":1,"aggregate_version":%d,"partition_key":%q,"tenant_id":%q,"job_id":%q,"trace_id":"trace-export-reliability"}`,
			eventID, aggregateVersion, assetExportIntegrationTenant+":"+jobID,
			assetExportIntegrationTenant, jobID)
		if _, err := db.Exec(`
			INSERT INTO asset_export_outbox(
			  event_id,job_id,tenant_id,event_type,aggregate_version,
			  schema_version,partition_key,payload,status,next_attempt_at
			) VALUES($1,$2,$3,'traffic.asset.export.v1.Completed',$4,1,$5,$6::jsonb,'pending',now())`,
			eventID, jobID, assetExportIntegrationTenant, aggregateVersion,
			assetExportIntegrationTenant+":"+jobID, payload); err != nil {
			t.Fatal(err)
		}
	}

	retryEventID := "10000000-0000-4000-8000-000000000004"
	insertSyntheticAssetExportEvent(retryEventID, 4)
	publisher.mu.Lock()
	publisher.failRemaining = 1
	publisher.mu.Unlock()
	retryDispatcher, err := repository.NewAssetExportOutboxDispatcher(db, publisher, repository.OutboxDispatcherConfig{
		WorkerID: "asset-export-retry-dispatcher", Lease: 30 * time.Second,
		MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := retryDispatcher.DispatchNext(context.Background()); !found || err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("retry first dispatch found=%v err=%v", found, err)
	}
	var status, lockedBy, lastError string
	var attempts int
	if err := db.QueryRow(`SELECT status,attempts,locked_by,last_error FROM asset_export_outbox WHERE event_id=$1`, retryEventID).Scan(&status, &attempts, &lockedBy, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 1 || lockedBy != "" || !strings.Contains(lastError, "injected") {
		t.Fatalf("retry state status=%s attempts=%d locked_by=%q last_error=%q", status, attempts, lockedBy, lastError)
	}
	if _, err := db.Exec(`UPDATE asset_export_outbox SET next_attempt_at=now() WHERE event_id=$1`, retryEventID); err != nil {
		t.Fatal(err)
	}
	if found, err := retryDispatcher.DispatchNext(context.Background()); !found || err != nil {
		t.Fatalf("retry second dispatch found=%v err=%v", found, err)
	}
	if err := db.QueryRow(`SELECT status,attempts,locked_by,last_error FROM asset_export_outbox WHERE event_id=$1`, retryEventID).Scan(&status, &attempts, &lockedBy, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "published" || attempts != 2 || lockedBy != "" || lastError != "" {
		t.Fatalf("retry published state status=%s attempts=%d locked_by=%q last_error=%q", status, attempts, lockedBy, lastError)
	}

	deadEventID := "10000000-0000-4000-8000-000000000005"
	insertSyntheticAssetExportEvent(deadEventID, 5)
	publisher.mu.Lock()
	publisher.failRemaining = 1
	publisher.mu.Unlock()
	deadDispatcher, err := repository.NewAssetExportOutboxDispatcher(db, publisher, repository.OutboxDispatcherConfig{
		WorkerID: "asset-export-dead-dispatcher", Lease: 30 * time.Second,
		MaxAttempts: 1, BatchSize: 1, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := deadDispatcher.DispatchNext(context.Background()); !found || err == nil {
		t.Fatalf("dead dispatch found=%v err=%v", found, err)
	}
	if err := db.QueryRow(`SELECT status,attempts,locked_by,last_error FROM asset_export_outbox WHERE event_id=$1`, deadEventID).Scan(&status, &attempts, &lockedBy, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "dead" || attempts != 1 || lockedBy != "" || !strings.Contains(lastError, "injected") {
		t.Fatalf("dead state status=%s attempts=%d locked_by=%q last_error=%q", status, attempts, lockedBy, lastError)
	}
}

// This test is gated by an explicit ephemeral endpoint and the same PostgreSQL
// sentinel as the lifecycle test. It therefore cannot silently use shared S3
// credentials or a non-test database.
func TestAssetExportRealMinIOArtifactLifecycle(t *testing.T) {
	dsn := os.Getenv("ASSET_ATOMIC_EPHEMERAL_PG_DSN")
	endpoint := strings.TrimSpace(os.Getenv("ASSET_EXPORT_EPHEMERAL_S3_ENDPOINT"))
	accessKey := os.Getenv("ASSET_EXPORT_EPHEMERAL_S3_ACCESS_KEY")
	secretKey := os.Getenv("ASSET_EXPORT_EPHEMERAL_S3_SECRET_KEY")
	bucket := strings.TrimSpace(os.Getenv("ASSET_EXPORT_EPHEMERAL_S3_BUCKET"))
	if dsn == "" || endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("explicit ephemeral PostgreSQL and MinIO settings are required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	cleanup := func() {
		for _, table := range []string{"asset_export_outbox", "asset_export_jobs", "audit_logs", "assets", "tenants"} {
			if _, cleanupErr := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE tenant_id=$1", table), assetExportMinIOIntegrationTenant); cleanupErr != nil {
				t.Errorf("cleanup %s: %v", table, cleanupErr)
			}
		}
	}
	cleanup()
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Export MinIO Integration')`, assetExportMinIOIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO assets(
		  tenant_id,display_code,asset_type,status,ip_address,mac_address,
		  hostname,department,campus,owner,criticality,source,last_seen
		) VALUES($1,'MINIO-001','server','active','192.0.2.40','02:00:00:00:00:40',
		  'minio-server','security','east','dana',80,'integration',now())`, assetExportMinIOIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	minioEndpoint := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	client, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.HasPrefix(endpoint, "https://"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for object := range client.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err != nil {
				t.Errorf("list ephemeral MinIO object: %v", object.Err)
				continue
			}
			if err := client.RemoveObject(context.Background(), bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				t.Errorf("remove ephemeral MinIO object %q: %v", object.Key, err)
			}
		}
		if err := client.RemoveBucket(context.Background(), bucket); err != nil {
			t.Errorf("remove ephemeral MinIO bucket: %v", err)
		}
	}()

	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{Export: config.AssetExportConfig{
		Enabled: true, MaxRows: 10, MaxBytes: 1 << 20,
		WorkerLease: 30 * time.Second, Retention: time.Hour,
		Bucket: bucket, S3Endpoint: endpoint,
		S3AccessKey: accessKey, S3SecretKey: secretKey,
	}}, repo, zap.NewNop())
	job, err := svc.CreateAssetExportJob(context.Background(), assetExportMinIOIntegrationTenant, config.AssetExportRequest{
		ActionID: config.AssetExportActionID, Format: "jsonl",
		Columns: []string{"display_code", "hostname", "ip_address"},
		Reason:  "verify real ephemeral MinIO artifact lifecycle",
	}, config.AssetExportCommand{
		IdempotencyKey: "asset-export-real-minio-integration",
		Actor:          "integration-analyst", TraceID: "trace-real-minio",
		RequestID: "request-real-minio", ClientIP: "127.0.0.1", UserAgent: "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := svc.ProcessNextAssetExport(context.Background(), "asset-export-real-minio-worker"); err != nil || !found {
		t.Fatalf("worker found=%v err=%v", found, err)
	}
	completed, err := svc.GetAssetExportJob(context.Background(), assetExportMinIOIntegrationTenant, job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != config.AssetExportStatusCompleted || completed.ObjectBucket != bucket || completed.ArtifactSHA256 == "" || completed.SizeBytes <= 0 {
		t.Fatalf("completed=%+v", completed)
	}
	if completed.TraceID != "trace-real-minio" || completed.SnapshotID == "" || completed.AsOf.IsZero() ||
		len(completed.SourceWatermarks) != 4 || completed.RetentionUntil.Before(time.Now().UTC()) {
		t.Fatalf("incomplete export manifest trace=%q snapshot=%q as_of=%v watermarks=%v retention=%v",
			completed.TraceID, completed.SnapshotID, completed.AsOf, completed.SourceWatermarks, completed.RetentionUntil)
	}
	expectedObjectKey := assetExportMinIOIntegrationTenant + "/assets/exports/" + job.JobID + ".jsonl"
	if completed.ObjectKey != expectedObjectKey {
		t.Fatalf("object key=%q want=%q", completed.ObjectKey, expectedObjectKey)
	}
	content, err := svc.ReadAssetExportArtifact(context.Background(), completed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("MINIO-001")) || !bytes.Contains(content, []byte("minio-server")) {
		t.Fatalf("unexpected MinIO artifact: %s", content)
	}
	contentSHA256 := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	if contentSHA256 != completed.ArtifactSHA256 {
		t.Fatalf("download sha256=%q manifest=%q", contentSHA256, completed.ArtifactSHA256)
	}

	stat, err := client.StatObject(context.Background(), completed.ObjectBucket, completed.ObjectKey, minio.StatObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size != completed.SizeBytes || stat.ContentType != completed.MIMEType {
		t.Fatalf("MinIO metadata size=%d/%d content_type=%q/%q", stat.Size, completed.SizeBytes, stat.ContentType, completed.MIMEType)
	}
	if err := svc.RecordAssetExportDownload(
		context.Background(), completed, "integration-analyst", "trace-real-minio",
		"request-real-minio-download", "127.0.0.1", "integration-test",
	); err != nil {
		t.Fatal(err)
	}
	var reconciled bool
	if err := db.QueryRow(`
		SELECT
		  j.trace_id=$3
		  AND j.snapshot_id<>''
		  AND j.artifact_sha256=$4
		  AND j.object_bucket=$5
		  AND j.object_key=$6
		  AND j.size_bytes=$7
		  AND (SELECT count(*) FROM jsonb_object_keys(j.source_watermarks))=4
		  AND (SELECT count(*) FROM asset_export_outbox o
		         WHERE o.tenant_id=$1 AND o.job_id=$2::uuid
		           AND o.partition_key=$1 || ':' || $2::text
		           AND o.payload->>'tenant_id'=$1
		           AND o.payload->>'job_id'=$2::text
		           AND o.payload->>'trace_id'=$3)=2
		  AND (SELECT count(*) FROM asset_export_outbox o
		         WHERE o.tenant_id=$1 AND o.job_id=$2::uuid
		           AND o.event_type='traffic.asset.export.v1.Requested'
		           AND o.aggregate_version=1
		           AND o.payload->>'query_sha256'=j.query_sha256)=1
		  AND (SELECT count(*) FROM asset_export_outbox o
		         WHERE o.tenant_id=$1 AND o.job_id=$2::uuid
		           AND o.event_type='traffic.asset.export.v1.Completed'
		           AND o.aggregate_version=j.revision
		           AND o.payload->>'snapshot_id'=j.snapshot_id
		           AND o.payload->>'artifact_sha256'=j.artifact_sha256
		           AND o.payload->>'object_bucket'=j.object_bucket
		           AND o.payload->>'object_key'=j.object_key)=1
		  AND (SELECT count(*) FROM audit_logs a
		         WHERE a.tenant_id=$1 AND a.object_id=$2::text AND a.trace_id=$3
		           AND a.detail->>'trace_id'=$3
		           AND a.action IN ('ASSET_EXPORT_REQUESTED','ASSET_EXPORT_COMPLETED','ASSET_EXPORT_DOWNLOADED'))=3
		  AND (SELECT count(*) FROM audit_logs a
		         WHERE a.tenant_id=$1 AND a.object_id=$2::text AND a.trace_id=$3
		           AND a.action='ASSET_EXPORT_COMPLETED'
		           AND a.detail->>'snapshot_id'=j.snapshot_id
		           AND a.detail->>'artifact_sha256'=j.artifact_sha256
		           AND a.detail->>'object_key'=j.object_key)=1
		  AND (SELECT count(*) FROM audit_logs a
		         WHERE a.tenant_id=$1 AND a.object_id=$2::text AND a.trace_id=$3
		           AND a.action='ASSET_EXPORT_DOWNLOADED'
		           AND a.detail->>'artifact_sha256'=j.artifact_sha256
		           AND a.detail->>'object_key'=j.object_key)=1
		FROM asset_export_jobs j
		WHERE j.tenant_id=$1 AND j.job_id=$2::uuid`,
		assetExportMinIOIntegrationTenant, job.JobID, "trace-real-minio",
		completed.ArtifactSHA256, bucket, completed.ObjectKey, completed.SizeBytes,
	).Scan(&reconciled); err != nil {
		t.Fatal(err)
	}
	if !reconciled {
		t.Fatal("PostgreSQL job, outbox, audit, manifest and MinIO object did not reconcile")
	}
	if err := client.RemoveObject(context.Background(), completed.ObjectBucket, completed.ObjectKey, minio.RemoveObjectOptions{}); err != nil {
		t.Fatal(err)
	}
}

// This test proves the PostgreSQL outbox row is acknowledged only after a real
// broker accepts the keyed lifecycle event and that the published headers and
// payload can be consumed from that broker.
func TestAssetExportRealKafkaOutboxLifecycle(t *testing.T) {
	dsn := os.Getenv("ASSET_ATOMIC_EPHEMERAL_PG_DSN")
	broker := strings.TrimSpace(os.Getenv("ASSET_EXPORT_EPHEMERAL_KAFKA_BROKER"))
	topic := strings.TrimSpace(os.Getenv("ASSET_EXPORT_EPHEMERAL_KAFKA_TOPIC"))
	if dsn == "" || broker == "" || topic == "" {
		t.Skip("explicit ephemeral PostgreSQL and Kafka settings are required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	cleanup := func() {
		for _, table := range []string{"asset_export_outbox", "asset_export_jobs", "audit_logs", "tenants"} {
			if _, cleanupErr := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE tenant_id=$1", table), assetExportKafkaIntegrationTenant); cleanupErr != nil {
				t.Errorf("cleanup %s: %v", table, cleanupErr)
			}
		}
	}
	cleanup()
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Export Kafka Integration')`, assetExportKafkaIntegrationTenant); err != nil {
		t.Fatal(err)
	}

	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{Export: config.AssetExportConfig{Enabled: true}}, repo, zap.NewNop())
	job, err := svc.CreateAssetExportJob(context.Background(), assetExportKafkaIntegrationTenant, config.AssetExportRequest{
		ActionID: config.AssetExportActionID, Format: "csv",
		Columns: []string{"display_code"}, Reason: "verify real ephemeral Kafka outbox delivery",
	}, config.AssetExportCommand{
		IdempotencyKey: "asset-export-real-kafka-integration",
		Actor:          "integration-analyst", TraceID: "trace-real-kafka",
		RequestID: "request-real-kafka", ClientIP: "127.0.0.1", UserAgent: "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	producer, err := kafkaCommon.NewProducer(kafkaCommon.ProducerConfig{
		Brokers: []string{broker}, Topic: topic, BatchSize: 1,
		BatchTimeout: 10 * time.Millisecond, MaxAttempts: 3,
		RequiredAcks: "all", Compression: "none", Async: false,
		IdempotentKey: "tenant_id+job_id",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	reader := segmentKafka.NewReader(segmentKafka.ReaderConfig{
		Brokers: []string{broker}, Topic: topic, Partition: 0,
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: time.Second,
	})
	defer reader.Close()
	if err := reader.SetOffset(segmentKafka.FirstOffset); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := repository.NewAssetExportOutboxDispatcher(db, producer, repository.OutboxDispatcherConfig{
		WorkerID: "asset-export-real-kafka-dispatcher", Lease: 30 * time.Second,
		MaxAttempts: 3, BatchSize: 1, Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if found, err := dispatcher.DispatchNext(context.Background()); err != nil || !found {
		t.Fatalf("dispatch found=%v err=%v", found, err)
	}
	readCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wantKey := assetExportKafkaIntegrationTenant + ":" + job.JobID
	var message segmentKafka.Message
	for {
		message, err = reader.ReadMessage(readCtx)
		if err != nil {
			t.Fatal(err)
		}
		if string(message.Key) == wantKey {
			break
		}
	}
	if string(message.Key) != wantKey || !bytes.Contains(message.Value, []byte("traffic.asset.export.v1.Requested")) || !bytes.Contains(message.Value, []byte(job.JobID)) {
		t.Fatalf("Kafka message key=%q payload=%s", message.Key, message.Value)
	}
	headers := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		headers[header.Key] = string(header.Value)
	}
	if headers["event_type"] != "traffic.asset.export.v1.Requested" || headers["tenant_id"] != assetExportKafkaIntegrationTenant || headers["job_id"] != job.JobID || headers["trace_id"] != "trace-real-kafka" {
		t.Fatalf("Kafka headers=%v", headers)
	}
	var status string
	var publishedAt sql.NullTime
	if err := db.QueryRow(`SELECT status,published_at FROM asset_export_outbox WHERE tenant_id=$1 AND job_id=$2`, assetExportKafkaIntegrationTenant, job.JobID).Scan(&status, &publishedAt); err != nil {
		t.Fatal(err)
	}
	if status != "published" || !publishedAt.Valid {
		t.Fatalf("outbox status=%q published_at=%v", status, publishedAt)
	}
}

func cleanupAssetExportIntegration(db *sql.DB) error {
	for _, table := range []string{
		"asset_export_outbox", "asset_export_jobs", "asset_column_preferences",
		"audit_logs", "assets", "tenants",
	} {
		if _, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE tenant_id IN ($1,'asset-export-other')", table), assetExportIntegrationTenant); err != nil {
			return err
		}
	}
	return nil
}

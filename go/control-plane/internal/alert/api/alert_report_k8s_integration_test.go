package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

// TestAlertReportK8sPostgresMinIOIntegration mutates only one run-scoped
// tenant and bucket in the existing Kubernetes PostgreSQL/MinIO services. The
// explicit sentinel and exact service-name checks prevent accidental execution
// against developer or production endpoints.
func TestAlertReportK8sPostgresMinIOIntegration(t *testing.T) {
	if os.Getenv("ALERT_REPORT_K8S_INTEGRATION") != "run-scoped-only" {
		t.Skip("ALERT_REPORT_K8S_INTEGRATION=run-scoped-only is required")
	}
	pgHost := strings.TrimSpace(os.Getenv("ALERT_REPORT_K8S_PG_HOST"))
	pgPassword := os.Getenv("ALERT_REPORT_K8S_PG_PASSWORD")
	endpoint := strings.TrimSpace(os.Getenv("ALERT_REPORT_K8S_MINIO_ENDPOINT"))
	accessKey := os.Getenv("ALERT_REPORT_K8S_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("ALERT_REPORT_K8S_MINIO_SECRET_KEY")
	suffix := strings.ToLower(strings.TrimSpace(os.Getenv("ALERT_REPORT_K8S_SUFFIX")))
	if pgHost != "postgres-primary.databases.svc" {
		t.Fatalf("refusing PostgreSQL host %q", pgHost)
	}
	if endpoint != "minio.minio.svc:9000" {
		t.Fatalf("refusing MinIO endpoint %q", endpoint)
	}
	if len(suffix) < 8 || len(suffix) > 16 {
		t.Fatalf("invalid run-scoped suffix %q", suffix)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dsn := (&url.URL{
		Scheme: "postgres", User: url.UserPassword("postgres", pgPassword), Host: pgHost + ":5432",
		Path: "/traffic_platform", RawQuery: "sslmode=disable",
	}).String()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var unrelatedRunnable int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM alert_report_jobs
		WHERE status IN ('accepted','running','cancel_requested','compensating')`).Scan(&unrelatedRunnable); err != nil {
		t.Fatal(err)
	}
	if unrelatedRunnable != 0 {
		t.Fatalf("refusing shared queue with %d runnable alert report jobs", unrelatedRunnable)
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "codex-m09-n016-" + suffix
	tenantID := "m09-n016-" + suffix
	cleanup := func(cleanupCtx context.Context) {
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM alert_report_control_requests WHERE tenant_id=$1`, tenantID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM alert_report_outbox WHERE tenant_id=$1`, tenantID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM alert_report_job_history WHERE tenant_id=$1`, tenantID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM alert_report_jobs WHERE tenant_id=$1`, tenantID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM audit_logs WHERE tenant_id=$1`, tenantID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM tenants WHERE tenant_id=$1`, tenantID)
		for object := range client.ListObjects(cleanupCtx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err == nil {
				_ = client.RemoveObject(cleanupCtx, bucket, object.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(cleanupCtx, bucket)
	}
	defer cleanup(context.Background())
	cleanup(ctx)
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,tenant_name,name) VALUES($1,$2,$2)`, tenantID, "M09 N016 run-scoped tenant"); err != nil {
		t.Fatal(err)
	}

	store := &minioAlertReportObjectStore{client: client}
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetAlertReportObjectStore(store)
	handler.SetAlertReportArtifactTTL(time.Hour)
	t.Setenv("ALERT_REPORT_BUCKET", bucket)

	alertID := "AL-N016-" + suffix
	jobID := "alert-report-" + suffix
	model := reportModel()
	model.TenantID, model.AlertID, model.SnapshotID = tenantID, alertID, "alert:"+alertID+":revision:7"
	model.Alert.TenantID, model.Alert.AlertID = tenantID, alertID
	model.MissingSections = []string{"asset_context"}
	model.SourceWatermarks = map[string]string{
		"clickhouse.alerts.state_version": "7", "opensearch.alerts.projection_version": "os-17",
	}
	snapshot, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	snapshotDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(snapshot))
	insertAlertReportIntegrationJob(t, db, alertReportIntegrationSeed{
		JobID: jobID, TenantID: tenantID, AlertID: alertID, Format: "json", Status: "accepted", Revision: 1,
		Snapshot: snapshot, SnapshotID: model.SnapshotID, SnapshotSHA256: snapshotDigest,
		MissingSections: model.MissingSections, SourceWatermarks: model.SourceWatermarks,
	})
	if err := handler.processNextAlertReport(ctx); err != nil {
		t.Fatal(err)
	}
	completed, err := loadAlertReportJob(ctx, db, tenantID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.Revision != 3 || completed.ObjectKey == "" || completed.ArtifactSHA256 == "" || completed.CompletedAt == nil {
		t.Fatalf("completed manifest=%+v", completed)
	}
	object, err := client.GetObject(ctx, bucket, completed.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	objectBytes, err := io.ReadAll(object)
	_ = object.Close()
	if err != nil || int64(len(objectBytes)) != completed.SizeBytes ||
		fmt.Sprintf("sha256:%x", sha256.Sum256(objectBytes)) != completed.ArtifactSHA256 {
		t.Fatalf("manifest mismatch size=%d err=%v", len(objectBytes), err)
	}
	if err := store.Put(ctx, bucket, completed.ObjectKey, bytes.NewReader(objectBytes), int64(len(objectBytes)), completed.MIMEType); err != nil {
		t.Fatal(err)
	}
	if count := countMinIOObjects(t, ctx, client, bucket); count != 1 {
		t.Fatalf("same-key retry created %d objects", count)
	}
	var historyCount, outboxCount, auditCount int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM alert_report_job_history WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM alert_report_outbox WHERE tenant_id=$1 AND job_id=$2),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='alert_report' AND object_id=$2)`, tenantID, jobID).
		Scan(&historyCount, &outboxCount, &auditCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 3 || outboxCount != 3 || auditCount != 1 {
		t.Fatalf("history=%d outbox=%d audit=%d", historyCount, outboxCount, auditCount)
	}

	statusRequest := alertReportIntegrationRequest(http.MethodGet, tenantID, alertID, jobID, "")
	statusRecorder := httptest.NewRecorder()
	handler.GetAlertReportJob(statusRecorder, statusRequest)
	if statusRecorder.Code != http.StatusOK || !strings.Contains(statusRecorder.Body.String(), `"manifest_version":1`) ||
		!strings.Contains(statusRecorder.Body.String(), `"partial":true`) || !strings.Contains(statusRecorder.Body.String(), `"download_url"`) {
		t.Fatalf("status=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}

	handler.SetAlertReportArtifactTTL(time.Nanosecond)
	expiredRecorder := httptest.NewRecorder()
	handler.GetAlertReportJob(expiredRecorder, statusRequest)
	if expiredRecorder.Code != http.StatusOK || !strings.Contains(expiredRecorder.Body.String(), `"artifact_expired":true`) ||
		strings.Contains(expiredRecorder.Body.String(), `"download_url"`) {
		t.Fatalf("expired status=%d body=%s", expiredRecorder.Code, expiredRecorder.Body.String())
	}
	downloadRequest := alertReportIntegrationRequest(http.MethodGet, tenantID, alertID, jobID, "download")
	downloadRecorder := httptest.NewRecorder()
	handler.DownloadAlertReport(downloadRecorder, downloadRequest)
	if downloadRecorder.Code != http.StatusGone || !strings.Contains(downloadRecorder.Body.String(), "REPORT_EXPIRED") {
		t.Fatalf("expired download=%d body=%s", downloadRecorder.Code, downloadRecorder.Body.String())
	}

	cancelJobID := "alert-report-cancel-" + suffix
	cancelKey := tenantID + "/alerts/" + alertID + "/" + cancelJobID + ".json"
	cancelBytes := []byte("temporary-report-object")
	if _, err := client.PutObject(ctx, bucket, cancelKey, bytes.NewReader(cancelBytes), int64(len(cancelBytes)), minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	insertAlertReportIntegrationJob(t, db, alertReportIntegrationSeed{
		JobID: cancelJobID, TenantID: tenantID, AlertID: alertID, Format: "json", Status: "cancel_requested", Revision: 2,
		Snapshot: snapshot, SnapshotID: model.SnapshotID, SnapshotSHA256: snapshotDigest,
		ObjectBucket: bucket, ObjectKey: cancelKey, ArtifactSHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(cancelBytes)),
		SizeBytes: int64(len(cancelBytes)), MIMEType: "application/json", CancellationReason: "run-scoped cancellation",
	})
	if err := handler.processNextAlertReport(ctx); err != nil {
		t.Fatal(err)
	}
	cancelled, err := loadAlertReportJob(ctx, db, tenantID, cancelJobID)
	if err != nil || cancelled.Status != "cancelled" || cancelled.ObjectKey != "" || cancelled.Revision != 3 {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	if _, err := client.StatObject(ctx, bucket, cancelKey, minio.StatObjectOptions{}); err == nil {
		t.Fatal("cancelled job retained its exact temporary object")
	}

	otherTenantRequest := alertReportIntegrationRequest(http.MethodGet, "other-tenant", alertID, jobID, "")
	otherTenantRecorder := httptest.NewRecorder()
	handler.GetAlertReportJob(otherTenantRecorder, otherTenantRequest)
	if otherTenantRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status=%d body=%s", otherTenantRecorder.Code, otherTenantRecorder.Body.String())
	}
	t.Logf("alert_report_k8s=pass tenant=%s bucket=%s job=%s", tenantID, bucket, jobID)
}

func TestAlertReportK8sCleanupOracle(t *testing.T) {
	if os.Getenv("ALERT_REPORT_K8S_INTEGRATION") != "run-scoped-only" {
		t.Skip("ALERT_REPORT_K8S_INTEGRATION=run-scoped-only is required")
	}
	pgHost := strings.TrimSpace(os.Getenv("ALERT_REPORT_K8S_PG_HOST"))
	endpoint := strings.TrimSpace(os.Getenv("ALERT_REPORT_K8S_MINIO_ENDPOINT"))
	suffix := strings.ToLower(strings.TrimSpace(os.Getenv("ALERT_REPORT_K8S_SUFFIX")))
	if pgHost != "postgres-primary.databases.svc" || endpoint != "minio.minio.svc:9000" || len(suffix) < 8 || len(suffix) > 16 {
		t.Fatal("refusing cleanup oracle without exact K8s endpoints and suffix")
	}
	dsn := (&url.URL{
		Scheme: "postgres", User: url.UserPassword("postgres", os.Getenv("ALERT_REPORT_K8S_PG_PASSWORD")),
		Host: pgHost + ":5432", Path: "/traffic_platform", RawQuery: "sslmode=disable",
	}).String()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantID := "m09-n016-" + suffix
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM tenants WHERE tenant_id=$1)+
		(SELECT count(*) FROM alert_report_jobs WHERE tenant_id=$1)+
		(SELECT count(*) FROM alert_report_outbox WHERE tenant_id=$1)+
		(SELECT count(*) FROM alert_report_job_history WHERE tenant_id=$1)+
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1)`, tenantID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("ALERT_REPORT_K8S_MINIO_ACCESS_KEY"), os.Getenv("ALERT_REPORT_K8S_MINIO_SECRET_KEY"), ""),
		Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	bucketExists, err := client.BucketExists(ctx, "codex-m09-n016-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 || bucketExists {
		t.Fatalf("run-scoped cleanup incomplete: postgres_rows=%d bucket_exists=%t", rows, bucketExists)
	}
	t.Log("alert_report_k8s_cleanup=pass")
}

type alertReportIntegrationSeed struct {
	JobID, TenantID, AlertID, Format, Status, SnapshotID, SnapshotSHA256  string
	ObjectBucket, ObjectKey, MIMEType, ArtifactSHA256, CancellationReason string
	Revision, SizeBytes                                                   int64
	Snapshot                                                              []byte
	MissingSections                                                       []string
	SourceWatermarks                                                      map[string]string
}

func insertAlertReportIntegrationJob(t *testing.T, db *sql.DB, seed alertReportIntegrationSeed) {
	t.Helper()
	missing, _ := json.Marshal(seed.MissingSections)
	watermarks, _ := json.Marshal(seed.SourceWatermarks)
	_, err := db.Exec(`INSERT INTO alert_report_jobs
		(job_id,tenant_id,alert_id,format,status,revision,idempotency_key,requested_snapshot_id,snapshot,snapshot_sha256,
		 missing_sections,source_watermarks,object_bucket,object_key,mime_type,artifact_sha256,size_bytes,cancellation_reason,
		 next_attempt_at,created_by,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11::jsonb,$12::jsonb,$13,$14,$15,$16,$17,$18,now()-interval '1 minute','k8s-integration','1900-01-01',now())`,
		seed.JobID, seed.TenantID, seed.AlertID, seed.Format, seed.Status, seed.Revision, "idempotency-"+seed.JobID,
		seed.SnapshotID, string(seed.Snapshot), seed.SnapshotSHA256, string(missing), string(watermarks), seed.ObjectBucket,
		seed.ObjectKey, seed.MIMEType, seed.ArtifactSHA256, seed.SizeBytes, seed.CancellationReason)
	if err != nil {
		t.Fatal(err)
	}
	for revision := int64(1); revision <= seed.Revision; revision++ {
		toStatus := "accepted"
		fromStatus := ""
		if revision == seed.Revision && seed.Status != "accepted" {
			fromStatus, toStatus = "accepted", seed.Status
		}
		if _, err := db.Exec(`INSERT INTO alert_report_job_history
			(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
			VALUES($1,$2,$3,$4,$5,'k8s-integration','run-scoped seed','','{}'::jsonb)`,
			seed.JobID, seed.TenantID, fromStatus, toStatus, revision); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO alert_report_outbox
			(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
			VALUES($1::uuid,$2,$3,'traffic.alert.v2.AlertReportRequested',$4,$5,$6::jsonb)`, uuid.NewString(),
			seed.JobID, seed.TenantID, revision, seed.TenantID+":"+seed.AlertID,
			fmt.Sprintf(`{"event_id":%q,"aggregate_version":%d}`, uuid.NewString(), revision)); err != nil {
			t.Fatal(err)
		}
	}
}

func alertReportIntegrationRequest(method, tenantID, alertID, jobID, action string) *http.Request {
	path := "/api/v1/alerts/" + alertID + "/reports/" + jobID
	if action != "" {
		path += "/" + action
	}
	request := httptest.NewRequest(method, path, nil)
	request = withTenant(request, tenantID)
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertExport}))
	return mux.SetURLVars(request, map[string]string{"id": alertID, "job_id": jobID})
}

func countMinIOObjects(t *testing.T, ctx context.Context, client *minio.Client, bucket string) int {
	t.Helper()
	count := 0
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if object.Err != nil {
			t.Fatal(object.Err)
		}
		count++
	}
	return count
}

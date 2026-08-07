package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

type failingCampaignReportObjectStore struct{ err error }

func (s failingCampaignReportObjectStore) Put(context.Context, string, string, io.Reader, int64, string) error {
	return s.err
}

func (s failingCampaignReportObjectStore) Open(context.Context, string, string) (io.ReadCloser, error) {
	return nil, s.err
}

func (s failingCampaignReportObjectStore) Remove(context.Context, string, string) error {
	return s.err
}

// TestCampaignReportExecutorPostgresMinIOIntegration requires two independent
// sentinel proofs. Merely setting a DSN or S3 endpoint never authorizes writes.
func TestCampaignReportExecutorPostgresMinIOIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN"))
	endpoint := strings.TrimSpace(os.Getenv("CAMPAIGN_REPORT_EPHEMERAL_MINIO_ENDPOINT"))
	accessKey := os.Getenv("CAMPAIGN_REPORT_EPHEMERAL_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("CAMPAIGN_REPORT_EPHEMERAL_MINIO_SECRET_KEY")
	bucketPrefix := strings.TrimSpace(os.Getenv("CAMPAIGN_REPORT_EPHEMERAL_MINIO_BUCKET_PREFIX"))
	if dsn == "" || endpoint == "" || accessKey == "" || secretKey == "" || bucketPrefix == "" {
		t.Skip("explicit ephemeral PostgreSQL and MinIO settings are required")
	}
	if !strings.HasPrefix(bucketPrefix, "codex-ephemeral-") {
		t.Fatalf("refusing MinIO bucket prefix without codex-ephemeral- guard: %q", bucketPrefix)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var pgSentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_campaign_aggregate_sentinel LIMIT 1`).Scan(&pgSentinel); err != nil || pgSentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", pgSentinel, err)
	}

	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	client, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sentinelObject, err := client.GetObject(ctx, "codex-ephemeral-campaign-report-sentinel", "ephemeral-only", minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sentinelBytes, sentinelErr := io.ReadAll(sentinelObject)
	_ = sentinelObject.Close()
	if sentinelErr != nil || string(sentinelBytes) != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel MinIO: marker=%q err=%v", string(sentinelBytes), sentinelErr)
	}

	suffix := strings.ToLower(strings.ReplaceAll(uuid.NewString()[:12], "-", ""))
	bucket := strings.TrimSuffix(bucketPrefix, "-") + "-" + suffix
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		objects := client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true})
		for object := range objects {
			if object.Err == nil {
				_ = client.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{})
			}
		}
		if err := client.RemoveBucket(ctx, bucket); err != nil {
			t.Errorf("remove MinIO bucket: %v", err)
		}
	}()

	tenantID := "campaign-report-it-" + time.Now().UTC().Format("150405000000")
	campaignID := "campaign-report-integration-1"
	cleanupCampaignAggregateIntegration(t, db, tenantID)
	defer cleanupCampaignAggregateIntegration(t, db, tenantID)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Report Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaign_alert_links
		(relation_id,tenant_id,campaign_id,alert_id,status,revision,reason,idempotency_key,created_by,updated_by)
		VALUES($1,$2,$3,'alert-report-1','linked',1,'report integration member',$4,'seed','seed')`,
		uuid.NewString(), tenantID, campaignID, "campaign-report-member-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	handler := NewSystemHandler(nil, db, zap.NewNop())
	handler.SetCampaignAggregateV2FeatureFlag(true)
	handler.SetCampaignReportObjectStore(&minioAlertReportObjectStore{client: client})
	t.Setenv("CAMPAIGN_REPORT_BUCKET", bucket)
	requestCtx := context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	requestCtx = context.WithValue(requestCtx, httpx.ContextKeyUserID, "campaign-report-operator")
	requestCtx = context.WithValue(requestCtx, httpx.ContextKeyTraceID, "trace-campaign-report-integration")
	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/"+campaignID+"/actions", nil).WithContext(requestCtx)
	campaign := campaignDTO{
		TenantID: tenantID, CampaignID: campaignID, Status: "active", TsStart: 100, TsEnd: 200,
		Alerts: []string{"alert-report-1"}, Summary: "integration report", Score: 88.5,
		CampaignType: "apt", EventID: "event-campaign-report", IngestTs: 12345,
	}
	initialLifecycle, err := handler.loadCampaignLifecycleRead(requestCtx, campaign)
	if err != nil {
		t.Fatal(err)
	}
	if initialLifecycle.Campaign.StateVersion != 0 || initialLifecycle.ActualMemberCount != 1 ||
		len(initialLifecycle.MemberAlertIDs) != 1 || initialLifecycle.MemberAlertIDs[0] != "alert-report-1" {
		t.Fatalf("initial lifecycle=%+v", initialLifecycle)
	}
	sourceSnapshotID := initialLifecycle.Campaign.SnapshotID
	reportRequest := campaignActionRequest{
		ActionID: "campaign-report-generate", Target: "战役复盘报告",
		Metadata: map[string]interface{}{
			"campaign_id": campaignID, "format": "pdf", "sections": []string{"证据链"},
			"evidence_count": float64(1), "dry_run": false, "snapshot_id": sourceSnapshotID,
		},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(0),
		Reason: "真实PostgreSQL与MinIO战役报告集成验证",
	}
	requestSHA, err := campaignCommandRequestSHA(campaignID, reportRequest)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := handler.commitCampaignAggregateV2Command(requestCtx, httpRequest, reportRequest,
		campaignActionSpecs[reportRequest.ActionID], campaignID, campaign, "campaign-report-integration-key", requestSHA)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	if accepted.Result["source_snapshot_id"] != sourceSnapshotID {
		t.Fatalf("accepted source snapshot=%v want=%s", accepted.Result["source_snapshot_id"], sourceSnapshotID)
	}
	reportID, _ := accepted.Result["report_id"].(string)
	if reportID == "" {
		t.Fatalf("accepted result lacks report_id: %+v", accepted.Result)
	}
	acceptedLifecycle, err := handler.loadCampaignLifecycleRead(requestCtx, campaign)
	if err != nil {
		t.Fatal(err)
	}
	if acceptedLifecycle.Campaign.StateVersion != 1 || acceptedLifecycle.ActualMemberCount != 1 ||
		len(acceptedLifecycle.Reports) != 1 ||
		acceptedLifecycle.Reports[0].CampaignRevision != 1 ||
		acceptedLifecycle.Reports[0].SourceSnapshotID != sourceSnapshotID {
		t.Fatalf("accepted lifecycle=%+v source_snapshot=%s", acceptedLifecycle, sourceSnapshotID)
	}
	if err := handler.processNextCampaignReport(requestCtx); err != nil {
		t.Fatal(err)
	}
	completed, err := handler.loadCampaignReport(requestCtx, tenantID, campaignID, reportID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.ArtifactSHA256 == "" || completed.SizeBytes <= 0 || completed.ObjectKey == "" {
		t.Fatalf("completed=%+v", completed)
	}
	object, err := client.GetObject(ctx, bucket, completed.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	objectBytes, err := io.ReadAll(object)
	_ = object.Close()
	if err != nil || int64(len(objectBytes)) != completed.SizeBytes ||
		"sha256:"+hex.EncodeToString(sha256Sum(objectBytes)) != completed.ArtifactSHA256 {
		t.Fatalf("object manifest mismatch bytes=%d err=%v", len(objectBytes), err)
	}
	var stateRevision int64
	var reportStatus, actionStatus string
	var historyCount, outboxCount, auditCount int
	if err := db.QueryRow(`SELECT s.state_version,r.status,j.status,
		(SELECT count(*) FROM campaign_aggregate_history WHERE tenant_id=$1),
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='campaign_report')
		FROM campaign_workbench_state s JOIN campaign_reports r ON r.tenant_id=s.tenant_id AND r.campaign_id=s.campaign_id
		JOIN campaign_action_jobs j ON j.job_id=r.job_id
		WHERE s.tenant_id=$1 AND s.campaign_id=$2 AND r.report_id=$3`, tenantID, campaignID, reportID).
		Scan(&stateRevision, &reportStatus, &actionStatus, &historyCount, &outboxCount, &auditCount); err != nil {
		t.Fatal(err)
	}
	if stateRevision != 2 || reportStatus != "completed" || actionStatus != "completed" || historyCount != 2 || outboxCount != 2 || auditCount != 1 {
		t.Fatalf("state=%d report=%s action=%s history=%d outbox=%d audit=%d", stateRevision, reportStatus, actionStatus, historyCount, outboxCount, auditCount)
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/reports/"+reportID, nil)
	statusRequest = statusRequest.WithContext(campaignReportRequestContext(tenantID, authmodel.ScopeCampaignRead))
	statusRequest = mux.SetURLVars(statusRequest, map[string]string{"id": campaignID, "report_id": reportID})
	statusRecorder := httptest.NewRecorder()
	handler.GetCampaignReport(statusRecorder, statusRequest)
	if statusRecorder.Code != http.StatusOK || !bytes.Contains(statusRecorder.Body.Bytes(), []byte(`"snapshot_id"`)) {
		t.Fatalf("status=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}
	otherTenantRequest := statusRequest.WithContext(campaignReportRequestContext("other-tenant", authmodel.ScopeCampaignRead))
	otherTenantRecorder := httptest.NewRecorder()
	handler.GetCampaignReport(otherTenantRecorder, otherTenantRequest)
	if otherTenantRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status=%d body=%s", otherTenantRecorder.Code, otherTenantRecorder.Body.String())
	}

	downloadRequest := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/reports/"+reportID+"/download", nil)
	downloadRequest = downloadRequest.WithContext(campaignReportRequestContext(tenantID, authmodel.ScopeCampaignReport))
	downloadRequest = mux.SetURLVars(downloadRequest, map[string]string{"id": campaignID, "report_id": reportID})
	downloadRecorder := httptest.NewRecorder()
	handler.DownloadCampaignReport(downloadRecorder, downloadRequest)
	if downloadRecorder.Code != http.StatusOK || !bytes.Equal(downloadRecorder.Body.Bytes(), objectBytes) {
		t.Fatalf("download=%d bytes=%d", downloadRecorder.Code, downloadRecorder.Body.Len())
	}
	if _, err := client.PutObject(ctx, bucket, completed.ObjectKey, strings.NewReader("tampered"), int64(len("tampered")), minio.PutObjectOptions{ContentType: completed.MIMEType}); err != nil {
		t.Fatal(err)
	}
	tamperedRecorder := httptest.NewRecorder()
	handler.DownloadCampaignReport(tamperedRecorder, downloadRequest)
	if tamperedRecorder.Code != http.StatusBadGateway {
		t.Fatalf("tampered download=%d body=%s", tamperedRecorder.Code, tamperedRecorder.Body.String())
	}

	secondRequest := reportRequest
	secondRequest.Metadata = map[string]interface{}{"campaign_id": campaignID, "format": "json", "sections": []string{"摘要"}, "evidence_count": float64(0), "dry_run": false}
	secondRequest.ExpectedRevision = int64Pointer(2)
	secondRequest.Reason = "对象写入失败必须保持可重试而非伪成功"
	secondSHA, _ := campaignCommandRequestSHA(campaignID, secondRequest)
	second, err := handler.commitCampaignAggregateV2Command(requestCtx, httpRequest, secondRequest,
		campaignActionSpecs[secondRequest.ActionID], campaignID, campaign, "campaign-report-retry-integration-key", secondSHA)
	if err != nil {
		t.Fatal(err)
	}
	handler.SetCampaignReportObjectStore(failingCampaignReportObjectStore{err: errors.New("sentinel object write failure")})
	if err := handler.processNextCampaignReport(requestCtx); err == nil {
		t.Fatal("expected object write failure")
	}
	secondReportID, _ := second.Result["report_id"].(string)
	var retryStatus, retryActionStatus string
	var attempts int
	if err := db.QueryRow(`SELECT r.status,r.attempts,j.status FROM campaign_reports r JOIN campaign_action_jobs j ON j.job_id=r.job_id WHERE r.report_id=$1`, secondReportID).
		Scan(&retryStatus, &attempts, &retryActionStatus); err != nil {
		t.Fatal(err)
	}
	if retryStatus != "accepted" || attempts != 1 || retryActionStatus != "accepted" {
		t.Fatalf("retry report=%s attempts=%d action=%s", retryStatus, attempts, retryActionStatus)
	}
	if _, err := db.Exec(`UPDATE campaign_reports SET attempts=4,next_attempt_at=now() WHERE report_id=$1 AND status='accepted'`, secondReportID); err != nil {
		t.Fatal(err)
	}
	if err := handler.processNextCampaignReport(requestCtx); err == nil {
		t.Fatal("expected terminal object write failure")
	}
	var failedReportStatus, failedActionStatus, failedError, failedEventType, failedAuditResult string
	var failedAttempts int
	var failedStateRevision int64
	if err := db.QueryRow(`SELECT r.status,r.attempts,r.error_message,j.status,s.state_version,
		(SELECT event_type FROM campaign_aggregate_history WHERE tenant_id=$1 AND campaign_id=$2 ORDER BY aggregate_revision DESC LIMIT 1),
		(SELECT action||':'||(detail->>'result') FROM audit_logs WHERE tenant_id=$1 AND object_type='campaign_report' AND object_id=$3 ORDER BY created_at DESC LIMIT 1)
		FROM campaign_reports r JOIN campaign_action_jobs j ON j.job_id=r.job_id
		JOIN campaign_workbench_state s ON s.tenant_id=r.tenant_id AND s.campaign_id=r.campaign_id
		WHERE r.tenant_id=$1 AND r.campaign_id=$2 AND r.report_id=$3`, tenantID, campaignID, secondReportID).
		Scan(&failedReportStatus, &failedAttempts, &failedError, &failedActionStatus, &failedStateRevision, &failedEventType, &failedAuditResult); err != nil {
		t.Fatal(err)
	}
	if failedReportStatus != "failed" || failedAttempts != campaignReportMaxAttempts ||
		failedActionStatus != "failed" || failedStateRevision != 4 ||
		failedEventType != "traffic.campaign.v2.ReportFailed" || failedAuditResult != "CAMPAIGN_REPORT_FAILED:failed" ||
		!strings.Contains(failedError, "sentinel object write failure") {
		t.Fatalf("terminal report=%s attempts=%d action=%s revision=%d event=%s audit=%s error=%q",
			failedReportStatus, failedAttempts, failedActionStatus, failedStateRevision, failedEventType, failedAuditResult, failedError)
	}
	failedStatusRequest := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/reports/"+secondReportID, nil)
	failedStatusRequest = failedStatusRequest.WithContext(campaignReportRequestContext(tenantID, authmodel.ScopeCampaignRead))
	failedStatusRequest = mux.SetURLVars(failedStatusRequest, map[string]string{"id": campaignID, "report_id": secondReportID})
	failedStatusRecorder := httptest.NewRecorder()
	handler.GetCampaignReport(failedStatusRecorder, failedStatusRequest)
	if failedStatusRecorder.Code != http.StatusOK || !bytes.Contains(failedStatusRecorder.Body.Bytes(), []byte(`"status":"failed"`)) {
		t.Fatalf("failed status=%d body=%s", failedStatusRecorder.Code, failedStatusRecorder.Body.String())
	}
}

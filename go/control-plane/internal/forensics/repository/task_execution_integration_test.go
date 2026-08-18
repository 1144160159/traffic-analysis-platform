package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

func TestForensicsVersionedExecutionEphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("FORENSICS_TASK_ATOMIC_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("FORENSICS_TASK_ATOMIC_EPHEMERAL_PG_DSN is not set")
	}
	client := forensicsIntegrationClient(t, dsn)
	defer client.Close()
	db := client.DB()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_forensics_task_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantID := "forensics-execution-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "Forensics Execution Integration"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, statement := range []string{
			`DELETE FROM forensics_job_manifests WHERE tenant_id=$1`,
			`DELETE FROM forensics_task_checkpoints WHERE tenant_id=$1`,
		} {
			if _, err := db.Exec(statement, tenantID); err != nil {
				t.Errorf("cleanup versioned execution: %v", err)
			}
		}
		cleanupForensicsTaskFixture(t, db, tenantID)
	}()
	repo := NewTaskRepository(client, zap.NewNop())
	if err := repo.VerifyVersionedExecutionSchema(ctx); err != nil {
		t.Fatal(err)
	}
	task := integrationTask(t, tenantID, "versioned-execution")
	if _, err := repo.CreateAtomic(ctx, task, integrationTaskMeta(tenantID, ForensicsTaskCreateAction, "forensics-versioned-create-0001", 0)); err != nil {
		t.Fatal(err)
	}
	leased, err := repo.GetPendingTasks(ctx, 1)
	if err != nil || len(leased) != 1 || leased[0].TaskID != task.TaskID {
		t.Fatalf("leased=%+v err=%v", leased, err)
	}
	claim, err := repo.ClaimVersionedExecution(ctx, leased[0], "worker-a", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimVersionedExecution(ctx, leased[0], "worker-b", 2*time.Minute); !errors.Is(err, ErrTaskExecutionLeaseUnavailable) {
		t.Fatalf("concurrent worker claim = %v", err)
	}
	if err := repo.AdvanceVersionedExecution(ctx, claim, "reading_source", map[string]any{"pcap_index_id": strings.Repeat("a", 64)}, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	resultObject := s3client.ObjectAuthority{
		Bucket: "forensics-results", Key: "tenants/" + tenantID + "/forensics/jobs/" + task.TaskID + "/pcap/result.pcap",
		VersionID: "version-1", ETag: "etag-1", SizeBytes: 128, SHA256: strings.Repeat("b", 64),
		ObservedAt: time.Now().UTC(), RetentionUntil: time.Now().UTC().Add(24 * time.Hour),
	}
	manifestJSON, err := json.Marshal(map[string]any{
		"manifest_version": 1, "tenant_id": tenantID, "task_id": task.TaskID,
		"status": "completed", "result_object": resultObject, "executable": false, "automatic_open": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifestJSON)
	manifest := VersionedTaskManifest{
		TenantID: tenantID, TaskID: task.TaskID, ManifestSHA256: hex.EncodeToString(digest[:]),
		ManifestJSON: manifestJSON, Status: TaskStatusCompleted, ResultObject: resultObject,
	}
	if err := repo.CompleteVersionedExecution(ctx, claim, manifest, 3, 128, 1); err != nil {
		t.Fatal(err)
	}
	// Exact replay is read-only and cannot create a second manifest/history fact.
	if err := repo.CompleteVersionedExecution(ctx, claim, manifest, 3, 128, 1); err != nil {
		t.Fatalf("exact completion replay failed: %v", err)
	}
	completed, err := repo.GetByIDForTenant(ctx, tenantID, task.TaskID)
	if err != nil || completed.Status != TaskStatusCompleted || completed.ResultSHA256 != resultObject.SHA256 {
		t.Fatalf("completed task=%+v err=%v", completed, err)
	}
	receipt, err := repo.GetVersionedManifestForTenant(ctx, tenantID, task.TaskID)
	if err != nil || receipt == nil || receipt.ManifestSHA256 != manifest.ManifestSHA256 || receipt.ObjectVersion != resultObject.VersionID {
		t.Fatalf("manifest receipt=%+v err=%v", receipt, err)
	}
	byResultKey, err := repo.GetVersionedManifestByResultKey(ctx, tenantID, resultObject.Key)
	if err != nil || byResultKey == nil || byResultKey.TaskID != task.TaskID ||
		byResultKey.ResultObject.Key != resultObject.Key || byResultKey.ResultObject.VersionID != resultObject.VersionID ||
		byResultKey.ResultObject.SHA256 != resultObject.SHA256 {
		t.Fatalf("result-key manifest receipt=%+v err=%v", byResultKey, err)
	}
	if crossTenant, err := repo.GetVersionedManifestByResultKey(ctx, "tenant-other", resultObject.Key); err != nil || crossTenant != nil {
		t.Fatalf("cross-tenant result-key manifest receipt=%+v err=%v", crossTenant, err)
	}
	var manifests, checkpoints, completions int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM forensics_job_manifests WHERE tenant_id=$1),
		(SELECT count(*) FROM forensics_task_checkpoints WHERE tenant_id=$1 AND phase='completed'),
		(SELECT count(*) FROM forensics_task_history WHERE tenant_id=$1 AND task_id=$2 AND operation='complete')`, tenantID, task.TaskID).
		Scan(&manifests, &checkpoints, &completions); err != nil {
		t.Fatal(err)
	}
	if manifests != 1 || checkpoints != 1 || completions != 1 {
		t.Fatalf("non-idempotent final facts manifests=%d checkpoints=%d completions=%d", manifests, checkpoints, completions)
	}
}

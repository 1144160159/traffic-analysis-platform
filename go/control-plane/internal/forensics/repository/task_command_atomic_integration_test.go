package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

func TestForensicsTaskCommandAtomicEphemeralPostgres(t *testing.T) {
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
	tenantID := "forensics-task-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "Forensics Task Integration"); err != nil {
		t.Fatal(err)
	}
	defer cleanupForensicsTaskFixture(t, db, tenantID)
	repo := NewTaskRepository(client, zap.NewNop())

	taskA := integrationTask(t, tenantID, "cancel-and-archive")
	createA := integrationTaskMeta(tenantID, ForensicsTaskCreateAction, "forensics-create-a-00000001", 0)
	createdA, err := repo.CreateAtomic(ctx, taskA, createA)
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	if createdA.Revision != 1 || createdA.Replayed || taskA.TaskID == "" || taskA.TaskID != createdA.TaskID {
		t.Fatalf("unexpected create A receipt=%+v task=%+v", createdA, taskA)
	}
	replayTask := integrationTask(t, tenantID, "cancel-and-archive")
	replayA, err := repo.CreateAtomic(ctx, replayTask, createA)
	if err != nil || !replayA.Replayed || replayA.EventID != createdA.EventID || replayA.TaskID != taskA.TaskID {
		t.Fatalf("create replay=%+v err=%v", replayA, err)
	}
	collisionTask := integrationTask(t, tenantID, "different-payload")
	if _, err := repo.CreateAtomic(ctx, collisionTask, createA); !commonerrors.IsCode(err, commonerrors.ErrCodeDedupConflict) {
		t.Fatalf("expected create idempotency conflict, got %v", err)
	}
	crossMeta := integrationTaskMeta(tenantID+"-other", ForensicsTaskCancelAction, "forensics-cross-tenant-00001", 1)
	if _, err := repo.CancelForTenant(ctx, tenantID+"-other", taskA.TaskID, crossMeta); !commonerrors.IsCode(err, commonerrors.ErrCodeResourceNotFound) {
		t.Fatalf("expected tenant-bound not found, got %v", err)
	}
	cancelA := integrationTaskMeta(tenantID, ForensicsTaskCancelAction, "forensics-cancel-a-00000001", 1)
	cancelledA, err := repo.CancelForTenant(ctx, tenantID, taskA.TaskID, cancelA)
	if err != nil || cancelledA.Status != TaskStatusCancelled || cancelledA.Revision != 2 {
		t.Fatalf("cancel A=%+v err=%v", cancelledA, err)
	}
	cancelReplay, err := repo.CancelForTenant(ctx, tenantID, taskA.TaskID, cancelA)
	if err != nil || !cancelReplay.Replayed || cancelReplay.EventID != cancelledA.EventID {
		t.Fatalf("cancel replay=%+v err=%v", cancelReplay, err)
	}
	staleCancel := integrationTaskMeta(tenantID, ForensicsTaskCancelAction, "forensics-cancel-stale-00001", 1)
	if _, err := repo.CancelForTenant(ctx, tenantID, taskA.TaskID, staleCancel); !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET completed_at=now()-interval '2 hours' WHERE tenant_id=$1 AND task_id=$2`, tenantID, taskA.TaskID); err != nil {
		t.Fatal(err)
	}
	archived, err := repo.CleanupOldTasks(ctx, time.Hour)
	if err != nil || archived != 1 {
		t.Fatalf("archive count=%d err=%v", archived, err)
	}
	if _, err := repo.GetByIDForTenant(ctx, tenantID, taskA.TaskID); !commonerrors.IsCode(err, commonerrors.ErrCodeResourceNotFound) {
		t.Fatalf("soft archived task remained visible: %v", err)
	}

	taskB := integrationTask(t, tenantID, "lease-progress-complete")
	if _, err := repo.CreateAtomic(ctx, taskB, integrationTaskMeta(tenantID, ForensicsTaskCreateAction, "forensics-create-b-00000001", 0)); err != nil {
		t.Fatal(err)
	}
	leased, err := repo.GetPendingTasks(ctx, 10)
	if err != nil || len(leased) != 1 || leased[0].TaskID != taskB.TaskID || leased[0].Status != TaskStatusProcessing || leased[0].Revision != 2 {
		t.Fatalf("leased=%+v err=%v", leased, err)
	}
	if err := repo.UpdateProgress(ctx, taskB.TaskID, 42, 123); err != nil {
		t.Fatal(err)
	}
	if err := repo.Complete(ctx, taskB.TaskID, "results/"+tenantID+"/b.pcap", strings.Repeat("a", 64), 123, 4567, 2); err != nil {
		t.Fatal(err)
	}
	completedB, err := repo.GetByIDForTenant(ctx, tenantID, taskB.TaskID)
	if err != nil || completedB.Status != TaskStatusCompleted || completedB.Revision != 4 || completedB.Progress != 100 {
		t.Fatalf("completed B=%+v err=%v", completedB, err)
	}

	taskC := integrationTask(t, tenantID, "status-fail")
	if _, err := repo.CreateAtomic(ctx, taskC, integrationTaskMeta(tenantID, ForensicsTaskCreateAction, "forensics-create-c-00000001", 0)); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, taskC.TaskID, TaskStatusProcessing); err != nil {
		t.Fatal(err)
	}
	if err := repo.Fail(ctx, taskC.TaskID, "integration failure"); err != nil {
		t.Fatal(err)
	}

	taskD := integrationTask(t, tenantID, "status-recover")
	if _, err := repo.CreateAtomic(ctx, taskD, integrationTaskMeta(tenantID, ForensicsTaskCreateAction, "forensics-create-d-00000001", 0)); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, taskD.TaskID, TaskStatusProcessing); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET updated_at=now()-interval '2 hours' WHERE tenant_id=$1 AND task_id=$2`, tenantID, taskD.TaskID); err != nil {
		t.Fatal(err)
	}
	recovered, err := repo.ResetStuckTasks(ctx, time.Hour)
	if err != nil || recovered != 1 {
		t.Fatalf("recover count=%d err=%v", recovered, err)
	}
	recoveredD, err := repo.GetByIDForTenant(ctx, tenantID, taskD.TaskID)
	if err != nil || recoveredD.Status != TaskStatusQueued || recoveredD.Revision != 3 {
		t.Fatalf("recovered D=%+v err=%v", recoveredD, err)
	}

	var tasks, archivedTasks, history, outbox, requests, audits int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM tasks WHERE tenant_id=$1),
		(SELECT count(*) FROM tasks WHERE tenant_id=$1 AND deleted_at IS NOT NULL),
		(SELECT count(*) FROM forensics_task_history WHERE tenant_id=$1),
		(SELECT count(*) FROM forensics_task_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM forensics_task_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='pcap_task')`, tenantID).
		Scan(&tasks, &archivedTasks, &history, &outbox, &requests, &audits); err != nil {
		t.Fatal(err)
	}
	if tasks != 4 || archivedTasks != 1 || history != 13 || outbox != 13 || requests != 13 || audits != 13 {
		t.Fatalf("atomic counts tasks=%d archived=%d history=%d outbox=%d requests=%d audits=%d",
			tasks, archivedTasks, history, outbox, requests, audits)
	}
}

func integrationTask(t *testing.T, tenantID, marker string) *Task {
	t.Helper()
	params, err := json.Marshal(map[string]interface{}{
		"tenant_id": tenantID, "marker": marker, "start_time": int64(1), "end_time": int64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Task{TenantID: tenantID, TaskType: TaskTypePcapCut, Status: TaskStatusQueued, ParamsJSON: params, CreatedBy: "integration-user"}
}

func integrationTaskMeta(tenantID, actionID, key string, revision int64) TaskCommandMeta {
	return TaskCommandMeta{
		TenantID: tenantID, ActorID: "integration-user", ActionID: actionID,
		IdempotencyKey: key, ExpectedRevision: &revision, Reason: "integration verification",
		TraceID: "trace-" + key, SourceIP: "127.0.0.1", UserAgent: "forensics-integration",
	}
}

func forensicsIntegrationClient(t *testing.T, dsn string) *storage.PostgresClient {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	client, err := storage.NewPostgresClient(storage.PostgresConfig{
		Host: parsed.Hostname(), Port: port, Database: strings.TrimPrefix(parsed.Path, "/"),
		Username: parsed.User.Username(), Password: password, SSLMode: "disable",
		MaxOpenConns: 5, MaxIdleConns: 2, ConnectTimeout: 5, SlowQueryThreshold: time.Second,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func cleanupForensicsTaskFixture(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	for _, statement := range []string{
		`DELETE FROM forensics_task_requests WHERE tenant_id=$1`,
		`DELETE FROM forensics_task_history WHERE tenant_id=$1`,
		`DELETE FROM forensics_task_outbox WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type='pcap_task'`,
		`DELETE FROM tasks WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, tenantID); err != nil {
			t.Errorf("cleanup forensics task fixture: %v", err)
		}
	}
}

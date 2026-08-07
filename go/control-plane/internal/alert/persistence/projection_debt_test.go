package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProjectionDebtBatchCommitsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").
		WithArgs("tenant-a", "alert-a", "event-a", sqlmock.AnyArg(), sqlmock.AnyArg(), "alerts-v2-write", "OpenSearch failed").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").
		WithArgs("tenant-a", "alert-b", "event-b", sqlmock.AnyArg(), sqlmock.AnyArg(), "alerts-v2-write", "OpenSearch failed").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	now := time.Unix(1_800_000_000, 0).UTC()
	alerts := []*Alert{
		{TenantID: "tenant-a", AlertID: "alert-a", EventID: "event-a", UpdatedTs: now},
		{TenantID: "tenant-a", AlertID: "alert-b", EventID: "event-b", UpdatedTs: now},
	}
	if err := NewProjectionDebtStore(db).RecordProjectionDebt(context.Background(), alerts, "alerts-v2-write", errors.New("OpenSearch failed")); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionReconcileManifestPersistsPostRepairReceipt(t *testing.T) {
	payload, err := projectionReconcileManifest(ProjectionReconcileResult{
		MissingIDs: []string{"before-missing"}, ExtraIDs: []string{"manual-extra"}, StaleIDs: []string{"before-stale"},
		VerificationPerformed: true, VerificationTargetCount: 3, RemainingExtraCount: 1,
		RemainingExtraIDs: []string{"manual-extra"}, WatermarksConverged: true, RepairConverged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		PostRepair struct {
			Performed           bool     `json:"performed"`
			TargetCount         int      `json:"target_count"`
			ExtraCount          int      `json:"extra_count"`
			ExtraIDs            []string `json:"extra_ids"`
			WatermarksConverged bool     `json:"watermarks_converged"`
			RepairConverged     bool     `json:"repair_converged"`
		} `json:"post_repair_verification"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if !manifest.PostRepair.Performed || manifest.PostRepair.TargetCount != 3 || manifest.PostRepair.ExtraCount != 1 ||
		len(manifest.PostRepair.ExtraIDs) != 1 || manifest.PostRepair.ExtraIDs[0] != "manual-extra" || !manifest.PostRepair.WatermarksConverged || !manifest.PostRepair.RepairConverged {
		t.Fatalf("post-repair receipt missing from manifest: %s", payload)
	}
}

func TestProjectionWatermarkMismatchQueryIsBoundedAndVersioned(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("WITH expected AS").WithArgs(sqlmock.AnyArg(), "alerts-v2-write").
		WillReturnRows(sqlmock.NewRows([]string{"alert_id"}).AddRow("alert-b"))
	now := time.Unix(1_800_000_000, 0).UTC()
	mismatches, err := NewProjectionDebtStore(db).ListProjectionWatermarkMismatches(context.Background(), []*Alert{
		{TenantID: "tenant-a", AlertID: "alert-a", UpdatedTs: now},
		{TenantID: "tenant-a", AlertID: "alert-b", UpdatedTs: now},
	}, "alerts-v2-write")
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 1 || mismatches[0] != "alert-b" {
		t.Fatalf("unexpected watermark mismatches: %v", mismatches)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionDebtBatchRollsBackIfAnyRowFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").WillReturnError(errors.New("PostgreSQL unavailable"))
	mock.ExpectRollback()
	now := time.Unix(1_800_000_000, 0).UTC()
	alerts := []*Alert{{TenantID: "tenant-a", AlertID: "alert-a", UpdatedTs: now}, {TenantID: "tenant-a", AlertID: "alert-b", UpdatedTs: now}}
	if err := NewProjectionDebtStore(db).RecordProjectionDebt(context.Background(), alerts, "alerts-v2-write", errors.New("OpenSearch failed")); err == nil {
		t.Fatal("partial debt persistence must fail and roll back")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionDebtSchemaReadinessIsMigrationBound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT version FROM alignment_schema_migrations").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("202608041100"))
	if err := NewProjectionDebtStore(db).CheckSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

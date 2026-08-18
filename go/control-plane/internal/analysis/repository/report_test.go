package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRequestHumanReportAtomicRequiresTerminalRun(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_sha256 FROM analysis_materialization_ledger").
		WithArgs("rep-idem").WillReturnRows(sqlmock.NewRows([]string{"request_sha256"}))
	mock.ExpectExec("INSERT INTO analysis_materialization_ledger").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT ru.state, COALESCE\\(s.canonical_sha256").
		WithArgs("run-1", "tenant-a").WillReturnRows(sqlmock.NewRows([]string{"state", "sha"}).AddRow("RUNNING", "sha-1"))
	mock.ExpectRollback()

	r := NewRepo(db)
	_, _, err := r.RequestHumanReportAtomic(context.Background(), "tenant-a", "run-1", "sha-1", "default-v1", "zh-CN", "h-1", "rep-idem")
	if err == nil {
		t.Fatalf("expected terminal-run rejection")
	}
}

func TestRequestHumanReportAtomicHappyPath(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_sha256 FROM analysis_materialization_ledger").
		WithArgs("rep-idem").WillReturnRows(sqlmock.NewRows([]string{"request_sha256"}))
	mock.ExpectExec("INSERT INTO analysis_materialization_ledger").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT ru.state, COALESCE\\(s.canonical_sha256").
		WithArgs("run-1", "tenant-a").WillReturnRows(sqlmock.NewRows([]string{"state", "sha"}).AddRow("SUCCEEDED", "sha-1"))
	mock.ExpectExec("INSERT INTO analysis_human_reports").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO analysis_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO analysis_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	r := NewRepo(db)
	reportID, replayed, err := r.RequestHumanReportAtomic(context.Background(), "tenant-a", "run-1", "sha-1", "default-v1", "zh-CN", "h-1", "rep-idem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replayed || reportID == "" {
		t.Fatalf("unexpected result: reportID=%s replayed=%v", reportID, replayed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestApplyHumanReportReceiptAtomicHashMismatch(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO analysis_inbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT summary_sha256 FROM analysis_human_reports").
		WithArgs("rep-1", "tenant-a").WillReturnRows(sqlmock.NewRows([]string{"summary_sha256"}).AddRow("other-sha"))
	mock.ExpectRollback()

	r := NewRepo(db)
	_, err := r.ApplyHumanReportReceiptAtomic(context.Background(), "tenant-a", "rep-1", "obj-key", strings64("obj"), 100, "v1", "expected-sha")
	if err == nil {
		t.Fatalf("expected summary hash mismatch rejection")
	}
}

func TestConfirmHumanReportObjectAtomic(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectExec("UPDATE analysis_human_reports SET state='AVAILABLE'").
		WithArgs("rep-1", "tenant-a", "obj-sha", int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))

	r := NewRepo(db)
	if err := r.ConfirmHumanReportObjectAtomic(context.Background(), "tenant-a", "rep-1", "obj-sha", 100); err != nil {
		t.Fatalf("confirm: %v", err)
	}
}

func strings64(seed string) string {
	const hexc = "0123456789abcdef"
	out := make([]byte, 64)
	for i := range out {
		out[i] = hexc[(i*7+len(seed))%16]
	}
	return string(out)
}

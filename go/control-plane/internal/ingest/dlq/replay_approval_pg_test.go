package dlq

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func testApproval() ReplayApproval {
	return ReplayApproval{
		TenantID:    "tenant-a",
		ApprovalID:  "APPROVAL-TEST-001",
		RequestedBy: "analyst-1",
		ApprovedBy:  "operator-2",
		Status:      ApprovalStatusApproved,
		Reason:      "unit test approval",
		RequestHash: "hash-abc",
		CreatedAt:   time.Now(),
	}
}

func TestPostgresReplayApprovalStoreCreateCommitsStateHistoryReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO dlq_replay_approvals").
		WithArgs("tenant-a", "APPROVAL-TEST-001", "analyst-1", "operator-2",
			ApprovalStatusApproved, "unit test approval", "hash-abc").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO dlq_replay_approval_history").
		WithArgs("tenant-a", "APPROVAL-TEST-001", "operator-2").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO dlq_replay_approval_receipts").
		WithArgs("tenant-a", "APPROVAL-TEST-001").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	store := NewPostgresReplayApprovalStore(db, nil)
	if err := store.CreateApproval(context.Background(), testApproval()); err != nil {
		t.Fatalf("CreateApproval returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresReplayApprovalStoreCreateConflictRejectsDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// ON CONFLICT DO NOTHING → 0 行受影响,权威层拒绝重复 approval_id。
	mock.ExpectExec("INSERT INTO dlq_replay_approvals").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	store := NewPostgresReplayApprovalStore(db, nil)
	err = store.CreateApproval(context.Background(), testApproval())
	if err == nil {
		t.Fatalf("expected duplicate approval error")
	}
	if err.Error() != "approval_id already exists" {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresReplayApprovalStoreGetOnlyApproved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"tenant_id", "approval_id", "requested_by", "approved_by", "status", "reason", "request_hash", "created_at",
	}).AddRow("tenant-a", "APPROVAL-TEST-001", "analyst-1", "operator-2",
		ApprovalStatusApproved, "unit test approval", "hash-abc", time.Now())
	mock.ExpectQuery("SELECT tenant_id, approval_id, requested_by, approved_by, status, reason, request_hash, created_at").
		WithArgs("tenant-a", "APPROVAL-TEST-001", ApprovalStatusApproved).
		WillReturnRows(rows)

	store := NewPostgresReplayApprovalStore(db, nil)
	approval, err := store.GetApproval(context.Background(), "tenant-a", "APPROVAL-TEST-001")
	if err != nil {
		t.Fatalf("GetApproval returned error: %v", err)
	}
	if approval.ApprovedBy != "operator-2" {
		t.Fatalf("approved_by=%q", approval.ApprovedBy)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresReplayApprovalStoreGetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT tenant_id, approval_id, requested_by, approved_by, status, reason, request_hash, created_at").
		WithArgs("tenant-a", "APPROVAL-MISSING", ApprovalStatusApproved).
		WillReturnError(errors.New("sql: no rows in result set"))

	store := NewPostgresReplayApprovalStore(db, nil)
	_, err = store.GetApproval(context.Background(), "tenant-a", "APPROVAL-MISSING")
	if err == nil {
		t.Fatalf("expected not found error")
	}
}

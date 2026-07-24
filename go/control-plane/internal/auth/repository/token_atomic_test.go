package repository

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

func validAtomicTestToken() *model.APIToken {
	actor := uuid.New()
	return &model.APIToken{
		TenantID: "default", Name: "atomic-test", TokenHash: "sha256-test", TokenPrefix: "api_atomic",
		Scopes: model.StringSlice{model.ScopeAlertRead}, CreatedBy: &actor,
	}
}

func TestCreateWithAuditRollsBackWhenAuditInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewTokenRepository(db, nil)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO api_tokens").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnError(stderrors.New("audit unavailable"))
	mock.ExpectRollback()

	err = repo.CreateWithAudit(context.Background(), validAtomicTestToken(), uuid.New())
	if err == nil {
		t.Fatal("expected audit failure")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction expectations: %v", err)
	}
}

func TestRotateWithAuditRollsBackOldRevokeAndNewTokenWhenAuditFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewTokenRepository(db, nil)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE api_tokens SET status='revoked'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO api_tokens").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnError(stderrors.New("audit unavailable"))
	mock.ExpectRollback()

	err = repo.RotateWithAudit(context.Background(), "default", uuid.New(), validAtomicTestToken(), uuid.New())
	if err == nil {
		t.Fatal("expected audit failure")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction expectations: %v", err)
	}
}

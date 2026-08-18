package api

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestExpireProbeOperationsTransactionMatrix(t *testing.T) {
	tests := []struct {
		name      string
		rows      int64
		execErr   error
		wantCount int
		wantErr   bool
	}{
		{name: "state history and outbox commit together", rows: 2, wantCount: 2},
		{name: "empty batch commits no effects", rows: 0, wantCount: 0},
		{name: "kill before commit rolls all facts back", execErr: context.Canceled, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			handler := NewSystemHandler(nil, db, zap.NewNop())
			mock.ExpectBegin()
			expectExec := mock.ExpectExec(regexp.QuoteMeta("WITH candidates AS (")).
				WithArgs(25)
			if test.execErr != nil {
				expectExec.WillReturnError(test.execErr)
				mock.ExpectRollback()
			} else {
				expectExec.WillReturnResult(sqlmock.NewResult(0, test.rows))
				mock.ExpectCommit()
			}
			count, err := handler.expireProbeOperations(context.Background(), 25)
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, test.wantErr)
			}
			if count != test.wantCount {
				t.Fatalf("count=%d want=%d", count, test.wantCount)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExpireProbeOperationsCommitFailureIsNotReportedSuccessful(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("WITH candidates AS (")).
		WithArgs(10).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit outcome unknown"))
	count, err := handler.expireProbeOperations(context.Background(), 10)
	if err == nil || count != 0 {
		t.Fatalf("count=%d err=%v, want zero/error", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

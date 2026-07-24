package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func TestFirstSettingsInsertRejectsNonzeroExpectedRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewSystemSettingsRepository(db, nil)
	mock.ExpectBegin()
	mock.ExpectQuery("WITH updated AS").WithArgs("default", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"revision", "updated_at"}))
	mock.ExpectRollback()

	_, _, err = repo.SaveSettingsWithAudit(context.Background(), "default", uuid.New(), 7, model.DefaultSystemSettings(), "system_settings_update", nil)
	if !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction expectations: %v", err)
	}
}

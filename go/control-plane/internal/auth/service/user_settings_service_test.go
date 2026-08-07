package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func TestUpdateUserSettingsCommandRejectsMissingRepository(t *testing.T) {
	service := &AuthService{}

	result, err := service.UpdateUserSettingsCommand(context.Background(), "tenant-a", uuid.New(), "display", UserSettingsUpdateCommand{
		Settings: map[string]interface{}{"page_size": 50},
	})

	if result != nil {
		t.Fatalf("expected no pseudo-success response, got %#v", result)
	}
	if !commonerrors.IsCode(err, commonerrors.ErrCodeServiceUnavailable) {
		t.Fatalf("expected service unavailable, got %v", err)
	}
}

package middleware

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/google/uuid"
)

func TestGetTenantID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyTenantID, "tenant-01")
	if got := GetTenantID(ctx); got != "tenant-01" {
		t.Errorf("GetTenantID = %q, want tenant-01", got)
	}
}

func TestGetUserID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyUserID, "user-123")
	if got := GetUserID(ctx); got != "user-123" {
		t.Errorf("GetUserID = %q, want user-123", got)
	}
}

func TestGetClaims(t *testing.T) {
	claims := &model.Claims{
		UserID:   uuid.New(),
		TenantID: "t1",
		Username: "testuser",
		Roles:    []string{"admin"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	extracted := GetClaims(ctx)
	if extracted == nil { t.Fatal("expected claims") }
	if extracted.TenantID != "t1" { t.Errorf("tenant=%s", extracted.TenantID) }
	if !claims.HasRole("admin") { t.Error("should have admin role") }
}

func TestGetClaimsNil(t *testing.T) {
	if claims := GetClaims(context.Background()); claims != nil {
		t.Error("expected nil from empty context")
	}
}

func TestEmptyTenantID(t *testing.T) {
	if got := GetTenantID(context.Background()); got != "" {
		t.Error("expected empty tenant from empty context")
	}
}

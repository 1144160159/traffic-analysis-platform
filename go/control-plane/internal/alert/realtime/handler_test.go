package realtime

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

type fakeValidator struct {
	claims *model.Claims
	err    error
}

func (v fakeValidator) ValidateToken(string) (*model.Claims, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.claims, nil
}

func TestHandleEventsRequiresTokenBeforeUpgrade(t *testing.T) {
	handler := NewHandler(fakeValidator{claims: validClaims()}, zap.NewNop())
	server := httptest.NewServer(http.HandlerFunc(handler.HandleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL + "/ws/events")
	if err != nil {
		t.Fatalf("GET /ws/events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestHandleEventsRejectsInvalidToken(t *testing.T) {
	handler := NewHandler(fakeValidator{err: errors.New("invalid")}, zap.NewNop())
	server := httptest.NewServer(http.HandlerFunc(handler.HandleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL + "/ws/events?token=bad-token")
	if err != nil {
		t.Fatalf("GET /ws/events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestHandleEventsRejectsTenantMismatch(t *testing.T) {
	handler := NewHandler(fakeValidator{claims: validClaims()}, zap.NewNop())
	server := httptest.NewServer(http.HandlerFunc(handler.HandleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL + "/ws/events?token=token-1&tenant_id=tenant-b")
	if err != nil {
		t.Fatalf("GET /ws/events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestHandleEventsUpgradesWithValidToken(t *testing.T) {
	handler := NewHandler(fakeValidator{claims: validClaims()}, zap.NewNop())
	handler.now = func() time.Time { return time.Date(2026, 6, 29, 1, 2, 3, 0, time.UTC) }
	server := httptest.NewServer(http.HandlerFunc(handler.HandleEvents))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events?token=token-1&tenant_id=tenant-a"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("websocket dial status=%d err=%v", resp.StatusCode, err)
		}
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	var msg Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read ready message: %v", err)
	}
	if msg.Type != "ready" {
		t.Fatalf("message type = %q, want ready", msg.Type)
	}
	if msg.TenantID != "tenant-a" {
		t.Fatalf("tenant_id = %q, want tenant-a", msg.TenantID)
	}
	if msg.Username != "alice" {
		t.Fatalf("username = %q, want alice", msg.Username)
	}
}

func validClaims() *model.Claims {
	return &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:  "tenant-a",
		Username:  "alice",
		TokenType: model.JWTTokenAccess,
		SessionID: "session-1",
	}
}

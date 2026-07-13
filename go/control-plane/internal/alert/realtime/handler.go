package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

type TokenValidator interface {
	ValidateToken(token string) (*model.Claims, error)
}

type Handler struct {
	validator TokenValidator
	logger    *zap.Logger
	now       func() time.Time
	upgrader  websocket.Upgrader
}

type Message struct {
	Type       string    `json:"type"`
	TenantID   string    `json:"tenant_id"`
	UserID     string    `json:"user_id,omitempty"`
	Username   string    `json:"username,omitempty"`
	ServerTime time.Time `json:"server_time"`
}

func NewHandler(validator TokenValidator, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		validator: validator,
		logger:    logger,
		now:       time.Now,
		upgrader: websocket.Upgrader{
			CheckOrigin: sameOrigin,
		},
	}
}

func (h *Handler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.validator == nil {
		http.Error(w, "realtime auth unavailable", http.StatusServiceUnavailable)
		return
	}

	token, err := bearerOrQueryToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	claims, err := h.validator.ValidateToken(token)
	if err != nil {
		h.logger.Debug("Realtime token validation failed", zap.Error(err))
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}
	if claims == nil || claims.TokenType != model.JWTTokenAccess {
		http.Error(w, "access token required", http.StatusUnauthorized)
		return
	}

	requestTenant := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if requestTenant != "" && requestTenant != claims.TenantID {
		h.logger.Warn("Realtime tenant mismatch",
			zap.String("claimed_tenant", claims.TenantID),
			zap.String("requested_tenant", requestTenant),
			zap.String("user_id", claims.UserID.String()))
		http.Error(w, "tenant mismatch", http.StatusForbidden)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Debug("Realtime websocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type:       "ready",
		TenantID:   claims.TenantID,
		UserID:     claims.UserID.String(),
		Username:   claims.Username,
		ServerTime: h.now().UTC(),
	}); err != nil {
		h.logger.Debug("Realtime ready message failed", zap.Error(err))
		return
	}

	h.runConnection(r.Context(), conn, claims)
}

func (h *Handler) runConnection(ctx context.Context, conn *websocket.Conn, claims *model.Claims) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(4096)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutdown"), h.now().Add(time.Second))
			return
		case <-done:
			return
		case <-ticker.C:
			if err := conn.WriteJSON(Message{
				Type:       "heartbeat",
				TenantID:   claims.TenantID,
				ServerTime: h.now().UTC(),
			}); err != nil {
				h.logger.Debug("Realtime heartbeat failed", zap.Error(err))
				return
			}
		}
	}
}

func bearerOrQueryToken(r *http.Request) (string, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			return "", errors.New("invalid authorization header format")
		}
		return strings.TrimSpace(parts[1]), nil
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		return "", errors.New("token required")
	}
	return token, nil
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

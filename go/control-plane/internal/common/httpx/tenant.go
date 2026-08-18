package httpx

import (
	"context"
	"net/http"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"go.uber.org/zap"
)

func BusinessContextExtractor() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			tenantID := strings.TrimSpace(GetTenantID(ctx))
			if claims := GetClaims(ctx); claims != nil {
				if err := ValidateRequestTenant(r, claims.GetTenantID()); err != nil {
					errors.WriteError(w, err, GetTraceID(ctx), r.URL.Path)
					return
				}
				tenantID = strings.TrimSpace(claims.GetTenantID())
			}

			runID := r.Header.Get("X-Run-ID")
			if runID == "" {
				runID = r.URL.Query().Get("run_id")
			}

			probeID := r.Header.Get("X-Probe-ID")
			if probeID == "" {
				probeID = r.URL.Query().Get("probe_id")
			}

			featureSetID := r.Header.Get("X-Feature-Set-ID")
			if featureSetID == "" {
				featureSetID = r.URL.Query().Get("feature_set_id")
			}

			eventID := r.Header.Get("X-Event-ID")

			if tenantID != "" {
				ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
				ctx = logging.WithTenantID(ctx, tenantID)
			}
			if runID != "" {
				ctx = context.WithValue(ctx, ContextKeyRunID, runID)
			}
			if probeID != "" {
				ctx = context.WithValue(ctx, ContextKeyProbeID, probeID)
			}
			if featureSetID != "" {
				ctx = context.WithValue(ctx, ContextKeyFeatureSetID, featureSetID)
			}
			if eventID != "" {
				ctx = context.WithValue(ctx, ContextKeyEventID, eventID)
			}

			if runID != "" {
				ctx = logging.WithRunID(ctx, runID)
			}
			if probeID != "" {
				ctx = logging.WithProbeID(ctx, probeID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TenantExtractor() Middleware {
	return BusinessContextExtractor()
}

func RequireTenant() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil || strings.TrimSpace(claims.GetTenantID()) == "" {
				err := errors.New(errors.ErrCodeUnauthorized, "Verified tenant identity is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			if err := ValidateRequestTenant(r, claims.GetTenantID()); err != nil {
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			tenantID := strings.TrimSpace(claims.GetTenantID())

			ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
			ctx = logging.WithTenantID(ctx, tenantID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type TenantValidator interface {
	ValidateTenant(ctx context.Context, tenantID string) (bool, error)
}

func ValidateTenant(validator TenantValidator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := strings.TrimSpace(GetTenantID(r.Context()))
			if tenantID == "" {
				err := errors.New(errors.ErrCodeUnauthorized, "Verified tenant identity is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			valid, err := validator.ValidateTenant(r.Context(), tenantID)
			if err != nil {
				appErr := errors.Wrap(err, errors.ErrCodeInternal, "Failed to validate tenant")
				errors.WriteError(w, appErr, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			if !valid {
				err := errors.Newf(errors.ErrCodeTenantNotFound, "Invalid tenant: %s", tenantID)
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func TenantIsolation() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				err := errors.New(errors.ErrCodeUnauthorized, "Verified tenant identity is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			if err := ValidateRequestTenant(r, claims.GetTenantID()); err != nil {
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

const (
	ContextKeyRunID        contextKey = "run_id"
	ContextKeyProbeID      contextKey = "probe_id"
	ContextKeyFeatureSetID contextKey = "feature_set_id"
	ContextKeyEventID      contextKey = "event_id"
)

func GetRunID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyRunID); v != nil {
		return v.(string)
	}
	return ""
}

func GetProbeID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyProbeID); v != nil {
		return v.(string)
	}
	return ""
}

func GetFeatureSetID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyFeatureSetID); v != nil {
		return v.(string)
	}
	return ""
}

func GetEventID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyEventID); v != nil {
		return v.(string)
	}
	return ""
}

type BusinessContext struct {
	TenantID     string
	UserID       string
	Username     string
	RunID        string
	ProbeID      string
	FeatureSetID string
	EventID      string
	TraceID      string
	RequestID    string
}

func GetBusinessContext(ctx context.Context) BusinessContext {
	return BusinessContext{
		TenantID:     GetTenantID(ctx),
		UserID:       GetUserID(ctx),
		Username:     GetUsername(ctx),
		RunID:        GetRunID(ctx),
		ProbeID:      GetProbeID(ctx),
		FeatureSetID: GetFeatureSetID(ctx),
		EventID:      GetEventID(ctx),
		TraceID:      GetTraceID(ctx),
		RequestID:    GetRequestID(ctx),
	}
}

func (bc BusinessContext) ToMap() map[string]interface{} {
	m := make(map[string]interface{})
	if bc.TenantID != "" {
		m["tenant_id"] = bc.TenantID
	}
	if bc.UserID != "" {
		m["user_id"] = bc.UserID
	}
	if bc.Username != "" {
		m["username"] = bc.Username
	}
	if bc.RunID != "" {
		m["run_id"] = bc.RunID
	}
	if bc.ProbeID != "" {
		m["probe_id"] = bc.ProbeID
	}
	if bc.FeatureSetID != "" {
		m["feature_set_id"] = bc.FeatureSetID
	}
	if bc.EventID != "" {
		m["event_id"] = bc.EventID
	}
	if bc.TraceID != "" {
		m["trace_id"] = bc.TraceID
	}
	if bc.RequestID != "" {
		m["request_id"] = bc.RequestID
	}
	return m
}

func RequireRunID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			runID := GetRunID(r.Context())
			if runID == "" {
				err := errors.New(errors.ErrCodeMissingParameter, "X-Run-ID header is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireProbeID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			probeID := GetProbeID(r.Context())
			if probeID == "" {
				err := errors.New(errors.ErrCodeMissingParameter, "X-Probe-ID header is required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireEventID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			eventID := GetEventID(r.Context())
			if eventID == "" {
				err := errors.New(errors.ErrCodeMissingParameter, "X-Event-ID header is required for idempotency")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func LogBusinessContext() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bc := GetBusinessContext(r.Context())
			logger := logging.FromContext(r.Context())

			logger.Debug("Business context extracted",
				zap.String(logging.FieldTenantID, bc.TenantID),
				zap.String(logging.FieldUserID, bc.UserID),
				zap.String(logging.FieldRunID, bc.RunID),
				zap.String(logging.FieldProbeID, bc.ProbeID),
				zap.String("feature_set_id", bc.FeatureSetID),
				zap.String(logging.FieldEventID, bc.EventID),
			)

			next.ServeHTTP(w, r)
		})
	}
}

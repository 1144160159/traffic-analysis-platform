package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-Request-ID", "X-Trace-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID"},
		AllowCredentials: true,
		MaxAge:           86400,
	}
}

func CORS(cfg *CORSConfig) Middleware {
	if cfg == nil {
		cfg = DefaultCORSConfig()
	}

	allowedOrigins := make(map[string]bool)
	allowAll := false
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			allowAll = true
			break
		}
		allowedOrigins[origin] = true
	}

	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	exposedHeaders := strings.Join(cfg.ExposedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if allowedOrigins[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if exposedHeaders != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func CORSWithConfig(allowedOrigins []string, allowCredentials bool) Middleware {
	cfg := &CORSConfig{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-Request-ID", "X-Trace-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID"},
		AllowCredentials: allowCredentials,
		MaxAge:           86400,
	}
	return CORS(cfg)
}

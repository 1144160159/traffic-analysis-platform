package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
	authjwt "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	authrepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	authservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

const (
	serviceName    = "threat-intel-service"
	serviceVersion = "1.0.0"
)

type server struct {
	intel               *threatintel.Service
	auditDB             *sql.DB
	threatIntelProducer *commonkafka.Producer
	threatIntelTopic    string
	logger              *zap.Logger
}

type tokenValidatorAdapter struct {
	authService *authservice.AuthService
}

func (a tokenValidatorAdapter) ValidateToken(tokenString string) (httpx.Claims, error) {
	return a.authService.ValidateToken(tokenString)
}

type importRequest struct {
	Source  string                   `json:"source"`
	Entries []threatintel.IntelEntry `json:"entries"`
}

type feedRunResult struct {
	Feed            threatintel.FeedSource `json:"feed"`
	Imported        int                    `json:"imported"`
	EventID         string                 `json:"event_id"`
	AuditWritten    bool                   `json:"audit_written"`
	KafkaPublished  bool                   `json:"kafka_published"`
	KafkaTopic      string                 `json:"kafka_topic"`
	SchedulerStatus string                 `json:"scheduler_status"`
}

type threatIntelEvent struct {
	EventID    string                   `json:"event_id"`
	EventType  string                   `json:"event_type"`
	Version    int                      `json:"version"`
	TenantID   string                   `json:"tenant_id"`
	UserID     string                   `json:"user_id,omitempty"`
	Username   string                   `json:"username,omitempty"`
	Source     string                   `json:"source"`
	Entry      *threatintel.IntelEntry  `json:"entry,omitempty"`
	Entries    []threatintel.IntelEntry `json:"entries,omitempty"`
	Count      int                      `json:"count"`
	RequestID  string                   `json:"request_id,omitempty"`
	TraceID    string                   `json:"trace_id,omitempty"`
	OccurredAt time.Time                `json:"occurred_at"`
}

func main() {
	logger, err := logging.NewLogger(logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Format:      getEnv("LOG_FORMAT", "json"),
		Output:      "stdout",
		Service:     serviceName,
		Version:     getEnv("SERVICE_VERSION", serviceVersion),
		Environment: getEnv("ENVIRONMENT", "development"),
	})
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logging.Sync(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pgClient, err := storage.NewPostgresClient(storage.PostgresConfig{
		Host:            getEnv("POSTGRES_HOST", "postgres-primary.databases.svc"),
		Port:            getIntEnv("POSTGRES_PORT", 5432),
		Database:        getEnv("POSTGRES_DATABASE", "traffic_platform"),
		Username:        getEnv("POSTGRES_USERNAME", "postgres"),
		Password:        getEnv("POSTGRES_PASSWORD", ""),
		SSLMode:         getEnv("POSTGRES_SSL_MODE", "disable"),
		MaxOpenConns:    getIntEnv("POSTGRES_MAX_OPEN_CONNS", 10),
		MaxIdleConns:    getIntEnv("POSTGRES_MAX_IDLE_CONNS", 3),
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		ConnectTimeout:  getIntEnv("POSTGRES_CONNECT_TIMEOUT", 10),
	}, logger)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer pgClient.Close()

	intel := threatintel.NewService(pgClient.DB(), logger)
	schemaCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	if err := intel.InitSchema(schemaCtx); err != nil {
		cancel()
		logger.Fatal("Failed to initialize threat intel schema", zap.Error(err))
	}
	cancel()

	threatIntelTopic := getEnv("KAFKA_THREAT_INTEL_TOPIC", "threat.intel.v1")
	threatIntelProducer, err := newThreatIntelProducer(logger, threatIntelTopic)
	if err != nil {
		logger.Fatal("Failed to initialize threat intel Kafka producer", zap.Error(err))
	}
	if threatIntelProducer != nil {
		defer func() {
			if closeErr := threatIntelProducer.Close(); closeErr != nil {
				logger.Warn("Failed to close threat intel Kafka producer", zap.Error(closeErr))
			}
		}()
	}

	srv := &server{
		intel:               intel,
		auditDB:             pgClient.DB(),
		threatIntelProducer: threatIntelProducer,
		threatIntelTopic:    threatIntelTopic,
		logger:              logger,
	}
	router := mux.NewRouter()
	router.HandleFunc("/health", srv.health).Methods(http.MethodGet)
	router.HandleFunc("/readyz", srv.health).Methods(http.MethodGet)
	router.Handle("/metrics", promhttp.Handler())

	var authMiddleware httpx.Middleware
	if jwtSecret := getEnv("JWT_SECRET_KEY", getEnv("JWT_SIGNING_KEY", "")); jwtSecret != "" {
		userRepo := authrepository.NewUserRepository(pgClient.DB(), logger)
		tokenRepo := authrepository.NewTokenRepository(pgClient.DB(), logger)
		jwtSvc, jwtErr := authjwt.NewService(authjwt.Config{
			SigningKey:      jwtSecret,
			SigningMethod:   "HS256",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "traffic-auth-service",
		}, nil, tokenRepo, logger)
		if jwtErr != nil {
			logger.Fatal("Failed to init JWT service", zap.Error(jwtErr))
		}
		authSvc := authservice.NewAuthService(userRepo, jwtSvc, nil, nil, logger, nil)
		authMiddleware = httpx.Auth(tokenValidatorAdapter{authService: authSvc}, logger)
		logger.Info("Auth middleware initialized")
	} else {
		logger.Warn("JWT_SECRET_KEY/JWT_SIGNING_KEY is empty, threat intel APIs will run without auth middleware")
	}

	apiRouter := router.PathPrefix("/api/v1/threat-intel").Subrouter()
	if authMiddleware != nil {
		apiRouter.Use(func(next http.Handler) http.Handler {
			return authMiddleware(next)
		})
	}
	authEnabled := authMiddleware != nil
	apiRouter.Handle("/lookup", protect(srv.lookup, authEnabled, "alert:read", "admin:*", "*")).Methods(http.MethodGet)
	apiRouter.Handle("/enrich", protect(srv.enrich, authEnabled, "alert:read", "admin:*", "*")).Methods(http.MethodGet)
	apiRouter.Handle("/entries", protect(srv.listEntries, authEnabled, "alert:read", "admin:*", "*")).Methods(http.MethodGet)
	apiRouter.Handle("/entries", protect(srv.upsertEntry, authEnabled, "alert:write", "admin:*", "*")).Methods(http.MethodPost)
	apiRouter.Handle("/import", protect(srv.importEntries, authEnabled, "alert:write", "admin:*", "*")).Methods(http.MethodPost)
	apiRouter.Handle("/feeds", protect(srv.listFeeds, authEnabled, "alert:read", "admin:*", "*")).Methods(http.MethodGet)
	apiRouter.Handle("/feeds", protect(srv.upsertFeed, authEnabled, "alert:write", "admin:*", "*")).Methods(http.MethodPost)
	apiRouter.Handle("/feeds/{name}/run", protect(srv.runFeed, authEnabled, "alert:write", "admin:*", "*")).Methods(http.MethodPost)

	api := &http.Server{
		Addr:              getEnv("API_LISTEN_ADDR", ":8087"),
		Handler:           httpx.DefaultChainWithLogger(serviceName, logger).Then(router),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("Starting Threat Intel Service", zap.String("addr", api.Addr))
		if err := api.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Threat Intel Service failed", zap.Error(err))
		}
	}()
	if getBoolEnv("THREAT_INTEL_FEED_SCHEDULER_ENABLED", true) {
		srv.startFeedScheduler(ctx, time.Duration(getIntEnv("THREAT_INTEL_FEED_SCHEDULER_TICK_SECONDS", 30))*time.Second)
	}

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := api.Shutdown(shutdownCtx); err != nil {
		logger.Error("Threat Intel Service shutdown failed", zap.Error(err))
	}
}

func protect(handler http.HandlerFunc, enabled bool, permissions ...string) http.Handler {
	h := http.Handler(handler)
	if enabled {
		h = requireAnyPermission(permissions...)(h)
	}
	return h
}

func requireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims := httpx.GetExtendedClaims(r.Context()); claims != nil {
				for _, permission := range permissions {
					if claims.HasPermission(permission) {
						next.ServeHTTP(w, r)
						return
					}
				}
			} else {
				granted := httpx.GetPermissions(r.Context())
				for _, current := range granted {
					for _, required := range permissions {
						if current == required || current == "*" {
							next.ServeHTTP(w, r)
							return
						}
					}
				}
			}
			httpx.JSONError(w, r.Context(), http.StatusForbidden, "THREAT_INTEL_PERMISSION_DENIED", "permission denied: one of "+strings.Join(permissions, ",")+" required")
		})
	}
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	httpx.JSONSuccess(w, r.Context(), map[string]interface{}{
		"service": serviceName,
		"status":  "ok",
	})
}

func (s *server) lookup(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("type")
	value := r.URL.Query().Get("value")
	entry, found, err := s.intel.LookupForTenant(r.Context(), requestTenantID(r), typ, value)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_LOOKUP_FAILED", err.Error())
		return
	}
	httpx.JSONSuccess(w, r.Context(), map[string]interface{}{
		"found":      found,
		"entry":      entry,
		"reputation": reputation(entry, found),
	})
}

func (s *server) enrich(w http.ResponseWriter, r *http.Request) {
	enrichment := s.intel.EnrichAlertForTenant(r.Context(), requestTenantID(r), r.URL.Query().Get("src_ip"), r.URL.Query().Get("dst_ip"))
	httpx.JSONSuccess(w, r.Context(), enrichment)
}

func (s *server) listEntries(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	entries, total, err := s.intel.ListForTenant(r.Context(), requestTenantID(r), threatintel.ListFilter{
		Type:       r.URL.Query().Get("type"),
		Reputation: r.URL.Query().Get("reputation"),
		Source:     r.URL.Query().Get("source"),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_LIST_FAILED", err.Error())
		return
	}
	httpx.JSONPaginated(w, r.Context(), entries, total, limit, offset)
}

func (s *server) upsertEntry(w http.ResponseWriter, r *http.Request) {
	var entry threatintel.IntelEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "THREAT_INTEL_BAD_REQUEST", "invalid entry JSON")
		return
	}
	if err := s.intel.InsertForTenant(r.Context(), requestTenantID(r), &entry); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "THREAT_INTEL_UPSERT_FAILED", err.Error())
		return
	}
	event := s.newThreatIntelEvent(r, "threat_intel.entry_upserted", entry.Source, &entry, nil, 1)
	if err := s.recordThreatIntelAudit(r.Context(), r, event, "THREAT_INTEL_ENTRY_UPSERTED", entry.Type+":"+entry.Value); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_AUDIT_FAILED", err.Error())
		return
	}
	if err := s.publishThreatIntelEvent(r.Context(), event); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_KAFKA_PUBLISH_FAILED", err.Error())
		return
	}
	httpx.JSONCreated(w, r.Context(), map[string]interface{}{
		"entry":           entry,
		"event_id":        event.EventID,
		"audit_written":   true,
		"kafka_published": true,
		"kafka_topic":     s.threatIntelTopic,
	})
}

func (s *server) importEntries(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "THREAT_INTEL_BAD_REQUEST", "invalid import JSON")
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	imported, err := s.intel.ImportEntriesForTenant(r.Context(), requestTenantID(r), req.Entries, req.Source)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "THREAT_INTEL_IMPORT_FAILED", err.Error())
		return
	}
	event := s.newThreatIntelEvent(r, "threat_intel.feed_imported", req.Source, nil, req.Entries[:imported], imported)
	if err := s.recordThreatIntelAudit(r.Context(), r, event, "THREAT_INTEL_FEED_IMPORTED", req.Source); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_AUDIT_FAILED", err.Error())
		return
	}
	if err := s.publishThreatIntelEvent(r.Context(), event); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_KAFKA_PUBLISH_FAILED", err.Error())
		return
	}
	httpx.JSONCreated(w, r.Context(), map[string]interface{}{
		"imported":        imported,
		"source":          req.Source,
		"event_id":        event.EventID,
		"audit_written":   true,
		"kafka_published": true,
		"kafka_topic":     s.threatIntelTopic,
	})
}

func (s *server) listFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := s.intel.ListFeeds(r.Context(), requestTenantID(r), r.URL.Query().Get("name"), strings.EqualFold(r.URL.Query().Get("enabled"), "true"))
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_FEED_LIST_FAILED", err.Error())
		return
	}
	httpx.JSONSuccess(w, r.Context(), feeds)
}

func (s *server) upsertFeed(w http.ResponseWriter, r *http.Request) {
	var feed threatintel.FeedSource
	if err := json.NewDecoder(r.Body).Decode(&feed); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "THREAT_INTEL_BAD_REQUEST", "invalid feed JSON")
		return
	}
	feed.TenantID = requestTenantID(r)
	if feed.IntervalSeconds <= 0 {
		feed.IntervalSeconds = getIntEnv("THREAT_INTEL_FEED_DEFAULT_INTERVAL_SECONDS", 3600)
	}
	if feed.NextRunAt == nil {
		now := time.Now().UTC()
		feed.NextRunAt = &now
	}
	updated, err := s.intel.UpsertFeed(r.Context(), &feed)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "THREAT_INTEL_FEED_UPSERT_FAILED", err.Error())
		return
	}
	httpx.JSONCreated(w, r.Context(), updated)
}

func (s *server) runFeed(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	feed, err := s.intel.GetFeed(r.Context(), requestTenantID(r), name)
	if err != nil {
		if err == sql.ErrNoRows {
			httpx.JSONError(w, r.Context(), http.StatusNotFound, "THREAT_INTEL_FEED_NOT_FOUND", "feed source not found")
			return
		}
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_FEED_GET_FAILED", err.Error())
		return
	}
	result, err := s.runThreatIntelFeed(r.Context(), feed, r, "threat_intel.feed_source_run", "THREAT_INTEL_FEED_SOURCE_RUN")
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadGateway, "THREAT_INTEL_FEED_RUN_FAILED", err.Error())
		return
	}
	httpx.JSONSuccess(w, r.Context(), result)
}

func (s *server) newThreatIntelEvent(r *http.Request, eventType, source string, entry *threatintel.IntelEntry, entries []threatintel.IntelEntry, count int) threatIntelEvent {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	return s.newThreatIntelEventWithTenant(ctx, requestTenantID(r), eventType, source, entry, entries, count)
}

func (s *server) newThreatIntelEventWithTenant(ctx context.Context, tenantID, eventType, source string, entry *threatintel.IntelEntry, entries []threatintel.IntelEntry, count int) threatIntelEvent {
	if source == "" && entry != nil {
		source = entry.Source
	}
	if source == "" {
		source = "manual"
	}
	if tenantID == "" {
		tenantID = "default"
	}
	return threatIntelEvent{
		EventID:    "ti-" + uuid.NewString(),
		EventType:  eventType,
		Version:    1,
		TenantID:   tenantID,
		UserID:     httpx.GetUserID(ctx),
		Username:   httpx.GetUsername(ctx),
		Source:     source,
		Entry:      entry,
		Entries:    entries,
		Count:      count,
		RequestID:  httpx.GetRequestID(ctx),
		TraceID:    httpx.GetTraceID(ctx),
		OccurredAt: time.Now().UTC(),
	}
}

func (s *server) runThreatIntelFeed(ctx context.Context, feed *threatintel.FeedSource, r *http.Request, eventType, action string) (*feedRunResult, error) {
	if feed == nil {
		return nil, fmt.Errorf("nil threat intel feed")
	}
	startedAt := time.Now().UTC()
	imported, err := s.intel.ImportEntriesForTenant(ctx, feed.TenantID, feed.Entries, feed.Name)
	if err != nil {
		_ = s.intel.RecordFeedRun(ctx, *feed, "failed", err.Error(), startedAt)
		return nil, err
	}
	event := s.newThreatIntelEventWithTenant(ctx, feed.TenantID, eventType, feed.Name, nil, feed.Entries[:imported], imported)
	if err := s.recordThreatIntelAudit(ctx, r, event, action, feed.Name); err != nil {
		_ = s.intel.RecordFeedRun(ctx, *feed, "failed", err.Error(), startedAt)
		return nil, err
	}
	if err := s.publishThreatIntelEvent(ctx, event); err != nil {
		_ = s.intel.RecordFeedRun(ctx, *feed, "failed", err.Error(), startedAt)
		return nil, err
	}
	if err := s.intel.RecordFeedRun(ctx, *feed, "success", "", startedAt); err != nil {
		return nil, err
	}
	updated, err := s.intel.GetFeed(ctx, feed.TenantID, feed.Name)
	if err != nil {
		updated = feed
	}
	return &feedRunResult{
		Feed:            *updated,
		Imported:        imported,
		EventID:         event.EventID,
		AuditWritten:    true,
		KafkaPublished:  true,
		KafkaTopic:      s.threatIntelTopic,
		SchedulerStatus: "success",
	}, nil
}

func (s *server) startFeedScheduler(ctx context.Context, tick time.Duration) {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	go func() {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.runDueFeeds(ctx)
				timer.Reset(tick)
			}
		}
	}()
}

func (s *server) runDueFeeds(ctx context.Context) {
	feeds, err := s.intel.DueFeeds(ctx, time.Now().UTC(), getIntEnv("THREAT_INTEL_FEED_SCHEDULER_BATCH_SIZE", 20))
	if err != nil {
		s.logger.Warn("Failed to list due threat intel feeds", zap.Error(err))
		return
	}
	for i := range feeds {
		feed := feeds[i]
		if _, err := s.runThreatIntelFeed(ctx, &feed, nil, "threat_intel.feed_scheduled_imported", "THREAT_INTEL_FEED_SCHEDULED_IMPORT"); err != nil {
			s.logger.Warn("Threat intel feed scheduler run failed", zap.String("feed", feed.Name), zap.Error(err))
		}
	}
}

func (s *server) publishThreatIntelEvent(ctx context.Context, event threatIntelEvent) error {
	if s.threatIntelProducer == nil {
		return fmt.Errorf("threat intel Kafka producer is not configured")
	}
	key := event.TenantID + ":" + event.EventType + ":" + event.Source + ":" + event.EventID
	return s.threatIntelProducer.SendJSON(ctx, key, event,
		commonkafka.MessageHeader{Key: "event_id", Value: event.EventID},
		commonkafka.MessageHeader{Key: "event_type", Value: event.EventType},
		commonkafka.MessageHeader{Key: "tenant_id", Value: event.TenantID},
		commonkafka.MessageHeader{Key: "user_id", Value: event.UserID},
		commonkafka.MessageHeader{Key: "source", Value: event.Source},
		commonkafka.MessageHeader{Key: "request_id", Value: event.RequestID},
		commonkafka.MessageHeader{Key: "trace_id", Value: event.TraceID},
	)
}

func (s *server) recordThreatIntelAudit(ctx context.Context, r *http.Request, event threatIntelEvent, action, objectID string) error {
	if s.auditDB == nil {
		return fmt.Errorf("audit database is not configured")
	}
	detail := map[string]interface{}{
		"event_id":    event.EventID,
		"event_type":  event.EventType,
		"source":      event.Source,
		"count":       event.Count,
		"kafka_topic": s.threatIntelTopic,
		"request_id":  event.RequestID,
		"trace_id":    event.TraceID,
	}
	if event.Entry != nil {
		detail["entry_type"] = event.Entry.Type
		detail["entry_value"] = event.Entry.Value
		detail["reputation"] = event.Entry.Reputation
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	userID := event.UserID
	if userID != "" {
		if _, err := uuid.Parse(userID); err != nil {
			userID = ""
		}
	}
	userAgent := ""
	if r != nil {
		userAgent = r.UserAgent()
	}
	_, err = s.auditDB.ExecContext(ctx, `
		INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7::jsonb, $8, $9)`,
		"audit-"+uuid.NewString(),
		event.TenantID,
		userID,
		action,
		"threat_intel",
		objectID,
		string(detailJSON),
		clientIP(r),
		userAgent)
	return err
}

func reputation(entry *threatintel.IntelEntry, found bool) threatintel.Reputation {
	if !found || entry == nil {
		return threatintel.RepUnknown
	}
	return entry.Reputation
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid int env %s=%q, using %d\n", key, raw, fallback)
		return fallback
	}
	return value
}

func getBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func requestTenantID(r *http.Request) string {
	if r == nil {
		return "default"
	}
	if tenantID := httpx.GetTenantID(r.Context()); tenantID != "" {
		return tenantID
	}
	if tenantID := strings.TrimSpace(r.Header.Get("X-Tenant-ID")); tenantID != "" {
		return tenantID
	}
	return "default"
}

func newThreatIntelProducer(logger *zap.Logger, topic string) (*commonkafka.Producer, error) {
	brokers := splitCSV(getEnv("KAFKA_BROKERS", ""))
	if len(brokers) == 0 {
		logger.Warn("KAFKA_BROKERS is empty, threat intel Kafka publishing is disabled")
		return nil, nil
	}
	return commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers:      brokers,
		Topic:        topic,
		BatchSize:    getIntEnv("KAFKA_BATCH_SIZE", 100),
		BatchTimeout: 100 * time.Millisecond,
		MaxAttempts:  getIntEnv("KAFKA_MAX_ATTEMPTS", 3),
		RequiredAcks: getEnv("KAFKA_REQUIRED_ACKS", "all"),
		Compression:  getEnv("KAFKA_COMPRESSION", "lz4"),
		Async:        false,
		Security: commonkafka.SecurityConfig{
			SecurityProtocol: getEnv("KAFKA_SECURITY_PROTOCOL", ""),
			SASLMechanism:    getEnv("KAFKA_SASL_MECHANISM", "SCRAM-SHA-512"),
			SASLUsername:     getEnv("KAFKA_SASL_USERNAME", ""),
			SASLPassword:     getEnv("KAFKA_SASL_PASSWORD", ""),
			TLSCAFile:        getEnv("KAFKA_TLS_CA_FILE", ""),
			TLSServerName:    getEnv("KAFKA_TLS_SERVER_NAME", ""),
		},
	}, logger)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if value := r.Header.Get(header); value != "" {
			return strings.TrimSpace(strings.Split(value, ",")[0])
		}
	}
	return r.RemoteAddr
}

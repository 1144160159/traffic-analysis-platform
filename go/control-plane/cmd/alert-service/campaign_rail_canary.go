package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/campaignrail"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/consumer"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const campaignRailCorrelationContractSHA256 = "f4f564d6d22084c1202634af99fabe29067bd51f5748dfd30b9fd48f0541980b"

var campaignRailCanaryStages = map[string]struct{}{
	"proto-consumer":          {},
	"proto-fixture":           {},
	"json-consumer":           {},
	"json-projection-fixture": {},
	"json-fixture":            {},
	"json-dispatcher":         {},
	"correlation":             {},
}

type campaignRailCanaryIdentity struct {
	RunID           uuid.UUID
	RunIDText       string
	CandidateSHA256 string
	TenantID        string
	Suffix          string
	CEPCampaignID   string
	AuthorityID     string
	AlertID         string
	ProtoEventID    string
	JSONEventID     string
	RelationID      string
	TraceID         string
}

type campaignRailCanaryRuntime struct {
	ready  func(context.Context) error
	close  func() error
	errors <-chan error
}

func loadCampaignRailCanaryIdentity() (campaignRailCanaryIdentity, error) {
	runIDText := strings.TrimSpace(os.Getenv("CAMPAIGN_RAIL_CANARY_RUN_ID"))
	runID, err := uuid.Parse(runIDText)
	if err != nil {
		return campaignRailCanaryIdentity{}, fmt.Errorf("CAMPAIGN_RAIL_CANARY_RUN_ID must be a UUID")
	}
	candidate := strings.TrimSpace(os.Getenv("CAMPAIGN_RAIL_CANARY_CANDIDATE_SHA256"))
	if len(candidate) != 64 || candidate != strings.ToLower(candidate) {
		return campaignRailCanaryIdentity{}, fmt.Errorf("CAMPAIGN_RAIL_CANARY_CANDIDATE_SHA256 must be lowercase SHA-256")
	}
	if _, err := hex.DecodeString(candidate); err != nil {
		return campaignRailCanaryIdentity{}, fmt.Errorf("CAMPAIGN_RAIL_CANARY_CANDIDATE_SHA256 must be lowercase SHA-256")
	}
	suffix := strings.ReplaceAll(runID.String(), "-", "")[:12]
	tenantID := "canary-m07-" + suffix
	if configured := strings.TrimSpace(os.Getenv("CAMPAIGN_RAIL_CANARY_TENANT_ID")); configured != tenantID {
		return campaignRailCanaryIdentity{}, fmt.Errorf("CAMPAIGN_RAIL_CANARY_TENANT_ID must equal %s", tenantID)
	}
	stableID := func(label string) string { return uuid.NewSHA1(runID, []byte(label)).String() }
	return campaignRailCanaryIdentity{
		RunID: runID, RunIDText: runID.String(), CandidateSHA256: candidate, TenantID: tenantID, Suffix: suffix,
		CEPCampaignID: "cep-canary-" + suffix, AuthorityID: "authority-canary-" + suffix,
		AlertID: "alert-canary-" + suffix, ProtoEventID: stableID("proto-event"),
		JSONEventID: stableID("json-event"), RelationID: stableID("relation"),
		TraceID: "trace-campaign-rail-" + suffix,
	}, nil
}

func validateCampaignRailCanaryStage(stage string, cfg *config.Config, identity campaignRailCanaryIdentity) error {
	if _, ok := campaignRailCanaryStages[stage]; !ok {
		return fmt.Errorf("unsupported CAMPAIGN_RAIL_CANARY_STAGE %q", stage)
	}
	if cfg == nil {
		return fmt.Errorf("campaign rail canary config is required")
	}
	if cfg.Kafka.CampaignProtoTopic != campaignrail.ProtoTopic ||
		cfg.Kafka.CampaignEventTopic != campaignrail.AggregateJSONTopic ||
		cfg.Kafka.CampaignMemberTopic != campaignrail.MembershipJSONTopic {
		return fmt.Errorf("campaign rail canary requires the three canonical topics")
	}
	if stage != "proto-fixture" && stage != "json-projection-fixture" && stage != "json-fixture" {
		expected := map[string][4]bool{
			"proto-consumer":  {true, false, false, false},
			"json-consumer":   {false, true, false, false},
			"json-dispatcher": {false, false, true, false},
			"correlation":     {false, false, false, true},
		}[stage]
		observed := [4]bool{cfg.Kafka.CampaignProtoEnabled, cfg.Kafka.CampaignEventConsumerEnabled,
			cfg.Kafka.CampaignEventDispatcherEnabled, cfg.Kafka.CampaignRailCorrelationEnabled}
		if observed != expected {
			return fmt.Errorf("campaign rail canary stage %s requires exact isolated switch vector %v", stage, expected)
		}
	}
	if stage == "proto-consumer" || stage == "correlation" {
		if strings.TrimSpace(os.Getenv("CAMPAIGNS_PROTO_CANDIDATE_SHA256")) != identity.CandidateSHA256 {
			return fmt.Errorf("Protobuf consumer candidate SHA is not bound to the canary image")
		}
	}
	if stage == "json-consumer" || stage == "json-dispatcher" || stage == "correlation" {
		if strings.TrimSpace(os.Getenv("CAMPAIGN_JSON_V2_CANDIDATE_SHA256")) != identity.CandidateSHA256 {
			return fmt.Errorf("JSON rail candidate SHA is not bound to the canary image")
		}
	}
	if stage == "correlation" && strings.TrimSpace(os.Getenv("CAMPAIGN_RAIL_CORRELATION_CONTRACT_SHA256")) != campaignRailCorrelationContractSHA256 {
		return fmt.Errorf("campaign rail correlation contract SHA is missing or stale")
	}
	return nil
}

func runCampaignRailCanary(parent context.Context, stage string, cfg *config.Config, logger *zap.Logger) error {
	identity, err := loadCampaignRailCanaryIdentity()
	if err != nil {
		return err
	}
	if err := validateCampaignRailCanaryStage(stage, cfg, identity); err != nil {
		return err
	}
	if stage == "proto-fixture" {
		return publishCampaignRailProtoFixture(parent, cfg, identity, logger)
	}
	if stage == "json-projection-fixture" {
		return publishCampaignRailJSONProjectionFixture(parent, cfg, identity, logger)
	}

	db, err := openCampaignRailCanaryDatabase(parent, cfg, identity)
	if err != nil {
		return err
	}
	defer db.Close()
	if stage == "json-fixture" {
		return seedCampaignRailJSONFixture(parent, db, identity)
	}

	stageCtx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runtime, err := startCampaignRailCanaryRuntime(stageCtx, stage, cfg, db, identity, logger)
	if err != nil {
		return err
	}
	defer runtime.close()
	return serveCampaignRailCanary(stageCtx, stage, identity, runtime, cfg.API.ListenAddr, logger)
}

func openCampaignRailCanaryDatabase(ctx context.Context, cfg *config.Config, identity campaignRailCanaryIdentity) (*sql.DB, error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("CAMPAIGN_RAIL_CANARY_ISOLATED_DATABASE")), "true") {
		return nil, fmt.Errorf("campaign rail canary refuses a database without the isolated-database guard")
	}
	db, err := sql.Open("postgres", cfg.Auth.ConnectionString())
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping campaign rail canary PostgreSQL: %w", err)
	}
	var candidate string
	var expiresAt time.Time
	err = db.QueryRowContext(pingCtx, `SELECT candidate_sha256,expires_at
		FROM campaign_rail_canary_sentinel WHERE run_id=$1::uuid`, identity.RunIDText).Scan(&candidate, &expiresAt)
	if err != nil || candidate != identity.CandidateSHA256 || !expiresAt.After(time.Now().UTC()) {
		db.Close()
		return nil, fmt.Errorf("campaign rail canary database sentinel mismatch or expired")
	}
	store, err := campaignrail.NewProtoProjectionStore(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := store.VerifySchema(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("verify campaign rail canary schema: %w", err)
	}
	return db, nil
}

func startCampaignRailCanaryRuntime(
	ctx context.Context,
	stage string,
	cfg *config.Config,
	db *sql.DB,
	identity campaignRailCanaryIdentity,
	logger *zap.Logger,
) (campaignRailCanaryRuntime, error) {
	store, err := campaignrail.NewProtoProjectionStore(db)
	if err != nil {
		return campaignRailCanaryRuntime{}, err
	}
	handler := api.NewSystemHandler(nil, db, logger)
	consumerErrors := make(chan error, 2)
	consumerConfig := func(topic, group string) commonkafka.ConsumerConfig {
		return commonkafka.ConsumerConfig{Brokers: cfg.Kafka.Brokers, Topic: topic, GroupID: group,
			MinBytes: 1, MaxWait: 500 * time.Millisecond, MaxRetries: 3, RetryBackoff: time.Second,
			EnableDLQ: true, DLQTopic: "dlq.v1", CommitOnDLQSuccess: true,
			CommitOnHandlerError: false, DLQPermanentOnly: true, Security: cfg.Kafka.Security}
	}
	switch stage {
	case "proto-consumer":
		base, err := commonkafka.NewConsumer(consumerConfig(cfg.Kafka.CampaignProtoTopic, cfg.Kafka.CampaignProtoGroup), logger)
		if err != nil {
			return campaignRailCanaryRuntime{}, err
		}
		rail, err := consumer.NewCampaignDetectionConsumer(base, store, identity.CandidateSHA256,
			cfg.Kafka.CampaignProtoTopic, cfg.Kafka.CampaignProtoGroup, logger)
		if err != nil {
			base.Close()
			return campaignRailCanaryRuntime{}, err
		}
		go runCampaignRailCanaryConsumer(ctx, consumerErrors, rail.Start)
		return campaignRailCanaryRuntime{ready: rail.Ready, close: rail.Close, errors: consumerErrors}, nil
	case "json-consumer":
		aggregateBase, err := commonkafka.NewConsumer(consumerConfig(cfg.Kafka.CampaignEventTopic, cfg.Kafka.CampaignEventGroup), logger)
		if err != nil {
			return campaignRailCanaryRuntime{}, err
		}
		aggregate, err := consumer.NewCampaignEventConsumer(aggregateBase, handler, "aggregate", cfg.Kafka.CampaignEventTopic, logger)
		if err == nil {
			err = aggregate.SetReadinessAuthority(store, identity.CandidateSHA256, cfg.Kafka.CampaignEventGroup)
		}
		if err != nil {
			aggregateBase.Close()
			return campaignRailCanaryRuntime{}, err
		}
		membershipBase, err := commonkafka.NewConsumer(consumerConfig(cfg.Kafka.CampaignMemberTopic, cfg.Kafka.CampaignMemberGroup), logger)
		if err != nil {
			aggregate.Close()
			return campaignRailCanaryRuntime{}, err
		}
		membership, err := consumer.NewCampaignEventConsumer(membershipBase, handler, "membership", cfg.Kafka.CampaignMemberTopic, logger)
		if err == nil {
			err = membership.SetReadinessAuthority(store, identity.CandidateSHA256, cfg.Kafka.CampaignMemberGroup)
		}
		if err != nil {
			aggregate.Close()
			membershipBase.Close()
			return campaignRailCanaryRuntime{}, err
		}
		go runCampaignRailCanaryConsumer(ctx, consumerErrors, aggregate.Start)
		go runCampaignRailCanaryConsumer(ctx, consumerErrors, membership.Start)
		ready := func(readyCtx context.Context) error {
			if err := aggregate.Ready(readyCtx); err != nil {
				return fmt.Errorf("aggregate JSON rail: %w", err)
			}
			if err := membership.Ready(readyCtx); err != nil {
				return fmt.Errorf("membership JSON rail: %w", err)
			}
			return nil
		}
		closeRuntime := func() error { return errors.Join(aggregate.Close(), membership.Close()) }
		return campaignRailCanaryRuntime{ready: ready, close: closeRuntime, errors: consumerErrors}, nil
	case "json-dispatcher":
		admit := campaignRailJSONAdmission(store, cfg, identity)
		admitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := admit(admitCtx)
		cancel()
		if err != nil {
			return campaignRailCanaryRuntime{}, fmt.Errorf("dispatcher admission: %w", err)
		}
		aggregate, err := commonkafka.NewProducer(commonkafka.ProducerConfig{Brokers: cfg.Kafka.Brokers,
			Topic: cfg.Kafka.CampaignEventTopic, BatchSize: 1, RequiredAcks: "all", Compression: "none",
			Async: false, Security: cfg.Kafka.Security}, logger)
		if err != nil {
			return campaignRailCanaryRuntime{}, err
		}
		membership, err := commonkafka.NewProducer(commonkafka.ProducerConfig{Brokers: cfg.Kafka.Brokers,
			Topic: cfg.Kafka.CampaignMemberTopic, BatchSize: 1, RequiredAcks: "all", Compression: "none",
			Async: false, Security: cfg.Kafka.Security}, logger)
		if err != nil {
			aggregate.Close()
			return campaignRailCanaryRuntime{}, err
		}
		handler.SetCampaignEventProducers(aggregate, membership)
		handler.SetCampaignDispatcherAdmission(admit)
		if err := handler.StartCampaignEventOutboxWorker(ctx, 250*time.Millisecond); err != nil {
			aggregate.Close()
			membership.Close()
			return campaignRailCanaryRuntime{}, err
		}
		return campaignRailCanaryRuntime{ready: admit,
			close: func() error { return errors.Join(aggregate.Close(), membership.Close()) }, errors: consumerErrors}, nil
	case "correlation":
		admit := campaignRailCorrelationAdmission(store, cfg, identity)
		admitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := admit(admitCtx)
		cancel()
		if err != nil {
			return campaignRailCanaryRuntime{}, fmt.Errorf("correlation admission: %w", err)
		}
		handler.SetCampaignRailCorrelationAdmission(admit)
		// A K8s rollout can cross the wall-clock boundary used by the periodic
		// window planner between publishing the CEP fixture and starting this
		// stage. Replay the immutable window that actually owns this run's CEP
		// event before starting the normal worker; never move the event into the
		// new wall-clock bucket merely to make the canary pass.
		if err := projectCampaignRailCanaryReplay(ctx, db, handler, identity, time.Now().UTC(), logger); err != nil {
			return campaignRailCanaryRuntime{}, err
		}
		if err := handler.StartCampaignRailCorrelationWorker(ctx, time.Second, 5*time.Minute, time.Minute, 100); err != nil {
			return campaignRailCanaryRuntime{}, err
		}
		return campaignRailCanaryRuntime{ready: admit, close: func() error { return nil }, errors: consumerErrors}, nil
	default:
		return campaignRailCanaryRuntime{}, fmt.Errorf("unsupported long-running canary stage %q", stage)
	}
}

func campaignRailCanaryReplayScope(
	identity campaignRailCanaryIdentity,
	eventTimeEnd time.Time,
	asOf time.Time,
) (api.CampaignRailScope, error) {
	eventTimeEnd = eventTimeEnd.UTC()
	asOf = asOf.UTC()
	windowFrom := eventTimeEnd.Truncate(5 * time.Minute)
	windowThrough := windowFrom.Add(5 * time.Minute)
	if identity.TenantID == "" || eventTimeEnd.IsZero() || asOf.IsZero() || windowThrough.After(asOf) {
		return api.CampaignRailScope{}, fmt.Errorf("campaign rail canary CEP window is not closed")
	}
	return api.CampaignRailScope{TenantID: identity.TenantID, WindowFrom: windowFrom,
		WindowThrough: windowThrough, AsOf: asOf, MaxCampaigns: 100}, nil
}

func projectCampaignRailCanaryReplay(
	ctx context.Context,
	db *sql.DB,
	handler *api.SystemHandler,
	identity campaignRailCanaryIdentity,
	asOf time.Time,
	logger *zap.Logger,
) error {
	var eventTimeEndMS int64
	err := db.QueryRowContext(ctx, `SELECT event_time_end_ms
		FROM campaign_proto_projection_inbox_v1
		WHERE event_id=$1::uuid AND tenant_id=$2 AND state='applied'`, identity.ProtoEventID, identity.TenantID).
		Scan(&eventTimeEndMS)
	if err != nil {
		return fmt.Errorf("load campaign rail canary CEP replay boundary: %w", err)
	}
	scope, err := campaignRailCanaryReplayScope(identity, time.UnixMilli(eventTimeEndMS), asOf)
	if err != nil {
		return err
	}
	projection, err := handler.ProjectCampaignRailCorrelations(ctx, scope)
	if err != nil {
		return fmt.Errorf("project campaign rail canary replay: %w", err)
	}
	if projection.Processed != 1 || projection.ByState["correlated"] != 1 {
		return fmt.Errorf("campaign rail canary replay was not exactly correlated: processed=%d states=%v",
			projection.Processed, projection.ByState)
	}
	reconcile, err := handler.ReconcileCampaignRails(ctx, scope)
	if err != nil {
		return fmt.Errorf("reconcile campaign rail canary replay: %w", err)
	}
	if reconcile.State != "exact" || reconcile.CorrelatedCount != 1 || reconcile.MissingCount != 0 ||
		reconcile.ConflictCount != 0 || reconcile.ExtraCount != 0 {
		return fmt.Errorf("campaign rail canary replay did not reconcile exactly: %+v", reconcile)
	}
	logger.Info("Campaign rail canary immutable window replay reconciled",
		zap.Time("window_from", scope.WindowFrom), zap.Time("window_through", scope.WindowThrough),
		zap.Time("as_of", scope.AsOf), zap.String("tenant_id", identity.TenantID))
	return nil
}

func runCampaignRailCanaryConsumer(ctx context.Context, errorChannel chan<- error, start func(context.Context) error) {
	if err := start(ctx); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		select {
		case errorChannel <- err:
		default:
		}
	}
}

func campaignRailJSONAdmission(store *campaignrail.ProtoProjectionStore, cfg *config.Config, identity campaignRailCanaryIdentity) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := store.AssertConsumerReady(ctx, campaignrail.AggregateJSONRailID, identity.CandidateSHA256,
			cfg.Kafka.CampaignEventTopic, cfg.Kafka.CampaignEventGroup); err != nil {
			return err
		}
		return store.AssertConsumerReady(ctx, campaignrail.MembershipJSONRailID, identity.CandidateSHA256,
			cfg.Kafka.CampaignMemberTopic, cfg.Kafka.CampaignMemberGroup)
	}
}

func campaignRailCorrelationAdmission(store *campaignrail.ProtoProjectionStore, cfg *config.Config, identity campaignRailCanaryIdentity) func(context.Context) error {
	jsonAdmit := campaignRailJSONAdmission(store, cfg, identity)
	return func(ctx context.Context) error {
		if err := store.AssertConsumerReady(ctx, campaignrail.ProtoRailID, identity.CandidateSHA256,
			cfg.Kafka.CampaignProtoTopic, cfg.Kafka.CampaignProtoGroup); err != nil {
			return err
		}
		return jsonAdmit(ctx)
	}
}

func serveCampaignRailCanary(
	ctx context.Context,
	stage string,
	identity campaignRailCanaryIdentity,
	runtime campaignRailCanaryRuntime,
	addr string,
	logger *zap.Logger,
) error {
	mux := http.NewServeMux()
	write := func(writer http.ResponseWriter, status int, state string, detail string) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_ = json.NewEncoder(writer).Encode(map[string]string{"stage": stage, "state": state,
			"run_id": identity.RunIDText, "candidate_sha256": identity.CandidateSHA256, "detail": detail})
	}
	mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) { write(writer, http.StatusOK, "running", "") })
	mux.HandleFunc("/health/ready", func(writer http.ResponseWriter, request *http.Request) {
		readyCtx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := runtime.ready(readyCtx); err != nil {
			write(writer, http.StatusServiceUnavailable, "waiting_for_durable_receipt", err.Error())
			return
		}
		write(writer, http.StatusOK, "ready", "")
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Campaign rail K8s canary stage listening", zap.String("stage", stage), zap.String("addr", addr),
			zap.String("candidate_sha256", identity.CandidateSHA256), zap.String("run_id", identity.RunIDText))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-runtime.errors:
	case runErr = <-serverErrors:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

func publishCampaignRailProtoFixture(ctx context.Context, cfg *config.Config, identity campaignRailCanaryIdentity, logger *zap.Logger) error {
	if !regexp.MustCompile(`(^|-)flink-cep($|-)`).MatchString(strings.ToLower(cfg.Kafka.Security.SASLUsername)) {
		return fmt.Errorf("Protobuf fixture requires the flink-cep Kafka principal")
	}
	window := 5 * time.Minute
	through := time.Now().UTC().Add(-time.Minute).Truncate(window)
	end := through.Add(-time.Minute)
	start := end.Add(-time.Minute)
	campaign := &trafficv1.Campaign{TenantId: identity.TenantID, CampaignId: identity.CEPCampaignID,
		TsStart: start.UnixMilli(), TsEnd: end.UnixMilli(), Entities: []string{"entity-" + identity.Suffix},
		Alerts: []string{identity.AlertID}, Score: 1, Summary: "isolated M07 campaign rail canary",
		EventId: identity.ProtoEventID, IngestTs: time.Now().UTC().UnixMilli(), CampaignType: "canary",
		Header: &trafficv1.EventHeader{EventId: identity.ProtoEventID, TenantId: identity.TenantID,
			RunId: identity.RunIDText, EventTs: end.UnixMilli(), IngestTs: time.Now().UTC().UnixMilli(),
			EventType: "traffic.campaign.v1.Detected", SchemaVersion: campaignrail.ProtoSchema,
			AggregateType: "campaign", AggregateId: identity.CEPCampaignID, AggregateVersion: 1,
			OccurredAt: end.UnixMilli(), ProducedAt: time.Now().UTC().UnixMilli(), TraceId: identity.TraceID,
			IdempotencyKey: identity.ProtoEventID, Producer: campaignrail.ProtoSourceService}}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(campaign)
	if err != nil {
		return err
	}
	producer, err := commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{Brokers: cfg.Kafka.Brokers,
		Topic: cfg.Kafka.CampaignProtoTopic, BatchSize: 1, RequiredAcks: "all", Compression: "none",
		Async: false, Security: cfg.Kafka.Security}, logger)
	if err != nil {
		return err
	}
	defer producer.Close()
	key := identity.TenantID + ":" + identity.CEPCampaignID
	receipt, err := producer.Send(ctx, key, payload,
		commonkafka.MessageHeader{Key: "content_type", Value: "application/x-protobuf"},
		commonkafka.MessageHeader{Key: "proto_message_type", Value: campaignrail.ProtoMessageType},
		commonkafka.MessageHeader{Key: "schema_version", Value: campaignrail.ProtoSchema},
		commonkafka.MessageHeader{Key: "source_service", Value: campaignrail.ProtoSourceService},
		commonkafka.MessageHeader{Key: "target_topic", Value: campaignrail.ProtoTopic},
		commonkafka.MessageHeader{Key: "tenant_id", Value: identity.TenantID},
		commonkafka.MessageHeader{Key: "campaign_id", Value: identity.CEPCampaignID},
		commonkafka.MessageHeader{Key: "event_id", Value: identity.ProtoEventID})
	if err != nil {
		return err
	}
	logger.Info("Campaign rail Protobuf fixture broker-acknowledged", zap.Int("partition", receipt.Partition),
		zap.Int64("offset", receipt.Offset), zap.String("event_id", identity.ProtoEventID))
	return nil
}

func campaignRailJSONFixturePayload(identity campaignRailCanaryIdentity) ([]byte, error) {
	payload := map[string]interface{}{"event_id": identity.JSONEventID,
		"event_type": "traffic.campaign.v2.AlertLinked", "tenant_id": identity.TenantID,
		"schema_version": 2, "aggregate_type": "campaign", "aggregate_id": identity.AuthorityID,
		"aggregate_version": 2, "partition_key": identity.TenantID + ":" + identity.AuthorityID,
		"campaign_id": identity.AuthorityID, "alert_id": identity.AlertID, "relation_id": identity.RelationID,
		"relation_revision": 1, "campaign_revision": 2, "status": "active", "assignee": "",
		"member_count": 1, "reason": "isolated M07 campaign rail canary", "trace_id": identity.TraceID}
	return json.Marshal(payload)
}

func publishCampaignRailJSONProjectionFixture(
	ctx context.Context,
	cfg *config.Config,
	identity campaignRailCanaryIdentity,
	logger *zap.Logger,
) error {
	if !regexp.MustCompile(`(^|-)alert-service($|-)`).MatchString(strings.ToLower(cfg.Kafka.Security.SASLUsername)) {
		return fmt.Errorf("JSON projection fixture requires the alert-service Kafka principal")
	}
	payload, err := campaignRailJSONFixturePayload(identity)
	if err != nil {
		return err
	}
	newProducer := func(topic string) (*commonkafka.KeyedProducer, error) {
		return commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{Brokers: cfg.Kafka.Brokers,
			Topic: topic, BatchSize: 1, RequiredAcks: "all", Compression: "none", Async: false,
			Security: cfg.Kafka.Security}, logger)
	}
	aggregate, err := newProducer(cfg.Kafka.CampaignEventTopic)
	if err != nil {
		return err
	}
	defer aggregate.Close()
	membership, err := newProducer(cfg.Kafka.CampaignMemberTopic)
	if err != nil {
		return err
	}
	defer membership.Close()
	key := identity.TenantID + ":" + identity.AuthorityID
	commonHeaders := []commonkafka.MessageHeader{
		{Key: "event_id", Value: identity.JSONEventID},
		{Key: "event_type", Value: "traffic.campaign.v2.AlertLinked"},
		{Key: "tenant_id", Value: identity.TenantID},
		{Key: "schema_version", Value: "2"},
		{Key: "trace_id", Value: identity.TraceID},
	}
	aggregateHeaders := append(append([]commonkafka.MessageHeader{}, commonHeaders...),
		commonkafka.MessageHeader{Key: "stream", Value: "aggregate"},
		commonkafka.MessageHeader{Key: "aggregate_id", Value: identity.AuthorityID},
		commonkafka.MessageHeader{Key: "campaign_id", Value: identity.AuthorityID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: "2"},
		commonkafka.MessageHeader{Key: "relation_revision", Value: "0"},
		commonkafka.MessageHeader{Key: "target_topic", Value: campaignrail.AggregateJSONTopic})
	membershipHeaders := append(append([]commonkafka.MessageHeader{}, commonHeaders...),
		commonkafka.MessageHeader{Key: "stream", Value: "membership"},
		commonkafka.MessageHeader{Key: "aggregate_id", Value: identity.RelationID},
		commonkafka.MessageHeader{Key: "campaign_id", Value: identity.AuthorityID},
		commonkafka.MessageHeader{Key: "aggregate_version", Value: "2"},
		commonkafka.MessageHeader{Key: "relation_revision", Value: "1"},
		commonkafka.MessageHeader{Key: "target_topic", Value: campaignrail.MembershipJSONTopic})
	aggregateReceipt, err := aggregate.Send(ctx, key, payload, aggregateHeaders...)
	if err != nil {
		return fmt.Errorf("publish aggregate JSON projection fixture: %w", err)
	}
	membershipReceipt, err := membership.Send(ctx, key, payload, membershipHeaders...)
	if err != nil {
		return fmt.Errorf("publish membership JSON projection fixture: %w", err)
	}
	logger.Info("Campaign rail JSON projection fixtures broker-acknowledged",
		zap.Int("aggregate_partition", aggregateReceipt.Partition), zap.Int64("aggregate_offset", aggregateReceipt.Offset),
		zap.Int("membership_partition", membershipReceipt.Partition), zap.Int64("membership_offset", membershipReceipt.Offset),
		zap.String("event_id", identity.JSONEventID))
	return nil
}

func seedCampaignRailJSONFixture(ctx context.Context, db *sql.DB, identity campaignRailCanaryIdentity) error {
	payloadJSON, err := campaignRailJSONFixturePayload(identity)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []struct {
		query string
		args  []interface{}
	}{
		{`INSERT INTO tenants(tenant_id,name) VALUES($1,$2) ON CONFLICT (tenant_id) DO NOTHING`, []interface{}{identity.TenantID, "M07 Campaign Rail Canary"}},
		{`INSERT INTO campaign_workbench_state(tenant_id,campaign_id,state_version,member_count,last_event_id)
		 VALUES($1,$2,2,1,$3::uuid) ON CONFLICT (tenant_id,campaign_id) DO NOTHING`, []interface{}{identity.TenantID, identity.AuthorityID, identity.JSONEventID}},
		{`INSERT INTO campaign_alert_links(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,idempotency_key)
		 VALUES($1::uuid,$2,$3,$4,'linked',1,2,$5,$6) ON CONFLICT (relation_id) DO NOTHING`, []interface{}{identity.RelationID, identity.TenantID, identity.AuthorityID, identity.AlertID, "isolated canary", identity.RunIDText}},
		{`INSERT INTO campaign_alert_link_history(event_id,relation_id,tenant_id,campaign_id,alert_id,event_type,revision,campaign_revision,payload)
		 VALUES($1::uuid,$2::uuid,$3,$4,$5,'linked',1,2,$6::jsonb) ON CONFLICT (event_id) DO NOTHING`, []interface{}{identity.JSONEventID, identity.RelationID, identity.TenantID, identity.AuthorityID, identity.AlertID, string(payloadJSON)}},
		{`INSERT INTO campaign_aggregate_history(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,member_count,payload,reason)
		 VALUES($1::uuid,$2,$3,2,'traffic.campaign.v2.AlertLinked','active',1,$4::jsonb,'isolated canary') ON CONFLICT (event_id) DO NOTHING`, []interface{}{identity.JSONEventID, identity.TenantID, identity.AuthorityID, string(payloadJSON)}},
		{`INSERT INTO campaign_alert_link_outbox(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload)
		 VALUES($1::uuid,$2,$3::uuid,1,'traffic.campaign.v2.AlertLinked',$4,$5::jsonb) ON CONFLICT (event_id) DO NOTHING`, []interface{}{identity.JSONEventID, identity.TenantID, identity.RelationID, identity.TenantID + ":" + identity.AuthorityID, string(payloadJSON)}},
		{`INSERT INTO campaign_aggregate_outbox(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload)
		 VALUES($1::uuid,$2,$3,2,'traffic.campaign.v2.AlertLinked',$4,$5::jsonb) ON CONFLICT (event_id) DO NOTHING`, []interface{}{identity.JSONEventID, identity.TenantID, identity.AuthorityID, identity.TenantID + ":" + identity.AuthorityID, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	var aggregateCount, membershipCount int
	if err := tx.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM campaign_aggregate_outbox WHERE event_id=$1::uuid AND published=false),
		(SELECT count(*) FROM campaign_alert_link_outbox WHERE event_id=$1::uuid AND published=false)`, identity.JSONEventID).
		Scan(&aggregateCount, &membershipCount); err != nil {
		return err
	}
	if aggregateCount != 1 || membershipCount != 1 {
		return fmt.Errorf("campaign JSON fixture did not produce exactly two pending outboxes")
	}
	return tx.Commit()
}

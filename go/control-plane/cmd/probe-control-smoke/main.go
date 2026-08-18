package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	alertconsumer "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/consumer"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

const (
	defaultTenantID = "default"
	defaultProbeID  = "probe-agent"
)

type acceptedResponse struct {
	Success bool `json:"success"`
	Data    struct {
		OperationID     string `json:"operation_id"`
		CommandRevision int64  `json:"command_revision"`
	} `json:"data"`
	Error interface{} `json:"error"`
}

type operationResult struct {
	OperationID     string   `json:"operation_id"`
	TenantID        string   `json:"tenant_id"`
	ProbeID         string   `json:"probe_id"`
	Status          string   `json:"status"`
	CommandRevision int64    `json:"command_revision"`
	CommandEventID  string   `json:"command_event_id"`
	AckEventID      string   `json:"ack_event_id"`
	HistoryStates   []string `json:"history_states"`
	AuditRows       int      `json:"audit_rows"`
	PendingOutbox   int      `json:"pending_outbox"`
	PublishedOutbox int      `json:"published_outbox"`
	AgentVersion    string   `json:"agent_version"`
	ReportedHash    string   `json:"reported_hash"`
	AcknowledgedAt  string   `json:"acknowledged_at"`
}

func main() {
	logger, err := logging.NewLogger(logging.Config{
		Level:       environment("LOG_LEVEL", "info"),
		Format:      "json",
		Output:      "stdout",
		Service:     "probe-control-smoke",
		Version:     environment("SERVICE_VERSION", "remediation"),
		Environment: "acceptance",
	})
	if err != nil {
		panic(err)
	}
	defer logging.Sync(logger)

	if err := run(logger); err != nil {
		logger.Error("F-PROBE G2 smoke failed", zap.Error(err))
		os.Exit(1)
	}
}

func run(logger *zap.Logger) error {
	cfg, err := alertconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tenantID := environment("PROBE_SMOKE_TENANT_ID", defaultTenantID)
	probeID := environment("PROBE_SMOKE_PROBE_ID", defaultProbeID)
	timeout, err := time.ParseDuration(environment("PROBE_SMOKE_TIMEOUT", "4m"))
	if err != nil || timeout <= 0 || timeout > 10*time.Minute {
		return fmt.Errorf("PROBE_SMOKE_TIMEOUT must be between 1ns and 10m")
	}

	db, err := sql.Open("postgres", cfg.Auth.ConnectionString())
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}

	if err := waitForProbe(ctx, db, tenantID, probeID); err != nil {
		return err
	}

	commandProducer, err := commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
		Brokers: cfg.Kafka.Brokers, Topic: "probe.control.v2", BatchSize: 1,
		RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
	}, logger)
	if err != nil {
		return fmt.Errorf("create command producer: %w", err)
	}
	defer commandProducer.Close()
	eventProducer, err := commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
		Brokers: cfg.Kafka.Brokers, Topic: "probe.events.v2", BatchSize: 1,
		RequiredAcks: "all", Compression: "lz4", Security: cfg.Kafka.Security,
	}, logger)
	if err != nil {
		return fmt.Errorf("create lifecycle producer: %w", err)
	}
	defer eventProducer.Close()

	handler := api.NewSystemHandler(nil, db, logger)
	handler.SetProbeOperationProducer(commandProducer)
	handler.SetProbeOperationEventProducer(eventProducer)
	if err := handler.StartProbeOperationOutboxWorker(ctx, 250*time.Millisecond); err != nil {
		return fmt.Errorf("start operation outbox: %w", err)
	}

	ackKafkaConsumer, err := commonkafka.NewConsumer(commonkafka.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers, Topic: cfg.Kafka.ProbeAckTopic,
		GroupID: cfg.Kafka.ProbeAckGroup, MaxRetries: 3, RetryBackoff: time.Second,
		StartOffset: segmentkafka.FirstOffset,
		EnableDLQ:   true, DLQTopicPrefix: "dlq.", CommitOnDLQSuccess: true,
		CommitOnHandlerError: false, Security: cfg.Kafka.Security,
	}, logger)
	if err != nil {
		return fmt.Errorf("create ACK consumer: %w", err)
	}
	ackConsumer, err := alertconsumer.NewProbeAckConsumer(ackKafkaConsumer, handler, logger)
	if err != nil {
		_ = ackKafkaConsumer.Close()
		return fmt.Errorf("initialize ACK consumer: %w", err)
	}
	defer ackConsumer.Close()
	consumeErrors := make(chan error, 1)
	go func() {
		consumeErrors <- ackConsumer.Start(ctx)
	}()

	idempotencyKey := "g2-probe-smoke-" + uuid.NewString()
	body := []byte(`{"targets":["ingest-gateway"],"reason":"F-PROBE-001 G2 canary"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/probes/"+probeID+"/connectivity-test", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request = mux.SetURLVars(request, map[string]string{"id": probeID})
	requestCtx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenantID)
	requestCtx = context.WithValue(requestCtx, httpx.ContextKeyPermissions, []string{authmodel.ScopeProbeWrite})
	requestCtx = context.WithValue(requestCtx, httpx.ContextKeyTraceID, uuid.NewString())
	request = request.WithContext(requestCtx)
	recorder := httptest.NewRecorder()
	handler.RunProbeConnectivityTest(recorder, request)
	if recorder.Code != http.StatusAccepted {
		return fmt.Errorf("connectivity handler returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var accepted acceptedResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		return fmt.Errorf("decode accepted response: %w", err)
	}
	if !accepted.Success || accepted.Data.OperationID == "" || accepted.Data.CommandRevision <= 0 {
		return fmt.Errorf("incomplete accepted response: %s", recorder.Body.String())
	}

	result, err := waitForOperation(ctx, db, tenantID, probeID, accepted.Data.OperationID)
	if err != nil {
		return err
	}
	output := map[string]interface{}{
		"schema_version": 1,
		"feature_id":     "F-PROBE-001",
		"gate":           "G2_G3_CANARY",
		"status":         "PASS",
		"trace_id":       httpx.GetTraceID(requestCtx),
		"operation":      result,
		"checks": map[string]bool{
			"http_handler_accepted":            true,
			"command_outbox_published":         result.CommandEventID != "",
			"agent_ack_receipt_persisted":      result.AckEventID != "",
			"operation_completed":              result.Status == "completed",
			"history_contains_completed":       contains(result.HistoryStates, "completed"),
			"audit_persisted":                  result.AuditRows > 0,
			"lifecycle_outbox_fully_published": result.PendingOutbox == 0 && result.PublishedOutbox >= 2,
		},
		"captured_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	encoded, _ := json.Marshal(output)
	fmt.Println(string(encoded))

	select {
	case consumeErr := <-consumeErrors:
		if consumeErr != nil && consumeErr != context.Canceled {
			return fmt.Errorf("ACK consumer stopped: %w", consumeErr)
		}
	default:
	}
	return nil
}

func waitForProbe(ctx context.Context, db *sql.DB, tenantID, probeID string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var exists bool
		if err := db.QueryRowContext(
			ctx,
			`SELECT EXISTS (SELECT 1 FROM probes WHERE tenant_id=$1 AND probe_id=$2)`,
			tenantID,
			probeID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("query canary probe: %w", err)
		}
		if exists {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for canary probe registration: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForOperation(
	ctx context.Context,
	db *sql.DB,
	tenantID, probeID, operationID string,
) (operationResult, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, err := loadOperation(ctx, db, tenantID, probeID, operationID)
		if err != nil {
			return operationResult{}, err
		}
		switch result.Status {
		case "completed":
			if result.AckEventID == "" || result.CommandEventID == "" ||
				result.AuditRows == 0 || result.PendingOutbox != 0 ||
				!contains(result.HistoryStates, "completed") {
				break
			}
			return result, nil
		case "failed", "expired", "stale", "cancelled":
			return operationResult{}, fmt.Errorf(
				"probe operation reached terminal status %s", result.Status,
			)
		}
		select {
		case <-ctx.Done():
			return operationResult{}, fmt.Errorf(
				"wait for probe operation %s: %w", operationID, ctx.Err(),
			)
		case <-ticker.C:
		}
	}
}

func loadOperation(
	ctx context.Context,
	db *sql.DB,
	tenantID, probeID, operationID string,
) (operationResult, error) {
	var result operationResult
	var historyJSON []byte
	err := db.QueryRowContext(ctx, `
		SELECT o.operation_id::text,o.tenant_id,o.probe_id,o.status,o.command_revision,
		       COALESCE((SELECT event_id::text FROM probe_operation_outbox
		         WHERE operation_id=o.operation_id AND event_type='traffic.probe.v2.OperationRequested'),''),
		       COALESCE((SELECT ack_id::text FROM probe_operation_ack_receipts
		         WHERE operation_id=o.operation_id),''),
		       COALESCE((SELECT json_agg(to_status ORDER BY state_revision) FROM probe_operation_history
		         WHERE operation_id=o.operation_id),'[]'::json),
		       (SELECT count(*) FROM audit_logs WHERE tenant_id=o.tenant_id
		         AND action='PROBE_CONNECTIVITY_TEST_QUEUED'
		         AND detail->>'operation_id'=o.operation_id::text),
		       (SELECT count(*) FROM probe_operation_outbox
		         WHERE operation_id=o.operation_id AND published=false),
		       (SELECT count(*) FROM probe_operation_outbox
		         WHERE operation_id=o.operation_id AND published=true),
		       o.agent_version,o.reported_hash,
		       COALESCE(to_char(o.acknowledged_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'')
		FROM probe_operations o
		WHERE o.tenant_id=$1 AND o.probe_id=$2 AND o.operation_id=$3::uuid`,
		tenantID,
		probeID,
		operationID,
	).Scan(
		&result.OperationID,
		&result.TenantID,
		&result.ProbeID,
		&result.Status,
		&result.CommandRevision,
		&result.CommandEventID,
		&result.AckEventID,
		&historyJSON,
		&result.AuditRows,
		&result.PendingOutbox,
		&result.PublishedOutbox,
		&result.AgentVersion,
		&result.ReportedHash,
		&result.AcknowledgedAt,
	)
	if err != nil {
		return result, fmt.Errorf("load probe operation: %w", err)
	}
	if err := json.Unmarshal(historyJSON, &result.HistoryStates); err != nil {
		return result, fmt.Errorf("decode operation history: %w", err)
	}
	return result, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func environment(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

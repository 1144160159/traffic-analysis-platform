package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

const (
	defaultFeedbackInboxBatchSize   = 50
	defaultFeedbackInboxMaxAttempts = 8
	feedbackV1Table                 = "traffic.alert_feedback"
	defaultFeedbackV2Table          = "traffic.alert_feedback_v2"
)

var modelFeedbackProjectionTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "traffic",
		Subsystem: "model_feedback",
		Name:      "clickhouse_projection_total",
		Help:      "Model feedback ClickHouse projection attempts by target and outcome.",
	},
	[]string{"target", "outcome"},
)

// ModelFeedbackProjectionOptions controls the additive deterministic V2
// projection. V2 is fail-closed when enabled and disabled by default.
type ModelFeedbackProjectionOptions struct {
	V2Enabled bool
	V2Table   string
}

// ModelFeedbackInboxWorker materializes the durable PostgreSQL feedback inbox
// into the existing ClickHouse traffic.alert_feedback training source.
//
// PostgreSQL remains authoritative. The ClickHouse write and PostgreSQL
// acknowledgement cannot share a transaction, so retries first reconcile the
// deterministic feedback_id. Exact rows are acknowledged, conflicting rows are
// dead-lettered, and missing rows are inserted with a stable ClickHouse
// deduplication token before being read back.
type ModelFeedbackInboxWorker struct {
	pg          *sql.DB
	clickhouse  *sql.DB
	logger      *zap.Logger
	workerID    string
	batchSize   int
	maxAttempts int
	v2Enabled   bool
	v2Table     string
}

type modelFeedbackInboxItem struct {
	FeedbackID     string
	EventID        string
	TenantID       string
	AlertID        string
	UserID         string
	Label          string
	ReasonCode     string
	ModelVersion   string
	RuleVersion    string
	EventTimestamp int64
	Payload        []byte
}

type modelFeedbackPayload struct {
	EventID          string   `json:"event_id"`
	EventType        string   `json:"event_type"`
	SchemaVersion    int      `json:"schema_version"`
	AggregateVersion int64    `json:"aggregate_version"`
	FeedbackID       string   `json:"feedback_id"`
	AlertID          string   `json:"alert_id"`
	TenantID         string   `json:"tenant_id"`
	UserID           string   `json:"user_id"`
	Label            string   `json:"label"`
	ReasonCode       string   `json:"reason_code"`
	Comment          string   `json:"comment"`
	AddToWhitelist   bool     `json:"add_to_whitelist"`
	AlertType        string   `json:"alert_type"`
	Severity         string   `json:"severity"`
	Labels           []string `json:"labels"`
	ModelVersion     string   `json:"model_version"`
	RuleVersion      string   `json:"rule_version"`
	Timestamp        int64    `json:"timestamp"`
}

func NewModelFeedbackInboxWorker(
	pg *sql.DB,
	clickhouse *sql.DB,
	logger *zap.Logger,
) (*ModelFeedbackInboxWorker, error) {
	return NewModelFeedbackInboxWorkerWithOptions(
		pg, clickhouse, logger, ModelFeedbackProjectionOptions{},
	)
}

func NewModelFeedbackInboxWorkerWithOptions(
	pg *sql.DB,
	clickhouse *sql.DB,
	logger *zap.Logger,
	options ModelFeedbackProjectionOptions,
) (*ModelFeedbackInboxWorker, error) {
	if pg == nil || clickhouse == nil {
		return nil, fmt.Errorf("model feedback inbox PostgreSQL and ClickHouse databases are required")
	}
	v2Table := strings.TrimSpace(options.V2Table)
	if v2Table == "" {
		v2Table = defaultFeedbackV2Table
	}
	if v2Table != defaultFeedbackV2Table {
		return nil, fmt.Errorf("unsupported model feedback V2 table %q", v2Table)
	}
	return &ModelFeedbackInboxWorker{
		pg: pg, clickhouse: clickhouse, logger: logger,
		workerID: uuid.NewString(), batchSize: defaultFeedbackInboxBatchSize,
		maxAttempts: defaultFeedbackInboxMaxAttempts,
		v2Enabled:   options.V2Enabled, v2Table: v2Table,
	}, nil
}

func (worker *ModelFeedbackInboxWorker) VerifySchema(ctx context.Context) error {
	var pgColumns int
	if err := worker.pg.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='model_feedback_inbox'
		  AND column_name IN ('feedback_id','event_id','tenant_id','alert_id','user_id',
		    'label','reason_code','model_version','rule_version','event_timestamp_ms',
		    'payload','status','attempts','last_error','next_attempt_at','locked_until',
		    'locked_by','applied_at','created_at','updated_at')`,
	).Scan(&pgColumns); err != nil {
		return fmt.Errorf("verify model feedback inbox schema: %w", err)
	}
	if pgColumns != 20 {
		return fmt.Errorf("model feedback inbox schema is incomplete: columns=%d want=20", pgColumns)
	}
	if err := worker.verifyClickHouseSchema(ctx, feedbackV1Table); err != nil {
		return err
	}
	if worker.v2Enabled {
		if err := worker.verifyClickHouseSchema(ctx, worker.v2Table); err != nil {
			return err
		}
	}
	return nil
}

func (worker *ModelFeedbackInboxWorker) verifyClickHouseSchema(ctx context.Context, table string) error {
	var chColumns int
	if err := worker.clickhouse.QueryRowContext(ctx, `
		SELECT count() FROM system.columns
		WHERE database='traffic' AND table=?
		  AND name IN ('feedback_id','alert_id','tenant_id','user_id','label',
		    'reason_code','comment','add_to_whitelist','alert_type','severity',
		    'model_version','rule_version','created_at')`,
		strings.TrimPrefix(table, "traffic."),
	).Scan(&chColumns); err != nil {
		return fmt.Errorf("verify ClickHouse model feedback schema %s: %w", table, err)
	}
	if chColumns != 13 {
		return fmt.Errorf("ClickHouse model feedback schema %s is incomplete: columns=%d want=13", table, chColumns)
	}
	return nil
}

func (worker *ModelFeedbackInboxWorker) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := worker.Drain(ctx); err != nil && ctx.Err() == nil && worker.logger != nil {
				worker.logger.Warn("Failed to drain model feedback inbox", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (worker *ModelFeedbackInboxWorker) Drain(ctx context.Context) (int, error) {
	items, err := worker.claim(ctx)
	if err != nil {
		return 0, err
	}
	applied := 0
	for _, item := range items {
		if err := worker.project(ctx, item); err != nil {
			if markErr := worker.markFailure(ctx, item.FeedbackID, err); markErr != nil {
				return applied, errors.Join(err, markErr)
			}
			if worker.logger != nil {
				worker.logger.Warn(
					"Model feedback projection failed",
					zap.String("feedback_id", item.FeedbackID),
					zap.Error(err),
				)
			}
			continue
		}
		if err := worker.markApplied(ctx, item.FeedbackID); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

func (worker *ModelFeedbackInboxWorker) claim(ctx context.Context) ([]modelFeedbackInboxItem, error) {
	rows, err := worker.pg.QueryContext(ctx, `
		WITH candidates AS (
			SELECT feedback_id FROM model_feedback_inbox
			WHERE (
				status IN ('pending','failed') AND next_attempt_at <= now()
			) OR (
				status='processing' AND locked_until < now()
			)
			ORDER BY next_attempt_at,created_at,feedback_id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE model_feedback_inbox inbox
			SET status='processing',attempts=attempts+1,
			    locked_until=now()+interval '60 seconds',locked_by=$2,updated_at=now()
			FROM candidates
			WHERE inbox.feedback_id=candidates.feedback_id
			RETURNING inbox.feedback_id::text,inbox.event_id::text,inbox.tenant_id,
			          inbox.alert_id,inbox.user_id,inbox.label,inbox.reason_code,
			          inbox.model_version,inbox.rule_version,inbox.event_timestamp_ms,
			          inbox.payload::text
		)
		SELECT * FROM claimed ORDER BY feedback_id`,
		worker.batchSize, worker.workerID,
	)
	if err != nil {
		return nil, fmt.Errorf("claim model feedback inbox: %w", err)
	}
	defer rows.Close()
	items := make([]modelFeedbackInboxItem, 0, worker.batchSize)
	for rows.Next() {
		var item modelFeedbackInboxItem
		var payload string
		if err := rows.Scan(
			&item.FeedbackID, &item.EventID, &item.TenantID, &item.AlertID,
			&item.UserID, &item.Label, &item.ReasonCode, &item.ModelVersion,
			&item.RuleVersion, &item.EventTimestamp, &payload,
		); err != nil {
			return nil, fmt.Errorf("scan model feedback inbox: %w", err)
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model feedback inbox: %w", err)
	}
	return items, nil
}

func (worker *ModelFeedbackInboxWorker) project(
	ctx context.Context,
	item modelFeedbackInboxItem,
) error {
	payload, err := validateModelFeedbackInboxItem(item)
	if err != nil {
		return err
	}
	if err := worker.projectTarget(ctx, feedbackV1Table, "v1", item, payload); err != nil {
		return err
	}
	if worker.v2Enabled {
		if err := worker.projectTarget(ctx, worker.v2Table, "v2", item, payload); err != nil {
			return err
		}
	}
	return nil
}

func (worker *ModelFeedbackInboxWorker) projectTarget(
	ctx context.Context,
	table string,
	target string,
	item modelFeedbackInboxItem,
	payload modelFeedbackPayload,
) error {
	found, exact, err := worker.lookupClickHouseFeedback(ctx, table, item, payload)
	if err != nil {
		modelFeedbackProjectionTotal.WithLabelValues(target, "error").Inc()
		return err
	}
	if found {
		if !exact {
			modelFeedbackProjectionTotal.WithLabelValues(target, "conflict").Inc()
			return fmt.Errorf("ClickHouse feedback_id collision in %s", table)
		}
		modelFeedbackProjectionTotal.WithLabelValues(target, "existing").Inc()
		return nil
	}

	// feedback_id is a validated UUID. Keep it as the stable replicated-block
	// deduplication token so a timeout after an accepted insert can be retried.
	deduplicationToken := item.FeedbackID
	if target != "v1" {
		deduplicationToken += ":" + target
	}
	var insertTemplate string
	switch table {
	case feedbackV1Table:
		insertTemplate = `
			INSERT INTO traffic.alert_feedback
				(feedback_id,alert_id,tenant_id,user_id,label,reason_code,comment,
				 add_to_whitelist,alert_type,severity,model_version,rule_version,created_at)
			SETTINGS insert_deduplication_token='%s'
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	case defaultFeedbackV2Table:
		insertTemplate = `
			INSERT INTO traffic.alert_feedback_v2
				(feedback_id,alert_id,tenant_id,user_id,label,reason_code,comment,
				 add_to_whitelist,alert_type,severity,model_version,rule_version,created_at)
			SETTINGS insert_deduplication_token='%s'
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	default:
		return fmt.Errorf("unsupported model feedback projection table %q", table)
	}
	insert := fmt.Sprintf(insertTemplate, deduplicationToken)
	if _, err := worker.clickhouse.ExecContext(
		ctx, insert,
		item.FeedbackID, item.AlertID, item.TenantID, item.UserID,
		item.Label, item.ReasonCode, payload.Comment, payload.AddToWhitelist,
		payload.AlertType, payload.Severity, item.ModelVersion, item.RuleVersion,
		time.UnixMilli(item.EventTimestamp).UTC(),
	); err != nil {
		modelFeedbackProjectionTotal.WithLabelValues(target, "error").Inc()
		return fmt.Errorf("insert ClickHouse model feedback into %s: %w", table, err)
	}
	found, exact, err = worker.lookupClickHouseFeedback(ctx, table, item, payload)
	if err != nil {
		modelFeedbackProjectionTotal.WithLabelValues(target, "error").Inc()
		return err
	}
	if !found || !exact {
		modelFeedbackProjectionTotal.WithLabelValues(target, "error").Inc()
		return fmt.Errorf("ClickHouse model feedback read-after-write reconciliation failed in %s", table)
	}
	modelFeedbackProjectionTotal.WithLabelValues(target, "inserted").Inc()
	return nil
}

func validateModelFeedbackInboxItem(item modelFeedbackInboxItem) (modelFeedbackPayload, error) {
	var payload modelFeedbackPayload
	decoder := json.NewDecoder(strings.NewReader(string(item.Payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, fmt.Errorf("decode model feedback inbox payload: %w", err)
	}
	if _, err := uuid.Parse(item.FeedbackID); err != nil {
		return payload, fmt.Errorf("invalid model feedback inbox feedback_id")
	}
	if payload.FeedbackID != item.FeedbackID || payload.EventID != item.EventID ||
		payload.EventType != "alert.feedback.v1" || payload.SchemaVersion != 1 ||
		payload.AggregateVersion != 1 ||
		payload.TenantID != item.TenantID || payload.AlertID != item.AlertID ||
		payload.UserID != item.UserID || payload.Label != item.Label ||
		payload.ReasonCode != item.ReasonCode || payload.ModelVersion != item.ModelVersion ||
		payload.RuleVersion != item.RuleVersion || payload.Timestamp != item.EventTimestamp {
		return payload, fmt.Errorf("model feedback inbox payload/column mismatch")
	}
	if item.Label != "TP" && item.Label != "FP" {
		return payload, fmt.Errorf("invalid model feedback inbox label")
	}
	return payload, nil
}

func (worker *ModelFeedbackInboxWorker) lookupClickHouseFeedback(
	ctx context.Context,
	table string,
	item modelFeedbackInboxItem,
	payload modelFeedbackPayload,
) (bool, bool, error) {
	query := fmt.Sprintf(`
		SELECT feedback_id,alert_id,tenant_id,user_id,label,reason_code,comment,
		       add_to_whitelist,alert_type,severity,model_version,rule_version,created_at
		FROM %s
		WHERE feedback_id=? LIMIT 2`, table)
	rows, err := worker.clickhouse.QueryContext(ctx, query,
		item.FeedbackID,
	)
	if err != nil {
		return false, false, fmt.Errorf("reconcile ClickHouse model feedback in %s: %w", table, err)
	}
	defer rows.Close()
	count := 0
	exact := true
	for rows.Next() {
		count++
		var feedbackID, alertID, tenantID, userID, label, reasonCode, comment string
		var addToWhitelist bool
		var alertType, severity, modelVersion, ruleVersion string
		var createdAt time.Time
		if err := rows.Scan(
			&feedbackID, &alertID, &tenantID, &userID, &label, &reasonCode,
			&comment, &addToWhitelist, &alertType, &severity, &modelVersion,
			&ruleVersion, &createdAt,
		); err != nil {
			return false, false, fmt.Errorf("scan ClickHouse model feedback in %s: %w", table, err)
		}
		exact = exact &&
			feedbackID == item.FeedbackID && alertID == item.AlertID &&
			tenantID == item.TenantID && userID == item.UserID &&
			label == item.Label && reasonCode == item.ReasonCode &&
			comment == payload.Comment && addToWhitelist == payload.AddToWhitelist &&
			alertType == payload.AlertType && severity == payload.Severity &&
			modelVersion == item.ModelVersion && ruleVersion == item.RuleVersion &&
			createdAt.Unix() == time.UnixMilli(item.EventTimestamp).Unix()
	}
	if err := rows.Err(); err != nil {
		return false, false, fmt.Errorf("iterate ClickHouse model feedback in %s: %w", table, err)
	}
	return count > 0, count == 1 && exact, nil
}

func (worker *ModelFeedbackInboxWorker) markApplied(ctx context.Context, feedbackID string) error {
	result, err := worker.pg.ExecContext(ctx, `
		UPDATE model_feedback_inbox
		SET status='applied',last_error='',locked_until=NULL,locked_by='',
		    applied_at=now(),updated_at=now()
		WHERE feedback_id=$1::uuid AND status='processing' AND locked_by=$2`,
		feedbackID, worker.workerID,
	)
	if err != nil {
		return fmt.Errorf("acknowledge model feedback projection: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect model feedback acknowledgement: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("model feedback inbox lease lost before acknowledgement")
	}
	return nil
}

func (worker *ModelFeedbackInboxWorker) markFailure(
	ctx context.Context,
	feedbackID string,
	projectionErr error,
) error {
	message := strings.TrimSpace(projectionErr.Error())
	if len(message) > 2000 {
		message = message[:2000]
	}
	result, err := worker.pg.ExecContext(ctx, `
		UPDATE model_feedback_inbox
		SET status=CASE WHEN attempts >= $3 THEN 'dead_letter' ELSE 'failed' END,
		    last_error=$4,
		    next_attempt_at=now()+
		      (LEAST(300,POWER(2,LEAST(attempts,8)))::text || ' seconds')::interval,
		    locked_until=NULL,locked_by='',updated_at=now()
		WHERE feedback_id=$1::uuid AND status='processing' AND locked_by=$2`,
		feedbackID, worker.workerID, worker.maxAttempts, message,
	)
	if err != nil {
		return fmt.Errorf("record model feedback projection failure: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect model feedback failure acknowledgement: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("model feedback inbox lease lost before failure acknowledgement")
	}
	return nil
}

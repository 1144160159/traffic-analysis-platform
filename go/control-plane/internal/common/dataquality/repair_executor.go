package dataquality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RepairReplayRequest contains only server-loaded and revalidated execution
// inputs. HTTP payloads never reach a replay driver directly.
type RepairReplayRequest struct {
	TenantID       string
	RepairID       string
	OperationID    string
	InputScope     map[string]interface{}
	ResourceBudget map[string]interface{}
	Revision       int64
	TraceID        string
}

// RepairReplayDriver performs one idempotent bounded replay. Implementations
// must use repair_id plus the stable source event identity as their dedupe key.
type RepairReplayDriver interface {
	Ready(context.Context) error
	Replay(context.Context, RepairReplayRequest) (map[string]interface{}, error)
}

// RepairExecutionWorker treats PostgreSQL status=executing as the durable
// queue. A session advisory lock prevents two replicas from processing the
// same repair concurrently; a crash releases the lock and leaves the row
// eligible for retry.
type RepairExecutionWorker struct {
	db       *sql.DB
	monitor  *Monitor
	driver   RepairReplayDriver
	interval time.Duration
	logger   *zap.Logger
}

func NewRepairExecutionWorker(db *sql.DB, monitor *Monitor, driver RepairReplayDriver, interval time.Duration, logger *zap.Logger) *RepairExecutionWorker {
	if interval <= 0 || interval > time.Minute {
		interval = 5 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RepairExecutionWorker{db: db, monitor: monitor, driver: driver, interval: interval, logger: logger}
}

func (w *RepairExecutionWorker) Ready(ctx context.Context) error {
	if w == nil || w.db == nil || w.monitor == nil || w.driver == nil {
		return fmt.Errorf("repair execution worker dependencies are unavailable")
	}
	if maximum := w.db.Stats().MaxOpenConnections; maximum == 1 {
		return fmt.Errorf("repair execution worker requires at least two PostgreSQL connections")
	}
	if err := w.db.PingContext(ctx); err != nil {
		return fmt.Errorf("repair execution PostgreSQL is unavailable: %w", err)
	}
	if err := w.driver.Ready(ctx); err != nil {
		return fmt.Errorf("repair replay driver is unavailable: %w", err)
	}
	return nil
}

func (w *RepairExecutionWorker) Run(ctx context.Context) {
	if w == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("data quality repair execution scan failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *RepairExecutionWorker) RunOnce(ctx context.Context) error {
	if err := w.Ready(ctx); err != nil {
		return err
	}
	rows, err := w.db.QueryContext(ctx, `
		SELECT tenant_id,repair_id::text
		FROM data_quality_repairs
		WHERE status='executing'
		ORDER BY updated_at,repair_id
		LIMIT 32
	`)
	if err != nil {
		return fmt.Errorf("scan executing repairs: %w", err)
	}
	var candidates [][2]string
	for rows.Next() {
		var candidate [2]string
		if err := rows.Scan(&candidate[0], &candidate[1]); err != nil {
			rows.Close()
			return fmt.Errorf("scan executing repair identity: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close executing repair scan: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate executing repair scan: %w", err)
	}
	var failures []error
	for _, candidate := range candidates {
		if err := w.executeCandidate(ctx, candidate[0], candidate[1]); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (w *RepairExecutionWorker) executeCandidate(ctx context.Context, tenantID, repairID string) error {
	conn, err := w.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire repair execution connection: %w", err)
	}
	defer conn.Close()
	lockKey := "data-quality-repair:" + tenantID + ":" + repairID
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, lockKey).Scan(&locked); err != nil {
		return fmt.Errorf("acquire repair execution lock: %w", err)
	}
	if !locked {
		return nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if unlockErr := conn.QueryRowContext(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, lockKey).Scan(&unlocked); unlockErr != nil || !unlocked {
			w.logger.Error("release data quality repair execution lock", zap.String("repair_id", repairID), zap.Error(unlockErr))
		}
	}()

	request, err := loadRepairReplayRequest(ctx, conn, tenantID, repairID)
	if errors.Is(err, ErrRepairConflict) || errors.Is(err, ErrRepairNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	started := time.Now().UTC()
	summary, replayErr := w.driver.Replay(ctx, request)
	if summary == nil {
		summary = map[string]interface{}{}
	}
	summary["repair_id"] = request.RepairID
	summary["operation_id"] = request.OperationID
	summary["execution_started_at"] = started.Format(time.RFC3339Nano)
	summary["execution_finished_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	summary["server_derived"] = true
	action := "record_executed"
	reason := "record successful bounded replay execution outcome"
	if replayErr != nil {
		action = "record_failed"
		reason = "record failed bounded replay execution outcome"
		summary["published"] = false
		summary["error_class"] = classifyReplayError(replayErr)
	}
	key := fmt.Sprintf("dq-replay-outcome:%s:%d", request.RepairID, request.Revision)
	_, persistErr := w.monitor.TransitionRepair(ctx, RepairTransitionCommand{
		TenantID: tenantID, RepairID: repairID, Action: action, ExpectedRevision: request.Revision,
		Summary: summary, ActionID: "repair-executor-" + action, IdempotencyKey: key,
		Reason: reason, Actor: "system:data-quality-repair-executor", TraceID: request.TraceID,
	}, true)
	if replayErr != nil {
		if persistErr != nil {
			return errors.Join(fmt.Errorf("bounded replay failed: %w", replayErr), fmt.Errorf("persist replay failure: %w", persistErr))
		}
		return fmt.Errorf("bounded replay failed: %w", replayErr)
	}
	if persistErr != nil {
		return fmt.Errorf("persist successful replay outcome: %w", persistErr)
	}
	return nil
}

func loadRepairReplayRequest(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, tenantID, repairID string) (RepairReplayRequest, error) {
	request := RepairReplayRequest{TenantID: tenantID, RepairID: repairID}
	var status string
	var scopeJSON, budgetJSON []byte
	err := queryer.QueryRowContext(ctx, `
		SELECT operation_id,status,input_scope,resource_budget,revision,trace_id
		FROM data_quality_repairs
		WHERE tenant_id=$1 AND repair_id=$2
	`, tenantID, repairID).Scan(&request.OperationID, &status, &scopeJSON, &budgetJSON, &request.Revision, &request.TraceID)
	if errors.Is(err, sql.ErrNoRows) {
		return request, ErrRepairNotFound
	}
	if err != nil {
		return request, fmt.Errorf("load executing repair: %w", err)
	}
	if status != "executing" || request.OperationID != "flow_replay_window_v1" {
		return request, ErrRepairConflict
	}
	if err := json.Unmarshal(scopeJSON, &request.InputScope); err != nil {
		return request, fmt.Errorf("decode executing repair scope: %w", err)
	}
	if err := json.Unmarshal(budgetJSON, &request.ResourceBudget); err != nil {
		return request, fmt.Errorf("decode executing repair budget: %w", err)
	}
	if err := validateRepairScope(tenantID, request.InputScope, request.ResourceBudget); err != nil {
		return request, fmt.Errorf("persisted executing repair scope failed validation: %w", err)
	}
	return request, nil
}

func classifyReplayError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(value, "timeout"):
		return "timeout"
	case strings.Contains(value, "budget"):
		return "budget_exceeded"
	default:
		return "execution_failed"
	}
}

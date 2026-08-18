package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/converter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"go.uber.org/zap"
)

// RuleUpdateAppliedAck is emitted once by every Flink rule-matcher subtask.
// Broker publication and one subtask receipt are not runtime-application proof.
type RuleUpdateAppliedAck struct {
	SchemaVersion  int    `json:"schema_version"`
	EventID        string `json:"event_id"`
	TenantID       string `json:"tenant_id"`
	RuleID         string `json:"rule_id"`
	Version        int64  `json:"version"`
	CurrentVersion int64  `json:"current_version"`
	Action         string `json:"action"`
	Checksum       string `json:"checksum"`
	SubtaskIndex   int    `json:"subtask_index"`
	Parallelism    int    `json:"parallelism"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	Timestamp      string `json:"timestamp"`
}

func validateRuleUpdateAppliedAck(ack RuleUpdateAppliedAck, expectedParallelism int) error {
	if ack.SchemaVersion != 1 {
		return fmt.Errorf("unsupported rule acknowledgement schema_version %d", ack.SchemaVersion)
	}
	if strings.TrimSpace(ack.EventID) == "" || strings.TrimSpace(ack.TenantID) == "" ||
		strings.TrimSpace(ack.RuleID) == "" || ack.Version <= 0 {
		return fmt.Errorf("rule acknowledgement is missing event, tenant, rule or version")
	}
	switch ack.Status {
	case "applied", "duplicate", "stale", "conflict":
	default:
		return fmt.Errorf("invalid rule acknowledgement status %q", ack.Status)
	}
	if ack.Parallelism != expectedParallelism {
		return fmt.Errorf("rule acknowledgement parallelism %d does not match server contract %d",
			ack.Parallelism, expectedParallelism)
	}
	if ack.SubtaskIndex < 0 || ack.SubtaskIndex >= ack.Parallelism {
		return fmt.Errorf("invalid rule acknowledgement subtask %d/%d",
			ack.SubtaskIndex, ack.Parallelism)
	}
	checksum := strings.TrimSpace(ack.Checksum)
	if len(checksum) != 32 || checksum != ack.Checksum || checksum != strings.ToLower(checksum) {
		return fmt.Errorf("invalid rule acknowledgement checksum")
	}
	for _, char := range checksum {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return fmt.Errorf("invalid rule acknowledgement checksum")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, ack.Timestamp); err != nil {
		return fmt.Errorf("invalid rule acknowledgement timestamp: %w", err)
	}
	return nil
}

// HandleRuleUpdateAppliedAck persists one receipt and only marks the outbox
// event applied when the exact subtask set [0, expectedParallelism) has
// successful applied/duplicate receipts and no stale/conflict receipt.
func (s *RuleService) HandleRuleUpdateAppliedAck(ctx context.Context, payload []byte) error {
	var ack RuleUpdateAppliedAck
	if err := json.Unmarshal(payload, &ack); err != nil {
		return fmt.Errorf("decode rule update acknowledgement: %w", err)
	}
	expectedParallelism := s.config.AppliedAckExpectedParallelism
	if expectedParallelism <= 0 {
		return fmt.Errorf("rule applied acknowledgement parallelism contract is not configured")
	}
	if err := validateRuleUpdateAppliedAck(ack, expectedParallelism); err != nil {
		return err
	}
	if s.db == nil {
		return fmt.Errorf("rule applied acknowledgement database is not configured")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rule applied acknowledgement: %w", err)
	}
	defer tx.Rollback()

	var ruleID string
	var commandPayload []byte
	var published bool
	if err := tx.QueryRowContext(ctx, `
		SELECT rule_id, payload, published
		FROM rule_outbox
		WHERE payload->>'event_id' = $1
		ORDER BY id DESC LIMIT 1
		FOR UPDATE
	`, ack.EventID).Scan(&ruleID, &commandPayload, &published); err != nil {
		return fmt.Errorf("resolve rule outbox event %s: %w", ack.EventID, err)
	}
	if !published {
		return fmt.Errorf("rule outbox event %s has no broker publication receipt", ack.EventID)
	}
	var command model.RuleCommand
	if err := json.Unmarshal(commandPayload, &command); err != nil {
		return fmt.Errorf("decode rule outbox event %s: %w", ack.EventID, err)
	}
	if command.Rule == nil {
		return fmt.Errorf("rule outbox event %s has no rule", ack.EventID)
	}
	expectedChecksum := converter.CommandToProto(&command).Checksum
	if command.EventID != ack.EventID || ruleID != ack.RuleID ||
		command.Rule.RuleID != ack.RuleID || command.Rule.TenantID != ack.TenantID ||
		command.Rule.Version != ack.Version || command.Action != ack.Action ||
		!strings.EqualFold(expectedChecksum, ack.Checksum) {
		return fmt.Errorf("rule acknowledgement does not match outbox contract for event %s", ack.EventID)
	}

	rawAck, err := json.Marshal(ack)
	if err != nil {
		return fmt.Errorf("encode rule update acknowledgement: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rule_update_applied_acks (
			event_id, tenant_id, rule_id, rule_version, action, checksum,
			subtask_index, parallelism, status, current_version, error, payload, acknowledged_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,now())
		ON CONFLICT (event_id, subtask_index) DO UPDATE SET
			status = CASE
				WHEN rule_update_applied_acks.status IN ('stale','conflict')
				THEN rule_update_applied_acks.status ELSE EXCLUDED.status END,
			current_version = GREATEST(rule_update_applied_acks.current_version, EXCLUDED.current_version),
			error = CASE
				WHEN rule_update_applied_acks.status IN ('stale','conflict')
				THEN rule_update_applied_acks.error ELSE EXCLUDED.error END,
			payload = EXCLUDED.payload,
			acknowledged_at = now()
	`, ack.EventID, ack.TenantID, ack.RuleID, ack.Version, ack.Action, ack.Checksum,
		ack.SubtaskIndex, ack.Parallelism, ack.Status, ack.CurrentVersion, ack.Error,
		string(rawAck)); err != nil {
		return fmt.Errorf("persist rule update acknowledgement: %w", err)
	}

	var successCount, minSuccessSubtask, maxSuccessSubtask int
	var hasFailure bool
	var failureReason string
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT subtask_index) FILTER (WHERE status IN ('applied','duplicate')),
		       COALESCE(BOOL_OR(status IN ('stale','conflict')), false),
		       COALESCE(MAX(NULLIF(error, '')) FILTER (WHERE status IN ('stale','conflict')), ''),
		       COALESCE(MIN(subtask_index) FILTER (WHERE status IN ('applied','duplicate')), -1),
		       COALESCE(MAX(subtask_index) FILTER (WHERE status IN ('applied','duplicate')), -1)
		FROM rule_update_applied_acks WHERE event_id = $1
	`, ack.EventID).Scan(
		&successCount, &hasFailure, &failureReason, &minSuccessSubtask, &maxSuccessSubtask); err != nil {
		return fmt.Errorf("aggregate rule update acknowledgements: %w", err)
	}

	runtimeStatus := ruleAckRuntimeStatus(
		successCount, minSuccessSubtask, maxSuccessSubtask, hasFailure, expectedParallelism)
	if runtimeStatus != "failed" {
		failureReason = ""
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE rule_outbox SET
			runtime_status = $2,
			runtime_applied_at = CASE WHEN $2 = 'applied' THEN now() ELSE runtime_applied_at END,
			runtime_last_error = $3
		WHERE payload->>'event_id' = $1
	`, ack.EventID, runtimeStatus, failureReason); err != nil {
		return fmt.Errorf("update rule outbox runtime status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rule update acknowledgement: %w", err)
	}
	return nil
}

func ruleAckRuntimeStatus(
	successCount, minSuccessSubtask, maxSuccessSubtask int,
	hasFailure bool,
	expectedParallelism int,
) string {
	if hasFailure {
		return "failed"
	}
	if successCount == expectedParallelism && minSuccessSubtask == 0 &&
		maxSuccessSubtask == expectedParallelism-1 {
		return "applied"
	}
	return "partial"
}

func (s *RuleService) startRuleAppliedAckTimeoutProcessor() {
	ticker := time.NewTicker(s.config.AppliedAckSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sweepCtx, sweepCancel := context.WithTimeout(context.Background(), 30*time.Second)
			count, err := s.expireTimedOutRuleApplications(sweepCtx, time.Now())
			sweepCancel()
			if err != nil {
				s.logger.Error("Failed to expire timed-out rule application acknowledgements", zap.Error(err))
				continue
			}
			if count > 0 {
				s.logger.Warn("Rule application acknowledgement deadline exceeded",
					zap.Int64("expired_events", count),
					zap.Duration("timeout", s.config.AppliedAckTimeout))
			}
		case <-s.outboxStopCh:
			return
		}
	}
}

func (s *RuleService) expireTimedOutRuleApplications(ctx context.Context, now time.Time) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("rule application acknowledgement database is not configured")
	}
	if s.config.AppliedAckTimeout <= 0 {
		return 0, fmt.Errorf("rule application acknowledgement timeout is not configured")
	}
	cutoff := now.Add(-s.config.AppliedAckTimeout)
	message := fmt.Sprintf("ACK_TIMEOUT: exact Flink subtask set not received within %s", s.config.AppliedAckTimeout)
	result, err := s.db.ExecContext(ctx, `
		UPDATE rule_outbox
		SET runtime_status = 'failed', runtime_last_error = $2
		WHERE published = true
		  AND published_at IS NOT NULL
		  AND published_at < $1
		  AND runtime_status IN ('pending','partial')
		  AND payload ? 'event_id'
	`, cutoff, message)
	if err != nil {
		return 0, fmt.Errorf("expire rule application acknowledgements: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect expired rule application acknowledgements: %w", err)
	}
	return count, nil
}

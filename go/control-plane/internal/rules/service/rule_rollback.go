package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
	"github.com/google/uuid"
)

type RuleRollbackResult struct {
	Rule            *model.Rule `json:"rule"`
	EventID         string      `json:"event_id"`
	RuntimeStatus   string      `json:"runtime_status"`
	ExpectedAcks    int         `json:"expected_acks"`
	TargetVersion   int64       `json:"target_version"`
	PreviousVersion int64       `json:"previous_version"`
	NewVersion      int64       `json:"new_version"`
}

type RuleApplicationStatus struct {
	EventID          string     `json:"event_id"`
	RuleID           string     `json:"rule_id"`
	Version          int64      `json:"version"`
	Action           string     `json:"action"`
	BrokerPublished  bool       `json:"broker_published"`
	RuntimeStatus    string     `json:"runtime_status"`
	ExpectedAcks     int        `json:"expected_acks"`
	ReceivedAcks     int        `json:"received_acks"`
	SuccessfulAcks   int        `json:"successful_acks"`
	StaleAcks        int        `json:"stale_acks"`
	ConflictAcks     int        `json:"conflict_acks"`
	ConsumerParallel int        `json:"consumer_parallelism"`
	CurrentVersion   int64      `json:"current_version"`
	RuntimeAppliedAt *time.Time `json:"runtime_applied_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
}

// GetRuleApplicationStatus exposes the broker and exact per-subtask runtime
// receipts for an outbox event. A database commit or broker publication alone
// is never returned as runtime applied.
func (s *RuleService) GetRuleApplicationStatus(
	ctx context.Context,
	ruleID string,
	eventID string,
	opCtx *OperationContext,
) (*RuleApplicationStatus, error) {
	ruleID = strings.TrimSpace(ruleID)
	eventID = strings.TrimSpace(eventID)
	if ruleID == "" || eventID == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "rule id and event id are required")
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "event id must be a UUID")
	}
	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, rule.TenantID); err != nil {
		return nil, err
	}
	if s.db == nil {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "rule application receipt database is not configured")
	}

	status := &RuleApplicationStatus{EventID: eventID, RuleID: ruleID, ExpectedAcks: s.config.AppliedAckExpectedParallelism}
	var appliedAt sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT
			o.published,
			o.runtime_status,
			o.runtime_applied_at,
			o.runtime_last_error,
			COALESCE((o.payload->>'version')::BIGINT, 0),
			COALESCE(o.payload->>'action', ''),
			COUNT(DISTINCT a.subtask_index),
			COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status IN ('applied','duplicate')),
			COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status = 'stale'),
			COUNT(DISTINCT a.subtask_index) FILTER (WHERE a.status = 'conflict'),
			COALESCE(MAX(a.parallelism), 0),
			COALESCE(MAX(a.current_version), 0)
		FROM rule_outbox o
		LEFT JOIN rule_update_applied_acks a ON a.event_id = o.payload->>'event_id'
		WHERE o.rule_id = $1 AND o.payload->>'event_id' = $2
		GROUP BY o.id, o.published, o.runtime_status, o.runtime_applied_at,
		         o.runtime_last_error, o.payload
		ORDER BY o.id DESC
		LIMIT 1
	`, ruleID, eventID).Scan(
		&status.BrokerPublished,
		&status.RuntimeStatus,
		&appliedAt,
		&status.LastError,
		&status.Version,
		&status.Action,
		&status.ReceivedAcks,
		&status.SuccessfulAcks,
		&status.StaleAcks,
		&status.ConflictAcks,
		&status.ConsumerParallel,
		&status.CurrentVersion,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeRuleNotFound, "rule application event not found: %s", eventID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get rule application status")
	}
	if appliedAt.Valid {
		value := appliedAt.Time
		status.RuntimeAppliedAt = &value
	}
	return status, nil
}

func buildRuleRollbackCandidate(
	current *model.Rule,
	target *model.RuleVersion,
	expectedVersion int64,
	operatorID string,
	now time.Time,
) (*model.Rule, error) {
	if current == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "current rule is required")
	}
	if target == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "rollback target is required")
	}
	if current.Version != expectedVersion {
		return nil, errors.Newf(
			errors.ErrCodeVersionConflict,
			"rule version changed: expected %d, current %d",
			expectedVersion,
			current.Version,
		)
	}
	if target.Version <= 0 || target.Version >= current.Version {
		return nil, errors.Newf(
			errors.ErrCodeInvalidStateTransition,
			"rollback target version %d must be older than current version %d",
			target.Version,
			current.Version,
		)
	}
	if target.RuleID != current.RuleID || target.TenantID != current.TenantID {
		return nil, errors.New(errors.ErrCodeInvalidStateTransition, "rollback target does not belong to the current rule and tenant")
	}

	restored, err := model.DecodeRuleVersionSnapshot(target)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidStateTransition, "rollback target integrity verification failed")
	}
	if restored.RuleID != target.RuleID || restored.TenantID != target.TenantID || restored.Version != target.Version {
		return nil, errors.New(errors.ErrCodeInvalidStateTransition, "rollback snapshot identity does not match its version record")
	}
	if err := validateRestorableRuleState(restored); err != nil {
		return nil, err
	}

	// Immutable ownership and provenance always come from the current row. Only
	// the versioned rule definition and its enabled state are restored.
	restored.RuleID = current.RuleID
	restored.TenantID = current.TenantID
	restored.CreatedAt = current.CreatedAt
	restored.CreatedBy = current.CreatedBy
	restored.Version = current.Version + 1
	restored.UpdatedAt = now
	restored.UpdatedBy = operatorID
	restored.ConditionsJSON = nil
	return restored, nil
}

func validateRestorableRuleState(rule *model.Rule) error {
	status := model.RuleStatus(rule.Status)
	switch status {
	case model.RuleStatusDraft, model.RuleStatusActive, model.RuleStatusDisabled:
	default:
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "rule status %s cannot be restored", rule.Status)
	}
	if rule.Enabled && status != model.RuleStatusActive {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "enabled rollback snapshot has non-active status %s", rule.Status)
	}
	if !rule.Enabled && status == model.RuleStatusActive {
		return errors.New(errors.ErrCodeInvalidStateTransition, "disabled rollback snapshot has active status")
	}
	if rule.RuleID == "" || rule.TenantID == "" || rule.Version <= 0 {
		return errors.New(errors.ErrCodeInvalidStateTransition, "rollback snapshot identity is incomplete")
	}
	if rule.UpdatedAt.IsZero() && rule.CreatedAt.IsZero() {
		return errors.New(errors.ErrCodeInvalidStateTransition, fmt.Sprintf("rollback snapshot %s has no provenance timestamp", rule.RuleID))
	}
	return nil
}

package whitelist

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	ActionCreate         = "whitelist-create"
	ActionSubmitApproval = "whitelist-submit-approval"
	ActionApprove        = "whitelist-approve"
	ActionReject         = "whitelist-reject"
	ActionExtend         = "whitelist-extend"
	ActionDisable        = "whitelist-disable"
	ActionAssign         = "whitelist-assign"
	ActionArchive        = "whitelist-archive"
	ActionExpire         = "whitelist-expire"
	RuleEffectPending    = "pending"
	RuleEffectApplied    = "applied"
	RuleEffectFailed     = "failed"
)

var (
	ErrCommandMetadata     = errors.New("whitelist command metadata invalid")
	ErrIdempotencyConflict = errors.New("whitelist idempotency conflict")
	ErrEntryArchived       = errors.New("whitelist entry archived")
)

// CommandMeta is supplied by the authenticated edge. It binds durable request
// identity, optimistic concurrency, actor, reason and trace to one transaction.
type CommandMeta struct {
	TenantID           string
	ActorID            string
	ActionID           string
	IdempotencyKey     string
	ExpectedVersion    int
	Reason             string
	TraceID            string
	SourceIP           string
	UserAgent          string
	ApprovalAuthorized bool
}

type CommandReceipt struct {
	EntryID          string `json:"entry_id"`
	Version          int    `json:"version"`
	EventID          string `json:"event_id"`
	ActionID         string `json:"action_id"`
	IdempotencyKey   string `json:"idempotency_key"`
	OutboxStatus     string `json:"outbox_status"`
	RuleEffectStatus string `json:"rule_effect_status,omitempty"`
	Replayed         bool   `json:"replayed"`
}

type CommandResult struct {
	Entry   *Entry
	Receipt CommandReceipt
}

// The following SQL primitives are intentionally private to the governed
// command boundary. Keeping the business writes beside the transaction,
// history, audit, outbox and idempotency records prevents a new caller from
// bypassing the whitelist lifecycle invariants.
func (r *Repository) createWithRunner(ctx context.Context, runner sqlRunner, entry *Entry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.Status == "" {
		entry.Status = "draft"
	}
	entry.Status = normalizeStatus(entry.Status, "draft")
	if entry.ApprovalStatus == "" {
		entry.ApprovalStatus = "draft"
	}
	entry.ApprovalStatus = normalizeApprovalStatus(entry.ApprovalStatus, "draft")
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = entry.CreatedAt
	}
	if entry.Version <= 0 {
		entry.Version = 1
	}
	entry.Type = normalizeType(entry.Type)
	entry.RiskLevel = normalizeRiskLevel(entry.RiskLevel)
	if entry.Status != "draft" || entry.ApprovalStatus != "draft" {
		return errors.New("new whitelist entries must start as draft/draft")
	}
	err := runner.QueryRowContext(ctx,
		`INSERT INTO whitelist (id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id, owner_role, scope, risk_level, covered_alerts, covered_assets, version, created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		 ON CONFLICT (tenant_id, type, value) DO NOTHING
		 RETURNING id, version, created_at, updated_at`,
		entry.ID, entry.TenantID, entry.Type, entry.Value, entry.Reason, entry.Description,
		entry.Status, entry.ApprovalStatus, entry.SourceAlertID, entry.FeedbackID, entry.OwnerRole,
		entry.Scope, entry.RiskLevel, entry.CoveredAlerts, entry.CoveredAssets, entry.Version,
		entry.CreatedBy, entry.ApprovedBy, entry.ApprovedAt, entry.DisabledAt, entry.ExpiresAt,
		entry.CreatedAt, entry.UpdatedAt).Scan(&entry.ID, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
	if err == sql.ErrNoRows {
		return ErrAlreadyExists
	}
	return err
}

func (r *Repository) updateWithRunner(ctx context.Context, runner sqlRunner, tenantID, id string, req UpdateRequest, actor string) (*Entry, error) {
	entry, err := r.getWithRunner(ctx, runner, tenantID, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if req.Reason != nil {
		entry.Reason = *req.Reason
	}
	if req.Description != nil {
		entry.Description = *req.Description
	}
	if req.OwnerRole != nil {
		entry.OwnerRole = *req.OwnerRole
	}
	if req.Scope != nil {
		entry.Scope = *req.Scope
	}
	if req.RiskLevel != nil {
		entry.RiskLevel = normalizeRiskLevel(*req.RiskLevel)
	}
	if req.ExpiresAt != nil {
		entry.ExpiresAt = req.ExpiresAt
	}
	if req.Status != nil {
		entry.Status = normalizeStatus(*req.Status, entry.Status)
		if entry.Status == "disabled" {
			entry.DisabledAt = &now
		}
	}
	if req.ApprovalStatus != nil {
		entry.ApprovalStatus = normalizeApprovalStatus(*req.ApprovalStatus, entry.ApprovalStatus)
	}
	if entry.Status == "active" && entry.ApprovalStatus == "" {
		entry.ApprovalStatus = "approved"
	}
	if entry.ApprovalStatus == "approved" && entry.ApprovedAt == nil {
		entry.ApprovedAt = &now
		entry.ApprovedBy = actor
	}
	if entry.Status != "disabled" {
		entry.DisabledAt = nil
	}

	expectedVersion := entry.Version
	if req.ExpectedVersion != nil {
		expectedVersion = *req.ExpectedVersion
	}
	err = runner.QueryRowContext(ctx,
		`UPDATE whitelist
		    SET reason=$3, description=$4, status=$5, approval_status=$6, owner_role=$7,
		        scope=$8, risk_level=$9, approved_by=$10, approved_at=$11, disabled_at=$12,
		        expires_at=$13, version=version+1, updated_at=now()
		  WHERE tenant_id=$1 AND id=$2 AND version=$14
		  RETURNING id, tenant_id, type, value, reason, description, status, approval_status, source_alert_id, feedback_id,
		            owner_role, scope, risk_level, covered_alerts, covered_assets, version,
		            created_by, approved_by, approved_at, disabled_at, expires_at, created_at, updated_at`,
		tenantID, id, entry.Reason, entry.Description, entry.Status, entry.ApprovalStatus, entry.OwnerRole,
		entry.Scope, entry.RiskLevel, entry.ApprovedBy, entry.ApprovedAt, entry.DisabledAt, entry.ExpiresAt, expectedVersion).Scan(
		&entry.ID, &entry.TenantID, &entry.Type, &entry.Value, &entry.Reason, &entry.Description, &entry.Status,
		&entry.ApprovalStatus, &entry.SourceAlertID, &entry.FeedbackID, &entry.OwnerRole, &entry.Scope, &entry.RiskLevel,
		&entry.CoveredAlerts, &entry.CoveredAssets, &entry.Version, &entry.CreatedBy,
		&entry.ApprovedBy, &entry.ApprovedAt, &entry.DisabledAt, &entry.ExpiresAt, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			if _, getErr := r.getWithRunner(ctx, runner, tenantID, id); getErr == nil {
				return nil, ErrVersionConflict
			}
		}
		return nil, err
	}
	return entry, nil
}

func (r *Repository) insertAuditWithRunner(ctx context.Context, runner sqlRunner, tenantID string, audit AuditRecord) error {
	detail := make(map[string]interface{}, len(audit.Detail)+1)
	for key, value := range audit.Detail {
		detail[key] = value
	}
	detail["result"] = "success"
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	userIDExpr := "NULLIF($3, '')"
	userID := audit.UserID
	if r.pgColumnType(ctx, "audit_logs", "user_id") == "uuid" {
		userIDExpr = "NULLIF($3, '')::uuid"
		if userID != "" {
			if _, err := uuid.Parse(userID); err != nil {
				userID = ""
			}
		}
	}
	args := []interface{}{tenantID, userID, audit.Action, "whitelist", audit.ObjectID, string(detailJSON), audit.IPAddress, audit.UserAgent}
	query := `INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, ` + strings.Replace(userIDExpr, "$3", "$2", 1) + `, $3, $4, $5, $6::jsonb, $7, $8)`
	if r.pgColumnExists(ctx, "audit_logs", "event_id") {
		eventID := audit.EventID
		if eventID == "" {
			eventID = "audit-" + uuid.NewString()
		}
		query = `INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, ` + userIDExpr + `, $4, $5, $6, $7::jsonb, $8, $9)`
		args = append([]interface{}{eventID}, args...)
	}
	_, err = runner.ExecContext(ctx, query, args...)
	return err
}

func (r *Repository) pgColumnExists(ctx context.Context, tableName, columnName string) bool {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2
	)`, tableName, columnName).Scan(&exists)
	return err == nil && exists
}

func (r *Repository) pgColumnType(ctx context.Context, tableName, columnName string) string {
	var dataType string
	err := r.db.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2
		ORDER BY CASE WHEN table_schema = 'public' THEN 0 ELSE 1 END LIMIT 1`, tableName, columnName).Scan(&dataType)
	if err != nil {
		return ""
	}
	return dataType
}

// CreateAtomic owns the transaction used by the whitelist HTTP command.
func (r *Repository) CreateAtomic(ctx context.Context, entry *Entry, meta CommandMeta, audit AuditRecord) (*CommandResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin whitelist create: %w", err)
	}
	defer tx.Rollback()
	result, err := r.CreateGovernedTx(ctx, tx, entry, meta, audit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit whitelist create: %w", err)
	}
	return result, nil
}

// CreateGovernedTx lets a containing aggregate (alert feedback) keep a single
// PostgreSQL commit while still satisfying whitelist history/outbox invariants.
func (r *Repository) CreateGovernedTx(ctx context.Context, tx *sql.Tx, entry *Entry, meta CommandMeta, audit AuditRecord) (*CommandResult, error) {
	if r == nil || r.db == nil || tx == nil || entry == nil {
		return nil, fmt.Errorf("%w: repository, transaction and entry are required", ErrCommandMetadata)
	}
	meta = normalizeCommandMeta(meta, entry.TenantID, ActionCreate)
	if err := validateCommandMeta(meta, ActionCreate); err != nil {
		return nil, err
	}
	if meta.ExpectedVersion != 0 {
		return nil, fmt.Errorf("%w: create expected version must be 0", ErrVersionConflict)
	}
	entry.TenantID = meta.TenantID
	entry.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("whitelist-entry\x00"+meta.TenantID+"\x00"+meta.IdempotencyKey)).String()
	requestHash, err := whitelistCommandHash(meta.ActionID, "create", createHashPayload(entry))
	if err != nil {
		return nil, err
	}
	if err := lockWhitelistCommandKey(ctx, tx, meta.TenantID, meta.IdempotencyKey); err != nil {
		return nil, err
	}
	if replay, found, err := loadWhitelistReplay(ctx, tx, meta.TenantID, meta.IdempotencyKey, requestHash); err != nil {
		return nil, err
	} else if found {
		replay.Receipt.Replayed = true
		*entry = *replay.Entry
		return replay, nil
	}
	if err := r.createWithRunner(ctx, tx, entry); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE whitelist SET last_action_id=$3,last_trace_id=$4 WHERE tenant_id=$1 AND id=$2`,
		entry.TenantID, entry.ID, meta.ActionID, meta.TraceID); err != nil {
		return nil, fmt.Errorf("set whitelist command identity: %w", err)
	}
	entry.LastActionID, entry.LastTraceID = meta.ActionID, meta.TraceID
	return r.persistWhitelistCommand(ctx, tx, entry, meta, audit, requestHash, "create", "", "traffic.whitelist.v2.EntryDrafted")
}

func (r *Repository) UpdateAtomic(ctx context.Context, entryID string, req UpdateRequest, meta CommandMeta, audit AuditRecord) (*CommandResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin whitelist update: %w", err)
	}
	defer tx.Rollback()
	result, err := r.UpdateGovernedTx(ctx, tx, entryID, req, meta, audit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit whitelist update: %w", err)
	}
	return result, nil
}

func (r *Repository) UpdateGovernedTx(ctx context.Context, tx *sql.Tx, entryID string, req UpdateRequest, meta CommandMeta, audit AuditRecord) (*CommandResult, error) {
	if r == nil || r.db == nil || tx == nil || strings.TrimSpace(entryID) == "" {
		return nil, fmt.Errorf("%w: repository, transaction and entry id are required", ErrCommandMetadata)
	}
	actionID, operation, eventType := commandIdentityForUpdate(req)
	if meta.ActionID == ActionExpire && actionID == ActionDisable {
		actionID, operation, eventType = ActionExpire, "expire", "traffic.whitelist.v2.EntryExpired"
	}
	meta = normalizeCommandMeta(meta, meta.TenantID, actionID)
	if err := validateCommandMeta(meta, actionID); err != nil {
		return nil, err
	}
	if meta.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: update expected version must be positive", ErrVersionConflict)
	}
	req.ExpectedVersion = intPtr(meta.ExpectedVersion)
	req.ExpectedRevision = intPtr(meta.ExpectedVersion)
	requestHash, err := whitelistCommandHash(meta.ActionID, operation, map[string]interface{}{
		"entry_id": entryID, "expected_version": meta.ExpectedVersion, "request": req,
	})
	if err != nil {
		return nil, err
	}
	if err := lockWhitelistCommandKey(ctx, tx, meta.TenantID, meta.IdempotencyKey); err != nil {
		return nil, err
	}
	if replay, found, err := loadWhitelistReplay(ctx, tx, meta.TenantID, meta.IdempotencyKey, requestHash); err != nil {
		return nil, err
	} else if found {
		replay.Receipt.Replayed = true
		return replay, nil
	}
	current, err := r.lockEntryForCommand(ctx, tx, meta.TenantID, entryID)
	if err != nil {
		return nil, err
	}
	if current.Version != meta.ExpectedVersion {
		return nil, fmt.Errorf("%w: expected=%d actual=%d", ErrVersionConflict, meta.ExpectedVersion, current.Version)
	}
	if code, message := validateWhitelistTransition(current, req, meta.ActorID, meta.ApprovalAuthorized); code != "" {
		return nil, &TransitionError{Code: code, Message: message}
	}
	entry, err := r.updateWithRunner(ctx, tx, meta.TenantID, entryID, req, meta.ActorID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE whitelist SET last_action_id=$3,last_trace_id=$4 WHERE tenant_id=$1 AND id=$2`,
		meta.TenantID, entryID, meta.ActionID, meta.TraceID); err != nil {
		return nil, fmt.Errorf("set whitelist command identity: %w", err)
	}
	entry.LastActionID, entry.LastTraceID = meta.ActionID, meta.TraceID
	return r.persistWhitelistCommand(ctx, tx, entry, meta, audit, requestHash, operation, current.Status, eventType)
}

// ArchiveAtomic replaces physical deletion. The row and every historical/event
// reference remain available for audit while normal reads hide it.
func (r *Repository) ArchiveAtomic(ctx context.Context, entryID string, meta CommandMeta, audit AuditRecord) (*CommandResult, error) {
	meta = normalizeCommandMeta(meta, meta.TenantID, ActionArchive)
	if err := validateCommandMeta(meta, ActionArchive); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	requestHash, err := whitelistCommandHash(meta.ActionID, "archive", map[string]interface{}{"entry_id": entryID, "expected_version": meta.ExpectedVersion})
	if err != nil {
		return nil, err
	}
	if err := lockWhitelistCommandKey(ctx, tx, meta.TenantID, meta.IdempotencyKey); err != nil {
		return nil, err
	}
	if replay, found, err := loadWhitelistReplay(ctx, tx, meta.TenantID, meta.IdempotencyKey, requestHash); err != nil {
		return nil, err
	} else if found {
		replay.Receipt.Replayed = true
		return replay, nil
	}
	current, err := r.lockEntryForCommand(ctx, tx, meta.TenantID, entryID)
	if err != nil {
		return nil, err
	}
	if current.Version != meta.ExpectedVersion {
		return nil, fmt.Errorf("%w: expected=%d actual=%d", ErrVersionConflict, meta.ExpectedVersion, current.Version)
	}
	if current.Status != "draft" && current.Status != "disabled" {
		return nil, &TransitionError{Code: "WHITELIST_DELETE_REQUIRES_DISABLED", Message: "pending or active whitelist entries must be disabled before archive"}
	}
	now := time.Now().UTC()
	entry := *current
	entry.Version++
	entry.Status = "disabled"
	if entry.ApprovalStatus == "draft" {
		entry.ApprovalStatus = "rejected"
	}
	entry.ArchivedAt, entry.DisabledAt, entry.UpdatedAt = &now, &now, now
	entry.LastActionID, entry.LastTraceID = meta.ActionID, meta.TraceID
	result, err := tx.ExecContext(ctx, `UPDATE whitelist SET status=$4,approval_status=$5,disabled_at=$6,
		archived_at=$6,version=$7,updated_at=$6,last_action_id=$8,last_trace_id=$9
		WHERE tenant_id=$1 AND id=$2 AND version=$3 AND archived_at IS NULL`,
		meta.TenantID, entryID, current.Version, entry.Status, entry.ApprovalStatus, now,
		entry.Version, meta.ActionID, meta.TraceID)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, ErrVersionConflict
	}
	command, err := r.persistWhitelistCommand(ctx, tx, &entry, meta, audit, requestHash, "archive", current.Status, "traffic.whitelist.v2.EntryArchived")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return command, nil
}

// ExpireDue transitions bounded, locked batches. An approved row becomes
// ineffective only after the revocation ACK is applied by the rule consumer.
func (r *Repository) ExpireDue(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `SELECT tenant_id,id::text,version FROM whitelist
		WHERE archived_at IS NULL AND status='active' AND approval_status='approved' AND expires_at<=now()
		ORDER BY expires_at,id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type dueEntry struct {
		tenantID, id string
		version      int
	}
	due := make([]dueEntry, 0, limit)
	for rows.Next() {
		var item dueEntry
		if err := rows.Scan(&item.tenantID, &item.id, &item.version); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	changed := 0
	for _, item := range due {
		disabled := "disabled"
		key := fmt.Sprintf("whitelist-expire-%s-v%d", item.id, item.version)
		_, err := r.UpdateAtomic(ctx, item.id, UpdateRequest{Status: &disabled}, CommandMeta{
			TenantID: item.tenantID, ActorID: "whitelist-expiry-sweeper", ActionID: ActionExpire,
			IdempotencyKey: key, ExpectedVersion: item.version, Reason: "approved expiry reached",
			TraceID: "whitelist-expiry:" + item.id + ":" + fmt.Sprint(item.version+1), ApprovalAuthorized: true,
		}, AuditRecord{UserID: "", Action: "WHITELIST_EXPIRED", ObjectID: item.id})
		if err != nil {
			if errors.Is(err, ErrVersionConflict) || errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return changed, err
		}
		changed++
	}
	return changed, nil
}

// RunExpirySweeper is tied to the service root context. Every pass is bounded;
// failures are observable and never converted into a successful expiry state.
func (r *Repository) RunExpirySweeper(ctx context.Context, interval time.Duration, limit int) {
	if interval < 10*time.Second {
		interval = time.Minute
	}
	run := func() {
		changed, err := r.ExpireDue(ctx, limit)
		if err != nil {
			if r.logger != nil && ctx.Err() == nil {
				r.logger.Error("Whitelist expiry sweep failed", zap.Error(err))
			}
			return
		}
		if changed > 0 && r.logger != nil {
			r.logger.Info("Whitelist expiry sweep committed", zap.Int("entries", changed))
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// AcknowledgeRuleEffect is the idempotent consumer-side projection receipt.
func (r *Repository) AcknowledgeRuleEffect(ctx context.Context, tenantID, entryID string, version int, status, ackEventID, ruleRevision, lastError string) error {
	if status != RuleEffectApplied && status != RuleEffectFailed {
		return fmt.Errorf("invalid whitelist rule effect status %q", status)
	}
	result, err := r.db.ExecContext(ctx, `UPDATE whitelist_rule_effects SET status=$4,ack_event_id=$5,
		rule_revision=$6,last_error=$7,acknowledged_at=now()
		WHERE tenant_id=$1 AND entry_id=$2 AND entry_version=$3 AND status='pending'`,
		tenantID, entryID, version, status, ackEventID, ruleRevision, lastError)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var existingStatus, existingAck, existingRevision, existingError string
	err = r.db.QueryRowContext(ctx, `SELECT status,ack_event_id,rule_revision,last_error FROM whitelist_rule_effects
		WHERE tenant_id=$1 AND entry_id=$2 AND entry_version=$3`, tenantID, entryID, version).
		Scan(&existingStatus, &existingAck, &existingRevision, &existingError)
	if err != nil {
		return err
	}
	if existingStatus == status && existingAck == ackEventID && existingRevision == ruleRevision && existingError == lastError {
		return nil
	}
	return ErrIdempotencyConflict
}

type TransitionError struct{ Code, Message string }

func (e *TransitionError) Error() string { return e.Code + ": " + e.Message }

func (r *Repository) persistWhitelistCommand(ctx context.Context, tx *sql.Tx, entry *Entry, meta CommandMeta, audit AuditRecord, requestHash, operation, previousStatus, eventType string) (*CommandResult, error) {
	eventID := deterministicWhitelistEventID(entry.TenantID, meta.IdempotencyKey)
	desired := desiredRuleState(eventType)
	if desired == "" && entry.Status == "active" && entry.ApprovalStatus == "approved" {
		desired = "effective"
	}
	snapshot, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO whitelist_entry_versions
		(event_id,tenant_id,entry_id,version,action_id,operation,actor_id,reason,trace_id,
		 previous_status,resulting_status,snapshot,occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,now())`,
		eventID, entry.TenantID, entry.ID, entry.Version, meta.ActionID, operation, meta.ActorID,
		meta.Reason, meta.TraceID, previousStatus, entry.Status, string(snapshot)); err != nil {
		return nil, fmt.Errorf("persist whitelist version: %w", err)
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID.String(), "event_type": eventType, "schema_version": 2,
		"tenant_id": entry.TenantID, "entry_id": entry.ID, "aggregate_version": entry.Version,
		"action_id": meta.ActionID, "reason": meta.Reason, "trace_id": meta.TraceID,
		"desired_rule_state": desired, "occurred_at": time.Now().UTC(), "entry": entry,
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO whitelist_event_outbox
		(event_id,tenant_id,entry_id,aggregate_version,event_type,partition_key,payload,trace_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`, eventID, entry.TenantID, entry.ID,
		entry.Version, eventType, entry.TenantID+":"+entry.ID, string(payload), meta.TraceID); err != nil {
		return nil, fmt.Errorf("persist whitelist outbox: %w", err)
	}
	ruleStatus := ""
	if desired != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO whitelist_rule_effects
			(tenant_id,entry_id,entry_version,event_id,desired_state,status)
			VALUES ($1,$2,$3,$4,$5,'pending')`, entry.TenantID, entry.ID, entry.Version, eventID, desired); err != nil {
			return nil, fmt.Errorf("persist whitelist rule effect: %w", err)
		}
		ruleStatus = RuleEffectPending
		entry.RuleEffectStatus = ruleStatus
	}
	audit.EventID = "audit-" + eventID.String()
	audit.Action = meta.ActionID
	audit.ObjectID = entry.ID
	audit.TraceID = meta.TraceID
	if audit.Detail == nil {
		audit.Detail = map[string]interface{}{}
	}
	audit.Detail["event_id"] = eventID.String()
	audit.Detail["version"] = entry.Version
	audit.Detail["operation"] = operation
	audit.Detail["reason"] = meta.Reason
	audit.Detail["outbox_status"] = "pending"
	if err = r.insertAuditWithRunner(ctx, tx, entry.TenantID, audit); err != nil {
		return nil, fmt.Errorf("persist whitelist audit: %w", err)
	}
	receipt := CommandReceipt{EntryID: entry.ID, Version: entry.Version, EventID: eventID.String(),
		ActionID: meta.ActionID, IdempotencyKey: meta.IdempotencyKey, OutboxStatus: "pending", RuleEffectStatus: ruleStatus}
	responseJSON, _ := json.Marshal(entry)
	if _, err = tx.ExecContext(ctx, `INSERT INTO whitelist_command_requests
		(tenant_id,idempotency_key,request_sha256,action_id,operation,entry_id,expected_version,
		 resulting_version,event_id,reason,trace_id,response_payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)`, entry.TenantID,
		meta.IdempotencyKey, requestHash, meta.ActionID, operation, entry.ID, meta.ExpectedVersion,
		entry.Version, eventID, meta.Reason, meta.TraceID, string(responseJSON)); err != nil {
		return nil, fmt.Errorf("persist whitelist request: %w", err)
	}
	return &CommandResult{Entry: entry, Receipt: receipt}, nil
}

func loadWhitelistReplay(ctx context.Context, tx *sql.Tx, tenantID, key, requestHash string) (*CommandResult, bool, error) {
	var priorHash, actionID, entryID, eventID, response string
	var resultingVersion int
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,action_id,entry_id::text,resulting_version,
		event_id::text,response_payload::text FROM whitelist_command_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, tenantID, key).
		Scan(&priorHash, &actionID, &entryID, &resultingVersion, &eventID, &response)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if priorHash != requestHash {
		return nil, false, ErrIdempotencyConflict
	}
	var entry Entry
	if err := json.Unmarshal([]byte(response), &entry); err != nil {
		return nil, false, err
	}
	return &CommandResult{Entry: &entry, Receipt: CommandReceipt{
		EntryID: entryID, Version: resultingVersion, EventID: eventID, ActionID: actionID,
		IdempotencyKey: key, OutboxStatus: "pending",
	}}, true, nil
}

func (r *Repository) lockEntryForCommand(ctx context.Context, tx *sql.Tx, tenantID, entryID string) (*Entry, error) {
	entry, err := scanWhitelistEntry(tx.QueryRowContext(ctx, whitelistEntrySelect+`
		WHERE w.tenant_id=$1 AND w.id=$2 FOR UPDATE OF w`, tenantID, entryID))
	if err == sql.ErrNoRows {
		var archived bool
		if checkErr := tx.QueryRowContext(ctx, `SELECT archived_at IS NOT NULL FROM whitelist WHERE tenant_id=$1 AND id=$2`, tenantID, entryID).Scan(&archived); checkErr == nil && archived {
			return nil, ErrEntryArchived
		}
	}
	return entry, err
}

func lockWhitelistCommandKey(ctx context.Context, tx *sql.Tx, tenantID, key string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+"::"+key)
	return err
}

func normalizeCommandMeta(meta CommandMeta, tenantID, actionID string) CommandMeta {
	meta.TenantID = strings.TrimSpace(meta.TenantID)
	if meta.TenantID == "" {
		meta.TenantID = strings.TrimSpace(tenantID)
	}
	meta.ActionID = strings.TrimSpace(meta.ActionID)
	if meta.ActionID == "" {
		meta.ActionID = actionID
	}
	meta.IdempotencyKey = strings.TrimSpace(meta.IdempotencyKey)
	meta.Reason = strings.TrimSpace(meta.Reason)
	meta.TraceID = strings.TrimSpace(meta.TraceID)
	return meta
}

func validateCommandMeta(meta CommandMeta, expectedAction string) error {
	if meta.TenantID == "" || meta.ActorID == "" || meta.ActionID != expectedAction || meta.Reason == "" || meta.TraceID == "" {
		return fmt.Errorf("%w: tenant, actor, canonical action, reason and trace are required", ErrCommandMetadata)
	}
	if len(meta.IdempotencyKey) < 16 || len(meta.IdempotencyKey) > 200 {
		return fmt.Errorf("%w: Idempotency-Key must contain 16 to 200 characters", ErrCommandMetadata)
	}
	return nil
}

func commandIdentityForUpdate(req UpdateRequest) (string, string, string) {
	if req.Status != nil && strings.EqualFold(strings.TrimSpace(*req.Status), "disabled") {
		if req.ApprovalStatus != nil && strings.EqualFold(strings.TrimSpace(*req.ApprovalStatus), "rejected") {
			return ActionReject, "reject", "traffic.whitelist.v2.EntryRejected"
		}
		return ActionDisable, "disable", "traffic.whitelist.v2.EntryRevoked"
	}
	if req.ApprovalStatus != nil {
		switch strings.ToLower(strings.TrimSpace(*req.ApprovalStatus)) {
		case "pending":
			return ActionSubmitApproval, "submit", "traffic.whitelist.v2.ApprovalSubmitted"
		case "approved":
			return ActionApprove, "approve", "traffic.whitelist.v2.EntryApproved"
		}
	}
	if req.ExpiresAt != nil {
		return ActionExtend, "extend", "traffic.whitelist.v2.EntryExtended"
	}
	if req.OwnerRole != nil {
		return ActionAssign, "assign", "traffic.whitelist.v2.EntryAssigned"
	}
	return "", "update", "traffic.whitelist.v2.EntryUpdated"
}

func desiredRuleState(eventType string) string {
	if eventType == "traffic.whitelist.v2.EntryApproved" {
		return "effective"
	}
	switch eventType {
	case "traffic.whitelist.v2.EntryRevoked", "traffic.whitelist.v2.EntryRejected", "traffic.whitelist.v2.EntryArchived", "traffic.whitelist.v2.EntryExpired":
		return "revoked"
	default:
		return ""
	}
}

func createHashPayload(entry *Entry) map[string]interface{} {
	return map[string]interface{}{
		"tenant_id": entry.TenantID, "type": entry.Type, "value": entry.Value,
		"reason": entry.Reason, "description": entry.Description, "source_alert_id": entry.SourceAlertID,
		"feedback_id": entry.FeedbackID, "owner_role": entry.OwnerRole, "scope": entry.Scope,
		"risk_level": entry.RiskLevel, "covered_alerts": entry.CoveredAlerts,
		"covered_assets": entry.CoveredAssets, "expires_at": entry.ExpiresAt,
	}
}

func whitelistCommandHash(action, operation string, payload interface{}) (string, error) {
	encoded, err := json.Marshal(map[string]interface{}{"action_id": action, "operation": operation, "payload": payload})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func deterministicWhitelistEventID(tenantID, key string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("whitelist-event\x00"+tenantID+"\x00"+key))
}

func intPtr(value int) *int { return &value }

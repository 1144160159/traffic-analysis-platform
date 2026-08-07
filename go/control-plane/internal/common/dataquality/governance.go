package dataquality

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
)

var (
	ErrGovernanceUnavailable = errors.New("PostgreSQL data quality governance is not available")
	ErrGovernanceConflict    = errors.New("data quality resource revision conflict")
	ErrGovernanceNotFound    = errors.New("data quality resource not found")
	ErrIdempotencyConflict   = errors.New("data quality idempotency key collision")
	ErrInvalidTransition     = errors.New("invalid data quality rule transition")
	ErrSelfApproval          = errors.New("data quality rule requester cannot approve or reject the same rule")
)

type DatasetCommand struct {
	TenantID              string   `json:"tenant_id"`
	DatasetID             string   `json:"dataset_id"`
	DisplayName           string   `json:"display_name"`
	Owner                 string   `json:"owner"`
	SchemaVersion         int64    `json:"schema_version"`
	SignalContractVersion string   `json:"signal_contract_version"`
	BusinessKeys          []string `json:"business_keys"`
	AllowedLateness       int64    `json:"allowed_lateness_seconds"`
	RetentionSeconds      int64    `json:"retention_seconds"`
	Upstreams             []string `json:"upstreams"`
	Downstreams           []string `json:"downstreams"`
	SLOTarget             float64  `json:"slo_target"`
	Status                string   `json:"status"`
	ExpectedRevision      int64    `json:"expected_revision"`
	ActionID              string   `json:"action_id"`
	IdempotencyKey        string   `json:"-"`
	Reason                string   `json:"reason"`
	Actor                 string   `json:"actor"`
	TraceID               string   `json:"-"`
}

type DatasetRecord struct {
	TenantID              string          `json:"tenant_id"`
	DatasetID             string          `json:"dataset_id"`
	DisplayName           string          `json:"display_name"`
	Owner                 string          `json:"owner"`
	SchemaVersion         int64           `json:"schema_version"`
	SignalContractVersion string          `json:"signal_contract_version"`
	BusinessKeys          json.RawMessage `json:"business_keys"`
	AllowedLateness       int64           `json:"allowed_lateness_seconds"`
	RetentionSeconds      int64           `json:"retention_seconds"`
	Upstreams             json.RawMessage `json:"upstreams"`
	Downstreams           json.RawMessage `json:"downstreams"`
	SLOTarget             float64         `json:"slo_target"`
	Status                string          `json:"status"`
	Revision              int64           `json:"revision"`
	TraceID               string          `json:"trace_id"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	Replayed              bool            `json:"replayed"`
}

type RuleCreateCommand struct {
	TenantID         string                 `json:"tenant_id"`
	DatasetID        string                 `json:"dataset_id"`
	RuleKey          string                 `json:"rule_key"`
	Dimension        string                 `json:"dimension"`
	FieldPath        string                 `json:"field_path"`
	Predicate        map[string]interface{} `json:"predicate"`
	Threshold        map[string]interface{} `json:"threshold"`
	WindowSeconds    int64                  `json:"window_seconds"`
	Sampling         map[string]interface{} `json:"sampling"`
	Severity         string                 `json:"severity"`
	Owner            string                 `json:"owner"`
	ExemptionPolicy  map[string]interface{} `json:"exemption_policy"`
	RepairAction     string                 `json:"repair_action"`
	GatePolicy       string                 `json:"gate_policy"`
	ExpectedRevision int64                  `json:"expected_revision"`
	ActionID         string                 `json:"action_id"`
	IdempotencyKey   string                 `json:"-"`
	Reason           string                 `json:"reason"`
	Actor            string                 `json:"actor"`
	TraceID          string                 `json:"-"`
}

type RuleTransitionCommand struct {
	TenantID         string `json:"tenant_id"`
	RuleID           string `json:"rule_id"`
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
	ActionID         string `json:"action_id"`
	IdempotencyKey   string `json:"-"`
	Reason           string `json:"reason"`
	Actor            string `json:"actor"`
	TraceID          string `json:"-"`
}

type RuleRecord struct {
	TenantID    string                 `json:"tenant_id"`
	RuleID      string                 `json:"rule_id"`
	RuleVersion int64                  `json:"rule_version"`
	DatasetID   string                 `json:"dataset_id"`
	Status      string                 `json:"status"`
	Revision    int64                  `json:"revision"`
	CreatedBy   string                 `json:"created_by"`
	ApprovedBy  string                 `json:"approved_by"`
	TraceID     string                 `json:"trace_id"`
	Snapshot    map[string]interface{} `json:"snapshot"`
	Replayed    bool                   `json:"replayed"`
}

func (m *Monitor) UpsertDataset(ctx context.Context, command DatasetCommand) (*DatasetRecord, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	if err := validateDatasetCommand(command); err != nil {
		return nil, err
	}
	requestSHA, err := commandSHA(command)
	if err != nil {
		return nil, err
	}
	tx, err := m.controlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin dataset command: %w", err)
	}
	defer tx.Rollback()
	if err := governanceCommandLock(ctx, tx, command.TenantID, command.IdempotencyKey); err != nil {
		return nil, err
	}
	var replay DatasetRecord
	found, err := loadGovernanceReceipt(ctx, tx, command.TenantID, command.IdempotencyKey, requestSHA, &replay)
	if err != nil || found {
		if found {
			replay.Replayed = true
			return &replay, nil
		}
		return nil, err
	}

	current, exists, err := loadDatasetForUpdate(ctx, tx, command.TenantID, command.DatasetID)
	if err != nil {
		return nil, err
	}
	operation := "created"
	revision := int64(1)
	if exists {
		if command.ExpectedRevision != current.Revision {
			return nil, ErrGovernanceConflict
		}
		revision = current.Revision + 1
		operation = "updated"
		if command.Status == "retired" {
			operation = "retired"
		}
	} else if command.ExpectedRevision != 0 {
		return nil, ErrGovernanceConflict
	}
	keys, _ := json.Marshal(command.BusinessKeys)
	upstreams, _ := json.Marshal(command.Upstreams)
	downstreams, _ := json.Marshal(command.Downstreams)
	row := DatasetRecord{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO data_quality_datasets (
			tenant_id,dataset_id,display_name,owner,schema_version,signal_contract_version,
			business_keys,allowed_lateness_seconds,retention_seconds,upstreams,downstreams,
			slo_target,status,revision,trace_id,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10::jsonb,$11::jsonb,$12,$13,$14,$15,now(),now())
		ON CONFLICT (tenant_id,dataset_id) DO UPDATE SET
			display_name=EXCLUDED.display_name,owner=EXCLUDED.owner,schema_version=EXCLUDED.schema_version,
			signal_contract_version=EXCLUDED.signal_contract_version,business_keys=EXCLUDED.business_keys,
			allowed_lateness_seconds=EXCLUDED.allowed_lateness_seconds,retention_seconds=EXCLUDED.retention_seconds,
			upstreams=EXCLUDED.upstreams,downstreams=EXCLUDED.downstreams,slo_target=EXCLUDED.slo_target,
			status=EXCLUDED.status,revision=EXCLUDED.revision,trace_id=EXCLUDED.trace_id,updated_at=now()
		RETURNING tenant_id,dataset_id,display_name,owner,schema_version,signal_contract_version,
			business_keys,allowed_lateness_seconds,retention_seconds,upstreams,downstreams,
			slo_target,status,revision,trace_id,created_at,updated_at
	`, command.TenantID, command.DatasetID, command.DisplayName, command.Owner, command.SchemaVersion,
		command.SignalContractVersion, string(keys), command.AllowedLateness, command.RetentionSeconds,
		string(upstreams), string(downstreams), command.SLOTarget, command.Status, revision, command.TraceID).Scan(
		&row.TenantID, &row.DatasetID, &row.DisplayName, &row.Owner, &row.SchemaVersion,
		&row.SignalContractVersion, &row.BusinessKeys, &row.AllowedLateness, &row.RetentionSeconds,
		&row.Upstreams, &row.Downstreams, &row.SLOTarget, &row.Status, &row.Revision, &row.TraceID,
		&row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("persist data quality dataset: %w", err)
	}
	eventID := governanceEventID(command.TenantID, command.IdempotencyKey)
	if err := persistDatasetGovernance(ctx, tx, command, row, operation, eventID, requestSHA); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit dataset command: %w", err)
	}
	return &row, nil
}

func (m *Monitor) CreateRule(ctx context.Context, command RuleCreateCommand) (*RuleRecord, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	if err := validateRuleCreateCommand(command); err != nil {
		return nil, err
	}
	requestSHA, err := commandSHA(command)
	if err != nil {
		return nil, err
	}
	tx, err := m.controlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin rule command: %w", err)
	}
	defer tx.Rollback()
	if err := governanceCommandLock(ctx, tx, command.TenantID, command.IdempotencyKey); err != nil {
		return nil, err
	}
	var replay RuleRecord
	found, err := loadGovernanceReceipt(ctx, tx, command.TenantID, command.IdempotencyKey, requestSHA, &replay)
	if err != nil || found {
		if found {
			replay.Replayed = true
			return &replay, nil
		}
		return nil, err
	}
	if command.ExpectedRevision != 0 {
		return nil, ErrGovernanceConflict
	}
	ruleID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("data-quality-rule:"+command.TenantID+":"+command.IdempotencyKey))
	predicate, _ := json.Marshal(command.Predicate)
	threshold, _ := json.Marshal(command.Threshold)
	sampling, _ := json.Marshal(command.Sampling)
	exemption, _ := json.Marshal(command.ExemptionPolicy)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO data_quality_rules (
			tenant_id,rule_id,rule_key,dataset_id,rule_version,dimension,field_path,predicate,
			threshold,window_seconds,sampling,severity,owner,exemption_policy,repair_action,
			gate_policy,status,revision,created_by,approved_by,reason,trace_id,created_at,updated_at
		) VALUES ($1,$2,$3,$4,1,$5,$6,$7::jsonb,$8::jsonb,$9,$10::jsonb,$11,$12,$13::jsonb,
			$14,$15,'draft',1,$16,'',$17,$18,now(),now())
	`, command.TenantID, ruleID, command.RuleKey, command.DatasetID, command.Dimension,
		command.FieldPath, string(predicate), string(threshold), command.WindowSeconds, string(sampling),
		command.Severity, command.Owner, string(exemption), command.RepairAction, command.GatePolicy,
		command.Actor, command.Reason, command.TraceID)
	if err != nil {
		return nil, fmt.Errorf("persist data quality rule draft: %w", err)
	}
	snapshot := ruleCreateSnapshot(command, ruleID.String())
	record := RuleRecord{TenantID: command.TenantID, RuleID: ruleID.String(), RuleVersion: 1,
		DatasetID: command.DatasetID, Status: "draft", Revision: 1, CreatedBy: command.Actor,
		TraceID: command.TraceID, Snapshot: snapshot}
	eventID := governanceEventID(command.TenantID, command.IdempotencyKey)
	if err := persistRuleGovernance(ctx, tx, command.TenantID, command.ActionID, command.IdempotencyKey,
		requestSHA, command.Actor, command.Reason, command.TraceID, "created", "", record, eventID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rule command: %w", err)
	}
	return &record, nil
}

func (m *Monitor) TransitionRule(ctx context.Context, command RuleTransitionCommand) (*RuleRecord, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	if err := validateRuleTransitionCommand(command); err != nil {
		return nil, err
	}
	ruleID, err := uuid.Parse(command.RuleID)
	if err != nil {
		return nil, fmt.Errorf("rule_id must be a UUID")
	}
	requestSHA, err := commandSHA(command)
	if err != nil {
		return nil, err
	}
	tx, err := m.controlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin rule transition: %w", err)
	}
	defer tx.Rollback()
	if err := governanceCommandLock(ctx, tx, command.TenantID, command.IdempotencyKey); err != nil {
		return nil, err
	}
	var replay RuleRecord
	found, err := loadGovernanceReceipt(ctx, tx, command.TenantID, command.IdempotencyKey, requestSHA, &replay)
	if err != nil || found {
		if found {
			replay.Replayed = true
			return &replay, nil
		}
		return nil, err
	}
	var raw []byte
	var current RuleRecord
	err = tx.QueryRowContext(ctx, `
		SELECT to_jsonb(r),r.rule_version,r.dataset_id,r.status,r.revision,r.created_by,r.approved_by,r.trace_id
		FROM data_quality_rules r WHERE tenant_id=$1 AND rule_id=$2
		ORDER BY rule_version DESC LIMIT 1 FOR UPDATE
	`, command.TenantID, ruleID).Scan(&raw, &current.RuleVersion, &current.DatasetID, &current.Status,
		&current.Revision, &current.CreatedBy, &current.ApprovedBy, &current.TraceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGovernanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock data quality rule: %w", err)
	}
	current.TenantID, current.RuleID = command.TenantID, ruleID.String()
	_ = json.Unmarshal(raw, &current.Snapshot)
	if command.ExpectedRevision != current.Revision {
		return nil, ErrGovernanceConflict
	}
	next, operation, err := nextRuleStatus(current.Status, command.Action)
	if err != nil {
		return nil, err
	}
	if (command.Action == "approve" || command.Action == "reject") && command.Actor == current.CreatedBy {
		return nil, ErrSelfApproval
	}
	approvedBy := current.ApprovedBy
	if command.Action == "approve" || command.Action == "reject" {
		approvedBy = command.Actor
	}
	newRevision := current.Revision + 1
	var updatedRaw []byte
	err = tx.QueryRowContext(ctx, `
		UPDATE data_quality_rules SET status=$4,revision=$5,approved_by=$6,reason=$7,trace_id=$8,updated_at=now()
		WHERE tenant_id=$1 AND rule_id=$2 AND rule_version=$3
		RETURNING to_jsonb(data_quality_rules)
	`, command.TenantID, ruleID, current.RuleVersion, next, newRevision, approvedBy,
		command.Reason, command.TraceID).Scan(&updatedRaw)
	if err != nil {
		return nil, fmt.Errorf("transition data quality rule: %w", err)
	}
	updated := current
	updated.Status, updated.Revision, updated.ApprovedBy, updated.TraceID = next, newRevision, approvedBy, command.TraceID
	updated.Snapshot = map[string]interface{}{}
	_ = json.Unmarshal(updatedRaw, &updated.Snapshot)
	eventID := governanceEventID(command.TenantID, command.IdempotencyKey)
	if err := persistRuleGovernance(ctx, tx, command.TenantID, command.ActionID, command.IdempotencyKey,
		requestSHA, command.Actor, command.Reason, command.TraceID, operation, current.Status, updated, eventID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rule transition: %w", err)
	}
	return &updated, nil
}

func (m *Monitor) ListDatasets(ctx context.Context, tenantID string) ([]DatasetRecord, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	rows, err := m.controlDB.QueryContext(ctx, `
		SELECT tenant_id,dataset_id,display_name,owner,schema_version,signal_contract_version,
			business_keys,allowed_lateness_seconds,retention_seconds,upstreams,downstreams,
			slo_target,status,revision,trace_id,created_at,updated_at
		FROM data_quality_datasets WHERE tenant_id=$1 ORDER BY dataset_id
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list data quality datasets: %w", err)
	}
	defer rows.Close()
	result := []DatasetRecord{}
	for rows.Next() {
		var row DatasetRecord
		if err := rows.Scan(&row.TenantID, &row.DatasetID, &row.DisplayName, &row.Owner, &row.SchemaVersion,
			&row.SignalContractVersion, &row.BusinessKeys, &row.AllowedLateness, &row.RetentionSeconds,
			&row.Upstreams, &row.Downstreams, &row.SLOTarget, &row.Status, &row.Revision, &row.TraceID,
			&row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (m *Monitor) ListRules(ctx context.Context, tenantID, datasetID string) ([]map[string]interface{}, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	rows, err := m.controlDB.QueryContext(ctx, `
		SELECT to_jsonb(r) FROM data_quality_rules r
		WHERE tenant_id=$1 AND ($2='' OR dataset_id=$2)
		ORDER BY dataset_id,rule_key,rule_version DESC
	`, tenantID, datasetID)
	if err != nil {
		return nil, fmt.Errorf("list data quality rules: %w", err)
	}
	defer rows.Close()
	result := []map[string]interface{}{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		item := map[string]interface{}{}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func validateDatasetCommand(command DatasetCommand) error {
	command.Status = strings.TrimSpace(command.Status)
	if command.TenantID == "" || command.DatasetID == "" || command.DisplayName == "" || command.Owner == "" ||
		command.SignalContractVersion == "" || command.ActionID == "" || command.Actor == "" || command.TraceID == "" {
		return fmt.Errorf("dataset identity, owner, action, actor and trace are required")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 || len([]rune(command.Reason)) < 8 || len([]rune(command.Reason)) > 1000 {
		return fmt.Errorf("idempotency key must be 16-200 characters and reason 8-1000 characters")
	}
	if command.SchemaVersion <= 0 || command.AllowedLateness < 0 || command.RetentionSeconds <= 0 || command.SLOTarget <= 0 || command.SLOTarget > 1 {
		return fmt.Errorf("dataset versions, retention and SLO are invalid")
	}
	if command.Status != "active" && command.Status != "paused" && command.Status != "retired" {
		return fmt.Errorf("dataset status must be active, paused or retired")
	}
	return nil
}

func validateRuleCreateCommand(command RuleCreateCommand) error {
	if command.TenantID == "" || command.DatasetID == "" || command.RuleKey == "" || command.Owner == "" ||
		command.ActionID == "" || command.Actor == "" || command.TraceID == "" {
		return fmt.Errorf("rule identity, owner, action, actor and trace are required")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 || len([]rune(command.Reason)) < 8 || len([]rune(command.Reason)) > 1000 {
		return fmt.Errorf("idempotency key must be 16-200 characters and reason 8-1000 characters")
	}
	dimensions := map[string]bool{"completeness": true, "uniqueness": true, "timeliness": true, "validity": true, "referential_integrity": true, "ordering": true, "duplicate": true, "lateness": true, "tenant_ownership": true, "object_availability": true}
	severities := map[string]bool{"info": true, "warning": true, "high": true, "critical": true}
	gates := map[string]bool{"observe": true, "degrade": true, "quarantine": true, "release_block": true}
	if !dimensions[command.Dimension] || !severities[command.Severity] || !gates[command.GatePolicy] {
		return fmt.Errorf("rule dimension, severity, gate policy or window is invalid")
	}
	if command.Predicate == nil || command.Threshold == nil || command.Sampling == nil || command.ExemptionPolicy == nil {
		return fmt.Errorf("rule JSON policies are required")
	}
	if command.DatasetID != "flows_raw" {
		return fmt.Errorf("rule dataset %q is not supported by the bounded evaluator", command.DatasetID)
	}
	predicate, err := json.Marshal(command.Predicate)
	if err != nil {
		return fmt.Errorf("rule predicate is invalid: %w", err)
	}
	if _, err := safeFlowPredicate(command.FieldPath, predicate); err != nil {
		return err
	}
	threshold, err := json.Marshal(command.Threshold)
	if err != nil {
		return fmt.Errorf("rule threshold is invalid: %w", err)
	}
	if _, err := minimumThreshold(threshold); err != nil {
		return fmt.Errorf("rule threshold is invalid: %w", err)
	}
	if _, err := checkedRuleWindow(command.WindowSeconds); err != nil {
		return err
	}
	return nil
}

func validateRuleTransitionCommand(command RuleTransitionCommand) error {
	if command.TenantID == "" || command.RuleID == "" || command.ActionID == "" || command.Actor == "" || command.TraceID == "" || command.ExpectedRevision <= 0 {
		return fmt.Errorf("rule transition identity, revision, actor and trace are required")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 || len([]rune(command.Reason)) < 8 || len([]rune(command.Reason)) > 1000 {
		return fmt.Errorf("idempotency key must be 16-200 characters and reason 8-1000 characters")
	}
	return nil
}

func nextRuleStatus(current, action string) (string, string, error) {
	transitions := map[string]map[string][2]string{
		"draft":            {"start_shadow": {"shadow", "shadow_started"}, "retire": {"retired", "retired"}},
		"shadow":           {"submit_approval": {"approval_pending", "approval_submitted"}, "retire": {"retired", "retired"}},
		"approval_pending": {"approve": {"active", "approved"}, "reject": {"rejected", "rejected"}},
		"active":           {"retire": {"retired", "retired"}},
	}
	transition, ok := transitions[current][action]
	if !ok {
		return "", "", fmt.Errorf("%w: %s cannot %s", ErrInvalidTransition, current, action)
	}
	return transition[0], transition[1], nil
}

func commandSHA(command interface{}) (string, error) {
	payload, err := json.Marshal(command)
	if err != nil {
		return "", fmt.Errorf("marshal governance command: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func governanceEventID(tenantID, idempotencyKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("data-quality-governance:"+tenantID+":"+idempotencyKey))
}

func governanceCommandLock(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		return fmt.Errorf("lock governance command: %w", err)
	}
	return nil
}

func loadGovernanceReceipt(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey, requestSHA string, target interface{}) (bool, error) {
	var existingSHA string
	var response []byte
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload FROM data_quality_command_requests WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&existingSHA, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load governance command receipt: %w", err)
	}
	if existingSHA != requestSHA {
		return false, ErrIdempotencyConflict
	}
	if err := json.Unmarshal(response, target); err != nil {
		return false, fmt.Errorf("decode governance command receipt: %w", err)
	}
	return true, nil
}

func loadDatasetForUpdate(ctx context.Context, tx *sql.Tx, tenantID, datasetID string) (DatasetRecord, bool, error) {
	var row DatasetRecord
	err := tx.QueryRowContext(ctx, `
		SELECT tenant_id,dataset_id,display_name,owner,schema_version,signal_contract_version,
			business_keys,allowed_lateness_seconds,retention_seconds,upstreams,downstreams,
			slo_target,status,revision,trace_id,created_at,updated_at
		FROM data_quality_datasets WHERE tenant_id=$1 AND dataset_id=$2 FOR UPDATE
	`, tenantID, datasetID).Scan(&row.TenantID, &row.DatasetID, &row.DisplayName, &row.Owner, &row.SchemaVersion,
		&row.SignalContractVersion, &row.BusinessKeys, &row.AllowedLateness, &row.RetentionSeconds,
		&row.Upstreams, &row.Downstreams, &row.SLOTarget, &row.Status, &row.Revision, &row.TraceID,
		&row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return row, false, nil
	}
	return row, err == nil, err
}

func persistDatasetGovernance(ctx context.Context, tx *sql.Tx, command DatasetCommand, row DatasetRecord, operation string, eventID uuid.UUID, requestSHA string) error {
	snapshot, _ := json.Marshal(row)
	if _, err := tx.ExecContext(ctx, `INSERT INTO data_quality_dataset_history (event_id,tenant_id,dataset_id,revision,operation,actor_id,reason,trace_id,snapshot) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, eventID, row.TenantID, row.DatasetID, row.Revision, operation, command.Actor, command.Reason, command.TraceID, string(snapshot)); err != nil {
		return fmt.Errorf("insert dataset history: %w", err)
	}
	if err := insertGovernanceOutbox(ctx, tx, eventID, row.TenantID, "dataset", row.DatasetID, row.Revision, "DATA_QUALITY_DATASET_"+strings.ToUpper(operation), command.TraceID, snapshot); err != nil {
		return err
	}
	if err := insertGovernanceAudit(ctx, tx, eventID, row.TenantID, command.Actor, "data_quality.dataset."+operation, "data_quality_dataset", row.DatasetID, command.TraceID, snapshot); err != nil {
		return err
	}
	return insertGovernanceReceipt(ctx, tx, command.TenantID, command.IdempotencyKey, requestSHA, command.ActionID, "dataset_"+operation, "dataset", row.DatasetID, row.Revision, eventID, snapshot)
}

func persistRuleGovernance(ctx context.Context, tx *sql.Tx, tenantID, actionID, idempotencyKey, requestSHA, actor, reason, traceID, operation, previousStatus string, row RuleRecord, eventID uuid.UUID) error {
	snapshot, _ := json.Marshal(row)
	if _, err := tx.ExecContext(ctx, `INSERT INTO data_quality_rule_history (event_id,tenant_id,rule_id,rule_version,revision,operation,previous_status,resulting_status,actor_id,reason,trace_id,snapshot) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)`, eventID, tenantID, row.RuleID, row.RuleVersion, row.Revision, operation, previousStatus, row.Status, actor, reason, traceID, string(snapshot)); err != nil {
		return fmt.Errorf("insert rule history: %w", err)
	}
	if err := insertGovernanceOutbox(ctx, tx, eventID, tenantID, "rule", row.RuleID, row.Revision, "DATA_QUALITY_RULE_"+strings.ToUpper(operation), traceID, snapshot); err != nil {
		return err
	}
	if err := insertGovernanceAudit(ctx, tx, eventID, tenantID, actor, "data_quality.rule."+operation, "data_quality_rule", row.RuleID, traceID, snapshot); err != nil {
		return err
	}
	return insertGovernanceReceipt(ctx, tx, tenantID, idempotencyKey, requestSHA, actionID, "rule_"+operation, "rule", row.RuleID, row.Revision, eventID, snapshot)
}

func insertGovernanceOutbox(ctx context.Context, tx *sql.Tx, eventID uuid.UUID, tenantID, aggregateType, aggregateID string, revision int64, eventType, traceID string, payload []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO data_quality_outbox (event_id,tenant_id,aggregate_type,aggregate_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id) VALUES ($1,$2,$3,$4,$5,$6,1,$7,$8::jsonb,$9)`, eventID, tenantID, aggregateType, aggregateID, revision, eventType, tenantID+":"+aggregateID, string(payload), traceID)
	if err != nil {
		return fmt.Errorf("insert data quality outbox: %w", err)
	}
	return nil
}

func insertGovernanceAudit(ctx context.Context, tx *sql.Tx, eventID uuid.UUID, tenantID, actor, action, objectType, objectID, traceID string, detail []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs (event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,success,result,risk_level) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,true,'success','medium')`, "dq-"+eventID.String(), tenantID, actor, action, objectType, objectID, string(detail), traceID)
	if err != nil {
		return fmt.Errorf("insert data quality audit: %w", err)
	}
	return nil
}

func insertGovernanceReceipt(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey, requestSHA, actionID, operation, aggregateType, aggregateID string, revision int64, eventID uuid.UUID, response []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO data_quality_command_requests (tenant_id,idempotency_key,request_sha256,action_id,operation,aggregate_type,aggregate_id,resulting_revision,event_id,response_payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)`, tenantID, idempotencyKey, requestSHA, actionID, operation, aggregateType, aggregateID, revision, eventID, string(response))
	if err != nil {
		return fmt.Errorf("insert data quality command receipt: %w", err)
	}
	return nil
}

func ruleCreateSnapshot(command RuleCreateCommand, ruleID string) map[string]interface{} {
	return map[string]interface{}{
		"tenant_id": command.TenantID, "rule_id": ruleID, "rule_key": command.RuleKey,
		"dataset_id": command.DatasetID, "rule_version": int64(1), "dimension": command.Dimension,
		"field_path": command.FieldPath, "predicate": command.Predicate, "threshold": command.Threshold,
		"window_seconds": command.WindowSeconds, "sampling": command.Sampling, "severity": command.Severity,
		"owner": command.Owner, "exemption_policy": command.ExemptionPolicy, "repair_action": command.RepairAction,
		"gate_policy": command.GatePolicy, "status": "draft", "revision": int64(1),
		"created_by": command.Actor, "approved_by": "", "reason": command.Reason, "trace_id": command.TraceID,
	}
}

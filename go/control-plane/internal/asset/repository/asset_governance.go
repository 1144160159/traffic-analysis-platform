package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

var (
	ErrAssetGovernanceNotFound            = errors.New("asset governance work order not found")
	ErrAssetGovernanceIdempotencyConflict = errors.New("asset governance idempotency conflict")
	ErrAssetGovernanceRevisionConflict    = errors.New("asset governance revision conflict")
	ErrAssetGovernanceStateConflict       = errors.New("asset governance state conflict")
	ErrAssetGovernanceSelfApproval        = errors.New("asset governance self approval forbidden")
	ErrAssetGovernanceEvidenceRequired    = errors.New("asset governance evidence required")
	ErrAssetGovernanceAssetStale          = errors.New("asset governance asset revision conflict")
)

const governanceColumns = `
	w.work_order_id::text,w.tenant_id,w.asset_id::text,w.action_id,
	w.source_lifecycle_state,w.target_lifecycle_state,
	COALESCE(w.target_asset_id::text,''),a.lifecycle_state,w.status,w.revision,
	w.expected_asset_revision,COALESCE(w.resulting_asset_revision,0),w.owner,
	w.requested_by,w.approved_by,w.due_at,w.evidence_required,w.evidence_refs,
	w.reason,w.external_system,w.external_ticket_id,w.external_status,w.trace_id,
	w.created_at,w.updated_at,w.completed_at`

func (r *AssetRepository) CreateAssetGovernanceWorkOrder(
	ctx context.Context,
	assetID string,
	command config.AssetGovernanceCreateCommand,
) (*config.AssetGovernanceWorkOrder, error) {
	identityJSON, _ := json.Marshal(map[string]any{
		"asset_id": assetID, "action_id": command.ActionID,
		"target_lifecycle_state": command.TargetLifecycleState,
		"target_asset_id":        command.TargetAssetID, "owner": command.Owner,
		"due_at":            command.DueAt.UTC().Format(time.RFC3339Nano),
		"evidence_required": command.EvidenceRequired, "reason": command.Reason,
		"expected_asset_revision": command.ExpectedAssetRevision, "actor": command.Actor,
	})
	digest := sha256.Sum256(identityJSON)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:asset-governance:%s", len(command.TenantID), command.TenantID, command.IdempotencyKey)); err != nil {
		return nil, err
	}
	prior, priorHash, err := getGovernanceByIdempotencyTx(ctx, tx, command.TenantID, command.IdempotencyKey)
	if err == nil {
		if priorHash != requestHash {
			return nil, ErrAssetGovernanceIdempotencyConflict
		}
		prior.IdempotentReplay = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var tenantID, lifecycle string
	var revision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT tenant_id,lifecycle_state,revision FROM assets
		 WHERE asset_id=$1 AND tenant_id=$2 FOR UPDATE`, assetID, command.TenantID).Scan(&tenantID, &lifecycle, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAssetGovernanceNotFound
		}
		return nil, err
	}
	if revision != command.ExpectedAssetRevision {
		return nil, ErrAssetGovernanceAssetStale
	}
	if lifecycle == command.TargetLifecycleState {
		return nil, ErrAssetGovernanceStateConflict
	}
	if command.TargetAssetID != "" {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT true FROM assets WHERE tenant_id=$1 AND asset_id=$2`, tenantID, command.TargetAssetID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrAssetGovernanceNotFound
			}
			return nil, err
		}
	}
	orderID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO asset_governance_work_orders(
		 work_order_id,tenant_id,asset_id,action_id,source_lifecycle_state,
		 target_lifecycle_state,target_asset_id,status,revision,expected_asset_revision,
		 owner,requested_by,due_at,evidence_required,reason,idempotency_key,request_hash,trace_id
		) VALUES($1,$2,$3,$4,$5,$6,
		  (SELECT asset_id FROM assets WHERE tenant_id=$2 AND asset_id::text=NULLIF($7,'')),
		  'pending_approval',1,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		orderID, tenantID, assetID, command.ActionID, lifecycle,
		command.TargetLifecycleState, command.TargetAssetID, revision, command.Owner,
		command.Actor, command.DueAt, command.EvidenceRequired, command.Reason,
		command.IdempotencyKey, requestHash, command.TraceID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Constraint == "uq_asset_governance_active_target" {
			return nil, ErrAssetGovernanceStateConflict
		}
		return nil, err
	}
	if err := insertGovernanceHistory(ctx, tx, orderID, tenantID, 1, command.ActionID,
		"", "pending_approval", lifecycle, lifecycle, command.Actor, command.Reason,
		nil, command.TraceID, map[string]any{"target_lifecycle_state": command.TargetLifecycleState}); err != nil {
		return nil, err
	}
	if err := insertGovernanceOutbox(ctx, tx, orderID, tenantID, assetID, 1,
		"traffic.asset.governance.v1.WorkOrderRequested", command.TraceID); err != nil {
		return nil, err
	}
	if err := insertGovernanceAudit(ctx, tx, orderID, tenantID, command.Actor,
		"ASSET_GOVERNANCE_WORK_ORDER_REQUESTED", command, command.TraceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAssetGovernanceWorkOrder(ctx, tenantID, orderID)
}

func (r *AssetRepository) GetAssetGovernanceWorkOrder(ctx context.Context, tenantID, orderID string) (*config.AssetGovernanceWorkOrder, error) {
	order, err := scanGovernance(r.db.QueryRowContext(ctx, `SELECT `+governanceColumns+`
		FROM asset_governance_work_orders w JOIN assets a ON a.asset_id=w.asset_id
		WHERE w.tenant_id=$1 AND w.work_order_id=$2`, tenantID, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAssetGovernanceNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT revision,action_id,from_status,to_status,
		from_lifecycle_state,to_lifecycle_state,actor,reason,evidence_refs,trace_id,detail,created_at
		FROM asset_governance_work_order_history WHERE tenant_id=$1 AND work_order_id=$2 ORDER BY revision`, tenantID, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h config.AssetGovernanceHistory
		var evidence, detail []byte
		if err := rows.Scan(&h.Revision, &h.ActionID, &h.FromStatus, &h.ToStatus, &h.FromLifecycleState,
			&h.ToLifecycleState, &h.Actor, &h.Reason, &evidence, &h.TraceID, &detail, &h.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(evidence, &h.EvidenceRefs)
		_ = json.Unmarshal(detail, &h.Detail)
		order.History = append(order.History, h)
	}
	return order, rows.Err()
}

func (r *AssetRepository) ListAssetGovernanceWorkOrders(ctx context.Context, tenantID, assetID string) ([]config.AssetGovernanceWorkOrder, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+governanceColumns+`
		FROM asset_governance_work_orders w JOIN assets a ON a.asset_id=w.asset_id
		WHERE w.tenant_id=$1 AND w.asset_id=$2 ORDER BY w.created_at DESC LIMIT 100`, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make([]config.AssetGovernanceWorkOrder, 0)
	for rows.Next() {
		o, err := scanGovernance(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *o)
	}
	return orders, rows.Err()
}

func (r *AssetRepository) ApplyAssetGovernanceAction(ctx context.Context, orderID string,
	command config.AssetGovernanceActionCommand) (*config.AssetGovernanceWorkOrder, error) {
	identityJSON, _ := json.Marshal(map[string]any{"work_order_id": orderID, "action_id": command.ActionID,
		"expected_revision": command.ExpectedRevision, "reason": command.Reason, "evidence_refs": command.EvidenceRefs,
		"actor": command.Actor})
	digest := sha256.Sum256(identityJSON)
	requestHash := hex.EncodeToString(digest[:])
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var priorHash, priorOrderID string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,work_order_id::text FROM asset_governance_control_requests
		WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, command.TenantID, command.IdempotencyKey).Scan(&priorHash, &priorOrderID)
	if err == nil {
		if priorHash != requestHash || priorOrderID != orderID {
			return nil, ErrAssetGovernanceIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		o, err := r.GetAssetGovernanceWorkOrder(ctx, command.TenantID, orderID)
		if o != nil {
			o.IdempotentReplay = true
		}
		return o, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	o, err := scanGovernance(tx.QueryRowContext(ctx, `SELECT `+governanceColumns+`
		FROM asset_governance_work_orders w JOIN assets a ON a.asset_id=w.asset_id
		WHERE w.tenant_id=$1 AND w.work_order_id=$2 FOR UPDATE OF w,a`, command.TenantID, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAssetGovernanceNotFound
	}
	if err != nil {
		return nil, err
	}
	if o.Revision != command.ExpectedRevision {
		return nil, ErrAssetGovernanceRevisionConflict
	}
	toStatus, eventType, err := governanceTransition(o, command)
	if err != nil {
		return nil, err
	}
	newRevision := o.Revision + 1
	resultingRevision := o.ResultingAssetRevision
	toLifecycle := o.CurrentLifecycleState
	if command.ActionID == "asset-governance-complete" {
		if o.EvidenceRequired && len(command.EvidenceRefs) == 0 {
			return nil, ErrAssetGovernanceEvidenceRequired
		}
		var assetEventID = uuid.NewString()
		err = tx.QueryRowContext(ctx, `UPDATE assets SET lifecycle_state=$1,
			merged_into_asset_id=CASE WHEN $1='merged' THEN
			  (SELECT target.asset_id FROM assets target WHERE target.tenant_id=$3 AND target.asset_id::text=NULLIF($2,''))
			  ELSE NULL END,
			revision=revision+1,updated_at=now() WHERE tenant_id=$3 AND asset_id=$4 AND revision=$5
			RETURNING revision`, o.TargetLifecycleState, o.TargetAssetID, o.TenantID, o.AssetID, o.ExpectedAssetRevision).Scan(&resultingRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAssetGovernanceAssetStale
		}
		if err != nil {
			return nil, err
		}
		toLifecycle = o.TargetLifecycleState
		if err := insertAssetLifecycleEvent(ctx, tx, o, resultingRevision, assetEventID, command.TraceID, false); err != nil {
			return nil, err
		}
	}
	if command.ActionID == "asset-governance-compensate" {
		if o.ResultingAssetRevision == 0 {
			return nil, ErrAssetGovernanceStateConflict
		}
		var assetEventID = uuid.NewString()
		err = tx.QueryRowContext(ctx, `UPDATE assets SET lifecycle_state=$1,merged_into_asset_id=NULL,
			revision=revision+1,updated_at=now() WHERE tenant_id=$2 AND asset_id=$3 AND revision=$4
			RETURNING revision`, o.SourceLifecycleState, o.TenantID, o.AssetID, o.ResultingAssetRevision).Scan(&resultingRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAssetGovernanceAssetStale
		}
		if err != nil {
			return nil, err
		}
		toLifecycle = o.SourceLifecycleState
		if err := insertAssetLifecycleEvent(ctx, tx, o, resultingRevision, assetEventID, command.TraceID, true); err != nil {
			return nil, err
		}
	}
	if command.EvidenceRefs == nil {
		command.EvidenceRefs = []string{}
	}
	evidenceJSON, _ := json.Marshal(command.EvidenceRefs)
	_, err = tx.ExecContext(ctx, `UPDATE asset_governance_work_orders SET status=$1,revision=$2,
		approved_by=CASE WHEN $3='asset-governance-approve' THEN $4 ELSE approved_by END,
		evidence_refs=CASE WHEN jsonb_array_length($5::jsonb)>0 THEN $5::jsonb ELSE evidence_refs END,
		resulting_asset_revision=NULLIF($6,0),updated_at=now(),
		completed_at=CASE WHEN $1 IN ('completed','compensated') THEN now() ELSE completed_at END,
		trace_id=$7 WHERE tenant_id=$8 AND work_order_id=$9`, toStatus, newRevision, command.ActionID,
		command.Actor, string(evidenceJSON), resultingRevision, command.TraceID, o.TenantID, o.WorkOrderID)
	if err != nil {
		return nil, err
	}
	if err := insertGovernanceHistory(ctx, tx, o.WorkOrderID, o.TenantID, newRevision, command.ActionID,
		o.Status, toStatus, o.CurrentLifecycleState, toLifecycle, command.Actor, command.Reason, command.EvidenceRefs,
		command.TraceID, map[string]any{"asset_revision": resultingRevision}); err != nil {
		return nil, err
	}
	if err := insertGovernanceOutbox(ctx, tx, o.WorkOrderID, o.TenantID, o.AssetID, newRevision, eventType, command.TraceID); err != nil {
		return nil, err
	}
	resultJSON, _ := json.Marshal(map[string]any{"work_order_id": o.WorkOrderID, "revision": newRevision, "status": toStatus,
		"resulting_asset_revision": resultingRevision})
	_, err = tx.ExecContext(ctx, `INSERT INTO asset_governance_control_requests(request_id,tenant_id,work_order_id,
		idempotency_key,request_hash,action_id,actor,expected_revision,resulting_revision,trace_id,result)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)`, uuid.NewString(), o.TenantID, o.WorkOrderID,
		command.IdempotencyKey, requestHash, command.ActionID, command.Actor, command.ExpectedRevision, newRevision,
		command.TraceID, string(resultJSON))
	if err != nil {
		return nil, err
	}
	if err := insertGovernanceAudit(ctx, tx, o.WorkOrderID, o.TenantID, command.Actor,
		"ASSET_GOVERNANCE_"+toStatus, command, command.TraceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAssetGovernanceWorkOrder(ctx, o.TenantID, o.WorkOrderID)
}

func governanceTransition(o *config.AssetGovernanceWorkOrder, c config.AssetGovernanceActionCommand) (string, string, error) {
	type transition struct{ from, to, event string }
	allowed := map[string]transition{
		"asset-governance-approve":    {"pending_approval", "approved", "traffic.asset.governance.v1.WorkOrderApproved"},
		"asset-governance-reject":     {"pending_approval", "rejected", "traffic.asset.governance.v1.WorkOrderRejected"},
		"asset-governance-start":      {"approved", "executing", "traffic.asset.governance.v1.WorkOrderStarted"},
		"asset-governance-complete":   {"executing", "completed", "traffic.asset.governance.v1.WorkOrderCompleted"},
		"asset-governance-fail":       {"executing", "failed", "traffic.asset.governance.v1.WorkOrderFailed"},
		"asset-governance-compensate": {"completed", "compensated", "traffic.asset.governance.v1.WorkOrderCompensated"},
	}
	t, ok := allowed[c.ActionID]
	if c.ActionID == "asset-governance-cancel" {
		if o.Status != "pending_approval" && o.Status != "approved" && o.Status != "executing" {
			return "", "", ErrAssetGovernanceStateConflict
		}
		return "cancelled", "traffic.asset.governance.v1.WorkOrderCancelled", nil
	}
	if !ok || o.Status != t.from {
		return "", "", ErrAssetGovernanceStateConflict
	}
	if c.ActionID == "asset-governance-approve" && c.Actor == o.RequestedBy {
		return "", "", ErrAssetGovernanceSelfApproval
	}
	if (c.ActionID == "asset-governance-start" || c.ActionID == "asset-governance-complete" || c.ActionID == "asset-governance-fail") && c.Actor != o.Owner {
		return "", "", ErrAssetGovernanceStateConflict
	}
	return t.to, t.event, nil
}

func insertAssetLifecycleEvent(ctx context.Context, tx *sql.Tx, o *config.AssetGovernanceWorkOrder,
	revision int64, eventID, traceID string, compensating bool) error {
	fromState, toState := o.CurrentLifecycleState, o.TargetLifecycleState
	eventType := "governance_lifecycle_changed"
	if compensating {
		toState = o.SourceLifecycleState
		eventType = "governance_lifecycle_compensated"
	}
	oldJSON, _ := json.Marshal(map[string]any{"lifecycle_state": fromState, "revision": revision - 1})
	newJSON, _ := json.Marshal(map[string]any{"lifecycle_state": toState, "revision": revision, "work_order_id": o.WorkOrderID})
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_events(event_uuid,asset_id,tenant_id,event_type,revision,trace_id,old_value,new_value)
		VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb)`, eventID, o.AssetID, o.TenantID, eventType, revision, traceID, string(oldJSON), string(newJSON))
	return err
}

func scanGovernance(row interface{ Scan(...any) error }) (*config.AssetGovernanceWorkOrder, error) {
	var o config.AssetGovernanceWorkOrder
	var evidence []byte
	err := row.Scan(&o.WorkOrderID, &o.TenantID, &o.AssetID, &o.ActionID, &o.SourceLifecycleState,
		&o.TargetLifecycleState, &o.TargetAssetID, &o.CurrentLifecycleState, &o.Status, &o.Revision,
		&o.ExpectedAssetRevision, &o.ResultingAssetRevision, &o.Owner, &o.RequestedBy, &o.ApprovedBy,
		&o.DueAt, &o.EvidenceRequired, &evidence, &o.Reason, &o.ExternalSystem, &o.ExternalTicketID,
		&o.ExternalStatus, &o.TraceID, &o.CreatedAt, &o.UpdatedAt, &o.CompletedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(evidence, &o.EvidenceRefs)
	return &o, nil
}

func getGovernanceByIdempotencyTx(ctx context.Context, tx *sql.Tx, tenantID, key string) (*config.AssetGovernanceWorkOrder, string, error) {
	var hash string
	o, err := scanGovernanceWithHash(tx.QueryRowContext(ctx, `SELECT `+governanceColumns+`,w.request_hash
		FROM asset_governance_work_orders w JOIN assets a ON a.asset_id=w.asset_id
		WHERE w.tenant_id=$1 AND w.idempotency_key=$2 FOR UPDATE OF w`, tenantID, key), &hash)
	return o, hash, err
}

func scanGovernanceWithHash(row interface{ Scan(...any) error }, hash *string) (*config.AssetGovernanceWorkOrder, error) {
	var o config.AssetGovernanceWorkOrder
	var evidence []byte
	err := row.Scan(&o.WorkOrderID, &o.TenantID, &o.AssetID, &o.ActionID, &o.SourceLifecycleState,
		&o.TargetLifecycleState, &o.TargetAssetID, &o.CurrentLifecycleState, &o.Status, &o.Revision,
		&o.ExpectedAssetRevision, &o.ResultingAssetRevision, &o.Owner, &o.RequestedBy, &o.ApprovedBy,
		&o.DueAt, &o.EvidenceRequired, &evidence, &o.Reason, &o.ExternalSystem, &o.ExternalTicketID,
		&o.ExternalStatus, &o.TraceID, &o.CreatedAt, &o.UpdatedAt, &o.CompletedAt, hash)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(evidence, &o.EvidenceRefs)
	return &o, nil
}

func insertGovernanceHistory(ctx context.Context, tx *sql.Tx, orderID, tenantID string, revision int64,
	actionID, fromStatus, toStatus, fromLifecycle, toLifecycle, actor, reason string,
	evidence []string, traceID string, detail map[string]any) error {
	if evidence == nil {
		evidence = []string{}
	}
	evidenceJSON, _ := json.Marshal(evidence)
	detailJSON, _ := json.Marshal(detail)
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_governance_work_order_history(
		work_order_id,tenant_id,revision,action_id,from_status,to_status,from_lifecycle_state,
		to_lifecycle_state,actor,reason,evidence_refs,trace_id,detail)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13::jsonb)`, orderID, tenantID, revision,
		actionID, fromStatus, toStatus, fromLifecycle, toLifecycle, actor, reason, string(evidenceJSON), traceID, string(detailJSON))
	return err
}

func insertGovernanceOutbox(ctx context.Context, tx *sql.Tx, orderID, tenantID, assetID string,
	revision int64, eventType, traceID string) error {
	eventID := uuid.NewString()
	payload, _ := json.Marshal(map[string]any{"event_id": eventID, "event_type": eventType,
		"schema_version": 1, "aggregate_version": revision, "partition_key": tenantID + ":" + orderID,
		"tenant_id": tenantID, "work_order_id": orderID, "asset_id": assetID, "trace_id": traceID})
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_governance_outbox(event_id,tenant_id,work_order_id,
		aggregate_version,partition_key,event_type,payload) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`,
		eventID, tenantID, orderID, revision, tenantID+":"+orderID, eventType, string(payload))
	return err
}

func insertGovernanceAudit(ctx context.Context, tx *sql.Tx, orderID, tenantID, actor, action string,
	command any, traceID string) error {
	detail, _ := json.Marshal(command)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(event_id,tenant_id,user_id,action,object_type,
		object_id,detail,request_id,trace_id,success,risk_level,result)
		VALUES($1,$2,$3,$4,'asset_governance_work_order',$5,$6::jsonb,$7,$7,true,'high','accepted')`,
		"asset-governance-audit-"+uuid.NewString(), tenantID, actor, action, orderID, string(detail), traceID)
	return err
}

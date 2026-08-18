package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func (repository *Repository) RequestApprovalTx(
	ctx context.Context,
	tx *sql.Tx,
	request ApprovalRequest,
	now time.Time,
) (ApprovalReceipt, error) {
	if tx == nil {
		return ApprovalReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := request.Validate(now); err != nil {
		return ApprovalReceipt{}, err
	}
	requestSHA, err := canonicalSHA256(map[string]interface{}{
		"tenant_id": request.TenantID, "baseline_id": request.BaselineID, "baseline_version": request.BaselineVersion,
		"expected_revision": request.ExpectedRevision, "candidate_sha256": request.CandidateSHA256,
		"requested_by": request.RequestedBy, "reason": request.Reason, "expires_at": request.ExpiresAt.UTC(),
	})
	if err != nil {
		return ApprovalReceipt{}, err
	}
	// 幂等键 advisory lock 先于查重：并发相同审批请求串行化，避免
	// 双插 approval 记录与回执。
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		request.TenantID+"::"+request.IdempotencyKey); err != nil {
		return ApprovalReceipt{}, fmt.Errorf("lock baseline approval idempotency key: %w", err)
	}
	if receipt, found, err := lookupApprovalReceiptTx(ctx, tx, request.TenantID, request.IdempotencyKey, requestSHA); err != nil || found {
		return receipt, err
	}
	var definitionRevision int64
	var expectedConsumers []string
	err = tx.QueryRowContext(ctx, `SELECT revision,expected_consumers FROM behavior_baseline_definitions_v1
		WHERE tenant_id=$1 AND baseline_id=$2 FOR UPDATE`, request.TenantID, request.BaselineID).Scan(
		&definitionRevision, pq.Array(&expectedConsumers))
	if err == sql.ErrNoRows {
		return ApprovalReceipt{}, fmt.Errorf("%w: baseline definition not found", ErrStateConflict)
	}
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("lock baseline definition for approval: %w", err)
	}
	if definitionRevision != request.ExpectedRevision {
		return ApprovalReceipt{}, fmt.Errorf("%w: expected definition revision %d, current %d", ErrRevisionConflict,
			request.ExpectedRevision, definitionRevision)
	}
	var state, quality, snapshotSHA, candidateSHA string
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state,quality_status,snapshot_sha256,candidate_sha256
		FROM behavior_baseline_versions_v1 WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 FOR UPDATE`,
		request.TenantID, request.BaselineID, request.BaselineVersion).Scan(&state, &quality, &snapshotSHA, &candidateSHA)
	if err == sql.ErrNoRows {
		return ApprovalReceipt{}, fmt.Errorf("%w: baseline version not found", ErrStateConflict)
	}
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("lock baseline version for approval: %w", err)
	}
	if state != "frozen" || quality != "complete" {
		return ApprovalReceipt{}, fmt.Errorf("%w: only complete frozen versions can request activation", ErrStateConflict)
	}
	if candidateSHA != request.CandidateSHA256 {
		return ApprovalReceipt{}, fmt.Errorf("%w: approval candidate differs from frozen version", ErrIdentityConflict)
	}
	approvalID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_approval_requests_v1 (
		approval_id,tenant_id,baseline_id,baseline_version,definition_revision,snapshot_sha256,candidate_sha256,
		status,requested_by,reason,requested_at,expires_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$11)`, approvalID, request.TenantID, request.BaselineID,
		request.BaselineVersion, definitionRevision, snapshotSHA, candidateSHA, request.RequestedBy, request.Reason, now, request.ExpiresAt)
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("create behavior baseline approval: %w", err)
	}
	receipt := ApprovalReceipt{ApprovalID: approvalID, BaselineID: request.BaselineID,
		BaselineVersion: request.BaselineVersion, Status: "pending", ExpectedConsumers: uniqueSorted(expectedConsumers)}
	responseJSON, _ := json.Marshal(receipt)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_command_receipts_v1 (
		tenant_id,idempotency_key,command_type,request_sha256,response_status,response_body
	) VALUES ($1,$2,'approval.request',$3,202,$4::jsonb)`, request.TenantID, request.IdempotencyKey, requestSHA, string(responseJSON))
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("record behavior baseline approval receipt: %w", err)
	}
	return receipt, nil
}

func (repository *Repository) DecideApprovalTx(
	ctx context.Context,
	tx *sql.Tx,
	decision ApprovalDecision,
	now time.Time,
) (ApprovalReceipt, error) {
	if tx == nil {
		return ApprovalReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := decision.Validate(); err != nil {
		return ApprovalReceipt{}, err
	}
	var baselineID, versionID, status, requestedBy, buildRequestedBy, snapshotSHA, candidateSHA, versionState, versionQuality string
	var baselineKind, algorithmVersion, thresholdSpecJSON, statisticsJSON string
	var version, revision int64
	var expiresAt time.Time
	err := tx.QueryRowContext(ctx, `SELECT a.baseline_id,a.baseline_version,v.version_id::text,a.definition_revision,a.status,a.requested_by,
		a.snapshot_sha256,a.candidate_sha256,a.expires_at,v.lifecycle_state,v.quality_status,COALESCE(j.requested_by,''),
		v.baseline_kind,v.algorithm_version,v.threshold_spec::text,v.statistics::text
		FROM behavior_baseline_approval_requests_v1 a
		JOIN behavior_baseline_versions_v1 v ON v.tenant_id=a.tenant_id AND v.baseline_id=a.baseline_id
		 AND v.baseline_version=a.baseline_version AND v.snapshot_sha256=a.snapshot_sha256 AND v.candidate_sha256=a.candidate_sha256
		LEFT JOIN behavior_baseline_build_jobs_v1 j ON j.tenant_id=v.tenant_id AND j.baseline_id=v.baseline_id
		 AND j.result_version_id=v.version_id
		WHERE a.tenant_id=$1 AND a.approval_id=$2 FOR UPDATE OF a,v`, decision.TenantID, decision.ApprovalID).Scan(
		&baselineID, &version, &versionID, &revision, &status, &requestedBy, &snapshotSHA, &candidateSHA, &expiresAt,
		&versionState, &versionQuality, &buildRequestedBy, &baselineKind, &algorithmVersion, &thresholdSpecJSON, &statisticsJSON)
	if err == sql.ErrNoRows {
		return ApprovalReceipt{}, fmt.Errorf("%w: approval not found", ErrStateConflict)
	}
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("lock behavior baseline approval: %w", err)
	}
	if status != "pending" {
		return ApprovalReceipt{}, fmt.Errorf("%w: approval is already %s", ErrStateConflict, status)
	}
	if requestedBy == decision.DecidedBy || buildRequestedBy == decision.DecidedBy {
		return ApprovalReceipt{}, fmt.Errorf("%w: build requester cannot approve activation", ErrStateConflict)
	}
	if versionState != "frozen" || versionQuality != "complete" {
		return ApprovalReceipt{}, fmt.Errorf("%w: approval version is no longer complete and frozen", ErrStateConflict)
	}
	if !expiresAt.After(now) {
		update, updateErr := tx.ExecContext(ctx, `UPDATE behavior_baseline_approval_requests_v1 SET status='expired'
			WHERE tenant_id=$1 AND approval_id=$2 AND status='pending'`, decision.TenantID, decision.ApprovalID)
		if updateErr != nil {
			return ApprovalReceipt{}, fmt.Errorf("expire behavior baseline approval: %w", updateErr)
		}
		if affected, _ := update.RowsAffected(); affected != 1 {
			return ApprovalReceipt{}, fmt.Errorf("%w: approval expiry update lost", ErrStateConflict)
		}
		return ApprovalReceipt{ApprovalID: decision.ApprovalID, BaselineID: baselineID,
			BaselineVersion: version, Status: "expired"}, nil
	}
	if revision != decision.ExpectedRevision || candidateSHA != decision.CandidateSHA256 {
		return ApprovalReceipt{}, fmt.Errorf("%w: approval candidate or definition revision changed", ErrRevisionConflict)
	}
	newStatus := "rejected"
	if decision.Approve {
		newStatus = "approved"
	}
	update, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_approval_requests_v1 SET status=$1,decided_by=$2,
		reason=$3,decided_at=$4 WHERE tenant_id=$5 AND approval_id=$6 AND status='pending'`, newStatus,
		decision.DecidedBy, decision.Reason, now, decision.TenantID, decision.ApprovalID)
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("decide behavior baseline approval: %w", err)
	}
	// 检查影响行数：并发决策时后到的 UPDATE 影响 0 行，必须拒绝而不是
	// 继续进入 approve 分支造成双重审批/重复激活。
	if affected, affectedErr := update.RowsAffected(); affectedErr != nil || affected != 1 {
		return ApprovalReceipt{}, fmt.Errorf("%w: approval decision update lost (concurrent decision)", ErrStateConflict)
	}
	receipt := ApprovalReceipt{ApprovalID: decision.ApprovalID, BaselineID: baselineID,
		BaselineVersion: version, Status: newStatus}
	if !decision.Approve {
		return receipt, nil
	}
	var expectedConsumers []string
	err = tx.QueryRowContext(ctx, `SELECT expected_consumers FROM behavior_baseline_definitions_v1
		WHERE tenant_id=$1 AND baseline_id=$2 AND revision=$3 FOR UPDATE`, decision.TenantID, baselineID, revision).Scan(pq.Array(&expectedConsumers))
	if err == sql.ErrNoRows {
		return ApprovalReceipt{}, fmt.Errorf("%w: approved definition revision is stale", ErrRevisionConflict)
	}
	if err != nil {
		return ApprovalReceipt{}, fmt.Errorf("read behavior baseline activation consumers: %w", err)
	}
	for _, consumerID := range uniqueSorted(expectedConsumers) {
		_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_activation_targets_v1 (
			tenant_id,baseline_id,baseline_version,consumer_id,required,status,candidate_sha256
		) VALUES ($1,$2,$3,$4,true,'pending',$5)`, decision.TenantID, baselineID, version, consumerID, candidateSHA)
		if err != nil {
			return ApprovalReceipt{}, fmt.Errorf("create behavior baseline activation target %s: %w", consumerID, err)
		}
	}
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.activation.requested.v1", "schema_version": 1,
		"tenant_id": decision.TenantID, "baseline_id": baselineID, "baseline_version": version,
		"definition_revision": revision, "snapshot_sha256": snapshotSHA, "candidate_sha256": candidateSHA,
		"baseline_kind": baselineKind, "algorithm_version": algorithmVersion,
		"threshold_spec": json.RawMessage(thresholdSpecJSON), "statistics": json.RawMessage(statisticsJSON),
		"approval_id": decision.ApprovalID, "expected_consumers": uniqueSorted(expectedConsumers),
		"partition_key": decision.TenantID + ":" + baselineID, "trace_id": decision.TraceID,
	}
	if err := appendOutboxTx(ctx, tx, decision.TenantID, baselineID, "baseline_version", versionID,
		version, "baseline.activation.requested.v1", eventID, decision.TraceID, payload); err != nil {
		return ApprovalReceipt{}, err
	}
	receipt.ExpectedConsumers = uniqueSorted(expectedConsumers)
	receipt.EventID = eventID
	return receipt, nil
}

func lookupApprovalReceiptTx(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey, requestSHA string) (ApprovalReceipt, bool, error) {
	var storedSHA, commandType, responseJSON string
	err := tx.QueryRowContext(ctx, `SELECT command_type,request_sha256,response_body::text
		FROM behavior_baseline_command_receipts_v1 WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		tenantID, idempotencyKey).Scan(&commandType, &storedSHA, &responseJSON)
	if err == sql.ErrNoRows {
		return ApprovalReceipt{}, false, nil
	}
	if err != nil {
		return ApprovalReceipt{}, false, fmt.Errorf("read behavior baseline approval receipt: %w", err)
	}
	if commandType != "approval.request" || storedSHA != requestSHA {
		return ApprovalReceipt{}, false, fmt.Errorf("%w: idempotency key reused with different approval bytes", ErrIdentityConflict)
	}
	var receipt ApprovalReceipt
	if err := json.Unmarshal([]byte(responseJSON), &receipt); err != nil {
		return ApprovalReceipt{}, false, fmt.Errorf("decode behavior baseline approval receipt: %w", err)
	}
	receipt.Replayed = true
	return receipt, true, nil
}

package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (repository *Repository) RecordActivationAckTx(
	ctx context.Context,
	tx *sql.Tx,
	ack ActivationAck,
) (ActivationReceipt, error) {
	if tx == nil {
		return ActivationReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := ack.Validate(); err != nil {
		return ActivationReceipt{}, err
	}
	var targetStatus, targetCandidate, storedEventID, storedAckSHA string
	err := tx.QueryRowContext(ctx, `SELECT status,candidate_sha256,COALESCE(ack_event_id::text,''),ack_sha256
		FROM behavior_baseline_activation_targets_v1
		WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 AND consumer_id=$4 FOR UPDATE`,
		ack.TenantID, ack.BaselineID, ack.BaselineVersion, ack.ConsumerID).Scan(
		&targetStatus, &targetCandidate, &storedEventID, &storedAckSHA)
	if err == sql.ErrNoRows {
		return ActivationReceipt{}, fmt.Errorf("%w: activation target is not registered", ErrStateConflict)
	}
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("lock behavior baseline activation target: %w", err)
	}
	if targetCandidate != ack.CandidateSHA256 {
		return ActivationReceipt{}, fmt.Errorf("%w: activation ACK candidate changed", ErrIdentityConflict)
	}
	if targetStatus == "acked" {
		if storedEventID != ack.EventID || storedAckSHA != ack.AckSHA256 {
			return ActivationReceipt{}, fmt.Errorf("%w: consumer replay changed ACK identity", ErrIdentityConflict)
		}
		receipt, err := activationReceiptTx(ctx, tx, ack.TenantID, ack.BaselineID, ack.BaselineVersion, ack.ConsumerID)
		if err != nil {
			return ActivationReceipt{}, err
		}
		var lifecycleState string
		if err := tx.QueryRowContext(ctx, `SELECT lifecycle_state FROM behavior_baseline_versions_v1
			WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3`, ack.TenantID,
			ack.BaselineID, ack.BaselineVersion).Scan(&lifecycleState); err != nil {
			return ActivationReceipt{}, fmt.Errorf("read replayed behavior baseline activation state: %w", err)
		}
		if lifecycleState == "active" || lifecycleState == "retired" {
			receipt.LifecycleState = lifecycleState
		}
		receipt.Replayed = true
		return receipt, nil
	}
	if targetStatus != "pending" {
		return ActivationReceipt{}, fmt.Errorf("%w: activation target is %s", ErrStateConflict, targetStatus)
	}
	var versionID, versionState, qualityStatus, snapshotSHA, versionCandidate string
	var definitionRevision int64
	err = tx.QueryRowContext(ctx, `SELECT version_id::text,lifecycle_state,quality_status,snapshot_sha256,candidate_sha256,definition_revision
		FROM behavior_baseline_versions_v1 WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 FOR UPDATE`,
		ack.TenantID, ack.BaselineID, ack.BaselineVersion).Scan(
		&versionID, &versionState, &qualityStatus, &snapshotSHA, &versionCandidate, &definitionRevision)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("lock behavior baseline activation version: %w", err)
	}
	if versionState != "frozen" || qualityStatus != "complete" {
		return ActivationReceipt{}, fmt.Errorf("%w: activation version is not complete and frozen", ErrStateConflict)
	}
	if snapshotSHA != ack.SnapshotSHA256 || versionCandidate != ack.CandidateSHA256 {
		return ActivationReceipt{}, fmt.Errorf("%w: activation ACK does not match frozen version", ErrIdentityConflict)
	}
	update, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_activation_targets_v1 SET status='acked',
		ack_event_id=$1,ack_sha256=$2,error_detail='',acknowledged_at=$3,updated_at=now()
		WHERE tenant_id=$4 AND baseline_id=$5 AND baseline_version=$6 AND consumer_id=$7 AND status='pending'`,
		ack.EventID, ack.AckSHA256, ack.AppliedAt, ack.TenantID, ack.BaselineID, ack.BaselineVersion, ack.ConsumerID)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("record behavior baseline activation ACK: %w", err)
	}
	if affected, _ := update.RowsAffected(); affected != 1 {
		return ActivationReceipt{}, fmt.Errorf("%w: activation ACK update lost", ErrStateConflict)
	}
	receipt, err := activationReceiptTx(ctx, tx, ack.TenantID, ack.BaselineID, ack.BaselineVersion, ack.ConsumerID)
	if err != nil || len(receipt.PendingConsumers) > 0 || len(receipt.FailedConsumers) > 0 {
		return receipt, err
	}
	var definitionState string
	var currentRevision int64
	var activeVersion sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state,revision,active_version FROM behavior_baseline_definitions_v1
		WHERE tenant_id=$1 AND baseline_id=$2 FOR UPDATE`, ack.TenantID, ack.BaselineID).Scan(
		&definitionState, &currentRevision, &activeVersion)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("lock behavior baseline definition for activation: %w", err)
	}
	if currentRevision != definitionRevision {
		return ActivationReceipt{}, fmt.Errorf("%w: definition changed after approval", ErrRevisionConflict)
	}
	if activeVersion.Valid && activeVersion.Int64 == ack.BaselineVersion {
		receipt.LifecycleState = "active"
		return receipt, nil
	}
	var retiredEventID string
	if activeVersion.Valid {
		var oldVersionID, oldSnapshotSHA string
		if err := tx.QueryRowContext(ctx, `SELECT version_id::text,snapshot_sha256
			FROM behavior_baseline_versions_v1 WHERE tenant_id=$1 AND baseline_id=$2
			AND baseline_version=$3 AND lifecycle_state='active' FOR UPDATE`, ack.TenantID,
			ack.BaselineID, activeVersion.Int64).Scan(&oldVersionID, &oldSnapshotSHA); err != nil {
			return ActivationReceipt{}, fmt.Errorf("lock previous active behavior baseline version: %w", err)
		}
		oldUpdate, updateErr := tx.ExecContext(ctx, `UPDATE behavior_baseline_versions_v1 SET lifecycle_state='retired',
			retired_at=now() WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 AND lifecycle_state='active'`,
			ack.TenantID, ack.BaselineID, activeVersion.Int64)
		if updateErr != nil {
			return ActivationReceipt{}, fmt.Errorf("retire previous behavior baseline version: %w", updateErr)
		}
		if affected, _ := oldUpdate.RowsAffected(); affected != 1 {
			return ActivationReceipt{}, fmt.Errorf("%w: previous active version changed", ErrStateConflict)
		}
		retiredEventID = uuid.NewString()
		retiredPayload := map[string]interface{}{
			"event_id": retiredEventID, "event_type": "baseline.version.retired.v1", "schema_version": 1,
			"tenant_id": ack.TenantID, "baseline_id": ack.BaselineID,
			"baseline_version": activeVersion.Int64, "retired_by_version": ack.BaselineVersion,
			"snapshot_sha256": oldSnapshotSHA, "candidate_sha256": ack.CandidateSHA256,
			"trace_id": ack.TraceID,
		}
		if err := appendOutboxTx(ctx, tx, ack.TenantID, ack.BaselineID, "baseline_version", oldVersionID,
			activeVersion.Int64, "baseline.version.retired.v1", retiredEventID, ack.TraceID, retiredPayload); err != nil {
			return ActivationReceipt{}, err
		}
	}
	newUpdate, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_versions_v1 SET lifecycle_state='active',activated_at=now()
		WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 AND lifecycle_state='frozen'`,
		ack.TenantID, ack.BaselineID, ack.BaselineVersion)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("activate behavior baseline version: %w", err)
	}
	if affected, _ := newUpdate.RowsAffected(); affected != 1 {
		return ActivationReceipt{}, fmt.Errorf("%w: frozen activation version changed", ErrStateConflict)
	}
	definitionUpdate, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_definitions_v1 SET lifecycle_state='active',
		previous_stable_version=active_version,active_version=$1,revision=revision+1,updated_by=$2,updated_at=now()
		WHERE tenant_id=$3 AND baseline_id=$4 AND revision=$5`, ack.BaselineVersion, ack.ConsumerID,
		ack.TenantID, ack.BaselineID, currentRevision)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("switch active behavior baseline version: %w", err)
	}
	if affected, _ := definitionUpdate.RowsAffected(); affected != 1 {
		return ActivationReceipt{}, fmt.Errorf("%w: definition activation lost", ErrRevisionConflict)
	}
	_, err = tx.ExecContext(ctx, `UPDATE behavior_baseline_approval_requests_v1 SET status='consumed',consumed_at=now()
		WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 AND status='approved'`,
		ack.TenantID, ack.BaselineID, ack.BaselineVersion)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("consume behavior baseline approval: %w", err)
	}
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.version.activated.v1", "schema_version": 1,
		"tenant_id": ack.TenantID, "baseline_id": ack.BaselineID, "baseline_version": ack.BaselineVersion,
		"snapshot_sha256": snapshotSHA, "candidate_sha256": ack.CandidateSHA256,
		"acked_consumers": receipt.AckedConsumers, "trace_id": ack.TraceID,
	}
	if err := appendOutboxTx(ctx, tx, ack.TenantID, ack.BaselineID, "baseline_version", versionID,
		ack.BaselineVersion, "baseline.version.activated.v1", eventID, ack.TraceID, payload); err != nil {
		return ActivationReceipt{}, err
	}
	if err := appendHistoryTx(ctx, tx, ack.TenantID, ack.BaselineID, currentRevision+1, &ack.BaselineVersion,
		definitionState, "active", "baseline.version.activated.v1", "all required consumers acknowledged",
		ack.ConsumerID, ack.TraceID, map[string]interface{}{
			"snapshot_sha256": snapshotSHA, "retired_event_id": retiredEventID,
		}); err != nil {
		return ActivationReceipt{}, err
	}
	receipt.LifecycleState = "active"
	receipt.ActivationEventID = eventID
	return receipt, nil
}

func activationReceiptTx(ctx context.Context, tx *sql.Tx, tenantID, baselineID string, version int64, consumerID string) (ActivationReceipt, error) {
	rows, err := tx.QueryContext(ctx, `SELECT consumer_id,status FROM behavior_baseline_activation_targets_v1
		WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3 AND required=true ORDER BY consumer_id`,
		tenantID, baselineID, version)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("read behavior baseline activation target set: %w", err)
	}
	defer rows.Close()
	acked, pending, failed := make([]string, 0), make([]string, 0), make([]string, 0)
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			return ActivationReceipt{}, fmt.Errorf("scan behavior baseline activation target: %w", err)
		}
		switch state {
		case "acked":
			acked = append(acked, id)
		case "pending":
			pending = append(pending, id)
		case "failed":
			failed = append(failed, id)
		}
	}
	if err := rows.Err(); err != nil {
		return ActivationReceipt{}, fmt.Errorf("iterate behavior baseline activation targets: %w", err)
	}
	state := "frozen"
	if len(pending) == 0 && len(failed) == 0 {
		state = "ready_to_activate"
	}
	return ActivationReceipt{BaselineID: baselineID, BaselineVersion: version, ConsumerID: consumerID,
		AckStatus: "acked", AckedConsumers: acked, PendingConsumers: pending,
		FailedConsumers: failed, LifecycleState: state}, nil
}

func (repository *Repository) RequestRollbackTx(
	ctx context.Context,
	tx *sql.Tx,
	request RollbackRequest,
) (RollbackReceipt, error) {
	if tx == nil {
		return RollbackReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return RollbackReceipt{}, err
	}
	requestSHA, err := canonicalSHA256(request)
	if err != nil {
		return RollbackReceipt{}, err
	}
	if receipt, found, err := lookupRollbackReceiptTx(ctx, tx, request.TenantID, request.IdempotencyKey, requestSHA); err != nil || found {
		return receipt, err
	}
	var state string
	var revision, nextVersion int64
	var activeVersion, previousStable sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state,revision,next_version,active_version,previous_stable_version
		FROM behavior_baseline_definitions_v1 WHERE tenant_id=$1 AND baseline_id=$2 FOR UPDATE`,
		request.TenantID, request.BaselineID).Scan(&state, &revision, &nextVersion, &activeVersion, &previousStable)
	if err != nil {
		return RollbackReceipt{}, fmt.Errorf("lock behavior baseline definition for rollback: %w", err)
	}
	if revision != request.ExpectedRevision {
		return RollbackReceipt{}, fmt.Errorf("%w: expected definition revision %d, current %d", ErrRevisionConflict,
			request.ExpectedRevision, revision)
	}
	if state != "active" || !activeVersion.Valid || !previousStable.Valid || previousStable.Int64 != request.TargetStableVersion {
		return RollbackReceipt{}, fmt.Errorf("%w: rollback target is not the previous stable version", ErrStateConflict)
	}
	var kind, algorithm, thresholdJSON, statisticsJSON, quality, oldSHA string
	var sampleID sql.NullString
	var windowStart, windowEnd sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT baseline_kind,COALESCE(sample_snapshot_id::text,''),window_start,window_end,
		algorithm_version,threshold_spec::text,statistics::text,quality_status,snapshot_sha256
		FROM behavior_baseline_versions_v1 WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3
		AND lifecycle_state='retired' FOR UPDATE`, request.TenantID, request.BaselineID, request.TargetStableVersion).Scan(
		&kind, &sampleID, &windowStart, &windowEnd, &algorithm, &thresholdJSON, &statisticsJSON, &quality, &oldSHA)
	if err == sql.ErrNoRows {
		return RollbackReceipt{}, fmt.Errorf("%w: previous stable version is not retained", ErrStateConflict)
	}
	if err != nil {
		return RollbackReceipt{}, fmt.Errorf("lock behavior baseline rollback target: %w", err)
	}
	if quality != "complete" {
		return RollbackReceipt{}, fmt.Errorf("%w: rollback target quality is not complete", ErrStateConflict)
	}
	rollbackCanonical := map[string]interface{}{
		"algorithm": "behavior-baseline-rollback-version-v1", "tenant_id": request.TenantID,
		"baseline_id": request.BaselineID, "baseline_version": nextVersion,
		"rollback_of_version": request.TargetStableVersion, "source_snapshot_sha256": oldSHA,
		"candidate_sha256": request.CandidateSHA256,
	}
	rollbackSHA, err := canonicalSHA256(rollbackCanonical)
	if err != nil {
		return RollbackReceipt{}, err
	}
	versionID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_versions_v1 (
		version_id,tenant_id,baseline_id,baseline_version,baseline_kind,definition_revision,lifecycle_state,
		sample_snapshot_id,window_start,window_end,algorithm_version,threshold_spec,statistics,quality_status,
		snapshot_sha256,candidate_sha256,rollback_of_version,created_by,frozen_at
	) VALUES ($1,$2,$3,$4,$5,$6,'frozen',NULLIF($7,'')::uuid,$8,$9,$10,$11::jsonb,$12::jsonb,
		'complete',$13,$14,$15,$16,now())`, versionID, request.TenantID, request.BaselineID, nextVersion, kind,
		revision, sampleID.String, nullableTime(windowStart), nullableTime(windowEnd), algorithm, thresholdJSON,
		statisticsJSON, rollbackSHA, request.CandidateSHA256, request.TargetStableVersion, request.RequestedBy)
	if err != nil {
		return RollbackReceipt{}, fmt.Errorf("create immutable behavior baseline rollback version: %w", err)
	}
	update, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_definitions_v1 SET next_version=next_version+1,
		updated_by=$1,updated_at=now() WHERE tenant_id=$2 AND baseline_id=$3 AND revision=$4 AND next_version=$5`,
		request.RequestedBy, request.TenantID, request.BaselineID, revision, nextVersion)
	if err != nil {
		return RollbackReceipt{}, fmt.Errorf("reserve behavior baseline rollback version: %w", err)
	}
	if affected, _ := update.RowsAffected(); affected != 1 {
		return RollbackReceipt{}, fmt.Errorf("%w: rollback version reservation lost", ErrRevisionConflict)
	}
	eventID := uuid.NewString()
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.version.frozen.v1", "schema_version": 1,
		"tenant_id": request.TenantID, "baseline_id": request.BaselineID, "baseline_kind": kind,
		"baseline_version": nextVersion, "version_id": versionID, "quality_status": "complete",
		"rollback_of_version": request.TargetStableVersion, "snapshot_sha256": rollbackSHA,
		"candidate_sha256": request.CandidateSHA256, "trace_id": request.TraceID,
	}
	if err := appendOutboxTx(ctx, tx, request.TenantID, request.BaselineID, "baseline_version", versionID,
		nextVersion, "baseline.version.frozen.v1", eventID, request.TraceID, payload); err != nil {
		return RollbackReceipt{}, err
	}
	if err := appendHistoryTx(ctx, tx, request.TenantID, request.BaselineID, revision, &nextVersion, state, state,
		"baseline.rollback-version.created.v1", request.Reason, request.RequestedBy, request.TraceID,
		map[string]interface{}{"rollback_of_version": request.TargetStableVersion, "snapshot_sha256": rollbackSHA}); err != nil {
		return RollbackReceipt{}, err
	}
	receipt := RollbackReceipt{BaselineID: request.BaselineID, TargetStableVersion: request.TargetStableVersion,
		RollbackVersion: nextVersion, VersionID: versionID, SnapshotSHA256: rollbackSHA,
		LifecycleState: "frozen", EventID: eventID}
	responseJSON, _ := json.Marshal(receipt)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_command_receipts_v1 (
		tenant_id,idempotency_key,command_type,request_sha256,response_status,response_body
	) VALUES ($1,$2,'rollback.request',$3,202,$4::jsonb)`, request.TenantID, request.IdempotencyKey, requestSHA, string(responseJSON))
	if err != nil {
		return RollbackReceipt{}, fmt.Errorf("record behavior baseline rollback receipt: %w", err)
	}
	return receipt, nil
}

func lookupRollbackReceiptTx(ctx context.Context, tx *sql.Tx, tenantID, key, requestSHA string) (RollbackReceipt, bool, error) {
	var commandType, storedSHA, responseJSON string
	err := tx.QueryRowContext(ctx, `SELECT command_type,request_sha256,response_body::text
		FROM behavior_baseline_command_receipts_v1 WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		tenantID, key).Scan(&commandType, &storedSHA, &responseJSON)
	if err == sql.ErrNoRows {
		return RollbackReceipt{}, false, nil
	}
	if err != nil {
		return RollbackReceipt{}, false, fmt.Errorf("read behavior baseline rollback receipt: %w", err)
	}
	if commandType != "rollback.request" || storedSHA != requestSHA {
		return RollbackReceipt{}, false, fmt.Errorf("%w: rollback idempotency key changed bytes", ErrIdentityConflict)
	}
	var receipt RollbackReceipt
	if err := json.Unmarshal([]byte(responseJSON), &receipt); err != nil {
		return RollbackReceipt{}, false, fmt.Errorf("decode behavior baseline rollback receipt: %w", err)
	}
	receipt.Replayed = true
	return receipt, true, nil
}

func nullableTime(value sql.NullTime) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Time
}

package baseline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) RequestBuildTx(ctx context.Context, tx *sql.Tx, request BuildRequest) (BuildReceipt, error) {
	if tx == nil {
		return BuildReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return BuildReceipt{}, err
	}
	requestSHA, err := canonicalSHA256(map[string]interface{}{
		"tenant_id": request.TenantID, "baseline_id": request.BaselineID, "baseline_kind": request.BaselineKind,
		"entity_type": request.EntityType, "entity_id": request.EntityID, "expected_revision": request.ExpectedRevision,
		"window_start": request.WindowStart, "window_end": request.WindowEnd, "algorithm_version": request.AlgorithmVersion,
		"sample_policy": request.SamplePolicy, "threshold_spec": request.ThresholdSpec,
		"expected_consumers": request.ExpectedConsumers, "candidate_sha256": request.CandidateSHA256,
		"requested_by": request.RequestedBy, "reason": request.Reason,
	})
	if err != nil {
		return BuildReceipt{}, err
	}
	// 幂等键 advisory lock 先于查重：并发相同请求串行化，第二个事务
	// 查重时必然命中回执走精确重放，避免双插入/双执行竞态。
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		request.TenantID+"::"+request.IdempotencyKey); err != nil {
		return BuildReceipt{}, fmt.Errorf("lock baseline build idempotency key: %w", err)
	}
	if receipt, found, err := lookupBuildReceiptTx(ctx, tx, request.TenantID, request.IdempotencyKey, requestSHA); err != nil || found {
		return receipt, err
	}
	samplePolicyJSON, _ := json.Marshal(request.SamplePolicy)
	thresholdJSON, _ := json.Marshal(request.ThresholdSpec)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_definitions_v1 (
		tenant_id,baseline_id,baseline_kind,entity_type,entity_id,algorithm_version,sample_policy,
		threshold_spec,expected_consumers,created_by,updated_by
	) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9,$10,$10) ON CONFLICT (tenant_id,baseline_id) DO NOTHING`,
		request.TenantID, request.BaselineID, request.BaselineKind, request.EntityType, request.EntityID,
		request.AlgorithmVersion, string(samplePolicyJSON), string(thresholdJSON), pq.Array(request.ExpectedConsumers), request.RequestedBy)
	if err != nil {
		return BuildReceipt{}, fmt.Errorf("create behavior baseline definition: %w", err)
	}
	var kind, entityType, entityID, state, algorithm, storedPolicy, storedThreshold string
	var revision, targetVersion int64
	var consumers []string
	err = tx.QueryRowContext(ctx, `SELECT baseline_kind,entity_type,entity_id,lifecycle_state,revision,next_version,
		algorithm_version,sample_policy::text,threshold_spec::text,expected_consumers
		FROM behavior_baseline_definitions_v1 WHERE tenant_id=$1 AND baseline_id=$2 FOR UPDATE`,
		request.TenantID, request.BaselineID).Scan(&kind, &entityType, &entityID, &state, &revision, &targetVersion,
		&algorithm, &storedPolicy, &storedThreshold, pq.Array(&consumers))
	if err != nil {
		return BuildReceipt{}, fmt.Errorf("lock behavior baseline definition: %w", err)
	}
	if revision != request.ExpectedRevision {
		return BuildReceipt{}, fmt.Errorf("%w: expected definition revision %d, current %d", ErrRevisionConflict, request.ExpectedRevision, revision)
	}
	if state == "retired" {
		return BuildReceipt{}, fmt.Errorf("%w: retired definition cannot build", ErrStateConflict)
	}
	if kind != request.BaselineKind || entityType != request.EntityType || entityID != request.EntityID || algorithm != request.AlgorithmVersion ||
		!jsonEquivalent(storedPolicy, samplePolicyJSON) || !jsonEquivalent(storedThreshold, thresholdJSON) ||
		!sameStrings(consumers, request.ExpectedConsumers) {
		return BuildReceipt{}, fmt.Errorf("%w: definition fields changed without a revisioned definition update", ErrIdentityConflict)
	}
	result, err := tx.ExecContext(ctx, `UPDATE behavior_baseline_definitions_v1 SET next_version=next_version+1,
		updated_by=$1,updated_at=now() WHERE tenant_id=$2 AND baseline_id=$3 AND revision=$4 AND next_version=$5`,
		request.RequestedBy, request.TenantID, request.BaselineID, revision, targetVersion)
	if err != nil {
		return BuildReceipt{}, fmt.Errorf("reserve behavior baseline version: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return BuildReceipt{}, fmt.Errorf("%w: version reservation lost", ErrRevisionConflict)
	}
	jobID, eventID := uuid.NewString(), uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_build_jobs_v1 (
		job_id,tenant_id,baseline_id,baseline_kind,definition_revision,target_version,idempotency_key,request_sha256,
		candidate_sha256,requested_window_start,requested_window_end,status,requested_by,reason,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'queued',$12,$13,$14)`, jobID, request.TenantID,
		request.BaselineID, request.BaselineKind, revision, targetVersion, request.IdempotencyKey, requestSHA,
		request.CandidateSHA256, request.WindowStart, request.WindowEnd, request.RequestedBy, request.Reason, request.TraceID)
	if err != nil {
		return BuildReceipt{}, fmt.Errorf("create behavior baseline build job: %w", err)
	}
	payload := map[string]interface{}{
		"event_id": eventID, "event_type": "baseline.build.requested.v1", "schema_version": 1,
		"tenant_id": request.TenantID, "baseline_id": request.BaselineID, "baseline_kind": request.BaselineKind,
		"job_id": jobID, "definition_revision": revision, "target_version": targetVersion,
		"candidate_sha256": request.CandidateSHA256, "window_start": request.WindowStart, "window_end": request.WindowEnd,
		"partition_key": request.TenantID + ":" + request.BaselineID, "trace_id": request.TraceID,
	}
	if err := appendOutboxTx(ctx, tx, request.TenantID, request.BaselineID, "baseline_build_job", jobID,
		targetVersion, "baseline.build.requested.v1", eventID, request.TraceID, payload); err != nil {
		return BuildReceipt{}, err
	}
	if err := appendHistoryTx(ctx, tx, request.TenantID, request.BaselineID, revision, &targetVersion,
		state, state, "baseline.build.requested.v1", request.Reason, request.RequestedBy, request.TraceID,
		map[string]interface{}{"job_id": jobID, "candidate_sha256": request.CandidateSHA256}); err != nil {
		return BuildReceipt{}, err
	}
	receipt := BuildReceipt{JobID: jobID, BaselineID: request.BaselineID, BaselineKind: request.BaselineKind,
		DefinitionRevision: revision, TargetVersion: targetVersion, Status: "queued", EventID: eventID, OutboxStatus: "pending"}
	responseJSON, _ := json.Marshal(receipt)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_command_receipts_v1 (
		tenant_id,idempotency_key,command_type,request_sha256,response_status,response_body
	) VALUES ($1,$2,'build.request',$3,202,$4::jsonb)`, request.TenantID, request.IdempotencyKey, requestSHA, string(responseJSON))
	if err != nil {
		return BuildReceipt{}, fmt.Errorf("record behavior baseline build receipt: %w", err)
	}
	return receipt, nil
}

func lookupBuildReceiptTx(ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey, requestSHA string) (BuildReceipt, bool, error) {
	var storedSHA, commandType, responseJSON string
	err := tx.QueryRowContext(ctx, `SELECT command_type,request_sha256,response_body::text
		FROM behavior_baseline_command_receipts_v1 WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`,
		tenantID, idempotencyKey).Scan(&commandType, &storedSHA, &responseJSON)
	if err == sql.ErrNoRows {
		return BuildReceipt{}, false, nil
	}
	if err != nil {
		return BuildReceipt{}, false, fmt.Errorf("read behavior baseline command receipt: %w", err)
	}
	if commandType != "build.request" || storedSHA != requestSHA {
		return BuildReceipt{}, false, fmt.Errorf("%w: idempotency key reused with different command bytes", ErrIdentityConflict)
	}
	var receipt BuildReceipt
	if err := json.Unmarshal([]byte(responseJSON), &receipt); err != nil {
		return BuildReceipt{}, false, fmt.Errorf("decode behavior baseline build receipt: %w", err)
	}
	receipt.Replayed = true
	return receipt, true, nil
}

func appendOutboxTx(ctx context.Context, tx *sql.Tx, tenantID, baselineID, aggregateType, aggregateID string,
	aggregateVersion int64, eventType, eventID, traceID string, payload map[string]interface{}) error {
	if _, exists := payload["partition_key"]; !exists {
		payload["partition_key"] = tenantID + ":" + baselineID
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal behavior baseline outbox: %w", err)
	}
	hash := sha256.Sum256(payloadJSON)
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_lifecycle_outbox_v1 (
		event_id,tenant_id,baseline_id,aggregate_type,aggregate_id,aggregate_version,event_type,partition_key,
		payload,payload_sha256,publish_state,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,'PENDING',$11)`, eventID, tenantID, baselineID,
		aggregateType, aggregateID, aggregateVersion, eventType, tenantID+":"+baselineID, string(payloadJSON),
		hex.EncodeToString(hash[:]), traceID)
	if err != nil {
		return fmt.Errorf("append behavior baseline outbox: %w", err)
	}
	return nil
}

func appendHistoryTx(ctx context.Context, tx *sql.Tx, tenantID, baselineID string, revision int64,
	version *int64, fromState, toState, eventType, reason, actorID, traceID string, metadata map[string]interface{}) error {
	metadataJSON, _ := json.Marshal(metadata)
	_, err := tx.ExecContext(ctx, `INSERT INTO behavior_baseline_lifecycle_history_v1 (
		history_id,tenant_id,baseline_id,definition_revision,baseline_version,from_state,to_state,event_type,
		reason,actor_id,trace_id,metadata
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)`, uuid.NewString(), tenantID, baselineID,
		revision, version, nullableState(fromState), toState, eventType, reason, actorID, traceID, string(metadataJSON))
	if err != nil {
		return fmt.Errorf("append behavior baseline history: %w", err)
	}
	return nil
}

func canonicalSHA256(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal canonical behavior baseline value: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func jsonEquivalent(stored string, wanted []byte) bool {
	var left, right interface{}
	return json.Unmarshal([]byte(stored), &left) == nil && json.Unmarshal(wanted, &right) == nil && reflect.DeepEqual(left, right)
}

func sameStrings(left, right []string) bool {
	left, right = uniqueSorted(left), uniqueSorted(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func nullableState(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func nowUTC() time.Time { return time.Now().UTC() }

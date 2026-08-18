package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

func (repository *Repository) EvaluateTx(
	ctx context.Context,
	tx *sql.Tx,
	request EvaluationRequest,
) (EvaluationReceipt, error) {
	if tx == nil {
		return EvaluationReceipt{}, fmt.Errorf("%w: transaction is required", ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return EvaluationReceipt{}, err
	}
	var definitionState, kind, policyJSON string
	var activeVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT lifecycle_state,baseline_kind,active_version,sample_policy::text
		FROM behavior_baseline_definitions_v1 WHERE tenant_id=$1 AND baseline_id=$2`,
		request.TenantID, request.BaselineID).Scan(&definitionState, &kind, &activeVersion, &policyJSON)
	if err == sql.ErrNoRows {
		return EvaluationReceipt{}, fmt.Errorf("%w: behavior baseline definition does not exist", ErrStateConflict)
	}
	if err != nil {
		return EvaluationReceipt{}, fmt.Errorf("read active behavior baseline definition: %w", err)
	}
	if definitionState != "active" || !activeVersion.Valid {
		return repository.insertFailedEvaluationTx(ctx, tx, request, nil, "", "missing", "unavailable", "BASELINE_NOT_ACTIVE")
	}
	var versionState, qualityStatus, snapshotSHA, statisticsJSON, thresholdJSON string
	var windowStart, windowEnd sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT lifecycle_state,quality_status,snapshot_sha256,statistics::text,
		threshold_spec::text,window_start,window_end FROM behavior_baseline_versions_v1
		WHERE tenant_id=$1 AND baseline_id=$2 AND baseline_version=$3`,
		request.TenantID, request.BaselineID, activeVersion.Int64).Scan(&versionState, &qualityStatus,
		&snapshotSHA, &statisticsJSON, &thresholdJSON, &windowStart, &windowEnd)
	if err != nil {
		return EvaluationReceipt{}, fmt.Errorf("read active behavior baseline version: %w", err)
	}
	if versionState != "active" {
		return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
			"missing", "unavailable", "ACTIVE_VERSION_STATE_MISMATCH")
	}
	if qualityStatus != "complete" {
		return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
			"partial", "partial", "ACTIVE_BASELINE_PARTIAL")
	}
	if kind == "dynamic" {
		var policy map[string]interface{}
		if json.Unmarshal([]byte(policyJSON), &policy) != nil {
			return EvaluationReceipt{}, fmt.Errorf("decode active behavior baseline sample policy")
		}
		maxAgeSeconds, ok := numericValue(policy["max_active_age_seconds"])
		if !ok || maxAgeSeconds <= 0 || !windowEnd.Valid {
			return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
				"stale", "stale", "BASELINE_EXPIRY_UNDEFINED")
		}
		if request.ObservedAt.After(windowEnd.Time.Add(time.Duration(maxAgeSeconds * float64(time.Second)))) {
			return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
				"stale", "stale", "BASELINE_EXPIRED")
		}
	}
	var statistics, thresholds map[string]interface{}
	if json.Unmarshal([]byte(statisticsJSON), &statistics) != nil || json.Unmarshal([]byte(thresholdJSON), &thresholds) != nil {
		return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
			"failed", "failed", "BASELINE_SNAPSHOT_INVALID")
	}
	metric, ok := statistics[request.MetricName].(map[string]interface{})
	if !ok {
		return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
			"failed", "failed", "METRIC_NOT_VERSIONED")
	}
	mean, meanOK := numericValue(metric["mean"])
	stddev, stddevOK := numericValue(metric["stddev"])
	warning, warningOK := numericValue(firstNonNil(thresholds["warning_multiplier"], thresholds["warning"]))
	alert, alertOK := numericValue(firstNonNil(thresholds["alert_multiplier"], thresholds["alert"]))
	if !meanOK || !stddevOK || !warningOK || !alertOK || warning <= 0 || alert <= warning {
		return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
			"failed", "failed", "METRIC_THRESHOLD_INVALID")
	}
	if stddev <= 0 && request.ObservedValue != mean {
		return repository.insertFailedEvaluationTx(ctx, tx, request, &activeVersion.Int64, snapshotSHA,
			"failed", "failed", "ZERO_VARIANCE")
	}
	deviation := 0.0
	if stddev > 0 {
		deviation = math.Abs(request.ObservedValue-mean) / stddev
	}
	disposition := "normal"
	if deviation >= alert {
		disposition = "alert"
	} else if deviation >= warning {
		disposition = "warning"
	}
	receipt := EvaluationReceipt{
		EvaluationID: uuid.NewString(), BaselineID: request.BaselineID, BaselineVersion: activeVersion.Int64,
		SnapshotSHA256: snapshotSHA, MetricName: request.MetricName, ObservedValue: request.ObservedValue,
		MeanValue: &mean, StdDevValue: &stddev, DeviationScore: &deviation,
		WarningThreshold: &warning, AlertThreshold: &alert, Disposition: disposition,
		QualityStatus: "complete", EvidenceRefs: request.EvidenceRefs,
	}
	if err := insertEvaluationTx(ctx, tx, request, receipt, nullableTime(windowStart), nullableTime(windowEnd)); err != nil {
		return EvaluationReceipt{}, err
	}
	return receipt, nil
}

func (repository *Repository) insertFailedEvaluationTx(
	ctx context.Context,
	tx *sql.Tx,
	request EvaluationRequest,
	version *int64,
	snapshotSHA, disposition, quality, failureCode string,
) (EvaluationReceipt, error) {
	receipt := EvaluationReceipt{EvaluationID: uuid.NewString(), BaselineID: request.BaselineID,
		SnapshotSHA256: snapshotSHA, MetricName: request.MetricName, ObservedValue: request.ObservedValue,
		Disposition: disposition, QualityStatus: quality, FailureCode: failureCode, EvidenceRefs: request.EvidenceRefs}
	if version != nil {
		receipt.BaselineVersion = *version
	}
	if err := insertEvaluationTx(ctx, tx, request, receipt, request.WindowStart, request.WindowEnd); err != nil {
		return EvaluationReceipt{}, err
	}
	return receipt, nil
}

func insertEvaluationTx(
	ctx context.Context,
	tx *sql.Tx,
	request EvaluationRequest,
	receipt EvaluationReceipt,
	windowStart, windowEnd interface{},
) error {
	evidenceJSON, _ := json.Marshal(receipt.EvidenceRefs)
	provenanceJSON, _ := json.Marshal(map[string]interface{}{
		"algorithm": "behavior-baseline-zscore-v1", "baseline_version": receipt.BaselineVersion,
		"snapshot_sha256": receipt.SnapshotSHA256, "fail_visible": receipt.FailureCode != "",
	})
	var version interface{}
	if receipt.BaselineVersion > 0 {
		version = receipt.BaselineVersion
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO behavior_baseline_drift_evaluations_v1 (
		evaluation_id,tenant_id,baseline_id,baseline_version,snapshot_sha256,metric_name,observed_value,
		observed_at,window_start,window_end,mean_value,stddev_value,deviation_score,warning_threshold,
		alert_threshold,disposition,quality_status,failure_code,evidence_refs,provenance,trace_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::jsonb,$20::jsonb,$21)`,
		receipt.EvaluationID, request.TenantID, request.BaselineID, version, receipt.SnapshotSHA256,
		request.MetricName, request.ObservedValue, request.ObservedAt, windowStart, windowEnd,
		receipt.MeanValue, receipt.StdDevValue, receipt.DeviationScore, receipt.WarningThreshold,
		receipt.AlertThreshold, receipt.Disposition, receipt.QualityStatus, receipt.FailureCode,
		string(evidenceJSON), string(provenanceJSON), request.TraceID)
	if err != nil {
		return fmt.Errorf("insert behavior baseline inference receipt: %w", err)
	}
	return nil
}

func firstNonNil(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

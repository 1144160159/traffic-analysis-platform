package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/modeldrift"
)

const (
	driftCandidateWorkflowPending   = "PENDING_WORKFLOW"
	driftCandidateWorkflowSubmitted = "WORKFLOW_SUBMITTED"
	driftCandidateWorkflowFailed    = "WORKFLOW_FAILED"
	driftFeatureBucketCount         = 18
)

var driftEvaluationNamespace = uuid.MustParse("26f294a6-a8aa-5e25-a012-83eb9ec99ac3")

type driftCandidateRecord struct {
	CandidateID  string
	EvaluationID string
	WorkflowName string
	State        string
	PolicySHA256 string
	SignalSHA256 string
	Reasons      []string
}

func (o *MLOpsOrchestrator) driftPolicy() modeldrift.Policy {
	policy := modeldrift.DefaultPolicy()
	policy.MaxPSI = o.config.MaxPSI
	policy.MaxFPRate = o.config.MaxFPRate
	policy.MaxPartialRate = o.config.MaxFeaturePartialRate
	if o.config.MinFeatureSamples > 0 {
		policy.MinFeatureSamples = uint64(o.config.MinFeatureSamples)
	}
	if o.config.MinFeedbackSamples > 0 {
		policy.MinFeedbackSamples = uint64(o.config.MinFeedbackSamples)
	}
	policy.MaxFeatureSignalAge = o.config.MaxFeatureSignalAge
	policy.MaxFeedbackSignalAge = o.config.MaxFeedbackSignalAge
	return policy
}

func (o *MLOpsOrchestrator) evaluateAndRecordGovernedDriftCandidate(ctx context.Context, scope *automatedMLOpsScope) (*RetrainDecision, *driftCandidateRecord, error) {
	decision, evaluated, err := o.evaluateGovernedDriftCandidate(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	candidate, err := o.recordGovernedDriftDecision(ctx, scope, evaluated.snapshot, evaluated.decision)
	if err != nil {
		return nil, nil, err
	}
	if candidate != nil {
		decision.EvaluationID = candidate.EvaluationID
		decision.PolicySHA256 = candidate.PolicySHA256
		decision.SignalSHA256 = candidate.SignalSHA256
		decision.Reason = strings.Join(candidate.Reasons, ",")
		if len(candidate.Reasons) == 1 && candidate.Reasons[0] == "false_positive_rate_threshold_exceeded" {
			decision.Trigger = TriggerFPRate
		} else {
			decision.Trigger = TriggerDrift
		}
	} else {
		decision.EvaluationID = evaluated.evaluationID
	}
	return decision, candidate, nil
}

type governedDriftEvaluation struct {
	evaluationID string
	snapshot     modeldrift.Snapshot
	decision     modeldrift.Decision
}

func (o *MLOpsOrchestrator) evaluateGovernedDriftCandidate(ctx context.Context, scope *automatedMLOpsScope) (*RetrainDecision, *governedDriftEvaluation, error) {
	if o.chDB == nil || o.pgDB == nil {
		return nil, nil, errors.New(errors.ErrCodeDatabaseError, "governed drift evaluation requires ClickHouse and PostgreSQL")
	}
	if scope == nil || strings.TrimSpace(scope.TenantID) == "" || strings.TrimSpace(scope.ModelID) == "" || strings.TrimSpace(scope.ModelVersion) == "" || strings.TrimSpace(scope.FeatureSetID) == "" {
		return nil, nil, errors.New(errors.ErrCodeInvalidParameter, "governed drift scope is incomplete")
	}
	if o.config.MinFeatureSamples <= 0 || o.config.MinFeedbackSamples <= 0 {
		return nil, nil, errors.New(errors.ErrCodeInvalidParameter, "governed drift sample thresholds must be positive")
	}
	snapshot, err := o.loadGovernedDriftSnapshot(ctx, scope, time.Now().UTC().Truncate(time.Hour))
	if err != nil {
		return nil, nil, err
	}
	evaluated, err := modeldrift.Evaluate(o.driftPolicy(), snapshot)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrCodeInvalidParameter, "invalid governed drift signal snapshot")
	}
	evaluationID := uuid.NewSHA1(driftEvaluationNamespace, []byte(evaluated.PolicySHA256+":"+evaluated.SignalSHA256)).String()
	decision := projectGovernedDriftDecision(scope, snapshot, evaluated)
	decision.EvaluationID = evaluationID
	return decision, &governedDriftEvaluation{evaluationID: evaluationID, snapshot: snapshot, decision: evaluated}, nil
}

func projectGovernedDriftDecision(scope *automatedMLOpsScope, snapshot modeldrift.Snapshot, evaluated modeldrift.Decision) *RetrainDecision {
	metrics := map[string]interface{}{
		"decision_state":        evaluated.State,
		"decision_reasons":      evaluated.Reasons,
		"psi":                   evaluated.PSI,
		"max_observed_psi":      evaluated.MaxObservedPSI,
		"false_positive_rate":   evaluated.FalsePositiveRate,
		"current_feature_count": snapshot.CurrentFeatureCount,
		"partial_feature_count": snapshot.PartialFeatureCount,
		"feedback_count":        snapshot.FeedbackCount,
		"false_positive_count":  snapshot.FalsePositiveCount,
		"feature_watermark":     snapshot.FeatureWatermark,
		"feedback_watermark":    snapshot.FeedbackWatermark,
		"activation_authorized": false,
		"candidate_only":        true,
	}
	trigger := TriggerDrift
	if len(evaluated.Reasons) == 1 && evaluated.Reasons[0] == "false_positive_rate_threshold_exceeded" {
		trigger = TriggerFPRate
	}
	return &RetrainDecision{
		ShouldRetrain: evaluated.State == modeldrift.DecisionCandidate,
		Trigger:       trigger,
		Reason:        strings.Join(evaluated.Reasons, ","), Metrics: metrics,
		TenantID: scope.TenantID, ModelID: scope.ModelID, ModelVersion: scope.ModelVersion, FeatureSetID: scope.FeatureSetID,
		Disposition: string(evaluated.State), SignalSHA256: evaluated.SignalSHA256, PolicySHA256: evaluated.PolicySHA256,
	}
}

func (o *MLOpsOrchestrator) loadGovernedDriftSnapshot(ctx context.Context, scope *automatedMLOpsScope, evaluatedAt time.Time) (modeldrift.Snapshot, error) {
	if o.config.DriftCurrentHours <= 0 || o.config.DriftBaselineHours <= 0 {
		return modeldrift.Snapshot{}, errors.New(errors.ErrCodeInvalidParameter, "drift current and baseline windows must be positive")
	}
	currentEnd := evaluatedAt
	currentStart := currentEnd.Add(-time.Duration(o.config.DriftCurrentHours) * time.Hour)
	baselineEnd := currentStart
	baselineStart := baselineEnd.Add(-time.Duration(o.config.DriftBaselineHours) * time.Hour)
	snapshot := modeldrift.Snapshot{
		TenantID: scope.TenantID, ModelID: scope.ModelID, ModelVersion: scope.ModelVersion, FeatureSetID: scope.FeatureSetID,
		EvaluatedAt: evaluatedAt, BaselineWindowStart: baselineStart, BaselineWindowEnd: baselineEnd,
		CurrentWindowStart: currentStart, CurrentWindowEnd: currentEnd,
		FeatureDistributions: make(map[string]modeldrift.Distribution, len(modeldrift.RequiredFeatures)),
	}

	featureColumns := map[string]string{"bps": "bps", "iat_mean_ms": "iat_mean_ms", "pktlen_mean": "pktlen_mean", "pps": "pps"}
	for _, feature := range modeldrift.RequiredFeatures {
		column := featureColumns[feature]
		query := fmt.Sprintf(`
			SELECT if(ts >= ?, 'current', 'baseline') AS signal_window,
			       toUInt8(least(%d, floor(log2(1 + greatest(toFloat64(%s), 0))))) AS bucket,
			       count() AS samples
			FROM traffic.feature_stat
			WHERE tenant_id = ? AND feature_set_id = ? AND ts >= ? AND ts < ?
			  AND isFinite(toFloat64(%s))
			GROUP BY signal_window, bucket
			ORDER BY signal_window, bucket`, driftFeatureBucketCount-1, column, column)
		rows, err := o.chDB.QueryContext(ctx, query, currentStart, scope.TenantID, scope.FeatureSetID, baselineStart, currentEnd)
		if err != nil {
			return modeldrift.Snapshot{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query governed feature distribution")
		}
		distribution := modeldrift.Distribution{Baseline: make([]uint64, driftFeatureBucketCount), Current: make([]uint64, driftFeatureBucketCount)}
		for rows.Next() {
			var signalWindow string
			var bucket uint8
			var samples uint64
			if err := rows.Scan(&signalWindow, &bucket, &samples); err != nil {
				_ = rows.Close()
				return modeldrift.Snapshot{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan governed feature distribution")
			}
			if int(bucket) >= driftFeatureBucketCount || (signalWindow != "baseline" && signalWindow != "current") {
				_ = rows.Close()
				return modeldrift.Snapshot{}, errors.New(errors.ErrCodeDatabaseError, "governed feature distribution returned an invalid bucket")
			}
			if signalWindow == "baseline" {
				distribution.Baseline[bucket] += samples
			} else {
				distribution.Current[bucket] += samples
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return modeldrift.Snapshot{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate governed feature distribution")
		}
		_ = rows.Close()
		snapshot.FeatureDistributions[feature] = distribution
	}

	var featureWatermarkMS sql.NullInt64
	qualityQuery := `
		SELECT countIf(ts >= ?) AS current_samples,
		       countIf(ts >= ? AND (is_partial = 1 OR notEmpty(missing_fields)
		         OR availability IN ('FEATURE_AVAILABILITY_UNSPECIFIED','FEATURE_AVAILABILITY_MISSING'))) AS partial_samples,
		       maxOrNull(toUnixTimestamp64Milli(ts)) AS feature_watermark_ms
		FROM traffic.feature_stat
		WHERE tenant_id = ? AND feature_set_id = ? AND ts >= ? AND ts < ?`
	if err := o.chDB.QueryRowContext(ctx, qualityQuery, currentStart, currentStart, scope.TenantID, scope.FeatureSetID, baselineStart, currentEnd).Scan(&snapshot.CurrentFeatureCount, &snapshot.PartialFeatureCount, &featureWatermarkMS); err != nil {
		return modeldrift.Snapshot{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query governed feature quality")
	}
	if featureWatermarkMS.Valid {
		snapshot.FeatureWatermark = time.UnixMilli(featureWatermarkMS.Int64).UTC()
	}

	var feedbackWatermarkMS int64
	feedbackQuery := `
		SELECT count(*), count(*) FILTER (WHERE label = 'FP'), COALESCE(max(occurred_at_ms),0)
		FROM model_feedback_revision_head
		WHERE tenant_id = $1 AND model_version = $2 AND adjudication_state = 'ADJUDICATED'
		  AND occurred_at_ms >= $3 AND occurred_at_ms < $4`
	if err := o.pgDB.QueryRowContext(ctx, feedbackQuery, scope.TenantID, scope.ModelVersion, currentStart.UnixMilli(), currentEnd.UnixMilli()).Scan(&snapshot.FeedbackCount, &snapshot.FalsePositiveCount, &feedbackWatermarkMS); err != nil {
		return modeldrift.Snapshot{}, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query governed feedback quality")
	}
	if feedbackWatermarkMS > 0 {
		snapshot.FeedbackWatermark = time.UnixMilli(feedbackWatermarkMS).UTC()
	}
	return snapshot, nil
}

func (o *MLOpsOrchestrator) recordGovernedDriftDecision(ctx context.Context, scope *automatedMLOpsScope, snapshot modeldrift.Snapshot, decision modeldrift.Decision) (*driftCandidateRecord, error) {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal drift snapshot")
	}
	psiJSON, err := json.Marshal(decision.PSI)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal drift PSI")
	}
	reasonsJSON, err := json.Marshal(decision.Reasons)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal drift reasons")
	}
	evaluationID := uuid.NewSHA1(driftEvaluationNamespace, []byte(decision.PolicySHA256+":"+decision.SignalSHA256)).String()
	tx, err := o.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin drift decision transaction")
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO model_drift_evaluation_receipt (
		  evaluation_id,tenant_id,model_id,model_version,feature_set_id,
		  policy_sha256,signal_sha256,decision_state,reasons,psi,max_observed_psi,false_positive_rate,
		  signal_snapshot,feature_watermark,feedback_watermark,baseline_window_start,baseline_window_end,
		  current_window_start,current_window_end,evaluated_at,activation_authorized
		) VALUES ($1,$2,$3::uuid,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11,$12,$13::jsonb,$14,$15,$16,$17,$18,$19,$20,false)
		ON CONFLICT (policy_sha256,signal_sha256) DO NOTHING`,
		evaluationID, scope.TenantID, scope.ModelID, scope.ModelVersion, scope.FeatureSetID,
		decision.PolicySHA256, decision.SignalSHA256, decision.State, reasonsJSON, psiJSON,
		decision.MaxObservedPSI, decision.FalsePositiveRate, snapshotJSON, nullableTime(snapshot.FeatureWatermark), nullableTime(snapshot.FeedbackWatermark),
		snapshot.BaselineWindowStart, snapshot.BaselineWindowEnd, snapshot.CurrentWindowStart, snapshot.CurrentWindowEnd, snapshot.EvaluatedAt)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist drift evaluation receipt")
	}
	var storedEvaluationID, storedState, storedTenant, storedModel, storedVersion, storedFeatureSet string
	if err := tx.QueryRowContext(ctx, `
		SELECT evaluation_id::text,decision_state,tenant_id,model_id::text,model_version,feature_set_id
		FROM model_drift_evaluation_receipt WHERE policy_sha256=$1 AND signal_sha256=$2`, decision.PolicySHA256, decision.SignalSHA256).
		Scan(&storedEvaluationID, &storedState, &storedTenant, &storedModel, &storedVersion, &storedFeatureSet); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to verify drift evaluation replay")
	}
	if storedEvaluationID != evaluationID || storedState != string(decision.State) || storedTenant != scope.TenantID || storedModel != scope.ModelID || storedVersion != scope.ModelVersion || storedFeatureSet != scope.FeatureSetID {
		return nil, errors.New(errors.ErrCodeVersionConflict, "drift evaluation replay identity mismatch")
	}

	var candidate *driftCandidateRecord
	if decision.State == modeldrift.DecisionCandidate {
		candidateID := uuid.NewSHA1(driftEvaluationNamespace, []byte("candidate:"+evaluationID)).String()
		workflowName := "mlops-drift-" + strings.ReplaceAll(candidateID, "-", "")[:20]
		_, err = tx.ExecContext(ctx, `
			INSERT INTO model_retrain_candidate_request (
			  candidate_id,evaluation_id,tenant_id,model_id,baseline_model_version,feature_set_id,
			  workflow_name,candidate_state,argo_namespace,workflow_template,approval_state,activation_authorized
			) VALUES ($1,$2,$3,$4::uuid,$5,$6,$7,$8,$9,$10,'NOT_REQUESTED',false)
			ON CONFLICT DO NOTHING`, candidateID, evaluationID, scope.TenantID, scope.ModelID,
			scope.ModelVersion, scope.FeatureSetID, workflowName, driftCandidateWorkflowPending, o.config.ArgoNamespace, o.config.WorkflowTemplate)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist retrain candidate request")
		}
		candidate = &driftCandidateRecord{}
		var candidateReasonsJSON []byte
		if err := tx.QueryRowContext(ctx, `
			SELECT candidate.candidate_id::text,candidate.evaluation_id::text,
			       candidate.workflow_name,candidate.candidate_state,
			       evaluation.policy_sha256,evaluation.signal_sha256,evaluation.reasons
			FROM model_retrain_candidate_request candidate
			JOIN model_drift_evaluation_receipt evaluation ON evaluation.evaluation_id=candidate.evaluation_id
			WHERE candidate.tenant_id=$1 AND candidate.model_id=$2::uuid
			  AND candidate.baseline_model_version=$3`, scope.TenantID, scope.ModelID, scope.ModelVersion).
			Scan(&candidate.CandidateID, &candidate.EvaluationID, &candidate.WorkflowName, &candidate.State,
				&candidate.PolicySHA256, &candidate.SignalSHA256, &candidateReasonsJSON); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to verify retrain candidate replay")
		}
		if err := json.Unmarshal(candidateReasonsJSON, &candidate.Reasons); err != nil || len(candidate.Reasons) == 0 {
			return nil, errors.New(errors.ErrCodeSerializationError, "stored retrain candidate reasons are invalid")
		}
		expectedCandidateID := uuid.NewSHA1(driftEvaluationNamespace, []byte("candidate:"+candidate.EvaluationID)).String()
		expectedWorkflowName := "mlops-drift-" + strings.ReplaceAll(expectedCandidateID, "-", "")[:20]
		if candidate.CandidateID != expectedCandidateID || candidate.WorkflowName != expectedWorkflowName {
			return nil, errors.New(errors.ErrCodeVersionConflict, "retrain candidate replay identity mismatch")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit drift decision")
	}
	return candidate, nil
}

func nullableTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value
}

func (o *MLOpsOrchestrator) markDriftCandidateWorkflow(ctx context.Context, candidateID, state, failure string) error {
	if state != driftCandidateWorkflowSubmitted && state != driftCandidateWorkflowFailed {
		return errors.New(errors.ErrCodeInvalidParameter, "invalid drift candidate workflow state")
	}
	result, err := o.pgDB.ExecContext(ctx, `
		UPDATE model_retrain_candidate_request
		SET candidate_state=$2, dispatch_attempts=dispatch_attempts+1,
		    last_error=$3, workflow_submitted_at=CASE WHEN $2='WORKFLOW_SUBMITTED' THEN now() ELSE workflow_submitted_at END,
		    updated_at=now()
		WHERE candidate_id=$1 AND candidate_state<>'WORKFLOW_SUBMITTED'`, candidateID, state, failure)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update drift candidate workflow state")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to inspect drift candidate workflow update")
	}
	if affected == 0 {
		var existing string
		if err := o.pgDB.QueryRowContext(ctx, `SELECT candidate_state FROM model_retrain_candidate_request WHERE candidate_id=$1`, candidateID).Scan(&existing); err != nil || existing != driftCandidateWorkflowSubmitted {
			return errors.New(errors.ErrCodeInvalidStateTransition, "drift candidate workflow state update was not applied")
		}
	}
	return nil
}

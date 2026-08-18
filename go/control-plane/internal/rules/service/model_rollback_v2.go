package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
)

const modelRollbackGovernanceVersion = "model-rollback.v2"

const (
	modelRollbackPhaseAttempt      = "attempt"
	modelRollbackPhaseCompensation = "compensation"
)

// ModelRollbackReceipt is the query-side contract for one rollback.  The
// active pointer is authoritative only when State=RECOVERED and
// ActiveSwitched=true.  PARTIAL and compensation states intentionally expose
// that serving consumers may not yet agree.
type ModelRollbackReceipt struct {
	RollbackID                  string     `json:"rollback_id"`
	ActionJobID                 string     `json:"action_job_id"`
	RollbackEventID             string     `json:"rollback_event_id"`
	CompensationEventID         string     `json:"compensation_event_id,omitempty"`
	GovernanceVersion           string     `json:"governance_version"`
	TenantID                    string     `json:"tenant_id"`
	ModelID                     string     `json:"model_id"`
	FromModelVersion            string     `json:"from_model_version"`
	FromRevision                int64      `json:"from_revision"`
	TargetModelVersion          string     `json:"target_model_version"`
	TargetRevision              int64      `json:"target_revision"`
	ConsumerDeploymentID        string     `json:"consumer_deployment_id"`
	ConsumerProfileSHA256       string     `json:"consumer_profile_sha256"`
	ExpectedParallelism         int        `json:"expected_parallelism"`
	AppliedSubtasks             int        `json:"applied_subtasks"`
	CompensationAppliedSubtasks int        `json:"compensation_applied_subtasks"`
	State                       string     `json:"state"`
	ActiveSwitched              bool       `json:"active_switched"`
	BrokerPublished             bool       `json:"broker_published"`
	CompensationBrokerPublished bool       `json:"compensation_broker_published"`
	RequestSHA256               string     `json:"request_sha256"`
	Reason                      string     `json:"reason"`
	FailureReason               string     `json:"failure_reason,omitempty"`
	CreatedAt                   time.Time  `json:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at"`
	RecoveredAt                 *time.Time `json:"recovered_at,omitempty"`
}

type modelRollbackRequestHashInput struct {
	GovernanceVersion     string `json:"governance_version"`
	TenantID              string `json:"tenant_id"`
	ModelID               string `json:"model_id"`
	FromModelVersion      string `json:"from_model_version"`
	FromRevision          int64  `json:"from_revision"`
	FromPackageSHA256     string `json:"from_package_sha256"`
	TargetModelVersion    string `json:"target_model_version"`
	TargetRevision        int64  `json:"target_revision"`
	TargetPackageSHA256   string `json:"target_package_sha256"`
	ConsumerDeploymentID  string `json:"consumer_deployment_id"`
	ConsumerProfileSHA256 string `json:"consumer_profile_sha256"`
	ExpectedParallelism   int    `json:"expected_parallelism"`
	Reason                string `json:"reason"`
	RequestedBy           string `json:"requested_by"`
	ActionID              string `json:"action_id"`
}

func hashModelRollbackRequest(value modelRollbackRequestHashInput) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:]), nil
}

// prepareModelRollbackV2 writes a rollback deployment and outbox event while
// deliberately leaving model_versions.status unchanged.  No code path in this
// function can move the active pointer.
func (s *ModelService) prepareModelRollbackV2(
	ctx context.Context,
	expectedModelID string,
	targetVersion string,
	opCtx *OperationContext,
	job *model.ModelActionJob,
) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.PrepareModelRollbackV2")
	defer span.End()

	if !s.config.EnableModelRollbackV2 {
		return errors.New(errors.ErrCodeServiceUnavailable, "governed model rollback v2 is disabled")
	}
	if !s.config.EnableKafkaNotification || s.publisher == nil {
		return errors.New(errors.ErrCodeServiceUnavailable, "governed model rollback requires the durable Kafka publisher")
	}
	if job == nil {
		return errors.New(errors.ErrCodeInvalidRequest, "governed model rollback must be submitted as a durable action job")
	}
	if opCtx == nil || !opCtx.Authenticated {
		return errors.New(errors.ErrCodeUnauthorized, "authenticated model rollback is required")
	}
	expectedActive := strings.TrimSpace(stringPayload(job.Payload, "expected_active_version"))
	expectedActiveRevision, ok := int64Payload(job.Payload, "expected_active_revision")
	if !ok || expectedActiveRevision <= 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "expected_active_revision must be a positive integer")
	}
	reason := strings.TrimSpace(stringPayload(job.Payload, "reason"))

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start governed model rollback transaction")
	}
	defer tx.Rollback()
	var advisoryLock interface{}
	if err := tx.QueryRowContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2,0))`,
		job.TenantID, job.ModelID,
	).Scan(&advisoryLock); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to serialize model rollback ownership")
	}

	target, err := s.getModelVersionForUpdate(ctx, tx, strings.TrimSpace(targetVersion))
	if err != nil {
		return err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelActivate, target.TenantID); err != nil {
		return err
	}
	if strings.TrimSpace(expectedModelID) == "" || target.ModelID != strings.TrimSpace(expectedModelID) {
		return errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found under model: %s", expectedModelID)
	}
	if target.TenantID != job.TenantID || target.ModelID != job.ModelID {
		return errors.New(errors.ErrCodePermissionDenied, "rollback target is outside the durable action scope")
	}
	if model.ModelStatus(target.Status) != model.ModelStatusDeprecated {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "rollback target must be deprecated, got: %s", target.Status)
	}
	if err := s.validateGovernedShadowMetadata(target); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidStateTransition, "rollback target immutable package is not deployable")
	}
	if _, err := governedModelArtifactSHA256(target); err != nil {
		return err
	}
	if err := s.requireExactConsumerReadyReceiptTx(ctx, tx); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidStateTransition, "model rollback consumer compatibility gate failed")
	}

	var currentVersion string
	var currentRevision int64
	var currentCreatedAt time.Time
	if err := tx.QueryRowContext(ctx, `
		SELECT model_version,revision,created_at
		FROM model_versions
		WHERE tenant_id=$1 AND model_id=$2::uuid AND status='active'
		FOR UPDATE
	`, target.TenantID, target.ModelID).Scan(
		&currentVersion, &currentRevision, &currentCreatedAt,
	); err == sql.ErrNoRows {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model rollback requires one currently active version")
	} else if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock the active model version")
	}
	if currentVersion != expectedActive || currentRevision != expectedActiveRevision {
		return errors.Newf(errors.ErrCodeVersionConflict,
			"active model changed: expected %s revision %d, current %s revision %d",
			expectedActive, expectedActiveRevision, currentVersion, currentRevision)
	}
	if currentVersion == target.ModelVersion {
		return errors.New(errors.ErrCodeInvalidStateTransition, "rollback target is already active")
	}

	current, err := s.getModelVersionForUpdate(ctx, tx, currentVersion)
	if err != nil {
		return err
	}
	if err := s.validateGovernedShadowMetadata(current); err != nil {
		return errors.Wrap(err, errors.ErrCodeInvalidStateTransition, "current model cannot be used as rollback compensation")
	}
	if _, err := governedModelArtifactSHA256(current); err != nil {
		return err
	}

	// "Previous" is a deterministic immutable lineage rule, not whichever
	// deprecated row the caller names.  The immediately preceding registered
	// package by creation order is the only eligible target.
	var previousVersion string
	if err := tx.QueryRowContext(ctx, `
		SELECT model_version
		FROM model_versions
		WHERE tenant_id=$1 AND model_id=$2::uuid AND status='deprecated'
		  AND created_at < $3
		ORDER BY created_at DESC,model_version DESC
		LIMIT 1
		FOR SHARE
	`, target.TenantID, target.ModelID, currentCreatedAt).Scan(&previousVersion); err == sql.ErrNoRows {
		return errors.New(errors.ErrCodeInvalidStateTransition, "no previous immutable model version is available")
	} else if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to resolve previous immutable model version")
	}
	if previousVersion != target.ModelVersion {
		return errors.Newf(errors.ErrCodeInvalidStateTransition,
			"rollback target %s is not the previous immutable version %s", target.ModelVersion, previousVersion)
	}

	hashInput := modelRollbackRequestHashInput{
		GovernanceVersion: modelRollbackGovernanceVersion,
		TenantID:          target.TenantID, ModelID: target.ModelID,
		FromModelVersion: current.ModelVersion, FromRevision: current.Revision,
		FromPackageSHA256:  current.PackageSHA256,
		TargetModelVersion: target.ModelVersion, TargetRevision: target.Revision,
		TargetPackageSHA256:   target.PackageSHA256,
		ConsumerDeploymentID:  s.config.ModelConsumerDeploymentID,
		ConsumerProfileSHA256: s.config.ModelConsumerProfileSHA256,
		ExpectedParallelism:   s.config.AppliedAckExpectedParallelism,
		Reason:                reason, RequestedBy: job.RequestedBy, ActionID: job.ActionID,
	}
	requestSHA, err := hashModelRollbackRequest(hashInput)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to hash model rollback request")
	}
	rollbackID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(modelRollbackGovernanceVersion+":"+job.ActionID)).String()
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("model-rollback-attempt.v2:"+rollbackID)).String()
	event, err := s.buildModelRollbackEvent(target, rollbackID, modelRollbackPhaseAttempt,
		current.ModelVersion, current.Revision, eventID)
	if err != nil {
		return err
	}
	if err := insertModelRollbackOutboxTx(ctx, tx, event, job.JobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_rollback_requests (
			rollback_id,action_job_id,rollback_event_id,tenant_id,model_id,
			from_model_version,from_revision,from_package_sha256,
			target_model_version,target_revision,target_package_sha256,
			consumer_deployment_id,consumer_profile_sha256,expected_parallelism,
			request_sha256,reason,requested_by,state
		) VALUES (
			$1::uuid,$2,$3,$4,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'PENDING_ACK'
		)
	`, rollbackID, job.JobID, eventID, target.TenantID, target.ModelID,
		current.ModelVersion, current.Revision, current.PackageSHA256,
		target.ModelVersion, target.Revision, target.PackageSHA256,
		s.config.ModelConsumerDeploymentID, s.config.ModelConsumerProfileSHA256,
		s.config.AppliedAckExpectedParallelism, requestSHA, reason, job.RequestedBy); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist governed model rollback receipt")
	}
	detail := map[string]interface{}{
		"rollback_id": rollbackID, "event_id": eventID, "job_id": job.JobID,
		"from_model_version": current.ModelVersion, "from_revision": current.Revision,
		"target_model_version": target.ModelVersion, "target_revision": target.Revision,
		"expected_parallelism":    s.config.AppliedAckExpectedParallelism,
		"consumer_deployment_id":  s.config.ModelConsumerDeploymentID,
		"consumer_profile_sha256": s.config.ModelConsumerProfileSHA256,
		"request_sha256":          requestSHA, "reason": reason,
		"active_pointer_changed": false,
	}
	if err := s.recordAuditLogTx(ctx, tx, opCtx, audit.EventType("MODEL_VERSION_ROLLBACK_PREPARED"), "model", target.ModelID, detail); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit governed model rollback preparation")
	}
	s.recordAuditStreamSuccess(ctx, opCtx, audit.EventType("MODEL_VERSION_ROLLBACK_PREPARED"), "model", target.ModelID, detail)
	return nil
}

func governedModelArtifactSHA256(mv *model.ModelVersion) (string, error) {
	value, _ := mv.Metrics["artifact_sha256"].(string)
	value = strings.TrimSpace(value)
	if !consumerSHA256Pattern.MatchString(value) {
		return "", errors.New(errors.ErrCodeInvalidStateTransition, "model immutable package is missing lowercase metrics.artifact_sha256")
	}
	return value, nil
}

func (s *ModelService) buildModelRollbackEvent(
	mv *model.ModelVersion,
	rollbackID string,
	phase string,
	fromVersion string,
	expectedActiveRevision int64,
	eventID string,
) (*ModelUpdateEvent, error) {
	if phase != modelRollbackPhaseAttempt && phase != modelRollbackPhaseCompensation {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid model rollback phase")
	}
	if _, err := governedModelArtifactSHA256(mv); err != nil {
		return nil, err
	}
	action := "rollback-activated"
	if phase == modelRollbackPhaseCompensation {
		action = "rollback-compensate"
	}
	return &ModelUpdateEvent{
		EventID: eventID, SchemaVersion: 2, TenantID: mv.TenantID,
		ModelID: mv.ModelID, ModelName: mv.ModelName, ModelType: mv.ModelType,
		Version: mv.ModelVersion, ArtifactURI: mv.ArtifactURI,
		ArtifactManifestURI:    mv.ArtifactManifestURI,
		ArtifactManifestSHA256: mv.ArtifactManifestSHA256,
		PackageID:              mv.PackageID, PackageSHA256: mv.PackageSHA256,
		EvaluationSHA256: mv.EvaluationSHA256, ExplanationSHA256: mv.ExplanationSHA256,
		GraphSnapshotID: mv.GraphSnapshotID, GraphSnapshotSHA256: mv.GraphSnapshotSHA256,
		AggregateRevision: mv.Revision, Compatibility: mv.Compatibility,
		Action: action, Metrics: mv.Metrics,
		ExpectedAppliedParallelism: s.config.AppliedAckExpectedParallelism,
		RollbackID:                 rollbackID, RollbackPhase: phase,
		RollbackFromVersion: fromVersion, ExpectedActiveRevision: expectedActiveRevision,
		ConsumerDeploymentID:  s.config.ModelConsumerDeploymentID,
		ConsumerProfileSHA256: s.config.ModelConsumerProfileSHA256,
		Timestamp:             time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func insertModelRollbackOutboxTx(ctx context.Context, tx *sql.Tx, event *ModelUpdateEvent, jobID string) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal governed model rollback event")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_update_outbox (
			event_id,tenant_id,model_id,model_version,action,partition_key,payload,
			action_job_id,status,available_at,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$3,$6::jsonb,$7,'pending',now(),now(),now())
	`, event.EventID, event.TenantID, event.ModelID, event.Version, event.Action, string(payload), jobID); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist governed model rollback outbox event")
	}
	return nil
}

// GetModelRollbackReceipt returns the control-plane and broker facts for a
// durable action job.  The existing versions/active endpoint remains the
// authoritative final-version query.
func (s *ModelService) GetModelRollbackReceipt(
	ctx context.Context, modelID, actionJobID string, opCtx *OperationContext,
) (*ModelRollbackReceipt, error) {
	if opCtx == nil || !opCtx.Authenticated {
		return nil, errors.New(errors.ErrCodeUnauthorized, "authenticated model rollback read is required")
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, opCtx.TenantID); err != nil {
		return nil, err
	}
	receipt, err := scanModelRollbackReceipt(s.db.QueryRowContext(ctx, `
		SELECT r.rollback_id::text,r.action_job_id,r.rollback_event_id,
		       COALESCE(r.compensation_event_id,''),r.tenant_id,r.model_id::text,
		       r.from_model_version,r.from_revision,r.target_model_version,r.target_revision,
		       r.consumer_deployment_id,r.consumer_profile_sha256,r.expected_parallelism,
		       r.applied_subtasks,r.compensation_applied_subtasks,r.state,r.active_switched,
		       (attempt.status='published'),COALESCE(compensation.status='published',false),
		       r.request_sha256,r.reason,r.failure_reason,r.created_at,r.updated_at,r.recovered_at
		FROM model_rollback_requests r
		JOIN model_update_outbox attempt ON attempt.event_id=r.rollback_event_id
		LEFT JOIN model_update_outbox compensation ON compensation.event_id=r.compensation_event_id
		WHERE r.tenant_id=$1 AND r.model_id=$2::uuid AND r.action_job_id=$3
	`, opCtx.TenantID, strings.TrimSpace(modelID), strings.TrimSpace(actionJobID)))
	if err == sql.ErrNoRows {
		return nil, errors.New(errors.ErrCodeModelVersionNotFound, "model rollback receipt not found")
	}
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to read model rollback receipt")
	}
	return receipt, nil
}

type modelRollbackScanner interface {
	Scan(dest ...interface{}) error
}

func scanModelRollbackReceipt(scanner modelRollbackScanner) (*ModelRollbackReceipt, error) {
	receipt := &ModelRollbackReceipt{GovernanceVersion: modelRollbackGovernanceVersion}
	var recoveredAt sql.NullTime
	err := scanner.Scan(
		&receipt.RollbackID, &receipt.ActionJobID, &receipt.RollbackEventID,
		&receipt.CompensationEventID, &receipt.TenantID, &receipt.ModelID,
		&receipt.FromModelVersion, &receipt.FromRevision,
		&receipt.TargetModelVersion, &receipt.TargetRevision,
		&receipt.ConsumerDeploymentID, &receipt.ConsumerProfileSHA256,
		&receipt.ExpectedParallelism, &receipt.AppliedSubtasks,
		&receipt.CompensationAppliedSubtasks, &receipt.State,
		&receipt.ActiveSwitched, &receipt.BrokerPublished,
		&receipt.CompensationBrokerPublished, &receipt.RequestSHA256,
		&receipt.Reason, &receipt.FailureReason, &receipt.CreatedAt,
		&receipt.UpdatedAt, &recoveredAt,
	)
	if err != nil {
		return nil, err
	}
	if recoveredAt.Valid {
		receipt.RecoveredAt = &recoveredAt.Time
	}
	return receipt, nil
}

// advanceModelRollbackFromAckTx owns the only active-pointer transition used
// by rollback v2.  It is called after the acknowledgement row and exact-set
// aggregate have been locked in the same transaction.
func (s *ModelService) advanceModelRollbackFromAckTx(
	ctx context.Context,
	tx *sql.Tx,
	ack ModelAppliedAck,
	contract modelAppliedContract,
	appliedCount int,
	allExpectedSubtasksApplied bool,
	hasFailure bool,
	failureReason string,
) (bool, error) {
	if contract.RollbackID == "" {
		return false, nil
	}
	if !s.config.EnableModelRollbackV2 {
		return true, errors.New(errors.ErrCodeServiceUnavailable, "governed model rollback acknowledgement writer is disabled")
	}

	var receipt ModelRollbackReceipt
	var fromPackageSHA, targetPackageSHA, requestedBy string
	var recoveredAt sql.NullTime
	var compensationEventID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT rollback_id::text,action_job_id,rollback_event_id,compensation_event_id,
		       tenant_id,model_id::text,from_model_version,from_revision,from_package_sha256,
		       target_model_version,target_revision,target_package_sha256,
		       consumer_deployment_id,consumer_profile_sha256,expected_parallelism,
		       applied_subtasks,compensation_applied_subtasks,state,active_switched,
		       request_sha256,reason,failure_reason,requested_by,created_at,updated_at,recovered_at
		FROM model_rollback_requests
		WHERE rollback_id=$1::uuid
		FOR UPDATE
	`, contract.RollbackID).Scan(
		&receipt.RollbackID, &receipt.ActionJobID, &receipt.RollbackEventID, &compensationEventID,
		&receipt.TenantID, &receipt.ModelID, &receipt.FromModelVersion, &receipt.FromRevision,
		&fromPackageSHA, &receipt.TargetModelVersion, &receipt.TargetRevision, &targetPackageSHA,
		&receipt.ConsumerDeploymentID, &receipt.ConsumerProfileSHA256,
		&receipt.ExpectedParallelism, &receipt.AppliedSubtasks,
		&receipt.CompensationAppliedSubtasks, &receipt.State, &receipt.ActiveSwitched,
		&receipt.RequestSHA256, &receipt.Reason, &receipt.FailureReason, &requestedBy,
		&receipt.CreatedAt, &receipt.UpdatedAt, &recoveredAt,
	)
	if err != nil {
		return true, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock model rollback receipt")
	}
	if compensationEventID.Valid {
		receipt.CompensationEventID = compensationEventID.String
	}
	expectedEventID := receipt.RollbackEventID
	expectedVersion := receipt.TargetModelVersion
	expectedPhase := modelRollbackPhaseAttempt
	if contract.RollbackPhase == modelRollbackPhaseCompensation {
		expectedEventID = receipt.CompensationEventID
		expectedVersion = receipt.FromModelVersion
		expectedPhase = modelRollbackPhaseCompensation
	}
	if contract.RollbackPhase != expectedPhase || ack.EventID != expectedEventID || ack.Version != expectedVersion ||
		ack.RollbackID != receipt.RollbackID || ack.RollbackPhase != expectedPhase ||
		ack.ConsumerDeploymentID != receipt.ConsumerDeploymentID ||
		ack.ConsumerProfileSHA256 != receipt.ConsumerProfileSHA256 {
		return true, errors.New(errors.ErrCodeVersionConflict, "model rollback acknowledgement does not match the frozen rollback identity")
	}

	if expectedPhase == modelRollbackPhaseAttempt {
		if receipt.State == "RECOVERED" || receipt.State == "FAILED_RESTORED" || receipt.State == "COMPENSATION_FAILED" {
			return true, nil
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE model_rollback_requests
			SET applied_subtasks=GREATEST(applied_subtasks,$2),
			    state=CASE WHEN state IN ('PENDING_ACK','PARTIAL') AND $2>0 THEN 'PARTIAL' ELSE state END,
			    failure_reason=CASE WHEN $3 THEN $4 ELSE failure_reason END,
			    updated_at=now()
			WHERE rollback_id=$1::uuid
		`, receipt.RollbackID, appliedCount, hasFailure, failureReason); err != nil {
			return true, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update model rollback acknowledgement progress")
		}
		if hasFailure {
			return true, s.startModelRollbackCompensationTx(ctx, tx, &receipt,
				"ROLLBACK_APPLY_FAILED: "+strings.TrimSpace(failureReason), requestedBy)
		}
		if !allExpectedSubtasksApplied {
			return true, nil
		}
		return true, s.commitRecoveredModelRollbackTx(ctx, tx, &receipt, requestedBy)
	}

	if receipt.State == "FAILED_RESTORED" || receipt.State == "COMPENSATION_FAILED" {
		return true, nil
	}
	if receipt.State != "COMPENSATING" {
		return true, errors.Newf(errors.ErrCodeInvalidStateTransition,
			"compensation acknowledgement received in rollback state %s", receipt.State)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_rollback_requests
		SET compensation_applied_subtasks=GREATEST(compensation_applied_subtasks,$2),updated_at=now()
		WHERE rollback_id=$1::uuid
	`, receipt.RollbackID, appliedCount); err != nil {
		return true, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update model rollback compensation progress")
	}
	if hasFailure {
		return true, s.finishModelRollbackCompensationTx(ctx, tx, &receipt, false,
			"ROLLBACK_COMPENSATION_FAILED: "+strings.TrimSpace(failureReason), requestedBy)
	}
	if !allExpectedSubtasksApplied {
		return true, nil
	}
	return true, s.finishModelRollbackCompensationTx(ctx, tx, &receipt, true, receipt.FailureReason, requestedBy)
}

func (s *ModelService) commitRecoveredModelRollbackTx(
	ctx context.Context, tx *sql.Tx, receipt *ModelRollbackReceipt, requestedBy string,
) error {
	var activeVersion string
	var activeRevision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT model_version,revision FROM model_versions
		WHERE tenant_id=$1 AND model_id=$2::uuid AND status='active' FOR UPDATE
	`, receipt.TenantID, receipt.ModelID).Scan(&activeVersion, &activeRevision); err != nil {
		return s.startModelRollbackCompensationTx(ctx, tx, receipt,
			"ACTIVE_POINTER_QUERY_FAILED: "+err.Error(), requestedBy)
	}
	if activeVersion != receipt.FromModelVersion || activeRevision != receipt.FromRevision {
		return s.startModelRollbackCompensationTx(ctx, tx, receipt,
			fmt.Sprintf("ACTIVE_POINTER_CONFLICT: expected %s revision %d, got %s revision %d",
				receipt.FromModelVersion, receipt.FromRevision, activeVersion, activeRevision), requestedBy)
	}
	fromResult, err := tx.ExecContext(ctx, `
		UPDATE model_versions SET status='deprecated',updated_at=now()
		WHERE tenant_id=$1 AND model_id=$2::uuid AND model_version=$3
		  AND revision=$4 AND status='active'
	`, receipt.TenantID, receipt.ModelID, receipt.FromModelVersion, receipt.FromRevision)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to retire the displaced active model")
	}
	if affected, _ := fromResult.RowsAffected(); affected != 1 {
		return errors.New(errors.ErrCodeConcurrentModify, "active model ownership was lost during rollback commit")
	}
	targetResult, err := tx.ExecContext(ctx, `
		UPDATE model_versions SET status='active',updated_at=now()
		WHERE tenant_id=$1 AND model_id=$2::uuid AND model_version=$3
		  AND revision=$4 AND status='deprecated'
	`, receipt.TenantID, receipt.ModelID, receipt.TargetModelVersion, receipt.TargetRevision)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to activate the acknowledged rollback target")
	}
	if affected, _ := targetResult.RowsAffected(); affected != 1 {
		return errors.New(errors.ErrCodeConcurrentModify, "rollback target ownership was lost during final commit")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_rollback_requests
		SET state='RECOVERED',applied_subtasks=expected_parallelism,
		    active_switched=true,recovered_at=now(),updated_at=now()
		WHERE rollback_id=$1::uuid AND state IN ('PENDING_ACK','PARTIAL')
	`, receipt.RollbackID); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to close recovered model rollback")
	}
	return finalizeModelRollbackJobTx(ctx, tx, receipt, "completed",
		"MODEL_VERSION_ROLLBACK_COMPLETED", "", requestedBy, true)
}

func (s *ModelService) startModelRollbackCompensationTx(
	ctx context.Context, tx *sql.Tx, receipt *ModelRollbackReceipt, reason, requestedBy string,
) error {
	if receipt.CompensationEventID != "" || receipt.State == "COMPENSATING" {
		return nil
	}
	from, err := s.getModelVersionForUpdate(ctx, tx, receipt.FromModelVersion)
	if err != nil {
		return err
	}
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("model-rollback-compensation.v2:"+receipt.RollbackID)).String()
	event, err := s.buildModelRollbackEvent(from, receipt.RollbackID,
		modelRollbackPhaseCompensation, receipt.TargetModelVersion, receipt.FromRevision, eventID)
	if err != nil {
		return err
	}
	if err := insertModelRollbackOutboxTx(ctx, tx, event, receipt.ActionJobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_rollback_requests
		SET compensation_event_id=$2,state='COMPENSATING',failure_reason=$3,updated_at=now()
		WHERE rollback_id=$1::uuid AND state IN ('PENDING_ACK','PARTIAL')
	`, receipt.RollbackID, eventID, strings.TrimSpace(reason)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start model rollback compensation")
	}
	return insertModelRollbackAuditTx(ctx, tx, receipt, requestedBy,
		"MODEL_VERSION_ROLLBACK_COMPENSATION_REQUESTED", "running", reason, false)
}

func (s *ModelService) finishModelRollbackCompensationTx(
	ctx context.Context, tx *sql.Tx, receipt *ModelRollbackReceipt,
	restored bool, failureReason, requestedBy string,
) error {
	state := "COMPENSATION_FAILED"
	jobStatus := "partial"
	auditAction := "MODEL_VERSION_ROLLBACK_COMPENSATION_FAILED"
	if restored {
		state = "FAILED_RESTORED"
		jobStatus = "failed"
		auditAction = "MODEL_VERSION_ROLLBACK_FAILED_RESTORED"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_rollback_requests
		SET state=$2,
		    compensation_applied_subtasks=CASE WHEN $3 THEN expected_parallelism ELSE compensation_applied_subtasks END,
		    failure_reason=$4,active_switched=false,recovered_at=NULL,updated_at=now()
		WHERE rollback_id=$1::uuid AND state='COMPENSATING'
	`, receipt.RollbackID, state, restored, strings.TrimSpace(failureReason)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to close model rollback compensation")
	}
	return finalizeModelRollbackJobTx(ctx, tx, receipt, jobStatus, auditAction,
		failureReason, requestedBy, false)
}

func finalizeModelRollbackJobTx(
	ctx context.Context, tx *sql.Tx, receipt *ModelRollbackReceipt,
	jobStatus, auditAction, failureReason, requestedBy string, activeSwitched bool,
) error {
	resultJSON, err := json.Marshal(map[string]interface{}{
		"rollback_id":          receipt.RollbackID,
		"from_model_version":   receipt.FromModelVersion,
		"target_model_version": receipt.TargetModelVersion,
		"active_switched":      activeSwitched,
		"stage":                "exact-flink-quorum",
		"failure":              strings.TrimSpace(failureReason),
	})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode model rollback result")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE model_action_jobs
		SET status=$2,result=$3::jsonb,error=$4,updated_at=now()
		WHERE job_id=$1 AND status='running'
	`, receipt.ActionJobID, jobStatus, string(resultJSON), strings.TrimSpace(failureReason))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to finalize model rollback action")
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New(errors.ErrCodeConcurrentModify, "model rollback action is no longer running")
	}
	return insertModelRollbackAuditTx(ctx, tx, receipt, requestedBy,
		auditAction, jobStatus, failureReason, activeSwitched)
}

func insertModelRollbackAuditTx(
	ctx context.Context, tx *sql.Tx, receipt *ModelRollbackReceipt, requestedBy,
	action, status, failureReason string, activeSwitched bool,
) error {
	detail, err := json.Marshal(map[string]interface{}{
		"rollback_id":            receipt.RollbackID,
		"job_id":                 receipt.ActionJobID,
		"from_model_version":     receipt.FromModelVersion,
		"target_model_version":   receipt.TargetModelVersion,
		"expected_parallelism":   receipt.ExpectedParallelism,
		"status":                 status,
		"active_pointer_changed": activeSwitched,
		"failure":                strings.TrimSpace(failureReason),
	})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode model rollback audit")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id,user_id,action,object_type,object_id,detail)
		VALUES ($1,NULLIF($2,'')::uuid,$3,'model',$4,$5::jsonb)
	`, receipt.TenantID, requestedBy, action, receipt.ModelID, string(detail)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist model rollback audit")
	}
	return nil
}

// expireTimedOutModelRollbacks turns an attempt timeout into an explicit
// compensation deployment.  A compensation timeout becomes terminal PARTIAL;
// it never changes the database active pointer or claims recovery.
func (s *ModelService) expireTimedOutModelRollbacks(ctx context.Context, now time.Time) (int, error) {
	if !s.config.EnableModelRollbackV2 {
		return 0, nil
	}
	if s.config.ModelRollbackAckTimeout <= 0 {
		return 0, errors.New(errors.ErrCodeInvalidParameter, "model rollback ACK timeout must be positive")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin model rollback timeout sweep")
	}
	defer tx.Rollback()
	cutoff := now.UTC().Add(-s.config.ModelRollbackAckTimeout)
	rows, err := tx.QueryContext(ctx, `
		SELECT rollback_id::text,action_job_id,tenant_id,model_id::text,
		       from_model_version,from_revision,target_model_version,target_revision,
		       expected_parallelism,state,requested_by,
		       COALESCE(compensation_event_id,''),failure_reason
		FROM model_rollback_requests
		WHERE state IN ('PENDING_ACK','PARTIAL','COMPENSATING') AND updated_at < $1
		ORDER BY updated_at,rollback_id
		FOR UPDATE SKIP LOCKED
		LIMIT 20
	`, cutoff)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to select timed out model rollbacks")
	}
	type timedOut struct {
		receipt     ModelRollbackReceipt
		requestedBy string
	}
	items := make([]timedOut, 0, 20)
	for rows.Next() {
		var item timedOut
		if err := rows.Scan(
			&item.receipt.RollbackID, &item.receipt.ActionJobID,
			&item.receipt.TenantID, &item.receipt.ModelID,
			&item.receipt.FromModelVersion, &item.receipt.FromRevision,
			&item.receipt.TargetModelVersion, &item.receipt.TargetRevision,
			&item.receipt.ExpectedParallelism, &item.receipt.State,
			&item.requestedBy, &item.receipt.CompensationEventID,
			&item.receipt.FailureReason,
		); err != nil {
			rows.Close()
			return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan timed out model rollback")
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to close model rollback timeout rows")
	}
	for index := range items {
		item := &items[index]
		if item.receipt.State == "COMPENSATING" {
			if err := s.finishModelRollbackCompensationTx(ctx, tx, &item.receipt, false,
				"ROLLBACK_COMPENSATION_ACK_TIMEOUT", item.requestedBy); err != nil {
				return 0, err
			}
			continue
		}
		if err := s.startModelRollbackCompensationTx(ctx, tx, &item.receipt,
			"ROLLBACK_ACK_TIMEOUT", item.requestedBy); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model rollback timeout sweep")
	}
	return len(items), nil
}

func (s *ModelService) reconcileDeadModelRollbackOutbox(ctx context.Context, eventID, publishFailure string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin dead rollback outbox reconciliation")
	}
	defer tx.Rollback()
	var receipt ModelRollbackReceipt
	var requestedBy string
	err = tx.QueryRowContext(ctx, `
		SELECT rollback_id::text,action_job_id,tenant_id,model_id::text,
		       from_model_version,from_revision,target_model_version,target_revision,
		       expected_parallelism,state,requested_by,
		       COALESCE(compensation_event_id,''),failure_reason
		FROM model_rollback_requests
		WHERE rollback_event_id=$1 OR compensation_event_id=$1
		FOR UPDATE
	`, strings.TrimSpace(eventID)).Scan(
		&receipt.RollbackID, &receipt.ActionJobID, &receipt.TenantID, &receipt.ModelID,
		&receipt.FromModelVersion, &receipt.FromRevision,
		&receipt.TargetModelVersion, &receipt.TargetRevision,
		&receipt.ExpectedParallelism, &receipt.State, &requestedBy,
		&receipt.CompensationEventID, &receipt.FailureReason,
	)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock dead rollback outbox receipt")
	}
	reason := "ROLLBACK_OUTBOX_DEAD: " + strings.TrimSpace(publishFailure)
	if eventID == receipt.CompensationEventID {
		if err := s.finishModelRollbackCompensationTx(ctx, tx, &receipt, false, reason, requestedBy); err != nil {
			return err
		}
	} else if err := s.startModelRollbackCompensationTx(ctx, tx, &receipt, reason, requestedBy); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit dead rollback outbox reconciliation")
	}
	return nil
}

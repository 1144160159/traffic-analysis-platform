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
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
)

const modelShadowActivationGovernanceVersion = "model-shadow-activation.v1"

// PrepareModelShadowActivationRequest is an independently approved command.
// The authenticated caller is the approver; RequestedBy identifies the
// different principal that requested the rollout.
type PrepareModelShadowActivationRequest struct {
	ModelID          string `json:"-"`
	ModelVersion     string `json:"-"`
	IdempotencyKey   string `json:"-"`
	ExpectedRevision *int64 `json:"expected_revision"`
	RequestedBy      string `json:"requested_by"`
	ApprovalReason   string `json:"approval_reason"`
}

// ModelShadowActivationReceipt is the durable control-plane fact.  It never
// claims that the model is serving; ShadowReady only means every isolated
// consumer subtask staged the signed package.
type ModelShadowActivationReceipt struct {
	RequestID            string     `json:"request_id"`
	EventID              string     `json:"event_id"`
	GovernanceVersion    string     `json:"governance_version"`
	TenantID             string     `json:"tenant_id"`
	ModelID              string     `json:"model_id"`
	ModelVersion         string     `json:"model_version"`
	PackageID            string     `json:"package_id"`
	PackageSHA256        string     `json:"package_sha256"`
	ExpectedRevision     int64      `json:"expected_revision"`
	AggregateRevision    int64      `json:"aggregate_revision"`
	RequestedBy          string     `json:"requested_by"`
	ApprovedBy           string     `json:"approved_by"`
	ApprovalReason       string     `json:"approval_reason"`
	RequestSHA256        string     `json:"request_sha256"`
	State                string     `json:"state"`
	ServingActivated     bool       `json:"serving_activated"`
	ShadowReadyExpiresAt *time.Time `json:"shadow_ready_expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

func (r *PrepareModelShadowActivationRequest) validate(tenantID, approvedBy string) error {
	var invalid []string
	if strings.TrimSpace(tenantID) == "" {
		invalid = append(invalid, "authenticated tenant is required")
	}
	if strings.TrimSpace(r.ModelID) == "" || strings.TrimSpace(r.ModelVersion) == "" {
		invalid = append(invalid, "model id and version are required")
	}
	key := strings.TrimSpace(r.IdempotencyKey)
	if len(key) < 16 || len(key) > 200 {
		invalid = append(invalid, "Idempotency-Key must contain 16 to 200 characters")
	}
	if r.ExpectedRevision == nil || *r.ExpectedRevision < 0 {
		invalid = append(invalid, "expected_revision must be present and non-negative")
	}
	if strings.TrimSpace(r.RequestedBy) == "" {
		invalid = append(invalid, "requested_by is required")
	}
	if strings.TrimSpace(approvedBy) == "" {
		invalid = append(invalid, "authenticated approver is required")
	}
	if strings.TrimSpace(r.RequestedBy) == strings.TrimSpace(approvedBy) {
		invalid = append(invalid, "self-approval is forbidden")
	}
	if len(strings.TrimSpace(r.ApprovalReason)) < 8 || len(strings.TrimSpace(r.ApprovalReason)) > 1000 {
		invalid = append(invalid, "approval_reason must contain 8 to 1000 characters")
	}
	if len(invalid) > 0 {
		return errors.New(errors.ErrCodeInvalidRequest, strings.Join(invalid, "; "))
	}
	return nil
}

func shadowActivationRequestSHA256(tenantID, approvedBy string, req *PrepareModelShadowActivationRequest) (string, error) {
	payload := struct {
		GovernanceVersion string `json:"governance_version"`
		TenantID          string `json:"tenant_id"`
		ModelID           string `json:"model_id"`
		ModelVersion      string `json:"model_version"`
		ExpectedRevision  int64  `json:"expected_revision"`
		RequestedBy       string `json:"requested_by"`
		ApprovedBy        string `json:"approved_by"`
		ApprovalReason    string `json:"approval_reason"`
	}{
		GovernanceVersion: modelShadowActivationGovernanceVersion,
		TenantID:          strings.TrimSpace(tenantID), ModelID: strings.TrimSpace(req.ModelID),
		ModelVersion: strings.TrimSpace(req.ModelVersion), ExpectedRevision: *req.ExpectedRevision,
		RequestedBy: strings.TrimSpace(req.RequestedBy), ApprovedBy: strings.TrimSpace(approvedBy),
		ApprovalReason: strings.TrimSpace(req.ApprovalReason),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

// PrepareModelShadowActivation atomically validates consumer readiness,
// approval, metadata, concurrency and idempotency before writing one schema-v2
// shadow-load outbox event.  It intentionally leaves model_versions.status and
// every current champion unchanged.
func (s *ModelService) PrepareModelShadowActivation(
	ctx context.Context,
	req *PrepareModelShadowActivationRequest,
	opCtx *OperationContext,
) (*ModelShadowActivationReceipt, error) {
	if !s.config.EnableModelShadowActivation {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "model shadow activation writer is disabled")
	}
	if req == nil || opCtx == nil || !opCtx.Authenticated {
		return nil, errors.New(errors.ErrCodeUnauthorized, "authenticated model shadow activation request is required")
	}
	if err := req.validate(opCtx.TenantID, opCtx.UserID); err != nil {
		return nil, err
	}
	requestSHA, err := shadowActivationRequestSHA256(opCtx.TenantID, opCtx.UserID, req)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to hash model shadow activation request")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start model shadow activation transaction")
	}
	defer tx.Rollback()

	var lock interface{}
	if err := tx.QueryRowContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))`,
		opCtx.TenantID, strings.TrimSpace(req.IdempotencyKey),
	).Scan(&lock); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock model shadow activation idempotency key")
	}
	if existing, err := getModelShadowActivationReceiptByIdempotencyTx(
		ctx, tx, opCtx.TenantID, strings.TrimSpace(req.IdempotencyKey),
	); err == nil {
		if existing.RequestSHA256 != requestSHA {
			return nil, errors.New(errors.ErrCodeVersionConflict, "Idempotency-Key was already used for a different shadow activation request")
		}
		if err := tx.Commit(); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit idempotent shadow activation read")
		}
		return existing, nil
	} else if err != sql.ErrNoRows {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to read shadow activation idempotency record")
	}

	mv, err := s.getModelVersionForUpdate(ctx, tx, req.ModelVersion)
	if err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelActivate, mv.TenantID); err != nil {
		return nil, err
	}
	if mv.TenantID != opCtx.TenantID || mv.ModelID != req.ModelID {
		return nil, errors.New(errors.ErrCodeModelVersionNotFound, "model version is outside the requested tenant/model scope")
	}
	if model.ModelStatus(mv.Status) != model.ModelStatusRegistered {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition,
			"shadow activation requires a registered version, got %s", mv.Status)
	}
	if err := s.validateGovernedShadowMetadata(mv); err != nil {
		return nil, err
	}
	if err := s.validateIndependentShadowApprovalTx(ctx, tx, mv, req.RequestedBy, opCtx.UserID); err != nil {
		return nil, err
	}
	if err := s.requireExactConsumerReadyReceiptTx(ctx, tx); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_shadow_activation_aggregates (tenant_id,model_id,aggregate_revision,updated_at)
		VALUES ($1,$2::uuid,0,now()) ON CONFLICT (tenant_id,model_id) DO NOTHING
	`, mv.TenantID, mv.ModelID); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to initialize model shadow aggregate")
	}
	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT aggregate_revision FROM model_shadow_activation_aggregates
		WHERE tenant_id=$1 AND model_id=$2::uuid FOR UPDATE
	`, mv.TenantID, mv.ModelID).Scan(&currentRevision); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock model shadow aggregate")
	}
	if currentRevision != *req.ExpectedRevision {
		return nil, errors.Newf(errors.ErrCodeVersionConflict,
			"shadow activation expected revision %d, current revision is %d", *req.ExpectedRevision, currentRevision)
	}
	aggregateRevision := currentRevision + 1
	requestID, eventID := uuid.NewString(), uuid.NewString()
	createdAt := time.Now().UTC()
	event := ModelUpdateEvent{
		EventID: eventID, SchemaVersion: 2, TenantID: mv.TenantID, ModelID: mv.ModelID,
		ModelName: mv.ModelName, ModelType: mv.ModelType, Version: mv.ModelVersion,
		ArtifactURI: mv.ArtifactURI, ArtifactManifestURI: mv.ArtifactManifestURI,
		ArtifactManifestSHA256: mv.ArtifactManifestSHA256, PackageID: mv.PackageID,
		PackageSHA256: mv.PackageSHA256, EvaluationSHA256: mv.EvaluationSHA256,
		ExplanationSHA256: mv.ExplanationSHA256, GraphSnapshotID: mv.GraphSnapshotID,
		GraphSnapshotSHA256: mv.GraphSnapshotSHA256, AggregateRevision: aggregateRevision,
		Compatibility: mv.Compatibility, Action: "shadow-load", Metrics: mv.Metrics,
		ExpectedAppliedParallelism: s.config.AppliedAckExpectedParallelism,
		Timestamp:                  createdAt.Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal schema-v2 shadow-load event")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_update_outbox (
			event_id,tenant_id,model_id,model_version,action,partition_key,payload,
			action_job_id,status,available_at,created_at,updated_at
		) VALUES ($1,$2,$3,$4,'shadow-load',$3,$5::jsonb,'','pending',now(),now(),now())
	`, eventID, mv.TenantID, mv.ModelID, mv.ModelVersion, string(payload)); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist schema-v2 shadow-load outbox")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_shadow_activation_requests (
			request_id,event_id,tenant_id,model_id,model_version,package_id,package_sha256,
			idempotency_key,request_sha256,expected_revision,aggregate_revision,
			requested_by,approved_by,approval_reason,created_at
		) VALUES ($1,$2,$3,$4::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, requestID, eventID, mv.TenantID, mv.ModelID, mv.ModelVersion, mv.PackageID,
		mv.PackageSHA256, strings.TrimSpace(req.IdempotencyKey), requestSHA,
		*req.ExpectedRevision, aggregateRevision, strings.TrimSpace(req.RequestedBy),
		strings.TrimSpace(opCtx.UserID), strings.TrimSpace(req.ApprovalReason), createdAt); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist model shadow activation command")
	}
	advanceResult, err := tx.ExecContext(ctx, `
		UPDATE model_shadow_activation_aggregates SET aggregate_revision=$3,updated_at=$4
		WHERE tenant_id=$1 AND model_id=$2::uuid AND aggregate_revision=$5
	`, mv.TenantID, mv.ModelID, aggregateRevision, createdAt, currentRevision)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to advance model shadow aggregate")
	}
	if affected, err := advanceResult.RowsAffected(); err != nil || affected != 1 {
		return nil, errors.New(errors.ErrCodeConcurrentModify, "model shadow aggregate ownership was lost")
	}
	detail := map[string]interface{}{
		"request_id": requestID, "event_id": eventID, "model_id": mv.ModelID,
		"model_version": mv.ModelVersion, "package_id": mv.PackageID,
		"package_sha256": mv.PackageSHA256, "expected_revision": currentRevision,
		"aggregate_revision": aggregateRevision, "requested_by": strings.TrimSpace(req.RequestedBy),
		"approved_by": strings.TrimSpace(opCtx.UserID), "request_sha256": requestSHA,
		"action": "shadow-load", "model_status_unchanged": mv.Status,
		"serving_activated": false,
	}
	if err := s.recordAuditLogTx(ctx, tx, opCtx, audit.EventType("MODEL_SHADOW_ACTIVATION_OUTBOX_PREPARED"), "model_version", mv.ModelVersion, detail); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model shadow activation")
	}
	return &ModelShadowActivationReceipt{
		RequestID: requestID, EventID: eventID, GovernanceVersion: modelShadowActivationGovernanceVersion,
		TenantID: mv.TenantID, ModelID: mv.ModelID, ModelVersion: mv.ModelVersion,
		PackageID: mv.PackageID, PackageSHA256: mv.PackageSHA256,
		ExpectedRevision: currentRevision, AggregateRevision: aggregateRevision,
		RequestedBy: strings.TrimSpace(req.RequestedBy), ApprovedBy: strings.TrimSpace(opCtx.UserID),
		ApprovalReason: strings.TrimSpace(req.ApprovalReason), RequestSHA256: requestSHA,
		State: "outbox_pending", ServingActivated: false, CreatedAt: createdAt,
	}, nil
}

func (s *ModelService) validateGovernedShadowMetadata(mv *model.ModelVersion) error {
	for field, value := range map[string]string{
		"artifact_uri": mv.ArtifactURI, "artifact_manifest_uri": mv.ArtifactManifestURI,
		"package_id": mv.PackageID, "graph_snapshot_id": mv.GraphSnapshotID,
		"signing_key_id": mv.SigningKeyID,
	} {
		if strings.TrimSpace(value) == "" {
			return errors.Newf(errors.ErrCodeInvalidStateTransition, "shadow activation metadata is missing %s", field)
		}
	}
	if _, err := uuid.Parse(mv.PackageID); err != nil {
		return errors.New(errors.ErrCodeInvalidStateTransition, "shadow activation package_id must be a UUID")
	}
	for field, value := range map[string]string{
		"package_sha256": mv.PackageSHA256, "artifact_manifest_sha256": mv.ArtifactManifestSHA256,
		"evaluation_sha256": mv.EvaluationSHA256, "explanation_sha256": mv.ExplanationSHA256,
		"graph_snapshot_sha256": mv.GraphSnapshotSHA256,
	} {
		if !consumerSHA256Pattern.MatchString(value) {
			return errors.Newf(errors.ErrCodeInvalidStateTransition, "shadow activation metadata %s is not a lowercase SHA-256", field)
		}
	}
	if stringValue(mv.Compatibility, "runtime_contract") != s.config.ModelConsumerRuntimeContract ||
		stringValue(mv.Compatibility, "runtime_version") != s.config.ModelConsumerRuntimeVersion ||
		intValue(mv.Compatibility, "feature_schema_version") != s.config.ModelConsumerFeatureSchema ||
		intValue(mv.Compatibility, "graph_schema_version") != s.config.ModelConsumerGraphSchema ||
		stringValue(mv.Compatibility, "feature_set_id") != mv.FeatureSetID {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model package compatibility does not match the configured shadow consumer")
	}
	baseline, baselineOK := mv.Compatibility["baseline"].(map[string]interface{})
	gnn, gnnOK := mv.Compatibility["gnn"].(map[string]interface{})
	if !baselineOK || stringValue(baseline, "format") != "onnx" ||
		!gnnOK || stringValue(gnn, "format") != "numpy_npz_v1" {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model package must contain compatible ONNX baseline and numpy_npz_v1 GNN artifacts")
	}
	return nil
}

func (s *ModelService) validateIndependentShadowApprovalTx(
	ctx context.Context, tx *sql.Tx, mv *model.ModelVersion, requestedBy, approvedBy string,
) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT COALESCE(payload->>'name','unnamed gate'),COALESCE(payload->>'status',''),
		       COALESCE(payload->>'requested_by',''),COALESCE(payload->>'approved_by','')
		FROM model_workbench_items
		WHERE tenant_id=$1 AND model_id=$2::uuid AND category='review_gates'
		ORDER BY ordinal,occurred_at DESC FOR UPDATE
	`, mv.TenantID, mv.ModelID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock model shadow review gates")
	}
	defer rows.Close()
	gateCount := 0
	callerApproved := false
	for rows.Next() {
		gateCount++
		var name, status, gateRequester, gateApprover string
		if err := rows.Scan(&name, &status, &gateRequester, &gateApprover); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model shadow review gate")
		}
		normalized := strings.ToLower(strings.TrimSpace(status))
		if normalized != "通过" && normalized != "已通过" && normalized != "passed" && normalized != "approved" {
			return errors.Newf(errors.ErrCodeInvalidStateTransition, "model shadow activation blocked by review gate %s", name)
		}
		if strings.TrimSpace(gateApprover) == "" || strings.TrimSpace(gateApprover) == strings.TrimSpace(requestedBy) {
			return errors.Newf(errors.ErrCodeInvalidStateTransition, "model shadow review gate %s lacks an independent approver", name)
		}
		if gateRequester != "" && strings.TrimSpace(gateRequester) != strings.TrimSpace(requestedBy) {
			return errors.Newf(errors.ErrCodeVersionConflict, "model shadow review gate %s requester does not match the command", name)
		}
		if strings.TrimSpace(gateApprover) == strings.TrimSpace(approvedBy) {
			callerApproved = true
		}
	}
	if err := rows.Err(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate model shadow review gates")
	}
	if gateCount == 0 || !callerApproved {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model shadow activation requires persisted gates approved by the authenticated independent approver")
	}
	return nil
}

func (s *ModelService) requireExactConsumerReadyReceiptTx(ctx context.Context, tx *sql.Tx) error {
	var expiresAt time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT expires_at FROM model_update_consumer_ready_receipts
		WHERE consumer_deployment_id=$1 AND consumer_profile_sha256=$2
		  AND runtime_contract=$3 AND runtime_version=$4
		  AND feature_schema_version=$5 AND graph_schema_version=$6
		  AND supported_model_formats=$7 AND expected_parallelism=$8
		  AND ready_subtasks=$8 AND status='ready' AND expires_at>now()
		FOR SHARE
	`, s.config.ModelConsumerDeploymentID, s.config.ModelConsumerProfileSHA256,
		s.config.ModelConsumerRuntimeContract, s.config.ModelConsumerRuntimeVersion,
		s.config.ModelConsumerFeatureSchema, s.config.ModelConsumerGraphSchema,
		s.config.ModelConsumerFormats, s.config.AppliedAckExpectedParallelism).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model shadow activation blocked: exact consumer-ready receipt is missing or expired")
	}
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to validate exact consumer-ready receipt")
	}
	return nil
}

func getModelShadowActivationReceiptByIdempotencyTx(
	ctx context.Context, tx *sql.Tx, tenantID, idempotencyKey string,
) (*ModelShadowActivationReceipt, error) {
	return scanModelShadowActivationReceipt(tx.QueryRowContext(ctx, `
		SELECT r.request_id,r.event_id,r.tenant_id,r.model_id::text,r.model_version,
		       r.package_id,r.package_sha256,r.expected_revision,r.aggregate_revision,
		       r.requested_by,r.approved_by,r.approval_reason,r.request_sha256,
		       CASE WHEN COALESCE(sa.has_failure,false) THEN 'failed'
		            WHEN sr.event_id IS NOT NULL AND sr.expires_at>now() THEN 'shadow_ready'
		            WHEN o.status='published' THEN 'published'
		            WHEN o.status='dead' THEN 'failed'
		            ELSE 'outbox_' || o.status END,
		       sr.expires_at,r.created_at
		FROM model_shadow_activation_requests r
		JOIN model_update_outbox o ON o.event_id=r.event_id
		LEFT JOIN model_update_shadow_ready_receipts sr ON sr.event_id=r.event_id
		LEFT JOIN (
			SELECT event_id,BOOL_OR(status IN ('failed','stale')) AS has_failure
			FROM model_update_shadow_acks GROUP BY event_id
		) sa ON sa.event_id=r.event_id
		WHERE r.tenant_id=$1 AND r.idempotency_key=$2
		FOR UPDATE OF r
	`, tenantID, idempotencyKey))
}

// GetModelShadowActivationReceipt reports durable outbox/shadow state without
// interpreting shadow readiness as a serving-model switch.
func (s *ModelService) GetModelShadowActivationReceipt(
	ctx context.Context, modelID, modelVersion, requestID string, opCtx *OperationContext,
) (*ModelShadowActivationReceipt, error) {
	if opCtx == nil || !opCtx.Authenticated {
		return nil, errors.New(errors.ErrCodeUnauthorized, "authenticated model shadow activation read is required")
	}
	if strings.TrimSpace(modelID) == "" || strings.TrimSpace(modelVersion) == "" || strings.TrimSpace(requestID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidRequest, "model id, version and request id are required")
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, opCtx.TenantID); err != nil {
		return nil, err
	}
	receipt, err := scanModelShadowActivationReceipt(s.db.QueryRowContext(ctx, `
		SELECT r.request_id,r.event_id,r.tenant_id,r.model_id::text,r.model_version,
		       r.package_id,r.package_sha256,r.expected_revision,r.aggregate_revision,
		       r.requested_by,r.approved_by,r.approval_reason,r.request_sha256,
		       CASE WHEN COALESCE(sa.has_failure,false) THEN 'failed'
		            WHEN sr.event_id IS NOT NULL AND sr.expires_at>now() THEN 'shadow_ready'
		            WHEN o.status='published' THEN 'published'
		            WHEN o.status='dead' THEN 'failed'
		            ELSE 'outbox_' || o.status END,
		       sr.expires_at,r.created_at
		FROM model_shadow_activation_requests r
		JOIN model_update_outbox o ON o.event_id=r.event_id
		LEFT JOIN model_update_shadow_ready_receipts sr ON sr.event_id=r.event_id
		LEFT JOIN (
			SELECT event_id,BOOL_OR(status IN ('failed','stale')) AS has_failure
			FROM model_update_shadow_acks GROUP BY event_id
		) sa ON sa.event_id=r.event_id
		WHERE r.tenant_id=$1 AND r.model_id=$2::uuid AND r.model_version=$3 AND r.request_id=$4
	`, opCtx.TenantID, strings.TrimSpace(modelID), strings.TrimSpace(modelVersion), strings.TrimSpace(requestID)))
	if err == sql.ErrNoRows {
		return nil, errors.New(errors.ErrCodeModelVersionNotFound, "model shadow activation receipt not found")
	}
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to read model shadow activation receipt")
	}
	return receipt, nil
}

type modelShadowActivationScanner interface {
	Scan(dest ...interface{}) error
}

func scanModelShadowActivationReceipt(scanner modelShadowActivationScanner) (*ModelShadowActivationReceipt, error) {
	receipt := &ModelShadowActivationReceipt{GovernanceVersion: modelShadowActivationGovernanceVersion, ServingActivated: false}
	var expiresAt sql.NullTime
	err := scanner.Scan(
		&receipt.RequestID, &receipt.EventID, &receipt.TenantID, &receipt.ModelID,
		&receipt.ModelVersion, &receipt.PackageID, &receipt.PackageSHA256,
		&receipt.ExpectedRevision, &receipt.AggregateRevision, &receipt.RequestedBy,
		&receipt.ApprovedBy, &receipt.ApprovalReason, &receipt.RequestSHA256,
		&receipt.State, &expiresAt, &receipt.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		receipt.ShadowReadyExpiresAt = &expiresAt.Time
	}
	return receipt, nil
}

func stringValue(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func intValue(values map[string]interface{}, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	default:
		return 0
	}
}

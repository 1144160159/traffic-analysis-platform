////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/repository/model_repository.go
// Model Repository — PostgreSQL CRUD for models & model_versions 表
//
// 对齐 common/sql/pg/03-models-deploy.sql
// 集成: MLOps Argo Workflows 训练流水线 + Flink Behavior Job 热更新
////////////////////////////////////////////////////////////////////////////////

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

// =============================================================================
// ModelRepository — 模型与模型版本的数据访问层
// =============================================================================

// ModelRepository 模型仓库
type ModelRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewModelRepository 创建模型仓库
func NewModelRepository(db *sql.DB, logger *zap.Logger) *ModelRepository {
	return &ModelRepository{db: db, logger: logger}
}

// =============================================================================
// 模型 CRUD
// =============================================================================

// CreateModel 创建模型
func (r *ModelRepository) CreateModel(ctx context.Context, m *model.Model) error {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.CreateModel")
	defer span.End()

	if m.ModelID == "" {
		m.ModelID = uuid.New().String()
	}
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	if err := m.MarshalMetadata(); err != nil {
		return err
	}

	query := `
		INSERT INTO models (model_id, tenant_id, name, model_type, description, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, name) DO NOTHING
		RETURNING model_id
	`

	var returnedID string
	err := r.db.QueryRowContext(ctx, query,
		m.ModelID, m.TenantID, m.Name, m.ModelType, m.Description, m.MetadataJSON, m.CreatedAt, m.UpdatedAt,
	).Scan(&returnedID)

	if err != nil {
		if err == sql.ErrNoRows {
			return errors.Newf(errors.ErrCodeResourceExists, "model with name '%s' already exists in tenant '%s'", m.Name, m.TenantID)
		}
		r.logger.Error("Failed to create model", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create model")
	}
	m.ModelID = returnedID
	return nil
}

// GetModel 获取模型
func (r *ModelRepository) GetModel(ctx context.Context, modelID string) (*model.Model, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.GetModel")
	defer span.End()

	query := `
		SELECT model_id, tenant_id, name, model_type, description, metadata, created_at, updated_at
		FROM models WHERE model_id = $1
	`

	var m model.Model
	var description sql.NullString

	err := r.db.QueryRowContext(ctx, query, modelID).Scan(
		&m.ModelID, &m.TenantID, &m.Name, &m.ModelType,
		&description, &m.MetadataJSON, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeModelNotFound, "model not found: %s", modelID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get model")
	}

	if description.Valid {
		m.Description = description.String
	}
	_ = m.UnmarshalMetadata()
	return &m, nil
}

// GetModelByName 按名称获取模型
func (r *ModelRepository) GetModelByName(ctx context.Context, tenantID, name string) (*model.Model, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.GetModelByName")
	defer span.End()

	query := `
		SELECT model_id, tenant_id, name, model_type, description, metadata, created_at, updated_at
		FROM models WHERE tenant_id = $1 AND name = $2
	`

	var m model.Model
	var description sql.NullString

	err := r.db.QueryRowContext(ctx, query, tenantID, name).Scan(
		&m.ModelID, &m.TenantID, &m.Name, &m.ModelType,
		&description, &m.MetadataJSON, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeModelNotFound, "model not found: tenant=%s name=%s", tenantID, name)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get model by name")
	}

	if description.Valid {
		m.Description = description.String
	}
	_ = m.UnmarshalMetadata()
	return &m, nil
}

// UpdateModel 更新模型
func (r *ModelRepository) UpdateModel(ctx context.Context, m *model.Model) error {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.UpdateModel")
	defer span.End()

	m.UpdatedAt = time.Now()
	if err := m.MarshalMetadata(); err != nil {
		return err
	}

	query := `
		UPDATE models SET name = $1, model_type = $2, description = $3, metadata = $4, updated_at = $5
		WHERE model_id = $6 AND tenant_id = $7
	`

	result, err := r.db.ExecContext(ctx, query,
		m.Name, m.ModelType, m.Description, m.MetadataJSON, m.UpdatedAt, m.ModelID, m.TenantID,
	)
	if err != nil {
		r.logger.Error("Failed to update model", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update model")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.Newf(errors.ErrCodeModelNotFound, "model not found: %s", m.ModelID)
	}
	return nil
}

// DeleteModel 删除模型
func (r *ModelRepository) DeleteModel(ctx context.Context, tenantID, modelID string) error {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.DeleteModel")
	defer span.End()

	query := `DELETE FROM models WHERE model_id = $1 AND tenant_id = $2`
	result, err := r.db.ExecContext(ctx, query, modelID, tenantID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to delete model")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.Newf(errors.ErrCodeModelNotFound, "model not found: %s", modelID)
	}
	return nil
}

// CreateModelAction persists a queued model action and its audit row in one
// transaction. Returning 202 is therefore impossible when audit persistence
// fails.
func (r *ModelRepository) CreateModelAction(ctx context.Context, job *model.ModelActionJob, auditAction, ipAddr, userAgent string) error {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal model action payload")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin model action transaction")
	}
	defer tx.Rollback()

	if job.Action == "request-retraining" {
		lockKey := fmt.Sprintf("model-retrain:%s:%s", job.TenantID, job.ModelID)
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock model retraining request")
		}
		var duplicate bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM model_action_jobs
				WHERE tenant_id = $1 AND model_id = $2::uuid
				  AND action = 'request-retraining' AND status IN ('queued', 'running')
			)
		`, job.TenantID, job.ModelID).Scan(&duplicate); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to check duplicate model retraining request")
		}
		if duplicate {
			return errors.New(errors.ErrCodeInvalidStateTransition, "a retraining job is already queued or running for this model")
		}
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO model_action_jobs (
			job_id, action_id, tenant_id, model_id, version, action, target,
			payload, status, requested_by, created_at
		)
		SELECT $1, $2, $3, model_id, $5, $6, $7, $8::jsonb, $9, $10, $11
		FROM models
		WHERE model_id = $4::uuid AND tenant_id = $3
	`, job.JobID, job.ActionID, job.TenantID, job.ModelID, job.Version, job.Action,
		job.Target, string(payload), job.Status, job.RequestedBy, job.CreatedAt)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create model action job")
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.Newf(errors.ErrCodeModelNotFound, "model not found for tenant: %s", job.ModelID)
	}

	detail, err := json.Marshal(map[string]interface{}{
		"action_id": job.ActionID,
		"job_id":    job.JobID,
		"version":   job.Version,
		"target":    job.Target,
		"status":    job.Status,
	})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal model action audit detail")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, $3, 'model', $4, $5::jsonb, $6, $7)
	`, job.TenantID, job.RequestedBy, auditAction, job.ModelID, string(detail), ipAddr, userAgent); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist model action audit")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model action transaction")
	}
	return nil
}

func (r *ModelRepository) ListModelWorkbenchItems(ctx context.Context, tenantID, modelID string) ([]*model.ModelWorkbenchItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT item_id, tenant_id, model_id::text, category, ordinal, payload, scenario_id, occurred_at
		FROM model_workbench_items
		WHERE tenant_id = $1 AND model_id = $2::uuid
		ORDER BY category, ordinal, occurred_at DESC
	`, tenantID, modelID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query model workbench items")
	}
	defer rows.Close()
	items := make([]*model.ModelWorkbenchItem, 0)
	for rows.Next() {
		item := &model.ModelWorkbenchItem{}
		if err := rows.Scan(&item.ItemID, &item.TenantID, &item.ModelID, &item.Category, &item.Ordinal, &item.Payload, &item.ScenarioID, &item.OccurredAt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model workbench item")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate model workbench items")
	}
	return items, nil
}

func (r *ModelRepository) ListModelActions(ctx context.Context, tenantID, modelID string, limit int) ([]*model.ModelActionJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT job_id, action_id, tenant_id, model_id::text, version, action, target,
		       payload, status, requested_by, created_at
		FROM model_action_jobs
		WHERE tenant_id = $1 AND model_id = $2::uuid
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, modelID, limit)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query model actions")
	}
	defer rows.Close()
	actions := make([]*model.ModelActionJob, 0)
	for rows.Next() {
		job := &model.ModelActionJob{}
		var payload []byte
		if err := rows.Scan(&job.JobID, &job.ActionID, &job.TenantID, &job.ModelID, &job.Version, &job.Action,
			&job.Target, &payload, &job.Status, &job.RequestedBy, &job.CreatedAt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model action")
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &job.Payload); err != nil {
				return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode model action payload")
			}
		}
		actions = append(actions, job)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate model actions")
	}
	return actions, nil
}

// ClaimNextModelAction atomically assigns one queued job to this worker. The
// SKIP LOCKED claim is safe when rule-manager has multiple replicas.
func (r *ModelRepository) ClaimNextModelAction(ctx context.Context) (*model.ModelActionJob, error) {
	job := &model.ModelActionJob{}
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
		WITH next_job AS (
			SELECT job_id
			FROM model_action_jobs
			WHERE status = 'queued'
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE model_action_jobs AS jobs
		SET status = 'running', updated_at = now()
		FROM next_job
		WHERE jobs.job_id = next_job.job_id
		RETURNING jobs.job_id, jobs.action_id, jobs.tenant_id, jobs.model_id::text,
		          jobs.version, jobs.action, jobs.target, jobs.payload, jobs.status,
		          jobs.requested_by, jobs.created_at
	`).Scan(&job.JobID, &job.ActionID, &job.TenantID, &job.ModelID, &job.Version,
		&job.Action, &job.Target, &payload, &job.Status, &job.RequestedBy, &job.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to claim model action")
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &job.Payload); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode claimed model action")
		}
	}
	return job, nil
}

func (r *ModelRepository) RecoverStaleModelActions(ctx context.Context, age time.Duration) error {
	if age <= 0 {
		age = 5 * time.Minute
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE model_action_jobs
		SET status = 'queued', updated_at = now()
		WHERE status = 'running' AND updated_at < now() - ($1 * interval '1 second')
		  AND NOT EXISTS (
			SELECT 1 FROM model_update_outbox
			WHERE action_job_id = model_action_jobs.job_id
			  AND status IN ('pending', 'processing')
		  )
	`, age.Seconds())
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to recover stale model actions")
	}
	return nil
}

// FinishModelAction persists the terminal job state and matching audit row in
// one transaction. For asynchronous MLOps actions, completed means the durable
// Kafka dispatch was acknowledged, not that training itself finished.
func (r *ModelRepository) FinishModelAction(ctx context.Context, job *model.ModelActionJob, status, auditAction, failure string) error {
	if status != "completed" && status != "failed" {
		return errors.New(errors.ErrCodeInvalidParameter, "invalid model action terminal status")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin model action completion")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE model_action_jobs SET status = $1, updated_at = now()
		WHERE job_id = $2 AND status = 'running'
	`, status, job.JobID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to finish model action")
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New(errors.ErrCodeConcurrentModify, "model action is no longer running")
	}
	detail, err := json.Marshal(map[string]interface{}{
		"action_id": job.ActionID,
		"job_id":    job.JobID,
		"action":    job.Action,
		"version":   job.Version,
		"status":    status,
		"failure":   failure,
		"stage":     "dispatcher",
	})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal model action completion audit")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail)
		VALUES ($1, $2, $3, 'model', $4, $5::jsonb)
	`, job.TenantID, job.RequestedBy, auditAction, job.ModelID, string(detail)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist model action completion audit")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model action completion")
	}
	return nil
}

// ListModels 列出模型
func (r *ModelRepository) ListModels(ctx context.Context, tenantID string, filter *model.ModelFilter) ([]*model.Model, int64, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.ListModels")
	defer span.End()

	if filter == nil {
		filter = &model.ModelFilter{}
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	baseWhere := `WHERE tenant_id = $1`
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.ModelType != "" {
		baseWhere += fmt.Sprintf(" AND model_type = $%d", argIdx)
		args = append(args, filter.ModelType)
		argIdx++
	}
	if filter.Keyword != "" {
		baseWhere += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filter.Keyword+"%")
		argIdx++
	}

	// 获取总数
	var total int64
	countQuery := "SELECT COUNT(*) FROM models " + baseWhere
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count models")
	}

	// 查询数据
	orderBy := "created_at DESC"
	if filter.OrderBy == "name" {
		orderBy = "name ASC"
	} else if filter.OrderBy == "updated_at" {
		orderBy = "updated_at DESC"
	}

	selectQuery := `SELECT model_id, tenant_id, name, model_type, description, metadata, created_at, updated_at
		FROM models ` + baseWhere + ` ORDER BY ` + orderBy

	selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query models")
	}
	defer rows.Close()

	var models []*model.Model
	for rows.Next() {
		var m model.Model
		var description sql.NullString
		if err := rows.Scan(&m.ModelID, &m.TenantID, &m.Name, &m.ModelType,
			&description, &m.MetadataJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model")
		}
		if description.Valid {
			m.Description = description.String
		}
		_ = m.UnmarshalMetadata()
		models = append(models, &m)
	}

	return models, total, nil
}

// =============================================================================
// 模型版本 CRUD
// =============================================================================

// CreateModelVersion 创建模型版本
func (r *ModelRepository) CreateModelVersion(ctx context.Context, mv *model.ModelVersion) error {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.CreateModelVersion")
	defer span.End()

	now := time.Now()
	mv.CreatedAt = now
	mv.UpdatedAt = now
	mv.SetDefaults()

	if err := mv.MarshalMetrics(); err != nil {
		return err
	}

	query := `
		INSERT INTO model_versions (model_version, model_id, tenant_id, feature_set_id, artifact_uri,
		                           metrics, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
		        (SELECT user_id FROM users WHERE user_id::text = $8 AND tenant_id = $3 LIMIT 1),
		        $9, $10)
		ON CONFLICT (model_version) DO NOTHING
	`

	result, err := r.db.ExecContext(ctx, query,
		mv.ModelVersion, mv.ModelID, mv.TenantID, mv.FeatureSetID,
		mv.ArtifactURI, mv.MetricsJSON, mv.Status, mv.CreatedBy,
		mv.CreatedAt, mv.UpdatedAt,
	)
	if err != nil {
		r.logger.Error("Failed to create model version", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create model version")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to confirm model version creation")
	}
	if rows != 1 {
		return errors.Newf(errors.ErrCodeVersionConflict, "model version already exists: %s", mv.ModelVersion)
	}
	return nil
}

// GetModelVersion 获取模型版本
func (r *ModelRepository) GetModelVersion(ctx context.Context, modelVersion string) (*model.ModelVersion, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.GetModelVersion")
	defer span.End()

	query := `
		SELECT mv.model_version, mv.model_id, mv.tenant_id, mv.feature_set_id,
		       mv.artifact_uri, mv.metrics, mv.status, mv.created_by,
		       mv.created_at, mv.updated_at,
		       m.name, m.model_type, m.description
		FROM model_versions mv
		LEFT JOIN models m ON m.model_id = mv.model_id
		WHERE mv.model_version = $1
	`

	var mv model.ModelVersion
	var createdBy, modelName, modelType, description sql.NullString

	err := r.db.QueryRowContext(ctx, query, modelVersion).Scan(
		&mv.ModelVersion, &mv.ModelID, &mv.TenantID, &mv.FeatureSetID,
		&mv.ArtifactURI, &mv.MetricsJSON, &mv.Status, &createdBy,
		&mv.CreatedAt, &mv.UpdatedAt,
		&modelName, &modelType, &description,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found: %s", modelVersion)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get model version")
	}

	if createdBy.Valid {
		mv.CreatedBy = createdBy.String
	}
	if modelName.Valid {
		mv.ModelName = modelName.String
	}
	if modelType.Valid {
		mv.ModelType = modelType.String
	}
	if description.Valid {
		mv.Description = description.String
	}
	_ = mv.UnmarshalMetrics()

	return &mv, nil
}

// ListModelVersions 列出模型的版本列表
func (r *ModelRepository) ListModelVersions(ctx context.Context, tenantID, modelID string, filter *model.ModelVersionFilter) ([]*model.ModelVersion, int64, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.ListModelVersions")
	defer span.End()

	if filter == nil {
		filter = &model.ModelVersionFilter{}
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	baseWhere := `WHERE mv.tenant_id = $1 AND mv.model_id = $2`
	args := []interface{}{tenantID, modelID}
	argIdx := 3

	if filter.Status != "" {
		baseWhere += fmt.Sprintf(" AND mv.status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}

	var total int64
	countQuery := `SELECT COUNT(*) FROM model_versions mv ` + baseWhere
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count model versions")
	}

	selectQuery := `
		SELECT mv.model_version, mv.model_id, mv.tenant_id, mv.feature_set_id,
		       mv.artifact_uri, mv.metrics, mv.status, mv.created_by,
		       mv.created_at, mv.updated_at,
		       m.name, m.model_type, m.description
		FROM model_versions mv
		LEFT JOIN models m ON m.model_id = mv.model_id
	` + baseWhere + ` ORDER BY mv.created_at DESC`

	selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query model versions")
	}
	defer rows.Close()

	var versions []*model.ModelVersion
	for rows.Next() {
		var mv model.ModelVersion
		var createdBy, modelName, modelType, description sql.NullString
		if err := rows.Scan(
			&mv.ModelVersion, &mv.ModelID, &mv.TenantID, &mv.FeatureSetID,
			&mv.ArtifactURI, &mv.MetricsJSON, &mv.Status, &createdBy,
			&mv.CreatedAt, &mv.UpdatedAt,
			&modelName, &modelType, &description,
		); err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model version")
		}
		if createdBy.Valid {
			mv.CreatedBy = createdBy.String
		}
		if modelName.Valid {
			mv.ModelName = modelName.String
		}
		if modelType.Valid {
			mv.ModelType = modelType.String
		}
		if description.Valid {
			mv.Description = description.String
		}
		_ = mv.UnmarshalMetrics()
		versions = append(versions, &mv)
	}

	return versions, total, nil
}

// GetActiveModelVersion 获取模型的当前激活版本
func (r *ModelRepository) GetActiveModelVersion(ctx context.Context, modelID string) (*model.ModelVersion, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.GetActiveModelVersion")
	defer span.End()

	query := `
		SELECT mv.model_version, mv.model_id, mv.tenant_id, mv.feature_set_id,
		       mv.artifact_uri, mv.metrics, mv.status, mv.created_by,
		       mv.created_at, mv.updated_at,
		       m.name, m.model_type, m.description
		FROM model_versions mv
		LEFT JOIN models m ON m.model_id = mv.model_id
		WHERE mv.model_id = $1 AND mv.status = 'active'
		ORDER BY mv.created_at DESC
		LIMIT 1
	`

	var mv model.ModelVersion
	var createdBy, modelName, modelType, description sql.NullString

	err := r.db.QueryRowContext(ctx, query, modelID).Scan(
		&mv.ModelVersion, &mv.ModelID, &mv.TenantID, &mv.FeatureSetID,
		&mv.ArtifactURI, &mv.MetricsJSON, &mv.Status, &createdBy,
		&mv.CreatedAt, &mv.UpdatedAt,
		&modelName, &modelType, &description,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeModelVersionNotFound, "no active version for model: %s", modelID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get active model version")
	}

	if createdBy.Valid {
		mv.CreatedBy = createdBy.String
	}
	if modelName.Valid {
		mv.ModelName = modelName.String
	}
	if modelType.Valid {
		mv.ModelType = modelType.String
	}
	if description.Valid {
		mv.Description = description.String
	}
	_ = mv.UnmarshalMetrics()

	return &mv, nil
}

// UpdateModelVersionStatus 更新模型版本状态
func (r *ModelRepository) UpdateModelVersionStatus(ctx context.Context, modelVersion string, status model.ModelStatus) error {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.UpdateModelVersionStatus")
	defer span.End()

	query := `UPDATE model_versions SET status = $1, updated_at = $2 WHERE model_version = $3`
	result, err := r.db.ExecContext(ctx, query, string(status), time.Now(), modelVersion)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update model version status")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found: %s", modelVersion)
	}
	return nil
}

// DeprecateOtherVersions 弃用模型的其他激活版本（激活新版本前调用）
func (r *ModelRepository) DeprecateOtherVersions(ctx context.Context, modelID, excludeVersion string) error {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.DeprecateOtherVersions")
	defer span.End()

	query := `UPDATE model_versions SET status = $1, updated_at = $2
		WHERE model_id = $3 AND status = 'active' AND model_version != $4`
	_, err := r.db.ExecContext(ctx, query, string(model.ModelStatusDeprecated), time.Now(), modelID, excludeVersion)
	return err
}

// GetModelSummary 获取模型摘要（含统计信息）
func (r *ModelRepository) GetModelSummary(ctx context.Context, tenantID, modelID string) (*model.ModelSummary, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.GetModelSummary")
	defer span.End()

	m, err := r.GetModel(ctx, modelID)
	if err != nil {
		return nil, err
	}

	summary := &model.ModelSummary{
		ModelID:   m.ModelID,
		Name:      m.Name,
		ModelType: m.ModelType,
	}

	// 获取激活版本
	activeVersion, _ := r.GetActiveModelVersion(ctx, modelID)
	if activeVersion != nil {
		summary.ActiveVersion = activeVersion.ModelVersion
		summary.Status = activeVersion.Status
		if f1, ok := activeVersion.GetF1Score(); ok {
			summary.BestF1Score = f1
		}
		summary.LastTrained = activeVersion.CreatedAt.Format(time.RFC3339)
	}

	// 统计总版本数
	countQuery := `SELECT COUNT(*) FROM model_versions WHERE model_id = $1`
	_ = r.db.QueryRowContext(ctx, countQuery, modelID).Scan(&summary.TotalVersions)

	return summary, nil
}

// =============================================================================
// 模型搜索
// =============================================================================

// SearchModels 搜索模型
func (r *ModelRepository) SearchModels(ctx context.Context, tenantID, query string, limit int) ([]*model.Model, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.SearchModels")
	defer span.End()

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	sqlQuery := `
		SELECT model_id, tenant_id, name, model_type, description, metadata, created_at, updated_at
		FROM models
		WHERE tenant_id = $1 AND (name ILIKE $2 OR description ILIKE $2)
		ORDER BY updated_at DESC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, sqlQuery, tenantID, "%"+query+"%", limit)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to search models")
	}
	defer rows.Close()

	var models []*model.Model
	for rows.Next() {
		var m model.Model
		var description sql.NullString
		if err := rows.Scan(&m.ModelID, &m.TenantID, &m.Name, &m.ModelType,
			&description, &m.MetadataJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model")
		}
		if description.Valid {
			m.Description = description.String
		}
		_ = m.UnmarshalMetadata()
		models = append(models, &m)
	}

	return models, nil
}

// ExportModel 导出模型的完整信息（含所有版本）
func (r *ModelRepository) ExportModel(ctx context.Context, modelID string) (map[string]interface{}, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.ExportModel")
	defer span.End()

	m, err := r.GetModel(ctx, modelID)
	if err != nil {
		return nil, err
	}

	// 获取所有版本
	versions, _, err := r.ListModelVersions(ctx, m.TenantID, modelID, &model.ModelVersionFilter{Limit: 100})
	if err != nil {
		return nil, err
	}

	export := map[string]interface{}{
		"model":       m,
		"versions":    versions,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
	}

	return export, nil
}

// ImportModelVersions 批量导入模型版本（从 JSON 导入）
func (r *ModelRepository) ImportModelVersions(ctx context.Context, versions []*model.ModelVersion) (int, error) {
	ctx, span := otel.StartSpan(ctx, "ModelRepository.ImportModelVersions")
	defer span.End()

	imported := 0
	for _, mv := range versions {
		if err := r.CreateModelVersion(ctx, mv); err != nil {
			r.logger.Warn("Failed to import model version",
				zap.String("model_version", mv.ModelVersion),
				zap.Error(err))
			continue
		}
		imported++
	}

	return imported, nil
}

// =============================================================================
// 模型 JSON 导入导出辅助
// =============================================================================

// ModelExport 模型导出结构
type ModelExport struct {
	Models     []*model.Model        `json:"models"`
	Versions   []*model.ModelVersion `json:"versions"`
	ExportedAt string                `json:"exported_at"`
}

// MarshalExport 序列化导出数据
func MarshalExport(export *ModelExport) ([]byte, error) {
	return json.Marshal(export)
}

// UnmarshalExport 反序列化导入数据
func UnmarshalExport(data []byte) (*ModelExport, error) {
	var export ModelExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("failed to unmarshal model export: %w", err)
	}
	return &export, nil
}

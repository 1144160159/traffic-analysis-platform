////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/api/handler_models.go
// Model Registry API Handler — MLOps 模型注册与版本管理
//
// REST API 端点:
//   GET    /api/v1/models                    - 列出模型
//   POST   /api/v1/models                    - 创建模型
//   GET    /api/v1/models/{id}               - 获取模型
//   PUT    /api/v1/models/{id}               - 更新模型
//   DELETE /api/v1/models/{id}               - 删除模型
//   GET    /api/v1/models/{id}/summary       - 模型摘要
//   GET    /api/v1/models/{id}/versions      - 列出模型版本
//   POST   /api/v1/models/{id}/versions      - 注册模型版本 (MLOps pipeline 调用)
//   GET    /api/v1/models/{id}/versions/{v}  - 获取模型版本详情
//   POST   /api/v1/models/{id}/versions/{v}/activate  - 激活模型版本
//   POST   /api/v1/models/{id}/versions/{v}/deprecate - 弃用模型版本
//   GET    /api/v1/models/{id}/versions/active        - 获取激活版本
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
)

// =============================================================================
// 路由注册（追加到 RegisterRoutes）
// =============================================================================

// RegisterModelRoutes 注册模型管理路由
func (h *Handler) RegisterModelRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()

	models := api.PathPrefix("/models").Subrouter()
	models.HandleFunc("", h.ListModels).Methods("GET")
	models.HandleFunc("", h.CreateModel).Methods("POST")
	models.HandleFunc("/{id}", h.GetModel).Methods("GET")
	models.HandleFunc("/{id}", h.UpdateModel).Methods("PUT")
	models.HandleFunc("/{id}", h.DeleteModel).Methods("DELETE")
	models.HandleFunc("/{id}/summary", h.GetModelSummary).Methods("GET")
	models.HandleFunc("/{id}/workbench", h.GetModelWorkbench).Methods("GET")
	models.HandleFunc("/{id}/versions", h.ListModelVersions).Methods("GET")
	models.HandleFunc("/{id}/versions", h.RegisterModelVersion).Methods("POST")
	models.HandleFunc("/{id}/versions/active", h.GetActiveModelVersion).Methods("GET")
	models.HandleFunc("/{id}/versions/{version}", h.GetModelVersion).Methods("GET")
	models.HandleFunc("/{id}/versions/{version}/activate", h.ActivateModelVersion).Methods("POST")
	models.HandleFunc("/{id}/versions/{version}/deprecate", h.DeprecateModelVersion).Methods("POST")
	models.HandleFunc("/{id}/feedback-samples", h.AppendModelFeedbackSamples).Methods("POST")
	models.HandleFunc("/{id}/retrain", h.RequestModelRetraining).Methods("POST")
	models.HandleFunc("/{id}/versions/{version}/evaluate", h.RequestModelEvaluation).Methods("POST")
	models.HandleFunc("/{id}/versions/{version}/rollback", h.RollbackModelVersion).Methods("POST")
	models.HandleFunc("/{id}/actions", h.SubmitModelContextAction).Methods("POST")
}

// =============================================================================
// 模型 CRUD Handlers
// =============================================================================

// CreateModel 创建模型
func (h *Handler) CreateModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	var req CreateModelRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	if err := req.Validate(); err != nil {
		h.writeError(w, r, err)
		return
	}

	m := &model.Model{
		TenantID:    opCtx.TenantID,
		Name:        req.Name,
		ModelType:   req.ModelType,
		Description: req.Description,
		Metadata:    req.Metadata,
	}

	if err := h.modelService.CreateModel(ctx, m, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, ModelResponse{
		Success: true,
		Data:    h.modelToDTO(m),
	})
}

// GetModel 获取模型
func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	m, err := h.modelService.GetModel(ctx, modelID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, ModelResponse{
		Success: true,
		Data:    h.modelToDTO(m),
	})
}

// UpdateModel 更新模型
func (h *Handler) UpdateModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	var req UpdateModelRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	m := &model.Model{
		ModelID:     modelID,
		TenantID:    opCtx.TenantID,
		Name:        req.Name,
		ModelType:   req.ModelType,
		Description: req.Description,
		Metadata:    req.Metadata,
	}

	if err := h.modelService.UpdateModel(ctx, m, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, ModelResponse{
		Success: true,
		Data:    h.modelToDTO(m),
	})
}

// DeleteModel 删除模型
func (h *Handler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	if err := h.modelService.DeleteModel(ctx, opCtx.TenantID, modelID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "model deleted successfully",
	})
}

// ListModels 列出模型
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	filter := &model.ModelFilter{
		ModelType: r.URL.Query().Get("model_type"),
		Keyword:   r.URL.Query().Get("keyword"),
		Limit:     h.parseIntParam(r, "limit", h.config.DefaultPageSize),
		Offset:    h.parseIntParam(r, "offset", 0),
		OrderBy:   r.URL.Query().Get("order_by"),
		OrderDir:  r.URL.Query().Get("order_dir"),
	}

	if filter.Limit > h.config.MaxPageSize {
		filter.Limit = h.config.MaxPageSize
	}

	models, total, err := h.modelService.ListModels(ctx, opCtx.TenantID, filter, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	dtos := make([]*ModelDTO, len(models))
	for i, m := range models {
		dto := h.modelToDTO(m)
		versions, _, versionErr := h.modelService.ListModelVersions(ctx, opCtx.TenantID, m.ModelID, &model.ModelVersionFilter{Limit: 100}, opCtx)
		if versionErr == nil && len(versions) > 0 {
			latest := versions[0]
			dto.ModelVersion = latest.ModelVersion
			dto.Status = latest.Status
			dto.Metrics = latest.Metrics
			for _, version := range versions {
				if version.Status == string(model.ModelStatusActive) && dto.ActiveVersion == "" {
					dto.ActiveVersion = version.ModelVersion
					continue
				}
				if version.Status == string(model.ModelStatusDeprecated) && dto.PreviousVersion == "" {
					dto.PreviousVersion = version.ModelVersion
				}
			}
		}
		dtos[i] = dto
	}

	h.writeJSON(w, http.StatusOK, ModelListResponse{
		Success: true,
		Data:    dtos,
		Pagination: PaginationInfo{
			Total:   total,
			Limit:   filter.Limit,
			Offset:  filter.Offset,
			HasMore: int64(filter.Offset+filter.Limit) < total,
		},
	})
}

// GetModelSummary 获取模型摘要
func (h *Handler) GetModelSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	summary, err := h.modelService.GetModelSummary(ctx, opCtx.TenantID, modelID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    summary,
	})
}

func (h *Handler) GetModelWorkbench(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}
	workbench, err := h.modelService.GetModelWorkbench(r.Context(), modelID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"model":    h.modelToDTO(workbench.Model),
			"versions": workbench.Versions,
			"items":    workbench.Items,
			"actions":  workbench.Actions,
			"source":   workbench.Source,
		},
	})
}

// =============================================================================
// 模型版本 Handlers
// =============================================================================

// RegisterModelVersion 注册模型版本（MLOps pipeline 调用入口）
func (h *Handler) RegisterModelVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]

	var req model.RegisterModelRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	// Path and authenticated context are authoritative even when the JSON body
	// attempts to name a different model or tenant.
	req.ModelID = modelID
	req.TenantID = opCtx.TenantID

	mv, err := h.modelService.RegisterModelVersion(ctx, &req, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, ModelVersionResponse{
		Success: true,
		Data:    h.modelVersionToDTO(mv),
	})
}

// GetModelVersion 获取模型版本详情
func (h *Handler) GetModelVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	version := mux.Vars(r)["version"]
	if version == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "version is required"))
		return
	}

	mv, err := h.modelService.GetModelVersion(ctx, version, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if modelID := mux.Vars(r)["id"]; modelID == "" || mv.ModelID != modelID {
		h.writeError(w, r, errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found under model: %s", modelID))
		return
	}

	h.writeJSON(w, http.StatusOK, ModelVersionResponse{
		Success: true,
		Data:    h.modelVersionToDTO(mv),
	})
}

// ListModelVersions 列出模型版本
func (h *Handler) ListModelVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	filter := &model.ModelVersionFilter{
		Status: r.URL.Query().Get("status"),
		Limit:  h.parseIntParam(r, "limit", h.config.DefaultPageSize),
		Offset: h.parseIntParam(r, "offset", 0),
	}

	if filter.Limit > h.config.MaxPageSize {
		filter.Limit = h.config.MaxPageSize
	}

	versions, total, err := h.modelService.ListModelVersions(ctx, opCtx.TenantID, modelID, filter, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	dtos := make([]*ModelVersionDTO, len(versions))
	for i, mv := range versions {
		dtos[i] = h.modelVersionToDTO(mv)
	}

	h.writeJSON(w, http.StatusOK, ModelVersionListResponse{
		Success: true,
		Data:    dtos,
		Pagination: PaginationInfo{
			Total:   total,
			Limit:   filter.Limit,
			Offset:  filter.Offset,
			HasMore: int64(filter.Offset+filter.Limit) < total,
		},
	})
}

// ActivateModelVersion 激活模型版本
func (h *Handler) ActivateModelVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	version := mux.Vars(r)["version"]
	if version == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "version is required"))
		return
	}
	modelID := mux.Vars(r)["id"]
	request := struct {
		GrayPercent int `json:"gray_percent"`
	}{GrayPercent: 100}
	if r.ContentLength != 0 {
		if err := h.decodeJSON(r, &request); err != nil {
			h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
			return
		}
	}

	if err := h.modelService.ActivateModelVersion(ctx, modelID, version, request.GrayPercent, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: fmt.Sprintf("model version activation accepted at %d%%", request.GrayPercent),
	})
}

// DeprecateModelVersion 弃用模型版本
func (h *Handler) DeprecateModelVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	version := mux.Vars(r)["version"]
	if version == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "version is required"))
		return
	}
	modelID := mux.Vars(r)["id"]

	if err := h.modelService.DeprecateModelVersion(ctx, modelID, version, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "model version deprecated",
	})
}

// GetActiveModelVersion 获取模型的激活版本
func (h *Handler) GetActiveModelVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	mv, err := h.modelService.GetActiveModelVersion(ctx, modelID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, ModelVersionResponse{
		Success: true,
		Data:    h.modelVersionToDTO(mv),
	})
}

// AppendModelFeedbackSamples queues a tenant-scoped feedback ingestion job.
func (h *Handler) AppendModelFeedbackSamples(w http.ResponseWriter, r *http.Request) {
	h.submitQueuedModelAction(w, r, "append-feedback-samples", rbac.PermModelWrite, "MODEL_FEEDBACK_INGEST_REQUESTED")
}

// RequestModelRetraining queues a tenant-scoped retraining job.
func (h *Handler) RequestModelRetraining(w http.ResponseWriter, r *http.Request) {
	h.submitQueuedModelAction(w, r, "request-retraining", rbac.PermModelWrite, "MODEL_RETRAIN_REQUESTED")
}

// RequestModelEvaluation queues a version evaluation job.
func (h *Handler) RequestModelEvaluation(w http.ResponseWriter, r *http.Request) {
	h.submitQueuedModelAction(w, r, "request-evaluation", rbac.PermModelWrite, "MODEL_EVALUATION_REQUESTED")
}

// RollbackModelVersion queues an explicitly audited rollback request. The
// worker applies the validated target version; this endpoint no longer claims
// a client-only success.
func (h *Handler) RollbackModelVersion(w http.ResponseWriter, r *http.Request) {
	h.submitQueuedModelAction(w, r, "rollback-version", rbac.PermModelActivate, "MODEL_VERSION_ROLLBACK_REQUESTED")
}

// SubmitModelContextAction queues a model workbench action chosen by the UI.
func (h *Handler) SubmitModelContextAction(w http.ResponseWriter, r *http.Request) {
	h.submitQueuedModelAction(w, r, "inspect-context", rbac.PermModelWrite, "MODEL_CONTEXT_ACTION_REQUESTED")
}

func (h *Handler) submitQueuedModelAction(w http.ResponseWriter, r *http.Request, action string, permission rbac.Permission, auditAction string) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	modelID := mux.Vars(r)["id"]
	if modelID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "model id is required"))
		return
	}

	payload := map[string]interface{}{}
	if err := h.decodeJSON(r, &payload); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}
	target := modelID
	if value, ok := payload["target"].(string); ok && value != "" {
		target = value
	}
	version := mux.Vars(r)["version"]
	if version == "" {
		if value, ok := payload["version"].(string); ok {
			version = value
		}
	}
	actionID, _ := payload["action_id"].(string)
	if value, ok := payload["action"].(string); ok && value != "" && auditAction == "MODEL_CONTEXT_ACTION_REQUESTED" {
		action = value
	}

	job, err := h.modelService.SubmitModelAction(r.Context(), modelID, &model.ModelActionRequest{
		ActionID: actionID,
		Action:   action,
		Target:   target,
		Version:  version,
		Payload:  payload,
	}, permission, auditAction, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": job})
}

// =============================================================================
// Request / Response DTOs
// =============================================================================

// CreateModelRequest API 请求
type CreateModelRequest struct {
	Name        string                 `json:"name"`
	ModelType   string                 `json:"model_type"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (r *CreateModelRequest) Validate() error {
	if r.Name == "" {
		return errors.New(errors.ErrCodeMissingParameter, "name is required")
	}
	if len(r.Name) > 256 {
		return errors.New(errors.ErrCodeInvalidParameter, "name too long, max 256 characters")
	}
	if r.ModelType == "" {
		return errors.New(errors.ErrCodeMissingParameter, "model_type is required")
	}
	if !model.IsValidModelType(r.ModelType) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid model_type: %s", r.ModelType)
	}
	return nil
}

type UpdateModelRequest struct {
	Name        string                 `json:"name"`
	ModelType   string                 `json:"model_type"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ModelDTO 模型 DTO
type ModelDTO struct {
	ModelID         string                 `json:"model_id"`
	TenantID        string                 `json:"tenant_id"`
	Name            string                 `json:"name"`
	ModelType       string                 `json:"model_type"`
	Description     string                 `json:"description,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	ModelVersion    string                 `json:"model_version,omitempty"`
	ActiveVersion   string                 `json:"active_version,omitempty"`
	PreviousVersion string                 `json:"previous_version,omitempty"`
	Status          string                 `json:"status,omitempty"`
	Metrics         map[string]interface{} `json:"metrics,omitempty"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
}

// ModelVersionDTO 模型版本 DTO
type ModelVersionDTO struct {
	ModelVersion string                 `json:"model_version"`
	ModelID      string                 `json:"model_id"`
	ModelName    string                 `json:"model_name,omitempty"`
	ModelType    string                 `json:"model_type,omitempty"`
	TenantID     string                 `json:"tenant_id"`
	FeatureSetID string                 `json:"feature_set_id"`
	ArtifactURI  string                 `json:"artifact_uri"`
	Metrics      map[string]interface{} `json:"metrics,omitempty"`
	Status       string                 `json:"status"`
	CreatedBy    string                 `json:"created_by,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

// ModelResponse 模型响应
type ModelResponse struct {
	Success bool      `json:"success"`
	Data    *ModelDTO `json:"data,omitempty"`
}

// ModelListResponse 模型列表响应
type ModelListResponse struct {
	Success    bool           `json:"success"`
	Data       []*ModelDTO    `json:"data"`
	Pagination PaginationInfo `json:"pagination"`
}

// ModelVersionResponse 模型版本响应
type ModelVersionResponse struct {
	Success bool             `json:"success"`
	Data    *ModelVersionDTO `json:"data,omitempty"`
}

// ModelVersionListResponse 模型版本列表响应
type ModelVersionListResponse struct {
	Success    bool               `json:"success"`
	Data       []*ModelVersionDTO `json:"data"`
	Pagination PaginationInfo     `json:"pagination"`
}

// =============================================================================
// DTO 转换
// =============================================================================

func (h *Handler) modelToDTO(m *model.Model) *ModelDTO {
	if m == nil {
		return nil
	}
	return &ModelDTO{
		ModelID:     m.ModelID,
		TenantID:    m.TenantID,
		Name:        m.Name,
		ModelType:   m.ModelType,
		Description: m.Description,
		Metadata:    m.Metadata,
		CreatedAt:   m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *Handler) modelVersionToDTO(mv *model.ModelVersion) *ModelVersionDTO {
	if mv == nil {
		return nil
	}
	f1, _ := mv.GetF1Score()
	metrics := mv.Metrics
	if metrics == nil {
		metrics = make(map[string]interface{})
	}
	if _, ok := metrics["f1_score"]; !ok {
		metrics["f1_score"] = f1
	}

	return &ModelVersionDTO{
		ModelVersion: mv.ModelVersion,
		ModelID:      mv.ModelID,
		ModelName:    mv.ModelName,
		ModelType:    mv.ModelType,
		TenantID:     mv.TenantID,
		FeatureSetID: mv.FeatureSetID,
		ArtifactURI:  mv.ArtifactURI,
		Metrics:      metrics,
		Status:       mv.Status,
		CreatedBy:    mv.CreatedBy,
		CreatedAt:    mv.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    mv.UpdatedAt.Format(time.RFC3339),
	}
}

// parseIntParam is defined in handler.go and reused here via the Handler receiver

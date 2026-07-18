////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/api/handler_orchestrator.go
// MLOps Orchestrator API — 自编排控制和状态查询
//
// REST API:
//   GET    /api/v1/mlops/status          - 编排器状态
//   POST   /api/v1/mlops/retrain         - 手动触发重训
//   GET    /api/v1/mlops/conditions      - 当前触发条件评估
//   POST   /api/v1/mlops/pause           - 暂停自动编排
//   POST   /api/v1/mlops/resume          - 恢复自动编排
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/service"
)

// RegisterOrchestratorRoutes 注册编排器路由
func (h *Handler) RegisterOrchestratorRoutes(r *mux.Router) {
	mlops := r.PathPrefix("/api/v1/mlops").Subrouter()
	mlops.HandleFunc("/status", h.GetOrchestratorStatus).Methods("GET")
	mlops.HandleFunc("/retrain", h.TriggerRetrain).Methods("POST")
	mlops.HandleFunc("/conditions", h.GetOrchestratorConditions).Methods("GET")
	mlops.HandleFunc("/workflows", h.ListMLOpsWorkflows).Methods("GET")
	mlops.HandleFunc("/workflows/{name}", h.GetMLOpsWorkflow).Methods("GET")
	mlops.HandleFunc("/workflows/{name}/retry", h.RetryMLOpsWorkflow).Methods("POST")
	mlops.HandleFunc("/workflows/{name}/stop", h.StopMLOpsWorkflow).Methods("POST")
	mlops.HandleFunc("/pause", h.PauseOrchestrator).Methods("POST")
	mlops.HandleFunc("/resume", h.ResumeOrchestrator).Methods("POST")
}

func (h *Handler) requireMLOpsPermission(opCtx *service.OperationContext, permission rbac.Permission) error {
	if opCtx == nil {
		return errors.New(errors.ErrCodeUnauthorized, "operation context required")
	}
	if h.rbacChecker != nil && !h.rbacChecker.HasPermission(opCtx.Permissions, permission) {
		return errors.Newf(errors.ErrCodePermissionDenied, "permission denied: %s required", permission)
	}
	return nil
}

func requireMLOpsWorkflowTenant(opCtx *service.OperationContext, workflow *service.MLOpsWorkflow) error {
	if opCtx == nil || workflow == nil {
		return errors.New(errors.ErrCodePermissionDenied, "workflow tenant scope is required")
	}
	tenantID := strings.TrimSpace(workflow.Parameters["tenant-id"])
	if tenantID == "" || tenantID != opCtx.TenantID {
		return errors.New(errors.ErrCodePermissionDenied, "cross-tenant MLOps workflow access denied")
	}
	return nil
}

// =============================================================================
// Orchestrator Handlers
// =============================================================================

// GetOrchestratorStatus 获取编排器状态
func (h *Handler) GetOrchestratorStatus(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelRead); err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"status":  "not_configured",
				"message": "MLOps orchestrator is not enabled",
			},
		})
		return
	}

	status := h.mlopsOrchestrator.GetStatus()
	if workflows, listErr := h.mlopsOrchestrator.ListWorkflows(r.Context()); listErr == nil {
		running := 0
		workflowCount := 0
		for _, workflow := range workflows {
			if requireMLOpsWorkflowTenant(opCtx, &workflow) != nil {
				continue
			}
			workflowCount++
			if workflow.Phase == "Pending" || workflow.Phase == "Running" {
				running++
			}
		}
		status["running_workflows"] = running
		status["workflow_count"] = workflowCount
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    status,
	})
}

// TriggerRetrain 手动触发模型重训
func (h *Handler) TriggerRetrain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelWrite); err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}

	var req service.ManualRetrainRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}
	req.TenantID = opCtx.TenantID
	if err := service.ValidateManualRetrainParameters(req.Params); err != nil {
		h.writeError(w, r, err)
		return
	}
	modelID := ""
	if req.Params != nil {
		modelID = strings.TrimSpace(fmt.Sprint(req.Params["model-id"]))
	}
	if modelID != "" {
		if _, err := h.modelService.GetModel(ctx, modelID, opCtx); err != nil {
			h.writeError(w, r, err)
			return
		}
	}

	req.WorkflowName = service.NewManualMLOpsWorkflowName()
	intentAction := "MLOPS_RETRAIN_SUBMIT_REQUESTED"
	workflowName, intentEventID, err := runAfterRequiredMLOpsAuditIntent(ctx, h.modelService, opCtx, intentAction, req.WorkflowName, map[string]interface{}{
		"model_id": modelID, "tenant_id": opCtx.TenantID, "feature_set_id": req.FeatureSetID,
	}, func() (string, error) {
		return h.mlopsOrchestrator.TriggerManualRetrain(ctx, &req)
	})
	if err != nil {
		if intentEventID != "" {
			_ = h.modelService.RecordMLOpsWorkflowAudit(ctx, opCtx, "MLOPS_RETRAIN_SUBMIT", req.WorkflowName, map[string]interface{}{"intent_event_id": intentEventID}, err)
		}
		h.writeError(w, r, err)
		return
	}
	auditEvent, auditCompletionPending, completionAuditErr := recordMLOpsCompletionAfterIntent(ctx, h.modelService, opCtx, "MLOPS_RETRAIN_SUBMITTED", intentAction, workflowName, map[string]interface{}{
		"model_id": modelID, "tenant_id": opCtx.TenantID, "feature_set_id": req.FeatureSetID, "intent_event_id": intentEventID,
	})
	if completionAuditErr != nil {
		// The required pre-mutation intent already exists. Returning an error
		// here would invite a duplicate workflow submission, so surface the
		// durable requested event and mark completion reconciliation pending.
		h.logger.Error("MLOps submit completion audit failed after durable intent", zap.Error(completionAuditErr), zap.String("workflow", workflowName), zap.String("intent_event_id", intentEventID))
	}

	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"workflow_name":            workflowName,
			"status":                   "submitted",
			"audit_event":              auditEvent,
			"audit_intent_event_id":    intentEventID,
			"audit_completion_pending": auditCompletionPending,
		},
		"message": "retraining workflow submitted",
	})
}

// GetOrchestratorConditions 获取当前触发条件评估
func (h *Handler) GetOrchestratorConditions(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelRead); err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}

	status := h.mlopsOrchestrator.GetStatus()

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"status": status,
			"triggers": []map[string]interface{}{
				{"name": "feedback", "description": "Feedback accumulation threshold reached"},
				{"name": "fp_rate", "description": "False positive rate exceeds limit"},
				{"name": "drift", "description": "Feature distribution drift detected (PSI > 0.25)"},
				{"name": "scheduled", "description": "Weekly cron schedule (Sunday 02:00)"},
				{"name": "manual", "description": "Manual trigger via API"},
			},
		},
	})
}

func (h *Handler) ListMLOpsWorkflows(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelRead); err != nil {
		h.writeError(w, r, err)
		return
	}
	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}
	workflows, err := h.mlopsOrchestrator.ListWorkflows(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	tenantWorkflows := make([]service.MLOpsWorkflow, 0, len(workflows))
	for index := range workflows {
		if requireMLOpsWorkflowTenant(opCtx, &workflows[index]) == nil {
			tenantWorkflows = append(tenantWorkflows, workflows[index])
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": tenantWorkflows})
}

func (h *Handler) GetMLOpsWorkflow(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelRead); err != nil {
		h.writeError(w, r, err)
		return
	}
	workflow, err := h.mlopsOrchestrator.GetWorkflow(r.Context(), mux.Vars(r)["name"])
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := requireMLOpsWorkflowTenant(opCtx, workflow); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": workflow})
}

func (h *Handler) RetryMLOpsWorkflow(w http.ResponseWriter, r *http.Request) {
	h.mutateMLOpsWorkflow(w, r, "retry", "MLOPS_WORKFLOW_RESUBMITTED")
}

func (h *Handler) StopMLOpsWorkflow(w http.ResponseWriter, r *http.Request) {
	h.mutateMLOpsWorkflow(w, r, "stop", "MLOPS_WORKFLOW_STOP_REQUESTED")
}

func (h *Handler) mutateMLOpsWorkflow(w http.ResponseWriter, r *http.Request, action, auditEvent string) {
	ctx := r.Context()
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelWrite); err != nil {
		h.writeError(w, r, err)
		return
	}
	name := mux.Vars(r)["name"]
	currentWorkflow, err := h.mlopsOrchestrator.GetWorkflow(ctx, name)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := requireMLOpsWorkflowTenant(opCtx, currentWorkflow); err != nil {
		h.writeError(w, r, err)
		return
	}
	intentAction := "MLOPS_WORKFLOW_STOP_INTENT"
	if action == "retry" {
		intentAction = "MLOPS_WORKFLOW_RESUBMIT_INTENT"
	}
	workflow, intentEventID, err := runAfterRequiredMLOpsAuditIntent(ctx, h.modelService, opCtx, intentAction, name, map[string]interface{}{
		"action": action, "source_workflow": name, "source_phase": currentWorkflow.Phase,
	}, func() (*service.MLOpsWorkflow, error) {
		if action == "retry" {
			return h.mlopsOrchestrator.RetryWorkflow(ctx, name)
		}
		return h.mlopsOrchestrator.StopWorkflow(ctx, name)
	})
	if err != nil {
		if intentEventID != "" {
			_ = h.modelService.RecordMLOpsWorkflowAudit(ctx, opCtx, auditEvent, name, map[string]interface{}{"intent_event_id": intentEventID}, err)
		}
		h.writeError(w, r, err)
		return
	}
	auditWorkflowName := name
	if action == "retry" && workflow.Name != "" {
		auditWorkflowName = workflow.Name
	}
	phase := workflow.Phase
	if action == "retry" || strings.TrimSpace(phase) == "" {
		phase = "Submitted"
	}
	responseAuditEvent, auditCompletionPending, completionAuditErr := recordMLOpsCompletionAfterIntent(ctx, h.modelService, opCtx, auditEvent, intentAction, auditWorkflowName, map[string]interface{}{
		"phase": phase, "action": action, "source_workflow": name, "intent_event_id": intentEventID,
	})
	if completionAuditErr != nil {
		h.logger.Error("MLOps mutation completion audit failed after durable intent", zap.Error(completionAuditErr), zap.String("workflow", auditWorkflowName), zap.String("intent_event_id", intentEventID))
	}
	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"workflow": workflow, "workflow_name": auditWorkflowName, "source_workflow": name, "status": phase, "audit_event": responseAuditEvent,
			"audit_intent_event_id": intentEventID, "audit_completion_pending": auditCompletionPending,
		},
	})
}

func runAfterRequiredMLOpsAuditIntent[T any](ctx context.Context, modelService *service.ModelService, opCtx *service.OperationContext, intentAction, workflowName string, detail map[string]interface{}, mutation func() (T, error)) (T, string, error) {
	var zero T
	intentEventID, err := modelService.RecordMLOpsWorkflowAuditIntent(ctx, opCtx, intentAction, workflowName, detail)
	if err != nil {
		return zero, "", err
	}
	result, err := mutation()
	return result, intentEventID, err
}

func recordMLOpsCompletionAfterIntent(ctx context.Context, modelService *service.ModelService, opCtx *service.OperationContext, completionAction, intentAction, workflowName string, detail map[string]interface{}) (string, bool, error) {
	if err := modelService.RecordMLOpsWorkflowAudit(ctx, opCtx, completionAction, workflowName, detail, nil); err != nil {
		return intentAction, true, err
	}
	return completionAction, false, nil
}

// PauseOrchestrator 暂停自动编排
func (h *Handler) PauseOrchestrator(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelWrite); err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}

	_, intentEventID, err := runAfterRequiredMLOpsAuditIntent(r.Context(), h.modelService, opCtx, "MLOPS_ORCHESTRATOR_PAUSE_INTENT", "orchestrator", map[string]interface{}{"requested_status": "paused"}, func() (struct{}, error) {
		h.mlopsOrchestrator.Stop()
		return struct{}{}, nil
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	_, completionPending, completionAuditErr := recordMLOpsCompletionAfterIntent(r.Context(), h.modelService, opCtx, "MLOPS_ORCHESTRATOR_PAUSED", "MLOPS_ORCHESTRATOR_PAUSE_INTENT", "orchestrator", map[string]interface{}{"status": "paused", "intent_event_id": intentEventID})
	if completionAuditErr != nil {
		h.logger.Error("MLOps pause completion audit failed after durable intent", zap.Error(completionAuditErr), zap.String("intent_event_id", intentEventID))
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "MLOps orchestrator paused",
		"data":    map[string]interface{}{"audit_intent_event_id": intentEventID, "audit_completion_pending": completionPending},
	})
}

// ResumeOrchestrator 恢复自动编排（需要重启 workflow controller）
func (h *Handler) ResumeOrchestrator(w http.ResponseWriter, r *http.Request) {
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.requireMLOpsPermission(opCtx, rbac.PermModelWrite); err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}

	resumeErr := errors.New(errors.ErrCodeInvalidStateTransition, "orchestrator resume requires a controlled rule-manager rollout")
	_ = h.modelService.RecordMLOpsWorkflowAudit(r.Context(), opCtx, "MLOPS_ORCHESTRATOR_RESUME", "orchestrator", nil, resumeErr)
	h.writeError(w, r, resumeErr)
}

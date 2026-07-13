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
	"net/http"

	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/service"
)

// RegisterOrchestratorRoutes 注册编排器路由
func (h *Handler) RegisterOrchestratorRoutes(r *mux.Router) {
	mlops := r.PathPrefix("/api/v1/mlops").Subrouter()
	mlops.HandleFunc("/status", h.GetOrchestratorStatus).Methods("GET")
	mlops.HandleFunc("/retrain", h.TriggerRetrain).Methods("POST")
	mlops.HandleFunc("/conditions", h.GetOrchestratorConditions).Methods("GET")
	mlops.HandleFunc("/pause", h.PauseOrchestrator).Methods("POST")
	mlops.HandleFunc("/resume", h.ResumeOrchestrator).Methods("POST")
}

// =============================================================================
// Orchestrator Handlers
// =============================================================================

// GetOrchestratorStatus 获取编排器状态
func (h *Handler) GetOrchestratorStatus(w http.ResponseWriter, r *http.Request) {
	_, err := h.extractOperationContext(r)
	if err != nil {
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

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    h.mlopsOrchestrator.GetStatus(),
	})
}

// TriggerRetrain 手动触发模型重训
func (h *Handler) TriggerRetrain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, err := h.extractOperationContext(r)
	if err != nil {
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

	workflowName, err := h.mlopsOrchestrator.TriggerManualRetrain(ctx, &req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"success":       true,
		"workflow_name": workflowName,
		"message":       "retraining workflow submitted",
	})
}

// GetOrchestratorConditions 获取当前触发条件评估
func (h *Handler) GetOrchestratorConditions(w http.ResponseWriter, r *http.Request) {
	_, err := h.extractOperationContext(r)
	if err != nil {
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
			"status":    status,
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

// PauseOrchestrator 暂停自动编排
func (h *Handler) PauseOrchestrator(w http.ResponseWriter, r *http.Request) {
	_, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}

	h.mlopsOrchestrator.Stop()

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "MLOps orchestrator paused",
	})
}

// ResumeOrchestrator 恢复自动编排（需要重启 workflow controller）
func (h *Handler) ResumeOrchestrator(w http.ResponseWriter, r *http.Request) {
	_, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	if h.mlopsOrchestrator == nil {
		h.writeError(w, r, errors.New(errors.ErrCodeServiceUnavailable, "MLOps orchestrator is not enabled"))
		return
	}

	// 恢复需要重启服务（编排器在 goroutine 中运行）
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "MLOps orchestrator resumed — requires service restart to take effect",
	})
}

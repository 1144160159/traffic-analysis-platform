// Package api 薄 HTTP 层:decode/auth/幂等/状态码;业务逻辑在 service。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// Handler 调度中心 API(前缀 /api/v1/analysis)。
type Handler struct {
	triggers   *service.TriggerService
	finalizer  *service.FinalizerService
	cancel     *service.CancelService
	scheduler  *service.Scheduler
	reports    *service.HumanReportService
	planAuthor *service.PlanAuthorService
	schedules  *service.ScheduleService
	retry      *service.RetryService
	taskDefs   *service.TaskDefinitionService
	preflight  *service.PreflightService
	retryTask  *service.RetryTaskService
	resources  *service.ResourceService
	allowed    *service.AllowedActionsService
	results    *RunResultsClient
	repo       RunReader
	logger     *zap.Logger
	// tenantPrincipal 由部署层注入(鉴权中间件);核心卷内为显式函数,便于测试与后续接中间件。
	tenantPrincipal func(r *http.Request) string
}

// RunReader 读接口仓储端口(依赖倒置,便于 handler 测试)。
type RunReader interface {
	GetRun(ctx context.Context, tenantID, runID string) (*repository.RunView, error)
	ListRunStages(ctx context.Context, tenantID, runID string) ([]repository.RunStageView, error)
	GetReport(ctx context.Context, tenantID, reportID string) (*repository.ReportView, error)
	ListRuns(ctx context.Context, p repository.ListRunsParams) ([]repository.RunView, error)
	ListTasks(ctx context.Context, tenantID string, limit, offset int) ([]repository.TaskView, error)
	ListTaskDefinitions(ctx context.Context, tenantID string) ([]repository.TaskDefinitionView, error)
	GetRunSummaryContent(ctx context.Context, tenantID, runID string) (*repository.RunSummaryContent, error)
	ListReports(ctx context.Context, tenantID string) ([]repository.ReportListView, error)
}

func NewHandler(t *service.TriggerService, f *service.FinalizerService, c *service.CancelService, sch *service.Scheduler, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		triggers: t, finalizer: f, cancel: c, scheduler: sch, logger: logger,
		tenantPrincipal: func(r *http.Request) string { return strings.TrimSpace(r.Header.Get("X-Tenant-ID")) },
	}
}

// SetReportService 注入报告服务(P4)。
func (h *Handler) SetReportService(svc *service.HumanReportService) { h.reports = svc }

// SetRunReader 注入读仓储(P4)。
func (h *Handler) SetRunReader(r RunReader) { h.repo = r }

// SetPlanAuthorService 注入人工选择列车服务(P2)。
func (h *Handler) SetPlanAuthorService(svc *service.PlanAuthorService) { h.planAuthor = svc }

// SetScheduleService 注入调度修订权威服务(§76.45.1)。
func (h *Handler) SetScheduleService(svc *service.ScheduleService) { h.schedules = svc }

// SetRetryService 注入节点级重试服务(§76.47.3)。
func (h *Handler) SetRetryService(svc *service.RetryService) { h.retry = svc }

// SetTaskDefinitionService 注入任务定义权威服务(§20 任务管理)。
func (h *Handler) SetTaskDefinitionService(svc *service.TaskDefinitionService) { h.taskDefs = svc }

// SetPreflightService 注入即时分析预检服务(§20 运行监控)。
func (h *Handler) SetPreflightService(svc *service.PreflightService) { h.preflight = svc }

// SetRetryTaskService 注入整 Run 重试服务(§76.47.3 RetryTask)。
func (h *Handler) SetRetryTaskService(svc *service.RetryTaskService) { h.retryTask = svc }

// SetResourceService 注入调度资源视图服务(§20 调度资源)。
func (h *Handler) SetResourceService(svc *service.ResourceService) { h.resources = svc }

// SetRunResultsClient 注入 CH 结果只读客户端(未注入时 GET /runs/{id}/results fail-closed 503)。
func (h *Handler) SetRunResultsClient(c *RunResultsClient) { h.results = c }

// SetAllowedActionsService 注入动作授权服务(§20/§21 服务端驱动)。
func (h *Handler) SetAllowedActionsService(svc *service.AllowedActionsService) { h.allowed = svc }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/analysis/health", h.health)
	mux.HandleFunc("/api/v1/analysis/triggers", h.submitTrigger)
	mux.HandleFunc("/api/v1/analysis/triggers/preflight", h.preflightTrigger)
	mux.HandleFunc("/api/v1/analysis/runs", h.listRuns)
	mux.HandleFunc("/api/v1/analysis/tasks", h.listTasks)
	mux.HandleFunc("/api/v1/analysis/tasks/", h.taskPlanDispatch) // POST /tasks/{id}/plans
	mux.HandleFunc("/api/v1/analysis/plans/", h.planDispatch)     // POST /plans/{id}/approve
	mux.HandleFunc("/api/v1/analysis/reports", h.reportDispatch)
	mux.HandleFunc("/api/v1/analysis/reports/", h.reportDispatch)
	mux.HandleFunc("/api/v1/analysis/schedules", h.scheduleDispatch)
	mux.HandleFunc("/api/v1/analysis/schedules/", h.scheduleDispatch) // POST /{id}/activate|pause、GET /{id}/triggers
	mux.HandleFunc("/api/v1/analysis/runs/", h.runDispatch)           // /runs/{id} 与子资源
	mux.HandleFunc("/api/v1/analysis/task-definitions", h.taskDefinitionDispatch)
	mux.HandleFunc("/api/v1/analysis/task-definitions/", h.taskDefinitionDispatch)
	mux.HandleFunc("/api/v1/analysis/resources", h.resourceDispatch)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// submitTrigger 即时分析提交(POST /triggers)。
func (h *Handler) submitTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	var req struct {
		TaskDefinitionID     string          `json:"task_definition_id"`
		PlanSource           string          `json:"plan_source"`
		CustomOverrides      json.RawMessage `json:"custom_overrides"`
		SourceKind           string          `json:"source_kind"`
		SourceSpec           json.RawMessage `json:"source_spec"`
		ClientIdempotencyKey string          `json:"client_idempotency_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	// 核心卷简化:目录/模板从 service 装配侧注入(真实环境接目录缓存)。
	resp, err := h.triggers.Submit(r.Context(), service.SubmitRequest{
		TenantID:             tenant,
		TaskDefinitionID:     req.TaskDefinitionID,
		PlanSource:           req.PlanSource,
		CustomOverrides:      req.CustomOverrides,
		SourceKind:           req.SourceKind,
		SourceSpec:           req.SourceSpec,
		ClientIdempotencyKey: req.ClientIdempotencyKey,
		Actor:                tenant,
		Approved:             true,
		CustomReleased:       true,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"task_id": resp.TaskID, "run_id": resp.RunID,
			"status_url": resp.StatusURL, "execution_spec_sha256": resp.ExecutionSpecSHA256,
		},
	})
}

// preflightTrigger 即时分析预检(POST /triggers/preflight):只解析+编译,不物化。
func (h *Handler) preflightTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.preflight == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "preflight service is not configured")
		return
	}
	var req struct {
		TaskDefinitionID     string          `json:"task_definition_id"`
		PlanSource           string          `json:"plan_source"`
		CustomOverrides      json.RawMessage `json:"custom_overrides"`
		SourceKind           string          `json:"source_kind"`
		SourceSpec           json.RawMessage `json:"source_spec"`
		ClientIdempotencyKey string          `json:"client_idempotency_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	resp, err := h.preflight.Preflight(r.Context(), service.SubmitRequest{
		TenantID:             tenant,
		TaskDefinitionID:     req.TaskDefinitionID,
		PlanSource:           req.PlanSource,
		CustomOverrides:      req.CustomOverrides,
		SourceKind:           req.SourceKind,
		SourceSpec:           req.SourceSpec,
		ClientIdempotencyKey: req.ClientIdempotencyKey,
		Actor:                tenant,
		Approved:             true,
		CustomReleased:       true,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": resp})
}

// runDispatch 运行子资源路由:/runs/{id}、/runs/{id}:cancel、/runs/{id}/report、
// /retry-stage、/retry-task、/summary、/receipts、/results。
func (h *Handler) runDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/runs/")
	switch {
	case strings.HasSuffix(path, ":cancel"):
		h.cancelRun(w, r, strings.TrimSuffix(path, ":cancel"))
	case strings.HasSuffix(path, "/report"):
		h.requestReport(w, r, strings.TrimSuffix(path, "/report"))
	case strings.HasSuffix(path, "/retry-stage"):
		h.retryStage(w, r, strings.TrimSuffix(path, "/retry-stage"))
	case strings.HasSuffix(path, "/retry-task"):
		h.retryTaskRun(w, r, strings.TrimSuffix(path, "/retry-task"))
	case strings.HasSuffix(path, "/summary"):
		h.getRunSummary(w, r, strings.TrimSuffix(path, "/summary"))
	case strings.HasSuffix(path, "/receipts"):
		h.getRunReceipts(w, r, strings.TrimSuffix(path, "/receipts"))
	case strings.HasSuffix(path, "/results"):
		h.getRunResults(w, r, strings.TrimSuffix(path, "/results"))
	case strings.HasSuffix(path, "/allowed-actions"):
		h.getRunAllowedActions(w, r, strings.TrimSuffix(path, "/allowed-actions"))
	default:
		h.getRun(w, r, path)
	}
}

// listRuns GET /runs(limit/state 过滤)。
func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "run reader is not configured")
		return
	}
	tenant := h.tenantPrincipal(r)
	q := r.URL.Query()
	limit := 20
	if v := q.Get("limit"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &limit)
	}
	rows, err := h.repo.ListRuns(r.Context(), repository.ListRunsParams{
		TenantID: tenant, State: q.Get("state"), Limit: limit,
	})
	if err != nil {
		h.logger.Error("list runs", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows, "meta": map[string]int{"count": len(rows)}})
}

// listTasks GET /tasks(七列)。
func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "task reader is not configured")
		return
	}
	tenant := h.tenantPrincipal(r)
	rows, err := h.repo.ListTasks(r.Context(), tenant, 100, 0)
	if err != nil {
		h.logger.Error("list tasks", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows, "meta": map[string]int{"count": len(rows)}})
}

// taskPlanDispatch POST /tasks/{taskID}/plans:保存人工定制计划草稿(P2,FP-103..106)。
func (h *Handler) taskPlanDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/tasks/")
	taskID := strings.TrimSuffix(path, "/plans")
	if taskID == "" || taskID == path || !strings.HasSuffix(path, "/plans") {
		writeError(w, http.StatusNotFound, string(contract.ErrCodeNotFound), "expect POST /tasks/{task_id}/plans")
		return
	}
	if h.planAuthor == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "plan author is not configured")
		return
	}
	var req struct {
		PlanSource           string          `json:"plan_source"` // AUTO_DEFAULT|MANUAL_CUSTOM(缺省 MANUAL_CUSTOM)
		CustomOverrides      json.RawMessage `json:"custom_overrides"`
		ClientIdempotencyKey string          `json:"client_idempotency_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if req.PlanSource == "" {
		req.PlanSource = "MANUAL_CUSTOM"
	}
	var resp interface{}
	var err error
	if req.PlanSource == "AUTO_DEFAULT" {
		resp, err = h.planAuthor.SaveDefault(r.Context(), service.SaveCustomPlanRequest{
			TenantID:             tenant,
			TaskDefinitionID:     taskID,
			Actor:                tenant,
			ClientIdempotencyKey: req.ClientIdempotencyKey,
		})
	} else {
		resp, err = h.planAuthor.SaveCustom(r.Context(), service.SaveCustomPlanRequest{
			TenantID:             tenant,
			TaskDefinitionID:     taskID,
			CustomOverrides:      req.CustomOverrides,
			Actor:                tenant,
			ClientIdempotencyKey: req.ClientIdempotencyKey,
		})
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": resp})
}

// planDispatch POST /plans/{planID}/approve:maker/checker 审批并激活(P2,FP-102)。
func (h *Handler) planDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/plans/")
	planID := strings.TrimSuffix(path, "/approve")
	if planID == "" || planID == path || !strings.HasSuffix(path, "/approve") {
		writeError(w, http.StatusNotFound, string(contract.ErrCodeNotFound), "expect POST /plans/{plan_id}/approve")
		return
	}
	if h.planAuthor == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "plan author is not configured")
		return
	}
	var req struct {
		Maker   string `json:"maker"`
		Checker string `json:"checker"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if err := h.planAuthor.Approve(r.Context(), service.ApprovePlanRequest{
		TenantID: tenant, PlanID: planID, Maker: req.Maker, Checker: req.Checker,
	}); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]string{
		"plan_id": planID, "state": "ACTIVE",
	}})
}

// getRunSummary GET /runs/{run_id}/summary:冻结机器摘要内容(报告渲染输入)。
func (h *Handler) getRunSummary(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "run reader is not configured")
		return
	}
	summary, err := h.repo.GetRunSummaryContent(r.Context(), tenant, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "run summary not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": summary})
}

// reportDispatch GET /reports(列表)、POST /reports/{id}/worker-receipt、
// POST /reports/{id}/verify(报告状态独立推进,不回退 Run)、
// POST /reports/{id}/retry(FAILED/CANCELLED→新 QUEUED)、
// POST /reports/{id}/download-ticket(仅 AVAILABLE)。
func (h *Handler) reportDispatch(w http.ResponseWriter, r *http.Request) {
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.reports == nil || h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "report services are not configured")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/reports" {
		rows, err := h.repo.ListReports(r.Context(), tenant)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/reports/")
	switch {
	case strings.HasSuffix(path, "/worker-receipt"):
		var req struct {
			ObjectKey           string `json:"object_key"`
			ObjectSHA256        string `json:"object_sha256"`
			ObjectSize          int64  `json:"object_size"`
			SourceSummarySHA256 string `json:"source_summary_sha256"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		state, err := h.reports.ApplyWorkerReceipt(r.Context(), tenant, strings.TrimSuffix(path, "/worker-receipt"),
			req.ObjectKey, req.ObjectSHA256, req.ObjectSize, req.SourceSummarySHA256)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]string{"report_state": state}})
	case strings.HasSuffix(path, "/verify"):
		var req struct {
			ObjectSHA256 string `json:"object_sha256"`
			ObjectSize   int64  `json:"object_size"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		if err := h.reports.VerifyAndConfirm(r.Context(), tenant, strings.TrimSuffix(path, "/verify"),
			req.ObjectSHA256, req.ObjectSize); err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]string{"report_state": "AVAILABLE"}})
	case strings.HasSuffix(path, "/download"):
		var req struct {
			TicketID string `json:"ticket_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		objectKey, err := h.reports.DownloadReport(r.Context(), tenant, strings.TrimSuffix(path, "/download"), req.TicketID)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]string{
			"report_id": strings.TrimSuffix(path, "/download"), "object_key": objectKey,
		}})
	case strings.HasSuffix(path, "/retry"):
		var req struct {
			ClientIdempotencyKey string `json:"client_idempotency_key"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		newReportID, replayed, err := h.reports.RetryReport(r.Context(), tenant, strings.TrimSuffix(path, "/retry"), req.ClientIdempotencyKey)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		status := http.StatusAccepted
		if replayed {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]interface{}{"success": true, "data": map[string]string{
			"report_id": newReportID, "state": "QUEUED",
		}, "replayed": replayed})
	case strings.HasSuffix(path, "/download-ticket"):
		var req struct {
			TTLSeconds int64  `json:"ttl_seconds"`
			Actor      string `json:"actor"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		if req.Actor == "" {
			req.Actor = tenant
		}
		ticketID, expiresAt, err := h.reports.IssueDownloadTicket(r.Context(), tenant, strings.TrimSuffix(path, "/download-ticket"),
			time.Duration(req.TTLSeconds)*time.Second, req.Actor)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": map[string]string{
			"ticket_id": ticketID, "expires_at": expiresAt.Format(time.RFC3339),
		}})
	default:
		writeError(w, http.StatusNotFound, string(contract.ErrCodeNotFound), "expect GET /reports or POST /reports/{id}/worker-receipt|verify|retry|download-ticket")
	}
}

// retryStage POST /runs/{run_id}/retry-stage:节点级重试(§76.47.3)。
// SHARED_STREAM 节点无重放输入 → STAGE_RETRY_UNSUPPORTED(引导整 Run retry)。
func (h *Handler) retryStage(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.retry == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "retry service is not configured")
		return
	}
	var req struct {
		ExecutionNodeID string `json:"execution_node_id"`
		Actor           string `json:"actor"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	resp, err := h.retry.RetryStage(r.Context(), tenant, runID, req.ExecutionNodeID, req.Actor)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": resp})
}

// retryTaskRun POST /runs/{run_id}/retry-task:整 Run 重试(同 task 新 run;§76.47.3)。
func (h *Handler) retryTaskRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.retryTask == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "retry task service is not configured")
		return
	}
	var req struct {
		ClientIdempotencyKey string `json:"client_idempotency_key"`
		Actor                string `json:"actor"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if req.Actor == "" {
		req.Actor = tenant
	}
	receipt, err := h.retryTask.RetryTask(r.Context(), tenant, runID, req.Actor, req.ClientIdempotencyKey)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": map[string]string{
		"task_id": receipt.TaskID, "run_id": receipt.RunID,
		"status_url": fmt.Sprintf("/api/v1/analysis/runs/%s", receipt.RunID),
	}})
}

// getRunReceipts GET /runs/{run_id}/receipts:阶段回执投影(运行详情"技术详情"页签)。
func (h *Handler) getRunReceipts(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.resources == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "resource service is not configured")
		return
	}
	rows, err := h.resources.StageReceipts(r.Context(), tenant, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
}

// getRunResults GET /runs/{run_id}/results:CH 只读结果(阶段结果 + 每输入×检测器处置)。
// 未配置 CH 读取时 fail-closed 503(§20 运行详情"结果"页签)。
func (h *Handler) getRunResults(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "run reader is not configured")
		return
	}
	if _, err := h.repo.GetRun(r.Context(), tenant, runID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
		return
	}
	if h.results == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "results read is not configured")
		return
	}
	ctx := r.Context()
	// CH 命名参数参数化({tenant:String}/{run:String}),无字符串拼接注入面。
	params := map[string]string{"tenant": tenant, "run": runID}
	stageRows, err := h.results.Query(ctx,
		`SELECT execution_node_id, attempt, input_count, output_count, error_count, reject_count, watermark_ms, fence_json, payload_hash
		 FROM traffic.analysis_run_results WHERE tenant_id={tenant:String} AND run_id={run:String} ORDER BY ts`, params)
	if err != nil {
		h.logger.Error("run results stage read", zap.Error(err))
		writeError(w, http.StatusBadGateway, "RESULTS_READ_FAILED", "clickhouse results read failed")
		return
	}
	detRows, err := h.results.Query(ctx,
		`SELECT detector_id, disposition, count() AS count, avg(score) AS avg_score
		 FROM traffic.analysis_detections WHERE tenant_id={tenant:String} AND run_id={run:String}
		 GROUP BY detector_id, disposition ORDER BY detector_id, disposition`, params)
	if err != nil {
		h.logger.Error("run results detection read", zap.Error(err))
		writeError(w, http.StatusBadGateway, "RESULTS_READ_FAILED", "clickhouse detection read failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{
		"run_id":         runID,
		"stage_results":  stageRows,
		"detection_rows": detRows,
	}})
}

// getRunAllowedActions GET /runs/{run_id}/allowed-actions:动作授权服务端判定(§20/§21)。
func (h *Handler) getRunAllowedActions(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.allowed == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "allowed actions service is not configured")
		return
	}
	actions, err := h.allowed.ForRun(r.Context(), tenant, runID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": actions})
}

// taskDefinitionDispatch 任务定义权威端点:
// POST /task-definitions(创建 DRAFT)、GET /task-definitions(列表)、
// GET /task-definitions/{id}(详情)、GET /{id}/plans、GET/POST /{id}/report-policies、
// POST /{id}/activate、POST /{id}/suspend(If-Match expected revision)。
func (h *Handler) taskDefinitionDispatch(w http.ResponseWriter, r *http.Request) {
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.taskDefs == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "task definition service is not configured")
		return
	}
	// GET /task-definitions(列表)
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/task-definitions" {
		if h.repo == nil {
			writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "run reader is not configured")
			return
		}
		rows, err := h.repo.ListTaskDefinitions(r.Context(), tenant)
		if err != nil {
			h.logger.Error("list task definitions", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
		return
	}
	// POST /task-definitions(创建 DRAFT)
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/analysis/task-definitions" {
		var req struct {
			Name                   string `json:"name"`
			Owner                  string `json:"owner"`
			DefaultSchedulingClass string `json:"default_scheduling_class"`
			ClientIdempotencyKey   string `json:"client_idempotency_key"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		if req.Owner == "" {
			req.Owner = tenant
		}
		defID, replayed, err := h.taskDefs.Create(r.Context(), tenant, req.Name, req.Owner, req.DefaultSchedulingClass, req.ClientIdempotencyKey)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]interface{}{"success": true, "data": map[string]string{
			"task_definition_id": defID, "state": "DRAFT",
		}, "replayed": replayed})
		return
	}
	// 子资源
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/task-definitions/")
	if path == "" {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	switch {
	case strings.HasSuffix(path, "/plans") && r.Method == http.MethodGet:
		defID := strings.TrimSuffix(path, "/plans")
		rows, err := h.taskDefs.Plans(r.Context(), tenant, defID)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
	case strings.HasSuffix(path, "/report-policies") && r.Method == http.MethodGet:
		defID := strings.TrimSuffix(path, "/report-policies")
		rows, err := h.taskDefs.ListReportPolicies(r.Context(), tenant, defID)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
	case strings.HasSuffix(path, "/report-policies") && r.Method == http.MethodPost:
		defID := strings.TrimSuffix(path, "/report-policies")
		var req struct {
			Mode                 string `json:"mode"`
			TemplateRevision     string `json:"template_revision"`
			Locale               string `json:"locale"`
			RetentionDays        int64  `json:"retention_days"`
			ClientIdempotencyKey string `json:"client_idempotency_key"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		policyID, revision, replayed, err := h.taskDefs.SaveReportPolicy(r.Context(), tenant, defID,
			req.Mode, req.TemplateRevision, req.Locale, req.RetentionDays, req.ClientIdempotencyKey)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]interface{}{"success": true, "data": map[string]interface{}{
			"policy_id": policyID, "revision": revision,
		}, "replayed": replayed})
	case strings.HasSuffix(path, "/allowed-actions") && r.Method == http.MethodGet:
		defID := strings.TrimSuffix(path, "/allowed-actions")
		actions, err := h.allowed.ForDefinition(r.Context(), tenant, defID)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": actions})
	case (strings.HasSuffix(path, "/activate") || strings.HasSuffix(path, "/suspend")) && r.Method == http.MethodPost:
		action := "activate"
		defID := strings.TrimSuffix(path, "/activate")
		if !strings.HasSuffix(path, "/activate") {
			action = "suspend"
			defID = strings.TrimSuffix(path, "/suspend")
		}
		var req struct {
			ExpectedRevision int64  `json:"expected_revision"`
			Actor            string `json:"actor"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		if req.Actor == "" {
			req.Actor = tenant
		}
		var newRevision int64
		var err error
		switch action {
		case "activate":
			newRevision, err = h.taskDefs.Activate(r.Context(), tenant, defID, req.ExpectedRevision, req.Actor)
		case "suspend":
			newRevision, err = h.taskDefs.Suspend(r.Context(), tenant, defID, req.ExpectedRevision, req.Actor)
		}
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{
			"task_definition_id": defID, "action": action, "authority_revision": newRevision,
		}})
	case r.Method == http.MethodGet:
		// GET /task-definitions/{id}(详情五 Tab)
		detail, err := h.taskDefs.Detail(r.Context(), tenant, path)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": detail})
	default:
		writeError(w, http.StatusNotFound, string(contract.ErrCodeNotFound), "unknown task-definition subresource")
	}
}

// resourceDispatch GET /resources:调度资源视图(容量配额/队列/租约/执行器)。
func (h *Handler) resourceDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.resources == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "resource service is not configured")
		return
	}
	views, err := h.resources.Views(r.Context(), tenant)
	if err != nil {
		h.logger.Error("resource views", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": views})
}

// scheduleDispatch 调度修订权威端点:
// POST /schedules(保存 DRAFT 修订)、GET /schedules(列表,含激活头)、
// POST /schedules/{id}/activate、POST /schedules/{id}/pause(If-Match expected revision)、
// GET /schedules/{id}/triggers(触发历史投影)。
func (h *Handler) scheduleDispatch(w http.ResponseWriter, r *http.Request) {
	tenant := h.tenantPrincipal(r)
	if tenant == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "tenant principal required")
		return
	}
	if h.schedules == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "schedule service is not configured")
		return
	}
	// GET /schedules(列表)
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/analysis/schedules" {
		rows, err := h.schedules.List(r.Context(), tenant)
		if err != nil {
			h.logger.Error("list schedules", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
		return
	}
	// GET /schedules/{id}/triggers(触发历史投影)
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/triggers") {
		scheduleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/schedules/"), "/triggers")
		rows, err := h.schedules.TriggerHistory(r.Context(), tenant, scheduleID)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": rows})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	// POST /schedules(保存)
	if r.URL.Path == "/api/v1/analysis/schedules" {
		var req struct {
			TaskDefinitionID     string          `json:"task_definition_id"`
			ApprovedPlanRevision int64           `json:"approved_plan_revision"`
			ExecutionSpecSHA256  string          `json:"execution_spec_sha256"`
			TriggerKind          string          `json:"trigger_kind"`
			Timezone             string          `json:"timezone"`
			WindowOrCron         json.RawMessage `json:"window_or_cron"`
			PrepareLeadTimeMs    int64           `json:"prepare_lead_time_ms"`
			MisfirePolicy        string          `json:"misfire_policy"`
			ConcurrencyPolicy    string          `json:"concurrency_policy"`
			SchedulingClass      string          `json:"scheduling_class"`
			ResourceRestrictions json.RawMessage `json:"resource_restrictions"`
			ClientIdempotencyKey string          `json:"client_idempotency_key"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		resp, replayed, err := h.schedules.Save(r.Context(), service.SaveScheduleRequest{
			TenantID:             tenant,
			TaskDefinitionID:     req.TaskDefinitionID,
			ApprovedPlanRevision: req.ApprovedPlanRevision,
			ExecutionSpecSHA256:  req.ExecutionSpecSHA256,
			TriggerKind:          req.TriggerKind,
			Timezone:             req.Timezone,
			WindowOrCron:         req.WindowOrCron,
			PrepareLeadTimeMs:    req.PrepareLeadTimeMs,
			MisfirePolicy:        req.MisfirePolicy,
			ConcurrencyPolicy:    req.ConcurrencyPolicy,
			SchedulingClass:      req.SchedulingClass,
			ResourceRestrictions: req.ResourceRestrictions,
			ClientIdempotencyKey: req.ClientIdempotencyKey,
		})
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]interface{}{"success": true, "data": resp, "replayed": replayed})
		return
	}
	// POST /schedules/{id}/activate|pause
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/schedules/")
	scheduleID := strings.TrimSuffix(path, "/activate")
	action := "activate"
	if !strings.HasSuffix(path, "/activate") {
		scheduleID = strings.TrimSuffix(path, "/pause")
		action = "pause"
	}
	if scheduleID == "" || scheduleID == path {
		writeError(w, http.StatusNotFound, string(contract.ErrCodeNotFound), "expect POST /schedules/{id}/activate or /pause")
		return
	}
	var req struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Actor            string `json:"actor"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	var newRevision int64
	var err error
	switch action {
	case "activate":
		newRevision, err = h.schedules.Activate(r.Context(), tenant, scheduleID, req.ExpectedRevision, req.Actor)
	case "pause":
		newRevision, err = h.schedules.Pause(r.Context(), tenant, scheduleID, req.ExpectedRevision, req.Actor)
	}
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]interface{}{
		"schedule_id": scheduleID, "action": action, "authority_revision": newRevision,
	}})
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request, runID string) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "run reader is not configured")
		return
	}
	tenant := h.tenantPrincipal(r)
	run, err := h.repo.GetRun(r.Context(), tenant, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
		return
	}
	stages, err := h.repo.ListRunStages(r.Context(), tenant, runID)
	if err != nil {
		stages = nil
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"run_id": run.RunID, "task_id": run.TaskID, "state": run.State,
			"completeness": run.Completeness, "integrity_state": run.IntegrityState,
			"finding_conclusion": run.FindingConclusion, "risk_severity": run.RiskSeverity,
			"execution_spec_sha256": run.ExecutionSpecSHA256,
			"window_start_ms":       run.WindowStartMs, "window_end_ms": run.WindowEndMs,
			"revision": run.Revision, "stages": stages,
		},
	})
}

func (h *Handler) cancelRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if h.cancel == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "cancel service is not configured")
		return
	}
	tenant := h.tenantPrincipal(r)
	if err := h.cancel.RequestCancel(r.Context(), tenant, runID); err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": map[string]string{"run_id": runID, "state": "CANCEL_REQUESTED"}})
}

func (h *Handler) requestReport(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if h.reports == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "report service is not configured")
		return
	}
	tenant := h.tenantPrincipal(r)
	reportID, replayed, err := h.reports.RequestReport(r.Context(), tenant, runID, "default-v1", "zh-CN")
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	if replayed {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": map[string]string{"state": "QUEUED", "replayed": "true"}})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": map[string]string{"report_id": reportID, "state": "QUEUED"}})
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	// 统一错误框架:common/errors AppError(码表统一 HTTP/重试分类)
	var ae *commonerrors.AppError
	if errors.As(err, &ae) {
		writeError(w, ae.HTTPStatus(), string(ae.Code), ae.Message)
		return
	}
	// 契约码前缀的原始错误(仓储/适配层)同样映射稳定 HTTP 语义码。
	if code, msg, ok := commonerrors.ParseErrorCode(err); ok {
		writeError(w, code.HTTPStatus(), string(code), msg)
		return
	}
	h.logger.Error("analysis api internal error", zap.Error(err))
	writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"success": false,
		"error":   map[string]string{"code": code, "message": message},
	})
}

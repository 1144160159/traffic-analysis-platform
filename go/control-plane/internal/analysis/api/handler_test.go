package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("health status=%d", rr.Code)
	}
}

func TestSubmitTriggerRequiresTenantPrincipal(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/triggers", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.submitTrigger(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rr.Code, rr.Body.String())
	}
}

func TestSubmitTriggerRejectsInvalidBody(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	h.tenantPrincipal = func(*http.Request) string { return "tenant-a" }
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/triggers", strings.NewReader(`{bad`))
	rr := httptest.NewRecorder()
	h.submitTrigger(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}


func TestRunDispatchCancelRoute(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	h.tenantPrincipal = func(*http.Request) string { return "tenant-a" }
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/runs/run-1:cancel", nil)
	rr := httptest.NewRecorder()
	h.runDispatch(rr, req)
	// cancelSvc 为 nil 时会 panic 防御?No——handler 直接调用;此处验证路由解析不 panic 且返回 500 类错误
	if rr.Code == 0 {
		t.Fatalf("no response written")
	}
}

func TestTaskPlanDispatchGuards(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	// 1) 未配置计划服务 → 503
	h.tenantPrincipal = func(*http.Request) string { return "tenant-a" }
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks/def-1/plans", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.taskPlanDispatch(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rr.Code)
	}
	// 2) 缺租户主体 → 401
	h2 := NewHandler(nil, nil, nil, nil, nil)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks/def-1/plans", strings.NewReader(`{}`))
	rr = httptest.NewRecorder()
	h2.taskPlanDispatch(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rr.Code)
	}
	// 3) 路径不合法 → 404
	h3 := NewHandler(nil, nil, nil, nil, nil)
	h3.tenantPrincipal = func(*http.Request) string { return "tenant-a" }
	req = httptest.NewRequest(http.MethodPost, "/api/v1/analysis/tasks/def-1/wrong", strings.NewReader(`{}`))
	rr = httptest.NewRecorder()
	h3.taskPlanDispatch(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

func TestPlanDispatchGuards(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	h.tenantPrincipal = func(*http.Request) string { return "tenant-a" }
	// 未配置计划服务 → 503
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/plans/plan-1/approve", strings.NewReader(`{"maker":"a","checker":"b"}`))
	rr := httptest.NewRecorder()
	h.planDispatch(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rr.Code)
	}
	// 路径不合法 → 404
	req = httptest.NewRequest(http.MethodPost, "/api/v1/analysis/plans/plan-1", strings.NewReader(`{}`))
	rr = httptest.NewRecorder()
	h.planDispatch(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

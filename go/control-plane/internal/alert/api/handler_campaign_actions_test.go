package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type campaignActionAuditRecorder struct {
	records []AlertActionAuditRecord
	err     error
}

type campaignActionJobRecorder struct {
	jobs       []campaignActionJob
	failedJobs []string
	err        error
}

func (r *campaignActionJobRecorder) Record(_ context.Context, job campaignActionJob) error {
	if r.err != nil {
		return r.err
	}
	r.jobs = append(r.jobs, job)
	return nil
}

func (r *campaignActionJobRecorder) MarkFailed(_ context.Context, _ string, jobID, _ string) error {
	r.failedJobs = append(r.failedJobs, jobID)
	return nil
}

func (r *campaignActionJobRecorder) Get(_ context.Context, tenantID, jobID string) (campaignActionJob, error) {
	for _, job := range r.jobs {
		if job.TenantID == tenantID && job.JobID == jobID {
			return job, nil
		}
	}
	return campaignActionJob{}, sql.ErrNoRows
}

func (r *campaignActionAuditRecorder) Record(_ context.Context, _ *http.Request, record AlertActionAuditRecord) error {
	r.records = append(r.records, record)
	return r.err
}

func TestSubmitCampaignActionRecordsAuthorizedRequest(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		actionID   string
		permission string
		objectID   string
		auditEvent string
		mutates    bool
	}{
		{
			name:       "campaign-specific action",
			path:       "/campaigns/campaign-42/actions",
			actionID:   "campaign-status-change",
			permission: authmodel.ScopeAlertWrite,
			objectID:   "campaign-42",
			auditEvent: "CAMPAIGN_STATUS_CHANGED",
			mutates:    true,
		},
		{
			name:       "collection action",
			path:       "/campaigns/actions",
			actionID:   "campaign-export",
			permission: authmodel.ScopeAlertRead,
			objectID:   "campaign-collection",
			auditEvent: "CAMPAIGN_EXPORT_REQUESTED",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			audit := &campaignActionAuditRecorder{}
			handler := newCampaignActionTestHandler()
			jobs := handler.campaignJobs.(*campaignActionJobRecorder)
			handler.actionAudit = audit
			router := mux.NewRouter()
			handler.RegisterRoutes(router)

			metadata := `"campaign_id":"campaign-42",`
			if test.path == "/campaigns/actions" {
				metadata = ""
			}
			simulation, dryRun := "true", "true"
			if test.mutates {
				simulation, dryRun = "false", "false"
				metadata += `"next_status":"investigating",`
			}
			body := `{"action_id":"` + test.actionID + `","target":"campaign-42","metadata":{` + metadata + `"dry_run":` + dryRun + `,"source":"test"},"simulation":` + simulation + `,"dry_run":` + dryRun + `}`
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(body))
			request = campaignActionRequestWithPermissions(request, test.permission)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					ActionID   string `json:"action_id"`
					AuditEvent string `json:"audit_event"`
					Status     string `json:"status"`
					Endpoint   string `json:"endpoint"`
					JobID      string `json:"job_id"`
					JobStatus  string `json:"job_status"`
					Simulation bool   `json:"simulation"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			require.Equal(t, test.actionID, response.Data.ActionID)
			require.Equal(t, test.auditEvent, response.Data.AuditEvent)
			require.Equal(t, "completed", response.Data.Status)
			require.Equal(t, test.path, response.Data.Endpoint)
			require.NotEmpty(t, response.Data.JobID)
			require.Equal(t, "completed", response.Data.JobStatus)
			require.Equal(t, !test.mutates, response.Data.Simulation)

			require.Len(t, audit.records, 1)
			record := audit.records[0]
			require.Equal(t, test.auditEvent, record.Action)
			require.Equal(t, "campaign", record.ObjectType)
			require.Equal(t, test.objectID, record.ObjectID)
			require.Equal(t, "completed", record.Result)
			require.Equal(t, test.actionID, record.Detail["action_id"])
			require.Equal(t, test.auditEvent, record.Detail["audit_event"])
			require.Equal(t, !test.mutates, record.Detail["simulation"])
			require.Equal(t, !test.mutates, record.Detail["dry_run"])
			require.Equal(t, response.Data.JobID, record.Detail["job_id"])
			require.Len(t, jobs.jobs, 1)
			require.Equal(t, response.Data.JobID, jobs.jobs[0].JobID)
			require.Equal(t, "completed", jobs.jobs[0].Status)
			require.Equal(t, "tenant-test", jobs.jobs[0].TenantID)
		})
	}
}

func TestSubmitCampaignCollectionActionRejectsCampaignIDMetadata(t *testing.T) {
	handler := newCampaignActionTestHandler()
	handler.actionAudit = &campaignActionAuditRecorder{}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	body := `{"action_id":"campaign-export","target":"current-page","metadata":{"campaign_id":"campaign-42","dry_run":true},"simulation":true,"dry_run":true}`
	request := campaignActionRequestWithPermissions(httptest.NewRequest(http.MethodPost, "/campaigns/actions", strings.NewReader(body)), authmodel.ScopeAlertRead)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "must not include metadata campaign_id")
}

func TestSubmitCampaignActionEnforcesActionScopes(t *testing.T) {
	tests := []struct {
		name       string
		actionID   string
		permission string
		wantStatus int
	}{
		{name: "read permits inspect", actionID: "campaign-phase-inspect", permission: authmodel.ScopeAlertRead, wantStatus: http.StatusOK},
		{name: "write permits inspect", actionID: "campaign-phase-inspect", permission: authmodel.ScopeAlertWrite, wantStatus: http.StatusOK},
		{name: "read cannot change status", actionID: "campaign-status-change", permission: authmodel.ScopeAlertRead, wantStatus: http.StatusForbidden},
		{name: "graph requires graph read", actionID: "campaign-graph-view", permission: authmodel.ScopeAlertRead, wantStatus: http.StatusForbidden},
		{name: "graph read permits graph", actionID: "campaign-graph-view", permission: authmodel.ScopeGraphRead, wantStatus: http.StatusOK},
		{name: "soar requires execute", actionID: "campaign-soar-response", permission: authmodel.ScopeAlertWrite, wantStatus: http.StatusForbidden},
		{name: "playbook execute permits soar", actionID: "campaign-soar-response", permission: "playbook:execute", wantStatus: http.StatusOK},
		{name: "admin permits write", actionID: "campaign-report-generate", permission: authmodel.ScopeAdminAll, wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			audit := &campaignActionAuditRecorder{}
			handler := newCampaignActionTestHandler()
			handler.actionAudit = audit
			router := mux.NewRouter()
			handler.RegisterRoutes(router)

			mutates := campaignActionSpecs[test.actionID].Mutates
			simulation, dryRun := "true", "true"
			metadata := `"campaign_id":"campaign-42",`
			if mutates {
				simulation, dryRun = "false", "false"
				if test.actionID == "campaign-status-change" {
					metadata += `"next_status":"investigating",`
				}
			}
			body := `{"action_id":"` + test.actionID + `","target":"campaign-42","metadata":{` + metadata + `"dry_run":` + dryRun + `},"simulation":` + simulation + `,"dry_run":` + dryRun + `}`
			request := httptest.NewRequest(http.MethodPost, "/campaigns/campaign-42/actions", strings.NewReader(body))
			request = campaignActionRequestWithPermissions(request, test.permission)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, test.wantStatus, recorder.Code, recorder.Body.String())
			if test.wantStatus == http.StatusForbidden {
				require.Empty(t, audit.records)
			} else {
				require.Len(t, audit.records, 1)
			}
		})
	}
}

func TestSubmitCampaignActionRejectsMalformedBody(t *testing.T) {
	audit := &campaignActionAuditRecorder{}
	handler := newCampaignActionTestHandler()
	handler.actionAudit = audit
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	request := httptest.NewRequest(http.MethodPost, "/campaigns/actions", strings.NewReader(`{"action_id":`))
	request = campaignActionRequestWithPermissions(request, authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Empty(t, audit.records)
}

func TestSubmitCampaignActionDoesNotSucceedWhenAuditFails(t *testing.T) {
	audit := &campaignActionAuditRecorder{err: errors.New("postgres unavailable")}
	handler := newCampaignActionTestHandler()
	jobs := handler.campaignJobs.(*campaignActionJobRecorder)
	handler.actionAudit = audit
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	body := `{"action_id":"campaign-context-action","target":"campaign-42","metadata":{"campaign_id":"campaign-42","dry_run":true},"simulation":true,"dry_run":true}`
	request := httptest.NewRequest(http.MethodPost, "/campaigns/campaign-42/actions", strings.NewReader(body))
	request = campaignActionRequestWithPermissions(request, authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "ACTION_COMMIT_FAILED")
	require.Len(t, audit.records, 1)
	require.Empty(t, jobs.failedJobs)
}

func TestSubmitCampaignActionDoesNotSucceedWithoutAuditWriter(t *testing.T) {
	handler := newCampaignActionTestHandler()
	jobs := handler.campaignJobs.(*campaignActionJobRecorder)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	body := `{"action_id":"campaign-context-action","target":"campaign-42","metadata":{"campaign_id":"campaign-42","dry_run":true},"simulation":true,"dry_run":true}`
	request := httptest.NewRequest(http.MethodPost, "/campaigns/campaign-42/actions", strings.NewReader(body))
	request = campaignActionRequestWithPermissions(request, authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "ACTION_COMMIT_FAILED")
	require.Empty(t, jobs.failedJobs)
}

func TestSubmitCampaignActionUsesAtomicCommitterWhenConfigured(t *testing.T) {
	handler := newCampaignActionTestHandler()
	var committedJob campaignActionJob
	var committedAudit AlertActionAuditRecord
	handler.commitCampaignAction = func(_ context.Context, _ *http.Request, job campaignActionJob, audit AlertActionAuditRecord) error {
		committedJob = job
		committedAudit = audit
		return nil
	}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	body := `{"action_id":"campaign-status-change","target":"campaign-42","metadata":{"campaign_id":"campaign-42","next_status":"investigating","dry_run":false},"simulation":false,"dry_run":false}`
	request := campaignActionRequestWithPermissions(httptest.NewRequest(http.MethodPost, "/campaigns/campaign-42/actions", strings.NewReader(body)), authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotEmpty(t, committedJob.JobID)
	require.Equal(t, committedJob.JobID, committedAudit.Detail["job_id"])
	require.Equal(t, "tenant-test", committedJob.TenantID)
}

func TestSubmitCampaignActionRequiresJobStore(t *testing.T) {
	handler := newCampaignActionTestHandler()
	handler.actionAudit = &campaignActionAuditRecorder{}
	handler.campaignJobs = nil
	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	body := `{"action_id":"campaign-context-action","target":"campaign-42","metadata":{"campaign_id":"campaign-42","dry_run":true},"simulation":true,"dry_run":true}`
	request := campaignActionRequestWithPermissions(httptest.NewRequest(http.MethodPost, "/campaigns/campaign-42/actions", strings.NewReader(body)), authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "JOB_STORE_UNAVAILABLE")
}

func TestSubmitCampaignActionRequiresSimulationAndDryRun(t *testing.T) {
	tests := []string{
		`{"action_id":"campaign-context-action","target":"campaign-42","metadata":{},"simulation":false}`,
		`{"action_id":"campaign-context-action","target":"campaign-42","metadata":{"dry_run":false},"simulation":true}`,
		`{"action_id":"campaign-context-action","target":"campaign-42","metadata":{},"simulation":true,"dry_run":false}`,
		`{"action_id":"campaign-context-action","target":"campaign-42","metadata":{"dry_run":true},"simulation":true}`,
		`{"action_id":"campaign-context-action","target":"campaign-42","metadata":{},"simulation":true,"dry_run":true}`,
	}
	for _, body := range tests {
		handler := newCampaignActionTestHandler()
		handler.actionAudit = &campaignActionAuditRecorder{}
		router := mux.NewRouter()
		handler.RegisterRoutes(router)
		request := httptest.NewRequest(http.MethodPost, "/campaigns/actions", strings.NewReader(body))
		request = campaignActionRequestWithPermissions(request, authmodel.ScopeAlertWrite)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
}

func TestSubmitCampaignActionRejectsUnknownTenantCampaign(t *testing.T) {
	handler := newCampaignActionTestHandler()
	handler.actionAudit = &campaignActionAuditRecorder{}
	handler.lookupCampaign = func(context.Context, string, string) (campaignDTO, error) {
		return campaignDTO{}, sql.ErrNoRows
	}
	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	body := `{"action_id":"campaign-status-change","target":"missing-campaign","metadata":{"campaign_id":"missing-campaign","next_status":"investigating","dry_run":false},"simulation":false,"dry_run":false}`
	request := campaignActionRequestWithPermissions(httptest.NewRequest(http.MethodPost, "/campaigns/missing-campaign/actions", strings.NewReader(body)), authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
}

func TestCampaignTenantAndReadPermissionCannotBeOverriddenByQuery(t *testing.T) {
	handler := newCampaignActionTestHandler()
	request := httptest.NewRequest(http.MethodGet, "/campaigns?tenant_id=other-tenant", nil)
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyTenantID, "tenant-test"))
	require.Equal(t, "tenant-test", queryTenantID(request))

	recorder := httptest.NewRecorder()
	handler.ListCampaigns(recorder, request)
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

func newCampaignActionTestHandler() *SystemHandler {
	handler := NewSystemHandler(nil, nil, nil)
	handler.campaignJobs = &campaignActionJobRecorder{}
	handler.commitCampaignAction = func(ctx context.Context, request *http.Request, job campaignActionJob, record AlertActionAuditRecord) error {
		if handler.actionAudit == nil {
			return errors.New("campaign action audit is not configured")
		}
		if err := handler.campaignJobs.Record(ctx, job); err != nil {
			return err
		}
		return handler.actionAudit.Record(ctx, request, record)
	}
	handler.lookupCampaign = func(_ context.Context, tenantID, campaignID string) (campaignDTO, error) {
		if tenantID != "tenant-test" || campaignID != "campaign-42" {
			return campaignDTO{}, sql.ErrNoRows
		}
		return campaignDTO{TenantID: tenantID, CampaignID: campaignID}, nil
	}
	return handler
}

func campaignActionRequestWithPermissions(request *http.Request, permissions ...string) *http.Request {
	ctx := context.WithValue(request.Context(), httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-test")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "user-test")
	return request.WithContext(ctx)
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestTaskCommandAdmissionRequiresFlagAndCompatibleWorker(t *testing.T) {
	for _, test := range []struct {
		name, wantCode   string
		pipeline, worker bool
		wantStatus       int
	}{
		{name: "both disabled", wantCode: "FORENSICS_PIPELINE_NOT_READY", wantStatus: http.StatusServiceUnavailable},
		{name: "writer flag only", pipeline: true, wantCode: "FORENSICS_PIPELINE_NOT_READY", wantStatus: http.StatusServiceUnavailable},
		{name: "worker receipt only", worker: true, wantCode: "FORENSICS_PIPELINE_NOT_READY", wantStatus: http.StatusServiceUnavailable},
		{name: "both ready reaches request validation", pipeline: true, worker: true, wantCode: "start_time and end_time are required", wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHandler(nil, nil, nil, nil, nil, nil)
			handler.SetTaskCommandAdmission(test.pipeline, test.worker)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/pcap/jobs", strings.NewReader(`{}`))
			handler.CreateJob(recorder, request)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantCode) {
				t.Fatalf("status=%d body=%s, want status=%d code=%s", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantCode)
			}
		})
	}
}

func TestVersionedForensicsTaskCommandMetaRejectsCompatibilityFallback(t *testing.T) {
	baseContext := context.WithValue(context.Background(), httpx.ContextKeyTraceID, "trace-forensics-1")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pcap/jobs", nil).WithContext(baseContext)
	if _, err := versionedForensicsTaskCommandMeta(request, "tenant-a", "actor-a", repository.ForensicsTaskCreateAction, 0, false); err == nil {
		t.Fatal("expected missing idempotency key and reason to fail closed")
	}
	request.Header.Set("Idempotency-Key", "forensics-create-000001")
	request.Header.Set("X-Action-Reason", "investigate encrypted exfiltration")
	meta, err := versionedForensicsTaskCommandMeta(request, "tenant-a", "actor-a", repository.ForensicsTaskCreateAction, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if meta.CompatibilityMode || meta.TenantID != "tenant-a" || meta.ActorID != "actor-a" || meta.TraceID != "trace-forensics-1" {
		t.Fatalf("unexpected versioned command meta: %+v", meta)
	}
}

func TestHasForensicsPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		required    string
		want        bool
	}{
		{name: "exact", permissions: []string{"pcap:read"}, required: "pcap:read", want: true},
		{name: "pcap wildcard", permissions: []string{"pcap:*"}, required: "pcap:download", want: true},
		{name: "pcap wildcard cannot admit restoration", permissions: []string{"pcap:*"}, required: "forensics:write", want: false},
		{name: "forensics write admits restoration", permissions: []string{"forensics:write"}, required: "forensics:write", want: true},
		{name: "admin wildcard", permissions: []string{"admin:*"}, required: "pcap:write", want: true},
		{name: "global wildcard", permissions: []string{"*"}, required: "pcap:write", want: true},
		{name: "read does not grant write", permissions: []string{"pcap:read"}, required: "pcap:write", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), httpx.ContextKeyPermissions, test.permissions)
			if got := hasForensicsPermission(ctx, test.required); got != test.want {
				t.Fatalf("hasForensicsPermission() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNormalizeResultKeyAcceptsLegacyAndVersionedTenantCoordinates(t *testing.T) {
	handler := &Handler{}
	for _, test := range []struct {
		key, tenant string
		valid       bool
	}{
		{key: "results/tenant-a/jobs/result.pcap", tenant: "tenant-a", valid: true},
		{key: "tenants/tenant-a/forensics/jobs/task-a/pcap/result.pcap", tenant: "tenant-a", valid: true},
		{key: "tenants/tenant-a/forensics/jobs/task-a/result.pcap", valid: false},
		{key: "tenants/tenant-a/forensics/jobs/../task-b/pcap/result.pcap", valid: false},
		{key: "/tenants/tenant-a/forensics/jobs/task-a/pcap/result.pcap", valid: false},
	} {
		key, tenant, err := handler.normalizeResultKey(test.key)
		if (err == nil) != test.valid {
			t.Fatalf("normalizeResultKey(%q) key=%q tenant=%q err=%v", test.key, key, tenant, err)
		}
		if test.valid && (key != test.key || tenant != test.tenant) {
			t.Fatalf("normalizeResultKey(%q) = %q,%q", test.key, key, tenant)
		}
	}
}

func TestNormalizeForensicsPurposeRejectsMissingOrUnsafeValues(t *testing.T) {
	if _, err := normalizeForensicsPurpose("", true); err == nil {
		t.Fatal("missing versioned download purpose was accepted")
	}
	if _, err := normalizeForensicsPurpose(strings.Repeat("x", 257), true); err == nil {
		t.Fatal("oversized versioned download purpose was accepted")
	}
	if _, err := normalizeForensicsPurpose("case\nheader", true); err == nil {
		t.Fatal("unsafe versioned download purpose was accepted")
	}
	if purpose, err := normalizeForensicsPurpose("  case CASE-1 review  ", true); err != nil || purpose != "case CASE-1 review" {
		t.Fatalf("safe purpose = %q, %v", purpose, err)
	}
}

func TestVerifyPCAPRejectsMalformedExpectedDigestBeforeObjectAccess(t *testing.T) {
	handler := &Handler{logger: zap.NewNop()}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pcap/verify", strings.NewReader(`{"key":"results/default/job/result.pcap","expected_sha256":"bad"}`))
	recorder := httptest.NewRecorder()
	handler.VerifyPCAP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCheckForensicsSchema(t *testing.T) {
	for _, test := range []struct {
		name  string
		ready bool
		want  bool
	}{
		{name: "required migration is complete", ready: true, want: true},
		{name: "required migration is incomplete", ready: false, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(test.ready))
			handler := &Handler{auditDB: db}
			err = handler.checkForensicsSchema(context.Background())
			if (err == nil) != test.want {
				t.Fatalf("checkForensicsSchema() error = %v, want success=%v", err, test.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFilterFixtureJobsUsesStructuredFields(t *testing.T) {
	jobs := []json.RawMessage{
		json.RawMessage(`{"job_id":"F-20260620-000128","status":"completed","params":{"asset_id":"asset-a","alert_id":"AL-1","campaign_id":"CP-1","baseline_id":"BL-1","evidence_id":"EV-1","evidence_type":"pcap","src_ip":"172.16.5.10","dst_ip":"185.22.14.9","src_port":44221,"dst_port":443,"protocol":"TLS"}}`),
		json.RawMessage(`{"job_id":"F-20260620-000127","status":"completed","params":{"asset_id":"asset-b","src_ip":"172.16.5.11","dst_ip":"8.8.8.8","src_port":53001,"dst_port":53,"protocol":"DNS"}}`),
	}

	tests := []struct {
		name   string
		filter repository.TaskListFilter
		wantID string
	}{
		{name: "source IP", filter: repository.TaskListFilter{SrcIP: "172.16.5.10"}, wantID: "F-20260620-000128"},
		{name: "destination port", filter: repository.TaskListFilter{Port: "53"}, wantID: "F-20260620-000127"},
		{name: "protocol case insensitive", filter: repository.TaskListFilter{Protocol: "tls"}, wantID: "F-20260620-000128"},
		{name: "tuple", filter: repository.TaskListFilter{Tuple: "172.16.5.10:44221 -> 185.22.14.9:443 TLS"}, wantID: "F-20260620-000128"},
		{name: "source alert", filter: repository.TaskListFilter{AlertID: "AL-1"}, wantID: "F-20260620-000128"},
		{name: "source campaign", filter: repository.TaskListFilter{CampaignID: "CP-1"}, wantID: "F-20260620-000128"},
		{name: "source baseline", filter: repository.TaskListFilter{BaselineID: "BL-1"}, wantID: "F-20260620-000128"},
		{name: "evidence reference", filter: repository.TaskListFilter{EvidenceID: "EV-1", EvidenceType: "PCAP"}, wantID: "F-20260620-000128"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filtered := filterFixtureJobs(jobs, test.filter)
			if len(filtered) != 1 {
				t.Fatalf("filterFixtureJobs() returned %d rows, want 1", len(filtered))
			}
			var job struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(filtered[0], &job); err != nil {
				t.Fatal(err)
			}
			if job.JobID != test.wantID {
				t.Fatalf("job id = %q, want %q", job.JobID, test.wantID)
			}
		})
	}
}

func TestShouldServeFixtureJobsDoesNotShadowOperationalFilters(t *testing.T) {
	tests := []struct {
		name    string
		filter  repository.TaskListFilter
		matched int
		want    bool
	}{
		{name: "unfiltered canonical view", filter: repository.TaskListFilter{}, matched: 0, want: true},
		{name: "matched canonical source reference", filter: repository.TaskListFilter{AlertID: "AL-1"}, matched: 1, want: true},
		{name: "unmatched task id reaches repository", filter: repository.TaskListFilter{TaskID: "new-task"}, matched: 0, want: false},
		{name: "unmatched source reference reaches repository", filter: repository.TaskListFilter{EvidenceID: "new-evidence"}, matched: 0, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldServeFixtureJobs(test.filter, test.matched); got != test.want {
				t.Fatalf("shouldServeFixtureJobs() = %v, want %v", got, test.want)
			}
		})
	}
}

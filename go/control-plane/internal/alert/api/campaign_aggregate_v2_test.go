package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type campaignV2AuditStub struct {
	records []AlertActionAuditRecord
	err     error
}

func (s *campaignV2AuditStub) recordWithExecutor(
	_ context.Context,
	_ auditSQLExecutor,
	_ *http.Request,
	record AlertActionAuditRecord,
) error {
	s.records = append(s.records, record)
	return s.err
}

func campaignV2Context() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "analyst-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-campaign-v2")
	return ctx
}

func TestCampaignAggregateV2StatusCommandCommitsStateHistoryOutboxJobAndAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	audit := &campaignV2AuditStub{}
	handler := NewSystemHandler(nil, nil, nil)
	handler.pgDB = db
	handler.campaignAuditWriter = audit

	request := campaignActionRequest{
		ActionID: "campaign-status-change", Target: "推进调查",
		Metadata:   map[string]interface{}{"campaign_id": "campaign-a", "next_status": "investigating", "dry_run": false},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(2),
		Reason: "确认进入调查阶段",
	}
	requestSHA, err := campaignCommandRequestSHA("campaign-a", request)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery("FROM campaign_action_jobs").
		WithArgs("tenant-a", "campaign-command-key-0001").
		WillReturnError(sql.ErrNoRows)
	expectCampaignNotMerged(mock, "tenant-a", "campaign-a")
	mock.ExpectExec("INSERT INTO campaign_workbench_state").
		WithArgs("tenant-a", "campaign-a", "active", "analyst-a").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WithArgs("tenant-a", "campaign-a").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id"}).
			AddRow("active", "owner-a", 2, 0, "00000000-0000-4000-8000-000000000002"))
	mock.ExpectQuery("SELECT alert_id FROM campaign_alert_links").
		WithArgs("tenant-a", "campaign-a").
		WillReturnRows(sqlmock.NewRows([]string{"alert_id"}).AddRow("alert-1").AddRow("alert-2"))
	mock.ExpectExec("UPDATE campaign_workbench_state").
		WithArgs("tenant-a", "campaign-a", "owner-a", "investigating", int64(3), 2, sqlmock.AnyArg(), "analyst-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_aggregate_history").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "campaign-a", int64(3), "traffic.campaign.v2.StatusChanged", "investigating", "owner-a", 2, sqlmock.AnyArg(), "确认进入调查阶段", "analyst-a").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_aggregate_outbox").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "campaign-a", int64(3), "traffic.campaign.v2.StatusChanged", "tenant-a:campaign-a", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_action_jobs").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "campaign-a", "campaign-status-change", "推进调查", sqlmock.AnyArg(), "succeeded", sqlmock.AnyArg(), "analyst-a", sqlmock.AnyArg(), "campaign-command-key-0001", requestSHA, int64(2), int64(3), "确认进入调查阶段").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/campaign-a/actions", nil).WithContext(campaignV2Context())
	job, err := handler.commitCampaignAggregateV2Command(
		httpRequest.Context(), httpRequest, request, campaignActionSpecs[request.ActionID],
		"campaign-a", campaignDTO{CampaignID: "campaign-a", Status: "active"},
		"campaign-command-key-0001", requestSHA,
	)
	require.NoError(t, err)
	require.Equal(t, "succeeded", job.Status)
	require.Equal(t, int64(3), job.ResourceRevision)
	require.Equal(t, "investigating", job.Result["campaign_status"])
	require.Equal(t, 2, job.Result["member_count"])
	require.Len(t, audit.records, 1)
	require.Equal(t, "CAMPAIGN_STATUS_CHANGED", audit.records[0].Action)
	require.Equal(t, "succeeded", audit.records[0].Result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCampaignAggregateV2RejectsStaleSnapshotInsideSerializableTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewSystemHandler(nil, nil, nil)
	handler.pgDB = db
	handler.campaignAuditWriter = &campaignV2AuditStub{}

	request := campaignActionRequest{
		ActionID: "campaign-status-change", Target: "推进调查",
		Metadata: map[string]interface{}{
			"campaign_id": "campaign-a", "next_status": "investigating", "dry_run": false,
			"snapshot_id": "campaign:campaign-a:revision:3:0000000000000000",
		},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(4),
		Reason: "陈旧页面不得提交战役命令",
	}
	requestSHA, err := campaignCommandRequestSHA("campaign-a", request)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery("FROM campaign_action_jobs").
		WithArgs("tenant-a", "campaign-command-stale-snapshot-0001").
		WillReturnError(sql.ErrNoRows)
	expectCampaignNotMerged(mock, "tenant-a", "campaign-a")
	mock.ExpectExec("INSERT INTO campaign_workbench_state").
		WithArgs("tenant-a", "campaign-a", "active", "analyst-a").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WithArgs("tenant-a", "campaign-a").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id"}).
			AddRow("active", "owner-a", int64(4), 1, "00000000-0000-4000-8000-000000000004"))
	mock.ExpectQuery("SELECT alert_id FROM campaign_alert_links").
		WithArgs("tenant-a", "campaign-a").
		WillReturnRows(sqlmock.NewRows([]string{"alert_id"}).AddRow("alert-1"))
	mock.ExpectRollback()

	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/campaign-a/actions", nil).WithContext(campaignV2Context())
	_, err = handler.commitCampaignAggregateV2Command(
		httpRequest.Context(), httpRequest, request, campaignActionSpecs[request.ActionID],
		"campaign-a", campaignDTO{CampaignID: "campaign-a", Status: "active", EventID: "ch-event-a", IngestTs: 100},
		"campaign-command-stale-snapshot-0001", requestSHA,
	)
	var commandErr *campaignCommandError
	require.ErrorAs(t, err, &commandErr)
	require.Equal(t, "CAMPAIGN_SNAPSHOT_CONFLICT", commandErr.code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCampaignAggregateV2IdempotentReplayDoesNotAppendAnotherAggregateEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	audit := &campaignV2AuditStub{}
	handler := NewSystemHandler(nil, nil, nil)
	handler.pgDB = db
	handler.campaignAuditWriter = audit
	request := campaignActionRequest{
		ActionID: "campaign-assign-owner", Target: "分派负责人",
		Metadata:   map[string]interface{}{"campaign_id": "campaign-a", "assignee": "owner-b", "dry_run": false},
		Simulation: boolPointer(false), DryRun: boolPointer(false), ExpectedRevision: int64Pointer(3),
		Reason: "交由二线分析员处理",
	}
	requestSHA, err := campaignCommandRequestSHA("campaign-a", request)
	require.NoError(t, err)
	createdAt := time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC)
	completedAt := createdAt.Add(time.Second)
	metadataJSON := `{"assignee":"owner-b","campaign_id":"campaign-a","dry_run":false}`
	resultJSON := `{"resource_revision":4}`

	mock.ExpectBegin()
	mock.ExpectQuery("FROM campaign_action_jobs").
		WithArgs("tenant-a", "campaign-command-key-0002").
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "tenant_id", "campaign_id", "action_id", "target", "metadata", "simulation", "dry_run", "status", "result",
			"error_message", "created_by", "created_at", "completed_at", "idempotency_key", "request_sha256", "expected_revision", "resource_revision", "reason",
		}).AddRow(
			"campaign-job-existing", "tenant-a", "campaign-a", "campaign-assign-owner", "分派负责人", metadataJSON, false, false, "succeeded", resultJSON,
			"", "analyst-a", createdAt, completedAt, "campaign-command-key-0002", requestSHA, int64(3), int64(4), "交由二线分析员处理",
		))
	mock.ExpectCommit()

	httpRequest := httptest.NewRequest(http.MethodPost, "/campaigns/campaign-a/actions", nil).WithContext(campaignV2Context())
	job, err := handler.commitCampaignAggregateV2Command(
		httpRequest.Context(), httpRequest, request, campaignActionSpecs[request.ActionID],
		"campaign-a", campaignDTO{CampaignID: "campaign-a", Status: "active"},
		"campaign-command-key-0002", requestSHA,
	)
	require.NoError(t, err)
	require.True(t, job.IdempotentReuse)
	require.Equal(t, "campaign-job-existing", job.JobID)
	require.Len(t, audit.records, 1)
	require.Equal(t, "CAMPAIGN_COMMAND_REUSED", audit.records[0].Action)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCampaignAggregateV2ReportRequiresBackfilledMembersAndRemainsAccepted(t *testing.T) {
	state := campaignAggregateState{Status: "investigating", Assignee: "owner-a", Revision: 4}
	request := campaignActionRequest{
		ActionID: "campaign-report-generate",
		Metadata: map[string]interface{}{"format": "pdf", "sections": []string{"证据链"}},
	}
	_, _, err := applyCampaignAggregateV2Command(&state, request, campaignDTO{Alerts: []string{"legacy-alert"}}, nil)
	var commandErr *campaignCommandError
	require.ErrorAs(t, err, &commandErr)
	require.Equal(t, "CAMPAIGN_MEMBERSHIP_BACKFILL_REQUIRED", commandErr.code)

	eventType, status, err := applyCampaignAggregateV2Command(&state, request, campaignDTO{}, []string{"alert-2", "alert-1"})
	require.NoError(t, err)
	require.Equal(t, "traffic.campaign.v2.ReportRequested", eventType)
	require.Equal(t, "accepted", status)
	snapshot := buildCampaignReportSnapshot(
		"tenant-a", "snapshot-a", "campaign:campaign-a:revision:4:source",
		campaignDTO{CampaignID: "campaign-a", TsStart: 10, TsEnd: 20}, state,
		[]string{"alert-2", "alert-1"}, request.Metadata,
	)
	_, firstSHA, err := canonicalCampaignSnapshot(snapshot)
	require.NoError(t, err)
	_, secondSHA, err := canonicalCampaignSnapshot(snapshot)
	require.NoError(t, err)
	require.Equal(t, firstSHA, secondSHA)
	require.Equal(t, []string{"alert-1", "alert-2"}, snapshot.MemberAlertIDs)
	require.Equal(t, "campaign:campaign-a:revision:4:source", snapshot.SourceSnapshotID)
}

func TestCampaignAggregateV2RejectsIllegalStateTransition(t *testing.T) {
	state := campaignAggregateState{Status: "active", Revision: 1}
	request := campaignActionRequest{ActionID: "campaign-status-change", Metadata: map[string]interface{}{"next_status": "contained"}}
	_, _, err := applyCampaignAggregateV2Command(&state, request, campaignDTO{}, nil)
	var commandErr *campaignCommandError
	require.ErrorAs(t, err, &commandErr)
	require.Equal(t, "INVALID_STATE_TRANSITION", commandErr.code)
	require.Equal(t, "active", state.Status)
}

func boolPointer(value bool) *bool { return &value }

func int64Pointer(value int64) *int64 { return &value }

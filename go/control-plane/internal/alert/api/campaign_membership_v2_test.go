package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func campaignMembershipTestContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "analyst-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-membership-v2")
	return ctx
}

func campaignMembershipHTTPRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil).WithContext(campaignMembershipTestContext())
}

func TestCommitCampaignMembershipV2LinksRelationAndAggregateAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))

	expectedCampaignRevision := int64(2)
	request := alertCampaignLinkRequest{
		CampaignID: "campaign-a", ExpectedRevision: int64Pointer(0),
		ExpectedCampaignRevision: &expectedCampaignRevision, Reason: "确认加入战役成员",
	}
	requestSHA, err := campaignMembershipRequestSHA(campaignMembershipLink, "tenant-a", "alert-a", request)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,result").
		WithArgs("tenant-a", "campaign-membership-link-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "result"}))
	mock.ExpectExec("INSERT INTO campaign_workbench_state").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WithArgs("tenant-a", "campaign-a").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id"}).AddRow("investigating", "owner-a", int64(2), 0, "00000000-0000-4000-8000-000000000002"))
	expectCampaignNotMerged(mock, "tenant-a", "campaign-a")
	mock.ExpectQuery("FROM campaign_alert_links WHERE tenant_id=\\$1 AND campaign_id=\\$2 AND alert_id=\\$3 FOR UPDATE").
		WithArgs("tenant-a", "campaign-a", "alert-a").
		WillReturnRows(sqlmock.NewRows([]string{"relation_id", "tenant_id", "alert_id", "campaign_id", "status", "revision", "campaign_revision", "created_at", "updated_at"}))
	mock.ExpectExec("INSERT INTO campaign_alert_links").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM campaign_alert_links").
		WithArgs("tenant-a", "campaign-a").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE campaign_workbench_state").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_alert_link_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_alert_link_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_aggregate_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_aggregate_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_membership_commands").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	link, err := handler.commitCampaignMembershipV2(
		campaignMembershipTestContext(), campaignMembershipHTTPRequest(http.MethodPost, "/alerts/alert-a/campaign-links"),
		campaignMembershipLink, "alert-a", request, "campaign-membership-link-key-0001", requestSHA,
	)
	require.NoError(t, err)
	require.Equal(t, "linked", link.Status)
	require.Equal(t, int64(1), link.Revision)
	require.Equal(t, int64(3), link.CampaignRevision)
	require.NotEmpty(t, link.EventID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommitCampaignMembershipV2ReplayDoesNotAppendSecondEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := alertCampaignLinkRequest{
		CampaignID: "campaign-a", ExpectedRevision: int64Pointer(0), Reason: "确认加入战役成员",
	}
	requestSHA, err := campaignMembershipRequestSHA(campaignMembershipLink, "tenant-a", "alert-a", request)
	require.NoError(t, err)
	stored, err := json.Marshal(alertCampaignLinkDTO{
		RelationID: "00000000-0000-4000-8000-000000000001", TenantID: "tenant-a",
		AlertID: "alert-a", CampaignID: "campaign-a", Status: "linked", Revision: 1,
		CampaignRevision: 3, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,result").
		WithArgs("tenant-a", "campaign-membership-link-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "result"}).AddRow(requestSHA, stored))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	replay, err := handler.commitCampaignMembershipV2(
		campaignMembershipTestContext(), campaignMembershipHTTPRequest(http.MethodPost, "/alerts/alert-a/campaign-links"),
		campaignMembershipLink, "alert-a", request, "campaign-membership-link-key-0001", requestSHA,
	)
	require.NoError(t, err)
	require.True(t, replay.IdempotentReuse)
	require.Equal(t, int64(3), replay.CampaignRevision)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommitCampaignMembershipV2UnlinksAndDecrementsAggregateMemberCount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	expectedCampaignRevision := int64(3)
	request := alertCampaignLinkRequest{
		CampaignID: "campaign-a", ExpectedRevision: int64Pointer(1),
		ExpectedCampaignRevision: &expectedCampaignRevision, Reason: "确认移出战役成员",
	}
	requestSHA, err := campaignMembershipRequestSHA(campaignMembershipUnlink, "tenant-a", "alert-a", request)
	require.NoError(t, err)
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,result").WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "result"}))
	mock.ExpectExec("INSERT INTO campaign_workbench_state").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id"}).AddRow("closed", "owner-a", int64(3), 1, "00000000-0000-4000-8000-000000000003"))
	expectCampaignNotMerged(mock, "tenant-a", "campaign-a")
	mock.ExpectQuery("FROM campaign_alert_links WHERE tenant_id=\\$1 AND campaign_id=\\$2 AND alert_id=\\$3 FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"relation_id", "tenant_id", "alert_id", "campaign_id", "status", "revision", "campaign_revision", "created_at", "updated_at"}).
			AddRow("00000000-0000-4000-8000-000000000001", "tenant-a", "alert-a", "campaign-a", "linked", int64(1), int64(3), now, now))
	mock.ExpectExec("UPDATE campaign_alert_links").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM campaign_alert_links").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE campaign_workbench_state").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_alert_link_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_alert_link_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_aggregate_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_aggregate_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_membership_commands").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	link, err := handler.commitCampaignMembershipV2(
		campaignMembershipTestContext(), campaignMembershipHTTPRequest(http.MethodDelete, "/alerts/alert-a/campaign-links/campaign-a"),
		campaignMembershipUnlink, "alert-a", request, "campaign-membership-unlink-key-001", requestSHA,
	)
	require.NoError(t, err)
	require.Equal(t, "unlinked", link.Status)
	require.Equal(t, int64(2), link.Revision)
	require.Equal(t, int64(4), link.CampaignRevision)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommitCampaignMembershipV2RejectsStaleCampaignRevisionBeforeRelationMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	expectedCampaignRevision := int64(2)
	request := alertCampaignLinkRequest{
		CampaignID: "campaign-a", ExpectedRevision: int64Pointer(0),
		ExpectedCampaignRevision: &expectedCampaignRevision, Reason: "陈旧战役版本不得写入",
	}
	requestSHA, err := campaignMembershipRequestSHA(campaignMembershipLink, "tenant-a", "alert-a", request)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT request_sha256,result").WillReturnRows(sqlmock.NewRows([]string{"request_sha256", "result"}))
	mock.ExpectExec("INSERT INTO campaign_workbench_state").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id"}).AddRow("active", "", int64(3), 0, "00000000-0000-4000-8000-000000000003"))
	expectCampaignNotMerged(mock, "tenant-a", "campaign-a")
	mock.ExpectRollback()

	_, err = handler.commitCampaignMembershipV2(
		campaignMembershipTestContext(), campaignMembershipHTTPRequest(http.MethodPost, "/alerts/alert-a/campaign-links"),
		campaignMembershipLink, "alert-a", request, "campaign-membership-stale-key-01", requestSHA,
	)
	var commandErr *campaignMembershipCommandError
	require.True(t, errors.As(err, &commandErr))
	require.Equal(t, "CAMPAIGN_REVISION_CONFLICT", commandErr.code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectCampaignNotMerged(mock sqlmock.Sqlmock, tenantID, campaignID string) {
	mock.ExpectQuery("SELECT target_campaign_id FROM campaign_merge_receipts").
		WithArgs(tenantID, campaignID).
		WillReturnError(sql.ErrNoRows)
}

func TestListCampaignMembersReturnsSameSnapshotAndExplicitReconciliationState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	handler.SetCampaignAggregateV2FeatureFlag(true)
	handler.lookupCampaign = func(context.Context, string, string) (campaignDTO, error) {
		return campaignDTO{CampaignID: "campaign-a", EventID: "ch-event-a", IngestTs: 100}, nil
	}
	mock.ExpectQuery(regexp.QuoteMeta(requiredPostgresColumnsQuery)).
		WillReturnRows(requiredColumnRows(campaignAggregateV2RequiredColumns))
	mock.ExpectBegin()
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id", "updated_at"}).
			AddRow("investigating", "owner-a", int64(4), 2, "00000000-0000-4000-8000-000000000010", now))
	mock.ExpectQuery("SELECT relation_id::text,tenant_id,alert_id,campaign_id,status,revision,campaign_revision,created_at,updated_at").
		WillReturnRows(sqlmock.NewRows([]string{"relation_id", "tenant_id", "alert_id", "campaign_id", "status", "revision", "campaign_revision", "created_at", "updated_at"}).
			AddRow("00000000-0000-4000-8000-000000000001", "tenant-a", "alert-a", "campaign-a", "linked", int64(1), int64(3), now, now).
			AddRow("00000000-0000-4000-8000-000000000002", "tenant-a", "alert-b", "campaign-a", "linked", int64(2), int64(4), now, now))
	mock.ExpectQuery("SELECT count\\(\\*\\),COALESCE\\(max\\(revision\\),0\\)").
		WillReturnRows(sqlmock.NewRows([]string{"count", "max", "legacy"}).AddRow(2, int64(2), 0))
	mock.ExpectCommit()

	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	request := campaignActionRequestWithPermissions(
		httptest.NewRequest(http.MethodGet, "/campaigns/campaign-a/members?limit=20", nil),
		authmodel.ScopeCampaignRead,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"campaign_revision":4`)
	require.Contains(t, recorder.Body.String(), `"member_count":2`)
	require.Contains(t, recorder.Body.String(), `"snapshot_id":"campaign:campaign-a:revision:4:`)
	require.Contains(t, recorder.Body.String(), `"partial":false`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListCampaignMembersRejectsStaleLifecycleSnapshotBeforeReadingMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	handler.SetCampaignAggregateV2FeatureFlag(true)
	handler.lookupCampaign = func(context.Context, string, string) (campaignDTO, error) {
		return campaignDTO{CampaignID: "campaign-a", EventID: "ch-event-a", IngestTs: 100}, nil
	}
	mock.ExpectQuery(regexp.QuoteMeta(requiredPostgresColumnsQuery)).
		WillReturnRows(requiredColumnRows(campaignAggregateV2RequiredColumns))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,assignee,state_version,member_count").
		WillReturnRows(sqlmock.NewRows([]string{"status", "assignee", "state_version", "member_count", "last_event_id", "updated_at"}).
			AddRow("investigating", "owner-a", int64(5), 2, "00000000-0000-4000-8000-000000000011", time.Now().UTC()))
	mock.ExpectRollback()

	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	request := campaignActionRequestWithPermissions(
		httptest.NewRequest(http.MethodGet, "/campaigns/campaign-a/members?snapshot_id=campaign%3Acampaign-a%3Arevision%3A4%3A0000000000000000", nil),
		authmodel.ScopeCampaignRead,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"code":"CAMPAIGN_SNAPSHOT_CONFLICT"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUnlinkCampaignMembershipIsDefaultOff(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	router := mux.NewRouter()
	handler.RegisterRoutes(router)
	request := httptest.NewRequest(http.MethodDelete, "/alerts/alert-a/campaign-links/campaign-a", strings.NewReader(`{"expected_revision":1,"reason":"确认解除战役关系"}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

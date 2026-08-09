package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCampaignCommandCompatibilityAddsMissingMetadata(t *testing.T) {
	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest("POST", "/campaigns/campaign-a/actions", nil)
	request := campaignActionRequest{}

	normalizeCampaignCommandCompatibility(recorder, httpRequest, &request, 7)

	require.True(t, request.CompatibilityMode)
	require.NotNil(t, request.ExpectedRevision)
	require.Equal(t, int64(7), *request.ExpectedRevision)
	require.Equal(t, "compatibility campaign action", request.Reason)
	require.Len(t, httpRequest.Header.Get("Idempotency-Key"), len("compat-campaign-")+36)
	require.Equal(t, "true", recorder.Header().Get("X-Compatibility-Mode"))
	require.Equal(t, httpRequest.Header.Get("Idempotency-Key"), recorder.Header().Get("Idempotency-Key"))
}

func TestNormalizeCampaignCommandCompatibilityPreservesStrictMetadata(t *testing.T) {
	revision := int64(9)
	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest("POST", "/campaigns/campaign-a/actions", nil)
	httpRequest.Header.Set("Idempotency-Key", "campaign-strict-key-0001")
	request := campaignActionRequest{ExpectedRevision: &revision, Reason: "strict command reason"}

	normalizeCampaignCommandCompatibility(recorder, httpRequest, &request, 3)

	require.False(t, request.CompatibilityMode)
	require.Equal(t, int64(9), *request.ExpectedRevision)
	require.Equal(t, "strict command reason", request.Reason)
	require.Empty(t, recorder.Header().Get("X-Compatibility-Mode"))
}

func TestEnrichCampaignWorkbenchStatesOverridesOperationalFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	updatedAt := time.Date(2026, 7, 23, 2, 3, 4, 0, time.UTC)
	mock.ExpectQuery("SELECT campaign_id, assignee, status, state_version, member_count").
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"campaign_id", "assignee", "status", "state_version", "member_count", "last_event_id", "updated_at"}).
			AddRow("campaign-a", "sec_analyst", "contained", 7, 2, "00000000-0000-4000-8000-000000000007", updatedAt))
	campaigns, err := handler.enrichCampaignWorkbenchStates(context.Background(), "tenant-a", []campaignDTO{
		{CampaignID: "campaign-a", ActivityStatus: "active", Status: "active"},
		{CampaignID: "campaign-b", ActivityStatus: "investigating", Status: "investigating"},
	})
	require.NoError(t, err)
	require.Equal(t, "contained", campaigns[0].Status)
	require.Equal(t, "active", campaigns[0].ActivityStatus)
	require.Equal(t, "sec_analyst", campaigns[0].Assignee)
	require.Equal(t, int64(7), campaigns[0].StateVersion)
	require.Equal(t, 2, campaigns[0].MemberCount)
	require.Contains(t, campaigns[0].SnapshotID, "campaign:campaign-a:revision:7:")
	require.Len(t, campaigns[0].SnapshotSHA256, 64)
	require.Equal(t, "investigating", campaigns[1].Status)
	require.Contains(t, campaigns[1].SnapshotID, "campaign:campaign-b:revision:0:")
	require.NoError(t, mock.ExpectationsWereMet())
}

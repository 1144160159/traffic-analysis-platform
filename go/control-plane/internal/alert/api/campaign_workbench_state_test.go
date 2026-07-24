package api

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestApplyCampaignStatusMutationPersistsVersionedState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	updatedAt := time.Date(2026, 7, 23, 1, 2, 3, 0, time.UTC)
	mock.ExpectQuery("INSERT INTO campaign_workbench_state").
		WithArgs("tenant-a", "campaign-a", "investigating", "analyst-a").
		WillReturnRows(sqlmock.NewRows([]string{"state_version", "updated_at"}).AddRow(3, updatedAt))
	mock.ExpectRollback()
	job := campaignActionJob{
		TenantID: "tenant-a", CampaignID: "campaign-a", ActionID: "campaign-status-change",
		Metadata: map[string]interface{}{"next_status": "investigating"},
		Result:   map[string]interface{}{}, CreatedBy: "analyst-a",
	}
	require.NoError(t, applyCampaignActionMutation(context.Background(), tx, &job))
	require.Equal(t, "investigating", job.Result["campaign_status"])
	require.Equal(t, int64(3), job.Result["state_version"])
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyCampaignReportMutationPersistsReport(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO campaign_reports").
		WithArgs("report-a", "tenant-a", "campaign-a", "pdf", `["攻击阶段","证据链"]`, 12, "analyst-a").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()
	job := campaignActionJob{
		TenantID: "tenant-a", CampaignID: "campaign-a", ActionID: "campaign-report-generate",
		Metadata: map[string]interface{}{"format": "pdf", "sections": []string{"攻击阶段", "证据链"}, "evidence_count": float64(12)},
		Result:   map[string]interface{}{"report_id": "report-a"}, CreatedBy: "analyst-a",
	}
	require.NoError(t, applyCampaignActionMutation(context.Background(), tx, &job))
	require.Equal(t, "completed", job.Result["report_status"])
	require.Equal(t, "pdf", job.Result["report_format"])
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnrichCampaignWorkbenchStatesOverridesOperationalFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	updatedAt := time.Date(2026, 7, 23, 2, 3, 4, 0, time.UTC)
	mock.ExpectQuery("SELECT campaign_id, assignee, status, state_version, updated_at").
		WithArgs("tenant-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"campaign_id", "assignee", "status", "state_version", "updated_at"}).
			AddRow("campaign-a", "sec_analyst", "contained", 7, updatedAt))
	campaigns, err := handler.enrichCampaignWorkbenchStates(context.Background(), "tenant-a", []campaignDTO{
		{CampaignID: "campaign-a", ActivityStatus: "active", Status: "active"},
		{CampaignID: "campaign-b", ActivityStatus: "investigating", Status: "investigating"},
	})
	require.NoError(t, err)
	require.Equal(t, "contained", campaigns[0].Status)
	require.Equal(t, "active", campaigns[0].ActivityStatus)
	require.Equal(t, "sec_analyst", campaigns[0].Assignee)
	require.Equal(t, int64(7), campaigns[0].StateVersion)
	require.Equal(t, "investigating", campaigns[1].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

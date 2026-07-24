package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

const campaignWorkbenchSchema = `
CREATE TABLE IF NOT EXISTS campaign_workbench_state (
  tenant_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  assignee TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','investigating','contained','closed')),
  state_version BIGINT NOT NULL DEFAULT 1,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, campaign_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_workbench_state_tenant_status
  ON campaign_workbench_state (tenant_id, status, updated_at DESC);
CREATE TABLE IF NOT EXISTS campaign_reports (
  report_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  format TEXT NOT NULL DEFAULT 'pdf' CHECK (format IN ('pdf','word','json')),
  status TEXT NOT NULL DEFAULT 'completed'
    CHECK (status IN ('queued','running','completed','failed')),
  sections JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_count INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_campaign_reports_campaign_time
  ON campaign_reports (tenant_id, campaign_id, created_at DESC);`

func ensureCampaignWorkbenchSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("postgres is not configured")
	}
	_, err := db.ExecContext(ctx, campaignWorkbenchSchema)
	return err
}

func applyCampaignActionMutation(ctx context.Context, executor campaignTransaction, job *campaignActionJob) error {
	switch job.ActionID {
	case "campaign-assign-owner":
		assignee := campaignMetadataString(job.Metadata, "assignee")
		if assignee == "" {
			return errors.New("assignee is required")
		}
		var version int64
		var updatedAt time.Time
		err := executor.QueryRowContext(ctx, `
			INSERT INTO campaign_workbench_state
			(tenant_id, campaign_id, assignee, status, state_version, updated_by, updated_at)
			VALUES ($1,$2,$3,'active',1,$4,now())
			ON CONFLICT (tenant_id, campaign_id) DO UPDATE SET
				assignee=EXCLUDED.assignee,
				state_version=campaign_workbench_state.state_version+1,
				updated_by=EXCLUDED.updated_by,
				updated_at=now()
			RETURNING state_version, updated_at`,
			job.TenantID, job.CampaignID, assignee, job.CreatedBy,
		).Scan(&version, &updatedAt)
		if err != nil {
			return err
		}
		job.Result["assignee"] = assignee
		job.Result["state_version"] = version
		job.Result["updated_at"] = updatedAt.UTC().Format(time.RFC3339Nano)
	case "campaign-status-change":
		status := strings.ToLower(campaignMetadataString(job.Metadata, "next_status"))
		if !validCampaignWorkbenchStatus(status) {
			return fmt.Errorf("unsupported next_status %q", status)
		}
		var version int64
		var updatedAt time.Time
		err := executor.QueryRowContext(ctx, `
			INSERT INTO campaign_workbench_state
			(tenant_id, campaign_id, assignee, status, state_version, updated_by, updated_at)
			VALUES ($1,$2,'',$3,1,$4,now())
			ON CONFLICT (tenant_id, campaign_id) DO UPDATE SET
				status=EXCLUDED.status,
				state_version=campaign_workbench_state.state_version+1,
				updated_by=EXCLUDED.updated_by,
				updated_at=now()
			RETURNING state_version, updated_at`,
			job.TenantID, job.CampaignID, status, job.CreatedBy,
		).Scan(&version, &updatedAt)
		if err != nil {
			return err
		}
		job.Result["campaign_status"] = status
		job.Result["state_version"] = version
		job.Result["updated_at"] = updatedAt.UTC().Format(time.RFC3339Nano)
	case "campaign-report-generate":
		format := strings.ToLower(campaignMetadataString(job.Metadata, "format"))
		if format == "" {
			format = "pdf"
		}
		if format != "pdf" && format != "word" && format != "json" {
			return fmt.Errorf("unsupported report format %q", format)
		}
		sections, err := json.Marshal(job.Metadata["sections"])
		if err != nil || string(sections) == "null" {
			sections = []byte(`[]`)
		}
		reportID := campaignMetadataString(job.Result, "report_id")
		if reportID == "" {
			return errors.New("report_id is required")
		}
		evidenceCount := campaignMetadataInt(job.Metadata, "evidence_count")
		_, err = executor.ExecContext(ctx, `
			INSERT INTO campaign_reports
			(report_id, tenant_id, campaign_id, format, status, sections, evidence_count, created_by, completed_at)
			VALUES ($1,$2,$3,$4,'completed',$5::jsonb,$6,$7,now())`,
			reportID, job.TenantID, job.CampaignID, format, string(sections), evidenceCount, job.CreatedBy,
		)
		if err != nil {
			return err
		}
		job.Result["report_status"] = "completed"
		job.Result["report_format"] = format
	}
	return nil
}

func (h *SystemHandler) enrichCampaignWorkbenchStates(ctx context.Context, tenantID string, campaigns []campaignDTO) ([]campaignDTO, error) {
	if h.pgDB == nil || len(campaigns) == 0 {
		return campaigns, nil
	}
	ids := make([]string, 0, len(campaigns))
	for _, campaign := range campaigns {
		ids = append(ids, campaign.CampaignID)
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT campaign_id, assignee, status, state_version, updated_at
		FROM campaign_workbench_state
		WHERE tenant_id=$1 AND campaign_id=ANY($2)`, tenantID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type state struct {
		assignee  string
		status    string
		version   int64
		updatedAt time.Time
	}
	states := make(map[string]state, len(campaigns))
	for rows.Next() {
		var campaignID string
		var item state
		if err := rows.Scan(&campaignID, &item.assignee, &item.status, &item.version, &item.updatedAt); err != nil {
			return nil, err
		}
		states[campaignID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range campaigns {
		if item, ok := states[campaigns[index].CampaignID]; ok {
			campaigns[index].Assignee = item.assignee
			campaigns[index].Status = item.status
			campaigns[index].StateVersion = item.version
			campaigns[index].WorkbenchUpdatedAt = item.updatedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return campaigns, nil
}

func campaignMetadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func campaignMetadataInt(metadata map[string]interface{}, key string) int {
	switch value := metadata[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func validCampaignWorkbenchStatus(status string) bool {
	switch status {
	case "active", "investigating", "contained", "closed":
		return true
	default:
		return false
	}
}

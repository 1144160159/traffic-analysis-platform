package api

import (
	"context"
	"strings"
	"time"

	"github.com/lib/pq"
)

func (h *SystemHandler) enrichCampaignWorkbenchStates(ctx context.Context, tenantID string, campaigns []campaignDTO) ([]campaignDTO, error) {
	if len(campaigns) == 0 {
		return campaigns, nil
	}
	for index := range campaigns {
		if campaigns[index].TenantID == "" {
			campaigns[index].TenantID = tenantID
		}
		campaigns[index].MemberCount = len(campaigns[index].Alerts)
	}
	if h.pgDB == nil {
		for index := range campaigns {
			if err := stampCampaignLifecycleSnapshot(&campaigns[index]); err != nil {
				return nil, err
			}
		}
		return campaigns, nil
	}
	ids := make([]string, 0, len(campaigns))
	for _, campaign := range campaigns {
		ids = append(ids, campaign.CampaignID)
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT campaign_id, assignee, status, state_version, member_count,
		       COALESCE(last_event_id::text,''), updated_at
		FROM campaign_workbench_state
		WHERE tenant_id=$1 AND campaign_id=ANY($2)`, tenantID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type state struct {
		assignee    string
		status      string
		version     int64
		memberCount int
		lastEventID string
		updatedAt   time.Time
	}
	states := make(map[string]state, len(campaigns))
	for rows.Next() {
		var campaignID string
		var item state
		if err := rows.Scan(&campaignID, &item.assignee, &item.status, &item.version, &item.memberCount, &item.lastEventID, &item.updatedAt); err != nil {
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
			campaigns[index].MemberCount = item.memberCount
			campaigns[index].LastEventID = item.lastEventID
			campaigns[index].WorkbenchUpdatedAt = item.updatedAt.UTC().Format(time.RFC3339Nano)
		}
		if err := stampCampaignLifecycleSnapshot(&campaigns[index]); err != nil {
			return nil, err
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

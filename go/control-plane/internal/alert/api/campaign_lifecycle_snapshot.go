package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const campaignLifecycleContractVersion = 3

// campaignLifecycleSnapshotMaterial is the cross-store identity shared by the
// campaign list, detail, membership and report-request surfaces. PostgreSQL is
// authoritative for mutable lifecycle state and membership; the ClickHouse
// event and ingest identities bind the analytical root used by the read model.
// Arrays are sorted before hashing so the identity does not depend on storage
// iteration order.
type campaignLifecycleSnapshotMaterial struct {
	TenantID       string   `json:"tenant_id"`
	CampaignID     string   `json:"campaign_id"`
	StateVersion   int64    `json:"state_version"`
	MemberCount    int      `json:"member_count"`
	LastEventID    string   `json:"last_event_id"`
	SourceEventID  string   `json:"source_event_id"`
	SourceIngestTS int64    `json:"source_ingest_ts"`
	TSStart        int64    `json:"ts_start"`
	TSEnd          int64    `json:"ts_end"`
	Status         string   `json:"status"`
	Assignee       string   `json:"assignee"`
	Summary        string   `json:"summary"`
	Score          float64  `json:"score"`
	CampaignType   string   `json:"campaign_type"`
	Entities       []string `json:"entities"`
	AttackPhases   []string `json:"attack_phases"`
	RuleIDs        []string `json:"rule_ids"`
	ModelIDs       []string `json:"model_ids"`
}

func stampCampaignLifecycleSnapshot(campaign *campaignDTO) error {
	if campaign == nil || strings.TrimSpace(campaign.TenantID) == "" || strings.TrimSpace(campaign.CampaignID) == "" {
		return fmt.Errorf("campaign snapshot identity requires tenant and campaign")
	}
	material := campaignLifecycleSnapshotMaterial{
		TenantID:       campaign.TenantID,
		CampaignID:     campaign.CampaignID,
		StateVersion:   campaign.StateVersion,
		MemberCount:    campaign.MemberCount,
		LastEventID:    campaign.LastEventID,
		SourceEventID:  campaign.EventID,
		SourceIngestTS: campaign.IngestTs,
		TSStart:        campaign.TsStart,
		TSEnd:          campaign.TsEnd,
		Status:         campaign.Status,
		Assignee:       campaign.Assignee,
		Summary:        campaign.Summary,
		Score:          campaign.Score,
		CampaignType:   campaign.CampaignType,
		Entities:       sortedCampaignSnapshotStrings(campaign.Entities),
		AttackPhases:   sortedCampaignSnapshotStrings(campaign.AttackPhases),
		RuleIDs:        sortedCampaignSnapshotStrings(campaign.RuleIDs),
		ModelIDs:       sortedCampaignSnapshotStrings(campaign.ModelIDs),
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	campaign.SnapshotSHA256 = hex.EncodeToString(digest[:])
	campaign.SnapshotID = fmt.Sprintf(
		"campaign:%s:revision:%d:%s",
		campaign.CampaignID,
		campaign.StateVersion,
		campaign.SnapshotSHA256[:16],
	)
	return nil
}

func sortedCampaignSnapshotStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func requestedCampaignSnapshotID(raw string) string {
	return strings.TrimSpace(raw)
}

func campaignSnapshotMatches(expected string, campaign campaignDTO) bool {
	expected = requestedCampaignSnapshotID(expected)
	return expected == "" || expected == campaign.SnapshotID
}

func campaignListSnapshotIdentity(campaigns []campaignDTO, total int64, limit, offset int) (string, string, error) {
	items := make([]map[string]interface{}, 0, len(campaigns))
	for _, campaign := range campaigns {
		items = append(items, map[string]interface{}{
			"campaign_id":   campaign.CampaignID,
			"snapshot_id":   campaign.SnapshotID,
			"state_version": campaign.StateVersion,
		})
	}
	encoded, err := json.Marshal(map[string]interface{}{
		"total": total, "limit": limit, "offset": offset, "items": items,
	})
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(encoded)
	sha := hex.EncodeToString(digest[:])
	return "campaign-list:" + sha[:16], sha, nil
}

func campaignListSourceWatermarks(campaigns []campaignDTO) map[string]string {
	var maxStateRevision, maxIngestTS int64
	for _, campaign := range campaigns {
		if campaign.StateVersion > maxStateRevision {
			maxStateRevision = campaign.StateVersion
		}
		if campaign.IngestTs > maxIngestTS {
			maxIngestTS = campaign.IngestTs
		}
	}
	return map[string]string{
		"postgresql.campaign_workbench_state.max_revision": strconv.FormatInt(maxStateRevision, 10),
		"clickhouse.campaigns.max_ingest_ts":               strconv.FormatInt(maxIngestTS, 10),
	}
}

type campaignReportSnapshotSummary struct {
	ReportID         string     `json:"report_id"`
	JobID            string     `json:"job_id"`
	Format           string     `json:"format"`
	Status           string     `json:"status"`
	CampaignRevision int64      `json:"campaign_revision"`
	SourceSnapshotID string     `json:"source_snapshot_id"`
	SnapshotID       string     `json:"snapshot_id"`
	SnapshotSHA256   string     `json:"snapshot_sha256"`
	ArtifactSHA256   string     `json:"artifact_sha256,omitempty"`
	SizeBytes        int64      `json:"size_bytes"`
	Attempts         int        `json:"attempts"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type campaignLifecycleRead struct {
	Campaign                 campaignDTO
	MemberAlertIDs           []string
	Reports                  []campaignReportSnapshotSummary
	ActualMemberCount        int
	MaxRelationRevision      int64
	LegacyUnboundMemberCount int
	StateMissing             bool
}

func (h *SystemHandler) loadCampaignLifecycleRead(
	ctx context.Context,
	campaign campaignDTO,
) (campaignLifecycleRead, error) {
	result := campaignLifecycleRead{
		Campaign:       campaign,
		MemberAlertIDs: append([]string(nil), campaign.Alerts...),
		Reports:        []campaignReportSnapshotSummary{},
	}
	result.Campaign.MemberCount = len(result.MemberAlertIDs)
	if h.pgDB == nil || !h.campaignAggregateV2 {
		if err := stampCampaignLifecycleSnapshot(&result.Campaign); err != nil {
			return campaignLifecycleRead{}, err
		}
		result.ActualMemberCount = len(result.MemberAlertIDs)
		return result, nil
	}
	if err := verifyCampaignAggregateV2Schema(ctx, h.pgDB); err != nil {
		return campaignLifecycleRead{}, err
	}
	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return campaignLifecycleRead{}, err
	}
	defer tx.Rollback()
	var updatedAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT assignee,status,state_version,member_count,
		COALESCE(last_event_id::text,''),updated_at
		FROM campaign_workbench_state WHERE tenant_id=$1 AND campaign_id=$2`,
		campaign.TenantID, campaign.CampaignID,
	).Scan(
		&result.Campaign.Assignee,
		&result.Campaign.Status,
		&result.Campaign.StateVersion,
		&result.Campaign.MemberCount,
		&result.Campaign.LastEventID,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		result.StateMissing = true
		result.Campaign.MemberCount = len(campaign.Alerts)
	} else if err != nil {
		return campaignLifecycleRead{}, err
	} else {
		result.Campaign.WorkbenchUpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	}

	memberRows, err := tx.QueryContext(ctx, `SELECT alert_id,revision,campaign_revision
		FROM campaign_alert_links
		WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'
		ORDER BY alert_id`, campaign.TenantID, campaign.CampaignID)
	if err != nil {
		return campaignLifecycleRead{}, err
	}
	result.MemberAlertIDs = []string{}
	for memberRows.Next() {
		var alertID string
		var relationRevision, campaignRevision int64
		if err := memberRows.Scan(&alertID, &relationRevision, &campaignRevision); err != nil {
			memberRows.Close()
			return campaignLifecycleRead{}, err
		}
		result.MemberAlertIDs = append(result.MemberAlertIDs, alertID)
		if relationRevision > result.MaxRelationRevision {
			result.MaxRelationRevision = relationRevision
		}
		if campaignRevision == 0 {
			result.LegacyUnboundMemberCount++
		}
	}
	if err := memberRows.Err(); err != nil {
		memberRows.Close()
		return campaignLifecycleRead{}, err
	}
	memberRows.Close()
	result.ActualMemberCount = len(result.MemberAlertIDs)
	result.Campaign.Alerts = append([]string(nil), result.MemberAlertIDs...)

	reportRows, err := tx.QueryContext(ctx, `SELECT report_id,COALESCE(job_id,''),format,status,
		campaign_revision,COALESCE(snapshot->>'source_snapshot_id',''),
		COALESCE(snapshot_id::text,''),snapshot_sha256,artifact_sha256,
		size_bytes,attempts,error_message,created_at,updated_at,completed_at
		FROM campaign_reports
		WHERE tenant_id=$1 AND campaign_id=$2
		ORDER BY created_at DESC,report_id DESC LIMIT 50`, campaign.TenantID, campaign.CampaignID)
	if err != nil {
		return campaignLifecycleRead{}, err
	}
	for reportRows.Next() {
		var item campaignReportSnapshotSummary
		var completedAt sql.NullTime
		if err := reportRows.Scan(
			&item.ReportID, &item.JobID, &item.Format, &item.Status,
			&item.CampaignRevision, &item.SourceSnapshotID,
			&item.SnapshotID, &item.SnapshotSHA256, &item.ArtifactSHA256,
			&item.SizeBytes, &item.Attempts, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt, &completedAt,
		); err != nil {
			reportRows.Close()
			return campaignLifecycleRead{}, err
		}
		if completedAt.Valid {
			value := completedAt.Time
			item.CompletedAt = &value
		}
		result.Reports = append(result.Reports, item)
	}
	if err := reportRows.Err(); err != nil {
		reportRows.Close()
		return campaignLifecycleRead{}, err
	}
	reportRows.Close()
	if err := stampCampaignLifecycleSnapshot(&result.Campaign); err != nil {
		return campaignLifecycleRead{}, err
	}
	if err := tx.Commit(); err != nil {
		return campaignLifecycleRead{}, err
	}
	return result, nil
}

func (read campaignLifecycleRead) missingSections() []string {
	missing := make([]string, 0, 3)
	if read.StateMissing || read.Campaign.MemberCount != read.ActualMemberCount {
		missing = append(missing, "campaign_state_reconcile")
	}
	if read.LegacyUnboundMemberCount > 0 {
		missing = append(missing, "membership_backfill")
	}
	return missing
}

func (read campaignLifecycleRead) sourceWatermarks() map[string]string {
	return map[string]string{
		"postgresql.campaign_workbench_state.revision":     strconv.FormatInt(read.Campaign.StateVersion, 10),
		"postgresql.campaign_workbench_state.member_count": strconv.Itoa(read.Campaign.MemberCount),
		"postgresql.campaign_alert_links.member_count":     strconv.Itoa(read.ActualMemberCount),
		"postgresql.campaign_alert_links.revision":         strconv.FormatInt(read.MaxRelationRevision, 10),
		"clickhouse.campaigns.event_id":                    read.Campaign.EventID,
		"clickhouse.campaigns.ingest_ts":                   strconv.FormatInt(read.Campaign.IngestTs, 10),
	}
}

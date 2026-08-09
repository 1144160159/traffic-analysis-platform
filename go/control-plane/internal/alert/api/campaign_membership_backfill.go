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
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	campaignMembershipBackfillContractVersion = 1
	campaignMembershipBackfillMaxCampaigns    = 100
	campaignMembershipBackfillMaxMembers      = 1000
)

type CampaignMembershipBackfillSource struct {
	Kind       string    `json:"kind"`
	URI        string    `json:"uri"`
	SHA256     string    `json:"sha256"`
	SnapshotID string    `json:"snapshot_id"`
	AsOf       time.Time `json:"as_of"`
}

type CampaignMembershipBackfillCampaign struct {
	CampaignID       string   `json:"campaign_id"`
	ExpectedRevision int64    `json:"expected_campaign_revision"`
	AlertIDs         []string `json:"alert_ids"`
}

type CampaignMembershipBackfillManifest struct {
	ContractVersion int                                  `json:"contract_version"`
	RunID           string                               `json:"run_id"`
	TenantID        string                               `json:"tenant_id"`
	Source          CampaignMembershipBackfillSource     `json:"source"`
	Reason          string                               `json:"reason"`
	CreatedBy       string                               `json:"created_by"`
	Campaigns       []CampaignMembershipBackfillCampaign `json:"campaigns"`
}

type CampaignMembershipBackfillResult struct {
	RunID                  string `json:"run_id"`
	TenantID               string `json:"tenant_id"`
	ManifestSHA256         string `json:"manifest_sha256"`
	Status                 string `json:"status"`
	CampaignCount          int    `json:"campaign_count"`
	SourceMemberCount      int    `json:"source_member_count"`
	CompletedCampaignCount int    `json:"completed_campaign_count"`
	FailedCampaignCount    int    `json:"failed_campaign_count"`
	InsertedCount          int    `json:"inserted_count"`
	BoundCount             int    `json:"bound_count"`
	UnchangedCount         int    `json:"unchanged_count"`
	SkippedUnlinkedCount   int    `json:"skipped_unlinked_count"`
}

type campaignMembershipBackfillRelation struct {
	RelationID       string
	AlertID          string
	Status           string
	Revision         int64
	CampaignRevision int64
}

type campaignMembershipBackfillCounts struct {
	Inserted        int
	Bound           int
	Unchanged       int
	SkippedUnlinked int
}

// RunCampaignMembershipBackfill applies one immutable ClickHouse export
// manifest. Each campaign is committed independently so a rerun with the same
// run_id and manifest resumes after the last completed campaign.
func RunCampaignMembershipBackfill(
	ctx context.Context,
	db *sql.DB,
	logger *zap.Logger,
	manifest CampaignMembershipBackfillManifest,
) (CampaignMembershipBackfillResult, error) {
	if db == nil {
		return CampaignMembershipBackfillResult{}, errors.New("campaign membership backfill database is required")
	}
	canonical, sourceMemberCount, err := canonicalCampaignMembershipBackfillManifest(manifest)
	if err != nil {
		return CampaignMembershipBackfillResult{}, err
	}
	if err := json.Unmarshal(canonical, &manifest); err != nil {
		return CampaignMembershipBackfillResult{}, err
	}
	digest := sha256.Sum256(canonical)
	manifestSHA := hex.EncodeToString(digest[:])
	manifestJSON := string(canonical)
	runID := manifest.RunID

	if _, err := db.ExecContext(ctx, `INSERT INTO campaign_membership_backfill_runs
		(run_id,tenant_id,source_kind,source_uri,source_sha256,source_snapshot_id,source_as_of,
		 manifest,manifest_sha256,reason,status,campaign_count,source_member_count,created_by)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,'running',$11,$12,$13)
		ON CONFLICT (run_id) DO NOTHING`, runID, manifest.TenantID, manifest.Source.Kind,
		manifest.Source.URI, manifest.Source.SHA256, manifest.Source.SnapshotID, manifest.Source.AsOf,
		manifestJSON, manifestSHA, manifest.Reason, len(manifest.Campaigns), sourceMemberCount,
		manifest.CreatedBy); err != nil {
		return CampaignMembershipBackfillResult{}, err
	}
	var storedTenantID, storedManifestSHA string
	if err := db.QueryRowContext(ctx, `SELECT tenant_id,manifest_sha256
		FROM campaign_membership_backfill_runs WHERE run_id=$1::uuid`, runID).
		Scan(&storedTenantID, &storedManifestSHA); err != nil {
		return CampaignMembershipBackfillResult{}, err
	}
	if storedTenantID != manifest.TenantID || storedManifestSHA != manifestSHA {
		return CampaignMembershipBackfillResult{}, fmt.Errorf("run_id %s is already bound to a different immutable manifest", runID)
	}

	auditWriter := NewAlertActionAuditWriter(db, logger)
	for _, campaign := range manifest.Campaigns {
		completed, err := campaignMembershipBackfillCampaignCompleted(ctx, db, runID, campaign.CampaignID)
		if err != nil {
			return CampaignMembershipBackfillResult{}, err
		}
		if completed {
			continue
		}
		err = applyCampaignMembershipBackfillCampaign(ctx, db, auditWriter, manifest, manifestSHA, campaign)
		if err != nil {
			if recordErr := recordCampaignMembershipBackfillFailure(ctx, db, manifest, manifestSHA, campaign, err); recordErr != nil {
				return CampaignMembershipBackfillResult{}, fmt.Errorf("campaign %s failed: %v; failure receipt: %w", campaign.CampaignID, err, recordErr)
			}
		}
	}
	result, err := refreshCampaignMembershipBackfillRun(ctx, db, runID)
	if err != nil {
		return CampaignMembershipBackfillResult{}, err
	}
	if result.FailedCampaignCount > 0 {
		return result, fmt.Errorf("campaign membership backfill %s is partial: %d campaign(s) failed", runID, result.FailedCampaignCount)
	}
	return result, nil
}

func canonicalCampaignMembershipBackfillManifest(manifest CampaignMembershipBackfillManifest) ([]byte, int, error) {
	if manifest.ContractVersion != campaignMembershipBackfillContractVersion {
		return nil, 0, fmt.Errorf("contract_version must be %d", campaignMembershipBackfillContractVersion)
	}
	if _, err := uuid.Parse(manifest.RunID); err != nil {
		return nil, 0, errors.New("run_id must be a UUID")
	}
	manifest.TenantID = strings.TrimSpace(manifest.TenantID)
	manifest.Source.Kind = strings.TrimSpace(manifest.Source.Kind)
	manifest.Source.URI = strings.TrimSpace(manifest.Source.URI)
	manifest.Source.SHA256 = strings.ToLower(strings.TrimSpace(manifest.Source.SHA256))
	manifest.Source.SnapshotID = strings.TrimSpace(manifest.Source.SnapshotID)
	manifest.Reason = strings.TrimSpace(manifest.Reason)
	manifest.CreatedBy = strings.TrimSpace(manifest.CreatedBy)
	if manifest.TenantID == "" || manifest.Source.Kind != "clickhouse_export" || manifest.Source.URI == "" ||
		manifest.Source.SnapshotID == "" || manifest.Source.AsOf.IsZero() || !validSHA256Hex(manifest.Source.SHA256) {
		return nil, 0, errors.New("tenant and complete clickhouse_export source metadata are required")
	}
	if reasonLength := len([]rune(manifest.Reason)); reasonLength < 8 || reasonLength > 1000 {
		return nil, 0, errors.New("reason must contain 8 to 1000 characters")
	}
	if manifest.CreatedBy == "" {
		return nil, 0, errors.New("created_by is required")
	}
	if len(manifest.Campaigns) == 0 || len(manifest.Campaigns) > campaignMembershipBackfillMaxCampaigns {
		return nil, 0, fmt.Errorf("campaigns must contain 1 to %d entries", campaignMembershipBackfillMaxCampaigns)
	}
	sort.Slice(manifest.Campaigns, func(i, j int) bool { return manifest.Campaigns[i].CampaignID < manifest.Campaigns[j].CampaignID })
	seenCampaigns := make(map[string]struct{}, len(manifest.Campaigns))
	sourceMemberCount := 0
	for index := range manifest.Campaigns {
		campaign := &manifest.Campaigns[index]
		campaign.CampaignID = strings.TrimSpace(campaign.CampaignID)
		if campaign.CampaignID == "" || campaign.ExpectedRevision < 0 {
			return nil, 0, errors.New("each campaign requires an id and non-negative expected revision")
		}
		if _, duplicate := seenCampaigns[campaign.CampaignID]; duplicate {
			return nil, 0, fmt.Errorf("duplicate campaign_id %s", campaign.CampaignID)
		}
		seenCampaigns[campaign.CampaignID] = struct{}{}
		if len(campaign.AlertIDs) > campaignMembershipBackfillMaxMembers {
			return nil, 0, fmt.Errorf("campaign %s exceeds the %d member budget", campaign.CampaignID, campaignMembershipBackfillMaxMembers)
		}
		for alertIndex := range campaign.AlertIDs {
			campaign.AlertIDs[alertIndex] = strings.TrimSpace(campaign.AlertIDs[alertIndex])
			if campaign.AlertIDs[alertIndex] == "" {
				return nil, 0, fmt.Errorf("campaign %s contains an empty alert id", campaign.CampaignID)
			}
		}
		sort.Strings(campaign.AlertIDs)
		for alertIndex := 1; alertIndex < len(campaign.AlertIDs); alertIndex++ {
			if campaign.AlertIDs[alertIndex] == campaign.AlertIDs[alertIndex-1] {
				return nil, 0, fmt.Errorf("campaign %s contains duplicate alert_id %s", campaign.CampaignID, campaign.AlertIDs[alertIndex])
			}
		}
		sourceMemberCount += len(campaign.AlertIDs)
	}
	encoded, err := json.Marshal(manifest)
	return encoded, sourceMemberCount, err
}

func validSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func campaignMembershipBackfillCampaignCompleted(ctx context.Context, db *sql.DB, runID, campaignID string) (bool, error) {
	var status string
	err := db.QueryRowContext(ctx, `SELECT status FROM campaign_membership_backfill_campaigns
		WHERE run_id=$1::uuid AND campaign_id=$2`, runID, campaignID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return status == "completed", err
}

func applyCampaignMembershipBackfillCampaign(
	ctx context.Context,
	db *sql.DB,
	auditWriter *AlertActionAuditWriter,
	manifest CampaignMembershipBackfillManifest,
	manifestSHA string,
	campaign CampaignMembershipBackfillCampaign,
) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_membership_backfill_campaigns
		(run_id,tenant_id,campaign_id,manifest_sha256,expected_campaign_revision,source_member_count,status,started_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,'running',$7)
		ON CONFLICT (run_id,campaign_id) DO UPDATE SET status='running',error_code='',error_message='',started_at=EXCLUDED.started_at`,
		manifest.RunID, manifest.TenantID, campaign.CampaignID, manifestSHA, campaign.ExpectedRevision,
		len(campaign.AlertIDs), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_workbench_state
		(tenant_id,campaign_id,assignee,status,state_version,member_count,updated_by,updated_at)
		VALUES ($1,$2,'','active',0,0,$3,$4) ON CONFLICT (tenant_id,campaign_id) DO NOTHING`,
		manifest.TenantID, campaign.CampaignID, manifest.CreatedBy, now); err != nil {
		return err
	}
	state, err := lockCampaignAggregateV2State(ctx, tx, manifest.TenantID, campaign.CampaignID)
	if err != nil {
		return err
	}
	if mergedInto, merged, err := loadCampaignMergeTarget(ctx, tx, manifest.TenantID, campaign.CampaignID); err != nil {
		return err
	} else if merged {
		return fmt.Errorf("CAMPAIGN_MERGED: campaign was merged into %s", mergedInto)
	}
	if state.Revision != campaign.ExpectedRevision {
		return fmt.Errorf("CAMPAIGN_REVISION_CONFLICT: expected %d but current revision is %d", campaign.ExpectedRevision, state.Revision)
	}
	relations, err := loadCampaignMembershipBackfillRelations(ctx, tx, manifest.TenantID, campaign.CampaignID, campaign.AlertIDs)
	if err != nil {
		return err
	}
	var actualMemberCountBefore int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM campaign_alert_links
		WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'`, manifest.TenantID, campaign.CampaignID).
		Scan(&actualMemberCountBefore); err != nil {
		return err
	}
	counts := campaignMembershipBackfillCounts{}
	for _, alertID := range campaign.AlertIDs {
		relation, found := relations[alertID]
		switch {
		case !found:
			counts.Inserted++
		case relation.Status == "unlinked":
			counts.SkippedUnlinked++
		case relation.CampaignRevision == 0:
			counts.Bound++
		default:
			counts.Unchanged++
		}
	}
	resultingMemberCount := actualMemberCountBefore + counts.Inserted
	stateChanged := counts.Inserted+counts.Bound > 0 || state.MemberCount != resultingMemberCount
	resultingCampaignRevision := state.Revision
	if stateChanged {
		resultingCampaignRevision++
	}
	for ordinal, alertID := range campaign.AlertIDs {
		relation, found := relations[alertID]
		outcome := "unchanged"
		var eventID interface{}
		if !found {
			outcome = "inserted"
			relation = campaignMembershipBackfillRelation{
				RelationID: deterministicCampaignBackfillUUID("relation", manifest.TenantID, campaign.CampaignID, alertID),
				AlertID:    alertID, Status: "linked", Revision: 1, CampaignRevision: resultingCampaignRevision,
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_links
				(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,
				 idempotency_key,created_by,updated_by,created_at,updated_at)
				VALUES ($1::uuid,$2,$3,$4,'linked',1,$5,$6,$7,$8,$8,$9,$9)`, relation.RelationID,
				manifest.TenantID, campaign.CampaignID, alertID, resultingCampaignRevision, manifest.Reason,
				"backfill:"+manifest.RunID+":"+campaign.CampaignID+":"+alertID, manifest.CreatedBy, now); err != nil {
				return err
			}
			eventID, err = emitCampaignBackfillMembershipEvent(ctx, tx, manifest, campaign.CampaignID,
				relation, resultingCampaignRevision, resultingMemberCount)
			if err != nil {
				return err
			}
		} else if relation.Status == "unlinked" {
			outcome = "skipped_explicit_unlink"
		} else if relation.CampaignRevision == 0 {
			outcome = "bound"
			relation.Revision++
			relation.CampaignRevision = resultingCampaignRevision
			if _, err := tx.ExecContext(ctx, `UPDATE campaign_alert_links
				SET revision=$4,campaign_revision=$5,reason=$6,idempotency_key=$7,updated_by=$8,updated_at=$9
				WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3`, manifest.TenantID, campaign.CampaignID,
				alertID, relation.Revision, resultingCampaignRevision, manifest.Reason,
				"backfill:"+manifest.RunID+":"+campaign.CampaignID+":"+alertID, manifest.CreatedBy, now); err != nil {
				return err
			}
			eventID, err = emitCampaignBackfillMembershipEvent(ctx, tx, manifest, campaign.CampaignID,
				relation, resultingCampaignRevision, resultingMemberCount)
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_membership_backfill_items
			(run_id,tenant_id,campaign_id,alert_id,source_ordinal,outcome,relation_id,relation_revision,
			 campaign_revision,event_id)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::uuid,$8,$9,$10::uuid)`, manifest.RunID,
			manifest.TenantID, campaign.CampaignID, alertID, ordinal, outcome, relation.RelationID,
			relation.Revision, relation.CampaignRevision, eventID); err != nil {
			return err
		}
	}

	var aggregateEventID interface{}
	if stateChanged {
		state.Revision = resultingCampaignRevision
		state.MemberCount = resultingMemberCount
		eventID := deterministicCampaignBackfillUUID("aggregate-event", manifest.RunID, campaign.CampaignID)
		aggregateEventID = eventID
		if _, err := tx.ExecContext(ctx, `UPDATE campaign_workbench_state
			SET state_version=$3,member_count=$4,last_event_id=$5::uuid,updated_by=$6,updated_at=$7
			WHERE tenant_id=$1 AND campaign_id=$2`, manifest.TenantID, campaign.CampaignID,
			resultingCampaignRevision, resultingMemberCount, eventID, manifest.CreatedBy, now); err != nil {
			return err
		}
		extra := map[string]interface{}{
			"run_id": manifest.RunID, "manifest_sha256": manifestSHA,
			"source_snapshot_id": manifest.Source.SnapshotID, "source_member_count": len(campaign.AlertIDs),
			"inserted_count": counts.Inserted, "bound_count": counts.Bound,
			"unchanged_count": counts.Unchanged, "skipped_unlinked_count": counts.SkippedUnlinked,
		}
		if err := emitCampaignMergeAggregateEvent(ctx, tx, manifest.TenantID, campaign.CampaignID, state,
			eventID, "traffic.campaign.v2.MembershipBackfilled", extra, manifest.Reason, manifest.CreatedBy,
			"campaign-membership-backfill:"+manifest.RunID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_membership_backfill_campaigns SET
		starting_campaign_revision=$3,resulting_campaign_revision=$4,resulting_member_count=$5,
		inserted_count=$6,bound_count=$7,unchanged_count=$8,skipped_unlinked_count=$9,
		status='completed',aggregate_event_id=$10::uuid,completed_at=$11,error_code='',error_message=''
		WHERE run_id=$1::uuid AND campaign_id=$2`, manifest.RunID, campaign.CampaignID, campaign.ExpectedRevision,
		resultingCampaignRevision, resultingMemberCount, counts.Inserted, counts.Bound, counts.Unchanged,
		counts.SkippedUnlinked, aggregateEventID, now); err != nil {
		return err
	}
	auditCtx := context.WithValue(ctx, httpx.ContextKeyTenantID, manifest.TenantID)
	auditCtx = context.WithValue(auditCtx, httpx.ContextKeyUserID, manifest.CreatedBy)
	auditCtx = context.WithValue(auditCtx, httpx.ContextKeyTraceID, "campaign-membership-backfill:"+manifest.RunID)
	if err := auditWriter.recordWithExecutor(auditCtx, tx, nil, AlertActionAuditRecord{
		Action: "CAMPAIGN_MEMBERSHIP_BACKFILLED", ObjectType: "campaign", ObjectID: campaign.CampaignID,
		TenantID: manifest.TenantID, UserID: manifest.CreatedBy, Reason: manifest.Reason, Result: "completed",
		Detail: map[string]interface{}{
			"run_id": manifest.RunID, "manifest_sha256": manifestSHA,
			"source_snapshot_id": manifest.Source.SnapshotID, "starting_campaign_revision": campaign.ExpectedRevision,
			"resulting_campaign_revision": resultingCampaignRevision, "resulting_member_count": resultingMemberCount,
			"inserted_count": counts.Inserted, "bound_count": counts.Bound,
			"unchanged_count": counts.Unchanged, "skipped_unlinked_count": counts.SkippedUnlinked,
		},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func loadCampaignMembershipBackfillRelations(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, campaignID string,
	alertIDs []string,
) (map[string]campaignMembershipBackfillRelation, error) {
	if len(alertIDs) == 0 {
		return map[string]campaignMembershipBackfillRelation{}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT relation_id::text,alert_id,status,revision,campaign_revision
		FROM campaign_alert_links WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=ANY($3)
		ORDER BY alert_id FOR UPDATE`, tenantID, campaignID, pq.Array(alertIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	relations := make(map[string]campaignMembershipBackfillRelation, len(alertIDs))
	for rows.Next() {
		var relation campaignMembershipBackfillRelation
		if err := rows.Scan(&relation.RelationID, &relation.AlertID, &relation.Status,
			&relation.Revision, &relation.CampaignRevision); err != nil {
			return nil, err
		}
		relations[relation.AlertID] = relation
	}
	return relations, rows.Err()
}

func emitCampaignBackfillMembershipEvent(
	ctx context.Context,
	tx *sql.Tx,
	manifest CampaignMembershipBackfillManifest,
	campaignID string,
	relation campaignMembershipBackfillRelation,
	campaignRevision int64,
	memberCount int,
) (string, error) {
	eventID := deterministicCampaignBackfillUUID("membership-event", manifest.RunID, campaignID, relation.AlertID)
	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": manifest.TenantID, "schema_version": 2,
		"aggregate_type": "campaign", "aggregate_id": campaignID, "aggregate_version": campaignRevision,
		"partition_key": manifest.TenantID + ":" + campaignID, "event_type": "traffic.campaign.v2.AlertLinked",
		"campaign_id": campaignID, "alert_id": relation.AlertID, "relation_id": relation.RelationID,
		"relation_revision": relation.Revision, "campaign_revision": campaignRevision,
		"member_count": memberCount, "reason": manifest.Reason,
		"trace_id":        "campaign-membership-backfill:" + manifest.RunID,
		"backfill_run_id": manifest.RunID, "source_snapshot_id": manifest.Source.SnapshotID,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_history
		(event_id,relation_id,tenant_id,campaign_id,alert_id,event_type,revision,campaign_revision,payload,created_by)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,'linked',$6,$7,$8::jsonb,$9)`, eventID, relation.RelationID,
		manifest.TenantID, campaignID, relation.AlertID, relation.Revision, campaignRevision,
		string(payloadJSON), manifest.CreatedBy); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_outbox
		(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3::uuid,$4,'traffic.campaign.v2.AlertLinked',$5,$6::jsonb)`, eventID,
		manifest.TenantID, relation.RelationID, relation.Revision, manifest.TenantID+":"+campaignID,
		string(payloadJSON)); err != nil {
		return "", err
	}
	return eventID, nil
}

func deterministicCampaignBackfillUUID(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join(parts, "\x1f"))).String()
}

func recordCampaignMembershipBackfillFailure(
	ctx context.Context,
	db *sql.DB,
	manifest CampaignMembershipBackfillManifest,
	manifestSHA string,
	campaign CampaignMembershipBackfillCampaign,
	cause error,
) error {
	code := "BACKFILL_CAMPAIGN_FAILED"
	message := cause.Error()
	if separator := strings.Index(message, ":"); separator > 0 {
		code = message[:separator]
	}
	_, err := db.ExecContext(ctx, `INSERT INTO campaign_membership_backfill_campaigns
		(run_id,tenant_id,campaign_id,manifest_sha256,expected_campaign_revision,source_member_count,
		 status,error_code,error_message,completed_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,'failed',$7,$8,now())
		ON CONFLICT (run_id,campaign_id) DO UPDATE SET status='failed',error_code=EXCLUDED.error_code,
		 error_message=EXCLUDED.error_message,completed_at=EXCLUDED.completed_at`, manifest.RunID,
		manifest.TenantID, campaign.CampaignID, manifestSHA, campaign.ExpectedRevision,
		len(campaign.AlertIDs), code, message)
	return err
}

func refreshCampaignMembershipBackfillRun(ctx context.Context, db *sql.DB, runID string) (CampaignMembershipBackfillResult, error) {
	if _, err := db.ExecContext(ctx, `WITH totals AS (
		SELECT count(*) FILTER (WHERE status='completed') AS completed_count,
		       count(*) FILTER (WHERE status='failed') AS failed_count,
		       COALESCE(sum(inserted_count) FILTER (WHERE status='completed'),0) AS inserted_count,
		       COALESCE(sum(bound_count) FILTER (WHERE status='completed'),0) AS bound_count,
		       COALESCE(sum(unchanged_count) FILTER (WHERE status='completed'),0) AS unchanged_count,
		       COALESCE(sum(skipped_unlinked_count) FILTER (WHERE status='completed'),0) AS skipped_count
		FROM campaign_membership_backfill_campaigns WHERE run_id=$1::uuid
	)
	UPDATE campaign_membership_backfill_runs r SET
		completed_campaign_count=t.completed_count,failed_campaign_count=t.failed_count,
		inserted_count=t.inserted_count,bound_count=t.bound_count,unchanged_count=t.unchanged_count,
		skipped_unlinked_count=t.skipped_count,
		status=CASE WHEN t.failed_count>0 THEN 'partial'
		            WHEN t.completed_count=r.campaign_count THEN 'completed' ELSE 'running' END,
		updated_at=now(),
		completed_at=CASE WHEN t.completed_count+t.failed_count=r.campaign_count THEN now() ELSE NULL END
	FROM totals t WHERE r.run_id=$1::uuid`, runID); err != nil {
		return CampaignMembershipBackfillResult{}, err
	}
	var result CampaignMembershipBackfillResult
	err := db.QueryRowContext(ctx, `SELECT run_id::text,tenant_id,manifest_sha256,status,campaign_count,
		source_member_count,completed_campaign_count,failed_campaign_count,inserted_count,bound_count,
		unchanged_count,skipped_unlinked_count FROM campaign_membership_backfill_runs WHERE run_id=$1::uuid`, runID).
		Scan(&result.RunID, &result.TenantID, &result.ManifestSHA256, &result.Status, &result.CampaignCount,
			&result.SourceMemberCount, &result.CompletedCampaignCount, &result.FailedCampaignCount,
			&result.InsertedCount, &result.BoundCount, &result.UnchangedCount, &result.SkippedUnlinkedCount)
	return result, err
}

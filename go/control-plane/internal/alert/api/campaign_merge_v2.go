package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const campaignMergeMaxMembers = 1000

type campaignMergeRelation struct {
	RelationID       string
	AlertID          string
	Status           string
	Revision         int64
	CampaignRevision int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type campaignMergeManifestItem struct {
	AlertID                string `json:"alert_id"`
	SourceRelationID       string `json:"source_relation_id"`
	SourceRelationRevision int64  `json:"source_relation_revision"`
	TargetRelationID       string `json:"target_relation_id,omitempty"`
	TargetStatus           string `json:"target_status,omitempty"`
	TargetRelationRevision int64  `json:"target_relation_revision,omitempty"`
}

type campaignMergeManifest struct {
	ContractVersion        int                         `json:"contract_version"`
	TenantID               string                      `json:"tenant_id"`
	SourceCampaignID       string                      `json:"source_campaign_id"`
	TargetCampaignID       string                      `json:"target_campaign_id"`
	SourceExpectedRevision int64                       `json:"source_expected_revision"`
	TargetExpectedRevision int64                       `json:"target_expected_revision"`
	Items                  []campaignMergeManifestItem `json:"items"`
}

func loadCampaignMergeTarget(ctx context.Context, tx *sql.Tx, tenantID, sourceCampaignID string) (string, bool, error) {
	var targetCampaignID string
	err := tx.QueryRowContext(ctx, `SELECT target_campaign_id FROM campaign_merge_receipts
		WHERE tenant_id=$1 AND source_campaign_id=$2`, tenantID, sourceCampaignID).Scan(&targetCampaignID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return targetCampaignID, err == nil, err
}

func (h *SystemHandler) commitCampaignMergeV2Command(
	ctx context.Context,
	httpRequest *http.Request,
	request campaignActionRequest,
	spec campaignActionSpec,
	sourceCampaign campaignDTO,
	targetCampaign campaignDTO,
	idempotencyKey string,
	requestSHA string,
) (campaignActionJob, error) {
	tenantID := httpx.GetTenantID(ctx)
	userID := httpx.GetUserID(ctx)
	sourceCampaignID := sourceCampaign.CampaignID
	targetCampaignID := targetCampaign.CampaignID
	if tenantID == "" || sourceCampaign.TenantID != tenantID || targetCampaign.TenantID != tenantID {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusNotFound, code: "CAMPAIGN_NOT_FOUND",
			message: "source or target campaign was not found in the authenticated tenant",
		}
	}
	if sourceCampaignID == "" || targetCampaignID == "" || sourceCampaignID == targetCampaignID || request.TargetExpectedRevision == nil {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusBadRequest, code: "INVALID_CAMPAIGN_MERGE",
			message: "a distinct source and target campaign with both expected revisions is required",
		}
	}

	tx, err := h.pgDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return campaignActionJob{}, err
	}
	defer tx.Rollback()

	existing, found, err := loadCampaignV2JobByIdempotency(ctx, tx, tenantID, idempotencyKey)
	if err != nil {
		return campaignActionJob{}, err
	}
	if found {
		if existing.CampaignID != sourceCampaignID || existing.ActionID != request.ActionID ||
			existing.ExpectedRevision != *request.ExpectedRevision || existing.RequestSHA256 != requestSHA {
			return campaignActionJob{}, &campaignCommandError{
				status: http.StatusConflict, code: "IDEMPOTENCY_KEY_CONFLICT",
				message: "Idempotency-Key was already used for a different campaign command",
			}
		}
		existing.IdempotentReuse = true
		if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, httpRequest, AlertActionAuditRecord{
			Action: "CAMPAIGN_MERGE_REUSED", ObjectType: "campaign", ObjectID: sourceCampaignID,
			TenantID: tenantID, UserID: userID, Reason: request.Reason, Result: "idempotent_replay",
			Detail: map[string]interface{}{
				"action_id": request.ActionID, "job_id": existing.JobID,
				"target_campaign_id":     targetCampaignID,
				"resource_revision":      existing.ResourceRevision,
				"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
			},
		}); err != nil {
			return campaignActionJob{}, err
		}
		if err := tx.Commit(); err != nil {
			return campaignActionJob{}, err
		}
		return existing, nil
	}

	if mergedInto, merged, err := loadCampaignMergeTarget(ctx, tx, tenantID, sourceCampaignID); err != nil {
		return campaignActionJob{}, err
	} else if merged {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_ALREADY_MERGED",
			message: fmt.Sprintf("source campaign was already merged into %s", mergedInto),
		}
	}
	if mergedInto, merged, err := loadCampaignMergeTarget(ctx, tx, tenantID, targetCampaignID); err != nil {
		return campaignActionJob{}, err
	} else if merged {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "TARGET_CAMPAIGN_MERGED",
			message: fmt.Sprintf("target campaign was already merged into %s", mergedInto),
		}
	}

	initialStates := map[string]string{
		sourceCampaignID: normalizedCampaignState(sourceCampaign.Status),
		targetCampaignID: normalizedCampaignState(targetCampaign.Status),
	}
	orderedCampaignIDs := []string{sourceCampaignID, targetCampaignID}
	sort.Strings(orderedCampaignIDs)
	for _, campaignID := range orderedCampaignIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_workbench_state
			(tenant_id,campaign_id,assignee,status,state_version,member_count,updated_by,updated_at)
			VALUES ($1,$2,'',$3,0,0,$4,now()) ON CONFLICT (tenant_id,campaign_id) DO NOTHING`,
			tenantID, campaignID, initialStates[campaignID], userID); err != nil {
			return campaignActionJob{}, err
		}
	}
	states := make(map[string]campaignAggregateState, 2)
	for _, campaignID := range orderedCampaignIDs {
		state, err := lockCampaignAggregateV2State(ctx, tx, tenantID, campaignID)
		if err != nil {
			return campaignActionJob{}, err
		}
		states[campaignID] = state
	}
	sourceState := states[sourceCampaignID]
	targetState := states[targetCampaignID]
	if sourceState.Revision != *request.ExpectedRevision {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "REVISION_CONFLICT",
			message: fmt.Sprintf("expected source revision %d but current revision is %d", *request.ExpectedRevision, sourceState.Revision),
		}
	}
	if targetState.Revision != *request.TargetExpectedRevision {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "TARGET_REVISION_CONFLICT",
			message: fmt.Sprintf("expected target revision %d but current revision is %d", *request.TargetExpectedRevision, targetState.Revision),
		}
	}
	if targetState.Status == "closed" {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "TARGET_CAMPAIGN_CLOSED",
			message: "closed target campaign cannot receive merged alerts",
		}
	}

	sourceRelations, err := loadCampaignMergeRelations(ctx, tx, tenantID, sourceCampaignID, true, campaignMergeMaxMembers+1)
	if err != nil {
		return campaignActionJob{}, err
	}
	if len(sourceRelations) == 0 {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusConflict, code: "CAMPAIGN_MERGE_EMPTY",
			message: "source campaign has no authoritative PostgreSQL members to merge",
		}
	}
	if len(sourceRelations) > campaignMergeMaxMembers {
		return campaignActionJob{}, &campaignCommandError{
			status: http.StatusRequestEntityTooLarge, code: "CAMPAIGN_MERGE_LIMIT_EXCEEDED",
			message: fmt.Sprintf("source campaign exceeds the bounded merge budget of %d members", campaignMergeMaxMembers),
		}
	}
	alertIDs := make([]string, 0, len(sourceRelations))
	for _, relation := range sourceRelations {
		alertIDs = append(alertIDs, relation.AlertID)
	}
	targetRelations, err := loadCampaignMergeTargetRelations(ctx, tx, tenantID, targetCampaignID, alertIDs)
	if err != nil {
		return campaignActionJob{}, err
	}
	var targetMemberCountBefore int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM campaign_alert_links
		WHERE tenant_id=$1 AND campaign_id=$2 AND status='linked'`, tenantID, targetCampaignID).Scan(&targetMemberCountBefore); err != nil {
		return campaignActionJob{}, err
	}

	manifest := campaignMergeManifest{
		ContractVersion: campaignAggregateContractVersion, TenantID: tenantID,
		SourceCampaignID: sourceCampaignID, TargetCampaignID: targetCampaignID,
		SourceExpectedRevision: *request.ExpectedRevision,
		TargetExpectedRevision: *request.TargetExpectedRevision,
		Items:                  make([]campaignMergeManifestItem, 0, len(sourceRelations)),
	}
	for _, relation := range sourceRelations {
		item := campaignMergeManifestItem{
			AlertID: relation.AlertID, SourceRelationID: relation.RelationID,
			SourceRelationRevision: relation.Revision,
		}
		if target, ok := targetRelations[relation.AlertID]; ok {
			item.TargetRelationID = target.RelationID
			item.TargetStatus = target.Status
			item.TargetRelationRevision = target.Revision
		}
		manifest.Items = append(manifest.Items, item)
	}
	manifestJSON, manifestSHA, err := canonicalCampaignMergeManifest(manifest)
	if err != nil {
		return campaignActionJob{}, err
	}

	mergeID := uuid.NewString()
	jobID := "campaign-" + uuid.NewString()
	now := time.Now().UTC()
	sourceRevision := sourceState.Revision + 1
	targetRevision := targetState.Revision + 1
	movedCount := 0
	deduplicatedCount := 0
	relinkedCount := 0
	for _, relation := range sourceRelations {
		target, targetFound := targetRelations[relation.AlertID]
		switch {
		case targetFound && target.Status == "linked":
			deduplicatedCount++
		case targetFound:
			relinkedCount++
		default:
			movedCount++
		}
	}
	targetMemberCountAfter := targetMemberCountBefore + movedCount + relinkedCount
	for _, relation := range sourceRelations {
		target, targetFound := targetRelations[relation.AlertID]
		outcome := "moved"
		var targetEventID string
		if targetFound && target.Status == "linked" {
			outcome = "deduplicated"
		} else if targetFound {
			outcome = "relinked"
			target.Revision++
			target.CampaignRevision = targetRevision
			target.Status = "linked"
			target.UpdatedAt = now
			if _, err := tx.ExecContext(ctx, `UPDATE campaign_alert_links
				SET status='linked',revision=$4,campaign_revision=$5,reason=$6,idempotency_key=$7,
				    updated_by=$8,updated_at=$9
				WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3`,
				tenantID, targetCampaignID, relation.AlertID, target.Revision, targetRevision,
				request.Reason, campaignMergeRelationKey(mergeID, "target", target.RelationID), userID, now); err != nil {
				return campaignActionJob{}, err
			}
			targetEventID, err = emitCampaignMergeMembershipEvent(ctx, tx, tenantID, targetCampaignID, target,
				targetRevision, targetMemberCountAfter, "traffic.campaign.v2.AlertLinked", request.Reason, userID, httpx.GetTraceID(ctx))
			if err != nil {
				return campaignActionJob{}, err
			}
		} else {
			target = campaignMergeRelation{
				RelationID: uuid.NewString(), AlertID: relation.AlertID, Status: "linked", Revision: 1,
				CampaignRevision: targetRevision, CreatedAt: now, UpdatedAt: now,
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_links
				(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,
				 idempotency_key,created_by,updated_by,created_at,updated_at)
				VALUES ($1::uuid,$2,$3,$4,'linked',1,$5,$6,$7,$8,$8,$9,$9)`,
				target.RelationID, tenantID, targetCampaignID, relation.AlertID, targetRevision,
				request.Reason, campaignMergeRelationKey(mergeID, "target", target.RelationID), userID, now); err != nil {
				return campaignActionJob{}, err
			}
			targetEventID, err = emitCampaignMergeMembershipEvent(ctx, tx, tenantID, targetCampaignID, target,
				targetRevision, targetMemberCountAfter, "traffic.campaign.v2.AlertLinked", request.Reason, userID, httpx.GetTraceID(ctx))
			if err != nil {
				return campaignActionJob{}, err
			}
		}

		relation.Status = "unlinked"
		relation.Revision++
		relation.CampaignRevision = sourceRevision
		relation.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `UPDATE campaign_alert_links
			SET status='unlinked',revision=$4,campaign_revision=$5,reason=$6,idempotency_key=$7,
			    updated_by=$8,updated_at=$9
			WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=$3`,
			tenantID, sourceCampaignID, relation.AlertID, relation.Revision, sourceRevision,
			request.Reason, campaignMergeRelationKey(mergeID, "source", relation.RelationID), userID, now); err != nil {
			return campaignActionJob{}, err
		}
		sourceEventID, err := emitCampaignMergeMembershipEvent(ctx, tx, tenantID, sourceCampaignID, relation,
			sourceRevision, 0, "traffic.campaign.v2.AlertUnlinked", request.Reason, userID, httpx.GetTraceID(ctx))
		if err != nil {
			return campaignActionJob{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_merge_items
			(merge_id,tenant_id,source_relation_id,target_relation_id,alert_id,outcome,
			 source_relation_revision,target_relation_revision,source_event_id,target_event_id)
			VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5,$6,$7,$8,$9::uuid,$10::uuid)`,
			mergeID, tenantID, relation.RelationID, target.RelationID, relation.AlertID, outcome,
			relation.Revision, target.Revision, sourceEventID, nullableUUID(targetEventID)); err != nil {
			return campaignActionJob{}, err
		}
	}

	sourceState.Status = "closed"
	sourceState.Revision = sourceRevision
	sourceState.MemberCount = 0
	targetState.Revision = targetRevision
	targetState.MemberCount = targetMemberCountAfter
	sourceAggregateEventID := uuid.NewString()
	targetAggregateEventID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_workbench_state
		SET status='closed',state_version=$3,member_count=0,last_event_id=$4::uuid,updated_by=$5,updated_at=$6
		WHERE tenant_id=$1 AND campaign_id=$2`, tenantID, sourceCampaignID, sourceRevision,
		sourceAggregateEventID, userID, now); err != nil {
		return campaignActionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_workbench_state
		SET state_version=$3,member_count=$4,last_event_id=$5::uuid,updated_by=$6,updated_at=$7
		WHERE tenant_id=$1 AND campaign_id=$2`, tenantID, targetCampaignID, targetRevision,
		targetMemberCountAfter, targetAggregateEventID, userID, now); err != nil {
		return campaignActionJob{}, err
	}

	mergeSummary := map[string]interface{}{
		"merge_id": mergeID, "source_campaign_id": sourceCampaignID, "target_campaign_id": targetCampaignID,
		"source_campaign_revision": sourceRevision, "target_campaign_revision": targetRevision,
		"source_member_count": len(sourceRelations), "target_member_count_before": targetMemberCountBefore,
		"target_member_count_after": targetMemberCountAfter, "moved_count": movedCount,
		"relinked_count": relinkedCount, "deduplicated_count": deduplicatedCount,
		"manifest_sha256": manifestSHA,
	}
	if err := emitCampaignMergeAggregateEvent(ctx, tx, tenantID, sourceCampaignID, sourceState,
		sourceAggregateEventID, "traffic.campaign.v2.Merged", mergeSummary, request.Reason, userID, httpx.GetTraceID(ctx)); err != nil {
		return campaignActionJob{}, err
	}
	if err := emitCampaignMergeAggregateEvent(ctx, tx, tenantID, targetCampaignID, targetState,
		targetAggregateEventID, "traffic.campaign.v2.MergeReceived", mergeSummary, request.Reason, userID, httpx.GetTraceID(ctx)); err != nil {
		return campaignActionJob{}, err
	}

	result := map[string]interface{}{
		"accepted": true, "final_effect": true, "action_id": request.ActionID,
		"audit_event": spec.AuditEvent, "job_id": jobID, "status": "succeeded",
		"resource_revision": sourceRevision, "target_resource_revision": targetRevision,
		"merge": mergeSummary,
	}
	metadataJSON, err := json.Marshal(request.Metadata)
	if err != nil {
		return campaignActionJob{}, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return campaignActionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_action_jobs
		(job_id,tenant_id,campaign_id,action_id,target,metadata,simulation,dry_run,status,result,
		 created_by,completed_at,idempotency_key,request_sha256,expected_revision,resource_revision,reason)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,false,false,'succeeded',$7::jsonb,$8,$9,$10,$11,$12,$13,$14)`,
		jobID, tenantID, sourceCampaignID, request.ActionID, request.Target, string(metadataJSON),
		string(resultJSON), userID, now, idempotencyKey, requestSHA, *request.ExpectedRevision,
		sourceRevision, request.Reason); err != nil {
		return campaignActionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_merge_receipts
		(merge_id,job_id,tenant_id,source_campaign_id,target_campaign_id,idempotency_key,request_sha256,
		 source_expected_revision,target_expected_revision,source_revision,target_revision,
		 source_member_count,target_member_count_before,target_member_count_after,moved_count,relinked_count,
		 deduplicated_count,manifest,manifest_sha256,reason,trace_id,created_by,created_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb,$19,$20,$21,$22,$23)`,
		mergeID, jobID, tenantID, sourceCampaignID, targetCampaignID, idempotencyKey, requestSHA,
		*request.ExpectedRevision, *request.TargetExpectedRevision, sourceRevision, targetRevision,
		len(sourceRelations), targetMemberCountBefore, targetMemberCountAfter, movedCount, relinkedCount,
		deduplicatedCount, string(manifestJSON), manifestSHA, request.Reason, httpx.GetTraceID(ctx), userID, now); err != nil {
		return campaignActionJob{}, err
	}
	if err := h.campaignAuditWriter.recordWithExecutor(ctx, tx, httpRequest, AlertActionAuditRecord{
		Action: spec.AuditEvent, ObjectType: "campaign", ObjectID: sourceCampaignID,
		TenantID: tenantID, UserID: userID, Reason: request.Reason, Result: "succeeded",
		Detail: map[string]interface{}{
			"action_id": request.ActionID, "job_id": jobID, "merge_id": mergeID,
			"target_campaign_id": targetCampaignID, "source_revision": sourceRevision,
			"target_revision": targetRevision, "moved_count": movedCount,
			"relinked_count": relinkedCount, "deduplicated_count": deduplicatedCount,
			"manifest_sha256": manifestSHA, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		return campaignActionJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return campaignActionJob{}, err
	}
	return campaignActionJob{
		JobID: jobID, TenantID: tenantID, CampaignID: sourceCampaignID, ActionID: request.ActionID,
		Target: request.Target, Metadata: request.Metadata, Status: "succeeded", Result: result,
		IdempotencyKey: idempotencyKey, RequestSHA256: requestSHA,
		ExpectedRevision: *request.ExpectedRevision, ResourceRevision: sourceRevision,
		Reason: request.Reason, CreatedBy: userID, CompletedAt: now,
	}, nil
}

func loadCampaignMergeRelations(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	campaignID string,
	linkedOnly bool,
	limit int,
) ([]campaignMergeRelation, error) {
	statusPredicate := ""
	if linkedOnly {
		statusPredicate = " AND status='linked'"
	}
	rows, err := tx.QueryContext(ctx, `SELECT relation_id::text,alert_id,status,revision,campaign_revision,created_at,updated_at
		FROM campaign_alert_links WHERE tenant_id=$1 AND campaign_id=$2`+statusPredicate+`
		ORDER BY alert_id LIMIT $3 FOR UPDATE`, tenantID, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]campaignMergeRelation, 0)
	for rows.Next() {
		var item campaignMergeRelation
		if err := rows.Scan(&item.RelationID, &item.AlertID, &item.Status, &item.Revision,
			&item.CampaignRevision, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCampaignMergeTargetRelations(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	targetCampaignID string,
	alertIDs []string,
) (map[string]campaignMergeRelation, error) {
	rows, err := tx.QueryContext(ctx, `SELECT relation_id::text,alert_id,status,revision,campaign_revision,created_at,updated_at
		FROM campaign_alert_links WHERE tenant_id=$1 AND campaign_id=$2 AND alert_id=ANY($3)
		ORDER BY alert_id FOR UPDATE`, tenantID, targetCampaignID, pq.Array(alertIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[string]campaignMergeRelation, len(alertIDs))
	for rows.Next() {
		var item campaignMergeRelation
		if err := rows.Scan(&item.RelationID, &item.AlertID, &item.Status, &item.Revision,
			&item.CampaignRevision, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items[item.AlertID] = item
	}
	return items, rows.Err()
}

func canonicalCampaignMergeManifest(manifest campaignMergeManifest) ([]byte, string, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(digest[:]), nil
}

func campaignMergeRelationKey(mergeID, side, relationID string) string {
	return "merge:" + mergeID + ":" + side + ":" + relationID
}

func nullableUUID(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func emitCampaignMergeMembershipEvent(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	campaignID string,
	relation campaignMergeRelation,
	campaignRevision int64,
	memberCount int,
	eventType string,
	reason string,
	userID string,
	traceID string,
) (string, error) {
	eventID := uuid.NewString()
	historyType := "linked"
	if eventType == "traffic.campaign.v2.AlertUnlinked" {
		historyType = "unlinked"
	}
	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "campaign", "aggregate_id": campaignID,
		"aggregate_version": campaignRevision, "partition_key": tenantID + ":" + campaignID,
		"event_type": eventType, "campaign_id": campaignID, "alert_id": relation.AlertID,
		"relation_id": relation.RelationID, "relation_revision": relation.Revision,
		"campaign_revision": campaignRevision, "member_count": memberCount,
		"reason": reason, "trace_id": traceID,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_history
		(event_id,relation_id,tenant_id,campaign_id,alert_id,event_type,revision,campaign_revision,payload,created_by)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`,
		eventID, relation.RelationID, tenantID, campaignID, relation.AlertID, historyType,
		relation.Revision, campaignRevision, string(payloadJSON), userID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_alert_link_outbox
		(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6,$7::jsonb)`, eventID, tenantID,
		relation.RelationID, relation.Revision, eventType, tenantID+":"+campaignID, string(payloadJSON)); err != nil {
		return "", err
	}
	return eventID, nil
}

func emitCampaignMergeAggregateEvent(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	campaignID string,
	state campaignAggregateState,
	eventID string,
	eventType string,
	mergeSummary map[string]interface{},
	reason string,
	userID string,
	traceID string,
) error {
	payload := map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "campaign", "aggregate_id": campaignID,
		"aggregate_version": state.Revision, "partition_key": tenantID + ":" + campaignID,
		"event_type": eventType, "campaign_id": campaignID, "status": state.Status,
		"assignee": state.Assignee, "member_count": state.MemberCount,
		"reason": reason, "trace_id": traceID,
	}
	for key, value := range mergeSummary {
		payload[key] = value
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_history
		(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,assignee,member_count,payload,reason,created_by)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)`,
		eventID, tenantID, campaignID, state.Revision, eventType, state.Status, state.Assignee,
		state.MemberCount, string(payloadJSON), reason, userID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_aggregate_outbox
		(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, tenantID, campaignID,
		state.Revision, eventType, tenantID+":"+campaignID, string(payloadJSON))
	return err
}

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
)

type feedbackCommitResult struct {
	IdempotentReplay bool
	CreatedAt        time.Time
}

func feedbackIdentity(tenantID, alertID, idempotencyKey string) string {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return uuid.NewString()
	}
	material := strings.Join([]string{
		"alert.feedback.v1", tenantID, alertID, idempotencyKey,
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(material)).String()
}

func normalizedIdempotencyKey(request *http.Request) (string, error) {
	if request == nil {
		return "", nil
	}
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(key) > 256 {
		return "", fmt.Errorf("Idempotency-Key exceeds 256 characters")
	}
	return key, nil
}

func (h *FeedbackHandler) commitFeedbackTransaction(
	ctx context.Context,
	request *http.Request,
	record *FeedbackRecord,
	event *AlertFeedbackExtended,
	idempotencyKey string,
	whitelistEntry *whitelist.Entry,
) (feedbackCommitResult, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return feedbackCommitResult{}, fmt.Errorf("feedback PostgreSQL transaction database is unavailable")
	}
	if record == nil || event == nil {
		return feedbackCommitResult{}, fmt.Errorf("feedback record and event are required")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return feedbackCommitResult{}, fmt.Errorf("marshal feedback event: %w", err)
	}
	userID := record.UserID
	if userID != "" {
		if _, parseErr := uuid.Parse(userID); parseErr != nil {
			userID = ""
		}
	}
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return feedbackCommitResult{}, fmt.Errorf("begin feedback transaction: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO alert_feedback
			(feedback_id,event_id,tenant_id,alert_id,user_id,label,reason_code,comment,
			 add_to_whitelist,alert_type,severity,model_version,rule_version,
			 idempotency_key,trace_id,payload,status,created_at,updated_at)
		VALUES
			($1::uuid,$2::uuid,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,
			 $9,$10,$11,$12,$13,$14,$15,$16::jsonb,'accepted',$17,$17)
		ON CONFLICT DO NOTHING`,
		record.FeedbackID, event.EventID, record.TenantID, record.AlertID, userID,
		record.Label, record.ReasonCode, record.Comment, record.AddToWhitelist,
		record.AlertType, record.Severity, record.ModelVersion, record.RuleVersion,
		idempotencyKey, httpx.GetTraceID(ctx), string(payload), record.CreatedAt,
	)
	if err != nil {
		return feedbackCommitResult{}, fmt.Errorf("insert authoritative feedback: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return feedbackCommitResult{}, fmt.Errorf("inspect authoritative feedback insert: %w", err)
	}
	if inserted == 0 {
		var tenantID, alertID, label, reasonCode, comment string
		var addToWhitelist bool
		var createdAt time.Time
		err = tx.QueryRowContext(ctx, `
			SELECT tenant_id,alert_id,label,COALESCE(reason_code,''),COALESCE(comment,''),
			       add_to_whitelist,created_at
			FROM alert_feedback WHERE feedback_id=$1::uuid`,
			record.FeedbackID,
		).Scan(
			&tenantID, &alertID, &label, &reasonCode, &comment,
			&addToWhitelist, &createdAt,
		)
		if err != nil {
			return feedbackCommitResult{}, fmt.Errorf("resolve feedback idempotency conflict: %w", err)
		}
		if tenantID != record.TenantID || alertID != record.AlertID ||
			label != record.Label || reasonCode != record.ReasonCode ||
			comment != record.Comment || addToWhitelist != record.AddToWhitelist {
			return feedbackCommitResult{}, fmt.Errorf("Idempotency-Key was already used for a different feedback command")
		}
		if err := tx.Commit(); err != nil {
			return feedbackCommitResult{}, fmt.Errorf("commit feedback idempotent replay: %w", err)
		}
		return feedbackCommitResult{IdempotentReplay: true, CreatedAt: createdAt}, nil
	}

	if whitelistEntry != nil {
		if h.whitelistRepo == nil {
			return feedbackCommitResult{}, fmt.Errorf("whitelist repository is unavailable")
		}
		commandKey := "feedback-whitelist-" + record.FeedbackID
		if _, err := h.whitelistRepo.CreateGovernedTx(ctx, tx, whitelistEntry, whitelist.CommandMeta{
			TenantID: record.TenantID, ActorID: record.UserID, ActionID: whitelist.ActionCreate,
			IdempotencyKey: commandKey, ExpectedVersion: 0,
			Reason:  "false-positive feedback requested a governed whitelist draft",
			TraceID: httpx.GetTraceID(ctx), SourceIP: clientIP(request), UserAgent: request.UserAgent(),
		}, whitelist.AuditRecord{
			UserID: record.UserID, Action: "WHITELIST_DRAFT_CREATED", ObjectID: whitelistEntry.ID,
			IPAddress: clientIP(request), UserAgent: request.UserAgent(),
			RequestID: httpx.GetRequestID(ctx), TraceID: httpx.GetTraceID(ctx),
			Detail: map[string]interface{}{
				"feedback_id": record.FeedbackID, "alert_id": record.AlertID,
				"type": whitelistEntry.Type, "value": whitelistEntry.Value,
				"creation_source": "alert_fp_feedback",
			},
		}); err != nil {
			return feedbackCommitResult{}, fmt.Errorf("insert governed feedback whitelist draft: %w", err)
		}
	}
	if err := h.actionAudit.recordWithExecutor(ctx, tx, request, AlertActionAuditRecord{
		Action: "ALERT_FEEDBACK_SUBMITTED", ObjectType: "alert_feedback",
		ObjectID: record.FeedbackID, TenantID: record.TenantID, UserID: record.UserID,
		AlertID: record.AlertID, Result: "success",
		Detail: map[string]interface{}{
			"label": record.Label, "reason_code": record.ReasonCode,
			"add_to_whitelist":        record.AddToWhitelist,
			"idempotency_key_present": idempotencyKey != "",
		},
	}); err != nil {
		return feedbackCommitResult{}, fmt.Errorf("insert feedback audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO alert_feedback_outbox
			(event_id,feedback_id,tenant_id,alert_id,partition_key,
			 schema_version,aggregate_version,payload)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,1,1,$6::jsonb)`,
		event.EventID, record.FeedbackID, record.TenantID, record.AlertID,
		record.TenantID+":"+record.AlertID, string(payload),
	); err != nil {
		return feedbackCommitResult{}, fmt.Errorf("insert feedback outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return feedbackCommitResult{}, fmt.Errorf("commit feedback transaction: %w", err)
	}
	return feedbackCommitResult{CreatedAt: record.CreatedAt}, nil
}

func (h *FeedbackHandler) getAuthoritativeFeedback(
	ctx context.Context,
	tenantID, alertID string,
) ([]*FeedbackRecord, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return nil, sql.ErrConnDone
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(payload->>'feedback_id',''),feedback_id::text),
		       event_id::text,COALESCE(payload->>'prediction_id',''),
		       COALESCE((payload->>'label_revision')::bigint,0),
		       COALESCE(payload->>'adjudication_state',''),
		       COALESCE(payload->>'previous_event_id',''),
		       alert_id,tenant_id,COALESCE(user_id::text,''),
		       label,COALESCE(reason_code,''),COALESCE(comment,''),add_to_whitelist,
		       alert_type,severity,model_version,rule_version,created_at
		FROM alert_feedback
		WHERE tenant_id=$1 AND alert_id=$2
		ORDER BY created_at DESC,feedback_id DESC`,
		tenantID, alertID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]*FeedbackRecord, 0)
	for rows.Next() {
		record := &FeedbackRecord{}
		if err := rows.Scan(
			&record.FeedbackID, &record.EventID, &record.PredictionID,
			&record.LabelRevision, &record.AdjudicationState, &record.PreviousEventID,
			&record.AlertID, &record.TenantID, &record.UserID,
			&record.Label, &record.ReasonCode, &record.Comment, &record.AddToWhitelist,
			&record.AlertType, &record.Severity, &record.ModelVersion,
			&record.RuleVersion, &record.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (h *FeedbackHandler) getAuthoritativeFeedbackStats(
	ctx context.Context,
	tenantID string,
) (map[string]interface{}, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return nil, sql.ErrConnDone
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `
		WITH effective AS (
			SELECT label FROM alert_feedback
			WHERE tenant_id=$1 AND created_at >= now()-interval '30 days'
			  AND payload->>'event_type' IS DISTINCT FROM 'model.feedback.v1'
			UNION ALL
			SELECT label FROM (
				SELECT DISTINCT ON (payload->>'prediction_id') label,
				       payload->>'adjudication_state' AS adjudication_state
				FROM alert_feedback
				WHERE tenant_id=$1 AND created_at >= now()-interval '30 days'
				  AND payload->>'event_type'='model.feedback.v1'
				ORDER BY payload->>'prediction_id',(payload->>'label_revision')::bigint DESC
			) heads WHERE adjudication_state <> 'RETRACTED'
		)
		SELECT label,count(*) FROM effective GROUP BY label`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tp, fp int64
	for rows.Next() {
		var label string
		var count int64
		if err := rows.Scan(&label, &count); err != nil {
			return nil, err
		}
		switch label {
		case "TP":
			tp = count
		case "FP":
			fp = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	total := tp + fp
	rate := float64(0)
	if total > 0 {
		rate = float64(fp) / float64(total)
	}
	return map[string]interface{}{
		"tp_count": tp, "fp_count": fp, "total": total, "fp_rate": rate,
	}, nil
}

func (h *FeedbackHandler) getAuthoritativeFPRanking(
	ctx context.Context,
	tenantID string,
	limit int,
) ([]map[string]interface{}, error) {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return nil, sql.ErrConnDone
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `
		WITH effective AS (
			SELECT label,reason_code FROM alert_feedback
			WHERE tenant_id=$1 AND created_at >= now()-interval '30 days'
			  AND payload->>'event_type' IS DISTINCT FROM 'model.feedback.v1'
			UNION ALL
			SELECT label,reason_code FROM (
				SELECT DISTINCT ON (payload->>'prediction_id') label,reason_code,
				       payload->>'adjudication_state' AS adjudication_state
				FROM alert_feedback
				WHERE tenant_id=$1 AND created_at >= now()-interval '30 days'
				  AND payload->>'event_type'='model.feedback.v1'
				ORDER BY payload->>'prediction_id',(payload->>'label_revision')::bigint DESC
			) heads WHERE adjudication_state <> 'RETRACTED'
		)
		SELECT COALESCE(reason_code,''),count(*) FROM effective WHERE label='FP'
		GROUP BY reason_code ORDER BY count(*) DESC,reason_code LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ranking := make([]map[string]interface{}, 0)
	for rows.Next() {
		var reasonCode string
		var count int64
		if err := rows.Scan(&reasonCode, &count); err != nil {
			return nil, err
		}
		ranking = append(ranking, map[string]interface{}{
			"reason_code": reasonCode, "count": count,
			"description": FPReasonCodes[reasonCode],
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ranking, nil
}

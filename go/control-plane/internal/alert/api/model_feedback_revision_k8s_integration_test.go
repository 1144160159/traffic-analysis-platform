package api

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func modelFeedbackK8sDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	if os.Getenv("MODEL_FEEDBACK_K8S_INTEGRATION") != "run-scoped-only" {
		t.Skip("MODEL_FEEDBACK_K8S_INTEGRATION=run-scoped-only is required")
	}
	host := strings.TrimSpace(os.Getenv("MODEL_FEEDBACK_K8S_PG_HOST"))
	suffix := strings.ToLower(strings.TrimSpace(os.Getenv("MODEL_FEEDBACK_K8S_SUFFIX")))
	if host != "postgres-primary.databases.svc" || len(suffix) < 8 || len(suffix) > 16 {
		t.Fatalf("refusing non-K8s PostgreSQL host=%q suffix=%q", host, suffix)
	}
	dsn := (&url.URL{
		Scheme: "postgres", User: url.UserPassword("postgres", os.Getenv("MODEL_FEEDBACK_K8S_PG_PASSWORD")),
		Host: host + ":5432", Path: "/traffic_platform", RawQuery: "sslmode=disable",
	}).String()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db, suffix
}

func cleanupModelFeedbackK8s(t *testing.T, db *sql.DB, suffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantA, tenantB := "m09-n017-"+suffix, "m09-n017-other-"+suffix
	group := "m09-n017-consumer-" + suffix
	optionalStatements := []struct {
		table string
		query string
		args  []interface{}
	}{
		{"model_feedback_consumer_readiness_receipt", `DELETE FROM model_feedback_consumer_readiness_receipt WHERE consumer_group=$1`, []interface{}{group}},
		{"model_feedback_revision_receipt", `DELETE FROM model_feedback_revision_receipt WHERE tenant_id IN ($1,$2)`, []interface{}{tenantA, tenantB}},
		{"model_feedback_revision_inbox", `DELETE FROM model_feedback_revision_inbox WHERE tenant_id IN ($1,$2)`, []interface{}{tenantA, tenantB}},
		{"model_feedback_revision_head", `DELETE FROM model_feedback_revision_head WHERE tenant_id IN ($1,$2)`, []interface{}{tenantA, tenantB}},
	}
	for _, statement := range optionalStatements {
		if !modelFeedbackK8sTableExists(ctx, db, statement.table) {
			continue
		}
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	for _, query := range []string{
		`DELETE FROM alert_feedback_outbox WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM alert_feedback WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM audit_logs WHERE tenant_id IN ($1,$2)`,
		`DELETE FROM tenants WHERE tenant_id IN ($1,$2)`,
	} {
		if _, err := db.ExecContext(ctx, query, tenantA, tenantB); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
}

func modelFeedbackK8sTableExists(ctx context.Context, db *sql.DB, table string) bool {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
		return false
	}
	return exists
}

func TestModelFeedbackRevisionK8sPostgresIntegration(t *testing.T) {
	db, suffix := modelFeedbackK8sDB(t)
	defer db.Close()
	cleanupModelFeedbackK8s(t, db, suffix)
	defer cleanupModelFeedbackK8s(t, db, suffix)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tenantA, tenantB := "m09-n017-"+suffix, "m09-n017-other-"+suffix
	for _, tenantID := range []string{tenantA, tenantB} {
		if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,tenant_name,name) VALUES($1,$2,$2)`, tenantID, "M09 N017 run-scoped tenant"); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewFeedbackHandler(nil, nil, nil, nil, nil, zap.NewNop())
	handler.actionAudit = NewAlertActionAuditWriter(db, zap.NewNop())
	request := httptest.NewRequest("POST", "/api/v1/alerts/alert-a/feedback", nil)

	first, firstCommand := modelFeedbackRevisionFixture()
	first.EventID = uuid.NewString()
	first.TenantID, first.PredictionID, first.AlertID = tenantA, "prediction-"+suffix, "alert-"+suffix
	first.FeedbackID = modelFeedbackAggregateIdentity(first.TenantID, first.PredictionID)
	first.AdjudicatedBy = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	first.OccurredAtMS = time.Now().UnixMilli()
	firstResult, err := handler.commitModelFeedbackRevision(ctx, request, &first, firstCommand, "first-"+suffix, nil)
	if err != nil || firstResult.Event.LabelRevision != 1 {
		t.Fatalf("first revision=%+v err=%v", firstResult.Event, err)
	}
	replay, err := handler.commitModelFeedbackRevision(ctx, request, &first, firstCommand, "first-"+suffix, nil)
	if err != nil || !replay.IdempotentReplay {
		t.Fatalf("idempotent replay=%+v err=%v", replay, err)
	}

	second := first
	second.EventID, second.Label, second.ReasonCode = uuid.NewString(), "TP", ""
	second.AdjudicationState, second.OccurredAtMS = "ADJUDICATED", time.Now().Add(time.Millisecond).UnixMilli()
	secondCommand := FeedbackRequest{Label: "TP", AdjudicationState: "ADJUDICATED", ExpectedLabelRevision: 1}
	secondResult, err := handler.commitModelFeedbackRevision(ctx, request, &second, secondCommand, "second-"+suffix, nil)
	if err != nil || secondResult.Event.LabelRevision != 2 || secondResult.Event.PreviousEventID != first.EventID {
		t.Fatalf("second revision=%+v err=%v", secondResult.Event, err)
	}

	stale := second
	stale.EventID = uuid.NewString()
	if _, err := handler.commitModelFeedbackRevision(ctx, request, &stale,
		FeedbackRequest{Label: "TP", AdjudicationState: "ADJUDICATED", ExpectedLabelRevision: 1}, "stale-"+suffix, nil); !stderrors.Is(err, errModelFeedbackRevisionConflict) {
		t.Fatalf("stale revision err=%v", err)
	}

	retraction := second
	retraction.EventID, retraction.AdjudicationState = uuid.NewString(), "RETRACTED"
	retraction.ReasonCode, retraction.OccurredAtMS = "OTHER", time.Now().Add(2*time.Millisecond).UnixMilli()
	retractionCommand := FeedbackRequest{Label: "TP", ReasonCode: "OTHER", AdjudicationState: "RETRACTED", ExpectedLabelRevision: 2}
	retracted, err := handler.commitModelFeedbackRevision(ctx, request, &retraction, retractionCommand, "retract-"+suffix, nil)
	if err != nil || retracted.Event.LabelRevision != 3 || retracted.Event.PreviousEventID != second.EventID {
		t.Fatalf("retraction=%+v err=%v", retracted.Event, err)
	}
	postTerminal := second
	postTerminal.EventID = uuid.NewString()
	if _, err := handler.commitModelFeedbackRevision(ctx, request, &postTerminal,
		FeedbackRequest{Label: "TP", AdjudicationState: "ADJUDICATED", ExpectedLabelRevision: 3}, "terminal-"+suffix, nil); !stderrors.Is(err, errModelFeedbackAlreadyRetracted) {
		t.Fatalf("post-terminal err=%v", err)
	}

	other := first
	other.EventID, other.TenantID = uuid.NewString(), tenantB
	other.FeedbackID = modelFeedbackAggregateIdentity(other.TenantID, other.PredictionID)
	other.OccurredAtMS = time.Now().Add(3 * time.Millisecond).UnixMilli()
	otherResult, err := handler.commitModelFeedbackRevision(ctx, request, &other, firstCommand, "other-"+suffix, nil)
	if err != nil || otherResult.Event.LabelRevision != 1 || other.FeedbackID == first.FeedbackID {
		t.Fatalf("cross-tenant revision=%+v err=%v", otherResult.Event, err)
	}

	var feedbackRows, outboxRows, audits, publishedRows int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM alert_feedback WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_feedback_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='model_feedback'),
		(SELECT count(*) FROM alert_feedback_outbox WHERE tenant_id=$1 AND published=true)`, tenantA).
		Scan(&feedbackRows, &outboxRows, &audits, &publishedRows); err != nil {
		t.Fatal(err)
	}
	if feedbackRows != 3 || outboxRows != 3 || audits != 3 || publishedRows != 0 {
		t.Fatalf("feedback=%d outbox=%d audits=%d published=%d", feedbackRows, outboxRows, audits, publishedRows)
	}
	rows, err := db.QueryContext(ctx, `SELECT payload::text FROM alert_feedback_outbox WHERE tenant_id=$1 ORDER BY outbox_id`, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var revisions []ModelFeedbackAdjudicatedV1
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var event ModelFeedbackAdjudicatedV1
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatal(err)
		}
		revisions = append(revisions, event)
	}
	if len(revisions) != 3 || revisions[0].LabelRevision != 1 || revisions[1].PreviousEventID != revisions[0].EventID ||
		revisions[2].AdjudicationState != "RETRACTED" || revisions[2].PreviousEventID != revisions[1].EventID {
		t.Fatalf("untraceable revision chain: %+v", revisions)
	}

	group := "m09-n017-consumer-" + suffix
	candidate, contract := strings.Repeat("a", 64), strings.Repeat("b", 64)
	consumerSchemaPresent := modelFeedbackK8sTableExists(ctx, db, "model_feedback_revision_inbox") &&
		modelFeedbackK8sTableExists(ctx, db, "model_feedback_revision_receipt") &&
		modelFeedbackK8sTableExists(ctx, db, "model_feedback_consumer_readiness_receipt")
	if consumerSchemaPresent {
		receiptEvent := uuid.NewString()
		offset := time.Now().UnixNano() & 0x3fffffffffffffff
		if _, err := db.ExecContext(ctx, `INSERT INTO model_feedback_revision_inbox
		(event_id,feedback_id,tenant_id,prediction_id,alert_id,label,label_revision,adjudication_state,
		 reason_code,model_version,rule_version,adjudicated_by,occurred_at_ms,trace_id,payload,payload_sha256,
		 kafka_topic,kafka_partition,kafka_offset,status)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,'TP',1,'ADJUDICATED','',$6,$7,$8,$9,$10,'{}'::jsonb,$11,'model.feedback.v1',0,$12,'pending')`,
			receiptEvent, uuid.NewString(), tenantA, "readiness-prediction-"+suffix, "readiness-alert-"+suffix,
			"model-v1", "rule-v1", first.AdjudicatedBy, time.Now().UnixMilli(), first.TraceID, strings.Repeat("c", 64), offset); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO model_feedback_revision_receipt
		(event_id,feedback_id,tenant_id,label_revision,outcome,payload_sha256,kafka_topic,kafka_partition,kafka_offset)
		SELECT event_id,feedback_id,tenant_id,label_revision,'ACCEPTED',payload_sha256,kafka_topic,kafka_partition,kafka_offset
		FROM model_feedback_revision_inbox WHERE event_id=$1::uuid`, receiptEvent); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO model_feedback_consumer_readiness_receipt
		(consumer_group,candidate_sha256,contract_sha256,kafka_topic,state,event_id,kafka_partition,kafka_offset,observed_at)
		VALUES($1,$2,$3,'model.feedback.v1','READY',$4::uuid,0,$5,now())`, group, candidate, contract, receiptEvent, offset); err != nil {
			t.Fatal(err)
		}
		if err := VerifyModelFeedbackProducerReadiness(ctx, db, ModelFeedbackProducerReadiness{
			Topic: "model.feedback.v1", ConsumerGroup: group, CandidateSHA256: candidate, ContractSHA256: contract,
		}); err != nil {
			t.Fatal(err)
		}
	} else if err := VerifyModelFeedbackProducerReadiness(ctx, db, ModelFeedbackProducerReadiness{
		Topic: "model.feedback.v1", ConsumerGroup: group, CandidateSHA256: candidate, ContractSHA256: contract,
	}); err == nil {
		t.Fatal("missing M08 consumer schema authorized producer")
	}
	if err := VerifyModelFeedbackProducerReadiness(ctx, db, ModelFeedbackProducerReadiness{
		Topic: "model.feedback.v1", ConsumerGroup: group, CandidateSHA256: strings.Repeat("0", 64), ContractSHA256: contract,
	}); err == nil {
		t.Fatal("zero candidate authorized producer")
	}
	t.Logf("model_feedback_k8s=pass tenant=%s revisions=%d producer_published=%d consumer_schema_present=%t", tenantA, feedbackRows, publishedRows, consumerSchemaPresent)
}

func TestModelFeedbackRevisionK8sCleanupOracle(t *testing.T) {
	db, suffix := modelFeedbackK8sDB(t)
	defer db.Close()
	cleanupModelFeedbackK8s(t, db, suffix)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tenantA, tenantB := "m09-n017-"+suffix, "m09-n017-other-"+suffix
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM tenants WHERE tenant_id IN ($1,$2))+
		(SELECT count(*) FROM alert_feedback WHERE tenant_id IN ($1,$2))+
		(SELECT count(*) FROM alert_feedback_outbox WHERE tenant_id IN ($1,$2))+
		(SELECT count(*) FROM audit_logs WHERE tenant_id IN ($1,$2))`, tenantA, tenantB).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"model_feedback_revision_inbox", "model_feedback_revision_receipt"} {
		if modelFeedbackK8sTableExists(ctx, db, table) {
			var count int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE tenant_id IN ($1,$2)`, tenantA, tenantB).Scan(&count); err != nil {
				t.Fatal(err)
			}
			rows += count
		}
	}
	if rows != 0 {
		t.Fatalf("run-scoped cleanup incomplete: rows=%d", rows)
	}
	t.Log("model_feedback_k8s_cleanup=pass")
}

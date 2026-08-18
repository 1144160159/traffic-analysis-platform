package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestAlertEvidenceLinkPipelineEphemeralKubernetes(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_EPHEMERAL_PG_DSN"))
	broker := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_EPHEMERAL_KAFKA_BROKER"))
	runID := strings.TrimSpace(os.Getenv("ALERT_EVIDENCE_LINK_CANARY_RUN_ID"))
	if dsn == "" || broker == "" || runID == "" {
		t.Skip("run-scoped Kubernetes PostgreSQL, Kafka and canary identity are required")
	}
	if _, err := uuid.Parse(runID); err != nil {
		t.Fatalf("invalid run id: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_alert_evidence_link_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	tenantID := "n012-" + strings.ReplaceAll(runID, "-", "")[:16]
	alertID := "alert-" + runID
	evidenceID := "evidence-" + runID
	objectSHA := strings.Repeat("a", 64)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'M09 N012 canary')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alert_evidence_manifests
		(tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,object_bucket,object_key,
		 object_version,object_sha256,size_bytes,content_type,state,revision,source_watermarks)
		VALUES ($1,$2,$3,$4,'pcap','minio','evidence',$5,'version-1',$6,42,
		 'application/vnd.tcpdump.pcap','available',1,'{"source":"n012-canary"}'::jsonb)`,
		tenantID, alertID, evidenceID, "manifest-"+runID,
		"tenants/"+tenantID+"/evidence/capture.pcap", objectSHA); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := alertEvidenceLinkRequest{
		ExpectedRevision: evidenceInt64Pointer(0), ExpectedManifestRevision: evidenceInt64Pointer(1),
		SourceStore: "minio", ObjectBucket: "evidence",
		ObjectKey: "tenants/" + tenantID + "/evidence/capture.pcap", ObjectVersion: "version-1",
		ObjectSHA256: objectSHA, Reason: "M09 N012 canary link",
	}
	httpRequest := httptest.NewRequest("PUT", "/api/v1/alerts/"+alertID+"/evidence-links/"+evidenceID, nil)
	ctx := context.WithValue(httpRequest.Context(), httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "operator-n012")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-"+runID)
	ctx = context.WithValue(ctx, httpx.ContextKeyRequestID, "request-"+runID)
	httpRequest = httpRequest.WithContext(ctx)

	commit := func(operation alertEvidenceLinkOperation, key string, candidate alertEvidenceLinkRequest) (AlertEvidenceLink, error) {
		digest, digestErr := alertEvidenceLinkRequestSHA(operation, tenantID, alertID, evidenceID, candidate)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		return handler.commitAlertEvidenceLink(ctx, httpRequest, operation, tenantID, alertID, evidenceID, key, digest, candidate)
	}
	firstKey := "n012-link-command-" + runID
	linked, err := commit(alertEvidenceLink, firstKey, request)
	if err != nil || !linked.Changed || linked.Revision != 1 || linked.Status != "linked" {
		t.Fatalf("initial link=%#v err=%v", linked, err)
	}
	replay, err := commit(alertEvidenceLink, firstKey, request)
	if err != nil || !replay.IdempotentReuse || replay.RelationID != linked.RelationID || replay.Revision != 1 {
		t.Fatalf("exact replay=%#v err=%v", replay, err)
	}
	request.ExpectedRevision = evidenceInt64Pointer(1)
	unchanged, err := commit(alertEvidenceLink, "n012-link-no-change-"+runID, request)
	if err != nil || unchanged.Changed || unchanged.Revision != 1 || unchanged.OutboxStatus != "unchanged" {
		t.Fatalf("same object no-op=%#v err=%v", unchanged, err)
	}
	changedDigest := request
	changedDigest.ObjectSHA256 = strings.Repeat("b", 64)
	if _, err := commit(alertEvidenceLink, "n012-link-digest-conflict-"+runID, changedDigest); !isEvidenceCommandCode(err, "OBJECT_IDENTITY_CONFLICT") {
		t.Fatalf("different digest must conflict, got %v", err)
	}
	stale := request
	stale.ExpectedRevision = evidenceInt64Pointer(0)
	if _, err := commit(alertEvidenceLink, "n012-link-stale-"+runID, stale); !isEvidenceCommandCode(err, "REVISION_CONFLICT") {
		t.Fatalf("stale relation revision must conflict, got %v", err)
	}
	unlinkRequest := alertEvidenceLinkRequest{ExpectedRevision: evidenceInt64Pointer(1), Reason: "M09 N012 canary unlink"}
	unlinked, err := commit(alertEvidenceUnlink, "n012-unlink-command-"+runID, unlinkRequest)
	if err != nil || unlinked.Revision != 2 || unlinked.Status != "unlinked" || !unlinked.Changed {
		t.Fatalf("unlink=%#v err=%v", unlinked, err)
	}
	if _, err := commit(alertEvidenceUnlink, "n012-unlink-stale-"+runID, unlinkRequest); !isEvidenceCommandCode(err, "REVISION_CONFLICT") {
		t.Fatalf("stale unlink must conflict, got %v", err)
	}
	request.ExpectedRevision = evidenceInt64Pointer(2)
	relinked, err := commit(alertEvidenceLink, "n012-relink-command-"+runID, request)
	if err != nil || relinked.Revision != 3 || relinked.Status != "linked" || !relinked.Changed {
		t.Fatalf("relink=%#v err=%v", relinked, err)
	}

	producer, err := commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
		Brokers: []string{broker}, Topic: AlertEvidenceLinkEventTopic, BatchSize: 1,
		RequiredAcks: "all", Compression: "none", Async: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	handler.SetAlertEvidenceLinkRuntime(true, func(context.Context) error { return nil }, producer)
	processed, err := handler.drainAlertEvidenceLinkOutbox(ctx, "n012-publisher-"+runID, 10)
	if err != nil || processed != 3 {
		t.Fatalf("published events=%d err=%v", processed, err)
	}
	var historyCount, commandCount, outboxCount, auditCount int
	if err := db.QueryRow(`SELECT count(*) FROM alert_evidence_link_history WHERE tenant_id=$1`, tenantID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT count(*) FROM alert_evidence_link_commands WHERE tenant_id=$1`, tenantID).Scan(&commandCount)
	_ = db.QueryRow(`SELECT count(*) FROM alert_evidence_link_outbox WHERE tenant_id=$1 AND status='published'
		AND broker_partition>=0 AND broker_offset>=0 AND broker_acknowledged_at IS NOT NULL`, tenantID).Scan(&outboxCount)
	_ = db.QueryRow(`SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='alert_evidence_link'`, tenantID).Scan(&auditCount)
	if historyCount != 3 || commandCount != 4 || outboxCount != 3 || auditCount != 5 {
		t.Fatalf("history=%d commands=%d published=%d audit=%d", historyCount, commandCount, outboxCount, auditCount)
	}
}

func isEvidenceCommandCode(err error, code string) bool {
	var commandErr *alertEvidenceLinkCommandError
	return errors.As(err, &commandErr) && commandErr.code == code
}

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// integrationCampaignProjectionTarget is intentionally an external-effect
// recorder, not a database mock. The integration test exercises the real
// PostgreSQL leasing, ordering, watermark and retry transactions while target
// adapters are tested independently against their wire contracts.
type integrationCampaignProjectionTarget struct {
	name string

	mu            sync.Mutex
	calls         []string
	failRemaining map[string]int
}

func (target *integrationCampaignProjectionTarget) Name() string { return target.name }

func (target *integrationCampaignProjectionTarget) Projection(event CampaignProjectionEvent) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"contract":           "traffic.campaign.projection.integration.v1",
		"event_id":           event.EventID,
		"projection_key":     event.ProjectionKey(),
		"projection_version": event.ProjectionVersion(),
		"target":             target.name,
		"tenant_id":          event.TenantID,
	})
}

func (target *integrationCampaignProjectionTarget) Apply(
	_ context.Context,
	event CampaignProjectionEvent,
	_ []byte,
) error {
	target.mu.Lock()
	defer target.mu.Unlock()
	callKey := event.TenantID + "/" + event.ProjectionKey() + fmt.Sprintf("/%d", event.ProjectionVersion())
	target.calls = append(target.calls, callKey)
	if target.failRemaining[event.EventID] > 0 {
		target.failRemaining[event.EventID]--
		return errors.New("injected external target timeout")
	}
	return nil
}

func (target *integrationCampaignProjectionTarget) callCount() int {
	target.mu.Lock()
	defer target.mu.Unlock()
	return len(target.calls)
}

func (target *integrationCampaignProjectionTarget) callOrder() []string {
	target.mu.Lock()
	defer target.mu.Unlock()
	return append([]string(nil), target.calls...)
}

type campaignProjectionIntegrationHarness struct {
	db      *sql.DB
	worker  *CampaignTargetProjectionWorker
	targets map[string]*integrationCampaignProjectionTarget
}

func newCampaignProjectionIntegrationHarness(t *testing.T, maxAttempts int) campaignProjectionIntegrationHarness {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("CAMPAIGN_TARGET_PROJECTION_EPHEMERAL_PG_DSN"))
	if dsn == "" {
		t.Skip("CAMPAIGN_TARGET_PROJECTION_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_campaign_target_projection_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	targets := map[string]*integrationCampaignProjectionTarget{
		campaignProjectionClickHouse: {name: campaignProjectionClickHouse, failRemaining: map[string]int{}},
		campaignProjectionOpenSearch: {name: campaignProjectionOpenSearch, failRemaining: map[string]int{}},
		campaignProjectionNebula:     {name: campaignProjectionNebula, failRemaining: map[string]int{}},
	}
	worker, err := NewCampaignTargetProjectionWorker(db, []CampaignProjectionTarget{
		targets[campaignProjectionClickHouse],
		targets[campaignProjectionOpenSearch],
		targets[campaignProjectionNebula],
	}, CampaignTargetProjectionWorkerConfig{
		WorkerID:    "campaign-target-projection-integration",
		Lease:       time.Second,
		Interval:    time.Millisecond,
		MaxAttempts: maxAttempts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return campaignProjectionIntegrationHarness{db: db, worker: worker, targets: targets}
}

func insertCampaignProjectionIntegrationTenant(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,$2)`, tenantID, "Campaign Target Projection Integration"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM campaign_target_projection_watermarks WHERE tenant_id=$1`, tenantID)
		_, _ = db.Exec(`DELETE FROM campaign_event_projection_inbox WHERE tenant_id=$1`, tenantID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE tenant_id=$1`, tenantID)
	})
}

func insertAggregateCampaignProjectionIntegrationEvent(
	t *testing.T,
	db *sql.DB,
	tenantID, campaignID string,
	revision int64,
	receivedAt time.Time,
) string {
	t.Helper()
	eventID := uuid.NewString()
	eventType := "traffic.campaign.v2.StatusChanged"
	traceID := "trace-" + eventID
	payload, err := json.Marshal(map[string]interface{}{
		"event_id":          eventID,
		"event_type":        eventType,
		"tenant_id":         tenantID,
		"aggregate_type":    "campaign",
		"aggregate_id":      campaignID,
		"aggregate_version": revision,
		"campaign_id":       campaignID,
		"schema_version":    2,
		"partition_key":     tenantID + ":" + campaignID,
		"trace_id":          traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO campaign_event_projection_inbox
		(stream,event_id,tenant_id,aggregate_id,campaign_id,event_type,schema_version,
		 aggregate_revision,relation_revision,partition_key,trace_id,payload,
		 first_kafka_topic,first_kafka_partition,first_kafka_offset,received_at,available_at)
		VALUES ('aggregate',$1::uuid,$2,$3,$3,$4,2,$5,0,$6,$7,$8::jsonb,
		        'campaign.events.v2',0,$5,$9,$9)`,
		eventID, tenantID, campaignID, eventType, revision, tenantID+":"+campaignID,
		traceID, string(payload), receivedAt)
	if err != nil {
		t.Fatal(err)
	}
	return eventID
}

func requireCampaignProjectionState(
	t *testing.T,
	db *sql.DB,
	eventID, wantStatus string,
	wantAttempts int,
	wantTargets map[string]string,
) {
	t.Helper()
	var status string
	var attempts int
	var targetJSON []byte
	if err := db.QueryRow(`SELECT projection_status,attempt_count,target_status FROM campaign_event_projection_inbox WHERE stream='aggregate' AND event_id=$1::uuid`, eventID).
		Scan(&status, &attempts, &targetJSON); err != nil {
		t.Fatal(err)
	}
	var targetStatus map[string]string
	if err := json.Unmarshal(targetJSON, &targetStatus); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || attempts != wantAttempts {
		t.Fatalf("event=%s status=%s attempts=%d, want status=%s attempts=%d targets=%v", eventID, status, attempts, wantStatus, wantAttempts, targetStatus)
	}
	for target, want := range wantTargets {
		if targetStatus[target] != want {
			t.Fatalf("event=%s target=%s status=%s, want=%s (all=%v)", eventID, target, targetStatus[target], want, targetStatus)
		}
	}
}

func makeCampaignProjectionImmediatelyAvailable(t *testing.T, db *sql.DB, eventID string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE campaign_event_projection_inbox SET available_at=now() WHERE stream='aggregate' AND event_id=$1::uuid`, eventID); err != nil {
		t.Fatal(err)
	}
}

func projectCampaignIntegrationNext(t *testing.T, worker *CampaignTargetProjectionWorker, wantError bool) {
	t.Helper()
	found, err := worker.ProjectNext(context.Background())
	if !found {
		t.Fatal("expected an eligible campaign projection")
	}
	if wantError && err == nil {
		t.Fatal("expected campaign projection failure")
	}
	if !wantError && err != nil {
		t.Fatal(err)
	}
}

func TestCampaignTargetProjectionPostgresPartialRetryLeaseAndTargetedRebuild(t *testing.T) {
	harness := newCampaignProjectionIntegrationHarness(t, 3)
	tenantID := "campaign-target-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	insertCampaignProjectionIntegrationTenant(t, harness.db, tenantID)
	now := time.Now().UTC().Add(-time.Hour)

	partialEvent := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantID, "campaign-partial", 1, now)
	harness.targets[campaignProjectionNebula].failRemaining[partialEvent] = 1
	projectCampaignIntegrationNext(t, harness.worker, true)
	requireCampaignProjectionState(t, harness.db, partialEvent, "partial", 1, map[string]string{
		campaignProjectionClickHouse: "applied",
		campaignProjectionOpenSearch: "applied",
		campaignProjectionNebula:     "pending",
	})
	makeCampaignProjectionImmediatelyAvailable(t, harness.db, partialEvent)
	projectCampaignIntegrationNext(t, harness.worker, false)
	requireCampaignProjectionState(t, harness.db, partialEvent, "applied", 2, map[string]string{
		campaignProjectionClickHouse: "applied",
		campaignProjectionOpenSearch: "applied",
		campaignProjectionNebula:     "applied",
	})
	if harness.targets[campaignProjectionClickHouse].callCount() != 1 ||
		harness.targets[campaignProjectionOpenSearch].callCount() != 1 ||
		harness.targets[campaignProjectionNebula].callCount() != 2 {
		t.Fatalf("partial retry repeated successful targets: ch=%d os=%d nebula=%d",
			harness.targets[campaignProjectionClickHouse].callCount(),
			harness.targets[campaignProjectionOpenSearch].callCount(),
			harness.targets[campaignProjectionNebula].callCount())
	}

	staleLeaseEvent := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantID, "campaign-stale-lease", 1, now.Add(time.Second))
	if _, err := harness.db.Exec(`UPDATE campaign_event_projection_inbox
		SET projection_status='processing',locked_by='dead-worker',locked_until=now()-interval '1 second'
		WHERE stream='aggregate' AND event_id=$1::uuid`, staleLeaseEvent); err != nil {
		t.Fatal(err)
	}
	projectCampaignIntegrationNext(t, harness.worker, false)
	requireCampaignProjectionState(t, harness.db, staleLeaseEvent, "applied", 1, map[string]string{
		campaignProjectionClickHouse: "applied",
		campaignProjectionOpenSearch: "applied",
		campaignProjectionNebula:     "applied",
	})

	// Insert revision 3 with an earlier received timestamp than revision 2.
	// The worker must still serialize the aggregate by revision.
	revision3 := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantID, "campaign-order", 3, now.Add(2*time.Second))
	revision2 := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantID, "campaign-order", 2, now.Add(3*time.Second))
	projectCampaignIntegrationNext(t, harness.worker, false)
	requireCampaignProjectionState(t, harness.db, revision2, "applied", 1, map[string]string{campaignProjectionClickHouse: "applied"})
	requireCampaignProjectionState(t, harness.db, revision3, "pending", 0, map[string]string{campaignProjectionClickHouse: "pending"})
	projectCampaignIntegrationNext(t, harness.worker, false)
	requireCampaignProjectionState(t, harness.db, revision3, "applied", 1, map[string]string{campaignProjectionClickHouse: "applied"})

	beforeCH := harness.targets[campaignProjectionClickHouse].callCount()
	beforeOS := harness.targets[campaignProjectionOpenSearch].callCount()
	beforeNebula := harness.targets[campaignProjectionNebula].callCount()
	if _, err := harness.db.Exec(`DELETE FROM campaign_target_projection_watermarks
		WHERE tenant_id=$1 AND projection_key='campaign:campaign-order' AND target='clickhouse'`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.db.Exec(`UPDATE campaign_event_projection_inbox
		SET target_status=jsonb_set(target_status,'{clickhouse}','"pending"'::jsonb,false),
		    projection_status='pending',applied_at=NULL,available_at=now(),locked_by='',locked_until=NULL
		WHERE tenant_id=$1 AND stream='aggregate' AND campaign_id='campaign-order'`, tenantID); err != nil {
		t.Fatal(err)
	}
	projectCampaignIntegrationNext(t, harness.worker, false)
	projectCampaignIntegrationNext(t, harness.worker, false)
	if harness.targets[campaignProjectionClickHouse].callCount() != beforeCH+2 ||
		harness.targets[campaignProjectionOpenSearch].callCount() != beforeOS ||
		harness.targets[campaignProjectionNebula].callCount() != beforeNebula {
		t.Fatalf("targeted rebuild calls ch=%d/%d os=%d/%d nebula=%d/%d",
			harness.targets[campaignProjectionClickHouse].callCount(), beforeCH+2,
			harness.targets[campaignProjectionOpenSearch].callCount(), beforeOS,
			harness.targets[campaignProjectionNebula].callCount(), beforeNebula)
	}
	order := harness.targets[campaignProjectionClickHouse].callOrder()
	if len(order) < 2 || !strings.HasSuffix(order[len(order)-2], "/2") || !strings.HasSuffix(order[len(order)-1], "/3") {
		t.Fatalf("targeted rebuild order=%v, want revision 2 then 3", order)
	}
}

func TestCampaignTargetProjectionPostgresDeadCollisionAndTenantIsolation(t *testing.T) {
	harness := newCampaignProjectionIntegrationHarness(t, 1)
	now := time.Now().UTC().Add(-time.Hour)
	tenantA := "campaign-target-a-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	tenantB := "campaign-target-b-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	insertCampaignProjectionIntegrationTenant(t, harness.db, tenantA)
	insertCampaignProjectionIntegrationTenant(t, harness.db, tenantB)

	deadEvent := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantA, "campaign-dead", 1, now)
	harness.targets[campaignProjectionNebula].failRemaining[deadEvent] = 1
	projectCampaignIntegrationNext(t, harness.worker, true)
	requireCampaignProjectionState(t, harness.db, deadEvent, "dead", 1, map[string]string{
		campaignProjectionClickHouse: "applied",
		campaignProjectionOpenSearch: "applied",
		campaignProjectionNebula:     "dead",
	})

	firstIdentity := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantA, "campaign-collision", 1, now.Add(time.Second))
	projectCampaignIntegrationNext(t, harness.worker, false)
	conflictingIdentity := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantA, "campaign-collision", 1, now.Add(2*time.Second))
	beforeCalls := map[string]int{}
	for name, target := range harness.targets {
		beforeCalls[name] = target.callCount()
	}
	projectCampaignIntegrationNext(t, harness.worker, true)
	requireCampaignProjectionState(t, harness.db, conflictingIdentity, "dead", 1, map[string]string{
		campaignProjectionClickHouse: "dead",
		campaignProjectionOpenSearch: "dead",
		campaignProjectionNebula:     "dead",
	})
	for name, target := range harness.targets {
		if target.callCount() != beforeCalls[name] {
			t.Fatalf("identity collision reached %s external target", name)
		}
	}
	var watermarkEvent string
	if err := harness.db.QueryRow(`SELECT event_id::text FROM campaign_target_projection_watermarks
		WHERE tenant_id=$1 AND projection_key='campaign:campaign-collision' AND target='clickhouse'`, tenantA).Scan(&watermarkEvent); err != nil {
		t.Fatal(err)
	}
	if watermarkEvent != firstIdentity {
		t.Fatalf("collision replaced winner watermark event=%s want=%s", watermarkEvent, firstIdentity)
	}

	sharedCampaign := "campaign-shared-id"
	eventA := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantA, sharedCampaign, 1, now.Add(3*time.Second))
	eventB := insertAggregateCampaignProjectionIntegrationEvent(t, harness.db, tenantB, sharedCampaign, 1, now.Add(3*time.Second))
	projectCampaignIntegrationNext(t, harness.worker, false)
	projectCampaignIntegrationNext(t, harness.worker, false)
	requireCampaignProjectionState(t, harness.db, eventA, "applied", 1, map[string]string{campaignProjectionClickHouse: "applied"})
	requireCampaignProjectionState(t, harness.db, eventB, "applied", 1, map[string]string{campaignProjectionClickHouse: "applied"})
	var tenantWatermarks int
	if err := harness.db.QueryRow(`SELECT count(*) FROM campaign_target_projection_watermarks
		WHERE tenant_id IN ($1,$2) AND projection_key=$3`, tenantA, tenantB, "campaign:"+sharedCampaign).Scan(&tenantWatermarks); err != nil {
		t.Fatal(err)
	}
	if tenantWatermarks != 6 {
		t.Fatalf("tenant-isolated watermark count=%d want=6", tenantWatermarks)
	}
}

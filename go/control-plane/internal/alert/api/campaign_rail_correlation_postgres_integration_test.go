package api

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestCampaignRailCorrelationPostgresClosedWindow(t *testing.T) {
	dsn := os.Getenv("CAMPAIGN_RAIL_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("CAMPAIGN_RAIL_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, statement := range []string{
		`DELETE FROM campaign_rail_reconcile_runs_v1 WHERE tenant_id='tenant-correlation-it'`,
		`DELETE FROM campaign_rail_correlation_v1 WHERE tenant_id='tenant-correlation-it'`,
		`DELETE FROM campaign_event_projection_inbox WHERE tenant_id='tenant-correlation-it'`,
		`DELETE FROM campaign_proto_projection_current_v1 WHERE tenant_id='tenant-correlation-it'`,
		`DELETE FROM campaign_proto_projection_inbox_v1 WHERE tenant_id='tenant-correlation-it'`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, statement := range []string{
			`DELETE FROM campaign_rail_reconcile_runs_v1 WHERE tenant_id='tenant-correlation-it'`,
			`DELETE FROM campaign_rail_correlation_v1 WHERE tenant_id='tenant-correlation-it'`,
			`DELETE FROM campaign_event_projection_inbox WHERE tenant_id='tenant-correlation-it'`,
			`DELETE FROM campaign_proto_projection_current_v1 WHERE tenant_id='tenant-correlation-it'`,
			`DELETE FROM campaign_proto_projection_inbox_v1 WHERE tenant_id='tenant-correlation-it'`,
		} {
			_, _ = db.Exec(statement)
		}
	})

	windowThrough := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	// The authority projection is deliberately late. The same immutable CEP
	// event-time window must be replayable at a later coordinated as_of.
	asOf := windowThrough.Add(5 * time.Minute)
	cepEventID := "11111111-1111-4111-8111-111111111111"
	campaign := &trafficv1.Campaign{TenantId: "tenant-correlation-it", CampaignId: "cep-campaign",
		EventId: cepEventID, TsStart: windowThrough.Add(-10 * time.Minute).UnixMilli(), TsEnd: windowThrough.Add(-5 * time.Minute).UnixMilli(),
		CampaignType: "scan", Score: 0.9, Alerts: []string{"alert-a", "alert-b"}, Entities: []string{"host-a"}}
	payload, err := proto.Marshal(campaign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO campaign_proto_projection_inbox_v1 (
		event_id,tenant_id,campaign_id,campaign_type,event_time_start_ms,event_time_end_ms,payload_sha256,
		payload_protobuf,source_topic,source_partition,source_offset,received_at,state,applied_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,repeat('a',64),$7,'campaigns.v1',1,10,$8,'applied',$8)`,
		cepEventID, campaign.TenantId, campaign.CampaignId, campaign.CampaignType, campaign.TsStart, campaign.TsEnd, payload, windowThrough.Add(-4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO campaign_proto_projection_current_v1 (
		tenant_id,campaign_id,event_id,payload_sha256,event_time_start_ms,event_time_end_ms,campaign_type,score,
		source_topic,source_partition,source_offset) VALUES ($1,$2,$3::uuid,repeat('a',64),$4,$5,$6,$7,'campaigns.v1',1,10)`,
		campaign.TenantId, campaign.CampaignId, cepEventID, campaign.TsStart, campaign.TsEnd, campaign.CampaignType, campaign.Score); err != nil {
		t.Fatal(err)
	}
	aggregateEventID := "22222222-2222-4222-8222-222222222222"
	if _, err := db.ExecContext(ctx, `INSERT INTO campaign_event_projection_inbox (
		stream,event_id,tenant_id,aggregate_id,campaign_id,relation_id,alert_id,event_type,schema_version,
		aggregate_revision,relation_revision,partition_key,trace_id,payload,first_kafka_topic,
		first_kafka_partition,first_kafka_offset,received_at) VALUES
		('aggregate',$1::uuid,$2,'authority-campaign','authority-campaign',NULL,'','traffic.campaign.v2.StatusChanged',2,
		 3,0,$2||':authority-campaign','trace-aggregate','{}','campaign.events.v2',0,20,$3)`,
		aggregateEventID, campaign.TenantId, windowThrough.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	members := []struct{ eventID, relationID, alertID string }{
		{"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", "alert-a"},
		{"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", "alert-b"},
	}
	for index, member := range members {
		if _, err := db.ExecContext(ctx, `INSERT INTO campaign_event_projection_inbox (
			stream,event_id,tenant_id,aggregate_id,campaign_id,relation_id,alert_id,event_type,schema_version,
			aggregate_revision,relation_revision,partition_key,trace_id,payload,first_kafka_topic,
			first_kafka_partition,first_kafka_offset,received_at) VALUES
			('membership',$1::uuid,$2,$3::text,'authority-campaign',$3::uuid,$4,'traffic.campaign.v2.AlertLinked',2,
			 3,$5,$2||':authority-campaign','trace-member','{}','campaign.membership.events.v2',2,$6,$7)`,
			member.eventID, campaign.TenantId, member.relationID, member.alertID, index+1, 30+index, windowThrough.Add(3*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	handler := NewSystemHandler(nil, db, zap.NewNop())
	scope := CampaignRailScope{TenantID: campaign.TenantId, WindowFrom: windowThrough.Add(-time.Hour),
		WindowThrough: windowThrough, AsOf: asOf, MaxCampaigns: 100}
	first, err := handler.ProjectCampaignRailCorrelations(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := handler.ProjectCampaignRailCorrelations(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if first.Inserted != 1 || first.ByState["correlated"] != 1 || second.Replayed != 1 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	laterScope := scope
	laterScope.AsOf = asOf.Add(time.Minute)
	later, err := handler.ProjectCampaignRailCorrelations(ctx, laterScope)
	if err != nil {
		t.Fatal(err)
	}
	if later.Inserted != 0 || later.Replayed != 1 || later.Receipts[0].CorrelationSHA256 != first.Receipts[0].CorrelationSHA256 {
		t.Fatalf("processing-time-only replay was not idempotent: first=%+v later=%+v", first, later)
	}
	var correlationCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM campaign_rail_correlation_v1 WHERE tenant_id=$1`, campaign.TenantId).
		Scan(&correlationCount); err != nil || correlationCount != 1 {
		t.Fatalf("correlation_count=%d err=%v", correlationCount, err)
	}
	var state, authorityID string
	var confidence float64
	var relationRevision int64
	if err := db.QueryRowContext(ctx, `SELECT state,aggregate_campaign_id,confidence,relation_revision
		FROM campaign_rail_correlation_v1 WHERE tenant_id=$1`, campaign.TenantId).
		Scan(&state, &authorityID, &confidence, &relationRevision); err != nil {
		t.Fatal(err)
	}
	if state != "correlated" || authorityID != "authority-campaign" || confidence != 1 || relationRevision != 2 {
		t.Fatalf("state=%s authority=%s confidence=%v relation_revision=%d", state, authorityID, confidence, relationRevision)
	}
	reconcile, err := handler.ReconcileCampaignRails(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if reconcile.State != "exact" || reconcile.MissingCount != 0 || reconcile.ExtraCount != 0 || reconcile.CorrelatedCount != 1 {
		t.Fatalf("reconcile=%+v", reconcile)
	}
	laterReconcile, err := handler.ReconcileCampaignRails(ctx, laterScope)
	if err != nil {
		t.Fatal(err)
	}
	if laterReconcile.State != "exact" || laterReconcile.MissingCount != 0 || laterReconcile.ExtraCount != 0 {
		t.Fatalf("later_reconcile=%+v", laterReconcile)
	}
}

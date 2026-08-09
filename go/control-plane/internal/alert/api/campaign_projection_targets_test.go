package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

type recordingCampaignClickHouseClient struct {
	query      string
	args       []interface{}
	pingErr    error
	tableFound bool
	tableDB    string
	tableName  string
}

func (client *recordingCampaignClickHouseClient) Exec(
	_ context.Context,
	query string,
	args ...interface{},
) error {
	client.query = query
	client.args = append([]interface{}(nil), args...)
	return nil
}

func (client *recordingCampaignClickHouseClient) Ping(context.Context) error {
	return client.pingErr
}

func (client *recordingCampaignClickHouseClient) TableExists(
	_ context.Context,
	database string,
	table string,
) (bool, error) {
	client.tableDB = database
	client.tableName = table
	return client.tableFound, nil
}

type recordingCampaignNebulaWriter struct {
	readyErr    error
	entity      graphNebula.CampaignEntityProjection
	membership  graphNebula.CampaignMembershipProjection
	entityCalls int
	memberCalls int
}

func (writer *recordingCampaignNebulaWriter) Ready(context.Context) error { return writer.readyErr }

func (writer *recordingCampaignNebulaWriter) UpsertCampaignEntity(
	_ context.Context,
	projection graphNebula.CampaignEntityProjection,
) error {
	writer.entityCalls++
	writer.entity = projection
	return nil
}

func (writer *recordingCampaignNebulaWriter) ApplyCampaignMembership(
	_ context.Context,
	projection graphNebula.CampaignMembershipProjection,
) error {
	writer.memberCalls++
	writer.membership = projection
	return nil
}

func validCampaignAggregateProjectionEvent() CampaignProjectionEvent {
	return CampaignProjectionEvent{
		Stream:            campaignAggregateStream,
		EventID:           "11111111-1111-4111-8111-111111111111",
		TenantID:          "tenant-a",
		AggregateID:       "campaign-a",
		CampaignID:        "campaign-a",
		EventType:         "traffic.campaign.v2.StatusChanged",
		SchemaVersion:     2,
		AggregateRevision: 3,
		PartitionKey:      "tenant-a:campaign-a",
		TraceID:           "trace-campaign-3",
		Payload:           []byte(validCampaignAggregateProjectionPayload()),
		ReceivedAt:        time.Date(2026, 8, 1, 1, 2, 3, 456000000, time.UTC),
	}
}

func validCampaignMembershipProjectionEvent(linked bool) CampaignProjectionEvent {
	eventType := "traffic.campaign.v2.AlertUnlinked"
	if linked {
		eventType = "traffic.campaign.v2.AlertLinked"
	}
	payload := `{"event_id":"22222222-2222-4222-8222-222222222222","event_type":"` + eventType + `","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":7,"campaign_id":"campaign-a","relation_id":"33333333-3333-4333-8333-333333333333","alert_id":"alert-a","relation_revision":4,"campaign_revision":7,"schema_version":2,"partition_key":"tenant-a:campaign-a","trace_id":"trace-membership-4","status":"active","member_count":1}`
	return CampaignProjectionEvent{
		Stream:            campaignMembershipStream,
		EventID:           "22222222-2222-4222-8222-222222222222",
		TenantID:          "tenant-a",
		AggregateID:       "33333333-3333-4333-8333-333333333333",
		CampaignID:        "campaign-a",
		RelationID:        "33333333-3333-4333-8333-333333333333",
		AlertID:           "alert-a",
		EventType:         eventType,
		SchemaVersion:     2,
		AggregateRevision: 7,
		RelationRevision:  4,
		PartitionKey:      "tenant-a:campaign-a",
		TraceID:           "trace-membership-4",
		Payload:           []byte(payload),
		ReceivedAt:        time.Date(2026, 8, 1, 1, 3, 4, 0, time.UTC),
	}
}

func TestClickHouseCampaignProjectionIsDeterministicAndUsesVersionedTable(t *testing.T) {
	client := &recordingCampaignClickHouseClient{tableFound: true}
	target, err := newClickHouseCampaignProjection(client, "traffic.campaign_projection_events_v2")
	if err != nil {
		t.Fatal(err)
	}
	event := validCampaignAggregateProjectionEvent()
	first, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("ClickHouse campaign projection is not deterministic")
	}
	if err := target.Apply(context.Background(), event, first); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(client.query, "INSERT INTO traffic.campaign_projection_events_v2") || len(client.args) != 15 {
		t.Fatalf("unexpected ClickHouse write query=%q args=%d", client.query, len(client.args))
	}
	if client.args[2] != event.TenantID || client.args[4] != event.ProjectionKey() || client.args[5] != event.ProjectionVersion() {
		t.Fatalf("ClickHouse write lost projection identity: %#v", client.args)
	}
	if err := target.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.tableDB != "traffic" || client.tableName != "campaign_projection_events_v2" {
		t.Fatalf("unexpected readiness table %s.%s", client.tableDB, client.tableName)
	}
}

func TestClickHouseCampaignProjectionRejectsIdentifierInjection(t *testing.T) {
	_, err := newClickHouseCampaignProjection(
		&recordingCampaignClickHouseClient{},
		"traffic.campaign_projection_events_v2;DROP TABLE traffic.alerts",
	)
	if err == nil {
		t.Fatal("ClickHouse table identifier injection must fail closed")
	}
}

func TestOpenSearchCampaignProjectionUsesStableStateIDAndExternalVersion(t *testing.T) {
	event := validCampaignAggregateProjectionEvent()
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		expectedPath := "/campaign-projections-v2-write/_doc/" + campaignProjectionStateID(event)
		if request.URL.Path != expectedPath {
			t.Fatalf("path=%s want=%s", request.URL.Path, expectedPath)
		}
		if request.URL.Query().Get("version") != "3" || request.URL.Query().Get("version_type") != "external_gte" {
			t.Fatalf("query=%s", request.URL.RawQuery)
		}
		requestBody, _ = io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"result":"created"}`))
	}))
	defer server.Close()
	target, err := NewOpenSearchCampaignProjection(
		[]string{server.URL}, "", "", "campaign-projections-v2-write",
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatal(err)
	}
	if string(requestBody) != string(projection) {
		t.Fatal("OpenSearch body differs from deterministic projection")
	}
}

func TestNebulaCampaignProjectionMapsAggregateAndMembershipWatermarks(t *testing.T) {
	writer := &recordingCampaignNebulaWriter{}
	target, err := NewNebulaCampaignProjection(writer)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := validCampaignAggregateProjectionEvent()
	aggregateProjection, err := target.Projection(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), aggregate, aggregateProjection); err != nil {
		t.Fatal(err)
	}
	if writer.entityCalls != 1 || writer.entity.Revision != 3 || writer.entity.TenantID != "tenant-a" {
		t.Fatalf("unexpected campaign entity: %+v", writer.entity)
	}
	aggregateHash := sha256.Sum256(aggregateProjection)
	if writer.entity.Metadata["projection_sha256"] != hex.EncodeToString(aggregateHash[:]) {
		t.Fatalf("campaign entity did not retain deterministic projection hash: %+v", writer.entity.Metadata)
	}
	linked := validCampaignMembershipProjectionEvent(true)
	linkedProjection, err := target.Projection(linked)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), linked, linkedProjection); err != nil {
		t.Fatal(err)
	}
	if writer.memberCalls != 1 || !writer.membership.Linked || writer.membership.Revision != 4 ||
		writer.membership.CampaignRevision != 7 || writer.membership.RelationID != linked.RelationID {
		t.Fatalf("unexpected campaign membership: %+v", writer.membership)
	}
	linkedHash := sha256.Sum256(linkedProjection)
	if writer.membership.Metadata["projection_sha256"] != hex.EncodeToString(linkedHash[:]) {
		t.Fatalf("campaign membership did not retain deterministic projection hash: %+v", writer.membership.Metadata)
	}
	unlinked := validCampaignMembershipProjectionEvent(false)
	unlinkedProjection, err := target.Projection(unlinked)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), unlinked, unlinkedProjection); err != nil {
		t.Fatal(err)
	}
	if writer.memberCalls != 2 || writer.membership.Linked {
		t.Fatalf("unlink did not remove deterministic membership: %+v", writer.membership)
	}
}

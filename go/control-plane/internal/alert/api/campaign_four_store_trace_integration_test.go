package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	graphConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
	nebula_go "github.com/vesoft-inc/nebula-go/v3"
	"go.uber.org/zap"
)

// TestCampaignEventRealKafkaFourStoreTrace starts with the exact event read
// back from the real Kafka boundary. It then drives the production target
// worker and proves that PostgreSQL, ClickHouse, OpenSearch and NebulaGraph all
// retain the same event, trace, revision and deterministic projection hash.
// Every dependency must be an explicitly marked disposable loopback instance.
func TestCampaignEventRealKafkaFourStoreTrace(t *testing.T) {
	requireCampaignFourStoreIntegrationEnvironment(t)
	requireEphemeralKafkaBroker(t, os.Getenv("CAMPAIGN_EVENT_EPHEMERAL_KAFKA_BROKER"))
	result := runCampaignEventRealKafkaProjectionBoundary(t)
	events := readCampaignProjectionEventsForTrace(t, result.db, result.tenantID, result.eventID)
	if len(events) != 2 {
		t.Fatalf("durable Kafka event rows=%d, want aggregate and membership", len(events))
	}

	clickHouseClient, clickHouseTarget := newFourStoreClickHouseTarget(t)
	openSearchAddress, openSearchTarget := newFourStoreOpenSearchTarget(t)
	nebulaSession, nebulaStore, nebulaTarget := newFourStoreNebulaTarget(t)
	defer nebulaSession.Release()
	defer nebulaStore.Close()

	worker, err := NewCampaignTargetProjectionWorker(
		result.db,
		[]CampaignProjectionTarget{clickHouseTarget, openSearchTarget, nebulaTarget},
		CampaignTargetProjectionWorkerConfig{
			WorkerID:    "campaign-four-store-trace",
			Lease:       5 * time.Second,
			Interval:    time.Millisecond,
			MaxAttempts: 3,
			Logger:      zap.NewNop(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	for projected := 0; projected < len(events); projected++ {
		found, projectErr := worker.ProjectNext(context.Background())
		if projectErr != nil || !found {
			t.Fatalf("project event %d found=%v err=%v", projected, found, projectErr)
		}
	}
	if found, projectErr := worker.ProjectNext(context.Background()); projectErr != nil || found {
		t.Fatalf("projection queue did not drain: found=%v err=%v", found, projectErr)
	}

	for _, event := range events {
		projection, projectionErr := renderCampaignProjectionDocument(event)
		if projectionErr != nil {
			t.Fatal(projectionErr)
		}
		hash := sha256.Sum256(projection)
		wantSHA := hex.EncodeToString(hash[:])
		requirePostgresCampaignProjectionReceipts(t, result.db, event, wantSHA)
		requireClickHouseCampaignProjectionReceipt(t, clickHouseClient, event, wantSHA)
		requireOpenSearchCampaignProjectionReceipt(t, openSearchAddress, event, projection, wantSHA)
	}
	requireNebulaCampaignProjectionReceipts(t, nebulaSession, events, result)
}

func requireCampaignFourStoreIntegrationEnvironment(t *testing.T) {
	t.Helper()
	required := []string{
		"CAMPAIGN_EVENT_EPHEMERAL_PG_DSN",
		"CAMPAIGN_EVENT_EPHEMERAL_KAFKA_BROKER",
		"CAMPAIGN_PROJECTION_EPHEMERAL_CLICKHOUSE_HOST",
		"CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_URL",
		"CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_ADDRESS",
	}
	for _, name := range required {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Skip("explicit four-store integration settings are required")
		}
	}
	if os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP") != "ephemeral-only" {
		t.Skip("explicit ephemeral NebulaGraph sentinel is required")
	}
}

func requireEphemeralKafkaBroker(t *testing.T, address string) {
	t.Helper()
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || (host != "127.0.0.1" && host != "localhost") {
		t.Fatalf("ephemeral Kafka must use a loopback broker: %q", address)
	}
}

func readCampaignProjectionEventsForTrace(
	t *testing.T,
	db *sql.DB,
	tenantID string,
	eventID string,
) []CampaignProjectionEvent {
	t.Helper()
	rows, err := db.Query(`
		SELECT stream,event_id::text,tenant_id,aggregate_id,campaign_id,
		       coalesce(relation_id::text,''),alert_id,event_type,schema_version,
		       aggregate_revision,relation_revision,partition_key,trace_id,
		       payload::text,received_at
		FROM campaign_event_projection_inbox
		WHERE tenant_id=$1 AND event_id=$2::uuid
		ORDER BY stream`, tenantID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	events := make([]CampaignProjectionEvent, 0, 2)
	for rows.Next() {
		var event CampaignProjectionEvent
		var payload string
		if err := rows.Scan(
			&event.Stream, &event.EventID, &event.TenantID, &event.AggregateID,
			&event.CampaignID, &event.RelationID, &event.AlertID, &event.EventType,
			&event.SchemaVersion, &event.AggregateRevision, &event.RelationRevision,
			&event.PartitionKey, &event.TraceID, &payload, &event.ReceivedAt,
		); err != nil {
			t.Fatal(err)
		}
		event.Payload = json.RawMessage(payload)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func newFourStoreClickHouseTarget(
	t *testing.T,
) (*storage.ClickHouseClient, *ClickHouseCampaignProjection) {
	t.Helper()
	host := strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_CLICKHOUSE_HOST"))
	loopbackHost, _, err := net.SplitHostPort(host)
	if err != nil || (loopbackHost != "127.0.0.1" && loopbackHost != "localhost") {
		t.Fatalf("ephemeral ClickHouse must use a loopback native address: %q", host)
	}
	const database = "campaign_projection_ephemeral"
	client, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts: []string{host}, Database: database, Username: "default",
		MaxOpenConns: 2, MaxIdleConns: 1, DialTimeout: 5 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		CompressionLZ4: true, EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	row, err := client.QueryRow(context.Background(), `SELECT marker FROM codex_ephemeral_campaign_projection_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var sentinel string
	if err := row.Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel ClickHouse: marker=%q err=%v", sentinel, err)
	}
	target, err := NewClickHouseCampaignProjection(client, database+".campaign_projection_events_v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client, target
}

func newFourStoreOpenSearchTarget(t *testing.T) (string, *OpenSearchCampaignProjection) {
	t.Helper()
	address := strings.TrimRight(strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_URL")), "/")
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatalf("ephemeral OpenSearch must use a loopback URL: %q", address)
	}
	sentinelRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet,
		address+"/codex-ephemeral-campaign-projection-sentinel/_doc/ephemeral-only", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	sentinelResponse, err := http.DefaultClient.Do(sentinelRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer sentinelResponse.Body.Close()
	var sentinel struct {
		Source map[string]interface{} `json:"_source"`
	}
	if sentinelResponse.StatusCode != http.StatusOK || json.NewDecoder(sentinelResponse.Body).Decode(&sentinel) != nil || sentinel.Source["marker"] != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel OpenSearch: status=%s source=%v", sentinelResponse.Status, sentinel.Source)
	}
	target, err := NewOpenSearchCampaignProjection(
		[]string{address},
		os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_USERNAME"),
		os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_PASSWORD"),
		"campaign-projections-ephemeral-write",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	return address, target
}

func newFourStoreNebulaTarget(
	t *testing.T,
) (*nebula_go.Session, *graphNebula.WorkbenchStore, *NebulaCampaignProjection) {
	t.Helper()
	if os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP") != "ephemeral-only" {
		t.Fatal("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP must equal ephemeral-only")
	}
	address := strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_ADDRESS"))
	host, port := requireEphemeralNebulaAddress(t, address)
	username := strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_USERNAME"))
	if username == "" {
		username = "root"
	}
	password := os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_PASSWORD")
	if password == "" {
		password = "nebula"
	}
	poolConfig := nebula_go.GetDefaultConf()
	poolConfig.TimeOut = 5 * time.Second
	poolConfig.MaxConnPoolSize = 2
	poolConfig.MinConnPoolSize = 1
	pool, err := nebula_go.NewConnectionPool(
		[]nebula_go.HostAddress{{Host: host, Port: port}}, poolConfig, nebula_go.DefaultLogger{},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	session, err := pool.GetSession(username, password)
	if err != nil {
		t.Fatal(err)
	}
	requireNebulaStatement(t, session, "USE campaign_projection_ephemeral;")
	store, err := graphNebula.NewWorkbenchStore(graphConfig.NebulaConfig{
		Enabled: true, Addresses: []string{address}, Username: username, Password: password,
		Space: "campaign_projection_ephemeral", Timeout: 5 * time.Second, IdleTime: time.Minute,
		MaxPoolSize: 2, MinPoolSize: 1,
	}, zap.NewNop())
	if err != nil {
		session.Release()
		t.Fatal(err)
	}
	target, err := NewNebulaCampaignProjection(store)
	if err != nil {
		store.Close()
		session.Release()
		t.Fatal(err)
	}
	if err := target.Ready(context.Background()); err != nil {
		store.Close()
		session.Release()
		t.Fatal(err)
	}
	return session, store, target
}

func requirePostgresCampaignProjectionReceipts(
	t *testing.T,
	db *sql.DB,
	event CampaignProjectionEvent,
	wantSHA string,
) {
	t.Helper()
	var status, targetStatus string
	if err := db.QueryRow(`
		SELECT projection_status,target_status::text
		FROM campaign_event_projection_inbox WHERE stream=$1 AND event_id=$2::uuid`,
		event.Stream, event.EventID,
	).Scan(&status, &targetStatus); err != nil {
		t.Fatal(err)
	}
	if status != "applied" || targetStatus != `{"clickhouse": "applied", "opensearch": "applied", "nebulagraph": "applied"}` {
		t.Fatalf("PostgreSQL event %s/%s status=%s targets=%s", event.Stream, event.EventID, status, targetStatus)
	}
	rows, err := db.Query(`
		SELECT target,event_id::text,projection_version,projection_sha256
		FROM campaign_target_projection_watermarks
		WHERE tenant_id=$1 AND projection_key=$2 ORDER BY target`, event.TenantID, event.ProjectionKey())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var target, eventID, projectionSHA string
		var version int64
		if err := rows.Scan(&target, &eventID, &version, &projectionSHA); err != nil {
			t.Fatal(err)
		}
		if eventID != event.EventID || version != event.ProjectionVersion() || projectionSHA != wantSHA {
			t.Fatalf("PostgreSQL %s receipt event=%s version=%d sha=%s", target, eventID, version, projectionSHA)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("PostgreSQL target receipts=%d, want 3", count)
	}
}

func requireClickHouseCampaignProjectionReceipt(
	t *testing.T,
	client *storage.ClickHouseClient,
	event CampaignProjectionEvent,
	wantSHA string,
) {
	t.Helper()
	row, err := client.QueryRow(context.Background(), `
		SELECT count(),any(trace_id),any(projection_version),any(projection_sha256)
		FROM campaign_projection_events_v2 FINAL
		WHERE tenant_id=? AND stream=? AND event_id=toUUID(?)`, event.TenantID, event.Stream, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	var count uint64
	var traceID, projectionSHA string
	var version uint64
	if err := row.Scan(&count, &traceID, &version, &projectionSHA); err != nil {
		t.Fatal(err)
	}
	if count != 1 || traceID != event.TraceID || version != uint64(event.ProjectionVersion()) || projectionSHA != wantSHA {
		t.Fatalf("ClickHouse %s receipt count=%d trace=%s version=%d sha=%s", event.Stream, count, traceID, version, projectionSHA)
	}
}

func requireOpenSearchCampaignProjectionReceipt(
	t *testing.T,
	address string,
	event CampaignProjectionEvent,
	wantProjection []byte,
	wantSHA string,
) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet,
		address+"/campaign-projections-ephemeral-read/_doc/"+campaignProjectionStateID(event), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OpenSearch receipt status=%s body=%s", response.Status, body)
	}
	var document struct {
		Source json.RawMessage `json:"_source"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	var observed, expected campaignProjectionDocument
	if err := json.Unmarshal(document.Source, &observed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantProjection, &expected); err != nil {
		t.Fatal(err)
	}
	canonicalSource, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(canonicalSource)
	if observed.EventID != event.EventID || observed.TraceID != event.TraceID ||
		observed.ProjectionVersion != event.ProjectionVersion() || observed.ProjectionID != expected.ProjectionID ||
		hex.EncodeToString(hash[:]) != wantSHA {
		t.Fatalf("OpenSearch %s receipt event=%s trace=%s version=%d sha=%s", event.Stream,
			observed.EventID, observed.TraceID, observed.ProjectionVersion, hex.EncodeToString(hash[:]))
	}
}

func requireNebulaCampaignProjectionReceipts(
	t *testing.T,
	session *nebula_go.Session,
	events []CampaignProjectionEvent,
	result campaignEventKafkaIntegrationResult,
) {
	t.Helper()
	metadataByStream := make(map[string]map[string]interface{}, 2)
	entityResult := requireNebulaStatement(t, session, fmt.Sprintf(
		`FETCH PROP ON entity %q YIELD entity.metadata_json AS metadata_json;`,
		ephemeralNebulaVID(result.tenantID, result.campaignID),
	))
	metadataByStream[campaignAggregateStream] = requireCampaignNebulaMetadata(t, entityResult)
	edgeResult := requireNebulaStatement(t, session, fmt.Sprintf(
		`FETCH PROP ON relation %q->%q@0 YIELD relation.attributes_json AS metadata_json;`,
		ephemeralNebulaVID(result.tenantID, result.campaignID),
		ephemeralNebulaVID(result.tenantID, result.alertID),
	))
	metadataByStream[campaignMembershipStream] = requireCampaignNebulaMetadata(t, edgeResult)
	for _, event := range events {
		projection, err := renderCampaignProjectionDocument(event)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(projection)
		metadata := metadataByStream[event.Stream]
		if metadata["event_id"] != event.EventID || metadata["trace_id"] != event.TraceID ||
			metadata["projection_sha256"] != hex.EncodeToString(hash[:]) ||
			metadata["projection_version"] != float64(event.ProjectionVersion()) {
			t.Fatalf("NebulaGraph %s receipt=%v", event.Stream, metadata)
		}
	}
}

func requireCampaignNebulaMetadata(t *testing.T, result *nebula_go.ResultSet) map[string]interface{} {
	t.Helper()
	if result.GetRowSize() != 1 {
		t.Fatalf("NebulaGraph projection rows=%d, want 1", result.GetRowSize())
	}
	record, err := result.GetRowValuesByIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	value, err := record.GetValueByColName("metadata_json")
	if err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := value.AsString()
	if err != nil {
		t.Fatal(err)
	}
	metadata := make(map[string]interface{})
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

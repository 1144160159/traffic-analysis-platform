package api

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	graphConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
	nebula_go "github.com/vesoft-inc/nebula-go/v3"
	"go.uber.org/zap"
)

func TestClickHouseCampaignProjectionEphemeralIntegration(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_CLICKHOUSE_HOST"))
	if host == "" {
		t.Skip("CAMPAIGN_PROJECTION_EPHEMERAL_CLICKHOUSE_HOST is not set")
	}
	const database = "campaign_projection_ephemeral"
	client, err := storage.NewClickHouseClient(storage.ClickHouseConfig{
		Hosts:               []string{host},
		Database:            database,
		Username:            "default",
		MaxOpenConns:        2,
		MaxIdleConns:        1,
		DialTimeout:         5 * time.Second,
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        5 * time.Second,
		CompressionLZ4:      true,
		EnableAutoReconnect: false,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var sentinel string
	row, err := client.QueryRow(context.Background(), `SELECT marker FROM codex_ephemeral_campaign_projection_sentinel LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
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
	event := validCampaignAggregateProjectionEvent()
	projection, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(projection)
	var rows, identities uint64
	var projectionSHA string
	var projectionVersion uint64
	row, err = client.QueryRow(context.Background(), `
		SELECT count(),uniqExact(projection_id),any(projection_sha256),any(projection_version)
		FROM campaign_projection_events_v2 FINAL WHERE event_id=?`, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if err := row.Scan(&rows, &identities, &projectionSHA, &projectionVersion); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || identities != 1 || projectionSHA != hex.EncodeToString(wantHash[:]) || projectionVersion != uint64(event.ProjectionVersion()) {
		t.Fatalf("ClickHouse projection rows=%d identities=%d sha=%s version=%d", rows, identities, projectionSHA, projectionVersion)
	}
}

func TestOpenSearchCampaignProjectionEphemeralIntegration(t *testing.T) {
	address := strings.TrimRight(strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_URL")), "/")
	if address == "" {
		t.Skip("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_URL is not set")
	}
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatalf("ephemeral OpenSearch must use a loopback URL: %q", address)
	}
	username := os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_USERNAME")
	password := os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_PASSWORD")
	requestJSON := func(method, path string) map[string]interface{} {
		t.Helper()
		request, requestErr := http.NewRequestWithContext(context.Background(), method, address+path, nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if username != "" {
			request.SetBasicAuth(username, password)
		}
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("OpenSearch %s %s: status=%s body=%s", method, path, response.Status, body)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	sentinel := requestJSON(http.MethodGet, "/codex-ephemeral-campaign-projection-sentinel/_doc/ephemeral-only")
	source, _ := sentinel["_source"].(map[string]interface{})
	if source["marker"] != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel OpenSearch: %v", sentinel)
	}
	const writeAlias = "campaign-projections-ephemeral-write"
	target, err := NewOpenSearchCampaignProjection([]string{address}, username, password, writeAlias)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	event := validCampaignAggregateProjectionEvent()
	projection, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatal(err)
	}
	document := requestJSON(http.MethodGet, "/campaign-projections-ephemeral-read/_doc/"+campaignProjectionStateID(event))
	documentSource, _ := document["_source"].(map[string]interface{})
	if documentSource["event_id"] != event.EventID || documentSource["projection_version"] != float64(event.ProjectionVersion()) {
		t.Fatalf("OpenSearch projection identity mismatch: %v", documentSource)
	}

	older := campaignProjectionAggregateEventAt(2, "44444444-4444-4444-8444-444444444444")
	olderProjection, err := target.Projection(older)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), older, olderProjection); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("older external version must be rejected, err=%v", err)
	}
	document = requestJSON(http.MethodGet, "/campaign-projections-ephemeral-read/_doc/"+campaignProjectionStateID(event))
	documentSource, _ = document["_source"].(map[string]interface{})
	if documentSource["event_id"] != event.EventID || documentSource["projection_version"] != float64(event.ProjectionVersion()) {
		t.Fatalf("older projection replaced current state: %v", documentSource)
	}

	strictRequest, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		address+"/"+writeAlias+"/_doc/strict-mapping-check",
		strings.NewReader(`{"unexpected_projection_field":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	strictRequest.Header.Set("Content-Type", "application/json")
	if username != "" {
		strictRequest.SetBasicAuth(username, password)
	}
	strictResponse, err := http.DefaultClient.Do(strictRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer strictResponse.Body.Close()
	strictBody, _ := io.ReadAll(io.LimitReader(strictResponse.Body, 1<<20))
	if strictResponse.StatusCode != http.StatusBadRequest || !strings.Contains(string(strictBody), "strict_dynamic_mapping_exception") {
		t.Fatalf("strict mapping accepted unknown field: status=%s body=%s", strictResponse.Status, strictBody)
	}
}

func TestNebulaCampaignProjectionEphemeralIntegration(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_ADDRESS"))
	if address == "" {
		t.Skip("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_ADDRESS is not set")
	}
	if os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP") != "ephemeral-only" {
		t.Fatal("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP must equal ephemeral-only")
	}
	storageAddress := strings.TrimSpace(os.Getenv("CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_STORAGE_ADDRESS"))
	graphHost, graphPort := requireEphemeralNebulaAddress(t, address)
	storageHost, storagePort := requireEphemeralNebulaAddress(t, storageAddress)
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
		[]nebula_go.HostAddress{{Host: graphHost, Port: graphPort}},
		poolConfig,
		nebula_go.DefaultLogger{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	session, err := pool.GetSession(username, password)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Release()

	hosts := requireNebulaStatement(t, session, "SHOW HOSTS;")
	hostTable := fmt.Sprint(hosts.AsStringTable())
	if !strings.Contains(hostTable, storageHost) || !strings.Contains(hostTable, strconv.Itoa(storagePort)) {
		requireNebulaStatement(
			t,
			session,
			fmt.Sprintf("ADD HOSTS %q:%d;", storageHost, storagePort),
		)
	}
	requireNebulaEventually(t, 30*time.Second, func() (*nebula_go.ResultSet, error) {
		result, executeErr := session.Execute("SHOW HOSTS;")
		if executeErr != nil || !result.IsSucceed() {
			return result, fmt.Errorf("show hosts: err=%v result=%v", executeErr, nebulaResultError(result))
		}
		if table := fmt.Sprint(result.AsStringTable()); !strings.Contains(table, storageHost) ||
			!strings.Contains(table, strconv.Itoa(storagePort)) || !strings.Contains(table, "ONLINE") {
			return result, fmt.Errorf("storage host is not online: %s", table)
		}
		return result, nil
	})

	const space = "campaign_projection_ephemeral"
	requireNebulaStatement(t, session,
		"CREATE SPACE IF NOT EXISTS "+space+"(partition_num=1, replica_factor=1, vid_type=FIXED_STRING(32));")
	requireNebulaEventually(t, 30*time.Second, func() (*nebula_go.ResultSet, error) {
		result, executeErr := session.Execute("USE " + space + ";")
		if executeErr != nil || !result.IsSucceed() {
			return result, fmt.Errorf("use space: err=%v result=%v", executeErr, nebulaResultError(result))
		}
		return result, nil
	})

	schemaStatements := []string{
		`CREATE TAG IF NOT EXISTS entity(tenant_id STRING NOT NULL, entity_id STRING NOT NULL, entity_type STRING NOT NULL, label STRING NOT NULL, detail STRING, risk_score INT64 DEFAULT 0, risk_level STRING, x DOUBLE DEFAULT 0.0, y DOUBLE DEFAULT 0.0, icon STRING, metadata_json STRING DEFAULT '{}', updated_at INT64);`,
		`CREATE EDGE IF NOT EXISTS relation(tenant_id STRING NOT NULL, relation_id STRING NOT NULL, source_id STRING NOT NULL, target_id STRING NOT NULL, relation_type STRING NOT NULL, risk_level STRING, evidence_id STRING, attributes_json STRING DEFAULT '{}', weight DOUBLE DEFAULT 1.0, observed_at INT64);`,
	}
	for _, statement := range schemaStatements {
		requireNebulaStatement(t, session, statement)
	}
	for _, statement := range []string{
		`CREATE TAG INDEX IF NOT EXISTS entity_tenant_idx ON entity(tenant_id(32));`,
		`CREATE TAG INDEX IF NOT EXISTS entity_id_idx ON entity(entity_id(32));`,
		`CREATE EDGE INDEX IF NOT EXISTS relation_tenant_idx ON relation(tenant_id(32));`,
		`CREATE EDGE INDEX IF NOT EXISTS relation_id_idx ON relation(relation_id(32));`,
	} {
		statement := statement
		requireNebulaEventually(t, 30*time.Second, func() (*nebula_go.ResultSet, error) {
			result, executeErr := session.Execute(statement)
			if executeErr != nil || !result.IsSucceed() {
				return result, fmt.Errorf("create index: err=%v result=%v", executeErr, nebulaResultError(result))
			}
			return result, nil
		})
	}

	var store *graphNebula.WorkbenchStore
	requireNebulaEventually(t, 30*time.Second, func() (*nebula_go.ResultSet, error) {
		candidate, storeErr := graphNebula.NewWorkbenchStore(graphConfig.NebulaConfig{
			Enabled:     true,
			Addresses:   []string{address},
			Username:    username,
			Password:    password,
			Space:       space,
			Timeout:     5 * time.Second,
			IdleTime:    time.Minute,
			MaxPoolSize: 2,
			MinPoolSize: 1,
		}, zap.NewNop())
		if storeErr != nil {
			return nil, storeErr
		}
		if readyErr := candidate.Ready(context.Background()); readyErr != nil {
			candidate.Close()
			return nil, readyErr
		}
		store = candidate
		return nil, nil
	})
	defer store.Close()
	target, err := NewNebulaCampaignProjection(store)
	if err != nil {
		t.Fatal(err)
	}

	aggregate := validCampaignAggregateProjectionEvent()
	aggregateProjection, err := target.Projection(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		requireNebulaEventually(t, 30*time.Second, func() (*nebula_go.ResultSet, error) {
			return nil, target.Apply(context.Background(), aggregate, aggregateProjection)
		})
	}
	campaignVID := ephemeralNebulaVID(aggregate.TenantID, aggregate.CampaignID)
	result := requireNebulaStatement(t, session, fmt.Sprintf(
		`FETCH PROP ON entity %q YIELD entity.tenant_id AS tenant_id, entity.entity_type AS entity_type, entity.metadata_json AS metadata_json;`,
		campaignVID,
	))
	if result.GetRowSize() != 1 {
		t.Fatalf("campaign projection row count=%d", result.GetRowSize())
	}
	campaignRow, err := result.GetRowValuesByIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	metadataValue, err := campaignRow.GetValueByColName("metadata_json")
	if err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := metadataValue.AsString()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadataJSON, `"projection_version":3`) ||
		!strings.Contains(metadataJSON, `"event_id":"`+aggregate.EventID+`"`) {
		t.Fatalf("campaign projection metadata=%s", metadataJSON)
	}

	linked := validCampaignMembershipProjectionEvent(true)
	linkedProjection, err := target.Projection(linked)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		requireNebulaEventually(t, 30*time.Second, func() (*nebula_go.ResultSet, error) {
			return nil, target.Apply(context.Background(), linked, linkedProjection)
		})
	}
	alertVID := ephemeralNebulaVID(linked.TenantID, linked.AlertID)
	result = requireNebulaStatement(t, session, fmt.Sprintf(
		`FETCH PROP ON relation %q->%q@0 YIELD relation.relation_id AS relation_id, relation.attributes_json AS attributes_json;`,
		campaignVID,
		alertVID,
	))
	if result.GetRowSize() != 1 {
		t.Fatalf("campaign relation row count=%d", result.GetRowSize())
	}
	relationRow, err := result.GetRowValuesByIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	attributesValue, err := relationRow.GetValueByColName("attributes_json")
	if err != nil {
		t.Fatal(err)
	}
	attributesJSON, err := attributesValue.AsString()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attributesJSON, `"relation_revision":4`) ||
		!strings.Contains(attributesJSON, `"campaign_revision":7`) {
		t.Fatalf("campaign relation attributes=%s", attributesJSON)
	}

	unlinked := validCampaignMembershipProjectionEvent(false)
	unlinkedProjection, err := target.Projection(unlinked)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), unlinked, unlinkedProjection); err != nil {
		t.Fatal(err)
	}
	result = requireNebulaStatement(t, session, fmt.Sprintf(
		`FETCH PROP ON relation %q->%q@0 YIELD relation.relation_id AS relation_id;`,
		campaignVID,
		alertVID,
	))
	if result.GetRowSize() != 0 {
		t.Fatalf("unlink left %d deterministic relation rows", result.GetRowSize())
	}
}

func requireEphemeralNebulaAddress(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("invalid ephemeral NebulaGraph address %q: %v", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Fatalf("ephemeral NebulaGraph must use a numeric loopback address: %q", address)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		t.Fatalf("invalid ephemeral NebulaGraph port: %q", address)
	}
	return host, port
}

func requireNebulaStatement(t *testing.T, session *nebula_go.Session, statement string) *nebula_go.ResultSet {
	t.Helper()
	result, err := session.Execute(statement)
	if err != nil {
		t.Fatalf("NebulaGraph statement %q: %v", statement, err)
	}
	if !result.IsSucceed() {
		t.Fatalf("NebulaGraph statement %q: %s", statement, nebulaResultError(result))
	}
	return result
}

func requireNebulaEventually(
	t *testing.T,
	timeout time.Duration,
	operation func() (*nebula_go.ResultSet, error),
) *nebula_go.ResultSet {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		result, err := operation()
		if err == nil {
			return result
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("NebulaGraph condition did not converge within %s: %v", timeout, lastErr)
	return nil
}

func nebulaResultError(result *nebula_go.ResultSet) string {
	if result == nil {
		return "nil result"
	}
	return fmt.Sprintf("code=%d message=%s", result.GetErrorCode(), result.GetErrorMsg())
}

func ephemeralNebulaVID(tenantID, entityID string) string {
	hash := md5.Sum([]byte(strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(entityID)))
	return hex.EncodeToString(hash[:])
}

func campaignProjectionAggregateEventAt(revision int64, eventID string) CampaignProjectionEvent {
	campaignID := "campaign-a"
	tenantID := "tenant-a"
	eventType := "traffic.campaign.v2.StatusChanged"
	traceID := fmt.Sprintf("trace-campaign-%d", revision)
	payload := fmt.Sprintf(`{"event_id":%q,"event_type":%q,"tenant_id":%q,"aggregate_type":"campaign","aggregate_id":%q,"aggregate_version":%d,"campaign_id":%q,"schema_version":2,"partition_key":%q,"trace_id":%q}`,
		eventID, eventType, tenantID, campaignID, revision, campaignID, tenantID+":"+campaignID, traceID)
	return CampaignProjectionEvent{
		Stream:            campaignAggregateStream,
		EventID:           eventID,
		TenantID:          tenantID,
		AggregateID:       campaignID,
		CampaignID:        campaignID,
		EventType:         eventType,
		SchemaVersion:     2,
		AggregateRevision: revision,
		PartitionKey:      tenantID + ":" + campaignID,
		TraceID:           traceID,
		Payload:           []byte(payload),
		ReceivedAt:        time.Date(2026, 8, 1, 1, 2, int(revision), 0, time.UTC),
	}
}

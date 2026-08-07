package consumer

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	nebula_go "github.com/vesoft-inc/nebula-go/v3"
	"go.uber.org/zap"

	graphConfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

// The alignment runner owns the three digest-pinned NebulaGraph containers and
// provides a numeric loopback graph endpoint. The storage identity is accepted
// only when it uses the runner's scoped container-name prefix.
func TestAssetProjectionRealNebulaDeterministicTenantVID(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("ASSET_PROJECTION_EPHEMERAL_NEBULA_ADDRESS"))
	if address == "" {
		t.Skip("ASSET_PROJECTION_EPHEMERAL_NEBULA_ADDRESS is not set")
	}
	if os.Getenv("ASSET_PROJECTION_EPHEMERAL_NEBULA_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing NebulaGraph endpoint without explicit ephemeral sentinel")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("refusing non-loopback NebulaGraph endpoint %q: %v", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		t.Fatalf("invalid ephemeral NebulaGraph port %q", portText)
	}
	storageHost := strings.TrimSpace(os.Getenv("ASSET_PROJECTION_EPHEMERAL_NEBULA_STORAGE_HOST"))
	if !strings.HasPrefix(storageHost, "codex-asset-projection-nebula-storage-") {
		t.Fatalf("refusing unscoped NebulaGraph storage host %q", storageHost)
	}
	storagePort, err := strconv.Atoi(os.Getenv("ASSET_PROJECTION_EPHEMERAL_NEBULA_STORAGE_PORT"))
	if err != nil || storagePort != 9779 {
		t.Fatalf("invalid ephemeral NebulaGraph storage port: %v", err)
	}

	poolConfig := nebula_go.GetDefaultConf()
	poolConfig.TimeOut = 5 * time.Second
	poolConfig.MaxConnPoolSize = 2
	poolConfig.MinConnPoolSize = 1
	var pool *nebula_go.ConnectionPool
	var session *nebula_go.Session
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		candidate, poolErr := nebula_go.NewConnectionPool(
			[]nebula_go.HostAddress{{Host: host, Port: port}}, poolConfig, nebula_go.DefaultLogger{},
		)
		if poolErr != nil {
			return poolErr
		}
		candidateSession, sessionErr := candidate.GetSession("root", "nebula")
		if sessionErr != nil {
			candidate.Close()
			return sessionErr
		}
		pool = candidate
		session = candidateSession
		return nil
	})
	defer pool.Close()
	defer session.Release()

	hosts := requireAssetNebulaStatement(t, session, "SHOW HOSTS;")
	if table := fmt.Sprint(hosts.AsStringTable()); !strings.Contains(table, storageHost) {
		requireAssetNebulaStatement(t, session, fmt.Sprintf("ADD HOSTS %q:%d;", storageHost, storagePort))
	}
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		result, executeErr := session.Execute("SHOW HOSTS;")
		if executeErr != nil || !result.IsSucceed() {
			return fmt.Errorf("show hosts: err=%v result=%v", executeErr, assetNebulaResultError(result))
		}
		table := fmt.Sprint(result.AsStringTable())
		if !strings.Contains(table, storageHost) || !strings.Contains(table, "ONLINE") {
			return fmt.Errorf("storage host is not online: %s", table)
		}
		return nil
	})

	const space = "asset_projection_ephemeral"
	requireAssetNebulaStatement(t, session,
		"CREATE SPACE IF NOT EXISTS "+space+"(partition_num=1, replica_factor=1, vid_type=FIXED_STRING(32));")
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		result, executeErr := session.Execute("USE " + space + ";")
		if executeErr != nil || !result.IsSucceed() {
			return fmt.Errorf("use space: err=%v result=%v", executeErr, assetNebulaResultError(result))
		}
		return nil
	})
	requireAssetNebulaStatement(t, session, `CREATE TAG IF NOT EXISTS entity(
		tenant_id STRING NOT NULL, entity_id STRING NOT NULL, entity_type STRING NOT NULL,
		label STRING NOT NULL, detail STRING, risk_score INT64 DEFAULT 0, risk_level STRING,
		x DOUBLE DEFAULT 0.0, y DOUBLE DEFAULT 0.0, icon STRING,
		metadata_json STRING DEFAULT '{}', updated_at INT64);`)
	requireAssetNebulaStatement(t, session, `CREATE EDGE IF NOT EXISTS relation(
		tenant_id STRING NOT NULL, relation_id STRING NOT NULL, source_id STRING NOT NULL,
		target_id STRING NOT NULL, relation_type STRING NOT NULL, risk_level STRING,
		evidence_id STRING, attributes_json STRING DEFAULT '{}',
		weight DOUBLE DEFAULT 1.0, observed_at INT64);`)
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		result, executeErr := session.Execute("DESC TAG entity;")
		if executeErr != nil || !result.IsSucceed() {
			return fmt.Errorf("entity schema is not visible: err=%v result=%v", executeErr, assetNebulaResultError(result))
		}
		result, executeErr = session.Execute("DESC EDGE relation;")
		if executeErr != nil || !result.IsSucceed() {
			return fmt.Errorf("relation schema is not visible: err=%v result=%v", executeErr, assetNebulaResultError(result))
		}
		return nil
	})

	var store *graphNebula.WorkbenchStore
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		candidate, storeErr := graphNebula.NewWorkbenchStore(graphConfig.NebulaConfig{
			Enabled: true, Addresses: []string{address}, Username: "root", Password: "nebula",
			Space: space, Timeout: 5 * time.Second, IdleTime: time.Minute,
			MaxPoolSize: 2, MinPoolSize: 1,
		}, zap.NewNop())
		if storeErr != nil {
			return storeErr
		}
		if readyErr := candidate.Ready(context.Background()); readyErr != nil {
			candidate.Close()
			return readyErr
		}
		store = candidate
		return nil
	})
	defer store.Close()
	target, err := NewNebulaAssetProjection(store)
	if err != nil {
		t.Fatal(err)
	}
	event := validAssetProjectionEvent()
	projection, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	requireAssetNebulaEventually(t, 45*time.Second, func() error {
		return target.Apply(context.Background(), event, projection)
	})
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatalf("apply deterministic replay: %v", err)
	}
	node, edges, truncated, err := store.LoadAssetProjection(context.Background(), event.TenantID, event.AssetID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if node.EntityID != event.AssetID || node.EntityType != "asset" || node.Label != event.Asset.Hostname || len(edges) != 0 || truncated {
		t.Fatalf("unexpected bounded projection node=%+v edges=%d truncated=%v", node, len(edges), truncated)
	}
	if fmt.Sprint(node.Metadata["event_id"]) != event.EventID || fmt.Sprint(node.Metadata["trace_id"]) != event.TraceID || fmt.Sprint(node.Metadata["revision"]) != "2" {
		t.Fatalf("missing event/revision watermark: %+v", node.Metadata)
	}

	other := event
	other.TenantID = "tenant-b"
	other.PartitionKey = other.TenantID + ":" + other.AssetID
	other.TraceID = "trace-asset-tenant-b"
	other.Asset.TenantID = other.TenantID
	other.Asset.Hostname = "server-tenant-b"
	otherProjection, err := target.Projection(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), other, otherProjection); err != nil {
		t.Fatal(err)
	}
	otherNode, _, _, err := store.LoadAssetProjection(context.Background(), other.TenantID, other.AssetID, 1)
	if err != nil {
		t.Fatal(err)
	}
	firstNode, _, _, err := store.LoadAssetProjection(context.Background(), event.TenantID, event.AssetID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if otherNode.Label != "server-tenant-b" || firstNode.Label != event.Asset.Hostname {
		t.Fatalf("tenant VID isolation failed first=%q other=%q", firstNode.Label, otherNode.Label)
	}
}

func requireAssetNebulaStatement(t *testing.T, session *nebula_go.Session, statement string) *nebula_go.ResultSet {
	t.Helper()
	result, err := session.Execute(statement)
	if err != nil {
		t.Fatalf("NebulaGraph statement %q: %v", statement, err)
	}
	if !result.IsSucceed() {
		t.Fatalf("NebulaGraph statement %q: %s", statement, assetNebulaResultError(result))
	}
	return result
}

func requireAssetNebulaEventually(t *testing.T, timeout time.Duration, operation func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := operation(); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("NebulaGraph condition did not converge within %s: %v", timeout, lastErr)
}

func assetNebulaResultError(result *nebula_go.ResultSet) string {
	if result == nil {
		return "nil result"
	}
	return fmt.Sprintf("code=%d message=%s", result.GetErrorCode(), result.GetErrorMsg())
}

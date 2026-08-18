package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/attackchain"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type fakeAttackChainSnapshotStore struct {
	items []attackchain.Snapshot
	err   error
}

func (store fakeAttackChainSnapshotStore) LoadCurrent(_ context.Context, tenantID, chainID string) (attackchain.Snapshot, error) {
	for _, item := range store.items {
		if item.TenantID == tenantID && item.ChainID == chainID {
			return item, nil
		}
	}
	return attackchain.Snapshot{}, sql.ErrNoRows
}

func (store fakeAttackChainSnapshotStore) ListCurrent(_ context.Context, tenantID string, _, _ int) ([]attackchain.Snapshot, int, error) {
	if store.err != nil {
		return nil, 0, store.err
	}
	items := make([]attackchain.Snapshot, 0)
	for _, item := range store.items {
		if item.TenantID == tenantID {
			items = append(items, item)
		}
	}
	return items, len(items), nil
}

func TestVersionedAttackChainListUsesTenantSnapshotAndContractMeta(t *testing.T) {
	item := attackchain.Snapshot{
		SnapshotID: "snapshot-1", TenantID: "tenant-a", ChainID: "chain-1", Version: 2,
		AsOf: time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC), Partial: true,
		PartialReasons: []string{"path:candidate-1"},
		GraphSnapshot:  attackchain.GraphSnapshot{SourceWatermarks: map[string]string{"clickhouse": "window:42"}},
	}
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAttackChainV1Runtime(true, fakeAttackChainSnapshotStore{items: []attackchain.Snapshot{item}})
	request := httptest.NewRequest(http.MethodGet, "/attack-chains", nil)
	request = request.WithContext(attackChainRequestContext(request.Context(), "tenant-a", true))
	response := httptest.NewRecorder()
	handler.ListAttackChains(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, token := range []string{`"snapshot_id":"snapshot-1"`, `"chain_id":"chain-1"`, `"partial":true`, `"chain-1:clickhouse":"window:42"`} {
		if !strings.Contains(body, token) {
			t.Fatalf("versioned attack-chain response lacks %s: %s", token, body)
		}
	}
}

func TestVersionedAttackChainRequiresGraphReadAndKeepsTenantIsolation(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAttackChainV1Runtime(true, fakeAttackChainSnapshotStore{items: []attackchain.Snapshot{{
		SnapshotID: "tenant-b-snapshot", TenantID: "tenant-b", ChainID: "chain-1",
	}}})
	request := httptest.NewRequest(http.MethodGet, "/attack-chains", nil)
	request = request.WithContext(attackChainRequestContext(request.Context(), "tenant-a", false))
	response := httptest.NewRecorder()
	handler.ListAttackChains(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing graph:read unexpectedly succeeded: %s", response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/attack-chains/chain-1", nil)
	request = mux.SetURLVars(request, map[string]string{"id": "chain-1"})
	request = request.WithContext(attackChainRequestContext(request.Context(), "tenant-a", true))
	response = httptest.NewRecorder()
	handler.GetAttackChain(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant snapshot leaked: %d %s", response.Code, response.Body.String())
	}
}

func TestVersionedAttackChainSubresourcesUseOneImmutableSnapshot(t *testing.T) {
	snapshot := versionedAttackChainAPISnapshot()
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAttackChainV1Runtime(true, fakeAttackChainSnapshotStore{items: []attackchain.Snapshot{snapshot}})
	tests := []struct {
		path        string
		handler     http.HandlerFunc
		mustHave    []string
		mustNotHave []string
	}{
		{
			path: "/attack-chains/chain-1/phases", handler: handler.GetAttackChainPhases,
			mustHave: []string{`"stage":"initial_access"`, `"edge_ids":["edge-analyst","edge-observed"]`, `"stage":"execution"`, `"edge_ids":["edge-rule"]`, `"snapshot_id":"snapshot-views-1"`},
		},
		{
			path: "/attack-chains/chain-1/paths?phase=execution", handler: handler.ListAttackChainPaths,
			mustHave:    []string{`"path_id":"candidate-1"`, `"total":1`, `"snapshot_id":"snapshot-views-1"`},
			mustNotHave: []string{`"path_id":"alternative-1"`},
		},
		{
			path: "/attack-chains/chain-1/evidence?type=rule_model", handler: handler.ListAttackChainEvidence,
			mustHave:    []string{`"evidence_id":"rule-evidence"`, `"path_ids":["candidate-1"]`, `"snapshot_id":"snapshot-views-1"`},
			mustNotHave: []string{`"evidence_id":"event-evidence"`},
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request = mux.SetURLVars(request, map[string]string{"id": "chain-1"})
			request = request.WithContext(attackChainRequestContext(request.Context(), "tenant-a", true))
			response := httptest.NewRecorder()
			test.handler(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
			}
			body := response.Body.String()
			for _, token := range test.mustHave {
				if !strings.Contains(body, token) {
					t.Fatalf("response lacks %s: %s", token, body)
				}
			}
			for _, token := range test.mustNotHave {
				if strings.Contains(body, token) {
					t.Fatalf("response unexpectedly contains %s: %s", token, body)
				}
			}
		})
	}
}

func TestVersionedAttackChainSubresourcesRequireGraphRead(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAttackChainV1Runtime(true, fakeAttackChainSnapshotStore{items: []attackchain.Snapshot{versionedAttackChainAPISnapshot()}})
	request := httptest.NewRequest(http.MethodGet, "/attack-chains/chain-1/paths", nil)
	request = mux.SetURLVars(request, map[string]string{"id": "chain-1"})
	request = request.WithContext(attackChainRequestContext(request.Context(), "tenant-a", false))
	response := httptest.NewRecorder()
	handler.ListAttackChainPaths(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("snapshot paths without graph:read unexpectedly succeeded: %s", response.Body.String())
	}
}

func TestVersionedAttackChainRecommendationsStayEmptyAndFailVisible(t *testing.T) {
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAttackChainV1Runtime(true, fakeAttackChainSnapshotStore{items: []attackchain.Snapshot{versionedAttackChainAPISnapshot()}})
	request := httptest.NewRequest(http.MethodGet, "/attack-chains/chain-1/recommendations?category=block", nil)
	request = mux.SetURLVars(request, map[string]string{"id": "chain-1"})
	request = request.WithContext(attackChainRequestContext(request.Context(), "tenant-a", true))
	response := httptest.NewRecorder()
	handler.ListAttackChainRecommendations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	for _, token := range []string{`"items":[]`, `"total":0`, `"result_code":"PARTIAL"`, `"chain-1:recommendations_not_in_snapshot"`} {
		if !strings.Contains(response.Body.String(), token) {
			t.Fatalf("versioned recommendations response lacks %s: %s", token, response.Body.String())
		}
	}
}

func TestAttackChainEmptySetMetaUsesCurrentTime(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	meta := attackChainContractMeta(context.Background(), "tenant-a", nil)
	asOf, err := time.Parse(time.RFC3339Nano, meta.AsOf)
	if err != nil || asOf.Before(before) {
		t.Fatalf("empty-set metadata emitted a zero or invalid as_of: %q %v", meta.AsOf, err)
	}
}

func versionedAttackChainAPISnapshot() attackchain.Snapshot {
	identity := func(id string) attackchain.Identity {
		return attackchain.Identity{TenantID: "tenant-a", EntityType: "asset", CanonicalID: id, VertexID: id + "-vid"}
	}
	source, middle, target := identity("source"), identity("middle"), identity("target")
	evidence := func(id, kind string) attackchain.EvidenceAnchor {
		return attackchain.EvidenceAnchor{
			TenantID: "tenant-a", EvidenceID: id, Kind: kind, ImmutableURI: "minio://evidence/" + id,
			SHA256: strings.Repeat("a", 64), SourceEventID: "event-" + id, OccurredAt: 1, Available: true,
		}
	}
	return attackchain.Snapshot{
		SnapshotID: "snapshot-views-1", TenantID: "tenant-a", ChainID: "chain-1", Version: 1,
		AsOf: time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC), Stages: []string{"initial_access", "execution"},
		CandidatePath: attackchain.Path{PathID: "candidate-1", Kind: "candidate", Edges: []attackchain.Edge{
			{EdgeID: "edge-observed", Stage: "initial_access", Source: source, Target: middle, Provenance: "observed", Evidence: []attackchain.EvidenceAnchor{evidence("event-evidence", "event")}},
			{EdgeID: "edge-rule", Stage: "execution", Source: middle, Target: target, Provenance: "derived", Evidence: []attackchain.EvidenceAnchor{evidence("rule-evidence", "rule")}},
		}},
		AlternativePaths: []attackchain.Path{{PathID: "alternative-1", Kind: "alternative", Edges: []attackchain.Edge{
			{EdgeID: "edge-analyst", Stage: "initial_access", Source: source, Target: target, Provenance: "analyst", Evidence: []attackchain.EvidenceAnchor{evidence("analyst-evidence", "analyst_conclusion")}},
		}}},
		GraphSnapshot: attackchain.GraphSnapshot{SourceWatermarks: map[string]string{"clickhouse": "42", "nebulagraph": "43", "postgresql": "44"}},
	}
}

func attackChainRequestContext(ctx context.Context, tenantID string, graphRead bool) context.Context {
	permissions := []string{authmodel.ScopeCampaignRead}
	if graphRead {
		permissions = append(permissions, authmodel.ScopeGraphRead)
	}
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	return context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
}

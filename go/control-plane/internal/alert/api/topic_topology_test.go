package api

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildTopicSnapshotDataKeepsTopologyContractWhenClickHouseUnavailable(t *testing.T) {
	handler := &SystemHandler{}
	scope := defaultTopicScope("tenant-a", "exfil")

	for _, topic := range []string{"exfil", "apt"} {
		built := handler.buildTopicSnapshotData(context.Background(), "tenant-a", topic, scope, 1, 2, time.UnixMilli(2).UTC())
		if !built.Partial {
			t.Fatalf("%s snapshot must be partial without ClickHouse", topic)
		}
		if nodes, ok := built.Data["topology_nodes"].([]topicTopologyNodeDTO); !ok || nodes == nil || len(nodes) != 0 {
			t.Fatalf("%s topology_nodes must be an explicit empty array, got %#v", topic, built.Data["topology_nodes"])
		}
		if links, ok := built.Data["topology_links"].([]topicTopologyLinkDTO); !ok || links == nil || len(links) != 0 {
			t.Fatalf("%s topology_links must be an explicit empty array, got %#v", topic, built.Data["topology_links"])
		}
	}
}

func TestBuildExfiltrationTopicTopologyIsDeterministicBoundedAndConnected(t *testing.T) {
	paths := make([]encryptedExfiltrationPathDTO, 0, 24)
	for index := 0; index < 24; index++ {
		paths = append(paths, encryptedExfiltrationPathDTO{
			SrcIP:        "10.0.0." + string(rune('1'+index)),
			DstIP:        "203.0.113." + string(rune('1'+index)),
			Protocol:     []string{"TLS", "SSH", "QUIC", "DNS"}[index%4],
			SessionCount: int64(index + 1),
			UploadBytes:  uint64(index+1) * 1024,
			Risk:         []string{"low", "medium", "high"}[index%3],
		})
	}

	nodes, links := buildExfiltrationTopicTopology(paths)
	repeatedNodes, repeatedLinks := buildExfiltrationTopicTopology(paths)
	if !reflect.DeepEqual(nodes, repeatedNodes) || !reflect.DeepEqual(links, repeatedLinks) {
		t.Fatal("topology must be deterministic")
	}
	if len(nodes) > exfilTopicTopologyMaxSources+exfilTopicTopologyMaxProtocols+exfilTopicTopologyMaxDestinations {
		t.Fatalf("node bound exceeded: %d", len(nodes))
	}
	if len(links) > exfilTopicTopologyMaxPaths*2 {
		t.Fatalf("link bound exceeded: %d", len(links))
	}
	assertTopicTopologyConnected(t, nodes, links)
}

func TestBuildAPTTopicTopologyUsesStablePhaseAndEvidenceRelations(t *testing.T) {
	campaigns := []campaignDTO{
		{CampaignID: "campaign-a", Score: 0.91, AttackPhases: []string{"initial_access", "lateral_movement"}, Entities: []string{"asset-a", "asset-b"}, Alerts: []string{"alert-a"}},
		{CampaignID: "campaign-b", Score: 0.64, AttackPhases: []string{"persistence", "exfiltration"}, Entities: []string{"asset-c"}, Alerts: []string{"alert-b"}},
	}

	nodes, links := buildAPTTopicTopology(campaigns)
	repeatedNodes, repeatedLinks := buildAPTTopicTopology(campaigns)
	if !reflect.DeepEqual(nodes, repeatedNodes) || !reflect.DeepEqual(links, repeatedLinks) {
		t.Fatal("topology must be deterministic")
	}
	if len(nodes) > aptTopicTopologyMaxCampaigns+aptTopicTopologyMaxPhases+aptTopicTopologyMaxEntities+aptTopicTopologyMaxEvidence {
		t.Fatalf("node bound exceeded: %d", len(nodes))
	}
	var lateralRelation, evidenceRelation bool
	for _, link := range links {
		if link.Target == "phase-lateral" {
			lateralRelation = true
		}
		if strings.HasPrefix(link.Target, "evidence-") {
			evidenceRelation = true
		}
	}
	if !lateralRelation || !evidenceRelation {
		t.Fatalf("expected lateral and evidence relations, lateral=%v evidence=%v", lateralRelation, evidenceRelation)
	}
	assertTopicTopologyConnected(t, nodes, links)
}

func assertTopicTopologyConnected(t *testing.T, nodes []topicTopologyNodeDTO, links []topicTopologyLinkDTO) {
	t.Helper()
	ids := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if node.ID == "" || node.Label == "" {
			t.Fatalf("invalid node: %+v", node)
		}
		if node.X < 0 || node.X > 100 || node.Y < 0 || node.Y > 100 {
			t.Fatalf("node outside percentage canvas: %+v", node)
		}
		if _, exists := ids[node.ID]; exists {
			t.Fatalf("duplicate node id: %s", node.ID)
		}
		ids[node.ID] = struct{}{}
	}
	for _, link := range links {
		if _, ok := ids[link.Source]; !ok {
			t.Fatalf("dangling source: %s", link.Source)
		}
		if _, ok := ids[link.Target]; !ok {
			t.Fatalf("dangling target: %s", link.Target)
		}
		if link.Source == link.Target {
			t.Fatalf("self link: %+v", link)
		}
	}
}

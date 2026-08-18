package api

import (
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

func TestWorkbenchContinuationIsTenantQueryAndExpiryBound(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := workbenchContinuation{
		Version: workbenchContinuationVersion, TenantID: "tenant-a", Fingerprint: "query-a",
		NodeLimit: 200, PageSize: 100, SinceMS: 1000, UntilMS: 2000, ExpiresAt: now.Add(time.Minute).Unix(),
	}
	token, err := encodeWorkbenchContinuation(payload, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWorkbenchContinuation(token, "tenant-a", "query-a", "0123456789abcdef0123456789abcdef", now)
	if err != nil || decoded.NodeLimit != 200 {
		t.Fatalf("valid continuation rejected: payload=%+v err=%v", decoded, err)
	}
	for name, test := range map[string]struct {
		tenant, fingerprint, candidate string
		at                             time.Time
	}{
		"cross tenant": {"tenant-b", "query-a", token, now},
		"filter drift": {"tenant-a", "query-b", token, now},
		"tampered":     {"tenant-a", "query-a", token + "x", now},
		"expired":      {"tenant-a", "query-a", token, now.Add(2 * time.Minute)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeWorkbenchContinuation(test.candidate, test.tenant, test.fingerprint, "0123456789abcdef0123456789abcdef", test.at); err == nil {
				t.Fatal("invalid continuation was accepted")
			}
		})
	}
}

func TestGovernWorkbenchGraphRedactsEvidenceAndSecretsWithoutMutation(t *testing.T) {
	graph := &query.WorkbenchGraph{
		CenterID: "host:a",
		Nodes: []*query.WorkbenchNode{
			{EntityID: "host:a", EntityType: "host", Detail: "host", Metadata: map[string]interface{}{"site": "main", "password": "do-not-leak"}},
			{EntityID: "evidence:e1", EntityType: "evidence", Detail: "s3://bucket/object", Metadata: map[string]interface{}{"object_key": "secret-key"}},
		},
		Edges: []*query.WorkbenchEdge{{RelationID: "r1", SourceID: "host:a", TargetID: "evidence:e1", EvidenceID: "e1", Attributes: map[string]interface{}{"raw_payload": "secret", "confidence": 0.9}}},
	}
	redacted, fields := governWorkbenchGraph(graph, []string{"graph:read"})
	if len(fields) < 4 || redacted.Nodes[0].Metadata["password"] != nil || redacted.Nodes[1].Detail == graph.Nodes[1].Detail ||
		redacted.Edges[0].EvidenceID != "" || redacted.Edges[0].Attributes["raw_payload"] != nil {
		t.Fatalf("evidence/secret redaction incomplete: fields=%v graph=%+v", fields, redacted)
	}
	if graph.Nodes[0].Metadata["password"] != "do-not-leak" || graph.Edges[0].EvidenceID != "e1" {
		t.Fatal("redaction mutated repository-owned graph")
	}
	privileged, _ := governWorkbenchGraph(graph, []string{"graph:read", "evidence:read"})
	if privileged.Edges[0].EvidenceID != "e1" || privileged.Nodes[1].Metadata["object_key"] != "secret-key" {
		t.Fatal("evidence:read did not preserve evidence fields")
	}
	if privileged.Nodes[0].Metadata["password"] != nil {
		t.Fatal("secret-like fields must never be returned")
	}
}

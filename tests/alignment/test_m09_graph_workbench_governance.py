import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def test_nebulagraph_traversal_is_bounded_before_decode():
    source = read("go/control-plane/internal/graph/nebula/workbench_store.go")
    assert "LoadWorkbenchGraphBounded" in source
    assert "relation.tenant_id == $tenant_id" in source
    assert "ORDER BY $-.relation_id | LIMIT %d" in source
    assert "visited := map[string]bool" in source
    assert 'reason = "hop_neighbor_budget"' in source


def test_continuation_and_redaction_are_server_governed_and_default_off():
    handler = read("go/control-plane/internal/graph/api/handler.go")
    governance = read("go/control-plane/internal/graph/api/workbench_governance.go")
    config = read("go/control-plane/internal/graph/config/config.go")
    manifest = read("deployments/kubernetes/applications/go-services.yaml")
    assert "decodeWorkbenchContinuation" in handler
    assert "governWorkbenchGraph" in handler
    assert "hmac.Equal" in governance
    assert "payload.TenantID != tenantID" in governance
    assert "evidence:read required" in governance
    assert '`env:"V2_ENABLED" envDefault:"false"`' in config
    assert 'name: GRAPH_WORKBENCH_V2_ENABLED, value: "false"' in manifest
    assert "GRAPH_WORKBENCH_CONTINUATION_SECRET" in manifest


def test_ui_uses_continuation_and_server_saved_view_without_fabrication():
    page = read("web/ui/src/pages/GraphEntityPage.tsx")
    client = read("web/ui/src/services/api.ts")
    assert "加载下一有界页" in page
    assert "页面不会补点或补线" in page
    assert "submitAlertTriageAction" in page
    assert "localStorage.setItem('traffic-graph-saved-view'" not in page
    assert "next_continuation" in client
    assert "redacted_fields" in client


def test_openapi_and_k8s_evidence_contract_cover_negative_boundaries():
    openapi = json.loads(read("contracts/openapi/alignment-v1.openapi.json"))
    for path in ("/v1/graph/workbench", "/v1/graph/workbench/path"):
        operation = openapi["paths"][path]["get"]
        assert operation["x-required-scope"] == "graph:read"
        assert operation["x-rollout-flag"] == "GRAPH_WORKBENCH_V2_ENABLED"
    runner = read("scripts/alignment/run_m09_graph_workbench_governance_k8s.py")
    assert "shared-nebulagraph-touched" in runner
    assert "TestGovernedWorkbenchNebulaK8sCleanupOracle" in runner
    evidence_path = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n014/k8s-graph-workbench-governance-latest.json"
    if evidence_path.exists():
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        assert evidence["status"] == "PASS"
        assert evidence["supernode_truncation_explicit"] is True
        assert evidence["cycle_visited_set_exact"] is True
        assert evidence["cross_tenant_edge_rejected"] is True
        assert evidence["run_scoped_nebulagraph_rows_removed"] is True
        assert evidence["production_applied"] is False

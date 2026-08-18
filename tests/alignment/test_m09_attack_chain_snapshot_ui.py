import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def source(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_attack_chain_snapshot_runtime_is_default_off_and_schema_checked():
    main = source("go/control-plane/cmd/alert-service/main.go")
    deployment = source("deployments/kubernetes/applications/go-services.yaml")
    repository = source("go/control-plane/internal/alert/attackchain/repository.go")
    migration = source("deployments/postgres/migrations/202608142100_m07_attack_chain_snapshot_v1.sql")
    assert 'getBoolEnv("ATTACK_CHAIN_V1_ENABLED", false)' in main
    assert '{name: ATTACK_CHAIN_V1_ENABLED, value: "false"}' in deployment
    assert "attackChainRepository.VerifySchema" in main
    for table in (
        "gnn_graph_snapshots_v1",
        "attack_chain_snapshots_v1",
        "attack_chain_snapshot_current_v1",
        "attack_chain_evidence_manifest_v1",
    ):
        assert f"CREATE TABLE IF NOT EXISTS {table}" in migration
        assert table in repository


def test_snapshot_api_keeps_one_snapshot_and_does_not_invent_recommendations():
    handler = source("go/control-plane/internal/alert/api/handler_system.go")
    views = source("go/control-plane/internal/alert/api/attack_chain_snapshot_views.go")
    for marker in (
        "attackChainSnapshotPhases(snapshot)",
        "attackChainSnapshotEvidence(snapshot",
        "attackChainSnapshotPaths(snapshot",
        "attackChainContractMeta(ctx, snapshot.TenantID",
    ):
        assert marker in handler
    assert "recommendations_not_in_snapshot" in handler
    assert '"items": []interface{}{}' in handler
    assert "PathIDs" in views
    assert "EdgeIDs" in views
    assert "EvidenceIDs" in views


def test_typed_ui_adapter_and_page_expose_all_snapshot_truth_classes():
    client = source("web/ui/src/services/attackChainApi.ts")
    page = source("web/ui/src/pages/AttackChainAnalysisPage.tsx")
    for marker in (
        "normalizeAttackChainDetail",
        "candidate_path",
        "alternative_paths",
        "provenance",
        "uncertainty",
        "continuation_boundary",
        "source_watermarks",
    ):
        assert marker in client
    for marker in (
        "M07 快照事实边界",
        "source",
        "target",
        "observed",
        "derived",
        "analyst",
        "替代路径",
        "路径结果已按预算截断，页面不会补线",
        "溯源结论",
        "onInspectEvidence",
    ):
        assert marker in page
    assert "不得从 derived 边推断人工结论" in page


def test_k8s_snapshot_and_bundle_evidence_is_immutable_and_run_scoped():
    evidence = json.loads(
        source(
            "doc/02_acceptance/topic1/tasks/t1-m09-n013/"
            "k8s-attack-chain-snapshot-ui-latest.json"
        )
    )
    assert evidence["artifact_kind"] == "M09_ATTACK_CHAIN_SNAPSHOT_UI_TEST_RESULT"
    assert evidence["task_id"] == "T1-M09-N013"
    assert evidence["status"] == "PASS"
    assert evidence["postgres_oracle"] == {
        "current_rows": 1,
        "evidence_anchors": 3,
        "graph_snapshots": 1,
        "migration": 1,
        "sentinel": "ephemeral-only",
        "snapshots": 1,
    }
    assert evidence["provenance_classes"] == ["observed", "derived", "analyst"]
    assert evidence["fabricated_recommendations"] is False
    assert evidence["run_scoped_resources_removed"] is True
    assert evidence["shared_postgres_touched"] is False
    assert evidence["production_applied"] is False
    assert evidence["mock_enabled"] is False
    assert evidence["kubernetes_dependency"]["image_id"]
    assert len(evidence["kubernetes_jobs"]) == 3
    assert all(item["pod_uid"] and item["image_id"] for item in evidence["kubernetes_jobs"])


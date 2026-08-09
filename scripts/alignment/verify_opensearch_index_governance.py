#!/usr/bin/env python3
"""Verify the repository-side T-OS-002 versioned index and alias candidate."""

from __future__ import annotations

import json
import hashlib
import re
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/opensearch/index-governance.v1.json")
CATALOG = Path("common/opensearch/index-templates.json")
CONFIG_DIR = Path("common/opensearch/alerts-v2")
DEFAULT_K8S = Path("deployments/kubernetes/init-jobs/04-opensearch-templates.yaml")
MIGRATION_K8S = Path("deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml")
EXPAND_RENDERER = Path("scripts/alignment/render_opensearch_alerts_v2_expand.py")
BACKFILL_PLANNER = Path("scripts/alignment/plan_opensearch_alerts_v2_backfill.py")
BACKFILL_RENDERER = Path("scripts/alignment/render_opensearch_alerts_v2_backfill.py")
GO_CONFIG = Path("go/control-plane/internal/alert/config/config.go")
GO_WRITER = Path("go/control-plane/internal/alert/persistence/opensearch.go")
GO_MODEL = Path("go/control-plane/internal/alert/persistence/alert.go")
GO_REPOSITORY = Path("go/control-plane/internal/alert/repository/opensearch.go")
GO_DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
FLINK_JOB = Path("java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/AlertGeneratorJob.java")
FLINK_SINK = Path("java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactory.java")
FLINK_PROPERTIES = Path("java/flink-jobs/flink-alert-generator-job/src/main/resources/alert-generator-job.properties")

PAYLOADS = {
    "mappings-component.json": CONFIG_DIR / "mappings-component.json",
    "settings-component.json": CONFIG_DIR / "settings-component.json",
    "index-template.json": CONFIG_DIR / "index-template.json",
    "ism-policy.json": CONFIG_DIR / "ism-policy.json",
    "bootstrap-index.json": CONFIG_DIR / "bootstrap-index.json",
}

REQUIRED_TYPES = {
    "tenant_id": "keyword",
    "alert_id": "keyword",
    "event_id": "keyword",
    "trace_id": "keyword",
    "src_ip": "ip",
    "dst_ip": "ip",
    "src_port": "integer",
    "dst_port": "integer",
    "protocol": "short",
    "alert_type": "keyword",
    "severity": "keyword",
    "score": "float",
    "status": "keyword",
    "first_seen": "date",
    "last_seen": "date",
    "count": "integer",
    "state_version": "long",
    "search_text": "text",
}


def load_json(root: Path, path: Path) -> dict[str, Any]:
    return json.loads((root / path).read_text(encoding="utf-8"))


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required = [CONTRACT, CATALOG, DEFAULT_K8S, MIGRATION_K8S, EXPAND_RENDERER,
                BACKFILL_PLANNER, BACKFILL_RENDERER,
                GO_CONFIG, GO_WRITER, GO_MODEL,
                GO_REPOSITORY, GO_DEPLOYMENT, FLINK_JOB, FLINK_SINK,
                FLINK_PROPERTIES, *PAYLOADS.values()]
    missing = [path.as_posix() for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = load_json(root, CONTRACT)
    if contract.get("remediation_id") != "T-OS-002":
        errors.append("contract remediation_id must be T-OS-002")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial repository slice must not claim closure")
    if contract.get("coverage_status") != "PARTIAL":
        errors.append("coverage_status must remain PARTIAL before live migration")
    if contract.get("production_applied") is not False:
        errors.append("repository candidate must not claim production apply")
    if not contract.get("closure_blockers"):
        errors.append("closure blockers must remain explicit")

    catalog = load_json(root, CATALOG)
    if catalog.get("kind") != "OpenSearchTemplateSourceCatalog":
        errors.append("common OpenSearch catalog must be valid versioned JSON")
    if catalog.get("legacy_multi_document_payload_removed") is not True:
        errors.append("legacy multi-document pseudo JSON must remain removed")

    payloads = {name: load_json(root, path) for name, path in PAYLOADS.items()}
    mappings = payloads["mappings-component.json"]
    mapping_body = mappings.get("template", {}).get("mappings", {})
    properties = mapping_body.get("properties", {})
    if mapping_body.get("dynamic") != "strict":
        errors.append("alerts v2 mapping must be dynamic=strict")
    for field, expected_type in REQUIRED_TYPES.items():
        actual_type = properties.get(field, {}).get("type")
        if actual_type != expected_type:
            errors.append(f"mapping type mismatch for {field}: {actual_type!r}")
    for field in ("src_ip", "dst_ip"):
        if properties.get(field, {}).get("ignore_malformed") is not True:
            errors.append(f"{field} must tolerate legacy empty/invalid values during shadow migration")
    for field in ("tenant_id", "alert_id", "severity", "status"):
        if properties.get(field, {}).get("type") == "text":
            errors.append(f"high-cardinality/filter field must not be text: {field}")

    settings = payloads["settings-component.json"].get("template", {}).get("settings", {})
    if settings.get("plugins.index_state_management.rollover_alias") != contract.get("write_alias"):
        errors.append("settings component rollover alias differs from contract")
    template = payloads["index-template.json"]
    if template.get("version") != 2 or template.get("priority") != 300:
        errors.append("index template must carry version=2 and priority=300")
    if template.get("composed_of") != contract.get("component_templates"):
        errors.append("index template component composition differs from contract")
    bootstrap = payloads["bootstrap-index.json"].get("aliases", {})
    if contract.get("read_alias") not in bootstrap:
        errors.append("bootstrap read alias missing")
    if bootstrap.get(contract.get("write_alias"), {}).get("is_write_index") is not True:
        errors.append("bootstrap write alias must be the write index")
    policy = payloads["ism-policy.json"].get("policy", {})
    if policy.get("schema_version") != 2:
        errors.append("ISM policy schema version must be 2")
    if policy.get("ism_template", {}).get("index_patterns") != ["alerts-v2-*"]:
        errors.append("ISM template must target only alerts-v2 physical indexes")

    manifests = list(yaml.safe_load_all((root / MIGRATION_K8S).read_text(encoding="utf-8")))
    configmaps = [item for item in manifests if item and item.get("kind") == "ConfigMap"
                  and item.get("metadata", {}).get("name") == "opensearch-alerts-v2-contract"]
    embedded: dict[str, str] = {}
    if len(configmaps) != 1:
        errors.append("exactly one alerts v2 contract ConfigMap is required")
    else:
        embedded = configmaps[0].get("data", {})
        for name, expected in payloads.items():
            try:
                actual = json.loads(embedded.get(name, ""))
            except json.JSONDecodeError as exc:
                errors.append(f"embedded {name} is not JSON: {exc}")
                continue
            if actual != expected:
                errors.append(f"embedded {name} drifts from common source payload")

    expand = contract.get("expand_execution", {})
    approval_bindings = {
        "approval_id", "approved_by", "approval_nonce", "not_before_epoch",
        "expires_at_epoch", "cluster_uuid", "g0_candidate_sha256", "g0_manifest_sha256",
        "contract_sha256",
    }
    if expand.get("manifest") != MIGRATION_K8S.as_posix() or expand.get("default_suspend") is not True:
        errors.append("expand execution manifest or default suspend contract drifted")
    if expand.get("approval_secret") != "opensearch-alerts-v2-approval":
        errors.append("expand approval Secret contract drifted")
    if expand.get("renderer") != EXPAND_RENDERER.as_posix():
        errors.append("expand renderer contract drifted")
    if expand.get("rendered_secret_immutable") is not True:
        errors.append("rendered approval Secret must remain immutable")
    if expand.get("max_approval_window_seconds") != 14_400:
        errors.append("expand approval window must remain bounded to four hours")
    if set(expand.get("required_approval_bindings", [])) != approval_bindings:
        errors.append("expand approval bindings are incomplete")
    if embedded:
        order = ["mappings-component.json", "settings-component.json", "index-template.json",
                 "ism-policy.json", "bootstrap-index.json"]
        digest = hashlib.sha256("".join(embedded[name] for name in order).encode()).hexdigest()
        if expand.get("contract_sha256") != digest:
            errors.append("expand contract SHA-256 does not bind the embedded payloads")

    default_k8s_text = (root / DEFAULT_K8S).read_text(encoding="utf-8")
    for forbidden in ("opensearch-alerts-v2-contract", "traffic-alerts-mappings-v2", "alerts-v2-000001"):
        if forbidden in default_k8s_text:
            errors.append(f"routine init path must not execute alerts v2 expand: {forbidden}")

    k8s_text = (root / MIGRATION_K8S).read_text(encoding="utf-8")
    for token in (
        "_component_template/traffic-alerts-mappings-v2",
        "_component_template/traffic-alerts-settings-v2",
        "_index_template/traffic-alerts-v2-template",
        "_index_template/_simulate_index/alerts-v2-000001",
        "_plugins/_ism/policies/traffic-alerts-hot-delete-v2",
        "_plugins/_ism/explain/alerts-v2-000001",
        "alerts-v2-read",
        "alerts-v2-write",
    ):
        if token not in k8s_text:
            errors.append(f"Kubernetes candidate missing token: {token}")
    jobs = [item for item in manifests if item and item.get("kind") == "Job"
            and item.get("metadata", {}).get("name") == "expand-opensearch-alerts-v2"]
    if len(jobs) != 1:
        errors.append("exactly one dedicated alerts v2 expand Job is required")
    else:
        spec = jobs[0].get("spec", {})
        if spec.get("suspend") is not True or spec.get("backoffLimit") != 0:
            errors.append("alerts v2 expand Job must be suspended and non-retrying by default")
        container_text = json.dumps(spec.get("template", {}).get("spec", {}).get("containers", []), ensure_ascii=False)
        for token in ("opensearch-alerts-v2-approval", "APPROVED_CLUSTER_UUID",
                      "APPROVED_G0_CANDIDATE_SHA256", "APPROVED_G0_MANIFEST_SHA256",
                      "APPROVED_CONTRACT_SHA256", "EXPECTED_CONTRACT_SHA256",
                      "CHANGE_APPROVAL_NONCE", "EXPECTED_APPROVAL_NONCE",
                      "CHANGE_WINDOW_NOT_BEFORE_EPOCH", "CHANGE_WINDOW_EXPIRES_AT_EPOCH",
                      "MAX_APPROVAL_WINDOW_SECONDS"):
            if token not in container_text:
                errors.append(f"alerts v2 expand approval guard missing: {token}")
        if str(expand.get("contract_sha256", "")) not in container_text:
            errors.append("alerts v2 expand Job is not bound to the contract SHA-256")

    renderer_text = (root / EXPAND_RENDERER).read_text(encoding="utf-8")
    for token in ("build_snapshot", '"immutable": True', "EXPECTED_APPROVAL_NONCE",
                  "MAX_WINDOW_SECONDS", "refusing to overwrite rendered migration"):
        if token not in renderer_text:
            errors.append(f"expand renderer guard missing: {token}")

    backfill = contract.get("backfill_execution", {})
    if backfill.get("planner") != BACKFILL_PLANNER.as_posix():
        errors.append("backfill planner contract drifted")
    expected_backfill = {
        "renderer": BACKFILL_RENDERER.as_posix(),
        "mode": "READ_ONLY_PLAN",
        "source_index": "alerts",
        "target_alias": "alerts-v2-write",
        "default_execute": False,
        "mutating_endpoint_called_by_planner": False,
        "max_window_seconds": 3600,
        "max_documents_per_slice": 100000,
        "max_slices": 4,
        "max_requests_per_second": 500,
        "minimum_free_bytes": 161061273600,
        "max_plan_age_seconds": 900,
        "conflict_policy": "abort",
        "destination_op_type": "create",
        "execute_only_from_approved_suspended_job": True,
        "task_cancel_required": True,
        "source_recount_before_execute": True,
        "target_scope_must_be_empty_before_execute": True,
        "target_recount_after_execute": True,
    }
    for key, expected in expected_backfill.items():
        if backfill.get(key) != expected:
            errors.append(f"backfill contract drifted: {key}")
    required_scope = {
        "tenant_id", "start_time", "end_time", "time_field", "max_documents",
        "slices", "requests_per_second", "minimum_free_bytes",
    }
    if set(backfill.get("required_scope_fields", [])) != required_scope:
        errors.append("backfill required scope fields are incomplete")
    expected_backfill_approval = {
        "approval_id", "approved_by", "approval_nonce", "not_before_epoch", "expires_at_epoch",
        "cluster_uuid", "g0_candidate_sha256", "g0_manifest_sha256", "contract_file_sha256",
        "plan_sha256", "plan_file_sha256",
    }
    if set(backfill.get("approval_binding_fields", [])) != expected_backfill_approval:
        errors.append("backfill approval bindings are incomplete")
    planner_text = (root / BACKFILL_PLANNER).read_text(encoding="utf-8")
    for token in ("READ_ONLY_PLAN", "tenant_id.keyword", "wait_for_completion=false",
                  "requests_per_second", "max_docs", '"op_type": "create"',
                  "/_tasks/{task_id}/_cancel", '"production_mutations": []'):
        if token not in planner_text:
            errors.append(f"bounded backfill planner guard missing: {token}")
    if 'reader.request("/_reindex"' in planner_text:
        errors.append("read-only backfill planner must never call _reindex")
    backfill_renderer_text = (root / BACKFILL_RENDERER).read_text(encoding="utf-8")
    for token in (
        "validate_plan", "build_snapshot", '"suspend": True', '"backoffLimit": 0',
        "EXPECTED_PLAN_SHA256", "EXPECTED_PLAN_FILE_SHA256", "EXPECTED_SOURCE_COUNT",
        "TARGET_COUNT_BEFORE", "TARGET_COUNT_AFTER", "_tasks/$TASK_ID/_cancel",
        "wait_for_completion=true", '"failures":[]', '"version_conflicts":0',
        "refusing to overwrite rendered backfill",
    ):
        if token not in backfill_renderer_text:
            errors.append(f"backfill renderer guard missing: {token}")

    go_config = (root / GO_CONFIG).read_text(encoding="utf-8")
    go_writer = (root / GO_WRITER).read_text(encoding="utf-8")
    go_model = (root / GO_MODEL).read_text(encoding="utf-8")
    go_repo = (root / GO_REPOSITORY).read_text(encoding="utf-8")
    for token in ("OPENSEARCH_ALERTS_V2_ENABLED", "OPENSEARCH_ALERTS_READ_ALIAS",
                  "OPENSEARCH_ALERTS_WRITE_ALIAS", "ReadTarget()", "WriteTarget()"):
        if token not in go_config:
            errors.append(f"Go OpenSearch migration config missing: {token}")
    for forbidden in ("EnsureIndex", "IndicesPutIndexTemplateRequest", "_index_template"):
        if forbidden in go_writer:
            errors.append(f"application startup schema mutation remains: {forbidden}")
    if '"search_text"' not in go_repo or "r.readTarget" not in go_repo:
        errors.append("Go repository must query the governed read target and dedicated text field")

    alert_struct = re.search(r"type Alert struct \{(.*?)\n\}", go_model, re.S)
    if not alert_struct:
        errors.append("Go Alert producer struct was not found")
        go_fields: set[str] = set()
    else:
        go_fields = set(re.findall(r'json:"([a-z0-9_]+)(?:,omitempty)?"', alert_struct.group(1)))
    java_fields = set(re.findall(r'doc\.put\("([a-z0-9_]+)"', (root / FLINK_SINK).read_text(encoding="utf-8")))
    unmapped = sorted((go_fields | java_fields) - set(properties))
    if unmapped:
        errors.append(f"strict mapping misses producer fields: {unmapped}")

    go_deployment = (root / GO_DEPLOYMENT).read_text(encoding="utf-8")
    if "OPENSEARCH_ALERTS_V2_ENABLED, value: \"false\"" not in go_deployment:
        errors.append("Go production candidate must keep alerts v2 traffic disabled")
    flink_job = (root / FLINK_JOB).read_text(encoding="utf-8")
    flink_properties = (root / FLINK_PROPERTIES).read_text(encoding="utf-8")
    if "resolveOpenSearchWriteTarget" not in flink_job:
        errors.append("Flink writer target must be selected through an explicit v2 guard")
    if "opensearch.alerts.v2.enabled=false" not in flink_properties:
        errors.append("Flink production defaults must keep alerts v2 traffic disabled")
    if "opensearch.alerts.write.alias=alerts-v2-write" not in flink_properties:
        errors.append("Flink v2 write alias is missing")

    runtime = contract.get("runtime_guards", {})
    for guard in (
        "application_schema_mutation_forbidden",
        "legacy_target_retained_until_cutover",
        "read_and_write_targets_are_distinct",
        "default_deploy_path_excludes_v2_expand",
        "expand_job_suspended_by_default",
        "approval_secret_required",
        "cluster_identity_bound",
        "approval_window_expiry_required",
        "approval_nonce_bound",
        "rendered_approval_secret_immutable",
        "backfill_planner_read_only",
        "backfill_scope_bounded",
    ):
        if runtime.get(guard) is not True:
            errors.append(f"runtime guard must remain true: {guard}")
    if runtime.get("v2_traffic_default_enabled") is not False:
        errors.append("v2 traffic must remain disabled by default")

    return {
        "status": "PASS" if not errors else "FAIL",
        "remediation_id": contract.get("remediation_id"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "mapping_fields": len(properties),
        "producer_fields": len(go_fields | java_fields),
        "component_templates": template.get("composed_of", []),
        "read_alias": contract.get("read_alias"),
        "write_alias": contract.get("write_alias"),
        "v2_traffic_default_enabled": runtime.get("v2_traffic_default_enabled"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

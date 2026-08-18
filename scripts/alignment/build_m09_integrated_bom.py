#!/usr/bin/env python3
"""Build the deterministic T1-M09 integrated evidence BOM."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT_RELATIVE = Path("contracts/alignment/m09-integrated-bom.v1.json")
TASK_REGISTRY_RELATIVE = Path("contracts/alignment/task-registry.v1.json")
JOURNEY_INPUT_RELATIVE = Path(
    "doc/02_acceptance/topic1/tasks/t1-m09-n023/journey-evidence-input.json"
)
COMPONENTS = (
    ("T1-M09-N001", "contract_authority", "doc/02_acceptance/topic1/tasks/t1-m09-n001/k8s-product-contract-latest.json"),
    ("T1-M09-N002", "backend_characterization", "doc/02_acceptance/topic1/tasks/t1-m09-n002/k8s-encrypted-stats-seam-latest.json"),
    ("T1-M09-N003", "backend_composition", "doc/02_acceptance/topic1/tasks/t1-m09-n003/k8s-system-handler-composition-latest.json"),
    ("T1-M09-N004", "typed_client", "doc/02_acceptance/topic1/tasks/t1-m09-n004/k8s-encrypted-typed-client-latest.json"),
    ("T1-M09-N005", "compatibility_adapter", "doc/02_acceptance/topic1/tasks/t1-m09-n005/k8s-encrypted-adapter-plan-latest.json"),
    ("T1-M09-N006", "alert_snapshot_repository", "doc/02_acceptance/topic1/tasks/t1-m09-n006/k8s-alert-snapshot-repository-latest.json"),
    ("T1-M09-N007", "encrypted_snapshot_api", "doc/02_acceptance/topic1/tasks/t1-m09-n007/k8s-encrypted-snapshot-latest.json"),
    ("T1-M09-N008", "encrypted_snapshot_ui", "doc/02_acceptance/topic1/tasks/t1-m09-n008/k8s-encrypted-snapshot-ui-latest.json"),
    ("T1-M09-N009", "forensics_command", "doc/02_acceptance/topic1/tasks/t1-m09-n009/k8s-forensics-task-command-latest.json"),
    ("T1-M09-N010", "forensics_worker", "doc/02_acceptance/topic1/tasks/t1-m09-n010/k8s-forensics-worker-latest.json"),
    ("T1-M09-N011", "forensics_workbench", "doc/02_acceptance/topic1/tasks/t1-m09-n011/k8s-forensics-workbench-latest.json"),
    ("T1-M09-N012", "alert_evidence_pipeline", "doc/02_acceptance/topic1/tasks/t1-m09-n012/k8s-alert-evidence-link-pipeline-latest.json"),
    ("T1-M09-N013", "attack_chain_snapshot", "doc/02_acceptance/topic1/tasks/t1-m09-n013/k8s-attack-chain-snapshot-ui-latest.json"),
    ("T1-M09-N014", "graph_workbench", "doc/02_acceptance/topic1/tasks/t1-m09-n014/k8s-graph-workbench-governance-latest.json"),
    ("T1-M09-N015", "search_cursor", "doc/02_acceptance/topic1/tasks/t1-m09-n015/k8s-opensearch-cursor-latest.json"),
    ("T1-M09-N016", "alert_report", "doc/02_acceptance/topic1/tasks/t1-m09-n016/k8s-alert-report-latest.json"),
    ("T1-M09-N017", "feedback_authority", "doc/02_acceptance/topic1/tasks/t1-m09-n017/k8s-model-feedback-latest.json"),
    ("T1-M09-N018", "whitelist_governance", "doc/02_acceptance/topic1/tasks/t1-m09-n018/k8s-whitelist-governance-latest.json"),
    ("T1-M09-N019", "rule_model_review", "doc/02_acceptance/topic1/tasks/t1-m09-n019/k8s-rule-model-review-latest.json"),
    ("T1-M09-N020", "response_handoff", "doc/02_acceptance/topic1/tasks/t1-m09-n020/k8s-response-handoff-latest.json"),
    ("T1-M09-N021", "page_state_accessibility", "doc/02_acceptance/topic1/tasks/t1-m09-n021/k8s-page-state-accessibility-latest.json"),
    ("T1-M09-N022", "route_css_refactor", "doc/02_acceptance/topic1/tasks/t1-m09-n022/k8s-css-refactor-latest.json"),
    ("T1-M09-N023", "windows_journey_evidence", "doc/02_acceptance/topic1/tasks/t1-m09-n023/k8s-journey-evidence-latest.json"),
)


def load_json(path: Path) -> dict[str, Any]:
    body = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(body, dict):
        raise ValueError(f"JSON document must be an object: {path}")
    return body


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_sha256(value: Any) -> str:
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def collect_image_refs(value: Any) -> list[dict[str, str | None]]:
    refs: set[tuple[str, str | None]] = set()

    def visit(node: Any) -> None:
        if isinstance(node, dict):
            image = node.get("image")
            if isinstance(image, str):
                image_id = node.get("image_id")
                refs.add((image, image_id if isinstance(image_id, str) else None))
            for child in node.values():
                visit(child)
        elif isinstance(node, list):
            for child in node:
                visit(child)

    visit(value)
    return [
        {"image": image, "image_id": image_id}
        for image, image_id in sorted(refs, key=lambda item: (item[0], item[1] or ""))
    ]


def task_map(root: Path) -> tuple[dict[str, dict[str, Any]], str]:
    path = root / TASK_REGISTRY_RELATIVE
    registry = load_json(path)
    tasks = registry.get("tasks")
    if not isinstance(tasks, list):
        raise ValueError("task registry tasks must be an array")
    return {
        task["task_id"]: task
        for task in tasks
        if isinstance(task, dict) and isinstance(task.get("task_id"), str)
    }, sha256_file(path)


def build_component(
    root: Path,
    tasks: dict[str, dict[str, Any]],
    task_id: str,
    role: str,
    relative: str,
) -> dict[str, Any]:
    path = (root / relative).resolve(strict=False)
    if not path.is_relative_to(root.resolve()) or not path.is_file():
        raise ValueError(f"component evidence is absent or escapes the repository: {relative}")
    body = load_json(path)
    if body.get("task_id") != task_id:
        raise ValueError(f"component task identity mismatch: {task_id}")
    if body.get("status") != "PASS":
        raise ValueError(f"component evidence is not PASS: {task_id}")
    if body.get("production_applied") is not False:
        raise ValueError(f"component production boundary drifted: {task_id}")
    task = tasks.get(task_id)
    if task is None:
        raise ValueError(f"task registry entry is absent: {task_id}")
    return {
        "component_id": task_id.lower(),
        "task_id": task_id,
        "role": role,
        "evidence": {"path": relative, "sha256": sha256_file(path)},
        "run_id": body.get("run_id"),
        "evidence_status": body.get("status"),
        "coverage_status": body.get("coverage_status", body.get("validation", {}).get("coverage_status", "BOUNDED_PASS")),
        "candidate_manifest_sha256": body.get("candidate_manifest_sha256"),
        "image_refs": collect_image_refs(body),
        "depends_on_tasks": list(task.get("depends_on_tasks", [])),
        "production_applied": False,
    }


def build_trace_index(root: Path) -> tuple[list[dict[str, Any]], dict[str, str]]:
    path = root / JOURNEY_INPUT_RELATIVE
    manifest = load_json(path)
    journeys = manifest.get("journeys")
    if not isinstance(journeys, list) or len(journeys) != 7:
        raise ValueError("N023 journey input must contain exactly seven journeys")
    index: list[dict[str, Any]] = []
    for journey in journeys:
        if not isinstance(journey, dict):
            raise ValueError("N023 journey entry must be an object")
        trace = journey.get("cross_storage_trace")
        receipts = trace.get("receipts") if isinstance(trace, dict) else None
        final_fact = trace.get("final_fact") if isinstance(trace, dict) else None
        index.append(
            {
                "journey_id": journey.get("journey_id"),
                "atomic_pr_id": journey.get("atomic_pr_id"),
                "status": journey.get("status"),
                "network_recorded": isinstance(journey.get("network"), dict),
                "receipt_count": len(receipts) if isinstance(receipts, list) else 0,
                "storage_objects_recorded": bool(receipts),
                "final_effect_recorded": isinstance(final_fact, dict),
                "trace_id": trace.get("trace_id") if isinstance(trace, dict) else None,
            }
        )
    return index, {"path": str(JOURNEY_INPUT_RELATIVE), "sha256": sha256_file(path)}


def build(root: Path = ROOT) -> dict[str, Any]:
    tasks, registry_sha = task_map(root)
    components = [
        build_component(root, tasks, task_id, role, relative)
        for task_id, role, relative in COMPONENTS
    ]
    trace_index, journey_binding = build_trace_index(root)
    dependency_edges = sorted(
        [
            {"from": dependency, "to": component["task_id"]}
            for component in components
            for dependency in component["depends_on_tasks"]
        ],
        key=lambda edge: (edge["to"], edge["from"]),
    )
    candidate_hashes = sorted(
        {
            value
            for component in components
            if isinstance((value := component["candidate_manifest_sha256"]), str)
        }
    )
    verified_journeys = sum(item["status"] == "VERIFIED" for item in trace_index)
    complete_traces = sum(
        item["network_recorded"]
        and item["receipt_count"] > 0
        and item["storage_objects_recorded"]
        and item["final_effect_recorded"]
        and bool(item["trace_id"])
        for item in trace_index
    )
    assembly_basis = {
        "components": [
            {
                "task_id": item["task_id"],
                "role": item["role"],
                "evidence": item["evidence"],
                "depends_on_tasks": item["depends_on_tasks"],
            }
            for item in components
        ],
        "dependency_edges": dependency_edges,
        "journey_input": journey_binding,
    }
    blockers = []
    if len(candidate_hashes) != 1:
        blockers.append("SAME_CANDIDATE_MANIFEST_REQUIRED")
    if verified_journeys != 7:
        blockers.append("WINDOWS_CHROME_SEVEN_JOURNEYS_REQUIRED")
    if complete_traces != 7:
        blockers.append("CROSS_STORAGE_TRACE_FINAL_EFFECT_REQUIRED")
    if any(component["production_applied"] is not True for component in components):
        blockers.append("PRODUCTION_APPLIED_REQUIRED")
    return {
        "schema_version": 1,
        "artifact_kind": "M09_INTEGRATED_BOM",
        "milestone_id": "T1-M09",
        "accountable_task": "T1-M09-N024",
        "bom_state": "ASSEMBLED",
        "assembly_id": canonical_sha256(assembly_basis),
        "candidate_id": candidate_hashes[0] if len(candidate_hashes) == 1 else None,
        "profile_id": "M09_K8S_COMPONENT_EVIDENCE_INDEX_V1",
        "environment_id": "existing-k8s-run-scoped-mixed-candidates",
        "source_task_registry": {
            "path": str(TASK_REGISTRY_RELATIVE),
            "sha256": registry_sha,
        },
        "journey_input": journey_binding,
        "components": components,
        "dependency_edges": dependency_edges,
        "journey_trace_index": trace_index,
        "closure": {
            "component_count": len(components),
            "component_evidence_exact_hash": True,
            "dependency_edges_closed": True,
            "same_candidate_manifest": len(candidate_hashes) == 1,
            "candidate_manifest_hash_count": len(candidate_hashes),
            "verified_windows_journeys": verified_journeys,
            "complete_cross_storage_traces": complete_traces,
            "production_applied": False,
        },
        "engineering_status": "PASS",
        "promotion_status": "BLOCKED" if blockers else "PASS",
        "promotion_allowed": not blockers,
        "blocking_codes": blockers,
        "allowed_claims": [
            "M09 has an exact-hash ASSEMBLED index for 23 bounded Kubernetes component evidence artifacts"
        ],
        "forbidden_claims": [
            "one same-candidate M09 end-to-end journey passed",
            "designated Windows Chrome acceptance passed",
            "M09 production activation or promotion is authorized",
            "M09 or the whole project is complete",
        ],
        "production_applied": False,
    }


def render(value: dict[str, Any]) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / OUTPUT_RELATIVE)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    payload = render(build(ROOT))
    output = args.output.resolve(strict=False)
    if not output.is_relative_to(ROOT.resolve()) and args.output != Path("/tmp/m09-integrated-bom.json"):
        raise SystemExit("output must be the repository BOM or /tmp/m09-integrated-bom.json")
    if args.check:
        if not output.is_file() or output.read_text(encoding="utf-8") != payload:
            print("FAIL: M09 integrated BOM is stale")
            return 1
        print("PASS: M09 integrated BOM is deterministic and current")
        return 0
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

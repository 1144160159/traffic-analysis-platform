#!/usr/bin/env python3
"""Evaluate M06 IDX/PROM closure without creating or repairing evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/alignment/m06-promotion-gate.v1.json"
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


def add_error(errors: list[dict[str, Any]], code: str, identity: str, detail: Any = None) -> None:
    errors.append({"code": code, "identity": identity, "detail": detail})


def load_binding(
    binding: Any,
    *,
    errors: list[dict[str, Any]],
    identity: str,
) -> tuple[dict[str, Any], dict[str, str] | None]:
    if not isinstance(binding, dict) or set(binding) != {"path", "sha256"}:
        add_error(errors, "BINDING_SHAPE", identity, binding)
        return {}, None
    relative, expected_sha = binding.get("path"), binding.get("sha256")
    if not isinstance(relative, str) or not SHA_RE.fullmatch(str(expected_sha or "")):
        add_error(errors, "BINDING_IDENTITY", identity, binding)
        return {}, None
    path = (ROOT / relative).resolve(strict=False)
    if not path.is_relative_to(ROOT.resolve()) or not path.is_file():
        add_error(errors, "BINDING_PATH", identity, relative)
        return {}, None
    observed_sha = hashlib.sha256(path.read_bytes()).hexdigest()
    if observed_sha != expected_sha:
        add_error(errors, "BINDING_HASH", identity, {"expected": expected_sha, "observed": observed_sha})
        return {}, None
    try:
        body = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        add_error(errors, "BINDING_JSON", identity, str(error))
        return {}, None
    if not isinstance(body, dict):
        add_error(errors, "BINDING_JSON", identity, type(body).__name__)
        return {}, None
    return body, {"path": relative, "sha256": expected_sha}


def exact_scope(body: dict[str, Any], candidate: str, profile: str, environment: str) -> bool:
    return (
        body.get("candidate_id"), body.get("profile_id"), body.get("environment_id")
    ) == (candidate, profile, environment)


def evaluate(manifest: dict[str, Any], contract: dict[str, Any]) -> dict[str, Any]:
    errors: list[dict[str, Any]] = []
    if set(manifest) != set(contract["manifest_fields"]):
        add_error(errors, "MANIFEST_EXACT_FIELDS", "manifest", sorted(manifest))
    if manifest.get("schema_version") != 1 or manifest.get("artifact_kind") != "M06_PROMOTION_MANIFEST":
        add_error(errors, "MANIFEST_IDENTITY", "manifest")
    candidate = manifest.get("candidate_id")
    profile = manifest.get("profile_id")
    environment = manifest.get("environment_id")
    if not SHA_RE.fullmatch(str(candidate or "")):
        add_error(errors, "CANDIDATE_ID", "manifest", candidate)
    if not isinstance(profile, str) or not profile or not isinstance(environment, str) or not environment:
        add_error(errors, "PROFILE_ENVIRONMENT", "manifest", [profile, environment])
    if manifest.get("production_applied") is not True:
        add_error(errors, "PRODUCTION_APPLIED_REQUIRED", "manifest", manifest.get("production_applied"))

    candidate_body, candidate_binding = load_binding(
        manifest.get("candidate_manifest"), errors=errors, identity="candidate-manifest"
    )
    if candidate_binding and candidate_binding["sha256"] != candidate:
        add_error(errors, "CANDIDATE_MANIFEST_HASH", "candidate-manifest")
    if (
        candidate_body
        and (
            candidate_body.get("artifact_kind") != "M06_IMPLEMENTATION_CANDIDATE_MANIFEST"
            or candidate_body.get("status") != "FROZEN"
            or candidate_body.get("profile_id") != profile
            or candidate_body.get("environment_id") != environment
        )
    ):
        add_error(errors, "CANDIDATE_MANIFEST", "candidate-manifest")

    required_tasks = contract["required_task_indexes"]
    tasks = manifest.get("task_indexes", {})
    if not isinstance(tasks, dict):
        add_error(errors, "TASK_INDEX_SHAPE", "task-indexes", type(tasks).__name__)
        tasks = {}
    if set(tasks) != set(required_tasks):
        add_error(errors, "TASK_INDEX_EXACT_SET", "task-indexes", sorted(tasks))
    task_bindings: dict[str, dict[str, str]] = {}
    for task in required_tasks:
        body, binding = load_binding(tasks.get(task), errors=errors, identity=task)
        if binding:
            task_bindings[task] = binding
        if body and (
            body.get("artifact_kind") != "M06_TASK_CURRENT_EVIDENCE_INDEX"
            or body.get("task_id") != task
            or body.get("status") != "PASS"
            or not exact_scope(body, candidate, profile, environment)
            or not isinstance(body.get("evidence_receipts"), list)
            or not body["evidence_receipts"]
        ):
            add_error(errors, "TASK_INDEX_RESULT", task)

    required_phases = contract["required_phase_acceptance"]
    phases = manifest.get("phase_acceptance", {})
    if not isinstance(phases, dict):
        add_error(errors, "PHASE_ACCEPTANCE_SHAPE", "phase-acceptance", type(phases).__name__)
        phases = {}
    if set(phases) != set(required_phases):
        add_error(errors, "PHASE_ACCEPTANCE_EXACT_SET", "phase-acceptance", sorted(phases))
    phase_bindings: dict[str, dict[str, str]] = {}
    for phase in required_phases:
        body, binding = load_binding(phases.get(phase), errors=errors, identity=f"phase:{phase}")
        if binding:
            phase_bindings[phase] = binding
        if body and (
            body.get("artifact_kind") != "M06_PHASE_ACCEPTANCE_RECEIPT"
            or body.get("phase") != phase
            or body.get("status") != "PASS"
            or body.get("production_applied") is not True
            or not exact_scope(body, candidate, profile, environment)
        ):
            add_error(errors, "PHASE_ACCEPTANCE_RESULT", phase)

    reconcile, reconcile_binding = load_binding(
        manifest.get("four_source_reconciliation"), errors=errors, identity="four-source-reconciliation"
    )
    expected_rails = contract["required_four_source_rails"]
    rail_results = reconcile.get("rail_results", {}) if reconcile else {}
    if not isinstance(rail_results, dict):
        add_error(errors, "FOUR_SOURCE_RAIL_SHAPE", "four-source-reconciliation", type(rail_results).__name__)
        rail_results = {}
    if reconcile and (
        reconcile.get("artifact_kind") != "M06_FOUR_SOURCE_WINDOW_RECONCILIATION"
        or reconcile.get("status") != "PASS"
        or reconcile.get("production_applied") is not True
        or not exact_scope(reconcile, candidate, profile, environment)
        or set(rail_results) != set(expected_rails)
        or any(not isinstance(rail_results.get(rail), dict) or rail_results[rail].get("status") != "PASS" for rail in expected_rails)
    ):
        add_error(errors, "FOUR_SOURCE_RECONCILIATION", "four-source-reconciliation")

    required_rollbacks = contract["required_canary_rollbacks"]
    rollbacks = manifest.get("canary_rollbacks", {})
    if not isinstance(rollbacks, dict):
        add_error(errors, "ROLLBACK_SHAPE", "canary-rollbacks", type(rollbacks).__name__)
        rollbacks = {}
    if set(rollbacks) != set(required_rollbacks):
        add_error(errors, "ROLLBACK_EXACT_SET", "canary-rollbacks", sorted(rollbacks))
    rollback_bindings: dict[str, dict[str, str]] = {}
    for phase in required_rollbacks:
        body, binding = load_binding(rollbacks.get(phase), errors=errors, identity=f"rollback:{phase}")
        if binding:
            rollback_bindings[phase] = binding
        rollback = body.get("rollback", {}) if body else {}
        if not isinstance(rollback, dict):
            rollback = {}
        if body and (
            body.get("artifact_kind") != "M06_CANARY_ROLLBACK_RESULT"
            or body.get("phase") != phase
            or body.get("status") != "PASS"
            or body.get("production_applied") is not False
            or rollback.get("status") != "PASS"
            or not exact_scope(body, candidate, profile, environment)
        ):
            add_error(errors, "ROLLBACK_RESULT", phase)

    if manifest.get("allowed_claims") != contract["allowed_claims"]:
        add_error(errors, "ALLOWED_CLAIMS", "claims")
    if manifest.get("forbidden_claims") != contract["forbidden_claims"]:
        add_error(errors, "FORBIDDEN_CLAIMS", "claims")
    status = "PASS" if not errors else "BLOCKED"
    return {
        "schema_version": 1,
        "artifact_kind": "M06_PROMOTION_EVALUATION",
        "candidate_id": candidate,
        "profile_id": profile,
        "environment_id": environment,
        "status": status,
        "promotion_allowed": status == "PASS",
        "current_evidence": {
            "candidate_manifest": candidate_binding,
            "task_indexes": task_bindings,
            "phase_acceptance": phase_bindings,
            "four_source_reconciliation": reconcile_binding,
            "canary_rollbacks": rollback_bindings,
        },
        "errors": errors,
        "allowed_claims": contract["allowed_claims"] if status == "PASS" else [],
        "forbidden_claims": contract["forbidden_claims"],
        "production_applied": False,
        "automatic_repair": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    manifest_path = args.manifest.resolve(strict=True)
    if not manifest_path.is_relative_to(ROOT.resolve()):
        raise SystemExit("manifest must be inside repository")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    result = evaluate(manifest, contract)
    payload = json.dumps(result, sort_keys=True, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if not output.is_relative_to(ROOT.resolve()):
            raise SystemExit("output must be inside repository")
        if output.exists():
            raise SystemExit(f"refusing to overwrite promotion evaluation: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

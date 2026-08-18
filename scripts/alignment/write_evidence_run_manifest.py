#!/usr/bin/env python3
"""Materialize one immutable, candidate-bound evidence-run manifest.

This adapter never evaluates business truth.  It may run only after the
leaf-specific verifier has produced a schema-validated PASS result; it binds
that result to the signed execution-package and plan identities so the overlay
can consume exactly one manifest for the declared gate.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

from build_topic1_task_registry import validate_against_schema


ROOT = Path(__file__).resolve().parents[2]
EVIDENCE_SCHEMA = ROOT / "contracts/alignment/evidence-run-manifest.schema.json"
IMPLEMENTATION_CANDIDATE_SCHEMA = ROOT / "contracts/alignment/implementation-candidate.schema.json"
DESIGN_CANDIDATE_SCHEMA = ROOT / "contracts/alignment/design-candidate-manifest.schema.json"
PLAN_SCHEMA = ROOT / "contracts/alignment/atomic-pr-plan-manifest.schema.json"


def validate_result_identity(
    result_payload: dict,
    design_candidate: dict,
    design_candidate_sha: str,
    profile_id: str,
    environment_id: str,
    run_id: str,
    subject_pr_id: str | None = None,
) -> None:
    """Reject a typed PASS that belongs to other bytes, identity, or run."""
    result_candidate = result_payload.get("candidate_manifest")
    if (
        not isinstance(result_candidate, dict)
        or result_candidate.get("sha256") != design_candidate_sha
        or result_payload.get("profile_id") != profile_id
        or result_payload.get("environment_id") != environment_id
        or result_payload.get("run_id") != run_id
    ):
        raise ValueError(
            "leaf-specific result is not bound to the current candidate/profile/environment/run"
        )
    if subject_pr_id is not None and result_payload.get("subject_pr_id") != subject_pr_id:
        raise ValueError("leaf-specific result belongs to another atomic PR")
    source_hashes = result_payload.get("source_blob_sha256")
    candidate_sources = design_candidate.get("source_blob_sha256")
    if not isinstance(source_hashes, dict) or not source_hashes:
        raise ValueError("leaf-specific result lacks a non-empty source hash binding")
    if not isinstance(candidate_sources, dict):
        raise ValueError("candidate manifest lacks source hash bindings")
    for source_path, source_sha in source_hashes.items():
        if candidate_sources.get(source_path) != source_sha:
            raise ValueError(f"leaf result crosses candidate source bytes: {source_path}")
    artifact_kind = result_payload.get("artifact_kind")
    if artifact_kind in {
        "ASSET_ATOMIC_EPHEMERAL_TEST_RESULT",
        "ASSET_PROJECTION_KAFKA_EPHEMERAL_TEST_RESULT",
    } and not result_payload.get("oracle_results"):
        raise ValueError("G1 result has no structured business oracle results")
    if artifact_kind == "EXACT_GO_TEST_RESULT" and not result_payload.get("event_counts"):
        raise ValueError("exact Go result has no exact run/pass event counts")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--subject-pr-id", required=True)
    parser.add_argument("--subject-work-id", required=True)
    parser.add_argument("--milestone-id", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--gate-id", required=True, choices=[f"G{i}" for i in range(9)])
    parser.add_argument("--candidate-manifest", required=True)
    parser.add_argument("--design-candidate-manifest", required=True)
    parser.add_argument("--profile-id", required=True)
    parser.add_argument("--environment-id", required=True)
    parser.add_argument("--execution-package-sha256", required=True)
    parser.add_argument("--plan", required=True)
    parser.add_argument("--time-window", required=True)
    parser.add_argument("--result-artifact", required=True)
    parser.add_argument("--result-schema", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    def repo_file(relative: str) -> Path:
        path = (ROOT / relative).resolve()
        if not path.is_relative_to(ROOT) or not path.is_file():
            raise ValueError(f"unsafe or absent artifact: {relative}")
        return path

    candidate_path = repo_file(args.candidate_manifest)
    candidate = json.loads(candidate_path.read_text(encoding="utf-8"))
    validate_against_schema(candidate, IMPLEMENTATION_CANDIDATE_SCHEMA)
    candidate_sha = hashlib.sha256(candidate_path.read_bytes()).hexdigest()
    if candidate["environment_id"] != args.environment_id:
        raise ValueError("implementation candidate environment differs from evidence environment")
    design_candidate_path = repo_file(args.design_candidate_manifest)
    design_candidate = json.loads(design_candidate_path.read_text(encoding="utf-8"))
    validate_against_schema(design_candidate, DESIGN_CANDIDATE_SCHEMA)
    design_candidate_sha = hashlib.sha256(design_candidate_path.read_bytes()).hexdigest()
    if (
        design_candidate["implementation_candidate_commit"]
        != candidate["implementation_candidate_commit"]
    ):
        raise ValueError("design and implementation candidates do not identify the same commit")
    plan_path = repo_file(args.plan)
    plan = json.loads(plan_path.read_text(encoding="utf-8"))
    validate_against_schema(plan, PLAN_SCHEMA)
    plan_sha = hashlib.sha256(plan_path.read_bytes()).hexdigest()
    if (
        plan["plan_kind"] != "TEST"
        or plan["status"] != "APPROVED"
        or plan["atomic_pr_id"] != args.subject_pr_id
        or plan["candidate_manifest_sha256"] != candidate_sha
        or plan["profile_id"] != args.profile_id
        or args.gate_id not in plan["content"]["required_gates"]
    ):
        raise ValueError("TEST plan does not authorize this candidate/profile/gate")
    result_path = repo_file(args.result_artifact)
    result_sha = hashlib.sha256(result_path.read_bytes()).hexdigest()
    result_payload = json.loads(result_path.read_text(encoding="utf-8"))
    if not isinstance(result_payload, dict) or result_payload.get("status", result_payload.get("result")) != "PASS":
        raise ValueError("leaf-specific result does not carry a typed PASS status")
    validate_against_schema(result_payload, repo_file(args.result_schema))
    validate_result_identity(
        result_payload, design_candidate, design_candidate_sha,
        args.profile_id, args.environment_id, args.run_id, args.subject_pr_id,
    )
    manifest = {
        "schema_version": "1.0.0",
        "run_id": args.run_id,
        "subject_pr_id": args.subject_pr_id,
        "subject_work_id": args.subject_work_id,
        "subject_milestone_id": args.milestone_id,
        "execution_package_sha256": args.execution_package_sha256,
        "plan_kind": "TEST",
        "plan_id": plan["plan_id"],
        "plan_sha256": plan_sha,
        "bom_transition_sha256": None,
        "candidate_manifest_sha256": candidate_sha,
        "profile_id": args.profile_id,
        "environment_id": args.environment_id,
        "time_window": args.time_window,
        "run_purpose": "VERIFICATION",
        "gate_id": args.gate_id,
        "result": "PASS",
        "artifacts": [{
            "direction": "OUTPUT",
            "artifact_id": f"RESULT-{args.subject_pr_id}-{args.gate_id}",
            "path": args.result_artifact,
            "sha256": result_sha,
            "schema_ref": args.result_schema,
        }],
        "production_applied": False,
        "exclusions": ["parent completion", "milestone completion", "production acceptance"],
    }
    validate_against_schema(manifest, EVIDENCE_SCHEMA)
    output = (ROOT / args.output).resolve()
    if not output.is_relative_to(ROOT):
        raise ValueError("output must be repository-relative")
    encoded = (json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
    output.parent.mkdir(parents=True, exist_ok=True)
    if output.exists() and output.read_bytes() != encoded:
        raise ValueError("immutable evidence manifest exists with different bytes")
    if not output.exists():
        output.write_bytes(encoded)
    print(f"PASS exact {args.gate_id} evidence manifest for {args.subject_pr_id}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

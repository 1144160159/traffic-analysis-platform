#!/usr/bin/env python3
"""Verify T-FLINK-005 static, release-bound and runtime Flink job registries."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
REGISTRY_PATH = ROOT / "contracts/flink/job-registry.v1.json"
APPLICATION_PATH = ROOT / "contracts/flink/application-cluster-migration.v1.json"
STATE_PATH = ROOT / "contracts/flink/state-recovery.v1.json"
CHECKPOINT_PATH = ROOT / "contracts/flink/checkpoint-ha-upgrade.v1.json"
SINK_PATH = ROOT / "contracts/flink/sink-reconciliation.v1.json"
SHA_RE = re.compile(r"^[0-9a-f]{64}$")
IMAGE_RE = re.compile(r"^[a-z0-9][a-z0-9._/:~-]*@sha256:[0-9a-f]{64}$")


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def canonical_uid_hash(uids: list[str]) -> str:
    payload = "\n".join(sorted(uids)) + "\n"
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def contract_sha256(registry: dict[str, Any]) -> str:
    payload = json.dumps(registry, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def expected_jobs(
    registry: dict[str, Any],
    application: dict[str, Any],
    state: dict[str, Any],
) -> dict[str, dict[str, Any]]:
    app_jobs = {item["id"]: item for item in application["jobs"]}
    common = registry["common"]
    state_roots = application["state"]
    expected: dict[str, dict[str, Any]] = {}
    for item in registry["jobs"]:
        job_id = item["id"]
        app = app_jobs[job_id]
        expected[job_id] = {
            "job_id": job_id,
            "job_name": app["job_name"],
            "entry_class": app["main_class"],
            "parallelism": app["parallelism"],
            "max_parallelism": item["max_parallelism"],
            "operator_uid_sha256": item["operator_uid_sha256"],
            "state_backend": common["state_backend"],
            "checkpoint_path": f"{state_roots['checkpoint_root']}/{job_id}",
            "savepoint_path": f"{state_roots['savepoint_root']}/{job_id}",
            "source_start_mode": common["source_start_mode"],
            "sink_guarantee": common["sink_guarantee"],
            "owner": item["owner"],
            "slo_ref": common["slo_ref"],
            "compatibility_scope": common["compatibility_scope"],
        }
    return expected


def _by_job(items: Any, label: str, errors: list[str]) -> dict[str, dict[str, Any]]:
    if not isinstance(items, list):
        errors.append(f"{label}.jobs must be a list")
        return {}
    result: dict[str, dict[str, Any]] = {}
    for item in items:
        if not isinstance(item, dict) or not isinstance(item.get("job_id"), str):
            errors.append(f"{label}.jobs contains an invalid job record")
            continue
        job_id = item["job_id"]
        if job_id in result:
            errors.append(f"{label}.jobs contains duplicate {job_id}")
        result[job_id] = item
    return result


def verify(
    registry: dict[str, Any],
    application: dict[str, Any],
    state: dict[str, Any],
    checkpoint: dict[str, Any],
    sink: dict[str, Any],
    release: dict[str, Any] | None = None,
    runtime: dict[str, Any] | None = None,
) -> dict[str, Any]:
    errors: list[str] = []
    if registry.get("schema_version") != 1 or registry.get("remediation_id") != "T-FLINK-005":
        errors.append("job registry must be schema v1 for T-FLINK-005")
    refs = registry.get("authoritative_contracts") or {}
    expected_refs = {
        "deployment": str(APPLICATION_PATH.relative_to(ROOT)),
        "operator_state": str(STATE_PATH.relative_to(ROOT)),
        "checkpoint_ha": str(CHECKPOINT_PATH.relative_to(ROOT)),
        "sink_protocol": str(SINK_PATH.relative_to(ROOT)),
    }
    if refs != expected_refs:
        errors.append("authoritative contract references drifted")

    registry_jobs = registry.get("jobs") or []
    registry_ids = [item.get("id") for item in registry_jobs if isinstance(item, dict)]
    application_ids = [item.get("id") for item in application.get("jobs") or []]
    state_ids = list((state.get("required_uids") or {}).keys())
    sink_ids = [item.get("id") for item in sink.get("jobs") or []]
    canonical = set(application_ids)
    if len(canonical) != 9:
        errors.append(f"application contract must contain nine jobs, found {len(canonical)}")
    for label, ids in (("registry", registry_ids), ("state", state_ids), ("sink", sink_ids)):
        if len(ids) != len(set(ids)) or set(ids) != canonical:
            errors.append(f"{label} canonical job set does not match application contract")

    app_by_id = {item["id"]: item for item in application.get("jobs") or []}
    required_uids = state.get("required_uids") or {}
    default_start = (checkpoint.get("source_start_policy") or {}).get("default")
    common = registry.get("common") or {}
    compatibility = common.get("compatibility_scope") or {}
    if default_start != common.get("source_start_mode"):
        errors.append("registry source start mode does not match checkpoint contract")
    if common.get("state_backend") != "rocksdb":
        errors.append("registry state backend must remain rocksdb")
    if compatibility.get("allow_non_restored_state") is not False:
        errors.append("allow_non_restored_state must be false")
    if compatibility.get("key_group_rescale_required") is not True:
        errors.append("key-group rescale must be required")

    for item in registry_jobs:
        job_id = item.get("id", "")
        app = app_by_id.get(job_id) or {}
        max_parallelism = item.get("max_parallelism")
        if app.get("max_parallelism") != max_parallelism:
            errors.append(f"{job_id}: max_parallelism drifts from deployment contract")
        if not isinstance(max_parallelism, int) or max_parallelism < int(app.get("parallelism", 1)):
            errors.append(f"{job_id}: max_parallelism is below current parallelism")
        uid_hash = canonical_uid_hash(required_uids.get(job_id) or [])
        if item.get("operator_uid_sha256") != uid_hash:
            errors.append(f"{job_id}: operator UID hash drifted")
        if not item.get("owner"):
            errors.append(f"{job_id}: owner is required")

    expected = expected_jobs(registry, application, state) if not errors else {}
    registry_digest = contract_sha256(registry)
    release_status = "NOT_PROVIDED"
    runtime_status = "NOT_PROVIDED"
    release_by_id: dict[str, dict[str, Any]] = {}
    if release is not None:
        release_status = "PASS"
        if release.get("schema_version") != 1:
            errors.append("release registry must use schema_version 1")
        if release.get("contract_sha256") != registry_digest:
            errors.append("release registry contract_sha256 does not match")
        if not SHA_RE.fullmatch(str(release.get("candidate_source_sha256", ""))):
            errors.append("release registry candidate_source_sha256 is invalid")
        release_by_id = _by_job(release.get("jobs"), "release", errors)
        if set(release_by_id) != canonical:
            errors.append("release registry canonical job set does not match")
        required = set((registry.get("release_binding") or {}).get("required_fields") or [])
        for job_id, item in release_by_id.items():
            missing = sorted(field for field in required if field not in item and field not in release)
            if missing:
                errors.append(f"release {job_id}: missing fields {missing}")
            if item.get("job_id") != job_id:
                errors.append(f"release {job_id}: job_id mismatch")
            if not SHA_RE.fullmatch(str(item.get("artifact_sha256", ""))):
                errors.append(f"release {job_id}: invalid artifact_sha256")
            if not IMAGE_RE.fullmatch(str(item.get("image_digest", ""))):
                errors.append(f"release {job_id}: mutable or invalid image_digest")
            if not SHA_RE.fullmatch(str(item.get("savepoint_sha256", ""))):
                errors.append(f"release {job_id}: invalid savepoint_sha256")
            for field, value in expected.get(job_id, {}).items():
                if field in item and item[field] != value:
                    errors.append(f"release {job_id}: {field} mismatch")

    if runtime is not None:
        runtime_status = "PASS"
        if release is None:
            errors.append("runtime diff requires a release registry")
        runtime_by_id = _by_job(runtime.get("jobs"), "runtime", errors)
        if set(runtime_by_id) != canonical:
            errors.append("runtime registry canonical job set does not match")
        required = set((registry.get("runtime_diff") or {}).get("required_fields") or [])
        for job_id, item in runtime_by_id.items():
            missing = sorted(required - set(item))
            if missing:
                errors.append(f"runtime {job_id}: missing fields {missing}")
            release_item = release_by_id.get(job_id) or {}
            for field in required:
                wanted = release_item.get(field, expected.get(job_id, {}).get(field))
                if item.get(field) != wanted:
                    errors.append(f"runtime {job_id}: {field} mismatch")

    if errors:
        if release is not None:
            release_status = "FAIL"
        if runtime is not None:
            runtime_status = "FAIL"
    return {
        "schema_version": 1,
        "contract_id": registry.get("contract_id"),
        "remediation_id": "T-FLINK-005",
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "PARTIAL" if runtime is None else ("COMPLETE" if not errors else "PARTIAL"),
        "canonical_jobs": len(canonical),
        "operator_uid_hashes": len(registry_jobs),
        "release_registry": release_status,
        "runtime_diff": runtime_status,
        "contract_sha256": registry_digest,
        "errors": errors,
        "remaining_gates": (registry.get("gate") or {}).get("remaining") or [],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--release-registry", type=Path)
    parser.add_argument("--runtime-snapshot", type=Path)
    args = parser.parse_args()
    if args.runtime_snapshot and not args.release_registry:
        raise SystemExit("--runtime-snapshot requires --release-registry")
    result = verify(
        load(REGISTRY_PATH),
        load(APPLICATION_PATH),
        load(STATE_PATH),
        load(CHECKPOINT_PATH),
        load(SINK_PATH),
        load(args.release_registry.resolve()) if args.release_registry else None,
        load(args.runtime_snapshot.resolve()) if args.runtime_snapshot else None,
    )
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Render a non-authorizing T-OS-002/T-OS-004 repair review package."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/opensearch/projection-shadow-backfill.v1.json"
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_rfc3339(value: str) -> datetime:
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    parsed = datetime.fromisoformat(normalized)
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include a timezone")
    return parsed.astimezone(timezone.utc)


def shadow_binding_sha256(binding: dict[str, Any]) -> str:
    # Go's ShadowBinding is marshalled in declared field order. Python preserves
    # the same order when loading the emitted manifest; any reordered/tampered
    # binding therefore fails closed instead of silently receiving a new hash.
    payload = json.dumps(binding, ensure_ascii=False, separators=(",", ":")).encode()
    return hashlib.sha256(payload).hexdigest()


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def current_head(root: Path) -> str:
    completed = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=root, check=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    return completed.stdout.strip()


def build_review_package(
    *,
    shadow: dict[str, Any],
    shadow_file_sha256: str,
    g0: dict[str, Any],
    g0_manifest_sha256: str,
    contract: dict[str, Any],
    repository_head: str,
    repository_content_sha256: str,
    now: datetime,
    immutable_tool_image_digest: str | None = None,
) -> dict[str, Any]:
    errors: list[str] = []
    now = now.astimezone(timezone.utc)
    if shadow.get("schema_version") != 1 or shadow.get("remediation_id") != "T-OS-004":
        errors.append("shadow manifest identity is not T-OS-004 schema v1")
    if shadow.get("mode") != "READ_ONLY_SHADOW":
        errors.append("shadow manifest is not read-only")
    if shadow.get("approval_readiness") != "READY_FOR_BOUNDED_REPAIR_REVIEW":
        errors.append("shadow manifest is not ready for bounded repair review")
    if shadow.get("production_applied") is not False or shadow.get("production_mutations") != []:
        errors.append("shadow manifest contains a production mutation claim")
    if shadow.get("source_truncated") is not False or shadow.get("target_truncated") is not False:
        errors.append("truncated shadow scope cannot enter review")
    binding = shadow.get("binding")
    if not isinstance(binding, dict):
        errors.append("shadow binding is missing")
        binding = {}
    expected_binding_sha = shadow_binding_sha256(binding)
    if shadow.get("binding_sha256") != expected_binding_sha or not SHA256_RE.fullmatch(expected_binding_sha):
        errors.append("shadow binding SHA-256 mismatch")
    try:
        captured_at = parse_rfc3339(str(shadow.get("captured_at", "")))
        age_seconds = int((now - captured_at).total_seconds())
        if age_seconds < 0 or age_seconds > int(contract["approval_package"]["maximum_shadow_age_seconds"]):
            errors.append("shadow manifest is expired or captured in the future")
    except (TypeError, ValueError):
        captured_at = None
        age_seconds = None
        errors.append("shadow captured_at is invalid")

    target = binding.get("target") if isinstance(binding.get("target"), dict) else {}
    write_indices = target.get("write_indices") if isinstance(target.get("write_indices"), list) else []
    write_index_names = [
        item.get("index") for item in write_indices
        if isinstance(item, dict) and item.get("is_write_index") is True and isinstance(item.get("index"), str) and item.get("index")
    ]
    if len(write_index_names) != 1:
        errors.append("shadow write alias does not bind exactly one write index")
    if target.get("write_alias") != "alerts-v2-write":
        errors.append("shadow target is not the approved alerts-v2-write alias")
    repairable = int(shadow.get("missing_count", 0) or 0) + int(shadow.get("stale_count", 0) or 0)
    if repairable < 1:
        errors.append("shadow contains no missing or stale projection to repair")
    if repairable > int(contract["execution_guards"]["maximum_documents"]):
        errors.append("repairable shadow delta exceeds the approved document budget")
    differences = binding.get("differences") if isinstance(binding.get("differences"), list) else []
    repair_ids = sorted(
        item["alert_id"] for item in differences
        if isinstance(item, dict) and item.get("classification") in {"missing", "stale"} and isinstance(item.get("alert_id"), str)
    )
    if len(repair_ids) != repairable or len(set(repair_ids)) != len(repair_ids):
        errors.append("repairable difference identities do not match the shadow counts")

    before = g0.get("candidate_before") if isinstance(g0.get("candidate_before"), dict) else {}
    after = g0.get("candidate_after") if isinstance(g0.get("candidate_after"), dict) else {}
    source = g0.get("candidate_source") if isinstance(g0.get("candidate_source"), dict) else {}
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        errors.append("candidate manifest is not a passing G0 result")
    if before.get("head") != after.get("head") or before.get("status") != [] or after.get("status") != []:
        errors.append("G0 candidate was not stable and clean")
    content_sha = source.get("content_sha256")
    if not isinstance(content_sha, str) or not SHA256_RE.fullmatch(content_sha):
        errors.append("G0 candidate content SHA-256 is missing")
    elif content_sha != repository_content_sha256:
        errors.append("G0 candidate content SHA-256 does not match the current repository source snapshot")
    if not SHA256_RE.fullmatch(g0_manifest_sha256) or not SHA256_RE.fullmatch(shadow_file_sha256):
        errors.append("input file SHA-256 binding is invalid")
    if errors:
        raise ValueError("; ".join(errors))

    approvals = {
        "sre": {"status": "PENDING", "approved_by": None, "approved_at": None},
        "qa": {"status": "PENDING", "approved_by": None, "approved_at": None},
        "security": {"status": "PENDING", "approved_by": None, "approved_at": None},
        "domain_accountable": {"status": "PENDING", "approved_by": None, "approved_at": None},
    }
    blockers = ["SRE, QA, security and domain accountable approvals are pending"]
    if immutable_tool_image_digest is None:
        blockers.append("immutable alert-projection-tools image digest is not bound")
    elif not immutable_tool_image_digest.startswith("sha256:") or not SHA256_RE.fullmatch(immutable_tool_image_digest.removeprefix("sha256:")):
        raise ValueError("immutable tool image must use sha256:<64 lowercase hex>")
    argv = [
        "alert-projection-reconcile", "--mode", "repair", "--confirm-repair",
        "--tenant", str(binding["tenant_id"]), "--requested-by", "APPROVED_OPERATOR_REQUIRED",
        "--trace-id", str(shadow["trace_id"]), "--start", str(binding["start_time"]),
        "--end", str(binding["end_time"]), "--alert-ids", ",".join(repair_ids),
        "--target-index-version", str(target["write_alias"]),
        "--expected-cluster-uuid", str(target["cluster_uuid"]),
        "--expected-read-target", str(target["read_target"]),
        "--expected-write-alias", str(target["write_alias"]),
        "--expected-write-index", write_index_names[0],
        "--max-documents", str(repairable),
    ]
    return {
        "schema_version": 1,
        "remediation_ids": ["T-OS-002", "T-OS-004"],
        "mode": "REPAIR_REVIEW_PACKAGE",
        "rendered_at": now.isoformat(),
        "execution_authorized": False,
        "production_applied": False,
        "production_mutations": [],
        "bindings": {
            "g0_run_id": g0.get("run_id"),
            "g0_candidate_head": before.get("head"),
            "g0_candidate_content_sha256": content_sha,
            "rendering_repository_head": repository_head,
            "g0_manifest_sha256": g0_manifest_sha256,
            "shadow_file_sha256": shadow_file_sha256,
            "shadow_binding_sha256": expected_binding_sha,
            "shadow_captured_at": captured_at.isoformat() if captured_at else None,
            "shadow_age_seconds": age_seconds,
            "environment_id": binding.get("environment_id"),
            "tenant_id": binding.get("tenant_id"),
            "start_time": binding.get("start_time"),
            "end_time": binding.get("end_time"),
            "cluster_uuid": target.get("cluster_uuid"),
            "read_target": target.get("read_target"),
            "write_alias": target.get("write_alias"),
            "write_index": write_index_names[0],
            "missing_count": shadow.get("missing_count"),
            "stale_count": shadow.get("stale_count"),
            "extra_count": shadow.get("extra_count"),
            "repair_ids": repair_ids,
            "immutable_tool_image_digest": immutable_tool_image_digest,
        },
        "proposed_execution": {
            "argv": argv,
            "shell": None,
            "maximum_documents": contract["execution_guards"]["maximum_documents"],
            "maximum_repairs_per_second": contract["execution_guards"]["maximum_repairs_per_second"],
            "stop_error_count": contract["execution_guards"]["stop_error_count"],
            "extra_documents_action": "manual_adjudication_only_never_auto_delete",
        },
        "approvals": approvals,
        "blockers": blockers,
        "rollback": {
            "read_alias_switch_in_package": False,
            "legacy_index_retained": True,
            "cancel_on_first_error": True,
            "same_scope_terminal_requery_required": True,
            "postgresql_watermark_receipt_required": True,
            "observation_windows": contract["execution_guards"]["observation_windows"],
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--shadow-manifest", type=Path, required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--immutable-tool-image-digest")
    args = parser.parse_args()
    shadow_path = args.shadow_manifest.resolve()
    g0_path = args.g0_manifest.resolve()
    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite review package: {output}")
    package = build_review_package(
        shadow=load_json(shadow_path), shadow_file_sha256=file_sha256(shadow_path),
        g0=load_json(g0_path), g0_manifest_sha256=file_sha256(g0_path),
        contract=load_json(CONTRACT), repository_head=current_head(ROOT),
        repository_content_sha256=str(build_snapshot()["content_sha256"]),
        now=datetime.now(timezone.utc), immutable_tool_image_digest=args.immutable_tool_image_digest,
    )
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(package, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": "PASS", "output": str(output), "execution_authorized": False,
        "production_mutations": 0, "blockers": package["blockers"],
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

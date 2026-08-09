#!/usr/bin/env python3
"""Render a one-time, immutable and time-bounded T-OS-002 expand run."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
BASE_MANIFEST = ROOT / "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml"
CONTRACT = ROOT / "contracts/opensearch/index-governance.v1.json"
MAX_WINDOW_SECONDS = 14_400
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_time(value: str) -> datetime:
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    parsed = datetime.fromisoformat(normalized)
    if parsed.tzinfo is None:
        raise ValueError("approval times must include a timezone")
    return parsed.astimezone(timezone.utc)


def validate_window(not_before: datetime, expires_at: datetime, now: datetime) -> tuple[int, int]:
    start = int(not_before.timestamp())
    end = int(expires_at.timestamp())
    current = int(now.timestamp())
    if end <= start:
        raise ValueError("approval expiry must be after not-before")
    if end - start > MAX_WINDOW_SECONDS:
        raise ValueError(f"approval window exceeds {MAX_WINDOW_SECONDS} seconds")
    if end <= current:
        raise ValueError("approval window is already expired")
    return start, end


def dns_name(prefix: str, run_id: str) -> str:
    slug = re.sub(r"[^a-z0-9-]+", "-", run_id.lower()).strip("-")
    if not slug:
        raise ValueError("run_id must contain a DNS-label character")
    digest = hashlib.sha256(run_id.encode()).hexdigest()[:10]
    room = 63 - len(prefix) - len(digest) - 2
    return f"{prefix}-{slug[:room].rstrip('-')}-{digest}"


def render_documents(
    documents: list[dict[str, Any]],
    *,
    run_id: str,
    approval_id: str,
    approved_by: str,
    cluster_uuid: str,
    not_before_epoch: int,
    expires_at_epoch: int,
    g0_candidate_sha256: str,
    g0_manifest_sha256: str,
    contract_sha256: str,
) -> list[dict[str, Any]]:
    if not approval_id.strip() or not approved_by.strip() or not cluster_uuid.strip():
        raise ValueError("approval_id, approved_by and cluster_uuid are required")
    for label, value in (
        ("g0 candidate", g0_candidate_sha256),
        ("g0 manifest", g0_manifest_sha256),
        ("contract", contract_sha256),
    ):
        if not SHA256_RE.fullmatch(value):
            raise ValueError(f"{label} SHA-256 is invalid")

    secret_name = dns_name("os-v2-approval", run_id)
    job_name = dns_name("expand-os-alerts-v2", run_id)
    secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {
            "name": secret_name,
            "namespace": "middleware",
            "labels": {
                "traffic.io/remediation-id": "T-OS-002",
                "traffic.io/migration-phase": "expand",
                "traffic.io/run-id": run_id,
            },
            "annotations": {
                "traffic.io/approval-expires-at-epoch": str(expires_at_epoch),
            },
        },
        "immutable": True,
        "type": "Opaque",
        "stringData": {
            "approval_id": approval_id,
            "approved_by": approved_by,
            "approval_nonce": run_id,
            "not_before_epoch": str(not_before_epoch),
            "expires_at_epoch": str(expires_at_epoch),
            "cluster_uuid": cluster_uuid,
            "g0_candidate_sha256": g0_candidate_sha256,
            "g0_manifest_sha256": g0_manifest_sha256,
            "contract_sha256": contract_sha256,
        },
    }

    rendered: list[dict[str, Any]] = []
    job_found = False
    for source in documents:
        item = json.loads(json.dumps(source))
        if item.get("kind") == "Job" and item.get("metadata", {}).get("name") == "expand-opensearch-alerts-v2":
            job_found = True
            item["metadata"]["name"] = job_name
            annotations = item["metadata"].setdefault("annotations", {})
            annotations.update({
                "traffic.io/run-id": run_id,
                "traffic.io/approval-id": approval_id,
                "traffic.io/approval-expires-at-epoch": str(expires_at_epoch),
                "traffic.io/g0-candidate-sha256": g0_candidate_sha256,
                "traffic.io/g0-manifest-sha256": g0_manifest_sha256,
                "traffic.io/contract-sha256": contract_sha256,
            })
            item["spec"]["suspend"] = True
            containers = item["spec"]["template"]["spec"]["containers"]
            for container in containers:
                for env in container.get("env", []):
                    reference = env.get("valueFrom", {}).get("secretKeyRef")
                    if reference and reference.get("name") == "opensearch-alerts-v2-approval":
                        reference["name"] = secret_name
                    if env.get("name") == "EXPECTED_APPROVAL_NONCE":
                        env["value"] = run_id
        rendered.append(item)
    if not job_found:
        raise ValueError("base expand Job was not found")
    return [rendered[0], secret, *rendered[1:]]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--approval-id", required=True)
    parser.add_argument("--approved-by", required=True)
    parser.add_argument("--cluster-uuid", required=True)
    parser.add_argument("--not-before", required=True, help="RFC3339 approval window start")
    parser.add_argument("--expires-at", required=True, help="RFC3339 approval window end")
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite rendered migration: {output}")
    now = datetime.now(timezone.utc)
    try:
        not_before_epoch, expires_at_epoch = validate_window(
            parse_time(args.not_before), parse_time(args.expires_at), now,
        )
    except ValueError as exc:
        raise SystemExit(str(exc)) from exc

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("G0 manifest must be a PASS full candidate")
    g0_candidate = (g0.get("candidate_source") or {}).get("content_sha256", "")
    current_candidate = build_snapshot()["content_sha256"]
    if g0_candidate != current_candidate:
        raise SystemExit("G0 candidate does not match the current source snapshot")

    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    contract_sha = contract["expand_execution"]["contract_sha256"]
    documents = [item for item in yaml.safe_load_all(BASE_MANIFEST.read_text(encoding="utf-8")) if item]
    rendered = render_documents(
        documents,
        run_id=args.run_id,
        approval_id=args.approval_id,
        approved_by=args.approved_by,
        cluster_uuid=args.cluster_uuid,
        not_before_epoch=not_before_epoch,
        expires_at_epoch=expires_at_epoch,
        g0_candidate_sha256=g0_candidate,
        g0_manifest_sha256=sha256(g0_path),
        contract_sha256=contract_sha,
    )
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(yaml.safe_dump_all(rendered, sort_keys=False), encoding="utf-8")
    print(json.dumps({
        "status": "RENDERED_SUSPENDED",
        "run_id": args.run_id,
        "output": str(output),
        "output_sha256": sha256(output),
        "not_before_epoch": not_before_epoch,
        "expires_at_epoch": expires_at_epoch,
        "g0_candidate_sha256": g0_candidate,
        "g0_manifest_sha256": sha256(g0_path),
        "production_mutations": [],
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

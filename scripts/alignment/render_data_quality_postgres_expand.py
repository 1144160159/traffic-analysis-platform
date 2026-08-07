#!/usr/bin/env python3
"""Render an immutable, suspended and approval-bound T-DQ-001 PG expand Job."""

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
MIGRATIONS = (
    "202608041400_data_quality_control_plane_v1.sql",
    "202608041500_data_quality_governance_v1.sql",
    "202608041600_data_quality_rule_evaluation_v1.sql",
    "202608041700_data_quality_repair_lifecycle_v1.sql",
    "202608041800_data_quality_replay_projection_v1.sql",
)
MIGRATION_DIR = ROOT / "deployments/postgres/migrations"
MIGRATION_VERSIONS = tuple(item.split("_", 1)[0] for item in MIGRATIONS)
EXPECTED_TABLES = (
    "data_quality_datasets", "data_quality_rules", "data_quality_baselines",
    "data_quality_watermarks", "data_quality_events", "data_quality_repairs",
    "data_quality_outbox", "data_quality_dataset_history", "data_quality_rule_history",
    "data_quality_command_requests", "data_quality_rule_evaluations",
    "data_quality_repair_history", "data_quality_repair_requests",
    "data_quality_flow_replay_projection", "data_quality_replay_projection_receipts",
)
MAX_WINDOW_SECONDS = 14_400
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
SYSTEM_ID_RE = re.compile(r"^[0-9]+$")


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


def dns_name(prefix: str, run_id: str) -> str:
    slug = re.sub(r"[^a-z0-9-]+", "-", run_id.lower()).strip("-")
    if not slug:
        raise ValueError("run_id must contain a DNS-label character")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:10]
    room = 63 - len(prefix) - len(suffix) - 2
    return f"{prefix}-{slug[:room].rstrip('-')}-{suffix}"


def migration_bundle() -> tuple[dict[str, str], str]:
    payloads: dict[str, str] = {}
    aggregate = hashlib.sha256()
    for name in MIGRATIONS:
        path = MIGRATION_DIR / name
        text = path.read_text(encoding="utf-8")
        payloads[name] = text
        aggregate.update(name.encode())
        aggregate.update(b"\0")
        aggregate.update(hashlib.sha256(text.encode()).hexdigest().encode())
        aggregate.update(b"\n")
    return payloads, aggregate.hexdigest()


def validate_state(value: str) -> str:
    parts = value.split(",")
    if len(parts) != len(MIGRATIONS) or any(part not in {"0", "1"} for part in parts):
        raise ValueError("expected migration state must contain five comma-separated 0/1 values")
    return value


def render(args: argparse.Namespace, now: datetime) -> list[dict[str, Any]]:
    if not args.approval_id.strip() or not args.approved_by.strip():
        raise ValueError("approval_id and approved_by are required")
    if not SYSTEM_ID_RE.fullmatch(args.postgres_system_identifier):
        raise ValueError("postgres system identifier must be numeric")
    not_before = int(parse_time(args.not_before).timestamp())
    expires_at = int(parse_time(args.expires_at).timestamp())
    if expires_at <= not_before or expires_at - not_before > MAX_WINDOW_SECONDS:
        raise ValueError("approval window must be positive and no longer than four hours")
    if expires_at <= int(now.timestamp()):
        raise ValueError("approval window is already expired")
    expected_state = validate_state(args.expected_migration_state)

    g0_path = args.g0_manifest.resolve()
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise ValueError("G0 manifest must be PASS")
    candidate = (g0.get("candidate_source") or {}).get("content_sha256", "")
    if not SHA256_RE.fullmatch(candidate) or candidate != build_snapshot()["content_sha256"]:
        raise ValueError("G0 candidate does not match the current source snapshot")

    payloads, bundle_sha = migration_bundle()
    run_id = args.run_id
    config_name = dns_name("dq-expand-sql", run_id)
    secret_name = dns_name("dq-expand-approval", run_id)
    job_name = dns_name("expand-data-quality", run_id)
    annotations = {
        "traffic.io/run-id": run_id,
        "traffic.io/approval-id": args.approval_id,
        "traffic.io/g0-candidate-sha256": candidate,
        "traffic.io/g0-manifest-sha256": sha256(g0_path),
        "traffic.io/migration-bundle-sha256": bundle_sha,
        "traffic.io/approval-expires-at-epoch": str(expires_at),
    }
    labels = {
        "traffic.io/remediation-id": "T-DQ-001",
        "traffic.io/migration-phase": "expand",
        "traffic.io/run-id": dns_name("run", run_id),
    }
    config_map = {
        "apiVersion": "v1", "kind": "ConfigMap",
        "metadata": {"name": config_name, "namespace": "databases", "labels": labels, "annotations": annotations},
        "immutable": True, "data": payloads,
    }
    secret = {
        "apiVersion": "v1", "kind": "Secret",
        "metadata": {"name": secret_name, "namespace": "databases", "labels": labels, "annotations": annotations},
        "immutable": True, "type": "Opaque",
        "stringData": {
            "approval_id": args.approval_id, "approved_by": args.approved_by,
            "approval_nonce": run_id, "not_before_epoch": str(not_before), "expires_at_epoch": str(expires_at),
            "postgres_system_identifier": args.postgres_system_identifier,
            "expected_migration_state": expected_state, "g0_candidate_sha256": candidate,
            "g0_manifest_sha256": sha256(g0_path), "migration_bundle_sha256": bundle_sha,
        },
    }
    version_sql = " UNION ALL ".join(
        f"SELECT '{version}',count(*)::text FROM alignment_schema_migrations WHERE version='{version}'"
        for version in MIGRATION_VERSIONS
    )
    table_sql = ",".join(f"'{name}'" for name in EXPECTED_TABLES)
    migration_loop = " ".join(MIGRATIONS)
    shell = f'''set -euo pipefail
now="$(date +%s)"
test "$APPROVED_CHANGE_ID" = "$EXPECTED_CHANGE_ID"
test "$APPROVED_BY" = "$EXPECTED_APPROVER"
test "$CHANGE_APPROVAL_NONCE" = "$EXPECTED_APPROVAL_NONCE"
test "$APPROVED_G0_CANDIDATE_SHA256" = "$EXPECTED_G0_CANDIDATE_SHA256"
test "$APPROVED_G0_MANIFEST_SHA256" = "$EXPECTED_G0_MANIFEST_SHA256"
test "$APPROVED_MIGRATION_BUNDLE_SHA256" = "$EXPECTED_MIGRATION_BUNDLE_SHA256"
test "$now" -ge "$CHANGE_WINDOW_NOT_BEFORE_EPOCH"
test "$now" -lt "$CHANGE_WINDOW_EXPIRES_AT_EPOCH"
test $((CHANGE_WINDOW_EXPIRES_AT_EPOCH-CHANGE_WINDOW_NOT_BEFORE_EPOCH)) -le {MAX_WINDOW_SECONDS}
actual_system_id="$(psql -v ON_ERROR_STOP=1 -h postgres-primary.databases.svc -U postgres -d traffic_platform -Atqc "SELECT system_identifier::text FROM pg_control_system()")"
test "$actual_system_id" = "$APPROVED_POSTGRES_SYSTEM_IDENTIFIER"
actual_state="$(psql -v ON_ERROR_STOP=1 -h postgres-primary.databases.svc -U postgres -d traffic_platform -Atqc "{version_sql} ORDER BY 1" | cut -d'|' -f2 | paste -sd, -)"
test "$actual_state" = "$APPROVED_EXPECTED_MIGRATION_STATE"
for file in {migration_loop}; do
  psql -v ON_ERROR_STOP=1 -h postgres-primary.databases.svc -U postgres -d traffic_platform -f "/migrations/$file"
done
final_state="$(psql -v ON_ERROR_STOP=1 -h postgres-primary.databases.svc -U postgres -d traffic_platform -Atqc "{version_sql} ORDER BY 1" | cut -d'|' -f2 | paste -sd, -)"
test "$final_state" = "1,1,1,1,1"
table_count="$(psql -v ON_ERROR_STOP=1 -h postgres-primary.databases.svc -U postgres -d traffic_platform -Atqc "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename IN ({table_sql})")"
test "$table_count" = "{len(EXPECTED_TABLES)}"
'''
    secret_env = {
        "APPROVED_CHANGE_ID": "approval_id", "APPROVED_BY": "approved_by",
        "CHANGE_APPROVAL_NONCE": "approval_nonce", "CHANGE_WINDOW_NOT_BEFORE_EPOCH": "not_before_epoch",
        "CHANGE_WINDOW_EXPIRES_AT_EPOCH": "expires_at_epoch", "APPROVED_POSTGRES_SYSTEM_IDENTIFIER": "postgres_system_identifier",
        "APPROVED_EXPECTED_MIGRATION_STATE": "expected_migration_state", "APPROVED_G0_CANDIDATE_SHA256": "g0_candidate_sha256",
        "APPROVED_G0_MANIFEST_SHA256": "g0_manifest_sha256", "APPROVED_MIGRATION_BUNDLE_SHA256": "migration_bundle_sha256",
    }
    env = [
        {"name": name, "valueFrom": {"secretKeyRef": {"name": secret_name, "key": key}}}
        for name, key in secret_env.items()
    ] + [
        {"name": "EXPECTED_CHANGE_ID", "value": args.approval_id},
        {"name": "EXPECTED_APPROVER", "value": args.approved_by},
        {"name": "EXPECTED_APPROVAL_NONCE", "value": run_id},
        {"name": "EXPECTED_G0_CANDIDATE_SHA256", "value": candidate},
        {"name": "EXPECTED_G0_MANIFEST_SHA256", "value": sha256(g0_path)},
        {"name": "EXPECTED_MIGRATION_BUNDLE_SHA256", "value": bundle_sha},
        {"name": "PGPASSWORD", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "PG_PASSWORD"}}},
    ]
    job = {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {"name": job_name, "namespace": "databases", "labels": labels, "annotations": annotations},
        "spec": {"suspend": True, "backoffLimit": 0, "ttlSecondsAfterFinished": 86400,
                 "template": {"metadata": {"labels": labels}, "spec": {"restartPolicy": "Never", "containers": [{
                     "name": "expand", "image": "docker.io/library/postgres@sha256:3a5291c749cd77894b6d3e6e2135b81a29b1e864026e6d899ab01bc3656c6bc7",
                     "command": ["bash", "-c"], "args": [shell], "env": env,
                     "volumeMounts": [{"name": "migrations", "mountPath": "/migrations", "readOnly": True}],
                     "resources": {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "500m", "memory": "256Mi"}},
                 }], "volumes": [{"name": "migrations", "configMap": {"name": config_name}}]}}},
    }
    return [config_map, secret, job]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--approval-id", required=True)
    parser.add_argument("--approved-by", required=True)
    parser.add_argument("--postgres-system-identifier", required=True)
    parser.add_argument("--expected-migration-state", default="0,0,0,0,0")
    parser.add_argument("--not-before", required=True)
    parser.add_argument("--expires-at", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite rendered migration: {output}")
    try:
        documents = render(args, datetime.now(timezone.utc))
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        raise SystemExit(str(exc)) from exc
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(yaml.safe_dump_all(documents, sort_keys=False), encoding="utf-8")
    print(json.dumps({"status": "RENDERED_SUSPENDED", "output": str(output), "output_sha256": sha256(output), "production_mutations": []}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

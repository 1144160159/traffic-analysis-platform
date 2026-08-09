#!/usr/bin/env python3
"""Capture immutable F-PROBE G2 expand/backfill evidence without exposing secrets."""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
MIGRATION = ROOT / "deployments/postgres/migrations/202607302015_probe_operation_ack.sql"
TOPICS = {
    "probe.control.v2": {"partitions": 6, "replication_factor": 3},
    "probe.acks.v2": {"partitions": 6, "replication_factor": 3},
    "dlq.probe.acks.v2": {"partitions": 3, "replication_factor": 3},
    "probe.events.v2": {"partitions": 6, "replication_factor": 3},
}
PG_TABLES = {
    "probe_operation_ack_receipts",
    "probe_operation_history",
    "probe_operation_outbox",
}


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _write_json(path: Path, value: Any) -> None:
    path.write_text(
        json.dumps(value, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )


def _run(command: list[str], *, check: bool = True) -> subprocess.CompletedProcess[str]:
    environment = os.environ.copy()
    environment.pop("HTTP_PROXY", None)
    environment.pop("HTTPS_PROXY", None)
    completed = subprocess.run(
        command,
        cwd=ROOT,
        env=environment,
        text=True,
        capture_output=True,
        check=False,
    )
    if check and completed.returncode != 0:
        raise RuntimeError(
            f"command failed ({completed.returncode}): {' '.join(command)}\n"
            f"{completed.stderr.strip()}"
        )
    return completed


def _kubectl(*args: str) -> str:
    return _run(["kubectl", "--request-timeout=30s", *args]).stdout.strip()


def _json_output(command: list[str]) -> Any:
    output = _run(command).stdout.strip()
    return json.loads(output)


def _capture_topics() -> dict[str, Any]:
    response = _kubectl(
        "exec",
        "-n",
        "middleware",
        "kafka-ui-0",
        "--",
        "sh",
        "-c",
        "wget -qO- "
        "'http://127.0.0.1:8080/kafka-ui/api/clusters/traffic-platform/"
        "topics?page=0&perPage=100&showInternal=false'",
    )
    payload = json.loads(response)
    selected = {
        item["name"]: item for item in payload.get("topics", []) if item.get("name") in TOPICS
    }
    checks = {}
    for name, expected in TOPICS.items():
        item = selected.get(name)
        checks[name] = {
            "present": item is not None,
            "partitions_match": bool(item)
            and item.get("partitionCount") == expected["partitions"],
            "replication_factor_match": bool(item)
            and item.get("replicationFactor") == expected["replication_factor"],
            "full_isr": bool(item)
            and item.get("inSyncReplicas") == item.get("replicas"),
            "under_replicated_partitions": item.get("underReplicatedPartitions")
            if item
            else None,
        }
    return {
        "cluster": "traffic-platform",
        "topics": selected,
        "checks": checks,
        "status": "PASS"
        if all(
            all(
                (
                    item["present"],
                    item["partitions_match"],
                    item["replication_factor_match"],
                    item["full_isr"],
                    item["under_replicated_partitions"] == 0,
                )
            )
            for item in checks.values()
        )
        else "FAIL",
    }


def _psql(pod: str, sql: str) -> Any:
    output = _kubectl(
        "exec",
        "-n",
        "databases",
        pod,
        "--",
        "psql",
        "-U",
        "postgres",
        "-d",
        "traffic_platform",
        "-Atqc",
        sql,
    )
    return json.loads(output)


def _capture_postgres() -> dict[str, Any]:
    primary_sql = """
SELECT json_build_object(
  'migration_record',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202607302015'),
  'operation_total',(SELECT count(*) FROM probe_operations),
  'status_counts',(SELECT COALESCE(json_object_agg(status,n),'{}'::json) FROM (SELECT status,count(*) n FROM probe_operations GROUP BY status) s),
  'revision_zero',(SELECT count(*) FROM probe_operations WHERE command_revision=0),
  'revision_duplicates',(SELECT count(*) FROM (SELECT tenant_id,probe_id,command_revision FROM probe_operations GROUP BY 1,2,3 HAVING count(*)>1) d),
  'tables',(SELECT COALESCE(json_agg(tablename ORDER BY tablename),'[]'::json) FROM pg_tables WHERE schemaname='public' AND tablename IN ('probe_operation_ack_receipts','probe_operation_history','probe_operation_outbox')),
  'ack_receipts',(SELECT count(*) FROM probe_operation_ack_receipts),
  'history_rows',(SELECT count(*) FROM probe_operation_history),
  'pending_outbox',(SELECT count(*) FROM probe_operation_outbox WHERE published=false)
)::text;
"""
    replica_sql = """
SELECT json_build_object(
  'migration_record',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202607302015'),
  'tables',(SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename IN ('probe_operation_ack_receipts','probe_operation_history','probe_operation_outbox')),
  'in_recovery',pg_is_in_recovery()
)::text;
"""
    lag_sql = """
SELECT COALESCE(json_agg(json_build_object(
  'application_name',application_name,
  'state',state,
  'sync_state',sync_state,
  'replay_lag_bytes',pg_wal_lsn_diff(pg_current_wal_lsn(),replay_lsn)
) ORDER BY application_name),'[]'::json)::text FROM pg_stat_replication;
"""
    primary = _psql("postgres-primary-0", primary_sql)
    replicas = {
        pod: _psql(pod, replica_sql)
        for pod in ("postgres-replica-0", "postgres-replica-1")
    }
    replication = _psql("postgres-primary-0", lag_sql)
    primary_ok = (
        primary["migration_record"] == 1
        and primary["operation_total"] > 0
        and primary["revision_zero"] == 0
        and primary["revision_duplicates"] == 0
        and set(primary["tables"]) == PG_TABLES
        and int(primary["status_counts"].get("queued", 0)) == 0
    )
    replicas_ok = all(
        item["migration_record"] == 1
        and item["tables"] == len(PG_TABLES)
        and item["in_recovery"] is True
        for item in replicas.values()
    )
    replication_ok = (
        len(replication) == 2
        and all(item["state"] == "streaming" for item in replication)
        and all(int(item["replay_lag_bytes"]) == 0 for item in replication)
    )
    return {
        "database": "traffic_platform",
        "primary": primary,
        "replicas": replicas,
        "replication": replication,
        "checks": {
            "primary_expand_backfill": primary_ok,
            "replicas_replayed": replicas_ok,
            "replication_zero_byte_lag": replication_ok,
        },
        "status": "PASS" if primary_ok and replicas_ok and replication_ok else "FAIL",
    }


def _capture_credentials() -> dict[str, Any]:
    source = json.loads(
        _kubectl(
            "get",
            "secret",
            "traffic-platform-prod-credentials",
            "-n",
            "external-secrets-source",
            "-o",
            "json",
        )
    )
    external = json.loads(
        _kubectl(
            "get",
            "externalsecret",
            "traffic-credentials",
            "-n",
            "traffic-analysis",
            "-o",
            "json",
        )
    )
    target = json.loads(
        _kubectl(
            "get",
            "secret",
            "traffic-credentials",
            "-n",
            "traffic-analysis",
            "-o",
            "json",
        )
    )
    encoded_probe_token = target.get("data", {}).get("PROBE_AUTH_TOKEN", "")
    token_digest = ""
    if encoded_probe_token:
        token_digest = hashlib.sha256(
            base64.b64decode(encoded_probe_token, validate=True)
        ).hexdigest()
    redis_exists = "0"
    if token_digest:
        redis_exists = _kubectl(
            "exec",
            "-n",
            "databases",
            "redis-master-0",
            "--",
            "redis-cli",
            "EXISTS",
            f"session:probe-agent:{token_digest}",
        ).splitlines()[-1]
    ready = any(
        condition.get("type") == "Ready" and condition.get("status") == "True"
        for condition in external.get("status", {}).get("conditions", [])
    )
    mapped = any(
        item.get("secretKey") == "PROBE_AUTH_TOKEN"
        for item in external.get("spec", {}).get("data", [])
    )
    result = {
        "source_has_probe_auth": bool(source.get("data", {}).get("PROBE_AUTH_TOKEN")),
        "external_secret_maps_probe_auth": mapped,
        "external_secret_ready": ready,
        "target_has_probe_auth": bool(target.get("data", {}).get("PROBE_AUTH_TOKEN")),
        "redis_token_cache_entry_present": redis_exists.strip() == "1",
        "secret_values_captured": False,
    }
    result["status"] = "PASS" if all(
        value for key, value in result.items() if key not in {"secret_values_captured"}
    ) else "FAIL"
    return result


def _capture_workloads() -> dict[str, Any]:
    deployments = json.loads(
        _kubectl(
            "get",
            "deployment",
            "ingest-gateway",
            "alert-service",
            "-n",
            "traffic-analysis",
            "-o",
            "json",
        )
    )
    daemonset = json.loads(
        _kubectl(
            "get",
            "daemonset",
            "probe-agent",
            "-n",
            "traffic-analysis",
            "-o",
            "json",
        )
    )
    items = {}
    for item in deployments["items"] + [daemonset]:
        container = item["spec"]["template"]["spec"]["containers"][0]
        items[item["metadata"]["name"]] = {
            "kind": item["kind"],
            "image": container["image"],
            "env_names": [entry["name"] for entry in container.get("env", [])],
            "desired": item.get("spec", {}).get("replicas")
            if item["kind"] == "Deployment"
            else item.get("status", {}).get("desiredNumberScheduled"),
            "ready": item.get("status", {}).get("readyReplicas")
            if item["kind"] == "Deployment"
            else item.get("status", {}).get("numberReady"),
        }
    return {
        "items": items,
        "note": "This is the pre-cutover production baseline; candidate deployment is intentionally excluded from the expand gate.",
    }


def _capture_local_images(image_refs: list[str]) -> dict[str, Any]:
    if not image_refs:
        return {"status": "NOT_REQUESTED", "images": []}
    completed = _run(["docker", "image", "inspect", *image_refs], check=False)
    if completed.returncode != 0:
        return {
            "status": "FAIL",
            "images": [],
            "error": completed.stderr.strip(),
        }
    payload = json.loads(completed.stdout)
    return {
        "status": "PASS",
        "images": [
            {
                "id": item["Id"],
                "repo_tags": item.get("RepoTags", []),
                "size_bytes": item.get("Size"),
                "source_revision": item.get("Config", {})
                .get("Labels", {})
                .get("org.opencontainers.image.revision"),
                "entrypoint": item.get("Config", {}).get("Entrypoint"),
                "cmd": item.get("Config", {}).get("Cmd"),
            }
            for item in payload
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    parser.add_argument("--image", action="append", default=[])
    args = parser.parse_args()

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    captured_at = datetime.now(timezone.utc).isoformat()
    candidate = build_snapshot()
    cluster_ready = _kubectl("get", "--raw=/readyz?verbose")
    topics = _capture_topics()
    postgres = _capture_postgres()
    credentials = _capture_credentials()
    workloads = _capture_workloads()
    images = _capture_local_images(args.image)

    _write_json(output / "candidate-source.json", candidate)
    _write_json(output / "kafka-topics.json", topics)
    _write_json(output / "postgres-expand.json", postgres)
    _write_json(output / "probe-auth-secret-expand.json", credentials)
    _write_json(output / "workload-baseline.json", workloads)
    _write_json(output / "candidate-images.json", images)
    (output / "kubernetes-readyz.txt").write_text(cluster_ready + "\n", encoding="utf-8")

    statuses = {
        "kubernetes_ready": "PASS" if "readyz check passed" in cluster_ready else "FAIL",
        "kafka_topics": topics["status"],
        "postgres_expand_backfill": postgres["status"],
        "probe_auth_secret_expand": credentials["status"],
        "candidate_images": images["status"],
    }
    required = {key: value for key, value in statuses.items() if key != "candidate_images"}
    status = "PASS" if all(value == "PASS" for value in required.values()) else "FAIL"
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": captured_at,
        "feature_id": "F-PROBE-001",
        "gate": "G2_EXPAND_PREREQUISITES",
        "status": status,
        "checks": statuses,
        "candidate_source_sha256": candidate["content_sha256"],
        "migration": {
            "path": MIGRATION.relative_to(ROOT).as_posix(),
            "sha256": _sha256(MIGRATION),
        },
        "scope": {
            "included": [
                "Kubernetes API readiness",
                "Kafka topic existence, partitioning, replication and ISR",
                "PostgreSQL expand/backfill invariants and replica replay",
                "ExternalSecret probe credential expand without secret values",
                "pre-cutover workload image baseline",
                "optional local candidate image metadata",
            ],
            "excluded": [
                "candidate workload deployment",
                "end-to-end command execution and ACK reconciliation",
                "fault injection and recovery",
                "Windows Chrome acceptance",
                "performance and release/rollback gates",
                "G7 and G8",
            ],
        },
        "g2_status": "OPEN",
        "g3_status": "OPEN",
        "g7_status": "OPEN",
        "g8_status": "BLOCKED",
    }
    manifest["artifacts"] = [
        {
            "path": path.name,
            "sha256": _sha256(path),
            "size_bytes": path.stat().st_size,
        }
        for path in sorted(output.iterdir())
        if path.is_file() and path.name != "manifest.json"
    ]
    _write_json(output / "manifest.json", manifest)
    print(
        json.dumps(
            {
                "status": status,
                "manifest": str(output / "manifest.json"),
                "manifest_sha256": _sha256(output / "manifest.json"),
                "g2_status": "OPEN",
                "g7_status": "OPEN",
                "g8_status": "BLOCKED",
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

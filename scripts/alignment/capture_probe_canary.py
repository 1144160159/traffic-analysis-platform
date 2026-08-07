#!/usr/bin/env python3
"""Capture immutable F-PROBE G2/G3 canary evidence without secret values."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
CANARY_LABEL = "alignment.traffic.io/canary=probe-control-g2"
UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-"
    r"[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)


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


def _capture_images(image_refs: list[str], expected_source: str) -> dict[str, Any]:
    completed = _run(["docker", "image", "inspect", *image_refs], check=False)
    if completed.returncode != 0:
        return {
            "status": "FAIL",
            "error": completed.stderr.strip(),
            "images": [],
        }
    images = []
    for item in json.loads(completed.stdout):
        labels = item.get("Config", {}).get("Labels", {})
        images.append(
            {
                "id": item["Id"],
                "repo_tags": item.get("RepoTags", []),
                "size_bytes": item.get("Size"),
                "source_revision": labels.get("org.opencontainers.image.revision"),
                "entrypoint": item.get("Config", {}).get("Entrypoint"),
                "cmd": item.get("Config", {}).get("Cmd"),
            }
        )
    revisions_match = (
        len(images) == len(image_refs)
        and all(item["source_revision"] == expected_source for item in images)
    )
    return {
        "status": "PASS" if revisions_match else "FAIL",
        "checks": {
            "all_images_present": len(images) == len(image_refs),
            "source_revisions_match": revisions_match,
        },
        "images": images,
    }


def _capture_workloads(
    expected_source: str,
    expected_images: set[str],
) -> dict[str, Any]:
    deployments = json.loads(
        _kubectl(
            "get",
            "deployment",
            "ingest-gateway-probe-canary",
            "probe-agent-g2-canary",
            "-n",
            "traffic-analysis",
            "-o",
            "json",
        )
    )
    job = json.loads(
        _kubectl(
            "get",
            "job",
            "probe-control-g2-smoke",
            "-n",
            "traffic-analysis",
            "-o",
            "json",
        )
    )
    pods = json.loads(
        _kubectl(
            "get",
            "pods",
            "-n",
            "traffic-analysis",
            "-l",
            CANARY_LABEL,
            "-o",
            "json",
        )
    )

    workload_items = deployments.get("items", []) + [job]
    source_annotations = {
        item["metadata"]["name"]: item.get("metadata", {})
        .get("annotations", {})
        .get("alignment.traffic.io/source-sha256")
        for item in workload_items
    }
    configured_images = {
        container["image"]
        for item in workload_items
        for container in item["spec"]["template"]["spec"].get("containers", [])
    }
    deployment_ready = all(
        item.get("status", {}).get("readyReplicas", 0)
        == item.get("spec", {}).get("replicas", 0)
        == 1
        for item in deployments.get("items", [])
    )
    job_complete = any(
        condition.get("type") == "Complete" and condition.get("status") == "True"
        for condition in job.get("status", {}).get("conditions", [])
    )
    job_unsuspended = job.get("spec", {}).get("suspend") is False
    pod_items = []
    for pod in pods.get("items", []):
        statuses = pod.get("status", {}).get("containerStatuses", [])
        pod_items.append(
            {
                "name": pod["metadata"]["name"],
                "node": pod.get("spec", {}).get("nodeName"),
                "phase": pod.get("status", {}).get("phase"),
                "source_revision": pod.get("metadata", {})
                .get("annotations", {})
                .get("alignment.traffic.io/source-sha256"),
                "containers": [
                    {
                        "name": status["name"],
                        "image": status["image"],
                        "image_id": status.get("imageID"),
                        "ready": status.get("ready"),
                        "restart_count": status.get("restartCount"),
                    }
                    for status in statuses
                ],
            }
        )
    source_match = (
        source_annotations
        and all(value == expected_source for value in source_annotations.values())
        and all(item["source_revision"] == expected_source for item in pod_items)
    )
    images_match = configured_images == expected_images
    status = "PASS" if all(
        (deployment_ready, job_complete, job_unsuspended, source_match, images_match)
    ) else "FAIL"
    return {
        "status": status,
        "checks": {
            "deployments_ready": deployment_ready,
            "job_complete": job_complete,
            "job_unsuspended": job_unsuspended,
            "source_annotations_match": source_match,
            "configured_images_match": images_match,
        },
        "source_annotations": source_annotations,
        "configured_images": sorted(configured_images),
        "pods": pod_items,
        "job": {
            "suspend": job.get("spec", {}).get("suspend"),
            "active": job.get("status", {}).get("active", 0),
            "succeeded": job.get("status", {}).get("succeeded", 0),
            "failed": job.get("status", {}).get("failed", 0),
            "conditions": job.get("status", {}).get("conditions", []),
        },
    }


def _parse_smoke_output(log_text: str) -> dict[str, Any]:
    for line in reversed(log_text.splitlines()):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if value.get("gate") == "G2_G3_CANARY":
            return value
    raise RuntimeError("smoke Job log does not contain the G2_G3_CANARY result")


def _psql_json(sql: str) -> Any:
    output = _kubectl(
        "exec",
        "-n",
        "databases",
        "postgres-primary-0",
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


def _capture_postgres(smoke: dict[str, Any]) -> dict[str, Any]:
    operation = smoke.get("operation", {})
    operation_id = str(operation.get("operation_id", "")).lower()
    if not UUID_RE.fullmatch(operation_id):
        raise RuntimeError("smoke output contains an invalid operation_id")
    sql = f"""
SELECT json_build_object(
  'operation',(
    SELECT json_build_object(
      'operation_id',operation_id::text,
      'tenant_id',tenant_id,
      'probe_id',probe_id,
      'status',status,
      'command_revision',command_revision,
      'state_revision',state_revision,
      'command_hash',command_hash,
      'reported_hash',reported_hash,
      'agent_version',agent_version,
      'acknowledged_at',acknowledged_at IS NOT NULL
    ) FROM probe_operations WHERE operation_id='{operation_id}'::uuid
  ),
  'history',(
    SELECT COALESCE(json_agg(json_build_object(
      'state_revision',state_revision,
      'from_status',from_status,
      'to_status',to_status,
      'created_at',created_at
    ) ORDER BY state_revision),'[]'::json)
    FROM probe_operation_history WHERE operation_id='{operation_id}'::uuid
  ),
  'ack_receipt',(
    SELECT json_build_object(
      'ack_id',ack_id::text,
      'tenant_id',tenant_id,
      'probe_id',probe_id,
      'command_revision',command_revision,
      'reported_hash',reported_hash,
      'agent_version',agent_version,
      'applied',applied,
      'accepted',accepted,
      'rejection_reason',rejection_reason,
      'acknowledged_at',acknowledged_at
    ) FROM probe_operation_ack_receipts WHERE operation_id='{operation_id}'::uuid
  ),
  'outbox',(
    SELECT COALESCE(json_agg(json_build_object(
      'event_id',event_id::text,
      'event_type',event_type,
      'aggregate_version',aggregate_version,
      'schema_version',schema_version,
      'partition_key',partition_key,
      'published',published,
      'attempts',attempts,
      'published_at',published_at
    ) ORDER BY aggregate_version),'[]'::json)
    FROM probe_operation_outbox WHERE operation_id='{operation_id}'::uuid
  ),
  'audit',(
    SELECT COALESCE(json_agg(json_build_object(
      'event_id',event_id,
      'action',action,
      'object_type',object_type,
      'object_id',object_id,
      'trace_id',trace_id,
      'success',success,
      'result',result,
      'created_at',created_at
    ) ORDER BY created_at),'[]'::json)
    FROM audit_logs
    WHERE tenant_id='default' AND detail->>'operation_id'='{operation_id}'
  )
)::text;
"""
    captured = _psql_json(sql)
    pg_operation = captured.get("operation") or {}
    ack = captured.get("ack_receipt") or {}
    outbox = captured.get("outbox") or []
    history = captured.get("history") or []
    audit = captured.get("audit") or []
    checks = {
        "operation_matches_smoke": (
            pg_operation.get("operation_id") == operation_id
            and pg_operation.get("tenant_id") == operation.get("tenant_id")
            and pg_operation.get("probe_id") == operation.get("probe_id")
            and pg_operation.get("status") == operation.get("status") == "completed"
            and pg_operation.get("command_revision")
            == operation.get("command_revision")
        ),
        "accepted_ack_matches_operation": (
            ack.get("accepted") is True
            and ack.get("applied") is True
            and ack.get("ack_id") == operation.get("ack_event_id")
            and ack.get("command_revision") == operation.get("command_revision")
            and ack.get("reported_hash") == pg_operation.get("reported_hash")
        ),
        "ordered_history_completed": (
            len(history) >= 2
            and [item["state_revision"] for item in history]
            == sorted({item["state_revision"] for item in history})
            and history[-1].get("to_status") == "completed"
        ),
        "outbox_fully_published": (
            len(outbox) >= 2
            and all(item.get("published") is True for item in outbox)
            and all(item.get("schema_version") == 2 for item in outbox)
        ),
        "audit_present": len(audit) >= 1,
    }
    captured["checks"] = checks
    captured["status"] = "PASS" if all(checks.values()) else "FAIL"
    captured["payload_values_captured"] = False
    return captured


def _capture_redis(operation_id: str) -> dict[str, Any]:
    exists = _kubectl(
        "exec",
        "-n",
        "databases",
        "redis-master-0",
        "--",
        "redis-cli",
        "HEXISTS",
        "probe_control:v2:default:probe-agent",
        operation_id,
    ).splitlines()[-1]
    absent = exists.strip() == "0"
    return {
        "status": "PASS" if absent else "FAIL",
        "command_key": "probe_control:v2:default:probe-agent",
        "operation_field_absent_after_accepted_ack": absent,
        "command_payload_captured": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--source-sha256", required=True)
    parser.add_argument("--ingest-image", required=True)
    parser.add_argument("--probe-image", required=True)
    parser.add_argument("--smoke-image", required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    if not re.fullmatch(r"[0-9a-f]{64}", args.source_sha256):
        raise SystemExit("source SHA-256 must contain exactly 64 lowercase hex characters")
    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    candidate = build_snapshot()
    if candidate["content_sha256"] != args.source_sha256:
        raise SystemExit(
            "current candidate source does not match --source-sha256; refusing mixed evidence"
        )
    image_refs = [args.ingest_image, args.probe_image, args.smoke_image]
    images = _capture_images(image_refs, args.source_sha256)
    workloads = _capture_workloads(args.source_sha256, set(image_refs))
    smoke_log = _kubectl(
        "logs",
        "job/probe-control-g2-smoke",
        "-n",
        "traffic-analysis",
    )
    smoke = _parse_smoke_output(smoke_log)
    smoke_checks = smoke.get("checks", {})
    smoke_status = (
        "PASS"
        if smoke.get("status") == "PASS"
        and smoke_checks
        and all(value is True for value in smoke_checks.values())
        else "FAIL"
    )
    postgres = _capture_postgres(smoke)
    operation_id = smoke["operation"]["operation_id"]
    redis = _capture_redis(operation_id)

    _write_json(output / "candidate-source.json", candidate)
    _write_json(output / "candidate-images.json", images)
    _write_json(output / "canary-workloads.json", workloads)
    _write_json(output / "smoke-result.json", smoke)
    _write_json(output / "postgres-reconcile.json", postgres)
    _write_json(output / "redis-reconcile.json", redis)
    (output / "smoke-job.log").write_text(smoke_log + "\n", encoding="utf-8")

    checks = {
        "candidate_images": images["status"],
        "canary_workloads": workloads["status"],
        "smoke_business_loop": smoke_status,
        "postgres_reconcile": postgres["status"],
        "redis_reconcile": redis["status"],
    }
    status = "PASS" if all(value == "PASS" for value in checks.values()) else "FAIL"
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-PROBE-001",
        "gate": "G2_G3_CANARY",
        "status": status,
        "checks": checks,
        "candidate_source_sha256": args.source_sha256,
        "operation_id": operation_id,
        "trace_id": smoke.get("trace_id"),
        "scope": {
            "included": [
                "isolated single-node canary Agent and ingest Gateway",
                "official connectivity-test HTTP handler acceptance",
                "transactional PG state, audit and outbox",
                "Kafka command delivery and Agent ACK consumption",
                "Redis command deletion only after ACK publish acceptance",
                "cross-record reconciliation by stable operation/event IDs",
            ],
            "excluded": [
                "production probe-agent DaemonSet rollout",
                "multi-node and shared-certificate identity migration",
                "fault injection and restart recovery",
                "performance, Windows Chrome, release/rollback and G7",
                "external G8 project gates",
            ],
        },
        "g2_status": "PASS" if status == "PASS" else "FAIL",
        "g3_status": "PASS" if status == "PASS" else "FAIL",
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
                "g2_status": manifest["g2_status"],
                "g3_status": manifest["g3_status"],
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

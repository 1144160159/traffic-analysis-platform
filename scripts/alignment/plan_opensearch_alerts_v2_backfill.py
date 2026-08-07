#!/usr/bin/env python3
"""Build a read-only, bounded T-OS-002 backfill plan without calling _reindex."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/opensearch/index-governance.v1.json"
TARGET_MAPPING = ROOT / "common/opensearch/alerts-v2/mappings-component.json"
WILDCARD_RE = re.compile(r"[*?\[\]]")


def parse_rfc3339(value: str) -> datetime:
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    parsed = datetime.fromisoformat(normalized)
    if parsed.tzinfo is None:
        raise ValueError("time bounds must include a timezone")
    return parsed.astimezone(timezone.utc)


def validate_scope(
    tenant_id: str,
    start_time: str,
    end_time: str,
    *,
    max_window_seconds: int,
) -> tuple[datetime, datetime]:
    if not tenant_id.strip() or WILDCARD_RE.search(tenant_id):
        raise ValueError("tenant_id must be one explicit non-wildcard tenant")
    start = parse_rfc3339(start_time)
    end = parse_rfc3339(end_time)
    seconds = int((end - start).total_seconds())
    if seconds <= 0:
        raise ValueError("end_time must be after start_time")
    if seconds > max_window_seconds:
        raise ValueError(f"scope exceeds {max_window_seconds} seconds")
    return start, end


def query_for_scope(
    tenant_id: str,
    start_time: str,
    end_time: str,
    time_field: str,
    *,
    tenant_field: str,
) -> dict[str, Any]:
    return {
        "bool": {
            "filter": [
                {"term": {tenant_field: tenant_id}},
                {"range": {time_field: {"gte": start_time, "lt": end_time}}},
            ]
        }
    }


def plan_sha256(binding: dict[str, Any]) -> str:
    canonical = json.dumps(binding, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(canonical).hexdigest()


def build_plan(
    *,
    observation: dict[str, Any],
    tenant_id: str,
    start_time: str,
    end_time: str,
    time_field: str,
    max_documents: int,
    slices: int,
    requests_per_second: int,
    min_free_bytes_required: int,
    contract: dict[str, Any],
) -> dict[str, Any]:
    backfill = contract["backfill_execution"]
    validate_scope(
        tenant_id,
        start_time,
        end_time,
        max_window_seconds=int(backfill["max_window_seconds"]),
    )
    if max_documents < 1 or max_documents > int(backfill["max_documents_per_slice"]):
        raise ValueError("max_documents is outside the contract bound")
    if slices < 1 or slices > int(backfill["max_slices"]):
        raise ValueError("slices is outside the contract bound")
    if requests_per_second < 1 or requests_per_second > int(backfill["max_requests_per_second"]):
        raise ValueError("requests_per_second is outside the contract bound")
    if min_free_bytes_required < int(backfill["minimum_free_bytes"]):
        raise ValueError("min_free_bytes_required cannot weaken the contract floor")

    source_fields = set(observation.get("source_mapping_fields", []))
    target_fields = set(observation.get("target_contract_fields", []))
    unknown_fields = sorted(source_fields - target_fields)
    aliases = observation.get("target_alias_indices", [])
    write_indices = [item for item in aliases if item.get("is_write_index") is True]
    count = observation.get("source_count")
    blockers: list[str] = []
    if observation.get("cluster_status") not in {"green", "yellow"}:
        blockers.append("cluster health is not green or yellow")
    if not observation.get("cluster_uuid"):
        blockers.append("cluster UUID is missing")
    if len(write_indices) != 1:
        blockers.append("target alias does not resolve to exactly one approved write index")
    if not isinstance(count, int) or count < 1:
        blockers.append("source scope is empty or count is unavailable")
    elif count > max_documents:
        blockers.append("source scope exceeds max_documents and must be split")
    if unknown_fields:
        blockers.append("source mapping contains fields absent from the strict target contract")
    free_bytes = observation.get("minimum_node_free_bytes")
    if not isinstance(free_bytes, int) or free_bytes < min_free_bytes_required:
        blockers.append("minimum node free space is below the approved floor or unavailable")

    source_query = query_for_scope(
        tenant_id, start_time, end_time, time_field, tenant_field="tenant_id.keyword",
    )
    target_query = query_for_scope(
        tenant_id, start_time, end_time, time_field, tenant_field="tenant_id",
    )
    request = {
        "max_docs": count if isinstance(count, int) and 0 < count <= max_documents else max_documents,
        "source": {
            "index": contract["logical_index"],
            "query": source_query,
        },
        "dest": {
            "index": contract["write_alias"],
            "op_type": "create",
        },
    }
    binding = {
        "cluster_uuid": observation.get("cluster_uuid"),
        "source_index": contract["logical_index"],
        "target_alias": contract["write_alias"],
        "target_write_index": write_indices[0].get("index") if len(write_indices) == 1 else None,
        "tenant_id": tenant_id,
        "start_time": start_time,
        "end_time": end_time,
        "time_field": time_field,
        "source_count": count,
        "max_documents": max_documents,
        "slices": slices,
        "requests_per_second": requests_per_second,
        "minimum_node_free_bytes": free_bytes,
        "minimum_free_bytes_required": min_free_bytes_required,
        "source_mapping_fields": sorted(source_fields),
        "target_contract_fields": sorted(target_fields),
        "unknown_source_fields": unknown_fields,
        "source_count_query": {"query": source_query},
        "target_count_query": {"query": target_query},
        "request": request,
    }
    return {
        "schema_version": 1,
        "remediation_id": "T-OS-002",
        "mode": "READ_ONLY_PLAN",
        "scoped_evidence_status": "PASS",
        "execution_readiness": "READY" if not blockers else "BLOCKED",
        "production_applied": False,
        "production_mutations": [],
        "plan_sha256": plan_sha256(binding),
        "binding": binding,
        "execution": {
            "method": "POST",
            "path": (
                f"/_reindex?wait_for_completion=false&slices={slices}"
                f"&requests_per_second={requests_per_second}"
            ),
            "body": request,
            "conflict_policy": "abort",
            "execute_only_from_approved_suspended_job": True,
            "cancel_path_template": "/_tasks/{task_id}/_cancel",
        },
        "stop_conditions": contract["stop_conditions"],
        "blockers": blockers,
    }


class KubectlOpenSearchReader:
    def __init__(self, namespace: str, pod: str, timeout_seconds: int) -> None:
        self.namespace = namespace
        self.pod = pod
        self.timeout_seconds = timeout_seconds

    def request(self, path: str, *, method: str = "GET", body: dict[str, Any] | None = None,
                allow_404: bool = False) -> dict[str, Any] | None:
        command = [
            "kubectl", f"--request-timeout={self.timeout_seconds}s", "-n", self.namespace,
            "exec", "-i", self.pod, "--", "curl", "-sS", "-X", method,
            "-H", "Content-Type: application/json", "-w", "\n%{http_code}",
            f"http://127.0.0.1:9200{path}",
        ]
        payload = json.dumps(body, separators=(",", ":")).encode() if body is not None else None
        if payload is not None:
            command.extend(["--data-binary", "@-"])
        completed = subprocess.run(
            command,
            cwd=ROOT,
            input=payload,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        raw = completed.stdout.decode(errors="replace")
        response, separator, status_text = raw.rpartition("\n")
        status = int(status_text) if separator and status_text.isdigit() else 0
        if allow_404 and status == 404:
            return None
        if completed.returncode != 0 or not 200 <= status < 300:
            stderr = completed.stderr.decode(errors="replace").strip()
            raise RuntimeError(f"read-only OpenSearch request failed: path={path} status={status} stderr={stderr}")
        return json.loads(response)


def mapping_fields(payload: dict[str, Any] | None) -> list[str]:
    fields: set[str] = set()
    for index in (payload or {}).values():
        fields.update(index.get("mappings", {}).get("properties", {}).keys())
    return sorted(fields)


def capture_observation(
    reader: KubectlOpenSearchReader,
    *,
    query: dict[str, Any],
    source_index: str,
    target_alias: str,
    target_contract_fields: list[str],
) -> dict[str, Any]:
    root = reader.request("/") or {}
    health = reader.request("/_cluster/health") or {}
    source_mapping = reader.request(f"/{source_index}/_mapping") or {}
    aliases_payload = reader.request(f"/_alias/{target_alias}", allow_404=True) or {}
    allocation = reader.request("/_cat/allocation?format=json&bytes=b&h=node,disk.avail") or []
    count = reader.request(f"/{source_index}/_count", method="POST", body={"query": query}) or {}
    aliases: list[dict[str, Any]] = []
    for index_name, value in aliases_payload.items():
        alias = value.get("aliases", {}).get(target_alias, {})
        aliases.append({"index": index_name, "is_write_index": alias.get("is_write_index") is True})
    free_values = [int(row["disk.avail"]) for row in allocation if str(row.get("disk.avail", "")).isdigit()]
    return {
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "cluster_uuid": root.get("cluster_uuid"),
        "cluster_status": health.get("status"),
        "source_mapping_fields": mapping_fields(source_mapping),
        "target_contract_fields": target_contract_fields,
        "target_alias_indices": aliases,
        "minimum_node_free_bytes": min(free_values) if free_values else None,
        "source_count": count.get("count"),
        "read_only_requests": [
            "/", "/_cluster/health", f"/{source_index}/_mapping",
            f"/_alias/{target_alias}", "/_cat/allocation", f"/{source_index}/_count",
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tenant-id", required=True)
    parser.add_argument("--start-time", required=True)
    parser.add_argument("--end-time", required=True)
    parser.add_argument("--time-field", default="ingest_ts", choices=("ingest_ts", "last_seen", "first_seen"))
    parser.add_argument("--max-documents", type=int, default=10_000)
    parser.add_argument("--slices", type=int, default=1)
    parser.add_argument("--requests-per-second", type=int, default=50)
    parser.add_argument("--min-free-bytes", type=int, default=161061273600)
    parser.add_argument("--namespace", default="middleware")
    parser.add_argument("--pod", default="opensearch-0")
    parser.add_argument("--request-timeout-seconds", type=int, default=20)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite plan: {output}")
    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    target_mapping = json.loads(TARGET_MAPPING.read_text(encoding="utf-8"))
    target_fields = sorted(target_mapping["template"]["mappings"]["properties"])
    try:
        validate_scope(
            args.tenant_id,
            args.start_time,
            args.end_time,
            max_window_seconds=int(contract["backfill_execution"]["max_window_seconds"]),
        )
        query = query_for_scope(
            args.tenant_id, args.start_time, args.end_time, args.time_field,
            tenant_field="tenant_id.keyword",
        )
        reader = KubectlOpenSearchReader(args.namespace, args.pod, args.request_timeout_seconds)
        observation = capture_observation(
            reader,
            query=query,
            source_index=contract["logical_index"],
            target_alias=contract["write_alias"],
            target_contract_fields=target_fields,
        )
        plan = build_plan(
            observation=observation,
            tenant_id=args.tenant_id,
            start_time=args.start_time,
            end_time=args.end_time,
            time_field=args.time_field,
            max_documents=args.max_documents,
            slices=args.slices,
            requests_per_second=args.requests_per_second,
            min_free_bytes_required=args.min_free_bytes,
            contract=contract,
        )
    except (ValueError, RuntimeError, json.JSONDecodeError) as exc:
        raise SystemExit(str(exc)) from exc
    plan["captured_at"] = observation["captured_at"]
    plan["observation"] = observation
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(plan, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "scoped_evidence_status": plan["scoped_evidence_status"],
        "execution_readiness": plan["execution_readiness"],
        "plan_sha256": plan["plan_sha256"],
        "blockers": plan["blockers"],
        "output": str(output),
        "production_mutations": [],
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

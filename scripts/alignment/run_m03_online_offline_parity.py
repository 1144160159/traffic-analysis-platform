#!/usr/bin/env python3
"""Compare real online and offline M03 projections for one immutable PCAP corpus."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import sys
from collections.abc import Iterable
from pathlib import Path
from typing import Any

from reconcile_m03_clickhouse_events import (
    TABLES,
    ClickHouseHttpClient,
    QueryClient,
    reconcile,
    validate_identifier,
)


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_CONTRACT = ROOT / "contracts/flink/m03-online-offline-parity.v1.json"
DEFAULT_CORPUS = ROOT / "tests/fixtures/pcap/m03/manifest.v1.json"
SHA256 = re.compile(r"^[0-9a-f]{64}$")
ROUTE_MODES = {
    "online": {"af_packet", "xdp", "xdp_skb", "xdp_offload"},
    "offline": {"pcap_offline"},
}
RECEIPT_FIELDS = {
    "schema_version", "route", "tenant_id", "probe_id", "run_id", "candidate_sha256",
    "corpus_id", "corpus_manifest_sha256", "capture_mode",
    "packet_count_received", "packet_count_dropped", "event_time_offset_ms",
    "source_completed", "pipeline_drained", "final_checkpoint_id",
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_json(path: Path, label: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot load {label} {path}: {error}") from error
    if not isinstance(value, dict):
        raise ValueError(f"{label} must be a JSON object")
    return value


def validate_contract(contract: dict[str, Any]) -> None:
    if contract.get("schema_version") != 1:
        raise ValueError("parity contract schema_version must be 1")
    if contract.get("contract_id") != "t1-m03-n012-online-offline-parity-v1":
        raise ValueError("unexpected parity contract_id")
    identity = contract.get("identity")
    if not isinstance(identity, dict):
        raise ValueError("parity identity policy is required")
    route_specific = set(identity.get("route_specific_fields", []))
    processing = set(identity.get("processing_telemetry_fields", []))
    if not {"event_id", "run_id", "session_id", "object_id", "window_id"} <= route_specific:
        raise ValueError("route-specific identity field policy is incomplete")
    if processing != {"ingest_ts", "kafka_ts", "flink_out_ts"}:
        raise ValueError("processing telemetry field policy drifted")
    table_contracts = contract.get("tables")
    if not isinstance(table_contracts, dict) or set(table_contracts) != set(TABLES):
        raise ValueError("parity contract must cover the exact five M03 tables")

    for table, spec in table_contracts.items():
        if not isinstance(spec, dict) or spec.get("required_nonempty") is not True:
            raise ValueError(f"{table}: required_nonempty must be true")
        groups = {
            "key_fields": spec.get("key_fields", []),
            "exact_fields": spec.get("exact_fields", []),
            "cardinality_fields": spec.get("cardinality_fields", []),
            "presence_fields": spec.get("presence_fields", []),
            "tolerances": list((spec.get("tolerances") or {}).keys()),
        }
        for label, values in groups.items():
            if not isinstance(values, list) or any(not isinstance(value, str) for value in values):
                raise ValueError(f"{table}: {label} must be a string list")
        all_declared: list[str] = [field for values in groups.values() for field in values]
        if len(all_declared) != len(set(all_declared)):
            raise ValueError(f"{table}: comparison fields overlap")
        if not groups["key_fields"]:
            raise ValueError(f"{table}: stable business key is required")
        timestamps = spec.get("timestamp_fields", [])
        if not isinstance(timestamps, list) or not set(timestamps) <= set(all_declared):
            raise ValueError(f"{table}: timestamp_fields must be compared fields")
        expected = set(TABLES[table]) - route_specific - processing
        actual = set(all_declared)
        if expected != actual:
            raise ValueError(
                f"{table}: field coverage drift missing={sorted(expected - actual)} "
                f"unexpected={sorted(actual - expected)}"
            )
        for field, tolerance in (spec.get("tolerances") or {}).items():
            if not isinstance(tolerance, dict) or set(tolerance) != {"absolute", "relative"}:
                raise ValueError(f"{table}.{field}: explicit absolute and relative tolerance required")
            absolute = tolerance["absolute"]
            relative = tolerance["relative"]
            if not isinstance(absolute, (int, float)) or not isinstance(relative, (int, float)):
                raise ValueError(f"{table}.{field}: tolerance must be numeric")
            if absolute < 0 or relative < 0 or not math.isfinite(absolute + relative):
                raise ValueError(f"{table}.{field}: tolerance must be finite and non-negative")
    maximum = (contract.get("acceptance") or {}).get("max_reported_differences")
    if not isinstance(maximum, int) or not 1 <= maximum <= 10_000:
        raise ValueError("max_reported_differences must be between 1 and 10000")


def validate_corpus(manifest_path: Path) -> dict[str, Any]:
    manifest_path = manifest_path.resolve()
    manifest = load_json(manifest_path, "PCAP corpus manifest")
    if manifest.get("schema_version") != "1.0.0" or not manifest.get("corpus_id"):
        raise ValueError("invalid M03 PCAP corpus identity")
    fixtures = manifest.get("fixtures")
    if not isinstance(fixtures, list) or not fixtures:
        raise ValueError("PCAP corpus has no fixtures")
    root = manifest_path.parent.resolve()
    packet_count = 0
    fixture_results = []
    seen_ids: set[str] = set()
    for fixture in fixtures:
        if not isinstance(fixture, dict):
            raise ValueError("PCAP fixture must be an object")
        fixture_id = fixture.get("fixture_id")
        relative = fixture.get("relative_path")
        expected_hash = fixture.get("sha256")
        expected_size = fixture.get("size_bytes")
        expected_packets = fixture.get("packet_count")
        if not isinstance(fixture_id, str) or not fixture_id or fixture_id in seen_ids:
            raise ValueError("PCAP fixture_id is blank or duplicated")
        seen_ids.add(fixture_id)
        if not isinstance(relative, str) or not relative:
            raise ValueError(f"{fixture_id}: relative_path is required")
        candidate = (root / relative).resolve()
        if not candidate.is_relative_to(root) or not candidate.is_file():
            raise ValueError(f"{fixture_id}: fixture escapes corpus or does not exist")
        if not isinstance(expected_hash, str) or not SHA256.fullmatch(expected_hash):
            raise ValueError(f"{fixture_id}: invalid sha256")
        actual_hash = sha256(candidate)
        actual_size = candidate.stat().st_size
        if actual_hash != expected_hash or actual_size != expected_size:
            raise ValueError(f"{fixture_id}: PCAP bytes do not match manifest")
        if not isinstance(expected_packets, int) or expected_packets < 0:
            raise ValueError(f"{fixture_id}: invalid packet_count")
        packet_count += expected_packets
        fixture_results.append({
            "fixture_id": fixture_id,
            "relative_path": relative,
            "sha256": actual_hash,
            "size_bytes": actual_size,
            "packet_count": expected_packets,
        })
    return {
        "corpus_id": manifest["corpus_id"],
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
        "packet_count": packet_count,
        "fixtures": fixture_results,
    }


def validate_receipts(
    online: dict[str, Any], offline: dict[str, Any], corpus: dict[str, Any]
) -> dict[str, Any]:
    receipts = {"online": online, "offline": offline}
    for route, receipt in receipts.items():
        extras = set(receipt) - RECEIPT_FIELDS
        missing = RECEIPT_FIELDS - set(receipt)
        if missing or extras:
            raise ValueError(f"{route} receipt shape drift missing={sorted(missing)} extra={sorted(extras)}")
        if receipt.get("schema_version") != 1 or receipt.get("route") != route:
            raise ValueError(f"{route} receipt identity is invalid")
        for field in ("tenant_id", "probe_id", "run_id"):
            if not isinstance(receipt.get(field), str) or not receipt[field].strip():
                raise ValueError(f"{route} receipt {field} is required")
        if not SHA256.fullmatch(str(receipt.get("candidate_sha256", ""))):
            raise ValueError(f"{route} receipt candidate_sha256 is invalid")
        if receipt.get("corpus_id") != corpus["corpus_id"]:
            raise ValueError(f"{route} receipt corpus_id does not match the immutable corpus")
        if receipt.get("corpus_manifest_sha256") != corpus["manifest_sha256"]:
            raise ValueError(f"{route} receipt corpus manifest hash does not match")
        if receipt.get("capture_mode") not in ROUTE_MODES[route]:
            raise ValueError(f"{route} receipt capture_mode is not a {route} source")
        if receipt.get("packet_count_received") != corpus["packet_count"]:
            raise ValueError(f"{route} receipt packet count does not match corpus")
        if receipt.get("packet_count_dropped") != 0:
            raise ValueError(f"{route} receipt contains capture drops")
        if receipt.get("source_completed") is not True or receipt.get("pipeline_drained") is not True:
            raise ValueError(f"{route} route did not complete and drain")
        for field in ("event_time_offset_ms", "final_checkpoint_id"):
            value = receipt.get(field)
            if not isinstance(value, int) or isinstance(value, bool):
                raise ValueError(f"{route} receipt {field} must be an integer")
        if receipt["final_checkpoint_id"] < 0:
            raise ValueError(f"{route} receipt final_checkpoint_id must be non-negative")
    if online["tenant_id"] != offline["tenant_id"]:
        raise ValueError("online/offline receipts use different tenants")
    if online["probe_id"] != offline["probe_id"]:
        raise ValueError("online/offline receipts use different probe identities")
    if online["run_id"] == offline["run_id"]:
        raise ValueError("same run_id self-comparison is forbidden")
    if online["candidate_sha256"] != offline["candidate_sha256"]:
        raise ValueError("online/offline receipts use different candidate source")
    return {
        "tenant_id": online["tenant_id"],
        "probe_id": online["probe_id"],
        "online_run_id": online["run_id"],
        "offline_run_id": offline["run_id"],
        "candidate_sha256": online["candidate_sha256"],
        "online_event_time_offset_ms": online["event_time_offset_ms"],
        "offline_event_time_offset_ms": offline["event_time_offset_ms"],
    }


def _select_expression(field: str, timestamp_fields: set[str], table: str) -> str:
    if field in timestamp_fields and table.startswith("feature_") and field in {"ts", "ts_start", "ts_end"}:
        return f"toUnixTimestamp64Milli({field}) AS {field}"
    return field


def projection_sql(database: str, table: str, spec: dict[str, Any]) -> str:
    validate_identifier(database, "database")
    if table not in TABLES:
        raise ValueError(f"unsupported M03 table: {table}")
    fields: list[str] = ["event_id"]
    for category in ("key_fields", "exact_fields", "cardinality_fields", "presence_fields"):
        for field in spec.get(category, []):
            if field not in fields:
                fields.append(field)
    for field in (spec.get("tolerances") or {}):
        if field not in fields:
            fields.append(field)
    timestamps = set(spec.get("timestamp_fields", []))
    select = ",\n    ".join(_select_expression(field, timestamps, table) for field in fields)
    return f"""SELECT
    {select}
FROM {database}.{table}
WHERE tenant_id = {{tenant:String}} AND run_id = {{run:String}}
FORMAT JSON"""


def fetch_projection(
    client: QueryClient,
    database: str,
    table: str,
    spec: dict[str, Any],
    tenant_id: str,
    run_id: str,
) -> list[dict[str, Any]]:
    rows = client.query_json(
        projection_sql(database, table, spec), {"tenant": tenant_id, "run": run_id}
    )
    if not isinstance(rows, list) or any(not isinstance(row, dict) for row in rows):
        raise RuntimeError(f"{table}: ClickHouse projection response is invalid")
    return rows


def _shift_timestamp(value: Any, offset_ms: int) -> Any:
    if value is None:
        return None
    if isinstance(value, bool):
        raise ValueError("boolean cannot be a timestamp")
    if isinstance(value, (int, float)):
        return value - offset_ms
    try:
        return int(value) - offset_ms
    except (TypeError, ValueError) as error:
        raise ValueError(f"timestamp value is not epoch milliseconds: {value!r}") from error


def normalize_row(row: dict[str, Any], spec: dict[str, Any], offset_ms: int) -> dict[str, Any]:
    normalized = dict(row)
    for field in spec.get("timestamp_fields", []):
        if field in normalized:
            normalized[field] = _shift_timestamp(normalized[field], offset_ms)
    return normalized


def _key(row: dict[str, Any], fields: Iterable[str]) -> tuple[Any, ...]:
    return tuple(json.dumps(row.get(field), ensure_ascii=False, sort_keys=True) for field in fields)


def _index_rows(
    rows: list[dict[str, Any]], spec: dict[str, Any], offset_ms: int, table: str, route: str
) -> tuple[dict[tuple[Any, ...], dict[str, Any]], list[dict[str, Any]]]:
    index: dict[tuple[Any, ...], dict[str, Any]] = {}
    duplicate_keys: list[dict[str, Any]] = []
    for row in rows:
        normalized = normalize_row(row, spec, offset_ms)
        key = _key(normalized, spec["key_fields"])
        if key in index:
            duplicate_keys.append({
                "kind": "duplicate_business_key", "table": table, "route": route,
                "key": dict(zip(spec["key_fields"], key)),
                "event_ids": [index[key].get("event_id"), normalized.get("event_id")],
            })
        else:
            index[key] = normalized
    return index, duplicate_keys


def _numeric_equal(left: Any, right: Any, absolute: float, relative: float) -> bool:
    if isinstance(left, list) or isinstance(right, list):
        if not isinstance(left, list) or not isinstance(right, list) or len(left) != len(right):
            return False
        return all(_numeric_equal(a, b, absolute, relative) for a, b in zip(left, right))
    try:
        a = float(left)
        b = float(right)
    except (TypeError, ValueError):
        return False
    if not math.isfinite(a) or not math.isfinite(b):
        return a == b
    return math.isclose(a, b, abs_tol=absolute, rel_tol=relative)


def compare_table(
    table: str,
    spec: dict[str, Any],
    online_rows: list[dict[str, Any]],
    offline_rows: list[dict[str, Any]],
    online_offset_ms: int,
    offline_offset_ms: int,
    max_differences: int,
) -> dict[str, Any]:
    online, differences = _index_rows(online_rows, spec, online_offset_ms, table, "online")
    offline, offline_duplicates = _index_rows(offline_rows, spec, offline_offset_ms, table, "offline")
    differences.extend(offline_duplicates)
    online_keys = set(online)
    offline_keys = set(offline)
    for key in sorted(online_keys - offline_keys):
        differences.append({"kind": "missing_offline", "table": table, "key": list(key)})
    for key in sorted(offline_keys - online_keys):
        differences.append({"kind": "missing_online", "table": table, "key": list(key)})

    for key in sorted(online_keys & offline_keys):
        left = online[key]
        right = offline[key]
        key_value = dict(zip(spec["key_fields"], key))
        for field in spec.get("exact_fields", []):
            if left.get(field) != right.get(field):
                differences.append({
                    "kind": "exact_field_mismatch", "table": table, "key": key_value,
                    "field": field, "online": left.get(field), "offline": right.get(field),
                })
        for field in spec.get("cardinality_fields", []):
            left_value = left.get(field)
            right_value = right.get(field)
            left_count = len(left_value) if isinstance(left_value, list) else None
            right_count = len(right_value) if isinstance(right_value, list) else None
            if left_count is None or right_count is None or left_count != right_count:
                differences.append({
                    "kind": "lineage_cardinality_mismatch", "table": table, "key": key_value,
                    "field": field, "online": left_count, "offline": right_count,
                })
        for field in spec.get("presence_fields", []):
            if bool(left.get(field)) != bool(right.get(field)):
                differences.append({
                    "kind": "presence_mismatch", "table": table, "key": key_value,
                    "field": field, "online": bool(left.get(field)),
                    "offline": bool(right.get(field)),
                })
        for field, tolerance in (spec.get("tolerances") or {}).items():
            if not _numeric_equal(
                left.get(field), right.get(field), tolerance["absolute"], tolerance["relative"]
            ):
                differences.append({
                    "kind": "tolerance_exceeded", "table": table, "key": key_value,
                    "field": field, "online": left.get(field), "offline": right.get(field),
                    "absolute_tolerance": tolerance["absolute"],
                    "relative_tolerance": tolerance["relative"],
                })
    total = len(differences)
    empty_failure = spec["required_nonempty"] and (not online_rows or not offline_rows)
    return {
        "table": table,
        "status": "PASS" if total == 0 and not empty_failure else "FAIL",
        "online_rows": len(online_rows),
        "offline_rows": len(offline_rows),
        "matched_business_keys": len(online_keys & offline_keys),
        "difference_count": total + (1 if empty_failure else 0),
        "differences": ([{
            "kind": "required_projection_empty", "table": table,
            "online_rows": len(online_rows), "offline_rows": len(offline_rows),
        }] if empty_failure else [])
        + differences[:max_differences],
        "differences_truncated": total > max_differences,
    }


def run_parity(
    client: QueryClient,
    *,
    database: str,
    contract: dict[str, Any],
    corpus: dict[str, Any],
    online_receipt: dict[str, Any],
    offline_receipt: dict[str, Any],
) -> dict[str, Any]:
    validate_contract(contract)
    scope = validate_receipts(online_receipt, offline_receipt, corpus)
    required = set(contract["tables"])
    reconciliations = {}
    for route, run_id in (
        ("online", scope["online_run_id"]), ("offline", scope["offline_run_id"])
    ):
        result = reconcile(
            client, database=database, tenant_id=scope["tenant_id"], run_id=run_id,
            required_nonempty=required,
        )
        reconciliations[route] = result
    if any(result["status"] != "PASS" for result in reconciliations.values()):
        return {
            "schema_version": 1,
            "contract_id": contract["contract_id"],
            "status": "FAIL",
            "phase": "per_route_event_id_reconciliation",
            "scope": scope,
            "corpus": corpus,
            "reconciliations": reconciliations,
            "tables": [],
            "errors": ["one or both routes contain empty, duplicate, blank or conflicting projections"],
        }

    maximum = contract["acceptance"]["max_reported_differences"]
    table_results = []
    remaining = maximum
    for table, spec in contract["tables"].items():
        online_rows = fetch_projection(
            client, database, table, spec, scope["tenant_id"], scope["online_run_id"]
        )
        offline_rows = fetch_projection(
            client, database, table, spec, scope["tenant_id"], scope["offline_run_id"]
        )
        result = compare_table(
            table, spec, online_rows, offline_rows,
            scope["online_event_time_offset_ms"], scope["offline_event_time_offset_ms"],
            max(0, remaining),
        )
        remaining -= len(result["differences"])
        table_results.append(result)
    difference_count = sum(item["difference_count"] for item in table_results)
    return {
        "schema_version": 1,
        "contract_id": contract["contract_id"],
        "status": "PASS" if difference_count == 0 else "FAIL",
        "phase": "field_level_projection_parity",
        "scope": scope,
        "corpus": corpus,
        "reconciliations": reconciliations,
        "tables": table_results,
        "difference_count": difference_count,
        "differences_truncated": difference_count > maximum,
        "claim_boundary": (
            "PASS covers this immutable corpus, candidate and two completed route receipts only; "
            "it is not a throughput, all-protocol or release-promotion result"
        ),
        "errors": [] if difference_count == 0 else ["online/offline projection differences exist"],
    }


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--online-receipt", type=Path, required=True)
    parser.add_argument("--offline-receipt", type=Path, required=True)
    parser.add_argument("--corpus-manifest", type=Path, default=DEFAULT_CORPUS)
    parser.add_argument("--contract", type=Path, default=DEFAULT_CONTRACT)
    parser.add_argument("--endpoint", default=os.getenv("CLICKHOUSE_HTTP_URL", "http://127.0.0.1:8123"))
    parser.add_argument("--database", default=os.getenv("CLICKHOUSE_DATABASE", "traffic"))
    parser.add_argument("--user", default=os.getenv("CLICKHOUSE_USER", "default"))
    parser.add_argument("--password-env", default="CLICKHOUSE_PASSWORD")
    parser.add_argument("--ca-file")
    parser.add_argument("--timeout-seconds", type=float, default=30.0)
    parser.add_argument("--output", type=Path, required=True)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if args.output.exists():
        raise SystemExit(f"refusing to overwrite immutable parity result: {args.output}")
    try:
        contract_path = args.contract.resolve()
        contract = load_json(contract_path, "parity contract")
        validate_contract(contract)
        corpus = validate_corpus(args.corpus_manifest)
        online_path = args.online_receipt.resolve()
        offline_path = args.offline_receipt.resolve()
        online = load_json(online_path, "online route receipt")
        offline = load_json(offline_path, "offline route receipt")
        client = ClickHouseHttpClient(
            endpoint=args.endpoint, user=args.user,
            password=os.getenv(args.password_env, ""), database=args.database,
            timeout_seconds=args.timeout_seconds, ca_file=args.ca_file,
        )
        result = run_parity(
            client, database=args.database, contract=contract, corpus=corpus,
            online_receipt=online, offline_receipt=offline,
        )
        result["artifacts"] = {
            "contract": str(contract_path), "contract_sha256": sha256(contract_path),
            "online_receipt": str(online_path), "online_receipt_sha256": sha256(online_path),
            "offline_receipt": str(offline_path), "offline_receipt_sha256": sha256(offline_path),
        }
    except (ValueError, RuntimeError, json.JSONDecodeError) as error:
        result = {"schema_version": 1, "status": "FAIL", "phase": "preflight", "error": str(error)}
    rendered = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(rendered, encoding="utf-8")
    sys.stdout.write(rendered)
    return 0 if result.get("status") == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

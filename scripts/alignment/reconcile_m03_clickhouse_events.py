#!/usr/bin/env python3
"""Reconcile M03 ClickHouse projections by deterministic event identity.

The Flink sinks deliberately fail checkpoints when ClickHouse does not
acknowledge a complete batch.  A process can still fail after the remote ACK
and before the checkpoint completes, so a replay with a different batch
boundary can leave physical duplicates in an ordinary MergeTree.  This tool
is the post-run, key-level guard for that failure window.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol


IDENTIFIER = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
TABLES: dict[str, tuple[str, ...]] = {
    "flows_raw": (
        "probe_id", "community_id", "src_ip", "dst_ip", "src_port", "dst_port",
        "protocol", "direction", "ts_start", "ts_end", "duration_ms", "packets_fwd",
        "packets_bwd", "bytes_fwd", "bytes_bwd", "pps", "bps", "tcp_flags_fwd",
        "tcp_flags_bwd", "tos", "feature_set_id", "event_ts", "pktlen_min", "pktlen_max",
        "pktlen_mean", "pktlen_std", "iat_min_ms", "iat_max_ms", "iat_mean_ms",
        "iat_std_ms", "active_min_ms", "active_max_ms", "active_mean_ms", "active_std_ms",
        "idle_min_ms", "idle_max_ms", "idle_mean_ms", "idle_std_ms", "subflow_count",
    ),
    "sessions": (
        "session_id", "community_id", "ts_start", "ts_end", "duration_ms", "packets_fwd",
        "packets_bwd", "bytes_fwd", "bytes_bwd", "feature_set_id", "event_ts", "probe_id",
        "src_ip", "dst_ip", "src_port", "dst_port", "protocol", "bytes_total",
        "up_down_ratio", "num_pkts", "avg_payload", "min_payload", "max_payload",
        "std_payload", "mean_iat_ms", "min_iat_ms", "max_iat_ms", "std_iat_ms",
        "flags_syn", "flags_ack", "flags_fin", "flags_psh", "flags_rst", "dns_pkt_cnt",
        "tcp_pkt_cnt", "udp_pkt_cnt", "icmp_pkt_cnt", "has_syn", "has_fin", "has_rst",
        "is_established", "evidence_count", "flow_ids", "end_reason", "event_schema_version",
        "aggregate_version", "identity_version", "session_version", "event_time_start_ms",
        "event_time_end_ms", "source_watermark_ms", "source_event_ids", "evidence_ids",
        "completeness", "is_partial", "missing_fields",
    ),
    "feature_stat": (
        "feature_set_id", "schema_version", "object_type", "object_id", "community_id", "ts",
        "protocol", "duration_ms", "pps", "bps", "up_down_ratio", "pktlen_mean", "pktlen_std",
        "iat_mean_ms", "iat_std_ms", "active_mean_ms", "idle_mean_ms", "tcp_flag_syn_cnt",
        "tcp_flag_ack_cnt", "tcp_init_win_bytes_fwd", "tcp_init_win_bytes_bwd", "extra",
        "event_schema_version", "aggregate_version", "event_time_start_ms", "event_time_end_ms",
        "source_watermark_ms", "source_event_ids", "evidence_ids", "feature_category",
        "availability", "algorithm_version", "window_id", "value_unit", "is_partial",
        "missing_fields", "missing_reason",
    ),
    "feature_seq": (
        "feature_set_id", "object_type", "object_id", "community_id", "window_id", "ts_start",
        "ts_end", "pktlen_seq_hash", "iat_seq_hash", "wavelet_releng_fwd",
        "wavelet_releng_bwd", "wavelet_entropy_fwd", "wavelet_entropy_bwd",
        "wavelet_detail_mean_fwd", "wavelet_detail_mean_bwd", "wavelet_detail_std_fwd",
        "wavelet_detail_std_bwd", "seq_blob_ref", "feature_category", "availability",
        "schema_version", "algorithm_version", "value_unit", "source_event_ids", "evidence_ids",
        "missing_fields", "missing_reason",
    ),
    "feature_fp": (
        "feature_set_id", "community_id", "session_id", "ts", "is_encrypted", "tls_version",
        "ja3", "ja4", "sni", "sni_hash", "cert_sha256", "cert_is_self_signed", "pubkey_len",
        "quic_version", "transport_security", "raw_traffic_ref", "hex_freq", "hex_ratio",
        "entropy_payload", "chi_square_bfd", "feature_category", "availability",
        "schema_version", "algorithm_version", "window_id", "event_time_start_ms",
        "event_time_end_ms", "source_event_ids", "evidence_ids", "missing_fields",
        "missing_reason",
    ),
}
DEFAULT_REQUIRED_NONEMPTY = ("flows_raw", "sessions", "feature_stat")


class QueryClient(Protocol):
    def query_json(self, sql: str, parameters: dict[str, str]) -> list[dict[str, Any]]: ...


@dataclass(frozen=True)
class ClickHouseHttpClient:
    endpoint: str
    user: str
    password: str
    database: str
    timeout_seconds: float = 30.0
    ca_file: str | None = None

    def __post_init__(self) -> None:
        parsed = urllib.parse.urlsplit(self.endpoint)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("ClickHouse endpoint must be an absolute http(s) URL")
        if parsed.username or parsed.password:
            raise ValueError("credentials in ClickHouse endpoint are forbidden")
        validate_identifier(self.database, "database")

    def query_json(self, sql: str, parameters: dict[str, str]) -> list[dict[str, Any]]:
        query = {"database": self.database}
        query.update({f"param_{key}": value for key, value in parameters.items()})
        separator = "&" if "?" in self.endpoint else "?"
        url = self.endpoint.rstrip("/") + separator + urllib.parse.urlencode(query)
        headers = {
            "Content-Type": "text/plain; charset=utf-8",
            "X-ClickHouse-User": self.user,
            "X-ClickHouse-Key": self.password,
        }
        request = urllib.request.Request(
            url, data=sql.encode("utf-8"), headers=headers, method="POST"
        )
        context = None
        if urllib.parse.urlsplit(url).scheme == "https":
            context = ssl.create_default_context(cafile=self.ca_file)
        handlers: list[Any] = [urllib.request.ProxyHandler({})]
        if context is not None:
            handlers.append(urllib.request.HTTPSHandler(context=context))
        opener = urllib.request.build_opener(*handlers)
        try:
            with opener.open(request, timeout=self.timeout_seconds) as response:
                payload = json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")[:1000]
            raise RuntimeError(f"ClickHouse query failed with HTTP {error.code}: {detail}") from error
        if not isinstance(payload, dict) or not isinstance(payload.get("data"), list):
            raise RuntimeError("ClickHouse response is not FORMAT JSON data")
        return payload["data"]


def validate_identifier(value: str, label: str) -> str:
    if not IDENTIFIER.fullmatch(value):
        raise ValueError(f"invalid ClickHouse {label}: {value!r}")
    return value


def grouped_sql(database: str, table: str) -> str:
    database = validate_identifier(database, "database")
    if table not in TABLES:
        raise ValueError(f"unsupported M03 table: {table}")
    payload = ", ".join(TABLES[table])
    return f"""SELECT
    sum(copies) AS physical_rows,
    count() AS unique_events,
    sum(copies) - count() AS duplicate_rows,
    countIf(payload_versions > 1) AS conflicting_events,
    sumIf(copies, event_id = '') AS blank_event_id_rows
FROM
(
    SELECT
        event_id,
        count() AS copies,
        uniqExact(sipHash64(tuple({payload}))) AS payload_versions
    FROM {database}.{table}
    WHERE tenant_id = {{tenant:String}} AND run_id = {{run:String}}
    GROUP BY event_id
)
FORMAT JSON"""


def detail_sql(database: str, table: str, limit: int) -> str:
    if not 1 <= limit <= 1000:
        raise ValueError("detail limit must be between 1 and 1000")
    grouped = grouped_sql(database, table)
    inner_start = grouped.index("(", grouped.index("FROM")) + 1
    inner_end = grouped.rindex(")\nFORMAT JSON")
    inner = grouped[inner_start:inner_end].strip()
    return f"""SELECT event_id, copies, payload_versions
FROM
(
{inner}
)
WHERE event_id = '' OR copies > 1 OR payload_versions > 1
ORDER BY payload_versions DESC, copies DESC, event_id
LIMIT {limit}
FORMAT JSON"""


def _as_int(row: dict[str, Any], field: str) -> int:
    value = row.get(field, 0)
    try:
        number = int(value or 0)
    except (TypeError, ValueError) as error:
        raise RuntimeError(f"ClickHouse returned invalid {field}: {value!r}") from error
    if number < 0:
        raise RuntimeError(f"ClickHouse returned negative {field}: {number}")
    return number


def reconcile(
    client: QueryClient,
    *,
    database: str,
    tenant_id: str,
    run_id: str,
    required_nonempty: set[str] | None = None,
    detail_limit: int = 100,
) -> dict[str, Any]:
    if not tenant_id.strip() or not run_id.strip():
        raise ValueError("tenant_id and run_id are required")
    validate_identifier(database, "database")
    required = set(DEFAULT_REQUIRED_NONEMPTY if required_nonempty is None else required_nonempty)
    unknown = required - TABLES.keys()
    if unknown:
        raise ValueError(f"unknown required tables: {sorted(unknown)}")

    table_results: list[dict[str, Any]] = []
    errors: list[str] = []
    parameters = {"tenant": tenant_id, "run": run_id}
    for table in TABLES:
        rows = client.query_json(grouped_sql(database, table), parameters)
        if len(rows) != 1:
            raise RuntimeError(f"{table} reconciliation returned {len(rows)} summary rows")
        summary = {name: _as_int(rows[0], name) for name in (
            "physical_rows", "unique_events", "duplicate_rows",
            "conflicting_events", "blank_event_id_rows",
        )}
        if summary["physical_rows"] < summary["unique_events"]:
            raise RuntimeError(f"{table} returned impossible physical/unique counts")
        details: list[dict[str, Any]] = []
        if any(summary[name] for name in (
            "duplicate_rows", "conflicting_events", "blank_event_id_rows"
        )):
            details = client.query_json(detail_sql(database, table, detail_limit), parameters)
        table_errors: list[str] = []
        if table in required and summary["physical_rows"] == 0:
            table_errors.append("required projection is empty")
        if summary["blank_event_id_rows"]:
            table_errors.append("blank event_id rows are present")
        if summary["duplicate_rows"]:
            table_errors.append("physical duplicate event_id rows are present")
        if summary["conflicting_events"]:
            table_errors.append("event_id payload conflicts are present")
        errors.extend(f"{table}: {message}" for message in table_errors)
        table_results.append({
            "table": table,
            **summary,
            "status": "PASS" if not table_errors else "FAIL",
            "details": details,
            "errors": table_errors,
        })
    return {
        "schema_version": 1,
        "contract_id": "t1-m03-n009-clickhouse-event-reconciliation-v1",
        "status": "PASS" if not errors else "FAIL",
        "scope": {"database": database, "tenant_id": tenant_id, "run_id": run_id},
        "required_nonempty_tables": sorted(required),
        "tables": table_results,
        "errors": errors,
        "claim_boundary": (
            "PASS proves no duplicate or conflicting event_id rows for this tenant/run snapshot; "
            "it does not prove source-offset completeness or global exactly-once delivery"
        ),
    }


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--endpoint", default=os.getenv("CLICKHOUSE_HTTP_URL", "http://127.0.0.1:8123"))
    parser.add_argument("--database", default=os.getenv("CLICKHOUSE_DATABASE", "traffic"))
    parser.add_argument("--user", default=os.getenv("CLICKHOUSE_USER", "default"))
    parser.add_argument("--password-env", default="CLICKHOUSE_PASSWORD")
    parser.add_argument("--tenant-id", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--ca-file")
    parser.add_argument("--timeout-seconds", type=float, default=30.0)
    parser.add_argument("--detail-limit", type=int, default=100)
    parser.add_argument(
        "--required-nonempty", action="append", choices=sorted(TABLES),
        help="repeat to override the default required core projections",
    )
    parser.add_argument("--output", type=Path)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if args.timeout_seconds <= 0:
        raise SystemExit("timeout-seconds must be positive")
    if args.output and args.output.exists():
        raise SystemExit(f"refusing to overwrite reconciliation output: {args.output}")
    client = ClickHouseHttpClient(
        endpoint=args.endpoint,
        user=args.user,
        password=os.getenv(args.password_env, ""),
        database=args.database,
        timeout_seconds=args.timeout_seconds,
        ca_file=args.ca_file,
    )
    try:
        result = reconcile(
            client,
            database=args.database,
            tenant_id=args.tenant_id,
            run_id=args.run_id,
            required_nonempty=(set(args.required_nonempty) if args.required_nonempty else None),
            detail_limit=args.detail_limit,
        )
    except (ValueError, RuntimeError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "FAIL", "error": str(error)}, ensure_ascii=False, indent=2))
        return 2
    rendered = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    sys.stdout.write(rendered)
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

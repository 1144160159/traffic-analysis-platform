#!/usr/bin/env python3
"""Capture a read-only M03 covered-profile observation.

Prometheus owns runtime checkpoint, task, lag and sink-latency measurements.
N009 reconciliation owns ClickHouse event identity counts. Protocol coverage
and online/offline parity enter through immutable candidate/run-bound receipts;
the collector does not infer them from metric or fixture names.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import math
import os
import ssl
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

from evaluate_m03_covered_profile import (
    OBSERVATION_SCHEMA,
    ProfileBlocked,
    load_json,
    parse_time,
    sha256_path,
    validate_profile,
)
from build_topic1_task_registry import validate_against_schema
from reconcile_m03_clickhouse_events import ClickHouseHttpClient, reconcile


RUNTIME_QUERIES = {
    "session_running_tasks": 'sum(flink_jobmanager_job_numRunningTasks{job_name="Session Aggregation Job V2"})',
    "session_completed_checkpoints": 'increase(flink_jobmanager_job_numberOfCompletedCheckpoints{job_name="Session Aggregation Job V2"}[5m])',
    "session_failed_checkpoints": 'increase(flink_jobmanager_job_numberOfFailedCheckpoints{job_name="Session Aggregation Job V2"}[5m])',
    "session_checkpoint_duration_ms": 'max_over_time(flink_jobmanager_job_lastCheckpointDuration{job_name="Session Aggregation Job V2"}[5m])',
    "feature_running_tasks": 'sum(flink_jobmanager_job_numRunningTasks{job_name="Feature Extraction Job v3 (Full Enhanced)"})',
    "feature_completed_checkpoints": 'increase(flink_jobmanager_job_numberOfCompletedCheckpoints{job_name="Feature Extraction Job v3 (Full Enhanced)"}[5m])',
    "feature_failed_checkpoints": 'increase(flink_jobmanager_job_numberOfFailedCheckpoints{job_name="Feature Extraction Job v3 (Full Enhanced)"}[5m])',
    "feature_checkpoint_duration_ms": 'max_over_time(flink_jobmanager_job_lastCheckpointDuration{job_name="Feature Extraction Job v3 (Full Enhanced)"}[5m])',
    "session_lag": 'sum(kafka_consumergroup_lag{consumergroup="flink-session-job"})',
    "feature_lag": 'sum(kafka_consumergroup_lag{consumergroup="flink-feature-job"})',
    "session_sink_latency_p95_ms": 'histogram_quantile(0.95,sum by(le)(rate(flink_taskmanager_job_task_operator_sink_commit_latency_ms_bucket{job_name="Session Aggregation Job V2",operator_name="clickhouse-async-sink"}[5m])))',
    "feature_sink_latency_p95_ms": 'histogram_quantile(0.95,sum by(le)(rate(flink_taskmanager_job_task_operator_sink_commit_latency_ms_bucket{job_name="Feature Extraction Job v3 (Full Enhanced)",operator_name=~"clickhouse.*sink"}[5m])))',
}


class MetricClient(Protocol):
    def scalar(self, query: str, at: dt.datetime) -> float: ...


@dataclass(frozen=True)
class PrometheusHttpClient:
    endpoint: str
    timeout_seconds: float = 15.0
    bearer_token: str = ""
    ca_file: str | None = None

    def __post_init__(self) -> None:
        parsed = urllib.parse.urlsplit(self.endpoint)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("Prometheus endpoint must be an absolute http(s) URL")
        if parsed.username or parsed.password:
            raise ValueError("Prometheus credentials in URL are forbidden")

    def scalar(self, query: str, at: dt.datetime) -> float:
        url = self.endpoint.rstrip("/") + "/api/v1/query?" + urllib.parse.urlencode({
            "query": query,
            "time": f"{at.timestamp():.3f}",
        })
        headers = {"Accept": "application/json"}
        if self.bearer_token:
            headers["Authorization"] = "Bearer " + self.bearer_token
        request = urllib.request.Request(url, headers=headers, method="GET")
        handlers: list[Any] = [urllib.request.ProxyHandler({})]
        if urllib.parse.urlsplit(url).scheme == "https":
            handlers.append(urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=self.ca_file)))
        try:
            with urllib.request.build_opener(*handlers).open(request, timeout=self.timeout_seconds) as response:
                body = json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")[:1000]
            raise RuntimeError(f"Prometheus query failed with HTTP {error.code}: {detail}") from error
        result = ((body.get("data") or {}).get("result") if isinstance(body, dict) else None)
        if body.get("status") != "success" or not isinstance(result, list) or len(result) != 1:
            raise RuntimeError(f"Prometheus scalar query returned {0 if not isinstance(result, list) else len(result)} series")
        value = result[0].get("value")
        if not isinstance(value, list) or len(value) != 2:
            raise RuntimeError("Prometheus result lacks a scalar sample")
        try:
            number = float(value[1])
        except (TypeError, ValueError) as error:
            raise RuntimeError(f"Prometheus returned invalid scalar: {value[1]!r}") from error
        if not math.isfinite(number) or number < 0:
            raise RuntimeError(f"Prometheus returned unsafe scalar: {number!r}")
        return number


def _bounded_int(value: float, label: str) -> int:
    rounded = round(value)
    if not math.isclose(value, rounded, abs_tol=1e-6):
        raise RuntimeError(f"{label} must be an integer counter, got {value}")
    return int(rounded)


def _validate_input_receipt(
    receipt: dict[str, Any], profile: dict[str, Any], kind: str, required: set[str]
) -> None:
    extras = set(receipt) - required
    missing = required - set(receipt)
    if extras or missing:
        raise ProfileBlocked(f"BLOCK_{kind}_RECEIPT_SHAPE", f"missing={sorted(missing)} extra={sorted(extras)}")
    scope = profile["scope"]
    mismatches = {
        "candidate_sha256": receipt["candidate_sha256"] != profile["candidate_sha256"],
        "tenant_id": receipt["tenant_id"] != scope["tenant_id"],
        "run_id": receipt["run_id"] != scope["run_id"],
    }
    if any(mismatches.values()):
        raise ProfileBlocked(f"BLOCK_{kind}_RECEIPT_BINDING", repr({k: v for k, v in mismatches.items() if v}))


def capture(
    profile: dict[str, Any],
    profile_sha256: str,
    coverage_receipt: dict[str, Any],
    parity_receipt: dict[str, Any],
    capture_receipt: dict[str, Any],
    metrics: MetricClient,
    clickhouse: Any,
    *,
    database: str,
) -> dict[str, Any]:
    validate_profile(profile)
    common = {"schema_version", "candidate_sha256", "tenant_id", "run_id"}
    _validate_input_receipt(coverage_receipt, profile, "COVERAGE", common | {"protocol_category_counts"})
    _validate_input_receipt(parity_receipt, profile, "PARITY", common | {"status", "unexplained_field_differences"})
    _validate_input_receipt(capture_receipt, profile, "CAPTURE", common | {
        "started_at", "ended_at", "duration_seconds", "system_attributable_drop_packets",
        "unexplained_difference_packets", "capture_error_count",
    })
    started = parse_time(capture_receipt["started_at"])
    ended = parse_time(capture_receipt["ended_at"])
    if ended <= started or int((ended - started).total_seconds()) != capture_receipt["duration_seconds"]:
        raise ProfileBlocked("BLOCK_CAPTURE_WINDOW", repr(capture_receipt))
    if capture_receipt["duration_seconds"] != profile["scope"]["duration_seconds"]:
        raise ProfileBlocked("BLOCK_CAPTURE_DURATION", str(capture_receipt["duration_seconds"]))

    observed = {name: metrics.scalar(query, ended) for name, query in RUNTIME_QUERIES.items()}
    reconciled = reconcile(
        clickhouse,
        database=database,
        tenant_id=profile["scope"]["tenant_id"],
        run_id=profile["scope"]["run_id"],
        required_nonempty={"flows_raw", "sessions", "feature_stat"},
    )
    projections = {
        item["table"]: {
            field: item[field]
            for field in ("unique_events", "duplicate_rows", "conflicting_events", "blank_event_id_rows")
        }
        for item in reconciled["tables"]
    }
    observation = {
        "schema_version": 1,
        "profile_sha256": profile_sha256,
        "candidate_sha256": profile["candidate_sha256"],
        "environment_id": profile["scope"]["environment_id"],
        "tenant_id": profile["scope"]["tenant_id"],
        "run_id": profile["scope"]["run_id"],
        "started_at": capture_receipt["started_at"],
        "ended_at": capture_receipt["ended_at"],
        "duration_seconds": capture_receipt["duration_seconds"],
        "capture": {field: capture_receipt[field] for field in (
            "system_attributable_drop_packets", "unexplained_difference_packets", "capture_error_count"
        )},
        "protocol_category_counts": coverage_receipt["protocol_category_counts"],
        "projections": projections,
        "flink_jobs": {
            "flink-session-job": {
                "running_tasks": _bounded_int(observed["session_running_tasks"], "session running tasks"),
                "expected_tasks": 24,
                "completed_checkpoints": _bounded_int(observed["session_completed_checkpoints"], "session completed checkpoints"),
                "failed_checkpoints": _bounded_int(observed["session_failed_checkpoints"], "session failed checkpoints"),
                "max_checkpoint_duration_ms": _bounded_int(observed["session_checkpoint_duration_ms"], "session checkpoint duration"),
            },
            "flink-feature-job": {
                "running_tasks": _bounded_int(observed["feature_running_tasks"], "feature running tasks"),
                "expected_tasks": 18,
                "completed_checkpoints": _bounded_int(observed["feature_completed_checkpoints"], "feature completed checkpoints"),
                "failed_checkpoints": _bounded_int(observed["feature_failed_checkpoints"], "feature failed checkpoints"),
                "max_checkpoint_duration_ms": _bounded_int(observed["feature_checkpoint_duration_ms"], "feature checkpoint duration"),
            },
        },
        "consumer_lag": {
            "flink-session-job": {
                "start": _bounded_int(metrics.scalar(RUNTIME_QUERIES["session_lag"], started), "session start lag"),
                "end": _bounded_int(observed["session_lag"], "session end lag"),
            },
            "flink-feature-job": {
                "start": _bounded_int(metrics.scalar(RUNTIME_QUERIES["feature_lag"], started), "feature start lag"),
                "end": _bounded_int(observed["feature_lag"], "feature end lag"),
            },
        },
        "sink_latency_p95_ms": {
            "session-clickhouse": observed["session_sink_latency_p95_ms"],
            "feature-clickhouse": observed["feature_sink_latency_p95_ms"],
        },
        "parity": {
            "status": parity_receipt["status"],
            "unexplained_field_differences": parity_receipt["unexplained_field_differences"],
        },
    }
    try:
        validate_against_schema(observation, OBSERVATION_SCHEMA)
    except ValueError as error:
        raise ProfileBlocked("BLOCK_CAPTURED_OBSERVATION_SCHEMA", str(error)) from error
    return observation


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", type=Path, required=True)
    parser.add_argument("--coverage-receipt", type=Path, required=True)
    parser.add_argument("--parity-receipt", type=Path, required=True)
    parser.add_argument("--capture-receipt", type=Path, required=True)
    parser.add_argument("--prometheus-endpoint", default=os.getenv("PROMETHEUS_URL", "http://127.0.0.1:9090"))
    parser.add_argument("--prometheus-token-env", default="PROMETHEUS_BEARER_TOKEN")
    parser.add_argument("--clickhouse-endpoint", default=os.getenv("CLICKHOUSE_HTTP_URL", "http://127.0.0.1:8123"))
    parser.add_argument("--clickhouse-database", default=os.getenv("CLICKHOUSE_DATABASE", "traffic"))
    parser.add_argument("--clickhouse-user", default=os.getenv("CLICKHOUSE_USER", "default"))
    parser.add_argument("--clickhouse-password-env", default="CLICKHOUSE_PASSWORD")
    parser.add_argument("--ca-file")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise SystemExit(f"refusing to overwrite observation: {args.output}")
    try:
        profile = load_json(args.profile)
        observation = capture(
            profile,
            sha256_path(args.profile),
            load_json(args.coverage_receipt),
            load_json(args.parity_receipt),
            load_json(args.capture_receipt),
            PrometheusHttpClient(
                args.prometheus_endpoint,
                bearer_token=os.getenv(args.prometheus_token_env, ""),
                ca_file=args.ca_file,
            ),
            ClickHouseHttpClient(
                endpoint=args.clickhouse_endpoint,
                user=args.clickhouse_user,
                password=os.getenv(args.clickhouse_password_env, ""),
                database=args.clickhouse_database,
                ca_file=args.ca_file,
            ),
            database=args.clickhouse_database,
        )
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "BLOCKED", "code": getattr(error, "code", "BLOCK_CAPTURE"), "detail": str(error)}, ensure_ascii=False))
        return 2
    args.output.write_text(json.dumps(observation, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": "CAPTURED_NOT_EVALUATED", "output": str(args.output), "sha256": sha256_path(args.output)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

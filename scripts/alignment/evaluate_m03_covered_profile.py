#!/usr/bin/env python3
"""Evaluate one M03 run only within an approved M02/M03 profile scope."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


ROOT = Path(__file__).resolve().parents[2]
PROFILE_SCHEMA = ROOT / "contracts/quality/m03-covered-profile.schema.json"
OBSERVATION_SCHEMA = ROOT / "contracts/quality/m03-covered-profile-observation.schema.json"
EXPECTED_AUTHORITIES = {"PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"}


class ProfileBlocked(ValueError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ProfileBlocked("BLOCK_JSON_ROOT", str(path))
    return value


def sha256_path(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def repo_path(value: str) -> Path:
    path = (ROOT / value).resolve(strict=False)
    if not path.is_relative_to(ROOT.resolve()):
        raise ProfileBlocked("BLOCK_PATH_ESCAPE", value)
    return path


def parse_time(value: str) -> dt.datetime:
    parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ProfileBlocked("BLOCK_NAIVE_TIME", value)
    return parsed


def _validate_approval(document: dict[str, Any], kind: str) -> None:
    if document.get("profile_status") != "APPROVED":
        raise ProfileBlocked(f"BLOCK_{kind}_NOT_APPROVED", str(document.get("profile_status")))
    approval = document.get("approval") or {}
    required = set(approval.get("required_authorities") or [])
    if required != EXPECTED_AUTHORITIES:
        raise ProfileBlocked(f"BLOCK_{kind}_AUTHORITY_SET", repr(sorted(required)))
    receipts = approval.get("receipts")
    if kind == "BASE_PROFILE":
        if not isinstance(receipts, list) or len(receipts) < 3 or any(not str(item).strip() for item in receipts):
            raise ProfileBlocked("BLOCK_BASE_PROFILE_RECEIPTS", repr(receipts))
    else:
        if not isinstance(receipts, dict) or set(receipts) != EXPECTED_AUTHORITIES:
            raise ProfileBlocked("BLOCK_M03_PROFILE_RECEIPTS", repr(receipts))
        if any(not isinstance(value, str) or not value.strip() for value in receipts.values()):
            raise ProfileBlocked("BLOCK_M03_PROFILE_RECEIPTS", "empty receipt")


def validate_base_profile(base: dict[str, Any]) -> None:
    _validate_approval(base, "BASE_PROFILE")
    if (
        base.get("schema_version") != "1.0.0"
        or base.get("artifact_kind") != "M02_CAPTURE_PROFILE"
        or not isinstance(base.get("line_rate_gbps"), (int, float))
        or base["line_rate_gbps"] < 10
    ):
        raise ProfileBlocked("BLOCK_BASE_PROFILE_IDENTITY", repr(base.get("profile_id")))
    traffic = base.get("traffic") or {}
    environment = base.get("environment") or {}
    thresholds = base.get("stop_thresholds") or {}
    required_environment = {
        "environment_id", "nic_model", "driver", "cpu_model", "kernel", "generator_identity"
    }
    if not isinstance(traffic.get("duration_seconds"), int) or traffic["duration_seconds"] < 60:
        raise ProfileBlocked("BLOCK_BASE_PROFILE_DURATION", repr(traffic))
    if set(environment) != required_environment or any(
        not isinstance(value, str) or not value.strip() or value.startswith("PENDING_")
        for value in environment.values()
    ):
        raise ProfileBlocked("BLOCK_BASE_PROFILE_ENVIRONMENT", repr(environment))
    expected_thresholds = {
        "system_attributable_drop_packets": 0,
        "unexplained_difference_packets": 0,
        "capture_error_count": 0,
    }
    if thresholds != expected_thresholds:
        raise ProfileBlocked("BLOCK_BASE_PROFILE_THRESHOLDS", repr(thresholds))


def validate_profile(profile: dict[str, Any]) -> dict[str, Any]:
    try:
        validate_against_schema(profile, PROFILE_SCHEMA)
    except ValueError as error:
        raise ProfileBlocked("BLOCK_M03_PROFILE_SCHEMA", str(error)) from error
    _validate_approval(profile, "M03_PROFILE")
    if not isinstance(profile.get("candidate_sha256"), str) or len(profile["candidate_sha256"]) != 64:
        raise ProfileBlocked("BLOCK_M03_CANDIDATE_IDENTITY", repr(profile.get("candidate_sha256")))
    if any(not isinstance(value, int) or isinstance(value, bool) for value in profile["expected_unique_events"].values()):
        raise ProfileBlocked("BLOCK_M03_EXPECTED_EVENT_COUNTS", repr(profile["expected_unique_events"]))
    base_ref = profile["base_m02_profile"]
    base_path = repo_path(base_ref["path"])
    if not base_path.is_file() or sha256_path(base_path) != base_ref["sha256"]:
        raise ProfileBlocked("BLOCK_BASE_PROFILE_DRIFT", str(base_path))
    base = load_json(base_path)
    validate_base_profile(base)
    corpus_ref = profile["golden_corpus"]
    corpus_path = repo_path(corpus_ref["path"])
    if not corpus_path.is_file() or sha256_path(corpus_path) != corpus_ref["sha256"]:
        raise ProfileBlocked("BLOCK_GOLDEN_CORPUS_DRIFT", str(corpus_path))
    corpus = load_json(corpus_path)
    if set(corpus.get("coverage") or []) != set(profile["required_protocol_categories"]):
        raise ProfileBlocked("BLOCK_PROTOCOL_DENOMINATOR_DRIFT", repr(corpus.get("coverage")))
    if profile["scope"]["duration_seconds"] != base["traffic"]["duration_seconds"]:
        raise ProfileBlocked("BLOCK_DURATION_DRIFT", repr(profile["scope"]))
    if profile["scope"]["environment_id"] != base["environment"]["environment_id"]:
        raise ProfileBlocked("BLOCK_ENVIRONMENT_DRIFT", repr(profile["scope"]))
    return base


def evaluate(profile: dict[str, Any], observation: dict[str, Any], profile_sha256: str) -> dict[str, Any]:
    validate_profile(profile)
    try:
        validate_against_schema(observation, OBSERVATION_SCHEMA)
    except ValueError as error:
        raise ProfileBlocked("BLOCK_OBSERVATION_SCHEMA", str(error)) from error
    scope = profile["scope"]
    bindings = {
        "profile_sha256": observation["profile_sha256"] == profile_sha256,
        "candidate_sha256": observation["candidate_sha256"] == profile["candidate_sha256"],
        "environment_id": observation["environment_id"] == scope["environment_id"],
        "tenant_id": observation["tenant_id"] == scope["tenant_id"],
        "run_id": observation["run_id"] == scope["run_id"],
        "duration_seconds": observation["duration_seconds"] == scope["duration_seconds"],
    }
    started = parse_time(observation["started_at"])
    ended = parse_time(observation["ended_at"])
    bindings["time_window"] = ended > started and int((ended - started).total_seconds()) == observation["duration_seconds"]
    if not all(bindings.values()):
        raise ProfileBlocked("BLOCK_OBSERVATION_BINDING", repr({k: v for k, v in bindings.items() if not v}))

    thresholds = profile["thresholds"]
    failures: list[dict[str, Any]] = []
    for metric in ("system_attributable_drop_packets", "unexplained_difference_packets", "capture_error_count"):
        actual = observation["capture"][metric]
        if actual > thresholds[metric]:
            failures.append({"metric": metric, "actual": actual, "limit": thresholds[metric]})

    required_categories = set(profile["required_protocol_categories"])
    category_counts = observation["protocol_category_counts"]
    if set(category_counts) != required_categories:
        failures.append({"metric": "protocol_category_exact_set", "actual": sorted(category_counts), "expected": sorted(required_categories)})
    for category, count in category_counts.items():
        if category != "empty" and count <= 0:
            failures.append({"metric": f"protocol_category:{category}", "actual": count, "limit": ">0"})

    for table, expected in profile["expected_unique_events"].items():
        actual = observation["projections"][table]
        if actual["unique_events"] != expected:
            failures.append({"metric": f"{table}.unique_events", "actual": actual["unique_events"], "expected": expected})
        for field in ("duplicate_rows", "conflicting_events", "blank_event_id_rows"):
            if actual[field] != 0:
                failures.append({"metric": f"{table}.{field}", "actual": actual[field], "limit": 0})

    for job_id, job in observation["flink_jobs"].items():
        if job["running_tasks"] != job["expected_tasks"]:
            failures.append({"metric": f"{job_id}.running_tasks", "actual": job["running_tasks"], "expected": job["expected_tasks"]})
        if job["completed_checkpoints"] <= 0:
            failures.append({"metric": f"{job_id}.completed_checkpoints", "actual": job["completed_checkpoints"], "limit": ">0"})
        if job["failed_checkpoints"] > thresholds["failed_checkpoints"]:
            failures.append({"metric": f"{job_id}.failed_checkpoints", "actual": job["failed_checkpoints"], "limit": thresholds["failed_checkpoints"]})
        if job["max_checkpoint_duration_ms"] > thresholds["max_checkpoint_duration_ms"]:
            failures.append({"metric": f"{job_id}.max_checkpoint_duration_ms", "actual": job["max_checkpoint_duration_ms"], "limit": thresholds["max_checkpoint_duration_ms"]})

    for group, lag in observation["consumer_lag"].items():
        growth = lag["end"] - lag["start"]
        if lag["end"] > thresholds["max_final_consumer_lag"]:
            failures.append({"metric": f"{group}.final_lag", "actual": lag["end"], "limit": thresholds["max_final_consumer_lag"]})
        if growth > thresholds["max_consumer_lag_growth"]:
            failures.append({"metric": f"{group}.lag_growth", "actual": growth, "limit": thresholds["max_consumer_lag_growth"]})
    for sink, latency in observation["sink_latency_p95_ms"].items():
        if latency > thresholds["max_sink_latency_p95_ms"]:
            failures.append({"metric": f"{sink}.p95_ms", "actual": latency, "limit": thresholds["max_sink_latency_p95_ms"]})
    if set(observation["sink_latency_p95_ms"]) != {"session-clickhouse", "feature-clickhouse"}:
        failures.append({
            "metric": "sink_latency_exact_set",
            "actual": sorted(observation["sink_latency_p95_ms"]),
            "expected": ["feature-clickhouse", "session-clickhouse"],
        })
    if observation["parity"]["status"] != "PASS" or observation["parity"]["unexplained_field_differences"] > thresholds["unexplained_field_differences"]:
        failures.append({"metric": "online_offline_parity", "actual": observation["parity"]})

    return {
        "schema_version": 1,
        "profile_id": profile["profile_id"],
        "candidate_sha256": profile["candidate_sha256"],
        "status": "PASS_FOR_COVERED_PROFILE" if not failures else "FAIL",
        "failures": failures,
        "claim_boundary": "valid only for the approved candidate, traffic, environment, protocol denominator and observation window; it is not all-protocol or general performance evidence",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", type=Path, required=True)
    parser.add_argument("--observation", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        profile = load_json(args.profile)
        result = evaluate(profile, load_json(args.observation), sha256_path(args.profile))
        exit_code = 0 if result["status"] == "PASS_FOR_COVERED_PROFILE" else 1
    except (OSError, json.JSONDecodeError, ProfileBlocked) as error:
        result = {"status": "BLOCKED", "code": getattr(error, "code", "BLOCK_IO_OR_JSON"), "detail": str(error)}
        exit_code = 2
    rendered = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        if args.output.exists():
            raise SystemExit(f"refusing to overwrite output: {args.output}")
        args.output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())

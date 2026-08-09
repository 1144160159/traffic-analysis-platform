#!/usr/bin/env python3
"""Verify repository-side T-FLINK-004 sink and reconciliation guards."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/flink/sink-reconciliation.v1.json"
APPLICATION_PATH = ROOT / "contracts/flink/application-cluster-migration.v1.json"
RUNTIME_DDL = re.compile(r"\b(?:CREATE|ALTER|DROP)\s+TABLE\b", re.IGNORECASE)


def _java_runtime_ddl() -> list[dict[str, Any]]:
    hits: list[dict[str, Any]] = []
    for source in sorted((ROOT / "java/flink-jobs").glob("*/src/main/java/**/*.java")):
        for line_number, line in enumerate(
            source.read_text(encoding="utf-8", errors="ignore").splitlines(), start=1
        ):
            if RUNTIME_DDL.search(line):
                hits.append({
                    "source": source.relative_to(ROOT).as_posix(),
                    "line": line_number,
                })
    return hits


def verify(
    contract: dict[str, Any] | None = None,
    application: dict[str, Any] | None = None,
) -> dict[str, Any]:
    contract = contract or json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    application = application or json.loads(APPLICATION_PATH.read_text(encoding="utf-8"))
    errors: list[str] = []

    canonical_jobs = {job["id"] for job in application["jobs"]}
    jobs = contract.get("jobs", [])
    contract_jobs = {job.get("id") for job in jobs}
    if canonical_jobs != contract_jobs:
        errors.append(
            f"canonical job drift missing={sorted(canonical_jobs - contract_jobs)} "
            f"unexpected={sorted(contract_jobs - canonical_jobs)}"
        )

    gap_count = 0
    for job in jobs:
        job_id = job.get("id", "unknown")
        if not job.get("reconciliation_keys"):
            errors.append(f"{job_id}: reconciliation key is required")
        if not job.get("sinks"):
            errors.append(f"{job_id}: sink inventory is required")
        gaps = job.get("gaps")
        if not isinstance(gaps, list):
            errors.append(f"{job_id}: gaps must be an explicit list")
        else:
            gap_count += len(gaps)
        if any("count-only" in key.lower() for key in job.get("reconciliation_keys", [])):
            errors.append(f"{job_id}: count-only reconciliation is forbidden")

    expected_metrics = {
        "input_total", "accepted_total", "dropped_total", "late_total",
        "failed_total", "dlq_total", "sink_success_total", "last_watermark",
    }
    actual_metrics = set(contract.get("required_job_metrics", []))
    if actual_metrics != expected_metrics:
        errors.append(
            f"metric contract drift missing={sorted(expected_metrics - actual_metrics)} "
            f"unexpected={sorted(actual_metrics - expected_metrics)}"
        )

    assertion_results: list[dict[str, Any]] = []
    for assertion in contract.get("source_assertions", []):
        relative = assertion["source"]
        source = ROOT / relative
        if not source.is_file():
            errors.append(f"missing sink source: {relative}")
            continue
        text = source.read_text(encoding="utf-8")
        missing = [token for token in assertion.get("required_tokens", []) if token not in text]
        forbidden = [token for token in assertion.get("forbidden_tokens", []) if token in text]
        ordering_failures = []
        for before, after in assertion.get("ordered_tokens", []):
            before_position = text.find(before)
            after_position = text.find(after)
            if before_position < 0 or after_position < 0 or before_position >= after_position:
                ordering_failures.append([before, after])
        if missing:
            errors.append(f"{relative}: missing tokens {missing}")
        if forbidden:
            errors.append(f"{relative}: forbidden tokens {forbidden}")
        if ordering_failures:
            errors.append(f"{relative}: ACK-before-clear ordering failures {ordering_failures}")
        assertion_results.append({
            "source": relative,
            "missing": missing,
            "forbidden": forbidden,
            "ordering_failures": ordering_failures,
        })

    schema_results: list[dict[str, Any]] = []
    for relative in contract.get("schema_sources", []):
        source = ROOT / relative
        if not source.is_file():
            errors.append(f"missing schema source: {relative}")
            continue
        text = source.read_text(encoding="utf-8")
        missing = [
            token for token in contract.get("schema_required_tokens", [])
            if token not in text
        ]
        if missing:
            errors.append(f"{relative}: schema tokens missing {missing}")
        schema_results.append({"source": relative, "missing": missing})

    runtime_ddl_hits = _java_runtime_ddl()
    if runtime_ddl_hits:
        errors.append(f"Flink runtime DDL is forbidden: {runtime_ddl_hits}")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": contract.get("remediation_id"),
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "COMPLETE" if not errors and gap_count == 0 else "PARTIAL",
        "canonical_jobs": len(canonical_jobs),
        "declared_sinks": sum(len(job.get("sinks", [])) for job in jobs),
        "declared_gaps": gap_count,
        "source_assertions": assertion_results,
        "schema_sources": schema_results,
        "runtime_ddl_hits": runtime_ddl_hits,
        "errors": errors,
        "remaining_gates": contract.get("gate", {}).get("remaining", []),
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

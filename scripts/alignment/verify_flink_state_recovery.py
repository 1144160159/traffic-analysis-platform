#!/usr/bin/env python3
"""Verify the T-FLINK-002 deterministic identity and recovery contract."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/flink/state-recovery.v1.json"
APPLICATION_PATH = ROOT / "contracts/flink/application-cluster-migration.v1.json"
ACL_PATH = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
UID_PATTERN = re.compile(r'\.uid\(\s*"([^"]+)"\s*\)')


def _production_java_sources() -> list[Path]:
    sources = []
    for source in (ROOT / "java/flink-jobs").glob("*/src/main/java/**/*.java"):
        relative = source.relative_to(ROOT).as_posix()
        if relative.startswith(
            "java/flink-jobs/flink-common/src/main/java/com/traffic/proto/"
        ):
            continue
        sources.append(source)
    return sorted(sources)


def verify(
    contract: dict[str, Any] | None = None,
    application: dict[str, Any] | None = None,
    acl: dict[str, Any] | None = None,
) -> dict[str, Any]:
    contract = contract or json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    application = application or json.loads(APPLICATION_PATH.read_text(encoding="utf-8"))
    acl = acl or json.loads(ACL_PATH.read_text(encoding="utf-8"))
    errors: list[str] = []

    production_sources = _production_java_sources()
    forbidden_hits: list[dict[str, Any]] = []
    for source in production_sources:
        text = source.read_text(encoding="utf-8")
        for token in contract["identity"]["forbidden_production_tokens"]:
            for line_number, line in enumerate(text.splitlines(), start=1):
                if token in line:
                    forbidden_hits.append(
                        {
                            "source": source.relative_to(ROOT).as_posix(),
                            "line": line_number,
                            "token": token,
                        }
                    )
    if forbidden_hits:
        errors.append(f"found {len(forbidden_hits)} replay-unstable identity tokens")

    deterministic_sources = []
    for relative in contract["identity"]["required_sources"]:
        source = ROOT / relative
        if not source.is_file():
            errors.append(f"missing deterministic identity source: {relative}")
            continue
        text = source.read_text(encoding="utf-8")
        if "DeterministicId" not in text:
            errors.append(f"{relative}: does not use DeterministicId")
            continue
        deterministic_sources.append(relative)

    app_by_id = {job["id"]: job for job in application["jobs"]}
    required_uids = contract["required_uids"]
    if set(app_by_id) != set(required_uids):
        errors.append("required_uids job set differs from canonical application contract")

    uid_results = []
    for job_id, expected in required_uids.items():
        job = app_by_id.get(job_id)
        if not job:
            continue
        source = (
            ROOT
            / "java/flink-jobs"
            / job["module"]
            / "src/main/java"
            / (job["main_class"].replace(".", "/") + ".java")
        )
        if not source.is_file():
            errors.append(f"{job_id}: main source is missing")
            continue
        actual = UID_PATTERN.findall(source.read_text(encoding="utf-8"))
        if len(actual) != len(set(actual)):
            errors.append(f"{job_id}: duplicate operator UID")
        missing = sorted(set(expected) - set(actual))
        unexpected = sorted(set(actual) - set(expected))
        if missing or unexpected:
            errors.append(
                f"{job_id}: UID drift missing={missing} unexpected={unexpected}"
            )
        uid_results.append(
            {
                "job_id": job_id,
                "required": len(expected),
                "found": len(actual),
                "missing": missing,
                "unexpected": unexpected,
            }
        )

    buffer_results = []
    for buffer_contract in contract["checkpointed_buffers"]:
        relative = buffer_contract["source"]
        source = ROOT / relative
        if not source.is_file():
            errors.append(f"missing checkpointed buffer source: {relative}")
            continue
        text = source.read_text(encoding="utf-8")
        required_tokens = (
            "CheckpointedFunction",
            "snapshotState(",
            "initializeState(",
            buffer_contract["state_name"],
            buffer_contract["ack_token"],
            buffer_contract["clear_token"],
        )
        missing_tokens = [token for token in required_tokens if token not in text]
        ack_position = text.find(buffer_contract["ack_token"])
        clear_position = text.find(buffer_contract["clear_token"])
        if missing_tokens:
            errors.append(f"{relative}: missing recovery tokens {missing_tokens}")
        elif ack_position >= clear_position:
            errors.append(f"{relative}: buffer clear is not ordered after external ACK")
        buffer_results.append(
            {
                "source": relative,
                "state_name": buffer_contract["state_name"],
                "ack_before_clear": not missing_tokens and ack_position < clear_position,
                "guarantee": buffer_contract["guarantee"],
            }
        )

    business_time = contract["business_time"]
    user_job = (
        ROOT
        / "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserBehaviorJob.java"
    ).read_text(encoding="utf-8")
    late_router = (
        ROOT
        / "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/LateUserEventRouter.java"
    ).read_text(encoding="utf-8")
    for token in (
        "forBoundedOutOfOrderness",
        business_time["watermark_config"],
        business_time["allowed_lateness_config"],
        business_time["durable_topic"],
    ):
        if token not in user_job:
            errors.append(f"user behavior business-time pipeline is missing {token}")
    if business_time["late_output_tag"] not in late_router:
        errors.append("late user-event OutputTag drift")

    dlq_binding = next(
        (binding for binding in acl["topic_bindings"]
         if binding["topic"] == business_time["durable_topic"]),
        None,
    )
    if not dlq_binding or business_time["producer_principal"] not in dlq_binding["producers"]:
        errors.append("late user-event producer lacks canonical DLQ ACL")

    async_boundary = contract["async_boundary"]
    behavior_job = (
        ROOT
        / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/BehaviorDetectionJob.java"
    ).read_text(encoding="utf-8")
    behavior_config = (
        ROOT
        / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/config/BehaviorJobConfig.java"
    ).read_text(encoding="utf-8")
    behavior_detector = (
        ROOT
        / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/BehaviorDetectorFunction.java"
    ).read_text(encoding="utf-8")
    if "AsyncDataStream.unorderedWait" not in behavior_job:
        errors.append("behavior async boundary is missing unorderedWait")
    for token in (
        async_boundary["timeout_config"],
        async_boundary["capacity_config"],
        async_boundary["retry_config"],
    ):
        if token not in behavior_config:
            errors.append(f"behavior async config is missing {token}")
    if "getAsyncMaxRetries()" not in behavior_detector or "inferWithRetry(" not in behavior_detector:
        errors.append("behavior async inference retry is not enforced")

    return {
        "schema_version": 1,
        "contract_id": contract["contract_id"],
        "remediation_id": contract["remediation_id"],
        "status": "PASS" if not errors else "FAIL",
        "canonical_jobs": len(app_by_id),
        "operator_uids": sum(item["found"] for item in uid_results),
        "deterministic_sources": len(deterministic_sources),
        "checkpointed_buffers": buffer_results,
        "uid_results": uid_results,
        "forbidden_hits": forbidden_hits,
        "errors": errors,
        "remaining_gates": contract["gate"]["remaining"],
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())

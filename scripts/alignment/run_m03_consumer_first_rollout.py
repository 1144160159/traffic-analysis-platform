#!/usr/bin/env python3
"""Evaluate and render the next M03 consumer-first rollout action.

This controller is deliberately non-mutating. It binds each transition to an
immutable candidate and ordered receipt, renders the exact Application Cluster
manifest for start actions, and emits a fail-closed rollback plan after the
first failed receipt. A deployment authority may execute the rendered action;
the controller never treats a planned action as runtime evidence.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema
from render_flink_application_cluster import load_json, render


ROOT = Path(__file__).resolve().parents[2]
PLAN_SCHEMA = ROOT / "contracts/flink/m03-consumer-first-rollout.v1.schema.json"
RECEIPT_SCHEMA = ROOT / "contracts/flink/m03-rollout-receipt.v1.schema.json"
STATE_CONTRACT = ROOT / "contracts/flink/state-recovery.v1.json"
APPLICATION_CONTRACT = ROOT / "contracts/flink/application-cluster-migration.v1.json"
SAVEPOINT_PREFIX = "s3://flink-checkpoints/savepoints/"

STAGES = [
    "STATIC_COMPATIBILITY_VERIFIED",
    "SESSION_SHADOW_VERIFIED",
    "FEATURE_SHADOW_VERIFIED",
    "OLD_SESSION_STOPPED_WITH_SAVEPOINT",
    "SESSION_PRODUCTION_VERIFIED",
    "OLD_FEATURE_STOPPED_WITH_SAVEPOINT",
    "FEATURE_PRODUCTION_VERIFIED",
    "PRODUCER_CANARY_VERIFIED",
    "RECONCILIATION_PASSED",
]
NEXT_ACTIONS = [
    "VERIFY_STATIC_COMPATIBILITY",
    "START_SESSION_SHADOW",
    "START_FEATURE_SHADOW",
    "STOP_OLD_SESSION_WITH_SAVEPOINT",
    "START_SESSION_PRODUCTION",
    "STOP_OLD_FEATURE_WITH_SAVEPOINT",
    "START_FEATURE_PRODUCTION",
    "ENABLE_PRODUCER_CANARY",
    "OBSERVE_AND_RECONCILE",
    "COMPLETE",
]
ROLLBACK_STEPS = [
    "DISABLE_PRODUCER_CANARY",
    "STOP_CANDIDATE_FEATURE",
    "STOP_CANDIDATE_SESSION",
    "RESTORE_OLD_SESSION_FROM_SAVEPOINT",
    "RESTORE_OLD_FEATURE_FROM_SAVEPOINT",
    "RECONCILE_OFFSETS_AND_SINKS",
]
JOB_BY_STAGE = {
    "SESSION_SHADOW_VERIFIED": "flink-session-job",
    "FEATURE_SHADOW_VERIFIED": "flink-feature-job",
    "OLD_SESSION_STOPPED_WITH_SAVEPOINT": "flink-session-job",
    "SESSION_PRODUCTION_VERIFIED": "flink-session-job",
    "OLD_FEATURE_STOPPED_WITH_SAVEPOINT": "flink-feature-job",
    "FEATURE_PRODUCTION_VERIFIED": "flink-feature-job",
}


class RolloutBlocked(ValueError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


def sha256_path(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def repo_path(value: str) -> Path:
    path = (ROOT / value).resolve(strict=False)
    if not path.is_relative_to(ROOT.resolve()):
        raise RolloutBlocked("BLOCK_PATH_ESCAPE", value)
    return path


def parse_time(value: str) -> dt.datetime:
    parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise RolloutBlocked("BLOCK_NAIVE_TIMESTAMP", value)
    return parsed


def load_plan(path: Path) -> dict[str, Any]:
    plan = load_json(path)
    try:
        validate_against_schema(plan, PLAN_SCHEMA)
    except ValueError as error:
        raise RolloutBlocked("BLOCK_PLAN_SCHEMA", str(error)) from error
    validate_plan(plan)
    return plan


def validate_plan(plan: dict[str, Any]) -> None:
    if plan["stages"] != STAGES:
        raise RolloutBlocked("BLOCK_STAGE_ORDER", repr(plan["stages"]))
    if plan["rollback_steps"] != ROLLBACK_STEPS:
        raise RolloutBlocked("BLOCK_ROLLBACK_ORDER", repr(plan["rollback_steps"]))
    observed_job_order = [item["job_id"] for item in plan["jobs"]]
    if observed_job_order != ["flink-session-job", "flink-feature-job"]:
        raise RolloutBlocked("BLOCK_JOB_ORDER", repr(observed_job_order))
    if plan["state_recovery_contract_sha256"] != sha256_path(STATE_CONTRACT):
        raise RolloutBlocked("BLOCK_STATE_CONTRACT_DRIFT", plan["state_recovery_contract_sha256"])
    app_contract = load_json(APPLICATION_CONTRACT)
    expected_tasks = {
        item["id"]: item["expected_tasks"]
        for item in app_contract["jobs"]
        if item["id"] in {"flink-session-job", "flink-feature-job"}
    }
    observed = {item["job_id"]: item["expected_tasks"] for item in plan["jobs"]}
    if observed != expected_tasks:
        raise RolloutBlocked("BLOCK_EXPECTED_TASK_DRIFT", repr(observed))
    savepoint_path = repo_path(plan["savepoint_manifest_path"])
    if not savepoint_path.is_file():
        raise RolloutBlocked("BLOCK_SAVEPOINT_MANIFEST_MISSING", str(savepoint_path))
    manifest = load_json(savepoint_path)
    for job in plan["jobs"]:
        entry = (manifest.get("savepoints") or {}).get(job["job_id"], {})
        if entry.get("source_job_id") != job["source_job_id"]:
            raise RolloutBlocked("BLOCK_SOURCE_JOB_DRIFT", job["job_id"])
        # The renderer owns the full URI/digest/source-cluster validation.
        render(job["job_id"], job["image"], manifest)


def load_receipts(paths: list[Path]) -> list[dict[str, Any]]:
    receipts: list[dict[str, Any]] = []
    for path in paths:
        receipt = load_json(path)
        try:
            validate_against_schema(receipt, RECEIPT_SCHEMA)
        except ValueError as error:
            raise RolloutBlocked("BLOCK_RECEIPT_SCHEMA", f"{path}: {error}") from error
        parse_time(receipt["observed_at"])
        receipts.append(receipt)
    return receipts


def _job(plan: dict[str, Any], job_id: str) -> dict[str, Any]:
    return next(item for item in plan["jobs"] if item["job_id"] == job_id)


def _initial_savepoint(plan: dict[str, Any], job_id: str) -> dict[str, Any]:
    manifest = load_json(repo_path(plan["savepoint_manifest_path"]))
    return manifest["savepoints"][job_id]


def _require_fields(evidence: dict[str, Any], stage: str, fields: set[str]) -> None:
    missing = sorted(fields - evidence.keys())
    if missing:
        raise RolloutBlocked("BLOCK_RECEIPT_FIELDS", f"{stage}: missing {missing}")


def validate_pass_evidence(
    plan: dict[str, Any], receipt: dict[str, Any], prior: list[dict[str, Any]]
) -> None:
    stage = receipt["stage"]
    evidence = receipt["evidence"]
    if stage == "STATIC_COMPATIBILITY_VERIFIED":
        _require_fields(evidence, stage, {
            "state_recovery_result", "state_recovery_contract_sha256", "savepoint_manifest_sha256"
        })
        expected_manifest = sha256_path(repo_path(plan["savepoint_manifest_path"]))
        if (
            evidence["state_recovery_result"] != "PASS"
            or evidence["state_recovery_contract_sha256"] != plan["state_recovery_contract_sha256"]
            or evidence["savepoint_manifest_sha256"] != expected_manifest
        ):
            raise RolloutBlocked("BLOCK_STATIC_COMPATIBILITY", repr(evidence))
        return

    if stage in {
        "SESSION_SHADOW_VERIFIED", "FEATURE_SHADOW_VERIFIED",
        "SESSION_PRODUCTION_VERIFIED", "FEATURE_PRODUCTION_VERIFIED",
    }:
        _require_fields(evidence, stage, {
            "job_id", "image", "activation", "consumer_group", "running_tasks",
            "expected_tasks", "restored_from_savepoint", "restored_savepoint_sha256",
            "completed_checkpoint_id", "failed_checkpoints", "external_writes",
        })
        job_id = JOB_BY_STAGE[stage]
        job = _job(plan, job_id)
        activation = "shadow" if "SHADOW" in stage else "production"
        expected_group = job_id
        if activation == "shadow":
            expected_group += "-shadow-" + plan["candidate_sha256"][:12]
        expected_savepoint = _initial_savepoint(plan, job_id)["sha256"]
        if activation == "production":
            stop_stage = (
                "OLD_SESSION_STOPPED_WITH_SAVEPOINT"
                if job_id == "flink-session-job"
                else "OLD_FEATURE_STOPPED_WITH_SAVEPOINT"
            )
            stop = next(item for item in prior if item["stage"] == stop_stage)
            expected_savepoint = stop["evidence"]["cutover_savepoint_sha256"]
        mismatches = {
            "job_id": evidence["job_id"] != job_id,
            "image": evidence["image"] != job["image"],
            "activation": evidence["activation"] != activation,
            "consumer_group": evidence["consumer_group"] != expected_group,
            "tasks": evidence["running_tasks"] != job["expected_tasks"]
            or evidence["expected_tasks"] != job["expected_tasks"],
            "restored": evidence["restored_from_savepoint"] is not True,
            "savepoint": evidence["restored_savepoint_sha256"] != expected_savepoint,
            "checkpoints": evidence["failed_checkpoints"] != 0,
            "shadow_writes": activation == "shadow" and evidence["external_writes"] != 0,
        }
        if any(mismatches.values()):
            raise RolloutBlocked("BLOCK_JOB_RECEIPT", repr({key: value for key, value in mismatches.items() if value}))
        return

    if stage in {"OLD_SESSION_STOPPED_WITH_SAVEPOINT", "OLD_FEATURE_STOPPED_WITH_SAVEPOINT"}:
        _require_fields(evidence, stage, {
            "job_id", "old_job_stopped", "shadow_job_stopped", "cutover_savepoint_uri",
            "cutover_savepoint_sha256", "cutover_source_job_id",
        })
        job_id = JOB_BY_STAGE[stage]
        if (
            evidence["job_id"] != job_id
            or evidence["old_job_stopped"] is not True
            or evidence["shadow_job_stopped"] is not True
        ):
            raise RolloutBlocked("BLOCK_OLD_JOB_NOT_STOPPED", job_id)
        uri = evidence["cutover_savepoint_uri"]
        if not uri.startswith(SAVEPOINT_PREFIX) or ".." in uri or "${" in uri:
            raise RolloutBlocked("BLOCK_CUTOVER_SAVEPOINT_URI", uri)
        return

    if stage == "PRODUCER_CANARY_VERIFIED":
        _require_fields(evidence, stage, {
            "producer_enabled", "tenant_id", "probe_ids", "event_contract_sha256",
            "session_checkpoint_id", "feature_checkpoint_id",
        })
        scope = plan["producer_scope"]
        if (
            evidence["producer_enabled"] is not True
            or evidence["tenant_id"] != scope["tenant_id"]
            or evidence["probe_ids"] != scope["probe_ids"]
            or evidence["event_contract_sha256"] != scope["event_contract_sha256"]
        ):
            raise RolloutBlocked("BLOCK_PRODUCER_SCOPE", repr(evidence))
        return

    if stage == "RECONCILIATION_PASSED":
        _require_fields(evidence, stage, {
            "event_reconciliation_result", "online_offline_parity_result",
            "conflicting_event_ids", "unexplained_field_differences",
        })
        if (
            evidence["event_reconciliation_result"] != "PASS"
            or evidence["online_offline_parity_result"] != "PASS"
            or evidence["conflicting_event_ids"] != 0
            or evidence["unexplained_field_differences"] != 0
        ):
            raise RolloutBlocked("BLOCK_RECONCILIATION", repr(evidence))


def evaluate(plan: dict[str, Any], receipts: list[dict[str, Any]]) -> dict[str, Any]:
    if len(receipts) > len(STAGES):
        raise RolloutBlocked("BLOCK_RECEIPT_COUNT", str(len(receipts)))
    prior: list[dict[str, Any]] = []
    for index, receipt in enumerate(receipts):
        expected_sequence = index + 1
        if receipt["rollout_id"] != plan["rollout_id"] or receipt["candidate_sha256"] != plan["candidate_sha256"]:
            raise RolloutBlocked("BLOCK_RECEIPT_BINDING", receipt["stage"])
        if receipt["sequence"] != expected_sequence or receipt["stage"] != STAGES[index]:
            raise RolloutBlocked("BLOCK_RECEIPT_ORDER", receipt["stage"])
        if receipt["status"] == "FAIL":
            _require_fields(receipt["evidence"], receipt["stage"], {"failure_code", "failure_detail"})
            if index != len(receipts) - 1:
                raise RolloutBlocked("BLOCK_RECEIPT_AFTER_FAILURE", receipt["stage"])
            return {
                "result": "rollback_required",
                "failed_stage": receipt["stage"],
                "failure_code": receipt["evidence"]["failure_code"],
                "next_action": ROLLBACK_STEPS[0],
                "rollback_steps": ROLLBACK_STEPS,
                "rollback_plan": [
                    {"action": action, "required": (
                        not action.startswith("RESTORE_OLD_")
                        or (
                            action == "RESTORE_OLD_SESSION_FROM_SAVEPOINT"
                            and any(item["stage"] == "OLD_SESSION_STOPPED_WITH_SAVEPOINT" for item in prior)
                        )
                        or (
                            action == "RESTORE_OLD_FEATURE_FROM_SAVEPOINT"
                            and any(item["stage"] == "OLD_FEATURE_STOPPED_WITH_SAVEPOINT" for item in prior)
                        )
                    )}
                    for action in ROLLBACK_STEPS
                ],
                "production_applied": index >= STAGES.index("SESSION_PRODUCTION_VERIFIED"),
            }
        validate_pass_evidence(plan, receipt, prior)
        prior.append(receipt)
    return {
        "result": "complete" if len(receipts) == len(STAGES) else "ready",
        "completed_stages": [item["stage"] for item in prior],
        "next_action": NEXT_ACTIONS[len(receipts)],
        "production_applied": len(receipts) > STAGES.index("SESSION_PRODUCTION_VERIFIED"),
        "producer_canary_enabled": len(receipts) > STAGES.index("PRODUCER_CANARY_VERIFIED"),
    }


def _cutover_manifest(plan: dict[str, Any], receipts: list[dict[str, Any]], job_id: str) -> dict[str, Any]:
    manifest = load_json(repo_path(plan["savepoint_manifest_path"]))
    stop_stage = (
        "OLD_SESSION_STOPPED_WITH_SAVEPOINT"
        if job_id == "flink-session-job"
        else "OLD_FEATURE_STOPPED_WITH_SAVEPOINT"
    )
    stop = next(item for item in receipts if item["stage"] == stop_stage)
    evidence = stop["evidence"]
    manifest["savepoints"][job_id] = {
        "uri": evidence["cutover_savepoint_uri"],
        "sha256": evidence["cutover_savepoint_sha256"],
        "source_job_id": evidence["cutover_source_job_id"],
    }
    return manifest


def render_next_action(plan: dict[str, Any], receipts: list[dict[str, Any]]) -> str:
    state = evaluate(plan, receipts)
    action = state["next_action"]
    if state["result"] == "rollback_required":
        return json.dumps(state, ensure_ascii=False, indent=2) + "\n"
    start_actions = {
        "START_SESSION_SHADOW": ("flink-session-job", "shadow"),
        "START_FEATURE_SHADOW": ("flink-feature-job", "shadow"),
        "START_SESSION_PRODUCTION": ("flink-session-job", "production"),
        "START_FEATURE_PRODUCTION": ("flink-feature-job", "production"),
    }
    if action in start_actions:
        job_id, activation = start_actions[action]
        job = _job(plan, job_id)
        manifest = load_json(repo_path(plan["savepoint_manifest_path"]))
        if activation == "production":
            manifest = _cutover_manifest(plan, receipts, job_id)
        return render(
            job_id,
            job["image"],
            manifest,
            activation,
            plan["candidate_sha256"],
            job["previous_image"],
        )
    payload: dict[str, Any] = {"action": action, "rollout_id": plan["rollout_id"]}
    if action == "VERIFY_STATIC_COMPATIBILITY":
        payload["command"] = ["python3", "scripts/alignment/verify_flink_state_recovery.py"]
        payload["required_contract_sha256"] = plan["state_recovery_contract_sha256"]
    elif action.startswith("STOP_OLD_"):
        job_id = "flink-session-job" if "SESSION" in action else "flink-feature-job"
        payload.update({
            "method": "FLINK_STOP_WITH_SAVEPOINT",
            "job_id": job_id,
            "source_job_id": _job(plan, job_id)["source_job_id"],
            "stop_shadow_cluster_first": (
                f"flink-app-{'session' if job_id == 'flink-session-job' else 'feature'}"
                f"-shadow-{plan['candidate_sha256'][:12]}"
            ),
            "drain": True,
            "target_directory": f"{SAVEPOINT_PREFIX}m03-cutover/{plan['rollout_id']}/{job_id}",
        })
    elif action == "ENABLE_PRODUCER_CANARY":
        payload["scope"] = plan["producer_scope"]
        payload["precondition"] = "both production consumer receipts passed with completed checkpoints"
    elif action == "OBSERVE_AND_RECONCILE":
        payload["commands"] = [
            ["python3", "scripts/alignment/reconcile_m03_clickhouse_events.py"],
            ["python3", "scripts/alignment/run_m03_online_offline_parity.py"],
        ]
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", type=Path, required=True)
    parser.add_argument("--receipt", action="append", type=Path, default=[])
    parser.add_argument("--render-next", action="store_true")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        plan = load_plan(args.plan)
        receipts = load_receipts(args.receipt)
        payload = render_next_action(plan, receipts) if args.render_next else (
            json.dumps(evaluate(plan, receipts), ensure_ascii=False, indent=2) + "\n"
        )
        if args.output:
            if args.output.exists():
                raise RolloutBlocked("BLOCK_OUTPUT_EXISTS", str(args.output))
            args.output.write_text(payload, encoding="utf-8")
        else:
            print(payload, end="")
        return 0
    except (OSError, ValueError) as error:
        code = error.code if isinstance(error, RolloutBlocked) else "BLOCK_IO_OR_JSON"
        print(json.dumps({"result": "blocked", "code": code, "detail": str(error)}, ensure_ascii=False))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

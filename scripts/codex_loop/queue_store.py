#!/usr/bin/env python3
"""Persistent queue store for Codex Loop scheduler and worker runs."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, rel_path, repo_path, write_json


QUEUE_ROOT = RUNS_ROOT / ".loop"
DEFAULT_QUEUE_PATH = QUEUE_ROOT / "queue.json"
DEFAULT_EVENTS_PATH = QUEUE_ROOT / "queue-events.jsonl"
QUEUE_VERSION = 1

CLAIMABLE_STATES = {"queued", "failed"}
TERMINAL_STATES = {"done", "quarantined"}


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def parse_time(value: str | None) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(value)
    except ValueError:
        return None


def default_queue() -> dict[str, Any]:
    return {
        "version": QUEUE_VERSION,
        "created_at": now_iso(),
        "updated_at": now_iso(),
        "items": [],
    }


def queue_path(path: str | Path | None = None) -> Path:
    return repo_path(path) if path else DEFAULT_QUEUE_PATH


def events_path(path: str | Path | None = None) -> Path:
    target = queue_path(path)
    return target.with_name("queue-events.jsonl")


def load_queue(path: str | Path | None = None) -> dict[str, Any]:
    target = queue_path(path)
    if not target.exists():
        return default_queue()
    data = json.loads(target.read_text(encoding="utf-8"))
    if "items" not in data:
        data["items"] = []
    return data


def save_queue(data: dict[str, Any], path: str | Path | None = None) -> Path:
    data["version"] = QUEUE_VERSION
    data["updated_at"] = now_iso()
    target = queue_path(path)
    write_json(target, data)
    return target


def append_event(event: dict[str, Any], path: str | Path | None = None) -> None:
    target = events_path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    payload = {"ts": now_iso(), **event}
    with target.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(payload, ensure_ascii=False) + "\n")


def item_history(item: dict[str, Any], event: dict[str, Any]) -> None:
    history = item.setdefault("history", [])
    history.append({"ts": now_iso(), **event})
    del history[:-20]


def find_item(data: dict[str, Any], task_id: str) -> dict[str, Any] | None:
    for item in data.get("items", []):
        if str(item.get("task_id")) == task_id:
            return item
    return None


def recover_expired_claims(data: dict[str, Any], path: str | Path | None = None) -> list[str]:
    recovered: list[str] = []
    current = datetime.now()
    for item in data.get("items", []):
        if item.get("state") != "claimed":
            continue
        expires_at = parse_time(item.get("claim_expires_at"))
        if expires_at and expires_at <= current:
            item["state"] = "queued"
            item["claimed_by"] = None
            item["claimed_at"] = None
            item["claim_expires_at"] = None
            item_history(item, {"event": "claim_expired"})
            append_event({"event": "claim_expired", "task_id": item.get("task_id")}, path)
            recovered.append(str(item.get("task_id")))
    return recovered


def queue_item_from_scheduler_item(item: dict[str, Any], plan: dict[str, Any]) -> dict[str, Any]:
    task_id = str(item.get("id"))
    return {
        "task_id": task_id,
        "title": item.get("title"),
        "priority": item.get("priority"),
        "mode": item.get("mode"),
        "data_mode": item.get("data_mode"),
        "lane_primary": item.get("lane_primary"),
        "lane_dependent": item.get("lane_dependent") or [],
        "subsystems": item.get("subsystems") or [],
        "acceptance_type": item.get("acceptance_type"),
        "risk_level": item.get("risk_level"),
        "resource_weight": item.get("resource_weight"),
        "resource_group": item.get("resource_group"),
        "score": item.get("score"),
        "allowed_paths": item.get("allowed_paths") or [],
        "task_path": item.get("path"),
        "source_scheduler_run_id": plan.get("run_id"),
        "scheduler_plan": f"doc/02_acceptance/runs/{plan.get('run_id')}/scheduler/scheduler-plan.json",
        "state": "queued",
        "attempts": int(item.get("attempt") or 1) - 1,
        "max_retries": int(item.get("max_retries") or 3),
        "created_at": now_iso(),
        "updated_at": now_iso(),
        "claimed_by": None,
        "claimed_at": None,
        "claim_expires_at": None,
        "last_worker_run_id": None,
        "last_result": None,
        "history": [{"ts": now_iso(), "event": "enqueued", "source_run_id": plan.get("run_id")}],
    }


def enqueue_plan(plan: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    data = load_queue(path)
    recover_expired_claims(data, path)
    enqueued: list[str] = []
    updated: list[str] = []
    skipped: list[dict[str, str]] = []
    for selected in plan.get("queue", {}).get("selected", []):
        task_id = str(selected.get("id"))
        existing = find_item(data, task_id)
        if existing and existing.get("state") in TERMINAL_STATES:
            skipped.append({"task_id": task_id, "reason": f"state {existing.get('state')}"})
            continue
        if existing:
            existing.update(
                {
                    "title": selected.get("title"),
                    "priority": selected.get("priority"),
                    "mode": selected.get("mode"),
                    "data_mode": selected.get("data_mode"),
                    "lane_primary": selected.get("lane_primary"),
                    "lane_dependent": selected.get("lane_dependent") or [],
                    "subsystems": selected.get("subsystems") or [],
                    "acceptance_type": selected.get("acceptance_type"),
                    "risk_level": selected.get("risk_level"),
                    "resource_weight": selected.get("resource_weight"),
                    "resource_group": selected.get("resource_group"),
                    "score": selected.get("score"),
                    "allowed_paths": selected.get("allowed_paths") or [],
                    "task_path": selected.get("path"),
                    "source_scheduler_run_id": plan.get("run_id"),
                    "scheduler_plan": f"doc/02_acceptance/runs/{plan.get('run_id')}/scheduler/scheduler-plan.json",
                    "updated_at": now_iso(),
                }
            )
            if existing.get("state") not in {"claimed", "failed"}:
                existing["state"] = "queued"
            item_history(existing, {"event": "updated_from_scheduler", "source_run_id": plan.get("run_id")})
            updated.append(task_id)
        else:
            data.setdefault("items", []).append(queue_item_from_scheduler_item(selected, plan))
            enqueued.append(task_id)
        append_event({"event": "enqueue_plan", "task_id": task_id, "source_run_id": plan.get("run_id")}, path)
    target = save_queue(data, path)
    return {
        "path": rel_path(target),
        "events_path": rel_path(events_path(path)),
        "enqueued": enqueued,
        "updated": updated,
        "skipped": skipped,
        "counts": queue_counts(data),
    }


def claim_item(task_id: str, worker_run_id: str, lease_seconds: int = 3600, path: str | Path | None = None) -> dict[str, Any]:
    data = load_queue(path)
    recover_expired_claims(data, path)
    item = find_item(data, task_id)
    if not item:
        return {"claimed": False, "task_id": task_id, "reason": "not queued"}
    state = str(item.get("state"))
    if state == "claimed":
        return {"claimed": False, "task_id": task_id, "reason": f"already claimed by {item.get('claimed_by')}"}
    if state in TERMINAL_STATES:
        return {"claimed": False, "task_id": task_id, "reason": f"state {state}"}
    if state not in CLAIMABLE_STATES:
        return {"claimed": False, "task_id": task_id, "reason": f"state {state}"}
    attempts = int(item.get("attempts") or 0)
    max_retries = int(item.get("max_retries") or 3)
    if attempts >= max_retries:
        item["state"] = "quarantined"
        item_history(item, {"event": "retry_budget_exhausted", "worker_run_id": worker_run_id})
        append_event({"event": "retry_budget_exhausted", "task_id": task_id, "worker_run_id": worker_run_id}, path)
        save_queue(data, path)
        return {"claimed": False, "task_id": task_id, "reason": "retry budget exhausted"}
    expires_at = datetime.now() + timedelta(seconds=lease_seconds)
    item.update(
        {
            "state": "claimed",
            "attempts": attempts + 1,
            "claimed_by": worker_run_id,
            "claimed_at": now_iso(),
            "claim_expires_at": expires_at.isoformat(timespec="seconds"),
            "updated_at": now_iso(),
            "last_worker_run_id": worker_run_id,
        }
    )
    item_history(item, {"event": "claimed", "worker_run_id": worker_run_id, "attempt": item["attempts"]})
    append_event({"event": "claimed", "task_id": task_id, "worker_run_id": worker_run_id, "attempt": item["attempts"]}, path)
    save_queue(data, path)
    return {
        "claimed": True,
        "task_id": task_id,
        "worker_run_id": worker_run_id,
        "attempt": item["attempts"],
        "max_retries": max_retries,
        "claim_expires_at": item["claim_expires_at"],
    }


def complete_item(task_id: str, worker_run_id: str, result: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    data = load_queue(path)
    item = find_item(data, task_id)
    if not item:
        return {"completed": False, "task_id": task_id, "reason": "not queued"}
    state = str(item.get("state"))
    if state != "claimed":
        return {"completed": False, "task_id": task_id, "reason": f"state {state}"}
    if str(item.get("claimed_by") or "") != worker_run_id:
        return {"completed": False, "task_id": task_id, "reason": f"claimed by {item.get('claimed_by')}"}
    item.update(
        {
            "state": "done",
            "claimed_by": None,
            "claimed_at": None,
            "claim_expires_at": None,
            "last_worker_run_id": worker_run_id,
            "last_result": result,
            "updated_at": now_iso(),
            "completed_at": now_iso(),
        }
    )
    item_history(item, {"event": "completed", "worker_run_id": worker_run_id, "result_status": result.get("status")})
    append_event({"event": "completed", "task_id": task_id, "worker_run_id": worker_run_id, "result_status": result.get("status")}, path)
    save_queue(data, path)
    return {"completed": True, "task_id": task_id, "state": "done"}


def fail_item(task_id: str, worker_run_id: str, result: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    data = load_queue(path)
    item = find_item(data, task_id)
    if not item:
        return {"failed": False, "task_id": task_id, "reason": "not queued"}
    state_before_fail = str(item.get("state"))
    if state_before_fail != "claimed":
        return {"failed": False, "task_id": task_id, "reason": f"state {state_before_fail}"}
    if str(item.get("claimed_by") or "") != worker_run_id:
        return {"failed": False, "task_id": task_id, "reason": f"claimed by {item.get('claimed_by')}"}
    attempts = int(item.get("attempts") or 0)
    max_retries = int(item.get("max_retries") or 3)
    state = "quarantined" if attempts >= max_retries else "failed"
    item.update(
        {
            "state": state,
            "claimed_by": None,
            "claimed_at": None,
            "claim_expires_at": None,
            "last_worker_run_id": worker_run_id,
            "last_result": result,
            "updated_at": now_iso(),
        }
    )
    item_history(item, {"event": "failed", "worker_run_id": worker_run_id, "state": state, "result_status": result.get("status")})
    append_event({"event": "failed", "task_id": task_id, "worker_run_id": worker_run_id, "state": state, "result_status": result.get("status")}, path)
    save_queue(data, path)
    return {"failed": True, "task_id": task_id, "state": state, "attempts": attempts, "max_retries": max_retries}


def queue_counts(data: dict[str, Any]) -> dict[str, int]:
    return dict(Counter(str(item.get("state", "unknown")) for item in data.get("items", [])))


def queue_status(path: str | Path | None = None) -> dict[str, Any]:
    data = load_queue(path)
    recovered = recover_expired_claims(data, path)
    if recovered:
        save_queue(data, path)
    return {
        "path": rel_path(queue_path(path)),
        "events_path": rel_path(events_path(path)),
        "updated_at": data.get("updated_at"),
        "counts": queue_counts(data),
        "total": len(data.get("items", [])),
        "recovered_claims": recovered,
        "items": data.get("items", []),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--queue-path", default=None)
    sub = parser.add_subparsers(dest="command", required=True)

    status_parser = sub.add_parser("status")
    status_parser.add_argument("--include-items", action="store_true")

    enqueue_parser = sub.add_parser("enqueue-plan")
    enqueue_parser.add_argument("--plan", required=True)

    claim_parser = sub.add_parser("claim")
    claim_parser.add_argument("--task-id", required=True)
    claim_parser.add_argument("--worker-run-id", required=True)
    claim_parser.add_argument("--lease-seconds", type=int, default=3600)

    complete_parser = sub.add_parser("complete")
    complete_parser.add_argument("--task-id", required=True)
    complete_parser.add_argument("--worker-run-id", required=True)

    fail_parser = sub.add_parser("fail")
    fail_parser.add_argument("--task-id", required=True)
    fail_parser.add_argument("--worker-run-id", required=True)

    args = parser.parse_args()
    if args.command == "status":
        status = queue_status(args.queue_path)
        if not args.include_items:
            status.pop("items", None)
        print(json.dumps(status, ensure_ascii=False, indent=2))
        return 0
    if args.command == "enqueue-plan":
        plan = json.loads(repo_path(args.plan).read_text(encoding="utf-8"))
        result = enqueue_plan(plan, args.queue_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    if args.command == "claim":
        result = claim_item(args.task_id, args.worker_run_id, args.lease_seconds, args.queue_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result.get("claimed") else 2
    if args.command == "complete":
        result = complete_item(args.task_id, args.worker_run_id, {"status": "manual_complete"}, args.queue_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result.get("completed") else 2
    if args.command == "fail":
        result = fail_item(args.task_id, args.worker_run_id, {"status": "manual_fail"}, args.queue_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result.get("failed") else 2
    return 2


if __name__ == "__main__":
    raise SystemExit(main())

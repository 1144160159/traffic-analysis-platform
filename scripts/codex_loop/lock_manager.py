#!/usr/bin/env python3
"""Manage Codex Loop workspace leases."""

from __future__ import annotations

import argparse
import json
import os
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any

from lib import repo_path, rel_path, write_json


LOCK_ROOT = repo_path("doc/02_acceptance/runs/.locks")
WORKSPACE_LOCK = LOCK_ROOT / "workspace.lock"


def now() -> datetime:
    return datetime.now()


def iso(value: datetime) -> str:
    return value.isoformat(timespec="seconds")


def parse_time(value: str | None) -> datetime | None:
    if not value:
        return None
    try:
        return datetime.fromisoformat(value)
    except ValueError:
        return None


def read_lock(path: Path = WORKSPACE_LOCK) -> dict[str, Any] | None:
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {"parse_error": True, "raw": path.read_text(encoding="utf-8")[:1000]}


def lock_expired(lock: dict[str, Any] | None, at: datetime | None = None) -> bool:
    if not lock:
        return False
    expires_at = parse_time(str(lock.get("expires_at") or ""))
    return bool(expires_at and expires_at <= (at or now()))


def acquire_lock(run_id: str, task_id: str, lease_seconds: int = 3600, holder: str = "codex-loop") -> dict[str, Any]:
    LOCK_ROOT.mkdir(parents=True, exist_ok=True)
    current = read_lock()
    if current and not lock_expired(current):
        return {
            "acquired": False,
            "path": rel_path(WORKSPACE_LOCK),
            "reason": "active lock exists",
            "current": current,
        }
    if current and lock_expired(current):
        archive = LOCK_ROOT / f"expired-{current.get('run_id', 'unknown')}-{int(now().timestamp())}.lock"
        WORKSPACE_LOCK.replace(archive)
    created = now()
    payload = {
        "run_id": run_id,
        "task_id": task_id,
        "holder": holder,
        "pid": os.getpid(),
        "created_at": iso(created),
        "heartbeat_at": iso(created),
        "expires_at": iso(created + timedelta(seconds=lease_seconds)),
        "lease_seconds": lease_seconds,
    }
    write_json(WORKSPACE_LOCK, payload)
    return {"acquired": True, "path": rel_path(WORKSPACE_LOCK), "payload": payload}


def heartbeat_lock(run_id: str, lease_seconds: int = 3600) -> dict[str, Any]:
    current = read_lock()
    if not current:
        return {"updated": False, "reason": "no lock"}
    if str(current.get("run_id")) != run_id:
        return {"updated": False, "reason": "lock belongs to another run", "current": current}
    stamp = now()
    current["heartbeat_at"] = iso(stamp)
    current["expires_at"] = iso(stamp + timedelta(seconds=lease_seconds))
    current["lease_seconds"] = lease_seconds
    write_json(WORKSPACE_LOCK, current)
    return {"updated": True, "path": rel_path(WORKSPACE_LOCK), "payload": current}


def release_lock(run_id: str | None = None, force: bool = False) -> dict[str, Any]:
    current = read_lock()
    if not current:
        return {"released": False, "reason": "no lock"}
    if not force and run_id and str(current.get("run_id")) != run_id:
        return {"released": False, "reason": "lock belongs to another run", "current": current}
    WORKSPACE_LOCK.unlink()
    return {"released": True, "previous": current}


def archive_expired_lock() -> dict[str, Any]:
    current = read_lock()
    if not current:
        return {"archived": False, "reason": "no lock"}
    if not lock_expired(current):
        return {"archived": False, "reason": "lock is still active", "current": current}
    LOCK_ROOT.mkdir(parents=True, exist_ok=True)
    archive = LOCK_ROOT / f"expired-{current.get('run_id', 'unknown')}-{int(now().timestamp())}.lock"
    WORKSPACE_LOCK.replace(archive)
    return {"archived": True, "path": rel_path(archive), "previous": current}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["status", "acquire", "heartbeat", "release", "recover-expired"])
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--task-id", default="unknown")
    parser.add_argument("--lease-seconds", type=int, default=3600)
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()

    if args.action == "status":
        result = {"lock": read_lock(), "expired": lock_expired(read_lock())}
    elif args.action == "acquire":
        if not args.run_id:
            raise SystemExit("--run-id is required for acquire")
        result = acquire_lock(args.run_id, args.task_id, args.lease_seconds)
    elif args.action == "heartbeat":
        if not args.run_id:
            raise SystemExit("--run-id is required for heartbeat")
        result = heartbeat_lock(args.run_id, args.lease_seconds)
    elif args.action == "recover-expired":
        result = archive_expired_lock()
    else:
        result = release_lock(args.run_id, args.force)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""SQLite-backed transactional queue store for Codex Loop."""

from __future__ import annotations

import argparse
import json
import sqlite3
from collections import Counter
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, rel_path, repo_path


QUEUE_ROOT = RUNS_ROOT / ".loop"
DEFAULT_DB_PATH = QUEUE_ROOT / "queue.sqlite3"
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


def db_path(path: str | Path | None = None) -> Path:
    return repo_path(path) if path else DEFAULT_DB_PATH


def connect(path: str | Path | None = None) -> sqlite3.Connection:
    target = db_path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(target), timeout=30, isolation_level=None)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA busy_timeout=30000")
    return conn


def init_db(path: str | Path | None = None) -> Path:
    with connect(path) as conn:
        conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS queue_meta (
              key TEXT PRIMARY KEY,
              value TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS queue_items (
              task_id TEXT PRIMARY KEY,
              title TEXT,
              priority TEXT,
              mode TEXT,
              data_mode TEXT,
              lane_primary TEXT,
              lane_dependent_json TEXT NOT NULL DEFAULT '[]',
              subsystems_json TEXT NOT NULL DEFAULT '[]',
              acceptance_type TEXT,
              risk_level TEXT,
              resource_weight INTEGER,
              resource_group TEXT,
              score INTEGER,
              allowed_paths_json TEXT NOT NULL DEFAULT '[]',
              task_path TEXT,
              source_scheduler_run_id TEXT,
              scheduler_plan TEXT,
              state TEXT NOT NULL,
              attempts INTEGER NOT NULL DEFAULT 0,
              max_retries INTEGER NOT NULL DEFAULT 3,
              created_at TEXT NOT NULL,
              updated_at TEXT NOT NULL,
              claimed_by TEXT,
              claimed_at TEXT,
              claim_expires_at TEXT,
              last_worker_run_id TEXT,
              last_result_json TEXT,
              completed_at TEXT,
              history_json TEXT NOT NULL DEFAULT '[]'
            );
            CREATE TABLE IF NOT EXISTS queue_events (
              id INTEGER PRIMARY KEY AUTOINCREMENT,
              ts TEXT NOT NULL,
              event TEXT NOT NULL,
              task_id TEXT,
              worker_run_id TEXT,
              source_run_id TEXT,
              payload_json TEXT NOT NULL DEFAULT '{}'
            );
            CREATE INDEX IF NOT EXISTS idx_queue_items_state ON queue_items(state);
            CREATE INDEX IF NOT EXISTS idx_queue_events_task ON queue_events(task_id);
            """
        )
        conn.execute(
            "INSERT OR REPLACE INTO queue_meta(key, value) VALUES(?, ?)",
            ("version", str(QUEUE_VERSION)),
        )
        conn.execute(
            "INSERT OR IGNORE INTO queue_meta(key, value) VALUES(?, ?)",
            ("created_at", now_iso()),
        )
        conn.execute(
            "INSERT OR IGNORE INTO queue_meta(key, value) VALUES(?, ?)",
            ("updated_at", now_iso()),
        )
        ensure_columns(conn)
    return db_path(path)


def ensure_columns(conn: sqlite3.Connection) -> None:
    existing = {row["name"] for row in conn.execute("PRAGMA table_info(queue_items)").fetchall()}
    columns = {
        "data_mode": "TEXT",
        "lane_primary": "TEXT",
        "lane_dependent_json": "TEXT NOT NULL DEFAULT '[]'",
        "subsystems_json": "TEXT NOT NULL DEFAULT '[]'",
        "acceptance_type": "TEXT",
        "risk_level": "TEXT",
        "resource_weight": "INTEGER",
        "resource_group": "TEXT",
    }
    for name, definition in columns.items():
        if name not in existing:
            conn.execute(f"ALTER TABLE queue_items ADD COLUMN {name} {definition}")


def json_dumps(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False)


def json_loads(value: str | None, default: Any) -> Any:
    if not value:
        return default
    try:
        return json.loads(value)
    except json.JSONDecodeError:
        return default


def append_history(row: sqlite3.Row | dict[str, Any] | None, event: dict[str, Any]) -> str:
    history = json_loads(row["history_json"] if row else None, [])
    history.append({"ts": now_iso(), **event})
    return json_dumps(history[-20:])


def append_event(conn: sqlite3.Connection, event: dict[str, Any]) -> None:
    conn.execute(
        """
        INSERT INTO queue_events(ts, event, task_id, worker_run_id, source_run_id, payload_json)
        VALUES(?, ?, ?, ?, ?, ?)
        """,
        (
            now_iso(),
            str(event.get("event")),
            event.get("task_id"),
            event.get("worker_run_id"),
            event.get("source_run_id"),
            json_dumps(event),
        ),
    )


def row_to_item(row: sqlite3.Row) -> dict[str, Any]:
    return {
        "task_id": row["task_id"],
        "title": row["title"],
        "priority": row["priority"],
        "mode": row["mode"],
        "data_mode": row["data_mode"],
        "lane_primary": row["lane_primary"],
        "lane_dependent": json_loads(row["lane_dependent_json"], []),
        "subsystems": json_loads(row["subsystems_json"], []),
        "acceptance_type": row["acceptance_type"],
        "risk_level": row["risk_level"],
        "resource_weight": row["resource_weight"],
        "resource_group": row["resource_group"],
        "score": row["score"],
        "allowed_paths": json_loads(row["allowed_paths_json"], []),
        "task_path": row["task_path"],
        "source_scheduler_run_id": row["source_scheduler_run_id"],
        "scheduler_plan": row["scheduler_plan"],
        "state": row["state"],
        "attempts": row["attempts"],
        "max_retries": row["max_retries"],
        "created_at": row["created_at"],
        "updated_at": row["updated_at"],
        "claimed_by": row["claimed_by"],
        "claimed_at": row["claimed_at"],
        "claim_expires_at": row["claim_expires_at"],
        "last_worker_run_id": row["last_worker_run_id"],
        "last_result": json_loads(row["last_result_json"], None),
        "completed_at": row["completed_at"],
        "history": json_loads(row["history_json"], []),
    }


def recover_expired_claims(path: str | Path | None = None) -> list[str]:
    init_db(path)
    recovered: list[str] = []
    current = datetime.now()
    with connect(path) as conn:
        conn.execute("BEGIN IMMEDIATE")
        rows = conn.execute("SELECT * FROM queue_items WHERE state = 'claimed'").fetchall()
        for row in rows:
            expires_at = parse_time(row["claim_expires_at"])
            if expires_at and expires_at <= current:
                history = append_history(row, {"event": "claim_expired"})
                conn.execute(
                    """
                    UPDATE queue_items
                    SET state='queued', claimed_by=NULL, claimed_at=NULL, claim_expires_at=NULL,
                        updated_at=?, history_json=?
                    WHERE task_id=?
                    """,
                    (now_iso(), history, row["task_id"]),
                )
                append_event(conn, {"event": "claim_expired", "task_id": row["task_id"]})
                recovered.append(str(row["task_id"]))
        if recovered:
            conn.execute("INSERT OR REPLACE INTO queue_meta(key, value) VALUES('updated_at', ?)", (now_iso(),))
        conn.execute("COMMIT")
    return recovered


def enqueue_plan(plan: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    init_db(path)
    recovered = recover_expired_claims(path)
    enqueued: list[str] = []
    updated: list[str] = []
    skipped: list[dict[str, str]] = []
    scheduler_plan = f"doc/02_acceptance/runs/{plan.get('run_id')}/scheduler/scheduler-plan.json"
    with connect(path) as conn:
        conn.execute("BEGIN IMMEDIATE")
        for selected in plan.get("queue", {}).get("selected", []):
            task_id = str(selected.get("id"))
            row = conn.execute("SELECT * FROM queue_items WHERE task_id=?", (task_id,)).fetchone()
            if row and row["state"] in TERMINAL_STATES:
                skipped.append({"task_id": task_id, "reason": f"state {row['state']}"})
                continue
            now = now_iso()
            if row:
                state = row["state"] if row["state"] in {"claimed", "failed"} else "queued"
                history = append_history(row, {"event": "updated_from_scheduler", "source_run_id": plan.get("run_id")})
                conn.execute(
                    """
                    UPDATE queue_items
                    SET title=?, priority=?, mode=?, data_mode=?, lane_primary=?, lane_dependent_json=?,
                        subsystems_json=?, acceptance_type=?, risk_level=?, resource_weight=?, resource_group=?,
                        score=?, allowed_paths_json=?, task_path=?,
                        source_scheduler_run_id=?, scheduler_plan=?, state=?, updated_at=?, history_json=?
                    WHERE task_id=?
                    """,
                    (
                        selected.get("title"),
                        selected.get("priority"),
                        selected.get("mode"),
                        selected.get("data_mode"),
                        selected.get("lane_primary"),
                        json_dumps(selected.get("lane_dependent") or []),
                        json_dumps(selected.get("subsystems") or []),
                        selected.get("acceptance_type"),
                        selected.get("risk_level"),
                        selected.get("resource_weight"),
                        selected.get("resource_group"),
                        selected.get("score"),
                        json_dumps(selected.get("allowed_paths") or []),
                        selected.get("path"),
                        plan.get("run_id"),
                        scheduler_plan,
                        state,
                        now,
                        history,
                        task_id,
                    ),
                )
                updated.append(task_id)
            else:
                history = json_dumps([{"ts": now, "event": "enqueued", "source_run_id": plan.get("run_id")}])
                conn.execute(
                    """
                    INSERT INTO queue_items(
                      task_id, title, priority, mode, data_mode, lane_primary, lane_dependent_json,
                      subsystems_json, acceptance_type, risk_level, resource_weight, resource_group,
                      score, allowed_paths_json, task_path,
                      source_scheduler_run_id, scheduler_plan, state, attempts, max_retries,
                      created_at, updated_at, history_json
                    ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?)
                    """,
                    (
                        task_id,
                        selected.get("title"),
                        selected.get("priority"),
                        selected.get("mode"),
                        selected.get("data_mode"),
                        selected.get("lane_primary"),
                        json_dumps(selected.get("lane_dependent") or []),
                        json_dumps(selected.get("subsystems") or []),
                        selected.get("acceptance_type"),
                        selected.get("risk_level"),
                        selected.get("resource_weight"),
                        selected.get("resource_group"),
                        selected.get("score"),
                        json_dumps(selected.get("allowed_paths") or []),
                        selected.get("path"),
                        plan.get("run_id"),
                        scheduler_plan,
                        int(selected.get("attempt") or 1) - 1,
                        int(selected.get("max_retries") or 3),
                        now,
                        now,
                        history,
                    ),
                )
                enqueued.append(task_id)
            append_event(conn, {"event": "enqueue_plan", "task_id": task_id, "source_run_id": plan.get("run_id")})
        conn.execute("INSERT OR REPLACE INTO queue_meta(key, value) VALUES('updated_at', ?)", (now_iso(),))
        conn.execute("COMMIT")
    status = queue_status(path)
    return {
        "backend": "sqlite",
        "path": rel_path(db_path(path)),
        "enqueued": enqueued,
        "updated": updated,
        "skipped": skipped,
        "recovered_claims": recovered,
        "counts": status["counts"],
    }


def claim_item(task_id: str, worker_run_id: str, lease_seconds: int = 3600, path: str | Path | None = None) -> dict[str, Any]:
    init_db(path)
    recover_expired_claims(path)
    with connect(path) as conn:
        conn.execute("BEGIN IMMEDIATE")
        row = conn.execute("SELECT * FROM queue_items WHERE task_id=?", (task_id,)).fetchone()
        if not row:
            conn.execute("ROLLBACK")
            return {"claimed": False, "task_id": task_id, "reason": "not queued"}
        state = str(row["state"])
        if state == "claimed":
            conn.execute("ROLLBACK")
            return {"claimed": False, "task_id": task_id, "reason": f"already claimed by {row['claimed_by']}"}
        if state in TERMINAL_STATES:
            conn.execute("ROLLBACK")
            return {"claimed": False, "task_id": task_id, "reason": f"state {state}"}
        if state not in CLAIMABLE_STATES:
            conn.execute("ROLLBACK")
            return {"claimed": False, "task_id": task_id, "reason": f"state {state}"}
        attempts = int(row["attempts"] or 0)
        max_retries = int(row["max_retries"] or 3)
        if attempts >= max_retries:
            history = append_history(row, {"event": "retry_budget_exhausted", "worker_run_id": worker_run_id})
            conn.execute(
                "UPDATE queue_items SET state='quarantined', updated_at=?, history_json=? WHERE task_id=?",
                (now_iso(), history, task_id),
            )
            append_event(conn, {"event": "retry_budget_exhausted", "task_id": task_id, "worker_run_id": worker_run_id})
            conn.execute("COMMIT")
            return {"claimed": False, "task_id": task_id, "reason": "retry budget exhausted"}
        expires_at = datetime.now() + timedelta(seconds=lease_seconds)
        claimed_at = now_iso()
        history = append_history(row, {"event": "claimed", "worker_run_id": worker_run_id, "attempt": attempts + 1})
        conn.execute(
            """
            UPDATE queue_items
            SET state='claimed', attempts=?, claimed_by=?, claimed_at=?, claim_expires_at=?,
                updated_at=?, last_worker_run_id=?, history_json=?
            WHERE task_id=?
            """,
            (attempts + 1, worker_run_id, claimed_at, expires_at.isoformat(timespec="seconds"), claimed_at, worker_run_id, history, task_id),
        )
        append_event(conn, {"event": "claimed", "task_id": task_id, "worker_run_id": worker_run_id, "attempt": attempts + 1})
        conn.execute("INSERT OR REPLACE INTO queue_meta(key, value) VALUES('updated_at', ?)", (claimed_at,))
        conn.execute("COMMIT")
    return {
        "claimed": True,
        "backend": "sqlite",
        "task_id": task_id,
        "worker_run_id": worker_run_id,
        "attempt": attempts + 1,
        "max_retries": max_retries,
        "claim_expires_at": expires_at.isoformat(timespec="seconds"),
    }


def complete_item(task_id: str, worker_run_id: str, result: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    init_db(path)
    with connect(path) as conn:
        conn.execute("BEGIN IMMEDIATE")
        row = conn.execute("SELECT * FROM queue_items WHERE task_id=?", (task_id,)).fetchone()
        if not row:
            conn.execute("ROLLBACK")
            return {"completed": False, "task_id": task_id, "reason": "not queued"}
        state = str(row["state"])
        if state != "claimed":
            conn.execute("ROLLBACK")
            return {"completed": False, "task_id": task_id, "reason": f"state {state}"}
        if str(row["claimed_by"] or "") != worker_run_id:
            conn.execute("ROLLBACK")
            return {"completed": False, "task_id": task_id, "reason": f"claimed by {row['claimed_by']}"}
        stamp = now_iso()
        history = append_history(row, {"event": "completed", "worker_run_id": worker_run_id, "result_status": result.get("status")})
        conn.execute(
            """
            UPDATE queue_items
            SET state='done', claimed_by=NULL, claimed_at=NULL, claim_expires_at=NULL,
                last_worker_run_id=?, last_result_json=?, updated_at=?, completed_at=?, history_json=?
            WHERE task_id=?
            """,
            (worker_run_id, json_dumps(result), stamp, stamp, history, task_id),
        )
        append_event(conn, {"event": "completed", "task_id": task_id, "worker_run_id": worker_run_id, "result_status": result.get("status")})
        conn.execute("INSERT OR REPLACE INTO queue_meta(key, value) VALUES('updated_at', ?)", (stamp,))
        conn.execute("COMMIT")
    return {"completed": True, "backend": "sqlite", "task_id": task_id, "state": "done"}


def fail_item(task_id: str, worker_run_id: str, result: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    init_db(path)
    with connect(path) as conn:
        conn.execute("BEGIN IMMEDIATE")
        row = conn.execute("SELECT * FROM queue_items WHERE task_id=?", (task_id,)).fetchone()
        if not row:
            conn.execute("ROLLBACK")
            return {"failed": False, "task_id": task_id, "reason": "not queued"}
        state = str(row["state"])
        if state != "claimed":
            conn.execute("ROLLBACK")
            return {"failed": False, "task_id": task_id, "reason": f"state {state}"}
        if str(row["claimed_by"] or "") != worker_run_id:
            conn.execute("ROLLBACK")
            return {"failed": False, "task_id": task_id, "reason": f"claimed by {row['claimed_by']}"}
        attempts = int(row["attempts"] or 0)
        max_retries = int(row["max_retries"] or 3)
        state = "quarantined" if attempts >= max_retries else "failed"
        history = append_history(row, {"event": "failed", "worker_run_id": worker_run_id, "state": state, "result_status": result.get("status")})
        conn.execute(
            """
            UPDATE queue_items
            SET state=?, claimed_by=NULL, claimed_at=NULL, claim_expires_at=NULL,
                last_worker_run_id=?, last_result_json=?, updated_at=?, history_json=?
            WHERE task_id=?
            """,
            (state, worker_run_id, json_dumps(result), now_iso(), history, task_id),
        )
        append_event(conn, {"event": "failed", "task_id": task_id, "worker_run_id": worker_run_id, "state": state, "result_status": result.get("status")})
        conn.execute("INSERT OR REPLACE INTO queue_meta(key, value) VALUES('updated_at', ?)", (now_iso(),))
        conn.execute("COMMIT")
    return {"failed": True, "backend": "sqlite", "task_id": task_id, "state": state, "attempts": attempts, "max_retries": max_retries}


def queue_status(path: str | Path | None = None, include_items: bool = True) -> dict[str, Any]:
    init_db(path)
    recovered = recover_expired_claims(path)
    with connect(path) as conn:
        rows = conn.execute("SELECT * FROM queue_items ORDER BY updated_at DESC, task_id").fetchall()
        counts = dict(Counter(str(row["state"]) for row in rows))
        updated = conn.execute("SELECT value FROM queue_meta WHERE key='updated_at'").fetchone()
    result = {
        "backend": "sqlite",
        "path": rel_path(db_path(path)),
        "updated_at": updated["value"] if updated else None,
        "counts": counts,
        "total": len(rows),
        "recovered_claims": recovered,
    }
    if include_items:
        result["items"] = [row_to_item(row) for row in rows]
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db-path", default=None)
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("init")

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
    if args.command == "init":
        path = init_db(args.db_path)
        print(json.dumps({"initialized": True, "path": rel_path(path)}, ensure_ascii=False, indent=2))
        return 0
    if args.command == "status":
        status = queue_status(args.db_path, include_items=args.include_items)
        print(json.dumps(status, ensure_ascii=False, indent=2))
        return 0
    if args.command == "enqueue-plan":
        plan = json.loads(repo_path(args.plan).read_text(encoding="utf-8"))
        result = enqueue_plan(plan, args.db_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    if args.command == "claim":
        result = claim_item(args.task_id, args.worker_run_id, args.lease_seconds, args.db_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result.get("claimed") else 2
    if args.command == "complete":
        result = complete_item(args.task_id, args.worker_run_id, {"status": "manual_complete"}, args.db_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result.get("completed") else 2
    if args.command == "fail":
        result = fail_item(args.task_id, args.worker_run_id, {"status": "manual_fail"}, args.db_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result.get("failed") else 2
    return 2


if __name__ == "__main__":
    raise SystemExit(main())

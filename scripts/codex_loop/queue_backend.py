#!/usr/bin/env python3
"""Queue backend dispatcher for Codex Loop."""

from __future__ import annotations

from pathlib import Path
from typing import Any

from lib import rel_path, repo_path

import queue_sqlite
import queue_store
import queue_http


JSON_BACKENDS = {"json", "repo-json", "repo_json"}
SQLITE_BACKENDS = {"sqlite", "sqlite3"}
HTTP_BACKENDS = {"http", "https", "queue-http", "queue_http", "remote"}


def normalize_backend(backend: str | None) -> str:
    value = (backend or "repo-json").strip().lower()
    if value in JSON_BACKENDS:
        return "repo-json"
    if value in SQLITE_BACKENDS:
        return "sqlite"
    if value in HTTP_BACKENDS:
        return "http"
    raise ValueError(f"Unsupported queue backend `{backend}`")


def display_path(path: str | Path | None) -> str | None:
    if not path:
        return None
    value = str(path)
    if value.startswith(("http://", "https://")):
        return value.rstrip("/")
    return rel_path(repo_path(value))


def enqueue_plan(plan: dict[str, Any], backend: str | None = None, path: str | Path | None = None) -> dict[str, Any]:
    selected = normalize_backend(backend)
    if selected == "http":
        return queue_http.enqueue_plan(plan, path)
    if selected == "sqlite":
        return queue_sqlite.enqueue_plan(plan, path)
    result = queue_store.enqueue_plan(plan, path)
    result["backend"] = "repo-json"
    return result


def claim_item(task_id: str, worker_run_id: str, lease_seconds: int = 3600, backend: str | None = None, path: str | Path | None = None) -> dict[str, Any]:
    selected = normalize_backend(backend)
    if selected == "http":
        return queue_http.claim_item(task_id, worker_run_id, lease_seconds, path)
    if selected == "sqlite":
        return queue_sqlite.claim_item(task_id, worker_run_id, lease_seconds, path)
    result = queue_store.claim_item(task_id, worker_run_id, lease_seconds, path)
    result["backend"] = "repo-json"
    return result


def complete_item(task_id: str, worker_run_id: str, result: dict[str, Any], backend: str | None = None, path: str | Path | None = None) -> dict[str, Any]:
    selected = normalize_backend(backend)
    if selected == "http":
        return queue_http.complete_item(task_id, worker_run_id, result, path)
    if selected == "sqlite":
        return queue_sqlite.complete_item(task_id, worker_run_id, result, path)
    data = queue_store.complete_item(task_id, worker_run_id, result, path)
    data["backend"] = "repo-json"
    return data


def fail_item(task_id: str, worker_run_id: str, result: dict[str, Any], backend: str | None = None, path: str | Path | None = None) -> dict[str, Any]:
    selected = normalize_backend(backend)
    if selected == "http":
        return queue_http.fail_item(task_id, worker_run_id, result, path)
    if selected == "sqlite":
        return queue_sqlite.fail_item(task_id, worker_run_id, result, path)
    data = queue_store.fail_item(task_id, worker_run_id, result, path)
    data["backend"] = "repo-json"
    return data


def recover_expired_claims(backend: str | None = None, path: str | Path | None = None) -> dict[str, Any]:
    selected = normalize_backend(backend)
    if selected == "http":
        return queue_http.recover_expired_claims(path)
    if selected == "sqlite":
        recovered = queue_sqlite.recover_expired_claims(path)
        status = queue_sqlite.queue_status(path, include_items=False)
        return {
            "backend": "sqlite",
            "path": status.get("path"),
            "recovered_claims": recovered,
            "counts": status.get("counts", {}),
        }
    data = queue_store.load_queue(path)
    recovered = queue_store.recover_expired_claims(data, path)
    if recovered:
        queue_store.save_queue(data, path)
    status = queue_store.queue_status(path)
    return {
        "backend": "repo-json",
        "path": status.get("path"),
        "events_path": status.get("events_path"),
        "recovered_claims": recovered,
        "counts": status.get("counts", {}),
    }


def queue_status(backend: str | None = None, path: str | Path | None = None, include_items: bool = True) -> dict[str, Any]:
    selected = normalize_backend(backend)
    if selected == "http":
        return queue_http.queue_status(path, include_items=include_items)
    if selected == "sqlite":
        return queue_sqlite.queue_status(path, include_items=include_items)
    data = queue_store.queue_status(path)
    data["backend"] = "repo-json"
    if not include_items:
        data.pop("items", None)
    return data

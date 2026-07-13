#!/usr/bin/env python3
"""HTTP queue client for Codex Loop queue service."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


DEFAULT_URL_ENV = "CODEX_LOOP_QUEUE_URL"
DEFAULT_TOKEN_ENV = "CODEX_LOOP_QUEUE_TOKEN"


def base_url(path: str | Path | None = None) -> str:
    value = str(path or os.environ.get(DEFAULT_URL_ENV) or "").strip()
    if not value:
        raise ValueError(f"HTTP queue backend requires queue path URL or {DEFAULT_URL_ENV}.")
    if not value.startswith(("http://", "https://")):
        raise ValueError(f"HTTP queue backend path must be an http(s) URL, got `{value}`.")
    return value.rstrip("/")


def token() -> str | None:
    value = os.environ.get(DEFAULT_TOKEN_ENV)
    return value if value else None


def request_json(method: str, path: str, payload: dict[str, Any] | None = None, base: str | Path | None = None) -> dict[str, Any]:
    data = None
    headers = {"Accept": "application/json"}
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    bearer = token()
    if bearer:
        headers["Authorization"] = f"Bearer {bearer}"
    request = urllib.request.Request(base_url(base) + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            body = response.read().decode("utf-8")
            result = json.loads(body) if body else {}
            if isinstance(result, dict):
                result.setdefault("backend", "http")
                result.setdefault("service_url", base_url(base))
            return result
            return {"backend": "http", "service_url": base_url(base), "value": result}
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {"raw": body}
        if isinstance(parsed, dict):
            parsed.setdefault("backend", "http")
            parsed.setdefault("service_url", base_url(base))
            parsed.setdefault("http_status", exc.code)
            return parsed
        return {"backend": "http", "service_url": base_url(base), "http_status": exc.code, "value": parsed}
    except urllib.error.URLError as exc:
        return {
            "backend": "http",
            "service_url": base_url(base),
            "unreachable": True,
            "error": str(exc.reason),
        }


def enqueue_plan(plan: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    return request_json("POST", "/v1/queue/enqueue-plan", {"plan": plan}, path)


def claim_item(task_id: str, worker_run_id: str, lease_seconds: int = 3600, path: str | Path | None = None) -> dict[str, Any]:
    return request_json(
        "POST",
        "/v1/queue/claim",
        {"task_id": task_id, "worker_run_id": worker_run_id, "lease_seconds": lease_seconds},
        path,
    )


def complete_item(task_id: str, worker_run_id: str, result: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    return request_json(
        "POST",
        "/v1/queue/complete",
        {"task_id": task_id, "worker_run_id": worker_run_id, "result": result},
        path,
    )


def fail_item(task_id: str, worker_run_id: str, result: dict[str, Any], path: str | Path | None = None) -> dict[str, Any]:
    return request_json(
        "POST",
        "/v1/queue/fail",
        {"task_id": task_id, "worker_run_id": worker_run_id, "result": result},
        path,
    )


def recover_expired_claims(path: str | Path | None = None) -> dict[str, Any]:
    return request_json("POST", "/v1/queue/recover", {}, path)


def queue_status(path: str | Path | None = None, include_items: bool = True) -> dict[str, Any]:
    query = urllib.parse.urlencode({"include_items": "true" if include_items else "false"})
    return request_json("GET", f"/v1/queue/status?{query}", None, path)

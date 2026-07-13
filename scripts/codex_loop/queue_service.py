#!/usr/bin/env python3
"""HTTP service boundary for the Codex Loop persistent queue."""

from __future__ import annotations

import argparse
import json
import os
import sys
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

from lib import RUNS_ROOT, ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from queue_backend import (
    claim_item,
    complete_item,
    display_path,
    enqueue_plan,
    fail_item,
    normalize_backend,
    queue_status,
    recover_expired_claims,
)


STATE_PATH = RUNS_ROOT / ".loop" / "queue-service-state.json"
MAX_BODY_BYTES = 1024 * 1024


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def load_json_body(handler: BaseHTTPRequestHandler) -> dict[str, Any]:
    length = int(handler.headers.get("Content-Length") or 0)
    if length > MAX_BODY_BYTES:
        raise ValueError("request body too large")
    if length <= 0:
        return {}
    raw = handler.rfile.read(length)
    if not raw:
        return {}
    data = json.loads(raw.decode("utf-8"))
    if not isinstance(data, dict):
        raise ValueError("request body must be a JSON object")
    return data


def write_response(handler: BaseHTTPRequestHandler, status: int, payload: dict[str, Any]) -> None:
    body = json.dumps(payload, ensure_ascii=False, indent=2).encode("utf-8")
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json; charset=utf-8")
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


def auth_ok(handler: BaseHTTPRequestHandler) -> bool:
    token = getattr(handler.server, "auth_token", "") or ""
    if not token:
        return True
    return handler.headers.get("Authorization") == f"Bearer {token}"


def load_plan(payload: dict[str, Any]) -> dict[str, Any]:
    if isinstance(payload.get("plan"), dict):
        return payload["plan"]
    plan_path = payload.get("plan_path")
    if not plan_path:
        raise ValueError("plan or plan_path is required")
    return json.loads(repo_path(str(plan_path)).read_text(encoding="utf-8"))


def public_status(server: ThreadingHTTPServer, include_items: bool = False) -> dict[str, Any]:
    status = queue_status(
        backend=getattr(server, "queue_backend", "sqlite"),
        path=getattr(server, "queue_path", None),
        include_items=include_items,
    )
    return {
        "status": "QUEUE_SERVICE_HEALTHY",
        "generated_at": now_iso(),
        "backend": getattr(server, "queue_backend", "sqlite"),
        "queue_path": display_path(getattr(server, "queue_path", None)),
        "queue": status,
    }


class QueueServiceHandler(BaseHTTPRequestHandler):
    server_version = "CodexLoopQueueService/1.0"

    def log_message(self, fmt: str, *args: Any) -> None:
        if getattr(self.server, "quiet", False):
            return
        super().log_message(fmt, *args)

    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        query = parse_qs(parsed.query)
        include_items = str(query.get("include_items", ["false"])[0]).lower() in {"1", "true", "yes"}
        try:
            if parsed.path == "/health":
                write_response(self, 200, public_status(self.server, include_items=False))
                return
            if not auth_ok(self):
                write_response(self, 401, {"error": "unauthorized"})
                return
            if parsed.path == "/v1/queue/status":
                status = queue_status(
                    backend=getattr(self.server, "queue_backend", "sqlite"),
                    path=getattr(self.server, "queue_path", None),
                    include_items=include_items,
                )
                write_response(self, 200, status)
                return
            write_response(self, 404, {"error": "not found", "path": parsed.path})
        except Exception as exc:  # pragma: no cover - defensive boundary.
            write_response(self, 500, {"error": str(exc)})

    def do_POST(self) -> None:
        if not auth_ok(self):
            write_response(self, 401, {"error": "unauthorized"})
            return
        parsed = urlparse(self.path)
        try:
            payload = load_json_body(self)
            backend = getattr(self.server, "queue_backend", "sqlite")
            queue_path = getattr(self.server, "queue_path", None)
            if parsed.path == "/v1/queue/enqueue-plan":
                result = enqueue_plan(load_plan(payload), backend=backend, path=queue_path)
                write_response(self, 200, result)
                return
            if parsed.path == "/v1/queue/claim":
                result = claim_item(
                    str(payload.get("task_id")),
                    str(payload.get("worker_run_id")),
                    int(payload.get("lease_seconds") or 3600),
                    backend=backend,
                    path=queue_path,
                )
                write_response(self, 200 if result.get("claimed") else 409, result)
                return
            if parsed.path == "/v1/queue/complete":
                result = complete_item(
                    str(payload.get("task_id")),
                    str(payload.get("worker_run_id")),
                    payload.get("result") if isinstance(payload.get("result"), dict) else {"status": "queue_service_complete"},
                    backend=backend,
                    path=queue_path,
                )
                write_response(self, 200 if result.get("completed") else 409, result)
                return
            if parsed.path == "/v1/queue/fail":
                result = fail_item(
                    str(payload.get("task_id")),
                    str(payload.get("worker_run_id")),
                    payload.get("result") if isinstance(payload.get("result"), dict) else {"status": "queue_service_fail"},
                    backend=backend,
                    path=queue_path,
                )
                write_response(self, 200 if result.get("failed") else 409, result)
                return
            if parsed.path == "/v1/queue/recover":
                result = recover_expired_claims(backend=backend, path=queue_path)
                write_response(self, 200, result)
                return
            if parsed.path == "/v1/service/stop":
                write_response(self, 200, {"stopping": True})
                threading.Thread(target=self.server.shutdown, daemon=True).start()
                return
            write_response(self, 404, {"error": "not found", "path": parsed.path})
        except json.JSONDecodeError as exc:
            write_response(self, 400, {"error": f"invalid JSON: {exc}"})
        except Exception as exc:
            write_response(self, 400, {"error": str(exc)})


def service_findings(host: str, backend: str, auth_token: str | None) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    if backend != "sqlite":
        findings.append({"level": "warning", "code": "QUEUE_SERVICE_NON_SQLITE_BACKEND", "message": "HTTP queue service is production-oriented for sqlite; repo-json is audit-only."})
    if host not in {"127.0.0.1", "localhost", "::1"} and not auth_token:
        findings.append({"level": "blocker", "code": "QUEUE_SERVICE_TOKEN_REQUIRED", "message": "Non-loopback queue service binding requires an auth token."})
    return findings


def make_server(host: str, port: int, backend: str, queue_path: str | None, auth_token: str | None, quiet: bool) -> ThreadingHTTPServer:
    server = ThreadingHTTPServer((host, port), QueueServiceHandler)
    server.queue_backend = normalize_backend(backend)  # type: ignore[attr-defined]
    server.queue_path = queue_path  # type: ignore[attr-defined]
    server.auth_token = auth_token or ""  # type: ignore[attr-defined]
    server.quiet = quiet  # type: ignore[attr-defined]
    return server


def write_state(payload: dict[str, Any]) -> None:
    write_json(STATE_PATH, payload)


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Queue Service",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- backend: `{summary['backend']}`",
        f"- queue_path: `{summary.get('queue_path') or 'default'}`",
        f"- host: `{summary.get('host')}`",
        f"- port: `{summary.get('port')}`",
        "",
        "## Checks",
    ]
    for item in summary.get("checks", []):
        lines.append(f"- `{item['name']}` http `{item.get('http_status')}` ok `{item.get('ok')}`")
    lines.extend(["", "## Findings"])
    findings = summary.get("findings") or []
    if findings:
        for item in findings:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- The service exposes queue operations only; scheduler, worker, sandbox, reviewer, and evidence gates still decide task execution and closure.",
            "- Default binding is loopback with sqlite queue storage.",
            "",
        ]
    )
    return "\n".join(lines)


def serve(args: argparse.Namespace) -> int:
    backend = normalize_backend(args.queue_backend)
    token = os.environ.get(args.auth_token_env) if args.auth_token_env else args.auth_token
    findings = service_findings(args.host, backend, token)
    if any(item["level"] == "blocker" for item in findings):
        print(json.dumps({"status": "QUEUE_SERVICE_BLOCKED", "findings": findings}, ensure_ascii=False, indent=2))
        return 2
    server = make_server(args.host, args.port, backend, args.queue_path, token, args.quiet)
    host, port = server.server_address[:2]
    state = {
        "status": "QUEUE_SERVICE_RUNNING",
        "pid": os.getpid(),
        "host": host,
        "port": port,
        "backend": backend,
        "queue_path": display_path(args.queue_path),
        "auth_token_env": args.auth_token_env,
        "started_at": now_iso(),
        "findings": findings,
    }
    write_state(state)
    print(json.dumps({key: value for key, value in state.items() if key != "auth_token"}, ensure_ascii=False, indent=2))
    try:
        server.serve_forever(poll_interval=0.5)
    finally:
        state["status"] = "QUEUE_SERVICE_STOPPED"
        state["stopped_at"] = now_iso()
        write_state(state)
        server.server_close()
    return 0


def request_json(base_url: str, method: str, path: str, token: str | None, payload: dict[str, Any] | None = None) -> dict[str, Any]:
    data = None
    headers = {"Accept": "application/json"}
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(base_url + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            body = response.read().decode("utf-8")
            return {
                "http_status": response.status,
                "body": json.loads(body) if body else {},
                "ok": 200 <= response.status < 300,
            }
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        parsed: Any
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {"raw": body}
        return {"http_status": exc.code, "body": parsed, "ok": False}
    except Exception as exc:
        return {"http_status": None, "body": {"error": str(exc)}, "ok": False}


def smoke_plan(run_id: str, task_id: str = "QUEUE-SERVICE-SMOKE") -> dict[str, Any]:
    return {
        "run_id": f"{run_id}-seed",
        "queue": {
            "selected": [
                {
                    "id": task_id,
                    "title": "Queue service smoke task",
                    "priority": "P0",
                    "mode": "plan",
                    "data_mode": "synthetic",
                    "lane_primary": "infra",
                    "lane_dependent": [],
                    "subsystems": ["codex_loop"],
                    "acceptance_type": "smoke",
                    "risk_level": "low",
                    "resource_weight": 1,
                    "resource_group": "queue-service-smoke",
                    "score": 100,
                    "allowed_paths": ["scripts/codex_loop"],
                    "path": "scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml",
                    "attempt": 1,
                    "max_retries": 3,
                }
            ],
            "deferred": [],
        },
    }


def smoke(args: argparse.Namespace) -> int:
    run_id = args.run_id or make_run_id("queue-service-smoke")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "queue-service"
    out_dir.mkdir(parents=True, exist_ok=True)
    backend = normalize_backend(args.queue_backend)
    queue_path = args.queue_path or f"doc/02_acceptance/runs/{run_id}/queue-service/queue-service-smoke.sqlite3"
    token = args.auth_token or "queue-service-smoke-token"
    findings = service_findings(args.host, backend, token)
    server = make_server(args.host, 0, backend, queue_path, token, quiet=True)
    host, port = server.server_address[:2]
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
    thread.start()
    base_url = f"http://{host}:{port}"
    time.sleep(0.05)
    checks: list[dict[str, Any]] = []

    def check(
        name: str,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
        auth_token: str | None = token,
        expected_status: int | None = None,
    ) -> dict[str, Any]:
        result = request_json(base_url, method, path, auth_token, payload)
        if expected_status is not None:
            result["ok"] = result.get("http_status") == expected_status
        checks.append({"name": name, **result})
        return result

    try:
        check("health_no_auth", "GET", "/health", auth_token=None, expected_status=200)
        check("status_requires_auth", "GET", "/v1/queue/status", auth_token=None, expected_status=401)
        check("health", "GET", "/health")
        check("enqueue_plan", "POST", "/v1/queue/enqueue-plan", {"plan": smoke_plan(run_id)})
        check("status_with_items", "GET", "/v1/queue/status?include_items=true")
        check("claim", "POST", "/v1/queue/claim", {"task_id": "QUEUE-SERVICE-SMOKE", "worker_run_id": f"{run_id}-worker", "lease_seconds": 60})
        check("complete", "POST", "/v1/queue/complete", {"task_id": "QUEUE-SERVICE-SMOKE", "worker_run_id": f"{run_id}-worker", "result": {"status": "queue_service_smoke_complete"}})
        check("recover", "POST", "/v1/queue/recover", {})
        check("final_status", "GET", "/v1/queue/status")
        old_token = os.environ.get("CODEX_LOOP_QUEUE_TOKEN")
        os.environ["CODEX_LOOP_QUEUE_TOKEN"] = token
        try:
            http_task_id = "QUEUE-SERVICE-SMOKE-HTTP"
            backend_enqueue = enqueue_plan(smoke_plan(run_id, http_task_id), backend="http", path=base_url)
            backend_claim = claim_item(http_task_id, f"{run_id}-http-worker", 60, backend="http", path=base_url)
            backend_complete = complete_item(http_task_id, f"{run_id}-http-worker", {"status": "http_backend_complete"}, backend="http", path=base_url)
            backend_recover = recover_expired_claims(backend="http", path=base_url)
            checks.append({"name": "http_backend_enqueue_plan", **backend_enqueue, "ok": bool(backend_enqueue.get("counts")), "http_status": "backend"})
            checks.append({"name": "http_backend_claim", **backend_claim, "ok": bool(backend_claim.get("claimed")), "http_status": "backend"})
            checks.append({"name": "http_backend_complete", **backend_complete, "ok": bool(backend_complete.get("completed")), "http_status": "backend"})
            checks.append({"name": "http_backend_recover", **backend_recover, "ok": "recovered_claims" in backend_recover, "http_status": "backend"})
        finally:
            if old_token is None:
                os.environ.pop("CODEX_LOOP_QUEUE_TOKEN", None)
            else:
                os.environ["CODEX_LOOP_QUEUE_TOKEN"] = old_token
        check("stop", "POST", "/v1/service/stop", {})
    finally:
        thread.join(timeout=5)
        if thread.is_alive():
            server.shutdown()
            thread.join(timeout=5)
        server.server_close()

    if not all(item.get("ok") for item in checks):
        findings.append({"level": "blocker", "code": "QUEUE_SERVICE_SMOKE_FAILED", "message": "One or more queue service HTTP checks failed."})
    final_queue = queue_status(backend=backend, path=queue_path, include_items=False)
    if final_queue.get("counts", {}).get("done") != 2:
        findings.append({"level": "blocker", "code": "QUEUE_SERVICE_FINAL_STATE_UNEXPECTED", "message": f"Expected two done smoke items, got {final_queue.get('counts')}."})
    status = "QUEUE_SERVICE_SMOKE_PASSED" if not any(item["level"] == "blocker" for item in findings) else "QUEUE_SERVICE_SMOKE_FAILED"
    summary = {
        "run_id": run_id,
        "run_kind": "queue_service",
        "status": status,
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "host": host,
        "port": port,
        "backend": backend,
        "queue_path": display_path(queue_path),
        "auth_required": True,
        "checks": [{key: value for key, value in item.items() if key != "body"} for item in checks],
        "final_queue": final_queue,
        "findings": findings,
        "outputs": [
            "queue-service/queue-service-summary.json",
            "queue-service/queue-service-report.md",
            "queue-service/smoke-responses.json",
        ],
    }
    write_json(out_dir / "queue-service-summary.json", summary)
    write_text(out_dir / "queue-service-report.md", render_report(summary))
    write_json(out_dir / "smoke-responses.json", {"checks": checks})
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} checks={len(checks)} findings={len(findings)}")
    return 0 if status == "QUEUE_SERVICE_SMOKE_PASSED" else 2


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    serve_parser = sub.add_parser("serve")
    serve_parser.add_argument("--host", default="127.0.0.1")
    serve_parser.add_argument("--port", type=int, default=8765)
    serve_parser.add_argument("--queue-backend", choices=["repo-json", "sqlite"], default="sqlite")
    serve_parser.add_argument("--queue-path", default=None)
    serve_parser.add_argument("--auth-token", default=None)
    serve_parser.add_argument("--auth-token-env", default="CODEX_LOOP_QUEUE_TOKEN")
    serve_parser.add_argument("--quiet", action="store_true")

    smoke_parser = sub.add_parser("smoke")
    smoke_parser.add_argument("--run-id", default=None)
    smoke_parser.add_argument("--host", default="127.0.0.1")
    smoke_parser.add_argument("--queue-backend", choices=["repo-json", "sqlite"], default="sqlite")
    smoke_parser.add_argument("--queue-path", default=None)
    smoke_parser.add_argument("--auth-token", default=None)

    args = parser.parse_args()
    if args.command == "serve":
        return serve(args)
    if args.command == "smoke":
        return smoke(args)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())

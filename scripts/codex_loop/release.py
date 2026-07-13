#!/usr/bin/env python3
"""Freeze Codex Loop release evidence and rollback instructions."""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from metrics import build_metrics
from queue_backend import display_path, queue_status
from service import build_health


def git_output(args: list[str]) -> str:
    return run_git(args)


def load_json(path: str | None) -> dict[str, Any]:
    if not path:
        return {}
    target = repo_path(path)
    if not target.exists():
        return {"missing": True, "path": rel_path(target)}
    try:
        return json.loads(target.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return {"parse_error": str(exc), "path": rel_path(target)}


def safe_build_health(queue_backend: str, queue_path: str | None, profile: str, skip_preflight: bool) -> dict[str, Any]:
    try:
        return build_health(
            expect_running=False,
            queue_backend=queue_backend,
            queue_path=queue_path,
            profile=profile,
            skip_preflight=skip_preflight,
        )
    except Exception as exc:  # noqa: BLE001 - release must write blocker evidence instead of crashing.
        return {
            "status": "UNHEALTHY",
            "error": type(exc).__name__,
            "message": str(exc),
        }


def safe_queue_status(queue_backend: str, queue_path: str | None) -> dict[str, Any]:
    try:
        return queue_status(backend=queue_backend, path=queue_path, include_items=False)
    except Exception as exc:  # noqa: BLE001 - queue failures are release blockers.
        return {
            "status": "QUEUE_STATUS_FAILED",
            "path": display_path(queue_path),
            "counts": {},
            "error": type(exc).__name__,
            "message": str(exc),
        }


def render_rollback(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Rollback Plan",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- commit: `{summary['commit']}`",
        f"- status: `{summary['status']}`",
        "",
        "## Immediate Stop",
        "",
        "```bash",
        "python -B scripts/codex_loop/service.py stop",
        "python -B scripts/codex_loop/lock_manager.py release --force",
        "python -B scripts/codex_loop/service.py health --run-id rollback-health",
        "```",
        "",
        "## Revert Scope",
        "",
        "The release manifest records the current diff and untracked loop evidence. Revert only loop-engine files after review:",
        "",
        "```bash",
        "git status --short -- scripts/codex_loop doc/01_design/自动开发Loop引擎设计.md doc/README.md doc/02_acceptance/runs/.loop",
        "```",
        "",
        "## Guardrail",
        "- Do not run destructive git reset or checkout without explicit human approval.",
        "- Do not delete acceptance evidence until a replacement manifest exists.",
        "",
    ]
    return "\n".join(lines)


def render_manifest(summary: dict[str, Any]) -> str:
    health = summary.get("health") or {}
    queue = summary.get("queue") or {}
    lines = [
        "# Codex Loop Release Manifest",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- commit: `{summary['commit']}`",
        f"- health: `{health.get('status')}`",
        f"- queue_counts: `{queue.get('counts')}`",
        f"- deploy_plan: `{summary.get('deploy_plan')}`",
        f"- deploy_status: `{(summary.get('deploy') or {}).get('status', 'none')}`",
        f"- deploy_image_layout: `{(summary.get('deploy') or {}).get('image_layout', 'none')}`",
        f"- deploy_k8s_queue_path: `{(summary.get('deploy') or {}).get('k8s_queue_path', 'none')}`",
        f"- image_build: `{summary.get('image_build_path') or 'none'}`",
        f"- image_build_status: `{(summary.get('image_build') or {}).get('status', 'none')}`",
        f"- image_build_layout: `{(summary.get('image_build') or {}).get('image_layout', 'none')}`",
        f"- image_distribution: `{summary.get('image_distribution_path') or 'none'}`",
        f"- image_distribution_status: `{(summary.get('image_distribution') or {}).get('status', 'none')}`",
        f"- k8s_bootstrap: `{summary.get('k8s_bootstrap_path') or 'none'}`",
        f"- k8s_bootstrap_status: `{(summary.get('k8s_bootstrap') or {}).get('status', 'none')}`",
        f"- sandbox_plan: `{summary.get('sandbox_plan') or 'none'}`",
        f"- sandbox_status: `{(summary.get('sandbox') or {}).get('status', 'none')}`",
        f"- sandbox_execution: `{summary.get('sandbox_execution_path') or 'none'}`",
        f"- sandbox_execution_status: `{(summary.get('sandbox_execution') or {}).get('status', 'none')}`",
        f"- sandbox_worker: `{summary.get('sandbox_worker_path') or 'none'}`",
        f"- sandbox_worker_status: `{(summary.get('sandbox_worker') or {}).get('status', 'none')}`",
        f"- resource_quota: `{summary.get('resource_quota_path') or 'none'}`",
        f"- resource_quota_status: `{(summary.get('resource_quota') or {}).get('status', 'none')}`",
        f"- resource_monitor: `{summary.get('resource_monitor_path') or 'none'}`",
        f"- resource_monitor_status: `{(summary.get('resource_monitor') or {}).get('status', 'none')}`",
        f"- workspace_isolation: `{summary.get('workspace_isolation_path') or 'none'}`",
        f"- workspace_isolation_status: `{(summary.get('workspace_isolation') or {}).get('status', 'none')}`",
        f"- workspace_cleanup: `{summary.get('workspace_cleanup_path') or 'none'}`",
        f"- workspace_cleanup_status: `{(summary.get('workspace_cleanup') or {}).get('status', 'none')}`",
        f"- executor_pool: `{summary.get('executor_pool_path') or 'none'}`",
        f"- executor_pool_status: `{(summary.get('executor_pool') or {}).get('status', 'none')}`",
        f"- executor_pool_activate_workspaces: `{(summary.get('executor_pool') or {}).get('activate_workspaces', 'none')}`",
        f"- executor_pool_stress: `{summary.get('executor_pool_stress_path') or 'none'}`",
        f"- executor_pool_stress_status: `{(summary.get('executor_pool_stress') or {}).get('status', 'none')}`",
        f"- executor_pool_stress_workspace_backend: `{(summary.get('executor_pool_stress') or {}).get('workspace_backend', 'none')}`",
        f"- remote_pool_stress: `{summary.get('remote_pool_stress_path') or 'none'}`",
        f"- remote_pool_stress_status: `{(summary.get('remote_pool_stress') or {}).get('status', 'none')}`",
        f"- remote_pool_stress_mode: `{(summary.get('remote_pool_stress') or {}).get('service_mode', 'none')}`",
        f"- remote_pool_stress_workers: `{(summary.get('remote_pool_stress') or {}).get('workers', 'none')}`",
        f"- remote_pool_k8s_stress: `{summary.get('remote_pool_k8s_stress_path') or 'none'}`",
        f"- remote_pool_k8s_stress_status: `{(summary.get('remote_pool_k8s_stress') or {}).get('status', 'none')}`",
        f"- remote_pool_k8s_stress_image_layout: `{(summary.get('remote_pool_k8s_stress') or {}).get('image_layout', 'none')}`",
        f"- remote_pool_k8s_stress_workers: `{(summary.get('remote_pool_k8s_stress') or {}).get('workers', 'none')}`",
        f"- remote_pool_k8s_readiness: `{summary.get('remote_pool_k8s_readiness_path') or 'none'}`",
        f"- remote_pool_k8s_readiness_status: `{(summary.get('remote_pool_k8s_readiness') or {}).get('status', 'none')}`",
        f"- soak: `{summary.get('soak_path') or 'none'}`",
        f"- soak_status: `{(summary.get('soak') or {}).get('status', 'none')}`",
        f"- soak_cycles_completed: `{(summary.get('soak') or {}).get('cycles_completed', 'none')}`",
        f"- model_profile: `{summary.get('model_profile_path') or 'none'}`",
        f"- model_profile_status: `{(summary.get('model_profile') or {}).get('status', 'none')}`",
        f"- model_profile_selected: `{(summary.get('model_profile') or {}).get('selected_profile', 'none')}`",
        f"- llm_review: `{summary.get('llm_review_path') or 'none'}`",
        f"- llm_review_status: `{(summary.get('llm_review') or {}).get('status', 'none')}`",
        f"- llm_review_decision: `{(summary.get('llm_review') or {}).get('decision', 'none')}`",
        f"- queue_service: `{summary.get('queue_service_path') or 'none'}`",
        f"- queue_service_status: `{(summary.get('queue_service') or {}).get('status', 'none')}`",
        f"- objective_stop: `{summary.get('objective_stop_path') or 'none'}`",
        f"- objective_stop_status: `{(summary.get('objective_stop') or {}).get('status', 'none')}`",
        "",
        "## Evidence",
    ]
    for item in summary.get("outputs", []):
        lines.append(f"- `{item}`")
    lines.extend(["", "## Guardrail", "- This manifest freezes loop-engine evidence only; it is not a business acceptance pass.", ""])
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--deploy-plan", default=None)
    parser.add_argument("--image-build", default=None)
    parser.add_argument("--image-distribution", default=None)
    parser.add_argument("--k8s-bootstrap", default=None)
    parser.add_argument("--sandbox-plan", default=None)
    parser.add_argument("--sandbox-execution", default=None)
    parser.add_argument("--sandbox-worker", default=None)
    parser.add_argument("--resource-quota", default=None)
    parser.add_argument("--resource-monitor", default=None)
    parser.add_argument("--workspace-isolation", default=None)
    parser.add_argument("--workspace-cleanup", default=None)
    parser.add_argument("--executor-pool", default=None)
    parser.add_argument("--executor-pool-stress", default=None)
    parser.add_argument("--remote-pool-stress", default=None)
    parser.add_argument("--remote-pool-k8s-stress", default=None)
    parser.add_argument("--remote-pool-k8s-readiness", default=None)
    parser.add_argument("--soak", default=None)
    parser.add_argument("--model-profile", default=None)
    parser.add_argument("--llm-review", default=None)
    parser.add_argument("--queue-service", default=None)
    parser.add_argument("--objective-stop", default=None)
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default="repo-json")
    parser.add_argument("--queue-path", default=None)
    parser.add_argument("--profile", default="conservative")
    parser.add_argument("--skip-preflight", action="store_true")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("loop-release")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "release"
    out_dir.mkdir(parents=True, exist_ok=True)
    health = safe_build_health(args.queue_backend, args.queue_path, args.profile, args.skip_preflight)
    deploy_plan = load_json(args.deploy_plan)
    image_build = load_json(args.image_build)
    image_distribution = load_json(args.image_distribution)
    k8s_bootstrap = load_json(args.k8s_bootstrap)
    sandbox = load_json(args.sandbox_plan)
    sandbox_execution = load_json(args.sandbox_execution)
    sandbox_worker = load_json(args.sandbox_worker)
    resource_quota = load_json(args.resource_quota)
    resource_monitor = load_json(args.resource_monitor)
    workspace_isolation = load_json(args.workspace_isolation)
    workspace_cleanup = load_json(args.workspace_cleanup)
    executor_pool = load_json(args.executor_pool)
    executor_pool_stress = load_json(args.executor_pool_stress)
    remote_pool_stress = load_json(args.remote_pool_stress)
    remote_pool_k8s_stress = load_json(args.remote_pool_k8s_stress)
    remote_pool_k8s_readiness = load_json(args.remote_pool_k8s_readiness)
    soak = load_json(args.soak)
    model_profile = load_json(args.model_profile)
    llm_review = load_json(args.llm_review)
    queue_service = load_json(args.queue_service)
    objective_stop = load_json(args.objective_stop)
    queue = safe_queue_status(args.queue_backend, args.queue_path)
    metrics = build_metrics(limit=20)
    tracked_diff = git_output(["diff", "--", "scripts/codex_loop", "doc/01_design/自动开发Loop引擎设计.md", "doc/README.md"])
    status_text = git_output(["status", "--short", "--", "scripts/codex_loop", "doc/01_design/自动开发Loop引擎设计.md", "doc/README.md", "doc/02_acceptance/runs/.loop"])
    commit = git_output(["rev-parse", "HEAD"]).strip()
    findings: list[dict[str, str]] = []
    if health.get("status") != "HEALTHY":
        findings.append({"level": "blocker", "code": "SERVICE_HEALTH_NOT_GREEN", "message": f"Health is {health.get('status')}."})
    if health.get("error"):
        findings.append({"level": "blocker", "code": "SERVICE_HEALTH_CHECK_FAILED", "message": f"{health.get('error')}: {health.get('message')}"})
    if queue.get("error"):
        findings.append({"level": "blocker", "code": "QUEUE_STATUS_FAILED", "message": f"{queue.get('error')}: {queue.get('message')}"})
    if queue.get("counts", {}).get("claimed"):
        findings.append({"level": "blocker", "code": "QUEUE_HAS_CLAIMED_ITEMS", "message": "Queue has in-flight claimed items."})
    if args.deploy_plan:
        if deploy_plan.get("missing"):
            findings.append({"level": "blocker", "code": "DEPLOY_PLAN_MISSING", "message": f"Deploy plan evidence is missing: {args.deploy_plan}."})
        elif deploy_plan.get("parse_error"):
            findings.append({"level": "blocker", "code": "DEPLOY_PLAN_PARSE_FAILED", "message": f"Deploy plan evidence could not be parsed: {deploy_plan.get('parse_error')}."})
        elif deploy_plan.get("status") != "DEPLOY_PLAN_READY":
            findings.append({"level": "blocker", "code": "DEPLOY_PLAN_NOT_READY", "message": f"Deploy plan status is {deploy_plan.get('status')}."})
    if args.image_build:
        if image_build.get("missing"):
            findings.append({"level": "blocker", "code": "IMAGE_BUILD_MISSING", "message": f"Image build evidence is missing: {args.image_build}."})
        elif image_build.get("parse_error"):
            findings.append({"level": "blocker", "code": "IMAGE_BUILD_PARSE_FAILED", "message": f"Image build evidence could not be parsed: {image_build.get('parse_error')}."})
        elif image_build.get("status") != "IMAGE_BUILD_COMPLETED":
            findings.append({"level": "blocker", "code": "IMAGE_BUILD_NOT_COMPLETED", "message": f"Image build status is {image_build.get('status')}."})
    if args.image_distribution:
        if image_distribution.get("missing"):
            findings.append({"level": "blocker", "code": "IMAGE_DISTRIBUTION_MISSING", "message": f"Image distribution evidence is missing: {args.image_distribution}."})
        elif image_distribution.get("parse_error"):
            findings.append({"level": "blocker", "code": "IMAGE_DISTRIBUTION_PARSE_FAILED", "message": f"Image distribution evidence could not be parsed: {image_distribution.get('parse_error')}."})
        elif image_distribution.get("status") != "IMAGE_DISTRIBUTION_READY":
            findings.append({"level": "blocker", "code": "IMAGE_DISTRIBUTION_NOT_READY", "message": f"Image distribution status is {image_distribution.get('status')}."})
    if args.k8s_bootstrap:
        if k8s_bootstrap.get("missing"):
            findings.append({"level": "blocker", "code": "K8S_BOOTSTRAP_MISSING", "message": f"K8s bootstrap evidence is missing: {args.k8s_bootstrap}."})
        elif k8s_bootstrap.get("parse_error"):
            findings.append({"level": "blocker", "code": "K8S_BOOTSTRAP_PARSE_FAILED", "message": f"K8s bootstrap evidence could not be parsed: {k8s_bootstrap.get('parse_error')}."})
        elif k8s_bootstrap.get("status") == "K8S_BOOTSTRAP_BLOCKED":
            findings.append({"level": "blocker", "code": "K8S_BOOTSTRAP_BLOCKED", "message": "K8s bootstrap evidence has blocker findings."})
        elif k8s_bootstrap.get("status") not in {"K8S_BOOTSTRAP_VALIDATED", "K8S_BOOTSTRAP_APPLIED"}:
            findings.append({"level": "blocker", "code": "K8S_BOOTSTRAP_NOT_READY", "message": f"K8s bootstrap status is {k8s_bootstrap.get('status')}."})
    if args.sandbox_plan:
        if sandbox.get("missing"):
            findings.append({"level": "blocker", "code": "SANDBOX_PLAN_MISSING", "message": f"Sandbox plan is missing: {args.sandbox_plan}."})
        elif sandbox.get("parse_error"):
            findings.append({"level": "blocker", "code": "SANDBOX_PLAN_PARSE_FAILED", "message": f"Sandbox plan could not be parsed: {sandbox.get('parse_error')}."})
        elif sandbox.get("status") != "SANDBOX_PLAN_READY":
            findings.append({"level": "blocker", "code": "SANDBOX_PLAN_NOT_READY", "message": f"Sandbox plan status is {sandbox.get('status')}."})
    if args.sandbox_execution:
        if sandbox_execution.get("missing"):
            findings.append({"level": "blocker", "code": "SANDBOX_EXECUTION_MISSING", "message": f"Sandbox execution is missing: {args.sandbox_execution}."})
        elif sandbox_execution.get("parse_error"):
            findings.append({"level": "blocker", "code": "SANDBOX_EXECUTION_PARSE_FAILED", "message": f"Sandbox execution could not be parsed: {sandbox_execution.get('parse_error')}."})
        elif sandbox_execution.get("status") != "SANDBOX_EXECUTION_COMPLETED":
            findings.append({"level": "blocker", "code": "SANDBOX_EXECUTION_NOT_COMPLETED", "message": f"Sandbox execution status is {sandbox_execution.get('status')}."})
    if args.sandbox_worker:
        if sandbox_worker.get("missing"):
            findings.append({"level": "blocker", "code": "SANDBOX_WORKER_MISSING", "message": f"Sandbox worker evidence is missing: {args.sandbox_worker}."})
        elif sandbox_worker.get("parse_error"):
            findings.append({"level": "blocker", "code": "SANDBOX_WORKER_PARSE_FAILED", "message": f"Sandbox worker evidence could not be parsed: {sandbox_worker.get('parse_error')}."})
        elif sandbox_worker.get("status") not in {"SANDBOX_WORKER_PLANNED", "SANDBOX_WORKER_COMPLETED"}:
            findings.append({"level": "blocker", "code": "SANDBOX_WORKER_NOT_READY", "message": f"Sandbox worker status is {sandbox_worker.get('status')}."})
    if args.resource_quota:
        if resource_quota.get("missing"):
            findings.append({"level": "blocker", "code": "RESOURCE_QUOTA_MISSING", "message": f"Resource quota evidence is missing: {args.resource_quota}."})
        elif resource_quota.get("parse_error"):
            findings.append({"level": "blocker", "code": "RESOURCE_QUOTA_PARSE_FAILED", "message": f"Resource quota evidence could not be parsed: {resource_quota.get('parse_error')}."})
        elif resource_quota.get("status") != "RESOURCE_QUOTA_READY":
            findings.append({"level": "blocker", "code": "RESOURCE_QUOTA_NOT_READY", "message": f"Resource quota status is {resource_quota.get('status')}."})
    if args.resource_monitor:
        if resource_monitor.get("missing"):
            findings.append({"level": "blocker", "code": "RESOURCE_MONITOR_MISSING", "message": f"Resource monitor evidence is missing: {args.resource_monitor}."})
        elif resource_monitor.get("parse_error"):
            findings.append({"level": "blocker", "code": "RESOURCE_MONITOR_PARSE_FAILED", "message": f"Resource monitor evidence could not be parsed: {resource_monitor.get('parse_error')}."})
        elif resource_monitor.get("status") == "RESOURCE_MONITOR_BLOCKED":
            findings.append({"level": "blocker", "code": "RESOURCE_MONITOR_BLOCKED", "message": "Resource monitor evidence has blocker findings."})
        elif resource_monitor.get("status") not in {"RESOURCE_MONITOR_READY", "RESOURCE_MONITOR_DEGRADED"}:
            findings.append({"level": "blocker", "code": "RESOURCE_MONITOR_NOT_READY", "message": f"Resource monitor status is {resource_monitor.get('status')}."})
    if args.workspace_isolation:
        if workspace_isolation.get("missing"):
            findings.append({"level": "blocker", "code": "WORKSPACE_ISOLATION_MISSING", "message": f"Workspace isolation evidence is missing: {args.workspace_isolation}."})
        elif workspace_isolation.get("parse_error"):
            findings.append({"level": "blocker", "code": "WORKSPACE_ISOLATION_PARSE_FAILED", "message": f"Workspace isolation evidence could not be parsed: {workspace_isolation.get('parse_error')}."})
        elif workspace_isolation.get("status") == "WORKSPACE_ISOLATION_BLOCKED":
            findings.append({"level": "blocker", "code": "WORKSPACE_ISOLATION_BLOCKED", "message": "Workspace isolation evidence has blocker findings."})
        elif workspace_isolation.get("status") not in {"WORKSPACE_ISOLATION_PLANNED", "WORKSPACE_ISOLATION_READY", "WORKSPACE_ISOLATION_DEGRADED"}:
            findings.append({"level": "blocker", "code": "WORKSPACE_ISOLATION_NOT_READY", "message": f"Workspace isolation status is {workspace_isolation.get('status')}."})
    if args.workspace_cleanup:
        if workspace_cleanup.get("missing"):
            findings.append({"level": "blocker", "code": "WORKSPACE_CLEANUP_MISSING", "message": f"Workspace cleanup evidence is missing: {args.workspace_cleanup}."})
        elif workspace_cleanup.get("parse_error"):
            findings.append({"level": "blocker", "code": "WORKSPACE_CLEANUP_PARSE_FAILED", "message": f"Workspace cleanup evidence could not be parsed: {workspace_cleanup.get('parse_error')}."})
        elif workspace_cleanup.get("status") == "WORKSPACE_CLEANUP_BLOCKED":
            findings.append({"level": "blocker", "code": "WORKSPACE_CLEANUP_BLOCKED", "message": "Workspace cleanup evidence has blocker findings."})
        elif workspace_cleanup.get("status") not in {"WORKSPACE_CLEANUP_PLANNED", "WORKSPACE_CLEANUP_COMPLETED", "WORKSPACE_CLEANUP_EMPTY"}:
            findings.append({"level": "blocker", "code": "WORKSPACE_CLEANUP_NOT_READY", "message": f"Workspace cleanup status is {workspace_cleanup.get('status')}."})
    if args.executor_pool:
        if executor_pool.get("missing"):
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_MISSING", "message": f"Executor pool evidence is missing: {args.executor_pool}."})
        elif executor_pool.get("parse_error"):
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_PARSE_FAILED", "message": f"Executor pool evidence could not be parsed: {executor_pool.get('parse_error')}."})
        elif executor_pool.get("status") not in {"EXECUTOR_POOL_PLANNED", "EXECUTOR_POOL_COMPLETED"}:
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_NOT_READY", "message": f"Executor pool status is {executor_pool.get('status')}."})
    if args.executor_pool_stress:
        if executor_pool_stress.get("missing"):
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_STRESS_MISSING", "message": f"Executor pool stress evidence is missing: {args.executor_pool_stress}."})
        elif executor_pool_stress.get("parse_error"):
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_STRESS_PARSE_FAILED", "message": f"Executor pool stress evidence could not be parsed: {executor_pool_stress.get('parse_error')}."})
        elif executor_pool_stress.get("status") == "EXECUTOR_POOL_STRESS_BLOCKED":
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_STRESS_BLOCKED", "message": "Executor pool stress evidence has blocker findings."})
        elif executor_pool_stress.get("status") not in {"EXECUTOR_POOL_STRESS_COMPLETED", "EXECUTOR_POOL_STRESS_EMPTY"}:
            findings.append({"level": "blocker", "code": "EXECUTOR_POOL_STRESS_NOT_READY", "message": f"Executor pool stress status is {executor_pool_stress.get('status')}."})
    if args.remote_pool_stress:
        if remote_pool_stress.get("missing"):
            findings.append({"level": "blocker", "code": "REMOTE_POOL_STRESS_MISSING", "message": f"Remote pool stress evidence is missing: {args.remote_pool_stress}."})
        elif remote_pool_stress.get("parse_error"):
            findings.append({"level": "blocker", "code": "REMOTE_POOL_STRESS_PARSE_FAILED", "message": f"Remote pool stress evidence could not be parsed: {remote_pool_stress.get('parse_error')}."})
        elif remote_pool_stress.get("status") == "REMOTE_POOL_STRESS_BLOCKED":
            findings.append({"level": "blocker", "code": "REMOTE_POOL_STRESS_BLOCKED", "message": "Remote pool stress evidence has blocker findings."})
        elif remote_pool_stress.get("status") not in {"REMOTE_POOL_STRESS_COMPLETED", "REMOTE_POOL_STRESS_EMPTY"}:
            findings.append({"level": "blocker", "code": "REMOTE_POOL_STRESS_NOT_READY", "message": f"Remote pool stress status is {remote_pool_stress.get('status')}."})
    if args.remote_pool_k8s_stress:
        if remote_pool_k8s_stress.get("missing"):
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_STRESS_MISSING", "message": f"Remote pool K8s stress evidence is missing: {args.remote_pool_k8s_stress}."})
        elif remote_pool_k8s_stress.get("parse_error"):
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_STRESS_PARSE_FAILED", "message": f"Remote pool K8s stress evidence could not be parsed: {remote_pool_k8s_stress.get('parse_error')}."})
        elif remote_pool_k8s_stress.get("status") == "REMOTE_POOL_K8S_STRESS_BLOCKED":
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_STRESS_BLOCKED", "message": "Remote pool K8s stress evidence has blocker findings."})
        elif remote_pool_k8s_stress.get("status") not in {"REMOTE_POOL_K8S_STRESS_PLANNED", "REMOTE_POOL_K8S_STRESS_VALIDATED", "REMOTE_POOL_K8S_STRESS_COMPLETED"}:
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_STRESS_NOT_READY", "message": f"Remote pool K8s stress status is {remote_pool_k8s_stress.get('status')}."})
    if args.remote_pool_k8s_readiness:
        if remote_pool_k8s_readiness.get("missing"):
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_READINESS_MISSING", "message": f"Remote pool K8s readiness evidence is missing: {args.remote_pool_k8s_readiness}."})
        elif remote_pool_k8s_readiness.get("parse_error"):
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_READINESS_PARSE_FAILED", "message": f"Remote pool K8s readiness evidence could not be parsed: {remote_pool_k8s_readiness.get('parse_error')}."})
        elif remote_pool_k8s_readiness.get("status") == "REMOTE_POOL_K8S_READINESS_BLOCKED":
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_READINESS_BLOCKED", "message": "Remote pool K8s readiness evidence has blocker findings."})
        elif remote_pool_k8s_readiness.get("status") not in {"REMOTE_POOL_K8S_READINESS_READY", "REMOTE_POOL_K8S_READINESS_DEGRADED"}:
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_READINESS_NOT_READY", "message": f"Remote pool K8s readiness status is {remote_pool_k8s_readiness.get('status')}."})
    if args.soak:
        if soak.get("missing"):
            findings.append({"level": "blocker", "code": "SOAK_MISSING", "message": f"Soak evidence is missing: {args.soak}."})
        elif soak.get("parse_error"):
            findings.append({"level": "blocker", "code": "SOAK_PARSE_FAILED", "message": f"Soak evidence could not be parsed: {soak.get('parse_error')}."})
        elif soak.get("status") == "SOAK_BLOCKED":
            findings.append({"level": "blocker", "code": "SOAK_BLOCKED", "message": "Soak evidence has blocker findings."})
        elif soak.get("status") not in {"SOAK_COMPLETED", "SOAK_DEGRADED", "SOAK_EMPTY"}:
            findings.append({"level": "blocker", "code": "SOAK_NOT_READY", "message": f"Soak status is {soak.get('status')}."})
    if args.model_profile:
        if model_profile.get("missing"):
            findings.append({"level": "blocker", "code": "MODEL_PROFILE_MISSING", "message": f"Model profile evidence is missing: {args.model_profile}."})
        elif model_profile.get("parse_error"):
            findings.append({"level": "blocker", "code": "MODEL_PROFILE_PARSE_FAILED", "message": f"Model profile evidence could not be parsed: {model_profile.get('parse_error')}."})
        elif model_profile.get("status") != "MODEL_PROFILE_SELECTED":
            findings.append({"level": "blocker", "code": "MODEL_PROFILE_NOT_SELECTED", "message": f"Model profile status is {model_profile.get('status')}."})
        elif any(item.get("level") == "blocker" for item in model_profile.get("findings", [])):
            findings.append({"level": "blocker", "code": "MODEL_PROFILE_HAS_BLOCKERS", "message": "Model profile evidence contains blocker findings."})
    if args.llm_review:
        if llm_review.get("missing"):
            findings.append({"level": "blocker", "code": "LLM_REVIEW_MISSING", "message": f"LLM review evidence is missing: {args.llm_review}."})
        elif llm_review.get("parse_error"):
            findings.append({"level": "blocker", "code": "LLM_REVIEW_PARSE_FAILED", "message": f"LLM review evidence could not be parsed: {llm_review.get('parse_error')}."})
        elif llm_review.get("status") == "LLM_REVIEW_BLOCKED":
            findings.append({"level": "blocker", "code": "LLM_REVIEW_BLOCKED", "message": "LLM reviewer evidence has blocker findings."})
        elif llm_review.get("status") not in {"LLM_REVIEW_PLANNED", "LLM_REVIEW_PASSED"}:
            findings.append({"level": "blocker", "code": "LLM_REVIEW_NOT_PASSED", "message": f"LLM reviewer status is {llm_review.get('status')}."})
        elif any(item.get("level") == "blocker" for item in llm_review.get("findings", [])):
            findings.append({"level": "blocker", "code": "LLM_REVIEW_HAS_BLOCKERS", "message": "LLM reviewer evidence contains blocker findings."})
    if args.queue_service:
        if queue_service.get("missing"):
            findings.append({"level": "blocker", "code": "QUEUE_SERVICE_MISSING", "message": f"Queue service evidence is missing: {args.queue_service}."})
        elif queue_service.get("parse_error"):
            findings.append({"level": "blocker", "code": "QUEUE_SERVICE_PARSE_FAILED", "message": f"Queue service evidence could not be parsed: {queue_service.get('parse_error')}."})
        elif queue_service.get("status") not in {"QUEUE_SERVICE_READY", "QUEUE_SERVICE_SMOKE_PASSED"}:
            findings.append({"level": "blocker", "code": "QUEUE_SERVICE_NOT_READY", "message": f"Queue service status is {queue_service.get('status')}."})
    if args.objective_stop:
        if objective_stop.get("missing"):
            findings.append({"level": "blocker", "code": "OBJECTIVE_STOP_MISSING", "message": f"Objective stop evidence is missing: {args.objective_stop}."})
        elif objective_stop.get("parse_error"):
            findings.append({"level": "blocker", "code": "OBJECTIVE_STOP_PARSE_FAILED", "message": f"Objective stop evidence could not be parsed: {objective_stop.get('parse_error')}."})
        elif objective_stop.get("status") != "OBJECTIVE_STOP_READY":
            findings.append({"level": "blocker", "code": "OBJECTIVE_STOP_NOT_READY", "message": f"Objective stop status is {objective_stop.get('status')}."})
    status = "RELEASE_FROZEN" if not findings else "RELEASE_BLOCKED"
    summary = {
        "run_id": run_id,
        "run_kind": "release_freeze",
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": commit,
        "deploy_plan": args.deploy_plan,
        "image_build_path": rel_path(repo_path(args.image_build)) if args.image_build else None,
        "image_distribution_path": rel_path(repo_path(args.image_distribution)) if args.image_distribution else None,
        "deploy": {
            "status": deploy_plan.get("status"),
            "target": deploy_plan.get("target"),
            "profile_name": deploy_plan.get("profile_name"),
            "executor_entry": ((deploy_plan.get("variables") or {}).get("executor_entry")),
            "image": ((deploy_plan.get("variables") or {}).get("image")),
            "image_layout": ((deploy_plan.get("variables") or {}).get("image_layout")),
            "app_root": ((deploy_plan.get("variables") or {}).get("app_root")),
            "state_root": ((deploy_plan.get("variables") or {}).get("state_root")),
            "k8s_queue_path": ((deploy_plan.get("variables") or {}).get("k8s_queue_path")),
            "findings": deploy_plan.get("findings", []),
        } if args.deploy_plan else None,
        "image_build": {
            "status": image_build.get("status"),
            "image": image_build.get("image"),
            "image_layout": image_build.get("image_layout"),
            "dockerfile": image_build.get("dockerfile"),
            "context": image_build.get("context"),
            "execute_requested": image_build.get("execute_requested"),
            "build_exit_code": ((image_build.get("build") or {}).get("exit_code")),
            "findings": image_build.get("findings", []),
        } if args.image_build else None,
        "image_distribution": {
            "status": image_distribution.get("status"),
            "image": image_distribution.get("image"),
            "nodes": [
                {
                    "name": ((item.get("node") or {}).get("name")),
                    "internal_ip": ((item.get("node") or {}).get("internal_ip")),
                    "available": item.get("available"),
                    "mode": item.get("mode"),
                }
                for item in image_distribution.get("nodes", [])
            ],
            "findings": image_distribution.get("findings", []),
        } if args.image_distribution else None,
        "k8s_bootstrap_path": rel_path(repo_path(args.k8s_bootstrap)) if args.k8s_bootstrap else None,
        "sandbox_plan": rel_path(repo_path(args.sandbox_plan)) if args.sandbox_plan else None,
        "sandbox_execution_path": rel_path(repo_path(args.sandbox_execution)) if args.sandbox_execution else None,
        "sandbox_worker_path": rel_path(repo_path(args.sandbox_worker)) if args.sandbox_worker else None,
        "resource_quota_path": rel_path(repo_path(args.resource_quota)) if args.resource_quota else None,
        "resource_monitor_path": rel_path(repo_path(args.resource_monitor)) if args.resource_monitor else None,
        "workspace_isolation_path": rel_path(repo_path(args.workspace_isolation)) if args.workspace_isolation else None,
        "workspace_cleanup_path": rel_path(repo_path(args.workspace_cleanup)) if args.workspace_cleanup else None,
        "executor_pool_path": rel_path(repo_path(args.executor_pool)) if args.executor_pool else None,
        "executor_pool_stress_path": rel_path(repo_path(args.executor_pool_stress)) if args.executor_pool_stress else None,
        "remote_pool_stress_path": rel_path(repo_path(args.remote_pool_stress)) if args.remote_pool_stress else None,
        "remote_pool_k8s_stress_path": rel_path(repo_path(args.remote_pool_k8s_stress)) if args.remote_pool_k8s_stress else None,
        "remote_pool_k8s_readiness_path": rel_path(repo_path(args.remote_pool_k8s_readiness)) if args.remote_pool_k8s_readiness else None,
        "soak_path": rel_path(repo_path(args.soak)) if args.soak else None,
        "model_profile_path": rel_path(repo_path(args.model_profile)) if args.model_profile else None,
        "llm_review_path": rel_path(repo_path(args.llm_review)) if args.llm_review else None,
        "queue_service_path": rel_path(repo_path(args.queue_service)) if args.queue_service else None,
        "objective_stop_path": rel_path(repo_path(args.objective_stop)) if args.objective_stop else None,
        "sandbox": {
            "status": sandbox.get("status"),
            "driver": sandbox.get("driver"),
            "stage": sandbox.get("stage"),
            "network_allowed": sandbox.get("network_allowed"),
            "policy_path": sandbox.get("policy_path"),
            "findings": sandbox.get("findings", []),
        } if args.sandbox_plan else None,
        "sandbox_execution": {
            "status": sandbox_execution.get("status"),
            "driver": sandbox_execution.get("driver"),
            "execute_requested": sandbox_execution.get("execute_requested"),
            "cleanup": sandbox_execution.get("cleanup"),
            "findings": sandbox_execution.get("findings", []),
        } if args.sandbox_execution else None,
        "sandbox_worker": {
            "status": sandbox_worker.get("status"),
            "driver": sandbox_worker.get("driver"),
            "stage": sandbox_worker.get("stage"),
            "execute_sandbox": sandbox_worker.get("execute_sandbox"),
            "claim_queue": sandbox_worker.get("claim_queue"),
            "selected_count": sandbox_worker.get("selected_count"),
        } if args.sandbox_worker else None,
        "resource_quota": {
            "status": resource_quota.get("status"),
            "policy": ((resource_quota.get("quota") or {}).get("policy")),
            "usage": ((resource_quota.get("quota") or {}).get("usage")),
        } if args.resource_quota else None,
        "resource_monitor": {
            "status": resource_monitor.get("status"),
            "policy_path": resource_monitor.get("policy_path"),
            "pressure": resource_monitor.get("pressure"),
            "admission": resource_monitor.get("admission"),
            "findings": resource_monitor.get("findings", []),
        } if args.resource_monitor else None,
        "workspace_isolation": {
            "status": workspace_isolation.get("status"),
            "mode": workspace_isolation.get("mode"),
            "workspace_root": workspace_isolation.get("workspace_root"),
            "workspaces": workspace_isolation.get("workspaces", []),
            "findings": workspace_isolation.get("findings", []),
        } if args.workspace_isolation else None,
        "workspace_cleanup": {
            "status": workspace_cleanup.get("status"),
            "execute_requested": workspace_cleanup.get("execute_requested"),
            "force": workspace_cleanup.get("force"),
            "workspaces": workspace_cleanup.get("workspaces", []),
            "cleanup_results": workspace_cleanup.get("cleanup_results", []),
            "findings": workspace_cleanup.get("findings", []),
        } if args.workspace_cleanup else None,
        "executor_pool": {
            "status": executor_pool.get("status"),
            "runner": executor_pool.get("runner"),
            "max_workers": executor_pool.get("max_workers"),
            "max_tasks": executor_pool.get("max_tasks"),
            "activate_workspaces": executor_pool.get("activate_workspaces"),
            "queue_backend": executor_pool.get("queue_backend"),
            "claim_queue": executor_pool.get("claim_queue"),
        } if args.executor_pool else None,
        "executor_pool_stress": {
            "status": executor_pool_stress.get("status"),
            "iterations_requested": executor_pool_stress.get("iterations_requested"),
            "max_workers": executor_pool_stress.get("max_workers"),
            "max_tasks": executor_pool_stress.get("max_tasks"),
            "workspace_backend": executor_pool_stress.get("workspace_backend"),
            "create_worktrees": executor_pool_stress.get("create_worktrees"),
            "activate_workspaces": executor_pool_stress.get("activate_workspaces"),
            "cleanup_worktrees": executor_pool_stress.get("cleanup_worktrees"),
            "findings": executor_pool_stress.get("findings", []),
        } if args.executor_pool_stress else None,
        "remote_pool_stress": {
            "status": remote_pool_stress.get("status"),
            "queue_backend": remote_pool_stress.get("queue_backend"),
            "service_backend": remote_pool_stress.get("service_backend"),
            "service_mode": remote_pool_stress.get("service_mode"),
            "service_url": remote_pool_stress.get("service_url"),
            "task_prefix": remote_pool_stress.get("task_prefix"),
            "workers": remote_pool_stress.get("workers"),
            "tasks": remote_pool_stress.get("tasks"),
            "rounds": remote_pool_stress.get("rounds"),
            "duplicate_successful_claims": remote_pool_stress.get("duplicate_successful_claims"),
            "successful_completions": remote_pool_stress.get("successful_completions"),
            "lease_integrity": (remote_pool_stress.get("lease_integrity") or {}).get("passed"),
            "target_counts": remote_pool_stress.get("target_counts"),
            "final_counts": (remote_pool_stress.get("final_queue") or {}).get("counts"),
            "findings": remote_pool_stress.get("findings", []),
        } if args.remote_pool_stress else None,
        "remote_pool_k8s_stress": {
            "status": remote_pool_k8s_stress.get("status"),
            "service_mode": remote_pool_k8s_stress.get("service_mode"),
            "service_url": remote_pool_k8s_stress.get("service_url"),
            "namespace": remote_pool_k8s_stress.get("namespace"),
            "image": remote_pool_k8s_stress.get("image"),
            "image_layout": remote_pool_k8s_stress.get("image_layout"),
            "app_root": remote_pool_k8s_stress.get("app_root"),
            "state_root": remote_pool_k8s_stress.get("state_root"),
            "job_name": remote_pool_k8s_stress.get("job_name"),
            "workers": remote_pool_k8s_stress.get("workers"),
            "tasks": remote_pool_k8s_stress.get("tasks"),
            "rounds": remote_pool_k8s_stress.get("rounds"),
            "task_prefix": remote_pool_k8s_stress.get("task_prefix"),
            "validate_requested": remote_pool_k8s_stress.get("validate_requested"),
            "execute_requested": remote_pool_k8s_stress.get("execute_requested"),
            "validation_exit_code": ((remote_pool_k8s_stress.get("validation") or {}).get("exit_code")),
            "target_counts": ((remote_pool_k8s_stress.get("execution") or {}).get("target_counts")),
            "findings": remote_pool_k8s_stress.get("findings", []),
        } if args.remote_pool_k8s_stress else None,
        "remote_pool_k8s_readiness": {
            "status": remote_pool_k8s_readiness.get("status"),
            "namespace": remote_pool_k8s_readiness.get("namespace"),
            "service_url": remote_pool_k8s_readiness.get("service_url"),
            "service_name": remote_pool_k8s_readiness.get("service_name"),
            "service_port": remote_pool_k8s_readiness.get("service_port"),
            "pvc_name": remote_pool_k8s_readiness.get("pvc_name"),
            "secret_name": remote_pool_k8s_readiness.get("secret_name"),
            "token_key_present": ((remote_pool_k8s_readiness.get("secret") or {}).get("token_key_present")),
            "worker_job": remote_pool_k8s_readiness.get("worker_job"),
            "worker_job_dry_run": remote_pool_k8s_readiness.get("worker_job_dry_run"),
            "execute_gate": remote_pool_k8s_readiness.get("execute_gate"),
            "findings": remote_pool_k8s_readiness.get("findings", []),
        } if args.remote_pool_k8s_readiness else None,
        "soak": {
            "status": soak.get("status"),
            "mode": soak.get("mode"),
            "cycles_requested": soak.get("cycles_requested"),
            "cycles_completed": soak.get("cycles_completed"),
            "interval_seconds": soak.get("interval_seconds"),
            "max_failures": soak.get("max_failures"),
            "worker_runner": soak.get("worker_runner"),
            "worker_stage": soak.get("worker_stage"),
            "queue_backend": soak.get("queue_backend"),
            "findings": soak.get("findings", []),
        } if args.soak else None,
        "model_profile": {
            "status": model_profile.get("status"),
            "selected_profile": model_profile.get("selected_profile"),
            "model": model_profile.get("model"),
            "sandbox": model_profile.get("sandbox"),
            "timeout_seconds": model_profile.get("timeout_seconds"),
            "patch_request": model_profile.get("patch_request"),
            "output_schema": model_profile.get("output_schema"),
            "findings": model_profile.get("findings", []),
        } if args.model_profile else None,
        "llm_review": {
            "status": llm_review.get("status"),
            "selected_profile": llm_review.get("selected_profile"),
            "model": llm_review.get("model"),
            "sandbox": llm_review.get("sandbox"),
            "timeout_seconds": llm_review.get("timeout_seconds"),
            "decision": llm_review.get("decision"),
            "review_request": llm_review.get("review_request"),
            "output_schema": llm_review.get("output_schema"),
            "findings": llm_review.get("findings", []),
        } if args.llm_review else None,
        "queue_service": {
            "status": queue_service.get("status"),
            "backend": queue_service.get("backend"),
            "queue_path": queue_service.get("queue_path"),
            "auth_required": queue_service.get("auth_required"),
            "checks": queue_service.get("checks", []),
            "findings": queue_service.get("findings", []),
        } if args.queue_service else None,
        "k8s_bootstrap": {
            "status": k8s_bootstrap.get("status"),
            "namespace": k8s_bootstrap.get("namespace"),
            "deploy_dir": k8s_bootstrap.get("deploy_dir"),
            "execute_requested": k8s_bootstrap.get("execute_requested"),
            "execute_gate": k8s_bootstrap.get("execute_gate"),
            "token_env_present": ((k8s_bootstrap.get("token_env") or {}).get("present")),
            "manifest_exit_code": ((k8s_bootstrap.get("manifest_validation") or {}).get("exit_code")),
            "secret_exit_code": ((k8s_bootstrap.get("secret_validation") or {}).get("exit_code")),
            "findings": k8s_bootstrap.get("findings", []),
        } if args.k8s_bootstrap else None,
        "objective_stop": {
            "status": objective_stop.get("status"),
            "stop_recommendation": objective_stop.get("stop_recommendation"),
            "objective": objective_stop.get("objective"),
            "open_required_tasks": ((objective_stop.get("tasks") or {}).get("open_required")),
            "release_status": ((objective_stop.get("evidence") or {}).get("release_status")),
            "findings": objective_stop.get("findings", []),
        } if args.objective_stop else None,
        "queue_backend": args.queue_backend,
        "queue_path": display_path(args.queue_path),
        "profile": args.profile,
        "health": {key: value for key, value in health.items() if key != "service"},
        "queue": {key: value for key, value in queue.items() if key != "items"},
        "metrics": {
            "runs_total": metrics["runs"]["total"],
            "runs_by_status": metrics["runs"]["by_status"],
            "runs_by_kind": metrics["runs"]["by_kind"],
        },
        "findings": findings,
        "outputs": [
            "release/release-manifest.json",
            "release/release-manifest.md",
            "release/rollback-plan.md",
            "release/git-status.txt",
            "release/loop-diff.patch",
        ],
    }
    write_json(out_dir / "release-manifest.json", summary)
    write_text(out_dir / "release-manifest.md", render_manifest(summary))
    write_text(out_dir / "rollback-plan.md", render_rollback(summary))
    write_text(out_dir / "git-status.txt", status_text)
    write_text(out_dir / "loop-diff.patch", tracked_diff)
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} findings={len(findings)}")
    return 1 if status == "RELEASE_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())

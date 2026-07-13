#!/usr/bin/env python3
"""Check runtime readiness before supervising Codex Loop execution."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, ensure_run_dir, list_of, load_yaml_subset, rel_path, repo_path, run_git, write_json, write_text
from lock_manager import lock_expired, read_lock
from queue_backend import display_path, normalize_backend
from resource_monitor import DEFAULT_POLICY as DEFAULT_RESOURCE_MONITOR_POLICY
from resource_monitor import build_resource_monitor, monitor_blocked, monitor_digest


DEFAULT_POLICY = SCRIPT_ROOT / "policies" / "runtime-preflight.yaml"
DEFAULT_PROFILES = SCRIPT_ROOT / "policies" / "resource-profiles.yaml"


def finding(level: str, code: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message}


def load_policy(path: Path) -> dict[str, Any]:
    policy = load_yaml_subset(path)
    policy.setdefault("min_python_major", 3)
    policy.setdefault("min_python_minor", 10)
    policy.setdefault("min_free_disk_mb", 512)
    policy.setdefault("min_available_memory_mb", 256)
    policy.setdefault("required_tools", ["git"])
    policy.setdefault("optional_tools", [])
    policy.setdefault("required_paths", ["agent.md", "scripts/codex_loop"])
    policy.setdefault("writable_paths", ["doc/02_acceptance/runs"])
    policy.setdefault("allowed_queue_backends", ["repo-json", "sqlite", "http"])
    policy.setdefault("max_dirty_items_warning", 800)
    policy.setdefault("resource_observability_policy", str(DEFAULT_RESOURCE_MONITOR_POLICY))
    policy.setdefault(
        "profile_required",
        {
            "worker_stage": "prepare",
            "allow_live_write": False,
            "allow_external_codex": False,
        },
    )
    policy.setdefault("profile_bounds", {"max_concurrent_workers": {"min": 1, "max": 4}})
    return policy


def load_profiles(path: Path) -> dict[str, Any]:
    data = load_yaml_subset(path)
    profiles = data.get("profiles") or {}
    if not isinstance(profiles, dict):
        return {}
    return profiles


def selected_profile(name: str, profiles_path: Path) -> tuple[dict[str, Any], list[dict[str, str]]]:
    profiles = load_profiles(profiles_path)
    profile = profiles.get(name)
    if not isinstance(profile, dict):
        return {}, [finding("blocker", "PROFILE_NOT_FOUND", f"Profile `{name}` was not found in {rel_path(profiles_path)}")]
    return profile, []


def available_memory_mb() -> int | None:
    meminfo = Path("/proc/meminfo")
    if not meminfo.exists():
        return None
    for line in meminfo.read_text(encoding="utf-8", errors="replace").splitlines():
        if line.startswith("MemAvailable:"):
            parts = line.split()
            if len(parts) >= 2 and parts[1].isdigit():
                return int(parts[1]) // 1024
    return None


def disk_free_mb(path: Path) -> int | None:
    try:
        usage = shutil.disk_usage(path)
    except OSError:
        return None
    return usage.free // (1024 * 1024)


def nearest_existing_parent(path: Path) -> Path | None:
    current = path if path.is_dir() else path.parent
    while current != current.parent:
        if current.exists():
            return current
        current = current.parent
    return current if current.exists() else None


def path_writable(path: Path) -> tuple[bool, str]:
    target = path if path.exists() else nearest_existing_parent(path)
    if not target:
        return False, "no existing parent"
    return os.access(target, os.W_OK), rel_path(target)


def git_status_count() -> int:
    text = run_git(["status", "--short"])
    return len([line for line in text.splitlines() if line.strip()])


def check_tools(policy: dict[str, Any]) -> tuple[dict[str, Any], list[dict[str, str]]]:
    findings: list[dict[str, str]] = []
    tools: dict[str, Any] = {}
    for tool in [str(item) for item in list_of(policy.get("required_tools"))]:
        path = shutil.which(tool)
        tools[tool] = {"required": True, "path": path}
        if not path:
            findings.append(finding("blocker", "REQUIRED_TOOL_MISSING", f"Required tool `{tool}` was not found on PATH."))
    for tool in [str(item) for item in list_of(policy.get("optional_tools"))]:
        path = shutil.which(tool)
        tools[tool] = {"required": False, "path": path}
        if not path:
            findings.append(finding("warning", "OPTIONAL_TOOL_MISSING", f"Optional tool `{tool}` was not found on PATH."))
    return tools, findings


def check_paths(policy: dict[str, Any]) -> tuple[dict[str, Any], list[dict[str, str]]]:
    findings: list[dict[str, str]] = []
    paths: dict[str, Any] = {}
    for raw in [str(item) for item in list_of(policy.get("required_paths"))]:
        target = repo_path(raw)
        exists = target.exists()
        paths[raw] = {"exists": exists, "path": rel_path(target)}
        if not exists:
            findings.append(finding("blocker", "REQUIRED_PATH_MISSING", f"Required path `{raw}` is missing."))
    for raw in [str(item) for item in list_of(policy.get("writable_paths"))]:
        target = repo_path(raw)
        writable, checked = path_writable(target)
        paths[raw] = {"exists": target.exists(), "path": rel_path(target), "writable": writable, "checked_path": checked}
        if not writable:
            findings.append(finding("blocker", "PATH_NOT_WRITABLE", f"Path `{raw}` is not writable through `{checked}`."))
    return paths, findings


def check_profile(profile_name: str, profile: dict[str, Any], policy: dict[str, Any]) -> tuple[dict[str, Any], list[dict[str, str]]]:
    findings: list[dict[str, str]] = []
    required = policy.get("profile_required") or {}
    for key, expected in required.items():
        actual = profile.get(key)
        if actual != expected:
            findings.append(finding("blocker", "PROFILE_GUARDRAIL_MISMATCH", f"Profile `{profile_name}` has `{key}`={actual!r}; expected {expected!r}."))
    for key, bounds in (policy.get("profile_bounds") or {}).items():
        if key not in profile:
            findings.append(finding("blocker", "PROFILE_BOUND_MISSING", f"Profile `{profile_name}` does not define bounded key `{key}`."))
            continue
        try:
            actual_number = int(profile.get(key))
        except (TypeError, ValueError):
            findings.append(finding("blocker", "PROFILE_BOUND_INVALID", f"Profile `{profile_name}` has non-integer `{key}`={profile.get(key)!r}."))
            continue
        min_value = int((bounds or {}).get("min") or actual_number)
        max_value = int((bounds or {}).get("max") or actual_number)
        if actual_number < min_value or actual_number > max_value:
            findings.append(finding("blocker", "PROFILE_BOUND_VIOLATION", f"Profile `{profile_name}` has `{key}`={actual_number}; expected {min_value}..{max_value}."))
    backend = str(profile.get("queue_backend") or "repo-json")
    allowed = {str(item) for item in list_of(policy.get("allowed_queue_backends"))}
    try:
        normalized = normalize_backend(backend)
    except ValueError:
        normalized = backend
        findings.append(finding("blocker", "QUEUE_BACKEND_INVALID", f"Profile `{profile_name}` uses unsupported queue backend `{backend}`."))
    if normalized not in allowed:
        findings.append(finding("blocker", "QUEUE_BACKEND_NOT_ALLOWED", f"Queue backend `{normalized}` is not allowed by runtime preflight policy."))
    return {"name": profile_name, "profile": profile, "queue_backend": normalized}, findings


def check_queue_path(queue_backend: str, queue_path: str | None, profile: dict[str, Any]) -> tuple[dict[str, Any], list[dict[str, str]]]:
    findings: list[dict[str, str]] = []
    selected_path = queue_path or profile.get("queue_path")
    result = {"backend": queue_backend, "path": display_path(selected_path)}
    if queue_backend == "http":
        value = str(selected_path or os.environ.get("CODEX_LOOP_QUEUE_URL") or "")
        if not value:
            findings.append(finding("blocker", "HTTP_QUEUE_URL_MISSING", "HTTP queue backend requires queue path URL or CODEX_LOOP_QUEUE_URL."))
        elif not value.startswith(("http://", "https://")):
            findings.append(finding("blocker", "HTTP_QUEUE_URL_INVALID", f"HTTP queue backend path must be an http(s) URL, got `{value}`."))
        if not os.environ.get("CODEX_LOOP_QUEUE_TOKEN"):
            findings.append(finding("warning", "HTTP_QUEUE_TOKEN_MISSING", "CODEX_LOOP_QUEUE_TOKEN is not set; remote queue operations may be unauthorized."))
        result.update({"url": value or None, "token_env_present": bool(os.environ.get("CODEX_LOOP_QUEUE_TOKEN"))})
        return result, findings
    if selected_path:
        target = repo_path(str(selected_path))
        writable, checked = path_writable(target)
        result.update({"exists": target.exists(), "writable": writable, "checked_path": checked})
        if not writable:
            findings.append(finding("blocker", "QUEUE_PATH_NOT_WRITABLE", f"Queue path `{selected_path}` is not writable through `{checked}`."))
    return result, findings


def check_resources(policy: dict[str, Any]) -> tuple[dict[str, Any], list[dict[str, str]]]:
    findings: list[dict[str, str]] = []
    repo_free = disk_free_mb(repo_path("."))
    evidence_free = disk_free_mb(repo_path("doc/02_acceptance/runs"))
    mem_available = available_memory_mb()
    min_disk = int(policy.get("min_free_disk_mb") or 0)
    min_mem = int(policy.get("min_available_memory_mb") or 0)
    if repo_free is None or repo_free < min_disk:
        findings.append(finding("blocker", "REPO_DISK_LOW", f"Repo filesystem free MB is {repo_free}; minimum is {min_disk}."))
    if evidence_free is None or evidence_free < min_disk:
        findings.append(finding("blocker", "EVIDENCE_DISK_LOW", f"Evidence filesystem free MB is {evidence_free}; minimum is {min_disk}."))
    if mem_available is not None and mem_available < min_mem:
        findings.append(finding("blocker", "MEMORY_LOW", f"Available memory MB is {mem_available}; minimum is {min_mem}."))
    return {
        "cpu_count": os.cpu_count(),
        "repo_free_mb": repo_free,
        "evidence_free_mb": evidence_free,
        "available_memory_mb": mem_available,
        "min_free_disk_mb": min_disk,
        "min_available_memory_mb": min_mem,
    }, findings


def status_for(findings: list[dict[str, str]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "RUNTIME_PREFLIGHT_BLOCKED"
    if findings:
        return "RUNTIME_PREFLIGHT_DEGRADED"
    return "RUNTIME_PREFLIGHT_READY"


def build_preflight(
    profile_name: str = "conservative",
    profiles_path: str | Path = DEFAULT_PROFILES,
    policy_path: str | Path = DEFAULT_POLICY,
    queue_backend: str | None = None,
    queue_path: str | None = None,
) -> dict[str, Any]:
    policy_target = repo_path(policy_path)
    profiles_target = repo_path(profiles_path)
    policy = load_policy(policy_target)
    profile, findings = selected_profile(profile_name, profiles_target)
    tools, tool_findings = check_tools(policy)
    paths, path_findings = check_paths(policy)
    resources, resource_findings = check_resources(policy)
    profile_info, profile_findings = check_profile(profile_name, profile, policy) if profile else ({"name": profile_name, "profile": {}}, [])
    selected_backend = normalize_backend(queue_backend or profile_info.get("queue_backend") or "repo-json")
    queue, queue_findings = check_queue_path(selected_backend, queue_path, profile)
    resource_policy = profile.get("resource_observability_policy") or policy.get("resource_observability_policy") or DEFAULT_RESOURCE_MONITOR_POLICY
    resource_monitor = build_resource_monitor(resource_policy, selected_backend, queue_path or profile.get("queue_path"))
    current_lock = read_lock()
    dirty_items = git_status_count()
    max_dirty = int(policy.get("max_dirty_items_warning") or 0)
    if max_dirty and dirty_items > max_dirty:
        findings.append(finding("warning", "WORKTREE_DIRTY_LARGE", f"git status has {dirty_items} items; warning threshold is {max_dirty}."))
    if current_lock and lock_expired(current_lock):
        findings.append(finding("blocker", "WORKSPACE_LOCK_EXPIRED", "Workspace lock is expired and should be recovered before service execution."))
    findings.extend(tool_findings)
    findings.extend(path_findings)
    findings.extend(resource_findings)
    findings.extend(profile_findings)
    findings.extend(queue_findings)
    findings.extend(resource_monitor.get("findings", []))
    if monitor_blocked(resource_monitor):
        findings.append(finding("blocker", "RESOURCE_MONITOR_BLOCKED", "Dynamic resource monitor has blocker findings."))

    python_required = (int(policy.get("min_python_major") or 3), int(policy.get("min_python_minor") or 10))
    python_current = sys.version_info[:2]
    if python_current < python_required:
        findings.append(finding("blocker", "PYTHON_TOO_OLD", f"Python {python_current[0]}.{python_current[1]} is below required {python_required[0]}.{python_required[1]}."))

    generated_at = datetime.now().isoformat(timespec="seconds")
    return {
        "generated_at": generated_at,
        "run_kind": "runtime_preflight",
        "status": status_for(findings),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "policy_path": rel_path(policy_target),
        "profiles_path": rel_path(profiles_target),
        "profile": profile_info,
        "queue": queue,
        "python": {"version": sys.version.split()[0], "required": f"{python_required[0]}.{python_required[1]}"},
        "tools": tools,
        "paths": paths,
        "resources": resources,
        "resource_monitor": monitor_digest(resource_monitor),
        "workspace": {
            "dirty_items": dirty_items,
            "lock_present": bool(current_lock),
            "lock_expired": lock_expired(current_lock) if current_lock else False,
            "lock": current_lock,
        },
        "findings": findings,
    }


def render_report(preflight: dict[str, Any]) -> str:
    resources = preflight.get("resources") or {}
    resource_monitor = preflight.get("resource_monitor") or {}
    pressure = resource_monitor.get("pressure") or {}
    queue = preflight.get("queue") or {}
    workspace = preflight.get("workspace") or {}
    lines = [
        "# Codex Loop Runtime Preflight",
        "",
        f"- status: `{preflight['status']}`",
        f"- generated_at: `{preflight['generated_at']}`",
        f"- profile: `{preflight.get('profile', {}).get('name')}`",
        f"- queue_backend: `{queue.get('backend')}`",
        f"- queue_path: `{queue.get('path') or 'default'}`",
        f"- repo_free_mb: `{resources.get('repo_free_mb')}`",
        f"- evidence_free_mb: `{resources.get('evidence_free_mb')}`",
        f"- available_memory_mb: `{resources.get('available_memory_mb')}`",
        f"- monitor_status: `{resource_monitor.get('status')}`",
        f"- monitor_cpu_busy_percent: `{pressure.get('cpu_busy_percent')}`",
        f"- monitor_queue_claimed: `{pressure.get('queue_claimed')}`",
        f"- dirty_items: `{workspace.get('dirty_items')}`",
        "",
        "## Findings",
    ]
    findings = preflight.get("findings") or []
    if findings:
        for item in findings:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Preflight only proves runtime readiness for loop supervision; it does not close product tasks.",
            "- Warnings are visible in health and release manifests, but only blockers stop execution.",
            "",
        ]
    )
    return "\n".join(lines)


def write_preflight_artifacts(run_id: str, preflight: dict[str, Any], write_summary: bool = True) -> Path:
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "preflight"
    out_dir.mkdir(parents=True, exist_ok=True)
    write_json(out_dir / "preflight.json", preflight)
    write_text(out_dir / "preflight.md", render_report(preflight))
    if write_summary:
        write_json(
            run_dir / "run-summary.json",
            {
                "run_id": run_id,
                "run_kind": "runtime_preflight",
                "status": preflight["status"],
                "created_at": preflight["generated_at"],
                "commit": preflight["commit"],
                "outputs": ["preflight/preflight.json", "preflight/preflight.md"],
            },
        )
    return out_dir


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--profile", default="conservative")
    parser.add_argument("--profiles", default=str(DEFAULT_PROFILES))
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--queue-backend", choices=["repo-json", "sqlite", "http"], default=None)
    parser.add_argument("--queue-path", default=None)
    args = parser.parse_args()

    run_id = args.run_id or datetime.now().strftime("%Y%m%d-%H%M%S-preflight")
    preflight = build_preflight(args.profile, args.profiles, args.policy, args.queue_backend, args.queue_path)
    out_dir = write_preflight_artifacts(run_id, preflight, write_summary=True)
    print(out_dir)
    print(f"status={preflight['status']} findings={len(preflight.get('findings') or [])}")
    return 1 if preflight["status"] == "RUNTIME_PREFLIGHT_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())

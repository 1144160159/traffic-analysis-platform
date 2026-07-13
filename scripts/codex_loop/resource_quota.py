#!/usr/bin/env python3
"""Evaluate lane and resource quotas for Codex Loop scheduling."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, ensure_run_dir, list_of, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


DEFAULT_POLICY = SCRIPT_ROOT / "policies" / "resource-quotas.yaml"


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def load_policy(path: str | Path | None = None) -> dict[str, Any]:
    target = repo_path(path) if path else DEFAULT_POLICY
    policy = load_yaml_subset(target)
    policy.setdefault("enabled", True)
    policy.setdefault("default_weight", 1)
    policy.setdefault("default_lane_limit", 1)
    policy.setdefault("default_subsystem_limit", 2)
    policy.setdefault("max_total_weight", 4)
    policy.setdefault("live_generated_limit", 1)
    policy.setdefault("release_freeze_limit", 1)
    policy.setdefault("lane_limits", {})
    policy.setdefault("mode_limits", {})
    policy["_path"] = rel_path(target)
    return policy


def load_tasks(tasks_dir: Path) -> dict[str, dict[str, Any]]:
    tasks: dict[str, dict[str, Any]] = {}
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = load_yaml_subset(path)
        task["_path"] = rel_path(path)
        tasks[str(task.get("id"))] = task
    return tasks


def primary_lane(item: dict[str, Any]) -> str:
    lane = item.get("lane") or {}
    return str(item.get("lane_primary") or lane.get("primary") or "unknown")


def item_mode(item: dict[str, Any]) -> str:
    execution = item.get("execution") or {}
    return str(item.get("mode") or execution.get("mode") or "plan")


def item_data_mode(item: dict[str, Any]) -> str:
    data_plan = item.get("data_plan") or {}
    return str(item.get("data_mode") or data_plan.get("mode") or "none")


def lane_limit(policy: dict[str, Any], lane: str) -> dict[str, Any]:
    limits = policy.get("lane_limits") or {}
    value = limits.get(lane) or {}
    return value if isinstance(value, dict) else {"max_selected": int(value)}


def lane_weight(policy: dict[str, Any], lane: str) -> int:
    limits = lane_limit(policy, lane)
    return int(limits.get("weight") or policy.get("default_weight") or 1)


def lane_group(policy: dict[str, Any], lane: str) -> str:
    limits = lane_limit(policy, lane)
    return str(limits.get("group") or lane)


def mode_limit(policy: dict[str, Any], mode: str) -> int | None:
    limits = policy.get("mode_limits") or {}
    if mode not in limits:
        return None
    return int(limits.get(mode) or 0)


def normalize_item(item: dict[str, Any]) -> dict[str, Any]:
    lane = item.get("lane") or {}
    normalized = dict(item)
    normalized["lane_primary"] = primary_lane(item)
    normalized["lane_dependent"] = list_of(item.get("lane_dependent") or lane.get("dependent"))
    normalized["mode"] = item_mode(item)
    normalized["data_mode"] = item_data_mode(item)
    normalized["subsystems"] = [str(value) for value in list_of(item.get("subsystems"))]
    return normalized


def item_from_task(task: dict[str, Any], recommendation: dict[str, Any] | None = None) -> dict[str, Any]:
    recommendation = recommendation or {}
    item = {
        "id": str(task.get("id")),
        "title": task.get("title"),
        "priority": task.get("priority"),
        "status": task.get("status"),
        "score": recommendation.get("score"),
        "mode": (task.get("execution") or {}).get("mode"),
        "data_mode": (task.get("data_plan") or {}).get("mode"),
        "lane_primary": (task.get("lane") or {}).get("primary"),
        "lane_dependent": list_of((task.get("lane") or {}).get("dependent")),
        "subsystems": list_of(task.get("subsystems")),
        "acceptance_type": task.get("acceptance_type"),
        "risk_level": (task.get("risk") or {}).get("level"),
        "allowed_paths": list_of((task.get("workspace") or {}).get("allowed_paths")),
        "path": task.get("_path"),
    }
    return normalize_item(item)


def new_usage() -> dict[str, Any]:
    return {
        "total_weight": 0,
        "lanes": Counter(),
        "lane_groups": Counter(),
        "modes": Counter(),
        "data_modes": Counter(),
        "subsystems": Counter(),
        "selected": [],
    }


def quota_findings(item: dict[str, Any], usage: dict[str, Any], policy: dict[str, Any]) -> list[dict[str, Any]]:
    if policy.get("enabled") is False:
        return []
    normalized = normalize_item(item)
    lane = primary_lane(normalized)
    mode = item_mode(normalized)
    data_mode = item_data_mode(normalized)
    weight = lane_weight(policy, lane)
    findings: list[dict[str, Any]] = []
    max_total_weight = int(policy.get("max_total_weight") or 0)
    if max_total_weight and int(usage["total_weight"]) + weight > max_total_weight:
        findings.append({"code": "TOTAL_WEIGHT_EXCEEDED", "message": f"total resource weight would exceed {max_total_weight}", "limit": max_total_weight})
    limit = int(lane_limit(policy, lane).get("max_selected") or policy.get("default_lane_limit") or 1)
    if limit >= 0 and int(usage["lanes"][lane]) >= limit:
        findings.append({"code": "LANE_QUOTA_EXCEEDED", "message": f"lane `{lane}` already has {usage['lanes'][lane]}/{limit} selected", "limit": limit})
    mode_cap = mode_limit(policy, mode)
    if mode_cap is not None and int(usage["modes"][mode]) >= mode_cap:
        findings.append({"code": "MODE_QUOTA_EXCEEDED", "message": f"mode `{mode}` already has {usage['modes'][mode]}/{mode_cap} selected", "limit": mode_cap})
    if data_mode == "live_generated":
        live_limit = int(policy.get("live_generated_limit") or 1)
        if int(usage["data_modes"][data_mode]) >= live_limit:
            findings.append({"code": "LIVE_GENERATED_QUOTA_EXCEEDED", "message": f"live_generated already has {usage['data_modes'][data_mode]}/{live_limit} selected", "limit": live_limit})
    if mode == "release-freeze":
        release_limit = int(policy.get("release_freeze_limit") or 1)
        if int(usage["modes"][mode]) >= release_limit:
            findings.append({"code": "RELEASE_FREEZE_QUOTA_EXCEEDED", "message": f"release-freeze already has {usage['modes'][mode]}/{release_limit} selected", "limit": release_limit})
    subsystem_limit = int(policy.get("default_subsystem_limit") or 0)
    if subsystem_limit:
        for subsystem in list_of(normalized.get("subsystems")):
            if int(usage["subsystems"][str(subsystem)]) >= subsystem_limit:
                findings.append({"code": "SUBSYSTEM_QUOTA_EXCEEDED", "message": f"subsystem `{subsystem}` already has {usage['subsystems'][str(subsystem)]}/{subsystem_limit} selected", "limit": subsystem_limit})
    return findings


def record_item(item: dict[str, Any], usage: dict[str, Any], policy: dict[str, Any]) -> dict[str, Any]:
    normalized = normalize_item(item)
    lane = primary_lane(normalized)
    mode = item_mode(normalized)
    data_mode = item_data_mode(normalized)
    weight = lane_weight(policy, lane)
    usage["total_weight"] += weight
    usage["lanes"][lane] += 1
    usage["lane_groups"][lane_group(policy, lane)] += 1
    usage["modes"][mode] += 1
    usage["data_modes"][data_mode] += 1
    for subsystem in list_of(normalized.get("subsystems")):
        usage["subsystems"][str(subsystem)] += 1
    usage["selected"].append(str(normalized.get("id")))
    normalized["resource_weight"] = weight
    normalized["resource_group"] = lane_group(policy, lane)
    return normalized


def usage_snapshot(usage: dict[str, Any]) -> dict[str, Any]:
    return {
        "total_weight": int(usage["total_weight"]),
        "lanes": dict(usage["lanes"]),
        "lane_groups": dict(usage["lane_groups"]),
        "modes": dict(usage["modes"]),
        "data_modes": dict(usage["data_modes"]),
        "subsystems": dict(usage["subsystems"]),
        "selected": list(usage["selected"]),
    }


def apply_quotas(items: list[dict[str, Any]], policy: dict[str, Any], max_items: int) -> dict[str, Any]:
    usage = new_usage()
    selected: list[dict[str, Any]] = []
    deferred: list[dict[str, Any]] = []
    for raw in items:
        item = normalize_item(raw)
        findings = quota_findings(item, usage, policy)
        if findings:
            first = findings[0]
            deferred.append({"id": item.get("id"), "reason": first["message"], "quota_code": first["code"], "findings": findings})
            continue
        selected.append(record_item(item, usage, policy))
        if len(selected) >= max_items:
            break
    return {
        "enabled": bool(policy.get("enabled", True)),
        "policy": policy.get("_path"),
        "max_items": max_items,
        "selected": selected,
        "deferred": deferred,
        "usage": usage_snapshot(usage),
    }


def candidates_from_guidance(guidance: dict[str, Any], tasks: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    candidates: list[dict[str, Any]] = []
    for recommendation in guidance.get("recommended_next", []):
        task_id = str(recommendation.get("id"))
        task = tasks.get(task_id)
        if task:
            candidates.append(item_from_task(task, recommendation))
    return candidates


def audit_scheduler_plan(plan: dict[str, Any], policy: dict[str, Any]) -> dict[str, Any]:
    usage = new_usage()
    findings: list[dict[str, Any]] = []
    selected: list[dict[str, Any]] = []
    for item in plan.get("queue", {}).get("selected", []):
        normalized = normalize_item(item)
        item_findings = quota_findings(normalized, usage, policy)
        if item_findings:
            findings.append({"id": normalized.get("id"), "findings": item_findings})
        selected.append(record_item(normalized, usage, policy))
    return {
        "enabled": bool(policy.get("enabled", True)),
        "policy": policy.get("_path"),
        "selected": selected,
        "findings": findings,
        "usage": usage_snapshot(usage),
        "status": "RESOURCE_QUOTA_BLOCKED" if findings else "RESOURCE_QUOTA_READY",
    }


def render_report(summary: dict[str, Any]) -> str:
    quota = summary.get("quota") or {}
    usage = quota.get("usage") or {}
    lines = [
        "# Codex Loop Resource Quota",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- policy: `{quota.get('policy')}`",
        f"- total_weight: `{usage.get('total_weight')}`",
        f"- selected: `{len(quota.get('selected') or [])}`",
        f"- deferred: `{len(quota.get('deferred') or [])}`",
        f"- findings: `{len(quota.get('findings') or [])}`",
        "",
        "## Usage",
        f"- lanes: `{usage.get('lanes')}`",
        f"- modes: `{usage.get('modes')}`",
        f"- subsystems: `{usage.get('subsystems')}`",
        "",
        "## Deferred",
    ]
    deferred = quota.get("deferred") or []
    if deferred:
        for item in deferred:
            lines.append(f"- `{item.get('id')}`: {item.get('reason')}")
    else:
        lines.append("- none")
    findings = quota.get("findings") or []
    if findings:
        lines.extend(["", "## Findings"])
        for item in findings:
            lines.append(f"- `{item.get('id')}`: {item.get('findings')}")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Quota evaluation only constrains scheduling; it does not execute tasks or close evidence gates.",
            "- Per-lane quota is a control-plane prerequisite for future multi-worker execution.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--scheduler-plan", default=None)
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--max-items", type=int, default=3)
    parser.add_argument("--run-id", default=None)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("resource-quota")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "resource-quota"
    out_dir.mkdir(parents=True, exist_ok=True)
    policy = load_policy(args.policy)
    if args.scheduler_plan:
        plan_path = repo_path(args.scheduler_plan)
        quota = audit_scheduler_plan(load_json(plan_path), policy)
        source = {"scheduler_plan": rel_path(plan_path)}
    else:
        guidance_path = repo_path(args.guidance) if args.guidance else None
        guidance = load_json(guidance_path)
        tasks = load_tasks(repo_path(args.tasks_dir))
        quota = apply_quotas(candidates_from_guidance(guidance, tasks), policy, args.max_items)
        quota["status"] = "RESOURCE_QUOTA_READY"
        source = {"guidance": rel_path(guidance_path) if guidance_path else None, "tasks_dir": rel_path(repo_path(args.tasks_dir))}
    status = quota.get("status") or ("RESOURCE_QUOTA_BLOCKED" if quota.get("findings") else "RESOURCE_QUOTA_READY")
    summary = {
        "run_id": run_id,
        "run_kind": "resource_quota",
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "source": source,
        "quota": quota,
        "outputs": ["resource-quota/resource-quota.json", "resource-quota/resource-quota.md"],
    }
    write_json(out_dir / "resource-quota.json", summary)
    write_text(out_dir / "resource-quota.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} selected={len(quota.get('selected') or [])} deferred={len(quota.get('deferred') or [])} findings={len(quota.get('findings') or [])}")
    return 1 if status == "RESOURCE_QUOTA_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())

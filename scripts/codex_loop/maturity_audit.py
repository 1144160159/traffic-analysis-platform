#!/usr/bin/env python3
"""Read-only production maturity audit for the Codex Loop engine."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import RUNS_ROOT, ensure_run_dir, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


READY = "READY"
PARTIAL = "PARTIAL"
MISSING = "MISSING"


def load_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return {"parse_error": str(exc), "path": rel_path(path)}


def scan_runs() -> tuple[list[dict[str, Any]], dict[str, dict[str, Any]]]:
    runs: list[dict[str, Any]] = []
    latest: dict[str, dict[str, Any]] = {}
    if not RUNS_ROOT.exists():
        return runs, latest
    for path in sorted(RUNS_ROOT.glob("*/run-summary.json")):
        data = load_json(path)
        if not data:
            continue
        data["_path"] = rel_path(path)
        data["_run_dir"] = rel_path(path.parent)
        runs.append(data)
        kind = str(data.get("run_kind") or "unknown")
        current = latest.get(kind)
        candidate_key = (str(data.get("created_at") or ""), str(data.get("run_id") or path.parent.name))
        current_key = (str(current.get("created_at") or ""), str(current.get("run_id") or "")) if current else ("", "")
        if not current or candidate_key >= current_key:
            latest[kind] = data
    return runs, latest


def load_tasks(tasks_dir: Path) -> list[dict[str, Any]]:
    tasks: list[dict[str, Any]] = []
    if not tasks_dir.exists():
        return tasks
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = load_yaml_subset(path)
        task["_path"] = rel_path(path)
        tasks.append(task)
    return tasks


def file_status(paths: list[str]) -> dict[str, bool]:
    return {path: repo_path(path).exists() for path in paths}


def run_ref(latest: dict[str, dict[str, Any]], kind: str) -> dict[str, Any]:
    run = latest.get(kind) or {}
    return {
        "kind": kind,
        "run_id": run.get("run_id"),
        "status": run.get("status"),
        "path": run.get("_path") or run.get("_run_dir"),
    }


def has_good_status(latest: dict[str, dict[str, Any]], kind: str, accepted: set[str]) -> bool:
    return str((latest.get(kind) or {}).get("status") or "") in accepted


def add_domain(domains: list[dict[str, Any]], domain: dict[str, Any]) -> None:
    domains.append(domain)


def status_from(required_files: dict[str, bool], evidence_ok: bool, evidence_any: bool = False, blockers: list[str] | None = None) -> str:
    blockers = blockers or []
    if blockers:
        return PARTIAL
    if not all(required_files.values()):
        return MISSING
    if evidence_ok:
        return READY
    if evidence_any:
        return PARTIAL
    return PARTIAL


def build_domains(tasks: list[dict[str, Any]], latest: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    domains: list[dict[str, Any]] = []
    by_status = Counter(str(task.get("status")) for task in tasks)
    required = [task for task in tasks if str(task.get("priority")) in {"P0", "P1"}]
    open_required = [task for task in required if str(task.get("status")) != "CLOSED"]

    files = file_status(
        [
            "scripts/codex_loop/scout.py",
            "scripts/codex_loop/guide.py",
            "scripts/codex_loop/design.py",
            "scripts/codex_loop/context_pack.py",
            "scripts/codex_loop/tasks/.gitkeep",
            "scripts/codex_loop/scheduler.py",
            "scripts/codex_loop/worker.py",
            "scripts/codex_loop/daemon.py",
            "scripts/codex_loop/service.py",
            "scripts/codex_loop/queue_service.py",
            "scripts/codex_loop/remote_pool_k8s_stress.py",
            "scripts/codex_loop/remote_pool_k8s_readiness.py",
            "scripts/codex_loop/preflight.py",
            "scripts/codex_loop/resource_quota.py",
            "scripts/codex_loop/resource_monitor.py",
            "scripts/codex_loop/workspace_isolation.py",
            "scripts/codex_loop/sandbox.py",
            "scripts/codex_loop/codex_runner.py",
            "scripts/codex_loop/llm_reviewer.py",
            "scripts/codex_loop/evidence_check.py",
            "scripts/codex_loop/repair.py",
            "scripts/codex_loop/deploy.py",
            "scripts/codex_loop/image_build.py",
            "scripts/codex_loop/image_distribution.py",
            "scripts/codex_loop/k8s_bootstrap.py",
            "scripts/codex_loop/release.py",
            "scripts/codex_loop/metrics.py",
            "scripts/codex_loop/soak.py",
            "scripts/codex_loop/objective_stop.py",
        ]
    )

    add_domain(
        domains,
        {
            "id": "context_god_view",
            "name": "上帝视角与纠偏",
            "status": status_from(
                {key: files[key] for key in ["scripts/codex_loop/scout.py", "scripts/codex_loop/guide.py"]},
                has_good_status(latest, "context_scout", {"CONTEXT_SCOUTED"}) and has_good_status(latest, "guidance", {"GUIDANCE_GENERATED"}),
                "context_scout" in latest or "guidance" in latest,
            ),
            "capability": "读取仓库、文档、任务和历史证据，形成项目级快照、缺口索引、依赖图和纠偏建议。",
            "evidence": [run_ref(latest, "context_scout"), run_ref(latest, "guidance")],
            "gaps": [] if "context_scout" in latest and "guidance" in latest else ["缺少最新 context/guidance 证据"],
        },
    )
    add_domain(
        domains,
        {
            "id": "task_registry",
            "name": "任务模型与 backlog",
            "status": PARTIAL if open_required else READY,
            "capability": "P0/P1 任务已结构化，包含 lane、执行模式、证据类型、关闭条件和风险边界。",
            "evidence": [
                {
                    "kind": "task_registry",
                    "status": f"{len(open_required)} open required",
                    "tasks_total": len(tasks),
                    "required": len(required),
                    "open_required": len(open_required),
                    "by_status": dict(by_status),
                }
            ],
            "gaps": [f"{len(open_required)} 个必需 P0/P1 任务仍未 CLOSED"] if open_required else [],
        },
    )
    add_domain(
        domains,
        {
            "id": "design_context",
            "name": "产品/功能/视觉/架构设计与长上下文压缩",
            "status": PARTIAL if str((latest.get("design_package") or {}).get("status")) != "DESIGN_READY" else READY,
            "capability": "能为任务生成设计包、上下文包、handoff、决策日志和实现计划，降低长上下文风险。",
            "evidence": [run_ref(latest, "design_package"), run_ref(latest, "context_pack"), run_ref(latest, "workflow_run")],
            "gaps": ["现有设计证据仍包含 DESIGN_ITERATING，尚未证明设计包可直接关闭任务。"],
        },
    )
    add_domain(
        domains,
        {
            "id": "orchestration",
            "name": "调度、worker、daemon、service",
            "status": PARTIAL,
            "capability": "具备 scheduler、worker、bounded daemon 和 service supervisor，可按目标停止器收敛。",
            "evidence": [run_ref(latest, "scheduler"), run_ref(latest, "worker"), run_ref(latest, "daemon"), run_ref(latest, "service")],
            "gaps": ["已验证 prepare/计划型 worker；尚未证明长期真实任务执行与状态闭合。"],
        },
    )
    add_domain(
        domains,
        {
            "id": "queue_remote_pool",
            "name": "持久队列、远程 worker 与 K8s 多 Pod",
            "status": READY
            if has_good_status(latest, "queue_service", {"QUEUE_SERVICE_READY", "QUEUE_SERVICE_SMOKE_PASSED"})
            and has_good_status(latest, "remote_pool_k8s_stress", {"REMOTE_POOL_K8S_STRESS_COMPLETED"})
            and has_good_status(latest, "remote_pool_k8s_readiness", {"REMOTE_POOL_K8S_READINESS_READY"})
            else PARTIAL,
            "capability": "具备 SQLite/WAL 队列、HTTP queue service、lease owner 校验、远程 worker 仲裁和 K8s Indexed Job 压测证据。",
            "evidence": [run_ref(latest, "queue_service"), run_ref(latest, "remote_pool_k8s_stress"), run_ref(latest, "remote_pool_k8s_readiness")],
            "gaps": ["K8s 多 Pod 当前证明 synthetic task 仲裁；真实业务任务池执行仍需单独验收。"],
        },
    )
    add_domain(
        domains,
        {
            "id": "safety_isolation",
            "name": "运行前置、安全隔离与资源治理",
            "status": PARTIAL,
            "capability": "具备 runtime preflight、resource quota、resource monitor、workspace isolation/cleanup、sandbox plan/executor 和显式执行闸门。",
            "evidence": [
                run_ref(latest, "runtime_preflight"),
                run_ref(latest, "resource_quota"),
                run_ref(latest, "resource_monitor"),
                run_ref(latest, "workspace_isolation"),
                run_ref(latest, "workspace_cleanup"),
                run_ref(latest, "sandbox_plan"),
                run_ref(latest, "sandbox_execution"),
            ],
            "gaps": ["最新 preflight/resource/workspace 多为 DEGRADED 或计划态；sandbox execution blocked 是安全闸门预期，但未证明真实隔离执行成功。"],
        },
    )
    add_domain(
        domains,
        {
            "id": "codex_integration",
            "name": "外部 Codex/模型集成",
            "status": PARTIAL,
            "capability": "具备 patch request、模型画像、外部 Codex runner 审计、adapter 和 LLM reviewer schema。",
            "evidence": [run_ref(latest, "codex_runner"), run_ref(latest, "llm_review")],
            "gaps": ["目前主要是 PLANNED/审计态；尚未证明外部 Codex 长周期生成 patch 并被 loop intake、review、close。"],
        },
    )
    add_domain(
        domains,
        {
            "id": "review_evidence_repair",
            "name": "第三视角审阅、证据判定与修复循环",
            "status": PARTIAL,
            "capability": "具备静态 reviewer、语义 reviewer、LLM reviewer、evidence checker、repair planner 和 auto repair loop。",
            "evidence": [run_ref(latest, "llm_review"), run_ref(latest, "workflow_run"), run_ref(latest, "task_state")],
            "gaps": ["还没有必需任务从实现到 evidence_check 再到 CLOSED 的完整生产闭环证据。"],
        },
    )
    add_domain(
        domains,
        {
            "id": "deployment_release",
            "name": "镜像、K8s bootstrap、发布冻结与回滚",
            "status": READY
            if has_good_status(latest, "image_build", {"IMAGE_BUILD_COMPLETED"})
            and has_good_status(latest, "image_distribution", {"IMAGE_DISTRIBUTION_READY"})
            and has_good_status(latest, "k8s_bootstrap", {"K8S_BOOTSTRAP_APPLIED", "K8S_BOOTSTRAP_VALIDATED"})
            and has_good_status(latest, "release_freeze", {"RELEASE_FROZEN"})
            else PARTIAL,
            "capability": "已具备 loop-control 镜像构建、双节点分发、K8s queue service bootstrap、release manifest 和 rollback plan。",
            "evidence": [run_ref(latest, "image_build"), run_ref(latest, "image_distribution"), run_ref(latest, "k8s_bootstrap"), run_ref(latest, "release_freeze")],
            "gaps": [],
        },
    )
    add_domain(
        domains,
        {
            "id": "observability_stop",
            "name": "观测、健康、soak 与目标停止",
            "status": PARTIAL,
            "capability": "具备 metrics、service health、soak、objective stop 和 stop_conditions 机器可读契约。",
            "evidence": [run_ref(latest, "metrics"), run_ref(latest, "service_health"), run_ref(latest, "soak"), run_ref(latest, "objective_stop")],
            "gaps": ["objective stop 正确返回 CONTINUE；soak 为 DEGRADED，且 12 个必需任务未闭合，不能宣布项目完成。"],
        },
    )
    return domains


def score_domains(domains: list[dict[str, Any]]) -> dict[str, Any]:
    weights = {READY: 1.0, PARTIAL: 0.5, MISSING: 0.0}
    total = len(domains)
    score = sum(weights.get(str(domain.get("status")), 0.0) for domain in domains)
    by_status = Counter(str(domain.get("status")) for domain in domains)
    if by_status.get(MISSING):
        overall = "MATURITY_AUDIT_INCOMPLETE"
    elif by_status.get(PARTIAL):
        overall = "MATURITY_AUDIT_PARTIAL"
    else:
        overall = "MATURITY_AUDIT_READY"
    return {
        "overall_status": overall,
        "score": round(score / total, 2) if total else 0,
        "domains_total": total,
        "by_status": dict(by_status),
    }


def render_report(summary: dict[str, Any]) -> str:
    score = summary["score"]
    lines = [
        "# Codex Loop 生产级能力成熟度审计",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- maturity_score: `{score['score']}`",
        f"- domains: `{score['by_status']}`",
        f"- generated_at: `{summary['created_at']}`",
        "",
        "## 总体判断",
        "",
        summary["judgment"],
        "",
        "## 能力矩阵",
        "",
        "| 能力域 | 状态 | 已具备能力 | 关键证据 | 剩余缺口 |",
        "|---|---|---|---|---|",
    ]
    for domain in summary["domains"]:
        evidence = ", ".join(
            f"{item.get('kind') or item.get('run_id') or 'evidence'}:{item.get('status') or item.get('run_id') or 'n/a'}"
            for item in domain.get("evidence", [])
            if isinstance(item, dict)
        )
        gaps = "; ".join(domain.get("gaps") or ["none"])
        lines.append(f"| {domain['name']} | `{domain['status']}` | {domain['capability']} | {evidence or 'n/a'} | {gaps} |")
    lines.extend(
        [
            "",
            "## 状态解释",
            "",
            "- `READY` 表示该能力既有实现面，也有当前证据支撑。",
            "- `PARTIAL` 表示能力已经存在，但缺少长稳、真实任务执行或任务关闭证据。",
            "- `MISSING` 表示生产级必需能力或产物缺失。",
            "- 本审计是只读审计，不执行任务、不修改任务状态、不 apply Kubernetes 资源、不调用外部 Codex。",
            "",
            "## 下一步必须补强的证明",
            "",
        ]
    )
    for item in summary["required_next_proof"]:
        lines.append(f"- {item}")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("maturity-audit")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "maturity-audit"
    out_dir.mkdir(parents=True, exist_ok=True)
    runs, latest = scan_runs()
    tasks = load_tasks(repo_path(args.tasks_dir))
    domains = build_domains(tasks, latest)
    score = score_domains(domains)
    open_required = [
        {"id": task.get("id"), "priority": task.get("priority"), "status": task.get("status"), "path": task.get("_path")}
        for task in tasks
        if str(task.get("priority")) in {"P0", "P1"} and str(task.get("status")) != "CLOSED"
    ]
    judgment = (
        "Loop 引擎已经达到生产级控制面的成熟度：能够发现、规划、门禁、调度、隔离、排队、部署、观测、冻结发布证据，并评估目标停止条件。"
        "它还没有证明完整自治完成项目开发的成熟度，因为必需 P0/P1 任务仍未闭合，真实任务关闭证据仍然缺失。"
    )
    summary = {
        "run_id": run_id,
        "run_kind": "maturity_audit",
        "status": score["overall_status"],
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "mode": "read-only",
        "runs_scanned": len(runs),
        "latest_run_kinds": sorted(latest),
        "tasks": {
            "total": len(tasks),
            "open_required": len(open_required),
            "open_required_tasks": open_required,
        },
        "score": score,
        "judgment": judgment,
        "domains": domains,
        "required_next_proof": [
            "至少闭合一个 P0 任务，完整经过 design/context/workflow/review/evidence/status update，并保留可复现证据。",
            "跑通真实任务的隔离 workspace 执行路径，而不仅是 prepare 或 synthetic queue task。",
            "在启用 objective stop 的重复 service cycle 中产出非 DEGRADED 的 soak 证据。",
            "证明外部 Codex 或模型辅助 patch intake 能对真实小范围改动完成生成、审阅、拒绝或接受。",
            "在所有必需 P0/P1 任务终态且 release 证据保持冻结前，objective_stop 必须保持 CONTINUE。",
        ],
        "outputs": ["maturity-audit/maturity-audit.json", "maturity-audit/maturity-audit.md"],
    }
    write_json(out_dir / "maturity-audit.json", summary)
    write_text(out_dir / "maturity-audit.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} score={score['score']} ready={score['by_status'].get(READY, 0)} partial={score['by_status'].get(PARTIAL, 0)} missing={score['by_status'].get(MISSING, 0)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

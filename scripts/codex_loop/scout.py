#!/usr/bin/env python3
"""Generate the Context Scout "god view" ledgers for this repository."""

from __future__ import annotations

import argparse
import json
import re
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
    REPO_ROOT,
    RUNS_ROOT,
    ensure_run_dir,
    list_of,
    load_yaml_subset,
    rel_path,
    repo_path,
    run_command,
    run_git,
    sha256_file,
    write_json,
    write_text,
)


DOC_INPUTS = [
    "agent.md",
    "doc/README.md",
    "doc/01_design/课题一产品与技术总体设计.md",
    "doc/01_design/Codex-Loop-Engineering-设计.md",
    "doc/01_design/自动开发Loop引擎设计.md",
    "doc/02_acceptance/README.md",
    "doc/03_review/专家深评整改清单.md",
    "doc/05_status/未开发项梳理-2026-06-19.md",
    "doc/05_status/代码实证状态核对-2026-06-19.md",
]

SUBSYSTEMS = {
    "rust_probe": ["rust/probe-agent"],
    "go_control_plane": ["go/control-plane"],
    "flink_jobs": ["java/flink-jobs"],
    "web_ui": ["web/ui"],
    "proto": ["proto/traffic/v1"],
    "mlops": ["mlops", "deployments/kubernetes/argo-events"],
    "deploy": ["deployments/kubernetes", "common"],
    "acceptance": ["doc/02_acceptance", "doc/05_status", "doc/03_review"],
}

KEY_FILES = {
    "web_routes": "web/ui/src/App.tsx",
    "web_layout": "web/ui/src/components/Layout/MainLayout.tsx",
    "apisix_routes": "deployments/kubernetes/configmaps/apisix-routes.yaml",
    "kafka_topics": "common/kafka/create-topics.sh",
    "proto_buf": "proto/buf.yaml",
    "proto_gen": "proto/buf.gen.yaml",
    "test_runner": "tests/run_tests.sh",
    "makefile": "Makefile",
}


def git_status_items() -> list[dict[str, str]]:
    items: list[dict[str, str]] = []
    for line in run_git(["status", "--short"]).splitlines():
        if not line:
            continue
        status = line[:2]
        path = line[3:] if len(line) > 3 else ""
        items.append({"status": status.strip() or status, "path": path})
    return items


def file_fingerprint(path: str) -> dict[str, Any]:
    target = repo_path(path)
    if not target.exists():
        return {"path": path, "exists": False}
    stat = target.stat()
    return {
        "path": path,
        "exists": True,
        "bytes": stat.st_size,
        "sha256": sha256_file(target),
    }


def doc_fingerprints() -> list[dict[str, Any]]:
    return [file_fingerprint(path) for path in DOC_INPUTS]


def list_files(pattern: str, limit: int = 500) -> list[str]:
    return sorted(rel_path(path) for path in REPO_ROOT.glob(pattern) if path.is_file())[:limit]


def extract_routes() -> dict[str, Any]:
    path = repo_path("web/ui/src/App.tsx")
    if not path.exists():
        return {"exists": False, "routes": []}
    routes: list[dict[str, Any]] = []
    protected_depth = 0
    for lineno, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if '<Route path="/" element={<ProtectedLayout />}' in line:
            protected_depth = 1
        elif protected_depth and "</Route>" in line:
            protected_depth = 0
        match = re.search(r'<Route\s+path="([^"]+)"', line)
        if match:
            raw = match.group(1)
            route = raw if raw.startswith("/") else f"/{raw}"
            routes.append(
                {
                    "path": route,
                    "line": lineno,
                    "protected_shell": bool(protected_depth and raw != "/"),
                    "note": "screen outside ProtectedLayout" if route == "/screen" and not protected_depth else "",
                }
            )
    return {"exists": True, "source": rel_path(path), "routes": routes}


def extract_proto_symbols() -> dict[str, Any]:
    symbols: dict[str, list[dict[str, Any]]] = {"message": [], "enum": [], "service": []}
    for path in sorted(repo_path("proto/traffic/v1").glob("*.proto")):
        text = path.read_text(encoding="utf-8")
        for lineno, line in enumerate(text.splitlines(), start=1):
            match = re.match(r"\s*(message|enum|service)\s+([A-Za-z0-9_]+)", line)
            if match:
                symbols[match.group(1)].append(
                    {"name": match.group(2), "file": rel_path(path), "line": lineno}
                )
    return {"files": list_files("proto/traffic/v1/*.proto"), "symbols": symbols}


def extract_topics() -> dict[str, Any]:
    paths = ["common/kafka/create-topics.sh", "deployments/kubernetes/init-jobs/01-kafka-topics.yaml"]
    topics: set[str] = set()
    topic_re = re.compile(r"\b[a-z][a-z0-9.-]*(?:\.v\d+|updates)\b")
    for path in paths:
        target = repo_path(path)
        if not target.exists():
            continue
        topics.update(topic_re.findall(target.read_text(encoding="utf-8")))
    return {"sources": [path for path in paths if repo_path(path).exists()], "topics": sorted(topics)}


def extract_schema_files() -> dict[str, Any]:
    files = list_files("common/sql/**/*.sql") + list_files("deployments/kubernetes/init-jobs/*.yaml")
    return {"files": files}


def load_tasks(tasks_dir: Path) -> list[dict[str, Any]]:
    tasks: list[dict[str, Any]] = []
    if not tasks_dir.exists():
        return tasks
    for path in sorted(tasks_dir.glob("*.yaml")):
        data = load_yaml_subset(path)
        data["_path"] = rel_path(path)
        tasks.append(data)
    return tasks


def build_gap_index(tasks: list[dict[str, Any]]) -> dict[str, Any]:
    by_priority = Counter(str(task.get("priority")) for task in tasks)
    by_status = Counter(str(task.get("status")) for task in tasks)
    by_lane = Counter(str((task.get("lane") or {}).get("primary")) for task in tasks)
    high_risk = [
        {
            "id": task.get("id"),
            "title": task.get("title"),
            "lane": (task.get("lane") or {}).get("primary"),
            "risk": task.get("risk"),
            "path": task.get("_path"),
        }
        for task in tasks
        if (task.get("risk") or {}).get("level") == "high"
    ]
    blockers = [
        {
            "id": task.get("id"),
            "priority": task.get("priority"),
            "title": task.get("title"),
            "acceptance_type": task.get("acceptance_type"),
            "mode": (task.get("execution") or {}).get("mode"),
            "path": task.get("_path"),
        }
        for task in tasks
        if task.get("priority") == "P0"
    ]
    return {
        "summary": {
            "total": len(tasks),
            "by_priority": dict(by_priority),
            "by_status": dict(by_status),
            "by_primary_lane": dict(by_lane),
        },
        "p0_blockers": blockers,
        "high_risk_tasks": high_risk,
    }


def build_dependency_map(tasks: list[dict[str, Any]]) -> dict[str, Any]:
    subsystem_to_tasks: dict[str, list[str]] = defaultdict(list)
    contract_to_tasks: dict[str, list[str]] = defaultdict(list)
    lane_to_tasks: dict[str, list[str]] = defaultdict(list)
    for task in tasks:
        task_id = str(task.get("id"))
        for subsystem in list_of(task.get("subsystems")):
            subsystem_to_tasks[str(subsystem)].append(task_id)
        for key, value in (task.get("contracts") or {}).items():
            if value:
                contract_to_tasks[str(key)].append(task_id)
        lane = task.get("lane") or {}
        lane_to_tasks[str(lane.get("primary"))].append(task_id)
        for dependent in list_of(lane.get("dependent")):
            lane_to_tasks[str(dependent)].append(task_id)
    return {
        "subsystems": SUBSYSTEMS,
        "subsystem_to_tasks": {key: sorted(value) for key, value in subsystem_to_tasks.items()},
        "contract_to_tasks": {key: sorted(value) for key, value in contract_to_tasks.items()},
        "lane_to_tasks": {key: sorted(value) for key, value in lane_to_tasks.items()},
        "routes": extract_routes(),
        "proto": extract_proto_symbols(),
        "kafka": extract_topics(),
        "schemas": extract_schema_files(),
        "key_files": {name: file_fingerprint(path) for name, path in KEY_FILES.items()},
    }


def evidence_required_missing(run_dir: Path, summary: dict[str, Any]) -> list[str]:
    if summary.get("run_kind") == "context_scout":
        required = [
            "run-summary.json",
            "context/context.snapshot.json",
            "context/gap-index.json",
            "context/dependency-map.json",
            "context/evidence-ledger.json",
            "context/god-view.md",
        ]
    elif summary.get("run_kind") == "design_package":
        required = [
            "run-summary.json",
            "task.yaml",
            "design/design-summary.json",
            "design/product-iteration.md",
            "design/feature-spec.md",
            "design/visual-correction.md",
            "design/architecture-evolution.md",
            "design/acceptance-cases.md",
            "design/implementation-plan.md",
        ]
    elif summary.get("run_kind") == "context_pack":
        required = [
            "run-summary.json",
            "task.yaml",
            "context-pack/task-context-pack.md",
            "context-pack/task-context-pack.json",
            "context-pack/context-budget.json",
            "context-pack/decision-log.jsonl",
            "context-pack/handoff.md",
        ]
    elif summary.get("run_kind") == "guidance":
        required = [
            "run-summary.json",
            "guidance/guidance.json",
            "guidance/guidance-report.md",
        ]
    elif summary.get("run_kind") == "workflow_run":
        required = [
            "run-summary.json",
            "task.yaml",
            "context/context.snapshot.json",
            "guidance/guidance.json",
            "design/design-summary.json",
            "context-pack/task-context-pack.md",
            "plan.md",
            "review-report.md",
            "workflow/workflow-summary.json",
            "workflow/workflow-report.md",
        ]
        if "implementation/implementation-brief.md" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "implementation/implementation-brief.md",
                    "implementation/patch-scope.json",
                    "implementation/patch-validation.json",
                    "implementation/apply-report.md",
                ]
            )
        if "patch-runner/patch-runner-summary.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "patch-runner/patch-request.md",
                    "patch-runner/patch-request.json",
                    "patch-runner/codex-output-contract.json",
                    "patch-runner/codex-output-schema.json",
                    "patch-runner/patch-intake.json",
                    "patch-runner/patch-runner-summary.json",
                ]
            )
        if "model-profile/model-profile.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "model-profile/model-profile.json",
                    "model-profile/model-profile.md",
                    "model-profile/command-template.txt",
                ]
            )
        if "review/review-summary.json" in list_of(summary.get("outputs")):
            required.append("review/review-summary.json")
        if "codex-adapter/invocation.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "codex-adapter/invocation-plan.md",
                    "codex-adapter/invocation.json",
                    "codex-adapter/stdout.txt",
                    "codex-adapter/stderr.txt",
                ]
            )
        if "codex-runner/invocation.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "codex-runner/invocation.json",
                    "codex-runner/codex-runner-report.md",
                    "codex-runner/stdout.txt",
                    "codex-runner/stderr.txt",
                ]
            )
        if "semantic-review/semantic-review.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "semantic-review/semantic-review.json",
                    "semantic-review/semantic-review-report.md",
                ]
            )
        if "llm-review/llm-review-summary.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "llm-review/llm-review-request.md",
                    "llm-review/llm-review-schema.json",
                    "llm-review/llm-review-profile.json",
                    "llm-review/command-template.txt",
                    "llm-review/llm-review-summary.json",
                    "llm-review/llm-review-report.md",
                ]
            )
        if "evidence-check/evidence-check.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "evidence-check/evidence-check.json",
                    "evidence-check/evidence-check-report.md",
                ]
            )
        if "repair/repair-plan.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "repair/repair-plan.json",
                    "repair/repair-report.md",
                    "repair/codex-repair-prompt.md",
                ]
            )
        if "auto-repair/auto-repair-summary.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "auto-repair/auto-repair-summary.json",
                    "auto-repair/auto-repair-report.md",
                ]
            )
    elif summary.get("run_kind") == "task_state":
        required = [
            "run-summary.json",
            "task-state/task-state.json",
            "task-state/task-board.md",
            "task-state/transition-plan.json",
            "task-state/apply-report.md",
            "task-state/transition-log.jsonl",
        ]
    elif summary.get("run_kind") == "implementation_guard":
        required = [
            "run-summary.json",
            "task.yaml",
            "implementation/implementation-brief.md",
            "implementation/codex-implementation-prompt.md",
            "implementation/patch-scope.json",
            "implementation/patch-validation.json",
            "implementation/apply-report.md",
        ]
    elif summary.get("run_kind") == "patch_runner":
        required = [
            "run-summary.json",
            "task.yaml",
            "patch-runner/patch-request.md",
            "patch-runner/patch-request.json",
            "patch-runner/codex-output-contract.json",
            "patch-runner/codex-output-schema.json",
            "patch-runner/patch-intake.json",
            "patch-runner/patch-runner-summary.json",
        ]
    elif summary.get("run_kind") == "model_profile":
        required = [
            "run-summary.json",
            "task.yaml",
            "model-profile/model-profile.json",
            "model-profile/model-profile.md",
            "model-profile/command-template.txt",
        ]
    elif summary.get("run_kind") == "scheduler":
        required = [
            "run-summary.json",
            "scheduler/scheduler-plan.json",
            "scheduler/queue.md",
        ]
    elif summary.get("run_kind") == "worker":
        required = [
            "run-summary.json",
            "worker/worker-summary.json",
            "worker/worker-report.md",
        ]
    elif summary.get("run_kind") == "daemon":
        required = [
            "run-summary.json",
            "daemon/daemon-summary.json",
            "daemon/daemon-report.md",
        ]
    elif summary.get("run_kind") == "metrics":
        required = [
            "run-summary.json",
            "metrics/loop-metrics.json",
            "metrics/loop-metrics.md",
        ]
    elif summary.get("run_kind") == "service":
        required = [
            "run-summary.json",
            "service/service-summary.json",
            "service/service-report.md",
        ]
        if "preflight/preflight.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "preflight/preflight.json",
                    "preflight/preflight.md",
                ]
            )
    elif summary.get("run_kind") == "service_health":
        required = [
            "run-summary.json",
            "service/health.json",
            "service/health.md",
        ]
    elif summary.get("run_kind") == "service_recover":
        required = [
            "run-summary.json",
            "service/recover.json",
        ]
    elif summary.get("run_kind") == "deploy_plan":
        required = [
            "run-summary.json",
            "deploy/deploy-plan.json",
            "deploy/deploy-report.md",
        ]
        outputs = set(list_of(summary.get("outputs")))
        if "deploy/codex-loop-pv.yaml" in outputs:
            required.append("deploy/codex-loop-pv.yaml")
        if "deploy/codex-loop.service" in outputs:
            required.append("deploy/codex-loop.service")
        if "deploy/codex-loop-cronjob.yaml" in outputs:
            required.extend(["deploy/codex-loop-cronjob.yaml", "deploy/kustomization.yaml"])
        if "deploy/codex-loop-queue-service-deployment.yaml" in outputs:
            required.extend(["deploy/codex-loop-queue-service-deployment.yaml", "deploy/codex-loop-queue-service.yaml", "deploy/kustomization.yaml"])
        if "deploy/validation.json" in outputs:
            required.append("deploy/validation.json")
        if "deploy/kubectl-dry-run.txt" in outputs:
            required.append("deploy/kubectl-dry-run.txt")
        if "deploy/systemd-verify.txt" in outputs:
            required.append("deploy/systemd-verify.txt")
    elif summary.get("run_kind") == "k8s_bootstrap":
        required = [
            "run-summary.json",
            "k8s-bootstrap/bootstrap-summary.json",
            "k8s-bootstrap/bootstrap-report.md",
            "k8s-bootstrap/command-template.txt",
        ]
        outputs = set(list_of(summary.get("outputs")))
        if "k8s-bootstrap/kubectl-dry-run.txt" in outputs:
            required.append("k8s-bootstrap/kubectl-dry-run.txt")
        if "k8s-bootstrap/secret-dry-run.txt" in outputs:
            required.append("k8s-bootstrap/secret-dry-run.txt")
    elif summary.get("run_kind") == "image_build":
        required = [
            "run-summary.json",
            "image-build/build-summary.json",
            "image-build/build-report.md",
            "image-build/command-template.txt",
        ]
        if "image-build/docker-build.txt" in list_of(summary.get("outputs")):
            required.append("image-build/docker-build.txt")
    elif summary.get("run_kind") == "image_distribution":
        required = [
            "run-summary.json",
            "image-distribution/distribution-summary.json",
            "image-distribution/distribution-report.md",
        ]
    elif summary.get("run_kind") == "release_freeze":
        required = [
            "run-summary.json",
            "release/release-manifest.json",
            "release/release-manifest.md",
            "release/rollback-plan.md",
            "release/git-status.txt",
            "release/loop-diff.patch",
        ]
    elif summary.get("run_kind") == "objective_stop":
        required = [
            "run-summary.json",
            "objective-stop/stop-summary.json",
            "objective-stop/stop-report.md",
            "objective-stop/stop-policy.json",
        ]
    elif summary.get("run_kind") == "historical_scaffold":
        required = ["run-summary.json"]
    elif summary.get("run_kind") == "runtime_preflight":
        required = [
            "run-summary.json",
            "preflight/preflight.json",
            "preflight/preflight.md",
        ]
    elif summary.get("run_kind") == "resource_quota":
        required = [
            "run-summary.json",
            "resource-quota/resource-quota.json",
            "resource-quota/resource-quota.md",
        ]
    elif summary.get("run_kind") == "resource_monitor":
        required = [
            "run-summary.json",
            "resource-monitor/resource-monitor.json",
            "resource-monitor/resource-monitor.md",
        ]
    elif summary.get("run_kind") == "workspace_isolation":
        required = [
            "run-summary.json",
            "workspace-isolation/isolation-plan.json",
            "workspace-isolation/isolation-report.md",
        ]
    elif summary.get("run_kind") == "workspace_cleanup":
        required = [
            "run-summary.json",
            "workspace-cleanup/cleanup-plan.json",
            "workspace-cleanup/cleanup-report.md",
        ]
    elif summary.get("run_kind") == "executor_pool":
        required = [
            "run-summary.json",
            "executor-pool/executor-pool-summary.json",
            "executor-pool/executor-pool-report.md",
        ]
        if "executor-pool/workspace-isolation.json" in list_of(summary.get("outputs")):
            required.extend(
                [
                    "executor-pool/workspace-isolation.json",
                    "executor-pool/workspace-isolation.md",
                ]
            )
    elif summary.get("run_kind") == "executor_pool_stress":
        required = [
            "run-summary.json",
            "executor-pool-stress/stress-summary.json",
            "executor-pool-stress/stress-report.md",
        ]
    elif summary.get("run_kind") == "remote_pool_stress":
        required = [
            "run-summary.json",
            "remote-pool-stress/stress-summary.json",
            "remote-pool-stress/stress-report.md",
            "remote-pool-stress/worker-results.json",
            "remote-pool-stress/http-responses.json",
        ]
    elif summary.get("run_kind") == "remote_pool_worker":
        required = [
            "run-summary.json",
            "remote-pool-stress/stress-summary.json",
            "remote-pool-stress/stress-report.md",
            "remote-pool-stress/worker-results.json",
            "remote-pool-stress/http-responses.json",
        ]
    elif summary.get("run_kind") == "remote_pool_k8s_stress":
        required = [
            "run-summary.json",
            "remote-pool-k8s-stress/stress-summary.json",
            "remote-pool-k8s-stress/stress-report.md",
            "remote-pool-k8s-stress/seed-plan.json",
            "remote-pool-k8s-stress/remote-pool-worker-job.yaml",
            "remote-pool-k8s-stress/command-template.txt",
        ]
        if "remote-pool-k8s-stress/kubectl-dry-run.txt" in list_of(summary.get("outputs")):
            required.append("remote-pool-k8s-stress/kubectl-dry-run.txt")
    elif summary.get("run_kind") == "remote_pool_k8s_readiness":
        required = [
            "run-summary.json",
            "remote-pool-k8s-readiness/readiness-summary.json",
            "remote-pool-k8s-readiness/readiness-report.md",
            "remote-pool-k8s-readiness/kubectl-checks.json",
        ]
        if "remote-pool-k8s-readiness/kubectl-dry-run.txt" in list_of(summary.get("outputs")):
            required.append("remote-pool-k8s-readiness/kubectl-dry-run.txt")
    elif summary.get("run_kind") == "soak":
        required = [
            "run-summary.json",
            "soak/soak-summary.json",
            "soak/soak-report.md",
        ]
    elif summary.get("run_kind") == "queue_service":
        required = [
            "run-summary.json",
            "queue-service/queue-service-summary.json",
            "queue-service/queue-service-report.md",
            "queue-service/smoke-responses.json",
        ]
    elif summary.get("run_kind") == "sandbox_plan":
        required = [
            "run-summary.json",
            "task.yaml",
            "sandbox/sandbox-plan.json",
            "sandbox/sandbox-report.md",
            "sandbox/codex-loop-sandbox-job.yaml",
            "sandbox/codex-loop-sandbox-networkpolicy.yaml",
            "sandbox/local-container-command.txt",
        ]
        outputs = set(list_of(summary.get("outputs")))
        if "sandbox/validation.json" in outputs:
            required.append("sandbox/validation.json")
        if "sandbox/kubectl-dry-run.txt" in outputs:
            required.append("sandbox/kubectl-dry-run.txt")
    elif summary.get("run_kind") == "sandbox_execution":
        required = [
            "run-summary.json",
            "sandbox-executor/execution.json",
            "sandbox-executor/execution-report.md",
        ]
        for item in list_of(summary.get("outputs")):
            if item.startswith("sandbox-executor/") and item not in required:
                required.append(str(item))
    elif summary.get("run_kind") == "sandbox_worker":
        required = [
            "run-summary.json",
            "sandbox-worker/sandbox-worker-summary.json",
            "sandbox-worker/sandbox-worker-report.md",
        ]
    elif summary.get("run_kind") == "codex_adapter":
        required = [
            "run-summary.json",
            "task.yaml",
            "codex-adapter/invocation-plan.md",
            "codex-adapter/invocation.json",
            "codex-adapter/stdout.txt",
            "codex-adapter/stderr.txt",
        ]
    elif summary.get("run_kind") == "codex_runner":
        required = [
            "run-summary.json",
            "task.yaml",
            "codex-runner/invocation.json",
            "codex-runner/codex-runner-report.md",
            "codex-runner/stdout.txt",
            "codex-runner/stderr.txt",
        ]
    elif summary.get("run_kind") == "semantic_review":
        required = [
            "run-summary.json",
            "task.yaml",
            "semantic-review/semantic-review.json",
            "semantic-review/semantic-review-report.md",
        ]
    elif summary.get("run_kind") == "llm_review":
        required = [
            "run-summary.json",
            "task.yaml",
            "llm-review/llm-review-request.md",
            "llm-review/llm-review-schema.json",
            "llm-review/llm-review-profile.json",
            "llm-review/command-template.txt",
            "llm-review/llm-review-summary.json",
            "llm-review/llm-review-report.md",
        ]
    elif summary.get("run_kind") == "auto_repair_loop":
        required = [
            "run-summary.json",
            "task.yaml",
            "auto-repair/auto-repair-summary.json",
            "auto-repair/auto-repair-report.md",
        ]
    else:
        required = ["run-summary.json", "task.yaml", "plan.md", "review-report.md"]
    return [name for name in required if not (run_dir / name).exists()]


def build_evidence_ledger(exclude_run_id: str | None = None) -> dict[str, Any]:
    runs: list[dict[str, Any]] = []
    if RUNS_ROOT.exists():
        for run_dir in sorted(path for path in RUNS_ROOT.iterdir() if path.is_dir()):
            if run_dir.name.startswith("."):
                continue
            if exclude_run_id and run_dir.name == exclude_run_id:
                continue
            summary_path = run_dir / "run-summary.json"
            summary: dict[str, Any] = {}
            if summary_path.exists():
                try:
                    summary = json.loads(summary_path.read_text(encoding="utf-8"))
                except json.JSONDecodeError:
                    summary = {"parse_error": True}
            runs.append(
                {
                    "run_id": run_dir.name,
                    "path": rel_path(run_dir),
                    "run_kind": summary.get("run_kind", "task"),
                    "task_id": summary.get("task_id"),
                    "status": summary.get("status", "NO_SUMMARY"),
                    "created_at": summary.get("created_at"),
                    "evidence_type": summary.get("evidence_type"),
                    "missing_core_files": evidence_required_missing(run_dir, summary),
                }
            )
    by_status = Counter(run.get("status") for run in runs)
    by_evidence_type = Counter(run.get("evidence_type") or "unknown" for run in runs)
    return {
        "summary": {
            "total_runs": len(runs),
            "by_status": dict(by_status),
            "by_evidence_type": dict(by_evidence_type),
        },
        "runs": runs,
    }


def build_context_snapshot(tasks: list[dict[str, Any]]) -> dict[str, Any]:
    status_items = git_status_items()
    status_counts = Counter(item["status"] for item in status_items)
    return {
        "generated_at": datetime.now().isoformat(timespec="seconds"),
        "repo_root": str(REPO_ROOT),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "branch": run_git(["branch", "--show-current"]).strip(),
        "git_status": {
            "total": len(status_items),
            "by_status": dict(status_counts),
            "items": status_items[:500],
            "truncated": len(status_items) > 500,
        },
        "documents": doc_fingerprints(),
        "subsystems": SUBSYSTEMS,
        "task_pool": {
            "count": len(tasks),
            "task_ids": [task.get("id") for task in tasks],
        },
        "runtime_probe": {
            "kubectl_available": bool(run_command(["bash", "-lc", "command -v kubectl"]).strip()),
            "node_available": bool(run_command(["bash", "-lc", "command -v node"]).strip()),
            "python": run_command(["python", "--version"]).strip(),
        },
    }


def render_god_view(
    context: dict[str, Any],
    gaps: dict[str, Any],
    deps: dict[str, Any],
    evidence: dict[str, Any],
) -> str:
    p0_count = gaps["summary"]["by_priority"].get("P0", 0)
    high_count = len(gaps["high_risk_tasks"])
    unprotected_screen = [
        route for route in deps["routes"].get("routes", []) if route.get("path") == "/screen" and route.get("note")
    ]
    lines = [
        "# Codex Loop God View",
        "",
        f"- generated_at: `{context['generated_at']}`",
        f"- commit: `{context['commit']}`",
        f"- branch: `{context['branch'] or '(detached or unknown)'}`",
        f"- dirty_items_seen: `{context['git_status']['total']}`",
        f"- task_pool: `{gaps['summary']['total']}` tasks, `{p0_count}` P0",
        f"- high_risk_tasks: `{high_count}`",
        f"- evidence_runs: `{evidence['summary']['total_runs']}`",
        "",
        "## Immediate Signals",
    ]
    if unprotected_screen:
        lines.append("- `/screen` is detected outside `ProtectedLayout`; keep `CLE-P0-SCREEN-001` high priority.")
    if deps["contract_to_tasks"]:
        for contract, task_ids in sorted(deps["contract_to_tasks"].items()):
            lines.append(f"- `{contract}` changes affect: {', '.join(task_ids)}")
    else:
        lines.append("- No contract-impacting task was detected.")
    lines.extend(["", "## Recommended Next Tasks"])
    for task in gaps["p0_blockers"][:5]:
        lines.append(f"- `{task['id']}` ({task['acceptance_type']}): {task['title']}")
    lines.extend(
        [
            "",
            "## Evidence Discipline",
            "- Current scout output is a regression-level context snapshot, not acceptance or third-party evidence.",
            "- Tasks can close only after their own `close_when` and Reviewer Gate pass.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--out-dir", default=None)
    args = parser.parse_args()

    run_id = args.run_id or f"scout-{datetime.now().strftime('%Y%m%d-%H%M%S')}"
    run_dir = ensure_run_dir(run_id)
    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "context"
    out_dir.mkdir(parents=True, exist_ok=True)

    tasks = load_tasks(repo_path(args.tasks_dir))
    context = build_context_snapshot(tasks)
    gaps = build_gap_index(tasks)
    deps = build_dependency_map(tasks)
    write_json(out_dir / "context.snapshot.json", context)
    write_json(out_dir / "gap-index.json", gaps)
    write_json(out_dir / "dependency-map.json", deps)
    write_json(
        run_dir / "run-summary.json",
        {
            "run_id": run_id,
            "run_kind": "context_scout",
            "status": "CONTEXT_SCOUTED",
            "evidence_type": "regression",
            "created_at": context["generated_at"],
            "commit": context["commit"],
            "outputs": [
                "context/context.snapshot.json",
                "context/gap-index.json",
                "context/dependency-map.json",
                "context/evidence-ledger.json",
                "context/god-view.md",
            ],
            "warning": "Context Scout output is a global engineering snapshot, not acceptance or third-party evidence.",
        },
    )
    evidence = build_evidence_ledger(exclude_run_id=run_id)
    write_json(out_dir / "evidence-ledger.json", evidence)
    write_text(out_dir / "god-view.md", render_god_view(context, gaps, deps, evidence))

    print(out_dir)
    print(f"tasks={gaps['summary']['total']} p0={gaps['summary']['by_priority'].get('P0', 0)}")
    print(f"evidence_runs={evidence['summary']['total_runs']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

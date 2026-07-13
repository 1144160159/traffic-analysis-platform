#!/usr/bin/env python3
"""Orchestrate God View guidance into one guarded task workflow."""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
    REPO_ROOT,
    SCRIPT_ROOT,
    copy_task_snapshot,
    ensure_run_dir,
    list_of,
    load_yaml_subset,
    make_run_id,
    rel_path,
    repo_path,
    run_git,
    write_json,
    write_text,
)


WORKFLOW_OUTPUTS = [
    "workflow/workflow-summary.json",
    "workflow/workflow-report.md",
    "workflow/gate-decision.md",
    "patch-runner/patch-request.md",
    "patch-runner/patch-request.json",
    "patch-runner/codex-output-contract.json",
    "patch-runner/codex-output-schema.json",
    "patch-runner/patch-intake.json",
    "patch-runner/patch-runner-summary.json",
    "model-profile/model-profile.json",
    "model-profile/model-profile.md",
    "model-profile/command-template.txt",
    "codex-adapter/invocation-plan.md",
    "codex-adapter/invocation.json",
    "codex-runner/invocation.json",
    "codex-runner/codex-runner-report.md",
    "review/review-summary.json",
    "semantic-review/semantic-review.json",
    "semantic-review/semantic-review-report.md",
    "llm-review/llm-review-request.md",
    "llm-review/llm-review-schema.json",
    "llm-review/llm-review-profile.json",
    "llm-review/command-template.txt",
    "llm-review/llm-review-summary.json",
    "llm-review/llm-review-report.md",
    "evidence-check/evidence-check.json",
    "evidence-check/evidence-check-report.md",
    "repair/repair-plan.json",
    "repair/repair-report.md",
    "repair/codex-repair-prompt.md",
    "auto-repair/auto-repair-summary.json",
    "auto-repair/auto-repair-report.md",
]


@dataclass
class StepResult:
    name: str
    command: list[str]
    exit_code: int
    skipped: bool = False
    reason: str = ""
    output_tail: str = ""

    def as_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "command": self.command,
            "exit_code": self.exit_code,
            "skipped": self.skipped,
            "reason": self.reason,
            "output_tail": self.output_tail,
        }


class WorkflowError(RuntimeError):
    def __init__(self, step: StepResult) -> None:
        super().__init__(f"Workflow step failed: {step.name}")
        self.step = step


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def run_script(name: str, args: list[str], steps: list[StepResult], critical: bool = True) -> StepResult:
    script = SCRIPT_ROOT / name
    command = [sys.executable, "-B", str(script), *args]
    proc = subprocess.run(
        command,
        cwd=REPO_ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    result = StepResult(
        name=name,
        command=[str(item) for item in command],
        exit_code=proc.returncode,
        output_tail=proc.stdout[-4000:],
    )
    steps.append(result)
    if critical and proc.returncode != 0:
        raise WorkflowError(result)
    return result


def skipped_step(name: str, reason: str, steps: list[StepResult]) -> StepResult:
    result = StepResult(name=name, command=[], exit_code=0, skipped=True, reason=reason)
    steps.append(result)
    return result


def resolve_task_path(task_arg: str, tasks_dir: Path) -> Path:
    direct = repo_path(task_arg)
    if direct.exists():
        return direct
    by_id = tasks_dir / f"{task_arg}.yaml"
    if by_id.exists():
        return by_id
    raise FileNotFoundError(f"Task not found as path or task id: {task_arg}")


def select_task_from_guidance(guidance: dict[str, Any], tasks_dir: Path) -> Path:
    for item in guidance.get("recommended_next", []):
        task_id = str(item.get("id"))
        if not task_id:
            continue
        path = tasks_dir / f"{task_id}.yaml"
        if path.exists():
            return path
    raise ValueError("guidance.recommended_next did not contain a task with an existing YAML file")


def task_blockers(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    current = str(task.get("id"))
    return [
        item
        for item in guidance.get("findings", [])
        if str(item.get("target")) == current and item.get("level") == "blocker"
    ]


def copy_if_different(source: Path, target: Path) -> None:
    if not source.exists():
        return
    source_resolved = source.resolve()
    target_resolved = target.resolve() if target.exists() else target
    if source_resolved == target_resolved:
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)


def mirror_inputs(run_dir: Path, context_dir: Path, guidance_path: Path) -> None:
    for filename in [
        "context.snapshot.json",
        "gap-index.json",
        "dependency-map.json",
        "evidence-ledger.json",
        "god-view.md",
    ]:
        copy_if_different(context_dir / filename, run_dir / "context" / filename)
    copy_if_different(guidance_path, run_dir / "guidance" / "guidance.json")
    guidance_report = guidance_path.parent / "guidance-report.md"
    copy_if_different(guidance_report, run_dir / "guidance" / "guidance-report.md")


def write_gate_decision(run_dir: Path, blockers: list[dict[str, Any]], stage: str) -> Path:
    lines = [
        "# Workflow Gate Decision",
        "",
        f"- stage: `{stage}`",
        "- decision: `STOP_BEFORE_EXECUTION`",
        "- reason: blocker-class findings are present for the selected task.",
        "",
        "## Blockers",
    ]
    for item in blockers:
        lines.append(f"- `{item.get('code')}`: {item.get('message')} Suggestion: {item.get('suggestion')}")
    lines.extend(
        [
            "",
            "## Rule",
            "- Design, context packing, planning and review scaffolds may be generated.",
            "- Local execution, live validation and task closure must wait until blockers are resolved or a human gate explicitly overrides.",
            "",
        ]
    )
    return write_text(run_dir / "workflow" / "gate-decision.md", "\n".join(lines))


def final_status(stage: str, blockers: list[dict[str, Any]], failed: StepResult | None) -> str:
    if failed:
        return "WORKFLOW_FAILED"
    if blockers:
        return "DESIGN_ITERATING"
    if stage == "prepare":
        return "WORKFLOW_PREPARED"
    if stage == "dry-run":
        return "WORKFLOW_DRY_RUN"
    if stage == "execute-local":
        return "LOCAL_VERIFIED"
    return "WORKFLOW_PREPARED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Workflow Report",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- selected_task: `{summary['task_id']}` {summary['task_title']}",
        f"- selected_from: `{summary['selected_from']}`",
        f"- stage: `{summary['stage']}`",
        f"- status: `{summary['status']}`",
        f"- evidence_type: `{summary['evidence_type']}`",
        f"- blocker_count: `{len(summary['blockers'])}`",
        "",
        "## Stage Flow",
    ]
    for step in summary["steps"]:
        if step["skipped"]:
            lines.append(f"- `{step['name']}` skipped: {step['reason']}")
        else:
            lines.append(f"- `{step['name']}` exit `{step['exit_code']}`")
    lines.extend(["", "## Blockers"])
    if summary["blockers"]:
        for item in summary["blockers"]:
            lines.append(f"- `{item.get('code')}`: {item.get('message')} Suggestion: {item.get('suggestion')}")
    else:
        lines.append("- none")
    lines.extend(["", "## Outputs"])
    for item in summary["outputs"]:
        lines.append(f"- `{item}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This workflow report does not close a task by itself.",
            "- `execute-local` is required before local verification commands are run.",
            "- Live write and destructive actions remain out of scope for this workflow MVP.",
            "",
        ]
    )
    return "\n".join(lines)


def build_summary(
    run_id: str,
    run_dir: Path,
    task: dict[str, Any],
    selected_from: str,
    stage: str,
    context_dir: Path,
    guidance_path: Path,
    blockers: list[dict[str, Any]],
    steps: list[StepResult],
    failed: StepResult | None,
) -> dict[str, Any]:
    status = final_status(stage, blockers, failed)
    evidence_type = "regression" if stage == "execute-local" and not failed and not blockers else "acceptance-prep"
    outputs = [
        "task.yaml",
        "context/context.snapshot.json",
        "guidance/guidance.json",
        "design/design-summary.json",
        "context-pack/task-context-pack.md",
        "implementation/implementation-brief.md",
        "implementation/patch-scope.json",
        "implementation/patch-validation.json",
        "patch-runner/patch-request.md",
        "patch-runner/patch-request.json",
        "patch-runner/codex-output-contract.json",
        "patch-runner/codex-output-schema.json",
        "patch-runner/patch-intake.json",
        "patch-runner/patch-runner-summary.json",
        "model-profile/model-profile.json",
        "model-profile/model-profile.md",
        "model-profile/command-template.txt",
        "codex-adapter/invocation-plan.md",
        "codex-adapter/invocation.json",
        "codex-runner/invocation.json",
        "codex-runner/codex-runner-report.md",
        "plan.md",
        "review-report.md",
        "review/review-summary.json",
        "semantic-review/semantic-review.json",
        "design-delta.md",
        "llm-review/llm-review-summary.json",
        "git-status.txt",
        "changed-files.txt",
        *WORKFLOW_OUTPUTS[:2],
    ]
    if (run_dir / "workflow" / "gate-decision.md").exists():
        outputs.append("workflow/gate-decision.md")
    if (run_dir / "patch-runner" / "patch-runner-summary.json").exists():
        outputs.extend(
            [
                "patch-runner/patch-request.md",
                "patch-runner/patch-request.json",
                "patch-runner/codex-output-contract.json",
                "patch-runner/codex-output-schema.json",
                "patch-runner/patch-intake.json",
                "patch-runner/patch-runner-summary.json",
                "patch-runner/patch-runner-report.md",
            ]
        )
    if (run_dir / "model-profile" / "model-profile.json").exists():
        outputs.extend(
            [
                "model-profile/model-profile.json",
                "model-profile/model-profile.md",
                "model-profile/command-template.txt",
            ]
        )
    if (run_dir / "review" / "review-summary.json").exists():
        outputs.append("review/review-summary.json")
    if (run_dir / "codex-adapter" / "invocation.json").exists():
        outputs.extend(
            [
                "codex-adapter/invocation-plan.md",
                "codex-adapter/invocation.json",
                "codex-adapter/stdout.txt",
                "codex-adapter/stderr.txt",
            ]
        )
    if (run_dir / "codex-runner" / "invocation.json").exists():
        outputs.extend(
            [
                "codex-runner/invocation.json",
                "codex-runner/codex-runner-report.md",
                "codex-runner/stdout.txt",
                "codex-runner/stderr.txt",
            ]
        )
    if (run_dir / "semantic-review" / "semantic-review.json").exists():
        outputs.extend(
            [
                "semantic-review/semantic-review.json",
                "semantic-review/semantic-review-report.md",
            ]
        )
    if (run_dir / "llm-review" / "llm-review-summary.json").exists():
        outputs.extend(
            [
                "llm-review/llm-review-request.md",
                "llm-review/llm-review-schema.json",
                "llm-review/llm-review-profile.json",
                "llm-review/command-template.txt",
                "llm-review/llm-review-summary.json",
                "llm-review/llm-review-report.md",
            ]
        )
    if (run_dir / "evidence-check" / "evidence-check.json").exists():
        outputs.extend(
            [
                "evidence-check/evidence-check.json",
                "evidence-check/evidence-check-report.md",
            ]
        )
    if (run_dir / "repair" / "repair-plan.json").exists():
        outputs.extend(
            [
                "repair/repair-plan.json",
                "repair/repair-report.md",
                "repair/codex-repair-prompt.md",
            ]
        )
    if (run_dir / "auto-repair" / "auto-repair-summary.json").exists():
        outputs.extend(
            [
                "auto-repair/auto-repair-summary.json",
                "auto-repair/auto-repair-report.md",
            ]
        )
    if (run_dir / "local-report.md").exists():
        outputs.append("local-report.md")
    return {
        "run_id": run_id,
        "run_kind": "workflow_run",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "selected_from": selected_from,
        "stage": stage,
        "status": status,
        "evidence_type": evidence_type,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "context_dir": rel_path(context_dir),
        "guidance_path": rel_path(guidance_path),
        "blockers": blockers,
        "steps": [step.as_dict() for step in steps],
        "failed_step": failed.as_dict() if failed else None,
        "outputs": outputs,
        "evidence_check": load_json(run_dir / "evidence-check" / "evidence-check.json")
        if (run_dir / "evidence-check" / "evidence-check.json").exists()
        else None,
        "repair_plan": load_json(run_dir / "repair" / "repair-plan.json")
        if (run_dir / "repair" / "repair-plan.json").exists()
        else None,
        "warning": "Workflow evidence is orchestration evidence. Task closure still requires close_when, verification, reviewer and evidence gates.",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", default=None, help="Task YAML path or task id. If omitted, use guidance.recommended_next.")
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--context-dir", default=None, help="Existing Context Scout directory. If omitted, scout.py is run.")
    parser.add_argument("--guidance", default=None, help="Existing guidance.json. If omitted, guide.py is run.")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--execute-local", action="store_true", help="Alias for --stage execute-local.")
    parser.add_argument("--max-chars", type=int, default=12000)
    parser.add_argument("--allow-blocker-execution", action="store_true")
    args = parser.parse_args()

    if args.execute_local:
        args.stage = "execute-local"

    run_id = args.run_id or make_run_id("workflow")
    run_dir = ensure_run_dir(run_id)
    workflow_dir = run_dir / "workflow"
    workflow_dir.mkdir(parents=True, exist_ok=True)
    tasks_dir = repo_path(args.tasks_dir)
    steps: list[StepResult] = []
    failed: StepResult | None = None

    try:
        if args.context_dir:
            context_dir = repo_path(args.context_dir)
            skipped_step("scout.py", f"using existing context dir {rel_path(context_dir)}", steps)
        else:
            run_script("scout.py", ["--run-id", run_id, "--tasks-dir", rel_path(tasks_dir)], steps)
            context_dir = run_dir / "context"

        if args.guidance:
            guidance_path = repo_path(args.guidance)
            skipped_step("guide.py", f"using existing guidance {rel_path(guidance_path)}", steps)
        else:
            run_script("guide.py", ["--context-dir", rel_path(context_dir), "--run-id", run_id, "--tasks-dir", rel_path(tasks_dir)], steps)
            guidance_path = run_dir / "guidance" / "guidance.json"

        mirror_inputs(run_dir, context_dir, guidance_path)
        guidance = load_json(guidance_path)
        if args.task:
            task_path = resolve_task_path(args.task, tasks_dir)
            selected_from = "explicit_task"
        else:
            task_path = select_task_from_guidance(guidance, tasks_dir)
            selected_from = "guidance_recommended_next"
        task = load_yaml_subset(task_path)
        copy_task_snapshot(task_path, run_dir)
        blockers = task_blockers(task, guidance)

        run_script(
            "design.py",
            [
                "--task",
                rel_path(task_path),
                "--context-dir",
                rel_path(context_dir),
                "--guidance",
                rel_path(guidance_path),
                "--run-id",
                run_id,
            ],
            steps,
        )
        run_script(
            "context_pack.py",
            [
                "--task",
                rel_path(task_path),
                "--context-dir",
                rel_path(context_dir),
                "--guidance",
                rel_path(guidance_path),
                "--design-dir",
                rel_path(run_dir / "design"),
                "--run-id",
                run_id,
                "--max-chars",
                str(args.max_chars),
            ],
            steps,
        )
        run_script("plan.py", ["--task", rel_path(task_path), "--run-id", run_id, "--write"], steps)
        run_script(
            "implement.py",
            [
                "--task",
                rel_path(task_path),
                "--context-pack",
                rel_path(run_dir / "context-pack" / "task-context-pack.md"),
                "--design-dir",
                rel_path(run_dir / "design"),
                "--plan",
                rel_path(run_dir / "plan.md"),
                "--guidance",
                rel_path(guidance_path),
                "--run-id",
                run_id,
            ],
            steps,
        )
        run_script(
            "patch_runner.py",
            [
                "--task",
                rel_path(task_path),
                "--context-pack",
                rel_path(run_dir / "context-pack" / "task-context-pack.md"),
                "--design-dir",
                rel_path(run_dir / "design"),
                "--guidance",
                rel_path(guidance_path),
                "--run-id",
                run_id,
            ],
            steps,
        )
        run_script(
            "model_profile.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--patch-request",
                rel_path(run_dir / "patch-runner" / "patch-request.md"),
                "--output-schema",
                rel_path(run_dir / "patch-runner" / "codex-output-schema.json"),
            ],
            steps,
        )
        run_script(
            "codex_adapter.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--patch-request",
                rel_path(run_dir / "patch-runner" / "patch-request.md"),
            ],
            steps,
        )
        run_script(
            "codex_runner.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--patch-request",
                rel_path(run_dir / "patch-runner" / "patch-request.md"),
                "--model-profile",
                rel_path(run_dir / "model-profile" / "model-profile.json"),
            ],
            steps,
        )
        run_script("review.py", ["--task", rel_path(task_path), "--run-id", run_id, "--guidance", rel_path(guidance_path)], steps)
        run_script(
            "semantic_reviewer.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--review-summary",
                rel_path(run_dir / "review" / "review-summary.json"),
            ],
            steps,
        )
        run_script(
            "llm_reviewer.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--review-summary",
                rel_path(run_dir / "review" / "review-summary.json"),
                "--semantic-review",
                rel_path(run_dir / "semantic-review" / "semantic-review.json"),
                "--context-pack",
                rel_path(run_dir / "context-pack" / "task-context-pack.md"),
                "--design-dir",
                rel_path(run_dir / "design"),
                "--patch-request",
                rel_path(run_dir / "patch-runner" / "patch-request.md"),
                "--patch-intake",
                rel_path(run_dir / "patch-runner" / "patch-intake.json"),
            ],
            steps,
        )

        should_run_task = args.stage in {"dry-run", "execute-local"}
        if blockers and should_run_task and not args.allow_blocker_execution:
            write_gate_decision(run_dir, blockers, args.stage)
            skipped_step("run_task.py", "blocked by guidance findings; gate-decision.md written", steps)
        elif should_run_task:
            mode = str((task.get("execution") or {}).get("mode"))
            run_args = ["--task", rel_path(task_path), "--mode", mode, "--run-id", run_id]
            if args.stage == "execute-local":
                run_args.append("--execute-local")
            run_script("run_task.py", run_args, steps, critical=args.stage == "execute-local")
        else:
            skipped_step("run_task.py", "stage prepare does not run task execution", steps)

        run_script(
            "collect_evidence.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--status",
                "WORKFLOW_EVIDENCE_COLLECTED",
                "--evidence-type",
                "acceptance-prep",
            ],
            steps,
        )
    except WorkflowError as exc:
        failed = exc.step
        if "task" not in locals():
            task = {"id": None, "title": None}
        if "task_path" in locals():
            copy_task_snapshot(task_path, run_dir)
        if "context_dir" not in locals():
            context_dir = run_dir / "context"
        if "guidance_path" not in locals():
            guidance_path = run_dir / "guidance" / "guidance.json"
        blockers = task_blockers(task, load_json(guidance_path)) if guidance_path.exists() else []
        selected_from = "failed_before_selection"

    summary = build_summary(
        run_id=run_id,
        run_dir=run_dir,
        task=task,
        selected_from=selected_from,
        stage=args.stage,
        context_dir=context_dir,
        guidance_path=guidance_path,
        blockers=blockers,
        steps=steps,
        failed=failed,
    )
    write_json(workflow_dir / "workflow-summary.json", summary)
    write_text(workflow_dir / "workflow-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)

    if "task_path" in locals():
        run_script(
            "evidence_check.py",
            [
                "--task",
                rel_path(task_path),
                "--run-id",
                run_id,
                "--guidance",
                rel_path(guidance_path),
            ],
            steps,
            critical=False,
        )
        evidence_check_path = run_dir / "evidence-check" / "evidence-check.json"
        if evidence_check_path.exists():
            evidence_check = load_json(evidence_check_path)
            if evidence_check.get("blocker_count", 0) > 0:
                run_script(
                    "repair.py",
                    [
                        "--task",
                        rel_path(task_path),
                        "--run-id",
                        run_id,
                        "--evidence-check",
                        rel_path(evidence_check_path),
                    ],
                    steps,
                    critical=False,
                )
                repair_plan_path = run_dir / "repair" / "repair-plan.json"
                if repair_plan_path.exists():
                    run_script(
                        "auto_repair_loop.py",
                        [
                            "--task",
                            rel_path(task_path),
                            "--repair-plan",
                            rel_path(repair_plan_path),
                            "--context-dir",
                            rel_path(context_dir),
                            "--guidance",
                            rel_path(guidance_path),
                            "--run-id",
                            run_id,
                        ],
                        steps,
                        critical=False,
                    )

    summary = build_summary(
        run_id=run_id,
        run_dir=run_dir,
        task=task,
        selected_from=selected_from,
        stage=args.stage,
        context_dir=context_dir,
        guidance_path=guidance_path,
        blockers=blockers,
        steps=steps,
        failed=failed,
    )
    write_json(workflow_dir / "workflow-summary.json", summary)
    write_text(workflow_dir / "workflow-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(workflow_dir)
    print(f"status={summary['status']} task={summary['task_id']} blockers={len(blockers)} stage={args.stage}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())

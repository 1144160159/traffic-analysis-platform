#!/usr/bin/env python3
"""Check whether a Codex Loop run has enough evidence to close a task.

The checker is deliberately conservative: missing, prep-only, pending-review,
or unproven close conditions keep the task out of CLOSED. It writes a machine
readable result plus a human report, but it does not modify task YAML or source
code.
"""

from __future__ import annotations

import argparse
import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
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


PREP_EVIDENCE_TYPES = {"acceptance-prep", "third-party-prep"}
PASS_REVIEW_DECISIONS = {"pass", "passed", "approved"}
BAD_STATUS_FRAGMENTS = {
    "FAILED",
    "BLOCKED",
    "INCOMPLETE",
    "PREP",
    "DESIGN_ITERATING",
    "IMPLEMENTATION_BLOCKED",
    "WORKFLOW_FAILED",
}
DEDICATED_REQUIRED_FILES = {
    "local-report.md",
    "review-report.md",
    "evidence-report.md",
}
RUNTIME_ERROR_PATTERNS = [
    r"\b5\d\d\b",
    r"\b4\d\d\b",
    r"requestfailed",
    r"pageerror",
    r"runtime exception",
    r"console\.error",
    r"\berror\b",
]
DESTRUCTIVE_SQL_PATTERNS = [
    r"\bDROP\s+DATABASE\b",
    r"\bDROP\s+TABLE\b",
    r"\bTRUNCATE\b",
    r"\bDELETE\s+FROM\b",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def result(level: str, code: str, message: str, evidence: str, suggestion: str) -> dict[str, str]:
    return {
        "level": level,
        "code": code,
        "message": message,
        "evidence": evidence,
        "suggestion": suggestion,
    }


def read_optional(path: Path) -> str:
    return path.read_text(encoding="utf-8") if path.exists() else ""


def extract_review_decision(text: str) -> str | None:
    for pattern in [
        r"decision:\s*`?([A-Za-z0-9_-]+)`?",
        r"^- decision:\s*`?([A-Za-z0-9_-]+)`?",
    ]:
        match = re.search(pattern, text, flags=re.MULTILINE | re.IGNORECASE)
        if match:
            return match.group(1).lower()
    return None


def local_report_failed(text: str) -> bool:
    for match in re.finditer(r"exit_code:\s*(-?\d+)", text):
        if int(match.group(1)) != 0:
            return True
    return False


def local_report_has_success(text: str) -> bool:
    matches = [int(match.group(1)) for match in re.finditer(r"exit_code:\s*(-?\d+)", text)]
    return bool(matches) and all(code == 0 for code in matches)


def line_is_negative_assertion(line: str) -> bool:
    lowered = line.strip().lower()
    if not lowered:
        return True
    return (
        lowered.startswith("- no ")
        or lowered.startswith("no ")
        or lowered.startswith("无")
        or lowered.startswith("- 无")
        or "0 failed" in lowered
        or "0 requestfailed" in lowered
        or "0 pageerror" in lowered
        or "none" == lowered
    )


def report_error_lines(text: str) -> list[str]:
    hits: list[str] = []
    for raw in text.splitlines():
        line = raw.strip()
        if line_is_negative_assertion(line):
            continue
        for pattern in RUNTIME_ERROR_PATTERNS:
            if re.search(pattern, line, flags=re.IGNORECASE):
                hits.append(line[:300])
                break
    return hits


def close_condition_covered(condition: str, report_text: str) -> bool:
    normalized_report = re.sub(r"\s+", " ", report_text.lower())
    normalized_condition = re.sub(r"\s+", " ", condition.lower()).strip()
    if normalized_condition and normalized_condition in normalized_report:
        return True
    tokens = [token for token in re.split(r"[^a-zA-Z0-9_/.-]+", normalized_condition) if len(token) >= 4]
    if not tokens:
        return False
    hits = sum(1 for token in tokens if token in normalized_report)
    return hits >= max(2, min(len(tokens), 4))


def guidance_blockers(task_id: str, guidance: dict[str, Any]) -> list[dict[str, Any]]:
    return [
        item
        for item in guidance.get("findings", [])
        if str(item.get("target")) == task_id and item.get("level") == "blocker"
    ]


def latest_summary(run_dir: Path) -> dict[str, Any]:
    summary = load_json(run_dir / "run-summary.json")
    workflow_summary = load_json(run_dir / "workflow" / "workflow-summary.json")
    if workflow_summary:
        return workflow_summary
    return summary


def check_required_files(task: dict[str, Any], run_dir: Path) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    required = list_of((task.get("evidence") or {}).get("required"))
    for name in required:
        if str(name) in DEDICATED_REQUIRED_FILES:
            continue
        if not (run_dir / str(name)).exists():
            findings.append(
                result(
                    "blocker",
                    "MISSING_REQUIRED_EVIDENCE",
                    f"Required evidence file `{name}` is missing.",
                    str(name),
                    "Generate the required evidence artifact before considering the task closed.",
                )
            )
    return findings


def check_evidence_type(task: dict[str, Any], summary: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    expected = str(task.get("acceptance_type") or "")
    actual = str(summary.get("evidence_type") or "unknown")
    if actual in PREP_EVIDENCE_TYPES:
        findings.append(
            result(
                "blocker",
                "PREP_EVIDENCE_CANNOT_CLOSE",
                f"Evidence type `{actual}` is preparation evidence and cannot close `{expected}`.",
                "run-summary.json",
                "Run the concrete verification gate and publish the matching evidence layer.",
            )
        )
    elif expected and actual != expected:
        findings.append(
            result(
                "blocker",
                "EVIDENCE_TYPE_MISMATCH",
                f"Task requires `{expected}` evidence but run has `{actual}`.",
                "run-summary.json",
                "Do not substitute smoke, regression, acceptance, security, or third-party evidence for each other.",
            )
        )
    return findings


def check_status(summary: dict[str, Any]) -> list[dict[str, str]]:
    status = str(summary.get("status") or "UNKNOWN")
    if any(fragment in status for fragment in BAD_STATUS_FRAGMENTS):
        return [
            result(
                "blocker",
                "RUN_STATUS_NOT_CLOSABLE",
                f"Run status `{status}` is not eligible for task closure.",
                "run-summary.json",
                "Resolve the blocked/prep/failure status and regenerate evidence.",
            )
        ]
    return []


def check_local_report(task: dict[str, Any], run_dir: Path) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    local_checks = list_of((task.get("verification") or {}).get("local"))
    if not local_checks:
        return findings
    local_path = run_dir / "local-report.md"
    if not local_path.exists():
        return [
            result(
                "blocker",
                "LOCAL_REPORT_MISSING",
                "Task declares local verification but `local-report.md` is missing.",
                "local-report.md",
                "Run the smallest relevant local verification through `run_task.py --execute-local` or attach an equivalent report.",
            )
        ]
    text = read_optional(local_path)
    if "Not executed" in text:
        findings.append(
            result(
                "blocker",
                "LOCAL_NOT_EXECUTED",
                "`local-report.md` exists but local verification was not executed.",
                "local-report.md",
                "Re-run with explicit local execution after checking the task plan.",
            )
        )
    elif local_report_failed(text):
        findings.append(
            result(
                "blocker",
                "LOCAL_FAILED",
                "`local-report.md` contains a non-zero exit code.",
                "local-report.md",
                "Fix the failing implementation or test environment, then rerun verification.",
            )
        )
    elif not local_report_has_success(text):
        findings.append(
            result(
                "warning",
                "LOCAL_RESULT_UNCLEAR",
                "`local-report.md` does not contain explicit exit codes.",
                "local-report.md",
                "Record command exit codes so the evidence can be replayed by a reviewer.",
            )
        )
    return findings


def check_review(task: dict[str, Any], run_dir: Path) -> list[dict[str, str]]:
    if not (task.get("review") or {}).get("required"):
        return []
    review_path = run_dir / "review-report.md"
    if not review_path.exists():
        return [
            result(
                "blocker",
                "REVIEW_REPORT_MISSING",
                "Task requires reviewer gate but `review-report.md` is missing.",
                "review-report.md",
                "Run or complete the third-view reviewer gate before closure.",
            )
        ]
    text = read_optional(review_path)
    decision = extract_review_decision(text)
    if not decision:
        return [
            result(
                "blocker",
                "REVIEW_DECISION_MISSING",
                "`review-report.md` has no parseable decision.",
                "review-report.md",
                "Set decision to pass, repair_required, design_update_required, or human_gate_required.",
            )
        ]
    if decision not in PASS_REVIEW_DECISIONS:
        return [
            result(
                "blocker",
                "REVIEW_NOT_PASSED",
                f"Reviewer decision is `{decision}`.",
                "review-report.md",
                "Resolve reviewer findings before using this evidence to close the task.",
            )
        ]
    return []


def check_llm_review(run_dir: Path) -> list[dict[str, str]]:
    summary = load_json(run_dir / "llm-review" / "llm-review-summary.json")
    if not summary:
        return []
    status = str(summary.get("status") or "")
    if status == "LLM_REVIEW_PLANNED":
        return [
            result(
                "warning",
                "LLM_REVIEW_NOT_EXECUTED",
                "LLM reviewer is planned but no model output was ingested.",
                "llm-review/llm-review-summary.json",
                "For high-confidence closure, ingest LLM reviewer output and rerun evidence_check.py.",
            )
        ]
    if status == "LLM_REVIEW_PASSED":
        return []
    return [
        result(
            "blocker",
            "LLM_REVIEW_NOT_PASSED",
            f"LLM reviewer status is `{status}` with decision `{summary.get('decision')}`.",
            "llm-review/llm-review-summary.json",
            "Resolve the LLM reviewer finding, rerun static/local evidence, or move the task behind a human gate.",
        )
    ]


def check_close_conditions(task: dict[str, Any], run_dir: Path) -> list[dict[str, str]]:
    evidence_report = read_optional(run_dir / "evidence-report.md")
    if not evidence_report:
        return [
            result(
                "blocker",
                "EVIDENCE_REPORT_MISSING",
                "`evidence-report.md` is missing, so close_when cannot be proven.",
                "evidence-report.md",
                "Write an evidence report that maps each close_when item to concrete files, commands, screenshots, SQL, or logs.",
            )
        ]
    findings: list[dict[str, str]] = []
    for condition in list_of(task.get("close_when")):
        if not close_condition_covered(str(condition), evidence_report):
            findings.append(
                result(
                    "blocker",
                    "CLOSE_WHEN_UNPROVEN",
                    f"Close condition is not evidenced: {condition}",
                    "evidence-report.md",
                    "Map this condition to a concrete artifact and result before closure.",
                )
            )
    return findings


def check_runtime_reports(task: dict[str, Any], run_dir: Path, summary: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    live_checks = list_of((task.get("verification") or {}).get("live_readonly"))
    live_path = run_dir / "live-report.md"
    if live_path.exists():
        error_lines = report_error_lines(read_optional(live_path))
        if error_lines:
            findings.append(
                result(
                    "blocker",
                    "LIVE_REPORT_HAS_ERRORS",
                    f"`live-report.md` contains runtime/API error signals: {' | '.join(error_lines[:3])}",
                    "live-report.md",
                    "Fix the live/API/browser issue or document expected auth-negative responses with exact status and route.",
                )
            )
    elif live_checks and summary.get("evidence_type") not in PREP_EVIDENCE_TYPES:
        findings.append(
            result(
                "warning",
                "LIVE_REPORT_MISSING",
                "Task declares live_readonly verification but `live-report.md` is missing.",
                "live-report.md",
                "Attach live readonly evidence before claiming live-backed regression or acceptance.",
            )
        )

    for name in ["browser-console-report.md", "browser-report.md"]:
        path = run_dir / name
        if not path.exists():
            continue
        error_lines = report_error_lines(read_optional(path))
        if error_lines:
            findings.append(
                result(
                    "blocker",
                    "BROWSER_REPORT_HAS_ERRORS",
                    f"`{name}` contains browser error signals: {' | '.join(error_lines[:3])}",
                    name,
                    "Browser-facing evidence must have no requestfailed, pageerror, runtime exception, or non-warning console errors.",
                )
            )
    return findings


def check_sql_artifacts(run_dir: Path) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    sql_dir = run_dir / "sql"
    if not sql_dir.exists():
        return findings
    for path in sorted(sql_dir.glob("*.sql")):
        text = path.read_text(encoding="utf-8")
        for pattern in DESTRUCTIVE_SQL_PATTERNS:
            if re.search(pattern, text, flags=re.IGNORECASE):
                findings.append(
                    result(
                        "blocker",
                        "DESTRUCTIVE_SQL_ARTIFACT",
                        f"`{rel_path(path)}` contains destructive SQL matching `{pattern}`.",
                        rel_path(path),
                        "Replace destructive SQL with read-only evidence queries or move the task behind a human gate.",
                    )
                )
                break
    return findings


def check_codex_structured_output(run_dir: Path) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    intake = load_json(run_dir / "patch-runner" / "patch-intake.json")
    if not intake:
        return findings
    validation = intake.get("validation") or {}
    for item in validation.get("findings", []):
        if item.get("level") == "blocker":
            findings.append(
                result(
                    "blocker",
                    "PATCH_RUNNER_BLOCKER",
                    f"{item.get('code')}: {item.get('message')}",
                    "patch-runner/patch-intake.json",
                    "Resolve the patch runner finding before closing the task.",
                )
            )
    codex_output_path = intake.get("codex_output_path")
    if codex_output_path:
        codex_output = load_json(repo_path(codex_output_path))
        for key in ["summary", "files", "tests", "evidence", "risks"]:
            if key not in codex_output:
                findings.append(
                    result(
                        "blocker",
                        "CODEX_OUTPUT_FIELD_MISSING",
                        f"`codex-output.json` is missing `{key}`.",
                        str(codex_output_path),
                        "Regenerate Codex structured output using patch-runner/codex-output-contract.json.",
                    )
                )
    return findings


def check_patch_validation(run_dir: Path) -> list[dict[str, str]]:
    validation = load_json(run_dir / "implementation" / "patch-validation.json")
    if validation and validation.get("valid") is False:
        return [
            result(
                "blocker",
                "PATCH_SCOPE_INVALID",
                "Implementation patch validation is false.",
                "implementation/patch-validation.json",
                "Fix scope, contract declaration, or guidance blockers before applying or closing the task.",
            )
        ]
    return []


def check_guidance(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    for item in guidance_blockers(str(task.get("id")), guidance):
        findings.append(
            result(
                "blocker",
                "GUIDANCE_BLOCKER",
                f"{item.get('code')}: {item.get('message')}",
                "guidance/guidance.json",
                str(item.get("suggestion") or "Resolve guidance blocker before task closure."),
            )
        )
    return findings


def next_state(findings: list[dict[str, str]]) -> str:
    blockers = [item for item in findings if item["level"] == "blocker"]
    if not blockers:
        return "CLOSED_CANDIDATE"
    codes = {item["code"] for item in blockers}
    if "GUIDANCE_BLOCKER" in codes or "REVIEW_NOT_PASSED" in codes and any(
        "design" in item["message"].lower() for item in blockers
    ):
        return "DESIGN_ITERATING"
    if "REVIEW_NOT_PASSED" in codes and any("human" in item["message"].lower() for item in blockers):
        return "NEEDS_HUMAN_GATE"
    return "REPAIRING"


def build_check(
    task: dict[str, Any],
    run_dir: Path,
    guidance: dict[str, Any],
) -> dict[str, Any]:
    summary = latest_summary(run_dir)
    findings: list[dict[str, str]] = []
    findings.extend(check_required_files(task, run_dir))
    findings.extend(check_evidence_type(task, summary))
    findings.extend(check_status(summary))
    findings.extend(check_guidance(task, guidance))
    findings.extend(check_patch_validation(run_dir))
    findings.extend(check_codex_structured_output(run_dir))
    findings.extend(check_local_report(task, run_dir))
    findings.extend(check_runtime_reports(task, run_dir, summary))
    findings.extend(check_sql_artifacts(run_dir))
    findings.extend(check_review(task, run_dir))
    findings.extend(check_llm_review(run_dir))
    findings.extend(check_close_conditions(task, run_dir))
    blockers = [item for item in findings if item["level"] == "blocker"]
    warnings = [item for item in findings if item["level"] == "warning"]
    status = "EVIDENCE_PASSED" if not blockers else "EVIDENCE_REJECTED"
    return {
        "run_kind": "evidence_check",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "checked_run": rel_path(run_dir),
        "status": status,
        "recommended_next_state": next_state(findings),
        "evidence_type": summary.get("evidence_type"),
        "expected_evidence_type": task.get("acceptance_type"),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "summary_status": summary.get("status"),
        "blocker_count": len(blockers),
        "warning_count": len(warnings),
        "findings": findings,
        "closable": not blockers,
    }


def render_report(check: dict[str, Any]) -> str:
    lines = [
        f"# Evidence Check: {check.get('task_id')}",
        "",
        f"- checked_run: `{check['checked_run']}`",
        f"- status: `{check['status']}`",
        f"- recommended_next_state: `{check['recommended_next_state']}`",
        f"- expected_evidence_type: `{check.get('expected_evidence_type')}`",
        f"- actual_evidence_type: `{check.get('evidence_type')}`",
        f"- summary_status: `{check.get('summary_status')}`",
        f"- blockers: `{check['blocker_count']}`",
        f"- warnings: `{check['warning_count']}`",
        "",
        "## Findings",
    ]
    if check["findings"]:
        for item in check["findings"]:
            lines.append(
                f"- `{item['level']}` `{item['code']}` `{item['evidence']}`: {item['message']} Suggestion: {item['suggestion']}"
            )
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Rule",
            "- This checker is conservative: prep evidence, pending review, missing local reports, and unproven close_when items cannot close a task.",
            "- The checker does not modify task status. Use `task_state.py --apply` only after reviewing this result.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--run-dir", default=None)
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--out-dir", default=None)
    parser.add_argument("--fail-on-blocker", action="store_true")
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    if args.run_dir:
        run_dir = repo_path(args.run_dir)
    else:
        run_id = args.run_id or make_run_id(str(task.get("id", "evidence-check")))
        run_dir = ensure_run_dir(run_id)
    copy_task_snapshot(task_path, run_dir)

    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "evidence-check"
    out_dir.mkdir(parents=True, exist_ok=True)
    guidance = load_json(repo_path(args.guidance) if args.guidance else run_dir / "guidance" / "guidance.json")
    check = build_check(task, run_dir, guidance)
    write_json(out_dir / "evidence-check.json", check)
    write_text(out_dir / "evidence-check-report.md", render_report(check))
    print(out_dir)
    print(f"status={check['status']} blockers={check['blocker_count']} next={check['recommended_next_state']}")
    return 2 if args.fail_on_blocker and check["blocker_count"] else 0


if __name__ == "__main__":
    raise SystemExit(main())

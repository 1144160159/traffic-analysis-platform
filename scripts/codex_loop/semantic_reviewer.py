#!/usr/bin/env python3
"""Add a deeper semantic-review layer on top of diff-aware review output."""

from __future__ import annotations

import argparse
import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import copy_task_snapshot, ensure_run_dir, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


DOMAIN_KEYWORDS = {
    "auth": ["auth", "token", "permission", "tenant", "unauthorized", "protected"],
    "screen": ["/screen", "readonly", "public", "sensitive", "脱敏"],
    "evidence": ["evidence", "run_id", "close_when", "review", "local-report"],
    "ui": ["route", "menu", "breadcrumb", "tab", "browser"],
}


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def finding(level: str, code: str, message: str, suggestion: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message, "suggestion": suggestion}


def text_for_review(task: dict[str, Any], run_dir: Path) -> str:
    parts = [
        str(task.get("title", "")),
        "\n".join(str(item) for item in task.get("close_when", []) or []),
    ]
    for path in [
        run_dir / "design" / "feature-spec.md",
        run_dir / "design" / "architecture-evolution.md",
        run_dir / "context-pack" / "task-context-pack.md",
        run_dir / "review-report.md",
        run_dir / "evidence-report.md",
    ]:
        if path.exists():
            parts.append(path.read_text(encoding="utf-8")[:12000])
    return "\n".join(parts).lower()


def semantic_findings(task: dict[str, Any], run_dir: Path, review: dict[str, Any]) -> list[dict[str, str]]:
    text = text_for_review(task, run_dir)
    findings: list[dict[str, str]] = []
    if str(task.get("priority")) == "P0" and review.get("decision") == "pass" and not (run_dir / "evidence-report.md").exists():
        findings.append(finding("blocker", "P0_PASS_WITHOUT_EVIDENCE_REPORT", "P0 review cannot pass without evidence-report.md.", "Attach evidence-report.md and rerun evidence_check.py."))
    if "/screen" in text:
        strategy_hits = sum(1 for word in ["public", "protected", "readonly", "脱敏"] if word in text)
        if strategy_hits == 0:
            findings.append(finding("blocker", "SCREEN_STRATEGY_NOT_SEMANTICALLY_STATED", "/screen strategy is not semantically stated.", "Choose public, protected, readonly-token, or desensitized-public and document it."))
    for domain, words in DOMAIN_KEYWORDS.items():
        if domain in {"auth", "evidence"} and any(word in text for word in words[:2]):
            missing = [word for word in words if word not in text]
            if len(missing) >= len(words) - 1:
                findings.append(finding("warning", f"{domain.upper()}_SEMANTIC_COVERAGE_THIN", f"{domain} topic is mentioned but semantic coverage is thin.", "Expand design/review evidence for this domain."))
    if re.search(r"\bmock\b", text) and str(task.get("acceptance_type")) in {"regression", "acceptance"}:
        findings.append(finding("warning", "MOCK_MENTION_IN_CLOSURE_CONTEXT", "Mock is mentioned in a regression/acceptance closure context.", "Ensure mock evidence is not used as live or acceptance proof."))
    return findings


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        f"# Semantic Review: {summary.get('task_id')}",
        "",
        f"- status: `{summary['status']}`",
        f"- review_decision: `{summary.get('review_decision')}`",
        f"- findings: `{len(summary['findings'])}`",
        "",
        "## Findings",
    ]
    if summary["findings"]:
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']} Suggestion: {item['suggestion']}")
    else:
        lines.append("- none")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--review-summary", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "semantic-review")))
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "semantic-review"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)
    review_path = repo_path(args.review_summary) if args.review_summary else run_dir / "review" / "review-summary.json"
    review = load_json(review_path)
    findings = semantic_findings(task, run_dir, review)
    if any(item["level"] == "blocker" for item in findings):
        status = "SEMANTIC_REVIEW_BLOCKED"
    elif review.get("decision") not in {"pass", "passed", "approved"}:
        status = "SEMANTIC_REVIEW_HELD"
    else:
        status = "SEMANTIC_REVIEW_PASSED"
    summary = {
        "run_id": run_id,
        "run_kind": "semantic_review",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": status,
        "review_decision": review.get("decision"),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "findings": findings,
        "outputs": ["semantic-review/semantic-review.json", "semantic-review/semantic-review-report.md"],
    }
    write_json(out_dir / "semantic-review.json", summary)
    write_text(out_dir / "semantic-review-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} findings={len(findings)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

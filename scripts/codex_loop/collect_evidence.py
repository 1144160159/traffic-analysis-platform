#!/usr/bin/env python3
"""Collect lightweight evidence for a Codex Loop run."""

from __future__ import annotations

import argparse
from datetime import datetime

from lib import copy_task_snapshot, ensure_run_dir, load_yaml_subset, run_git, write_json, write_text


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--task", default=None)
    parser.add_argument("--status", default="EVIDENCE_COLLECTED")
    parser.add_argument("--evidence-type", default="regression")
    args = parser.parse_args()

    run_dir = ensure_run_dir(args.run_id)
    if args.task:
        copy_task_snapshot(args.task, run_dir)
        task = load_yaml_subset(args.task)
    else:
        task = {}

    write_text(run_dir / "git-status.txt", run_git(["status", "--short"]))
    write_text(run_dir / "changed-files.txt", run_git(["diff", "--name-only"]))
    summary = {
        "run_id": args.run_id,
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": args.status,
        "evidence_type": args.evidence_type,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "files": {
            "git_status": "git-status.txt",
            "changed_files": "changed-files.txt",
        },
        "warning": "This evidence collector is lightweight and does not imply acceptance or third-party pass.",
    }
    write_json(run_dir / "run-summary.json", summary)
    print(run_dir)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

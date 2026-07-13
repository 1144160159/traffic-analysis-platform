#!/usr/bin/env python3
"""Update the status field in a run-summary.json file."""

from __future__ import annotations

import argparse
import json
from datetime import datetime

from lib import ensure_run_dir, write_json


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--status", required=True)
    args = parser.parse_args()

    run_dir = ensure_run_dir(args.run_id)
    summary_path = run_dir / "run-summary.json"
    if summary_path.exists():
        summary = json.loads(summary_path.read_text(encoding="utf-8"))
    else:
        summary = {"run_id": args.run_id}
    summary["status"] = args.status
    summary["updated_at"] = datetime.now().isoformat(timespec="seconds")
    write_json(summary_path, summary)
    print(summary_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

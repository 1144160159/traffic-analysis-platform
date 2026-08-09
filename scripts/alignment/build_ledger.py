#!/usr/bin/env python3
"""Generate the 102-item remediation ledger from canonical ownership sources."""

from __future__ import annotations

import argparse
import json
from datetime import date
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
REGISTRY = ROOT / "contracts/alignment/canonical-registry.json"
PACKAGES = ROOT / "contracts/alignment/work-packages.json"
OVERRIDES = ROOT / "contracts/alignment/progress-overrides.json"
DEFAULT_OUTPUT = ROOT / "contracts/alignment/remediation-ledger.json"

WAVE_BY_PACKAGE = {
    "WP-00-INVENTORY": "W0-W1",
    "WP-01-ERROR-ADAPTER": "W1",
    "WP-02-PLATFORM-SEC": "W1-W7",
    "WP-03-OBS-DQ": "W1-W7",
    "WP-04-DASHBOARD": "W1-W2",
    "WP-05-PROBE": "W1-W2",
    "WP-06-ASSET": "W2",
    "WP-07-ALERT": "W3",
    "WP-08-CAMPAIGN-CHAIN": "W1-W3",
    "WP-09-TOPIC": "W4",
    "WP-10-ENCRYPTED": "W1-W7",
    "WP-11-FORENSICS": "W1-W7",
    "WP-12-GRAPH": "W5",
    "WP-13-FUSION-BASELINE": "W1-W7",
    "WP-14-RULE-DEPLOY": "W1-W7",
    "WP-15-MODEL-MLOPS": "W1-W7",
    "WP-16-PLAYBOOK": "W1-W7",
    "WP-17-WHITELIST": "W1-W7",
    "WP-18-COMPLIANCE-AUDIT": "W1-W7",
    "WP-19-NOTIFY-SETTINGS": "W1-W7",
    "WP-20-SEARCH-EXPORT": "W1-W7",
    "WP-21-PG": "W1-W6",
    "WP-22-CH": "W1-W6",
    "WP-23-OS": "W1-W6",
    "WP-24-KAFKA": "W1-W6",
    "WP-25-FLINK": "W1-W6",
    "WP-26-REDIS": "W1-W6",
    "WP-27-MINIO": "W1-W6",
    "WP-28-DR": "W1-W7",
}
ALLOWED_STATUSES = {"OPEN", "BLOCKED", "IMPLEMENTING", "VERIFYING", "OBSERVING", "CLOSED"}


def _load(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def build_ledger() -> dict[str, Any]:
    registry = _load(REGISTRY)
    package_document = _load(PACKAGES)
    override_document = _load(OVERRIDES)
    packages = {item["id"]: item for item in package_document["packages"]}
    overrides = override_document["overrides"]
    unknown_overrides = sorted(set(overrides) - {item["id"] for item in registry["items"]})
    if unknown_overrides:
        raise ValueError(f"unknown progress overrides: {unknown_overrides}")

    entries = []
    for item in registry["items"]:
        package = packages[item["work_package"]]
        override = overrides.get(item["id"], {})
        status = override.get("status", "OPEN")
        if status not in ALLOWED_STATUSES:
            raise ValueError(f"{item['id']} has invalid status {status}")
        entries.append(
            {
                "id": item["id"],
                "priority": item["priority"],
                "work_package": item["work_package"],
                "planned_wave": WAVE_BY_PACKAGE[item["work_package"]],
                "owner": package["accountable"],
                "status": status,
                "evidence": override.get("evidence", []),
                "blocker": override.get("blocker"),
                "note": override.get("note", ""),
            }
        )

    status_counts: dict[str, int] = {}
    for entry in entries:
        status_counts[entry["status"]] = status_counts.get(entry["status"], 0) + 1
    return {
        "schema_version": 1,
        "generated_from": [
            REGISTRY.relative_to(ROOT).as_posix(),
            PACKAGES.relative_to(ROOT).as_posix(),
            OVERRIDES.relative_to(ROOT).as_posix(),
        ],
        "as_of": override_document.get("updated_at", date.today().isoformat()),
        "g7_status": "OPEN",
        "g8_status": "BLOCKED",
        "g8_blockers": [
            "100G/Mpps实验室",
            "破坏性HA窗口",
            "检测质量盲测集",
            "现场资产签认",
            "用户及第三方验收"
        ],
        "counts": {
            "total": len(entries),
            "by_status": dict(sorted(status_counts.items())),
        },
        "items": entries,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    payload = json.dumps(build_ledger(), ensure_ascii=False, indent=2) + "\n"
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(payload, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Create a bounded, read-only inventory of external PCAP replay candidates."""

from __future__ import annotations

import argparse
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def label_for(path: Path) -> str:
    lowered = str(path).lower()
    labels = [
        "benign",
        "normal",
        "mitm",
        "dns",
        "fingerprint",
        "upload",
        "exfiltration",
        "tunnel",
        "ddos",
        "dos",
        "bruteforce",
        "sql",
        "xss",
        "ransomware",
        "backdoor",
        "scan",
    ]
    matches = [label for label in labels if label in lowered]
    return ",".join(matches) if matches else "unclassified"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dataset-root", type=Path, default=Path("/home/wangwt/task/datasets"))
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--candidate-limit", type=int, default=20)
    parser.add_argument("--candidate-max-bytes", type=int, default=64 * 1024 * 1024)
    args = parser.parse_args()

    files = sorted(
        (path for path in args.dataset_root.rglob("*") if path.is_file() and path.suffix.lower() in {".pcap", ".pcapng"}),
        key=lambda path: (path.stat().st_size, str(path)),
    )
    candidates = [path for path in files if path.stat().st_size <= args.candidate_max_bytes][: args.candidate_limit]
    payload = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "dataset_root": str(args.dataset_root),
        "read_only": True,
        "pcap_count": len(files),
        "total_bytes": sum(path.stat().st_size for path in files),
        "candidate_policy": {
            "limit": args.candidate_limit,
            "max_bytes": args.candidate_max_bytes,
            "ordering": "size_then_path",
        },
        "candidates": [
            {
                "path": str(path),
                "bytes": path.stat().st_size,
                "sha256": sha256(path),
                "label": label_for(path),
            }
            for path in candidates
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"result": "pass", "pcap_count": len(files), "total_bytes": payload["total_bytes"], "candidates": len(candidates), "output": str(args.output)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

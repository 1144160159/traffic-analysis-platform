#!/usr/bin/env python3
"""Run the probe-agent offline replayer against external dataset PCAPs."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import subprocess
from pathlib import Path


RESULT_RE = re.compile(
    r"DATASET_PCAP_REPLAY_OK path=(?P<path>.*?) "
    r"file_bytes=(?P<file_bytes>\d+) captured=(?P<captured>\d+) "
    r"parsed=(?P<parsed>\d+) replay_bytes=(?P<replay_bytes>\d+)"
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--pcap", action="append", required=True)
    parser.add_argument(
        "--workspace",
        default="rust/probe-agent",
        help="Cargo workspace containing the probe-agent package",
    )
    parser.add_argument(
        "--output",
        default="doc/02_acceptance/02-regression/pcap-dataset-replay-latest.json",
    )
    args = parser.parse_args()

    repo = Path(__file__).resolve().parents[2]
    workspace = (repo / args.workspace).resolve()
    results: list[dict[str, object]] = []
    overall = "pass"

    for raw_path in args.pcap:
        pcap = Path(raw_path).expanduser().resolve()
        item: dict[str, object] = {
            "path": str(pcap),
            "exists": pcap.is_file(),
        }
        if not pcap.is_file():
            item["result"] = "fail"
            item["error"] = "PCAP file does not exist"
            results.append(item)
            overall = "fail"
            continue

        item["file_bytes"] = pcap.stat().st_size
        item["sha256"] = sha256(pcap)
        env = os.environ.copy()
        env["TRAFFIC_TEST_PCAP"] = str(pcap)
        completed = subprocess.run(
            [
                "cargo",
                "test",
                "-p",
                "probe-agent",
                "--test",
                "dataset_pcap_replay_test",
                "--",
                "--ignored",
                "--nocapture",
            ],
            cwd=workspace,
            env=env,
            check=False,
            capture_output=True,
            text=True,
        )
        output = completed.stdout + completed.stderr
        match = RESULT_RE.search(output)
        if completed.returncode == 0 and match:
            item.update(
                {
                    "result": "pass",
                    "captured": int(match.group("captured")),
                    "parsed": int(match.group("parsed")),
                    "replay_bytes": int(match.group("replay_bytes")),
                }
            )
        else:
            item["result"] = "fail"
            item["exit_code"] = completed.returncode
            item["output_tail"] = "\n".join(output.splitlines()[-80:])
            overall = "fail"
        results.append(item)

    report = {
        "schema_version": 1,
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "result": overall,
        "runner": "probe-agent PcapReplayer + PacketParser",
        "read_only_source": True,
        "samples": results,
    }
    output_path = (repo / args.output).resolve()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"result": overall, "output": str(output_path), "samples": len(results)}))
    return 0 if overall == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())

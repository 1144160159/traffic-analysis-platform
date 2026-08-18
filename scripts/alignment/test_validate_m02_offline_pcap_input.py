#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import json
from pathlib import Path


SCRIPT = Path(__file__).with_name("validate_m02_offline_pcap_input.py")
SPEC = importlib.util.spec_from_file_location("validate_m02_offline_pcap_input", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def expect_failure(manifest: dict, expected: str) -> None:
    try:
        MODULE.validate_manifest(manifest)
    except ValueError as error:
        assert expected in str(error), error
        return
    raise AssertionError(f"mutation did not fail: {expected}")


def main() -> int:
    manifest = json.loads(MODULE.MANIFEST.read_text(encoding="utf-8"))
    MODULE.validate_manifest(manifest)

    mutated = copy.deepcopy(manifest)
    mutated["entries"][0]["relative_path"] = "../escape.pcap"
    expect_failure(mutated, "normalized relative path")
    mutated = copy.deepcopy(manifest)
    mutated["entries"][1]["entry_id"] = mutated["entries"][0]["entry_id"]
    expect_failure(mutated, "duplicated")
    mutated = copy.deepcopy(manifest)
    mutated["approval_status"] = "APPROVED"
    expect_failure(mutated, "overstated")
    mutated = copy.deepcopy(manifest)
    mutated["entries"][0]["sha256"] = "A" * 64
    expect_failure(mutated, "lowercase")
    print("PASS M02 offline PCAP source manifest mutations")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import tempfile
from pathlib import Path


SCRIPT = Path(__file__).with_name("run_m02_loopback_kafka_minio.py")
SPEC = importlib.util.spec_from_file_location("run_m02_loopback_kafka_minio", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def main() -> int:
    summary = MODULE.self_check()
    assert summary["required_parent_index_count"] == 12
    assert summary["all_images_digest_pinned"] is True
    assert summary["production_applied"] is False

    with tempfile.TemporaryDirectory(prefix="m02-n014-runner-test-") as directory:
        root = Path(directory)
        original = MODULE.REQUIRED_PARENT_INDEXES
        try:
            MODULE.REQUIRED_PARENT_INDEXES = tuple(root / f"n{number:03d}.json" for number in range(1, 13))
            try:
                MODULE.require_current_parent_indexes()
                raise AssertionError("missing parent indexes did not block")
            except MODULE.LoopbackRejection as error:
                assert error.code == "BLOCK_M02_PARENT_CURRENT_IDX_MISSING"
            for number, path in enumerate(MODULE.REQUIRED_PARENT_INDEXES, start=1):
                path.write_text(
                    '{"parent_task_id":"T1-M02-N%03d","status":"PASS"}\n' % number,
                    encoding="utf-8",
                )
            receipts = MODULE.require_current_parent_indexes()
            assert len(receipts) == 12
            assert all(len(item["sha256"]) == 64 for item in receipts)
        finally:
            MODULE.REQUIRED_PARENT_INDEXES = original
    print("PASS M02 N014 loopback runner self-test")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

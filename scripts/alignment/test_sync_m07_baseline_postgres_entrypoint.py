#!/usr/bin/env python3
"""Mutation tests for deterministic M07 baseline migration synchronization."""

from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("sync_m07_baseline_postgres_entrypoint.py")
SPEC = importlib.util.spec_from_file_location("sync_m07_baseline_postgres_entrypoint", MODULE_PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class M07BaselinePostgresSyncTest(unittest.TestCase):
    def setUp(self) -> None:
        self.block = (
            MODULE.BEGIN_MARKER
            + f"  {MODULE.K8S_KEY}: |\n"
            + "    SELECT 40;\n"
            + MODULE.END_MARKER
            + "\n"
        )
        self.source = (
            "apiVersion: v1\nkind: ConfigMap\ndata:\n"
            "  39-m07-fusion-snapshots-v1.sql: |\n    SELECT 39;\n"
            "---\napiVersion: batch/v1\nkind: Job\nspec:\n"
            "  template:\n    spec:\n      containers:\n      - args:\n"
            "        - |\n"
            "          for f in 38-rule-version-rollback-v1.sql 39-m07-fusion-snapshots-v1.sql; do\n"
            "            echo $f\n          done\n"
        )

    def test_append_is_ordered_and_idempotent(self) -> None:
        rendered = MODULE.synchronize(self.source, self.block)
        self.assertIn(
            "39-m07-fusion-snapshots-v1.sql 40-m07-behavior-baseline-v1.sql; do", rendered
        )
        self.assertEqual(rendered, MODULE.synchronize(rendered, self.block))

    def test_duplicate_runner_entry_is_rejected(self) -> None:
        source = self.source.replace(
            "39-m07-fusion-snapshots-v1.sql; do",
            "39-m07-fusion-snapshots-v1.sql 40-m07-behavior-baseline-v1.sql "
            "40-m07-behavior-baseline-v1.sql; do",
        )
        with self.assertRaisesRegex(ValueError, "duplicated"):
            MODULE.synchronize(source, self.block)

    def test_out_of_order_append_is_rejected(self) -> None:
        source = self.source.replace(
            "39-m07-fusion-snapshots-v1.sql; do",
            "39-m07-fusion-snapshots-v1.sql future.sql; do",
        )
        with self.assertRaisesRegex(ValueError, "append after"):
            MODULE.synchronize(source, self.block)

    def test_partial_generated_markers_are_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "only one marker"):
            MODULE.synchronize(MODULE.BEGIN_MARKER + self.source, self.block)


if __name__ == "__main__":
    unittest.main()

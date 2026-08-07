from __future__ import annotations

import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from inventory_pg_mutations import CONTRACT, compare_snapshot, scan_root  # noqa: E402


class PostgresMutationInventoryTest(unittest.TestCase):
    def test_repository_matches_versioned_snapshot(self) -> None:
        result = compare_snapshot(scan_root())
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(0, result["summary"]["unclassified"])
        self.assertGreater(result["summary"]["backend_counts"]["postgresql"], 0)
        self.assertGreater(result["summary"]["backend_counts"]["clickhouse"], 0)

    def test_known_clickhouse_write_is_not_counted_as_postgresql(self) -> None:
        inventory = scan_root()
        matches = [
            mutation
            for record in inventory["records"]
            if record["source"].endswith("internal/alert/persistence/clickhouse.go")
            for mutation in record["mutations"]
            if mutation["table_expression"] == "traffic.alerts"
        ]
        self.assertTrue(matches)
        self.assertEqual({"clickhouse"}, {item["backend"] for item in matches})

    def test_unknown_dynamic_table_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "go/control-plane/internal/example/repository.go"
            source.parent.mkdir(parents=True)
            source.write_text(
                "package example\nfunc write() { q := `UPDATE %s SET value=$1` ; _ = q }\n",
                encoding="utf-8",
            )
            inventory = scan_root(root, dynamic_overrides={})
        self.assertEqual(1, inventory["summary"]["unclassified"])
        self.assertIn("dynamic table", inventory["unclassified"][0]["reason"])

    def test_unknown_qualified_schema_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "go/control-plane/internal/example/repository.go"
            source.parent.mkdir(parents=True)
            source.write_text(
                "package example\nvar q = `INSERT INTO mystery.events (id) VALUES ($1)`\n",
                encoding="utf-8",
            )
            inventory = scan_root(root, dynamic_overrides={})
        self.assertEqual(1, inventory["summary"]["unclassified"])
        self.assertIn("mystery", inventory["unclassified"][0]["reason"])

    def test_saved_view_source_exposes_all_source_level_signals(self) -> None:
        inventory = scan_root()
        record = next(
            item
            for item in inventory["records"]
            if item["source"].endswith("internal/alert/api/handler_alert_actions.go")
        )
        facts = record["source_facts"]
        self.assertTrue(facts["has_transaction_begin"])
        self.assertTrue(facts["uses_transaction_handle"])
        self.assertTrue(facts["has_audit_signal"])
        self.assertTrue(facts["has_outbox_signal"])
        self.assertTrue(facts["has_idempotency_signal"])
        roles = {
            mutation["table_expression"]: mutation["role"]
            for mutation in record["mutations"]
        }
        self.assertEqual("business", roles["alert_saved_views"])
        self.assertEqual("history", roles["alert_saved_view_history"])
        self.assertEqual("outbox", roles["alert_saved_view_outbox"])
        self.assertEqual("control_idempotency", roles["alert_saved_view_requests"])

    def test_graph_hot_ip_cache_is_a_reviewed_rebuildable_projection(self) -> None:
        inventory = scan_root()
        record = next(
            item
            for item in inventory["records"]
            if item["source"].endswith("internal/graph/cache/cache_warmup.go")
        )
        mutations = [
            item for item in record["mutations"] if item["table_expression"] == "graph_hot_ips"
        ]
        self.assertEqual(1, len(mutations))
        self.assertEqual("inbox_projection", mutations[0]["role"])
        self.assertIn("rebuildable", mutations[0]["role_review_basis"])
        self.assertFalse(any(
            item["source"] == record["source"] for item in inventory["review_queue"]
        ))

    def test_governed_command_audit_helpers_are_exactly_recognized(self) -> None:
        inventory = scan_root()
        by_source = {record["source"]: record for record in inventory["records"]}
        expected = (
            "go/control-plane/internal/alert/whitelist/command_atomic.go",
            "go/control-plane/cmd/threat-intel-service/command_atomic.go",
        )
        queued = {item["source"] for item in inventory["review_queue"]}
        for source in expected:
            with self.subTest(source=source):
                facts = by_source[source]["source_facts"]
                self.assertTrue(facts["has_transaction_begin"])
                self.assertTrue(facts["uses_transaction_handle"])
                self.assertTrue(facts["has_audit_signal"])
                self.assertTrue(facts["has_outbox_signal"])
                self.assertTrue(facts["has_idempotency_signal"])
                self.assertNotIn(source, queued)

    def test_legacy_whitelist_file_has_no_business_mutation_bypass(self) -> None:
        inventory = scan_root()
        self.assertFalse(any(
            record["source"] == "go/control-plane/internal/alert/whitelist/whitelist.go"
            for record in inventory["records"]
        ))

    def test_snapshot_drift_is_rejected(self) -> None:
        snapshot = json.loads(CONTRACT.read_text(encoding="utf-8"))
        mutated = copy.deepcopy(snapshot)
        mutated["summary"]["source_files"] += 1
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "snapshot.json"
            path.write_text(json.dumps(mutated), encoding="utf-8")
            result = compare_snapshot(scan_root(), path)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("differs" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()

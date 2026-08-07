import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_alert_projection_kafka_five_store_ephemeral.py"


def load_module():
    spec = importlib.util.spec_from_file_location("alert_projection_kafka_five_store_g1", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class AlertProjectionKafkaFiveStoreEphemeralGuardTest(unittest.TestCase):
    def test_all_images_are_digest_pinned(self) -> None:
        module = load_module()
        for image in (
            module.CLICKHOUSE_IMAGE,
            module.POSTGRES_IMAGE,
            module.OPENSEARCH_IMAGE,
            module.REDIS_IMAGE,
            module.KAFKA_IMAGE,
        ):
            self.assertIn("@sha256:", image)

    def test_names_are_deterministic_scoped_and_complete(self) -> None:
        module = load_module()
        first = module.names("run-a")
        self.assertEqual(first, module.names("run-a"))
        self.assertNotEqual(first, module.names("run-b"))
        self.assertEqual(len(first), 5)
        self.assertTrue(all(name.startswith("codex-alert-kafka-receipt-") for name in first))

    def test_empty_run_id_is_rejected(self) -> None:
        module = load_module()
        with self.assertRaisesRegex(ValueError, "run_id is required"):
            module.names(" ")

    def test_bootstrap_is_canonical_and_standalone(self) -> None:
        module = load_module()
        sql, schema_hash = module.clickhouse_bootstrap()
        text = sql.decode()
        self.assertEqual(len(schema_hash), 64)
        self.assertIn("CREATE TABLE traffic.alerts", text)
        self.assertIn("CREATE TABLE traffic.alerts_latest AS traffic.alerts", text)
        self.assertNotIn(" ON CLUSTER ", text)
        self.assertNotIn("Replicated", text)

    def test_runner_invokes_production_consumer_and_failure_barrier(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("TestAlertProjectionReceiptRealKafka", source)
        self.assertIn("TestWriteBatchAppliedReceiptFailureBlocksCommit", source)
        self.assertIn('"postgres_receipt_failure_offset_retained_verified": False', source)
        self.assertIn('"same_group_restart_redelivery_verified": False', source)
        self.assertIn('"retry_cross_store_convergence_verified": False', source)
        self.assertIn('"persistent_volume_attached": False', source)
        self.assertIn('"shared_environment_touched": False', source)
        self.assertIn('"production_applied": False', source)


if __name__ == "__main__":
    unittest.main()

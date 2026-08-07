import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_alert_projection_kafka_five_store_ephemeral.py"
DEDUP_SOURCE = ROOT / "go/control-plane/internal/alert/dedup/redis_dedup.go"
FINGERPRINT_SOURCE = ROOT / "go/control-plane/internal/alert/dedup/fingerprint.go"
INTEGRATION_TEST = ROOT / "go/control-plane/internal/alert/consumer/projection_receipt_real_kafka_integration_test.go"
CONSUMER_SOURCE = ROOT / "go/control-plane/internal/alert/consumer/kafka_consumer.go"


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
        self.assertIn('"redis_exact_event_replay_count_stable_verified": False', source)
        self.assertIn('"source_version_hash_stable_across_restart_verified": False', source)
        self.assertIn('"event_identity_collision_rejected_verified": False', source)
        self.assertIn('"source_time_dedup_bucket_verified": False', source)
        self.assertIn('"out_of_order_distinct_events_verified": False', source)
        self.assertIn('"redis_first_last_monotonic_verified": False', source)
        self.assertIn('"delayed_exact_replay_snapshot_verified": False', source)
        self.assertIn('"multi_partition_two_member_rebalance_verified": False', source)
        self.assertIn('"departed_partition_takeover_verified": False', source)
        self.assertIn('"topic_partitions": 2', source)
        self.assertIn('"broker_replicas": 1', source)
        self.assertIn('"persistent_volume_attached": False', source)
        self.assertIn('"shared_environment_touched": False', source)
        self.assertIn('"production_applied": False', source)

    def test_source_time_and_out_of_order_guards_are_production_bound(self) -> None:
        dedup_source = DEDUP_SOURCE.read_text(encoding="utf-8")
        fingerprint_source = FINGERPRINT_SOURCE.read_text(encoding="utf-8")
        integration_source = INTEGRATION_TEST.read_text(encoding="utf-8")
        consumer_source = CONSUMER_SOURCE.read_text(encoding="utf-8")
        self.assertIn("event_id -> {fingerprint,count,first_seen,last_seen}", dedup_source)
        self.assertIn("event_ts < first_seen", dedup_source)
        self.assertIn("event_ts > last_seen", dedup_source)
        self.assertIn("HGET', event_key, 'count'", dedup_source)
        self.assertIn("eventTime := detectionEventTime(batch)", fingerprint_source)
        self.assertIn("PASS_ALERT_PROJECTION_OUT_OF_ORDER_DISTINCT_EVENTS", integration_source)
        self.assertIn("PASS_ALERT_PROJECTION_MULTI_PARTITION_REBALANCE", integration_source)
        self.assertIn("waitForAlertProjectionGroupAssignments", integration_source)
        self.assertIn("waitForAlertProjectionOffset", integration_source)
        self.assertIn("canonicalDetectionEventMillis(&detection)", consumer_source)
        self.assertIn("source event timestamp is required", consumer_source)


if __name__ == "__main__":
    unittest.main()

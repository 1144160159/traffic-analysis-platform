import importlib.util
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/verify_dashboard_task_real_components_ephemeral.py"
GO_TEST = ROOT / "go/control-plane/internal/alert/api/dashboard_task_real_components_integration_test.go"
PROFILE_GO_TEST = ROOT / "go/control-plane/internal/alert/api/dashboard_task_bounded_profile_integration_test.go"


def load_module():
    spec = importlib.util.spec_from_file_location("dashboard_real_components", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class DashboardTaskRealComponentsGuardTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_module()
        cls.script = SCRIPT.read_text(encoding="utf-8")
        cls.go_test = GO_TEST.read_text(encoding="utf-8")
        cls.profile_go_test = PROFILE_GO_TEST.read_text(encoding="utf-8")

    def test_images_are_immutable_and_names_are_run_scoped(self):
        self.assertIn("postgres@sha256:", self.module.POSTGRES_IMAGE)
        self.assertIn("redpandadata/redpanda@sha256:", self.module.KAFKA_IMAGE)
        first = self.module.names("dashboard-real-components-a")
        second = self.module.names("dashboard-real-components-b")
        self.assertNotEqual(first, second)
        self.assertTrue(all(name.startswith("codex-dashboard-real-components-") for name in first))

    def test_runner_is_loopback_volume_free_and_cleans_both_containers(self):
        self.assertIn('"127.0.0.1"', self.script)
        self.assertIn('"-p", f"127.0.0.1:{kafka_port}:19092"', self.script)
        self.assertIn('"-p", "127.0.0.1::5432"', self.script)
        self.assertNotIn('"--volume"', self.script)
        self.assertNotIn('"--mount"', self.script)
        self.assertIn('run(["docker", "rm", "-f", postgres_container]', self.script)
        self.assertIn('run(["docker", "rm", "-f", kafka_container]', self.script)
        self.assertIn('"shared_environment_touched": False', self.script)
        self.assertIn('"production_applied": False', self.script)

    def test_go_test_requires_sentinels_and_exercises_real_boundaries(self):
        self.assertIn("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_PG_DSN", self.go_test)
        self.assertIn("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_KAFKA_BROKER", self.go_test)
        self.assertIn("codex_ephemeral_dashboard_task_real_components_sentinel", self.go_test)
        self.assertIn("commonkafka.NewProducer", self.go_test)
        self.assertIn("commonkafka.NewConsumer", self.go_test)
        self.assertIn("NewHTTPDashboardTaskExecutor", self.go_test)
        self.assertIn("NewHTTPDashboardTaskCompensator", self.go_test)
        self.assertIn("not backed by terminal PostgreSQL authority", self.go_test)
        self.assertIn("restart the real consumer group", self.go_test.lower())
        self.assertIn("ConfigureAuthorityLookup", self.go_test)
        self.assertIn('"receipt_found"', self.go_test)
        self.assertIn("authority lookup was not atomically audited", self.go_test)
        self.assertIn("DLQ acknowledgement failure retains source offset", self.go_test)
        self.assertIn("dashboard_task_dlq_receipts", self.go_test)
        self.assertIn("DASHBOARD_TASK_EVENT_QUARANTINED", self.go_test)
        self.assertIn("DecodeDLQMessage", self.go_test)
        self.assertIn("SetDLQAcknowledgementBarrier", self.go_test)

    def test_runner_records_the_exact_test_and_non_secret_sumdb_override(self):
        self.assertIn('test_name = "TestDashboardTaskRealComponents"', self.script)
        self.assertIn('test_name = "TestDashboardTaskBoundedPerformanceProfile"', self.script)
        self.assertIn('"-run", f"^{test_name}$"', self.script)
        self.assertIn('test_env["GOSUMDB"]', self.script)
        self.assertIn('"secrets_captured": False', self.script)
        self.assertNotIn("DASHBOARD_TASK_PROVIDER_TOKEN", self.script)
        self.assertEqual("dlq.v1", self.module.DLQ_TOPIC)
        self.assertIn('"poison_message_dlq_ack_verified": False', self.script)
        self.assertIn('"source_offset_dlq_postgres_audit_verified": False', self.script)

    def test_bounded_profile_is_sentinel_guarded_and_not_a_production_slo(self):
        self.assertIn("DASHBOARD_TASK_REAL_COMPONENTS_EPHEMERAL_SENTINEL", self.profile_go_test)
        self.assertIn("DASHBOARD_TASK_BOUNDED_PROFILE_RESULT", self.profile_go_test)
        self.assertIn("OWNED_PREFLIGHT_CEILING_NOT_PRODUCTION_SLO", self.profile_go_test)
        self.assertIn("approved_release_candidate", self.profile_go_test)
        self.assertRegex(
            self.profile_go_test.lower(), r'"production_applied"\s*:\s*false'
        )
        self.assertIn("os.O_EXCL", self.profile_go_test)

    def test_bounded_profile_has_fault_resource_and_lag_stop_conditions(self):
        for marker in (
            "retry_amplification_bounded",
            "kafka_final_lag_zero",
            "heap_growth_below_owned_ceiling",
            "goroutine_growth_below_owned_ceiling",
            "end_to_end_p99_below_owned_ceiling",
            "consumer_cold_start_below_owned_ceiling",
            "unknown_timeout_with_external_effect",
        ):
            self.assertIn(marker, self.profile_go_test)
        self.assertIn('"docker", "stats", "--no-stream"', self.script)
        self.assertIn("post_workload_single_snapshot_not_capacity_evidence", self.script)
        self.assertEqual("512m", self.module.POSTGRES_MEMORY_LIMIT)
        self.assertEqual("1g", self.module.REDPANDA_MEMORY_LIMIT)
        self.assertIn('"--memory", REDPANDA_MEMORY_LIMIT', self.script)
        self.assertIn('"--memory", POSTGRES_MEMORY_LIMIT', self.script)
        self.assertIn('"--smp", "1", "--memory", "512M", "--reserve-memory", "0M"', self.script)


if __name__ == "__main__":
    unittest.main()

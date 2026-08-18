import json
import re
import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/clickhouse/four-source-facts.v1.json"


class M06FourSourceFactsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
        cls.ddl = (ROOT / cls.contract["migration"]).read_text(encoding="utf-8")

    def test_four_tables_have_exact_common_contract(self) -> None:
        self.assertFalse(self.contract["production_applied"])
        self.assertEqual(4, len(self.contract["tables"]))
        for table in self.contract["tables"].values():
            local = table + "_local"
            self.assertIn(f"CREATE TABLE IF NOT EXISTS {local} (", self.ddl)
            block = self.ddl.split(
                f"CREATE TABLE IF NOT EXISTS {local} (", 1
            )[1].split(")\nENGINE", 1)[0]
            for column in self.contract["common_columns"]:
                self.assertRegex(block, rf"(?m)^\s*{re.escape(column)}\s+")
            self.assertIn(f"CREATE TABLE IF NOT EXISTS {table}\n", self.ddl)
        self.assertNotRegex(
            re.sub(r"--[^\n]*", "", self.ddl), r"\bON\s+CLUSTER\b"
        )

    def test_source_fact_sink_enforces_version_hash_and_checkpoint_barrier(self) -> None:
        sink = (ROOT / "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/sourcefact/SourceFactClickHouseSink.java").read_text()
        record = (ROOT / "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/sourcefact/SourceFactRecord.java").read_text()
        self.assertIn("implements CheckpointedFunction", sink)
        self.assertIn("snapshotState", sink)
        self.assertIn("stale source-fact version", sink)
        self.assertIn("same source-fact version already has another hash", sink)
        self.assertIn("payloadBase64", record)
        self.assertIn("sourceQualityReceiptId", record)

    def test_all_four_writers_are_wired_but_default_off(self) -> None:
        flow = (ROOT / "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/SessionJob.java").read_text()
        log = (ROOT / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/LogJob.java").read_text()
        user = (ROOT / "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserBehaviorJob.java").read_text()
        asset = (ROOT / "go/control-plane/internal/asset/consumer/asset_projection_targets.go").read_text()
        self.assertIn("toFlowSourceFact", flow)
        self.assertIn("toDeviceLogSourceFact", log)
        self.assertIn("toUserSourceFact", user)
        self.assertIn("type ClickHouseAssetProjection struct", asset)

        session = yaml.safe_load((ROOT / "deployments/kubernetes/flink/flink-session-job.yaml").read_text())
        self.assertIn("--source.fact.sink.enabled", session["spec"]["template"]["spec"]["containers"][0]["args"])
        for path, name in (
            ("deployments/kubernetes/flink/flink-log-job.yaml", "SOURCE_FACT_WRITES_ENABLED"),
            ("deployments/kubernetes/flink/flink-user-behavior-job.yaml", "SOURCE_FACT_WRITES_ENABLED"),
        ):
            manifest = yaml.safe_load((ROOT / path).read_text())
            env = {item["name"]: item for item in manifest["spec"]["template"]["spec"]["containers"][0]["env"]}
            self.assertEqual("false", env[name]["value"])
        services = list(yaml.safe_load_all((ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text()))
        asset_deployment = next(item for item in services if item and item.get("kind") == "Deployment" and item["metadata"]["name"] == "asset-service")
        env = {item["name"]: item for item in asset_deployment["spec"]["template"]["spec"]["containers"][0]["env"]}
        self.assertEqual("false", env["ASSET_PROJECTION_CLICKHOUSE_ENABLED"]["value"])

    def test_kubernetes_migration_job_embeds_the_authoritative_runner(self) -> None:
        migration = self.contract["kubernetes_migration"]
        self.assertFalse(migration["production_applied"])
        manifest = (ROOT / migration["manifest"]).read_text(encoding="utf-8")
        self.assertIn("kind: ConfigMap", manifest)
        self.assertIn("kind: Job", manifest)
        self.assertIn("run-migrations.sh: |-", manifest)
        self.assertIn(Path(self.contract["migration"]).name + ": |-", manifest)
        self.assertIn("traffic.sessions_local", manifest)

    def test_opensearch_targets_are_deterministic_and_versioned(self) -> None:
        asset = (ROOT / "go/control-plane/internal/asset/consumer/asset_projection_targets.go").read_text()
        device = (ROOT / "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/OpenSearchSinkFactory.java").read_text()
        self.assertIn('DocumentID:  event.AssetID', asset)
        self.assertIn('VersionType: "external_gte"', asset)
        self.assertIn("deterministicTargetId", device)
        self.assertIn("VersionType.EXTERNAL_GTE", device)
        self.assertIn("getOffset() + 1L", device)


if __name__ == "__main__":
    unittest.main()

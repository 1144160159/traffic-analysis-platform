import hashlib
import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_clickhouse_schema_authority import verify  # noqa: E402


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    contract_relative = Path("contracts/clickhouse/schema-authority.v1.json")
    contract = json.loads((ROOT / contract_relative).read_text(encoding="utf-8"))
    required = [
        contract_relative.as_posix(),
        contract["migration_runner"],
        *[item["path"] for item in contract["legacy_schema_sources"]],
    ]
    required.extend(
        path.relative_to(ROOT).as_posix()
        for path in (ROOT / contract["authoritative_migration_directory"]).glob("*.sql")
    )
    for relative in required:
        source = ROOT / relative
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
    return candidate


def make_fake_client(directory: Path) -> Path:
    fake = directory / "fake-clickhouse-client"
    fake.write_text(
        """#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' \"$*\" >> \"$FAKE_CLICKHOUSE_LOG\"
case \"$*\" in
  *\"EXISTS TABLE traffic.alignment_schema_migrations_local\"*)
    printf '%s\\n' \"${FAKE_EXISTS:-0}\"
    ;;
  *\"SELECT argMax(checksum, applied_at)\"*)
    printf '%s\\n' \"${FAKE_CHECKSUM:-}\"
    ;;
  *\"--multiquery\"*)
    cat >/dev/null
    ;;
esac
""",
        encoding="utf-8",
    )
    fake.chmod(0o755)
    return fake


class ClickHouseSchemaAuthorityTest(unittest.TestCase):
    def test_authority_and_legacy_inventory_pass_without_live_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        migration_paths = {item["path"] for item in result["migration_inventory"]}
        self.assertEqual(7, result["migration_count"])
        self.assertIn(
            "deployments/clickhouse/migrations/202608031600_sessions_daily_rollup_v1.sql",
            migration_paths,
        )
        self.assertIn(
            "deployments/clickhouse/migrations/202608041300_alert_trace_correlation_v1.sql",
            migration_paths,
        )
        self.assertEqual(12, result["legacy_source_count"])
        self.assertTrue(result["legacy_drift_detected"])
        self.assertFalse(result["production_applied"])

    def test_on_cluster_migration_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "deployments/clickhouse/migrations/202608031000_user_anomalies_v2.sql"
            path.write_text(
                path.read_text(encoding="utf-8").replace(
                    "CREATE TABLE IF NOT EXISTS traffic.user_anomalies_v2_local (",
                    "CREATE TABLE IF NOT EXISTS traffic.user_anomalies_v2_local ON CLUSTER traffic_cluster (",
                ),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("contains ON CLUSTER" in error for error in result["errors"]))

    def test_unregistered_legacy_source_change_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "common/sql/ch/01-extended.sql"
            path.write_text(path.read_text(encoding="utf-8") + "\n-- changed\n", encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("changed without inventory update" in error for error in result["errors"]))

    def test_contract_cannot_claim_closed_or_production_applied(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / "contracts/clickhouse/schema-authority.v1.json"
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim T-CH-001 closure" in error for error in result["errors"]))
            self.assertTrue(any("must not claim production apply" in error for error in result["errors"]))

    def test_runner_applies_and_records_checksum(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            migrations = temp / "migrations"
            migrations.mkdir()
            migration = migrations / "202607300000_schema_authority.sql"
            migration.write_text("SELECT 1;\n", encoding="utf-8")
            log = temp / "client.log"
            fake = make_fake_client(temp)
            env = os.environ.copy()
            env.update(
                {
                    "CLICKHOUSE_PASSWORD": "",
                    "CLICKHOUSE_HOSTS": "ch-a ch-b",
                    "CLICKHOUSE_MIGRATIONS": str(migrations),
                    "CLICKHOUSE_CLIENT": str(fake),
                    "FAKE_CLICKHOUSE_LOG": str(log),
                }
            )
            result = subprocess.run(
                [str(ROOT / "scripts/clickhouse/run-migrations.sh")],
                cwd=ROOT,
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(0, result.returncode, result.stderr)
            self.assertEqual(2, result.stdout.count("applying 202607300000_schema_authority.sql"))
            client_log = log.read_text(encoding="utf-8")
            self.assertEqual(2, client_log.count("INSERT INTO traffic.alignment_schema_migrations_local"))

    def test_runner_rejects_applied_checksum_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            migrations = temp / "migrations"
            migrations.mkdir()
            migration = migrations / "202607300000_schema_authority.sql"
            migration.write_text("SELECT 1;\n", encoding="utf-8")
            log = temp / "client.log"
            fake = make_fake_client(temp)
            env = os.environ.copy()
            env.update(
                {
                    "CLICKHOUSE_PASSWORD": "",
                    "CLICKHOUSE_MIGRATIONS": str(migrations),
                    "CLICKHOUSE_CLIENT": str(fake),
                    "FAKE_CLICKHOUSE_LOG": str(log),
                    "FAKE_EXISTS": "1",
                    "FAKE_CHECKSUM": hashlib.sha256(b"different\n").hexdigest(),
                }
            )
            result = subprocess.run(
                [str(ROOT / "scripts/clickhouse/run-migrations.sh")],
                cwd=ROOT,
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(3, result.returncode)
            self.assertIn("Checksum mismatch for applied migration", result.stderr)


if __name__ == "__main__":
    unittest.main()

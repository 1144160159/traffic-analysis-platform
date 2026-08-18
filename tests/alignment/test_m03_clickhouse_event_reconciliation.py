import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from reconcile_m03_clickhouse_events import (  # noqa: E402
    TABLES,
    ClickHouseHttpClient,
    detail_sql,
    grouped_sql,
    reconcile,
)


class FakeClient:
    def __init__(self, summaries, details=None):
        self.summaries = summaries
        self.details = details or {}
        self.calls = []

    def query_json(self, sql, parameters):
        self.calls.append((sql, parameters))
        table = next(name for name in TABLES if f"traffic.{name}" in sql)
        if "SELECT event_id, copies, payload_versions" in sql:
            return self.details.get(table, [])
        return [self.summaries[table]]


def summary(rows=1, unique=1, duplicates=0, conflicts=0, blank=0):
    return {
        "physical_rows": rows,
        "unique_events": unique,
        "duplicate_rows": duplicates,
        "conflicting_events": conflicts,
        "blank_event_id_rows": blank,
    }


class M03ClickHouseEventReconciliationTest(unittest.TestCase):
    def test_sql_is_parameterized_and_hashes_business_payload(self):
        sql = grouped_sql("traffic", "feature_fp")
        self.assertIn("tenant_id = {tenant:String}", sql)
        self.assertIn("run_id = {run:String}", sql)
        self.assertIn("sipHash64(tuple(", sql)
        self.assertIn("transport_security", sql)
        self.assertNotIn("ingest_ts", sql)
        self.assertNotIn("tenant-a", sql)
        details = detail_sql("traffic", "feature_fp", 17)
        self.assertIn("LIMIT 17", details)
        self.assertIn("payload_versions > 1", details)

    def test_clean_nonempty_core_and_optional_empty_tables_pass(self):
        summaries = {name: summary() for name in TABLES}
        summaries["feature_seq"] = summary(0, 0)
        summaries["feature_fp"] = summary(0, 0)
        client = FakeClient(summaries)
        result = reconcile(client, database="traffic", tenant_id="tenant-a", run_id="run-a")
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(len(TABLES), len(client.calls))
        self.assertTrue(all(call[1] == {"tenant": "tenant-a", "run": "run-a"} for call in client.calls))

    def test_empty_required_projection_fails(self):
        summaries = {name: summary() for name in TABLES}
        summaries["sessions"] = summary(0, 0)
        result = reconcile(FakeClient(summaries), database="traffic", tenant_id="t", run_id="r")
        self.assertEqual("FAIL", result["status"])
        self.assertIn("sessions: required projection is empty", result["errors"])

    def test_duplicate_and_conflicting_payload_fetch_key_details(self):
        summaries = {name: summary() for name in TABLES}
        summaries["feature_stat"] = summary(3, 1, duplicates=2, conflicts=1)
        details = {"feature_stat": [{"event_id": "event-1", "copies": 3, "payload_versions": 2}]}
        result = reconcile(
            FakeClient(summaries, details), database="traffic", tenant_id="t", run_id="r"
        )
        table = next(item for item in result["tables"] if item["table"] == "feature_stat")
        self.assertEqual("FAIL", result["status"])
        self.assertEqual(details["feature_stat"], table["details"])
        self.assertIn("feature_stat: physical duplicate event_id rows are present", result["errors"])
        self.assertIn("feature_stat: event_id payload conflicts are present", result["errors"])

    def test_identifiers_credentials_and_required_tables_are_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "invalid ClickHouse database"):
            grouped_sql("traffic; DROP TABLE x", "flows_raw")
        with self.assertRaisesRegex(ValueError, "credentials.*forbidden"):
            ClickHouseHttpClient("https://user:secret@example.test", "u", "p", "traffic")
        with self.assertRaisesRegex(ValueError, "unknown required tables"):
            reconcile(
                FakeClient({name: summary() for name in TABLES}),
                database="traffic", tenant_id="t", run_id="r", required_nonempty={"unknown"},
            )


if __name__ == "__main__":
    unittest.main()

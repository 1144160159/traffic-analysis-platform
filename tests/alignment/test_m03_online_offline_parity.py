import copy
import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from reconcile_m03_clickhouse_events import TABLES  # noqa: E402
from run_m03_online_offline_parity import (  # noqa: E402
    compare_table,
    projection_sql,
    run_parity,
    validate_contract,
    validate_corpus,
    validate_receipts,
)


CONTRACT_PATH = ROOT / "contracts/flink/m03-online-offline-parity.v1.json"
CORPUS_PATH = ROOT / "tests/fixtures/pcap/m03/manifest.v1.json"


def contract():
    return json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))


def receipt(route, corpus, run_id, offset=0):
    return {
        "schema_version": 1,
        "route": route,
        "tenant_id": "tenant-a",
        "probe_id": "probe-a",
        "run_id": run_id,
        "candidate_sha256": "a" * 64,
        "corpus_id": corpus["corpus_id"],
        "corpus_manifest_sha256": corpus["manifest_sha256"],
        "capture_mode": "af_packet" if route == "online" else "pcap_offline",
        "packet_count_received": corpus["packet_count"],
        "packet_count_dropped": 0,
        "event_time_offset_ms": offset,
        "source_completed": True,
        "pipeline_drained": True,
        "final_checkpoint_id": 42,
    }


def value_for(field):
    if field in {"source_event_ids", "evidence_ids", "flow_ids", "missing_fields"}:
        return ["lineage-1"]
    if field in {"extra", "hex_freq", "hex_ratio"}:
        return [1.0, 2.0]
    if field.endswith("_id") or field in {
        "community_id", "probe_id", "src_ip", "dst_ip", "direction", "feature_set_id",
        "end_reason", "event_schema_version", "identity_version", "completeness",
        "schema_version", "object_type", "feature_category", "availability",
        "algorithm_version", "value_unit", "missing_reason", "tls_version", "ja3", "ja4",
        "sni", "sni_hash", "cert_sha256", "quic_version", "transport_security",
        "pktlen_seq_hash", "iat_seq_hash", "seq_blob_ref", "raw_traffic_ref",
    }:
        return f"v-{field}"
    return 10


def projection_row(spec, event_id="event-a"):
    fields = {event_id and "event_id" or "event_id": event_id}
    for group in ("key_fields", "exact_fields", "cardinality_fields", "presence_fields"):
        for field in spec.get(group, []):
            fields[field] = value_for(field)
    for field in spec.get("tolerances", {}):
        fields[field] = value_for(field)
    return fields


class FakeClickHouse:
    def __init__(self, contract_value, rows_by_route=None, duplicate_run=None):
        self.contract = contract_value
        self.rows_by_route = rows_by_route or {
            route: {table: [projection_row(spec, f"{route}-{table}")] for table, spec in contract_value["tables"].items()}
            for route in ("online", "offline")
        }
        self.duplicate_run = duplicate_run

    def query_json(self, sql, parameters):
        table = next(name for name in TABLES if f"traffic.{name}" in sql)
        route = "online" if parameters["run"] == "online-run" else "offline"
        if "sum(copies) AS physical_rows" in sql:
            rows = self.rows_by_route[route][table]
            duplicate = parameters["run"] == self.duplicate_run and table == "sessions"
            return [{
                "physical_rows": len(rows) + (1 if duplicate else 0),
                "unique_events": len(rows),
                "duplicate_rows": 1 if duplicate else 0,
                "conflicting_events": 0,
                "blank_event_id_rows": 0,
            }]
        if "SELECT event_id, copies, payload_versions" in sql:
            return [{"event_id": "dup", "copies": 2, "payload_versions": 1}]
        return copy.deepcopy(self.rows_by_route[route][table])


class M03OnlineOfflineParityTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.contract = contract()
        cls.corpus = validate_corpus(CORPUS_PATH)

    def test_contract_has_exact_column_coverage_and_parameterized_queries(self):
        validate_contract(self.contract)
        for table, spec in self.contract["tables"].items():
            sql = projection_sql("traffic", table, spec)
            self.assertIn("tenant_id = {tenant:String}", sql)
            self.assertIn("run_id = {run:String}", sql)
            self.assertNotIn("ingest_ts", sql)
        mutated = copy.deepcopy(self.contract)
        mutated["tables"]["feature_stat"]["exact_fields"].remove("feature_set_id")
        with self.assertRaisesRegex(ValueError, "field coverage drift"):
            validate_contract(mutated)

    def test_corpus_hash_and_path_escape_are_rejected(self):
        corpus = validate_corpus(CORPUS_PATH)
        self.assertEqual("m03-pcap-golden-v1", corpus["corpus_id"])
        self.assertEqual(150, corpus["packet_count"])
        manifest = json.loads(CORPUS_PATH.read_text(encoding="utf-8"))
        manifest["fixtures"][0]["relative_path"] = "../m03/../escape.pcap"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "manifest.json"
            path.write_text(json.dumps(manifest), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "escapes corpus or does not exist"):
                validate_corpus(path)

    def test_receipts_require_two_routes_same_candidate_probe_and_no_drops(self):
        online = receipt("online", self.corpus, "online-run")
        offline = receipt("offline", self.corpus, "offline-run")
        scope = validate_receipts(online, offline, self.corpus)
        self.assertEqual("probe-a", scope["probe_id"])
        mutated = copy.deepcopy(offline)
        mutated["run_id"] = "online-run"
        with self.assertRaisesRegex(ValueError, "self-comparison"):
            validate_receipts(online, mutated, self.corpus)
        mutated = copy.deepcopy(offline)
        mutated["packet_count_dropped"] = 1
        with self.assertRaisesRegex(ValueError, "capture drops"):
            validate_receipts(online, mutated, self.corpus)
        mutated = copy.deepcopy(offline)
        mutated["probe_id"] = "probe-b"
        with self.assertRaisesRegex(ValueError, "probe identities"):
            validate_receipts(online, mutated, self.corpus)

    def test_compare_normalizes_only_declared_time_shift_and_applies_tolerance(self):
        spec = self.contract["tables"]["feature_stat"]
        offline = projection_row(spec, "offline-event")
        online = projection_row(spec, "online-event")
        for field in spec["timestamp_fields"]:
            if online.get(field) is not None:
                online[field] += 1000
        online["pps"] = offline["pps"] + 0.000001
        result = compare_table("feature_stat", spec, [online], [offline], 1000, 0, 100)
        self.assertEqual("PASS", result["status"], result)
        online["pps"] = offline["pps"] + 1.0
        result = compare_table("feature_stat", spec, [online], [offline], 1000, 0, 100)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any(item.get("field") == "pps" for item in result["differences"]))

    def test_full_parity_passes_and_route_duplicates_stop_before_diff(self):
        online = receipt("online", self.corpus, "online-run", offset=1000)
        offline = receipt("offline", self.corpus, "offline-run", offset=0)
        rows = {
            route: {} for route in ("online", "offline")
        }
        for table, spec in self.contract["tables"].items():
            base = projection_row(spec, f"offline-{table}")
            online_row = copy.deepcopy(base)
            online_row["event_id"] = f"online-{table}"
            for field in spec["timestamp_fields"]:
                if online_row.get(field) is not None:
                    online_row[field] += 1000
            rows["online"][table] = [online_row]
            rows["offline"][table] = [base]
        result = run_parity(
            FakeClickHouse(self.contract, rows), database="traffic", contract=self.contract,
            corpus=self.corpus, online_receipt=online, offline_receipt=offline,
        )
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(0, result["difference_count"])
        failed = run_parity(
            FakeClickHouse(self.contract, rows, duplicate_run="online-run"),
            database="traffic", contract=self.contract, corpus=self.corpus,
            online_receipt=online, offline_receipt=offline,
        )
        self.assertEqual("per_route_event_id_reconciliation", failed["phase"])
        self.assertEqual("FAIL", failed["status"])


if __name__ == "__main__":
    unittest.main()

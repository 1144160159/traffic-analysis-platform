import hashlib
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_flink_application_clusters import verify  # noqa: E402


class VerifyFlinkApplicationClustersTest(unittest.TestCase):
    def setUp(self) -> None:
        self.contract = json.loads(
            (ROOT / "contracts/flink/application-cluster-migration.v1.json").read_text(
                encoding="utf-8"
            )
        )
        payload = json.dumps(
            self.contract, ensure_ascii=False, sort_keys=True, separators=(",", ":")
        )
        self.manifest = {
            "schema_version": 1,
            "contract_sha256": hashlib.sha256(payload.encode()).hexdigest(),
            "completed_migration_order": 9,
            "endpoints": {
                job["id"]: {
                    "cluster_id": job["cluster_id"],
                    "endpoint": f"http://{job['cluster_id']}-rest.flink.svc:8081",
                }
                for job in self.contract["jobs"]
            },
        }
        self.jobs = {job["id"]: job for job in self.contract["jobs"]}

    def _fetch(self, endpoint: str, path: str):
        job = next(job for job in self.contract["jobs"] if job["cluster_id"] in endpoint)
        jid = str(job["migration_order"]) * 32
        if path == "/jobs/overview":
            return {
                "jobs": [{
                    "jid": jid,
                    "name": job["job_name"],
                    "state": "RUNNING",
                    "tasks": {"running": job["expected_tasks"], "total": job["expected_tasks"]},
                }]
            }
        if path.endswith("/checkpoints"):
            return {
                "counts": {"restored": 1},
                "latest": {"completed": {
                    "id": 42,
                    "external_path": f"s3://flink-checkpoints/checkpoints/application-clusters/{job['id']}/chk-42",
                }},
            }
        if path.endswith("/exceptions"):
            return {"root_exception": None}
        raise AssertionError(path)

    def test_all_nine_application_clusters_pass_independently(self) -> None:
        result = verify(self.manifest, self._fetch)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(9, result["verified_application_clusters"])
        self.assertEqual(128, result["verified_running_tasks"])

    def test_missing_cluster_and_missing_restore_fail(self) -> None:
        manifest = json.loads(json.dumps(self.manifest))
        manifest["endpoints"].pop("flink-log-job")

        def fetch(endpoint: str, path: str):
            response = self._fetch(endpoint, path)
            if path.endswith("/checkpoints"):
                response["counts"]["restored"] = 0
            return response

        result = verify(manifest, fetch)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("endpoint job ids differ" in item for item in result["errors"]))
        self.assertTrue(any("no restore recorded" in item for item in result["errors"]))

    def test_serial_stage_accepts_only_contiguous_migrated_prefix(self) -> None:
        manifest = json.loads(json.dumps(self.manifest))
        manifest["completed_migration_order"] = 2
        manifest["endpoints"] = {
            job_id: entry for job_id, entry in manifest["endpoints"].items()
            if self.jobs[job_id]["migration_order"] <= 2
        }
        result = verify(manifest, self._fetch)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(2, result["verified_application_clusters"])
        self.assertEqual(12, result["verified_running_tasks"])

        manifest["endpoints"]["flink-session-job"] = self.manifest["endpoints"]["flink-session-job"]
        result = verify(manifest, self._fetch)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("extra=['flink-session-job']" in item for item in result["errors"]))


if __name__ == "__main__":
    unittest.main()

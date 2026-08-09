import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_flink_job_registry import (  # noqa: E402
    APPLICATION_PATH,
    CHECKPOINT_PATH,
    REGISTRY_PATH,
    SINK_PATH,
    STATE_PATH,
    contract_sha256,
    expected_jobs,
    load,
    verify,
)
from build_flink_job_release_registry import build  # noqa: E402


class FlinkJobRegistryTest(unittest.TestCase):
    def setUp(self) -> None:
        self.registry = load(REGISTRY_PATH)
        self.application = load(APPLICATION_PATH)
        self.state = load(STATE_PATH)
        self.checkpoint = load(CHECKPOINT_PATH)
        self.sink = load(SINK_PATH)

    def verify(self, registry=None, release=None, runtime=None):
        return verify(
            registry or self.registry,
            self.application,
            self.state,
            self.checkpoint,
            self.sink,
            release,
            runtime,
        )

    def release_registry(self):
        expected = expected_jobs(self.registry, self.application, self.state)
        jobs = []
        for index, (job_id, fields) in enumerate(expected.items(), start=1):
            item = copy.deepcopy(fields)
            item.update(
                {
                    "artifact_sha256": f"{index:x}" * 64,
                    "image_digest": f"registry.local/traffic/{job_id}@sha256:"
                    + f"{(index + 9) % 16:x}" * 64,
                    "savepoint_uri": f"s3://flink-checkpoints/savepoints/application-clusters/{job_id}/savepoint-test",
                    "savepoint_sha256": f"{index + 1:x}" * 64,
                }
            )
            jobs.append(item)
        return {
            "schema_version": 1,
            "candidate_source_sha256": "a" * 64,
            "contract_sha256": contract_sha256(self.registry),
            "jobs": jobs,
        }

    def runtime_snapshot(self, release):
        fields = set(self.registry["runtime_diff"]["required_fields"])
        return {
            "schema_version": 1,
            "jobs": [
                {key: value for key, value in item.items() if key in fields}
                for item in release["jobs"]
            ],
        }

    def test_repository_registry_is_partial_until_release_and_runtime_are_bound(self):
        result = self.verify()
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("PARTIAL", result["coverage_status"])
        self.assertEqual(9, result["canonical_jobs"])
        self.assertEqual("NOT_PROVIDED", result["release_registry"])
        self.assertEqual("NOT_PROVIDED", result["runtime_diff"])

    def test_matching_release_and_runtime_registries_pass_exact_diff(self):
        release = self.release_registry()
        runtime = self.runtime_snapshot(release)
        result = self.verify(release=release, runtime=runtime)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual("COMPLETE", result["coverage_status"])
        self.assertEqual("PASS", result["release_registry"])
        self.assertEqual("PASS", result["runtime_diff"])

    def test_operator_uid_hash_drift_is_rejected(self):
        candidate = copy.deepcopy(self.registry)
        candidate["jobs"][0]["operator_uid_sha256"] = "0" * 64
        result = self.verify(registry=candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("operator UID hash drifted" in error for error in result["errors"]))

    def test_max_parallelism_drift_is_rejected(self):
        candidate = copy.deepcopy(self.registry)
        candidate["jobs"][2]["max_parallelism"] = 8
        result = self.verify(registry=candidate)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("max_parallelism" in error for error in result["errors"]))

    def test_mutable_release_image_is_rejected(self):
        release = self.release_registry()
        release["jobs"][0]["image_digest"] = "registry.local/traffic/flink:latest"
        result = self.verify(release=release)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("image_digest" in error for error in result["errors"]))

    def test_runtime_parallelism_or_unknown_job_is_rejected(self):
        release = self.release_registry()
        runtime = self.runtime_snapshot(release)
        runtime["jobs"][0]["parallelism"] += 1
        runtime["jobs"].append({"job_id": "unregistered-job"})
        result = self.verify(release=release, runtime=runtime)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any("parallelism mismatch" in error for error in result["errors"]))
        self.assertTrue(any("canonical job set" in error for error in result["errors"]))

    def test_release_builder_binds_artifact_image_savepoint_and_g0_hashes(self):
        job_ids = [item["id"] for item in self.registry["jobs"]]
        images = {
            "schema_version": 1,
            "jobs": [
                {
                    "job_id": job_id,
                    "image_digest": f"registry.local/traffic/{job_id}@sha256:" + f"{index:x}" * 64,
                }
                for index, job_id in enumerate(job_ids, start=1)
            ],
        }
        savepoints = {
            "schema_version": 1,
            "source_cluster_id": self.application["source_session_cluster_id"],
            "savepoints": {
                job_id: {
                    "uri": f"{self.application['state']['savepoint_root']}/{job_id}/savepoint-test",
                    "sha256": f"{index + 1:x}" * 64,
                }
                for index, job_id in enumerate(job_ids, start=1)
            },
        }
        artifacts = {
            "status": "PASS",
            "artifacts": [
                {"job_id": job_id, "sha256": f"{index + 2:x}" * 64}
                for index, job_id in enumerate(job_ids, start=1)
            ],
        }
        g0 = {
            "gate": "G0",
            "status": "PASS",
            "run_id": "test-g0",
            "candidate_source": {"content_sha256": "f" * 64},
        }
        release = build(images, savepoints, g0, artifacts)
        self.assertEqual(9, len(release["jobs"]))
        self.assertEqual("f" * 64, release["candidate_source_sha256"])
        self.assertEqual("PASS", release["validation"]["status"])
        self.assertFalse(release["production_applied"])


if __name__ == "__main__":
    unittest.main()

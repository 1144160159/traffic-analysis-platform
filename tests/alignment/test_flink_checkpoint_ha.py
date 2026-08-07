import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_flink_checkpoint_ha import verify  # noqa: E402


class FlinkCheckpointHaContractTest(unittest.TestCase):
    def test_repository_satisfies_checkpoint_ha_contract(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertEqual(9, result["canonical_jobs"])
        self.assertEqual(2, result["session_jobmanager_replicas"])
        self.assertEqual(2, result["application_jobmanager_replicas"])
        self.assertEqual([], result["hard_coded_latest_sources"])

    def test_single_jobmanager_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            shutil.copytree(ROOT, candidate, ignore=shutil.ignore_patterns(".git", "target", "node_modules"))
            manifest = candidate / "deployments/kubernetes/infrastructure/07-flink.yaml"
            source = manifest.read_text(encoding="utf-8")
            manifest.write_text(source.replace("  replicas: 2", "  replicas: 1", 1), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("at least two replicas" in error for error in result["errors"]))

    def test_latest_source_default_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = Path(directory) / "repo"
            shutil.copytree(ROOT, candidate, ignore=shutil.ignore_patterns(".git", "target", "node_modules"))
            source_file = candidate / "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/FeatureJob.java"
            source = source_file.read_text(encoding="utf-8")
            source_file.write_text(
                source.replace("KafkaStartingOffsets.from(params)", "OffsetsInitializer.latest()", 1),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("hard-code latest" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()

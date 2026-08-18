import importlib.util
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


parity = load_module(
    "m08_model_inference_parity",
    ROOT / "mlops/scripts/model_inference_parity.py",
)
runner = load_module(
    "m08_model_inference_parity_k8s",
    ROOT / "scripts/alignment/run_m08_model_inference_parity_k8s.py",
)


class M08ModelInferenceParityTest(unittest.TestCase):
    def setUp(self) -> None:
        self.profile = json.loads(
            (
                ROOT
                / "contracts/mlops/m08-model-inference-parity-internal.v1.json"
            ).read_text()
        )

    def test_profile_is_internal_only_and_uses_production_feature_order(self) -> None:
        self.assertEqual(self.profile["claim_scope"], "INTERNAL_ENGINEERING_ONLY")
        self.assertFalse(self.profile["cnas_claim_authorized"])
        self.assertFalse(self.profile["production_promotion_authorized"])
        self.assertEqual(
            self.profile["feature_columns"],
            ["bps", "iat_mean_ms", "pktlen_mean", "pps"],
        )
        result_schema = json.loads(
            (
                ROOT / "contracts/mlops/model-inference-parity-result.schema.json"
            ).read_text()
        )
        self.assertEqual(
            result_schema["properties"]["cnas_claim_authorized"]["const"], False
        )
        self.assertEqual(
            result_schema["properties"]["production_promotion_authorized"]["const"],
            False,
        )

    def test_score_comparison_enforces_the_exact_tolerance(self) -> None:
        accepted = parity.compare_scores({"a": 0.5}, {"a": 0.500001}, 0.00001)
        rejected = parity.compare_scores({"a": 0.5}, {"a": 0.51}, 0.00001)
        self.assertEqual(accepted["status"], "PASS")
        self.assertEqual(rejected["status"], "FAIL")
        with self.assertRaisesRegex(ValueError, "different sample sets"):
            parity.compare_scores({"a": 0.5}, {"b": 0.5}, 0.00001)

    def test_route_receipt_rejects_candidate_or_sample_drift(self) -> None:
        digest = "a" * 64
        sample = {"sample_id": "sample-1", "features": {"bps": 1.0}}
        bundle = {
            "run_id": "33333333-3333-4333-8333-333333333333",
            "profile_id": self.profile["profile_id"],
            "profile_sha256": digest,
            "candidate_sha256": digest,
            "model_id": "model-1",
            "model_version": "v1",
            "model_artifact_sha256": digest,
            "feature_columns_sha256": digest,
            "sample_set_sha256": digest,
            "bundle_sha256": digest,
            "samples": [sample] * self.profile["sample_count"],
        }
        receipt = {
            "schema_version": 1,
            "route": "flink",
            **{key: bundle[key] for key in parity.ROUTE_IDENTITY_FIELDS},
            "measured_inferences": self.profile["sample_count"]
            * self.profile["measured_iterations"],
            "latency_ms": {"p50": 1.0, "p95": 1.0, "p99": 1.0, "max": 1.0},
            "throughput_per_second": 100.0,
            "cpu_seconds": 1.0,
            "peak_rss_bytes": 1024,
            "predictions": [
                {"sample_id": f"sample-{index}", "score": 0.5}
                for index in range(self.profile["sample_count"])
            ],
        }
        bundle["samples"] = [
            {"sample_id": f"sample-{index}", "features": {"bps": 1.0}}
            for index in range(self.profile["sample_count"])
        ]
        parity.validate_route_receipt(receipt, "flink", bundle, self.profile)
        receipt["candidate_sha256"] = "b" * 64
        with self.assertRaisesRegex(ValueError, "candidate_sha256"):
            parity.validate_route_receipt(receipt, "flink", bundle, self.profile)

    def test_java_route_is_a_real_flink_datastream_using_production_functions(self) -> None:
        source = (
            ROOT
            / "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelInferenceParityMain.java"
        ).read_text()
        for marker in (
            "StreamExecutionEnvironment.getExecutionEnvironment()",
            "FeatureStatVectorizer.vectorize",
            "GovernedModelPackageLoader.runBaseline",
            "executeAndCollect",
            "refusing to overwrite immutable Flink parity receipt",
        ):
            self.assertIn(marker, source)

    def test_onnx_export_has_a_deterministic_graph_identity(self) -> None:
        source = (ROOT / "mlops/scripts/model_inference_parity.py").read_text()
        self.assertIn(
            'converted.graph.name = "m08-model-inference-parity-v1"', source
        )

    def test_kubernetes_job_is_isolated_and_sequential(self) -> None:
        run_id = "33333333-3333-4333-8333-333333333333"
        suffix = runner.validate_inputs(
            "traffic/mlops-trainer:m08-governed-export-w2c-20260815-r10",
            "traffic/flink-behavior-job:m08-parity-r1",
            "a" * 64,
            run_id,
            "8-2tb",
        )
        objects = runner.build_objects(
            runner.names(suffix),
            "traffic/mlops-trainer:m08-governed-export-w2c-20260815-r10",
            "traffic/flink-behavior-job:m08-parity-r1",
            "a" * 64,
            run_id,
            "8-2tb",
        )
        job = objects[1]
        annotations = job["metadata"]["annotations"]
        for store in ("postgres", "clickhouse", "kafka", "flink"):
            self.assertEqual(annotations[f"traffic.analysis/shared-{store}-touched"], "false")
        spec = job["spec"]["template"]["spec"]
        self.assertFalse(spec["automountServiceAccountToken"])
        self.assertEqual(
            [container["name"] for container in spec["initContainers"]],
            ["prepare-python-and-onnx", "run-flink-route"],
        )
        self.assertTrue(any("emptyDir" in volume for volume in spec["volumes"]))


if __name__ == "__main__":
    unittest.main()

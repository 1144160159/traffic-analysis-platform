#!/usr/bin/env python3
"""Prepare and evaluate the bounded M08 Python/ONNX/Flink parity profile.

This tool is intentionally an internal engineering gate.  It creates a
deterministic synthetic model and sample set, records Python and ONNX Runtime
receipts, then validates a receipt emitted by the production Java ONNX parser
inside a Flink DataStream job.  It cannot authorize activation, promotion, or
CNAS quality claims.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import platform
import resource
import time
import uuid
from pathlib import Path
from typing import Any, Callable

import numpy as np


PARITY_NAMESPACE = uuid.UUID("a6fd303f-2158-5706-b8bb-42f58717d691")
SHA256_PATTERN = "0123456789abcdef"


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(
        value, sort_keys=True, separators=(",", ":"), ensure_ascii=False,
        allow_nan=False,
    ).encode("utf-8")


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(canonical_bytes(value)).hexdigest()


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def load_json(path: Path, description: str) -> dict[str, Any]:
    if not path.is_file():
        raise ValueError(f"{description} is missing: {path}")
    with path.open("r", encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError(f"{description} must be a JSON object")
    return value


def write_json_exclusive(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = json.dumps(
        value, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False,
    ) + "\n"
    with path.open("x", encoding="utf-8") as handle:
        handle.write(payload)
        handle.flush()
        os.fsync(handle.fileno())


def validate_sha256(value: str, name: str) -> None:
    if len(value) != 64 or any(char not in SHA256_PATTERN for char in value):
        raise ValueError(f"{name} must be a lowercase SHA-256")


def validate_profile(profile: dict[str, Any], schema: dict[str, Any]) -> None:
    from jsonschema import Draft202012Validator, FormatChecker

    Draft202012Validator.check_schema(schema)
    Draft202012Validator(schema, format_checker=FormatChecker()).validate(profile)
    if profile["claim_scope"] != "INTERNAL_ENGINEERING_ONLY" or \
            profile["cnas_claim_authorized"] is not False or \
            profile["production_promotion_authorized"] is not False:
        raise ValueError("parity profile attempted to widen its internal-only claim boundary")


def positive_probability(outputs: list[Any], row_count: int) -> np.ndarray:
    for value in outputs:
        if isinstance(value, list) and len(value) == row_count and all(
            isinstance(item, dict) for item in value
        ):
            return np.asarray(
                [float(item.get(1, item.get("1"))) for item in value], dtype=float,
            )
        array = np.asarray(value)
        if array.ndim == 2 and array.shape == (row_count, 2) and \
                np.issubdtype(array.dtype, np.number):
            return array[:, 1].astype(float)
        if array.ndim == 1 and array.shape == (row_count,) and \
                np.issubdtype(array.dtype, np.floating):
            if np.isfinite(array).all() and ((array >= 0) & (array <= 1)).all():
                return array.astype(float)
    raise ValueError("ONNX output does not expose an aligned binary probability")


def deterministic_matrix(seed: int, rows: int) -> np.ndarray:
    rng = np.random.default_rng(seed)
    bps = rng.uniform(1_000.0, 8_000_000.0, rows)
    iat = rng.uniform(0.1, 2_000.0, rows)
    pktlen = rng.uniform(40.0, 1_500.0, rows)
    pps = rng.uniform(0.1, 8_000.0, rows)
    return np.column_stack((bps, iat, pktlen, pps)).astype(np.float32)


def deterministic_labels(matrix: np.ndarray) -> np.ndarray:
    risk = (
        matrix[:, 0] / 8_000_000.0
        - matrix[:, 1] / 2_000.0
        + (300.0 - matrix[:, 2]) / 1_500.0
        + matrix[:, 3] / 8_000.0
    )
    return (risk > float(np.median(risk))).astype(np.int32)


def train_and_export(seed: int, feature_count: int, workdir: Path) -> tuple[Any, Path, Path]:
    import onnx
    import xgboost as xgb
    from onnxmltools import convert_xgboost
    from onnxmltools.convert.common.data_types import FloatTensorType

    train = deterministic_matrix(seed ^ 0x5A5A5A5A, 512)
    if train.shape[1] != feature_count:
        raise ValueError("profile feature count differs from the fixed FeatureStat parity matrix")
    labels = deterministic_labels(train)
    model = xgb.XGBClassifier(
        n_estimators=32,
        max_depth=3,
        learning_rate=0.15,
        subsample=1.0,
        colsample_bytree=1.0,
        random_state=seed,
        n_jobs=1,
        eval_metric="logloss",
    )
    model.fit(train, labels)
    native_path = workdir / "python-model.json"
    model.save_model(native_path)

    converted_model = xgb.XGBClassifier()
    converted_model.load_model(native_path)
    converted_model.get_booster().feature_names = None
    converted_model.get_booster().feature_types = None
    converted = convert_xgboost(
        converted_model,
        initial_types=[("float_input", FloatTensorType([None, feature_count]))],
        target_opset=15,
    )
    # onnxmltools otherwise assigns a random UUID-like graph name, making the
    # immutable ONNX artifact drift even when weights and inputs are identical.
    converted.graph.name = "m08-model-inference-parity-v1"
    onnx.checker.check_model(converted)
    onnx_path = workdir / "baseline-model.onnx"
    with onnx_path.open("xb") as handle:
        handle.write(converted.SerializeToString())
        handle.flush()
        os.fsync(handle.fileno())
    return model, native_path, onnx_path


def percentile(values: list[float], quantile: float) -> float:
    if not values:
        raise ValueError("latency sample is empty")
    return float(np.percentile(np.asarray(values), quantile, method="higher"))


def peak_rss_bytes() -> int:
    value = int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss)
    # ru_maxrss is KiB on Linux and bytes on macOS. Kubernetes evidence is Linux.
    return value if platform.system() == "Darwin" else value * 1024


def benchmark_route(
    route: str,
    engine: str,
    engine_version: str,
    predict_one: Callable[[np.ndarray], float],
    samples: list[dict[str, Any]],
    matrix: np.ndarray,
    warmup_iterations: int,
    measured_iterations: int,
    identity: dict[str, Any],
) -> dict[str, Any]:
    for _ in range(warmup_iterations):
        for row in matrix:
            score = float(predict_one(row))
            if not math.isfinite(score) or score < 0.0 or score > 1.0:
                raise ValueError(f"{route} warmup produced an invalid probability")

    latencies: list[float] = []
    scores_by_sample: dict[str, list[float]] = {
        sample["sample_id"]: [] for sample in samples
    }
    wall_started = time.perf_counter_ns()
    cpu_started = time.process_time_ns()
    for _ in range(measured_iterations):
        for sample, row in zip(samples, matrix, strict=True):
            started = time.perf_counter_ns()
            score = float(predict_one(row))
            elapsed = time.perf_counter_ns() - started
            if not math.isfinite(score) or score < 0.0 or score > 1.0:
                raise ValueError(f"{route} produced an invalid probability")
            latencies.append(elapsed / 1_000_000.0)
            scores_by_sample[sample["sample_id"]].append(score)
    cpu_elapsed = time.process_time_ns() - cpu_started
    wall_elapsed = time.perf_counter_ns() - wall_started
    measured = len(latencies)
    predictions = []
    for sample in samples:
        repeated = scores_by_sample[sample["sample_id"]]
        if max(repeated) - min(repeated) > 1e-9:
            raise ValueError(f"{route} repeated inference is nondeterministic")
        predictions.append({"sample_id": sample["sample_id"], "score": repeated[0]})
    return {
        "schema_version": 1,
        "route": route,
        **identity,
        "engine": engine,
        "engine_version": engine_version,
        "measured_inferences": measured,
        "latency_ms": {
            "p50": percentile(latencies, 50),
            "p95": percentile(latencies, 95),
            "p99": percentile(latencies, 99),
            "max": max(latencies),
        },
        "throughput_per_second": measured / (wall_elapsed / 1_000_000_000.0),
        "cpu_seconds": cpu_elapsed / 1_000_000_000.0,
        "peak_rss_bytes": peak_rss_bytes(),
        "predictions": predictions,
    }


def prepare(args: argparse.Namespace) -> None:
    import onnxruntime as ort
    import xgboost as xgb

    workdir = args.workdir.resolve()
    workdir.mkdir(parents=True, exist_ok=True)
    if any(workdir.iterdir()):
        raise FileExistsError("parity work directory must be empty")
    profile_path = args.profile.resolve()
    schema_path = args.profile_schema.resolve()
    profile = load_json(profile_path, "parity profile")
    validate_profile(profile, load_json(schema_path, "parity profile schema"))
    validate_sha256(args.candidate_sha256, "candidate_sha256")
    run_id = str(uuid.UUID(args.run_id))
    if run_id != args.run_id:
        raise ValueError("run_id must be a canonical lowercase UUID")

    columns = profile["feature_columns"]
    if columns != ["bps", "iat_mean_ms", "pktlen_mean", "pps"]:
        raise ValueError("internal parity feature order drifted from the production vectorizer profile")
    matrix = deterministic_matrix(profile["random_seed"], profile["sample_count"])
    samples = [
        {
            "sample_id": str(uuid.uuid5(PARITY_NAMESPACE, f"{run_id}:sample:{index:04d}")),
            "features": {
                column: float(matrix[index, offset])
                for offset, column in enumerate(columns)
            },
        }
        for index in range(len(matrix))
    ]
    profile_sha = file_sha256(profile_path)
    feature_sha = canonical_sha256(columns)
    sample_sha = canonical_sha256(samples)
    model, native_path, onnx_path = train_and_export(
        profile["random_seed"], len(columns), workdir,
    )
    artifact_sha = file_sha256(onnx_path)
    model_id = str(uuid.uuid5(PARITY_NAMESPACE, f"{args.candidate_sha256}:{profile_sha}"))
    identity = {
        "run_id": run_id,
        "profile_id": profile["profile_id"],
        "profile_sha256": profile_sha,
        "candidate_sha256": args.candidate_sha256,
        "model_id": model_id,
        "model_version": f"internal-parity-{artifact_sha[:16]}",
        "model_artifact_sha256": artifact_sha,
        "feature_columns_sha256": feature_sha,
        "sample_set_sha256": sample_sha,
    }
    bundle_meaning = {
        "schema_version": 1,
        **identity,
        "input_name": "float_input",
        "feature_columns": columns,
        "decision_threshold": profile["decision_threshold"],
        "warmup_iterations": profile["warmup_iterations"],
        "measured_iterations": profile["measured_iterations"],
        "flink_parallelism": profile["flink_parallelism"],
        "samples": samples,
    }
    bundle = {**bundle_meaning, "bundle_sha256": canonical_sha256(bundle_meaning)}
    identity["bundle_sha256"] = bundle["bundle_sha256"]
    write_json_exclusive(workdir / "parity-input.json", bundle)

    python_receipt = benchmark_route(
        "python", "xgboost", xgb.__version__,
        lambda row: float(model.predict_proba(row.reshape(1, -1))[0, 1]),
        samples, matrix, profile["warmup_iterations"], profile["measured_iterations"],
        identity,
    )
    session = ort.InferenceSession(str(onnx_path), providers=["CPUExecutionProvider"])
    try:
        onnx_receipt = benchmark_route(
            "onnx", "onnxruntime", ort.__version__,
            lambda row: float(positive_probability(
                session.run(None, {"float_input": row.reshape(1, -1)}), 1,
            )[0]),
            samples, matrix, profile["warmup_iterations"], profile["measured_iterations"],
            identity,
        )
    finally:
        del session
    write_json_exclusive(workdir / "python-receipt.json", python_receipt)
    write_json_exclusive(workdir / "onnx-receipt.json", onnx_receipt)
    if file_sha256(native_path) == artifact_sha:
        raise ValueError("native and ONNX artifacts unexpectedly share one digest")
    print(json.dumps({
        "phase": "prepared",
        **identity,
        "sample_count": len(samples),
        "python_receipt_sha256": file_sha256(workdir / "python-receipt.json"),
        "onnx_receipt_sha256": file_sha256(workdir / "onnx-receipt.json"),
    }, sort_keys=True))


ROUTE_IDENTITY_FIELDS = (
    "run_id", "profile_id", "profile_sha256", "candidate_sha256", "model_id",
    "model_version", "model_artifact_sha256", "feature_columns_sha256",
    "sample_set_sha256", "bundle_sha256",
)


def validate_route_receipt(
    receipt: dict[str, Any], route: str, bundle: dict[str, Any], profile: dict[str, Any],
) -> dict[str, float]:
    if receipt.get("schema_version") != 1 or receipt.get("route") != route:
        raise ValueError(f"{route} receipt version or route is invalid")
    for field in ROUTE_IDENTITY_FIELDS:
        expected = bundle.get(field)
        if receipt.get(field) != expected:
            raise ValueError(f"{route} receipt identity mismatch: {field}")
    expected_measured = profile["sample_count"] * profile["measured_iterations"]
    if receipt.get("measured_inferences") != expected_measured:
        raise ValueError(f"{route} receipt measured inference count is incomplete")
    predictions = receipt.get("predictions")
    if not isinstance(predictions, list) or len(predictions) != profile["sample_count"]:
        raise ValueError(f"{route} receipt prediction count is incomplete")
    expected_ids = [sample["sample_id"] for sample in bundle["samples"]]
    actual_ids = [item.get("sample_id") for item in predictions]
    if actual_ids != expected_ids or len(set(actual_ids)) != len(actual_ids):
        raise ValueError(f"{route} receipt sample identity/order drifted")
    scores: dict[str, float] = {}
    for item in predictions:
        score = float(item.get("score"))
        if not math.isfinite(score) or score < 0.0 or score > 1.0:
            raise ValueError(f"{route} receipt contains an invalid probability")
        scores[item["sample_id"]] = score
    metrics = receipt.get("latency_ms")
    if not isinstance(metrics, dict) or any(
        not math.isfinite(float(metrics.get(key, math.nan))) or float(metrics[key]) < 0
        for key in ("p50", "p95", "p99", "max")
    ):
        raise ValueError(f"{route} latency metrics are incomplete")
    if any(not math.isfinite(float(receipt.get(key, math.nan))) for key in (
        "throughput_per_second", "cpu_seconds", "peak_rss_bytes",
    )):
        raise ValueError(f"{route} resource metrics are incomplete")
    return scores


def compare_scores(
    left: dict[str, float], right: dict[str, float], tolerance: float,
) -> dict[str, Any]:
    if set(left) != set(right):
        raise ValueError("parity comparison received different sample sets")
    maximum = max(abs(left[key] - right[key]) for key in sorted(left))
    return {
        "max_absolute_score_error": maximum,
        "tolerance": tolerance,
        "status": "PASS" if maximum <= tolerance else "FAIL",
    }


def route_summary(receipt: dict[str, Any], path: Path) -> dict[str, Any]:
    return {
        "receipt_sha256": file_sha256(path),
        "engine": receipt["engine"],
        "engine_version": receipt["engine_version"],
        "measured_inferences": receipt["measured_inferences"],
        "latency_ms": receipt["latency_ms"],
        "throughput_per_second": receipt["throughput_per_second"],
        "cpu_seconds": receipt["cpu_seconds"],
        "peak_rss_bytes": receipt["peak_rss_bytes"],
    }


def finalize(args: argparse.Namespace) -> None:
    from jsonschema import Draft202012Validator, FormatChecker

    workdir = args.workdir.resolve()
    profile_path = args.profile.resolve()
    profile = load_json(profile_path, "parity profile")
    validate_profile(profile, load_json(args.profile_schema.resolve(), "parity profile schema"))
    bundle = load_json(workdir / "parity-input.json", "parity input bundle")
    bundle_meaning = {key: value for key, value in bundle.items() if key != "bundle_sha256"}
    if canonical_sha256(bundle_meaning) != bundle.get("bundle_sha256"):
        raise ValueError("parity input bundle identity is invalid")
    if file_sha256(profile_path) != bundle.get("profile_sha256"):
        raise ValueError("parity profile changed between prepare and finalize")
    route_paths = {
        "python": workdir / "python-receipt.json",
        "onnx": workdir / "onnx-receipt.json",
        "flink": workdir / "flink-receipt.json",
    }
    receipts = {route: load_json(path, f"{route} receipt") for route, path in route_paths.items()}
    scores = {
        route: validate_route_receipt(receipt, route, bundle, profile)
        for route, receipt in receipts.items()
    }
    tolerance = profile["score_tolerances"]
    comparisons = {
        "python_to_onnx": compare_scores(scores["python"], scores["onnx"], tolerance["python_to_onnx"]),
        "onnx_to_flink": compare_scores(scores["onnx"], scores["flink"], tolerance["onnx_to_flink"]),
        "python_to_flink": compare_scores(scores["python"], scores["flink"], tolerance["python_to_flink"]),
    }
    threshold = float(profile["decision_threshold"])
    decision_mismatches = sum(
        1 for sample_id in scores["python"]
        if len({scores[route][sample_id] >= threshold for route in scores}) != 1
    )
    gates: list[dict[str, Any]] = []
    for name, comparison in comparisons.items():
        gates.append({
            "gate": f"score_parity.{name}",
            "status": comparison["status"],
            "actual": comparison["max_absolute_score_error"],
            "limit": comparison["tolerance"],
        })
    gates.append({
        "gate": "decision_parity",
        "status": "PASS" if decision_mismatches <= tolerance["max_decision_mismatches"] else "FAIL",
        "actual": decision_mismatches,
        "limit": tolerance["max_decision_mismatches"],
    })
    for route, receipt in receipts.items():
        budget = profile["performance_budgets"][route]
        checks = (
            ("p99_inference_latency_ms", receipt["latency_ms"]["p99"], budget["max_p99_inference_latency_ms"], "max"),
            ("throughput_per_second", receipt["throughput_per_second"], budget["min_throughput_per_second"], "min"),
            ("peak_rss_bytes", receipt["peak_rss_bytes"], budget["max_peak_rss_bytes"], "max"),
        )
        for metric, actual, limit, direction in checks:
            passed = actual <= limit if direction == "max" else actual >= limit
            gates.append({
                "gate": f"{route}.{metric}",
                "status": "PASS" if passed else "FAIL",
                "actual": actual,
                "limit": limit,
            })

    result_meaning = {
        "schema_version": 1,
        "result_id": str(uuid.uuid5(
            PARITY_NAMESPACE,
            f"{bundle['run_id']}:{bundle['profile_sha256']}:{bundle['model_artifact_sha256']}",
        )),
        "run_id": bundle["run_id"],
        "profile_id": bundle["profile_id"],
        "profile_sha256": bundle["profile_sha256"],
        "candidate_sha256": bundle["candidate_sha256"],
        "claim_scope": "INTERNAL_ENGINEERING_ONLY",
        "cnas_claim_authorized": False,
        "production_promotion_authorized": False,
        "model_artifact_sha256": bundle["model_artifact_sha256"],
        "feature_columns_sha256": bundle["feature_columns_sha256"],
        "sample_set_sha256": bundle["sample_set_sha256"],
        "sample_count": len(bundle["samples"]),
        "routes": {
            route: route_summary(receipt, route_paths[route])
            for route, receipt in receipts.items()
        },
        "parity": {**comparisons, "decision_mismatches": decision_mismatches},
        "gates": gates,
        "status": "PASS" if all(item["status"] == "PASS" for item in gates) else "FAIL",
        "limitations": [
            "This is a deterministic synthetic internal engineering profile, not a production traffic observation.",
            "The Flink route runs the production FeatureStat vectorizer and ONNX probability parser in a Kubernetes-hosted local DataStream job; it is not a shared Flink deployment.",
            "This result cannot authorize model activation, production promotion, or a CNAS accuracy or false-positive-rate claim."
        ],
    }
    result = {**result_meaning, "result_sha256": canonical_sha256(result_meaning)}
    result_schema = load_json(args.result_schema.resolve(), "parity result schema")
    Draft202012Validator.check_schema(result_schema)
    Draft202012Validator(
        result_schema, format_checker=FormatChecker(),
    ).validate(result)
    write_json_exclusive(args.output.resolve(), result)
    print(json.dumps(result, sort_keys=True))
    if result["status"] != "PASS":
        raise SystemExit("internal model inference parity profile failed")


def parser() -> argparse.ArgumentParser:
    value = argparse.ArgumentParser(description=__doc__)
    subparsers = value.add_subparsers(dest="command", required=True)
    for name in ("prepare", "finalize"):
        command = subparsers.add_parser(name)
        command.add_argument("--workdir", type=Path, required=True)
        command.add_argument("--profile", type=Path, required=True)
        command.add_argument("--profile-schema", type=Path, required=True)
        if name == "prepare":
            command.add_argument("--candidate-sha256", required=True)
            command.add_argument("--run-id", required=True)
        else:
            command.add_argument("--result-schema", type=Path, required=True)
            command.add_argument("--output", type=Path, required=True)
    return value


def main() -> None:
    args = parser().parse_args()
    if args.command == "prepare":
        prepare(args)
    else:
        finalize(args)


if __name__ == "__main__":
    main()

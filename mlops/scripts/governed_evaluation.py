#!/usr/bin/env python3
"""Leakage-safe known/open-set evaluation and graph-ablation primitives."""

from __future__ import annotations

import json
import math
import os
import re
import uuid
from pathlib import Path
from typing import Any, Callable, Mapping

import numpy as np
import pandas as pd
import xgboost as xgb
from sklearn.metrics import (
    accuracy_score,
    average_precision_score,
    f1_score,
    precision_recall_fscore_support,
    precision_score,
    recall_score,
    roc_auc_score,
)

from dataset_governance import (
    canonical_json_sha256,
    sha256_file,
    validate_split_isolation,
    write_json_exclusive,
)
from train_model import load_governed_training_data


METHOD_CONTRACT_ID = "traffic.mlops.governed-evaluation-method.v1"
METHOD_CONTRACT_SHA256 = "047aaed8d3dcd60a5426cef9c1bde6470fa3957c788e5f004d734a0795383b65"
EVALUATION_NAMESPACE = uuid.UUID("e1b57550-8dc0-5756-9fe7-f0bfbd58e4c2")
GRAPH_VARIANTS = (
    "non_graph_baseline",
    "gnn_full",
    "gnn_no_edges",
    "gnn_no_sources",
)


def validate_evaluation_manifest_identity(manifest: Mapping[str, Any]) -> None:
    meaning = {
        key: value
        for key, value in manifest.items()
        if key not in {"schema_version", "evaluation_id", "state", "evaluation_sha256"}
    }
    expected_sha = canonical_json_sha256(meaning)
    expected_id = str(uuid.uuid5(EVALUATION_NAMESPACE, expected_sha))
    if manifest.get("schema_version") != 1 or manifest.get("state") != "evaluated":
        raise ValueError("evaluation manifest version or state is invalid")
    if manifest.get("evaluation_sha256") != expected_sha or manifest.get("evaluation_id") != expected_id:
        raise ValueError("evaluation manifest identity or meaning hash mismatch")


def required_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required when MLOPS_GOVERNED_EVALUATION_V1_ENABLED=true")
    return value


def strict_flag(name: str, default: bool = False) -> bool:
    value = os.getenv(name)
    if value is None or not value.strip():
        return default
    normalized = value.strip().lower()
    if normalized not in {"true", "false"}:
        raise ValueError(f"{name} must be explicitly true or false")
    return normalized == "true"


def _safe_div(numerator: int | float, denominator: int | float) -> float:
    return float(numerator / denominator) if denominator else 0.0


def _scores(model: Any, features: pd.DataFrame) -> np.ndarray:
    values = np.asarray(model.predict_proba(features)[:, 1], dtype=float)
    if values.ndim != 1 or len(values) != len(features):
        raise ValueError("model produced an invalid probability vector")
    if not np.isfinite(values).all() or ((values < 0) | (values > 1)).any():
        raise ValueError("model probabilities must be finite values in [0,1]")
    return values


def confidence(scores: np.ndarray) -> np.ndarray:
    values = np.asarray(scores, dtype=float)
    return np.maximum(values, 1.0 - values)


def select_abstain_threshold(scores: np.ndarray, known_retention_target: float) -> float:
    if not 0 < known_retention_target <= 1:
        raise ValueError("known retention target must be in (0,1]")
    values = confidence(scores)
    if len(values) < 2:
        raise ValueError("validation requires at least two scores")
    try:
        selected = np.quantile(values, 1.0 - known_retention_target, method="lower")
    except TypeError:  # numpy <1.22 compatibility
        selected = np.quantile(values, 1.0 - known_retention_target, interpolation="lower")
    return float(min(1.0, max(0.5, selected)))


def binary_metrics(
    labels: np.ndarray | pd.Series,
    scores: np.ndarray,
    *,
    classification_threshold: float = 0.5,
    accepted: np.ndarray | None = None,
) -> dict[str, Any]:
    y = np.asarray(labels, dtype=int)
    p = np.asarray(scores, dtype=float)
    if len(y) == 0 or len(y) != len(p) or not set(np.unique(y)).issubset({0, 1}):
        raise ValueError("binary evaluation requires aligned non-empty 0/1 labels and scores")
    decisions = p >= classification_threshold
    if accepted is not None:
        mask = np.asarray(accepted, dtype=bool)
        if len(mask) != len(y):
            raise ValueError("accepted mask length does not match labels")
        decisions &= mask
    tn = int(((y == 0) & ~decisions).sum())
    fp = int(((y == 0) & decisions).sum())
    fn = int(((y == 1) & ~decisions).sum())
    tp = int(((y == 1) & decisions).sum())
    precision, recall, f1, support = precision_recall_fscore_support(
        y, decisions.astype(int), labels=[0, 1], zero_division=0,
    )
    both = len(np.unique(y)) == 2
    return {
        "accuracy": float(accuracy_score(y, decisions)),
        "precision": float(precision_score(y, decisions, zero_division=0)),
        "recall": float(recall_score(y, decisions, zero_division=0)),
        "f1": float(f1_score(y, decisions, zero_division=0)),
        "macro_f1": float(f1_score(y, decisions, average="macro", zero_division=0)),
        "micro_f1": float(f1_score(y, decisions, average="micro", zero_division=0)),
        "auc_roc": float(roc_auc_score(y, p)) if both else 0.0,
        "auc_pr": float(average_precision_score(y, p)) if both else 0.0,
        "normal_false_positive_rate": _safe_div(fp, fp + tn),
        "known_attack_recall": _safe_div(tp, tp + fn),
        "confusion_matrix": {"tn": tn, "fp": fp, "fn": fn, "tp": tp},
        "per_class": {
            str(label): {
                "precision": float(precision[index]),
                "recall": float(recall[index]),
                "f1": float(f1[index]),
                "support": int(support[index]),
            }
            for index, label in enumerate((0, 1))
        },
    }


def calibration_metrics(labels: np.ndarray, scores: np.ndarray, bins: int = 10) -> dict[str, Any]:
    y = np.asarray(labels, dtype=float)
    p = np.asarray(scores, dtype=float)
    if len(y) != len(p) or len(y) == 0 or bins < 2 or bins > 100:
        raise ValueError("invalid calibration inputs")
    brier = float(np.mean((p - y) ** 2))
    edges = np.linspace(0.0, 1.0, bins + 1)
    ece = 0.0
    for index in range(bins):
        if index == bins - 1:
            mask = (p >= edges[index]) & (p <= edges[index + 1])
        else:
            mask = (p >= edges[index]) & (p < edges[index + 1])
        if not mask.any():
            continue
        ece += float(mask.mean()) * abs(float(p[mask].mean()) - float(y[mask].mean()))
    return {"brier_score": brier, "expected_calibration_error": float(ece), "bins": bins}


def bootstrap_interval(
    size: int,
    statistic: Callable[[np.ndarray], float],
    *,
    seed: int,
    rounds: int,
    point: float,
) -> dict[str, float]:
    if size < 2 or rounds < 100 or rounds > 10_000 or seed < 0:
        raise ValueError("bootstrap requires size>=2, seed>=0 and 100..10000 rounds")
    rng = np.random.default_rng(seed)
    observed: list[float] = []
    for _ in range(rounds):
        indices = rng.integers(0, size, size=size)
        value = float(statistic(indices))
        if math.isfinite(value):
            observed.append(value)
    if len(observed) < max(20, rounds // 2):
        raise ValueError("too few valid bootstrap replicates")
    lower, upper = np.percentile(np.asarray(observed), [2.5, 97.5])
    return {
        "point": float(point),
        "lower": float(lower),
        "upper": float(upper),
        "confidence": 0.95,
    }


def _proportion_ci(values: np.ndarray, *, seed: int, rounds: int) -> dict[str, float]:
    flags = np.asarray(values, dtype=float)
    point = float(flags.mean())
    return bootstrap_interval(
        len(flags), lambda indices: float(flags[indices].mean()),
        seed=seed, rounds=rounds, point=point,
    )


def _auc_ci(labels: np.ndarray, scores: np.ndarray, *, seed: int, rounds: int) -> dict[str, float]:
    y = np.asarray(labels, dtype=int)
    p = np.asarray(scores, dtype=float)
    point = float(roc_auc_score(y, p))

    def statistic(indices: np.ndarray) -> float:
        sampled = y[indices]
        if len(np.unique(sampled)) != 2:
            return float("nan")
        return float(roc_auc_score(sampled, p[indices]))

    return bootstrap_interval(len(y), statistic, seed=seed, rounds=rounds, point=point)


def compare_graph_ablations(
    variants: Mapping[str, pd.DataFrame],
    *,
    classification_threshold: float = 0.5,
) -> dict[str, Any]:
    missing = [name for name in GRAPH_VARIANTS if name not in variants]
    if missing:
        return {
            "state": "NOT_EXECUTED",
            "required_variants": list(GRAPH_VARIANTS),
            "missing_variants": missing,
            "reason": "all graph and non-graph prediction artifacts are required",
        }
    normalized: dict[str, pd.DataFrame] = {}
    reference_ids: list[str] | None = None
    reference_labels: list[int] | None = None
    for name in GRAPH_VARIANTS:
        frame = variants[name]
        required = {"event_id", "label", "score"}
        if required - set(frame.columns) or frame.empty:
            raise ValueError(f"graph ablation {name} is missing event_id/label/score")
        ordered = frame.assign(event_id=frame["event_id"].astype(str)).sort_values(
            "event_id", kind="mergesort"
        ).reset_index(drop=True)
        if ordered["event_id"].duplicated().any():
            raise ValueError(f"graph ablation {name} contains duplicate event_id")
        ids = ordered["event_id"].tolist()
        labels = ordered["label"].astype(int).tolist()
        if reference_ids is None:
            reference_ids, reference_labels = ids, labels
        elif ids != reference_ids or labels != reference_labels:
            raise ValueError(f"graph ablation {name} does not use the exact reference population")
        normalized[name] = ordered

    metrics = {
        name: {
            key: value
            for key, value in binary_metrics(
                frame["label"].to_numpy(), frame["score"].to_numpy(),
                classification_threshold=classification_threshold,
            ).items()
            if key in {"accuracy", "f1", "macro_f1", "micro_f1", "auc_roc", "auc_pr"}
        }
        for name, frame in normalized.items()
    }

    def delta(left: str, right: str) -> dict[str, float]:
        return {key: float(metrics[left][key] - metrics[right][key]) for key in metrics[left]}

    return {
        "state": "EVALUATED",
        "required_variants": list(GRAPH_VARIANTS),
        "missing_variants": [],
        "population_sha256": canonical_json_sha256(reference_ids),
        "metrics": metrics,
        "deltas": {
            "gnn_full_minus_non_graph": delta("gnn_full", "non_graph_baseline"),
            "gnn_full_minus_no_edges": delta("gnn_full", "gnn_no_edges"),
            "gnn_full_minus_no_sources": delta("gnn_full", "gnn_no_sources"),
        },
        "graph_gain_interpretation": "preserve measured positive, zero or negative deltas without promotion rewriting",
    }


from governance_common import load_json_object as _load_json  # noqa: F401  (DRY 收敛)


def _verify_training_manifest(manifest: Mapping[str, Any]) -> None:
    meaning = {
        key: value
        for key, value in manifest.items()
        if key not in {"schema_version", "state", "run_sha256"}
    }
    if manifest.get("schema_version") != 1 or manifest.get("state") != "trained":
        raise ValueError("training run manifest state or version is invalid")
    if canonical_json_sha256(meaning) != manifest.get("run_sha256"):
        raise ValueError("training run manifest meaning hash mismatch")


def load_governed_evaluation_inputs(
    data_dir: str, model_dir: str,
) -> tuple[Any, dict[str, pd.DataFrame], list[str], dict[str, Any], dict[str, Any]]:
    data_root, model_root = Path(data_dir).resolve(), Path(model_dir).resolve()
    _, _, _, _, feature_columns, dataset = load_governed_training_data(str(data_root))
    frames = {
        name: pd.read_parquet(data_root / f"{name}.parquet", engine="pyarrow")
        for name in dataset["splits"]
    }
    validate_split_isolation(frames)
    required_splits = {"train", "validation", "test", "open_set"}
    missing_splits = sorted(required_splits - set(frames))
    if missing_splits:
        raise ValueError(f"governed evaluation is missing splits: {missing_splits}")
    open_families = set(frames["open_set"]["attack_family"].astype(str))
    learned_families = set(pd.concat([
        frames["train"]["attack_family"], frames["validation"]["attack_family"]
    ]).astype(str))
    if not open_families or open_families & learned_families:
        raise ValueError("open-set families must be non-empty and absent from train/validation")
    if set(frames["open_set"]["label"].astype(int)) != {1}:
        raise ValueError("open_set must contain only independently held-out attack rows")
    if set(frames["test"]["label"].astype(int)) != {0, 1}:
        raise ValueError("known test must contain normal and known attack rows")

    training = _load_json(model_root / "training-run-manifest.json", "training run manifest")
    _verify_training_manifest(training)
    if training.get("dataset_id") != dataset["dataset_id"] or \
            training.get("dataset_sha256") != dataset["dataset_sha256"]:
        raise ValueError("training run does not bind the governed evaluation dataset")
    model_path = model_root / "model.json"
    if training.get("algorithm") != "xgboost":
        raise ValueError("governed evaluator currently loads the approved XGBoost baseline artifact")
    if training.get("artifacts", {}).get("model") != sha256_file(model_path):
        raise ValueError("model artifact hash does not match training run manifest")
    model = xgb.XGBClassifier()
    model.load_model(model_path)
    return model, frames, feature_columns, dataset, training


def _load_graph_variants(
    directory: str, baseline: pd.DataFrame, dataset: Mapping[str, Any],
) -> tuple[dict[str, pd.DataFrame], dict[str, Any] | None]:
    variants: dict[str, pd.DataFrame] = {"non_graph_baseline": baseline}
    if not directory.strip():
        return variants, None
    root = Path(directory).resolve()
    training = _load_json(root / "gnn-training-run-manifest.json", "GNN training run manifest")
    _verify_training_manifest(training)
    if training.get("algorithm") != "gnn" or training.get("dataset_id") != dataset.get("dataset_id") or \
            training.get("dataset_sha256") != dataset.get("dataset_sha256"):
        raise ValueError("GNN training run does not bind the governed evaluation dataset")
    if training.get("graph_snapshot") != dataset.get("graph_snapshot"):
        raise ValueError("GNN training run does not bind the governed graph snapshot")
    for name in GRAPH_VARIANTS[1:]:
        path = root / f"{name}.parquet"
        artifact_name = f"{name}_predictions"
        if not path.is_file() or training.get("artifacts", {}).get(artifact_name) != sha256_file(path):
            raise ValueError(f"{name} prediction hash does not match GNN training run")
        variants[name] = pd.read_parquet(path, engine="pyarrow")
    return variants, {
        "run_id": training["run_id"],
        "run_sha256": training["run_sha256"],
        "graph_snapshot": training["graph_snapshot"],
    }


def _write_parquet_exclusive(path: Path, frame: pd.DataFrame) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        raise FileExistsError(f"refusing to overwrite immutable evaluation artifact: {path}")
    temporary = path.with_name(f".{path.name}.{uuid.uuid4().hex}.tmp")
    try:
        frame.to_parquet(temporary, index=False, engine="pyarrow")
        os.link(temporary, path)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def run_governed_evaluation(data_dir: str, model_dir: str, output_dir: str) -> dict[str, Any]:
    seed = int(required_env("EVALUATION_BOOTSTRAP_SEED"))
    rounds = int(required_env("EVALUATION_BOOTSTRAP_ROUNDS"))
    retention_target = float(required_env("EVALUATION_KNOWN_RETENTION_TARGET"))
    image_digest = required_env("EVALUATOR_IMAGE_DIGEST")
    if seed < 0 or rounds < 100 or rounds > 10_000:
        raise ValueError("evaluation seed/rounds are outside the governed range")
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", image_digest):
        raise ValueError("EVALUATOR_IMAGE_DIGEST must be sha256:<lowercase digest>")

    model, frames, feature_columns, dataset, training = load_governed_evaluation_inputs(
        data_dir, model_dir
    )
    validation_scores = _scores(model, frames["validation"][feature_columns])
    test_scores = _scores(model, frames["test"][feature_columns])
    unknown_scores = _scores(model, frames["open_set"][feature_columns])
    classification_threshold = 0.5
    abstain_threshold = select_abstain_threshold(validation_scores, retention_target)
    test_accepted = confidence(test_scores) >= abstain_threshold
    unknown_abstained = confidence(unknown_scores) < abstain_threshold
    test_labels = frames["test"]["label"].astype(int).to_numpy()

    known = binary_metrics(
        test_labels, test_scores,
        classification_threshold=classification_threshold,
        accepted=test_accepted,
    )
    unknown_by_family = {
        family: {
            "support": int(mask.sum()),
            "unknown_recall": float(unknown_abstained[mask].mean()),
        }
        for family in sorted(set(frames["open_set"]["attack_family"].astype(str)))
        for mask in [(frames["open_set"]["attack_family"].astype(str).to_numpy() == family)]
    }
    unknown_recall = float(unknown_abstained.mean())
    unknown_detection_rate = float((unknown_scores >= classification_threshold).mean())
    known_retention = float(test_accepted.mean())
    normal_mask, attack_mask = test_labels == 0, test_labels == 1
    classification_decision = test_scores >= classification_threshold
    detected_known = classification_decision & test_accepted

    prediction_rows = pd.concat([
        pd.DataFrame({
            "event_id": frames["test"]["event_id"].astype(str),
            "population": "known_test",
            "label": test_labels,
            "attack_family": frames["test"]["attack_family"].astype(str),
            "score": test_scores,
            "predicted_attack": detected_known,
            "abstained": ~test_accepted,
        }),
        pd.DataFrame({
            "event_id": frames["open_set"]["event_id"].astype(str),
            "population": "unknown_attack",
            "label": frames["open_set"]["label"].astype(int),
            "attack_family": frames["open_set"]["attack_family"].astype(str),
            "score": unknown_scores,
            "predicted_attack": unknown_scores >= classification_threshold,
            "abstained": unknown_abstained,
        }),
    ], ignore_index=True).sort_values("event_id", kind="mergesort").reset_index(drop=True)
    output_root = Path(output_dir).resolve()
    prediction_path = output_root / "governed-predictions.parquet"
    manifest_path = output_root / "evaluation-manifest.json"
    if manifest_path.exists():
        raise FileExistsError(f"refusing to overwrite immutable evaluation artifact: {manifest_path}")
    _write_parquet_exclusive(prediction_path, prediction_rows)

    baseline_variant = pd.DataFrame({
        "event_id": frames["test"]["event_id"].astype(str),
        "label": test_labels,
        "score": test_scores,
    })
    graph_variants, graph_training_run = _load_graph_variants(
        os.getenv("GRAPH_ABLATION_PREDICTIONS_DIR", ""), baseline_variant, dataset
    )
    ablations = compare_graph_ablations(graph_variants)
    calibration = calibration_metrics(test_labels, test_scores)
    metrics = {
        "known": known,
        "known_attack_recall": float(detected_known[attack_mask].mean()),
        "normal_false_positive_rate": float(detected_known[normal_mask].mean()),
        "unknown_recall": unknown_recall,
        "unknown_detection_rate": unknown_detection_rate,
        "unknown_false_accept_rate": float((~unknown_abstained).mean()),
        "known_retention": known_retention,
        "known_abstain_rate": float((~test_accepted).mean()),
        "unknown_by_family": unknown_by_family,
        # 口径说明（代码审查 H40 收敛项）：内部 unknown_recall 是 unknown 攻击上
        # abstain（低置信拒识）比例；盲评 metric-definition.md 的 unknown recall
        # 是 unknown_detected/unknown_attack_total（判为攻击的比例）。二者语义
        # 不同，必须同时输出并各自标注，不得混用。
        "metric_definitions": {
            "known_attack_recall": "预警准确率/检测率: TP/(TP+FN) over accepted known-test attacks; gate lower 95% CI >= 0.95 (signed method)",
            "normal_false_positive_rate": "误报率: FP/(FP+TN) over accepted known-test normals; gate upper 95% CI <= 0.05",
            "unknown_recall": "internal: fraction of unknown attacks abstained (low confidence); NOT the blind unknown_detected/total",
            "unknown_detection_rate": "blind-aligned: unknown attacks predicted as attack / total unknown attacks",
            "known_retention": "fraction of known-test inputs accepted (not abstained)",
        },
    }
    intervals = {
        "known_attack_recall": _proportion_ci(
            detected_known[attack_mask], seed=seed + 1, rounds=rounds,
        ),
        "normal_false_positive_rate": _proportion_ci(
            detected_known[normal_mask], seed=seed + 2, rounds=rounds,
        ),
        "unknown_recall": _proportion_ci(
            unknown_abstained, seed=seed + 3, rounds=rounds,
        ),
        "unknown_detection_rate": _proportion_ci(
            unknown_scores >= classification_threshold, seed=seed + 6, rounds=rounds,
        ),
        "known_retention": _proportion_ci(
            test_accepted, seed=seed + 4, rounds=rounds,
        ),
        "auc_roc": _auc_ci(test_labels, test_scores, seed=seed + 5, rounds=rounds),
    }
    meaning = {
        "dataset_id": dataset["dataset_id"],
        "dataset_sha256": dataset["dataset_sha256"],
        "training_run_id": training["run_id"],
        "training_run_sha256": training["run_sha256"],
        "graph_training_run": graph_training_run,
        "model_artifact_sha256": training["artifacts"]["model"],
        "prediction_artifact_sha256": sha256_file(prediction_path),
        "method_contract_sha256": METHOD_CONTRACT_SHA256,
        "evaluator_code_sha256": sha256_file(__file__),
        "evaluator_image_digest": image_digest,
        "seed": seed,
        "bootstrap_rounds": rounds,
        "thresholds": {
            "classification": classification_threshold,
            "abstain_confidence": abstain_threshold,
            "known_retention_target": retention_target,
            "selected_from": "validation",
        },
        "populations": {
            "validation": len(frames["validation"]),
            "known_test": len(frames["test"]),
            "known_normal": int(normal_mask.sum()),
            "known_attack": int(attack_mask.sum()),
            "unknown_attack": len(frames["open_set"]),
        },
        "metrics": metrics,
        "confidence_intervals": intervals,
        "calibration": calibration,
        "graph_ablations": ablations,
        "activation_authorized": False,
    }
    evaluation_sha = canonical_json_sha256(meaning)
    evaluation_id = str(uuid.uuid5(EVALUATION_NAMESPACE, evaluation_sha))
    manifest = {
        "schema_version": 1,
        "evaluation_id": evaluation_id,
        "state": "evaluated",
        **meaning,
        "evaluation_sha256": evaluation_sha,
    }
    write_json_exclusive(manifest_path, manifest)
    return manifest

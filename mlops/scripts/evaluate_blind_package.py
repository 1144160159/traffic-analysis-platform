#!/usr/bin/env python3
"""
Evaluate a frozen blind-test package for the topic-1 detection-quality gate.

The script deliberately refuses to synthesize labels or predictions. If the
blind package only contains templates, the generated evidence is "blocked" and
lists the exact missing artifacts.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
import math
import os
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Tuple


REQUIRED_CONTRACT_FILES = [
    "README.md",
    "dataset-manifest.template.yaml",
    "label-schema.yaml",
    "metric-definition.md",
]

ACTUAL_ARTIFACT_CANDIDATES = {
    "dataset_manifest": ["dataset-manifest.yaml"],
    "threshold_lock": ["threshold-lock.json"],
    "labels": ["labels.csv", "labels/labels.csv"],
    "predictions": ["predictions.csv", "predictions/predictions.csv"],
    "third_party_attestation": [
        "third-party-attestation.yaml",
        "third-party-attestation.json",
        "reports/third-party-attestation.yaml",
        "reports/third-party-attestation.json",
    ],
}

REQUIRED_LABEL_COLUMNS = {"sample_id", "ground_truth"}
REQUIRED_PREDICTION_ID_COLUMN = "sample_id"

TEMPLATE_MARKERS = {
    "review_required",
    "review-template",
    "bootstrap_review_required",
    "template_review_required",
    "must fill this",
    "not an attestation",
    "replace this template",
    "threshold is intentionally null",
    "do not use this bootstrap file",
}

ATTESTATION_REQUIRED_MARKERS = {"signed_by", "signed_at"}

NORMAL_LABELS = {"0", "normal", "benign", "clean", "negative", "none"}
ATTACK_LABELS = {
    "1",
    "attack",
    "malicious",
    "positive",
    "known_attack",
    "unknown_attack",
    "encrypted_attack",
    "c2",
    "scan",
    "tunnel",
    "exfiltration",
    "lateral",
    "bruteforce",
    "botnet",
    "exploit",
}


@dataclass(frozen=True)
class Thresholds:
    min_detection_rate: float = 0.95
    max_false_positive_rate: float = 0.05
    min_unknown_recall: float = 0.80


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def add_check(
    checks: List[Dict[str, Any]],
    phase: str,
    name: str,
    severity: str,
    passed: bool,
    status: str,
    detail: str,
    artifact: str = "",
) -> None:
    checks.append(
        {
            "phase": phase,
            "name": name,
            "severity": severity,
            "passed": bool(passed),
            "status": status,
            "detail": detail,
            "artifact": artifact,
        }
    )


def find_artifact(package_dir: Path, candidates: Iterable[str]) -> Optional[Path]:
    for candidate in candidates:
        path = package_dir / candidate
        if path.is_file():
            return path
    return None


def read_csv_rows(path: Path) -> List[Dict[str, str]]:
    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        reader = csv.DictReader(handle)
        return [{key: (value or "").strip() for key, value in row.items()} for row in reader]


def write_csv(path: Path, rows: List[Dict[str, Any]], fieldnames: List[str]) -> None:
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow({field: row.get(field, "") for field in fieldnames})


def parse_bool(value: str) -> bool:
    return str(value).strip().lower() in {"1", "true", "yes", "y", "unknown", "encrypted"}


def parse_float(value: str) -> Optional[float]:
    try:
        if value is None or str(value).strip() == "":
            return None
        return float(value)
    except (TypeError, ValueError):
        return None


def load_threshold(path: Optional[Path]) -> Optional[float]:
    if path is None:
        return None
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return None
    for key in ("threshold", "score_threshold", "attack_threshold"):
        value = parse_float(payload.get(key, ""))
        if value is not None:
            return value
    return None


def normalize_truth(row: Dict[str, str]) -> Tuple[bool, str, bool, bool]:
    label = (
        row.get("ground_truth")
        or row.get("truth")
        or row.get("label")
        or row.get("class")
        or ""
    ).strip().lower()
    is_unknown = parse_bool(row.get("is_unknown", "")) or "unknown" in label
    is_encrypted = parse_bool(row.get("is_encrypted", "")) or "encrypted" in label
    if label in NORMAL_LABELS:
        return False, "normal", is_unknown, is_encrypted
    if label in ATTACK_LABELS or label:
        if is_unknown:
            return True, "unknown_attack", True, is_encrypted
        if is_encrypted:
            return True, "encrypted_attack", False, True
        return True, "known_attack", False, False
    return False, "normal", is_unknown, is_encrypted


def normalize_prediction(row: Dict[str, str], locked_threshold: Optional[float]) -> Tuple[bool, str]:
    label = (
        row.get("prediction")
        or row.get("predicted_label")
        or row.get("predicted_class")
        or row.get("label")
        or ""
    ).strip().lower()
    threshold = parse_float(row.get("threshold", "")) or locked_threshold
    score = parse_float(row.get("score", "")) or parse_float(row.get("attack_score", ""))

    if label in NORMAL_LABELS:
        return False, "normal"
    if label in ATTACK_LABELS:
        if "unknown" in label:
            return True, "unknown_attack"
        if "encrypted" in label:
            return True, "encrypted_attack"
        return True, "attack"
    if not label and score is not None and threshold is not None:
        return score >= threshold, "attack" if score >= threshold else "normal"
    if label:
        return True, label
    return False, "normal"


def wilson_interval(successes: int, total: int, z: float = 1.96) -> Dict[str, Optional[float]]:
    if total <= 0:
        return {"point": None, "lower": None, "upper": None, "successes": successes, "total": total}
    point = successes / total
    denom = 1 + (z * z / total)
    center = (point + (z * z) / (2 * total)) / denom
    spread = z * math.sqrt((point * (1 - point) / total) + (z * z / (4 * total * total))) / denom
    return {
        "point": point,
        "lower": max(0.0, center - spread),
        "upper": min(1.0, center + spread),
        "successes": successes,
        "total": total,
    }


def inventory_package(package_dir: Path) -> List[Dict[str, Any]]:
    files = []
    for path in sorted(p for p in package_dir.rglob("*") if p.is_file()):
        files.append(
            {
                "path": str(path.relative_to(package_dir)),
                "bytes": path.stat().st_size,
                "sha256": sha256_file(path),
            }
        )
    return files


def duplicate_ids(rows: List[Dict[str, str]]) -> List[str]:
    counts = Counter(row.get("sample_id", "") for row in rows)
    return sorted(sample_id for sample_id, count in counts.items() if sample_id and count > 1)


def map_by_sample_id(rows: List[Dict[str, str]]) -> Dict[str, Dict[str, str]]:
    return {row.get("sample_id", ""): row for row in rows if row.get("sample_id", "")}


def read_artifact_text(path: Path, limit: int = 1024 * 1024) -> str:
    with path.open("rb") as handle:
        return handle.read(limit).decode("utf-8", errors="replace")


def template_markers_in_artifact(path: Path) -> List[str]:
    text = read_artifact_text(path).lower()
    return sorted(marker for marker in TEMPLATE_MARKERS if marker in text)


def blank_attestation_fields(path: Path) -> List[str]:
    text = read_artifact_text(path).lower()
    blanks = []
    for field in ATTESTATION_REQUIRED_MARKERS:
        yaml_blank = f"{field}: \"\"" in text or f"{field}: ''" in text
        json_blank = f'"{field}": ""' in text or f'"{field}":""' in text
        if yaml_blank or json_blank:
            blanks.append(field)
    return blanks


def compute_metrics(
    labels: List[Dict[str, str]],
    predictions: List[Dict[str, str]],
    locked_threshold: Optional[float],
) -> Tuple[Dict[str, Any], List[Dict[str, Any]], List[Dict[str, Any]]]:
    label_by_id = map_by_sample_id(labels)
    prediction_by_id = map_by_sample_id(predictions)
    common_ids = sorted(set(label_by_id) & set(prediction_by_id))

    binary = Counter()
    truth_family = Counter()
    pred_family = Counter()
    family_matrix: Dict[str, Counter] = defaultdict(Counter)
    stratum_counters: Dict[str, Counter] = defaultdict(Counter)

    for sample_id in common_ids:
        truth_attack, truth_label, is_unknown, is_encrypted = normalize_truth(label_by_id[sample_id])
        pred_attack, pred_label = normalize_prediction(prediction_by_id[sample_id], locked_threshold)
        truth_family[truth_label] += 1
        pred_family[pred_label] += 1
        family_matrix[truth_label][pred_label] += 1

        if truth_attack and pred_attack:
            binary["tp"] += 1
        elif truth_attack and not pred_attack:
            binary["fn"] += 1
        elif not truth_attack and pred_attack:
            binary["fp"] += 1
        else:
            binary["tn"] += 1

        strata = []
        if not truth_attack:
            strata.append("normal")
        elif is_unknown:
            strata.append("unknown_attack")
        else:
            strata.append("known_attack")
        if is_encrypted:
            strata.append("encrypted")

        for stratum in strata:
            stratum_counters[stratum]["total"] += 1
            if truth_attack:
                stratum_counters[stratum]["attack_total"] += 1
                if pred_attack:
                    stratum_counters[stratum]["detected"] += 1
            else:
                stratum_counters[stratum]["normal_total"] += 1
                if pred_attack:
                    stratum_counters[stratum]["false_positive"] += 1

    attack_total = binary["tp"] + binary["fn"]
    normal_total = binary["tn"] + binary["fp"]
    total = attack_total + normal_total
    unknown_total = stratum_counters["unknown_attack"]["attack_total"]
    unknown_detected = stratum_counters["unknown_attack"]["detected"]
    encrypted_attack_total = stratum_counters["encrypted"]["attack_total"]
    encrypted_detected = stratum_counters["encrypted"]["detected"]

    metrics = {
        "sample_count": total,
        "matched_sample_count": len(common_ids),
        "unmatched_label_count": len(set(label_by_id) - set(prediction_by_id)),
        "extra_prediction_count": len(set(prediction_by_id) - set(label_by_id)),
        "confusion_matrix": {
            "tn": binary["tn"],
            "fp": binary["fp"],
            "fn": binary["fn"],
            "tp": binary["tp"],
        },
        "accuracy": wilson_interval(binary["tp"] + binary["tn"], total),
        "detection_rate": wilson_interval(binary["tp"], attack_total),
        "false_positive_rate": wilson_interval(binary["fp"], normal_total),
        "false_negative_rate": wilson_interval(binary["fn"], attack_total),
        "unknown_recall": wilson_interval(unknown_detected, unknown_total),
        "encrypted_attack_detection_rate": wilson_interval(encrypted_detected, encrypted_attack_total),
        "truth_label_counts": dict(truth_family),
        "prediction_label_counts": dict(pred_family),
    }

    matrix_rows = []
    for truth_label in sorted(family_matrix):
        for pred_label in sorted(family_matrix[truth_label]):
            matrix_rows.append(
                {
                    "truth_label": truth_label,
                    "prediction_label": pred_label,
                    "count": family_matrix[truth_label][pred_label],
                }
            )

    stratum_rows = []
    for stratum in sorted(stratum_counters):
        counters = stratum_counters[stratum]
        if stratum == "normal":
            interval = wilson_interval(counters["false_positive"], counters["normal_total"])
            metric_name = "false_positive_rate"
        else:
            interval = wilson_interval(counters["detected"], counters["attack_total"])
            metric_name = "detection_rate"
        stratum_rows.append(
            {
                "stratum": stratum,
                "metric": metric_name,
                "successes": interval["successes"],
                "total": interval["total"],
                "point": interval["point"],
                "ci95_lower": interval["lower"],
                "ci95_upper": interval["upper"],
            }
        )

    return metrics, matrix_rows, stratum_rows


def result_from_checks(checks: List[Dict[str, Any]]) -> str:
    blockers = [check for check in checks if check["severity"] == "blocker" and not check["passed"]]
    if blockers:
        return "blocked"
    warnings = [check for check in checks if check["severity"] == "warn" and not check["passed"]]
    if warnings:
        return "warn"
    return "pass"


def format_rate(interval: Dict[str, Optional[float]]) -> str:
    if interval["point"] is None:
        return "n/a"
    return f"{interval['point']:.4f} [{interval['lower']:.4f}, {interval['upper']:.4f}]"


def write_markdown_report(path: Path, summary: Dict[str, Any]) -> None:
    checks = summary["checks"]
    metrics = summary.get("metrics") or {}
    lines = [
        "# Detection Quality Blind Package Preflight",
        "",
        f"- Run ID: `{summary['run_id']}`",
        f"- Result: `{summary['result']}`",
        f"- Package: `{summary['package_dir']}`",
        f"- Generated at: `{summary['generated_at']}`",
        "",
        "## Gate Thresholds",
        "",
        f"- Detection rate lower 95% CI >= {summary['thresholds']['min_detection_rate']}",
        f"- False-positive rate upper 95% CI <= {summary['thresholds']['max_false_positive_rate']}",
        f"- Unknown recall lower 95% CI >= {summary['thresholds']['min_unknown_recall']}",
        "",
        "## Metrics",
        "",
    ]
    if metrics:
        lines.extend(
            [
                f"- Samples matched: {metrics['matched_sample_count']}",
                f"- Confusion matrix: TN={metrics['confusion_matrix']['tn']}, FP={metrics['confusion_matrix']['fp']}, FN={metrics['confusion_matrix']['fn']}, TP={metrics['confusion_matrix']['tp']}",
                f"- Detection rate: {format_rate(metrics['detection_rate'])}",
                f"- False-positive rate: {format_rate(metrics['false_positive_rate'])}",
                f"- Unknown recall: {format_rate(metrics['unknown_recall'])}",
                f"- Encrypted attack detection rate: {format_rate(metrics['encrypted_attack_detection_rate'])}",
            ]
        )
    else:
        lines.append("- Metrics were not computed because required blind labels or predictions are missing.")

    lines.extend(
        [
            "",
            "## Checks",
            "",
            "| Phase | Check | Severity | Status | Detail |",
            "|---|---|---:|---|---|",
        ]
    )
    for check in checks:
        passed = "pass" if check["passed"] else "fail"
        detail = str(check.get("detail", "")).replace("|", "\\|")
        lines.append(
            f"| {check['phase']} | {check['name']} | {check['severity']} | {passed}/{check['status']} | {detail} |"
        )

    blockers = [check for check in checks if check["severity"] == "blocker" and not check["passed"]]
    lines.extend(["", "## Blockers", ""])
    if blockers:
        for check in blockers:
            lines.append(f"- {check['name']}: {check['detail']}")
    else:
        lines.append("- None.")

    lines.extend(
        [
            "",
            "## Integrity Note",
            "",
            "This preflight does not create sample labels or predictions. A passing result requires frozen blind-package artifacts, locked thresholds, metric evidence, and third-party attestation.",
            "",
        ]
    )
    path.write_text("\n".join(lines), encoding="utf-8")


def evaluate_package(package_dir: Path, output_dir: Path, run_id: str, thresholds: Thresholds) -> Dict[str, Any]:
    output_dir.mkdir(parents=True, exist_ok=True)
    checks: List[Dict[str, Any]] = []
    artifacts = {name: find_artifact(package_dir, candidates) for name, candidates in ACTUAL_ARTIFACT_CANDIDATES.items()}
    inventory = inventory_package(package_dir)

    for required in REQUIRED_CONTRACT_FILES:
        path = package_dir / required
        add_check(
            checks,
            "contract",
            f"{required} present",
            "info" if path.is_file() else "blocker",
            path.is_file(),
            "ok" if path.is_file() else "missing",
            str(path.relative_to(package_dir)) if path.is_file() else f"missing {required}",
            required,
        )

    for name, path in artifacts.items():
        required = name != "third_party_attestation"
        add_check(
            checks,
            "package",
            f"{name.replace('_', ' ')} present",
            "blocker" if required else "blocker",
            path is not None,
            "ok" if path is not None else "missing",
            str(path.relative_to(package_dir)) if path is not None else f"missing {ACTUAL_ARTIFACT_CANDIDATES[name][0]}",
            str(path.relative_to(package_dir)) if path is not None else "",
        )
        if path is not None:
            markers = template_markers_in_artifact(path)
            add_check(
                checks,
                "integrity",
                f"{name.replace('_', ' ')} is not bootstrap or review template",
                "blocker",
                not markers,
                "ok" if not markers else "review_required",
                "no review-required/template markers" if not markers else f"markers={','.join(markers)}",
                str(path.relative_to(package_dir)),
            )
            if name == "third_party_attestation":
                blank_fields = blank_attestation_fields(path)
                add_check(
                    checks,
                    "integrity",
                    "third-party attestation has signature fields filled",
                    "blocker",
                    not blank_fields,
                    "ok" if not blank_fields else "unsigned",
                    "signature fields present" if not blank_fields else f"blank_fields={','.join(blank_fields)}",
                    str(path.relative_to(package_dir)),
                )

    add_check(
        checks,
        "integrity",
        "bootstrap and review-template artifacts are blocked from formal pass",
        "info",
        True,
        "ok",
        "formal artifacts are scanned for review_required/template markers and unsigned attestation fields",
        "mlops/scripts/evaluate_blind_package.py",
    )

    labels: List[Dict[str, str]] = []
    predictions: List[Dict[str, str]] = []
    metrics: Optional[Dict[str, Any]] = None
    matrix_rows: List[Dict[str, Any]] = []
    stratum_rows: List[Dict[str, Any]] = []

    locked_threshold = load_threshold(artifacts["threshold_lock"])
    if artifacts["threshold_lock"] is not None:
        add_check(
            checks,
            "package",
            "threshold lock has numeric threshold",
            "blocker",
            locked_threshold is not None,
            "ok" if locked_threshold is not None else "invalid",
            f"threshold={locked_threshold}" if locked_threshold is not None else "threshold-lock.json lacks threshold",
            str(artifacts["threshold_lock"].relative_to(package_dir)),
        )

    if artifacts["labels"] is not None:
        labels = read_csv_rows(artifacts["labels"])
        label_columns = set(labels[0].keys()) if labels else set()
        missing = sorted(REQUIRED_LABEL_COLUMNS - label_columns)
        add_check(
            checks,
            "labels",
            "labels.csv schema",
            "blocker",
            not missing and bool(labels),
            "ok" if not missing and labels else "invalid",
            f"{len(labels)} rows" if not missing and labels else f"missing columns={missing} rows={len(labels)}",
            str(artifacts["labels"].relative_to(package_dir)),
        )
        dups = duplicate_ids(labels)
        add_check(
            checks,
            "labels",
            "labels sample_id uniqueness",
            "blocker",
            not dups,
            "ok" if not dups else "duplicate",
            "unique" if not dups else f"{len(dups)} duplicate IDs",
            str(artifacts["labels"].relative_to(package_dir)),
        )

    if artifacts["predictions"] is not None:
        predictions = read_csv_rows(artifacts["predictions"])
        pred_columns = set(predictions[0].keys()) if predictions else set()
        missing_id = REQUIRED_PREDICTION_ID_COLUMN not in pred_columns
        has_prediction_or_score = bool({"prediction", "predicted_label", "predicted_class", "score", "attack_score"} & pred_columns)
        add_check(
            checks,
            "predictions",
            "predictions.csv schema",
            "blocker",
            bool(predictions) and not missing_id and has_prediction_or_score,
            "ok" if bool(predictions) and not missing_id and has_prediction_or_score else "invalid",
            f"{len(predictions)} rows" if bool(predictions) and not missing_id and has_prediction_or_score else f"columns={sorted(pred_columns)}",
            str(artifacts["predictions"].relative_to(package_dir)),
        )
        dups = duplicate_ids(predictions)
        add_check(
            checks,
            "predictions",
            "predictions sample_id uniqueness",
            "blocker",
            not dups,
            "ok" if not dups else "duplicate",
            "unique" if not dups else f"{len(dups)} duplicate IDs",
            str(artifacts["predictions"].relative_to(package_dir)),
        )

    if labels and predictions:
        label_ids = set(map_by_sample_id(labels))
        prediction_ids = set(map_by_sample_id(predictions))
        missing_predictions = sorted(label_ids - prediction_ids)
        extra_predictions = sorted(prediction_ids - label_ids)
        add_check(
            checks,
            "data",
            "labels and predictions sample_id match",
            "blocker",
            not missing_predictions and not extra_predictions,
            "ok" if not missing_predictions and not extra_predictions else "mismatch",
            f"missing_predictions={len(missing_predictions)} extra_predictions={len(extra_predictions)}",
            "",
        )

        metrics, matrix_rows, stratum_rows = compute_metrics(labels, predictions, locked_threshold)

        truth_counts = metrics["truth_label_counts"]
        has_normal = truth_counts.get("normal", 0) > 0
        known_attack_total = next((row["total"] for row in stratum_rows if row["stratum"] == "known_attack"), 0)
        has_known = known_attack_total > 0
        has_unknown = truth_counts.get("unknown_attack", 0) > 0
        encrypted_total = next((row["total"] for row in stratum_rows if row["stratum"] == "encrypted"), 0)
        add_check(checks, "data", "normal stratum present", "blocker", has_normal, "ok" if has_normal else "missing", str(truth_counts), "")
        add_check(checks, "data", "known attack stratum present", "blocker", has_known, "ok" if has_known else "missing", str(truth_counts), "")
        add_check(checks, "data", "unknown attack stratum present", "blocker", has_unknown, "ok" if has_unknown else "missing", str(truth_counts), "")
        add_check(checks, "data", "encrypted stratum present", "blocker", encrypted_total > 0, "ok" if encrypted_total > 0 else "missing", f"encrypted={encrypted_total}", "")

        detection = metrics["detection_rate"]
        fpr = metrics["false_positive_rate"]
        unknown = metrics["unknown_recall"]
        add_check(
            checks,
            "metrics",
            "detection rate lower 95% CI meets gate",
            "blocker",
            detection["lower"] is not None and detection["lower"] >= thresholds.min_detection_rate,
            "ok" if detection["lower"] is not None and detection["lower"] >= thresholds.min_detection_rate else "below_gate",
            format_rate(detection),
            "blind-evaluation-summary.json",
        )
        add_check(
            checks,
            "metrics",
            "false-positive rate upper 95% CI meets gate",
            "blocker",
            fpr["upper"] is not None and fpr["upper"] <= thresholds.max_false_positive_rate,
            "ok" if fpr["upper"] is not None and fpr["upper"] <= thresholds.max_false_positive_rate else "above_gate",
            format_rate(fpr),
            "blind-evaluation-summary.json",
        )
        add_check(
            checks,
            "metrics",
            "unknown recall lower 95% CI meets gate",
            "blocker",
            unknown["lower"] is not None and unknown["lower"] >= thresholds.min_unknown_recall,
            "ok" if unknown["lower"] is not None and unknown["lower"] >= thresholds.min_unknown_recall else "below_gate",
            format_rate(unknown),
            "blind-evaluation-summary.json",
        )

    summary = {
        "run_id": run_id,
        "generated_at": utc_now(),
        "package_dir": str(package_dir),
        "output_dir": str(output_dir),
        "thresholds": {
            "min_detection_rate": thresholds.min_detection_rate,
            "max_false_positive_rate": thresholds.max_false_positive_rate,
            "min_unknown_recall": thresholds.min_unknown_recall,
        },
        "locked_threshold": locked_threshold,
        "artifacts": {
            name: str(path.relative_to(package_dir)) if path is not None else None
            for name, path in artifacts.items()
        },
        "inventory_file_count": len(inventory),
        "metrics": metrics,
        "checks": checks,
    }
    summary["result"] = result_from_checks(checks)

    (output_dir / "package-file-inventory.json").write_text(
        json.dumps(inventory, indent=2, ensure_ascii=True), encoding="utf-8"
    )
    (output_dir / "blind-evaluation-summary.json").write_text(
        json.dumps(summary, indent=2, ensure_ascii=True), encoding="utf-8"
    )
    write_csv(output_dir / "confusion-matrix.csv", matrix_rows, ["truth_label", "prediction_label", "count"])
    write_csv(
        output_dir / "stratum-metrics.csv",
        stratum_rows,
        ["stratum", "metric", "successes", "total", "point", "ci95_lower", "ci95_upper"],
    )
    write_markdown_report(output_dir / "blind-evaluation-report.md", summary)
    return summary


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Evaluate topic-1 blind detection-quality package")
    parser.add_argument("--package-dir", default="mlops/eval_packages/topic1_blind")
    parser.add_argument("--output-dir", required=True)
    parser.add_argument("--run-id", default=f"{dt.datetime.now():%Y%m%d%H%M%S}-detection-quality-preflight")
    parser.add_argument("--min-detection-rate", type=float, default=0.95)
    parser.add_argument("--max-false-positive-rate", type=float, default=0.05)
    parser.add_argument("--min-unknown-recall", type=float, default=0.80)
    parser.add_argument("--fail-on-blockers", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    summary = evaluate_package(
        Path(args.package_dir),
        Path(args.output_dir),
        args.run_id,
        Thresholds(
            min_detection_rate=args.min_detection_rate,
            max_false_positive_rate=args.max_false_positive_rate,
            min_unknown_recall=args.min_unknown_recall,
        ),
    )
    print(json.dumps({"run_id": summary["run_id"], "result": summary["result"]}, ensure_ascii=True))
    if args.fail_on_blockers and summary["result"] == "blocked":
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

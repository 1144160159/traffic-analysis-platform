# Topic 1 Blind Detection-Quality Package

This directory is the repository contract for the GATE-P0-06 blind evaluation package. It separates model training/test-set metrics from acceptance-grade evidence for detection rate, false-positive rate, unknown-attack recall, and encrypted-sample coverage.

## Required Frozen Artifacts

The package is ready for evaluation only when these files exist:

| File | Purpose |
|---|---|
| `dataset-manifest.yaml` | Frozen package metadata, sample strata counts, source hashes, and storage URIs. |
| `threshold-lock.json` | The locked model, feature set, and attack-score threshold used before predictions are generated. |
| `labels/labels.csv` | Blind labels held by the evaluator. Do not expose this file to the prediction run. |
| `predictions/predictions.csv` | Model predictions generated without access to labels. |
| `third-party-attestation.yaml` | CNAS or third-party reviewer attestation, tool version, and signature record. |

Large samples should stay in immutable object storage. Commit only manifests, hashes, schemas, and signed reports unless the sample owner explicitly approves storing a sanitized subset in the repository.

## Evaluation

Run:

```bash
ALLOW_BLOCKERS=true RUN_ID=20260629-detection-quality-preflight-r1 tests/e2e/live_detection_quality_preflight.sh
```

The evaluator writes:

- `blind-evaluation-summary.json`
- `blind-evaluation-report.md`
- `confusion-matrix.csv`
- `stratum-metrics.csv`
- `package-file-inventory.json`

A passing result requires all frozen artifacts, matching sample IDs, required strata, locked thresholds, 95% Wilson confidence-interval gates, and third-party attestation. Missing real artifacts produce a `blocked` result by design.

# Detection Quality Metric Definition

## Binary Decision

`normal` samples are negative. Known, unknown, and encrypted attacks are positive. Predictions are positive when `prediction` is an attack label, or when no label is supplied and `score >= threshold-lock.json.threshold`.

## Core Metrics

| Metric | Formula | Gate |
|---|---|---|
| Detection rate | `TP / (TP + FN)` | 95% Wilson lower confidence bound must be `>= 0.95`. |
| False-positive rate | `FP / (FP + TN)` | 95% Wilson upper confidence bound must be `<= 0.05`. |
| False-negative rate | `FN / (TP + FN)` | Reported for diagnostics. |
| Unknown recall | `unknown_detected / unknown_attack_total` | 95% Wilson lower confidence bound must be `>= 0.80` unless the task book sets a stricter threshold. |
| Encrypted attack detection rate | `encrypted_detected / encrypted_attack_total` | Reported and included in stratum evidence. |

## Acceptance Rules

1. The package must include normal, known attack, unknown attack, and encrypted strata.
2. `labels.csv` and `predictions.csv` must have matching unique `sample_id` values.
3. Thresholds must be locked before prediction generation.
4. Metrics must be computed from frozen labels and predictions, not from training or tuning data.
5. Third-party attestation is required before GATE-P0-06 can be marked as passed.

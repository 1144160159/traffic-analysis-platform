# Detection Quality Metric Definition

> 口径说明（2026-08-16 与任务书"预警准确率 ≥ 95%、误报率 < 5%"对齐）：
> **预警准确率**按事前签字方法（本文件第 11 行）由 **Detection rate
> `TP / (TP + FN)`（攻击召回）** 承载，采用 95% Wilson 置信下界 `>= 0.95`
> 判定；**Accuracy `(TP+TN)/total`** 同时输出供透明对照，但不参与门禁判定。
> **误报率** = `FP / (FP + TN)`，采用 95% Wilson 置信上界 `<= 0.05` 判定。
> 评估脚本（`evaluate_blind_package.py`）对 accuracy 与 detection rate 双口径
> 均输出；任何一处口径歧义都不得改变已签字公式的数值。

## Binary Decision

`normal` samples are negative. Known, unknown, and encrypted attacks are positive. Predictions are positive when `prediction` is an attack label, or when no label is supplied and `score >= threshold-lock.json.threshold`.

## Core Metrics

| Metric | Formula | Gate |
|---|---|---|
| Detection rate（预警准确率，签字方法） | `TP / (TP + FN)` | 95% Wilson lower confidence bound must be `>= 0.95`. |
| Accuracy（诊断对照，不参与门禁） | `(TP + TN) / (TP + TN + FP + FN)` | Reported alongside detection rate for transparency only. |
| False-positive rate（误报率） | `FP / (FP + TN)` | 95% Wilson upper confidence bound must be `<= 0.05`. |
| False-negative rate | `FN / (TP + FN)` | Reported for diagnostics. |
| Unknown recall | `unknown_detected / unknown_attack_total` | 95% Wilson lower confidence bound must be `>= 0.80` unless the task book sets a stricter threshold. |
| Encrypted attack detection rate | `encrypted_detected / encrypted_attack_total` | Reported and included in stratum evidence. |

## Acceptance Rules

1. The package must include normal, known attack, unknown attack, and encrypted strata.
2. `labels.csv` and `predictions.csv` must have matching unique `sample_id` values.
3. Thresholds must be locked before prediction generation.
4. Metrics must be computed from frozen labels and predictions, not from training or tuning data.
5. Third-party attestation is required before GATE-P0-06 can be marked as passed.
6. All `ground_truth` values in `labels.csv` must be one of the allowed aliases in `label-schema.yaml` (`allowed_ground_truth`); unrecognized labels are invalid and block the gate instead of being counted as an attack or as normal.

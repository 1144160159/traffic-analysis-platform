# 检测质量验收证据

本目录承载 GATE-P0-06 的稳定证据副本。它只证明盲测包预检和指标计算结果，不把 MLOps 训练集/测试集内部 F1、AUC 自动外推为任务书的 95% 检出率和 5% 误报率。

## 当前入口

- 盲测包契约：`mlops/eval_packages/topic1_blind/`
- 预检脚本：`tests/e2e/live_detection_quality_preflight.sh`
- 指标计算器：`mlops/scripts/evaluate_blind_package.py`
- 候选包草案工具：`tests/e2e/live_detection_quality_package_bootstrap.sh`
- 盲测复核包工具：`tests/e2e/live_detection_quality_review_packet.sh`

## 稳定证据

运行预检后会刷新：

- `detection-quality-preflight-latest.json`
- `detection-quality-preflight-latest.md`
- `package-file-inventory-latest.json`
- `confusion-matrix-latest.csv`
- `stratum-metrics-latest.csv`
- `dataset-manifest.template.yaml`
- `label-schema.yaml`
- `metric-definition.md`

如果缺少冻结 `dataset-manifest.yaml`、`threshold-lock.json`、真实 `labels.csv`、真实 `predictions.csv` 或第三方签认，结果必须保持 `blocked`。

## Bootstrap 草案

`tests/e2e/live_detection_quality_package_bootstrap.sh` 只读真实 APISIX `/api/v1/alerts` 和 `/api/v1/fusion/value-report`，生成 `bootstrap/latest/` 下的候选样本与评审模板，用于缩短第三方盲测材料准备时间。

最新草案 `20260630-detection-quality-bootstrap-r1` 导出 45 个 live alert 候选样本，稳定入口：

- `bootstrap/detection-quality-bootstrap-latest.json`
- `bootstrap/detection-quality-bootstrap-latest.md`
- `bootstrap/latest/sample-index.bootstrap.csv`
- `bootstrap/latest/labels/labels.review-template.csv`
- `bootstrap/latest/predictions/predictions.review-template.csv`
- `bootstrap/latest/reports/third-party-attestation.review-template.yaml`

这些文件均为 `review_required` / `review-template`，不会生成正式的 `labels.csv`、`predictions.csv`、`dataset-manifest.yaml`、`threshold-lock.json` 或 `third-party-attestation.yaml`。因此它们不能关闭 GATE-P0-06，只能作为第三方冻结、标注、预测和签认前的准备材料。

## Review Packet

`tests/e2e/live_detection_quality_review_packet.sh` 会读取 `bootstrap/latest/`，生成 `review/latest/` 下的盲测复核工作台：

- `review/detection-quality-review-latest.json`
- `review/detection-quality-review-latest.md`
- `review/latest/sample-review.csv`
- `review/latest/labeling-worklist.csv`
- `review/latest/prediction-worklist.csv`
- `review/latest/formal-package-manifest.template.yaml`
- `review/latest/threshold-lock.template.json`
- `review/latest/third-party-attestation.template.yaml`
- `review/latest/review-checklist.md`

当前 `20260701-detection-quality-review-r1` 为 `pass`：45 个候选样本、0 个重复 `sample_id`、6/6 checks passed、0 blockers、0 warnings。该包只把第三方标注、无标签预测、阈值锁定和签认步骤整理为复核工作台，不会写入 `mlops/eval_packages/topic1_blind/` 的正式 artifact。

## 最新正式预检

`20260701-detection-quality-preflight-r5-review-packet` 仍为 `blocked`：5/10 checks passed、5 blockers、0 warnings。当前阻断项是缺正式 `dataset-manifest.yaml`、`threshold-lock.json`、`labels.csv`、`predictions.csv` 和 `third-party-attestation.yaml`。

`mlops/scripts/evaluate_blind_package.py` 已加入正式包完整性 guard：所有正式 artifact 会扫描 bootstrap/review-template 标记，第三方签认还必须填入 `signed_by` 和 `signed_at`。本轮已用临时合成包把 bootstrap/template 文件改名成正式路径验证 guard 会阻断，避免模板文件被误当作 GATE-P0-06 正式盲测结论。

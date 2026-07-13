# Detection Quality Review Checklist

1. Freeze the sample set and immutable source artifacts.
2. Fill `labeling-worklist.csv` without exposing labels to the prediction runner.
3. Lock threshold in `threshold-lock.template.json` before generating predictions.
4. Fill `prediction-worklist.csv` from a no-label model run.
5. Produce `dataset-manifest.yaml`, `threshold-lock.json`, `labels/labels.csv`, `predictions/predictions.csv`, and `third-party-attestation.yaml` in `mlops/eval_packages/topic1_blind/` only after all TBD/review-template markers are removed.
6. Rerun `ALLOW_BLOCKERS=false tests/e2e/live_detection_quality_preflight.sh`.

Do not copy this review packet into the formal package unchanged.

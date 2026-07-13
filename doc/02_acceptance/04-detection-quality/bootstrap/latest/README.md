# Detection Quality Bootstrap Package

Run ID: `20260630-detection-quality-bootstrap-r1`

This directory contains review-required candidate material generated from live observed alerts. It is intentionally not a formal blind package.

## Files

- `sample-index.bootstrap.csv`: candidate sample IDs derived from live alerts.
- `labels/labels.review-template.csv`: blank label template for third-party evaluator review.
- `predictions/predictions.review-template.csv`: blank prediction template for a no-label prediction run.
- `dataset-manifest.bootstrap.json`: package draft metadata.
- `threshold-lock.review-template.json`: threshold lock template; threshold is intentionally null.
- `reports/third-party-attestation.review-template.yaml`: unsigned attestation template.

## Non-Closure Rule

Do not rename these files into the formal `topic1_blind` package until sample freezing, blind labels, locked threshold, predictions, and third-party attestation are complete.

# Detection Quality Blind Package Preflight

- Run ID: `20260630-detection-quality-preflight-r3-integrity-guard`
- Result: `blocked`
- Package: `mlops/eval_packages/topic1_blind`
- Generated at: `2026-06-30T15:06:41Z`

## Gate Thresholds

- Detection rate lower 95% CI >= 0.95
- False-positive rate upper 95% CI <= 0.05
- Unknown recall lower 95% CI >= 0.8

## Metrics

- Metrics were not computed because required blind labels or predictions are missing.

## Checks

| Phase | Check | Severity | Status | Detail |
|---|---|---:|---|---|
| contract | README.md present | info | pass/ok | README.md |
| contract | dataset-manifest.template.yaml present | info | pass/ok | dataset-manifest.template.yaml |
| contract | label-schema.yaml present | info | pass/ok | label-schema.yaml |
| contract | metric-definition.md present | info | pass/ok | metric-definition.md |
| package | dataset manifest present | blocker | fail/missing | missing dataset-manifest.yaml |
| package | threshold lock present | blocker | fail/missing | missing threshold-lock.json |
| package | labels present | blocker | fail/missing | missing labels.csv |
| package | predictions present | blocker | fail/missing | missing predictions.csv |
| package | third party attestation present | blocker | fail/missing | missing third-party-attestation.yaml |

## Blockers

- dataset manifest present: missing dataset-manifest.yaml
- threshold lock present: missing threshold-lock.json
- labels present: missing labels.csv
- predictions present: missing predictions.csv
- third party attestation present: missing third-party-attestation.yaml

## Integrity Note

This preflight does not create sample labels or predictions. A passing result requires frozen blind-package artifacts, locked thresholds, metric evidence, and third-party attestation.

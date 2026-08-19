# Detection Quality Review Packet

- Run ID: `20260701-detection-quality-review-r1`
- Result: `pass`
- Input bootstrap: `doc/02_acceptance/04-detection-quality/bootstrap/latest`
- Candidate samples: 45
- Duplicate sample IDs: 0
- Stable packet: `doc/02_acceptance/04-detection-quality/review/latest`

This package turns live alert candidates into a review board for blind labels, prediction execution, threshold locking, and third-party attestation. It is not a frozen blind package and cannot close GATE-P0-06.

## Files

- `sample-review.csv`: row-level sample review board
- `labeling-worklist.csv`: evaluator label worklist
- `prediction-worklist.csv`: no-label model prediction worklist
- `formal-package-manifest.template.yaml`: manifest template for the frozen package
- `threshold-lock.template.json`: threshold lock template
- `third-party-attestation.template.yaml`: attestation template
- `review-checklist.md`: freeze and rerun checklist
- `review-summary.json`: package metadata

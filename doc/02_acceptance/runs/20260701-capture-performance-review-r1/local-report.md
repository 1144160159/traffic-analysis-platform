# Capture Performance Review Packet

- Run ID: `20260701-capture-performance-review-r1`
- Result: `pass`
- Input bootstrap: `doc/02_acceptance/03-performance/bootstrap/latest`
- Targets: 2
- Review files: 7
- Stable packet: `doc/02_acceptance/03-performance/review/latest`

This package turns the 10 x 100Gbps / 512Mpps bootstrap into an operator review board for the hardware window. It is not a signed performance result and cannot close GATE-P0-03/04.

## Files

- `hardware-review.csv`: lab hardware and NIC review worklist
- `traffic-profile-review.csv`: generator and traffic profile review worklist
- `result-summary-worklist.csv`: required result summary checklist
- `formal-artifact-manifest.template.json`: formal artifact manifest template
- `operator-approval.template.md`: hardware-window approval template
- `review-checklist.md`: execution and rerun checklist
- `review-summary.json`: package metadata

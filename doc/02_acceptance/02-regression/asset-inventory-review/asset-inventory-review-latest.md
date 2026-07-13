# Asset Inventory Review Packet

- Run ID: `20260701-asset-inventory-review-r1`
- Result: `pass`
- Input inventory: `doc/02_acceptance/02-regression/asset-discovery-site-inventory.bootstrap-latest.json`
- Asset rows: 27
- Duplicate key groups: 0
- Stable packet: `doc/02_acceptance/02-regression/asset-inventory-review/latest`

This package converts the live observed asset bootstrap into review-ready files for the site owner. It is not an approved site inventory and cannot close the formal asset discovery coverage gate.

## Files

- `review-assets.csv`: row-level review worklist
- `formal-site-inventory.template.json`: template to fill after site-owner review
- `review-checklist.md`: approval checklist and rerun command
- `review-summary.json`: package metadata

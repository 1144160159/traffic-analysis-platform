# Site Asset Inventory Formal Check

- Result: `blocked`
- Generated: `2026-07-03T00:27:30.134Z`
- Input: `doc/02_acceptance/02-regression/asset-discovery-site-inventory.bootstrap-latest.json`
- Assets: `27`
- Passed: `8/16`
- Blockers: `6`
- Warnings: `2`

This check validates that a site asset inventory is formal evidence, not a bootstrap or review template. It does not approve an inventory; it only rejects files that cannot be used as formal coverage input.

## Checks

- pass: site inventory JSON parses (ok) doc/02_acceptance/02-regression/asset-discovery-site-inventory.bootstrap-latest.json
- pass: inventory is an object with an assets array (ok) formal inventory must include approval metadata next to assets
- blocker: review_required is false (review_required=true) formal coverage cannot use a review-required packet
- blocker: approved_by is filled (missing) approved_by is required
- blocker: approved_at is filled (missing) approved_at is required
- blocker: approval_evidence is filled (missing) approval_evidence is required
- blocker: approved_at is parseable (invalid_date)
- blocker: no draft markers remain in file (draft_marker_detected) blocked markers: TBD, review-template, needs_site_owner_review, bootstrap
- pass: asset count meets minimum (asset_count=27) min_assets=1
- pass: every asset has id, identity key, and expected_type (ok)
- pass: asset rows do not contain draft markers (ok)
- warn: asset rows include location context (missing_location) 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27
- pass: asset_id values are unique (ok) []
- pass: mac_address values are unique when present (ok) []
- warn: ip_address values are unique when present (duplicates) [{"value":"10.12.0.41","indexes":[1,3,5,7]},{"value":"10.12.0.31","indexes":[2,4,6,8]}]
- pass: hostname values are unique when present (ok) []


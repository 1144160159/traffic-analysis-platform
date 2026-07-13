# Site Asset Inventory Review Checklist

This packet is generated from live observed assets and is review-required.

## Review Steps

1. Open `review-assets.csv` and set `site_owner_decision` to `approve`, `modify`, or `exclude` for every row.
2. Fill `approved_hostname`, `approved_location`, and `site_owner_comment` for modified rows.
3. Remove excluded rows from `formal-site-inventory.template.json`.
4. Replace `approved_by`, `approved_at`, and `approval_evidence`; no `TBD`, `review-template`, `bootstrap`, or `needs_site_owner_review` markers may remain.
5. Rerun the formal gate:

```bash
SITE_ASSET_INVENTORY_JSON=/path/to/site-owner-approved-assets.json \
MIN_DISCOVERY_COVERAGE_PCT=95 \
ALLOW_BLOCKERS=false \
tests/e2e/live_asset_discovery_coverage_report.sh
```

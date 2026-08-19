# Fusion x Threat Intel Contract Report

- Run ID: `20260722-fusion-r624-contract`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Web URL: `http://10.0.5.8:30180`
- Web image expected: `docker.io/traffic/web-ui@sha256:49d84d77044ebc5b786773776219b3b07e9ca13504bc69846c90d365bc3cf8fc`
- Checks: 77/77 passed, blockers=0, warnings=0

## Blockers
- None

## Warnings
- None

## Evidence
- NDJSON: `doc/02_acceptance/runs/20260722-fusion-r624-contract/live-fusion-threat-intel-20260722-fusion-r624-contract.ndjson`
- Summary: `doc/02_acceptance/runs/20260722-fusion-r624-contract/live-fusion-threat-intel-20260722-fusion-r624-contract-summary.json`
- Vitest: `doc/02_acceptance/runs/20260722-fusion-r624-contract/web-vitest-fusion-threat-intel.log`
- Web deployment: `doc/02_acceptance/runs/20260722-fusion-r624-contract/web-ui-deploy-live.json`
- Web bundle marker: `doc/02_acceptance/runs/20260722-fusion-r624-contract/live-web-bundle-marker.txt`
- Fusion stats: `doc/02_acceptance/runs/20260722-fusion-r624-contract/api-fusion-stats.json`
- Fusion entities: `doc/02_acceptance/runs/20260722-fusion-r624-contract/api-fusion-entities.json`
- Threat Intel entries: `doc/02_acceptance/runs/20260722-fusion-r624-contract/api-threat-intel-entries.json`
- Fusion conflict resolution: `doc/02_acceptance/runs/20260722-fusion-r624-contract/api-fusion-conflict-resolve.json`
- Fusion rule update: `doc/02_acceptance/runs/20260722-fusion-r624-contract/api-fusion-rule-update.json`
- PG conflict row count: `doc/02_acceptance/runs/20260722-fusion-r624-contract/pg-fusion-conflict-resolution-count.txt`
- PG rule row count: `doc/02_acceptance/runs/20260722-fusion-r624-contract/pg-fusion-rule-override-count.txt`
- PG audit row count: `doc/02_acceptance/runs/20260722-fusion-r624-contract/pg-fusion-write-audit-count.txt`

## Scope

This report verifies that the Fusion page contract consumes the Threat Intel service through APISIX, maps live intelligence into Fusion source status, metrics, rows, timeline, and evidence, and writes Fusion conflict/rule actions through APISIX into PostgreSQL and audit_logs.

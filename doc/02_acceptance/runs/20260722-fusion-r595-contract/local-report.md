# Fusion x Threat Intel Contract Report

- Run ID: `20260722-fusion-r595-contract`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Web URL: `http://10.0.5.8:30180`
- Web image expected: `docker.io/traffic/web-ui@sha256:5ff71af8fd1264d198879c3fd875c09886a410cc6a405637b4cf2f5db0649823`
- Checks: 54/54 passed, blockers=0, warnings=0

## Blockers
- None

## Warnings
- None

## Evidence
- NDJSON: `doc/02_acceptance/runs/20260722-fusion-r595-contract/live-fusion-threat-intel-20260722-fusion-r595-contract.ndjson`
- Summary: `doc/02_acceptance/runs/20260722-fusion-r595-contract/live-fusion-threat-intel-20260722-fusion-r595-contract-summary.json`
- Vitest: `doc/02_acceptance/runs/20260722-fusion-r595-contract/web-vitest-fusion-threat-intel.log`
- Web deployment: `doc/02_acceptance/runs/20260722-fusion-r595-contract/web-ui-deploy-live.json`
- Web bundle marker: `doc/02_acceptance/runs/20260722-fusion-r595-contract/live-web-bundle-marker.txt`
- Fusion stats: `doc/02_acceptance/runs/20260722-fusion-r595-contract/api-fusion-stats.json`
- Fusion entities: `doc/02_acceptance/runs/20260722-fusion-r595-contract/api-fusion-entities.json`
- Threat Intel entries: `doc/02_acceptance/runs/20260722-fusion-r595-contract/api-threat-intel-entries.json`
- Fusion conflict resolution: `doc/02_acceptance/runs/20260722-fusion-r595-contract/api-fusion-conflict-resolve.json`
- Fusion rule update: `doc/02_acceptance/runs/20260722-fusion-r595-contract/api-fusion-rule-update.json`
- PG conflict row count: `doc/02_acceptance/runs/20260722-fusion-r595-contract/pg-fusion-conflict-resolution-count.txt`
- PG rule row count: `doc/02_acceptance/runs/20260722-fusion-r595-contract/pg-fusion-rule-override-count.txt`
- PG audit row count: `doc/02_acceptance/runs/20260722-fusion-r595-contract/pg-fusion-write-audit-count.txt`

## Scope

This report verifies that the Fusion page contract consumes the Threat Intel service through APISIX, maps live intelligence into Fusion source status, metrics, rows, timeline, and evidence, and writes Fusion conflict/rule actions through APISIX into PostgreSQL and audit_logs.

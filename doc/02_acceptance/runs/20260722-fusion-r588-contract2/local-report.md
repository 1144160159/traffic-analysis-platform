# Fusion x Threat Intel Contract Report

- Run ID: `20260722-fusion-r588-contract2`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Web URL: `http://10.0.5.8:30180`
- Web image expected: `docker.io/traffic/web-ui@sha256:c40d480ef739012f828bdca5038d958c211d322d8eeeb0d4ba0ab58eb0105f03`
- Checks: 35/35 passed, blockers=0, warnings=0

## Blockers
- None

## Warnings
- None

## Evidence
- NDJSON: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/live-fusion-threat-intel-20260722-fusion-r588-contract2.ndjson`
- Summary: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/live-fusion-threat-intel-20260722-fusion-r588-contract2-summary.json`
- Vitest: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/web-vitest-fusion-threat-intel.log`
- Web deployment: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/web-ui-deploy-live.json`
- Web bundle marker: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/live-web-bundle-marker.txt`
- Fusion stats: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/api-fusion-stats.json`
- Fusion entities: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/api-fusion-entities.json`
- Threat Intel entries: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/api-threat-intel-entries.json`
- Fusion conflict resolution: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/api-fusion-conflict-resolve.json`
- Fusion rule update: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/api-fusion-rule-update.json`
- PG conflict row count: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/pg-fusion-conflict-resolution-count.txt`
- PG rule row count: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/pg-fusion-rule-override-count.txt`
- PG audit row count: `doc/02_acceptance/runs/20260722-fusion-r588-contract2/pg-fusion-write-audit-count.txt`

## Scope

This report verifies that the Fusion page contract consumes the Threat Intel service through APISIX, maps live intelligence into Fusion source status, metrics, rows, timeline, and evidence, and writes Fusion conflict/rule actions through APISIX into PostgreSQL and audit_logs.

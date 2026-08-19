# Threat Intel Service Live Preflight Report

- Run ID: `20260629-threat-intel-service-r6`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Checks: 47/47 passed, blockers=0, warnings=0
- Side effect: writes/updates PostgreSQL test indicators with sources `codex-live-smoke`, `codex-live-import`, `codex-live-scheduled` and `codex-live-cross-tenant`

## Blockers
- None

## Warnings
- None

## Evidence
- NDJSON: `doc/02_acceptance/runs/20260629-threat-intel-service/live-threat-intel-service-20260629-threat-intel-service-r6.ndjson`
- Summary: `doc/02_acceptance/runs/20260629-threat-intel-service/live-threat-intel-service-20260629-threat-intel-service-r6-summary.json`
- K8s deployment: `doc/02_acceptance/runs/20260629-threat-intel-service/k8s-threat-intel-deployment.json`
- Kafka topic: `doc/02_acceptance/runs/20260629-threat-intel-service/kafka-threat-intel-topic.txt`
- Kafka events: `doc/02_acceptance/runs/20260629-threat-intel-service/kafka-threat-intel-events.txt`
- Audit upsert count: `doc/02_acceptance/runs/20260629-threat-intel-service/pg-audit-upsert-count.txt`
- Audit import count: `doc/02_acceptance/runs/20260629-threat-intel-service/pg-audit-import-count.txt`
- Audit scheduled event: `doc/02_acceptance/runs/20260629-threat-intel-service/pg-audit-scheduled-event-id.txt`
- Scheduled feed status: `doc/02_acceptance/runs/20260629-threat-intel-service/api-feed-list-scheduled.json`
- Cross tenant lookup: `doc/02_acceptance/runs/20260629-threat-intel-service/api-lookup-cross-tenant-default.json`
- Expired token lookup: `doc/02_acceptance/runs/20260629-threat-intel-service/api-lookup-expired-token.json`
- Builtin lookup: `doc/02_acceptance/runs/20260629-threat-intel-service/api-lookup-builtin-c2.json`
- Smoke lookup: `doc/02_acceptance/runs/20260629-threat-intel-service/api-lookup-smoke-entry.json`
- Scheduled lookup: `doc/02_acceptance/runs/20260629-threat-intel-service/api-scheduled-feed-lookup.json`
- Enrichment: `doc/02_acceptance/runs/20260629-threat-intel-service/api-enrich-builtin-c2.json`

## Scope

This preflight verifies the repo contract, Kubernetes workload, Kafka topic catalog entry, APISIX route, JWT/RBAC read-write gates, tenant-scoped PostgreSQL-backed upsert/lookup/list/import, cross-tenant and expired-token negative cases, scheduled feed import, synchronous audit_logs writes, threat.intel.v1 publish/consume evidence, and alert enrichment API for the Threat Intel service.

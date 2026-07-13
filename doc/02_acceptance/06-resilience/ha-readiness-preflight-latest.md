# HA Readiness Preflight

Run: `20260701-ha-readiness-preflight-r10-review-packet`

Result: `blocked`

This is a non-destructive live preflight for GATE-P0-08. It reads Kubernetes, Kafka, Flink, ClickHouse, PostgreSQL, Redis Sentinel, MinIO, and APISIX state. It does not delete pods, scale workloads, force failover, write traffic records, or rotate storage.

## Summary

| Metric | Count |
|---|---:|
| Checks | 14 |
| Passed | 13 |
| Blockers | 1 |
| Warnings | 0 |
| Running Flink jobs | 9 |
| Kafka topics inspected | 29 |
| ClickHouse replicated tables | 13 |
| PostgreSQL streaming replicas | 2 |
| RTO/RPO drill evidence files | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260701-ha-readiness-preflight-r10-review-packet/live-ha-readiness-preflight-20260701-ha-readiness-preflight-r10-review-packet-summary.json`
- `doc/02_acceptance/runs/20260701-ha-readiness-preflight-r10-review-packet/live-ha-readiness-preflight-20260701-ha-readiness-preflight-r10-review-packet.ndjson`
- `ha-workload-readiness.json`
- `ha-pdb-readiness.json`
- `ha-pvc-readiness.json`
- `ha-endpoint-readiness.json`
- `kafka-topic-health.json`
- `flink-running-job-health.json`
- `clickhouse-replication.json`
- `postgres-replication.csv`
- `redis-sentinel-master.txt`
- `minio-health-body.txt`

## Interpretation

This preflight can prove whether the live cluster is ready for a controlled HA drill. GATE-P0-08 remains blocked until the destructive Kafka, Flink, ClickHouse, PostgreSQL, and MinIO failover drills are run in a maintenance window and produce RTO/RPO plus data-consistency reports.

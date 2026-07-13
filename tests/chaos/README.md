# Chaos And Resilience Tests

This directory contains the gated resilience workflow for GATE-P0-08.

`live_ha_readiness_preflight.sh` is read-only. It checks live Kubernetes readiness, Kafka ISR, Flink checkpoint and exception state, ClickHouse replication, PostgreSQL streaming replicas, Redis Sentinel, MinIO health, and APISIX reachability. It also records whether destructive RTO/RPO drill evidence already exists.

`ha_drill_plan.yaml` is the maintenance-window drill contract. It documents the destructive injections that must be run with human approval before GATE-P0-08 can close. The preflight script intentionally does not execute those injections.

`live_ha_drill_evidence_bootstrap.sh` is also read-only. It packages the current drill plan, latest HA readiness result, operator approval template, timeline template, failover report templates, RTO/RPO table, and evidence manifest into a review-required draft under `doc/02_acceptance/06-resilience/bootstrap/`. It deliberately writes only `*.bootstrap.*` and `*.review-template.*` artifacts and does not create the formal root-level failover or RTO/RPO reports required by `live_ha_readiness_preflight.sh`.

`live_ha_drill_review_packet.sh` is read-only as well. It turns the bootstrap draft into a maintenance-window review board under `doc/02_acceptance/06-resilience/review/`, including component drill review, RTO/RPO evidence worklist, approval template, formal artifact manifest template, data-consistency checklist, and operator checklist. It also keeps `formal_artifact_count=0`, so it cannot close GATE-P0-08 by itself.

Run:

```bash
ALLOW_BLOCKERS=true tests/chaos/live_ha_readiness_preflight.sh
RUN_ID=20260630-ha-drill-evidence-bootstrap-r1 tests/chaos/live_ha_drill_evidence_bootstrap.sh
RUN_ID=20260701-ha-drill-review-r1 tests/chaos/live_ha_drill_review_packet.sh
```

Evidence is written to `doc/02_acceptance/runs/20260629-ha-readiness-preflight/` and stable latest copies are written to `doc/02_acceptance/06-resilience/`.

# Architecture Evolution: CLE-P0-SCREEN-001

- run_id: `mvp-5-workflow-screen`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Evolution Principle
- Evolve one contract boundary at a time and keep each step reversible or compatibility-preserving.
- Put product behavior, API contract, data sensitivity and verification gate in writing before code changes.
- Treat this package as design evidence, not as implementation evidence.

## Recommended Architecture Step
- Keep `/screen` protected by default; allow display-wall usage only through an explicit read-only token with scoped tenant/site/time-window claims, expiry, audit, and desensitized fallback data.

## Dependency Signals
- `apisix_routes` impacts: CLE-P0-ROUTE-001, CLE-P0-SEC-001
- `database_schema` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001
- `kafka_topics` impacts: CLE-P0-DLQ-001, CLE-P0-SEC-001
- `proto` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001

## Slow-evolution Slices For `/screen`
- Slice 1: make auth strategy explicit and verify unauthorized behavior.
- Slice 2: centralize route/menu metadata so `/screen` is not a hidden exception.
- Slice 3: add scoped read-only display-token capability only if product owners confirm display-wall need.
- Slice 4: align realtime refresh and API polling with the same auth/tenant boundary.
- Slice 5: run visual/browser regression after the old frontend has been backed up.

## Architecture Stop Conditions
- Do not close the task from this package alone.
- Keep smoke, regression, acceptance and third-party evidence as separate evidence layers.
- Prefer existing repository contracts and UI documents over newly invented behavior.

# Data Contract Sketch: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Data Plan
- mode: `live_existing`
- tenant: `default`
- cleanup: `none`

## Data Rules
- Prefer real API/DB/Kafka paths for verification; mock data cannot prove live integration.
- Generated live data requires run_id, tenant scoping and cleanup before execution.
- Sensitive data policy must be explicit for screenshots, browser reports and acceptance artifacts.

# Data Contract Sketch: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Storage / Data Quality`
- dependent_lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance_type: `regression`

## Data Plan
- mode: `live_existing`
- tenant: `default`
- cleanup: `none`

## Data Rules
- Prefer real API/DB/Kafka paths for verification; mock data cannot prove live integration.
- Generated live data requires run_id, tenant scoping and cleanup before execution.
- Sensitive data policy must be explicit for screenshots, browser reports and acceptance artifacts.

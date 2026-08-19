# Data Contract Sketch: CLE-P0-REVIEWER-001

- run_id: `mvp-36b-workflow-llm-reviewer-docsync`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Data Plan
- mode: `live_existing`
- tenant: `default`
- cleanup: `none`

## Data Rules
- Prefer real API/DB/Kafka paths for verification; mock data cannot prove live integration.
- Generated live data requires run_id, tenant scoping and cleanup before execution.
- Sensitive data policy must be explicit for screenshots, browser reports and acceptance artifacts.

# API Contract Sketch: CLE-P0-REVIEWER-001

- run_id: `mvp-36-workflow-llm-reviewer`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Contract Impact Declared By Task
- `proto`: `False`
- `kafka_topics`: `False`
- `database_schema`: `False`
- `apisix_routes`: `False`

## API Shape To Confirm Before Implementation
- Name the existing API/service/repository layer before creating a new endpoint.
- If new API is needed, define auth, tenant, audit, pagination and error semantics first.
- If no API is changed, record that this package is UI/doc/verification-only.

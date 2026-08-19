# API Contract Sketch: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Contract Impact Declared By Task
- `proto`: `True`
- `kafka_topics`: `False`
- `database_schema`: `True`
- `apisix_routes`: `False`

## API Shape To Confirm Before Implementation
- Name the existing API/service/repository layer before creating a new endpoint.
- If new API is needed, define auth, tenant, audit, pagination and error semantics first.
- If no API is changed, record that this package is UI/doc/verification-only.

# API Contract Sketch: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Storage / Data Quality`
- dependent_lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance_type: `regression`

## Contract Impact Declared By Task
- `proto`: `True`
- `kafka_topics`: `True`
- `database_schema`: `True`
- `apisix_routes`: `False`

## API Shape To Confirm Before Implementation
- Name the existing API/service/repository layer before creating a new endpoint.
- If new API is needed, define auth, tenant, audit, pagination and error semantics first.
- If no API is changed, record that this package is UI/doc/verification-only.

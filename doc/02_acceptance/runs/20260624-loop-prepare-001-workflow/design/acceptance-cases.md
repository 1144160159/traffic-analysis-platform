# Acceptance Cases: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Storage / Data Quality`
- dependent_lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance_type: `regression`

## Evidence Layer
- declared_acceptance_type: `regression`
- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.

## Close Conditions To Preserve
- bad message injection path exists
- manual repair, approval, replay and audit are represented
- idempotency key and duplicate-write evidence are recorded

## Local Verification Candidates
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh proto`

## Live-readonly Verification Candidates
- `curl --noproxy '*' http://10.0.5.8:30180/login`

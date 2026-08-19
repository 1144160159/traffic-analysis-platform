# Acceptance Cases: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Evidence Layer
- declared_acceptance_type: `acceptance-prep`
- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.

## Close Conditions To Preserve
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

## Local Verification Candidates
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh web`

## Live-readonly Verification Candidates
- `curl --noproxy '*' http://10.0.5.8:30180/login`

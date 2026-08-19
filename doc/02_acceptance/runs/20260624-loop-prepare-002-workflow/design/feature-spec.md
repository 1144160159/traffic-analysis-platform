# Feature Spec: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Actors
- authenticated operator
- system auditor
- third-view reviewer

## Capability
- Use the documented UI suite as the visual source of truth, then migrate page by page behind regression gates.
- The feature must expose a clear product state for authorized, unauthorized, expired, degraded and empty-data scenarios.
- The frontend must surface loading and error states without fabricating successful business data.

## Functional Requirements
- Keep behavior inside the task's allowed workspace and declared lane.
- Document any change to public UX, API semantics or evidence status before implementation closes.
- Require negative cases for auth, tenant, empty data and degraded upstream dependencies when applicable.

## Acceptance-facing Behavior
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

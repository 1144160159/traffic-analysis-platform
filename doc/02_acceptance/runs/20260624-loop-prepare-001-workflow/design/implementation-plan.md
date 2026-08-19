# Implementation Plan: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Storage / Data Quality`
- dependent_lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance_type: `regression`

## Phase 0: Review Design Package
- Confirm product behavior, data sensitivity, API boundary and visual source of truth.
- Keep any unresolved blocker in DESIGN_ITERATING; do not start closure work.

## Phase 1: Protect Existing Work
- Capture git status and task snapshot before implementation.

## Phase 2: Implement Narrow Slice
- Change only files inside the task allowed paths unless a new task/design-delta expands scope.
- `go/control-plane/`
- `java/flink-jobs/`
- `proto/traffic/v1/`
- `common/`
- `doc/02_acceptance/`

## Phase 3: Verify
- Run the smallest declared local gate first.
- Add browser/API negative checks when auth or visual behavior changes.
- Record failures as repair input, not as acceptance evidence.

## Phase 4: Review And Evidence
- Run third-view review.
- Update `design-delta.md` if implementation changes product or architecture decisions.
- Keep evidence type honest: design package, smoke, regression, acceptance and third-party are not interchangeable.

## Guidance Status Suggestions
- `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.

# Implementation Plan: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Phase 0: Review Design Package
- Confirm product behavior, data sensitivity, API boundary and visual source of truth.
- Keep any unresolved blocker in DESIGN_ITERATING; do not start closure work.

## Phase 1: Protect Existing Work
- Run or explicitly account for `CLE-P0-UIBACKUP-001` before broad visual rebuild.

## Phase 2: Implement Narrow Slice
- Change only files inside the task allowed paths unless a new task/design-delta expands scope.
- `go/control-plane/`
- `java/flink-jobs/`
- `web/ui/src/`
- `proto/traffic/v1/`
- `doc/02_acceptance/`
- `doc/01_design/`

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

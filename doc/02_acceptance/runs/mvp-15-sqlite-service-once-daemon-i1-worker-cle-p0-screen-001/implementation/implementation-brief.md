# Implementation Brief: CLE-P0-SCREEN-001

- run_id: `mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- current_status: `DISCOVERED`
- execution_mode: `local`
- patch_valid: `False`

## Inputs
- context_pack: `doc/02_acceptance/runs/mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001/design`
- plan: `doc/02_acceptance/runs/mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001/plan.md`

## Allowed Paths
- `web/ui/src/`
- `web/ui/e2e/`
- `go/control-plane/internal/auth/`
- `doc/02_acceptance/`

## Close Conditions
- /screen has exactly one public/protected/readonly strategy
- unauthorized behavior is verified
- sensitive data display policy is documented

## Local Verification
- `tests/run_tests.sh web`

## Implementation Rules
- Refresh `git status --short` before editing.
- Read the context pack and exact source refs before relying on summarized text.
- Keep changes inside workspace.allowed_paths unless a design delta expands scope.
- If touching Proto, Kafka, DB schema or APISIX routes, the task contract must declare it and consumers must be checked.
- Do not treat this brief, patch validation or context evidence as task closure.
- After patching, run the task's smallest relevant verification and third-view review.

## Patch Scope
- no patch supplied

## Blocking Findings
- `GUIDANCE_BLOCKER` `CLE-P0-SCREEN-001`: SCREEN_AUTH_BOUNDARY: /screen is outside ProtectedLayout.

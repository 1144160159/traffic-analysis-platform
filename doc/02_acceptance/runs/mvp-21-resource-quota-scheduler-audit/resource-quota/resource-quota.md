# Codex Loop Resource Quota

- run_id: `mvp-21-resource-quota-scheduler-audit`
- status: `RESOURCE_QUOTA_READY`
- policy: `scripts/codex_loop/policies/resource-quotas.yaml`
- total_weight: `2`
- selected: `1`
- deferred: `0`
- findings: `0`

## Usage
- lanes: `{'UI Rebuild': 1}`
- modes: `{'local': 1}`
- subsystems: `{'web/ui': 1, 'go/control-plane/internal/auth': 1}`

## Deferred
- none

## Guardrail
- Quota evaluation only constrains scheduling; it does not execute tasks or close evidence gates.
- Per-lane quota is a control-plane prerequisite for future multi-worker execution.

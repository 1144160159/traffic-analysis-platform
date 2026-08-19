# Codex Loop Resource Quota

- run_id: `mvp-21-resource-quota-guidance`
- status: `RESOURCE_QUOTA_READY`
- policy: `scripts/codex_loop/policies/resource-quotas.yaml`
- total_weight: `4`
- selected: `2`
- deferred: `10`
- findings: `0`

## Usage
- lanes: `{'UI Rebuild': 1, 'Storage / Data Quality': 1}`
- modes: `{'local': 1, 'plan': 1}`
- subsystems: `{'web/ui': 1, 'go/control-plane/internal/auth': 1, 'go/control-plane': 1, 'java/flink-jobs': 1, 'proto': 1, 'common': 1}`

## Deferred
- `CLE-P0-UIBACKUP-001`: lane `UI Rebuild` already has 1/1 selected
- `CLE-P0-P95-001`: total resource weight would exceed 4
- `CLE-P0-PCAP-001`: total resource weight would exceed 4
- `CLE-P0-SEC-001`: total resource weight would exceed 4
- `CLE-P0-AUTH-001`: total resource weight would exceed 4
- `CLE-P0-ROUTE-001`: total resource weight would exceed 4
- `CLE-P0-REVIEWER-001`: total resource weight would exceed 4
- `CLE-P0-BASELINE-001`: total resource weight would exceed 4
- `CLE-P1-FUSION-001`: total resource weight would exceed 4
- `CLE-P1-PILOT-001`: total resource weight would exceed 4

## Guardrail
- Quota evaluation only constrains scheduling; it does not execute tasks or close evidence gates.
- Per-lane quota is a control-plane prerequisite for future multi-worker execution.

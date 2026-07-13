# 4K Static Background Inventory

> Scope: static 3840x2160 background assets for traffic-analysis-platform UI composition.
> These assets follow the same repo-managed image generation workflow as the UI image suite, but they are not counted in the 163 screenshot-style UI deliverables.

## Generation Rules

- Output format: PNG, 3840x2160.
- Storage path: `doc/04_assets/ui_suite_gpt_v1/backgrounds/4k/`.
- Prompt path: `doc/04_assets/ui_suite_gpt_v1/prompts/backgrounds-4k/`.
- Generation method: built-in image generation, then `extract_latest_imagegen.py --width 3840 --height 2160`.
- Current P0 batch: generated on 2026-06-23. Final PNG files are 3840x2160; raw imagegen outputs are retained next to each target as `*.raw-imagegen.png`.
- Current P1 batch: generated on 2026-06-23. Final PNG files are 3840x2160; raw imagegen outputs are retained next to each target as `*.raw-imagegen.png`.
- Current P2 batch: generated on 2026-06-23. Final PNG files are 3840x2160; raw imagegen outputs are retained next to each target as `*.raw-imagegen.png`.
- Visual role: static background or master visual material for UI composition, not full application screenshots.
- Avoid: readable tiny text, real credentials, real IP addresses, product logos, browser chrome, people, marketing hero layouts, one-note decorative gradients, or complete UI controls.

## P0

| Priority | ID | Role | Prompt | Target | Status |
| --- | --- | --- | --- | --- | --- |
| P0 | `bg-visual-system-master-4k` | Master visual background for the full traffic security operation loop | `prompts/backgrounds-4k/bg-visual-system-master-4k.prompt.txt` | `backgrounds/4k/bg-visual-system-master-4k.png` | generated |
| P0 | `bg-appshell-dark-grid-4k` | Reusable dark AppShell grid and texture background | `prompts/backgrounds-4k/bg-appshell-dark-grid-4k.prompt.txt` | `backgrounds/4k/bg-appshell-dark-grid-4k.png` | generated |
| P0 | `bg-campus-digital-twin-4k` | Campus network digital twin background for large-screen and overview scenes | `prompts/backgrounds-4k/bg-campus-digital-twin-4k.prompt.txt` | `backgrounds/4k/bg-campus-digital-twin-4k.png` | generated |
| P0 | `bg-collection-pipeline-4k` | Probe to analytics pipeline background | `prompts/backgrounds-4k/bg-collection-pipeline-4k.prompt.txt` | `backgrounds/4k/bg-collection-pipeline-4k.png` | generated |
| P0 | `bg-threat-situation-map-4k` | Threat situation map and risk heat background | `prompts/backgrounds-4k/bg-threat-situation-map-4k.prompt.txt` | `backgrounds/4k/bg-threat-situation-map-4k.png` | generated |
| P0 | `bg-evidence-forensics-chain-4k` | Evidence chain and forensics background | `prompts/backgrounds-4k/bg-evidence-forensics-chain-4k.prompt.txt` | `backgrounds/4k/bg-evidence-forensics-chain-4k.png` | generated |
| P0 | `bg-entity-graph-canvas-4k` | Entity relationship graph canvas background | `prompts/backgrounds-4k/bg-entity-graph-canvas-4k.prompt.txt` | `backgrounds/4k/bg-entity-graph-canvas-4k.png` | generated |
| P0 | `bg-acceptance-evidence-board-4k` | Acceptance and evidence board background | `prompts/backgrounds-4k/bg-acceptance-evidence-board-4k.prompt.txt` | `backgrounds/4k/bg-acceptance-evidence-board-4k.png` | generated |

## P1

| Priority | ID | Role | Prompt | Target | Status |
| --- | --- | --- | --- | --- | --- |
| P1 | `bg-login-secure-identity-4k` | Secure identity and login background | `prompts/backgrounds-4k/bg-login-secure-identity-4k.prompt.txt` | `backgrounds/4k/bg-login-secure-identity-4k.png` | generated |
| P1 | `bg-dashboard-ops-workbench-4k` | Operations dashboard workbench background | `prompts/backgrounds-4k/bg-dashboard-ops-workbench-4k.prompt.txt` | `backgrounds/4k/bg-dashboard-ops-workbench-4k.png` | generated |
| P1 | `bg-topic-encrypted-tunnel-4k` | Encrypted tunnel topic background | `prompts/backgrounds-4k/bg-topic-encrypted-tunnel-4k.prompt.txt` | `backgrounds/4k/bg-topic-encrypted-tunnel-4k.png` | generated |
| P1 | `bg-topic-data-exfiltration-4k` | Data exfiltration topic background | `prompts/backgrounds-4k/bg-topic-data-exfiltration-4k.prompt.txt` | `backgrounds/4k/bg-topic-data-exfiltration-4k.png` | generated |
| P1 | `bg-topic-apt-campaign-4k` | APT campaign topic background | `prompts/backgrounds-4k/bg-topic-apt-campaign-4k.prompt.txt` | `backgrounds/4k/bg-topic-apt-campaign-4k.png` | generated |
| P1 | `bg-probe-deployment-topology-4k` | Probe deployment and topology background | `prompts/backgrounds-4k/bg-probe-deployment-topology-4k.prompt.txt` | `backgrounds/4k/bg-probe-deployment-topology-4k.png` | generated |
| P1 | `bg-alert-triage-workbench-4k` | Alert triage workbench background | `prompts/backgrounds-4k/bg-alert-triage-workbench-4k.prompt.txt` | `backgrounds/4k/bg-alert-triage-workbench-4k.png` | generated |
| P1 | `bg-campaign-timeline-4k` | Campaign timeline and investigation background | `prompts/backgrounds-4k/bg-campaign-timeline-4k.prompt.txt` | `backgrounds/4k/bg-campaign-timeline-4k.png` | generated |
| P1 | `bg-attack-chain-lanes-4k` | Attack chain lane background | `prompts/backgrounds-4k/bg-attack-chain-lanes-4k.prompt.txt` | `backgrounds/4k/bg-attack-chain-lanes-4k.png` | generated |
| P1 | `bg-encrypted-traffic-fingerprint-4k` | Encrypted traffic fingerprint background | `prompts/backgrounds-4k/bg-encrypted-traffic-fingerprint-4k.prompt.txt` | `backgrounds/4k/bg-encrypted-traffic-fingerprint-4k.png` | generated |
| P1 | `bg-asset-inventory-context-4k` | Asset inventory and business context background | `prompts/backgrounds-4k/bg-asset-inventory-context-4k.prompt.txt` | `backgrounds/4k/bg-asset-inventory-context-4k.png` | generated |
| P1 | `bg-fusion-multisource-4k` | Multi-source fusion background | `prompts/backgrounds-4k/bg-fusion-multisource-4k.prompt.txt` | `backgrounds/4k/bg-fusion-multisource-4k.png` | generated |
| P1 | `bg-baseline-behavior-4k` | Behavioral baseline background | `prompts/backgrounds-4k/bg-baseline-behavior-4k.prompt.txt` | `backgrounds/4k/bg-baseline-behavior-4k.png` | generated |

## P2

| Priority | ID | Role | Prompt | Target | Status |
| --- | --- | --- | --- | --- | --- |
| P2 | `bg-rule-lifecycle-4k` | Rule lifecycle and publish governance background | `prompts/backgrounds-4k/bg-rule-lifecycle-4k.prompt.txt` | `backgrounds/4k/bg-rule-lifecycle-4k.png` | generated |
| P2 | `bg-deployment-rollout-4k` | Deployment rollout and rollback background | `prompts/backgrounds-4k/bg-deployment-rollout-4k.prompt.txt` | `backgrounds/4k/bg-deployment-rollout-4k.png` | generated |
| P2 | `bg-model-governance-4k` | Model governance and registry background | `prompts/backgrounds-4k/bg-model-governance-4k.prompt.txt` | `backgrounds/4k/bg-model-governance-4k.png` | generated |
| P2 | `bg-mlops-dag-4k` | MLOps workflow DAG background | `prompts/backgrounds-4k/bg-mlops-dag-4k.prompt.txt` | `backgrounds/4k/bg-mlops-dag-4k.png` | generated |
| P2 | `bg-playbook-flow-4k` | Playbook automation flow background | `prompts/backgrounds-4k/bg-playbook-flow-4k.prompt.txt` | `backgrounds/4k/bg-playbook-flow-4k.png` | generated |
| P2 | `bg-whitelist-governance-4k` | Whitelist and approval governance background | `prompts/backgrounds-4k/bg-whitelist-governance-4k.prompt.txt` | `backgrounds/4k/bg-whitelist-governance-4k.png` | generated |
| P2 | `bg-audit-trail-4k` | Audit trail and compliance trace background | `prompts/backgrounds-4k/bg-audit-trail-4k.prompt.txt` | `backgrounds/4k/bg-audit-trail-4k.png` | generated |
| P2 | `bg-notification-routing-4k` | Notification routing and escalation background | `prompts/backgrounds-4k/bg-notification-routing-4k.prompt.txt` | `backgrounds/4k/bg-notification-routing-4k.png` | generated |
| P2 | `bg-settings-security-4k` | Settings and security administration background | `prompts/backgrounds-4k/bg-settings-security-4k.prompt.txt` | `backgrounds/4k/bg-settings-security-4k.png` | generated |
| P2 | `bg-not-found-safe-error-4k` | Safe error and not-found background | `prompts/backgrounds-4k/bg-not-found-safe-error-4k.prompt.txt` | `backgrounds/4k/bg-not-found-safe-error-4k.png` | generated |

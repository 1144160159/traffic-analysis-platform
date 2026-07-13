# UI Visual Interaction Evidence Finalization

- Run ID: `202607030-ui-visual-evidence-finalize-current-gap-r1`
- Result: `blocked`
- Visual evidence passed: `0/30`
- Interaction evidence passed: `0/28`
- Metrics generated: `4`
- Evidence dir: `doc/02_acceptance/02-regression/ui-visual-interaction/latest`

This finalizer only evaluates existing Desktop Chrome evidence. It does not capture screenshots and does not replace the dual gate preflight.

## Visual Blockers

- `login`: actual screenshot size (2559, 1271) != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080; source/actual size mismatch 1920x1080 vs 2559x1271; pixel mismatch ratio 0.9999099151449858 > 0.015
- `screen`: actual screenshot size (2559, 1271) != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080; source/actual size mismatch 1920x1080 vs 2559x1271; pixel mismatch ratio 0.9999508069051117 > 0.015
- `dashboard`: actual screenshot size (2559, 1271) != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080; source/actual size mismatch 1920x1080 vs 2559x1271; pixel mismatch ratio 0.9999188313934344 > 0.015
- `alerts`: actual screenshot size (2559, 1271) != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080; source/actual size mismatch 1920x1080 vs 2559x1271; pixel mismatch ratio 0.9999425055703494 > 0.015
- `alert-detail`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/alert-detail/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/alert-detail/capture-meta.json
- `campaigns`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaigns/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaigns/capture-meta.json
- `campaign-detail`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaign-detail/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaign-detail/capture-meta.json
- `attack-chains`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/attack-chains/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/attack-chains/capture-meta.json
- `encrypted-traffic`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/encrypted-traffic/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/encrypted-traffic/capture-meta.json
- `forensics`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/forensics/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/forensics/capture-meta.json
- `assets`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/assets/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/assets/capture-meta.json
- `graph`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/graph/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/graph/capture-meta.json
- `fusion`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/fusion/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/fusion/capture-meta.json
- `baselines`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/baselines/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/baselines/capture-meta.json
- `probes`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/probes/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/probes/capture-meta.json
- `rules`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/rules/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/rules/capture-meta.json
- `deployments`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/deployments/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/deployments/capture-meta.json
- `models`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/models/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/models/capture-meta.json
- `mlops`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/mlops/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/mlops/capture-meta.json
- `data-quality`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/data-quality/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/data-quality/capture-meta.json
- `playbooks`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/playbooks/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/playbooks/capture-meta.json
- `whitelist`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/whitelist/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/whitelist/capture-meta.json
- `compliance`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/compliance/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/compliance/capture-meta.json
- `audit-log`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/audit-log/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/audit-log/capture-meta.json
- `notifications`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/notifications/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/notifications/capture-meta.json
- `settings`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/settings/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/settings/capture-meta.json
- `not-found`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/not-found/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/not-found/capture-meta.json
- `topics-encrypted-tunnel`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-encrypted-tunnel/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-encrypted-tunnel/capture-meta.json
- `topics-data-exfiltration`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-data-exfiltration/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-data-exfiltration/capture-meta.json
- `topics-apt-campaign`: actual screenshot missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-apt-campaign/actual-1920.png; capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-apt-campaign/capture-meta.json

## Interaction Blockers

- `login`: interaction screenshot size (2559, 1271) != 1920x1080; interaction-capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/login/interaction-capture-meta.json
- `screen`: interaction screenshot size (2559, 1271) != 1920x1080; interaction-capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/screen/interaction-capture-meta.json
- `dashboard`: interaction screenshot size (2559, 1271) != 1920x1080; interaction-capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/interaction-capture-meta.json
- `alerts`: interaction screenshot size (2559, 1271) != 1920x1080; interaction-capture-meta missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/alerts/interaction-capture-meta.json
- `alert-detail`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/alert-detail/interaction.json
- `campaigns`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaigns/interaction.json
- `campaign-detail`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaign-detail/interaction.json
- `attack-chains`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/attack-chains/interaction.json
- `encrypted-traffic`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/encrypted-traffic/interaction.json
- `forensics`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/forensics/interaction.json
- `assets`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/assets/interaction.json
- `graph`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/graph/interaction.json
- `fusion`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/fusion/interaction.json
- `baselines`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/baselines/interaction.json
- `probes`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/probes/interaction.json
- `rules`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/rules/interaction.json
- `deployments`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/deployments/interaction.json
- `models`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/models/interaction.json
- `mlops`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/mlops/interaction.json
- `data-quality`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/data-quality/interaction.json
- `playbooks`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/playbooks/interaction.json
- `whitelist`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/whitelist/interaction.json
- `compliance`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/compliance/interaction.json
- `audit-log`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/audit-log/interaction.json
- `notifications`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/notifications/interaction.json
- `settings`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/settings/interaction.json
- `not-found`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/not-found/interaction.json
- `topics`: interaction missing: doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics/interaction.json

## Formal Rerun

```bash
ALLOW_BLOCKERS=false tests/e2e/ui_visual_interaction_evidence_finalize.py
DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh
ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh
```

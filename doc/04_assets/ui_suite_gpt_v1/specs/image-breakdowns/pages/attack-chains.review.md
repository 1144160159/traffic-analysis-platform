# attack-chains.png review

## Review Status

- Status: `business-pixel-accepted`
- Strict pixel status: `fail-documented`
- Production Web image: `docker.io/traffic/web-ui:attack-chain-r785`
- Production API image: `docker.io/traffic/alert-service:attack-chain-r780`
- Production route: `/attack-chains`
- Browser evidence: Windows Chrome 150 through Xshell CDP `127.0.0.1:9224 -> 127.0.0.1:9222`
- Desktop screenshot: `evidence/ui-image-breakdowns/pages/attack-chains/implementation-r785-final.png`
- Responsive screenshot: `evidence/ui-image-breakdowns/pages/attack-chains/implementation-r785-responsive-1366.png`
- Combined comparison: `evidence/ui-image-breakdowns/pages/attack-chains/comparison-r785-final.png`
- Diff image: `evidence/ui-image-breakdowns/pages/attack-chains/diff-r785-final.png`

## Database-backed Demo State

- Fixed campaign ID: `attack-chain-demo-c2-20260726`.
- ClickHouse fixtures:
  - `common/sql/ch/fixtures/attack-chain-demo-campaign.jsonl`
  - `common/sql/ch/fixtures/attack-chain-demo-alerts.jsonl`
  - `common/sql/ch/fixtures/attack-chain-demo-evidence.jsonl`
- Idempotent seed script: `scripts/seed_attack_chain_demo.sh`.
- Live ClickHouse verification: one campaign, six ATT&CK phases, six campaign alert IDs, six alert rows, six evidence rows, score `0.92`.
- The seed script deletes and recreates only the fixed demo ID on both shards. It does not clear or rewrite unrelated campaign data.

## Checks

| Check | Result | Evidence |
|---|---|---|
| Live ClickHouse data | pass | `campaigns=1`, `phases=6`, `alerts=6`, `evidence=6`, score `0.92` |
| API mapping | pass | `handler_system.go` maps phase, entity label, endpoints, evidence IDs and MITRE technique |
| API unit test | pass | `campaign_detail_enrichment_test.go` |
| Production React implementation | pass | `AttackChainAnalysisPage.tsx`; no target raster is loaded |
| Production API path | pass | `/api/v1/attack-chains` and `/api/v1/attack-chains/{id}` |
| Initial direct-route selection | pass | the first available live chain is selected and its detail is requested |
| Attack canvas | pass | six API-driven phase columns, entity nodes, alert events, evidence anchors and response actions |
| Evidence anchors | pass | six database-backed evidence rows; type tabs filter the visible rows |
| Response recommendations | pass | six phase-linked recommendations; phase selection filters the rail |
| Bottom analysis | pass | six ATT&CK matrix items and five key-jump path rows |
| Desktop interaction gate | pass | phase select/clear, zoom `1 -> 1.08`, PCAP filter `3` rows |
| Responsive interaction gate | pass | `1366x768` keeps the rail below the canvas; the same interactions pass |
| Runtime gate | pass | no application 4xx/5xx, failed request, console error or page error |
| Overflow gate | pass | no hidden persistent control or horizontal business-area overflow |
| Production deployment | pass | Web r785 and API r780 Deployments are Ready |
| Business visual gate | pass | mismatch ratio `0.1016772762345679 <= 0.15`, channel tolerance `64` |
| Strict visual gate | fail-documented | live timestamps, database content, font glyph rasterization and exact table density are not pixel-identical |

## Visual Region Results

| Region | Mismatch ratio |
|---|---:|
| Full viewport | `0.1016772762345679` |
| Business region | `0.09952911862832045` |
| Chain workspace | `0.09686327862454741` |
| Right rail | `0.10001898354171082` |
| Bottom detail | `0.12990839073525903` |

The final side-by-side image compares the canonical target and the production
Windows Chrome capture at the same `1920x1080`, DPR 1 state. No actionable
P0/P1/P2 layout or interaction mismatch remains. Exact glyph rasterization,
current timestamps and live database values remain documented as non-blocking
P3 differences.

## Acceptance Evidence

- Desktop interaction report:
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-attack-chains-r785-final.json`
- Responsive interaction report:
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-attack-chains-r785-responsive-1366.json`
- Visual metrics:
  `evidence/ui-image-breakdowns/pages/attack-chains/metrics-r785-final.json`

## Record Completeness Review

- The page remains inside the common AppShell and preserves the existing threat-analysis menu.
- The six stages are sourced from ClickHouse campaign and alert data, rather than a frontend-only mock snapshot.
- Entity, source/destination endpoint, evidence ID and MITRE technique provenance are retained in the API response.
- Phase selection drives both evidence and response-rail state.
- Zoom, auto-layout, view mode, evidence type filter and detail entry are interactive.
- The compact layout changes flow direction instead of letting the right rail cover the attack canvas.

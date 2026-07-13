# campaign-detail-impact-department.png review

## Review Status

- Status: `business-pixel-accepted`
- Strict pixel status: `fail`
- Production image: `traffic/web-ui:ui-campaign-impact-department-20260710-r177`
- Production route: `/campaigns/:campaignId?impact=department`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final focus screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/implementation-r177-final.png`
- Normal route screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/normal-route-r177.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Target and route state | pass | `impact=department`; focus state contains department summary and Top 5 |
| Production React implementation | pass | `CampaignDetailPage.tsx`; no target raster is loaded |
| Typed dynamic data | pass | `CampaignDetailImpactDepartment` from `/v1/campaigns/{campaignId}` with typed fallback |
| Dynamic risk ring and table | pass | risk counts drive CSS angles; five rows render from `snapshot.impactDepartment` |
| Dynamic progress bars | pass | `DepartmentImpactTable` writes `--taf-impact-progress` from row progress |
| Local gates | pass | 3 test files, 6 tests and production build passed |
| Production deployment | pass | r177 Deployment is `1/1 Ready`; APISIX `/campaigns` is 200 |
| Windows Chrome runtime | pass | no 4xx/5xx, failed request, console/page error, forbidden raster request or scroll |
| Normal business geometry | pass | department tab, five rows, five progress values and downlink are inside the panel body |
| Business visual gate | pass | `0.07469184027777778 <= 0.35`, channel tolerance `64` |
| Strict visual gate | fail | `0.9997222222222222 > 0.015`, channel tolerance `0` |
| Auxiliary review | pass | r177 review allows business acceptance only |
| Main-thread judgment | pass | accepted only as `business-pixel-accepted`; strict failure remains explicit |

## Repair History

- Before r177, `department` was parsed as a tab state but fell back to generic asset table content.
- r177 added `CampaignDetailImpactDepartment`, `CampaignDetailDepartmentRow`, `buildImpactDepartment`, and department-specific focus/normal rendering.
- r177 added a DOM/CSS progress bar driven by typed row progress and tests preventing target-image substitution.
- The first normal-route runtime check used a too-narrow active-tab selector; visual evidence showed the tab active. The runtime checker was corrected to read the active tab's full text and passed.

## Evidence Interpretation

- The focus screenshot is a real APISIX/Web UI React route using a deterministic typed snapshot for repeatable visual evidence.
- The normal route uses the live `/v1/campaigns/{campaignId}` path with typed fallback when department fields are absent.
- The PNG is 1920x1080. Windows reports a 2133x1200 CSS viewport at DPR 0.9; this scaling is recorded in capture metadata.
- `business-pixel-accepted` is not `pixel-perfect`. The strict `0.015 / 0` result remains failed and unresolved.
- The historical `implementation.html` in the evidence directory is not current acceptance evidence and is not loaded by production code.

## Auxiliary Review

- Agent: `019f4ace-10da-71c3-8149-f0ba1470bf0d`
- Verdict: PASS
- Dynamic React/CSS/typed implementation: pass
- Windows Chrome CDP, APISIX and runtime: pass
- Normal route AppShell and geometry: pass
- Business tolerance gate: pass
- Strict pixel gate: fail, correctly documented
- Token/JWT leakage scan: pass
- Conclusion: allow `business-pixel-accepted` only; do not claim strict pixel acceptance

## Evidence

- `target.png`, `regions-overlay.png`, `measurement.json`
- `implementation-r177-final.png`, `implementation.png`
- `diff-business-r177-final.png`, `metrics-business-r177-final.json`
- `diff-strict-r177-final.png`, `metrics-strict-r177-final.json`
- `capture-meta-r177-final.json`, `production-route-report-r177-final.json`
- `normal-route-r177.png`, `normal-route-runtime-r177.json`
- `cdp-version-r177-final.json`, `cdp-list-r177-final.json`
- `verification.json`

## Decision

The auxiliary review and main-thread recheck both pass for the business-tolerance gate. `campaign-detail-impact-department` is accepted as `business-pixel-accepted`; strict pixel matching remains failed and must not be represented otherwise.

## Breakdown Depth Review

- Record gate: `breakdown-accepted`.
- Depth: 16 regions, 52 structured texts, 12 components, 10 icons, 18 tokens, 8 interactions.
- Target was read directly; department counts, owners, risks and progress values match the image ledger.
- Mapping, evidence, differences and conclusion are explicit; strict-pixel failure remains documented.

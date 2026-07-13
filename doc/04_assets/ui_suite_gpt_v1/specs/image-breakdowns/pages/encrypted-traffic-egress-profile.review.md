# encrypted-traffic-egress-profile.png review

## Review Status

- Status: `main-thread-accepted-r189`
- Scope: production React route `/encrypted-traffic?tab=egress-profile`
- Browser evidence: Windows Chrome CDP through `http://127.0.0.1:9224`
- Target: `doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-egress-profile.png`
- Current evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-egress-profile/implementation.png`

## Main-Thread Checks

| Check | Result | Evidence |
|---|---|---|
| Production route and deployment | pass | `traffic/web-ui:ui-encrypted-traffic-egress-profile-20260710-r189` is ready; alert API remains `encrypted-egress-data-20260710-r188` |
| ECharts map | pass | `WorldActivityMap` custom/scatter/lines; Windows Chrome reports five canvases |
| ECharts trend | pass | `ExfilStackedTrendChart` receives the five structured time-bucket series |
| ECharts entity relation | pass | `ExfilGraphChart` node-link series is rendered and selected with the destination |
| API-first data | pass | `/v1/encrypted-traffic/exfiltration` returns `top_destinations`, `top_sources`, `paths`, and `trend`; the adapter never maps a source IP as a destination |
| Empty API fallback | pass | current API arrays are empty; page visibly marks the typed fallback as `仿真数据（API 空）` |
| Cross-panel selection | pass | `normal-route-r189-echarts-interactions-runtime.json` records `104.16.12.34` in map and graph aria states |
| Audited write action | pass | Windows Chrome POST returned 200 and filtered `/v1/audit/logs` returned the matching `ENCRYPTED_EGRESS_AUDIT_WRITTEN` row |
| Browser runtime | pass | no 4xx/5xx, request failure, console/page error, or horizontal overflow |
| Business visual diff | pass | mismatch ratio `0.13109230324074075` <= `0.35` at 1920 x 1080, tolerance 64 |
| Tests and build | pass | focused UI suite 48/48; `npm run build`; `go test ./internal/alert/api` |

## Evidence Details

- The screenshot is a Windows Chrome capture of the APISIX route, not an `implementation.html` replay and not a copied target bitmap.
- `WorldActivityMap` renders custom land geometry, scatter nodes and ECharts line effects in a single chart option.
- `ExfilStackedTrendChart` consumes backend-shaped category series; it does not derive categories from fixed proportions.
- `ExfilGraphChart` renders the selected destination as a graph node and shares selection state with the map and destination table.
- The audit success toast is shown only after the POST resolves with `recorded`; the filtered readback confirms that the database write occurred.
- The test fixture verifies a nonempty API response maps destination `203.0.113.45` and raw trend values without a source-IP fallback.
- The current live payload intentionally proves the empty branch, including its API schema; it does not substitute a mock network request.
- `r189` changes the frontend deployment image after the adapter regression cleanup. The alert-service API image is unchanged from the reviewed r188 backend.

## Review Resolution

- The independent r188 review confirmed the source/destination, raw trend and persistent write-action fixes. It requested refreshed formal evidence and correction of stale adapter assertions.
- r189 refreshes the Windows Chrome capture, `verification.json`, page record, acceptance bundle and all affected adapter assertions. No egress implementation behavior changed after that review; the frontend image bump carries the current workspace build.
- A nonempty live external-destination feed is not presently available. This remains documented rather than fabricated; the page shows explicit simulated mode and the live-field mapping is covered by tests.

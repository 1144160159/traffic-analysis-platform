# Campaign r298/r210 Targeted Verification

- Captured: `2026-07-12T13:22:00+08:00`
- Route: `http://10.0.5.8:30180/campaigns`
- Browser: Windows Chrome CDP, `Chrome/150.0.7871.49`

## Passed

- Alert API: `go test -count=1 ./internal/alert/...` passed.
- Web UI: 3 files and 50 tests passed.
- Web UI production build passed; only the existing Vite chunk-size warning remains.
- `node --check tests/e2e/ui_campaign_interactions.mjs` passed.
- Windows Chrome interaction report passed against Web UI r298 and Alert r210.
- Two ECharts canvases were visible and nonblank with 126 painted samples each.
- Pagination loaded distinct offset 0 and offset 8 records.
- The 100-row request no longer gets truncated to 8 rows by the adapter.
- All 16 action response job IDs matched 16 persisted server audit job IDs.
- Viewer write access was denied with HTTP 403.
- Business ROI was `0.07928317482271255`; production-data ROI was `0.07741815773041669`. Both are below `0.125`.
- Web UI r298 and Alert r210 rolled out Ready `1/1`.

## Corrected Semantics

- Global campaign total remains an API total.
- Loaded-page counts are labeled `current page API` and no longer imply global aggregation.
- Risk percentages use loaded rows as their denominator.
- Evidence completeness is `API field unavailable`; the UI no longer invents a percentage.

## Hard Failures

- Alert's Kafka consumer continuously returns EOF while the broker reports SSL handshake failures.
- The 30-minute log window contains 3 Campaign read 5xx responses in 83 Campaign requests.
- Rollback has not been executed and verified.
- Campaign filters still operate over the first 100 loaded records rather than server-side filters over the full result set.
- Campaign action endpoints record an audited dry run; they do not yet create a durable report, SOAR execution, or state transition job.

This evidence permits a repair decision only. It is not a positive learning episode or production-stability sign-off.

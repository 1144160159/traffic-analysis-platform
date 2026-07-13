# Encrypted Traffic Evidence Center r207

Windows Chrome CDP production-route evidence for `/encrypted-traffic?tab=evidence-center` after the API-semantic correction and independent visual review.

- `actual-1920.png`, `target-1920.png`, `diff-business.png`, and `metrics-business.json` record the valid 1920 x 1080 capture: ratio `0.10848717206790123`, configured business threshold `0.35`, tolerance `64`.
- `stable-runtime.json` records two repeat captures with the same ratio, fixed five-tab geometry, three ECharts canvases, and Session-to-anchor-to-right-rail selection synchronization.
- `api-contract-runtime.json` proves that the live API reports `entropy_available=false`, reserves an empty `entropy_trend`, and returns `anomaly_trend` separately rather than presenting anomaly scores as payload entropy.
- `verification.json` records the independent review and all remaining strict-pixel differences.

This package proves production semantics and business interaction only. It does not grant strict pixel acceptance because the ratio remains above `0.015`. Evidence preservation continues to represent an audited request, not a completed external forensic task.

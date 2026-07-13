# Capture Performance Preflight

- Run ID: `20260701-capture-performance-preflight-r4-review-packet`
- Result: `blocked`
- Plan dir: `tests/perf/100g_capture`

## Summary

- Checks: 11/18 passed, blockers=4, warnings=3

## Failed Checks

- [blocker] hardware inventory provided: tests/perf/100g_capture/hardware-inventory.yaml
- [blocker] traffic profile provided: tests/perf/100g_capture/traffic-profile.yaml
- [warn] existing 500k stress report is only non-acceptance context: 0.94Mpps/1.3Gbps class report, not GATE-P0-03/04
- [warn] live probe capture mode is AF_XDP: mode=af_packet
- [warn] live probe CPU pinning has multi-queue capacity: cpu_cores=2
- [blocker] 10x100g-line-rate result summary present: tests/perf/100g_capture/results/10x100g-summary.json
- [blocker] 512mpps-small-packet result summary present: tests/perf/100g_capture/results/512mpps-summary.json

## Boundary

This preflight does not execute TRex/pktgen or destructive line-rate traffic. GATE-P0-03/04 require signed hardware-window result summaries.

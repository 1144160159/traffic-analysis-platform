# Capture Performance Bootstrap Runbook

Run ID: `20260630-capture-performance-bootstrap-r1`

This directory is a review-required draft for GATE-P0-03 and GATE-P0-04. It is not a formal acceptance package.

## Use This Draft

1. Review `hardware-inventory.bootstrap.yaml` with the lab operator and fill real NIC, firmware, NUMA, CPU pinning, generator, cable, and switch path details.
2. Review `traffic-profile.bootstrap.yaml` and lock generator ports, packet mix, flow cardinality, duration, and thresholds.
3. Run the approved hardware-window test on isolated 10 x 100Gbps / 512Mpps-capable equipment.
4. Replace the review templates with real completed summaries named exactly `results/10x100g-summary.json` and `results/512mpps-summary.json` under `tests/perf/100g_capture`.
5. Rerun `ALLOW_BLOCKERS=true tests/perf/100g_capture/live_capture_performance_preflight.sh` and keep the result blocked unless both formal summaries meet every gate.

## Boundary

The current live context can show that probe-agent is deployed and what the small cluster profile looks like. It cannot prove line-rate capture, 512Mpps small-packet handling, packet-loss thresholds, generator telemetry, or signed operator acceptance.

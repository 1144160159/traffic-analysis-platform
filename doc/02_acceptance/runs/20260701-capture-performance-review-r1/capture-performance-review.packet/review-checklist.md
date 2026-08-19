# Capture Performance Review Checklist

1. Fill `hardware-review.csv` with real generator, NIC, firmware, queue, NUMA, switch, and cable information.
2. Fill `traffic-profile-review.csv` with the approved generator packet mix, flow cardinality, duration, and thresholds.
3. Create formal `tests/perf/100g_capture/hardware-inventory.yaml` and `traffic-profile.yaml` only after operator approval.
4. Execute the isolated 10 x 100Gbps and 512Mpps hardware-window tests.
5. Replace review templates with real `results/10x100g-summary.json` and `results/512mpps-summary.json` including raw log, switch telemetry, Prometheus snapshot, signed report, and SHA256 manifest URIs.
6. Rerun `ALLOW_BLOCKERS=false tests/perf/100g_capture/live_capture_performance_preflight.sh`.

Do not rename bootstrap or review-template files into formal artifact paths.

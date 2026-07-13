# 100G Capture Performance Acceptance Package

This package is the execution and evidence contract for GATE-P0-03 and GATE-P0-04.
It does not replace a real hardware-window test. The preflight only verifies the
repo contract, current live readiness signals, and any real result files produced
by a TRex/pktgen/line-rate lab run.

## What Must Exist For Acceptance

Required operator-provided files:

| File | Purpose |
|---|---|
| `hardware-inventory.yaml` | NIC model/firmware, link speed, NUMA/CPU pinning, cable/switch path, generator host mapping. |
| `traffic-profile.yaml` | Traffic mix, packet-size distribution, 10 x 100Gbps and 512Mpps scenarios, duration, loss thresholds. |
| `results/10x100g-summary.json` | Real multi-port 10 x 100Gbps execution summary. |
| `results/512mpps-summary.json` | Real small-packet 512Mpps execution summary. |

Templates live next to this README. Large PCAPs, TRex raw logs, and switch telemetry should stay in immutable storage; commit only summary JSON, hashes, and signed reports.

## Preflight

Run from the repository root:

```bash
ALLOW_BLOCKERS=true RUN_ID=20260629-capture-performance-preflight-r1 tests/perf/100g_capture/live_capture_performance_preflight.sh
```

The preflight writes run evidence under `doc/02_acceptance/runs/<run-id>/` and stable copies under `doc/02_acceptance/03-performance/`.

Expected current state is `blocked` until the real hardware inventory, traffic profile, and result summaries are provided.

## Bootstrap Draft

To prepare a lab review package without closing the formal gate, run:

```bash
RUN_ID=20260630-capture-performance-bootstrap-r1 tests/perf/100g_capture/live_capture_performance_package_bootstrap.sh
```

The bootstrap writes only `*.bootstrap.*` and `*.review-template.*` files under `doc/02_acceptance/03-performance/bootstrap/`. It must not be renamed into the formal acceptance files until the hardware-window run is complete and signed.

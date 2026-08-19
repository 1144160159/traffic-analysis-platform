# NetworkPolicy Enforcement Readiness

- Run ID: `20260630-network-policy-enforcement-readiness-r1`
- Result: `pass`
- Latest formal preflight: `20260630-network-policy-enforcement-preflight-r1-flannel-blocked` / `blocked`
- Policy-capable CNI pods: `0`
- Flannel markers: `2`
- Live NetworkPolicy objects: `20`
- Bootstrap dir: `doc/02_acceptance/runs/20260630-network-policy-enforcement-readiness-r1/network-policy-enforcement-readiness.bootstrap`
- Stable bootstrap dir: `doc/02_acceptance/05-security/network-policy-readiness/latest`

This package prepares the CNI migration and enforcement proof workflow. It does not install a CNI, does not run destructive network changes, and does not satisfy GATE-P0-07 or GATE-P0-10.

## Required Formal Closure

After a policy-capable CNI is installed and Ready, rerun:

```bash
ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh
```

The formal gate only passes when baseline connectivity works, default-deny blocks the isolated probe, and allow-list restores the probe.

## Non-Passing Checks

- [warn] policy-capable CNI is already present: policy_capable_count=0 flannel_markers=2; formal negative probe remains invalid
- [warn] formal NetworkPolicy enforcement gate status: expected until policy-capable CNI and probe proof exist

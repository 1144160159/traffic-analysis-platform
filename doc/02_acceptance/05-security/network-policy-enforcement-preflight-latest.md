# NetworkPolicy Enforcement Preflight

- Run: `20260630-network-policy-enforcement-preflight-r1-flannel-blocked`
- Result: `blocked`
- Checks: `2/4` passed, `2` blockers, `0` warnings
- Policy-capable CNI pods: `0`
- Flannel markers: `2`
- Live NetworkPolicy objects: `20`
- Enforcement probe namespace: `np-enforce-20260630-network-policy-enforcement-pref`

This gate proves whether NetworkPolicy enforcement is available before GATE-P0-07
or GATE-P0-10 can be claimed. When a policy-capable CNI is present, the probe
uses an isolated namespace to prove three facts: baseline service connectivity,
default-deny ingress blocking, and allow-list ingress restoration.

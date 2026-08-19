# CNI Migration Runbook Template

## Boundary

This is a review-required runbook for closing NetworkPolicy enforcement. It does not install a CNI and does not prove enforcement by itself.

## Candidate Selection

- Candidate CNI:
- Selected version:
- Reason for selection:
- Operator:
- Maintenance window:
- Rollback owner:

## Pre-Change Capture

- `kubectl get nodes -o wide`
- `kubectl get pods -A -o wide`
- `kubectl get networkpolicy -A`
- `kubectl get daemonset -A | grep -Ei 'flannel|calico|cilium|antrea|kube-router|ovn|weave|canal'`
- Latest `doc/02_acceptance/05-security/network-policy-enforcement-preflight-latest.json`

## Change Steps

1. Freeze workload rollout changes.
2. Back up current CNI manifests and kube-system/kube-flannel DaemonSet state.
3. Install or migrate to the approved policy-capable CNI.
4. Wait for every CNI DaemonSet to be Ready on every schedulable node.
5. Confirm kube-dns, APISIX, control-plane services, Kafka, ClickHouse, PostgreSQL, Redis, MinIO and probe-agent paths remain healthy.
6. Rerun `ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh`.

## Required Exit Evidence

- Policy-capable CNI pod count is greater than 0.
- Live NetworkPolicy object count is greater than 0.
- Isolated probe proves baseline connectivity, default-deny blocking, and allow-list restoration.
- Production security preflight no longer reports the CNI enforcement blocker.

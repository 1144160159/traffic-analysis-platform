# T1-M10-N009 minimum network policy

This runbook separates a rendered NetworkPolicy candidate from verified
enforcement. The checked-in candidate is default-off and must not be applied to
the current Flannel-only cluster.

## Static candidate gate

```bash
python3 scripts/alignment/build_m10_minimum_network_policy.py --check
python3 scripts/alignment/verify_m10_minimum_network_policy.py
python3 -m unittest tests.alignment.test_m10_minimum_network_policy -v
kubectl apply --dry-run=client \
  -f deployments/kubernetes/security/m10-minimum-network-policies.v1.yaml
```

## Read-only cluster evidence

```bash
python3 scripts/alignment/run_m10_network_policy_k8s.py
python3 scripts/alignment/verify_m10_network_policy_k8s.py
```

The runner only performs `get`, discovery and client-side dry-run operations.
When it observes no enforcement-capable CNI it creates no objects and records
all three required negative probes as not executed. Existing NetworkPolicy API
objects do not change that result.

## Authorized future enforcement test

After an approved enforcement-capable CNI or an approved equivalent control is
installed, use a separately approved run-scoped namespace and immutable probe
images to test all of the following before any production apply:

1. an unauthorized Pod cannot reach an allowed service port;
2. an otherwise authorized Pod cannot reach a non-allowlisted port;
3. an application Pod cannot reach an undeclared external address;
4. every declared service, broker, storage and DNS path still works;
5. removal restores the pre-run state and leaves no probe resources.

Do not mark N009 complete until the real failures, rollback, CNI identity,
candidate digest and exception approvals are all recorded. Full CNI hardening
and management-plane isolation remain in M13-E.

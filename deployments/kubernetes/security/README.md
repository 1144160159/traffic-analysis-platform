# Production Security Profile

This directory contains production-hardening manifests that must be reviewed and
dry-run before they are applied to a live environment.

The current profile is intentionally a starter policy, not a blind apply target.
It models the required direction for GATE-P0-07 and GATE-P0-10:

- default-deny ingress and egress on application and data namespaces;
- public ingress only to the APISIX business HTTP port;
- no public APISIX admin port;
- explicit traffic-analysis to middleware/database/object-store paths;
- DNS egress kept explicit;
- ExternalSecret templates for the namespaces that currently need
  `traffic-credentials`;
- live Kafka TLS/SASL/ACL and External Secrets Operator rollout are tracked by
  acceptance evidence; this profile still does not prove NetworkPolicy
  enforcement until the cluster CNI supports it.

Validate before applying:

```bash
kubectl apply --dry-run=client -f deployments/kubernetes/security/00-network-policies.yaml
ALLOW_BLOCKERS=true tests/e2e/live_network_policy_enforcement_preflight.sh
ALLOW_BLOCKERS=true tests/e2e/live_production_security_preflight.sh
```

Real production rollout still needs a maintenance window, a NetworkPolicy
enforcement-capable CNI instead of the current Flannel-only profile, negative
connectivity tests, the target site SecretStore or rotation flow when required,
and admission/signature policy evidence. Kafka TLS/SASL/ACL, production
ExternalSecret reconciliation, and live workload digest pins now have separate
passing evidence under `doc/02_acceptance/05-security/`.

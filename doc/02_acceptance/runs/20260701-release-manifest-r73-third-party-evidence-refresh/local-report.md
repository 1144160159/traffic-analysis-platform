# Release Manifest Regression Evidence

Run: `20260701-release-manifest-r73-third-party-evidence-refresh`

This run freezes the current repo and live Kubernetes baseline as regression evidence. It does not claim performance, detection-quality, production-security, or HA acceptance.

## Evidence

| Check | Result | Artifact |
|---|---:|---|
| Git commit/status captured | pass | `commit-sha.txt`, `git-status.txt`, `git-diff-stat.txt` |
| Source hashes captured | pass | `file-hashes.json` |
| K8s core manifests dry-run | pass | `k8s-dry-run.txt`, `k8s-dry-run.err` |
| Live workload images and pod image IDs | pass | `k8s-workloads-summary.json`, `k8s-pod-images-summary.json` |
| Repo/live Kafka topic catalog | pass | `kafka-topics-repo.json`, `kafka-topics-live.json` |
| Model/rule/deployment API catalog | see summary | `models-summary.json`, `rules-summary.json`, `deployments-summary.json` |
| Release manifest | pass | `release-manifest.json` |
| Baseline stable copy | pass | `doc/02_acceptance/00-baseline/release-manifest-latest.json` |

## Result

Summary: `doc/02_acceptance/runs/20260701-release-manifest-r73-third-party-evidence-refresh/live-release-manifest-20260701-release-manifest-r73-third-party-evidence-refresh-summary.json`

Checks: 12/12 passed, 0 failed.

## Follow-up Gates

The manifest keeps these gates open: Kafka TLS/SASL/ACL, ExternalSecret, NetworkPolicy, HA/RTO/RPO, 10 x 100Gbps / 512Mpps, and third-party detection-quality evidence.

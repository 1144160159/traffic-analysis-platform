# 07 Deployment Evidence

更新时间：2026-06-30

本目录保存 GATE-P0-09 的部署可复现证据。这里的 preflight 是只读检查，不会 apply 清单、修改 live 资源或读取 Secret 明文。

## 当前结论

`deployment-preflight-latest.json` 来自 `20260630-deployment-preflight-r59-asset-discovery-rbac`，结果为 `pass`：16/16 checks passed，0 blockers，0 warnings。release package 覆盖 136 files，repo image lock missing lines 为 0，live workload digest pin 缺口为 0，非业务外部端口为 0，Pending PVC 为 0，APISIX 业务入口 HTTP 200。

## 证据入口

- `deployment-preflight-latest.json`：部署预检总结果。
- `deployment-preflight-latest.md`：人工阅读版摘要。
- `release-package-manifest-latest.json`：发布包文件哈希清单。
- `site-values-observed-latest.json`：live 集群观测到的 namespace、StorageClass、NodePort、节点和 PVC/PV 状态。
- `secret-reference-readiness-latest.json`：Secret 引用和 key 存在性检查，不包含明文值。
- `non-business-external-ports-latest.json`：非业务外部端口清单。
- `unpinned-or-latest-images-latest.json`：live 未 digest pin 或 latest 镜像清单。
- `repo-image-lock-summary-latest.json`：仓库 K8s 镜像 evidence lock 覆盖摘要。
- `repo-image-lock-inventory-latest.json`：仓库 K8s 镜像引用到 digest/imageID 证据的逐行映射。
- `repo-latest-or-mutable-image-lines-latest.txt`：已被 lock 覆盖但仍为 mutable/latest 的仓库镜像引用。
- `repo-service-exposure-summary-latest.json`：仓库 Service 暴露面摘要，目标为仅 APISIX 业务端口外露。
- `repo-service-exposure-blockers-latest.json`：仓库非业务外部 Service 端口 blocker 清单。
- `apply-convergence-service-exposure-summary-latest.json`：基于当前 live 对象执行 `kubectl apply --dry-run=client -o json` 后的 Service 暴露面收敛摘要。
- `apply-convergence-service-exposure-blockers-latest.json`：apply 收敛态中会被保留的非业务外部 Service 端口 blocker 清单。

## 关闭条件

GATE-P0-09 只能在以下条件都满足后关闭：

- `tests/e2e/live_deployment_preflight.sh` 结果为 `pass`。
- `deployments/kubernetes/site-values.template.yaml` 已转为现场 `site-values.yaml` 或等价制品，并纳入发布包。
- Release package manifest 覆盖 K8s、SQL、Topic、Proto、规则、MLOps workflow、E2E/chaos 测试和前端 route manifest。
- 仓库镜像 evidence lock 缺口为 0，仓库 Service profile 与 apply 收敛态均仅保留 APISIX 业务入口外露，live workload image spec 已滚动为 pullable `image@sha256` 引用，镜像签名、生产安全 profile、NetworkPolicy 和安全负例不再 blocked。
- 当前 live 资源与 release package 的漂移有明确解释或变更记录。

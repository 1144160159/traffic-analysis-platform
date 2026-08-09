# T-SEC-001 服务身份、Secret 与租户权威整改运行手册

## 1. 目标与边界

本控制项为每个工作负载建立可版本化、可核验的身份与敏感配置目录，并将以下风险变成默认失败的门禁：默认或共享 Kubernetes ServiceAccount、非必要的 API token 自动挂载、未批准提权、明文 Secret、共享敏感凭据被隐藏、Kafka 通配或匿名主体，以及从请求 payload、query 或普通 header 取得权威 tenant ID。

候选目录位于 `contracts/security/service-identity-catalog.v1.json`。目录只保存 Secret 名称、key、使用方、轮换元数据和 live resourceVersion；禁止保存 Secret value、token、私钥、完整请求 payload 或认证响应。

本切片只证明仓库身份护栏和只读实况差异可复现，不代表 T-SEC-001 关闭。服务间 mTLS、每服务数据存储角色、Secret 轮换、租户负向用例、灰度、回滚与观察期仍须独立取证。

## 2. 权威来源

- Kubernetes 工作负载：`deployments/kubernetes/applications/go-services.yaml`、`web-ui.yaml`、`probe-agent.yaml` 和 `deployments/kubernetes/flink/flink-log-job.yaml`。
- 候选 ServiceAccount：`deployments/kubernetes/security/go-service-identities.v1.yaml`；探针的 ServiceAccount、RBAC 和特权例外仍由其自身清单管理。
- Kafka 身份和 ACL：`contracts/events/kafka-acl-catalog.v1.json` 及其生成的 ExternalSecret/ACL 计划。
- Secret 供应与轮换元数据：`external-secrets-template.yaml` 和 `generated-kafka-service-identities.v1.yaml`。
- IAM scope：`go/control-plane/internal/auth/model/scopes.go`。
- 网络隔离：`deployments/kubernetes/security/00-network-policies.yaml`。
- Go 运行时用户：`go/control-plane/deployments/docker/Dockerfile.runtime`。

任何权威文件变化后必须重新生成目录；`catalog_sha256` 与每个 authority hash 必须同步变化。不得手改 JSON 以掩盖真实扫描结果。

## 3. 当前候选策略

八个 Go 服务和 web-ui、flink-log-job 使用独立 ServiceAccount；除探针明确例外外均设置 `automountServiceAccountToken: false`。八个 Go 主容器使用 UID/GID 1000、禁止提权、drop ALL capabilities，并采用 `RuntimeDefault` seccomp。探针因抓包和 Kubernetes 节点/Pod 发现需要 token、host network 与特权能力，其例外必须每次发布复核，不得扩展给其他服务。

web-ui 和 flink-log-job 的非 root 运行用户尚未被镜像证据确认，因此保持显式 gap，不能为了使计数归零而盲目设置 UID。未与候选制品完全对应的 mutable image tag 也不得借用 live Pod digest 进行伪 pin。

共享 JWT、PG、ClickHouse、OpenSearch、Nebula、MinIO 凭据以及 tenant header/query fallback 保持 `PARTIAL`，直至完成每服务身份拆分、轮换和负向测试。

## 4. 仓库门禁

运行：

```bash
python3 scripts/alignment/build_service_identity_catalog.py --check
python3 scripts/alignment/verify_service_identity_catalog.py
python3 -m unittest tests.alignment.test_service_identity_catalog -v
kubectl apply --dry-run=client -f deployments/kubernetes/security/go-service-identities.v1.yaml
kubectl apply --dry-run=client -f deployments/kubernetes/applications/go-services.yaml
kubectl apply --dry-run=client -f deployments/kubernetes/applications/web-ui.yaml
kubectl apply --dry-run=client -f deployments/kubernetes/applications/probe-agent.yaml
kubectl apply --dry-run=client -f deployments/kubernetes/flink/flink-log-job.yaml
```

目录完整性 `PASS` 仅说明风险没有被隐藏。`security_compliance=PARTIAL` 时禁止将该控制项写成关闭。

负向测试至少覆盖默认 ServiceAccount、token 自动挂载、Go 容器加权或缺少 hardening、隐藏共享凭据、隐藏 tenant fallback、Kafka 通配主体和删除探针例外说明。

## 5. 只读实况证据

先生成与当前 candidate hash 完全一致的 G0 PASS manifest，再运行：

```bash
make alignment-capture-service-identity-catalog \
  RUN_ID=<immutable-run-id> \
  G0_MANIFEST=<matching-g0-manifest>
```

采集器只读取工作负载、ServiceAccount、Pod imageID 和 Secret metadata resourceVersion；不会读取 Secret data。候选尚未发布时，live ServiceAccount、安全上下文、镜像或缺失 Job 的差异是预期的发布阻断证据，不得改写为 PASS，也不得在证据采集阶段自动 apply。

## 6. Expand、灰度与切换

1. 为新 ServiceAccount、每服务 ExternalSecret、数据存储角色、Kafka ACL、MinIO policy 和证书身份生成不可变候选，先做 server/client dry-run。
2. 在内部 tenant 的单副本 canary 中部署新身份；旧身份保持只读兼容，不得同时扩大新旧写权限。
3. 验证启动、健康、业务读写、审计、Kafka producer/consumer、数据库、OpenSearch、Nebula、MinIO 及证书握手；所有认证失败必须能映射到明确主体和 trace。
4. 执行跨租户、under-scoped、重复请求、过期凭据、撤销证书、Secret 轮换中断和 ACL 拒绝负向用例。
5. 候选稳定后逐 tenant 扩大，观察旧身份使用量归零，再撤销旧权限。严禁先删除旧 Secret 或旧 ACL 后验证。

## 7. 停止扩大条件

出现任一情况立即停止扩大：跨租户访问、匿名或默认身份成功访问、Secret 泄漏、服务不可恢复认证失败、Kafka offset/事务异常、数据写入丢失、证书链或 SAN 错误、共享凭据使用量扩大、审计缺失、P99 或资源预算持续越界。

只读采集失败不触发自动修复；先保存错误类别、candidate hash 和当时资源版本，再定位控制面、RBAC 或资源缺失原因。

## 8. 回滚

回滚使用上一版带 digest 的工作负载、ServiceAccount/role binding、ExternalSecret version、Kafka ACL、数据库角色、对象策略和证书 bundle。恢复旧身份后先验证只读，再恢复写入；在途 Kafka/Flink/异步任务按稳定 event/action ID 对账，禁止以重启成功替代业务一致性证明。

回滚期间保留新身份和 Secret metadata 供审计，但撤销其业务权限；禁止删除证据 run。Secret value 如已疑似泄漏，必须轮换而不是简单恢复旧值。

## 9. 关闭标准

T-SEC-001 只有在以下条件全部满足后才可进入 `VERIFYING/OBSERVING` 并最终关闭：

- 11 个工作负载的 unique identity、token 策略、镜像 digest 和安全上下文与 live 候选完全一致；批准例外只有探针且经过签认。
- JWT 与各数据存储、Kafka、MinIO 的共享高敏凭据拆分为每服务身份，并有两次连续轮换证据。
- 服务间 mTLS、证书 SAN/链/撤销、T-PKI-001 轮换和回滚通过。
- tenant ID 只来自可信认证上下文；payload/query/header 伪造、跨租户与 under-scoped 请求全部拒绝并产生审计。
- 日志、DLQ、对象 manifest 和浏览器证据不包含 Secret、token、私钥或完整敏感 payload。
- G0—G6、T+0/T+1/T+3/T+7 和至少一个低频业务周期通过；独立安全/QA/SRE 签认 G7。

外部门禁未完成时保持 G7 或 G8 的真实状态，不得由仓库测试、HTTP 2xx 或截图代替。

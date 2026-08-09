# T-MINIO-003/004 MinIO 服务身份扩展与切换 Runbook

## 1. 当前边界

本 runbook 对应 `contracts/minio/service-identities.v1.json`。仓库候选已建立七个最小权限服务身份、IAM 兼容策略、ExternalSecret 映射和消费者 Secret 引用，但没有修改 live MinIO、External Secrets 后端或 Kubernetes 工作负载。

`minio-service-identities-v1` Job 默认 `spec.suspend: true`。它只执行 expand，不删除旧用户、不解除旧策略、不轮换 root 凭证。MinIO TLS、CA 分发与客户端证书校验仍是独立后续切片；当前不得仅把 `S3_USE_SSL` 或 `MINIO_SECURE` 改成 `true`。

## 2. 不可跳过的前置门禁

1. 确认候选 manifest、G0 hash、变更窗口、执行人、reviewer、回滚负责人和 canary tenant。
2. 在外部 Secret 后端创建 `traffic-platform-prod-minio-identities` 的十四个属性；access key 和 secret key 必须独立随机生成，不得在终端、工单或证据包输出值。
3. 先应用 `deployments/kubernetes/security/external-secrets-template.yaml` 中的 MinIO scoped ExternalSecret，等待所有目标 Secret `Ready=True`。
4. 只检查 Secret 名称、key 集合、resourceVersion 和 ExternalSecret condition；禁止 base64 解码或打印值。
5. 运行：

   ```bash
   make alignment-verify-minio-service-identities
   python -m unittest tests.alignment.test_minio_service_identities -v
   ```

6. 对 `minio-service-identities-v1` 清单执行 server-side dry-run，并确认 Job 仍为 suspended。

MinIO 管理命令采用官方 `mc admin user add`、`mc admin policy create` 和 `mc admin policy attach --user` 语义。策略 `create` 允许以同名策略覆盖更新，因此相同身份的策略演进必须由合同 diff 和审批控制，而不能依赖手工修改。

## 3. Expand 执行

1. 应用 `deployments/kubernetes/security/minio-service-identities.v1.yaml`，确认 ConfigMap 已创建且 Job 未启动。
2. 从 live MinIO 只读导出现有用户名称、策略名称和绑定关系，生成不含凭证值的 pre-diff。
3. 经双人批准后，仅在窗口内将 Job `spec.suspend` 改为 `false`。
4. 等待 Job 成功终止；检查日志只能包含身份和策略验证结果，不得出现 access key、secret key 或 root 凭证。
5. 再次只读导出用户、策略和绑定关系，逐项与七身份合同对比。任何额外 action、bucket、prefix 或缺失绑定立即停止。
6. 对每个身份执行允许/拒绝双向 canary：

   - probe：允许写 `pcap-archive`，拒绝读报告和访问其他 bucket。
   - alert：允许告警/战役报告读写与受控清理、允许读 PCAP，拒绝写 PCAP。
   - asset：允许资产导出读写、允许读 PCAP，拒绝告警报告和 PCAP 写入。
   - forensics：允许 PCAP 读取、裁剪结果写入和清理，拒绝报告、模型和状态 bucket。
   - MLOps：允许模型前缀读写，拒绝 checkpoint 与 Argo artifact。
   - Flink：允许 checkpoint/savepoint/HA/job-result-store 和模型只读，拒绝模型写入。
   - Argo：允许 workflow artifact，拒绝模型、PCAP、报告和 checkpoint。

7. canary 证据必须记录 identity ID、测试对象前缀、预期、实际 HTTP/S3 错误码、trace/run ID 和时间，不记录凭证。

## 4. 消费者灰度顺序

按以下顺序一次只切一个身份，每步完成健康、功能和拒绝面验证后再继续：

1. Argo artifact controller。
2. MLOps model writer。
3. Flink state/model reader；必须绑定 savepoint 回滚点并验证 checkpoint 恢复。
4. Asset service。
5. Alert service。
6. Forensics service。
7. Probe agent；先单节点/单 probe，再扩至全部 DaemonSet Pod。

每个消费者至少验证启动、readiness、一次允许操作、一次拒绝操作、审计/指标和原业务结果。HTTP 2xx、Pod Ready 或对象存在任一单项都不能单独作为切换完成证据。

## 5. 停止条件与回滚

出现下列任一情况立即停止扩大：策略超出合同、跨 bucket/前缀访问成功、合法访问被拒绝、对象 manifest 不一致、checkpoint 无法恢复、PCAP ACK 失败、报告/模型/Argo artifact 出现部分成功，或错误率/延迟/资源越界。

回滚步骤：

1. 将当前消费者 Deployment/WorkflowTemplate 恢复到候选前已验证的 Secret 引用和镜像，不批量回滚其他已健康身份。
2. 保留新用户和策略用于取证，不在故障处理中删除；Job 重新保持 suspended。
3. 验证旧路径恢复、在途 multipart/job/checkpoint 状态和 PG manifest/审计结果。
4. 记录失败 occurrence ID。旧 occurrence 若已关闭，不得重开。

## 6. 收缩与关闭条件

只有七身份全部完成 canary、灰度、跨租户负向验证、T+0/T+1/T+3/T+7 观察，且 TLS 切片和 live reconcile 通过后，才可进入 contract：

1. 删除所有应用工作负载对 `traffic-credentials.MINIO_ACCESS_KEY/MINIO_SECRET_KEY` 的引用。
2. 轮换 root 凭证，并确认 root 只供 MinIO server 和审批后的管理 bootstrap 使用。
3. 解除并删除旧应用共享身份/策略；保存 before/after policy hash 和审计证据。
4. 将 T-MINIO-003/004 从 `IMPLEMENTING` 推进到 `VERIFYING/OBSERVING`，完成完整观察期后才能 `CLOSED`。

在 live 执行和观察完成前，结论只能是“仓库 expand 候选通过”，不得写成生产整改完成。

## 7. 官方命令参考

- [MinIO `mc admin user add`](https://docs.min.io/aistor/administration/iam/identity/built-in-identity/)
- [MinIO `mc admin policy create`](https://docs.min.io/aistor/reference/cli/admin/mc-admin-policy/mc-admin-policy-create/)
- [MinIO `mc admin policy attach`](https://docs.min.io/aistor/reference/cli/admin/mc-admin-policy/mc-admin-policy-attach/)

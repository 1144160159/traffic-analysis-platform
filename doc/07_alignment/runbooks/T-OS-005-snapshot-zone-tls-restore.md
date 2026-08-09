# T-OS-005 OpenSearch 分区隔离、TLS、快照与恢复手册

## 1. 状态与边界

本手册对应 `T-OS-005 / WP-23-OS`。仓库中的 `opensearch-ha-v1` 是默认不部署的候选目标，不是生产完成证据。OpenSearch 是可重建投影；ClickHouse、PostgreSQL、Kafka 与 MinIO manifest 仍是权威来源。快照用于缩短投影恢复时间，不能替代权威数据备份、跨存储对账或投影重建能力。

当前生产集群只有两个 Kubernetes 节点，没有 `topology.kubernetes.io/zone` 标签；三个 OpenSearch Pod 中两个位于同一节点，HTTP 仍为明文，共享 admin 凭据仍被应用使用，`_snapshot` 仓库为空。因此本项保持 `IMPLEMENTING/PARTIAL`，不得写成 HA、TLS 或恢复验收完成。

## 2. 不变量和停止条件

- 三个 OpenSearch Pod 必须位于三个可调度 zone；zone 标签来自真实故障域，不得把同一机房或同一宿主机伪装成三个 zone。
- `cluster.routing.allocation.awareness.attributes=zone`，并强制 `zone-a,zone-b,zone-c`；缺少任一故障域时停止扩大。
- HTTP 与 transport 均启用 TLS，transport 校验主机名，HTTP 要求客户端证书；禁止明文回退。
- alert-service、asset-service、Flink sink、snapshot operator 和 monitor 使用不同证书身份。业务容器和健康探针不得使用 admin。
- 低、高、flood-stage 水位分别为 70%、80%、90%。达到 high、出现持续 unassigned shard、pending task 增长或 write rejection 时停止发布；达到 flood-stage 立即回滚流量切换并禁止扩大。
- 快照仅接受 `SUCCESS`；恢复必须在 UUID 不同的隔离集群，使用新索引名，禁止覆盖现有索引或同集群恢复。
- 旧索引删除前必须确认活动 PIT 为零，或等待批准的 PIT 到期；恢复和重建期间不得缩短 PIT 生命周期。
- OpenSearch 不可用时，API 必须返回带 `trace_id`、`partial/missing_sections` 的降级或错误，浏览器必须显示“数据源不可用/可重试”，禁止呈现为空告警或零结果。
- 数据丢失、跨租户、mapping/settings/alias/文档/版本/hash/query oracle 不一致时立即停止。

## 3. 候选制品

- 合同：`contracts/opensearch/ha-security-restore.v1.json`
- opt-in overlay：`deployments/kubernetes/security/opensearch-ha-v1/`
- 插件镜像：`deployments/opensearch/Dockerfile.ha-v1`
- 服务角色：`deployments/kubernetes/security/opensearch-ha-v1/service-roles.v1.json`
- 告警规则：`deployments/kubernetes/observability/opensearch-ha-alert-rules.yaml`
- 计划/恢复工具：`scripts/alignment/opensearch_snapshot_restore.py`
- 受控渲染器：`scripts/alignment/render_opensearch_ha_security.py`

基础清单位于 overlay 目录之外，只有受控渲染器允许以 `LoadRestrictionsNone` 读取该已知文件。禁止使用通用的宽松 kustomize 配置。当前镜像为 `registry.invalid` 加全零 digest 的 fail-safe 占位符；不替换为已构建、扫描并批准的真实不可变 digest时，`--require-approved-image` 必须失败。

## 4. 发布前只读预检

在独立 run 目录保存以下输出、stderr、时间和 SHA-256：

```bash
kubectl get nodes -L topology.kubernetes.io/zone -o wide
kubectl -n middleware get pod -l app=opensearch -o wide
kubectl -n middleware get statefulset opensearch -o yaml
kubectl -n middleware exec opensearch-0 -- curl -fsS http://127.0.0.1:9200/_cluster/health
kubectl -n middleware exec opensearch-0 -- curl -fsS http://127.0.0.1:9200/_cluster/settings?include_defaults=true
kubectl -n middleware exec opensearch-0 -- curl -fsS http://127.0.0.1:9200/_cat/allocation?format=json
kubectl -n middleware exec opensearch-0 -- curl -fsS http://127.0.0.1:9200/_cat/shards?format=json
kubectl -n middleware exec opensearch-0 -- curl -fsS http://127.0.0.1:9200/_cluster/pending_tasks
kubectl -n middleware exec opensearch-0 -- curl -fsS http://127.0.0.1:9200/_snapshot
```

预检必须证明三个不同 zone、PDB、存储容量、无持续 relocation/unassigned/pending/rejection、证书 SAN/链完整、ExternalSecret 已同步、服务身份和角色已在隔离环境验证。不得为了通过预检临时修改标签、禁用校验或扩大权限。

## 5. 镜像、PKI 与服务身份迁移

1. 构建 `Dockerfile.ha-v1`，生成 SBOM、漏洞扫描和签名；将 overlay 的 fail-safe 镜像替换为批准 digest。
2. 为每个 Pod 颁发包含 `<pod>.opensearch.middleware.svc` 的节点证书；transport 证书的 DN 进入 nodes DN，admin 证书只供受控 securityadmin/破窗流程。
3. 通过 External Secrets 提供 node、admin、monitor、snapshot operator、alert-service、asset-service 和 Flink 客户端证书；私钥和口令不得进入 Git、ConfigMap、日志或证据包。
4. 将 `service-roles.v1.json` 转换为经审查的 OpenSearch Security roles/roles_mapping。先在隔离集群验证每个身份的允许和拒绝矩阵，再执行 securityadmin 变更。
5. 应用侧先支持 CA、client cert 和 key，逐个内部 tenant 灰度。所有客户端从明文和共享 admin 迁出后，才允许启用 HTTP `clientauth_mode=REQUIRE`。
6. serial canary 轮换证书，验证新旧信任窗口、撤销和回滚；禁止一次性替换全部节点证书。

## 6. S3 仓库与周期快照

镜像包含 `repository-s3`，S3 凭据必须通过受控 Secret 写入 OpenSearch keystore。注册仓库时固定 bucket、base path、服务端加密、只允许 snapshot operator 管理。先做 repository verify，再运行计划模式：

```bash
python scripts/alignment/opensearch_snapshot_restore.py snapshot \
  --endpoint https://opensearch.middleware.svc:9200 \
  --ca-file /run/secrets/ca.crt --cert-file /run/secrets/tls.crt --key-file /run/secrets/tls.key \
  --repository traffic-s3 --snapshot alerts-YYYYMMDDTHHMMSSZ \
  --indices alerts-v2 assets-v2
```

计划输出的 `mutations` 必须为空。只有变更单、operator、reason 和 approval ID 完整时才可增加 `--execute`。快照任务必须记录开始/结束、状态、分片、索引、大小、对象清单和审计事件；周期任务失败或超过25小时无成功快照必须告警。

## 7. 隔离恢复审批清单

恢复审批 JSON 必须由变更系统产生并独立签认，至少包含：

```json
{
  "approval_status": "APPROVED",
  "operation": "opensearch_isolated_restore",
  "approval_id": "CHG-...",
  "approved_by": "independent-reviewer",
  "expires_at": "2026-08-05T12:00:00Z",
  "target_isolated": true,
  "source_endpoint_sha256": "<sha256>",
  "target_endpoint_sha256": "<sha256>",
  "source_cluster_uuid": "<uuid>",
  "target_cluster_uuid": "<different-uuid>",
  "repository": "traffic-s3",
  "snapshot": "alerts-...",
  "indices": ["alerts-v2"],
  "rename_pattern": "^(.+)$",
  "rename_replacement": "restore-<run>-$1",
  "verification": {
    "required": ["mapping_sha256", "settings_sha256", "aliases", "document_count", "sample_document_ids", "sample_versions", "sample_content_sha256", "query_oracle"],
    "indices": [{"source_index": "alerts-v2", "restored_index": "restore-<run>-alerts-v2", "expected": {}, "samples": [], "queries": []}]
  }
}
```

真实审批必须填写所有 expected、抽样文档和有界 query oracle。执行前保存文件 SHA-256。先运行不带 `--execute` 的 restore 计划；计划会读取源/目标 UUID、检查 `SUCCESS` snapshot、检查索引集合以及目标索引不存在。执行必须同时提供 `--approved-manifest-sha256`、operator 和 reason：

```bash
python scripts/alignment/opensearch_snapshot_restore.py restore \
  --source-endpoint https://source.example:9200 \
  --target-endpoint https://isolated-restore.example:9200 \
  --approved-manifest /approved/restore.json \
  --approved-manifest-sha256 <sha256> \
  --source-ca-file /run/source/ca.crt --source-cert-file /run/source/tls.crt --source-key-file /run/source/tls.key \
  --target-ca-file /run/target/ca.crt --target-cert-file /run/target/tls.crt --target-key-file /run/target/tls.key
```

增加 `--execute --operator <id> --reason <ticket>` 后才会发生恢复。工具恢复为只读的新索引，并立即核验 mapping、settings、aliases、count、抽样 ID/version/content hash 和 query oracle；任一不一致返回失败且不得切换 alias。

## 8. 灰度、切换与回滚

采用 expand → client dual trust → 三节点逐个滚动 → 内部 tenant → 非关键 tenant → alias/流量切换。每一步记录集群状态、allocation explanation、任务队列、拒绝、磁盘、P50/P95/P99 和数据 oracle。旧路径和旧证书保留到 T+7 且至少一个完整业务周期。

回滚优先顺序：停止扩大；将客户端切回旧 endpoint/alias；恢复旧信任链；滚回上一个 digest；等待 shard 稳定；对权威源和投影对账。禁止通过关闭 TLS、关闭 hostname verification、移除 zone awareness、清空数据目录或覆盖恢复索引来“恢复服务”。

## 9. 故障演练与关闭证据

必须独立执行并保存以下演练：data/master 节点失败、单 zone 丢失、磁盘 high/flood、bulk 部分失败、mapping conflict、alias cutover、snapshot failure、隔离恢复、证书轮换和过期、PIT 与旧索引生命周期冲突。每次记录注入、影响、告警、降级 UI、RTO/RPO、数据对账和回滚。

关闭需要 G0—G7 全链证据，以及 T+0、T+1、T+3、T+7 观察。当前外部三 zone 资源、破坏性窗口、独立恢复环境和浏览器候选证据缺失时，只能保持 `PARTIAL/BLOCKED`，不能以清单、HTTP 2xx 或截图单独关闭。

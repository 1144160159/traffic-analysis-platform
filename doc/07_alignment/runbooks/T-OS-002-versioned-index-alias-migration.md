# T-OS-002 OpenSearch 版本化索引与 alias 迁移手册

## 1. 当前结论

本手册对应 `T-OS-002`，只提供候选、只读盘点、门禁和经批准后的执行顺序。仓库候选状态为 `IMPLEMENTING/PARTIAL`，`production_applied=false`。

2026-08-05 的最新只读现场盘点显示：集群 green、3 个数据节点、0 个未分配分片；物理索引 `alerts` 有 73,477,908 个文档、109,233,533,216 bytes，总体为 1 primary + 1 replica。当前没有业务 alias、没有组件模板、没有 ISM policy。实际 `alerts` 由动态映射创建，tenant、ID、IP、severity、status 等关键字段为 `text + keyword` 或 text，主字段不可聚合；运行时生成的 `traffic-alerts-template` 只匹配 `traffic-alerts-*`，未治理真实 `alerts`。

不得把仓库静态 PASS、模板 dry-run 或 HTTP 2xx 写成迁移完成。生产 apply、72M 文档回填、对账、原子切换、回滚和 T+7 观察均未执行。

当前候选完整 G0：`doc/02_acceptance/runs/20260805-remediation-g0-full-v133/manifest.json#sha256=7f1957d3468ff8bea9d324783f5ce5f502d55b106c020b3149d9d1019f45732d`。同候选只读治理证据：`doc/02_acceptance/runs/20260805-remediation-opensearch-index-governance-v5/manifest.json#sha256=9ba94f88c9f16e5e74b0731685f15a662bc5baf7f1c0e98901f30733006c5a3d`；其 `scoped_evidence_status=PASS`、总状态仍为 `PARTIAL`、`production_applied=false`。其中 backfill 预检绑定的 default tenant 一秒窗口为 14 条、未知 strict 字段为 0、生产变更为空，并因目标 write alias 尚未创建而按预期 `BLOCKED`。

## 2. 候选对象

| 对象 | 值 |
|---|---|
| mappings component | `traffic-alerts-mappings-v2` |
| settings component | `traffic-alerts-settings-v2` |
| index template | `traffic-alerts-v2-template`，version 2，priority 300 |
| ISM policy | `traffic-alerts-hot-delete-v2` |
| 物理索引 | `alerts-v2-000001`，后续按数字后缀 rollover |
| read alias | `alerts-v2-read` |
| write alias | `alerts-v2-write` |
| 业务开关 | Go `OPENSEARCH_ALERTS_V2_ENABLED=false`；Flink `opensearch.alerts.v2.enabled=false` |

应用启动过程不再创建或更新模板。40 个字段使用 `dynamic=strict` 固定映射；过滤/聚合字段使用 keyword、ip、integer、short、float、date 等确定类型，全文检索集中到 `search_text`。

## 3. 执行前门禁

以下全部满足前禁止执行生产写操作：

1. 候选 manifest、镜像 digest、配置 hash 和 G0 结果冻结。
2. 由 OpenSearch owner、告警 owner、Flink owner、SRE、QA 和安全共同批准变更单。
3. 固定数据规模、回填速率、最大并发、磁盘/JVM/CPU 水位、P95/P99 和停止条件。
4. 目标索引空间预算至少覆盖新索引、旧索引、merge 临时空间、replica 和回滚保留。
5. 校验所有 live 字段均能被 strict v2 映射接收；未知字段、非法 IP/日期和超长值先形成可审计转换规则。
6. 校验租户权限过滤、alert_id 确定性、重复写幂等和 bulk 部分失败处理。
7. 快照仓库、最近成功快照和隔离恢复演练可用。

## 4. expand 与模板验证

日常部署目录 `deployments/kubernetes/init-jobs/04-opensearch-templates.yaml` 明确不包含告警 V2 expand；不得用例行部署绕过迁移审批。独立 manifest 的 Job 默认 `suspend=true`、`backoffLimit=0`，仅创建挂起对象，不会访问或修改 OpenSearch。

以下操作会改变集群或 OpenSearch，仅可在批准窗口执行。审批必须绑定变更单、审批人、一次性 nonce、最长 4 小时的 not-before/expiry、目标 cluster UUID、同一候选 G0 source hash、G0 manifest hash，以及固定合同 hash `c6c3b32eebf6b1ab6da472e9c0849ba613b59c8ebdeaea0e0ee1aeb8c8895c28`。禁止手工创建可复用的静态审批 Secret，也禁止直接解除仓库基线 Job 的暂停；基线 Job 的 `EXPECTED_APPROVAL_NONCE=UNRENDERED` 会主动拒绝运行。

```bash
python3 scripts/alignment/render_opensearch_alerts_v2_expand.py \
  --run-id <unique_run_id> \
  --approval-id <approved_change_id> \
  --approved-by <independent_approvers> \
  --cluster-uuid <approved_cluster_uuid> \
  --not-before <approved_rfc3339_start> \
  --expires-at <approved_rfc3339_end_within_4h> \
  --g0-manifest <matching_full_g0_manifest> \
  --output /tmp/<unique_run_id>-expand.yaml

kubectl apply --dry-run=client -f /tmp/<unique_run_id>-expand.yaml -o name
kubectl apply -f /tmp/<unique_run_id>-expand.yaml

kubectl -n middleware patch job <rendered_expand_job_name> \
  --type=merge -p '{"spec":{"suspend":false}}'
kubectl -n middleware wait --for=condition=complete \
  job/<rendered_expand_job_name> --timeout=300s
```

renderer 拒绝覆盖已有产物，并核对完整 G0 与当前 candidate source hash；生成的审批 Secret 使用唯一名称且 `immutable=true`，Job 仍保持 `suspend=true`。Job 在第一条 OpenSearch 写入前核验 nonce、窗口有效期、合同哈希、G0 哈希格式、审批字段和目标 cluster UUID。随后读取并保存组件模板、组合模板模拟、ISM explain、物理索引 settings/mapping 和两个 alias。必须确认业务开关仍为 false，legacy `alerts` 未被删除、未改 alias、未改 mapping。审批 Secret 和 Job 日志进入不可变证据包后按安全策略清理 Secret；不得删除 Job 结果或审批记录。

## 5. shadow/backfill

先对小 tenant 和明确时间片生成只读计划；只有 `execution_readiness=READY` 才能进入独立审批的回填执行器。规划器只调用根信息、集群健康、mapping、alias、allocation 和 `_count`，不会调用 `_reindex`。tenant 不允许通配符，时间窗最多 1 小时，单 slice 最多 100,000 文档、最多 4 slices、最高 500 requests/s，节点最小可用空间不得低于 150 GiB。

```bash
python3 scripts/alignment/plan_opensearch_alerts_v2_backfill.py \
  --tenant-id <one_explicit_tenant> \
  --start-time <rfc3339_start> \
  --end-time <rfc3339_end_within_1h> \
  --time-field last_seen \
  --max-documents 100 \
  --slices 1 \
  --requests-per-second 10 \
  --min-free-bytes 161061273600 \
  --output /tmp/<unique_run_id>-backfill-plan.json
```

计划绑定 cluster UUID、源索引、目标 alias/唯一 write index、tenant、时间窗、文档数、mapping 字段、磁盘下限和完整请求体，并生成 `plan_sha256`。alias 尚未 expand、源范围超量、严格 mapping 不兼容、磁盘不足或健康异常时返回 `BLOCKED`，不得绕过。禁止复制计划中的 `_reindex` 请求手工执行；实际回填必须由批准且默认挂起、绑定相同 plan hash 的执行 Job 发起。

`READY` 计划生成后 15 分钟内，使用同一候选 G0 和独立审批渲染一次性 backfill Job：

```bash
python3 scripts/alignment/render_opensearch_alerts_v2_backfill.py \
  --plan /tmp/<unique_run_id>-backfill-plan.json \
  --run-id <unique_backfill_run_id> \
  --approval-id <approved_change_id> \
  --approved-by <independent_approvers> \
  --not-before <approved_rfc3339_start> \
  --expires-at <approved_rfc3339_end_within_4h> \
  --g0-manifest <matching_full_g0_manifest> \
  --output /tmp/<unique_backfill_run_id>-backfill.yaml

kubectl apply --dry-run=client -f /tmp/<unique_backfill_run_id>-backfill.yaml -o name
kubectl apply -f /tmp/<unique_backfill_run_id>-backfill.yaml
kubectl -n middleware patch job <rendered_backfill_job_name> \
  --type=merge -p '{"spec":{"suspend":false}}'
kubectl -n middleware wait --for=condition=complete \
  job/<rendered_backfill_job_name> --timeout=14400s
```

renderer 核验计划为 `READY`、未被篡改且不超过 15 分钟，并绑定 candidate、G0 manifest、contract 文件、cluster UUID、plan 与 plan-file SHA。生成的 ConfigMap 和审批 Secret 均为 immutable，Job 默认 `suspend=true`、`backoffLimit=0`。Job 执行前重新核验 source count、唯一 write index 和目标切片为空；执行期间轮询 task，审批过期即请求 `_cancel`；完成后要求 failures 为空、version conflict 为 0，且目标切片计数等于计划源计数。任何一项不满足均退出失败，不得扩大下一切片。

通过后再对该小 tenant 和明确时间片执行 slice 化、限速回填；禁止直接对 72M 文档运行无界 reindex。每个 slice 记录 task ID、源查询、目标索引、速率、开始/结束水位、成功、version conflict、失败和取消结果。

推荐顺序：

1. 选内部 tenant 和最小时间片，使用确定性 `_id=alert_id`。
2. 开启低速 shadow backfill，持续监控磁盘、JVM、merge、refresh、search/index latency 和 rejection。
3. 逐级扩大 slice；任一停止条件触发即 `_tasks/<task_id>/_cancel`，不得继续扩大。
4. 回填结束后保留源 `alerts`，不立即切换或删除。

## 6. G3 对账 oracle

对账至少包含：

- 全量与 tenant×时间桶文档数；
- alert_id 集合差异和重复数；
- 固定抽样 alert_id 的规范化 `_source` SHA-256；
- tenant、severity、status、alert_type、IP 和时间范围过滤结果；
- viewer/operator/admin 权限过滤及跨租户负例；
- 冷热缓存 P50/P95/P99、错误率、timeout、partial 和资源水位；
- bulk item failure、mapping rejection、429、节点超时和任务取消。

任何数据丢失、跨租户、差异扩大、manifest 不一致、P99 或资源越界均立即停止。

## 7. 切换与回滚

先在内部 tenant 启用受控 dual-write，并以 alert_id 重放证明幂等。对账稳定且经批准后，先切 read target 到 `alerts-v2-read`；观察通过后才切 write target 到 `alerts-v2-write`。两个切换必须绑定独立 revision、审计、候选 digest 和回滚决定。

回滚时先把业务读写开关恢复到 legacy target，再取消在途回填/dual-write，确认旧 `alerts` 可读写并完成反向对账。旧索引、旧镜像、旧配置和回滚 manifest 至少保留到 T+7；不得在观察窗内删除源索引。

## 8. 关闭条件

只有 expand、backfill/shadow、G3 对账、性能、故障、权限、指定 Windows Chrome、灰度、切换、回滚和 T+0/T+1/T+3/T+7 全部形成不可变证据，`T-OS-002` 才能从 `IMPLEMENTING` 进入 `VERIFYING/OBSERVING`，再由独立验收人裁决关闭。该项关闭也不替代 `T-OS-003/004/005` 或 G8 项目级门禁。

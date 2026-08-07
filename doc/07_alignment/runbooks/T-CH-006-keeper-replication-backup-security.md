# T-CH-006 Keeper、复制、备份与安全运行手册

## 当前结论

本候选只完成仓库侧护栏，状态为 `IMPLEMENTING/PARTIAL`，`production_applied=false`。2026-08-04 的只读采样显示四个 ClickHouse Pod 与三个 Keeper Pod 均运行；Keeper 为一个 leader、两个 synced follower；复制表无 readonly、队列和绝对延迟均为零。该快照只说明采样时健康，不证明故障域、告警、TLS、逐服务权限、备份恢复或故障演练已经完成。

当前集群仅有 `8-2tb` 与 `zeus-server` 两台节点，且没有 `topology.kubernetes.io/zone` 标签。三个 Keeper 中两个位于同一节点；`preferredDuringSchedulingIgnoredDuringExecution` 只是调度倾向，不能作为三故障域证据。不得把它直接改为 hostname 级强反亲和，否则三副本在两节点上不可调度。

## 仓库候选

- ClickHouse Server 与 Keeper 声明 `/metrics:9363` 原生指标端点，Pod 和 Service 同步暴露该端口。
- `clickhouse-ha-alert-rules-v1` 定义 Keeper 可达数、指标端点、readonly、复制队列与延迟、part、mutation、merge、磁盘和 Distributed 队列规则。
- `traffic_runtime` profile 固定内存、线程、执行时间、读取字节、分布式连接和深度；`skip_unavailable_shards=0` 且禁止回退到陈旧副本，先以失败阻断静默少算。
- profile 当前没有绑定用户；逐服务用户和 TLS 切换前不得宣称安全基线生效。
- 合同分别定义原始事实、告警/证据和派生聚合在丢副本或分片时的失败或显式 `partial` 语义。
- 备份计划登记关键原始事实、DDL/dictionary 权威、RPO/RTO 和隔离恢复 oracle，但目标、密钥恢复和真实 restore 尚未批准或执行。

## 发布前检查

1. 重新采集节点、zone、Pod、PVC、Keeper `mntr`、`system.replicas`、`system.replication_queue`、`system.parts`、`system.mutations`、`system.merges`、`system.distribution_queue`、`system.disks`、用户、quota 和关键 settings。
2. 在与生产镜像 digest 相同的隔离 Pod 验证 ClickHouse 与 Keeper 配置可加载，并抓取 `/metrics`，确认规则使用的指标名全部存在。
3. 部署指标 collector、rule evaluator 与通知路由；当前仅有 VictoriaMetrics 单节点存储，没有 vmagent/vmalert 或 Prometheus Operator CRD，Pod scrape annotation 本身不会形成告警。
4. 对每个服务建立唯一用户、ExternalSecret、最小数据库/表权限和 `traffic_runtime` 派生 profile；先双栈启用 9440/TLS，再迁移客户端，最后撤销 default 的远程访问。禁止把同一密码复制给不同身份。
5. 依据 profile 记录真实 query log 的 read_rows、read_bytes、memory、elapsed、并发和 P99；越界时调整已评审的派生 profile，不直接放开全局 default。
6. 审批三故障域拓扑、备份对象存储、容量、加密密钥恢复、保留、RPO/RTO 和破坏性演练窗口。

## 故障与部分结果裁决

- 丢一个副本：只在另一个副本为当前状态时服务；否则查询失败，不返回完成态空集或低计数。
- 丢一个分片：原始事实与告警/证据默认失败；允许降级的派生聚合必须返回 `meta.partial=true`、`missing_sections`、`source_watermarks` 和 `trace_id`。
- `HTTP 2xx`、截图、Keeper leader 存在、Pod Running 或备份 Job Success 均不能单独证明业务成功。
- 客户端、网关或 API 若无法表达 `partial`，必须保持 fail-closed；不得仅设置 `skip_unavailable_shards=1`。

## 备份与恢复

备份目标必须为批准的加密对象存储，并保留不可变 manifest、对象 SHA-256、ClickHouse 版本、schema migration 版本和 Keeper/集群拓扑。恢复只在新隔离集群进行：先恢复 DDL/dictionary，再恢复关键事实，最后构建副本并开放只读验证。

切读前至少比较数据库/表/分区清单、tenant×分区行数、稳定主键样本、count/sum/min/max、readonly 与复制队列，并执行应用查询 oracle。任何差异、缺失 manifest、密钥不可恢复或 RPO/RTO 越界都判定恢复失败。V2 大表迁移与 restore 演练必须安排在不同资源窗口。

## 演练与回滚

需在批准窗口分别演练 Keeper 单节点丢失、复制延迟、分片不可用、磁盘水位、坏 part 和隔离恢复。每次只引入一个故障，保存操作前后 trace、指标、查询响应、权威数据、投影、审计、恢复时间和回滚证据。

配置发布采用单副本 canary，再逐 shard 扩大。出现 Keeper quorum 风险、readonly、复制/Distributed 队列持续扩大、磁盘低水位、静默少算、TLS/身份失败或资源越界时立即停止；恢复旧配置和旧客户端凭据路径，确认队列收敛后再裁决。按 T+0、T+1、T+3、T+7 观察，未经独立 QA/SRE/安全签字不得关闭 T-CH-006。

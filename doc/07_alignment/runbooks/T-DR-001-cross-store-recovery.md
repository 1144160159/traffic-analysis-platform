# T-DR-001 跨存储灾备与恢复运行手册

## 当前结论

本手册只建立八个数据域的恢复权威、顺序、证据和停止条件，不授权故障注入、主从切换、对象覆盖、同集群恢复或生产流量切换。仓库门禁通过仅代表 `IMPLEMENTING/PARTIAL`；八个域当前均无可接受的隔离恢复证据，也没有完整获批的 RPO/RTO，因此不得宣称灾备完成。

版本化真源为 `contracts/reliability/dr-recovery-catalog.v1.json`。任何单域合同、备份作业成功、Pod Ready、HTTP 2xx 或截图都不能代替跨存储恢复验收。

## 恢复顺序

1. 恢复并验证身份、DNS、配置、证书信任、密钥托管和审计入口；不得从证据包还原私钥或口令。
2. 在隔离环境恢复 PostgreSQL 权威业务状态、revision、审计和 outbox；确认唯一可写主、单调 primary epoch、旧主 fencing、WAL 连续性和 PITR 目标。
3. 恢复 Kafka Topic、分区、副本、ACL、Schema 和批准的 offset 范围；只有 PostgreSQL 权威状态稳定后才允许回放事件。
4. 恢复 ClickHouse 原始事实、DDL 和 dictionary 权威，按 tenant、分区、稳定业务键和聚合 oracle 对账。
5. 恢复 MinIO 对象，并逐条对照 PostgreSQL manifest 的 bucket、key、size、SHA-256、类型、权限和保留策略；对象存在不等于 manifest 完成。
6. 从 PG/CH/Kafka 水位重建或隔离恢复 Nebula 与 OpenSearch 投影；保存确定性节点、边、文档 ID 和版本对账。
7. 恢复 Redis coordination/session 的 AOF 与 Sentinel 语义，cache 域允许清空后从权威存储重建；不得让 cache 成为事实真源。
8. 使用已批准且 hash 固定的 savepoint 恢复 Flink；禁止 `allowNonRestoredState`，并核对 source offset、watermark、DLQ 和外部 sink。
9. 执行跨存储 trace、event ID、aggregate revision、watermark、manifest、projection 和浏览器业务旅程对账；未通过不得切换生产读取或写入。

## 演练前置门禁

- 变更单明确候选 hash、精确域、恢复点、隔离目标、容量、RPO/RTO、owner、reviewer、两名审批人和维护窗口。
- 备份 manifest 不可变，包含对象 SHA-256、大小、加密/密钥恢复状态、Schema/migration、源时间线或 offset、水位及保留策略。
- 隔离目标具有不同集群身份、网络和写端点；禁止覆盖生产 PVC、Topic、bucket、index、space 或 savepoint。
- 每个域具有业务 oracle，不得只比较资源数量、Pod Ready、备份任务退出码或 HTTP 2xx。
- PG、Kafka、Redis、Flink 和投影 writer 的 fencing/epoch 方案已审核；任何时刻不能存在两个可写权威。
- HA 故障域、加密密钥恢复、告警和回滚制品均已验证；资源或 P99 越界自动停止扩大。

## 逐域验收最低证据

- PostgreSQL：base backup/WAL manifest、`pg_verifybackup`、隔离 PITR、timeline、唯一主、fencing、事务标记、outbox 与租户对账。
- Kafka：Topic/partition/replica/ACL/Schema 快照、恢复 offset、空洞与重复判定、确定性 event ID 下游对账。
- ClickHouse：数据库/表/分区、tenant 行数、稳定主键样本、count/sum/min/max、replica queue、readonly 和应用查询 oracle。
- MinIO：PG manifest 与对象的 bucket/key/size/SHA-256/类型/权限/保留逐项一致，且 orphan、missing、corrupt 明确为零或获批 partial。
- Nebula：meta/storage 故障域、space/schema、确定性 VID/edge key、tenant 有界抽样及 PG/CH 来源水位。
- OpenSearch：成功 snapshot、隔离 rename restore、mapping/settings/alias、文档数、ID/version/content hash 和查询 oracle。
- Redis：Sentinel 主切换、AOF 恢复、coordination/session fail-closed、cache 丢弃重建、跨域 key prefix 为零。
- Flink：checkpoint/savepoint URI 与 SHA-256、artifact/image digest、operator UID/state 兼容、恢复时间、offset/watermark、DLQ 与 sink 对账。

## 停止条件

出现任一情况立即停止：多主或 fencing 未知；WAL/offset/checkpoint 缺口；恢复目标不是隔离环境；manifest/hash 不一致；对象和 metadata 部分成功被写成完成；跨租户数据；PG、Kafka、CH、MinIO、投影间差异持续扩大；RPO/RTO、P99、磁盘、网络或内存越界；加密密钥不可恢复；回滚制品缺失；候选 hash 漂移。

停止后保留所有源和恢复证据，禁止通过删除 DLQ、重置 offset、覆盖对象、手改审计、清空差异表或重建同名索引来伪造一致。恢复执行者不得单独批准自己的结果。

## 回滚与关闭

回滚先冻结新 writer 和自动切换，确认唯一保留权威及 epoch，再把流量指回已验证的旧端点；在途任务必须进入可判定的失败、取消或补偿状态。旧 Schema、Topic、索引、对象和 savepoint 至少保留至 T+7 并按批准流程处置。

关闭 `T-DR-001` 需要八域隔离恢复、跨存储对账、故障注入、回滚和 T+0/T+1/T+3/T+7 观察均通过独立 QA/SRE/安全签认。G7 通过仍不等于 G8 项目级 HA、现场和第三方验收完成。

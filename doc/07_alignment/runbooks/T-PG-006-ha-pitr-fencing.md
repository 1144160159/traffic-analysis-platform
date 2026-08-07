# T-PG-006 PostgreSQL HA、Fencing 与 PITR 手册

## 1. 当前边界

仓库当前处于 `safe-hold-readiness-only`，`production_applied=false`。这表示旧的无 fencing 自动 `pg_promote()` 路径已从候选清单移除，`pg-failover-check` 仅执行只读角色检查；它不表示自动故障切换、PITR、TLS、RTO/RPO或生产演练已经完成。

当前静态拓扑仍是一个声明主库、两个声明副本和静态写 Service。主库 readiness 必须返回 `NOT pg_is_in_recovery()`，副本 readiness 必须返回 `pg_is_in_recovery()`。任一角色不符均失败关闭，不允许CronJob自行提升副本、修改标签或切换Service。

## 2. 启用自动切换的前置门禁

只有以下能力由同一个经过批准的HA控制器闭合后，才能修改合同中的 `automated_promotion_enabled=false`：

1. 使用分布式租约或共识选出唯一主，并在Pod之外持久化单调 `primary_epoch`。
2. 发布新写端点前完成旧主fencing；网络隔离、存储隔离或等价STONITH必须可验证。
3. 选主、fencing、角色标签和写端点发布由同一控制协议负责，禁止两个控制器各自提升。
4. 客户端使用TLS写端点，具备有限重试、重新解析拓扑、连接与statement/lock timeout。
5. 旧主只能作为新副本重新加入；必须执行rewind或重新base backup，不得直接恢复写服务或抢占。
6. 维护窗口、审批人、回滚负责人、RPO/RTO、停止扩大条件和证据目录已经冻结。

## 3. PITR 准备与恢复

PITR必须使用持久WAL归档和加密基础备份，不能把同一PVC上的文件称为灾备。每次备份记录timeline、LSN、开始/结束时间、对象version、sha256、schema migration版本和权限。使用 `pg_verifybackup` 验证基础备份，连续性检查必须证明目标区间没有WAL缺口。

恢复只在隔离环境进行。先恢复身份、密钥和配置，再恢复基础备份，配置 `restore_command` 和获批的 `recovery_target_time`，保持目标库隔离和只读。恢复完成后记录实际recovery point，对tenant业务行、审计、outbox event_id/aggregate_version、对象manifest和副本timeline进行对账。未通过对账不得切流。

## 4. 维护窗口演练

演练前冻结候选hash、生产拓扑、复制槽、WAL归档水位、最近有效备份、主从LSN、业务事务marker和outbox高水位。依次演练主Pod、主节点、网络分区、写Service端点、存储挂载和PITR目标时间，不并行制造多个故障。

每个场景必须证明：任意时刻可写主数量为1，旧主在端点发布前已fenced；已确认事务损失不超过批准RPO；在途事务结果可判定；连接池在预算内恢复；outbox不丢失且没有不可接受重复；恢复旧主不会形成双主。

出现以下任一情况立即停止：可写主数量不等于1、fencing状态未知、WAL缺口、已确认事务超出RPO、outbox重复扩大、连接池持续向只读副本写入、恢复对象hash不一致或资源越界。

## 5. 回滚与恢复旧主

如果新控制器或切换失败，先阻断所有自动提升和端点变更，再由两名审批人确认唯一保留主及其 `primary_epoch`。对其余节点执行fencing，写Service只指向已确认主。回滚应用时保持兼容Schema和旧连接地址，但不得绕过TLS或角色检查。

旧主只能作为新副本重新加入：确认其不在写端点、清除旧租约、通过rewind或新base backup与当前timeline对齐，验证 `pg_is_in_recovery()` 为true后再加入只读Service。禁止把旧PVC直接挂回并启动为主。

发布后按T+0、T+1、T+3、T+7观察主epoch、复制延迟、WAL归档、slot、磁盘、只读错配、连接池恢复、审计和outbox。正式关闭需要独立QA/SRE批准的 `postgres-failover.md` 和跨组件RTO/RPO报告。

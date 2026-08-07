# T-FLINK-003 checkpoint、HA 与升级运行手册

本手册只允许在已批准窗口内使用。默认动作是停止扩大，不自动修改生产作业。

## 前置条件

1. 候选镜像必须使用 digest，合同、UID 清单、JAR SHA 和当前 source offset 已归档。
2. 当前作业无 root exception，最近 checkpoint 完成，Kafka lag 与 sink 对账在批准范围内。
3. MinIO checkpoint、savepoint、HA metadata 和 JobResultStore 前缀可由独立身份读写；不得依赖单节点 `file://`。
4. 同一时间只迁移一个作业，旧 artifact 和 savepoint 保留到 T+7 后经批准处置。

## 升级与验证

1. 触发 stop-with-savepoint，记录 trigger、job ID、source offsets、对象 URI、size 和 SHA-256。
2. 在隔离 Application Cluster 中以 `allowNonRestoredState=false` 恢复，核对 operator UID/state compatibility。
3. 用冻结输入范围比较旧、新输出的稳定业务 ID、数量、迟到、DLQ 和 sink 结果；差异未经批准不得灰度。
4. 灰度后要求两个 JobManager 候选、一个 leader、新 completed checkpoint、watermark 推进和 sink 对账同时成立。
5. 采集 checkpoint completed/failed、duration、alignment、state size、external path 和 restore time；成功率低于 99.9%、持续时间超过间隔 50% 或恢复超过 300 秒即停止扩大。

## 回滚

取消新 Application Cluster但不 dispose savepoint，以旧 digest artifact 从已记录 savepoint 恢复。禁止使用 `allowNonRestoredState`。确认新 checkpoint、source offsets、Kafka lag、迟到/DLQ、外部 sink 和业务 ID 对账后才解除 HOLD。

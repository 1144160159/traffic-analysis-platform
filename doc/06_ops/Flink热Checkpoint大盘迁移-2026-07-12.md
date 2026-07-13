# Flink 热 Checkpoint 大盘迁移报告

- 执行日期：`2026-07-12`
- 集群：`flink` namespace，Flink `1.18.1`
- 迁移前：`file:///home/k8s-data/flink/checkpoints/...`
- 迁移后：`s3://flink-checkpoints/checkpoints/...`
- Savepoint：`s3://flink-checkpoints/savepoints/...`
- 本地保留：`/home/k8s-data/flink/rocksdb`，继续承载 RocksDB 热状态。

## 迁移理由

两台节点的 `/home/k8s-data` 是本地 SSD，Flink TaskManager 跨节点调度时不能把该目录视为共享持久化存储。MinIO 由两节点四盘组成，底层使用大容量 HDD，适合作为 checkpoint/savepoint 的共享恢复路径。

未将 RocksDB localdir 迁移到 HDD，避免降低状态访问性能。HA 和旧 checkpoint 本地目录暂时保留，待稳定观察和恢复演练完成后再单独清理。

## 执行过程

1. 创建并核验 `flink-checkpoints` bucket，不配置 PCAP 的 14 天生命周期策略。
2. 对 Session Job 触发不中断 S3 savepoint canary，确认 S3 插件、endpoint 和凭据有效。
3. 先迁移 PCAP Index Job，验证从 S3 savepoint 恢复并连续写入 S3 checkpoint。
4. 依次迁移 User Behavior、Session、Feature、Rule、CEP、Behavior、Alert Generator 和 Device Log。
5. 每个作业均按“stop-with-savepoint -> 从 savepoint 恢复 -> RUNNING -> 新 S3 checkpoint”串行执行。
6. 更新 TaskManager 和 JobManager 的默认 checkpoint/savepoint 配置，先滚动 TaskManager，再滚动 JobManager。
7. 更新 `flink-job-config`、MinIO init job、提交脚本、Java 默认值和部署清单。

## 复验结果

| 作业 | 新 Job ID | Tasks | 最新 checkpoint 目录 |
|---|---|---:|---|
| Session Aggregation Job V2 | `edc950c92f1f63850ad88393f47416cf` | 24/24 | `checkpoints/session-job/` |
| Feature Extraction Job v3 | `8202597a48dbe13b8da7446a77e278da` | 18/18 | `checkpoints/feature-job/` |
| Rule Engine Job | `99935ddfea8fdbaad94e5785df41a261` | 12/12 | `checkpoints/rule-job/` |
| CEP Correlation Job | `82355e89a30d1c3f547d5418ed2ee0f9` | 32/32 | `checkpoints/cep-job/` |
| Behavior Detection Job | `2dddf7c3044e1254830fbec2e5c2a03d` | 12/12 | `checkpoints/behavior-job/` |
| Alert Generator Job | `a58e2d7bcc2f86b1fd16dd45e9149e7d` | 16/16 | `checkpoints/alert-generator-job/` |
| User Behavior Job | `a3a5d8abf2bf8d6911e12e7dc3e67bf8` | 10/10 | `checkpoints/user-behavior-job/` |
| PCAP Index Job | `f38b252f6827bbc275edc5e590f26267` | 2/2 | `checkpoints/pcap-index-job/` |
| Device Log Job | `8b3295a4f3ba74a8f8e57b19df0a3767` | 2/2 | `checkpoints/flink-traffic/` |

复验结论：

- `9/9` 作业为 `RUNNING`，共 `128/128` tasks RUNNING。
- 每个作业在 JobManager/TaskManager 滚动后均产生新的 `s3://flink-checkpoints/checkpoints/...` checkpoint。
- 当前每个作业的 exceptions 数组均为空。
- TaskManager 滚动瞬间部分作业累计记录一次 checkpoint failure，滚动后均已连续完成新 checkpoint，不是当前故障。
- 复验时 bucket 约 `14MiB`、`145` 个对象；容量会随状态量和保留策略动态变化。

## 配置真源

- `deployments/kubernetes/infrastructure/07-flink.yaml`
- `deployments/kubernetes/flink/flink-configmap.yaml`
- `deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml`
- `deployments/kubernetes/site-values.template.yaml`
- `java/flink-jobs/scripts/submit-*-job.sh`
- 各 Flink Job 的 Java/properties 默认 checkpoint 路径

## 回退与清理

- 本次迁移生成的 savepoint 保留在 `savepoints/migration-20260712/`，可用于回退。
- 旧 `/home/k8s-data/flink/checkpoints` 当前不删除。
- 至少完成一次 JobManager/TaskManager 故障恢复演练并确认 S3 restore 后，才能安排旧 checkpoint 清理。
- `flink-checkpoints` bucket 不使用 PCAP 归档生命周期；checkpoint 清理由 Flink externalized checkpoint 策略负责，savepoint 由运维审批后清理。

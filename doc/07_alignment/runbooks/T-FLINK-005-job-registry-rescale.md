# T-FLINK-005 Job Registry、运行态差异与可恢复扩缩容

## 1. 边界

本手册只覆盖九个 canonical Flink 作业的登记、候选绑定、运行态差异、savepoint 恢复和 key-group 扩缩容。仓库静态验证通过只允许把 `T-FLINK-005` 置为 `IMPLEMENTING`；没有候选镜像、真实 savepoint、运行作业快照和业务对账时，不得写成 G2/G3 或专项完成。

## 2. 三层登记

1. `contracts/flink/job-registry.v1.json` 是静态治理真源，登记 owner、max parallelism、operator UID hash、状态后端、输入起点、sink 保证、SLO和兼容范围。
2. release registry 绑定同一候选的 `candidate_source_sha256`、合同hash、九个JAR hash、九个镜像digest及九个savepoint URI/hash。
3. runtime snapshot 来自实际运行作业和部署对象，字段必须与release registry逐项相等；未知、缺失或任一字段漂移均阻断验收。

## 3. 候选冻结

先完成九个JAR构建、九个单作业镜像构建与推送，再创建只含digest的image manifest。禁止使用tag、`latest`或未绑定同一G0候选的镜像。

```bash
python3 scripts/alignment/verify_flink_application_artifacts.py
python3 scripts/alignment/verify_flink_job_registry.py
python3 scripts/alignment/build_flink_job_release_registry.py \
  --image-manifest /approved/flink-images.json \
  --savepoint-manifest /approved/savepoints.json \
  --g0-manifest doc/02_acceptance/runs/<g0-run>/manifest.json \
  --output doc/02_acceptance/runs/<release-run>/job-release-registry.json
```

生成器拒绝覆盖已有文件。image、artifact或savepoint集合必须与九个canonical job完全相等；savepoint必须位于该job的MinIO固定前缀。

## 4. 运行态快照与门禁

从Kubernetes Application Cluster、Flink REST和候选注解采集九条规范记录。至少包含：

- `job_id`、`job_name`、`artifact_sha256`、`image_digest`和`entry_class`；
- `parallelism`、`max_parallelism`和`operator_uid_sha256`；
- `state_backend`、`checkpoint_path`和`savepoint_path`；
- `source_start_mode`、`sink_guarantee`。

执行：

```bash
python3 scripts/alignment/verify_flink_job_registry.py \
  --release-registry /approved/job-release-registry.json \
  --runtime-snapshot /evidence/flink-runtime-registry.json
```

正式候选必须得到 `release_registry=PASS`、`runtime_diff=PASS` 和 `coverage_status=COMPLETE`。这只证明登记一致，不替代checkpoint、外部sink和业务结果对账。

## 5. Savepoint升级与扩缩容

每次只操作一个job：

1. 记录RUNNING、无root exception、最新checkpoint、source offset、watermark、sink水位和业务键集合。
2. stop-with-savepoint，保存URI、SHA-256、source job ID、operator UID hash、artifact和image digest。
3. 在隔离cluster用相同并行度恢复；禁止 `allowNonRestoredState`，比较状态、输出和业务键。
4. 在不改变`max_parallelism=128`的前提下，分别验证批准的缩容和扩容并记录key-group分配、checkpoint体积、alignment、backpressure与恢复时间。
5. 串行灰度新artifact；产生新checkpoint后按source offset和业务键对账。

如果UID、state serializer、max parallelism、savepoint hash、source起点或sink保证不兼容，立即停止。不得通过丢弃状态、改用latest或跳过失败operator继续发布。

## 6. 回滚与关闭

取消新Application Cluster但保留savepoint，使用已登记的旧artifact/image从兼容savepoint恢复；重新验证RUNNING task、checkpoint、watermark、lag、DLQ和sink业务键。T+0、T+1、T+3、T+7均无扩大差异后，方可提交独立验收。

当前仍需真实镜像、live registry、逐作业rescale、JM/TM/节点/MinIO/Kafka/sink/网络故障、性能和发布观察证据，因此本项保持 `IMPLEMENTING`。

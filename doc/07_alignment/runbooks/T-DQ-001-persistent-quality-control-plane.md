# T-DQ-001 持久化数据质量控制面运行手册

## 1. 当前结论

本候选完成 `PARTIAL` 的仓库侧 `flows_raw` 纵切：baseline 从进程内状态迁移为 PostgreSQL 版本化记录，并在同一事务写入最小 audit 和 outbox；Schema 签名覆盖字段名、类型和默认表达式；版本化数据集信号合同固定五类交接信号。Kafka lag 由 broker end offset 减 consumer-group committed offset 得出，Flink watermark 从指定 RUNNING job/vertex 的全部有限 `currentOutputWatermark` 取最小值，sink commit 从按 tenant 过滤的 ClickHouse `max(ingest_ts)` 得出。`flows_raw` 没有业务 aggregate revision 或对象载荷，后两类信号明确为 `not_applicable`，不是零值或通过。

候选同时增加 tenant-scoped 数据集目录和规则治理接口：数据集按 revision 创建、更新、暂停或退役；规则只能先创建 `draft`，再依次进入 `shadow`、`approval_pending`，最后由不同于创建者的审批人批准为 `active` 或拒绝。每条命令使用稳定幂等键和乐观 revision，在一个 serializable PostgreSQL 事务内同时提交业务状态、不可变历史、outbox、audit 和命令回执；仓库测试和临时 PostgreSQL 已证明精确重放、幂等冲突、非法转换、自批拒绝和跨租户隔离。

候选还实现默认关闭的 `flows_raw` 规则评估器：只查询 PostgreSQL 中已审批的 `active` 规则，字段和算子采用固定白名单，ClickHouse 查询强制按 tenant 和闭合时间窗限定；空窗口写为 `unknown`，不得算作通过。评估记录使用确定性 ID；失败时质量事件、评估记录、两条 outbox 和 audit 在同一 serializable 事务提交，精确重放不产生重复副作用。代码与部署均保持 `DATA_QUALITY_RULE_EVALUATION_ENABLED=false`，该证明不代表生产 migration、候选服务或真实 ClickHouse 评估已发布。

候选新增 `flow_replay_window_v1` 修复控制生命周期：只允许 `flows_raw`、认证 tenant、一小时以内时间窗、最多 100000 行和 300 秒预算；状态按 `planned → dry_run_passed → approval_pending → approved → executing → executed → reconciled` 推进，禁止从 `executing` 绕过真实执行结果直接对账。申请人不能批准或执行最终对账，执行与投影分别由服务端 `DATA_QUALITY_REPAIR_EXECUTION_ENABLED=false`、`DATA_QUALITY_REPAIR_PROJECTION_ENABLED=false` 默认关闭；显式开启执行时必须同时具备已迁移 PostgreSQL、ClickHouse、独立 Topic 元数据、consumer group 和健康 executor，否则服务启动失败。客户端 summary 仅为兼容字段，不参与 dry-run 或 reconcile 判定。服务端 dry-run provider 从 PostgreSQL 读取已持久化 scope/budget，再以 tenant 和 ingest 时间窗对 ClickHouse `traffic.flows_raw` 计算行数、distinct event ID、重复数、event ID hash 和水位；空范围或越过预算不通过。执行工作器把 PostgreSQL `status=executing` 行作为持久队列，并使用 session advisory lock 防止多副本同时执行；崩溃释放锁后可重试，服务端执行结果经 repair history、outbox、audit 和幂等回执原子记录为 `executed/failed`。回放驱动在执行前重新检查行数预算，只按 tenant/ingest 时间窗有界读取，去重稳定 event ID，保留原事件身份并添加 repair causation/correlation/idempotency envelope；它拒绝写回 `flow.events.v1`，只允许 `flow.projection-replay.v1`。消费者同时校验 Kafka topic/key/headers 与 Protobuf body，在同一 PostgreSQL 事务中写入不可变 `data_quality_flow_replay_projection` 目标对象和 `data_quality_replay_projection_receipts` 回执，事务提交后才允许 Kafka offset 提交。reconcile 独立比较 ClickHouse 源 event ID 集、PostgreSQL 目标集、回执集及 payload hash；任何缺失、额外或 hash 不一致均不得进入 `reconciled`。该闭环只证明 `flow-replay-pg-v1` 受控修复投影，不代表 Session、告警或其他后续投影已修复。只有执行受理才返回 `202`。代码和临时 PostgreSQL仍需完成本候选全量验证与部署验证，生产尚未应用，不能据此宣称专项关闭。

后台采集器将五类结果以 `measured/unknown/not_applicable/error` 四态原子持久化到 PostgreSQL，`GET /v1/data-quality` 只读取最近一次持久化结果，不在请求路径采集或写入。代码默认 `DATA_QUALITY_SIGNAL_COLLECTION_ENABLED=false`；候选部署只能在 migration 验证通过后启用。以上不代表 T-DQ-001、F-DATAQUALITY-001 或生产数据质量闭环已经关闭。

尚未完成的关闭条件包括：其余数据集目录和真实适用性合同、默认关闭评估器的候选部署与真实 ClickHouse 证据、服务端真实 dry-run、受限 replay executor、MinIO 大样本和报告、跨存储权威对账、性能、故障、Windows Chrome、灰度、回滚和观察期证据。

## 2. 不变量

- tenant 只能来自认证身份，不接受请求体覆盖。
- `measured` 必须同时携带非空 value 和 observed time；`unknown/not_applicable` 不得携带伪 value；`error` 必须携带采集错误且不得携带 value。
- 缺失、不可用或未接入的必需测量必须是 `unknown/partial`，采集故障必须是 `error/partial`，均不得显示为绿色或计入通过率。
- Kafka lag 只能由 broker end offset 减 consumer-group committed offset 得出；CH 插入速率只能作为辅助流量指标。
- baseline、最小 audit 和 outbox 必须在同一 PostgreSQL 事务提交；任一步失败全部回滚。
- 规则、baseline、事件和 repair 都按 tenant、数据集、稳定 ID 和版本管理。
- 只有 `active` 规则可进入 evaluator；字段、算子、tenant、数据集和时间窗都必须由服务端白名单与预算控制，禁止执行规则 JSON 中的任意 SQL。
- 空窗口必须持久化为 `unknown`；评估失败才创建稳定质量事件，精确重放不得重复写事件、outbox 或 audit。
- release blocking 默认关闭；repair 默认 dry-run；申请人与审批人必须分离。
- repair 执行和最终对账必须与申请人分离；执行开关只能来自服务端配置，禁止请求体启用。
- repair 执行成功不等于问题关闭，只有权威事实、offset、水位、投影、对象和审计对账完成后才允许关闭。
- 大样本和报告进入 MinIO；PG 仅保存 manifest、hash、size、状态、权限、保留和业务引用。

## 3. Expand 与静态验证

```bash
python3 scripts/alignment/verify_data_quality_control_plane.py
python3 -m unittest tests.alignment.test_data_quality_control_plane -v
cd go/control-plane && go test ./internal/common/kafka ./internal/common/dataquality ./internal/alert/config ./internal/alert/api ./cmd/alert-service -count=1
```

版本化 migration 为 `202608041400_data_quality_control_plane_v1.sql`、`202608041500_data_quality_governance_v1.sql`、`202608041600_data_quality_rule_evaluation_v1.sql`、`202608041700_data_quality_repair_lifecycle_v1.sql` 和 `202608041800_data_quality_replay_projection_v1.sql`（均位于 `deployments/postgres/migrations/`），数据集信号合同为 `contracts/data-quality/dataset-signals.v1.json`。首次只允许 expand：创建 dataset、rule、baseline、watermark、event、repair、outbox、不可变 history、幂等 command receipt、rule evaluation、repair history/receipt、受控投影对象和投影回执表，不删除旧表、字段或 API。三个 PostgreSQL 初始化入口必须先通过 `python3 scripts/alignment/sync_data_quality_postgres_entrypoints.py --check`，禁止手工维护生成镜像。

迁移和应用必须分两次发布，禁止把带 `DATA_QUALITY_SIGNAL_COLLECTION_ENABLED=true` 的应用与未验证的 migration 同批竞速启动：

1. 先更新 PostgreSQL init ConfigMap/Job 并执行 migration。
2. 只读核对 `alignment_schema_migrations.version IN ('202608041400','202608041500','202608041600','202608041700','202608041800')` 各恰好一条，十五张表存在，四态、治理状态和投影不可变约束有效。
3. 再发布候选 alert-service；保持 `DATA_QUALITY_RULE_EVALUATION_ENABLED=false`、`DATA_QUALITY_REPAIR_EXECUTION_ENABLED=false` 和 `DATA_QUALITY_RELEASE_BLOCKING_ENABLED=false`。
4. 核对 background loop 每个 active tenant 恰好持久化五类信号，随后验证 GET 只读且返回相同 collection。

共享环境不得直接重跑包含全部历史 SQL 的 `init-postgres-schema` Job。先用
`make alignment-render-data-quality-expand` 生成只包含五个 T-DQ-001 migration
的独立制品；该制品默认 `suspend: true`、`backoffLimit: 0`，并绑定候选 G0、
migration bundle、目标 PostgreSQL `system_identifier`、执行前 migration 状态、
独立审批人和最长四小时窗口。应用 YAML 只创建 immutable ConfigMap、immutable
approval Secret 和暂停 Job，不会执行 SQL；批准人复核制品 hash 后，才允许在
窗口内单独解除该 Job 的暂停状态。

治理接口保持 additive：`GET/PUT /v1/data-quality/datasets[/{dataset_id}]`、`GET/POST /v1/data-quality/rules`、`POST /v1/data-quality/rules/{rule_id}/transitions`、`POST /v1/data-quality/events/{quality_event_id}/repairs` 和 `POST /v1/data-quality/repairs/{repair_id}/transitions`。写请求必须携带 16—200 字符的 `Idempotency-Key`、`action_id`、适用时的 `expected_revision` 和 8—1000 字符的 reason；tenant、actor 和 trace 只能来自认证上下文。候选发布前必须运行 `python3 scripts/alignment/verify_data_quality_governance_ephemeral.py --run-id <immutable-run-id>`，同时通过 repository 和 HTTP 两层真实 PostgreSQL 生命周期测试。

若要撤回采集器而不回退 expand schema，将 `DATA_QUALITY_SIGNAL_COLLECTION_ENABLED=false` 后滚动 alert-service；保留已持久化的 measurement、audit 和 outbox 证据。

评估器只能在 migration、active 规则目录、ClickHouse 查询预算和内部 tenant 数据均验证后单独灰度。启用 `DATA_QUALITY_RULE_EVALUATION_ENABLED=true` 前先确认无未知字段/算子；撤回时恢复为 `false` 并滚动 alert-service，保留已提交的 evaluation、quality event、outbox 和 audit。评估循环错误不得自动切换 release-blocking。

## 4. 灰度顺序

1. 固定候选 source、镜像、配置、migration 和 OpenAPI hash。
2. 在隔离环境应用 expand migration，核对十五张表、约束、索引和 migration ledger。
3. 仅为内部 tenant 注册数据集并生成 shadow baseline；保持规则评估和 release blocking 均关闭。
4. 启用后台采集前确认 `flow.events.v1/flink-session-job` group、唯一的 `Session Aggregation Job V2`、唯一包含 `Assign FlowEvent Watermarks` 的 RUNNING vertex，以及 tenant-scoped `traffic.flows_raw` 均可只读访问；任何一项缺失都保持 `unknown/error`，不允许用代理值补齐。
5. 完成 shadow 观察和独立审批后，只为内部 tenant 启用 evaluator；注入空窗口、缺字段、类型漂移、重复、乱序、迟到、积压、水位停滞、投影缺失和对象悬挂引用，核对 pass/fail/unknown、稳定 ID、事件、outbox、audit 和精确重放。
6. 先由服务端读取权威范围并生成 repair dry-run 证据，由不同人员批准，再显式启用 executor 并在隔离资源配额下执行；每个步骤保存稳定 action/repair ID、revision、reason、trace 和审计。客户端自报 summary 不得作为生产 dry-run 通过证据。
7. 对权威事实、投影、对象、offset 和审计做差异报告；只有全部对账且 SLO 恢复后进入观察期。

## 5. 停止条件

出现跨租户、baseline/outbox/audit 不原子、offset 或 watermark 倒退、对象 manifest 不一致、重放范围扩大、重复副作用、Kafka/Flink 不可恢复、PG 锁/复制延迟越界、在线 P99 或存储资源越界时，立即停止扩大。未测量或采集器故障也必须停止 release-blocking 判定，不能降级为全绿。

## 6. 回滚

1. 将 `DATA_QUALITY_RULE_EVALUATION_ENABLED=false`、`DATA_QUALITY_REPAIR_EXECUTION_ENABLED=false` 和 release-blocking flag 关闭，停止接收新的非 dry-run repair。
2. 回滚 alert-service 和相关 evaluator/executor 到前一不可变镜像；保留旧兼容查询路由。
3. 保留 expand 表、已写 baseline、事件、outbox、audit 和对象 manifest，不执行 `DROP` 或批量删除。
4. 对回滚前已受理任务按 repair ID 和 revision 逐项对账；能安全补偿的走独立批准，不能确认的保持 `partial/failed`。
5. 重新核对权威事实、Kafka offset、Flink watermark、sink commit、投影和对象，生成带 hash 的回滚证据包。

## 7. 关闭证据

T-DQ-001 关闭至少需要：全部规则维度和状态机、服务重启不丢 baseline/事件、真实五类水位、MinIO 样本 manifest、申请/审批分离、重复/乱序/DLQ/部分失败注入、修复前后跨存储对账、固定规模 P50/P95/P99 与资源、指定 Windows Chrome mock-off、发布/回滚以及 T+0/T+1/T+3/T+7 观察。仓库测试、HTTP 2xx 或单张截图不能单独作为关闭依据。

2026-08-04 的不可变只读 pre-rollout 证据显示：生产 migration 和候选镜像仍未应用；Kafka、Flink 和 ClickHouse 的实时值只能以对应 run 的 manifest 为准。前一轮发现的 session 恢复链路问题已有独立不可变恢复证据；仓库和临时 PostgreSQL 已证明 governance、默认关闭 evaluator 及修复控制状态机的事务语义，但生产 Schema、候选 API/评估器、真实 ClickHouse 评估、服务端 dry-run、replay executor 和跨存储权威对账仍是明确的 G2/G3 阻断项，不得改写为生产 PASS。

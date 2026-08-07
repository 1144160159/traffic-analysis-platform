# 前后端逻辑对齐整改控制面

本目录把《前后端逻辑对齐整改报告》中的 24 周计划转换为可执行、可审计的仓库合同。

## 真源与生成物

- `canonical-registry.json`：102 个 canonical ID 的唯一清单，不允许删除或改名。
- `work-packages.json`：WP-00～WP-28 的唯一 Accountable、范围模式、验收项和回滚项。
- `features/*.json`：版本化 Feature Contract。
- `progress-overrides.json`：仅记录非默认状态及其证据；未覆盖项一律为 `OPEN`。
- `remediation-ledger.json`：由前三项生成的 102 项任务台账，禁止手工编辑。
- `runtime-ddl-baseline.json`：历史运行时 DDL 债务基线，只允许减少，不允许新增。
- `../events/kafka-topic-catalog.v1.json`：24 个 canonical Kafka topic 到 key、权威
  Protobuf/JSON Schema、producer、consumer 和已知缺口的机器可校验矩阵。
- `../events/kafka-json-events-v1.schema.json`：仍使用 JSON wire format 的 Kafka
  事件定义；Proto wire format 继续以 `proto/traffic/v1` 为真源。
- `../openapi/alignment-v1.openapi.json`：整改 REST 接口真源。

## 状态语义

`OPEN → IMPLEMENTING → VERIFYING → OBSERVING → CLOSED` 是正常路径；存在外部依赖时可进入
`BLOCKED`。代码合并、HTTP 2xx、单张截图或单个单测均不能直接把条目置为 `CLOSED`。

本期未实施的 P1/P2 必须保持 `OPEN/BLOCKED`。G7 与 G8 独立：G7 只表示整改专项关闭，
100G/Mpps、检测质量、HA、现场及第三方验收未完成时，G8 必须保持 `BLOCKED`。

## 本地门禁

```bash
make alignment-test
python scripts/alignment/validate.py --strict-w1
```

生成 W0 不可变证据包：

```bash
make alignment-baseline \
  SOURCE_WORKTREE=/path/to/current/dirty/worktree \
  RUN_ID=YYYYMMDD-remediation-w0
```

同一 `RUN_ID` 不可覆盖。`doc/02_acceptance/runs/` 是本地不可变运行证据目录，不提交大体积
运行产物；正式归档系统应以 manifest SHA-256 为索引保存。

仓库全量基线由三条独立命令组成，不能互相替代：

```bash
tests/run_tests.sh full
make python-test
ROUNDS=100 LOG_DIR=/tmp/<run_id> tests/run_tests.sh live
```

`full` 先生成 Proto，再运行各语言消费者。`live` 会写入真实 API/数据库，只能在已批准的
候选环境执行。

把 G0 三条只读/本地门禁的完整输出固化为不可覆盖、带 hash 的证据包：

```bash
make alignment-g0-evidence \
  RUN_ID=YYYYMMDD-remediation-g0 \
  INVENTORY_MANIFEST=doc/02_acceptance/runs/<inventory-run>/manifest.json
```

功能级增量 G0 可额外传入 `G0_PROFILE=probe-publisher`；其 manifest 会明确列出包含与排除项，
不能冒充全仓 full suite。

当前已归档 G0 证据由 `evidence-index.json` 指向具体 run 和 manifest SHA-256；该指针只说明
对应候选快照的 G0 结果，不替代 100 轮 live、Windows Chrome、G2—G6 或 G8。

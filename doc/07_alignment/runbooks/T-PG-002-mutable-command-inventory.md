# T-PG-002 PostgreSQL 可变命令清单与审查队列

## 目的与证据边界

本清单冻结 Go control-plane 非测试源码中的 SQL 变更语句，并把 PostgreSQL 业务状态、审计、历史、outbox、投影/inbox、幂等控制与 ClickHouse 写入分开。它用于发现和排序 T-PG-002 缺口，不把字符串扫描、同文件 token 或测试通过写成事务原子性证明。

权威快照为 `contracts/postgres/mutable-command-inventory.v1.json`，生成器为 `scripts/alignment/inventory_pg_mutations.py`。快照不含采集时间，按路径、源码行和语句顺序稳定生成；新增、删除或移动写入都会使 `--check` 失败。

## 扫描与分类

扫描范围仅包括：

- `go/control-plane/internal/**/*.go`；
- `go/control-plane/cmd/**/*.go`；
- 排除 `*_test.go`、注释和不构成 SQL 的普通错误文案。

扫描器只读取 Go 字符串字面量中的 `INSERT INTO`、`UPDATE ... SET` 和 `DELETE FROM`。`traffic.*` 归属 ClickHouse，`public.*` 和不带 schema 的表归属 PostgreSQL；未知 schema 或动态 `%s` 表必须有代码级白名单和显式 backend override，否则拒绝生成快照。

每个 PostgreSQL 写入按表名记录以下角色之一：`business`、`audit`、`history`、`outbox`、`inbox_projection`、`control_idempotency`。无法从表名判定时，只允许添加逐表、带 `review_basis` 的窄角色 override；`graph_hot_ips` 已确认是由查询频次派生、仅用于选择预热目标且可重建的缓存投影，不作为权威业务命令进入 P0 队列。同文件中的 `BeginTx`、`tx.Exec/Query`、audit、history、outbox 和 idempotency token 只记为 `source_facts`，必须继续逐函数确认事务边界。

## 当前基线

当前静态基线包含 88 个源文件和 526 条 SQL 变更语句，其中 PostgreSQL 510 条、ClickHouse 16 条、未分类 0 条。PostgreSQL 角色分布为：business 262、audit 65、history 45、outbox 91、inbox/projection 32、control/idempotency 15。审查队列为 29 个源文件：P0_REVIEW 7、P1_REVIEW 16、P2_REVIEW 6。

审查队列不是缺陷计数。`P0_REVIEW` 表示用户可触发的业务写入缺少足够的同文件事务证据，应优先进入人工命令边界审计；投影、dispatcher、后台 worker 使用较低审查级别，但不能因此豁免其幂等与恢复要求。

## 通知规则纵切裁决

盘点后复核确认：通知规则、模板、升级策略、静默规则和全局设置此前均存在业务状态与 actor/reason/trace 审计、history、outbox、幂等请求登记不在同一事务的问题；全局设置在没有仓库时还会返回伪成功。当前纵切已把这些配置命令统一纳入同一事务。

通知规则、通知模板、升级策略、静默规则和全局设置已形成连续纵切：

1. Web UI 提交稳定 `action_id`/`Idempotency-Key`、原因和资源 revision；
2. serializable 事务统一提交对应业务表、rich audit、history、outbox 和请求登记；
3. 相同 key 同 payload 重放已提交结果，不同 payload 或陈旧 revision 返回 409；
4. outbox 具备租约、退避、dead 和 Kafka ACK 后标记，专用 topic/ACL/部署配置已登记；
5. migration 已同步 common SQL、Kubernetes init、Docker merged 和版本化 migration，并在临时 PostgreSQL 16 中双重回放，18 表253列摘要一致；
6. 专用 topic 仍为 `producer_only`，真实 Kafka 发布、consumer、故障注入和 PG-Kafka-audit 对账仍为开放门禁。

用户偏好命令已进一步完成同级事务化并使原 `user.events.v1` consumer-only 缺口转为 active；`graph_hot_ips` 经源码审查被记录为可重建缓存投影，不再作为业务命令误报。其余 P0 队列仍需逐命令边界审查。

## 验证命令

```bash
python3 scripts/alignment/inventory_pg_mutations.py --check
python3 -m unittest tests.alignment.test_pg_mutation_inventory -v
python3 scripts/alignment/verify_pg_transaction_outbox.py
make alignment-verify-pg-mutation-inventory
```

只有批准新增或重分类 SQL 写入时才运行 `--write-snapshot`，且必须评审快照 diff。禁止为了让门禁通过而给未知动态表添加宽泛 override。

## 未覆盖门禁

静态清单不证明运行时路径可达、事务提交/回滚、锁行为、触发器实际安装、Kafka 发布、consumer 幂等、跨存储对账、性能、浏览器、发布或 G8。T-PG-002 继续保持 `IMPLEMENTING`。

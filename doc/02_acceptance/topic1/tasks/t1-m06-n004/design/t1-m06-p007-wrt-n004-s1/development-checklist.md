# T1-M06-P007-WRT-n004-s1 开发领取清单（设计候选）

> 当前状态：`DRAFT / NOT_CLAIMABLE / BLOCKED_UNTIL_SIGNED_OVERLAY`。本清单用于把缺口变成可核查动作，不构成执行授权。

## 1. 领取前硬门

- [ ] `T1-M06-P031-IDX-n012-task-completion` 为同candidate的可信`PASS`，不是非空receipt ID。
- [ ] owner=`multi-source-data-owner`、Go language owner、安全/tenant reviewer、QA/SRE reviewer和approver均绑定真实身份。
- [ ] clean implementation candidate、candidate manifest、profile、environment与signed overlay同一哈希链。
- [x] writable primary精确为`repository.(*AssetRepository).UpsertAtomic`，不是`assetUpsertIdentity`结构体。
- [x] primary、直接caller、5个仓内callee均有candidate-bound Go AST receipt。
- [ ] resolver正式bootstrap、受保护builder/hash/签署及receipt.calls exact-set比较完成；当前回执仅为候选级。
- [ ] 领域owner批准“action class × 字段 × observed_at”来源裁决矩阵。
- [ ] 模式proposal完成：默认选择`DIRECT + PROJECT-TRANSACTIONAL-OUTBOX`；GoF Command保持拒绝，除非出现真实Invoker与多个可替换实现。
- [ ] 16个body step、5个effect、error exact-set、test-oracle exact-set经函数评审无P0/veto。

任一项不满足立即停止，不得编辑生产代码。

## 2. 本叶唯一写面

- [ ] 生产代码仅修改`go/control-plane/internal/asset/repository/atomic_upsert.go`中授权AST node。
- [ ] 当前设计不修改HTTP/gRPC、config loader、dispatcher、migration或投影代码。
- [ ] `atomic_upsert_test.go`与`atomic_upsert_integration_test.go`如需新增测试，必须由独立REF/TST叶授权，不能借P007越界。
- [ ] 不把`hash/decide`实质逻辑伪装为“compile-only companion”；需要抽函数时新建明确叶或修订scope。

## 3. 函数内部实现顺序

- [ ] B01复制输入并冻结调用前置；不在repository虚构tenant/auth校验。
- [ ] B02保留v1 canonical bytes/hash，提交golden bytes与SHA-256测试。
- [ ] B03–B07按“BeginTx → idem lock → ledger → asset lock → authority read”固定锁序。
- [ ] B08按签署来源矩阵计算create/update/replay/conflict与next state。
- [ ] B09执行INSERT或revision CAS；零行稳定映射revision conflict。
- [ ] B10–B14按authority → history → audit → pending outbox → ledger顺序构造完整事务。
- [ ] B15增加typed commit-unknown语义；调用方只能用同tenant+同key恢复。
- [ ] B16区分新提交成功与exact replay，不宣称Kafka/投影完成。
- [ ] 在当前P007按DELTA-JSON-MAP-MARSHAL、DELTA-OLD-HISTORY-MARSHAL、DELTA-AUDIT-MARSHAL修复`jsonObject`、`oldJSON,_`、`auditDetail,_`静默降级；三项都必须由命名故障测试证明回滚。

## 4. 测试与故障矩阵

- [ ] 单元测试命令无`SKIP`且覆盖exact replay/create/history rollback。
- [ ] PostgreSQL sentinel为`ASSET_ATOMIC_EPHEMERAL_PG_DSN`；缺失时证据为`BLOCKED`，不是`PASS`。
- [ ] tenant body conflict与viewer在首次DB调用前拒绝。
- [ ] same-key/same-payload无重复五类effect；same-key/different-payload稳定冲突。
- [ ] stale revision与并发CAS不覆盖新版本。
- [ ] B09/B11/B12/B13/B14之后、B15之前逐点故障均验证五表零残留。
- [ ] B15 unknown和B15成功后response loss均以原key收敛到exactly-one。
- [ ] broker无ACK保持pending属于独立真Kafka叶；本P007不得凭sqlmock宣称。

## 5. 回滚、观察与证据

- [ ] commit前回滚不留子集；commit后只forward reconcile，不删authority/history/audit/outbox/ledger。
- [ ] 观测窗口`T+0/T+1h/T+24h/T+72h/T+7d`及stop threshold绑定同candidate/profile/environment。
- [ ] 指标禁止tenant、asset、MAC、idempotency key等高基数或敏感label。
- [ ] test/evidence/rollback/observation制品均通过统一plan schema与typed hash校验。
- [ ] G2/G3及真实PG/Kafka证据输出为不可变result/case-report，失败原件保留。
- [ ] 终端`T1-M06-P008-IDX-n004-task-completion`只聚合已批准新叶、依赖、rollback与evidence exact-set。

## 6. 禁止声明

- [ ] 不称`FUNCTION_DESIGN_COMPLETE`，直至末端function review receipt可信统一。
- [ ] 不称`READY_BINDING/EXECUTION_AUTHORIZED`，直至既有overlay/package validator通过。
- [ ] 不称`IMPLEMENTATION_COMPLETE/PRODUCTION_ACCEPTED/DEPLOYMENT_ACCEPTED`。
- [ ] 不用设计文档、schema PASS、单测exit 0或SKIP替代运行验收。

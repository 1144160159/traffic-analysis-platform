# T-OS-003 OpenSearch 深分页与 PIT 一致性查询手册

## 1. 当前结论

本手册对应 `T-OS-003`、`F-SEARCH-001` 与 `T1-M09-N015`。仓库候选为 `IMPLEMENTING/PARTIAL`，`production_applied=false`；它不能证明生产已启用、生产规模性能达标、浏览器验收完成或专项关闭。

2026-08-16 的 K8s run `4d99d81c-619f-4885-97c5-ece81f61da94` 已在现有 `middleware/opensearch` 上用两个 run-scoped 索引和一个 run-scoped alias 证明 live alias 漂移失败关闭、PIT alias 切换保持冻结快照、tenant 隔离、目标水位和 OpenSearch 不可用失败关闭；cleanup Job 随后证明这些索引和 alias 均已删除。该证据只绑定测试镜像与不可变 Web bundle，不是生产 alias、72M 数据集、APISIX 或 Windows Chrome 证据。

2026-08-04 只读盘点显示，生产 `alerts` 索引约有 72,065,235 个文档，采样时活动 PIT 为 0，`alert-service` Deployment 中尚无 `OPENSEARCH_SEARCH_CURSOR_V1_ENABLED`。现有正式 API 使用 `from/size`，因此大页深度会放大协调节点堆、CPU、分片取回和网络成本。

候选保留 `POST /v1/alerts/search` 及全部既有请求字段，并增加 `cursor`、`cursor_mode=live|pit` 和 `DELETE /v1/alerts/search/cursor`。运行时开关默认 `false`；未获批准不得部署、启用或创建生产 PIT。

## 2. 合同与预算

| 项目 | 值 |
|---|---|
| 旧兼容分页 | `from/size`，启用新合同时仅允许前 1000 个结果 |
| live 深分页 | `search_after` |
| 一致遍历／导出 | PIT + `search_after` |
| 稳定排序 | 业务排序字段 + `alert_id` tie-breaker |
| cursor 页大小 | 1—200，默认 50 |
| cursor 最大长度 | 8192 bytes |
| PIT/cursor TTL | 2 分钟，每页续租 |
| OpenSearch query timeout | 2 秒 |
| total 策略 | cursor 模式跟踪到 10,000，返回 `eq` 或 `gte` |
| partial 策略 | `allow_partial_search_results=false`，`timed_out=true` 或 failed shard 直接失败 |
| 字段策略 | 固定 `_source` 和 sort allowlist，不暴露 wildcard/regexp query 类型 |
| 权限 | `alert:read`；tenant 仅来自认证身份并进入 filter context |
| 后端运行时开关 | `OPENSEARCH_SEARCH_CURSOR_V1_ENABLED=false` |
| 前端运行时开关 | `ALERT_SEARCH_CURSOR_V1_ENABLED=false` |

游标使用从 Secret 注入的 `JWT_SECRET_KEY` 作为根密钥，但以 `traffic.alert.search.cursor.v1` 域分离后再做 HMAC-SHA256。签名声明绑定 tenant、规范化查询指纹、解析后的物理索引集合 SHA、模式、页大小、排序值、PIT ID、快照时间和到期时间。跨租户、篡改、过期及变更筛选／排序／页大小／模式的游标在访问 OpenSearch 前拒绝。live 续页会重新解析 alias；物理集合变化即返回 stale cursor。PIT 首次创建前解析并冻结物理索引，续页只使用 PIT，因此批准的 alias 切换不会把 PIT 偷换到新索引。

## 3. API 旅程

### 3.1 受控浅页兼容

不传 `cursor_mode` 时继续接受原有 `from` 和 `size`。启用候选后 `from + size` 不得超过 1000；更深页面必须改为 cursor。旧模式继续返回精确 total 与既有聚合，避免一次切换同时改变所有页面语义。

### 3.2 live 交互分页

首页传 `cursor_mode=live`、固定筛选、排序和 `size`。服务用 `size+1` 判断 `has_more`，只返回 `size` 条，并以最后一条实际返回记录的排序值签发 `next_cursor`。后续请求携原查询和 cursor；不得同时携非零 `from`。live 模式不保持服务端上下文，新增文档可能改变后续可见集合，因此页面必须标识它不是一致导出快照。

### 3.3 PIT 一致遍历

首页传 `cursor_mode=pit`。服务在已批准的 read alias 上创建 PIT，搜索请求不再携 index，而携 PIT ID、keep_alive 与 `search_after`。OpenSearch 可能轮换 PIT ID；服务把最新 ID 签入下一个 cursor。末页、显式关闭或首个查询失败时释放 PIT；关闭失败只允许依赖短 TTL 兜底并必须告警。

客户端完成、取消或离开一致遍历时调用 `DELETE /v1/alerts/search/cursor`，body 为 `{"cursor":"..."}`。live cursor 是幂等 no-op；PIT cursor 关闭所属 tenant 的上下文。客户端永远不能直接提交或替换 PIT ID。

告警页面在前端开关开启时只使用 PIT 顺序分页，缓存已经访问的页，禁止跳到未签发游标的页。重查、改变过滤器、改变页大小、刷新或离开页面时显式关闭当前 PIT 并从第一页建立新快照。响应必须同时带 `snapshot_id`、`as_of` 和 `opensearch.alerts.target_sha256`；缺少任一字段即显示失败，不回退 `/v1/alerts`、fixture 或前端拼装数据。

## 4. G2/G3 真实验证

仓库单元测试和静态验证仅属于 G0/G1。进入真实服务验证前必须满足：

1. `T-OS-002` 的 versioned read alias 已完成 shadow/reindex、权限与身份对账，并获准作为 canary read target；不得把 72M 单索引直接当作完成态。
2. 固定内部 tenant、查询窗、冷热缓存、并发、页深和正确性 oracle；保存候选镜像 digest、配置 hash、索引版本和 run_id。
3. 从同一固定查询取浅页基线、live cursor 和 PIT cursor，记录每页 `alert_id`、主排序值、总 relation、重复、遗漏和顺序违例。
4. 用权威 tenant×时间范围的 ID 集合校验权限不过滤遗漏；跨租户 cursor、变更筛选、变更排序、变更 size、过期、篡改均必须在 OpenSearch 前失败。
5. 注入分片失败、节点超时、PIT 丢失和客户端重复提交；任何 partial 空结果不得被解释为“没有告警”。
6. 保存 OpenSearch profile、task、search context/PIT inventory、slow log、JVM/CPU/heap/rejection、服务 trace 和 API 响应；不得保存凭证、完整敏感 `_source` 或原始签名密钥。

一致性 oracle 必须覆盖 PIT 遍历期间并发新增／更新：PIT 全部页面的 ID 集合和排序保持创建时快照；live 模式允许新数据影响可见边界，但同一稳定排序不能重复已返回 ID。若 tie-breaker 字段在目标 mapping 中不是可排序 keyword，必须停止，不能改回 `_id` 或无稳定排序后继续。

## 5. G4 性能与资源门禁

先批准而后测量具体目标，禁止用本手册中的 2 秒查询超时冒充 P95/P99 SLO。至少比较：

- 浅页 `from/size` 深度 0、100、500、1000；
- live cursor 第 1、10、100、1000 页；
- PIT cursor 同样页深及 PIT 建立／续租／关闭成本；
- 无全文词、全文词、多个精确 filter、冷缓存和热缓存；
- P50/P95/P99、错误率、吞吐、协调节点 heap/CPU、fetch bytes、rejection 和活动 PIT 数。

出现跨租户、页边界遗漏／重复、持续 timeout/failed shard、活动 PIT 不回落、P99 或资源越界时立即停止扩大。不得为了让测试通过而提高 `max_result_window`、无限延长 keep_alive、提高 bool clause 或放开 `_source`。

## 6. 灰度、回滚和观察

只有 G0—G4 和安全评审通过后，才可构建候选镜像并在内部 tenant canary 显式设置 `OPENSEARCH_SEARCH_CURSOR_V1_ENABLED=true`。前端仍需保留旧浅页兼容路径；先验证 API/脚本消费者，再绑定生产候选 bundle 做指定 Windows Chrome 旅程。

回滚条件包括合同／bundle 错配、数据或权限差异、P99／资源超限、PIT 泄漏、OpenSearch 不可恢复错误。回滚时先把开关恢复为 false 并回滚镜像／配置，停止新 PIT，再等待或显式关闭已有 PIT，确认旧浅页 API、告警页面和审计可用。该过程不删除索引、不切 alias、不终止与本次候选无关的 search context。

按 T+0、T+1、T+3、T+7 保存流量比例、错误、PIT 数、资源、页边界对账、回滚可用性和审批。低频一致导出至少经历一个完整业务周期。只有真实服务、数据、性能、浏览器、故障、发布、回滚和观察证据由独立 QA/SRE/安全裁决通过后，`T-OS-003` 才能进入 `VERIFYING/OBSERVING` 并最终关闭；这不替代 `T-OS-002/004/005` 或 G8 项目级门禁。

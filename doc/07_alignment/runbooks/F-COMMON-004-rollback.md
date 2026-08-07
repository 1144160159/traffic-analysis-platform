# F-COMMON-004 错误、审计、trace 与业务结果协议回滚

## 范围

回滚对象是新增结构化错误字段和 handler 采用路径。旧 `code/message/details` 保持可读；不得回滚为 HTTP 200 伪成功、不得丢弃 trace，也不删除审计记录。

## 停止条件

- 网关与上游 HTTP 状态或 `error.code` 不一致。
- 错误响应泄漏堆栈、凭证、跨租户 ID 或内部地址。
- `accepted/running` 被展示为最终成功。
- HTTP、任务、权威数据或审计的 code/result 出现扩大性分歧。

## 回滚步骤

1. 停止扩大 `common_error_result_v1`，冻结候选与错误分布证据。
2. 回滚新采用 handler 到上一个已验证响应写入器，但保留非 2xx 失败语义和 trace。
3. 恢复上一组 hash 绑定的 OpenAPI 与生成 TypeScript 类型。
4. 不回滚业务事实、审计、outbox 或已终止任务；在通用协议外保留查询能力。
5. 对参数错误、权限、409、429、503、504 和异步最终失败复验。

## 回滚验收

同一 trace 下 HTTP、任务、权威数据和审计结果一致，失败不返回 2xx，无敏感信息泄漏，无外部契约删除。

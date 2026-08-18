# P902 `HTTPHandler.upsertAsset`函数设计

状态：`DESIGN_CANDIDATE / BLOCKED_UNTIL_SIGNED_OVERLAY`。before locator由`locator-resolver-receipt.json`绑定；after签名不变：

```go
func (h *HTTPHandler) upsertAsset(w http.ResponseWriter, r *http.Request)
```

唯一变化轴是“权威事务结果未知如何安全映射到HTTP”，选择`DIRECT`，不新增Command/Strategy/State对象。事务outbox只作为下游约束，不由本函数实现。

候选调用集合已由同目录8份Go AST receipt绑定：直接caller为`api.(*HTTPHandler).ServeHTTP`（仅在`POST /api/v1/assets`分支调用）；仓内直接callee为`api.(*HTTPHandler).requireAssetDiscoveryWrite`、`service.(*AssetService).UpsertAssetAtomic`、`api.writeAssetCommandError`、`api.auditActor`、`api.clientIP`和`api.writeJSON`。`json.Decoder`、`strings.TrimSpace`、`errors.Is`、`strconv.FormatInt`、`time.Now`、`uuid.NewString`以及zap为标准库/批准外部依赖，不伪装成仓内locator。after-state若新增或删除仓内调用，必须重生成primary/caller/callee receipt并按exact-set重新评审。

| Step | guard / reads | writes / invokes | postcondition / error | 独立测试 |
|---|---|---|---|---|
| H01 | `POST`且`requireAssetDiscoveryWrite`通过 | 读取JWT identity、trace/request/key | 未授权/错scope在service/DB前返回 | 既有viewer负例 |
| H02 | H01通过 | decode body；校验body tenant；绑定可信tenant | 语法错误400，tenant冲突409，不泄漏内部cause | 既有tenant负例 |
| H03 | 输入有效 | 构造`AssetUpsertCommand`并调用`svc.UpsertAssetAtomic` | 原tenant/key/payload identity传入权威边界 | P909请求捕获 |
| H04 | `ErrAssetRevisionConflict` | `writeAssetCommandError(409,"revision_conflict",safe)` | revision冲突语义不回归 | P909 table case |
| H05 | `ErrAssetIdempotencyConflict` | `writeAssetCommandError(409,"idempotency_conflict",safe)` | same-key/different-payload不可盲重试 | P909 table case |
| H06 | `errors.Is(err, ErrAssetCommitUnknown)` | 固定HTTP `503`；`error.code=asset_upsert_outcome_unknown`、`error.message=asset upsert outcome is unknown; retry with the client-held original Idempotency-Key`、`error.retryable=true`、`error.retry_after_ms=1000`；`meta`只回显`trace_id/request_id`，禁止回显key | unknown不宣称failed/succeeded；调用方保留并复用原key；不得生成新operation | `TestAtomicAssetUpsertCommitUnknownReturnsSafePending` |
| H07 | 其他error | 内部结构化日志记录cause；响应固定`error.code=asset_upsert_failed`、`error.message=asset upsert request failed`、`retryable=false` | SQL/secret/payload字节不进body | P909 no-leak case |
| H08 | result已知成功 | 返回现有data/meta envelope | 只表示PG权威成功，不表示Kafka published/projected | 既有成功兼容用例 |

P909必须使用真实`HTTPHandler→AssetService→AssetRepository`加`sqlmock`，在`Commit`注入含SQL/secret marker的cause；P910通过`go test -json`逐事件断言该精确函数恰好出现一次run和一次pass，零命中、SKIP、FAIL或重复终局均拒绝。回滚仅恢复旧HTTP映射；可能已提交的PG事实不得删除。

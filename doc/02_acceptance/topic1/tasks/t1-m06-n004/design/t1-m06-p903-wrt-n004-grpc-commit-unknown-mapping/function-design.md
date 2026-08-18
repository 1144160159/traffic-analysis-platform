# P903 `AssetHandler.UpsertAsset`函数设计

状态：`DESIGN_CANDIDATE / BLOCKED_UNTIL_SIGNED_OVERLAY`。before locator由`locator-resolver-receipt.json`绑定；after签名不变：

```go
func (h *AssetHandler) UpsertAsset(ctx context.Context, req *pb.UpsertAssetRequest) (*pb.UpsertAssetResponse, error)
```

选择`DIRECT`；本函数是传输Adapter边界，但不为“使用Adapter模式”新增抽象。唯一变化是typed error到gRPC status的安全映射。

候选调用集合已由同目录6份Go AST receipt绑定：直接caller为生成的`trafficv1._AssetService_UpsertAsset_Handler`；仓内直接callee为`api.(*AssetHandler).assetUpsertCommandFromGRPC`、`api.protoToRecord`、`service.(*AssetService).UpsertAssetAtomic`和`api.(*AssetHandler).logError`。`req.GetAsset`、`status.Error`、`errors.Is`与zap属于生成接口/批准库调用。handler caller是只读生成物，P903不得修改；Proto字段或service descriptor变化必须另开CTR/生成物PR。

| Step | guard / reads | writes / invokes | postcondition / error | 独立测试 |
|---|---|---|---|---|
| G01 | request进入 | `req.GetAsset`，校验MAC | nil/MAC缺失=`InvalidArgument` | 既有nil/MAC用例 |
| G02 | G01通过 | `assetUpsertCommandFromGRPC`读取认证metadata、key、trace、request、revision | 未认证/无scope/缺key在DB前拒绝 | 既有metadata用例 |
| G03 | command有效 | proto→record，body tenant与可信tenant比较 | 冲突=`PermissionDenied`；source默认兼容 | P911兼容表 |
| G04 | 输入完整 | 调用`svc.UpsertAssetAtomic` | 只接收typed result/error | P911 sqlmock路径 |
| G05 | revision/idempotency冲突 | 保持`Aborted`/`AlreadyExists` | 旧客户端语义不回归 | P911 table case |
| G06 | typed commit unknown | 固定`codes.Unavailable`与`asset upsert outcome is unknown; retry with the same idempotency key` | unknown可用原key恢复；message无cause；本PR不新增RetryInfo或metadata协议 | `TestAssetHandlerCommitUnknownReturnsUnavailableSafeMessage` |
| G07 | 其他error | 内部log保留cause；客户端固定`codes.Internal`与`asset upsert request failed` | 不使用`err.Error()`构造status | P911 no-leak case |
| G08 | known success | 保持`AssetId/Created`响应 | 不额外承诺Kafka/投影 | 成功兼容用例 |

P911在`sqlmock.Commit`注入含SQL/secret marker的cause并验证`status.Code`及message白名单；P912通过`go test -json`断言planned test恰好一次run+pass，零命中、SKIP、FAIL或重复终局均拒绝。回滚不得改变Proto字段或已提交PG事实。

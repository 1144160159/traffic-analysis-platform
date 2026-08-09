# F-TOPIC-001 专题统一快照回滚

适用范围：`topic_snapshot_v1` 的三类专题统一快照、manifest 和 Web UI 主读取路径。

## 触发条件

- 同一页面的主视图、分析面板和右侧摘要出现不同 `snapshot_id` 或 `as_of`；
- manifest 的 payload SHA-256 与返回 payload 不一致；
- 跨租户读取、缺失投影未标记 `partial`，或 mock 关闭后仍返回 simulation；
- ClickHouse/PG 查询持续超预算，或候选 bundle 与契约版本不匹配。

## 停止扩大

1. 停止扩大候选 tenant，保留已采集的 HAR、trace 和 manifest。
2. 将 Web UI 主接口恢复为兼容的 `/v1/topics/tunnel|exfil|apt`。
3. 将 `TOPIC_SNAPSHOT_V1_ENABLED=false` 并发布配置回滚。
4. 不删除 `topic_snapshot_manifests`；它是审计和差异定位证据。

## 恢复与验证

重新开启前验证同一快照覆盖三面板与右栏、PG tenant 隔离、payload hash、各来源水位和
partial 语义，并在 Windows Chrome mock-off 环境保存截图、HAR、console、trace 与源事实。
回滚只切读取路径和功能开关，不回退 migration。

# F-ASSET-005 资产详情统一快照回滚

## 适用范围

用于资产统一详情快照出现跨租户、不同 section 水位错配、伪造零值、无界历史/拓扑读取、错误深链或性能越界时停止扩大。该查询功能不写业务状态，回滚不得删除资产、历史、拓扑、告警或证据。

## 立即停止条件

- 任意跨租户资产、历史、拓扑、告警或证据泄露。
- `partial=true` 或 `missing_sections` 与实际不可用来源不一致。
- 浏览器展示了服务端未返回的 IP、关系、告警、证据或完成状态。
- repeatable-read 事务、历史或拓扑读取超出批准的时长、行数或资源预算。
- 生产 bundle 与候选 manifest 不一致。

## 回滚步骤

1. 将 `ASSET_DETAIL_SNAPSHOT_V1_ENABLED=false`，停止新的统一快照流量。
2. 前端 bundle 回退到上一批准候选；旧 `/assets/{id}`、`details`、`history`、`topology` 兼容读接口保持可用，但必须继续显示真实空态。
3. 不回滚、不删除任何 PG/CH/Nebula/MinIO 事实；保存问题 trace、snapshot、tenant、asset revision 和所有 source watermark。
4. 对抽样资产逐项核对 PG 权威记录、历史、拓扑、CH 观测、告警、对象 manifest 和图投影，记录缺失来源而不是填零。
5. 记录 feature flag、候选 hash、在途请求、缓存失效、回滚验证和批准人。

## 恢复条件

- 跨租户、缺失 section、零值、硬编码 IP、伪拓扑和深链权限回归全部通过。
- 同一 snapshot 的各来源水位可解释，对账差异为零或均有批准的 partial 处置。
- G0—G3 重新通过；扩大流量前重新执行 G4—G6。

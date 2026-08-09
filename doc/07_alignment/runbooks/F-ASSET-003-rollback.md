# F-ASSET-003 主动发现任务 V2 回滚

1. 停止扩大灰度，记录候选 bundle、租户、run_id、trace_id 和当前 revision。
2. 将 `ASSET_DISCOVERY_WORKER_ENABLED=false`，等待正在执行的租约到期；不得删除运行中任务。
3. 将 `ASSET_DISCOVERY_JOBS_V2_ENABLED=false`，恢复旧接口兼容路径。
4. 保留 `asset_discovery_runs`、history、candidates、control requests、discovery outbox、资产历史和资产 projection outbox。仅 `pending/approved` 候选不是权威资产；已 `merged` 候选对应的资产是已提交业务事实，回滚不得自动删除。
5. 对账 active job、过期租约、候选状态、资产 revision、审计和两类 outbox。已合并资产如需撤销，必须另行发起带 revision、原因、幂等键和审计的授权修正，不以回滚脚本删除。
6. 修复后以原 idempotency key 和 trace 证据验证重放，再重新开启内部租户灰度。

禁止将“API 已回滚”表述为外部扫描已经停止；必须确认 worker 租约和网络侧活动均已结束。

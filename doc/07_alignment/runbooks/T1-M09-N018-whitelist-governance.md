# T1-M09-N018 whitelist 草案与检测治理

## 当前结论

实现状态为 `PARTIAL`。代码和 run-scoped Kubernetes 验收已经覆盖从 FP 反馈生成草案、独立审批、事件 outbox、真实 Kafka、规则投影 ACK、确定性规则版本、到期自动撤销以及 UI ACK 可见性；生产 consumer、producer 和 detection matcher 均保持默认关闭。本任务不执行 iptables、nftables、ACL 下发或任何真实网络阻断。

最新验收 run 为 `a52db73e-0a2a-4207-9745-33346c9a1755`，证据在 `doc/02_acceptance/topic1/tasks/t1-m09-n018/k8s-whitelist-governance-latest.json`。该 run 使用 Kubernetes 内临时 PostgreSQL 16、3 分区 Kafka/Redpanda 和同时包含 N016–N022 功能的 Web 镜像 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2`，四个测试 Job 全部成功，数据库清理 oracle 为零，所有 run-scoped 资源已删除。验收脚本现在会等待 advertised Kafka listener 真正可达，再创建 topic，避免把 Pod Ready 与 broker 对外可用错误地视为同一时刻。

## 上线顺序

1. 应用 `202608161100_m09_whitelist_consumer_readiness_v2.sql`，保持三个新开关为 `false`。
2. 发布 rule-manager 候选，配置非零 `WHITELIST_CONSUMER_CANDIDATE_SHA256` 和合同 hash，单独开启 `WHITELIST_EVENT_CONSUMER_V2_ENABLED`。
3. 用 canary 事件取得真实 broker offset、规则投影和 applied effect 后的 readiness receipt。手工 READY 行不能通过生产者 join gate。
4. alert-service 使用相同 candidate/contract/group，开启 `WHITELIST_EVENT_PRODUCER_V2_ENABLED`；先对历史 outbox 和 projection 做全量对账。
5. 只有所有 active/approved 当前版本都有 applied/effective ACK 后，才开启 `WHITELIST_DETECTION_MATCHER_V2_ENABLED`。
6. `WHITELIST_EVENT_PIPELINE_V2_ENABLED` 仅保留解析兼容，运行时不再授予任何 rail。

## 停止与回滚

发现错误规则版本、跨租户匹配、过期条目仍命中、outbox published 早于 broker ACK、readiness join 失配或 projection/effect 分叉时立即停止。回滚顺序为 matcher→producer→consumer；先关闭 matcher，避免继续抑制检测，再关闭 producer，等待在途 consumer 提交或停止，最后关闭 consumer。所有 whitelist、history、audit、outbox、effect、projection 和 readiness 记录保留，禁止用物理删除代替补偿。需要恢复某条规则时，提交新的 versioned disable/revoke 命令并等待 revoked ACK。

## 未关闭项

生产候选部署授权、规模性能、Kafka/PG 故障注入、正式回滚演练、Windows Chrome 生命周期以及 T+0/T+1/T+3/T+7 观察尚未完成，因此不得把本证据解释为 M09 或项目整体完成。

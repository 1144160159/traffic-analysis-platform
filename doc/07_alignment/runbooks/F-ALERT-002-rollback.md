# F-ALERT-002 告警—战役关系回滚

适用范围：`CAMPAIGN_ALERT_LINKS_V1_ENABLED` 兼容关系创建/按告警查询，以及默认关闭的
`CAMPAIGN_AGGREGATE_V2_ENABLED` link/unlink、战役侧成员查询和聚合revision绑定。
同时覆盖默认关闭的 `CAMPAIGN_EVENT_PIPELINE_V2_ENABLED` 双流投递，以及
`CAMPAIGN_TARGET_PROJECTION_V2_ENABLED` 对 ClickHouse、OpenSearch、NebulaGraph 的独立确认投影。

## 触发条件

- 任一跨租户关系、closed 战役仍可新增关系；
- 关系、history、outbox、audit 任一原子性或幂等性失败；
- ClickHouse 战役权威读取与 PG workbench revision 持续分歧；
- 关系投影延迟或成员数对账持续扩大。
- 任一目标进入dead、出现同revision身份碰撞，或PG目标水位与CH事件、OS当前态、Nebula关系边持续分歧；
- 三目标readiness失败、OpenSearch alias指错候选、ClickHouse表只读或Nebula schema不兼容。

## 停止扩大

1. 停止新增灰度 tenant。
2. 如触发条件来自外部投影，先将 `CAMPAIGN_TARGET_PROJECTION_V2_ENABLED=false`；如来自Kafka发布、DLQ或inbox持续积压，再将 `CAMPAIGN_EVENT_PIPELINE_V2_ENABLED=false`。命令受理不安全时将 `CAMPAIGN_AGGREGATE_V2_ENABLED=false`，停止新link/unlink聚合命令；如关系读取也不安全，再将 `CAMPAIGN_ALERT_LINKS_V1_ENABLED=false`。已提交业务事务、outbox、inbox和目标水位不得删除。
3. 确认unlink和战役成员端点返回404；只关闭聚合V2时，告警侧既有关系只读和旧link兼容路径仍可用。
4. 保留 `campaign_alert_links`、history、outbox 和 audit；禁止回写调查笔记作为替代。

## 数据处理

- 不删除已经提交的关系或 `campaign_membership_commands`；两套未发布 outbox 均保留等待恢复；
- 禁止以 ClickHouse 数组反向覆盖 PG 关系真源；
- 对账报告分别列出 PG relation revision、relation campaign_revision、workbench revision、member_count、command receipt、双流`(stream,event_id)`、Kafka topic/partition/offset 和投影水位；
- 三目标对账分别列出PG `campaign_target_projection_watermarks` 的target/revision/event/hash、CH immutable projection_id/hash、OS deterministic state ID/external version以及Nebula `relation`边的`attributes_json.relation_revision`、`event_id`、`trace_id`和`projection_sha256`；`relation_type=campaign_alert`是属性值而非独立edge Schema；任何单一2xx或行数都不能代替完整对账；
- 单目标重建必须先关闭worker并冻结精确tenant/relation/revision范围，只重置该目标的watermark与`target_status`键，保持其他目标的applied状态；同revision身份碰撞未裁决前禁止重放；
- 如需业务撤销，必须通过后续正式 unlink 命令生成新 revision，不直接改库。

## 恢复与验证

重新开启前，验证双 scope、跨租户 404、closed 冲突、重复幂等键、并发 revision 冲突、
relation/history/outbox、campaign state/history/outbox、command receipt、audit 原子提交，以及
重复/乱序/过期lease/部分失败/dead/碰撞/租户隔离/单目标重建、三目标readiness与hash对账、
Windows Chrome link/unlink和双向可见性。回滚不删除 migration。
新建或扩展NebulaGraph空间时，必须先等待`entity` tag与`relation` edge传播到storage；出现
`No schema found`时保持目标投影flag关闭并按依赖未就绪处理，不得用重复发布掩盖传播窗口。

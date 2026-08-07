# T-REDIS-001 Redis可靠性域迁移与回滚手册

当前边界：`production_applied=false`，仓库状态为 `SAFE_HOLD`，不得据此声明Redis可靠性域已经在生产关闭。

## 1. 不变量

- coordination使用Sentinel发现、DB 0、AOF和`noeviction`；幂等、去重、限流、租约与撤销状态不得进入`redis-cache`。
- session使用同一可靠实例的独立DB 1和`noeviction`，PostgreSQL仍是令牌与权限权威；Redis不可用且无法完成PG验证时认证fail closed。
- cache使用独立`redis-cache.databases.svc:6379`、`allkeys-lru`且不持久化；丢失后从PG、ClickHouse、OpenSearch或Nebula权威数据重建，失败时绕过缓存。
- Redis Stream/queue不得替代Kafka，除非另有版本化合同、持久化、消费、重放和对账门禁。
- 旧`redis.databases.svc`仅兼容保留，新客户端禁止使用。

## 2. Expand与盘点

1. 只读取key名称和类型，不读取value；按prefix、DB、TTL、内存和调用方生成不可变清单。
2. 记录`maxmemory_policy`、AOF状态、master/replica角色、Sentinel master、`evicted_keys`和过期水位。
3. 先将现有混合客户端保持在可靠`noeviction`域；部署空的cache实例，但不把混合客户端整体切过去。
4. 应用清单显式声明`REDIS_RELIABILITY_DOMAIN`与`REDIS_DB`，候选中不得出现新客户端连接通用Service。

## 3. Backfill、shadow与切换

1. 逐服务拆分cache client与coordination/session client；规则、图谱等纯缓存key可进入cache，JWT、限流、去重和幂等仍走可靠域。
2. cache不做权威backfill；按请求惰性重建。shadow阶段同时读权威源与cache，记录命中、值hash、版本和水位，不在日志保存敏感value。
3. 每次只切一个内部tenant或一个非关键工作负载；观察跨域key数、coordination/session evictions、cache bypass率、P95/P99和权威查询负载。
4. 发现可靠key写入cache、协调域eviction、认证误放行、缓存值覆盖新revision或权威负载越界时立即停止扩大。

## 4. 故障注入

- 对cache执行flush/Pod删除/内存淘汰，业务必须绕过并从权威源恢复，不能把空缓存写成业务不存在。
- 在批准窗口演练Sentinel master故障、网络分区与连接池重建；coordination和session状态不得因淘汰丢失。
- 并发压测热点miss，验证请求合并、随机TTL与权威后端资源预算；当前合同未证明这些门禁。
- 对认证、DLQ重放、告警去重和限流执行重复与中断测试，核对PG审计/receipt与Redis短状态。

## 5. 回滚

- 纯cache客户端可回到可靠`noeviction`域或直接关闭缓存；不得把coordination/session客户端回滚到`redis-cache`。
- 保留`redis-cache`空实例不会改变权威状态；回滚时停止新写、清空可丢缓存并恢复上一候选配置。
- 任何回滚都重新检查DB 0/DB 1绑定、Sentinel master、`evicted_keys=0`、认证负例和权威数据一致性。

## 6. 关闭证据

证据包至少包含候选hash、声明与effective配置hash、按prefix/DB的无value清单、零跨域key对账、eviction与内存指标、Sentinel故障时间线、缓存绕过、P95/P99、回滚和T+0/T+1/T+3/T+7观察。T-REDIS-002的ACL、TLS、服务身份和危险命令限制未通过前，安全门保持开放。

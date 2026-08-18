# P904 `Config.validate`函数设计

状态：`DESIGN_CANDIDATE / BLOCKED_UNTIL_SIGNED_OVERLAY`。before locator由`locator-resolver-receipt.json`绑定；after签名不变：

```go
func (c *Config) validate() error
```

选择`DIRECT`。rail catalog只有在产品允许多组版本化topic时才引入；当前固定两个契约，无需Strategy/Factory。

直接caller已由同目录`caller-load.json`绑定为`config.Load`；`Load`先执行`env.Parse`和JWT fallback，再且仅再调用`(*Config).validate`，因此rail错误发生在producer、consumer、数据库连接或监听端口创建之前。`validate`没有仓内直接callee；`fmt.Errorf`和`len`属于标准库/语言内建调用。配置main和各producer/consumer构造器是下游只读影响面，不进入P904写集合。

| Step | guard / reads | decision / write | postcondition / error | 独立测试 |
|---|---|---|---|---|
| C01 | `c!=nil`且env解析完成 | 保留现有server/PG/discovery/export/detail验证顺序 | 既有错误优先级不被无意改变 | 既有config tests |
| C02 | `Kafka.Enabled` | `strings.TrimSpace(Kafka.Topic)=="asset.bindings.v1"` | 否则精确返回`asset binding topic must be asset.bindings.v1 when Kafka.Enabled` | P913 binding-only/both table |
| C03 | `Kafka.EventOutboxEnabled || Kafka.ProjectionEnabled` | `strings.TrimSpace(Kafka.EventTopic)=="asset.events.v2"` | 否则精确返回`asset event topic must be asset.events.v2 when EventOutboxEnabled or ProjectionEnabled` | P913 outbox-only/projection-only/both table |
| C04 | 两rail同时enabled | 比较规范化Topic/EventTopic | 相等精确返回`asset binding and event topics must differ when both rails are enabled` | P913 equal case |
| C05 | 四个enablement组合 | all-off不校验；binding-only只校验Topic；outbox-only和projection-only只校验EventTopic；both再校验不相等 | disabled rail不受约束，enabled rail绝不放宽 | P913 four-combination table |
| C06 | rail通过 | 继续现有projection/detail验证 | producer创建前完成fail closed | P914 exact run |

P913使用一个满足其他约束的基础`Config`，覆盖all-off、binding-only、outbox-only、projection-only、both-enabled五个合法行，以及binding错写event、event错写binding、两者相等三个精确错误行；断言上述完整错误文本而非仅`err!=nil`。P914通过`go test -json`要求命名测试恰好一次run+pass，零命中、SKIP、FAIL或重复终局均拒绝。回滚恢复旧校验只允许在未启用前进行；已发生串轨时须停intake并对账，不能删事件。

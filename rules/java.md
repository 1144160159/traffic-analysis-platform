# Java / Flink 开发规范

基于 Alibaba Java Guide、Flink 官方最佳实践。

## 1. 命名

```java
// 类：PascalCase
public class SessionAligner { }
// 方法/变量：camelCase
public void processSession(Session s) { }
private long lastCheckpointTime;
// 常量：UPPER_SNAKE_CASE
private static final int MAX_RETRY = 3;
private static final Duration STATE_TTL = Duration.hours(2);
```

## 2. Flink 关键约束

```java
// 必须设置 state TTL
StateTtlConfig ttl = StateTtlConfig
    .newBuilder(Time.minutes(30))
    .setUpdateType(UpdateType.OnCreateAndWrite)
    .setStateVisibility(StateVisibility.NeverReturnExpired)
    .cleanupFullSnapshot()
    .build();

// Checkpoint: 60s 间隔, 10min 超时
env.enableCheckpointing(60_000);
env.getCheckpointConfig().setCheckpointTimeout(600_000);

// 每个 Operator 必须 uid
stream.keyBy(Session::getCommunityId)
    .process(new SessionAligner())
    .uid("session-aligner")      // ← 必须, 保证 savepoint 兼容
    .name("Session Aligner");

// RocksDB 必须指向 SSD
// state.backend.rocksdb.localdir: /home/k8s-data/flink-rocksdb
```

## 3. Sink 批量写入

```java
// ClickHouse: 5000 条或 2s 批量
JdbcExecutionOptions.builder()
    .withBatchSize(5000)
    .withBatchIntervalMs(2000)
    .build();

// 禁止单条 INSERT, HDD 会严重降速
```

## 4. 序列化

```java
// 优先 Protobuf（跨语言兼容）
// 禁止: Java Serializable, 未注册 Kryo 类型, JSON 状态
```

## 5. 错误处理

```java
// 解析失败 → 侧输出流 (不丢数据)
OutputTag<byte[]> failures = new OutputTag<>("parse-failures"){};
try { out.collect(FlowEvent.parseFrom(data)); }
catch (Exception e) { ctx.output(failures, data); }
```

## 6. 测试

```java
// Flink mini-cluster 测试
StreamExecutionEnvironment env = StreamExecutionEnvironment.createLocalEnvironment();
env.setParallelism(1);
// ... construct pipeline ...
CloseableIterator<Session> it = result.executeAndCollect();
```

# Kafka/Flink 坏消息进入 DLQ 验收报告

日期：2026-06-29

## 目标

补齐数据质量闭环中的 live Kafka/Flink 坏消息注入证据：向 `flow.events.v1` 写入不可解析的 FlowEvent payload，验证 Session Flink 作业不崩溃、checkpoint 继续推进，并将坏消息以 `DeadLetter` protobuf 写入统一 `dlq.v1`。

## 本轮实现

- `SessionJob` 输入由 value-only `ProtoDeserializer<FlowEvent>` 改为 raw Kafka record source，保留 topic、partition、offset、key、headers 和原始 value。
- 新增 `FlowEventParseFunction`：合法 FlowEvent 进入原 Session 聚合链路；解析失败、空 payload、缺 `tenant_id` 或缺 `community_id` 进入 `DeadLetter` side output。
- 新增 `KafkaSinkFactory.createDeadLetterSink`，将解析失败写入 `dlq.v1`，保留确定性 `event_id=flink-session-parse:<topic>:<partition>:<offset>`、`source_key`、`error_msg` 和 base64 原始 payload。
- 新增 `tests/e2e/live_kafka_flink_bad_message_dlq.sh`，可复跑坏消息注入、Flink checkpoint、protobuf DeadLetter 解码验证。

## 验证结果

- 定向单测：`mvn -pl flink-session-job -am test -Dtest=FlowEventParseFunctionTest -DfailIfNoTests=false -Dsurefire.failIfNoSpecifiedTests=false`，3/3 passed。
- Package：`mvn -pl flink-session-job -am package -DskipTests` 通过；shade overlap 警告为既有依赖打包警告。
- Live 部署：`flink-session-job-parse-dlq-20260629-r2.jar` 已提交为 `Session Aggregation Job V2`，JobID `726eafd2883a14fd18c300d204d323b5`。
- Live 验证：`live-kafka-flink-bad-message-dlq-20260629-kafka-flink-badmsg-r1-summary.json` 显示 7/7 checks passed，0 failed。

关键证据：

- 注入 source key：`campus-a:codex-flink-bad-message:20260629-kafka-flink-badmsg-r1`
- 注入 payload：`codex-invalid-flowevent-protobuf-20260629-kafka-flink-badmsg-r1`
- Flink checkpoint：`Session Aggregation Job V2` checkpoint 从 `1` 推进到 `2`
- DeadLetter：`event_id=flink-session-parse:flow.events.v1:12:15333449`
- DeadLetter error：`invalid FlowEvent protobuf: Protocol message tag had invalid wire type.`
- DeadLetter raw payload：`raw_matches=true`
- 验证后 Session job：24/24 tasks RUNNING，failed tasks 0

## 证据文件

- `live-kafka-flink-bad-message-dlq-20260629-kafka-flink-badmsg-r1-summary.json`
- `live-kafka-flink-bad-message-dlq-20260629-kafka-flink-badmsg-r1.ndjson`
- `20260629-kafka-flink-badmsg-r1-deadletter-match.json`
- `20260629-kafka-flink-badmsg-r1-flink-jobs-before.json`
- `20260629-kafka-flink-badmsg-r1-flink-jobs-after.json`
- `20260629-kafka-flink-badmsg-r1-flink-checkpoints-before.json`
- `20260629-kafka-flink-badmsg-r1-flink-checkpoints-after.json`
- `20260629-kafka-flink-badmsg-r1-consume-deadletter.go`
- `20260629-kafka-flink-badmsg-r1-deadletter-consumer.err`

## 副作用与边界

- 本轮 live 验证向 `flow.events.v1` 写入 1 条带 run_id 的坏消息，并向 `dlq.v1` 写入 1 条对应 DeadLetter，作为可追溯验收样本保留。
- r1 live 部署曾暴露 `RawKafkaRecord.headers` 使用 unmodifiable map 导致 Kryo copy 失败；已修复为普通 `HashMap` 并以 r2 重新部署，最终 live 证据均来自 r2。
- 本轮关闭了“Kafka/Flink 坏消息注入进入 DLQ”的证据缺口；数据质量页合法登录态 Desktop 浏览器业务页点击仍需单独补证。

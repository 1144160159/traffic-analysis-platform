# 端到端 P95 时间戳链 live report

日期：2026-06-29

## 范围

- 后端：alert-service 提供 `GET /api/v1/data-quality/latency-chain`，从 ClickHouse 统计 `event_ts/ingest_ts/kafka_ts/flink_out_ts` 和告警 `last_seen/created_at` 分段 P50/P90/P95/P99。
- 测试：`tests/e2e/live_latency_chain_report.sh` 同轮采集 ClickHouse、APISIX API、Playwright UI、截图和 summary；合并 UI `ui_seen_ts` 后重新计算 pass/fail/gap。
- 部署：alert-service r3 已运行在 `docker.io/traffic/alert-service:latency-chain-20260629-r3`；Flink Session r8 以 parallelism 12 运行；TaskManager 已调整为 `taskmanager.numberOfTaskSlots=32`、`taskmanager.memory.jvm-metaspace.size=1024m`；滚动更新后 PCAP Index 与 CEP 作业已恢复为 RUNNING。

## 关键修复

- Proto/Go/Java/Rust/ClickHouse 链路补齐 `kafka_ts` 与 `flink_out_ts`。
- Session Job 低延迟 profile 使用 5s session gap、5s watermark，并写入 `flows_raw`/`sessions`。
- Feature Job 默认 tenant 配置改为 priority 10 且禁用降级，避免 live 默认租户因 backpressure 被跳过。
- 告警延迟口径从 `created_at - first_seen` 改为 `created_at - last_seen`，避免把去重窗口生命周期误记为处理延迟。
- Flink TaskManager 修复 Metaspace OOM，并扩展 slot，使 Session parallelism 12 可运行。
- PCAP Index / CEP 提交参数统一为 ClickHouse `host:port[,host:port]`，并在脚本中拒绝误传 JDBC URL。

## 最终验收

最终证据：`live-latency-chain-20260629-latency-chain-r15-3m-final-r8-slot32-summary.json`

- 结果：`pass`
- `full_chain_closed`：`true`
- `command_failures`：`0`
- `gaps`：`[]`
- UI：`/data-quality?tab=topic-health` Playwright 成功测得 `ui_seen_ts`，无 console/page error/request failure。

3 分钟窗口 P95：

| 分段 | P95 |
|---|---:|
| flow_event_to_ingest | 5960 ms |
| session_event_to_ingest | 5980 ms |
| session_ingest_to_kafka | 198 ms |
| session_kafka_to_flink | 16113 ms |
| session_event_to_flink | 22013 ms |
| alert_last_seen_to_created | 22275 ms |

数据质量 API：`overall=healthy`；`flow_rate=52165.3 flows/min`；`end_to_end_latency` P95 为 `5.972 ms`。`data_completeness` 仍为 warn，Feature/Session ratio `0.89 < 0.9`，不阻断 GATE-P0-05，但后续应继续观察 Feature 输出比例。

## 证据文件

- `live-latency-chain-20260629-latency-chain-r15-3m-final-r8-slot32-summary.json`
- `live-latency-chain-20260629-latency-chain-r15-3m-final-r8-slot32.ndjson`
- `20260629-latency-chain-r15-3m-final-r8-slot32-latency-chain-api.json`
- `20260629-latency-chain-r15-3m-final-r8-slot32-data-quality-api.json`
- `20260629-latency-chain-r15-3m-final-r8-slot32-ui-data-quality.json`
- `20260629-latency-chain-r15-3m-final-r8-slot32-ui-data-quality.png`
- `20260629-latency-chain-r15-3m-final-r8-slot32-clickhouse-latency-columns.jsonl`
- `20260629-latency-chain-r15-3m-final-r8-slot32-clickhouse-flows-latency.jsonl`
- `20260629-latency-chain-r15-3m-final-r8-slot32-clickhouse-sessions-latency.jsonl`
- `20260629-latency-chain-r15-3m-final-r8-slot32-clickhouse-alerts-latency.jsonl`
- `alert-service-pods-r3-20260629.txt`
- `flink-taskmanager-env-slot32-metaspace1024-20260629.txt`
- `flink-taskmanagers-slot32-20260629.json`
- `flink-session-job-r8-overview-20260629.json`
- `flink-session-job-r8-checkpoints-20260629.json`
- `flink-session-job-r8-exceptions-20260629.json`
- `kafka-lag-flink-session-job-20260629-r8-final.txt`
- `flink-final-overview-20260629.json`
- `flink-pcap-index-r2-health-20260629.txt`
- `flink-cep-r2-health-20260629.txt`
- `flink-pcap-index-restore-submit-20260629-r2.out`
- `flink-cep-restore-submit-20260629-r2.out`

## 边界

- 本报告关闭的是 GATE-P0-05 的 3 分钟 live P95 时间戳链，不替代 10 x 100Gbps、512Mpps、第三方检测质量或 HA 故障演练。
- `/api/v1/data-quality/latency-chain` 原始 API 仍把 `ui_seen_ts` 标记为 browser test missing；脚本在 Playwright 阶段补入 UI 证据后生成最终闭环 summary。

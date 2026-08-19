# Codex Loop God View

- generated_at: `2026-06-24T06:48:31`
- commit: `e3316aec4ac1d6592e28aefc86853128ecde7408`
- branch: `main`
- dirty_items_seen: `557`
- task_pool: `12` tasks, `10` P0
- high_risk_tasks: `6`
- evidence_runs: `263`

## Immediate Signals
- `/screen` is detected outside `ProtectedLayout`; keep `CLE-P0-SCREEN-001` high priority.
- `apisix_routes` changes affect: CLE-P0-ROUTE-001, CLE-P0-SEC-001
- `database_schema` changes affect: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001
- `kafka_topics` changes affect: CLE-P0-DLQ-001, CLE-P0-SEC-001
- `proto` changes affect: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001

## Recommended Next Tasks
- `CLE-P0-AUTH-001` (regression): 启动 /auth/me 鉴权和 WebSocket 延迟连接
- `CLE-P0-BASELINE-001` (regression): 生成 baseline/release manifest 草案
- `CLE-P0-DLQ-001` (regression): DLQ replay API、审批、审计、幂等验证
- `CLE-P0-P95-001` (acceptance-prep): 完整 P95 时间戳链设计与埋点计划
- `CLE-P0-PCAP-001` (regression): PCAP hash、签名 URL、跨租户拒绝、下载审计

## Evidence Discipline
- Current scout output is a regression-level context snapshot, not acceptance or third-party evidence.
- Tasks can close only after their own `close_when` and Reviewer Gate pass.

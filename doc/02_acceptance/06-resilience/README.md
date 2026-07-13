# 06 Resilience Evidence

更新时间：2026-07-01

本目录保存 GATE-P0-08 的稳定证据副本。当前有只读 readiness preflight、维护窗口演练证据 bootstrap 和 HA drill review packet，不等同于 HA 验收通过。

## 当前结论

`ha-readiness-preflight-latest.json` 来自 `20260701-ha-readiness-preflight-r10-review-packet`，结果为 `blocked`：

- 13/14 checks passed，1 blocker，0 warning。
- Kafka 29 个 topic leader/ISR 正常；readiness 脚本已改为使用 `SASL_SSL` secure admin config 与明确 Kafka admin heap，避免 plaintext/heap 假阻塞。
- Flink 9 个业务 job 均 RUNNING，checkpoint 与 exception 检查通过；本轮先清理失效 HA JobGraph 句柄恢复 JobManager，再用完整 Kafka `SASL_SSL`/PKCS12、ClickHouse、PostgreSQL 提交环境恢复 9 个业务作业。
- ClickHouse 13 张复制表健康，PostgreSQL 2 个 streaming replicas，Redis Sentinel、MinIO cluster-local health、APISIX 业务入口可达。
- Kafka 与 Redis PDB selector 已与实际 Pod label 对齐，所有关键 PDB 当前均允许 1 个 voluntary disruption。
- 唯一 blocker：根目录 6 个正式演练报告均未产出，缺 `kafka-failover.md`、`flink-failover.md`、`clickhouse-failover.md`、`postgres-failover.md`、`minio-failover.md` 和 `ha-rto-rpo-latest.json`。
- 新增 integrity guard：正式 HA/RTO-RPO artifact 会扫描 `review-template`、`review_required`、`template_review_required`、`formal_gate_note`、`TBD` 等草案标记；把 `bootstrap/latest/` 下的模板改名放到根目录也会被 blocker 拦住。
- 旧 `databases/redis-data-redis-0` Pending PVC 和 3 个 stale Released PV 已清理，不再是当前 HA warning。

`bootstrap/ha-drill-evidence-bootstrap-latest.json` 来自 `20260630-ha-drill-evidence-bootstrap-r1`，结果为 `pass`，但该结果只表示演练包草案生成成功：

- 7/8 checks passed，0 blocker，1 warning。
- `formal_artifact_count=0`，证明 bootstrap 没有在本目录根部生成正式 `*failover*.md` 或 `*rto-rpo*.json` 报告。
- 草案包整理了 `ha_drill_plan.yaml`、最新 HA readiness、operator approval、timeline、snapshot index、RTO/RPO table、data consistency report、5 类组件 failover report review-template 和 evidence manifest。
- 所有人工审批、维护窗口执行记录、RTO/RPO 数值和数据一致性结论仍需 SRE/QA 在真实演练后填写与签认。

`review/ha-drill-review-latest.json` 来自 `20260701-ha-drill-review-r1`，结果为 `pass`，但该结果只表示维护窗口前的 review board 已生成：

- 6/6 checks passed，0 blocker，0 warning。
- 5 个 HA 组件目标齐全，7 个 review 文件已写入 `review/latest/`。
- `formal_artifact_count=0`，证明 review packet 不生成正式根目录 failover/RTO-RPO 报告。
- 该包把 bootstrap 模板转换成 component drill review、RTO/RPO evidence worklist、maintenance-window approval template、formal artifact manifest template、data-consistency checklist 和 operator checklist，用于 SRE/QA 审查后执行真实 destructive drill。

## 证据入口

- `ha-readiness-preflight-latest.json`
- `ha-readiness-preflight-latest.md`
- `ha-workload-readiness-latest.json`
- `kafka-topic-health-latest.json`
- `flink-running-job-health-latest.json`
- `clickhouse-replication-latest.json`
- `bootstrap/ha-drill-evidence-bootstrap-latest.json`
- `bootstrap/ha-drill-evidence-bootstrap-latest.md`
- `bootstrap/latest/`
- `review/ha-drill-review-latest.json`
- `review/ha-drill-review-latest.md`
- `review/latest/`

## 下一步

按 `tests/chaos/ha_drill_plan.yaml` 在维护窗口执行 destructive drill。可先使用 `bootstrap/latest/` 和 `review/latest/` 组织审批、时间线、快照、报告和审查工作板；只有产出根目录正式 `kafka-failover.md`、`flink-failover.md`、`clickhouse-failover.md`、`postgres-failover.md`、`minio-failover.md` 和 `ha-rto-rpo-latest.json`，且这些正式文件不再包含草案/模板标记后，才能将 GATE-P0-08 从 blocked 改为通过。

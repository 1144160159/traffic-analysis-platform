# 试点部署证明模板

## 1. 基本信息

| 项 | 填写 |
|---|---|
| 试点单位 | 待用户填写 |
| 试点站点/园区 | 待用户填写 |
| 试点周期 | 待用户填写 |
| 系统版本 | `20260630-release-manifest-r72-ui-r12-desktop-transport-current` |
| 部署负责人 | 待用户填写 |
| 用户联系人 | 待用户填写 |
| 数据授权编号 | 待用户填写 |

## 2. 入场前置确认

| 检查项 | 结论 | 证据 |
|---|---|---|
| TAP/SPAN/镜像点位已授权 | 待现场确认 | 授权单编号 |
| site values 已确认 | 待现场确认 | `deployments/kubernetes/site-values.template.yaml` 的现场实例 |
| 资源配额和保留期已确认 | 待现场确认 | CPU/内存/磁盘/PCAP retention 表 |
| NTP/PTP 时间同步已确认 | 待现场确认 | 时间同步检查记录 |
| 只读观测优先策略已确认 | 待现场确认 | 变更审批或会议纪要 |
| 明文 Secret 未进入交付件 | 内部证据通过，待交付复核 | production security r49、ExternalSecret reconciliation r1、waiver registry |

## 3. 部署拓扑

| 层级 | 组件 | 现场值 | 证据 |
|---|---|---|---|
| 采集 | Probe Agent | 内部证据通过，待现场签认 | probe-ops governance r2、release manifest r72 |
| 接入 | APISIX / Ingest Gateway | 内部证据通过，待现场签认 | deployment preflight r60、business flow API r26 |
| 消息 | Kafka | TLS/SASL/ACL live rollout 已通过，待现场签认 | Kafka rollout r6、Kafka security preflight r9、release manifest r72 live topics |
| 分析 | Flink jobs | 内部证据通过，待现场签认 | release manifest r72 workloads、HA readiness r9 non-destructive checks |
| 存储 | PostgreSQL / ClickHouse / OpenSearch / MinIO | 内部证据通过，待现场签认 | release manifest r72、HA readiness r9 non-destructive checks |
| 控制面 | Go API services | 内部证据通过，待现场签认 | deployment preflight r60、business flow API r26 |
| 前端 | Web UI | repo/UI/API 契约通过，待 Desktop Chrome bridge 恢复 | UI contract r12 |

## 4. 版本基线

| 基线项 | 证据 |
|---|---|
| Git commit / dirty status | `doc/02_acceptance/00-baseline/release-manifest-latest.json` |
| 镜像 digest | `deployments/kubernetes/image-digests.lock.json` |
| K8s manifest hash | release manifest file hashes |
| 数据库 schema hash | release manifest file hashes |
| Kafka topic catalog | release manifest repo/live topic list |
| 模型/规则/部署版本 | release manifest API catalog |

## 5. 部署验收记录

| 检查项 | 通过标准 | 结果 | 证据 |
|---|---|---|---|
| deployment preflight | `pass` 且 0 blocker | pass：r60 16/16 | `doc/02_acceptance/07-deployment/deployment-preflight-latest.json` |
| business flow API | 全部经 APISIX 通过 | pass：r26 46/46 | `doc/02_acceptance/02-regression/business-flow-api-preflight-latest.json` |
| UI contract | repo/UI/API 通过，Desktop Chrome 需记录当前状态 | partial：r12 非浏览器 19/19，Desktop Chrome `Transport closed` | `doc/02_acceptance/02-regression/ui-contract-preflight-latest.json` |
| P95 latency chain | full chain closed | pass：r15 full chain closed | `doc/02_acceptance/runs/20260629-latency-chain/` |
| DLQ/replay | dry-run、非 dry-run、幂等和 partial failure 证据齐全 | pass：DLQ dry-run/recovery/failure/kafka bad message evidence present | `doc/02_acceptance/runs/20260629-*dlq*` |
| PCAP integrity | hash、verify、presign、audit 通过 | pass：hash、verify、presign、audit evidence present | `doc/02_acceptance/runs/20260629-pcap-forensics-integrity/` |

## 6. 连续运行证明

| 时间窗 | 运行时长 | 关键指标 | 结论 | 证据 |
|---|---:|---|---|---|
| 待试点周报填写 | 待试点周报填写 | Probe ready、Flink checkpoint、Kafka ISR、API 5xx、UI error | 待现场连续运行确认 | `pilot-weekly-report-template.md` |

## 7. 遗留项

| 遗留项 | 等级 | 是否影响试点 | 关闭证据 |
|---|---|---|---|
| Desktop Chrome bridge runtime | P0 | 影响浏览器试点签认 | UI contract preflight browser checks pass |
| NetworkPolicy enforcement-capable CNI | P0 | 影响生产安全签认 | 默认拒绝/白名单负例通过 |
| HA 破坏性演练 | P0 | 影响 HA 签认 | RTO/RPO 报告 |
| 第三方盲测签认 | P0 | 影响算法质量签认 | CNAS/第三方报告 |
| 10 x 100Gbps / 512Mpps | P0 | 影响性能验收签认 | 真实硬件窗口 summary |
| 现场资产清单 | P0 | 影响资产覆盖签认 | 用户确认 `SITE_ASSET_INVENTORY_JSON` |

## 8. 签认

| 角色 | 姓名 | 日期 | 意见 |
|---|---|---|---|
| 用户代表 | 待用户填写 | 待签字填写 | 待签字 |
| 实施负责人 | 待填写 | 待签字填写 | 待签字 |
| 项目经理 | 待填写 | 待签字填写 | 待签字 |

# 用户确认与签认模板

## 1. 确认范围

| 项 | 填写 |
|---|---|
| 试点单位 | 待用户填写 |
| 系统名称 | 园区网络全流量采集与分析系统 |
| 版本基线 | `20260701-release-manifest-r80-ha-review-r1` |
| 试点周期 | 待用户填写 |
| 签认日期 | 待签字填写 |

## 2. 功能确认

| 功能 | 用户确认 | 证据 |
|---|---|---|
| 探针接入和采集健康 | 内部证据通过，待用户签认 | `20260630-probe-ops-governance-r2`、release manifest r80 |
| 告警运营和状态闭环 | 内部证据通过，待用户签认 | alert state / Playbook / Whitelist / Baseline governance evidence |
| PCAP 取证和完整性校验 | 内部证据通过，待用户签认 | `20260629-pcap-forensics-integrity`、Forensics task state machine |
| 资产/图谱分析 | 内部证据通过，待现场资产清单签认 | business flow API r26、asset coverage r3 blocked on site inventory review |
| 规则/模型/部署治理 | 内部证据通过，待用户签认 | rule/model/deployment state evidence |
| 数据质量和 DLQ 重放 | 内部证据通过，待用户签认 | DLQ replay/recovery evidence、latency-chain r15 |
| 审计追溯 | 内部证据通过，待用户签认 | compliance/notification/topic/whitelist/baseline/settings/probe audit evidence |
| OIDC/SSO 登录入口 | 内部证据通过，待用户签认 | OIDC/SSO preflight r4：login entry、callback chunk、Keycloak authorization page 均通过 |
| UI 演示链路 | repo/UI/API 静态契约通过，待 Desktop Chrome bridge 和视觉交互证据恢复后签认 | UI contract r17：静态契约通过；UI visual/interaction r37：visual diff 0/30、business interaction 4/28；finalizer r1 blocked；Desktop Chrome wrapper `Transport closed` |

## 3. 验收状态

| 类别 | 当前状态 | 是否签认 |
|---|---|---|
| 功能回归 | 内部回归证据通过：business flow API、governance/state flows、Fusion value-report structure 均已 pass | 待用户签认 |
| 可复现部署 | release manifest r80、deployment preflight r60 已 pass | 待用户签认 |
| OIDC/SSO 登录 | oidc-sso preflight r4 已 pass | 待用户签认 |
| UI 视觉和业务交互 | blocked：待 Desktop Chrome extension backend 恢复并补齐 30/30 视觉 diff 与 28/28 业务交互证据 | 待用户签认 |
| P95 延迟链 | latency-chain r15 已记录 full chain closed | 待用户签认 |
| 10 x 100Gbps / 512Mpps | blocked：待专项验证和真实硬件窗口结果 | 待用户签认 |
| 95%/5% 检测质量 | blocked：待第三方/盲测签认，正式 labels/predictions/attestation 未提供 | 待用户签认 |
| 生产安全 | blocked：Kafka TLS/SASL/ACL、ExternalSecret、digest pin 和 waiver registry 已闭环；待 policy-capable CNI 与 NetworkPolicy 负例通过 | 待用户签认 |
| HA RTO/RPO | blocked：待维护窗口破坏性演练和 RTO/RPO 报告 | 待用户签认 |

## 4. 例外项

| 例外项 | 等级 | 是否影响本次试点签认 | 后续关闭标准 |
|---|---|---|---|
| Desktop Chrome bridge runtime | P0 | 影响浏览器签认 | 恢复 Codex Desktop Chrome extension bridge 后 rerun UI contract preflight、UI visual/interaction preflight 和 evidence finalizer 为 pass |
| UI visual/interaction evidence | P0 | 影响 UI 业务签认 | 补齐 30 个 1920x1080 页面视觉 diff 和 28 条业务交互证据，且 finalizer 通过 |
| NetworkPolicy-capable CNI | P0 | 影响生产安全签认 | 接入或迁移 policy-capable CNI，并通过 default-deny/allow-list 负例 |
| HA destructive RTO/RPO | P0 | 影响 HA 签认 | 完成 Kafka/Flink/ClickHouse/PostgreSQL/MinIO 破坏性演练并发布 RTO/RPO 报告 |
| 10 x 100Gbps / 512Mpps | P0 | 影响性能验收签认 | 提供真实硬件窗口结果 summary 和原始证据 |
| Detection quality third-party package | P0 | 影响算法质量签认 | 提供冻结数据集、labels、predictions、threshold lock 和第三方 attestation |
| Site asset inventory | P0 | 影响资产覆盖签认 | 用户确认 `SITE_ASSET_INVENTORY_JSON` 并 rerun coverage gate |

## 5. 签字

| 单位 | 角色 | 姓名 | 日期 | 签字 |
|---|---|---|---|---|
| 用户单位 | 业务负责人 | 待用户填写 | 待签字填写 | 待签字 |
| 用户单位 | 技术负责人 | 待用户填写 | 待签字填写 | 待签字 |
| 承建单位 | 项目经理 | 待填写 | 待签字填写 | 待签字 |
| 承建单位 | 实施负责人 | 待填写 | 待签字填写 | 待签字 |

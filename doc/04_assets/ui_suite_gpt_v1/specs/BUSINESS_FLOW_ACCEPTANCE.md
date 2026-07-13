# 业务链路验收清单

本文件从业务闭环角度约束前端实现，防止页面视觉完成但业务动作断链。

## 登录、会话、权限与审计入口

- 路由：`/login` -> `/dashboard` -> `/screen` -> `/settings` -> `/audit-log`
- API：`/api/v1/dashboard/stats`、`/api/v1/dashboard/alerts/trend`、`/api/v1/dashboard/attack-phases`、`/api/v1/dashboard/encrypted/trend`、`/api/v1/tokens/scopes`、`/api/v1/tokens`、`/api/v1/tokens/scopes/probe`、`/api/v1/audit/logs`
- 浮层：`dropdown-user-menu`、`drawer-mobile-navigation`、`modal-global-search`、`dropdown-quick-entry`、`modal-login-error-captcha`、`drawer-dashboard-kpi-detail`、`drawer-dashboard-task-detail`、`modal-settings-token`、`drawer-audit-operation-detail`、`modal-audit-export`、`popconfirm-settings-token-revoke`、`drawer-settings-rbac-edit`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/login.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/dashboard.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/screen.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/settings.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/audit-log.md`
- 验收：
  - 未登录跳转登录页
  - 权限不足展示 403 且列出 requiredScopes
  - 只读大屏受 screen:view 或 masked-demo 门禁约束
  - 用户菜单能进入设置/审计并可退出
  - 敏感操作写入审计线索

## 日常值班闭环：仪表盘 -> 告警 -> 证据 -> 反馈

- 路由：`/dashboard` -> `/alerts` -> `/alerts/:alertId` -> `/forensics` -> `/whitelist` -> `/audit-log`
- API：`/api/v1/dashboard/stats`、`/api/v1/dashboard/alerts/trend`、`/api/v1/dashboard/attack-phases`、`/api/v1/alerts`、`/api/v1/alerts/{id}`、`/api/v1/alerts/{id}/evidence`、`/api/v1/alerts/{id}/feedback`、`/api/v1/pcap/jobs`、`/api/v1/pcap/stats`、`/api/v1/whitelist`、`/api/v1/audit/logs`
- 浮层：`drawer-mobile-navigation`、`modal-global-search`、`dropdown-quick-entry`、`drawer-dashboard-kpi-detail`、`drawer-dashboard-task-detail`、`modal-alert-batch`、`dropdown-alert-batch-actions`、`dropdown-alert-row-actions`、`modal-alert-status`、`modal-alert-feedback`、`modal-evidence-detail`、`modal-forensics-task`、`modal-playbook-trigger`、`modal-whitelist-draft-from-alert`、`popconfirm-pcap-download`、`drawer-session-replay`、`modal-whitelist-add`、`drawer-whitelist-approval`、`modal-forensics-evidence-export`、`drawer-audit-operation-detail`、`modal-audit-export`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/dashboard.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/alerts.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/alert-detail.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/forensics.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/whitelist.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/audit-log.md`
- 验收：
  - 仪表盘待办可定位到告警队列
  - 告警详情展示证据链和状态机
  - 处置/反馈必须经过二次确认或权限提示
  - 取证下载和反馈操作留痕

## 专题研判闭环：隧道、外传、APT 三态

- 路由：`/topics` -> `/encrypted-traffic` -> `/campaigns` -> `/campaigns/:campaignId` -> `/attack-chains` -> `/forensics`
- API：`/api/v1/topics/tunnel`、`/api/v1/topics/exfil`、`/api/v1/topics/apt`、`/api/v1/encrypted-traffic/stats`、`/api/v1/encrypted-traffic/sessions`、`/api/v1/encrypted-traffic/ja3`、`/api/v1/encrypted-traffic/tunnels`、`/api/v1/encrypted-traffic/exfiltration`、`/api/v1/campaigns`、`/api/v1/campaigns/{id}`、`/api/v1/attack-chains`、`/api/v1/pcap/jobs`、`/api/v1/pcap/stats`
- 浮层：`modal-forensics-task`、`drawer-campaign-detail`、`drawer-attack-chain-detail`、`drawer-encrypted-fingerprint`、`drawer-certificate-detail`、`popconfirm-pcap-download`、`drawer-session-replay`、`modal-topic-save-view`、`drawer-topic-scope-edit`、`modal-topic-report-export`、`modal-topic-evidence-package-export`、`drawer-topic-subscription`、`dropdown-topic-share-favorite`、`modal-forensics-evidence-export`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/topics.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/encrypted-traffic.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/campaigns.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/campaign-detail.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/attack-chains.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/forensics.md`
- 验收：
  - 专题切换保留筛选上下文
  - 专题报告/证据包导出有权限与影响范围
  - 可下钻到加密流量、战役、攻击链和取证
  - 审计 trace 可回放

## 资产证据闭环：资产 -> 图谱 -> 融合 -> 基线

- 路由：`/assets` -> `/graph` -> `/fusion` -> `/baselines` -> `/alerts`
- API：`/api/v1/assets`、`/api/v1/graph/explore`、`/api/v1/fusion/stats`、`/api/v1/fusion/entities`、`/api/v1/fusion/value-report`、`/api/v1/baselines`、`/api/v1/alerts`
- 浮层：`modal-alert-batch`、`dropdown-alert-batch-actions`、`dropdown-alert-row-actions`、`drawer-asset-detail`、`modal-asset-edit`、`drawer-asset-history`、`drawer-graph-entity`、`drawer-graph-path-analysis`、`drawer-fusion-conflict`、`modal-fusion-rule-edit`、`modal-baseline-threshold`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/assets.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/graph.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/fusion.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/baselines.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/alerts.md`
- 验收：
  - 资产详情、实体图谱和告警上下文互跳
  - 融合冲突可解释并可审计
  - 基线阈值调整有预览和回滚
  - 风险分数与证据来源对应

## 采集质量闭环：探针 -> 数据质量 -> DLQ/重放

- 路由：`/probes` -> `/data-quality` -> `/mlops` -> `/compliance`
- API：`/api/v1/probes`、`/api/v1/data-quality`、`/api/v1/mlops/status`、`/api/v1/mlops/conditions`、`/api/v1/compliance/reports`、`/api/v1/compliance/audit-trail`
- 浮层：`drawer-probe-detail`、`modal-probe-config`、`modal-probe-batch-upgrade`、`modal-probe-cert-rotate`、`drawer-probe-log`、`drawer-dlq-sample`、`modal-data-replay-task`、`drawer-field-quality-sample`、`drawer-mlops-task-detail`、`drawer-compliance-gate-detail`、`modal-compliance-evidence-package-export`、`modal-compliance-report-export`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/probes.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/data-quality.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/mlops.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/compliance.md`
- 验收：
  - 探针离线、背压、字段缺失有专门状态
  - DLQ 样本可查看和重放
  - 质量指标进入验收/合规视图
  - 所有修复动作记录操作者和时间窗

## 检测发布闭环：规则 -> 部署 -> 模型 -> 剧本 -> 白名单

- 路由：`/rules` -> `/deployments` -> `/models` -> `/mlops` -> `/playbooks` -> `/whitelist`
- API：`/api/v1/rules`、`/api/v1/deployments`、`/api/v1/models`、`/api/v1/mlops/status`、`/api/v1/mlops/conditions`、`/api/v1/playbooks/catalog`、`/api/v1/playbooks/executions`、`/api/v1/whitelist`
- 浮层：`modal-rule-edit`、`drawer-rule-detail`、`popconfirm-delete`、`modal-rule-publish`、`modal-deployment-create`、`modal-deployment-rollback`、`drawer-model-detail`、`drawer-mlops-task-detail`、`modal-playbook-edit`、`modal-whitelist-add`、`drawer-whitelist-approval`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/rules.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/deployments.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/models.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/mlops.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/playbooks.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/whitelist.md`
- 验收：
  - 规则发布前必须测试验证
  - 部署支持灰度、回滚和审计
  - 模型激活可回退
  - 剧本触发有风险控制
  - 白名单有审批和到期治理

## 治理闭环：合规、审计、通知、系统配置

- 路由：`/compliance` -> `/audit-log` -> `/notifications` -> `/settings`
- API：`/api/v1/compliance/reports`、`/api/v1/compliance/audit-trail`、`/api/v1/audit/logs`、`/api/v1/notifications/settings`、`/api/v1/tokens/scopes`、`/api/v1/tokens`、`/api/v1/tokens/scopes/probe`
- 浮层：`dropdown-user-menu`、`drawer-notification-center`、`modal-settings-token`、`drawer-compliance-gate-detail`、`modal-compliance-evidence-package-export`、`modal-compliance-report-export`、`drawer-audit-operation-detail`、`modal-audit-export`、`modal-notification-channel-edit`、`modal-notification-template-preview-test`、`drawer-notification-silence-rule`、`popconfirm-settings-token-revoke`、`drawer-settings-rbac-edit`
- 契约：`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/compliance.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/audit-log.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/notifications.md`、`doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/settings.md`
- 验收：
  - 合规证据包可导出
  - 审计日志可按用户/对象/动作追踪
  - 通知升级和静默规则可测试
  - API token 创建/撤销必须权限提示和二次确认


## 通用业务底线

- 所有危险动作必须包含权限提示、影响范围、审计 trace 和取消/确认动作。
- 页面内状态流转必须能从数据驱动，不能只写静态卡片。
- 同一业务对象跨页面跳转时必须保留对象 ID、时间窗、租户/站点上下文。
- 导出、下载、撤销、发布、回滚、封禁、隔离等动作必须可追溯。

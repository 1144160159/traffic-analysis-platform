# GPT Imagegen UI Suite Generation State

更新日期：2026-06-28

本文档用于重连、压缩上下文或中断后继续生成 181 张高保真 UI 套装。

最新接力压缩文档：`doc/04_assets/ui_suite_gpt_v1/CONTEXT_HANDOFF.md`。重连后如只需恢复本轮 UI 生图上下文，先读该文件。

## 当前进度

- 总数：181 张
- 已完成：181 张
- 当前优先任务：`/topics` 专题面板不再生成单张页面主图；加密隧道、数据外传、APT 战役的拆分设计输入在前端开发时合并为同一个 `/topics` 页内 Tab/Segmented 状态
- 下一张常规生成项：无
- 下一张 prompt：无
- 下一张目标：无
- 2026-06-28 P7 扩容 batch-09：完成 `popconfirm-settings-token-revoke`、`drawer-settings-rbac-edit`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。当前 P7 已完成 18/18；manifest 交付基线完成 181/181，无下一张常规生成项。
- 2026-06-28 P7 扩容 batch-08：完成 `modal-notification-template-preview-test`、`drawer-notification-silence-rule`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-07：完成 `modal-audit-export`、`modal-notification-channel-edit`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-06：完成 `modal-compliance-report-export`、`drawer-audit-operation-detail`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-05：完成 `drawer-compliance-gate-detail`、`modal-compliance-evidence-package-export`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-04：完成 `modal-campaign-report-export`、`modal-forensics-evidence-export`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-03：完成 `drawer-topic-subscription`、`dropdown-topic-share-favorite`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-02：完成 `modal-topic-report-export`、`modal-topic-evidence-package-export`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 P7 扩容 batch-01：用户确认“全部处理生成 UI 图”后，已把 P7 18 张浮层扩展进 `manifest.json` 和 prompt 队列，总计划从 163 扩展到 181。已完成 `modal-topic-save-view`、`drawer-topic-scope-edit`，两张均使用内置 `image_gen.imagegen` 逐张生成，并通过 `extract_latest_imagegen.py` 提取到 `screens/overlays/`；最终 PNG 均为 `1920x1080`，raw 原图均已保留。
- 2026-06-28 文档与 prompt 源头修正：旧设计文档中的左侧导航尺寸/结构口径已统一到当前 `screen.png` 的 `166px` 单栏展开式 AppShell；`build_prompt_manifest.mjs` 与现有 state prompt 已补齐 401/403 硬门禁，后续重建 prompts 时 `state-unauthorized` 只能表达重新认证，`state-forbidden` 只能表达已登录但权限不足。
- 2026-06-27 浮层新口径：后续 overlay 不需要公共 AppShell 或宿主页面公共区，只输出弹窗/抽屉/下拉/确认框的业务区域本体；公共区域规范只作为视觉 token 参考。
- 2026-06-28 业务合理性复核：已基于 `screens/` contact sheet 检查 pages、overlays、components、states、responsive，并返工 28 张存在业务语义不足的图片：`screens/states/` 16 张和 `screens/responsive/` 12 张。详见 `doc/04_assets/ui_suite_gpt_v1/BUSINESS_REASONABILITY_AUDIT.md`。

已完成图片：

| 序号 | ID | 类型 | 文件 |
|---:|---|---|---|
| 1 | `foundation-visual-reference` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-visual-reference.png` |
| 2 | `foundation-layout-grid` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-layout-grid.png` |
| 3 | `foundation-color-status` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png` |
| 4 | `foundation-typography-density` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-typography-density.png` |
| 5 | `foundation-icons-actions` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-icons-actions.png` |
| 6 | `foundation-data-viz` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-data-viz.png` |
| 7 | `foundation-table-form` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-table-form.png` |
| 8 | `foundation-responsive` | foundation | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-responsive.png` |
| 9 | `login` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/login.png` |
| 10 | `screen` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png` |
| 11 | `dashboard` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png` |
| 12 | `alerts` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png` |
| 13 | `alert-detail` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.png` |
| 14 | `campaigns` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/campaigns.png` |
| 15 | `campaign-detail` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail.png` |
| 16 | `attack-chains` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.png` |
| 17 | `encrypted-traffic` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png` |
| 18 | `forensics` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.png` |
| 19 | `assets` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.png` |
| 20 | `graph` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/graph.png` |
| 21 | `fusion` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png` |
| 22 | `baselines` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/baselines.png` |
| 23 | `probes` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.png` |
| 24 | `rules` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/rules.png` |
| 25 | `deployments` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.png` |
| 26 | `models` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/models.png` |
| 27 | `mlops` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png` |
| 28 | `data-quality` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png` |
| 29 | `playbooks` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/playbooks.png` |
| 30 | `whitelist` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/whitelist.png` |
| 31 | `compliance` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/compliance.png` |
| 32 | `audit-log` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/audit-log.png` |
| 33 | `notifications` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.png` |
| 34 | `settings` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.png` |
| 35 | `not-found` | page | `doc/04_assets/ui_suite_gpt_v1/screens/pages/not-found.png` |
| 36 | `dropdown-user-menu` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-user-menu.png` |
| 37 | `drawer-mobile-navigation` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mobile-navigation.png` |
| 38 | `drawer-notification-center` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-center.png` |
| 39 | `modal-global-search` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-global-search.png` |
| 40 | `dropdown-quick-entry` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-quick-entry.png` |
| 41 | `modal-login-error-captcha` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-login-error-captcha.png` |
| 42 | `drawer-dashboard-kpi-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-kpi-detail.png` |
| 43 | `drawer-dashboard-task-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-task-detail.png` |
| 44 | `modal-screen-readonly-token` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-screen-readonly-token.png` |
| 45 | `drawer-probe-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-detail.png` |
| 46 | `modal-probe-config` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-config.png` |
| 47 | `modal-probe-batch-upgrade` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-batch-upgrade.png` |
| 48 | `modal-probe-cert-rotate` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-cert-rotate.png` |
| 49 | `drawer-probe-log` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-log.png` |
| 50 | `drawer-dlq-sample` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dlq-sample.png` |
| 51 | `modal-data-replay-task` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-data-replay-task.png` |
| 52 | `drawer-field-quality-sample` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-field-quality-sample.png` |
| 53 | `modal-alert-batch` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-batch.png` |
| 54 | `dropdown-alert-batch-actions` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-batch-actions.png` |
| 55 | `dropdown-alert-row-actions` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-row-actions.png` |
| 56 | `modal-alert-status` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-status.png` |
| 57 | `modal-alert-feedback` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-feedback.png` |
| 58 | `modal-evidence-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-evidence-detail.png` |
| 59 | `modal-playbook-trigger` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-playbook-trigger.png` |
| 60 | `modal-whitelist-draft-from-alert` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-whitelist-draft-from-alert.png` |
| 61 | `popconfirm-pcap-download` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-pcap-download.png` |
| 62 | `drawer-session-replay` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-session-replay.png` |
| 63 | `drawer-asset-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-asset-detail.png` |
| 64 | `modal-asset-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-asset-edit.png` |
| 65 | `drawer-asset-history` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-asset-history.png` |
| 66 | `drawer-graph-entity` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-graph-entity.png` |
| 67 | `drawer-graph-path-analysis` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-graph-path-analysis.png` |
| 68 | `drawer-fusion-conflict` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-fusion-conflict.png` |
| 69 | `modal-fusion-rule-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-fusion-rule-edit.png` |
| 70 | `modal-baseline-threshold` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-baseline-threshold.png` |
| 71 | `modal-forensics-task` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-forensics-task.png` |
| 72 | `drawer-campaign-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-campaign-detail.png` |
| 73 | `drawer-attack-chain-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-attack-chain-detail.png` |
| 74 | `drawer-encrypted-fingerprint` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-encrypted-fingerprint.png` |
| 75 | `drawer-certificate-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-certificate-detail.png` |
| 76 | `modal-rule-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-rule-edit.png` |
| 77 | `drawer-rule-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-rule-detail.png` |
| 78 | `popconfirm-delete` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-delete.png` |
| 79 | `modal-rule-publish` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-rule-publish.png` |
| 80 | `modal-deployment-create` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-create.png` |
| 81 | `modal-deployment-rollback` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-rollback.png` |
| 82 | `drawer-model-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-model-detail.png` |
| 83 | `drawer-mlops-task-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mlops-task-detail.png` |
| 84 | `modal-playbook-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-playbook-edit.png` |
| 85 | `modal-whitelist-add` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-whitelist-add.png` |
| 86 | `drawer-whitelist-approval` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-whitelist-approval.png` |
| 87 | `modal-settings-token` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-settings-token.png` |
| 88 | `component-app-header` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-app-header.png` |
| 89 | `component-primary-sidebar` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-primary-sidebar.png` |
| 90 | `component-secondary-menu` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-secondary-menu.png` |
| 91 | `component-bottom-status-bar` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-bottom-status-bar.png` |
| 92 | `component-breadcrumb-context` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-breadcrumb-context.png` |
| 93 | `component-site-time-selector` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-site-time-selector.png` |
| 94 | `component-quick-entry` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-quick-entry.png` |
| 95 | `component-user-menu` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-user-menu.png` |
| 96 | `component-button` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-button.png` |
| 97 | `component-icon-button` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-icon-button.png` |
| 98 | `component-status-chip` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-status-chip.png` |
| 99 | `component-tooltip` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-tooltip.png` |
| 100 | `component-tabs` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-tabs.png` |
| 101 | `component-segmented` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-segmented.png` |
| 102 | `component-dropdown` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-dropdown.png` |
| 103 | `component-pagination` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-pagination.png` |
| 104 | `component-input` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-input.png` |
| 105 | `component-search` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-search.png` |
| 106 | `component-select` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-select.png` |
| 107 | `component-date-range` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-date-range.png` |
| 108 | `component-switch-checkbox-radio` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-switch-checkbox-radio.png` |
| 109 | `component-condition-builder` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-condition-builder.png` |
| 110 | `component-batch-action-bar` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-batch-action-bar.png` |
| 111 | `component-data-table` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-data-table.png` |
| 112 | `component-description-list` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-description-list.png` |
| 113 | `component-kpi-tile` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-kpi-tile.png` |
| 114 | `component-health-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-health-card.png` |
| 115 | `component-ranking-list` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-ranking-list.png` |
| 116 | `component-log-list` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-log-list.png` |
| 117 | `component-evidence-file-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-file-card.png` |
| 118 | `component-empty-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-empty-card.png` |
| 119 | `component-permission-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-permission-card.png` |
| 120 | `component-line-area-chart` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-line-area-chart.png` |
| 121 | `component-donut-chart` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-donut-chart.png` |
| 122 | `component-bar-ranking-chart` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-bar-ranking-chart.png` |
| 123 | `component-sankey-flow` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-sankey-flow.png` |
| 124 | `component-radar-quality` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-radar-quality.png` |
| 125 | `component-heatmap` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-heatmap.png` |
| 126 | `component-topology-graph` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-topology-graph.png` |
| 127 | `component-timeline-state-machine` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-timeline-state-machine.png` |
| 128 | `component-alert-queue` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-queue.png` |
| 129 | `component-risk-score` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-risk-score.png` |
| 130 | `component-alert-timeline` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-timeline.png` |
| 131 | `component-evidence-drawer` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-drawer.png` |
| 132 | `component-asset-context` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-asset-context.png` |
| 133 | `component-action-rail` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-action-rail.png` |
| 134 | `component-feedback-block` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-feedback-block.png` |
| 135 | `component-acceptance-gate-matrix` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-acceptance-gate-matrix.png` |
| 136 | `state-page-loading` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-page-loading.png` |
| 137 | `state-table-loading` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-table-loading.png` |
| 138 | `state-chart-loading` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-chart-loading.png` |
| 139 | `state-empty-page` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-page.png` |
| 140 | `state-empty-table` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-table.png` |
| 141 | `state-empty-chart` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-chart.png` |
| 142 | `state-api-error` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-api-error.png` |
| 143 | `state-network-error` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-network-error.png` |
| 144 | `state-unauthorized` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-unauthorized.png` |
| 145 | `state-forbidden` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-forbidden.png` |
| 146 | `state-partial-degraded` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-partial-degraded.png` |
| 147 | `state-offline-probe` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-offline-probe.png` |
| 148 | `state-stream-backpressure` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-stream-backpressure.png` |
| 149 | `state-task-running` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-task-running.png` |
| 150 | `state-task-failed` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-task-failed.png` |
| 151 | `state-success-accepted` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-success-accepted.png` |
| 152 | `responsive-dashboard-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-dashboard-1440.png` |
| 153 | `responsive-dashboard-1920` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-dashboard-1920.png` |
| 154 | `responsive-screen-4k` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-screen-4k.png` |
| 155 | `responsive-alerts-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1440.png` |
| 156 | `responsive-alerts-1920` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1920.png` |
| 157 | `responsive-forensics-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-forensics-1440.png` |
| 158 | `responsive-graph-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-graph-1440.png` |
| 159 | `responsive-compliance-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-compliance-1440.png` |
| 160 | `responsive-tablet-dashboard` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-tablet-dashboard.png` |
| 161 | `responsive-tablet-alert-detail` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-tablet-alert-detail.png` |
| 162 | `responsive-mobile-navigation` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-mobile-navigation.png` |
| 163 | `responsive-mobile-alert-list` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-mobile-alert-list.png` |
| 164 | `modal-topic-save-view` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-save-view.png` |
| 165 | `drawer-topic-scope-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-topic-scope-edit.png` |
| 166 | `modal-topic-report-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-report-export.png` |
| 167 | `modal-topic-evidence-package-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-evidence-package-export.png` |
| 168 | `drawer-topic-subscription` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-topic-subscription.png` |
| 169 | `dropdown-topic-share-favorite` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-topic-share-favorite.png` |
| 170 | `modal-campaign-report-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-campaign-report-export.png` |
| 171 | `modal-forensics-evidence-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-forensics-evidence-export.png` |
| 172 | `drawer-compliance-gate-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-compliance-gate-detail.png` |
| 173 | `modal-compliance-evidence-package-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-compliance-evidence-package-export.png` |
| 174 | `modal-compliance-report-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-compliance-report-export.png` |
| 175 | `drawer-audit-operation-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-audit-operation-detail.png` |
| 176 | `modal-audit-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-audit-export.png` |
| 177 | `modal-notification-channel-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-notification-channel-edit.png` |
| 178 | `modal-notification-template-preview-test` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-notification-template-preview-test.png` |
| 179 | `drawer-notification-silence-rule` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-silence-rule.png` |
| 180 | `popconfirm-settings-token-revoke` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-settings-token-revoke.png` |
| 181 | `drawer-settings-rbac-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-settings-rbac-edit.png` |

所有已完成交付图均为 `1920x1080` PNG。`*.raw-imagegen.png` 是内置 imagegen 原始尺寸产物，`*.raw-deterministic.png` 是确定性绘制/裁切产物，均保留用于追溯。

## 本轮返工记录

- 2026-06-27：按用户新口径生成 `drawer-mobile-navigation`。本图为独立移动端侧滑导航抽屉，只包含业务交互容器本体，不含桌面 AppShell、顶部栏、左侧菜单、底部栏、宿主页面背景、手机外壳或浏览器框；内容包括站点/时间/在线状态、菜单搜索、一级菜单与展开的 `综合态势` 二级项、快捷入口、运行状态和底部用户动作。最终图已提取到 `screens/overlays/drawer-mobile-navigation.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-notification-center`。
- 2026-06-27：继续按新口径生成 `drawer-notification-center`。本图为独立桌面端通知中心抽屉，只包含通知流业务容器本体，不含宿主页面；内容包括未读/高危/待处理/系统/归档筛选、通知分组、告警升级、处置反馈、系统健康、审计提醒、模型门禁和底部审计留痕。最终图已提取到 `screens/overlays/drawer-notification-center.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-global-search`。
- 2026-06-27：继续按新口径生成 `modal-global-search`。本图为独立全局搜索弹窗，只包含 Command Search 模态框本体，不含宿主页面；内容包括跨对象筛选、C2 查询结果、资产/PCAP/Session/规则/审计结果、命令建议、最近访问、受限结果和审计提示。最终图已提取到 `screens/overlays/modal-global-search.png` 并标准化为 `1920x1080`；下一张常规项为 `dropdown-quick-entry`。
- 2026-06-27：继续按新口径生成 `dropdown-quick-entry`。本图为独立快速入口下拉，只包含快捷入口业务容器本体，不含宿主页面；内容包括 PCAP/资产/规则/脚本/帮助/更多应用入口、最近动作、受限入口和审计提示。最终图已提取到 `screens/overlays/dropdown-quick-entry.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-login-error-captcha`。
- 2026-06-27：继续按新口径生成 `modal-login-error-captcha`。本图为独立登录安全校验弹窗，只包含验证码错误/登录验证业务容器本体，不含完整登录页；内容保持低暴露，仅展示脱敏账号、验证码、重试提示、SSO/OIDC 入口、脱敏审计编号和安全提示。最终图已提取到 `screens/overlays/modal-login-error-captcha.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-dashboard-kpi-detail`。
- 2026-06-27：继续按新口径生成 `drawer-dashboard-kpi-detail`。本图为独立仪表盘 KPI 详情抽屉，只包含告警处理 SLA 指标业务容器本体，不含完整仪表盘；内容包括指标解释、趋势分析、分解构成、影响范围、下钻动作和审计留痕，且不展示责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单或个人任务归属。最终图已提取到 `screens/overlays/drawer-dashboard-kpi-detail.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-dashboard-task-detail`。
- 2026-06-27：继续按新口径生成 `drawer-dashboard-task-detail`。本图为独立仪表盘待办任务详情抽屉，只包含证据缺口复核任务业务容器本体，不含完整仪表盘；内容包括任务摘要、处置阶段、关联对象、证据缺口、操作建议、风险影响和审计留痕，且不展示责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单或个人任务归属。最终图已提取到 `screens/overlays/drawer-dashboard-task-detail.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-screen-readonly-token`。
- 2026-06-27：继续按新口径生成 `modal-screen-readonly-token`。本图为独立态势大屏只读访问令牌/脱敏配置弹窗，只包含令牌生成业务容器本体，不含完整态势大屏；内容包括访问范围、脱敏配置、有效期、权限边界、脱敏令牌预览、状态提示和审计留痕，且不展示真实令牌、完整签名 URL、完整 trace_id、内部 API 路径、内部服务名、拓扑细节或资产明细。最终图已提取到 `screens/overlays/modal-screen-readonly-token.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-probe-detail`。
- 2026-06-27：继续按新口径生成 `drawer-probe-detail`。本图为独立探针详情抽屉，只包含 probe-07 采集探针业务容器本体，不含完整探针管理页面；内容包括基础信息、心跳与链路、采集吞吐、接口与丢包、证书与配置、最近日志、操作与审计。最终图已提取到 `screens/overlays/drawer-probe-detail.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-probe-config`。
- 2026-06-27：继续按新口径生成 `modal-probe-config`。本图为独立探针配置下发弹窗，只包含配置下发业务容器本体，不含完整探针管理页面；内容包括目标范围、配置变更对比、下发策略、预检查、影响范围、回滚与审计，确认下发在预检查通过前保持门禁状态。最终图已提取到 `screens/overlays/modal-probe-config.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-probe-batch-upgrade`。
- 2026-06-27：继续按新口径生成 `modal-probe-batch-upgrade`。本图为独立探针批量升级确认弹窗，只包含升级业务容器本体，不含完整探针管理页面；内容包括升级范围、版本变更、分批策略、预检查结果、影响范围、回滚与审计，确认升级在审批和预检查通过前保持门禁状态。最终图已提取到 `screens/overlays/modal-probe-batch-upgrade.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-probe-cert-rotate`。
- 2026-06-27：继续按新口径生成 `modal-probe-cert-rotate`。本图为独立探针证书轮换弹窗，只包含证书轮换业务容器本体，不含完整探针管理页面；内容包括证书范围、当前证书、新证书、预检查结果、轮换策略、影响范围、回滚与审计，确认轮换在预检查与审批门禁通过前保持置灰状态，且不展示私钥、完整证书或完整指纹。最终图已提取到 `screens/overlays/modal-probe-cert-rotate.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-probe-log`。
- 2026-06-27：继续按新口径生成 `drawer-probe-log`。本图为独立探针日志抽屉，只包含日志业务容器本体，不含完整探针管理页面；内容包括探针状态、日志筛选、脱敏日志表、事件状态解释、建议动作和审计留痕，日志中的 trace_id 等敏感字段均按脱敏口径展示。最终图已提取到 `screens/overlays/drawer-probe-log.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-dlq-sample`。
- 2026-06-27：继续按新口径生成 `drawer-dlq-sample`。本图为独立 DLQ 样例详情抽屉，只包含 DLQ 诊断业务容器本体，不含完整数据质量页面；内容包括核心摘要、诊断链路、脱敏样本对比、影响评估、当前解释、重放门禁动作和审计留痕，不展示完整原始 payload 或可复用凭据。最终图已提取到 `screens/overlays/drawer-dlq-sample.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-data-replay-task`。
- 2026-06-27：继续按新口径生成 `modal-data-replay-task`。本图为独立数据重放任务弹窗，只包含重放任务业务容器本体，不含完整数据质量页面；内容包括重放范围、门禁校验、重放策略、影响评估、样本预览、审计与权限，提交重放在校验通过前置灰。最终图已提取到 `screens/overlays/modal-data-replay-task.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-field-quality-sample`。
- 2026-06-27：继续按新口径生成 `drawer-field-quality-sample`。本图为独立字段质量样例抽屉，只包含字段质量诊断业务容器本体，不含完整数据质量页面；内容包括字段画像、异常分布、样本对比、下游影响、修复建议和审计留痕，字段样本均以脱敏 event_id 和建议值展示。最终图已提取到 `screens/overlays/drawer-field-quality-sample.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-alert-batch`。
- 2026-06-27：继续按新口径生成 `modal-alert-batch`。本图为独立告警批量操作确认弹窗，只包含批量操作确认业务容器本体，不含完整告警中心页面；内容包括操作类型、状态变更预览、选择范围、影响提示、选中告警预览、权限审计、备注和影响检查门禁，确认按钮在备注与检查未完成前置灰。最终图已提取到 `screens/overlays/modal-alert-batch.png` 并标准化为 `1920x1080`；下一张常规项为 `dropdown-alert-batch-actions`。
- 2026-06-27：继续按新口径生成 `dropdown-alert-batch-actions`。本图为独立告警批量操作下拉菜单，只包含批量操作业务容器本体，不含完整告警中心页面；内容包括状态处理、证据与联动、例外与忽略三组动作，以及高危/需审批/脱敏/审计等风险标签。最终图已提取到 `screens/overlays/dropdown-alert-batch-actions.png` 并标准化为 `1920x1080`；下一张常规项为 `dropdown-alert-row-actions`。
- 2026-06-27：继续按新口径生成 `dropdown-alert-row-actions`。本图为独立告警行操作下拉菜单，只包含单条告警操作业务容器本体，不含完整告警中心页面；内容包括查看与证据、处置动作、例外与反馈三组动作，以及高危、需权限、需审批、高风险和审计提示。最终图已提取到 `screens/overlays/dropdown-alert-row-actions.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-alert-status`。
- 2026-06-27：继续按新口径生成 `modal-alert-status`。本图为独立更新告警状态弹窗，只包含状态变更业务容器本体，不含完整告警详情页面；内容包括状态机、证据检查、处置备注、影响联动、权限审计和状态检查门禁，确认更新在备注与检查未完成前置灰。最终图已提取到 `screens/overlays/modal-alert-status.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-alert-feedback`。
- 2026-06-27：继续按新口径生成 `modal-alert-feedback`。本图为独立提交告警反馈弹窗，只包含反馈闭环业务容器本体，不含完整告警详情页面；内容包括反馈标签、置信度、证据引用、回流范围、样本标注、说明输入、门禁审计和反馈校验，提交反馈在说明与校验未完成前置灰。最终图已提取到 `screens/overlays/modal-alert-feedback.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-evidence-detail`。
- 2026-06-27：继续按新口径生成 `modal-evidence-detail`。本图为独立证据详情弹窗，只包含证据详情业务容器本体，不含完整告警详情页面；内容包括证据属性、脱敏十六进制预览、链路完整性、操作区、权限审计和下载授权提示，签名 URL 未生成以黄色状态表达。最终图已提取到 `screens/overlays/modal-evidence-detail.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-playbook-trigger`。
- 2026-06-27：继续按新口径生成 `modal-playbook-trigger`。本图为独立触发 SOAR 剧本弹窗，只包含剧本触发业务容器本体，不含完整告警详情页面；内容包括剧本选择、节点预览、参数映射、影响与回滚、门禁检查、审计与执行按钮，触发剧本在审批通过前置灰。最终图已提取到 `screens/overlays/modal-playbook-trigger.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-whitelist-draft-from-alert`。
- 2026-06-27：继续按新口径生成 `modal-whitelist-draft-from-alert`。本图为独立从告警生成白名单草案弹窗，只包含草案业务容器本体，不含完整告警详情页面；内容包括告警来源、条件提取、生效范围、例外原因、影响评估、审批门禁、状态解释、审计留痕和审批前影响评估提示。最终图已提取到 `screens/overlays/modal-whitelist-draft-from-alert.png` 并标准化为 `1920x1080`；下一张常规项为 `popconfirm-pcap-download`。
- 2026-06-27：继续按新口径生成 `popconfirm-pcap-download`。本图为独立 PCAP 下载确认浮层，只包含下载二次确认业务容器本体，不含完整取证分析页面；内容包括下载对象、下载前检查、影响与风险、授权选项、审计留痕和签名 URL 生成前置灰确认下载。最终图已提取到 `screens/overlays/popconfirm-pcap-download.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-session-replay`。
- 2026-06-27：继续按新口径生成 `drawer-session-replay`。本图为独立会话复放抽屉，只包含 Session 复放业务容器本体，不含完整取证分析页面；内容包括会话摘要、复放控制、会话时间线、协议解码与载荷摘要、风险定位、证据关联、审计留痕和导出权限提示。最终图已提取到 `screens/overlays/drawer-session-replay.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-asset-detail`。
- 2026-06-27：继续按新口径生成并按用户反馈重生 `drawer-asset-detail`。本图为独立资产详情抽屉父容器，只包含选中资产摘要、风险概览、详情小 Tab 导航、当前 Tab 轻摘要、关键入口、关联状态、图谱预览、操作门禁和审计留痕；不含完整资产台账页面、资产分类大 Tab，也不展开基础信息、网络接口、开放服务、归属信息或历史变更小 Tab 的完整表格内容。最终图已覆盖到 `screens/overlays/drawer-asset-detail.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-asset-edit`。
- 2026-06-27：继续按新口径生成 `modal-asset-edit`。本图为独立资产编辑弹窗，只包含编辑资产 Modal 业务容器本体，不含完整资产台账页面、公共 AppShell、宿主背景或资产详情父容器；内容包括资产摘要、基础/归属/标签/业务系统/园区网段/维护窗口表单、变更 Diff 表、影响范围概览、变更门禁状态、审计信息和底部取消/保存草稿/提交审批/确认更新动作，确认更新在门禁未完成时锁定。最终图已提取到 `screens/overlays/modal-asset-edit.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-asset-history`。
- 2026-06-27：继续按新口径生成 `drawer-asset-history`。本图为独立资产历史抽屉，只包含资产历史业务容器本体，不含完整资产台账页面、公共 AppShell、宿主背景或 `assets-detail-history` 小 Tab 内容页；内容包括资产摘要、变更时间线、字段 Diff、来源证据、影响范围、关联告警、图谱邻居、可回滚项、审批门禁和审计留痕。最终图已提取到 `screens/overlays/drawer-asset-history.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-graph-entity`。
- 2026-06-27：继续按新口径生成 `drawer-graph-entity`。本图为独立图谱实体详情抽屉，只包含实体详情业务容器本体，不含完整实体图谱页面、公共 AppShell、宿主图谱画布或 `graph-*` 小 Tab 结果页；内容包括实体画像、局部关系预览、关联边、最近会话、命中规则、关联告警、攻击路径入口、影响范围、权限门禁和审计记录。最终图已提取到 `screens/overlays/drawer-graph-entity.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-graph-path-analysis`。
- 2026-06-27：继续按新口径生成 `drawer-graph-path-analysis`。本图为独立图谱路径分析抽屉，只包含路径分析业务容器本体，不含完整实体图谱页面、公共 AppShell、宿主图谱画布或 `graph-*` 小 Tab 结果页；内容包括路径候选、当前路径链路预览、边证据详情、影响范围评估、关联告警、可导出证据、权限门禁和下一步建议。最终图已提取到 `screens/overlays/drawer-graph-path-analysis.png` 并标准化为 `1920x1080`；下一张常规项为 `drawer-fusion-conflict`。
- 2026-06-27：继续按新口径生成 `drawer-fusion-conflict`。本图为独立数据融合冲突处理抽屉，只包含冲突处理业务容器本体，不含完整数据融合页面、公共 AppShell、宿主背景或融合规则编辑弹窗；内容包括冲突摘要、多源可信度、冲突处理流程、字段候选对比、关联证据、决策建议、影响范围、审计策略和审批门禁。最终图已提取到 `screens/overlays/drawer-fusion-conflict.png` 并标准化为 `1920x1080`；下一张常规项为 `modal-fusion-rule-edit`。
- 2026-06-27：按“每 2 张后压缩上下文成为文档”的新节奏完成一批 overlay：`modal-fusion-rule-edit` 和 `modal-baseline-threshold`。两张图均只包含业务 Modal 容器本体，不含完整宿主页面、顶部栏、左侧菜单或底部栏；分别覆盖融合规则编辑和基线阈值编辑的权限、影响范围、状态解释、下一步动作和审计留痕。最终图已提取到 `screens/overlays/modal-fusion-rule-edit.png`、`screens/overlays/modal-baseline-threshold.png` 并标准化为 `1920x1080`；下一批按 manifest 当前首个缺口回补，下一张常规项为 `modal-forensics-task`。
- 2026-06-27：按两张一批继续完成 `modal-forensics-task` 和 `drawer-campaign-detail`。`modal-forensics-task` 只包含取证任务详情 Modal 本体，覆盖切片范围、证据输出、存储位置、处理状态、失败重试、权限和审计；`drawer-campaign-detail` 只包含战役详情 Drawer 本体，覆盖聚类原因、攻击阶段、证据、影响范围、处置建议和审计门禁。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `drawer-attack-chain-detail`、`drawer-encrypted-fingerprint`。
- 2026-06-27：按两张一批继续完成 `drawer-attack-chain-detail` 和 `drawer-encrypted-fingerprint`。`drawer-attack-chain-detail` 只包含攻击链详情 Drawer 本体，覆盖节点上下文、证据、命中规则、关联资产和下一步调查；`drawer-encrypted-fingerprint` 只包含加密指纹详情 Drawer 本体，覆盖 TLS/JA3/JA4 指纹、证书链、相似样本、风险解释、影响范围和审计动作。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `drawer-certificate-detail`、`modal-rule-edit`。
- 2026-06-27：按两张一批继续完成 `drawer-certificate-detail` 和 `modal-rule-edit`。`drawer-certificate-detail` 只包含证书详情 Drawer 本体，覆盖证书链、风险检查、相似样本、影响范围、动作和审计；`modal-rule-edit` 只包含规则编辑 Modal 本体，覆盖规则 DSL、测试门禁、影响范围、版本 Diff、审批和审计。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `drawer-rule-detail`、`popconfirm-delete`。
- 2026-06-28：按两张一批继续完成 `drawer-rule-detail` 和 `popconfirm-delete`。`drawer-rule-detail` 只包含规则详情 Drawer 本体，覆盖规则生命周期、版本历史、命中趋势、发布记录、关联模型、影响范围和审计；`popconfirm-delete` 只包含规则删除确认 Popconfirm 本体，覆盖规则名、影响范围、删除原因、权限提示、下一步动作和审计留痕。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `modal-rule-publish`、`modal-deployment-create`。
- 2026-06-28：按两张一批继续完成 `modal-rule-publish` 和 `modal-deployment-create`。`modal-rule-publish` 只包含规则发布 Modal 本体，覆盖发布配置、灰度范围、发布前检查、影响预估、审批链和回滚策略；`modal-deployment-create` 只包含创建部署 Modal 本体，覆盖能力包版本、目标部署集、灰度策略、预检查、回滚和审计。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `modal-deployment-rollback`、`drawer-model-detail`。
- 2026-06-28：按两张一批继续完成 `modal-deployment-rollback` 和 `drawer-model-detail`。`modal-deployment-rollback` 只包含部署回滚 Modal 本体，覆盖目标版本、回滚范围、回滚前检查、影响范围、观测窗口、审批和审计；`drawer-model-detail` 只包含模型详情 Drawer 本体，覆盖模型版本、评估指标、特征解释、激活状态、影响范围、回滚和审计。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `drawer-mlops-task-detail`、`modal-playbook-edit`。
- 2026-06-28：按两张一批继续完成 `drawer-mlops-task-detail` 和 `modal-playbook-edit`。`drawer-mlops-task-detail` 只包含 MLOps 任务详情 Drawer 本体，覆盖任务 DAG、阶段状态、指标、日志、产物、失败重试、发布门禁和审计；`modal-playbook-edit` 只包含 SOAR 剧本编辑 Modal 本体，覆盖节点编排、参数映射、风险控制、测试验证、影响范围、审批和审计。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一批为 `modal-whitelist-add`、`drawer-whitelist-approval`。
- 2026-06-28：按两张一批继续完成 `modal-whitelist-add` 和 `drawer-whitelist-approval`。`modal-whitelist-add` 只包含新增白名单 Modal 本体，覆盖条件构造、生效策略、风险评估、影响范围、审批链和审计；`drawer-whitelist-approval` 只包含白名单审批详情 Drawer 本体，覆盖审批流程、命中证据、风险解释、到期治理、审批动作和审计。两张最终图已提取到 `screens/overlays/` 并标准化为 `1920x1080`；下一张为 overlay 队列最后一张 `modal-settings-token`。
- 2026-06-28：完成 overlay 队列最后一张 `modal-settings-token`。本图只包含 API 令牌管理 Modal 本体，覆盖令牌配置、脱敏 token 展示、权限 scope、有效期、IP 白名单、轮换/吊销风险和审计；未展示真实可用密钥。最终图已提取到 `screens/overlays/modal-settings-token.png` 并标准化为 `1920x1080`。至此 overlay 52/52 全部完成；下一阶段从 component 队列 `component-app-header` 开始。
- 2026-06-28：进入 component 队列，按两张一批完成 `component-app-header` 和 `component-primary-sidebar`。`component-app-header` 为顶部状态栏组件规范板，覆盖产品标题、站点/时间、风险态势、告警数、采集健康、数据质量、快捷入口和状态变体；`component-primary-sidebar` 为左侧一级导航组件规范板，覆盖单栏展开式菜单、一级域、二级菜单、激活/悬停/禁用/告警态、底部用户区和尺寸标注。两张最终图已提取到 `screens/components/` 并标准化为 `1920x1080`；下一批为 `component-secondary-menu`、`component-bottom-status-bar`。
- 2026-06-28：按当前 `screen.png` 和已修复 foundations 返工 `component-app-header`、`component-primary-sidebar`。确认顶部头部的通知/用户动作组会与左侧底部用户区、底部右侧全局动作区重复，因此顶部状态栏只保留产品标题、站点/时间、风险态势、告警总数、关键告警、采集健康、数据质量和六个快捷入口；用户身份/角色/在线状态只归属左侧底部用户区，通知角标/设置/全局配置/电源只归属底部状态栏右侧。两张最终 PNG 已改为从 `screen.png` 裁切重组的确定性规格板，原 imagegen 版本备份为 `*.before-duplication-fix.png`。
- 2026-06-28：按两张一批完成 `component-secondary-menu` 和 `component-bottom-status-bar`。两张均以当前 `screen.png` 为硬基准做确定性裁切重组：`component-secondary-menu` 明确二级菜单与一级域同处 166px 左侧单栏，禁止独立二级栏、第三层菜单和业务模块塞入菜单；`component-bottom-status-bar` 明确底部单栏固定 y=997 / h=83，固定顺序为数据延迟、系统运行、告警处理SLA、数据质量合格率、存储使用、带宽使用、日志吞吐、右侧全局动作区，通知角标/设置/全局配置/电源不得上移到顶部。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-breadcrumb-context`、`component-site-time-selector`。
- 2026-06-28：按两张一批完成 `component-breadcrumb-context` 和 `component-site-time-selector`。两张均以当前 `screen.png` 和 foundations token 做确定性绘制：`component-breadcrumb-context` 定位为业务内容区顶部上下文导航，只解释业务域、列表页、详情对象、脱敏对象 ID 和继承筛选，不得进入顶部状态栏、不得替代左侧菜单、不得承载用户/通知或危险提交；`component-site-time-selector` 定位为顶部 80px 状态栏内的站点与时间模块，只承载站点、当前时间、时间窗、刷新/NTP 状态，不得混入通知、用户、设置、电源或页面业务筛选。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-quick-entry`、`component-user-menu`。
- 2026-06-28：按两张一批完成 `component-quick-entry` 和 `component-user-menu`。两张均以当前 `screen.png` 和 foundations token 做确定性绘制：`component-quick-entry` 只解释顶部 80px 状态栏右侧 `PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用` 六个快捷入口，禁止混入通知、用户、设置、全局配置或电源；`component-user-menu` 只解释左侧底部用户卡及其右向弹出菜单，顶部不得重复用户头像、用户名、用户组或个人菜单。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-button`、`component-icon-button`。
- 2026-06-28：按两张一批完成 `component-button` 和 `component-icon-button`。两张均为基础控件组件板，不绘制完整 AppShell，只使用 foundations token 确定性绘制：`component-button` 覆盖主按钮、次按钮、幽灵按钮、文本按钮、危险按钮、审批/门禁按钮以及正常、悬停、按下、禁用、加载、成功、警告、危险状态；`component-icon-button` 覆盖取证、详情、编辑、复制、下载、刷新、筛选、定位、展开、折叠、重试、删除、回滚、设置、帮助等图标按钮，并强调 tooltip、权限锁定和危险动作二次确认。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-status-chip`、`component-tooltip`。
- 2026-06-28：按两张一批完成 `component-status-chip` 和 `component-tooltip`。两张均为基础控件组件板，不绘制完整 AppShell，只使用 foundations token 确定性绘制：`component-status-chip` 覆盖状态标签、风险 Badge、计数徽标、业务对象 Tag、禁用/加载/选中/关闭态和颜色语义锁定；`component-tooltip` 覆盖 placement、字段解释、权限/风险/校验提示、Popover 边界、disabled wrapper 和危险确认转 Popconfirm/Modal 的规则。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-tabs`、`component-segmented`。
- 2026-06-28：按两张一批完成 `component-tabs` 和 `component-segmented`。两张均为基础控件组件板，不绘制完整 AppShell，只使用 foundations token 确定性绘制：`component-tabs` 覆盖横向 Tabs、Card Tabs、业务详情小 Tab、Badge、禁用/加载/错误态、稳定内容区和不新增路由/左侧菜单边界；`component-segmented` 覆盖专题模式、时间粒度、视图密度、风险级别、状态矩阵和同一区域轻量互斥切换边界。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-dropdown`、`component-pagination`。
- 2026-06-28：按两张一批完成 `component-dropdown` 和 `component-pagination`。两张均为基础控件组件板，不绘制完整 AppShell，只使用 foundations token 确定性绘制：`component-dropdown` 覆盖行操作、批量操作、快速入口、分组/二级菜单、禁用/加载/危险态和危险动作确认边界；`component-pagination` 覆盖基础分页、表格底部分页、pageSize、跳页、服务端分页、游标分页、加载/禁用/错误态和大数据量性能提示。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-input`、`component-search`。
- 2026-06-28：按两张一批完成 `component-input` 和 `component-search`。两张均为基础控件组件板，不绘制完整 AppShell，只使用 foundations token 和 Noto Sans CJK 字体确定性绘制：`component-input` 覆盖 Input、InputNumber、Password、TextArea、校验、前后缀、脱敏、单位和危险变更审计边界；`component-search` 覆盖本地/服务端/实体/审计/PCAP 搜索、建议层、筛选 chip、查询状态矩阵和脱敏命中结果。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-select`、`component-date-range`。
- 2026-06-28：按两张一批完成 `component-select` 和 `component-date-range`。两张均为基础控件组件板，不绘制完整 AppShell，只使用 foundations token 和 Noto Sans CJK 字体确定性绘制：`component-select` 覆盖单选、多选、分组、远程搜索、长列表、受限选项、状态矩阵和高影响选择审计边界；`component-date-range` 覆盖绝对/相对时间、快捷窗口、双月面板、状态校验、业务时间窗、高成本查询预估和权限审计门禁。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批为 `component-switch-checkbox-radio`、`component-condition-builder`。
- 2026-06-28：按两张一批完成 `component-switch-checkbox-radio` 和 `component-condition-builder`。两张均为基础表单组件板，不绘制完整 AppShell，只使用 foundations token 和 Noto Sans CJK 字体确定性绘制：`component-switch-checkbox-radio` 覆盖 Switch、Checkbox、Checkbox.Group、Radio、Radio.Group、半选树、互斥策略、状态矩阵和高影响开关审计边界；`component-condition-builder` 覆盖条件组、AND/OR 逻辑、字段类型、嵌套条件、拖拽编辑、命中预估、状态校验和权限审计门禁。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批从 `component-batch-action-bar`、`component-data-table` 开始。
- 2026-06-28：按两张一批完成 `component-batch-action-bar` 和 `component-data-table`。两张均为基础数据操作组件板，不绘制完整 AppShell，只使用 foundations token 和 Noto Sans CJK 字体确定性绘制：`component-batch-action-bar` 覆盖选中计数、跨页全选、筛选范围快照、排除项、动作分组、权限锁定、危险批量动作确认和审计门禁；`component-data-table` 覆盖高密度表格、固定表头/列、排序筛选、行选择、展开行、行状态、虚拟滚动、服务端分页、加载/空态/错误/权限降级和 React 映射边界。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批从 `component-description-list`、`component-kpi-tile` 开始。
- 2026-06-28：按两张一批完成 `component-description-list` 和 `component-kpi-tile`。两张均为基础数据展示组件板，不绘制完整 AppShell，只使用 foundations token 和 Noto Sans CJK 字体确定性绘制：`component-description-list` 覆盖 Ant Design Descriptions、详情键值、分组、脱敏、复制、权限锁定、字段级错误和审计 trace_id；`component-kpi-tile` 覆盖指标结构、主数值、单位、趋势 sparkline、阈值、状态矩阵、数据新鲜度、下钻权限和审计留痕。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批从 `component-health-card`、`component-ranking-list` 开始。
- 2026-06-28：按两张一批完成 `component-health-card` 和 `component-ranking-list`。两张均为基础数据展示组件板，不绘制完整 AppShell，只使用 foundations token 和 Noto Sans CJK 字体确定性绘制：`component-health-card` 覆盖健康卡结构、Probe/Kafka/Flink/ClickHouse/OpenSearch/MinIO/数据质量/模型部署健康样例、状态矩阵、依赖子检查和修复审计边界；`component-ranking-list` 覆盖 TopN 行结构、高风险资产/异常链路/外联目的地/Kafka lag/规则命中/证据缺口/模型特征/慢查询样例、状态矩阵、排序阈值和行级审计边界。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批从 `component-log-list`、`component-evidence-file-card` 开始。
- 2026-06-28：按两张一批完成 `component-log-list` 和 `component-evidence-file-card`。两张均为基础数据展示组件板，不绘制完整 AppShell，只使用 foundations token、DroidSansFallback 中文字体和 DejaVu Sans 英文字体确定性绘制：`component-log-list` 覆盖时间戳、日志级别、来源组件、对象 ID、trace_id、message 摘要、上下文标签、展开详情、复制、定位来源、筛选高亮、审计留痕、脱敏提示和状态矩阵；`component-evidence-file-card` 覆盖文件类型图标、文件名、证据类型、关联对象、大小、时间窗、hash 校验、签名 URL、保留期、权限范围、下载/预览/复制 hash/关联告警动作和审计提示。两张最终图已保存到 `screens/components/` 并标准化为 `1920x1080`；同时保留 `*.raw-deterministic.png` 作为确定性来源追溯。下一批从 `component-empty-card`、`component-permission-card` 开始。
- 2026-06-28：按两张一批连续补齐最后 46 张 manifest 缺口：组件 18 张、状态图 16 张、响应式图 12 张。所有图均不绘制完整 AppShell，只使用 foundations token、DroidSansFallback 中文字体和 DejaVu Sans 英文字体确定性绘制；最终图已分别保存到 `screens/components/`、`screens/states/`、`screens/responsive/`，统一标准化为 `1920x1080`，并保留 `*.raw-deterministic.png` 作为确定性来源追溯。至此 manifest 交付基线完成 163/163，下一张常规生成项为无。
- 2026-06-26：用户最终纠正专题产品口径：不需要再次生成 `topics.png`，专题设计已拆分为三张 Tab 页；前端开发时必须将三张 Tab 设计合并到同一个 `/topics` 页面内，菜单只保留 `专题面板`。现役 manifest 回到 163 张，不包含单张 `topics` 页面主图；旧 `/topics/tunnel`、`/topics/exfil`、`/topics/apt` 只作为兼容深链或 API 语义来源，进入后映射到 `/topics?topic=...`。
- 2026-06-26：补充通用规则：后续任何页面如果 UI 设计图按 Tab 拆成多张，前端也必须合并到一个路由页面内作为页内 Tab/Segmented 状态实现；除非产品文档明确要求新增左侧菜单，否则不得把 Tab 拆成多个左侧菜单或独立业务路由。
- 2026-06-23：曾按当时约束移除 `topics` 页面、`modal-topic-create`、`modal-topic-report-export`。其中 `topics/专题面板` 不再作为单张 UI suite 页面主图恢复；其前端菜单和页内 Tab 实现仍保持现役。
- `modal-topic-create`、`modal-topic-report-export` 仍保持清退；如后续需要专题创建或专题报告导出浮层，应由用户重新授权后再恢复到 manifest。
- 2026-06-23：用户确认现阶段不再需要保留修复 UI 图；已清理 `doc/04_assets/ui_suite_gpt_v1/repair_batches/` 和 `doc/04_assets/ui_suite_gpt_v1/audits/` 过程目录。后续以最终交付图、manifest、prompt、`CONTEXT_HANDOFF.md` 和本文档为准。

- 2026-06-20：此前按当时约束重新生成 `screen` 与 `dashboard`。
- `screen` 保持态势大屏定位，采用“园区数字孪生拓扑 + 采集流处理管道 + 威胁态势 + 证据取证闭环 + 响应反馈”构图。
- `dashboard` 保持仪表盘定位，采用“脱敏运营 KPI + 优先级待办队列 + 采集与数据健康门禁 + 告警处置阶段工作篮 + 证据/反馈质量摘要 + 验收缺口”构图，不展示责任人、负责人、头像、班组、值班表、个人账号、待指派、未认领责任或交接名单。
- 两张图均已覆盖到 `doc/04_assets/ui_suite_gpt_v1/screens/pages/`，最终尺寸均为 `1920x1080`。
- 2026-06-20：修复 `screen` 底部状态栏。态势大屏底部必须与 `dashboard` 和 foundations 一致，为单层固定 AppShell Statusbar；大屏刷新间隔、拓扑渲染延迟、链路带宽水位、流向动画帧率等运行底座指标只能放在主内容区运行底座面板，禁止追加第二行底栏。
- 2026-06-20：`screen` 明确以用户提供的态势大屏附件图为最终基准，已直接覆盖到 `doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png` 并标准化为 `1920x1080`；不再使用临时修图规则。
- 2026-06-20：压缩本轮上下文到 `CONTEXT_HANDOFF.md`。随后用户明确要求基于最新附件 `codex-clipboard-1315c1d0-c327-48b5-9f45-6415a9fe94e0.png` 使用 imagegen 重新生成态势大屏，业务内容保持不变，仅让右侧“威胁态势总览”和“运行底座（大屏性能与渲染）”边框分开且闭合。第一版 imagegen 内容漂移未落盘；第二版局部修补提示结果已提取为 `screen.png`，并标准化为 `1920x1080`。
- 2026-06-20：按用户要求重新生成 `login`。旧登录页暴露过多系统信息，现已改为低暴露统一身份认证入口：无业务导航、无顶部 KPI、无底部运维状态栏、无能力摘要、无拓扑/链路/组件名/指标/版本/时间戳/追踪 ID/默认账号/只读演示入口；仅保留品牌、登录表单、OIDC/SSO、帮助/隐私/忘记密码和通用安全提示。`login.png` 已提取并标准化为 `1920x1080`。
- 2026-06-20：按用户要求重新生成 `dashboard`。新图采用脱敏运营工作台口径，主视觉为脱敏 KPI、优先级待办队列、采集与数据健康门禁、验收缺口与建议动作、告警处置阶段工作篮、证据与反馈质量摘要和 Top Talkers 风险贡献；未出现责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单或个人任务归属。`dashboard.png` 已提取并标准化为 `1920x1080`。
- 2026-06-20：按用户要求用 imagegen 修正 `dashboard` 右上快捷入口，使其与 `screen` 和 foundations 一致：PCAP检索、资产检索、规则检索、脚本中心、帮助中心、更多应用。`dashboard.png` 已重新提取并标准化为 `1920x1080`。
- 2026-06-20：继续常规清单生成 `alerts` 告警中心。生成前已补充 prompt 门禁：一级菜单高亮 `威胁分析`，二级子项高亮 `告警中心`，右上快捷入口固定为 `PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用`，底部必须为单层 AppShell Statusbar。用户指出首版左侧菜单与 foundations 和态势大屏不一致，已基于用户提供的告警中心图用 GPT imagegen 重新生成并覆盖 `alerts.png`：左侧导航改为与态势大屏 `screen.png` 一致的单栏展开式菜单，在同一侧栏内展开 `威胁分析` 子项，不再使用“窄一级栏 + 独立二级栏”的双栏结构；最终图已标准化为 `1920x1080`，主结构为告警筛选、告警队列表格、右侧选中告警研判详情、研判时间线、关联告警簇和处理反馈表单。
- 2026-06-21：按用户要求先完成 `采集监测` 的 `probes` 探针管理和 `data-quality` 数据质量。生成前已把两个 prompt 的左侧菜单旧“双栏导航”口径改为与态势大屏 `screen.png` 一致的单栏展开式菜单：同一侧栏内展开 `采集监测`，分别高亮 `探针管理` 和 `数据质量`，禁止独立二级菜单栏和页面内部模块进入左侧菜单。两张图均已用 GPT 内置 imagegen 生成、提取到 `screens/pages/`，并标准化为 `1920x1080`。
- 2026-06-21：按用户要求继续后续菜单并避免反复返工，已对 `doc/04_assets/ui_suite_gpt_v1/prompts/*.prompt.txt` 做机械修正：将旧双栏导航口径统一替换为与态势大屏 `screen.png` 一致的单栏展开式左侧菜单约束。
- 2026-06-21：完成威胁分析后续 6 张：`alert-detail` 告警详情、`campaigns` 战役列表、`campaign-detail` 战役详情、`attack-chains` 攻击链分析、`encrypted-traffic` 加密流量、`forensics` 取证分析。六张图均使用 GPT 内置 imagegen 逐张生成，提取到 `screens/pages/`，并标准化为 `1920x1080`。下一张进入资产图谱 `assets`。
- 2026-06-21：完成资产图谱 `assets` 资产台账。图中左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `资产图谱` 并高亮子项 `资产台账`；主结构为高密度资产表、资产分类指标、右侧资产详情与风险抽屉、流量画像、协议分布、Top 对端、周期性连接和关联证据入口。最终图已提取到 `screens/pages/assets.png` 并标准化为 `1920x1080`。下一张进入 `graph` 实体图谱。
- 2026-06-21：完成资产图谱 `graph` 实体图谱。图中左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `资产图谱` 并高亮子项 `实体图谱`；主结构为实体搜索、关系筛选、抽象实体关系图谱画布、路径分析结果、图查询治理、右侧实体详情、邻居统计和关联时间线。最终图已提取到 `screens/pages/graph.png` 并标准化为 `1920x1080`。下一张进入 `fusion` 数据融合。
- 2026-06-21：完成资产图谱 `fusion` 数据融合。图中左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `资产图谱` 并高亮子项 `数据融合`；主结构为数据源状态、多源融合编排、融合规则管理、融合收益对比、冲突队列、融合事件审计、右侧冲突处理抽屉和融合质量看板。最终图已提取到 `screens/pages/fusion.png` 并标准化为 `1920x1080`。下一张进入 `baselines` 行为基准。
- 2026-06-21：完成资产图谱 `baselines` 行为基准。图中左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `资产图谱` 并高亮子项 `行为基准`；主结构为基线范围筛选、基线状态机、行为分布分析、偏离列表、基线版本管理、右侧偏离解释与治理操作。最终图已提取到 `screens/pages/baselines.png` 并标准化为 `1920x1080`。`probes` 和 `data-quality` 已在前序完成，下一张跳转到未完成项 `rules`。
- 2026-06-21：补记检测运营 `rules` 规则管理已完成。图中左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `检测运营` 并高亮子项 `规则管理`；主结构为规则列表、规则状态、命中趋势、规则编排与规则详情配置。最终图已位于 `screens/pages/rules.png` 并标准化为 `1920x1080`。
- 2026-06-21：完成检测运营 `deployments` 部署管理。图中左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `检测运营` 并高亮子项 `部署管理`；主结构为发布清单、灰度策略、发布健康、版本对比、回滚管理和发布证据链。最终图已提取到 `screens/pages/deployments.png` 并标准化为 `1920x1080`。下一张进入 `models` 模型管理。
- 2026-06-21：按“三张一批后压缩上下文”节奏完成检测运营 `models`、`mlops`、`playbooks`。三张图均使用 GPT 内置 imagegen 逐张生成，分别提取到 `screens/pages/models.png`、`screens/pages/mlops.png`、`screens/pages/playbooks.png`，并标准化为 `1920x1080`。
- 2026-06-21：`models` 模型管理主结构为模型列表、模型指标、Champion/Challenger 状态机、数据集与样本、解释与特征、激活与回滚；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `模型管理`。
- 2026-06-21：`mlops` MLOps 编排主结构为闭环编排 DAG、反馈样本池、训练任务队列、评估与门禁、注册与发布和效果回流；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `MLOps 编排`。
- 2026-06-21：`playbooks` SOAR 剧本主结构为剧本列表、剧本编排流程画布、节点配置/触发策略、风险控制、执行历史、处置效果和审计证据；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `SOAR 剧本`。下一张进入 `whitelist` 白名单。
- 2026-06-21：完成第二个“三张一批后压缩上下文”批次：`whitelist`、`compliance`、`audit-log`。三张图均使用 GPT 内置 imagegen 逐张生成，分别提取到 `screens/pages/whitelist.png`、`screens/pages/compliance.png`、`screens/pages/audit-log.png`，并标准化为 `1920x1080`。
- 2026-06-21：`whitelist` 白名单主结构为白名单列表、条件构造器、审批流程状态机、命中监控、到期治理、反馈关联和影响矩阵；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `白名单`。
- 2026-06-21：`compliance` 合规审计主结构为验收门禁矩阵、指标映射追踪表、证据包完整度、运行报告预览、缺口治理和第三方评测批次；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `合规审计`。
- 2026-06-21：`audit-log` 审计日志主结构为日志检索、高密度审计表、操作详情 Diff、高风险审计、关联链路、留存状态和导出取证；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `审计日志`。下一张进入 `notifications` 通知配置。
- 2026-06-21：完成第三个“三张一批后压缩上下文”批次：`notifications`、`settings`、`not-found`。三张图均使用 GPT 内置 imagegen 逐张生成，分别提取到 `screens/pages/notifications.png`、`screens/pages/settings.png`、`screens/pages/not-found.png`，并标准化为 `1920x1080`。
- 2026-06-21：`notifications` 通知配置主结构为通知渠道健康、订阅规则、条件构造器、升级策略流程、模板管理、发送历史、抑制与静默；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `通知配置`。
- 2026-06-21：`settings` 系统设置主结构为租户与站点树表、RBAC 权限矩阵、API 令牌、数据留存策略、集成配置健康、安全策略和系统参数；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `系统设置`。
- 2026-06-21：`not-found` 404 异常页主结构为工程化错误摘要、返回入口、安全提示、辅助动作、最近可用入口和相关系统状态；不展示敏感路径、堆栈、接口细节或凭据。下一张进入 overlay `dropdown-user-menu`。
- 2026-06-21：启动 pages AppShell 公共区返工。`batch-01` 已使用 GPT 内置 imagegen 完成 `alert-detail`、`alerts`、`assets` 三张图公共壳修复，并覆盖 `screens/pages/` 目标图。每张图均先提取 imagegen 候选，再回贴原始业务保护区 `(198,80)-(1920,997)`；最终尺寸均为 `1920x1080`，中区像素差异均为 `None`。修复过程图已于 2026-06-23 清理。
- 2026-06-21：`batch-02` 已完成并覆盖 `attack-chains`、`audit-log`、`baselines`。`attack-chains` 使用 GPT 内置 imagegen 完成公共壳修复并回贴业务保护区；`audit-log` 前三次 imagegen 候选因一级菜单图标漂移未采纳，最终采用 `screen.png` 一级菜单截图图标贴回方式修正；`baselines` 使用 GPT 内置 imagegen 生成公共壳候选后，回贴原始业务保护区，并固定一级菜单截图图标与资产图谱二级菜单图标。三张最终图均为 `1920x1080`，中区像素差异均为 `None`。下一张：`campaign-detail`。
- 2026-06-21：`batch-03` 启动并完成 `campaign-detail`。本张使用 GPT 内置 imagegen 生成公共壳候选，回贴原始业务保护区，并固定一级菜单截图图标与威胁分析二级菜单图标；详情路由继承 `战役列表` 高亮。最终图为 `1920x1080`，中区像素差异为 `None`。下一张：`campaigns`。
- 2026-06-21：`batch-03` 继续完成 `campaigns`。本张使用 GPT 内置 imagegen 生成公共壳候选，回贴原始业务保护区，并固定一级菜单截图图标与威胁分析二级菜单图标；展开 `威胁分析` 并高亮 `战役列表`。最终图为 `1920x1080`，中区像素差异为 `None`。下一张：`compliance`。
- 2026-06-21：`batch-03` 继续完成 `compliance`。本张使用 GPT 内置 imagegen 生成公共壳候选，回贴原始业务保护区，并固定一级菜单截图图标；展开 `审计配置` 并高亮 `合规审计`。二级图标强制贴回的首版坐标偏低未采纳，最终使用 `compliance.final-v2.png` 覆盖。最终图为 `1920x1080`，中区像素差异为 `None`。下一张：`dashboard`。
- 2026-06-21：`batch-03` 继续完成 `dashboard`。本张使用 GPT 内置 imagegen 生成公共壳候选，回贴原始业务保护区；展开 `综合态势` 并高亮 `仪表盘`。顶部栏修复为 `screen.png` 同款结构，左侧恢复用户状态卡，底部修复为统一单栏；中部脱敏运营仪表盘业务内容保持不变。最终图为 `1920x1080`，中区像素差异为 `None`。下一张：`data-quality`。
- 2026-06-21：`batch-04` 启动并按用户纠正重做 `data-quality`。本张重新采用完整图像参照方法：打开 `screen.png` 与原始 `data-quality.png`，两次使用 GPT 内置 imagegen 生成候选，第一次候选因错误保留 `综合态势` 二级菜单未采纳；第二次候选修正为 `采集监测` 展开并高亮 `数据质量`。最终图使用第二次候选公共布局、原始业务区回贴、顶部/底部 `screen.png` 逐像素覆盖和一级/二级标准图标贴片。最终图为 `1920x1080`，业务保护区 `(198,80)-(1920,997)` 与严格业务区 `(166,80)-(1920,997)` 像素差异均为 `None`，顶部和底部与 `screen.png` 像素差异均为 `None`。下一张：`deployments`。
- 2026-06-22：`batch-04` 继续完成 `deployments`。本张使用既有 GPT 内置 imagegen 公共壳候选，展开 `检测运营` 并高亮 `部署管理`；顶部和底部直接覆盖 `screen.png`，像素差异均为 `None`。由于原图仍是旧双栏导航，真实业务内容从 `x=252` 开始，本张将原始业务源区 `(252,80)-(1920,997)` 原样搬移到标准内容起点 `(198,80)-(1866,997)`，源区像素差异为 `None`；右侧 54px 只延展原业务图最右边缘像素，避免重复控件。最终图为 `1920x1080`，已覆盖 `screens/pages/deployments.png`。下一张：`encrypted-traffic`。
- 2026-06-22：`batch-04` 继续完成 `encrypted-traffic`。本张原图已是单栏展开式左侧导航，展开 `威胁分析` 并高亮 `加密流量`，因此采用确定性修复：保留原左侧菜单区 `(0,80)-(198,997)` 和业务保护区 `(198,80)-(1920,997)`，顶部与底部直接覆盖 `screen.png`。最终图为 `1920x1080`，顶部、底部、左侧菜单区和业务保护区像素差异均为 `None`，已覆盖 `screens/pages/encrypted-traffic.png`。下一张：`forensics`。
- 2026-06-22：`batch-04` 继续完成 `forensics`。本张原图已是单栏展开式左侧导航，展开 `威胁分析` 并高亮 `取证分析`，因此采用确定性修复：保留原左侧菜单区 `(0,80)-(198,997)` 和业务保护区 `(198,80)-(1920,997)`，顶部与底部直接覆盖 `screen.png`。最终图为 `1920x1080`，顶部、底部、左侧菜单区和业务保护区像素差异均为 `None`，已覆盖 `screens/pages/forensics.png`。下一张：`fusion`。
- 2026-06-22：`batch-04` 继续完成 `fusion`。本张原图已是单栏展开式左侧导航，展开 `资产图谱` 并高亮 `数据融合`；最终采用确定性修复和标准图标贴片：业务保护区 `(198,80)-(1920,997)` 保持不变，顶部与底部直接覆盖 `screen.png`，左侧一级图标和资产图谱二级图标使用 `icon_stencils` 标准贴片并按激活态重新着色。首版擦底候选因出现可见色块未采纳。最终图为 `1920x1080`，顶部、底部和业务保护区像素差异均为 `None`，左侧差异仅在图标贴片范围内，已覆盖 `screens/pages/fusion.png`。下一张：`graph`。
- 2026-06-22：`batch-04` 继续完成 `graph`。本张原图已是单栏展开式左侧导航，展开 `资产图谱` 并高亮 `实体图谱`；最终采用确定性修复和标准图标贴片：业务保护区 `(198,80)-(1920,997)` 保持不变，顶部与底部直接覆盖 `screen.png`，左侧一级图标和资产图谱二级图标使用 `icon_stencils` 标准贴片并按激活态重新着色。首版复用 `fusion` 坐标的候选因出现旧图标残影未采纳，最终改用本图检测出的图标中心。最终图为 `1920x1080`，顶部、底部和业务保护区像素差异均为 `None`，左侧差异仅在图标贴片范围内，已覆盖 `screens/pages/graph.png`。下一张：`mlops`。
- 2026-06-22：`batch-04` 继续完成 `mlops`。本张原图仍是旧宽左栏，真实业务内容从 `x=232` 开始；最终采用确定性公共区修复：顶部与底部直接覆盖 `screen.png`，左侧切换为检测运营域标准宽度公共栏并高亮 `MLOps 编排`，一级图标使用 `icon_stencils` 标准贴片，原始业务源区 `(232,80)-(1920,997)` 原样搬移到 `(198,80)-(1886,997)`，右侧 34px 延展原图最右边缘像素。首版高亮切换候选和一级图标灰块候选均未采纳。最终图为 `1920x1080`，顶部、底部和业务源区像素差异均为 `None`，已覆盖 `screens/pages/mlops.png`。下一张：`models`。
- 2026-06-22：`batch-04` 继续完成 `models`。本张采用默认业务保护区 `(198,80)-(1920,997)`，顶部与底部直接覆盖 `screen.png`，左侧切换为检测运营域标准宽度公共栏并高亮 `模型管理`，一级图标使用 `icon_stencils` 标准贴片。最终图为 `1920x1080`，顶部、底部和业务保护区像素差异均为 `None`，已覆盖 `screens/pages/models.png`。下一张：`not-found`。
- 2026-06-22：`batch-04` 继续完成 `not-found`。本张是 404 异常页，prompt 明确不显示常规二级菜单；最终按 pages 公共 AppShell 门禁处理：顶部、左侧、底部直接覆盖 `screen.png`，原始异常页业务源区 `(232,80)-(1920,997)` 原样搬移到 `(198,80)-(1886,997)`，右侧 34px 延展原图最右边缘像素。最终图为 `1920x1080`，顶部、左侧、底部和业务源区像素差异均为 `None`，已覆盖 `screens/pages/not-found.png`。下一张：`notifications`。
- 2026-06-22：`batch-04` 继续完成 `notifications`。本张属于审计配置域，使用审计配置公共栏并高亮 `通知配置`；顶部与底部直接覆盖 `screen.png`，原始业务源区 `(206,80)-(1920,997)` 原样搬移到 `(198,80)-(1912,997)`，右侧 8px 延展原图最右边缘像素。首版通知激活行候选因残留旧激活态纹理未采纳，最终恢复审计配置标题区并清理残留。最终图为 `1920x1080`，顶部、底部和业务源区像素差异均为 `None`，已覆盖 `screens/pages/notifications.png`。下一张：`playbooks`。
- 2026-06-22：`batch-04` 完成现役 page 公共区返工中的 `playbooks`、`probes`、`rules`、`settings`、`whitelist` 等输出。专题相关产物已于 2026-06-23 归档，不再计入现役 page 或后续生成范围。至此现役 pages 公共 AppShell 返工已覆盖除 `login.png`、`screen.png` 外的全部 page。
- 2026-06-22：追加最终顶底栏通行。全量校验发现 `audit-log`、`baselines`、`campaign-detail`、`campaigns`、`compliance`、`dashboard` 的顶部或底部仍未与当前 `screen.png` 逐像素一致；已只覆盖这 6 张的顶部 `(0,0)-(1920,80)` 和底部 `(0,997)-(1920,1080)`，业务保护区 `(198,80)-(1920,997)` 与修正前备份像素差异均为 `None`。最终全量校验：除 `login.png`、`screen.png` 外的 29 张 page 顶部和底部与 `screen.png` 像素差异均为 `None`。

## AppShell 一致性门禁

- 2026-06-21：曾检查除 `login.png` 外的 pages 图，发现底部状态栏内容和左侧菜单图标未全量保持一致；该问题已完成返工，过程审计图已于 2026-06-23 清理。
- 2026-06-21：新增全局公共 AppShell 标准 `doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md`，并生成视觉裁切基准 `doc/04_assets/ui_suite_gpt_v1/standards/app-shell-baseline-screen-dashboard.png`。
- 2026-06-21：根据最新返工要求升级门禁：`pages/` 下除 `login.png` 和 `screen.png` 外的所有 UI 图，公共部分只以态势大屏 `screen.png` 为基准；`dashboard.png` 也是待修复页面，不作为公共壳基准。
- 全部适用 UI 图的公共部分必须与态势大屏 `screen.png` 完全一致：顶部单栏、左侧单栏、底部单栏的内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态都不得按页面自行变化。
- 顶部状态栏必须保持同一系统名称、站点/时间/风险/告警/严重告警/采集健康/数据质量/快捷入口的结构和顺序；快捷入口固定为 `PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用`。
- 底部状态栏统一为 `数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 / 右侧全局动作图标组`，顺序、图标语义、分隔线、状态色和字号不得按页面改写。
- 左侧一级菜单图标语义和风格必须固定：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置六个一级菜单图标必须逐项复刻 `screen.png`；二级菜单图标按 `APP_SHELL_ICON_STANDARD.md` 的固定 iconId 清单执行，展开域只变高亮项和文本，不得更换图标体系、改成双栏或新增第三层。
- 修复既有 page 图片时，只允许修改顶部、左侧、底部公共区域，中部业务内容区必须保持原图不变，不得重绘、替换指标、调整业务面板或改变业务布局。
- 已将该全局公共 AppShell 绝对一致性硬门禁批量注入 prompt 文件。后续生成、返工或重生成必须使用这些 prompt 约束。

## 清退模块

- `topics/专题面板` 是现役 Web 菜单项和前端路由，但不作为单张 `/topics` 页面主图生成；三张专题 Tab 设计输入只作为该页内模式资产使用。
- `modal-topic-create`、`modal-topic-report-export` 仍保持清退；如后续需要专题创建或专题报告导出浮层，应由用户重新授权后再恢复到 manifest。

## 硬约束

1. 使用 GPT 内置 `imagegen` 逐张生成，不切换 CLI/API fallback，除非用户明确要求。
2. 最终视觉基线固定为：`doc/04_assets/generated/campus_full_traffic_system_visual_reference_20260620_business_corrected.png`；`doc/04_assets/ui_suite_gpt_v1/screens/foundations` 仅作为 UI 规范约束，不替代视觉基线。
3. 保存路径必须位于：`doc/04_assets/ui_suite_gpt_v1/screens/...`。
4. 每张高保真图最终交付尺寸必须是：`1920x1080 px`。
5. 后续所有生成、编辑或重生成的 UI 图片都必须严格符合 `doc/04_assets/ui_suite_gpt_v1/screens/foundations` 下的 UI 规范板，而不是只保持风格相似；不得以单张图、局部修图、业务差异或风格自由发挥为由绕过 foundations：
   - 最终视觉基准
   - 布局与栅格
   - 色彩与状态语义
   - 字体与密度
   - 图标与动作语义
   - 数据可视化
   - 表格与表单密度
   - 响应式适配原则
6. 必须锁定 AppShell、12 栅格、8px 面板间距、深色 token、状态色、字号密度、圆角、表格行高、ECharts 深色样式和响应式策略。
7. 不同独立页面不能相似，不能只替换标题、菜单或数字。
8. 不同独立页面的主工作区、右侧栏、表格和图表指标名称不能重叠；系统固定顶部状态条和底部状态栏除外。
9. 页面要保持统一视觉基线，但主视觉结构、核心组件、数据口径、下钻动作必须随页面业务变化。
10. 底部状态栏全套统一为单层 AppShell Statusbar，约 40px 高；禁止任意页面在底栏上方或内部增加第二行页面专属指标。
11. 任何按 Tab 拆分的 UI 设计输入都必须在前端合并为单一路由页面的页内状态；不因设计图拆张而新增左侧菜单、独立业务路由或额外 AppShell 入口。

## `screen` 与 `dashboard` 专项约束

`screen` 是态势大屏，使用大屏专属指标：

- 楼宇覆盖率
- 园区在线覆盖
- 核心链路状态
- 汇聚链路状态
- 异常链路位置
- 探针覆盖地图
- 攻击阶段热度
- 战役簇密度
- 风险区域密度
- 异常链路影响面
- 外联流向强度
- PCAP 覆盖率
- Session 还原率
- 日志关联率
- 对象存储归档率
- hash 校验通过率
- 签名 URL 可用率
- 隔离动作数
- 阻断动作数
- 封禁动作数
- 下发脚本数
- 反馈标注数
- 模型学习批次数
- 拓扑渲染延迟
- 链路带宽水位
- 流向动画帧率

`dashboard` 是值班仪表盘，使用值班专属指标：

- 超时 SLA
- 临近超时数
- 高危未处理
- 待取证
- 待反馈
- 待复核
- 队列积压量
- 今日闭环进度
- 健康门禁通过率
- 门禁失败项
- 平均确认时长
- 平均闭环时长
- 验收缺口数
- 复核完成率
- 审计留痕缺口
- 证据完整度缺口
- 反馈覆盖率
- Top Talkers 风险贡献
- 待补证据数
- 待回流样本数
- 工单逾期数

`dashboard` 禁止展示敏感人员字段：责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单、个人任务归属。确需查看人员或组织归属时，必须进入具备权限控制和审计记录的业务详情页。

## 继续生成流程

1. 读取 `doc/04_assets/ui_suite_gpt_v1/manifest.json`。
2. 找到第一条 `targetFile` 不存在的 item。
3. 读取该 item 的 `promptFile`。
4. 调用内置 `image_gen.imagegen` 生成一张图。
5. 生成后运行：

```bash
python3 doc/04_assets/ui_suite_gpt_v1/extract_latest_imagegen.py <targetFile>
```

6. 使用 `view_image` 抽检：
   - 是否符合 foundations 规范
   - 是否 1920x1080
   - 产品名是否为“园区网络全流量采集与分析系统”
   - 是否与已生成页面明显不同
   - 是否没有复用其他独立页面的主工作区指标
7. 如产品名有错字，允许本地后处理只修正固定产品名，不改布局和业务内容。
8. 更新本文件的“当前进度”和“已完成图片”；每累计 2 张图后同步关键断点到 `CONTEXT_HANDOFF.md`，形成可重连的压缩上下文。

## 当前脚本

- `doc/04_assets/ui_suite_gpt_v1/build_prompt_manifest.mjs`：生成 163 个 prompt 与 manifest。
- `doc/04_assets/ui_suite_gpt_v1/extract_latest_imagegen.py`：从 Codex Desktop 会话中提取最新 imagegen 结果，保存 raw 并输出标准 `1920x1080` PNG。

## 注意事项

- 不要把图片保存到 `doc/04_assets/ui_suite_gpt_v1/pages`；该目录已废弃。
- 不要把 `screen` 做成普通仪表盘，也不要把 `dashboard` 做成态势大屏。
- 后续页面生成时，如果内容结构与前面页面接近，必须先调整 prompt，再生成。

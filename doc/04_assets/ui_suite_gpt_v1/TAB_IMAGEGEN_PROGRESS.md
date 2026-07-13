# Tab 变体 UI 图生成进度

更新时间：2026-06-27

本文档用于在每次只生成 1 张 Tab 变体图后压缩上下文，避免依赖聊天历史继续推进。

## 当前方法

1. 先读 `agent.md`、`doc/04_assets/ui_suite_gpt_v1/README.md`、`CHAT_IMAGEGEN_INVENTORY.md`、`CONTEXT_HANDOFF.md`、`doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md`、`doc/01_design/面向园区网络的全流量采集分析系统-Tab页功能点与表现形式矩阵.md`。
2. 只从用户确认的 Tab 补图队列中选择当前断点；如果用户明确要求重生，即使 `screens/pages/<建议输出 ID>.png` 已存在也要覆盖旧图。
3. 生成前必须先判定 Tab 层级：大 Tab 是页面级主工作区切换；小 Tab 是详情、抽屉、操作详情或功能块内部局部切换。
4. 每轮只生成 1 张图；截至 2026-06-27，显式小 Tab 队列已完成，资产台账层级问题已完成返工。
5. 使用内置 `image_gen.imagegen`，不使用 CLI/API fallback，除非用户明确改口。
6. 小 Tab 独立图生成前只把 `screens/foundations/foundation-generation-reference.png`、对应主页面图和既有小 Tab 对齐样式作为参考；最终画面不得包含公共 AppShell、宿主页面公共区或业务公共区。
7. 生成后立即执行：

```bash
python doc/04_assets/ui_suite_gpt_v1/extract_latest_imagegen.py <target> --session <current-session-jsonl>
```

8. 最终图保存到 `doc/04_assets/ui_suite_gpt_v1/screens/pages/`，同时保留 `<id>.raw-imagegen.png`。
9. 每张图必须记录 prompt、目标路径、尺寸、质量结论、Tab 层级和下一张队列。

## 大 Tab / 小 Tab 口径

- 大 Tab：页面级主工作区切换，通常位于页面标题、筛选栏或主内容顶部，切换后整块业务工作台改变，但仍不新增左侧菜单或独立路由。当前大 Tab 补图已完成；不要再把 `路径分析 Tab`、`规则编辑 Tab`、`样本回放 Tab`、`解释与特征 Tab`、`激活与回滚 Tab`、`条件构造器 Tab`、`到期治理 Tab` 归入大 Tab 队列。
- 小 Tab：详情态、抽屉、操作详情或功能块内部的局部内容切换。用户最新要求以下小 Tab 只生成独立局部图：画面只包含左侧小 Tab 列和右侧当前 Tab 内容，不包含公共 AppShell、宿主页面公共区或业务公共区。
- 资产台账专门口径：`终端 / 服务器 / 网络设备 / 业务系统 / 未知资产` 是 `/assets` 页面级大 Tab，保留完整资产台账工作台，右侧只放选中对象摘要；`基础信息 / 网络接口 / 开放服务 / 归属信息 / 历史变更` 是资产详情局部小 Tab，只输出详情组件本体。
- 大 Tab 生成要求：prompt 必须说明这是同一路由内的页面级主 Tab，左侧菜单和 AppShell 不变，当前激活 Tab 明显，主工作区整体围绕该 Tab 重组。
- 小 Tab 生成要求：prompt 必须说明这是小 Tab 独立组件图，左侧小 Tab 对齐，右侧是当前 Tab 内容；不把小 Tab 画成新的菜单、独立路由、完整页面或宿主页面截图。

## 验收口径

- 当前大/小 Tab 层级与激活态清晰，未激活 Tab 可见。
- 主工作区必须能看出该 Tab 的业务主题，不允许只替换 Tab 标题。
- 至少覆盖 Tab 矩阵中 `页面内容（必须画出来）` 的 3 类关键对象。
- 必须出现矩阵要求的主表现形式，例如热力图、趋势图、表格、关系图、状态机、Diff 或时间线。
- 大 Tab 图保留顶部、左侧、底部公共 AppShell；小 Tab 独立图禁止出现公共 AppShell、宿主页面公共区或业务公共区。
- 大 Tab 必须表现为主工作区整体切换；小 Tab 必须表现为局部组件内的左侧小 Tab 列与右侧内容切换。
- 每张图都要保留闭环动作入口，例如定位、导出、修复任务、审批、查看证据或进入详情。

## 已生成

| 时间 | ID | 来源页面 | Tab 分组 | 激活 Tab | 输出文件 | Prompt | 尺寸 | 结果 |
|---|---|---|---|---|---|---|---|---|
| 2026-06-26 | `data-quality-topic-health` | `/data-quality` | 数据质量顶部 Tab | Topic 健康 | `screens/pages/data-quality-topic-health.png` | `prompts/data-quality-topic-health.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `data-quality-flink-quality` | `/data-quality` | 数据质量顶部 Tab | Flink 质量 | `screens/pages/data-quality-flink-quality.png` | `prompts/data-quality-flink-quality.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `data-quality-field-quality` | `/data-quality` | 数据质量顶部 Tab | 字段质量 | `screens/pages/data-quality-field-quality.png` | `prompts/data-quality-field-quality.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `data-quality-storage-quality` | `/data-quality` | 数据质量顶部 Tab | 存储质量 | `screens/pages/data-quality-storage-quality.png` | `prompts/data-quality-storage-quality.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `data-quality-replay-reconcile` | `/data-quality` | 数据质量顶部 Tab | 重放对账 | `screens/pages/data-quality-replay-reconcile.png` | `prompts/data-quality-replay-reconcile.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `data-quality-report` | `/data-quality` | 数据质量顶部 Tab | 质量报告 | `screens/pages/data-quality-report.png` | `prompts/data-quality-report.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `data-quality-settings` | `/data-quality` | 数据质量顶部 Tab | 质量设置 | `screens/pages/data-quality-settings.png` | `prompts/data-quality-settings.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `encrypted-traffic-fingerprint` | `/encrypted-traffic` | 加密流量顶部 Tab | 指纹分析 | `screens/pages/encrypted-traffic-fingerprint.png` | `prompts/encrypted-traffic-fingerprint.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `encrypted-traffic-tunnel-detection` | `/encrypted-traffic` | 加密流量顶部 Tab | 隧道检测 | `screens/pages/encrypted-traffic-tunnel-detection.png` | `prompts/encrypted-traffic-tunnel-detection.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `encrypted-traffic-egress-profile` | `/encrypted-traffic` | 加密流量顶部 Tab | 外联画像 | `screens/pages/encrypted-traffic-egress-profile.png` | `prompts/encrypted-traffic-egress-profile.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `encrypted-traffic-evidence-center` | `/encrypted-traffic` | 加密流量顶部 Tab | 证据中心 | `screens/pages/encrypted-traffic-evidence-center.png` | `prompts/encrypted-traffic-evidence-center.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-27 | `assets` | `/assets` | 资产类型 Tab（页面级大 Tab） | 终端 | `screens/pages/assets.png` | `prompts/assets.prompt.txt` | 1920x1080 | 已按资产分类页面级 Tab 口径重生，右侧改为选中终端摘要 |
| 2026-06-27 | `assets-server` | `/assets` | 资产类型 Tab（页面级大 Tab） | 服务器 | `screens/pages/assets-server.png` | `prompts/assets-server.prompt.txt` | 1920x1080 | 已按资产分类页面级 Tab 口径重生，右侧改为选中服务器摘要 |
| 2026-06-27 | `assets-network-device` | `/assets` | 资产类型 Tab（页面级大 Tab） | 网络设备 | `screens/pages/assets-network-device.png` | `prompts/assets-network-device.prompt.txt` | 1920x1080 | 已按资产分类页面级 Tab 口径重生，右侧改为选中网络设备摘要 |
| 2026-06-27 | `assets-business-system` | `/assets` | 资产类型 Tab（页面级大 Tab） | 业务系统 | `screens/pages/assets-business-system.png` | `prompts/assets-business-system.prompt.txt` | 1920x1080 | 已按资产分类页面级 Tab 口径重生，右侧改为选中业务系统摘要 |
| 2026-06-27 | `assets-unknown` | `/assets` | 资产类型 Tab（页面级大 Tab） | 未知资产 | `screens/pages/assets-unknown.png` | `prompts/assets-unknown.prompt.txt` | 1920x1080 | 已按资产分类页面级 Tab 口径重生，右侧改为选中未知资产摘要 |
| 2026-06-27 | `assets-detail-basic` | `/assets` | 资产详情小 Tab（局部详情组件） | 基础信息 | `screens/pages/assets-detail-basic.png` | `prompts/assets-detail-basic.prompt.txt` | 1920x1080 | 已按资产详情局部组件生成并落盘 |
| 2026-06-27 | `assets-detail-network-interface` | `/assets` | 资产详情小 Tab（局部详情组件） | 网络接口 | `screens/pages/assets-detail-network-interface.png` | `prompts/assets-detail-network-interface.prompt.txt` | 1920x1080 | 已按资产详情局部组件重生并落盘 |
| 2026-06-27 | `assets-detail-open-services` | `/assets` | 资产详情小 Tab（局部详情组件） | 开放服务 | `screens/pages/assets-detail-open-services.png` | `prompts/assets-detail-open-services.prompt.txt` | 1920x1080 | 已按资产详情局部组件重生并落盘 |
| 2026-06-27 | `assets-detail-ownership` | `/assets` | 资产详情小 Tab（局部详情组件） | 归属信息 | `screens/pages/assets-detail-ownership.png` | `prompts/assets-detail-ownership.prompt.txt` | 1920x1080 | 已按资产详情局部组件重生并落盘 |
| 2026-06-27 | `assets-detail-history` | `/assets` | 资产详情小 Tab（局部详情组件） | 历史变更 | `screens/pages/assets-detail-history.png` | `prompts/assets-detail-history.prompt.txt` | 1920x1080 | 已按资产详情局部组件重生并落盘 |
| 2026-06-26 | `baselines-account` | `/baselines` | 基线类型 Tab | 账号基线 | `screens/pages/baselines-account.png` | `prompts/baselines-account.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `baselines-port` | `/baselines` | 基线类型 Tab | 端口基线 | `screens/pages/baselines-port.png` | `prompts/baselines-port.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `baselines-protocol` | `/baselines` | 基线类型 Tab | 协议基线 | `screens/pages/baselines-protocol.png` | `prompts/baselines-protocol.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `baselines-time-window` | `/baselines` | 基线类型 Tab | 时间段基线 | `screens/pages/baselines-time-window.png` | `prompts/baselines-time-window.prompt.txt` | 1920x1080 | 已生成并落盘 |
| 2026-06-26 | `graph-attack-path` | `/graph` | 路径分析 Tab | 攻击路径 | `screens/pages/graph-attack-path.png` | `prompts/graph-attack-path.prompt.txt` | 1920x1080 | 已重生为小 Tab 独立图并落盘 |
| 2026-06-26 | `graph-communication-path` | `/graph` | 路径分析 Tab | 通信路径 | `screens/pages/graph-communication-path.png` | `prompts/graph-communication-path.prompt.txt` | 1920x1080 | 已确认为小 Tab 独立图并补登记 |
| 2026-06-26 | `graph-account-access-path` | `/graph` | 路径分析 Tab | 账号访问路径 | `screens/pages/graph-account-access-path.png` | `prompts/graph-account-access-path.prompt.txt` | 1920x1080 | 已确认为小 Tab 独立图并补登记 |
| 2026-06-26 | `rules-editor-test-validation` | `/rules` | 规则编辑 Tab | 测试验证 | `screens/pages/rules-editor-test-validation.png` | `prompts/rules-editor-test-validation.prompt.txt` | 1920x1080 | 已按 `rules.png` 横向 Tab 尺寸重生并落盘 |
| 2026-06-26 | `rules-editor-dependencies` | `/rules` | 规则编辑 Tab | 依赖引用 | `screens/pages/rules-editor-dependencies.png` | `prompts/rules-editor-dependencies.prompt.txt` | 1920x1080 | 已按 `rules.png` 横向 Tab 尺寸重生并落盘 |
| 2026-06-26 | `rules-sample-session` | `/rules` | 样本回放 Tab | Session 样本 | `screens/pages/rules-sample-session.png` | `prompts/rules-sample-session.prompt.txt` | 1920x1080 | 已按样本回放验证组件尺寸生成并落盘 |
| 2026-06-26 | `rules-sample-logs` | `/rules` | 样本回放 Tab | 日志样本 | `screens/pages/rules-sample-logs.png` | `prompts/rules-sample-logs.prompt.txt` | 1920x1080 | 已按样本回放验证组件尺寸生成并落盘 |
| 2026-06-26 | `models-feature-rule-contribution` | `/models` | 解释与特征 Tab | 规则贡献 | `screens/pages/models-feature-rule-contribution.png` | `prompts/models-feature-rule-contribution.prompt.txt` | 1920x1080 | 已按解释与特征组件尺寸生成并落盘 |
| 2026-06-26 | `models-feature-anomaly-explanation` | `/models` | 解释与特征 Tab | 异常解释 | `screens/pages/models-feature-anomaly-explanation.png` | `prompts/models-feature-anomaly-explanation.prompt.txt` | 1920x1080 | 已按解释与特征组件尺寸生成并落盘 |
| 2026-06-26 | `models-feature-sample-examples` | `/models` | 解释与特征 Tab | 样本示例 | `screens/pages/models-feature-sample-examples.png` | `prompts/models-feature-sample-examples.prompt.txt` | 1920x1080 | 已按解释与特征组件尺寸生成并落盘 |
| 2026-06-26 | `models-activation-audit-gate` | `/models` | 激活与回滚 Tab | 审计门禁 | `screens/pages/models-activation-audit-gate.png` | `prompts/models-activation-audit-gate.prompt.txt` | 1920x1080 | 已按激活与回滚组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-condition-ip` | `/whitelist` | 条件构造器 Tab | IP | `screens/pages/whitelist-condition-ip.png` | `prompts/whitelist-condition-ip.prompt.txt` | 1920x1080 | 已按条件构造器组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-condition-asset` | `/whitelist` | 条件构造器 Tab | 资产 | `screens/pages/whitelist-condition-asset.png` | `prompts/whitelist-condition-asset.prompt.txt` | 1920x1080 | 已按条件构造器组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-condition-account` | `/whitelist` | 条件构造器 Tab | 账号 | `screens/pages/whitelist-condition-account.png` | `prompts/whitelist-condition-account.prompt.txt` | 1920x1080 | 已按条件构造器组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-condition-rule` | `/whitelist` | 条件构造器 Tab | 规则 | `screens/pages/whitelist-condition-rule.png` | `prompts/whitelist-condition-rule.prompt.txt` | 1920x1080 | 已按条件构造器组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-condition-model` | `/whitelist` | 条件构造器 Tab | 模型 | `screens/pages/whitelist-condition-model.png` | `prompts/whitelist-condition-model.prompt.txt` | 1920x1080 | 已按条件构造器组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-expiry-expired-unhandled` | `/whitelist` | 到期治理 Tab | 过期未处理 | `screens/pages/whitelist-expiry-expired-unhandled.png` | `prompts/whitelist-expiry-expired-unhandled.prompt.txt` | 1920x1080 | 已按到期治理组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-expiry-long-lived` | `/whitelist` | 到期治理 Tab | 长期生效（>180天） | `screens/pages/whitelist-expiry-long-lived.png` | `prompts/whitelist-expiry-long-lived.prompt.txt` | 1920x1080 | 已按到期治理组件尺寸生成并落盘 |
| 2026-06-26 | `whitelist-expiry-unassigned-owner` | `/whitelist` | 到期治理 Tab | 未归属责任角色 | `screens/pages/whitelist-expiry-unassigned-owner.png` | `prompts/whitelist-expiry-unassigned-owner.prompt.txt` | 1920x1080 | 已按到期治理组件尺寸生成并落盘 |
| 2026-06-26 | `alert-detail-evidence-pcap` | `/alerts/:alertId` | 告警证据 Tab | PCAP | `screens/pages/alert-detail-evidence-pcap.png` | `prompts/alert-detail-evidence-pcap.prompt.txt` | 1920x1080 | 已按证据链组件尺寸生成并落盘 |
| 2026-06-26 | `alert-detail-evidence-session` | `/alerts/:alertId` | 告警证据 Tab | Session | `screens/pages/alert-detail-evidence-session.png` | `prompts/alert-detail-evidence-session.prompt.txt` | 1920x1080 | 已按证据链组件尺寸生成并落盘 |
| 2026-06-26 | `alert-detail-evidence-logs` | `/alerts/:alertId` | 告警证据 Tab | 日志 | `screens/pages/alert-detail-evidence-logs.png` | `prompts/alert-detail-evidence-logs.prompt.txt` | 1920x1080 | 已按证据链组件尺寸生成并落盘 |
| 2026-06-26 | `alert-detail-evidence-graph-path` | `/alerts/:alertId` | 告警证据 Tab | 图谱路径 | `screens/pages/alert-detail-evidence-graph-path.png` | `prompts/alert-detail-evidence-graph-path.prompt.txt` | 1920x1080 | 已按证据链组件尺寸生成并落盘 |
| 2026-06-26 | `alert-detail-evidence-files` | `/alerts/:alertId` | 告警证据 Tab | 文件 | `screens/pages/alert-detail-evidence-files.png` | `prompts/alert-detail-evidence-files.prompt.txt` | 1920x1080 | 已按证据链组件尺寸生成并落盘 |
| 2026-06-26 | `audit-log-operation-context` | `/audit-log` | 操作详情 Tab | 操作上下文 | `screens/pages/audit-log-operation-context.png` | `prompts/audit-log-operation-context.prompt.txt` | 1920x1080 | 已按操作详情组件尺寸生成并落盘 |
| 2026-06-26 | `audit-log-related-chain` | `/audit-log` | 操作详情 Tab | 关联链路 | `screens/pages/audit-log-related-chain.png` | `prompts/audit-log-related-chain.prompt.txt` | 1920x1080 | 已按操作详情组件尺寸生成并落盘 |
| 2026-06-26 | `campaign-detail-impact-account` | `/campaigns/:campaignId` | 影响范围 Tab | 账号 | `screens/pages/campaign-detail-impact-account.png` | `prompts/campaign-detail-impact-account.prompt.txt` | 1920x1080 | 已按影响范围组件尺寸生成并落盘 |
| 2026-06-26 | `campaign-detail-impact-service` | `/campaigns/:campaignId` | 影响范围 Tab | 服务 | `screens/pages/campaign-detail-impact-service.png` | `prompts/campaign-detail-impact-service.prompt.txt` | 1920x1080 | 已按影响范围组件尺寸生成并落盘 |
| 2026-06-26 | `campaign-detail-impact-department` | `/campaigns/:campaignId` | 影响范围 Tab | 部门 | `screens/pages/campaign-detail-impact-department.png` | `prompts/campaign-detail-impact-department.prompt.txt` | 1920x1080 | 已按影响范围组件尺寸生成并落盘 |
| 2026-06-26 | `campaign-detail-impact-campus` | `/campaigns/:campaignId` | 影响范围 Tab | 园区 | `screens/pages/campaign-detail-impact-campus.png` | `prompts/campaign-detail-impact-campus.prompt.txt` | 1920x1080 | 已按影响范围组件尺寸生成并落盘 |
| 2026-06-26 | `campaign-detail-impact-business-system` | `/campaigns/:campaignId` | 影响范围 Tab | 业务系统 | `screens/pages/campaign-detail-impact-business-system.png` | `prompts/campaign-detail-impact-business-system.prompt.txt` | 1920x1080 | 已按影响范围组件尺寸生成并落盘 |
专题面板恢复说明：此前误用运行态截图恢复 `topic-tunnel.png`、`topic-exfil.png`、`topic-apt.png`，用户确认这三张不是原始 UI 图，已从正式目录撤下。随后已从 Codex imagegen 会话历史恢复原始三张专题面板 UI 图：`topics-encrypted-tunnel.png`、`topics-data-exfiltration.png`、`topics-apt-campaign.png`，并保留对应 `*.raw-imagegen.png`。三张图仍只作为 `/topics` 单一页面的页内 Tab/Segmented 状态图，不重新拆分左侧菜单，不恢复独立业务路由，也不计入 `manifest.json` 的 163 张主生图清单。

`data-quality-topic-health` 内容覆盖：Kafka Topic 健康明细、offset、积压、消费延迟 P95、分区倾斜热力图、消息大小分布、Consumer Group 健康、异常分区处置队列、右侧质量异常告警与报告动作。

质量备注：主业务内容符合 Tab 矩阵；后续若做公共壳精修，可继续以 `screen.png` 为唯一 AppShell 基准。

`data-quality-flink-quality` 内容覆盖：Flink 作业健康明细、checkpoint 成功率、watermark 延迟 P95、backpressure 热力图、迟到数据与窗口闭合、异常与失败原因、Sink 写入质量、右侧异常告警、快速定位、修复建议与报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/data-quality-flink-quality.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/data-quality-flink-quality.png",
  "size": [1920, 1080],
  "bytes": 1957868
}
```

质量备注：主业务内容符合 Tab 矩阵；`Flink 质量` 激活态明确，保留 `/data-quality` 页面公共 AppShell 和左侧 `采集监测 / 数据质量` 高亮。

`data-quality-field-quality` 内容覆盖：关键字段质量矩阵、五元组、community_id、tenant、asset_id、protocol、timestamp 完整性/格式/一致性校验、字段异常趋势、五元组与 community_id 校验、异常样本表、字段血缘与映射、修复任务与规则建议、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/data-quality-field-quality.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/data-quality-field-quality.png",
  "size": [1920, 1080],
  "bytes": 2044163
}
```

质量备注：主业务内容符合 Tab 矩阵；`字段质量` 激活态明确，主画面不只是通用质量卡片，而是围绕字段矩阵、异常样本、血缘映射和修复任务展开。

`data-quality-storage-quality` 内容覆盖：ClickHouse、OpenSearch、NebulaGraph、MinIO 存储组件健康总览、写入速率与延迟趋势、容量与水位趋势、失败写入与原因列表、索引与归档链路、副本/分片/对象健康、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/data-quality-storage-quality.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/data-quality-storage-quality.png",
  "size": [1920, 1080],
  "bytes": 1913452
}
```

质量备注：主业务内容符合 Tab 矩阵；`存储质量` 激活态明确，主画面围绕写入、索引、图关系、对象归档、容量、失败重试和修复闭环展开。

`data-quality-replay-reconcile` 内容覆盖：DLQ 重放任务表、时间窗对账报告、幂等检查与重复检测、差异样本与原因、重放链路状态、验收证据与导出、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/data-quality-replay-reconcile.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/data-quality-replay-reconcile.png",
  "size": [1920, 1080],
  "bytes": 1843605
}
```

质量备注：主业务内容符合 Tab 矩阵；`重放对账` 激活态明确，主画面围绕重放任务、源端/落库对账、幂等冲突、重复记录、差异样本和验收证据闭环展开。

`data-quality-report` 内容覆盖：质量日报预览、报告章节导航、异常归因摘要、导出记录、验收报告与审批、证据清单补齐、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/data-quality-report.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/data-quality-report.png",
  "size": [1920, 1080],
  "bytes": 1894558
}
```

质量备注：主业务内容符合 Tab 矩阵；`质量报告` 激活态明确，主画面围绕日报预览、章节完成度、异常归因、导出记录、SLA Gate 和审批闭环展开。

`data-quality-settings` 内容覆盖：质量阈值配置、检测规则分组、告警策略与路由、报告周期与模板、保存确认与影响评估、审计记录、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/data-quality-settings.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/data-quality-settings.png",
  "size": [1920, 1080],
  "bytes": 1950091
}
```

质量备注：主业务内容符合 Tab 矩阵；`质量设置` 激活态明确，主画面围绕阈值表、规则开关、告警路由、报告周期、保存审批和审计记录展开。

`encrypted-traffic-fingerprint` 内容覆盖：JA3/JA3S 指纹明细、指纹分布与聚类、证书 issuer 与 SNI 分布、TLS 版本与密码套件矩阵、指纹关联规则、证书详情预览、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/encrypted-traffic-fingerprint.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/encrypted-traffic-fingerprint.png",
  "size": [1920, 1080],
  "bytes": 1995586
}
```

质量备注：主业务内容符合 Tab 矩阵；`指纹分析` 激活态明确，主画面围绕 JA3/JA3S、SNI、issuer、ALPN、TLS 版本、密码套件、规则和证据闭环展开。

`encrypted-traffic-tunnel-detection` 内容覆盖：DoH、异常长连接、低熵/高熵特征、心跳通信、隧道异常列表、熵值与会话时长散点图、心跳通信时间序列、检测规则命中、会话证据预览、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/encrypted-traffic-tunnel-detection.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/encrypted-traffic-tunnel-detection.png",
  "size": [1920, 1080],
  "bytes": 1915201
}
```

质量备注：主业务内容符合 Tab 矩阵；`隧道检测` 激活态明确，主画面围绕 DoH 会话、长连接、熵值聚类、心跳时序、检测规则、会话证据和告警创建闭环展开。

`encrypted-traffic-egress-profile` 内容覆盖：境外 IP、CDN、云服务、异常域名、首次出现目的地、外联目的地地图、域名画像卡片、Top 外联目的地、首次出现与异常域名趋势、实体图谱入口、右侧定位/修复/报告动作。

落盘记录：

```json
{
  "raw": "screens/pages/encrypted-traffic-egress-profile.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/encrypted-traffic-egress-profile.png",
  "size": [1920, 1080],
  "bytes": 2054102
}
```

质量备注：主业务内容符合 Tab 矩阵；`外联画像` 激活态明确，主画面以世界地图外联弧线、域名画像卡、Top 列表和实体图谱入口区分于指纹分析/隧道检测。公共顶部栏与 `screen.png` 仍有轻微漂移，后续如做 AppShell 统一精修可作为批量修复项。

`encrypted-traffic-evidence-center` 内容覆盖：Session、PCAP 索引、证书详情、握手元数据、加密会话证据表、PCAP 索引与切片、证据抽屉预览、证书详情与握手时间线、证据完整度、Hash 与审计校验、右侧取证/报告/审计动作。

落盘记录：

```json
{
  "raw": "screens/pages/encrypted-traffic-evidence-center.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/encrypted-traffic-evidence-center.png",
  "size": [1920, 1080],
  "bytes": 1977878
}
```

质量备注：主业务内容符合 Tab 矩阵；`证据中心` 激活态明确，主画面以证据表格、PCAP 切片、证据抽屉、证书/握手元数据、证据完整度和 Hash 审计校验形成证据工作台。公共标题与 `screen.png` 仍有轻微漂移，后续如做 AppShell 统一精修可作为批量修复项。

资产台账层级改造（2026-06-27）：`assets`、`assets-server`、`assets-network-device`、`assets-business-system`、`assets-unknown` 已重生为 `/assets` 页面级资产分类大 Tab，右侧只保留选中对象摘要，不再混入资产详情内部小 Tab。`assets-detail-basic`、`assets-detail-network-interface`、`assets-detail-open-services`、`assets-detail-ownership`、`assets-detail-history` 已重生为资产详情局部小 Tab，只包含详情组件本体，不包含 AppShell、资产台账列表、页面级资产类型 Tab、筛选区或统计区。

资产台账大 Tab 落盘记录：

```json
[
  {"id": "assets", "raw_size": [1672, 941], "size": [1920, 1080], "bytes": 1982499},
  {"id": "assets-server", "raw_size": [1672, 941], "size": [1920, 1080], "bytes": 1987000},
  {"id": "assets-network-device", "raw_size": [1672, 941], "size": [1920, 1080], "bytes": 2092308},
  {"id": "assets-business-system", "raw_size": [1672, 941], "size": [1920, 1080], "bytes": 2062828},
  {"id": "assets-unknown", "raw_size": [1672, 941], "size": [1920, 1080], "bytes": 2095347}
]
```

资产详情小 Tab 落盘记录：

```json
[
  {"id": "assets-detail-basic", "raw_size": [1024, 1536], "size": [1920, 1080], "bytes": 1462879},
  {"id": "assets-detail-network-interface", "raw_size": [1536, 1024], "size": [1920, 1080], "bytes": 1688250},
  {"id": "assets-detail-open-services", "raw_size": [1536, 1024], "size": [1920, 1080], "bytes": 1666625},
  {"id": "assets-detail-ownership", "raw_size": [1536, 1024], "size": [1920, 1080], "bytes": 1662902},
  {"id": "assets-detail-history", "raw_size": [1536, 1024], "size": [1920, 1080], "bytes": 1702322}
]
```

质量备注：页面级资产分类大 Tab 已分别围绕终端、服务器、网络设备、业务系统、未知资产重组主工作区；右侧摘要区均为当前选中对象摘要。资产详情小 Tab 已分别围绕基础字段、网络接口、开放服务、归属信息、历史变更展开，均未包含公共 AppShell 或资产台账公共区。

`baselines-account` 内容覆盖：账号登录时间、访问资产、异常地理位置、权限漂移、账号基线状态机、账号登录时间箱线图、访问资产基线、异常账号列表、异常地理位置地图、权限漂移矩阵、账号行为表、告警入口与证据、基线版本与治理、右侧偏离解释抽屉和底部证据入口。

落盘记录：

```json
{
  "raw": "screens/pages/baselines-account.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/baselines-account.png",
  "size": [1920, 1080],
  "bytes": 2238039
}
```

质量备注：主业务内容符合 Tab 矩阵；`账号基线` 激活态明确，页面继承 `资产图谱 / 行为基准` 高亮，主画面围绕账号状态机、登录时间箱线图、访问资产关系、异常账号队列、异常地理位置、权限漂移、告警入口和模型反馈闭环展开。

`baselines-port` 内容覆盖：常用端口、新端口、端口扫描、服务变更、端口基线状态机、端口热力图、服务变化趋势、端口偏离表、端口扫描特征、常用端口画像、服务变更明细、告警入口与证据、基线治理与版本、右侧偏离解释抽屉和底部证据入口。

落盘记录：

```json
{
  "raw": "screens/pages/baselines-port.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/baselines-port.png",
  "size": [1920, 1080],
  "bytes": 2142975
}
```

质量备注：主业务内容符合 Tab 矩阵；`端口基线` 激活态明确，页面继承 `资产图谱 / 行为基准` 高亮，主画面围绕端口热力图、服务变化趋势、端口偏离表、扫描特征、服务变更、告警取证入口和端口基线治理闭环展开。

`baselines-protocol` 内容覆盖：协议分布、异常协议、新协议、协议占比漂移、协议基线状态机、协议分布环图、协议占比漂移趋势、协议偏离列表、新协议发现、异常协议画像、协议基线明细、告警入口与证据、基线治理与版本、右侧偏离解释抽屉和底部证据入口。

落盘记录：

```json
{
  "raw": "screens/pages/baselines-protocol.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/baselines-protocol.png",
  "size": [1920, 1080],
  "bytes": 2316264
}
```

质量备注：主业务内容符合 Tab 矩阵；`协议基线` 是大 Tab，激活态明确，页面继承 `资产图谱 / 行为基准` 高亮，主画面围绕协议环图、占比漂移趋势、协议偏离列表、新协议发现、异常协议画像、告警取证入口和协议基线治理闭环展开。

`baselines-time-window` 内容覆盖：工作日/夜间/周末行为、异常时间访问、周期性连接、时间段基线状态机、时间热力图、日历视图、异常时段列表、周期性连接分析、工作日/夜间/周末画像、时间段基线明细、告警入口与证据、基线治理与版本、右侧偏离解释抽屉和底部证据入口。

落盘记录：

```json
{
  "raw": "screens/pages/baselines-time-window.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/baselines-time-window.png",
  "size": [1920, 1080],
  "bytes": 2296558
}
```

质量备注：主业务内容符合 Tab 矩阵；`时间段基线` 是大 Tab，激活态明确，页面继承 `资产图谱 / 行为基准` 高亮，主画面围绕时间热力图、月历视图、异常时段列表、周期性连接、工作日/夜间/周末画像、告警取证入口和时间段基线治理闭环展开。

`graph-attack-path` 内容覆盖：按 Tab 矩阵生成攻击路径小 Tab 独立结果框，只展示攻击阶段、告警节点、横向移动路径、证据锚点、阶段化攻击路径图、路径结果表和跳转攻击链；不得复用最短路径的“源到目标最短路径、边权重、普通通信链路”口径。

落盘记录：

```json
{
  "raw": "screens/pages/graph-attack-path.raw-imagegen.png",
  "raw_size": [1705, 922],
  "target": "screens/pages/graph-attack-path.png",
  "size": [1920, 1080],
  "bytes": 1710191
}
```

质量备注：主业务内容符合 Tab 矩阵；`攻击路径` 是小 Tab 独立结果框，激活态明确，只输出结果性质业务内容，不包含公共 AppShell、宿主图谱画布或业务公共区。它与 `最短路径` 的区别是阶段化攻击链路、告警节点、横向移动、证据锚点和跳转攻击链。

`graph-communication-path` 内容覆盖：按 Tab 矩阵生成通信路径小 Tab 独立结果框，只展示源 IP、目标资产、服务、端口、通信频次、字节数、延迟、通信关系图、边宽权重和 Top 对端列表；不得混入攻击阶段、告警节点或横向移动口径。

落盘记录：

```json
{
  "raw": "screens/pages/graph-communication-path.raw-imagegen.png",
  "raw_size": [1729, 910],
  "target": "screens/pages/graph-communication-path.png",
  "size": [1920, 1080],
  "bytes": 1603627
}
```

质量备注：主业务内容符合 Tab 矩阵；`通信路径` 是小 Tab 独立结果框，激活态明确，只输出结果性质业务内容，不包含公共 AppShell、宿主图谱画布或业务公共区。它与攻击路径的区别是服务端口、通信频次、边宽权重和 Top 对端列表。

`graph-account-access-path` 内容覆盖：按 Tab 矩阵生成账号访问路径小 Tab 独立结果框，只展示账号、主机、服务、资产之间的访问链路、身份标签、异常访问列表、审计/Session/PCAP 证据入口和账号画像跳转；不得混入普通通信 Top 对端或攻击阶段时间线口径。

落盘记录：

```json
{
  "raw": "screens/pages/graph-account-access-path.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/graph-account-access-path.png",
  "size": [1920, 1080],
  "bytes": 1725353
}
```

质量备注：主业务内容符合 Tab 矩阵；`账号访问路径` 是小 Tab 独立结果框，激活态明确，只输出结果性质业务内容，不包含公共 AppShell、宿主图谱画布或业务公共区。它与通信路径的区别是账号身份、访问链路、异常原因和账号画像闭环。

`rules-editor-test-validation` 内容覆盖：按 Tab 矩阵和 `rules.png` 实测规则编辑局部框生成。`rules.png` 中规则编辑局部框约 `x=834 y=211 w=598 h=456`，顶部横向 Tab 行约 `x=846 y=247 w=250 h=39`，单个 Tab 宽约 `82-88px`、高约 `37px`，激活下划线位于 `y=279-280`。重生图只展示顶部横向 `规则定义 / 测试验证 / 依赖引用` 小 Tab 和 `测试验证` 内容，覆盖样本回放、命中结果、误报样本、性能影响、测试面板、结果表、命中差异和性能指标；不包含完整规则管理页面、规则列表、生命周期侧栏或左侧竖向小 Tab。

落盘记录：

```json
{
  "raw": "screens/pages/rules-editor-test-validation.raw-imagegen.png",
  "raw_size": [1437, 1094],
  "target": "screens/pages/rules-editor-test-validation.png",
  "size": [1920, 1080],
  "bytes": 1610092
}
```

质量备注：主业务内容符合 Tab 矩阵；`测试验证` 按 `rules.png` 横向 Tab 尺寸重生，激活态明确，只输出规则编辑局部 Tab 内容，含回放控制、命中差异、性能影响、验证结果表和误报闭环。

`rules-editor-dependencies` 内容覆盖：按 Tab 矩阵和 `rules.png` 实测规则编辑局部框生成。重生图只展示顶部横向 `规则定义 / 测试验证 / 依赖引用` 小 Tab 和 `依赖引用` 内容，覆盖关联模型、白名单、部署、数据源、字段、告警类型、依赖关系表、引用图和影响范围提示；不包含完整规则管理页面、规则列表、生命周期侧栏或左侧竖向小 Tab。

落盘记录：

```json
{
  "raw": "screens/pages/rules-editor-dependencies.raw-imagegen.png",
  "raw_size": [1584, 993],
  "target": "screens/pages/rules-editor-dependencies.png",
  "size": [1920, 1080],
  "bytes": 1752617
}
```

质量备注：主业务内容符合 Tab 矩阵；`依赖引用` 按 `rules.png` 横向 Tab 尺寸重生，激活态明确，只输出规则编辑局部 Tab 内容，含依赖引用图、影响范围提示、依赖关系表和影响报告闭环。

`rules-sample-session` 内容覆盖：按用户提供的 `样本回放验证（近 7 天）` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x392`，可见外框约 `501x368`；同组件在 `rules.png` 中约为 `x=205 y=672 w=360 h=264`。重生图只展示样本回放验证局部组件，顶部横向 `PCAP 样本 32 / Session 样本 128 / 日志样本 256` Tab 中 `Session 样本 128` 激活，覆盖 Session 样本、五元组、协议摘要、命中字段、Session 样本表、字段命中高亮和结果对比；不包含完整规则管理页面、规则编辑器、规则列表或左侧导航。

落盘记录：

```json
{
  "raw": "screens/pages/rules-sample-session.raw-imagegen.png",
  "raw_size": [1555, 1011],
  "target": "screens/pages/rules-sample-session.png",
  "size": [1920, 1080],
  "bytes": 1477891
}
```

质量备注：主业务内容符合 Tab 矩阵；`Session 样本` 按样本回放验证组件尺寸生成，激活态明确，表格包含五元组、协议摘要、命中字段高亮和查看全部样本闭环。

`rules-sample-logs` 内容覆盖：按用户提供的 `样本回放验证（近 7 天）` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x392`，可见外框约 `501x368`；同组件在 `rules.png` 中约为 `x=205 y=672 w=360 h=264`。重生图只展示样本回放验证局部组件，顶部横向 `PCAP 样本 32 / Session 样本 128 / 日志样本 256` Tab 中 `日志样本 256` 激活，覆盖设备日志、用户事件、规则字段、命中原因、日志样本表、字段高亮和误报标记；不包含完整规则管理页面、规则编辑器、规则列表或左侧导航。

落盘记录：

```json
{
  "raw": "screens/pages/rules-sample-logs.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/rules-sample-logs.png",
  "size": [1920, 1080],
  "bytes": 1308362
}
```

质量备注：主业务内容符合 Tab 矩阵；`日志样本` 按样本回放验证组件尺寸生成，激活态明确，表格包含规则字段高亮、命中原因、查看/标记动作和误报标记闭环。

`models-feature-rule-contribution` 内容覆盖：按用户提供的 `解释与特征` 小组件和 Tab 矩阵生成。参考图尺寸为 `516x371`，可见外框约 `512x360`；同组件在 `models.png` 中约为 `x=842 y=521 w=580 h=388`。重生图只展示模型解释与特征局部组件，顶部横向 `重要特征 / 规则贡献 / 异常解释 / 样本示例` Tab 中 `规则贡献` 激活，覆盖规则命中特征、规则权重、模型融合贡献、贡献表、瀑布图和规则关联入口；不包含完整模型管理页面、模型列表、数据集面板或激活状态机。

落盘记录：

```json
{
  "raw": "screens/pages/models-feature-rule-contribution.raw-imagegen.png",
  "raw_size": [1498, 1050],
  "target": "screens/pages/models-feature-rule-contribution.png",
  "size": [1920, 1080],
  "bytes": 1450786
}
```

质量备注：主业务内容符合 Tab 矩阵；`规则贡献` 按解释与特征组件尺寸生成，激活态明确，表格包含命中特征、权重、贡献值和关联规则闭环。

`models-feature-anomaly-explanation` 内容覆盖：按用户提供的 `解释与特征` 小组件和 Tab 矩阵生成。参考图尺寸为 `516x371`，可见外框约 `512x360`；同组件在 `models.png` 中约为 `x=842 y=521 w=580 h=388`。重生图只展示模型解释与特征局部组件，顶部横向 `重要特征 / 规则贡献 / 异常解释 / 样本示例` Tab 中 `异常解释` 激活，覆盖异常原因、特征偏离、相似样本、置信区间、解释卡、偏离图和相似样本列表；不包含完整模型管理页面、模型列表、数据集面板或激活状态机。

落盘记录：

```json
{
  "raw": "screens/pages/models-feature-anomaly-explanation.raw-imagegen.png",
  "raw_size": [1609, 977],
  "target": "screens/pages/models-feature-anomaly-explanation.png",
  "size": [1920, 1080],
  "bytes": 1614007
}
```

质量备注：主业务内容符合 Tab 矩阵；`异常解释` 按解释与特征组件尺寸生成，激活态明确，包含异常原因解释卡、特征偏离条、置信区间和相似样本闭环。

`models-feature-sample-examples` 内容覆盖：按用户提供的 `解释与特征` 小组件和 Tab 矩阵生成。参考图尺寸为 `516x371`，可见外框约 `512x360`；同组件在 `models.png` 中约为 `x=842 y=521 w=580 h=388`。重生图只展示模型解释与特征局部组件，顶部横向 `重要特征 / 规则贡献 / 异常解释 / 样本示例` Tab 中 `样本示例` 激活，覆盖 TP/FP 样本、误报原因、训练/验证/测试集样本、样本表、标签分布和抽样预览；不包含完整模型管理页面、模型列表、数据集面板或激活状态机。

落盘记录：

```json
{
  "raw": "screens/pages/models-feature-sample-examples.raw-imagegen.png",
  "raw_size": [1615, 974],
  "target": "screens/pages/models-feature-sample-examples.png",
  "size": [1920, 1080],
  "bytes": 1490327
}
```

质量备注：主业务内容符合 Tab 矩阵；`样本示例` 按解释与特征组件尺寸生成，激活态明确，包含标签分布、抽样预览、TP/FP 样本表、误报原因和预览闭环。

`models-activation-audit-gate` 内容覆盖：按用户提供的 `激活与回滚` 小组件和 Tab 矩阵生成。参考图尺寸为 `755x423`，可见外框约 `748x404`；同组件在 `models.png` 中约为 `x=1280 y=633 w=600 h=309`。重生图只展示模型激活与回滚局部组件，顶部横向 `激活流程 / 审计门禁` Tab 中 `审计门禁` 激活，覆盖激活审批、回归集门禁、漂移阈值、风险确认、发布审计、门禁矩阵、审批流、风险提示和审计记录；不包含完整模型管理页面、模型列表、数据集面板或解释与特征组件。

落盘记录：

```json
{
  "raw": "screens/pages/models-activation-audit-gate.raw-imagegen.png",
  "raw_size": [1665, 944],
  "target": "screens/pages/models-activation-audit-gate.png",
  "size": [1920, 1080],
  "bytes": 1405495
}
```

质量备注：主业务内容符合 Tab 矩阵；`审计门禁` 按激活与回滚组件尺寸生成，激活态明确，包含门禁矩阵、审批流、审计记录、审计报告/继续审批/驳回发布闭环。

`whitelist-condition-ip` 内容覆盖：按用户提供的 `B. 条件构造器 / 新增白名单草案` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x450`，可见外框约 `491x441`；同组件在 `whitelist.png` 中约为 `x=835 y=215 w=520 h=455`。重生图只展示白名单条件构造器局部组件，顶部横向 `IP / 资产 / 账号 / 域名 / 规则 / 模型` Tab 中 `IP` 激活，覆盖 IP、CIDR、源/目的方向、生效范围、IP 条件表、范围标签和风险提示；不包含完整白名单页面、白名单列表、审批区或到期治理区。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-condition-ip.raw-imagegen.png",
  "raw_size": [1324, 1188],
  "target": "screens/pages/whitelist-condition-ip.png",
  "size": [1920, 1080],
  "bytes": 1552545
}
```

质量备注：主业务内容符合 Tab 矩阵；`IP` 按条件构造器组件尺寸生成，激活态明确，包含 IP/CIDR、方向、范围、关联告警、标签和影响评估闭环。

`whitelist-condition-asset` 内容覆盖：按用户提供的 `B. 条件构造器 / 新增白名单草案` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x450`，可见外框约 `491x441`；同组件在 `whitelist.png` 中约为 `x=835 y=215 w=520 h=455`。重生图只展示白名单条件构造器局部组件，顶部横向 `IP / 资产 / 账号 / 域名 / 规则 / 模型` Tab 中 `资产` 激活，覆盖资产 ID、资产组、业务系统、园区、部门、资产选择器、范围树和影响资产预览；不包含完整白名单页面、白名单列表、审批区或到期治理区。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-condition-asset.raw-imagegen.png",
  "raw_size": [1183, 1330],
  "target": "screens/pages/whitelist-condition-asset.png",
  "size": [1920, 1080],
  "bytes": 1325897
}
```

质量备注：主业务内容符合 Tab 矩阵；`资产` 按条件构造器组件尺寸生成，激活态明确，包含资产对象、资产组、业务系统、园区/部门、范围标签和影响资产评估。

`whitelist-condition-account` 内容覆盖：按用户提供的 `B. 条件构造器 / 新增白名单草案` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x450`，可见外框约 `491x441`；同组件在 `whitelist.png` 中约为 `x=835 y=215 w=520 h=455`。重生图只展示白名单条件构造器局部组件，顶部横向 `IP / 资产 / 账号 / 域名 / 规则 / 模型` Tab 中 `账号` 激活，覆盖用户账号、服务账号、登录源、访问目标、账号选择器、访问范围和异常提示；不包含完整白名单页面、白名单列表、审批区或到期治理区。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-condition-account.raw-imagegen.png",
  "raw_size": [1185, 1327],
  "target": "screens/pages/whitelist-condition-account.png",
  "size": [1920, 1080],
  "bytes": 1640820
}
```

质量备注：主业务内容符合 Tab 矩阵；`账号` 按条件构造器组件尺寸生成，激活态明确，包含账号类型、登录源、访问目标、周期时间窗、异常原因和影响评估。

`whitelist-condition-rule` 内容覆盖：按用户提供的 `B. 条件构造器 / 新增白名单草案` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x450`，可见外框约 `491x441`；同组件在 `whitelist.png` 中约为 `x=835 y=215 w=520 h=455`。重生图只展示白名单条件构造器局部组件，顶部横向 `IP / 资产 / 账号 / 域名 / 规则 / 模型` Tab 中 `规则` 激活，覆盖规则 ID、规则类型、命中字段、例外条件、规则选择器、命中趋势和误报样本链接；不包含完整白名单页面、白名单列表、审批区或到期治理区。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-condition-rule.raw-imagegen.png",
  "raw_size": [1182, 1330],
  "target": "screens/pages/whitelist-condition-rule.png",
  "size": [1920, 1080],
  "bytes": 1446015
}
```

质量备注：主业务内容符合 Tab 矩阵；`规则` 按条件构造器组件尺寸生成，激活态明确，包含规则类型、命中字段、例外 DSL、误报样本入口、标签和影响规则评估。

`whitelist-condition-model` 内容覆盖：按用户提供的 `B. 条件构造器 / 新增白名单草案` 小组件和 Tab 矩阵生成。参考图尺寸为 `504x450`，可见外框约 `491x441`；同组件在 `whitelist.png` 中约为 `x=835 y=215 w=520 h=455`。重生图只展示白名单条件构造器局部组件，顶部横向 `IP / 资产 / 账号 / 域名 / 规则 / 模型` Tab 中 `模型` 激活，覆盖模型版本、特征条件、置信度阈值、样本来源、模型选择器、阈值滑块和样本预览；不包含完整白名单页面、白名单列表、审批区或到期治理区。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-condition-model.raw-imagegen.png",
  "raw_size": [1177, 1336],
  "target": "screens/pages/whitelist-condition-model.png",
  "size": [1920, 1080],
  "bytes": 1646447
}
```

质量备注：主业务内容符合 Tab 矩阵；`模型` 按条件构造器组件尺寸生成，激活态明确，包含模型版本、特征条件、置信度阈值、样本来源、样本入口、标签和影响模型评估。

`whitelist-expiry-expired-unhandled` 内容覆盖：按用户提供的 `E. 到期治理` 小组件和 Tab 矩阵生成。参考图尺寸为 `588x410`，可见外框约 `588x390`；重生图只展示白名单到期治理局部组件，顶部横向 `即将到期（7天内） / 过期未处理 / 长期生效（>180天） / 未归属责任角色` Tab 中 `过期未处理` 激活，覆盖已过期但仍命中的白名单、风险说明、处理 SLA、风险列表、超时标签和停用确认；不包含完整白名单页面、条件构造器、审批流程或主列表区域。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-expiry-expired-unhandled.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/whitelist-expiry-expired-unhandled.png",
  "size": [1920, 1080],
  "bytes": 1320143
}
```

质量备注：主业务内容符合 Tab 矩阵；`过期未处理` 按到期治理组件尺寸生成，激活态明确，包含超期天数、风险/SLA 标签、责任角色和停用/补审/指派闭环。

`whitelist-expiry-long-lived` 内容覆盖：按用户提供的 `E. 到期治理` 小组件和 Tab 矩阵生成。参考图尺寸为 `588x410`，可见外框约 `588x390`；重生图只展示白名单到期治理局部组件，顶部横向 `即将到期（7天内） / 过期未处理 / 长期生效（>180天） / 未归属责任角色` Tab 中 `长期生效（>180天）` 激活，覆盖长期例外、复审周期、漏报风险、业务依据、长期白名单表、复审状态和风险矩阵；不包含完整白名单页面、条件构造器、审批流程或主列表区域。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-expiry-long-lived.raw-imagegen.png",
  "raw_size": [1717, 916],
  "target": "screens/pages/whitelist-expiry-long-lived.png",
  "size": [1920, 1080],
  "bytes": 1473688
}
```

质量备注：主业务内容符合 Tab 矩阵；`长期生效（>180天）` 按到期治理组件尺寸生成，激活态明确，包含已生效天数、复审周期、业务依据、漏报风险、复审状态和延期/停用闭环。

`whitelist-expiry-unassigned-owner` 内容覆盖：按用户提供的 `E. 到期治理` 小组件和 Tab 矩阵生成。参考图尺寸为 `588x410`，可见外框约 `588x390`；重生图只展示白名单到期治理局部组件，顶部横向 `即将到期（7天内） / 过期未处理 / 长期生效（>180天） / 未归属责任角色` Tab 中 `未归属责任角色` 激活，覆盖无责任人、责任角色失效、组织变更后的例外项、未归属队列、责任人分配抽屉入口和审计提示；不包含完整白名单页面、条件构造器、审批流程或主列表区域。

落盘记录：

```json
{
  "raw": "screens/pages/whitelist-expiry-unassigned-owner.raw-imagegen.png",
  "raw_size": [1672, 941],
  "target": "screens/pages/whitelist-expiry-unassigned-owner.png",
  "size": [1920, 1080],
  "bytes": 1291661
}
```

质量备注：主业务内容符合 Tab 矩阵；`未归属责任角色` 按到期治理组件尺寸生成，激活态明确，包含来源、原责任角色、失效原因、风险等级和指派/分配/审计/停用闭环。

`alert-detail-evidence-pcap` 内容覆盖：按用户提供的 `证据链（6）` 小组件和 Tab 矩阵生成。参考图尺寸为 `1338x369`，可见外框约 `1336x363`；重生图只展示告警详情证据链局部组件，顶部横向 `全部 6 / PCAP 1 / Session 2 / 日志 1 / 图谱路径 1 / 文件 1` Tab 中 `PCAP 1` 激活，覆盖 PCAP 切片、对象路径、大小、hash、下载审计、PCAP 表格、hash 标签、下载按钮和校验状态；不包含告警详情公共区、告警摘要、处置面板或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/alert-detail-evidence-pcap.raw-imagegen.png",
  "raw_size": [1853, 849],
  "target": "screens/pages/alert-detail-evidence-pcap.png",
  "size": [1920, 1080],
  "bytes": 1191215
}
```

质量备注：主业务内容符合 Tab 矩阵；`PCAP 1` 按证据链组件尺寸生成，激活态明确，包含对象路径、SHA256、校验状态、下载审计和下载/预览闭环。

`alert-detail-evidence-session` 内容覆盖：按用户提供的 `证据链（6）` 小组件和 Tab 矩阵生成。参考图尺寸为 `1338x369`，可见外框约 `1336x363`；重生图只展示告警详情证据链局部组件，顶部横向 `全部 6 / PCAP 1 / Session 2 / 日志 1 / 图谱路径 1 / 文件 1` Tab 中 `Session 2` 激活，覆盖会话五元组、请求响应摘要、字节数、持续时间、Session 表、时间轴和会话详情抽屉入口；不包含告警详情公共区、告警摘要、处置面板或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/alert-detail-evidence-session.raw-imagegen.png",
  "raw_size": [1877, 838],
  "target": "screens/pages/alert-detail-evidence-session.png",
  "size": [1920, 1080],
  "bytes": 1321696
}
```

质量备注：主业务内容符合 Tab 矩阵；`Session 2` 按证据链组件尺寸生成，激活态明确，包含两条 Session、五元组、持续时间、关联 PCAP、时间轴和详情/预览闭环。

`alert-detail-evidence-logs` 内容覆盖：按用户提供的 `证据链（6）` 小组件和 Tab 矩阵生成。参考图尺寸为 `1338x369`，可见外框约 `1336x363`；重生图只展示告警详情证据链局部组件，顶部横向 `全部 6 / PCAP 1 / Session 2 / 日志 1 / 图谱路径 1 / 文件 1` Tab 中 `日志 1` 激活，覆盖设备日志、规则命中日志、用户事件、系统日志、日志表、来源标签、字段高亮和检索过滤；不包含告警详情公共区、告警摘要、处置面板或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/alert-detail-evidence-logs.raw-imagegen.png",
  "raw_size": [2167, 726],
  "target": "screens/pages/alert-detail-evidence-logs.png",
  "size": [1920, 1080],
  "bytes": 1276370
}
```

质量备注：主业务内容符合 Tab 矩阵；`日志 1` 按证据链组件尺寸生成，激活态明确，包含命中字段、关键字段高亮、来源标签、检索入口和预览闭环。

`alert-detail-evidence-graph-path` 内容覆盖：按用户提供的 `证据链（6）` 小组件和 Tab 矩阵生成。参考图尺寸为 `1338x369`，可见外框约 `1336x363`；重生图只展示告警详情证据链局部组件，顶部横向 `全部 6 / PCAP 1 / Session 2 / 日志 1 / 图谱路径 1 / 文件 1` Tab 中 `图谱路径 1` 激活，覆盖告警资产、账号、域名、服务之间的路径证据、小型关系图、路径列表、边权重和跳转实体图谱入口；不包含告警详情公共区、告警摘要、实体图谱主画布或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/alert-detail-evidence-graph-path.raw-imagegen.png",
  "raw_size": [1902, 827],
  "target": "screens/pages/alert-detail-evidence-graph-path.png",
  "size": [1920, 1080],
  "bytes": 1370049
}
```

质量备注：主业务内容符合 Tab 矩阵；`图谱路径 1` 按证据链组件尺寸生成，激活态明确，只嵌入小型路径关系图，包含边权重、风险评分、关联资源和跳转闭环。

`alert-detail-evidence-files` 内容覆盖：按用户提供的 `证据链（6）` 小组件和 Tab 矩阵生成。参考图尺寸为 `1338x369`，可见外框约 `1336x363`；重生图只展示告警详情证据链局部组件，顶部横向 `全部 6 / PCAP 1 / Session 2 / 日志 1 / 图谱路径 1 / 文件 1` Tab 中 `文件 1` 激活，覆盖附件、报告、脚本、导出文件、hash、签名 URL、文件列表、类型图标、hash 校验和下载审计；不包含告警详情公共区、告警摘要、处置面板或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/alert-detail-evidence-files.raw-imagegen.png",
  "raw_size": [2169, 725],
  "target": "screens/pages/alert-detail-evidence-files.png",
  "size": [1920, 1080],
  "bytes": 1267696
}
```

质量备注：主业务内容符合 Tab 矩阵；`文件 1` 按证据链组件尺寸生成，激活态明确，包含 hash 清单、签名 URL、文件标签、下载审计、校验状态和下载/预览闭环。

`audit-log-operation-context` 内容覆盖：按用户提供的 `操作详情 / Diff 视图` 小组件和 Tab 矩阵生成。参考图尺寸为 `707x456`，可见外框约 `705x449`；重生图只展示审计日志操作详情局部组件，顶部横向 `字段变更对比 / 操作上下文 / 关联链路` Tab 中 `操作上下文` 激活，覆盖用户、租户、IP、User-Agent、trace_id、请求 ID、上下文字段卡、请求链路和来源页面入口；不包含完整审计日志列表、检索区、公共 AppShell 或字段变更对比表作为主内容。

落盘记录：

```json
{
  "raw": "screens/pages/audit-log-operation-context.raw-imagegen.png",
  "raw_size": [1536, 1024],
  "target": "screens/pages/audit-log-operation-context.png",
  "size": [1920, 1080],
  "bytes": 1432112
}
```

质量备注：主业务内容符合 Tab 矩阵；`操作上下文` 按操作详情组件尺寸生成，激活态明确，包含用户/租户/角色、来源 IP、User-Agent、请求 ID、trace_id、会话 ID、请求链路、来源页面和上下文校验闭环。

`audit-log-related-chain` 内容覆盖：按用户提供的 `操作详情 / Diff 视图` 小组件和 Tab 矩阵生成。参考图尺寸为 `707x456`，可见外框约 `705x449`；重生图只展示审计日志操作详情局部组件，顶部横向 `字段变更对比 / 操作上下文 / 关联链路` Tab 中 `关联链路` 激活，覆盖告警、证据、规则、模型、部署、白名单、合规报告审计链、关系链、时间线和业务对象跳转；不包含完整审计日志列表、检索区、公共 AppShell 或字段变更对比表作为主内容。

落盘记录：

```json
{
  "raw": "screens/pages/audit-log-related-chain.raw-imagegen.png",
  "raw_size": [1561, 1008],
  "target": "screens/pages/audit-log-related-chain.png",
  "size": [1920, 1080],
  "bytes": 1490824
}
```

质量备注：主业务内容符合 Tab 矩阵；`关联链路` 按操作详情组件尺寸生成，激活态明确，包含告警、证据、规则、模型、部署、白名单、合规报告节点链路、关系表、审计提示和跳转闭环。

`campaign-detail-impact-account` 内容覆盖：按用户提供的 `影响范围` 小组件和 Tab 矩阵生成。参考图尺寸为 `450x591`，可见外框约 `438x557`；重生图只展示战役详情影响范围局部组件，顶部横向 `资产 / 账号 / 服务 / 部门 / 园区 / 业务系统` Tab 中 `账号` 激活，覆盖受影响账号、账号类型、权限风险、登录链路、异常访问标签和查看全部入口；不包含战役详情公共区、时间线、证据包、复盘结论或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/campaign-detail-impact-account.raw-imagegen.png",
  "raw_size": [1004, 1567],
  "target": "screens/pages/campaign-detail-impact-account.png",
  "size": [1920, 1080],
  "bytes": 1450908
}
```

质量备注：主业务内容符合 Tab 矩阵；`账号` 按影响范围组件尺寸生成，激活态明确，包含账号表、权限风险标签、登录链路和查看全部账号闭环。

`campaign-detail-impact-service` 内容覆盖：按用户提供的 `影响范围` 小组件和 Tab 矩阵生成。参考图尺寸为 `450x591`，可见外框约 `438x557`；重生图只展示战役详情影响范围局部组件，顶部横向 `资产 / 账号 / 服务 / 部门 / 园区 / 业务系统` Tab 中 `服务` 激活，覆盖受影响服务、端口、协议、依赖关系、风险标签和查看全部入口；不包含战役详情公共区、时间线、证据包、复盘结论或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/campaign-detail-impact-service.raw-imagegen.png",
  "raw_size": [1086, 1448],
  "target": "screens/pages/campaign-detail-impact-service.png",
  "size": [1920, 1080],
  "bytes": 1409251
}
```

质量备注：主业务内容符合 Tab 矩阵；`服务` 按影响范围组件尺寸生成，激活态明确，包含服务列表、端口/协议、依赖关系、风险标签和查看全部服务闭环。

`campaign-detail-impact-department` 内容覆盖：按用户提供的 `影响范围` 小组件和 Tab 矩阵生成。参考图尺寸为 `450x591`，可见外框约 `438x557`；重生图只展示战役详情影响范围局部组件，顶部横向 `资产 / 账号 / 服务 / 部门 / 园区 / 业务系统` Tab 中 `部门` 激活，覆盖部门影响面、责任人、处置进度、部门矩阵、风险标签和查看全部入口；不包含战役详情公共区、时间线、证据包、复盘结论或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/campaign-detail-impact-department.raw-imagegen.png",
  "raw_size": [1086, 1448],
  "target": "screens/pages/campaign-detail-impact-department.png",
  "size": [1920, 1080],
  "bytes": 1579992
}
```

质量备注：主业务内容符合 Tab 矩阵；`部门` 按影响范围组件尺寸生成，激活态明确，包含责任人、风险标签、处置进度条和查看全部部门闭环。

`campaign-detail-impact-campus` 内容覆盖：按用户提供的 `影响范围` 小组件和 Tab 矩阵生成。参考图尺寸为 `450x591`，可见外框约 `438x557`；重生图只展示战役详情影响范围局部组件，顶部横向 `资产 / 账号 / 服务 / 部门 / 园区 / 业务系统` Tab 中 `园区` 激活，覆盖园区风险分布、楼宇、链路、资产覆盖、链路高亮和查看全部入口；不包含战役详情公共区、时间线、证据包、复盘结论或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/campaign-detail-impact-campus.raw-imagegen.png",
  "raw_size": [1080, 1456],
  "target": "screens/pages/campaign-detail-impact-campus.png",
  "size": [1920, 1080],
  "bytes": 1254752
}
```

质量备注：主业务内容符合 Tab 矩阵；`园区` 按影响范围组件尺寸生成，激活态明确，包含园区/楼宇列表、覆盖资产、链路标签、风险分布和查看全部园区闭环。

`campaign-detail-impact-business-system` 内容覆盖：按用户提供的 `影响范围` 小组件和 Tab 矩阵生成。参考图尺寸为 `450x591`，可见外框约 `438x557`；重生图只展示战役详情影响范围局部组件，顶部横向 `资产 / 账号 / 服务 / 部门 / 园区 / 业务系统` Tab 中 `业务系统` 激活，覆盖业务系统影响、关键服务、依赖提示、恢复优先级、风险标签和查看全部入口；不包含战役详情公共区、时间线、证据包、复盘结论或完整 AppShell。

落盘记录：

```json
{
  "raw": "screens/pages/campaign-detail-impact-business-system.raw-imagegen.png",
  "raw_size": [1086, 1448],
  "target": "screens/pages/campaign-detail-impact-business-system.png",
  "size": [1920, 1080],
  "bytes": 1310527
}
```

质量备注：主业务内容符合 Tab 矩阵；`业务系统` 按影响范围组件尺寸生成，激活态明确，包含业务系统表、关键服务、恢复优先级标签、风险分布和查看全部业务系统闭环。

## 下一张

显式小 Tab 独立图队列已完成：

```text
无
```

目标文件：

```text
无
```

Tab 层级：

```text
小 Tab 队列已完成
```

必须画出来：

- 无

主要表现形式：

- 无

## 剩余队列

剩余 0 张，显式小 Tab 独立图队列已完成：

```text
无
```

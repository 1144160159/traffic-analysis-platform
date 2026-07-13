# GPT 生成图存档台账

更新时间：2026-06-20

## 存档规则

每次调用 GPT 画图功能后，必须把图片和提示词纳入本台账。

| 要求 | 说明 |
|---|---|
| 图片落盘 | 最终 PNG 保存到 `doc/04_assets/generated/` |
| 提示词落盘 | 同目录保存 `.prompt.txt`，文件名与 PNG 对应 |
| 状态标记 | 标注为 `adopted`、`superseded`、`rejected` 或 `pending-export` |
| 不只留聊天 | 聊天内显示不能作为唯一存档来源 |
| 设计约束 | 产品标题统一为“园区网络全流量采集与分析系统”；左侧一级菜单使用业务域；需要展示二级菜单时必须在提示词中显式约束 |

## 2026-06-19

| 时间 | 文件 | 状态 | 说明 |
|---|---|---|---|
| 2026-06-19 16:15 | `situation_screen_concept_chatgpt_20260619.png` | superseded | 早期态势感知大屏概念图，可作为氛围和蓝图参考 |
| 2026-06-19 16:59 | `command_workbench_nav_standard_attempt1_no_submenu_20260619.png` | rejected | 一级菜单改成业务域，但未展开二级菜单 |
| 2026-06-19 17:03 | `command_workbench_two_level_nav_attempt2_old_title_20260619.png` | superseded | 已展开威胁分析二级菜单，但画面标题仍沿用旧方向名 |
| 2026-06-19 17:07 | `campus_full_traffic_system_two_level_nav_20260619.png` | superseded | 标题为“园区网络全流量采集与分析系统”，左侧保留一级业务域和二级菜单；已被后续 GPT 视觉参考稿替代 |
| 2026-06-19 17:41 | `campus_full_traffic_system_topology_stats_gpt_20260619.png` | pending-export | 已通过 GPT 生成聊天图像；仅在园区拓扑总览位置增加楼宇、终端数、关键资产、在线探针信息卡。内置图像工具未暴露可复制本地 PNG 路径，等待导出后补齐图片文件 |
| 2026-06-19 17:48 | `campus_full_traffic_system_title_nav_gpt_20260619.png` | pending-export | 已通过 GPT 生成聊天图像；将左上“指挥研判台 / Command Workbench”替换为“园区网络全流量采集与分析系统”，并将左侧菜单替换为第三张双层菜单参考，其余保持不变。内置图像工具未暴露可复制本地 PNG 路径，等待导出后补齐图片文件 |
| 2026-06-19 18:01 | `campus_full_traffic_system_topology_header_gpt_20260619.png` | pending-export | 已通过 GPT 生成聊天图像；仅将数字孪生/拓扑面板标题栏替换为“园区拓扑总览（数字孪生）”和右侧工具按钮，其余保持不变。内置图像工具未暴露可复制本地 PNG 路径，等待导出后补齐图片文件 |
| 2026-06-19 18:39 | `campus_full_traffic_system_chinese_typography_gpt_20260619.png` | pending-export | 已通过 GPT 生成聊天图像；剔除面板标题中的英文副标题并统一页面字体层级，技术专名 Kafka/Flink/ClickHouse/PCAP 等保留。内置图像工具未暴露可复制本地 PNG 路径，等待导出后补齐图片文件 |
| 2026-06-19 18:44 | `campus_full_traffic_system_chinese_titles_pixel_preserve_attempt_gpt_20260619.png` | rejected | 已通过 GPT 生成聊天图像；虽然提示词要求像素级保留内容，但输出仍重绘了拓扑、局部文字和部分布局，违反“不允许改变图的内容”，不得作为采用稿 |
| 2026-06-19 19:10 | `campus_full_traffic_system_final_visual_cn_titles_20260619.png` | superseded | 基于用户确认截图做本地标题行局部修订，未调用 GPT 重绘；清理带英文副标题的标题行并按“园区拓扑总览（数字孪生）”同等像素高度重写中文标题，业务数据、图表、拓扑、菜单和右侧详情不变；已被后续 GPT 视觉参考稿替代 |
| 2026-06-19 20:46 | `campus_full_traffic_system_visual_reference_gpt_20260619.png` | superseded | 用户导出的 GPT 语义重生成视觉参考稿；统一中文面板标题字号，保留“园区网络全流量采集与分析系统”产品标题、左侧二级菜单、采集链路、告警研判、取证证据、响应处置、反馈学习和验收证据闭环；因底部状态栏缺失，已被补底版本替代 |
| 2026-06-19 22:52 | `campus_full_traffic_system_visual_reference_gpt_20260619_bottom_restored_extended.png` | superseded | 基于用户补充的底部状态栏截图做本地确定性拼接；因业务口径会造成误解，已按用户要求删除本地 PNG，并由 2026-06-20 业务修正版替代 |
| 2026-06-19 22:20 | `ui_suite_gpt_v1/screens/**/*.png` | pending-export | 已完成 28 张页面和 24 张浮层的 GPT 生图 prompt、manifest 与本地生成脚本；`alerts` 真实试跑已连通 OpenAI，但返回 `billing_hard_limit_reached`，等待账单硬限额解除或切换可用 key 后批量输出 PNG |
| 2026-06-19 22:35 | `ui_suite_gpt_v1/screens/pages/alerts.png` | pending-export | 已通过内置 GPT 生图工具先生成一张“告警中心”页面预览；当前工具未在 `$CODEX_HOME`、`/root`、`/tmp`、`/home/wangwt` 暴露可复制本地图片路径，等待导出或 API 账单恢复后补齐 PNG |
| 2026-06-20 00:18 | `ui_suite_gpt_v1/screens/pages/screen.png` | pending-export | 已通过内置 GPT ImageGen 生成“综合态势 / 态势大屏”页面预览，采用六组左侧菜单并突出园区拓扑、采集链路、流量态势、威胁态势、证据闭环和验收指标；当前工具未在 `$CODEX_HOME`、`/tmp` 或项目目录暴露可复制本地图片路径，提示词已保存到 `screen_overview_imagegen_prompt_20260620.md` |
| 2026-06-20 04:04 | `ui_suite_gpt_v1/screens/pages/screen.png` | pending-export | 已再次通过内置 GPT ImageGen 按最终视觉基线生成“综合态势 / 态势大屏”首图；提示词保存到 `doc/04_assets/ui_suite_gpt_v1/prompts/screen-chat-imagegen-v1.prompt.txt`。已尝试 Computer Use 自动下载，但当前会话返回 `Computer Use native pipe path is unavailable`；Windows 收件箱同步 `pulled_count=0`，等待图片对象下载或导出后补齐 PNG |
| 2026-06-20 | `campus_full_traffic_system_visual_reference_20260620_business_corrected.png` | adopted | 用户提供并确认的新最终视觉参考基线；修正旧图业务口径，当前页为“综合态势 / 态势大屏”，左侧六组一级导航、综合态势二级菜单、采集流处理、告警队列、取证证据、响应反馈和验收证据闭环均作为高保真 UI 套装生成基线 |

## 当前采用图

- 图片：`campus_full_traffic_system_visual_reference_20260620_business_corrected.png`
- 来源说明：`campus_full_traffic_system_visual_reference_20260620_business_corrected.prompt.txt`
- 关键约束：
  - 主标题：园区网络全流量采集与分析系统
  - 一级菜单：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置
  - 二级菜单：仪表盘、态势大屏、专题面板
  - 当前页：综合态势 / 态势大屏
  - 面板标题：中文-only；统一标题字号、字重和基线，保留 Kafka、Flink、ClickHouse、PCAP 等技术名词
  - 视觉基线：深色园区安全运营台，密集但可读，形成“采集-分析-研判-取证-响应-反馈-验收”的业务闭环

# 统一任务中心UI候选图

生成日期：2026-08-16

状态：`VISUAL_CANDIDATE / DEFERRED / NOT_IMPLEMENTED / NOT_ACCEPTANCE_EVIDENCE`

导航说明：本目录是上一轮三种页面骨架的设计沿革，已由`../../pages/analysis-scheduling/`中的八张核心页面视觉候选替代。这里的旧菜单、常驻详情栏和“低风险百分比”等表达不得作为实现合同。

三张图均以现有`foundation-visual-reference.png`、`dashboard.png`和`mlops.png`为视觉参考，保持深色Shell、左侧导航、顶部状态条、青色主强调与Ant Design式组件。

| 文件 | 生成提示摘要 |
|---|---|
| `task-center-run-queue-priority-20260816.png` | 以任务运行队列表格为主区，三项轻摘要、四个筛选、五段进度和选中任务右侧上下文；人读报告独立显示 |
| `task-center-stage-lane-priority-20260816.png` | 以固定五阶段业务泳道为主叙事，点击阶段过滤下方任务列表；禁止自由DAG和自动/人工双lane |
| `task-center-run-narrative-priority-20260816.png` | 左侧紧凑任务列表，右侧聚焦一个Run的范围、五阶段、机器结论、完整性、关键发现与独立人读报告状态 |

共同生成约束：按1920×1080产品基线构图；核心菜单为综合态势、任务中心、研判取证、数据与模型、策略与模板、综合报告、审计配置；页面只有一个主要按钮“新建分析任务”；默认/自定义方案与持续窗口/定时/事件/按需触发正交；主链止于机器摘要；人读报告不阻塞Run终态；避免KPI墙、卡片套卡片、技术ID堆积、装饰图表和自由DAG。

生成器实际输出均为1672×941的16:9预览。选定方向后的实现与验收必须回到真实1920×1080 CSS viewport，不得把预览像素尺寸当成验收尺寸。

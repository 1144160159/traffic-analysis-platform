# 统一分析任务调度中心八页视觉候选

更新时间：2026-08-16

状态：`UI_D1_VISUAL_CANDIDATE / NOT_IMPLEMENTED / NOT_BROWSER_VERIFIED / NOT_ACCEPTANCE_EVIDENCE`

本目录保存统一分析任务调度中心的八张核心页面视觉候选。它们用于冻结页面职责、信息层级和跨页业务语义，不证明 Web 页面、API、权限、响应式、键盘操作或真实业务闭环已经实现。

## 页面清单

| 页面 | 文件 | 核心业务问题 |
|---|---|---|
| 任务管理 | `analysis-task-management-20260816.png` | 管理哪些可复用任务定义，不直接启动 Run |
| 调度管理 | `analysis-schedule-management-20260816.png` | 哪个批准方案在何时触发；方案来源与触发方式正交 |
| 任务编排 | `analysis-orchestration-20260816.png` | 固定五段使用哪些 exact-set 组件版本 |
| 运行监控 | `analysis-run-monitor-20260816.png` | 一个 Run 到哪一步、机器结论和证据是否可信 |
| 即时分析向导 | `analysis-on-demand-wizard-20260816.png` | ON_DEMAND 下选择默认或自定义方案并完成预检提交 |
| 运行详情 | `analysis-run-detail-20260816.png` | 单个 Run 的范围、阶段、结论、证据和独立报告状态 |
| 调度资源 | `analysis-resource-management-20260816.png` | 队列、租约、执行器和配额在哪里阻塞 |
| 报告中心 | `analysis-report-center-20260816.png` | 区分冻结机器摘要与独立异步人读报告 |

## 统一语义

- `默认方案 / 自定义方案`只表示计划准备来源，不表示触发方式、队列、优先级或另一套执行链。
- `持续窗口 / 定时 / 事件 / 即时`单独表示 TriggerKind。
- 五个用户阶段固定为`数据采集 → 特征处理 → 加密特征识别 → 恶意流量检测 → 机器摘要`；PlanReady、Reconcile是技术闸门，人读报告不是运行阶段。
- `RunState / FindingConclusion / RiskSeverity / Completeness / IntegrityState / ReportState`必须分别展示；DetectorDisposition只进入检测明细。任何未知、空输入、未运行或不完整状态都不得用成功绿或“安全”代替。
- 图中若出现字段合并、文字渲染或间距偏差，以`doc/07_alignment/01_主链与调度设计/统一分析任务调度中心菜单与UI详细设计.md`的字段与状态合同为准。

## 视觉与验收边界

- 文件实际为 1672×941 的 16:9 视觉预览；实现基线仍是 1920×1080 CSS viewport，并需在 1440、1024、390 宽度复核。
- 继续复用现有 AppShell、深海军蓝 token、166px 左侧导航、6px 圆角和 Ant Design 组件语言；这些图片不是可直接切图实现的素材。
- 生成图不能证明真实 Shell 的 80px 顶栏、83px 底部状态栏、焦点顺序、对比度、Drawer 焦点返回或接口状态已正确。
- UI-D1 评审通过后仍需形成任务定义详情、调度创建四步工作区、异常状态板和响应式帧；这些不应被八张核心图替代。

生成模式与提示词合同见 `PROMPT_MANIFEST.md`。

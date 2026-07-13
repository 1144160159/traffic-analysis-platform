# Pages 拆解、实现、验证闭环

本流程从 `pages/login` 重新开始，然后按应用菜单顺序推进。目标 UI 图主要作为拆解、测量、OCR、overlay 和 diff 的证据输入，禁止放入 `web/ui/public` 或作为整页/业务页面 UI 资源加载。经显式放宽后，页面背景图可以作为受控背景资源复刻；业务页面 UI 图、整页截图、完整卡片/表单/业务面板仍不得作为页面资源。真实页面差异必须回到前端代码实现层修复；如果后端 API 不存在，只预留前端 API 契约和 mock fallback，不要求补后端。

## 执行顺序

执行队列由 `build_pages_menu_order_queue.mjs` 生成：

- 机器队列：`doc/04_assets/ui_suite_gpt_v1/specs/pages-menu-order-queue.json`
- 人工可读队列：`doc/04_assets/ui_suite_gpt_v1/specs/PAGES_MENU_ORDER_QUEUE.md`

顺序规则：

1. `login` 第一张处理。
2. 之后按 `routeManifest.tsx` 的菜单顺序处理：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置。
3. 非主菜单直达的详情页、tab 态、证据态和子状态页挂靠到对应主菜单页面之后处理。
4. `pixel-perfect-breakdown-index.json` 只作为 pages 全集来源，不作为执行顺序。

## 单图闭环

1. 锁定 `pages-menu-order-queue.json` 中当前 `pages` 图片，按队列顺序处理。
2. 视觉读取目标图，记录版式、文本、图标、表格、菜单、状态和交互语义。
3. 做坐标测量和 OCR 辅助校正，补齐 `.md`、`.json`、`.review.md`。
4. 抽取 design tokens：颜色、字号、间距、圆角、阴影、边框、状态色、图标规格。
5. 确认组件映射：优先复用现有 React/AntD/本地组件；缺组件时新增代码组件。
   - 先区分业务动态图示与独立图标：地图、拓扑、流向图、雷达、趋势、热力、表格、状态机、仪表盘等需要随 API 数据变化的图示，必须使用 React/CSS/SVG/canvas/ECharts 等组件代码实现，不能用截图或图片资源替代。
   - 硬规则：业务区域涉及图示时，必须是基于 API 数据或明确 typed API contract/mock fallback 的动态图示；地图、拓扑、密度图、流向图、雷达、趋势折线、热力、仪表、管道状态等不得写成不可数据驱动的静态装饰。
   - 动态图示验收必须记录数据来源：优先真实 API；后端缺口时记录 typed API contract、adapter 和 mock fallback 字段映射。生产路由截图之外，还应尽量补一张非 `__codex_ui_breakdown_production=1` 的 API-live 截图或 runtime 元数据，证明图示不是目标图静态回放。
   - 独立图标/徽标/小型符号才进入图标资源流程；如果现有图标库或代码样式已经与目标足够一致，不强制截图提取。
   - 如果差异明确属于独立图标、徽标或小型符号，图标来源放开：允许使用 GPT/ImageGen 生成，也允许从任意截图来源提取。
   - 如果现有组件库图标、CSS/SVG/Canvas 样式图标已经与目标图在截图裁剪和 diff 指标中一致或足够接近，不强制截图提取；只有图标本身成为明确 mismatch 热点时才回到拆解层选择截图提取、生成或改代码。
   - GPT/ImageGen 图标应落在 `web/ui/src/assets/generated-icons/`、本地图标组件或 SVG/Canvas 代码中，并在当前图的 `.json` / `.review.md` 记录 prompt、尺寸、颜色 token、使用位置和截图证据。
   - 截图提取图标应落在 `web/ui/src/assets/screenshot-icons/`，来源截图不限制渠道或画质；但输出必须是 SVG，或位图三档 `24px/@1x`、`48px/@2x`、`72px/@3x`。位图图标需用同名 `.source.json` 记录 `usage` 和 density variants，建议同时记录来源截图路径、截取 bbox 和生成时间。
   - 图标例外只适用于独立图标/徽标/小型符号，不得把整张 UI 图、完整卡片/表单/业务面板截图或 `target.png` 作为页面资源。
   - 底部/footer 面板可在当前页面经显式批准后截图为受控资源，建议落在 `web/ui/src/assets/screenshot-panels/` 并用同名 `.source.json` 记录 usage、scope、source image、bbox 和适用页面；该例外不扩展到完整表单、完整卡片、整页或业务页面 UI 图。
   - 页面背景图可在当前页面经显式批准后复刻为受控背景资源，建议落在 `web/ui/src/assets/screenshot-backgrounds/` 并用同名 `.source.json` 记录 usage、scope、source image、处理方式，以及 `contains_business_page_ui=false`；业务页面 UI 图仍禁止作为资源。
   - 若图标仍与目标有差异，按图标 bbox 回到拆解层补齐坐标、线宽、填充、阴影、透明度和状态映射后重新生成、重新截图提取或改代码。
6. 确认交互状态：tab、筛选、菜单、modal、drawer、empty、loading、error、hover/active/disabled。
7. 生成或更新 `target.png`、`regions-overlay.png`、`implementation.png`、`diff.png`、`metrics.json`、`capture-meta.json`。
8. 实现阶段只修改前端代码、样式、mock 数据、API adapter/类型契约；不得引用目标 UI 图。
9. 使用 Windows Chrome CDP `http://127.0.0.1:9224` 打开真实路由截图，不能改用 Linux Chrome。
10. 根据截图、diff、runtime 错误、布局重叠、文本错漏、响应式问题进行主线程判定。
11. 未过则回到拆解层，补齐坐标、文本、组件、图标、token、交互、实现映射或差异说明，再改代码、重截、重算 diff。

## 证据规则

- `implementation.png` 必须来自真实 APISIX/Web UI 路由或明确标记的本地前端预览路由，且通过 Windows Chrome CDP 截图。
- `reference-raster`、`implementation.html`、`target.png` 页面回放只能作为历史校准，不作为验收。
- 页面运行时如果请求 `/ui-assets/canonical/`、`/screens/pages/`、`/evidence/ui-image-breakdowns/`、`target.png`、`regions-overlay.png` 或 `implementation.html`，该图直接失败。
- `web/ui/public` 中若出现用于页面复刻的 canonical/screens/target/overlay/implementation 资源，`pages` 队列直接失败。
- 独立图标可以作为实现资产，来源可为 GPT/ImageGen 或任意截图提取；截图图标不要求来源截图为 4K，但必须输出为 SVG，或位图三档 `24px/@1x`、`48px/@2x`、`72px/@3x`，并用同名 `.source.json` 记录用途和输出规格。
- 经显式批准的底部/footer 面板截图可以作为实现资产，但必须落在受控目录、带 source manifest，并限定在当前页面底部/footer 范围。
- 经显式批准的页面背景图可以作为实现资产，但必须落在受控目录、带 source manifest，并明确不包含业务页面 UI；业务页面 UI 图、整页截图、完整卡片/表单/业务面板、canonical/screen/target/overlay/implementation 资源仍禁用。
- 任何截图背景或底部面板 manifest 必须显式声明 `contains_business_dynamic_diagram=false`；业务地图、拓扑、密度图、流向图、雷达、趋势折线、热力、仪表、管道状态等动态图示只能由 API/typed fallback 数据驱动的组件生成。
- 验收仍以 Windows Chrome 路由截图和 diff 为准，不能用资源存在本身替代真实页面截图。
- 每个结论必须能追溯到 URL、视口、页面状态和 evidence 截图路径。
- 后续拆解 UI 图、人工审查和视觉 diff 判定只把业务页面是否通过作为门禁重点；业务内容区的亮度、透明度、背景噪声、背景线网和纯视觉纹理如果造成业务层次混乱、可读性下降、与 UI 图关键结构不一致或影响主线程判断，可以作为阻断项。AppShell、登录/404 等非业务区的氛围差异只记录为备注，除非影响业务页面验收。

## 后端缺口规则

- 页面需要数据但后端没有对应 API 时，在前端新增 typed API contract、client 方法和 mock fallback。
- mock 数据必须服务于当前 UI 状态复刻，字段名保持可迁移到真实 API。
- 不为 UI 复刻临时修改 Go 后端或 Kubernetes 服务，除非后续验收明确要求真实接口。

## Windows Chrome 前置检查

每轮截图前先检查：

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
```

如果 `9224` 不可用，先恢复 Windows Chrome CDP 和 SSH 隧道；禁止改用 Linux Chrome。

## 主线程验收口径

- runtime：无 4xx/5xx、requestfailed、console/pageerror、水平溢出、明显几何异常。
- visual：`metrics.json` 通过设定阈值，并用 `implementation.png`、`diff.png`、`regions-overlay.png` 人工复核。
- business ROI：页面配置业务评分区域（例如 `content-root`）时，ROI 必须严格 `<0.125`；公共 AppShell 只保留为整图诊断，不得以整图结果替代业务门禁。历史 `0.13` 结论必须按该门槛重新判定。
- semantic：文本、菜单、图标、组件层级、交互状态与目标图一致或有明确差异说明。
- review：智能体辅助审查完成后，主线程才能把状态改为 `pixel-accepted`。
- business-focus：拆解 UI 图和视觉 diff 的通过/失败结论只围绕业务页面、业务内容区、业务动态图示、业务文案、业务操作入口和业务状态是否达标；业务内容区的亮度、透明度、背景线网、面板透底和纹理噪声可作为阻断项。AppShell、登录/404、非业务氛围差异只记录为备注，除非影响业务页面验收。

## 严格铁律闭环（可执行清单）

### 0) 人员动作铁律

1. 只按 `pages-menu-order-queue.json` 队列顺序执行，不跳页。
2. 每张页都按固定节奏执行：锁图 -> 拆解 -> 实现 -> 生产截图 -> 复检 -> 主线程判断 -> 回修 -> 重拍 -> 复核。
3. 未通过主线程判定不得进入下一页。
4. 每条结论必须挂在 `URL + 视口 + 页面状态 + 证据路径 + diff/metrics` 上。

### 1) 业务动态图示铁律（绝对红线）

以下类型默认走动态组件化方案，不得静态截图替代：

- 地图类：世界地图、园区地图、外联流向图、探针覆盖图。
- 拓扑类：2D/3D 园区数字孪生、资产链路关系、攻击链、通信路径。
- 密度类：战役簇、威胁密度、风险热度、热点分布。
- 趋势类：采集与流处理折线、攻击阶段变化、质量趋势、告警变化。
- 仪表/闭环类：证据完整度、反馈质量、SLA、状态环。
- 状态与流程类：告警闭环、状态流转、处理动作链路。

对上述类型必须满足：

- 优先真实 API；无 API 时写明 typed API contract 与 fallback 映射。
- API/typed 数据必须通过 `services/api.ts` 进入组件。
- 除分页表格外，页面中的相关业务数据与图表应支持定期刷新（按页面语义设置重刷节奏）。
- 使用 ECharts/SVG/canvas/React 组件实现，不允许贴图/背景替代表达业务含义。
- 任何“像素误差”不能成为图表不动态化的理由；先追踪数据来源再修复实现。

### 2) 图标与资源边界铁律

1. 独立图标可截图提取、GPT/ImageGen 生成或复用组件库图标；前提是独立、可复用、非业务大块。
2. 禁止将以下作为截图资源：业务地图、拓扑、趋势、密度、流程、状态流转、告警闭环图、主业务面板、整卡片、整表单、整页。
3. 截图图标若存在：输出 SVG 或 `24/48/72` 三档位图；落在 `web/ui/src/assets/screenshot-icons/`，并带同名 `.source.json`（usage / bbox / 版本 / 输出规格）。
4. 页面背景、底部 panel 允许仅在明确审批后使用，必须 manifest 标注不可包含业务核心模块且 `contains_business_dynamic_diagram=false`。

### 3) 截图与证据铁律

1. 每轮截图前执行：

```bash
curl -i --max-time 5 http://127.0.0.1:9224/json/version
curl -i --max-time 5 http://127.0.0.1:9224/json/list
```

2. 9224 不可用时，先恢复 Windows Chrome CDP / SSH 隧道，不得改用 Linux Chrome。
3. 生产实现必须使用 `http://10.0.5.8:30180` 路由截图。
4. 每页必须更新 `implementation.png`、`diff.png`、`metrics.json`、`measurement.json`、`capture-meta.json`、`verification.json`、`target.png`、`regions-overlay.png`。
5. 实测复核点：动态图表是否为真实数据驱动、文本/菜单/按钮是否齐全、模块是否重叠，字体是否可见完整、关键指标是否随窗口重排时自适应。

### 4) 回修触发铁律

满足以下任一条件直接回到拆解层补齐：

- 业务动态图示仍未实现为 API 或 typed 动态图。
- 模块重叠、裁切、溢出或关键状态布局漂移。
- 文字被截断且悬停看不到完整内容。
- 业务内容区亮度、透明度、背景线网或纹理噪声导致页面难读、层次混乱或与目标 UI 关键结构明显不一致。
- 生产证据未覆盖 URL 与 viewport 状态映射。
- 结论无法绑定 `metrics` 与 `diff` 的可复核证据。

# foundation-responsive.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 图片 ID：foundation-responsive
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-responsive.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-responsive.prompt.txt`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-responsive.json`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应路由：无
- 宿主路由：无
- 页面性质：设计系统 foundation 规范板
- 当前阶段：`pixel-accepted`
- 拆解目的：锁定桌面、1440、平板、移动和 4K 的响应式策略
- 重要边界：交付图仍为 1920 x 1080，它只表达不同断点下的重排策略
- 坐标系统：所有 bbox 均基于目标 PNG 全图坐标
- OCR 方式：人工视觉读取并用局部裁剪校正
- 辅助审查：Carver 已完成只读视觉拆解
- 证据目录：`evidence/ui-image-breakdowns/foundations/foundation-responsive/`
- 目标图证据：`evidence/ui-image-breakdowns/foundations/foundation-responsive/target.png`
- 实现截图证据：`evidence/ui-image-breakdowns/foundations/foundation-responsive/implementation.png`
- 视觉差异证据：`evidence/ui-image-breakdowns/foundations/foundation-responsive/diff.png`
- 区域覆盖证据：`evidence/ui-image-breakdowns/foundations/foundation-responsive/regions-overlay.png`
- 验收证据：`evidence/ui-image-breakdowns/foundations/foundation-responsive/verification.json`
- 生产边界：本图是 responsive token 和断点策略来源，不是独立业务页面

## 目标图观察

- 整体是深色安全运营台风格的响应式规范板。
- 顶部标题区高度约 73px。
- 主标题为 `Foundation Responsive Strategy`。
- 副标题说明交付图仍是 1920x1080。
- 副标题强调图内表达不同断点下的 AppShell 与内容重排策略。
- 右上角固定显示 `第一基准：screen.png`。
- 主体上半部分分为三块断点策略面板。
- 左侧面板是 `01 桌面基准 1920`。
- 中间面板是 `02 1440 桌面`。
- 右侧面板是 `03 平板 / 移动`。
- 底部整宽面板是 `04 响应式门禁`。
- 所有面板使用青蓝描边、6px 圆角和 43px 标题栏。
- 左侧 1920 面板最大，展示完整桌面 AppShell。
- 1920 预览保留完整顶部栏、左侧栏、内容区和底部状态栏。
- 1920 预览下方列出桌面固定尺寸。
- 1920 的顶部栏为 `80px fixed`。
- 1920 的左侧栏为 `166px single column`。
- 1920 的底部栏为 `83px fixed status`。
- 1920 的内容区为 `12-column dense panels`。
- 1440 面板上方展示缩小后的桌面预览。
- 1440 面板下方用四个绿色 bullet 写出压缩策略。
- 1440 首要规则是保持 AppShell 顺序。
- 1440 第二规则是优先压缩右侧栏密度。
- 1440 第三规则是图表减少标签。
- 1440 第四规则是表格保持行高。
- 平板/移动面板上方展示压缩后的平板横向内容预览。
- 平板/移动面板下方展示移动端导航抽屉层。
- 移动抽屉不是手机外壳，而是窄面板样例。
- 移动抽屉菜单词汇保持六个一级菜单。
- 抽屉菜单包括综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置。
- 底部门禁面板用四条规则约束后续实现。
- 门禁第 1 条是 1440 保持顶部/左侧/底部层级。
- 门禁第 1 条还强调先压缩右侧栏再调整菜单。
- 门禁第 2 条允许平板左侧菜单变抽屉。
- 门禁第 2 条要求词汇和图标固定。
- 门禁第 3 条要求移动端使用 `drawer-mobile-navigation` 资产。
- 门禁第 3 条禁止强塞完整桌面壳。
- 门禁第 4 条要求大屏/4K 放大内容节奏。
- 门禁第 4 条禁止新增第二底栏或新配色。
- 本图的重点是断点策略，不是业务指标本身。
- 缩略 dashboard 内部仍展示 screen.png 公共结构。
- 后续实现遇到窄屏时，应先按本图策略处理结构层级。
- 不允许因为响应式适配改变菜单词汇。
- 不允许因为响应式适配更换图标家族。
- 不允许因为 4K 放大引入新的颜色主题。

## 区域与坐标

坐标为基于目标 PNG 直接视觉读取和局部裁剪校正后的人工测量，格式为 `x,y,w,h`。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 16:9 foundation 规范板 | 保持 1920x1080 |
| 顶部标题区 | `0,0,1920,73` | 1 | 标题、副标题、基准说明 | y=72 分割线 |
| 1920 面板 | `24,92,757,721` | 1 | `01 桌面基准 1920` | 左侧最大面板 |
| 1920 标题栏 | `25,93,755,43` | 2 | section title | 编号和标题 |
| 1920 桌面预览 | `54,149,696,392` | 2 | 完整桌面 AppShell | 顶部/左侧/底部都可见 |
| 1920 固定尺寸列表 | `69,594,420,122` | 2 | 四行桌面 token | 标签左、数值右 |
| 1440 面板 | `812,92,548,721` | 1 | `02 1440 桌面` | 中间面板 |
| 1440 标题栏 | `813,93,546,43` | 2 | section title | 标题对齐 |
| 1440 桌面预览 | `842,151,489,276` | 2 | 压缩桌面预览 | 保持 AppShell 顺序 |
| 1440 规则列表 | `849,496,270,158` | 2 | 四条 bullet | 绿色圆点，48px 行距 |
| 平板/移动面板 | `1392,92,505,721` | 1 | `03 平板 / 移动` | 右侧面板 |
| 平板/移动标题栏 | `1393,93,503,43` | 2 | section title | 标题对齐 |
| 平板内容预览 | `1443,149,401,273` | 2 | 横向压缩预览 | 保留主内容与右栏 |
| 移动抽屉外框 | `1564,470,161,302` | 2 | drawer sample | 不是手机外壳 |
| 移动抽屉内容 | `1573,477,143,286` | 3 | 菜单词汇 | 六个固定菜单 |
| 响应式门禁面板 | `24,842,1873,197` | 1 | `04 响应式门禁` | 底部整宽面板 |
| 响应式门禁标题栏 | `25,843,1871,43` | 2 | section title | 与上方面板一致 |
| 响应式门禁列表 | `58,904,870,112` | 2 | 四条规则 | 绿色圆点 |

## 文本清单

| 序号 | 文本 | 位置 | 类型 | 复刻要求 |
|---:|---|---|---|---|
| 1 | Foundation Responsive Strategy | 顶部标题区 | 主标题 | 必须完全一致 |
| 2 | 交付图仍为 1920x1080；画面表达不同断点下的 AppShell 与内容重排策略 | 顶部标题区 | 副标题 | 必须完全一致 |
| 3 | 第一基准：screen.png | 顶部右侧 | 基准说明 | 必须完全一致 |
| 4 | 01 桌面基准 1920 | 左侧面板标题 | 区块标题 | 必须完全一致 |
| 5 | 顶部栏 | 1920 尺寸列表 | 标签 | 必须完全一致 |
| 6 | 80px fixed | 1920 尺寸列表 | 值 | 必须完全一致 |
| 7 | 左侧栏 | 1920 尺寸列表 | 标签 | 必须完全一致 |
| 8 | 166px single column | 1920 尺寸列表 | 值 | 必须完全一致 |
| 9 | 底部栏 | 1920 尺寸列表 | 标签 | 必须完全一致 |
| 10 | 83px fixed status | 1920 尺寸列表 | 值 | 必须完全一致 |
| 11 | 内容区 | 1920 尺寸列表 | 标签 | 必须完全一致 |
| 12 | 12-column dense panels | 1920 尺寸列表 | 值 | 必须完全一致 |
| 13 | 02 1440 桌面 | 中间面板标题 | 区块标题 | 必须完全一致 |
| 14 | 保持 AppShell 顺序 | 1440 规则 | bullet | 必须完全一致 |
| 15 | 优先压缩右侧栏密度 | 1440 规则 | bullet | 必须完全一致 |
| 16 | 图表减少标签 | 1440 规则 | bullet | 必须完全一致 |
| 17 | 表格保持行高 | 1440 规则 | bullet | 必须完全一致 |
| 18 | 03 平板 / 移动 | 右侧面板标题 | 区块标题 | 必须完全一致 |
| 19 | 移动端导航抽屉层 | 移动抽屉 | 抽屉标题 | 必须完全一致 |
| 20 | 综合态势 | 移动抽屉 | 菜单 | 必须完全一致 |
| 21 | 采集监测 | 移动抽屉 | 菜单 | 必须完全一致 |
| 22 | 威胁分析 | 移动抽屉 | 菜单 | 必须完全一致 |
| 23 | 资产图谱 | 移动抽屉 | 菜单 | 必须完全一致 |
| 24 | 检测运营 | 移动抽屉 | 菜单 | 必须完全一致 |
| 25 | 审计配置 | 移动抽屉 | 菜单 | 必须完全一致 |
| 26 | 04 响应式门禁 | 底部面板标题 | 区块标题 | 必须完全一致 |
| 27 | 1440：保持顶部/左侧/底部层级，先压缩右侧栏再调整菜单 | 底部门禁 | gate rule | 必须完全一致 |
| 28 | 平板：左侧菜单可变为抽屉，但词汇和图标固定 | 底部门禁 | gate rule | 必须完全一致 |
| 29 | 移动端：使用 drawer-mobile-navigation 资产，不强塞完整桌面壳 | 底部门禁 | gate rule | 必须完全一致 |
| 30 | 大屏 / 4K：放大内容节奏，不新增第二底栏或新配色 | 底部门禁 | gate rule | 必须完全一致 |

## 组件清单

| 组件 | bbox | 类型 | 说明 | 前端映射 |
|---|---:|---|---|---|
| FoundationBoardHeader | `0,0,1920,73` | layout header | 响应式规范板标题 | Static header |
| Desktop1920Panel | `24,92,757,721` | foundation panel | 1920 桌面基准 | FoundationPanel |
| Desktop1920Preview | `54,149,696,392` | reference screenshot | 完整桌面 AppShell | Desktop preview |
| Desktop1920SpecList | `69,594,420,122` | spec list | 四个桌面 token | SpecList |
| Desktop1440Panel | `812,92,548,721` | foundation panel | 1440 策略 | FoundationPanel |
| Desktop1440Preview | `842,151,489,276` | reference screenshot | 压缩桌面 | Desktop preview |
| Desktop1440RuleList | `849,496,270,158` | bullet list | 四条 1440 规则 | RuleList |
| TabletMobilePanel | `1392,92,505,721` | foundation panel | 平板/移动策略 | FoundationPanel |
| TabletPreview | `1443,149,401,273` | reference screenshot | 平板横向预览 | Tablet preview |
| MobileNavigationDrawer | `1564,470,161,302` | drawer sample | 移动端抽屉 | DrawerMobileNavigation |
| MobileDrawerMenuList | `1573,477,143,286` | drawer menu | 六个菜单词汇 | DrawerMenuList |
| ResponsiveGatePanel | `24,842,1873,197` | foundation panel | 响应式门禁 | FoundationPanel |
| ResponsiveGateRuleList | `58,904,870,112` | bullet list | 四条门禁规则 | RuleList |

## 图标清单

| 图标 | bbox | 状态 | 语义 | 复刻要点 |
|---|---:|---|---|---|
| 1920 顶部产品图标 | `61,157,11,14` | brand | product-identity | 缩略图内仍可见 |
| 1920 侧栏当前项图标 | `69,222,9,9` | active | situation-screen | 保持激活蓝语义 |
| 1920 底部通知角标 | `674,427,13,12` | alert-badge | notification | 底部栏右侧 |
| 1440 规则绿点 1 | `849,498,12,12` | success | rule-bullet | bullet 规则 |
| 1440 规则绿点 2 | `849,546,12,12` | success | rule-bullet | bullet 规则 |
| 1440 规则绿点 3 | `849,594,12,12` | success | rule-bullet | bullet 规则 |
| 1440 规则绿点 4 | `849,642,12,12` | success | rule-bullet | bullet 规则 |
| 门禁绿点 1 | `57,906,12,12` | success | gate-bullet | 底部门禁 |
| 门禁绿点 2 | `57,938,12,12` | success | gate-bullet | 底部门禁 |
| 门禁绿点 3 | `57,970,12,12` | success | gate-bullet | 底部门禁 |
| 门禁绿点 4 | `57,1002,12,12` | success | gate-bullet | 底部门禁 |
| 移动抽屉外框 | `1564,470,161,302` | sample | mobile-navigation-drawer | 青蓝描边 |

## Token 与样式

| Token | 值 | 用途 | 约束 |
|---|---|---|---|
| desktop-base-width | `1920px` | 桌面基准断点 | 完整 AppShell |
| desktop-medium-width | `1440px` | 中等桌面断点 | 保持 AppShell 顺序 |
| deliverable-size | `1920x1080` | foundation 交付图 | 不拆成多张真实分辨率 |
| topbar-height | `80px fixed` | 1920 桌面顶部栏 | 不随页面变化 |
| sidebar-width | `166px single column` | 1920 桌面侧栏 | 单栏展开式 |
| statusbar-height | `83px fixed status` | 1920 桌面底栏 | 单层状态栏 |
| desktop-content-grid | `12-column dense panels` | 1920 内容区 | 高密度 12 栅格 |
| panel-radius | `6px` | 所有主面板 | 保持 foundation 风格 |
| button-radius | `4px` | 按钮和抽屉内项 | 紧凑 |
| mobile-drawer-asset | `drawer-mobile-navigation` | 移动端导航 | 禁止强塞完整桌面壳 |
| success-green | `#36d66b` | bullet 和通过状态 | 不替代其他状态语义 |
| active-blue | `#1e9cff` | 当前项/强调 | 与 screen.png 一致 |
| panel-bg | `#071f32` | 面板底色 | 低饱和深蓝 |
| border-weak | `rgba(56,151,201,.22)` | 面板和抽屉边框 | 细线 |

## 状态与交互

- 1920 是桌面完整基准。
- 1920 顶部栏保持 80px fixed。
- 1920 左侧栏保持 166px single column。
- 1920 底部栏保持 83px fixed status。
- 1920 内容区保持 12-column dense panels。
- 1440 不重做 AppShell。
- 1440 先保持顶部、左侧、底部层级。
- 1440 优先压缩右侧栏密度。
- 1440 后续才考虑菜单调整。
- 1440 图表应减少标签，而不是更换图表风格。
- 1440 表格保持行高，不为了塞内容随意压扁。
- 平板可把左侧菜单变为抽屉。
- 平板抽屉词汇和图标固定。
- 移动端使用 `drawer-mobile-navigation` 资产。
- 移动端不强塞完整桌面 AppShell。
- 大屏/4K 放大内容节奏。
- 大屏/4K 不新增第二底栏。
- 大屏/4K 不新增新配色。
- 响应式变化不能改变状态色语义。
- 响应式变化不能改变菜单词汇。
- 响应式变化不能改变图标家族。
- 缩略 preview 仅是规范展示，不代表真实 iframe 缩放实现。

## 实现映射

- `ResponsiveAppShell` 根据断点选择桌面、压缩桌面、平板、移动策略。
- `Desktop1920Preview` 对应完整桌面基准。
- `Desktop1440Preview` 对应中等桌面压缩策略。
- `TabletPreview` 对应平板横向内容保留策略。
- `MobileNavigationDrawer` 对应移动端抽屉导航资产。
- `ResponsiveGateList` 对应验收门禁文档。
- 1920 桌面直接继承 layout-grid foundation 的尺寸。
- 1440 桌面保留 AppShell 顺序，压缩右侧栏。
- 平板端侧栏可抽屉化，但菜单词汇和图标必须固定。
- 移动端优先展示 drawer navigation 和主内容，不把桌面壳等比缩小。
- 4K 只放大内容节奏，不增加第二套公共区。
- CSS breakpoints 应按语义命名，避免硬编码无说明的 magic number。
- 图表组件应提供 label-density 策略。
- 表格组件应提供 compact row 策略。
- Drawer 组件应使用相同菜单数据源。
- 顶部/底部公共区状态语义由 screen.png 固定，不由断点重新发明。

## 验收证据

- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-responsive.png`
- target：`evidence/ui-image-breakdowns/foundations/foundation-responsive/target.png`
- implementation：`evidence/ui-image-breakdowns/foundations/foundation-responsive/implementation.png`
- diff：`evidence/ui-image-breakdowns/foundations/foundation-responsive/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/foundations/foundation-responsive/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-responsive/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-responsive/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/foundations/foundation-responsive/metrics.json`
- capture metadata：`evidence/ui-image-breakdowns/foundations/foundation-responsive/capture-meta.json`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-responsive/verification.json`
- 浏览器要求：Windows Chrome CDP `http://127.0.0.1:9224`
- 截图要求：1920 x 1080，DPR 1
- diff 要求：对 target 与 implementation 生成 `diff.png`
- 审查要求：辅助智能体检查证据，主线程最终判断
- 证据边界：reference-raster 证明目标 PNG 复刻，生产响应式逻辑仍需按本记录落地

## 差异清单

| 类型 | 位置 | 当前记录 | 验收要求 | 状态 |
|---|---|---|---|---|
| 视觉截图 | 全图 | Windows Chrome 截图由像素门禁生成 | 必须有 implementation.png | closed |
| 视觉差异 | 全图 | diff 由像素门禁生成 | mismatch ratio 达到门禁 | closed |
| 区域覆盖 | 全图 | JSON 已记录 18 个区域 | overlay 覆盖三断点和门禁面板 | closed |
| 文本校正 | 全图 | 已人工校正 30 条文本 | text ledger 同步记录 | closed |
| 响应式语义 | 上三面板和底部门禁 | 1920/1440/平板/移动/4K 规则已拆分 | 不混淆断点策略 | closed |
| 生产边界 | React/CSS 实现 | reference-raster 只证明像素复刻 | 响应式实现按本记录落地 | documented |

## 结论

- `foundation-responsive.png` 是响应式适配原则规范板。
- 它明确最终交付图仍是 1920x1080。
- 它用三块上方面板表达 1920、1440、平板/移动策略。
- 它用底部整宽面板表达响应式门禁。
- 1920 桌面保持完整 AppShell 和 12 栅格。
- 1440 桌面优先压缩右侧栏密度，并保持 AppShell 顺序。
- 平板允许左侧菜单抽屉化，但词汇和图标固定。
- 移动端使用 `drawer-mobile-navigation` 资产，不强塞完整桌面壳。
- 大屏/4K 只放大内容节奏，不新增第二底栏或新配色。
- 本记录已完成区域、文本、组件、图标、token、交互和实现映射拆解。
- Windows Chrome 截图、视觉 diff、辅助审查和主线程判定已经完成。

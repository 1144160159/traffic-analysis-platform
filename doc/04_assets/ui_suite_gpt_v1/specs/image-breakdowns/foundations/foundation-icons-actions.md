# foundation-icons-actions.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 图片 ID：foundation-icons-actions
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-icons-actions.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-icons-actions.prompt.txt`
- 对应 layer：`doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-icons-actions.json`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 对应路由：无
- 宿主路由：无
- 页面性质：设计系统 foundation 规范板
- 当前阶段：`pixel-accepted`
- 复刻目标：已记录完整视觉结构，并完成 Windows Chrome reference-raster 截图与像素门禁
- 主线程判断口径：截图、diff、辅助审查和主线程判定齐全，本图已标为 `pixel-accepted`
- 语义边界：本图不是生产业务页面，不直接映射单一路由
- 生产落地方向：AppShell 左侧导航、顶部快捷入口、底部状态栏、全局动作、菜单词汇、危险动作规则
- 坐标系统：所有 bbox 均以 1920 x 1080 原图左上角为原点
- OCR 方式：人工视觉读取并校正，Tesseract 在当前 Linux 环境不可用
- 辅助审查：Maxwell 只读视觉拆解已提示父子高亮、快捷入口双规格和底部状态条小字风险
- 证据目录：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/`
- 目标图证据：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/target.png`
- 实现截图证据：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/implementation.png`
- 视觉差异证据：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/diff.png`
- 区域覆盖证据：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/regions-overlay.png`
- 验收证据：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/verification.json`

## 目标图观察

- 整体画布为深色安全运营台规范板，背景接近 `#03111c`。
- 顶部 73px 是标题栏，左侧英文主标题，右侧是基准说明。
- 主标题为 `Foundation Icons And Actions`，表达图标和动作语义。
- 副标题明确图标体系和顺序以 `screen.png` 为准。
- 右上角 `第一基准：screen.png` 强调该图继承态势大屏公共区域。
- 页面主体从 x=24 开始到 x=1897，纵向从 y=92 到 y=1039。
- 主体是三列布局：左列、中列、右列。
- 左列为 `01 左侧导航图标`。
- 中列上方为 `02 顶部快捷入口`。
- 中列中部为 `03 底部状态与全局动作`。
- 中列下方为 `04 动作语义`。
- 右列为 `05 固定菜单词汇`。
- 五个面板共享薄青蓝边框、6px 圆角、43px 标题栏。
- 左侧面板内部放置一个缩放版单栏导航。
- 该单栏导航不是生产 166px 宽度原尺寸，而是规范板中的缩小演示。
- 左侧菜单重点不是页面业务内容，而是图标家族、顺序、激活语义。
- `综合态势` 表示父级域处于展开/激活语义。
- `态势大屏` 表示当前子路由高亮。
- 两种高亮同时出现，不能误判为重复激活。
- 底部用户区属于左侧导航内部，包含头像、用户名、角色、在线状态和本地动作。
- 顶部快捷入口展示了两种规格。
- 上方有小图标+小标签的紧凑图标按钮组。
- 下方有大字号文字词汇行，用来锁定标签顺序。
- 这两行不是两个不同菜单，而是同一组快捷入口的两种展示参考。
- 底部状态条样例是一个缩略版 AppShell bottom bar。
- 底部状态条包含健康指标和右侧全局动作。
- 通知、设置、全局配置、电源动作出现在底部右侧，不应搬到顶部栏。
- 动作语义区是实现规则，不是装饰性说明。
- 危险动作必须包含确认、影响范围、权限、审计留痕。
- 图标按钮必须有 tooltip 和 aria-label。
- 禁用、加载、错误态不能替换图标家族。
- 组件板展示图标时优先复刻 `screen.png` 线性风格。
- 右侧固定菜单词汇列出六个一级菜单。
- 右侧六项是词汇白名单，不是另一个运行时侧栏。
- 所有按钮右侧均为 `>` chevron，表示进入或展开。
- 视觉基调保持克制：无大面积渐变、无营销图、无浮动装饰。
- 信息密度高但排版稳定，所有文字均在面板内对齐。
- 图标以线性描边为主，激活和强调才使用高亮蓝。
- 状态健康颜色以绿色为主，通知角标使用红色。
- 技术落地需映射到 React + Ant Design + lucide/自定义 icon 体系。

## 区域与坐标

坐标为基于目标 PNG 直接视觉读取和局部裁剪校正后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 16:9 foundation 规范板 | 保持原始 PNG 尺寸，不出现浏览器外框 |
| 顶部标题区 | `0,0,1920,73` | 1 | 主标题、副标题、基准说明 | y=72 有 1px 青蓝分割线 |
| 英文主标题 | `28,14,503,34` | 2 | `Foundation Icons And Actions` | 约 30px 粗体浅色文字 |
| 中文副标题 | `29,53,360,14` | 2 | 图标体系说明 | 小号次级文字，单行显示 |
| 右上基准说明 | `1586,32,181,18` | 2 | `第一基准：screen.png` | 中文前缀青蓝，文件名更亮 |
| 左侧大面板 | `24,92,477,947` | 1 | `01 左侧导航图标` | 外框圆角 6px，边框青蓝 |
| 左侧标题栏 | `25,93,475,43` | 2 | section header | 编号加粗，底部分割线 |
| 单栏导航模拟器 | `174,149,155,842` | 2 | 侧栏缩放样例 | 单栏展开式，不拆成双栏 |
| 父级综合态势 | `184,150,141,48` | 3 | 父级域激活/展开 | 左侧亮青竖条和蓝色图标 |
| 仪表盘菜单项 | `188,205,134,35` | 3 | 默认子菜单 | 灰白图标和文字 |
| 态势大屏当前项 | `188,245,135,38` | 3 | 当前子路由 | 蓝色描边、蓝色文字、蓝色图标 |
| 专题面板菜单项 | `189,290,133,34` | 3 | 默认子菜单 | 清单图标，弱文字 |
| 一级菜单余项 | `188,350,135,268` | 3 | 采集到审计五个域 | 纵向间距约 58px |
| 用户信息区 | `184,896,142,75` | 3 | 头像、用户名、角色、在线、本地动作 | 归属左侧底部，不放顶部 |
| 中上面板 | `528,92,843,319` | 1 | `02 顶部快捷入口` | 宽面板，内容居中 |
| 中上标题栏 | `529,93,841,43` | 2 | section header | 与其他标题栏同高 |
| 快捷图标组容器 | `758,159,382,82` | 2 | 紧凑图标按钮样例 | 6 个图标等距排列 |
| 快捷大文字行 | `575,296,672,18` | 2 | 六个入口标签 | 大字号中文标签，横向均分 |
| 中部面板 | `528,438,843,305` | 1 | `03 底部状态与全局动作` | 面板高度 305px |
| 中部标题栏 | `529,439,841,43` | 2 | section header | 标题下分割线 |
| 底部状态条样例 | `559,518,780,36` | 2 | 单层状态栏缩略图 | 指标和全局动作同一行 |
| 固定顺序说明 | `560,624,676,20` | 2 | slash 分隔顺序 | 次级文字，不能换顺序 |
| 中下面板 | `528,771,843,268` | 1 | `04 动作语义` | 与中上面板左对齐 |
| 动作语义标题栏 | `529,772,841,43` | 2 | section header | 编号 04 |
| 动作语义列表 | `559,826,686,156` | 2 | 四条绿色圆点规则 | 圆点 x 约 565，行距约 48px |
| 右侧大面板 | `1398,92,499,947` | 1 | `05 固定菜单词汇` | 右列高度与左列一致 |
| 右侧标题栏 | `1399,93,497,43` | 2 | section header | 编号 05 |
| 固定菜单按钮列表 | `1429,161,438,428` | 2 | 六个按钮 | 每项高约 47px，间距约 28px |
| 综合态势按钮 | `1430,162,437,47` | 3 | 第一项 | 左文字、右 chevron |
| 采集监测按钮 | `1430,237,437,47` | 3 | 第二项 | 与第一项同宽同高 |
| 威胁分析按钮 | `1430,313,437,47` | 3 | 第三项 | 文字对齐 x=1471 |
| 资产图谱按钮 | `1430,389,437,47` | 3 | 第四项 | 右侧 chevron 对齐 |
| 检测运营按钮 | `1430,465,437,47` | 3 | 第五项 | 同一视觉状态 |
| 审计配置按钮 | `1430,542,437,47` | 3 | 第六项 | 列表结束项 |

## 文本清单

| 序号 | 文本 | 位置 | 类型 | 复刻要求 |
|---:|---|---|---|---|
| 1 | Foundation Icons And Actions | 顶部标题区 | 主标题 | 必须完全一致 |
| 2 | 导航、快捷入口、底栏动作以 screen.png 图标体系和顺序为准 | 顶部标题区 | 副标题 | 必须完全一致 |
| 3 | 第一基准：screen.png | 顶部右侧 | 基准说明 | 必须完全一致 |
| 4 | 01 左侧导航图标 | 左侧标题栏 | 区块标题 | 必须完全一致 |
| 5 | 综合态势 | 左侧导航顶部 | 父级菜单 | 必须完全一致 |
| 6 | 仪表盘 | 左侧导航 | 子菜单 | 必须完全一致 |
| 7 | 态势大屏 | 左侧导航 | 当前子菜单 | 必须完全一致 |
| 8 | 专题面板 | 左侧导航 | 子菜单 | 必须完全一致 |
| 9 | 采集监测 | 左侧导航 | 一级菜单 | 必须完全一致 |
| 10 | 威胁分析 | 左侧导航 | 一级菜单 | 必须完全一致 |
| 11 | 资产图谱 | 左侧导航 | 一级菜单 | 必须完全一致 |
| 12 | 检测运营 | 左侧导航 | 一级菜单 | 必须完全一致 |
| 13 | 审计配置 | 左侧导航 | 一级菜单 | 必须完全一致 |
| 14 | sec_analyst | 用户区 | 用户名 | 必须完全一致 |
| 15 | 安全分析师 | 用户区 | 角色 | 必须完全一致 |
| 16 | 在线 | 用户区 | 在线状态 | 必须完全一致 |
| 17 | 02 顶部快捷入口 | 中上标题栏 | 区块标题 | 必须完全一致 |
| 18 | PCAP检索 | 快捷小图标组 | 快捷入口 | 必须完全一致 |
| 19 | 资产检索 | 快捷小图标组 | 快捷入口 | 必须完全一致 |
| 20 | 规则检索 | 快捷小图标组 | 快捷入口 | 必须完全一致 |
| 21 | 脚本中心 | 快捷小图标组 | 快捷入口 | 必须完全一致 |
| 22 | 帮助中心 | 快捷小图标组 | 快捷入口 | 必须完全一致 |
| 23 | 更多应用 | 快捷小图标组 | 快捷入口 | 必须完全一致 |
| 24 | PCAP检索 | 快捷大文字行 | 词汇锁定 | 必须完全一致 |
| 25 | 资产检索 | 快捷大文字行 | 词汇锁定 | 必须完全一致 |
| 26 | 规则检索 | 快捷大文字行 | 词汇锁定 | 必须完全一致 |
| 27 | 脚本中心 | 快捷大文字行 | 词汇锁定 | 必须完全一致 |
| 28 | 帮助中心 | 快捷大文字行 | 词汇锁定 | 必须完全一致 |
| 29 | 更多应用 | 快捷大文字行 | 词汇锁定 | 必须完全一致 |
| 30 | 03 底部状态与全局动作 | 中部标题栏 | 区块标题 | 必须完全一致 |
| 31 | 数据延迟 1.23s | 状态条 | 指标 | 小字需人工校正 |
| 32 | 系统运行 23天14小时 | 状态条 | 指标 | 小字需人工校正 |
| 33 | 告警处置SLA 98.2% | 状态条 | 指标 | 小字需人工校正 |
| 34 | 规则覆盖命中率 99.1% | 状态条 | 指标 | 小字需人工校正 |
| 35 | 存储使用 68.7/120 TB (57%) | 状态条 | 指标 | 小字需人工校正 |
| 36 | 带宽使用 42.7/100 Gbps (43%) | 状态条 | 指标 | 小字需人工校正 |
| 37 | 日志吞吐 12.6 K EPS | 状态条 | 指标 | 小字需人工校正 |
| 38 | 固定顺序：数据延迟 / 系统运行 / SLA / 数据质量 / 存储 / 带宽 / 日志 / 全局动作 | 状态说明 | 顺序说明 | 必须完全一致 |
| 39 | 04 动作语义 | 中下标题栏 | 区块标题 | 必须完全一致 |
| 40 | 危险动作必须包含确认、影响范围、权限和审计留痕 | 动作列表 | 规则 | 必须完全一致 |
| 41 | 图标按钮需要 tooltip / aria-label 语义 | 动作列表 | 规则 | 必须完全一致 |
| 42 | 禁用、加载、错误态不能改变图标家族 | 动作列表 | 规则 | 必须完全一致 |
| 43 | 组件板展示图标时优先复刻 screen.png 线性风格 | 动作列表 | 规则 | 必须完全一致 |
| 44 | 05 固定菜单词汇 | 右侧标题栏 | 区块标题 | 必须完全一致 |
| 45 | 综合态势 | 右侧按钮 1 | 一级菜单词汇 | 必须完全一致 |
| 46 | 采集监测 | 右侧按钮 2 | 一级菜单词汇 | 必须完全一致 |
| 47 | 威胁分析 | 右侧按钮 3 | 一级菜单词汇 | 必须完全一致 |
| 48 | 资产图谱 | 右侧按钮 4 | 一级菜单词汇 | 必须完全一致 |
| 49 | 检测运营 | 右侧按钮 5 | 一级菜单词汇 | 必须完全一致 |
| 50 | 审计配置 | 右侧按钮 6 | 一级菜单词汇 | 必须完全一致 |

## 组件清单

| 组件 | bbox | 类型 | 说明 | 前端映射 |
|---|---:|---|---|---|
| FoundationBoardHeader | `0,0,1920,73` | layout header | 规范板标题栏 | 静态标题组件 |
| FoundationPanel.LeftNav | `24,92,477,947` | panel | 左侧导航图标板 | FoundationPanel |
| PanelTitlebar.LeftNav | `25,93,475,43` | panel titlebar | 左侧面板标题 | SectionTitle |
| AppSidebarSample | `174,149,155,842` | nav sample | 单栏导航样例 | AppSidebar |
| SidebarParentMenuItem | `184,150,141,48` | nav item | 父级综合态势激活 | AppSidebarItem |
| SidebarNormalChildItem | `188,205,134,35` | nav item | 仪表盘默认项 | AppSidebarItem |
| SidebarActiveChildItem | `188,245,135,38` | active nav item | 态势大屏当前项 | AppSidebarItem active |
| SidebarTopicItem | `189,290,133,34` | nav item | 专题面板默认项 | AppSidebarItem |
| SidebarPrimaryDomainItems | `188,350,135,268` | nav group | 五个一级域 | AppSidebar domain list |
| SidebarUserZone | `184,896,142,75` | user block | 底部用户信息 | UserIdentityBlock |
| FoundationPanel.QuickEntry | `528,92,843,319` | panel | 顶部快捷入口板 | FoundationPanel |
| QuickEntryIconButtonStrip | `758,159,382,82` | icon button group | 六个小图标按钮 | QuickEntryIconButton[] |
| QuickEntryVocabularyRow | `575,296,672,18` | text row | 六个词汇大字展示 | TextVocabularyRow |
| FoundationPanel.BottomStatus | `528,438,843,305` | panel | 底部状态与全局动作板 | FoundationPanel |
| BottomStatusStripSample | `559,518,780,36` | status strip | 缩略状态栏 | BottomStatusBar |
| StatusOrderNote | `560,624,676,20` | note | 固定顺序说明 | SecondaryText |
| FoundationPanel.ActionSemantics | `528,771,843,268` | panel | 动作语义板 | FoundationPanel |
| ActionSemanticsList | `559,826,686,156` | bullet list | 四条规则 | RuleBulletList |
| FoundationPanel.FixedMenu | `1398,92,499,947` | panel | 固定菜单词汇板 | FoundationPanel |
| FixedMenuButtonList | `1429,161,438,428` | button list | 六个词汇按钮 | MenuVocabularyButton[] |
| FixedMenuButton | `1430,162,437,47` | button | 统一菜单按钮样式 | Button ghost/outline |
| ChevronRight | `1816,178,14,14` | icon | 进入/展开提示 | ChevronRight icon |

## 图标清单

| 图标 | bbox | 状态 | 语义 | 复刻要点 |
|---|---:|---|---|---|
| 综合态势网格 | `204,164,20,20` | parent-active | overview-domain | 蓝色四宫格，左侧竖条配合父级高亮 |
| 仪表盘房屋 | `205,212,18,18` | default | dashboard | 线性房屋，弱色 |
| 态势大屏圆脸 | `206,254,18,18` | child-active | situation-screen | 蓝色线性，位于当前项背景内 |
| 专题面板清单 | `207,296,17,18` | default | topic-panel | 清单/表单语义 |
| 采集监测准星 | `204,354,23,23` | default | collection-monitoring | 线性准星，弱色 |
| 威胁分析准星 | `204,414,23,23` | default | threat-analysis | 与采集监测同族但语义由文本区分 |
| 资产图谱卡片 | `205,472,21,20` | default | asset-graph | 卡片和连接语义 |
| 检测运营节点 | `204,526,23,23` | default | detection-operations | 三节点连接 |
| 审计配置剪贴板 | `204,586,22,20` | default | audit-settings | 剪贴板/配置语义 |
| 用户头像 | `196,908,28,31` | identity | current-user | 灰色圆形头像 |
| 在线状态绿点 | `232,950,9,9` | success | online | 成功绿，紧贴在线文字 |
| 用户区小动作 | `292,949,15,15` | action | sidebar-user-action | 小型线性图标 |
| PCAP检索 | `792,180,18,18` | action | pcap-search | 青蓝线性图标 |
| 资产检索 | `855,180,19,19` | action | asset-search | 放大镜语义 |
| 规则检索 | `917,181,18,18` | action | rule-search | 规则/图表语义 |
| 脚本中心 | `977,180,18,18` | action | script-center | 文件/脚本语义，非普通文档 |
| 帮助中心 | `1038,180,19,19` | action | help-center | 问号圆形 |
| 更多应用 | `1096,180,19,19` | action | more-apps | 扩展/全屏样式，但语义是更多应用 |
| 数据延迟 | `571,529,8,8` | success | latency | 绿色健康指标 |
| 系统运行 | `648,529,8,8` | success | uptime | 绿色闪电 |
| 告警处置 SLA | `740,529,8,8` | success | sla | 绿色菱形 |
| 规则覆盖命中率 | `860,529,9,9` | success | rule-coverage | 绿色十字星 |
| 存储使用 | `988,529,9,9` | success | storage | 绿色方框 |
| 带宽使用 | `1134,529,9,9` | success | bandwidth | 绿色圆环 |
| 日志吞吐 | `1285,529,9,9` | success | log-throughput | 绿色圆点 |
| 通知铃铛 | `1254,525,17,16` | alert-badge | notification | 红色角标属于底部全局动作 |
| 设置齿轮 | `1278,529,11,11` | action | settings | 底部右侧 |
| 全局配置 | `1298,529,11,11` | action | global-config | 底部右侧 |
| 电源退出 | `1318,529,11,11` | danger-capable | power-or-logout | 危险动作语义 |
| 右箭头 | `1816,178,14,14` | navigation | enter-or-expand | 六个菜单按钮重复使用 |

## Token 与样式

| Token | 值 | 用途 | 约束 |
|---|---|---|---|
| page-bg | `#03111c` | 画布底色 | 全局 foundation 背景 |
| shell-bg | `#061827` | 框架背景 | AppShell 与内层底 |
| panel-bg | `#071f32` | 面板背景 | 五个主面板 |
| deep-panel-bg | `#041827` | 侧栏/图标条内部 | 更深层容器 |
| border-weak | `rgba(56,151,201,.22)` | 面板边框 | 细线，不加厚 |
| border-strong | `#1f8fd0` | 强边框 | 激活和按钮描边 |
| active-blue | `#1e9cff` | 当前项 | 当前导航和强调图标 |
| cyan-accent | `#18d8ff` | 基准文字/快捷图标 | 不替代状态绿 |
| main-text | `#eaf7ff` | 主文字 | 标题和大标签 |
| secondary-text | `#9db9c9` | 次级文字 | 副标题和说明 |
| muted-text | `#5e7b8d` | 弱文字 | 状态条小字、默认图标 |
| success-green | `#36d66b` | 健康态 | 在线、指标、bullet |
| warning-yellow | `#ffb020` | 告警/待确认 | 只用于 warning 语义 |
| danger-red | `#ff4d4f` | 危险和失败 | 通知角标、危险动作 |
| panel-radius | `6px` | 主面板圆角 | 所有大面板一致 |
| button-radius | `4px` | 菜单按钮圆角 | 右侧按钮和 active item |
| titlebar-height | `43px` | 面板标题栏 | 五个面板一致 |
| sidebar-sample-width | `155px` | 导航样例宽度 | 仅为缩放规范样例 |
| status-strip-height | `36px` | 状态条样例 | 单层条 |
| menu-button-height | `47px` | 右侧词汇按钮 | 六项一致 |

## 状态与交互

- 父级导航激活：`综合态势` 通过左侧青色竖条和高亮图标表达。
- 当前子路由：`态势大屏` 通过蓝色文字、蓝色图标、蓝色描边和深色填充表达。
- 默认导航项：`仪表盘`、`专题面板`、其他一级域均使用弱色线性图标。
- 用户在线态：头像旁角色信息下方有绿色 dot 和 `在线` 文本。
- 顶部快捷入口：六个图标均为可点击动作入口。
- PCAP检索：进入 PCAP 搜索，不承担通知语义。
- 资产检索：进入资产检索，不替代资产图谱页面导航。
- 规则检索：进入规则检索，不替代规则管理完整页面。
- 脚本中心：进入脚本中心，图标语义为脚本/文件。
- 帮助中心：进入帮助中心，问号 icon 要有 aria-label。
- 更多应用：图标近似扩展/全屏，但可见文字锁定为更多应用。
- 状态条指标：只读健康信息，不作为主业务指标卡。
- 数据延迟：绿色健康图标，示例值 `1.23s`。
- 系统运行：绿色闪电图标，示例值 `23天14小时`。
- SLA：绿色菱形图标，示例值 `98.2%`。
- 规则覆盖命中率：绿色十字星，示例值 `99.1%`。
- 存储使用：绿色方框图标，示例值 `68.7/120 TB (57%)`。
- 带宽使用：绿色圆环图标，示例值 `42.7/100 Gbps (43%)`。
- 日志吞吐：绿色圆点图标，示例值 `12.6 K EPS`。
- 通知铃铛：红色角标表示有通知。
- 设置齿轮：底部全局动作，不应放在顶部。
- 全局配置/外观：底部全局动作，不应和业务按钮混排。
- 电源/退出：危险动作，必须触发确认链。
- 右侧固定菜单按钮：表示词汇锁定和进入/展开方向。
- 危险动作确认：必须包含确认、影响范围、权限和审计留痕。
- 图标按钮无障碍：必须具备 tooltip 和 aria-label。
- 禁用态：可调低透明度，但不能换图标家族。
- 加载态：可叠加 spinner 或 busy 状态，但不能替换为其他语义图标。
- 错误态：可切换颜色或状态提示，但图标家族保持一致。
- 组件板展示图标：优先复刻 `screen.png` 线性风格。

## 实现映射

- `FoundationBoardHeader` 映射为静态规范板标题组件。
- `FoundationPanel` 映射为统一低饱和深色面板。
- `PanelTitlebar` 统一 43px 高度、左侧编号、底部分割线。
- `AppSidebarSample` 映射到生产 `AppSidebar` 的缩放参考。
- `SidebarParentMenuItem` 映射为一级域展开/激活状态。
- `SidebarActiveChildMenuItem` 映射为当前路由 active 状态。
- `SidebarUserBlock` 映射为侧栏底部用户区。
- `QuickEntryIconButtonStrip` 映射为顶部栏快捷入口按钮组。
- `QuickEntryVocabularyRow` 映射为开发词汇校验，不一定单独渲染。
- `BottomStatusStripSample` 映射为底部状态栏的顺序和动作归属。
- `GlobalActionIconGroup` 映射为通知、设置、全局配置、电源动作。
- `ActionSemanticsList` 映射为交互实现门禁。
- `FixedMenuButtonList` 映射为一级菜单词汇白名单。
- 图标库优先使用 lucide 或已有项目图标，但外观需贴近 `screen.png` 线性风格。
- Ant Design 按钮需要覆盖深色主题边框、背景和 hover 状态。
- 图标按钮需要明确 `aria-label`。
- 危险按钮需要权限校验和审计事件。
- 状态条应使用等宽数字或 tabular-nums。
- 菜单文字应使用中文固定词，不引入非规范同义词。
- 右侧词汇列表不应在生产页面中变成第二个导航栏。
- 顶部栏不应出现用户头像、设置或电源动作。
- 左侧用户区不应搬到顶部栏。
- 底部状态条不应拆成两层。
- 各状态颜色不得交换语义。

## 验收证据

- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-icons-actions.png`
- target：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/target.png`
- implementation：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/implementation.png`
- diff：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/diff.png`
- regions overlay：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/regions-overlay.png`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/text-ocr.txt`
- metrics：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/metrics.json`
- capture metadata：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/capture-meta.json`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-icons-actions/verification.json`
- 浏览器要求：Windows Chrome CDP `http://127.0.0.1:9224`
- 截图要求：1920 x 1080，DPR 1，Windows Chrome 真实截图
- diff 要求：对 target 与 implementation 生成 `diff.png` 和 `metrics.json`
- 辅助审查要求：智能体只读检查截图、overlay、diff 和 verification
- 主线程要求：主线程最终判断 `pixel-accepted`
- 证据边界：reference-raster 证明目标 PNG 可像素复刻，生产语义实现仍需按本记录开发

## 差异清单

| 类型 | 位置 | 当前记录 | 验收要求 | 状态 |
|---|---|---|---|---|
| 视觉截图 | 全图 | Windows Chrome 截图由后续门禁生成 | 必须产生 implementation.png | closed |
| 视觉差异 | 全图 | diff 由后续门禁生成 | mismatch ratio 达到门禁 | closed |
| 区域覆盖 | 全图 | JSON 已记录 30 个区域 | overlay 必须覆盖主结构 | closed |
| 文本校正 | 全图 | 已人工校正 50 条文本 | text-ocr.txt 同步记录 | closed |
| 父子高亮 | 左侧导航 | 综合态势和态势大屏为不同层级状态 | 不得合并为单一 active | closed |
| 快捷入口双规格 | 顶部快捷入口 | 图标组和大文字行分开记录 | 不得误判为重复菜单 | closed |
| 状态动作归属 | 底部状态条 | 全局动作归属底部右侧 | 不得移到顶部栏 | closed |
| 生产边界 | React 实现 | reference-raster 只证明像素复刻 | 语义实现按本记录落地 | documented |

## 结论

- `foundation-icons-actions.png` 是全局图标与动作语义规范板。
- 它锁定了左侧导航图标、顶部快捷入口、底部状态与全局动作、危险动作语义、固定菜单词汇。
- 拆解记录已经按独立图片建档，覆盖区域、文本、组件、图标、token、交互和实现映射。
- 左侧父级 `综合态势` 与子级 `态势大屏` 的双高亮语义已经单独记录。
- 顶部快捷入口的小图标组和大文字行已经按双规格拆开。
- 底部通知、设置、全局配置、电源动作归属已经固定在底部状态栏。
- 右侧六个词汇按钮是一级菜单白名单，不是第二套导航。
- Windows Chrome 截图、视觉 diff、辅助审查和主线程判定已经完成。
- 本图已写入 `pixel-accepted`，生产语义实现仍需按本记录落地。

import fs from "node:fs";
import path from "node:path";

const rootDir = path.resolve(new URL(".", import.meta.url).pathname);
const referenceImage = "doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png";
const outputSize = "1920x1080";
const globalImageConstraint =
  "全局生图约束：后续所有生成、编辑或重生成的 UI 图片，无论是页面、浮层、组件、状态图还是响应式适配图，都必须严格遵循 foundations 的 UI 规范；不得以单张图、局部修图、业务差异或风格自由发挥为由绕过 foundations。";
const globalAppShellConstraint =
  "公共 AppShell 绝对一致性硬门禁：除 login.png、screen.png、登录/认证态和明确不展示 AppShell 的独立素材外，所有 UI 图的公共部分必须与态势大屏 screen.png 完全一致。公共部分包括顶部单栏、左侧单栏和底部单栏；三者的内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态都不得按页面自行变化。顶部栏固定为 screen.png 的系统名称、站点/时间、风险态势、告警数、严重告警、采集健康、数据质量和快捷入口结构；快捷入口固定为 PCAP检索、资产检索、规则检索、脚本中心、帮助中心、更多应用。顶部单栏不得加入通知铃铛、用户头像、用户菜单、设置或电源动作组；顶部的告警数/严重告警只是运行指标，不是通知中心入口。左侧栏固定为 screen.png 的单栏展开式菜单，一级菜单图标和二级菜单图标必须遵循 doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md；不同页面只允许改变当前展开域、二级菜单文本和当前高亮项；用户身份、角色、在线状态和用户动作只归属左侧底部用户区，顶部不得重复展示。底部单栏固定为 screen.png 的数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 / 右侧全局动作图标组；通知角标、设置、全局配置和电源只归属底部右侧全局动作区。修复既有 UI 图时，只允许修改顶部、左侧、底部公共区域，中部业务内容区必须保持原图不变，不得重绘、替换指标、调整业务面板或改变业务布局。";
const globalLeftNavigationConstraint =
  "全局左侧菜单约束：除登录/认证页、态势大屏基准图和移动端导航抽屉等明确例外外，所有出现左侧菜单的 UI 图片都必须与态势大屏 screen.png 的左侧单栏完全一致；同一个侧栏内承载一级菜单和当前一级业务域下的二级菜单，禁止恢复“窄一级栏 + 独立二级栏”的双栏结构。菜单整体宽度、背景、图标尺寸、文字密度、分割线、底部用户区和激活蓝色样式必须复刻 screen.png；一级菜单固定为“综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置”，二级菜单只能显示当前业务域页面，二级菜单图标按 doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md 固定；禁止新增第三层菜单，禁止把二级菜单画成厚重卡片、巨大按钮、工具面板或专题目录，禁止把页面内部模块塞进左侧菜单。";
const promptDir = path.join(rootDir, "prompts");
const screenDirs = {
  foundation: "screens/foundations",
  page: "screens/pages",
  overlay: "screens/overlays",
  component: "screens/components",
  state: "screens/states",
  responsive: "screens/responsive",
};

fs.mkdirSync(promptDir, { recursive: true });
for (const dir of Object.values(screenDirs)) {
  fs.mkdirSync(path.join(rootDir, dir), { recursive: true });
}

for (const file of fs.readdirSync(promptDir)) {
  if (file.endsWith(".prompt.txt") && !file.includes("-chat-imagegen-")) {
    fs.rmSync(path.join(promptDir, file));
  }
}

const pageModules = {
  login: {
    main: "登录入口",
    secondary: "身份认证",
    shell: "auth",
    focus: "统一身份认证、验证码、租户/站点选择、账号安全提示、OIDC/SSO 入口、认证失败提示。",
    blocks: [
      "登录表单：账号、密码、验证码、租户/站点选择、记住登录、提交按钮。",
      "身份入口：账号密码登录、OIDC/SSO 登录、只读演示入口。",
      "安全提示：失败原因、过期回跳、追踪 ID、账号锁定或验证码状态。",
      "能力摘要：采集健康、告警闭环、证据审计、数据质量等产品能力以小型状态块呈现。",
    ],
    visuals: "无左侧导航、无业务顶部 KPI 条；居中登录面板叠加暗色园区拓扑或链路背景，右侧能力摘要保持深色安全运营风格。",
    actions: "登录成功回跳原页面；认证失败显示原因和追踪 ID；可切换租户/站点；只读演示入口进入态势大屏。",
  },
  screen: {
    main: "综合态势",
    secondary: "态势大屏",
    focus: "领导汇报、售前演示、值班大屏和验收展示，一屏讲清全流量采集分析闭环。",
    responsibility:
      "业务职责切分：态势大屏的中心对象是“系统闭环叙事”，不是“待处理事项”。它必须像全流量闭环展示墙，而不是放大版仪表盘。页面主区要让领导、客户或评审一眼理解系统如何从全流量采集走到告警研判、PCAP 取证、响应反馈和验收证据。",
    difference:
      "与仪表盘的差异：态势大屏可以共用顶部状态条和全局数据源，但主工作区不能复用仪表盘的脱敏待办队列、SLA 表格、证据缺口清单、工单逾期清单或验收缺口工作篮。大屏只显示这些事项的总体状态，不展开任务表和批量操作。",
    blocks: [
      "园区拓扑总览：楼宇覆盖率、园区在线覆盖、核心链路状态、汇聚链路状态、异常链路位置、探针覆盖地图。",
      "采集流处理管道：探针采集、协议解析、归一化、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO。",
      "威胁态势：攻击阶段热度、战役簇密度、风险区域密度、异常链路影响面、外联流向强度。",
      "证据取证：PCAP 覆盖率、Session 还原率、日志关联率、对象存储归档率、hash 校验通过率、签名 URL 可用率。",
      "响应与反馈：隔离动作数、阻断动作数、封禁动作数、下发脚本数、反馈标注数、模型学习批次数。",
      "运行底座：大屏刷新间隔、拓扑渲染延迟、链路带宽水位、流向动画帧率、展示脱敏状态。",
    ],
    metrics: "页面专属指标只使用：楼宇覆盖率、园区在线覆盖、核心链路状态、汇聚链路状态、异常链路位置、探针覆盖地图、攻击阶段热度、战役簇密度、风险区域密度、异常链路影响面、外联流向强度、PCAP 覆盖率、Session 还原率、日志关联率、对象存储归档率、hash 校验通过率、签名 URL 可用率、隔离动作数、阻断动作数、封禁动作数、下发脚本数、反馈标注数、模型学习批次数、拓扑渲染延迟、链路带宽水位、流向动画帧率。",
    avoidMetrics: "不要在主工作区使用仪表盘专属指标：超时 SLA、临近超时数、队列积压量、待取证、待反馈、待复核、健康门禁通过率、验收缺口数、复核完成率、审计留痕缺口、证据完整度缺口、Top Talkers 风险贡献。",
    visuals: "保留最终视觉基线的单栏展开式左侧导航、顶部状态条、右侧闭环栏和底部状态栏；不要隐藏常规侧栏。主画面使用大尺寸园区拓扑、横向采集流处理管道、攻击阶段时间线、告警簇、证据完整度和桑基/趋势图。",
    layoutGuard:
      "去重构图要求：不要使用仪表盘式“队列 + 门禁矩阵 + 证据缺口 + 质量摘要”构图；不要出现高密度待办表、SLA 倒计时队列、批量操作栏、工单逾期列表、复杂筛选表单，也不要出现责任人、负责人、头像、班组、个人账号或交接名单。应使用“拓扑 + 管道 + 威胁态势 + 证据闭环 + 响应反馈”的展示墙构图。",
    actions: "从拓扑跳转仪表盘/探针管理，从管道下钻数据质量/部署管理，从威胁态势跳转告警中心/战役列表，从证据跳转取证分析，从响应反馈跳转 SOAR 剧本/MLOps 编排。",
  },
  dashboard: {
    main: "综合态势",
    secondary: "仪表盘",
    focus: "日常运营脱敏工作台，回答今天必须优先关注什么、哪些 SLA/门禁有风险、哪些证据或反馈需要补齐，但不暴露人员、班组、头像或个人任务归属。",
    responsibility:
      "业务职责切分：仪表盘的中心对象是“脱敏的待处理事项和运营工作量”，不是“系统闭环叙事”，也不是人员责任看板。页面主区要围绕异常原因、风险优先级、处理阶段、下一步动作、证据/反馈质量和验收缺口组织。",
    difference:
      "与态势大屏的差异：仪表盘可以共用顶部状态条和全局数据源，但主工作区不能复用态势大屏的园区拓扑、横向采集管道、攻击热力地图、流向动画、证据闭环展示墙或领导汇报式大图。仪表盘应让值班员坐下后立即知道先处理哪几个问题。",
    blocks: [
      "脱敏运营 KPI：超时 SLA、临近超时数、高危未处理、待取证、待反馈、待复核、队列积压量、今日闭环进度。",
      "优先级待办队列：高危未处理、临近超时、待取证、待反馈、待复核、需审计留痕，表格只展示事件 ID、风险级别、资产组、业务系统、处置阶段、剩余时间和证据状态。",
      "采集与数据健康门禁：Probe、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PostgreSQL 的通过/警告/失败状态、失败原因、影响范围和最近更新时间。",
      "告警处置阶段工作篮：今日必处理、处理中、待反馈、待取证、待复核、需审计留痕，以阶段分组和数量变化呈现。",
      "证据与反馈质量摘要：PCAP/Session/日志证据完整度、反馈覆盖率、误报回流量、审计留痕缺口、样本回流缺口。",
      "验收缺口看板：待补证据数、待回流样本数、审计留痕缺口、工单逾期数、合规门禁缺口及建议下一步动作类型。",
    ],
    metrics: "页面专属指标只使用：超时 SLA、临近超时数、高危未处理、待取证、待反馈、待复核、队列积压量、今日闭环进度、健康门禁通过率、门禁失败项、平均确认时长、平均闭环时长、验收缺口数、复核完成率、审计留痕缺口、证据完整度缺口、反馈覆盖率、待补证据数、待回流样本数、工单逾期数、Top Talkers 风险贡献。",
    avoidMetrics: "不要在仪表盘主工作区展示敏感人员信息：责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单、个人任务归属；也不要使用态势大屏专属指标：楼宇覆盖率、园区在线覆盖、核心链路状态、汇聚链路状态、异常链路位置、探针覆盖地图、攻击阶段热度、战役簇密度、风险区域密度、外联流向强度、PCAP 覆盖率、Session 还原率、日志关联率、对象存储归档率、hash 校验通过率、签名 URL 可用率、隔离动作数、阻断动作数、封禁动作数、下发脚本数、拓扑渲染延迟、链路带宽水位。",
    visuals: "左侧一级和二级导航，主区域采用脱敏运营工作台结构：顶部脱敏运营 KPI，左中为优先级待办与 SLA 队列，中部为采集与数据健康门禁矩阵，右侧为验收缺口与建议下一步动作类型，下方为证据/反馈质量摘要、复核完成率和待补证据/待回流样本清单。禁止出现责任人列、负责人头像、班组排班、个人账号、交接时间轴、人员矩阵、园区拓扑画布、攻击热力地图、外联流向动画或横向全链路管道。",
    layoutGuard:
      "去重构图要求：不要使用大屏式中央拓扑，不要使用“左拓扑 + 中管道 + 右闭环栏”的态势大屏构图。应使用“脱敏 KPI + 优先级队列 + 健康门禁 + 证据质量 + 验收缺口”的运营工作台构图，表格和任务清单是主视觉，拓扑和链路只允许作为小图标或入口状态出现。",
    actions: "KPI 跳转告警中心、数据质量、探针管理和合规审计；待办进入告警详情或取证分析；健康门禁进入对应组件详情；证据缺口进入取证分析；验收缺口进入合规审计；审计留痕缺口进入审计日志。",
  },
  topics: {
    main: "综合态势",
    secondary: "专题面板",
    focus: "把加密隧道、数据外传和 APT 战役三类固定攻击场景合并到一个专题研判页面，围绕场景范围、证据门禁、专题信号、处置建议和报告导出形成可交付视图。",
    responsibility:
      "业务职责切分：专题面板的中心对象是一组已固化的攻击场景和证据范围，不是全局态势大屏，也不是日常待办仪表盘。左侧菜单只能出现“专题面板”一个二级入口；加密隧道、数据外传、APT 战役只能作为页面内专题切换状态呈现。",
    difference:
      "与三张 4K 专题背景的关系：4K 背景只作为页内专题模式的视觉信号或头图素材，不拆成三个左侧菜单页面。旧 /topics/tunnel、/topics/exfil、/topics/apt 只作为兼容深链，进入后映射到 /topics?topic=...。",
    blocks: [
      "专题模式切换：加密隧道专题、数据外传专题、APT 战役专题，切换后保留时间窗、站点范围和证据上下文。",
      "场景化 KPI：活跃会话、异常外联、受影响资产、证据完整度、风险评分、报告就绪度。",
      "专题信号卡：隧道协议、外传路径、战役阶段、C2 线索、影响范围、规则/模型命中。",
      "分析路径：发现、聚合、取证、处置、报告五阶段状态线，明确每阶段证据和缺口。",
      "证据门禁表：告警、PCAP、Session、日志、图谱路径、处置记录、hash 校验、审计留痕。",
      "右侧动作栏：保存视图、订阅专题、导出报告、跳转取证、进入 SOAR、回写审计。",
    ],
    metrics: "页面专属指标使用：专题风险评分、证据完整度、报告就绪度、关联告警数、关联资产数、异常外联路径数、隧道会话数、外传链路数、战役阶段数、处置动作数、审计留痕覆盖率。",
    avoidMetrics: "不要把加密隧道、数据外传、APT 战役画成左侧三个二级菜单；不要复用仪表盘待办队列、态势大屏中央拓扑、全局管道大屏或通用专题目录；不要出现 /topics/tunnel、/topics/exfil、/topics/apt 作为菜单项。",
    visuals: "左侧展开综合态势并高亮专题面板；主区为深色高密度专题工作台，可在顶部或主视觉区引用三张 4K 专题背景的科技感信号，但必须叠加可读业务面板。使用页内 segmented control、KPI 条、专题信号网格、五阶段分析路径、证据门禁表、右侧处置动作和报告预览。",
    actions: "切换专题模式；从信号下钻加密流量、取证分析、实体图谱、资产台账、战役列表和攻击链分析；导出专题报告；保存视图；写入审计日志。",
  },
  alerts: {
    main: "威胁分析",
    secondary: "告警中心",
    focus: "安全研判主工作台，围绕告警完成筛选、研判、取证、处置、反馈。",
    blocks: [
      "告警队列：高危、中危、低危、未处理、处理中、已确认、已忽略。",
      "筛选检索：时间窗、资产、IP、规则、模型、攻击阶段、状态、置信度。",
      "选中告警摘要：告警 ID、名称、风险评分、资产、源/目的、规则/模型。",
      "研判时间线：首次发生、异常行为、横向移动、证据生成、处置动作。",
      "关联告警簇：同源 IP、同资产、同攻击链、同规则、同战役。",
      "处置与反馈：隔离、阻断、封禁、下发脚本、TP/FP、误报原因、白名单草案。",
    ],
    visuals: "高密度告警表格、筛选条、右侧详情栏、风险评分环、时间线、关联图、聚类卡片和反馈表单；不要把本页做成园区拓扑大屏或采集管道页。",
    actions: "批量指派、批量状态变更、保存视图；进入告警详情；跳转取证分析；合并战役或进入战役详情；写入 SOAR、MLOps、审计日志。",
  },
  "alert-detail": {
    main: "威胁分析",
    secondary: "告警中心",
    focus: "单条告警研判详情，保留告警上下文并完成证据、资产、响应、反馈闭环。",
    blocks: [
      "研判摘要：告警 ID、严重级别、风险评分、置信度、状态、责任人。",
      "资产上下文：源/目的 IP、主机、服务、业务系统、最近风险画像。",
      "事件时间线：首次发生、规则/模型命中、证据生成、处置动作。",
      "证据链：PCAP、Session、日志、图谱路径、hash 和签名 URL 状态。",
      "响应动作：隔离主机、阻断 IP、封禁账户、下发脚本、创建工单。",
      "反馈学习：TP/FP、误报原因、白名单草案、模型样本回流。",
    ],
    visuals: "详情页保留面包屑和返回入口，主区为研判摘要、时间线、证据表格，右侧固定闭环操作栏。",
    actions: "从告警跳转 PCAP、图谱、资产、SOAR、MLOps 和审计日志，所有危险动作显示影响范围和审计提示。",
  },
  campaigns: {
    main: "威胁分析",
    secondary: "战役列表",
    focus: "把多条告警聚合成事件/战役，支持跨时间、跨资产、跨阶段调查。",
    blocks: [
      "战役总览：战役数量、活跃战役、影响资产、最高风险、持续时间。",
      "战役列表：战役名称、阶段、风险、资产数、告警数、负责人、状态。",
      "战役时间线：初始访问、执行、持久化、横向移动、外联、数据外传。",
      "影响范围：资产、账号、服务、部门、园区、业务系统。",
      "证据汇总：告警、PCAP、Session、日志、图谱路径、处置记录。",
    ],
    visuals: "KPI 卡、趋势图、高密度战役表、ATT&CK 阶段时间线、影响矩阵、资产列表和证据完整度条。",
    actions: "跳转战役详情；指派负责人、变更状态；下钻攻击链分析；跳转资产台账或实体图谱；生成战役报告。",
  },
  "campaign-detail": {
    main: "威胁分析",
    secondary: "战役列表",
    focus: "战役画像、攻击时间轴、关联告警、影响范围、证据包、复盘结论。",
    blocks: [
      "战役画像：名称、风险、阶段、持续时间、负责人、当前状态。",
      "攻击时间轴：关键阶段、关联告警、证据节点、处置节点。",
      "影响范围：资产、账号、服务、部门、园区、业务系统。",
      "证据包：PCAP、Session、日志、图谱路径、处置记录和完整度。",
      "复盘结论：根因、阻断点、遗留风险、整改建议。",
    ],
    visuals: "从发现到闭环的战役故事板，使用时间线、阶段卡、影响矩阵、证据列表和右侧处置建议。",
    actions: "下钻告警详情、攻击链分析、资产台账、实体图谱和取证分析；生成战役报告并写入审计。",
  },
  "attack-chains": {
    main: "威胁分析",
    secondary: "攻击链分析",
    focus: "解释攻击如何发生、经过哪些阶段、影响哪些实体和资产。",
    blocks: [
      "攻击链画布：阶段节点、实体节点、告警节点、证据节点、处置节点。",
      "阶段识别：侦察、初始访问、执行、横向移动、C2、外传。",
      "路径分析：源 IP、目标资产、账号、服务、域名之间的路径。",
      "证据锚点：每个阶段对应 PCAP、Session、日志、规则命中、模型特征。",
      "处置建议：阻断点、隔离对象、白名单风险、剧本推荐。",
    ],
    visuals: "横向攻击链图、泳道图、ATT&CK 矩阵、路径图、证据卡和建议列表并列。",
    actions: "下钻告警和证据；关联规则和模型；跳转实体图谱；跳转取证分析；触发 SOAR 剧本。",
  },
  "encrypted-traffic": {
    main: "威胁分析",
    secondary: "加密流量",
    focus: "对 TLS、QUIC、VPN、隧道和未知加密外联进行可解释分析。",
    blocks: [
      "加密流量总览：TLS/QUIC 比例、未知 SNI、异常证书、可疑 JA3、外联目的地。",
      "指纹分析：JA3/JA3S、证书 issuer、SNI、ALPN、TLS 版本、密码套件。",
      "隧道检测：DNS over HTTPS、异常长连接、低熵/高熵特征、心跳通信。",
      "外联画像：境外 IP、CDN、云服务、异常域名、首次出现目的地。",
      "证据提取：Session、PCAP 索引、证书详情、握手元数据。",
    ],
    visuals: "KPI、协议环图、趋势图、指纹表格、分布图、异常列表、散点图、域名卡和证据抽屉入口。",
    actions: "跳转告警或取证；关联规则和模型；创建告警或战役；跳转实体图谱；进入取证分析。",
  },
  forensics: {
    main: "威胁分析",
    secondary: "取证分析",
    focus: "围绕告警、资产、时间窗完成 PCAP、Session、日志证据检索、切片、下载和审计。",
    blocks: [
      "取证任务：告警 ID、资产、五元组、时间窗、任务状态、责任人。",
      "PCAP 索引：pcap_index、对象路径、大小、hash、时间窗、协议。",
      "会话复放：Session 列表、请求响应摘要、协议、字节数、持续时间。",
      "证据完整性：hash 校验、签名 URL、过期时间、租户隔离、下载审计。",
      "证据导出：PCAP、CSV、日志、图谱路径、报告材料。",
      "跨页上下文：来自告警、战役、资产、图谱的筛选条件。",
    ],
    visuals: "任务表格、状态机、证据表格、hash 标签、会话时间轴、完整度卡、导出弹窗入口和顶部上下文条。",
    actions: "新建取证任务；下载或校验 PCAP；回到告警详情；写入审计日志；生成合规证据包；一键返回来源页面。",
  },
  assets: {
    main: "资产图谱",
    secondary: "资产台账",
    focus: "管理和查看园区终端、服务器、网络设备、业务系统和风险画像。",
    blocks: [
      "资产列表：IP、MAC、主机名、类型、部门、园区、操作系统、重要性。",
      "风险画像：最近告警、暴露端口、异常行为、漏洞、弱口令、外联。",
      "资产详情：基础信息、网络接口、开放服务、归属、负责人、历史变化。",
      "流量画像：入站/出站/东西向、协议分布、Top 对端、周期性连接。",
      "证据关联：关联告警、PCAP、Session、日志、图谱邻居。",
    ],
    visuals: "高密度资产表格、标签、筛选器、风险评分环、趋势图、详情抽屉入口和关联列表。",
    actions: "进入资产详情；跳转告警中心；修改资产或生成工单；跳转行为基准；进入取证分析或实体图谱。",
  },
  graph: {
    main: "资产图谱",
    secondary: "实体图谱",
    focus: "展示 IP、主机、账号、服务、域名、告警、证据之间的实体关系。",
    blocks: [
      "实体搜索：IP、账号、主机、域名、服务、告警 ID、资产 ID。",
      "邻居图谱：一跳/二跳邻居、关系类型、边权重、最近活跃时间。",
      "路径分析：源到目标路径、攻击路径、通信路径、账号访问路径。",
      "实体详情：实体属性、关联资产、关联告警、流量统计、证据。",
      "查询治理：慢查询、节点限制、图谱缓存、查询历史。",
    ],
    visuals: "大图谱画布、搜索建议、关系图例、路径高亮、路径列表、右侧实体详情抽屉和查询状态条。",
    actions: "定位中心节点；打开实体详情；跳转攻击链分析、资产台账、告警详情、取证分析和审计日志。",
  },
  fusion: {
    main: "资产图谱",
    secondary: "数据融合",
    focus: "解释流量、资产、设备日志、用户行为、漏洞/情报等多源数据如何融合并提升检测效果。",
    blocks: [
      "数据源状态：Flow、Asset、Device Log、User Event、Threat Intel、Vulnerability。",
      "融合规则：IP-MAC、账号-主机、资产-部门、域名-IP、告警-资产映射。",
      "实体对齐质量：冲突、缺失、重复、置信度、最近更新时间。",
      "融合增益：融合前后告警准确率、误报率、覆盖率、解释字段完整度。",
      "冲突处理：多来源冲突、人工确认、覆盖策略、审计记录。",
    ],
    visuals: "数据源健康卡、融合规则表、关系示意、雷达图、冲突列表、对比图、指标卡和冲突处理抽屉入口。",
    actions: "跳转数据质量；编辑融合规则；生成修复任务；输出验收证据；冲突确认写入审计日志。",
  },
  baselines: {
    main: "资产图谱",
    secondary: "行为基准",
    focus: "沉淀资产、账号、端口、协议、时间段的正常行为基线，用于异常检测和解释。",
    blocks: [
      "基线总览：资产基线、账号基线、端口基线、流量基线、覆盖率。",
      "偏离检测：新目的地、新端口、异常时间、异常流量、异常会话长度。",
      "基线详情：历史窗口、均值、分位数、阈值、样本量、更新时间。",
      "解释链路：为什么判定异常、关联告警、关联资产、证据来源。",
      "基线治理：冷启动、漂移、重建、冻结、版本管理。",
    ],
    visuals: "KPI、分布图、覆盖率条、偏离列表、箱线图、散点图、趋势图、解释卡和状态机。",
    actions: "下钻具体基线；创建告警或反馈模型；调整阈值；跳转告警/取证；进入模型管理或 MLOps 编排。",
  },
  probes: {
    main: "采集监测",
    secondary: "探针管理",
    focus: "管理园区全流量采集入口，证明系统看得见、采得全、采得稳。",
    blocks: [
      "探针总览：探针数量、在线率、版本、采集模式、CPU/内存、接口状态。",
      "部署拓扑：探针与楼宇、交换机、链路、镜像口、采集网卡关系。",
      "吞吐与丢包：PPS、Gbps、丢包率、解析率、背压、批量发送状态。",
      "探针配置：采集接口、过滤策略、PCAP 归档、mTLS、CPU 亲和、缓冲区。",
      "心跳与日志：最近心跳、异常日志、重启记录、证书状态。",
      "批量运维：批量升级、批量启停、策略下发、连通性测试。",
    ],
    visuals: "KPI 卡、探针状态矩阵、部署拓扑、地图点位、实时折线、阈值线、日志时间线和批量操作栏。",
    actions: "跳转探针详情；下钻园区拓扑和资产台账；跳转数据质量；下发配置并写审计；触发重启、升级、证书轮换；进入部署管理和审计日志。",
  },
  rules: {
    main: "检测运营",
    secondary: "规则管理",
    focus: "管理检测规则生命周期，覆盖创建、测试、发布、命中、误报、回滚。",
    blocks: [
      "规则列表：规则名、类型、严重级别、状态、版本、命中数、误报率。",
      "规则编辑：条件、特征、阈值、MITRE 阶段、适用范围、例外条件。",
      "测试验证：样本回放、命中结果、误报样本、性能影响。",
      "生命周期：草稿、待审、灰度、启用、停用、回滚。",
      "反馈关联：TP/FP、误报原因、白名单草案、规则复审建议。",
    ],
    visuals: "规则表格、筛选器、DSL/条件编辑入口、测试面板、状态机、版本时间线和命中趋势。",
    actions: "进入规则详情；保存草稿或提交审批；进入部署管理；发布或回滚；进入白名单或 MLOps 编排。",
  },
  deployments: {
    main: "检测运营",
    secondary: "部署管理",
    focus: "统一管理规则、模型、采集策略、Flink 作业和配置的发布、灰度、回滚。",
    blocks: [
      "发布清单：发布对象、版本、环境、状态、负责人、时间、影响范围。",
      "灰度策略：按租户、园区、探针、资产组、流量比例灰度。",
      "发布健康：Flink checkpoint、Kafka 消费、告警量变化、误报率、延迟。",
      "回滚管理：可回滚版本、回滚原因、回滚影响、确认人。",
      "发布证据：manifest、镜像、DDL、topic、规则版本、模型版本。",
    ],
    visuals: "发布表格、状态标签、灰度进度、发布监控图、版本对比、回滚弹窗入口和证据列表。",
    actions: "进入发布详情；扩大或停止灰度；回滚或继续发布；回滚写入审计；导出证据并进入合规审计。",
  },
  models: {
    main: "检测运营",
    secondary: "模型管理",
    focus: "管理 AI/行为检测模型的版本、指标、数据集、解释和在线状态。",
    blocks: [
      "模型列表：模型名、类型、版本、状态、线上版本、训练时间、负责人。",
      "模型指标：准确率、召回率、F1、AUC、误报率、漂移、置信区间。",
      "数据集与样本：训练集、验证集、测试集、反馈样本、样本分布。",
      "解释与特征：重要特征、规则贡献、异常解释、样本示例。",
      "激活与回滚：champion/challenger、灰度、激活、停用、回滚。",
    ],
    visuals: "模型表格、状态标签、指标卡、曲线、混淆矩阵、数据集表、分布图、特征条形图和状态机。",
    actions: "进入模型详情；进入 MLOps 编排；追加样本或重训；关联告警详情；写入部署和审计。",
  },
  mlops: {
    main: "检测运营",
    secondary: "MLOps 编排",
    focus: "形成反馈样本 -> 标注 -> 训练 -> 评估 -> 注册 -> 发布 -> 效果回流的模型闭环。",
    blocks: [
      "编排看板：训练任务、评估任务、注册任务、发布任务、失败任务。",
      "反馈样本池：TP/FP、误报原因、标注人、来源告警、白名单建议。",
      "标注管理：待标注、已标注、冲突标注、复核状态、样本质量。",
      "训练任务：数据集版本、算法配置、特征版本、资源占用、日志。",
      "评估与门禁：准确率、召回率、F1、误报率、漂移、回归集通过率。",
      "注册与发布：模型包、版本、签名、模型卡、线上候选、灰度策略。",
      "效果回流：上线后告警命中、反馈量、误报变化、漂移趋势。",
    ],
    visuals: "流水线 DAG、任务队列、样本表、标签分布、训练时间线、日志抽屉入口、门禁矩阵、混淆矩阵和反馈漏斗。",
    actions: "进入任务详情；发起标注或训练；推送训练任务；失败重试或停止；注册模型或退回训练；进入部署管理和模型管理。",
  },
  "data-quality": {
    main: "采集监测",
    secondary: "数据质量",
    focus: "证明数据可信，定位采集、解析、传输、处理、入库中的质量问题。",
    blocks: [
      "质量总分：完整性、及时性、准确性、重复率、字段缺失、DLQ 数量。",
      "Topic 健康：Kafka offset、积压、消费延迟、分区倾斜、消息大小。",
      "Flink 处理质量：checkpoint、watermark、backpressure、异常、迟到数据。",
      "字段质量：五元组、community_id、tenant、asset_id、protocol、timestamp 缺失和异常。",
      "存储质量：ClickHouse 写入、OpenSearch 索引、NebulaGraph 边写入、MinIO 对象归档。",
      "重放与对账：DLQ 重放、时间窗对账、幂等检查、重复检测。",
    ],
    visuals: "质量评分、雷达图、门禁状态、Topic 表格、延迟折线、分区热力、作业健康卡、字段矩阵、存储健康卡和重放任务表。",
    actions: "跳转合规审计；定位 Flink 作业或探针；跳转部署管理；生成修复任务或重放任务；进入系统设置；形成验收证据。",
  },
  playbooks: {
    main: "检测运营",
    secondary: "SOAR 剧本",
    focus: "把人工研判后的响应动作产品化，形成可授权、可回滚、可审计的自动化处置。",
    blocks: [
      "剧本列表：剧本名称、适用告警、动作类型、风险级别、启用状态、最近执行。",
      "剧本编排：条件节点、人工确认节点、隔离/阻断/封禁/脚本节点、回滚节点。",
      "触发策略：告警严重级别、资产重要性、规则/模型命中、时间窗、责任人。",
      "执行历史：执行对象、步骤状态、耗时、失败原因、操作者、关联告警。",
      "风险控制：高危动作二次确认、授权边界、执行前影响评估、冷却时间。",
      "处置效果：执行前后告警变化、连接阻断效果、主机隔离状态、误操作反馈。",
    ],
    visuals: "剧本表格、动作类型标签、流程画布、节点配置抽屉入口、策略表单、步骤时间线、影响范围预览和效果对比图。",
    actions: "进入剧本详情；保存草稿或提交审批；绑定告警中心；重试、回滚或转人工；写入审计日志；回流告警和合规证据。",
  },
  whitelist: {
    main: "检测运营",
    secondary: "白名单",
    focus: "管理业务例外和误报沉淀，避免白名单变成不可控的检测盲区。",
    blocks: [
      "白名单列表：对象类型、匹配条件、生效范围、有效期、责任人、来源告警。",
      "新增白名单：IP、资产、账号、域名、规则、模型、时间窗、例外原因。",
      "审批流程：申请人、审批人、影响范围、风险说明、到期策略。",
      "命中监控：白名单命中次数、覆盖告警、覆盖资产、潜在漏报风险。",
      "到期治理：即将到期、过期未处理、长期生效、无人负责。",
      "反馈关联：从告警 TP/FP、规则复审、模型误报生成白名单草案。",
    ],
    visuals: "白名单表格、状态标签、到期提醒、条件构造器入口、审批状态机、命中趋势、影响矩阵和来源链路卡。",
    actions: "查看或编辑白名单；提交审批；生效或驳回；调整范围或撤销；延期、停用、转审计；回到告警和规则管理。",
  },
  compliance: {
    main: "审计配置",
    secondary: "合规审计",
    focus: "面向任务书、第三方评测和工程验收，证明系统能力、证据和运行状态可交付。",
    blocks: [
      "验收门禁：采集覆盖、数据质量、告警链路、PCAP 证据、MLOps、审计留痕、部署基线。",
      "指标映射：任务书指标、测试项、对应页面、对应数据源、责任人。",
      "证据包：测试报告、PCAP hash、审计日志、模型版本、规则版本、部署 manifest。",
      "运行报告：时间窗内告警、处置、数据质量、系统健康、模型效果。",
      "缺口治理：未达标项、原因、责任模块、计划完成时间、复验状态。",
      "第三方评测：外部评测样本、测试批次、通过率、复测记录。",
    ],
    visuals: "门禁矩阵、红黄绿状态、指标追踪表、覆盖率条、证据列表、完整度进度、报告预览、缺口看板和批次表格。",
    actions: "下钻未通过项；生成整改任务；导出证据包；导出 PDF/Word；指派整改和复验；固化验收记录。",
  },
  "audit-log": {
    main: "审计配置",
    secondary: "审计日志",
    focus: "追踪所有关键操作，满足安全运营、内部追责和验收取证要求。",
    blocks: [
      "日志检索：用户、租户、时间、对象类型、动作类型、结果、请求 ID。",
      "操作详情：操作前后值、对象 ID、IP、User-Agent、trace_id、失败原因。",
      "高风险审计：导出下载、PCAP 访问、规则发布、模型激活、剧本执行、令牌变更。",
      "关联链路：告警、证据、规则、模型、部署、白名单、合规报告之间的审计链。",
      "留存状态：日志保留周期、归档位置、完整性校验、脱敏状态。",
      "导出取证：按时间窗、对象、用户导出审计材料。",
    ],
    visuals: "搜索筛选条、高密度表格、详情抽屉入口、Diff 视图、高风险标签、关系链、时间线、留存卡和 hash 校验。",
    actions: "定位操作记录；关联业务对象；触发复核；跳回来源页面；进入系统设置；生成合规证据。",
  },
  notifications: {
    main: "审计配置",
    secondary: "通知配置",
    focus: "配置告警、系统异常、验收缺口和运营任务的通知渠道与升级策略。",
    blocks: [
      "通知渠道：邮件、短信、Webhook、企业微信/钉钉、工单系统。",
      "订阅规则：严重级别、告警类型、资产组、园区、时间窗、接收人。",
      "升级策略：SLA 超时、未确认、处置失败、重复告警、验收缺口。",
      "模板管理：告警模板、取证模板、数据质量模板、合规报告模板。",
      "发送历史：通知对象、渠道、状态、失败原因、重试次数。",
      "抑制与静默：维护窗口、重复合并、低优先级静默、专题免打扰。",
    ],
    visuals: "渠道卡片、健康标签、规则表、条件构造器、升级流程图、时间阶梯、模板编辑器、历史表格和日历视图。",
    actions: "新增或测试渠道；保存订阅策略；绑定负责人和值班表；测试发送；重试或静默；写入审计日志。",
  },
  settings: {
    main: "审计配置",
    secondary: "系统设置",
    focus: "管理租户、角色权限、令牌、数据留存、集成凭据和系统级参数。",
    blocks: [
      "租户与站点：租户、园区、部门、资产组、数据隔离范围。",
      "角色权限：安全值班员、研判员、管理员、审计员、只读大屏账号。",
      "API 令牌：令牌名称、权限范围、过期时间、最近使用、轮换状态。",
      "数据留存：Flow、Session、Alert、Evidence、PCAP、Audit 的保留周期。",
      "集成配置：Keycloak、APISIX、Kafka、MinIO、OpenSearch、NebulaGraph、Webhook。",
      "安全策略：登录策略、密码策略、MFA、IP 访问控制、脱敏策略。",
      "系统参数：时间窗默认值、告警阈值、页面刷新频率、大屏脱敏、功能开关。",
    ],
    visuals: "树表、站点卡、RBAC 矩阵、权限树、令牌表、留存策略表、集成卡、策略表单和分组参数表。",
    actions: "同步资产和权限范围；保存并写审计；创建、轮换、吊销令牌；更新生命周期策略；连接测试；触发安全审计；提示配置影响范围。",
  },
  "not-found": {
    main: "异常状态",
    secondary: "404 异常页",
    shell: "error",
    focus: "页面不存在、返回工作流、查看审计日志、联系管理员，不能暴露敏感路径。",
    blocks: [
      "错误摘要：页面不存在、追踪 ID、时间、当前租户/站点。",
      "返回入口：返回仪表盘、态势大屏、告警中心或上一页。",
      "安全提示：不展示内部路径、堆栈、凭据或接口细节。",
      "辅助动作：查看审计日志、联系管理员、复制追踪 ID。",
    ],
    visuals: "保持深色产品系统，使用工程化错误面板和小型链路背景，不使用营销插画或空泛装饰。",
    actions: "返回首页或上一页；复制追踪 ID；跳转审计日志；联系管理员。",
  },
};

const overlayDetails = {
  "modal-alert-batch": {
    baseTitle: "告警中心",
    focus: "批量确认告警、批量分派、批量忽略前的影响提示",
    layout: "在告警中心页面上方显示居中 Modal，背景轻微压暗，底部有取消和确认按钮。",
  },
  "dropdown-alert-batch-actions": {
    baseTitle: "告警中心",
    focus: "批量操作下拉菜单，包含批量分派、标记处理中、导出证据、忽略",
    layout: "保持告警队列可见，在表格工具栏按钮下展开紧凑下拉菜单。",
  },
  "dropdown-alert-row-actions": {
    baseTitle: "告警中心",
    focus: "单条告警行操作，包含查看详情、生成 PCAP、创建工单、加入战役",
    layout: "在告警表格行尾展开小型菜单，当前行高亮。",
  },
  "modal-alert-status": {
    baseTitle: "告警详情",
    focus: "更新告警状态为处理中、已遏制、已关闭并填写处置备注",
    layout: "详情页上显示状态变更 Modal，含状态单选、负责人、备注和审计提示。",
  },
  "modal-alert-feedback": {
    baseTitle: "告警详情",
    focus: "提交真实威胁、误报、低危、中危、高危反馈，回流模型学习",
    layout: "Modal 中显示标签结果、置信度星级、说明输入框和提交反馈按钮。",
  },
  "modal-evidence-detail": {
    baseTitle: "告警详情",
    focus: "查看单条 PCAP/会话/日志证据的元数据、十六进制摘要和下载动作",
    layout: "宽 Modal，左侧证据属性，右侧数据预览和链路完整性。",
  },
  "modal-forensics-task": {
    baseTitle: "取证分析",
    focus: "取证任务详情、切片范围、存储位置、处理状态、失败重试",
    layout: "任务详情 Modal 覆盖取证工作台，突出进度和输出文件。",
  },
  "drawer-campaign-detail": {
    baseTitle: "战役列表",
    focus: "从列表右侧滑出战役详情，展示聚类原因、阶段、证据和处置建议",
    layout: "右侧 Drawer 宽约 38%，底层战役列表保持可见。",
  },
  "drawer-attack-chain-detail": {
    baseTitle: "攻击链分析",
    focus: "攻击链节点详情、证据、命中规则、关联资产和下一步调查",
    layout: "右侧 Drawer 覆盖攻击链画布，节点高亮不变。",
  },
  "drawer-graph-entity": {
    baseTitle: "实体图谱",
    focus: "实体节点详情、标签、风险、关联边、最近会话和告警",
    layout: "图谱右侧属性抽屉，选中节点在画布中发光。",
  },
  "drawer-asset-detail": {
    baseTitle: "资产台账",
    focus: "资产详情、端口服务、风险评分、告警历史、基线偏离",
    layout: "从右侧展开资产详情 Drawer，保留资产表格背景。",
  },
  "drawer-baseline-metrics": {
    baseTitle: "行为基准",
    focus: "基准指标详情、学习窗口、偏离阈值、异常样本",
    layout: "右侧 Drawer 展示曲线和指标说明。",
  },
  "modal-rule-edit": {
    baseTitle: "规则管理",
    focus: "新建或编辑检测规则，包含 DSL、条件、严重级别、灰度范围",
    layout: "大 Modal，左侧表单，右侧规则预览和测试结果。",
  },
  "drawer-rule-detail": {
    baseTitle: "规则管理",
    focus: "规则详情、版本历史、命中趋势、发布记录、关联模型",
    layout: "右侧 Drawer 展示规则生命周期。",
  },
  "modal-deployment-create": {
    baseTitle: "部署管理",
    focus: "新建部署计划，选择对象、环境、窗口、回滚策略和审批人",
    layout: "分步骤 Modal，底层部署流水线压暗。",
  },
  "modal-deployment-rollback": {
    baseTitle: "部署管理",
    focus: "回滚确认，展示影响范围、最近版本、风险和确认输入",
    layout: "危险操作确认 Modal，红色风险提示明确但不夸张。",
  },
  "modal-whitelist-add": {
    baseTitle: "白名单",
    focus: "添加白名单，填写对象、规则、理由、有效期和审批链",
    layout: "表单 Modal，右侧显示影响评估。",
  },
  "popconfirm-whitelist-delete": {
    baseTitle: "白名单",
    focus: "删除白名单二次确认，展示受影响规则和最近命中数",
    layout: "小型 Popconfirm 锚定表格行操作按钮。",
  },
  "drawer-model-detail": {
    baseTitle: "模型管理",
    focus: "模型详情、版本、训练数据、评估指标、在线表现和回滚入口",
    layout: "右侧 Drawer，底部固定上线/回滚动作。",
  },
  "modal-settings-token": {
    baseTitle: "系统设置",
    focus: "创建 API 令牌，设置权限、有效期、访问范围和安全提醒",
    layout: "设置页上打开 Modal，令牌值区域带一次性展示提示。",
  },
  "popconfirm-settings-token-revoke": {
    baseTitle: "系统设置",
    focus: "撤销 API 令牌确认，展示调用方、最近使用时间和影响范围",
    layout: "小型危险 Popconfirm，锚定令牌列表行按钮。",
  },
  "modal-topic-save-view": {
    baseTitle: "专题面板",
    focus: "固化当前专题模式、筛选范围、时间窗、资产范围和证据门禁，保存为可复用视图",
    layout: "居中 Modal，只展示专题保存业务容器本体；包含视图名称、专题模式、筛选摘要、可见范围、刷新策略、权限边界和保存审计提示。",
  },
  "drawer-topic-scope-edit": {
    baseTitle: "专题面板",
    focus: "编辑专题范围、名称、时间窗、站点/资产范围、刷新周期、证据门禁和影响提示",
    layout: "右侧宽 Drawer，只展示专题范围编辑容器；左侧为基础信息和范围表单，右侧为影响预览、证据覆盖率和审计门禁。",
  },
  "modal-topic-report-export": {
    baseTitle: "专题面板",
    focus: "导出专题报告 PDF/Word，选择报告模板、脱敏级别、证据范围和审批门禁",
    layout: "居中宽 Modal，只展示专题报告导出容器；包含报告格式、章节选择、专题时间窗、附件清单、脱敏策略、审批提示和导出动作。",
  },
  "modal-topic-evidence-package-export": {
    baseTitle: "专题面板",
    focus: "导出专题证据包或试点周报，聚合告警、PCAP、Session、日志、图谱路径和审计留痕",
    layout: "居中宽 Modal，只展示证据包导出容器；左侧为证据类型选择，右侧为完整性校验、hash、签名 URL、有效期和权限提示。",
  },
  "drawer-topic-subscription": {
    baseTitle: "专题面板",
    focus: "配置专题订阅、静默、免打扰、阈值触发、通知渠道和审计留痕",
    layout: "右侧 Drawer，只展示专题订阅容器；包含订阅对象、触发条件、静默窗口、通知渠道、模板预览和生效范围。",
  },
  "dropdown-topic-share-favorite": {
    baseTitle: "专题面板",
    focus: "专题分享、收藏、复制链接、生成只读链接和权限边界提示",
    layout: "紧凑 Dropdown，只展示从专题工具按钮展开的菜单容器；包含收藏、复制链接、分享给角色、生成只读链接和审计提示。",
  },
  "modal-campaign-report-export": {
    baseTitle: "战役详情",
    focus: "战役详情生成战役报告，选择攻击阶段、影响范围、证据包、处置结论和复盘建议",
    layout: "居中宽 Modal，只展示战役报告生成容器；包含报告模板、阶段时间轴、证据完整度、影响范围、脱敏策略、审批门禁和导出动作。",
  },
  "modal-forensics-evidence-export": {
    baseTitle: "取证分析",
    focus: "导出 PCAP、CSV、日志、图谱路径和取证材料，体现 hash 校验、签名 URL、有效期和下载审计",
    layout: "居中宽 Modal，只展示取证导出容器；包含证据类型选择、时间窗、对象路径、hash 校验、签名链接、脱敏级别和权限确认。",
  },
  "drawer-compliance-gate-detail": {
    baseTitle: "合规审计",
    focus: "验收门禁未通过项下钻，展示门禁规则、失败原因、影响范围、证据缺口和整改动作",
    layout: "右侧宽 Drawer，只展示合规门禁详情容器；包含门禁摘要、失败项矩阵、证据缺口、责任域、整改建议、复验入口和审计编号。",
  },
  "modal-compliance-evidence-package-export": {
    baseTitle: "合规审计",
    focus: "导出合规证据包，选择门禁项、证据类型、时间窗、脱敏级别、hash 校验和审批门禁",
    layout: "居中宽 Modal，只展示合规证据包导出容器；包含证据清单、完整性校验、导出格式、签名 URL、有效期和审计提示。",
  },
  "modal-compliance-report-export": {
    baseTitle: "合规审计",
    focus: "导出运行报告 PDF/Word，覆盖门禁结果、整改状态、证据摘要、风险说明和审批留痕",
    layout: "居中宽 Modal，只展示合规运行报告导出容器；包含章节配置、报告格式、数据范围、脱敏策略、审批链和导出预检。",
  },
  "drawer-audit-operation-detail": {
    baseTitle: "审计日志",
    focus: "审计操作详情抽屉，承载字段 Diff、请求上下文、关联链路、Trace ID 和取证动作",
    layout: "右侧宽 Drawer，只展示审计操作详情容器；包含操作摘要、字段变更 Diff、请求上下文、关联告警/资产/证据链和导出审计材料动作。",
  },
  "modal-audit-export": {
    baseTitle: "审计日志",
    focus: "导出审计取证材料并做权限确认，选择日志范围、字段脱敏、签名校验和审批门禁",
    layout: "居中 Modal，只展示审计导出容器；包含时间窗、操作类型、对象范围、字段选择、脱敏预览、hash 校验和权限确认。",
  },
  "modal-notification-channel-edit": {
    baseTitle: "通知配置",
    focus: "新增或编辑通知渠道，配置 Webhook、邮件、短信、IM、认证方式、路由规则和测试状态",
    layout: "居中宽 Modal，只展示通知渠道编辑容器；包含渠道类型、连接参数、凭证脱敏、路由条件、健康检查和保存审计提示。",
  },
  "modal-notification-template-preview-test": {
    baseTitle: "通知配置",
    focus: "通知模板预览和测试发送，展示变量映射、样例消息、目标渠道、测试结果和审计提示",
    layout: "居中宽 Modal，只展示模板预览与测试发送容器；左侧为模板变量和样例数据，右侧为预览消息、测试目标、发送结果和失败原因。",
  },
  "drawer-notification-silence-rule": {
    baseTitle: "通知配置",
    focus: "抑制、静默和维护窗口规则，配置作用对象、条件、时间窗、例外项和冲突提示",
    layout: "右侧宽 Drawer，只展示通知静默规则容器；包含规则条件、维护窗口、受影响通知、冲突检测、审批门禁和生效审计。",
  },
  "drawer-settings-rbac-edit": {
    baseTitle: "系统设置",
    focus: "RBAC 权限矩阵编辑，展示角色、资源、动作、租户边界、影响范围和审批提示",
    layout: "右侧宽 Drawer，只展示 RBAC 编辑容器；包含角色选择、权限矩阵、资源范围、策略冲突、影响用户数、审批门禁和审计留痕。",
  },
  "drawer-mobile-navigation": {
    baseTitle: "仪表盘",
    focus: "移动端侧滑菜单，展示一级菜单、二级菜单、用户状态和站点选择",
    layout: "模拟窄屏移动端，左侧 Drawer 展开，背景内容压暗。",
  },
  "dropdown-user-menu": {
    baseTitle: "系统设置",
    focus: "用户菜单，包含个人信息、切换角色、安全设置、退出登录",
    layout: "从左侧底部用户卡触发的用户菜单浮层本体，向右展开到业务内容区；不绘制顶部头像入口、通知用户组或完整公共 AppShell。",
  },
  "popconfirm-delete": {
    baseTitle: "规则管理",
    focus: "规则删除确认，显示规则名、影响范围和审计记录",
    layout: "小型 Popconfirm 锚定规则表格删除按钮，危险按钮为红色。",
  },
};

const secondaryMenus = {
  "综合态势": ["仪表盘", "态势大屏", "专题面板"],
  "采集监测": ["探针管理", "数据质量"],
  "威胁分析": ["告警中心", "战役列表", "攻击链分析", "加密流量", "取证分析"],
  "资产图谱": ["资产台账", "实体图谱", "数据融合", "行为基准"],
  "检测运营": ["规则管理", "部署管理", "模型管理", "MLOps 编排", "SOAR 剧本", "白名单"],
  "审计配置": ["合规审计", "审计日志", "通知配置", "系统设置"],
};

const foundationSpecs = [
  ["foundation-visual-reference", "最终视觉基准板", "说明最终深色安全运营台的导航、顶部状态条、主工作区、右侧闭环栏、底部状态栏和业务闭环。"],
  ["foundation-layout-grid", "布局与栅格规范", "展示 1920x1080 AppShell、单栏展开式导航、内容网格、面板间距，以及桌面/平板/移动端适配原则。"],
  ["foundation-color-status", "色彩与状态语义", "展示背景、面板、边框、主色、成功、警告、高危、禁用、审计等语义色。"],
  ["foundation-typography-density", "字体与密度规范", "展示产品标题、页面标题、面板标题、KPI 数字、表格正文、字段标签和辅助说明字号。"],
  ["foundation-icons-actions", "图标与动作语义", "展示导航图标、告警、资产、证据、处置、审计、配置等图标和动作使用规则。"],
  ["foundation-data-viz", "数据可视化规范", "展示折线、面积、环图、桑基、拓扑、图谱、时间线、矩阵、热力图样式。"],
  ["foundation-table-form", "表格与表单密度规范", "展示高密度表格、筛选区、表单项、按钮组、批量操作和分页。"],
  ["foundation-responsive", "响应式适配原则", "展示桌面、平板、移动端抽屉导航、右侧栏收起和图表压缩策略。"],
].map(([id, title, focus]) => ({ type: "foundation", id, title, focus }));

const overlaySpecs = [
  ["dropdown-user-menu", "用户下拉菜单", "settings"],
  ["drawer-mobile-navigation", "移动端侧滑菜单", "dashboard"],
  ["drawer-notification-center", "通知中心抽屉", "notifications"],
  ["modal-global-search", "全局搜索弹窗", "dashboard"],
  ["dropdown-quick-entry", "快速入口下拉", "dashboard"],
  ["modal-login-error-captcha", "登录异常与验证码状态", "login"],
  ["drawer-dashboard-kpi-detail", "仪表盘 KPI 详情", "dashboard"],
  ["drawer-dashboard-task-detail", "待办任务详情", "dashboard"],
  ["modal-screen-readonly-token", "态势大屏只读令牌/脱敏配置", "screen"],
  ["drawer-probe-detail", "探针详情", "probes"],
  ["modal-probe-config", "探针配置下发", "probes"],
  ["modal-probe-batch-upgrade", "探针批量升级确认", "probes"],
  ["modal-probe-cert-rotate", "证书轮换确认", "probes"],
  ["drawer-probe-log", "探针日志抽屉", "probes"],
  ["drawer-dlq-sample", "DLQ 样例详情", "data-quality"],
  ["modal-data-replay-task", "数据重放任务", "data-quality"],
  ["drawer-field-quality-sample", "字段质量样例", "data-quality"],
  ["modal-alert-batch", "告警批量操作确认", "alerts"],
  ["dropdown-alert-batch-actions", "告警批量操作下拉", "alerts"],
  ["dropdown-alert-row-actions", "告警行操作下拉", "alerts"],
  ["modal-alert-status", "更新告警状态", "alert-detail"],
  ["modal-alert-feedback", "提交告警反馈", "alert-detail"],
  ["modal-evidence-detail", "证据详情", "alert-detail"],
  ["modal-forensics-task", "取证任务详情", "forensics"],
  ["drawer-campaign-detail", "战役详情抽屉", "campaigns"],
  ["drawer-attack-chain-detail", "攻击链详情抽屉", "attack-chains"],
  ["drawer-encrypted-fingerprint", "加密指纹详情", "encrypted-traffic"],
  ["drawer-certificate-detail", "证书详情", "encrypted-traffic"],
  ["modal-playbook-trigger", "从告警触发剧本", "alert-detail"],
  ["modal-whitelist-draft-from-alert", "从告警生成白名单草案", "alert-detail"],
  ["popconfirm-pcap-download", "PCAP 下载确认", "forensics"],
  ["drawer-session-replay", "会话复放抽屉", "forensics"],
  ["drawer-asset-detail", "资产详情", "assets"],
  ["modal-asset-edit", "编辑资产", "assets"],
  ["drawer-asset-history", "资产历史", "assets"],
  ["drawer-graph-entity", "图谱实体详情", "graph"],
  ["drawer-graph-path-analysis", "图谱路径分析", "graph"],
  ["drawer-fusion-conflict", "数据融合冲突处理", "fusion"],
  ["modal-fusion-rule-edit", "融合规则编辑", "fusion"],
  ["modal-baseline-threshold", "基线阈值编辑", "baselines"],
  ["modal-rule-edit", "新建/编辑规则", "rules"],
  ["drawer-rule-detail", "规则详情", "rules"],
  ["popconfirm-delete", "规则删除确认", "rules"],
  ["modal-rule-publish", "规则发布确认", "rules"],
  ["modal-deployment-create", "新建部署", "deployments"],
  ["modal-deployment-rollback", "回滚部署确认", "deployments"],
  ["drawer-model-detail", "模型详情", "models"],
  ["drawer-mlops-task-detail", "MLOps 任务详情", "mlops"],
  ["modal-playbook-edit", "剧本编辑", "playbooks"],
  ["modal-whitelist-add", "添加白名单", "whitelist"],
  ["drawer-whitelist-approval", "白名单审批详情", "whitelist"],
  ["modal-settings-token", "创建 API 令牌", "settings"],
  ["modal-topic-save-view", "专题保存视图", "topics"],
  ["drawer-topic-scope-edit", "专题范围编辑", "topics"],
  ["modal-topic-report-export", "专题报告导出", "topics"],
  ["modal-topic-evidence-package-export", "专题证据包导出", "topics"],
  ["drawer-topic-subscription", "专题订阅配置", "topics"],
  ["dropdown-topic-share-favorite", "专题分享收藏菜单", "topics"],
  ["modal-campaign-report-export", "战役报告导出", "campaign-detail"],
  ["modal-forensics-evidence-export", "取证证据导出", "forensics"],
  ["drawer-compliance-gate-detail", "合规门禁详情", "compliance"],
  ["modal-compliance-evidence-package-export", "合规证据包导出", "compliance"],
  ["modal-compliance-report-export", "合规运行报告导出", "compliance"],
  ["drawer-audit-operation-detail", "审计操作详情", "audit-log"],
  ["modal-audit-export", "审计材料导出", "audit-log"],
  ["modal-notification-channel-edit", "通知渠道编辑", "notifications"],
  ["modal-notification-template-preview-test", "通知模板预览测试", "notifications"],
  ["drawer-notification-silence-rule", "通知静默规则", "notifications"],
  ["popconfirm-settings-token-revoke", "API 令牌吊销确认", "settings"],
  ["drawer-settings-rbac-edit", "RBAC 权限编辑", "settings"],
].map(([id, title, base]) => ({ type: "overlay", id, title, base }));

const componentSpecs = [
  ["component-app-header", "顶部状态栏"], ["component-primary-sidebar", "左侧一级导航"],
  ["component-secondary-menu", "二级菜单"], ["component-bottom-status-bar", "底部状态栏"],
  ["component-breadcrumb-context", "面包屑与上下文条"], ["component-site-time-selector", "站点与时间选择"],
  ["component-quick-entry", "快速入口"], ["component-user-menu", "用户菜单"],
  ["component-button", "按钮"], ["component-icon-button", "图标按钮"],
  ["component-status-chip", "标签与 Badge"], ["component-tooltip", "Tooltip"],
  ["component-tabs", "Tabs"], ["component-segmented", "Segmented"],
  ["component-dropdown", "Dropdown"], ["component-pagination", "Pagination"],
  ["component-input", "输入框"], ["component-search", "搜索框"],
  ["component-select", "选择器"], ["component-date-range", "日期时间窗"],
  ["component-switch-checkbox-radio", "开关/复选/单选"], ["component-condition-builder", "条件构造器"],
  ["component-batch-action-bar", "批量操作栏"], ["component-data-table", "高密度表格"],
  ["component-description-list", "详情描述列表"], ["component-kpi-tile", "KPI 卡"],
  ["component-health-card", "状态卡"], ["component-ranking-list", "排行列表"],
  ["component-log-list", "日志列表"], ["component-evidence-file-card", "证据文件卡"],
  ["component-empty-card", "空状态卡"], ["component-permission-card", "权限提示卡"],
  ["component-line-area-chart", "折线/面积图"], ["component-donut-chart", "环图"],
  ["component-bar-ranking-chart", "柱状/排行图"], ["component-sankey-flow", "桑基流向图"],
  ["component-radar-quality", "雷达/质量评分"], ["component-heatmap", "热力图"],
  ["component-topology-graph", "拓扑/图谱"], ["component-timeline-state-machine", "时间线/状态机"],
  ["component-alert-queue", "告警队列"], ["component-risk-score", "风险评分"],
  ["component-alert-timeline", "告警时间线"], ["component-evidence-drawer", "证据抽屉"],
  ["component-asset-context", "资产上下文"], ["component-action-rail", "响应动作栏"],
  ["component-feedback-block", "反馈学习块"], ["component-acceptance-gate-matrix", "验收门禁矩阵"],
].map(([id, title]) => ({ type: "component", id, title }));

const componentDetails = {
  "component-health-card": [
    "本次修改要求：本图只生成 Health Card / 健康状态卡组件规格板，不绘制完整 AppShell、顶部栏、左侧完整菜单、底部状态栏、通知铃铛、顶部用户头像、顶部用户名、设置/电源动作组或宿主页面。组件板可以使用 foundations 的颜色、字号、边框、圆角和状态语义，但画面焦点必须完全落在健康卡组件本体。",
    "组件结构必须覆盖：标题、对象名称、健康状态、健康分、核心指标、子检查项、最近心跳、错误摘要、依赖服务徽标、趋势微图、建议动作、下钻/修复按钮、审计提示。",
    "业务样例必须覆盖：Probe 节点健康、Kafka Topic 健康、Flink checkpoint、ClickHouse/OpenSearch/NebulaGraph/MinIO 存储健康、数据质量门禁、模型部署健康。",
    "状态矩阵必须覆盖：健康、预警、降级、严重、离线、维护中、加载中、数据陈旧、权限锁定；健康/通过用绿色，预警用黄色，严重/失败用红色，信息态用蓝色。",
    "边界约束：Health Card 不等同于 KPI Tile，不做单指标摘要；不等同于 Alert Card，不承载告警处置流；不等同于 Ranking List，不展示 TopN 排名；不复刻完整监控大屏或顶部状态栏指标。",
  ],
  "component-ranking-list": [
    "本次修改要求：本图只生成 Ranking List / TopN 排行列表组件规格板，不绘制完整 AppShell、顶部栏、左侧完整菜单、底部状态栏、通知铃铛、顶部用户头像、顶部用户名、设置/电源动作组或宿主页面。组件板可以使用 foundations 的颜色、字号、边框、圆角和状态语义，但画面焦点必须完全落在排行列表组件本体。",
    "组件结构必须覆盖：排名序号、实体名称、主指标值、环比/趋势、风险等级、进度条、标签、趋势微图、行 hover、行 selected、下钻动作、审计提示。",
    "业务样例必须覆盖：高风险资产 TopN、异常链路 TopN、攻击阶段 TopN、外联目的地 TopN、Kafka lag 分区 TopN、规则命中 TopN、证据缺口 TopN、模型特征贡献 TopN、慢查询 TopN。",
    "状态矩阵必须覆盖：正常、悬停、选中、加载骨架、空数据、数据陈旧、权限隐藏、文本溢出、并列排名、阈值预警；高风险必须使用红色，中风险使用黄色，健康/低风险使用绿色或蓝色。",
    "边界约束：Ranking List 不等同于 DataTable，不承载多列表格筛选和批量动作；不等同于 Bar Chart，不把横向柱图作为主体；不复刻完整 dashboard；列表行只提供轻量下钻、复制、定位和审计入口。",
  ],
  "component-log-list": [
    "本次修改要求：本图只生成 Log List / 日志列表组件规格板，不绘制完整 AppShell、顶部栏、左侧完整菜单、底部状态栏、通知铃铛、顶部用户头像、顶部用户名、设置/电源动作组或宿主页面。组件板可以使用 foundations 的颜色、字号、边框、圆角和状态语义，但画面焦点必须完全落在日志列表组件本体。",
    "组件结构必须覆盖：时间戳、日志级别、来源组件、对象 ID、trace_id、message 摘要、上下文标签、展开详情、复制、筛选高亮、定位来源、审计留痕和脱敏提示。",
    "业务样例必须覆盖：Probe 采集日志、Kafka 消费日志、Flink checkpoint 日志、ClickHouse 写入日志、OpenSearch 索引日志、SOAR 执行日志、审计操作日志、告警处置日志。",
    "状态矩阵必须覆盖：正常、hover、selected、展开详情、加载骨架、空日志、实时暂停、数据陈旧、敏感字段脱敏、错误行、权限锁定；ERROR/FATAL 必须使用红色，WARN 使用黄色，INFO/DEBUG 使用蓝色或绿色。",
    "边界约束：Log List 不等同于 DataTable，不承载复杂多列表格和批量操作；不等同于 Audit Log 页面，不展示完整审计检索页；不等同于 Timeline，不按阶段故事线表达；不复刻完整日志中心页面。",
  ],
  "component-evidence-file-card": [
    "本次修改要求：本图只生成 Evidence File Card / 证据文件卡组件规格板，不绘制完整 AppShell、顶部栏、左侧完整菜单、底部状态栏、通知铃铛、顶部用户头像、顶部用户名、设置/电源动作组或宿主页面。组件板可以使用 foundations 的颜色、字号、边框、圆角和状态语义，但画面焦点必须完全落在证据文件卡组件本体。",
    "组件结构必须覆盖：文件类型图标、文件名、证据类型、关联对象、大小、时间窗、hash 校验、签名 URL 状态、保留期、权限范围、下载/预览/复制 hash/关联告警动作、审计提示。",
    "业务样例必须覆盖：PCAP 切片、Session 重建、日志包、图谱路径证据、模型样本、合规报告、SOAR 执行产物、规则测试样本。",
    "状态矩阵必须覆盖：可下载、签名生成中、hash 通过、hash 失败、即将过期、已归档、权限不足、脱敏预览、病毒/风险拦截、加载骨架、空证据；hash 失败和风险拦截使用红色，到期和签名生成使用黄色，校验通过使用绿色。",
    "边界约束：Evidence File Card 不等同于完整证据详情 Modal，不展开十六进制预览或全文日志；不等同于文件表格，不承载批量选择；不等同于上传组件，不处理拖拽上传；危险下载或外发必须进入确认/审计流程。",
  ],
};

const stateSpecs = [
  ["state-page-loading", "页面加载中"], ["state-table-loading", "表格加载中"],
  ["state-chart-loading", "图表加载中"], ["state-empty-page", "页面空数据"],
  ["state-empty-table", "表格空数据"], ["state-empty-chart", "图表空数据"],
  ["state-api-error", "API 错误"], ["state-network-error", "网络异常"],
  ["state-unauthorized", "未登录"], ["state-forbidden", "无权限"],
  ["state-partial-degraded", "部分降级"], ["state-offline-probe", "探针离线"],
  ["state-stream-backpressure", "流处理背压"], ["state-task-running", "任务运行中"],
  ["state-task-failed", "任务失败可重试"], ["state-success-accepted", "验收通过/操作成功"],
].map(([id, title]) => ({ type: "state", id, title }));

const stateDetails = {
  "state-unauthorized": [
    "认证语义硬门禁：本图只表达 HTTP 401、登录态失效、Token 过期、会话超时或未携带有效凭证。状态主因必须写清“需要重新认证”或“会话已过期”，主按钮必须是“重新登录”或“重新认证”，辅助动作可为“返回登录页”“查看会话日志”“联系管理员”。不得画成 RBAC 权限不足、租户授权拒绝、资源访问被拒或管理员审批场景；不得把“申请权限”作为主恢复动作。画面需要展示 Trace ID、失效时间、租户/站点上下文、当前请求路径，并说明会话失效已写入安全审计。",
  ],
  "state-forbidden": [
    "授权语义硬门禁：本图只表达 HTTP 403、用户已登录但角色权限不足、租户边界不允许、RBAC/ABAC 策略拒绝或资源授权缺失。状态主因必须写清“当前账号无权访问该资源”或“策略拒绝”，主按钮必须是“申请权限”或“联系管理员”，辅助动作可为“返回上一页”“查看审计记录”“复制 Trace ID”。不得画成未登录、Token 过期或会话超时；不得把“重新登录”作为主恢复动作。画面需要展示当前用户/角色、目标资源、所需权限、拒绝策略、审计编号和 Trace ID，并明确拒绝事件已写入审计日志。",
  ],
};

const responsiveSpecs = [
  ["responsive-dashboard-1440", "仪表盘 1440 视口适配策略"],
  ["responsive-dashboard-1920", "仪表盘 1920x1080"],
  ["responsive-screen-4k", "态势大屏 4K 视口适配策略"],
  ["responsive-alerts-1440", "告警中心 1440 视口适配策略"],
  ["responsive-alerts-1920", "告警中心 1920x1080"],
  ["responsive-forensics-1440", "取证分析 1440 视口适配策略"],
  ["responsive-graph-1440", "实体图谱 1440 视口适配策略"],
  ["responsive-compliance-1440", "合规审计 1440 视口适配策略"],
  ["responsive-tablet-dashboard", "平板仪表盘适配策略"],
  ["responsive-tablet-alert-detail", "平板告警详情适配策略"],
  ["responsive-mobile-navigation", "移动端导航抽屉适配策略"],
  ["responsive-mobile-alert-list", "移动端告警列表适配策略"],
].map(([id, title]) => ({ type: "responsive", id, title }));

const splitTabDesignPageIds = new Set(["topics"]);
const allPageSpecs = [
  ["login", "登录", "/login"],
  ["screen", "态势大屏", "/screen"],
  ["dashboard", "仪表盘", "/dashboard"],
  ["topics", "专题面板", "/topics"],
  ["alerts", "告警中心", "/alerts"],
  ["alert-detail", "告警详情", "/alerts/:alertId"],
  ["campaigns", "战役列表", "/campaigns"],
  ["campaign-detail", "战役详情", "/campaigns/:campaignId"],
  ["attack-chains", "攻击链分析", "/attack-chains"],
  ["encrypted-traffic", "加密流量", "/encrypted-traffic"],
  ["forensics", "取证分析", "/forensics"],
  ["assets", "资产台账", "/assets"],
  ["graph", "实体图谱", "/graph"],
  ["fusion", "数据融合", "/fusion"],
  ["baselines", "行为基准", "/baselines"],
  ["probes", "探针管理", "/probes"],
  ["rules", "规则管理", "/rules"],
  ["deployments", "部署管理", "/deployments"],
  ["models", "模型管理", "/models"],
  ["mlops", "MLOps 编排", "/mlops"],
  ["data-quality", "数据质量", "/data-quality"],
  ["playbooks", "SOAR 剧本", "/playbooks"],
  ["whitelist", "白名单", "/whitelist"],
  ["compliance", "合规审计", "/compliance"],
  ["audit-log", "审计日志", "/audit-log"],
  ["notifications", "通知配置", "/notifications"],
  ["settings", "系统设置", "/settings"],
  ["not-found", "404 异常页", "*"],
]
  .map(([id, title, route]) => ({ type: "page", id, title, route, file: `pages/${id}.png` }));

const pageSpecs = allPageSpecs.filter((item) => !splitTabDesignPageIds.has(item.id));

function pageSpecFor(id) {
  return allPageSpecs.find((x) => x.type === "page" && x.id === id);
}

function moduleFor(item) {
  if (item.type === "page") return pageModules[item.id] ?? pageModules.dashboard;
  const base = pageSpecFor(item.base);
  return pageModules[base?.id] ?? pageModules.dashboard;
}

function routeFor(item) {
  if (item.type === "page") return item.route;
  const base = pageSpecFor(item.base);
  return base?.route ?? item.base;
}

function overlayFor(item) {
  const base = pageSpecFor(item.base);
  return overlayDetails[item.id] ?? {
    baseTitle: base?.title ?? item.base,
    focus: `${item.title}，必须体现权限、影响范围、状态解释、下一步动作和审计留痕。`,
    layout: "在对应业务页面上展示清晰的业务浮层状态，背景轻微压暗；浮层内容按深色高密度表单、详情、状态机或确认提示组织。",
  };
}

function formatLines(lines) {
  return lines.map((line) => `- ${line}`).join("\n");
}

const foundationCompliance =
  [
    globalImageConstraint,
    "Foundation 硬门禁：生成图必须严格遵守参考图中的 8 张 foundations 规范板，不能只是风格相似。",
    globalAppShellConstraint,
    "必须锁定 AppShell 结构：1920x1080 单屏、公共区域必须与当前态势大屏 screen.png 一致；顶部状态栏实测 80px，左侧单栏实测 166px，底部状态栏实测 y=997 / h=83，主内容区按 12 栅格和 8px 面板间距组织。",
    globalLeftNavigationConstraint,
    "必须锁定视觉 token：页面底 #03111c、框架底 #061827、面板底 rgba(6,28,43,0.86) 或 #071f32、弱边框 rgba(56,151,201,0.22)、激活蓝 #1e9cff、主文字 #eaf7ff、次级文字 #9db9c9。",
    "必须锁定状态语义：健康/通过用绿色，信息/低危用蓝色，中危/待确认用黄色或琥珀色，高危/失败用红色；不得交换状态颜色。",
    "必须锁定字体密度：产品标题约 24px，页面主标题约 18-20px，面板标题 15-16px，表格正文 12-13px，辅助说明 11-12px，数字使用等宽数字；不得出现夸张大标题或不可读小字。",
    "必须锁定组件规范：面板圆角 6px，按钮圆角 4px，表格行高约 32px，图标按钮带 tooltip 语义，禁止卡片套卡片，禁止营销式大卡片堆叠。",
    "必须锁定图表规范：ECharts 风格深色透明背景，网格线低透明青色，图例不遮挡标题，容器高度稳定，loading/empty/error/unauthorized/offline/degraded/success 状态可表达。",
    "如果生成结果偏离 foundations 的布局、色彩、字号、圆角、表格密度、图表样式或状态语义，应视为不合格并重新生成。",
  ].join("\n");

const metricUniqueness =
  "指标去重约束：除系统固定顶部状态条和底部状态栏外，不同独立页面的主工作区、右侧栏、表格和图表指标名称不能重叠；每个页面必须使用本页面专属指标口径，不能复用其他页面的业务指标。";

function commonVisualPrompt() {
  return [
    "生成一张企业级前端 UI 高保真图，系统名称固定为“园区网络全流量采集与分析系统”。",
    `参考图像：${referenceImage}。请以它作为最终视觉基线，严格延续深色安全运营台风格、信息密度、单栏展开式导航、右侧闭环栏和单层底部状态栏。`,
    `画布比例 16:9，目标 ${outputSize} px，单屏完整展示，不要出现浏览器边框、手机外壳、营销海报、水印或解释性外框。`,
    "风格固定：深海军蓝 SOC 指挥台、青蓝描边、低饱和面板、细分割线、绿色健康、黄色中危、红色高危、克制发光、高密度表格、工程系统质感。",
    foundationCompliance,
    metricUniqueness,
    "差异化约束：每张图必须围绕自身图片 ID、页面/组件类型和业务重点重新组织主视觉、内容区比例、核心图表/表格/流程和闭环动作；不得与已生成页面仅替换标题、菜单或数字后形成近似画面。",
    "一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。",
    "界面文字以中文为主，面板标题中文-only；技术词允许 Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PCAP、TLS、DNS、IP、JA3、MLOps、SOAR、Gbps、K EPS。",
    "内容必须贴合 Probe、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PostgreSQL、PCAP、MLOps 和审计等真实链路。",
    "危险动作必须体现确认、权限、影响范围和审计提示。",
  ].join("\n");
}

function commonPrompt(item) {
  const mod = moduleFor(item);
  const menus = secondaryMenus[mod.main] ?? [];
  const menuLine = mod.shell === "auth"
    ? "登录/认证类页面不显示左侧业务导航和二级菜单，但视觉风格必须与主系统一致。"
    : menus.length > 0
    ? `左侧二级菜单显示：${menus.join("、")}，当前高亮“${mod.secondary}”。`
    : "当前页面不显示常规二级菜单。";
  const topLine = mod.shell === "auth"
    ? "登录/认证类页面不显示业务顶部 KPI 条，可展示系统名称、租户/站点、安全提示和能力摘要。"
    : "顶部状态条保留站点、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、快捷入口等能力入口。";

  return [
    "生成一张企业级前端 UI 视觉效果图，系统名称固定为“园区网络全流量采集与分析系统”。",
    `参考图像：${referenceImage}。请严格延续该参考图的视觉风格、密度和信息架构，而不是重新发明风格。`,
    "风格关键词：深海军蓝 SOC 指挥台、暗色运维后台、青蓝描边、低饱和面板、细分割线、绿色健康状态、黄色中危预警、红色高危风险、克制发光、紧凑表格、工程系统质感。",
    foundationCompliance,
    metricUniqueness,
    `画布比例 16:9，目标 ${outputSize} px，单屏完整展示，不要出现浏览器边框、手机外壳或营销落地页布局。`,
    "所有页面标题和卡片标题使用统一字号：主标题约 18px，面板标题约 16px，表格正文约 13px，辅助说明约 12px；标题不要忽大忽小。",
    "界面文字必须以中文为主，严禁出现“中文 / English”这种双语标题；只允许保留必要技术词和单位，例如 Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PCAP、TLS、DNS、IP、JA3、MLOps、SOAR、Gbps、K EPS。",
    "左侧一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置；不要使用“看见、研判、取证、治理、验收”等非规范菜单词。",
    menuLine,
    topLine,
    "页面内容应体现园区网络全流量采集与分析业务闭环：采集接入、流式处理、资产识别、威胁检测、告警研判、证据取证、响应处置、反馈学习、审计验收。",
    "不要改变最终参考图的整体视觉方向：不要改成浅色、不要做成插画海报、不要过度圆角、不要大面积渐变球或装饰光斑。",
    "页面差异化约束：每个独立页面必须有独特的信息架构和主工作区形态，不能与其他页面相似；需根据本页面业务选择不同的主视觉重心，例如拓扑、队列表格、攻击链、证据工作台、图谱画布、规则状态机、MLOps DAG、合规门禁矩阵、设置表单等。禁止只复用同一套卡片网格、折线图和右侧栏后替换文字。",
  ].join("\n");
}

function promptFor(item) {
  if (item.type === "foundation") {
    return [
      commonVisualPrompt(),
      "",
      `本图类型：视觉基线与规范板。\n规范板名称：${item.title}。\n图片 ID：${item.id}。`,
      `规范重点：${item.focus}`,
      "必须以设计系统看板方式呈现，可包含标注、组件样例、状态样例、尺寸线和简短中文说明；不要生成营销海报。",
      "必须服务后续 React + Ant Design + ECharts 落地，所有示例都采用深色安全运营台风格。",
      "",
      "输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。",
    ].join("\n");
  }
  if (item.type === "component") {
    return [
      commonVisualPrompt(),
      "",
      `本图类型：元件与组件板。\n组件板名称：${item.title}。\n图片 ID：${item.id}。`,
      ...(componentDetails[item.id] ?? []),
      "组件板必须展示正常、悬停、选中、禁用、加载、错误或危险等关键状态，必要时展示尺寸、间距、颜色和交互语义。",
      "组件内容必须贴合全流量采集分析系统，例如告警、资产、证据、规则、模型、审计、采集链路、数据质量和响应动作。",
      "组件要能被前端拆成 React + Ant Design + ECharts 组件，不要只做装饰图。",
      "",
      "输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。",
    ].join("\n");
  }
  if (item.type === "state") {
    return [
      commonVisualPrompt(),
      "",
      `本图类型：通用状态规范图。\n状态名称：${item.title}。\n图片 ID：${item.id}。`,
      ...(stateDetails[item.id] ?? []),
      "状态图必须展示状态原因、可恢复动作、权限或审计提示，以及在页面、表格、图表或任务中的落位方式。",
      "错误、异常、失败、高危状态必须能说明原因，并提供重试、查看详情、返回、联系管理员、跳转证据或写入审计等下一步动作。",
      "不要使用空泛插画；使用工程化深色状态面板、图标、按钮、追踪 ID、时间窗和对象上下文。",
      "",
      "输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。",
    ].join("\n");
  }
  if (item.type === "responsive") {
    return [
      commonVisualPrompt(),
      "",
      `本图类型：响应式与大屏适配图。\n场景名称：${item.title}。\n图片 ID：${item.id}。`,
      "交付图片本身仍必须是 1920x1080 px。可以在同一画布内表达对应视口下的布局策略，例如导航收起、右侧闭环栏折叠、表格压缩、图表重排和移动端抽屉。",
      "必须保持 6 个一级菜单、当前业务模块、状态色、中文标题和关键业务闭环不变。",
      "不要输出真实 1440、2K、4K、平板或手机原始像素尺寸；只在 1920x1080 画布内做适配策略说明。",
      "",
      "输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。",
    ].join("\n");
  }

  const mod = moduleFor(item);
  const route = routeFor(item);
  const header = item.type === "page"
    ? `本图类型：完整页面。\n页面名称：${item.title}。\n路由：${route}。`
    : `本图类型：浮层/弹窗/抽屉/下拉状态。\n浮层名称：${item.title}。\n基准页面：${overlayFor(item).baseTitle}。\n基准路由：${route}。`;
  const detail = item.type === "page"
    ? [
      `业务模块：${mod.main} / ${mod.secondary}。`,
      `页面重点：${mod.focus}`,
      ...(mod.responsibility ? [mod.responsibility] : []),
      ...(mod.difference ? [mod.difference] : []),
      ...(mod.metrics ? [`页面专属指标：${mod.metrics}`] : []),
      ...(mod.avoidMetrics ? [`禁止复用指标：${mod.avoidMetrics}`] : []),
      "必须包含的业务模块：",
      formatLines(mod.blocks ?? []),
      `表现形式与布局：${mod.visuals ?? mod.layout}`,
      ...(mod.layoutGuard ? [mod.layoutGuard] : []),
      `闭环动作与下钻：${mod.actions ?? "必须提供下一步动作、下钻入口和审计留痕。"}`,
    ].join("\n")
    : [
      `业务模块：${mod.main} / ${mod.secondary}。`,
      `浮层重点：${overlayFor(item).focus}`,
      `浮层布局：${overlayFor(item).layout}`,
      "浮层图以当前交互容器本体为主，可使用深色空画布、轻微压暗背景或极弱宿主暗影作为承托；除非本条 prompt 明确要求，不绘制完整顶部栏、左侧菜单、底部栏或宿主页面公共区。浮层文字、按钮、状态和危险提示必须清楚。",
    ].join("\n");

  return [
    commonPrompt(item),
    "",
    header,
    detail,
    "",
    "输出要求：只输出 UI 视觉效果图本身，不要额外解释，不要水印，不要伪造浏览器地址栏。画面需要能作为前端开发和 Figma 设计参考。",
  ].join("\n");
}

function materializeItem(item) {
  const outFile = `${screenDirs[item.type]}/${item.id}.png`;
  const promptFile = `prompts/${item.id}.prompt.txt`;
  const promptPath = path.join(rootDir, promptFile);
  fs.writeFileSync(promptPath, promptFor(item), "utf8");
  const route = item.type === "page" || item.type === "overlay" ? routeFor(item) : undefined;
  return {
    ...item,
    file: outFile,
    ...(route ? { route } : {}),
    targetFile: `doc/04_assets/ui_suite_gpt_v1/${outFile}`,
    promptFile: `doc/04_assets/ui_suite_gpt_v1/${promptFile}`,
    referenceImage,
    status: "prompt-ready",
  };
}

const generatedItems = [
  ...foundationSpecs,
  ...pageSpecs,
  ...overlaySpecs,
  ...componentSpecs,
  ...stateSpecs,
  ...responsiveSpecs,
].map(materializeItem);

const manifest = {
  version: "gpt-ui-suite-v1",
  product: "园区网络全流量采集与分析系统",
  referenceImage,
  globalImageConstraint,
  globalAppShellConstraint,
  globalLeftNavigationConstraint,
  outputSize,
  legacySourceSuite: "doc/04_assets/ui_suite_v1 removed on 2026-06-23; page specs are self-contained in build_prompt_manifest.mjs",
  total: generatedItems.length,
  counts: generatedItems.reduce((acc, item) => {
    acc[item.type] = (acc[item.type] ?? 0) + 1;
    return acc;
  }, {}),
  items: generatedItems,
};

fs.writeFileSync(path.join(rootDir, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`, "utf8");

const pageRows = generatedItems
  .filter((item) => item.type === "page")
  .map((item, index) => `| ${index + 1} | ${item.id} | ${item.title} | ${item.route} | ${item.targetFile} |`)
  .join("\n");
const overlayRows = generatedItems
  .filter((item) => item.type === "overlay")
  .map((item, index) => `| ${index + 1} | ${item.id} | ${item.title} | ${item.base} | ${item.targetFile} |`)
  .join("\n");
const foundationRows = generatedItems
  .filter((item) => item.type === "foundation")
  .map((item, index) => `| ${index + 1} | ${item.id} | ${item.title} | ${item.targetFile} |`)
  .join("\n");
const componentRows = generatedItems
  .filter((item) => item.type === "component")
  .map((item, index) => `| ${index + 1} | ${item.id} | ${item.title} | ${item.targetFile} |`)
  .join("\n");
const stateRows = generatedItems
  .filter((item) => item.type === "state")
  .map((item, index) => `| ${index + 1} | ${item.id} | ${item.title} | ${item.targetFile} |`)
  .join("\n");
const responsiveRows = generatedItems
  .filter((item) => item.type === "responsive")
  .map((item, index) => `| ${index + 1} | ${item.id} | ${item.title} | ${item.targetFile} |`)
  .join("\n");

const readme = `# GPT 生图 UI 视觉套装 v1

本目录用于生成“园区网络全流量采集与分析系统”${manifest.total} 张工业级高保真 UI 套装。视觉基准采用：

- ${referenceImage}

像素规范：所有高保真 UI 图一律输出为 \`${outputSize} px\` PNG。

全局生图约束：后续所有生成、编辑或重生成的 UI 图片，无论是页面、浮层、组件、状态图还是响应式适配图，都必须严格遵循 foundations 的 UI 规范；不得以单张图、局部修图、业务差异或风格自由发挥为由绕过 foundations。

${globalAppShellConstraint}

${globalLeftNavigationConstraint}

## 范围

- 总图数：${manifest.total}
- 视觉基线与规范板：${manifest.counts.foundation}
- 页面主图：${manifest.counts.page}
- 业务浮层图：${manifest.counts.overlay}
- 元件与组件板：${manifest.counts.component}
- 通用状态图：${manifest.counts.state}
- 响应式与大屏适配图：${manifest.counts.responsive}

## 当前状态

- 已生成 ${manifest.total} 个 GPT 生图 prompt：\`prompts/*.prompt.txt\`
- 已生成本地生成清单：\`manifest.json\`
- 图片目标目录：\`screens/foundations\`、\`screens/pages\`、\`screens/overlays\`、\`screens/components\`、\`screens/states\`、\`screens/responsive\`
- 最近一次 API 冒烟调用已到达 OpenAI，但当前本地 \`OPENAI_API_KEY\` 返回 \`401 invalid_api_key\`；切换可用项目 key 后，可直接执行 \`run_generation.sh\` 继续生成。

## 生成命令

\`\`\`bash
cd /home/wangwt/phase_2/code/traffic-analysis-platform
QUALITY=medium SIZE=1920x1080 bash doc/04_assets/ui_suite_gpt_v1/run_generation.sh
\`\`\`

可选参数：

- \`ONLY_ID=alerts\`：只生成指定条目
- \`LIMIT=3\`：只生成前 3 张
- \`START_AT=rules\`：从指定条目开始恢复
- \`DRY_RUN=1\`：只打印计划，不调用 API

## 视觉基线与规范板

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
${foundationRows}

## 页面清单

| # | ID | 页面 | 路由 | 目标文件 |
| - | - | - | - | - |
${pageRows}

Tab 拆分设计合并口径：\`/topics\` 是当前唯一现役专题页面和左侧菜单项；加密隧道、数据外传、APT 战役已有拆分 Tab 设计输入，前端开发时必须合并到同一个 \`/topics\` 页面内作为页内 Tab/Segmented 状态实现，不再生成单张 \`topics.png\`。旧 \`/topics/tunnel\`、\`/topics/exfil\`、\`/topics/apt\` 只作为兼容深链或 API 语义来源，不进入页面主图清单。后续任何页面如果 UI 设计图按 Tab 拆多张，前端也必须合并为一个路由页面内的 Tab 状态，不能拆成多个左侧菜单或独立业务路由。

## 浮层清单

| # | ID | 浮层 | 基准页面 | 目标文件 |
| - | - | - | - | - |
${overlayRows}

## 元件与组件板

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
${componentRows}

## 通用状态图

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
${stateRows}

## 响应式与大屏适配图

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
${responsiveRows}
`;

fs.writeFileSync(path.join(rootDir, "README.md"), readme, "utf8");

console.log(JSON.stringify({
  total: manifest.total,
  counts: manifest.counts,
  outputSize,
  promptDir: "doc/04_assets/ui_suite_gpt_v1/prompts",
  manifest: "doc/04_assets/ui_suite_gpt_v1/manifest.json",
}, null, 2));

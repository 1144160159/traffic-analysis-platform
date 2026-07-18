# 规则管理 前端实现契约

## 基本信息

- ID：`rules`
- 路由：`/rules`
- 领域：`detection-ops`
- React 页面：`RuleManagementPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/rules.png`
- API：`/api/v1/rules`

## 必须实现的业务层

- 规则定义
- 测试验证
- 依赖引用
- 样本回放
- 发布门禁

规则定义、测试验证、依赖引用以及 PCAP/Session/日志样本均为 `/rules` 原页面内部 Tab 状态；切换时不得产生独立路由或 `view` 查询参数。测试验证和依赖引用始终复用中部规则编辑容器，三类样本始终复用左下样本回放容器，切换不得改变规则列表、中部编辑器、右侧栏和底部业务区的外框几何。

## 关键布局约束

- 业务区域顶部只显示“规则管理”，不显示路径或面包屑。
- 规则草稿、待审核规则、灰度规则、启用规则、回滚候选和高耗时规则采用一个连续 KPI 条；六个语义图标必须清晰可辨，并与标签、数值、变化量形成稳定的三行布局。
- 生命周期固定显示草稿、待审、灰度、启用、停用和回滚六阶段，采用六个单圆形语义图标、五段带箭头连接线；连接线必须贴合相邻圆圈边缘，箭头必须位于连线几何中点并与圆心同轴。当前阶段紫色高亮由 `/api/v1/rules` 返回的规则状态动态决定；下方同时显示当前状态、最近变更时间和操作人，禁止出现重复圆点。
- 规则定义的四条条件及例外条件均采用“字段 / 运算符 / 值”三个真实下拉选择框，保持原图边框、箭头和行内删除入口，不得退化为普通文本。
- DSL 使用可输入、可修改、可聚焦的代码编辑区，保留“格式化”操作，禁止使用只读 `pre` 冒充编辑器。
- MITRE 阶段区域左侧显示可删除的“TA0011 指挥与控制”，右侧保留“添加阶段”；删除与恢复均须可交互。
- 三类样本回放 Tab 复用左下原模块，并分别按 UI 图显示 PCAP 文件表、Session 五元组/协议/命中字段徽标/回放下载、日志来源/规则字段徽标/命中原因/查看标记/误报开关；四行数据和“查看全部样本”入口完整可见。
- 版本历史必须在右侧版本容器内完整展示四条版本记录和底部“查看更多版本”，不得被审批清单覆盖。
- 测试验证结果表和依赖关系表使用各自容器内的纵向滚动条，不得扩张或覆盖原编辑容器。
- 依赖引用图使用 ECharts 动态关系图，保留中心规则、六类依赖对象、分类颜色、箭头连线和影响提示；缩放到原编辑容器后标签仍须可辨。
- PCAP、Session、日志样本表的表头与四行数据必须保持单元格边界，长文本单行省略，操作按钮或开关不得压住相邻字段；规则字段继续使用带边框徽标，标题字和数据行字号须与模块密度协调。

## 当前生产验收

- 镜像：`docker.io/traffic/web-ui:rule-management-visual-fix-20260717-r137`
- Windows Chrome：150，视口 `1920x1080`
- 自动交互：`16/16 passed`
- 单元测试：`163/163 passed`
- 证据：`evidence/ui-image-breakdowns/pages/rules/interaction-r253-rules-lifecycle-typography.json`
- 截图：`interaction-r253-overview.png`、`interaction-r253-sample-pcap-embedded.png`、`interaction-r253-sample-session-embedded.png`、`interaction-r253-sample-logs-embedded.png`
- 生命周期几何：5 段连接线的左右端点分别与相邻圆圈边缘重合（误差小于 1px），箭头中心位于各段几何中点，连线中心与圆心同轴。
- 样本字号：三个页签的 Tab、表头和数据行均为 `8px`，规则字段徽标为 `7px`，模块标题为 `10px`；字段徽标、操作按钮及误报开关均位于自身单元格内。
- 视觉差异：业务 ROI `content-root(198,80,1722,917)` 的像素不匹配率为 `0.074915`，低于门限 `0.125`；见 `metrics-business-r137.json`。
- 正常模式：Windows Chrome 直接打开不带视觉参数的 `/rules`，真实 `/api/v1/rules` 两页分页、`workbench.source=postgresql`、11 类 46 条工作台数据、`disabled -> 停用` 生命周期映射全部通过；无 `-SIM` 数据。
- 权限与审计：viewer action `403`、跨租户 workbench `403`、admin action `202 queued`，`rule_action_jobs:audit_logs=1:1`；见 `interaction-r254-normal-api.json`。
- 正常模式视觉差异：业务 ROI 像素不匹配率 `0.069168 < 0.125`；见 `metrics-normal-api-r254.json`。
- Rule Manager：`docker.io/traffic/rule-manager@sha256:e6484634d5745506c312d76954b982741b64a955b9c78caad56d4eea9bda1b90`。

## 分层参数

- `topbar`：global-app-shell，bbox=`{"x":0,"y":0,"w":1920,"h":80}`
- `sidebar`：global-app-shell，bbox=`{"x":0,"y":80,"w":166,"h":917}`
- `content`：page-workspace，bbox=`{"x":198,"y":80,"w":1722,"h":917}`
- `bottombar`：global-app-shell，bbox=`{"x":0,"y":997,"w":1920,"h":83}`
- `right-rail`：closed-loop-rail，bbox=`{"x":1460,"y":104,"w":420,"h":860}`

## 组件映射

- AppShell
- WorkPanel
- MetricTile
- Table
- Tabs
- ECharts
- StatusTag

## 关联浮层

- `modal-rule-edit`：新建/编辑规则，Modal
- `drawer-rule-detail`：规则详情，Drawer
- `popconfirm-delete`：规则删除确认，Popconfirm
- `modal-rule-publish`：规则发布确认，Modal

## 验收清单

- [x] 最终 PNG 必须为 1920x1080
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色必须遵守 success/info/warning/danger/critical token
- [x] 危险动作必须具备影响范围、权限提示和审计留痕
- [x] 公共 AppShell 必须与 screen.png 目标参数一致
- [x] 页面主工作区不得复用相邻页面的业务组件组合
- [x] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [x] React Query 必须覆盖 loading/error/empty 状态
- [x] 所有 Tab 切换后 URL 必须保持 `/rules`，Tab 栏必须保留在原容器
- [x] 三个样本 Tab 的直接单元格不存在相交或文字重叠
- [x] 版本历史底部操作完整位于版本历史容器内
- [x] 测试验证结果和依赖关系表均提供容器内滚动条
- [x] Windows Chrome 1920x1080 截图与目标图进行同屏视觉核对
- [x] 生命周期为六节点、五连接段，且无旧样式产生的重复圆点
- [x] 四条规则条件与例外条件均为三个可交互下拉选择框
- [x] DSL 编辑区可直接修改并恢复内容，MITRE 阶段支持删除与添加
- [x] 生命周期连接线、箭头与六个圆形节点处于同一中心线
- [x] 生命周期五段连线均贴合相邻圆圈，箭头位于各连线几何中点
- [x] 生命周期高亮阶段由 API 返回的规则状态动态决定
- [x] PCAP、Session、日志三类样本表均按各自 UI 图重构且无单元格重叠
- [x] 三类样本保留规则字段边框，表头和数据行字号与目标图一致

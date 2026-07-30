# 专题面板前端实现契约

## 基本信息

- ID：`topics`
- 路由：`/topics?topic=tunnel|exfil|apt`
- React 页面：`web/ui/src/pages/TopicWorkbenchPage.tsx`
- 固定区域：页面标题“专题面板”和三个专题 Tab 保持不变。
- 视觉真源：
  - `screens/pages/topics-encrypted-tunnel.png`
  - `screens/pages/topics-data-exfiltration.png`
  - `screens/pages/topics-apt-campaign.png`
- 读取 API：`/api/v1/topics/tunnel`、`/api/v1/topics/exfil`、`/api/v1/topics/apt`
- 模拟数据真源：PostgreSQL `topic_panel_simulations.payload`，由 `scripts/seed_topic_panel_simulations.postgres.sql` 可重复写入；数据库无启用记录时后端回退 ClickHouse 实时聚合。

## 业务区域契约

三个 Tab 以下均使用各自 UI 图的结构重建，不复用旧通用专题工作台：

- 加密隧道：9 项 KPI、局部影响关系图、分析卡片、128 条事件证据、右侧交付/完整度/报告/专题动作。局部影响关系图必须按用户最新局部参考图呈现“资产组 → 探针 → 高危隧道源 → 隧道协议 → 代理/跳板 → 外部端点”六层链，禁止退回以单个 IP 为中心的放射图；底部固定展示由 API 返回的异常隧道告警、攻击阶段、关联战役三张摘要卡。
- 数据外传：8 项 KPI、源到目的路径关系图、敏感类型和地域/ASN 分布、128 条事件证据、右侧交付/完整度/报告/专题动作。
- APT/战役：9 项 KPI、攻击链关系图、IoC/处置状态、156 条事件证据、右侧交付/完整度/报告/专题动作。
- 三个主关系图必须统一使用 `TopicTopologyGraph` ECharts graph；节点和边由三个专题 API 的当前 payload 构建，实线表示确认关系，虚线表示推断/影响关系，并支持拖拽漫游、滚轮缩放、放大、缩小和自动适配。
- 加密隧道的协议族识别、高危用户、指纹证据是三个真实切换入口，分别激活协议分析、隧道源 TOP5 和指纹证据内容。
- 表格筛选、搜索、分页、布局切换、全屏、报告预览、行级动作均须产生真实 UI 状态；需要持久化或审计的动作必须调用后端。
- 百分比圆环和业务图表使用 ECharts；禁止 CSS/DOM 伪图表。

## 后端与持久化

- `topic_panel_simulations`：保存三个专题的版本化 JSONB 模拟快照；租户专属记录优先于共享租户 `*`。
- 读取响应显式返回 `data_mode=simulated`、`simulation_id`、`simulation_version`，并用请求的 scope/time range 更新运行时上下文。
- 保存视图、分享、收藏、范围编辑、报告导出、证据包导出、订阅、静默、试点周报、报告预览和行级处置使用既有专题治理/动作 API，并写 PostgreSQL 任务、导出、订阅和审计记录。
- 报告预览必须实际调用 `/v1/topics/reports/export`，解析返回制品和快照；行级动作必须返回并持久化 `business_effect`，模拟模式直接进入 `completed`，真实隔离动作进入 `accepted` 执行通道。
- viewer 写动作保持 403；跨租户对象保持不可见。

## 覆盖层

- `modal-topic-save-view`：620px 内在尺寸 Modal。
- `modal-topic-scope-edit`：620px 内在尺寸 Modal。
- `modal-topic-report-export`：620px 内在尺寸 Modal。
- `modal-topic-evidence-package-export`：620px 内在尺寸 Modal。
- `modal-topic-subscription`：620px 内在尺寸 Modal；这是完成三图忠实实现后做的合理性修正，替代 1080px 全高 Drawer。
- `dropdown-topic-share-favorite`：内容自适应 Dropdown/Menu。
- `modal-topic-report-preview`：最大 920px 居中 Modal，内容来自后端报告制品。
- `modal-topic-row-action`：520px 居中 Modal，确认后写业务任务结果和审计；禁止使用右侧 Drawer。

## 验收清单

- [x] 三张参考图和三个生产实现截图均为 1920×1080、DPR 1，并在同一比较输入中检查。
- [x] 标题及三个专题 Tab 保持不变，Tab 以下业务区和右侧摘要全部重建。
- [x] 页面由数据库模拟数据驱动，前端无专题视觉 fixture 常量兜底。
- [x] 三个读取接口、治理写接口、动作接口、PostgreSQL 持久化和审计已连通。
- [x] 筛选、搜索、分页、布局、全屏、预览和按钮覆盖 loading/error/success 状态。
- [x] 1920×1080、1600×900、1366×768 无业务区水平溢出；三专题左右区重叠面积为 0，文字裁切检测为 0。
- [x] Windows Chrome 150 经 Xshell CDP `127.0.0.1:9224 -> 9222` 验收。
- [x] r842 取消使用 r839 的 `0.125` 宽松整页差异门作为完成依据；以用户本轮三张内容区 UI 图逐区复核元数据、KPI、关系画布、分析区、证据表、右侧摘要和真实报告预览。
- [x] r843 按用户最新局部参考图将加密隧道影响面纠正为 API 动态六层链；PostgreSQL 快照包含 20 节点、26 边和 3 张摘要卡，Windows Chrome 三档响应式通过，修正后隧道控件门禁 `58/58`。
- [x] Windows Chrome 最终全控件门禁 `158/158` 通过；最终 actor 的 65 条动作、20 条导出、6 个保存视图、3 个范围、3 个订阅和 106 条审计均可追溯。
- [x] 视觉忠实实现完成后，才执行订阅弹层和长分页的合理性修正。
- [x] r851 保持局部影响面与分析区高度不随高视口变化；资产卡与协议卡的框、图标、标题和明细按统一规则缩放，框始终包围文字。
- [x] r851 证据表固定 10 条/页：空间不足才启用表内纵向滚动，空间足够时关闭内部滚动；窄屏由页面和影响图容器提供滚动。
- [x] r851 隧道源 TOP5 与端点国家分布分别使用 API `tunnelUsers` 与 `destinationDistribution`，五类行级动作返回不同业务结果码，Windows Chrome 专项门禁 `88/88`。

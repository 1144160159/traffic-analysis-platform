# 专题面板三 Tab 业务与报告一致性设计

## 1. 目标与边界

- 路由：`/topics?topic=tunnel|exfil|apt`。
- 页面标题“专题面板”和三个 Tab 是固定外壳，不改变名称、顺序和切换语义。
- 三个 Tab 的数据、图、表、右侧交付区均由专题 API 返回的同一快照驱动；前端不维护第二套业务统计口径。
- 数据外传和 APT/战役按视觉真源逐区复刻：
  - `screens/pages/topics-data-exfiltration.png`
  - `screens/pages/topics-apt-campaign.png`
- 加密隧道保留 r852 已验收结构，只参与共享状态、报告快照和动作契约收敛。

## 2. UI 拆解

### 2.1 共享外壳

从上到下固定为：

1. 标题、三个 Tab、编辑范围、保存视图。
2. 两行专题事实字段。字段从当前 API 快照 `presentation` 和持久化 `scope` 合成，`scope` 优先。
3. KPI 条。隧道 9 项、外传 8 项、APT 9 项。
4. 主业务区。
5. 右侧交付摘要、证据完整度、报告预览、专题动作。

在 1920×1080 时左业务区与右侧交付区并排；窄视口重排、页面滚动和画布局部滚动严格执行第 6 节末尾的响应式裁决，不压缩图节点、文字或图表到互相覆盖。

### 2.2 数据外传专题

主业务区按原图分为：

- 左上“数据外传路径分析”：五列关系图
  `内部源资产 -> 数据类型/文件服务 -> 代理/中转 -> 外部目的地 -> 风险路径`。
  使用共享 `TopicTopologyGraph`；节点为矩形卡片，确认关系为实线，推断/风险关系为虚线，线宽按流量权重分级。支持拖动画布平移、滚轮缩放、放大、缩小、自动适配，节点不可拖动。
- 右上“数据外传分析”：六张卡片按 2 列 3 行排布：
  目的国家/ASN TOP5、敏感数据类型、异常上传会话趋势、外传协议占比、路径置信度、可疑源资产 TOP5。
- 下方“数据外传关联事件与证据”：严格按原图提供“风险：全部”“协议：全部”和“搜索外传事件”三项控件，10 条/页。容器能容纳 10 条时不出现表内滚动；不足时仅表体滚动。
- 右侧：专题交付摘要、6 项证据完整度、报告预览、9 个专题动作。

### 2.3 APT/战役专题

主业务区按原图分为：

- 左上“APT/战役攻击链画布”：分层关系图
  `战役簇 -> 攻击阶段 -> 证据节点/资产账号 -> C2/外联`。
  与另外两个专题共用 `TopicTopologyGraph`，确认关系为实线，推断影响为虚线，支持同一组缩放与自动适配能力。
- 右上“战役分析”：默认 ATT&CK 阶段覆盖矩阵，下方同屏展示战役时间线与关键 IoC TOP5。其余分析维度是同一区域的真实切换状态，不创建新路由。
- 左下“战役关联事件与证据”：严格按原图提供“阶段：全部”“状态：全部”和“搜索战役 / IoC”三项控件，10 条/页；行级按钮在居中确认弹窗完成动作，不使用右侧滑窗。
- 右下“处置动作状态”：完成、进行中、待处置三类数据来自专题 API。
- 右侧：战役交付摘要、6 项证据完整度、报告预览、8 个专题动作。

## 3. 前端状态与数据流

### 3.1 页面状态

| 状态 | 真源 | 更新条件 | 影响范围 |
| --- | --- | --- | --- |
| `selectedTopic` | URL `topic/tab` | 点击三个 Tab | 重新请求当前专题快照并重置专题内局部状态 |
| `scopeRevision` | 范围编辑成功事件 | `PUT /v1/topics/scopes/{topic}` 成功 | 失效当前快照查询 |
| `snapshot` | `GET /v1/topics/{topic}` | Tab、范围或刷新变化 | KPI、图、表、右侧摘要、报告输入 |
| `analysisMode` | 当前 Tab 局部状态 | 点击分析维度 | 只切换分析视图，不重新解释统计口径 |
| `tableFilters/page` | 当前 Tab 局部状态 | 筛选、搜索、分页 | 过滤已返回事件；任一筛选变化后页码回到 1 |
| `selectedGraphNode` | 当前画布局部状态 | 点击图节点 | 高亮节点和相邻边 |
| `graphViewport` | ECharts 实例局部状态 | 缩放、平移、适配 | 保存 `zoom/center/layoutMode`，不改业务快照 |
| `reportSnapshot` | 报告 API | 点击预览 | 预览和后续下载共用同一 `export_id/snapshot_sha256` |

URL 规范：

- `topic` 是规范参数，`tab` 只用于兼容旧链接。
- 两者同时存在且不一致时以 `topic` 为准，并在下一次用户切换时只写一致的 `topic/tab`。
- 非法值回退 `tunnel`，不向 API 发送非法专题名。

### 3.2 数据约束

- 页面适配层必须产出一个规范化 `TopicPageSnapshot`：

```ts
type TopicSnapshotBase<TTopic extends TopicCode, TAnalysis> = {
  topic: TTopic;
  version: { dataMode: 'live' | 'simulated'; simulationId?: string; simulationVersion?: string; updatedAt: number };
  presentation: TopicPresentation;
  summary: Record<string, number>;
  graph: { nodes: TopicTopologyNode[]; links: TopicTopologyLink[] };
  events: SnapshotRow[];
  evidenceBundle: TopicEvidenceItem[];
  analysis: TAnalysis;
};

type TunnelAnalysis = { protocols: TunnelProtocol[]; users: TunnelUser[]; fingerprints: FingerprintEvidence[]; trend: TrendPoint[] };
type ExfilAnalysis = { sources: ExfilSource[]; destinations: ExfilDestination[]; riskTypes: ExfilRiskType[]; paths: ExfilPath[]; trend: TrendPoint[] };
type AptAnalysis = { campaigns: AptCampaign[]; phaseDistribution: AptPhase[]; iocs: AptIoc[]; response: AptResponse };

type TopicPageSnapshot =
  | TopicSnapshotBase<'tunnel', TunnelAnalysis>
  | TopicSnapshotBase<'exfil', ExfilAnalysis>
  | TopicSnapshotBase<'apt', AptAnalysis>;
```

- `TopicVisuals` 保存 `presentation/summary/graph/analysis/version`，`PageSnapshot.rows` 保存同一 API 快照的规范化 `events`；二者必须由一次适配调用原子生成，组件不直接读取原始响应。
- `TopicVisuals.topologyNodes/topologyLinks` 是三张关系图的唯一生产真源。`data_mode=live|simulated` 时任一拓扑数组缺失均显示画布局部错误态和重试，不允许生产环境使用前端推导关系；测试回退必须由显式 `allowTopologyFixture=true` 注入。
- `summary` 只负责 KPI 和交付摘要；列表数量不得反向覆盖 API 已返回的汇总值。
- `events` 只驱动表格，`graph` 只驱动关系图，`analysis` 只驱动分析卡；`paths/campaigns` 仅是各自 `analysis` 内的规范化字段。表格筛选不得改变 KPI、图或分析统计。
- 所有数组在适配器中完成字段归一化，组件不读取 snake_case 原始响应。
- `data_mode=simulated` 时，响应必须同时携带 `simulation_id` 和 `simulation_version`。
- API 已显式返回的数值 `0` 是合法业务值。适配器仅在字段不存在或为 `null` 时回退，禁止用 `||` 覆盖合法 `0`。
- 加载失败时 React Query 以当前 query key 的最近成功值作 `placeholderData`；若不存在旧值则显示整页错误。画布或分析的必填字段缺失只显示对应区域错误，不伪造业务数据。

### 3.3 图状态

- `layoutMode`：隧道 `layered|radial`，外传固定 `layered`，APT 固定 `layered`。
- `zoom/center` 由 ECharts `graphRoam` 事件维护；放大、缩小和适配按钮调用同一图组件控制器。
- 图背景拖动只平移视口；节点按 UI 真源固定分层，不允许用户拖乱生产坐标。
- 点击节点派生 `selectedNodeId/selectedAdjacentEdgeIds`，再次点击同一节点或点击空白取消选中。
- 切换 Tab 时恢复该专题最近一次视口；scope、模拟版本或拓扑版本变化时执行自动适配并清空选中。
- 共享图组件必须在 API 节点尺寸、容器尺寸变化时重新计算标签字号、图标尺寸和边界，框始终包围图标、标题和明细。

### 3.4 APT 分析状态

| 模式 | 输入 | 可见区域 | 空态 |
| --- | --- | --- | --- |
| ATT&CK阶段覆盖 | `phase_distribution/campaigns` | 覆盖矩阵 + 时间线 + IoC TOP5 | 无战役时显示“当前范围无战役” |
| 战役耗时线 | `campaigns.ts_start/ts_end/events` | 时间线主卡 | 无时间字段时显示缺字段错误 |
| 关键 IoC 命中 | `iocs` | IoC TOP5 主卡 | `iocs=[]` 显示“当前范围无 IoC 命中” |
| 横向移动路径 | `topology_links/phase_distribution` | 横向移动链路卡 | 无链路时显示空态，不用阶段计数伪造 |
| 处置动作状态 | `response` | 三态圆环与数量 | 总数为 0 时显示 0，不回退 |
| 证据关联强度 | `evidence_bundle` | 六项完整度与关联强度 | 缺字段显示局部错误 |

切换分析模式只改变分析区域，保留图视口、表格筛选和页码。

### 3.5 交互状态机

读操作：`idle -> loading -> ready | error`。错误保留上一次成功快照并允许重试。

写操作：`idle -> confirming -> submitting -> completed | failed`。

- `completed` 必须显示后端返回的 `business_effect`，并写审计。
- `failed` 不允许提前更新成功态。
- 范围、保存视图、订阅、静默、分享、收藏、导出和行级动作都按该状态机处理。
- 行级动作统一使用居中 Modal；动作完成后仅在业务需要时刷新当前专题快照。
- 外传 Ant Table 使用受控 `current/pageSize`；风险、协议或关键字变化均将 `current=1`。
- 外传和 APT 表格都以 `ResizeObserver` 判断可用表体高度：可容纳表头、10 行和分页时 `overflow-y: visible`，不足时只给表体设置 `max-height/overflow-y:auto`。

### 3.6 右侧动作矩阵

| UI 动作 | action code / API | 确认 | 成功后置条件 |
| --- | --- | --- | --- |
| 编辑范围 | `PUT /v1/topics/scopes/{topic}` | Modal | scopeRevision+1，报告快照失效 |
| 保存视图 | `POST /v1/topics/views` | Modal | 保存 view_id，不刷新业务快照 |
| 导出总报告/战役报告 | `POST /v1/topics/reports/export` | Modal | 生成或复用报告快照并下载 |
| 导出证据包 | `POST /v1/topics/evidence-packages/export` | Modal | 返回 ZIP 制品与审计 ID |
| 试点周报导出 | `POST /v1/topics/reports/export` | Modal | 格式 PDF，复用当前报告快照 |
| 订阅 | `POST /v1/topics/subscriptions` | Modal | 展示 subscription_id |
| 静默 | `POST /v1/topics/{topic}/actions`，`mute` | Modal | 展示 business_effect，不刷新快照 |
| 分享 | `PATCH /v1/topics/views/{id}`，`shared` | Dropdown | 当前视图标记 shared |
| 收藏 | `PATCH /v1/topics/views/{id}`，`favorite` | Dropdown/按钮 | 当前视图标记 favorite |
| 行级 PCAP/Session/证书/回溯/审计 | `POST /v1/topics/{topic}/actions` | 520px Modal | 展示差异化 business_effect；需改变业务聚合时才刷新 |

权限由后端逐接口校验；viewer 写动作返回 403。前端不以隐藏按钮代替权限校验。

## 4. 后端与数据库逻辑

### 4.1 读取

`GET /v1/topics/{topic}`：

1. 校验租户、scope 和时间窗。
2. 优先选择租户专属启用模拟快照，其次选择共享租户 `*` 的启用快照。
3. 无模拟快照时执行实时聚合。
4. 返回统一结构：
   `presentation/summary/topology_nodes/topology_links/events/evidence_bundle`，
   外传附加 `top_sources/destinations/risk_types/paths/trend`，
   APT 附加 `campaigns/phase_distribution/iocs/response`。
5. 同一响应中的 KPI、图、表、证据完整度必须来自同一个快照版本。

### 4.2 写动作

- 范围：`topic_scopes`，更新成功后影响下一次专题快照。
- 保存视图、分享、收藏：`topic_saved_views`。
- 订阅：`topic_subscriptions`。
- 行级及专题动作：`topic_actions`，保存 `action/target/status/detail/business_effect`。
- 每个写动作与导出都在同一事务中写业务记录和审计记录；审计失败则业务操作不报告成功。

### 4.3 报告快照一致性

预览与下载不得分别重新查询专题数据。报告流程为：

1. `POST /v1/topics/reports/export`，格式 `json`，后端在事务内加载一次专题快照。
2. 后端规范化报告模型，计算 `snapshot_sha256`，生成 `export_id`，并将规范化报告模型与快照哈希保存在 `topic_exports.result`。
3. 前端预览只渲染该规范化报告模型，并缓存 `export_id/snapshot_sha256`。
4. 预览弹窗中的 PDF、DOCX、JSON 下载必须携带 `source_export_id`。
5. 后端收到 `source_export_id` 后读取原报告快照并生成目标格式，禁止再次执行专题查询；返回的 `snapshot_sha256` 必须与预览一致。
6. 右侧“导出总报告”若当前存在有效预览快照则复用它；不存在时先创建一次报告快照，再生成下载制品。

报告快照由 `TopicReportSnapshotProvider` 在当前专题页内共享，不能封装在单个预览按钮内部。以下任一条件变化时失效：`topic`、`scope.updated_at`、`simulation_version`、业务快照 `updated_at`、用户主动刷新；导出记录超过后端保留期时也必须重新生成。单纯切换分析模式、表格筛选、页码或图视口不使报告快照失效，因为这些只改变展示状态，不改变报告业务快照。

`topic_exports.result` 至少包含：

```json
{
  "filename": "apt-report-....pdf",
  "content_type": "application/pdf",
  "content_base64": "...",
  "sha256": "sha256:...",
  "snapshot_sha256": "sha256:...",
  "report_model": {
    "title": "...",
    "topic_id": "...",
    "time_range": "...",
    "scope": "...",
    "summary": {},
    "findings": [],
    "evidence_sections": []
  },
  "source_export_id": "..."
}
```

## 5. 数据库模拟数据

- `topic-exfil-ui-v2`：补齐五列 `topology_nodes/topology_links`、8 项 KPI、128 条事件、6 项证据、目的地/风险类型/趋势。
- `topic-apt-ui-v2`：补齐分层 `topology_nodes/topology_links`、9 项 KPI、156 条事件、战役/阶段/IoC/处置/证据。
- 模拟数据使用固定 2026-06-20 时间窗，保证 UI 图、报告预览和下载可重复比对。
- 更新 seed 后必须重新执行数据库脚本，并通过 API 证明当前启用版本。

## 6. 验收门

1. 前端状态与交互设计审核通过。
2. 数据外传、APT 前端实现通过构建、单测、Lint。
3. 后端/数据库/报告设计审核通过。
4. Go 测试证明快照复用、哈希一致、跨租户隔离和审计原子性。
5. K8s 发布 Web UI、alert-service、数据库 seed。
6. Xshell 隧道下 Windows Chrome 1920×1080 逐区截图，与两张视觉真源同尺寸并排比较。
7. 执行筛选、分析切换、图缩放、10 条分页、行级动作、报告预览及三种下载；保存网络、控制台、数据库和哈希证据。
8. 页面逻辑审核、布局审核、主线程裁决；问题回修后重拍并复核。

响应式裁决：延续三 Tab 已验收规则。`>=1800px` 左业务区与右栏同屏；`<1800px` 主业务区保持设计最小宽度，右栏下沉为下一行；`<1440px` 业务画布和分析区不压缩节点与字号，页面负责纵向滚动，画布容器在确有需要时提供局部横向滚动。整页不得出现元素重叠或文字被图覆盖。

# 专题面板开发流程与最终验收结论

更新时间：2026-07-28

## 会话 019f7330-4db3-7062-8a8e-8d2a9a09c5b0 的实际开发流程

该会话的专题面板工作遵循以下证据闭环，而不是只做前端照图：

1. 阅读 `agent.md`、`doc/`、上一轮会话和验收证据，确定 K8s-first、Data-backed、Evidence-gated 约束。
2. 锁定三张 UI 图，保留标题“专题面板”和三个 Tab，拆解并重建 Tab 以下业务区及右侧摘要。
3. 前端实现视觉与交互，后端补齐读取、导出、治理与业务动作契约，PostgreSQL 提供版本化模拟快照和持久化/审计。
4. 构建镜像并导入两个 K8s 节点，经 APISIX 发布生产态。
5. 使用用户指定的 Windows Chrome，经 Xshell `9224 -> 9222` 做真实截图、全部按钮、API、响应式和运行时错误检查。
6. 将参考图与实现截图放入同一比较输入，量化差异；逻辑代理、布局代理分别审查，主线程裁决并回修。
7. 只有源码、测试、数据库、部署、浏览器、视觉和独立复审同时通过后，才同步契约/状态/运维/验收文档。

## r840/r843 最终实现

- 数据库：`topic_panel_simulations` 保存 tunnel 128、exfil 128、apt 156 条事件以及 KPI、关系图、证据包和报告摘要；无启用快照时后端回退 ClickHouse。
- 统一关系图：加密隧道局部影响面、数据外传路径和 APT 攻击链全部使用 `TopicTopologyGraph` ECharts graph；节点/边由专题 GET API 返回数据构建，支持实线确认关系、虚线推断/影响关系、滚轮漫游、放大、缩小和自动适配。
- 加密隧道局部影响面：r843 按用户最新局部参考图纠正为六层业务链，依次为资产组、探针、高危隧道源、隧道协议、代理/跳板和外部端点；PostgreSQL 当前快照返回 20 个节点、26 条连线和 3 张底部摘要卡，前端不再使用 IP 中心放射图。
- 自适应：1920 保留参考图左右结构；1600/1366 将专题主区和右栏纵向重排，右栏使用两列；图容器有终端级联最小高度，避免后置通用 CSS 再次压缩节点文字。
- 隧道切换：协议族识别、高危用户、指纹证据分别切换到协议分析、隧道源 TOP5 和 API 指纹证据内容。
- 报告预览：`预览报告` 调用 `POST /v1/topics/reports/export` 生成 JSON 报告制品，解析后端摘要、快照、关键指标、关键发现和证据范围，在居中 Modal 展示；每次预览均写 `topic_exports` 与对应 `TOPIC_REPORT_EXPORTED` 审计。
- 列表动作：不再使用右侧滑窗；所有行级操作使用居中确认 Modal。后端根据动作生成 `business_effect`、`completed/accepted` 状态、目标状态、下一路由或证据引用，并写 `topic_actions` 和审计。
- 导出制品：后端在 PostgreSQL 模拟快照不可用时回退 ClickHouse 实时聚合；PDF/DOCX/ZIP 均包含专题指标与数据快照，ZIP 内 manifest 和 snapshot 为不同内容并附 SHA-256。
- 后置合理性修正：先完成三图忠实实现，再将全高订阅 Drawer、行级 Drawer 和长分页改为边界明确的 Modal/紧凑分页；响应式在窄屏采用滚动的纵向信息流。

## 最终证据

- 构建/测试：专题相关 TypeScript 定向 ESLint 通过；`npm run build` 通过；page API plan/snapshot adapter `50/50`；Go API 包和 `topicActionBusinessEffect` 直接单测通过。全仓 ESLint 仍有其他脏工作树页面的既有问题，本轮未越界修改。
- Windows Chrome：`topics-live-windows-cdp-topic-panel-r843.json` 在 1920×1080、1600×900、1366×768 三档通过；三专题 `left_rail_overlap_area=0`、`clipped_rail_text=[]`、无页面级横向溢出、每页 API 动态关系图计数为 1。r843 的加密隧道主截图为 `implementation-topic-panel-r843-live.png`。
- 全量控件：加密隧道局部影响面修正后由 `topics-all-controls-topic-panel-r843-tunnel-corrected.json` 再次验证 `58/58`，与 r842 的 `exfil+apt=100/100` 合计 `158/158`；应用 4xx/5xx、请求失败、console error、page error 均为 0；缩放状态实测 `1 -> 1.2 -> 1`，自动适配回到 1。
- 弹窗实图：三个 `report-preview-topic-panel-r842-*.png` 和 `list-action-modal-topic-panel-r842-*.png` 证明报告预览为真实中文报告文档、列表动作均为居中 Modal。
- PostgreSQL：r842 两个分段验收 actor `9757259a-326f-44ff-a9d4-da4eef5b19e4`、`a51bcac1-b616-4b06-a8ee-b9a8060ab75b` 合计写入 65 条动作、20 条导出、6 个保存视图、3 个范围、3 个订阅和 106 条审计；65 条动作均含 `business_effect` 与模拟来源，三个专题报告预览均有制品和一一对应的导出审计。详见 `topics-all-controls-topic-panel-r842-db-audit.json`。
- 视觉复检：不再沿用 r839 的 `0.125` 宽松整页阈值作为通过依据；r843 以用户最新局部参考图逐层复核加密隧道六层链、实/虚线、节点标签和底部三摘要卡，并对 1600/1366 单独检查裁切与重叠。
- 独立复审：r843 由逻辑代理与布局代理针对最新加密隧道局部影响面重新复核；复审结论归档于本轮验收记录。
- 部署：alert-service `topic-panel-r840` generation/observedGeneration=`260/260`、Web UI `topic-panel-r843` 为 `1058/1058`，均 Ready `1/1`。Web OCI manifest `sha256:5a165d7dc5e048fb32d3a841dd529692481904e2ff47e5e89792e84bbbd4972e`、image ID `sha256:a7dcdebb70ef5f1f34a7ffa9a51eb98faf3c79a94ce2ef421df99d14057338f3` 与运行清单一致。

## 结论

专题面板已完成数据库模拟、API 动态三图、响应式回修、隧道三项切换、真实报告预览、居中列表业务操作、PostgreSQL/audit 闭环和 Windows Chrome 生产验收。加密隧道局部影响面最终以 r843 六层链为准。主证据为 `topics-live-windows-cdp-topic-panel-r843.json`、`topics-all-controls-topic-panel-r843-tunnel-corrected.json`、`topics-all-controls-topic-panel-r842-exfil-apt.json`、`topics-all-controls-topic-panel-r842-db-audit.json`、r843 三专题 1920 主截图和响应式截图、r842 三份报告预览截图。

## r850/r851 加密隧道整页最终回修

- 中部空间：1920×1080 与 1920×1300 的局部影响面/加密隧道分析业务板高度均为 381px，视口增高只扩展证据表，不压缩或拉伸中部业务区。
- 图节点：资产组和隧道协议节点使用同一缩放系数；缩放同时作用于框、Ant Design 图标、标题和明细，节点背景与边框完整包围文字。关系图仍由 API 返回 20 节点、26 条实/虚边，支持缩放、漫游、适配、布局和全屏。
- 表格：固定 10 条/页。1920×1080 可用高度 134px、内容 270px，内部滚动开启；1920×1300 可用高度 354px、内容 354px，内部滚动关闭。1600×900 与 1366×768 使用页面纵向滚动，未压缩业务画布。
- 分析切换：协议分析、隧道源 TOP5、端点国家分布均使用专题 API 数据；隧道源按 `totalBytes/count` 排序，国家分布使用 `destinationDistribution`，并保留 ASN 明细。
- 业务动作：PCAP、Session、证书、回溯路径、审计日志分别返回 `extract_pcap`、`inspect_session`、`inspect_certificate`、`trace_path`、`write_audit`，不再共用泛化处理。报告预览实际调用导出 API。
- Windows Chrome：最终全控件 `88/88`；三专题运行态和 1920/1600/1366/1300 响应式门禁通过，无应用 4xx/5xx、请求失败、console error 或 page error。证据为 `topics-all-controls-topic-panel-r851-final3-tunnel.json` 与 `topics-live-windows-cdp-topic-panel-r851-final2.json`。
- 视觉诊断：最终同帧比较 `comparison-topic-panel-r851-final2.png`；容差 90 下全帧 mismatch `0.0965827546`、业务区 `0.1023004622`。该指标用于定位可见差异，不替代逐区人工复核。
- 生产态：alert-service `topic-panel-r850` generation=`261/261`、Web UI `topic-panel-r851` generation=`1065/1065`，均 Ready `1/1`。

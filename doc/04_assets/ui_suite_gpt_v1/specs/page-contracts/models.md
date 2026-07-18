# 模型管理 前端实现契约

## 基本信息

- ID：`models`
- 路由：`/models`
- 领域：`detection-ops`
- React 页面：`ModelManagementPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/models.png`
- API：`/api/v1/models`

## 必须实现的业务层

- 模型版本
- 特征解释
- 异常解释
- 样本示例
- 激活回滚

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

- `drawer-model-detail`：模型详情，Drawer

## 验收清单

- [x] 最终 PNG 必须为 1920x1080
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色必须遵守 success/info/warning/danger/critical token
- [x] 危险动作必须具备影响范围、权限提示和审计留痕
- [x] 公共 AppShell 必须与 screen.png 目标参数一致
- [x] 页面主工作区不得复用相邻页面的业务组件组合
- [x] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [x] React Query 必须覆盖 loading/error/empty 状态

## 2026-07-18 续作进度

- 已移除 `SIM-MODEL-*` 补造行和 sessionStorage 仿真任务；列表、总数、版本、线上版本、回滚版本、指标和工作台均经真实 API/PostgreSQL 返回。
- 工作台使用明确标注的 `acceptance-bootstrap-v2` 验收数据（5 个模型、5 个变体、210 行），包含 lineage 和 observed metric；它不冒充生产训练真值。模型注册、版本、动作和审计使用实时 PostgreSQL 记录。
- `model_action_jobs` 由常驻 worker 原子 claim；上下文动作本地完成，反馈、重训、评估通过独立 `model-actions.v1` Kafka topic 取得 broker ack 后完成，回滚执行真实版本状态迁移。请求与完成/分发审计均可查询。
- 注册表激活在服务端事务内锁定并检查持久化 review gate；零门禁或任一未通过均 fail-closed。该接口只接受 100% 全量切换，5%～99% 分阶段流量明确交由部署编排，避免伪灰度。
- 回滚使用独立事务把当前 active 版本停用并恢复指定 deprecated 版本，同时原子写入 applied 审计和 `model_update_outbox`。Outbox 以持久化稳定 `event_id` 至少一次投递；broker ack 后在同一数据库事务内标记 outbox published、job completed 并写 `MODEL_VERSION_ROLLBACK_COMPLETED`，关闭崩溃和双写窗口。
- Flink Behavior Job 必须把 `rollback-activated` 作为激活类热更新处理，保存 `event_id` 并抑制同一事件重放。当前 Job `f7b85d363aa31f7d1e8721ae05f6bcc6` 从 canonical savepoint 恢复，12/12 task RUNNING；四个广播消费者完成真实 MinIO 下载、SHA 校验、XGBoost load/warmup 与 tenant+model 原子切换后分别回传 applied ACK。
- Kubernetes `postgres-init-sql` 的 `03-models-deploy.sql` 必须同步 `model_update_outbox` 表与四个工作索引；r325 在隔离临时数据库完整执行 7 份 ConfigMap SQL 后证明表、18 列、4 个索引和状态约束存在，并清理临时库。
- 反馈、重训、评估请求均校验数据集/版本/策略；拒绝内联原始样本，重复 queued/running 重训通过 advisory lock 串行化并返回 409。模型更新与版本注册均以租户和 URL path 为权威。
- 前端只允许激活候选版本和回滚已停用版本；没有真实制品/特征集选择表单时禁用导入，避免提交占位业务参数。
- Windows Chrome r328：精确 CSS/PNG 1920×1080、DPR≈1；5 条 API 数据对应 5 条 UI 行，激活门禁 409、跨租户更新 403 且数据不变，3/3 动作均从 202 到 `completed` 并闭合数据库请求/完成或分发审计。
- 隔离状态机 r328 的 11/11 检查通过：真实 rollback artifact 只有在 4/4 同 URI/SHA 的 applied ACK 后完成。控制面以服务端事件快照固定期望并行度并显式校验 `COUNT(DISTINCT)=N/MIN=0/MAX=N-1`；伪造 1/1、错误 URI/SHA 或缺失 action 指纹均 fail-closed。
- 生产部署使用 digest pin：Rule Manager r11 `sha256:ff2f2e...793ca9`、Web UI r12 `sha256:93bb26...2209`、Flink XGBoost runtime `sha256:8c9b09...771f9`；Pod imageID、双节点 manifest、OCI label/source digest、Flink JAR 和完整关键源码集合见 `build-provenance-r328.json`。
- 五个生产状态 raw diff 全部通过：主页面 `0.103220 <= 0.125`，其余四个 focus 状态分别为 `0.079793/0.108913/0.089881/0.106493 <= 0.15`。正式裁决见 `doc/02_acceptance/02-regression/model-management-visual-adjudication-latest.json`，原始图与指标保存在 `evidence/ui-image-breakdowns/pages/models/visual-r328/`。
- 主工作区 `scrollHeight=clientHeight=917`，右侧 rail 与最近操作记录均完全位于固定底栏上方；“规则融合瀑布图”已按真实图形语义更名为“规则正负贡献图”。
- 规则正负贡献均可见，TP/FP 与通过/待审颜色具有机器断言；应用自身 0 console error、0 runtime exception、0 request failure。
- 当前状态为 `review_pending`：等待逻辑、布局和综合三路最终复审；所有 P0/P1 关闭后才允许主线程接受。

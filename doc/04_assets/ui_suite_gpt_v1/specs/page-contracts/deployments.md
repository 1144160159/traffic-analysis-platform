# 部署管理 前端实现契约

## 基本信息

- ID：`deployments`
- 路由：`/deployments`
- 领域：`detection-ops`
- React 页面：`DeploymentManagementPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.png`
- API：`/api/v1/deployments`

## 必须实现的业务层

- 发布计划
- 灰度状态
- 回滚窗口
- 运行健康
- 审计记录

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

- `modal-deployment-create`：新建部署，Modal
- `modal-deployment-rollback`：回滚部署确认，Modal

## 验收清单

- [x] 最终 PNG 必须为 1920x1080
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色必须遵守 success/info/warning/danger/critical token
- [x] 危险动作必须具备影响范围、权限提示和审计留痕
- [x] 公共 AppShell 必须与 screen.png 目标参数一致
- [x] 页面主工作区不得复用相邻页面的业务组件组合
- [x] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [x] React Query 必须覆盖 loading/error/empty 状态

## 2026-07-18 最终实机验收证据

- Xshell Windows Chrome CDP 全流程：`evidence/ui-image-breakdowns/pages/deployments/full-stack-r264.json`（43/43，Chrome 150，精确 1920×1080，0 bad response / console error / page error）。
- 部署状态机、并发、失败审计与配额回滚：`doc/02_acceptance/02-regression/deployment-state-machine-latest.json`（70/70，包含 quota rollback 429 的失败审计）。
- Kafka/outbox 实际消费：`doc/02_acceptance/02-regression/deployment-outbox-kafka-latest.json`（4/4；dead predecessor 不阻塞、过期 lease 恢复、schema v1、稳定 event_id）。
- 主页面视觉指标：`evidence/ui-image-breakdowns/pages/deployments/visual/main-metrics-r20.json`（`0.110578 <= 0.125`，`effective_status=pass`）。
- 创建部署弹窗原始视觉指标：`evidence/ui-image-breakdowns/pages/deployments/visual/create-metrics-r20.json`（`0.105156 > 0.08`，raw=`fail`，effective=`pass_with_contract_exception`）。
- 回滚确认弹窗原始视觉指标：`evidence/ui-image-breakdowns/pages/deployments/visual/rollback-metrics-r20.json`（`0.100451 > 0.08`，raw=`fail`，effective=`pass_with_contract_exception`）。
- 弹层裁决：`doc/04_assets/ui_suite_gpt_v1/specs/overlay-contracts/deployment-modal-size-adjudication.json`。旧近全屏参考图与 `agent.md:115` 冲突；r264 实测创建/回滚 Modal 均约 `959.99×759.99`，满足桌面端 `<=960×760` 且保留业务上下文。原始失败值保留，只有已登记且几何满足约束的目标可获得契约例外；未裁决失败会使 comparator 非零退出。
- 同屏比对：`evidence/ui-image-breakdowns/pages/deployments/visual/main-side-by-side-r20.png`、`create-side-by-side-r20.png`、`rollback-side-by-side-r20.png`。
- 运行镜像：`docker.io/traffic/web-ui:deployment-management-20260718-r21`、`docker.io/traffic/rule-manager:deployment-management-20260718-r15`；两 Pod 1/1 Ready、0 restart，回滚 dry-run 通过。
- 终审裁决：逻辑、布局、综合三路复审无 P0/P1；主线程接受本页，并把两个审计/正向用例增强项记录为非阻断后续。

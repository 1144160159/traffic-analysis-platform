# 前端准确实现 UI 图的 5 种方法

本项目推荐把 5 种方法组合使用：前三种指导开发，第四种做自动验收，第五种处理业务语义和专家意见。

## 1. 分层 JSON 契约法

- 优先级：主方法
- 输入：`layers/<id>.json`、`tokens.json`、`app-shell.json`、`目标 PNG`
- 输出：页面区域坐标、组件角色、验收 checklist
- 风险：需要把 bbox 转成响应式 CSS 约束，不能只写固定像素。

## 2. 组件库优先法

- 优先级：主方法
- 输入：`component-map.json`、`48 张 component 图`、`现有 Ant Design/ECharts 封装`
- 输出：可复用 WorkPanel/MetricTile/StatusTag/Chart/Table/Form 模块
- 风险：组件抽象过早会拖慢页面，必须从真实页面复用点反推。

## 3. 路由与 API 契约法

- 优先级：主方法
- 输入：`route-page-map.json`、`web/ui/src/routes/routeManifest.tsx`、`web/ui/src/services/*.ts`
- 输出：页面数据 hook、loading/error/empty 状态、权限 requiredScopes
- 风险：不能在页面中直接 fetch，也不能用 mock 掩盖真实 API 缺口。

## 4. 截图回归法

- 优先级：验收方法
- 输入：`visual-acceptance.json`、`目标 PNG`、`Playwright 1920x1080 截图`
- 输出：公共区像素级差异、主工作区结构差异、交互状态截图证据
- 风险：截图差异只能证明视觉接近，业务动作仍需 API 和审计验证。

## 5. 人工标注评审法

- 优先级：补充方法
- 输入：`BUSINESS_FLOW_ACCEPTANCE.md`、`页面 PR 截图`、`产品/安全专家评审意见`
- 输出：业务语义修正、缺失动作或危险动作提示、专家验收记录
- 风险：不能替代自动校验，评审意见必须回写到契约或代码任务。


## 推荐组合

- 新页面：分层 JSON 契约法 + 路由与 API 契约法。
- 公共控件：组件库优先法 + 截图回归法。
- 关键业务闭环：路由与 API 契约法 + 人工标注评审法。
- 视觉争议：截图回归法作为证据，人工评审决定是否修改契约。

# T1-M09-N021 页面状态与可访问性

## 当前结论

实现状态为 `PARTIAL`。新增共享 `PageStateBoundary` 和纯状态解析器，明确区分 `loading / empty / partial / unavailable / conflict / final`；告警详情根页面与战役详情 Drawer 已接入。新增 Drawer 焦点生命周期 hook，在打开后聚焦显式标题，关闭后把焦点还给原触发元素。

最新 Kubernetes run 为 `c2c6fa96-b4dc-4b9e-9b0c-9cf3da7ed37d`，证据位于 `doc/02_acceptance/topic1/tasks/t1-m09-n021/k8s-page-state-accessibility-latest.json`。两个 Job 在节点 `8-2tb` 上检查统一不可变 Web 镜像 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2`：一条验证六态语义、Drawer focus 合同和无 mock worker，另一条验证长文本及 1366/1600 CSS 合同。Docker image ID 为 `sha256:6ec59641d1b271198c4a842218c79ff4c575d7c3f191375a4278d58dd0496b13`，run-scoped Job/Pod 已删除。

## 状态决策

1. 首次请求且没有数据时是 `loading`，页面不会先渲染空快照中的占位业务事实。
2. 没有数据且 HTTP 409 时是 `conflict`；其他错误为 `unavailable`。两者使用 `role=alert`，提供“刷新权威状态”，并禁止渲染 children 作为回退内容。
3. 请求完成、没有数据且没有错误时才是 `empty`。
4. 已有权威数据但缺少 section 或后台刷新失败时是 `partial`，使用 `role=status`，保留已有内容并明确列出缺失项。
5. 仅在有完整权威数据且无错误时是 `final`。

## 已接入路径

告警详情根视图用真实 query data 决定状态；首次加载和 API 失败不再继续显示 `emptySnapshot`。证据或反馈子接口缺失时，页面根状态为 partial，现有真实摘要继续显示。

战役详情 Drawer 使用同一状态解析器，`snapshot.partial` 与 `missingSections` 直接进入 partial。打开 Drawer 前记录当前 trigger，open transition 完成后把焦点移动到 `data-campaign-detail-initial-focus` 标题，关闭后仅在原 trigger 仍连接 DOM 时恢复焦点。

## 响应式与回滚

共享状态容器统一 `min-width:0`、`max-width:100%` 和 `overflow-wrap:anywhere`。1366 视口降低空态最小高度与 padding，1600 视口增加空间；长错误、缺失 section 和 trace 文本不会强制撑宽页面。

回滚时可先从单个路由移除 `PageStateBoundary` 接入，再删除公共组件导入；不要删除真实接口错误或 partial 元数据。焦点 hook 回滚不能用“关闭后聚焦 body”替代，必须恢复到路由原有、可验证的触发控件。

## 未关闭项

其余 M09 路由尚未全部迁移；同一候选的 Windows Chrome 键盘、焦点、ARIA、屏幕阅读器及 1366/1600 像素对比尚未执行。因此 K8s bundle PASS 不能宣告 N021、M09 或项目完成。

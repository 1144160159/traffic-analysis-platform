# UI 契约回归预检报告

- Run ID：`20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current`
- 结果：`blocked`
- APISIX：`http://10.0.5.8:30180`
- 检查数：20/21 passed，blockers=1，warnings=0

## Blockers

| 阶段 | 检查 | 等级 | 状态 | 证据 |
| --- | --- | --- | --- | --- |
| browser | Desktop Chrome wrapper opened protected business page | blocker | redirected_to_login | - |

## Warnings

- 无

## 证据

- NDJSON：`doc/02_acceptance/runs/20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current/live-ui-contract-preflight-20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current.ndjson`
- Summary：`doc/02_acceptance/runs/20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current/live-ui-contract-preflight-20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current-summary.json`
- UI contract matrix：`doc/02_acceptance/runs/20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current/ui-contract-matrix.json`

## 口径

本报告证明设计菜单、UI 图契约、routeManifest、前端权限单测、APISIX 登录入口和 Desktop Chrome 合法登录态业务页点击是否对齐。Desktop Chrome 是 MCP 桥接工具，shell 脚本不直接操作浏览器；本轮结果通过 `DESKTOP_CHROME_STATUS` / `DESKTOP_CHROME_BUSINESS_STATUS` 等变量记录 wrapper 实测结论。

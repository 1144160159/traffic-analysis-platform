# Desktop Chrome 业务页点击最新证据

- 最新 Run：`20260629-desktop-chrome-business-smoke-r4`
- 结果：`pass`
- 最终 URL：`http://10.0.5.8:30180/alerts`
- 验证点：`告警`、`处置`、`反馈`、`实时通道`、`数据质量` 可见；无登录页、无 403、无 smoke token 残留。
- 关联 UI 契约预检：`20260629-ui-contract-preflight-r5`，21/21 passed。
- 关联发布基线：`20260629-release-manifest-r10`，12/12 passed，Web UI 镜像为 `traffic/web-ui:desktop-smoke-token-20260629-r3`。

原始证据见 `../runs/20260629-desktop-chrome-business-smoke/desktop-chrome-business-smoke-r4.json`。

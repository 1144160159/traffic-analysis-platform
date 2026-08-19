# Desktop Chrome 业务页点击烟测

- Run ID：`20260629-desktop-chrome-business-smoke-r4`
- 结果：`pass`
- 浏览器后端：Codex Desktop Chrome extension backend
- 目标入口：`http://10.0.5.8:30180/dashboard#codex_smoke_token=<redacted>`
- 最终业务页：`http://10.0.5.8:30180/alerts`
- Web UI 镜像：`traffic/web-ui:desktop-smoke-token-20260629-r3`
- 运行后状态：`DESKTOP_SMOKE_TOKEN_ENABLED=false`

## 证据

- `desktop-chrome-business-smoke-r4.json`：claim 已打开的 `/alerts` tab 后验证业务页 DOM 信号。
- `proxy-output-r4.jsonl`：前序动作打开 `/dashboard` 并点击 `/alerts`，代理在后续大文本读取阶段超时；随后 tab list 证明最新页已到 `/alerts`。
- `proxy-output-r4-claim.jsonl`：重新 claim `/alerts` tab，验证 `告警`、`处置`、`反馈`、`实时通道`、`数据质量` 可见，且没有 `账号密码登录`、`权限不足`、`codex_smoke_token`。

## 口径

本轮不提交登录表单。短期 JWT 只用于受控 smoke hash，证据文件不包含明文 token。`DESKTOP_SMOKE_TOKEN_ENABLED` 是默认关闭配置，测试完成后已恢复为 `false`。

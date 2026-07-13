# 前端实现差异清单

本文件只指出 UI 图契约与当前前端 token 的可见差异，不自动修改前端代码。

| 参数 | UI 图目标 | 当前代码 | 状态 |
|---|---:|---:|---|
| shellTopbar | `80px` | `80px` | 一致 |
| shellSidebar | `166px` | `166px` | 一致 |
| shellBottombar | `83px` | `83px` | 一致 |

## 处理原则

- 若实现目标是复刻 UI 图，以上 `需对齐` 项必须优先修正。
- 修正后运行 `npm run build` 和 Playwright 截图回归。
- 如产品确认以前端现状为准，必须反向更新 UI suite 的 AppShell 目标参数和所有验收文档。

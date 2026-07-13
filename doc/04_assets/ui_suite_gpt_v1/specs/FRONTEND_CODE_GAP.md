# 前端代码差距报告

本报告把 UI 契约与当前 `web/ui` 静态代码对齐，输出可执行的修复队列。它不替代浏览器截图和真实 API 验收，但能先挡住明显的开发偏差。

## 总览

| 类别 | 结果 |
| --- | ---: |
| 页面文件覆盖 | 28/28 |
| 路由/懒加载覆盖 | 28/28 |
| 契约 API 在 pageApiPlans 覆盖 | 46/46 |
| 直接 fetch 违规 | 0 |
| AppShell token 缺口 | 0 |
| 浮层 confirmed/partial/missing | 70/0/0 |

## AppShell 公共参数

- 已与 UI 契约一致。

## 页面代码覆盖

| 页面 | 文件 | 数据接入 | 状态覆盖 | 危险动作保护 | 备注 |
| --- | --- | --- | --- | --- | --- |
| `login` | 有 | 通过 | 登录专用 | 无危险动作 | 登录页使用 login/fetchCaptcha，不走 page snapshot。 |
| `screen` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `dashboard` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `alerts` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `alert-detail` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `campaigns` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `campaign-detail` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `attack-chains` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `encrypted-traffic` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `forensics` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `assets` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `graph` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `fusion` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `baselines` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `probes` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `rules` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `deployments` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `models` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `mlops` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `data-quality` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `playbooks` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `whitelist` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `compliance` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `audit-log` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `notifications` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `settings` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `not-found` | 有 | 通过 | 404 专用 | 无危险动作 | 404 页不需要业务 API。 |
| `topics` | 有 | 通过 | loading/error/empty | 无危险动作 |  |

## 浮层静态追踪

| 状态 | 数量 | 说明 |
| --- | ---: | --- |
| confirmed | 70 | 同时发现语义线索和目标容器组件。 |
| partial | 0 | 只发现语义线索或容器组件，需人工核查。 |
| missing | 0 | 未发现可追踪实现，需按契约补齐。 |

## 优先修复队列 Top 20

当前无静态缺口。

## 使用方式

```bash
node doc/04_assets/ui_suite_gpt_v1/build_frontend_contracts.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_handoff.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_code_gap.mjs
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
```

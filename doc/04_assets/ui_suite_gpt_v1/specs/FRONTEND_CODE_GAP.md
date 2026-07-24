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
| 浮层 confirmed/partial/missing | 65/5/0 |

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
| `assets` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `graph` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `fusion` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `baselines` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `probes` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `rules` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `deployments` | 有 | 需修正 | 不足 | 无危险动作 |  |
| `models` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `mlops` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `data-quality` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `playbooks` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `whitelist` | 有 | 通过 | loading/error/empty | 需人工验真 |  |
| `compliance` | 有 | 通过 | loading/error/empty | 需人工验真 | r367/r368 已通过 dedicated scope、真实导出、整改幂等、不可变固化及 Windows Chrome 4/4 视觉/交互门禁；见 `doc/02_acceptance/02-regression/compliance-development-progress-latest.json`。 |
| `audit-log` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `notifications` | 有 | 通过 | loading/error/empty | 无危险动作 |  |
| `settings` | 有 | 通过 | loading/error/empty | Popconfirm + 权限禁用 + 事务审计 | r510 已接受：88/88 API/DB、20/20 Windows Chrome、5 个区域内 Drawer、视觉 0.07115355；`admin:read/admin:write/token:write` 契约已与后端一致。 |
| `not-found` | 有 | 通过 | 404 专用 | 无危险动作 | 404 页不需要业务 API。 |
| `topics` | 有 | 通过 | loading/error/empty | 无危险动作 |  |

## 浮层静态追踪

| 状态 | 数量 | 说明 |
| --- | ---: | --- |
| confirmed | 65 | 同时发现语义线索和目标容器组件。 |
| partial | 5 | 只发现语义线索或容器组件，需人工核查。 |
| missing | 0 | 未发现可追踪实现，需按契约补齐。 |

## 优先修复队列 Top 20

1. `P1` Page deployments：页面未能静态证明使用 React Query + service 接入契约 API。 补齐 useQuery/service hook，保留 loading/error/empty 状态。
2. `P2` Page deployments：页面状态覆盖不足或未能静态识别完整 loading/error/empty。 补齐状态组件，必要时使用 state-* 设计图作为参考。
3. `P2` Overlay /forensics：3 个浮层只有部分静态信号：modal-forensics-task, popconfirm-pcap-download, modal-forensics-evidence-export 补容器：modal-forensics-task -> Modal；popconfirm-pcap-download -> Popconfirm；modal-forensics-evidence-export -> Modal。目标文件：web/ui/src/pages/ForensicsWorkbenchPage.tsx。完成后做截图验收，并保留权限提示、影响范围和审计 trace。
4. `P2` Overlay /deployments：2 个浮层只有部分静态信号：modal-deployment-create, modal-deployment-rollback 补容器：modal-deployment-create -> Modal；modal-deployment-rollback -> Modal。目标文件：web/ui/src/pages/DeploymentManagementPage.tsx。完成后做截图验收，并保留权限提示、影响范围和审计 trace。
5. `P3` Batch 02-threat-forensics：批次仍有页面缺口 0、缺失浮层 0、部分浮层 3 按本批次缺口逐页补齐状态、危险动作保护和浮层触发。
6. `P3` Batch 04-detection-ops：批次仍有页面缺口 2、缺失浮层 0、部分浮层 2 按本批次缺口逐页补齐状态、危险动作保护和浮层触发。

## 使用方式

```bash
node doc/04_assets/ui_suite_gpt_v1/build_frontend_contracts.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_handoff.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_code_gap.mjs
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
```

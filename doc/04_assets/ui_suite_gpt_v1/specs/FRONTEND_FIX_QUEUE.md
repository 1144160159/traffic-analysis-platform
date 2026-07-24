# 前端修复队列

本文件按优先级列出当前代码相对 UI 契约的可执行修复项。P0/P1 先处理，P2/P3 在页面批次内处理。

| 优先级 | 区域 | 目标 | 问题 | 动作 |
| --- | --- | --- | --- | --- |
| P1 | Page | `deployments` | 页面未能静态证明使用 React Query + service 接入契约 API。 | 补齐 useQuery/service hook，保留 loading/error/empty 状态。 |
| P2 | Page | `deployments` | 页面状态覆盖不足或未能静态识别完整 loading/error/empty。 | 补齐状态组件，必要时使用 state-* 设计图作为参考。 |
| P2 | Overlay | `/forensics` | 3 个浮层只有部分静态信号：modal-forensics-task, popconfirm-pcap-download, modal-forensics-evidence-export | 补容器：modal-forensics-task -> Modal；popconfirm-pcap-download -> Popconfirm；modal-forensics-evidence-export -> Modal。目标文件：web/ui/src/pages/ForensicsWorkbenchPage.tsx。完成后做截图验收，并保留权限提示、影响范围和审计 trace。 |
| P2 | Overlay | `/deployments` | 2 个浮层只有部分静态信号：modal-deployment-create, modal-deployment-rollback | 补容器：modal-deployment-create -> Modal；modal-deployment-rollback -> Modal。目标文件：web/ui/src/pages/DeploymentManagementPage.tsx。完成后做截图验收，并保留权限提示、影响范围和审计 trace。 |
| P3 | Batch | `02-threat-forensics` | 批次仍有页面缺口 0、缺失浮层 0、部分浮层 3 | 按本批次缺口逐页补齐状态、危险动作保护和浮层触发。 |
| P3 | Batch | `04-detection-ops` | 批次仍有页面缺口 2、缺失浮层 0、部分浮层 2 | 按本批次缺口逐页补齐状态、危险动作保护和浮层触发。 |

## 已闭环页面

- `compliance`：2026-07-19 r367/r368 已完成 fail-closed 业务链路、真实 ZIP/PDF/DOCX、整改幂等、不可变固化和 Windows Chrome 主页面/Drawer/双 Modal 4/4 双门禁；独立逻辑与布局复审 P0=0/P1=0/P2=0。该项从活动修复队列移出，项目级第三方正式材料仍单独跟踪。
- `settings`：2026-07-20 r509/r510 已完成真实系统配置、连接探测、RBAC 只读/写入分离、令牌权限上限、事务审计与 Windows Chrome 非全屏区域交互。生产预检 88/88、浏览器 20/20、视觉差异率 `0.07115355 < 0.125`，逻辑与布局复审均 P0=0/P1=0/P2=0。下一项为 `not-found`。

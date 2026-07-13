# 资产台账 UI 契约对齐稿 v3

更新时间：2026-07-13  
状态：设计契约对齐完成，尚未证明前端/API/RBAC/审计链路已实现或通过。

本目录保留 v2 的暗色、高密度安全运营视觉方向，但恢复项目现行资产台账状态契约。v2 仅作为视觉探索和审查证据保留，不再作为实现目标。

## 不可变契约

- 顶层分类固定为：`终端、服务器、网络设备、业务系统、未知资产`。
- 页面状态固定为：`tab + assetId + detail`。
- 默认对象固定为：`tab=endpoint`、`assetId=PC-0082`。
- 资产详情仅从 `tab=server` 和 `SRV-*` 对象打开。
- `detail` 仅允许：`basic`、`network-interface`、`open-services`、`ownership`、`history`。
- “高风险、最近变更”只作为筛选/保存视图，不升级为顶层 Tab。
- “关系”只提供跳转 `/graph?assetId=...` 的入口；“行为、证据”不新增为资产详情状态。
- 编辑使用小尺寸 Modal；批量治理使用两步确认，第二步只显示确认摘要。

## 九图映射

| 顺序 | 文件 | 契约状态 | 说明 |
|---:|---|---|---|
| 01 | `01-assets-endpoint-PC-0082.png` | `tab=endpoint&assetId=PC-0082` | 终端默认态，保留五类资产 Tab |
| 02 | `02-assets-unknown-attribution.png` | `tab=unknown&assetId=UNK-10.12.88.45` | 未知资产归属闭环；“确认归属”为主操作 |
| 03 | `03-assets-server-high-risk.png` | `tab=server&assetId=SRV-0007` + 高风险筛选 | 高风险是筛选态；开放服务通过既有入口进入 |
| 04 | `04-assets-detail-history.png` | `tab=server&assetId=SRV-0007&detail=history` | 变更、Diff、审计、回滚门槛 |
| 05 | `05-assets-detail-basic.png` | `tab=server&assetId=SRV-0007&detail=basic` | 基础信息；关系能力仅保留“跳转实体图谱” |
| 06 | `06-assets-detail-network-interface.png` | `tab=server&assetId=SRV-0007&detail=network-interface` | 网络接口、镜像口、链路与流量摘要 |
| 07 | `07-assets-detail-ownership.png` | `tab=server&assetId=SRV-0007&detail=ownership` | 责任角色、业务系统、数据域与审计 |
| 08 | `08-modal-asset-edit-compact.png` | `tab=server&assetId=SRV-0007&detail=basic` + 编辑操作 | 560×400 小 Modal，ROI 约 0.108 |
| 09 | `09-modal-asset-batch-governance-step2.png` | `tab=server` + 高风险筛选 + 批量治理第二步 | 600×400 确认 Modal，ROI 约 0.116 |

`open-services` 仍属于正式详情枚举，但本批九图通过 03 的“查看开放服务”入口承接；现有目标图 `../assets-detail-open-services.png` 仍是该状态的视觉真源。

## Overlay 约束

### 编辑资产

- 只包含名称、主机名、重要性、责任部门、负责人、标签和备注。
- IP、MAC、操作系统由采集源维护，只读提示必须可见。
- 负责人缺失时显示文字错误，不只显示红色边框。
- 唯一主操作为“保存并记录审计”。

### 批量治理

- 第一步选择操作，第二步确认并提交。
- 第二步显示选择数量、风险、告警、业务系统影响和权限校验结果。
- 责任冲突使用图标 + 文字警告。
- 每个资产生成独立审计记录和回滚边界。
- 唯一主操作为“确认生成 3 个工单”。

## 验证边界

- 本目录只证明静态视觉、信息架构和截图状态映射。
- `contract.json` 定义 route、前置交互、breakdown 区域和 capture 门禁；它是前端实现与采集脚本的输入草案。
- 仍需在 Windows Chrome 中验证真实分页、筛选、键盘焦点、RBAC、审计、下载授权、loading/error/empty/forbidden 状态。
- 正式验收必须生成同批次 actual/diff/metrics/capture-meta，视觉 ROI 门禁为 `< 0.125`。

## 生成说明

- 01–07 直接复用仓库现行契约目标图，避免以新视觉稿重定义状态模型。
- 08–09 使用图像生成进行语义整改，再做确定性 1920×1080 复合与 ROI 尺寸约束。
- `.raw-imagegen.png` 仅用于生成溯源；无 `.raw-imagegen` 后缀的文件是最终交付图。

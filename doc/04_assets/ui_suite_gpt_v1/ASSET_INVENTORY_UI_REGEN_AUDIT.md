# 资产台账 UI 重生逻辑审计

更新日期：2026-07-13

## 结论

资产台账需要先按状态模型重排，再做图片重生和前端对齐。当前问题不是单纯“画得不好看”，而是三类 UI 状态被混在一起：

1. `/assets` 页面级资产分类大 Tab：`终端 / 服务器 / 网络设备 / 业务系统 / 未知资产`。
2. 选中服务器后的资产详情小 Tab：`基础信息 / 网络接口 / 开放服务 / 归属信息 / 历史变更`。
3. 资产详情父容器：右侧 Drawer，只承载选中资产轻摘要、小 Tab 导航、入口动作、门禁和审计线索，不替代小 Tab 内容页。

当前前端状态模型已经基本正确：只有 `server + SRV-*` 可以打开详情小 Tab；`endpoint / network-device / business-system / unknown` 不应出现服务器详情小 Tab，也不应自动打开详情 Drawer。后续 UI 图必须严格跟随这个模型。

## 本轮新增硬约束

- 资产台账现有 UI 图只作为视觉参考，不作为业务逻辑真源。
- 生成和开发前必须审核五类资产的业务对象、字段、分析模块、状态流、详情能力、权限和处置闭环；不能只按旧图换皮。
- 如果旧图与路由状态、资产类型能力或真实接口不一致，允许自主调整页面结构和交互，以项目逻辑为准。
- 五个分类页必须共享稳定的 AppShell、分类导航和基础布局语言，但主内容要体现各资产类型的真实差异，不能只替换标题与数字。
- 1920x1080 主业务区域必须被有价值的内容充分利用，不允许出现因拆解不足、固定高度或栅格失衡产生的大面积空白。
- “填满”不等于堆叠装饰：补充内容必须来自摘要、清单、关系、行为、证据、风险、任务、Owner 或审计闭环。
- 最终以前端真实页面的逻辑性、美观度和可用性为验收对象；实现若合理纠正了参考图，必须同步更新 UI 图及页面 contract。

## 现有新增产物

本轮已把聊天内置 Image Gen 生成但未落入项目的原图转为项目内非破坏性候选稿：

- 候选终稿：`doc/04_assets/ui_suite_gpt_v1/screens/pages/assets-endpoint-redesign-v1.png`
- 原始追溯：`doc/04_assets/ui_suite_gpt_v1/screens/pages/assets-endpoint-redesign-v1.raw-imagegen.png`
- 原图尺寸：`1672x941`
- 项目候选尺寸：`1920x1080`

该候选稿只作为视觉密度参考，暂不能覆盖 `assets.png`，原因是顶部公共壳带入了用户/通知动作，与当前 AppShell 责任边界不一致。

临时比对图：

- `tmp/assets-ui-audit-contact-sheet.png`

## 正确页面状态模型

| 层级 | URL 状态 | 图像/组件对象 | 是否显示 AppShell | 是否显示资产分类大 Tab | 是否显示详情小 Tab | 备注 |
|---|---|---|---:|---:|---:|---|
| 资产分类大 Tab - 终端 | `/assets?tab=endpoint&assetId=PC-0082` | `assets.png` | 是 | 是 | 否 | 选中 `PC-0082 / 实验楼-PC-0082`，不能出现服务器详情页签。 |
| 资产分类大 Tab - 服务器 | `/assets?tab=server&assetId=SRV-0007` | `assets-server.png` | 是 | 是 | 右侧入口可见 | 选中 `SRV-0007 / 实验楼-SRV-12`，允许进入详情 Drawer。 |
| 资产分类大 Tab - 网络设备 | `/assets?tab=network-device&assetId=NET-0001` | `assets-network-device.png` | 是 | 是 | 否 | 网络设备只展示设备清单、接口矩阵、链路拓扑、镜像口等。 |
| 资产分类大 Tab - 业务系统 | `/assets?tab=business-system&assetId=BIZ-0001` | `assets-business-system.png` | 是 | 是 | 否 | 业务系统展示依赖资产、关键服务、SLA、责任部门。 |
| 资产分类大 Tab - 未知资产 | `/assets?tab=unknown&assetId=UNK-10.12.88.45` | `assets-unknown.png` | 是 | 是 | 否 | 未知资产展示发现、归属候选、置信度、工单闭环。 |
| 详情小 Tab | `/assets?tab=server&assetId=SRV-0007&detail=basic` 等 | `assets-detail-*.png` / `AssetDetailWorkspace` | 否，作为 Drawer 内容 | 否 | 是 | 只在服务器详情 Drawer 内展示，不是独立页面主图。 |
| 详情父容器 | `/assets?tab=server&assetId=SRV-0007&detail=basic` | `drawer-asset-detail.png` | 宿主页面 + 右侧窄 Drawer | 宿主保留 | 父容器仅展示入口 | 不展开任一详情小 Tab 的完整表格。 |

## 需要重新生成的图

### P0：必须重生

1. `screens/pages/assets.png`
   - 目标状态：`/assets?tab=endpoint&assetId=PC-0082`
   - 原因：用户明确反馈资产台账页面视觉不够美观、填充不满；本轮候选 `assets-endpoint-redesign-v1.png` 虽然更饱满，但 AppShell 不合规，不能直接替换。
   - 重生要求：
     - 保留 `/assets` 页面级分类大 Tab。
     - 终端态不能出现 `基础信息 / 网络接口 / 开放服务 / 归属信息 / 历史变更`。
     - 选中对象必须稳定为 `PC-0082 / 实验楼-PC-0082`。
     - 顶部、左侧、底部公共 AppShell 必须以 `screens/pages/screen.png` 为准，不能新增顶部用户、通知、设置、电源入口。
     - 内容区必须填满：筛选、五个 KPI、终端表、流量画像、协议分布、Top 对端、周期热力图、关联证据、右侧闭环栏。

2. `screens/pages/assets-server.png`
   - 目标状态：`/assets?tab=server&assetId=SRV-0007`
   - 原因：如果 `assets.png` 采用新的高密度布局，服务器分类页必须同步重生以保持资产分类大 Tab 的统一视觉语言。
   - 重生要求：
     - 只在服务器分类下暴露详情入口。
     - 右侧摘要可以出现详情小 Tab 入口，但不能把完整详情页平铺到页面主区。

3. `screens/pages/assets-network-device.png`
4. `screens/pages/assets-business-system.png`
5. `screens/pages/assets-unknown.png`
   - 原因：与 `assets.png`、`assets-server.png` 同属资产分类大 Tab。若只重生终端，会造成分类切换时视觉密度和布局语言不一致。
   - 重生要求：
     - 共享同一 AppShell 和资产台账页面框架。
     - 每个分类的业务主区必须不同，不允许只替换标题/数字。

### P1：暂缓重生，先保留

1. `screens/pages/assets-detail-basic.png`
2. `screens/pages/assets-detail-network-interface.png`
3. `screens/pages/assets-detail-open-services.png`
4. `screens/pages/assets-detail-ownership.png`
5. `screens/pages/assets-detail-history.png`

这些图当前应被视为“服务器详情 Drawer 内的小 Tab 内容图”，不是 `/assets` 页面主图。它们的主要风险是命名和 breakdown 里仍写着 pages/route `/assets`，容易误导后续实现。先不重生图片，先在实现和文档中明确其局部组件身份。

### P2：不作为本轮 UI 重生对象

1. `screens/overlays/drawer-asset-detail.png`
2. `screens/overlays/modal-asset-edit.png`
3. `screens/overlays/drawer-asset-history.png`

这些是资产相关 overlay，不属于资产分类大 Tab 本体。除非生产截图证明尺寸或交互不合格，否则不进入第一批 UI 重生。

## 前端实现对齐点

当前代码入口：

- `web/ui/src/pages/AssetInventoryPage.tsx`
- `web/ui/src/pages/AssetDetailWorkspace.tsx`
- `web/ui/src/pages/assetInventoryState.ts`
- `web/ui/src/styles/pages.css`

必须保持的逻辑：

- `assetTabs` 为五个页面级分类大 Tab。
- `assetDetailTabs` 只属于服务器详情工作区。
- `defaultAssetIdByTab.endpoint = PC-0082`
- `defaultAssetIdByTab.server = SRV-0007`
- `canOpenAssetDetail(tab, assetId)` 只能允许 `server + SRV-*`。
- 终端、网络设备、业务系统、未知资产不显示服务器详情小 Tab。

需要优化的实现方向：

- 资产台账主区继续采用两栏结构：主工作区 + 右侧闭环栏。
- 主工作区需要进一步压缩垂直空隙，表格行数、图表区和证据条要形成满屏闭环。
- 右侧栏需要从摘要扩展为完整闭环：身份、风险评分、风险因素、证据数量、处理任务、Owner/审计、动作按钮。
- 详情 Drawer 保持右侧窄 Drawer，不改成全屏覆盖层。

## 下一步执行顺序

1. 用现有 `assets.png`、`assets-endpoint-redesign-v1.png`、`screen.png` 作为三张参考，重新生成合规的 `assets.png` 候选。
2. 通过视觉检查确认 AppShell、选中对象、详情小 Tab 禁用规则均合格。
3. 再按同一框架批量生成 `assets-server / assets-network-device / assets-business-system / assets-unknown`。
4. 前端页面按新 UI 图回修 CSS 和内容密度。
5. 运行资产台账交互验证，重点验证 URL 状态、选中对象、详情 Drawer、分类切换和生产截图。

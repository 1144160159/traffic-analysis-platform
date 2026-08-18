# T1-M09-N013 攻击链 Snapshot 与页面回滚

适用对象：`RB-T1-M09-P029-PRJ-n013-s1` 的 M07 Snapshot read projection，以及 `RB-T1-M09-P030-UI-n013-s2` 的 attack-chain/campaign typed adapter 和页面。后端新读路径默认仍为 `ATTACK_CHAIN_V1_ENABLED=false`。

## 停止条件

出现跨租户 Snapshot、source/target 被交换、页面把非连续边补成路径、observed/derived/analyst 来源混淆、替代路径或不确定性被隐藏、truncated/partial 被显示为完整、证据按钮指向不同 evidence ID、图或 UI 反写 PostgreSQL 权威，或 M07 Snapshot 不含建议时页面仍生成处置建议，立即停止扩大流量。

## 回滚顺序

1. 设置 `ATTACK_CHAIN_V1_ENABLED=false`，使后端回到已有 campaign compatibility read；不删除新版路由，也不把新版 Snapshot 改写为旧 campaign DTO。
2. Web 回切到上一个固定 digest 镜像。若前后端分阶段回切，typed adapter 必须继续同时接受旧 campaign DTO 与 M07 Snapshot，避免 mixed-version 窗口白屏。
3. 保存出错请求的 tenant、chain ID、Snapshot ID/version/SHA、graph Snapshot ID/SHA、as-of、source watermarks、trace ID、candidate/alternative path ID 和 evidence ID；不能只保存截图。
4. 对照 `attack_chain_snapshots_v1`、`attack_chain_snapshot_current_v1`、`gnn_graph_snapshots_v1` 和 `attack_chain_evidence_manifest_v1`，确认 Snapshot 本身未被 UI 或读 API 改写。若只是派生读形态错误，修复 adapter 后用同一 Snapshot 重放。

## 数据处理

迁移 `202608142100` 是 expand-only。回滚不执行 DROP，不回退 current 指针，不删除 Snapshot、图 Snapshot 或证据锚点，也不从 NebulaGraph 推导事实反写 PostgreSQL。partial/truncated 的边界和 continuation 只能由同一有界查询合同生成；不得人工删减缺失原因来制造 complete。

M07 Snapshot 没有 response recommendation 时，API 必须返回显式 partial 空集合，页面不得根据 relation type、confidence 或图位置猜测处置动作。旧 compatibility read 的已有建议不是新版 Snapshot 的组成部分，不能混入同一溯源结论。

## 重新启用

使用同一候选后端与 Web 镜像，先在 K8s 隔离 PostgreSQL 证明 version 单调、exact replay、跨租户拒绝、candidate/alternative/analyst evidence 不丢失；再证明 API 的 phases/paths/evidence 都引用同一 Snapshot，最后检查 Web bundle 与指定浏览器旅程。必须逐边核对 source/target、provenance、uncertainty、evidence drilldown、partial/truncated 和 source watermark。该流程不授权生产发布、共享数据迁移、Windows Chrome 验收或全项目完成声明。

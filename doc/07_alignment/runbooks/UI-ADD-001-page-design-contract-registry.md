# UI-ADD-001 页面设计契约注册表

## 目标

`web/ui/src/routes/pageDesignContracts.v1.json` 是28条视觉验收页面的版本化设计契约。它把目标任务、基准状态、1920×1080 Windows Chrome 视觉真值、信息架构、API plan、TypeScript 类型源、设计 Token、兼容约束、canonical ID、owner 和独立 reviewer 固定在一个可机器验证的注册表中。

28页口径为：24个导航页面、2个详情页面、登录页和404页。`/topics/tunnel`、`/topics/exfil`、`/topics/apt` 是兼容重定向，只登记为 alias，不重复计为独立页面。

## 门禁

运行：

```bash
python3 scripts/alignment/verify_ui_page_design_contracts.py
cd web/ui && npm run test -- --run src/routes/pageDesignContracts.test.ts
```

验证器必须同时证明：

- 页面契约、capture plan 和运行时 route manifest 覆盖28/28。
- page ID、route、API plan owner 无重复、未知或孤儿。
- 三条旧专题 URL 仍存在且只重定向到统一专题页。
- canonical ID、TypeScript 类型源、1920×1080视觉真值和设计 Token 均存在。
- 候选 manifest 的合同、路由、API plan、设计 Token、capture plan、dist、镜像 ID 和镜像 manifest digest 与当前文件或已部署不可变候选一致。

## 变更规则

页面、Tab、控件、scope、API 或审计事件的删除必须先进入六类兼容差异裁决。视觉真值只约束布局、层级和交互意图，不允许把图中的固定数字、节点、趋势或状态复制为生产事实。生产候选必须关闭 mock，并保存同 bundle 的 Windows Chrome 截图、HAR、console、trace、权威数据和审计证据。

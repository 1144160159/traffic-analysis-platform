# UI-ADD-001 页面设计契约候选绑定

本证据包把28页版本化设计契约、运行时路由、页面 API plan、设计 Token 和 capture plan 绑定到已部署的不可变 Web UI 候选。`candidate-manifest.json` 中的源码和证据哈希必须由 `scripts/alignment/verify_ui_page_design_contracts.py` 复算通过。

文件说明：

- `candidate-manifest.json`：设计契约与不可变候选绑定及未证明边界。
- `runtime-observation.json`：生成绑定时读取的 Kubernetes Deployment/Pod 状态。
- `validation.json`：契约验证、全量单测、lint、生产构建与仍开放事项。

`pass` 只表示 `UI-ADD-001_PAGE_DESIGN_CONTRACT` 门禁通过，不代表28页前端整改、交互验收或 G5/G7 已完成。当前 Windows Chrome 证据仅覆盖4个业务页面样本；其余24页、全量设计差异、写操作最终效果和无障碍仍保持未证明状态。该设计契约是对既有不可变候选的外部绑定，尚未嵌入候选 bundle。

运行：

```bash
python3 scripts/alignment/verify_ui_page_design_contracts.py
cd web/ui && npm run test -- --run src/routes/pageDesignContracts.test.ts
```

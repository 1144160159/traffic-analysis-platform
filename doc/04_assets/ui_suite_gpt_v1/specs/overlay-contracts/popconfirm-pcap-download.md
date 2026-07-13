# PCAP 下载确认 浮层实现契约

## 基本信息

- ID：`popconfirm-pcap-download`
- 宿主路由：`/forensics`
- 推荐组件：`Popconfirm`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-pcap-download.png`
- Prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/popconfirm-pcap-download.prompt.txt`

## 分层参数

- `popconfirm-surface`：interaction-container，bbox=`{"x":640,"y":330,"w":640,"h":400}`
- `action-bar`：cancel-confirm-actions，bbox=`{"x":1240,"y":950,"w":560,"h":52}`
- `audit-strip`：audit-and-risk-hint，bbox=`{"x":640,"y":950,"w":760,"h":52}`

## 数据与动作

- API 继承：`/api/v1/pcap/jobs`、`/api/v1/pcap/stats`
- 必须包含：权限提示、影响范围、审计 trace、取消/确认动作。
- 危险动作：默认要求二次确认，确认按钮在必填条件未满足时禁用。

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 必须实现为 Popconfirm 或等价语义组件
- [ ] 浮层只承载当前交互容器本体，不恢复完整宿主 AppShell
- [ ] 确认类动作必须出现取消/确认，危险确认默认不可误触

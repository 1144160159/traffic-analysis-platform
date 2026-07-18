# 取证分析 前端实现契约

## 基本信息

- ID：`forensics`
- 路由：`/forensics`
- 领域：`threat-analysis`
- React 页面：`ForensicsWorkbenchPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.png`
- API：`/api/v1/pcap/jobs`、`/api/v1/pcap/stats`、`/api/v1/pcap/verify`、`/api/v1/pcap/presign`
- 页面形态：单页工作台；不允许使用 Tab 分割业务模块

## 必须实现的业务层

- 取证任务
- PCAP 索引
- 会话复放
- 证据完整性
- 证据导出
- 跨页上下文
- Hash 校验结果
- 签名 URL 与有效期
- 取证操作与真实审计日志

## 分层参数

- `topbar`：global-app-shell，bbox=`{"x":0,"y":0,"w":1920,"h":80}`
- `sidebar`：global-app-shell，bbox=`{"x":0,"y":80,"w":166,"h":917}`
- `content`：page-workspace，bbox=`{"x":198,"y":80,"w":1722,"h":917}`
- `bottombar`：global-app-shell，bbox=`{"x":0,"y":997,"w":1920,"h":83}`
- `right-rail`：closed-loop-rail，bbox=`{"x":1460,"y":104,"w":420,"h":860}`

## 组件映射

- AppShell
- WorkPanel
- MetricTile
- Table
- ECharts
- StatusTag

## 数据与分页合同

- 取证任务、PCAP 索引、会话复放、证据导出包、Hash 校验结果均须分页。
- 五个分页栏必须保留固定高度，翻页或过滤后不得随当前行数上下移动。
- 验收场景允许使用 `forensics_ui_fixtures` 的租户级数据库种子；种子必须显式启用、可删除，未启用时回退真实任务数据。
- 标准场景数量为：任务 190、PCAP 索引 1,256、Hash 最近 20；状态机为新建 12、排队中 5、采集中 8、解析中 6、完成 156、失败 3。
- 过滤条件必须传入真实 `/api/v1/pcap/jobs` 查询，不得只在浏览器内伪过滤。

## 安全与审计合同

- 读、写、下载分别要求 `pcap:read`、`pcap:write`、`pcap:download`。
- 未登录访问必须返回 401；只有读权限的用户执行创建或下载必须返回 403。
- 创建、取消、下载、签名 URL、Hash 校验必须同步写入审计；审计落库失败不得返回成功。

## 关联浮层

- `modal-forensics-task`：取证任务详情，Modal
- `popconfirm-pcap-download`：PCAP 下载确认，Popconfirm
- `drawer-session-replay`：会话复放抽屉，Drawer
- `modal-forensics-evidence-export`：取证证据导出，Modal

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
- [ ] Windows Chrome 必须完成目标图并排比对，业务 ROI 差异率小于 `0.125`
- [ ] 五个分页必须逐一翻页并证明分页栏相对模块坐标不变

# 前端 Product Design 审计整改清单（当前开放项）

## 1. 审计边界

- 审计方法：Product Design；不使用 Figma。
- 浏览器证据：Windows Chrome 151，通过既有 Xshell 隧道连接，1920×1080，生产候选路由，`mock=false`。
- 首轮证据：`doc/02_acceptance/runs/20260805-product-design-audit-v1/`。
- 当前不可变候选为 `doc/02_acceptance/runs/20260806-product-design-remediation-candidate-v7/manifest.json`，隔离工作候选源码 SHA256 为 `8a6b22c1ce7fecdd25d95805f73028ac264882a0db76bfb37001f5042671d797`；对应完整 G0 v146 与 Windows Chrome 只读证据均已绑定。
- 关闭规则：只有代码、真实 API、Windows Chrome、权威数据/审计等适用证据闭环后才关闭；关闭项不再进入后续开放清单、进度说明、难度排序或阻塞说明。

## 2. 当前开放项

无。新发现的前端问题必须使用新的 occurrence ID 重新登记，不得恢复已关闭项。

## 3. 当前候选与发布边界

- 当前完整 G0 v146 的 alignment/full/Python 分别耗时 `66.680s / 122.255s / 210.562s`，三项均为 `PASS`；manifest SHA256 为 `2d6f1afd3188c1921e522e07501dc23fe7e48d6323e057e5011610741ab53097`。
- 当前隔离工作候选源码 SHA256：`8a6b22c1ce7fecdd25d95805f73028ac264882a0db76bfb37001f5042671d797`。
- 当前 Web UI 只读预览镜像：`docker.io/traffic/web-ui:remediation-ui-8a6b22c1ce7f`，image ID `sha256:ee9913920f6223058e3bca3a115cdf4f5e763e7b0b20a0dbbb3e99c070301564`。
- 当前 Alert Service 候选镜像：`docker.io/traffic/alert-service:remediation-8a6b22c1ce7f`，image ID `sha256:0d6cd02082b1fce5d2228528c1b5bf12c91cd401b3a684204b758246df57cb0c`；其二进制与前一只读验证镜像逐字节一致。
- 当前未部署、未执行生产 mutation；平台数据与发布门禁继续在 canonical ledger 和专项 runbook 中管理，不计入本前端开放清单。
- 当前隔离只读 Windows Chrome Product Design 复检不执行生产写入；本清单只保留仍未闭环的开放项。

## 4. 证据限制

- 当前截图不能单独证明键盘顺序、焦点可见性、200% 缩放、屏幕阅读器语义或业务写入完成。
- 页面无 4xx/5xx 不能证明跨存储一致；HTTP 2xx、截图和单元测试不能互相替代。
- 候选浏览器证据必须绑定上述不可变镜像、关闭 mock，并保留 HAR/请求结果、console、trace、权威数据和审计引用。

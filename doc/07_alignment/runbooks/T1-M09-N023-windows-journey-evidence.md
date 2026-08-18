# T1-M09-N023 指定 Windows Chrome 与跨存储旅程证据

状态：`PARTIAL`。本任务已经实现只读证据合同、聚合器、mutation 测试和 Kubernetes Job；尚未取得指定 Windows Chrome 的 7 条旅程回执，因此不得晋级。

## 代码入口

- `validate_source_evidence(root, contract)`：校验 N012-N020 前置 K8s 证据的路径、SHA256、task/run 身份、PASS 状态和非生产边界。
- `validate_journey(journey, required, contract)`：对单条旅程执行浏览器、候选、六类检查、network/console、跨存储 receipt 和 final fact 校验。
- `_validate_trace(...)`：要求同一个 `trace_id` 和候选镜像 hash 覆盖合同声明的每个存储，并要求最终业务事实落在同一 trace 中。
- `validate_manifest(root, contract, manifest)`：按 P056-P062 的固定顺序只读聚合 7 条旅程；只有全部 `VERIFIED` 才返回 `COMPLETE` 和 `promotion_eligible=true`。
- `validate_kubernetes_evidence(...)`：绑定 Job/Pod/image 身份、源文件 hash、资源清理和所有共享存储未触碰声明。
- `run_m09_journey_evidence_k8s.py`：创建 run-scoped ConfigMap/Job，以非 root、只读根文件系统、禁用 service-account token 的方式执行聚合器并在 finally 中回收资源。

## 单条旅程通过条件

每条旅程必须同时满足：

1. 浏览器为 Windows Chrome，backend 为 `chrome_extension`，viewport 为 `1366x900` 或 `1600x900`，URL 使用指定 `127.0.0.1:25173` 隧道。
2. app image、image ID、Web K8s manifest hash、APISIX route config hash 与合同完全相同。
3. `query`、`mutation`、`permission`、`failure_recovery`、`download`、`final_fact` 六类检查全部有独立 oracle 且 PASS。
4. network 无 request failure/4xx/5xx；console 无 error/pageerror/runtime exception；两类原始工件都有 SHA256。
5. 所有要求的 PostgreSQL/Kafka/ClickHouse/OpenSearch/MinIO/NebulaGraph 存储均产生同 trace、同候选 hash 的 receipt，最终业务事实也绑定该 trace。
6. `dirty=false`、`source_hash_match=true`。dirty 截图、异 hash 截图和 Linux Chromium 截图一律不可用于 Windows 旅程晋级。

## 当前 Kubernetes 结果

- run-id：`7062f88b-a1a2-471d-ad0d-6f586da7a21f`
- runtime image：`traffic/m09-journey-evidence-test:m09-n023-20260816-r1`
- image ID：`sha256:472cc54139e0046eead8e9d3d62876291a89a0fa376a1ac993f41ab5765e693e`
- 结果：聚合器执行 PASS；覆盖状态 `PARTIAL`；0 条 `VERIFIED`，7 条 `BLOCKED_MISSING_WINDOWS_CHROME`；`promotion_eligible=false`。
- 边界：未连接共享 PostgreSQL、Kafka、ClickHouse、MinIO、NebulaGraph；未应用生产变更；Job、Pod、ConfigMap 已按 run-id 清理。

## 执行与回滚

正式浏览器采集必须从部署相同候选镜像和配置 hash 开始，再按 P056-P062 顺序写入 `journey-evidence-input.json`。每完成一条先运行定向测试和聚合器；任一 hash、trace、权限负例、恢复或最终事实不匹配时保留 `BLOCKED`，不得改写为 `VERIFIED`。

本任务不改变运行开关和业务数据。回滚仅删除 N023 新增的合同、聚合器、测试、runbook 和 run-scoped K8s 资源；不得删除前置任务证据或共享存储数据。

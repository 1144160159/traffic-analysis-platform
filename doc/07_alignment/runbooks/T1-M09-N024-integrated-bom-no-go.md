# T1-M09-N024 一体化 BOM 与发布指针

状态：BOM `ASSEMBLED`，工程索引 `PASS`，发布 `NO_GO`。`ASSEMBLED` 只说明 N001-N023 的 23 个 task-local K8s 证据、26 条依赖边、组件角色和 SHA256 已被一个确定性 BOM 收拢，不说明它们属于同一 implementation candidate。

## 代码入口

- `build_m09_integrated_bom.py#build_component`：校验证据路径、task/status、非生产边界，提取 run/image identity 和依赖。
- `build_m09_integrated_bom.py#build_trace_index`：按 P056-P062 固定顺序登记 network、receipt、storage/object、final effect 和 trace-id 是否存在。
- `build_m09_integrated_bom.py#build`：生成 23 组件 BOM、26 条边、assembly-id、闭包计数和阻断码。
- `build_m09_release_artifacts.py#build_index`：P064 只登记既有证据，不运行测试、不修改结果。
- `build_m09_release_artifacts.py#build_pointer`：P065 只表达发布决定；当前强制 `NO_GO`，不修改生产代码、配置或证据。
- `verify_m09_integrated_bom.py#validate`：重新生成期望值并逐字节比较 BOM/index/pointer，额外拒绝 candidate、Windows、trace、production 或 GO 过度声明。

## 当前闭包

- assembly-id：`ac11577a22b436c337bd045a22a6b5a74ee52b75a772af0244f3453c5abfea32`
- BOM SHA256：`91bac6e61e436276b0108764e49827670061340731b8bfa01cbd9cc57e058e2b`
- P064 evidence-index SHA256：`6b0d607741ec2ba8c5687059ebae6479957a3cf55a95a5cf4fc784e1c5f5467c`
- P065 release pointer SHA256：`a37507857c72687b8c97d70f4f145df4ebcceed08ab879531664519cf4ed32b9`
- 组件：23/23 有精确 evidence hash；依赖边：26；Windows 旅程：0/7 verified；完整跨存储 trace：0/7；production applied：false。

## NO-GO 条件

以下四项必须全部关闭后才能重新生成 GO 候选：

1. `SAME_CANDIDATE_MANIFEST_REQUIRED`
2. `WINDOWS_CHROME_SEVEN_JOURNEYS_REQUIRED`
3. `CROSS_STORAGE_TRACE_FINAL_EFFECT_REQUIRED`
4. `PRODUCTION_APPLIED_REQUIRED`

不得用 HTTP 2xx、截图、Linux Chromium、分散 demo、多个镜像 tag 或历史不同 hash 的 K8s PASS 替代同一 candidate manifest 下的完整终态旅程。

## 回滚

本任务没有运行态变更。回滚只回退 N024 的生成器、BOM、P064 index、P065 pointer、验证器、测试和本手册；不得删除 N001-N023 原始证据。若未来 candidate 或任一 evidence hash 变化，旧 BOM 保留为历史制品并生成新 assembly-id，不原地改写已发布指针。

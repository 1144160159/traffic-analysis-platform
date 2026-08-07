# F-ADAPTER-002 数据猜测型 adapter 清理回滚

## 范围

清理采用“typed 字段先扩展、内部猜测后退出”。回滚只可恢复经登记的显式字段别名、版本兼容或单位换算；不得恢复硬编码业务 IP、演示计数、fixture 回退、前端派生拓扑或 truthy 零值覆盖。

## 停止条件

- typed 字段与权威源对账不一致。
- 生产 bundle 出现运行时 mock/fixture 依赖。
- 合法 0、空字符串、空数组、未采集和 unavailable 被合并。
- 新增未登记风险或外部路由、字段、按钮、操作被移除。

## 回滚步骤

1. 停止扩大 `typed_adapter_cleanup_v1`，保留运行样本、HAR、trace 和 adapter registry。
2. 回滚到上一个 hash 绑定的 typed adapter，仅恢复已登记的兼容映射。
3. 对缺少新字段的页面显示 partial/暂不可用，不从其他数组或演示常量构造值。
4. 恢复旧字段别名时记录 owner、调用量和复审日期，禁止静默多参数猜测。
5. 重跑 0值、空值、partial、mock-off bundle 和候选兼容差异门禁。

## 回滚验收

用户可见能力不减，页面不显示伪业务事实，风险清单无未登记新项，六类 `removed_*` 仍全空。

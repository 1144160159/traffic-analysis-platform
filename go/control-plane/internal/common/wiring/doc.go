// Package wiring 定义 cmd/* 入口装配的约定与顺序契约(GoF Mediator 的
// 显式化替代)。不引入中央协调器:跨服务调用已经由 APISIX 路由 + 各域
// typed client 充当"分布式中介者",本包只固化进程内装配纪律。
//
// 装配顺序契约(与《主业务闭环快速落地执行方案 v1》2.1 节一致):
//   1. 配置与凭证(只从环境/Secret 读取,禁止硬编码);
//   2. 存储与消息依赖(PG/CH/Redis/Kafka)按需连接,失败 fail-fast;
//   3. 域服务/仓库/适配器构造,显式注入(禁止服务内再建全局单例链);
//   4. 中间件责任链单一构造点(Recovery→RequestID→Logging→CORS→
//      Metrics→Tenant→Authenticate→契约解释器,见 cmd/alert-service);
//   5. HTTP 路由挂载;
//   6. 消费者/后台 worker 启动 + readiness 门禁;
//   7. 优雅停机与回滚路径。
//
// 每服务重构时把 main 中的装配抽成本服务 wiring.go(返回依赖结构体),
// 保持"先合同、后迁移、再消费者、最后生产者和 UI"的顺序显式可见。
package wiring

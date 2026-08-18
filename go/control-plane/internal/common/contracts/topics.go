// Package contracts 控制面跨域公共契约常量:topic 名、信封 schema 版本、
// 事件类型标签(Go/Java/Rust 三语对齐的唯一真源引用,实体定义仍在 proto/契约文件)。
package contracts

const (
	// Topic 名(与 contracts/events/kafka-topic-catalog.v1.json 对齐)
	TopicFlowEvents             = "flow.events.v1"
	TopicDLQ                    = "dlq.v1"
	TopicAnalysisPlanEvents     = "analysis.plan.events.v1"
	TopicAnalysisRunEvents      = "analysis.run.events.v1"
	TopicAnalysisEnvelopes      = "analysis.envelopes.v1"
	TopicAnalysisReceipts       = "analysis.receipts.v1"
	TopicAnalysisReportRequests = "analysis.report.requests.v1"

	// 信封 schema 版本(加法式演进起点)
	SchemaVersionV1 = "1"

	// 事件类型标签(EventHeader.event_type / aggregate_type)
	EventTypeTrafficFlow = "traffic.flow.v1"
	AggregateTypeFlow    = "flow"

	// 流身份算法修订:2 = 规范采集事件时间身份(与 proto identity_revision 注释一致)
	FlowIdentityRevisionCanonical = 2

	// 回放数据面生产者/探针标签
	ProducerAnalysisReplay = "analysis-replay"
	ProducerProbeAgent     = "probe-agent"
	ProbeIDPcapReplay      = "pcap-replay"
)

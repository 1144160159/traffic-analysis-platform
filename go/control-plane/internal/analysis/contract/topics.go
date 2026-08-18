// Package contract 统一分析任务调度中心跨边界合同(P0 冻结):
// 稳定错误码、Kafka topic 与信封结构。错误码与 topic 名统一复用
// internal/common(common/errors 码表、common/contracts 常量),本包保留
// 分析域信封结构与 AllTopics/AllErrorCodes 聚合视图。
package contract

import (
	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// 稳定错误码(卷A §2.4)。真源:common/errors 码表(统一 HTTP/重试分类)。
const (
	ErrCodePlanNotApproved           = commonerrors.ErrCodeAnalysisPlanNotApproved
	ErrCodeIdempotencyPayloadMismatch = commonerrors.ErrCodeAnalysisIdempotencyPayloadMismatch
	ErrCodeWindowMisfired            = commonerrors.ErrCodeAnalysisWindowMisfired
	ErrCodeCapacityDenied            = commonerrors.ErrCodeAnalysisCapacityDenied
	ErrCodeStaleFence                = commonerrors.ErrCodeAnalysisStaleFence
	ErrCodeRunNotCancelable          = commonerrors.ErrCodeAnalysisRunNotCancelable
	ErrCodeFeatureNotReleased        = commonerrors.ErrCodeAnalysisFeatureNotReleased
	ErrCodeStageRetryUnsupported     = commonerrors.ErrCodeAnalysisStageRetryUnsupported
	ErrCodeInvalidTransition         = commonerrors.ErrCodeAnalysisInvalidTransition
	ErrCodeNotFound                  = commonerrors.ErrCodeAnalysisNotFound
	ErrCodeMissingIdempotencyKey     = commonerrors.ErrCodeAnalysisMissingIdempotencyKey
	ErrCodeRevisionConflict          = commonerrors.ErrCodeAnalysisRevisionConflict
)

// Kafka topic 目录(卷B §3.1)。真源:common/contracts 常量 + init-jobs/01-kafka-topics.yaml。
const (
	TopicPlanEvents     = commoncontracts.TopicAnalysisPlanEvents    // key=tenant+execution_spec_sha256(compact)
	TopicRunEvents      = commoncontracts.TopicAnalysisRunEvents     // key=tenant+run_id
	TopicEnvelopes      = commoncontracts.TopicAnalysisEnvelopes     // key=tenant+run_id+community_id
	TopicReceipts       = commoncontracts.TopicAnalysisReceipts      // key=tenant+run_id+execution_node_id+attempt
	TopicReportRequests = commoncontracts.TopicAnalysisReportRequests // key=tenant+report_id
)

// AllTopics 全部调度中心 topic(建表/对账用)。
var AllTopics = []string{TopicPlanEvents, TopicRunEvents, TopicEnvelopes, TopicReceipts, TopicReportRequests}

// AllErrorCodes 稳定错误码全集(跨边界错误解析用,顺序无关)。
var AllErrorCodes = []commonerrors.ErrorCode{
	ErrCodePlanNotApproved,
	ErrCodeIdempotencyPayloadMismatch,
	ErrCodeWindowMisfired,
	ErrCodeCapacityDenied,
	ErrCodeStaleFence,
	ErrCodeRunNotCancelable,
	ErrCodeFeatureNotReleased,
	ErrCodeStageRetryUnsupported,
	ErrCodeInvalidTransition,
	ErrCodeNotFound,
	ErrCodeMissingIdempotencyKey,
	ErrCodeRevisionConflict,
}

// ParseError 从 "CODE: message" 前缀错误文本还原稳定错误码(common/errors 统一框架入口);
// 非契约错误返回 ok=false。
func ParseError(err error) (code string, message string, ok bool) {
	c, msg, ok := commonerrors.ParseErrorCode(err)
	return string(c), msg, ok
}

////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/otel/attributes.go
// 修复版：添加业务属性辅助函数
////////////////////////////////////////////////////////////////////////////////

package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// 业务属性键定义
const (
	TenantIDKey     = "tenant.id"
	UserIDKey       = "user.id"
	ProbeIDKey      = "probe.id"
	RunIDKey        = "run.id"
	EventIDKey      = "event.id"
	CommunityIDKey  = "community.id"
	SessionIDKey    = "session.id"
	FlowIDKey       = "flow.id"
	FeatureSetIDKey = "feature_set.id"
	ModelVersionKey = "model.version"
	RuleVersionKey  = "rule.version"
)

// AddTenantAttribute 修复：添加租户ID到Span
func AddTenantAttribute(ctx context.Context, tenantID string) {
	if tenantID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(TenantIDKey, tenantID))
}

// AddUserAttribute 修复：添加用户ID到Span
func AddUserAttribute(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(UserIDKey, userID))
}

// AddProbeAttribute 修复：添加探针ID到Span
func AddProbeAttribute(ctx context.Context, probeID string) {
	if probeID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(ProbeIDKey, probeID))
}

// AddRunAttribute 添加运行ID到Span
func AddRunAttribute(ctx context.Context, runID string) {
	if runID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(RunIDKey, runID))
}

// AddEventAttribute 添加事件ID到Span
func AddEventAttribute(ctx context.Context, eventID string) {
	if eventID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(EventIDKey, eventID))
}

// AddCommunityAttribute 添加社区ID到Span
func AddCommunityAttribute(ctx context.Context, communityID string) {
	if communityID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(CommunityIDKey, communityID))
}

// AddSessionAttribute 添加会话ID到Span
func AddSessionAttribute(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(SessionIDKey, sessionID))
}

// AddFlowAttribute 添加流ID到Span
func AddFlowAttribute(ctx context.Context, flowID string) {
	if flowID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(FlowIDKey, flowID))
}

// AddFeatureSetAttribute 添加特征集ID到Span
func AddFeatureSetAttribute(ctx context.Context, featureSetID string) {
	if featureSetID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(FeatureSetIDKey, featureSetID))
}

// AddModelVersionAttribute 添加模型版本到Span
func AddModelVersionAttribute(ctx context.Context, modelVersion string) {
	if modelVersion == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(ModelVersionKey, modelVersion))
}

// AddRuleVersionAttribute 添加规则版本到Span
func AddRuleVersionAttribute(ctx context.Context, ruleVersion string) {
	if ruleVersion == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(RuleVersionKey, ruleVersion))
}

// AddBusinessAttributes 批量添加业务属性
func AddBusinessAttributes(ctx context.Context, attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}

	span := trace.SpanFromContext(ctx)
	kvs := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		if v != "" {
			kvs = append(kvs, attribute.String(k, v))
		}
	}

	if len(kvs) > 0 {
		span.SetAttributes(kvs...)
	}
}

// AddIntAttribute 添加整数属性
func AddIntAttribute(ctx context.Context, key string, value int64) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Int64(key, value))
}

// AddFloatAttribute 添加浮点数属性
func AddFloatAttribute(ctx context.Context, key string, value float64) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Float64(key, value))
}

// AddBoolAttribute 添加布尔属性
func AddBoolAttribute(ctx context.Context, key string, value bool) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Bool(key, value))
}

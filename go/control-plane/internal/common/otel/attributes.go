package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

func AddTenantAttribute(ctx context.Context, tenantID string) {
	if tenantID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(TenantIDKey, tenantID))
}

func AddUserAttribute(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(UserIDKey, userID))
}

func AddProbeAttribute(ctx context.Context, probeID string) {
	if probeID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(ProbeIDKey, probeID))
}

func AddRunAttribute(ctx context.Context, runID string) {
	if runID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(RunIDKey, runID))
}

func AddEventAttribute(ctx context.Context, eventID string) {
	if eventID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(EventIDKey, eventID))
}

func AddCommunityAttribute(ctx context.Context, communityID string) {
	if communityID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(CommunityIDKey, communityID))
}

func AddSessionAttribute(ctx context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(SessionIDKey, sessionID))
}

func AddFlowAttribute(ctx context.Context, flowID string) {
	if flowID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(FlowIDKey, flowID))
}

func AddFeatureSetAttribute(ctx context.Context, featureSetID string) {
	if featureSetID == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(FeatureSetIDKey, featureSetID))
}

func AddModelVersionAttribute(ctx context.Context, modelVersion string) {
	if modelVersion == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(ModelVersionKey, modelVersion))
}

func AddRuleVersionAttribute(ctx context.Context, ruleVersion string) {
	if ruleVersion == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(RuleVersionKey, ruleVersion))
}

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

func AddIntAttribute(ctx context.Context, key string, value int64) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Int64(key, value))
}

func AddFloatAttribute(ctx context.Context, key string, value float64) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Float64(key, value))
}

func AddBoolAttribute(ctx context.Context, key string, value bool) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Bool(key, value))
}

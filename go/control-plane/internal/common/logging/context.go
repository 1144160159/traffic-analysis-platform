package logging

import (
	"context"

	"go.uber.org/zap"
)

type contextKey string

const (
	loggerKey            contextKey = "logger"
	contextKeyLogContext contextKey = "log_context"
)

// WithLogger 将logger注入context
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext 从context中获取logger
func FromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return logger
	}
	return zap.L()
}

// WithLogContext 将日志上下文注入context
func WithLogContext(ctx context.Context, lc *LogContext) context.Context {
	return context.WithValue(ctx, contextKeyLogContext, lc)
}

// LogContextFromContext 从context中获取日志上下文
func LogContextFromContext(ctx context.Context) *LogContext {
	if lc, ok := ctx.Value(contextKeyLogContext).(*LogContext); ok {
		return lc
	}
	return &LogContext{}
}

// WithTenantID 添加租户ID到context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.TenantID = tenantID
	return WithLogContext(ctx, &newLC)
}

// WithUserID 添加用户ID到context
func WithUserID(ctx context.Context, userID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.UserID = userID
	return WithLogContext(ctx, &newLC)
}

// WithTraceID 添加TraceID到context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.TraceID = traceID
	return WithLogContext(ctx, &newLC)
}

// WithRequestID 添加RequestID到context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.RequestID = requestID
	return WithLogContext(ctx, &newLC)
}

// WithProbeID 添加ProbeID到context
func WithProbeID(ctx context.Context, probeID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.ProbeID = probeID
	return WithLogContext(ctx, &newLC)
}

// WithRunID 添加RunID到context
func WithRunID(ctx context.Context, runID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.RunID = runID
	return WithLogContext(ctx, &newLC)
}

// L 获取带上下文字段的logger
func L(ctx context.Context) *zap.Logger {
	logger := FromContext(ctx)
	lc := LogContextFromContext(ctx)

	fields := make([]zap.Field, 0)
	for k, v := range lc.ToFields() {
		fields = append(fields, zap.Any(k, v))
	}

	if len(fields) > 0 {
		return logger.With(fields...)
	}
	return logger
}

// S 获取带上下文字段的SugaredLogger
func S(ctx context.Context) *zap.SugaredLogger {
	return L(ctx).Sugar()
}

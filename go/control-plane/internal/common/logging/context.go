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

func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return logger
	}
	return zap.L()
}

func WithLogContext(ctx context.Context, lc *LogContext) context.Context {
	return context.WithValue(ctx, contextKeyLogContext, lc)
}

func LogContextFromContext(ctx context.Context) *LogContext {
	if lc, ok := ctx.Value(contextKeyLogContext).(*LogContext); ok {
		return lc
	}
	return &LogContext{}
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.TenantID = tenantID
	return WithLogContext(ctx, &newLC)
}

func WithUserID(ctx context.Context, userID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.UserID = userID
	return WithLogContext(ctx, &newLC)
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.TraceID = traceID
	return WithLogContext(ctx, &newLC)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.RequestID = requestID
	return WithLogContext(ctx, &newLC)
}

func WithProbeID(ctx context.Context, probeID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.ProbeID = probeID
	return WithLogContext(ctx, &newLC)
}

func WithRunID(ctx context.Context, runID string) context.Context {
	lc := LogContextFromContext(ctx)
	newLC := *lc
	newLC.RunID = runID
	return WithLogContext(ctx, &newLC)
}

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

func S(ctx context.Context) *zap.SugaredLogger {
	return L(ctx).Sugar()
}

package auth

import "context"

// WithTestTenant sets tenant ID in context for integration testing.
func WithTestTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// WithTestProbe sets probe ID in context for integration testing.
func WithTestProbe(ctx context.Context, probeID string) context.Context {
	return context.WithValue(ctx, ProbeIDKey, probeID)
}

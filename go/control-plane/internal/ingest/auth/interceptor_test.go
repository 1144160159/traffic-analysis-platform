////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/interceptor_test.go
////////////////////////////////////////////////////////////////////////////////

package auth

import (
	"testing"
)

func TestExtractTenantFromProbeID(t *testing.T) {
	tests := []struct {
		name     string
		probeID  string
		expected string
	}{
		{
			name:     "valid probe ID with tenant",
			probeID:  "probe-tenant01-001",
			expected: "tenant01",
		},
		{
			name:     "valid probe ID with underscore in tenant",
			probeID:  "probe-tenant_production-002",
			expected: "tenant_production",
		},
		{
			name:     "valid probe ID with dash in tenant",
			probeID:  "probe-tenant-prod-003",
			expected: "tenant-prod",
		},
		{
			name:     "probe ID too short",
			probeID:  "probe",
			expected: "",
		},
		{
			name:     "probe ID without correct prefix",
			probeID:  "agent-tenant01-001",
			expected: "",
		},
		{
			name:     "probe ID with only prefix",
			probeID:  "probe-",
			expected: "",
		},
		{
			name:     "probe ID without probe number",
			probeID:  "probe-tenant01",
			expected: "",
		},
		{
			name:     "empty probe ID",
			probeID:  "",
			expected: "",
		},
		{
			name:     "probe ID with invalid characters in tenant",
			probeID:  "probe-tenant@01-001",
			expected: "",
		},
		{
			name:     "probe ID with numbers in tenant",
			probeID:  "probe-tenant123-001",
			expected: "tenant123",
		},
		{
			name:     "probe ID with uppercase",
			probeID:  "probe-TenantA-001",
			expected: "TenantA",
		},
		{
			name:     "complex tenant ID",
			probeID:  "probe-org1-team2-prod-001",
			expected: "org1-team2-prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTenantFromProbeID(tt.probeID)
			if result != tt.expected {
				t.Errorf("extractTenantFromProbeID(%q) = %q, want %q",
					tt.probeID, result, tt.expected)
			}
		})
	}
}

func TestGetFirstMetadataValue(t *testing.T) {
	// 这个测试需要创建 gRPC metadata，这里只是占位
	// 实际测试应该使用 mock 或集成测试
	t.Skip("Requires gRPC metadata mock")
}

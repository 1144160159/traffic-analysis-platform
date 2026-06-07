package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidUUID(t *testing.T) {
	assert.True(t, IsValidUUID("550e8400-e29b-41d4-a716-446655440000"))
	assert.True(t, IsValidUUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"))
	assert.False(t, IsValidUUID(""))
	assert.False(t, IsValidUUID("not-a-uuid"))
	assert.False(t, IsValidUUID("550e8400-e29b-41d4-a716-44665544000"))  // too short
	assert.False(t, IsValidUUID("550e8400-e29b-41d4-a716-4466554400000")) // too long
	assert.False(t, IsValidUUID("ZZZZZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZZZZZZZZZ"))
}

func TestNormalizeUUID(t *testing.T) {
	s, err := NormalizeUUID("  550E8400-E29B-41D4-A716-446655440000  ")
	assert.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", s)

	_, err = NormalizeUUID("invalid")
	assert.Error(t, err)
}

func TestIsValidPort(t *testing.T) {
	assert.True(t, IsValidPort(1))
	assert.True(t, IsValidPort(80))
	assert.True(t, IsValidPort(443))
	assert.True(t, IsValidPort(65535))
	assert.False(t, IsValidPort(0))
	assert.False(t, IsValidPort(65536))
}

func TestIsValidPortOrZero(t *testing.T) {
	assert.True(t, IsValidPortOrZero(0))
	assert.True(t, IsValidPortOrZero(8080))
	assert.False(t, IsValidPortOrZero(65536))
}

func TestParsePort(t *testing.T) {
	p, err := ParsePort("80")
	assert.NoError(t, err)
	assert.Equal(t, uint32(80), p)
	p, err = ParsePort("65535")
	assert.NoError(t, err)
	assert.Equal(t, uint32(65535), p)
	_, err = ParsePort("0")
	assert.Error(t, err)
	_, err = ParsePort("99999")
	assert.Error(t, err)
	_, err = ParsePort("")
	assert.Error(t, err)
}

func TestIsValidPortRange(t *testing.T) {
	assert.True(t, IsValidPortRange(8000, 9000))
	assert.False(t, IsValidPortRange(9000, 8000))
	assert.False(t, IsValidPortRange(0, 9000))
}

func TestIsValidTimestampMs(t *testing.T) {
	assert.True(t, IsValidTimestampMs(1700000000000)) // 2023
	assert.True(t, IsValidTimestampMs(2500000000000)) // 2049
	assert.False(t, IsValidTimestampMs(0))
	assert.False(t, IsValidTimestampMs(5000000000000)) // year 2128
}

func TestIsValidEnum(t *testing.T) {
	valid := []string{"TCP", "UDP", "ICMP"}
	assert.True(t, IsValidEnum("TCP", valid))
	assert.True(t, IsValidEnum("tcp", valid))
	assert.True(t, IsValidEnum("UDP", valid))
	assert.False(t, IsValidEnum("HTTP", valid))
	assert.False(t, IsValidEnum("", valid))
}

func TestIsValidEnumExact(t *testing.T) {
	valid := []string{"TCP", "UDP", "ICMP"}
	assert.True(t, IsValidEnumExact("TCP", valid))
	assert.False(t, IsValidEnumExact("tcp", valid))
}

func TestIsValidStringLength(t *testing.T) {
	assert.True(t, IsValidStringLength("hello", 1, 10))
	assert.False(t, IsValidStringLength("", 1, 10))
	assert.False(t, IsValidStringLength("hello world this is too long", 1, 10))
}

func TestIsValidTenantID(t *testing.T) {
	assert.True(t, IsValidTenantID("tenant-1"))
	assert.True(t, IsValidTenantID("default"))
	assert.True(t, IsValidTenantID("org_123"))
	assert.False(t, IsValidTenantID(""))
	assert.False(t, IsValidTenantID("tenant with spaces"))
	assert.False(t, IsValidTenantID("very-long-tenant-id-that-exceeds-the-maximum-allowed-length-of-64-chars"))
}

func TestValidateRequired(t *testing.T) {
	fields := map[string]string{"name": "test", "email": ""}
	errs := ValidateRequired(fields)
	assert.NotNil(t, errs)
	assert.Equal(t, 1, len(errs))

	fields2 := map[string]string{"name": "test", "email": "x@y.com"}
	assert.Nil(t, ValidateRequired(fields2))
}

package unit

import (
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func TestNewError(t *testing.T) {
	err := errors.New(errors.ErrCodeInvalidRequest, "tenant not found")
	if err == nil { t.Fatal("nil") }
	if err.Error() == "" { t.Error("empty msg") }
}

func TestNewfError(t *testing.T) {
	err := errors.Newf(errors.ErrCodeTenantNotFound, "tenant %s not found", "t1")
	if err.Code != errors.ErrCodeTenantNotFound { t.Errorf("code=%s", err.Code) }
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code errors.ErrorCode
	}{
		{"InvalidRequest", errors.ErrCodeInvalidRequest},
		{"PermissionDenied", errors.ErrCodePermissionDenied},
		{"TenantNotFound", errors.ErrCodeTenantNotFound},
		{"QuotaExceeded", errors.ErrCodeQuotaExceeded},
		{"TokenExpired", errors.ErrCodeTokenExpired},
		{"MTLSRequired", errors.ErrCodeMTLSRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.code, "test")
			if err.Code != tt.code { t.Errorf("code=%s want=%s", err.Code, tt.code) }
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	inner := errors.New(errors.ErrCodeTenantNotFound, "inner")
	outer := errors.Wrap(inner, errors.ErrCodeInvalidRequest, "wrap")
	if outer.Unwrap() != inner { t.Error("Unwrap failed") }
}

func BenchmarkErrorCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = errors.New(errors.ErrCodeInvalidRequest, "bench")
	}
}

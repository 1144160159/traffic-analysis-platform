package errors

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

type AppError struct {
	Code     ErrorCode              `json:"code"`
	Message  string                 `json:"message"`
	Details  map[string]interface{} `json:"details,omitempty"`
	Cause    error                  `json:"-"`
	Stack    string                 `json:"-"`
	TraceID  string                 `json:"trace_id,omitempty"`
	TenantID string                 `json:"tenant_id,omitempty"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

func (e *AppError) HTTPStatus() int {
	return e.Code.HTTPStatus()
}

func (e *AppError) IsRetryable() bool {
	return e.Code.IsRetryable()
}

func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

func (e *AppError) WithTraceID(traceID string) *AppError {
	e.TraceID = traceID
	return e
}

func (e *AppError) WithTenantID(tenantID string) *AppError {
	e.TenantID = tenantID
	return e
}

func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Stack:   captureStack(2),
	}
}

func Newf(code ErrorCode, format string, args ...interface{}) *AppError {
	return &AppError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Stack:   captureStack(2),
	}
}

func Wrap(err error, code ErrorCode, message string) *AppError {
	if err == nil {
		return nil
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return &AppError{
			Code:    code,
			Message: message,
			Cause:   err,
			Stack:   appErr.Stack,
			Details: appErr.Details,
		}
	}

	return &AppError{
		Code:    code,
		Message: message,
		Cause:   err,
		Stack:   captureStack(2),
	}
}

func Wrapf(err error, code ErrorCode, format string, args ...interface{}) *AppError {
	if err == nil {
		return nil
	}
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

func GetCode(err error) ErrorCode {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ErrCodeInternal
}

func GetHTTPStatus(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus()
	}
	return 500
}

func IsCode(err error, code ErrorCode) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}

func IsRetryableError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.IsRetryable()
	}
	return false
}

func captureStack(skip int) string {
	var builder strings.Builder
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip+1, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "runtime/") {
			builder.WriteString(fmt.Sprintf("%s:%d %s\n", frame.File, frame.Line, frame.Function))
		}
		if !more {
			break
		}
	}

	return builder.String()
}

func ErrUnauthorized(message string) *AppError {
	return New(ErrCodeUnauthorized, message)
}

func ErrPermissionDenied(message string) *AppError {
	return New(ErrCodePermissionDenied, message)
}

func ErrInvalidRequest(message string) *AppError {
	return New(ErrCodeInvalidRequest, message)
}

func ErrNotFound(code ErrorCode, message string) *AppError {
	return New(code, message)
}

func ErrInternal(message string) *AppError {
	return New(ErrCodeInternal, message)
}

func ErrDatabase(err error, message string) *AppError {
	return Wrap(err, ErrCodeDatabaseError, message)
}

func ErrCache(err error, message string) *AppError {
	return Wrap(err, ErrCodeCacheError, message)
}

func ErrKafka(err error, message string) *AppError {
	return Wrap(err, ErrCodeKafkaError, message)
}

func ErrQuotaExceeded(tenantID string) *AppError {
	return New(ErrCodeQuotaExceeded, "quota exceeded").WithTenantID(tenantID)
}

func ErrTimeout(operation string) *AppError {
	return Newf(ErrCodeTimeout, "operation timed out: %s", operation)
}

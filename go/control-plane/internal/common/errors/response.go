////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/errors/response.go
////////////////////////////////////////////////////////////////////////////////

package errors

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// ErrorResponse API错误响应
type ErrorResponse struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Path      string                 `json:"path,omitempty"`
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(err error, traceID, path string) *ErrorResponse {
	resp := &ErrorResponse{
		Code:      string(ErrCodeInternal),
		Message:   "Internal server error",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceID:   traceID,
		Path:      path,
	}

	var appErr *AppError
	if AsAppError(err, &appErr) {
		resp.Code = string(appErr.Code)
		resp.Message = appErr.Message
		resp.Details = appErr.Details
		if appErr.TraceID != "" {
			resp.TraceID = appErr.TraceID
		}
	} else if err != nil {
		resp.Message = err.Error()
	}

	return resp
}

// AsAppError 类型断言辅助函数
func AsAppError(err error, target **AppError) bool {
	return errors.As(err, target)
}

// WriteError 写入错误响应
func WriteError(w http.ResponseWriter, err error, traceID, path string) {
	var appErr *AppError

	statusCode := http.StatusInternalServerError
	if AsAppError(err, &appErr) {
		statusCode = appErr.HTTPStatus()
	}

	resp := NewErrorResponse(err, traceID, path)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// WriteErrorWithStatus 写入指定状态码的错误响应
func WriteErrorWithStatus(w http.ResponseWriter, statusCode int, code ErrorCode, message, traceID, path string) {
	resp := &ErrorResponse{
		Code:      string(code),
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceID:   traceID,
		Path:      path,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// SuccessResponse 成功响应
type SuccessResponse struct {
	Data      interface{} `json:"data,omitempty"`
	Message   string      `json:"message,omitempty"`
	Timestamp string      `json:"timestamp"`
	TraceID   string      `json:"trace_id,omitempty"`
}

// WriteSuccess 写入成功响应
func WriteSuccess(w http.ResponseWriter, data interface{}, traceID string) {
	resp := &SuccessResponse{
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceID:   traceID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// WriteCreated 写入创建成功响应
func WriteCreated(w http.ResponseWriter, data interface{}, traceID string) {
	resp := &SuccessResponse{
		Data:      data,
		Message:   "Created successfully",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceID:   traceID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// WriteNoContent 写入无内容响应
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// PaginatedResponse 分页响应
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
	Timestamp  string      `json:"timestamp"`
	TraceID    string      `json:"trace_id,omitempty"`
}

// Pagination 分页信息
type Pagination struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

// WritePaginated 写入分页响应
func WritePaginated(w http.ResponseWriter, data interface{}, total int64, limit, offset int, traceID string) {
	resp := &PaginatedResponse{
		Data: data,
		Pagination: Pagination{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: int64(offset+limit) < total,
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceID:   traceID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

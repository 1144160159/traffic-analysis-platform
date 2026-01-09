////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/httpx/response.go
// 修复版：移除重复的 Timeout 函数，保留在 timeout.go 中
////////////////////////////////////////////////////////////////////////////////

package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Response 统一响应结构
type Response struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *ErrorInfo  `json:"error,omitempty"`
	Meta      *MetaInfo   `json:"meta,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// MetaInfo 元信息
type MetaInfo struct {
	RequestID string    `json:"request_id,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
	Page      *PageInfo `json:"page,omitempty"`
}

// PageInfo 分页信息
type PageInfo struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

// ResponseWriter 响应写入器
type ResponseWriter struct {
	w         http.ResponseWriter
	ctx       context.Context
	requestID string
	traceID   string
}

// NewResponseWriter 创建响应写入器
func NewResponseWriter(w http.ResponseWriter, ctx context.Context) *ResponseWriter {
	return &ResponseWriter{
		w:         w,
		ctx:       ctx,
		requestID: GetRequestID(ctx),
		traceID:   GetTraceID(ctx),
	}
}

// Success 成功响应
func (rw *ResponseWriter) Success(data interface{}) {
	rw.write(http.StatusOK, &Response{
		Success:   true,
		Data:      data,
		Meta:      rw.meta(nil),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Created 创建成功响应
func (rw *ResponseWriter) Created(data interface{}) {
	rw.write(http.StatusCreated, &Response{
		Success:   true,
		Data:      data,
		Meta:      rw.meta(nil),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// NoContent 无内容响应
func (rw *ResponseWriter) NoContent() {
	rw.w.WriteHeader(http.StatusNoContent)
}

// Paginated 分页响应
func (rw *ResponseWriter) Paginated(data interface{}, total int64, limit, offset int) {
	rw.write(http.StatusOK, &Response{
		Success: true,
		Data:    data,
		Meta: rw.meta(&PageInfo{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: int64(offset+limit) < total,
		}),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Error 错误响应
func (rw *ResponseWriter) Error(statusCode int, code, message string, details map[string]interface{}) {
	rw.write(statusCode, &Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta:      rw.meta(nil),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorFromAppError 从AppError创建错误响应
func (rw *ResponseWriter) ErrorFromAppError(err error) {
	var code string = "INTERNAL_ERROR"
	var message string = "Internal server error"
	var statusCode int = http.StatusInternalServerError
	var details map[string]interface{}

	// 尝试解析为AppError
	if appErr, ok := err.(interface {
		HTTPStatus() int
	}); ok {
		statusCode = appErr.HTTPStatus()
	}

	if appErr, ok := err.(interface {
		Error() string
	}); ok {
		message = appErr.Error()
	}

	rw.Error(statusCode, code, message, details)
}

func (rw *ResponseWriter) meta(page *PageInfo) *MetaInfo {
	return &MetaInfo{
		RequestID: rw.requestID,
		TraceID:   rw.traceID,
		Page:      page,
	}
}

func (rw *ResponseWriter) write(statusCode int, resp *Response) {
	rw.w.Header().Set("Content-Type", "application/json")
	rw.w.WriteHeader(statusCode)
	json.NewEncoder(rw.w).Encode(resp)
}

// 便捷函数

// JSON 写入JSON响应
func JSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// JSONSuccess 写入成功JSON响应
func JSONSuccess(w http.ResponseWriter, ctx context.Context, data interface{}) {
	rw := NewResponseWriter(w, ctx)
	rw.Success(data)
}

// JSONCreated 写入创建成功JSON响应
func JSONCreated(w http.ResponseWriter, ctx context.Context, data interface{}) {
	rw := NewResponseWriter(w, ctx)
	rw.Created(data)
}

// JSONError 写入错误JSON响应
func JSONError(w http.ResponseWriter, ctx context.Context, statusCode int, code, message string) {
	rw := NewResponseWriter(w, ctx)
	rw.Error(statusCode, code, message, nil)
}

// JSONPaginated 写入分页JSON响应
func JSONPaginated(w http.ResponseWriter, ctx context.Context, data interface{}, total int64, limit, offset int) {
	rw := NewResponseWriter(w, ctx)
	rw.Paginated(data, total, limit, offset)
}

// 注意：Timeout 中间件已移至 timeout.go 文件中
// 请使用 TimeoutWithConfig 函数

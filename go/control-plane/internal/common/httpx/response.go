package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Response struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *ErrorInfo  `json:"error,omitempty"`
	Meta      *MetaInfo   `json:"meta,omitempty"`
	Timestamp string      `json:"timestamp"`
}

type ErrorInfo struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type MetaInfo struct {
	RequestID string    `json:"request_id,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
	Page      *PageInfo `json:"page,omitempty"`
}

type PageInfo struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

type ResponseWriter struct {
	w         http.ResponseWriter
	ctx       context.Context
	requestID string
	traceID   string
}

func NewResponseWriter(w http.ResponseWriter, ctx context.Context) *ResponseWriter {
	return &ResponseWriter{
		w:         w,
		ctx:       ctx,
		requestID: GetRequestID(ctx),
		traceID:   GetTraceID(ctx),
	}
}

func (rw *ResponseWriter) Success(data interface{}) {
	rw.write(http.StatusOK, &Response{
		Success:   true,
		Data:      data,
		Meta:      rw.meta(nil),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (rw *ResponseWriter) Created(data interface{}) {
	rw.write(http.StatusCreated, &Response{
		Success:   true,
		Data:      data,
		Meta:      rw.meta(nil),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (rw *ResponseWriter) NoContent() {
	rw.w.WriteHeader(http.StatusNoContent)
}

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

func (rw *ResponseWriter) ErrorFromAppError(err error) {
	var code string = "INTERNAL_ERROR"
	var message string = "Internal server error"
	var statusCode int = http.StatusInternalServerError
	var details map[string]interface{}

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

func JSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func JSONSuccess(w http.ResponseWriter, ctx context.Context, data interface{}) {
	rw := NewResponseWriter(w, ctx)
	rw.Success(data)
}

func JSONCreated(w http.ResponseWriter, ctx context.Context, data interface{}) {
	rw := NewResponseWriter(w, ctx)
	rw.Created(data)
}

func JSONError(w http.ResponseWriter, ctx context.Context, statusCode int, code, message string) {
	rw := NewResponseWriter(w, ctx)
	rw.Error(statusCode, code, message, nil)
}

func JSONPaginated(w http.ResponseWriter, ctx context.Context, data interface{}, total int64, limit, offset int) {
	rw := NewResponseWriter(w, ctx)
	rw.Paginated(data, total, limit, offset)
}

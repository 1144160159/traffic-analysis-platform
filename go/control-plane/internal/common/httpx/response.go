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
	Code                      string                 `json:"code"`
	Message                   string                 `json:"message"`
	TraceID                   string                 `json:"trace_id"`
	Retryable                 bool                   `json:"retryable"`
	FieldErrors               []FieldError           `json:"field_errors,omitempty"`
	OperationID               string                 `json:"operation_id,omitempty"`
	CurrentRevision           *int64                 `json:"current_revision,omitempty"`
	ProjectionStatus          string                 `json:"projection_status,omitempty"`
	StatusURL                 string                 `json:"status_url,omitempty"`
	ExpectedConsistencyWindow string                 `json:"expected_consistency_window,omitempty"`
	Details                   map[string]interface{} `json:"details,omitempty"`
}

// FieldError identifies one invalid request field without requiring clients to
// parse a localized message.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorOptions carries the machine-readable business result fields shared by
// HTTP errors, logs, audit records and asynchronous status responses. Zero
// values remain wire-compatible with legacy callers.
type ErrorOptions struct {
	Retryable                 bool
	FieldErrors               []FieldError
	OperationID               string
	CurrentRevision           *int64
	ProjectionStatus          string
	StatusURL                 string
	ExpectedConsistencyWindow string
	Details                   map[string]interface{}
}

type MetaInfo struct {
	RequestID string    `json:"request_id,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
	Page      *PageInfo `json:"page,omitempty"`
}

// ContractMeta is the versioned data snapshot metadata required by alignment
// feature contracts. It is intentionally separate from the legacy MetaInfo so
// existing responses remain wire-compatible during the additive rollout.
type ContractMeta struct {
	ContractVersion  int               `json:"contract_version"`
	SchemaVersion    int               `json:"schema_version"`
	SnapshotID       string            `json:"snapshot_id"`
	AsOf             string            `json:"as_of"`
	GeneratedAt      string            `json:"generated_at"`
	TraceID          string            `json:"trace_id"`
	ResultCode       string            `json:"result_code"`
	OperationID      string            `json:"operation_id,omitempty"`
	TenantID         string            `json:"tenant_id,omitempty"`
	Partial          bool              `json:"partial"`
	MissingSections  []string          `json:"missing_sections"`
	SourceWatermarks map[string]string `json:"source_watermarks"`
	ProjectionStatus string            `json:"projection_status,omitempty"`
}

type ContractResponse struct {
	Success   bool         `json:"success"`
	Data      interface{}  `json:"data"`
	Meta      ContractMeta `json:"meta"`
	Error     *ErrorInfo   `json:"error"`
	Timestamp string       `json:"timestamp"`
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

func (rw *ResponseWriter) Accepted(data interface{}) {
	rw.write(http.StatusAccepted, &Response{
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
	rw.ContractError(statusCode, code, message, ErrorOptions{
		Retryable: defaultRetryableStatus(statusCode),
		Details:   details,
	})
}

// ContractError writes the additive F-COMMON-004 error shape. HTTP status
// continues to carry transport semantics while code and the structured fields
// carry the business result; a 2xx response is never emitted from this path.
func (rw *ResponseWriter) ContractError(statusCode int, code, message string, options ErrorOptions) {
	rw.write(statusCode, &Response{
		Success: false,
		Error: &ErrorInfo{
			Code:                      code,
			Message:                   message,
			TraceID:                   rw.traceID,
			Retryable:                 options.Retryable,
			FieldErrors:               options.FieldErrors,
			OperationID:               options.OperationID,
			CurrentRevision:           options.CurrentRevision,
			ProjectionStatus:          options.ProjectionStatus,
			StatusURL:                 options.StatusURL,
			ExpectedConsistencyWindow: options.ExpectedConsistencyWindow,
			Details:                   options.Details,
		},
		Meta:      rw.meta(nil),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func defaultRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
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

func JSONAccepted(w http.ResponseWriter, ctx context.Context, data interface{}) {
	rw := NewResponseWriter(w, ctx)
	rw.Accepted(data)
}

func JSONContractSuccess(w http.ResponseWriter, ctx context.Context, data interface{}, meta ContractMeta) {
	jsonContract(w, ctx, http.StatusOK, data, meta)
}

func JSONContractCreated(w http.ResponseWriter, ctx context.Context, data interface{}, meta ContractMeta) {
	jsonContract(w, ctx, http.StatusCreated, data, meta)
}

func JSONContractAccepted(w http.ResponseWriter, ctx context.Context, data interface{}, meta ContractMeta) {
	jsonContract(w, ctx, http.StatusAccepted, data, meta)
}

func jsonContract(w http.ResponseWriter, ctx context.Context, statusCode int, data interface{}, meta ContractMeta) {
	if meta.ContractVersion == 0 {
		meta.ContractVersion = 1
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = 1
	}
	if meta.AsOf == "" {
		meta.AsOf = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if meta.GeneratedAt == "" {
		meta.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if meta.TraceID == "" {
		meta.TraceID = GetTraceID(ctx)
	}
	if meta.ResultCode == "" {
		meta.ResultCode = "SUCCESS"
	}
	if meta.TenantID == "" {
		meta.TenantID = GetTenantID(ctx)
	}
	if meta.MissingSections == nil {
		meta.MissingSections = []string{}
	}
	if meta.SourceWatermarks == nil {
		meta.SourceWatermarks = map[string]string{}
	}
	JSON(w, statusCode, &ContractResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Error:     nil,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func JSONError(w http.ResponseWriter, ctx context.Context, statusCode int, code, message string) {
	rw := NewResponseWriter(w, ctx)
	rw.Error(statusCode, code, message, nil)
}

func JSONContractError(w http.ResponseWriter, ctx context.Context, statusCode int, code, message string, options ErrorOptions) {
	rw := NewResponseWriter(w, ctx)
	rw.ContractError(statusCode, code, message, options)
}

func JSONPaginated(w http.ResponseWriter, ctx context.Context, data interface{}, total int64, limit, offset int) {
	rw := NewResponseWriter(w, ctx)
	rw.Paginated(data, total, limit, offset)
}

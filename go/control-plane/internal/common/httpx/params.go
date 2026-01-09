////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/httpx/params.go
// 新增文件：HTTP 参数解析工具
// 功能：
// 1. 统一 UUID 解析逻辑（修复 #10）
// 2. 统一字符串、整数、布尔参数解析
// 3. 提供友好的错误提示
////////////////////////////////////////////////////////////////////////////////

package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// =============================================================================
// UUID 解析（修复 #10）
// =============================================================================

// ParseUUIDParam 从路径参数中解析 UUID
// 修复 #10：统一处理空字符串和格式错误
func ParseUUIDParam(r *http.Request, paramName string) (uuid.UUID, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

	// 检查是否为空
	if paramValue == "" {
		return uuid.Nil, errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	// 解析 UUID
	id, err := uuid.Parse(paramValue)
	if err != nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be a valid UUID (e.g., 123e4567-e89b-12d3-a456-426614174000)", paramName)
	}

	// 检查是否为 Nil UUID（某些情况下需要拒绝）
	if id == uuid.Nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"%s cannot be nil UUID", paramName)
	}

	return id, nil
}

// ParseOptionalUUIDParam 从路径参数中解析可选的 UUID
// 如果参数不存在或为空，返回 uuid.Nil 而不是错误
func ParseOptionalUUIDParam(r *http.Request, paramName string) (uuid.UUID, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

	// 空值返回 Nil UUID
	if paramValue == "" {
		return uuid.Nil, nil
	}

	// 解析 UUID
	id, err := uuid.Parse(paramValue)
	if err != nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be a valid UUID", paramName)
	}

	return id, nil
}

// ParseUUIDFromQuery 从查询参数中解析 UUID
func ParseUUIDFromQuery(r *http.Request, paramName string) (uuid.UUID, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return uuid.Nil, errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	id, err := uuid.Parse(paramValue)
	if err != nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be a valid UUID", paramName)
	}

	if id == uuid.Nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"%s cannot be nil UUID", paramName)
	}

	return id, nil
}

// ParseOptionalUUIDFromQuery 从查询参数中解析可选的 UUID
func ParseOptionalUUIDFromQuery(r *http.Request, paramName string) (uuid.UUID, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return uuid.Nil, nil
	}

	id, err := uuid.Parse(paramValue)
	if err != nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be a valid UUID", paramName)
	}

	return id, nil
}

// ValidateUUID 验证 UUID 字符串（不解析）
func ValidateUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

// =============================================================================
// 字符串参数解析
// =============================================================================

// ParseStringParam 从路径参数中解析字符串
func ParseStringParam(r *http.Request, paramName string) (string, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

	if paramValue == "" {
		return "", errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	return paramValue, nil
}

// ParseOptionalStringParam 从路径参数中解析可选字符串
func ParseOptionalStringParam(r *http.Request, paramName string) string {
	vars := mux.Vars(r)
	return vars[paramName]
}

// ParseStringFromQuery 从查询参数中解析字符串
func ParseStringFromQuery(r *http.Request, paramName string) (string, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return "", errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	return paramValue, nil
}

// ParseOptionalStringFromQuery 从查询参数中解析可选字符串
func ParseOptionalStringFromQuery(r *http.Request, paramName, defaultValue string) string {
	paramValue := r.URL.Query().Get(paramName)
	if paramValue == "" {
		return defaultValue
	}
	return paramValue
}

// =============================================================================
// 整数参数解析
// =============================================================================

// ParseIntParam 从路径参数中解析整数
func ParseIntParam(r *http.Request, paramName string) (int, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

	if paramValue == "" {
		return 0, errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	value, err := strconv.Atoi(paramValue)
	if err != nil {
		return 0, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be an integer", paramName)
	}

	return value, nil
}

// ParseIntFromQuery 从查询参数中解析整数
func ParseIntFromQuery(r *http.Request, paramName string, defaultValue int) (int, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(paramValue)
	if err != nil {
		return 0, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be an integer", paramName)
	}

	return value, nil
}

// ParseInt64FromQuery 从查询参数中解析 int64
func ParseInt64FromQuery(r *http.Request, paramName string, defaultValue int64) (int64, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseInt(paramValue, 10, 64)
	if err != nil {
		return 0, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be a valid integer", paramName)
	}

	return value, nil
}

// =============================================================================
// 布尔参数解析
// =============================================================================

// ParseBoolFromQuery 从查询参数中解析布尔值
func ParseBoolFromQuery(r *http.Request, paramName string, defaultValue bool) (bool, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(paramValue)
	if err != nil {
		return false, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be true or false", paramName)
	}

	return value, nil
}

// =============================================================================
// 分页参数解析
// =============================================================================

// PaginationParams 分页参数
type PaginationParams struct {
	Limit  int
	Offset int
}

// ParsePaginationParams 解析分页参数
func ParsePaginationParams(r *http.Request, defaultLimit, maxLimit int) (*PaginationParams, error) {
	limit, err := ParseIntFromQuery(r, "limit", defaultLimit)
	if err != nil {
		return nil, err
	}

	offset, err := ParseIntFromQuery(r, "offset", 0)
	if err != nil {
		return nil, err
	}

	// 验证范围
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	return &PaginationParams{
		Limit:  limit,
		Offset: offset,
	}, nil
}

// =============================================================================
// 时间参数解析
// =============================================================================

// ParseTimeFromQuery 从查询参数中解析时间（RFC3339 格式）
func ParseTimeFromQuery(r *http.Request, paramName string) (time.Time, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return time.Time{}, errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	t, err := time.Parse(time.RFC3339, paramValue)
	if err != nil {
		return time.Time{}, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be RFC3339 (e.g., 2006-01-02T15:04:05Z07:00)", paramName)
	}

	return t, nil
}

// ParseOptionalTimeFromQuery 从查询参数中解析可选时间
func ParseOptionalTimeFromQuery(r *http.Request, paramName string) (*time.Time, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, paramValue)
	if err != nil {
		return nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be RFC3339", paramName)
	}

	return &t, nil
}

// ParseUnixTimestampFromQuery 从查询参数中解析 Unix 时间戳（秒）
func ParseUnixTimestampFromQuery(r *http.Request, paramName string) (time.Time, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return time.Time{}, errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	timestamp, err := strconv.ParseInt(paramValue, 10, 64)
	if err != nil {
		return time.Time{}, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be Unix timestamp", paramName)
	}

	return time.Unix(timestamp, 0), nil
}

// =============================================================================
// 数组参数解析
// =============================================================================

// ParseStringArrayFromQuery 从查询参数中解析字符串数组（逗号分隔）
func ParseStringArrayFromQuery(r *http.Request, paramName string) []string {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return []string{}
	}

	// 简单分割（不处理空格）
	parts := splitAndTrim(paramValue, ",")
	return parts
}

// splitAndTrim 分割并去除空格
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return []string{}
	}

	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString 简单字符串分割（避免导入 strings）
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}

	var result []string
	start := 0

	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}

	result = append(result, s[start:])
	return result
}

// trimSpace 去除字符串两端空格
func trimSpace(s string) string {
	start := 0
	end := len(s)

	// 去除前导空格
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// 去除尾部空格
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func ParseUUIDParam(r *http.Request, paramName string) (uuid.UUID, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

	if paramValue == "" {
		return uuid.Nil, errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	id, err := uuid.Parse(paramValue)
	if err != nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"Invalid %s format: must be a valid UUID (e.g., 123e4567-e89b-12d3-a456-426614174000)", paramName)
	}

	if id == uuid.Nil {
		return uuid.Nil, errors.Newf(errors.ErrCodeInvalidParameter,
			"%s cannot be nil UUID", paramName)
	}

	return id, nil
}

func ParseOptionalUUIDParam(r *http.Request, paramName string) (uuid.UUID, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

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

func ValidateUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func ParseStringParam(r *http.Request, paramName string) (string, error) {
	vars := mux.Vars(r)
	paramValue := vars[paramName]

	if paramValue == "" {
		return "", errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	return paramValue, nil
}

func ParseOptionalStringParam(r *http.Request, paramName string) string {
	vars := mux.Vars(r)
	return vars[paramName]
}

func ParseStringFromQuery(r *http.Request, paramName string) (string, error) {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return "", errors.Newf(errors.ErrCodeMissingParameter,
			"%s is required", paramName)
	}

	return paramValue, nil
}

func ParseOptionalStringFromQuery(r *http.Request, paramName, defaultValue string) string {
	paramValue := r.URL.Query().Get(paramName)
	if paramValue == "" {
		return defaultValue
	}
	return paramValue
}

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

type PaginationParams struct {
	Limit  int
	Offset int
}

func ParsePaginationParams(r *http.Request, defaultLimit, maxLimit int) (*PaginationParams, error) {
	limit, err := ParseIntFromQuery(r, "limit", defaultLimit)
	if err != nil {
		return nil, err
	}

	offset, err := ParseIntFromQuery(r, "offset", 0)
	if err != nil {
		return nil, err
	}

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

func ParseStringArrayFromQuery(r *http.Request, paramName string) []string {
	paramValue := r.URL.Query().Get(paramName)

	if paramValue == "" {
		return []string{}
	}

	parts := splitAndTrim(paramValue, ",")
	return parts
}

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

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

package opensearchbulk

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type ItemFailure struct {
	Operation string
	ID        string
	Status    int
	Reason    string
	Retryable bool
}

type PartialFailureError struct {
	Expected int
	Received int
	Failures []ItemFailure
}

func (e *PartialFailureError) Error() string {
	parts := make([]string, 0, min(len(e.Failures), 10))
	for index, failure := range e.Failures {
		if index == 10 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s/%s status=%d reason=%s",
			failure.Operation, failure.ID, failure.Status, failure.Reason))
	}
	return fmt.Sprintf("OpenSearch bulk incomplete: expected=%d received=%d failures=%d [%s]",
		e.Expected, e.Received, len(e.Failures), strings.Join(parts, "; "))
}

// FailedIDs is deliberately empty when the bulk response is incomplete: the
// caller must conservatively enqueue every submitted document in that case.
func (e *PartialFailureError) FailedIDs() []string {
	if e == nil || e.Received != e.Expected {
		return nil
	}
	ids := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		if failure.ID == "" {
			return nil
		}
		ids = append(ids, failure.ID)
	}
	return ids
}

type response struct {
	Errors bool                         `json:"errors"`
	Items  []map[string]json.RawMessage `json:"items"`
}

type itemResult struct {
	ID     string          `json:"_id"`
	Status int             `json:"status"`
	Error  json.RawMessage `json:"error"`
}

// DecodeSuccess accepts a bulk response only when it contains exactly one
// successful item acknowledgement per submitted document. HTTP 2xx and
// errors=false alone are insufficient.
func DecodeSuccess(reader io.Reader, expected int) error {
	var result response
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("decode OpenSearch bulk response: %w", err)
	}
	failures := make([]ItemFailure, 0)
	for _, rawItem := range result.Items {
		if len(rawItem) != 1 {
			failures = append(failures, ItemFailure{Operation: "unknown", Reason: "invalid item envelope"})
			continue
		}
		for operation, rawResult := range rawItem {
			var item itemResult
			if err := json.Unmarshal(rawResult, &item); err != nil {
				failures = append(failures, ItemFailure{Operation: operation, Reason: "invalid item response"})
				continue
			}
			if item.Status >= 300 || hasJSONValue(item.Error) {
				failures = append(failures, ItemFailure{
					Operation: operation,
					ID:        item.ID,
					Status:    item.Status,
					Reason:    compactReason(item.Error),
					Retryable: item.Status == 429 || item.Status == 502 || item.Status == 503 || item.Status == 504,
				})
			}
		}
	}
	if result.Errors || len(result.Items) != expected || len(failures) > 0 {
		return &PartialFailureError{Expected: expected, Received: len(result.Items), Failures: failures}
	}
	return nil
}

func hasJSONValue(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null" && value != "{}"
}

func compactReason(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "unspecified item failure"
	}
	if len(value) > 512 {
		return value[:512]
	}
	return value
}

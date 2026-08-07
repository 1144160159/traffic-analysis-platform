package opensearchbulk

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeSuccessRejectsPartialFailure(t *testing.T) {
	payload := `{"errors":true,"items":[
		{"index":{"_id":"evt-ok","status":201}},
		{"index":{"_id":"evt-bad","status":429,"error":{"type":"rejected_execution_exception"}}}
	]}`
	err := DecodeSuccess(strings.NewReader(payload), 2)
	var partial *PartialFailureError
	if !errors.As(err, &partial) {
		t.Fatalf("error=%v want PartialFailureError", err)
	}
	if len(partial.Failures) != 1 || partial.Failures[0].ID != "evt-bad" || !partial.Failures[0].Retryable {
		t.Fatalf("unexpected failures: %#v", partial.Failures)
	}
}

func TestDecodeSuccessRejectsMissingAcknowledgement(t *testing.T) {
	payload := `{"errors":false,"items":[{"index":{"_id":"evt-only","status":201}}]}`
	if err := DecodeSuccess(strings.NewReader(payload), 2); err == nil {
		t.Fatal("missing bulk item acknowledgement must fail")
	}
}

func TestDecodeSuccessAcceptsCompleteBatch(t *testing.T) {
	payload := `{"errors":false,"items":[
		{"index":{"_id":"evt-1","status":201}},
		{"index":{"_id":"evt-2","status":200}}
	]}`
	if err := DecodeSuccess(strings.NewReader(payload), 2); err != nil {
		t.Fatal(err)
	}
}

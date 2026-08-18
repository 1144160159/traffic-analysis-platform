package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelRollbackRequestStrictContract(t *testing.T) {
	handler := &Handler{config: DefaultHandlerConfig()}

	valid := httptest.NewRequest("POST", "/", strings.NewReader(`{
		"action_id":"11111111-1111-4111-8111-111111111111",
		"reason":"candidate crossed the rollback threshold",
		"expected_active_version":"model-v2",
		"expected_active_revision":2
	}`))
	var request ModelRollbackRequest
	if err := handler.decodeJSONStrict(valid, &request); err != nil {
		t.Fatalf("valid rollback request rejected: %v", err)
	}
	payload := request.payload()
	if payload["expected_active_revision"] != int64(2) || payload["expected_active_version"] != "model-v2" {
		t.Fatalf("rollback payload lost optimistic-concurrency identity: %#v", payload)
	}

	unknown := httptest.NewRequest("POST", "/", strings.NewReader(`{
		"reason":"candidate crossed the rollback threshold",
		"expected_active_version":"model-v2",
		"expected_active_revision":2,
		"target":"a-different-model"
	}`))
	if err := handler.decodeJSONStrict(unknown, &request); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("rollback request accepted an unbound target field: %v", err)
	}

	trailing := httptest.NewRequest("POST", "/", strings.NewReader(`{} {}`))
	if err := handler.decodeJSONStrict(trailing, &request); err == nil || !strings.Contains(err.Error(), "exactly one JSON value") {
		t.Fatalf("rollback request accepted trailing JSON: %v", err)
	}
}

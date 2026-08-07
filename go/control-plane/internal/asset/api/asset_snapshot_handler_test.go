package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestAssetDetailSnapshotRouteIsFailClosedWhenDisabled(t *testing.T) {
	handler := NewHTTPHandler(nil, zap.NewNop())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/assets/20000000-0000-4000-8000-000000000001/snapshot", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Success || response.Error.Code != "FEATURE_DISABLED" {
		t.Fatalf("response=%s", recorder.Body.String())
	}
}

func TestAssetDetailSnapshotRouteRejectsMutationMethodsBeforeDispatch(t *testing.T) {
	handler := &HTTPHandler{detailSnapshotV1Enabled: true, logger: zap.NewNop()}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assets/20000000-0000-4000-8000-000000000001/snapshot", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

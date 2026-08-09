package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestReadinessCheckFailsWhenKafkaConsumerIsUnhealthy(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetConsumerHealthCheck(func(context.Context) error { return errors.New("kafka fetch failed 3 consecutive times: EOF") })
	recorder := httptest.NewRecorder()

	handler.ReadinessCheck(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "KAFKA_NOT_READY") {
		t.Fatalf("response should identify Kafka readiness failure: %s", recorder.Body.String())
	}
}

func TestReadinessCheckFailsWhenNamedProjectionDependencyIsUnhealthy(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.AddReadinessCheck("campaign_target_projection", func(context.Context) error {
		return errors.New("opensearch write alias is absent")
	})
	recorder := httptest.NewRecorder()

	handler.ReadinessCheck(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "DEPENDENCY_NOT_READY") ||
		!strings.Contains(recorder.Body.String(), "campaign_target_projection") ||
		!strings.Contains(recorder.Body.String(), "opensearch write alias is absent") {
		t.Fatalf("response should identify the failed named dependency: %s", recorder.Body.String())
	}
}

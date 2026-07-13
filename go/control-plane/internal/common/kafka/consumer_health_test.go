package kafka

import (
	"errors"
	"testing"
)

func TestConsumerHealthCheckTracksPersistentFetchFailuresAndRecovery(t *testing.T) {
	consumer := &Consumer{}
	for range 2 {
		consumer.recordFetchFailure(errors.New("EOF"))
	}
	if err := consumer.HealthCheck(); err != nil {
		t.Fatalf("two transient failures should remain ready: %v", err)
	}

	consumer.recordFetchFailure(errors.New("EOF"))
	if err := consumer.HealthCheck(); err == nil {
		t.Fatal("three consecutive failures should fail readiness")
	}
	metrics := consumer.GetMetrics()
	if metrics.ConsecutiveFetchFailures != 3 || metrics.LastFetchErrorUnix <= 0 {
		t.Fatalf("unexpected health metrics: %+v", metrics)
	}

	consumer.recordFetchSuccess()
	if err := consumer.HealthCheck(); err != nil {
		t.Fatalf("successful fetch should restore readiness: %v", err)
	}
}

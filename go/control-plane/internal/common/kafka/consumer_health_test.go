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

func TestConsumerHealthCheckFailsImmediatelyOnProcessingBarrier(t *testing.T) {
	consumer := &Consumer{}
	consumer.recordProcessingFailure(errors.New("postgres unavailable"))
	if err := consumer.HealthCheck(); err == nil {
		t.Fatal("an uncommitted processing failure must withdraw readiness")
	}
	metrics := consumer.GetMetrics()
	if metrics.ConsecutiveProcessingFailures != 1 || metrics.LastProcessingErrorUnix <= 0 {
		t.Fatalf("unexpected processing health metrics: %+v", metrics)
	}

	consumer.recordProcessingSuccess()
	if err := consumer.HealthCheck(); err != nil {
		t.Fatalf("successful processing should restore readiness: %v", err)
	}
}

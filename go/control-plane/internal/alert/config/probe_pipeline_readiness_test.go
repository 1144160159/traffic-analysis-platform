package config

import (
	"testing"
	"time"
)

func TestProbePipelineReadinessReceiptValidation(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	valid := ProbePipelineReadinessReceipt{
		PipelineID: ProbeOperationPipelineID, ConsumerRole: ProbeAckAuthorityConsumer,
		ConsumerGroup: "alert-probe-acks", OwnerID: "member-a", OwnerEpoch: 7,
		State: ProbePipelineReady, ObservedAt: now, LeaseExpiresAt: now.Add(time.Minute),
	}
	if err := valid.Validate(now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ProbePipelineReadinessReceipt){
		"expired":      func(value *ProbePipelineReadinessReceipt) { value.LeaseExpiresAt = now },
		"oversized":    func(value *ProbePipelineReadinessReceipt) { value.LeaseExpiresAt = now.Add(6 * time.Minute) },
		"unknown role": func(value *ProbePipelineReadinessReceipt) { value.ConsumerRole = "UNKNOWN" },
		"zero epoch":   func(value *ProbePipelineReadinessReceipt) { value.OwnerEpoch = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(now); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	revoked := valid
	revoked.State = ProbePipelineRevoked
	revoked.LeaseExpiresAt = time.Time{}
	if err := revoked.Validate(now); err != nil {
		t.Fatal(err)
	}
}

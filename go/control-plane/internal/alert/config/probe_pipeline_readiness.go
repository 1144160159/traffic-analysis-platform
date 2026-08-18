package config

import (
	"fmt"
	"strings"
	"time"
)

type ProbePipelineConsumerRole string

const (
	ProbeCommandDeliveryConsumer     ProbePipelineConsumerRole = "COMMAND_DELIVERY"
	ProbeAckAuthorityConsumer        ProbePipelineConsumerRole = "ACK_AUTHORITY"
	ProbeLifecycleProjectionConsumer ProbePipelineConsumerRole = "LIFECYCLE_PROJECTION"
)

type ProbePipelineReadinessState string

const (
	ProbePipelineReady   ProbePipelineReadinessState = "READY"
	ProbePipelineRevoked ProbePipelineReadinessState = "REVOKED"
)

const ProbeOperationPipelineID = "probe-operation-v2"

// ProbePipelineReadinessReceipt identifies one real consumer-group ownership
// epoch. It is an authority input, not a process-liveness approximation.
type ProbePipelineReadinessReceipt struct {
	PipelineID     string
	ConsumerRole   ProbePipelineConsumerRole
	ConsumerGroup  string
	OwnerID        string
	OwnerEpoch     int64
	State          ProbePipelineReadinessState
	ObservedAt     time.Time
	LeaseExpiresAt time.Time
}

func (receipt ProbePipelineReadinessReceipt) Validate(now time.Time) error {
	if receipt.PipelineID != ProbeOperationPipelineID ||
		strings.TrimSpace(receipt.ConsumerGroup) == "" ||
		strings.TrimSpace(receipt.OwnerID) == "" || receipt.OwnerEpoch <= 0 ||
		receipt.ObservedAt.IsZero() || receipt.ObservedAt.After(now.Add(time.Minute)) {
		return fmt.Errorf("incomplete probe pipeline readiness receipt identity")
	}
	switch receipt.ConsumerRole {
	case ProbeCommandDeliveryConsumer, ProbeAckAuthorityConsumer, ProbeLifecycleProjectionConsumer:
	default:
		return fmt.Errorf("unsupported probe pipeline consumer role")
	}
	switch receipt.State {
	case ProbePipelineReady:
		lease := receipt.LeaseExpiresAt.Sub(receipt.ObservedAt)
		if !receipt.LeaseExpiresAt.After(now) || lease < time.Second || lease > 5*time.Minute {
			return fmt.Errorf("probe pipeline readiness lease is invalid")
		}
	case ProbePipelineRevoked:
		if !receipt.LeaseExpiresAt.IsZero() && receipt.LeaseExpiresAt.After(receipt.ObservedAt) {
			return fmt.Errorf("revoked probe pipeline receipt cannot extend a lease")
		}
	default:
		return fmt.Errorf("unsupported probe pipeline readiness state")
	}
	return nil
}

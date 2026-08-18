package model

import "time"

// DeploymentRuntimeReceipt is one exact, version-bound runtime application
// receipt used by the rule/model deployment review gate. Broker publication
// without the complete server-configured subtask set is never ready.
type DeploymentRuntimeReceipt struct {
	Component        string     `json:"component"`
	ComponentID      string     `json:"component_id"`
	EventID          string     `json:"event_id,omitempty"`
	Status           string     `json:"status"`
	BrokerPublished  bool       `json:"broker_published"`
	ExpectedAcks     int        `json:"expected_acks"`
	ReceivedAcks     int        `json:"received_acks"`
	SuccessfulAcks   int        `json:"successful_acks"`
	FailedAcks       int        `json:"failed_acks"`
	ConsumerParallel int        `json:"consumer_parallelism"`
	AppliedAt        *time.Time `json:"applied_at,omitempty"`
	KafkaPartition   *int       `json:"kafka_partition,omitempty"`
	KafkaOffset      *int64     `json:"kafka_offset,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
}

// DeploymentRuntimeGate is additive API state.  When enabled, Ready and
// ExpansionAllowed are true only when every component required by the
// deployment has an exact acknowledgement set.  DeploymentProjection is
// required only when expanding an already-started gray deployment.
type DeploymentRuntimeGate struct {
	Enabled              bool                      `json:"enabled"`
	Status               string                    `json:"status"`
	Ready                bool                      `json:"ready"`
	ExpansionAllowed     bool                      `json:"expansion_allowed"`
	Rule                 *DeploymentRuntimeReceipt `json:"rule,omitempty"`
	Model                *DeploymentRuntimeReceipt `json:"model,omitempty"`
	DeploymentProjection *DeploymentRuntimeReceipt `json:"deployment_projection,omitempty"`
	CheckedAt            time.Time                 `json:"checked_at"`
	BlockingReasons      []string                  `json:"blocking_reasons,omitempty"`
}

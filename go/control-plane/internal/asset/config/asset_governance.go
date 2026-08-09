package config

import "time"

const AssetGovernanceCreateAction = "asset-governance-work-order-create"

type AssetGovernanceWorkOrder struct {
	WorkOrderID            string                   `json:"work_order_id"`
	TenantID               string                   `json:"tenant_id"`
	AssetID                string                   `json:"asset_id"`
	ActionID               string                   `json:"action_id"`
	SourceLifecycleState   string                   `json:"source_lifecycle_state"`
	TargetLifecycleState   string                   `json:"target_lifecycle_state"`
	TargetAssetID          string                   `json:"target_asset_id,omitempty"`
	CurrentLifecycleState  string                   `json:"current_lifecycle_state"`
	Status                 string                   `json:"status"`
	Revision               int64                    `json:"revision"`
	ExpectedAssetRevision  int64                    `json:"expected_asset_revision"`
	ResultingAssetRevision int64                    `json:"resulting_asset_revision,omitempty"`
	Owner                  string                   `json:"owner"`
	RequestedBy            string                   `json:"requested_by"`
	ApprovedBy             string                   `json:"approved_by,omitempty"`
	DueAt                  time.Time                `json:"due_at"`
	EvidenceRequired       bool                     `json:"evidence_required"`
	EvidenceRefs           []string                 `json:"evidence_refs"`
	Reason                 string                   `json:"reason"`
	ExternalSystem         string                   `json:"external_system"`
	ExternalTicketID       string                   `json:"external_ticket_id,omitempty"`
	ExternalStatus         string                   `json:"external_status,omitempty"`
	TraceID                string                   `json:"trace_id"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
	CompletedAt            *time.Time               `json:"completed_at,omitempty"`
	IdempotentReplay       bool                     `json:"idempotent_replay"`
	History                []AssetGovernanceHistory `json:"history,omitempty"`
}

type AssetGovernanceHistory struct {
	Revision           int64          `json:"revision"`
	ActionID           string         `json:"action_id"`
	FromStatus         string         `json:"from_status"`
	ToStatus           string         `json:"to_status"`
	FromLifecycleState string         `json:"from_lifecycle_state"`
	ToLifecycleState   string         `json:"to_lifecycle_state"`
	Actor              string         `json:"actor"`
	Reason             string         `json:"reason"`
	EvidenceRefs       []string       `json:"evidence_refs"`
	TraceID            string         `json:"trace_id"`
	Detail             map[string]any `json:"detail"`
	CreatedAt          time.Time      `json:"created_at"`
}

type AssetGovernanceCreateCommand struct {
	ActionID              string    `json:"action_id"`
	TargetLifecycleState  string    `json:"target_lifecycle_state"`
	TargetAssetID         string    `json:"target_asset_id,omitempty"`
	Owner                 string    `json:"owner"`
	DueAt                 time.Time `json:"due_at"`
	EvidenceRequired      bool      `json:"evidence_required"`
	Reason                string    `json:"reason"`
	ExpectedAssetRevision int64     `json:"expected_asset_revision"`
	IdempotencyKey        string    `json:"-"`
	TenantID              string    `json:"-"`
	Actor                 string    `json:"-"`
	TraceID               string    `json:"-"`
	RequestID             string    `json:"-"`
	ClientIP              string    `json:"-"`
	UserAgent             string    `json:"-"`
}

type AssetGovernanceActionCommand struct {
	ActionID         string   `json:"action_id"`
	ExpectedRevision int64    `json:"expected_revision"`
	Reason           string   `json:"reason"`
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
	IdempotencyKey   string   `json:"-"`
	TenantID         string   `json:"-"`
	Actor            string   `json:"-"`
	TraceID          string   `json:"-"`
	RequestID        string   `json:"-"`
	ClientIP         string   `json:"-"`
	UserAgent        string   `json:"-"`
}

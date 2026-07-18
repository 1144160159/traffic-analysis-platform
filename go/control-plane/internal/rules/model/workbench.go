package model

import (
	"encoding/json"
	"time"
)

// RuleWorkbenchItem is a tenant-scoped, database-backed row rendered by the
// rule-management workbench. Payload stays category-specific so new visual
// modules can be added without changing the transport envelope.
type RuleWorkbenchItem struct {
	ItemID     string          `json:"item_id"`
	TenantID   string          `json:"tenant_id"`
	RuleID     string          `json:"rule_id"`
	Category   string          `json:"category"`
	Ordinal    int             `json:"ordinal"`
	Payload    json.RawMessage `json:"payload"`
	ScenarioID string          `json:"scenario_id"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type RuleWorkbench struct {
	Rule     *Rule                        `json:"rule"`
	Versions []*RuleVersion               `json:"versions"`
	Items    map[string][]json.RawMessage `json:"items"`
	Source   string                       `json:"source"`
}

type RuleWorkbenchActionRequest struct {
	ActionID string                 `json:"action_id"`
	Action   string                 `json:"action"`
	Target   string                 `json:"target"`
	Payload  map[string]interface{} `json:"payload,omitempty"`
}

type RuleWorkbenchActionJob struct {
	JobID       string                 `json:"job_id"`
	ActionID    string                 `json:"action_id"`
	TenantID    string                 `json:"tenant_id"`
	RuleID      string                 `json:"rule_id"`
	Action      string                 `json:"action"`
	Target      string                 `json:"target"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	Status      string                 `json:"status"`
	RequestedBy string                 `json:"requested_by"`
	CreatedAt   time.Time              `json:"created_at"`
}

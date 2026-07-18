package model

import (
	"encoding/json"
	"time"
)

// ModelActionRequest is the authenticated request envelope for model
// workbench actions that run asynchronously.
type ModelActionRequest struct {
	ActionID string                 `json:"action_id,omitempty"`
	Action   string                 `json:"action"`
	Target   string                 `json:"target"`
	Version  string                 `json:"version,omitempty"`
	Payload  map[string]interface{} `json:"payload,omitempty"`
}

// ModelActionJob is persisted before a 202 response is returned. The matching
// audit row is written in the same transaction.
type ModelActionJob struct {
	JobID       string                 `json:"job_id"`
	ActionID    string                 `json:"action_id"`
	TenantID    string                 `json:"tenant_id"`
	ModelID     string                 `json:"model_id"`
	Version     string                 `json:"version,omitempty"`
	Action      string                 `json:"action"`
	Target      string                 `json:"target"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	Status      string                 `json:"status"`
	RequestedBy string                 `json:"requested_by"`
	CreatedAt   time.Time              `json:"created_at"`
}

type ModelWorkbenchItem struct {
	ItemID     string          `json:"item_id"`
	TenantID   string          `json:"tenant_id"`
	ModelID    string          `json:"model_id"`
	Category   string          `json:"category"`
	Ordinal    int             `json:"ordinal"`
	Payload    json.RawMessage `json:"payload"`
	ScenarioID string          `json:"scenario_id"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type ModelWorkbench struct {
	Model    *Model                       `json:"model"`
	Versions []*ModelVersion              `json:"versions"`
	Items    map[string][]json.RawMessage `json:"items"`
	Actions  []*ModelActionJob            `json:"actions"`
	Source   string                       `json:"source"`
}

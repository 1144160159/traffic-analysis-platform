package model

import "time"

// DeploymentWorkbenchItem is a tenant-scoped data-backed card/table row used by
// the deployment-management workspace.
type DeploymentWorkbenchItem struct {
	ItemID       string                 `json:"item_id"`
	TenantID     string                 `json:"tenant_id"`
	DeploymentID string                 `json:"deployment_id"`
	Category     string                 `json:"category"`
	Ordinal      int                    `json:"ordinal"`
	Payload      map[string]interface{} `json:"payload"`
	ScenarioID   string                 `json:"scenario_id"`
	OccurredAt   time.Time              `json:"occurred_at"`
}

// DeploymentWorkbench is the normal-mode source for health, evidence,
// version-diff and rollback panels. Visual-breakdown mode remains isolated in
// the frontend and is never accepted as business data.
type DeploymentWorkbench struct {
	Deployment *Deployment                         `json:"deployment"`
	History    []*DeploymentHistoryRecord          `json:"history"`
	Items      map[string][]map[string]interface{} `json:"items"`
	Source     string                              `json:"source"`
}

// DeploymentHistoryRecord is the API-safe history representation shared by
// the service and workbench response.
type DeploymentHistoryRecord struct {
	ID           int64                  `json:"id"`
	DeploymentID string                 `json:"deployment_id"`
	Action       string                 `json:"action"`
	OperatorID   string                 `json:"operator_id"`
	CreatedAt    time.Time              `json:"created_at"`
	Detail       map[string]interface{} `json:"detail,omitempty"`
}

// DeploymentEvidenceBundle is the audited server-generated evidence payload
// downloaded by the deployment-management page.
type DeploymentEvidenceBundle struct {
	ExportID        string                     `json:"export_id"`
	GeneratedAt     time.Time                  `json:"generated_at"`
	GeneratedBy     string                     `json:"generated_by"`
	Deployment      *Deployment                `json:"deployment"`
	History         []*DeploymentHistoryRecord `json:"history"`
	Evidence        []map[string]interface{}   `json:"evidence"`
	Source          string                     `json:"source"`
	BundleChecksum  string                     `json:"bundle_checksum"`
	DownloadContent string                     `json:"download_content"`
}

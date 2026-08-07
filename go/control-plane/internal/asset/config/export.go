package config

import "time"

const (
	AssetExportActionID = "asset-inventory-export"

	AssetExportStatusAccepted  = "accepted"
	AssetExportStatusRunning   = "running"
	AssetExportStatusCompleted = "completed"
	AssetExportStatusFailed    = "failed"
	AssetExportStatusCancelled = "cancelled"
)

type AssetExportRequest struct {
	ActionID string          `json:"action_id"`
	Format   string          `json:"format"`
	Columns  []string        `json:"columns"`
	Filter   AssetListFilter `json:"filter"`
	Reason   string          `json:"reason"`
}

type AssetExportCommand struct {
	IdempotencyKey string
	Actor          string
	TraceID        string
	RequestID      string
	ClientIP       string
	UserAgent      string
}

type AssetExportJob struct {
	JobID            string            `json:"job_id"`
	TenantID         string            `json:"tenant_id"`
	ActionID         string            `json:"action_id"`
	Format           string            `json:"format"`
	Status           string            `json:"status"`
	Revision         int64             `json:"revision"`
	Columns          []string          `json:"columns"`
	Filter           AssetListFilter   `json:"filter"`
	QuerySHA256      string            `json:"query_sha256"`
	Reason           string            `json:"reason"`
	SnapshotID       string            `json:"snapshot_id,omitempty"`
	AsOf             time.Time         `json:"as_of,omitempty"`
	SourceWatermarks map[string]string `json:"source_watermarks"`
	RowCount         int               `json:"row_count"`
	ObjectBucket     string            `json:"object_bucket,omitempty"`
	ObjectKey        string            `json:"object_key,omitempty"`
	MIMEType         string            `json:"mime_type,omitempty"`
	ArtifactSHA256   string            `json:"artifact_sha256,omitempty"`
	SizeBytes        int64             `json:"size_bytes"`
	RetentionUntil   time.Time         `json:"retention_until,omitempty"`
	ErrorMessage     string            `json:"error_message,omitempty"`
	Attempts         int               `json:"attempts"`
	CreatedBy        string            `json:"created_by"`
	TraceID          string            `json:"trace_id"`
	IdempotentReplay bool              `json:"idempotent_replay,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	CompletedAt      time.Time         `json:"completed_at,omitempty"`
	LockedBy         string            `json:"-"`
}

type AssetExportSnapshot struct {
	Assets           []*AssetRecord
	SnapshotID       string
	AsOf             time.Time
	SourceWatermarks map[string]string
}

type AssetColumnPreference struct {
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	ViewID    string    `json:"view_id"`
	Columns   []string  `json:"columns"`
	Revision  int64     `json:"revision"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AssetColumnPreferenceCommand struct {
	ViewID           string   `json:"view_id"`
	Columns          []string `json:"columns"`
	ExpectedRevision int64    `json:"expected_revision"`
	Reason           string   `json:"reason"`
	Actor            string   `json:"-"`
	TraceID          string   `json:"-"`
	RequestID        string   `json:"-"`
	ClientIP         string   `json:"-"`
	UserAgent        string   `json:"-"`
}

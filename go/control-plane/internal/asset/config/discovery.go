package config

import "time"

const (
	DiscoveryModeSNMP     = "snmp"
	DiscoveryModeLLDP     = "lldp"
	DiscoveryModeSNMPLLDP = "snmp_lldp"

	DiscoveryStatusQueued          = "queued"
	DiscoveryStatusRunning         = "running"
	DiscoveryStatusCancelRequested = "cancel_requested"
	DiscoveryStatusCancelled       = "cancelled"
	DiscoveryStatusSucceeded       = "succeeded"
	DiscoveryStatusPartial         = "partial"
	DiscoveryStatusFailed          = "failed"
	DiscoveryStatusBlocked         = "blocked"
	// DiscoveryStatusCompleted is retained for the feature-flagged legacy
	// synchronous path. V2 jobs use DiscoveryStatusSucceeded.
	DiscoveryStatusCompleted = "completed"
)

type DiscoveryCredential struct {
	CredentialID     string    `json:"credential_id"`
	TenantID         string    `json:"tenant_id"`
	ActionID         string    `json:"action_id,omitempty"`
	Name             string    `json:"name"`
	Protocol         string    `json:"protocol"`
	Endpoint         string    `json:"endpoint,omitempty"`
	SecretRef        string    `json:"secret_ref"`
	CreatedBy        string    `json:"created_by,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	ExpectedRevision int64     `json:"expected_revision"`
	Revision         int64     `json:"revision"`
	IdempotentReplay bool      `json:"idempotent_replay,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type DiscoveryNeighbor struct {
	MACAddress string `json:"mac_address,omitempty"`
	IPAddress  string `json:"ip_address,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	Interface  string `json:"interface,omitempty"`
	VlanID     string `json:"vlan_id,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

type DiscoveryObservation struct {
	IPAddress  string              `json:"ip_address,omitempty"`
	MACAddress string              `json:"mac_address,omitempty"`
	Hostname   string              `json:"hostname,omitempty"`
	Vendor     string              `json:"vendor,omitempty"`
	OSType     string              `json:"os_type,omitempty"`
	VlanID     string              `json:"vlan_id,omitempty"`
	SwitchPort string              `json:"switch_port,omitempty"`
	Neighbors  []DiscoveryNeighbor `json:"neighbors,omitempty"`
}

type ActiveDiscoveryRequest struct {
	TenantID     string                 `json:"tenant_id"`
	ActionID     string                 `json:"action_id,omitempty"`
	Mode         string                 `json:"mode"`
	TargetCIDR   string                 `json:"target_cidr,omitempty"`
	CredentialID string                 `json:"credential_id,omitempty"`
	RequestedBy  string                 `json:"requested_by,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
	RateLimit    int                    `json:"rate_limit_per_second,omitempty"`
	SecurityFrom time.Time              `json:"security_window_start,omitempty"`
	SecurityTo   time.Time              `json:"security_window_end,omitempty"`
	ApprovedBy   string                 `json:"approved_by,omitempty"`
	Observations []DiscoveryObservation `json:"observations,omitempty"`
}

type DiscoveryRun struct {
	RunID            string    `json:"run_id"`
	TenantID         string    `json:"tenant_id"`
	Mode             string    `json:"mode"`
	TargetCIDR       string    `json:"target_cidr,omitempty"`
	CredentialID     string    `json:"credential_id,omitempty"`
	ActionID         string    `json:"action_id"`
	Status           string    `json:"status"`
	Revision         int64     `json:"revision"`
	RequestedBy      string    `json:"requested_by,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	RateLimit        int       `json:"rate_limit_per_second"`
	SecurityFrom     time.Time `json:"security_window_start,omitempty"`
	SecurityTo       time.Time `json:"security_window_end,omitempty"`
	ApprovedBy       string    `json:"approved_by,omitempty"`
	TraceID          string    `json:"trace_id,omitempty"`
	IdempotentReplay bool      `json:"idempotent_replay,omitempty"`
	DiscoveredAssets int       `json:"discovered_assets"`
	DiscoveredLinks  int       `json:"discovered_links"`
	CandidateCount   int       `json:"discovered_candidates"`
	RejectedRecords  int       `json:"rejected_records"`
	ResultWatermark  string    `json:"result_watermark,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	QueuedAt         time.Time `json:"queued_at"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
}

type DiscoveryResult struct {
	Run             *DiscoveryRun `json:"run"`
	AcceptedAssets  int           `json:"accepted_assets"`
	AcceptedLinks   int           `json:"accepted_links"`
	RejectedRecords int           `json:"rejected_records"`
}

type DiscoveryJobCommand struct {
	IdempotencyKey string
	Actor          string
	TraceID        string
	RequestID      string
	ClientIP       string
	UserAgent      string
}

type DiscoveryResourceCommand struct {
	ActionID               string
	ExpectedRevision       int64
	ResolveCurrentRevision bool
	IdempotencyKey         string
	Actor                  string
	Reason                 string
	TraceID                string
	RequestID              string
	ClientIP               string
	UserAgent              string
}

type DiscoveryCandidate struct {
	CandidateID    string               `json:"candidate_id"`
	RunID          string               `json:"run_id"`
	TenantID       string               `json:"tenant_id"`
	Fingerprint    string               `json:"fingerprint"`
	Observation    DiscoveryObservation `json:"observation"`
	Status         string               `json:"status"`
	Revision       int64                `json:"revision"`
	SourceAssetID  string               `json:"source_asset_id,omitempty"`
	DecisionReason string               `json:"decision_reason,omitempty"`
	DecidedBy      string               `json:"decided_by,omitempty"`
	DiscoveredAt   time.Time            `json:"discovered_at"`
	DecidedAt      time.Time            `json:"decided_at,omitempty"`
}

type DiscoveryRunHistory struct {
	TransitionID int64          `json:"transition_id"`
	RunID        string         `json:"run_id"`
	TenantID     string         `json:"tenant_id"`
	FromStatus   string         `json:"from_status"`
	ToStatus     string         `json:"to_status"`
	Revision     int64          `json:"revision"`
	Actor        string         `json:"actor"`
	Reason       string         `json:"reason"`
	TraceID      string         `json:"trace_id"`
	Detail       map[string]any `json:"detail"`
	CreatedAt    time.Time      `json:"created_at"`
}

type DiscoveryCandidateMergeCommand struct {
	ExpectedCandidateRevision int64  `json:"expected_candidate_revision"`
	ExpectedAssetRevision     int64  `json:"expected_asset_revision"`
	MergeMode                 string `json:"merge_mode"`
	Reason                    string `json:"reason"`
	IdempotencyKey            string `json:"-"`
	Actor                     string `json:"-"`
	TraceID                   string `json:"-"`
	RequestID                 string `json:"-"`
	ClientIP                  string `json:"-"`
	UserAgent                 string `json:"-"`
}

type DiscoveryCandidateMergeResult struct {
	Candidate        *DiscoveryCandidate `json:"candidate"`
	AssetID          string              `json:"asset_id"`
	AssetRevision    int64               `json:"asset_revision"`
	AssetCreated     bool                `json:"asset_created"`
	EventID          string              `json:"event_id"`
	OutboxID         int64               `json:"outbox_id"`
	IdempotentReplay bool                `json:"idempotent_replay"`
	TraceID          string              `json:"trace_id"`
}

type TopologyLink struct {
	LinkID            string    `json:"link_id"`
	TenantID          string    `json:"tenant_id"`
	RunID             string    `json:"run_id,omitempty"`
	SourceAssetID     string    `json:"source_asset_id,omitempty"`
	SourceMAC         string    `json:"source_mac,omitempty"`
	SourceIP          string    `json:"source_ip,omitempty"`
	SourceInterface   string    `json:"source_interface,omitempty"`
	NeighborAssetID   string    `json:"neighbor_asset_id,omitempty"`
	NeighborMAC       string    `json:"neighbor_mac,omitempty"`
	NeighborIP        string    `json:"neighbor_ip,omitempty"`
	NeighborInterface string    `json:"neighbor_interface,omitempty"`
	Protocol          string    `json:"protocol"`
	Confidence        int       `json:"confidence"`
	Revision          int64     `json:"revision"`
	IdempotentReplay  bool      `json:"idempotent_replay,omitempty"`
	ObservedAt        time.Time `json:"observed_at"`
	CreatedAt         time.Time `json:"created_at"`
}

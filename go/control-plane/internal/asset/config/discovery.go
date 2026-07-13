package config

import "time"

const (
	DiscoveryModeSNMP     = "snmp"
	DiscoveryModeLLDP     = "lldp"
	DiscoveryModeSNMPLLDP = "snmp_lldp"

	DiscoveryStatusQueued    = "queued"
	DiscoveryStatusCompleted = "completed"
	DiscoveryStatusFailed    = "failed"
)

type DiscoveryCredential struct {
	CredentialID string    `json:"credential_id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	Protocol     string    `json:"protocol"`
	Endpoint     string    `json:"endpoint,omitempty"`
	SecretRef    string    `json:"secret_ref"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
	Mode         string                 `json:"mode"`
	TargetCIDR   string                 `json:"target_cidr,omitempty"`
	CredentialID string                 `json:"credential_id,omitempty"`
	RequestedBy  string                 `json:"requested_by,omitempty"`
	Observations []DiscoveryObservation `json:"observations,omitempty"`
}

type DiscoveryRun struct {
	RunID            string    `json:"run_id"`
	TenantID         string    `json:"tenant_id"`
	Mode             string    `json:"mode"`
	TargetCIDR       string    `json:"target_cidr,omitempty"`
	CredentialID     string    `json:"credential_id,omitempty"`
	Status           string    `json:"status"`
	RequestedBy      string    `json:"requested_by,omitempty"`
	DiscoveredAssets int       `json:"discovered_assets"`
	DiscoveredLinks  int       `json:"discovered_links"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
}

type DiscoveryResult struct {
	Run             *DiscoveryRun `json:"run"`
	AcceptedAssets  int           `json:"accepted_assets"`
	AcceptedLinks   int           `json:"accepted_links"`
	RejectedRecords int           `json:"rejected_records"`
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
	ObservedAt        time.Time `json:"observed_at"`
	CreatedAt         time.Time `json:"created_at"`
}

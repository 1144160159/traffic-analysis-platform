package entityresolution

const (
	RuleVersion = "entity-resolution/v1"

	IdentifierAssetID     IdentifierKind = "asset_id"
	IdentifierUserID      IdentifierKind = "user_id"
	IdentifierIP          IdentifierKind = "ip"
	IdentifierMAC         IdentifierKind = "mac"
	IdentifierProbeID     IdentifierKind = "probe_id"
	IdentifierCommunityID IdentifierKind = "community_id"

	RailFlow           SourceRail = "flow"
	RailAssetAuthority SourceRail = "asset_authority"
	RailAssetBinding   SourceRail = "asset_binding"
	RailDeviceLog      SourceRail = "device_log"
	RailUserBehavior   SourceRail = "user_behavior"
	RailProbeIngest    SourceRail = "probe_ingest"

	StatusAccepted     ResolutionStatus = "accepted"
	StatusPartial      ResolutionStatus = "partial"
	StatusAmbiguous    ResolutionStatus = "ambiguous"
	StatusConflict     ResolutionStatus = "conflict"
	StatusInsufficient ResolutionStatus = "insufficient"

	AssetExactConfidencePPM       = 1_000_000
	UserExactConfidencePPM        = 1_000_000
	ProbeExactConfidencePPM       = 1_000_000
	MACAssetConfidencePPM         = 950_000
	IPAssetConfidencePPM          = 700_000
	CommunityConfidencePPM        = 400_000
	MACMaximumLinkAgeMS     int64 = 2_592_000_000
	IPMaximumLinkAgeMS      int64 = 3_600_000
)

type IdentifierKind string
type SourceRail string
type ResolutionStatus string

type Identifier struct {
	Kind  IdentifierKind `json:"kind"`
	Value string         `json:"value"`
	Role  string         `json:"role"`
}

// SourceReference identifies the immutable input fact independently from its payload.
// Kafka facts use topic/partition/offset. Direct authority facts use
// authority/event_id/source_revision. PayloadSHA256 is always required.
type SourceReference struct {
	Rail           SourceRail `json:"rail"`
	Authority      string     `json:"authority"`
	Topic          string     `json:"topic,omitempty"`
	Partition      int        `json:"partition,omitempty"`
	Offset         int64      `json:"offset,omitempty"`
	EventID        string     `json:"event_id"`
	SourceRevision int64      `json:"source_revision,omitempty"`
	ObservedAtMS   int64      `json:"observed_at_ms"`
	PayloadSHA256  string     `json:"payload_sha256"`
}

type Observation struct {
	TenantID    string          `json:"tenant_id"`
	Source      SourceReference `json:"source"`
	Identifiers []Identifier    `json:"identifiers"`
}

type EntityMatch struct {
	EntityType    string       `json:"entity_type"`
	EntityID      string       `json:"entity_id"`
	Role          string       `json:"role"`
	MatchedBy     []Identifier `json:"matched_by"`
	ConfidencePPM int          `json:"confidence_ppm"`
	RuleID        string       `json:"rule_id"`
}

type ResolutionIssue struct {
	Class              string     `json:"class"`
	Code               string     `json:"code"`
	Scope              string     `json:"scope"`
	Identifier         Identifier `json:"identifier"`
	CandidateEntityIDs []string   `json:"candidate_entity_ids"`
	RuleID             string     `json:"rule_id"`
}

type Correlation struct {
	Identifier    Identifier `json:"identifier"`
	ConfidencePPM int        `json:"confidence_ppm"`
	RuleID        string     `json:"rule_id"`
}

type ResolutionResult struct {
	ResolutionID          string            `json:"resolution_id"`
	DecisionSHA256        string            `json:"decision_sha256,omitempty"`
	RuleVersion           string            `json:"rule_version"`
	TenantID              string            `json:"tenant_id"`
	Source                SourceReference   `json:"source"`
	Status                ResolutionStatus  `json:"status"`
	NormalizedIdentifiers []Identifier      `json:"normalized_identifiers"`
	Entities              []EntityMatch     `json:"entities"`
	Issues                []ResolutionIssue `json:"issues"`
	Correlations          []Correlation     `json:"correlations"`
}

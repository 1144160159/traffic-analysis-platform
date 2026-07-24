package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type encryptedTrafficStatsDTO struct {
	TotalSessions       int64   `json:"total_sessions"`
	ObservedSessions    int64   `json:"observed_sessions"`
	EncryptedRatio      float64 `json:"encrypted_ratio"`
	TLSSessions         int64   `json:"tls_sessions"`
	QUICSessions        int64   `json:"quic_sessions"`
	JA3Fingerprints     int64   `json:"ja3_fingerprints"`
	MaliciousJA3Matches int64   `json:"malicious_ja3_matches"`
}

type encryptedTrafficSessionDTO struct {
	SessionID             string  `json:"session_id"`
	CommunityID           string  `json:"community_id"`
	SrcIP                 string  `json:"src_ip"`
	DstIP                 string  `json:"dst_ip"`
	DstPort               uint32  `json:"dst_port"`
	Protocol              string  `json:"protocol"`
	SNI                   string  `json:"sni,omitempty"`
	SNIHash               string  `json:"sni_hash,omitempty"`
	JA3Fingerprint        string  `json:"ja3_fingerprint,omitempty"`
	JA3SFingerprint       string  `json:"ja3s_fingerprint,omitempty"`
	CipherSuite           string  `json:"cipher_suite,omitempty"`
	TLSVersion            string  `json:"tls_version,omitempty"`
	CertificateHash       string  `json:"certificate_hash,omitempty"`
	CertificateIssuer     string  `json:"certificate_issuer,omitempty"`
	CertificateValidUntil int64   `json:"certificate_valid_until,omitempty"`
	EntropyScore          float64 `json:"entropy_score"`
	AnomalyScore          float64 `json:"anomaly_score"`
	RiskLevel             string  `json:"risk_level"`
	PacketCount           uint32  `json:"packet_count"`
	ByteCount             uint64  `json:"byte_count"`
	EvidenceCount         uint32  `json:"evidence_count,omitempty"`
	PcapIndex             string  `json:"pcap_index,omitempty"`
	HasHandshakeMetadata  bool    `json:"has_handshake_metadata"`
	StartTime             int64   `json:"start_time"`
	EndTime               int64   `json:"end_time"`
}

type encryptedJA3FingerprintDTO struct {
	JA3Fingerprint string  `json:"ja3"`
	TLSVersion     string  `json:"tls_version"`
	SessionCount   uint64  `json:"session_count"`
	SNICount       uint64  `json:"sni_count"`
	TrafficRatio   float64 `json:"traffic_ratio"`
	EntropyAverage float64 `json:"entropy_average"`
	RiskLevel      string  `json:"risk_level"`
}

type encryptedTunnelProtocolDTO struct {
	Protocol   string `json:"protocol"`
	Count      int64  `json:"count"`
	TotalBytes uint64 `json:"total_bytes"`
}

type encryptedTunnelUserDTO struct {
	IP         string `json:"ip"`
	Count      int64  `json:"count"`
	Protocol   string `json:"protocol"`
	Risk       string `json:"risk"`
	TotalBytes uint64 `json:"total_bytes"`
	LastSeen   int64  `json:"last_seen"`
}

type encryptedExfiltrationSourceDTO struct {
	SrcIP        string `json:"src_ip"`
	SessionCount int64  `json:"session_count"`
	UploadBytes  uint64 `json:"upload_bytes"`
	TotalBytes   uint64 `json:"total_bytes"`
	DstCount     int64  `json:"dst_count"`
	LastSeen     int64  `json:"last_seen"`
	Risk         string `json:"risk"`
}

type encryptedExfiltrationDestinationDTO struct {
	DstIP        string `json:"dst_ip"`
	SessionCount int64  `json:"session_count"`
	UploadBytes  uint64 `json:"upload_bytes"`
	TotalBytes   uint64 `json:"total_bytes"`
	SrcCount     int64  `json:"src_count"`
	LastSeen     int64  `json:"last_seen"`
	Risk         string `json:"risk"`
}

type encryptedExfiltrationRiskDTO struct {
	Type       string `json:"type"`
	Count      int64  `json:"count"`
	Severity   string `json:"severity"`
	TotalBytes uint64 `json:"total_bytes"`
}

type encryptedExfiltrationPathDTO struct {
	SrcIP        string `json:"src_ip"`
	DstIP        string `json:"dst_ip"`
	SessionCount int64  `json:"session_count"`
	UploadBytes  uint64 `json:"upload_bytes"`
	LastSeen     int64  `json:"last_seen"`
	Risk         string `json:"risk"`
}

type encryptedExfiltrationTrendDTO struct {
	BucketStart             int64 `json:"bucket_start"`
	DestinationCount        int64 `json:"destination_count"`
	LargeUploadSessions     int64 `json:"large_upload_sessions"`
	LongLivedSessions       int64 `json:"long_lived_sessions"`
	NonStandardPortSessions int64 `json:"non_standard_port_sessions"`
	EncryptedSessions       int64 `json:"encrypted_sessions"`
}

type encryptedEvidencePcapDTO struct {
	FileKey        string `json:"file_key"`
	ProbeID        string `json:"probe_id"`
	StartTime      int64  `json:"start_time"`
	EndTime        int64  `json:"end_time"`
	PacketCount    int64  `json:"packet_count"`
	ByteCount      uint64 `json:"byte_count"`
	StoragePath    string `json:"storage_path"`
	SHA256         string `json:"sha256"`
	CompressedSize uint64 `json:"compressed_size"`
}

type encryptedEvidenceTrendDTO struct {
	BucketStart int64  `json:"bucket_start"`
	ByteCount   uint64 `json:"byte_count"`
	PacketCount int64  `json:"packet_count"`
}

type encryptedEvidenceEntropyTrendDTO struct {
	BucketStart  int64   `json:"bucket_start"`
	EntropyScore float64 `json:"entropy_score"`
}

type encryptedEvidenceAnomalyTrendDTO struct {
	BucketStart  int64   `json:"bucket_start"`
	AnomalyScore float64 `json:"anomaly_score"`
}

type encryptedEvidenceCompletenessDTO struct {
	Label    string `json:"label"`
	Complete int64  `json:"complete"`
	Total    int64  `json:"total"`
}

type encryptedTrafficEgressActionRequest struct {
	Action   string `json:"action"`
	Target   string `json:"target"`
	DataMode string `json:"data_mode"`
}

type encryptedTrafficEvidenceActionRequest struct {
	Action   string `json:"action"`
	Target   string `json:"target"`
	DataMode string `json:"data_mode"`
}

type dataSourceDTO struct {
	SourceID         string                 `json:"source_id"`
	TenantID         string                 `json:"tenant_id"`
	Name             string                 `json:"name"`
	SourceType       string                 `json:"source_type"`
	Status           string                 `json:"status"`
	LastIngestAt     int64                  `json:"last_ingest_at"`
	RecordsPerMinute float64                `json:"records_per_minute"`
	ErrorRate        *float64               `json:"error_rate"`
	FieldCoverage    *float64               `json:"field_coverage"`
	RecentTrend      []int64                `json:"recent_trend"`
	Config           map[string]interface{} `json:"config"`
	CreatedAt        int64                  `json:"created_at"`
}

type fusionStatsDTO struct {
	TotalEvents     int64                              `json:"total_events"`
	EntitiesAligned int64                              `json:"entities_aligned"`
	AlignmentRate   float64                            `json:"alignment_rate"`
	DataSourceStats map[string]fusionDataSourceStatDTO `json:"data_source_stats"`
	QualityMetrics  fusionQualityMetricsDTO            `json:"quality_metrics"`
}

type fusionDataSourceStatDTO struct {
	Count         int64   `json:"count"`
	RecordsPerMin float64 `json:"records_per_min"`
}

type fusionQualityMetricsDTO struct {
	Completeness    float64 `json:"completeness"`
	Accuracy        float64 `json:"accuracy"`
	Freshness       float64 `json:"freshness"`
	DuplicationRate float64 `json:"duplication_rate"`
}

type fusionValueReportDTO struct {
	TenantID             string                   `json:"tenant_id"`
	FormulaVersion       string                   `json:"formula_version"`
	WindowHours          int                      `json:"window_hours"`
	TimeRange            map[string]int64         `json:"time_range"`
	SourceCount          int                      `json:"source_count"`
	ActiveSourceCount    int                      `json:"active_source_count"`
	SingleSourceBaseline fusionValueMetricsDTO    `json:"single_source_baseline"`
	MultiSource          fusionValueMetricsDTO    `json:"multi_source"`
	Delta                fusionValueDeltaDTO      `json:"delta"`
	QualityGates         []fusionValueGateDTO     `json:"quality_gates"`
	Evidence             []fusionValueEvidenceDTO `json:"evidence"`
}

type fusionValueMetricsDTO struct {
	DetectionCount     int64   `json:"detection_count"`
	ResolvedCount      int64   `json:"resolved_count"`
	FalsePositiveCount int64   `json:"false_positive_count"`
	FalsePositiveRate  float64 `json:"false_positive_rate"`
	AvgMTTRMinutes     float64 `json:"avg_mttr_minutes"`
	AvgLeadTimeMinutes float64 `json:"avg_lead_time_minutes"`
	CoverageRate       float64 `json:"coverage_rate"`
	Confidence         float64 `json:"confidence"`
	SourceCount        int     `json:"source_count"`
	EntityCount        int64   `json:"entity_count"`
}

type fusionValueDeltaDTO struct {
	LeadTimeMinutes           float64 `json:"lead_time_minutes"`
	FalsePositiveReductionPct float64 `json:"false_positive_reduction_pct"`
	MTTRReductionPct          float64 `json:"mttr_reduction_pct"`
	ConfidenceLiftPct         float64 `json:"confidence_lift_pct"`
	CoverageLiftPct           float64 `json:"coverage_lift_pct"`
}

type fusionValueGateDTO struct {
	Gate     string `json:"gate"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type fusionValueEvidenceDTO struct {
	Label  string `json:"label"`
	Source string `json:"source"`
	Count  int64  `json:"count"`
	Status string `json:"status"`
}

type fusionAlertValueSummary struct {
	TotalAlerts        int64
	ResolvedAlerts     int64
	FalsePositives     int64
	HighSeverityAlerts int64
	AvgResponseTimeMin float64
}

type fusionConflictResolveRequest struct {
	ObjectID             string                 `json:"object_id"`
	ObjectType           string                 `json:"object_type"`
	FieldName            string                 `json:"field_name"`
	SelectedSource       string                 `json:"selected_source"`
	SelectedValue        string                 `json:"selected_value"`
	Strategy             string                 `json:"strategy"`
	Note                 string                 `json:"note"`
	RuleID               string                 `json:"rule_id"`
	ExpectedStateVersion *int64                 `json:"expected_state_version"`
	Detail               map[string]interface{} `json:"detail"`
}

type fusionConflictResolutionDTO struct {
	TenantID       string                 `json:"tenant_id"`
	ConflictID     string                 `json:"conflict_id"`
	ObjectID       string                 `json:"object_id"`
	ObjectType     string                 `json:"object_type"`
	FieldName      string                 `json:"field_name"`
	SelectedSource string                 `json:"selected_source"`
	SelectedValue  string                 `json:"selected_value"`
	Strategy       string                 `json:"strategy"`
	Note           string                 `json:"note"`
	RuleID         string                 `json:"rule_id"`
	StateVersion   int64                  `json:"state_version"`
	ResolvedBy     string                 `json:"resolved_by"`
	ResolvedAt     int64                  `json:"resolved_at"`
	Detail         map[string]interface{} `json:"detail"`
}

type fusionRepairTaskDTO struct {
	TaskID     string `json:"task_id"`
	TenantID   string `json:"tenant_id"`
	ConflictID string `json:"conflict_id"`
	TaskType   string `json:"task_type"`
	Status     string `json:"status"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  int64  `json:"created_at"`
}

type fusionRepairTaskEvidenceDTO struct {
	TaskID         string                 `json:"task_id"`
	TenantID       string                 `json:"tenant_id"`
	ConflictID     string                 `json:"conflict_id"`
	ObjectID       string                 `json:"object_id"`
	ObjectType     string                 `json:"object_type"`
	FieldName      string                 `json:"field_name"`
	RuleID         string                 `json:"rule_id"`
	SelectedSource string                 `json:"selected_source"`
	SelectedValue  string                 `json:"selected_value"`
	StateVersion   int64                  `json:"state_version"`
	Status         string                 `json:"status"`
	RequestedBy    string                 `json:"requested_by"`
	Note           string                 `json:"note"`
	Detail         map[string]interface{} `json:"detail"`
	CreatedAt      int64                  `json:"created_at"`
	UpdatedAt      int64                  `json:"updated_at"`
}

type fusionRuleUpdateRequest struct {
	RuleName            string                 `json:"rule_name"`
	Status              string                 `json:"status"`
	Strategy            string                 `json:"strategy"`
	ConfidenceThreshold *float64               `json:"confidence_threshold"`
	Note                string                 `json:"note"`
	ExpectedVersion     *int64                 `json:"expected_version"`
	Detail              map[string]interface{} `json:"detail"`
}

type fusionRuleOverrideDTO struct {
	TenantID            string                 `json:"tenant_id"`
	RuleID              string                 `json:"rule_id"`
	RuleName            string                 `json:"rule_name"`
	Version             int64                  `json:"version"`
	Status              string                 `json:"status"`
	Strategy            string                 `json:"strategy"`
	ConfidenceThreshold float64                `json:"confidence_threshold"`
	Note                string                 `json:"note"`
	UpdatedBy           string                 `json:"updated_by"`
	UpdatedAt           int64                  `json:"updated_at"`
	Detail              map[string]interface{} `json:"detail"`
}

type fusionConflictDTO struct {
	TenantID     string                   `json:"tenant_id"`
	ConflictID   string                   `json:"conflict_id"`
	ObjectID     string                   `json:"object_id"`
	ObjectType   string                   `json:"object_type"`
	FieldName    string                   `json:"field_name"`
	SourceValues []map[string]interface{} `json:"source_values"`
	SourceCount  int                      `json:"source_count"`
	Confidence   float64                  `json:"confidence"`
	Severity     string                   `json:"severity"`
	Status       string                   `json:"status"`
	RuleID       string                   `json:"rule_id"`
	StateVersion int64                    `json:"state_version"`
	Origin       string                   `json:"origin"`
	Detail       map[string]interface{}   `json:"detail"`
	DetectedAt   int64                    `json:"detected_at"`
	UpdatedAt    int64                    `json:"updated_at"`
}

type fusionWorkbenchDTO struct {
	Sources           []dataSourceDTO         `json:"sources"`
	Stats             fusionStatsDTO          `json:"stats"`
	Rules             []fusionRuleOverrideDTO `json:"rules"`
	PipelineRules     []fusionRuleOverrideDTO `json:"pipeline_rules"`
	RuleTotal         int64                   `json:"rule_total"`
	RuleLimit         int                     `json:"rule_limit"`
	RuleOffset        int                     `json:"rule_offset"`
	Conflicts         []fusionConflictDTO     `json:"conflicts"`
	ConflictTotal     int64                   `json:"conflict_total"`
	ConflictLimit     int                     `json:"conflict_limit"`
	ConflictOffset    int                     `json:"conflict_offset"`
	AuditEvents       []auditTrailDTO         `json:"audit_events"`
	AuditTotal        int64                   `json:"audit_total"`
	AuditLimit        int                     `json:"audit_limit"`
	AuditOffset       int                     `json:"audit_offset"`
	EntityCounts      map[string]int64        `json:"entity_counts"`
	PendingCount      int64                   `json:"pending_count"`
	ResolvedCount     int64                   `json:"resolved_count"`
	PendingRiskCounts map[string]int64        `json:"pending_risk_counts"`
}

type fusionEvidencePackageRequest struct {
	ConflictID string `json:"conflict_id"`
}

type alignedEntityDTO struct {
	EntityID         string            `json:"entity_id"`
	EntityType       string            `json:"entity_type"`
	Identifiers      map[string]string `json:"identifiers"`
	RiskScore        int               `json:"risk_score"`
	AssetCriticality string            `json:"asset_criticality"`
	LastUpdated      int64             `json:"last_updated"`
}

type behaviorBaselineDTO struct {
	BaselineID   string              `json:"baseline_id"`
	TenantID     string              `json:"tenant_id"`
	Name         string              `json:"name"`
	EntityType   string              `json:"entity_type"`
	EntityID     string              `json:"entity_id"`
	BaselineType string              `json:"baseline_type"`
	Metrics      []behaviorMetricDTO `json:"metrics"`
	Status       string              `json:"status"`
	CreatedAt    int64               `json:"created_at"`
	UpdatedAt    int64               `json:"updated_at"`
	Version      int                 `json:"version"`
	Frozen       bool                `json:"frozen"`
	DriftWatch   bool                `json:"drift_watch"`
}

type behaviorBaselineSummaryDTO struct {
	Scope    string `json:"scope"`
	Total    int    `json:"total"`
	Learning int    `json:"learning"`
	Active   int    `json:"active"`
	Drift    int    `json:"drift"`
	Frozen   int    `json:"frozen"`
	Alerts   int    `json:"alerts"`
	Rebuild  int    `json:"rebuild"`
}

type behaviorBaselineDistributionDTO struct {
	MetricName string     `json:"metric_name"`
	Unit       string     `json:"unit"`
	Values     [5]float64 `json:"values"`
}

type behaviorBaselineSeriesPointDTO struct {
	Timestamp int64     `json:"timestamp"`
	Mean      float64   `json:"mean"`
	P50       float64   `json:"p50"`
	P95       float64   `json:"p95"`
	P99       float64   `json:"p99"`
	Upper     float64   `json:"upper"`
	Lower     float64   `json:"lower"`
	Samples   []float64 `json:"samples"`
}

type behaviorBaselineAnalyticsDTO struct {
	BaselineID    string                            `json:"baseline_id"`
	WindowDays    int                               `json:"window_days"`
	MetricName    string                            `json:"metric_name"`
	Unit          string                            `json:"unit"`
	Distributions []behaviorBaselineDistributionDTO `json:"distributions"`
	Series        []behaviorBaselineSeriesPointDTO  `json:"series"`
}

// behaviorBaselineOverviewDTO is the page-level, data-backed contract used by
// the five behavior-baseline tabs.  It intentionally carries source rows and
// aggregate values rather than presentation conclusions so the Web UI never
// has to invent relationships, locations, protocol shares, scan ratios or
// calendar activity.
type behaviorBaselineOverviewDTO struct {
	BaselineType string                              `json:"baseline_type"`
	WindowDays   int                                 `json:"window_days"`
	Source       string                              `json:"source"`
	KPIs         []behaviorBaselineKPIDTO            `json:"kpis"`
	Boxplots     []behaviorBaselineBoxplotDTO        `json:"boxplots"`
	Heatmap      behaviorBaselineHeatmapDTO          `json:"heatmap"`
	Calendar     behaviorBaselineHeatmapDTO          `json:"calendar"`
	Series       []behaviorBaselineOverviewSeriesDTO `json:"series"`
	Shares       []behaviorBaselineShareDTO          `json:"shares"`
	Links        []behaviorBaselineLinkDTO           `json:"links"`
	Facts        []behaviorBaselineFactDTO           `json:"facts"`
	Availability map[string]string                   `json:"availability"`
}

type behaviorBaselineKPIDTO struct {
	Key    string  `json:"key"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit,omitempty"`
	Source string  `json:"source"`
}

type behaviorBaselineBoxplotDTO struct {
	EntityID string     `json:"entity_id"`
	Values   [5]float64 `json:"values"`
	Samples  uint64     `json:"samples"`
}

type behaviorBaselineHeatmapDTO struct {
	X      []string                          `json:"x"`
	Y      []string                          `json:"y"`
	Values []behaviorBaselineHeatmapValueDTO `json:"values"`
}

type behaviorBaselineHeatmapValueDTO struct {
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Value float64 `json:"value"`
}

type behaviorBaselineOverviewSeriesDTO struct {
	Timestamp int64   `json:"timestamp"`
	Key       string  `json:"key"`
	Value     float64 `json:"value"`
}

type behaviorBaselineShareDTO struct {
	Key       string  `json:"key"`
	Sessions  uint64  `json:"sessions"`
	Bytes     uint64  `json:"bytes"`
	Share     float64 `json:"share"`
	FirstSeen int64   `json:"first_seen"`
}

type behaviorBaselineLinkDTO struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Count  uint64 `json:"count"`
	Denied uint64 `json:"denied"`
}

type behaviorBaselineFactDTO struct {
	Kind      string  `json:"kind"`
	EntityID  string  `json:"entity_id"`
	RelatedID string  `json:"related_id,omitempty"`
	Label     string  `json:"label,omitempty"`
	Value     float64 `json:"value"`
	Count     uint64  `json:"count"`
	Denied    uint64  `json:"denied,omitempty"`
	Timestamp int64   `json:"timestamp,omitempty"`
	Status    string  `json:"status,omitempty"`
}

type behaviorMetricDTO struct {
	MetricName      string                  `json:"metric_name"`
	Unit            string                  `json:"unit"`
	NormalRange     [2]float64              `json:"normal_range"`
	Mean            float64                 `json:"mean"`
	StdDev          float64                 `json:"std_dev"`
	CurrentValue    float64                 `json:"current_value,omitempty"`
	DeviationScore  float64                 `json:"deviation_score,omitempty"`
	ThresholdConfig behaviorThresholdConfig `json:"threshold_config"`
}

type behaviorThresholdConfig struct {
	WarningMultiplier float64 `json:"warning_multiplier"`
	AlertMultiplier   float64 `json:"alert_multiplier"`
}

type behaviorBaselineActionRequest struct {
	Action            string                 `json:"action"`
	Reason            string                 `json:"reason,omitempty"`
	WarningMultiplier *float64               `json:"warning_multiplier,omitempty"`
	AlertMultiplier   *float64               `json:"alert_multiplier,omitempty"`
	TargetVersion     *int                   `json:"target_version,omitempty"`
	Detail            map[string]interface{} `json:"detail,omitempty"`
}

type behaviorBaselineActionDTO struct {
	ActionID           string                 `json:"action_id"`
	TenantID           string                 `json:"tenant_id"`
	BaselineID         string                 `json:"baseline_id"`
	Action             string                 `json:"action"`
	Status             string                 `json:"status"`
	LocalStateApplied  bool                   `json:"local_state_applied"`
	DownstreamStatus   string                 `json:"downstream_status,omitempty"`
	DownstreamAttempts int                    `json:"downstream_attempts,omitempty"`
	DownstreamError    string                 `json:"downstream_error,omitempty"`
	Reason             string                 `json:"reason"`
	Request            map[string]interface{} `json:"request"`
	RequestedBy        string                 `json:"requested_by"`
	CreatedAt          int64                  `json:"created_at"`
}

type behaviorBaselineVersionDTO struct {
	BaselineID     string                 `json:"baseline_id"`
	Version        int                    `json:"version"`
	Snapshot       map[string]interface{} `json:"snapshot"`
	SourceActionID string                 `json:"source_action_id,omitempty"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      int64                  `json:"created_at"`
}

type complianceReportDTO struct {
	ReportID    string                 `json:"report_id"`
	TenantID    string                 `json:"tenant_id"`
	ReportType  string                 `json:"report_type"`
	TimeRange   map[string]int64       `json:"time_range"`
	GeneratedAt int64                  `json:"generated_at"`
	Status      string                 `json:"status"`
	Summary     complianceSummaryDTO   `json:"summary"`
	Sections    []complianceSectionDTO `json:"sections"`
	GeneratedBy string                 `json:"generated_by"`
}

type complianceExportDTO struct {
	ExportID      string `json:"export_id"`
	ReportID      string `json:"report_id"`
	ArtifactType  string `json:"artifact_type"`
	Filename      string `json:"filename"`
	MIMEType      string `json:"mime_type"`
	SHA256        string `json:"sha256"`
	ContentBase64 string `json:"content_base64"`
	GeneratedAt   int64  `json:"generated_at"`
}

type complianceSummaryDTO struct {
	TotalAlerts        int64   `json:"total_alerts"`
	CriticalAlerts     int64   `json:"critical_alerts"`
	ResolvedAlerts     int64   `json:"resolved_alerts"`
	FalsePositives     int64   `json:"false_positives"`
	AvgResponseTimeMin float64 `json:"avg_response_time_min"`
	SLAViolations      int64   `json:"sla_violations"`
}

type complianceSectionDTO struct {
	SectionName string                 `json:"section_name"`
	Title       string                 `json:"title"`
	Content     map[string]interface{} `json:"content"`
	Status      string                 `json:"status"`
}

type topicViewDTO struct {
	ViewID     string                 `json:"view_id"`
	TenantID   string                 `json:"tenant_id"`
	Topic      string                 `json:"topic"`
	Name       string                 `json:"name"`
	Filters    map[string]interface{} `json:"filters"`
	Visibility string                 `json:"visibility"`
	Favorite   bool                   `json:"favorite"`
	Shared     bool                   `json:"shared"`
	ShareToken string                 `json:"share_token,omitempty"`
	CreatedBy  string                 `json:"created_by"`
	CreatedAt  int64                  `json:"created_at"`
	UpdatedAt  int64                  `json:"updated_at"`
}

type topicScopeDTO struct {
	TenantID       string                 `json:"tenant_id"`
	Topic          string                 `json:"topic"`
	ScopeName      string                 `json:"scope_name"`
	IncludedAssets []string               `json:"included_assets"`
	ExcludedAssets []string               `json:"excluded_assets"`
	RiskLevels     []string               `json:"risk_levels"`
	TimeWindow     string                 `json:"time_window"`
	UpdatedBy      string                 `json:"updated_by"`
	UpdatedAt      int64                  `json:"updated_at"`
	Detail         map[string]interface{} `json:"detail"`
}

type topicSubscriptionDTO struct {
	SubscriptionID string                 `json:"subscription_id"`
	TenantID       string                 `json:"tenant_id"`
	Topic          string                 `json:"topic"`
	Channel        string                 `json:"channel"`
	Threshold      string                 `json:"threshold"`
	Schedule       string                 `json:"schedule"`
	Recipients     []string               `json:"recipients"`
	Enabled        bool                   `json:"enabled"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      int64                  `json:"created_at"`
	UpdatedAt      int64                  `json:"updated_at"`
	Detail         map[string]interface{} `json:"detail"`
}

type topicExportDTO struct {
	ExportID    string                 `json:"export_id"`
	TenantID    string                 `json:"tenant_id"`
	Topic       string                 `json:"topic"`
	ExportType  string                 `json:"export_type"`
	Status      string                 `json:"status"`
	Parameters  map[string]interface{} `json:"parameters"`
	Result      map[string]interface{} `json:"result"`
	GeneratedBy string                 `json:"generated_by"`
	GeneratedAt int64                  `json:"generated_at"`
}

type topicSaveViewRequest struct {
	Topic      string                 `json:"topic"`
	Name       string                 `json:"name"`
	Filters    map[string]interface{} `json:"filters"`
	Visibility string                 `json:"visibility"`
	Favorite   bool                   `json:"favorite"`
}

type topicViewUpdateRequest struct {
	Name       string                 `json:"name"`
	Filters    map[string]interface{} `json:"filters"`
	Visibility string                 `json:"visibility"`
	Favorite   *bool                  `json:"favorite"`
	Shared     *bool                  `json:"shared"`
}

type topicScopeUpdateRequest struct {
	ScopeName      string                 `json:"scope_name"`
	IncludedAssets []string               `json:"included_assets"`
	ExcludedAssets []string               `json:"excluded_assets"`
	RiskLevels     []string               `json:"risk_levels"`
	TimeWindow     string                 `json:"time_window"`
	Detail         map[string]interface{} `json:"detail"`
}

type topicSubscriptionRequest struct {
	Topic      string                 `json:"topic"`
	Channel    string                 `json:"channel"`
	Threshold  string                 `json:"threshold"`
	Schedule   string                 `json:"schedule"`
	Recipients []string               `json:"recipients"`
	Enabled    *bool                  `json:"enabled"`
	Detail     map[string]interface{} `json:"detail"`
}

type topicSubscriptionUpdateRequest struct {
	Channel    string                 `json:"channel"`
	Threshold  string                 `json:"threshold"`
	Schedule   string                 `json:"schedule"`
	Recipients []string               `json:"recipients"`
	Enabled    *bool                  `json:"enabled"`
	Detail     map[string]interface{} `json:"detail"`
}

type topicExportRequest struct {
	Topic      string                 `json:"topic"`
	Format     string                 `json:"format"`
	TimeRange  *complianceRange       `json:"time_range"`
	Parameters map[string]interface{} `json:"parameters"`
}

type auditTrailDTO struct {
	LogID        string                 `json:"log_id"`
	TenantID     string                 `json:"tenant_id"`
	UserID       string                 `json:"user_id"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id"`
	Details      map[string]interface{} `json:"details"`
	IPAddress    string                 `json:"ip_address"`
	Timestamp    int64                  `json:"timestamp"`
	Result       string                 `json:"result"`
}

func (h *SystemHandler) GetEncryptedTrafficStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := writeTenantID(r)
	if h.serveEncryptedTrafficReferenceFixture(w, r, tenantID, "stats") {
		return
	}
	start, end := queryTimeRange(r, 24*time.Hour)

	var total, tls, quic, ssh uint64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT count(),
		       countIf(protocol != 17 AND dst_port IN (443, 8443, 853, 993, 995, 465)),
		       countIf(protocol = 17 AND dst_port IN (443, 8443)),
		       countIf(dst_port = 22)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?`, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := row.Scan(&total, &tls, &quic, &ssh); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	encrypted := tls + quic + ssh
	stats := encryptedTrafficStatsDTO{
		TotalSessions:    int64(encrypted),
		ObservedSessions: int64(total),
		TLSSessions:      int64(tls),
		QUICSessions:     int64(quic),
		JA3Fingerprints:  0,
	}
	if total > 0 {
		stats.EncryptedRatio = float64(encrypted) / float64(total)
	}
	if h.encryptedEvidenceFingerprintTableAvailable(ctx) {
		row, queryErr := h.chClient.QueryRow(ctx, `
			SELECT uniqExact(ja3),
			       uniqExactIf(ja3, entropy_payload >= 7.5 OR cert_is_self_signed = 1)
			FROM traffic.feature_fp
			WHERE tenant_id=? AND is_encrypted=1 AND ja3!=''
			  AND toUnixTimestamp64Milli(ts)>=? AND toUnixTimestamp64Milli(ts)<=?`, tenantID, start, end)
		if queryErr == nil {
			var fingerprints, malicious uint64
			if scanErr := row.Scan(&fingerprints, &malicious); scanErr == nil {
				stats.JA3Fingerprints = int64(fingerprints)
				stats.MaliciousJA3Matches = int64(malicious)
			}
		}
	}
	httpx.JSONSuccess(w, ctx, stats)
}

func (h *SystemHandler) ListEncryptedTrafficSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := writeTenantID(r)
	if h.serveEncryptedTrafficReferenceFixture(w, r, tenantID, "sessions") {
		return
	}
	// The workbench applies entity/status/version filters across the full tab
	// scope. A behavior-baseline row is an aggregate (not a raw event), so a
	// larger bounded page is safe and avoids silently filtering only the first
	// 100/200 entities.
	limit, offset := parsePageLimitOffset(r, 20, 5000)
	start, end := queryTimeRange(r, 24*time.Hour)
	riskFilter := r.URL.Query().Get("risk_level")
	protocolFilter := strings.ToUpper(r.URL.Query().Get("protocol"))

	rows, err := h.chClient.Query(ctx, `
		SELECT session_id, community_id, src_ip, dst_ip, dst_port, protocol,
		       bytes_total, num_pkts, ts_start, ts_end, duration_ms
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (443, 8443, 853, 993, 995, 465, 22)
		ORDER BY ts_end DESC
		LIMIT ? OFFSET ?`, tenantID, start, end, limit*3, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	sessions := make([]encryptedTrafficSessionDTO, 0, limit)
	for rows.Next() {
		var item encryptedTrafficSessionDTO
		var proto uint8
		var duration uint32
		if err := rows.Scan(&item.SessionID, &item.CommunityID, &item.SrcIP, &item.DstIP, &item.DstPort, &proto, &item.ByteCount, &item.PacketCount, &item.StartTime, &item.EndTime, &duration); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		item.Protocol = encryptedProtocol(proto, item.DstPort)
		item.TLSVersion = tlsVersionLabel(item.Protocol)
		item.AnomalyScore = encryptedAnomalyScore(item.ByteCount, duration, item.DstPort)
		item.EntropyScore = item.AnomalyScore
		item.RiskLevel = encryptedRisk(item.AnomalyScore)
		if riskFilter != "" && item.RiskLevel != riskFilter {
			continue
		}
		if protocolFilter != "" && strings.ToUpper(item.Protocol) != protocolFilter {
			continue
		}
		sessions = append(sessions, item)
		if len(sessions) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if h.encryptedEvidenceFingerprintTableAvailable(ctx) {
		h.enrichEncryptedEvidenceFingerprints(ctx, tenantID, sessions)
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"sessions": sessions, "total": len(sessions)})
}

func (h *SystemHandler) ListJA3Fingerprints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := writeTenantID(r)
	if h.serveEncryptedTrafficReferenceFixture(w, r, tenantID, "ja3") {
		return
	}
	if !h.encryptedEvidenceFingerprintTableAvailable(ctx) {
		httpx.JSONSuccess(w, ctx, map[string]interface{}{
			"fingerprints": []encryptedJA3FingerprintDTO{},
			"total":        0,
			"source_state": "unavailable",
		})
		return
	}

	limit, offset := parsePageLimitOffset(r, 20, 200)
	start, end := queryTimeRange(r, 24*time.Hour)
	var total, totalSessions uint64
	countRow, err := h.chClient.QueryRow(ctx, `
		SELECT uniqExact(ja3), uniqExact(session_id)
		FROM traffic.feature_fp
		WHERE tenant_id=? AND is_encrypted=1 AND ja3!=''
		  AND toUnixTimestamp64Milli(ts)>=? AND toUnixTimestamp64Milli(ts)<=?`, tenantID, start, end)
	if err != nil || countRow.Scan(&total, &totalSessions) != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to query JA3 fingerprint totals")
		return
	}

	rows, err := h.chClient.Query(ctx, `
		SELECT ja3,
		       any(tls_version),
		       uniqExact(session_id) AS session_count,
		       uniqExactIf(sni_hash, sni_hash!='') AS sni_count,
		       avg(entropy_payload) AS entropy_average,
		       max(cert_is_self_signed) AS has_self_signed
		FROM traffic.feature_fp
		WHERE tenant_id=? AND is_encrypted=1 AND ja3!=''
		  AND toUnixTimestamp64Milli(ts)>=? AND toUnixTimestamp64Milli(ts)<=?
		GROUP BY ja3
		ORDER BY session_count DESC, ja3 ASC
		LIMIT ? OFFSET ?`, tenantID, start, end, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	fingerprints := make([]encryptedJA3FingerprintDTO, 0, limit)
	for rows.Next() {
		var item encryptedJA3FingerprintDTO
		var selfSigned uint8
		if err := rows.Scan(&item.JA3Fingerprint, &item.TLSVersion, &item.SessionCount, &item.SNICount, &item.EntropyAverage, &selfSigned); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		if totalSessions > 0 {
			item.TrafficRatio = float64(item.SessionCount) / float64(totalSessions)
		}
		item.RiskLevel = "normal"
		if selfSigned == 1 || item.EntropyAverage >= 7.5 {
			item.RiskLevel = "malicious"
		} else if item.EntropyAverage >= 6.5 {
			item.RiskLevel = "suspicious"
		}
		fingerprints = append(fingerprints, item)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"fingerprints": fingerprints,
		"total":        total,
		"source_state": "live",
	})
}

func (h *SystemHandler) GetEncryptedTunnelAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := writeTenantID(r)
	if h.serveEncryptedTrafficReferenceFixture(w, r, tenantID, "tunnels") {
		return
	}
	start, end := queryTimeRange(r, 24*time.Hour)

	protocols, err := h.queryTunnelProtocols(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	users, err := h.queryTunnelUsers(ctx, tenantID, start, end, 10)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"protocols": protocols,
		"users":     users,
		"total":     len(users),
	})
}

func (h *SystemHandler) GetEncryptedExfiltrationAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := writeTenantID(r)
	if h.serveEncryptedTrafficReferenceFixture(w, r, tenantID, "exfiltration") {
		return
	}
	limit, _ := parsePageLimitOffset(r, 10, 50)
	start, end := queryTimeRange(r, 24*time.Hour)

	sources, err := h.queryExfiltrationSources(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	risks, err := h.queryExfiltrationRisks(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	paths, err := h.queryExfiltrationPaths(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	destinations, err := h.queryExfiltrationDestinations(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	trend, err := h.queryExfiltrationTrend(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"top_sources":      sources,
		"top_destinations": destinations,
		"risk_types":       risks,
		"paths":            paths,
		"trend":            trend,
		"total":            len(destinations),
	})
}

// GetEncryptedTrafficEvidence aggregates session and PCAP-index evidence for the encrypted evidence center.
func (h *SystemHandler) GetEncryptedTrafficEvidence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := writeTenantID(r)
	if h.serveEncryptedTrafficReferenceFixture(w, r, tenantID, "evidence") {
		return
	}
	limit, _ := parsePageLimitOffset(r, 12, 50)
	start, end := queryTimeRange(r, 24*time.Hour)

	sessions, err := h.queryEncryptedEvidenceSessions(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	pcapIndexes, err := h.queryEncryptedEvidencePcapIndexes(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	pcapTrend, err := h.queryEncryptedEvidencePcapTrend(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	entropyTrend := []encryptedEvidenceEntropyTrendDTO{}
	entropyAvailable := false
	if h.encryptedEvidenceFingerprintTableAvailable(ctx) {
		entropyTrend, err = h.queryEncryptedEvidenceEntropyTrend(ctx, tenantID, start, end)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		entropyAvailable = true
	}

	completeness := encryptedEvidenceCompleteness(sessions, pcapIndexes)
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"sessions":          sessions,
		"pcap_indexes":      pcapIndexes,
		"pcap_trend":        pcapTrend,
		"entropy_trend":     entropyTrend,
		"entropy_available": entropyAvailable,
		"anomaly_trend":     encryptedEvidenceAnomalyTrend(sessions),
		"completeness":      completeness,
		"total":             len(sessions),
	})
}

// serveEncryptedTrafficReferenceFixture exposes an explicitly activated, database-backed
// canonical UI dataset. Absence of the fixture always falls through to the live ClickHouse path.
func (h *SystemHandler) serveEncryptedTrafficReferenceFixture(w http.ResponseWriter, r *http.Request, tenantID, endpoint string) bool {
	if h.pgDB == nil {
		return false
	}
	var payload []byte
	err := h.pgDB.QueryRowContext(r.Context(), `
		SELECT payload
		FROM encrypted_traffic_ui_fixtures
		WHERE tenant_id=$1 AND endpoint=$2 AND active=true
		LIMIT 1`, tenantID, endpoint).Scan(&payload)
	if err != nil {
		return false
	}
	var decoded interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		if h.logger != nil {
			h.logger.Warn("invalid encrypted traffic UI fixture", zap.String("tenant_id", tenantID), zap.String("endpoint", endpoint), zap.Error(err))
		}
		return false
	}
	httpx.JSONSuccess(w, r.Context(), decoded)
	return true
}

func (h *SystemHandler) queryEncryptedEvidenceSessions(ctx context.Context, tenantID string, start, end int64, limit int) ([]encryptedTrafficSessionDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT session_id, community_id, src_ip, dst_ip, dst_port, protocol,
		       bytes_total, num_pkts, evidence_count, ts_start, ts_end, duration_ms
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (443, 8443, 853, 993, 995, 465, 22)
		ORDER BY ts_end DESC
		LIMIT ?`, tenantID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]encryptedTrafficSessionDTO, 0, limit)
	for rows.Next() {
		var item encryptedTrafficSessionDTO
		var proto uint8
		var duration uint32
		if err := rows.Scan(&item.SessionID, &item.CommunityID, &item.SrcIP, &item.DstIP, &item.DstPort, &proto, &item.ByteCount, &item.PacketCount, &item.EvidenceCount, &item.StartTime, &item.EndTime, &duration); err != nil {
			return nil, err
		}
		item.Protocol = encryptedProtocol(proto, item.DstPort)
		item.TLSVersion = tlsVersionLabel(item.Protocol)
		item.AnomalyScore = encryptedAnomalyScore(item.ByteCount, duration, item.DstPort)
		item.EntropyScore = item.AnomalyScore
		item.RiskLevel = encryptedRisk(item.AnomalyScore)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if h.encryptedEvidenceFingerprintTableAvailable(ctx) {
		h.enrichEncryptedEvidenceFingerprints(ctx, tenantID, items)
	}
	return items, nil
}

func (h *SystemHandler) encryptedEvidenceFingerprintTableAvailable(ctx context.Context) bool {
	if h.chClient == nil {
		return false
	}
	var count uint64
	row, err := h.chClient.QueryRow(ctx, `SELECT count() FROM system.tables WHERE database='traffic' AND name='feature_fp'`)
	return err == nil && row.Scan(&count) == nil && count > 0
}

// enrichEncryptedEvidenceFingerprints is optional because older clusters may not yet provision traffic.feature_fp.
func (h *SystemHandler) enrichEncryptedEvidenceFingerprints(ctx context.Context, tenantID string, sessions []encryptedTrafficSessionDTO) {
	for index := range sessions {
		item := &sessions[index]
		row, err := h.chClient.QueryRow(ctx, `
			SELECT tls_version, ja3, sni_hash, cert_sha256
			FROM traffic.feature_fp
			WHERE tenant_id=? AND community_id=? AND session_id=?
			ORDER BY ts DESC
			LIMIT 1`, tenantID, item.CommunityID, item.SessionID)
		if err != nil {
			continue
		}
		var tlsVersion, ja3, sniHash, certificateHash string
		if err := row.Scan(&tlsVersion, &ja3, &sniHash, &certificateHash); err != nil {
			continue
		}
		item.HasHandshakeMetadata = tlsVersion != "" || ja3 != "" || sniHash != "" || certificateHash != ""
		if tlsVersion != "" {
			item.TLSVersion = tlsVersion
		}
		item.JA3Fingerprint = ja3
		item.SNIHash = sniHash
		item.CertificateHash = certificateHash
	}
}

func (h *SystemHandler) queryEncryptedEvidencePcapIndexes(ctx context.Context, tenantID string, start, end int64, limit int) ([]encryptedEvidencePcapDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		WITH
			multiIf(ts_start >= 100000000000000000, intDiv(ts_start, 1000000), ts_start >= 100000000000000, intDiv(ts_start, 1000), ts_start) AS start_ms,
			multiIf(ts_end >= 100000000000000000, intDiv(ts_end, 1000000), ts_end >= 100000000000000, intDiv(ts_end, 1000), ts_end) AS end_ms
		SELECT file_key, probe_id, start_ms, end_ms, packet_count, byte_count, s3_path, sha256, compressed_size
		FROM traffic.pcap_index
		WHERE tenant_id=? AND start_ms>=? AND start_ms<=?
		ORDER BY end_ms DESC
		LIMIT ?`, tenantID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]encryptedEvidencePcapDTO, 0, limit)
	for rows.Next() {
		var item encryptedEvidencePcapDTO
		var packets uint64
		if err := rows.Scan(&item.FileKey, &item.ProbeID, &item.StartTime, &item.EndTime, &packets, &item.ByteCount, &item.StoragePath, &item.SHA256, &item.CompressedSize); err != nil {
			return nil, err
		}
		item.PacketCount = int64(packets)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SystemHandler) queryEncryptedEvidencePcapTrend(ctx context.Context, tenantID string, start, end int64) ([]encryptedEvidenceTrendDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		WITH multiIf(ts_start >= 100000000000000000, intDiv(ts_start, 1000000), ts_start >= 100000000000000, intDiv(ts_start, 1000), ts_start) AS start_ms
		SELECT intDiv(start_ms, 300000) * 300000 AS bucket_start, sum(byte_count), sum(packet_count)
		FROM traffic.pcap_index
		WHERE tenant_id=? AND start_ms>=? AND start_ms<=?
		GROUP BY bucket_start
		ORDER BY bucket_start
		LIMIT 48`, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trend := make([]encryptedEvidenceTrendDTO, 0, 48)
	for rows.Next() {
		var item encryptedEvidenceTrendDTO
		var packets uint64
		if err := rows.Scan(&item.BucketStart, &item.ByteCount, &packets); err != nil {
			return nil, err
		}
		item.PacketCount = int64(packets)
		trend = append(trend, item)
	}
	return trend, rows.Err()
}

func (h *SystemHandler) queryEncryptedEvidenceEntropyTrend(ctx context.Context, tenantID string, start, end int64) ([]encryptedEvidenceEntropyTrendDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT intDiv(toUnixTimestamp64Milli(ts), 300000) * 300000 AS bucket_start,
		       avg(entropy_payload) AS entropy_score
		FROM traffic.feature_fp
		WHERE tenant_id=? AND is_encrypted=1
		  AND toUnixTimestamp64Milli(ts)>=? AND toUnixTimestamp64Milli(ts)<=?
		GROUP BY bucket_start
		ORDER BY bucket_start
		LIMIT 48`, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trend := make([]encryptedEvidenceEntropyTrendDTO, 0, 48)
	for rows.Next() {
		var item encryptedEvidenceEntropyTrendDTO
		if err := rows.Scan(&item.BucketStart, &item.EntropyScore); err != nil {
			return nil, err
		}
		trend = append(trend, item)
	}
	return trend, rows.Err()
}

func encryptedEvidenceCompleteness(sessions []encryptedTrafficSessionDTO, pcaps []encryptedEvidencePcapDTO) []encryptedEvidenceCompletenessDTO {
	var sessionComplete, pcapLinked, metadataComplete, hashComplete int64
	for _, item := range sessions {
		if item.SessionID != "" && item.SrcIP != "" && item.DstIP != "" {
			sessionComplete++
		}
		if item.PcapIndex != "" {
			pcapLinked++
		}
		if item.HasHandshakeMetadata {
			metadataComplete++
		}
	}
	for _, item := range pcaps {
		if item.SHA256 != "" {
			hashComplete++
		}
	}
	return []encryptedEvidenceCompletenessDTO{
		{Label: "Session", Complete: sessionComplete, Total: int64(len(sessions))},
		{Label: "PCAP关联", Complete: pcapLinked, Total: int64(len(sessions))},
		{Label: "握手", Complete: metadataComplete, Total: int64(len(sessions))},
		{Label: "索引Hash", Complete: hashComplete, Total: int64(len(pcaps))},
	}
}

func encryptedEvidenceAnomalyTrend(sessions []encryptedTrafficSessionDTO) []encryptedEvidenceAnomalyTrendDTO {
	trend := make([]encryptedEvidenceAnomalyTrendDTO, 0, len(sessions))
	for _, session := range sessions {
		trend = append(trend, encryptedEvidenceAnomalyTrendDTO{BucketStart: session.StartTime, AnomalyScore: session.AnomalyScore})
	}
	return trend
}

// SubmitEncryptedTrafficEgressAction persists an operator request before the UI reports success.
// Alert creation remains an asynchronous rule-service decision; this endpoint records the request.
func (h *SystemHandler) SubmitEncryptedTrafficEgressAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}

	var req encryptedTrafficEgressActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid encrypted egress action payload")
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Target = strings.TrimSpace(req.Target)
	req.DataMode = strings.TrimSpace(req.DataMode)
	if req.Target == "" || len(req.Target) > 255 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "target is required and must be at most 255 characters")
		return
	}
	if req.DataMode == "" {
		req.DataMode = "unavailable"
	}

	auditEvent, ok := encryptedEgressAuditEvents[req.Action]
	if !ok {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "unsupported encrypted egress action")
		return
	}

	tenantID := writeTenantID(r)
	actionID := fmt.Sprintf("egress-%d", time.Now().UTC().UnixNano())
	if err := h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), auditEvent, "encrypted_egress_action", actionID, map[string]interface{}{
		"action": req.Action, "target": req.Target, "data_mode": req.DataMode,
	}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "failed to persist encrypted egress action audit")
		return
	}

	httpx.JSONSuccess(w, ctx, map[string]string{
		"action_id":   actionID,
		"action":      req.Action,
		"audit_event": auditEvent,
		"status":      "recorded",
		"target":      req.Target,
	})
}

var encryptedEgressAuditEvents = map[string]string{
	"create_alert":     "ENCRYPTED_EGRESS_ALERT_REQUESTED",
	"lookup_evidence":  "ENCRYPTED_EGRESS_EVIDENCE_LOOKUP",
	"entity_graph":     "ENCRYPTED_EGRESS_GRAPH_DRILLDOWN",
	"write_audit":      "ENCRYPTED_EGRESS_AUDIT_WRITTEN",
	"response_request": "ENCRYPTED_EGRESS_RESPONSE_REQUESTED",
}

// SubmitEncryptedTrafficEvidenceAction records evidence-center operator requests before reporting success.
func (h *SystemHandler) SubmitEncryptedTrafficEvidenceAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) {
		return
	}

	var req encryptedTrafficEvidenceActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid encrypted evidence action payload")
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Target = strings.TrimSpace(req.Target)
	req.DataMode = strings.TrimSpace(req.DataMode)
	if req.Target == "" || len(req.Target) > 255 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "target is required and must be at most 255 characters")
		return
	}
	if req.DataMode == "" {
		req.DataMode = "unavailable"
	}
	if req.DataMode != "live" && req.DataMode != "partial" && req.DataMode != "simulated" && req.DataMode != "unavailable" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "unsupported evidence data_mode")
		return
	}

	auditEvent, ok := encryptedEvidenceAuditEvents[req.Action]
	if !ok {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "unsupported encrypted evidence action")
		return
	}
	if !h.requirePostgres(w, ctx) {
		return
	}

	actionID := fmt.Sprintf("evidence-%d", time.Now().UTC().UnixNano())
	if err := h.insertAuditLog(ctx, writeTenantID(r), httpx.GetUserID(ctx), auditEvent, "encrypted_evidence_action", actionID, map[string]interface{}{
		"action": req.Action, "target": req.Target, "data_mode": req.DataMode,
	}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "failed to persist encrypted evidence action audit")
		return
	}

	httpx.JSONSuccess(w, ctx, map[string]string{
		"action_id":   actionID,
		"action":      req.Action,
		"audit_event": auditEvent,
		"status":      "recorded",
		"target":      req.Target,
	})
}

var encryptedEvidenceAuditEvents = map[string]string{
	"create_task":           "ENCRYPTED_EVIDENCE_TASK_REQUESTED",
	"download_pcap":         "ENCRYPTED_EVIDENCE_PCAP_DOWNLOAD_REQUESTED",
	"verify_hash":           "ENCRYPTED_EVIDENCE_HASH_VERIFICATION_REQUESTED",
	"export_package":        "ENCRYPTED_EVIDENCE_EXPORT_REQUESTED",
	"associate_analysis":    "ENCRYPTED_EVIDENCE_ANALYSIS_REQUESTED",
	"preserve_evidence":     "ENCRYPTED_EVIDENCE_PRESERVATION_REQUESTED",
	"link_alert":            "ENCRYPTED_EVIDENCE_ALERT_LINK_REQUESTED",
	"expert_review":         "ENCRYPTED_EVIDENCE_EXPERT_REVIEW_REQUESTED",
	"mark_gap":              "ENCRYPTED_EVIDENCE_GAP_MARKED",
	"submit_recommendation": "ENCRYPTED_EVIDENCE_RECOMMENDATION_SUBMITTED",
	"export_report":         "ENCRYPTED_EVIDENCE_REPORT_EXPORT_REQUESTED",
	"write_audit":           "ENCRYPTED_EVIDENCE_AUDIT_WRITTEN",
}

func (h *SystemHandler) GetTunnelTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	start, end := queryTimeRange(r, 24*time.Hour)

	protocols, err := h.queryTunnelProtocols(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	users, err := h.queryTunnelUsers(ctx, tenantID, start, end, 20)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	var sessionCount int64
	var totalBytes uint64
	for _, protocol := range protocols {
		sessionCount += protocol.Count
		totalBytes += protocol.TotalBytes
	}

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"topic":      "tunnel",
		"updated_at": time.Now().UnixMilli(),
		"time_range": map[string]int64{"start": start, "end": end},
		"summary": map[string]interface{}{
			"protocol_count":  len(protocols),
			"active_users":    len(users),
			"session_count":   sessionCount,
			"total_bytes":     totalBytes,
			"high_risk_users": countTunnelUsersByRisk(users, "high"),
		},
		"protocols": protocols,
		"users":     users,
	})
}

func (h *SystemHandler) GetExfiltrationTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	limit, _ := parsePageLimitOffset(r, 20, 100)
	start, end := queryTimeRange(r, 24*time.Hour)

	sources, err := h.queryExfiltrationSources(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	risks, err := h.queryExfiltrationRisks(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	paths, err := h.queryExfiltrationPaths(ctx, tenantID, start, end, limit)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	var uploadBytes uint64
	var sessionCount int64
	for _, source := range sources {
		uploadBytes += source.UploadBytes
		sessionCount += source.SessionCount
	}

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"topic":      "exfil",
		"updated_at": time.Now().UnixMilli(),
		"time_range": map[string]int64{"start": start, "end": end},
		"summary": map[string]interface{}{
			"source_count":      len(sources),
			"path_count":        len(paths),
			"session_count":     sessionCount,
			"upload_bytes":      uploadBytes,
			"high_risk_sources": countExfiltrationSourcesByRisk(sources, "high"),
		},
		"top_sources": sources,
		"risk_types":  risks,
		"paths":       paths,
	})
}

func (h *SystemHandler) GetAPTTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	limit, _ := parsePageLimitOffset(r, 20, 100)
	start, end := queryTimeRange(r, 7*24*time.Hour)

	campaigns, total, err := h.queryCampaigns(ctx, tenantID, campaignQueryFilters{}, start, end, limit, 0)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	phaseDistribution := map[string]int{}
	entities := map[string]bool{}
	alertCount := 0
	highRisk := 0
	for _, campaign := range campaigns {
		alertCount += len(campaign.Alerts)
		if campaign.Score >= 0.75 {
			highRisk++
		}
		for _, phase := range campaign.AttackPhases {
			if phase != "" {
				phaseDistribution[phase]++
			}
		}
		for _, entity := range campaign.Entities {
			if entity != "" {
				entities[entity] = true
			}
		}
	}

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"topic":              "apt",
		"updated_at":         time.Now().UnixMilli(),
		"time_range":         map[string]int64{"start": start, "end": end},
		"campaigns":          campaigns,
		"phase_distribution": phaseDistribution,
		"summary": map[string]interface{}{
			"campaign_count":   total,
			"listed_campaigns": len(campaigns),
			"high_risk_count":  highRisk,
			"entity_count":     len(entities),
			"alert_count":      alertCount,
		},
	})
}

func (h *SystemHandler) ListTopicViews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 100)
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))

	args := []interface{}{tenantID}
	where := "tenant_id=$1"
	if topic != "" {
		if !isValidTopicKey(topic) {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid topic")
			return
		}
		args = append(args, topic)
		where += fmt.Sprintf(" AND topic=$%d", len(args))
	}
	var total int
	if err := h.pgDB.QueryRowContext(ctx, "SELECT count(*) FROM topic_saved_views WHERE "+where, args...).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	args = append(args, limit, offset)
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT view_id::text, tenant_id, topic, name, filters::text, visibility, favorite, shared, COALESCE(share_token, ''), created_by, created_at, updated_at
		FROM topic_saved_views WHERE `+where+`
		ORDER BY updated_at DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()
	views := make([]topicViewDTO, 0, limit)
	for rows.Next() {
		view, err := scanTopicView(rows)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"views": views, "total": total})
}

func (h *SystemHandler) SaveTopicView(w http.ResponseWriter, r *http.Request) {
	if !h.requireTopicWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	var req topicSaveViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.Topic = normalizeTopicKey(req.Topic)
	req.Name = strings.TrimSpace(req.Name)
	req.Visibility = topicVisibility(req.Visibility)
	if req.Filters == nil {
		req.Filters = map[string]interface{}{}
	}
	if req.Name == "" || !isValidTopicKey(req.Topic) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "topic and name are required")
		return
	}
	filtersJSON, _ := json.Marshal(req.Filters)
	view, err := scanTopicView(h.pgDB.QueryRowContext(ctx, `
		INSERT INTO topic_saved_views (tenant_id, topic, name, filters, visibility, favorite, created_by)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		RETURNING view_id::text, tenant_id, topic, name, filters::text, visibility, favorite, shared, COALESCE(share_token, ''), created_by, created_at, updated_at`,
		tenantID, req.Topic, req.Name, string(filtersJSON), req.Visibility, req.Favorite, httpx.GetUserID(ctx)))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "TOPIC_VIEW_SAVED", "topic_saved_view", view.ViewID, map[string]interface{}{
		"topic": req.Topic, "name": req.Name, "visibility": req.Visibility, "favorite": req.Favorite,
	}, r)
	httpx.JSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": view})
}

func (h *SystemHandler) UpdateTopicView(w http.ResponseWriter, r *http.Request) {
	if !h.requireTopicWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	viewID := strings.TrimSpace(mux.Vars(r)["id"])
	var req topicViewUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	current, err := h.queryTopicView(ctx, tenantID, viewID)
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "topic view not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	name := firstNonEmpty(strings.TrimSpace(req.Name), current.Name)
	visibility := current.Visibility
	if req.Visibility != "" {
		visibility = topicVisibility(req.Visibility)
	}
	filters := current.Filters
	if req.Filters != nil {
		filters = req.Filters
	}
	favorite := current.Favorite
	if req.Favorite != nil {
		favorite = *req.Favorite
	}
	shared := current.Shared
	if req.Shared != nil {
		shared = *req.Shared
	}
	shareToken := current.ShareToken
	if shared && shareToken == "" {
		shareToken = fmt.Sprintf("topic-share-%d", time.Now().UnixNano())
	}
	if !shared {
		shareToken = ""
	}
	filtersJSON, _ := json.Marshal(filters)
	view, err := scanTopicView(h.pgDB.QueryRowContext(ctx, `
		UPDATE topic_saved_views
		SET name=$3, filters=$4::jsonb, visibility=$5, favorite=$6, shared=$7, share_token=NULLIF($8, ''), updated_at=now()
		WHERE tenant_id=$1 AND view_id=$2
		RETURNING view_id::text, tenant_id, topic, name, filters::text, visibility, favorite, shared, COALESCE(share_token, ''), created_by, created_at, updated_at`,
		tenantID, viewID, name, string(filtersJSON), visibility, favorite, shared, shareToken))
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "topic view not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	action := "TOPIC_VIEW_UPDATED"
	if req.Shared != nil && *req.Shared {
		action = "TOPIC_VIEW_SHARED"
	} else if req.Favorite != nil {
		action = "TOPIC_VIEW_FAVORITE_UPDATED"
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), action, "topic_saved_view", view.ViewID, map[string]interface{}{
		"topic": view.Topic, "favorite": view.Favorite, "shared": view.Shared, "visibility": view.Visibility,
	}, r)
	httpx.JSONSuccess(w, ctx, view)
}

func (h *SystemHandler) UpdateTopicScope(w http.ResponseWriter, r *http.Request) {
	if !h.requireTopicWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	topic := normalizeTopicKey(mux.Vars(r)["topic"])
	if !isValidTopicKey(topic) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid topic")
		return
	}
	var req topicScopeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.ScopeName = firstNonEmpty(strings.TrimSpace(req.ScopeName), "默认专题范围")
	req.TimeWindow = firstNonEmpty(strings.TrimSpace(req.TimeWindow), "24h")
	if req.Detail == nil {
		req.Detail = map[string]interface{}{}
	}
	includedJSON, _ := json.Marshal(cleanStringList(req.IncludedAssets))
	excludedJSON, _ := json.Marshal(cleanStringList(req.ExcludedAssets))
	riskJSON, _ := json.Marshal(cleanStringList(req.RiskLevels))
	detailJSON, _ := json.Marshal(req.Detail)
	scope, err := scanTopicScope(h.pgDB.QueryRowContext(ctx, `
		INSERT INTO topic_scope_overrides (tenant_id, topic, scope_name, included_assets, excluded_assets, risk_levels, time_window, detail, updated_by)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7, $8::jsonb, $9)
		ON CONFLICT (tenant_id, topic) DO UPDATE SET
			scope_name=EXCLUDED.scope_name,
			included_assets=EXCLUDED.included_assets,
			excluded_assets=EXCLUDED.excluded_assets,
			risk_levels=EXCLUDED.risk_levels,
			time_window=EXCLUDED.time_window,
			detail=EXCLUDED.detail,
			updated_by=EXCLUDED.updated_by,
			updated_at=now()
		RETURNING tenant_id, topic, scope_name, included_assets::text, excluded_assets::text, risk_levels::text, time_window, updated_by, updated_at, detail::text`,
		tenantID, topic, req.ScopeName, string(includedJSON), string(excludedJSON), string(riskJSON), req.TimeWindow, string(detailJSON), httpx.GetUserID(ctx)))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "TOPIC_SCOPE_UPDATED", "topic_scope", topic, map[string]interface{}{
		"topic": topic, "scope_name": req.ScopeName, "time_window": req.TimeWindow, "included_assets": scope.IncludedAssets,
	}, r)
	httpx.JSONSuccess(w, ctx, scope)
}

func (h *SystemHandler) ListTopicSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 100)
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	args := []interface{}{tenantID}
	where := "tenant_id=$1"
	if topic != "" {
		if !isValidTopicKey(topic) {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid topic")
			return
		}
		args = append(args, topic)
		where += fmt.Sprintf(" AND topic=$%d", len(args))
	}
	var total int
	if err := h.pgDB.QueryRowContext(ctx, "SELECT count(*) FROM topic_subscriptions WHERE "+where, args...).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	args = append(args, limit, offset)
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT subscription_id::text, tenant_id, topic, channel, threshold, schedule, recipients::text, enabled, created_by, created_at, updated_at, detail::text
		FROM topic_subscriptions WHERE `+where+`
		ORDER BY updated_at DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()
	subscriptions := make([]topicSubscriptionDTO, 0, limit)
	for rows.Next() {
		subscription, err := scanTopicSubscription(rows)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		subscriptions = append(subscriptions, subscription)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"subscriptions": subscriptions, "total": total})
}

func (h *SystemHandler) CreateTopicSubscription(w http.ResponseWriter, r *http.Request) {
	if !h.requireTopicWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	var req topicSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.Topic = normalizeTopicKey(req.Topic)
	req.Channel = firstNonEmpty(strings.TrimSpace(req.Channel), "webhook")
	req.Threshold = firstNonEmpty(strings.TrimSpace(req.Threshold), "high")
	req.Schedule = firstNonEmpty(strings.TrimSpace(req.Schedule), "realtime")
	req.Recipients = cleanStringList(req.Recipients)
	if req.Detail == nil {
		req.Detail = map[string]interface{}{}
	}
	if !isValidTopicKey(req.Topic) || len(req.Recipients) == 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "topic and recipients are required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	recipientsJSON, _ := json.Marshal(req.Recipients)
	detailJSON, _ := json.Marshal(req.Detail)
	subscription, err := scanTopicSubscription(h.pgDB.QueryRowContext(ctx, `
		INSERT INTO topic_subscriptions (tenant_id, topic, channel, threshold, schedule, recipients, enabled, created_by, detail)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9::jsonb)
		RETURNING subscription_id::text, tenant_id, topic, channel, threshold, schedule, recipients::text, enabled, created_by, created_at, updated_at, detail::text`,
		tenantID, req.Topic, req.Channel, req.Threshold, req.Schedule, string(recipientsJSON), enabled, httpx.GetUserID(ctx), string(detailJSON)))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "TOPIC_SUBSCRIPTION_CREATED", "topic_subscription", subscription.SubscriptionID, map[string]interface{}{
		"topic": req.Topic, "channel": req.Channel, "threshold": req.Threshold, "schedule": req.Schedule,
	}, r)
	httpx.JSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": subscription})
}

func (h *SystemHandler) UpdateTopicSubscription(w http.ResponseWriter, r *http.Request) {
	if !h.requireTopicWritePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	subscriptionID := strings.TrimSpace(mux.Vars(r)["id"])
	current, err := h.queryTopicSubscription(ctx, tenantID, subscriptionID)
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "topic subscription not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	var req topicSubscriptionUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	channel := firstNonEmpty(strings.TrimSpace(req.Channel), current.Channel)
	threshold := firstNonEmpty(strings.TrimSpace(req.Threshold), current.Threshold)
	schedule := firstNonEmpty(strings.TrimSpace(req.Schedule), current.Schedule)
	recipients := current.Recipients
	if len(req.Recipients) > 0 {
		recipients = cleanStringList(req.Recipients)
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	detail := current.Detail
	if req.Detail != nil {
		detail = req.Detail
	}
	recipientsJSON, _ := json.Marshal(recipients)
	detailJSON, _ := json.Marshal(detail)
	subscription, err := scanTopicSubscription(h.pgDB.QueryRowContext(ctx, `
		UPDATE topic_subscriptions
		SET channel=$3, threshold=$4, schedule=$5, recipients=$6::jsonb, enabled=$7, detail=$8::jsonb, updated_at=now()
		WHERE tenant_id=$1 AND subscription_id=$2
		RETURNING subscription_id::text, tenant_id, topic, channel, threshold, schedule, recipients::text, enabled, created_by, created_at, updated_at, detail::text`,
		tenantID, subscriptionID, channel, threshold, schedule, string(recipientsJSON), enabled, string(detailJSON)))
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "topic subscription not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "TOPIC_SUBSCRIPTION_UPDATED", "topic_subscription", subscription.SubscriptionID, map[string]interface{}{
		"topic": subscription.Topic, "enabled": subscription.Enabled, "channel": subscription.Channel,
	}, r)
	httpx.JSONSuccess(w, ctx, subscription)
}

func (h *SystemHandler) ExportTopicReport(w http.ResponseWriter, r *http.Request) {
	h.exportTopicArtifact(w, r, "report", "TOPIC_REPORT_EXPORTED")
}

func (h *SystemHandler) ExportTopicEvidencePackage(w http.ResponseWriter, r *http.Request) {
	h.exportTopicArtifact(w, r, "evidence_package", "TOPIC_EVIDENCE_PACKAGE_EXPORTED")
}

func (h *SystemHandler) exportTopicArtifact(w http.ResponseWriter, r *http.Request, exportType, auditAction string) {
	if !h.requireTopicExportPermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) || !h.ensureTopicGovernanceSchema(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	var req topicExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.Topic = normalizeTopicKey(req.Topic)
	if !isValidTopicKey(req.Topic) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid topic")
		return
	}
	format := firstNonEmpty(strings.TrimSpace(req.Format), "json")
	if req.Parameters == nil {
		req.Parameters = map[string]interface{}{}
	}
	start, end := complianceReportRange("daily", req.TimeRange)
	req.Parameters["format"] = format
	req.Parameters["time_range"] = map[string]int64{"start": start, "end": end}
	result := map[string]interface{}{
		"file_key":       fmt.Sprintf("topics/%s/%s-%d.%s", req.Topic, exportType, time.Now().Unix(), format),
		"summary":        topicExportSummary(req.Topic, exportType),
		"retention_days": 30,
		"audit_required": true,
	}
	paramsJSON, _ := json.Marshal(req.Parameters)
	resultJSON, _ := json.Marshal(result)
	exported, err := scanTopicExport(h.pgDB.QueryRowContext(ctx, `
		INSERT INTO topic_exports (tenant_id, topic, export_type, status, parameters, result, generated_by)
		VALUES ($1, $2, $3, 'completed', $4::jsonb, $5::jsonb, $6)
		RETURNING export_id::text, tenant_id, topic, export_type, status, parameters::text, result::text, generated_by, generated_at`,
		tenantID, req.Topic, exportType, string(paramsJSON), string(resultJSON), httpx.GetUserID(ctx)))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), auditAction, "topic_export", exported.ExportID, map[string]interface{}{
		"topic": req.Topic, "export_type": exportType, "format": format,
	}, r)
	httpx.JSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": exported})
}

func (h *SystemHandler) ListFusionSources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	sources := h.fusionSources(ctx, tenantID)
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"sources": sources, "total": len(sources)})
}

func (h *SystemHandler) GetFusionStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	sources := h.fusionSources(ctx, tenantID)
	stats := h.buildFusionStats(ctx, tenantID, sources)
	httpx.JSONSuccess(w, ctx, stats)
}

func (h *SystemHandler) GetFusionValueReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	lookback := 7 * 24 * time.Hour
	if hours := parseInt64Query(r, "window_hours"); hours > 0 {
		if hours > 24*90 {
			hours = 24 * 90
		}
		lookback = time.Duration(hours) * time.Hour
	}
	start, end := queryTimeRange(r, lookback)
	sources := h.fusionSources(ctx, tenantID)
	stats := h.buildFusionStats(ctx, tenantID, sources)
	summary := h.fusionAlertValueSummary(ctx, tenantID, start, end)
	report := h.buildFusionValueReport(tenantID, start, end, sources, stats, summary)
	httpx.JSONSuccess(w, ctx, report)
}

func (h *SystemHandler) GetFusionWorkbench(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	ruleLimit := boundedPositiveIntQuery(r, "rule_limit", 100, 200)
	ruleOffset := boundedIntQuery(r, "rule_offset", 0, 1000000)
	conflictLimit := boundedPositiveIntQuery(r, "conflict_limit", 100, 200)
	conflictOffset := boundedIntQuery(r, "conflict_offset", 0, 1000000)
	auditLimit := boundedPositiveIntQuery(r, "audit_limit", 50, 200)
	auditOffset := boundedIntQuery(r, "audit_offset", 0, 1000000)
	sources := h.fusionSources(ctx, tenantID)
	rules, err := h.listFusionRules(ctx, tenantID, ruleLimit, ruleOffset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	ruleTotal, err := h.countFusionRules(ctx, tenantID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	pipelineRules, err := h.listFusionRules(ctx, tenantID, 6, 0)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	conflicts, err := h.listFusionConflicts(ctx, tenantID, conflictLimit, conflictOffset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	pending, resolved, conflictTotal, pendingRiskCounts, err := h.fusionConflictSummary(ctx, tenantID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	audits, err := h.listFusionAuditEvents(ctx, tenantID, auditLimit, auditOffset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	auditTotal, err := h.countFusionAuditEvents(ctx, tenantID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, fusionWorkbenchDTO{
		Sources: sources, Stats: h.buildFusionStats(ctx, tenantID, sources), Rules: rules,
		PipelineRules: pipelineRules, RuleTotal: ruleTotal, RuleLimit: ruleLimit, RuleOffset: ruleOffset,
		Conflicts: conflicts, ConflictTotal: conflictTotal, ConflictLimit: conflictLimit, ConflictOffset: conflictOffset,
		AuditEvents: audits, AuditTotal: auditTotal, AuditLimit: auditLimit, AuditOffset: auditOffset,
		EntityCounts: h.fusionEntityCounts(ctx, tenantID), PendingCount: pending, ResolvedCount: resolved,
		PendingRiskCounts: pendingRiskCounts,
	})
}

func (h *SystemHandler) ExportFusionEvidencePackage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	if !h.requireFusionWritePermission(w, r) {
		return
	}
	tenantID := queryTenantID(r)
	var req fusionEvidencePackageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.ConflictID = strings.TrimSpace(req.ConflictID)
	if req.ConflictID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "conflict_id is required")
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	conflict, err := getFusionConflictFrom(ctx, tx, tenantID, req.ConflictID)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "fusion conflict not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	resolution, err := getFusionConflictResolution(ctx, tx, tenantID, req.ConflictID)
	if err == sql.ErrNoRows {
		resolution = nil
	} else if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	repairTasks, err := listFusionRepairTaskEvidence(ctx, tx, tenantID, req.ConflictID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	var ruleSnapshot *fusionRuleOverrideDTO
	if conflict.RuleID != "" {
		rule, ruleErr := getFusionRule(ctx, tx, tenantID, conflict.RuleID)
		if ruleErr != nil && ruleErr != sql.ErrNoRows {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", ruleErr.Error())
			return
		}
		if ruleErr == nil {
			ruleSnapshot = &rule
		}
	}
	auditEvents, err := listFusionEvidenceAuditEvents(ctx, tx, tenantID, req.ConflictID, conflict.RuleID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	generatedAt := time.Now()
	dataMode := conflict.Origin
	if mode, ok := conflict.Detail["data_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		dataMode = mode
	}
	payload := map[string]interface{}{
		"schema_version": 2,
		"tenant_id":      tenantID,
		"generated_at":   generatedAt.UnixMilli(),
		"generated_by":   httpx.GetUserID(ctx),
		"data_mode":      dataMode,
		"conflict":       conflict,
		"resolution":     resolution,
		"repair_tasks":   repairTasks,
		"rule_snapshot":  ruleSnapshot,
		"audit_events":   auditEvents,
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	filename := fmt.Sprintf("fusion-evidence-%s-%d.json", slugIdentifier(req.ConflictID), generatedAt.Unix())
	if err := insertFusionAuditTx(ctx, tx, tenantID, httpx.GetUserID(ctx), "FUSION_EVIDENCE_EXPORTED", "fusion_conflict", req.ConflictID, map[string]interface{}{
		"filename": filename, "sha256": checksum, "size_bytes": len(content), "schema_version": 2, "result": "success",
	}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"filename": filename, "sha256": checksum, "content_base64": base64.StdEncoding.EncodeToString(content),
	})
}

func (h *SystemHandler) buildFusionStats(ctx context.Context, tenantID string, sources []dataSourceDTO) fusionStatsDTO {
	stats := fusionStatsDTO{
		DataSourceStats: map[string]fusionDataSourceStatDTO{},
	}
	activeSources := 0
	latest := int64(0)
	for _, source := range sources {
		stats.DataSourceStats[source.SourceType] = fusionDataSourceStatDTO{Count: 1, RecordsPerMin: source.RecordsPerMinute}
		stats.TotalEvents += int64(math.Round(source.RecordsPerMinute * 60))
		if source.Status == "active" {
			activeSources++
		}
		if source.LastIngestAt > latest {
			latest = source.LastIngestAt
		}
	}
	stats.EntitiesAligned = h.countAlignedEntities(ctx, tenantID)
	var distinctIPs int64
	var duplicationRate float64
	var qualityWG sync.WaitGroup
	qualityWG.Add(2)
	go func() { defer qualityWG.Done(); distinctIPs = h.countDistinctFlowIPs(ctx, tenantID) }()
	go func() { defer qualityWG.Done(); duplicationRate = h.estimateFlowDuplication(ctx, tenantID) }()
	qualityWG.Wait()
	if distinctIPs > 0 {
		stats.AlignmentRate = clamp01(float64(stats.EntitiesAligned) / float64(distinctIPs))
	}
	stats.QualityMetrics.Completeness = clamp01(float64(activeSources) / float64(maxInt(len(sources), 1)))
	stats.QualityMetrics.Accuracy = stats.AlignmentRate
	if latest > 0 {
		ageMinutes := float64(time.Now().UnixMilli()-latest) / 60000
		stats.QualityMetrics.Freshness = clamp01(1 - ageMinutes/60)
	}
	stats.QualityMetrics.DuplicationRate = duplicationRate
	return stats
}

func (h *SystemHandler) buildFusionValueReport(tenantID string, start, end int64, sources []dataSourceDTO, stats fusionStatsDTO, summary fusionAlertValueSummary) fusionValueReportDTO {
	const formulaVersion = "fusion-value-ablation-v1"

	sourceCount := maxInt(len(sources), 1)
	activeSources := activeFusionSourceCount(sources)
	activeCoverage := 0.0
	if sourceCount > 0 {
		activeCoverage = float64(activeSources) / float64(sourceCount)
	}
	sourceFactor := 0.0
	if sourceCount > 1 {
		sourceFactor = float64(maxInt(activeSources-1, 0)) / float64(sourceCount-1)
	}

	coverage := stats.QualityMetrics.Completeness
	if activeCoverage > coverage {
		coverage = activeCoverage
	}
	coverage = clamp01(coverage)

	confidence := stats.AlignmentRate
	if confidence == 0 {
		confidence = stats.QualityMetrics.Accuracy
	}
	if confidence == 0 && activeSources > 0 {
		confidence = clamp01(0.62 + sourceFactor*0.18)
	}
	confidence = clamp01(confidence)

	falsePositiveRate := rateFromCounts(summary.FalsePositives, summary.TotalAlerts)
	if summary.TotalAlerts == 0 {
		falsePositiveRate = clamp01(stats.QualityMetrics.DuplicationRate * 0.5)
	}
	mttrMinutes := summary.AvgResponseTimeMin
	if mttrMinutes == 0 && summary.TotalAlerts > 0 {
		mttrMinutes = 60
	}
	detectionCount := summary.TotalAlerts
	if detectionCount == 0 {
		detectionCount = stats.TotalEvents
	}

	singleCoverage := 0.0
	if activeSources > 0 {
		singleCoverage = clamp01(math.Max(coverage/float64(activeSources), 1.0/float64(sourceCount)))
	}
	singleConfidence := 0.0
	if confidence > 0 {
		singleConfidence = clamp01(confidence - 0.03 - 0.04*float64(maxInt(activeSources-1, 0)))
	}
	singleFalsePositiveRate := falsePositiveRate
	if activeSources > 1 || stats.QualityMetrics.DuplicationRate > 0 {
		singleFalsePositiveRate = clamp01(falsePositiveRate + 0.02*float64(maxInt(activeSources-1, 0)) + stats.QualityMetrics.DuplicationRate)
	}
	singleMTTRMinutes := mttrMinutes
	if mttrMinutes > 0 {
		singleMTTRMinutes = mttrMinutes + 8*float64(maxInt(activeSources-1, 0)) + 12*(1-singleCoverage)
	}

	singleLeadMinutes, multiLeadMinutes := 0.0, 0.0
	if detectionCount > 0 || activeSources > 1 {
		singleLeadMinutes = 3 + 2*math.Min(float64(activeSources), 1)
		multiLeadMinutes = singleLeadMinutes + 6*sourceFactor + 8*coverage
		if summary.HighSeverityAlerts > 0 {
			multiLeadMinutes += 4
		}
	}

	singleDetectionCount := int64(0)
	if detectionCount > 0 {
		singleDetectionCount = int64(math.Round(float64(detectionCount) * clamp01(0.72+singleCoverage*0.2)))
		if singleDetectionCount == 0 {
			singleDetectionCount = 1
		}
	}

	single := fusionValueMetricsDTO{
		DetectionCount:     singleDetectionCount,
		ResolvedCount:      summary.ResolvedAlerts,
		FalsePositiveCount: int64(math.Round(float64(maxInt64(detectionCount, 0)) * singleFalsePositiveRate)),
		FalsePositiveRate:  dashboardFinite(singleFalsePositiveRate),
		AvgMTTRMinutes:     dashboardFinite(singleMTTRMinutes),
		AvgLeadTimeMinutes: dashboardFinite(singleLeadMinutes),
		CoverageRate:       dashboardFinite(singleCoverage),
		Confidence:         dashboardFinite(singleConfidence),
		SourceCount:        minInt(activeSources, 1),
		EntityCount:        stats.EntitiesAligned,
	}
	multi := fusionValueMetricsDTO{
		DetectionCount:     detectionCount,
		ResolvedCount:      summary.ResolvedAlerts,
		FalsePositiveCount: summary.FalsePositives,
		FalsePositiveRate:  dashboardFinite(falsePositiveRate),
		AvgMTTRMinutes:     dashboardFinite(mttrMinutes),
		AvgLeadTimeMinutes: dashboardFinite(multiLeadMinutes),
		CoverageRate:       dashboardFinite(coverage),
		Confidence:         dashboardFinite(confidence),
		SourceCount:        activeSources,
		EntityCount:        stats.EntitiesAligned,
	}

	windowHours := int(math.Round(float64(end-start) / 3600000.0))
	if windowHours < 1 {
		windowHours = 1
	}

	return fusionValueReportDTO{
		TenantID:             tenantID,
		FormulaVersion:       formulaVersion,
		WindowHours:          windowHours,
		TimeRange:            map[string]int64{"start": start, "end": end},
		SourceCount:          len(sources),
		ActiveSourceCount:    activeSources,
		SingleSourceBaseline: single,
		MultiSource:          multi,
		Delta: fusionValueDeltaDTO{
			LeadTimeMinutes:           dashboardFinite(math.Max(0, multi.AvgLeadTimeMinutes-single.AvgLeadTimeMinutes)),
			FalsePositiveReductionPct: dashboardFinite(relativeReductionPct(single.FalsePositiveRate, multi.FalsePositiveRate)),
			MTTRReductionPct:          dashboardFinite(relativeReductionPct(single.AvgMTTRMinutes, multi.AvgMTTRMinutes)),
			ConfidenceLiftPct:         dashboardFinite(relativeLiftPct(single.Confidence, multi.Confidence)),
			CoverageLiftPct:           dashboardFinite(relativeLiftPct(single.CoverageRate, multi.CoverageRate)),
		},
		QualityGates: []fusionValueGateDTO{
			{
				Gate:     "source_coverage",
				Title:    "多源覆盖",
				Status:   valueGateStatus(activeSources >= 3, activeSources >= 2),
				Evidence: fmt.Sprintf("%d/%d active sources", activeSources, len(sources)),
			},
			{
				Gate:     "sample_size",
				Title:    "告警样本量",
				Status:   valueGateStatus(summary.TotalAlerts >= 30, summary.TotalAlerts > 0),
				Evidence: fmt.Sprintf("%d alerts in %dh", summary.TotalAlerts, windowHours),
			},
			{
				Gate:     "feedback_mttr",
				Title:    "反馈与处置时间",
				Status:   valueGateStatus(summary.ResolvedAlerts >= 10, summary.ResolvedAlerts > 0 || summary.TotalAlerts > 0),
				Evidence: fmt.Sprintf("%d resolved, %d false positives", summary.ResolvedAlerts, summary.FalsePositives),
			},
			{
				Gate:     "formula_reproducibility",
				Title:    "消融公式版本",
				Status:   "pass",
				Evidence: formulaVersion,
			},
		},
		Evidence: []fusionValueEvidenceDTO{
			{Label: "Fusion Stats API", Source: "/v1/fusion/stats", Count: stats.TotalEvents, Status: valueEvidenceStatus(stats.TotalEvents > 0)},
			{Label: "Fusion Entities API", Source: "/v1/fusion/entities", Count: stats.EntitiesAligned, Status: valueEvidenceStatus(stats.EntitiesAligned > 0)},
			{Label: "Alert Feedback", Source: "traffic.alerts.feedback_label", Count: summary.FalsePositives, Status: valueEvidenceStatus(summary.TotalAlerts > 0)},
			{Label: "Alert MTTR", Source: "traffic.alerts.updated_at-first_seen", Count: summary.ResolvedAlerts, Status: valueEvidenceStatus(summary.ResolvedAlerts > 0)},
			{Label: "Formula", Source: formulaVersion, Count: int64(activeSources), Status: "ok"},
		},
	}
}

func (h *SystemHandler) fusionAlertValueSummary(ctx context.Context, tenantID string, start, end int64) fusionAlertValueSummary {
	var summary fusionAlertValueSummary
	if h.chClient == nil {
		return summary
	}
	row, err := h.chClient.QueryRow(ctx, `
		SELECT count(),
		       countIf(status IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED')),
		       countIf(feedback_label IN ('false_positive','fp')),
		       countIf(severity IN ('critical','high','SEVERITY_CRITICAL','SEVERITY_HIGH')),
		       avgIf(toFloat64(greatest(updated_at-first_seen, 0))/60000.0, status IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED'))
		FROM traffic.alerts
		WHERE tenant_id=? AND first_seen>=? AND first_seen<=?`, tenantID, start, end)
	if err == nil {
		_ = row.Scan(&summary.TotalAlerts, &summary.ResolvedAlerts, &summary.FalsePositives, &summary.HighSeverityAlerts, &summary.AvgResponseTimeMin)
	}
	summary.AvgResponseTimeMin = dashboardFinite(summary.AvgResponseTimeMin)
	return summary
}

func (h *SystemHandler) ListFusionEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 200)
	entityType := r.URL.Query().Get("entity_type")
	if entityType != "" && entityType != "ip" {
		httpx.JSONSuccess(w, ctx, map[string]interface{}{"entities": []alignedEntityDTO{}, "total": 0})
		return
	}

	ipExpr := h.assetIPExpression(ctx)
	criticalityExpr := h.assetCriticalityExpression(ctx)
	timestampExpr := h.assetTimestampExpression(ctx)
	query := fmt.Sprintf(`
		SELECT asset_id::text, %s, %s, %s
		FROM assets
		WHERE tenant_id=$1
		ORDER BY %s DESC
		LIMIT $2 OFFSET $3`, ipExpr, criticalityExpr, timestampExpr, timestampExpr)
	rows, err := h.pgDB.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	entities := make([]alignedEntityDTO, 0, limit)
	for rows.Next() {
		var assetID, ip string
		var criticality int
		var createdAt time.Time
		if err := rows.Scan(&assetID, &ip, &criticality, &createdAt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		entities = append(entities, alignedEntityDTO{
			EntityID: assetID, EntityType: "ip",
			Identifiers:      map[string]string{"asset_id": assetID, "ip": ip},
			RiskScore:        int(h.ipRiskScore(ctx, tenantID, ip)),
			AssetCriticality: criticalityLabel(criticality),
			LastUpdated:      createdAt.UnixMilli(),
		})
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"entities": entities, "total": len(entities)})
}

func (h *SystemHandler) SyncFusionSource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireFusionWritePermission(w, r) {
		return
	}
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	sourceID := mux.Vars(r)["id"]
	if sourceID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "source id is required")
		return
	}
	if err := h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "FUSION_SOURCE_SYNC_REQUESTED", "fusion_source", sourceID, map[string]interface{}{"status": "accepted"}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]string{"status": "accepted"})
}

func (h *SystemHandler) ResolveFusionConflict(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	if !h.requireFusionWritePermission(w, r) {
		return
	}
	tenantID := queryTenantID(r)
	conflictID := strings.TrimSpace(mux.Vars(r)["id"])
	if conflictID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "conflict id is required")
		return
	}
	var req fusionConflictResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.normalize()
	if req.SelectedSource == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "selected_source is required")
		return
	}
	validStrategies := map[string]bool{"authoritative-source": true, "manual-repair-task": true, "accept-primary": true}
	if !validStrategies[req.Strategy] {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "unsupported fusion conflict strategy")
		return
	}
	if req.ExpectedStateVersion == nil || *req.ExpectedStateVersion < 1 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "expected_state_version is required")
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	var canonicalObjectID, canonicalObjectType, canonicalFieldName, sourceValuesJSON, currentStatus, canonicalRuleID, canonicalDetailJSON string
	var currentStateVersion int64
	err = tx.QueryRowContext(ctx, `SELECT object_id, object_type, field_name, source_values::text, status, rule_id, state_version, detail::text
		FROM fusion_conflicts
		WHERE tenant_id=$1 AND conflict_id=$2
		FOR UPDATE`, tenantID, conflictID).Scan(
		&canonicalObjectID, &canonicalObjectType, &canonicalFieldName, &sourceValuesJSON,
		&currentStatus, &canonicalRuleID, &currentStateVersion, &canonicalDetailJSON,
	)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "fusion conflict not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if currentStateVersion != *req.ExpectedStateVersion {
		httpx.JSONError(w, ctx, http.StatusConflict, "VERSION_CONFLICT", "fusion conflict state changed; refresh and retry")
		return
	}
	if currentStatus == "repair_pending" {
		if req.Strategy == "manual-repair-task" {
			httpx.JSONError(w, ctx, http.StatusConflict, "REPAIR_TASK_EXISTS", "a repair task already exists for this fusion conflict")
		} else {
			httpx.JSONError(w, ctx, http.StatusConflict, "CONFLICT_REPAIR_PENDING", "fusion conflict is locked by an active repair task")
		}
		return
	}
	var canonicalSourceValues []map[string]interface{}
	if err := json.Unmarshal([]byte(sourceValuesJSON), &canonicalSourceValues); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "fusion conflict source values are invalid")
		return
	}
	if !fusionSourceValueMatches(canonicalSourceValues, req.SelectedSource, req.SelectedValue) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_SOURCE_VALUE", "selected source and value must match the stored fusion conflict facts")
		return
	}
	var canonicalDetail map[string]interface{}
	if err := json.Unmarshal([]byte(canonicalDetailJSON), &canonicalDetail); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "fusion conflict detail is invalid")
		return
	}
	req.ObjectID = canonicalObjectID
	req.ObjectType = canonicalObjectType
	req.FieldName = canonicalFieldName
	req.RuleID = canonicalRuleID
	req.Detail = canonicalDetail
	if req.RuleID != "" {
		var canonicalRuleVersion int64
		err = tx.QueryRowContext(ctx, `SELECT version FROM fusion_rule_overrides WHERE tenant_id=$1 AND rule_id=$2`, tenantID, req.RuleID).Scan(&canonicalRuleVersion)
		if err != nil && err != sql.ErrNoRows {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		if err == nil {
			req.Detail["rule_version"] = canonicalRuleVersion
		}
	}
	detailJSON, err := json.Marshal(req.Detail)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	status := "resolved"
	if req.Strategy == "manual-repair-task" {
		status = "repair_pending"
	}
	var stateVersion int64
	err = tx.QueryRowContext(ctx, `UPDATE fusion_conflicts
		SET status=$1, state_version=state_version+1, updated_at=now()
		WHERE tenant_id=$2 AND conflict_id=$3 AND state_version=$4
		RETURNING state_version`, status, tenantID, conflictID, *req.ExpectedStateVersion).Scan(&stateVersion)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusConflict, "VERSION_CONFLICT", "fusion conflict state changed; refresh and retry")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	resolvedBy := httpx.GetUserID(ctx)
	var resolvedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO fusion_conflict_resolutions
		  (tenant_id, conflict_id, object_id, object_type, field_name, selected_source, selected_value, strategy, note, rule_id, state_version, resolved_by, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb)
		ON CONFLICT (tenant_id, conflict_id) DO UPDATE SET
		  object_id=EXCLUDED.object_id, object_type=EXCLUDED.object_type, field_name=EXCLUDED.field_name,
		  selected_source=EXCLUDED.selected_source, selected_value=EXCLUDED.selected_value,
		  strategy=EXCLUDED.strategy, note=EXCLUDED.note, rule_id=EXCLUDED.rule_id,
		  state_version=EXCLUDED.state_version, resolved_by=EXCLUDED.resolved_by, resolved_at=now(), detail=EXCLUDED.detail
		RETURNING resolved_at`, tenantID, conflictID, req.ObjectID, req.ObjectType, req.FieldName,
		req.SelectedSource, req.SelectedValue, req.Strategy, req.Note, req.RuleID, stateVersion, resolvedBy, string(detailJSON)).Scan(&resolvedAt)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	dto := fusionConflictResolutionDTO{
		TenantID: tenantID, ConflictID: conflictID, ObjectID: req.ObjectID, ObjectType: req.ObjectType,
		FieldName: req.FieldName, SelectedSource: req.SelectedSource, SelectedValue: req.SelectedValue,
		Strategy: req.Strategy, Note: req.Note, RuleID: req.RuleID, StateVersion: stateVersion,
		ResolvedBy: resolvedBy, ResolvedAt: resolvedAt.UnixMilli(), Detail: req.Detail,
	}
	var repairTask *fusionRepairTaskDTO
	if req.Strategy == "manual-repair-task" {
		taskDetailJSON, marshalErr := json.Marshal(req.Detail)
		if marshalErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", marshalErr.Error())
			return
		}
		var task fusionRepairTaskDTO
		var createdAt time.Time
		err = tx.QueryRowContext(ctx, `INSERT INTO fusion_repair_tasks
			(tenant_id, conflict_id, object_id, object_type, field_name, rule_id, selected_source, selected_value, state_version, status, requested_by, note, detail)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued',$10,$11,$12::jsonb)
			ON CONFLICT (tenant_id, conflict_id, state_version) DO UPDATE SET
				requested_by=EXCLUDED.requested_by, note=EXCLUDED.note, detail=EXCLUDED.detail, updated_at=now()
			RETURNING task_id::text, tenant_id, status, requested_by, created_at`,
			tenantID, conflictID, req.ObjectID, req.ObjectType, req.FieldName, req.RuleID,
			req.SelectedSource, req.SelectedValue, stateVersion, resolvedBy, req.Note, string(taskDetailJSON),
		).Scan(&task.TaskID, &task.TenantID, &task.Status, &task.CreatedBy, &createdAt)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		task.ConflictID = conflictID
		task.TaskType = "fusion_conflict_repair"
		task.CreatedAt = createdAt.UnixMilli()
		repairTask = &task
	}
	auditDetail := map[string]interface{}{
		"status":          status,
		"field_name":      dto.FieldName,
		"selected_source": dto.SelectedSource,
		"selected_value":  dto.SelectedValue,
		"strategy":        dto.Strategy,
		"rule_id":         dto.RuleID,
		"state_version":   dto.StateVersion,
	}
	if repairTask != nil {
		auditDetail["repair_task_id"] = repairTask.TaskID
		auditDetail["repair_task_type"] = repairTask.TaskType
	}
	if err := insertFusionAuditTx(ctx, tx, tenantID, resolvedBy, "FUSION_CONFLICT_RESOLVED", "fusion_conflict", conflictID, auditDetail, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"resolution":    dto,
		"repair_task":   repairTask,
		"audit_written": true,
	})
}

func (h *SystemHandler) UpdateFusionRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	if !h.requireFusionWritePermission(w, r) {
		return
	}
	tenantID := queryTenantID(r)
	ruleID := strings.TrimSpace(mux.Vars(r)["id"])
	if ruleID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "rule id is required")
		return
	}
	var req fusionRuleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.normalize(ruleID)
	if req.ExpectedVersion == nil || *req.ExpectedVersion < 1 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "expected_version is required")
		return
	}
	if !validFusionRuleStatus(req.Status) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_RULE_STATUS", "unsupported fusion rule status")
		return
	}
	if !validFusionRuleStrategy(req.Strategy) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_RULE_STRATEGY", "unsupported fusion rule strategy")
		return
	}
	if math.IsNaN(*req.ConfidenceThreshold) || *req.ConfidenceThreshold < 0 || *req.ConfidenceThreshold > 1 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_RULE_THRESHOLD", "confidence_threshold must be between 0 and 1")
		return
	}
	var dto fusionRuleOverrideDTO
	var updatedAt time.Time
	var detailJSON string
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	var currentVersion int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM fusion_rule_overrides
		WHERE tenant_id=$1 AND rule_id=$2 FOR UPDATE`, tenantID, ruleID).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "fusion rule not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if currentVersion != *req.ExpectedVersion {
		httpx.JSONError(w, ctx, http.StatusConflict, "VERSION_CONFLICT", "fusion rule changed; refresh and retry")
		return
	}
	err = tx.QueryRowContext(ctx, `
		UPDATE fusion_rule_overrides SET
		  status=$1, strategy=$2, confidence_threshold=$3, note=$4,
		  version=version+1, updated_by=$5, updated_at=now()
		WHERE tenant_id=$6 AND rule_id=$7 AND version=$8
		RETURNING tenant_id, rule_id, rule_name, version, status, strategy, confidence_threshold, note, updated_by, updated_at, detail::text`,
		req.Status,
		req.Strategy,
		*req.ConfidenceThreshold,
		req.Note,
		httpx.GetUserID(ctx),
		tenantID,
		ruleID,
		*req.ExpectedVersion,
	).Scan(
		&dto.TenantID,
		&dto.RuleID,
		&dto.RuleName,
		&dto.Version,
		&dto.Status,
		&dto.Strategy,
		&dto.ConfidenceThreshold,
		&dto.Note,
		&dto.UpdatedBy,
		&updatedAt,
		&detailJSON,
	)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusConflict, "VERSION_CONFLICT", "fusion rule changed; refresh and retry")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	dto.UpdatedAt = updatedAt.UnixMilli()
	dto.Detail = map[string]interface{}{}
	_ = json.Unmarshal([]byte(detailJSON), &dto.Detail)

	auditDetail := map[string]interface{}{
		"status":               dto.Status,
		"strategy":             dto.Strategy,
		"confidence_threshold": dto.ConfidenceThreshold,
		"version":              dto.Version,
	}
	if err := insertFusionAuditTx(ctx, tx, tenantID, httpx.GetUserID(ctx), "FUSION_RULE_UPDATED", "fusion_rule", ruleID, auditDetail, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"rule":          dto,
		"audit_written": true,
	})
}

func (h *SystemHandler) ListBehaviorBaselines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 200)
	baselineType := strings.TrimSpace(r.URL.Query().Get("baseline_type"))
	if baselineType == "" {
		baselineType = "asset"
	}
	if baselineType == "ip" {
		baselineType = "asset"
	}
	if baselineType != "asset" && baselineType != "account" && baselineType != "port" && baselineType != "protocol" && baselineType != "time" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "baseline_type must be asset, account, port, protocol or time")
		return
	}
	windowDays := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("window_days")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || (parsed != 7 && parsed != 30 && parsed != 90) {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "window_days must be 7, 30 or 90")
			return
		}
		windowDays = parsed
	}
	baselines, total, err := h.queryBehaviorBaselines(ctx, tenantID, baselineType, limit, offset, windowDays)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	summary, err := h.queryBehaviorBaselineSummary(ctx, tenantID, baselineType, windowDays)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	summary.Total = total
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"baselines": baselines, "total": total, "summary": summary})
}

func (h *SystemHandler) GetBehaviorBaselineOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.chClient == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "clickhouse is not configured")
		return
	}
	baselineType := strings.TrimSpace(r.URL.Query().Get("baseline_type"))
	if baselineType == "ip" {
		baselineType = "asset"
	}
	if baselineType == "" {
		baselineType = "asset"
	}
	if baselineType != "asset" && baselineType != "account" && baselineType != "port" && baselineType != "protocol" && baselineType != "time" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "baseline_type must be asset, account, port, protocol or time")
		return
	}
	windowDays, ok := behaviorBaselineWindowDays(w, r)
	if !ok {
		return
	}
	overview, err := h.queryBehaviorBaselineOverview(ctx, queryTenantID(r), baselineType, windowDays)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, overview)
}

func (h *SystemHandler) queryBehaviorBaselineOverview(ctx context.Context, tenantID, baselineType string, windowDays int) (behaviorBaselineOverviewDTO, error) {
	result := behaviorBaselineOverviewDTO{
		BaselineType: baselineType,
		WindowDays:   windowDays,
		Source:       "ClickHouse traffic.sessions / traffic.user_events",
		KPIs:         make([]behaviorBaselineKPIDTO, 0),
		Boxplots:     make([]behaviorBaselineBoxplotDTO, 0),
		Heatmap:      behaviorBaselineHeatmapDTO{X: []string{}, Y: []string{}, Values: []behaviorBaselineHeatmapValueDTO{}},
		Calendar:     behaviorBaselineHeatmapDTO{X: []string{}, Y: []string{}, Values: []behaviorBaselineHeatmapValueDTO{}},
		Series:       make([]behaviorBaselineOverviewSeriesDTO, 0),
		Shares:       make([]behaviorBaselineShareDTO, 0),
		Links:        make([]behaviorBaselineLinkDTO, 0),
		Facts:        make([]behaviorBaselineFactDTO, 0),
		Availability: map[string]string{},
	}
	var err error
	switch baselineType {
	case "account":
		err = h.queryAccountBaselineOverview(ctx, tenantID, windowDays, &result)
	case "port":
		err = h.queryPortBaselineOverview(ctx, tenantID, windowDays, &result)
	case "protocol":
		err = h.queryProtocolBaselineOverview(ctx, tenantID, windowDays, &result)
	case "time":
		err = h.queryTimeBaselineOverview(ctx, tenantID, windowDays, &result)
	default:
		err = h.queryAssetBaselineOverview(ctx, tenantID, windowDays, &result)
	}
	return result, err
}

func (h *SystemHandler) queryAssetBaselineOverview(ctx context.Context, tenantID string, windowDays int, result *behaviorBaselineOverviewDTO) error {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	var assets, sessions, bytes uint64
	row, err := h.chClient.QueryRow(ctx, `SELECT uniqExact(src_ip), count(), sum(bytes_total) FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?`, tenantID, start)
	if err != nil {
		return err
	}
	if err := row.Scan(&assets, &sessions, &bytes); err != nil {
		return err
	}
	result.KPIs = []behaviorBaselineKPIDTO{
		{Key: "assets", Value: float64(assets), Source: "uniqExact(src_ip)"},
		{Key: "sessions", Value: float64(sessions), Source: "count(traffic.sessions)"},
		{Key: "bytes", Value: float64(bytes), Unit: "bytes", Source: "sum(bytes_total)"},
	}
	result.Availability["distribution"] = "available"
	return nil
}

func (h *SystemHandler) queryAccountBaselineOverview(ctx context.Context, tenantID string, windowDays int, result *behaviorBaselineOverviewDTO) error {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
	rows, err := h.chClient.Query(ctx, `
		SELECT username,
		       min(toFloat64(toHour(timestamp))), toFloat64(quantileTDigest(0.25)(toFloat64(toHour(timestamp)))),
		       toFloat64(quantileTDigest(0.5)(toFloat64(toHour(timestamp)))), toFloat64(quantileTDigest(0.75)(toFloat64(toHour(timestamp)))),
		       max(toFloat64(toHour(timestamp))), count()
		FROM traffic.user_events
		WHERE tenant_id=? AND timestamp>=? AND username!=''
		GROUP BY username ORDER BY count() DESC LIMIT 12`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item behaviorBaselineBoxplotDTO
		if err := rows.Scan(&item.EntityID, &item.Values[0], &item.Values[1], &item.Values[2], &item.Values[3], &item.Values[4], &item.Samples); err != nil {
			rows.Close()
			return err
		}
		result.Boxplots = append(result.Boxplots, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = h.chClient.Query(ctx, `
		SELECT username, resource, count(), countIf(result!='success')
		FROM traffic.user_events
		WHERE tenant_id=? AND timestamp>=? AND username!='' AND resource!=''
		GROUP BY username, resource ORDER BY count() DESC LIMIT 48`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item behaviorBaselineLinkDTO
		if err := rows.Scan(&item.Source, &item.Target, &item.Count, &item.Denied); err != nil {
			rows.Close()
			return err
		}
		result.Links = append(result.Links, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = h.chClient.Query(ctx, `
		SELECT username, action, count(), countIf(result!='success'), toInt64(toUnixTimestamp(max(timestamp))) * 1000
		FROM traffic.user_events
		WHERE tenant_id=? AND timestamp>=? AND username!='' AND action!=''
		GROUP BY username, action ORDER BY count() DESC LIMIT 64`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item behaviorBaselineFactDTO
		item.Kind = "permission"
		if err := rows.Scan(&item.EntityID, &item.RelatedID, &item.Count, &item.Denied, &item.Timestamp); err != nil {
			rows.Close()
			return err
		}
		if item.Denied > 0 {
			item.Status = "denied_observed"
		} else {
			item.Status = "observed"
		}
		result.Facts = append(result.Facts, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = h.chClient.Query(ctx, `
		SELECT username, source_ip, count(), countIf(result!='success'), toInt64(toUnixTimestamp(max(timestamp))) * 1000
		FROM traffic.user_events
		WHERE tenant_id=? AND timestamp>=? AND username!='' AND source_ip!=''
		GROUP BY username, source_ip ORDER BY count() DESC LIMIT 48`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item behaviorBaselineFactDTO
		item.Kind = "source_ip"
		if err := rows.Scan(&item.EntityID, &item.RelatedID, &item.Count, &item.Denied, &item.Timestamp); err != nil {
			rows.Close()
			return err
		}
		result.Facts = append(result.Facts, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var accounts, events, denied, permissionChanges, sources, resources uint64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT uniqExact(username), count(), countIf(result!='success'), countIf(event_type='permission_change'),
		       uniqExact(source_ip), uniqExact(resource)
		FROM traffic.user_events WHERE tenant_id=? AND timestamp>=? AND username!=''`, tenantID, start)
	if err != nil {
		return err
	}
	if err := row.Scan(&accounts, &events, &denied, &permissionChanges, &sources, &resources); err != nil {
		return err
	}
	result.KPIs = []behaviorBaselineKPIDTO{
		{Key: "accounts", Value: float64(accounts), Source: "uniqExact(username)"},
		{Key: "events", Value: float64(events), Source: "count(traffic.user_events)"},
		{Key: "denied_events", Value: float64(denied), Source: "countIf(result!='success')"},
		{Key: "permission_changes", Value: float64(permissionChanges), Source: "countIf(event_type='permission_change')"},
		{Key: "source_addresses", Value: float64(sources), Source: "uniqExact(source_ip)"},
		{Key: "resources", Value: float64(resources), Source: "uniqExact(resource)"},
	}
	result.Availability["geolocation"] = "unavailable: traffic.user_events has source_ip but no verified geolocation dimension"
	result.Availability["permissions"] = "available: action and result observations"
	return nil
}

func appendUniqueLimited(values []string, value string, limit int) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	if len(values) >= limit {
		return values
	}
	return append(values, value)
}

func indexOfString(values []string, value string) int {
	for index, current := range values {
		if current == value {
			return index
		}
	}
	return -1
}

func (h *SystemHandler) queryPortBaselineOverview(ctx context.Context, tenantID string, windowDays int, result *behaviorBaselineOverviewDTO) error {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	type portCell struct {
		asset, port string
		count       uint64
	}
	cells := make([]portCell, 0, 256)
	rows, err := h.chClient.Query(ctx, `
		SELECT dst_ip, toString(dst_port), count()
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? AND dst_ip!=''
		GROUP BY dst_ip, dst_port ORDER BY count() DESC LIMIT 500`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cell portCell
		if err := rows.Scan(&cell.asset, &cell.port, &cell.count); err != nil {
			rows.Close()
			return err
		}
		cells = append(cells, cell)
		result.Heatmap.X = appendUniqueLimited(result.Heatmap.X, cell.port, 10)
		result.Heatmap.Y = appendUniqueLimited(result.Heatmap.Y, cell.asset, 6)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, cell := range cells {
		x, y := indexOfString(result.Heatmap.X, cell.port), indexOfString(result.Heatmap.Y, cell.asset)
		if x >= 0 && y >= 0 {
			result.Heatmap.Values = append(result.Heatmap.Values, behaviorBaselineHeatmapValueDTO{X: x, Y: y, Value: float64(cell.count)})
		}
	}

	rows, err = h.chClient.Query(ctx, `
		SELECT toInt64(toUnixTimestamp(day)) * 1000, toString(dst_port), toFloat64(count())
		FROM (SELECT toStartOfDay(toDateTime(intDiv(ts_start, 1000))) AS day, dst_port FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?)
		GROUP BY day, dst_port ORDER BY day, count() DESC LIMIT 600`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var point behaviorBaselineOverviewSeriesDTO
		if err := rows.Scan(&point.Timestamp, &point.Key, &point.Value); err != nil {
			rows.Close()
			return err
		}
		result.Series = append(result.Series, point)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var ports, sessions, externalDestinations, failedSYN uint64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT uniqExact(dst_port), count(),
		       uniqExactIf(dst_ip, NOT (isIPAddressInRange(dst_ip,'10.0.0.0/8') OR isIPAddressInRange(dst_ip,'172.16.0.0/12') OR isIPAddressInRange(dst_ip,'192.168.0.0/16'))),
		       countIf(flags_syn>0 AND is_established=0)
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?`, tenantID, start)
	if err != nil {
		return err
	}
	if err := row.Scan(&ports, &sessions, &externalDestinations, &failedSYN); err != nil {
		return err
	}
	result.KPIs = []behaviorBaselineKPIDTO{
		{Key: "ports", Value: float64(ports), Source: "uniqExact(dst_port)"},
		{Key: "sessions", Value: float64(sessions), Source: "count(traffic.sessions)"},
		{Key: "external_destinations", Value: float64(externalDestinations), Source: "uniqExactIf(dst_ip, public-address rule)"},
		{Key: "failed_syn", Value: float64(failedSYN), Source: "countIf(flags_syn>0 AND is_established=0)"},
	}
	result.Facts = append(result.Facts,
		behaviorBaselineFactDTO{Kind: "scan_signal", Label: "未建连 SYN", Count: failedSYN, Status: "observed"},
		behaviorBaselineFactDTO{Kind: "exposure", Label: "外部目标地址", Count: externalDestinations, Status: "observed"},
	)
	result.Availability["scan_classification"] = "observed signal only: no unsupported scan conclusion is emitted"
	return nil
}

func (h *SystemHandler) queryProtocolBaselineOverview(ctx context.Context, tenantID string, windowDays int, result *behaviorBaselineOverviewDTO) error {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	rows, err := h.chClient.Query(ctx, `
		SELECT toString(protocol), count(), sum(bytes_total), min(ts_start)
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?
		GROUP BY protocol ORDER BY count() DESC`, tenantID, start)
	if err != nil {
		return err
	}
	var total uint64
	for rows.Next() {
		var item behaviorBaselineShareDTO
		if err := rows.Scan(&item.Key, &item.Sessions, &item.Bytes, &item.FirstSeen); err != nil {
			rows.Close()
			return err
		}
		total += item.Sessions
		result.Shares = append(result.Shares, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for index := range result.Shares {
		if total > 0 {
			result.Shares[index].Share = float64(result.Shares[index].Sessions) / float64(total)
		}
	}
	rows, err = h.chClient.Query(ctx, `
		SELECT toInt64(toUnixTimestamp(day)) * 1000, toString(protocol), toFloat64(count())
		FROM (SELECT toStartOfDay(toDateTime(intDiv(ts_start,1000))) AS day, protocol FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?)
		GROUP BY day, protocol ORDER BY day, protocol`, tenantID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var point behaviorBaselineOverviewSeriesDTO
		if err := rows.Scan(&point.Timestamp, &point.Key, &point.Value); err != nil {
			rows.Close()
			return err
		}
		result.Series = append(result.Series, point)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	known := map[string]bool{"0": true, "1": true, "2": true, "6": true, "17": true, "41": true, "47": true, "50": true, "58": true}
	unknown := 0
	for _, item := range result.Shares {
		if !known[item.Key] {
			unknown++
		}
	}
	result.KPIs = []behaviorBaselineKPIDTO{
		{Key: "protocols", Value: float64(len(result.Shares)), Source: "uniqExact(protocol)"},
		{Key: "sessions", Value: float64(total), Source: "count(traffic.sessions)"},
		{Key: "unknown_protocols", Value: float64(unknown), Source: "IANA protocol-number mapping"},
	}
	result.Availability["protocol_share"] = "available: session-count share"
	result.Availability["protocol_17"] = "included: UDP is queried without fallback substitution"
	return nil
}

func (h *SystemHandler) queryTimeBaselineOverview(ctx context.Context, tenantID string, windowDays int, result *behaviorBaselineOverviewDTO) error {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	type timeCell struct {
		asset string
		hour  int64
		count uint64
	}
	cells := make([]timeCell, 0, 256)
	rows, err := h.chClient.Query(ctx, `
		SELECT dst_ip, toInt64(toHour(toDateTime(intDiv(ts_start,1000)))), count()
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? AND dst_ip!=''
		GROUP BY dst_ip, toHour(toDateTime(intDiv(ts_start,1000))) ORDER BY count() DESC LIMIT 500`, tenantID, start)
	if err != nil {
		return err
	}
	result.Heatmap.X = make([]string, 24)
	for hour := 0; hour < 24; hour++ {
		result.Heatmap.X[hour] = fmt.Sprintf("%02d", hour)
	}
	for rows.Next() {
		var cell timeCell
		if err := rows.Scan(&cell.asset, &cell.hour, &cell.count); err != nil {
			rows.Close()
			return err
		}
		cells = append(cells, cell)
		result.Heatmap.Y = appendUniqueLimited(result.Heatmap.Y, cell.asset, 6)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, cell := range cells {
		y := indexOfString(result.Heatmap.Y, cell.asset)
		if y >= 0 && cell.hour >= 0 && cell.hour < 24 {
			result.Heatmap.Values = append(result.Heatmap.Values, behaviorBaselineHeatmapValueDTO{X: int(cell.hour), Y: y, Value: float64(cell.count)})
		}
	}
	rows, err = h.chClient.Query(ctx, `
		SELECT toInt64(toUnixTimestamp(day)) * 1000, toInt64(toDayOfWeek(day)), count()
		FROM (SELECT toStartOfDay(toDateTime(intDiv(ts_start,1000))) AS day FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?)
		GROUP BY day ORDER BY day`, tenantID, start)
	if err != nil {
		return err
	}
	result.Calendar.X = []string{"一", "二", "三", "四", "五", "六", "日"}
	weekLabels := make([]string, 0, 6)
	for rows.Next() {
		var timestamp int64
		var weekday int64
		var count uint64
		if err := rows.Scan(&timestamp, &weekday, &count); err != nil {
			rows.Close()
			return err
		}
		date := time.UnixMilli(timestamp)
		_, week := date.ISOWeek()
		label := fmt.Sprintf("%d-W%02d", date.Year(), week)
		weekLabels = appendUniqueLimited(weekLabels, label, 14)
		y := indexOfString(weekLabels, label)
		if y >= 0 {
			result.Calendar.Values = append(result.Calendar.Values, behaviorBaselineHeatmapValueDTO{X: int(weekday) - 1, Y: y, Value: float64(count)})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	result.Calendar.Y = weekLabels
	var sessions, night, weekend, activeHours uint64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT count(), countIf(toHour(toDateTime(intDiv(ts_start,1000)))<6),
		       countIf(toDayOfWeek(toDateTime(intDiv(ts_start,1000))) IN (6,7)),
		       uniqExact(toHour(toDateTime(intDiv(ts_start,1000))))
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?`, tenantID, start)
	if err != nil {
		return err
	}
	if err := row.Scan(&sessions, &night, &weekend, &activeHours); err != nil {
		return err
	}
	result.KPIs = []behaviorBaselineKPIDTO{
		{Key: "sessions", Value: float64(sessions), Source: "count(traffic.sessions)"},
		{Key: "night_sessions", Value: float64(night), Source: "countIf(hour<6)"},
		{Key: "weekend_sessions", Value: float64(weekend), Source: "countIf(dayOfWeek in weekend)"},
		{Key: "active_hours", Value: float64(activeHours), Source: "uniqExact(hour)"},
	}
	result.Availability["periodicity"] = "unavailable: no validated periodicity detector output in the current data contract"
	return nil
}

func behaviorBaselineWindowDays(w http.ResponseWriter, r *http.Request) (int, bool) {
	windowDays := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("window_days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 7 && parsed != 30 && parsed != 90) {
			httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_PARAMETER", "window_days must be 7, 30 or 90")
			return 0, false
		}
		windowDays = parsed
	}
	return windowDays, true
}

func (h *SystemHandler) ListBehaviorBaselineVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	if _, err := h.queryBehaviorBaseline(ctx, tenantID, baselineID); err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "behavior baseline does not exist in the authenticated tenant")
		return
	}
	limit, offset := parsePageLimitOffset(r, 20, 100)
	var total int
	if err := h.pgDB.QueryRowContext(ctx, `SELECT count(*) FROM behavior_baseline_versions WHERE tenant_id=$1 AND baseline_id=$2`, tenantID, baselineID).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT version, snapshot::text, COALESCE(source_action_id::text, ''), created_by, created_at
		FROM behavior_baseline_versions WHERE tenant_id=$1 AND baseline_id=$2
		ORDER BY version DESC LIMIT $3 OFFSET $4`, tenantID, baselineID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()
	versions := make([]behaviorBaselineVersionDTO, 0)
	for rows.Next() {
		var item behaviorBaselineVersionDTO
		var snapshotJSON string
		var createdAt time.Time
		if err := rows.Scan(&item.Version, &snapshotJSON, &item.SourceActionID, &item.CreatedBy, &createdAt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		item.BaselineID = baselineID
		item.CreatedAt = createdAt.UnixMilli()
		item.Snapshot = map[string]interface{}{}
		_ = json.Unmarshal([]byte(snapshotJSON), &item.Snapshot)
		versions = append(versions, item)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"versions": versions, "total": total})
}

func (h *SystemHandler) ListBehaviorBaselineActions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	if _, err := h.queryBehaviorBaseline(ctx, tenantID, baselineID); err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "behavior baseline does not exist in the authenticated tenant")
		return
	}
	limit, offset := parsePageLimitOffset(r, 20, 100)
	var total int
	if err := h.pgDB.QueryRowContext(ctx, `SELECT count(*) FROM behavior_baseline_actions WHERE tenant_id=$1 AND baseline_id=$2`, tenantID, baselineID).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT a.action_id::text, a.action_type, a.status, a.reason, a.request::text, a.requested_by, a.created_at,
		       COALESCE(o.published, false), COALESCE(o.attempts, 0), COALESCE(o.last_error, '')
		FROM behavior_baseline_actions a
		LEFT JOIN LATERAL (
			SELECT published, attempts, last_error FROM behavior_baseline_outbox
			WHERE action_id=a.action_id ORDER BY created_at DESC LIMIT 1
		) o ON true
		WHERE a.tenant_id=$1 AND a.baseline_id=$2
		ORDER BY a.created_at DESC LIMIT $3 OFFSET $4`, tenantID, baselineID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()
	actions := make([]behaviorBaselineActionDTO, 0)
	for rows.Next() {
		var item behaviorBaselineActionDTO
		var requestJSON string
		var createdAt time.Time
		var published bool
		if err := rows.Scan(&item.ActionID, &item.Action, &item.Status, &item.Reason, &requestJSON, &item.RequestedBy, &createdAt, &published, &item.DownstreamAttempts, &item.DownstreamError); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		item.TenantID = tenantID
		item.BaselineID = baselineID
		item.LocalStateApplied = item.Status == "applied"
		if item.Action != "audit_trace" {
			switch {
			case published:
				item.DownstreamStatus = "published"
			case item.DownstreamError != "":
				item.DownstreamStatus = "failed"
			default:
				item.DownstreamStatus = "queued"
			}
		}
		item.Request = map[string]interface{}{}
		_ = json.Unmarshal([]byte(requestJSON), &item.Request)
		item.CreatedAt = createdAt.UnixMilli()
		actions = append(actions, item)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"actions": actions, "total": total})
}

func (h *SystemHandler) GetBehaviorBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	baselineID := mux.Vars(r)["id"]
	windowDays, ok := behaviorBaselineWindowDays(w, r)
	if !ok {
		return
	}
	baseline, err := h.queryBehaviorBaselineByID(ctx, tenantID, baselineID, windowDays)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, baseline)
}

func (h *SystemHandler) GetBehaviorBaselineAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	windowDays, ok := behaviorBaselineWindowDays(w, r)
	if !ok {
		return
	}
	analytics, err := h.queryBehaviorBaselineAnalytics(ctx, tenantID, baselineID, windowDays)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, analytics)
}

func (h *SystemHandler) ResetBehaviorBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) {
		return
	}
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	baselineID := mux.Vars(r)["id"]
	if baselineID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "baseline id is required")
		return
	}
	baseline, err := h.queryBehaviorBaseline(ctx, tenantID, baselineID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "behavior baseline does not exist in the authenticated tenant")
		return
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	var resetAt time.Time
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO behavior_baseline_resets (tenant_id, baseline_id, reset_at, requested_by)
		VALUES ($1, $2, now(), $3)
		ON CONFLICT (tenant_id, baseline_id)
		DO UPDATE SET reset_at=EXCLUDED.reset_at, requested_by=EXCLUDED.requested_by
		RETURNING reset_at`, tenantID, baselineID, httpx.GetUserID(ctx)).Scan(&resetAt); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO behavior_baseline_settings (tenant_id, baseline_id, frozen, drift_watch, updated_by, updated_at)
		VALUES ($1, $2, false, false, $3, now())
		ON CONFLICT (tenant_id, baseline_id) DO UPDATE SET
		frozen=false, drift_watch=false, version=behavior_baseline_settings.version+1,
		updated_by=EXCLUDED.updated_by, updated_at=now()`, tenantID, baselineID, httpx.GetUserID(ctx)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := insertFusionAuditTx(ctx, tx, tenantID, httpx.GetUserID(ctx), "BEHAVIOR_BASELINE_RESET", "baseline", baselineID, map[string]interface{}{"status": "reset"}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	baseline.Status = "learning"
	baseline.CreatedAt = resetAt.UnixMilli()
	baseline.UpdatedAt = resetAt.UnixMilli()
	for index := range baseline.Metrics {
		baseline.Metrics[index].Mean = 0
		baseline.Metrics[index].StdDev = 0
		baseline.Metrics[index].CurrentValue = 0
		baseline.Metrics[index].DeviationScore = 0
		baseline.Metrics[index].NormalRange = [2]float64{}
	}
	httpx.JSONSuccess(w, ctx, baseline)
}

// SubmitBehaviorBaselineAction persists every governance request and its audit event.
// External alert, forensics and model systems consume queued requests asynchronously;
// local threshold, freeze, drift, rebuild and rollback state is applied transactionally.
func (h *SystemHandler) SubmitBehaviorBaselineAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireBehaviorBaselineWritePermission(w, r) || !h.requirePostgres(w, ctx) {
		return
	}

	baselineID := strings.TrimSpace(mux.Vars(r)["id"])
	if baselineID == "" || len(baselineID) > 255 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "baseline id is required")
		return
	}
	var req behaviorBaselineActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid behavior baseline action payload")
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Reason = strings.TrimSpace(req.Reason)
	allowed := map[string]bool{
		"create_alert": true, "adjust_threshold": true, "freeze": true, "unfreeze": true,
		"forensics": true, "feedback_model": true, "cold_start": true, "drift_watch": true,
		"rebuild": true, "rollback": true, "audit_trace": true,
	}
	if !allowed[req.Action] {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "unsupported behavior baseline action")
		return
	}
	if len(req.Reason) > 500 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "reason is too long")
		return
	}
	if req.Action != "audit_trace" && req.Reason == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "reason is required for behavior baseline governance actions")
		return
	}
	if req.Action == "adjust_threshold" {
		if req.WarningMultiplier == nil || req.AlertMultiplier == nil || *req.WarningMultiplier <= 0 || *req.AlertMultiplier <= *req.WarningMultiplier || *req.AlertMultiplier > 20 {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "thresholds require 0 < warning_multiplier < alert_multiplier <= 20")
			return
		}
	}
	if req.Action == "rollback" && (req.TargetVersion == nil || *req.TargetVersion <= 0) {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "target_version must be positive")
		return
	}
	if req.Detail == nil {
		req.Detail = map[string]interface{}{}
	}

	tenantID := writeTenantID(r)
	if _, err := h.queryBehaviorBaseline(ctx, tenantID, baselineID); err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "behavior baseline does not exist in the authenticated tenant")
		return
	}
	requestedBy := httpx.GetUserID(ctx)
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO behavior_baseline_settings (tenant_id, baseline_id, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, baseline_id) DO NOTHING`, tenantID, baselineID, requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO behavior_baseline_versions (tenant_id, baseline_id, version, snapshot, created_by)
		SELECT tenant_id, baseline_id, version,
		       jsonb_build_object('warning_multiplier', warning_multiplier, 'alert_multiplier', alert_multiplier, 'frozen', frozen, 'drift_watch', drift_watch), $3
		FROM behavior_baseline_settings WHERE tenant_id=$1 AND baseline_id=$2
		ON CONFLICT (tenant_id, baseline_id, version) DO NOTHING`, tenantID, baselineID, requestedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	status := "queued"
	localStateApplied := false
	switch req.Action {
	case "adjust_threshold":
		localStateApplied = true
		_, err = tx.ExecContext(ctx, `
			INSERT INTO behavior_baseline_settings (tenant_id, baseline_id, warning_multiplier, alert_multiplier, version, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, 1, $5, now())
			ON CONFLICT (tenant_id, baseline_id) DO UPDATE SET
			warning_multiplier=EXCLUDED.warning_multiplier,
			alert_multiplier=EXCLUDED.alert_multiplier,
			version=behavior_baseline_settings.version+1,
			updated_by=EXCLUDED.updated_by,
			updated_at=now()`, tenantID, baselineID, *req.WarningMultiplier, *req.AlertMultiplier, requestedBy)
	case "freeze", "unfreeze":
		localStateApplied = true
		_, err = tx.ExecContext(ctx, `
			INSERT INTO behavior_baseline_settings (tenant_id, baseline_id, frozen, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (tenant_id, baseline_id) DO UPDATE SET frozen=EXCLUDED.frozen, version=behavior_baseline_settings.version+1, updated_by=EXCLUDED.updated_by, updated_at=now()`, tenantID, baselineID, req.Action == "freeze", requestedBy)
	case "drift_watch":
		localStateApplied = true
		_, err = tx.ExecContext(ctx, `
			INSERT INTO behavior_baseline_settings (tenant_id, baseline_id, drift_watch, updated_by, updated_at)
			VALUES ($1, $2, true, $3, now())
			ON CONFLICT (tenant_id, baseline_id) DO UPDATE SET drift_watch=true, version=behavior_baseline_settings.version+1, updated_by=EXCLUDED.updated_by, updated_at=now()`, tenantID, baselineID, requestedBy)
	case "cold_start", "rebuild":
		localStateApplied = true
		_, err = tx.ExecContext(ctx, `
			INSERT INTO behavior_baseline_resets (tenant_id, baseline_id, reset_at, requested_by)
			VALUES ($1, $2, now(), $3)
			ON CONFLICT (tenant_id, baseline_id) DO UPDATE SET reset_at=EXCLUDED.reset_at, requested_by=EXCLUDED.requested_by`, tenantID, baselineID, requestedBy)
		if err == nil {
			initialVersion := 1
			if req.Action == "rebuild" {
				initialVersion = 2
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO behavior_baseline_settings (tenant_id, baseline_id, frozen, drift_watch, version, updated_by, updated_at)
				VALUES ($1, $2, false, false, $3, $4, now())
				ON CONFLICT (tenant_id, baseline_id) DO UPDATE SET
				frozen=false, drift_watch=false, version=behavior_baseline_settings.version+1,
				updated_by=EXCLUDED.updated_by, updated_at=now()`, tenantID, baselineID, initialVersion, requestedBy)
		}
	case "rollback":
		localStateApplied = true
		var snapshotJSON string
		if scanErr := tx.QueryRowContext(ctx, `SELECT snapshot::text FROM behavior_baseline_versions WHERE tenant_id=$1 AND baseline_id=$2 AND version=$3`, tenantID, baselineID, *req.TargetVersion).Scan(&snapshotJSON); scanErr != nil {
			if scanErr == sql.ErrNoRows {
				httpx.JSONError(w, ctx, http.StatusConflict, "VERSION_NOT_FOUND", "target behavior baseline version does not exist")
				return
			}
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", scanErr.Error())
			return
		}
		var snapshot struct {
			WarningMultiplier float64 `json:"warning_multiplier"`
			AlertMultiplier   float64 `json:"alert_multiplier"`
			Frozen            bool    `json:"frozen"`
			DriftWatch        bool    `json:"drift_watch"`
		}
		if json.Unmarshal([]byte(snapshotJSON), &snapshot) != nil || snapshot.WarningMultiplier <= 0 || snapshot.AlertMultiplier <= snapshot.WarningMultiplier {
			httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_VERSION", "target behavior baseline version snapshot is invalid")
			return
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE behavior_baseline_settings SET warning_multiplier=$3, alert_multiplier=$4, frozen=$5, drift_watch=$6,
			version=version+1, updated_by=$7, updated_at=now() WHERE tenant_id=$1 AND baseline_id=$2`,
			tenantID, baselineID, snapshot.WarningMultiplier, snapshot.AlertMultiplier, snapshot.Frozen, snapshot.DriftWatch, requestedBy)
	case "audit_trace":
		localStateApplied = true
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if localStateApplied {
		status = "applied"
	}

	requestMap := map[string]interface{}{"detail": req.Detail}
	if req.WarningMultiplier != nil {
		requestMap["warning_multiplier"] = *req.WarningMultiplier
	}
	if req.AlertMultiplier != nil {
		requestMap["alert_multiplier"] = *req.AlertMultiplier
	}
	if req.TargetVersion != nil {
		requestMap["target_version"] = *req.TargetVersion
	}
	requestJSON, _ := json.Marshal(requestMap)
	var action behaviorBaselineActionDTO
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO behavior_baseline_actions (tenant_id, baseline_id, action_type, status, reason, request, requested_by)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		RETURNING action_id::text, created_at`, tenantID, baselineID, req.Action, status, req.Reason, string(requestJSON), requestedBy).Scan(&action.ActionID, &createdAt)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if req.Action == "adjust_threshold" || req.Action == "freeze" || req.Action == "unfreeze" || req.Action == "drift_watch" || req.Action == "rebuild" || req.Action == "rollback" {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO behavior_baseline_versions (tenant_id, baseline_id, version, snapshot, source_action_id, created_by)
			SELECT tenant_id, baseline_id, version,
			       jsonb_build_object('warning_multiplier', warning_multiplier, 'alert_multiplier', alert_multiplier, 'frozen', frozen, 'drift_watch', drift_watch),
			       $3::uuid, $4
			FROM behavior_baseline_settings WHERE tenant_id=$1 AND baseline_id=$2
			ON CONFLICT (tenant_id, baseline_id, version) DO NOTHING`, tenantID, baselineID, action.ActionID, requestedBy)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
	}
	if req.Action != "audit_trace" {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO behavior_baseline_outbox (tenant_id, baseline_id, action_id, event_type, payload)
			VALUES ($1, $2, $3::uuid, $4, $5::jsonb)`, tenantID, baselineID, action.ActionID, "behavior.baseline."+req.Action, string(requestJSON))
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
	}
	auditAction := "BEHAVIOR_BASELINE_" + strings.ToUpper(req.Action)
	if err = insertFusionAuditTx(ctx, tx, tenantID, requestedBy, auditAction, "baseline", baselineID, map[string]interface{}{"status": status, "reason": req.Reason, "request": requestMap}, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	action.TenantID = tenantID
	action.BaselineID = baselineID
	action.Action = req.Action
	action.Status = status
	action.LocalStateApplied = localStateApplied
	if req.Action != "audit_trace" {
		action.DownstreamStatus = "queued"
	}
	action.Reason = req.Reason
	action.Request = requestMap
	action.RequestedBy = requestedBy
	action.CreatedAt = createdAt.UnixMilli()
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"action": action, "audit_written": true})
}

func (h *SystemHandler) ListComplianceReports(w http.ResponseWriter, r *http.Request) {
	if !h.requireComplianceReadPermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 100)
	reportType := r.URL.Query().Get("report_type")

	args := []interface{}{tenantID}
	where := `tenant_id=$1
		AND status <> 'invalidated'
		AND NOT (
			status = 'completed'
			AND COALESCE((summary->>'total_alerts')::bigint, 0) = 0
			AND NOT EXISTS (
				SELECT 1 FROM jsonb_array_elements(sections) AS section
				WHERE COALESCE(section->>'status', '') <> 'pass'
			)
		)`
	if reportType != "" {
		args = append(args, reportType)
		where += fmt.Sprintf(" AND report_type=$%d", len(args))
	}
	var total int
	if err := h.pgDB.QueryRowContext(ctx, "SELECT count(*) FROM compliance_reports WHERE "+where, args...).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	args = append(args, limit, offset)
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT report_id::text, tenant_id, report_type, time_start, time_end, status, summary::text, sections::text, generated_by, generated_at
		FROM compliance_reports WHERE `+where+`
		ORDER BY generated_at DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	reports := make([]complianceReportDTO, 0)
	for rows.Next() {
		report, err := scanComplianceReport(rows)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"reports": reports, "total": total})
}

func (h *SystemHandler) GenerateComplianceReport(w http.ResponseWriter, r *http.Request) {
	if !h.requireComplianceReportGeneratePermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	var req struct {
		ReportType string           `json:"report_type"`
		TimeRange  *complianceRange `json:"time_range"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	req.ReportType = strings.ToLower(strings.TrimSpace(req.ReportType))
	if req.ReportType == "" {
		req.ReportType = "weekly"
	}
	if req.ReportType != "weekly" && req.ReportType != "monthly" && req.ReportType != "custom" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REPORT_TYPE", "report_type must be weekly, monthly or custom")
		return
	}
	start, end := complianceReportRange(req.ReportType, req.TimeRange)
	if err := validateComplianceRange(start, end, time.Now()); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_TIME_RANGE", err.Error())
		return
	}
	summary, err := h.complianceSummary(ctx, tenantID, start, end)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "COMPLIANCE_SOURCE_UNAVAILABLE", err.Error())
		return
	}
	sections := complianceSections(summary)
	status := complianceReportStatus(sections)

	summaryJSON, _ := json.Marshal(summary)
	sectionsJSON, _ := json.Marshal(sections)
	var reportID string
	var generatedAt time.Time
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		INSERT INTO compliance_reports (tenant_id, report_type, time_start, time_end, status, summary, sections, generated_by)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
		RETURNING report_id::text, generated_at`, tenantID, req.ReportType, start, end, status, string(summaryJSON), string(sectionsJSON), httpx.GetUserID(ctx)).Scan(&reportID, &generatedAt)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "COMPLIANCE_REPORT_GENERATED", ObjectType: "compliance_report", ObjectID: reportID,
		TenantID: tenantID, UserID: httpx.GetUserID(ctx), Result: "success",
		Detail: map[string]interface{}{"report_type": req.ReportType, "time_start": start, "time_end": end, "source_status": status, "total_alerts": summary.TotalAlerts},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "COMPLIANCE_AUDIT_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, complianceReportDTO{
		ReportID: reportID, TenantID: tenantID, ReportType: req.ReportType,
		TimeRange: map[string]int64{"start": start, "end": end}, GeneratedAt: generatedAt.UnixMilli(),
		Status: status, Summary: summary, Sections: sections, GeneratedBy: httpx.GetUserID(ctx),
	})
}

func (h *SystemHandler) ExportComplianceEvidencePackage(w http.ResponseWriter, r *http.Request) {
	if !h.requireComplianceExportPermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := writeTenantID(r)
	reportID := strings.TrimSpace(mux.Vars(r)["id"])
	if reportID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "report id is required")
		return
	}
	report, err := h.loadComplianceReport(ctx, tenantID, reportID)
	if err != nil {
		if errorsIsNoRows(err) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "compliance report not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	content, checksum, err := buildComplianceEvidencePackage(report)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	exportID := "COMP-EVIDENCE-" + strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte(reportID+time.Now().UTC().String())))[:12])
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	writer := NewAlertActionAuditWriter(h.pgDB, h.logger)
	if err := writer.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "COMPLIANCE_EVIDENCE_EXPORTED", ObjectType: "compliance_report", ObjectID: reportID,
		TenantID: tenantID, UserID: httpx.GetUserID(ctx), Result: "success",
		Detail: map[string]interface{}{"export_id": exportID, "sha256": checksum, "artifact_type": "evidence_package", "size_bytes": len(content)},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "COMPLIANCE_AUDIT_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, complianceExportDTO{
		ExportID: exportID, ReportID: reportID, ArtifactType: "evidence_package",
		Filename: "compliance-evidence-" + reportID + ".zip", MIMEType: "application/zip",
		SHA256: checksum, ContentBase64: base64.StdEncoding.EncodeToString(content), GeneratedAt: time.Now().UnixMilli(),
	})
}

func (h *SystemHandler) ListAuditTrail(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditReadPermission(w, r) {
		return
	}
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 200)
	action := r.URL.Query().Get("action")
	userID := r.URL.Query().Get("user_id")
	objectType := firstNonEmpty(r.URL.Query().Get("object_type"), r.URL.Query().Get("resource_type"))
	objectID := firstNonEmpty(r.URL.Query().Get("object_id"), r.URL.Query().Get("resource_id"))

	args := []interface{}{tenantID}
	where := "tenant_id=$1"
	if action != "" {
		args = append(args, action)
		where += fmt.Sprintf(" AND action=$%d", len(args))
	}
	if userID != "" {
		args = append(args, userID)
		where += fmt.Sprintf(" AND user_id::text=$%d", len(args))
	}
	if objectType != "" {
		args = append(args, objectType)
		where += fmt.Sprintf(" AND object_type=$%d", len(args))
	}
	if objectID != "" {
		args = append(args, objectID)
		where += fmt.Sprintf(" AND object_id=$%d", len(args))
	}
	var total int
	if err := h.pgDB.QueryRowContext(ctx, "SELECT count(*) FROM audit_logs WHERE "+where, args...).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	args = append(args, limit, offset)
	idExpr := "id::text"
	if h.pgColumnExists(ctx, "audit_logs", "event_id") {
		idExpr = "COALESCE(event_id, id::text)"
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT `+idExpr+`, tenant_id, COALESCE(user_id::text,''), action, COALESCE(object_type,''), COALESCE(object_id,''), COALESCE(detail,'{}'::jsonb)::text, COALESCE(ip_addr,''), created_at
		FROM audit_logs WHERE `+where+`
		ORDER BY created_at DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()
	trails := make([]auditTrailDTO, 0)
	for rows.Next() {
		var trail auditTrailDTO
		var detailJSON string
		var createdAt time.Time
		if err := rows.Scan(&trail.LogID, &trail.TenantID, &trail.UserID, &trail.Action, &trail.ResourceType, &trail.ResourceID, &detailJSON, &trail.IPAddress, &createdAt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		trail.Details = map[string]interface{}{}
		_ = json.Unmarshal([]byte(detailJSON), &trail.Details)
		trail.Timestamp = createdAt.UnixMilli()
		trail.Result = auditResult(trail.Action, trail.Details)
		trails = append(trails, trail)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"trails": trails, "total": total})
}

type complianceRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

func (h *SystemHandler) requirePostgres(w http.ResponseWriter, ctx context.Context) bool {
	if h.pgDB != nil {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "postgres is not configured")
	return false
}

func writeTenantID(r *http.Request) string {
	if tenantID := httpx.GetTenantID(r.Context()); tenantID != "" {
		return tenantID
	}
	return queryTenantID(r)
}

func (h *SystemHandler) requireFusionWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeRuleWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: rule:write required")
	return false
}

func (h *SystemHandler) requireComplianceReportGeneratePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeComplianceWrite, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: compliance:write required")
	return false
}

func (h *SystemHandler) requireComplianceReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeComplianceRead, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: compliance:read required")
	return false
}

func (h *SystemHandler) requireComplianceExportPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeComplianceExport, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: compliance:export required")
	return false
}

func (h *SystemHandler) requireComplianceRemediatePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeComplianceRemediate, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: compliance:remediate required")
	return false
}

func (h *SystemHandler) requireComplianceFinalizePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeComplianceFinalize, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: compliance:finalize required")
	return false
}

func (h *SystemHandler) requireAuditReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeAuditRead, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: audit:read required")
	return false
}

func (h *SystemHandler) requireBehaviorBaselineWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, authmodel.ScopeAlertWrite) || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: alert:write required")
	return false
}

func hasSystemPermission(ctx context.Context, permission string) bool {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasRole("admin") || claims.HasRole("super_admin") || claims.HasPermission(permission) || claims.HasPermission(authmodel.ScopeAdminAll)
	}
	if httpx.HasRole(ctx, "admin") || httpx.HasRole(ctx, "super_admin") {
		return true
	}
	for _, granted := range httpx.GetPermissions(ctx) {
		if permissionMatches(granted, permission) || permissionMatches(granted, authmodel.ScopeAdminAll) {
			return true
		}
	}
	return false
}

func permissionMatches(granted, required string) bool {
	granted = strings.TrimSpace(granted)
	if granted == authmodel.ScopeAll || granted == required {
		return true
	}
	if strings.HasSuffix(granted, ":*") {
		return strings.HasPrefix(required, strings.TrimSuffix(granted, "*"))
	}
	return false
}

func (h *SystemHandler) requireTopicWritePermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, "topic:write") || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: topic:write required")
	return false
}

func (h *SystemHandler) requireTopicExportPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasSystemPermission(ctx, "topic:export") || hasSystemPermission(ctx, "topic:write") || hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: topic:export required")
	return false
}

func (h *SystemHandler) ensureTopicGovernanceSchema(w http.ResponseWriter, ctx context.Context) bool {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS topic_saved_views (
			view_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			name TEXT NOT NULL,
			filters JSONB NOT NULL DEFAULT '{}'::jsonb,
			visibility TEXT NOT NULL DEFAULT 'private',
			favorite BOOLEAN NOT NULL DEFAULT false,
			shared BOOLEAN NOT NULL DEFAULT false,
			share_token TEXT,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_topic_saved_views_tenant_topic
			ON topic_saved_views (tenant_id, topic, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS topic_scope_overrides (
			tenant_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			scope_name TEXT NOT NULL DEFAULT '',
			included_assets JSONB NOT NULL DEFAULT '[]'::jsonb,
			excluded_assets JSONB NOT NULL DEFAULT '[]'::jsonb,
			risk_levels JSONB NOT NULL DEFAULT '[]'::jsonb,
			time_window TEXT NOT NULL DEFAULT '24h',
			detail JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_by TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (tenant_id, topic)
		)`,
		`CREATE TABLE IF NOT EXISTS topic_subscriptions (
			subscription_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			channel TEXT NOT NULL,
			threshold TEXT NOT NULL DEFAULT 'high',
			schedule TEXT NOT NULL DEFAULT 'realtime',
			recipients JSONB NOT NULL DEFAULT '[]'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			detail JSONB NOT NULL DEFAULT '{}'::jsonb
		)`,
		`CREATE INDEX IF NOT EXISTS idx_topic_subscriptions_tenant_topic
			ON topic_subscriptions (tenant_id, topic, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS topic_exports (
			export_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			export_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'completed',
			parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
			result JSONB NOT NULL DEFAULT '{}'::jsonb,
			generated_by TEXT NOT NULL DEFAULT '',
			generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_topic_exports_tenant_time
			ON topic_exports (tenant_id, generated_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := h.pgDB.ExecContext(ctx, stmt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
			return false
		}
	}
	return true
}

func normalizeTopicKey(topic string) string {
	return strings.TrimSpace(strings.ToLower(topic))
}

func isValidTopicKey(topic string) bool {
	switch normalizeTopicKey(topic) {
	case "tunnel", "exfil", "apt":
		return true
	default:
		return false
	}
}

func topicVisibility(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "team", "tenant":
		return "team"
	case "public":
		return "public"
	default:
		return "private"
	}
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

type sqlScanner interface {
	Scan(dest ...interface{}) error
}

type fusionQueryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func scanTopicView(scanner sqlScanner) (topicViewDTO, error) {
	var view topicViewDTO
	var filtersJSON string
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&view.ViewID, &view.TenantID, &view.Topic, &view.Name, &filtersJSON, &view.Visibility, &view.Favorite, &view.Shared, &view.ShareToken, &view.CreatedBy, &createdAt, &updatedAt); err != nil {
		return topicViewDTO{}, err
	}
	view.Filters = map[string]interface{}{}
	_ = json.Unmarshal([]byte(filtersJSON), &view.Filters)
	view.CreatedAt = createdAt.UnixMilli()
	view.UpdatedAt = updatedAt.UnixMilli()
	return view, nil
}

func scanTopicScope(scanner sqlScanner) (topicScopeDTO, error) {
	var scope topicScopeDTO
	var includedJSON, excludedJSON, risksJSON, detailJSON string
	var updatedAt time.Time
	if err := scanner.Scan(&scope.TenantID, &scope.Topic, &scope.ScopeName, &includedJSON, &excludedJSON, &risksJSON, &scope.TimeWindow, &scope.UpdatedBy, &updatedAt, &detailJSON); err != nil {
		return topicScopeDTO{}, err
	}
	scope.IncludedAssets = jsonStringList(includedJSON)
	scope.ExcludedAssets = jsonStringList(excludedJSON)
	scope.RiskLevels = jsonStringList(risksJSON)
	scope.Detail = map[string]interface{}{}
	_ = json.Unmarshal([]byte(detailJSON), &scope.Detail)
	scope.UpdatedAt = updatedAt.UnixMilli()
	return scope, nil
}

func scanTopicSubscription(scanner sqlScanner) (topicSubscriptionDTO, error) {
	var subscription topicSubscriptionDTO
	var recipientsJSON, detailJSON string
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&subscription.SubscriptionID, &subscription.TenantID, &subscription.Topic, &subscription.Channel, &subscription.Threshold, &subscription.Schedule, &recipientsJSON, &subscription.Enabled, &subscription.CreatedBy, &createdAt, &updatedAt, &detailJSON); err != nil {
		return topicSubscriptionDTO{}, err
	}
	subscription.Recipients = jsonStringList(recipientsJSON)
	subscription.Detail = map[string]interface{}{}
	_ = json.Unmarshal([]byte(detailJSON), &subscription.Detail)
	subscription.CreatedAt = createdAt.UnixMilli()
	subscription.UpdatedAt = updatedAt.UnixMilli()
	return subscription, nil
}

func scanTopicExport(scanner sqlScanner) (topicExportDTO, error) {
	var exported topicExportDTO
	var parametersJSON, resultJSON string
	var generatedAt time.Time
	if err := scanner.Scan(&exported.ExportID, &exported.TenantID, &exported.Topic, &exported.ExportType, &exported.Status, &parametersJSON, &resultJSON, &exported.GeneratedBy, &generatedAt); err != nil {
		return topicExportDTO{}, err
	}
	exported.Parameters = map[string]interface{}{}
	exported.Result = map[string]interface{}{}
	_ = json.Unmarshal([]byte(parametersJSON), &exported.Parameters)
	_ = json.Unmarshal([]byte(resultJSON), &exported.Result)
	exported.GeneratedAt = generatedAt.UnixMilli()
	return exported, nil
}

func jsonStringList(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	return cleanStringList(values)
}

func (h *SystemHandler) queryTopicView(ctx context.Context, tenantID, viewID string) (topicViewDTO, error) {
	return scanTopicView(h.pgDB.QueryRowContext(ctx, `
		SELECT view_id::text, tenant_id, topic, name, filters::text, visibility, favorite, shared, COALESCE(share_token, ''), created_by, created_at, updated_at
		FROM topic_saved_views
		WHERE tenant_id=$1 AND view_id=$2`, tenantID, viewID))
}

func (h *SystemHandler) queryTopicSubscription(ctx context.Context, tenantID, subscriptionID string) (topicSubscriptionDTO, error) {
	return scanTopicSubscription(h.pgDB.QueryRowContext(ctx, `
		SELECT subscription_id::text, tenant_id, topic, channel, threshold, schedule, recipients::text, enabled, created_by, created_at, updated_at, detail::text
		FROM topic_subscriptions
		WHERE tenant_id=$1 AND subscription_id=$2`, tenantID, subscriptionID))
}

func topicExportSummary(topic, exportType string) string {
	switch exportType {
	case "evidence_package":
		return fmt.Sprintf("%s topic evidence package with source APIs, filters and audit trail", topic)
	default:
		return fmt.Sprintf("%s topic operational report", topic)
	}
}

func (req *fusionConflictResolveRequest) normalize() {
	req.ObjectID = strings.TrimSpace(req.ObjectID)
	req.ObjectType = nonEmpty(strings.TrimSpace(req.ObjectType), "entity")
	req.FieldName = strings.TrimSpace(req.FieldName)
	req.SelectedSource = strings.TrimSpace(req.SelectedSource)
	req.SelectedValue = strings.TrimSpace(req.SelectedValue)
	req.Strategy = nonEmpty(strings.TrimSpace(req.Strategy), "manual")
	req.Note = strings.TrimSpace(req.Note)
	req.RuleID = strings.TrimSpace(req.RuleID)
	if req.Detail == nil {
		req.Detail = map[string]interface{}{}
	}
}

func fusionSourceValueMatches(sourceValues []map[string]interface{}, selectedSource, selectedValue string) bool {
	for _, sourceValue := range sourceValues {
		source, sourceOK := sourceValue["source"].(string)
		value, valueOK := sourceValue["value"].(string)
		if sourceOK && valueOK && source == selectedSource && value == selectedValue {
			return true
		}
	}
	return false
}

func (req *fusionRuleUpdateRequest) normalize(ruleID string) {
	req.RuleName = nonEmpty(strings.TrimSpace(req.RuleName), ruleID)
	req.Status = nonEmpty(strings.TrimSpace(req.Status), "draft")
	req.Strategy = nonEmpty(strings.TrimSpace(req.Strategy), "manual-review")
	req.Note = strings.TrimSpace(req.Note)
	if req.ConfidenceThreshold == nil {
		value := 0.85
		req.ConfidenceThreshold = &value
	}
	if req.Detail == nil {
		req.Detail = map[string]interface{}{}
	}
}

func validFusionRuleStatus(value string) bool {
	switch value {
	case "active", "draft", "disabled":
		return true
	default:
		return false
	}
}

func validFusionRuleStrategy(value string) bool {
	switch value {
	case "authoritative-source", "weighted-confidence", "latest-observation", "manual-review":
		return true
	default:
		return false
	}
}

func (h *SystemHandler) countFusionRules(ctx context.Context, tenantID string) (int64, error) {
	var total int64
	err := h.pgDB.QueryRowContext(ctx, `SELECT count(*) FROM fusion_rule_overrides
		WHERE tenant_id=$1 AND detail ? 'source_a' AND detail ? 'source_b' AND detail ? 'field'`, tenantID).Scan(&total)
	return total, err
}

func scanFusionRule(scanner sqlScanner) (fusionRuleOverrideDTO, error) {
	var item fusionRuleOverrideDTO
	var updatedAt time.Time
	var detailJSON string
	if err := scanner.Scan(&item.TenantID, &item.RuleID, &item.RuleName, &item.Version, &item.Status, &item.Strategy, &item.ConfidenceThreshold, &item.Note, &item.UpdatedBy, &updatedAt, &detailJSON); err != nil {
		return item, err
	}
	item.UpdatedAt = updatedAt.UnixMilli()
	item.Detail = map[string]interface{}{}
	_ = json.Unmarshal([]byte(detailJSON), &item.Detail)
	return item, nil
}

func getFusionRule(ctx context.Context, queryer fusionQueryer, tenantID, ruleID string) (fusionRuleOverrideDTO, error) {
	return scanFusionRule(queryer.QueryRowContext(ctx, `SELECT tenant_id, rule_id, rule_name, version, status, strategy, confidence_threshold, note, updated_by, updated_at, detail::text
		FROM fusion_rule_overrides WHERE tenant_id=$1 AND rule_id=$2`, tenantID, ruleID))
}

func (h *SystemHandler) listFusionRules(ctx context.Context, tenantID string, limit, offset int) ([]fusionRuleOverrideDTO, error) {
	rows, err := h.pgDB.QueryContext(ctx, `SELECT tenant_id, rule_id, rule_name, version, status, strategy, confidence_threshold, note, updated_by, updated_at, detail::text
		FROM fusion_rule_overrides
		WHERE tenant_id=$1 AND detail ? 'source_a' AND detail ? 'source_b' AND detail ? 'field'
		ORDER BY CASE rule_id
			WHEN 'IP_MAC_BIND_V3' THEN 1 WHEN 'ACCOUNT_HOST_LINK' THEN 2 WHEN 'ASSET_DEPT_COMPLETION' THEN 3
			WHEN 'DOMAIN_IP_RESOLUTION' THEN 4 WHEN 'ALERT_ASSET_JOIN' THEN 5 WHEN 'VULN_SERVICE_MATCH' THEN 6 ELSE 99 END,
			rule_id
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]fusionRuleOverrideDTO, 0)
	for rows.Next() {
		item, err := scanFusionRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanFusionConflict(scanner sqlScanner) (fusionConflictDTO, error) {
	var item fusionConflictDTO
	var valuesJSON string
	var detailJSON string
	var detectedAt, updatedAt time.Time
	if err := scanner.Scan(&item.TenantID, &item.ConflictID, &item.ObjectID, &item.ObjectType, &item.FieldName, &valuesJSON, &item.SourceCount, &item.Confidence, &item.Severity, &item.Status, &item.RuleID, &item.StateVersion, &item.Origin, &detailJSON, &detectedAt, &updatedAt); err != nil {
		return item, err
	}
	item.SourceValues = []map[string]interface{}{}
	_ = json.Unmarshal([]byte(valuesJSON), &item.SourceValues)
	item.Detail = map[string]interface{}{}
	_ = json.Unmarshal([]byte(detailJSON), &item.Detail)
	item.DetectedAt = detectedAt.UnixMilli()
	item.UpdatedAt = updatedAt.UnixMilli()
	return item, nil
}

const fusionConflictSelect = `SELECT tenant_id, conflict_id, object_id, object_type, field_name, source_values::text, source_count, confidence, severity, status, rule_id, state_version, origin, detail::text, detected_at, updated_at FROM fusion_conflicts`

func (h *SystemHandler) listFusionConflicts(ctx context.Context, tenantID string, limit, offset int) ([]fusionConflictDTO, error) {
	rows, err := h.pgDB.QueryContext(ctx, fusionConflictSelect+` WHERE tenant_id=$1 AND status <> 'resolved' ORDER BY CASE status WHEN 'pending' THEN 0 WHEN 'repair_pending' THEN 1 ELSE 2 END, detected_at DESC, conflict_id ASC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]fusionConflictDTO, 0)
	for rows.Next() {
		item, scanErr := scanFusionConflict(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SystemHandler) fusionConflictSummary(ctx context.Context, tenantID string) (pending, resolved, total int64, risk map[string]int64, err error) {
	risk = map[string]int64{"high": 0, "medium": 0, "low": 0}
	var high, medium, low int64
	err = h.pgDB.QueryRowContext(ctx, `SELECT
		count(*) FILTER (WHERE status <> 'resolved'),
		count(*) FILTER (WHERE status = 'resolved'),
		count(*),
		count(*) FILTER (WHERE status <> 'resolved' AND severity IN ('high','critical')),
		count(*) FILTER (WHERE status <> 'resolved' AND severity = 'medium'),
		count(*) FILTER (WHERE status <> 'resolved' AND severity = 'low')
		FROM fusion_conflicts WHERE tenant_id=$1`, tenantID).Scan(
		&pending, &resolved, &total, &high, &medium, &low)
	risk["high"], risk["medium"], risk["low"] = high, medium, low
	return
}

func (h *SystemHandler) getFusionConflict(ctx context.Context, tenantID, conflictID string) (fusionConflictDTO, error) {
	return getFusionConflictFrom(ctx, h.pgDB, tenantID, conflictID)
}

func getFusionConflictFrom(ctx context.Context, queryer fusionQueryer, tenantID, conflictID string) (fusionConflictDTO, error) {
	return scanFusionConflict(queryer.QueryRowContext(ctx, fusionConflictSelect+` WHERE tenant_id=$1 AND conflict_id=$2`, tenantID, conflictID))
}

func getFusionConflictResolution(ctx context.Context, queryer fusionQueryer, tenantID, conflictID string) (*fusionConflictResolutionDTO, error) {
	var item fusionConflictResolutionDTO
	var resolvedAt time.Time
	var detailJSON string
	err := queryer.QueryRowContext(ctx, `SELECT tenant_id, conflict_id, object_id, object_type, field_name, selected_source, selected_value, strategy, note, rule_id, state_version, resolved_by, resolved_at, detail::text
		FROM fusion_conflict_resolutions WHERE tenant_id=$1 AND conflict_id=$2`, tenantID, conflictID).Scan(
		&item.TenantID, &item.ConflictID, &item.ObjectID, &item.ObjectType, &item.FieldName,
		&item.SelectedSource, &item.SelectedValue, &item.Strategy, &item.Note, &item.RuleID,
		&item.StateVersion, &item.ResolvedBy, &resolvedAt, &detailJSON,
	)
	if err != nil {
		return nil, err
	}
	item.ResolvedAt = resolvedAt.UnixMilli()
	item.Detail = map[string]interface{}{}
	_ = json.Unmarshal([]byte(detailJSON), &item.Detail)
	return &item, nil
}

func listFusionRepairTaskEvidence(ctx context.Context, queryer fusionQueryer, tenantID, conflictID string) ([]fusionRepairTaskEvidenceDTO, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT task_id::text, tenant_id, conflict_id, object_id, object_type, field_name, rule_id, selected_source, selected_value, state_version, status, requested_by, note, detail::text, created_at, updated_at
		FROM fusion_repair_tasks WHERE tenant_id=$1 AND conflict_id=$2 ORDER BY state_version, created_at, task_id`, tenantID, conflictID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]fusionRepairTaskEvidenceDTO, 0)
	for rows.Next() {
		var item fusionRepairTaskEvidenceDTO
		var detailJSON string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.TaskID, &item.TenantID, &item.ConflictID, &item.ObjectID, &item.ObjectType, &item.FieldName, &item.RuleID, &item.SelectedSource, &item.SelectedValue, &item.StateVersion, &item.Status, &item.RequestedBy, &item.Note, &detailJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.Detail = map[string]interface{}{}
		_ = json.Unmarshal([]byte(detailJSON), &item.Detail)
		item.CreatedAt = createdAt.UnixMilli()
		item.UpdatedAt = updatedAt.UnixMilli()
		items = append(items, item)
	}
	return items, rows.Err()
}

func listFusionEvidenceAuditEvents(ctx context.Context, queryer fusionQueryer, tenantID, conflictID, ruleID string) ([]auditTrailDTO, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT COALESCE(event_id,id::text), tenant_id, COALESCE(user_id::text,''), action, COALESCE(object_type,''), COALESCE(object_id,''), COALESCE(detail,'{}'::jsonb)::text, COALESCE(ip_addr,''), created_at
		FROM audit_logs
		WHERE tenant_id=$1 AND action LIKE 'FUSION_%'
		  AND (object_id=$2 OR ($3 <> '' AND object_id=$3) OR detail->>'conflict_id'=$2)
		ORDER BY created_at ASC, COALESCE(event_id,id::text) ASC`, tenantID, conflictID, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]auditTrailDTO, 0)
	for rows.Next() {
		var item auditTrailDTO
		var detailJSON string
		var createdAt time.Time
		if err := rows.Scan(&item.LogID, &item.TenantID, &item.UserID, &item.Action, &item.ResourceType, &item.ResourceID, &detailJSON, &item.IPAddress, &createdAt); err != nil {
			return nil, err
		}
		item.Details = map[string]interface{}{}
		_ = json.Unmarshal([]byte(detailJSON), &item.Details)
		item.Timestamp = createdAt.UnixMilli()
		item.Result = auditResult(item.Action, item.Details)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SystemHandler) listFusionAuditEvents(ctx context.Context, tenantID string, limit, offset int) ([]auditTrailDTO, error) {
	rows, err := h.pgDB.QueryContext(ctx, `SELECT COALESCE(event_id,id::text), tenant_id, COALESCE(user_id::text,''), action, COALESCE(object_type,''), COALESCE(object_id,''), COALESCE(detail,'{}'::jsonb)::text, COALESCE(ip_addr,''), created_at
		FROM audit_logs WHERE tenant_id=$1 AND action LIKE 'FUSION_%' ORDER BY created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]auditTrailDTO, 0)
	for rows.Next() {
		var item auditTrailDTO
		var detailJSON string
		var createdAt time.Time
		if err := rows.Scan(&item.LogID, &item.TenantID, &item.UserID, &item.Action, &item.ResourceType, &item.ResourceID, &detailJSON, &item.IPAddress, &createdAt); err != nil {
			return nil, err
		}
		item.Details = map[string]interface{}{}
		_ = json.Unmarshal([]byte(detailJSON), &item.Details)
		item.Timestamp = createdAt.UnixMilli()
		item.Result = auditResult(item.Action, item.Details)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SystemHandler) countFusionAuditEvents(ctx context.Context, tenantID string) (int64, error) {
	var total int64
	err := h.pgDB.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action LIKE 'FUSION_%'`, tenantID).Scan(&total)
	return total, err
}

func boundedIntQuery(r *http.Request, key string, defaultValue, maxValue int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || value < 0 {
		value = defaultValue
	}
	if value > maxValue {
		value = maxValue
	}
	return value
}

func boundedPositiveIntQuery(r *http.Request, key string, defaultValue, maxValue int) int {
	value := boundedIntQuery(r, key, defaultValue, maxValue)
	if value <= 0 {
		return defaultValue
	}
	return value
}

func (h *SystemHandler) fusionEntityCounts(ctx context.Context, tenantID string) map[string]int64 {
	counts := map[string]int64{"host": 0, "account": 0, "asset": 0, "domain": 0, "service": 0, "alert": 0}
	rows, err := h.pgDB.QueryContext(ctx, `SELECT asset_type, count(*) FROM assets WHERE tenant_id=$1 GROUP BY asset_type`, tenantID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var assetType string
			var count int64
			if rows.Scan(&assetType, &count) == nil {
				counts["asset"] += count
				switch assetType {
				case "server", "endpoint", "network-device":
					counts["host"] += count
				case "business-system":
					counts["service"] += count
				}
			}
		}
	}
	var accountCount int64
	_ = h.pgDB.QueryRowContext(ctx, `SELECT count(DISTINCT user_id::text) FROM audit_logs WHERE tenant_id=$1 AND user_id IS NOT NULL`, tenantID).Scan(&accountCount)
	counts["account"] = accountCount
	return counts
}

func slugIdentifier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	return strings.Trim(value, "-")
}

func (h *SystemHandler) fusionSources(ctx context.Context, tenantID string) []dataSourceDTO {
	now := time.Now().UnixMilli()
	loaders := []func() dataSourceDTO{
		func() dataSourceDTO {
			return h.clickHouseSource(ctx, tenantID, "traffic", "traffic", "流量元数据", "traffic.sessions", "ingest_ts", now)
		},
		func() dataSourceDTO {
			return h.postgresSource(ctx, tenantID, "asset", "asset", "资产信息", "assets", "created_at", now)
		},
		func() dataSourceDTO {
			return h.clickHouseDateTimeSource(ctx, tenantID, "log", "log", "设备日志", "traffic.device_logs", "timestamp", now)
		},
		func() dataSourceDTO {
			return h.clickHouseDateTimeSource(ctx, tenantID, "behavior", "behavior", "用户事件", "traffic.user_events", "timestamp", now)
		},
		func() dataSourceDTO {
			return h.postgresSource(ctx, tenantID, "threat_intel", "threat_intel", "威胁情报", "threat_intel", "updated_at", now)
		},
		func() dataSourceDTO {
			return h.postgresAssetVulnerabilitySource(ctx, tenantID, now)
		},
	}
	sources := make([]dataSourceDTO, len(loaders))
	var wg sync.WaitGroup
	wg.Add(len(loaders))
	for index, load := range loaders {
		go func(index int, load func() dataSourceDTO) { defer wg.Done(); sources[index] = load() }(index, load)
	}
	wg.Wait()
	return sources
}

func (h *SystemHandler) clickHouseSource(ctx context.Context, tenantID, sourceID, sourceType, name, table, timeColumn string, createdAt int64) dataSourceDTO {
	if h.chClient == nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "clickhouse."+table, "SOURCE_NOT_CONFIGURED", createdAt)
	}
	var total, recent uint64
	var latest int64
	query := fmt.Sprintf("SELECT count(), countIf(%s >= ?), max(%s) FROM %s WHERE tenant_id=?", timeColumn, timeColumn, table)
	row, err := h.chClient.QueryRow(ctx, query, time.Now().Add(-time.Hour).UnixMilli(), tenantID)
	if err != nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "clickhouse."+table, "SOURCE_QUERY_FAILED", createdAt)
	}
	if err := row.Scan(&total, &recent, &latest); err != nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "clickhouse."+table, "SOURCE_QUERY_FAILED", createdAt)
	}
	source := sourceFromCounts(tenantID, sourceID, sourceType, name, int64(total), int64(recent), latest, createdAt)
	source.Config["storage"] = "clickhouse." + table
	source.RecentTrend = h.clickHouseInt64Trend(ctx, tenantID, table, timeColumn)
	return source
}

func (h *SystemHandler) clickHouseDateTimeSource(ctx context.Context, tenantID, sourceID, sourceType, name, table, timeColumn string, createdAt int64) dataSourceDTO {
	if h.chClient == nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "clickhouse."+table, "SOURCE_NOT_CONFIGURED", createdAt)
	}
	var total, recent uint64
	var latest time.Time
	query := fmt.Sprintf("SELECT count(), countIf(%s >= ?), max(%s) FROM %s WHERE tenant_id=?", timeColumn, timeColumn, table)
	row, err := h.chClient.QueryRow(ctx, query, time.Now().Add(-time.Hour), tenantID)
	if err != nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "clickhouse."+table, "SOURCE_QUERY_FAILED", createdAt)
	}
	if err := row.Scan(&total, &recent, &latest); err != nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "clickhouse."+table, "SOURCE_QUERY_FAILED", createdAt)
	}
	latestMs := int64(0)
	if !latest.IsZero() {
		latestMs = latest.UnixMilli()
	}
	source := sourceFromCounts(tenantID, sourceID, sourceType, name, int64(total), int64(recent), latestMs, createdAt)
	source.Config["storage"] = "clickhouse." + table
	source.RecentTrend = h.clickHouseDateTimeTrend(ctx, tenantID, table, timeColumn)
	return source
}

func (h *SystemHandler) postgresSource(ctx context.Context, tenantID, sourceID, sourceType, name, table, timeColumn string, createdAt int64) dataSourceDTO {
	var total, recent int64
	var latest sql.NullTime
	if h.pgDB == nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "postgres."+table, "SOURCE_NOT_CONFIGURED", createdAt)
	}
	query := fmt.Sprintf("SELECT count(*), count(*) FILTER (WHERE %s >= now() - interval '1 hour'), max(%s) FROM %s WHERE tenant_id=$1", timeColumn, timeColumn, table)
	if err := h.pgDB.QueryRowContext(ctx, query, tenantID).Scan(&total, &recent, &latest); err != nil {
		return unavailableFusionSource(tenantID, sourceID, sourceType, name, "postgres."+table, "SOURCE_QUERY_FAILED", createdAt)
	}
	latestMs := int64(0)
	if latest.Valid {
		latestMs = latest.Time.UnixMilli()
	}
	source := sourceFromCounts(tenantID, sourceID, sourceType, name, total, recent, latestMs, createdAt)
	source.RecentTrend = h.postgresTimestampTrend(ctx, tenantID, table, timeColumn)
	source.Config["storage"] = "postgres." + table
	return source
}

func (h *SystemHandler) postgresAssetVulnerabilitySource(ctx context.Context, tenantID string, createdAt int64) dataSourceDTO {
	const vulnerabilityItems = `CASE WHEN jsonb_typeof(metadata->'vulnerabilities')='array' THEN metadata->'vulnerabilities' ELSE '[]'::jsonb END`
	var total, recent int64
	var latest sql.NullTime
	if h.pgDB == nil {
		return unavailableFusionSource(tenantID, "vulnerability", "vulnerability", "漏洞信息", "postgres.assets.metadata.vulnerabilities", "SOURCE_NOT_CONFIGURED", createdAt)
	}
	query := fmt.Sprintf(`SELECT
		COALESCE(SUM(jsonb_array_length(%s)), 0),
		COALESCE(SUM(CASE WHEN updated_at >= now() - interval '1 hour' THEN jsonb_array_length(%s) ELSE 0 END), 0),
		MAX(updated_at) FILTER (WHERE jsonb_array_length(%s) > 0)
		FROM assets WHERE tenant_id=$1`, vulnerabilityItems, vulnerabilityItems, vulnerabilityItems)
	if err := h.pgDB.QueryRowContext(ctx, query, tenantID).Scan(&total, &recent, &latest); err != nil {
		return unavailableFusionSource(tenantID, "vulnerability", "vulnerability", "漏洞信息", "postgres.assets.metadata.vulnerabilities", "SOURCE_QUERY_FAILED", createdAt)
	}
	latestMillis := int64(0)
	if latest.Valid {
		latestMillis = latest.Time.UnixMilli()
	}
	source := sourceFromCounts(tenantID, "vulnerability", "vulnerability", "漏洞信息", total, recent, latestMillis, createdAt)
	source.Config["storage"] = "postgres.assets.metadata.vulnerabilities"
	source.RecentTrend = h.postgresAssetVulnerabilityTrend(ctx, tenantID)
	return source
}

const fusionTrendBucketMillis int64 = 10 * 60 * 1000
const fusionTrendBucketCount = 8

func emptyFusionTrend() []int64 {
	return make([]int64, fusionTrendBucketCount)
}

func fillFusionTrend(points map[int64]int64, nowMillis int64) []int64 {
	trend := emptyFusionTrend()
	endBucket := nowMillis / fusionTrendBucketMillis * fusionTrendBucketMillis
	startBucket := endBucket - int64(fusionTrendBucketCount-1)*fusionTrendBucketMillis
	for index := range trend {
		trend[index] = points[startBucket+int64(index)*fusionTrendBucketMillis]
	}
	return trend
}

func (h *SystemHandler) clickHouseInt64Trend(ctx context.Context, tenantID, table, timeColumn string) []int64 {
	nowMillis := time.Now().UnixMilli()
	query := fmt.Sprintf(`SELECT intDiv(%s, %d) * %d AS bucket, count()
		FROM %s WHERE tenant_id=? AND %s>=? GROUP BY bucket ORDER BY bucket`,
		timeColumn, fusionTrendBucketMillis, fusionTrendBucketMillis, table, timeColumn)
	rows, err := h.chClient.Query(ctx, query, tenantID, nowMillis-int64(fusionTrendBucketCount)*fusionTrendBucketMillis)
	if err != nil {
		return emptyFusionTrend()
	}
	defer rows.Close()
	points := make(map[int64]int64, fusionTrendBucketCount)
	for rows.Next() {
		var bucket int64
		var count uint64
		if err := rows.Scan(&bucket, &count); err != nil {
			return emptyFusionTrend()
		}
		points[bucket] = int64(count)
	}
	return fillFusionTrend(points, nowMillis)
}

func (h *SystemHandler) clickHouseDateTimeTrend(ctx context.Context, tenantID, table, timeColumn string) []int64 {
	now := time.Now()
	query := fmt.Sprintf(`SELECT toUnixTimestamp(toStartOfInterval(%s, INTERVAL 10 MINUTE)) * 1000 AS bucket, count()
		FROM %s WHERE tenant_id=? AND %s>=? GROUP BY bucket ORDER BY bucket`, timeColumn, table, timeColumn)
	rows, err := h.chClient.Query(ctx, query, tenantID, now.Add(-time.Duration(fusionTrendBucketCount)*10*time.Minute))
	if err != nil {
		return emptyFusionTrend()
	}
	defer rows.Close()
	points := make(map[int64]int64, fusionTrendBucketCount)
	for rows.Next() {
		var bucket int64
		var count uint64
		if err := rows.Scan(&bucket, &count); err != nil {
			return emptyFusionTrend()
		}
		points[bucket] = int64(count)
	}
	return fillFusionTrend(points, now.UnixMilli())
}

func (h *SystemHandler) postgresTimestampTrend(ctx context.Context, tenantID, table, timeColumn string) []int64 {
	now := time.Now()
	query := fmt.Sprintf(`SELECT (floor(extract(epoch FROM %s) / 600)::bigint * 600000) AS bucket, count(*)
		FROM %s WHERE tenant_id=$1 AND %s >= $2 GROUP BY bucket ORDER BY bucket`, timeColumn, table, timeColumn)
	rows, err := h.pgDB.QueryContext(ctx, query, tenantID, now.Add(-time.Duration(fusionTrendBucketCount)*10*time.Minute))
	if err != nil {
		return emptyFusionTrend()
	}
	defer rows.Close()
	points := make(map[int64]int64, fusionTrendBucketCount)
	for rows.Next() {
		var bucket, count int64
		if err := rows.Scan(&bucket, &count); err != nil {
			return emptyFusionTrend()
		}
		points[bucket] = count
	}
	return fillFusionTrend(points, now.UnixMilli())
}

func (h *SystemHandler) postgresAssetVulnerabilityTrend(ctx context.Context, tenantID string) []int64 {
	now := time.Now()
	rows, err := h.pgDB.QueryContext(ctx, `SELECT
		(floor(extract(epoch FROM updated_at) / 600)::bigint * 600000) AS bucket,
		SUM(jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'vulnerabilities')='array' THEN metadata->'vulnerabilities' ELSE '[]'::jsonb END))
		FROM assets
		WHERE tenant_id=$1 AND updated_at >= $2
		  AND jsonb_array_length(CASE WHEN jsonb_typeof(metadata->'vulnerabilities')='array' THEN metadata->'vulnerabilities' ELSE '[]'::jsonb END) > 0
		GROUP BY bucket ORDER BY bucket`, tenantID, now.Add(-time.Duration(fusionTrendBucketCount)*10*time.Minute))
	if err != nil {
		return emptyFusionTrend()
	}
	defer rows.Close()
	points := make(map[int64]int64, fusionTrendBucketCount)
	for rows.Next() {
		var bucket, count int64
		if err := rows.Scan(&bucket, &count); err != nil {
			return emptyFusionTrend()
		}
		points[bucket] = count
	}
	return fillFusionTrend(points, now.UnixMilli())
}

func sourceFromCounts(tenantID, sourceID, sourceType, name string, total, recent, latest, createdAt int64) dataSourceDTO {
	status := "inactive"
	if latest > 0 && time.Since(time.UnixMilli(latest)) <= 24*time.Hour {
		status = "active"
	}
	return dataSourceDTO{
		SourceID: sourceID, TenantID: tenantID, Name: name, SourceType: sourceType, Status: status,
		LastIngestAt: latest, RecordsPerMinute: float64(recent) / 60.0, ErrorRate: nil,
		RecentTrend: emptyFusionTrend(), Config: map[string]interface{}{"total_records": total}, CreatedAt: createdAt,
	}
}

func unavailableFusionSource(tenantID, sourceID, sourceType, name, storage, errorCode string, createdAt int64) dataSourceDTO {
	source := sourceFromCounts(tenantID, sourceID, sourceType, name, 0, 0, 0, createdAt)
	source.Status = "unavailable"
	source.Config["storage"] = storage
	source.Config["error_code"] = errorCode
	return source
}

func (h *SystemHandler) countAlignedEntities(ctx context.Context, tenantID string) int64 {
	var count int64
	if h.pgDB == nil {
		return 0
	}
	_ = h.pgDB.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE tenant_id=$1", tenantID).Scan(&count)
	return count
}

func (h *SystemHandler) countDistinctFlowIPs(ctx context.Context, tenantID string) int64 {
	if h.chClient == nil {
		return 0
	}
	var count uint64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT uniqExact(ip) FROM
		(SELECT src_ip AS ip FROM traffic.sessions WHERE tenant_id=?
		 UNION ALL SELECT dst_ip AS ip FROM traffic.sessions WHERE tenant_id=?)`, tenantID, tenantID)
	if err == nil {
		_ = row.Scan(&count)
	}
	return int64(count)
}

func (h *SystemHandler) queryTunnelProtocols(ctx context.Context, tenantID string, start, end int64) ([]encryptedTunnelProtocolDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT tunnel_protocol, count(), sum(bytes_total)
		FROM (
			SELECT multiIf(
				dns_pkt_cnt >= 20, 'DNS_HIGH_FREQUENCY',
				dst_port = 22 AND duration_ms >= 600000, 'SSH_LONG_LIVED',
				protocol = 17 AND dst_port IN (443, 8443) AND duration_ms >= 600000, 'QUIC_LONG_LIVED',
				dst_port IN (443, 8443, 853, 993, 995, 465) AND duration_ms >= 600000 AND bytes_total >= 104857600, 'TLS_LARGE_LONG_LIVED',
				'OTHER'
			) AS tunnel_protocol, bytes_total
			FROM traffic.sessions
			WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
			  AND (dns_pkt_cnt >= 20
			       OR (dst_port = 22 AND duration_ms >= 600000)
			       OR (protocol = 17 AND dst_port IN (443, 8443) AND duration_ms >= 600000)
			       OR (dst_port IN (443, 8443, 853, 993, 995, 465) AND duration_ms >= 600000 AND bytes_total >= 104857600))
		)
		WHERE tunnel_protocol != 'OTHER'
		GROUP BY tunnel_protocol
		ORDER BY count() DESC`, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	protocols := make([]encryptedTunnelProtocolDTO, 0)
	for rows.Next() {
		var item encryptedTunnelProtocolDTO
		var count uint64
		if err := rows.Scan(&item.Protocol, &count, &item.TotalBytes); err != nil {
			return nil, err
		}
		item.Count = int64(count)
		protocols = append(protocols, item)
	}
	return protocols, rows.Err()
}

func (h *SystemHandler) queryTunnelUsers(ctx context.Context, tenantID string, start, end int64, limit int) ([]encryptedTunnelUserDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT src_ip, tunnel_protocol, count(), sum(bytes_total), max(ts_end)
		FROM (
			SELECT src_ip, ts_end, bytes_total,
			       multiIf(
			           dns_pkt_cnt >= 20, 'DNS_HIGH_FREQUENCY',
			           dst_port = 22 AND duration_ms >= 600000, 'SSH_LONG_LIVED',
			           protocol = 17 AND dst_port IN (443, 8443) AND duration_ms >= 600000, 'QUIC_LONG_LIVED',
			           dst_port IN (443, 8443, 853, 993, 995, 465) AND duration_ms >= 600000 AND bytes_total >= 104857600, 'TLS_LARGE_LONG_LIVED',
			           'OTHER'
			       ) AS tunnel_protocol
			FROM traffic.sessions
			WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
			  AND (dns_pkt_cnt >= 20
			       OR (dst_port = 22 AND duration_ms >= 600000)
			       OR (protocol = 17 AND dst_port IN (443, 8443) AND duration_ms >= 600000)
			       OR (dst_port IN (443, 8443, 853, 993, 995, 465) AND duration_ms >= 600000 AND bytes_total >= 104857600))
		)
		WHERE tunnel_protocol != 'OTHER'
		GROUP BY src_ip, tunnel_protocol
		ORDER BY count() DESC, sum(bytes_total) DESC
		LIMIT ?`, tenantID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]encryptedTunnelUserDTO, 0, limit)
	for rows.Next() {
		var item encryptedTunnelUserDTO
		var count uint64
		if err := rows.Scan(&item.IP, &item.Protocol, &count, &item.TotalBytes, &item.LastSeen); err != nil {
			return nil, err
		}
		item.Count = int64(count)
		item.Risk = tunnelRisk(item.Protocol, item.Count, item.TotalBytes)
		users = append(users, item)
	}
	return users, rows.Err()
}

func (h *SystemHandler) queryExfiltrationSources(ctx context.Context, tenantID string, start, end int64, limit int) ([]encryptedExfiltrationSourceDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT src_ip, count(), sum(bytes_fwd), sum(bytes_total), uniqExact(dst_ip), max(ts_end)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (22, 443, 8443, 853, 993, 995, 465)
		  AND bytes_fwd > 0
		  AND NOT (
			startsWith(dst_ip, '10.') OR startsWith(dst_ip, '192.168.')
			OR match(dst_ip, '^172\\.(1[6-9]|2[0-9]|3[01])\\.')
			OR startsWith(dst_ip, '127.') OR startsWith(dst_ip, '169.254.')
			OR dst_ip IN ('', '0.0.0.0', '::1')
			OR startsWith(lower(dst_ip), 'fc') OR startsWith(lower(dst_ip), 'fd')
			OR startsWith(lower(dst_ip), 'fe80:')
		  )
		GROUP BY src_ip
		ORDER BY sum(bytes_fwd) DESC
		LIMIT ?`, tenantID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make([]encryptedExfiltrationSourceDTO, 0, limit)
	for rows.Next() {
		var item encryptedExfiltrationSourceDTO
		var sessionCount, dstCount uint64
		if err := rows.Scan(&item.SrcIP, &sessionCount, &item.UploadBytes, &item.TotalBytes, &dstCount, &item.LastSeen); err != nil {
			return nil, err
		}
		item.SessionCount = int64(sessionCount)
		item.DstCount = int64(dstCount)
		item.Risk = exfiltrationRisk(item.UploadBytes, item.SessionCount)
		sources = append(sources, item)
	}
	return sources, rows.Err()
}

func (h *SystemHandler) queryExfiltrationDestinations(ctx context.Context, tenantID string, start, end int64, limit int) ([]encryptedExfiltrationDestinationDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT dst_ip, count(), sum(bytes_fwd), sum(bytes_total), uniqExact(src_ip), max(ts_end)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (22, 443, 8443, 853, 993, 995, 465)
		  AND bytes_fwd > 0
		  AND NOT (
			startsWith(dst_ip, '10.') OR startsWith(dst_ip, '192.168.')
			OR match(dst_ip, '^172\\.(1[6-9]|2[0-9]|3[01])\\.')
			OR startsWith(dst_ip, '127.') OR startsWith(dst_ip, '169.254.')
			OR dst_ip IN ('', '0.0.0.0', '::1')
			OR startsWith(lower(dst_ip), 'fc') OR startsWith(lower(dst_ip), 'fd')
			OR startsWith(lower(dst_ip), 'fe80:')
		  )
		GROUP BY dst_ip
		ORDER BY sum(bytes_fwd) DESC
		LIMIT ?`, tenantID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	destinations := make([]encryptedExfiltrationDestinationDTO, 0, limit)
	for rows.Next() {
		var item encryptedExfiltrationDestinationDTO
		var sessionCount, srcCount uint64
		if err := rows.Scan(&item.DstIP, &sessionCount, &item.UploadBytes, &item.TotalBytes, &srcCount, &item.LastSeen); err != nil {
			return nil, err
		}
		item.SessionCount = int64(sessionCount)
		item.SrcCount = int64(srcCount)
		item.Risk = exfiltrationRisk(item.UploadBytes, item.SessionCount)
		destinations = append(destinations, item)
	}
	return destinations, rows.Err()
}

func (h *SystemHandler) queryExfiltrationTrend(ctx context.Context, tenantID string, start, end int64) ([]encryptedExfiltrationTrendDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT intDiv(ts_start, 7200000) * 7200000 AS bucket_start,
		       uniqExact(dst_ip),
		       countIf(bytes_fwd >= ?),
		       countIf(duration_ms >= ?),
		       countIf(dst_port IN (22, 8443, 853, 993, 995, 465)),
		       count()
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (22, 443, 8443, 853, 993, 995, 465)
		  AND NOT (
			startsWith(dst_ip, '10.') OR startsWith(dst_ip, '192.168.')
			OR match(dst_ip, '^172\\.(1[6-9]|2[0-9]|3[01])\\.')
			OR startsWith(dst_ip, '127.') OR startsWith(dst_ip, '169.254.')
			OR dst_ip IN ('', '0.0.0.0', '::1')
			OR startsWith(lower(dst_ip), 'fc') OR startsWith(lower(dst_ip), 'fd')
			OR startsWith(lower(dst_ip), 'fe80:')
		  )
		GROUP BY bucket_start
		ORDER BY bucket_start
		LIMIT 24`, uint64(100*1024*1024), uint32(3600*1000), tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trend := make([]encryptedExfiltrationTrendDTO, 0, 24)
	for rows.Next() {
		var item encryptedExfiltrationTrendDTO
		var destinationCount, largeUpload, longLived, nonStandard, encrypted uint64
		if err := rows.Scan(&item.BucketStart, &destinationCount, &largeUpload, &longLived, &nonStandard, &encrypted); err != nil {
			return nil, err
		}
		item.DestinationCount = int64(destinationCount)
		item.LargeUploadSessions = int64(largeUpload)
		item.LongLivedSessions = int64(longLived)
		item.NonStandardPortSessions = int64(nonStandard)
		item.EncryptedSessions = int64(encrypted)
		trend = append(trend, item)
	}
	return trend, rows.Err()
}

func (h *SystemHandler) queryExfiltrationRisks(ctx context.Context, tenantID string, start, end int64) ([]encryptedExfiltrationRiskDTO, error) {
	var largeCount, longCount, nonStandardCount uint64
	var largeBytes, longBytes, nonStandardBytes uint64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT
			countIf(bytes_fwd >= ?),
			sumIf(bytes_fwd, bytes_fwd >= ?),
			countIf(duration_ms >= ?),
			sumIf(bytes_fwd, duration_ms >= ?),
			countIf(dst_port IN (22, 8443, 853, 993, 995, 465)),
			sumIf(bytes_fwd, dst_port IN (22, 8443, 853, 993, 995, 465))
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (22, 443, 8443, 853, 993, 995, 465)
		  AND NOT (
			startsWith(dst_ip, '10.') OR startsWith(dst_ip, '192.168.')
			OR match(dst_ip, '^172\\.(1[6-9]|2[0-9]|3[01])\\.')
			OR startsWith(dst_ip, '127.') OR startsWith(dst_ip, '169.254.')
			OR dst_ip IN ('', '0.0.0.0', '::1')
			OR startsWith(lower(dst_ip), 'fc') OR startsWith(lower(dst_ip), 'fd')
			OR startsWith(lower(dst_ip), 'fe80:')
		  )`,
		uint64(100*1024*1024), uint64(100*1024*1024), uint32(3600*1000), uint32(3600*1000), tenantID, start, end)
	if err != nil {
		return nil, err
	}
	if err := row.Scan(&largeCount, &largeBytes, &longCount, &longBytes, &nonStandardCount, &nonStandardBytes); err != nil {
		return nil, err
	}
	return []encryptedExfiltrationRiskDTO{
		{Type: "large_encrypted_upload", Count: int64(largeCount), Severity: riskSeverity(largeCount, "high"), TotalBytes: largeBytes},
		{Type: "long_lived_encrypted_session", Count: int64(longCount), Severity: riskSeverity(longCount, "medium"), TotalBytes: longBytes},
		{Type: "non_standard_encrypted_port", Count: int64(nonStandardCount), Severity: riskSeverity(nonStandardCount, "medium"), TotalBytes: nonStandardBytes},
	}, nil
}

func (h *SystemHandler) queryExfiltrationPaths(ctx context.Context, tenantID string, start, end int64, limit int) ([]encryptedExfiltrationPathDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT src_ip, dst_ip, count(), sum(bytes_fwd), max(ts_end)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=?
		  AND dst_port IN (22, 443, 8443, 853, 993, 995, 465)
		  AND bytes_fwd > 0
		  AND NOT (
			startsWith(dst_ip, '10.') OR startsWith(dst_ip, '192.168.')
			OR match(dst_ip, '^172\\.(1[6-9]|2[0-9]|3[01])\\.')
			OR startsWith(dst_ip, '127.') OR startsWith(dst_ip, '169.254.')
			OR dst_ip IN ('', '0.0.0.0', '::1')
			OR startsWith(lower(dst_ip), 'fc') OR startsWith(lower(dst_ip), 'fd')
			OR startsWith(lower(dst_ip), 'fe80:')
		  )
		GROUP BY src_ip, dst_ip
		ORDER BY sum(bytes_fwd) DESC
		LIMIT ?`, tenantID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make([]encryptedExfiltrationPathDTO, 0, limit)
	for rows.Next() {
		var item encryptedExfiltrationPathDTO
		var sessionCount uint64
		if err := rows.Scan(&item.SrcIP, &item.DstIP, &sessionCount, &item.UploadBytes, &item.LastSeen); err != nil {
			return nil, err
		}
		item.SessionCount = int64(sessionCount)
		item.Risk = exfiltrationRisk(item.UploadBytes, item.SessionCount)
		paths = append(paths, item)
	}
	return paths, rows.Err()
}

func (h *SystemHandler) estimateFlowDuplication(ctx context.Context, tenantID string) float64 {
	if h.chClient == nil {
		return 0
	}
	var total, distinct uint64
	row, err := h.chClient.QueryRow(ctx, `SELECT count(), uniqExact(session_id) FROM traffic.sessions WHERE tenant_id=? AND ts_start>=?`, tenantID, time.Now().Add(-24*time.Hour).UnixMilli())
	if err != nil || row.Scan(&total, &distinct) != nil || total == 0 || distinct >= total {
		return 0
	}
	return clamp01(float64(total-distinct) / float64(total))
}

func (h *SystemHandler) ipRiskScore(ctx context.Context, tenantID, ip string) int64 {
	if h.chClient == nil {
		return 0
	}
	var alerts uint64
	row, err := h.chClient.QueryRow(ctx, `SELECT count() FROM traffic.alerts WHERE tenant_id=? AND (src_ip=? OR dst_ip=?) AND last_seen>=?`, tenantID, ip, ip, time.Now().Add(-7*24*time.Hour).UnixMilli())
	if err != nil || row.Scan(&alerts) != nil {
		return 0
	}
	score := int64(alerts) * 10
	if score > 100 {
		score = 100
	}
	return score
}

func (h *SystemHandler) queryBehaviorBaselineSummary(ctx context.Context, tenantID, baselineType string, windowDays int) (behaviorBaselineSummaryDTO, error) {
	summary := behaviorBaselineSummaryDTO{Scope: "all_entities_in_window"}
	startMillis := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	items, err := h.queryBehaviorBaselineSummaryItems(ctx, tenantID, baselineType, startMillis)
	if err != nil {
		return summary, err
	}
	type summarySetting struct {
		warning, alert float64
		frozen, drift  bool
	}
	settings := map[string]summarySetting{}
	resetAt := map[string]int64{}
	rebuild := map[string]bool{}
	if h.pgDB != nil {
		prefix := baselineType + ":%"
		rows, queryErr := h.pgDB.QueryContext(ctx, `SELECT baseline_id, warning_multiplier, alert_multiplier, frozen, drift_watch FROM behavior_baseline_settings WHERE tenant_id=$1 AND baseline_id LIKE $2`, tenantID, prefix)
		if queryErr != nil {
			return summary, queryErr
		}
		for rows.Next() {
			var id string
			var value summarySetting
			if scanErr := rows.Scan(&id, &value.warning, &value.alert, &value.frozen, &value.drift); scanErr != nil {
				rows.Close()
				return summary, scanErr
			}
			settings[id] = value
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return summary, rowsErr
		}
		rows.Close()

		rows, queryErr = h.pgDB.QueryContext(ctx, `SELECT baseline_id, reset_at FROM behavior_baseline_resets WHERE tenant_id=$1 AND baseline_id LIKE $2 AND reset_at >= $3`, tenantID, prefix, time.UnixMilli(startMillis))
		if queryErr != nil {
			return summary, queryErr
		}
		for rows.Next() {
			var id string
			var at time.Time
			if scanErr := rows.Scan(&id, &at); scanErr != nil {
				rows.Close()
				return summary, scanErr
			}
			resetAt[id] = at.UnixMilli()
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return summary, rowsErr
		}
		rows.Close()

		rows, queryErr = h.pgDB.QueryContext(ctx, `
			SELECT DISTINCT a.baseline_id FROM behavior_baseline_actions a
			JOIN behavior_baseline_outbox o ON o.action_id=a.action_id
			WHERE a.tenant_id=$1 AND a.baseline_id LIKE $2 AND a.action_type='rebuild' AND o.published=false`, tenantID, prefix)
		if queryErr != nil {
			return summary, queryErr
		}
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr != nil {
				rows.Close()
				return summary, scanErr
			}
			rebuild[id] = true
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return summary, rowsErr
		}
		rows.Close()
	}

	for id, item := range items {
		if at, ok := resetAt[id]; ok && at > startMillis {
			if refreshed, refreshErr := h.queryBehaviorBaselineByEntityFromStart(ctx, tenantID, item.BaselineType, item.EntityID, at); refreshErr == nil {
				item = refreshed
			} else {
				item = zeroBehaviorBaselineDTO(tenantID, item.BaselineType, item.EntityID, at)
			}
		}
		setting, hasSetting := settings[id]
		if hasSetting {
			for index := range item.Metrics {
				item.Metrics[index].ThresholdConfig.WarningMultiplier = setting.warning
				item.Metrics[index].ThresholdConfig.AlertMultiplier = setting.alert
			}
		}
		switch {
		case hasSetting && setting.frozen:
			summary.Frozen++
		case rebuild[id]:
			summary.Rebuild++
		case hasSetting && setting.drift:
			summary.Drift++
		case item.Status == "learning":
			summary.Learning++
		default:
			summary.Active++
		}
		for _, metric := range item.Metrics {
			if metric.ThresholdConfig.AlertMultiplier > 0 && metric.DeviationScore >= metric.ThresholdConfig.AlertMultiplier {
				summary.Alerts++
				break
			}
		}
	}
	summary.Total = len(items)
	return summary, nil
}

func (h *SystemHandler) queryBehaviorBaselineSummaryItems(ctx context.Context, tenantID, baselineType string, startMillis int64) (map[string]behaviorBaselineDTO, error) {
	items := map[string]behaviorBaselineDTO{}
	if baselineType == "account" {
		rows, err := h.chClient.Query(ctx, `
			SELECT username, count(), toInt64(toUnixTimestamp(max(timestamp))) * 1000, uniqExact(source_ip), uniqExact(resource), uniqExact(event_type)
			FROM traffic.user_events WHERE tenant_id=? AND timestamp>=? AND username!='' GROUP BY username`, tenantID, time.UnixMilli(startMillis))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var username string
			var samples, sources, resources, eventTypes uint64
			var updated int64
			if err := rows.Scan(&username, &samples, &updated, &sources, &resources, &eventTypes); err != nil {
				return nil, err
			}
			status := "learning"
			if samples >= 30 {
				status = "active"
			}
			dto := behaviorBaselineDTO{BaselineID: baselineID("account", username), TenantID: tenantID, Name: baselineDisplayName("account", username), EntityType: "account", EntityID: username, BaselineType: "account", Status: status, CreatedAt: startMillis, UpdatedAt: updated, Version: 1, Metrics: []behaviorMetricDTO{metricDTO("events_per_window", "events", float64(samples), 0, float64(samples)), metricDTO("source_ip_count", "addresses", float64(sources), 0, float64(sources)), metricDTO("resource_count", "resources", float64(resources), float64(eventTypes), float64(resources))}}
			items[dto.BaselineID] = dto
		}
		return items, rows.Err()
	}
	dimensions := map[string]string{"asset": "src_ip", "port": "toString(dst_port)", "protocol": "toString(protocol)", "time": "toString(toHour(toDateTime(intDiv(ts_start, 1000))))"}
	dimension, ok := dimensions[baselineType]
	if !ok {
		return nil, fmt.Errorf("unsupported baseline type: %s", baselineType)
	}
	query := fmt.Sprintf(`
		SELECT toString(%s), count(), max(ts_end),
		       avg(toFloat64(bytes_total)), stddevPop(toFloat64(bytes_total)), argMax(toFloat64(bytes_total), ts_end),
		       avg(toFloat64(num_pkts)), stddevPop(toFloat64(num_pkts)), argMax(toFloat64(num_pkts), ts_end),
		       avg(toFloat64(duration_ms)), stddevPop(toFloat64(duration_ms)), argMax(toFloat64(duration_ms), ts_end)
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? GROUP BY %s`, dimension, dimension)
	rows, err := h.chClient.Query(ctx, query, tenantID, startMillis)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entityID string
		var samples uint64
		var updated int64
		var bytesMean, bytesStd, bytesCurrent, packetsMean, packetsStd, packetsCurrent, durationMean, durationStd, durationCurrent float64
		if err := rows.Scan(&entityID, &samples, &updated, &bytesMean, &bytesStd, &bytesCurrent, &packetsMean, &packetsStd, &packetsCurrent, &durationMean, &durationStd, &durationCurrent); err != nil {
			return nil, err
		}
		status := "learning"
		if samples >= 30 {
			status = "active"
		}
		dto := behaviorBaselineDTO{BaselineID: baselineID(baselineType, entityID), TenantID: tenantID, Name: baselineDisplayName(baselineType, entityID), EntityType: baselineType, EntityID: entityID, BaselineType: baselineType, Status: status, CreatedAt: startMillis, UpdatedAt: updated, Version: 1, Metrics: []behaviorMetricDTO{metricDTO("bytes_per_session", "bytes", bytesMean, bytesStd, bytesCurrent), metricDTO("packets_per_session", "packets", packetsMean, packetsStd, packetsCurrent), metricDTO("duration_ms", "ms", durationMean, durationStd, durationCurrent)}}
		items[dto.BaselineID] = dto
	}
	return items, rows.Err()
}

func (h *SystemHandler) queryBehaviorBaselines(ctx context.Context, tenantID, baselineType string, limit, offset, windowDays int) ([]behaviorBaselineDTO, int, error) {
	if h.chClient == nil {
		return nil, 0, fmt.Errorf("clickhouse is not configured")
	}
	if baselineType == "account" {
		return h.queryAccountBehaviorBaselines(ctx, tenantID, limit, offset, windowDays)
	}
	dimensions := map[string]string{
		"asset":    "src_ip",
		"port":     "toString(dst_port)",
		"protocol": "toString(protocol)",
		"time":     "toString(toHour(toDateTime(intDiv(ts_start, 1000))))",
	}
	dimension, ok := dimensions[baselineType]
	if !ok {
		return nil, 0, fmt.Errorf("unsupported baseline type: %s", baselineType)
	}
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	var total uint64
	countQuery := fmt.Sprintf("SELECT count() FROM (SELECT %s FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? GROUP BY %s)", dimension, dimension)
	row, err := h.chClient.QueryRow(ctx, countQuery, tenantID, start)
	if err != nil {
		return nil, 0, fmt.Errorf("count behavior baselines: %w", err)
	}
	if err = row.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("scan behavior baseline count: %w", err)
	}
	query := fmt.Sprintf(`
		SELECT toString(%s), count(), max(ts_end),
		       avg(toFloat64(bytes_total)), stddevPop(toFloat64(bytes_total)), argMax(toFloat64(bytes_total), ts_end),
		       avg(toFloat64(num_pkts)), stddevPop(toFloat64(num_pkts)), argMax(toFloat64(num_pkts), ts_end),
		       avg(toFloat64(duration_ms)), stddevPop(toFloat64(duration_ms)), argMax(toFloat64(duration_ms), ts_end)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=?
		GROUP BY %s
		ORDER BY max(ts_end) DESC
		LIMIT ? OFFSET ?`, dimension, dimension)
	rows, err := h.chClient.Query(ctx, query, tenantID, start, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	baselines := make([]behaviorBaselineDTO, 0)
	for rows.Next() {
		var entityID string
		var samples uint64
		var updated int64
		var bytesMean, bytesStd, bytesCurrent float64
		var packetsMean, packetsStd, packetsCurrent float64
		var durationMean, durationStd, durationCurrent float64
		if err := rows.Scan(&entityID, &samples, &updated, &bytesMean, &bytesStd, &bytesCurrent, &packetsMean, &packetsStd, &packetsCurrent, &durationMean, &durationStd, &durationCurrent); err != nil {
			return nil, 0, err
		}
		status := "learning"
		if samples >= 30 {
			status = "active"
		}
		dto := behaviorBaselineDTO{
			BaselineID: baselineID(baselineType, entityID), TenantID: tenantID, Name: baselineDisplayName(baselineType, entityID),
			EntityType: baselineType, EntityID: entityID, BaselineType: baselineType,
			Metrics: []behaviorMetricDTO{
				metricDTO("bytes_per_session", "bytes", bytesMean, bytesStd, bytesCurrent),
				metricDTO("packets_per_session", "packets", packetsMean, packetsStd, packetsCurrent),
				metricDTO("duration_ms", "ms", durationMean, durationStd, durationCurrent),
			},
			Status: status, CreatedAt: start, UpdatedAt: updated, Version: 1,
		}
		h.applyBehaviorBaselineSettings(ctx, tenantID, &dto)
		baselines = append(baselines, dto)
	}
	return baselines, int(total), rows.Err()
}

func (h *SystemHandler) queryAccountBehaviorBaselines(ctx context.Context, tenantID string, limit, offset, windowDays int) ([]behaviorBaselineDTO, int, error) {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
	var total uint64
	row, err := h.chClient.QueryRow(ctx, `SELECT uniqExact(username) FROM traffic.user_events WHERE tenant_id=? AND timestamp>=? AND username!=''`, tenantID, start)
	if err != nil {
		return nil, 0, fmt.Errorf("count account baselines: %w", err)
	}
	if err = row.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("scan account baseline count: %w", err)
	}
	rows, err := h.chClient.Query(ctx, `
		SELECT username, count(), toInt64(toUnixTimestamp(max(timestamp))) * 1000,
		       uniqExact(source_ip), uniqExact(resource), uniqExact(event_type),
		       avg(toFloat64(toHour(timestamp))), stddevPop(toFloat64(toHour(timestamp))), argMax(toFloat64(toHour(timestamp)), timestamp)
		FROM traffic.user_events
		WHERE tenant_id=? AND timestamp>=? AND username!=''
		GROUP BY username ORDER BY max(timestamp) DESC LIMIT ? OFFSET ?`, tenantID, start, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]behaviorBaselineDTO, 0)
	for rows.Next() {
		var username string
		var samples, sources, resources, eventTypes uint64
		var loginHourMean, loginHourStd, loginHourCurrent float64
		var updated int64
		if err := rows.Scan(&username, &samples, &updated, &sources, &resources, &eventTypes, &loginHourMean, &loginHourStd, &loginHourCurrent); err != nil {
			return nil, 0, err
		}
		status := "learning"
		if samples >= 30 {
			status = "active"
		}
		dto := behaviorBaselineDTO{
			BaselineID: baselineID("account", username), TenantID: tenantID, Name: baselineDisplayName("account", username),
			EntityType: "account", EntityID: username, BaselineType: "account", Status: status,
			CreatedAt: start.UnixMilli(), UpdatedAt: updated, Version: 1,
			Metrics: []behaviorMetricDTO{
				metricDTO("events_per_window", "events", float64(samples), 0, float64(samples)),
				metricDTO("source_ip_count", "addresses", float64(sources), 0, float64(sources)),
				metricDTO("resource_count", "resources", float64(resources), float64(eventTypes), float64(resources)),
				metricDTO("login_hour", "hour", loginHourMean, loginHourStd, loginHourCurrent),
			},
		}
		h.applyBehaviorBaselineSettings(ctx, tenantID, &dto)
		result = append(result, dto)
	}
	return result, int(total), rows.Err()
}

func baselineDisplayName(baselineType, entityID string) string {
	labels := map[string]string{"asset": "资产", "account": "账号", "port": "端口", "protocol": "协议", "time": "时间段"}
	return labels[baselineType] + "行为基线 " + entityID
}

func (h *SystemHandler) applyBehaviorBaselineSettings(ctx context.Context, tenantID string, dto *behaviorBaselineDTO) {
	if h.pgDB == nil || dto == nil {
		return
	}
	var resetAt time.Time
	if err := h.pgDB.QueryRowContext(ctx, `SELECT reset_at FROM behavior_baseline_resets WHERE tenant_id=$1 AND baseline_id=$2`, tenantID, dto.BaselineID).Scan(&resetAt); err == nil && resetAt.UnixMilli() > dto.CreatedAt {
		if refreshed, refreshErr := h.queryBehaviorBaselineByEntityFromStart(ctx, tenantID, dto.BaselineType, dto.EntityID, resetAt.UnixMilli()); refreshErr == nil {
			dto.Metrics = refreshed.Metrics
			dto.Status = refreshed.Status
			dto.CreatedAt = resetAt.UnixMilli()
			dto.UpdatedAt = refreshed.UpdatedAt
		} else {
			dto.CreatedAt = resetAt.UnixMilli()
			dto.Status = "learning"
			for index := range dto.Metrics {
				dto.Metrics[index].Mean = 0
				dto.Metrics[index].StdDev = 0
				dto.Metrics[index].CurrentValue = 0
				dto.Metrics[index].DeviationScore = 0
				dto.Metrics[index].NormalRange = [2]float64{}
			}
		}
	}
	var warning, alert float64
	if err := h.pgDB.QueryRowContext(ctx, `SELECT warning_multiplier, alert_multiplier, frozen, drift_watch, version FROM behavior_baseline_settings WHERE tenant_id=$1 AND baseline_id=$2`, tenantID, dto.BaselineID).Scan(&warning, &alert, &dto.Frozen, &dto.DriftWatch, &dto.Version); err == nil {
		for index := range dto.Metrics {
			dto.Metrics[index].ThresholdConfig.WarningMultiplier = warning
			dto.Metrics[index].ThresholdConfig.AlertMultiplier = alert
		}
	}
	if dto.Frozen {
		dto.Status = "frozen"
	} else if dto.DriftWatch {
		dto.Status = "drift"
	}
}

func (h *SystemHandler) queryBehaviorBaseline(ctx context.Context, tenantID, id string) (behaviorBaselineDTO, error) {
	return h.queryBehaviorBaselineByID(ctx, tenantID, id, 90)
}

func (h *SystemHandler) queryBehaviorBaselineByID(ctx context.Context, tenantID, id string, windowDays int) (behaviorBaselineDTO, error) {
	entityType, entityID := parseBaselineID(id)
	if entityType == "ip" {
		entityType = "asset"
	}
	if entityID == "" || (entityType != "asset" && entityType != "account" && entityType != "port" && entityType != "protocol" && entityType != "time") {
		return behaviorBaselineDTO{}, fmt.Errorf("baseline not found: %s", id)
	}
	return h.queryBehaviorBaselineByEntity(ctx, tenantID, entityType, entityID, windowDays)
}

func (h *SystemHandler) queryBehaviorBaselineByEntity(ctx context.Context, tenantID, baselineType, entityID string, windowDays int) (behaviorBaselineDTO, error) {
	if h.chClient == nil {
		return behaviorBaselineDTO{}, fmt.Errorf("clickhouse is not configured")
	}
	if windowDays != 7 && windowDays != 30 && windowDays != 90 {
		windowDays = 30
	}
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	originalStart := start
	resetApplied := false
	if h.pgDB != nil {
		var resetAt time.Time
		if err := h.pgDB.QueryRowContext(ctx, `SELECT reset_at FROM behavior_baseline_resets WHERE tenant_id=$1 AND baseline_id=$2`, tenantID, baselineID(baselineType, entityID)).Scan(&resetAt); err == nil && resetAt.UnixMilli() > start {
			start = resetAt.UnixMilli()
			resetApplied = true
		}
	}
	dto, err := h.queryBehaviorBaselineByEntityFromStart(ctx, tenantID, baselineType, entityID, start)
	if err != nil {
		if !resetApplied {
			return behaviorBaselineDTO{}, err
		}
		if _, historicErr := h.queryBehaviorBaselineByEntityFromStart(ctx, tenantID, baselineType, entityID, originalStart); historicErr != nil {
			return behaviorBaselineDTO{}, err
		}
		dto = zeroBehaviorBaselineDTO(tenantID, baselineType, entityID, start)
	}
	h.applyBehaviorBaselineSettings(ctx, tenantID, &dto)
	return dto, nil
}

func zeroBehaviorBaselineDTO(tenantID, baselineType, entityID string, startMillis int64) behaviorBaselineDTO {
	metrics := []behaviorMetricDTO{
		metricDTO("bytes_per_session", "bytes", 0, 0, 0),
		metricDTO("packets_per_session", "packets", 0, 0, 0),
		metricDTO("duration_ms", "ms", 0, 0, 0),
	}
	if baselineType == "account" {
		metrics = []behaviorMetricDTO{
			metricDTO("events_per_window", "events", 0, 0, 0),
			metricDTO("source_ip_count", "addresses", 0, 0, 0),
			metricDTO("resource_count", "resources", 0, 0, 0),
			metricDTO("login_hour", "hour", 0, 0, 0),
		}
	}
	return behaviorBaselineDTO{BaselineID: baselineID(baselineType, entityID), TenantID: tenantID, Name: baselineDisplayName(baselineType, entityID), EntityType: baselineType, EntityID: entityID, BaselineType: baselineType, Metrics: metrics, Status: "learning", CreatedAt: startMillis, UpdatedAt: startMillis, Version: 1}
}

func (h *SystemHandler) queryBehaviorBaselineByEntityFromStart(ctx context.Context, tenantID, baselineType, entityID string, startMillis int64) (behaviorBaselineDTO, error) {
	if baselineType == "account" {
		start := time.UnixMilli(startMillis)
		var samples, sources, resources, eventTypes uint64
		var loginHourMean, loginHourStd, loginHourCurrent float64
		var updated int64
		row, err := h.chClient.QueryRow(ctx, `
			SELECT count(), toInt64(toUnixTimestamp(max(timestamp))) * 1000,
			       uniqExact(source_ip), uniqExact(resource), uniqExact(event_type),
			       avg(toFloat64(toHour(timestamp))), stddevPop(toFloat64(toHour(timestamp))), argMax(toFloat64(toHour(timestamp)), timestamp)
			FROM traffic.user_events WHERE tenant_id=? AND timestamp>=? AND username=? AND username!=''`, tenantID, start, entityID)
		if err != nil {
			return behaviorBaselineDTO{}, err
		}
		if err := row.Scan(&samples, &updated, &sources, &resources, &eventTypes, &loginHourMean, &loginHourStd, &loginHourCurrent); err != nil || samples == 0 {
			return behaviorBaselineDTO{}, fmt.Errorf("baseline not found: %s:%s", baselineType, entityID)
		}
		status := "learning"
		if samples >= 30 {
			status = "active"
		}
		dto := behaviorBaselineDTO{
			BaselineID: baselineID(baselineType, entityID), TenantID: tenantID, Name: baselineDisplayName(baselineType, entityID),
			EntityType: baselineType, EntityID: entityID, BaselineType: baselineType, Status: status,
			CreatedAt: start.UnixMilli(), UpdatedAt: updated, Version: 1,
			Metrics: []behaviorMetricDTO{
				metricDTO("events_per_window", "events", float64(samples), 0, float64(samples)),
				metricDTO("source_ip_count", "addresses", float64(sources), 0, float64(sources)),
				metricDTO("resource_count", "resources", float64(resources), float64(eventTypes), float64(resources)),
				metricDTO("login_hour", "hour", loginHourMean, loginHourStd, loginHourCurrent),
			},
		}
		return dto, nil
	}
	dimensions := map[string]string{
		"asset": "src_ip", "port": "toString(dst_port)", "protocol": "toString(protocol)",
		"time": "toString(toHour(toDateTime(intDiv(ts_start, 1000))))",
	}
	dimension, ok := dimensions[baselineType]
	if !ok {
		return behaviorBaselineDTO{}, fmt.Errorf("baseline not found: %s:%s", baselineType, entityID)
	}
	query := fmt.Sprintf(`
		SELECT count(), max(ts_end),
		       avg(toFloat64(bytes_total)), stddevPop(toFloat64(bytes_total)), argMax(toFloat64(bytes_total), ts_end),
		       avg(toFloat64(num_pkts)), stddevPop(toFloat64(num_pkts)), argMax(toFloat64(num_pkts), ts_end),
		       avg(toFloat64(duration_ms)), stddevPop(toFloat64(duration_ms)), argMax(toFloat64(duration_ms), ts_end)
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? AND toString(%s)=?`, dimension)
	var samples uint64
	var updated int64
	var bytesMean, bytesStd, bytesCurrent, packetsMean, packetsStd, packetsCurrent, durationMean, durationStd, durationCurrent float64
	row, err := h.chClient.QueryRow(ctx, query, tenantID, startMillis, entityID)
	if err != nil {
		return behaviorBaselineDTO{}, err
	}
	if err := row.Scan(&samples, &updated, &bytesMean, &bytesStd, &bytesCurrent, &packetsMean, &packetsStd, &packetsCurrent, &durationMean, &durationStd, &durationCurrent); err != nil || samples == 0 {
		return behaviorBaselineDTO{}, fmt.Errorf("baseline not found: %s:%s", baselineType, entityID)
	}
	status := "learning"
	if samples >= 30 {
		status = "active"
	}
	dto := behaviorBaselineDTO{
		BaselineID: baselineID(baselineType, entityID), TenantID: tenantID, Name: baselineDisplayName(baselineType, entityID),
		EntityType: baselineType, EntityID: entityID, BaselineType: baselineType, Status: status,
		CreatedAt: startMillis, UpdatedAt: updated, Version: 1,
		Metrics: []behaviorMetricDTO{
			metricDTO("bytes_per_session", "bytes", bytesMean, bytesStd, bytesCurrent),
			metricDTO("packets_per_session", "packets", packetsMean, packetsStd, packetsCurrent),
			metricDTO("duration_ms", "ms", durationMean, durationStd, durationCurrent),
		},
	}
	return dto, nil
}

func (h *SystemHandler) queryBehaviorBaselineAnalytics(ctx context.Context, tenantID, id string, windowDays int) (behaviorBaselineAnalyticsDTO, error) {
	baseline, err := h.queryBehaviorBaselineByID(ctx, tenantID, id, windowDays)
	if err != nil {
		return behaviorBaselineAnalyticsDTO{}, err
	}
	result := behaviorBaselineAnalyticsDTO{
		BaselineID:    baseline.BaselineID,
		WindowDays:    windowDays,
		Distributions: make([]behaviorBaselineDistributionDTO, 0),
		Series:        make([]behaviorBaselineSeriesPointDTO, 0),
	}
	if len(baseline.Metrics) == 0 {
		return result, nil
	}
	if baseline.Status == "learning" && behaviorBaselineHasZeroSamples(baseline) {
		for _, metric := range baseline.Metrics {
			result.Distributions = append(result.Distributions, behaviorBaselineDistributionDTO{MetricName: metric.MetricName, Unit: metric.Unit})
		}
		result.MetricName = baseline.Metrics[0].MetricName
		result.Unit = baseline.Metrics[0].Unit
		return result, nil
	}
	primary := baseline.Metrics[0]
	result.MetricName = primary.MetricName
	result.Unit = primary.Unit
	upper := primary.Mean + primary.StdDev*primary.ThresholdConfig.AlertMultiplier
	lower := math.Max(0, primary.Mean-primary.StdDev*primary.ThresholdConfig.WarningMultiplier)
	startMillis := baseline.CreatedAt

	if baseline.BaselineType == "account" {
		bucketQuery := `
			SELECT toInt64(toUnixTimestamp(bucket)) * 1000, toFloat64(events), toFloat64(sources), toFloat64(resources), login_hours
			FROM (
				SELECT toStartOfHour(timestamp) AS bucket, count() AS events, uniqExact(source_ip) AS sources,
				       uniqExact(resource) AS resources, groupArray(toFloat64(toHour(timestamp))) AS login_hours
				FROM traffic.user_events WHERE tenant_id=? AND username=? AND timestamp>=?
				GROUP BY bucket ORDER BY bucket
			)`
		rows, queryErr := h.chClient.Query(ctx, bucketQuery, tenantID, baseline.EntityID, time.UnixMilli(startMillis))
		if queryErr != nil {
			return result, queryErr
		}
		events := make([]float64, 0)
		sources := make([]float64, 0)
		resources := make([]float64, 0)
		loginHours := make([]float64, 0)
		for rows.Next() {
			var timestamp int64
			var eventCount, sourceCount, resourceCount float64
			var bucketLoginHours []float64
			if scanErr := rows.Scan(&timestamp, &eventCount, &sourceCount, &resourceCount, &bucketLoginHours); scanErr != nil {
				rows.Close()
				return result, scanErr
			}
			events = append(events, eventCount)
			sources = append(sources, sourceCount)
			resources = append(resources, resourceCount)
			loginHours = append(loginHours, bucketLoginHours...)
			result.Series = append(result.Series, behaviorBaselineSeriesPointDTO{Timestamp: timestamp, Mean: eventCount, P50: eventCount, P95: eventCount, P99: eventCount, Upper: upper, Lower: lower, Samples: []float64{eventCount}})
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return result, rowsErr
		}
		rows.Close()
		result.Distributions = []behaviorBaselineDistributionDTO{
			{MetricName: "login_hour", Unit: "hour", Values: fiveNumberSummary(loginHours)},
			{MetricName: "events_per_window", Unit: "events", Values: fiveNumberSummary(events)},
			{MetricName: "source_ip_count", Unit: "addresses", Values: fiveNumberSummary(sources)},
			{MetricName: "resource_count", Unit: "resources", Values: fiveNumberSummary(resources)},
		}
		return result, nil
	}

	dimensions := map[string]string{
		"asset": "src_ip", "port": "toString(dst_port)", "protocol": "toString(protocol)",
		"time": "toString(toHour(toDateTime(intDiv(ts_start, 1000))))",
	}
	dimension, ok := dimensions[baseline.BaselineType]
	if !ok {
		return result, fmt.Errorf("unsupported baseline type: %s", baseline.BaselineType)
	}
	distributionQuery := fmt.Sprintf(`
		SELECT
		 min(toFloat64(bytes_total)), toFloat64(quantileTDigest(0.25)(toFloat64(bytes_total))), toFloat64(quantileTDigest(0.5)(toFloat64(bytes_total))), toFloat64(quantileTDigest(0.75)(toFloat64(bytes_total))), max(toFloat64(bytes_total)),
		 min(toFloat64(num_pkts)), toFloat64(quantileTDigest(0.25)(toFloat64(num_pkts))), toFloat64(quantileTDigest(0.5)(toFloat64(num_pkts))), toFloat64(quantileTDigest(0.75)(toFloat64(num_pkts))), max(toFloat64(num_pkts)),
		 min(toFloat64(duration_ms)), toFloat64(quantileTDigest(0.25)(toFloat64(duration_ms))), toFloat64(quantileTDigest(0.5)(toFloat64(duration_ms))), toFloat64(quantileTDigest(0.75)(toFloat64(duration_ms))), max(toFloat64(duration_ms))
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? AND toString(%s)=?`, dimension)
	values := make([]float64, 15)
	row, err := h.chClient.QueryRow(ctx, distributionQuery, tenantID, startMillis, baseline.EntityID)
	if err != nil {
		return result, err
	}
	if err := row.Scan(&values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6], &values[7], &values[8], &values[9], &values[10], &values[11], &values[12], &values[13], &values[14]); err != nil {
		return result, err
	}
	result.Distributions = []behaviorBaselineDistributionDTO{
		{MetricName: "bytes_per_session", Unit: "bytes", Values: [5]float64{values[0], values[1], values[2], values[3], values[4]}},
		{MetricName: "packets_per_session", Unit: "packets", Values: [5]float64{values[5], values[6], values[7], values[8], values[9]}},
		{MetricName: "duration_ms", Unit: "ms", Values: [5]float64{values[10], values[11], values[12], values[13], values[14]}},
	}
	seriesQuery := fmt.Sprintf(`
		SELECT toInt64(toUnixTimestamp(bucket)) * 1000,
		       avg(toFloat64(bytes_total)), toFloat64(quantileTDigest(0.5)(toFloat64(bytes_total))), toFloat64(quantileTDigest(0.95)(toFloat64(bytes_total))), toFloat64(quantileTDigest(0.99)(toFloat64(bytes_total))),
		       groupArray(20)(toFloat64(bytes_total))
		FROM (
			SELECT toStartOfHour(toDateTime(intDiv(ts_start, 1000))) AS bucket, bytes_total
			FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? AND toString(%s)=?
		)
		GROUP BY bucket ORDER BY bucket`, dimension)
	rows, err := h.chClient.Query(ctx, seriesQuery, tenantID, startMillis, baseline.EntityID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var point behaviorBaselineSeriesPointDTO
		if err := rows.Scan(&point.Timestamp, &point.Mean, &point.P50, &point.P95, &point.P99, &point.Samples); err != nil {
			return result, err
		}
		point.Upper = dashboardFinite(upper)
		point.Lower = dashboardFinite(lower)
		point.Mean = dashboardFinite(point.Mean)
		point.P50 = dashboardFinite(point.P50)
		point.P95 = dashboardFinite(point.P95)
		point.P99 = dashboardFinite(point.P99)
		result.Series = append(result.Series, point)
	}
	return result, rows.Err()
}

func behaviorBaselineHasZeroSamples(baseline behaviorBaselineDTO) bool {
	if len(baseline.Metrics) == 0 {
		return true
	}
	for _, metric := range baseline.Metrics {
		if metric.Mean != 0 || metric.StdDev != 0 || metric.CurrentValue != 0 || metric.DeviationScore != 0 {
			return false
		}
	}
	return true
}

func fiveNumberSummary(values []float64) [5]float64 {
	if len(values) == 0 {
		return [5]float64{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	quantile := func(p float64) float64 {
		index := p * float64(len(sorted)-1)
		lower := int(math.Floor(index))
		upper := int(math.Ceil(index))
		if lower == upper {
			return dashboardFinite(sorted[lower])
		}
		return dashboardFinite(sorted[lower] + (sorted[upper]-sorted[lower])*(index-float64(lower)))
	}
	return [5]float64{dashboardFinite(sorted[0]), quantile(.25), quantile(.5), quantile(.75), dashboardFinite(sorted[len(sorted)-1])}
}

func (h *SystemHandler) queryBaselineMetrics(ctx context.Context, tenantID, ip string, start int64) ([]behaviorMetricDTO, uint64, int64, error) {
	var samples uint64
	var updated int64
	var bytesMean, bytesStd, packetsMean, packetsStd, durationMean, durationStd float64
	row, err := h.chClient.QueryRow(ctx, `
		SELECT count(), max(ts_end),
		       avg(toFloat64(bytes_total)), stddevPop(toFloat64(bytes_total)),
		       avg(toFloat64(num_pkts)), stddevPop(toFloat64(num_pkts)),
		       avg(toFloat64(duration_ms)), stddevPop(toFloat64(duration_ms))
		FROM traffic.sessions
		WHERE tenant_id=? AND src_ip=? AND ts_start>=?`, tenantID, ip, start)
	if err != nil {
		return nil, 0, 0, err
	}
	if err := row.Scan(&samples, &updated, &bytesMean, &bytesStd, &packetsMean, &packetsStd, &durationMean, &durationStd); err != nil {
		return nil, 0, 0, err
	}
	currentBytes := h.currentBaselineValue(ctx, tenantID, ip, "bytes_total")
	currentPackets := h.currentBaselineValue(ctx, tenantID, ip, "num_pkts")
	currentDuration := h.currentBaselineValue(ctx, tenantID, ip, "duration_ms")
	return []behaviorMetricDTO{
		metricDTO("bytes_per_session", "bytes", bytesMean, bytesStd, currentBytes),
		metricDTO("packets_per_session", "packets", packetsMean, packetsStd, currentPackets),
		metricDTO("duration_ms", "ms", durationMean, durationStd, currentDuration),
	}, samples, updated, nil
}

func (h *SystemHandler) currentBaselineValue(ctx context.Context, tenantID, ip, column string) float64 {
	var value float64
	query := fmt.Sprintf("SELECT avg(toFloat64(%s)) FROM traffic.sessions WHERE tenant_id=? AND src_ip=? AND ts_start>=?", column)
	row, err := h.chClient.QueryRow(ctx, query, tenantID, ip, time.Now().Add(-15*time.Minute).UnixMilli())
	if err == nil {
		_ = row.Scan(&value)
	}
	return dashboardFinite(value)
}

func metricDTO(name, unit string, mean, std, current float64) behaviorMetricDTO {
	mean = dashboardFinite(mean)
	std = dashboardFinite(std)
	current = dashboardFinite(current)
	low := mean - 2*std
	if low < 0 {
		low = 0
	}
	dev := 0.0
	if std > 0 {
		dev = math.Abs(current-mean) / std
	}
	return behaviorMetricDTO{
		MetricName: name, Unit: unit, NormalRange: [2]float64{low, mean + 2*std},
		Mean: mean, StdDev: std, CurrentValue: current, DeviationScore: dashboardFinite(dev),
		ThresholdConfig: behaviorThresholdConfig{WarningMultiplier: 2, AlertMultiplier: 3},
	}
}

func (h *SystemHandler) complianceSummary(ctx context.Context, tenantID string, start, end int64) (complianceSummaryDTO, error) {
	var summary complianceSummaryDTO
	var totalAlerts, criticalAlerts, resolvedAlerts, falsePositives, slaViolations uint64
	if h.chClient == nil {
		return summary, fmt.Errorf("clickhouse is not configured")
	}
	row, err := h.chClient.QueryRow(ctx, `
		SELECT count(),
		       countIf(severity IN ('critical','SEVERITY_CRITICAL')),
		       countIf(status IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED')),
		       countIf(feedback_label IN ('false_positive','fp')),
		       avgIf(toFloat64(greatest(updated_at-first_seen, 0))/60000.0, status IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED')),
		       countIf(severity IN ('critical','high','SEVERITY_CRITICAL','SEVERITY_HIGH') AND status NOT IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED') AND (? - first_seen) > 86400000)
		FROM traffic.alerts
		WHERE tenant_id=? AND first_seen>=? AND first_seen<=?`, end, tenantID, start, end)
	if err != nil {
		return summary, fmt.Errorf("query compliance source: %w", err)
	}
	if err := row.Scan(&totalAlerts, &criticalAlerts, &resolvedAlerts, &falsePositives, &summary.AvgResponseTimeMin, &slaViolations); err != nil {
		return summary, fmt.Errorf("scan compliance source: %w", err)
	}
	summary.TotalAlerts = int64(totalAlerts)
	summary.CriticalAlerts = int64(criticalAlerts)
	summary.ResolvedAlerts = int64(resolvedAlerts)
	summary.FalsePositives = int64(falsePositives)
	summary.SLAViolations = int64(slaViolations)
	summary.AvgResponseTimeMin = dashboardFinite(summary.AvgResponseTimeMin)
	return summary, nil
}

func complianceSections(summary complianceSummaryDTO) []complianceSectionDTO {
	resolutionRate := 0.0
	if summary.TotalAlerts > 0 {
		resolutionRate = float64(summary.ResolvedAlerts) / float64(summary.TotalAlerts)
	}
	alertEvidenceStatus := "available"
	alertResponseStatus := sectionStatus(resolutionRate >= 0.8, resolutionRate >= 0.5)
	criticalStatus := sectionStatus(summary.SLAViolations == 0, summary.SLAViolations <= 3)
	feedbackStatus := sectionStatus(summary.FalsePositives > 0, false)
	if summary.TotalAlerts == 0 {
		alertEvidenceStatus = "insufficient"
		alertResponseStatus = "insufficient_evidence"
		criticalStatus = "insufficient_evidence"
		feedbackStatus = "insufficient_evidence"
	}
	missing := func(source, reason string) map[string]interface{} {
		return map[string]interface{}{
			"evidence_status": "insufficient",
			"source":          source,
			"actual_value":    "not_provided",
			"reason":          reason,
			"evaluated":       false,
		}
	}
	return []complianceSectionDTO{
		{SectionName: "collection_coverage", Title: "采集覆盖率", Content: missing("probe inventory and heartbeat", "probe coverage evidence was not supplied"), Status: "insufficient_evidence"},
		{SectionName: "data_quality", Title: "数据质量", Content: missing("Kafka, Flink and ClickHouse quality metrics", "integrity and deduplication evidence was not supplied"), Status: "insufficient_evidence"},
		{
			SectionName: "alert_response", Title: "告警响应闭环",
			Content: map[string]interface{}{"resolved_alerts": summary.ResolvedAlerts, "total_alerts": summary.TotalAlerts, "resolution_rate": resolutionRate, "avg_response_time_min": summary.AvgResponseTimeMin, "evidence_status": alertEvidenceStatus, "source": "traffic.alerts"},
			Status:  alertResponseStatus,
		},
		{SectionName: "pcap_evidence", Title: "PCAP 证据覆盖", Content: missing("pcap_index and evidence objects", "PCAP hash coverage was not supplied"), Status: "insufficient_evidence"},
		{SectionName: "model_quality", Title: "模型效果", Content: missing("model evaluation registry", "model F1 evidence was not supplied"), Status: "insufficient_evidence"},
		{SectionName: "audit_integrity", Title: "审计链路完整性", Content: missing("audit_logs coverage summary", "audit completeness evidence was not supplied"), Status: "insufficient_evidence"},
		{SectionName: "deployment_baseline", Title: "部署基线一致性", Content: missing("Kubernetes workload and image manifests", "deployment baseline evidence was not supplied"), Status: "insufficient_evidence"},
		{
			SectionName: "critical_alerts", Title: "严重风险处置",
			Content: map[string]interface{}{"critical_alerts": summary.CriticalAlerts, "sla_violations": summary.SLAViolations, "evidence_status": alertEvidenceStatus, "source": "traffic.alerts"},
			Status:  criticalStatus,
		},
		{
			SectionName: "feedback_quality", Title: "误报反馈质量",
			Content: map[string]interface{}{"false_positives": summary.FalsePositives, "evidence_status": alertEvidenceStatus, "source": "traffic.alerts"},
			Status:  feedbackStatus,
		},
	}
}

func complianceReportStatus(sections []complianceSectionDTO) string {
	status := "completed"
	for _, section := range sections {
		switch section.Status {
		case "insufficient_evidence", "blocked":
			return "insufficient_evidence"
		case "fail", "warning", "warn":
			status = "non_compliant"
		}
	}
	return status
}

func scanComplianceReport(scanner interface {
	Scan(dest ...interface{}) error
}) (complianceReportDTO, error) {
	var report complianceReportDTO
	var start, end int64
	var summaryJSON, sectionsJSON string
	var generatedAt time.Time
	if err := scanner.Scan(&report.ReportID, &report.TenantID, &report.ReportType, &start, &end, &report.Status, &summaryJSON, &sectionsJSON, &report.GeneratedBy, &generatedAt); err != nil {
		return complianceReportDTO{}, err
	}
	report.TimeRange = map[string]int64{"start": start, "end": end}
	report.GeneratedAt = generatedAt.UnixMilli()
	_ = json.Unmarshal([]byte(summaryJSON), &report.Summary)
	_ = json.Unmarshal([]byte(sectionsJSON), &report.Sections)
	return report, nil
}

func validateComplianceRange(start, end int64, now time.Time) error {
	if start <= 0 || end <= 0 || start >= end {
		return fmt.Errorf("time range must satisfy start < end")
	}
	if end > now.Add(5*time.Minute).UnixMilli() {
		return fmt.Errorf("time range cannot end in the future")
	}
	if end-start > int64((366*24*time.Hour)/time.Millisecond) {
		return fmt.Errorf("time range cannot exceed 366 days")
	}
	return nil
}

func buildComplianceEvidencePackage(report complianceReportDTO) ([]byte, string, error) {
	reportJSON, err := canonicalComplianceReportJSON(report)
	if err != nil {
		return nil, "", err
	}
	manifest := map[string]interface{}{
		"schema_version": 1,
		"artifact_type":  "compliance_evidence_package",
		"report_id":      report.ReportID,
		"tenant_id":      report.TenantID,
		"report_status":  report.Status,
		"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
		"report_sha256":  fmt.Sprintf("sha256:%x", sha256.Sum256(reportJSON)),
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, "", err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string][]byte{"manifest.json": manifestJSON, "report.json": reportJSON} {
		entry, createErr := writer.Create(name)
		if createErr != nil {
			return nil, "", createErr
		}
		if _, writeErr := entry.Write(content); writeErr != nil {
			return nil, "", writeErr
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	content := buffer.Bytes()
	return content, fmt.Sprintf("sha256:%x", sha256.Sum256(content)), nil
}

func canonicalComplianceReportJSON(report complianceReportDTO) ([]byte, error) {
	return json.Marshal(report)
}

func complianceReportSHA256(report complianceReportDTO) (string, error) {
	payload, err := canonicalComplianceReportJSON(report)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(payload)), nil
}

func (h *SystemHandler) insertAuditLog(ctx context.Context, tenantID, userID, action, objectType, objectID string, detail map[string]interface{}, r *http.Request) error {
	if h.pgDB == nil {
		return nil
	}
	detailJSON, _ := json.Marshal(detail)
	ip := clientIP(r)
	userAgent := r.UserAgent()
	var err error
	if h.pgColumnExists(ctx, "audit_logs", "event_id") {
		eventID := "audit-" + time.Now().UTC().Format("20060102150405.000000000")
		_, err = h.pgDB.ExecContext(ctx, `
			INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7::jsonb, $8, $9)`,
			eventID, tenantID, userID, action, objectType, objectID, string(detailJSON), ip, userAgent)
		return err
	}
	_, err = h.pgDB.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6::jsonb, $7, $8)`,
		tenantID, userID, action, objectType, objectID, string(detailJSON), ip, userAgent)
	return err
}

func insertFusionAuditTx(ctx context.Context, tx *sql.Tx, tenantID, userID, action, objectType, objectID string, detail map[string]interface{}, r *http.Request) error {
	detailJSON, _ := json.Marshal(detail)
	eventID := fmt.Sprintf("audit-fusion-%d", time.Now().UTC().UnixNano())
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7::jsonb, $8, $9)`,
		eventID, tenantID, userID, action, objectType, objectID, string(detailJSON), clientIP(r), r.UserAgent())
	return err
}

func (h *SystemHandler) assetIPExpression(ctx context.Context) string {
	hasIPAddress := h.pgColumnExists(ctx, "assets", "ip_address")
	hasIP := h.pgColumnExists(ctx, "assets", "ip")
	switch {
	case hasIPAddress && hasIP:
		return "COALESCE(NULLIF(ip_address, ''), NULLIF(ip, ''), '')"
	case hasIPAddress:
		return "COALESCE(ip_address, '')"
	case hasIP:
		return "COALESCE(ip, '')"
	default:
		return "''"
	}
}

func (h *SystemHandler) assetCriticalityExpression(ctx context.Context) string {
	if h.pgColumnExists(ctx, "assets", "criticality") {
		return "COALESCE(criticality, 0)"
	}
	return "0"
}

func (h *SystemHandler) assetTimestampExpression(ctx context.Context) string {
	columns := make([]string, 0, 3)
	for _, column := range []string{"last_seen", "updated_at", "created_at"} {
		if h.pgColumnExists(ctx, "assets", column) {
			columns = append(columns, column)
		}
	}
	if len(columns) == 0 {
		return "now()"
	}
	columns = append(columns, "now()")
	return "COALESCE(" + strings.Join(columns, ", ") + ")"
}

func (h *SystemHandler) pgColumnExists(ctx context.Context, tableName, columnName string) bool {
	if h.pgDB == nil {
		return false
	}
	var exists bool
	err := h.pgDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1
			  AND column_name = $2
		)`, tableName, columnName).Scan(&exists)
	return err == nil && exists
}

func queryTimeRange(r *http.Request, lookback time.Duration) (int64, int64) {
	start, end, err := dashboardRange(r, lookback)
	if err != nil {
		now := time.Now()
		return now.Add(-lookback).UnixMilli(), now.UnixMilli()
	}
	return start, end
}

func parsePageLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit, _ := strconvAtoi(firstNonEmpty(r.URL.Query().Get("page_size"), r.URL.Query().Get("limit")))
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset, _ := strconvAtoi(r.URL.Query().Get("offset"))
	if page, err := strconvAtoi(r.URL.Query().Get("page")); err == nil && page > 0 {
		offset = (page - 1) * limit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func strconvAtoi(value string) (int, error) {
	return strconv.Atoi(value)
}

func encryptedProtocol(proto uint8, port uint32) string {
	if port == 22 {
		return "SSH"
	}
	if proto == 17 && (port == 443 || port == 8443) {
		return "QUIC"
	}
	return "TLS"
}

func tlsVersionLabel(protocol string) string {
	if protocol == "TLS" {
		return "TLS"
	}
	return ""
}

func encryptedAnomalyScore(bytes uint64, durationMs uint32, port uint32) float64 {
	score := 0.05
	if bytes > 100*1024*1024 {
		score += 0.35
	}
	if durationMs > 3600*1000 {
		score += 0.25
	}
	if port != 443 && port != 8443 {
		score += 0.15
	}
	return clamp01(score)
}

func encryptedRisk(score float64) string {
	if score >= 0.75 {
		return "malicious"
	}
	if score >= 0.35 {
		return "suspicious"
	}
	return "normal"
}

func tunnelRisk(protocol string, count int64, totalBytes uint64) string {
	if totalBytes >= 1024*1024*1024 || count >= 100 || (protocol == "DNS" && count >= 50) {
		return "high"
	}
	if totalBytes >= 100*1024*1024 || count >= 20 {
		return "medium"
	}
	return "low"
}

func exfiltrationRisk(uploadBytes uint64, sessionCount int64) string {
	if uploadBytes >= 1024*1024*1024 || sessionCount >= 100 {
		return "high"
	}
	if uploadBytes >= 100*1024*1024 || sessionCount >= 20 {
		return "medium"
	}
	return "low"
}

func riskSeverity(count uint64, activeSeverity string) string {
	if count == 0 {
		return "low"
	}
	return activeSeverity
}

func criticalityLabel(value int) string {
	if value >= 80 {
		return "critical"
	}
	if value >= 40 {
		return "important"
	}
	return "normal"
}

func baselineID(entityType, entityID string) string {
	return entityType + ":" + entityID
}

func parseBaselineID(id string) (string, string) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func complianceReportRange(reportType string, custom *complianceRange) (int64, int64) {
	if custom != nil && custom.Start > 0 && custom.End >= custom.Start {
		return custom.Start, custom.End
	}
	now := time.Now()
	switch reportType {
	case "daily":
		return now.Add(-24 * time.Hour).UnixMilli(), now.UnixMilli()
	case "monthly":
		return now.Add(-30 * 24 * time.Hour).UnixMilli(), now.UnixMilli()
	default:
		return now.Add(-7 * 24 * time.Hour).UnixMilli(), now.UnixMilli()
	}
}

func sectionStatus(pass, warning bool) string {
	if pass {
		return "pass"
	}
	if warning {
		return "warning"
	}
	return "fail"
}

func auditResult(action string, details map[string]interface{}) string {
	if value, ok := details["result"].(string); ok && value != "" {
		if strings.Contains(strings.ToLower(value), "fail") {
			return "failure"
		}
		return "success"
	}
	if strings.Contains(strings.ToLower(action), "failed") || strings.Contains(strings.ToLower(action), "failure") {
		return "failure"
	}
	return "success"
}

func countTunnelUsersByRisk(users []encryptedTunnelUserDTO, risk string) int {
	count := 0
	for _, user := range users {
		if user.Risk == risk {
			count++
		}
	}
	return count
}

func countExfiltrationSourcesByRisk(sources []encryptedExfiltrationSourceDTO, risk string) int {
	count := 0
	for _, source := range sources {
		if source.Risk == risk {
			count++
		}
	}
	return count
}

func activeFusionSourceCount(sources []dataSourceDTO) int {
	active := 0
	for _, source := range sources {
		if source.Status == "active" {
			active++
		}
	}
	return active
}

func rateFromCounts(part, total int64) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	return clamp01(float64(part) / float64(total))
}

func relativeReductionPct(baseline, current float64) float64 {
	if baseline <= 0 {
		return 0
	}
	return math.Max(0, (baseline-current)/baseline*100)
}

func relativeLiftPct(baseline, current float64) float64 {
	if baseline <= 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return math.Max(0, (current-baseline)/baseline*100)
}

func valueGateStatus(pass, warn bool) string {
	if pass {
		return "pass"
	}
	if warn {
		return "warn"
	}
	return "blocked"
}

func valueEvidenceStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "warn"
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}

func clamp01(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

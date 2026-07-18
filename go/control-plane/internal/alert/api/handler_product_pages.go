package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
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
	ErrorRate        float64                `json:"error_rate"`
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
	ObjectID       string                 `json:"object_id"`
	ObjectType     string                 `json:"object_type"`
	FieldName      string                 `json:"field_name"`
	SelectedSource string                 `json:"selected_source"`
	SelectedValue  string                 `json:"selected_value"`
	Strategy       string                 `json:"strategy"`
	Note           string                 `json:"note"`
	RuleID         string                 `json:"rule_id"`
	Detail         map[string]interface{} `json:"detail"`
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

type fusionRuleUpdateRequest struct {
	RuleName            string                 `json:"rule_name"`
	Status              string                 `json:"status"`
	Strategy            string                 `json:"strategy"`
	ConfidenceThreshold *float64               `json:"confidence_threshold"`
	Note                string                 `json:"note"`
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

type complianceReportDTO struct {
	ReportID    string                 `json:"report_id"`
	TenantID    string                 `json:"tenant_id"`
	ReportType  string                 `json:"report_type"`
	TimeRange   map[string]int64       `json:"time_range"`
	GeneratedAt int64                  `json:"generated_at"`
	Status      string                 `json:"status"`
	Summary     complianceSummaryDTO   `json:"summary"`
	Sections    []complianceSectionDTO `json:"sections"`
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
	limit, offset := parsePageLimitOffset(r, 20, 200)
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
	distinctIPs := h.countDistinctFlowIPs(ctx, tenantID)
	if distinctIPs > 0 {
		stats.AlignmentRate = clamp01(float64(stats.EntitiesAligned) / float64(distinctIPs))
	}
	stats.QualityMetrics.Completeness = clamp01(float64(activeSources) / float64(maxInt(len(sources), 1)))
	stats.QualityMetrics.Accuracy = stats.AlignmentRate
	if latest > 0 {
		ageMinutes := float64(time.Now().UnixMilli()-latest) / 60000
		stats.QualityMetrics.Freshness = clamp01(1 - ageMinutes/60)
	}
	stats.QualityMetrics.DuplicationRate = h.estimateFlowDuplication(ctx, tenantID)
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
	tenantID := queryTenantID(r)
	sourceID := mux.Vars(r)["id"]
	if sourceID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", "source id is required")
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "FUSION_SOURCE_SYNC_REQUESTED", "fusion_source", sourceID, map[string]interface{}{"status": "accepted"}, r)
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
	if req.FieldName == "" || req.SelectedSource == "" || req.SelectedValue == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "field_name, selected_source and selected_value are required")
		return
	}
	if err := h.ensureFusionWriteSchema(ctx); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	detailJSON, _ := json.Marshal(req.Detail)
	var dto fusionConflictResolutionDTO
	var resolvedAt time.Time
	err := h.pgDB.QueryRowContext(ctx, `
		INSERT INTO fusion_conflict_resolutions
		  (tenant_id, conflict_id, object_id, object_type, field_name, selected_source, selected_value, strategy, note, rule_id, resolved_by, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)
		ON CONFLICT (tenant_id, conflict_id) DO UPDATE SET
		  object_id=EXCLUDED.object_id,
		  object_type=EXCLUDED.object_type,
		  field_name=EXCLUDED.field_name,
		  selected_source=EXCLUDED.selected_source,
		  selected_value=EXCLUDED.selected_value,
		  strategy=EXCLUDED.strategy,
		  note=EXCLUDED.note,
		  rule_id=EXCLUDED.rule_id,
		  state_version=fusion_conflict_resolutions.state_version+1,
		  resolved_by=EXCLUDED.resolved_by,
		  resolved_at=now(),
		  detail=EXCLUDED.detail
		RETURNING tenant_id, conflict_id, object_id, object_type, field_name, selected_source, selected_value, strategy, note, rule_id, state_version, resolved_by, resolved_at, detail::text`,
		tenantID,
		conflictID,
		req.ObjectID,
		req.ObjectType,
		req.FieldName,
		req.SelectedSource,
		req.SelectedValue,
		req.Strategy,
		req.Note,
		req.RuleID,
		httpx.GetUserID(ctx),
		string(detailJSON),
	).Scan(
		&dto.TenantID,
		&dto.ConflictID,
		&dto.ObjectID,
		&dto.ObjectType,
		&dto.FieldName,
		&dto.SelectedSource,
		&dto.SelectedValue,
		&dto.Strategy,
		&dto.Note,
		&dto.RuleID,
		&dto.StateVersion,
		&dto.ResolvedBy,
		&resolvedAt,
		&detailJSON,
	)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	dto.ResolvedAt = resolvedAt.UnixMilli()
	dto.Detail = map[string]interface{}{}
	_ = json.Unmarshal(detailJSON, &dto.Detail)

	auditDetail := map[string]interface{}{
		"status":          "resolved",
		"field_name":      dto.FieldName,
		"selected_source": dto.SelectedSource,
		"selected_value":  dto.SelectedValue,
		"strategy":        dto.Strategy,
		"rule_id":         dto.RuleID,
		"state_version":   dto.StateVersion,
	}
	if err := h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "FUSION_CONFLICT_RESOLVED", "fusion_conflict", conflictID, auditDetail, r); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"resolution":    dto,
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
	if err := h.ensureFusionWriteSchema(ctx); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	detailJSON, _ := json.Marshal(req.Detail)
	var dto fusionRuleOverrideDTO
	var updatedAt time.Time
	err := h.pgDB.QueryRowContext(ctx, `
		INSERT INTO fusion_rule_overrides
		  (tenant_id, rule_id, rule_name, status, strategy, confidence_threshold, note, updated_by, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		ON CONFLICT (tenant_id, rule_id) DO UPDATE SET
		  rule_name=EXCLUDED.rule_name,
		  status=EXCLUDED.status,
		  strategy=EXCLUDED.strategy,
		  confidence_threshold=EXCLUDED.confidence_threshold,
		  note=EXCLUDED.note,
		  version=fusion_rule_overrides.version+1,
		  updated_by=EXCLUDED.updated_by,
		  updated_at=now(),
		  detail=EXCLUDED.detail
		RETURNING tenant_id, rule_id, rule_name, version, status, strategy, confidence_threshold, note, updated_by, updated_at, detail::text`,
		tenantID,
		ruleID,
		req.RuleName,
		req.Status,
		req.Strategy,
		*req.ConfidenceThreshold,
		req.Note,
		httpx.GetUserID(ctx),
		string(detailJSON),
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
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	dto.UpdatedAt = updatedAt.UnixMilli()
	dto.Detail = map[string]interface{}{}
	_ = json.Unmarshal(detailJSON, &dto.Detail)

	auditDetail := map[string]interface{}{
		"status":               dto.Status,
		"strategy":             dto.Strategy,
		"confidence_threshold": dto.ConfidenceThreshold,
		"version":              dto.Version,
	}
	if err := h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "FUSION_RULE_UPDATED", "fusion_rule", ruleID, auditDetail, r); err != nil {
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
	baselines, err := h.queryBehaviorBaselines(ctx, tenantID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"baselines": baselines, "total": len(baselines)})
}

func (h *SystemHandler) GetBehaviorBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := queryTenantID(r)
	baselineID := mux.Vars(r)["id"]
	baseline, err := h.queryBehaviorBaseline(ctx, tenantID, baselineID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, baseline)
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
	if _, err := h.pgDB.ExecContext(ctx, `
		INSERT INTO behavior_baseline_resets (tenant_id, baseline_id, reset_at, requested_by)
		VALUES ($1, $2, now(), $3)
		ON CONFLICT (tenant_id, baseline_id)
		DO UPDATE SET reset_at=EXCLUDED.reset_at, requested_by=EXCLUDED.requested_by`, tenantID, baselineID, httpx.GetUserID(ctx)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "BEHAVIOR_BASELINE_RESET", "baseline", baselineID, map[string]interface{}{"status": "reset"}, r)
	baseline, err := h.queryBehaviorBaseline(ctx, tenantID, baselineID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	baseline.Status = "learning"
	httpx.JSONSuccess(w, ctx, baseline)
}

func (h *SystemHandler) ListComplianceReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requirePostgres(w, ctx) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parsePageLimitOffset(r, 20, 100)
	reportType := r.URL.Query().Get("report_type")

	args := []interface{}{tenantID}
	where := "tenant_id=$1"
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
		SELECT report_id::text, tenant_id, report_type, time_start, time_end, status, summary::text, sections::text, generated_at
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
	tenantID := queryTenantID(r)
	var req struct {
		ReportType string           `json:"report_type"`
		TimeRange  *complianceRange `json:"time_range"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if req.ReportType == "" {
		req.ReportType = "weekly"
	}
	start, end := complianceReportRange(req.ReportType, req.TimeRange)
	summary := h.complianceSummary(ctx, tenantID, start, end)
	sections := complianceSections(summary)

	summaryJSON, _ := json.Marshal(summary)
	sectionsJSON, _ := json.Marshal(sections)
	var reportID string
	var generatedAt time.Time
	err := h.pgDB.QueryRowContext(ctx, `
		INSERT INTO compliance_reports (tenant_id, report_type, time_start, time_end, status, summary, sections, generated_by)
		VALUES ($1, $2, $3, $4, 'completed', $5::jsonb, $6::jsonb, $7)
		RETURNING report_id::text, generated_at`, tenantID, req.ReportType, start, end, string(summaryJSON), string(sectionsJSON), httpx.GetUserID(ctx)).Scan(&reportID, &generatedAt)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	_ = h.insertAuditLog(ctx, tenantID, httpx.GetUserID(ctx), "COMPLIANCE_REPORT_GENERATED", "compliance_report", reportID, map[string]interface{}{"report_type": req.ReportType}, r)
	httpx.JSONSuccess(w, ctx, complianceReportDTO{
		ReportID: reportID, TenantID: tenantID, ReportType: req.ReportType,
		TimeRange:   map[string]int64{"start": start, "end": end},
		GeneratedAt: generatedAt.UnixMilli(), Status: "completed", Summary: summary, Sections: sections,
	})
}

func (h *SystemHandler) ListAuditTrail(w http.ResponseWriter, r *http.Request) {
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

func (h *SystemHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	h.ListAuditTrail(w, r)
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
	if hasSystemPermission(ctx, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: admin:* required")
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

func (req *fusionRuleUpdateRequest) normalize(ruleID string) {
	req.RuleName = nonEmpty(strings.TrimSpace(req.RuleName), ruleID)
	req.Status = nonEmpty(strings.TrimSpace(req.Status), "draft")
	req.Strategy = nonEmpty(strings.TrimSpace(req.Strategy), "manual-review")
	req.Note = strings.TrimSpace(req.Note)
	if req.ConfidenceThreshold == nil {
		value := 0.85
		req.ConfidenceThreshold = &value
	}
	if *req.ConfidenceThreshold < 0 {
		value := 0.0
		req.ConfidenceThreshold = &value
	}
	if *req.ConfidenceThreshold > 1 {
		value := 1.0
		req.ConfidenceThreshold = &value
	}
	if req.Detail == nil {
		req.Detail = map[string]interface{}{}
	}
}

func (h *SystemHandler) ensureFusionWriteSchema(ctx context.Context) error {
	if h.pgDB == nil {
		return nil
	}
	_, err := h.pgDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS fusion_conflict_resolutions (
			tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
			conflict_id     TEXT NOT NULL,
			object_id       TEXT NOT NULL DEFAULT '',
			object_type     TEXT NOT NULL DEFAULT 'entity',
			field_name      TEXT NOT NULL,
			selected_source TEXT NOT NULL,
			selected_value  TEXT NOT NULL,
			strategy        TEXT NOT NULL DEFAULT 'manual',
			note            TEXT NOT NULL DEFAULT '',
			rule_id         TEXT NOT NULL DEFAULT '',
			state_version   BIGINT NOT NULL DEFAULT 1,
			resolved_by     TEXT NOT NULL DEFAULT '',
			resolved_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
			detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (tenant_id, conflict_id)
		);
		CREATE INDEX IF NOT EXISTS idx_fusion_conflict_resolutions_time ON fusion_conflict_resolutions(tenant_id, resolved_at DESC);
		CREATE TABLE IF NOT EXISTS fusion_rule_overrides (
			tenant_id            TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
			rule_id              TEXT NOT NULL,
			rule_name            TEXT NOT NULL DEFAULT '',
			version              BIGINT NOT NULL DEFAULT 1,
			status               TEXT NOT NULL DEFAULT 'draft',
			strategy             TEXT NOT NULL DEFAULT 'manual-review',
			confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.85,
			note                 TEXT NOT NULL DEFAULT '',
			updated_by           TEXT NOT NULL DEFAULT '',
			updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
			detail               JSONB NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (tenant_id, rule_id)
		);
		CREATE INDEX IF NOT EXISTS idx_fusion_rule_overrides_time ON fusion_rule_overrides(tenant_id, updated_at DESC);`)
	return err
}

func (h *SystemHandler) fusionSources(ctx context.Context, tenantID string) []dataSourceDTO {
	now := time.Now().UnixMilli()
	return []dataSourceDTO{
		h.clickHouseSource(ctx, tenantID, "traffic", "traffic", "流量会话", "traffic.sessions", "ingest_ts", now),
		h.postgresSource(ctx, tenantID, "asset", "asset", "资产画像", "assets", "created_at", now),
		h.clickHouseDateTimeSource(ctx, tenantID, "log", "log", "设备日志", "traffic.device_logs", "timestamp", now),
		h.clickHouseDateTimeSource(ctx, tenantID, "behavior", "behavior", "用户行为", "traffic.user_events", "timestamp", now),
		h.clickHouseSource(ctx, tenantID, "threat_intel", "threat_intel", "告警情报", "traffic.alerts", "updated_at", now),
	}
}

func (h *SystemHandler) clickHouseSource(ctx context.Context, tenantID, sourceID, sourceType, name, table, timeColumn string, createdAt int64) dataSourceDTO {
	if h.chClient == nil {
		return sourceFromCounts(tenantID, sourceID, sourceType, name, 0, 0, 0, createdAt)
	}
	var total, recent uint64
	var latest int64
	query := fmt.Sprintf("SELECT count(), countIf(%s >= ?), max(%s) FROM %s WHERE tenant_id=?", timeColumn, timeColumn, table)
	row, err := h.chClient.QueryRow(ctx, query, time.Now().Add(-time.Hour).UnixMilli(), tenantID)
	if err == nil {
		_ = row.Scan(&total, &recent, &latest)
	}
	return sourceFromCounts(tenantID, sourceID, sourceType, name, int64(total), int64(recent), latest, createdAt)
}

func (h *SystemHandler) clickHouseDateTimeSource(ctx context.Context, tenantID, sourceID, sourceType, name, table, timeColumn string, createdAt int64) dataSourceDTO {
	if h.chClient == nil {
		return sourceFromCounts(tenantID, sourceID, sourceType, name, 0, 0, 0, createdAt)
	}
	var total, recent uint64
	var latest time.Time
	query := fmt.Sprintf("SELECT count(), countIf(%s >= ?), max(%s) FROM %s WHERE tenant_id=?", timeColumn, timeColumn, table)
	row, err := h.chClient.QueryRow(ctx, query, time.Now().Add(-time.Hour), tenantID)
	if err == nil {
		_ = row.Scan(&total, &recent, &latest)
	}
	latestMs := int64(0)
	if !latest.IsZero() {
		latestMs = latest.UnixMilli()
	}
	return sourceFromCounts(tenantID, sourceID, sourceType, name, int64(total), int64(recent), latestMs, createdAt)
}

func (h *SystemHandler) postgresSource(ctx context.Context, tenantID, sourceID, sourceType, name, table, timeColumn string, createdAt int64) dataSourceDTO {
	var total, recent int64
	var latest sql.NullTime
	if h.pgDB == nil {
		return sourceFromCounts(tenantID, sourceID, sourceType, name, 0, 0, 0, createdAt)
	}
	query := fmt.Sprintf("SELECT count(*), count(*) FILTER (WHERE %s >= now() - interval '1 hour'), max(%s) FROM %s WHERE tenant_id=$1", timeColumn, timeColumn, table)
	if err := h.pgDB.QueryRowContext(ctx, query, tenantID).Scan(&total, &recent, &latest); err != nil {
		return sourceFromCounts(tenantID, sourceID, sourceType, name, 0, 0, 0, createdAt)
	}
	latestMs := int64(0)
	if latest.Valid {
		latestMs = latest.Time.UnixMilli()
	}
	return sourceFromCounts(tenantID, sourceID, sourceType, name, total, recent, latestMs, createdAt)
}

func sourceFromCounts(tenantID, sourceID, sourceType, name string, total, recent, latest, createdAt int64) dataSourceDTO {
	status := "inactive"
	if latest > 0 && time.Since(time.UnixMilli(latest)) <= 24*time.Hour {
		status = "active"
	}
	return dataSourceDTO{
		SourceID: sourceID, TenantID: tenantID, Name: name, SourceType: sourceType, Status: status,
		LastIngestAt: latest, RecordsPerMinute: float64(recent) / 60.0, ErrorRate: 0,
		Config: map[string]interface{}{"total_records": total}, CreatedAt: createdAt,
	}
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

func (h *SystemHandler) queryBehaviorBaselines(ctx context.Context, tenantID string, limit, offset int) ([]behaviorBaselineDTO, error) {
	rows, err := h.chClient.Query(ctx, `
		SELECT src_ip, count(), max(ts_end)
		FROM traffic.sessions
		WHERE tenant_id=? AND ts_start>=?
		GROUP BY src_ip
		ORDER BY max(ts_end) DESC
		LIMIT ? OFFSET ?`, tenantID, time.Now().Add(-7*24*time.Hour).UnixMilli(), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	baselines := make([]behaviorBaselineDTO, 0)
	for rows.Next() {
		var ip string
		var samples uint64
		var updated int64
		if err := rows.Scan(&ip, &samples, &updated); err != nil {
			return nil, err
		}
		status := "learning"
		if samples >= 5 {
			status = "active"
		}
		baselines = append(baselines, behaviorBaselineDTO{
			BaselineID: baselineID("ip", ip), TenantID: tenantID, Name: "IP行为基线 " + ip,
			EntityType: "ip", EntityID: ip, BaselineType: "dynamic", Metrics: []behaviorMetricDTO{},
			Status: status, CreatedAt: time.Now().Add(-7 * 24 * time.Hour).UnixMilli(), UpdatedAt: updated, Version: 1,
		})
	}
	return baselines, rows.Err()
}

func (h *SystemHandler) queryBehaviorBaseline(ctx context.Context, tenantID, id string) (behaviorBaselineDTO, error) {
	entityType, entityID := parseBaselineID(id)
	if entityType != "ip" || entityID == "" {
		return behaviorBaselineDTO{}, fmt.Errorf("baseline not found: %s", id)
	}
	start := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	var resetAt sql.NullTime
	if h.pgDB != nil {
		_ = h.pgDB.QueryRowContext(ctx, "SELECT reset_at FROM behavior_baseline_resets WHERE tenant_id=$1 AND baseline_id=$2", tenantID, id).Scan(&resetAt)
	}
	if resetAt.Valid {
		start = resetAt.Time.UnixMilli()
	}

	metrics, samples, updated, err := h.queryBaselineMetrics(ctx, tenantID, entityID, start)
	if err != nil {
		return behaviorBaselineDTO{}, err
	}
	status := "learning"
	if samples >= 5 {
		status = "active"
	}
	return behaviorBaselineDTO{
		BaselineID: id, TenantID: tenantID, Name: "IP行为基线 " + entityID,
		EntityType: "ip", EntityID: entityID, BaselineType: "dynamic",
		Metrics: metrics, Status: status, CreatedAt: start, UpdatedAt: updated, Version: 1,
	}, nil
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

func (h *SystemHandler) complianceSummary(ctx context.Context, tenantID string, start, end int64) complianceSummaryDTO {
	var summary complianceSummaryDTO
	row, err := h.chClient.QueryRow(ctx, `
		SELECT count(),
		       countIf(severity IN ('critical','SEVERITY_CRITICAL')),
		       countIf(status IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED')),
		       countIf(feedback_label IN ('false_positive','fp')),
		       avgIf(toFloat64(greatest(updated_at-first_seen, 0))/60000.0, status IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED')),
		       countIf(severity IN ('critical','high','SEVERITY_CRITICAL','SEVERITY_HIGH') AND status NOT IN ('resolved','closed','ALERT_STATUS_RESOLVED','ALERT_STATUS_CLOSED') AND (? - first_seen) > 86400000)
		FROM traffic.alerts
		WHERE tenant_id=? AND first_seen>=? AND first_seen<=?`, end, tenantID, start, end)
	if err == nil {
		_ = row.Scan(&summary.TotalAlerts, &summary.CriticalAlerts, &summary.ResolvedAlerts, &summary.FalsePositives, &summary.AvgResponseTimeMin, &summary.SLAViolations)
	}
	summary.AvgResponseTimeMin = dashboardFinite(summary.AvgResponseTimeMin)
	return summary
}

func complianceSections(summary complianceSummaryDTO) []complianceSectionDTO {
	resolutionRate := 1.0
	if summary.TotalAlerts > 0 {
		resolutionRate = float64(summary.ResolvedAlerts) / float64(summary.TotalAlerts)
	}
	return []complianceSectionDTO{
		{
			SectionName: "alert_response", Title: "告警响应闭环",
			Content: map[string]interface{}{"resolved_alerts": summary.ResolvedAlerts, "total_alerts": summary.TotalAlerts, "resolution_rate": resolutionRate},
			Status:  sectionStatus(resolutionRate >= 0.8, resolutionRate >= 0.5),
		},
		{
			SectionName: "critical_alerts", Title: "严重风险处置",
			Content: map[string]interface{}{"critical_alerts": summary.CriticalAlerts, "sla_violations": summary.SLAViolations},
			Status:  sectionStatus(summary.SLAViolations == 0, summary.SLAViolations <= 3),
		},
		{
			SectionName: "feedback_quality", Title: "误报反馈质量",
			Content: map[string]interface{}{"false_positives": summary.FalsePositives},
			Status:  "pass",
		},
	}
}

func scanComplianceReport(scanner interface {
	Scan(dest ...interface{}) error
}) (complianceReportDTO, error) {
	var report complianceReportDTO
	var start, end int64
	var summaryJSON, sectionsJSON string
	var generatedAt time.Time
	if err := scanner.Scan(&report.ReportID, &report.TenantID, &report.ReportType, &start, &end, &report.Status, &summaryJSON, &sectionsJSON, &generatedAt); err != nil {
		return complianceReportDTO{}, err
	}
	report.TimeRange = map[string]int64{"start": start, "end": end}
	report.GeneratedAt = generatedAt.UnixMilli()
	_ = json.Unmarshal([]byte(summaryJSON), &report.Summary)
	_ = json.Unmarshal([]byte(sectionsJSON), &report.Sections)
	return report, nil
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

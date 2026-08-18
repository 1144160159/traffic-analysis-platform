package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const (
	encryptedTrafficSnapshotContractVersion  = 1
	encryptedTrafficSnapshotMaxWindow        = 7 * 24 * time.Hour
	encryptedTrafficSnapshotPageSize         = 500
	encryptedTrafficSnapshotMaxOffset        = 100000
	encryptedTrafficSnapshotMaxEvidenceRefs  = 1000
	encryptedTrafficSnapshotMaxResponseBytes = 4 << 20
	encryptedTrafficSnapshotTimeout          = 2 * time.Second
	encryptedTunnelRuleVersion               = "builtin:encrypted-tunnel-indicator-v1"
	encryptedExfiltrationRuleVersion         = "builtin:encrypted-exfiltration-indicator-v1"
)

type EncryptedTrafficSnapshotSection struct {
	Availability    string      `json:"availability"`
	SampleCount     int         `json:"sample_count"`
	Source          string      `json:"source"`
	SourceWatermark string      `json:"source_watermark"`
	RuleVersions    []string    `json:"rule_versions"`
	ModelVersions   []string    `json:"model_versions"`
	Partial         bool        `json:"partial"`
	MissingReasons  []string    `json:"missing_reasons"`
	Facts           interface{} `json:"facts"`
}

type EncryptedTrafficSnapshotData struct {
	SnapshotID           string                          `json:"snapshot_id"`
	TenantID             string                          `json:"tenant_id"`
	AsOf                 string                          `json:"as_of"`
	WindowStart          string                          `json:"window_start"`
	WindowEnd            string                          `json:"window_end"`
	FlowMetadata         EncryptedTrafficSnapshotSection `json:"flow_metadata"`
	PlaintextVisible     EncryptedTrafficSnapshotSection `json:"plaintext_visible"`
	SideChannel          EncryptedTrafficSnapshotSection `json:"side_channel"`
	RawReference         EncryptedTrafficSnapshotSection `json:"raw_reference"`
	RandomnessStatistics EncryptedTrafficSnapshotSection `json:"randomness_statistics"`
	NextContinuation     string                          `json:"next_continuation,omitempty"`
}

type encryptedTrafficSnapshotQuery struct {
	TenantID            string
	Start               time.Time
	End                 time.Time
	AssetID             string
	SessionID           string
	Protocol            string
	Offset              int
	Limit               int
	PcapReadAllowed     bool
	PcapDownloadAllowed bool
}

type encryptedTrafficSnapshotReadResult struct {
	FlowMetadata         EncryptedTrafficSnapshotSection
	PlaintextVisible     EncryptedTrafficSnapshotSection
	SideChannel          EncryptedTrafficSnapshotSection
	RawReference         EncryptedTrafficSnapshotSection
	RandomnessStatistics EncryptedTrafficSnapshotSection
	SourceWatermarks     map[string]string
	MissingSections      []string
	NextOffset           int
}

type encryptedTrafficSnapshotReader interface {
	ReadEncryptedTrafficSnapshot(context.Context, encryptedTrafficSnapshotQuery) encryptedTrafficSnapshotReadResult
}

type encryptedTrafficSnapshotQueryer interface {
	Query(context.Context, string, ...interface{}) (driver.Rows, error)
	QueryRow(context.Context, string, ...interface{}) (driver.Row, error)
}

type EncryptedTrafficSnapshotHandler struct {
	reader     encryptedTrafficSnapshotReader
	enabled    bool
	signingKey []byte
	now        func() time.Time
	logger     *zap.Logger
}

type encryptedTrafficSnapshotProductionReader struct {
	clickhouse encryptedTrafficSnapshotQueryer
	logger     *zap.Logger
}

type encryptedTrafficContinuation struct {
	TenantID  string `json:"tenant_id"`
	StartMS   int64  `json:"start_ms"`
	EndMS     int64  `json:"end_ms"`
	AssetID   string `json:"asset_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	Offset    int    `json:"offset"`
}

func NewEncryptedTrafficSnapshotHandler(
	clickhouse *storage.ClickHouseClient,
	logger *zap.Logger,
	enabled bool,
	signingKey string,
) *EncryptedTrafficSnapshotHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EncryptedTrafficSnapshotHandler{
		reader:  &encryptedTrafficSnapshotProductionReader{clickhouse: clickhouse, logger: logger},
		enabled: enabled, signingKey: []byte(signingKey), now: time.Now, logger: logger,
	}
}

func newEncryptedTrafficSnapshotHandlerForTest(reader encryptedTrafficSnapshotReader, enabled bool, signingKey string) *EncryptedTrafficSnapshotHandler {
	return &EncryptedTrafficSnapshotHandler{
		reader: reader, enabled: enabled, signingKey: []byte(signingKey),
		now: func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }, logger: zap.NewNop(),
	}
}

func (h *EncryptedTrafficSnapshotHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/encrypted-traffic/snapshot", h.GetSnapshot).Methods(http.MethodGet)
}

func (h *EncryptedTrafficSnapshotHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.enabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "encrypted traffic snapshot v1 is disabled")
		return
	}
	tenantID, _, authenticated := authenticatedDashboardIdentity(ctx)
	if !authenticated {
		h.writeError(w, ctx, http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant and user are required")
		return
	}
	if !hasSystemPermission(ctx, authmodel.ScopeAlertRead) {
		h.writeError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: alert:read required")
		return
	}
	if r.URL.Query().Has("tenant_id") {
		h.writeError(w, ctx, http.StatusBadRequest, "TENANT_SOURCE_FORBIDDEN", "tenant_id is derived from authenticated identity")
		return
	}
	if len(h.signingKey) < 32 {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "CURSOR_SIGNING_UNAVAILABLE", "encrypted traffic snapshot continuation signing is unavailable")
		return
	}

	asOf := h.now().UTC().Truncate(time.Millisecond)
	query, err := encryptedTrafficSnapshotRequest(r, tenantID, asOf, h.signingKey)
	if err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}
	query.PcapReadAllowed = hasSystemPermission(ctx, authmodel.ScopePcapRead)
	query.PcapDownloadAllowed = hasSystemPermission(ctx, authmodel.ScopePcapDownload)

	queryCtx, cancel := context.WithTimeout(ctx, encryptedTrafficSnapshotTimeout)
	defer cancel()
	result := h.reader.ReadEncryptedTrafficSnapshot(queryCtx, query)
	if applyEncryptedTrafficEvidenceBudget(&result, encryptedTrafficSnapshotMaxEvidenceRefs) {
		result.MissingSections = append(result.MissingSections, "evidence_references.truncated")
	}
	result.MissingSections = sortedUniqueStrings(result.MissingSections)
	if result.SourceWatermarks == nil {
		result.SourceWatermarks = map[string]string{}
	}
	snapshotID := encryptedTrafficSnapshotID(query, result.SourceWatermarks, result.MissingSections)
	data := EncryptedTrafficSnapshotData{
		SnapshotID: snapshotID, TenantID: tenantID, AsOf: asOf.Format(time.RFC3339Nano),
		WindowStart: query.Start.Format(time.RFC3339Nano), WindowEnd: query.End.Format(time.RFC3339Nano),
		FlowMetadata: result.FlowMetadata, PlaintextVisible: result.PlaintextVisible,
		SideChannel: result.SideChannel, RawReference: result.RawReference,
		RandomnessStatistics: result.RandomnessStatistics,
	}
	if result.NextOffset > 0 {
		next := encryptedTrafficContinuation{
			TenantID: tenantID, StartMS: query.Start.UnixMilli(), EndMS: query.End.UnixMilli(),
			AssetID: query.AssetID, SessionID: query.SessionID, Protocol: query.Protocol, Offset: result.NextOffset,
		}
		data.NextContinuation, err = encodeEncryptedTrafficContinuation(next, h.signingKey)
		if err != nil {
			h.writeError(w, ctx, http.StatusInternalServerError, "SERIALIZATION_FAILED", "failed to encode continuation")
			return
		}
	}
	meta := httpx.ContractMeta{
		ContractVersion: encryptedTrafficSnapshotContractVersion, SnapshotID: snapshotID,
		AsOf: data.AsOf, TraceID: httpx.GetTraceID(ctx), Partial: len(result.MissingSections) > 0,
		MissingSections: result.MissingSections, SourceWatermarks: result.SourceWatermarks,
	}
	encoded, err := json.Marshal(httpx.ContractResponse{
		Success: true, Data: data, Meta: meta, Error: nil,
		Timestamp: asOf.Format(time.RFC3339),
	})
	if err != nil {
		h.writeError(w, ctx, http.StatusInternalServerError, "SERIALIZATION_FAILED", "failed to serialize encrypted traffic snapshot")
		return
	}
	if len(encoded) > encryptedTrafficSnapshotMaxResponseBytes {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "RESPONSE_BUDGET_EXCEEDED", "encrypted traffic snapshot exceeds the 4 MiB response budget")
		return
	}
	h.logger.Info("encrypted traffic snapshot served", zap.String("tenant_id", tenantID), zap.String("snapshot_id", snapshotID),
		zap.Bool("partial", meta.Partial), zap.Strings("missing_sections", meta.MissingSections))
	httpx.JSONContractSuccess(w, ctx, data, meta)
}

func (h *EncryptedTrafficSnapshotHandler) writeError(w http.ResponseWriter, ctx context.Context, status int, code, message string) {
	httpx.NewResponseWriter(w, ctx).Error(status, code, message, nil)
}

func encryptedTrafficSnapshotRequest(r *http.Request, tenantID string, asOf time.Time, signingKey []byte) (encryptedTrafficSnapshotQuery, error) {
	startRaw := strings.TrimSpace(r.URL.Query().Get("start_time"))
	endRaw := strings.TrimSpace(r.URL.Query().Get("end_time"))
	if startRaw == "" || endRaw == "" {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("start_time and end_time are required")
	}
	start, err := parseDashboardSnapshotTime(startRaw)
	if err != nil {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("invalid start_time")
	}
	end, err := parseDashboardSnapshotTime(endRaw)
	if err != nil {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("invalid end_time")
	}
	if start.After(end) {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("start_time must be less than or equal to end_time")
	}
	if end.Sub(start) > encryptedTrafficSnapshotMaxWindow {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("encrypted traffic window must not exceed 7 days")
	}
	if end.After(asOf.Add(time.Minute)) {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("end_time must not be in the future")
	}
	assetID := strings.TrimSpace(r.URL.Query().Get("asset_id"))
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	protocol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("protocol")))
	if len(assetID) > 128 || len(sessionID) > 256 {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("asset_id or session_id is too long")
	}
	if protocol != "" && protocol != "TLS" && protocol != "QUIC" && protocol != "SSH" {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("protocol must be TLS, QUIC or SSH")
	}
	query := encryptedTrafficSnapshotQuery{
		TenantID: tenantID, Start: start, End: end, AssetID: assetID,
		SessionID: sessionID, Protocol: protocol, Limit: encryptedTrafficSnapshotPageSize,
	}
	continuation := strings.TrimSpace(r.URL.Query().Get("continuation"))
	if len(continuation) > 8192 {
		return encryptedTrafficSnapshotQuery{}, fmt.Errorf("continuation is too large")
	}
	if continuation != "" {
		claims, err := decodeEncryptedTrafficContinuation(continuation, signingKey)
		if err != nil {
			return encryptedTrafficSnapshotQuery{}, err
		}
		if claims.TenantID != tenantID || claims.StartMS != start.UnixMilli() || claims.EndMS != end.UnixMilli() ||
			claims.AssetID != assetID || claims.SessionID != sessionID || claims.Protocol != protocol {
			return encryptedTrafficSnapshotQuery{}, fmt.Errorf("continuation does not match the authenticated query")
		}
		if claims.Offset < 1 || claims.Offset > encryptedTrafficSnapshotMaxOffset {
			return encryptedTrafficSnapshotQuery{}, fmt.Errorf("continuation offset is outside the bounded range")
		}
		query.Offset = claims.Offset
	}
	return query, nil
}

func encodeEncryptedTrafficContinuation(claims encryptedTrafficContinuation, signingKey []byte) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func decodeEncryptedTrafficContinuation(token string, signingKey []byte) (encryptedTrafficContinuation, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return encryptedTrafficContinuation{}, fmt.Errorf("invalid continuation")
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return encryptedTrafficContinuation{}, fmt.Errorf("invalid continuation")
	}
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return encryptedTrafficContinuation{}, fmt.Errorf("invalid continuation signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return encryptedTrafficContinuation{}, fmt.Errorf("invalid continuation")
	}
	var claims encryptedTrafficContinuation
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return encryptedTrafficContinuation{}, fmt.Errorf("invalid continuation")
	}
	return claims, nil
}

func encryptedTrafficSnapshotID(query encryptedTrafficSnapshotQuery, watermarks map[string]string, missing []string) string {
	parts := []string{"encrypted-traffic-snapshot-v1", query.TenantID, query.Start.Format(time.RFC3339Nano), query.End.Format(time.RFC3339Nano),
		query.AssetID, query.SessionID, query.Protocol, strconv.Itoa(query.Offset)}
	keys := make([]string, 0, len(watermarks))
	for key := range watermarks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+watermarks[key])
	}
	parts = append(parts, missing...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "encrypted-" + hex.EncodeToString(sum[:16])
}

type encryptedSnapshotSessionRow struct {
	SessionID, CommunityID, EventID, RunID, FeatureSetID, ProbeID string
	SrcIP, DstIP                                                  string
	SrcPort, DstPort                                              uint32
	Protocol                                                      uint8
	TSStart, TSEnd, SourceWatermark                               int64
	DurationMS                                                    uint32
	PacketsFwd, PacketsBwd, NumPackets                            uint32
	BytesFwd, BytesBwd, BytesTotal                                uint64
	UpDownRatio                                                   float32
	AvgPayload, StdPayload, MeanIAT, StdIAT                       float32
	MinPayload, MaxPayload                                        uint32
	MinIAT, MaxIAT                                                float32
	EvidenceCount                                                 uint32
	SourceEventIDs, EvidenceIDs, MissingFields                    []string
	IsPartial                                                     uint8
}

type encryptedSnapshotFingerprintRow struct {
	SessionID, CommunityID, EventID, FeatureSetID                string
	TLSVersion, JA3, JA4, SNI, SNIHash, CertificateSHA256        string
	QUICVersion, TransportSecurity, RawTrafficReference          string
	Availability, SchemaVersion, AlgorithmVersion, MissingReason string
	EventTime, EventTimeStart, EventTimeEnd                      int64
	CertificateSelfSigned                                        uint8
	EntropyPayload, ChiSquareBFD                                 float32
	SourceEventIDs, EvidenceIDs, MissingFields                   []string
}

type encryptedSnapshotPcapRow struct {
	PcapIndexID, FileKey, ProbeID, ObjectVersion, RawSHA256 string
	CommunityID, FlowID                                     string
	StartTime, EndTime                                      int64
	OffsetStart, OffsetEnd                                  uint64
}

type EncryptedTrafficFlowMetadataFact struct {
	SessionID      string   `json:"session_id"`
	CommunityID    string   `json:"community_id"`
	Transport      string   `json:"transport"`
	TLSVersion     *string  `json:"tls_version"`
	CipherSuite    *string  `json:"cipher_suite"`
	ALPN           *string  `json:"alpn"`
	Direction      string   `json:"direction"`
	SrcIP          string   `json:"src_ip"`
	DstIP          string   `json:"dst_ip"`
	SrcPort        uint32   `json:"src_port"`
	DstPort        uint32   `json:"dst_port"`
	Bytes          uint64   `json:"bytes"`
	Packets        uint32   `json:"packets"`
	StartTime      int64    `json:"start_time"`
	EndTime        int64    `json:"end_time"`
	FeatureSetID   string   `json:"feature_set_id,omitempty"`
	ProbeID        string   `json:"probe_id,omitempty"`
	SourceEventIDs []string `json:"source_event_ids"`
	EvidenceRefs   []string `json:"evidence_refs"`
	Partial        bool     `json:"partial"`
	MissingFields  []string `json:"missing_fields"`
}

type EncryptedTrafficPlaintextFact struct {
	SessionID             string   `json:"session_id"`
	SNI                   string   `json:"sni,omitempty"`
	SNIHash               string   `json:"sni_hash,omitempty"`
	CertificateSHA256     string   `json:"certificate_sha256,omitempty"`
	CertificateSubject    *string  `json:"certificate_subject"`
	CertificateIssuer     *string  `json:"certificate_issuer"`
	CertificateValidity   *string  `json:"certificate_validity"`
	CertificateSelfSigned bool     `json:"certificate_self_signed"`
	JA3                   string   `json:"ja3,omitempty"`
	JA3S                  *string  `json:"ja3s"`
	JA4                   string   `json:"ja4,omitempty"`
	TLSVersion            string   `json:"tls_version,omitempty"`
	QUICVersion           string   `json:"quic_version,omitempty"`
	TransportSecurity     string   `json:"transport_security,omitempty"`
	SourceEventIDs        []string `json:"source_event_ids"`
	EvidenceRefs          []string `json:"evidence_refs"`
	Limitations           []string `json:"limitations"`
}

type EncryptedTrafficDistribution struct {
	Mean float64 `json:"mean"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Std  float64 `json:"std"`
	Unit string  `json:"unit"`
}

type EncryptedTrafficIndicator struct {
	IndicatorID  string   `json:"indicator_id"`
	Observed     bool     `json:"observed"`
	RuleVersion  string   `json:"rule_version"`
	Contribution string   `json:"contribution"`
	Limitation   string   `json:"limitation"`
	EvidenceRefs []string `json:"evidence_refs"`
	Verdict      string   `json:"verdict"`
}

type EncryptedTrafficSideChannelFact struct {
	SessionID                string                       `json:"session_id"`
	PacketLengthDistribution EncryptedTrafficDistribution `json:"packet_length_distribution"`
	InterArrivalDistribution EncryptedTrafficDistribution `json:"inter_arrival_distribution"`
	BurstShape               map[string]interface{}       `json:"burst_shape"`
	DirectionRatio           float64                      `json:"direction_ratio"`
	TunnelIndicators         []EncryptedTrafficIndicator  `json:"tunnel_indicators"`
	ExfiltrationIndicators   []EncryptedTrafficIndicator  `json:"exfiltration_indicators"`
}

type EncryptedTrafficRawReferenceFact struct {
	SessionID       string   `json:"session_id"`
	CommunityID     string   `json:"community_id"`
	EventIDs        []string `json:"event_ids"`
	EvidenceRefs    []string `json:"evidence_refs"`
	PcapIndexIDs    []string `json:"pcap_index_ids"`
	PcapFileKeys    []string `json:"pcap_file_keys"`
	DownloadAllowed bool     `json:"download_allowed"`
}

type EncryptedTrafficRandomnessFact struct {
	SessionID           string   `json:"session_id"`
	EntropyValue        *float64 `json:"entropy_value"`
	ChiSquareValue      *float64 `json:"chi_square_value"`
	SampleBytes         *uint64  `json:"sample_bytes"`
	MethodID            string   `json:"method_id"`
	MethodVersion       string   `json:"method_version"`
	Computability       string   `json:"computability"`
	MissingReason       string   `json:"missing_reason"`
	LegacyObservedValue *float64 `json:"legacy_observed_value,omitempty"`
	SourceEventIDs      []string `json:"source_event_ids"`
	EvidenceRefs        []string `json:"evidence_refs"`
}

func (r *encryptedTrafficSnapshotProductionReader) ReadEncryptedTrafficSnapshot(ctx context.Context, query encryptedTrafficSnapshotQuery) encryptedTrafficSnapshotReadResult {
	result := encryptedTrafficSnapshotReadResult{SourceWatermarks: map[string]string{}, MissingSections: []string{}}
	if r == nil || r.clickhouse == nil {
		reason := "clickhouse_client_unavailable"
		result.FlowMetadata = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions", reason)
		result.PlaintextVisible = encryptedSnapshotUnavailableSection("clickhouse.traffic.feature_fp", reason)
		result.SideChannel = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions", reason)
		result.RandomnessStatistics = encryptedSnapshotUnavailableSection("clickhouse.traffic.feature_fp", reason)
		if query.PcapReadAllowed {
			result.RawReference = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions+traffic.pcap_index_v2", reason)
		} else {
			result.RawReference = encryptedSnapshotForbiddenSection("pcap:read_required")
		}
		result.MissingSections = []string{"flow_metadata", "plaintext_visible", "side_channel", "randomness_statistics", "raw_reference"}
		return result
	}

	sessions, totalSessions, sessionWatermark, nextOffset, err := r.readSessions(ctx, query)
	if err != nil {
		r.logger.Warn("encrypted snapshot session source unavailable", zap.String("tenant_id", query.TenantID), zap.Error(err))
		reason := "clickhouse_sessions_query_failed"
		result.FlowMetadata = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions", reason)
		result.SideChannel = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions", reason)
		result.PlaintextVisible = encryptedSnapshotUnavailableSection("clickhouse.traffic.feature_fp", "session_scope_unavailable")
		result.RandomnessStatistics = encryptedSnapshotUnavailableSection("clickhouse.traffic.feature_fp", "session_scope_unavailable")
		if query.PcapReadAllowed {
			result.RawReference = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions+traffic.pcap_index_v2", "session_scope_unavailable")
		} else {
			result.RawReference = encryptedSnapshotForbiddenSection("pcap:read_required")
		}
		result.MissingSections = []string{"flow_metadata", "plaintext_visible", "side_channel", "randomness_statistics", "raw_reference"}
		return result
	}
	result.NextOffset = nextOffset
	result.SourceWatermarks["clickhouse.sessions"] = sessionWatermark
	result.FlowMetadata = buildEncryptedFlowMetadataSection(sessions, totalSessions, sessionWatermark)
	result.SideChannel = buildEncryptedSideChannelSection(sessions, totalSessions, sessionWatermark)
	if result.FlowMetadata.Partial {
		result.MissingSections = append(result.MissingSections, "flow_metadata.cipher_suite", "flow_metadata.alpn")
	}
	if result.SideChannel.Partial {
		result.MissingSections = append(result.MissingSections, "side_channel.partial_sessions")
	}

	sessionIDs := make([]string, 0, len(sessions))
	communityIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.SessionID)
		if session.CommunityID != "" {
			communityIDs = append(communityIDs, session.CommunityID)
		}
	}
	fingerprints, fingerprintWatermark, fingerprintErr := r.readFingerprints(ctx, query, sessionIDs)
	if fingerprintErr != nil {
		r.logger.Warn("encrypted snapshot fingerprint source unavailable", zap.String("tenant_id", query.TenantID), zap.Error(fingerprintErr))
		result.PlaintextVisible = encryptedSnapshotUnavailableSection("clickhouse.traffic.feature_fp", "feature_fp_query_failed")
		result.RandomnessStatistics = encryptedSnapshotUnavailableSection("clickhouse.traffic.feature_fp", "feature_fp_query_failed")
		result.MissingSections = append(result.MissingSections, "plaintext_visible", "randomness_statistics")
	} else {
		result.SourceWatermarks["clickhouse.feature_fp"] = fingerprintWatermark
		applyEncryptedHandshakeMetadata(&result.FlowMetadata, fingerprints)
		result.PlaintextVisible = buildEncryptedPlaintextSection(fingerprints, fingerprintWatermark)
		result.RandomnessStatistics = buildEncryptedRandomnessSection(fingerprints, fingerprintWatermark)
		if result.PlaintextVisible.Partial {
			result.MissingSections = append(result.MissingSections, "plaintext_visible.certificate_identity", "plaintext_visible.ja3s")
		}
		if result.RandomnessStatistics.Partial {
			result.MissingSections = append(result.MissingSections, "randomness_statistics.sample_bytes")
		}
	}

	if !query.PcapReadAllowed {
		result.RawReference = encryptedSnapshotForbiddenSection("pcap:read_required")
		result.MissingSections = append(result.MissingSections, "raw_reference.permission")
		return result
	}
	pcaps, pcapWatermark, pcapErr := r.readPcapReferences(ctx, query, communityIDs)
	if pcapErr != nil {
		r.logger.Warn("encrypted snapshot pcap source unavailable", zap.String("tenant_id", query.TenantID), zap.Error(pcapErr))
		result.RawReference = encryptedSnapshotUnavailableSection("clickhouse.traffic.sessions+traffic.pcap_index_v2", "pcap_index_v2_query_failed")
		result.MissingSections = append(result.MissingSections, "raw_reference.pcap_index")
		return result
	}
	result.SourceWatermarks["clickhouse.pcap_index_v2"] = pcapWatermark
	result.RawReference = buildEncryptedRawReferenceSection(sessions, pcaps, sessionWatermark, pcapWatermark, query.PcapDownloadAllowed)
	return result
}

func (r *encryptedTrafficSnapshotProductionReader) readSessions(ctx context.Context, query encryptedTrafficSnapshotQuery) ([]encryptedSnapshotSessionRow, uint64, string, int, error) {
	columns, err := r.readEncryptedSnapshotTableColumns(ctx, "sessions")
	if err != nil {
		return nil, 0, "", 0, err
	}
	where, args := encryptedSnapshotSessionWhere(query)
	var total, identityXOR uint64
	var maxEnd int64
	row, err := r.clickhouse.QueryRow(ctx, `SELECT count(),ifNull(max(ts_end),0),
		groupBitXor(cityHash64(concat(session_id,':',toString(ts_end),':',toString(bytes_total))))
		FROM traffic.sessions WHERE `+where, args...)
	if err != nil {
		return nil, 0, "", 0, err
	}
	if err := row.Scan(&total, &maxEnd, &identityXOR); err != nil {
		return nil, 0, "", 0, err
	}
	rowsArgs := append(append([]interface{}{}, args...), query.Limit+1, query.Offset)
	rows, err := r.clickhouse.Query(ctx, fmt.Sprintf(`SELECT session_id,community_id,event_id,run_id,feature_set_id,probe_id,
		src_ip,dst_ip,src_port,dst_port,protocol,ts_start,ts_end,duration_ms,
		packets_fwd,packets_bwd,bytes_fwd,bytes_bwd,bytes_total,up_down_ratio,num_pkts,
		avg_payload,min_payload,max_payload,std_payload,mean_iat_ms,min_iat_ms,max_iat_ms,std_iat_ms,
		evidence_count,%s,%s,%s,%s,%s
		FROM traffic.sessions WHERE `+where+`
		ORDER BY ts_end DESC,session_id ASC LIMIT ? OFFSET ?`,
		encryptedSnapshotArrayColumn(columns, "source_event_ids"),
		encryptedSnapshotArrayColumn(columns, "evidence_ids"),
		encryptedSnapshotSessionPartialExpression(columns),
		encryptedSnapshotMissingFieldsExpression(columns, []string{"source_event_ids", "evidence_ids", "is_partial", "source_watermark_ms", "event_time_end_ms"}),
		encryptedSnapshotWatermarkExpression(columns)), rowsArgs...)
	if err != nil {
		return nil, 0, "", 0, err
	}
	defer rows.Close()
	items := make([]encryptedSnapshotSessionRow, 0, query.Limit+1)
	for rows.Next() {
		var item encryptedSnapshotSessionRow
		if err := rows.Scan(&item.SessionID, &item.CommunityID, &item.EventID, &item.RunID, &item.FeatureSetID, &item.ProbeID,
			&item.SrcIP, &item.DstIP, &item.SrcPort, &item.DstPort, &item.Protocol, &item.TSStart, &item.TSEnd, &item.DurationMS,
			&item.PacketsFwd, &item.PacketsBwd, &item.BytesFwd, &item.BytesBwd, &item.BytesTotal, &item.UpDownRatio, &item.NumPackets,
			&item.AvgPayload, &item.MinPayload, &item.MaxPayload, &item.StdPayload, &item.MeanIAT, &item.MinIAT, &item.MaxIAT, &item.StdIAT,
			&item.EvidenceCount, &item.SourceEventIDs, &item.EvidenceIDs, &item.IsPartial, &item.MissingFields, &item.SourceWatermark); err != nil {
			return nil, 0, "", 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", 0, err
	}
	nextOffset := 0
	if len(items) > query.Limit {
		items = items[:query.Limit]
		nextOffset = query.Offset + query.Limit
		if nextOffset > encryptedTrafficSnapshotMaxOffset {
			nextOffset = 0
		}
	}
	watermark := encryptedSnapshotWatermark("sessions-v1", query, total, maxEnd, identityXOR)
	return items, total, watermark, nextOffset, nil
}

func encryptedSnapshotSessionWhere(query encryptedTrafficSnapshotQuery) (string, []interface{}) {
	conditions := []string{"tenant_id=?", "ts_start>=?", "ts_start<=?", "dst_port IN (443,8443,853,993,995,465,22)"}
	args := []interface{}{query.TenantID, query.Start.UnixMilli(), query.End.UnixMilli()}
	if query.AssetID != "" {
		conditions = append(conditions, "(src_ip=? OR dst_ip=?)")
		args = append(args, query.AssetID, query.AssetID)
	}
	if query.SessionID != "" {
		conditions = append(conditions, "session_id=?")
		args = append(args, query.SessionID)
	}
	switch query.Protocol {
	case "TLS":
		conditions = append(conditions, "protocol!=17", "dst_port IN (443,8443,853,993,995,465)")
	case "QUIC":
		conditions = append(conditions, "protocol=17", "dst_port IN (443,8443)")
	case "SSH":
		conditions = append(conditions, "dst_port=22")
	}
	return strings.Join(conditions, " AND "), args
}

func (r *encryptedTrafficSnapshotProductionReader) readEncryptedSnapshotTableColumns(ctx context.Context, table string) (map[string]bool, error) {
	rows, err := r.clickhouse.Query(ctx, `SELECT name FROM system.columns WHERE database='traffic' AND table=? ORDER BY name`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func encryptedSnapshotStringColumn(columns map[string]bool, name string) string {
	if columns[name] {
		return name
	}
	return "CAST('', 'String')"
}

func encryptedSnapshotInt64Column(columns map[string]bool, name string) string {
	if columns[name] {
		return name
	}
	return "toInt64(0)"
}

func encryptedSnapshotArrayColumn(columns map[string]bool, name string) string {
	if columns[name] {
		return name
	}
	return "CAST([], 'Array(String)')"
}

func encryptedSnapshotSessionPartialExpression(columns map[string]bool) string {
	for _, name := range []string{"source_event_ids", "evidence_ids", "is_partial", "missing_fields", "source_watermark_ms", "event_time_end_ms"} {
		if !columns[name] {
			return "toUInt8(1)"
		}
	}
	return "is_partial"
}

func encryptedSnapshotMissingFieldsExpression(columns map[string]bool, optional []string) string {
	base := "CAST([], 'Array(String)')"
	if columns["missing_fields"] {
		base = "missing_fields"
	}
	missing := make([]string, 0, len(optional)+1)
	for _, name := range optional {
		if !columns[name] {
			missing = append(missing, "'"+name+"_not_persisted'")
		}
	}
	if !columns["missing_fields"] {
		missing = append(missing, "'missing_fields_not_persisted'")
	}
	if len(missing) == 0 {
		return base
	}
	return "arrayConcat(" + base + ",CAST([" + strings.Join(missing, ",") + "], 'Array(String)'))"
}

func encryptedSnapshotWatermarkExpression(columns map[string]bool) string {
	switch {
	case columns["source_watermark_ms"] && columns["event_time_end_ms"]:
		return "ifNull(source_watermark_ms,event_time_end_ms)"
	case columns["source_watermark_ms"]:
		return "ifNull(source_watermark_ms,ts_end)"
	case columns["event_time_end_ms"]:
		return "event_time_end_ms"
	default:
		return "ts_end"
	}
}

func (r *encryptedTrafficSnapshotProductionReader) readFingerprints(ctx context.Context, query encryptedTrafficSnapshotQuery, sessionIDs []string) ([]encryptedSnapshotFingerprintRow, string, error) {
	if len(sessionIDs) == 0 {
		return []encryptedSnapshotFingerprintRow{}, encryptedSnapshotEmptyWatermark("feature-fp-v1", query), nil
	}
	columns, err := r.readEncryptedSnapshotTableColumns(ctx, "feature_fp")
	if err != nil {
		return nil, "", err
	}
	rows, err := r.clickhouse.Query(ctx, fmt.Sprintf(`SELECT session_id,community_id,event_id,feature_set_id,
		tls_version,ja3,%s,%s,sni_hash,cert_sha256,cert_is_self_signed,%s,%s,%s,
		entropy_payload,chi_square_bfd,%s,%s,%s,
		toUnixTimestamp64Milli(ts),%s,%s,%s,%s,%s,%s
		FROM traffic.feature_fp
		WHERE tenant_id=? AND toUnixTimestamp64Milli(ts)>=? AND toUnixTimestamp64Milli(ts)<=? AND session_id IN ?
		ORDER BY ts DESC LIMIT 1 BY session_id LIMIT ?`,
		encryptedSnapshotStringColumn(columns, "ja4"),
		encryptedSnapshotStringColumn(columns, "sni"),
		encryptedSnapshotStringColumn(columns, "quic_version"),
		encryptedSnapshotStringColumn(columns, "transport_security"),
		encryptedSnapshotStringColumn(columns, "raw_traffic_ref"),
		encryptedSnapshotStringColumn(columns, "availability"),
		encryptedSnapshotStringColumn(columns, "schema_version"),
		encryptedSnapshotStringColumn(columns, "algorithm_version"),
		encryptedSnapshotInt64Column(columns, "event_time_start_ms"),
		encryptedSnapshotInt64Column(columns, "event_time_end_ms"),
		encryptedSnapshotArrayColumn(columns, "source_event_ids"),
		encryptedSnapshotArrayColumn(columns, "evidence_ids"),
		encryptedSnapshotMissingFieldsExpression(columns, []string{"ja4", "sni", "quic_version", "transport_security", "raw_traffic_ref", "availability", "schema_version", "algorithm_version", "event_time_start_ms", "event_time_end_ms", "source_event_ids", "evidence_ids", "missing_reason"}),
		encryptedSnapshotStringColumn(columns, "missing_reason")), query.TenantID, query.Start.UnixMilli(), query.End.UnixMilli(), sessionIDs, query.Limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]encryptedSnapshotFingerprintRow, 0, len(sessionIDs))
	var maxEvent int64
	var identityXOR uint64
	for rows.Next() {
		var item encryptedSnapshotFingerprintRow
		if err := rows.Scan(&item.SessionID, &item.CommunityID, &item.EventID, &item.FeatureSetID,
			&item.TLSVersion, &item.JA3, &item.JA4, &item.SNI, &item.SNIHash, &item.CertificateSHA256,
			&item.CertificateSelfSigned, &item.QUICVersion, &item.TransportSecurity, &item.RawTrafficReference,
			&item.EntropyPayload, &item.ChiSquareBFD, &item.Availability, &item.SchemaVersion, &item.AlgorithmVersion,
			&item.EventTime, &item.EventTimeStart, &item.EventTimeEnd, &item.SourceEventIDs, &item.EvidenceIDs,
			&item.MissingFields, &item.MissingReason); err != nil {
			return nil, "", err
		}
		if item.EventTime > maxEvent {
			maxEvent = item.EventTime
		}
		identityXOR ^= stableEncryptedIdentity(item.SessionID, item.EventID, strconv.FormatInt(item.EventTime, 10))
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return items, encryptedSnapshotWatermark("feature-fp-v1", query, uint64(len(items)), maxEvent, identityXOR), nil
}

func (r *encryptedTrafficSnapshotProductionReader) readPcapReferences(ctx context.Context, query encryptedTrafficSnapshotQuery, communityIDs []string) ([]encryptedSnapshotPcapRow, string, error) {
	if len(communityIDs) == 0 {
		return []encryptedSnapshotPcapRow{}, encryptedSnapshotEmptyWatermark("pcap-index-v2", query), nil
	}
	rows, err := r.clickhouse.Query(ctx, `SELECT projection_identity,file_key,probe_id,object_version,raw_sha256,
		community_id,flow_id,toUnixTimestamp64Milli(ts_start),toUnixTimestamp64Milli(ts_end),
		ifNull(offset_start,0),ifNull(offset_end,0)
		FROM traffic.pcap_index_v2
		WHERE tenant_id=? AND ts_start>=? AND ts_start<=? AND community_id IN ?
		ORDER BY ts_end DESC,projection_identity ASC LIMIT ?`, query.TenantID, query.Start, query.End, communityIDs, query.Limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]encryptedSnapshotPcapRow, 0)
	var maxEnd int64
	var identityXOR uint64
	for rows.Next() {
		var item encryptedSnapshotPcapRow
		if err := rows.Scan(&item.PcapIndexID, &item.FileKey, &item.ProbeID, &item.ObjectVersion, &item.RawSHA256,
			&item.CommunityID, &item.FlowID, &item.StartTime, &item.EndTime, &item.OffsetStart, &item.OffsetEnd); err != nil {
			return nil, "", err
		}
		if item.EndTime > maxEnd {
			maxEnd = item.EndTime
		}
		identityXOR ^= stableEncryptedIdentity(item.PcapIndexID, item.RawSHA256, item.ObjectVersion)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return items, encryptedSnapshotWatermark("pcap-index-v2", query, uint64(len(items)), maxEnd, identityXOR), nil
}

func buildEncryptedFlowMetadataSection(sessions []encryptedSnapshotSessionRow, total uint64, watermark string) EncryptedTrafficSnapshotSection {
	facts := make([]EncryptedTrafficFlowMetadataFact, 0, len(sessions))
	missing := []string{}
	partial := false
	for _, session := range sessions {
		rowMissing := append([]string{}, session.MissingFields...)
		rowMissing = append(rowMissing, "cipher_suite_not_persisted", "alpn_not_persisted")
		rowMissing = sortedUniqueStrings(rowMissing)
		if session.IsPartial != 0 || len(rowMissing) > 0 {
			partial = true
		}
		facts = append(facts, EncryptedTrafficFlowMetadataFact{
			SessionID: session.SessionID, CommunityID: session.CommunityID,
			Transport:  encryptedProtocol(session.Protocol, session.DstPort),
			TLSVersion: nil, CipherSuite: nil, ALPN: nil,
			Direction: encryptedSnapshotDirection(session.BytesFwd, session.BytesBwd),
			SrcIP:     session.SrcIP, DstIP: session.DstIP, SrcPort: session.SrcPort, DstPort: session.DstPort,
			Bytes: session.BytesTotal, Packets: session.NumPackets, StartTime: session.TSStart, EndTime: session.TSEnd,
			FeatureSetID: session.FeatureSetID, ProbeID: session.ProbeID,
			SourceEventIDs: sortedUniqueStrings(append([]string{session.EventID}, session.SourceEventIDs...)),
			EvidenceRefs:   sortedUniqueStrings(session.EvidenceIDs), Partial: session.IsPartial != 0 || len(rowMissing) > 0,
			MissingFields: rowMissing,
		})
	}
	availability := "available"
	if total == 0 {
		availability = "no_sample"
	}
	if partial {
		missing = append(missing, "cipher_suite_not_persisted", "alpn_not_persisted")
	}
	return EncryptedTrafficSnapshotSection{
		Availability: availability, SampleCount: len(facts), Source: "clickhouse.traffic.sessions",
		SourceWatermark: watermark, RuleVersions: []string{}, ModelVersions: []string{},
		Partial: partial, MissingReasons: sortedUniqueStrings(missing), Facts: facts,
	}
}

func buildEncryptedSideChannelSection(sessions []encryptedSnapshotSessionRow, total uint64, watermark string) EncryptedTrafficSnapshotSection {
	facts := make([]EncryptedTrafficSideChannelFact, 0, len(sessions))
	partial := false
	for _, session := range sessions {
		if session.IsPartial != 0 {
			partial = true
		}
		tunnelObserved := session.DstPort == 22 || session.DstPort == 853 || session.DstPort == 8443
		exfilObserved := session.BytesFwd >= 10*1024*1024 && session.BytesFwd > session.BytesBwd*4
		evidence := sortedUniqueStrings(append(append([]string{}, session.EvidenceIDs...), session.SourceEventIDs...))
		facts = append(facts, EncryptedTrafficSideChannelFact{
			SessionID: session.SessionID,
			PacketLengthDistribution: EncryptedTrafficDistribution{
				Mean: float64(session.AvgPayload), Min: float64(session.MinPayload), Max: float64(session.MaxPayload),
				Std: float64(session.StdPayload), Unit: "bytes",
			},
			InterArrivalDistribution: EncryptedTrafficDistribution{
				Mean: float64(session.MeanIAT), Min: float64(session.MinIAT), Max: float64(session.MaxIAT),
				Std: float64(session.StdIAT), Unit: "milliseconds",
			},
			BurstShape: map[string]interface{}{
				"duration_ms": session.DurationMS, "packets_forward": session.PacketsFwd,
				"packets_backward": session.PacketsBwd, "bytes_forward": session.BytesFwd, "bytes_backward": session.BytesBwd,
			},
			DirectionRatio: float64(session.UpDownRatio),
			TunnelIndicators: []EncryptedTrafficIndicator{{
				IndicatorID: "tunnel-associated-port", Observed: tunnelObserved, RuleVersion: encryptedTunnelRuleVersion,
				Contribution: fmt.Sprintf("destination_port=%d transport=%s", session.DstPort, encryptedProtocol(session.Protocol, session.DstPort)),
				Limitation:   "a tunnel-associated port is a routing hint, not proof of tunneling or malicious behavior",
				EvidenceRefs: evidence, Verdict: "indicator_only",
			}},
			ExfiltrationIndicators: []EncryptedTrafficIndicator{{
				IndicatorID: "large-directional-upload", Observed: exfilObserved, RuleVersion: encryptedExfiltrationRuleVersion,
				Contribution: fmt.Sprintf("bytes_forward=%d bytes_backward=%d", session.BytesFwd, session.BytesBwd),
				Limitation:   "volume and direction alone do not establish data exfiltration",
				EvidenceRefs: evidence, Verdict: "indicator_only",
			}},
		})
	}
	availability := "available"
	if total == 0 {
		availability = "no_sample"
	}
	missing := []string{}
	if partial {
		missing = append(missing, "source_session_partial")
	}
	return EncryptedTrafficSnapshotSection{
		Availability: availability, SampleCount: len(facts), Source: "clickhouse.traffic.sessions",
		SourceWatermark: watermark, RuleVersions: []string{encryptedExfiltrationRuleVersion, encryptedTunnelRuleVersion},
		ModelVersions: []string{}, Partial: partial, MissingReasons: missing, Facts: facts,
	}
}

func buildEncryptedPlaintextSection(fingerprints []encryptedSnapshotFingerprintRow, watermark string) EncryptedTrafficSnapshotSection {
	facts := make([]EncryptedTrafficPlaintextFact, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		limitations := append([]string{}, fingerprint.MissingFields...)
		limitations = append(limitations, "ja3s_not_persisted", "certificate_subject_not_persisted",
			"certificate_issuer_not_persisted", "certificate_validity_not_persisted")
		if fingerprint.MissingReason != "" {
			limitations = append(limitations, fingerprint.MissingReason)
		}
		facts = append(facts, EncryptedTrafficPlaintextFact{
			SessionID: fingerprint.SessionID, SNI: fingerprint.SNI, SNIHash: fingerprint.SNIHash,
			CertificateSHA256: fingerprint.CertificateSHA256, CertificateSubject: nil, CertificateIssuer: nil,
			CertificateValidity: nil, CertificateSelfSigned: fingerprint.CertificateSelfSigned != 0,
			JA3: fingerprint.JA3, JA3S: nil, JA4: fingerprint.JA4, TLSVersion: fingerprint.TLSVersion,
			QUICVersion: fingerprint.QUICVersion, TransportSecurity: fingerprint.TransportSecurity,
			SourceEventIDs: sortedUniqueStrings(append([]string{fingerprint.EventID}, fingerprint.SourceEventIDs...)),
			EvidenceRefs:   sortedUniqueStrings(fingerprint.EvidenceIDs), Limitations: sortedUniqueStrings(limitations),
		})
	}
	availability := "available"
	partial := len(facts) > 0
	missing := []string{"ja3s_not_persisted", "certificate_identity_not_persisted"}
	if len(facts) == 0 {
		availability = "no_sample"
		partial = false
		missing = []string{}
	}
	return EncryptedTrafficSnapshotSection{
		Availability: availability, SampleCount: len(facts), Source: "clickhouse.traffic.feature_fp",
		SourceWatermark: watermark, RuleVersions: []string{}, ModelVersions: []string{},
		Partial: partial, MissingReasons: missing, Facts: facts,
	}
}

func buildEncryptedRandomnessSection(fingerprints []encryptedSnapshotFingerprintRow, watermark string) EncryptedTrafficSnapshotSection {
	facts := make([]EncryptedTrafficRandomnessFact, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		legacyEntropy := float64(fingerprint.EntropyPayload)
		methodVersion := strings.TrimSpace(fingerprint.AlgorithmVersion)
		if methodVersion == "" {
			methodVersion = strings.TrimSpace(fingerprint.SchemaVersion)
		}
		if methodVersion == "" {
			methodVersion = "unversioned_legacy_observation"
		}
		facts = append(facts, EncryptedTrafficRandomnessFact{
			SessionID: fingerprint.SessionID, EntropyValue: nil, ChiSquareValue: nil, SampleBytes: nil,
			MethodID: "feature_fp.entropy_payload", MethodVersion: methodVersion,
			Computability: "not_computable", MissingReason: "sample_bytes_not_persisted",
			LegacyObservedValue: &legacyEntropy,
			SourceEventIDs:      sortedUniqueStrings(append([]string{fingerprint.EventID}, fingerprint.SourceEventIDs...)),
			EvidenceRefs:        sortedUniqueStrings(fingerprint.EvidenceIDs),
		})
	}
	if len(facts) == 0 {
		return EncryptedTrafficSnapshotSection{
			Availability: "no_sample", SampleCount: 0, Source: "clickhouse.traffic.feature_fp",
			SourceWatermark: watermark, RuleVersions: []string{}, ModelVersions: []string{},
			Partial: false, MissingReasons: []string{}, Facts: facts,
		}
	}
	return EncryptedTrafficSnapshotSection{
		Availability: "not_computable", SampleCount: len(facts), Source: "clickhouse.traffic.feature_fp",
		SourceWatermark: watermark, RuleVersions: []string{}, ModelVersions: []string{},
		Partial: true, MissingReasons: []string{"sample_bytes_not_persisted"}, Facts: facts,
	}
}

func applyEncryptedHandshakeMetadata(section *EncryptedTrafficSnapshotSection, fingerprints []encryptedSnapshotFingerprintRow) {
	if section == nil {
		return
	}
	facts, ok := section.Facts.([]EncryptedTrafficFlowMetadataFact)
	if !ok {
		return
	}
	bySession := make(map[string]encryptedSnapshotFingerprintRow, len(fingerprints))
	for _, fingerprint := range fingerprints {
		bySession[fingerprint.SessionID] = fingerprint
	}
	for index := range facts {
		fingerprint, found := bySession[facts[index].SessionID]
		if !found {
			if facts[index].Transport == "TLS" || facts[index].Transport == "QUIC" {
				facts[index].MissingFields = sortedUniqueStrings(append(facts[index].MissingFields, "handshake_metadata_not_available"))
				facts[index].Partial = true
				section.Partial = true
				section.MissingReasons = sortedUniqueStrings(append(section.MissingReasons, "handshake_metadata_not_available"))
			}
			continue
		}
		if facts[index].Transport == "TLS" && strings.TrimSpace(fingerprint.TLSVersion) != "" {
			value := fingerprint.TLSVersion
			facts[index].TLSVersion = &value
		}
	}
	section.Facts = facts
}

func applyEncryptedTrafficEvidenceBudget(result *encryptedTrafficSnapshotReadResult, remaining int) bool {
	if result == nil || remaining < 0 {
		return false
	}
	truncated := false
	take := func(values []string) ([]string, bool) {
		values = sortedUniqueStrings(values)
		if len(values) <= remaining {
			remaining -= len(values)
			return values, false
		}
		truncated = truncated || len(values) > 0
		kept := append([]string{}, values[:remaining]...)
		remaining = 0
		return kept, true
	}
	mark := func(section *EncryptedTrafficSnapshotSection) {
		section.Partial = true
		section.MissingReasons = sortedUniqueStrings(append(section.MissingReasons, "evidence_reference_budget_exhausted"))
	}
	if facts, ok := result.FlowMetadata.Facts.([]EncryptedTrafficFlowMetadataFact); ok {
		sectionTruncated := false
		for index := range facts {
			var itemTruncated bool
			facts[index].EvidenceRefs, itemTruncated = take(facts[index].EvidenceRefs)
			sectionTruncated = sectionTruncated || itemTruncated
		}
		result.FlowMetadata.Facts = facts
		if sectionTruncated {
			mark(&result.FlowMetadata)
		}
	}
	if facts, ok := result.PlaintextVisible.Facts.([]EncryptedTrafficPlaintextFact); ok {
		sectionTruncated := false
		for index := range facts {
			var itemTruncated bool
			facts[index].EvidenceRefs, itemTruncated = take(facts[index].EvidenceRefs)
			sectionTruncated = sectionTruncated || itemTruncated
		}
		result.PlaintextVisible.Facts = facts
		if sectionTruncated {
			mark(&result.PlaintextVisible)
		}
	}
	if facts, ok := result.SideChannel.Facts.([]EncryptedTrafficSideChannelFact); ok {
		sectionTruncated := false
		for factIndex := range facts {
			for indicatorIndex := range facts[factIndex].TunnelIndicators {
				var itemTruncated bool
				facts[factIndex].TunnelIndicators[indicatorIndex].EvidenceRefs, itemTruncated = take(facts[factIndex].TunnelIndicators[indicatorIndex].EvidenceRefs)
				sectionTruncated = sectionTruncated || itemTruncated
			}
			for indicatorIndex := range facts[factIndex].ExfiltrationIndicators {
				var itemTruncated bool
				facts[factIndex].ExfiltrationIndicators[indicatorIndex].EvidenceRefs, itemTruncated = take(facts[factIndex].ExfiltrationIndicators[indicatorIndex].EvidenceRefs)
				sectionTruncated = sectionTruncated || itemTruncated
			}
		}
		result.SideChannel.Facts = facts
		if sectionTruncated {
			mark(&result.SideChannel)
		}
	}
	if facts, ok := result.RawReference.Facts.([]EncryptedTrafficRawReferenceFact); ok {
		sectionTruncated := false
		for index := range facts {
			var itemTruncated bool
			facts[index].EvidenceRefs, itemTruncated = take(facts[index].EvidenceRefs)
			sectionTruncated = sectionTruncated || itemTruncated
		}
		result.RawReference.Facts = facts
		if sectionTruncated {
			mark(&result.RawReference)
		}
	}
	if facts, ok := result.RandomnessStatistics.Facts.([]EncryptedTrafficRandomnessFact); ok {
		sectionTruncated := false
		for index := range facts {
			var itemTruncated bool
			facts[index].EvidenceRefs, itemTruncated = take(facts[index].EvidenceRefs)
			sectionTruncated = sectionTruncated || itemTruncated
		}
		result.RandomnessStatistics.Facts = facts
		if sectionTruncated {
			mark(&result.RandomnessStatistics)
		}
	}
	return truncated
}

func buildEncryptedRawReferenceSection(sessions []encryptedSnapshotSessionRow, pcaps []encryptedSnapshotPcapRow, sessionWatermark, pcapWatermark string, downloadAllowed bool) EncryptedTrafficSnapshotSection {
	pcapsByCommunity := make(map[string][]encryptedSnapshotPcapRow)
	for _, pcap := range pcaps {
		pcapsByCommunity[pcap.CommunityID] = append(pcapsByCommunity[pcap.CommunityID], pcap)
	}
	facts := make([]EncryptedTrafficRawReferenceFact, 0, len(sessions))
	for _, session := range sessions {
		pcapIndexIDs := []string{}
		pcapFileKeys := []string{}
		for _, pcap := range pcapsByCommunity[session.CommunityID] {
			pcapIndexIDs = append(pcapIndexIDs, pcap.PcapIndexID)
			pcapFileKeys = append(pcapFileKeys, pcap.FileKey)
		}
		eventIDs := append([]string{session.EventID}, session.SourceEventIDs...)
		facts = append(facts, EncryptedTrafficRawReferenceFact{
			SessionID: session.SessionID, CommunityID: session.CommunityID,
			EventIDs: sortedUniqueStrings(eventIDs), EvidenceRefs: sortedUniqueStrings(session.EvidenceIDs),
			PcapIndexIDs: sortedUniqueStrings(pcapIndexIDs), PcapFileKeys: sortedUniqueStrings(pcapFileKeys),
			DownloadAllowed: downloadAllowed,
		})
	}
	availability := "available"
	if len(sessions) == 0 && len(pcaps) == 0 {
		availability = "no_sample"
	}
	watermark := "sessions=" + sessionWatermark + ";pcap_index_v2=" + pcapWatermark
	return EncryptedTrafficSnapshotSection{
		Availability: availability, SampleCount: len(facts), Source: "clickhouse.traffic.sessions+traffic.pcap_index_v2",
		SourceWatermark: watermark, RuleVersions: []string{}, ModelVersions: []string{},
		Partial: false, MissingReasons: []string{}, Facts: facts,
	}
}

func encryptedSnapshotUnavailableSection(source, reason string) EncryptedTrafficSnapshotSection {
	return EncryptedTrafficSnapshotSection{
		Availability: "unavailable", SampleCount: 0, Source: source, SourceWatermark: "unavailable",
		RuleVersions: []string{}, ModelVersions: []string{}, Partial: true,
		MissingReasons: []string{reason}, Facts: []interface{}{},
	}
}

func encryptedSnapshotForbiddenSection(reason string) EncryptedTrafficSnapshotSection {
	return EncryptedTrafficSnapshotSection{
		Availability: "forbidden", SampleCount: 0, Source: "permission.pcap:read", SourceWatermark: "forbidden",
		RuleVersions: []string{}, ModelVersions: []string{}, Partial: true,
		MissingReasons: []string{reason}, Facts: []interface{}{},
	}
}

func encryptedSnapshotDirection(bytesForward, bytesBackward uint64) string {
	if bytesForward == 0 && bytesBackward == 0 {
		return "no_payload"
	}
	if bytesForward == 0 {
		return "reverse_only"
	}
	if bytesBackward == 0 {
		return "forward_only"
	}
	return "bidirectional"
}

func encryptedSnapshotWatermark(version string, query encryptedTrafficSnapshotQuery, count uint64, maxEvent int64, identityXOR uint64) string {
	canonical := strings.Join([]string{
		version, query.TenantID, strconv.FormatInt(query.Start.UnixMilli(), 10), strconv.FormatInt(query.End.UnixMilli(), 10),
		strconv.FormatUint(count, 10), strconv.FormatInt(maxEvent, 10), strconv.FormatUint(identityXOR, 10),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("max_event_time_ms=%d;row_count=%d;closed_window_sha256=%s", maxEvent, count, hex.EncodeToString(sum[:]))
}

func encryptedSnapshotEmptyWatermark(version string, query encryptedTrafficSnapshotQuery) string {
	return encryptedSnapshotWatermark(version, query, 0, 0, 0)
}

func stableEncryptedIdentity(parts ...string) uint64 {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	value, _ := strconv.ParseUint(hex.EncodeToString(sum[:8]), 16, 64)
	return value
}

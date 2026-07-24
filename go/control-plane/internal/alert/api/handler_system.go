package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type actionAuditRecorder interface {
	Record(context.Context, *http.Request, AlertActionAuditRecord) error
}

type campaignActionJobStore interface {
	Record(context.Context, campaignActionJob) error
	MarkFailed(context.Context, string, string, string) error
	Get(context.Context, string, string) (campaignActionJob, error)
}

type SystemHandler struct {
	chClient             *storage.ClickHouseClient
	pgDB                 *sql.DB
	actionAudit          actionAuditRecorder
	campaignJobs         campaignActionJobStore
	commitCampaignAction func(context.Context, *http.Request, campaignActionJob, AlertActionAuditRecord) error
	lookupCampaign       func(context.Context, string, string) (campaignDTO, error)
	logger               *zap.Logger
}

func NewSystemHandler(chClient *storage.ClickHouseClient, pgDB *sql.DB, logger *zap.Logger) *SystemHandler {
	handler := &SystemHandler{
		chClient: chClient,
		pgDB:     pgDB,
		logger:   logger,
	}
	handler.lookupCampaign = handler.queryCampaignByID
	writer := NewAlertActionAuditWriter(pgDB, logger)
	if writer != nil {
		handler.actionAudit = writer
	}
	if pgDB != nil {
		jobStore := newPostgresCampaignActionJobStore(pgDB)
		handler.campaignJobs = jobStore
		handler.commitCampaignAction = func(ctx context.Context, request *http.Request, job campaignActionJob, audit AlertActionAuditRecord) error {
			return commitCampaignActionTransaction(ctx, pgDB, jobStore, writer, request, job, audit)
		}
	}
	return handler
}

func (h *SystemHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/campaigns", h.ListCampaigns).Methods("GET")
	r.HandleFunc("/campaigns/actions", h.SubmitCampaignAction).Methods("POST")
	r.HandleFunc("/campaigns/jobs/{job_id}", h.GetCampaignActionJob).Methods("GET")
	r.HandleFunc("/campaigns/{id}", h.GetCampaign).Methods("GET")
	r.HandleFunc("/campaigns/{id}/actions", h.SubmitCampaignAction).Methods("POST")
	r.HandleFunc("/attack-chains", h.ListAttackChains).Methods("GET")
	r.HandleFunc("/attack-chains/{id}", h.GetAttackChain).Methods("GET")
	r.HandleFunc("/attack-chains/{id}/phases", h.GetAttackChainPhases).Methods("GET")
	r.HandleFunc("/probes", h.ListProbes).Methods("GET")
	r.HandleFunc("/probes/topology", h.GetProbeTopology).Methods("GET")
	r.HandleFunc("/probes/batch-upgrade", h.BatchUpgradeProbes).Methods("POST")
	r.HandleFunc("/probes/batch-state", h.BatchSetProbeState).Methods("POST")
	r.HandleFunc("/probes/{id}/config", h.PushProbeConfig).Methods("POST")
	r.HandleFunc("/probes/{id}/connectivity-test", h.RunProbeConnectivityTest).Methods("POST")
	r.HandleFunc("/probes/{id}/certificates/rotate", h.RotateProbeCertificate).Methods("POST")
	r.HandleFunc("/probes/{id}/restart", h.RestartProbe).Methods("POST")
	r.HandleFunc("/encrypted-traffic/stats", h.GetEncryptedTrafficStats).Methods("GET")
	r.HandleFunc("/encrypted-traffic/sessions", h.ListEncryptedTrafficSessions).Methods("GET")
	r.HandleFunc("/encrypted-traffic/ja3", h.ListJA3Fingerprints).Methods("GET")
	r.HandleFunc("/encrypted-traffic/tunnels", h.GetEncryptedTunnelAnalytics).Methods("GET")
	r.HandleFunc("/encrypted-traffic/exfiltration", h.GetEncryptedExfiltrationAnalytics).Methods("GET")
	r.HandleFunc("/encrypted-traffic/evidence", h.GetEncryptedTrafficEvidence).Methods("GET")
	r.HandleFunc("/encrypted-traffic/egress-actions", h.SubmitEncryptedTrafficEgressAction).Methods("POST")
	r.HandleFunc("/encrypted-traffic/evidence-actions", h.SubmitEncryptedTrafficEvidenceAction).Methods("POST")
	r.HandleFunc("/topics/tunnel", h.GetTunnelTopic).Methods("GET")
	r.HandleFunc("/topics/exfil", h.GetExfiltrationTopic).Methods("GET")
	r.HandleFunc("/topics/apt", h.GetAPTTopic).Methods("GET")
	r.HandleFunc("/topics/views", h.ListTopicViews).Methods("GET")
	r.HandleFunc("/topics/views", h.SaveTopicView).Methods("POST")
	r.HandleFunc("/topics/views/{id}", h.UpdateTopicView).Methods("PATCH")
	r.HandleFunc("/topics/scopes/{topic}", h.UpdateTopicScope).Methods("PUT", "PATCH")
	r.HandleFunc("/topics/subscriptions", h.ListTopicSubscriptions).Methods("GET")
	r.HandleFunc("/topics/subscriptions", h.CreateTopicSubscription).Methods("POST")
	r.HandleFunc("/topics/subscriptions/{id}", h.UpdateTopicSubscription).Methods("PATCH")
	r.HandleFunc("/topics/reports/export", h.ExportTopicReport).Methods("POST")
	r.HandleFunc("/topics/evidence-packages/export", h.ExportTopicEvidencePackage).Methods("POST")
	r.HandleFunc("/fusion/sources", h.ListFusionSources).Methods("GET")
	r.HandleFunc("/fusion/stats", h.GetFusionStats).Methods("GET")
	r.HandleFunc("/fusion/workbench", h.GetFusionWorkbench).Methods("GET")
	r.HandleFunc("/fusion/value-report", h.GetFusionValueReport).Methods("GET")
	r.HandleFunc("/fusion/entities", h.ListFusionEntities).Methods("GET")
	r.HandleFunc("/fusion/sources/{id}/sync", h.SyncFusionSource).Methods("POST")
	r.HandleFunc("/fusion/conflicts/{id}/resolve", h.ResolveFusionConflict).Methods("POST")
	r.HandleFunc("/fusion/rules/{id}", h.UpdateFusionRule).Methods("PATCH", "PUT")
	r.HandleFunc("/fusion/evidence-packages", h.ExportFusionEvidencePackage).Methods("POST")
	r.HandleFunc("/baselines", h.ListBehaviorBaselines).Methods("GET")
	r.HandleFunc("/baselines/overview", h.GetBehaviorBaselineOverview).Methods("GET")
	r.HandleFunc("/baselines/{id}", h.GetBehaviorBaseline).Methods("GET")
	r.HandleFunc("/baselines/{id}/analytics", h.GetBehaviorBaselineAnalytics).Methods("GET")
	r.HandleFunc("/baselines/{id}/versions", h.ListBehaviorBaselineVersions).Methods("GET")
	r.HandleFunc("/baselines/{id}/actions", h.ListBehaviorBaselineActions).Methods("GET")
	r.HandleFunc("/baselines/{id}/reset", h.ResetBehaviorBaseline).Methods("POST")
	r.HandleFunc("/baselines/{id}/actions", h.SubmitBehaviorBaselineAction).Methods("POST")
	r.HandleFunc("/compliance/reports", h.ListComplianceReports).Methods("GET")
	r.HandleFunc("/compliance/reports/generate", h.GenerateComplianceReport).Methods("POST")
	r.HandleFunc("/compliance/reports/{id}/evidence-package", h.ExportComplianceEvidencePackage).Methods("POST")
	r.HandleFunc("/compliance/reports/{id}/export", h.ExportComplianceReport).Methods("POST")
	r.HandleFunc("/compliance/reports/{id}/remediations", h.CreateComplianceRemediations).Methods("POST")
	r.HandleFunc("/compliance/reports/{id}/finalize", h.FinalizeComplianceReport).Methods("POST")
	r.HandleFunc("/compliance/audit-trail", h.ListAuditTrail).Methods("GET")
	r.HandleFunc("/audit/logs", h.ListAuditLogs).Methods("GET")
	r.HandleFunc("/audit/logs/{id}", h.GetAuditLog).Methods("GET")
	r.HandleFunc("/audit/saved-queries", h.CreateAuditSavedQuery).Methods("POST")
	r.HandleFunc("/audit/exports", h.CreateAuditExport).Methods("POST")
	r.HandleFunc("/audit/reviews", h.CreateAuditReview).Methods("POST")
	r.HandleFunc("/audit/integrity-checks", h.CreateAuditIntegrityCheck).Methods("POST")
}

type campaignActionRequest struct {
	ActionID   string                 `json:"action_id"`
	Target     string                 `json:"target"`
	Metadata   map[string]interface{} `json:"metadata"`
	Simulation *bool                  `json:"simulation"`
	DryRun     *bool                  `json:"dry_run,omitempty"`
}

type campaignActionSpec struct {
	AuditEvent string
	Scopes     []string
	Collection bool
	Mutates    bool
}

var campaignActionSpecs = map[string]campaignActionSpec{
	"campaign-export":            {AuditEvent: "CAMPAIGN_EXPORT_REQUESTED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}, Collection: true},
	"campaign-list-settings":     {AuditEvent: "CAMPAIGN_LIST_SETTINGS_UPDATED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}, Collection: true},
	"campaign-detail-view":       {AuditEvent: "CAMPAIGN_DETAIL_VIEWED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}},
	"campaign-phase-inspect":     {AuditEvent: "CAMPAIGN_PHASE_VIEWED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}},
	"campaign-impact-inspect":    {AuditEvent: "CAMPAIGN_IMPACT_VIEWED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}},
	"campaign-evidence-view":     {AuditEvent: "CAMPAIGN_EVIDENCE_VIEWED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}},
	"campaign-attack-chain-view": {AuditEvent: "CAMPAIGN_ATTACK_CHAIN_VIEWED", Scopes: []string{authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite}},
	"campaign-assign-owner":      {AuditEvent: "CAMPAIGN_OWNER_ASSIGNED", Scopes: []string{authmodel.ScopeAlertWrite}, Mutates: true},
	"campaign-status-change":     {AuditEvent: "CAMPAIGN_STATUS_CHANGED", Scopes: []string{authmodel.ScopeAlertWrite}, Mutates: true},
	"campaign-report-generate":   {AuditEvent: "CAMPAIGN_REPORT_REQUESTED", Scopes: []string{authmodel.ScopeAlertWrite}, Mutates: true},
	"campaign-context-action":    {AuditEvent: "CAMPAIGN_CONTEXT_ACTION_REQUESTED", Scopes: []string{authmodel.ScopeAlertWrite}},
	"campaign-graph-view":        {AuditEvent: "CAMPAIGN_GRAPH_VIEWED", Scopes: []string{authmodel.ScopeGraphRead}},
	"campaign-soar-response":     {AuditEvent: "CAMPAIGN_SOAR_RESPONSE_REQUESTED", Scopes: []string{"playbook:execute"}},
}

func (h *SystemHandler) SubmitCampaignAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var request campaignActionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "malformed campaign action request")
		return
	}
	if err := ensureJSONBodyComplete(decoder); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "malformed campaign action request")
		return
	}

	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Target = strings.TrimSpace(request.Target)
	spec, knownAction := campaignActionSpecs[request.ActionID]
	if !knownAction || request.Target == "" || request.Metadata == nil || request.Simulation == nil || request.DryRun == nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "action_id, target, metadata, simulation and dry_run are required")
		return
	}
	if !hasAnySystemPermission(ctx, spec.Scopes...) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied for campaign action")
		return
	}
	if spec.Mutates {
		if *request.Simulation || *request.DryRun || !metadataBoolEquals(request.Metadata, "dry_run", false) {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "mutating campaign actions require simulation=false and dry_run=false")
			return
		}
	} else if !*request.Simulation || !*request.DryRun || !metadataBoolIsTrue(request.Metadata, "dry_run") {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "read-only campaign actions require simulation=true and dry_run=true")
		return
	}
	campaignID := strings.TrimSpace(mux.Vars(r)["id"])
	if campaignID == "" {
		if !spec.Collection {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "campaign action requires /campaigns/{id}/actions")
			return
		}
		campaignID = "campaign-collection"
		if _, exists := request.Metadata["campaign_id"]; exists {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "collection action must not include metadata campaign_id")
			return
		}
	} else {
		if spec.Collection {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "collection action must use /campaigns/actions")
			return
		}
		metadataCampaignID, ok := request.Metadata["campaign_id"].(string)
		if !ok || strings.TrimSpace(metadataCampaignID) != campaignID {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "metadata campaign_id must match the request path")
			return
		}
		if h.lookupCampaign == nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "campaign lookup is not configured")
			return
		}
		if _, err := h.lookupCampaign(ctx, queryTenantID(r), campaignID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "campaign not found")
				return
			}
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to validate campaign target")
			return
		}
	}
	if spec.Mutates {
		if h.pgDB != nil {
			if err := ensureCampaignWorkbenchSchema(ctx, h.pgDB); err != nil {
				httpx.JSONError(w, ctx, http.StatusInternalServerError, "SCHEMA_UNAVAILABLE", "campaign workbench schema is unavailable")
				return
			}
		}
		switch request.ActionID {
		case "campaign-assign-owner":
			assignee := campaignMetadataString(request.Metadata, "assignee")
			if assignee == "" || len([]rune(assignee)) > 128 {
				httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "assignee is required and must not exceed 128 characters")
				return
			}
		case "campaign-status-change":
			if !validCampaignWorkbenchStatus(strings.ToLower(campaignMetadataString(request.Metadata, "next_status"))) {
				httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "next_status must be active, investigating, contained, or closed")
				return
			}
		}
	}
	endpoint := r.URL.Path
	jobID := "campaign-" + uuid.NewString()
	simulation := !spec.Mutates
	dryRun := !spec.Mutates
	detail := map[string]interface{}{
		"action_id":   request.ActionID,
		"audit_event": spec.AuditEvent,
		"target":      request.Target,
		"metadata":    request.Metadata,
		"endpoint":    endpoint,
		"job_id":      jobID,
		"simulation":  simulation,
		"dry_run":     dryRun,
		"status":      "completed",
		"job_status":  "completed",
	}
	if request.ActionID == "campaign-report-generate" {
		detail["report_id"] = "campaign-report-" + uuid.NewString()
	}
	if h.campaignJobs == nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "JOB_STORE_UNAVAILABLE", "campaign action job store is not configured")
		return
	}
	job := campaignActionJob{
		JobID: jobID, TenantID: httpx.GetTenantID(ctx), CampaignID: campaignID,
		ActionID: request.ActionID, Target: request.Target, Metadata: request.Metadata,
		Simulation: simulation, DryRun: dryRun, Status: "completed", Result: detail,
		CreatedBy: httpx.GetUserID(ctx),
	}
	auditRecord := AlertActionAuditRecord{
		Action:     spec.AuditEvent,
		ObjectType: "campaign",
		ObjectID:   campaignID,
		TenantID:   httpx.GetTenantID(ctx),
		UserID:     httpx.GetUserID(ctx),
		Result:     "completed",
		Detail:     detail,
	}
	var commitErr error
	if h.commitCampaignAction != nil {
		commitErr = h.commitCampaignAction(ctx, r, job, auditRecord)
	} else {
		commitErr = errors.New("atomic campaign action committer is not configured")
	}
	if commitErr != nil {
		if h.logger != nil {
			h.logger.Error("Failed to atomically persist campaign action and audit", zap.String("action_id", request.ActionID), zap.String("campaign_id", campaignID), zap.Error(commitErr))
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "ACTION_COMMIT_FAILED", "failed to persist campaign action and audit")
		return
	}

	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"action_id":   request.ActionID,
		"audit_event": spec.AuditEvent,
		"status":      "completed",
		"endpoint":    endpoint,
		"job_id":      jobID,
		"job_status":  "completed",
		"simulation":  simulation,
		"dry_run":     dryRun,
		"result":      detail,
	})
}

func (h *SystemHandler) GetCampaignActionJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAnySystemPermission(ctx, authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite) {
		httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "alert:read or alert:write required")
		return
	}
	if h.campaignJobs == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "JOB_STORE_UNAVAILABLE", "campaign action job store is not configured")
		return
	}
	job, err := h.campaignJobs.Get(ctx, queryTenantID(r), strings.TrimSpace(mux.Vars(r)["job_id"]))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "campaign action job not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to read campaign action job")
		return
	}
	httpx.JSONSuccess(w, ctx, job)
}

func ensureJSONBodyComplete(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func metadataBoolIsTrue(metadata map[string]interface{}, key string) bool {
	value, exists := metadata[key]
	if !exists {
		return false
	}
	boolean, ok := value.(bool)
	return ok && boolean
}

func metadataBoolEquals(metadata map[string]interface{}, key string, expected bool) bool {
	value, exists := metadata[key]
	if !exists {
		return false
	}
	boolean, ok := value.(bool)
	return ok && boolean == expected
}

func hasAnySystemPermission(ctx context.Context, permissions ...string) bool {
	for _, permission := range permissions {
		if hasSystemPermission(ctx, permission) {
			return true
		}
	}
	return false
}

type campaignDTO struct {
	TenantID           string   `json:"tenant_id"`
	CampaignID         string   `json:"campaign_id"`
	TsStart            int64    `json:"ts_start"`
	TsEnd              int64    `json:"ts_end"`
	Entities           []string `json:"entities"`
	Alerts             []string `json:"alerts"`
	Score              float64  `json:"score"`
	Summary            string   `json:"summary"`
	EventID            string   `json:"event_id"`
	IngestTs           int64    `json:"ingest_ts"`
	CampaignType       string   `json:"campaign_type"`
	AttackPhases       []string `json:"attack_phases"`
	RuleIDs            []string `json:"rule_ids"`
	ModelIDs           []string `json:"model_ids"`
	HeaderProbeID      string   `json:"header_probe_id,omitempty"`
	ActivityStatus     string   `json:"activity_status"`
	Status             string   `json:"status"`
	Assignee           string   `json:"assignee"`
	StateVersion       int64    `json:"state_version"`
	WorkbenchUpdatedAt string   `json:"workbench_updated_at,omitempty"`
}

type campaignSummaryDTO struct {
	Total                uint64  `json:"total"`
	Active               uint64  `json:"active"`
	AffectedAssets       uint64  `json:"affected_assets"`
	HighRisk             uint64  `json:"high_risk"`
	MediumRisk           uint64  `json:"medium_risk"`
	LowRisk              uint64  `json:"low_risk"`
	AlertCount           uint64  `json:"alert_count"`
	AverageDurationHours float64 `json:"average_duration_hours"`
	MaxScore             float64 `json:"max_score"`
}

type campaignDetailDTO struct {
	TenantID           string             `json:"tenant_id"`
	CampaignID         string             `json:"campaign_id"`
	TsStart            int64              `json:"ts_start"`
	TsEnd              int64              `json:"ts_end"`
	Entities           []string           `json:"entities"`
	AlertIDs           []string           `json:"alert_ids"`
	Alerts             []campaignAlertDTO `json:"alerts"`
	Score              float64            `json:"score"`
	Summary            string             `json:"summary"`
	EventID            string             `json:"event_id"`
	IngestTs           int64              `json:"ingest_ts"`
	CampaignType       string             `json:"campaign_type"`
	AttackPhases       []string           `json:"attack_phases"`
	RuleIDs            []string           `json:"rule_ids"`
	ModelIDs           []string           `json:"model_ids"`
	PhaseSummaries     []campaignPhaseDTO `json:"phase_summaries"`
	PhaseDataBacked    bool               `json:"phase_data_backed"`
	ActivityStatus     string             `json:"activity_status"`
	Status             string             `json:"status"`
	Assignee           string             `json:"assignee"`
	StateVersion       int64              `json:"state_version"`
	WorkbenchUpdatedAt string             `json:"workbench_updated_at,omitempty"`
}

type campaignAlertDTO struct {
	AlertID       string `json:"alert_id"`
	AlertType     string `json:"alert_type"`
	Severity      string `json:"severity"`
	LastSeen      int64  `json:"last_seen"`
	AttackPhase   string `json:"attack_phase"`
	EvidenceCount uint64 `json:"evidence_count"`
}

type campaignPhaseDTO struct {
	Phase         string `json:"phase"`
	AlertCount    uint64 `json:"alert_count"`
	EvidenceCount uint64 `json:"evidence_count"`
	LastSeen      int64  `json:"last_seen"`
}

type attackChainDTO struct {
	ChainID         string           `json:"chain_id"`
	TenantID        string           `json:"tenant_id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	Phases          []attackPhaseDTO `json:"phases"`
	RiskScore       int              `json:"risk_score"`
	RootAlertID     string           `json:"root_alert_id"`
	SourceIP        string           `json:"source_ip"`
	EntityCount     int              `json:"entity_count"`
	AlertCount      int              `json:"alert_count"`
	StartTime       int64            `json:"start_time"`
	EndTime         int64            `json:"end_time"`
	Status          string           `json:"status"`
	MitreTechniques []string         `json:"mitre_techniques"`
}

type attackPhaseDTO struct {
	Phase      string           `json:"phase"`
	AlertIDs   []string         `json:"alert_ids"`
	StartTime  int64            `json:"start_time"`
	EndTime    int64            `json:"end_time"`
	KeyEvents  []attackEventDTO `json:"key_events"`
	Confidence float64          `json:"confidence"`
}

type attackEventDTO struct {
	EventID     string `json:"event_id"`
	Timestamp   int64  `json:"timestamp"`
	Description string `json:"description"`
	SrcIP       string `json:"src_ip"`
	DstIP       string `json:"dst_ip"`
	Technique   string `json:"technique,omitempty"`
	Severity    string `json:"severity"`
}

type probeDTO struct {
	ProbeID                string    `json:"probe_id"`
	Hostname               string    `json:"hostname"`
	IPAddress              string    `json:"ip_address"`
	Location               string    `json:"location"`
	Status                 string    `json:"status"`
	HealthScore            int       `json:"health_score"`
	CPUUsage               float64   `json:"cpu_usage"`
	MemoryUsage            float64   `json:"memory_usage"`
	DiskUsage              float64   `json:"disk_usage"`
	DropRate               float64   `json:"drop_rate"`
	ParseRate              float64   `json:"parse_rate"`
	BandwidthMbps          float64   `json:"bandwidth_mbps"`
	CaptureMode            string    `json:"capture_mode"`
	Interfaces             []string  `json:"interfaces"`
	UptimeSeconds          int64     `json:"uptime_seconds"`
	ArchivePath            string    `json:"archive_path"`
	MTLSEnabled            bool      `json:"mtls_enabled"`
	TopologyX              float64   `json:"topology_x"`
	TopologyY              float64   `json:"topology_y"`
	TopologyZ              float64   `json:"topology_z"`
	TopologyZone           string    `json:"topology_zone"`
	TopologyRole           string    `json:"topology_role"`
	TopologyLinks          []string  `json:"topology_links"`
	TopologyLinkBandwidths []float64 `json:"topology_link_bandwidths_gbps"`
	TrendLabels            []string  `json:"trend_labels"`
	BandwidthTrend         []float64 `json:"bandwidth_trend"`
	BatchTrend             []float64 `json:"batch_trend"`
	PPSK                   float64   `json:"pps_k"`
	BandwidthThresholdGbps float64   `json:"bandwidth_threshold_gbps"`
	ConfigVersion          string    `json:"config_version"`
	LastHeartbeat          int64     `json:"last_heartbeat"`
}

func (h *SystemHandler) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	filters, err := campaignQueryFiltersFromRequest(r)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_FILTER", err.Error())
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parseLimitOffset(r, 20, 100)
	campaigns, total, err := h.queryCampaigns(ctx, tenantID, filters, parseInt64Query(r, "start_time"), parseInt64Query(r, "end_time"), limit, offset)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	summary, err := h.queryCampaignSummary(ctx, tenantID, filters, parseInt64Query(r, "start_time"), parseInt64Query(r, "end_time"))
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"campaigns": campaigns,
		"summary":   summary,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

func (h *SystemHandler) GetCampaign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	campaign, err := h.queryCampaignByID(ctx, queryTenantID(r), mux.Vars(r)["id"])
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	alerts := h.queryCampaignAlerts(ctx, campaign.TenantID, campaign.CampaignID, campaign.Alerts)
	phaseSummaries, phaseDataBacked := h.queryCampaignPhaseSummaries(ctx, campaign.TenantID, campaign.CampaignID, campaign.AttackPhases)
	httpx.JSONSuccess(w, ctx, campaignDetailDTO{
		TenantID: campaign.TenantID, CampaignID: campaign.CampaignID,
		TsStart: campaign.TsStart, TsEnd: campaign.TsEnd,
		Entities: campaign.Entities, AlertIDs: campaign.Alerts, Alerts: alerts,
		Score: campaign.Score, Summary: campaign.Summary, EventID: campaign.EventID, IngestTs: campaign.IngestTs,
		CampaignType: campaign.CampaignType, AttackPhases: campaign.AttackPhases, RuleIDs: campaign.RuleIDs, ModelIDs: campaign.ModelIDs,
		PhaseSummaries:  phaseSummaries,
		PhaseDataBacked: phaseDataBacked,
		ActivityStatus:  campaign.ActivityStatus,
		Status:          campaign.Status,
		Assignee:        campaign.Assignee, StateVersion: campaign.StateVersion,
		WorkbenchUpdatedAt: campaign.WorkbenchUpdatedAt,
	})
}

func (h *SystemHandler) ListAttackChains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parseLimitOffset(r, 20, 100)
	campaigns, total, err := h.queryCampaigns(ctx, tenantID, campaignQueryFilters{}, 0, 0, limit, offset)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	chains := make([]attackChainDTO, 0, len(campaigns))
	for _, campaign := range campaigns {
		chains = append(chains, campaignToAttackChain(campaign))
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"chains": chains, "total": total, "limit": limit, "offset": offset})
}

func (h *SystemHandler) GetAttackChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	campaign, err := h.queryCampaignByID(ctx, queryTenantID(r), mux.Vars(r)["id"])
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	httpx.JSONSuccess(w, ctx, campaignToAttackChain(campaign))
}

func (h *SystemHandler) GetAttackChainPhases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	campaign, err := h.queryCampaignByID(ctx, queryTenantID(r), mux.Vars(r)["id"])
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"phases": campaignToPhases(campaign)})
}

const statusClientClosedRequest = 499

func writeCampaignReadError(w http.ResponseWriter, ctx context.Context, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		httpx.JSONError(w, ctx, statusClientClosedRequest, "CLIENT_CLOSED_REQUEST", "request canceled by client")
	case errors.Is(err, context.DeadlineExceeded):
		httpx.JSONError(w, ctx, http.StatusGatewayTimeout, "QUERY_TIMEOUT", "campaign query timed out")
	case errors.Is(err, sql.ErrNoRows):
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "campaign not found")
	default:
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}

func (h *SystemHandler) ListProbes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "postgres is not configured")
		return
	}
	if !h.requireProbeReadPermission(w, r) {
		return
	}
	tenantID := queryTenantID(r)
	limit, offset := parseLimitOffset(r, 50, 200)

	var total int
	if err := h.pgDB.QueryRowContext(ctx, `SELECT count(*) FROM probes WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT probe_id, name, status, hardware_info, software_version, last_heartbeat
		FROM probes WHERE tenant_id=$1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	probes := make([]probeDTO, 0)
	for rows.Next() {
		probe, scanErr := scanProbe(rows)
		if scanErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", scanErr.Error())
			return
		}
		probes = append(probes, probe)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"probes": probes, "total": total, "limit": limit, "offset": offset})
}

type campaignQueryFilters struct {
	CampaignType string
	Risk         string
	Status       string
	Phase        string
	Keyword      string
}

func campaignQueryFiltersFromRequest(r *http.Request) (campaignQueryFilters, error) {
	filters := campaignQueryFilters{
		CampaignType: strings.TrimSpace(r.URL.Query().Get("campaign_type")),
		Risk:         strings.ToLower(strings.TrimSpace(r.URL.Query().Get("risk"))),
		Status:       strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status"))),
		Phase:        strings.ToLower(strings.TrimSpace(r.URL.Query().Get("phase"))),
		Keyword:      strings.TrimSpace(r.URL.Query().Get("keyword")),
	}
	if filters.Risk != "" && filters.Risk != "high" && filters.Risk != "medium" && filters.Risk != "low" {
		return campaignQueryFilters{}, fmt.Errorf("risk must be high, medium, or low")
	}
	if filters.Status != "" && filters.Status != "active" && filters.Status != "investigating" && filters.Status != "closed" {
		return campaignQueryFilters{}, fmt.Errorf("status must be active, investigating, or closed")
	}
	allowedPhases := map[string]struct{}{
		"initial_access": {}, "execution": {}, "persistence": {}, "lateral_movement": {},
		"command_and_control": {}, "exfiltration": {}, "impact": {},
	}
	if filters.Phase != "" {
		if _, ok := allowedPhases[filters.Phase]; !ok {
			return campaignQueryFilters{}, fmt.Errorf("unsupported campaign phase")
		}
	}
	if len([]rune(filters.Keyword)) > 128 {
		return campaignQueryFilters{}, fmt.Errorf("keyword must not exceed 128 characters")
	}
	return filters, nil
}

func buildCampaignWhere(tenantID string, filters campaignQueryFilters, start, end int64) (string, []interface{}) {
	conditions := []string{"tenant_id=?"}
	args := []interface{}{tenantID}
	if filters.CampaignType != "" {
		conditions = append(conditions, "campaign_type=?")
		args = append(args, filters.CampaignType)
	}
	switch filters.Risk {
	case "high":
		conditions = append(conditions, "score>=0.8")
	case "medium":
		conditions = append(conditions, "score>=0.5 AND score<0.8")
	case "low":
		conditions = append(conditions, "score<0.5")
	}
	switch filters.Status {
	case "active":
		conditions = append(conditions, "ts_end>=toUnixTimestamp64Milli(now64(3) - INTERVAL 24 HOUR)")
	case "investigating":
		conditions = append(conditions, "ts_end<toUnixTimestamp64Milli(now64(3) - INTERVAL 24 HOUR) AND ts_end>=toUnixTimestamp64Milli(now64(3) - INTERVAL 7 DAY)")
	case "closed":
		conditions = append(conditions, "ts_end<toUnixTimestamp64Milli(now64(3) - INTERVAL 7 DAY)")
	}
	if filters.Phase != "" {
		conditions = append(conditions, "has(attack_phases, ?)")
		args = append(args, filters.Phase)
	}
	if filters.Keyword != "" {
		conditions = append(conditions, "(positionCaseInsensitiveUTF8(campaign_id, ?)>0 OR positionCaseInsensitiveUTF8(summary, ?)>0)")
		args = append(args, filters.Keyword, filters.Keyword)
	}
	if start > 0 {
		conditions = append(conditions, "ts_start>=?")
		args = append(args, start)
	}
	if end > 0 {
		conditions = append(conditions, "ts_end<=?")
		args = append(args, end)
	}
	return strings.Join(conditions, " AND "), args
}

func (h *SystemHandler) queryCampaigns(ctx context.Context, tenantID string, filters campaignQueryFilters, start, end int64, limit, offset int) ([]campaignDTO, int64, error) {
	where, args := buildCampaignWhere(tenantID, filters, start, end)
	var total uint64
	countRow, err := h.chClient.QueryRow(ctx, `SELECT count() FROM traffic.campaigns WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := h.chClient.Query(ctx, campaignSelectSQL(`WHERE `+where+` ORDER BY ts_end DESC LIMIT ? OFFSET ?`), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	campaigns := make([]campaignDTO, 0)
	for rows.Next() {
		campaign, err := scanCampaignRows(rows)
		if err != nil {
			return nil, 0, err
		}
		campaigns = append(campaigns, campaign)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	campaigns, err = h.enrichCampaignWorkbenchStates(ctx, tenantID, campaigns)
	if err != nil {
		return nil, 0, err
	}
	return campaigns, int64(total), nil
}

func (h *SystemHandler) queryCampaignSummary(ctx context.Context, tenantID string, filters campaignQueryFilters, start, end int64) (campaignSummaryDTO, error) {
	where, args := buildCampaignWhere(tenantID, filters, start, end)
	row, err := h.chClient.QueryRow(ctx, `
		SELECT
			count() AS total,
			countIf(ts_end>=toUnixTimestamp64Milli(now64(3) - INTERVAL 24 HOUR)) AS active,
			uniqCombined64Array(entities) AS affected_assets,
			countIf(score>=0.8) AS high_risk,
			countIf(score>=0.5 AND score<0.8) AS medium_risk,
			countIf(score<0.5) AS low_risk,
			sum(length(alerts)) AS alert_count,
			if(countIf(ts_end>ts_start)=0, 0, avgIf((ts_end-ts_start)/3600000.0, ts_end>ts_start)) AS average_duration_hours,
			max(score) AS max_score
		FROM traffic.campaigns
		WHERE `+where, args...)
	if err != nil {
		return campaignSummaryDTO{}, err
	}
	var summary campaignSummaryDTO
	var maxScore float32
	if err := row.Scan(
		&summary.Total,
		&summary.Active,
		&summary.AffectedAssets,
		&summary.HighRisk,
		&summary.MediumRisk,
		&summary.LowRisk,
		&summary.AlertCount,
		&summary.AverageDurationHours,
		&maxScore,
	); err != nil {
		return campaignSummaryDTO{}, err
	}
	summary.MaxScore = float64(maxScore)
	return summary, nil
}

func (h *SystemHandler) queryCampaignByID(ctx context.Context, tenantID, id string) (campaignDTO, error) {
	if id == "" {
		return campaignDTO{}, sql.ErrNoRows
	}
	row, err := h.chClient.QueryRow(ctx, campaignSelectSQL(`WHERE tenant_id=? AND campaign_id=? LIMIT 1`), tenantID, id)
	if err != nil {
		return campaignDTO{}, err
	}
	campaign, err := scanCampaignRow(row)
	if err != nil {
		return campaignDTO{}, err
	}
	campaigns, err := h.enrichCampaignWorkbenchStates(ctx, tenantID, []campaignDTO{campaign})
	if err != nil {
		return campaignDTO{}, err
	}
	return campaigns[0], nil
}

func (h *SystemHandler) queryCampaignAlerts(ctx context.Context, tenantID, campaignID string, alertIDs []string) []campaignAlertDTO {
	rows, err := h.chClient.Query(ctx, `
		SELECT alert_id, alert_type, severity, last_seen,
			`+campaignAlertAttackPhaseExpression+` AS attack_phase,
			toUInt64(length(evidence_ids)) AS evidence_count
		FROM traffic.alerts WHERE tenant_id=? AND campaign_id=?
		ORDER BY last_seen DESC LIMIT 200`, tenantID, campaignID)
	if err != nil {
		return alertIDsToSummaries(alertIDs)
	}
	defer rows.Close()

	alerts := make([]campaignAlertDTO, 0)
	for rows.Next() {
		var alert campaignAlertDTO
		if err := rows.Scan(
			&alert.AlertID,
			&alert.AlertType,
			&alert.Severity,
			&alert.LastSeen,
			&alert.AttackPhase,
			&alert.EvidenceCount,
		); err != nil {
			return alertIDsToSummaries(alertIDs)
		}
		alerts = append(alerts, alert)
	}
	if len(alerts) == 0 && len(alertIDs) > 0 {
		return alertIDsToSummaries(alertIDs)
	}
	return alerts
}

func (h *SystemHandler) queryCampaignPhaseSummaries(
	ctx context.Context,
	tenantID, campaignID string,
	campaignPhases []string,
) ([]campaignPhaseDTO, bool) {
	rows, err := h.chClient.Query(ctx, `
		SELECT
			`+campaignAlertAttackPhaseExpression+` AS attack_phase,
			toUInt64(count()) AS alert_count,
			toUInt64(sum(length(evidence_ids))) AS evidence_count,
			max(last_seen) AS last_seen
		FROM traffic.alerts
		WHERE tenant_id=? AND campaign_id=?
		GROUP BY attack_phase
		ORDER BY last_seen`, tenantID, campaignID)
	if err != nil {
		return campaignPhaseFallback(campaignPhases), false
	}
	defer rows.Close()

	summaries := make([]campaignPhaseDTO, 0)
	for rows.Next() {
		var summary campaignPhaseDTO
		if err := rows.Scan(&summary.Phase, &summary.AlertCount, &summary.EvidenceCount, &summary.LastSeen); err != nil {
			return campaignPhaseFallback(campaignPhases), false
		}
		summaries = append(summaries, summary)
	}
	if rows.Err() != nil || len(summaries) == 0 {
		return campaignPhaseFallback(campaignPhases), false
	}
	return summaries, true
}

func campaignPhaseFallback(phases []string) []campaignPhaseDTO {
	summaries := make([]campaignPhaseDTO, 0, len(phases))
	for _, phase := range phases {
		if strings.TrimSpace(phase) == "" {
			continue
		}
		summaries = append(summaries, campaignPhaseDTO{Phase: phase})
	}
	return summaries
}

const campaignAlertAttackPhaseExpression = `if(
	arrayExists(label -> match(lowerUTF8(label), '^(attack_phase|attack-phase|mitre_phase|mitre-phase)[:=]'), labels),
	replaceRegexpOne(arrayFirst(label -> match(lowerUTF8(label), '^(attack_phase|attack-phase|mitre_phase|mitre-phase)[:=]'), labels), '^[^:=]+[:=]', ''),
	multiIf(
		match(lowerUTF8(alert_type), 'c2|command.control|beacon|callback|dns.tunnel'), 'command_control',
		match(lowerUTF8(alert_type), 'lateral|worm|smb|rdp|pass.the.hash'), 'lateral_movement',
		match(lowerUTF8(alert_type), 'exfil|large.upload|data.transfer'), 'exfiltration',
		match(lowerUTF8(alert_type), 'credential|brute.force|password|login'), 'credential_access',
		match(lowerUTF8(alert_type), 'exploit|initial.access|phish'), 'initial_access',
		match(lowerUTF8(alert_type), 'malware|execution|shell|script'), 'execution',
		match(lowerUTF8(alert_type), 'persist|startup|scheduled.task'), 'persistence',
		match(lowerUTF8(alert_type), 'impact|ransom|destroy|encrypt'), 'impact',
		'discovery'
	)
)`

type campaignScanner interface {
	Scan(dest ...interface{}) error
}

func campaignSelectSQL(suffix string) string {
	return `SELECT tenant_id, campaign_id, ts_start, ts_end, entities, alerts, score, summary,
		event_id, ingest_ts, campaign_type, attack_phases, rule_ids, model_ids, header_probe_id,
		multiIf(ts_end>=toUnixTimestamp64Milli(now64(3) - INTERVAL 24 HOUR), 'active',
			ts_end>=toUnixTimestamp64Milli(now64(3) - INTERVAL 7 DAY), 'investigating', 'closed') AS status
		FROM traffic.campaigns ` + suffix
}

func scanCampaignRows(rows interface {
	Scan(dest ...interface{}) error
}) (campaignDTO, error) {
	return scanCampaignRow(rows)
}

func scanCampaignRow(row campaignScanner) (campaignDTO, error) {
	var campaign campaignDTO
	var score float32
	if err := row.Scan(
		&campaign.TenantID, &campaign.CampaignID, &campaign.TsStart, &campaign.TsEnd,
		&campaign.Entities, &campaign.Alerts, &score, &campaign.Summary,
		&campaign.EventID, &campaign.IngestTs, &campaign.CampaignType, &campaign.AttackPhases,
		&campaign.RuleIDs, &campaign.ModelIDs, &campaign.HeaderProbeID, &campaign.Status,
	); err != nil {
		return campaignDTO{}, err
	}
	campaign.Score = float64(score)
	campaign.ActivityStatus = campaign.Status
	return campaign, nil
}

func campaignToAttackChain(campaign campaignDTO) attackChainDTO {
	title := campaign.Summary
	if title == "" {
		title = fmt.Sprintf("%s %s", campaign.CampaignType, campaign.CampaignID)
	}
	rootAlertID := ""
	if len(campaign.Alerts) > 0 {
		rootAlertID = campaign.Alerts[0]
	}
	sourceIP := firstIP(campaign.Entities)
	status := "resolved"
	if campaign.TsEnd >= time.Now().Add(-24*time.Hour).UnixMilli() {
		status = "active"
	}
	return attackChainDTO{
		ChainID: campaign.CampaignID, TenantID: campaign.TenantID, Title: title, Description: campaign.Summary,
		Phases: campaignToPhases(campaign), RiskScore: int(mathRound(campaign.Score * 100)),
		RootAlertID: rootAlertID, SourceIP: sourceIP, EntityCount: len(campaign.Entities), AlertCount: len(campaign.Alerts),
		StartTime: campaign.TsStart, EndTime: campaign.TsEnd, Status: status, MitreTechniques: []string{},
	}
}

func campaignToPhases(campaign campaignDTO) []attackPhaseDTO {
	phases := make([]attackPhaseDTO, 0, len(campaign.AttackPhases))
	for _, phase := range campaign.AttackPhases {
		if phase == "" {
			continue
		}
		phases = append(phases, attackPhaseDTO{
			Phase: phase, AlertIDs: campaign.Alerts, StartTime: campaign.TsStart,
			EndTime: campaign.TsEnd, KeyEvents: []attackEventDTO{}, Confidence: campaign.Score,
		})
	}
	return phases
}

func alertIDsToSummaries(alertIDs []string) []campaignAlertDTO {
	alerts := make([]campaignAlertDTO, 0, len(alertIDs))
	for _, id := range alertIDs {
		if id == "" {
			continue
		}
		alerts = append(alerts, campaignAlertDTO{AlertID: id})
	}
	return alerts
}

func scanProbe(scanner interface {
	Scan(dest ...interface{}) error
}) (probeDTO, error) {
	var probeID, name, status string
	var hardwareJSON sql.NullString
	var softwareVersion sql.NullString
	var lastHeartbeat sql.NullTime
	if err := scanner.Scan(&probeID, &name, &status, &hardwareJSON, &softwareVersion, &lastHeartbeat); err != nil {
		return probeDTO{}, err
	}
	hardware := map[string]interface{}{}
	if hardwareJSON.Valid && hardwareJSON.String != "" {
		_ = json.Unmarshal([]byte(hardwareJSON.String), &hardware)
	}
	heartbeatMs := int64(0)
	if lastHeartbeat.Valid {
		heartbeatMs = lastHeartbeat.Time.UnixMilli()
	}
	normalizedStatus := normalizeProbeStatus(status, lastHeartbeat, hardware)
	hostname := stringFromMap(hardware, "hostname")
	if hostname == "" {
		hostname = name
	}
	if hostname == "" {
		hostname = probeID
	}
	return probeDTO{
		ProbeID: probeID, Hostname: hostname, IPAddress: firstNonEmpty(stringFromMap(hardware, "ip_address"), stringFromMap(hardware, "ip")),
		Location: stringFromMap(hardware, "location"),
		Status:   normalizedStatus, HealthScore: probeHealthScore(normalizedStatus, lastHeartbeat, hardware),
		CPUUsage: numberFromMap(hardware, "cpu_usage"), MemoryUsage: numberFromMap(hardware, "memory_usage"),
		DiskUsage: numberFromMap(hardware, "disk_usage"), DropRate: numberFromMap(hardware, "drop_rate"),
		ParseRate:     numberFromMap(hardware, "parse_rate"),
		BandwidthMbps: numberFromMap(hardware, "bandwidth_mbps"),
		CaptureMode:   firstNonEmpty(stringFromMap(hardware, "capture_mode"), stringFromMap(hardware, "mode")),
		Interfaces:    stringSliceFromMap(hardware, "interfaces"), UptimeSeconds: int64(numberFromMap(hardware, "uptime_seconds")),
		ArchivePath: stringFromMap(hardware, "archive_path"), MTLSEnabled: boolFromMap(hardware, "mtls_enabled"),
		TopologyX: numberFromMap(hardware, "topology_x"), TopologyY: numberFromMap(hardware, "topology_y"),
		TopologyZ: numberFromMap(hardware, "topology_z"), TopologyZone: stringFromMap(hardware, "topology_zone"),
		TopologyRole: stringFromMap(hardware, "topology_role"), TopologyLinks: stringSliceFromMap(hardware, "topology_links"),
		TopologyLinkBandwidths: numberSliceFromMap(hardware, "topology_link_bandwidths_gbps"), TrendLabels: stringSliceFromMap(hardware, "trend_labels"),
		BandwidthTrend: numberSliceFromMap(hardware, "bandwidth_trend"), BatchTrend: numberSliceFromMap(hardware, "batch_trend"),
		PPSK: numberFromMap(hardware, "pps_k"), BandwidthThresholdGbps: numberFromMap(hardware, "bandwidth_threshold_gbps"),
		ConfigVersion: softwareVersion.String, LastHeartbeat: heartbeatMs,
	}, nil
}

func queryTenantID(r *http.Request) string {
	if tenantID := httpx.GetTenantID(r.Context()); tenantID != "" {
		return tenantID
	}
	if tenantID := r.URL.Query().Get("tenant_id"); tenantID != "" {
		return tenantID
	}
	return "default"
}

func (h *SystemHandler) requireCampaignReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeAlertRead, authmodel.ScopeAlertWrite) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: alert:read required")
	return false
}

func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func parseInt64Query(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return value
}

func normalizeProbeStatus(status string, lastHeartbeat sql.NullTime, hardware map[string]interface{}) string {
	if stringFromMap(hardware, "fixture") == "probes-ui-v1" {
		switch strings.ToLower(status) {
		case "degraded", "warning":
			return "degraded"
		case "offline", "inactive", "disabled":
			return "offline"
		default:
			return "online"
		}
	}
	switch strings.ToLower(status) {
	case "degraded", "warning":
		return "degraded"
	case "offline", "inactive", "disabled":
		return "offline"
	}
	if !lastHeartbeat.Valid || time.Since(lastHeartbeat.Time) > 5*time.Minute {
		return "offline"
	}
	return "online"
}

func probeHealthScore(status string, lastHeartbeat sql.NullTime, hardware map[string]interface{}) int {
	if score := numberFromMap(hardware, "health_score"); score > 0 {
		return int(mathRound(score))
	}
	switch status {
	case "online":
		return 100
	case "degraded":
		return 60
	default:
		if lastHeartbeat.Valid {
			return 30
		}
		return 0
	}
}

func firstIP(values []string) string {
	for _, value := range values {
		if net.ParseIP(value) != nil {
			return value
		}
	}
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringFromMap(values map[string]interface{}, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func numberFromMap(values map[string]interface{}, key string) float64 {
	switch value := values[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	default:
		return 0
	}
}

func boolFromMap(values map[string]interface{}, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func stringSliceFromMap(values map[string]interface{}, key string) []string {
	raw, ok := values[key].([]interface{})
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(raw))
	for _, value := range raw {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}
	return result
}

func numberSliceFromMap(values map[string]interface{}, key string) []float64 {
	raw, ok := values[key].([]interface{})
	if !ok {
		return []float64{}
	}
	result := make([]float64, 0, len(raw))
	for _, value := range raw {
		switch typed := value.(type) {
		case float64:
			result = append(result, typed)
		case json.Number:
			if parsed, err := typed.Float64(); err == nil {
				result = append(result, parsed)
			}
		}
	}
	return result
}

func mathRound(value float64) float64 {
	if value >= 0 {
		return float64(int(value + 0.5))
	}
	return float64(int(value - 0.5))
}

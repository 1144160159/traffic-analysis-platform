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
	"github.com/lib/pq"
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
	r.HandleFunc("/attack-chains/{id}/evidence", h.ListAttackChainEvidence).Methods("GET")
	r.HandleFunc("/attack-chains/{id}/paths", h.ListAttackChainPaths).Methods("GET")
	r.HandleFunc("/attack-chains/{id}/recommendations", h.ListAttackChainRecommendations).Methods("GET")
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
	r.HandleFunc("/topics/scopes/{topic}", h.GetTopicScope).Methods("GET")
	r.HandleFunc("/topics/scopes/{topic}", h.UpdateTopicScope).Methods("PUT", "PATCH")
	r.HandleFunc("/topics/subscriptions", h.ListTopicSubscriptions).Methods("GET")
	r.HandleFunc("/topics/subscriptions", h.CreateTopicSubscription).Methods("POST")
	r.HandleFunc("/topics/subscriptions/{id}", h.UpdateTopicSubscription).Methods("PATCH")
	r.HandleFunc("/topics/reports/export", h.ExportTopicReport).Methods("POST")
	r.HandleFunc("/topics/evidence-packages/export", h.ExportTopicEvidencePackage).Methods("POST")
	r.HandleFunc("/topics/{topic}/actions", h.SubmitTopicAction).Methods("POST")
	r.HandleFunc("/topics/{topic}/evidence-actions", h.SubmitTopicAction).Methods("POST")
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
	TenantID           string                        `json:"tenant_id"`
	CampaignID         string                        `json:"campaign_id"`
	TsStart            int64                         `json:"ts_start"`
	TsEnd              int64                         `json:"ts_end"`
	Entities           []string                      `json:"entities"`
	AlertIDs           []string                      `json:"alert_ids"`
	Alerts             []campaignAlertDTO            `json:"alerts"`
	Score              float64                       `json:"score"`
	Summary            string                        `json:"summary"`
	EventID            string                        `json:"event_id"`
	IngestTs           int64                         `json:"ingest_ts"`
	CampaignType       string                        `json:"campaign_type"`
	AttackPhases       []string                      `json:"attack_phases"`
	RuleIDs            []string                      `json:"rule_ids"`
	ModelIDs           []string                      `json:"model_ids"`
	PhaseSummaries     []campaignPhaseDTO            `json:"phase_summaries"`
	PhaseDataBacked    bool                          `json:"phase_data_backed"`
	EvidenceSummary    []campaignEvidenceSummaryDTO  `json:"evidence_summary"`
	StatusTransitions  []campaignStatusTransitionDTO `json:"status_transitions"`
	ImpactAssets       []campaignImpactAssetDTO      `json:"impact_assets"`
	ImpactAccounts     []campaignImpactAccountDTO    `json:"impact_accounts"`
	ImpactServices     []campaignImpactServiceDTO    `json:"impact_services"`
	ImpactDepartments  []campaignImpactDepartmentDTO `json:"impact_departments"`
	ImpactCampuses     []campaignImpactCampusDTO     `json:"impact_campuses"`
	ImpactSystems      []campaignImpactSystemDTO     `json:"impact_business_systems"`
	ImpactDataBacked   map[string]bool               `json:"impact_data_backed"`
	ActivityStatus     string                        `json:"activity_status"`
	Status             string                        `json:"status"`
	Assignee           string                        `json:"assignee"`
	StateVersion       int64                         `json:"state_version"`
	WorkbenchUpdatedAt string                        `json:"workbench_updated_at,omitempty"`
}

type campaignImpactAssetDTO struct {
	Asset          string `json:"asset"`
	Type           string `json:"type"`
	Department     string `json:"department"`
	Campus         string `json:"campus"`
	BusinessSystem string `json:"business_system"`
	Owner          string `json:"owner"`
	Risk           string `json:"risk"`
	Evidence       string `json:"evidence"`
}

type campaignImpactAccountDTO struct {
	Account        string `json:"account"`
	AccountType    string `json:"account_type"`
	PermissionRisk string `json:"permission_risk"`
	LoginPath      string `json:"login_path"`
}

type campaignImpactServiceDTO struct {
	ServiceName  string `json:"service_name"`
	PortProtocol string `json:"port_protocol"`
	Risk         string `json:"risk"`
	Dependency   string `json:"dependency"`
}

type campaignImpactDepartmentDTO struct {
	Department string `json:"department"`
	Owner      string `json:"owner"`
	Risk       string `json:"risk"`
	Progress   int    `json:"progress"`
}

type campaignImpactCampusDTO struct {
	Campus        string `json:"campus"`
	CoveredAssets int    `json:"covered_assets"`
	Risk          string `json:"risk"`
	Link          string `json:"link"`
}

type campaignImpactSystemDTO struct {
	BusinessSystem   string `json:"business_system"`
	KeyService       string `json:"key_service"`
	Risk             string `json:"risk"`
	RecoveryPriority string `json:"recovery_priority"`
}

type campaignEvidenceSummaryDTO struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Current   *uint64 `json:"current,omitempty"`
	Expected  *uint64 `json:"expected,omitempty"`
	Available bool    `json:"available"`
}

type campaignStatusTransitionDTO struct {
	Status    string `json:"status"`
	ChangedAt string `json:"changed_at"`
	Source    string `json:"source"`
}

type campaignAlertDTO struct {
	AlertID       string   `json:"alert_id"`
	AlertType     string   `json:"alert_type"`
	Severity      string   `json:"severity"`
	LastSeen      int64    `json:"last_seen"`
	AttackPhase   string   `json:"attack_phase"`
	Entity        string   `json:"entity"`
	SrcIP         string   `json:"src_ip"`
	DstIP         string   `json:"dst_ip"`
	EvidenceIDs   []string `json:"evidence_ids"`
	EvidenceCount uint64   `json:"evidence_count"`
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
	EventID     string   `json:"event_id"`
	Timestamp   int64    `json:"timestamp"`
	Description string   `json:"description"`
	Entity      string   `json:"entity,omitempty"`
	SrcIP       string   `json:"src_ip"`
	DstIP       string   `json:"dst_ip"`
	Technique   string   `json:"technique,omitempty"`
	Severity    string   `json:"severity"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type attackChainEvidenceDTO struct {
	EvidenceID       string `json:"evidence_id"`
	AlertID          string `json:"alert_id"`
	Phase            string `json:"phase"`
	Type             string `json:"type"`
	Summary          string `json:"summary"`
	Timestamp        int64  `json:"timestamp"`
	Integrity        uint8  `json:"integrity"`
	VisualizationURL string `json:"visualization_url,omitempty"`
}

type attackChainPathDTO struct {
	PathID        string `json:"path_id"`
	Phase         string `json:"phase"`
	Technique     string `json:"technique"`
	Entity        string `json:"entity"`
	Alert         string `json:"alert"`
	EvidenceID    string `json:"evidence_id"`
	Action        string `json:"action"`
	Status        string `json:"status"`
	SourceIP      string `json:"source_ip"`
	DestinationIP string `json:"destination_ip"`
	Timestamp     int64  `json:"timestamp"`
}

type attackChainRecommendationDTO struct {
	RecommendationID string `json:"recommendation_id"`
	Category         string `json:"category"`
	Priority         string `json:"priority"`
	Target           string `json:"target"`
	Action           string `json:"action"`
	Impact           string `json:"impact"`
	Phase            string `json:"phase"`
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
	phaseSummaries, phaseDataBacked := h.queryCampaignPhaseSummaries(
		ctx,
		campaign.TenantID,
		campaign.CampaignID,
		campaign.Alerts,
		campaign.AttackPhases,
	)
	statusTransitions, responseRecordCount, responseRecordsAvailable := h.queryCampaignAuditState(
		ctx,
		campaign.TenantID,
		campaign.CampaignID,
		campaign.TsStart,
		campaign.TsEnd,
		campaign.Status,
		campaign.WorkbenchUpdatedAt,
	)
	impactObservations, networkEntities, observationDataBacked := h.queryCampaignImpactObservations(
		ctx,
		campaign.TenantID,
		campaign.CampaignID,
		campaign.Alerts,
	)
	impactAccounts, accountDataBacked := h.queryCampaignImpactAccounts(
		ctx,
		campaign.TenantID,
		networkEntities,
		campaign.TsStart,
		campaign.TsEnd,
	)
	observedServices := campaignImpactServicesFromObservations(impactObservations)
	impactAssets, assetAccounts, assetServices, impactDepartments, impactCampuses, impactSystems, assetDataBacked := h.queryCampaignImpactAssets(
		ctx,
		campaign.TenantID,
		append(append([]string{}, campaign.Entities...), networkEntities...),
		campaignStatusProgress(campaign.Status),
	)
	impactAccounts = mergeCampaignImpactAccounts(impactAccounts, assetAccounts)
	impactServices := mergeCampaignImpactServices(observedServices, assetServices)
	httpx.JSONSuccess(w, ctx, campaignDetailDTO{
		TenantID: campaign.TenantID, CampaignID: campaign.CampaignID,
		TsStart: campaign.TsStart, TsEnd: campaign.TsEnd,
		Entities: campaign.Entities, AlertIDs: campaign.Alerts, Alerts: alerts,
		Score: campaign.Score, Summary: campaign.Summary, EventID: campaign.EventID, IngestTs: campaign.IngestTs,
		CampaignType: campaign.CampaignType, AttackPhases: campaign.AttackPhases, RuleIDs: campaign.RuleIDs, ModelIDs: campaign.ModelIDs,
		PhaseSummaries:    phaseSummaries,
		PhaseDataBacked:   phaseDataBacked,
		EvidenceSummary:   campaignEvidenceSummary(alerts, phaseSummaries, phaseDataBacked, responseRecordCount, responseRecordsAvailable),
		StatusTransitions: statusTransitions,
		ImpactAssets:      impactAssets,
		ImpactAccounts:    impactAccounts,
		ImpactServices:    impactServices,
		ImpactDepartments: impactDepartments,
		ImpactCampuses:    impactCampuses,
		ImpactSystems:     impactSystems,
		ImpactDataBacked: map[string]bool{
			"accounts":         accountDataBacked || assetDataBacked,
			"services":         observationDataBacked || assetDataBacked,
			"assets":           assetDataBacked,
			"departments":      assetDataBacked,
			"campuses":         assetDataBacked,
			"business_systems": assetDataBacked,
		},
		ActivityStatus: campaign.ActivityStatus,
		Status:         campaign.Status,
		Assignee:       campaign.Assignee, StateVersion: campaign.StateVersion,
		WorkbenchUpdatedAt: campaign.WorkbenchUpdatedAt,
	})
}

func (h *SystemHandler) queryCampaignImpactAssets(
	ctx context.Context,
	tenantID string,
	entities []string,
	responseProgress int,
) ([]campaignImpactAssetDTO, []campaignImpactAccountDTO, []campaignImpactServiceDTO, []campaignImpactDepartmentDTO, []campaignImpactCampusDTO, []campaignImpactSystemDTO, bool) {
	if h.pgDB == nil || len(entities) == 0 {
		return []campaignImpactAssetDTO{}, []campaignImpactAccountDTO{}, []campaignImpactServiceDTO{}, []campaignImpactDepartmentDTO{}, []campaignImpactCampusDTO{}, []campaignImpactSystemDTO{}, false
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(display_code,''), NULLIF(hostname,''), NULLIF(ip_address,''), NULLIF(ip,''), asset_id::text),
		       asset_type, COALESCE(department,''), COALESCE(campus,''), COALESCE(owner,''),
		       criticality, COALESCE(metadata,'{}'::jsonb)::text
		FROM assets
		WHERE tenant_id=$1
		  AND (
		    asset_id::text = ANY($2) OR display_code = ANY($2) OR hostname = ANY($2)
		    OR ip_address = ANY($2) OR ip = ANY($2)
		  )
		ORDER BY criticality DESC, last_seen DESC`, tenantID, pq.Array(entities))
	if err != nil {
		if h.logger != nil {
			h.logger.Debug("campaign impact asset query unavailable", zap.Error(err), zap.String("campaign_tenant", tenantID))
		}
		return []campaignImpactAssetDTO{}, []campaignImpactAccountDTO{}, []campaignImpactServiceDTO{}, []campaignImpactDepartmentDTO{}, []campaignImpactCampusDTO{}, []campaignImpactSystemDTO{}, false
	}
	defer rows.Close()

	assets := make([]campaignImpactAssetDTO, 0)
	accounts := make([]campaignImpactAccountDTO, 0)
	services := make([]campaignImpactServiceDTO, 0)
	departmentMap := map[string]campaignImpactDepartmentDTO{}
	campusMap := map[string]campaignImpactCampusDTO{}
	systemMap := map[string]campaignImpactSystemDTO{}
	for rows.Next() {
		var assetName, assetType, department, campus, owner, metadataJSON string
		var criticality int
		if scanErr := rows.Scan(&assetName, &assetType, &department, &campus, &owner, &criticality, &metadataJSON); scanErr != nil {
			continue
		}
		metadata := map[string]interface{}{}
		_ = json.Unmarshal([]byte(metadataJSON), &metadata)
		risk := campaignAssetRiskLabel(criticality)
		businessSystem := firstNonEmpty(stringFromMap(metadata, "business_system"), stringFromMap(metadata, "system"))
		keyService := firstNonEmpty(stringFromMap(metadata, "key_service"), stringFromMap(metadata, "service"))
		accounts = mergeCampaignImpactAccounts(accounts, campaignImpactAccountsFromMetadata(metadata))
		services = mergeCampaignImpactServices(services, campaignImpactServicesFromMetadata(metadata, businessSystem))
		assets = append(assets, campaignImpactAssetDTO{
			Asset: assetName, Type: assetType, Department: department, Campus: campus,
			BusinessSystem: businessSystem, Owner: owner, Risk: risk, Evidence: "PostgreSQL assets",
		})
		if department != "" {
			current := departmentMap[department]
			if current.Department == "" || campaignRiskRank(risk) > campaignRiskRank(current.Risk) {
				departmentMap[department] = campaignImpactDepartmentDTO{
					Department: department,
					Owner:      owner,
					Risk:       risk,
					Progress:   responseProgress,
				}
			}
		}
		if campus != "" {
			current := campusMap[campus]
			current.Campus = campus
			current.CoveredAssets++
			if campaignRiskRank(risk) > campaignRiskRank(current.Risk) {
				current.Risk = risk
			}
			current.Link = stringFromMap(metadata, "network_path")
			campusMap[campus] = current
		}
		if businessSystem != "" {
			current := systemMap[businessSystem]
			current.BusinessSystem = businessSystem
			if current.KeyService == "" {
				current.KeyService = keyService
			}
			if campaignRiskRank(risk) > campaignRiskRank(current.Risk) {
				current.Risk = risk
			}
			current.RecoveryPriority = stringFromMap(metadata, "recovery_priority")
			systemMap[businessSystem] = current
		}
	}
	departments := make([]campaignImpactDepartmentDTO, 0, len(departmentMap))
	for _, item := range departmentMap {
		departments = append(departments, item)
	}
	campuses := make([]campaignImpactCampusDTO, 0, len(campusMap))
	for _, item := range campusMap {
		campuses = append(campuses, item)
	}
	systems := make([]campaignImpactSystemDTO, 0, len(systemMap))
	for _, item := range systemMap {
		systems = append(systems, item)
	}
	return assets, accounts, services, departments, campuses, systems, rows.Err() == nil
}

type campaignImpactObservation struct {
	CommunityID string
	SrcIP       string
	DstIP       string
	DstPort     uint16
	Protocol    uint8
	Severity    string
}

func (h *SystemHandler) queryCampaignImpactObservations(
	ctx context.Context,
	tenantID, campaignID string,
	alertIDs []string,
) ([]campaignImpactObservation, []string, bool) {
	if h.chClient == nil {
		return []campaignImpactObservation{}, []string{}, false
	}
	rows, err := h.chClient.Query(ctx, `
		SELECT
			argMax(community_id, last_seen) AS latest_community_id,
			argMax(src_ip, last_seen) AS latest_src_ip,
			argMax(dst_ip, last_seen) AS latest_dst_ip,
			toUInt16(argMax(dst_port, last_seen)) AS latest_dst_port,
			toUInt8(argMax(protocol, last_seen)) AS latest_protocol,
			argMax(severity, last_seen) AS latest_severity
		FROM traffic.alerts
		WHERE tenant_id=? AND (campaign_id=? OR has(?, alert_id))
		GROUP BY alert_id
		ORDER BY max(last_seen) DESC
		LIMIT 500`, tenantID, campaignID, alertIDs)
	if err != nil {
		return []campaignImpactObservation{}, []string{}, false
	}
	defer rows.Close()

	observations := make([]campaignImpactObservation, 0)
	entitySet := map[string]struct{}{}
	for rows.Next() {
		var observation campaignImpactObservation
		if err := rows.Scan(
			&observation.CommunityID,
			&observation.SrcIP,
			&observation.DstIP,
			&observation.DstPort,
			&observation.Protocol,
			&observation.Severity,
		); err != nil {
			return []campaignImpactObservation{}, []string{}, false
		}
		observations = append(observations, observation)
		if observation.SrcIP != "" {
			entitySet[observation.SrcIP] = struct{}{}
		}
		if observation.DstIP != "" {
			entitySet[observation.DstIP] = struct{}{}
		}
	}
	if rows.Err() != nil {
		return []campaignImpactObservation{}, []string{}, false
	}
	networkEntities := make([]string, 0, len(entitySet))
	for entity := range entitySet {
		networkEntities = append(networkEntities, entity)
	}
	enriched, enrichedEntities, enrichedOK := h.queryCampaignImpactSessions(ctx, tenantID, observations)
	if enrichedOK && len(enriched) > 0 {
		return enriched, enrichedEntities, true
	}
	return observations, networkEntities, true
}

func (h *SystemHandler) queryCampaignImpactSessions(
	ctx context.Context,
	tenantID string,
	alertObservations []campaignImpactObservation,
) ([]campaignImpactObservation, []string, bool) {
	communityIDs := make([]string, 0, len(alertObservations))
	severityByCommunityID := make(map[string]string, len(alertObservations))
	for _, observation := range alertObservations {
		if observation.CommunityID == "" {
			continue
		}
		communityIDs = append(communityIDs, observation.CommunityID)
		currentSeverity, exists := severityByCommunityID[observation.CommunityID]
		if !exists || campaignRiskRank(campaignAlertRiskLabel(observation.Severity)) >
			campaignRiskRank(campaignAlertRiskLabel(currentSeverity)) {
			severityByCommunityID[observation.CommunityID] = observation.Severity
		}
	}
	if len(communityIDs) == 0 {
		return []campaignImpactObservation{}, []string{}, false
	}

	rows, err := h.chClient.Query(ctx, `
		SELECT
			community_id,
			argMax(src_ip, ts_end) AS latest_src_ip,
			argMax(dst_ip, ts_end) AS latest_dst_ip,
			toUInt16(argMax(dst_port, ts_end)) AS latest_dst_port,
			toUInt8(argMax(protocol, ts_end)) AS latest_protocol
		FROM traffic.sessions
		WHERE tenant_id=? AND has(?, community_id)
		GROUP BY community_id
		ORDER BY max(ts_end) DESC
		LIMIT 500`, tenantID, communityIDs)
	if err != nil {
		return []campaignImpactObservation{}, []string{}, false
	}
	defer rows.Close()

	observations := make([]campaignImpactObservation, 0)
	entitySet := map[string]struct{}{}
	for rows.Next() {
		var observation campaignImpactObservation
		if err := rows.Scan(
			&observation.CommunityID,
			&observation.SrcIP,
			&observation.DstIP,
			&observation.DstPort,
			&observation.Protocol,
		); err != nil {
			return []campaignImpactObservation{}, []string{}, false
		}
		observation.Severity = severityByCommunityID[observation.CommunityID]
		observations = append(observations, observation)
		if observation.SrcIP != "" {
			entitySet[observation.SrcIP] = struct{}{}
		}
		if observation.DstIP != "" {
			entitySet[observation.DstIP] = struct{}{}
		}
	}
	if rows.Err() != nil {
		return []campaignImpactObservation{}, []string{}, false
	}
	entities := make([]string, 0, len(entitySet))
	for entity := range entitySet {
		entities = append(entities, entity)
	}
	return observations, entities, true
}

func (h *SystemHandler) queryCampaignImpactAccounts(
	ctx context.Context,
	tenantID string,
	networkEntities []string,
	tsStart, tsEnd int64,
) ([]campaignImpactAccountDTO, bool) {
	if h.chClient == nil || len(networkEntities) == 0 || tsStart <= 0 || tsEnd <= 0 {
		return []campaignImpactAccountDTO{}, false
	}
	rows, err := h.chClient.Query(ctx, `
		SELECT
			username,
			argMax(user_id, timestamp) AS latest_user_id,
			argMax(resource, timestamp) AS latest_resource,
			argMax(result, timestamp) AS latest_result,
			argMax(source_ip, timestamp) AS latest_source_ip,
			toUInt64(countIf(lowerUTF8(result) NOT IN ('success','ok','allow','allowed','passed','通过','成功'))) AS failure_count
		FROM traffic.user_events
		WHERE tenant_id=?
		  AND timestamp BETWEEN toDateTime(intDiv(?, 1000)) - INTERVAL 1 HOUR
		                    AND toDateTime(intDiv(?, 1000)) + INTERVAL 1 HOUR
		  AND has(?, source_ip)
		  AND username != ''
		GROUP BY username
		ORDER BY failure_count DESC, max(timestamp) DESC
		LIMIT 100`, tenantID, tsStart, tsEnd, networkEntities)
	if err != nil {
		return []campaignImpactAccountDTO{}, false
	}
	defer rows.Close()

	accounts := make([]campaignImpactAccountDTO, 0)
	for rows.Next() {
		var username, userID, resource, result, sourceIP string
		var failureCount uint64
		if err := rows.Scan(&username, &userID, &resource, &result, &sourceIP, &failureCount); err != nil {
			return []campaignImpactAccountDTO{}, false
		}
		accounts = append(accounts, campaignImpactAccountDTO{
			Account:        username,
			AccountType:    campaignAccountType(username, userID),
			PermissionRisk: campaignAccountRisk(username, result, failureCount),
			LoginPath:      campaignLoginPath(sourceIP, resource),
		})
	}
	if rows.Err() != nil {
		return []campaignImpactAccountDTO{}, false
	}
	return accounts, true
}

func campaignImpactServicesFromObservations(observations []campaignImpactObservation) []campaignImpactServiceDTO {
	services := make([]campaignImpactServiceDTO, 0)
	serviceIndex := map[string]int{}
	for _, observation := range observations {
		if observation.DstPort == 0 {
			continue
		}
		protocol := campaignTransportProtocol(observation.Protocol)
		key := fmt.Sprintf("%d/%s", observation.DstPort, protocol)
		risk := campaignAlertRiskLabel(observation.Severity)
		if index, ok := serviceIndex[key]; ok {
			if campaignRiskRank(risk) > campaignRiskRank(services[index].Risk) {
				services[index].Risk = risk
			}
			continue
		}
		serviceIndex[key] = len(services)
		services = append(services, campaignImpactServiceDTO{
			ServiceName:  campaignServiceName(observation.DstPort),
			PortProtocol: key,
			Risk:         risk,
			Dependency:   observation.DstIP,
		})
	}
	return services
}

func campaignImpactAccountsFromMetadata(metadata map[string]interface{}) []campaignImpactAccountDTO {
	raw, ok := metadata["campaign_accounts"].([]interface{})
	if !ok {
		return []campaignImpactAccountDTO{}
	}
	accounts := make([]campaignImpactAccountDTO, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		account := firstNonEmpty(stringFromMap(record, "account"), stringFromMap(record, "username"))
		if account == "" {
			continue
		}
		accounts = append(accounts, campaignImpactAccountDTO{
			Account:        account,
			AccountType:    firstNonEmpty(stringFromMap(record, "account_type"), campaignAccountType(account, stringFromMap(record, "user_id"))),
			PermissionRisk: firstNonEmpty(stringFromMap(record, "permission_risk"), stringFromMap(record, "risk"), "低危"),
			LoginPath:      firstNonEmpty(stringFromMap(record, "login_path"), stringFromMap(record, "access_path")),
		})
	}
	return accounts
}

func campaignImpactServicesFromMetadata(metadata map[string]interface{}, businessSystem string) []campaignImpactServiceDTO {
	raw, ok := metadata["open_services"].([]interface{})
	if !ok {
		return []campaignImpactServiceDTO{}
	}
	services := make([]campaignImpactServiceDTO, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		port := uint16(intFromMap(record, "port"))
		protocol := strings.ToUpper(firstNonEmpty(stringFromMap(record, "protocol"), "TCP"))
		name := firstNonEmpty(stringFromMap(record, "service"), stringFromMap(record, "name"))
		if name == "" && port > 0 {
			name = campaignServiceName(port)
		}
		if name == "" {
			continue
		}
		portProtocol := protocol
		if port > 0 {
			portProtocol = fmt.Sprintf("%d/%s", port, protocol)
		}
		services = append(services, campaignImpactServiceDTO{
			ServiceName:  name,
			PortProtocol: portProtocol,
			Risk:         firstNonEmpty(stringFromMap(record, "risk_level"), stringFromMap(record, "risk"), "低危"),
			Dependency:   firstNonEmpty(stringFromMap(record, "dependency"), businessSystem),
		})
	}
	return services
}

func mergeCampaignImpactAccounts(groups ...[]campaignImpactAccountDTO) []campaignImpactAccountDTO {
	merged := make([]campaignImpactAccountDTO, 0)
	indexByAccount := map[string]int{}
	for _, group := range groups {
		for _, account := range group {
			key := strings.ToLower(strings.TrimSpace(account.Account))
			if key == "" {
				continue
			}
			if index, ok := indexByAccount[key]; ok {
				if campaignRiskRank(account.PermissionRisk) > campaignRiskRank(merged[index].PermissionRisk) {
					merged[index] = account
				}
				continue
			}
			indexByAccount[key] = len(merged)
			merged = append(merged, account)
		}
	}
	return merged
}

func mergeCampaignImpactServices(groups ...[]campaignImpactServiceDTO) []campaignImpactServiceDTO {
	merged := make([]campaignImpactServiceDTO, 0)
	indexByService := map[string]int{}
	for _, group := range groups {
		for _, service := range group {
			key := strings.ToLower(strings.TrimSpace(service.PortProtocol))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(service.ServiceName))
			}
			if key == "" {
				continue
			}
			if index, ok := indexByService[key]; ok {
				if campaignRiskRank(service.Risk) > campaignRiskRank(merged[index].Risk) {
					merged[index] = service
				}
				continue
			}
			indexByService[key] = len(merged)
			merged = append(merged, service)
		}
	}
	return merged
}

func intFromMap(record map[string]interface{}, key string) int {
	switch value := record[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case json.Number:
		number, _ := value.Int64()
		return int(number)
	case string:
		number, _ := strconv.Atoi(value)
		return number
	default:
		return 0
	}
}

func campaignStatusProgress(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "closed":
		return 100
	case "remediated":
		return 90
	case "contained":
		return 70
	case "investigating", "active":
		return 40
	case "triaged":
		return 25
	default:
		return 10
	}
}

func campaignTransportProtocol(protocol uint8) string {
	switch protocol {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	default:
		return fmt.Sprintf("IP-%d", protocol)
	}
}

func campaignServiceName(port uint16) string {
	switch port {
	case 22:
		return "SSH"
	case 53:
		return "DNS"
	case 80:
		return "HTTP"
	case 389:
		return "LDAP"
	case 443:
		return "HTTPS"
	case 2049:
		return "NFS"
	case 3306:
		return "MySQL"
	case 5432:
		return "PostgreSQL"
	case 6379:
		return "Redis"
	case 9000:
		return "MinIO API"
	case 9092:
		return "Kafka"
	case 9200:
		return "OpenSearch"
	default:
		return fmt.Sprintf("Service %d", port)
	}
}

func campaignAlertRiskLabel(severity string) string {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(severity)), "severity_")
	switch normalized {
	case "critical", "high", "严重", "高危":
		return "高危"
	case "medium", "中危":
		return "中危"
	default:
		return "低危"
	}
}

func campaignAccountType(username, userID string) string {
	normalized := strings.ToLower(strings.TrimSpace(username))
	if strings.HasPrefix(normalized, "svc_") || strings.HasPrefix(normalized, "service_") {
		return "服务账号"
	}
	if strings.Contains(normalized, "admin") || strings.Contains(normalized, "root") {
		return "管理账号"
	}
	if strings.TrimSpace(userID) == "" {
		return "未知账号"
	}
	return "人员账号"
}

func campaignAccountRisk(username, result string, failureCount uint64) string {
	normalized := strings.ToLower(strings.TrimSpace(username))
	if failureCount >= 3 || strings.Contains(normalized, "admin") || strings.Contains(normalized, "root") {
		return "高危"
	}
	result = strings.ToLower(strings.TrimSpace(result))
	if failureCount > 0 || strings.Contains(result, "fail") || strings.Contains(result, "deny") || strings.HasPrefix(normalized, "svc_") {
		return "中危"
	}
	return "低危"
}

func campaignLoginPath(sourceIP, resource string) string {
	sourceIP = strings.TrimSpace(sourceIP)
	resource = strings.TrimSpace(resource)
	if sourceIP == "" {
		return resource
	}
	if resource == "" {
		return sourceIP
	}
	return sourceIP + " -> " + resource
}

func campaignAssetRiskLabel(criticality int) string {
	if criticality >= 4 {
		return "高危"
	}
	if criticality >= 2 {
		return "中危"
	}
	return "低危"
}

func campaignRiskRank(risk string) int {
	if strings.Contains(risk, "高") {
		return 3
	}
	if strings.Contains(risk, "中") {
		return 2
	}
	if strings.Contains(risk, "低") {
		return 1
	}
	return 0
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
		alerts := h.queryCampaignAlerts(ctx, campaign.TenantID, campaign.CampaignID, campaign.Alerts)
		chain := campaignToAttackChain(campaign)
		chain.Phases = campaignToPhasesWithAlerts(campaign, alerts)
		chains = append(chains, chain)
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
	alerts := h.queryCampaignAlerts(ctx, campaign.TenantID, campaign.CampaignID, campaign.Alerts)
	chain := campaignToAttackChain(campaign)
	chain.Phases = campaignToPhasesWithAlerts(campaign, alerts)
	httpx.JSONSuccess(w, ctx, chain)
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

func (h *SystemHandler) ListAttackChainEvidence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	campaignID := mux.Vars(r)["id"]
	campaign, err := h.queryCampaignByID(ctx, queryTenantID(r), campaignID)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	evidenceType, err := normalizeAttackChainEvidenceType(r.URL.Query().Get("type"))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	phase := strings.TrimSpace(r.URL.Query().Get("phase"))
	alerts := h.queryCampaignAlerts(ctx, campaign.TenantID, campaign.CampaignID, campaign.Alerts)
	phaseByAlertID := make(map[string]string, len(alerts))
	phaseAlertIDs := make([]string, 0)
	for _, alert := range alerts {
		phaseByAlertID[alert.AlertID] = alert.AttackPhase
		if phase != "" && strings.EqualFold(strings.TrimSpace(alert.AttackPhase), phase) {
			phaseAlertIDs = append(phaseAlertIDs, alert.AlertID)
		}
	}
	if phase != "" && len(phaseAlertIDs) == 0 {
		httpx.JSONSuccess(w, ctx, map[string]interface{}{
			"items": []attackChainEvidenceDTO{}, "total": 0, "limit": 0, "offset": 0,
		})
		return
	}
	limit, offset := parseLimitOffset(r, 4, 50)
	items, total, err := h.queryAttackChainEvidence(
		ctx, campaign.TenantID, campaign.CampaignID, evidenceType, phaseAlertIDs, limit, offset,
	)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	for index := range items {
		items[index].Phase = phaseByAlertID[items[index].AlertID]
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

func (h *SystemHandler) ListAttackChainPaths(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	campaignID := mux.Vars(r)["id"]
	campaign, err := h.queryCampaignByID(ctx, queryTenantID(r), campaignID)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	limit, offset := parseLimitOffset(r, 3, 50)
	alerts, total, err := h.queryCampaignAlertsPage(
		ctx, campaign.TenantID, campaign.CampaignID, campaign.Alerts,
		strings.TrimSpace(r.URL.Query().Get("phase")), limit, offset,
	)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	items := make([]attackChainPathDTO, 0, len(alerts))
	for index, alert := range alerts {
		items = append(items, attackChainPathFromAlert(alert, offset+index))
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

func (h *SystemHandler) ListAttackChainRecommendations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireCampaignReadPermission(w, r) {
		return
	}
	category, err := normalizeAttackChainRecommendationCategory(r.URL.Query().Get("category"))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	campaignID := mux.Vars(r)["id"]
	campaign, err := h.queryCampaignByID(ctx, queryTenantID(r), campaignID)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	limit, offset := parseLimitOffset(r, 6, 50)
	items, total, err := h.queryAttackChainRecommendations(
		ctx, campaign.TenantID, campaign.CampaignID, category,
		strings.TrimSpace(r.URL.Query().Get("phase")), limit, offset,
	)
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"items": items, "category": category, "total": total, "limit": limit, "offset": offset,
	})
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
		SELECT alert_id,
			argMax(alert_type, last_seen) AS latest_alert_type,
			argMax(severity, last_seen) AS latest_severity,
			max(last_seen) AS latest_seen,
			argMax(`+campaignAlertAttackPhaseExpression+`, last_seen) AS latest_attack_phase,
			argMax(`+campaignAlertEntityExpression+`, last_seen) AS latest_entity,
			argMax(src_ip, last_seen) AS latest_src_ip,
			argMax(dst_ip, last_seen) AS latest_dst_ip,
			argMax(evidence_ids, last_seen) AS latest_evidence_ids,
			toUInt64(max(length(evidence_ids))) AS evidence_count
		FROM traffic.alerts
		WHERE tenant_id=? AND (campaign_id=? OR has(?, alert_id))
		GROUP BY alert_id
		ORDER BY latest_seen DESC LIMIT 200`, tenantID, campaignID, alertIDs)
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
			&alert.Entity,
			&alert.SrcIP,
			&alert.DstIP,
			&alert.EvidenceIDs,
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

func (h *SystemHandler) queryCampaignAlertsPage(
	ctx context.Context,
	tenantID, campaignID string,
	alertIDs []string,
	phase string,
	limit, offset int,
) ([]campaignAlertDTO, int64, error) {
	where := "tenant_id=? AND (campaign_id=? OR has(?, alert_id))"
	args := []interface{}{tenantID, campaignID, alertIDs}
	if phase != "" {
		where += " AND " + campaignAlertAttackPhaseExpression + "=?"
		args = append(args, phase)
	}
	var total uint64
	countRow, err := h.chClient.QueryRow(ctx, `
		SELECT count()
		FROM (
			SELECT alert_id
			FROM traffic.alerts
			WHERE `+where+`
			GROUP BY alert_id
		)`, args...)
	if err != nil {
		return nil, 0, err
	}
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := h.chClient.Query(ctx, `
		SELECT alert_id,
			argMax(alert_type, last_seen) AS latest_alert_type,
			argMax(severity, last_seen) AS latest_severity,
			max(last_seen) AS latest_seen,
			argMax(`+campaignAlertAttackPhaseExpression+`, last_seen) AS latest_attack_phase,
			argMax(`+campaignAlertEntityExpression+`, last_seen) AS latest_entity,
			argMax(src_ip, last_seen) AS latest_src_ip,
			argMax(dst_ip, last_seen) AS latest_dst_ip,
			argMax(evidence_ids, last_seen) AS latest_evidence_ids,
			toUInt64(max(length(evidence_ids))) AS evidence_count
		FROM traffic.alerts
		WHERE `+where+`
		GROUP BY alert_id
		ORDER BY latest_seen ASC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	alerts := make([]campaignAlertDTO, 0, limit)
	for rows.Next() {
		var alert campaignAlertDTO
		if err := rows.Scan(
			&alert.AlertID,
			&alert.AlertType,
			&alert.Severity,
			&alert.LastSeen,
			&alert.AttackPhase,
			&alert.Entity,
			&alert.SrcIP,
			&alert.DstIP,
			&alert.EvidenceIDs,
			&alert.EvidenceCount,
		); err != nil {
			return nil, 0, err
		}
		alerts = append(alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return alerts, int64(total), nil
}

func (h *SystemHandler) queryAttackChainEvidence(
	ctx context.Context,
	tenantID, campaignID, evidenceType string,
	phaseAlertIDs []string,
	limit, offset int,
) ([]attackChainEvidenceDTO, int64, error) {
	where := "tenant_id=? AND event_id=?"
	args := []interface{}{tenantID, campaignID}
	switch evidenceType {
	case "alert":
		where += " AND lowerUTF8(type) IN ('alert', '告警')"
	case "pcap":
		where += " AND (lowerUTF8(type)='pcap' OR positionCaseInsensitiveUTF8(summary, '.pcap')>0)"
	case "session":
		where += " AND (positionCaseInsensitiveUTF8(type, 'session')>0 OR positionUTF8(type, '会话')>0)"
	case "log":
		where += " AND (lowerUTF8(type)='log' OR positionUTF8(type, '日志')>0 OR positionCaseInsensitiveUTF8(summary, '.log')>0)"
	case "graph":
		where += " AND (positionCaseInsensitiveUTF8(type, 'graph')>0 OR positionUTF8(type, '图谱')>0)"
	case "rule_model":
		where += " AND (positionCaseInsensitiveUTF8(type, 'rule')>0 OR positionCaseInsensitiveUTF8(type, 'model')>0 OR positionUTF8(type, '规则')>0 OR positionUTF8(type, '模型')>0)"
	}
	if len(phaseAlertIDs) > 0 {
		where += " AND has(?, alert_id)"
		args = append(args, phaseAlertIDs)
	}
	var total uint64
	countRow, err := h.chClient.QueryRow(ctx, `
		SELECT uniqExact(evidence_id)
		FROM traffic.evidence
		WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := h.chClient.Query(ctx, `
		SELECT evidence_id,
			argMax(alert_id, ingest_ts) AS latest_alert_id,
			argMax(type, ingest_ts) AS latest_type,
			argMax(summary, ingest_ts) AS latest_summary,
			max(ts) AS latest_ts,
			toUInt8(round(max(confidence)*100)) AS integrity,
			argMax(visualization_url, ingest_ts) AS latest_visualization_url
		FROM traffic.evidence
		WHERE `+where+`
		GROUP BY evidence_id
		ORDER BY latest_ts ASC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]attackChainEvidenceDTO, 0, limit)
	for rows.Next() {
		var item attackChainEvidenceDTO
		if err := rows.Scan(
			&item.EvidenceID,
			&item.AlertID,
			&item.Type,
			&item.Summary,
			&item.Timestamp,
			&item.Integrity,
			&item.VisualizationURL,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, int64(total), nil
}

func (h *SystemHandler) queryAttackChainRecommendations(
	ctx context.Context,
	tenantID, campaignID, category, phase string,
	limit, offset int,
) ([]attackChainRecommendationDTO, int64, error) {
	where := "tenant_id=? AND campaign_id=? AND category=?"
	args := []interface{}{tenantID, campaignID, category}
	if phase != "" {
		where += " AND lowerUTF8(phase)=lowerUTF8(?)"
		args = append(args, phase)
	}

	var total uint64
	countRow, err := h.chClient.QueryRow(ctx, `
		SELECT uniqExact(recommendation_id)
		FROM traffic.attack_chain_recommendations
		WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := h.chClient.Query(ctx, `
		SELECT recommendation_id,
			argMax(category, created_at) AS latest_category,
			argMax(priority, created_at) AS latest_priority,
			argMax(target, created_at) AS latest_target,
			argMax(action, created_at) AS latest_action,
			argMax(impact, created_at) AS latest_impact,
			argMax(phase, created_at) AS latest_phase,
			max(sort_order) AS latest_sort_order
		FROM traffic.attack_chain_recommendations
		WHERE `+where+`
		GROUP BY recommendation_id
		ORDER BY latest_sort_order ASC, recommendation_id ASC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]attackChainRecommendationDTO, 0, limit)
	for rows.Next() {
		var item attackChainRecommendationDTO
		var sortOrder uint16
		if err := rows.Scan(
			&item.RecommendationID,
			&item.Category,
			&item.Priority,
			&item.Target,
			&item.Action,
			&item.Impact,
			&item.Phase,
			&sortOrder,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, int64(total), nil
}

func normalizeAttackChainEvidenceType(value string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", "all", "全部":
		return "", nil
	case "alert", "告警":
		return "alert", nil
	case "pcap":
		return "pcap", nil
	case "session":
		return "session", nil
	case "log", "日志":
		return "log", nil
	case "graph", "图谱":
		return "graph", nil
	case "rule_model", "rule/model", "规则/模型":
		return "rule_model", nil
	default:
		return "", fmt.Errorf("unsupported attack-chain evidence type")
	}
}

func normalizeAttackChainRecommendationCategory(value string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", "block", "阻断点":
		return "block", nil
	case "isolate", "隔离建议":
		return "isolate", nil
	case "allowlist", "白名单风险":
		return "allowlist", nil
	case "playbook", "剧本推荐":
		return "playbook", nil
	default:
		return "", fmt.Errorf("unsupported attack-chain recommendation category")
	}
}

func attackChainPathFromAlert(alert campaignAlertDTO, index int) attackChainPathDTO {
	evidenceID := ""
	if len(alert.EvidenceIDs) > 0 {
		evidenceID = alert.EvidenceIDs[0]
	}
	return attackChainPathDTO{
		PathID:        fmt.Sprintf("path-%d-%s", index+1, alert.AlertID),
		Phase:         alert.AttackPhase,
		Technique:     mitreTechniqueForAttackPhase(alert.AttackPhase),
		Entity:        firstNonEmpty(alert.Entity, alert.DstIP, alert.SrcIP),
		Alert:         alert.AlertType,
		EvidenceID:    evidenceID,
		Action:        attackChainRecommendationAction("block", alert),
		Status:        "confirmed",
		SourceIP:      alert.SrcIP,
		DestinationIP: alert.DstIP,
		Timestamp:     alert.LastSeen,
	}
}

func attackChainRecommendationFromAlert(category string, alert campaignAlertDTO, index int) attackChainRecommendationDTO {
	target := firstNonEmpty(alert.Entity, alert.DstIP, alert.SrcIP)
	impact := "低影响"
	switch category {
	case "isolate":
		impact = "中等影响"
	case "allowlist":
		impact = "需审批"
	case "playbook":
		impact = "自动化"
	}
	return attackChainRecommendationDTO{
		RecommendationID: fmt.Sprintf("%s-%d-%s", category, index+1, alert.AlertID),
		Category:         category,
		Priority:         attackChainRecommendationPriority(alert.Severity),
		Target:           target,
		Action:           attackChainRecommendationAction(category, alert),
		Impact:           impact,
		Phase:            alert.AttackPhase,
	}
}

func attackChainRecommendationPriority(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "高危":
		return "高"
	case "medium", "中危":
		return "中"
	default:
		return "低"
	}
}

func attackChainRecommendationAction(category string, alert campaignAlertDTO) string {
	target := firstNonEmpty(alert.Entity, alert.DstIP, alert.SrcIP)
	switch category {
	case "isolate":
		return "隔离 " + target
	case "allowlist":
		return "复核白名单 " + target
	case "playbook":
		return firstNonEmpty(attackChainPlaybookName(alert.AttackPhase), "执行通用研判剧本")
	default:
		if strings.Contains(strings.ToLower(alert.AttackPhase), "initial") {
			return "加固入口 " + target
		}
		return "阻断 " + target
	}
}

func attackChainPlaybookName(phase string) string {
	normalized := strings.ToLower(strings.TrimSpace(phase))
	switch {
	case strings.Contains(normalized, "recon"):
		return "执行扫描源封禁剧本"
	case strings.Contains(normalized, "initial"), strings.Contains(normalized, "access"):
		return "执行入口加固剧本"
	case strings.Contains(normalized, "execution"):
		return "执行恶意进程处置剧本"
	case strings.Contains(normalized, "lateral"):
		return "执行横向移动隔离剧本"
	case strings.Contains(normalized, "command"), strings.Contains(normalized, "control"), strings.Contains(normalized, "c2"):
		return "执行 C2 阻断剧本"
	case strings.Contains(normalized, "exfil"):
		return "执行数据外传阻断剧本"
	default:
		return ""
	}
}

func (h *SystemHandler) queryCampaignPhaseSummaries(
	ctx context.Context,
	tenantID, campaignID string,
	alertIDs []string,
	campaignPhases []string,
) ([]campaignPhaseDTO, bool) {
	rows, err := h.chClient.Query(ctx, `
		SELECT attack_phase,
			toUInt64(count()) AS alert_count,
			toUInt64(sum(evidence_count)) AS evidence_count,
			max(latest_seen) AS phase_last_seen
		FROM (
			SELECT alert_id,
				argMax(`+campaignAlertAttackPhaseExpression+`, last_seen) AS attack_phase,
				toUInt64(max(length(evidence_ids))) AS evidence_count,
				max(last_seen) AS latest_seen
			FROM traffic.alerts
			WHERE tenant_id=? AND (campaign_id=? OR has(?, alert_id))
			GROUP BY alert_id
		)
		GROUP BY attack_phase
		ORDER BY phase_last_seen`, tenantID, campaignID, alertIDs)
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

func campaignEvidenceSummary(
	alerts []campaignAlertDTO,
	phases []campaignPhaseDTO,
	phaseDataBacked bool,
	responseRecordCount uint64,
	responseRecordsAvailable bool,
) []campaignEvidenceSummaryDTO {
	alertCount := uint64(len(alerts))
	evidenceCount := uint64(0)
	for _, phase := range phases {
		evidenceCount += phase.EvidenceCount
	}
	items := []campaignEvidenceSummaryDTO{
		{Key: "alerts", Label: "告警", Current: &alertCount, Available: true},
		{Key: "packet_session", Label: "PCAP / Session", Available: phaseDataBacked},
		{Key: "logs", Label: "日志", Available: false},
		{Key: "graph_paths", Label: "图谱路径", Available: false},
		{Key: "response_records", Label: "处置记录", Available: responseRecordsAvailable},
	}
	if phaseDataBacked {
		items[1].Current = &evidenceCount
	}
	if responseRecordsAvailable {
		items[4].Current = &responseRecordCount
	}
	return items
}

func (h *SystemHandler) queryCampaignAuditState(
	ctx context.Context,
	tenantID, campaignID string,
	tsStart, tsEnd int64,
	currentStatus, workbenchUpdatedAt string,
) ([]campaignStatusTransitionDTO, uint64, bool) {
	transitions := []campaignStatusTransitionDTO{{
		Status:    "new",
		ChangedAt: campaignTimestampRFC3339(tsStart),
		Source:    "campaign",
	}}
	if h.pgDB == nil {
		return appendCurrentCampaignTransition(transitions, currentStatus, workbenchUpdatedAt, tsEnd), 0, false
	}
	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT action,
			COALESCE(NULLIF(detail->'metadata'->>'next_status',''), NULLIF(detail->'result'->>'campaign_status',''), ''),
			created_at
		FROM audit_logs
		WHERE tenant_id=$1 AND object_type='campaign' AND object_id=$2
		  AND action IN ('CAMPAIGN_STATUS_CHANGED','CAMPAIGN_OWNER_ASSIGNED','CAMPAIGN_REPORT_REQUESTED','CAMPAIGN_SOAR_RESPONSE_REQUESTED')
		ORDER BY created_at`, tenantID, campaignID)
	if err != nil {
		return appendCurrentCampaignTransition(transitions, currentStatus, workbenchUpdatedAt, tsEnd), 0, false
	}
	defer rows.Close()

	var responseRecordCount uint64
	for rows.Next() {
		var action, status string
		var changedAt time.Time
		if err := rows.Scan(&action, &status, &changedAt); err != nil {
			return appendCurrentCampaignTransition(transitions, currentStatus, workbenchUpdatedAt, tsEnd), 0, false
		}
		responseRecordCount++
		if action == "CAMPAIGN_STATUS_CHANGED" && validCampaignWorkbenchStatus(strings.ToLower(strings.TrimSpace(status))) {
			transitions = append(transitions, campaignStatusTransitionDTO{
				Status:    strings.ToLower(strings.TrimSpace(status)),
				ChangedAt: changedAt.UTC().Format(time.RFC3339Nano),
				Source:    "audit_log",
			})
		}
	}
	if rows.Err() != nil {
		return appendCurrentCampaignTransition(transitions, currentStatus, workbenchUpdatedAt, tsEnd), 0, false
	}
	return appendCurrentCampaignTransition(transitions, currentStatus, workbenchUpdatedAt, tsEnd), responseRecordCount, true
}

func appendCurrentCampaignTransition(
	transitions []campaignStatusTransitionDTO,
	currentStatus, workbenchUpdatedAt string,
	tsEnd int64,
) []campaignStatusTransitionDTO {
	status := strings.ToLower(strings.TrimSpace(currentStatus))
	if !validCampaignWorkbenchStatus(status) {
		return transitions
	}
	for _, transition := range transitions {
		if transition.Status == status {
			return transitions
		}
	}
	changedAt := strings.TrimSpace(workbenchUpdatedAt)
	source := "workbench_state"
	if changedAt == "" {
		changedAt = campaignTimestampRFC3339(tsEnd)
		source = "campaign"
	}
	return append(transitions, campaignStatusTransitionDTO{Status: status, ChangedAt: changedAt, Source: source})
}

func campaignTimestampRFC3339(value int64) string {
	if value <= 0 {
		return ""
	}
	if value < 100_000_000_000 {
		return time.Unix(value, 0).UTC().Format(time.RFC3339Nano)
	}
	return time.UnixMilli(value).UTC().Format(time.RFC3339Nano)
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

const campaignAlertEntityExpression = `if(
	arrayExists(label -> match(lowerUTF8(label), '^entity[:=]'), labels),
	replaceRegexpOne(arrayFirst(label -> match(lowerUTF8(label), '^entity[:=]'), labels), '^[^:=]+[:=]', ''),
	''
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

func campaignToPhasesWithAlerts(campaign campaignDTO, alerts []campaignAlertDTO) []attackPhaseDTO {
	phases := make([]attackPhaseDTO, 0, len(campaign.AttackPhases))
	for _, phase := range campaign.AttackPhases {
		if phase == "" {
			continue
		}
		alertIDs := make([]string, 0)
		keyEvents := make([]attackEventDTO, 0)
		var phaseStart, phaseEnd int64
		for _, alert := range alerts {
			if alert.AttackPhase != "" && !strings.EqualFold(strings.TrimSpace(alert.AttackPhase), strings.TrimSpace(phase)) {
				continue
			}
			if alert.AlertID != "" {
				alertIDs = append(alertIDs, alert.AlertID)
			}
			if alert.LastSeen > 0 {
				if phaseStart == 0 || alert.LastSeen < phaseStart {
					phaseStart = alert.LastSeen
				}
				if alert.LastSeen > phaseEnd {
					phaseEnd = alert.LastSeen
				}
			}
			keyEvents = append(keyEvents, attackEventDTO{
				EventID: alert.AlertID, Timestamp: alert.LastSeen, Description: alert.AlertType,
				Entity: alert.Entity, SrcIP: alert.SrcIP, DstIP: alert.DstIP, Technique: mitreTechniqueForAttackPhase(phase),
				Severity: alert.Severity, EvidenceIDs: alert.EvidenceIDs,
			})
		}
		if phaseStart == 0 {
			phaseStart = campaign.TsStart
		}
		if phaseEnd == 0 {
			phaseEnd = campaign.TsEnd
		}
		phases = append(phases, attackPhaseDTO{
			Phase: phase, AlertIDs: alertIDs, StartTime: phaseStart, EndTime: phaseEnd,
			KeyEvents: keyEvents, Confidence: campaign.Score,
		})
	}
	return phases
}

func mitreTechniqueForAttackPhase(phase string) string {
	normalized := strings.ToLower(strings.TrimSpace(phase))
	switch {
	case strings.Contains(normalized, "recon"):
		return "TA0043"
	case strings.Contains(normalized, "initial"), strings.Contains(normalized, "access"):
		return "TA0001"
	case strings.Contains(normalized, "execution"):
		return "TA0002"
	case strings.Contains(normalized, "lateral"):
		return "TA0008"
	case strings.Contains(normalized, "command"), strings.Contains(normalized, "control"), strings.Contains(normalized, "c2"):
		return "TA0011"
	case strings.Contains(normalized, "exfil"):
		return "TA0010"
	default:
		return phase
	}
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
